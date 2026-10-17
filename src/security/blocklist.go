// Package security implements security-related features per AI.md PART 11.
package security

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"golang.org/x/net/idna"
)

const blocklistMaxDownloadBytes int64 = 64 << 20

var blocklistSourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// BlocklistManager manages IP and domain blocklists per AI.md PART 18.
// Downloads blocklists from configured sources and maintains in-memory sets.
type BlocklistManager struct {
	mu             sync.RWMutex
	securityDirMu  sync.RWMutex
	blockedIPs     map[string]bool
	blockedNets    []*net.IPNet
	blockedDomains map[string]bool
	dataDir        string
	securityDir    string
	sources        []BlocklistSource
	client         *http.Client
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
		blockedIPs:     make(map[string]bool),
		blockedNets:    make([]*net.IPNet, 0),
		blockedDomains: make(map[string]bool),
		dataDir:        dataDir,
		securityDir:    filepath.Join(dataDir, "security"),
		sources:        sources,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewBlocklistManagerWithSecurityDir uses the configured security data directory.
func NewBlocklistManagerWithSecurityDir(dataDir, securityDir string, sources []BlocklistSource) *BlocklistManager {
	manager := NewBlocklistManager(dataDir, sources)
	if strings.TrimSpace(securityDir) != "" {
		manager.securityDir = filepath.Clean(securityDir)
	}
	return manager
}

func (m *BlocklistManager) securityDataDir() string {
	m.securityDirMu.RLock()
	defer m.securityDirMu.RUnlock()
	return m.securityDir
}

func (m *BlocklistManager) blocklistDir() string {
	return filepath.Join(m.securityDataDir(), "blocklists")
}

// SetSecurityDir changes the external security data directory and reloads its cache.
func (m *BlocklistManager) SetSecurityDir(securityDir string) {
	securityDir = strings.TrimSpace(securityDir)
	if securityDir == "" {
		securityDir = filepath.Join(m.dataDir, "security")
	}
	m.securityDirMu.Lock()
	m.securityDir = filepath.Clean(securityDir)
	m.securityDirMu.Unlock()
	if err := m.LoadFromDisk(); err != nil {
		slog.Warn("blocklist reload after security directory change failed", "error", err)
	}
}

// SetSources replaces the configured feeds used by subsequent updates and reloads.
func (m *BlocklistManager) SetSources(sources []BlocklistSource) {
	m.mu.Lock()
	m.sources = append([]BlocklistSource(nil), sources...)
	m.mu.Unlock()
	if err := m.LoadFromDisk(); err != nil {
		slog.Warn("blocklist reload after configuration change failed", "error", err)
	}
}

func (m *BlocklistManager) sourceSnapshot() []BlocklistSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]BlocklistSource(nil), m.sources...)
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

// IsBlockedDomain checks a domain and its subdomains against loaded blocklists.
func (m *BlocklistManager) IsBlockedDomain(host string) bool {
	domain := normalizeBlocklistDomain(host)
	if domain == "" {
		return false
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for candidate := domain; candidate != ""; {
		if m.blockedDomains[candidate] {
			return true
		}
		separator := strings.IndexByte(candidate, '.')
		if separator < 0 {
			break
		}
		candidate = candidate[separator+1:]
	}
	return false
}

// Update downloads and parses all enabled blocklist sources.
// Per AI.md PART 18: blocklist_update task runs daily at 04:00.
func (m *BlocklistManager) Update(ctx context.Context) error {
	slog.Info("starting blocklist update")

	blocklistDir := m.blocklistDir()
	if err := os.MkdirAll(blocklistDir, 0700); err != nil {
		return fmt.Errorf("failed to create blocklist directory: %w", err)
	}

	newIPs := make(map[string]bool)
	var newNets []*net.IPNet
	newDomains := make(map[string]bool)
	var updateErrors []string

	for _, source := range m.sourceSnapshot() {
		if !source.Enabled {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ips, nets, domains, err := m.downloadAndParse(ctx, source, blocklistDir)
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
		for domain := range domains {
			newDomains[domain] = true
		}

		slog.Info("blocklist source updated",
			"source", source.Name,
			"ips", len(ips),
			"nets", len(nets),
			"domains", len(domains))
	}

	// A partial update must not discard rules from feeds that failed. Retain
	// the last known rules and merge successful additions until every feed
	// can be refreshed together.
	m.mu.Lock()
	if len(updateErrors) > 0 {
		for ip := range m.blockedIPs {
			newIPs[ip] = true
		}
		for domain := range m.blockedDomains {
			newDomains[domain] = true
		}
		seenNets := make(map[string]bool, len(newNets)+len(m.blockedNets))
		for _, subnet := range newNets {
			seenNets[subnet.String()] = true
		}
		for _, subnet := range m.blockedNets {
			if !seenNets[subnet.String()] {
				newNets = append(newNets, subnet)
				seenNets[subnet.String()] = true
			}
		}
	}
	m.blockedIPs = newIPs
	m.blockedNets = newNets
	m.blockedDomains = newDomains
	m.mu.Unlock()

	slog.Info("blocklist update complete",
		"total_ips", len(newIPs),
		"total_nets", len(newNets),
		"total_domains", len(newDomains),
		"errors", len(updateErrors))

	if len(updateErrors) > 0 {
		return fmt.Errorf("blocklist update incomplete; retained last known rules: %s", strings.Join(updateErrors, "; "))
	}
	if err := writeSecurityDataTimestamp(blocklistDir, time.Now().UTC()); err != nil {
		return fmt.Errorf("record blocklist update timestamp: %w", err)
	}

	return nil
}

// downloadAndParse downloads a blocklist source and parses it.
func (m *BlocklistManager) downloadAndParse(ctx context.Context, source BlocklistSource, dir string) (map[string]bool, []*net.IPNet, map[string]bool, error) {
	if !blocklistSourceNamePattern.MatchString(source.Name) {
		return nil, nil, nil, fmt.Errorf("invalid source name")
	}
	if source.Type != "ip" && source.Type != "cidr" && source.Type != "domain" {
		return nil, nil, nil, fmt.Errorf("invalid source type")
	}
	if source.URL == "" {
		return nil, nil, nil, fmt.Errorf("source URL is required")
	}

	// Download file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create blocklist request")
	}

	resp, err := m.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, nil, ctx.Err()
		}
		return nil, nil, nil, fmt.Errorf("blocklist request failed")
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, nil, nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > blocklistMaxDownloadBytes {
		return nil, nil, nil, fmt.Errorf("blocklist exceeds %d-byte limit", blocklistMaxDownloadBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, blocklistMaxDownloadBytes+1))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to read blocklist response")
	}
	if int64(len(body)) > blocklistMaxDownloadBytes {
		return nil, nil, nil, fmt.Errorf("blocklist exceeds %d-byte limit", blocklistMaxDownloadBytes)
	}

	reader := bytes.NewReader(body)
	var ips map[string]bool
	var nets []*net.IPNet
	var domains map[string]bool
	if source.Type == "domain" {
		domains, err = parseDomainBlocklist(reader)
	} else {
		ips, nets, err = m.parseBlocklist(reader, source.Type)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if len(ips) == 0 && len(nets) == 0 && len(domains) == 0 {
		return nil, nil, nil, fmt.Errorf("blocklist contains no usable rules")
	}
	if err := writeBlocklistFile(filepath.Join(dir, source.Name+".txt"), body); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to persist blocklist: %w", err)
	}
	return ips, nets, domains, nil
}

func writeBlocklistFile(path string, body []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if err := tmp.Chmod(0600); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(body); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// parseDomainBlocklist accepts domain-per-line and hosts-file formats.
func parseDomainBlocklist(r io.Reader) (map[string]bool, error) {
	domains := make(map[string]bool)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.SplitN(scanner.Text(), "#", 2)[0])
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if net.ParseIP(fields[0]) != nil {
			fields = fields[1:]
		}
		for _, field := range fields {
			if domain := normalizeBlocklistDomain(field); domain != "" {
				domains[domain] = true
			}
		}
	}
	return domains, scanner.Err()
}

// normalizeBlocklistDomain canonicalizes a DNS name and rejects other entries.
func normalizeBlocklistDomain(host string) string {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		host = parsed
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if net.ParseIP(host) != nil || strings.ContainsAny(host, "/:@*\\") {
		return ""
	}
	ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.ToLower(host), "."))
	if err != nil || len(ascii) == 0 || len(ascii) > 253 || !strings.Contains(ascii, ".") {
		return ""
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return ""
			}
		}
	}
	return ascii
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
	blocklistDir := m.blocklistDir()

	entries, err := os.ReadDir(blocklistDir)
	if os.IsNotExist(err) {
		m.mu.Lock()
		m.blockedIPs = make(map[string]bool)
		m.blockedNets = nil
		m.blockedDomains = make(map[string]bool)
		m.mu.Unlock()
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read blocklist directory: %w", err)
	}

	newIPs := make(map[string]bool)
	var newNets []*net.IPNet
	newDomains := make(map[string]bool)
	sources := make(map[string]BlocklistSource)
	for _, source := range m.sourceSnapshot() {
		sources[source.Name] = source
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		sourceName := strings.TrimSuffix(entry.Name(), ".txt")
		source, configured := sources[sourceName]
		if !configured || !source.Enabled {
			continue
		}
		path := filepath.Join(blocklistDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to inspect blocklist %q: %w", entry.Name(), err)
		}
		if info.Size() > blocklistMaxDownloadBytes {
			return fmt.Errorf("blocklist %q exceeds %d-byte limit", entry.Name(), blocklistMaxDownloadBytes)
		}

		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open blocklist %q: %w", entry.Name(), err)
		}

		var ips map[string]bool
		var nets []*net.IPNet
		var domains map[string]bool
		if source.Type == "domain" {
			domains, err = parseDomainBlocklist(file)
		} else {
			ips, nets, err = m.parseBlocklist(file, source.Type)
		}
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			return fmt.Errorf("failed to parse blocklist %q: %w", entry.Name(), err)
		}

		for ip := range ips {
			newIPs[ip] = true
		}
		newNets = append(newNets, nets...)
		for domain := range domains {
			newDomains[domain] = true
		}
	}

	m.mu.Lock()
	m.blockedIPs = newIPs
	m.blockedNets = newNets
	m.blockedDomains = newDomains
	m.mu.Unlock()

	slog.Info("blocklists loaded from disk",
		"total_ips", len(newIPs),
		"total_nets", len(newNets),
		"total_domains", len(newDomains))

	return nil
}

// Count returns the total number of blocked entries.
func (m *BlocklistManager) Count() (ips, nets int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.blockedIPs), len(m.blockedNets)
}

// DomainCount returns the number of unique blocked domain entries.
func (m *BlocklistManager) DomainCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.blockedDomains)
}

// NeedsInitialUpdate reports whether enabled feeds exist but no usable rules
// have been loaded yet.
func (m *BlocklistManager) NeedsInitialUpdate() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.blockedIPs) > 0 || len(m.blockedNets) > 0 || len(m.blockedDomains) > 0 {
		return false
	}
	for _, source := range m.sources {
		if source.Enabled {
			return true
		}
	}
	return false
}
