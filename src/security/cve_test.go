package security

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCVEManager_NewCVEManager(t *testing.T) {
	tests := []struct {
		name        string
		dataDir     string
		sources     []CVESource
		wantSources int
	}{
		{
			name:        "nil sources uses defaults",
			dataDir:     "/tmp/test",
			sources:     nil,
			wantSources: len(DefaultCVESources()),
		},
		{
			name:    "custom sources",
			dataDir: "/tmp/test",
			sources: []CVESource{
				{Name: "custom", URL: "https://example.com/cve.json"},
			},
			wantSources: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := NewCVEManager(tt.dataDir, tt.sources)
			if mgr == nil {
				t.Fatal("NewCVEManager returned nil")
			}
			if len(mgr.sources) != tt.wantSources {
				t.Errorf("sources = %d, want %d", len(mgr.sources), tt.wantSources)
			}
		})
	}
}

func TestCVEManager_DefaultCVESources(t *testing.T) {
	sources := DefaultCVESources()
	if len(sources) == 0 {
		t.Error("DefaultCVESources returned empty slice")
	}

	for _, s := range sources {
		if s.Name == "" {
			t.Error("source has empty name")
		}
		if s.URL == "" {
			t.Error("source has empty URL")
		}
		if !strings.HasPrefix(s.URL, "https://") {
			t.Errorf("source %s URL should use HTTPS: %s", s.Name, s.URL)
		}
	}
}

func TestCVEManager_Count(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})
	if mgr.Count() != 0 {
		t.Errorf("Count() = %d, want 0", mgr.Count())
	}

	// Add an entry directly
	mgr.entries["CVE-2024-0001"] = CVEEntry{ID: "CVE-2024-0001"}
	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}
}

func TestCVEManager_Lookup(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})
	mgr.entries["CVE-2024-1234"] = CVEEntry{
		ID:          "CVE-2024-1234",
		Description: "Test vulnerability",
		Severity:    "HIGH",
	}

	tests := []struct {
		name   string
		cveID  string
		wantOK bool
	}{
		{"existing CVE", "CVE-2024-1234", true},
		{"non-existent CVE", "CVE-2024-9999", false},
		{"empty ID", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := mgr.Lookup(tt.cveID)
			if ok != tt.wantOK {
				t.Errorf("Lookup(%q) ok = %v, want %v", tt.cveID, ok, tt.wantOK)
			}
			if ok && entry.ID != tt.cveID {
				t.Errorf("Lookup(%q) ID = %q, want %q", tt.cveID, entry.ID, tt.cveID)
			}
		})
	}
}

func TestCVEManager_Search(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})
	mgr.entries["CVE-2024-1234"] = CVEEntry{
		ID:          "CVE-2024-1234",
		Description: "SQL injection in login form",
		Affected:    []string{"vendor/product-a"},
	}
	mgr.entries["CVE-2024-5678"] = CVEEntry{
		ID:          "CVE-2024-5678",
		Description: "XSS vulnerability in admin panel",
		Affected:    []string{"vendor/product-b"},
	}

	tests := []struct {
		name    string
		keyword string
		want    int
	}{
		{"match description", "injection", 1},
		{"match affected", "product-a", 1},
		{"match multiple", "vendor", 2},
		{"no match", "buffer overflow", 0},
		{"case insensitive", "SQL", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := mgr.Search(tt.keyword)
			if len(results) != tt.want {
				t.Errorf("Search(%q) = %d results, want %d", tt.keyword, len(results), tt.want)
			}
		})
	}
}

func TestCVEManager_ParseCISA(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	cisaJSON := `{
		"title": "CISA KEV Catalog",
		"catalogVersion": "2024.01.01",
		"dateReleased": "2024-01-01",
		"vulnerabilities": [
			{
				"cveID": "CVE-2024-0001",
				"vendorProject": "TestVendor",
				"product": "TestProduct",
				"vulnerabilityName": "Test Vuln",
				"dateAdded": "2024-01-01",
				"shortDescription": "A test vulnerability"
			},
			{
				"cveID": "CVE-2024-0002",
				"vendorProject": "OtherVendor",
				"product": "OtherProduct",
				"vulnerabilityName": "Other Vuln",
				"dateAdded": "2024-01-02",
				"shortDescription": "Another test vulnerability"
			}
		]
	}`

	count := mgr.parseCISA([]byte(cisaJSON))
	if count != 2 {
		t.Errorf("parseCISA() = %d, want 2", count)
	}

	if mgr.Count() != 2 {
		t.Errorf("Count() = %d, want 2", mgr.Count())
	}

	entry, ok := mgr.Lookup("CVE-2024-0001")
	if !ok {
		t.Error("CVE-2024-0001 not found")
	}
	if entry.Description != "A test vulnerability" {
		t.Errorf("Description = %q, want %q", entry.Description, "A test vulnerability")
	}
}

func TestCVEManager_LoadFromDisk(t *testing.T) {
	tmpDir := t.TempDir()
	cveDir := filepath.Join(tmpDir, "security", "cve")
	if err := os.MkdirAll(cveDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write test CISA data
	cisaJSON := `{
		"vulnerabilities": [
			{"cveID": "CVE-2024-1111", "shortDescription": "Test from disk"}
		]
	}`
	if err := os.WriteFile(filepath.Join(cveDir, "cisa_known_exploited.json"), []byte(cisaJSON), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	mgr := NewCVEManager(tmpDir, []CVESource{})
	if err := mgr.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk: %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}

	entry, ok := mgr.Lookup("CVE-2024-1111")
	if !ok {
		t.Error("CVE-2024-1111 not found")
	}
	if entry.Description != "Test from disk" {
		t.Errorf("Description = %q, want %q", entry.Description, "Test from disk")
	}
}

func TestCVEManager_LoadFromDisk_NoDir(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCVEManager(tmpDir, []CVESource{})

	// Should not error when directory doesn't exist
	if err := mgr.LoadFromDisk(); err != nil {
		t.Errorf("LoadFromDisk() error = %v, want nil", err)
	}
}

func TestCVEManager_Update_Canceled(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewCVEManager(tmpDir, DefaultCVESources())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.Update(ctx)
	if err == nil {
		t.Error("Update with canceled context should return error")
	}
}

func TestCVEManager_LastSync(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	// Initially should be zero
	if !mgr.LastSync().IsZero() {
		t.Error("LastSync() should be zero initially")
	}

	// After setting, should return the time
	now := time.Now()
	mgr.lastSync = now
	if !mgr.LastSync().Equal(now) {
		t.Errorf("LastSync() = %v, want %v", mgr.LastSync(), now)
	}
}

func TestParseCVEList(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "valid CVEs",
			input: "CVE-2024-0001\nCVE-2024-0002\nCVE-2023-12345",
			want:  []string{"CVE-2024-0001", "CVE-2024-0002", "CVE-2023-12345"},
		},
		{
			name:  "with comments",
			input: "# This is a comment\nCVE-2024-0001\n# Another comment",
			want:  []string{"CVE-2024-0001"},
		},
		{
			name:  "with empty lines",
			input: "CVE-2024-0001\n\n\nCVE-2024-0002\n",
			want:  []string{"CVE-2024-0001", "CVE-2024-0002"},
		},
		{
			name:  "invalid entries skipped",
			input: "CVE-2024-0001\nnot-a-cve\nCVE-2024-0002",
			want:  []string{"CVE-2024-0001", "CVE-2024-0002"},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseCVEList(strings.NewReader(tt.input))
			if len(got) != len(tt.want) {
				t.Errorf("ParseCVEList() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ParseCVEList()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCVEManager_parseGenericJSON(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	// Valid array of CVE entries
	jsonData := `[
		{"id": "CVE-2024-0001", "description": "Test 1"},
		{"id": "CVE-2024-0002", "description": "Test 2"}
	]`

	count := mgr.parseGenericJSON([]byte(jsonData))
	if count != 2 {
		t.Errorf("parseGenericJSON() = %d, want 2", count)
	}

	// Invalid JSON returns 0
	count = mgr.parseGenericJSON([]byte("invalid json"))
	if count != 0 {
		t.Errorf("parseGenericJSON(invalid) = %d, want 0", count)
	}
}
