package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
	"golang.org/x/crypto/bcrypt"
)

// AuthManager handles admin authentication
type AuthManager struct {
	config   *config.Config
	sessions map[string]*AdminSession
	tokens   map[string]*APIToken
	mu       sync.RWMutex
}

// AdminSession represents an authenticated admin session
type AdminSession struct {
	ID        string
	UserID    string
	Username  string
	CreatedAt time.Time
	ExpiresAt time.Time
	IP        string
	UserAgent string
}

// APIToken represents a bearer token for API access
type APIToken struct {
	Token       string
	Name        string
	Description string
	Permissions []string
	CreatedAt   time.Time
	ExpiresAt   time.Time
	LastUsed    time.Time
}

// NewAuthManager creates a new auth manager
func NewAuthManager(cfg *config.Config) *AuthManager {
	am := &AuthManager{
		config:   cfg,
		sessions: make(map[string]*AdminSession),
		tokens:   make(map[string]*APIToken),
	}

	// Start cleanup goroutine
	go am.cleanupLoop()

	return am
}

// Authenticate validates username and password
func (am *AuthManager) Authenticate(username, password string) bool {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// Check against configured admin user
	if am.config.Server.Admin.Username == "" || am.config.Server.Admin.Password == "" {
		return false
	}

	// Constant-time comparison for username to prevent timing attacks
	usernameMatch := subtle.ConstantTimeCompare(
		[]byte(username),
		[]byte(am.config.Server.Admin.Username),
	) == 1

	if !usernameMatch {
		return false
	}

	// Try bcrypt comparison first (for hashed passwords)
	storedPassword := am.config.Server.Admin.Password
	if err := bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(password)); err == nil {
		return true
	}

	// Fall back to constant-time plain comparison for initial setup/migration
	// This allows the first login with auto-generated password to work
	return subtle.ConstantTimeCompare([]byte(password), []byte(storedPassword)) == 1
}

// CreateSession creates a new admin session
func (am *AuthManager) CreateSession(username, ip, userAgent string) *AdminSession {
	am.mu.Lock()
	defer am.mu.Unlock()

	sessionID := generateSecureToken(32)
	session := &AdminSession{
		ID:        sessionID,
		UserID:    username,
		Username:  username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(time.Duration(am.config.Server.Session.Timeout) * time.Second),
		IP:        ip,
		UserAgent: userAgent,
	}

	am.sessions[sessionID] = session
	return session
}

// GetSession retrieves a session by ID
func (am *AuthManager) GetSession(sessionID string) (*AdminSession, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	session, ok := am.sessions[sessionID]
	if !ok {
		return nil, false
	}

	// Check expiration
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}

	return session, true
}

// DeleteSession removes a session
func (am *AuthManager) DeleteSession(sessionID string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.sessions, sessionID)
}

// RefreshSession extends a session's expiration
func (am *AuthManager) RefreshSession(sessionID string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	session, ok := am.sessions[sessionID]
	if !ok {
		return false
	}

	session.ExpiresAt = time.Now().Add(time.Duration(am.config.Server.Session.Timeout) * time.Second)
	return true
}

// CreateAPIToken creates a new API bearer token
func (am *AuthManager) CreateAPIToken(name, description string, permissions []string, validDays int) *APIToken {
	am.mu.Lock()
	defer am.mu.Unlock()

	token := generateSecureToken(48)
	apiToken := &APIToken{
		Token:       token,
		Name:        name,
		Description: description,
		Permissions: permissions,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().AddDate(0, 0, validDays),
		LastUsed:    time.Time{},
	}

	am.tokens[token] = apiToken
	return apiToken
}

// ValidateAPIToken validates a bearer token and returns permissions
func (am *AuthManager) ValidateAPIToken(token string) (*APIToken, bool) {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Check against static token from config
	if am.config.Server.Admin.APIToken != "" {
		if subtle.ConstantTimeCompare([]byte(token), []byte(am.config.Server.Admin.APIToken)) == 1 {
			return &APIToken{
				Token:       token,
				Name:        "config",
				Permissions: []string{"*"},
				CreatedAt:   time.Now(),
				ExpiresAt:   time.Now().AddDate(100, 0, 0), // Never expires
			}, true
		}
	}

	// Check dynamic tokens
	apiToken, ok := am.tokens[token]
	if !ok {
		return nil, false
	}

	// Check expiration
	if time.Now().After(apiToken.ExpiresAt) {
		return nil, false
	}

	// Update last used
	apiToken.LastUsed = time.Now()

	return apiToken, true
}

// RevokeAPIToken revokes an API token
func (am *AuthManager) RevokeAPIToken(token string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	if _, ok := am.tokens[token]; ok {
		delete(am.tokens, token)
		return true
	}
	return false
}

// ListAPITokens returns all active API tokens (without the actual token values)
func (am *AuthManager) ListAPITokens() []*APIToken {
	am.mu.RLock()
	defer am.mu.RUnlock()

	tokens := make([]*APIToken, 0, len(am.tokens))
	for _, t := range am.tokens {
		// Return a copy with masked token
		tokens = append(tokens, &APIToken{
			Token:       maskToken(t.Token),
			Name:        t.Name,
			Description: t.Description,
			Permissions: t.Permissions,
			CreatedAt:   t.CreatedAt,
			ExpiresAt:   t.ExpiresAt,
			LastUsed:    t.LastUsed,
		})
	}
	return tokens
}

// cleanupLoop periodically removes expired sessions and tokens
func (am *AuthManager) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		am.cleanup()
	}
}

// cleanup removes expired sessions and tokens
func (am *AuthManager) cleanup() {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()

	// Cleanup sessions
	for id, session := range am.sessions {
		if now.After(session.ExpiresAt) {
			delete(am.sessions, id)
		}
	}

	// Cleanup tokens
	for id, token := range am.tokens {
		if now.After(token.ExpiresAt) {
			delete(am.tokens, id)
		}
	}
}

// SetSessionCookie sets the admin session cookie
func (am *AuthManager) SetSessionCookie(w http.ResponseWriter, session *AdminSession) {
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    session.ID,
		Path:     "/admin",
		HttpOnly: true,
		Secure:   am.config.Server.SSL.Enabled,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   am.config.Server.Session.Timeout,
	})
}

// ClearSessionCookie removes the admin session cookie
func (am *AuthManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    "",
		Path:     "/admin",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

// GetSessionFromRequest extracts session from request cookie
func (am *AuthManager) GetSessionFromRequest(r *http.Request) (*AdminSession, bool) {
	cookie, err := r.Cookie("admin_session")
	if err != nil {
		return nil, false
	}
	return am.GetSession(cookie.Value)
}

// GetTokenFromRequest extracts bearer token from Authorization header
func (am *AuthManager) GetTokenFromRequest(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return ""
	}

	// Check for Bearer token
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	return ""
}

// Helper functions

// generateSecureToken generates a cryptographically secure random token
func generateSecureToken(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to time-based token (not ideal but better than nothing)
		return base64.URLEncoding.EncodeToString([]byte(time.Now().String()))[:length]
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}

// hashPassword creates a bcrypt hash of the password
func hashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		// This should never happen with valid input
		return ""
	}
	return string(hash)
}

// HashPassword exports the hash function for use in config
func HashPassword(password string) string {
	return hashPassword(password)
}

// VerifyPassword checks if a password matches a bcrypt hash
func VerifyPassword(password, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// maskToken masks a token for display (shows first 8 and last 4 characters)
func maskToken(token string) string {
	if len(token) <= 12 {
		return "********"
	}
	return token[:8] + "..." + token[len(token)-4:]
}

// GetClientIP extracts the client IP from the request
func GetClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}
