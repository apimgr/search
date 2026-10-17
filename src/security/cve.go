package security

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
)

// CVEEntry represents a single CVE record
type CVEEntry struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Severity    string   `json:"severity"`
	CVSS        float64  `json:"cvss"`
	Published   string   `json:"published"`
	Modified    string   `json:"modified"`
	References  []string `json:"references"`
	Affected    []string `json:"affected"`
}

// CVESource represents a CVE data source
type CVESource struct {
	Name string
	URL  string
}

// CVEManager manages CVE/security database updates
type CVEManager struct {
	mu            sync.RWMutex
	securityDirMu sync.RWMutex
	dataDir       string
	securityDir   string
	sources       []CVESource
	entries       map[string]CVEEntry
	client        *http.Client
	lastSync      time.Time
}

// NewCVEManager creates a new CVE manager
func NewCVEManager(dataDir string, sources []CVESource) *CVEManager {
	if sources == nil {
		sources = DefaultCVESources()
	}
	return &CVEManager{
		dataDir:     dataDir,
		securityDir: filepath.Join(dataDir, "security"),
		sources:     sources,
		entries:     make(map[string]CVEEntry),
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// NewCVEManagerWithSecurityDir uses the configured security data directory.
func NewCVEManagerWithSecurityDir(dataDir, securityDir string, sources []CVESource) *CVEManager {
	manager := NewCVEManager(dataDir, sources)
	if strings.TrimSpace(securityDir) != "" {
		manager.securityDir = filepath.Clean(securityDir)
	}
	return manager
}

func (m *CVEManager) securityDataDir() string {
	m.securityDirMu.RLock()
	defer m.securityDirMu.RUnlock()
	return m.securityDir
}

func (m *CVEManager) cveCacheDir() string {
	return filepath.Join(m.securityDataDir(), "cve")
}

// SetSources replaces the CVE feeds used by subsequent updates.
func (m *CVEManager) SetSources(sources []CVESource) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources = append([]CVESource(nil), sources...)
}

// SetSecurityDir changes the external security data directory and reloads its cache.
func (m *CVEManager) SetSecurityDir(securityDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	securityDir = strings.TrimSpace(securityDir)
	if securityDir == "" {
		securityDir = filepath.Join(m.dataDir, "security")
	}
	m.securityDirMu.Lock()
	m.securityDir = filepath.Clean(securityDir)
	m.securityDirMu.Unlock()
	m.entries = make(map[string]CVEEntry)
	m.lastSync = time.Time{}
	return m.loadFromDiskLocked()
}

// DefaultCVESources returns the default CVE data sources
func DefaultCVESources() []CVESource {
	return []CVESource{{
		Name: "nvd",
		URL:  "https://services.nvd.nist.gov/rest/json/cves/2.0",
	}}
}

// Update downloads and updates the CVE database from configured sources
func (m *CVEManager) Update(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cveDir := m.cveCacheDir()
	if err := os.MkdirAll(cveDir, 0750); err != nil {
		return fmt.Errorf("create CVE directory: %w", err)
	}

	var totalNew int
	var failedSources int
	for _, source := range m.sources {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var count int
		var err error
		if source.Name == "nvd" {
			count, err = m.updateNVD(ctx, source)
		} else {
			count, err = m.downloadSource(ctx, source, cveDir)
		}
		if err != nil {
			slog.Warn("CVE source download failed", "source", source.Name, "err", err)
			failedSources++
			continue
		}
		totalNew += count
		slog.Info("CVE source updated", "source", source.Name, "entries", count)
	}

	slog.Info("CVE update complete", "total_entries", len(m.entries), "new_entries", totalNew)
	if failedSources > 0 {
		return fmt.Errorf("CVE update incomplete: %d of %d sources failed", failedSources, len(m.sources))
	}
	m.lastSync = time.Now()
	return nil
}

// downloadSource downloads CVE data from a single source
func (m *CVEManager) downloadSource(ctx context.Context, source CVESource, cveDir string) (int, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`).MatchString(source.Name) || strings.TrimSpace(source.URL) == "" {
		return 0, fmt.Errorf("invalid CVE source configuration")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "search/1.0 (security-update)")

	resp, err := m.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download: %w", err)
	}
	defer closeutil.Close(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	const maxCVEFeedSize = 100 * 1024 * 1024
	if resp.ContentLength > maxCVEFeedSize {
		return 0, fmt.Errorf("CVE feed exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCVEFeedSize+1))
	if err != nil {
		return 0, fmt.Errorf("read response: %w", err)
	}
	if len(data) > maxCVEFeedSize {
		return 0, fmt.Errorf("CVE feed exceeds size limit")
	}
	count, err := m.parseSource(source.Name, data)
	if err != nil {
		return 0, err
	}

	destFile := filepath.Join(cveDir, source.Name+".json")
	f, err := os.CreateTemp(cveDir, ".cve-*.tmp")
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return 0, fmt.Errorf("write file: %w", errors.Join(err, f.Close(), os.Remove(f.Name())))
	}
	if err := f.Sync(); err != nil {
		return 0, fmt.Errorf("sync file: %w", errors.Join(err, f.Close(), os.Remove(f.Name())))
	}
	if err := f.Close(); err != nil {
		return 0, fmt.Errorf("close file: %w", errors.Join(err, os.Remove(f.Name())))
	}
	if err := os.Rename(f.Name(), destFile); err != nil {
		return 0, fmt.Errorf("rename file: %w", errors.Join(err, os.Remove(f.Name())))
	}
	return count, nil
}

// parseSource parses CVE entries from downloaded data
func (m *CVEManager) parseSource(sourceName string, data []byte) (int, error) {
	var count int

	switch sourceName {
	case "nvd":
		var response nvdResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return 0, fmt.Errorf("parse NVD feed: invalid JSON")
		}
		if response.Vulnerabilities == nil && response.TotalResults != 0 {
			return 0, fmt.Errorf("parse NVD feed: missing vulnerabilities")
		}
		for _, item := range response.Vulnerabilities {
			entry, ok := normalizeNVDEntry(item.CVE)
			if !ok {
				continue
			}
			if _, exists := m.entries[entry.ID]; !exists {
				count++
			}
			m.entries[entry.ID] = entry
		}
	case "cisa_known_exploited":
		var response CISAResponse
		if err := json.Unmarshal(data, &response); err != nil {
			return 0, fmt.Errorf("parse CVE feed: invalid JSON")
		}
		if response.Vulnerabilities == nil {
			return 0, fmt.Errorf("parse CVE feed: missing vulnerabilities")
		}
		count = m.parseCISA(data)
	default:
		var entries []CVEEntry
		if err := json.Unmarshal(data, &entries); err != nil || entries == nil {
			return 0, fmt.Errorf("parse CVE feed: expected a CVE array")
		}
		count = m.parseGenericJSON(data)
	}

	return count, nil
}

// CISAResponse represents the CISA KEV catalog response
type CISAResponse struct {
	Title           string `json:"title"`
	CatalogVersion  string `json:"catalogVersion"`
	DateReleased    string `json:"dateReleased"`
	Vulnerabilities []struct {
		CVEID             string `json:"cveID"`
		VendorProject     string `json:"vendorProject"`
		Product           string `json:"product"`
		VulnerabilityName string `json:"vulnerabilityName"`
		DateAdded         string `json:"dateAdded"`
		ShortDescription  string `json:"shortDescription"`
		RequiredAction    string `json:"requiredAction"`
		DueDate           string `json:"dueDate"`
	} `json:"vulnerabilities"`
}

// parseCISA parses CISA Known Exploited Vulnerabilities catalog
func (m *CVEManager) parseCISA(data []byte) int {
	var resp CISAResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		slog.Debug("CISA parse error", "err", err)
		return 0
	}

	var count int
	for _, v := range resp.Vulnerabilities {
		if !validCVEID(v.CVEID) {
			continue
		}
		if _, exists := m.entries[v.CVEID]; !exists {
			count++
		}
		m.entries[v.CVEID] = CVEEntry{
			ID:          v.CVEID,
			Description: v.ShortDescription,
			Severity:    "HIGH",
			Published:   v.DateAdded,
			Affected:    []string{v.VendorProject + "/" + v.Product},
		}
	}
	return count
}

func validCVEID(id string) bool {
	return regexp.MustCompile(`^CVE-[0-9]{4}-[0-9]{4,}$`).MatchString(strings.ToUpper(id))
}

// parseGenericJSON attempts to parse generic JSON CVE data
func (m *CVEManager) parseGenericJSON(data []byte) int {
	// Try parsing as array of CVE entries
	var entries []CVEEntry
	if err := json.Unmarshal(data, &entries); err == nil {
		var count int
		for _, e := range entries {
			if validCVEID(e.ID) {
				if _, exists := m.entries[e.ID]; !exists {
					count++
				}
				m.entries[e.ID] = e
			}
		}
		return count
	}
	return 0
}

// LoadFromDisk loads cached CVE data from disk
func (m *CVEManager) LoadFromDisk() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadFromDiskLocked()
}

func (m *CVEManager) loadFromDiskLocked() error {
	cveDir := m.cveCacheDir()
	if _, err := os.Stat(cveDir); os.IsNotExist(err) {
		return nil
	}

	nvdPath := filepath.Join(cveDir, "nvd.json")
	if cache, err := readNVDCache(nvdPath); err == nil {
		for _, entry := range cache.Entries {
			if validCVEID(entry.ID) {
				m.entries[entry.ID] = entry
			}
		}
		m.lastSync = cache.LastUpdated
	} else if !os.IsNotExist(err) {
		slog.Warn("NVD cache load failed", "err", err)
	}

	files, err := os.ReadDir(cveDir)
	if err != nil {
		return fmt.Errorf("read CVE directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || file.Name() == "nvd.json" || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		filePath := filepath.Join(cveDir, file.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			slog.Debug("read CVE file failed", "file", file.Name(), "err", err)
			continue
		}

		sourceName := strings.TrimSuffix(file.Name(), ".json")
		count, _ := m.parseSource(sourceName, data)
		slog.Debug("loaded CVE source", "source", sourceName, "entries", count)
	}

	return nil
}

// Lookup returns a CVE entry by ID
func (m *CVEManager) Lookup(cveID string) (CVEEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.entries[cveID]
	return entry, ok
}

// Search searches CVE entries by keyword in description or affected products
func (m *CVEManager) Search(keyword string) []CVEEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keyword = strings.ToLower(keyword)
	var results []CVEEntry

	for _, entry := range m.entries {
		if strings.Contains(strings.ToLower(entry.Description), keyword) {
			results = append(results, entry)
			continue
		}
		for _, affected := range entry.Affected {
			if strings.Contains(strings.ToLower(affected), keyword) {
				results = append(results, entry)
				break
			}
		}
	}

	return results
}

// Count returns the number of CVE entries loaded
func (m *CVEManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// NeedsInitialUpdate reports whether configured CVE feeds exist but no CVE
// entries have been loaded yet.
func (m *CVEManager) NeedsInitialUpdate() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries) == 0 && len(m.sources) > 0
}

// LastSync returns the time of the last successful sync
func (m *CVEManager) LastSync() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.lastSync
}

// ParseCVEList parses a simple text file with one CVE ID per line
func ParseCVEList(r io.Reader) []string {
	var ids []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Basic CVE ID validation: CVE-YYYY-NNNNN
		if strings.HasPrefix(line, "CVE-") {
			ids = append(ids, line)
		}
	}
	return ids
}
