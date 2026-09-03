// Package security implements security-related features per AI.md PART 11.
package security

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// BlocklistManager manages IP and domain blocklists per AI.md PART 18.
// Downloads blocklists from configured sources and maintains in-memory sets.
type BlocklistManager struct {
	mu          sync.RWMutex
	blockedIPs  map[string]bool
	blockedNets []*net.IPNet
	dataDir     string
	sources     []BlocklistSource
	client      *http.Client
}

// BlocklistSource defines a blocklist download source.
type BlocklistSource struct {
	Name    string
	URL     string
	Type    string // "ip", "cidr", "domain"
	Enabled bool
}

// DefaultBlocklistSources returns the default blocklist sources.
// Per AI.md PART 18: Configurable sources, daily updates.
func DefaultBlocklistSources() []BlocklistSource {
	return []BlocklistSource{
		{
			Name:    "firehol_level1",
			URL:     "https://raw.githubusercontent.com/firehol/blocklist-ipsets/master/firehol_level1.netset",
			Type:    "cidr",
			Enabled: true,
		},
		{
			Name:    "emerging_threats",
			URL:     "https://rules.emergingthreats.net/fwrules/emerging-Block-IPs.txt",
			Type:    "ip",
			Enabled: true,
		},
	}
}

// NewBlocklistManager creates a new blocklist manager.
func NewBlocklistManager(dataDir string, sources []BlocklistSource) *BlocklistManager {
	if sources == nil {
		sources = DefaultBlocklistSources()
	}
	return &BlocklistManager{
		blockedIPs:  make(map[string]bool),
		blockedNets: make([]*net.IPNet, 0),
		dataDir:     dataDir,
		sources:     sources,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// IsBlocked checks if an IP address is in any blocklist.
func (m *BlocklistManager) IsBlocked(ip string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check exact IP match
	if m.blockedIPs[ip] {
		return true
	}

	// Check CIDR ranges
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	for _, cidr := range m.blockedNets {
		if cidr.Contains(parsedIP) {
			return true
		}
	}

	return false
}

// Update downloads and parses all enabled blocklist sources.
// Per AI.md PART 18: blocklist_update task runs daily at 04:00.
func (m *BlocklistManager) Update(ctx context.Context) error {
	slog.Info("starting blocklist update")

	blocklistDir := filepath.Join(m.dataDir, "security", "blocklists")
	if err := os.MkdirAll(blocklistDir, 0700); err != nil {
		return fmt.Errorf("failed to create blocklist directory: %w", err)
	}

	newIPs := make(map[string]bool)
	var newNets []*net.IPNet
	var updateErrors []string

	for _, source := range m.sources {
		if !source.Enabled {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ips, nets, err := m.downloadAndParse(ctx, source, blocklistDir)
		if err != nil {
			slog.Warn("blocklist source failed", "source", source.Name, "error", err)
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", source.Name, err))
			continue
		}

		// Merge results
		for ip := range ips {
			newIPs[ip] = true
		}
		newNets = append(newNets, nets...)

		slog.Info("blocklist source updated",
			"source", source.Name,
			"ips", len(ips),
			"nets", len(nets))
	}

	// Update in-memory blocklists
	m.mu.Lock()
	m.blockedIPs = newIPs
	m.blockedNets = newNets
	m.mu.Unlock()

	slog.Info("blocklist update complete",
		"total_ips", len(newIPs),
		"total_nets", len(newNets),
		"errors", len(updateErrors))

	if len(updateErrors) > 0 && len(newIPs) == 0 && len(newNets) == 0 {
		return fmt.Errorf("all blocklist sources failed: %s", strings.Join(updateErrors, "; "))
	}

	return nil
}

// downloadAndParse downloads a blocklist source and parses it.
func (m *BlocklistManager) downloadAndParse(ctx context.Context, source BlocklistSource, dir string) (map[string]bool, []*net.IPNet, error) {
	// Download file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Save to file
	filePath := filepath.Join(dir, source.Name+".txt")
	file, err := os.Create(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Tee to file while parsing
	reader := io.TeeReader(resp.Body, file)
	return m.parseBlocklist(reader, source.Type)
}

// parseBlocklist parses a blocklist from a reader.
func (m *BlocklistManager) parseBlocklist(r io.Reader, listType string) (map[string]bool, []*net.IPNet, error) {
	ips := make(map[string]bool)
	var nets []*net.IPNet

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Handle different formats
		switch listType {
		case "cidr":
			if strings.Contains(line, "/") {
				_, cidr, err := net.ParseCIDR(line)
				if err == nil {
					nets = append(nets, cidr)
				}
			} else {
				// Single IP in CIDR file
				if ip := net.ParseIP(line); ip != nil {
					ips[ip.String()] = true
				}
			}
		case "ip":
			// Extract IP from various formats (may have trailing comments)
			parts := strings.Fields(line)
			if len(parts) > 0 {
				ipStr := parts[0]
				if ip := net.ParseIP(ipStr); ip != nil {
					ips[ip.String()] = true
				}
			}
		}
	}

	return ips, nets, scanner.Err()
}

// LoadFromDisk loads blocklists from previously downloaded files.
func (m *BlocklistManager) LoadFromDisk() error {
	blocklistDir := filepath.Join(m.dataDir, "security", "blocklists")

	entries, err := os.ReadDir(blocklistDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read blocklist directory: %w", err)
	}

	newIPs := make(map[string]bool)
	var newNets []*net.IPNet

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		// Determine type from source config
		sourceName := strings.TrimSuffix(entry.Name(), ".txt")
		listType := "ip"
		for _, s := range m.sources {
			if s.Name == sourceName {
				listType = s.Type
				break
			}
		}

		file, err := os.Open(filepath.Join(blocklistDir, entry.Name()))
		if err != nil {
			slog.Warn("failed to open blocklist file", "file", entry.Name(), "error", err)
			continue
		}

		ips, nets, err := m.parseBlocklist(file, listType)
		file.Close()
		if err != nil {
			slog.Warn("failed to parse blocklist file", "file", entry.Name(), "error", err)
			continue
		}

		for ip := range ips {
			newIPs[ip] = true
		}
		newNets = append(newNets, nets...)
	}

	m.mu.Lock()
	m.blockedIPs = newIPs
	m.blockedNets = newNets
	m.mu.Unlock()

	slog.Info("blocklists loaded from disk",
		"total_ips", len(newIPs),
		"total_nets", len(newNets))

	return nil
}

// Count returns the total number of blocked entries.
func (m *BlocklistManager) Count() (ips, nets int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.blockedIPs), len(m.blockedNets)
}
