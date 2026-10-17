package i2p

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	raw := newTestKeyBlob(4, 33)
	encoded := i2pBase64.EncodeToString(raw)
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

func TestGetI2PTunnelsConf(t *testing.T) {
	cfg := DefaultI2PConfig()
	conf := getI2PTunnelsConf(&cfg, "/data/i2p/site/site-keys.dat", 64123)

	for _, want := range []string{
		"[site]",
		"type = server",
		"host = 127.0.0.1",
		"port = 64123",
		"keys = /data/i2p/site/site-keys.dat",
		"inbound.length = 3",
		"outbound.length = 3",
		"inbound.quantity = 5",
		"outbound.quantity = 5",
		"signaturetype = 7",
	} {
		if !strings.Contains(conf, want) {
			t.Errorf("tunnels.conf missing %q:\n%s", want, conf)
		}
	}
	if !strings.HasSuffix(conf, "\n") {
		t.Error("tunnels.conf must end with a newline")
	}
}

func TestUpdateI2PTunnelsWritesRestrictedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "i2p", "tunnels.conf")
	if err := updateI2PTunnels(path, []byte("[site]\n")); err != nil {
		t.Fatalf("updateI2PTunnels: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("tunnels.conf perm = %o, want 600", perm)
	}
	// Regenerating on every startup must overwrite cleanly.
	if err := updateI2PTunnels(path, []byte("[site]\nport = 1\n")); err != nil {
		t.Fatalf("regenerate tunnels.conf: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "port = 1") {
		t.Error("tunnels.conf was not regenerated")
	}
}

func TestResolveI2PDBinaryHonorsExplicitPath(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "i2pd")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\n"), 0700); err != nil {
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
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	if !samReachable(listener.Addr().String()) {
		t.Error("samReachable = false for a live listener")
	}

	listener.Close()
	if samReachable(listener.Addr().String()) {
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
	if perm := info.Mode().Perm(); perm != 0600 {
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
	t.Setenv("SEARCH_CONFIG_DIR", filepath.Join(root, "config"))
	t.Setenv("SEARCH_DATA_DIR", filepath.Join(root, "data"))
	t.Setenv("SEARCH_LOG_DIR", filepath.Join(root, "log"))
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
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
			t.Errorf("%s perm = %o, want 700", dir, info.Mode().Perm())
		}
	}
	// Running twice must be a no-op, since it runs on every startup.
	if err := ensureI2PDirs(); err != nil {
		t.Fatalf("second ensureI2PDirs: %v", err)
	}
}

func TestStartI2PDModelA(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake i2pd stand-in is a POSIX shell script")
	}
	useTempDirs(t)
	if err := ensureI2PDirs(); err != nil {
		t.Fatalf("ensureI2PDirs: %v", err)
	}

	root := t.TempDir()
	blob := newTestKeyBlob(4, 640)
	blobPath := filepath.Join(root, "keys.fixture")
	if err := os.WriteFile(blobPath, blob, 0600); err != nil {
		t.Fatalf("write key fixture: %v", err)
	}

	keysPath := filepath.Join(config.GetDataDir(), "i2p", "site", "site-keys.dat")

	// Stand-in for i2pd: publishes the destination key the way i2pd does, then
	// stays alive until it is signalled.
	fake := filepath.Join(root, "i2pd")
	script := "#!/bin/sh\ncp " + blobPath + " " + keysPath + "\nexec sleep 60\n"
	if err := os.WriteFile(fake, []byte(script), 0700); err != nil {
		t.Fatalf("write fake i2pd: %v", err)
	}

	cfg := DefaultI2PConfig()
	cfg.Enabled = true
	cfg.Binary = fake
	cfg.BootstrapTimeout = 20

	svc := &I2PService{provider: I2PProviderI2PD}
	addr, err := startI2PD(context.Background(), &cfg, fake, keysPath, 64123, svc)
	if err != nil {
		t.Fatalf("startI2PD: %v", err)
	}
	defer svc.Close()

	dest, err := destinationFromKeys(blob)
	if err != nil {
		t.Fatalf("destinationFromKeys: %v", err)
	}
	if addr != b32Address(dest) {
		t.Errorf("eepsite address = %q, want %q", addr, b32Address(dest))
	}
	if !i2pProviderHealthy(svc) {
		t.Error("i2pProviderHealthy = false while the i2pd stand-in is running")
	}

	// tunnels.conf is derived state regenerated on every startup.
	conf, err := os.ReadFile(filepath.Join(config.GetConfigDir(), "i2p", "tunnels.conf"))
	if err != nil {
		t.Fatalf("read tunnels.conf: %v", err)
	}
	if !strings.Contains(string(conf), "port = 64123") {
		t.Errorf("tunnels.conf does not carry the backend port:\n%s", conf)
	}

	if err := svc.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if i2pProviderHealthy(svc) {
		t.Error("i2pProviderHealthy = true after the provider was shut down")
	}
}

func TestStartI2PDTimesOutWithoutDestination(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake i2pd stand-in is a POSIX shell script")
	}
	useTempDirs(t)
	if err := ensureI2PDirs(); err != nil {
		t.Fatalf("ensureI2PDirs: %v", err)
	}

	root := t.TempDir()
	fake := filepath.Join(root, "i2pd")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexec sleep 60\n"), 0700); err != nil {
		t.Fatalf("write fake i2pd: %v", err)
	}

	cfg := DefaultI2PConfig()
	cfg.Enabled = true
	cfg.Binary = fake
	cfg.BootstrapTimeout = 1

	svc := &I2PService{provider: I2PProviderI2PD}
	if _, err := startI2PD(context.Background(), &cfg, fake,
		filepath.Join(config.GetDataDir(), "i2p", "site", "site-keys.dat"), 64123, svc); err == nil {
		svc.Close()
		t.Fatal("expected a bootstrap timeout when no destination is published")
	}
}

// startFakeSAM runs a minimal SAMv3 router that answers each command with the
// reply mapped to its first two words, so the Model B flow can be exercised
// without a real I2P router.
func startFakeSAM(t *testing.T, replies map[string]string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen fake SAM: %v", err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					fields := strings.Fields(line)
					if len(fields) < 2 {
						return
					}
					reply, ok := replies[fields[0]+" "+fields[1]]
					if !ok {
						return
					}
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
	addr := startFakeSAM(t, map[string]string{
		"HELLO VERSION":  "HELLO REPLY RESULT=OK VERSION=3.3\n",
		"DEST GENERATE":  "DEST REPLY PUB=ignored PRIV=" + i2pBase64.EncodeToString(blob) + "\n",
		"SESSION CREATE": "SESSION STATUS RESULT=OK DESTINATION=ignored\n",
		"STREAM FORWARD": "STREAM STATUS RESULT=OK\n",
	})

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

	// The destination must be persisted so the address survives a restart.
	persisted, err := os.ReadFile(keysPath)
	if err != nil {
		t.Fatalf("read persisted keys: %v", err)
	}
	if string(persisted) != string(blob) {
		t.Error("persisted destination does not match the generated one")
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
	if _, err := startSAMEepsite(&cfg, filepath.Join(t.TempDir(), "site-keys.dat"), 64123, svc); err == nil {
		t.Fatal("expected an error when SESSION CREATE fails")
	}
	if svc.samConn != nil {
		t.Error("the SAM connection must be closed when the session fails")
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

func TestManagerStartDisabledReturnsError(t *testing.T) {
	cfg := DefaultI2PConfig()
	mgr := NewI2PManager(context.Background(), &cfg, "127.0.0.1:8080")
	if err := mgr.Start(); err == nil {
		t.Error("expected Start to report that I2P is opt-in and disabled")
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

func TestWaitForI2PDAddressTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site-keys.dat")
	start := time.Now()
	if _, err := waitForI2PDAddress(context.Background(), path, 50*time.Millisecond); err == nil {
		t.Fatal("expected a timeout when i2pd never writes the destination")
	}
	if time.Since(start) > 30*time.Second {
		t.Error("waitForI2PDAddress overshot its timeout")
	}
}

func TestWaitForI2PDAddressReadsPersistedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site-keys.dat")
	blob := newTestKeyBlob(4, 640)
	if err := os.WriteFile(path, blob, 0600); err != nil {
		t.Fatalf("write key blob: %v", err)
	}
	addr, err := waitForI2PDAddress(context.Background(), path, 5*time.Second)
	if err != nil {
		t.Fatalf("waitForI2PDAddress: %v", err)
	}
	dest, err := destinationFromKeys(blob)
	if err != nil {
		t.Fatalf("destinationFromKeys: %v", err)
	}
	if addr != b32Address(dest) {
		t.Errorf("address = %q, want %q", addr, b32Address(dest))
	}
}
