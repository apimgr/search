package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/config"
)

// AuthPageData represents data for the unified login page (PART 11).
type AuthPageData struct {
	PageData
	Error    string
	Success  string
	Username string
	Redirect string
}

// handleLogin handles GET/POST on /auth/login.
//
// Per AI.md PART 11: /auth/login is the unified login form. Admin authentication
// is delegated to src/admin/AuthManager. PART 34 (regular users) is not
// implemented for this project (see IDEA.md).
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.renderLoginPage(w, r, "", "")
	case http.MethodPost:
		s.processLogin(w, r)
	default:
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
	}
}

func (s *Server) renderLoginPage(w http.ResponseWriter, r *http.Request, errorMsg, successMsg string) {
	baseData := s.newPageData(w, r, "Login", "auth/login")
	baseData.Description = "Sign in to your account"
	baseData.CSRFToken = s.getCSRFToken(r)
	data := &AuthPageData{
		PageData: *baseData,
		Error:    errorMsg,
		Success:  successMsg,
		Redirect: r.URL.Query().Get("redirect"),
	}

	if err := s.renderer.Render(w, "auth/login", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

func (s *Server) processLogin(w http.ResponseWriter, r *http.Request) {
	// Per AI.md PART 11: Rate limit login attempts (5 per 15 minutes per IP)
	ip := getClientIPSimple(r)
	if !s.loginLimiter.Allow(ip) {
		remaining := s.loginLimiter.RemainingTime(ip)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(remaining.Seconds())))
		s.renderLoginPage(w, r, "Too many login attempts. Please try again later.", "")
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderLoginPage(w, r, "Invalid form data", "")
		return
	}

	if !s.csrf.ValidateToken(r) {
		s.renderLoginPage(w, r, "Invalid request. Please try again.", "")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	if username == "" || password == "" {
		s.renderLoginPage(w, r, "Username and password are required", "")
		return
	}

	if s.adminHandler == nil || s.adminHandler.AuthManager() == nil {
		s.renderLoginPage(w, r, "Login is not available", "")
		return
	}

	if !s.adminHandler.AuthManager().Authenticate(username, password) {
		// Per AI.md PART 11: Failed login does NOT reveal whether the username exists.
		s.renderLoginPage(w, r, "Invalid username or password", "")
		return
	}

	session := s.adminHandler.AuthManager().CreateSession(username, ip, r.UserAgent())
	s.adminHandler.AuthManager().SetSessionCookie(w, session)

	// Per AI.md PART 11: Admin login ALWAYS redirects to /{admin_path}.
	http.Redirect(w, r, "/"+config.GetAdminPath(), http.StatusSeeOther)
}

// handleLogout clears the admin session and returns the user to the login page.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if s.adminHandler != nil {
		am := s.adminHandler.AuthManager()
		if am != nil {
			if session, ok := am.GetSessionFromRequest(r); ok {
				am.DeleteSession(session.ID)
			}
			am.ClearSessionCookie(w)
		}
	}
	http.Redirect(w, r, "/auth/login?logout=1", http.StatusSeeOther)
}

// getClientIPSimple extracts the client IP address from a request.
// Used by processLogin and src/server/alerts.go.
func getClientIPSimple(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}
