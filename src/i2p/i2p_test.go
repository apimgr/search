package i2p

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

// newTestKeyBlob builds a synthetic I2P private-key blob: a destination with a
// certificate of certLen bytes followed by privLen bytes of private key data.
func newTestKeyBlob(certLen, privLen int) []byte {
	blob := make([]byte, minDestinationLen+certLen+privLen)
	for i := range blob {
		blob[i] = byte(i % 251)
	}
	// Certificate header at offset 384: [type:1][length:2]
	blob[384] = 5
	blob[385] = byte(certLen >> 8)
	blob[386] = byte(certLen & 0xff)
	return blob
}

func TestDefaultI2PConfigIsOptIn(t *testing.T) {
	cfg := DefaultI2PConfig()
	if cfg.Enabled {
		t.Fatal("I2P must be disabled by default per AI.md PART 31.2")
	}
	if cfg.SAMAddress != "127.0.0.1:7656" {
		t.Errorf("SAMAddress = %q, want 127.0.0.1:7656", cfg.SAMAddress)
	}
	if cfg.VirtualPort != 80 {
		t.Errorf("VirtualPort = %d, want 80", cfg.VirtualPort)
	}
	if cfg.SignatureType != 7 {
		t.Errorf("SignatureType = %d, want 7", cfg.SignatureType)
	}
	if cfg.BootstrapTimeout != 300 {
		t.Errorf("BootstrapTimeout = %d, want 300", cfg.BootstrapTimeout)
	}
}

func TestSAMHelloRejectsLegacyProtocolWithActionableError(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		reader := bufio.NewReader(server)
		_, _ = reader.ReadString('\n')
		_, _ = server.Write([]byte("HELLO REPLY RESULT=NOVERSION\n"))
	}()
	err := samHello(client, bufio.NewReader(client), time.Second)
	if err == nil || !strings.Contains(err.Error(), "SAM 3.1 or newer is required") {
		t.Fatalf("SAM NOVERSION error = %v, want actionable minimum-version message", err)
	}
}

func TestNormalizeClampsOutOfRangeValues(t *testing.T) {
	cfg := I2PConfig{
		Enabled:          true,
		VirtualPort:      0,
		InboundLength:    9,
		OutboundLength:   -1,
		InboundQuantity:  99,
		OutboundQuantity: 0,
		SignatureType:    0,
		BootstrapTimeout: 5,
	}
	cfg.Normalize()

	def := DefaultI2PConfig()
	if cfg.SAMAddress != def.SAMAddress {
		t.Errorf("SAMAddress = %q, want %q", cfg.SAMAddress, def.SAMAddress)
	}
	if cfg.VirtualPort != def.VirtualPort {
		t.Errorf("VirtualPort = %d, want %d", cfg.VirtualPort, def.VirtualPort)
	}
	if cfg.InboundLength != def.InboundLength || cfg.OutboundLength != def.OutboundLength {
		t.Errorf("tunnel lengths = %d/%d, want %d/%d",
			cfg.InboundLength, cfg.OutboundLength, def.InboundLength, def.OutboundLength)
	}
	if cfg.InboundQuantity != def.InboundQuantity || cfg.OutboundQuantity != def.OutboundQuantity {
		t.Errorf("tunnel quantities = %d/%d, want %d/%d",
			cfg.InboundQuantity, cfg.OutboundQuantity, def.InboundQuantity, def.OutboundQuantity)
	}
	if cfg.SignatureType != def.SignatureType {
		t.Errorf("SignatureType = %d, want %d", cfg.SignatureType, def.SignatureType)
	}
	if cfg.BootstrapTimeout != def.BootstrapTimeout {
		t.Errorf("BootstrapTimeout = %d, want %d", cfg.BootstrapTimeout, def.BootstrapTimeout)
	}
	if !cfg.Enabled {
		t.Error("Normalize must not change the opt-in flag")
	}
}

func TestNormalizeKeepsValidValues(t *testing.T) {
	cfg := I2PConfig{
		SAMAddress:       "10.0.0.1:7656",
		VirtualPort:      8080,
		InboundLength:    0,
		OutboundLength:   7,
		InboundQuantity:  1,
		OutboundQuantity: 16,
		SignatureType:    11,
		BootstrapTimeout: 30,
	}
	want := cfg
	cfg.Normalize()
	if cfg != want {
		t.Errorf("Normalize altered valid config: %+v, want %+v", cfg, want)
	}
}

func TestB32Address(t *testing.T) {
	dest := []byte("i2p-destination-fixture")
	got := b32Address(dest)

	sum := sha256.Sum256(dest)
	want := strings.ToLower(
		base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]),
	) + ".b32.i2p"

	if got != want {
		t.Errorf("b32Address = %q, want %q", got, want)
	}
	if !strings.HasSuffix(got, ".b32.i2p") {
		t.Errorf("b32Address = %q, must end in .b32.i2p", got)
	}
	if strings.Contains(got, "=") {
		t.Errorf("b32Address = %q, must be unpadded", got)
	}
}

func TestB32AddressIsStable(t *testing.T) {
	dest := newTestKeyBlob(4, 0)
	first := b32Address(dest)
	second := b32Address(dest)
	if first != second {
		t.Errorf("b32Address changed between calls: %q != %q", first, second)
	}
}

func TestB32AddressMatchesKnownDestinationVector(t *testing.T) {
	destination := make([]byte, minDestinationLen)
	destination[384] = 5
	const want = "eowetcgkd524au55atzcrwt5fckargv5yipooge6ldj33h4xolcq.b32.i2p"
	if got := b32Address(destination); got != want {
		t.Fatalf("b32Address() = %q, want vector %q", got, want)
	}
}

func TestDestinationFromKeys(t *testing.T) {
	blob := newTestKeyBlob(4, 640)
	dest, err := destinationFromKeys(blob)
	if err != nil {
		t.Fatalf("destinationFromKeys: %v", err)
	}
	if len(dest) != minDestinationLen+4 {
		t.Errorf("destination length = %d, want %d", len(dest), minDestinationLen+4)
	}
}

func TestDestinationFromKeysNullCertificate(t *testing.T) {
	blob := newTestKeyBlob(0, 640)
	dest, err := destinationFromKeys(blob)
	if err != nil {
		t.Fatalf("destinationFromKeys: %v", err)
	}
	if len(dest) != minDestinationLen {
		t.Errorf("destination length = %d, want %d", len(dest), minDestinationLen)
	}
}

func TestDestinationFromKeysRejectsShortBlob(t *testing.T) {
	if _, err := destinationFromKeys(make([]byte, 100)); err == nil {
		t.Error("expected an error for a blob shorter than a destination")
	}
}

func TestDestinationFromKeysRejectsTruncatedCertificate(t *testing.T) {
	blob := newTestKeyBlob(0, 0)
	// Declare a certificate the blob does not actually carry.
	blob[385] = 0
	blob[386] = 32
	if _, err := destinationFromKeys(blob); err == nil {
		t.Error("expected an error for a truncated certificate")
	}
}

func TestDecodeI2PBase64RoundTrip(t *testing.T) {
	raw := []byte{0, 15, 191}
	encoded := i2pBase64.EncodeToString(raw)
	if !strings.Contains(encoded, "-") || !strings.Contains(encoded, "~") {
		t.Fatalf("I2P base64 encoding %q did not exercise the - and ~ alphabet", encoded)
	}
	decoded, err := decodeI2PBase64(encoded)
	if err != nil {
		t.Fatalf("decodeI2PBase64: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Error("round trip through the I2P base64 alphabet lost data")
	}
}

func TestDecodeI2PBase64AcceptsUnpadded(t *testing.T) {
	raw := []byte{1, 2, 3, 4, 5}
	encoded := strings.TrimRight(i2pBase64.EncodeToString(raw), "=")
	decoded, err := decodeI2PBase64(encoded)
	if err != nil {
		t.Fatalf("decodeI2PBase64: %v", err)
	}
	if string(decoded) != string(raw) {
		t.Errorf("decoded = %v, want %v", decoded, raw)
	}
}

func TestDecodeI2PBase64RejectsGarbage(t *testing.T) {
	if _, err := decodeI2PBase64("!!!!"); err == nil {
		t.Error("expected an error for a non-base64 string")
	}
}

func TestUpdateI2PTunnelsWritesRestrictedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "i2p", "tunnels.conf")
	if err := updateI2PTunnels(path, nil); err != nil {
		t.Fatalf("updateI2PTunnels: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("tunnels.conf perm = %o, want 600", perm)
	}
	// Regenerating on every startup must overwrite cleanly.
	if err := updateI2PTunnels(path, []byte("stale tunnel config")); err != nil {
		t.Fatalf("regenerate tunnels.conf: %v", err)
	}
	if err := updateI2PTunnels(path, nil); err != nil {
		t.Fatalf("clear tunnels.conf: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("tunnels.conf = %q, want an empty tunnel definition", data)
	}
}

func TestGetI2PDConfDisablesNonSAMServices(t *testing.T) {
	conf := getI2PDConf(64321, "/var/log/search/i2pd.log")
	for _, want := range []string{
		"daemon = false", "notransit = true", "floodfill = false",
		"logfile = /var/log/search/i2pd.log", "[sam]", "enabled = true",
		"address = 127.0.0.1", "port = 64321", "[http]", "[httpproxy]",
		"[socksproxy]", "[bob]", "[i2cp]", "[i2pcontrol]", "[upnp]",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("i2pd.conf missing %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "enabled = true\n[httpproxy]") || strings.Contains(conf, "[site]") {
		t.Fatalf("i2pd.conf enables an unrelated service or tunnel:\n%s", conf)
	}
}

func TestResolveI2PDBinaryHonorsExplicitPath(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "i2pd")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	cfg := I2PConfig{Binary: fake}
	got, err := resolveI2PDBinary(&cfg)
	if err != nil {
		t.Fatalf("resolveI2PDBinary: %v", err)
	}
	if got != fake {
		t.Errorf("resolveI2PDBinary = %q, want %q", got, fake)
	}
}

func TestResolveI2PDBinaryRejectsMissingExplicitPath(t *testing.T) {
	cfg := I2PConfig{Binary: filepath.Join(t.TempDir(), "absent")}
	if _, err := resolveI2PDBinary(&cfg); err == nil {
		t.Error("expected an error when the configured i2pd binary is absent")
	}
}

func TestSamReachable(t *testing.T) {
	addr := startFakeSAM(t, map[string]string{
		"HELLO VERSION": "HELLO REPLY RESULT=OK VERSION=3.3\n",
	})
	if !samReachable(addr) {
		t.Error("samReachable = false for a live SAMv3 bridge")
	}
	tooOld := startFakeSAM(t, map[string]string{
		"HELLO VERSION": "HELLO REPLY RESULT=NOVERSION\n",
	})
	if !samReachable(tooOld) {
		t.Error("samReachable = false for a SAM bridge that needs the actionable old-version error")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen temporary address: %v", err)
	}
	dead := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close temporary listener: %v", err)
	}
	if samReachable(dead) {
		t.Error("samReachable = true for a closed listener")
	}
}

func TestProviderName(t *testing.T) {
	cases := map[I2PProvider]string{
		I2PProviderNone: "none",
		I2PProviderI2PD: "i2pd",
		I2PProviderSAM:  "sam",
	}
	for provider, want := range cases {
		if got := providerName(provider); got != want {
			t.Errorf("providerName(%d) = %q, want %q", provider, got, want)
		}
	}
}

func TestSplitSAMTokensKeepsQuotedMessage(t *testing.T) {
	tokens := splitSAMTokens(` RESULT=I2P_ERROR MESSAGE="no router here"`)
	if len(tokens) != 2 {
		t.Fatalf("tokens = %v, want 2", tokens)
	}
	if tokens[1] != `MESSAGE="no router here"` {
		t.Errorf("tokens[1] = %q", tokens[1])
	}
}

func TestReadSAMReply(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("HELLO REPLY RESULT=OK VERSION=3.3\n"))
	fields, err := readSAMReply(r, "HELLO REPLY")
	if err != nil {
		t.Fatalf("readSAMReply: %v", err)
	}
	if fields["VERSION"] != "3.3" {
		t.Errorf("VERSION = %q, want 3.3", fields["VERSION"])
	}
}

func TestReadSAMReplyRejectsFailure(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(`SESSION STATUS RESULT=DUPLICATED_ID MESSAGE="in use"` + "\n"))
	if _, err := readSAMReply(r, "SESSION STATUS"); err == nil {
		t.Error("expected an error for RESULT != OK")
	}
}

func TestReadSAMReplyRejectsWrongCommand(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("HELLO REPLY RESULT=OK\n"))
	if _, err := readSAMReply(r, "SESSION STATUS"); err == nil {
		t.Error("expected an error for an unexpected reply prefix")
	}
}

func TestWriteSAMKeysUses0600(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site", "site-keys.dat")
	if err := writeSAMKeys(path, []byte("key-material")); err != nil {
		t.Fatalf("writeSAMKeys: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("site-keys.dat perm = %o, want 600", perm)
	}
}

func TestLoadOrCreateSAMDestinationReusesPersistedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site-keys.dat")
	blob := newTestKeyBlob(4, 640)
	if err := writeSAMKeys(path, blob); err != nil {
		t.Fatalf("writeSAMKeys: %v", err)
	}

	// A persisted key must be reused without touching the router, so a nil
	// connection and an empty reader are enough here.
	dest, err := loadOrCreateSAMDestination(nil, bufio.NewReader(strings.NewReader("")), path, 7)
	if err != nil {
		t.Fatalf("loadOrCreateSAMDestination: %v", err)
	}
	if len(dest.pub) != minDestinationLen+4 {
		t.Errorf("destination length = %d, want %d", len(dest.pub), minDestinationLen+4)
	}
	if dest.priv != i2pBase64.EncodeToString(blob) {
		t.Error("persisted private blob was not passed through verbatim")
	}
}

func TestLoadOrCreateSAMDestinationMigratesLegacyBinaryKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site-keys.dat")
	blob := newTestKeyBlob(4, 640)
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		t.Fatalf("write legacy key fixture: %v", err)
	}
	dest, err := loadOrCreateSAMDestination(nil, bufio.NewReader(strings.NewReader("")), path, 7)
	if err != nil {
		t.Fatalf("loadOrCreateSAMDestination: %v", err)
	}
	if dest.priv != i2pBase64.EncodeToString(blob) {
		t.Fatal("legacy key identity changed during migration")
	}
	persisted, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated key: %v", err)
	}
	if string(persisted) != dest.priv+"\n" {
		t.Fatalf("migrated key file = %q, want one SAM PRIV base64 line", persisted)
	}
}

func TestLoadOrCreateSAMDestinationRejectsCorruptKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site-keys.dat")
	if err := writeSAMKeys(path, []byte("too short")); err != nil {
		t.Fatalf("writeSAMKeys: %v", err)
	}
	if _, err := loadOrCreateSAMDestination(nil, bufio.NewReader(strings.NewReader("")), path, 7); err == nil {
		t.Error("expected an error for a corrupt persisted destination")
	}
}

// useTempDirs redirects the config, data and log directories into the test's
// own temp dir so directory creation and file writes never touch the host.
func useTempDirs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CONFIG_DIR", filepath.Join(root, "config"))
	t.Setenv("DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("LOG_DIR", filepath.Join(root, "log"))
	return root
}

func TestEnsureI2PDirs(t *testing.T) {
	useTempDirs(t)
	if err := ensureI2PDirs(); err != nil {
		t.Fatalf("ensureI2PDirs: %v", err)
	}
	for _, dir := range []string{
		filepath.Join(config.GetConfigDir(), "i2p"),
		filepath.Join(config.GetDataDir(), "i2p"),
		filepath.Join(config.GetDataDir(), "i2p", "site"),
	} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Errorf("%s perm = %o, want 700", dir, info.Mode().Perm())
		}
	}
	// Running twice must be a no-op, since it runs on every startup.
	if err := ensureI2PDirs(); err != nil {
		t.Fatalf("second ensureI2PDirs: %v", err)
	}
}

type samCommandObservation struct {
	client  string
	command string
}

func startFakeSAM(t *testing.T, replies map[string]string) string {
	return startFakeSAMWithRecorder(t, replies, nil)
}

func startFakeSAMWithRecorder(t *testing.T, replies map[string]string, commands chan<- samCommandObservation) string {
	sequences := make(map[string][]string, len(replies))
	for key, reply := range replies {
		sequences[key] = []string{reply}
	}
	return startFakeSAMWithSequence(t, sequences, commands)
}

func startFakeSAMWithSequence(t *testing.T, replies map[string][]string, commands chan<- samCommandObservation) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake SAM: %v", err)
	}
	t.Cleanup(func() { listener.Close() })
	var replyMu sync.Mutex
	replyCounts := make(map[string]int)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				client := conn.RemoteAddr().String()
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					fields := strings.Fields(line)
					if commands != nil {
						select {
						case commands <- samCommandObservation{client: client, command: strings.TrimSpace(line)}:
						default:
						}
					}
					if len(fields) < 2 {
						return
					}
					key := fields[0] + " " + fields[1]
					replyMu.Lock()
					responses, ok := replies[key]
					if !ok || len(responses) == 0 {
						replyMu.Unlock()
						return
					}
					index := replyCounts[key]
					if index >= len(responses) {
						index = len(responses) - 1
					} else {
						replyCounts[key] = index + 1
					}
					reply := responses[index]
					replyMu.Unlock()
					if _, err := io.WriteString(conn, reply); err != nil {
						return
					}
				}
			}()
		}
	}()
	return listener.Addr().String()
}

func TestStartSAMEepsiteFullFlow(t *testing.T) {
	blob := newTestKeyBlob(4, 640)
	commands := make(chan samCommandObservation, 16)
	addr := startFakeSAMWithRecorder(t, map[string]string{
		"HELLO VERSION":  "HELLO REPLY RESULT=OK VERSION=3.3\n",
		"DEST GENERATE":  "DEST REPLY PUB=ignored PRIV=" + i2pBase64.EncodeToString(blob) + "\n",
		"SESSION CREATE": "SESSION STATUS RESULT=OK DESTINATION=ignored\n",
		"STREAM FORWARD": "STREAM STATUS RESULT=OK\n",
	}, commands)

	cfg := DefaultI2PConfig()
	cfg.SAMAddress = addr
	keysPath := filepath.Join(t.TempDir(), "site", "site-keys.dat")

	svc := &I2PService{}
	got, err := startSAMEepsite(&cfg, keysPath, 64123, svc)
	if err != nil {
		t.Fatalf("startSAMEepsite: %v", err)
	}
	defer svc.Close()

	dest, err := destinationFromKeys(blob)
	if err != nil {
		t.Fatalf("destinationFromKeys: %v", err)
	}
	if got != b32Address(dest) {
		t.Errorf("eepsite address = %q, want %q", got, b32Address(dest))
	}
	if svc.samConn == nil {
		t.Error("the SAM control connection must stay open for the session lifetime")
	}
	if svc.forwardConn == nil {
		t.Error("the separate SAM forward connection must stay open for the session lifetime")
	}
	if svc.samConn != nil && svc.forwardConn != nil && svc.samConn == svc.forwardConn {
		t.Error("SAM control and forward roles must use distinct sockets")
	}
	var sessionClient, forwardClient string
	destIndex, sessionIndex := -1, -1
	commandIndex := 0
	for range len(commands) {
		observation := <-commands
		if strings.HasPrefix(observation.command, "DEST GENERATE ") && destIndex == -1 {
			destIndex = commandIndex
		}
		if strings.HasPrefix(observation.command, "SESSION CREATE ") {
			if sessionIndex == -1 {
				sessionIndex = commandIndex
			}
			sessionClient = observation.client
		}
		if strings.HasPrefix(observation.command, "STREAM FORWARD ") {
			forwardClient = observation.client
			if !strings.Contains(observation.command, "SILENT=true") {
				t.Error("STREAM FORWARD must set SILENT=true")
			}
		}
		commandIndex++
	}
	if sessionClient == "" || forwardClient == "" || sessionClient == forwardClient {
		t.Fatalf("SESSION CREATE and STREAM FORWARD sockets = %q and %q, want distinct sockets", sessionClient, forwardClient)
	}
	if destIndex < 0 || sessionIndex < 0 || destIndex >= sessionIndex {
		t.Fatalf("destination key generation index=%d, session index=%d; key must be persisted before session creation", destIndex, sessionIndex)
	}

	// The destination must be persisted so the address survives a restart.
	persisted, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatalf("read persisted keys: %v", err)
	}
	if string(persisted) != i2pBase64.EncodeToString(blob)+"\n" {
		t.Error("persisted destination is not one canonical SAM PRIV base64 line")
	}
}

func TestStartSAMEepsiteRejectsSessionFailure(t *testing.T) {
	blob := newTestKeyBlob(0, 640)
	addr := startFakeSAM(t, map[string]string{
		"HELLO VERSION":  "HELLO REPLY RESULT=OK VERSION=3.3\n",
		"DEST GENERATE":  "DEST REPLY PUB=ignored PRIV=" + i2pBase64.EncodeToString(blob) + "\n",
		"SESSION CREATE": `SESSION STATUS RESULT=DUPLICATED_DEST MESSAGE="already in use"` + "\n",
	})

	cfg := DefaultI2PConfig()
	cfg.SAMAddress = addr
	svc := &I2PService{}
	if _, err := startSAMEepsite(&cfg, filepath.Join(t.TempDir(), "site-keys.dat"), 64123, svc); !errors.Is(err, errSAMDuplicatedDest) {
		t.Fatalf("SESSION CREATE error = %v, want duplicated destination classification", err)
	}
	if svc.samConn != nil {
		t.Error("the SAM connection must be closed when the session fails")
	}
}

func TestStartSAMEepsiteRetriesDuplicatedSessionID(t *testing.T) {
	blob := newTestKeyBlob(0, 640)
	commands := make(chan samCommandObservation, 16)
	addr := startFakeSAMWithSequence(t, map[string][]string{
		"HELLO VERSION": {
			"HELLO REPLY RESULT=OK VERSION=3.3\n",
			"HELLO REPLY RESULT=OK VERSION=3.3\n",
		},
		"DEST GENERATE": {"DEST REPLY PRIV=" + i2pBase64.EncodeToString(blob) + "\n"},
		"SESSION CREATE": {
			"SESSION STATUS RESULT=DUPLICATED_ID MESSAGE=collision\n",
			"SESSION STATUS RESULT=OK\n",
		},
		"STREAM FORWARD": {"STREAM STATUS RESULT=OK\n"},
	}, commands)
	cfg := DefaultI2PConfig()
	cfg.SAMAddress = addr
	svc := &I2PService{}
	if _, err := startSAMEepsite(&cfg, filepath.Join(t.TempDir(), "site-keys.dat"), 64123, svc); err != nil {
		t.Fatalf("startSAMEepsite: %v", err)
	}
	defer svc.Close()
	var firstID, retryID, forwardID string
	for range len(commands) {
		observation := <-commands
		fields := strings.Fields(observation.command)
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "SESSION" && fields[1] == "CREATE" {
			for _, field := range fields {
				if strings.HasPrefix(field, "ID=") {
					if firstID == "" {
						firstID = strings.TrimPrefix(field, "ID=")
					} else {
					retryID = strings.TrimPrefix(field, "ID=")
					}
				}
			}
		}
		if fields[0] == "STREAM" && fields[1] == "FORWARD" {
			for _, field := range fields {
				if strings.HasPrefix(field, "ID=") {
					forwardID = strings.TrimPrefix(field, "ID=")
				}
			}
		}
	}
	if firstID == "" || retryID != firstID+"-retry" || forwardID != retryID {
		t.Fatalf("session ids first=%q retry=%q forward=%q; want one retry and matching forward ID", firstID, retryID, forwardID)
	}
}

func TestStartSAMEepsiteDoesNotRegenerateInvalidPersistedKey(t *testing.T) {
	blob := newTestKeyBlob(0, 640)
	path := filepath.Join(t.TempDir(), "site-keys.dat")
	if err := writeSAMKeys(path, blob); err != nil {
		t.Fatalf("write persisted key: %v", err)
	}
	commands := make(chan samCommandObservation, 8)
	addr := startFakeSAMWithSequence(t, map[string][]string{
		"HELLO VERSION": {"HELLO REPLY RESULT=OK VERSION=3.3\n"},
		"SESSION CREATE": {"SESSION STATUS RESULT=INVALID_KEY MESSAGE=corrupt\n"},
	}, commands)
	cfg := DefaultI2PConfig()
	cfg.SAMAddress = addr
	if _, err := startSAMEepsite(&cfg, path, 64123, &I2PService{}); !errors.Is(err, errSAMInvalidKey) {
		t.Fatalf("INVALID_KEY error = %v, want invalid key classification", err)
	}
	for range len(commands) {
		if command := <-commands; strings.HasPrefix(command.command, "DEST GENERATE") {
			t.Fatal("invalid persisted key was silently regenerated")
		}
	}
	persisted, err := readSAMKeys(path)
	if err != nil {
		t.Fatalf("read persisted key after failure: %v", err)
	}
	if string(persisted) != string(blob) {
		t.Fatal("invalid-key failure changed the persisted destination")
	}
}

func TestStartSAMEepsiteRejectsUnreachableRouter(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := listener.Addr().String()
	listener.Close()

	cfg := DefaultI2PConfig()
	cfg.SAMAddress = dead
	if _, err := startSAMEepsite(&cfg, filepath.Join(t.TempDir(), "site-keys.dat"), 64123, &I2PService{}); err == nil {
		t.Error("expected an error when the SAM router is unreachable")
	}
}

func TestStartDedicatedI2PWithoutProvider(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := listener.Addr().String()
	listener.Close()

	cfg := DefaultI2PConfig()
	cfg.Enabled = true
	// Point at a path that cannot resolve so i2pd is never selected.
	cfg.Binary = filepath.Join(t.TempDir(), "absent-i2pd")
	cfg.SAMAddress = dead

	svc, err := startDedicatedI2P(context.Background(), &cfg, "127.0.0.1:8080")
	if err == nil {
		t.Fatal("expected an error when no provider is available")
	}
	if svc != nil {
		t.Fatal("no service may be created without a provider")
	}
	if !strings.Contains(err.Error(), "no provider available") {
		t.Errorf("error = %q, want it to name the missing provider", err)
	}
}

func TestI2PBackendRelaysToClearnetListener(t *testing.T) {
	upstream, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen upstream: %v", err)
	}
	defer upstream.Close()

	go func() {
		conn, err := upstream.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 5)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return
		}
		conn.Write([]byte("pong:" + string(buf)))
	}()

	backend, err := newI2PBackend(0, upstream.Addr().String())
	if err != nil {
		t.Fatalf("newI2PBackend: %v", err)
	}
	defer backend.Close()

	if backend.Port() == 0 {
		t.Fatal("backend Port = 0")
	}

	conn, err := net.DialTimeout("tcp", backend.listener.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatalf("dial backend: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "pong:hello" {
		t.Errorf("relayed response = %q, want %q", got, "pong:hello")
	}
}

func TestI2PBackendCloseIsIdempotent(t *testing.T) {
	backend, err := newI2PBackend(0, "127.0.0.1:1")
	if err != nil {
		t.Fatalf("newI2PBackend: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestI2PBackendIsEepsiteConn(t *testing.T) {
	backend, err := newI2PBackend(0, "127.0.0.1:1")
	if err != nil {
		t.Fatalf("newI2PBackend: %v", err)
	}
	defer backend.Close()
	if backend.IsEepsiteConn("127.0.0.1:12345") {
		t.Error("IsEepsiteConn = true for an address that never came through the eepsite")
	}
}

func TestStartDedicatedI2PDisabledIsNoOp(t *testing.T) {
	cfg := DefaultI2PConfig()
	svc, err := startDedicatedI2P(context.Background(), &cfg, "127.0.0.1:8080")
	if err == nil {
		t.Fatal("expected an error when I2P is disabled")
	}
	if svc != nil {
		t.Fatal("no service may be created when I2P is disabled")
	}
	if !strings.Contains(err.Error(), "opt-in") {
		t.Errorf("error = %q, want it to name the opt-in gate", err)
	}
}

func TestManagerDisabledReportsDisabled(t *testing.T) {
	cfg := DefaultI2PConfig()
	mgr := NewI2PManager(context.Background(), &cfg, "127.0.0.1:8080")

	if mgr.Enabled() {
		t.Error("Enabled = true for the default (opt-in) config")
	}
	if mgr.IsRunning() {
		t.Error("IsRunning = true with no provider started")
	}
	if got := mgr.Status(); got != "Disabled" {
		t.Errorf("Status = %q, want Disabled", got)
	}
	if got := mgr.EepsiteAddress(); got != "" {
		t.Errorf("EepsiteAddress = %q, want empty", got)
	}
	if got := mgr.ProviderName(); got != "none" {
		t.Errorf("ProviderName = %q, want none", got)
	}
	if mgr.IsEepsiteConn("127.0.0.1:1") {
		t.Error("IsEepsiteConn = true with no eepsite running")
	}
	if err := mgr.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestManagerStartDisabledIsSilentNoop(t *testing.T) {
	cfg := DefaultI2PConfig()
	mgr := NewI2PManager(context.Background(), &cfg, "127.0.0.1:8080")
	if err := mgr.Start(); err != nil {
		t.Errorf("Start() with disabled I2P returned error: %v", err)
	}
	if mgr.Status() != "Disabled" {
		t.Errorf("Status() = %q, want Disabled", mgr.Status())
	}
}

func TestManagerUpdateConfigToDisabledSucceeds(t *testing.T) {
	cfg := DefaultI2PConfig()
	mgr := NewI2PManager(context.Background(), &cfg, "127.0.0.1:8080")

	off := DefaultI2PConfig()
	if err := mgr.UpdateConfig(&off); err != nil {
		t.Fatalf("UpdateConfig to disabled: %v", err)
	}
	if mgr.IsRunning() {
		t.Error("eepsite is running after being disabled")
	}
}

func TestManagerMonitorReturnsWhenDisabled(t *testing.T) {
	cfg := DefaultI2PConfig()
	mgr := NewI2PManager(context.Background(), &cfg, "127.0.0.1:8080")

	done := make(chan struct{})
	go func() {
		mgr.Monitor()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Monitor did not return immediately while I2P is disabled")
	}
}

func TestManagerCheckAndRestartNoopsWhenDisabled(t *testing.T) {
	cfg := DefaultI2PConfig()
	mgr := NewI2PManager(context.Background(), &cfg, "127.0.0.1:8080")
	if err := mgr.CheckAndRestart(); err != nil {
		t.Fatalf("CheckAndRestart() returned error while disabled: %v", err)
	}
	if got := mgr.LifecycleState(); got != "disabled" {
		t.Fatalf("LifecycleState() = %q, want disabled", got)
	}
}

func TestManagerRetryBackoffIsCapped(t *testing.T) {
	manager := NewI2PManager(context.Background(), &I2PConfig{Enabled: true}, "127.0.0.1:8080")
	for _, want := range []time.Duration{5, 10, 20, 40, 80, 160, 300, 300} {
		manager.mu.Lock()
		manager.scheduleRetryLocked()
		got := manager.retryDelay
		manager.mu.Unlock()
		if got != want*time.Second {
			t.Fatalf("retry delay = %s, want %s", got, want*time.Second)
		}
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if !manager.retryNeeded || time.Until(manager.nextRetry) <= 0 {
		t.Fatal("retry scheduling did not set a future retry time")
	}
}

func TestManagerRetryPolicyForSAMSessionErrors(t *testing.T) {
	t.Run("invalid key does not retry", func(t *testing.T) {
		manager := NewI2PManager(context.Background(), &I2PConfig{Enabled: true}, "127.0.0.1:8080")
		manager.mu.Lock()
		manager.scheduleRetryForErrorLocked(errSAMInvalidKey)
		defer manager.mu.Unlock()
		if manager.retryNeeded || !manager.nextRetry.IsZero() {
			t.Fatal("INVALID_KEY must wait for operator address regeneration, not retry")
		}
	})
	t.Run("duplicate destination backs off one minute", func(t *testing.T) {
		manager := NewI2PManager(context.Background(), &I2PConfig{Enabled: true}, "127.0.0.1:8080")
		manager.mu.Lock()
		manager.scheduleRetryForErrorLocked(errSAMDuplicatedDest)
		gotDelay := manager.retryDelay
		gotRetry := manager.retryNeeded
		gotNext := manager.nextRetry
		manager.mu.Unlock()
		if gotDelay != 60*time.Second || !gotRetry || time.Until(gotNext) <= 0 {
			t.Fatalf("duplicate destination retry delay=%s retry=%v next=%v", gotDelay, gotRetry, gotNext)
		}
	})
}

func TestManagerEepsiteAddressAvailableDuringStartup(t *testing.T) {
	cfg := DefaultI2PConfig()
	cfg.Enabled = true
	mgr := NewI2PManager(context.Background(), &cfg, "127.0.0.1:8080")
	mgr.dataDir = t.TempDir()
	keysPath := filepath.Join(mgr.dataDir, "site", "site-keys.dat")
	if err := os.MkdirAll(filepath.Dir(keysPath), 0o700); err != nil {
		t.Fatal(err)
	}
	keys := newTestKeyBlob(4, 640)
	if err := os.WriteFile(keysPath, keys, 0o600); err != nil {
		t.Fatal(err)
	}
	destination, err := destinationFromKeys(keys)
	if err != nil {
		t.Fatal(err)
	}
	want := b32Address(destination)
	if got := mgr.EepsiteAddress(); got != want {
		t.Fatalf("EepsiteAddress() = %q, want %q", got, want)
	}
}

func TestManagerMonitorStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := DefaultI2PConfig()
	cfg.Enabled = true
	mgr := NewI2PManager(ctx, &cfg, "127.0.0.1:8080")

	done := make(chan struct{})
	go func() {
		mgr.monitorWithInterval(10 * time.Millisecond)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Monitor did not stop after the context was cancelled")
	}
}

func TestI2PProviderHealthyRejectsNilAndNone(t *testing.T) {
	if i2pProviderHealthy(nil) {
		t.Error("i2pProviderHealthy(nil) = true")
	}
	if i2pProviderHealthy(&I2PService{provider: I2PProviderNone}) {
		t.Error("i2pProviderHealthy with no provider = true")
	}
	if i2pProviderHealthy(&I2PService{provider: I2PProviderI2PD}) {
		t.Error("i2pProviderHealthy with no i2pd process = true")
	}
	if i2pProviderHealthy(&I2PService{provider: I2PProviderSAM}) {
		t.Error("i2pProviderHealthy with no SAM connection = true")
	}
}

func TestServiceAccessorsOnEmptyService(t *testing.T) {
	svc := &I2PService{}
	if svc.EepsiteAddress() != "" {
		t.Error("EepsiteAddress must be empty before the eepsite starts")
	}
	if svc.Provider() != I2PProviderNone {
		t.Error("Provider must be I2PProviderNone before a provider is resolved")
	}
	if svc.ProviderName() != "none" {
		t.Error("ProviderName must be none before a provider is resolved")
	}
	if svc.BackendPort() != 0 {
		t.Error("BackendPort must be 0 before a port is allocated")
	}
	if svc.IsEepsiteConn("127.0.0.1:1") {
		t.Error("IsEepsiteConn = true with no backend listener")
	}
	if svc.IsRunning() {
		t.Error("IsRunning = true with no provider")
	}
	if err := svc.Close(); err != nil {
		t.Errorf("Close on an unstarted service: %v", err)
	}
}
