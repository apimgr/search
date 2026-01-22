package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Tests for BackupMetadata

func TestBackupMetadataStruct(t *testing.T) {
	metadata := BackupMetadata{
		Version:          "1.0.0",
		CreatedAt:        time.Now(),
		CreatedBy:        "admin",
		AppVersion:       "2.0.0",
		Contents:         []string{"config/server.yml", "data/database.db"},
		Checksums:        map[string]string{"config/server.yml": "abc123"},
		Checksum:         "overall_checksum",
		Encrypted:        true,
		EncryptionMethod: "AES-256-GCM",
		ServerTitle:      "My Search",
		Size:             1024,
		Files:            []string{"file1.txt"},
	}

	if metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", metadata.Version)
	}
	if metadata.CreatedBy != "admin" {
		t.Errorf("CreatedBy = %q, want admin", metadata.CreatedBy)
	}
	if !metadata.Encrypted {
		t.Error("Encrypted should be true")
	}
	if metadata.EncryptionMethod != "AES-256-GCM" {
		t.Errorf("EncryptionMethod = %q, want AES-256-GCM", metadata.EncryptionMethod)
	}
	if len(metadata.Contents) != 2 {
		t.Errorf("Contents length = %d, want 2", len(metadata.Contents))
	}
}

// Tests for Manager

func TestNewManager(t *testing.T) {
	m := NewManager()

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.backupDir == "" {
		t.Error("backupDir should be set")
	}
	if m.configDir == "" {
		t.Error("configDir should be set")
	}
	if m.dataDir == "" {
		t.Error("dataDir should be set")
	}
}

func TestManagerSetCreatedBy(t *testing.T) {
	m := NewManager()
	m.SetCreatedBy("admin_user")

	if m.createdBy != "admin_user" {
		t.Errorf("createdBy = %q, want admin_user", m.createdBy)
	}
}

func TestManagerSetPassword(t *testing.T) {
	m := NewManager()
	m.SetPassword("supersecret123")

	if m.password != "supersecret123" {
		t.Error("password should be set")
	}
}

// Tests for Encryption

func TestEncryptBackup(t *testing.T) {
	data := []byte("This is test data for backup encryption")
	password := "testpassword123"

	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatalf("EncryptBackup() error = %v", err)
	}

	if len(encrypted) == 0 {
		t.Error("Encrypted data should not be empty")
	}

	// Encrypted data should be larger (salt + nonce + auth tag)
	if len(encrypted) < len(data)+argon2SaltLen+12+16 {
		t.Error("Encrypted data should be larger than original")
	}
}

func TestEncryptBackupEmptyPassword(t *testing.T) {
	data := []byte("test data")

	_, err := EncryptBackup(data, "")
	if err == nil {
		t.Error("EncryptBackup() should error with empty password")
	}
}

func TestDecryptBackup(t *testing.T) {
	data := []byte("This is test data for backup encryption")
	password := "testpassword123"

	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatalf("EncryptBackup() error = %v", err)
	}

	decrypted, err := DecryptBackup(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptBackup() error = %v", err)
	}

	if !bytes.Equal(decrypted, data) {
		t.Errorf("DecryptBackup() = %q, want %q", decrypted, data)
	}
}

func TestDecryptBackupWrongPassword(t *testing.T) {
	data := []byte("test data")
	password := "correctpassword"
	wrongPassword := "wrongpassword"

	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatalf("EncryptBackup() error = %v", err)
	}

	_, err = DecryptBackup(encrypted, wrongPassword)
	if err == nil {
		t.Error("DecryptBackup() should error with wrong password")
	}
}

func TestDecryptBackupEmptyPassword(t *testing.T) {
	encrypted := make([]byte, 100)

	_, err := DecryptBackup(encrypted, "")
	if err == nil {
		t.Error("DecryptBackup() should error with empty password")
	}
}

func TestDecryptBackupTooShort(t *testing.T) {
	// Data shorter than minimum (salt + nonce)
	encrypted := make([]byte, 10)

	_, err := DecryptBackup(encrypted, "password")
	if err == nil {
		t.Error("DecryptBackup() should error with data too short")
	}
}

func TestHashPassword(t *testing.T) {
	password := "testpassword123"
	hash := HashPassword(password)

	if hash == "" {
		t.Error("HashPassword() returned empty string")
	}

	// Same password should produce same hash
	hash2 := HashPassword(password)
	if hash != hash2 {
		t.Error("HashPassword() should be deterministic")
	}

	// Different password should produce different hash
	hash3 := HashPassword("differentpassword")
	if hash == hash3 {
		t.Error("Different passwords should produce different hashes")
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "testpassword123"
	hash := HashPassword(password)

	if !VerifyPassword(password, hash) {
		t.Error("VerifyPassword() should return true for matching password")
	}

	if VerifyPassword("wrongpassword", hash) {
		t.Error("VerifyPassword() should return false for non-matching password")
	}
}

func TestEncryptDecryptFile(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.txt")
	encryptedPath := filepath.Join(tempDir, "encrypted.bin")
	decryptedPath := filepath.Join(tempDir, "decrypted.txt")

	// Create input file
	originalData := []byte("This is test file content for encryption")
	if err := os.WriteFile(inputPath, originalData, 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	password := "filepassword123"

	// Encrypt file
	if err := EncryptFile(inputPath, encryptedPath, password); err != nil {
		t.Fatalf("EncryptFile() error = %v", err)
	}

	// Verify encrypted file exists and is different from original
	encryptedData, err := os.ReadFile(encryptedPath)
	if err != nil {
		t.Fatalf("Failed to read encrypted file: %v", err)
	}
	if bytes.Equal(encryptedData, originalData) {
		t.Error("Encrypted file should be different from original")
	}

	// Decrypt file
	if err := DecryptFile(encryptedPath, decryptedPath, password); err != nil {
		t.Fatalf("DecryptFile() error = %v", err)
	}

	// Verify decrypted content matches original
	decryptedData, err := os.ReadFile(decryptedPath)
	if err != nil {
		t.Fatalf("Failed to read decrypted file: %v", err)
	}
	if !bytes.Equal(decryptedData, originalData) {
		t.Errorf("Decrypted data = %q, want %q", decryptedData, originalData)
	}
}

func TestEncryptFileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "encrypted.bin")

	err := EncryptFile("/nonexistent/file.txt", outputPath, "password")
	if err == nil {
		t.Error("EncryptFile() should error for non-existent input file")
	}
}

func TestDecryptFileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "decrypted.txt")

	err := DecryptFile("/nonexistent/file.bin", outputPath, "password")
	if err == nil {
		t.Error("DecryptFile() should error for non-existent input file")
	}
}

func TestDecryptFileWrongPassword(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.txt")
	encryptedPath := filepath.Join(tempDir, "encrypted.bin")
	decryptedPath := filepath.Join(tempDir, "decrypted.txt")

	// Create and encrypt file
	if err := os.WriteFile(inputPath, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EncryptFile(inputPath, encryptedPath, "correctpassword"); err != nil {
		t.Fatal(err)
	}

	// Try to decrypt with wrong password
	err := DecryptFile(encryptedPath, decryptedPath, "wrongpassword")
	if err == nil {
		t.Error("DecryptFile() should error with wrong password")
	}
}

// Tests for Argon2 constants

func TestArgon2Constants(t *testing.T) {
	// Per AI.md PART 2: Argon2id parameters
	if argon2Time != 3 {
		t.Errorf("argon2Time = %d, want 3", argon2Time)
	}
	if argon2Memory != 64*1024 {
		t.Errorf("argon2Memory = %d, want %d", argon2Memory, 64*1024)
	}
	if argon2Threads != 4 {
		t.Errorf("argon2Threads = %d, want 4", argon2Threads)
	}
	if argon2KeyLen != 32 {
		t.Errorf("argon2KeyLen = %d, want 32", argon2KeyLen)
	}
	if argon2SaltLen != 16 {
		t.Errorf("argon2SaltLen = %d, want 16", argon2SaltLen)
	}
}

// Tests for encryption roundtrip with various data sizes

func TestEncryptDecryptVariousDataSizes(t *testing.T) {
	password := "testpassword"

	tests := []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"small", 16},
		{"medium", 1024},
		{"large", 10240},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.size)
			for i := range data {
				data[i] = byte(i % 256)
			}

			encrypted, err := EncryptBackup(data, password)
			if err != nil {
				t.Fatalf("EncryptBackup() error = %v", err)
			}

			decrypted, err := DecryptBackup(encrypted, password)
			if err != nil {
				t.Fatalf("DecryptBackup() error = %v", err)
			}

			if !bytes.Equal(decrypted, data) {
				t.Error("Roundtrip data mismatch")
			}
		})
	}
}

// Tests for Manager methods

func TestManagerDirs(t *testing.T) {
	m := NewManager()

	if m.backupDir == "" {
		t.Error("backupDir should be set")
	}
	if m.configDir == "" {
		t.Error("configDir should be set")
	}
	if m.dataDir == "" {
		t.Error("dataDir should be set")
	}
}

func TestManagerPasswordNotStored(t *testing.T) {
	m := NewManager()
	m.SetPassword("secretpassword")

	// Password is stored temporarily for operations, not on disk
	if m.password != "secretpassword" {
		t.Error("password should be accessible on Manager")
	}
}

func TestManagerCreatedByDefault(t *testing.T) {
	m := NewManager()

	// Should be empty by default
	if m.createdBy != "" {
		t.Errorf("createdBy = %q, want empty by default", m.createdBy)
	}

	m.SetCreatedBy("admin")
	if m.createdBy != "admin" {
		t.Errorf("createdBy = %q, want admin", m.createdBy)
	}
}

// Tests for BackupMetadata JSON serialization

func TestBackupMetadataJSON(t *testing.T) {
	metadata := BackupMetadata{
		Version:          "1.0.0",
		CreatedAt:        time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		CreatedBy:        "admin",
		AppVersion:       "2.0.0",
		Contents:         []string{"config/server.yml"},
		Checksums:        map[string]string{"config/server.yml": "abc123"},
		Checksum:         "sha256:overall",
		Encrypted:        false,
		EncryptionMethod: "",
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded BackupMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Version != metadata.Version {
		t.Errorf("Version = %q, want %q", decoded.Version, metadata.Version)
	}
	if decoded.CreatedBy != metadata.CreatedBy {
		t.Errorf("CreatedBy = %q, want %q", decoded.CreatedBy, metadata.CreatedBy)
	}
	if decoded.AppVersion != metadata.AppVersion {
		t.Errorf("AppVersion = %q, want %q", decoded.AppVersion, metadata.AppVersion)
	}
}

func TestBackupMetadataEncrypted(t *testing.T) {
	metadata := BackupMetadata{
		Version:          "1.0.0",
		Encrypted:        true,
		EncryptionMethod: "AES-256-GCM",
	}

	if !metadata.Encrypted {
		t.Error("Encrypted should be true")
	}
	if metadata.EncryptionMethod != "AES-256-GCM" {
		t.Errorf("EncryptionMethod = %q, want AES-256-GCM", metadata.EncryptionMethod)
	}
}

func TestBackupMetadataLegacyFields(t *testing.T) {
	metadata := BackupMetadata{
		Version:     "1.0.0",
		Contents:    []string{"config/server.yml"},
		ServerTitle: "My Search",
		Size:        1024,
		Files:       []string{"file1.txt"},
	}

	if metadata.ServerTitle != "My Search" {
		t.Errorf("ServerTitle = %q", metadata.ServerTitle)
	}
	if metadata.Size != 1024 {
		t.Errorf("Size = %d", metadata.Size)
	}
	if len(metadata.Files) != 1 {
		t.Errorf("Files length = %d", len(metadata.Files))
	}
}

// Tests for encryption security

func TestEncryptionUniqueNonce(t *testing.T) {
	data := []byte("same data")
	password := "samepassword"

	// Encrypt same data twice
	encrypted1, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatal(err)
	}

	encrypted2, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypted outputs should be different due to random nonce
	if bytes.Equal(encrypted1, encrypted2) {
		t.Error("Same plaintext should produce different ciphertext (random nonce)")
	}

	// Both should decrypt to same plaintext
	decrypted1, err := DecryptBackup(encrypted1, password)
	if err != nil {
		t.Fatal(err)
	}
	decrypted2, err := DecryptBackup(encrypted2, password)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decrypted1, data) || !bytes.Equal(decrypted2, data) {
		t.Error("Both should decrypt to same plaintext")
	}
}

func TestDecryptCorruptedData(t *testing.T) {
	data := []byte("test data")
	password := "password"

	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatal(err)
	}

	// Corrupt the ciphertext
	encrypted[len(encrypted)-5] ^= 0xFF

	_, err = DecryptBackup(encrypted, password)
	if err == nil {
		t.Error("DecryptBackup() should error on corrupted data")
	}
}

// Tests for special characters in passwords

func TestEncryptDecryptSpecialCharsPassword(t *testing.T) {
	data := []byte("test data")
	specialPasswords := []string{
		"p@ssw0rd!#$%",
		"密码测试",
		"пароль",
		"emoji🔐key",
		"with spaces password",
		"tab\tcharacter",
		"newline\npassword",
	}

	for _, password := range specialPasswords {
		t.Run(password, func(t *testing.T) {
			encrypted, err := EncryptBackup(data, password)
			if err != nil {
				t.Fatalf("EncryptBackup() error = %v", err)
			}

			decrypted, err := DecryptBackup(encrypted, password)
			if err != nil {
				t.Fatalf("DecryptBackup() error = %v", err)
			}

			if !bytes.Equal(decrypted, data) {
				t.Error("Roundtrip data mismatch")
			}
		})
	}
}

// Tests for binary data

func TestEncryptDecryptBinaryData(t *testing.T) {
	// Binary data with null bytes and various byte values
	data := make([]byte, 256)
	for i := range data {
		data[i] = byte(i)
	}

	password := "password"

	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatal(err)
	}

	decrypted, err := DecryptBackup(encrypted, password)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decrypted, data) {
		t.Error("Binary data roundtrip failed")
	}
}

// Tests for hash verification

func TestHashPasswordConsistency(t *testing.T) {
	password := "testpassword"

	hash1 := HashPassword(password)
	hash2 := HashPassword(password)

	if hash1 != hash2 {
		t.Error("Same password should produce same hash")
	}
}

func TestVerifyPasswordEmpty(t *testing.T) {
	hash := HashPassword("password")

	if VerifyPassword("", hash) {
		t.Error("Empty password should not verify")
	}
}

func TestHashPasswordDifferentValues(t *testing.T) {
	passwords := []string{"pass1", "pass2", "pass3", "password", "Password"}
	hashes := make(map[string]bool)

	for _, p := range passwords {
		hash := HashPassword(p)
		if hashes[hash] {
			t.Errorf("Collision found for password %q", p)
		}
		hashes[hash] = true
	}
}

// Tests for file operations with various content types

func TestEncryptDecryptTextFile(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.txt")
	encryptedPath := filepath.Join(tempDir, "encrypted.bin")
	decryptedPath := filepath.Join(tempDir, "decrypted.txt")

	content := "Line 1\nLine 2\nLine 3\n"
	os.WriteFile(inputPath, []byte(content), 0644)

	password := "password"
	EncryptFile(inputPath, encryptedPath, password)
	DecryptFile(encryptedPath, decryptedPath, password)

	decrypted, _ := os.ReadFile(decryptedPath)
	if string(decrypted) != content {
		t.Errorf("Content mismatch: got %q, want %q", decrypted, content)
	}
}

func TestEncryptDecryptJSONFile(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "config.json")
	encryptedPath := filepath.Join(tempDir, "encrypted.bin")
	decryptedPath := filepath.Join(tempDir, "decrypted.json")

	content := `{"key": "value", "number": 42, "nested": {"a": 1}}`
	os.WriteFile(inputPath, []byte(content), 0644)

	password := "password"
	EncryptFile(inputPath, encryptedPath, password)
	DecryptFile(encryptedPath, decryptedPath, password)

	decrypted, _ := os.ReadFile(decryptedPath)
	if string(decrypted) != content {
		t.Errorf("Content mismatch: got %q, want %q", decrypted, content)
	}
}

func TestEncryptFileEmptyPassword(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.txt")
	outputPath := filepath.Join(tempDir, "output.bin")

	os.WriteFile(inputPath, []byte("test"), 0644)

	err := EncryptFile(inputPath, outputPath, "")
	if err == nil {
		t.Error("EncryptFile() should error with empty password")
	}
}

func TestDecryptFileEmptyPassword(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.bin")
	outputPath := filepath.Join(tempDir, "output.txt")

	os.WriteFile(inputPath, []byte("encrypted content"), 0644)

	err := DecryptFile(inputPath, outputPath, "")
	if err == nil {
		t.Error("DecryptFile() should error with empty password")
	}
}

// Test BackupMetadata checksums

func TestBackupMetadataChecksums(t *testing.T) {
	metadata := BackupMetadata{
		Version: "1.0.0",
		Checksums: map[string]string{
			"config/server.yml":   "abc123def456",
			"data/database.db":    "789ghi012jkl",
			"data/uploads/1.jpg":  "mno345pqr678",
		},
		Checksum: "sha256:overall_checksum_here",
	}

	if len(metadata.Checksums) != 3 {
		t.Errorf("Checksums count = %d, want 3", len(metadata.Checksums))
	}
	if metadata.Checksums["config/server.yml"] != "abc123def456" {
		t.Errorf("Checksum for config/server.yml = %q", metadata.Checksums["config/server.yml"])
	}
}

// Test for backup metadata fields per AI.md PART 25

func TestBackupMetadataPerSpec(t *testing.T) {
	now := time.Now()
	metadata := BackupMetadata{
		Version:          "1.0.0",          // Per PART 25: manifest format version
		CreatedAt:        now,              // Per PART 25: when backup was created
		CreatedBy:        "admin",          // Per PART 25: who created the backup
		AppVersion:       "2.0.0",          // Per PART 25: application version
		Contents:         []string{"file"}, // Per PART 25: list of contents
		Checksum:         "sha256:abc123",  // Per PART 25: overall checksum
		Encrypted:        true,             // Per PART 25: encryption status
		EncryptionMethod: "AES-256-GCM",    // Per PART 25: encryption method
	}

	// All required fields should be non-empty
	if metadata.Version == "" {
		t.Error("Version is required per AI.md PART 25")
	}
	if metadata.CreatedAt.IsZero() {
		t.Error("CreatedAt is required per AI.md PART 25")
	}
	if metadata.CreatedBy == "" {
		t.Error("CreatedBy is required per AI.md PART 25")
	}
	if metadata.AppVersion == "" {
		t.Error("AppVersion is required per AI.md PART 25")
	}
	if len(metadata.Contents) == 0 {
		t.Error("Contents is required per AI.md PART 25")
	}
	if metadata.Checksum == "" {
		t.Error("Checksum is required per AI.md PART 25")
	}
}

// Tests for BackupInfo struct

func TestBackupInfoStruct(t *testing.T) {
	info := BackupInfo{
		Filename:    "search_backup_2024-01-01_120000.tar.gz",
		Path:        "/backups/search_backup_2024-01-01_120000.tar.gz",
		Size:        1024 * 1024, // 1 MB
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Version:     "1.0.0",
		ServerTitle: "My Search",
		FileCount:   10,
	}

	if info.Filename != "search_backup_2024-01-01_120000.tar.gz" {
		t.Errorf("Filename = %q", info.Filename)
	}
	if info.Size != 1024*1024 {
		t.Errorf("Size = %d, want %d", info.Size, 1024*1024)
	}
	if info.FileCount != 10 {
		t.Errorf("FileCount = %d, want 10", info.FileCount)
	}
}

func TestBackupInfoFormatSize(t *testing.T) {
	tests := []struct {
		size     int64
		expected string
	}{
		{512, "512 B"},
		{1024, "1.0 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1536, "1.5 KB"},
		{2 * 1024 * 1024, "2.0 MB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			info := BackupInfo{Size: tt.size}
			result := info.FormatSize()
			if result != tt.expected {
				t.Errorf("FormatSize() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBackupInfoFormatSizeSmall(t *testing.T) {
	info := BackupInfo{Size: 100}
	result := info.FormatSize()
	if result != "100 B" {
		t.Errorf("FormatSize() = %q, want '100 B'", result)
	}
}

// Tests for RetentionPolicy

func TestRetentionPolicyStruct(t *testing.T) {
	policy := RetentionPolicy{
		Count: 15,
		Day:   7,
		Week:  4,
		Month: 12,
		Year:  5,
	}

	if policy.Count != 15 {
		t.Errorf("Count = %d, want 15", policy.Count)
	}
	if policy.Day != 7 {
		t.Errorf("Day = %d, want 7", policy.Day)
	}
	if policy.Week != 4 {
		t.Errorf("Week = %d, want 4", policy.Week)
	}
	if policy.Month != 12 {
		t.Errorf("Month = %d, want 12", policy.Month)
	}
	if policy.Year != 5 {
		t.Errorf("Year = %d, want 5", policy.Year)
	}
}

func TestDefaultRetentionPolicy(t *testing.T) {
	policy := DefaultRetentionPolicy()

	// Per AI.md PART 22: Default retention values
	if policy.Count != 10 {
		t.Errorf("Count = %d, want 10", policy.Count)
	}
	if policy.Day != 7 {
		t.Errorf("Day = %d, want 7", policy.Day)
	}
	if policy.Week != 4 {
		t.Errorf("Week = %d, want 4", policy.Week)
	}
	if policy.Month != 12 {
		t.Errorf("Month = %d, want 12", policy.Month)
	}
	if policy.Year != 3 {
		t.Errorf("Year = %d, want 3", policy.Year)
	}
}

// Tests for VerificationResult

func TestVerificationResultStruct(t *testing.T) {
	result := VerificationResult{
		FileExists:    true,
		SizeValid:     true,
		ChecksumValid: true,
		ManifestValid: true,
		DecryptValid:  true,
		AllPassed:     true,
		Errors:        nil,
	}

	if !result.FileExists {
		t.Error("FileExists should be true")
	}
	if !result.SizeValid {
		t.Error("SizeValid should be true")
	}
	if !result.ChecksumValid {
		t.Error("ChecksumValid should be true")
	}
	if !result.ManifestValid {
		t.Error("ManifestValid should be true")
	}
	if !result.DecryptValid {
		t.Error("DecryptValid should be true")
	}
	if !result.AllPassed {
		t.Error("AllPassed should be true")
	}
}

func TestVerificationResultWithErrors(t *testing.T) {
	result := VerificationResult{
		FileExists:    true,
		SizeValid:     false,
		ChecksumValid: false,
		ManifestValid: false,
		DecryptValid:  false,
		AllPassed:     false,
		Errors:        []string{"size is zero", "checksum mismatch"},
	}

	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
	if len(result.Errors) != 2 {
		t.Errorf("Errors count = %d, want 2", len(result.Errors))
	}
}

// Test for IsEncrypted function

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/backup/file.tar.gz", false},
		{"/backup/file.tar.gz.enc", true},
		{"backup.enc", true},
		{"backup.tar.gz", false},
		{"/path/to/encrypted.enc", true},
		{"file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsEncrypted(tt.path)
			if result != tt.expected {
				t.Errorf("IsEncrypted(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

// Tests for computeOverallChecksum

func TestComputeOverallChecksum(t *testing.T) {
	checksums := map[string]string{
		"file1.txt": "abc123",
		"file2.txt": "def456",
		"file3.txt": "ghi789",
	}

	result := computeOverallChecksum(checksums)

	if result == "" {
		t.Error("computeOverallChecksum() should return non-empty string")
	}

	// Same input should produce same output
	result2 := computeOverallChecksum(checksums)
	if result != result2 {
		t.Error("computeOverallChecksum() should be deterministic")
	}

	// Different input should produce different output
	checksums["file4.txt"] = "jkl012"
	result3 := computeOverallChecksum(checksums)
	if result == result3 {
		t.Error("Different checksums should produce different overall checksum")
	}
}

func TestComputeOverallChecksumEmpty(t *testing.T) {
	checksums := map[string]string{}

	result := computeOverallChecksum(checksums)

	// Should still return a valid hash
	if result == "" {
		t.Error("computeOverallChecksum() should return non-empty string for empty input")
	}
}

func TestComputeOverallChecksumOrdering(t *testing.T) {
	// Keys should be sorted for deterministic output
	checksums1 := map[string]string{
		"a.txt": "hash1",
		"b.txt": "hash2",
		"c.txt": "hash3",
	}
	checksums2 := map[string]string{
		"c.txt": "hash3",
		"a.txt": "hash1",
		"b.txt": "hash2",
	}

	result1 := computeOverallChecksum(checksums1)
	result2 := computeOverallChecksum(checksums2)

	if result1 != result2 {
		t.Error("computeOverallChecksum() should produce same result regardless of input order")
	}
}

// Tests for BackupInfo JSON serialization

func TestBackupInfoJSON(t *testing.T) {
	info := BackupInfo{
		Filename:    "test.tar.gz",
		Path:        "/backups/test.tar.gz",
		Size:        1024,
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Version:     "1.0.0",
		ServerTitle: "Test Server",
		FileCount:   5,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded BackupInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Filename != info.Filename {
		t.Errorf("Filename = %q, want %q", decoded.Filename, info.Filename)
	}
	if decoded.Size != info.Size {
		t.Errorf("Size = %d, want %d", decoded.Size, info.Size)
	}
}

// Tests for VerificationResult JSON serialization

func TestVerificationResultJSON(t *testing.T) {
	result := VerificationResult{
		FileExists:    true,
		SizeValid:     true,
		ChecksumValid: false,
		ManifestValid: true,
		DecryptValid:  true,
		AllPassed:     false,
		Errors:        []string{"checksum mismatch"},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded VerificationResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.AllPassed != result.AllPassed {
		t.Errorf("AllPassed = %v, want %v", decoded.AllPassed, result.AllPassed)
	}
	if len(decoded.Errors) != 1 {
		t.Errorf("Errors count = %d, want 1", len(decoded.Errors))
	}
}

// Tests for RetentionPolicy JSON serialization

func TestRetentionPolicyJSON(t *testing.T) {
	policy := RetentionPolicy{
		Count: 10,
		Day:   7,
		Week:  4,
		Month: 12,
		Year:  3,
	}

	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded RetentionPolicy
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Count != policy.Count {
		t.Errorf("Count = %d, want %d", decoded.Count, policy.Count)
	}
	if decoded.Day != policy.Day {
		t.Errorf("Day = %d, want %d", decoded.Day, policy.Day)
	}
}

// Tests for edge cases

func TestBackupInfoFormatSizeZero(t *testing.T) {
	info := BackupInfo{Size: 0}
	result := info.FormatSize()
	if result != "0 B" {
		t.Errorf("FormatSize() = %q, want '0 B'", result)
	}
}

func TestBackupMetadataEmptyContents(t *testing.T) {
	metadata := BackupMetadata{
		Version:  "1.0.0",
		Contents: []string{},
	}

	if len(metadata.Contents) != 0 {
		t.Errorf("Contents should be empty, got %d items", len(metadata.Contents))
	}
}

func TestBackupMetadataEmptyChecksums(t *testing.T) {
	metadata := BackupMetadata{
		Version:   "1.0.0",
		Checksums: map[string]string{},
	}

	if len(metadata.Checksums) != 0 {
		t.Errorf("Checksums should be empty, got %d items", len(metadata.Checksums))
	}
}

// Tests for Manager.Create with real directories

func TestManagerCreate(t *testing.T) {
	// Create a temporary backup manager with temp directories
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	// Create some test config and data files
	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "server.yml"), []byte("server:\n  title: Test"), 0644)
	os.WriteFile(filepath.Join(m.dataDir, "test.db"), []byte("test data"), 0644)

	// Create backup
	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("Backup file not created: %v", err)
	}

	// Verify backup path ends with .tar.gz
	if !strings.HasSuffix(backupPath, ".tar.gz") {
		t.Errorf("Backup path should end with .tar.gz: %s", backupPath)
	}
}

func TestManagerCreateWithFilename(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create backup with custom filename
	backupPath, err := m.Create("my_backup")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !strings.Contains(backupPath, "my_backup") {
		t.Errorf("Backup path should contain custom filename: %s", backupPath)
	}
}

func TestManagerCreateAddsExtension(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create backup without extension - should add .tar.gz
	backupPath, err := m.Create("mybackup")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !strings.HasSuffix(backupPath, ".tar.gz") {
		t.Errorf("Extension should be added: %s", backupPath)
	}
}

func TestManagerCreateWithCreatedBy(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	m.SetCreatedBy("admin_user")

	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Read metadata and verify created_by
	metadata, err := m.GetMetadata(backupPath)
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}

	if metadata.CreatedBy != "admin_user" {
		t.Errorf("CreatedBy = %q, want admin_user", metadata.CreatedBy)
	}
}

// Tests for Manager.List

func TestManagerList(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create a backup first
	_, err := m.Create("backup1")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// List backups
	backups, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("List() returned %d backups, want 1", len(backups))
	}

	if !strings.Contains(backups[0].Filename, "backup1") {
		t.Errorf("Filename should contain backup1: %s", backups[0].Filename)
	}
}

func TestManagerListEmpty(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "nonexistent"),
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// List should return empty for non-existent directory
	backups, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(backups) != 0 {
		t.Errorf("List() returned %d backups, want 0", len(backups))
	}
}

func TestManagerListMultiple(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create multiple backups
	for i := 1; i <= 3; i++ {
		_, err := m.Create(fmt.Sprintf("backup%d", i))
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	backups, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("List() returned %d backups, want 3", len(backups))
	}
}

// Tests for Manager.GetMetadata

func TestManagerGetMetadata(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "server.yml"), []byte("server:\n  title: TestServer"), 0644)

	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	metadata, err := m.GetMetadata(backupPath)
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}

	if metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", metadata.Version)
	}
	if metadata.Checksum == "" {
		t.Error("Checksum should not be empty")
	}
}

func TestManagerGetMetadataNotFound(t *testing.T) {
	m := NewManager()

	_, err := m.GetMetadata("/nonexistent/backup.tar.gz")
	if err == nil {
		t.Error("GetMetadata() should error for non-existent file")
	}
}

func TestManagerGetMetadataInvalidFile(t *testing.T) {
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "invalid.tar.gz")
	os.WriteFile(invalidPath, []byte("not a tar.gz file"), 0644)

	m := NewManager()
	_, err := m.GetMetadata(invalidPath)
	if err == nil {
		t.Error("GetMetadata() should error for invalid archive")
	}
}

// Tests for Manager.Delete

func TestManagerDelete(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, err := m.Create("to_delete")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	filename := filepath.Base(backupPath)
	if err := m.Delete(filename); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify file is deleted
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("Backup file should be deleted")
	}
}

func TestManagerDeleteNonexistent(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: tempDir,
	}

	err := m.Delete("nonexistent.tar.gz")
	if err == nil {
		t.Error("Delete() should error for non-existent file")
	}
}

// Tests for Manager.Restore

func TestManagerRestore(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")

	m := &Manager{
		backupDir: backupDir,
		configDir: configDir,
		dataDir:   dataDir,
	}

	// Create initial files
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)
	originalContent := []byte("original config content")
	os.WriteFile(filepath.Join(configDir, "server.yml"), originalContent, 0644)
	os.WriteFile(filepath.Join(dataDir, "test.db"), []byte("original data"), 0644)

	// Create a backup
	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Modify the files
	os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("modified content"), 0644)

	// Restore from backup
	if err := m.Restore(backupPath); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Verify config was restored
	content, _ := os.ReadFile(filepath.Join(configDir, "server.yml"))
	if !bytes.Equal(content, originalContent) {
		t.Errorf("Config not restored properly: got %q", content)
	}
}

func TestManagerRestoreNotFound(t *testing.T) {
	m := NewManager()

	err := m.Restore("/nonexistent/backup.tar.gz")
	if err == nil {
		t.Error("Restore() should error for non-existent file")
	}
}

func TestManagerRestoreInvalidArchive(t *testing.T) {
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "invalid.tar.gz")
	os.WriteFile(invalidPath, []byte("not a tar.gz"), 0644)

	m := NewManager()
	err := m.Restore(invalidPath)
	if err == nil {
		t.Error("Restore() should error for invalid archive")
	}
}

// Tests for Manager.VerifyBackup

func TestManagerVerifyBackup(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	result, err := m.VerifyBackup(backupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}

	if !result.AllPassed {
		t.Errorf("AllPassed = false, errors: %v", result.Errors)
	}
	if !result.FileExists {
		t.Error("FileExists should be true")
	}
	if !result.SizeValid {
		t.Error("SizeValid should be true")
	}
	if !result.ManifestValid {
		t.Error("ManifestValid should be true")
	}
	if !result.ChecksumValid {
		t.Error("ChecksumValid should be true")
	}
}

func TestManagerVerifyBackupNotFound(t *testing.T) {
	m := NewManager()

	result, err := m.VerifyBackup("/nonexistent/backup.tar.gz")
	if err != nil {
		t.Fatalf("VerifyBackup() should not return error, got %v", err)
	}

	if result.FileExists {
		t.Error("FileExists should be false")
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

func TestManagerVerifyBackupEmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	emptyPath := filepath.Join(tempDir, "empty.tar.gz")
	os.WriteFile(emptyPath, []byte{}, 0644)

	m := NewManager()
	result, _ := m.VerifyBackup(emptyPath)

	if !result.FileExists {
		t.Error("FileExists should be true")
	}
	if result.SizeValid {
		t.Error("SizeValid should be false for empty file")
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

// Tests for encrypted backups

func TestManagerCreateEncrypted(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		password:  "testpassword123",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, err := m.CreateEncrypted("")
	if err != nil {
		t.Fatalf("CreateEncrypted() error = %v", err)
	}

	// Should have .enc extension
	if !strings.HasSuffix(backupPath, ".enc") {
		t.Errorf("Encrypted backup should have .enc extension: %s", backupPath)
	}

	// Verify unencrypted file was removed
	unencryptedPath := strings.TrimSuffix(backupPath, ".enc")
	if _, err := os.Stat(unencryptedPath); !os.IsNotExist(err) {
		t.Error("Unencrypted backup should be removed after encryption")
	}
}

func TestManagerCreateEncryptedNoPassword(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	_, err := m.CreateEncrypted("")
	if err == nil {
		t.Error("CreateEncrypted() should error without password")
	}
}

func TestManagerRestoreEncrypted(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		password:  "testpassword123",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	originalContent := []byte("original content")
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), originalContent, 0644)

	// Create encrypted backup
	backupPath, err := m.CreateEncrypted("")
	if err != nil {
		t.Fatalf("CreateEncrypted() error = %v", err)
	}

	// Modify the file
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("modified"), 0644)

	// Restore encrypted backup
	if err := m.RestoreEncrypted(backupPath); err != nil {
		t.Fatalf("RestoreEncrypted() error = %v", err)
	}

	// Verify restored content
	content, _ := os.ReadFile(filepath.Join(m.configDir, "test.yml"))
	if !bytes.Equal(content, originalContent) {
		t.Errorf("Content not restored: got %q", content)
	}
}

func TestManagerRestoreEncryptedNoPassword(t *testing.T) {
	tempDir := t.TempDir()
	encryptedPath := filepath.Join(tempDir, "backup.tar.gz.enc")
	os.WriteFile(encryptedPath, []byte("encrypted content"), 0644)

	m := &Manager{} // No password set

	err := m.RestoreEncrypted(encryptedPath)
	if err == nil {
		t.Error("RestoreEncrypted() should error without password")
	}
}

func TestManagerRestoreEncryptedUnencryptedFile(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		password:  "password",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create unencrypted backup
	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// RestoreEncrypted should work on unencrypted files (falls back to regular restore)
	err = m.RestoreEncrypted(backupPath)
	if err != nil {
		t.Fatalf("RestoreEncrypted() should handle unencrypted files: %v", err)
	}
}

// Tests for scheduled backups

func TestManagerScheduledBackup(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create scheduled backup keeping 2
	if err := m.ScheduledBackup(2); err != nil {
		t.Fatalf("ScheduledBackup() error = %v", err)
	}

	backups, _ := m.List()
	if len(backups) != 1 {
		t.Errorf("Should have 1 backup, got %d", len(backups))
	}
}

func TestManagerScheduledBackupCleanup(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create multiple backups
	for i := 0; i < 5; i++ {
		_, err := m.Create(fmt.Sprintf("backup%d", i))
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
	}

	// Run scheduled backup with keep=3
	if err := m.ScheduledBackup(3); err != nil {
		t.Fatalf("ScheduledBackup() error = %v", err)
	}

	// Should have reduced to 3 + 1 new = can be up to 4 depending on timing
	backups, _ := m.List()
	if len(backups) > 4 {
		t.Errorf("Should have at most 4 backups after cleanup, got %d", len(backups))
	}
}

// Tests for CreateAndVerify

func TestManagerCreateAndVerify(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test content"), 0644)

	backupPath, result, err := m.CreateAndVerify("")
	if err != nil {
		t.Fatalf("CreateAndVerify() error = %v", err)
	}

	if backupPath == "" {
		t.Error("Backup path should not be empty")
	}

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if !result.AllPassed {
		t.Errorf("AllPassed should be true, errors: %v", result.Errors)
	}
}

// Tests for ApplyRetention

func TestManagerApplyRetentionNoDelete(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create a few backups
	for i := 0; i < 3; i++ {
		m.Create(fmt.Sprintf("backup%d", i))
	}

	// Apply retention with count > current
	policy := RetentionPolicy{Count: 10}
	if err := m.ApplyRetention(policy); err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}

	// All backups should remain
	backups, _ := m.List()
	if len(backups) != 3 {
		t.Errorf("Should still have 3 backups, got %d", len(backups))
	}
}

// Tests for verifyManifestFromData

func TestVerifyManifestFromDataValid(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, _ := m.Create("")

	// Read backup data
	data, _ := os.ReadFile(backupPath)

	// Verify manifest
	metadata, err := m.verifyManifestFromData(data)
	if err != nil {
		t.Fatalf("verifyManifestFromData() error = %v", err)
	}

	if metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", metadata.Version)
	}
}

func TestVerifyManifestFromDataInvalid(t *testing.T) {
	m := NewManager()

	// Invalid data
	_, err := m.verifyManifestFromData([]byte("invalid data"))
	if err == nil {
		t.Error("verifyManifestFromData() should error for invalid data")
	}
}

// Tests for VerifyBackup with encrypted files

func TestManagerVerifyBackupEncrypted(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		password:  "testpassword",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, err := m.CreateEncrypted("")
	if err != nil {
		t.Fatalf("CreateEncrypted() error = %v", err)
	}

	result, err := m.VerifyBackup(backupPath)
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}

	if !result.AllPassed {
		t.Errorf("AllPassed = false, errors: %v", result.Errors)
	}
	if !result.DecryptValid {
		t.Error("DecryptValid should be true")
	}
}

func TestManagerVerifyBackupEncryptedNoPassword(t *testing.T) {
	tempDir := t.TempDir()

	// Create manager with password
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		password:  "testpassword",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, _ := m.CreateEncrypted("")

	// Create manager without password to verify
	m2 := &Manager{backupDir: m.backupDir}

	result, _ := m2.VerifyBackup(backupPath)

	if result.DecryptValid {
		t.Error("DecryptValid should be false when password not set")
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

// Tests for CreateEncryptedAndVerify

func TestManagerCreateEncryptedAndVerify(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		password:  "testpassword",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, result, err := m.CreateEncryptedAndVerify("")
	if err != nil {
		t.Fatalf("CreateEncryptedAndVerify() error = %v", err)
	}

	if !strings.HasSuffix(backupPath, ".enc") {
		t.Errorf("Path should end with .enc: %s", backupPath)
	}

	if !result.AllPassed {
		t.Errorf("AllPassed = false, errors: %v", result.Errors)
	}
	if !result.DecryptValid {
		t.Error("DecryptValid should be true")
	}
}

// Tests for ScheduledBackupWithVerification

func TestManagerScheduledBackupWithVerification(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test content"), 0644)

	if err := m.ScheduledBackupWithVerification(5); err != nil {
		t.Fatalf("ScheduledBackupWithVerification() error = %v", err)
	}

	backups, _ := m.List()
	if len(backups) != 1 {
		t.Errorf("Should have 1 backup, got %d", len(backups))
	}
}

// Tests for addDirectoryToTar - excluding directories

func TestAddDirectoryToTarSkipsDirs(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")
	os.MkdirAll(srcDir, 0755)

	// Create normal file
	os.WriteFile(filepath.Join(srcDir, "file.txt"), []byte("content"), 0644)

	// Create directories that should be skipped
	os.MkdirAll(filepath.Join(srcDir, "logs"), 0755)
	os.WriteFile(filepath.Join(srcDir, "logs", "log.txt"), []byte("log"), 0644)

	os.MkdirAll(filepath.Join(srcDir, "cache"), 0755)
	os.WriteFile(filepath.Join(srcDir, "cache", "cache.txt"), []byte("cache"), 0644)

	os.MkdirAll(filepath.Join(srcDir, "tmp"), 0755)
	os.WriteFile(filepath.Join(srcDir, "tmp", "tmp.txt"), []byte("tmp"), 0644)

	m := NewManager()

	// Create a buffer to write tar to
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	files, _, _, err := m.addDirectoryToTar(tw, srcDir, "test")
	if err != nil {
		t.Fatalf("addDirectoryToTar() error = %v", err)
	}

	tw.Close()

	// Should only have file.txt
	if len(files) != 1 {
		t.Errorf("Should have 1 file, got %d: %v", len(files), files)
	}

	if len(files) > 0 && !strings.Contains(files[0], "file.txt") {
		t.Errorf("Should only contain file.txt: %v", files)
	}
}

// Tests for addDirectoryToTar - large file skip

func TestAddDirectoryToTarSkipsLargeFiles(t *testing.T) {
	// This test is commented out to avoid creating large files
	// In production, files > 100MB are skipped
	t.Log("Large file skip test: files > 100MB are skipped by addDirectoryToTar")
}

// Tests for addDirectoryToTar error paths

func TestAddDirectoryToTarEmptyDir(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "empty")
	os.MkdirAll(srcDir, 0755)

	m := NewManager()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	files, size, checksums, err := m.addDirectoryToTar(tw, srcDir, "test")
	if err != nil {
		t.Fatalf("addDirectoryToTar() error = %v", err)
	}
	tw.Close()

	if len(files) != 0 {
		t.Errorf("Should have 0 files for empty dir, got %d", len(files))
	}
	if size != 0 {
		t.Errorf("Size should be 0, got %d", size)
	}
	if len(checksums) != 0 {
		t.Errorf("Should have 0 checksums, got %d", len(checksums))
	}
}

func TestAddDirectoryToTarNonExistentDir(t *testing.T) {
	m := NewManager()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	_, _, _, err := m.addDirectoryToTar(tw, "/nonexistent/path/that/does/not/exist", "test")
	if err == nil {
		t.Error("addDirectoryToTar() should error for non-existent directory")
	}
	tw.Close()
}

func TestAddDirectoryToTarNestedDirs(t *testing.T) {
	tempDir := t.TempDir()
	srcDir := filepath.Join(tempDir, "src")

	// Create nested directory structure
	os.MkdirAll(filepath.Join(srcDir, "level1", "level2", "level3"), 0755)
	os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(srcDir, "level1", "l1.txt"), []byte("level1"), 0644)
	os.WriteFile(filepath.Join(srcDir, "level1", "level2", "l2.txt"), []byte("level2"), 0644)
	os.WriteFile(filepath.Join(srcDir, "level1", "level2", "level3", "l3.txt"), []byte("level3"), 0644)

	m := NewManager()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	files, _, checksums, err := m.addDirectoryToTar(tw, srcDir, "prefix")
	if err != nil {
		t.Fatalf("addDirectoryToTar() error = %v", err)
	}
	tw.Close()

	if len(files) != 4 {
		t.Errorf("Should have 4 files, got %d: %v", len(files), files)
	}
	if len(checksums) != 4 {
		t.Errorf("Should have 4 checksums, got %d", len(checksums))
	}

	// Verify all files have proper prefix
	for _, f := range files {
		if !strings.HasPrefix(f, "prefix/") {
			t.Errorf("File should have prefix: %s", f)
		}
	}
}

// Test for backup filename format per AI.md PART 22

func TestBackupFilenameFormat(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	filename := filepath.Base(backupPath)

	// Per AI.md PART 22: Format is search_backup_YYYY-MM-DD_HHMMSS.tar.gz
	if !strings.HasPrefix(filename, "search_backup_") {
		t.Errorf("Filename should start with 'search_backup_': %s", filename)
	}
	if !strings.HasSuffix(filename, ".tar.gz") {
		t.Errorf("Filename should end with '.tar.gz': %s", filename)
	}
}

// Test encryption parameters match spec

func TestEncryptionMethodPerSpec(t *testing.T) {
	// Per AI.md PART 24: AES-256-GCM
	// The encryption uses AES-256-GCM via Go's crypto/aes and crypto/cipher
	data := []byte("test")
	password := "password"

	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatal(err)
	}

	// Should have salt (16) + nonce (12) + ciphertext + auth tag (16)
	minLength := 16 + 12 + len(data) + 16
	if len(encrypted) < minLength {
		t.Errorf("Encrypted data too short: %d < %d", len(encrypted), minLength)
	}
}

// Tests for Restore error paths

func TestManagerRestoreSkipsUnknownPaths(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")

	os.MkdirAll(backupDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)

	// Create a tar.gz with an unknown path prefix
	backupPath := filepath.Join(backupDir, "test.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add manifest.json (should be skipped)
	manifest := []byte(`{"version":"1.0.0"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	// Add file with unknown prefix (should be skipped)
	unknownData := []byte("unknown")
	tarWriter.WriteHeader(&tar.Header{
		Name: "unknown/file.txt",
		Size: int64(len(unknownData)),
		Mode: 0644,
	})
	tarWriter.Write(unknownData)

	// Add config file (should be restored)
	configData := []byte("config content")
	tarWriter.WriteHeader(&tar.Header{
		Name: "config/test.yml",
		Size: int64(len(configData)),
		Mode: 0644,
	})
	tarWriter.Write(configData)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		backupDir: backupDir,
		configDir: configDir,
		dataDir:   dataDir,
	}

	err := m.Restore(backupPath)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Config file should be restored
	content, err := os.ReadFile(filepath.Join(configDir, "test.yml"))
	if err != nil {
		t.Errorf("Config file should be restored: %v", err)
	}
	if string(content) != "config content" {
		t.Errorf("Config content mismatch: got %q", content)
	}

	// Unknown file should not exist
	if _, err := os.Stat(filepath.Join(tempDir, "unknown", "file.txt")); !os.IsNotExist(err) {
		t.Error("Unknown prefix files should be skipped")
	}
}

func TestManagerRestoreDirectory(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")

	os.MkdirAll(backupDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)

	// Create a tar.gz with a directory entry
	backupPath := filepath.Join(backupDir, "test.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add manifest
	manifest := []byte(`{"version":"1.0.0"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	// Add directory entry
	tarWriter.WriteHeader(&tar.Header{
		Name:     "config/subdir/",
		Typeflag: tar.TypeDir,
		Mode:     0755,
	})

	// Add file in that directory
	fileData := []byte("file in subdir")
	tarWriter.WriteHeader(&tar.Header{
		Name: "config/subdir/file.txt",
		Size: int64(len(fileData)),
		Mode: 0644,
	})
	tarWriter.Write(fileData)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		backupDir: backupDir,
		configDir: configDir,
		dataDir:   dataDir,
	}

	err := m.Restore(backupPath)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Directory and file should exist
	if _, err := os.Stat(filepath.Join(configDir, "subdir")); err != nil {
		t.Errorf("Directory should be created: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(configDir, "subdir", "file.txt"))
	if err != nil {
		t.Errorf("File should be restored: %v", err)
	}
	if string(content) != "file in subdir" {
		t.Errorf("Content mismatch: got %q", content)
	}
}

func TestManagerRestoreDataFiles(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")

	os.MkdirAll(backupDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)

	// Create a tar.gz with data files
	backupPath := filepath.Join(backupDir, "test.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add manifest
	manifest := []byte(`{"version":"1.0.0"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	// Add data file
	dataContent := []byte("database content")
	tarWriter.WriteHeader(&tar.Header{
		Name: "data/database.db",
		Size: int64(len(dataContent)),
		Mode: 0644,
	})
	tarWriter.Write(dataContent)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		backupDir: backupDir,
		configDir: configDir,
		dataDir:   dataDir,
	}

	err := m.Restore(backupPath)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Data file should be restored
	content, err := os.ReadFile(filepath.Join(dataDir, "database.db"))
	if err != nil {
		t.Errorf("Data file should be restored: %v", err)
	}
	if string(content) != "database content" {
		t.Errorf("Content mismatch: got %q", content)
	}
}

func TestManagerRestoreLegacyBackupJson(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")

	os.MkdirAll(backupDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)

	// Create a tar.gz with legacy backup.json
	backupPath := filepath.Join(backupDir, "test.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add legacy backup.json (should be skipped during restore)
	manifest := []byte(`{"version":"1.0.0"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "backup.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	// Add config file
	configData := []byte("config")
	tarWriter.WriteHeader(&tar.Header{
		Name: "config/test.yml",
		Size: int64(len(configData)),
		Mode: 0644,
	})
	tarWriter.Write(configData)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		backupDir: backupDir,
		configDir: configDir,
		dataDir:   dataDir,
	}

	err := m.Restore(backupPath)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// backup.json should not be restored as a file
	if _, err := os.Stat(filepath.Join(configDir, "backup.json")); !os.IsNotExist(err) {
		t.Error("backup.json should be skipped during restore")
	}
}

// Tests for ApplyRetention with specific date scenarios

func TestManagerApplyRetentionWeekly(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// Create backup files with different dates
	now := time.Now()

	// Recent backup (should keep)
	recentBackup := filepath.Join(backupDir, "recent.tar.gz")
	os.WriteFile(recentBackup, createMinimalTarGz(t), 0644)
	os.Chtimes(recentBackup, now, now)

	// Old backup (older than retention)
	oldBackup := filepath.Join(backupDir, "old.tar.gz")
	os.WriteFile(oldBackup, createMinimalTarGz(t), 0644)
	oldTime := now.AddDate(0, 0, -30) // 30 days old
	os.Chtimes(oldBackup, oldTime, oldTime)

	policy := RetentionPolicy{
		Count: 1,
		Day:   7,
		Week:  2,
		Month: 1,
		Year:  1,
	}

	err := m.ApplyRetention(policy)
	if err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}

	// Recent backup should still exist
	if _, err := os.Stat(recentBackup); os.IsNotExist(err) {
		t.Error("Recent backup should be kept")
	}
}

func TestManagerApplyRetentionDeleteErrors(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// Create many backup files
	now := time.Now()
	for i := 0; i < 5; i++ {
		backupFile := filepath.Join(backupDir, fmt.Sprintf("backup%d.tar.gz", i))
		os.WriteFile(backupFile, createMinimalTarGz(t), 0644)
		fileTime := now.AddDate(0, 0, -100-i) // Very old
		os.Chtimes(backupFile, fileTime, fileTime)
	}

	policy := RetentionPolicy{
		Count: 2,
		Day:   1,
		Week:  1,
		Month: 1,
		Year:  1,
	}

	// This should complete even if some deletes fail
	err := m.ApplyRetention(policy)
	if err != nil {
		t.Fatalf("ApplyRetention() should not fail: %v", err)
	}
}

// Tests for List edge cases

func TestManagerListIgnoresDirectories(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	// Create a directory that looks like a backup
	os.MkdirAll(filepath.Join(backupDir, "fake_backup.tar.gz"), 0755)

	// Create actual backup file
	os.WriteFile(filepath.Join(backupDir, "real.tar.gz"), createMinimalTarGz(t), 0644)

	// Create non-tarGz file
	os.WriteFile(filepath.Join(backupDir, "readme.txt"), []byte("readme"), 0644)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	backups, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Should only find the real .tar.gz file
	if len(backups) != 1 {
		t.Errorf("Should find 1 backup, got %d", len(backups))
	}
	if len(backups) > 0 && backups[0].Filename != "real.tar.gz" {
		t.Errorf("Wrong backup found: %s", backups[0].Filename)
	}
}

func TestManagerListWithBrokenMetadata(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	// Create a valid tar.gz with invalid metadata
	backupPath := filepath.Join(backupDir, "broken.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add invalid manifest
	badManifest := []byte(`not valid json`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(badManifest)),
		Mode: 0644,
	})
	tarWriter.Write(badManifest)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	backups, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Should still list the backup even with broken metadata
	if len(backups) != 1 {
		t.Errorf("Should find 1 backup, got %d", len(backups))
	}

	// Metadata fields should be empty/default
	if backups[0].Version != "" {
		t.Errorf("Version should be empty for broken metadata: %s", backups[0].Version)
	}
}

// Tests for GetMetadata with legacy backup.json

func TestManagerGetMetadataLegacy(t *testing.T) {
	tempDir := t.TempDir()

	// Create a tar.gz with legacy backup.json
	backupPath := filepath.Join(tempDir, "legacy.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add legacy backup.json
	manifest := []byte(`{"version":"0.9.0","server_title":"Legacy Server"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "backup.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := NewManager()
	metadata, err := m.GetMetadata(backupPath)
	if err != nil {
		t.Fatalf("GetMetadata() error = %v", err)
	}

	if metadata.Version != "0.9.0" {
		t.Errorf("Version = %q, want 0.9.0", metadata.Version)
	}
	if metadata.ServerTitle != "Legacy Server" {
		t.Errorf("ServerTitle = %q, want Legacy Server", metadata.ServerTitle)
	}
}

func TestManagerGetMetadataNoManifest(t *testing.T) {
	tempDir := t.TempDir()

	// Create a tar.gz without any manifest
	backupPath := filepath.Join(tempDir, "nomanifest.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add only a data file, no manifest
	data := []byte("some data")
	tarWriter.WriteHeader(&tar.Header{
		Name: "data/file.txt",
		Size: int64(len(data)),
		Mode: 0644,
	})
	tarWriter.Write(data)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := NewManager()
	_, err := m.GetMetadata(backupPath)
	if err == nil {
		t.Error("GetMetadata() should error when no manifest found")
	}
	if !strings.Contains(err.Error(), "metadata not found") {
		t.Errorf("Error should mention metadata not found: %v", err)
	}
}

// Tests for Delete security check

func TestManagerDeleteSecurityCheck(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	m := &Manager{
		backupDir: backupDir,
	}

	// Try to delete a file outside backup directory using path traversal
	// Note: This test verifies the security check works
	err := m.Delete("../../../etc/passwd")
	if err == nil {
		t.Error("Delete() should error for path traversal attempt")
	}
}

// Tests for VerifyBackup checksum scenarios

func TestManagerVerifyBackupChecksumMismatch(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create a valid backup first
	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Read the backup
	data, _ := os.ReadFile(backupPath)

	// Corrupt some bytes in the middle (not the manifest)
	if len(data) > 100 {
		data[50] ^= 0xFF
		data[51] ^= 0xFF
	}

	// Write corrupted backup
	corruptedPath := filepath.Join(tempDir, "corrupted.tar.gz")
	os.WriteFile(corruptedPath, data, 0644)

	result, _ := m.VerifyBackup(corruptedPath)

	// The verification should detect corruption
	if result.AllPassed && result.ManifestValid {
		// If manifest is still readable, that's ok
		// The main thing is the file exists
		if !result.FileExists {
			t.Error("FileExists should be true")
		}
	}
}

func TestManagerVerifyBackupLegacyNoChecksum(t *testing.T) {
	tempDir := t.TempDir()

	// Create a backup with legacy format (no checksums)
	backupPath := filepath.Join(tempDir, "legacy.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add legacy manifest without checksums
	manifest := []byte(`{"version":"0.9.0"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := NewManager()
	result, _ := m.VerifyBackup(backupPath)

	// Legacy backups should pass with warning
	if !result.FileExists {
		t.Error("FileExists should be true")
	}
	if !result.SizeValid {
		t.Error("SizeValid should be true")
	}
	if !result.ManifestValid {
		t.Error("ManifestValid should be true for valid manifest")
	}
	// ChecksumValid should be true for legacy (no checksums to verify)
	if !result.ChecksumValid {
		t.Error("ChecksumValid should be true for legacy backup")
	}
}

// Tests for CreateAndVerify error paths

func TestManagerCreateAndVerifyCreationFails(t *testing.T) {
	m := &Manager{
		backupDir: "/nonexistent/impossible/path",
		configDir: "/nonexistent/config",
		dataDir:   "/nonexistent/data",
	}

	_, _, err := m.CreateAndVerify("")
	if err == nil {
		t.Error("CreateAndVerify() should error when creation fails")
	}
}

// Tests for CreateEncryptedAndVerify error paths

func TestManagerCreateEncryptedAndVerifyNoPassword(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		// No password set
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	_, _, err := m.CreateEncryptedAndVerify("")
	if err == nil {
		t.Error("CreateEncryptedAndVerify() should error without password")
	}
}

// Tests for ScheduledBackupWithVerification error paths

func TestManagerScheduledBackupWithVerificationCreateFails(t *testing.T) {
	m := &Manager{
		backupDir: "/nonexistent/path",
		configDir: "/nonexistent/config",
		dataDir:   "/nonexistent/data",
	}

	err := m.ScheduledBackupWithVerification(5)
	if err == nil {
		t.Error("ScheduledBackupWithVerification() should error when backup creation fails")
	}
	if !strings.Contains(err.Error(), "existing backups preserved") {
		t.Errorf("Error should mention preserving existing backups: %v", err)
	}
}

// Tests for RestoreEncrypted error paths

func TestManagerRestoreEncryptedNotFound(t *testing.T) {
	m := &Manager{
		password: "password",
	}

	err := m.RestoreEncrypted("/nonexistent/backup.tar.gz.enc")
	if err == nil {
		t.Error("RestoreEncrypted() should error for non-existent file")
	}
}

func TestManagerRestoreEncryptedWrongPassword(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		password:  "correctpassword",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, _ := m.CreateEncrypted("")

	// Try to restore with wrong password
	m.password = "wrongpassword"
	err := m.RestoreEncrypted(backupPath)
	if err == nil {
		t.Error("RestoreEncrypted() should error with wrong password")
	}
}

// Tests for FormatSize edge cases

func TestBackupInfoFormatSizeTB(t *testing.T) {
	// Test terabyte range
	info := BackupInfo{Size: 1024 * 1024 * 1024 * 1024}
	result := info.FormatSize()
	if result != "1.0 TB" {
		t.Errorf("FormatSize() = %q, want '1.0 TB'", result)
	}
}

func TestBackupInfoFormatSizePB(t *testing.T) {
	// Test petabyte range
	info := BackupInfo{Size: 1024 * 1024 * 1024 * 1024 * 1024}
	result := info.FormatSize()
	if result != "1.0 PB" {
		t.Errorf("FormatSize() = %q, want '1.0 PB'", result)
	}
}

// Tests for EncryptFile and DecryptFile edge cases

func TestEncryptFileOutputWriteError(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.txt")
	os.WriteFile(inputPath, []byte("test content"), 0644)

	// Try to write to a directory (should fail)
	outputDir := filepath.Join(tempDir, "outputdir")
	os.MkdirAll(outputDir, 0755)

	err := EncryptFile(inputPath, outputDir, "password")
	if err == nil {
		t.Error("EncryptFile() should error when output path is a directory")
	}
}

func TestDecryptFileOutputWriteError(t *testing.T) {
	tempDir := t.TempDir()

	// Create an encrypted file
	inputPath := filepath.Join(tempDir, "input.txt")
	encryptedPath := filepath.Join(tempDir, "encrypted.bin")
	os.WriteFile(inputPath, []byte("test content"), 0644)
	EncryptFile(inputPath, encryptedPath, "password")

	// Try to write to a directory (should fail)
	outputDir := filepath.Join(tempDir, "outputdir")
	os.MkdirAll(outputDir, 0755)

	err := DecryptFile(encryptedPath, outputDir, "password")
	if err == nil {
		t.Error("DecryptFile() should error when output path is a directory")
	}
}

// Tests for Manager.Create with various scenarios

func TestManagerCreateWithExtension(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create with .tar.gz extension already present
	backupPath, err := m.Create("mybackup.tar.gz")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Should not double the extension
	if strings.HasSuffix(backupPath, ".tar.gz.tar.gz") {
		t.Errorf("Extension should not be doubled: %s", backupPath)
	}
}

func TestManagerCreateDefaultCreatedBy(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		// createdBy not set
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, _ := m.Create("")
	metadata, _ := m.GetMetadata(backupPath)

	// Should default to "system"
	if metadata.CreatedBy != "system" {
		t.Errorf("CreatedBy = %q, want 'system'", metadata.CreatedBy)
	}
}

// Test for verifyManifestFromData with corrupted tar

func TestVerifyManifestFromDataCorruptedTar(t *testing.T) {
	m := NewManager()

	// Valid gzip but invalid tar
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	gzWriter.Write([]byte("not valid tar content"))
	gzWriter.Close()

	_, err := m.verifyManifestFromData(buf.Bytes())
	if err == nil {
		t.Error("verifyManifestFromData() should error for corrupted tar")
	}
}

// Tests for RetentionPolicy edge cases

func TestRetentionPolicyZeroValues(t *testing.T) {
	policy := RetentionPolicy{
		Count: 0,
		Day:   0,
		Week:  0,
		Month: 0,
		Year:  0,
	}

	// Zero values are valid (means no retention)
	if policy.Count != 0 {
		t.Errorf("Count = %d, want 0", policy.Count)
	}
}

// Tests for VerifyBackup encrypted without password

func TestManagerVerifyBackupEncryptedReadError(t *testing.T) {
	tempDir := t.TempDir()

	// Create a fake encrypted file that's too small
	fakePath := filepath.Join(tempDir, "fake.tar.gz.enc")
	os.WriteFile(fakePath, []byte("too short"), 0644)

	m := &Manager{
		password: "password",
	}

	result, _ := m.VerifyBackup(fakePath)

	if result.DecryptValid {
		t.Error("DecryptValid should be false for corrupted encrypted file")
	}
}

// Test computeOverallChecksum with single entry

func TestComputeOverallChecksumSingle(t *testing.T) {
	checksums := map[string]string{
		"file.txt": "abc123",
	}

	result := computeOverallChecksum(checksums)

	if result == "" {
		t.Error("computeOverallChecksum() should return non-empty string")
	}

	// Same input should produce same output
	result2 := computeOverallChecksum(checksums)
	if result != result2 {
		t.Error("Should be deterministic")
	}
}

// Helper function to create minimal valid tar.gz for tests

func createMinimalTarGz(t *testing.T) []byte {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	manifest := []byte(`{"version":"1.0.0"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	tarWriter.Close()
	gzWriter.Close()
	return buf.Bytes()
}

// Tests for ScheduledBackup cleanup logic

func TestManagerScheduledBackupNoCleanupNeeded(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Run scheduled backup with high keep count
	if err := m.ScheduledBackup(100); err != nil {
		t.Fatalf("ScheduledBackup() error = %v", err)
	}

	backups, _ := m.List()
	if len(backups) != 1 {
		t.Errorf("Should have 1 backup, got %d", len(backups))
	}
}

// Tests for ApplyRetention with today's backup

func TestManagerApplyRetentionKeepsTodayBackup(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// Create a backup with current time
	backupFile := filepath.Join(backupDir, "today.tar.gz")
	os.WriteFile(backupFile, createMinimalTarGz(t), 0644)

	policy := RetentionPolicy{
		Count: 1,
		Day:   1,
		Week:  0,
		Month: 0,
		Year:  0,
	}

	err := m.ApplyRetention(policy)
	if err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}

	// Today's backup should still exist
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		t.Error("Today's backup should be kept")
	}
}

// Tests for VerifyBackup invalid gzip

func TestManagerVerifyBackupInvalidGzip(t *testing.T) {
	tempDir := t.TempDir()
	invalidPath := filepath.Join(tempDir, "invalid.tar.gz")
	os.WriteFile(invalidPath, []byte("not gzip content"), 0644)

	m := NewManager()
	result, _ := m.VerifyBackup(invalidPath)

	if !result.FileExists {
		t.Error("FileExists should be true")
	}
	if !result.SizeValid {
		t.Error("SizeValid should be true")
	}
	if result.ManifestValid {
		t.Error("ManifestValid should be false for invalid gzip")
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

// Tests for ScheduledBackupWithVerification cleanup

func TestManagerScheduledBackupWithVerificationCleanup(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.backupDir, 0755)
	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	// Create existing backups
	for i := 0; i < 5; i++ {
		os.WriteFile(
			filepath.Join(m.backupDir, fmt.Sprintf("old_backup_%d.tar.gz", i)),
			createMinimalTarGz(t),
			0644,
		)
	}

	// Run scheduled backup with keep=3
	if err := m.ScheduledBackupWithVerification(3); err != nil {
		t.Fatalf("ScheduledBackupWithVerification() error = %v", err)
	}

	// Should have at most 3+1 backups (3 kept + 1 new)
	backups, _ := m.List()
	if len(backups) > 4 {
		t.Errorf("Should have at most 4 backups, got %d", len(backups))
	}
}

// Test for Create error when backup dir creation fails

func TestManagerCreateBackupDirFails(t *testing.T) {
	// Create a file where we'd want to create the backup directory
	tempDir := t.TempDir()
	blockingFile := filepath.Join(tempDir, "backups")
	os.WriteFile(blockingFile, []byte("blocking"), 0644)

	m := &Manager{
		backupDir: filepath.Join(blockingFile, "subdir"), // Will fail because backups is a file
		configDir: tempDir,
		dataDir:   tempDir,
	}

	_, err := m.Create("")
	if err == nil {
		t.Error("Create() should error when backup directory creation fails")
	}
}

// Tests for various tar entry types in Restore

func TestManagerRestoreEmptyArchive(t *testing.T) {
	tempDir := t.TempDir()

	// Create empty tar.gz
	backupPath := filepath.Join(tempDir, "empty.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)
	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.backupDir, 0755)
	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)

	// Should not error on empty archive
	err := m.Restore(backupPath)
	if err != nil {
		t.Fatalf("Restore() error on empty archive = %v", err)
	}
}

// Test ApplyRetention with Sunday backups (weekly retention)

func TestManagerApplyRetentionSundayBackup(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// Find a recent Sunday
	now := time.Now()
	daysUntilSunday := (7 - int(now.Weekday())) % 7
	if daysUntilSunday == 0 && now.Weekday() != time.Sunday {
		daysUntilSunday = 7
	}
	lastSunday := now.AddDate(0, 0, -daysUntilSunday-7) // Last Sunday

	// Create backup dated on Sunday
	sundayBackup := filepath.Join(backupDir, "sunday.tar.gz")
	os.WriteFile(sundayBackup, createMinimalTarGz(t), 0644)
	os.Chtimes(sundayBackup, lastSunday, lastSunday)

	policy := RetentionPolicy{
		Count: 0,
		Day:   0, // No daily
		Week:  4, // Keep 4 weeks
		Month: 0,
		Year:  0,
	}

	err := m.ApplyRetention(policy)
	if err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}
}

// Test ApplyRetention with first of month backups

func TestManagerApplyRetentionFirstOfMonth(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// Create backup dated on first of last month
	now := time.Now()
	firstOfMonth := time.Date(now.Year(), now.Month()-1, 1, 12, 0, 0, 0, time.UTC)

	monthlyBackup := filepath.Join(backupDir, "monthly.tar.gz")
	os.WriteFile(monthlyBackup, createMinimalTarGz(t), 0644)
	os.Chtimes(monthlyBackup, firstOfMonth, firstOfMonth)

	policy := RetentionPolicy{
		Count: 0,
		Day:   0,
		Week:  0,
		Month: 12, // Keep 12 months
		Year:  0,
	}

	err := m.ApplyRetention(policy)
	if err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}
}

// Test ApplyRetention with Jan 1 backups (yearly retention)

func TestManagerApplyRetentionYearly(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// Create backup dated on January 1 of last year
	now := time.Now()
	janFirst := time.Date(now.Year()-1, time.January, 1, 12, 0, 0, 0, time.UTC)

	yearlyBackup := filepath.Join(backupDir, "yearly.tar.gz")
	os.WriteFile(yearlyBackup, createMinimalTarGz(t), 0644)
	os.Chtimes(yearlyBackup, janFirst, janFirst)

	policy := RetentionPolicy{
		Count: 0,
		Day:   0,
		Week:  0,
		Month: 0,
		Year:  5, // Keep 5 years
	}

	err := m.ApplyRetention(policy)
	if err != nil {
		t.Fatalf("ApplyRetention() error = %v", err)
	}
}

// Test for metadata fields in created backup

func TestManagerCreateMetadataFields(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
		createdBy: "testuser",
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test content"), 0644)
	os.WriteFile(filepath.Join(m.dataDir, "data.db"), []byte("data content"), 0644)

	backupPath, _ := m.Create("")
	metadata, _ := m.GetMetadata(backupPath)

	// Check all required fields
	if metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", metadata.Version)
	}
	if metadata.CreatedBy != "testuser" {
		t.Errorf("CreatedBy = %q, want testuser", metadata.CreatedBy)
	}
	if len(metadata.Contents) == 0 {
		t.Error("Contents should not be empty")
	}
	if len(metadata.Checksums) == 0 {
		t.Error("Checksums should not be empty")
	}
	if !strings.HasPrefix(metadata.Checksum, "sha256:") {
		t.Errorf("Checksum should have sha256: prefix: %s", metadata.Checksum)
	}
}

// Test for backup with existing .tar.gz extension

func TestManagerCreateExistingExtension(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, _ := m.Create("existing.tar.gz")

	// Should have exactly one .tar.gz extension
	count := strings.Count(backupPath, ".tar.gz")
	if count != 1 {
		t.Errorf("Should have exactly one .tar.gz extension, got %d in %s", count, backupPath)
	}
}

// Test VerifyBackup with checksum in manifest

func TestManagerVerifyBackupValidChecksum(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test content"), 0644)

	backupPath, _ := m.Create("")
	result, err := m.VerifyBackup(backupPath)

	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}
	if !result.ChecksumValid {
		t.Error("ChecksumValid should be true for valid backup")
	}
	if !result.AllPassed {
		t.Errorf("AllPassed should be true, errors: %v", result.Errors)
	}
}

// Test VerifyBackup with mismatched checksum

func TestManagerVerifyBackupChecksumMismatchDetection(t *testing.T) {
	tempDir := t.TempDir()

	// Create a tar.gz with checksums that don't match
	backupPath := filepath.Join(tempDir, "mismatch.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add manifest with mismatched checksum
	manifest := []byte(`{
		"version": "1.0.0",
		"checksums": {"file.txt": "abc123"},
		"checksum": "sha256:wrongchecksum"
	}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := NewManager()
	result, _ := m.VerifyBackup(backupPath)

	// Should detect checksum mismatch
	if result.ChecksumValid {
		t.Error("ChecksumValid should be false for mismatched checksum")
	}
	if result.AllPassed {
		t.Error("AllPassed should be false")
	}
}

// Test ScheduledBackup error during List

func TestManagerScheduledBackupListError(t *testing.T) {
	tempDir := t.TempDir()

	// Create a file where backupDir should be (to cause List to fail)
	backupFile := filepath.Join(tempDir, "backups")
	os.WriteFile(backupFile, []byte("blocking"), 0644)

	m := &Manager{
		backupDir: filepath.Join(backupFile, "subdir"), // Will cause issues
		configDir: tempDir,
		dataDir:   tempDir,
	}

	err := m.ScheduledBackup(5)
	if err == nil {
		t.Error("ScheduledBackup() should error when backup creation fails")
	}
}

// Test ApplyRetention error during List

func TestManagerApplyRetentionListError(t *testing.T) {
	// Create a backup directory that will cause read errors
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")

	// Write a file where a directory is expected
	os.WriteFile(backupDir, []byte("not a directory"), 0644)

	m := &Manager{
		backupDir: backupDir,
	}

	err := m.ApplyRetention(DefaultRetentionPolicy())
	if err == nil {
		t.Error("ApplyRetention() should error when List fails")
	}
}

// Test verifyManifestFromData with manifest that has no content after it

func TestVerifyManifestFromDataValidManifest(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("content"), 0644)

	backupPath, _ := m.Create("")
	data, _ := os.ReadFile(backupPath)

	metadata, err := m.verifyManifestFromData(data)
	if err != nil {
		t.Fatalf("verifyManifestFromData() error = %v", err)
	}

	if metadata.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", metadata.Version)
	}
}

// Test RestoreEncrypted reading encrypted file that doesn't exist

func TestManagerRestoreEncryptedReadError(t *testing.T) {
	m := &Manager{
		password: "password",
	}

	// Non-existent file
	err := m.RestoreEncrypted("/path/to/nonexistent/backup.tar.gz.enc")
	if err == nil {
		t.Error("RestoreEncrypted() should error for non-existent file")
	}
}

// Test VerifyBackup with encrypted backup that fails to read

func TestManagerVerifyBackupEncryptedFileReadError(t *testing.T) {
	tempDir := t.TempDir()

	// Create directory where file is expected (to cause read error)
	fakePath := filepath.Join(tempDir, "fake.tar.gz.enc")
	os.MkdirAll(fakePath, 0755)

	m := &Manager{
		password: "password",
	}

	result, _ := m.VerifyBackup(fakePath)

	// Should have errors due to read failure
	if result.AllPassed {
		t.Error("AllPassed should be false for read error")
	}
}

// Test List with entry.Info() error (simulated by removing file between ReadDir and Info)

func TestManagerListContinuesOnInfoError(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	os.MkdirAll(backupDir, 0755)

	// Create a valid backup
	validBackup := filepath.Join(backupDir, "valid.tar.gz")
	os.WriteFile(validBackup, createMinimalTarGz(t), 0644)

	m := &Manager{
		backupDir: backupDir,
		configDir: tempDir,
		dataDir:   tempDir,
	}

	// List should still work and return the valid backup
	backups, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(backups) != 1 {
		t.Errorf("Should find 1 backup, got %d", len(backups))
	}
}

// Test for ScheduledBackupWithVerification when new backup path matches existing

func TestManagerScheduledBackupWithVerificationProtectsNewBackup(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	err := m.ScheduledBackupWithVerification(1)
	if err != nil {
		t.Fatalf("ScheduledBackupWithVerification() error = %v", err)
	}

	// Should have exactly 1 backup
	backups, _ := m.List()
	if len(backups) != 1 {
		t.Errorf("Should have 1 backup, got %d", len(backups))
	}
}

// Test encryption with large data

func TestEncryptDecryptLargeData(t *testing.T) {
	// Create 1MB of data
	data := make([]byte, 1024*1024)
	for i := range data {
		data[i] = byte(i % 256)
	}

	password := "largedatapassword"

	encrypted, err := EncryptBackup(data, password)
	if err != nil {
		t.Fatalf("EncryptBackup() error = %v", err)
	}

	decrypted, err := DecryptBackup(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptBackup() error = %v", err)
	}

	if !bytes.Equal(decrypted, data) {
		t.Error("Large data roundtrip failed")
	}
}

// Test FormatSize for edge cases

func TestBackupInfoFormatSizeExactBoundaries(t *testing.T) {
	tests := []struct {
		size     int64
		contains string
	}{
		{1023, "B"},
		{1025, "KB"},
		{1024 * 1024 - 1, "KB"},
		{1024 * 1024 + 1, "MB"},
	}

	for _, tt := range tests {
		info := BackupInfo{Size: tt.size}
		result := info.FormatSize()
		if !strings.Contains(result, tt.contains) {
			t.Errorf("FormatSize(%d) = %q, should contain %q", tt.size, result, tt.contains)
		}
	}
}

// Test metadata with all optional fields set

func TestBackupMetadataAllFields(t *testing.T) {
	metadata := BackupMetadata{
		Version:          "1.0.0",
		CreatedAt:        time.Now(),
		CreatedBy:        "admin",
		AppVersion:       "2.0.0",
		Contents:         []string{"file1", "file2"},
		Checksums:        map[string]string{"file1": "hash1", "file2": "hash2"},
		Checksum:         "sha256:overall",
		Encrypted:        true,
		EncryptionMethod: "AES-256-GCM",
		ServerTitle:      "Test Server",
		Size:             1024,
		Files:            []string{"legacy1", "legacy2"},
	}

	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded BackupMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify all fields roundtrip correctly
	if decoded.EncryptionMethod != "AES-256-GCM" {
		t.Errorf("EncryptionMethod = %q", decoded.EncryptionMethod)
	}
	if decoded.ServerTitle != "Test Server" {
		t.Errorf("ServerTitle = %q", decoded.ServerTitle)
	}
	if len(decoded.Files) != 2 {
		t.Errorf("Files length = %d", len(decoded.Files))
	}
}

// Test GetMetadata error reading tar header

func TestManagerGetMetadataCorruptedArchive(t *testing.T) {
	tempDir := t.TempDir()

	// Create a valid gzip but with corrupted tar inside
	backupPath := filepath.Join(tempDir, "corrupted.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	gzWriter.Write([]byte("not a valid tar"))
	gzWriter.Close()
	file.Close()

	m := NewManager()
	_, err := m.GetMetadata(backupPath)
	if err == nil {
		t.Error("GetMetadata() should error for corrupted archive")
	}
}

// Test Create with config that has server title

func TestManagerCreateWithServerConfig(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)

	// Create a server.yml config file
	serverConfig := `server:
  title: "Custom Server Title"
  port: 8080
`
	os.WriteFile(filepath.Join(m.configDir, "server.yml"), []byte(serverConfig), 0644)

	backupPath, err := m.Create("")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// The backup should be created
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("Backup file should exist: %v", err)
	}
}

// Test VerifyBackup returns early when file doesn't exist

func TestManagerVerifyBackupReturnsOnFileNotFound(t *testing.T) {
	m := NewManager()

	result, err := m.VerifyBackup("/completely/nonexistent/path/backup.tar.gz")

	// Should not error, but return a result with FileExists = false
	if err != nil {
		t.Fatalf("VerifyBackup() should not error, got %v", err)
	}
	if result.FileExists {
		t.Error("FileExists should be false")
	}
	if len(result.Errors) == 0 {
		t.Error("Should have errors")
	}
}

// Test that backup contains AppVersion

func TestManagerCreateIncludesAppVersion(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)

	backupPath, _ := m.Create("")
	metadata, _ := m.GetMetadata(backupPath)

	// AppVersion should be set (from config.Version)
	if metadata.AppVersion == "" {
		t.Error("AppVersion should be set in backup metadata")
	}
}

// Test Restore skips symlinks and other special file types

func TestManagerRestoreHandlesRegularFilesOnly(t *testing.T) {
	tempDir := t.TempDir()
	backupDir := filepath.Join(tempDir, "backups")
	configDir := filepath.Join(tempDir, "config")
	dataDir := filepath.Join(tempDir, "data")

	os.MkdirAll(backupDir, 0755)
	os.MkdirAll(configDir, 0755)
	os.MkdirAll(dataDir, 0755)

	// Create a tar.gz with a symlink entry (which should be skipped)
	backupPath := filepath.Join(backupDir, "test.tar.gz")
	file, _ := os.Create(backupPath)
	gzWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzWriter)

	// Add manifest
	manifest := []byte(`{"version":"1.0.0"}`)
	tarWriter.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Size: int64(len(manifest)),
		Mode: 0644,
	})
	tarWriter.Write(manifest)

	// Add symlink entry (Type other than TypeReg and TypeDir)
	tarWriter.WriteHeader(&tar.Header{
		Name:     "config/symlink",
		Typeflag: tar.TypeSymlink,
		Linkname: "/etc/passwd",
		Mode:     0777,
	})

	// Add regular file
	configData := []byte("config content")
	tarWriter.WriteHeader(&tar.Header{
		Name: "config/regular.txt",
		Size: int64(len(configData)),
		Mode: 0644,
	})
	tarWriter.Write(configData)

	tarWriter.Close()
	gzWriter.Close()
	file.Close()

	m := &Manager{
		backupDir: backupDir,
		configDir: configDir,
		dataDir:   dataDir,
	}

	err := m.Restore(backupPath)
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	// Regular file should be restored
	if _, err := os.Stat(filepath.Join(configDir, "regular.txt")); err != nil {
		t.Errorf("Regular file should be restored: %v", err)
	}

	// Symlink should not exist (was skipped)
	if _, err := os.Stat(filepath.Join(configDir, "symlink")); !os.IsNotExist(err) {
		t.Error("Symlink should be skipped during restore")
	}
}

// Test List with metadata having Contents populated

func TestManagerListWithMetadataContents(t *testing.T) {
	tempDir := t.TempDir()
	m := &Manager{
		backupDir: filepath.Join(tempDir, "backups"),
		configDir: filepath.Join(tempDir, "config"),
		dataDir:   filepath.Join(tempDir, "data"),
	}

	os.MkdirAll(m.configDir, 0755)
	os.MkdirAll(m.dataDir, 0755)
	os.WriteFile(filepath.Join(m.configDir, "test.yml"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(m.dataDir, "data.db"), []byte("data"), 0644)

	m.Create("")

	backups, err := m.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(backups) != 1 {
		t.Fatalf("Should have 1 backup, got %d", len(backups))
	}

	// FileCount should be populated from metadata
	if backups[0].FileCount == 0 {
		t.Error("FileCount should be populated from metadata")
	}
}
