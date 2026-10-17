package i2p

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
)

// I2PManager handles all I2P lifecycle operations. It stores NO backend port -
// the dedicated port is allocated inside startDedicatedI2P (only when I2P is
// enabled AND a provider is available), not by the caller.
// Per AI.md PART 31.2 "Restart Implementation".
type I2PManager struct {
	mu      sync.Mutex
	service *I2PService
	config  *I2PConfig
	dataDir string
	// Loopback address of the clearnet HTTP listener eepsite traffic relays to
	clearnetAddr string
	ctx          context.Context
}

// NewI2PManager creates a new I2P manager with the given configuration.
// clearnetAddr is the app's own loopback listener address (127.0.0.1:{port}),
// which the dedicated plain backend listener relays eepsite traffic to.
func NewI2PManager(ctx context.Context, cfg *I2PConfig, clearnetAddr string) *I2PManager {
	return &I2PManager{
		config:       cfg,
		dataDir:      filepath.Join(config.GetDataDir(), "i2p"),
		clearnetAddr: clearnetAddr,
		ctx:          ctx,
	}
}

// Start initializes the eepsite if I2P is enabled and a provider is available.
func (im *I2PManager) Start() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.startLocked()
}

func (im *I2PManager) startLocked() error {
	service, err := startDedicatedI2P(im.ctx, im.config, im.clearnetAddr)
	if err != nil {
		return err
	}
	im.service = service
	return nil
}

// UpdateConfig applies new settings and restarts I2P. startLocked ->
// startDedicatedI2P regenerates tunnels.conf (with a freshly allocated backend
// port) from the new config, so no separate tunnels.conf write is needed here.
func (im *I2PManager) UpdateConfig(cfg *I2PConfig) error {
	im.mu.Lock()
	defer im.mu.Unlock()
	im.config = cfg
	var closeErr error
	if im.service != nil {
		closeErr = im.service.Close()
		im.service = nil
	}
	// If the new config disables I2P, startDedicatedI2P returns early and the
	// eepsite stays down - opt-in respected.
	if !cfg.Enabled {
		return closeErr
	}
	return errors.Join(closeErr, im.startLocked())
}

// RegenerateAddress creates a new random .b32.i2p destination.
func (im *I2PManager) RegenerateAddress() (string, error) {
	im.mu.Lock()
	defer im.mu.Unlock()
	var closeErr error
	if im.service != nil {
		closeErr = im.service.Close()
		im.service = nil
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.RemoveAll(filepath.Join(im.dataDir, "site")); err != nil {
		return "", fmt.Errorf("failed to remove old i2p keys: %w", err)
	}
	if err := im.startLocked(); err != nil {
		return "", err
	}
	return im.service.EepsiteAddress(), nil
}

// EepsiteAddress returns the current .b32.i2p address (empty if not running).
func (im *I2PManager) EepsiteAddress() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return im.service.EepsiteAddress()
	}
	return ""
}

// ProviderName returns the active provider ("i2pd", "sam", or "none") for the
// healthz `features.i2p.provider` field and the --status output.
func (im *I2PManager) ProviderName() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		return im.service.ProviderName()
	}
	return providerName(I2PProviderNone)
}

// Enabled reports the opt-in setting itself, independent of whether a provider
// was found. It feeds healthz `features.i2p.enabled`.
func (im *I2PManager) Enabled() bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.config != nil && im.config.Enabled
}

// IsRunning reports whether the eepsite is live, feeding healthz
// `features.i2p.running`.
func (im *I2PManager) IsRunning() bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.service != nil && i2pProviderHealthy(im.service)
}

// Status renders the healthz `features.i2p.status` value and the CLI
// "I2P Eepsite:" line, per AI.md PART 31.2 "CLI".
func (im *I2PManager) Status() string {
	im.mu.Lock()
	defer im.mu.Unlock()
	switch {
	case im.config == nil || !im.config.Enabled:
		return "Disabled"
	case im.service == nil:
		return "No Provider"
	case !i2pProviderHealthy(im.service):
		return "Error"
	default:
		return fmt.Sprintf("Running (%s)", im.service.ProviderName())
	}
}

// IsEepsiteConn reports whether a request whose RemoteAddr is remoteAddr
// arrived over the eepsite rather than the clearnet listener, so overlay HTTP
// semantics (PART 12) can treat it like a .onion request.
func (im *I2PManager) IsEepsiteConn(remoteAddr string) bool {
	im.mu.Lock()
	defer im.mu.Unlock()
	return im.service != nil && im.service.IsEepsiteConn(remoteAddr)
}

// Monitor watches the provider and restarts it if it dies, per AI.md PART 31.2
// "I2P Monitoring". It returns immediately when I2P is disabled and blocks
// until the manager's context is cancelled otherwise, so callers run it in a
// goroutine.
func (im *I2PManager) Monitor() {
	im.monitorWithInterval(30 * time.Second)
}

func (im *I2PManager) monitorWithInterval(interval time.Duration) {
	if !im.Enabled() {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-im.ctx.Done():
			return
		case <-ticker.C:
			im.mu.Lock()
			needsRestart := im.service != nil && !i2pProviderHealthy(im.service)
			if needsRestart {
				slog.Warn("I2P provider unresponsive, restarting")
				if err := im.service.Close(); err != nil {
					slog.Warn("failed to stop unhealthy I2P service", "error", err)
				}
				im.service = nil
				if err := im.startLocked(); err != nil {
					slog.Error("Failed to restart I2P", "err", err)
				}
			}
			im.mu.Unlock()
		}
	}
}

// Close shuts down the provider.
func (im *I2PManager) Close() error {
	im.mu.Lock()
	defer im.mu.Unlock()
	if im.service != nil {
		err := im.service.Close()
		im.service = nil
		return err
	}
	return nil
}
