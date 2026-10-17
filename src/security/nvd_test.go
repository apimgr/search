package security

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestCVEManagerNVDFullSyncPersistsAndReloads(t *testing.T) {
	payload := `{"resultsPerPage":2,"startIndex":0,"totalResults":2,"vulnerabilities":[
		{"cve":{"id":"CVE-2024-0001","published":"2024-01-01T00:00:00.000","lastModified":"2024-01-02T00:00:00.000","descriptions":[{"lang":"fr","value":"français"},{"lang":"en","value":"English description"}],"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":8.8,"baseSeverity":"HIGH"}}]},"configurations":[{"nodes":[{"cpeMatch":[{"criteria":"cpe:2.3:a:vendor:product:*:*:*:*:*:*:*:*","versionEndExcluding":"2.4.0"}]}]}],"references":[{"url":"https://vendor.example/advisory"}]}},
		{"cve":{"id":"CVE-2024-0002","descriptions":[{"lang":"en","value":"second record"}]}}
	]}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("resultsPerPage") != strconv.Itoa(nvdPageSize) {
			t.Errorf("resultsPerPage = %q, want %d", r.URL.Query().Get("resultsPerPage"), nvdPageSize)
		}
		if r.URL.Query().Has("lastModStartDate") {
			t.Error("initial full sync unexpectedly filtered by last-modified date")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	manager := NewCVEManager(t.TempDir(), []CVESource{{Name: "nvd", URL: server.URL}})
	manager.client = server.Client()
	if err := manager.Update(context.Background()); err != nil {
		t.Fatalf("initial NVD sync: %v", err)
	}
	if manager.Count() != 2 {
		t.Fatalf("loaded %d NVD records, want 2", manager.Count())
	}
	entry, ok := manager.Lookup("CVE-2024-0001")
	if !ok || entry.Description != "English description" || entry.CVSS != 8.8 || entry.Severity != "HIGH" {
		t.Fatalf("normalized NVD record = %#v, found %v", entry, ok)
	}
	if len(entry.Affected) != 1 || entry.Affected[0] != "cpe:2.3:a:vendor:product:*:*:*:*:*:*:*:* (end excluding 2.4.0)" {
		t.Errorf("affected CPE/range = %#v", entry.Affected)
	}
	if len(entry.References) != 1 || entry.References[0] != "https://vendor.example/advisory" {
		t.Errorf("references = %#v", entry.References)
	}
	cachePath := filepath.Join(manager.dataDir, "security", "cve", "nvd.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("NVD cache missing: %v", err)
	}
	reloaded := NewCVEManager(manager.dataDir, []CVESource{{Name: "nvd", URL: server.URL}})
	if err := reloaded.LoadFromDisk(); err != nil {
		t.Fatalf("load NVD cache: %v", err)
	}
	if reloaded.Count() != 2 || reloaded.LastSync().IsZero() {
		t.Fatalf("reloaded count/time = %d/%v", reloaded.Count(), reloaded.LastSync())
	}
}

func TestCVEManagerNVDIncrementalSyncKeepsUnchangedRecords(t *testing.T) {
	responses := []string{
		`{"resultsPerPage":1,"startIndex":0,"totalResults":1,"vulnerabilities":[{"cve":{"id":"CVE-2024-1111","descriptions":[{"lang":"en","value":"before"}]}}]}`,
		`{"resultsPerPage":1,"startIndex":0,"totalResults":1,"vulnerabilities":[{"cve":{"id":"CVE-2024-1111","lastModified":"2024-02-02T00:00:00.000","descriptions":[{"lang":"en","value":"after"}]}}]}`,
	}
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls == 1 && !r.URL.Query().Has("lastModStartDate") {
			t.Error("incremental sync omitted lastModStartDate")
		}
		if calls >= len(responses) {
			t.Errorf("unexpected NVD request %s", r.URL)
			http.Error(w, "unexpected request", http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprint(w, responses[calls])
		calls++
	}))
	defer server.Close()
	manager := NewCVEManager(t.TempDir(), []CVESource{{Name: "nvd", URL: server.URL}})
	manager.client = server.Client()
	for i := range responses {
		if i == 1 {
			manager.lastSync = time.Now().UTC().Add(-time.Hour)
		}
		if err := manager.Update(context.Background()); err != nil {
			t.Fatalf("NVD sync %d: %v", i+1, err)
		}
	}
	entry, ok := manager.Lookup("CVE-2024-1111")
	if !ok || entry.Description != "after" || manager.Count() != 1 {
		t.Fatalf("incremental NVD record/count = %#v/%d", entry, manager.Count())
	}
}

func TestCVEManagerNVDFailurePreservesLoadedCache(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"resultsPerPage":1,"startIndex":0,"totalResults":2,"vulnerabilities":[{"cve":{"id":"CVE-2024-0001","descriptions":[{"lang":"en","value":"partial"}]}}]}`))
	}))
	defer server.Close()
	manager := NewCVEManager(t.TempDir(), []CVESource{{Name: "nvd", URL: server.URL}})
	manager.client = server.Client()
	manager.entries["CVE-2020-0001"] = CVEEntry{ID: "CVE-2020-0001", Description: "cached"}
	before := manager.LastSync()
	if err := manager.Update(context.Background()); err == nil {
		t.Fatal("incomplete NVD pagination succeeded")
	}
	if manager.Count() != 1 {
		t.Fatalf("partial NVD response changed in-memory cache: %d entries", manager.Count())
	}
	if !manager.LastSync().Equal(before) {
		t.Fatal("failed NVD update advanced last sync")
	}
	if _, err := os.Stat(filepath.Join(manager.dataDir, "security", "cve", "nvd.json")); !os.IsNotExist(err) {
		t.Fatalf("failed update wrote cache file: %v", err)
	}
}

func TestCVEManagerSetSecurityDirReloadsSelectedCache(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	manager := NewCVEManagerWithSecurityDir(t.TempDir(), firstDir, []CVESource{})
	if err := os.MkdirAll(filepath.Join(firstDir, "cve"), 0750); err != nil {
		t.Fatal(err)
	}
	cache := nvdCache{
		LastUpdated: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		Entries:     []CVEEntry{{ID: "CVE-2024-0101", Description: "cached in first directory"}},
	}
	if err := manager.writeNVDCache(cache); err != nil {
		t.Fatal(err)
	}
	if err := manager.LoadFromDisk(); err != nil {
		t.Fatal(err)
	}
	if manager.Count() != 1 {
		t.Fatalf("initial cache count = %d, want 1", manager.Count())
	}
	if err := manager.SetSecurityDir(secondDir); err != nil {
		t.Fatalf("change security directory: %v", err)
	}
	if manager.Count() != 0 || !manager.LastSync().IsZero() {
		t.Fatalf("cache from previous directory retained: count=%d last_sync=%v", manager.Count(), manager.LastSync())
	}
}
