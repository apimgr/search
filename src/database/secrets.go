package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// SecretInstallation is the root secret every other derived key hangs off.
// AI.md PART 11 "Cryptographic Keys".
const SecretInstallation = "installation_secret"

// SecretCookieSigning signs session cookies (HMAC-SHA256).
// AI.md PART 11 "Other Project-Level Secrets": auto-rotated every 90 days,
// previous value valid for 7 days.
const SecretCookieSigning = "cookie_signing_key"

// SecretCSRFToken is the HMAC base for CSRF tokens.
// AI.md PART 11 "Other Project-Level Secrets": auto-rotated every 180 days.
const SecretCSRFToken = "csrf_token_secret"

// secretLength is the mandated size of every app_secrets value: 32 bytes
// (256 bits) of crypto/rand, stored base64-encoded.
const secretLength = 32

// secretGrace is how long a rotated-out secret stays valid so in-flight
// HMACs (security_id URLs, signed cookies) still validate: 7 days.
const secretGrace = 7 * 24 * time.Hour

// AppSecret is one row of the app_secrets table.
// Values are base64-encoded and MUST never be logged or returned by an API.
type AppSecret struct {
	Name              string
	Value             string
	PreviousValue     string
	PreviousExpiresAt time.Time
	CreatedAt         time.Time
	RotatedAt         time.Time
}

// generateSecret returns a new 32-byte crypto/rand secret, base64-encoded.
func generateSecret() (string, error) {
	buf := make([]byte, secretLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

// GetSecret loads one app_secrets row. It returns sql.ErrNoRows when the
// secret has not been generated yet.
func GetSecret(ctx context.Context, db *DB, name string) (*AppSecret, error) {
	table := ServerTableName(db, "app_secrets")
	query := fmt.Sprintf(`SELECT name, value, COALESCE(previous_value, ''),
		COALESCE(previous_expires_at, CURRENT_TIMESTAMP), created_at, rotated_at
		FROM %s WHERE name = ?`, table)

	var (
		secret    AppSecret
		prevExp   sql.NullTime
		createdAt sql.NullTime
		rotatedAt sql.NullTime
	)
	row := db.QueryRow(ctx, query, name)
	if err := row.Scan(&secret.Name, &secret.Value, &secret.PreviousValue,
		&prevExp, &createdAt, &rotatedAt); err != nil {
		return nil, err
	}
	if prevExp.Valid {
		secret.PreviousExpiresAt = prevExp.Time
	}
	if createdAt.Valid {
		secret.CreatedAt = createdAt.Time
	}
	if rotatedAt.Valid {
		secret.RotatedAt = rotatedAt.Time
	}
	// A previous value past its grace window is no longer valid material.
	if secret.PreviousValue != "" && !secret.PreviousExpiresAt.IsZero() &&
		time.Now().After(secret.PreviousExpiresAt) {
		secret.PreviousValue = ""
	}
	return &secret, nil
}

// EnsureSecret returns the named secret, generating and persisting a new
// 32-byte value on first call. AI.md PART 11: "Generated on first start,
// before any user-visible operation."
func EnsureSecret(ctx context.Context, db *DB, name string) (*AppSecret, error) {
	secret, err := GetSecret(ctx, db, name)
	if err == nil {
		return secret, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	value, err := generateSecret()
	if err != nil {
		return nil, err
	}

	table := ServerTableName(db, "app_secrets")
	insert := fmt.Sprintf(`INSERT INTO %s (name, value, created_at, rotated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(name) DO NOTHING`, table)
	if _, err := db.Exec(ctx, insert, name, value); err != nil {
		return nil, fmt.Errorf("failed to store %s: %w", name, err)
	}

	// Re-read so a concurrent generator's value wins consistently for every
	// caller rather than each returning its own losing candidate.
	return GetSecret(ctx, db, name)
}

// RotateSecret generates a new value for the named secret and keeps the
// prior value valid for the grace window so in-flight HMACs still verify.
// The new secret is returned; the caller is responsible for the audit log
// entry (AI.md PART 11 "Secret Rotation").
func RotateSecret(ctx context.Context, db *DB, name string) (*AppSecret, error) {
	current, err := EnsureSecret(ctx, db, name)
	if err != nil {
		return nil, err
	}

	value, err := generateSecret()
	if err != nil {
		return nil, err
	}

	table := ServerTableName(db, "app_secrets")
	update := fmt.Sprintf(`UPDATE %s SET value = ?, previous_value = ?,
		previous_expires_at = ?, rotated_at = CURRENT_TIMESTAMP
		WHERE name = ?`, table)
	if _, err := db.Exec(ctx, update, value, current.Value,
		time.Now().Add(secretGrace), name); err != nil {
		return nil, fmt.Errorf("failed to rotate %s: %w", name, err)
	}
	return GetSecret(ctx, db, name)
}
