package security

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/apimgr/search/src/database"
)

// StagePGPRotation writes every rotation file before the database commit.
func StagePGPRotation(configDir string, keypair *Keypair, signature string, now time.Time) error {
	if keypair == nil || keypair.PublicKeyArmor == "" || keypair.EncryptedPrivateKey == "" || signature == "" {
		return fmt.Errorf("PGP rotation stage is incomplete")
	}
	dir := filepath.Join(configDir, PGPDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create PGP rotation directory: %w", err)
	}
	staged := []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{PGPPublicKeyStagedFilename, []byte(keypair.PublicKeyArmor), 0o644},
		{PGPEncryptedPrivateKeyStagedFilename, []byte(keypair.EncryptedPrivateKey), 0o600},
		{PGPRotationSignatureStagedFilename, []byte(signature), 0o644},
		{PGPOldKeyExpiryStagedFilename, []byte(now.UTC().Add(PGPOldKeyGracePeriod).Format(time.RFC3339)), 0o600},
	}
	for _, item := range staged {
		if _, err := os.Stat(filepath.Join(dir, item.name)); err == nil {
			return fmt.Errorf("staged PGP rotation file %s already exists", item.name)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect staged PGP rotation file %s: %w", item.name, err)
		}
	}
	var created []string
	for _, item := range staged {
		path := filepath.Join(dir, item.name)
		if err := writeSyncedFile(path, item.data, item.mode); err != nil {
			var cleanupErr error
			for _, createdPath := range created {
				if removeErr := os.Remove(createdPath); removeErr != nil && !os.IsNotExist(removeErr) {
					cleanupErr = errors.Join(cleanupErr, removeErr)
				}
			}
			return errors.Join(fmt.Errorf("stage PGP rotation file %s: %w", item.name, err), cleanupErr)
		}
		created = append(created, path)
	}
	return syncDirectory(dir)
}

// DiscardPGPRotation removes only files that have not been promoted to active
// key paths.
func DiscardPGPRotation(configDir string) error {
	dir := filepath.Join(configDir, PGPDirName)
	var errs []error
	for _, name := range []string{PGPPublicKeyStagedFilename, PGPEncryptedPrivateKeyStagedFilename, PGPRotationSignatureStagedFilename, PGPOldKeyExpiryStagedFilename} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove staged PGP rotation file %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// CheckPGPRotationFiles prevents entering the database commit point when the
// fixed old-key retention path is still occupied by a previous rotation.
func CheckPGPRotationFiles(configDir string) error {
	dir := filepath.Join(configDir, PGPDirName)
	oldPath := filepath.Join(dir, PGPOldEncryptedPrivateKeyFilename)
	activePath := filepath.Join(dir, PGPEncryptedPrivateKeyFilename)
	if _, err := os.Stat(oldPath); err != nil {
		if os.IsNotExist(err) { return nil }
		return fmt.Errorf("inspect retained PGP private key: %w", err)
	}
	if _, err := os.Stat(activePath); err != nil {
		if os.IsNotExist(err) { return nil }
		return fmt.Errorf("inspect active PGP private key: %w", err)
	}
	return fmt.Errorf("retired PGP private key already exists; wait for its retention period to expire before rotating again")
}

// FinalizePGPRotation promotes staged keys after the database fingerprint has
// committed. It is safe to call again after an interrupted promotion.
func FinalizePGPRotation(configDir, expectedFingerprint string) error {
	dir := filepath.Join(configDir, PGPDirName)
	stagedPrivate := filepath.Join(dir, PGPEncryptedPrivateKeyStagedFilename)
	oldPrivate := filepath.Join(dir, PGPOldEncryptedPrivateKeyFilename)
	if _, stagedErr := os.Stat(stagedPrivate); stagedErr == nil {
		if _, oldErr := os.Stat(oldPrivate); oldErr == nil {
			if _, activeErr := os.Stat(filepath.Join(dir, PGPEncryptedPrivateKeyFilename)); activeErr == nil {
				return fmt.Errorf("retired PGP private key already exists; refusing to replace retained key")
			} else if !os.IsNotExist(activeErr) {
				return fmt.Errorf("inspect active PGP private key: %w", activeErr)
			}
		} else if !os.IsNotExist(oldErr) {
			return fmt.Errorf("inspect retired PGP private key: %w", oldErr)
		}
	} else if !os.IsNotExist(stagedErr) {
		return fmt.Errorf("inspect staged PGP private key: %w", stagedErr)
	}
	activePublic := filepath.Join(dir, PGPPublicKeyFilename)
	stagedPublic := filepath.Join(dir, PGPPublicKeyStagedFilename)
	public, err := os.ReadFile(stagedPublic)
	if os.IsNotExist(err) {
		public, err = os.ReadFile(activePublic)
	}
	if err != nil {
		return fmt.Errorf("read PGP rotation public key: %w", err)
	}
	fingerprint, err := KeyFingerprint(string(public))
	if err != nil {
		return fmt.Errorf("fingerprint PGP rotation public key: %w", err)
	}
	if fingerprint != expectedFingerprint {
		return fmt.Errorf("staged PGP fingerprint %s does not match committed fingerprint %s", fingerprint, expectedFingerprint)
	}
	if _, err := os.Stat(stagedPublic); err == nil {
		if err := os.Remove(activePublic); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove previous PGP public key before promotion: %w", err)
		}
		if err := os.Rename(stagedPublic, activePublic); err != nil {
			return fmt.Errorf("promote staged PGP public key: %w", err)
		}
	}

	activePrivate := filepath.Join(dir, PGPEncryptedPrivateKeyFilename)
	if _, err := os.Stat(stagedPrivate); err == nil {
		if _, activeErr := os.Stat(activePrivate); activeErr == nil {
			if err := os.Rename(activePrivate, oldPrivate); err != nil {
				return fmt.Errorf("archive previous PGP private key: %w", err)
			}
		} else if !os.IsNotExist(activeErr) {
			return fmt.Errorf("inspect active PGP private key: %w", activeErr)
		}
		if err := os.Rename(stagedPrivate, activePrivate); err != nil {
			return fmt.Errorf("promote staged PGP private key: %w", err)
		}
	}

	for _, pair := range [][2]string{
		{PGPRotationSignatureStagedFilename, PGPRotationSignatureFilename},
		{PGPOldKeyExpiryStagedFilename, PGPOldKeyExpiryFilename},
	} {
		staged := filepath.Join(dir, pair[0])
		if _, err := os.Stat(staged); err == nil {
			if err := os.Rename(staged, filepath.Join(dir, pair[1])); err != nil {
				return fmt.Errorf("promote PGP rotation file %s: %w", pair[0], err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect staged PGP rotation file %s: %w", pair[0], err)
		}
	}
	return syncDirectory(dir)
}

// RecoverPGPRotation completes a committed staged rotation or discards a
// pre-commit stage before the server accepts security reports.
func RecoverPGPRotation(configDir string, db *database.DB) error {
	dir := filepath.Join(configDir, PGPDirName)
	staged := false
	for _, name := range []string{PGPPublicKeyStagedFilename, PGPEncryptedPrivateKeyStagedFilename, PGPRotationSignatureStagedFilename, PGPOldKeyExpiryStagedFilename} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			staged = true
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect staged PGP rotation file %s: %w", name, err)
		}
	}
	if !staged {
		return nil
	}
	publicPath := filepath.Join(dir, PGPPublicKeyStagedFilename)
	public, err := os.ReadFile(publicPath)
	if os.IsNotExist(err) {
		public, err = os.ReadFile(filepath.Join(dir, PGPPublicKeyFilename))
	}
	if err != nil {
		return fmt.Errorf("read candidate PGP rotation public key: %w", err)
	}
	fingerprint, err := KeyFingerprint(string(public))
	if err != nil {
		return fmt.Errorf("fingerprint candidate PGP rotation public key: %w", err)
	}
	table := database.ServerTableName(db, "pgp_keypair")
	var committed string
	queryErr := db.QueryRow(context.Background(), fmt.Sprintf("SELECT fingerprint FROM %s WHERE revoked = 0 ORDER BY id DESC LIMIT 1", table)).Scan(&committed)
	if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
		return fmt.Errorf("read committed PGP rotation fingerprint: %w", queryErr)
	}
	if queryErr == nil && committed == fingerprint {
		return FinalizePGPRotation(configDir, fingerprint)
	}
	return DiscardPGPRotation(configDir)
}

func writeSyncedFile(path string, data []byte, mode os.FileMode) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	if _, err = file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func syncDirectory(path string) error {
	if runtime.GOOS == "windows" { return nil }
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		return errors.Join(err, dir.Close())
	}
	return dir.Close()
}
