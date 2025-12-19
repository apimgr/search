package service

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
)

// TorService manages Tor hidden service
type TorService struct {
	config     *config.Config
	torProcess *exec.Cmd
	running    bool
	onionAddr  string
	mu         sync.RWMutex
}

// NewTorService creates a new Tor service manager
func NewTorService(cfg *config.Config) *TorService {
	return &TorService{
		config: cfg,
	}
}

// Start starts the Tor hidden service
func (t *TorService) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return nil
	}

	torConfig := t.config.Server.Tor
	if !torConfig.Enabled {
		return fmt.Errorf("tor service is not enabled in configuration")
	}

	// Get Tor directory
	torDir := config.GetTorDir()

	// Ensure directories exist
	hiddenServiceDir := filepath.Join(torDir, "hidden_service")
	if err := os.MkdirAll(hiddenServiceDir, 0700); err != nil {
		return fmt.Errorf("failed to create hidden service directory: %w", err)
	}

	// Generate torrc
	torrcPath := filepath.Join(torDir, "torrc")
	if err := t.generateTorrc(torrcPath, hiddenServiceDir); err != nil {
		return fmt.Errorf("failed to generate torrc: %w", err)
	}

	// Find Tor binary
	torBinary, err := t.findTorBinary()
	if err != nil {
		return err
	}

	// Start Tor process
	t.torProcess = exec.Command(torBinary, "-f", torrcPath)
	t.torProcess.Dir = torDir

	// Set up output
	if t.config.Server.Mode == "development" {
		t.torProcess.Stdout = os.Stdout
		t.torProcess.Stderr = os.Stderr
	}

	if err := t.torProcess.Start(); err != nil {
		return fmt.Errorf("failed to start tor: %w", err)
	}

	t.running = true

	// Wait for onion address
	go t.waitForOnionAddress(hiddenServiceDir)

	log.Println("[Tor] Service started")
	return nil
}

// Stop stops the Tor hidden service
func (t *TorService) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.running {
		return nil
	}

	if t.torProcess != nil {
		if err := t.torProcess.Process.Kill(); err != nil {
			return fmt.Errorf("failed to kill tor process: %w", err)
		}
	}

	t.running = false
	t.onionAddr = ""
	log.Println("[Tor] Service stopped")
	return nil
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

// generateTorrc generates the Tor configuration file
func (t *TorService) generateTorrc(path, hiddenServiceDir string) error {
	torConfig := t.config.Server.Tor

	// Determine ports
	socksPort := torConfig.SocksPort
	if socksPort == 0 {
		socksPort = 9050
	}

	controlPort := torConfig.ControlPort
	if controlPort == 0 {
		controlPort = 9051
	}

	virtualPort := torConfig.HiddenServicePort
	if virtualPort == 0 {
		virtualPort = 80
	}

	targetPort := t.config.Server.Port

	// Build torrc content
	var lines []string
	lines = append(lines, fmt.Sprintf("SocksPort %d", socksPort))
	lines = append(lines, fmt.Sprintf("ControlPort %d", controlPort))
	lines = append(lines, fmt.Sprintf("DataDirectory %s", filepath.Dir(hiddenServiceDir)))
	lines = append(lines, fmt.Sprintf("HiddenServiceDir %s", hiddenServiceDir))
	lines = append(lines, fmt.Sprintf("HiddenServicePort %d 127.0.0.1:%d", virtualPort, targetPort))

	// Add control password if configured
	if torConfig.ControlPassword != "" {
		lines = append(lines, fmt.Sprintf("HashedControlPassword %s", torConfig.ControlPassword))
	}

	// Write torrc
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(path, []byte(content), 0600)
}

// findTorBinary finds the Tor binary
func (t *TorService) findTorBinary() (string, error) {
	// Check configured path
	if t.config.Server.Tor.Binary != "" {
		if _, err := os.Stat(t.config.Server.Tor.Binary); err == nil {
			return t.config.Server.Tor.Binary, nil
		}
	}

	// Check common locations
	paths := []string{
		"/usr/bin/tor",
		"/usr/local/bin/tor",
		"/opt/homebrew/bin/tor",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Try to find in PATH
	path, err := exec.LookPath("tor")
	if err != nil {
		return "", fmt.Errorf("tor binary not found: please install tor or configure the binary path")
	}

	return path, nil
}

// waitForOnionAddress waits for the onion address file to be created
func (t *TorService) waitForOnionAddress(hiddenServiceDir string) {
	hostnameFile := filepath.Join(hiddenServiceDir, "hostname")

	// Wait up to 60 seconds for the hostname file
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)

		data, err := os.ReadFile(hostnameFile)
		if err != nil {
			continue
		}

		addr := strings.TrimSpace(string(data))
		if addr != "" {
			t.mu.Lock()
			t.onionAddr = addr
			t.mu.Unlock()
			log.Printf("[Tor] Hidden service address: %s", addr)
			return
		}
	}

	log.Println("[Tor] Warning: Could not read onion address after 60 seconds")
}

// CheckTorConnection tests if Tor is accessible
func CheckTorConnection(socksAddr string) bool {
	if socksAddr == "" {
		socksAddr = "127.0.0.1:9050"
	}

	conn, err := net.DialTimeout("tcp", socksAddr, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
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

	if t.running {
		socksPort := t.config.Server.Tor.SocksPort
		if socksPort == 0 {
			socksPort = 9050
		}
		status["socks_port"] = socksPort
		status["socks_connected"] = CheckTorConnection(fmt.Sprintf("127.0.0.1:%d", socksPort))
	}

	return status
}

// ReadTorLog reads recent lines from the Tor log
func (t *TorService) ReadTorLog(lines int) ([]string, error) {
	torDir := config.GetTorDir()
	logFile := filepath.Join(torDir, "tor.log")

	file, err := os.Open(logFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var logLines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		logLines = append(logLines, scanner.Text())
		if len(logLines) > lines {
			logLines = logLines[1:]
		}
	}

	return logLines, scanner.Err()
}

// TorHTTPClient creates an HTTP client that routes through Tor
type TorHTTPClient struct {
	socksAddr string
}

// NewTorHTTPClient creates a new Tor HTTP client
func NewTorHTTPClient(socksPort int) *TorHTTPClient {
	if socksPort == 0 {
		socksPort = 9050
	}
	return &TorHTTPClient{
		socksAddr: fmt.Sprintf("127.0.0.1:%d", socksPort),
	}
}

// CheckIP uses Tor to check the external IP address
func (c *TorHTTPClient) CheckIP() (string, error) {
	// This would require implementing a SOCKS5 HTTP client
	// For now, return a placeholder
	return "tor-connected", nil
}
