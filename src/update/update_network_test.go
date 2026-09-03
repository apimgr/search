package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// redirectTransport intercepts all outgoing requests and forwards them to the
// mock server. fetchReleases and VerifyChecksum create http.Client{} without
// setting Transport, so they fall back to http.DefaultTransport — which we
// replace here to inject a mock without changing production code.
type redirectTransport struct {
	orig   http.RoundTripper
	target string
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newURL := rt.target + req.URL.Path
	if req.URL.RawQuery != "" {
		newURL += "?" + req.URL.RawQuery
	}
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range req.Header {
		for _, v := range vs {
			newReq.Header.Add(k, v)
		}
	}
	return rt.orig.RoundTrip(newReq)
}

// patchDefaultTransport replaces http.DefaultTransport to redirect all requests
// to server.URL. Returns a cleanup function that restores the original transport.
func patchDefaultTransport(t *testing.T, server *httptest.Server) func() {
	t.Helper()
	orig := http.DefaultTransport
	http.DefaultTransport = &redirectTransport{orig: orig, target: server.URL}
	return func() { http.DefaultTransport = orig }
}

// makeTarGz creates a .tar.gz file at archivePath containing a single regular
// file named entryName with the given content.
func makeTarGz(t *testing.T, archivePath, entryName string, content []byte) {
	t.Helper()
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("makeTarGz: Create %q: %v", archivePath, err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     entryName,
		Mode:     0755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("makeTarGz: WriteHeader: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("makeTarGz: Write content: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("makeTarGz: tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("makeTarGz: gzip Close: %v", err)
	}
}

// sha256HexFile returns the hex-encoded SHA-256 hash of the named file.
func sha256HexFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("sha256HexFile: ReadFile %q: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestFetchReleasesSuccess tests successful release retrieval via a mock server.
func TestFetchReleasesSuccess(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		{
			TagName:     "v1.2.0",
			Name:        "Release 1.2.0",
			Body:        "Improvements",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now,
			Assets: []Asset{
				{Name: "search_linux_amd64.tar.gz", Size: 5000, DownloadURL: "https://example.com/linux-amd64"},
			},
		},
		{
			TagName:     "v1.1.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now.Add(-24 * time.Hour),
			Assets:      []Asset{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Accept header = %q, want application/vnd.github.v3+json", r.Header.Get("Accept"))
		}
		if !strings.Contains(r.Header.Get("User-Agent"), "Search/") {
			t.Errorf("User-Agent = %q, want it to contain Search/", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	got, err := m.fetchReleases()
	if err != nil {
		t.Fatalf("fetchReleases() error = %v", err)
	}
	if len(got) != 2 {
		t.Errorf("fetchReleases() = %d releases, want 2", len(got))
	}
	if got[0].TagName != "v1.2.0" {
		t.Errorf("releases[0].TagName = %q, want v1.2.0", got[0].TagName)
	}
}

// TestFetchReleasesEmptyArray tests that an empty JSON array is handled without error.
func TestFetchReleasesEmptyArray(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	got, err := m.fetchReleases()
	if err != nil {
		t.Fatalf("fetchReleases() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("fetchReleases() = %d releases, want 0", len(got))
	}
}

// TestFetchReleasesNonOKStatus tests that non-200 HTTP responses return errors.
func TestFetchReleasesNonOKStatus(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"forbidden", http.StatusForbidden},
		{"internal server error", http.StatusInternalServerError},
		{"rate limited", http.StatusTooManyRequests},
		{"not found", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()
			restore := patchDefaultTransport(t, server)
			defer restore()

			m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
			_, err := m.fetchReleases()
			if err == nil {
				t.Errorf("fetchReleases() expected error for status %d, got nil", tt.statusCode)
				return
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("%d", tt.statusCode)) {
				t.Errorf("fetchReleases() error = %q, want it to include status code %d", err.Error(), tt.statusCode)
			}
		})
	}
}

// TestFetchReleasesInvalidJSON tests that malformed JSON returns an error.
func TestFetchReleasesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("{not valid json"))
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	_, err := m.fetchReleases()
	if err == nil {
		t.Error("fetchReleases() expected error for invalid JSON, got nil")
	}
}

// TestCheckForUpdatesUpdateAvailable tests detection of a newer available version.
func TestCheckForUpdatesUpdateAvailable(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		{
			TagName:     "v2.0.0",
			Name:        "Major Release",
			Body:        "Breaking changes",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now,
			Assets: []Asset{
				{
					Name:        fmt.Sprintf("search_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH),
					Size:        9999,
					DownloadURL: "https://example.com/download",
				},
				{Name: "checksums.txt", Size: 100, DownloadURL: "https://example.com/checksums"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	info, err := m.CheckForUpdates(false)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if !info.Available {
		t.Error("Available = false, want true (v2.0.0 > v1.0.0)")
	}
	if info.LatestVersion != "v2.0.0" {
		t.Errorf("LatestVersion = %q, want v2.0.0", info.LatestVersion)
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want 1.0.0", info.CurrentVersion)
	}
	if info.ChecksumURL != "https://example.com/checksums" {
		t.Errorf("ChecksumURL = %q, want https://example.com/checksums", info.ChecksumURL)
	}
}

// TestCheckForUpdatesAlreadyLatest tests that no update is reported when current == latest.
func TestCheckForUpdatesAlreadyLatest(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		{
			TagName:     "v1.0.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now,
			Assets:      []Asset{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	info, err := m.CheckForUpdates(false)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if info.Available {
		t.Error("Available = true for same version, want false")
	}
}

// TestCheckForUpdatesEmptyReleases tests CheckForUpdates when GitHub returns no releases.
func TestCheckForUpdatesEmptyReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	info, err := m.CheckForUpdates(false)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if info.Available {
		t.Error("Available = true for empty releases list, want false")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want 1.0.0", info.CurrentVersion)
	}
}

// TestCheckForUpdatesSkipsDraftsAndPrerelease verifies drafts and prereleases are filtered out.
func TestCheckForUpdatesSkipsDraftsAndPrerelease(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		{TagName: "v2.0.0", Draft: true, Prerelease: false, PublishedAt: now},
		{TagName: "v1.5.0-beta", Draft: false, Prerelease: true, PublishedAt: now.Add(-time.Hour)},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	info, err := m.CheckForUpdates(false)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if info.Available {
		t.Error("Available = true when only drafts/prereleases exist, want false")
	}
}

// TestCheckForUpdatesPrereleaseIncluded verifies prereleases surface when includePrerelease is true.
func TestCheckForUpdatesPrereleaseIncluded(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		{
			TagName:     "v2.0.0-beta",
			Draft:       false,
			Prerelease:  true,
			PublishedAt: now,
			Assets:      []Asset{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	info, err := m.CheckForUpdates(true)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if !info.Available {
		t.Error("Available = false with includePrerelease=true, want true")
	}
	if !info.IsPrerelease {
		t.Error("IsPrerelease = false, want true")
	}
}

// TestCheckForUpdatesNetworkError verifies that a server-side error wraps correctly.
func TestCheckForUpdatesNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	_, err := m.CheckForUpdates(false)
	if err == nil {
		t.Error("CheckForUpdates() expected error for server 500, got nil")
	}
	if !strings.Contains(err.Error(), "failed to fetch releases") {
		t.Errorf("CheckForUpdates() error = %q, want it to mention failed to fetch releases", err.Error())
	}
}

// TestCheckForUpdatesNoMatchingAsset tests that DownloadURL is empty when no asset matches the platform.
func TestCheckForUpdatesNoMatchingAsset(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		{
			TagName:     "v2.0.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now,
			Assets: []Asset{
				{Name: "search_fakeos_fakearch.tar.gz", Size: 100, DownloadURL: "https://example.com/fake"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	info, err := m.CheckForUpdates(false)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if info.DownloadURL != "" {
		t.Errorf("DownloadURL = %q, want empty string for no matching asset", info.DownloadURL)
	}
}

// TestListAvailableVersionsSuccess tests listing non-draft release versions.
func TestListAvailableVersionsSuccess(t *testing.T) {
	releases := []Release{
		{TagName: "v3.0.0", Draft: false},
		{TagName: "v2.0.0-draft", Draft: true},
		{TagName: "v2.0.0", Draft: false},
		{TagName: "v1.0.0", Draft: false},
		{TagName: "v0.9.0-draft", Draft: true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	versions, err := m.ListAvailableVersions()
	if err != nil {
		t.Fatalf("ListAvailableVersions() error = %v", err)
	}
	expected := []string{"v3.0.0", "v2.0.0", "v1.0.0"}
	if len(versions) != len(expected) {
		t.Fatalf("ListAvailableVersions() returned %d versions, want %d: %v", len(versions), len(expected), versions)
	}
	for i, v := range versions {
		if v != expected[i] {
			t.Errorf("versions[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

// TestListAvailableVersionsEmpty tests that an empty releases list returns an empty slice.
func TestListAvailableVersionsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("[]"))
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	versions, err := m.ListAvailableVersions()
	if err != nil {
		t.Fatalf("ListAvailableVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("ListAvailableVersions() = %d versions, want 0", len(versions))
	}
}

// TestListAvailableVersionsNetworkError tests ListAvailableVersions when the server fails.
func TestListAvailableVersionsNetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	_, err := m.ListAvailableVersions()
	if err == nil {
		t.Error("ListAvailableVersions() expected error for server 503, got nil")
	}
}

// TestVerifyChecksumSkippedForEmpty verifies that an empty checksumURL is a no-op.
func TestVerifyChecksumSkippedForEmpty(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "verify-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "archive.tar.gz")
	os.WriteFile(filePath, []byte("content"), 0644)

	m := &Manager{}
	if err := m.VerifyChecksum(filePath, ""); err != nil {
		t.Errorf("VerifyChecksum() with empty URL = %v, want nil", err)
	}
}

// TestVerifyChecksumSuccess tests the happy path: correct hash in checksum file.
func TestVerifyChecksumSuccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "verify-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := []byte("archive content for hash test")
	filePath := filepath.Join(tempDir, "archive.tar.gz")
	os.WriteFile(filePath, content, 0644)

	sum := sha256.Sum256(content)
	correctHash := hex.EncodeToString(sum[:])
	checksumBody := fmt.Sprintf("%s  archive.tar.gz\n", correctHash)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(checksumBody))
	}))
	defer server.Close()

	m := &Manager{}
	if err := m.VerifyChecksum(filePath, server.URL+"/checksums.txt"); err != nil {
		t.Errorf("VerifyChecksum() error = %v, want nil", err)
	}
}

// TestVerifyChecksumMismatch tests that a wrong hash returns an error and deletes the file.
func TestVerifyChecksumMismatch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "verify-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := []byte("real archive content")
	filePath := filepath.Join(tempDir, "archive.tar.gz")
	os.WriteFile(filePath, content, 0644)

	wrongHash := strings.Repeat("a", 64)
	checksumBody := fmt.Sprintf("%s  archive.tar.gz\n", wrongHash)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(checksumBody))
	}))
	defer server.Close()

	m := &Manager{}
	err = m.VerifyChecksum(filePath, server.URL+"/checksums.txt")
	if err == nil {
		t.Error("VerifyChecksum() expected error for hash mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("VerifyChecksum() error = %q, want it to mention checksum mismatch", err.Error())
	}
	if _, statErr := os.Stat(filePath); !os.IsNotExist(statErr) {
		t.Error("VerifyChecksum() should delete the file on mismatch")
	}
}

// TestVerifyChecksumFilenameNotInChecksumFile tests the error when filename is absent.
func TestVerifyChecksumFilenameNotInChecksumFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "verify-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "archive.tar.gz")
	os.WriteFile(filePath, []byte("content"), 0644)

	checksumBody := strings.Repeat("b", 64) + "  other_file.tar.gz\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(checksumBody))
	}))
	defer server.Close()

	m := &Manager{}
	err = m.VerifyChecksum(filePath, server.URL+"/checksums.txt")
	if err == nil {
		t.Error("VerifyChecksum() expected error when filename not in checksum file, got nil")
	}
	if !strings.Contains(err.Error(), "no checksum found") {
		t.Errorf("VerifyChecksum() error = %q, want it to mention no checksum found", err.Error())
	}
}

// TestVerifyChecksumServerNonOK tests the error when the checksum server returns non-200.
func TestVerifyChecksumServerNonOK(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "verify-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "archive.tar.gz")
	os.WriteFile(filePath, []byte("content"), 0644)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	m := &Manager{}
	err = m.VerifyChecksum(filePath, server.URL+"/checksums.txt")
	if err == nil {
		t.Error("VerifyChecksum() expected error for server 500, got nil")
	}
	if !strings.Contains(err.Error(), "checksums endpoint returned") {
		t.Errorf("VerifyChecksum() error = %q, want checksums endpoint returned", err.Error())
	}
}

// TestVerifyChecksumFetchFailed tests the error when the checksum URL is unreachable.
func TestVerifyChecksumFetchFailed(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "verify-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "archive.tar.gz")
	os.WriteFile(filePath, []byte("content"), 0644)

	m := &Manager{}
	err = m.VerifyChecksum(filePath, "http://127.0.0.1:1/checksums.txt")
	if err == nil {
		t.Error("VerifyChecksum() expected error for unreachable server, got nil")
	}
	if !strings.Contains(err.Error(), "failed to fetch checksums") {
		t.Errorf("VerifyChecksum() error = %q, want failed to fetch checksums", err.Error())
	}
}

// TestVerifyChecksumFileCannotOpen tests the error when the file to hash does not exist.
func TestVerifyChecksumFileCannotOpen(t *testing.T) {
	expectedHash := strings.Repeat("c", 64)
	checksumBody := fmt.Sprintf("%s  missing.tar.gz\n", expectedHash)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(checksumBody))
	}))
	defer server.Close()

	m := &Manager{}
	err := m.VerifyChecksum("/nonexistent/missing.tar.gz", server.URL+"/checksums.txt")
	if err == nil {
		t.Error("VerifyChecksum() expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open file") {
		t.Errorf("VerifyChecksum() error = %q, want failed to open file", err.Error())
	}
}

// TestVerifyChecksumWithCommentsAndBlanks tests that comment/blank lines in checksum files are skipped.
func TestVerifyChecksumWithCommentsAndBlanks(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "verify-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := []byte("file content for comments test")
	filePath := filepath.Join(tempDir, "myarchive.tar.gz")
	os.WriteFile(filePath, content, 0644)

	h := sha256.Sum256(content)
	realHash := hex.EncodeToString(h[:])

	checksumBody := fmt.Sprintf(
		"# SHA256 checksums generated by release workflow\n\n%s  other.tar.gz\n%s  myarchive.tar.gz\n",
		strings.Repeat("d", 64), realHash,
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(checksumBody))
	}))
	defer server.Close()

	m := &Manager{}
	if err := m.VerifyChecksum(filePath, server.URL+"/checksums.txt"); err != nil {
		t.Errorf("VerifyChecksum() with comment/blank lines error = %v, want nil", err)
	}
}

// TestFindChecksumURLCanonicalNames tests all recognised checksum filenames.
func TestFindChecksumURLCanonicalNames(t *testing.T) {
	tests := []struct {
		filename string
		wantURL  string
	}{
		{"checksums.txt", "https://example.com/checksums.txt"},
		{"sha256sums.txt", "https://example.com/sha256sums.txt"},
		{"SHA256SUMS", "https://example.com/SHA256SUMS"},
		{"checksums", "https://example.com/checksums"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			m := &Manager{}
			assets := []Asset{
				{Name: "search_linux_amd64.tar.gz", DownloadURL: "https://example.com/binary"},
				{Name: tt.filename, DownloadURL: tt.wantURL},
			}
			got := m.findChecksumURL(assets)
			if got != tt.wantURL {
				t.Errorf("findChecksumURL() for %q = %q, want %q", tt.filename, got, tt.wantURL)
			}
		})
	}
}

// TestFindChecksumURLCaseInsensitive verifies case-insensitive name matching.
func TestFindChecksumURLCaseInsensitive(t *testing.T) {
	m := &Manager{}
	assets := []Asset{
		{Name: "CHECKSUMS.TXT", DownloadURL: "https://example.com/CHECKSUMS.TXT"},
	}
	got := m.findChecksumURL(assets)
	if got != "https://example.com/CHECKSUMS.TXT" {
		t.Errorf("findChecksumURL() case-insensitive = %q, want https://example.com/CHECKSUMS.TXT", got)
	}
}

// TestFindChecksumURLNotFound tests the empty-string return when no checksum asset exists.
func TestFindChecksumURLNotFound(t *testing.T) {
	m := &Manager{}
	assets := []Asset{
		{Name: "search_linux_amd64.tar.gz", DownloadURL: "https://example.com/binary"},
		{Name: "README.md", DownloadURL: "https://example.com/readme"},
	}
	if got := m.findChecksumURL(assets); got != "" {
		t.Errorf("findChecksumURL() = %q, want empty string", got)
	}
}

// TestFindChecksumURLEmptyAssets tests that an empty asset list returns an empty string.
func TestFindChecksumURLEmptyAssets(t *testing.T) {
	m := &Manager{}
	if got := m.findChecksumURL([]Asset{}); got != "" {
		t.Errorf("findChecksumURL() for empty assets = %q, want empty string", got)
	}
}

// TestInstallUpdateWithChecksumSuccess tests a full install with valid checksum verification.
func TestInstallUpdateWithChecksumSuccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "install-checksum-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}

	currentBinary := filepath.Join(tempDir, binaryName)
	os.WriteFile(currentBinary, []byte("old binary"), 0755)

	archivePath := filepath.Join(tempDir, "update.tar.gz")
	newBinaryContent := []byte("new binary content for checksum install")
	makeTarGz(t, archivePath, binaryName, newBinaryContent)

	archiveHash := sha256HexFile(t, archivePath)
	checksumBody := fmt.Sprintf("%s  %s\n", archiveHash, filepath.Base(archivePath))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(checksumBody))
	}))
	defer server.Close()

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     currentBinary,
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	if err := m.InstallUpdate(archivePath, server.URL+"/checksums.txt"); err != nil {
		t.Fatalf("InstallUpdate() with valid checksum error = %v", err)
	}

	data, err := os.ReadFile(currentBinary)
	if err != nil {
		t.Fatalf("ReadFile after install: %v", err)
	}
	if string(data) != string(newBinaryContent) {
		t.Errorf("Binary after install = %q, want %q", string(data), string(newBinaryContent))
	}
}

// TestInstallUpdateWithChecksumMismatch verifies that a bad checksum aborts the install.
func TestInstallUpdateWithChecksumMismatch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "install-bad-checksum-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tempDir)

	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	currentBinary := filepath.Join(tempDir, binaryName)
	originalContent := []byte("original binary")
	os.WriteFile(currentBinary, originalContent, 0755)

	archivePath := filepath.Join(tempDir, "update.tar.gz")
	makeTarGz(t, archivePath, binaryName, []byte("new binary"))

	wrongHash := strings.Repeat("f", 64)
	checksumBody := fmt.Sprintf("%s  %s\n", wrongHash, filepath.Base(archivePath))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(checksumBody))
	}))
	defer server.Close()

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     currentBinary,
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	err = m.InstallUpdate(archivePath, server.URL+"/checksums.txt")
	if err == nil {
		t.Error("InstallUpdate() expected error for checksum mismatch, got nil")
	}
	if !strings.Contains(err.Error(), "checksum verification failed") {
		t.Errorf("InstallUpdate() error = %q, want checksum verification failed", err.Error())
	}

	data, _ := os.ReadFile(currentBinary)
	if string(data) != string(originalContent) {
		t.Errorf("Binary should be unchanged after bad checksum, got %q", string(data))
	}
}

// TestCheckForUpdatesSelectsLatestByDate tests that when multiple releases exist
// the most recently published one wins.
func TestCheckForUpdatesSelectsLatestByDate(t *testing.T) {
	now := time.Now().UTC()
	releases := []Release{
		{
			TagName:     "v1.1.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now.Add(-2 * time.Hour),
			Assets:      []Asset{},
		},
		{
			TagName:     "v1.3.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now.Add(-time.Hour),
			Assets:      []Asset{},
		},
		{
			TagName:     "v1.2.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now.Add(-3 * time.Hour),
			Assets:      []Asset{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	restore := patchDefaultTransport(t, server)
	defer restore()

	m := &Manager{currentVersion: "1.0.0", tempDir: os.TempDir()}
	info, err := m.CheckForUpdates(false)
	if err != nil {
		t.Fatalf("CheckForUpdates() error = %v", err)
	}
	if info.LatestVersion != "v1.3.0" {
		t.Errorf("LatestVersion = %q, want v1.3.0 (most recently published)", info.LatestVersion)
	}
}
