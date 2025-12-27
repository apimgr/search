package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TokenManager handles user API token management
type TokenManager struct {
	db *sql.DB
}

// UserToken represents a user's API token
type UserToken struct {
	ID          int64      `json:"id" db:"id"`
	UserID      int64      `json:"user_id" db:"user_id"`
	Name        string     `json:"name" db:"name"`
	TokenHash   string     `json:"-" db:"token_hash"`
	TokenPrefix string     `json:"token_prefix" db:"token_prefix"`
	Permissions string     `json:"permissions,omitempty" db:"permissions"`
	LastUsed    *time.Time `json:"last_used,omitempty" db:"last_used"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" db:"expires_at"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// CreateTokenRequest represents a request to create a new token
type CreateTokenRequest struct {
	Name        string
	Permissions []string
	ExpiresIn   time.Duration // 0 for no expiration
}

// CreateTokenResponse contains the newly created token (only returned once)
type CreateTokenResponse struct {
	Token  string     `json:"token"`
	ID     int64      `json:"id"`
	Name   string     `json:"name"`
	Prefix string     `json:"prefix"`
	Expiry *time.Time `json:"expiry,omitempty"`
}

// Token errors
var (
	ErrTokenNotFound  = errors.New("token not found")
	ErrTokenExpired   = errors.New("token expired")
	ErrTokenInvalid   = errors.New("invalid token")
	ErrTokenNameEmpty = errors.New("token name is required")
)

// Token prefix for user API tokens
// Per AI.md PART 23: API key prefix must be "key_"
const userTokenPrefix = "key_"

// NewTokenManager creates a new token manager
func NewTokenManager(db *sql.DB) *TokenManager {
	return &TokenManager{db: db}
}

// Create creates a new API token for a user
func (tm *TokenManager) Create(ctx context.Context, userID int64, req CreateTokenRequest) (*CreateTokenResponse, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, ErrTokenNameEmpty
	}

	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Create the full token with prefix
	rawToken := hex.EncodeToString(tokenBytes)
	fullToken := userTokenPrefix + rawToken

	// Hash the token for storage
	hash := sha256.Sum256([]byte(fullToken))
	tokenHash := hex.EncodeToString(hash[:])

	// Token prefix for display (first 8 chars after prefix)
	displayPrefix := userTokenPrefix + rawToken[:8] + "..."

	// Calculate expiry
	var expiresAt *time.Time
	if req.ExpiresIn > 0 {
		exp := time.Now().Add(req.ExpiresIn)
		expiresAt = &exp
	}

	// Permissions as comma-separated string
	permissions := strings.Join(req.Permissions, ",")

	// Insert token
	result, err := tm.db.ExecContext(ctx, `
		INSERT INTO user_tokens (user_id, name, token_hash, token_prefix, permissions, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, req.Name, tokenHash, displayPrefix, permissions, expiresAt, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	id, _ := result.LastInsertId()

	return &CreateTokenResponse{
		Token:  fullToken,
		ID:     id,
		Name:   req.Name,
		Prefix: displayPrefix,
		Expiry: expiresAt,
	}, nil
}

// Validate validates a token and returns the associated user
func (tm *TokenManager) Validate(ctx context.Context, token string) (*User, *UserToken, error) {
	// Check prefix
	if !strings.HasPrefix(token, userTokenPrefix) {
		return nil, nil, ErrTokenInvalid
	}

	// Hash the token
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Find the token and user
	var ut UserToken
	var user User
	var lastUsed, expiresAt, userLastLogin sql.NullTime

	err := tm.db.QueryRowContext(ctx, `
		SELECT t.id, t.user_id, t.name, t.token_hash, t.token_prefix, t.permissions, t.last_used, t.expires_at, t.created_at,
		       u.id, u.username, u.email, u.password_hash, u.display_name, u.avatar_url, u.bio, u.role, u.email_verified, u.active, u.created_at, u.updated_at, u.last_login
		FROM user_tokens t
		JOIN users u ON t.user_id = u.id
		WHERE t.token_hash = ?
	`, tokenHash).Scan(
		&ut.ID, &ut.UserID, &ut.Name, &ut.TokenHash, &ut.TokenPrefix, &ut.Permissions, &lastUsed, &expiresAt, &ut.CreatedAt,
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Role, &user.EmailVerified, &user.Active, &user.CreatedAt, &user.UpdatedAt, &userLastLogin,
	)
	if err == sql.ErrNoRows {
		return nil, nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate token: %w", err)
	}

	if lastUsed.Valid {
		ut.LastUsed = &lastUsed.Time
	}
	if expiresAt.Valid {
		ut.ExpiresAt = &expiresAt.Time
		if ut.ExpiresAt.Before(time.Now()) {
			return nil, nil, ErrTokenExpired
		}
	}
	if userLastLogin.Valid {
		user.LastLogin = &userLastLogin.Time
	}

	if !user.Active {
		return nil, nil, ErrUserInactive
	}

	// Update last used
	_, _ = tm.db.ExecContext(ctx, "UPDATE user_tokens SET last_used = ? WHERE id = ?", time.Now(), ut.ID)

	return &user, &ut, nil
}

// List lists all tokens for a user
func (tm *TokenManager) List(ctx context.Context, userID int64) ([]UserToken, error) {
	rows, err := tm.db.QueryContext(ctx, `
		SELECT id, user_id, name, token_hash, token_prefix, permissions, last_used, expires_at, created_at
		FROM user_tokens WHERE user_id = ? ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []UserToken
	for rows.Next() {
		var t UserToken
		var lastUsed, expiresAt sql.NullTime

		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.TokenPrefix, &t.Permissions, &lastUsed, &expiresAt, &t.CreatedAt); err != nil {
			return nil, err
		}

		if lastUsed.Valid {
			t.LastUsed = &lastUsed.Time
		}
		if expiresAt.Valid {
			t.ExpiresAt = &expiresAt.Time
		}

		tokens = append(tokens, t)
	}

	return tokens, nil
}

// Revoke revokes a specific token
func (tm *TokenManager) Revoke(ctx context.Context, userID, tokenID int64) error {
	result, err := tm.db.ExecContext(ctx, "DELETE FROM user_tokens WHERE id = ? AND user_id = ?", tokenID, userID)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrTokenNotFound
	}

	return nil
}

// RevokeAll revokes all tokens for a user
func (tm *TokenManager) RevokeAll(ctx context.Context, userID int64) (int64, error) {
	result, err := tm.db.ExecContext(ctx, "DELETE FROM user_tokens WHERE user_id = ?", userID)
	if err != nil {
		return 0, fmt.Errorf("failed to revoke tokens: %w", err)
	}

	return result.RowsAffected()
}

// CleanupExpired removes expired tokens
func (tm *TokenManager) CleanupExpired(ctx context.Context) (int64, error) {
	result, err := tm.db.ExecContext(ctx, "DELETE FROM user_tokens WHERE expires_at IS NOT NULL AND expires_at < ?", time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup tokens: %w", err)
	}

	return result.RowsAffected()
}

// GetPermissions parses the permissions string into a slice
func (ut *UserToken) GetPermissions() []string {
	if ut.Permissions == "" {
		return nil
	}
	return strings.Split(ut.Permissions, ",")
}

// HasPermission checks if the token has a specific permission
func (ut *UserToken) HasPermission(permission string) bool {
	for _, p := range ut.GetPermissions() {
		if p == permission || p == "*" {
			return true
		}
	}
	return false
}

// IsExpired checks if the token is expired
func (ut *UserToken) IsExpired() bool {
	if ut.ExpiresAt == nil {
		return false
	}
	return ut.ExpiresAt.Before(time.Now())
}

// TokenInfo returns safe token info for display
type TokenInfo struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Prefix      string     `json:"prefix"`
	Permissions []string   `json:"permissions,omitempty"`
	LastUsed    *time.Time `json:"last_used,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Expired     bool       `json:"expired"`
}

// ToInfo converts a token to safe display info
func (ut *UserToken) ToInfo() TokenInfo {
	return TokenInfo{
		ID:          ut.ID,
		Name:        ut.Name,
		Prefix:      ut.TokenPrefix,
		Permissions: ut.GetPermissions(),
		LastUsed:    ut.LastUsed,
		ExpiresAt:   ut.ExpiresAt,
		CreatedAt:   ut.CreatedAt,
		Expired:     ut.IsExpired(),
	}
}
