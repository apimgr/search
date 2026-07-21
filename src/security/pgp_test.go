package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// generateTestKeypair is a shared helper so every test that needs a real
// keypair doesn't pay the Ed25519 generation cost more than once per run.
func generateTestKeypair(t *testing.T) *Keypair {
	t.Helper()
	kp, err := GenerateKeypair("TestApp", "security@example.com", "test-installation-secret")
	if err != nil {
		t.Fatalf("GenerateKeypair() error = %v", err)
	}
	return kp
}

func TestGenerateKeypair(t *testing.T) {
	tests := []struct {
		name               string
		appName            string
		securityContact    string
		installationSecret string
		wantErr            bool
	}{
		{"valid inputs", "TestApp", "security@example.com", "s3cret", false},
		{"empty installation secret rejected", "TestApp", "security@example.com", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kp, err := GenerateKeypair(tt.appName, tt.securityContact, tt.installationSecret)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GenerateKeypair() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if kp.Fingerprint == "" {
				t.Error("GenerateKeypair() Fingerprint is empty")
			}
			if kp.PublicKeyArmor == "" {
				t.Error("GenerateKeypair() PublicKeyArmor is empty")
			}
			if kp.EncryptedPrivateKey == "" {
				t.Error("GenerateKeypair() EncryptedPrivateKey is empty")
			}
			wantExpiry := kp.CreatedAt.Add(PGPKeyLifetime)
			if !kp.ExpiresAt.Equal(wantExpiry) {
				t.Errorf("GenerateKeypair() ExpiresAt = %v, want %v", kp.ExpiresAt, wantExpiry)
			}
		})
	}
}

func TestKeypair_LockUnlockRoundTrip(t *testing.T) {
	kp := generateTestKeypair(t)

	tests := []struct {
		name       string
		passphrase string
		wantErr    bool
	}{
		{"correct installation secret unlocks", "test-installation-secret", false},
		{"wrong installation secret rejected", "wrong-secret", true},
		{"empty installation secret rejected", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unlocked, err := UnlockPrivateKey(kp.EncryptedPrivateKey, tt.passphrase)
			if (err != nil) != tt.wantErr {
				t.Fatalf("UnlockPrivateKey() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !unlocked.IsPrivate() {
				t.Error("UnlockPrivateKey() result is not a private key")
			}
			locked, err := unlocked.IsLocked()
			if err != nil {
				t.Fatalf("IsLocked() error = %v", err)
			}
			if locked {
				t.Error("UnlockPrivateKey() result should be unlocked")
			}
		})
	}
}

func TestKeyFingerprint(t *testing.T) {
	kp := generateTestKeypair(t)

	got, err := KeyFingerprint(kp.PublicKeyArmor)
	if err != nil {
		t.Fatalf("KeyFingerprint() error = %v", err)
	}
	if got != kp.Fingerprint {
		t.Errorf("KeyFingerprint() = %q, want %q", got, kp.Fingerprint)
	}

	if _, err := KeyFingerprint("not an armored key"); err == nil {
		t.Error("KeyFingerprint() should error on garbage input")
	}
}

func TestIsKeypairExpired(t *testing.T) {
	created := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"just after creation", created.Add(time.Hour), false},
		{"one day before expiry", created.Add(PGPKeyLifetime - 24*time.Hour), false},
		{"exact expiry instant not yet after", created.Add(PGPKeyLifetime), false},
		{"one second after expiry", created.Add(PGPKeyLifetime + time.Second), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsKeypairExpired(created, tt.now); got != tt.want {
				t.Errorf("IsKeypairExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveLoadPublicKey(t *testing.T) {
	dir := t.TempDir()
	kp := generateTestKeypair(t)

	if err := SavePublicKey(dir, kp.PublicKeyArmor); err != nil {
		t.Fatalf("SavePublicKey() error = %v", err)
	}
	got, err := LoadPublicKey(dir)
	if err != nil {
		t.Fatalf("LoadPublicKey() error = %v", err)
	}
	if got != kp.PublicKeyArmor {
		t.Error("LoadPublicKey() did not round-trip SavePublicKey() content")
	}

	info, err := filepath.Glob(filepath.Join(dir, PGPDirName, PGPPublicKeyFilename))
	if err != nil || len(info) != 1 {
		t.Fatalf("expected public key file to exist at %s", filepath.Join(dir, PGPDirName, PGPPublicKeyFilename))
	}
}

func TestSaveLoadEncryptedPrivateKey(t *testing.T) {
	dir := t.TempDir()
	kp := generateTestKeypair(t)

	if err := SaveEncryptedPrivateKey(dir, kp.EncryptedPrivateKey); err != nil {
		t.Fatalf("SaveEncryptedPrivateKey() error = %v", err)
	}
	got, err := LoadEncryptedPrivateKey(dir)
	if err != nil {
		t.Fatalf("LoadEncryptedPrivateKey() error = %v", err)
	}
	if got != kp.EncryptedPrivateKey {
		t.Error("LoadEncryptedPrivateKey() did not round-trip SaveEncryptedPrivateKey() content")
	}
}

func TestLoadPublicKey_MissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadPublicKey(dir); err == nil {
		t.Error("LoadPublicKey() should error when no key file exists")
	}
}

func TestDeleteKeypairFiles(t *testing.T) {
	dir := t.TempDir()
	kp := generateTestKeypair(t)

	if err := SavePublicKey(dir, kp.PublicKeyArmor); err != nil {
		t.Fatalf("SavePublicKey() error = %v", err)
	}
	if err := SaveEncryptedPrivateKey(dir, kp.EncryptedPrivateKey); err != nil {
		t.Fatalf("SaveEncryptedPrivateKey() error = %v", err)
	}

	if err := DeleteKeypairFiles(dir); err != nil {
		t.Fatalf("DeleteKeypairFiles() error = %v", err)
	}
	if _, err := LoadPublicKey(dir); err == nil {
		t.Error("public key should be gone after DeleteKeypairFiles()")
	}
	if _, err := LoadEncryptedPrivateKey(dir); err == nil {
		t.Error("private key should be gone after DeleteKeypairFiles()")
	}

	// Deleting again (no files present) must not error.
	if err := DeleteKeypairFiles(dir); err != nil {
		t.Errorf("DeleteKeypairFiles() on already-empty dir error = %v", err)
	}
}

func TestImportPrivateKey(t *testing.T) {
	kp := generateTestKeypair(t)
	unlocked, err := UnlockPrivateKey(kp.EncryptedPrivateKey, "test-installation-secret")
	if err != nil {
		t.Fatalf("UnlockPrivateKey() error = %v", err)
	}
	decryptedArmor, err := unlocked.Armor()
	if err != nil {
		t.Fatalf("Armor() error = %v", err)
	}

	t.Run("unlocked private key imports successfully", func(t *testing.T) {
		imported, err := ImportPrivateKey(decryptedArmor, "TestApp", "security@example.com", "new-secret")
		if err != nil {
			t.Fatalf("ImportPrivateKey() error = %v", err)
		}
		if imported.Fingerprint != kp.Fingerprint {
			t.Errorf("ImportPrivateKey() Fingerprint = %q, want %q", imported.Fingerprint, kp.Fingerprint)
		}
		// Re-locked private key must be unlockable with the new secret, not the old one.
		if _, err := UnlockPrivateKey(imported.EncryptedPrivateKey, "new-secret"); err != nil {
			t.Errorf("re-locked imported key should unlock with new-secret: %v", err)
		}
	})

	t.Run("locked private key is rejected", func(t *testing.T) {
		if _, err := ImportPrivateKey(kp.EncryptedPrivateKey, "TestApp", "security@example.com", "new-secret"); err == nil {
			t.Error("ImportPrivateKey() should reject a still-locked private key")
		}
	})

	t.Run("public key is rejected", func(t *testing.T) {
		if _, err := ImportPrivateKey(kp.PublicKeyArmor, "TestApp", "security@example.com", "new-secret"); err == nil {
			t.Error("ImportPrivateKey() should reject a public key")
		}
	})

	t.Run("garbage input is rejected", func(t *testing.T) {
		if _, err := ImportPrivateKey("not armored data", "TestApp", "security@example.com", "new-secret"); err == nil {
			t.Error("ImportPrivateKey() should reject unparseable input")
		}
	})
}

func TestIdentityMatches(t *testing.T) {
	kp := generateTestKeypair(t)

	tests := []struct {
		name            string
		securityContact string
		want            bool
	}{
		{"matching contact", "security@example.com", true},
		{"mismatched contact", "someone-else@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IdentityMatches(kp.Key, "TestApp", tt.securityContact); got != tt.want {
				t.Errorf("IdentityMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIdentityMatches_NilEntity(t *testing.T) {
	if IdentityMatches(&crypto.Key{}, "TestApp", "security@example.com") {
		t.Error("IdentityMatches() should reject a key with no entity")
	}
}

func TestFormatFingerprint(t *testing.T) {
	tests := []struct {
		name string
		fp   string
		want string
	}{
		{"lowercase hex uppercased", "abcd1234", "ABCD1234"},
		{"already uppercase unchanged", "ABCD1234", "ABCD1234"},
		{"non-hex input returned as-is", "not-hex!!", "not-hex!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatFingerprint(tt.fp); got != tt.want {
				t.Errorf("FormatFingerprint(%q) = %q, want %q", tt.fp, got, tt.want)
			}
		})
	}
}

func TestPublishToKeyservers(t *testing.T) {
	okServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/vks/v1/upload" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer okServer.Close()

	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("keyserver unavailable"))
	}))
	defer failServer.Close()

	kp := generateTestKeypair(t)
	results := PublishToKeyservers(context.Background(), okServer.Client(), kp.PublicKeyArmor, []string{
		okServer.URL,
		failServer.URL,
		"://not a url",
	})

	if len(results) != 3 {
		t.Fatalf("PublishToKeyservers() returned %d results, want 3", len(results))
	}
	if !results[0].Published || results[0].Err != nil {
		t.Errorf("expected keyserver 0 to publish successfully, got Published=%v Err=%v", results[0].Published, results[0].Err)
	}
	if results[1].Published || results[1].Err == nil {
		t.Errorf("expected keyserver 1 to fail, got Published=%v Err=%v", results[1].Published, results[1].Err)
	}
	if results[2].Published || results[2].Err == nil {
		t.Errorf("expected keyserver 2 (invalid URL) to fail, got Published=%v Err=%v", results[2].Published, results[2].Err)
	}
}
