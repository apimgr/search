package user

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// EmailManager handles user email operations per AI.md PART 31
// Account email: Security & account recovery (password reset, 2FA recovery, security alerts)
// Notification email: Non-security communications (newsletters, updates, general notifications)
type EmailManager struct {
	db *sql.DB
}

// UserEmail represents an email address associated with a user
type UserEmail struct {
	ID                  string     `json:"id" db:"id"`
	UserID              string     `json:"user_id" db:"user_id"`
	Email               string     `json:"email" db:"email"`
	Verified            bool       `json:"verified" db:"verified"`
	IsPrimary           bool       `json:"is_primary" db:"is_primary"`
	IsNotification      bool       `json:"is_notification" db:"is_notification"`
	VerificationToken   string     `json:"-" db:"verification_token"`
	VerificationExpires *time.Time `json:"-" db:"verification_expires"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	VerifiedAt          *time.Time `json:"verified_at,omitempty" db:"verified_at"`
}


// Email errors
var (
	ErrEmailExists         = errors.New("email address already registered")
	ErrEmailNotFoundEM     = errors.New("email address not found")
	ErrEmailAlreadyPrimary = errors.New("email is already the primary address")
	ErrCannotRemovePrimary = errors.New("cannot remove primary email address")
	ErrVerificationExpired = errors.New("verification link has expired")
	ErrInvalidVerification = errors.New("invalid verification token")
	ErrMinimumOneEmail     = errors.New("user must have at least one email address")
)

// NewEmailManager creates a new email manager
func NewEmailManager(db *sql.DB) *EmailManager {
	return &EmailManager{db: db}
}

// AddEmail adds a new email address for a user
func (em *EmailManager) AddEmail(ctx context.Context, userID, email string) (*UserEmail, string, error) {
	email = NormalizeEmail(email)
	if err := ValidateEmail(email); err != nil {
		return nil, "", err
	}

	// Check if email already exists
	var count int
	err := em.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM user_emails WHERE email = ?
	`, email).Scan(&count)
	if err != nil {
		return nil, "", fmt.Errorf("failed to check email: %w", err)
	}
	if count > 0 {
		return nil, "", ErrEmailExists
	}

	// Also check the main users table
	err = em.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM users WHERE email = ? AND id != ?
	`, email, userID).Scan(&count)
	if err != nil {
		return nil, "", fmt.Errorf("failed to check primary email: %w", err)
	}
	if count > 0 {
		return nil, "", ErrEmailExists
	}

	// Generate verification token
	token, err := generateVerificationToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate verification token: %w", err)
	}

	// Generate ID
	id, err := GenerateToken(16)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate email ID: %w", err)
	}

	now := time.Now()
	expires := now.Add(24 * time.Hour)

	userEmail := &UserEmail{
		ID:                  id,
		UserID:              userID,
		Email:               email,
		Verified:            false,
		IsPrimary:           false,
		IsNotification:      false,
		VerificationToken:   token,
		VerificationExpires: &expires,
		CreatedAt:           now,
	}

	_, err = em.db.ExecContext(ctx, `
		INSERT INTO user_emails (id, user_id, email, verified, is_primary, is_notification, verification_token, verification_expires, created_at)
		VALUES (?, ?, ?, 0, 0, 0, ?, ?, ?)
	`, userEmail.ID, userEmail.UserID, userEmail.Email, token, expires, now)
	if err != nil {
		return nil, "", fmt.Errorf("failed to add email: %w", err)
	}

	return userEmail, token, nil
}

// VerifyEmail verifies an email address using a token
func (em *EmailManager) VerifyEmail(ctx context.Context, token string) (*UserEmail, error) {
	var userEmail UserEmail
	var expires sql.NullTime

	err := em.db.QueryRowContext(ctx, `
		SELECT id, user_id, email, verified, is_primary, is_notification, verification_expires, created_at
		FROM user_emails WHERE verification_token = ?
	`, token).Scan(
		&userEmail.ID, &userEmail.UserID, &userEmail.Email, &userEmail.Verified,
		&userEmail.IsPrimary, &userEmail.IsNotification, &expires, &userEmail.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidVerification
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find email: %w", err)
	}

	if expires.Valid && time.Now().After(expires.Time) {
		return nil, ErrVerificationExpired
	}

	now := time.Now()
	_, err = em.db.ExecContext(ctx, `
		UPDATE user_emails SET verified = 1, verification_token = NULL, verification_expires = NULL, verified_at = ?
		WHERE id = ?
	`, now, userEmail.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to verify email: %w", err)
	}

	userEmail.Verified = true
	userEmail.VerifiedAt = &now
	userEmail.VerificationToken = ""
	userEmail.VerificationExpires = nil

	return &userEmail, nil
}

// SetAsNotificationEmail sets an email as the notification email
func (em *EmailManager) SetAsNotificationEmail(ctx context.Context, userID, emailID string) error {
	// Verify the email exists, belongs to user, and is verified
	var verified bool
	err := em.db.QueryRowContext(ctx, `
		SELECT verified FROM user_emails WHERE id = ? AND user_id = ?
	`, emailID, userID).Scan(&verified)
	if err == sql.ErrNoRows {
		return ErrEmailNotFoundEM
	}
	if err != nil {
		return fmt.Errorf("failed to get email: %w", err)
	}
	if !verified {
		return ErrEmailNotVerified
	}

	// Clear existing notification email
	_, err = em.db.ExecContext(ctx, `
		UPDATE user_emails SET is_notification = 0 WHERE user_id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear notification flag: %w", err)
	}

	// Also clear on users table
	_, err = em.db.ExecContext(ctx, `
		UPDATE users SET notification_email = NULL, notification_email_verified = 0 WHERE id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear user notification email: %w", err)
	}

	// Set new notification email
	_, err = em.db.ExecContext(ctx, `
		UPDATE user_emails SET is_notification = 1 WHERE id = ? AND user_id = ?
	`, emailID, userID)
	if err != nil {
		return fmt.Errorf("failed to set notification email: %w", err)
	}

	// Update users table with notification email
	var email string
	_ = em.db.QueryRowContext(ctx, "SELECT email FROM user_emails WHERE id = ?", emailID).Scan(&email)
	if email != "" {
		_, _ = em.db.ExecContext(ctx, `
			UPDATE users SET notification_email = ?, notification_email_verified = 1 WHERE id = ?
		`, email, userID)
	}

	return nil
}

// ClearNotificationEmail removes the notification email designation
func (em *EmailManager) ClearNotificationEmail(ctx context.Context, userID string) error {
	_, err := em.db.ExecContext(ctx, `
		UPDATE user_emails SET is_notification = 0 WHERE user_id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear notification email: %w", err)
	}

	_, err = em.db.ExecContext(ctx, `
		UPDATE users SET notification_email = NULL, notification_email_verified = 0 WHERE id = ?
	`, userID)
	if err != nil {
		return fmt.Errorf("failed to clear user notification email: %w", err)
	}

	return nil
}

// RemoveEmail removes an email address
func (em *EmailManager) RemoveEmail(ctx context.Context, userID, emailID string) error {
	// Check if this is the primary email
	var isPrimary bool
	err := em.db.QueryRowContext(ctx, `
		SELECT is_primary FROM user_emails WHERE id = ? AND user_id = ?
	`, emailID, userID).Scan(&isPrimary)
	if err == sql.ErrNoRows {
		return ErrEmailNotFoundEM
	}
	if err != nil {
		return fmt.Errorf("failed to get email: %w", err)
	}
	if isPrimary {
		return ErrCannotRemovePrimary
	}

	// Delete the email
	result, err := em.db.ExecContext(ctx, `
		DELETE FROM user_emails WHERE id = ? AND user_id = ?
	`, emailID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove email: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrEmailNotFoundEM
	}

	return nil
}

// GetEmails returns all email addresses for a user
func (em *EmailManager) GetEmails(ctx context.Context, userID string) ([]*UserEmail, error) {
	rows, err := em.db.QueryContext(ctx, `
		SELECT id, user_id, email, verified, is_primary, is_notification, created_at, verified_at
		FROM user_emails WHERE user_id = ?
		ORDER BY is_primary DESC, created_at ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get emails: %w", err)
	}
	defer rows.Close()

	var emails []*UserEmail
	for rows.Next() {
		var email UserEmail
		var verifiedAt sql.NullTime
		err := rows.Scan(
			&email.ID, &email.UserID, &email.Email, &email.Verified,
			&email.IsPrimary, &email.IsNotification, &email.CreatedAt, &verifiedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan email: %w", err)
		}
		if verifiedAt.Valid {
			email.VerifiedAt = &verifiedAt.Time
		}
		emails = append(emails, &email)
	}

	return emails, rows.Err()
}

// GetAccountEmail returns the account (primary) email for a user
func (em *EmailManager) GetAccountEmail(ctx context.Context, userID string) (string, error) {
	// First try user_emails table
	var email string
	err := em.db.QueryRowContext(ctx, `
		SELECT email FROM user_emails WHERE user_id = ? AND is_primary = 1
	`, userID).Scan(&email)
	if err == nil {
		return email, nil
	}

	// Fall back to users table
	err = em.db.QueryRowContext(ctx, `
		SELECT email FROM users WHERE id = ?
	`, userID).Scan(&email)
	if err == sql.ErrNoRows {
		return "", ErrEmailNotFoundEM
	}
	if err != nil {
		return "", fmt.Errorf("failed to get account email: %w", err)
	}

	return email, nil
}

// GetNotificationEmail returns the notification email for a user
// Falls back to account email if no notification email is set
func (em *EmailManager) GetNotificationEmail(ctx context.Context, userID string) (string, error) {
	// First check users table notification_email
	var notificationEmail sql.NullString
	err := em.db.QueryRowContext(ctx, `
		SELECT notification_email FROM users WHERE id = ?
	`, userID).Scan(&notificationEmail)
	if err != nil && err != sql.ErrNoRows {
		return "", fmt.Errorf("failed to get notification email: %w", err)
	}
	if notificationEmail.Valid && notificationEmail.String != "" {
		return notificationEmail.String, nil
	}

	// Try user_emails table
	var email string
	err = em.db.QueryRowContext(ctx, `
		SELECT email FROM user_emails WHERE user_id = ? AND is_notification = 1 AND verified = 1
	`, userID).Scan(&email)
	if err == nil {
		return email, nil
	}

	// Fall back to account email
	return em.GetAccountEmail(ctx, userID)
}

// GetEmailForType returns the appropriate email for the given type
func (em *EmailManager) GetEmailForType(ctx context.Context, userID string, emailType string) (string, error) {
	switch emailType {
	case EmailTypeAccount:
		return em.GetAccountEmail(ctx, userID)
	case EmailTypeNotification:
		return em.GetNotificationEmail(ctx, userID)
	default:
		return em.GetAccountEmail(ctx, userID)
	}
}

// ResendVerification resends a verification email
func (em *EmailManager) ResendVerification(ctx context.Context, emailID string) (string, error) {
	// Check if email exists and is not verified
	var verified bool
	var userID string
	err := em.db.QueryRowContext(ctx, `
		SELECT user_id, verified FROM user_emails WHERE id = ?
	`, emailID).Scan(&userID, &verified)
	if err == sql.ErrNoRows {
		return "", ErrEmailNotFoundEM
	}
	if err != nil {
		return "", fmt.Errorf("failed to get email: %w", err)
	}
	if verified {
		return "", errors.New("email already verified")
	}

	// Generate new token
	token, err := generateVerificationToken()
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	expires := time.Now().Add(24 * time.Hour)
	_, err = em.db.ExecContext(ctx, `
		UPDATE user_emails SET verification_token = ?, verification_expires = ? WHERE id = ?
	`, token, expires, emailID)
	if err != nil {
		return "", fmt.Errorf("failed to update token: %w", err)
	}

	return token, nil
}

// MaskEmail masks an email for display (j***n@e***.com format)
func MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return "***@***.***"
	}

	local := parts[0]
	domain := parts[1]

	// Mask local part
	maskedLocal := maskString(local)

	// Mask domain (before TLD)
	domainParts := strings.Split(domain, ".")
	if len(domainParts) >= 2 {
		maskedDomain := maskDomainPart(domainParts[0])
		tld := strings.Join(domainParts[1:], ".")
		return maskedLocal + "@" + maskedDomain + "." + tld
	}

	return maskedLocal + "@" + maskString(domain)
}

// maskString masks a string keeping first and last characters
func maskString(s string) string {
	if len(s) <= 2 {
		return "***"
	}
	return string(s[0]) + "***" + string(s[len(s)-1])
}

// maskDomainPart masks domain part, showing first char even for short domains
func maskDomainPart(s string) string {
	if len(s) <= 1 {
		return "***"
	}
	if len(s) == 2 {
		return string(s[0]) + "***"
	}
	return string(s[0]) + "***" + string(s[len(s)-1])
}

// generateVerificationToken generates a secure verification token
func generateVerificationToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// UserEmailInfo represents user email info for display (different from User.EmailInfo)
type UserEmailInfo struct {
	ID             string     `json:"id"`
	Email          string     `json:"email"`
	MaskedEmail    string     `json:"masked_email"`
	Verified       bool       `json:"verified"`
	IsPrimary      bool       `json:"is_primary"`
	IsNotification bool       `json:"is_notification"`
	CreatedAt      time.Time  `json:"created_at"`
	VerifiedAt     *time.Time `json:"verified_at,omitempty"`
}

// ToInfo converts UserEmail to UserEmailInfo for display
func (ue *UserEmail) ToInfo() UserEmailInfo {
	return UserEmailInfo{
		ID:             ue.ID,
		Email:          ue.Email,
		MaskedEmail:    MaskEmail(ue.Email),
		Verified:       ue.Verified,
		IsPrimary:      ue.IsPrimary,
		IsNotification: ue.IsNotification,
		CreatedAt:      ue.CreatedAt,
		VerifiedAt:     ue.VerifiedAt,
	}
}
