package backup

import (
	"encoding/binary"
	"testing"
)

func TestEncryptBackupBKUP1HeaderAndRoundTrip(t *testing.T) {
	data := []byte("full backup archive")
	encrypted, err := EncryptBackup(data, "correct horse battery staple")
	if err != nil {
		t.Fatalf("EncryptBackup() error = %v", err)
	}
	if string(encrypted[:5]) != "BKUP1" {
		t.Fatalf("archive magic = %q, want BKUP1", encrypted[:5])
	}
	header, headerLen, err := ReadBackupEncryptionHeader(encrypted)
	if err != nil {
		t.Fatalf("ReadBackupEncryptionHeader() error = %v", err)
	}
	if headerLen != backupHeaderFixedLen || header.BaseName != "" {
		t.Fatalf("header length/base name = %d/%q, want %d/empty", headerLen, header.BaseName, backupHeaderFixedLen)
	}
	keyID, err := BackupEncryptionKeyID("correct horse battery staple", header)
	if err != nil {
		t.Fatalf("BackupEncryptionKeyID() error = %v", err)
	}
	if keyID != header.KeyID {
		t.Fatalf("derived key ID = %q, header has %q", keyID, header.KeyID)
	}
	decrypted, err := DecryptBackup(encrypted, "correct horse battery staple")
	if err != nil {
		t.Fatalf("DecryptBackup() error = %v", err)
	}
	if string(decrypted) != string(data) {
		t.Fatalf("DecryptBackup() = %q, want %q", decrypted, data)
	}
}

func TestEncryptBackupWithBaseAuthenticatesHeader(t *testing.T) {
	data := []byte("incremental backup archive")
	baseName := "backup-20250115.tar.gz.enc"
	encrypted, err := EncryptBackupWithBase(data, "correct horse battery staple", baseName)
	if err != nil {
		t.Fatalf("EncryptBackupWithBase() error = %v", err)
	}
	header, headerLen, err := ReadBackupEncryptionHeader(encrypted)
	if err != nil {
		t.Fatalf("ReadBackupEncryptionHeader() error = %v", err)
	}
	if header.BaseName != baseName || headerLen != backupHeaderFixedLen+len(baseName) {
		t.Fatalf("parsed base header = %q/%d, want %q/%d", header.BaseName, headerLen, baseName, backupHeaderFixedLen+len(baseName))
	}
	decrypted, err := DecryptBackup(encrypted, "correct horse battery staple")
	if err != nil {
		t.Fatalf("DecryptBackup() error = %v", err)
	}
	if string(decrypted) != string(data) {
		t.Fatalf("DecryptBackup() = %q, want %q", decrypted, data)
	}

	tampered := append([]byte(nil), encrypted...)
	tampered[backupHeaderFixedLen] ^= 1
	if _, err := DecryptBackup(tampered, "correct horse battery staple"); err == nil {
		t.Fatal("DecryptBackup() succeeded after authenticated base name was changed")
	}
}

func TestReadBackupEncryptionHeaderRejectsMalformedHeaders(t *testing.T) {
	valid, err := EncryptBackup(nil, "password")
	if err != nil {
		t.Fatalf("EncryptBackup() error = %v", err)
	}
	tests := []struct {
		name string
		edit func([]byte) []byte
	}{
		{
			name: "bad magic",
			edit: func(data []byte) []byte {
				data[0] = 'X'
				return data
			},
		},
		{
			name: "base name length above limit",
			edit: func(data []byte) []byte {
				binary.BigEndian.PutUint16(data[53:55], backupHeaderMaxBase+1)
				return data
			},
		},
		{
			name: "truncated base name",
			edit: func(data []byte) []byte {
				binary.BigEndian.PutUint16(data[53:55], 1)
				return data[:backupHeaderFixedLen]
			},
		},
		{
			name: "zero iterations",
			edit: func(data []byte) []byte {
				binary.BigEndian.PutUint32(data[25:29], 0)
				return data
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			malformed := append([]byte(nil), valid...)
			malformed = test.edit(malformed)
			if _, _, err := ReadBackupEncryptionHeader(malformed); err == nil {
				t.Fatal("ReadBackupEncryptionHeader() succeeded for malformed header")
			}
		})
	}
}

func TestEncryptBackupWithBaseRejectsUnsafeNames(t *testing.T) {
	for _, name := range []string{"../base.tar.gz.enc", "dir\\base.tar.gz.enc", string([]byte{0xff})} {
		if _, err := EncryptBackupWithBase(nil, "password", name); err == nil {
			t.Errorf("EncryptBackupWithBase() accepted unsafe base name %q", name)
		}
	}
	longName := make([]byte, backupHeaderMaxBase+1)
	for i := range longName {
		longName[i] = 'a'
	}
	if _, err := EncryptBackupWithBase(nil, "password", string(longName)); err == nil {
		t.Error("EncryptBackupWithBase() accepted a base name longer than 255 bytes")
	}
}
