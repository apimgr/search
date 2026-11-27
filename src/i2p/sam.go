package i2p

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var samKeyWriteMu sync.Mutex

var (
	errSAMInvalidKey     = errors.New("SAM rejected the persisted I2P destination key")
	errSAMDuplicatedDest = errors.New("SAM destination is already in use")
	errSAMTooOld          = errors.New("SAM 3.1 or newer is required")
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
	fields, err := readSAMReplyFields(r, expect)
	if err != nil {
		return nil, err
	}
	if result, ok := fields["RESULT"]; ok && result != "OK" {
		return nil, fmt.Errorf("SAM %s failed: RESULT=%s", expect, result)
	}
	return fields, nil
}

func readSAMReplyFields(r *bufio.Reader, expect string) (map[string]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("read SAM reply for %q: %w", expect, err)
	}
	line = strings.TrimRight(line, "\r\n")
	if !strings.HasPrefix(line, expect) {
		fields := strings.Fields(line)
		got := "unknown"
		if len(fields) >= 2 {
			got = fields[0] + " " + fields[1]
		}
		return nil, fmt.Errorf("unexpected SAM reply command: want %q, got %q", expect, got)
	}

	fields := map[string]string{}
	for _, tok := range splitSAMTokens(strings.TrimPrefix(line, expect)) {
		k, v, found := strings.Cut(tok, "=")
		if !found {
			continue
		}
		fields[k] = strings.Trim(v, `"`)
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
func writeSAMKeys(keysPath string, priv []byte) (resultErr error) {
	samKeyWriteMu.Lock()
	defer samKeyWriteMu.Unlock()
	dir := filepath.Dir(keysPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create i2p key dir: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".site-keys-*")
	if err != nil {
		return fmt.Errorf("create temporary I2P key file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		if err := os.Remove(tempPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove temporary I2P key file: %w", err))
		}
	}()
	if err := temp.Chmod(0600); err != nil {
		return errors.Join(err, temp.Close())
	}
	if _, err := fmt.Fprintln(temp, i2pBase64.EncodeToString(priv)); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Sync(); err != nil {
		return errors.Join(err, temp.Close())
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, keysPath); err != nil {
		return fmt.Errorf("publish I2P destination key: %w", err)
	}
	if err := os.Chmod(keysPath, 0600); err != nil {
		return err
	}
	dirHandle, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(dirHandle.Sync(), dirHandle.Close())
}

func readSAMKeys(keysPath string) ([]byte, error) {
	data, err := os.ReadFile(keysPath)
	if err != nil {
		return nil, err
	}
	value := strings.TrimSpace(string(data))
	if raw, decodeErr := decodeI2PBase64(value); decodeErr == nil {
		return raw, nil
	}
	// Earlier releases stored the decoded key blob directly. Convert that
	// existing identity to the canonical one-line SAM PRIV representation.
	if _, err := destinationFromKeys(data); err != nil {
		return nil, fmt.Errorf("persisted I2P destination at %s is unusable: %w", keysPath, err)
	}
	if err := writeSAMKeys(keysPath, data); err != nil {
		return nil, fmt.Errorf("migrate persisted I2P destination at %s: %w", keysPath, err)
	}
	return data, nil
}

// loadOrCreateSAMDestination returns the persisted destination when
// {data_dir}/i2p/site/site-keys.dat exists, and otherwise asks the router for a
// fresh one via DEST GENERATE and persists it as one SAM PRIV base64 line.
func loadOrCreateSAMDestination(conn net.Conn, r *bufio.Reader, keysPath string, signatureType int) (*samDestination, error) {
	if _, err := os.Stat(keysPath); err == nil {
		data, err := readSAMKeys(keysPath)
		if err != nil {
			return nil, err
		}
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
// Per AI.md PART 31.2 "SAM Session (both models)".
func startSAMEepsite(cfg *I2PConfig, keysPath string, i2pBackendPort int, svc *I2PService) (string, error) {
	return startSAMEepsiteWithBuilding(cfg, keysPath, i2pBackendPort, svc, nil)
}

func startSAMEepsiteWithBuilding(cfg *I2PConfig, keysPath string, i2pBackendPort int, svc *I2PService, onBuilding func()) (string, error) {
	return startSAMEepsiteWithContext(context.Background(), cfg, keysPath, i2pBackendPort, svc, onBuilding)
}

func startSAMEepsiteWithContext(ctx context.Context, cfg *I2PConfig, keysPath string, i2pBackendPort int, svc *I2PService, onBuilding func()) (string, error) {
	conn, err := dialSAM(cfg.SAMAddress, 10*time.Second)
	if err != nil {
		return "", fmt.Errorf("failed to dial SAM %s: %w", cfg.SAMAddress, err)
	}
	stopControlOnCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopControlOnCancel()
	r := bufio.NewReader(conn)
	closeControl := func(err error) (string, error) {
		return "", errors.Join(err, conn.Close())
	}
	if err := samHello(conn, r, 10*time.Second); err != nil {
		return closeControl(err)
	}
	dest, err := loadOrCreateSAMDestination(conn, r, keysPath, cfg.SignatureType)
	if err != nil {
		return closeControl(err)
	}
	if err := conn.SetDeadline(time.Now().Add(time.Duration(cfg.BootstrapTimeout) * time.Second)); err != nil {
		return closeControl(err)
	}
	if onBuilding != nil {
		onBuilding()
	}

	sessionID := fmt.Sprintf("site-%d", os.Getpid())
	created := false
	for attempt := 0; attempt < 2; attempt++ {
		id := sessionID
		if attempt > 0 {
			id += "-retry"
		}
		sessionID = id
		if _, err := fmt.Fprintf(conn, "SESSION CREATE STYLE=STREAM ID=%s DESTINATION=%s i2cp.leaseSetEncType=4 inbound.length=%d outbound.length=%d inbound.quantity=%d outbound.quantity=%d\n",
			id, dest.priv, cfg.InboundLength, cfg.OutboundLength, cfg.InboundQuantity, cfg.OutboundQuantity); err != nil {
			return closeControl(err)
		}
		fields, err := readSAMReplyFields(r, "SESSION STATUS")
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				return closeControl(errors.Join(errors.New("I2P tunnels not ready"), err))
			}
			return closeControl(err)
		}
		if fields["RESULT"] == "OK" {
			created = true
			break
		}
		if fields["RESULT"] != "DUPLICATED_ID" || attempt > 0 {
			switch fields["RESULT"] {
			case "INVALID_KEY":
				return closeControl(fmt.Errorf("SAM SESSION CREATE failed: %w", errSAMInvalidKey))
			case "DUPLICATED_DEST":
				return closeControl(fmt.Errorf("SAM SESSION CREATE failed: %w", errSAMDuplicatedDest))
			default:
				return closeControl(fmt.Errorf("SAM SESSION CREATE failed: RESULT=%s", fields["RESULT"]))
			}
		}
	}
	if !created {
		return closeControl(errors.New("SAM SESSION CREATE failed after duplicated-ID retry"))
	}

	forwardConn, err := dialSAM(cfg.SAMAddress, 10*time.Second)
	if err != nil {
		return closeControl(fmt.Errorf("failed to open SAM forward socket: %w", err))
	}
	stopForwardOnCancel := context.AfterFunc(ctx, func() { _ = forwardConn.Close() })
	defer stopForwardOnCancel()
	forwardReader := bufio.NewReader(forwardConn)
	if err := samHello(forwardConn, forwardReader, 10*time.Second); err != nil {
		return "", errors.Join(err, forwardConn.Close(), conn.Close())
	}
	if _, err := fmt.Fprintf(forwardConn, "STREAM FORWARD ID=%s PORT=%d HOST=127.0.0.1 SILENT=true\n", sessionID, i2pBackendPort); err != nil {
		return "", errors.Join(err, forwardConn.Close(), conn.Close())
	}
	if _, err := readSAMReply(forwardReader, "STREAM STATUS"); err != nil {
		return "", errors.Join(err, forwardConn.Close(), conn.Close())
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return "", errors.Join(err, forwardConn.Close(), conn.Close())
	}
	if err := forwardConn.SetDeadline(time.Time{}); err != nil {
		return "", errors.Join(err, forwardConn.Close(), conn.Close())
	}
	svc.samConn = conn
	svc.forwardConn = forwardConn
	svc.samAddress = cfg.SAMAddress
	svc.samSessionLost = make(chan struct{})
	for _, socket := range []net.Conn{conn, forwardConn} {
		go func(socket net.Conn) {
			var probe [1]byte
			_, _ = socket.Read(probe[:])
			svc.samLossOnce.Do(func() { close(svc.samSessionLost) })
		}(socket)
	}
	return b32Address(dest.pub), nil
}

func dialSAM(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return conn, nil
}

func samHello(conn net.Conn, reader *bufio.Reader, timeout time.Duration) error {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	if _, err := fmt.Fprint(conn, "HELLO VERSION MIN=3.1 MAX=3.3\n"); err != nil {
		return err
	}
	fields, err := readSAMReplyFields(reader, "HELLO REPLY")
	if err != nil {
		return err
	}
	if fields["RESULT"] != "OK" {
		if fields["RESULT"] == "NOVERSION" {
			return fmt.Errorf("SAM router is too old: %w", errSAMTooOld)
		}
		return fmt.Errorf("SAM HELLO VERSION failed: RESULT=%s", fields["RESULT"])
	}
	switch fields["VERSION"] {
	case "3.1", "3.2", "3.3":
	default:
		return fmt.Errorf("SAM router protocol version %q is unsupported; SAM 3.1 or newer is required", fields["VERSION"])
	}
	return nil
}

func waitForSAMBridge(ctx context.Context, addr string, exited <-chan struct{}) error {
	deadlineAt := time.Now().Add(60 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadlineAt) {
		deadlineAt = contextDeadline
	}
	deadline := time.NewTimer(time.Until(deadlineAt))
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		remaining := time.Until(deadlineAt)
		if remaining <= 0 {
			return errors.New("i2pd SAM bridge did not become ready within 60 seconds")
		}
		dialTimeout := min(time.Second, remaining)
		conn, err := net.DialTimeout("tcp", addr, dialTimeout)
		if err == nil {
			helloTimeout := min(2*time.Second, time.Until(deadlineAt))
			if helloTimeout > 0 {
				err = samHello(conn, bufio.NewReader(conn), helloTimeout)
			} else {
				err = context.DeadlineExceeded
			}
			closeErr := conn.Close()
			if err == nil {
				return closeErr
			}
			if errors.Is(err, errSAMTooOld) {
				return errors.Join(err, closeErr)
			}
		}
		select {
		case <-exited:
			return errors.New("i2pd exited before its SAM bridge became ready")
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("i2pd SAM bridge did not become ready within 60 seconds")
		case <-ticker.C:
		}
	}
}

func probeSAMBridge(addr string) (resultErr error) {
	conn, err := dialSAM(addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() {
		if err := conn.Close(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close SAM liveness probe connection: %w", err))
		}
	}()
	if err := samHello(conn, bufio.NewReader(conn), 5*time.Second); err != nil {
		return err
	}
	if _, err := fmt.Fprint(conn, "QUIT\n"); err != nil {
		return fmt.Errorf("SAM liveness probe QUIT failed: %w", err)
	}
	return nil
}
