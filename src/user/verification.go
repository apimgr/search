package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// VerificationManager handles email verification and password reset tokens
type VerificationManager struct {
	db *sql.DB
}

// VerificationToken represents a verification token
type VerificationToken struct {
	ID        int64     `json:"id" db:"id"`
	UserID    int64     `json:"user_id" db:"user_id"`
	Token     string    `json:"token" db:"token"`
	Type      string    `json:"type" db:"type"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Verification token types
const (
	TokenTypeEmailVerify   = "email_verify"
	TokenTypePasswordReset = "password_reset"
)

// Token durations
const (
	EmailVerifyDuration   = 24 * time.Hour    // 24 hours
	PasswordResetDuration = 1 * time.Hour     // 1 hour
)

// Verification errors
var (
	ErrVerificationTokenNotFound = errors.New("verification token not found")
	ErrVerificationTokenExpired  = errors.New("verification token expired")
	ErrVerificationTokenInvalid  = errors.New("invalid verification token")
)

// NewVerificationManager creates a new verification manager
func NewVerificationManager(db *sql.DB) *VerificationManager {
	return &VerificationManager{db: db}
}

// CreateEmailVerification creates an email verification token
func (vm *VerificationManager) CreateEmailVerification(ctx context.Context, userID int64) (string, error) {
	return vm.createToken(ctx, userID, TokenTypeEmailVerify, EmailVerifyDuration)
}

// CreatePasswordReset creates a password reset token
func (vm *VerificationManager) CreatePasswordReset(ctx context.Context, userID int64) (string, error) {
	return vm.createToken(ctx, userID, TokenTypePasswordReset, PasswordResetDuration)
}

// createToken creates a verification token
func (vm *VerificationManager) createToken(ctx context.Context, userID int64, tokenType string, duration time.Duration) (string, error) {
	// Invalidate any existing tokens of the same type
	_, err := vm.db.ExecContext(ctx, "DELETE FROM verification_tokens WHERE user_id = ? AND type = ?", userID, tokenType)
	if err != nil {
		return "", fmt.Errorf("failed to invalidate existing tokens: %w", err)
	}

	// Generate new token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes)

	// Store token
	now := time.Now()
	expiresAt := now.Add(duration)

	_, err = vm.db.ExecContext(ctx, `
		INSERT INTO verification_tokens (user_id, token, type, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, userID, token, tokenType, expiresAt, now)
	if err != nil {
		return "", fmt.Errorf("failed to create verification token: %w", err)
	}

	return token, nil
}

// VerifyEmail verifies an email verification token and marks the user's email as verified
func (vm *VerificationManager) VerifyEmail(ctx context.Context, token string) (*User, error) {
	vt, err := vm.validateToken(ctx, token, TokenTypeEmailVerify)
	if err != nil {
		return nil, err
	}

	// Mark email as verified
	_, err = vm.db.ExecContext(ctx, "UPDATE users SET email_verified = 1, updated_at = ? WHERE id = ?", time.Now(), vt.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify email: %w", err)
	}

	// Delete the token
	_, _ = vm.db.ExecContext(ctx, "DELETE FROM verification_tokens WHERE id = ?", vt.ID)

	// Get the user
	var user User
	var lastLogin sql.NullTime
	err = vm.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, display_name, avatar_url, bio, role, email_verified, active, created_at, updated_at, last_login
		FROM users WHERE id = ?
	`, vt.UserID).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Role, &user.EmailVerified, &user.Active, &user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// ValidatePasswordReset validates a password reset token and returns the user
func (vm *VerificationManager) ValidatePasswordReset(ctx context.Context, token string) (*User, error) {
	vt, err := vm.validateToken(ctx, token, TokenTypePasswordReset)
	if err != nil {
		return nil, err
	}

	// Get the user
	var user User
	var lastLogin sql.NullTime
	err = vm.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, display_name, avatar_url, bio, role, email_verified, active, created_at, updated_at, last_login
		FROM users WHERE id = ?
	`, vt.UserID).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Role, &user.EmailVerified, &user.Active, &user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// ConsumePasswordReset validates and consumes a password reset token
func (vm *VerificationManager) ConsumePasswordReset(ctx context.Context, token string) (*User, error) {
	user, err := vm.ValidatePasswordReset(ctx, token)
	if err != nil {
		return nil, err
	}

	// Delete the token
	_, _ = vm.db.ExecContext(ctx, "DELETE FROM verification_tokens WHERE token = ? AND type = ?", token, TokenTypePasswordReset)

	return user, nil
}

// validateToken validates a verification token
func (vm *VerificationManager) validateToken(ctx context.Context, token, tokenType string) (*VerificationToken, error) {
	if token == "" {
		return nil, ErrVerificationTokenInvalid
	}

	var vt VerificationToken
	err := vm.db.QueryRowContext(ctx, `
		SELECT id, user_id, token, type, expires_at, created_at
		FROM verification_tokens WHERE token = ? AND type = ?
	`, token, tokenType).Scan(
		&vt.ID, &vt.UserID, &vt.Token, &vt.Type, &vt.ExpiresAt, &vt.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrVerificationTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if vt.ExpiresAt.Before(time.Now()) {
		// Delete expired token
		_, _ = vm.db.ExecContext(ctx, "DELETE FROM verification_tokens WHERE id = ?", vt.ID)
		return nil, ErrVerificationTokenExpired
	}

	return &vt, nil
}

// CleanupExpired removes expired verification tokens
func (vm *VerificationManager) CleanupExpired(ctx context.Context) (int64, error) {
	result, err := vm.db.ExecContext(ctx, "DELETE FROM verification_tokens WHERE expires_at < ?", time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup tokens: %w", err)
	}

	return result.RowsAffected()
}

// GetPendingVerification checks if a user has a pending email verification
func (vm *VerificationManager) GetPendingVerification(ctx context.Context, userID int64) (*VerificationToken, error) {
	var vt VerificationToken
	err := vm.db.QueryRowContext(ctx, `
		SELECT id, user_id, token, type, expires_at, created_at
		FROM verification_tokens WHERE user_id = ? AND type = ? AND expires_at > ?
	`, userID, TokenTypeEmailVerify, time.Now()).Scan(
		&vt.ID, &vt.UserID, &vt.Token, &vt.Type, &vt.ExpiresAt, &vt.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &vt, nil
}
