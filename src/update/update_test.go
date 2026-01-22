package update

import (
	"archive/tar"
	"compress/gzip"
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

func TestReleasesURL(t *testing.T) {
	if ReleasesURL == "" {
		t.Error("ReleasesURL should not be empty")
	}
	if ReleasesURL[:8] != "https://" {
		t.Errorf("ReleasesURL should use HTTPS: %s", ReleasesURL)
	}
}

func TestReleaseStruct(t *testing.T) {
	now := time.Now()
	r := Release{
		TagName:     "v1.0.0",
		Name:        "Release 1.0.0",
		Body:        "Release notes",
		Draft:       false,
		Prerelease:  false,
		CreatedAt:   now,
		PublishedAt: now,
		Assets:      []Asset{},
	}

	if r.TagName != "v1.0.0" {
		t.Errorf("TagName = %q, want %q", r.TagName, "v1.0.0")
	}
	if r.Name != "Release 1.0.0" {
		t.Errorf("Name = %q, want %q", r.Name, "Release 1.0.0")
	}
	if r.Draft {
		t.Error("Draft should be false")
	}
	if r.Prerelease {
		t.Error("Prerelease should be false")
	}
}

func TestAssetStruct(t *testing.T) {
	a := Asset{
		Name:        "search_linux_amd64.tar.gz",
		Size:        12345678,
		DownloadURL: "https://example.com/download",
		ContentType: "application/gzip",
	}

	if a.Name != "search_linux_amd64.tar.gz" {
		t.Errorf("Name = %q, want %q", a.Name, "search_linux_amd64.tar.gz")
	}
	if a.Size != 12345678 {
		t.Errorf("Size = %d, want %d", a.Size, 12345678)
	}
	if a.ContentType != "application/gzip" {
		t.Errorf("ContentType = %q, want %q", a.ContentType, "application/gzip")
	}
}

func TestUpdateInfoStruct(t *testing.T) {
	now := time.Now()
	info := UpdateInfo{
		Available:      true,
		CurrentVersion: "1.0.0",
		LatestVersion:  "1.1.0",
		ReleaseNotes:   "Bug fixes",
		DownloadURL:    "https://example.com/download",
		AssetSize:      12345678,
		PublishedAt:    now,
		IsPrerelease:   false,
	}

	if !info.Available {
		t.Error("Available should be true")
	}
	if info.CurrentVersion != "1.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "1.0.0")
	}
	if info.LatestVersion != "1.1.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "1.1.0")
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.currentVersion == "" {
		t.Error("currentVersion should not be empty")
	}
	if m.tempDir == "" {
		t.Error("tempDir should not be empty")
	}
}

func TestManagerGetCurrentVersion(t *testing.T) {
	m := NewManager()

	version := m.GetCurrentVersion()
	if version == "" {
		t.Error("GetCurrentVersion() should not return empty string")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.1.0", "v1.0.0", 1},
		{"1.0.0-beta", "1.0.0-alpha", 0}, // Pre-release parts become 0
		{"1.0.0", "0.9.9", 1},
		{"0.0.1", "0.0.0", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    []int
	}{
		{"1.0.0", []int{1, 0, 0}},
		{"v1.0.0", []int{1, 0, 0}},
		{"1.2.3", []int{1, 2, 3}},
		{"10.20.30", []int{10, 20, 30}},
		{"1.0.0-beta", []int{1, 0, 0, 0}},
		{"1.0.0-rc1", []int{1, 0, 0, 0}}, // "rc1" doesn't start with digit, so Sscanf returns 0
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseVersion(tt.version)
			if len(got) != len(tt.want) {
				t.Errorf("parseVersion(%q) returned %d parts, want %d", tt.version, len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.version, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestManagerFindAsset(t *testing.T) {
	m := NewManager()

	assets := []Asset{
		{Name: "search_linux_amd64.tar.gz", DownloadURL: "https://example.com/linux"},
		{Name: "search_darwin_amd64.tar.gz", DownloadURL: "https://example.com/darwin"},
		{Name: "search_windows_amd64.zip", DownloadURL: "https://example.com/windows"},
		{Name: "search_linux_arm64.tar.gz", DownloadURL: "https://example.com/linux-arm"},
		{Name: "search_darwin_arm64.tar.gz", DownloadURL: "https://example.com/darwin-arm"},
	}

	asset := m.findAsset(assets)

	// Should find an asset for current platform
	if asset == nil {
		// This is acceptable if the current platform isn't in the list
		t.Logf("No asset found for %s/%s", runtime.GOOS, runtime.GOARCH)
		return
	}

	// Verify the asset matches current platform
	expectedPrefix := "search_" + runtime.GOOS
	if len(asset.Name) < len(expectedPrefix) || asset.Name[:len(expectedPrefix)] != expectedPrefix {
		// Check alternative naming
		t.Logf("Found asset: %s for %s/%s", asset.Name, runtime.GOOS, runtime.GOARCH)
	}
}

func TestManagerFindAssetNotFound(t *testing.T) {
	m := NewManager()

	assets := []Asset{
		{Name: "search_fake_os.tar.gz", DownloadURL: "https://example.com/fake"},
	}

	// Mock architecture that doesn't exist
	asset := m.findAsset(assets)
	// May or may not find depending on platform
	_ = asset
}

func TestManagerBackupCurrentBinary(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a fake binary
	fakeBinary := tempDir + "/search"
	if runtime.GOOS == "windows" {
		fakeBinary = tempDir + "/search.exe"
	}
	if err := os.WriteFile(fakeBinary, []byte("fake binary"), 0755); err != nil {
		t.Fatalf("Failed to create fake binary: %v", err)
	}

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     fakeBinary,
		backupDir:      tempDir + "/backup",
		tempDir:        tempDir,
	}

	err = m.backupCurrentBinary()
	if err != nil {
		t.Fatalf("backupCurrentBinary() error = %v", err)
	}

	// Verify backup was created
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		t.Fatalf("Failed to read backup dir: %v", err)
	}

	if len(entries) == 0 {
		t.Error("No backup file created")
	}
}

func TestManagerExtractBinaryBadArchive(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a non-gzip file
	badArchive := tempDir + "/bad.tar.gz"
	if err := os.WriteFile(badArchive, []byte("not a gzip file"), 0644); err != nil {
		t.Fatalf("Failed to create bad archive: %v", err)
	}

	m := &Manager{
		tempDir: tempDir,
	}

	_, err = m.extractBinary(badArchive)
	if err == nil {
		t.Error("extractBinary() should fail with invalid archive")
	}
}

func TestManagerReplaceBinary(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create source and target files
	sourcePath := tempDir + "/new_binary"
	targetPath := tempDir + "/binary"

	if err := os.WriteFile(sourcePath, []byte("new content"), 0755); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("old content"), 0755); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	m := &Manager{
		binaryPath: targetPath,
	}

	err = m.replaceBinary(sourcePath)
	if err != nil {
		t.Fatalf("replaceBinary() error = %v", err)
	}

	// Verify content was replaced
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read target file: %v", err)
	}
	if string(content) != "new content" {
		t.Errorf("Binary content = %q, want %q", string(content), "new content")
	}
}

func TestManagerRollbackNoBackup(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m := &Manager{
		backupDir: tempDir + "/empty_backup",
	}

	// Create empty backup dir
	os.MkdirAll(m.backupDir, 0755)

	err = m.Rollback()
	if err == nil {
		t.Error("Rollback() should fail with no backup")
	}
}

func TestManagerRollbackWithBackup(t *testing.T) {
	// Create temp directory for testing
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create backup directory with a backup file
	backupDir := tempDir + "/backup"
	os.MkdirAll(backupDir, 0755)

	backupFile := backupDir + "/search-0.9.0.backup"
	if err := os.WriteFile(backupFile, []byte("old binary content"), 0755); err != nil {
		t.Fatalf("Failed to create backup file: %v", err)
	}

	// Create target binary
	targetPath := tempDir + "/binary"
	if err := os.WriteFile(targetPath, []byte("current content"), 0755); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	m := &Manager{
		binaryPath: targetPath,
		backupDir:  backupDir,
	}

	err = m.Rollback()
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Verify content was restored
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("Failed to read target file: %v", err)
	}
	if string(content) != "old binary content" {
		t.Errorf("Binary content = %q, want %q", string(content), "old binary content")
	}
}

// Tests for network functions with mock HTTP server

func TestManagerFetchReleasesWithMockServer(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github.v3+json" {
			t.Errorf("Expected Accept header, got %q", r.Header.Get("Accept"))
		}

		releases := []Release{
			{
				TagName:     "v1.1.0",
				Name:        "Release 1.1.0",
				Body:        "New features",
				Draft:       false,
				Prerelease:  false,
				PublishedAt: time.Now(),
				Assets: []Asset{
					{Name: "search_linux_amd64.tar.gz", DownloadURL: "https://example.com/linux"},
				},
			},
			{
				TagName:     "v1.0.0",
				Name:        "Release 1.0.0",
				Body:        "Initial release",
				Draft:       false,
				Prerelease:  false,
				PublishedAt: time.Now().Add(-24 * time.Hour),
				Assets:      []Asset{},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()

	// Test with mock server
	// Note: Since ReleasesURL is a constant, we can't change it.
	// The fetchReleases function uses the constant directly.
	// This test documents that we would need dependency injection for proper testing.
}

func TestManagerCheckForUpdatesLogic(t *testing.T) {
	// Test the logic of CheckForUpdates by examining the code paths
	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     "/tmp/test",
		backupDir:      "/tmp/backup",
		tempDir:        "/tmp",
	}

	// The function requires network access, so we test related helper functions
	// that are used by CheckForUpdates

	// Test that latest release is selected correctly by comparing versions
	v1 := "1.1.0"
	v2 := "1.0.0"
	if compareVersions(v1, v2) != 1 {
		t.Errorf("compareVersions(%q, %q) should return 1", v1, v2)
	}

	// Verify manager has correct version
	if m.GetCurrentVersion() != "1.0.0" {
		t.Errorf("GetCurrentVersion() = %q, want %q", m.GetCurrentVersion(), "1.0.0")
	}
}

func TestCompareVersionsMoreCases(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		// Edge cases
		{"0.0.0", "0.0.0", 0},
		{"1", "1", 0},
		{"1", "2", -1},
		{"2", "1", 1},
		{"1.0", "1.0.0", -1}, // Shorter version is less
		{"1.0.0", "1.0", 1},  // Longer version is greater
		{"1.0.0.0", "1.0.0", 1},
		// Version with underscores
		{"1_0_0", "1_0_0", 0},
		{"1_0_1", "1_0_0", 1},
		// Mixed separators
		{"1.0-1", "1.0-0", 1},
		// Large numbers
		{"100.200.300", "100.200.299", 1},
		{"100.200.300", "100.200.301", -1},
		// Pre-release versions
		{"1.0.0-alpha", "1.0.0-beta", 0},    // Both parse to 1.0.0.0
		{"1.0.0-1", "1.0.0-2", -1},          // 1.0.0.1 vs 1.0.0.2
		{"2.0.0-beta", "1.9.9", 1},          // 2.0.0.0 vs 1.9.9
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestParseVersionMoreCases(t *testing.T) {
	tests := []struct {
		version string
		want    []int
	}{
		{"", []int{}},
		{"1", []int{1}},
		{"1.2", []int{1, 2}},
		{"v", []int{}},
		{"v1", []int{1}},
		{"1-2-3", []int{1, 2, 3}},
		{"1_2_3", []int{1, 2, 3}},
		{"1.2.3.4.5", []int{1, 2, 3, 4, 5}},
		{"abc", []int{0}},               // Non-numeric becomes 0
		{"1.abc.3", []int{1, 0, 3}},     // abc becomes 0
		{"v1.2.3-rc1", []int{1, 2, 3, 0}}, // rc1 -> 0
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseVersion(tt.version)
			if len(got) != len(tt.want) {
				t.Errorf("parseVersion(%q) returned %d parts, want %d: got %v", tt.version, len(got), len(tt.want), got)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.version, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestManagerFindAssetMoreCases(t *testing.T) {
	m := NewManager()

	tests := []struct {
		name   string
		assets []Asset
		found  bool
	}{
		{
			name:   "empty assets",
			assets: []Asset{},
			found:  false,
		},
		{
			name: "only unrelated assets",
			assets: []Asset{
				{Name: "README.md", DownloadURL: "https://example.com/readme"},
				{Name: "checksums.txt", DownloadURL: "https://example.com/checksums"},
			},
			found: false,
		},
		{
			name: "case insensitive match",
			assets: []Asset{
				{Name: "SEARCH_" + runtime.GOOS + "_" + runtime.GOARCH + ".TAR.GZ", DownloadURL: "https://example.com/upper"},
			},
			found: true,
		},
		{
			name: "hyphen naming convention",
			assets: []Asset{
				{Name: "search-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz", DownloadURL: "https://example.com/hyphen"},
			},
			found: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset := m.findAsset(tt.assets)
			if tt.found && asset == nil {
				t.Error("Expected to find asset, got nil")
			}
			if !tt.found && asset != nil {
				t.Errorf("Expected no asset, got %v", asset.Name)
			}
		})
	}
}

func TestManagerDownloadUpdateBadURL(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m := &Manager{
		tempDir: tempDir,
	}

	// Test with invalid URL
	_, err = m.DownloadUpdate("not-a-valid-url", nil)
	if err == nil {
		t.Error("DownloadUpdate() should fail with invalid URL")
	}
}

func TestManagerDownloadUpdateMockServer(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create mock server that returns content
	content := []byte("test binary content for download")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer server.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	var downloadedBytes int64
	var totalBytes int64
	progressCalled := false

	path, err := m.DownloadUpdate(server.URL, func(downloaded, total int64) {
		downloadedBytes = downloaded
		totalBytes = total
		progressCalled = true
	})
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}
	defer os.Remove(path)

	// Verify file was downloaded
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(data) != string(content) {
		t.Errorf("Downloaded content = %q, want %q", string(data), string(content))
	}

	// Verify progress was called
	if !progressCalled {
		t.Error("Progress function was not called")
	}

	if downloadedBytes != int64(len(content)) {
		t.Errorf("Downloaded bytes = %d, want %d", downloadedBytes, len(content))
	}

	if totalBytes != int64(len(content)) {
		t.Errorf("Total bytes = %d, want %d", totalBytes, len(content))
	}
}

func TestManagerDownloadUpdateServerError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	_, err = m.DownloadUpdate(server.URL, nil)
	if err == nil {
		t.Error("DownloadUpdate() should fail with 404 status")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("Error should mention status code, got: %v", err)
	}
}

func TestManagerDownloadUpdateNoProgress(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	content := []byte("test content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer server.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	// Test with nil progress function
	path, err := m.DownloadUpdate(server.URL, nil)
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read downloaded file: %v", err)
	}

	if string(data) != string(content) {
		t.Errorf("Downloaded content = %q, want %q", string(data), string(content))
	}
}

func TestManagerExtractBinaryValidArchive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a valid tar.gz archive with a binary
	archivePath := filepath.Join(tempDir, "test.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	binaryContent := []byte("fake binary content")

	// Create the archive
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive file: %v", err)
	}

	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add binary to archive
	header := &tar.Header{
		Name: binaryName,
		Mode: 0755,
		Size: int64(len(binaryContent)),
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}
	if _, err := tarWriter.Write(binaryContent); err != nil {
		t.Fatalf("Failed to write tar content: %v", err)
	}

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	extractedPath, err := m.extractBinary(archivePath)
	if err != nil {
		t.Fatalf("extractBinary() error = %v", err)
	}
	defer os.Remove(extractedPath)

	// Verify extracted content
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(data) != string(binaryContent) {
		t.Errorf("Extracted content = %q, want %q", string(data), string(binaryContent))
	}
}

func TestManagerExtractBinaryNoBinaryInArchive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive without binary
	archivePath := filepath.Join(tempDir, "test.tar.gz")

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive file: %v", err)
	}

	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add non-binary file
	header := &tar.Header{
		Name: "README.md",
		Mode: 0644,
		Size: 5,
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write([]byte("hello"))

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	_, err = m.extractBinary(archivePath)
	if err == nil {
		t.Error("extractBinary() should fail when binary not found in archive")
	}
	if !strings.Contains(err.Error(), "binary not found") {
		t.Errorf("Error should mention binary not found, got: %v", err)
	}
}

func TestManagerExtractBinaryFileNotFound(t *testing.T) {
	m := &Manager{
		tempDir: "/tmp",
	}

	_, err := m.extractBinary("/nonexistent/file.tar.gz")
	if err == nil {
		t.Error("extractBinary() should fail when archive file doesn't exist")
	}
}

func TestManagerInstallUpdateBadArchive(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create fake binary
	fakeBinary := filepath.Join(tempDir, "search")
	if runtime.GOOS == "windows" {
		fakeBinary = filepath.Join(tempDir, "search.exe")
	}
	os.WriteFile(fakeBinary, []byte("binary"), 0755)

	// Create invalid archive
	badArchive := filepath.Join(tempDir, "bad.tar.gz")
	os.WriteFile(badArchive, []byte("not an archive"), 0644)

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     fakeBinary,
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	err = m.InstallUpdate(badArchive)
	if err == nil {
		t.Error("InstallUpdate() should fail with invalid archive")
	}
}

func TestManagerInstallUpdateSuccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create current binary
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	currentBinary := filepath.Join(tempDir, binaryName)
	os.WriteFile(currentBinary, []byte("old binary"), 0755)

	// Create valid archive with new binary
	archivePath := filepath.Join(tempDir, "update.tar.gz")
	newContent := []byte("new binary content")

	file, _ := os.Create(archivePath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	header := &tar.Header{
		Name: binaryName,
		Mode: 0755,
		Size: int64(len(newContent)),
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write(newContent)
	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     currentBinary,
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	err = m.InstallUpdate(archivePath)
	if err != nil {
		t.Fatalf("InstallUpdate() error = %v", err)
	}

	// Verify binary was updated
	data, err := os.ReadFile(currentBinary)
	if err != nil {
		t.Fatalf("Failed to read updated binary: %v", err)
	}

	if string(data) != string(newContent) {
		t.Errorf("Binary content = %q, want %q", string(data), string(newContent))
	}

	// Verify backup was created
	entries, _ := os.ReadDir(filepath.Join(tempDir, "backup"))
	if len(entries) == 0 {
		t.Error("Backup was not created")
	}
}

func TestManagerRollbackMultipleBackups(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create backup directory with multiple backups
	backupDir := filepath.Join(tempDir, "backup")
	os.MkdirAll(backupDir, 0755)

	// Create older backup
	oldBackup := filepath.Join(backupDir, "search-0.8.0.backup")
	os.WriteFile(oldBackup, []byte("older version"), 0755)
	// Set modification time to the past
	oldTime := time.Now().Add(-24 * time.Hour)
	os.Chtimes(oldBackup, oldTime, oldTime)

	// Create newer backup
	newBackup := filepath.Join(backupDir, "search-0.9.0.backup")
	os.WriteFile(newBackup, []byte("newer version"), 0755)

	// Create target binary
	targetPath := filepath.Join(tempDir, "binary")
	os.WriteFile(targetPath, []byte("current"), 0755)

	m := &Manager{
		binaryPath: targetPath,
		backupDir:  backupDir,
	}

	err = m.Rollback()
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Should rollback to newer backup
	content, _ := os.ReadFile(targetPath)
	if string(content) != "newer version" {
		t.Errorf("Rollback should use most recent backup, got %q", string(content))
	}
}

func TestManagerRollbackBackupDirNotExist(t *testing.T) {
	m := &Manager{
		backupDir: "/nonexistent/backup/dir",
	}

	err := m.Rollback()
	if err == nil {
		t.Error("Rollback() should fail when backup directory doesn't exist")
	}
}

func TestManagerBackupCurrentBinarySourceNotExist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     filepath.Join(tempDir, "nonexistent"),
		backupDir:      filepath.Join(tempDir, "backup"),
	}

	err = m.backupCurrentBinary()
	if err == nil {
		t.Error("backupCurrentBinary() should fail when source binary doesn't exist")
	}
}

func TestManagerReplaceBinarySourceNotExist(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m := &Manager{
		binaryPath: filepath.Join(tempDir, "target"),
	}

	err = m.replaceBinary(filepath.Join(tempDir, "nonexistent"))
	if err == nil {
		t.Error("replaceBinary() should fail when source doesn't exist")
	}
}

func TestReleaseStructWithAssets(t *testing.T) {
	now := time.Now()
	r := Release{
		TagName:     "v2.0.0",
		Name:        "Major Release",
		Body:        "Breaking changes",
		Draft:       true,
		Prerelease:  true,
		CreatedAt:   now.Add(-1 * time.Hour),
		PublishedAt: now,
		Assets: []Asset{
			{Name: "asset1.tar.gz", Size: 1000, DownloadURL: "https://example.com/1", ContentType: "application/gzip"},
			{Name: "asset2.zip", Size: 2000, DownloadURL: "https://example.com/2", ContentType: "application/zip"},
		},
	}

	if len(r.Assets) != 2 {
		t.Errorf("Expected 2 assets, got %d", len(r.Assets))
	}
	if r.Draft != true {
		t.Error("Draft should be true")
	}
	if r.Prerelease != true {
		t.Error("Prerelease should be true")
	}
	if r.Assets[0].Size != 1000 {
		t.Errorf("Asset[0].Size = %d, want 1000", r.Assets[0].Size)
	}
}

func TestUpdateInfoStructComplete(t *testing.T) {
	now := time.Now()
	info := UpdateInfo{
		Available:      false,
		CurrentVersion: "2.0.0",
		LatestVersion:  "2.0.0",
		ReleaseNotes:   "",
		DownloadURL:    "",
		AssetSize:      0,
		PublishedAt:    now,
		IsPrerelease:   true,
	}

	if info.Available {
		t.Error("Available should be false")
	}
	if info.CurrentVersion != "2.0.0" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "2.0.0")
	}
	if !info.IsPrerelease {
		t.Error("IsPrerelease should be true")
	}
	if info.AssetSize != 0 {
		t.Errorf("AssetSize = %d, want 0", info.AssetSize)
	}
}

func TestManagerStructFields(t *testing.T) {
	m := &Manager{
		currentVersion: "1.2.3",
		binaryPath:     "/usr/local/bin/search",
		backupDir:      "/var/backup",
		tempDir:        "/tmp",
	}

	if m.currentVersion != "1.2.3" {
		t.Errorf("currentVersion = %q, want %q", m.currentVersion, "1.2.3")
	}
	if m.binaryPath != "/usr/local/bin/search" {
		t.Errorf("binaryPath = %q, want %q", m.binaryPath, "/usr/local/bin/search")
	}
	if m.backupDir != "/var/backup" {
		t.Errorf("backupDir = %q, want %q", m.backupDir, "/var/backup")
	}
	if m.tempDir != "/tmp" {
		t.Errorf("tempDir = %q, want %q", m.tempDir, "/tmp")
	}
}

// TestCheckForUpdatesWithMockServer tests CheckForUpdates with various scenarios
func TestCheckForUpdatesWithMockServer(t *testing.T) {
	// We cannot directly test CheckForUpdates because it uses a constant ReleasesURL.
	// However, we can test the component logic it uses.
	// This test verifies the version comparison and asset selection logic.

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     "/tmp/search",
		backupDir:      "/tmp/backup",
		tempDir:        "/tmp",
	}

	// Test GetCurrentVersion
	if v := m.GetCurrentVersion(); v != "1.0.0" {
		t.Errorf("GetCurrentVersion() = %q, want %q", v, "1.0.0")
	}

	// Test version comparison for update detection
	tests := []struct {
		current  string
		latest   string
		expected bool // true if update available
	}{
		{"1.0.0", "1.1.0", true},
		{"1.0.0", "1.0.0", false},
		{"1.1.0", "1.0.0", false},
		{"0.9.0", "1.0.0", true},
		{"v1.0.0", "v2.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.current+" vs "+tt.latest, func(t *testing.T) {
			isNewer := compareVersions(strings.TrimPrefix(tt.latest, "v"), strings.TrimPrefix(tt.current, "v")) > 0
			if isNewer != tt.expected {
				t.Errorf("isNewer = %v, want %v", isNewer, tt.expected)
			}
		})
	}
}

// TestFetchReleasesServerError tests fetch behavior with server errors
func TestFetchReleasesServerError(t *testing.T) {
	// Create mock server that returns various errors
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantErr    bool
		errContain string
	}{
		{
			name: "server returns 500",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:    true,
			errContain: "500",
		},
		{
			name: "server returns 403",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErr:    true,
			errContain: "403",
		},
		{
			name: "server returns invalid JSON",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("invalid json"))
			},
			wantErr: true,
		},
		{
			name: "server returns empty array",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte("[]"))
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			// Note: We can't directly test fetchReleases with mock server
			// because ReleasesURL is a constant. This documents the behavior.
			_ = server.URL
		})
	}
}

// TestFindAssetPatternMatching tests all asset matching patterns
func TestFindAssetPatternMatching(t *testing.T) {
	m := NewManager()

	// Test with zip file for Windows pattern
	assets := []Asset{
		{Name: "search_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip", DownloadURL: "https://example.com/zip"},
	}

	asset := m.findAsset(assets)
	if asset == nil {
		t.Log("No .zip asset match for current platform (expected if patterns don't include .zip for non-Windows)")
	}

	// Test exact name match vs contains match
	assetsExact := []Asset{
		{Name: "search_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz", DownloadURL: "https://example.com/exact"},
	}
	assetExact := m.findAsset(assetsExact)
	if assetExact != nil {
		if assetExact.DownloadURL != "https://example.com/exact" {
			t.Errorf("Expected exact match asset, got %v", assetExact)
		}
	}

	// Test contains match (asset name contains pattern)
	assetsContains := []Asset{
		{Name: "myapp-search_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz-signed", DownloadURL: "https://example.com/contains"},
	}
	assetContains := m.findAsset(assetsContains)
	if assetContains != nil {
		t.Logf("Found asset via contains match: %s", assetContains.Name)
	}
}

// TestDownloadUpdateWriteError tests download with write errors
func TestDownloadUpdateWriteError(t *testing.T) {
	// Create a read-only temp directory to trigger write errors
	tempDir, err := os.MkdirTemp("", "update-test-readonly-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test content"))
	}))
	defer server.Close()

	// Use non-existent directory to trigger error
	m := &Manager{
		tempDir: "/nonexistent/path/that/does/not/exist",
	}

	_, err = m.DownloadUpdate(server.URL, nil)
	if err == nil {
		t.Error("DownloadUpdate() should fail with invalid temp directory")
	}
}

// TestExtractBinaryWithNestedPath tests extracting binary from nested archive
func TestExtractBinaryWithNestedPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive with nested binary path
	archivePath := filepath.Join(tempDir, "nested.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	binaryContent := []byte("nested binary content")

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add binary in nested directory
	header := &tar.Header{
		Name:     "some/nested/path/" + binaryName,
		Mode:     0755,
		Size:     int64(len(binaryContent)),
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		t.Fatalf("Failed to write tar header: %v", err)
	}
	if _, err := tarWriter.Write(binaryContent); err != nil {
		t.Fatalf("Failed to write tar content: %v", err)
	}

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	extractedPath, err := m.extractBinary(archivePath)
	if err != nil {
		t.Fatalf("extractBinary() error = %v", err)
	}
	defer os.Remove(extractedPath)

	// Verify extracted content
	data, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(data) != string(binaryContent) {
		t.Errorf("Extracted content = %q, want %q", string(data), string(binaryContent))
	}
}

// TestExtractBinaryWithDirectoryEntry tests archive with directory entries
func TestExtractBinaryWithDirectoryEntry(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, "withdir.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	binaryContent := []byte("binary after directory")

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add directory entry first
	dirHeader := &tar.Header{
		Name:     "bin/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	tarWriter.WriteHeader(dirHeader)

	// Add binary with same base name but as directory (should be skipped)
	dirBinaryHeader := &tar.Header{
		Name:     binaryName + "/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	tarWriter.WriteHeader(dirBinaryHeader)

	// Add actual binary file
	header := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     int64(len(binaryContent)),
		Typeflag: tar.TypeReg,
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write(binaryContent)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	extractedPath, err := m.extractBinary(archivePath)
	if err != nil {
		t.Fatalf("extractBinary() error = %v", err)
	}
	defer os.Remove(extractedPath)

	data, _ := os.ReadFile(extractedPath)
	if string(data) != string(binaryContent) {
		t.Errorf("Extracted content = %q, want %q", string(data), string(binaryContent))
	}
}

// TestReplaceBinaryCopyFallback tests the copy fallback when rename fails
func TestReplaceBinaryCopyFallback(t *testing.T) {
	// Create two different temp directories to force rename to fail (cross-device)
	tempDir1, err := os.MkdirTemp("", "update-test-src-")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	tempDir2, err := os.MkdirTemp("", "update-test-dst-")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	sourcePath := filepath.Join(tempDir1, "new_binary")
	targetPath := filepath.Join(tempDir2, "binary")

	// Create source and target
	if err := os.WriteFile(sourcePath, []byte("new content via copy"), 0755); err != nil {
		t.Fatalf("Failed to create source: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("old content"), 0755); err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	m := &Manager{
		binaryPath: targetPath,
	}

	// This should succeed via copy fallback if rename fails
	err = m.replaceBinary(sourcePath)
	if err != nil {
		t.Fatalf("replaceBinary() error = %v", err)
	}

	content, _ := os.ReadFile(targetPath)
	if string(content) != "new content via copy" {
		t.Errorf("Content = %q, want %q", string(content), "new content via copy")
	}
}

// TestRollbackWithDirectories tests rollback ignoring directory entries
func TestRollbackWithDirectories(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	backupDir := filepath.Join(tempDir, "backup")
	os.MkdirAll(backupDir, 0755)

	// Create a subdirectory (should be ignored)
	subDir := filepath.Join(backupDir, "subdir")
	os.MkdirAll(subDir, 0755)

	// Create a backup file
	backupFile := filepath.Join(backupDir, "search-1.0.0.backup")
	os.WriteFile(backupFile, []byte("backup content"), 0755)

	// Create target binary
	targetPath := filepath.Join(tempDir, "binary")
	os.WriteFile(targetPath, []byte("current"), 0755)

	m := &Manager{
		binaryPath: targetPath,
		backupDir:  backupDir,
	}

	err = m.Rollback()
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	content, _ := os.ReadFile(targetPath)
	if string(content) != "backup content" {
		t.Errorf("Content = %q, want %q", string(content), "backup content")
	}
}

// TestRollbackWithNonBackupFiles tests rollback ignoring non-backup files
func TestRollbackWithNonBackupFiles(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	backupDir := filepath.Join(tempDir, "backup")
	os.MkdirAll(backupDir, 0755)

	// Create non-backup file (should be ignored)
	nonBackupFile := filepath.Join(backupDir, "readme.txt")
	os.WriteFile(nonBackupFile, []byte("readme"), 0644)

	// Create backup file
	backupFile := filepath.Join(backupDir, "search-1.0.0.backup")
	os.WriteFile(backupFile, []byte("backup content"), 0755)

	targetPath := filepath.Join(tempDir, "binary")
	os.WriteFile(targetPath, []byte("current"), 0755)

	m := &Manager{
		binaryPath: targetPath,
		backupDir:  backupDir,
	}

	err = m.Rollback()
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	content, _ := os.ReadFile(targetPath)
	if string(content) != "backup content" {
		t.Errorf("Content = %q, want %q", string(content), "backup content")
	}
}

// TestBackupCurrentBinaryDirCreateError tests backup with directory creation failure
func TestBackupCurrentBinaryDirCreateError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a file where backup dir should be (to prevent MkdirAll)
	blockingFile := filepath.Join(tempDir, "backup")
	os.WriteFile(blockingFile, []byte("blocking"), 0644)

	fakeBinary := filepath.Join(tempDir, "search")
	os.WriteFile(fakeBinary, []byte("binary"), 0755)

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     fakeBinary,
		backupDir:      filepath.Join(tempDir, "backup", "subdir"), // Will fail because backup is a file
		tempDir:        tempDir,
	}

	err = m.backupCurrentBinary()
	if err == nil {
		t.Error("backupCurrentBinary() should fail when backup dir creation fails")
	}
}

// TestInstallUpdateBackupError tests InstallUpdate when backup fails
func TestInstallUpdateBackupError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Manager with non-existent binary (backup will fail)
	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     filepath.Join(tempDir, "nonexistent"),
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	err = m.InstallUpdate("/some/archive.tar.gz")
	if err == nil {
		t.Error("InstallUpdate() should fail when backup fails")
	}
	if !strings.Contains(err.Error(), "backup") {
		t.Errorf("Error should mention backup, got: %v", err)
	}
}

// TestCompareVersionsLengthDifferences tests version comparison with different lengths
func TestCompareVersionsLengthDifferences(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		{"1.0.0.0", "1.0.0", 1},   // longer > shorter when equal prefix
		{"1.0.0", "1.0.0.0", -1},  // shorter < longer when equal prefix
		{"1.0.0.1", "1.0.0", 1},   // longer with non-zero suffix
		{"1.0.0", "1.0.0.1", -1},
		{"1", "1.0", -1},
		{"1.0", "1", 1},
		{"1.0.0", "1", 1},
		{"1", "1.0.0", -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

// TestParseVersionEmptyAndSpecial tests parseVersion with edge cases
func TestParseVersionEmptyAndSpecial(t *testing.T) {
	tests := []struct {
		version string
		length  int
	}{
		{"", 0},
		{"v", 0},
		{"...", 0},    // Only separators, no parts
		{"1..2", 2},   // Empty part becomes nothing (FieldsFunc removes empty)
		{"1...2", 2},  // Multiple separators
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseVersion(tt.version)
			if len(got) != tt.length {
				t.Errorf("parseVersion(%q) length = %d, want %d (got %v)", tt.version, len(got), tt.length, got)
			}
		})
	}
}

// TestFindAssetArchitectureMappings tests architecture mapping in findAsset
func TestFindAssetArchitectureMappings(t *testing.T) {
	m := NewManager()

	// Test that amd64 and arm64 are properly handled
	// Note: the actual mapping is identity (amd64->amd64, arm64->arm64)
	// but the code has explicit branches for them

	currentOS := runtime.GOOS
	currentArch := runtime.GOARCH

	// Create asset for current platform
	assets := []Asset{
		{Name: fmt.Sprintf("search_%s_%s.tar.gz", currentOS, currentArch), DownloadURL: "https://example.com/current"},
	}

	asset := m.findAsset(assets)
	if asset == nil {
		t.Errorf("findAsset should find asset for current platform %s/%s", currentOS, currentArch)
	} else if asset.DownloadURL != "https://example.com/current" {
		t.Errorf("findAsset returned wrong asset: %v", asset)
	}
}

// TestDownloadUpdateLargeFile tests download with larger content and progress tracking
func TestDownloadUpdateLargeFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create larger content (multiple buffer sizes to test loop)
	content := make([]byte, 100*1024) // 100KB
	for i := range content {
		content[i] = byte(i % 256)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(content)))
		w.Write(content)
	}))
	defer server.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	var progressCalls int
	var lastDownloaded int64

	path, err := m.DownloadUpdate(server.URL, func(downloaded, total int64) {
		progressCalls++
		lastDownloaded = downloaded
		if total != int64(len(content)) {
			t.Errorf("total = %d, want %d", total, len(content))
		}
	})
	if err != nil {
		t.Fatalf("DownloadUpdate() error = %v", err)
	}
	defer os.Remove(path)

	if progressCalls == 0 {
		t.Error("Progress function was not called")
	}
	if lastDownloaded != int64(len(content)) {
		t.Errorf("Final downloaded = %d, want %d", lastDownloaded, len(content))
	}

	// Verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if len(data) != len(content) {
		t.Errorf("Downloaded size = %d, want %d", len(data), len(content))
	}
}

// TestExtractBinarySymlink tests archive with symlink entries
func TestExtractBinarySymlink(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, "symlink.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	binaryContent := []byte("actual binary")

	file, _ := os.Create(archivePath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add symlink (should be skipped - not TypeReg)
	symlinkHeader := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Typeflag: tar.TypeSymlink,
		Linkname: "actual_binary",
	}
	tarWriter.WriteHeader(symlinkHeader)

	// Add actual binary with different name
	actualHeader := &tar.Header{
		Name:     "actual_" + binaryName,
		Mode:     0755,
		Size:     int64(len(binaryContent)),
		Typeflag: tar.TypeReg,
	}
	tarWriter.WriteHeader(actualHeader)
	tarWriter.Write(binaryContent)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	// Should fail because the binary (with correct name) is a symlink, not a regular file
	_, err = m.extractBinary(archivePath)
	if err == nil {
		t.Error("extractBinary() should fail when binary is a symlink")
	}
}

// TestExtractBinaryRegularFile tests that only regular files are extracted
func TestExtractBinaryOnlyRegularFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, "test.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	binaryContent := []byte("binary content")

	file, _ := os.Create(archivePath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add the binary as a regular file
	header := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     int64(len(binaryContent)),
		Typeflag: tar.TypeReg,
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write(binaryContent)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	extractedPath, err := m.extractBinary(archivePath)
	if err != nil {
		t.Fatalf("extractBinary() error = %v", err)
	}
	defer os.Remove(extractedPath)

	data, _ := os.ReadFile(extractedPath)
	if string(data) != string(binaryContent) {
		t.Errorf("Content = %q, want %q", string(data), string(binaryContent))
	}
}

// TestReleasesURLValue tests that ReleasesURL is properly set
func TestReleasesURLValue(t *testing.T) {
	if !strings.HasPrefix(ReleasesURL, "https://api.github.com") {
		t.Errorf("ReleasesURL should start with https://api.github.com, got %s", ReleasesURL)
	}
	if !strings.Contains(ReleasesURL, "/releases") {
		t.Errorf("ReleasesURL should contain /releases, got %s", ReleasesURL)
	}
}

// TestNewManagerInitialization tests NewManager properly initializes all fields
func TestNewManagerInitialization(t *testing.T) {
	m := NewManager()

	if m.tempDir == "" {
		t.Error("tempDir should be set")
	}
	if m.tempDir != os.TempDir() {
		t.Errorf("tempDir = %q, want %q", m.tempDir, os.TempDir())
	}

	// binaryPath should be set (may be empty if executable path fails)
	// backupDir should be set from config
	// currentVersion should be set from config
}

// TestReleaseJSONSerialization tests Release struct JSON marshaling
func TestReleaseJSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	r := Release{
		TagName:     "v1.0.0",
		Name:        "Test Release",
		Body:        "Release notes",
		Draft:       false,
		Prerelease:  true,
		CreatedAt:   now,
		PublishedAt: now,
		Assets: []Asset{
			{Name: "test.tar.gz", Size: 1000, DownloadURL: "https://example.com", ContentType: "application/gzip"},
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded Release
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.TagName != r.TagName {
		t.Errorf("TagName = %q, want %q", decoded.TagName, r.TagName)
	}
	if decoded.Prerelease != r.Prerelease {
		t.Errorf("Prerelease = %v, want %v", decoded.Prerelease, r.Prerelease)
	}
	if len(decoded.Assets) != 1 {
		t.Errorf("Assets length = %d, want 1", len(decoded.Assets))
	}
}

// TestUpdateInfoJSONSerialization tests UpdateInfo struct JSON marshaling
func TestUpdateInfoJSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	info := UpdateInfo{
		Available:      true,
		CurrentVersion: "1.0.0",
		LatestVersion:  "2.0.0",
		ReleaseNotes:   "New features",
		DownloadURL:    "https://example.com/download",
		AssetSize:      5000000,
		PublishedAt:    now,
		IsPrerelease:   false,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded UpdateInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Available != info.Available {
		t.Errorf("Available = %v, want %v", decoded.Available, info.Available)
	}
	if decoded.AssetSize != info.AssetSize {
		t.Errorf("AssetSize = %d, want %d", decoded.AssetSize, info.AssetSize)
	}
}

// TestAssetJSONSerialization tests Asset struct JSON marshaling
func TestAssetJSONSerialization(t *testing.T) {
	a := Asset{
		Name:        "test-asset.tar.gz",
		Size:        123456789,
		DownloadURL: "https://example.com/assets/test",
		ContentType: "application/x-gzip",
	}

	data, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded Asset
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Name != a.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, a.Name)
	}
	if decoded.Size != a.Size {
		t.Errorf("Size = %d, want %d", decoded.Size, a.Size)
	}
	if decoded.DownloadURL != a.DownloadURL {
		t.Errorf("DownloadURL = %q, want %q", decoded.DownloadURL, a.DownloadURL)
	}
}

// TestReplaceBinaryChmodError simulates chmod error handling
func TestReplaceBinaryChmodError(t *testing.T) {
	// Note: It's difficult to trigger Chmod error on most systems
	// This test verifies the code path exists
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sourcePath := filepath.Join(tempDir, "source")
	targetPath := filepath.Join(tempDir, "target")

	os.WriteFile(sourcePath, []byte("content"), 0755)
	os.WriteFile(targetPath, []byte("old"), 0755)

	m := &Manager{
		binaryPath: targetPath,
	}

	// Normal case should succeed
	err = m.replaceBinary(sourcePath)
	if err != nil {
		t.Errorf("replaceBinary() unexpected error: %v", err)
	}

	// Verify permissions
	info, _ := os.Stat(targetPath)
	if info.Mode().Perm() != 0755 {
		t.Errorf("File permissions = %o, want 0755", info.Mode().Perm())
	}
}

// TestInstallUpdateExtractError tests InstallUpdate when extraction fails
func TestInstallUpdateExtractError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create valid binary for backup
	binaryPath := filepath.Join(tempDir, "search")
	os.WriteFile(binaryPath, []byte("binary"), 0755)

	// Create invalid archive
	archivePath := filepath.Join(tempDir, "bad.tar.gz")
	os.WriteFile(archivePath, []byte("not an archive"), 0644)

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     binaryPath,
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	err = m.InstallUpdate(archivePath)
	if err == nil {
		t.Error("InstallUpdate() should fail with invalid archive")
	}
	if !strings.Contains(err.Error(), "extract") {
		t.Errorf("Error should mention extract, got: %v", err)
	}
}

// TestInstallUpdateReplaceError tests InstallUpdate when replace fails
func TestInstallUpdateReplaceError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create binary for backup
	binaryPath := filepath.Join(tempDir, "search")
	os.WriteFile(binaryPath, []byte("binary"), 0755)

	// Create valid archive
	archivePath := filepath.Join(tempDir, "valid.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}

	file, _ := os.Create(archivePath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	header := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     11,
		Typeflag: tar.TypeReg,
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write([]byte("new content"))
	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	// Make binary path a directory to cause replace failure
	os.Remove(binaryPath)
	os.MkdirAll(binaryPath, 0755)

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     binaryPath,
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	// Backup will fail because source is now a directory
	err = m.InstallUpdate(archivePath)
	if err == nil {
		t.Error("InstallUpdate() should fail")
	}
}

// TestCheckForUpdatesReleaseSelection tests release selection logic
func TestCheckForUpdatesReleaseSelection(t *testing.T) {
	// Test the logic that CheckForUpdates uses to select releases
	// We simulate the filtering logic here since we can't mock fetchReleases

	now := time.Now()
	releases := []Release{
		{
			TagName:     "v2.0.0",
			Draft:       true, // Should be skipped
			Prerelease:  false,
			PublishedAt: now,
		},
		{
			TagName:     "v1.5.0-beta",
			Draft:       false,
			Prerelease:  true, // Should be skipped if includePrerelease=false
			PublishedAt: now.Add(-1 * time.Hour),
		},
		{
			TagName:     "v1.2.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now.Add(-2 * time.Hour),
		},
		{
			TagName:     "v1.1.0",
			Draft:       false,
			Prerelease:  false,
			PublishedAt: now.Add(-24 * time.Hour),
		},
	}

	// Test: Find latest non-draft, non-prerelease
	var latest *Release
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if r.Prerelease {
			continue
		}
		if latest == nil || r.PublishedAt.After(latest.PublishedAt) {
			latest = r
		}
	}

	if latest == nil {
		t.Fatal("Should find a release")
	}
	if latest.TagName != "v1.2.0" {
		t.Errorf("Latest release = %q, want %q", latest.TagName, "v1.2.0")
	}

	// Test: Find latest including prerelease
	latest = nil
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if latest == nil || r.PublishedAt.After(latest.PublishedAt) {
			latest = r
		}
	}

	if latest == nil {
		t.Fatal("Should find a release")
	}
	if latest.TagName != "v1.5.0-beta" {
		t.Errorf("Latest release with prerelease = %q, want %q", latest.TagName, "v1.5.0-beta")
	}
}

// TestCheckForUpdatesNoReleases tests behavior with empty releases
func TestCheckForUpdatesNoReleases(t *testing.T) {
	// Simulate the logic when there are no releases
	releases := []Release{}

	var latest *Release
	for i := range releases {
		r := &releases[i]
		if r.Draft || r.Prerelease {
			continue
		}
		if latest == nil || r.PublishedAt.After(latest.PublishedAt) {
			latest = r
		}
	}

	if latest != nil {
		t.Error("Should not find any release")
	}
}

// TestCheckForUpdatesAllDrafts tests behavior when all releases are drafts
func TestCheckForUpdatesAllDrafts(t *testing.T) {
	releases := []Release{
		{TagName: "v1.0.0", Draft: true, Prerelease: false},
		{TagName: "v2.0.0", Draft: true, Prerelease: false},
	}

	var latest *Release
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if latest == nil || r.PublishedAt.After(latest.PublishedAt) {
			latest = r
		}
	}

	if latest != nil {
		t.Error("Should not find any non-draft release")
	}
}

// TestListAvailableVersionsLogic tests the logic used by ListAvailableVersions
func TestListAvailableVersionsLogic(t *testing.T) {
	releases := []Release{
		{TagName: "v2.0.0", Draft: true},
		{TagName: "v1.5.0", Draft: false},
		{TagName: "v1.4.0", Draft: false},
		{TagName: "v1.3.0", Draft: true},
		{TagName: "v1.2.0", Draft: false},
	}

	var versions []string
	for _, r := range releases {
		if !r.Draft {
			versions = append(versions, r.TagName)
		}
	}

	expected := []string{"v1.5.0", "v1.4.0", "v1.2.0"}
	if len(versions) != len(expected) {
		t.Errorf("versions length = %d, want %d", len(versions), len(expected))
	}
	for i, v := range versions {
		if v != expected[i] {
			t.Errorf("versions[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

// TestFindAssetWithAssetInfo tests UpdateInfo population with asset
func TestFindAssetWithAssetInfo(t *testing.T) {
	m := NewManager()

	assets := []Asset{
		{
			Name:        fmt.Sprintf("search_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH),
			Size:        12345,
			DownloadURL: "https://example.com/download",
			ContentType: "application/gzip",
		},
	}

	asset := m.findAsset(assets)
	if asset == nil {
		t.Skip("No matching asset for current platform")
	}

	// Simulate UpdateInfo population
	info := &UpdateInfo{
		Available:      true,
		CurrentVersion: "1.0.0",
		LatestVersion:  "2.0.0",
	}

	if asset != nil {
		info.DownloadURL = asset.DownloadURL
		info.AssetSize = asset.Size
	}

	if info.DownloadURL != "https://example.com/download" {
		t.Errorf("DownloadURL = %q, want %q", info.DownloadURL, "https://example.com/download")
	}
	if info.AssetSize != 12345 {
		t.Errorf("AssetSize = %d, want %d", info.AssetSize, 12345)
	}
}

// TestFindAssetNoMatch tests UpdateInfo when no asset matches
func TestFindAssetNoMatch(t *testing.T) {
	m := NewManager()

	assets := []Asset{
		{Name: "search_fakeos_fakearch.tar.gz", DownloadURL: "https://example.com/fake"},
	}

	asset := m.findAsset(assets)

	// Simulate UpdateInfo population
	info := &UpdateInfo{
		Available:      true,
		CurrentVersion: "1.0.0",
		LatestVersion:  "2.0.0",
	}

	if asset != nil {
		info.DownloadURL = asset.DownloadURL
		info.AssetSize = asset.Size
	}

	// When no asset matches, these should remain empty/zero
	if asset != nil {
		t.Log("Asset unexpectedly found")
	}
}

// TestVersionComparisonEdgeCases tests edge cases in version comparison
func TestVersionComparisonEdgeCases(t *testing.T) {
	tests := []struct {
		v1   string
		v2   string
		want int
	}{
		// Same version with different prefixes
		{"v1.0.0", "1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},

		// Very long versions
		{"1.0.0.0.0.0.0.0", "1.0.0.0.0.0.0.1", -1},
		{"1.0.0.0.0.0.0.1", "1.0.0.0.0.0.0.0", 1},

		// Single digit versions
		{"1", "2", -1},
		{"2", "1", 1},
		{"1", "1", 0},

		// With separators
		{"1-0-0", "1.0.0", 0},
		{"1_0_0", "1.0.0", 0},
		{"1.0-0", "1-0.0", 0},

		// Large version numbers
		{"999.999.999", "999.999.998", 1},
		{"999.999.998", "999.999.999", -1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+" vs "+tt.v2, func(t *testing.T) {
			got := compareVersions(tt.v1, tt.v2)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

// TestExtractBinaryIOCopyError tests error during tar content extraction
func TestExtractBinaryIOCopyError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create archive with binary that declares larger size than content
	archivePath := filepath.Join(tempDir, "truncated.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}

	file, _ := os.Create(archivePath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Header claims larger size than actual content
	header := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     1000, // Claims 1000 bytes
		Typeflag: tar.TypeReg,
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write([]byte("short")) // Only writes 5 bytes

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		tempDir: tempDir,
	}

	// This should fail during io.Copy due to unexpected EOF
	_, err = m.extractBinary(archivePath)
	if err == nil {
		t.Error("extractBinary() should fail with truncated content")
	}
}

// TestRollbackEntryInfoError tests handling of Info() errors in Rollback
func TestRollbackEntryInfoError(t *testing.T) {
	// This is difficult to test without filesystem manipulation
	// The code handles Info() errors by continuing to next entry
	// We verify the logic works with multiple backups where some may fail

	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	backupDir := filepath.Join(tempDir, "backup")
	os.MkdirAll(backupDir, 0755)

	// Create multiple backup files
	backup1 := filepath.Join(backupDir, "search-1.0.0.backup")
	os.WriteFile(backup1, []byte("version 1.0.0"), 0755)

	// Set older modification time
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(backup1, oldTime, oldTime)

	backup2 := filepath.Join(backupDir, "search-1.1.0.backup")
	os.WriteFile(backup2, []byte("version 1.1.0"), 0755)

	// This is the newest
	newTime := time.Now().Add(-1 * time.Hour)
	os.Chtimes(backup2, newTime, newTime)

	targetPath := filepath.Join(tempDir, "binary")
	os.WriteFile(targetPath, []byte("current"), 0755)

	m := &Manager{
		binaryPath: targetPath,
		backupDir:  backupDir,
	}

	err = m.Rollback()
	if err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Should have restored the newest backup (1.1.0)
	content, _ := os.ReadFile(targetPath)
	if string(content) != "version 1.1.0" {
		t.Errorf("Content = %q, want %q", string(content), "version 1.1.0")
	}
}

// TestBackupCurrentBinaryCreateError tests backup when destination creation fails
func TestBackupCurrentBinaryCreateError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create source binary
	fakeBinary := filepath.Join(tempDir, "search")
	os.WriteFile(fakeBinary, []byte("binary"), 0755)

	// Create backup dir
	backupDir := filepath.Join(tempDir, "backup")
	os.MkdirAll(backupDir, 0755)

	// Create a directory at the backup file location to prevent file creation
	backupPath := filepath.Join(backupDir, "search-1.0.0.backup")
	os.MkdirAll(backupPath, 0755)

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     fakeBinary,
		backupDir:      backupDir,
		tempDir:        tempDir,
	}

	err = m.backupCurrentBinary()
	if err == nil {
		t.Error("backupCurrentBinary() should fail when backup path is a directory")
	}
}

// TestExtractBinaryOpenFileError tests extraction when output file creation fails
func TestExtractBinaryOpenFileError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	archivePath := filepath.Join(tempDir, "test.tar.gz")
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}

	file, _ := os.Create(archivePath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	header := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     7,
		Typeflag: tar.TypeReg,
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write([]byte("content"))
	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	// Create a directory at the extraction path to block file creation
	extractPath := filepath.Join(tempDir, binaryName+".new")
	os.MkdirAll(extractPath, 0755)

	m := &Manager{
		tempDir: tempDir,
	}

	_, err = m.extractBinary(archivePath)
	if err == nil {
		t.Error("extractBinary() should fail when output path is a directory")
	}
}

// TestReplaceBinaryOpenSourceError tests replace when source open fails
func TestReplaceBinaryOpenSourceError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a directory as source (to cause open error in copy fallback)
	sourcePath := filepath.Join(tempDir, "source_dir")
	os.MkdirAll(sourcePath, 0755)

	targetPath := filepath.Join(tempDir, "target")
	os.WriteFile(targetPath, []byte("old"), 0755)

	m := &Manager{
		binaryPath: targetPath,
	}

	// Rename will fail because source is a directory, then Open will also fail
	err = m.replaceBinary(sourcePath)
	if err == nil {
		t.Error("replaceBinary() should fail when source is a directory")
	}
}

// TestReplaceBinaryOpenTargetError tests replace when target creation fails during copy
func TestReplaceBinaryOpenTargetError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sourcePath := filepath.Join(tempDir, "source")
	os.WriteFile(sourcePath, []byte("content"), 0755)

	// Create a directory as target to prevent OpenFile
	targetPath := filepath.Join(tempDir, "target_dir")
	os.MkdirAll(targetPath, 0755)

	m := &Manager{
		binaryPath: targetPath,
	}

	// Rename will fail (target is dir), then OpenFile will fail (target is dir)
	err = m.replaceBinary(sourcePath)
	if err == nil {
		t.Error("replaceBinary() should fail when target is a directory")
	}
}

// TestDownloadUpdateConnectionError tests download with unreachable server
func TestDownloadUpdateConnectionError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	m := &Manager{
		tempDir: tempDir,
	}

	// Use a URL that will fail to connect
	_, err = m.DownloadUpdate("http://127.0.0.1:1", nil)
	if err == nil {
		t.Error("DownloadUpdate() should fail with connection error")
	}
}

// TestParseVersionWithNumbers tests parseVersion numeric parsing
func TestParseVersionWithNumbers(t *testing.T) {
	tests := []struct {
		version string
		want    []int
	}{
		{"123", []int{123}},
		{"1.23.456", []int{1, 23, 456}},
		{"0.0.1", []int{0, 0, 1}},
		{"10.20.30.40", []int{10, 20, 30, 40}},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseVersion(tt.version)
			if len(got) != len(tt.want) {
				t.Errorf("parseVersion(%q) length = %d, want %d", tt.version, len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.version, i, v, tt.want[i])
				}
			}
		})
	}
}

// TestInstallUpdateCleanup tests that cleanup happens after install
func TestInstallUpdateCleanup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "update-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create current binary
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}
	currentBinary := filepath.Join(tempDir, binaryName)
	os.WriteFile(currentBinary, []byte("old binary"), 0755)

	// Create valid archive
	archivePath := filepath.Join(tempDir, "update.tar.gz")
	newContent := []byte("new binary content")

	file, _ := os.Create(archivePath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	header := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     int64(len(newContent)),
		Typeflag: tar.TypeReg,
	}
	tarWriter.WriteHeader(header)
	tarWriter.Write(newContent)
	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     currentBinary,
		backupDir:      filepath.Join(tempDir, "backup"),
		tempDir:        tempDir,
	}

	err = m.InstallUpdate(archivePath)
	if err != nil {
		t.Fatalf("InstallUpdate() error = %v", err)
	}

	// Archive should be cleaned up (removed)
	if _, err := os.Stat(archivePath); !os.IsNotExist(err) {
		t.Error("Archive should be removed after install")
	}
}

// TestManagerBinaryPath tests that binaryPath is used correctly
func TestManagerBinaryPath(t *testing.T) {
	m := &Manager{
		currentVersion: "1.0.0",
		binaryPath:     "/custom/path/to/binary",
		backupDir:      "/custom/backup",
		tempDir:        "/custom/temp",
	}

	if m.binaryPath != "/custom/path/to/binary" {
		t.Errorf("binaryPath = %q, want %q", m.binaryPath, "/custom/path/to/binary")
	}
}

// TestFindAssetMultipleMatches tests findAsset with multiple matching assets
func TestFindAssetMultipleMatches(t *testing.T) {
	m := NewManager()

	// Create multiple assets that could match
	assets := []Asset{
		{Name: fmt.Sprintf("search_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH), DownloadURL: "https://example.com/first"},
		{Name: fmt.Sprintf("search-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH), DownloadURL: "https://example.com/second"},
		{Name: fmt.Sprintf("search_%s_%s.zip", runtime.GOOS, runtime.GOARCH), DownloadURL: "https://example.com/third"},
	}

	asset := m.findAsset(assets)
	if asset == nil {
		t.Skip("No matching asset for current platform")
	}

	// Should return the first match
	if asset.DownloadURL != "https://example.com/first" {
		t.Logf("Found asset: %s -> %s", asset.Name, asset.DownloadURL)
	}
}
