package security

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestBlocklistManager creates a BlocklistManager pointing at a mock HTTP server.
// The sources slice uses the server URL so no real network access is needed.
func newTestBlocklistManager(t *testing.T, dataDir string, sources []BlocklistSource) *BlocklistManager {
	t.Helper()
	return NewBlocklistManager(dataDir, sources)
}

// TestBlocklistManager_Update_Success verifies a full successful update cycle:
// the manager downloads from a mock HTTP server and populates in-memory state.
func TestBlocklistManager_Update_Success(t *testing.T) {
	ipListBody := "# comment\n1.2.3.4\n5.6.7.8\nnot-an-ip\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(ipListBody))
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "test_ip", URL: srv.URL + "/iplist.txt", Type: "ip", Enabled: true},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	if err := m.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	ips, _ := m.Count()
	if ips != 2 {
		t.Errorf("Count() ips = %d, want 2", ips)
	}

	if !m.IsBlocked("1.2.3.4") {
		t.Error("expected 1.2.3.4 to be blocked after update")
	}
	if !m.IsBlocked("5.6.7.8") {
		t.Error("expected 5.6.7.8 to be blocked after update")
	}
	if m.IsBlocked("9.9.9.9") {
		t.Error("expected 9.9.9.9 not to be blocked")
	}
}

// TestBlocklistManager_Update_CIDR verifies that CIDR-type blocklists are loaded
// correctly and that range membership checks work after an update.
func TestBlocklistManager_Update_CIDR(t *testing.T) {
	cidrBody := "10.0.0.0/8\n192.168.0.0/16\n# ignore\n172.16.0.1\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(cidrBody))
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "test_cidr", URL: srv.URL + "/cidr.txt", Type: "cidr", Enabled: true},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	if err := m.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	tests := []struct {
		ip   string
		want bool
	}{
		{"10.1.2.3", true},
		{"192.168.100.5", true},
		{"172.16.0.1", true},
		{"11.0.0.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := m.IsBlocked(tt.ip); got != tt.want {
				t.Errorf("IsBlocked(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestBlocklistManager_Update_SkipsDisabledSources ensures that sources with
// Enabled=false are never contacted, even if the context is live.
func TestBlocklistManager_Update_SkipsDisabledSources(t *testing.T) {
	contacted := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacted = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1.2.3.4\n"))
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "disabled", URL: srv.URL + "/list.txt", Type: "ip", Enabled: false},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	if err := m.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if contacted {
		t.Error("Update() contacted a disabled source")
	}

	ips, _ := m.Count()
	if ips != 0 {
		t.Errorf("Count() = %d, want 0 (disabled source should not populate)", ips)
	}
}

// TestBlocklistManager_Update_PartialFailure verifies that when one source fails
// (HTTP 500) but another succeeds, the update still succeeds and loads data
// from the working source.
func TestBlocklistManager_Update_PartialFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "bad") {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("3.3.3.3\n"))
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "bad_source", URL: srv.URL + "/bad", Type: "ip", Enabled: true},
		{Name: "good_source", URL: srv.URL + "/good", Type: "ip", Enabled: true},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	if err := m.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v, want nil (partial failure should not error)", err)
	}

	ips, _ := m.Count()
	if ips != 1 {
		t.Errorf("Count() ips = %d, want 1", ips)
	}
}

// TestBlocklistManager_Update_AllFail verifies that when every source fails
// AND there are no existing entries, Update returns an error.
func TestBlocklistManager_Update_AllFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "source_a", URL: srv.URL + "/a", Type: "ip", Enabled: true},
		{Name: "source_b", URL: srv.URL + "/b", Type: "ip", Enabled: true},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	err := m.Update(context.Background())
	if err == nil {
		t.Error("Update() should return error when all sources fail and nothing was loaded")
	}
}

// TestBlocklistManager_Update_NotFound verifies that an HTTP 404 from a single
// source is treated as a non-fatal error (logged and skipped).
func TestBlocklistManager_Update_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "missing", URL: srv.URL + "/404", Type: "ip", Enabled: true},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	// One source, one failure — Update returns error because all failed + nothing loaded.
	err := m.Update(context.Background())
	if err == nil {
		t.Error("Update() should return error when all sources fail")
	}
	if !strings.Contains(err.Error(), "all blocklist sources failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestBlocklistManager_Update_WritesFileToDisk verifies that a successful update
// saves the downloaded blocklist to the configured data directory.
func TestBlocklistManager_Update_WritesFileToDisk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("7.7.7.7\n"))
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	sources := []BlocklistSource{
		{Name: "mylist", URL: srv.URL + "/list", Type: "ip", Enabled: true},
	}
	m := newTestBlocklistManager(t, tmpDir, sources)

	if err := m.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	expectedFile := filepath.Join(tmpDir, "security", "blocklists", "mylist.txt")
	if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
		t.Errorf("expected file %s to exist after update", expectedFile)
	}
}

// TestBlocklistManager_Update_ContextCancelMidLoop verifies that a context
// canceled between source iterations causes Update to abort promptly.
func TestBlocklistManager_Update_ContextCancelMidLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1.1.1.1\n"))
		// Cancel after first response so the second source's ctx.Done() fires.
		cancel()
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "first", URL: srv.URL + "/first", Type: "ip", Enabled: true},
		{Name: "second", URL: srv.URL + "/second", Type: "ip", Enabled: true},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	err := m.Update(ctx)
	if err == nil {
		t.Error("Update() should return error when context canceled mid-loop")
	}
}

// TestBlocklistManager_ParseBlocklist_SemicolonComment verifies that lines
// starting with ";" are treated as comments and skipped.
func TestBlocklistManager_ParseBlocklist_SemicolonComment(t *testing.T) {
	m := NewBlocklistManager("", nil)
	input := "; this is a semicolon comment\n1.2.3.4\n"

	ips, _, err := m.parseBlocklist(strings.NewReader(input), "ip")
	if err != nil {
		t.Fatalf("parseBlocklist() error = %v", err)
	}
	if len(ips) != 1 {
		t.Errorf("len(ips) = %d, want 1", len(ips))
	}
}

// TestBlocklistManager_ParseBlocklist_InvalidIP verifies that invalid IP strings
// in an "ip"-type list are silently skipped.
func TestBlocklistManager_ParseBlocklist_InvalidIP(t *testing.T) {
	m := NewBlocklistManager("", nil)
	input := "not-an-ip\n999.999.999.999\n1.2.3.4\n"

	ips, _, err := m.parseBlocklist(strings.NewReader(input), "ip")
	if err != nil {
		t.Fatalf("parseBlocklist() error = %v", err)
	}
	if len(ips) != 1 {
		t.Errorf("len(ips) = %d, want 1 (only 1.2.3.4 is valid)", len(ips))
	}
}

// TestBlocklistManager_ParseBlocklist_InvalidCIDR verifies that invalid CIDR
// strings in a "cidr"-type list are silently skipped.
func TestBlocklistManager_ParseBlocklist_InvalidCIDR(t *testing.T) {
	m := NewBlocklistManager("", nil)
	input := "not-cidr/32\n10.0.0.0/8\n999.0.0.0/24\n"

	_, nets, err := m.parseBlocklist(strings.NewReader(input), "cidr")
	if err != nil {
		t.Fatalf("parseBlocklist() error = %v", err)
	}
	if len(nets) != 1 {
		t.Errorf("len(nets) = %d, want 1 (only 10.0.0.0/8 is valid)", len(nets))
	}
}

// TestBlocklistManager_IsBlocked_IPv6 verifies that IPv6 addresses are checked
// against the CIDR ranges correctly.
func TestBlocklistManager_IsBlocked_IPv6(t *testing.T) {
	m := NewBlocklistManager("", nil)

	_, ipv6Net, _ := net.ParseCIDR("2001:db8::/32")
	m.mu.Lock()
	m.blockedNets = append(m.blockedNets, ipv6Net)
	m.mu.Unlock()

	tests := []struct {
		ip   string
		want bool
	}{
		{"2001:db8::1", true},
		{"2001:db9::1", false},
		{"192.168.1.1", false},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			if got := m.IsBlocked(tt.ip); got != tt.want {
				t.Errorf("IsBlocked(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// TestBlocklistManager_LoadFromDisk_SkipsNonTxtFiles verifies that files without
// the ".txt" extension in the blocklist directory are ignored.
func TestBlocklistManager_LoadFromDisk_SkipsNonTxtFiles(t *testing.T) {
	tmpDir := t.TempDir()
	blocklistDir := filepath.Join(tmpDir, "security", "blocklists")
	if err := os.MkdirAll(blocklistDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write a .json file which should be ignored.
	if err := os.WriteFile(filepath.Join(blocklistDir, "ignore.json"), []byte("1.2.3.4"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Write a valid .txt file.
	if err := os.WriteFile(filepath.Join(blocklistDir, "valid.txt"), []byte("9.9.9.9\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	sources := []BlocklistSource{{Name: "valid", Type: "ip", Enabled: true}}
	m := NewBlocklistManager(tmpDir, sources)

	if err := m.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk() error = %v", err)
	}

	ips, _ := m.Count()
	if ips != 1 {
		t.Errorf("Count() ips = %d, want 1 (only valid.txt should be loaded)", ips)
	}
}

// TestBlocklistManager_LoadFromDisk_UnknownSource verifies that a .txt file whose
// name does not match any configured source is loaded using the default "ip" type.
func TestBlocklistManager_LoadFromDisk_UnknownSource(t *testing.T) {
	tmpDir := t.TempDir()
	blocklistDir := filepath.Join(tmpDir, "security", "blocklists")
	if err := os.MkdirAll(blocklistDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// "unknown_source.txt" does not match any source in the manager.
	if err := os.WriteFile(filepath.Join(blocklistDir, "unknown_source.txt"), []byte("4.4.4.4\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Manager has no sources at all.
	m := NewBlocklistManager(tmpDir, []BlocklistSource{})

	if err := m.LoadFromDisk(); err != nil {
		t.Fatalf("LoadFromDisk() error = %v", err)
	}

	ips, _ := m.Count()
	if ips != 1 {
		t.Errorf("Count() ips = %d, want 1 (unknown source defaults to ip type)", ips)
	}
}

// TestBlocklistManager_Update_MergeSources verifies that data from multiple
// sources is merged into the same in-memory blocklist.
func TestBlocklistManager_Update_MergeSources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			w.Write([]byte("1.1.1.1\n"))
		case "/b":
			w.Write([]byte("2.2.2.2\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	sources := []BlocklistSource{
		{Name: "src_a", URL: srv.URL + "/a", Type: "ip", Enabled: true},
		{Name: "src_b", URL: srv.URL + "/b", Type: "ip", Enabled: true},
	}
	m := newTestBlocklistManager(t, t.TempDir(), sources)

	if err := m.Update(context.Background()); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	ips, _ := m.Count()
	if ips != 2 {
		t.Errorf("Count() ips = %d, want 2 (one from each source)", ips)
	}
}
