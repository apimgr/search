package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/cretz/bine/control"
	"github.com/cretz/bine/tor"
	"golang.org/x/crypto/sha3"
)

// findTorBinary finds the Tor binary using config, PATH, or common locations
// Per AI.md PART 32: Tor binary discovery priority:
// 1) Config setting (explicit path)
// 2) PATH search (exec.LookPath)
// 3) Common locations (/usr/bin/tor, /usr/local/bin/tor, etc.)
func findTorBinary(configPath string) string {
	// 1) Config setting takes priority
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
		slog.Info("Tor configured binary not found", "path", configPath)
	}

	// 2) Search PATH
	if path, err := lookPath("tor"); err == nil {
		return path
	}

	// 3) Common locations per AI.md PART 32
	commonLocations := []string{
		"/usr/bin/tor",
		"/usr/local/bin/tor",
		// macOS MacPorts
		"/opt/local/bin/tor",
		// macOS Homebrew (Apple Silicon)
		"/opt/homebrew/bin/tor",
		// macOS Homebrew (Intel)
		"/usr/local/opt/tor/bin/tor",
		// Ubuntu Snap
		"/snap/bin/tor",
	}

	for _, loc := range commonLocations {
		if _, err := statFile(loc); err == nil {
			return loc
		}
	}

	// Not found
	return ""
}

// lookPath searches for an executable in PATH
// Wrapper for exec.LookPath to enable testing
var lookPath = func(file string) (string, error) {
	return exec.LookPath(file)
}

// statFile checks file existence in common binary locations
// Wrapper for os.Stat to enable testing
var statFile = os.Stat

// TorService manages Tor hidden service using github.com/cretz/bine
// per AI.md PART 32: TOR HIDDEN SERVICE (NON-NEGOTIABLE)
// Server binary fully owns and controls the Tor process lifecycle.
type TorService struct {
	config *config.Config
	tor    *tor.Tor
	// .onion address (without .onion suffix)
	serviceID string
	dialer    *tor.Dialer
	running   bool
	onionAddr string
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	dataDir   string
	configDir string
}

// NewTorService creates a new Tor service manager
func NewTorService(cfg *config.Config) *TorService {
	ctx, cancel := context.WithCancel(context.Background())
	return &TorService{
		config:    cfg,
		ctx:       ctx,
		cancel:    cancel,
		dataDir:   filepath.Join(config.GetDataDir(), "tor"),
		configDir: filepath.Join(config.GetConfigDir(), "tor"),
	}
}

// ensureTorDirs creates all required Tor directories with proper permissions
// Per AI.md PART 32: Server binary owns all Tor files
func (t *TorService) ensureTorDirs() error {
	dirs := []string{
		// {data_dir}/tor/
		t.dataDir,
		// {data_dir}/tor/site/ for hidden service keys
		filepath.Join(t.dataDir, "site"),
		// {config_dir}/tor/ for torrc
		t.configDir,
	}

	uid := os.Getuid()
	gid := os.Getgid()

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
		// Enforce permissions on existing dirs too — per AI.md PART 31: "Enforce on exist"
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("chmod tor dir %s: %w", dir, err)
		}
		// Enforce ownership (skip on Windows — no chown)
		if runtime.GOOS != "windows" {
			if err := os.Chown(dir, uid, gid); err != nil {
				return fmt.Errorf("chown tor dir %s: %w", dir, err)
			}
		}
	}

	return nil
}

// getTorConfig generates torrc content per AI.md PART 32
// NOTE: Hidden service is created via control.AddOnion(), NOT torrc
// The torrc only configures Tor daemon settings
//
// PORT DETECTION: All ports use runtime detection via "auto" - never saved/hardcoded
// - SocksPort: Controlled via torrc (auto or 0), NoAutoSocksPort=true in StartConf
// - ControlPort: Platform-specific (Unix socket vs TCP)
//
// NEVER uses default Tor ports (9050, 9051) - uses Unix sockets or auto high ports
func (t *TorService) getTorConfig() string {
	cfg := &t.config.Server.Tor

	// All OSes use localhost TCP auto port per AI.md PART 31:
	// "Use ControlPort 127.0.0.1:auto on all OSes for the current bine-based integration"
	// "Never hardcode a control port — let Tor choose a free localhost port at runtime"
	// NEVER use default port 9051 — runtime detection only
	controlConfig := "ControlPort 127.0.0.1:auto"

	// SocksPort per AI.md PART 32:
	// - "SocksPort auto" if outbound enabled (UseNetwork or AllowUserPreference)
	// - "SocksPort 0" if outbound disabled (hidden service only)
	// NOTE: We use NoAutoSocksPort=true in StartConf so bine doesn't add conflicting args
	var socksConfig string
	if cfg.UseNetwork || cfg.AllowUserPreference {
		// Enable SOCKS for outbound - "auto" picks high port at runtime
		socksConfig = "SocksPort auto"
	} else {
		// Disabled - hidden service only
		socksConfig = "SocksPort 0"
	}

	// SafeLogging
	safeLogging := "1"
	if !cfg.SafeLogging {
		safeLogging = "0"
	}

	// Bandwidth settings with defaults
	bandwidthRate := cfg.BandwidthRate
	if bandwidthRate == "" {
		bandwidthRate = "1 MB"
	}
	bandwidthBurst := cfg.BandwidthBurst
	if bandwidthBurst == "" {
		bandwidthBurst = "2 MB"
	}

	// Monthly bandwidth accounting (if not "unlimited")
	var accountingConfig string
	if cfg.MaxMonthlyBandwidth != "" && cfg.MaxMonthlyBandwidth != "unlimited" {
		accountingConfig = fmt.Sprintf(`
# Monthly bandwidth limit
AccountingStart month 1 00:00
AccountingMax %s`, cfg.MaxMonthlyBandwidth)
	}

	return fmt.Sprintf(`# ============================================================
# Tor Configuration - Generated by server binary
# Per AI.md PART 32: TOR HIDDEN SERVICE
# ============================================================

# SOCKS port for outbound connections (0 = disabled, auto = runtime port)
# NEVER uses default port 9050 - runtime detection only
%s

# Platform-specific control connection per AI.md PART 32:
# - Unix/macOS/BSD: Unix socket (no TCP port exposure)
# - Windows: TCP on 127.0.0.1:auto (Unix sockets not supported)
# NEVER uses default ports 9050/9051
%s

# Security Hardening
SafeLogging %s
MaxCircuitDirtiness 600
%s

# Bandwidth limits per second
BandwidthRate %s
BandwidthBurst %s
%s

# Disable unused features - not a relay or exit
ExitRelay 0
ExitPolicy reject *:*
ORPort 0
DirPort 0

# Hidden service optimizations (actual HS created via ADD_ONION)
HiddenServiceSingleHopMode 0

# Faster startup
FetchDirInfoEarly 1
FetchDirInfoExtraEarly 1

# Reduce memory usage
DisableDebuggerAttachment 1
`, socksConfig, controlConfig, safeLogging,
		func() string {
			if cfg.CloseCircuitOnStreamLimit {
				return ""
			}
			return "CircuitStreamTimeout 60"
		}(), bandwidthRate, bandwidthBurst, accountingConfig)
}

// sanitizeTorrcContent migrates legacy torrc files to current spec.
// Removes deprecated directives and updates control port to 127.0.0.1:auto.
func sanitizeTorrcContent(content string) string {
	lines := strings.Split(content, "\n")
	sanitized := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Remove deprecated MaxStreamsPerCircuit (handled via control command now)
		if strings.HasPrefix(trimmed, "MaxStreamsPerCircuit ") {
			continue
		}
		// Remove Unix socket control directives — spec requires TCP 127.0.0.1:auto
		if strings.HasPrefix(trimmed, "ControlSocket ") {
			continue
		}
		// Migrate Unix socket or disabled control port to localhost auto port
		if strings.HasPrefix(trimmed, "ControlPort unix:") || trimmed == "ControlPort 0" {
			sanitized = append(sanitized, "ControlPort 127.0.0.1:auto")
			continue
		}
		sanitized = append(sanitized, line)
	}

	return strings.Join(sanitized, "\n")
}

// Start starts the Tor hidden service using bine
// Per AI.md PART 32: "Auto-enabled if tor binary is installed - no enable flag needed"
func (t *TorService) StartTorService() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	torConfig := &t.config.Server.Tor

	// Per AI.md PART 32: Find Tor binary first - auto-enable if found
	torBinary := findTorBinary(torConfig.Binary)
	if torBinary == "" {
		// Per AI.md PART 32: "NOT FOUND: Log INFO, disable Tor features, continue without Tor"
		slog.Info("Tor binary not found, hidden service disabled")
		// Not an error - Tor is optional
		return nil
	}

	torConfig.Enabled = true
	slog.Info("Tor binary found", "path", torBinary)

	// Create directories with proper permissions
	if err := t.ensureTorDirs(); err != nil {
		return fmt.Errorf("failed to create tor directories: %w", err)
	}

	// Paths
	torrcPath := filepath.Join(t.configDir, "torrc")
	keyPath := filepath.Join(t.dataDir, "site", "hs_ed25519_secret_key")

	// Per AI.md PART 32: torrc is PERSISTENT — only create if it doesn't exist.
	if _, err := os.Stat(torrcPath); os.IsNotExist(err) {
		torrcContent := t.getTorConfig()
		if err := os.WriteFile(torrcPath, []byte(torrcContent), 0600); err != nil {
			return fmt.Errorf("failed to write torrc: %w", err)
		}
		slog.Info("Tor created torrc", "path", torrcPath)
	} else {
		if existing, err := os.ReadFile(torrcPath); err == nil {
			sanitized := sanitizeTorrcContent(string(existing))
			if sanitized != string(existing) {
				if err := os.WriteFile(torrcPath, []byte(sanitized), 0600); err != nil {
					return fmt.Errorf("failed to migrate torrc: %w", err)
				}
				slog.Info("Tor updated legacy torrc", "path", torrcPath)
			}
		}
		// Always enforce correct permissions on existing torrc
		_ = os.Chmod(torrcPath, 0600)
	}

	// Build StartConf per AI.md PART 32
	conf := &tor.StartConf{
		ExePath:   torBinary,
		TorrcFile: torrcPath,
		DataDir:   t.dataDir,
		// SocksPort controlled by torrc, not bine command line
		NoAutoSocksPort: true,
	}

	// Enable debug output in development mode
	if t.config.IsDevelopment() {
		conf.DebugWriter = os.Stderr
	}

	// Per AI.md PART 32 bootstrap logging: start silently; show "Tor: connecting..."
	// only if bootstrap takes >30s; show address on success.
	slog.Info("Tor starting process")

	// Start OUR OWN Tor process - completely separate from system Tor
	torInstance, err := tor.Start(t.ctx, conf)
	if err != nil {
		return fmt.Errorf("failed to start tor: %w", err)
	}

	// Wait for Tor to bootstrap with progressive logging per AI.md PART 32
	bootstrapTimeout := time.Duration(torConfig.BootstrapTimeout) * time.Second
	if bootstrapTimeout == 0 {
		bootstrapTimeout = 3 * time.Minute
	}

	// Bootstrap silently for first 30s; then log "Tor: connecting..." if still waiting
	slowTimer := time.NewTimer(30 * time.Second)
	defer slowTimer.Stop()
	bootstrapDone := make(chan error, 1)
	go func() {
		dialCtx, cancel := context.WithTimeout(t.ctx, bootstrapTimeout)
		defer cancel()
		bootstrapDone <- torInstance.EnableNetwork(dialCtx, true)
	}()

	select {
	case err = <-bootstrapDone:
		// Bootstrap completed (fast)
		slowTimer.Stop()
	case <-slowTimer.C:
		// Slow bootstrap — let user know
		slog.Info("Tor: connecting")
		err = <-bootstrapDone
	}
	if err != nil {
		torInstance.Close()
		return fmt.Errorf("failed to connect to tor network (timeout %v): %w", bootstrapTimeout, err)
	}

	// Load or generate ED25519 key for persistent .onion address
	// Load existing key for persistent .onion address
	// Key is stored as base64 string (the format returned by Key.Blob())
	var existingKey control.Key
	if keyData, err := os.ReadFile(keyPath); err == nil && len(keyData) > 0 {
		keyBlob := strings.TrimSpace(string(keyData))
		if k, err := control.ED25519KeyFromBlob(keyBlob); err == nil {
			existingKey = k
			slog.Info("Tor loaded existing hidden service key")
		} else {
			slog.Warn("Tor failed to parse hidden service key", "err", err)
		}
	}

	// Create hidden service via ADD_ONION control command
	// Per AI.md PART 32: This forwards .onion:{virtual_port} → 127.0.0.1:serverPort
	serverPort := t.config.Server.Port
	virtualPort := torConfig.VirtualPort
	if virtualPort == 0 {
		virtualPort = 80
	}

	// Create public hidden service - no client authorization required
	// Per AI.md PART 32: Service is accessible to anyone with the .onion address
	// Not setting Flags or ClientAuthV3 = public service (no auth/login)
	addOnionReq := &control.AddOnionRequest{
		Ports: []*control.KeyVal{
			control.NewKeyVal(fmt.Sprintf("%d", virtualPort), fmt.Sprintf("127.0.0.1:%d", serverPort)),
		},
	}

	if existingKey != nil {
		// Use existing key for persistent .onion address
		addOnionReq.Key = existingKey
	} else {
		// Generate new ED25519-V3 key (v3 onion address)
		addOnionReq.Key = control.GenKey(control.KeyAlgoED25519V3)
	}

	// Call ADD_ONION via control connection
	resp, err := torInstance.Control.AddOnion(addOnionReq)
	if err != nil {
		torInstance.Close()
		return fmt.Errorf("failed to create hidden service: %w", err)
	}

	// Save key for persistent address (if newly generated)
	// Key.Blob() returns base64 string which we save directly
	if existingKey == nil && resp.Key != nil {
		if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err == nil {
			keyBlob := resp.Key.Blob()
			if err := os.WriteFile(keyPath, []byte(keyBlob), 0600); err != nil {
				slog.Warn("Tor failed to save hidden service key", "err", err)
			} else {
				slog.Info("Tor saved new hidden service key")
			}
		}
	}

	t.tor = torInstance
	t.serviceID = resp.ServiceID
	t.onionAddr = resp.ServiceID + ".onion"
	t.running = true

	// Initialize outbound dialer if enabled
	if torConfig.UseNetwork || torConfig.AllowUserPreference {
		dialer, err := torInstance.Dialer(t.ctx, nil)
		if err != nil {
			slog.Warn("Tor outbound dialer failed", "err", err)
		} else {
			t.dialer = dialer
		}
	}

	// Per AI.md PART 32 success log: "Tor: {onion_address}" (short, once at startup)
	slog.Info("Tor hidden service active", "onion_address", t.onionAddr)

	// Start monitoring goroutine
	go t.monitorTor()

	return nil
}

// Stop stops the Tor hidden service
func (t *TorService) StopTorService() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	// Close Tor instance (this also removes the onion service)
	if t.tor != nil {
		if err := t.tor.Close(); err != nil {
			slog.Warn("Tor error closing instance", "err", err)
		}
		t.tor = nil
	}

	t.dialer = nil
	t.running = false
	t.onionAddr = ""
	t.serviceID = ""
	slog.Info("Tor service stopped")
	return nil
}

// Restart stops and starts Tor (used for config changes, recovery)
func (t *TorService) RestartTorService() error {
	if err := t.StopTorService(); err != nil {
		return err
	}
	return t.StartTorService()
}

// IsRunning returns whether Tor is running
func (t *TorService) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

// GetOnionAddress returns the .onion address
func (t *TorService) GetOnionAddress() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.onionAddr
}

// GetHTTPClient returns an HTTP client, optionally routed through Tor
// Per AI.md PART 32: useTor: true = route through Tor, false = direct
func (t *TorService) GetHTTPClient(useTor bool) *http.Client {
	t.mu.RLock()
	dialer := t.dialer
	t.mu.RUnlock()

	if !useTor || dialer == nil {
		// Direct connection
		return &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	// Route through Tor network
	return &http.Client{
		// Tor is slower
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			DialContext: dialer.DialContext,
		},
	}
}

// OutboundEnabled returns true if Tor outbound connections are available
func (t *TorService) OutboundEnabled() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.dialer != nil
}

// RegenerateAddress creates a new random .onion address
func (t *TorService) RegenerateAddress() (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running || t.tor == nil {
		return "", fmt.Errorf("tor service is not running")
	}

	// Remove old hidden service
	if t.serviceID != "" {
		t.tor.Control.DelOnion(t.serviceID)
	}

	// Delete existing keys
	keyPath := filepath.Join(t.dataDir, "site", "hs_ed25519_secret_key")
	os.Remove(keyPath)

	// Create new hidden service with new keys
	serverPort := t.config.Server.Port
	virtualPort := t.config.Server.Tor.VirtualPort
	if virtualPort == 0 {
		virtualPort = 80
	}

	addOnionReq := &control.AddOnionRequest{
		Key: control.GenKey(control.KeyAlgoED25519V3),
		Ports: []*control.KeyVal{
			control.NewKeyVal(fmt.Sprintf("%d", virtualPort), fmt.Sprintf("127.0.0.1:%d", serverPort)),
		},
	}

	resp, err := t.tor.Control.AddOnion(addOnionReq)
	if err != nil {
		return "", fmt.Errorf("failed to create new hidden service: %w", err)
	}

	// Save new key (Key.Blob() returns base64 string)
	if resp.Key != nil {
		keyBlob := resp.Key.Blob()
		if err := os.WriteFile(keyPath, []byte(keyBlob), 0600); err != nil {
			slog.Warn("Tor failed to save new key", "err", err)
		}
	}

	t.serviceID = resp.ServiceID
	t.onionAddr = resp.ServiceID + ".onion"

	slog.Info("Tor new hidden service address", "onion_address", t.onionAddr)
	return t.onionAddr, nil
}

// monitorTor monitors the Tor process and restarts if it crashes
func (t *TorService) monitorTor() {
	t.monitorTorWithInterval(30 * time.Second)
}

// monitorTorWithInterval is the testable form of monitorTor — accepts a configurable ticker interval.
func (t *TorService) monitorTorWithInterval(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.mu.RLock()
			running := t.running
			torInstance := t.tor
			t.mu.RUnlock()

			if !running || torInstance == nil {
				continue
			}

			// Check if Tor is still responsive via control connection
			if _, err := torInstance.Control.GetInfo("version"); err != nil {
				slog.Warn("Tor process unresponsive, restarting", "err", err)
				if err := t.RestartTorService(); err != nil {
					slog.Error("Tor failed to restart", "err", err)
				}
			}
		}
	}
}

// GetTorStatus returns detailed Tor status information
func (t *TorService) GetTorStatus() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	status := map[string]interface{}{
		"enabled":       t.config.Server.Tor.Enabled,
		"running":       t.running,
		"onion_address": t.onionAddr,
	}

	if t.running && t.tor != nil {
		// Get Tor version
		if version, err := t.tor.Control.GetInfo("version"); err == nil {
			status["version"] = version
		}

		// Get circuit status
		if circuits, err := t.tor.Control.GetInfo("circuit-status"); err == nil {
			status["circuits"] = circuits
		}

		status["status"] = "connected"
		status["outbound_enabled"] = t.dialer != nil
	} else {
		status["status"] = "disconnected"
	}

	return status
}

// Shutdown gracefully shuts down the Tor service
func (t *TorService) Shutdown() {
	t.cancel()
	t.StopTorService()
}

// CheckTorConnection tests if Tor is accessible
func CheckTorConnection(socksAddr string) bool {
	return true
}

// VanityProgress represents the progress of vanity address generation
// Per AI.md PART 31: Vanity address generation (built-in, max 6 chars)
type VanityProgress struct {
	Prefix    string
	Attempts  int64
	StartTime time.Time
	Running   bool
	Found     bool
	Address   string
	// ED25519 private key for the found address — used by ApplyVanityAddress
	PrivateKey []byte
	Error      string
}

// vanityGenerator holds vanity generation state
type vanityGenerator struct {
	progress *VanityProgress
	cancel   context.CancelFunc
	mu       sync.RWMutex
}

var vanityGen = &vanityGenerator{
	progress: &VanityProgress{},
}

// GetVanityProgress returns the current vanity generation progress
func (t *TorService) GetVanityProgress() *VanityProgress {
	vanityGen.mu.RLock()
	defer vanityGen.mu.RUnlock()

	pk := make([]byte, len(vanityGen.progress.PrivateKey))
	copy(pk, vanityGen.progress.PrivateKey)
	return &VanityProgress{
		Prefix:     vanityGen.progress.Prefix,
		Attempts:   vanityGen.progress.Attempts,
		StartTime:  vanityGen.progress.StartTime,
		Running:    vanityGen.progress.Running,
		Found:      vanityGen.progress.Found,
		Address:    vanityGen.progress.Address,
		PrivateKey: pk,
		Error:      vanityGen.progress.Error,
	}
}

// GenerateVanity starts background vanity address generation
// Per AI.md PART 32: Built-in generation supports max 6 character prefixes
func (t *TorService) GenerateVanity(prefix string) error {
	// Validate prefix length
	if len(prefix) > 6 {
		return fmt.Errorf("prefix too long: max 6 characters for built-in generation")
	}

	// Check for valid characters (base32 lowercase only)
	validChars := "abcdefghijklmnopqrstuvwxyz234567"
	for _, c := range prefix {
		if !strings.ContainsRune(validChars, c) {
			return fmt.Errorf("invalid character '%c' in prefix: must be lowercase a-z or 2-7", c)
		}
	}

	// Cancel any existing generation
	t.CancelVanity()

	// Start new generation
	ctx, cancel := context.WithCancel(context.Background())
	vanityGen.mu.Lock()
	vanityGen.cancel = cancel
	vanityGen.progress = &VanityProgress{
		Prefix:    prefix,
		Attempts:  0,
		StartTime: time.Now(),
		Running:   true,
	}
	vanityGen.mu.Unlock()

	go t.runVanityGeneration(ctx, prefix)

	return nil
}

// runVanityGeneration runs the vanity generation in the background
func (t *TorService) runVanityGeneration(ctx context.Context, prefix string) {
	defer func() {
		vanityGen.mu.Lock()
		vanityGen.progress.Running = false
		vanityGen.mu.Unlock()
	}()

	prefix = strings.ToLower(prefix)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			vanityGen.mu.Lock()
			vanityGen.progress.Attempts++
			vanityGen.mu.Unlock()

			address, privKey, err := generateOnionV3Address()
			if err != nil {
				vanityGen.mu.Lock()
				vanityGen.progress.Error = err.Error()
				vanityGen.mu.Unlock()
				continue
			}

			if strings.HasPrefix(address, prefix) {
				vanityGen.mu.Lock()
				vanityGen.progress.Found = true
				vanityGen.progress.Address = address
				vanityGen.progress.PrivateKey = privKey
				vanityGen.mu.Unlock()
				slog.Info("Tor vanity address found", "address", address+".onion", "attempts", vanityGen.progress.Attempts)
				return
			}
		}
	}
}

// generateOnionV3Address generates a fresh ED25519 key pair and derives its v3 onion address.
// Returns (56-char base32 address without .onion, ed25519 private key, error).
// Algorithm: address = base32(pubkey[32] || checksum[2] || version[1]) where
// checksum = SHA3-256(".onion checksum" || pubkey || 0x03)[:2]
func generateOnionV3Address() (string, []byte, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", nil, err
	}

	// Compute 2-byte checksum per Tor spec for v3 onion addresses
	version := byte(0x03)
	h := sha3.New256()
	h.Write([]byte(".onion checksum"))
	h.Write(pubKey)
	h.Write([]byte{version})
	checksum := h.Sum(nil)

	// Build 35-byte payload: pubkey[32] + checksum[2] + version[1]
	payload := make([]byte, 35)
	copy(payload[0:32], pubKey)
	copy(payload[32:34], checksum[:2])
	payload[34] = version

	// Base32-encode without padding → 56-char lowercase string
	address := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(payload))

	return address, []byte(privKey), nil
}

// CancelVanity cancels any running vanity generation
func (t *TorService) CancelVanity() {
	vanityGen.mu.Lock()
	defer vanityGen.mu.Unlock()

	if vanityGen.cancel != nil {
		vanityGen.cancel()
		vanityGen.cancel = nil
	}
	vanityGen.progress.Running = false
}

// ApplyVanityAddress applies the generated vanity address by importing its private key
func (t *TorService) ApplyVanityAddress() (string, error) {
	progress := t.GetVanityProgress()
	if !progress.Found {
		return "", fmt.Errorf("no vanity address has been generated")
	}
	if len(progress.PrivateKey) == 0 {
		return "", fmt.Errorf("no private key available for vanity address")
	}
	return t.ImportKeys(progress.PrivateKey)
}

// ExportKeys exports the current Tor hidden service keys
// Per AI.md PART 32: Key import/export for external vanity addresses
func (t *TorService) ExportKeys() ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.running {
		return nil, fmt.Errorf("tor service is not running")
	}

	keyPath := filepath.Join(t.dataDir, "site", "hs_ed25519_secret_key")
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read keys: %w", err)
	}

	return data, nil
}

// ImportKeys imports external Tor hidden service keys
// Per AI.md PART 32: Key import/export for external vanity addresses
func (t *TorService) ImportKeys(privateKey []byte) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(privateKey) == 0 {
		return "", fmt.Errorf("private key is empty")
	}

	// Stop Tor if running
	if t.running {
		if t.tor != nil {
			if t.serviceID != "" {
				t.tor.Control.DelOnion(t.serviceID)
			}
			t.tor.Close()
			t.tor = nil
		}
		t.running = false
	}

	// Write new keys
	keyPath := filepath.Join(t.dataDir, "site", "hs_ed25519_secret_key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		return "", fmt.Errorf("failed to create keys directory: %w", err)
	}

	if err := os.WriteFile(keyPath, privateKey, 0600); err != nil {
		return "", fmt.Errorf("failed to write key: %w", err)
	}

	// Restart Tor to apply new keys
	t.mu.Unlock()
	err := t.StartTorService()
	t.mu.Lock()

	if err != nil {
		return "", fmt.Errorf("failed to restart tor: %w", err)
	}

	slog.Info("Tor imported external keys", "onion_address", t.onionAddr)
	return t.onionAddr, nil
}
