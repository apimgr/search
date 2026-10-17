package update

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

// newTestManager returns a Manager wired to temporary directories so tests
// never touch the real binary or backup directory
func newTestManager(t *testing.T, version string) *Manager {
	t.Helper()
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "bin", config.ProjectName)
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("current binary"), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return &Manager{
		currentVersion: version,
		binaryPath:     binaryPath,
		backupDir:      filepath.Join(dir, "backups"),
		tempDir:        filepath.Join(dir, "tmp"),
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager() = nil, want a manager")
	}
	if m.GetCurrentVersion() != config.Version {
		t.Errorf("GetCurrentVersion() = %q, want %q", m.GetCurrentVersion(), config.Version)
	}
	if m.backupDir == "" {
		t.Error("NewManager() backupDir is empty")
	}
	if m.tempDir == "" {
		t.Error("NewManager() tempDir is empty")
	}
}

func TestNormalizeBranch(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"stable", BranchStable},
		{"beta", BranchBeta},
		{"devel", BranchDevel},
		{"daily", BranchDevel},
		{"rolling", BranchDevel},
		{"dev", BranchDevel},
		{"development", BranchDevel},
		{" DEVELOPMENT ", BranchDevel},
		{"BETA", BranchBeta},
		{"  Daily  ", BranchDevel},
		{"", BranchStable},
		{"nightly", BranchStable},
		{"garbage", BranchStable},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormalizeBranch(tt.in); got != tt.want {
				t.Errorf("NormalizeBranch(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsValidBranch(t *testing.T) {
	valid := []string{BranchStable, BranchBeta, BranchDevel, "daily", "rolling", "dev", "development", " Stable ", "DEV"}
	for _, b := range valid {
		if !IsValidBranch(b) {
			t.Errorf("IsValidBranch(%q) = false, want true", b)
		}
	}

	invalid := []string{"", "nightly", "edge"}
	for _, b := range invalid {
		if IsValidBranch(b) {
			t.Errorf("IsValidBranch(%q) = true, want false", b)
		}
	}
}

func TestMatchesBranchIsCumulative(t *testing.T) {
	stable := Release{TagName: "v1.2.3", Prerelease: false}
	beta := Release{TagName: "v1.3.0-beta", Prerelease: true}
	devel := Release{TagName: DevelTag, Prerelease: true}
	otherPre := Release{TagName: "v2.0.0-rc1", Prerelease: true}

	tests := []struct {
		name    string
		release Release
		branch  string
		want    bool
	}{
		{"stable release on stable channel", stable, BranchStable, true},
		{"stable release on beta channel", stable, BranchBeta, true},
		{"stable release on devel channel", stable, BranchDevel, true},
		{"beta release on stable channel", beta, BranchStable, false},
		{"beta release on beta channel", beta, BranchBeta, true},
		{"beta release on devel channel", beta, BranchDevel, true},
		{"devel release on stable channel", devel, BranchStable, false},
		{"devel release on beta channel", devel, BranchBeta, false},
		{"devel release on devel channel", devel, BranchDevel, true},
		{"unknown prerelease on devel channel", otherPre, BranchDevel, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesBranch(tt.release, tt.branch); got != tt.want {
				t.Errorf("matchesBranch(%q, %q) = %v, want %v", tt.release.TagName, tt.branch, got, tt.want)
			}
		})
	}
}

func TestIsSemverTag(t *testing.T) {
	semver := []string{"1.0.0", "v1.0.0", "v10.20.30", "v1.2.3-beta"}
	for _, tag := range semver {
		if !isSemverTag(tag) {
			t.Errorf("isSemverTag(%q) = false, want true", tag)
		}
	}

	nonSemver := []string{"devel", "daily", "", "beta", "v1.2", "latest"}
	for _, tag := range nonSemver {
		if isSemverTag(tag) {
			t.Errorf("isSemverTag(%q) = true, want false", tag)
		}
	}
}

func TestIsNewerReleaseSemver(t *testing.T) {
	m := newTestManager(t, "1.2.3")

	tests := []struct {
		tag  string
		want bool
	}{
		{"v1.2.4", true},
		{"v2.0.0", true},
		{"v1.3.0", true},
		{"v1.2.3", false},
		{"1.2.3", false},
		{"v1.2.2", false},
		{"v1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := m.isNewerRelease(Release{TagName: tt.tag})
			if got != tt.want {
				t.Errorf("isNewerRelease(%q) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestIsNewerReleaseUsesBuildEpochForRollingTags(t *testing.T) {
	buildTime := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	m := newTestManager(t, "1.2.3")
	m.buildEpoch = buildTime.Unix()

	newer := Release{TagName: DevelTag, Prerelease: true, PublishedAt: buildTime.Add(24 * time.Hour)}
	if !m.isNewerRelease(newer) {
		t.Error("isNewerRelease(devel published after build) = false, want true")
	}

	older := Release{TagName: DevelTag, Prerelease: true, PublishedAt: buildTime.Add(-24 * time.Hour)}
	if m.isNewerRelease(older) {
		t.Error("isNewerRelease(devel published before build) = true, want false")
	}

	// Without a build epoch nothing about a rolling tag can be proven newer
	m.buildEpoch = 0
	if m.isNewerRelease(newer) {
		t.Error("isNewerRelease(devel) with no build epoch = true, want false")
	}
}

func TestGetBinaryName(t *testing.T) {
	got := GetBinaryName()

	want := config.ProjectName + "-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want += ".exe"
	}
	if got != want {
		t.Errorf("GetBinaryName() = %q, want %q", got, want)
	}

	// Per AI.md PART 22 the release publishes raw binaries, never archives
	if strings.HasSuffix(got, ".tar.gz") || strings.HasSuffix(got, ".zip") {
		t.Errorf("GetBinaryName() = %q, want an unarchived binary name", got)
	}
}

func TestFindAssetExactMatch(t *testing.T) {
	m := newTestManager(t, "1.0.0")
	want := GetBinaryName()

	assets := []Asset{
		{Name: "sha256.txt", DownloadURL: "https://example.invalid/sha256.txt"},
		{Name: config.ProjectName + "-cli-" + runtime.GOOS + "-" + runtime.GOARCH, DownloadURL: "https://example.invalid/cli"},
		{Name: want, DownloadURL: "https://example.invalid/binary", Size: 4096},
	}

	asset := m.findAsset(assets)
	if asset == nil {
		t.Fatalf("findAsset() = nil, want the %q asset", want)
	}
	if asset.Name != want {
		t.Errorf("findAsset() name = %q, want %q", asset.Name, want)
	}
	if asset.Size != 4096 {
		t.Errorf("findAsset() size = %d, want 4096", asset.Size)
	}
}

func TestFindAssetNeverMatchesClientBinary(t *testing.T) {
	m := newTestManager(t, "1.0.0")

	// The client binary name contains the server binary name as a prefix, so a
	// substring match would wrongly select it
	assets := []Asset{
		{Name: config.ProjectName + "-cli-" + runtime.GOOS + "-" + runtime.GOARCH, DownloadURL: "https://example.invalid/cli"},
	}

	if asset := m.findAsset(assets); asset != nil {
		t.Errorf("findAsset() = %q, want nil", asset.Name)
	}
}

func TestFindAssetNoMatch(t *testing.T) {
	m := newTestManager(t, "1.0.0")

	assets := []Asset{
		{Name: config.ProjectName + "-plan9-riscv64"},
		{Name: "version.txt"},
	}

	if asset := m.findAsset(assets); asset != nil {
		t.Errorf("findAsset() = %q, want nil", asset.Name)
	}
}

func TestFindAssetEmpty(t *testing.T) {
	m := newTestManager(t, "1.0.0")
	if asset := m.findAsset(nil); asset != nil {
		t.Errorf("findAsset(nil) = %q, want nil", asset.Name)
	}
}

func TestFindChecksumURL(t *testing.T) {
	m := newTestManager(t, "1.0.0")

	tests := []struct {
		name   string
		assets []Asset
		want   string
	}{
		{
			name:   "sha256.txt",
			assets: []Asset{{Name: "sha256.txt", DownloadURL: "https://example.invalid/sha256.txt"}},
			want:   "https://example.invalid/sha256.txt",
		},
		{
			name:   "checksums.txt",
			assets: []Asset{{Name: "checksums.txt", DownloadURL: "https://example.invalid/checksums.txt"}},
			want:   "https://example.invalid/checksums.txt",
		},
		{
			name:   "case insensitive",
			assets: []Asset{{Name: "SHA256.TXT", DownloadURL: "https://example.invalid/sha"}},
			want:   "https://example.invalid/sha",
		},
		{
			name:   "absent",
			assets: []Asset{{Name: "version.txt", DownloadURL: "https://example.invalid/version.txt"}},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.findChecksumURL(tt.assets); got != tt.want {
				t.Errorf("findChecksumURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		version string
		want    []int
	}{
		{"1.2.3", []int{1, 2, 3}},
		{"v1.2.3", []int{1, 2, 3}},
		{"0.0.1", []int{0, 0, 1}},
		{"10.20.30", []int{10, 20, 30}},
		{"1.2.3-4", []int{1, 2, 3, 4}},
		{"1_2_3", []int{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseVersion(tt.version)
			if len(got) != len(tt.want) {
				t.Fatalf("parseVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseVersion(%q)[%d] = %d, want %d", tt.version, i, got[i], tt.want[i])
				}
			}
		})
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
		{"2.0.0", "1.9.9", 1},
		{"1.10.0", "1.9.0", 1},
		{"1.0.0", "1.0.0.1", -1},
		{"1.0.0.1", "1.0.0", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			if got := compareVersions(tt.v1, tt.v2); got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
			}
		})
	}
}

func TestBackupCurrentBinary(t *testing.T) {
	m := newTestManager(t, "1.0.0")

	if err := m.backupCurrentBinary(); err != nil {
		t.Fatalf("backupCurrentBinary() error = %v", err)
	}

	backupPath := filepath.Join(m.backupDir, fmt.Sprintf("search-%s.backup", m.currentVersion))
	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("ReadFile(backup) error = %v", err)
	}
	if string(content) != "current binary" {
		t.Errorf("backup content = %q, want %q", string(content), "current binary")
	}
}

func TestBackupCurrentBinaryMissingSource(t *testing.T) {
	m := newTestManager(t, "1.0.0")
	if err := os.Remove(m.binaryPath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if err := m.backupCurrentBinary(); err == nil {
		t.Error("backupCurrentBinary() = nil, want an error when the binary is missing")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")

	if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := copyFile(dst, src); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "payload" {
		t.Errorf("copied content = %q, want %q", string(content), "payload")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if info.Mode().Perm()&0111 == 0 {
			t.Errorf("copyFile() mode = %v, want an executable file", info.Mode().Perm())
		}
	}
}

func TestCopyFileMissingSource(t *testing.T) {
	dir := t.TempDir()
	if err := copyFile(filepath.Join(dir, "dst"), filepath.Join(dir, "missing")); err == nil {
		t.Error("copyFile() = nil, want an error for a missing source")
	}
}

func TestReplaceBinary(t *testing.T) {
	m := newTestManager(t, "1.0.0")

	if err := os.MkdirAll(m.tempDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	newBinary := filepath.Join(m.tempDir, "new")
	if err := os.WriteFile(newBinary, []byte("new binary"), 0755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := m.replaceBinary(newBinary); err != nil {
		t.Fatalf("replaceBinary() error = %v", err)
	}

	content, err := os.ReadFile(m.binaryPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != "new binary" {
		t.Errorf("installed content = %q, want %q", string(content), "new binary")
	}
}

func TestGetCurrentVersion(t *testing.T) {
	m := newTestManager(t, "3.4.5")
	if got := m.GetCurrentVersion(); got != "3.4.5" {
		t.Errorf("GetCurrentVersion() = %q, want %q", got, "3.4.5")
	}
}

func TestServiceHelpersDoNotPanic(t *testing.T) {
	// ServiceManaged and ServiceInstalled probe the host; they must always
	// answer rather than fail, whatever the environment
	_ = ServiceManaged()
	_ = ServiceInstalled()
}
