package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/apimgr/search/src/config"
)

// BackupMetadata contains information about a backup
// Per AI.md PART 25: manifest.json format with required fields
type BackupMetadata struct {
	Type         string               `json:"type,omitempty"`
	SnapshotAt   time.Time            `json:"snapshot_at,omitempty"`
	SHA256       string               `json:"sha256,omitempty"`
	FileManifest []BackupFileMetadata `json:"file_manifest,omitempty"`
	BaseBackup   string               `json:"base_backup,omitempty"`
	BaseSHA256   string               `json:"base_sha256,omitempty"`
	Deleted      []string             `json:"deleted,omitempty"`
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

// BackupFileMetadata stores the source state represented by one archive entry.
type BackupFileMetadata struct {
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
	SHA256     string    `json:"sha256"`
}

// Manager handles backup and restore operations
type Manager struct {
	backupDir   string
	configDir   string
	dataDir     string
	databaseDir string
	includeSSL  bool
	includeData bool
	serverDB    interface {
		SnapshotTo(context.Context, string) error
	}
	beforeRestore func() error
	// Backup encryption password (never stored on disk)
	password string
	// Username of who created the backup (per PART 25)
	createdBy     string
	restoreLimits RestoreLimits
}

// SetIncludeSSL controls whether the optional config/ssl directory is archived.
func (m *Manager) SetIncludeSSL(include bool) { m.includeSSL = include }

// SetIncludeData controls whether optional data files are archived.
func (m *Manager) SetIncludeData(include bool) { m.includeData = include }

// RestoreLimits bounds compressed archives and their cumulative extraction.
type RestoreLimits struct {
	MaxArchiveSize  int64
	MaxExpandedSize int64
	MaxFiles        int64
	MaxFileSize     int64
	MaxRatio        int64
}

func defaultRestoreLimits() RestoreLimits {
	return RestoreLimits{MaxArchiveSize: 1 << 30, MaxExpandedSize: 4 << 30, MaxFiles: 10000, MaxFileSize: 512 << 20, MaxRatio: 100}
}

// RestoreLimitsFromConfig resolves server.restore values with AI.md defaults.
func RestoreLimitsFromConfig(cfg config.RestoreConfig) (RestoreLimits, error) {
	defaults := defaultRestoreLimits()
	archive, err := parseRestoreSize(cfg.MaxArchiveSize, defaults.MaxArchiveSize)
	if err != nil {
		return RestoreLimits{}, fmt.Errorf("max_archive_size: %w", err)
	}
	expanded, err := parseRestoreSize(cfg.MaxExpandedSize, defaults.MaxExpandedSize)
	if err != nil {
		return RestoreLimits{}, fmt.Errorf("max_expanded_size: %w", err)
	}
	fileSize, err := parseRestoreSize(cfg.MaxFileSize, defaults.MaxFileSize)
	if err != nil {
		return RestoreLimits{}, fmt.Errorf("max_file_size: %w", err)
	}
	limits := defaults
	limits.MaxArchiveSize, limits.MaxExpandedSize, limits.MaxFileSize = archive, expanded, fileSize
	if cfg.MaxFiles > 0 {
		limits.MaxFiles = int64(cfg.MaxFiles)
	}
	if cfg.MaxRatio > 0 {
		limits.MaxRatio = int64(cfg.MaxRatio)
	}
	return limits, nil
}

func parseRestoreSize(value string, fallback int64) (int64, error) {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "" {
		return fallback, nil
	}
	multiplier := int64(1)
	for _, unit := range []struct {
		suffix     string
		multiplier int64
	}{{"KB", 1 << 10}, {"K", 1 << 10}, {"MB", 1 << 20}, {"M", 1 << 20}, {"GB", 1 << 30}, {"G", 1 << 30}} {
		if strings.HasSuffix(value, unit.suffix) {
			multiplier = unit.multiplier
			value = strings.TrimSpace(strings.TrimSuffix(value, unit.suffix))
			break
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n < 1 || n > int64(^uint64(0)>>1)/multiplier {
		return 0, fmt.Errorf("invalid positive size %q", value)
	}
	return n * multiplier, nil
}

// SetRestoreLimits overrides the restore limits with server.restore settings.
func (m *Manager) SetRestoreLimits(limits RestoreLimits) { m.restoreLimits = limits }

func (m *Manager) effectiveRestoreLimits() RestoreLimits {
	limits := m.restoreLimits
	defaults := defaultRestoreLimits()
	if limits.MaxArchiveSize <= 0 {
		limits.MaxArchiveSize = defaults.MaxArchiveSize
	}
	if limits.MaxExpandedSize <= 0 {
		limits.MaxExpandedSize = defaults.MaxExpandedSize
	}
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = defaults.MaxFiles
	}
	if limits.MaxFileSize <= 0 {
		limits.MaxFileSize = defaults.MaxFileSize
	}
	if limits.MaxRatio <= 0 {
		limits.MaxRatio = defaults.MaxRatio
	}
	return limits
}

var (
	ErrNoFullBackup = errors.New("no full backup is available as an incremental base")
	ErrBaseMissing  = errors.New("ERR_BASE_MISSING")
	ErrBaseCorrupt  = errors.New("ERR_BASE_CORRUPT")
)

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
		backupDir:   resolveBackupDir(),
		configDir:   config.GetConfigDir(),
		dataDir:     config.GetDataDir(),
		databaseDir: config.GetDatabaseDir(),
	}
}

// SetServerDatabaseSnapshotter supplies the live server database's write
// handle so backups use SQLite's online snapshot operation.
func (m *Manager) SetServerDatabaseSnapshotter(db interface {
	SnapshotTo(context.Context, string) error
},
) {
	m.serverDB = db
}

// SetBeforeRestore installs a hook that closes database handles after the
// pre-restore backup is captured and before database directories are swapped.
func (m *Manager) SetBeforeRestore(hook func() error) {
	m.beforeRestore = hook
}

func (m *Manager) databaseIsWithinData() (string, bool) {
	rel, err := filepath.Rel(m.dataDir, m.databaseDir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

func (m *Manager) databaseArchivePath() string {
	if rel, ok := m.databaseIsWithinData(); ok {
		return filepath.ToSlash(filepath.Join("data", rel, "server.db"))
	}
	return "database/server.db"
}

// normalizeBackupFilename resolves the on-disk archive name for a requested
// backup filename. An empty name becomes the PART 21 timestamped manual format
// (search_backup_YYYY-MM-DD_HHMMSS.tar.gz); a caller-supplied ".enc" suffix is
// dropped because encryption appends it, and ".tar.gz" is always ensured so
// names like "search-daily.tar.gz.enc" never become
// "search-daily.tar.gz.enc.tar.gz.enc".
func normalizeBackupFilename(filename string) string {
	if filename == "" {
		return fmt.Sprintf("search_backup_%s.tar.gz", time.Now().Format("2006-01-02_150405"))
	}

	filename = strings.TrimSuffix(filename, ".enc")
	if !strings.HasSuffix(filename, ".tar.gz") {
		filename += ".tar.gz"
	}
	return filename
}

// complianceRequiresEncryption reports whether compliance mode is enabled for
// this server. Per AI.md PART 21 an unencrypted backup is refused in that mode.
func (m *Manager) complianceRequiresEncryption() bool {
	cfg, err := config.Load(filepath.Join(m.configDir, "server.yml"))
	if err != nil || cfg == nil {
		return false
	}
	return cfg.Server.Compliance.AnyEnabled()
}

// writeFileAtomic writes data to path crash-safely: the bytes land in a temp
// file in the same directory, are flushed to stable storage, and only then
// replace path with a rename. A partially written file is never visible at path.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	cleanup := func() {
		if err := tmp.Close(); err != nil {
			slog.Warn("failed to close backup temp file", "error", err)
		}
		if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to remove backup temp file", "error", err)
		}
	}

	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return fmt.Errorf("failed to set permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("failed to write file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("failed to flush file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			slog.Warn("failed to remove backup temp file", "error", removeErr)
		}
		return fmt.Errorf("failed to close file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if removeErr := os.Remove(tmpPath); removeErr != nil && !os.IsNotExist(removeErr) {
			slog.Warn("failed to remove backup temp file", "error", removeErr)
		}
		return fmt.Errorf("failed to finalize file: %w", err)
	}

	return nil
}

// Create creates a new backup archive
// Per AI.md PART 21: the archive is built in memory and written crash-safely;
// under compliance mode an unencrypted backup is refused outright.
func (m *Manager) Create(filename string) (string, error) {
	return m.CreateAt(filename, time.Now().UTC())
}

// CreateAt creates a full backup that represents snapshotAt.
func (m *Manager) CreateAt(filename string, snapshotAt time.Time) (string, error) {
	if m.password == "" && m.complianceRequiresEncryption() {
		return "", fmt.Errorf("compliance mode requires encrypted backups: set a backup password before creating a backup")
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	filename = normalizeBackupFilename(filename)
	backupPath := filepath.Join(m.backupDir, filename)

	archive, err := m.buildArchiveAt(snapshotAt)
	if err != nil {
		return "", err
	}

	if err := writeFileAtomic(backupPath, archive, 0o600); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupPath, nil
}

func (m *Manager) buildArchiveAt(snapshotAt time.Time) ([]byte, error) {
	var buf bytes.Buffer

	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	var totalSize int64
	var files []string
	checksums := make(map[string]string)

	databaseSnapshot := ""
	if m.serverDB != nil {
		base := filepath.Join(os.TempDir(), config.InternalOrg)
		if err := os.MkdirAll(base, 0o700); err != nil {
			return nil, fmt.Errorf("create backup snapshot temp root: %w", err)
		}
		snapshotDir, err := os.MkdirTemp(base, config.InternalName+"-")
		if err != nil {
			return nil, fmt.Errorf("create backup snapshot temp directory: %w", err)
		}
		defer func() {
			if err := os.RemoveAll(snapshotDir); err != nil {
				slog.Warn("failed to remove backup snapshot temp directory", "path", snapshotDir, "err", err)
			}
		}()
		databaseSnapshot = filepath.Join(snapshotDir, "server.db")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		err = m.serverDB.SnapshotTo(ctx, databaseSnapshot)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("snapshot server database: %w", err)
		}
	}

	// Load configuration so referenced TLS files and external manual server
	// certificates can be included even when the optional SSL directory is not.
	cfg := config.DefaultConfig()
	loadedConfig, err := config.Load(filepath.Join(m.configDir, "server.yml"))
	if err == nil {
		cfg = loadedConfig
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("load configuration for backup contents: %w", err)
	}

	// Backup config directory. The SSL directory is opt-in; individually
	// referenced connection TLS files are added below.
	configFiles, configSize, configChecksums, err := m.addDirectoryToTar(tarWriter, m.configDir, "config")
	if err != nil {
		return nil, fmt.Errorf("failed to backup config: %w", err)
	}
	files = append(files, configFiles...)
	totalSize += configSize
	for k, v := range configChecksums {
		checksums[k] = v
	}

	// Data files are optional; the database snapshot above remains included.
	if m.includeData {
		dataFiles, dataSize, dataChecksums, err := m.addDirectoryToTar(tarWriter, m.dataDir, "data")
		if err != nil {
			return nil, fmt.Errorf("failed to backup data: %w", err)
		}
		files = append(files, dataFiles...)
		totalSize += dataSize
		for k, v := range dataChecksums {
			checksums[k] = v
		}
	}
	extraSources := make(map[string]string)
	if err := m.addReferencedSSLFiles(tarWriter, cfg, &files, checksums, &totalSize, extraSources); err != nil {
		return nil, fmt.Errorf("backup referenced TLS files: %w", err)
	}
	if databaseSnapshot != "" {
		archiveName := m.databaseArchivePath()
		name, size, checksum, err := addFileToTar(tarWriter, databaseSnapshot, archiveName)
		if err != nil {
			return nil, fmt.Errorf("add server database snapshot: %w", err)
		}
		files = append(files, name)
		totalSize += size
		checksums[name] = checksum
	}

	serverTitle := "Search"
	serverTitle = cfg.Server.Title

	// Determine created_by (per AI.md PART 25)
	createdBy := m.createdBy
	if createdBy == "" {
		// Default for CLI/scheduled backups
		createdBy = "system"
	}

	// Calculate overall checksum from individual file checksums (per AI.md PART 25)
	// This provides a single verification value for the entire backup
	overallChecksum := computeOverallChecksum(checksums)
	fileManifest, err := m.fileManifest(files, checksums, databaseSnapshot, extraSources)
	if err != nil {
		return nil, fmt.Errorf("failed to build file manifest: %w", err)
	}
	snapshotAt = snapshotAt.UTC().Truncate(time.Second)

	// Create metadata per AI.md PART 25: manifest.json format
	metadata := BackupMetadata{
		Type:         "full",
		SnapshotAt:   snapshotAt,
		SHA256:       overallChecksum,
		FileManifest: fileManifest,
		// Manifest format version
		Version:   "1.0.0",
		CreatedAt: snapshotAt,
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
	metaJSON, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode manifest: %w", err)
	}
	metaHeader := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(metaJSON)),
		Mode:    0o644,
		ModTime: snapshotAt,
	}
	if err := tarWriter.WriteHeader(metaHeader); err != nil {
		return nil, fmt.Errorf("failed to write metadata header: %w", err)
	}
	if _, err := tarWriter.Write(metaJSON); err != nil {
		return nil, fmt.Errorf("failed to write metadata: %w", err)
	}

	// Close writers explicitly so flush errors surface instead of producing a
	// silently truncated archive
	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize tar archive: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize gzip stream: %w", err)
	}

	return buf.Bytes(), nil
}

func (m *Manager) fileManifest(files []string, checksums map[string]string, databaseSnapshot string, extraSources map[string]string) ([]BackupFileMetadata, error) {
	manifest := make([]BackupFileMetadata, 0, len(files))
	for _, name := range files {
		var sourcePath string
		switch {
		case strings.HasPrefix(name, "config/"):
			sourcePath = filepath.Join(m.configDir, filepath.FromSlash(strings.TrimPrefix(name, "config/")))
		case databaseSnapshot != "" && filepath.ToSlash(name) == m.databaseArchivePath():
			sourcePath = databaseSnapshot
		case extraSources[name] != "":
			sourcePath = extraSources[name]
		case strings.HasPrefix(name, "data/"):
			sourcePath = filepath.Join(m.dataDir, filepath.FromSlash(strings.TrimPrefix(name, "data/")))
		default:
			return nil, fmt.Errorf("unexpected backup path %q", name)
		}
		info, err := os.Stat(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", name, err)
		}
		manifest = append(manifest, BackupFileMetadata{
			Path:       filepath.ToSlash(name),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC(),
			SHA256:     checksums[name],
		})
	}
	sort.Slice(manifest, func(i, j int) bool { return manifest[i].Path < manifest[j].Path })
	return manifest, nil
}

func (m *Manager) addReferencedSSLFiles(tw *tar.Writer, cfg *config.Config, files *[]string, checksums map[string]string, totalSize *int64, extraSources map[string]string) error {
	sslRoot, err := filepath.Abs(filepath.Join(m.configDir, "ssl"))
	if err != nil {
		return err
	}
	addReferenced := func(path, archiveName string, private bool) error {
		if _, exists := checksums[filepath.ToSlash(archiveName)]; exists {
			return nil
		}
		if strings.TrimSpace(path) == "" {
			return nil
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(sslRoot, path)
		}
		resolved, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		name, size, digest, err := addFileToTarWithMode(tw, resolved, archiveName, private)
		if err != nil {
			return fmt.Errorf("include %s: %w", archiveName, err)
		}
		*files = append(*files, name)
		checksums[name] = digest
		*totalSize += size
		extraSources[name] = resolved
		return nil
	}

	// Connection TLS files are always included when referenced, independent of
	// the optional whole-SSL-directory flag.
	for _, path := range []string{cfg.Server.Cache.TLSCA, cfg.Server.Cache.TLSCert, cfg.Server.Cache.TLSKey} {
		if path == "" {
			continue
		}
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(sslRoot, resolved)
		}
		rel, relErr := filepath.Rel(sslRoot, resolved)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
			return fmt.Errorf("connection TLS file %q must be inside config ssl directory", path)
		}
		if err := addReferenced(path, filepath.ToSlash(filepath.Join("config", "ssl", rel)), false); err != nil {
			return err
		}
	}

	certPath, keyPath := cfg.Server.SSL.CertFile, cfg.Server.SSL.KeyFile
	if certPath == "" && keyPath == "" {
		return nil
	}
	if certPath == "" || keyPath == "" {
		return fmt.Errorf("server.ssl.cert and server.ssl.key must be configured together")
	}
	resolveServerSSL := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(m.configDir, path)
	}
	certResolved, err := filepath.Abs(resolveServerSSL(certPath))
	if err != nil {
		return err
	}
	keyResolved, err := filepath.Abs(resolveServerSSL(keyPath))
	if err != nil {
		return err
	}
	certRel, certErr := filepath.Rel(sslRoot, certResolved)
	keyRel, keyErr := filepath.Rel(sslRoot, keyResolved)
	certInside := certErr == nil && certRel != ".." && !strings.HasPrefix(certRel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(certRel)
	keyInside := keyErr == nil && keyRel != ".." && !strings.HasPrefix(keyRel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(keyRel)
	if certInside && keyInside {
		return nil
	}
	if err := addReferenced(certResolved, "ssl-external/server.crt", false); err != nil {
		return err
	}
	if err := addReferenced(keyResolved, "ssl-external/server.key", true); err != nil {
		return err
	}
	return nil
}

func addFileToTarWithMode(tw *tar.Writer, sourcePath, archivePath string, private bool) (string, int64, string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", 0, "", err
	}
	if !info.Mode().IsRegular() {
		return "", 0, "", fmt.Errorf("not a regular file")
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return "", 0, "", err
	}
	header.Name = filepath.ToSlash(archivePath)
	if private {
		header.Mode = 0o600
	}
	if err := tw.WriteHeader(header); err != nil {
		return "", 0, "", err
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return "", 0, "", err
	}
	hash := sha256.New()
	written, copyErr := io.CopyN(io.MultiWriter(tw, hash), file, header.Size)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", written, "", err
	}
	return header.Name, written, hex.EncodeToString(hash.Sum(nil)), nil
}

// CreateIncrementalAt writes a cumulative differential against the newest full backup.
func (m *Manager) CreateIncrementalAt(filename string, snapshotAt time.Time) (string, error) {
	if m.password == "" && m.complianceRequiresEncryption() {
		return "", fmt.Errorf("compliance mode requires encrypted backups: set a backup password before creating a backup")
	}
	basePath, base, err := m.latestFullBackup()
	if err != nil {
		return "", err
	}
	baseSHA, err := hashFile(basePath)
	if err != nil {
		return "", fmt.Errorf("hash incremental base: %w", err)
	}
	archive, metadata, err := m.buildIncrementalArchive(snapshotAt, base, filepath.Base(basePath), baseSHA)
	if err != nil {
		return "", err
	}
	_ = metadata
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}
	filename = normalizeBackupFilename(filename)
	path := filepath.Join(m.backupDir, filename)
	if err := writeFileAtomic(path, archive, 0o600); err != nil {
		return "", fmt.Errorf("failed to write incremental backup: %w", err)
	}
	return path, nil
}

// CreateIncremental creates the fixed daily or hourly cumulative incremental.
func (m *Manager) CreateIncremental(filename string) (string, error) {
	return m.CreateIncrementalAt(filename, time.Now().UTC())
}

// CreateEncryptedIncrementalAt writes an encrypted cumulative differential.
func (m *Manager) CreateEncryptedIncrementalAt(filename string, snapshotAt time.Time) (string, error) {
	if m.password == "" {
		return "", fmt.Errorf("encryption password not set; configure server.backup.encryption_password")
	}
	basePath, base, err := m.latestFullBackup()
	if err != nil {
		return "", err
	}
	baseSHA, err := hashFile(basePath)
	if err != nil {
		return "", fmt.Errorf("hash incremental base: %w", err)
	}
	archive, metadata, err := m.buildIncrementalArchive(snapshotAt, base, filepath.Base(basePath), baseSHA)
	if err != nil {
		return "", err
	}
	metadata.Encrypted = true
	metadata.EncryptionMethod = "AES-256-GCM"
	archive, err = replaceArchiveManifest(archive, metadata, nil)
	if err != nil {
		return "", err
	}
	encrypted, err := EncryptBackup(archive, m.password)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt incremental backup: %w", err)
	}
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}
	path := filepath.Join(m.backupDir, normalizeBackupFilename(filename)+".enc")
	if err := writeFileAtomic(path, encrypted, 0o600); err != nil {
		return "", fmt.Errorf("failed to write encrypted incremental backup: %w", err)
	}
	return path, nil
}

func (m *Manager) CreateEncryptedIncremental(filename string) (string, error) {
	return m.CreateEncryptedIncrementalAt(filename, time.Now().UTC())
}

func (m *Manager) latestFullBackup() (string, *BackupMetadata, error) {
	backups, err := m.List()
	if err != nil {
		return "", nil, err
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt.After(backups[j].CreatedAt) })
	for _, item := range backups {
		metadata, err := m.GetMetadata(item.Path)
		if err != nil || metadata == nil || metadata.Type == "incremental" {
			continue
		}
		if len(metadata.FileManifest) == 0 {
			continue
		}
		return item.Path, metadata, nil
	}
	return "", nil, ErrNoFullBackup
}

func (m *Manager) buildIncrementalArchive(snapshotAt time.Time, base *BackupMetadata, baseName, baseSHA string) ([]byte, *BackupMetadata, error) {
	fullArchive, err := m.buildArchiveAt(snapshotAt)
	if err != nil {
		return nil, nil, err
	}
	return m.buildIncrementalArchiveFromFull(fullArchive, base, baseName, baseSHA)
}

func (m *Manager) buildIncrementalArchiveFromFull(fullArchive []byte, base *BackupMetadata, baseName, baseSHA string) ([]byte, *BackupMetadata, error) {
	current, err := m.verifyManifestFromData(fullArchive)
	if err != nil {
		return nil, nil, err
	}
	baseFiles := make(map[string]BackupFileMetadata, len(base.FileManifest))
	for _, entry := range base.FileManifest {
		baseFiles[entry.Path] = entry
	}
	changed := make(map[string]bool)
	currentFiles := make(map[string]bool, len(current.FileManifest))
	changedManifest := make([]BackupFileMetadata, 0)
	checksums := make(map[string]string)
	var size int64
	for _, entry := range current.FileManifest {
		currentFiles[entry.Path] = true
		old, ok := baseFiles[entry.Path]
		if ok && old.Size == entry.Size && old.ModifiedAt.Equal(entry.ModifiedAt) && old.SHA256 == entry.SHA256 {
			continue
		}
		changed[entry.Path] = true
		changedManifest = append(changedManifest, entry)
		checksums[entry.Path] = entry.SHA256
		size += entry.Size
	}
	deleted := make([]string, 0)
	for _, entry := range base.FileManifest {
		if !currentFiles[entry.Path] {
			deleted = append(deleted, entry.Path)
		}
	}
	sort.Strings(deleted)
	current.Type = "incremental"
	current.BaseBackup = baseName
	current.BaseSHA256 = baseSHA
	current.Deleted = deleted
	current.FileManifest = changedManifest
	current.Contents = make([]string, 0, len(changedManifest))
	for _, entry := range changedManifest {
		current.Contents = append(current.Contents, entry.Path)
	}
	current.Files = append([]string(nil), current.Contents...)
	current.Checksums = checksums
	current.Checksum = "sha256:" + computeOverallChecksum(checksums)
	current.SHA256 = computeOverallChecksum(checksums)
	current.Size = size
	archive, err := replaceArchiveManifest(fullArchive, current, changed)
	if err != nil {
		return nil, nil, err
	}
	return archive, current, nil
}

func replaceArchiveManifest(data []byte, metadata *BackupMetadata, included map[string]bool) ([]byte, error) {
	var out bytes.Buffer
	gz := gzip.NewWriter(&out)
	tarOut := tar.NewWriter(gz)
	gzIn, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	tarIn := tar.NewReader(gzIn)
	for {
		header, err := tarIn.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = gzIn.Close()
			return nil, err
		}
		if header.Name == "manifest.json" || header.Name == "backup.json" {
			continue
		}
		if included != nil && !included[filepath.ToSlash(header.Name)] {
			continue
		}
		if err := tarOut.WriteHeader(header); err != nil {
			_ = gzIn.Close()
			return nil, err
		}
		if _, err := io.CopyN(tarOut, tarIn, header.Size); err != nil {
			_ = gzIn.Close()
			return nil, err
		}
	}
	if err := gzIn.Close(); err != nil {
		return nil, err
	}
	manifest, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := tarOut.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(manifest)), Mode: 0o644, ModTime: metadata.SnapshotAt}); err != nil {
		return nil, err
	}
	if _, err := tarOut.Write(manifest); err != nil {
		return nil, err
	}
	if err := tarOut.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func hashFile(path string) (checksum string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close file after hashing: %w", closeErr))
		}
	}()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// addDirectoryToTar adds a directory to the tar archive with SHA256 checksums
// Per AI.md PART 26: SHA256 checksums for all backup files
func (m *Manager) addDirectoryToTar(tw *tar.Writer, srcDir, prefix string) ([]string, int64, map[string]string, error) {
	var files []string
	var totalSize int64
	checksums := make(map[string]string)

	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip transient files that vanish mid-walk (e.g. a GeoIP
			// download .tmp renamed away between lstat and open); a
			// missing root directory still errors.
			if os.IsNotExist(err) && path != srcDir {
				slog.Warn("skipping vanished path in backup", "path", path, "err", err)
				return nil
			}
			return err
		}

		// Skip directories we don't want to backup
		if info.IsDir() {
			if prefix == "config" && filepath.Clean(path) == filepath.Join(filepath.Clean(srcDir), "ssl") && !m.includeSSL {
				return filepath.SkipDir
			}
			if prefix == "data" && filepath.Clean(path) != filepath.Clean(srcDir) && filepath.Clean(path) == filepath.Clean(m.databaseDir) {
				return filepath.SkipDir
			}
			name := info.Name()
			if name == "logs" || name == "cache" || name == "tmp" {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			slog.Warn("skipping symbolic link in backup", "path", path)
			return nil
		}
		if !info.Mode().IsRegular() {
			slog.Warn("skipping non-regular file in backup", "path", path, "mode", info.Mode().String())
			return nil
		}
		if prefix == "data" && isSQLiteDatabaseArtifact(m.databaseDir, path) {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		// Create tar header
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join(prefix, relPath))

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		// Open file and compute checksum while copying
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		closed := false
		defer func() {
			if !closed {
				if err := file.Close(); err != nil {
					slog.Warn("failed to close backup file", "error", err)
				}
			}
		}()

		// Create a hash writer to compute SHA256 while copying
		hash := sha256.New()
		multiWriter := io.MultiWriter(tw, hash)

		written, err := io.CopyN(multiWriter, file, header.Size)
		closeErr := file.Close()
		closed = true
		if err != nil {
			return errors.Join(err, closeErr)
		}
		if closeErr != nil {
			return closeErr
		}

		files = append(files, header.Name)
		totalSize += written
		checksums[header.Name] = hex.EncodeToString(hash.Sum(nil))

		return nil
	})

	return files, totalSize, checksums, err
}

func isSQLiteDatabaseArtifact(databaseDir, path string) bool {
	rel, err := filepath.Rel(databaseDir, path)
	if err != nil || filepath.Dir(rel) != "." {
		return false
	}
	name := filepath.Base(rel)
	for _, base := range []string{"server.db", "user.db"} {
		if name == base || name == base+"-wal" || name == base+"-shm" {
			return true
		}
	}
	return false
}

// addFileToTar writes one file and records the checksum for its archive path.
func addFileToTar(tw *tar.Writer, sourcePath, archivePath string) (string, int64, string, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return "", 0, "", err
	}
	header, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return "", 0, "", err
	}
	header.Name = filepath.ToSlash(archivePath)
	if err := tw.WriteHeader(header); err != nil {
		return "", 0, "", err
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return "", 0, "", err
	}
	hash := sha256.New()
	written, copyErr := io.CopyN(io.MultiWriter(tw, hash), file, header.Size)
	closeErr := file.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return "", written, "", err
	}
	return header.Name, written, hex.EncodeToString(hash.Sum(nil)), nil
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

// Restore restores from a backup archive.
// Per AI.md PART 21 the backup is fully verified first and the restore itself
// is atomic: everything is staged beside the live directories and swapped in
// with renames, so a failure at any point leaves the previous state intact.
func (m *Manager) Restore(backupPath string) error {
	// Verify backup file exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Per AI.md PART 21 "Restore Verification": only proceed when every check
	// passes (format, decrypt, checksum, manifest, extraction, database)
	result, err := m.VerifyBackup(backupPath)
	if err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}
	if !result.AllPassed {
		return fmt.Errorf("backup verification failed: %v", result.Errors)
	}

	data, err := m.readArchive(backupPath)
	if err != nil {
		return err
	}
	metadata, err := m.verifyManifestFromData(data)
	if err != nil {
		return err
	}
	if metadata.Type != "incremental" {
		return m.restoreFromData(data)
	}
	basePath := filepath.Join(filepath.Dir(backupPath), metadata.BaseBackup)
	baseData, err := m.readArchive(basePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBaseCorrupt, err)
	}
	return m.restoreFromLayers([][]byte{baseData, data}, metadata.Deleted)
}

// readArchive loads a backup archive into memory, decrypting it first when the
// file carries the encrypted .enc extension. The decrypted archive is never
// written to disk.
func (m *Manager) readArchive(backupPath string) ([]byte, error) {
	info, err := os.Stat(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat backup file: %w", err)
	}
	if info.Size() > m.effectiveRestoreLimits().MaxArchiveSize {
		return nil, fmt.Errorf("restore max_archive_size exceeded: %d > %d", info.Size(), m.effectiveRestoreLimits().MaxArchiveSize)
	}
	raw, err := os.ReadFile(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup file: %w", err)
	}

	if !IsEncrypted(backupPath) {
		return raw, nil
	}

	if m.password == "" {
		return nil, fmt.Errorf("encrypted backup requires a password")
	}

	decrypted, err := DecryptBackup(raw, m.password)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}
	return decrypted, nil
}

// restoreTargetPath maps an archive entry to its destination inside targetRoot,
// rejecting any entry that would escape the destination directory (tar slip).
func restoreTargetPath(targetRoot, relPath string) (string, error) {
	slashPath := filepath.ToSlash(relPath)
	if slashPath == "" || strings.HasPrefix(slashPath, "/") || (len(slashPath) > 1 && slashPath[1] == ':') {
		return "", fmt.Errorf("unsafe path in archive: %s", relPath)
	}
	for _, segment := range strings.Split(slashPath, "/") {
		if segment == ".." || segment == "" {
			return "", fmt.Errorf("unsafe path in archive: %s", relPath)
		}
	}
	cleaned := filepath.Clean(filepath.FromSlash(slashPath))
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("empty path in archive")
	}

	target := filepath.Join(targetRoot, cleaned)
	rel, err := filepath.Rel(targetRoot, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path in archive: %s", relPath)
	}
	return target, nil
}

// restoreFromData performs the atomic restore of an in-memory archive.
// The current config and data directories are copied into staging directories,
// the archive contents are applied on top of the copies, and only once every
// entry has been written are the staging directories renamed into place. Any
// failure aborts before the first rename, and a failure between renames rolls
// the already-swapped directory back.
func (m *Manager) restoreFromData(data []byte) error {
	return m.restoreFromLayers([][]byte{data}, nil)
}

func (m *Manager) restoreFromLayers(archives [][]byte, deleted []string) error {
	// Create backup of current state before restoring
	preRestoreName := fmt.Sprintf("pre-restore-%s.tar.gz", time.Now().Format("20060102-150405"))
	var preRestoreErr error
	if m.password != "" {
		_, preRestoreErr = m.CreateEncrypted(preRestoreName)
	} else {
		_, preRestoreErr = m.Create(preRestoreName)
	}
	if preRestoreErr != nil {
		// Non-fatal, just warn
		slog.Warn("could not create pre-restore backup", "err", preRestoreErr)
	}
	if m.beforeRestore != nil {
		if err := m.beforeRestore(); err != nil {
			return fmt.Errorf("failed to close database before restore: %w", err)
		}
	}

	stamp := time.Now().Format("20060102-150405.000000000")
	targets := map[string]string{
		"config": m.configDir,
		"data":   m.dataDir,
	}
	if _, withinData := m.databaseIsWithinData(); !withinData && archivesContainServerDatabase(archives) {
		targets["database"] = m.databaseDir
	}
	staging := make(map[string]string, len(targets))

	// Restore stages begin empty so a full archive replaces the prior state.
	for prefix, dir := range targets {
		stage := fmt.Sprintf("%s.restore-%s", dir, stamp)
		if err := os.MkdirAll(stage, 0o700); err != nil {
			removeAll(staging)
			return fmt.Errorf("failed to create %s restore staging directory: %w", prefix, err)
		}
		staging[prefix] = stage
	}

	for _, data := range archives {
		if err := m.applyArchiveToStaging(data, staging); err != nil {
			removeAll(staging)
			return err
		}
	}
	if err := m.restoreExternalSSLFiles(staging["config"]); err != nil {
		removeAll(staging)
		return err
	}
	for _, name := range deleted {
		var root, rel string
		switch {
		case strings.HasPrefix(name, "config/"):
			root, rel = staging["config"], strings.TrimPrefix(name, "config/")
		case strings.HasPrefix(name, "data/"):
			root, rel = staging["data"], strings.TrimPrefix(name, "data/")
		case strings.HasPrefix(name, "database/") && staging["database"] != "":
			root, rel = staging["database"], strings.TrimPrefix(name, "database/")
		default:
			removeAll(staging)
			return fmt.Errorf("refusing to restore incremental: unsafe deleted path %q", name)
		}
		relPath := filepath.FromSlash(rel)
		if filepath.IsAbs(relPath) || filepath.Clean(relPath) != relPath || relPath == "." || relPath == ".." || (len(relPath) > 1 && relPath[1] == ':') || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
			removeAll(staging)
			return fmt.Errorf("refusing to restore incremental: unsafe deleted path %q", name)
		}
		if err := os.RemoveAll(filepath.Join(root, relPath)); err != nil {
			removeAll(staging)
			return fmt.Errorf("failed to apply deleted path %q: %w", name, err)
		}
	}

	// Swap staged directories into place, rolling back on failure
	var swapped []string
	replaced := make(map[string]string, len(targets))
	for prefix, dir := range targets {
		previous := fmt.Sprintf("%s.prerestore-%s", dir, stamp)
		if _, err := os.Stat(dir); err == nil {
			if err := os.Rename(dir, previous); err != nil {
				rollbackSwap(swapped, targets, replaced)
				removeAll(staging)
				return fmt.Errorf("failed to move current %s directory aside: %w", prefix, err)
			}
			replaced[prefix] = previous
		}
		if err := os.Rename(staging[prefix], dir); err != nil {
			if prev, ok := replaced[prefix]; ok {
				if rollbackErr := os.Rename(prev, dir); rollbackErr != nil {
					slog.Error("failed to roll back backup restore", "error", rollbackErr)
				}
				delete(replaced, prefix)
			}
			rollbackSwap(swapped, targets, replaced)
			removeAll(staging)
			return fmt.Errorf("failed to activate restored %s directory: %w", prefix, err)
		}
		swapped = append(swapped, prefix)
	}

	// Restore succeeded: drop the superseded directories
	for _, previous := range replaced {
		if err := os.RemoveAll(previous); err != nil {
			slog.Warn("failed to remove pre-restore directory", "path", previous, "err", err)
		}
	}

	return nil
}

func archivesContainServerDatabase(archives [][]byte) bool {
	for _, data := range archives {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			continue
		}
		tarReader := tar.NewReader(reader)
		for {
			header, err := tarReader.Next()
			if err != nil {
				break
			}
			if header.Name == "database/server.db" {
				_ = reader.Close()
				return true
			}
		}
		_ = reader.Close()
	}
	return false
}

// applyArchiveToStaging writes every archive entry into the matching staging
// directory. Manifest entries and entries outside the known prefixes are
// skipped, and unsafe paths abort the restore before anything is swapped in.
func (m *Manager) applyArchiveToStaging(data []byte, staging map[string]string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer func() {
		if err := gzReader.Close(); err != nil {
			slog.Warn("failed to close backup restore gzip reader", "error", err)
		}
	}()

	tarReader := tar.NewReader(gzReader)

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

		var stageRoot, relPath string
		switch {
		case strings.HasPrefix(header.Name, "config/"):
			stageRoot = staging["config"]
			relPath = strings.TrimPrefix(header.Name, "config/")
		case strings.HasPrefix(header.Name, "ssl-external/"):
			stageRoot = staging["config"]
			relPath = filepath.Join(".__restore_external_ssl", strings.TrimPrefix(header.Name, "ssl-external/"))
		case strings.HasPrefix(header.Name, "data/"):
			stageRoot = staging["data"]
			relPath = strings.TrimPrefix(header.Name, "data/")
		case strings.HasPrefix(header.Name, "database/") && staging["database"] != "":
			stageRoot = staging["database"]
			relPath = strings.TrimPrefix(header.Name, "database/")
		default:
			// Skip unknown paths
			continue
		}
		if header.Typeflag == tar.TypeDir {
			relPath = strings.TrimSuffix(relPath, "/")
		}

		targetPath, err := restoreTargetPath(stageRoot, relPath)
		if err != nil {
			return fmt.Errorf("refusing to restore backup: %w", err)
		}

		// Create parent directories
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
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
				return fmt.Errorf("failed to write file %s: %w", targetPath, errors.Join(err, outFile.Close()))
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("failed to write file %s: %w", targetPath, err)
			}
		}
	}

	return nil
}

func (m *Manager) restoreExternalSSLFiles(configStage string) error {
	externalDir := filepath.Join(configStage, ".__restore_external_ssl")
	certPath := filepath.Join(externalDir, "server.crt")
	keyPath := filepath.Join(externalDir, "server.key")
	_, certErr := os.Stat(certPath)
	_, keyErr := os.Stat(keyPath)
	if os.IsNotExist(certErr) && os.IsNotExist(keyErr) {
		return nil
	}
	if certErr != nil || keyErr != nil {
		return fmt.Errorf("backup contains an incomplete external TLS certificate pair")
	}
	sslDir := filepath.Join(configStage, "ssl")
	if err := os.MkdirAll(sslDir, 0o700); err != nil {
		return fmt.Errorf("create restored SSL directory: %w", err)
	}
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return fmt.Errorf("read archived external TLS certificate: %w", err)
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Errorf("read archived external TLS key: %w", err)
	}
	certTarget := filepath.Join(sslDir, "server.crt")
	keyTarget := filepath.Join(sslDir, "server.key")
	if err := writeFileAtomic(certTarget, certData, 0o644); err != nil {
		return fmt.Errorf("restore external TLS certificate: %w", err)
	}
	if err := writeFileAtomic(keyTarget, keyData, 0o600); err != nil {
		return fmt.Errorf("restore external TLS key: %w", err)
	}
	certTarget = filepath.Join(m.configDir, "ssl", "server.crt")
	keyTarget = filepath.Join(m.configDir, "ssl", "server.key")
	var certUpdated, keyUpdated bool
	err = filepath.WalkDir(configStage, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(path) != ".yml" {
			return nil
		}
		configData, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var document yaml.Node
		if parseErr := yaml.Unmarshal(configData, &document); parseErr != nil {
			return fmt.Errorf("parse restored %s for TLS path update: %w", filepath.Base(path), parseErr)
		}
		root := &document
		if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
			root = root.Content[0]
		}
		sslNode := mappingValue(mappingValue(root, "server"), "ssl")
		if sslNode == nil {
			return nil
		}
		changed := false
		if mappingValue(sslNode, "cert") != nil {
			if err := setMappingScalar(sslNode, "cert", certTarget); err != nil {
				return err
			}
			certUpdated, changed = true, true
		}
		if mappingValue(sslNode, "key") != nil {
			if err := setMappingScalar(sslNode, "key", keyTarget); err != nil {
				return err
			}
			keyUpdated, changed = true, true
		}
		if !changed {
			return nil
		}
		updated, marshalErr := yaml.Marshal(&document)
		if marshalErr != nil {
			return marshalErr
		}
		if writeErr := writeFileAtomic(path, updated, 0o600); writeErr != nil {
			return writeErr
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("rewrite restored server TLS paths: %w", err)
	}
	if !certUpdated || !keyUpdated {
		return fmt.Errorf("restored configuration does not contain server.ssl.cert and server.ssl.key mappings")
	}
	if err := os.RemoveAll(externalDir); err != nil {
		return fmt.Errorf("remove temporary external TLS restore files: %w", err)
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func setMappingScalar(node *yaml.Node, key, value string) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping for %s", key)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return nil
		}
	}
	return fmt.Errorf("mapping key %s not found", key)
}

// rollbackSwap undoes directory swaps already performed by a failed restore,
// putting the previous directories back where they were.
func rollbackSwap(swapped []string, targets map[string]string, replaced map[string]string) {
	for _, prefix := range swapped {
		dir := targets[prefix]
		previous, ok := replaced[prefix]
		if !ok {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			slog.Error("restore rollback failed to remove partial directory", "path", dir, "err", err)
			continue
		}
		if err := os.Rename(previous, dir); err != nil {
			slog.Error("restore rollback failed to restore directory", "path", dir, "err", err)
		}
	}
}

// removeAll deletes leftover staging directories, logging any failure.
func removeAll(paths map[string]string) {
	for _, path := range paths {
		if err := os.RemoveAll(path); err != nil {
			slog.Warn("failed to remove staging directory", "path", path, "err", err)
		}
	}
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
		if entry.IsDir() || (!strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tar.gz.enc")) {
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
	data, err := m.readArchive(backupPath)
	if err != nil {
		return nil, err
	}
	return m.verifyManifestFromData(data)
}

// Delete deletes a backup file
func (m *Manager) Delete(filename string) error {
	backupPath := filepath.Join(m.backupDir, filename)

	// Verify it's in the backup directory (security check): the resolved path
	// must stay inside the backup dir, so a traversing filename can never
	// delete anything outside it
	rel, err := filepath.Rel(m.backupDir, backupPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
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

// ScheduledBackup performs a scheduled backup with cleanup of old backups.
// Per AI.md PART 21 pruning goes through the retention buckets, which sort by
// date, delete oldest first, and never count the fixed-name incrementals.
func (m *Manager) ScheduledBackup(keepCount int) error {
	// Create new backup
	_, err := m.Create("")
	if err != nil {
		return err
	}

	return m.ApplyRetention(countOnlyRetention(keepCount))
}

// countOnlyRetention builds a retention policy from a bare keep-count, as used
// by the scheduled-backup helpers. Per AI.md PART 21 max_backups is at least 1;
// no weekly/monthly/yearly bucket and no size cap is implied by a count alone.
func countOnlyRetention(keepCount int) RetentionPolicy {
	if keepCount < 1 {
		keepCount = 1
	}
	return RetentionPolicy{
		Count: keepCount,
		// A bare keep-count carries no size cap
		MaxTotalSize: "0",
	}
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

	// Keep any full archive referenced by a currently retained incremental.
	for baseName := range m.retainedIncrementalBases() {
		kept[baseName] = true
	}
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

func (m *Manager) retainedIncrementalBases() map[string]bool {
	protected := make(map[string]bool)
	backups, err := m.List()
	if err != nil {
		return protected
	}
	for _, item := range backups {
		if !isIncrementalBackup(item.Filename) {
			continue
		}
		metadata, err := m.GetMetadata(item.Path)
		if err != nil || metadata == nil {
			// If an encrypted incremental cannot be read because no password is
			// available, conservatively retain all full backups.
			for _, candidate := range backups {
				if !isIncrementalBackup(candidate.Filename) {
					protected[candidate.Filename] = true
				}
			}
			continue
		}
		if metadata.BaseBackup != "" {
			protected[metadata.BaseBackup] = true
		}
	}
	return protected
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

	for total > limitBytes {
		protected := m.retainedIncrementalBases()
		candidate := -1
		for i, b := range backups {
			if !isIncrementalBackup(b.Filename) && !protected[b.Filename] {
				candidate = i
				break
			}
		}
		if candidate < 0 {
			for i, b := range backups {
				if isIncrementalBackup(b.Filename) {
					candidate = i
					break
				}
			}
		}
		if candidate < 0 {
			break
		}
		b := backups[candidate]
		if err := m.Delete(b.Filename); err != nil {
			slog.Warn("failed to delete backup over max_total_size", "file", b.Filename, "err", err)
			backups = append(backups[:candidate], backups[candidate+1:]...)
			continue
		}
		total -= b.Size
		backups = append(backups[:candidate], backups[candidate+1:]...)
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
	return m.CreateEncryptedAt(filename, time.Now().UTC())
}

// CreateEncryptedAt creates an encrypted full archive representing snapshotAt.
func (m *Manager) CreateEncryptedAt(filename string, snapshotAt time.Time) (string, error) {
	if m.password == "" {
		return "", fmt.Errorf("encryption password not set; configure server.backup.encryption_password")
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Per AI.md PART 21: the plaintext archive is built in memory and encrypted
	// before anything is written — the unencrypted archive never touches disk
	archive, err := m.buildArchiveAt(snapshotAt)
	if err != nil {
		return "", err
	}
	metadata, err := m.verifyManifestFromData(archive)
	if err != nil {
		return "", err
	}
	metadata.Encrypted = true
	metadata.EncryptionMethod = "AES-256-GCM"
	archive, err = replaceArchiveManifest(archive, metadata, nil)
	if err != nil {
		return "", err
	}

	encrypted, err := EncryptBackup(archive, m.password)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt backup: %w", err)
	}

	// Create encrypted file with .enc extension per AI.md PART 21
	encryptedPath := filepath.Join(m.backupDir, normalizeBackupFilename(filename)) + ".enc"
	if err := writeFileAtomic(encryptedPath, encrypted, 0o600); err != nil {
		return "", fmt.Errorf("failed to write encrypted backup: %w", err)
	}

	return encryptedPath, nil
}

// RestoreEncrypted restores from an encrypted backup (.enc extension)
// Per AI.md PART 21: .enc extension for encrypted backups
func (m *Manager) RestoreEncrypted(backupPath string) error {
	if m.password == "" {
		return fmt.Errorf("decryption password not set; supply the encrypted backup password")
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}

	// Restore handles both encrypted and unencrypted archives: it verifies the
	// backup, decrypts it in memory when needed, and swaps the restored state
	// in atomically
	return m.Restore(backupPath)
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
	BaseValid     bool `json:"base_valid"`
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
	return m.verifyBackup(backupPath, true)
}

func (m *Manager) verifyBackup(backupPath string, enforceRestoreLimits bool) (*VerificationResult, error) {
	result := &VerificationResult{
		// Default true for non-encrypted
		DecryptValid: true,
		BaseValid:    true,
	}
	var compressedTotal int64

	// Check 1: File exists
	info, err := os.Stat(backupPath)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("file not found: %v", err))
		return result, nil
	}
	result.FileExists = true
	compressedTotal = info.Size()

	// Check 2: Size > 0
	if info.Size() == 0 {
		result.Errors = append(result.Errors, "backup file is empty (size = 0)")
	} else {
		result.SizeValid = true
	}
	if enforceRestoreLimits && info.Size() > m.effectiveRestoreLimits().MaxArchiveSize {
		result.SizeValid = false
		result.Errors = append(result.Errors, fmt.Sprintf("restore max_archive_size exceeded: %d > %d", info.Size(), m.effectiveRestoreLimits().MaxArchiveSize))
		return result, nil
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
	combinedArchives := [][]byte{dataToVerify}
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("manifest verification failed: %v", err))
	} else {
		result.ManifestValid = true
		if metadata.Type == "incremental" {
			baseData, baseErr := m.verifyIncrementalBase(backupPath, metadata)
			if baseErr != nil {
				result.BaseValid = false
				result.Errors = append(result.Errors, baseErr.Error())
			} else {
				combinedArchives = append(combinedArchives, baseData)
				baseInfo, statErr := os.Stat(filepath.Join(filepath.Dir(backupPath), metadata.BaseBackup))
				if statErr == nil {
					compressedTotal += baseInfo.Size()
				}
			}
		}

		// Check 4: Checksum valid (verify stored checksum)
		if metadata.Checksum != "" && metadata.Checksums != nil {
			expectedChecksum := computeOverallChecksum(metadata.Checksums)
			storedChecksum := strings.TrimPrefix(metadata.Checksum, "sha256:")
			if expectedChecksum == storedChecksum && (metadata.SHA256 == "" || expectedChecksum == metadata.SHA256) {
				result.ChecksumValid = true
			} else {
				result.Errors = append(result.Errors, "checksum mismatch: backup may be corrupted")
			}
		} else {
			// Legacy backups without checksums - pass with warning
			result.ChecksumValid = true
		}
	}
	limitsValid := true
	if enforceRestoreLimits {
		if limitErr := m.validateRestoreLimits(combinedArchives, compressedTotal); limitErr != nil {
			limitsValid = false
			result.Errors = append(result.Errors, limitErr.Error())
		}
	}

	// Check 5 & 6: Content extraction and database integrity
	contentValid, databaseValid, contentErrs := m.verifyContentAndDatabase(dataToVerify)
	result.ContentValid = contentValid && limitsValid
	result.DatabaseValid = databaseValid
	result.Errors = append(result.Errors, contentErrs...)

	// Determine overall result
	result.AllPassed = result.FileExists && result.SizeValid && result.ChecksumValid && result.ManifestValid &&
		result.ContentValid && result.DatabaseValid && result.DecryptValid && result.BaseValid

	return result, nil
}

func (m *Manager) verifyIncrementalBase(incrementalPath string, incremental *BackupMetadata) ([]byte, error) {
	if incremental.BaseBackup == "" || filepath.Base(incremental.BaseBackup) != incremental.BaseBackup {
		return nil, fmt.Errorf("%w: invalid base backup name", ErrBaseMissing)
	}
	basePath := filepath.Join(filepath.Dir(incrementalPath), incremental.BaseBackup)
	if _, err := os.Stat(basePath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrBaseMissing, incremental.BaseBackup)
		}
		return nil, fmt.Errorf("%w: %v", ErrBaseMissing, err)
	}
	actualSHA, err := hashFile(basePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBaseCorrupt, err)
	}
	if incremental.BaseSHA256 == "" || actualSHA != incremental.BaseSHA256 {
		return nil, fmt.Errorf("%w: %s checksum mismatch", ErrBaseCorrupt, incremental.BaseBackup)
	}
	baseData, err := m.readArchive(basePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBaseCorrupt, err)
	}
	baseMetadata, err := m.verifyManifestFromData(baseData)
	if err != nil || (baseMetadata.Type != "" && baseMetadata.Type != "full") {
		return nil, fmt.Errorf("%w: base is not a valid full archive", ErrBaseCorrupt)
	}
	return baseData, nil
}

func (m *Manager) validateRestoreLimits(archives [][]byte, compressedTotal int64) error {
	limits := m.effectiveRestoreLimits()
	if compressedTotal > limits.MaxArchiveSize {
		return fmt.Errorf("restore max_archive_size exceeded: %d > %d", compressedTotal, limits.MaxArchiveSize)
	}
	var expanded, files int64
	for _, data := range archives {
		reader, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("restore archive is not valid gzip: %w", err)
		}
		tarReader := tar.NewReader(reader)
		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				_ = reader.Close()
				return fmt.Errorf("read restore archive: %w", err)
			}
			name := filepath.FromSlash(header.Name)
			if name == "" || filepath.IsAbs(name) || strings.HasPrefix(name, ".."+string(os.PathSeparator)) || name == ".." || (len(name) > 1 && name[1] == ':') {
				_ = reader.Close()
				return fmt.Errorf("restore archive contains unsafe path %q", header.Name)
			}
			if header.Size < 0 || header.Size > limits.MaxFileSize {
				_ = reader.Close()
				return fmt.Errorf("restore max_file_size exceeded for %q", header.Name)
			}
			switch header.Typeflag {
			case tar.TypeDir:
				continue
			case tar.TypeReg:
				files++
				if files > limits.MaxFiles {
					_ = reader.Close()
					return fmt.Errorf("restore max_files exceeded: %d > %d", files, limits.MaxFiles)
				}
				expanded += header.Size
				if expanded > limits.MaxExpandedSize {
					_ = reader.Close()
					return fmt.Errorf("restore max_expanded_size exceeded: %d > %d", expanded, limits.MaxExpandedSize)
				}
				if _, err := io.CopyN(io.Discard, tarReader, header.Size); err != nil {
					_ = reader.Close()
					return fmt.Errorf("read restore file %q: %w", header.Name, err)
				}
			default:
				_ = reader.Close()
				return fmt.Errorf("restore archive contains unsupported entry %q", header.Name)
			}
		}
		if err := reader.Close(); err != nil {
			return err
		}
	}
	if compressedTotal == 0 && expanded > 0 || compressedTotal > 0 && float64(expanded)/float64(compressedTotal) > float64(limits.MaxRatio) {
		return fmt.Errorf("restore max_ratio exceeded: expanded %d bytes from %d bytes", expanded, compressedTotal)
	}
	return nil
}

// verifyContentAndDatabase test-extracts every file in the backup archive to a
// temp dir and validates any embedded server.db as a well-formed SQLite file.
// Per AI.md PART 21: Content extraction and Database integrity are Fatal checks.
// Absence of a database file in the archive is not fatal (first-run/empty DB).
func (m *Manager) verifyContentAndDatabase(data []byte) (contentValid bool, databaseValid bool, errs []string) {
	contentValid = true
	databaseValid = true

	base := filepath.Join(os.TempDir(), config.ProjectOrg)
	if err := os.MkdirAll(base, 0o700); err != nil {
		return false, true, []string{fmt.Sprintf("content extraction failed: %v", err)}
	}
	extractDir, err := os.MkdirTemp(base, config.InternalName+"-verify-")
	if err != nil {
		return false, true, []string{fmt.Sprintf("content extraction failed: %v", err)}
	}
	defer func() {
		if err := os.RemoveAll(extractDir); err != nil {
			slog.Warn("failed to remove backup verification directory", "error", err)
		}
	}()

	reader := bytes.NewReader(data)
	gzReader, err := gzip.NewReader(reader)
	if err != nil {
		return false, true, []string{fmt.Sprintf("content extraction failed: invalid gzip format: %v", err)}
	}
	defer func() {
		if err := gzReader.Close(); err != nil {
			slog.Warn("failed to close backup verification gzip reader", "error", err)
		}
	}()

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
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
				continue
			}
			out, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
				continue
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", err))
			}
			if closeErr := out.Close(); closeErr != nil {
				contentValid = false
				errs = append(errs, fmt.Sprintf("content extraction failed: %v", closeErr))
			}

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
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("failed to close SQLite verification file", "error", err)
		}
	}()

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
	defer func() {
		if err := gzReader.Close(); err != nil {
			slog.Warn("failed to close backup manifest gzip reader", "error", err)
		}
	}()

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
		removeErr := os.Remove(backupPath)
		return "", nil, fmt.Errorf("backup verification process failed: %w", errors.Join(err, removeErr))
	}

	if !result.AllPassed {
		// Verification failed - delete the failed backup per AI.md PART 21
		removeErr := os.Remove(backupPath)
		return "", result, fmt.Errorf("backup verification failed: %v", errors.Join(errors.New(strings.Join(result.Errors, "; ")), removeErr))
	}

	return backupPath, result, nil
}

// CreateAtAndVerify creates and verifies a full backup representing snapshotAt.
func (m *Manager) CreateAtAndVerify(filename string, snapshotAt time.Time) (string, *VerificationResult, error) {
	path, err := m.CreateAt(filename, snapshotAt)
	if err != nil {
		return "", nil, err
	}
	return m.verifyCreated(path)
}

// CreateEncryptedAtAndVerify creates and verifies an encrypted full backup at snapshotAt.
func (m *Manager) CreateEncryptedAtAndVerify(filename string, snapshotAt time.Time) (string, *VerificationResult, error) {
	path, err := m.CreateEncryptedAt(filename, snapshotAt)
	if err != nil {
		return "", nil, err
	}
	return m.verifyCreated(path)
}

// CreateIncrementalAndVerify writes and verifies a differential backup.
func (m *Manager) CreateIncrementalAndVerify(filename string, snapshotAt time.Time) (string, *VerificationResult, error) {
	path, err := m.CreateIncrementalAt(filename, snapshotAt)
	if err != nil {
		return "", nil, err
	}
	return m.verifyCreated(path)
}

// CreateEncryptedIncrementalAndVerify writes and verifies an encrypted differential backup.
func (m *Manager) CreateEncryptedIncrementalAndVerify(filename string, snapshotAt time.Time) (string, *VerificationResult, error) {
	path, err := m.CreateEncryptedIncrementalAt(filename, snapshotAt)
	if err != nil {
		return "", nil, err
	}
	return m.verifyCreated(path)
}

// CreateDailyPairAndVerify captures one archive and derives the incremental and
// full backup from the same captured bytes. The incremental is written first.
func (m *Manager) CreateDailyPairAndVerify(incrementalName, fullName string, snapshotAt time.Time) (string, string, *VerificationResult, *VerificationResult, bool, error) {
	if m.password == "" && m.complianceRequiresEncryption() {
		return "", "", nil, nil, false, fmt.Errorf("compliance mode requires encrypted backups")
	}
	snapshotAt = snapshotAt.UTC().Truncate(time.Second)
	fullArchive, err := m.buildArchiveAt(snapshotAt)
	if err != nil {
		return "", "", nil, nil, false, err
	}
	basePath, base, err := m.latestFullBackup()
	skipIncremental := errors.Is(err, ErrNoFullBackup)
	if err != nil && !skipIncremental {
		return "", "", nil, nil, false, err
	}
	var incrementalPath string
	var incrementalVerify *VerificationResult
	var incrementalStage, incrementalTarget string
	if !skipIncremental {
		baseSHA, hashErr := hashFile(basePath)
		if hashErr != nil {
			return "", "", nil, nil, false, fmt.Errorf("hash incremental base: %w", hashErr)
		}
		incrementalArchive, _, buildErr := m.buildIncrementalArchiveFromFull(fullArchive, base, filepath.Base(basePath), baseSHA)
		if buildErr != nil {
			return "", "", nil, nil, false, buildErr
		}
		incrementalStage, incrementalTarget, incrementalVerify, err = m.stageVerifiedArchive(incrementalName, incrementalArchive)
		if err != nil {
			return "", "", incrementalVerify, nil, false, err
		}
	}
	fullStage, fullPath, fullVerify, err := m.stageVerifiedArchive(fullName, fullArchive)
	if err != nil {
		if incrementalStage != "" {
			if removeErr := os.Remove(incrementalStage); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove staged incremental backup: %w", removeErr))
			}
		}
		return "", "", incrementalVerify, fullVerify, skipIncremental, err
	}
	pair := []verifiedBackup{{stagePath: fullStage, targetPath: fullPath}}
	if incrementalStage != "" {
		pair = append(pair, verifiedBackup{stagePath: incrementalStage, targetPath: incrementalTarget})
	}
	if err := publishVerifiedBackupPair(pair); err != nil {
		for _, staged := range pair {
			if removeErr := os.Remove(staged.stagePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, fmt.Errorf("remove staged backup: %w", removeErr))
			}
		}
		return "", "", incrementalVerify, fullVerify, skipIncremental, err
	}
	if incrementalStage != "" {
		incrementalPath = incrementalTarget
	}
	return incrementalPath, fullPath, incrementalVerify, fullVerify, skipIncremental, nil
}

func (m *Manager) stageVerifiedArchive(filename string, archive []byte) (string, string, *VerificationResult, error) {
	if err := os.MkdirAll(m.backupDir, 0o755); err != nil {
		return "", "", nil, fmt.Errorf("failed to create backup directory: %w", err)
	}
	if m.password != "" {
		metadata, err := m.verifyManifestFromData(archive)
		if err != nil {
			return "", "", nil, err
		}
		metadata.Encrypted = true
		metadata.EncryptionMethod = "AES-256-GCM"
		archive, err = replaceArchiveManifest(archive, metadata, nil)
		if err != nil {
			return "", "", nil, err
		}
		archive, err = EncryptBackup(archive, m.password)
		if err != nil {
			return "", "", nil, err
		}
		filename = normalizeBackupFilename(filename) + ".enc"
	} else {
		filename = normalizeBackupFilename(filename)
	}
	targetPath := filepath.Join(m.backupDir, filename)
	extension := filepath.Ext(filename)
	stage, err := os.CreateTemp(m.backupDir, ".backup-stage-*"+extension)
	if err != nil {
		return "", "", nil, fmt.Errorf("create staged backup: %w", err)
	}
	stagePath := stage.Name()
	cleanup := func() {
		if removeErr := os.Remove(stagePath); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			slog.Warn("failed to remove staged backup", "path", stagePath, "err", removeErr)
		}
	}
	if _, err := stage.Write(archive); err != nil {
		closeErr := stage.Close()
		cleanup()
		return "", "", nil, errors.Join(fmt.Errorf("write staged backup: %w", err), closeErr)
	}
	if err := stage.Sync(); err != nil {
		closeErr := stage.Close()
		cleanup()
		return "", "", nil, errors.Join(fmt.Errorf("sync staged backup: %w", err), closeErr)
	}
	if err := stage.Close(); err != nil {
		cleanup()
		return "", "", nil, fmt.Errorf("close staged backup: %w", err)
	}
	result, err := m.verifyBackup(stagePath, false)
	if err != nil {
		cleanup()
		return "", "", result, fmt.Errorf("verify staged backup: %w", err)
	}
	if result == nil || !result.AllPassed {
		cleanup()
		if result == nil {
			return "", "", nil, fmt.Errorf("staged backup verification returned no result")
		}
		return "", "", result, fmt.Errorf("staged backup verification failed: %s", strings.Join(result.Errors, "; "))
	}
	return stagePath, targetPath, result, nil
}

type verifiedBackup struct {
	stagePath  string
	targetPath string
	previous   string
	published  bool
}

func publishVerifiedBackupPair(backups []verifiedBackup) error {
	for i := range backups {
		if _, err := os.Stat(backups[i].targetPath); err == nil {
			placeholder, createErr := os.CreateTemp(filepath.Dir(backups[i].targetPath), ".backup-previous-*")
			if createErr != nil {
				return rollbackBackupPair(backups[:i], fmt.Errorf("reserve previous backup path: %w", createErr))
			}
			backups[i].previous = placeholder.Name()
			if closeErr := placeholder.Close(); closeErr != nil {
				_ = os.Remove(backups[i].previous)
				return rollbackBackupPair(backups[:i], fmt.Errorf("close previous backup placeholder: %w", closeErr))
			}
			if err := os.Remove(backups[i].previous); err != nil {
				return rollbackBackupPair(backups[:i], fmt.Errorf("remove previous backup placeholder: %w", err))
			}
			if err := os.Rename(backups[i].targetPath, backups[i].previous); err != nil {
				return rollbackBackupPair(backups[:i], fmt.Errorf("preserve previous backup: %w", err))
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return rollbackBackupPair(backups[:i], fmt.Errorf("inspect backup target: %w", err))
		}
		if err := os.Rename(backups[i].stagePath, backups[i].targetPath); err != nil {
			return rollbackBackupPair(backups[:i+1], fmt.Errorf("publish verified backup: %w", err))
		}
		backups[i].published = true
	}
	for i := range backups {
		if backups[i].previous != "" {
			if err := os.Remove(backups[i].previous); err != nil {
				slog.Warn("failed to remove replaced backup after successful publish", "path", backups[i].previous, "err", err)
			}
		}
	}
	return nil
}

func rollbackBackupPair(backups []verifiedBackup, cause error) error {
	for i := len(backups) - 1; i >= 0; i-- {
		if backups[i].published {
			if err := os.Remove(backups[i].targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				cause = errors.Join(cause, fmt.Errorf("remove failed new backup %s: %w", backups[i].targetPath, err))
			}
		}
		if backups[i].previous != "" {
			if err := os.Rename(backups[i].previous, backups[i].targetPath); err != nil {
				cause = errors.Join(cause, fmt.Errorf("restore previous backup %s: %w", backups[i].targetPath, err))
			}
		}
	}
	return cause
}

func (m *Manager) verifyCreated(path string) (string, *VerificationResult, error) {
	result, err := m.verifyBackup(path, false)
	if err != nil {
		removeErr := os.Remove(path)
		return "", nil, fmt.Errorf("backup verification process failed: %w", errors.Join(err, removeErr))
	}
	if !result.AllPassed {
		removeErr := os.Remove(path)
		return "", result, fmt.Errorf("backup verification failed: %v", errors.Join(errors.New(strings.Join(result.Errors, "; ")), removeErr))
	}
	return path, result, nil
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
		removeErr := os.Remove(backupPath)
		return "", nil, fmt.Errorf("backup verification process failed: %w", errors.Join(err, removeErr))
	}

	if !result.AllPassed {
		removeErr := os.Remove(backupPath)
		return "", result, fmt.Errorf("backup verification failed: %v", errors.Join(errors.New(strings.Join(result.Errors, "; ")), removeErr))
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

	// Only now that verification passed, apply retention. The bucket algorithm
	// keeps the newest max_backups entries, so the backup just written at
	// backupPath is always among the survivors.
	slog.Info("scheduled backup verified", "path", backupPath)

	return m.ApplyRetention(countOnlyRetention(keepCount))
}
