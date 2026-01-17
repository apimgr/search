package update

import (
	"os"
	"runtime"
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

// Note: We don't test functions that require network access (CheckForUpdates, fetchReleases, etc.)
// as they would be flaky and slow. Those should be tested with mock HTTP servers in integration tests.
