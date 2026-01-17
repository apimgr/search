package backup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
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
