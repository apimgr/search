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
	dataDir    string
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
		tempDir:        filepath.Join(os.TempDir(), config.ProjectOrg),
		dataDir:        config.GetDataDir(),
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
	// Only full numeric semantic-version releases count as stable releases.
	// Stable releases match every channel, while arbitrary non-prerelease tags
	// do not silently enter the cumulative channel set.
	if !r.Prerelease {
		return isSemverTag(r.TagName)
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
// and optional prerelease/build metadata.
var semverPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)

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
		if resp.StatusCode == http.StatusNotFound {
			return []Release{}, nil
		}
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

	// Keep update artifacts in a project-scoped, owner-only temporary directory.
	orgDir := m.tempDir
	if orgDir == "" {
		orgDir = filepath.Join(os.TempDir(), config.ProjectOrg)
	}
	if err := os.MkdirAll(orgDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create update temp namespace: %w", err)
	}
	tempDir, err := os.MkdirTemp(orgDir, config.InternalName+"-")
	if err != nil {
		return "", fmt.Errorf("failed to create update temp directory: %w", err)
	}
	keepTempDir := false
	defer func() {
		if !keepTempDir {
			if err := os.RemoveAll(tempDir); err != nil {
				slog.Warn("failed to remove update temp directory", "error", err)
			}
		}
	}()
	tempFile, err := os.CreateTemp(tempDir, "update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}

	// Download with progress
	total := resp.ContentLength
	var downloaded int64

	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := tempFile.Write(buf[:n]); werr != nil {
				return "", errors.Join(fmt.Errorf("failed to write: %w", werr), tempFile.Close())
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
			return "", errors.Join(fmt.Errorf("failed to read: %w", err), tempFile.Close())
		}
	}
	if err := tempFile.Sync(); err != nil {
		return "", errors.Join(fmt.Errorf("failed to flush update temp file: %w", err), tempFile.Close())
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("failed to close update temp file: %w", err)
	}

	keepTempDir = true
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
	defer cleanupUpdateDownload(downloadPath)
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

type stagedVersion struct {
	Version     string    `json:"version"`
	PublishedAt time.Time `json:"published_at"`
}

// StageUpdate verifies a scheduled release and atomically places it under
// {data_dir}/update for the next root-started process to install.
func (m *Manager) StageUpdate(downloadPath, assetName, checksumURL, version string, publishedAt time.Time) error {
	defer cleanupUpdateDownload(downloadPath)
	if filepath.Base(version) != version || strings.TrimSpace(version) == "" {
		return fmt.Errorf("invalid staged update version %q", version)
	}
	if err := m.VerifyChecksum(downloadPath, assetName, checksumURL); err != nil {
		return fmt.Errorf("checksum verification failed: %w", err)
	}
	if runtime.GOOS == "windows" {
		return errors.New("scheduled self-update is not supported on Windows services")
	}
	dataDir := m.dataDir
	if dataDir == "" {
		dataDir = config.GetDataDir()
	}
	updateDir := filepath.Join(dataDir, "update")
	if err := os.MkdirAll(updateDir, 0700); err != nil {
		return fmt.Errorf("create staged update directory: %w", err)
	}
	if err := os.Chmod(updateDir, 0700); err != nil {
		return fmt.Errorf("secure staged update directory: %w", err)
	}

	stagedBinary := filepath.Join(updateDir, config.ProjectName+".new")
	temporaryBinary := stagedBinary + ".tmp"
	_ = os.Remove(temporaryBinary)
	if err := copyFile(temporaryBinary, downloadPath); err != nil {
		return fmt.Errorf("copy verified update into staging: %w", err)
	}
	if err := os.Rename(temporaryBinary, stagedBinary); err != nil {
		_ = os.Remove(temporaryBinary)
		return fmt.Errorf("publish staged update binary: %w", err)
	}

	digest, err := hashUpdateFile(stagedBinary)
	if err != nil {
		return fmt.Errorf("hash staged update binary: %w", err)
	}
	if err := writeStagedFile(filepath.Join(updateDir, config.ProjectName+".sha256"), []byte(digest+"\n")); err != nil {
		return err
	}
	metadata, err := json.Marshal(stagedVersion{Version: version, PublishedAt: publishedAt.UTC()})
	if err != nil {
		return fmt.Errorf("encode staged update metadata: %w", err)
	}
	if err := writeStagedFile(filepath.Join(updateDir, config.ProjectName+".json"), append(metadata, '\n')); err != nil {
		return err
	}
	slog.Info("update staged", "version", version)
	return nil
}

// ApplyStagedUpdate verifies and installs a staged release during root startup.
// On success it re-executes the new binary; a false result means no update was
// staged.
func (m *Manager) ApplyStagedUpdate() (bool, error) {
	if runtime.GOOS == "windows" || !config.StartedElevated() {
		return false, nil
	}
	dataDir := m.dataDir
	if dataDir == "" {
		dataDir = config.GetDataDir()
	}
	updateDir := filepath.Join(dataDir, "update")
	binaryPath := filepath.Join(updateDir, config.ProjectName+".new")
	metadataPath := filepath.Join(updateDir, config.ProjectName+".json")
	checksumPath := filepath.Join(updateDir, config.ProjectName+".sha256")
	if _, err := os.Stat(updateDir); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("inspect staged update directory: %w", err)
	}
	cleanup := func() {
		if err := os.RemoveAll(updateDir); err != nil {
			slog.Error("failed to remove staged update files", "err", err)
		}
	}
	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		cleanup()
		return false, fmt.Errorf("read staged update metadata: %w", err)
	}
	var metadata stagedVersion
	if err := json.Unmarshal(metadataBytes, &metadata); err != nil || metadata.Version == "" {
		cleanup()
		return false, fmt.Errorf("invalid staged update metadata")
	}
	if !m.isNewerRelease(Release{TagName: metadata.Version, PublishedAt: metadata.PublishedAt}) {
		cleanup()
		return false, fmt.Errorf("staged update %s is not newer than %s", metadata.Version, m.currentVersion)
	}
	expectedBytes, err := os.ReadFile(checksumPath)
	if err != nil {
		cleanup()
		return false, fmt.Errorf("read staged update checksum: %w", err)
	}
	actualChecksum, err := hashUpdateFile(binaryPath)
	if err != nil {
		cleanup()
		return false, fmt.Errorf("hash staged update binary: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(string(expectedBytes)), actualChecksum) {
		cleanup()
		return false, errors.New("staged update checksum mismatch")
	}
	if err := m.backupCurrentBinary(); err != nil {
		cleanup()
		return false, fmt.Errorf("backup current binary before staged update: %w", err)
	}
	if err := m.replaceBinary(binaryPath); err != nil {
		cleanup()
		return false, fmt.Errorf("replace binary from staged update: %w", err)
	}
	cleanup()
	if err := os.Setenv("_SEARCH_UPDATE_PREVIOUS_VERSION", m.currentVersion); err != nil {
		rollbackErr := m.replaceBinary(filepath.Join(m.backupDir, fmt.Sprintf("search-%s.backup", m.currentVersion)))
		return false, errors.Join(fmt.Errorf("record previous version for update restart: %w", err), rollbackErr)
	}
	if err := os.Setenv("_SEARCH_UPDATE_NEW_VERSION", metadata.Version); err != nil {
		_ = os.Unsetenv("_SEARCH_UPDATE_PREVIOUS_VERSION")
		rollbackErr := m.replaceBinary(filepath.Join(m.backupDir, fmt.Sprintf("search-%s.backup", m.currentVersion)))
		return false, errors.Join(fmt.Errorf("record new version for update restart: %w", err), rollbackErr)
	}
	if err := RestartSelf(); err != nil {
		_ = os.Unsetenv("_SEARCH_UPDATE_PREVIOUS_VERSION")
		_ = os.Unsetenv("_SEARCH_UPDATE_NEW_VERSION")
		backupPath := filepath.Join(m.backupDir, fmt.Sprintf("search-%s.backup", m.currentVersion))
		rollbackErr := m.replaceBinary(backupPath)
		return false, errors.Join(fmt.Errorf("restart after staged update: %w", err), rollbackErr)
	}
	return true, nil
}

func writeStagedFile(path string, content []byte) (result error) {
	temporary := path + ".tmp"
	defer func() {
		if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, fmt.Errorf("remove staged update metadata: %w", err))
		}
	}()
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create staged update metadata: %w", err)
	}
	if _, err := file.Write(content); err != nil {
		return errors.Join(fmt.Errorf("write staged update metadata: %w", err), file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync staged update metadata: %w", err), file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close staged update metadata: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("publish staged update metadata: %w", err)
	}
	return nil
}

func hashUpdateFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func cleanupUpdateDownload(downloadPath string) {
	if !strings.HasPrefix(filepath.Base(downloadPath), "update-") {
		return
	}
	tempDir := filepath.Dir(downloadPath)
	orgDir := filepath.Join(os.TempDir(), config.ProjectOrg)
	if filepath.Dir(tempDir) != orgDir || !strings.HasPrefix(filepath.Base(tempDir), config.InternalName+"-") {
		return
	}
	if err := os.RemoveAll(tempDir); err != nil {
		slog.Warn("failed to remove update temp directory", "error", err)
	}
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
