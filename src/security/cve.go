package security

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
	mu       sync.RWMutex
	dataDir  string
	sources  []CVESource
	entries  map[string]CVEEntry
	client   *http.Client
	lastSync time.Time
}

// NewCVEManager creates a new CVE manager
func NewCVEManager(dataDir string, sources []CVESource) *CVEManager {
	if sources == nil {
		sources = DefaultCVESources()
	}
	return &CVEManager{
		dataDir: dataDir,
		sources: sources,
		entries: make(map[string]CVEEntry),
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

// DefaultCVESources returns the default CVE data sources
func DefaultCVESources() []CVESource {
	return []CVESource{
		{
			Name: "cisa_known_exploited",
			// CISA Known Exploited Vulnerabilities Catalog
			URL: "https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json",
		},
		{
			Name: "github_advisory",
			// GitHub Advisory Database (summary)
			URL: "https://github.com/advisories?query=type%3Areviewed",
		},
	}
}

// Update downloads and updates the CVE database from configured sources
func (m *CVEManager) Update(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cveDir := filepath.Join(m.dataDir, "security", "cve")
	if err := os.MkdirAll(cveDir, 0750); err != nil {
		return fmt.Errorf("create CVE directory: %w", err)
	}

	var totalNew int
	for _, source := range m.sources {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		count, err := m.downloadSource(ctx, source, cveDir)
		if err != nil {
			slog.Warn("CVE source download failed", "source", source.Name, "err", err)
			continue
		}
		totalNew += count
		slog.Info("CVE source updated", "source", source.Name, "entries", count)
	}

	m.lastSync = time.Now()
	slog.Info("CVE update complete", "total_entries", len(m.entries), "new_entries", totalNew)
	return nil
}

// downloadSource downloads CVE data from a single source
func (m *CVEManager) downloadSource(ctx context.Context, source CVESource, cveDir string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "search/1.0 (security-update)")

	resp, err := m.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Save raw data to disk
	destFile := filepath.Join(cveDir, source.Name+".json")
	tmpFile := destFile + ".tmp"

	f, err := os.Create(tmpFile)
	if err != nil {
		return 0, fmt.Errorf("create temp file: %w", err)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024*1024))
	if err != nil {
		f.Close()
		os.Remove(tmpFile)
		return 0, fmt.Errorf("read response: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmpFile)
		return 0, fmt.Errorf("write file: %w", err)
	}
	f.Close()

	if err := os.Rename(tmpFile, destFile); err != nil {
		os.Remove(tmpFile)
		return 0, fmt.Errorf("rename file: %w", err)
	}

	// Parse entries based on source type
	return m.parseSource(source.Name, data)
}

// parseSource parses CVE entries from downloaded data
func (m *CVEManager) parseSource(sourceName string, data []byte) (int, error) {
	var count int

	switch sourceName {
	case "cisa_known_exploited":
		count = m.parseCISA(data)
	default:
		// Generic JSON parsing for unknown sources
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

// parseGenericJSON attempts to parse generic JSON CVE data
func (m *CVEManager) parseGenericJSON(data []byte) int {
	// Try parsing as array of CVE entries
	var entries []CVEEntry
	if err := json.Unmarshal(data, &entries); err == nil {
		var count int
		for _, e := range entries {
			if e.ID != "" {
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

	cveDir := filepath.Join(m.dataDir, "security", "cve")
	if _, err := os.Stat(cveDir); os.IsNotExist(err) {
		return nil
	}

	files, err := os.ReadDir(cveDir)
	if err != nil {
		return fmt.Errorf("read CVE directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
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
