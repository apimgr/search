package admin

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"golang.org/x/crypto/argon2"
)

// AuthManager handles admin authentication
// Per AI.md PART 17: Admin sessions stored in admin_sessions table (server.db)
type AuthManager struct {
	config   *config.Config
	db       *database.DB // Database for session persistence (server.db)
	sessions map[string]*AdminSession // Fallback in-memory (only if db is nil)
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

// SetDatabase sets the database for session persistence
// Per AI.md PART 17: Admin sessions stored in admin_sessions table (server.db)
func (am *AuthManager) SetDatabase(db *database.DB) {
	am.mu.Lock()
	defer am.mu.Unlock()
	am.db = db
}

// Argon2id parameters per AI.md specification (line 932)
const (
	argon2Time    = 3         // iterations
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	argon2KeyLen  = 32
	argon2SaltLen = 16
)

// Authenticate validates username and password
func (am *AuthManager) Authenticate(username, password string) bool {
	// Per AI.md PART 6 line 6270: Admin auth BYPASSED in debug mode
	// For manual development only - NOT for automated tests
	if am.config.IsDebug() {
		return true
	}

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

	// Try Argon2id comparison first (for hashed passwords)
	storedPassword := am.config.Server.Admin.Password
	if verifyArgon2idHash(password, storedPassword) {
		return true
	}

	// Fall back to constant-time plain comparison for initial setup/migration
	// This allows the first login with auto-generated password to work
	return subtle.ConstantTimeCompare([]byte(password), []byte(storedPassword)) == 1
}

// verifyArgon2idHash verifies a password against an Argon2id hash
func verifyArgon2idHash(password, encodedHash string) bool {
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

// CreateSession creates a new admin session
// Per AI.md PART 17: Sessions stored in admin_sessions table when db is available
func (am *AuthManager) CreateSession(username, ip, userAgent string) *AdminSession {
	am.mu.Lock()
	defer am.mu.Unlock()

	sessionID := generateSecureToken(32)

	// Parse session duration from config (e.g., "30d", "24h")
	// Falls back to Timeout if Duration parsing fails, or 24h if both are 0
	// Get admin session max age from config (per AI.md PART 13)
	sessionDuration := time.Duration(am.config.Server.Session.GetAdminMaxAge()) * time.Second

	session := &AdminSession{
		ID:        sessionID,
		UserID:    username,
		Username:  username,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(sessionDuration),
		IP:        ip,
		UserAgent: userAgent,
	}

	// Store in database if available (per AI.md PART 17)
	if am.db != nil {
		ctx := context.Background()
		_, err := am.db.Exec(ctx,
			`INSERT INTO admin_sessions (token, username, ip_address, user_agent, expires_at)
			 VALUES (?, ?, ?, ?, ?)`,
			sessionID, username, ip, userAgent, session.ExpiresAt)
		if err != nil {
			// Log error but continue with in-memory fallback
			// This ensures graceful degradation
		}
	}

	// Also store in memory for fast lookups
	am.sessions[sessionID] = session
	return session
}

// GetSession retrieves a session by ID
// Per AI.md PART 17: Check database for session if not in memory (supports restart persistence)
func (am *AuthManager) GetSession(sessionID string) (*AdminSession, bool) {
	am.mu.RLock()
	session, ok := am.sessions[sessionID]
	am.mu.RUnlock()

	if ok {
		// Check expiration
		if time.Now().After(session.ExpiresAt) {
			return nil, false
		}
		return session, true
	}

	// Try database if not in memory (per AI.md PART 17)
	if am.db != nil {
		ctx := context.Background()
		row := am.db.QueryRow(ctx,
			`SELECT token, username, ip_address, user_agent, created_at, expires_at
			 FROM admin_sessions WHERE token = ? AND expires_at > CURRENT_TIMESTAMP`,
			sessionID)

		var s AdminSession
		err := row.Scan(&s.ID, &s.Username, &s.IP, &s.UserAgent, &s.CreatedAt, &s.ExpiresAt)
		if err == nil {
			s.UserID = s.Username // Map to existing field
			// Cache in memory for future lookups
			am.mu.Lock()
			am.sessions[sessionID] = &s
			am.mu.Unlock()
			return &s, true
		}
	}

	return nil, false
}

// DeleteSession removes a session
// Per AI.md PART 17: Remove from both memory and database
func (am *AuthManager) DeleteSession(sessionID string) {
	am.mu.Lock()
	defer am.mu.Unlock()
	delete(am.sessions, sessionID)

	// Also delete from database (per AI.md PART 17)
	if am.db != nil {
		ctx := context.Background()
		_, _ = am.db.Exec(ctx, `DELETE FROM admin_sessions WHERE token = ?`, sessionID)
	}
}

// RefreshSession extends a session's expiration
// Per AI.md PART 17: Update expiration in both memory and database
func (am *AuthManager) RefreshSession(sessionID string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	session, ok := am.sessions[sessionID]
	if !ok {
		return false
	}

	// Parse session duration from config (e.g., "30d", "24h")
	// Falls back to Timeout if Duration parsing fails, or 24h if both are 0
	// Get admin session max age from config (per AI.md PART 13)
	sessionDuration := time.Duration(am.config.Server.Session.GetAdminMaxAge()) * time.Second

	newExpiry := time.Now().Add(sessionDuration)
	session.ExpiresAt = newExpiry

	// Update database (per AI.md PART 17)
	if am.db != nil {
		ctx := context.Background()
		_, _ = am.db.Exec(ctx, `UPDATE admin_sessions SET expires_at = ? WHERE token = ?`, newExpiry, sessionID)
	}

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
// Per AI.md PART 17: Clean both in-memory and database
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

	// Cleanup expired sessions from database (per AI.md PART 17)
	if am.db != nil {
		ctx := context.Background()
		_, _ = am.db.Exec(ctx, `DELETE FROM admin_sessions WHERE expires_at <= CURRENT_TIMESTAMP`)
	}
}

// SetSessionCookie sets the admin session cookie
func (am *AuthManager) SetSessionCookie(w http.ResponseWriter, session *AdminSession) {
	cfg := am.config.Server.Session
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.GetAdminCookieName(),
		Value:    session.ID,
		Path:     "/admin",
		HttpOnly: cfg.IsHTTPOnly(),
		Secure:   cfg.IsSecure(am.config.Server.SSL.Enabled),
		SameSite: http.SameSiteStrictMode,
		MaxAge:   cfg.GetAdminMaxAge(),
	})
}

// ClearSessionCookie removes the admin session cookie
func (am *AuthManager) ClearSessionCookie(w http.ResponseWriter) {
	cfg := am.config.Server.Session
	http.SetCookie(w, &http.Cookie{
		Name:     cfg.GetAdminCookieName(),
		Value:    "",
		Path:     "/admin",
		HttpOnly: cfg.IsHTTPOnly(),
		MaxAge:   -1,
	})
}

// GetSessionFromRequest extracts session from request cookie
func (am *AuthManager) GetSessionFromRequest(r *http.Request) (*AdminSession, bool) {
	cookie, err := r.Cookie(am.config.Server.Session.GetAdminCookieName())
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

// hashPassword creates an Argon2id hash of the password (per AI.md - NEVER bcrypt)
func hashPassword(password string) string {
	// Generate random salt
	salt := make([]byte, argon2SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return ""
	}

	// Generate hash using Argon2id
	hash := argon2.IDKey([]byte(password), salt, argon2Time, argon2Memory, argon2Threads, argon2KeyLen)

	// Encode as: $argon2id$v=19$m=65536,t=1,p=4$<base64-salt>$<base64-hash>
	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Memory, argon2Time, argon2Threads, b64Salt, b64Hash)
}

// HashPassword exports the hash function for use in config
func HashPassword(password string) string {
	return hashPassword(password)
}

// VerifyPassword checks if a password matches an Argon2id hash
func VerifyPassword(password, hash string) bool {
	return verifyArgon2idHash(password, hash)
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
