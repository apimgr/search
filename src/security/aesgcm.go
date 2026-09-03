package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// EncryptAESGCM encrypts plaintext with AES-256-GCM using base64Key (the
// base64-encoded 32-byte server.security.encryption_key), returning a
// base64-encoded blob of nonce||ciphertext. Used per AI.md PART 11
// "Submission Flow" step 3 as the at-rest encryption fallback for
// coordinated-disclosure security reports when no PGP keypair is configured.
func EncryptAESGCM(base64Key string, plaintext []byte) (string, error) {
	gcm, err := newAESGCM(base64Key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptAESGCM reverses EncryptAESGCM, returning the original plaintext.
func DecryptAESGCM(base64Key, encoded string) ([]byte, error) {
	gcm, err := newAESGCM(base64Key)
	if err != nil {
		return nil, err
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ciphertext := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// newAESGCM builds an AES-256-GCM cipher from a base64-encoded 32-byte key.
func newAESGCM(base64Key string) (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("decode key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("build cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("build gcm: %w", err)
	}
	return gcm, nil
}
