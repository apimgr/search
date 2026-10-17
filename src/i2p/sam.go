package i2p

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
)

// samDestination is a SAMv3 destination pair: the binary destination the
// .b32.i2p address derives from, and the I2P-base64 private blob SESSION CREATE
// takes verbatim.
type samDestination struct {
	pub  []byte
	priv string
}

// readSAMReply reads one SAMv3 reply line and verifies it both starts with the
// expected command prefix and reports RESULT=OK.
func readSAMReply(r *bufio.Reader, expect string) (map[string]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read SAM reply for %q: %w", expect, err)
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, expect) {
		return nil, fmt.Errorf("unexpected SAM reply, want %q got %q", expect, line)
	}

	fields := map[string]string{}
	for _, tok := range splitSAMTokens(strings.TrimPrefix(line, expect)) {
		k, v, found := strings.Cut(tok, "=")
		if !found {
			continue
		}
		fields[k] = strings.Trim(v, `"`)
	}

	// DEST REPLY carries no RESULT field; every other reply must report OK.
	if result, ok := fields["RESULT"]; ok && result != "OK" {
		return nil, fmt.Errorf("SAM %s failed: RESULT=%s MESSAGE=%s", expect, result, fields["MESSAGE"])
	}
	return fields, nil
}

// splitSAMTokens splits a SAM reply body on spaces, keeping quoted values (such
// as MESSAGE="...") intact.
func splitSAMTokens(s string) []string {
	var tokens []string
	var cur strings.Builder
	quoted := false
	for _, r := range s {
		switch {
		case r == '"':
			quoted = !quoted
			cur.WriteRune(r)
		case r == ' ' && !quoted:
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

// writeSAMKeys persists the destination private blob with 0600 permissions so
// the .b32.i2p address survives restarts.
func writeSAMKeys(keysPath string, priv []byte) error {
	if err := os.MkdirAll(filepath.Dir(keysPath), 0700); err != nil {
		return fmt.Errorf("create i2p key dir: %w", err)
	}
	if err := os.WriteFile(keysPath, priv, 0600); err != nil {
		return fmt.Errorf("write i2p destination key: %w", err)
	}
	return os.Chmod(keysPath, 0600)
}

// loadOrCreateSAMDestination returns the persisted destination when
// {data_dir}/i2p/site/site-keys.dat exists, and otherwise asks the router for a
// fresh one via DEST GENERATE and persists it. The file holds the binary
// private blob, the same layout i2pd's site-keys.dat uses, so both providers
// read and write one format.
func loadOrCreateSAMDestination(conn net.Conn, r *bufio.Reader, keysPath string, signatureType int) (*samDestination, error) {
	if data, err := os.ReadFile(keysPath); err == nil {
		pub, err := destinationFromKeys(data)
		if err != nil {
			return nil, fmt.Errorf("persisted i2p destination at %s is unusable: %w", keysPath, err)
		}
		return &samDestination{pub: pub, priv: i2pBase64.EncodeToString(data)}, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read i2p destination key: %w", err)
	}

	if _, err := fmt.Fprintf(conn, "DEST GENERATE SIGNATURE_TYPE=%d\n", signatureType); err != nil {
		return nil, fmt.Errorf("send DEST GENERATE: %w", err)
	}
	fields, err := readSAMReply(r, "DEST REPLY")
	if err != nil {
		return nil, err
	}
	priv, ok := fields["PRIV"]
	if !ok {
		return nil, fmt.Errorf("SAM DEST REPLY carried no PRIV field")
	}

	raw, err := decodeI2PBase64(priv)
	if err != nil {
		return nil, err
	}
	pub, err := destinationFromKeys(raw)
	if err != nil {
		return nil, err
	}
	if err := writeSAMKeys(keysPath, raw); err != nil {
		return nil, err
	}
	return &samDestination{pub: pub, priv: priv}, nil
}

// startSAMEepsite opens a SAMv3 control connection, loads (or generates and
// persists) the destination, creates a STREAM session, and forwards incoming
// streams to the dedicated backend port. Returns the .b32.i2p address.
// Per AI.md PART 31.2 "SAM Session (Model B)".
func startSAMEepsite(cfg *I2PConfig, keysPath string, i2pBackendPort int, svc *I2PService) (string, error) {
	conn, err := net.DialTimeout("tcp", cfg.SAMAddress, 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to dial SAM %s: %w", cfg.SAMAddress, err)
	}
	r := bufio.NewReader(conn)

	// Bound the whole control exchange; the deadline is cleared before the
	// connection is handed to the service for the session's lifetime.
	if err := conn.SetDeadline(time.Now().Add(time.Duration(cfg.BootstrapTimeout) * time.Second)); err != nil {
		return "", errors.Join(err, conn.Close())
	}

	// 1. Handshake
	if _, err := fmt.Fprintf(conn, "HELLO VERSION MIN=3.0 MAX=3.3\n"); err != nil {
		return "", errors.Join(err, conn.Close())
	}
	if _, err := readSAMReply(r, "HELLO REPLY"); err != nil {
		closeutil.Close(conn)
		return "", err
	}

	// 2. Load persisted destination or generate + persist a new one.
	dest, err := loadOrCreateSAMDestination(conn, r, keysPath, cfg.SignatureType)
	if err != nil {
		closeutil.Close(conn)
		return "", err
	}

	// 3. Create the STREAM session bound to that destination.
	if _, err := fmt.Fprintf(conn, "SESSION CREATE STYLE=STREAM ID=site DESTINATION=%s "+
		"inbound.length=%d outbound.length=%d inbound.quantity=%d outbound.quantity=%d\n",
		dest.priv, cfg.InboundLength, cfg.OutboundLength, cfg.InboundQuantity, cfg.OutboundQuantity); err != nil {
		closeutil.Close(conn)
		return "", err
	}
	if _, err := readSAMReply(r, "SESSION STATUS"); err != nil {
		closeutil.Close(conn)
		return "", err
	}

	// 4. Forward incoming eepsite streams to the backend port.
	if _, err := fmt.Fprintf(conn, "STREAM FORWARD ID=site PORT=%d HOST=127.0.0.1\n", i2pBackendPort); err != nil {
		closeutil.Close(conn)
		return "", err
	}
	if _, err := readSAMReply(r, "STREAM STATUS"); err != nil {
		closeutil.Close(conn)
		return "", err
	}

	// Keep the control connection open for the session lifetime.
	if err := conn.SetDeadline(time.Time{}); err != nil {
		closeutil.Close(conn)
		return "", err
	}
	svc.samConn = conn
	return b32Address(dest.pub), nil
}
