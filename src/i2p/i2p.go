package i2p

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/apimgr/search/src/config"
)

// I2PService manages the I2P eepsite. The server binary owns the provider
// lifecycle, per AI.md PART 31.2 "Implementation".
type I2PService struct {
	provider I2PProvider
	// Full .b32.i2p address, derived from the persisted destination
	eepsiteAddress string
	// Dedicated plain loopback port the eepsite forwards to
	i2pBackendPort int
	// Model A: the managed i2pd process (nil for Model B)
	i2pd *exec.Cmd
	// Model B: the live SAM control connection (nil for Model A)
	samConn net.Conn
	// Closed when the Model-A i2pd child process exits, however it exits
	i2pdExited chan struct{}
	// Plain loopback listener that relays eepsite traffic to the clearnet port
	backend *i2pBackend
}

// resolveI2PDBinary locates the i2pd executable: an explicit cfg.Binary override
// wins, then common install locations, then $PATH. Returns an error when no i2pd
// binary is available - in which case the caller falls back to SAM (Model B).
func resolveI2PDBinary(cfg *I2PConfig) (string, error) {
	if cfg.Binary != "" {
		if _, err := os.Stat(cfg.Binary); err == nil {
			return cfg.Binary, nil
		}
		return "", fmt.Errorf("configured i2pd binary not found: %s", cfg.Binary)
	}
	for _, p := range []string{
		"/usr/bin/i2pd",
		"/usr/sbin/i2pd",
		"/usr/local/bin/i2pd",
		"/opt/homebrew/bin/i2pd",
	} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if p, err := exec.LookPath("i2pd"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("i2pd binary not found")
}

// samReachable reports whether a SAMv3 bridge is accepting connections at addr.
func samReachable(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	if err := conn.Close(); err != nil {
		slog.Debug("failed to close I2P connectivity probe", "error", err)
	}
	return true
}

// ensureI2PDirs creates all I2P directories with correct permissions BEFORE any
// file is written (mirrors ensureTorDirs), per AI.md PART 31.2 "Runtime
// Directory Handling".
func ensureI2PDirs() error {
	configDir := config.GetConfigDir()
	dataDir := config.GetDataDir()
	dirs := []string{
		filepath.Join(configDir, "i2p"),
		filepath.Join(dataDir, "i2p"),
		filepath.Join(dataDir, "i2p", "site"),
	}
	uid := os.Getuid()
	gid := os.Getgid()

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create i2p dir %s: %w", dir, err)
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return fmt.Errorf("chmod i2p dir %s: %w", dir, err)
		}
		// Enforce ownership (skip on Windows — no chown)
		if runtime.GOOS != "windows" {
			if err := os.Chown(dir, uid, gid); err != nil {
				return fmt.Errorf("chown i2p dir %s: %w", dir, err)
			}
		}
	}
	return nil
}

// getI2PTunnelsConf generates the i2pd server-tunnel definition. The eepsite is
// declared here via a [site] server tunnel pointing at the dedicated backend
// port; i2pd persists the destination in keysPath and derives the .b32.i2p.
func getI2PTunnelsConf(cfg *I2PConfig, keysPath string, i2pBackendPort int) string {
	return fmt.Sprintf(`[site]
type = server
host = 127.0.0.1
port = %d
keys = %s
inbound.length = %d
outbound.length = %d
inbound.quantity = %d
outbound.quantity = %d
signaturetype = %d
`, i2pBackendPort, keysPath,
		cfg.InboundLength, cfg.OutboundLength,
		cfg.InboundQuantity, cfg.OutboundQuantity, cfg.SignatureType)
}

// updateI2PTunnels writes tunnels.conf with 0600 permissions, creating the
// parent directory first. tunnels.conf is derived state, safe to overwrite: the
// destination identity lives in the keys file, never here.
func updateI2PTunnels(tunnelsPath string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(tunnelsPath), 0700); err != nil {
		return fmt.Errorf("create tunnels dir: %w", err)
	}
	if err := os.WriteFile(tunnelsPath, content, 0600); err != nil {
		return err
	}
	return os.Chmod(tunnelsPath, 0600)
}

// waitForI2PDAddress polls the destination key file i2pd persists and derives
// the .b32.i2p address from it once the blob is complete.
func waitForI2PDAddress(ctx context.Context, keysPath string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if data, err := os.ReadFile(keysPath); err == nil {
			if dest, err := destinationFromKeys(data); err == nil {
				return b32Address(dest), nil
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return "", fmt.Errorf("timed out after %s waiting for i2pd destination at %s", timeout, keysPath)
			}
		}
	}
}

// startI2PD writes tunnels.conf (regenerated each run) and starts a dedicated
// i2pd child process, then reads the .b32.i2p from the persisted destination
// once it is ready. i2pd creates/persists site-keys.dat at keysPath.
func startI2PD(ctx context.Context, cfg *I2PConfig, binary, keysPath string, i2pBackendPort int, svc *I2PService) (string, error) {
	configDir := config.GetConfigDir()
	dataDir := config.GetDataDir()
	tunnelsPath := filepath.Join(configDir, "i2p", "tunnels.conf")

	conf := getI2PTunnelsConf(cfg, keysPath, i2pBackendPort)
	// Regenerate every startup (derived state); identity lives in keysPath.
	if err := updateI2PTunnels(tunnelsPath, []byte(conf)); err != nil {
		return "", fmt.Errorf("failed to write tunnels.conf: %w", err)
	}
	slog.Info("I2P regenerated tunnels.conf", "path", tunnelsPath, "backend_port", i2pBackendPort)

	cmd := exec.CommandContext(ctx, binary,
		"--datadir", filepath.Join(dataDir, "i2p"),
		"--tunconf", tunnelsPath,
		"--pidfile", filepath.Join(dataDir, "i2p", "i2pd.pid"),
		"--log", "file",
		"--logfile", filepath.Join(config.GetLogDir(), "i2pd.log"),
		"--loglevel", "warn",
	)
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start i2pd: %w", err)
	}
	svc.i2pd = cmd
	// Reap the child in one place: the monitor reads this channel for liveness
	// and Close waits on it, so i2pd never lingers as a zombie.
	exited := make(chan struct{})
	svc.i2pdExited = exited
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Debug("i2pd process exited with an error", "error", err)
		}
		close(exited)
	}()

	// Wait for i2pd to publish the destination, then derive the address.
	deadline := time.Duration(cfg.BootstrapTimeout) * time.Second
	addr, err := waitForI2PDAddress(ctx, keysPath, deadline)
	if err != nil {
		if cmd.Process != nil {
			if err := cmd.Process.Signal(os.Interrupt); err != nil {
				slog.Warn("failed to interrupt i2pd after startup failure", "error", err)
			}
			<-exited
		}
		svc.i2pd = nil
		svc.i2pdExited = nil
		return "", err
	}
	return addr, nil
}

// startDedicatedI2P creates the eepsite when I2P is enabled AND a provider is
// available. It resolves the provider FIRST (i2pd binary, else a reachable SAM
// bridge); if neither is available it returns an error and NO backend port is
// allocated (I2P stays disabled). Only after a provider is confirmed does it
// allocate a DEDICATED plain loopback listener the eepsite forwards to - using
// the same random-unused port detection the server uses for its own port.
// The eepsite maps: .b32.i2p:{virtual_port} → 127.0.0.1:{backend port}.
// clearnetAddr is the loopback address of the app's clearnet HTTP listener that
// relayed eepsite connections are handed to.
func startDedicatedI2P(ctx context.Context, cfg *I2PConfig, clearnetAddr string) (*I2PService, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("i2p disabled (opt-in) - eepsite not started")
	}
	cfg.Normalize()

	// Resolve provider FIRST. No provider → return before allocating a port or
	// writing any files (mirrors the Tor smart-port gate).
	provider := I2PProviderNone
	i2pdBinary := ""
	if b, err := resolveI2PDBinary(cfg); err == nil {
		provider, i2pdBinary = I2PProviderI2PD, b
	} else if samReachable(cfg.SAMAddress) {
		provider = I2PProviderSAM
	} else {
		return nil, fmt.Errorf("i2p enabled but no provider available (no i2pd binary, SAM %s unreachable)", cfg.SAMAddress)
	}

	if err := ensureI2PDirs(); err != nil {
		return nil, fmt.Errorf("failed to create i2p directories: %w", err)
	}

	// Allocate the dedicated plain loopback port only now that a provider is
	// confirmed, using the same random-unused-port detection as the server's own
	// port (64000-64999). Not persisted: a fresh port each run.
	backend, err := newI2PBackend(config.GetRandomPort(), clearnetAddr)
	if err != nil {
		return nil, err
	}
	i2pBackendPort := backend.Port()

	dataDir := config.GetDataDir()
	// The destination key persists here; the .b32.i2p address derives from it.
	keysPath := filepath.Join(dataDir, "i2p", "site", "site-keys.dat")

	svc := &I2PService{provider: provider, i2pBackendPort: i2pBackendPort, backend: backend}

	switch provider {
	case I2PProviderI2PD:
		// Model A: regenerate tunnels.conf every startup (derived state), then
		// start a dedicated i2pd. i2pd persists the destination in keysPath.
		addr, err := startI2PD(ctx, cfg, i2pdBinary, keysPath, i2pBackendPort, svc)
		if err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		svc.eepsiteAddress = addr
	case I2PProviderSAM:
		// Model B: create a STREAM session from the persisted destination and
		// STREAM FORWARD incoming connections to the backend port.
		addr, err := startSAMEepsite(cfg, keysPath, i2pBackendPort, svc)
		if err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		svc.eepsiteAddress = addr
	}

	slog.Info("I2P eepsite started",
		"provider", providerName(provider),
		"address", svc.eepsiteAddress,
		"virtual_port", cfg.VirtualPort,
		"backend_port", i2pBackendPort)
	return svc, nil
}

// EepsiteAddress returns the full .b32.i2p address.
func (s *I2PService) EepsiteAddress() string { return s.eepsiteAddress }

// Provider returns the backend that created the eepsite.
func (s *I2PService) Provider() I2PProvider { return s.provider }

// ProviderName returns the provider as the string reported by healthz and
// --status ("i2pd", "sam", or "none").
func (s *I2PService) ProviderName() string { return providerName(s.provider) }

// BackendPort returns the dedicated plain loopback port the eepsite forwards to.
func (s *I2PService) BackendPort() int { return s.i2pBackendPort }

// IsEepsiteConn reports whether a request whose RemoteAddr is remoteAddr
// arrived over the eepsite instead of the clearnet listener.
func (s *I2PService) IsEepsiteConn(remoteAddr string) bool {
	if s.backend == nil {
		return false
	}
	return s.backend.IsEepsiteConn(remoteAddr)
}

// IsRunning reports whether the provider is still alive.
func (s *I2PService) IsRunning() bool { return i2pProviderHealthy(s) }

// Close shuts down the provider (i2pd process or SAM session).
func (s *I2PService) Close() error {
	var closeErr error
	if s.backend != nil {
		closeErr = errors.Join(closeErr, s.backend.Close())
		s.backend = nil
	}
	if s.samConn != nil {
		closeErr = errors.Join(closeErr, s.samConn.Close())
		s.samConn = nil
	}
	if s.i2pd != nil && s.i2pd.Process != nil {
		proc := s.i2pd.Process
		exited := s.i2pdExited
		s.i2pd = nil
		s.i2pdExited = nil
		if err := proc.Signal(os.Interrupt); err != nil {
			return errors.Join(closeErr, err)
		}
		// The reaper goroutine started in startI2PD closes this once i2pd exits.
		if exited != nil {
			<-exited
		}
		return nil
	}
	s.i2pd = nil
	s.i2pdExited = nil
	return closeErr
}

// i2pProviderHealthy reports whether the selected provider is still usable:
// Model A checks the i2pd child process, Model B checks the SAM control
// connection the router keeps open for the session's lifetime.
func i2pProviderHealthy(s *I2PService) bool {
	if s == nil {
		return false
	}
	switch s.provider {
	case I2PProviderI2PD:
		if s.i2pd == nil || s.i2pdExited == nil {
			return false
		}
		// The reaper goroutine closes i2pdExited the moment the child exits, so
		// this is a portable liveness check with no signal probing.
		select {
		case <-s.i2pdExited:
			return false
		default:
			return true
		}
	case I2PProviderSAM:
		if s.samConn == nil {
			return false
		}
		// A closed SAM control connection ends the session, so a zero-length
		// write is enough to detect that the router hung up.
		if err := s.samConn.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return false
		}
		_, err := s.samConn.Write(nil)
		if err := s.samConn.SetWriteDeadline(time.Time{}); err != nil {
			slog.Debug("failed to clear SAM write deadline", "error", err)
		}
		return err == nil
	default:
		return false
	}
}
