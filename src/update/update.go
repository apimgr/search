package update

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/service"
)

// ReleasesURL is the URL to check for releases
const ReleasesURL = "https://api.github.com/repos/apimgr/search/releases"

// Release channels per AI.md PART 22 "Update Branches".
// Channels are cumulative: a less-stable channel also accepts every release
// from all more-stable channels, so a beta or devel user is never left older
// than the newest stable release.
const (
	// BranchStable considers full releases only (v*, *.*.*)
	BranchStable = "stable"
	// BranchBeta considers beta pre-releases (*-beta) plus all stable releases
	BranchBeta = "beta"
	// BranchDevel considers the rolling "devel" pre-release plus beta and stable.
	BranchDevel = "devel"
	// BranchDaily is a legacy name for BranchDevel.
	BranchDaily = BranchDevel
)

// DevelTag is the rolling pre-release tag that is deleted and recreated nightly.
const DevelTag = "devel"

// DailyTag is a legacy name for DevelTag.
const DailyTag = DevelTag

// Manager handles update operations
type Manager struct {
	currentVersion string
	// buildEpoch is the embedded build timestamp, needed to tell whether the
	// rolling "devel" release is newer than this binary (AI.md PART 22)
	buildEpoch int64
	binaryPath string
	backupDir  string
	tempDir    string
}

// Release represents a GitHub release
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	CreatedAt   time.Time `json:"created_at"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Asset represents a release asset
type Asset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"browser_download_url"`
	ContentType string `json:"content_type"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
	ReleaseNotes   string `json:"release_notes"`
	DownloadURL    string `json:"download_url"`
	// Per AI.md PART 22: URL to sha256.txt for SHA-256 verification
	ChecksumURL string `json:"checksum_url,omitempty"`
	// AssetName is the release asset filename; sha256.txt keys its entries by
	// this name, so it is required to look the expected hash up
	AssetName string `json:"asset_name,omitempty"`
	// Branch is the release channel this result was resolved against
	Branch       string    `json:"branch"`
	AssetSize    int64     `json:"asset_size"`
	PublishedAt  time.Time `json:"published_at"`
	IsPrerelease bool      `json:"is_prerelease"`
}

// NewManager creates a new update manager
func NewManager() *Manager {
	binaryPath, _ := os.Executable()
	epoch, err := strconv.ParseInt(config.BuildEpoch, 10, 64)
	if err != nil {
		epoch = 0
	}
	return &Manager{
		currentVersion: config.Version,
		buildEpoch:     epoch,
		binaryPath:     binaryPath,
		backupDir:      config.GetBackupDir(),
		tempDir:        os.TempDir(),
	}
}

// NormalizeBranch maps a configured or CLI-supplied channel name onto a known
// release channel. Anything unrecognised falls back to the stable channel,
// which is the PART 22 default.
func NormalizeBranch(branch string) string {
	return config.NormalizeUpdateBranch(branch)
}

// IsValidBranch reports whether branch names a known release channel
func IsValidBranch(branch string) bool {
	switch strings.ToLower(strings.TrimSpace(branch)) {
	case BranchStable, BranchBeta, BranchDevel, "daily", "rolling", "dev", "development":
		return true
	default:
		return false
	}
}

// CheckForUpdates checks whether a newer release exists on the given channel.
// Per AI.md PART 22 the channels are cumulative, so the newest release across
// the requested channel and every more-stable channel wins.
func (m *Manager) CheckForUpdates(branch string) (*UpdateInfo, error) {
	return m.check(branch, time.Time{})
}

// CheckForUpdatesDeferred behaves like CheckForUpdates but only considers
// releases that have already been public for deferDays days, per the
// defer_days semantics in AI.md PART 22. Eligibility is per release: the
// NEWEST eligible release wins, so a brand-new release never resets the clock
// for an older release that has already aged past the window.
func (m *Manager) CheckForUpdatesDeferred(branch string, deferDays int) (*UpdateInfo, error) {
	branch = NormalizeBranch(branch)
	if branch == BranchDevel {
		return m.check(branch, time.Time{})
	}
	if deferDays <= 0 {
		return m.check(branch, time.Time{})
	}
	cutoff := time.Now().UTC().Add(-time.Duration(deferDays) * 24 * time.Hour)
	return m.check(branch, cutoff)
}

// check resolves the newest release on branch that is newer than the running
// build. When cutoff is non-zero, releases published after it are ineligible.
func (m *Manager) check(branch string, cutoff time.Time) (*UpdateInfo, error) {
	branch = NormalizeBranch(branch)

	releases, err := m.fetchReleases()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}

	info := &UpdateInfo{
		Available:      false,
		CurrentVersion: m.currentVersion,
		Branch:         branch,
	}

	// Consider candidates newest-first; the first release that both belongs to
	// the channel and is newer than the running build is the one to install
	candidates := make([]*Release, 0, len(releases))
	for i := range releases {
		r := &releases[i]
		if r.Draft || !matchesBranch(*r, branch) {
			continue
		}
		if !cutoff.IsZero() && r.PublishedAt.After(cutoff) {
			continue
		}
		candidates = append(candidates, r)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].PublishedAt.After(candidates[j].PublishedAt)
	})

	var latest *Release
	for _, r := range candidates {
		if m.isNewerRelease(*r) {
			latest = r
			break
		}
	}

	if latest == nil {
		return info, nil
	}

	info.Available = true
	info.LatestVersion = latest.TagName
	info.ReleaseNotes = latest.Body
	info.PublishedAt = latest.PublishedAt
	info.IsPrerelease = latest.Prerelease

	// Find the appropriate asset for this platform
	if asset := m.findAsset(latest.Assets); asset != nil {
		info.DownloadURL = asset.DownloadURL
		info.AssetName = asset.Name
		info.AssetSize = asset.Size
		// Look for sha256.txt in the same release for SHA-256 verification
		info.ChecksumURL = m.findChecksumURL(latest.Assets)
	}

	return info, nil
}

// matchesBranch implements the cumulative channel rule from AI.md PART 22:
// each channel also accepts every release from all more-stable channels.
func matchesBranch(r Release, branch string) bool {
	// Stable releases match every channel
	if !r.Prerelease {
		return true
	}
	isBeta := strings.HasSuffix(r.TagName, "-beta")
	// The devel channel is a single rolling release: tag "devel", rebuilt nightly.
	isDevel := r.TagName == DevelTag
	switch branch {
	case BranchBeta:
		return isBeta
	case BranchDevel:
		return isBeta || isDevel
	default:
		return false
	}
}

// isNewerRelease reports whether r is newer than the running build.
// Semver tags are compared numerically. The rolling "devel" tag and any other
// non-semver tag (timestamped beta builds) carry no comparable version, so
// they are compared against the embedded build epoch instead.
func (m *Manager) isNewerRelease(r Release) bool {
	if strings.EqualFold(r.TagName, m.currentVersion) ||
		strings.EqualFold(strings.TrimPrefix(r.TagName, "v"), strings.TrimPrefix(m.currentVersion, "v")) {
		return false
	}
	if isSemverTag(r.TagName) && isSemverTag(m.currentVersion) {
		return compareVersions(strings.TrimPrefix(r.TagName, "v"), strings.TrimPrefix(m.currentVersion, "v")) > 0
	}
	// A rolling or timestamped build is newer when it was published after this
	// binary was compiled; without a build epoch nothing can be proven newer
	if m.buildEpoch <= 0 {
		return false
	}
	return r.PublishedAt.Unix() > m.buildEpoch
}

// isSemverTag reports whether tag looks like MAJOR.MINOR.PATCH with an
// optional leading "v"
func isSemverTag(tag string) bool {
	return semverPattern.MatchString(strings.TrimSpace(tag))
}

// semverPattern matches a MAJOR.MINOR.PATCH tag with an optional "v" prefix
var semverPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+`)

// findChecksumURL returns the download URL of a checksums file in the release assets.
func (m *Manager) findChecksumURL(assets []Asset) string {
	names := []string{"sha256.txt", "checksums.txt", "sha256sums.txt", "SHA256SUMS", "checksums"}
	for _, asset := range assets {
		for _, name := range names {
			if strings.EqualFold(asset.Name, name) {
				return asset.DownloadURL
			}
		}
	}
	return ""
}

// fetchReleases fetches releases from GitHub API
func (m *Manager) fetchReleases() ([]Release, error) {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest("GET", ReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "Search/"+m.currentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Warn("failed to close update download response", "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	return releases, nil
}

// GetBinaryName returns the release asset name for the running platform.
// Per AI.md PART 22 the release publishes raw, unarchived binaries named
// "{project_name}-{GOOS}-{GOARCH}" with a ".exe" suffix on Windows, which is
// exactly what the Makefile release target and .github/workflows/release.yml
// upload.
func GetBinaryName() string {
	name := config.ProjectName + "-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// findAsset finds the release asset holding the binary for this platform
func (m *Manager) findAsset(assets []Asset) *Asset {
	want := GetBinaryName()
	for i := range assets {
		if assets[i].Name == want {
			return &assets[i]
		}
	}
	return nil
}

// DownloadUpdate downloads the update to a temporary location
func (m *Manager) DownloadUpdate(downloadURL string, progressFn func(downloaded, total int64)) (string, error) {
	client := &http.Client{Timeout: 30 * time.Minute}

	resp, err := client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download: %w", err)
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Create temp file
	tempFile, err := os.CreateTemp(m.tempDir, config.ProjectName+"-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err := tempFile.Close(); err != nil {
			slog.Warn("failed to close update temp file", "error", err)
		}
	}()

	// Download with progress
	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tempFile.Write(buf[:n]); werr != nil {
				return "", fmt.Errorf("failed to write: %w", werr)
			}
			downloaded += int64(n)
			if progressFn != nil {
				progressFn(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read: %w", err)
		}
	}

	return tempFile.Name(), nil
}

// VerifyChecksum verifies the SHA-256 checksum of a downloaded file.
// Per AI.md PART 22: "download binary → verify SHA-256 → atomic replace".
// checksumURL points to a sha256.txt-format file where each line is
// "<hex-sha256>  <filename>", keyed by the RELEASE ASSET name rather than the
// local temp filename, so assetName must be supplied. Verification is
// MANDATORY: an empty checksumURL means the release published no sha256.txt
// asset and the update is rejected.
func (m *Manager) VerifyChecksum(filePath, assetName, checksumURL string) error {
	if checksumURL == "" {
		return fmt.Errorf("release has no sha256.txt asset: refusing to install unverified binary")
	}
	if assetName == "" {
		assetName = filepath.Base(filePath)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(checksumURL)
	if err != nil {
		return fmt.Errorf("failed to fetch checksums: %w", err)
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("checksums endpoint returned %d", resp.StatusCode)
	}

	// Find the expected hash for the published asset name
	filename := assetName
	var expected string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == filename {
			expected = parts[0]
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read checksums: %w", err)
	}
	if expected == "" {
		return fmt.Errorf("no checksum found for %s in checksums file", filename)
	}

	// Hash the local file
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for checksum: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("failed to close update checksum file", "error", err)
		}
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed to hash file: %w", err)
	}
	got := hex.EncodeToString(h.Sum(nil))

	if got != expected {
		// Delete the corrupt download immediately — never install a bad binary
		removeErr := os.Remove(filePath)
		return errors.Join(fmt.Errorf("checksum mismatch for %s: expected %s, got %s", filename, expected, got), removeErr)
	}
	return nil
}

// InstallUpdate installs a downloaded release binary in place.
// downloadPath is the raw binary fetched from the release; assetName is the
// published asset name used to look its hash up in sha256.txt; checksumURL
// points at that sha256.txt. Per AI.md PART 22 the flow is
// "verify SHA-256 -> back up current binary -> replace running binary".
func (m *Manager) InstallUpdate(downloadPath, assetName, checksumURL string) error {
	// Per AI.md PART 22: check binary path is writable before proceeding;
	// escalate privileges if not, then return so the elevated process continues.
	// ErrNotExist is skipped — the backup step will fail with a clearer message.
	f, err := os.OpenFile(m.binaryPath, os.O_WRONLY, 0)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if errors.Is(err, os.ErrPermission) {
			if escalateErr := service.ReExecWithPrivileges(); escalateErr != nil {
				return fmt.Errorf("binary path %s is not writable and privilege escalation failed: %w", m.binaryPath, escalateErr)
			}
			return nil
		}
		return fmt.Errorf("failed to check binary path writability: %w", err)
	}
	if err == nil {
		if closeErr := f.Close(); closeErr != nil {
			return fmt.Errorf("failed to close binary writability check: %w", closeErr)
		}
	}

	// Per AI.md PART 22: verify SHA-256 before installing
	if err := m.VerifyChecksum(downloadPath, assetName, checksumURL); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}

	// Create backup of current binary
	if err := m.backupCurrentBinary(); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// The release asset is the binary itself; make it executable before it
	// takes the place of the running one
	if runtime.GOOS != "windows" {
		if err := os.Chmod(downloadPath, 0755); err != nil {
			return fmt.Errorf("failed to set permissions: %w", err)
		}
	}

	// Replace current binary
	if err := m.replaceBinary(downloadPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	// Clean up any leftover download
	if err := os.Remove(downloadPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("update installed but failed to remove download: %w", err)
	}
	return nil
}

// backupCurrentBinary creates a backup of the current binary
func (m *Manager) backupCurrentBinary() error {
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return err
	}

	backupPath := filepath.Join(m.backupDir, fmt.Sprintf("search-%s.backup", m.currentVersion))

	src, err := os.Open(m.binaryPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := src.Close(); err != nil {
			slog.Warn("failed to close current binary source", "error", err)
		}
	}()

	dst, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := dst.Close(); err != nil {
			slog.Warn("failed to close current binary backup", "error", err)
		}
	}()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}

// replaceBinary swaps the new binary in for the currently installed one using
// the platform-specific strategy from AI.md PART 22
func (m *Manager) replaceBinary(newBinaryPath string) error {
	return replaceBinary(m.binaryPath, newBinaryPath)
}

// ListAvailableVersions returns a list of available versions
func (m *Manager) ListAvailableVersions() ([]string, error) {
	releases, err := m.fetchReleases()
	if err != nil {
		return nil, err
	}

	var versions []string
	for _, r := range releases {
		if !r.Draft {
			versions = append(versions, r.TagName)
		}
	}

	return versions, nil
}

// compareVersions compares two semantic versions
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Parse versions
	parts1 := parseVersion(v1)
	parts2 := parseVersion(v2)

	// Compare each part
	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		if parts1[i] < parts2[i] {
			return -1
		}
		if parts1[i] > parts2[i] {
			return 1
		}
	}

	// If all compared parts are equal, longer version is greater
	if len(parts1) < len(parts2) {
		return -1
	}
	if len(parts1) > len(parts2) {
		return 1
	}

	return 0
}

// parseVersion parses a version string into numeric parts
func parseVersion(v string) []int {
	// Remove any prefix
	v = strings.TrimPrefix(v, "v")

	// Split by dots, dashes, and other separators
	parts := strings.FieldsFunc(v, func(r rune) bool {
		return r == '.' || r == '-' || r == '_'
	})

	var nums []int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		nums = append(nums, n)
	}

	return nums
}

// GetCurrentVersion returns the current version
func (m *Manager) GetCurrentVersion() string {
	return m.currentVersion
}
