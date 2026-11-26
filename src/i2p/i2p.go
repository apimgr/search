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
	"strconv"
	"strings"
	"sync"

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
	// Live SAM control connection for both managed and external routers.
	samConn net.Conn
	// The separate STREAM FORWARD socket stays open for the session lifetime.
	forwardConn net.Conn
	// Closed when either long-lived SAM socket ends unexpectedly.
	samSessionLost chan struct{}
	samLossOnce   sync.Once
	samAddress    string
	samProbeFailures int
	samProbeFailed bool
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

// samReachable reports whether a SAMv3 bridge completes the required HELLO at addr.
func samReachable(addr string) bool {
	err := probeSAMBridge(addr)
	// An old but responsive SAM bridge is still the configured provider; the
	// session startup path reports the actionable unsupported-version error.
	return err == nil || errors.Is(err, errSAMTooOld)
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

// getI2PDConf renders a dedicated router configuration with only its private
// SAM bridge enabled. The application owns the SAM session and forwarding.
func getI2PDConf(samPort int, logPath string) string {
	return fmt.Sprintf(`daemon = false
ipv4 = true
ipv6 = false
notransit = true
floodfill = false
log = file
logfile = %s
loglevel = warn

[http]
enabled = false
[httpproxy]
enabled = false
[socksproxy]
enabled = false
[sam]
enabled = true
address = 127.0.0.1
port = %d
[bob]
enabled = false
[i2cp]
enabled = false
[i2pcontrol]
enabled = false
[upnp]
enabled = false
`, logPath, samPort)
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

func unusedLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portText)
}

func reapStaleI2PD(pidPath, binary string) error {
	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read stale i2pd pid file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove invalid i2pd pid file: %w", err)
		}
		return nil
	}
	matches, err := processMatchesI2PD(pid, binary)
	if err != nil {
		return fmt.Errorf("verify stale i2pd process %d: %w", pid, err)
	}
	if !matches {
		if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale i2pd pid file: %w", err)
		}
		return nil
	}
	if err := terminateStaleI2PD(pid, binary); err != nil {
		return fmt.Errorf("terminate stale i2pd process %d: %w", pid, err)
	}
	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale i2pd pid file: %w", err)
	}
	return nil
}

// startI2PDWithBuilding writes the private router config and empty tunnel
// configuration, starts a dedicated i2pd child, waits for its SAM bridge, then
// creates the app-owned SAM session and forwarding socket.
func startI2PDWithBuilding(ctx context.Context, cfg *I2PConfig, binary, keysPath string, i2pBackendPort int, svc *I2PService, onBuilding func()) (string, error) {
	configDir := config.GetConfigDir()
	dataDir := config.GetDataDir()
	confPath := filepath.Join(configDir, "i2p", "i2pd.conf")
	logPath := filepath.Join(config.GetLogDir(), "i2pd.log")
	tunnelsPath := filepath.Join(configDir, "i2p", "tunnels.conf")
	tunnelsDir := filepath.Join(configDir, "i2p", "tunnels.d")
	pidPath := filepath.Join(dataDir, "i2p", "i2pd.pid")
	if err := reapStaleI2PD(pidPath, binary); err != nil {
		return "", err
	}
	samPort, err := unusedLoopbackPort()
	if err != nil {
		return "", fmt.Errorf("allocate private I2P SAM port: %w", err)
	}
	if err := updateI2PTunnels(confPath, []byte(getI2PDConf(samPort, logPath))); err != nil {
		return "", fmt.Errorf("failed to write i2pd.conf: %w", err)
	}
	if err := updateI2PTunnels(tunnelsPath, nil); err != nil {
		return "", fmt.Errorf("failed to write empty tunnels.conf: %w", err)
	}
	if err := os.RemoveAll(tunnelsDir); err != nil {
		return "", fmt.Errorf("clear tunnels.d: %w", err)
	}
	if err := os.MkdirAll(tunnelsDir, 0700); err != nil {
		return "", fmt.Errorf("create empty tunnels.d: %w", err)
	}
	if err := os.Chmod(tunnelsDir, 0700); err != nil {
		return "", fmt.Errorf("secure tunnels.d: %w", err)
	}
	slog.Info("I2P regenerated private router config", "path", confPath, "sam_port", samPort)

	cmd := exec.Command(binary,
		"--conf", confPath,
		"--tunconf", tunnelsPath,
		"--tunnelsdir", tunnelsDir,
		"--datadir", filepath.Join(dataDir, "i2p"),
		"--pidfile", pidPath,
	)
	setParentDeathSignal(cmd)
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

	samAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(samPort))
	if err := waitForSAMBridge(ctx, samAddress, exited); err != nil {
		err = fmt.Errorf("%w (see %s)", err, logPath)
		if stopErr := stopI2PD(cmd, exited); stopErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to stop i2pd after SAM startup failure: %w", stopErr))
		}
		svc.i2pd = nil
		svc.i2pdExited = nil
		return "", err
	}
	localConfig := *cfg
	localConfig.SAMAddress = samAddress
	addr, err := startSAMEepsiteWithContext(ctx, &localConfig, keysPath, i2pBackendPort, svc, onBuilding)
	if err != nil {
		stopErr := stopI2PD(cmd, exited)
		svc.i2pd = nil
		svc.i2pdExited = nil
		return "", errors.Join(err, stopErr)
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
	return startDedicatedI2PWithLifecycle(ctx, cfg, clearnetAddr, nil)
}

func startDedicatedI2PWithLifecycle(ctx context.Context, cfg *I2PConfig, clearnetAddr string, onBuilding func()) (*I2PService, error) {
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
		// Model A: regenerate the private router configuration, then create the
		// app-owned SAM destination, session, and forward stream.
		addr, err := startI2PDWithBuilding(ctx, cfg, i2pdBinary, keysPath, i2pBackendPort, svc, onBuilding)
		if err != nil {
			return nil, errors.Join(err, backend.Close())
		}
		svc.eepsiteAddress = addr
	case I2PProviderSAM:
		// Model B: create a STREAM session through the external SAM bridge and
		// forward incoming connections to the backend port.
		addr, err := startSAMEepsiteWithContext(ctx, cfg, keysPath, i2pBackendPort, svc, onBuilding)
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
	if s.forwardConn != nil {
		closeErr = errors.Join(closeErr, s.forwardConn.Close())
		s.forwardConn = nil
	}
	if s.samConn != nil {
		closeErr = errors.Join(closeErr, s.samConn.Close())
		s.samConn = nil
	}
	if s.i2pd != nil && s.i2pd.Process != nil {
		cmd := s.i2pd
		exited := s.i2pdExited
		s.i2pd = nil
		s.i2pdExited = nil
		return errors.Join(closeErr, stopI2PD(cmd, exited))
	}
	s.i2pd = nil
	s.i2pdExited = nil
	return closeErr
}

// i2pProviderHealthy reports whether the selected provider process or SAM
// session sockets remain usable.
func i2pProviderHealthy(s *I2PService) bool {
	if s == nil {
		return false
	}
	if s.samSessionLost != nil {
		select {
		case <-s.samSessionLost:
			return false
		default:
		}
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
		return s.forwardConn != nil && !s.samProbeFailed
	default:
		return false
	}
}
