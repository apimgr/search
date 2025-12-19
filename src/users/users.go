package users

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// User represents a registered user
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
	ErrPasswordRequired     = errors.New("password is required")
	ErrPasswordTooShort     = errors.New("password must be at least 8 characters")
	ErrPasswordTooWeak      = errors.New("password must contain at least one uppercase letter, one lowercase letter, and one number")
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
// Per TEMPLATE.md specification - 100+ reserved words
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
func ValidatePassword(password string, minLength int) error {
	if password == "" {
		return ErrPasswordRequired
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

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// CheckPassword compares a password with a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateToken generates a secure random token
func GenerateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// GenerateSessionToken generates a session token (32 bytes = 64 hex chars)
func GenerateSessionToken() (string, error) {
	return GenerateToken(32)
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
