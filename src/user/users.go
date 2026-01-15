package user

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// User represents a registered user
// Per AI.md PART 31: Account email vs Notification email
// - Email: Account email for security (password reset, 2FA, security alerts, login notifications)
// - NotificationEmail: Non-security communications (newsletters, updates, general notifications)
type User struct {
	ID            int64      `json:"id" db:"id"`
	Username      string     `json:"username" db:"username"`
	Email         string     `json:"email" db:"email"`
	PasswordHash  string     `json:"-" db:"password_hash"`
	DisplayName   string     `json:"display_name,omitempty" db:"display_name"`
	AvatarURL     string     `json:"avatar_url,omitempty" db:"avatar_url"`
	Bio           string     `json:"bio,omitempty" db:"bio"`
	Role          string     `json:"role" db:"role"`
	EmailVerified bool       `json:"email_verified" db:"email_verified"`
	Active        bool       `json:"active" db:"active"`
	CreatedAt     time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" db:"updated_at"`
	LastLogin     *time.Time `json:"last_login,omitempty" db:"last_login"`

	// Notification email (per AI.md PART 31)
	// Optional separate email for non-security communications
	NotificationEmail         string `json:"notification_email,omitempty" db:"notification_email"`
	NotificationEmailVerified bool   `json:"notification_email_verified" db:"notification_email_verified"`
}

// UserSession represents an active user session
type UserSession struct {
	ID         int64     `json:"id" db:"id"`
	UserID     int64     `json:"user_id" db:"user_id"`
	Token      string    `json:"-" db:"token"`
	IPAddress  string    `json:"ip_address" db:"ip_address"`
	UserAgent  string    `json:"user_agent" db:"user_agent"`
	DeviceName string    `json:"device_name" db:"device_name"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	ExpiresAt  time.Time `json:"expires_at" db:"expires_at"`
	LastUsed   time.Time `json:"last_used" db:"last_used"`
}

// UserRole constants
const (
	RoleUser      = "user"
	RoleAdmin     = "admin"
	RoleModerator = "moderator"
)

// Validation errors
var (
	ErrUsernameRequired     = errors.New("username is required")
	ErrUsernameTooShort     = errors.New("username must be at least 3 characters")
	ErrUsernameTooLong      = errors.New("username must be at most 32 characters")
	ErrUsernameInvalid      = errors.New("username can only contain lowercase letters, numbers, underscore, and hyphen")
	ErrUsernameReserved     = errors.New("this username is reserved")
	ErrEmailRequired        = errors.New("email is required")
	ErrEmailInvalid         = errors.New("invalid email address")
	ErrPasswordRequired   = errors.New("password is required")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
	ErrPasswordTooWeak    = errors.New("password must contain at least one uppercase letter, one lowercase letter, and one number")
	ErrPasswordWhitespace = errors.New("password cannot start or end with whitespace")
	ErrUserNotFound         = errors.New("user not found")
	ErrInvalidCredentials   = errors.New("invalid username or password")
	ErrUserInactive         = errors.New("user account is inactive")
	ErrEmailNotVerified     = errors.New("email not verified")
	ErrUsernameTaken        = errors.New("username is already taken")
	ErrEmailTaken           = errors.New("email is already registered")
	ErrSessionExpired       = errors.New("session expired")
	ErrSessionNotFound      = errors.New("session not found")
	ErrRegistrationDisabled = errors.New("registration is currently disabled")
)

// Username validation regex
var usernameRegex = regexp.MustCompile(`^[a-z0-9_-]+$`)

// Email validation regex (basic)
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// BlockedUsernames contains reserved usernames that cannot be registered
// Per AI.md specification - 100+ reserved words
var BlockedUsernames = map[string]bool{
	// System & Admin
	"admin":         true,
	"administrator": true,
	"root":          true,
	"system":        true,
	"superuser":     true,
	"super":         true,
	"sudo":          true,
	"owner":         true,
	"master":        true,
	"operator":      true,
	"moderator":     true,
	"mod":           true,
	"staff":         true,
	"support":       true,
	"help":          true,
	"info":          true,
	"contact":       true,
	"abuse":         true,
	"postmaster":    true,
	"webmaster":     true,
	"hostmaster":    true,
	"security":      true,
	"noreply":       true,
	"no-reply":      true,
	"mailer-daemon": true,
	"nobody":        true,
	"anonymous":     true,
	"guest":         true,
	"test":          true,
	"testing":       true,
	"demo":          true,
	"example":       true,
	"sample":        true,

	// Application-specific
	"search":      true,
	"api":         true,
	"www":         true,
	"web":         true,
	"app":         true,
	"mobile":      true,
	"static":      true,
	"assets":      true,
	"cdn":         true,
	"media":       true,
	"images":      true,
	"files":       true,
	"upload":      true,
	"uploads":     true,
	"download":    true,
	"downloads":   true,
	"public":      true,
	"private":     true,
	"internal":    true,
	"external":    true,
	"beta":        true,
	"alpha":       true,
	"staging":     true,
	"production":  true,
	"dev":         true,
	"development": true,
	"localhost":   true,

	// Routes & Features
	"login":        true,
	"logout":       true,
	"signin":       true,
	"signout":      true,
	"signup":       true,
	"register":     true,
	"registration": true,
	"auth":         true,
	"oauth":        true,
	"sso":          true,
	"account":      true,
	"accounts":     true,
	"profile":      true,
	"profiles":     true,
	"user":         true,
	"users":        true,
	"member":       true,
	"members":      true,
	"settings":     true,
	"preferences":  true,
	"config":       true,
	"configuration":true,
	"dashboard":    true,
	"home":         true,
	"about":        true,
	"terms":        true,
	"privacy":      true,
	"legal":        true,
	"tos":          true,
	"faq":          true,
	"feedback":     true,
	"report":       true,
	"status":       true,
	"health":       true,
	"healthz":      true,
	"metrics":      true,
	"stats":        true,
	"analytics":    true,

	// API & Technical
	"graphql":  true,
	"rest":     true,
	"webhook":  true,
	"webhooks": true,
	"callback": true,
	"redirect": true,
	"oauth2":   true,
	"openid":   true,
	"saml":     true,
	"token":    true,
	"tokens":   true,
	"key":      true,
	"keys":     true,
	"secret":   true,
	"secrets":  true,
	"password": true,
	"reset":    true,
	"verify":   true,
	"confirm":  true,
	"activate": true,
	"delete":   true,
	"remove":   true,
	"ban":      true,
	"block":    true,
	"unblock":  true,
	"mute":     true,
	"unmute":   true,

	// Common words
	"null":      true,
	"undefined": true,
	"true":      true,
	"false":     true,
	"nil":       true,
	"void":      true,
	"error":     true,
	"errors":    true,
	"success":   true,
	"failure":   true,
	"unknown":   true,
	"default":   true,
	"new":       true,
	"create":    true,
	"edit":      true,
	"update":    true,
	"all":       true,
	"none":      true,
	"everyone":  true,
}

// ValidateUsername validates a username
func ValidateUsername(username string) error {
	if username == "" {
		return ErrUsernameRequired
	}

	username = strings.ToLower(strings.TrimSpace(username))

	if len(username) < 3 {
		return ErrUsernameTooShort
	}

	if len(username) > 32 {
		return ErrUsernameTooLong
	}

	if !usernameRegex.MatchString(username) {
		return ErrUsernameInvalid
	}

	if IsBlockedUsername(username) {
		return ErrUsernameReserved
	}

	return nil
}

// IsBlockedUsername checks if a username is in the blocklist
func IsBlockedUsername(username string) bool {
	username = strings.ToLower(strings.TrimSpace(username))
	return BlockedUsernames[username]
}

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	if email == "" {
		return ErrEmailRequired
	}

	email = strings.ToLower(strings.TrimSpace(email))

	if !emailRegex.MatchString(email) {
		return ErrEmailInvalid
	}

	return nil
}

// ValidatePassword validates a password
// Per AI.md: Passwords cannot start or end with whitespace
func ValidatePassword(password string, minLength int) error {
	if password == "" {
		return ErrPasswordRequired
	}

	// Check for leading/trailing whitespace - passwords cannot start or end with whitespace
	if len(password) > 0 && (password[0] == ' ' || password[0] == '\t' || password[len(password)-1] == ' ' || password[len(password)-1] == '\t') {
		return ErrPasswordWhitespace
	}

	if len(password) < minLength {
		return ErrPasswordTooShort
	}

	var hasUpper, hasLower, hasNumber bool
	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsNumber(c):
			hasNumber = true
		}
	}

	if !hasUpper || !hasLower || !hasNumber {
		return ErrPasswordTooWeak
	}

	return nil
}

// Argon2id parameters per AI.md specification
const (
	argon2Time    = 3         // iterations (per AI.md line 932)
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// HashPassword hashes a password using Argon2id (per AI.md - NEVER bcrypt)
func HashPassword(password string) (string, error) {
	// Generate random salt
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	// Generate hash using Argon2id
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// Encode as: $argon2id$v=19$m=65536,t=1,p=4$<base64-salt>$<base64-hash>
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Time, argon2Threads, b64Salt, b64Hash), nil
}

// CheckPassword compares a password with an Argon2id hash
func CheckPassword(password, encodedHash string) bool {
	// Parse the encoded hash
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

	// Compute hash with same parameters
	computedHash := argon2.IDKey([]byte(password), salt, time, memory, threads, uint32(len(expectedHash)))

	// Constant-time comparison
	return subtle.ConstantTimeCompare(computedHash, expectedHash) == 1
}

// GenerateToken generates a secure random token
func GenerateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateSessionToken generates a session token with ses_ prefix
// Per AI.md PART 23: Session token prefix must be "ses_"
func GenerateSessionToken() (string, error) {
	token, err := GenerateToken(32)
	if err != nil {
		return "", err
	}
	return "ses_" + token, nil
}

// NormalizeUsername normalizes a username (lowercase, trimmed)
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// NormalizeEmail normalizes an email (lowercase, trimmed)
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NewUser creates a new user with validated and normalized fields
func NewUser(username, email, password string, minPasswordLength int) (*User, error) {
	// Normalize
	username = NormalizeUsername(username)
	email = NormalizeEmail(email)

	// Validate
	if err := ValidateUsername(username); err != nil {
		return nil, err
	}
	if err := ValidateEmail(email); err != nil {
		return nil, err
	}
	if err := ValidatePassword(password, minPasswordLength); err != nil {
		return nil, err
	}

	// Hash password
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	return &User{
		Username:      username,
		Email:         email,
		PasswordHash:  hash,
		Role:          RoleUser,
		EmailVerified: false,
		Active:        true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

// IsAdmin checks if the user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// IsModerator checks if the user has moderator role
func (u *User) IsModerator() bool {
	return u.Role == RoleModerator || u.Role == RoleAdmin
}

// CanLogin checks if the user can log in
func (u *User) CanLogin() bool {
	return u.Active
}

// PublicProfile returns a user's public profile data
type PublicProfile struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name,omitempty"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
	Bio         string    `json:"bio,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToPublicProfile converts a user to their public profile
func (u *User) ToPublicProfile() PublicProfile {
	displayName := u.DisplayName
	if displayName == "" {
		displayName = u.Username
	}
	return PublicProfile{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: displayName,
		AvatarURL:   u.AvatarURL,
		Bio:         u.Bio,
		CreatedAt:   u.CreatedAt,
	}
}

// Email type constants for dual email system (per AI.md PART 31)
const (
	// EmailTypeAccount is for security-related communications
	// Password reset, 2FA recovery, security alerts, login notifications
	EmailTypeAccount = "account"

	// EmailTypeNotification is for non-security communications
	// Newsletters, updates, marketing, general notifications
	EmailTypeNotification = "notification"
)

// GetAccountEmail returns the user's account email (primary email for security)
// Per AI.md PART 31: Account email receives security-sensitive communications ONLY
func (u *User) GetAccountEmail() string {
	return u.Email
}

// GetNotificationEmail returns the email to use for non-security notifications
// Per AI.md PART 31: If notification email is set and verified, use it
// Otherwise fall back to the account email
func (u *User) GetNotificationEmail() string {
	if u.NotificationEmail != "" && u.NotificationEmailVerified {
		return u.NotificationEmail
	}
	return u.Email
}

// GetEmailForType returns the appropriate email for the given email type
// Per AI.md PART 31: Account emails and notification emails have different purposes
func (u *User) GetEmailForType(emailType string) string {
	switch emailType {
	case EmailTypeAccount:
		return u.GetAccountEmail()
	case EmailTypeNotification:
		return u.GetNotificationEmail()
	default:
		return u.GetAccountEmail()
	}
}

// HasSeparateNotificationEmail checks if user has a verified separate notification email
func (u *User) HasSeparateNotificationEmail() bool {
	return u.NotificationEmail != "" && u.NotificationEmailVerified && u.NotificationEmail != u.Email
}

// SetNotificationEmail sets the notification email (requires verification before use)
func (u *User) SetNotificationEmail(email string) error {
	if email == "" {
		// Clear notification email
		u.NotificationEmail = ""
		u.NotificationEmailVerified = false
		return nil
	}

	email = NormalizeEmail(email)
	if err := ValidateEmail(email); err != nil {
		return err
	}

	u.NotificationEmail = email
	u.NotificationEmailVerified = false
	return nil
}

// VerifyNotificationEmail marks the notification email as verified
func (u *User) VerifyNotificationEmail() {
	if u.NotificationEmail != "" {
		u.NotificationEmailVerified = true
	}
}

// ClearNotificationEmail removes the separate notification email
// Notifications will fall back to the account email
func (u *User) ClearNotificationEmail() {
	u.NotificationEmail = ""
	u.NotificationEmailVerified = false
}

// EmailInfo provides information about user's email configuration
type EmailInfo struct {
	AccountEmail              string `json:"account_email"`
	AccountEmailVerified      bool   `json:"account_email_verified"`
	NotificationEmail         string `json:"notification_email,omitempty"`
	NotificationEmailVerified bool   `json:"notification_email_verified"`
	UsingSeparateNotification bool   `json:"using_separate_notification"`
}

// GetEmailInfo returns detailed information about user's email configuration
func (u *User) GetEmailInfo() EmailInfo {
	return EmailInfo{
		AccountEmail:              u.Email,
		AccountEmailVerified:      u.EmailVerified,
		NotificationEmail:         u.NotificationEmail,
		NotificationEmailVerified: u.NotificationEmailVerified,
		UsingSeparateNotification: u.HasSeparateNotificationEmail(),
	}
}
