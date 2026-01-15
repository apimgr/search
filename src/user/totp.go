package user

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

// TOTPManager handles two-factor authentication
type TOTPManager struct {
	db            *sql.DB
	issuer        string
	encryptionKey []byte
}

// User2FA represents a user's 2FA configuration
type User2FA struct {
	ID              int64      `json:"id" db:"id"`
	UserID          int64      `json:"user_id" db:"user_id"`
	SecretEncrypted string     `json:"-" db:"secret_encrypted"`
	Enabled         bool       `json:"enabled" db:"enabled"`
	Verified        bool       `json:"verified" db:"verified"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	EnabledAt       *time.Time `json:"enabled_at,omitempty" db:"enabled_at"`
}

// TOTPSetupResponse contains data needed for 2FA setup
type TOTPSetupResponse struct {
	Secret    string `json:"secret"`
	QRCodeURL string `json:"qr_code_url"`
	Issuer    string `json:"issuer"`
	Account   string `json:"account"`
}

// 2FA errors
var (
	ErrTOTPNotEnabled    = errors.New("2FA is not enabled for this user")
	ErrTOTPAlreadySetup  = errors.New("2FA is already set up")
	ErrTOTPInvalidCode   = errors.New("invalid 2FA code")
	ErrTOTPNotVerified   = errors.New("2FA setup not verified")
	ErrEncryptionFailed  = errors.New("encryption failed")
	ErrDecryptionFailed  = errors.New("decryption failed")
)

// NewTOTPManager creates a new TOTP manager
func NewTOTPManager(db *sql.DB, issuer string, encryptionKey []byte) (*TOTPManager, error) {
	if len(encryptionKey) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}

	return &TOTPManager{
		db:            db,
		issuer:        issuer,
		encryptionKey: encryptionKey,
	}, nil
}

// Setup initiates 2FA setup for a user
func (tm *TOTPManager) Setup(ctx context.Context, user *User) (*TOTPSetupResponse, error) {
	// Check if already set up and enabled
	existing, err := tm.Get2FA(ctx, user.ID)
	if err == nil && existing.Enabled {
		return nil, ErrTOTPAlreadySetup
	}

	// Generate new TOTP key
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      tm.issuer,
		AccountName: user.Email,
		Period:      30,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	// Encrypt the secret
	encryptedSecret, err := tm.encrypt(key.Secret())
	if err != nil {
		return nil, err
	}

	// Store or update 2FA record
	now := time.Now()
	if existing != nil {
		// Update existing record
		_, err = tm.db.ExecContext(ctx, `
			UPDATE user_2fa SET secret_encrypted = ?, verified = 0, enabled = 0, created_at = ? WHERE user_id = ?
		`, encryptedSecret, now, user.ID)
	} else {
		// Insert new record
		_, err = tm.db.ExecContext(ctx, `
			INSERT INTO user_2fa (user_id, secret_encrypted, enabled, verified, created_at) VALUES (?, ?, 0, 0, ?)
		`, user.ID, encryptedSecret, now)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to store 2FA secret: %w", err)
	}

	return &TOTPSetupResponse{
		Secret:    key.Secret(),
		QRCodeURL: key.URL(),
		Issuer:    tm.issuer,
		Account:   user.Email,
	}, nil
}

// VerifySetup verifies the initial 2FA setup with a code
func (tm *TOTPManager) VerifySetup(ctx context.Context, userID int64, code string) error {
	// Get 2FA record
	twofa, err := tm.Get2FA(ctx, userID)
	if err != nil {
		return err
	}

	if twofa.Enabled {
		return ErrTOTPAlreadySetup
	}

	// Decrypt secret
	secret, err := tm.decrypt(twofa.SecretEncrypted)
	if err != nil {
		return err
	}

	// Validate code
	if !totp.Validate(code, secret) {
		return ErrTOTPInvalidCode
	}

	// Enable 2FA
	now := time.Now()
	_, err = tm.db.ExecContext(ctx, `
		UPDATE user_2fa SET enabled = 1, verified = 1, enabled_at = ? WHERE user_id = ?
	`, now, userID)
	if err != nil {
		return fmt.Errorf("failed to enable 2FA: %w", err)
	}

	return nil
}

// Verify verifies a TOTP code for an enabled 2FA
func (tm *TOTPManager) Verify(ctx context.Context, userID int64, code string) error {
	// Get 2FA record
	twofa, err := tm.Get2FA(ctx, userID)
	if err != nil {
		return err
	}

	if !twofa.Enabled {
		return ErrTOTPNotEnabled
	}

	// Decrypt secret
	secret, err := tm.decrypt(twofa.SecretEncrypted)
	if err != nil {
		return err
	}

	// Validate code
	if !totp.Validate(code, secret) {
		return ErrTOTPInvalidCode
	}

	return nil
}

// Disable disables 2FA for a user (requires password verification first)
func (tm *TOTPManager) Disable(ctx context.Context, userID int64) error {
	result, err := tm.db.ExecContext(ctx, "DELETE FROM user_2fa WHERE user_id = ?", userID)
	if err != nil {
		return fmt.Errorf("failed to disable 2FA: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrTOTPNotEnabled
	}

	return nil
}

// Get2FA retrieves the 2FA configuration for a user
func (tm *TOTPManager) Get2FA(ctx context.Context, userID int64) (*User2FA, error) {
	var twofa User2FA
	var enabledAt sql.NullTime

	err := tm.db.QueryRowContext(ctx, `
		SELECT id, user_id, secret_encrypted, enabled, verified, created_at, enabled_at
		FROM user_2fa WHERE user_id = ?
	`, userID).Scan(
		&twofa.ID, &twofa.UserID, &twofa.SecretEncrypted, &twofa.Enabled, &twofa.Verified, &twofa.CreatedAt, &enabledAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrTOTPNotEnabled
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get 2FA: %w", err)
	}

	if enabledAt.Valid {
		twofa.EnabledAt = &enabledAt.Time
	}

	return &twofa, nil
}

// Is2FAEnabled checks if 2FA is enabled for a user
func (tm *TOTPManager) Is2FAEnabled(ctx context.Context, userID int64) bool {
	var enabled bool
	err := tm.db.QueryRowContext(ctx, "SELECT enabled FROM user_2fa WHERE user_id = ?", userID).Scan(&enabled)
	if err != nil {
		return false
	}
	return enabled
}

// encrypt encrypts a string using AES-GCM
func (tm *TOTPManager) encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(tm.encryptionKey)
	if err != nil {
		return "", ErrEncryptionFailed
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrEncryptionFailed
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", ErrEncryptionFailed
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decrypt decrypts a string using AES-GCM
func (tm *TOTPManager) decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	block, err := aes.NewCipher(tm.encryptionKey)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	if len(data) < gcm.NonceSize() {
		return "", ErrDecryptionFailed
	}

	nonce := data[:gcm.NonceSize()]
	ciphertextBytes := data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}

// GenerateBackupCodes generates backup codes for 2FA recovery
// This is a simpler alternative to full recovery keys
func (tm *TOTPManager) GenerateBackupCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		bytes := make([]byte, 4)
		if _, err := rand.Read(bytes); err != nil {
			return nil, err
		}
		// Format as XXXX-XXXX
		code := fmt.Sprintf("%04x-%04x", bytes[:2], bytes[2:])
		codes[i] = code
	}
	return codes, nil
}

// TOTPStatus represents the 2FA status for display
type TOTPStatus struct {
	Enabled   bool       `json:"enabled"`
	Verified  bool       `json:"verified"`
	EnabledAt *time.Time `json:"enabled_at,omitempty"`
}

// GetStatus returns the 2FA status for a user
func (tm *TOTPManager) GetStatus(ctx context.Context, userID int64) TOTPStatus {
	twofa, err := tm.Get2FA(ctx, userID)
	if err != nil {
		return TOTPStatus{Enabled: false, Verified: false}
	}
	return TOTPStatus{
		Enabled:   twofa.Enabled,
		Verified:  twofa.Verified,
		EnabledAt: twofa.EnabledAt,
	}
}
