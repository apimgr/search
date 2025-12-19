package update

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// ReleasesURL is the URL to check for releases
const ReleasesURL = "https://api.github.com/repos/apimgr/search/releases"

// Manager handles update operations
type Manager struct {
	currentVersion string
	binaryPath     string
	backupDir      string
	tempDir        string
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
	Available      bool      `json:"available"`
	CurrentVersion string    `json:"current_version"`
	LatestVersion  string    `json:"latest_version"`
	ReleaseNotes   string    `json:"release_notes"`
	DownloadURL    string    `json:"download_url"`
	AssetSize      int64     `json:"asset_size"`
	PublishedAt    time.Time `json:"published_at"`
	IsPrerelease   bool      `json:"is_prerelease"`
}

// NewManager creates a new update manager
func NewManager() *Manager {
	binaryPath, _ := os.Executable()
	return &Manager{
		currentVersion: config.Version,
		binaryPath:     binaryPath,
		backupDir:      config.GetBackupDir(),
		tempDir:        os.TempDir(),
	}
}

// CheckForUpdates checks if a new version is available
func (m *Manager) CheckForUpdates(includePrerelease bool) (*UpdateInfo, error) {
	releases, err := m.fetchReleases()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}

	if len(releases) == 0 {
		return &UpdateInfo{
			Available:      false,
			CurrentVersion: m.currentVersion,
		}, nil
	}

	// Find the latest applicable release
	var latest *Release
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if r.Prerelease && !includePrerelease {
			continue
		}
		if latest == nil || r.PublishedAt.After(latest.PublishedAt) {
			latest = r
		}
	}

	if latest == nil {
		return &UpdateInfo{
			Available:      false,
			CurrentVersion: m.currentVersion,
		}, nil
	}

	// Check if newer than current
	latestVersion := strings.TrimPrefix(latest.TagName, "v")
	currentVersion := strings.TrimPrefix(m.currentVersion, "v")

	// Simple version comparison (for semver)
	isNewer := compareVersions(latestVersion, currentVersion) > 0

	// Find the appropriate asset for this platform
	asset := m.findAsset(latest.Assets)

	info := &UpdateInfo{
		Available:      isNewer,
		CurrentVersion: m.currentVersion,
		LatestVersion:  latest.TagName,
		ReleaseNotes:   latest.Body,
		PublishedAt:    latest.PublishedAt,
		IsPrerelease:   latest.Prerelease,
	}

	if asset != nil {
		info.DownloadURL = asset.DownloadURL
		info.AssetSize = asset.Size
	}

	return info, nil
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var releases []Release
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}

	return releases, nil
}

// findAsset finds the appropriate asset for the current platform
func (m *Manager) findAsset(assets []Asset) *Asset {
	// Build expected filename pattern
	os := runtime.GOOS
	arch := runtime.GOARCH

	// Map architecture names
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	}

	// Look for matching asset
	patterns := []string{
		fmt.Sprintf("search_%s_%s.tar.gz", os, arch),
		fmt.Sprintf("search-%s-%s.tar.gz", os, arch),
		fmt.Sprintf("search_%s_%s.zip", os, arch),
	}

	for _, asset := range assets {
		for _, pattern := range patterns {
			if strings.Contains(strings.ToLower(asset.Name), strings.ToLower(pattern)) ||
				asset.Name == pattern {
				return &asset
			}
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Create temp file
	tempFile, err := os.CreateTemp(m.tempDir, "search-update-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tempFile.Close()

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

// InstallUpdate installs an update from a downloaded archive
func (m *Manager) InstallUpdate(archivePath string) error {
	// Create backup of current binary
	if err := m.backupCurrentBinary(); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Extract new binary from archive
	newBinaryPath, err := m.extractBinary(archivePath)
	if err != nil {
		return fmt.Errorf("failed to extract binary: %w", err)
	}

	// Replace current binary
	if err := m.replaceBinary(newBinaryPath); err != nil {
		return fmt.Errorf("failed to replace binary: %w", err)
	}

	// Clean up
	os.Remove(archivePath)
	os.Remove(newBinaryPath)

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
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return nil
}

// extractBinary extracts the binary from a tar.gz archive
func (m *Manager) extractBinary(archivePath string) (string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	// Look for the binary
	binaryName := "search"
	if runtime.GOOS == "windows" {
		binaryName = "search.exe"
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		// Check if this is the binary
		if filepath.Base(header.Name) == binaryName && header.Typeflag == tar.TypeReg {
			// Extract to temp location
			extractPath := filepath.Join(m.tempDir, binaryName+".new")
			outFile, err := os.OpenFile(extractPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return "", err
			}

			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return "", err
			}
			outFile.Close()

			return extractPath, nil
		}
	}

	return "", fmt.Errorf("binary not found in archive")
}

// replaceBinary replaces the current binary with the new one
func (m *Manager) replaceBinary(newBinaryPath string) error {
	// On Windows, we can't replace a running binary directly
	if runtime.GOOS == "windows" {
		// Rename current to .old
		oldPath := m.binaryPath + ".old"
		os.Remove(oldPath)
		if err := os.Rename(m.binaryPath, oldPath); err != nil {
			return err
		}
	}

	// Move new binary to target location
	if err := os.Rename(newBinaryPath, m.binaryPath); err != nil {
		// Try copy instead
		src, err := os.Open(newBinaryPath)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.OpenFile(m.binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
	}

	// Ensure executable permission
	return os.Chmod(m.binaryPath, 0755)
}

// Rollback rolls back to the previous version
func (m *Manager) Rollback() error {
	// Find the most recent backup
	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return fmt.Errorf("failed to read backup directory: %w", err)
	}

	var latestBackup string
	var latestTime time.Time

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".backup") {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if latestBackup == "" || info.ModTime().After(latestTime) {
				latestBackup = entry.Name()
				latestTime = info.ModTime()
			}
		}
	}

	if latestBackup == "" {
		return fmt.Errorf("no backup found to rollback to")
	}

	backupPath := filepath.Join(m.backupDir, latestBackup)
	return m.replaceBinary(backupPath)
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
		var n int
		fmt.Sscanf(p, "%d", &n)
		nums = append(nums, n)
	}

	return nums
}

// GetCurrentVersion returns the current version
func (m *Manager) GetCurrentVersion() string {
	return m.currentVersion
}
