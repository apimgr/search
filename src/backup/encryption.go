package backup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2 parameters for password-based key derivation.
// Per AI.md PART 2: Argon2id parameters (OWASP 2023).
// Reader-side maxima bound work from unauthenticated archive headers.
const (
	argon2Time    = 3
	argon2Memory  = 64 * 1024
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16

	backupHeaderFixedLen = 55
	backupHeaderMaxBase  = 255
	backupMaxMemoryKiB   = 128 * 1024
	backupMaxIterations  = 6
	backupMaxParallelism = 8
)

var backupMagic = []byte("BKUP1")

// BackupEncryptionHeader contains the cleartext metadata authenticated by an encrypted backup.
type BackupEncryptionHeader struct {
	Salt        []byte
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint32
	Nonce       []byte
	KeyID       string
	BaseName    string

	encoded []byte
}

// EncryptBackup encrypts backup data using the BKUP1 AES-256-GCM archive format.
// An empty base name identifies a full backup.
func EncryptBackup(data []byte, password string) ([]byte, error) {
	return EncryptBackupWithBase(data, password, "")
}

// EncryptBackupWithBase encrypts backup data and records the base archive name for an incremental.
func EncryptBackupWithBase(data []byte, password, baseName string) ([]byte, error) {
	if password == "" {
		return nil, fmt.Errorf("password required for encryption")
	}
	if err := validateBackupBaseName(baseName); err != nil {
		return nil, err
	}

	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	header := BackupEncryptionHeader{
		Salt:        salt,
		MemoryKiB:   argon2Memory,
		Iterations:  argon2Time,
		Parallelism: argon2Threads,
		Nonce:       nonce,
		BaseName:    baseName,
	}
	if err := validateArgon2Params(header.MemoryKiB, header.Iterations, header.Parallelism); err != nil {
		return nil, err
	}
	key := deriveBackupKey(password, header)
	keyID := backupKeyID(key)
	header.KeyID = keyID
	header.encoded = encodeBackupHeader(header)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, data, header.encoded)
	return append(header.encoded, ciphertext...), nil
}

// ReadBackupEncryptionHeader parses and validates the cleartext BKUP1 header without deriving a key.
func ReadBackupEncryptionHeader(data []byte) (BackupEncryptionHeader, int, error) {
	if len(data) < backupHeaderFixedLen {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: truncated fixed header")
	}
	if !hmac.Equal(data[:len(backupMagic)], backupMagic) {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: bad magic")
	}
	baseLen := int(binary.BigEndian.Uint16(data[53:55]))
	if baseLen > backupHeaderMaxBase {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: base name exceeds %d bytes", backupHeaderMaxBase)
	}
	headerLen := backupHeaderFixedLen + baseLen
	if len(data) < headerLen {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: truncated base name")
	}
	keyID := string(data[45:53])
	if len(keyID) != 8 || strings.ToLower(keyID) != keyID {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: key_id must be 8 lowercase hexadecimal characters")
	}
	if _, err := hex.DecodeString(keyID); err != nil {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: malformed key_id")
	}
	baseBytes := data[backupHeaderFixedLen:headerLen]
	if !utf8.Valid(baseBytes) {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: base name is not valid UTF-8")
	}
	baseName := string(baseBytes)
	if err := validateBackupBaseName(baseName); err != nil {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: %w", err)
	}
	header := BackupEncryptionHeader{
		Salt:        append([]byte(nil), data[5:21]...),
		MemoryKiB:   binary.BigEndian.Uint32(data[21:25]),
		Iterations:  binary.BigEndian.Uint32(data[25:29]),
		Parallelism: binary.BigEndian.Uint32(data[29:33]),
		Nonce:       append([]byte(nil), data[33:45]...),
		KeyID:       keyID,
		BaseName:    baseName,
		encoded:     append([]byte(nil), data[:headerLen]...),
	}
	if err := validateArgon2Params(header.MemoryKiB, header.Iterations, header.Parallelism); err != nil {
		return BackupEncryptionHeader{}, 0, fmt.Errorf("invalid BKUP1 header: %w", err)
	}
	return header, headerLen, nil
}

// BackupEncryptionKeyID derives the key identifier for an archive header and password.
func BackupEncryptionKeyID(password string, header BackupEncryptionHeader) (string, error) {
	if password == "" {
		return "", fmt.Errorf("password required for key identifier")
	}
	if len(header.Salt) != argon2SaltLen {
		return "", fmt.Errorf("invalid backup header salt length")
	}
	if err := validateArgon2Params(header.MemoryKiB, header.Iterations, header.Parallelism); err != nil {
		return "", err
	}
	return backupKeyID(deriveBackupKey(password, header)), nil
}

// DecryptBackup decrypts BKUP1 data using AES-256-GCM and the archive's stored Argon2id parameters.
func DecryptBackup(encrypted []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, fmt.Errorf("password required for decryption")
	}
	header, headerLen, err := ReadBackupEncryptionHeader(encrypted)
	if err != nil {
		return nil, err
	}
	if len(encrypted)-headerLen < 16 {
		return nil, fmt.Errorf("invalid encrypted data: ciphertext is shorter than the GCM tag")
	}
	key := deriveBackupKey(password, header)
	if subtle.ConstantTimeCompare([]byte(backupKeyID(key)), []byte(header.KeyID)) != 1 {
		return nil, fmt.Errorf("decryption failed (wrong password?): key identifier mismatch")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	plaintext, err := gcm.Open(nil, header.Nonce, encrypted[headerLen:], header.encoded)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password?): %w", err)
	}
	return plaintext, nil
}

func deriveBackupKey(password string, header BackupEncryptionHeader) []byte {
	return argon2.IDKey([]byte(password), header.Salt, header.Iterations, header.MemoryKiB, uint8(header.Parallelism), argon2KeyLen)
}

func backupKeyID(key []byte) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("key-id"))
	return hex.EncodeToString(mac.Sum(nil)[:4])
}

func encodeBackupHeader(header BackupEncryptionHeader) []byte {
	encoded := make([]byte, backupHeaderFixedLen+len(header.BaseName))
	copy(encoded[:5], backupMagic)
	copy(encoded[5:21], header.Salt)
	binary.BigEndian.PutUint32(encoded[21:25], header.MemoryKiB)
	binary.BigEndian.PutUint32(encoded[25:29], header.Iterations)
	binary.BigEndian.PutUint32(encoded[29:33], header.Parallelism)
	copy(encoded[33:45], header.Nonce)
	copy(encoded[45:53], header.KeyID)
	binary.BigEndian.PutUint16(encoded[53:55], uint16(len(header.BaseName)))
	copy(encoded[55:], header.BaseName)
	return encoded
}

func validateBackupBaseName(baseName string) error {
	if len(baseName) > backupHeaderMaxBase {
		return fmt.Errorf("base name exceeds %d bytes", backupHeaderMaxBase)
	}
	if !utf8.ValidString(baseName) {
		return fmt.Errorf("base name is not valid UTF-8")
	}
	if strings.ContainsAny(baseName, "/\\\x00") {
		return fmt.Errorf("base name must not contain path separators or NUL")
	}
	return nil
}

func validateArgon2Params(memoryKiB, iterations, parallelism uint32) error {
	if parallelism == 0 || parallelism > backupMaxParallelism {
		return fmt.Errorf("invalid Argon2id parallelism: must be between 1 and %d", backupMaxParallelism)
	}
	if iterations == 0 || iterations > backupMaxIterations {
		return fmt.Errorf("invalid Argon2id iterations: must be between 1 and %d", backupMaxIterations)
	}
	if memoryKiB < 8*parallelism || memoryKiB > backupMaxMemoryKiB {
		return fmt.Errorf("invalid Argon2id memory: must be at least 8 KiB per lane and at most %d KiB", backupMaxMemoryKiB)
	}
	return nil
}

// EncryptFile encrypts a file and writes to output path.
func EncryptFile(inputPath, outputPath, password string) error {
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read input file: %w", err)
	}
	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}
	if err := writeFileAtomic(outputPath, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to write encrypted file: %w", err)
	}
	return nil
}

// DecryptFile decrypts a file and writes to output path.
func DecryptFile(inputPath, outputPath, password string) error {
	encrypted, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("failed to read encrypted file: %w", err)
	}
	decrypted, err := DecryptBackup(encrypted, password)
	if err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}
	if err := writeFileAtomic(outputPath, decrypted, 0600); err != nil {
		return fmt.Errorf("failed to write decrypted file: %w", err)
	}
	return nil
}
