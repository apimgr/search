package service

import (
	"context"
	"errors"
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
	"github.com/cretz/bine/tor"
)

// findTorBinary finds the Tor binary using config, PATH, or common locations
// Per AI.md PART 31: Tor binary discovery priority:
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

	// 3) Common locations per AI.md PART 31
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
// per AI.md PART 31: TOR HIDDEN SERVICE (NON-NEGOTIABLE)
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
	// Dedicated PROXY-protocol loopback listener the hidden service targets
	backend *torBackend
	// Runtime-allocated port the backend listener binds; never the clearnet port
	backendPort int
	// Last startup failure, surfaced as status "error:{message}"
	lastError string
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
// Per AI.md PART 31: Server binary owns all Tor files
func (t *TorService) ensureTorDirs() error {
	dirs := []string{
		// {data_dir}/tor/
		t.dataDir,
		// {data_dir}/tor/site/ for hidden service keys
		filepath.Join(t.dataDir, "site"),
		// {data_dir}/tor/vanity/ for vanity candidate key directories
		filepath.Join(t.dataDir, "vanity"),
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

// getTorConfig generates torrc content per AI.md PART 31.
// The hidden service is DECLARED IN torrc (HiddenServiceDir + HiddenServicePort),
// never created with ADD_ONION, so the keys stay on disk in Tor's own format and
// HiddenServiceExportCircuitID can be enabled. torrc is derived state: it is
// regenerated on every startup from TorConfig plus the current backend port.
//
// PORT DETECTION: All ports use runtime detection via "auto" - never saved/hardcoded
// - SocksPort: Controlled via torrc (auto or 0), NoAutoSocksPort=true in StartConf
// - ControlPort: TCP 127.0.0.1:auto on all OSes (bine reads the port after start)
//
// NEVER uses default Tor ports (9050, 9051) - runtime auto ports only
func (t *TorService) getTorConfig() string {
	cfg := &t.config.Server.Tor

	// All OSes use localhost TCP auto port per AI.md PART 31:
	// "Use ControlPort 127.0.0.1:auto on all OSes for the current bine-based integration"
	// "Never hardcode a control port — let Tor choose a free localhost port at runtime"
	// NEVER use default port 9051 — runtime detection only
	controlConfig := "ControlPort 127.0.0.1:auto"

	// SocksPort per AI.md PART 31:
	// - "SocksPort auto" if outbound enabled (UseNetwork or AllowUserPreference)
	// - "SocksPort 0" if outbound disabled (hidden service only)
	// NOTE: We use NoAutoSocksPort=true in StartConf so bine doesn't add conflicting args
	var socksConfig string
	if cfg.UseNetwork || cfg.AllowUserPreference {
		// Enable SOCKS for outbound — bound to loopback, runtime port, never 9050
		socksConfig = "SocksPort 127.0.0.1:auto\nSocksPolicy accept 127.0.0.1\nSocksPolicy reject *"
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

	// Hidden service declaration per AI.md PART 31. The service targets the
	// dedicated PROXY-protocol backend listener — NEVER the clearnet port — and
	// exports the rendezvous circuit ID in HAProxy PROXY v1 format.
	virtualPort := cfg.VirtualPort
	if virtualPort == 0 {
		virtualPort = 80
	}
	hiddenService := fmt.Sprintf(`HiddenServiceDir %s
HiddenServiceVersion 3
HiddenServicePort %d 127.0.0.1:%d
HiddenServiceExportCircuitID haproxy`,
		filepath.Join(t.dataDir, "site"), virtualPort, t.backendPort)

	pidFile := fmt.Sprintf("PidFile %s", filepath.Join(t.dataDir, "tor.pid"))

	return fmt.Sprintf(`# ============================================================
# Tor Configuration - Generated by server binary
# Per AI.md PART 31: TOR HIDDEN SERVICE
# ============================================================

# SOCKS port for outbound connections (0 = disabled, auto = runtime port)
# NEVER uses default port 9050 - runtime detection only
%s

# Control connection per AI.md PART 31:
# - All OSes: TCP on 127.0.0.1:auto (localhost only, never hardcoded)
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
PublishServerDescriptor 0

# Process id file per AI.md PART 31 storage layout
%s

# Hidden service (declared in torrc, never ADD_ONION) per AI.md PART 31
%s

# Guard-discovery-attack defense (vanguards-lite) per AI.md PART 31 - built into
# Tor >= 0.4.7; keep enabled, never disable
VanguardsLiteEnabled 1

# Never trade anonymity for speed — full three-hop circuits always
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
		}(), bandwidthRate, bandwidthBurst, accountingConfig, pidFile, hiddenService)
}

// readOnionHostname waits for Tor to publish {data_dir}/tor/site/hostname and
// returns the address without the .onion suffix. Tor creates the file during
// startup, so a short bounded poll covers the gap between process start and the
// key material landing on disk.
func (t *TorService) readOnionHostname() (string, error) {
	hostPath := filepath.Join(t.dataDir, "site", "hostname")
	deadline := time.Now().Add(30 * time.Second)

	for {
		data, err := os.ReadFile(hostPath)
		if err == nil {
			host := strings.TrimSpace(string(data))
			if host != "" {
				return strings.TrimSuffix(host, ".onion"), nil
			}
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("hostname file %s was not published within 30s", hostPath)
		}
		select {
		case <-t.ctx.Done():
			return "", t.ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// StartTorService starts the Tor hidden service using bine.
// Per AI.md PART 31: "Auto-enabled if tor binary is installed - no enable flag needed".
// A failure is recorded so status can report error:{message} per PART 31.
func (t *TorService) StartTorService() error {
	err := t.startTorProcess()
	t.mu.Lock()
	if err != nil {
		t.lastError = err.Error()
	}
	t.mu.Unlock()
	return err
}

// startTorProcess performs the actual startup sequence.
func (t *TorService) startTorProcess() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	torConfig := &t.config.Server.Tor

	// Per AI.md PART 31: Find Tor binary first - auto-enable if found
	torBinary := findTorBinary(torConfig.Binary)
	if torBinary == "" {
		// Per AI.md PART 31: "NOT FOUND: Log INFO, disable Tor features, continue without Tor"
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

	// Per AI.md PART 31: the hidden service targets a DEDICATED backend listener
	// on a runtime-allocated loopback port that speaks PROXY protocol, never the
	// clearnet port. The port is allocated only once the Tor binary is found.
	if t.backend == nil {
		port := config.GetRandomPort()
		backend, err := newTorBackend(port, fmt.Sprintf("127.0.0.1:%d", t.config.Server.Port))
		if err != nil {
			return err
		}
		t.backend = backend
		t.backendPort = port
	}

	torrcPath := filepath.Join(t.configDir, "torrc")

	// Per AI.md PART 31: regenerate torrc every startup — it is derived state
	// from TorConfig plus the current backend port, never hand-edited state.
	if err := os.WriteFile(torrcPath, []byte(t.getTorConfig()), 0600); err != nil {
		return fmt.Errorf("failed to write torrc: %w", err)
	}
	slog.Info("Tor wrote torrc", "path", torrcPath)

	// Build StartConf per AI.md PART 31
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

	// Per AI.md PART 31 bootstrap logging: start silently; show "Tor: connecting..."
	// only if bootstrap takes >30s; show address on success.
	slog.Info("Tor starting process")

	// Start OUR OWN Tor process - completely separate from system Tor
	torInstance, err := tor.Start(t.ctx, conf)
	if err != nil {
		return fmt.Errorf("failed to start tor: %w", err)
	}

	// Wait for Tor to bootstrap with progressive logging per AI.md PART 31
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
		return fmt.Errorf("failed to connect to tor network (timeout %v): %w", bootstrapTimeout, errors.Join(err, torInstance.Close()))
	}

	// The hidden service is declared in torrc, so Tor itself writes the keys and
	// the hostname file into {data_dir}/tor/site/. Read the published address
	// back from disk rather than from an ADD_ONION response.
	serviceID, err := t.readOnionHostname()
	if err != nil {
		return fmt.Errorf("failed to read hidden service hostname: %w", errors.Join(err, torInstance.Close()))
	}

	t.tor = torInstance
	t.serviceID = serviceID
	t.onionAddr = serviceID + ".onion"
	t.running = true
	t.lastError = ""

	// Initialize outbound dialer if enabled
	if torConfig.UseNetwork || torConfig.AllowUserPreference {
		dialer, err := torInstance.Dialer(t.ctx, nil)
		if err != nil {
			slog.Warn("Tor outbound dialer failed", "err", err)
		} else {
			t.dialer = dialer
		}
	}

	// Per AI.md PART 31 success log: "Tor: {onion_address}" (short, once at startup)
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

	if t.backend != nil {
		if err := t.backend.Close(); err != nil {
			slog.Warn("Tor error closing backend listener", "err", err)
		}
		t.backend = nil
		t.backendPort = 0
	}

	t.dialer = nil
	t.running = false
	t.onionAddr = ""
	t.serviceID = ""
	slog.Info("Tor service stopped")
	return nil
}

// CircuitID returns the tor:{circuit_id} token for a request whose RemoteAddr is
// remoteAddr, or "" when the request did not arrive over the hidden service.
// Per AI.md PART 31 the token is opaque — it is never an IP and never
// deanonymizing — and is what logs, audit trails, and rate limiting key on.
func (t *TorService) CircuitID(remoteAddr string) string {
	t.mu.RLock()
	backend := t.backend
	t.mu.RUnlock()

	if backend == nil {
		return ""
	}
	return backend.CircuitID(remoteAddr)
}

// BackendPort returns the loopback port the dedicated PROXY-protocol listener is
// bound to, or 0 when Tor is not running.
func (t *TorService) BackendPort() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.backendPort
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
// Per AI.md PART 31: useTor: true = route through Tor, false = direct
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

// RegenerateAddress discards the current hidden service identity and lets Tor
// mint a new one. Per AI.md PART 31 this is destructive and irreversible: the
// old .onion address can never be recovered once its keys are deleted.
func (t *TorService) RegenerateAddress() (string, error) {
	t.mu.RLock()
	running := t.running
	t.mu.RUnlock()

	if !running {
		return "", fmt.Errorf("tor service is not running")
	}

	if err := t.StopTorService(); err != nil {
		return "", fmt.Errorf("stop tor: %w", err)
	}

	// Removing the whole site directory forces Tor to generate a fresh keypair
	// and hostname on the next start.
	if err := os.RemoveAll(filepath.Join(t.dataDir, "site")); err != nil {
		return "", fmt.Errorf("remove hidden service keys: %w", err)
	}

	if err := t.StartTorService(); err != nil {
		return "", fmt.Errorf("start tor with new keys: %w", err)
	}

	addr := t.GetOnionAddress()
	slog.Info("Tor new hidden service address", "onion_address", addr)
	return addr, nil
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
		// Per AI.md PART 31 healthz exposes the address as features.tor.hostname
		"hostname":     t.onionAddr,
		"backend_port": t.backendPort,
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

		// Per AI.md PART 31: status is "healthy" or "error:{short message}"
		status["status"] = "healthy"
		status["outbound_enabled"] = t.dialer != nil
	} else if t.lastError != "" {
		status["status"] = "error:" + t.lastError
	} else {
		status["status"] = ""
	}

	return status
}

// Shutdown gracefully shuts down the Tor service
func (t *TorService) Shutdown() {
	t.cancel()
	if err := t.StopTorService(); err != nil {
		slog.Warn("failed to stop Tor service", "error", err)
	}
}

// ExportKeys exports the current Tor hidden service keys
// Per AI.md PART 31: Key import/export for external vanity addresses
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
// Per AI.md PART 31: Key import/export for external vanity addresses
func (t *TorService) ImportKeys(privateKey []byte) (string, error) {
	if len(privateKey) == 0 {
		return "", fmt.Errorf("private key is empty")
	}

	// Normalize whatever the operator supplied (Tor's own secret key file, a raw
	// expanded key, or a seed) into the three on-disk files Tor expects, then
	// reuse the same swap path a vanity candidate takes.
	expanded, pub, address, err := parseTorKeyMaterial(privateKey)
	if err != nil {
		return "", fmt.Errorf("import keys: %w", err)
	}

	staging := filepath.Join(t.dataDir, "vanity", ".import")
	if err := os.RemoveAll(staging); err != nil {
		return "", fmt.Errorf("remove previous Tor key staging directory: %w", err)
	}
	if err := writeTorKeyDir(staging, expanded, pub, address); err != nil {
		return "", err
	}
	defer func() {
		if err := os.RemoveAll(staging); err != nil {
			slog.Warn("failed to remove Tor key staging directory", "error", err)
		}
	}()

	applied, err := t.applyKeyDir(staging, address)
	if err != nil {
		return "", err
	}

	slog.Info("Tor imported external keys", "onion_address", applied)
	return applied, nil
}
