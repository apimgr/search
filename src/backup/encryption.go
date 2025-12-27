package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/argon2"
)

// Argon2 parameters for password-based key derivation
// Per TEMPLATE.md PART 2: Argon2id parameters (OWASP 2023)
const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// EncryptBackup encrypts backup data using AES-256-GCM with password-based key derivation
// Per TEMPLATE.md PART 24: Backup Encryption (NON-NEGOTIABLE)
// Algorithm: AES-256-GCM
// Key Derivation: Argon2id (password → encryption key)
// Password Storage: NEVER stored - admin must remember
func EncryptBackup(data []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, fmt.Errorf("password required for encryption")
	}

	// Generate random salt
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from password using Argon2id
	key := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Time,
		argon2Memory,
		argon2Threads,
		argon2KeyLen,
	)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt data
	ciphertext := gcm.Seal(nil, nonce, data, nil)

	// Prepend salt and nonce to ciphertext for decryption
	// Format: [salt(16)][nonce(12)][ciphertext]
	encrypted := make([]byte, 0, len(salt)+len(nonce)+len(ciphertext))
	encrypted = append(encrypted, salt...)
	encrypted = append(encrypted, nonce...)
	encrypted = append(encrypted, ciphertext...)

	return encrypted, nil
}

// DecryptBackup decrypts backup data using AES-256-GCM
// Per TEMPLATE.md PART 24: Backup Encryption
func DecryptBackup(encrypted []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, fmt.Errorf("password required for decryption")
	}

	// Check minimum size (salt + nonce + at least some data)
	minSize := argon2SaltLen + 12 // salt + GCM nonce
	if len(encrypted) < minSize {
		return nil, fmt.Errorf("invalid encrypted data: too short")
	}

	// Extract salt, nonce, and ciphertext
	salt := encrypted[:argon2SaltLen]
	nonce := encrypted[argon2SaltLen : argon2SaltLen+12]
	ciphertext := encrypted[argon2SaltLen+12:]

	// Derive decryption key from password using same parameters
	key := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Time,
		argon2Memory,
		argon2Threads,
		argon2KeyLen,
	)

	// Create AES cipher
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt data
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?): %w", err)
	}

	return plaintext, nil
}

// HashPassword creates a SHA-256 hash of the password for verification
// This is NOT stored - only used to verify password hasn't changed
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// VerifyPassword checks if a password matches the stored hash
func VerifyPassword(password, hash string) bool {
	return HashPassword(password) == hash
}

// EncryptFile encrypts a file and writes to output path
func EncryptFile(inputPath, outputPath, password string) error {
	// Read input file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}

	// Encrypt data
	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	// Write encrypted data
	if err := os.WriteFile(outputPath, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to write encrypted file: %w", err)
	}

	return nil
}

// DecryptFile decrypts a file and writes to output path
func DecryptFile(inputPath, outputPath, password string) error {
	// Read encrypted file
	encrypted, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %w", err)
	}

	// Decrypt data
	decrypted, err := DecryptBackup(encrypted, password)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}

	// Write decrypted data
	if err := os.WriteFile(outputPath, decrypted, 0600); err != nil {
		return fmt.Errorf("failed to write decrypted file: %w", err)
	}

	return nil
}
