package users

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// RecoveryManager handles recovery key generation and validation
type RecoveryManager struct {
	db       *sql.DB
	keyCount int
}

// RecoveryKey represents a recovery key record
type RecoveryKey struct {
	ID        int64      `json:"id" db:"id"`
	UserID    int64      `json:"user_id" db:"user_id"`
	KeyHash   string     `json:"-" db:"key_hash"`
	Used      bool       `json:"used" db:"used"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UsedAt    *time.Time `json:"used_at,omitempty" db:"used_at"`
}

// Recovery errors
var (
	ErrNoRecoveryKeys      = errors.New("no recovery keys available")
	ErrInvalidRecoveryKey  = errors.New("invalid recovery key")
	ErrRecoveryKeyUsed     = errors.New("recovery key already used")
	ErrRecoveryKeyNotFound = errors.New("recovery key not found")
)

// NewRecoveryManager creates a new recovery key manager
func NewRecoveryManager(db *sql.DB, keyCount int) *RecoveryManager {
	if keyCount <= 0 {
		keyCount = 10 // Default to 10 recovery keys
	}
	return &RecoveryManager{
		db:       db,
		keyCount: keyCount,
	}
}

// Generate generates new recovery keys for a user
// This invalidates any existing recovery keys
func (rm *RecoveryManager) Generate(ctx context.Context, userID int64) ([]string, error) {
	// Delete existing recovery keys
	_, err := rm.db.ExecContext(ctx, "DELETE FROM recovery_keys WHERE user_id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete existing recovery keys: %w", err)
	}

	// Generate new keys
	keys := make([]string, rm.keyCount)
	now := time.Now()

	for i := 0; i < rm.keyCount; i++ {
		// Generate a random key (format: XXXX-XXXX-XXXX-XXXX)
		key, err := generateRecoveryKey()
		if err != nil {
			return nil, fmt.Errorf("failed to generate recovery key: %w", err)
		}
		keys[i] = key

		// Hash the key for storage using Argon2id
		hash, err := hashRecoveryKey(normalizeRecoveryKey(key))
		if err != nil {
			return nil, fmt.Errorf("failed to hash recovery key: %w", err)
		}

		// Store the hashed key
		_, err = rm.db.ExecContext(ctx, `
			INSERT INTO recovery_keys (user_id, key_hash, used, created_at) VALUES (?, ?, 0, ?)
		`, userID, hash, now)
		if err != nil {
			return nil, fmt.Errorf("failed to store recovery key: %w", err)
		}
	}

	return keys, nil
}

// Validate validates and consumes a recovery key
func (rm *RecoveryManager) Validate(ctx context.Context, userID int64, key string) error {
	// Normalize the key
	key = normalizeRecoveryKey(key)

	// Get all unused recovery keys for the user
	rows, err := rm.db.QueryContext(ctx, `
		SELECT id, key_hash FROM recovery_keys WHERE user_id = ? AND used = 0
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to get recovery keys: %w", err)
	}
	defer rows.Close()

	var found bool
	var keyID int64

	for rows.Next() {
		var id int64
		var hash string
		if err := rows.Scan(&id, &hash); err != nil {
			continue
		}

		// Check if this key matches using Argon2id
		if verifyRecoveryKey(key, hash) {
			found = true
			keyID = id
			break
		}
	}

	if !found {
		return ErrInvalidRecoveryKey
	}

	// Mark the key as used
	now := time.Now()
	_, err = rm.db.ExecContext(ctx, `
		UPDATE recovery_keys SET used = 1, used_at = ? WHERE id = ?
	`, now, keyID)
	if err != nil {
		return fmt.Errorf("failed to mark recovery key as used: %w", err)
	}

	return nil
}

// GetRemainingCount returns the number of unused recovery keys
func (rm *RecoveryManager) GetRemainingCount(ctx context.Context, userID int64) (int, error) {
	var count int
	err := rm.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM recovery_keys WHERE user_id = ? AND used = 0
	`, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count recovery keys: %w", err)
	}
	return count, nil
}

// HasRecoveryKeys checks if a user has any unused recovery keys
func (rm *RecoveryManager) HasRecoveryKeys(ctx context.Context, userID int64) bool {
	count, err := rm.GetRemainingCount(ctx, userID)
	return err == nil && count > 0
}

// RevokeAll revokes all recovery keys for a user
func (rm *RecoveryManager) RevokeAll(ctx context.Context, userID int64) error {
	_, err := rm.db.ExecContext(ctx, "DELETE FROM recovery_keys WHERE user_id = ?", userID)
	return err
}

// GetUsageStats returns recovery key usage statistics
type RecoveryKeyStats struct {
	Total     int `json:"total"`
	Used      int `json:"used"`
	Remaining int `json:"remaining"`
}

func (rm *RecoveryManager) GetUsageStats(ctx context.Context, userID int64) (*RecoveryKeyStats, error) {
	var total, used int

	err := rm.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN used = 1 THEN 1 ELSE 0 END), 0)
		FROM recovery_keys WHERE user_id = ?
	`, userID).Scan(&total, &used)
	if err != nil {
		return nil, fmt.Errorf("failed to get recovery key stats: %w", err)
	}

	return &RecoveryKeyStats{
		Total:     total,
		Used:      used,
		Remaining: total - used,
	}, nil
}

// generateRecoveryKey generates a random recovery key in format XXXX-XXXX-XXXX-XXXX
func generateRecoveryKey() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Format as XXXX-XXXX-XXXX-XXXX using hex
	hex := hex.EncodeToString(bytes)
	return fmt.Sprintf("%s-%s-%s-%s", hex[0:4], hex[4:8], hex[8:12], hex[12:16]), nil
}

// normalizeRecoveryKey normalizes a recovery key for comparison
func normalizeRecoveryKey(key string) string {
	// Remove dashes and convert to uppercase
	key = strings.ReplaceAll(key, "-", "")
	key = strings.ReplaceAll(key, " ", "")
	return strings.ToUpper(key)
}

// FormatRecoveryKey formats a recovery key for display
func FormatRecoveryKey(key string) string {
	key = normalizeRecoveryKey(key)
	if len(key) != 16 {
		return key
	}
	return fmt.Sprintf("%s-%s-%s-%s", key[0:4], key[4:8], key[8:12], key[12:16])
}

// Argon2id parameters for recovery keys (per AI.md line 932)
const (
	recoveryArgon2Time    = 3         // iterations
	recoveryArgon2Memory  = 64 * 1024 // 64 MB
	recoveryArgon2Threads = 4
	recoveryArgon2KeyLen  = 32
	recoveryArgon2SaltLen = 16
)

// hashRecoveryKey hashes a recovery key using Argon2id
func hashRecoveryKey(key string) (string, error) {
	salt := make([]byte, recoveryArgon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(key), salt, recoveryArgon2Time, recoveryArgon2Memory, recoveryArgon2Threads, recoveryArgon2KeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, recoveryArgon2Memory, recoveryArgon2Time, recoveryArgon2Threads, b64Salt, b64Hash), nil
}

// verifyRecoveryKey verifies a recovery key against an Argon2id hash
func verifyRecoveryKey(key, encodedHash string) bool {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return false
	}

	if parts[1] != "argon2id" {
		return false
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false
	}

	var memory, time uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false
	}

	computedHash := argon2.IDKey([]byte(key), salt, time, memory, threads, uint32(len(expectedHash)))

	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}
