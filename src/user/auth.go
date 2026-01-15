package user

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AuthManager handles user authentication
type AuthManager struct {
	db              *sql.DB
	sessionDuration time.Duration
	cookieName      string
	cookieDomain    string
	cookieSecure    bool
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	SessionDurationDays int
	CookieName          string
	CookieDomain        string
	CookieSecure        bool
}

// NewAuthManager creates a new authentication manager
func NewAuthManager(db *sql.DB, config AuthConfig) *AuthManager {
	if config.SessionDurationDays <= 0 {
		config.SessionDurationDays = 7
	}
	if config.CookieName == "" {
		config.CookieName = "user_session"
	}

	return &AuthManager{
		db:              db,
		sessionDuration: time.Duration(config.SessionDurationDays) * 24 * time.Hour,
		cookieName:      config.CookieName,
		cookieDomain:    config.CookieDomain,
		cookieSecure:    config.CookieSecure,
	}
}

// Register creates a new user account
func (am *AuthManager) Register(ctx context.Context, username, email, password string, minPasswordLength int) (*User, error) {
	// Create user with validation
	user, err := NewUser(username, email, password, minPasswordLength)
	if err != nil {
		return nil, err
	}

	// Check if username exists
	var count int
	err = am.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE username = ?", user.Username).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check username: %w", err)
	}
	if count > 0 {
		return nil, ErrUsernameTaken
	}

	// Check if email exists
	err = am.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE email = ?", user.Email).Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("failed to check email: %w", err)
	}
	if count > 0 {
		return nil, ErrEmailTaken
	}

	// Insert user
	result, err := am.db.ExecContext(ctx, `
		INSERT INTO users (username, email, password_hash, display_name, role, email_verified, active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, user.Username, user.Email, user.PasswordHash, user.DisplayName, user.Role, user.EmailVerified, user.Active, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get user ID: %w", err)
	}
	user.ID = id

	return user, nil
}

// Login authenticates a user and creates a session
func (am *AuthManager) Login(ctx context.Context, usernameOrEmail, password, ipAddress, userAgent string) (*User, *UserSession, error) {
	// Normalize input
	usernameOrEmail = strings.ToLower(strings.TrimSpace(usernameOrEmail))

	// Find user by username or email
	user, err := am.getUserByUsernameOrEmail(ctx, usernameOrEmail)
	if err != nil {
		return nil, nil, ErrInvalidCredentials
	}

	// Check password
	if !CheckPassword(password, user.PasswordHash) {
		return nil, nil, ErrInvalidCredentials
	}

	// Check if user can login
	if !user.CanLogin() {
		return nil, nil, ErrUserInactive
	}

	// Create session
	session, err := am.createSession(ctx, user.ID, ipAddress, userAgent)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Update last login
	_, _ = am.db.ExecContext(ctx, "UPDATE users SET last_login = ? WHERE id = ?", time.Now(), user.ID)

	return user, session, nil
}

// Logout terminates a user session
func (am *AuthManager) Logout(ctx context.Context, token string) error {
	_, err := am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE token = ?", token)
	return err
}

// LogoutAll terminates all sessions for a user except the current one
func (am *AuthManager) LogoutAll(ctx context.Context, userID int64, exceptToken string) error {
	_, err := am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE user_id = ? AND token != ?", userID, exceptToken)
	return err
}

// ValidateSession validates a session token and returns the user
func (am *AuthManager) ValidateSession(ctx context.Context, token string) (*User, *UserSession, error) {
	if token == "" {
		return nil, nil, ErrSessionNotFound
	}

	var session UserSession
	var user User

	err := am.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.token, s.ip_address, s.user_agent, s.device_name, s.created_at, s.expires_at, s.last_used,
		       u.id, u.username, u.email, u.password_hash, u.display_name, u.avatar_url, u.bio, u.role, u.email_verified, u.active, u.created_at, u.updated_at, u.last_login
		FROM user_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.token = ? AND s.expires_at > ?
	`, token, time.Now()).Scan(
		&session.ID, &session.UserID, &session.Token, &session.IPAddress, &session.UserAgent, &session.DeviceName, &session.CreatedAt, &session.ExpiresAt, &session.LastUsed,
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Role, &user.EmailVerified, &user.Active, &user.CreatedAt, &user.UpdatedAt, &user.LastLogin,
	)
	if err == sql.ErrNoRows {
		return nil, nil, ErrSessionExpired
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to validate session: %w", err)
	}

	if !user.Active {
		return nil, nil, ErrUserInactive
	}

	// Update last used
	_, _ = am.db.ExecContext(ctx, "UPDATE user_sessions SET last_used = ? WHERE id = ?", time.Now(), session.ID)

	return &user, &session, nil
}

// GetUserByID retrieves a user by ID
func (am *AuthManager) GetUserByID(ctx context.Context, id int64) (*User, error) {
	var user User
	var lastLogin sql.NullTime

	err := am.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, display_name, avatar_url, bio, role, email_verified, active, created_at, updated_at, last_login
		FROM users WHERE id = ?
	`, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Role, &user.EmailVerified, &user.Active, &user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by username
func (am *AuthManager) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	username = NormalizeUsername(username)
	return am.getUserByField(ctx, "username", username)
}

// GetUserByEmail retrieves a user by email
func (am *AuthManager) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	email = NormalizeEmail(email)
	return am.getUserByField(ctx, "email", email)
}

// UpdatePassword updates a user's password
func (am *AuthManager) UpdatePassword(ctx context.Context, userID int64, newPassword string, minPasswordLength int) error {
	if err := ValidatePassword(newPassword, minPasswordLength); err != nil {
		return err
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	_, err = am.db.ExecContext(ctx, "UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?", hash, time.Now(), userID)
	return err
}

// UpdateProfile updates a user's profile
func (am *AuthManager) UpdateProfile(ctx context.Context, userID int64, displayName, bio, avatarURL string) error {
	_, err := am.db.ExecContext(ctx, `
		UPDATE users SET display_name = ?, bio = ?, avatar_url = ?, updated_at = ? WHERE id = ?
	`, displayName, bio, avatarURL, time.Now(), userID)
	return err
}

// GetUserSessions retrieves all active sessions for a user
func (am *AuthManager) GetUserSessions(ctx context.Context, userID int64) ([]UserSession, error) {
	rows, err := am.db.QueryContext(ctx, `
		SELECT id, user_id, token, ip_address, user_agent, device_name, created_at, expires_at, last_used
		FROM user_sessions WHERE user_id = ? AND expires_at > ? ORDER BY last_used DESC
	`, userID, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to get sessions: %w", err)
	}
	defer rows.Close()

	var sessions []UserSession
	for rows.Next() {
		var s UserSession
		if err := rows.Scan(&s.ID, &s.UserID, &s.Token, &s.IPAddress, &s.UserAgent, &s.DeviceName, &s.CreatedAt, &s.ExpiresAt, &s.LastUsed); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// RevokeSession revokes a specific session
func (am *AuthManager) RevokeSession(ctx context.Context, userID, sessionID int64) error {
	result, err := am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE id = ? AND user_id = ?", sessionID, userID)
	if err != nil {
		return err
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// SetSessionCookie sets the session cookie on the response
func (am *AuthManager) SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     am.cookieName,
		Value:    token,
		Path:     "/",
		Domain:   am.cookieDomain,
		MaxAge:   int(am.sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   am.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie clears the session cookie
func (am *AuthManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     am.cookieName,
		Value:    "",
		Path:     "/",
		Domain:   am.cookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   am.cookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

// GetSessionToken extracts the session token from a request
func (am *AuthManager) GetSessionToken(r *http.Request) string {
	// Check cookie first
	if cookie, err := r.Cookie(am.cookieName); err == nil {
		return cookie.Value
	}

	// Check Authorization header (Bearer token)
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	return ""
}

// CleanupExpiredSessions removes expired sessions
func (am *AuthManager) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	result, err := am.db.ExecContext(ctx, "DELETE FROM user_sessions WHERE expires_at < ?", time.Now())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// VerifyEmail marks a user's email as verified
func (am *AuthManager) VerifyEmail(ctx context.Context, userID int64) error {
	_, err := am.db.ExecContext(ctx, "UPDATE users SET email_verified = 1, updated_at = ? WHERE id = ?", time.Now(), userID)
	return err
}

// SetUserActive activates or deactivates a user
func (am *AuthManager) SetUserActive(ctx context.Context, userID int64, active bool) error {
	_, err := am.db.ExecContext(ctx, "UPDATE users SET active = ?, updated_at = ? WHERE id = ?", active, time.Now(), userID)
	return err
}

// SetUserRole sets a user's role
func (am *AuthManager) SetUserRole(ctx context.Context, userID int64, role string) error {
	_, err := am.db.ExecContext(ctx, "UPDATE users SET role = ?, updated_at = ? WHERE id = ?", role, time.Now(), userID)
	return err
}

// Helper functions

func (am *AuthManager) getUserByUsernameOrEmail(ctx context.Context, usernameOrEmail string) (*User, error) {
	var user User
	var lastLogin sql.NullTime

	err := am.db.QueryRowContext(ctx, `
		SELECT id, username, email, password_hash, display_name, avatar_url, bio, role, email_verified, active, created_at, updated_at, last_login
		FROM users WHERE username = ? OR email = ?
	`, usernameOrEmail, usernameOrEmail).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Role, &user.EmailVerified, &user.Active, &user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

func (am *AuthManager) getUserByField(ctx context.Context, field, value string) (*User, error) {
	var user User
	var lastLogin sql.NullTime

	query := fmt.Sprintf(`
		SELECT id, username, email, password_hash, display_name, avatar_url, bio, role, email_verified, active, created_at, updated_at, last_login
		FROM users WHERE %s = ?
	`, field)

	err := am.db.QueryRowContext(ctx, query, value).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash, &user.DisplayName, &user.AvatarURL, &user.Bio, &user.Role, &user.EmailVerified, &user.Active, &user.CreatedAt, &user.UpdatedAt, &lastLogin,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

func (am *AuthManager) createSession(ctx context.Context, userID int64, ipAddress, userAgent string) (*UserSession, error) {
	token, err := GenerateSessionToken()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	expiresAt := now.Add(am.sessionDuration)

	// Extract device name from user agent (simplified)
	deviceName := extractDeviceName(userAgent)

	result, err := am.db.ExecContext(ctx, `
		INSERT INTO user_sessions (user_id, token, ip_address, user_agent, device_name, created_at, expires_at, last_used)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, userID, token, ipAddress, userAgent, deviceName, now, expiresAt, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()

	return &UserSession{
		ID:         id,
		UserID:     userID,
		Token:      token,
		IPAddress:  ipAddress,
		UserAgent:  userAgent,
		DeviceName: deviceName,
		CreatedAt:  now,
		ExpiresAt:  expiresAt,
		LastUsed:   now,
	}, nil
}

// extractDeviceName extracts a simple device description from user agent
func extractDeviceName(userAgent string) string {
	ua := strings.ToLower(userAgent)

	// Check for common patterns
	switch {
	case strings.Contains(ua, "iphone"):
		return "iPhone"
	case strings.Contains(ua, "ipad"):
		return "iPad"
	case strings.Contains(ua, "android"):
		if strings.Contains(ua, "mobile") {
			return "Android Phone"
		}
		return "Android Tablet"
	case strings.Contains(ua, "windows"):
		return "Windows PC"
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os"):
		return "Mac"
	case strings.Contains(ua, "linux"):
		return "Linux PC"
	case strings.Contains(ua, "chrome"):
		return "Chrome Browser"
	case strings.Contains(ua, "firefox"):
		return "Firefox Browser"
	case strings.Contains(ua, "safari"):
		return "Safari Browser"
	default:
		return "Unknown Device"
	}
}
