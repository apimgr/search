package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// BackupMetadata contains information about a backup
// Per AI.md PART 25: manifest.json format with required fields
type BackupMetadata struct {
	Version          string            `json:"version"`           // Manifest format version (e.g., "1.0.0")
	CreatedAt        time.Time         `json:"created_at"`        // When backup was created
	CreatedBy        string            `json:"created_by"`        // Who created the backup (per PART 25)
	AppVersion       string            `json:"app_version"`       // Application version (per PART 25)
	Contents         []string          `json:"contents"`          // List of files/directories in backup
	Checksums        map[string]string `json:"checksums"`         // SHA256 checksums per file
	Checksum         string            `json:"checksum"`          // Overall archive checksum (per PART 25)
	Encrypted        bool              `json:"encrypted"`         // Per AI.md PART 25
	EncryptionMethod string            `json:"encryption_method"` // "AES-256-GCM" if encrypted
	// Legacy fields for backwards compatibility
	ServerTitle string `json:"server_title,omitempty"` // Server title (optional)
	Size        int64  `json:"size,omitempty"`         // Total size in bytes (optional)
	Files       []string `json:"files,omitempty"`      // Deprecated: use Contents
}

// Manager handles backup and restore operations
type Manager struct {
	backupDir string
	configDir string
	dataDir   string
	password  string // Backup encryption password (never stored on disk)
	createdBy string // Username of who created the backup (per PART 25)
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
		backupDir: config.GetBackupDir(),
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
	// Per AI.md PART 22: Format is apimgr_backup_YYYY-MM-DD_HHMMSS.tar.gz
	if filename == "" {
		filename = fmt.Sprintf("apimgr_backup_%s.tar.gz", time.Now().Format("2006-01-02_150405"))
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
		createdBy = "system" // Default for CLI/scheduled backups
	}

	// Calculate overall checksum from individual file checksums (per AI.md PART 25)
	// This provides a single verification value for the entire backup
	overallChecksum := computeOverallChecksum(checksums)

	// Create metadata per AI.md PART 25: manifest.json format
	metadata := BackupMetadata{
		Version:     "1.0.0",                     // Manifest format version
		CreatedAt:   time.Now(),
		CreatedBy:   createdBy,                   // Per PART 25: who created the backup
		AppVersion:  config.Version,              // Per PART 25: application version
		Contents:    files,                       // Per PART 25: list of contents
		Checksums:   checksums,
		Checksum:    "sha256:" + overallChecksum, // Per PART 25: overall checksum
		ServerTitle: serverTitle,                 // Legacy/optional
		Size:        totalSize,                   // Legacy/optional
		Files:       files,                       // Legacy/deprecated: use Contents
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
		fmt.Printf("Warning: could not create pre-restore backup: %v\n", err)
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
			continue // Skip unknown paths
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
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
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
				fmt.Printf("Warning: failed to delete old backup %s: %v\n", backups[i].Filename, err)
			}
		}
	}

	return nil
}

// RetentionPolicy defines backup retention rules per AI.md PART 22
// Per AI.md PART 22: Retention policies (count, day, week, month, year)
type RetentionPolicy struct {
	Count int `json:"count" yaml:"count"` // Number of backups to keep
	Day   int `json:"day" yaml:"day"`     // Days to keep daily backups
	Week  int `json:"week" yaml:"week"`   // Weeks to keep weekly backups
	Month int `json:"month" yaml:"month"` // Months to keep monthly backups
	Year  int `json:"year" yaml:"year"`   // Years to keep yearly backups
}

// DefaultRetentionPolicy returns the default retention policy
// Per AI.md PART 22: Reasonable defaults
func DefaultRetentionPolicy() RetentionPolicy {
	return RetentionPolicy{
		Count: 10,  // Keep at least 10 backups
		Day:   7,   // Keep 7 days of daily backups
		Week:  4,   // Keep 4 weeks of weekly backups
		Month: 12,  // Keep 12 months of monthly backups
		Year:  3,   // Keep 3 years of yearly backups
	}
}

// ApplyRetention applies retention policy to backups
// Per AI.md PART 22: Smart retention with daily/weekly/monthly/yearly buckets
func (m *Manager) ApplyRetention(policy RetentionPolicy) error {
	backups, err := m.List()
	if err != nil {
		return err
	}

	if len(backups) <= policy.Count {
		return nil // Nothing to delete
	}

	now := time.Now()
	var toDelete []string

	// Categorize backups by age
	for _, b := range backups {
		age := now.Sub(b.CreatedAt)

		// Determine if backup should be kept based on retention rules
		keep := false

		// Keep all backups from today
		if age < 24*time.Hour {
			keep = true
		}

		// Keep daily backups for Day days
		if age < time.Duration(policy.Day)*24*time.Hour {
			keep = true
		}

		// Keep weekly backups (one per week) for Week weeks
		if age < time.Duration(policy.Week)*7*24*time.Hour {
			if b.CreatedAt.Weekday() == time.Sunday {
				keep = true
			}
		}

		// Keep monthly backups (first of month) for Month months
		if age < time.Duration(policy.Month)*30*24*time.Hour {
			if b.CreatedAt.Day() == 1 {
				keep = true
			}
		}

		// Keep yearly backups (first of year) for Year years
		if age < time.Duration(policy.Year)*365*24*time.Hour {
			if b.CreatedAt.Month() == time.January && b.CreatedAt.Day() == 1 {
				keep = true
			}
		}

		if !keep {
			toDelete = append(toDelete, b.Filename)
		}
	}

	// Delete old backups
	for _, filename := range toDelete {
		if err := m.Delete(filename); err != nil {
			fmt.Printf("Warning: failed to delete old backup %s: %v\n", filename, err)
		}
	}

	return nil
}

// CreateEncrypted creates an encrypted backup with .enc extension
// Per AI.md PART 22: .enc extension for encrypted backups
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

	// Create encrypted file with .enc extension per AI.md PART 22
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
// Per AI.md PART 22: .enc extension for encrypted backups
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
// Per AI.md PART 22: .enc extension for encrypted backups
func IsEncrypted(backupPath string) bool {
	return strings.HasSuffix(backupPath, ".enc")
}
