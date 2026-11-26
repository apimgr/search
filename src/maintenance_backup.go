package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/security"
)

func maintenanceBackupConfig(load func() (*config.Config, error)) (*config.Config, error) {
	cfg, err := load()
	if err != nil {
		return nil, fmt.Errorf("load configuration for backup: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("load configuration for backup: initialization returned no configuration")
	}
	return cfg, nil
}

type maintenanceBackupOptions struct {
	Filename    string
	IncludeSSL  bool
	IncludeData bool
}

func parseMaintenanceBackupOptions(args []string) (maintenanceBackupOptions, error) {
	var options maintenanceBackupOptions
	for _, arg := range args {
		switch arg {
		case "--include-ssl":
			options.IncludeSSL = true
		case "--include-data":
			options.IncludeData = true
		default:
			if strings.HasPrefix(arg, "-") {
				return options, fmt.Errorf("unknown backup option %q", arg)
			}
			if options.Filename != "" {
				return options, fmt.Errorf("backup accepts at most one filename")
			}
			options.Filename = arg
		}
	}
	return options, nil
}

func runMaintenanceBackupTest(filename string) error {
	cfg, err := config.Initialize()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	mgr := backup.NewManager()
	limits, err := backup.RestoreLimitsFromConfig(cfg.Server.Restore)
	if err != nil {
		return fmt.Errorf("resolve restore limits: %w", err)
	}
	mgr.SetRestoreLimits(limits)
	var password string
	if filename == "" {
		backups, listErr := mgr.List()
		if listErr != nil {
			return fmt.Errorf("list backups: %w", listErr)
		}
		for _, item := range backups {
			if !backup.IsEncrypted(item.Path) {
				continue
			}
			password = readBackupPassword("Enter backup password to select the newest snapshot: ")
			if password == "" {
				return fmt.Errorf("selecting the newest encrypted backup requires an interactive terminal")
			}
			mgr.SetPassword(password)
			break
		}
		filename, err = latestBackup(mgr)
		if err != nil {
			return err
		}
	}
	path := filename
	if !filepath.IsAbs(path) {
		path = filepath.Join(config.GetBackupDir(), filename)
	}
	if backup.IsEncrypted(path) && password == "" {
		password = readBackupPassword("Enter backup password: ")
		if password == "" {
			return fmt.Errorf("encrypted backup test requires an interactive terminal to enter its password")
		}
		mgr.SetPassword(password)
	}
	result, err := mgr.VerifyBackup(path)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("backup verification returned no results")
	}
	checks := []struct {
		name string
		pass bool
	}{
		{"File exists", result.FileExists},
		{"Size valid", result.SizeValid},
		{"Manifest valid", result.ManifestValid},
		{"Checksum valid", result.ChecksumValid},
		{"Content extractable", result.ContentValid},
		{"Database valid", result.DatabaseValid},
		{"Decryption valid", result.DecryptValid},
		{"Incremental base valid", result.BaseValid},
	}
	for _, check := range checks {
		status := "FAILED"
		if check.pass {
			status = "OK"
		}
		fmt.Printf("  %s: %s\n", check.name, status)
	}
	if !result.AllPassed {
		return fmt.Errorf("backup verification checks failed: %s", strings.Join(result.Errors, "; "))
	}
	pgpChecked, err := verifyBackupPGPKeypair(path, password)
	if err != nil {
		return err
	}
	if pgpChecked {
		fmt.Println("  PGP keypair: OK")
	} else {
		fmt.Println("  PGP keypair: not configured in this backup")
	}
	return nil
}

func latestBackup(mgr *backup.Manager) (string, error) {
	backups, err := mgr.List()
	if err != nil {
		return "", fmt.Errorf("list backups: %w", err)
	}
	if len(backups) == 0 {
		return "", fmt.Errorf("no backups found")
	}
	type candidate struct {
		path      string
		snapshot  int64
		createdAt int64
	}
	items := make([]candidate, 0, len(backups))
	for _, item := range backups {
		metadata, metadataErr := mgr.GetMetadata(item.Path)
		if metadataErr != nil {
			continue
		}
		snapshot := metadata.SnapshotAt
		if snapshot.IsZero() {
			snapshot = metadata.CreatedAt
		}
		items = append(items, candidate{path: item.Path, snapshot: snapshot.UnixNano(), createdAt: item.CreatedAt.UnixNano()})
	}
	if len(items) == 0 {
		return "", fmt.Errorf("no readable backups found")
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].snapshot == items[j].snapshot {
			return items[i].createdAt > items[j].createdAt
		}
		return items[i].snapshot > items[j].snapshot
	})
	return items[0].path, nil
}

func verifyBackupPGPKeypair(backupPath, password string) (bool, error) {
	mgr := backup.NewManager()
	if password != "" {
		mgr.SetPassword(password)
	}
	metadata, err := mgr.GetMetadata(backupPath)
	if err != nil {
		return false, fmt.Errorf("read backup manifest for PGP key check: %w", err)
	}
	if metadata.Type == "incremental" {
		if metadata.BaseBackup == "" || filepath.Base(metadata.BaseBackup) != metadata.BaseBackup {
			return false, fmt.Errorf("incremental backup has an invalid base name")
		}
		backupPath = filepath.Join(filepath.Dir(backupPath), metadata.BaseBackup)
	}
	archive, err := readBackupArchive(backupPath, password)
	if err != nil {
		return false, fmt.Errorf("read backup contents for PGP key check: %w", err)
	}
	baseDir := filepath.Join(os.TempDir(), config.ProjectOrg)
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return false, fmt.Errorf("create private backup-test directory: %w", err)
	}
	workDir, err := os.MkdirTemp(baseDir, config.InternalName+"-backup-test-")
	if err != nil {
		return false, fmt.Errorf("create private backup-test directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			slog.Warn("failed to remove backup test directory", "error", err)
		}
	}()
	if err := extractBackupPGPFiles(archive, workDir); err != nil {
		return false, err
	}
	privatePath := filepath.Join(workDir, "config", "security", security.PGPEncryptedPrivateKeyFilename)
	publicPath := filepath.Join(workDir, "config", "security", security.PGPPublicKeyFilename)
	_, privateErr := os.Stat(privatePath)
	_, publicErr := os.Stat(publicPath)
	if os.IsNotExist(privateErr) && os.IsNotExist(publicErr) {
		return false, nil
	}
	if privateErr != nil || publicErr != nil {
		return false, fmt.Errorf("backup contains an incomplete PGP keypair")
	}
	publicBytes, err := os.ReadFile(publicPath)
	if err != nil {
		return false, fmt.Errorf("read backed-up PGP public key: %w", err)
	}
	privateBytes, err := os.ReadFile(privatePath)
	if err != nil {
		return false, fmt.Errorf("read backed-up PGP private key: %w", err)
	}
	dataDir := filepath.Join(workDir, "data")
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return false, fmt.Errorf("prepare backup database: %w", err)
	}
	databasePath := filepath.Join(dataDir, "server.db")
	if _, err := os.Stat(databasePath); os.IsNotExist(err) {
		alternate := filepath.Join(workDir, "database", "server.db")
		if _, altErr := os.Stat(alternate); altErr != nil {
			return false, fmt.Errorf("backup PGP keypair has no server database containing installation_secret")
		}
		data, readErr := os.ReadFile(alternate)
		if readErr != nil {
			return false, fmt.Errorf("read backed-up server database: %w", readErr)
		}
		if err := os.WriteFile(databasePath, data, 0o600); err != nil {
			return false, fmt.Errorf("stage backed-up server database: %w", err)
		}
	}
	dm, err := database.NewDatabaseManager(&database.Config{Driver: "sqlite", DataDir: dataDir, MaxOpen: 2, MaxIdle: 1, Lifetime: 60})
	if err != nil {
		return false, fmt.Errorf("open backed-up server database: %w", err)
	}
	secret, secretErr := database.GetSecret(context.Background(), dm.ServerDB(), database.SecretInstallation)
	closeErr := dm.Close()
	if secretErr != nil {
		return false, fmt.Errorf("read installation_secret from backed-up server database: %w", secretErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close backed-up server database: %w", closeErr)
	}
	privateKey, err := security.UnlockPrivateKey(string(privateBytes), secret.Value)
	if err != nil {
		return false, fmt.Errorf("decrypt backed-up PGP private key: %w", err)
	}
	publicFingerprint, err := security.KeyFingerprint(string(publicBytes))
	if err != nil {
		return false, fmt.Errorf("parse backed-up PGP public key: %w", err)
	}
	if privateKey.GetFingerprint() != publicFingerprint {
		return false, fmt.Errorf("backed-up PGP public and private key fingerprints do not match")
	}
	return true, nil
}

func readBackupArchive(path, password string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if backup.IsEncrypted(path) {
		return backup.DecryptBackup(data, password)
	}
	return data, nil
}

func extractBackupPGPFiles(data []byte, workDir string) error {
	gzReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open backup archive: %w", err)
	}
	defer func() {
		if err := gzReader.Close(); err != nil {
			slog.Warn("failed to close backup test gzip reader", "error", err)
		}
	}()
	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read backup archive: %w", err)
		}
		name := filepath.ToSlash(filepath.Clean(header.Name))
		isPGPKey := name == "config/security/"+security.PGPEncryptedPrivateKeyFilename || name == "config/security/"+security.PGPPublicKeyFilename
		isDataDB := strings.HasPrefix(name, "data/") && filepath.Base(name) == "server.db"
		isDatabaseDB := name == "database/server.db"
		if (!isPGPKey && !isDataDB && !isDatabaseDB) || header.Typeflag != tar.TypeReg {
			continue
		}
		targetName := name
		if isDataDB {
			targetName = "data/server.db"
		}
		target := filepath.Join(workDir, filepath.FromSlash(targetName))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create backup-test path: %w", err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("create extracted backup file: %w", err)
		}
		_, copyErr := io.Copy(file, tarReader)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("extract backup file: %w", copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close extracted backup file: %w", closeErr)
		}
	}
}
