package security

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPGPRotationStagesThenPromotesKeypair(t *testing.T) {
	configDir := t.TempDir()
	oldKeypair := generateTestKeypair(t)
	newKeypair := generateTestKeypair(t)
	if err := SavePublicKey(configDir, oldKeypair.PublicKeyArmor); err != nil {
		t.Fatal(err)
	}
	if err := SaveEncryptedPrivateKey(configDir, oldKeypair.EncryptedPrivateKey); err != nil {
		t.Fatal(err)
	}
	oldPrivate, err := UnlockPrivateKey(oldKeypair.EncryptedPrivateKey, "test-installation-secret")
	if err != nil {
		t.Fatal(err)
	}
	signature, err := SignDetached(oldPrivate, []byte(newKeypair.PublicKeyArmor))
	if err != nil {
		t.Fatal(err)
	}
	rotatedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := StagePGPRotation(configDir, newKeypair, signature, rotatedAt); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(configDir, PGPDirName)
	activePublic, err := os.ReadFile(filepath.Join(dir, PGPPublicKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(activePublic) != oldKeypair.PublicKeyArmor {
		t.Fatal("staging replaced the active public key before the metadata commit")
	}
	if err := FinalizePGPRotation(configDir, newKeypair.Fingerprint); err != nil {
		t.Fatal(err)
	}
	activePublic, err = os.ReadFile(filepath.Join(dir, PGPPublicKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(activePublic) != newKeypair.PublicKeyArmor {
		t.Fatal("active public key was not promoted")
	}
	activePrivate, err := os.ReadFile(filepath.Join(dir, PGPEncryptedPrivateKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(activePrivate) != newKeypair.EncryptedPrivateKey {
		t.Fatal("active encrypted private key was not promoted")
	}
	oldPrivateBytes, err := os.ReadFile(filepath.Join(dir, PGPOldEncryptedPrivateKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(oldPrivateBytes) != oldKeypair.EncryptedPrivateKey {
		t.Fatal("previous encrypted private key was not retained")
	}
	expiry, err := os.ReadFile(filepath.Join(dir, PGPOldKeyExpiryFilename))
	if err != nil {
		t.Fatal(err)
	}
	wantExpiry := rotatedAt.Add(PGPOldKeyGracePeriod).Format(time.RFC3339)
	if string(expiry) != wantExpiry {
		t.Fatalf("retired key expiry = %q, want %q", expiry, wantExpiry)
	}
	for _, name := range []string{PGPPublicKeyStagedFilename, PGPEncryptedPrivateKeyStagedFilename, PGPRotationSignatureStagedFilename, PGPOldKeyExpiryStagedFilename} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("staged file %s remains after promotion: %v", name, err)
		}
	}
}

func TestPGPRotationRefusesToReplaceRetiredPrivateKey(t *testing.T) {
	configDir := t.TempDir()
	oldKeypair := generateTestKeypair(t)
	newKeypair := generateTestKeypair(t)
	if err := SavePublicKey(configDir, oldKeypair.PublicKeyArmor); err != nil {
		t.Fatal(err)
	}
	if err := SaveEncryptedPrivateKey(configDir, oldKeypair.EncryptedPrivateKey); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, PGPDirName, PGPOldEncryptedPrivateKeyFilename), []byte("retained"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := StagePGPRotation(configDir, newKeypair, "signature", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := FinalizePGPRotation(configDir, newKeypair.Fingerprint); err == nil {
		t.Fatal("FinalizePGPRotation() succeeded with an existing retired private key")
	}
	activePublic, err := os.ReadFile(filepath.Join(configDir, PGPDirName, PGPPublicKeyFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(activePublic) != oldKeypair.PublicKeyArmor {
		t.Fatal("failed promotion replaced the active public key")
	}
}
