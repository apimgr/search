package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// BackupMetadata contains information about a backup
// Per AI.md PART 25: manifest.json format with required fields
type BackupMetadata struct {
	// Manifest format version (e.g., "1.0.0")
	Version string `json:"version"`
	// When backup was created
	CreatedAt time.Time `json:"created_at"`
	// Who created the backup (per PART 25)
	CreatedBy string `json:"created_by"`
	// Application version (per PART 25)
	AppVersion string `json:"app_version"`
	// List of files/directories in backup
	Contents []string `json:"contents"`
	// SHA256 checksums per file
	Checksums map[string]string `json:"checksums"`
	// Overall archive checksum (per PART 25)
	Checksum string `json:"checksum"`
	// Per AI.md PART 25
	Encrypted bool `json:"encrypted"`
	// "AES-256-GCM" if encrypted
	EncryptionMethod string `json:"encryption_method"`
	// Legacy fields for backwards compatibility
	// Server title (optional)
	ServerTitle string `json:"server_title,omitempty"`
	// Total size in bytes (optional)
	Size int64 `json:"size,omitempty"`
	// Deprecated: use Contents
	Files []string `json:"files,omitempty"`
}

// Manager handles backup and restore operations
type Manager struct {
	backupDir string
	configDir string
	dataDir   string
	// Backup encryption password (never stored on disk)
	password string
	// Username of who created the backup (per PART 25)
	createdBy string
}

// cachedBackupDir holds the backup directory resolved once at server
// startup, per AI.md PART 21: "Never re-resolve the path at cleanup time."
// backupDirExplicitlySet distinguishes a real SetBackupDir call (server
// startup) from callers that never made one (one-shot CLI commands), which
// must keep re-reading config.GetBackupDir() on every call so per-invocation
// overrides (e.g. --backup-dir, or config.SetBackupDirOverride in tests)
// still take effect.
var (
	cachedBackupDir        string
	backupDirExplicitlySet bool
)

// SetBackupDir caches the backup directory resolved at server startup so
// every subsequent retention sweep and cleanup pass reuses the same path.
func SetBackupDir(path string) {
	cachedBackupDir = path
	backupDirExplicitlySet = true
}

// resolveBackupDir returns the path cached by an explicit SetBackupDir call,
// or a fresh config.GetBackupDir() resolution otherwise.
func resolveBackupDir() string {
	if backupDirExplicitlySet {
		return cachedBackupDir
	}
	return config.GetBackupDir()
}

// SetCreatedBy sets the username for backup attribution (per AI.md PART 25)
func (m *Manager) SetCreatedBy(username string) {
	m.createdBy = username
}

// SetPassword sets the backup encryption password
// Per AI.md PART 24: Password is NEVER stored - derived on-demand
func (m *Manager) SetPassword(password string) {
	m.password = password
}

// NewManager creates a new backup manager
func NewManager() *Manager {
	return &Manager{
		backupDir: resolveBackupDir(),
		configDir: config.GetConfigDir(),
		dataDir:   config.GetDataDir(),
	}
}

// Create creates a new backup archive
func (m *Manager) Create(filename string) (string, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(m.backupDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate filename if not specified
	// Per AI.md PART 21: Format is search_backup_YYYY-MM-DD_HHMMSS.tar.gz
	if filename == "" {
		filename = fmt.Sprintf("search_backup_%s.tar.gz", time.Now().Format("2006-01-02_150405"))
	}

	// Ensure .tar.gz extension
	if !strings.HasSuffix(filename, ".tar.gz") {
		filename += ".tar.gz"
	}

	backupPath := filepath.Join(m.backupDir, filename)

	// Create the archive file
	file, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	var totalSize int64
	var files []string
	checksums := make(map[string]string)

	// Backup config directory
	configFiles, configSize, configChecksums, err := m.addDirectoryToTar(tarWriter, m.configDir, "config")
	if err != nil {
		return "", fmt.Errorf("failed to backup config: %w", err)
	}
	files = append(files, configFiles...)
	totalSize += configSize
	for k, v := range configChecksums {
		checksums[k] = v
	}

	// Backup data directory (excluding logs and cache)
	dataFiles, dataSize, dataChecksums, err := m.addDirectoryToTar(tarWriter, m.dataDir, "data")
	if err != nil {
		return "", fmt.Errorf("failed to backup data: %w", err)
	}
	files = append(files, dataFiles...)
	totalSize += dataSize
	for k, v := range dataChecksums {
		checksums[k] = v
	}

	// Load config for metadata
	cfg, _ := config.Load(filepath.Join(m.configDir, "server.yml"))
	serverTitle := "Search"
	if cfg != nil {
		serverTitle = cfg.Server.Title
	}

	// Determine created_by (per AI.md PART 25)
	createdBy := m.createdBy
	if createdBy == "" {
		// Default for CLI/scheduled backups
		createdBy = "system"
	}

	// Calculate overall checksum from individual file checksums (per AI.md PART 25)
	// This provides a single verification value for the entire backup
	overallChecksum := computeOverallChecksum(checksums)

	// Create metadata per AI.md PART 25: manifest.json format
	metadata := BackupMetadata{
		// Manifest format version
		Version:   "1.0.0",
		CreatedAt: time.Now(),
		// Per PART 25: who created the backup
		CreatedBy: createdBy,
		// Per PART 25: application version
		AppVersion: config.Version,
		// Per PART 25: list of contents
		Contents:  files,
		Checksums: checksums,
		// Per PART 25: overall checksum
		Checksum: "sha256:" + overallChecksum,
		// Legacy/optional
		ServerTitle: serverTitle,
		// Legacy/optional
		Size: totalSize,
		// Legacy/deprecated: use Contents
		Files: files,
	}

	// Add metadata to archive as manifest.json
	metaJSON, _ := json.MarshalIndent(metadata, "", "  ")
	metaHeader := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(metaJSON)),
		Mode:    0644,
		ModTime: time.Now(),
	}
	if err := tarWriter.WriteHeader(metaHeader); err != nil {
		return "", fmt.Errorf("failed to write metadata header: %w", err)
	}
	if _, err := tarWriter.Write(metaJSON); err != nil {
		return "", fmt.Errorf("failed to write metadata: %w", err)
	}

	return backupPath, nil
}

// addDirectoryToTar adds a directory to the tar archive with SHA256 checksums
// Per AI.md PART 26: SHA256 checksums for all backup files
func (m *Manager) addDirectoryToTar(tw *tar.Writer, srcDir, prefix string) ([]string, int64, map[string]string, error) {
	var files []string
	var totalSize int64
	checksums := make(map[string]string)

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories we don't want to backup
		if info.IsDir() {
			name := info.Name()
			if name == "logs" || name == "cache" || name == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Skip large files (>100MB)
		if info.Size() > 100*1024*1024 {
			return nil
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.Join(prefix, relPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Open file and compute checksum while copying
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Create a hash writer to compute SHA256 while copying
		hash := sha256.New()
		multiWriter := io.MultiWriter(tw, hash)

		written, err := io.Copy(multiWriter, file)
		if err != nil {
			return err
		}

		files = append(files, header.Name)
		totalSize += written
		checksums[header.Name] = hex.EncodeToString(hash.Sum(nil))

		return nil
	})

	return files, totalSize, checksums, err
}

// computeOverallChecksum calculates a combined checksum from individual file checksums
// Per AI.md PART 25: overall checksum for backup verification
func computeOverallChecksum(checksums map[string]string) string {
	// Sort keys for deterministic output
	keys := make([]string, 0, len(checksums))
	for k := range checksums {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Combine all checksums into one hash
	hash := sha256.New()
	for _, k := range keys {
		hash.Write([]byte(k))
		hash.Write([]byte(checksums[k]))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Restore restores from a backup archive
func (m *Manager) Restore(backupPath string) error {
	// Verify backup file exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Open the archive
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	// Create backup of current config before restoring
	currentBackup := filepath.Join(m.backupDir, fmt.Sprintf("pre-restore-%s.tar.gz", time.Now().Format("20060102-150405")))
	if _, err := m.Create(filepath.Base(currentBackup)); err != nil {
		// Non-fatal, just warn
		slog.Warn("could not create pre-restore backup", "err", err)
	}

	// Extract files
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading tar: %w", err)
		}

		// Skip metadata file (manifest.json or legacy backup.json)
		if header.Name == "manifest.json" || header.Name == "backup.json" {
			continue
		}

		// Determine target path
		var targetPath string
		if strings.HasPrefix(header.Name, "config/") {
			relPath := strings.TrimPrefix(header.Name, "config/")
			targetPath = filepath.Join(m.configDir, relPath)
		} else if strings.HasPrefix(header.Name, "data/") {
			relPath := strings.TrimPrefix(header.Name, "data/")
			targetPath = filepath.Join(m.dataDir, relPath)
		} else {
			// Skip unknown paths
			continue
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Handle based on file type
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}
		case tar.TypeReg:
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file %s: %w", targetPath, err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}
			outFile.Close()
		}
	}

	return nil
}

// List returns all available backups
func (m *Manager) List() ([]BackupInfo, error) {
	var backups []BackupInfo

	if _, err := os.Stat(m.backupDir); os.IsNotExist(err) {
		return backups, nil
	}

	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !(strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tar.gz.enc")) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		backupPath := filepath.Join(m.backupDir, entry.Name())
		metadata, _ := m.GetMetadata(backupPath)

		bi := BackupInfo{
			Filename:  entry.Name(),
			Path:      backupPath,
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		}

		if metadata != nil {
			bi.Version = metadata.Version
			bi.ServerTitle = metadata.ServerTitle
			bi.FileCount = len(metadata.Files)
		}

		backups = append(backups, bi)
	}

	return backups, nil
}

// GetMetadata reads metadata from a backup archive
// Looks for manifest.json (per AI.md PART 26) or legacy backup.json
func (m *Manager) GetMetadata(backupPath string) (*BackupMetadata, error) {
	file, err := os.Open(backupPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("metadata not found in backup")
		}
		if err != nil {
			return nil, err
		}

		// Look for manifest.json (new format) or backup.json (legacy)
		if header.Name == "manifest.json" || header.Name == "backup.json" {
			var metadata BackupMetadata
			if err := json.NewDecoder(tarReader).Decode(&metadata); err != nil {
				return nil, err
			}
			return &metadata, nil
		}
	}
}

// Delete deletes a backup file
func (m *Manager) Delete(filename string) error {
	backupPath := filepath.Join(m.backupDir, filename)

	// Verify it's in the backup directory (security check)
	if !strings.HasPrefix(backupPath, m.backupDir) {
		return fmt.Errorf("invalid backup path")
	}

	return os.Remove(backupPath)
}

// BackupInfo contains summary information about a backup
type BackupInfo struct {
	Filename    string    `json:"filename"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	CreatedAt   time.Time `json:"created_at"`
	Version     string    `json:"version,omitempty"`
	ServerTitle string    `json:"server_title,omitempty"`
	FileCount   int       `json:"file_count,omitempty"`
}

// FormatSize returns a human-readable size
func (bi BackupInfo) FormatSize() string {
	const unit = 1024
	if bi.Size < unit {
		return fmt.Sprintf("%d B", bi.Size)
	}
	div, exp := int64(unit), 0
	for n := bi.Size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bi.Size)/float64(div), "KMGTPE"[exp])
}

// ScheduledBackup performs a scheduled backup with cleanup of old backups
func (m *Manager) ScheduledBackup(keepCount int) error {
	// Create new backup
	_, err := m.Create("")
	if err != nil {
		return err
	}

	// List all backups
	backups, err := m.List()
	if err != nil {
		return err
	}

	// Delete old backups if we have more than keepCount
	if len(backups) > keepCount {
		// Sort by creation time (newest first)
		// Note: backups are already sorted by filename which includes timestamp
		for i := keepCount; i < len(backups); i++ {
			if err := m.Delete(backups[i].Filename); err != nil {
				// Log but don't fail
				slog.Warn("failed to delete old backup", "file", backups[i].Filename, "err", err)
			}
		}
	}

	return nil
}

// RetentionPolicy defines backup retention rules per AI.md PART 21
// Per AI.md PART 21: exclusive priority-ordered buckets (yearly > monthly > weekly > daily)
type RetentionPolicy struct {
	// Count is max_backups: how many daily/full backups to keep once not
	// claimed by a higher-priority bucket (oldest deleted first beyond this)
	Count int `json:"count" yaml:"count"`
	// Day is unused by the bucket algorithm; retained for API/config compatibility
	Day int `json:"day" yaml:"day"`
	// Week is keep_weekly: how many Sunday backups to keep
	Week int `json:"week" yaml:"week"`
	// Month is keep_monthly: how many first-of-month backups to keep
	Month int `json:"month" yaml:"month"`
	// Year is keep_yearly: how many Jan-1st backups to keep
	Year int `json:"year" yaml:"year"`
	// MaxTotalSize is an absolute (e.g. "50G") or percent-of-volume (e.g. "10%")
	// cap enforced after bucket pruning, overriding count limits by deleting
	// oldest files first. Falsey values (empty, "0", "false", "off", ...) disable it.
	MaxTotalSize string `json:"max_total_size" yaml:"max_total_size"`
}

// DefaultRetentionPolicy returns the default retention policy
// Per AI.md PART 21: default settings keep 2 files total (1 full + 1 incremental)
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		// Keep 1 full/daily backup by default
		Count: 1,
		Day:   0,
		// No weekly/monthly/yearly retention by default
		Week:  0,
		Month: 0,
		Year:  0,
		// Default size cap: 10% of the backup volume
		MaxTotalSize: "10%",
	}
}

// isIncrementalBackup reports whether filename is a fixed-name daily/hourly
// incremental (always exactly one file, replaced every run) rather than a
// counted full/timestamped backup. Per AI.md PART 21: incrementals are
// excluded entirely from counted retention.
func isIncrementalBackup(filename string) bool {
	base := strings.TrimSuffix(filename, ".enc")
	base = strings.TrimSuffix(base, ".tar.gz")
	return strings.HasSuffix(base, "-daily") || strings.HasSuffix(base, "-hourly")
}

// markKeep marks up to limit of the newest matches (per predicate) in backups
// as kept in the shared kept set, and returns the remaining unmarked backups
// (still oldest-first) for further bucket processing. A backup matched here
// is claimed by this bucket only, never double-counted by a lower-priority one.
func markKeep(backups []BackupInfo, kept map[string]bool, limit int, matches func(BackupInfo) bool) []BackupInfo {
	if limit <= 0 {
		return backups
	}

	marked := make(map[string]bool)
	count := 0
	for i := len(backups) - 1; i >= 0 && count < limit; i-- {
		if matches(backups[i]) {
			kept[backups[i].Filename] = true
			marked[backups[i].Filename] = true
			count++
		}
	}

	var remaining []BackupInfo
	for _, b := range backups {
		if !marked[b.Filename] {
			remaining = append(remaining, b)
		}
	}
	return remaining
}

// ApplyRetention applies retention policy to backups
// Per AI.md PART 21: exclusive priority-ordered buckets — yearly (Jan 1) >
// monthly (1st) > weekly (Sunday) > daily (max_backups), oldest deleted
// first, followed by a max_total_size size-cap pass that overrides count
// limits. Incremental backups are excluded entirely from counted retention.
func (m *Manager) ApplyRetention(policy RetentionPolicy) error {
	backups, err := m.List()
	if err != nil {
		return err
	}

	var countable []BackupInfo
	for _, b := range backups {
		if !isIncrementalBackup(b.Filename) {
			countable = append(countable, b)
		}
	}

	sort.Slice(countable, func(i, j int) bool {
		return countable[i].CreatedAt.Before(countable[j].CreatedAt)
	})

	kept := make(map[string]bool)
	remaining := countable

	// Yearly: Jan 1st, highest priority
	remaining = markKeep(remaining, kept, policy.Year, func(b BackupInfo) bool {
		return b.CreatedAt.Month() == time.January && b.CreatedAt.Day() == 1
	})

	// Monthly: 1st of month
	remaining = markKeep(remaining, kept, policy.Month, func(b BackupInfo) bool {
		return b.CreatedAt.Day() == 1
	})

	// Weekly: Sunday
	remaining = markKeep(remaining, kept, policy.Week, func(b BackupInfo) bool {
		return b.CreatedAt.Weekday() == time.Sunday
	})

	// Daily: whatever is left, up to max_backups, oldest deleted first
	dailyLimit := policy.Count
	if dailyLimit < 0 {
		dailyLimit = 0
	}
	if dailyLimit > len(remaining) {
		dailyLimit = len(remaining)
	}
	for i := len(remaining) - dailyLimit; i < len(remaining); i++ {
		if i >= 0 {
			kept[remaining[i].Filename] = true
		}
	}

	// Delete everything not claimed by a bucket, oldest-first
	var toDelete []BackupInfo
	for _, b := range countable {
		if !kept[b.Filename] {
			toDelete = append(toDelete, b)
		}
	}
	sort.Slice(toDelete, func(i, j int) bool {
		return toDelete[i].CreatedAt.Before(toDelete[j].CreatedAt)
	})
	for _, b := range toDelete {
		if err := m.Delete(b.Filename); err != nil {
			slog.Warn("failed to delete old backup", "file", b.Filename, "err", err)
		}
	}

	// Size-cap pass runs after bucket pruning and overrides count limits
	if err := m.enforceMaxTotalSize(policy.MaxTotalSize); err != nil {
		slog.Warn("failed to enforce max_total_size", "err", err)
	}

	return nil
}

// enforceMaxTotalSize prunes oldest backups first until total backup-directory
// size is under maxTotalSize. Per AI.md PART 21 this overrides count-based
// retention limits and runs after bucket pruning. Full/timestamped backups are
// pruned before incrementals, since incrementals are meant to always exist as
// exactly one replaced file; if the cap still cannot be met, incrementals are
// pruned as a last resort.
func (m *Manager) enforceMaxTotalSize(maxTotalSize string) error {
	limitBytes, enabled, err := parseMaxTotalSize(maxTotalSize, m.backupDir)
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	backups, err := m.List()
	if err != nil {
		return err
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.Before(backups[j].CreatedAt)
	})

	var total int64
	for _, b := range backups {
		total += b.Size
	}

	for _, incrementalsOnly := range []bool{false, true} {
		for _, b := range backups {
			if total <= limitBytes {
				return nil
			}
			if isIncrementalBackup(b.Filename) != incrementalsOnly {
				continue
			}
			if err := m.Delete(b.Filename); err != nil {
				slog.Warn("failed to delete backup over max_total_size", "file", b.Filename, "err", err)
				continue
			}
			total -= b.Size
		}
	}

	return nil
}

// parseMaxTotalSize resolves a max_total_size setting ("10%", "50G", or a
// falsey value) into an absolute byte limit. Percent values are resolved
// against the total capacity of the volume containing path.
func parseMaxTotalSize(value, path string) (limitBytes int64, enabled bool, err error) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "0", "false", "no", "none", "disable", "disabled", "off", "never":
		return 0, false, nil
	}

	if strings.HasSuffix(v, "%") {
		pct, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
		if err != nil {
			return 0, false, fmt.Errorf("invalid max_total_size percent %q: %w", value, err)
		}
		_, total, err := DiskUsage(path)
		if err != nil {
			return 0, false, fmt.Errorf("failed to resolve disk size for max_total_size: %w", err)
		}
		return int64(float64(total) * pct / 100), true, nil
	}

	multiplier := int64(1)
	numPart := v
	switch {
	case strings.HasSuffix(v, "t"):
		multiplier = 1024 * 1024 * 1024 * 1024
		numPart = strings.TrimSuffix(v, "t")
	case strings.HasSuffix(v, "g"):
		multiplier = 1024 * 1024 * 1024
		numPart = strings.TrimSuffix(v, "g")
	case strings.HasSuffix(v, "m"):
		multiplier = 1024 * 1024
		numPart = strings.TrimSuffix(v, "m")
	case strings.HasSuffix(v, "k"):
		multiplier = 1024
		numPart = strings.TrimSuffix(v, "k")
	case strings.HasSuffix(v, "b"):
		numPart = strings.TrimSuffix(v, "b")
	}

	num, err := strconv.ParseFloat(numPart, 64)
	if err != nil {
		return 0, false, fmt.Errorf("invalid max_total_size %q: %w", value, err)
	}

	return int64(num * float64(multiplier)), true, nil
}

// CheckDiskSpace reports whether it is safe to create a new backup.
// Per AI.md PART 21: abort when free space is under 2x the most recent
// backup's size, or when disk usage exceeds thresholdPercent.
func (m *Manager) CheckDiskSpace(thresholdPercent int) (ok bool, freeBytes uint64, usedPercent float64, err error) {
	used, total, err := DiskUsage(m.backupDir)
	if err != nil {
		return false, 0, 0, err
	}
	if total == 0 {
		return false, 0, 0, fmt.Errorf("unable to determine disk size for %s", m.backupDir)
	}

	free := total - used
	usedPercent = float64(used) / float64(total) * 100

	if thresholdPercent > 0 && usedPercent > float64(thresholdPercent) {
		return false, free, usedPercent, nil
	}

	backups, listErr := m.List()
	if listErr == nil && len(backups) > 0 {
		sort.Slice(backups, func(i, j int) bool {
			return backups[i].CreatedAt.Before(backups[j].CreatedAt)
		})
		mostRecent := backups[len(backups)-1]
		if mostRecent.Size > 0 && free < uint64(mostRecent.Size)*2 {
			return false, free, usedPercent, nil
		}
	}

	return true, free, usedPercent, nil
}

// CreateEncrypted creates an encrypted backup with .enc extension
// Per AI.md PART 21: .enc extension for encrypted backups
func (m *Manager) CreateEncrypted(filename string) (string, error) {
	if m.password == "" {
		return "", fmt.Errorf("encryption password not set - use SetPassword() or BACKUP_PASSWORD env var")
	}

	// Create unencrypted backup first
	backupPath, err := m.Create(filename)
	if err != nil {
		return "", err
	}

	// Read backup data
	data, err := os.ReadFile(backupPath)
	if err != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("failed to read backup: %w", err)
	}

	// Encrypt the data
	encrypted, err := EncryptBackup(data, m.password)
	if err != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("failed to encrypt backup: %w", err)
	}

	// Create encrypted file with .enc extension per AI.md PART 21
	encryptedPath := backupPath + ".enc"
	if err := os.WriteFile(encryptedPath, encrypted, 0600); err != nil {
		os.Remove(backupPath)
		return "", fmt.Errorf("failed to write encrypted backup: %w", err)
	}

	// Remove unencrypted backup
	os.Remove(backupPath)

	return encryptedPath, nil
}

// RestoreEncrypted restores from an encrypted backup (.enc extension)
// Per AI.md PART 21: .enc extension for encrypted backups
func (m *Manager) RestoreEncrypted(backupPath string) error {
	if m.password == "" {
		return fmt.Errorf("decryption password not set - use SetPassword() or BACKUP_PASSWORD env var")
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Check if file has .enc extension
	if !strings.HasSuffix(backupPath, ".enc") {
		// Not encrypted, use regular restore
		return m.Restore(backupPath)
	}

	// Read encrypted data
	encrypted, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("failed to read encrypted backup: %w", err)
	}

	// Decrypt the data
	decrypted, err := DecryptBackup(encrypted, m.password)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	// Write to temporary file
	tempPath := backupPath + ".temp"
	if err := os.WriteFile(tempPath, decrypted, 0600); err != nil {
		return fmt.Errorf("failed to write decrypted backup: %w", err)
	}
	defer os.Remove(tempPath)

	// Restore from temporary file
	return m.Restore(tempPath)
}

// IsEncrypted checks if a backup file is encrypted (has .enc extension)
// Per AI.md PART 21: .enc extension for encrypted backups
func IsEncrypted(backupPath string) bool {
	return strings.HasSuffix(backupPath, ".enc")
}

// VerificationResult contains the results of backup verification
// Per AI.md PART 21: Backup verification is NON-NEGOTIABLE
type VerificationResult struct {
	FileExists    bool `json:"file_exists"`
	SizeValid     bool `json:"size_valid"`
	ChecksumValid bool `json:"checksum_valid"`
	ManifestValid bool `json:"manifest_valid"`
	ContentValid  bool `json:"content_valid"`
	DatabaseValid bool `json:"database_valid"`
	// Only for encrypted backups
	DecryptValid bool     `json:"decrypt_valid"`
	AllPassed    bool     `json:"all_passed"`
	Errors       []string `json:"errors,omitempty"`
}

// VerifyBackup verifies backup integrity immediately after creation
// Per AI.md PART 21 (NON-NEGOTIABLE):
// - File exists
// - Size > 0
// - Checksum valid
// - Manifest readable
// - Content extraction (test extract all files to temp dir)
// - Database integrity (verify SQLite is valid, if present)
// - Decrypt test (if encrypted)
func (m *Manager) VerifyBackup(backupPath string) (*VerificationResult, error) {
	result := &VerificationResult{
		// Default true for non-encrypted
		DecryptValid: true,
	}

	// Check 1: File exists
	info, err := os.Stat(backupPath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("file not found: %v", err))
		return result, nil
	}
	result.FileExists = true

	// Check 2: Size > 0
	if info.Size() == 0 {
		result.Errors = append(result.Errors, "backup file is empty (size = 0)")
	} else {
		result.SizeValid = true
	}

	// Check 3 & 4: For encrypted backups, decrypt first
	isEncrypted := IsEncrypted(backupPath)
	var dataToVerify []byte

	if isEncrypted {
		if m.password == "" {
			result.DecryptValid = false
			result.Errors = append(result.Errors, "cannot verify encrypted backup: password not set")
			return result, nil
		}

		encrypted, err := os.ReadFile(backupPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to read encrypted backup: %v", err))
			return result, nil
		}

		decrypted, err := DecryptBackup(encrypted, m.password)
		if err != nil {
			result.DecryptValid = false
			result.Errors = append(result.Errors, fmt.Sprintf("decryption failed: %v", err))
			return result, nil
		}
		result.DecryptValid = true
		dataToVerify = decrypted
	} else {
		var err error
		dataToVerify, err = os.ReadFile(backupPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("failed to read backup: %v", err))
			return result, nil
		}
	}

	// Check 3: Manifest readable
	metadata, err := m.verifyManifestFromData(dataToVerify)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("manifest verification failed: %v", err))
	} else {
		result.ManifestValid = true

		// Check 4: Checksum valid (verify stored checksum)
		if metadata.Checksum != "" && metadata.Checksums != nil {
			expectedChecksum := computeOverallChecksum(metadata.Checksums)
			storedChecksum := strings.TrimPrefix(metadata.Checksum, "sha256:")
			if expectedChecksum == storedChecksum {
				result.ChecksumValid = true
			} else {
				result.Errors = append(result.Errors, "checksum mismatch: backup may be corrupted")
			}
		} else {
			// Legacy backups without checksums - pass with warning
			result.ChecksumValid = true
		}
	}

	// Check 5 & 6: Content extraction and database integrity
	contentValid, databaseValid, contentErrs := m.verifyContentAndDatabase(dataToVerify)
	result.ContentValid = contentValid
	result.DatabaseValid = databaseValid
	result.Errors = append(result.Errors, contentErrs...)

	// Determine overall result
	result.AllPassed = result.FileExists && result.SizeValid && result.ChecksumValid && result.ManifestValid &&
		result.ContentValid && result.DatabaseValid && result.DecryptValid

	return result, nil
}

// verifyContentAndDatabase test-extracts every file in the backup archive to a
// temp dir and validates any embedded server.db as a well-formed SQLite file.
// Per AI.md PART 21: Content extraction and Database integrity are Fatal checks.
// Absence of a database file in the archive is not fatal (first-run/empty DB).
func (m *Manager) verifyContentAndDatabase(data []byte) (contentValid bool, databaseValid bool, errs []string) {
	contentValid = true
	databaseValid = true

	base := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(base, 0755); err != nil {
		return false, true, []string{fmt.Sprintf("content extraction failed: %v", err)}
	}
	extractDir, err := os.MkdirTemp(base, "search-verify-")
	if err != nil {
		return false, true, []string{fmt.Sprintf("content extraction failed: %v", err)}
	}
	defer os.RemoveAll(extractDir)

	reader := bytes.NewReader(data)
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return false, true, []string{fmt.Sprintf("content extraction failed: invalid gzip format: %v", err)}
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			contentValid = false
			errs = append(errs, fmt.Sprintf("content extraction failed: error reading tar: %v", err))
			break
		}

		cleanName := filepath.Clean(header.Name)
		if cleanName == ".." || strings.HasPrefix(cleanName, "../") || strings.HasPrefix(cleanName, string(os.PathSeparator)) {
			contentValid = false
			errs = append(errs, fmt.Sprintf("content extraction failed: unsafe path in archive: %s", header.Name))
			continue
		}
		targetPath := filepath.Join(extractDir, cleanName)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
				continue
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
			if err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
				continue
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
			}
			out.Close()

			if filepath.Base(cleanName) == "server.db" {
				if !isValidSQLiteFile(targetPath) {
					databaseValid = false
					errs = append(errs, "database integrity check failed: server.db is not a valid SQLite file")
				}
			}
		}
	}

	return contentValid, databaseValid, errs
}

// isValidSQLiteFile checks whether the file at path begins with the SQLite
// format magic header.
func isValidSQLiteFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	header := make([]byte, 16)
	n, err := f.Read(header)
	if err != nil || n < 16 {
		return false
	}
	return string(header) == "SQLite format 3\x00"
}

// verifyManifestFromData extracts and parses manifest from backup data
func (m *Manager) verifyManifestFromData(data []byte) (*BackupMetadata, error) {
	reader := bytes.NewReader(data)
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("invalid gzip format: %w", err)
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("manifest.json not found in backup")
		}
		if err != nil {
			return nil, fmt.Errorf("error reading tar: %w", err)
		}

		if header.Name == "manifest.json" || header.Name == "backup.json" {
			var metadata BackupMetadata
			if err := json.NewDecoder(tarReader).Decode(&metadata); err != nil {
				return nil, fmt.Errorf("invalid manifest JSON: %w", err)
			}
			return &metadata, nil
		}
	}
}

// CreateAndVerify creates a backup and verifies it immediately
// Per AI.md PART 21: Only delete old backups if new backup passes ALL verification checks
func (m *Manager) CreateAndVerify(filename string) (string, *VerificationResult, error) {
	// Create the backup
	backupPath, err := m.Create(filename)
	if err != nil {
		return "", nil, fmt.Errorf("backup creation failed: %w", err)
	}

	// Verify immediately
	result, err := m.VerifyBackup(backupPath)
	if err != nil {
		// Verification process failed - delete the backup
		os.Remove(backupPath)
		return "", nil, fmt.Errorf("backup verification process failed: %w", err)
	}

	if !result.AllPassed {
		// Verification failed - delete the failed backup per AI.md PART 21
		os.Remove(backupPath)
		return "", result, fmt.Errorf("backup verification failed: %v", result.Errors)
	}

	return backupPath, result, nil
}

// CreateEncryptedAndVerify creates an encrypted backup and verifies it
// Per AI.md PART 21: All encrypted backups must pass decrypt test
func (m *Manager) CreateEncryptedAndVerify(filename string) (string, *VerificationResult, error) {
	// Create encrypted backup
	backupPath, err := m.CreateEncrypted(filename)
	if err != nil {
		return "", nil, fmt.Errorf("encrypted backup creation failed: %w", err)
	}

	// Verify immediately (includes decrypt test)
	result, err := m.VerifyBackup(backupPath)
	if err != nil {
		os.Remove(backupPath)
		return "", nil, fmt.Errorf("backup verification process failed: %w", err)
	}

	if !result.AllPassed {
		os.Remove(backupPath)
		return "", result, fmt.Errorf("backup verification failed: %v", result.Errors)
	}

	return backupPath, result, nil
}

// ScheduledBackupWithVerification performs a scheduled backup with verification
// Per AI.md PART 21: Only delete old backups if new backup passes ALL verification checks
func (m *Manager) ScheduledBackupWithVerification(keepCount int) error {
	// Create and verify new backup
	backupPath, result, err := m.CreateAndVerify("")
	if err != nil {
		// Backup or verification failed - DO NOT delete any existing backups
		return fmt.Errorf("scheduled backup failed (existing backups preserved): %w", err)
	}

	if !result.AllPassed {
		// Verification failed - DO NOT delete any existing backups
		return fmt.Errorf("backup verification failed (existing backups preserved): %v", result.Errors)
	}

	// Only now that verification passed, apply retention
	backups, err := m.List()
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	// Delete old backups if we have more than keepCount
	if len(backups) > keepCount {
		for i := keepCount; i < len(backups); i++ {
			// Never delete the backup we just created
			if backups[i].Path == backupPath {
				continue
			}
			if err := m.Delete(backups[i].Filename); err != nil {
				slog.Warn("failed to delete old backup", "file", backups[i].Filename, "err", err)
			}
		}
	}

	return nil
}
