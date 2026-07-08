package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestCVEManager_Update_CISA exercises the full Update path with a mock HTTP
// server returning CISA KEV catalog JSON. Verifies entries are parsed and stored.
func TestCVEManager_Update_CISA(t *testing.T) {
	cisaBody := `{
		"title": "CISA KEV",
		"catalogVersion": "2024.01",
		"dateReleased": "2024-01-01",
		"vulnerabilities": [
			{
				"cveID": "CVE-2024-1001",
				"vendorProject": "Acme",
				"product": "Widget",
				"vulnerabilityName": "Remote Code Execution",
				"dateAdded": "2024-01-01",
				"shortDescription": "Allows remote code execution via crafted input"
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cisaBody))
	}))
	defer srv.Close()

	sources := []CVESource{
		{Name: "cisa_known_exploited", URL: srv.URL + "/cve.json"},
	}
	mgr := NewCVEManager(t.TempDir(), sources)
	mgr.client = &http.Client{}

	if err := mgr.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1", mgr.Count())
	}

	entry, ok := mgr.Lookup("CVE-2024-1001")
	if !ok {
		t.Fatal("CVE-2024-1001 not found after update")
	}
	if entry.Description != "Allows remote code execution via crafted input" {
		t.Errorf("Description = %q, want %q", entry.Description, "Allows remote code execution via crafted input")
	}
	if entry.Severity != "HIGH" {
		t.Errorf("Severity = %q, want HIGH", entry.Severity)
	}

	if mgr.LastSync().IsZero() {
		t.Error("LastSync() should not be zero after successful update")
	}
}

// TestCVEManager_Update_GenericJSON exercises the Update path with a non-CISA
// source that returns a generic CVE entry array.
func TestCVEManager_Update_GenericJSON(t *testing.T) {
	genericBody := `[
		{"id": "CVE-2024-2001", "description": "Integer overflow"},
		{"id": "CVE-2024-2002", "description": "Use after free"}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(genericBody))
	}))
	defer srv.Close()

	sources := []CVESource{
		{Name: "custom_source", URL: srv.URL + "/cve.json"},
	}
	mgr := NewCVEManager(t.TempDir(), sources)
	mgr.client = &http.Client{}

	if err := mgr.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if mgr.Count() != 2 {
		t.Errorf("Count() = %d, want 2", mgr.Count())
	}
}

// TestCVEManager_Update_SourceFails verifies that a failed source (HTTP 500) is
// logged and skipped; Update still returns nil when at least the directory setup
// succeeded (no entries needed to succeed when sources simply fail).
func TestCVEManager_Update_SourceFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	sources := []CVESource{
		{Name: "bad_source", URL: srv.URL + "/cve.json"},
	}
	mgr := NewCVEManager(t.TempDir(), sources)
	mgr.client = &http.Client{}

	// Update does not return error on individual source failure; it only logs.
	if err := mgr.Update(context.Background()); err != nil {
		t.Errorf("Update() error = %v, want nil (source failure is non-fatal)", err)
	}

	if mgr.Count() != 0 {
		t.Errorf("Count() = %d, want 0 (no data downloaded)", mgr.Count())
	}
}

// TestCVEManager_Update_HTTP404 verifies that a 404 response from a CVE source
// is treated as a non-fatal error.
func TestCVEManager_Update_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	sources := []CVESource{
		{Name: "missing_source", URL: srv.URL + "/missing.json"},
	}
	mgr := NewCVEManager(t.TempDir(), sources)
	mgr.client = &http.Client{}

	if err := mgr.Update(context.Background()); err != nil {
		t.Errorf("Update() error = %v, want nil", err)
	}
}

// TestCVEManager_Update_WritesFileToDisk verifies that downloaded CVE data is
// persisted to the configured data directory.
func TestCVEManager_Update_WritesFileToDisk(t *testing.T) {
	cisaBody := `{"vulnerabilities": [
		{"cveID": "CVE-2024-3001", "shortDescription": "Test"}
	]}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cisaBody))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	sources := []CVESource{
		{Name: "cisa_known_exploited", URL: srv.URL + "/cve.json"},
	}
	mgr := NewCVEManager(tmpDir, sources)
	mgr.client = &http.Client{}

	if err := mgr.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	expectedFile := filepath.Join(tmpDir, "security", "cve", "cisa_known_exploited.json")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("expected CVE file %s to exist after update", expectedFile)
	}
}

// TestCVEManager_Update_MultipleSources verifies that entries from several
// sources are merged into the single in-memory map.
func TestCVEManager_Update_MultipleSources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			w.Write([]byte(`[{"id": "CVE-2024-4001", "description": "A"}]`))
		case "/b":
			w.Write([]byte(`[{"id": "CVE-2024-4002", "description": "B"}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	sources := []CVESource{
		{Name: "src_a", URL: srv.URL + "/a"},
		{Name: "src_b", URL: srv.URL + "/b"},
	}
	mgr := NewCVEManager(t.TempDir(), sources)
	mgr.client = &http.Client{}

	if err := mgr.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if mgr.Count() != 2 {
		t.Errorf("Count() = %d, want 2", mgr.Count())
	}
}

// TestCVEManager_Update_ContextCanceled verifies that a pre-canceled context
// causes Update to abort with an error before contacting any source.
func TestCVEManager_Update_ContextCanceled(t *testing.T) {
	contacted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	sources := []CVESource{
		{Name: "unreachable", URL: srv.URL + "/cve.json"},
	}
	mgr := NewCVEManager(t.TempDir(), sources)
	mgr.client = &http.Client{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := mgr.Update(ctx)
	if err == nil {
		t.Error("Update() should return error when context is canceled")
	}

	if contacted {
		t.Error("Update() should not contact any source when context is pre-canceled")
	}
}

// TestCVEManager_parseCISA_InvalidJSON verifies that malformed JSON in a CISA
// feed returns 0 entries without panicking.
func TestCVEManager_parseCISA_InvalidJSON(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	count := mgr.parseCISA([]byte("this is not json {{{"))
	if count != 0 {
		t.Errorf("parseCISA(invalid JSON) = %d, want 0", count)
	}
}

// TestCVEManager_parseCISA_DuplicateCVE verifies that updating an existing CVE
// is counted as 0 new entries but still updates the stored description.
func TestCVEManager_parseCISA_DuplicateCVE(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	firstJSON := `{"vulnerabilities": [
		{"cveID": "CVE-2024-0001", "shortDescription": "Original description"}
	]}`

	count := mgr.parseCISA([]byte(firstJSON))
	if count != 1 {
		t.Errorf("first parseCISA() = %d, want 1", count)
	}

	// Second parse with same ID — should be 0 new entries.
	secondJSON := `{"vulnerabilities": [
		{"cveID": "CVE-2024-0001", "shortDescription": "Updated description"}
	]}`

	count = mgr.parseCISA([]byte(secondJSON))
	if count != 0 {
		t.Errorf("second parseCISA() = %d, want 0 (duplicate, not new)", count)
	}

	// Description should be updated to the new value.
	entry, ok := mgr.Lookup("CVE-2024-0001")
	if !ok {
		t.Fatal("CVE-2024-0001 not found")
	}
	if entry.Description != "Updated description" {
		t.Errorf("Description = %q, want %q", entry.Description, "Updated description")
	}
}

// TestCVEManager_parseSource_DefaultCase verifies that parseSource routes
// unrecognized source names to parseGenericJSON.
func TestCVEManager_parseSource_DefaultCase(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	data := []byte(`[{"id": "CVE-2024-9999", "description": "Generic"}]`)
	count, err := mgr.parseSource("some_unknown_source", data)
	if err != nil {
		t.Fatalf("parseSource() error = %v", err)
	}
	if count != 1 {
		t.Errorf("parseSource() = %d, want 1", count)
	}
}

// TestCVEManager_parseGenericJSON_EmptyID verifies that entries with empty IDs
// are skipped.
func TestCVEManager_parseGenericJSON_EmptyID(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	data := []byte(`[
		{"id": "", "description": "no ID entry"},
		{"id": "CVE-2024-1111", "description": "valid entry"}
	]`)
	count := mgr.parseGenericJSON(data)
	if count != 1 {
		t.Errorf("parseGenericJSON() = %d, want 1 (empty ID must be skipped)", count)
	}

	_, ok := mgr.Lookup("")
	if ok {
		t.Error("empty-ID entry should not be stored")
	}
}

// TestCVEManager_parseGenericJSON_DuplicateID verifies that updating an existing
// entry counts as 0 new entries.
func TestCVEManager_parseGenericJSON_DuplicateID(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})

	firstData := []byte(`[{"id": "CVE-2024-5555", "description": "First"}]`)
	count := mgr.parseGenericJSON(firstData)
	if count != 1 {
		t.Errorf("first parseGenericJSON() = %d, want 1", count)
	}

	secondData := []byte(`[{"id": "CVE-2024-5555", "description": "Second"}]`)
	count = mgr.parseGenericJSON(secondData)
	if count != 0 {
		t.Errorf("second parseGenericJSON() = %d, want 0 (duplicate)", count)
	}
}

// TestCVEManager_LoadFromDisk_SkipsNonJSON verifies that non-.json files in the
// CVE directory are silently skipped during LoadFromDisk.
func TestCVEManager_LoadFromDisk_SkipsNonJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cveDir := filepath.Join(tmpDir, "security", "cve")
	if err := os.MkdirAll(cveDir, 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a .txt file that should be ignored.
	if err := os.WriteFile(filepath.Join(cveDir, "notes.txt"), []byte("some text"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Write a valid .json file.
	cisaJSON := `{"vulnerabilities": [{"cveID": "CVE-2024-0042", "shortDescription": "Test"}]}`
	if err := os.WriteFile(filepath.Join(cveDir, "cisa_known_exploited.json"), []byte(cisaJSON), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	mgr := NewCVEManager(tmpDir, []CVESource{})
	if err := mgr.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk() error = %v", err)
	}

	if mgr.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (only .json files should load)", mgr.Count())
	}
}

// TestCVEManager_Search_MatchesAffected verifies that Search matches against
// the Affected field when the keyword isn't in the description.
func TestCVEManager_Search_NoKeywordMatch(t *testing.T) {
	mgr := NewCVEManager("/tmp/test", []CVESource{})
	mgr.entries["CVE-2024-7777"] = CVEEntry{
		ID:          "CVE-2024-7777",
		Description: "Memory corruption issue",
		Affected:    []string{"vendor/targetapp"},
	}

	results := mgr.Search("targetapp")
	if len(results) != 1 {
		t.Errorf("Search(\"targetapp\") = %d results, want 1", len(results))
	}

	results = mgr.Search("xyzzy_no_match")
	if len(results) != 0 {
		t.Errorf("Search(\"xyzzy_no_match\") = %d results, want 0", len(results))
	}
}

// TestCVEManager_Update_LargeResponse verifies that the download path works for
// multi-entry JSON responses (exercises the 100MiB read-limit code path).
func TestCVEManager_Update_LargeResponse(t *testing.T) {
	body := `[
		{"id": "CVE-2024-A001", "description": "a"},
		{"id": "CVE-2024-A002", "description": "b"},
		{"id": "CVE-2024-A003", "description": "c"}
	]`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(body))
	}))
	defer srv.Close()

	sources := []CVESource{
		{Name: "bulk_source", URL: srv.URL + "/bulk.json"},
	}
	mgr := NewCVEManager(t.TempDir(), sources)
	mgr.client = &http.Client{}

	if err := mgr.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if mgr.Count() != 3 {
		t.Errorf("Count() = %d, want 3", mgr.Count())
	}
}
