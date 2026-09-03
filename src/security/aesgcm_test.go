package security

import (
	"encoding/base64"
	"strings"
	"testing"
)

// testAESGCMKey is a dummy base64-encoded 32-byte key for AES-256-GCM tests.
const testAESGCMKey = "0udjKLl1uyKegN1WiUBRZ8QCZA1lTq6UQm6PbGlBeNs="

// TestEncryptDecryptAESGCM_RoundTrip verifies plaintext survives an
// encrypt/decrypt round trip, including the empty-plaintext boundary.
func TestEncryptDecryptAESGCM_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"normal message", []byte("coordinated disclosure report body")},
		{"empty plaintext", []byte("")},
		{"binary data", []byte{0x00, 0x01, 0xFF, 0xFE, 0x10}},
		{"long message", []byte(strings.Repeat("a", 10000))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EncryptAESGCM(testAESGCMKey, tt.plaintext)
			if err != nil {
				t.Fatalf("EncryptAESGCM() error = %v", err)
			}
			if encoded == "" {
				t.Fatal("EncryptAESGCM() returned empty string")
			}

			decoded, err := DecryptAESGCM(testAESGCMKey, encoded)
			if err != nil {
				t.Fatalf("DecryptAESGCM() error = %v", err)
			}
			if string(decoded) != string(tt.plaintext) {
				t.Errorf("DecryptAESGCM() = %q, want %q", decoded, tt.plaintext)
			}
		})
	}
}

// TestEncryptAESGCM_Nondeterministic verifies two encryptions of the same
// plaintext produce different ciphertext (random nonce per call).
func TestEncryptAESGCM_Nondeterministic(t *testing.T) {
	plaintext := []byte("same message")

	first, err := EncryptAESGCM(testAESGCMKey, plaintext)
	if err != nil {
		t.Fatalf("EncryptAESGCM() error = %v", err)
	}
	second, err := EncryptAESGCM(testAESGCMKey, plaintext)
	if err != nil {
		t.Fatalf("EncryptAESGCM() error = %v", err)
	}
	if first == second {
		t.Error("EncryptAESGCM() should produce different ciphertext for identical plaintext (nonce reuse)")
	}
}

// TestEncryptAESGCM_InvalidKey covers malformed and wrong-length key errors.
func TestEncryptAESGCM_InvalidKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"not base64", "not-valid-base64!!!"},
		{"empty key", ""},
		{"wrong length key (20 bytes, not a valid AES key size)", base64.StdEncoding.EncodeToString(make([]byte, 20))},
		{"wrong length key (short)", base64.StdEncoding.EncodeToString([]byte("short"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncryptAESGCM(tt.key, []byte("data"))
			if err == nil {
				t.Errorf("EncryptAESGCM(%q) expected error, got nil", tt.key)
			}
		})
	}
}

// TestDecryptAESGCM_Errors covers invalid key, invalid base64 ciphertext,
// truncated ciphertext, and tampered ciphertext (authentication failure).
func TestDecryptAESGCM_Errors(t *testing.T) {
	validCiphertext, err := EncryptAESGCM(testAESGCMKey, []byte("secret"))
	if err != nil {
		t.Fatalf("setup EncryptAESGCM() error = %v", err)
	}

	tests := []struct {
		name    string
		key     string
		encoded string
	}{
		{"invalid key", "not-valid-base64!!!", validCiphertext},
		{"invalid base64 ciphertext", testAESGCMKey, "not-valid-base64!!!"},
		{"empty ciphertext too short", testAESGCMKey, ""},
		{"truncated ciphertext shorter than nonce", testAESGCMKey, base64.StdEncoding.EncodeToString([]byte("x"))},
		{"tampered ciphertext fails authentication", testAESGCMKey, validCiphertext[:len(validCiphertext)-4] + "AAAA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := DecryptAESGCM(tt.key, tt.encoded)
			if err == nil {
				t.Errorf("DecryptAESGCM(%q, %q) expected error, got nil", tt.key, tt.encoded)
			}
		})
	}
}

// TestDecryptAESGCM_WrongKeyFails verifies decryption with a different key
// than was used to encrypt fails rather than silently returning garbage.
func TestDecryptAESGCM_WrongKeyFails(t *testing.T) {
	encoded, err := EncryptAESGCM(testAESGCMKey, []byte("secret data"))
	if err != nil {
		t.Fatalf("EncryptAESGCM() error = %v", err)
	}

	otherKey := base64.StdEncoding.EncodeToString([]byte("11111111111111111111111111111111")[:32])
	_, err = DecryptAESGCM(otherKey, encoded)
	if err == nil {
		t.Error("DecryptAESGCM() with wrong key expected error, got nil")
	}
}
