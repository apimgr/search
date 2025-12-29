package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/apimgr/search/src/config"
)

// Session represents a user session
type Session struct {
	ID        string
	Data      map[string]interface{}
	UserID    string
	IP        string
	UserAgent string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
}

// SessionManager manages user sessions
type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	config   *config.Config
}

// NewSessionManager creates a new session manager
func NewSessionManager(cfg *config.Config) *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*Session),
		config:   cfg,
	}

	// Start cleanup goroutine
	go sm.cleanup()

	return sm
}

// cleanup removes expired sessions periodically
func (sm *SessionManager) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		sm.mu.Lock()
		now := time.Now()
		for id, sess := range sm.sessions {
			if now.After(sess.ExpiresAt) {
				delete(sm.sessions, id)
			}
		}
		sm.mu.Unlock()
	}
}

// Create creates a new session
func (sm *SessionManager) Create(userID, ip, userAgent string) *Session {
	// Generate session ID
	b := make([]byte, 32)
	rand.Read(b)
	id := hex.EncodeToString(b)

	// Get max age from config (use user session settings for general sessions)
	maxAge := sm.config.Server.Session.GetUserMaxAge()
	duration := time.Duration(maxAge) * time.Second

	session := &Session{
		ID:        id,
		Data:      make(map[string]interface{}),
		UserID:    userID,
		IP:        ip,
		UserAgent: userAgent,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(duration),
		LastSeen:  time.Now(),
	}

	sm.mu.Lock()
	sm.sessions[id] = session
	sm.mu.Unlock()

	return session
}

// Get retrieves a session by ID
func (sm *SessionManager) Get(id string) (*Session, bool) {
	sm.mu.RLock()
	session, exists := sm.sessions[id]
	sm.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Check if expired
	if time.Now().After(session.ExpiresAt) {
		sm.Destroy(id)
		return nil, false
	}

	// Update last seen
	sm.mu.Lock()
	session.LastSeen = time.Now()
	sm.mu.Unlock()

	return session, true
}

// Destroy removes a session
func (sm *SessionManager) Destroy(id string) {
	sm.mu.Lock()
	delete(sm.sessions, id)
	sm.mu.Unlock()
}

// Refresh extends a session's expiration
func (sm *SessionManager) Refresh(id string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	session, exists := sm.sessions[id]
	if !exists {
		return false
	}

	// Use max age from config
	maxAge := sm.config.Server.Session.GetUserMaxAge()
	duration := time.Duration(maxAge) * time.Second
	session.ExpiresAt = time.Now().Add(duration)
	session.LastSeen = time.Now()

	return true
}

// SetCookie sets the session cookie on the response
func (sm *SessionManager) SetCookie(w http.ResponseWriter, session *Session) {
	cfg := sm.config.Server.Session

	sameSite := http.SameSiteLaxMode
	switch cfg.GetSameSite() {
	case "strict", "Strict":
		sameSite = http.SameSiteStrictMode
	case "none", "None":
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     cfg.GetUserCookieName(),
		Value:    session.ID,
		Path:     "/",
		HttpOnly: cfg.IsHTTPOnly(),
		Secure:   cfg.IsSecure(sm.config.Server.SSL.Enabled),
		SameSite: sameSite,
		Expires:  session.ExpiresAt,
	})
}

// ClearCookie removes the session cookie
func (sm *SessionManager) ClearCookie(w http.ResponseWriter) {
	cfg := sm.config.Server.Session

	http.SetCookie(w, &http.Cookie{
		Name:     cfg.GetUserCookieName(),
		Value:    "",
		Path:     "/",
		HttpOnly: cfg.IsHTTPOnly(),
		Secure:   cfg.IsSecure(sm.config.Server.SSL.Enabled),
		MaxAge:   -1,
	})
}

// GetFromRequest retrieves session from request cookie
func (sm *SessionManager) GetFromRequest(r *http.Request) (*Session, bool) {
	cfg := sm.config.Server.Session

	cookie, err := r.Cookie(cfg.GetUserCookieName())
	if err != nil {
		return nil, false
	}

	return sm.Get(cookie.Value)
}

// Count returns the number of active sessions
func (sm *SessionManager) Count() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.sessions)
}
