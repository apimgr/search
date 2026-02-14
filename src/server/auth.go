package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/config"
	emailpkg "github.com/apimgr/search/src/email"
	userpkg "github.com/apimgr/search/src/user"
)

// AuthPageData represents data for auth pages
type AuthPageData struct {
	PageData
	Error        string
	Success      string
	Username     string
	Email        string
	SSOProviders []SSOProvider
	RequireEmail bool
}

// SSOProvider represents a single sign-on provider
type SSOProvider struct {
	Name    string
	ID      string
	IconURL string
	URL     string
}

// TwoFactorPageData represents data for 2FA pages
type TwoFactorPageData struct {
	PageData
	Error           string
	SessionID       string
	RemainingKeys   int
	UseRecoveryKey  bool
}

// handleLogin renders the login page and processes login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	// Check if user management is enabled
	if !s.config.Server.Users.Enabled {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderLoginPage(w, r, "", "")
	case http.MethodPost:
		s.processLogin(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) renderLoginPage(w http.ResponseWriter, r *http.Request, errorMsg, successMsg string) {
	data := &AuthPageData{
		PageData: PageData{
			Title:       "Login",
			Description: "Sign in to your account",
			Page:        "auth/login",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		Error:   errorMsg,
		Success: successMsg,
	}

	// Add SSO providers if configured
	if s.config.Server.Users.SSO.Enabled {
		data.SSOProviders = s.getSSOProviders()
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

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.renderLoginPage(w, r, "Invalid request. Please try again.", "")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	remember := r.FormValue("remember") == "on"

	if username == "" || password == "" {
		s.renderLoginPage(w, r, "Username and password are required", "")
		return
	}

	// Get client info
	ipAddress := getClientIPSimple(r)
	userAgent := r.UserAgent()

	// Per AI.md PART 11: Scoped Login Redirect (NON-NEGOTIABLE)
	// Single login form, scoped redirect based on account type
	// 1. Check admin credentials first
	// 2. If not admin, check user credentials (if multi-user enabled)
	// 3. Redirect based on account type

	// Try admin authentication first
	if s.adminHandler != nil && s.adminHandler.AuthManager() != nil {
		if s.adminHandler.AuthManager().Authenticate(username, password) {
			// Admin login successful - create admin session
			adminSession := s.adminHandler.AuthManager().CreateSession(username, ipAddress, userAgent)
			s.adminHandler.AuthManager().SetSessionCookie(w, adminSession)

			// Per AI.md PART 11: Admin login ALWAYS redirects to /{admin_path}
			// Never to user routes, never to ?redirect= param
			adminPath := "/" + config.GetAdminPath()
			http.Redirect(w, r, adminPath, http.StatusSeeOther)
			return
		}
	}

	// Try user authentication (if multi-user enabled)
	if !s.config.Server.Users.Enabled {
		// Multi-user not enabled and admin auth failed
		s.renderLoginPage(w, r, "Invalid username or password", "")
		return
	}

	// Attempt user login
	user, session, err := s.userAuthManager.Login(r.Context(), username, password, ipAddress, userAgent)
	if err != nil {
		// Per AI.md PART 11: Failed login does NOT reveal if username exists
		// Use generic error message for all failure cases
		switch err {
		case userpkg.ErrInvalidCredentials:
			s.renderLoginPage(w, r, "Invalid username or password", "")
		case userpkg.ErrUserInactive:
			s.renderLoginPage(w, r, "Your account has been deactivated", "")
		default:
			s.renderLoginPage(w, r, "Login failed. Please try again.", "")
		}
		return
	}

	// Check if 2FA is required
	if s.totpManager != nil && s.totpManager.Is2FAEnabled(r.Context(), user.ID) {
		// Redirect to 2FA verification page
		http.Redirect(w, r, "/auth/2fa?session="+session.Token, http.StatusSeeOther)
		return
	}

	// Set session cookie
	s.userAuthManager.SetSessionCookie(w, session.Token)

	// Update remember duration if checked
	if remember {
		// Extend cookie duration (handled by cookie settings)
	}

	// Per AI.md PART 11: User login redirects to /users or ?redirect= param
	// User login NEVER redirects to admin routes
	redirectURL := r.URL.Query().Get("redirect")
	if redirectURL == "" || !strings.HasPrefix(redirectURL, "/") {
		redirectURL = "/users"
	}
	// Security: Prevent user redirect to admin routes
	// Per AI.md PART 17: Admin path is configurable (default: "admin")
	adminPath := "/" + config.GetAdminPath()
	if strings.HasPrefix(redirectURL, adminPath) {
		redirectURL = "/users"
	}
	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// handleLogout processes logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Users.Enabled {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	token := s.userAuthManager.GetSessionToken(r)
	if token != "" {
		_ = s.userAuthManager.Logout(r.Context(), token)
	}

	s.userAuthManager.ClearSessionCookie(w)
	http.Redirect(w, r, "/auth/login?logout=1", http.StatusSeeOther)
}

// handleRegister renders the registration page and processes registration
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Users.Enabled || !s.config.Server.Users.Registration.Enabled {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderRegisterPage(w, r, "", "")
	case http.MethodPost:
		s.processRegister(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) renderRegisterPage(w http.ResponseWriter, r *http.Request, errorMsg, username string) {
	data := &AuthPageData{
		PageData: PageData{
			Title:       "Register",
			Description: "Create a new account",
			Page:        "auth/register",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		Error:        errorMsg,
		Username:     username,
		RequireEmail: s.config.Server.Users.Registration.RequireEmailVerification,
	}

	if err := s.renderer.Render(w, "auth/register", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

func (s *Server) processRegister(w http.ResponseWriter, r *http.Request) {
	// Per AI.md PART 11: Rate limit registration (5 per 1 hour per IP)
	ip := getClientIPSimple(r)
	if !s.registerLimiter.Allow(ip) {
		remaining := s.registerLimiter.RemainingTime(ip)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(remaining.Seconds())))
		s.renderRegisterPage(w, r, "Too many registration attempts. Please try again later.", "")
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderRegisterPage(w, r, "Invalid form data", "")
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.renderRegisterPage(w, r, "Invalid request. Please try again.", "")
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate inputs
	if username == "" || email == "" || password == "" {
		s.renderRegisterPage(w, r, "All fields are required", username)
		return
	}

	if password != confirmPassword {
		s.renderRegisterPage(w, r, "Passwords do not match", username)
		return
	}

	// Check email domain restrictions
	if len(s.config.Server.Users.Registration.AllowedDomains) > 0 {
		emailParts := strings.Split(email, "@")
		if len(emailParts) != 2 {
			s.renderRegisterPage(w, r, "Invalid email address", username)
			return
		}
		domain := strings.ToLower(emailParts[1])
		allowed := false
		for _, d := range s.config.Server.Users.Registration.AllowedDomains {
			if strings.ToLower(d) == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			s.renderRegisterPage(w, r, "Registration is not allowed for this email domain", username)
			return
		}
	}

	// Register user
	_, err := s.userAuthManager.Register(r.Context(), username, email, password, s.config.Server.Users.Auth.PasswordMinLength)
	if err != nil {
		switch err {
		case userpkg.ErrUsernameTaken:
			s.renderRegisterPage(w, r, "This username is already taken", "")
		case userpkg.ErrEmailTaken:
			s.renderRegisterPage(w, r, "This email is already registered", username)
		case userpkg.ErrUsernameReserved:
			s.renderRegisterPage(w, r, "This username is reserved", "")
		case userpkg.ErrUsernameInvalid, userpkg.ErrUsernameTooShort, userpkg.ErrUsernameTooLong:
			s.renderRegisterPage(w, r, "Invalid username. Use only lowercase letters, numbers, underscores, and hyphens (3-32 characters)", "")
		case userpkg.ErrEmailInvalid:
			s.renderRegisterPage(w, r, "Invalid email address", username)
		case userpkg.ErrPasswordTooShort:
			s.renderRegisterPage(w, r, "Password must be at least 8 characters", username)
		case userpkg.ErrPasswordTooWeak:
			s.renderRegisterPage(w, r, "Password must contain at least one uppercase letter, one lowercase letter, and one number", username)
		case userpkg.ErrPasswordWhitespace:
			s.renderRegisterPage(w, r, "Password cannot start or end with whitespace", username)
		default:
			s.renderRegisterPage(w, r, "Registration failed. Please try again.", username)
		}
		return
	}

	// Email verification is triggered via the API endpoint when email service is configured
	// Registration can complete without email verification based on server.user.registration settings

	// Redirect to login with success message
	http.Redirect(w, r, "/auth/login?registered=1", http.StatusSeeOther)
}

// handleForgot renders the forgot password page and processes requests
func (s *Server) handleForgot(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Users.Enabled {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderForgotPage(w, r, "", "")
	case http.MethodPost:
		s.processForgot(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) renderForgotPage(w http.ResponseWriter, r *http.Request, errorMsg, successMsg string) {
	data := &AuthPageData{
		PageData: PageData{
			Title:       "Forgot Password",
			Description: "Reset your password",
			Page:        "auth/forgot",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		Error:   errorMsg,
		Success: successMsg,
	}

	if err := s.renderer.Render(w, "auth/forgot", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

func (s *Server) processForgot(w http.ResponseWriter, r *http.Request) {
	// Per AI.md PART 11: Rate limit password reset (3 per 1 hour per IP)
	ip := getClientIPSimple(r)
	if !s.forgotLimiter.Allow(ip) {
		remaining := s.forgotLimiter.RemainingTime(ip)
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(remaining.Seconds())))
		s.renderForgotPage(w, r, "Too many password reset requests. Please try again later.", "")
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderForgotPage(w, r, "Invalid form data", "")
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.renderForgotPage(w, r, "Invalid request. Please try again.", "")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		s.renderForgotPage(w, r, "Email is required", "")
		return
	}

	// Create password reset token if user exists (silent failure to prevent email enumeration)
	if s.verificationManager != nil && s.mailer != nil && s.mailer.IsEnabled() {
		if user, err := s.userAuthManager.GetUserByEmail(r.Context(), email); err == nil && user != nil {
			if token, err := s.verificationManager.CreatePasswordReset(r.Context(), user.ID); err == nil {
				// Construct base URL from config
				scheme := "http"
				if s.config.Server.SSL.Enabled {
					scheme = "https"
				}
				host := s.config.Server.Address
				if host == "" || host == "0.0.0.0" {
					host = "localhost"
				}
				baseURL := fmt.Sprintf("%s://%s:%d", scheme, host, s.config.Server.Port)

				// Send password reset email with token
				resetURL := fmt.Sprintf("%s/auth/reset?token=%s", baseURL, token)
				msg := emailpkg.NewMessage([]string{user.Email}, "Password Reset Request",
					fmt.Sprintf("Click the following link to reset your password:\n\n%s\n\nThis link expires in 1 hour.\n\nIf you didn't request this, please ignore this email.", resetURL))
				_ = s.mailer.Send(msg) // Silent failure - don't expose whether email was sent
			}
		}
	}

	s.renderForgotPage(w, r, "", "If an account exists with this email, a password reset link has been sent.")
}

// handleVerify handles email verification
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Users.Enabled {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		s.handleError(w, r, http.StatusBadRequest, "Invalid Verification", "No verification token provided.")
		return
	}

	// Validate token and verify email via VerificationManager
	if s.verificationManager == nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "Verification Unavailable", "Email verification is not configured.")
		return
	}

	_, err := s.verificationManager.VerifyEmail(r.Context(), token)
	if err != nil {
		switch err {
		case userpkg.ErrVerificationTokenExpired:
			s.handleError(w, r, http.StatusBadRequest, "Link Expired", "This verification link has expired. Please request a new one.")
		default:
			s.handleError(w, r, http.StatusBadRequest, "Invalid Verification", "This verification link is invalid or has already been used.")
		}
		return
	}

	// Show success page
	data := &AuthPageData{
		PageData: PageData{
			Title:       "Email Verified",
			Description: "Your email has been verified",
			Page:        "auth/verify",
			Theme:       "dark",
			Config:      s.config,
		},
		Success: "Your email has been verified successfully. You can now log in.",
	}

	if err := s.renderer.Render(w, "auth/verify", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handle2FA handles 2FA verification
func (s *Server) handle2FA(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Users.Enabled {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.render2FAPage(w, r, "", false)
	case http.MethodPost:
		s.process2FA(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) render2FAPage(w http.ResponseWriter, r *http.Request, errorMsg string, useRecovery bool) {
	sessionID := r.URL.Query().Get("session")
	if sessionID == "" {
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	data := &TwoFactorPageData{
		PageData: PageData{
			Title:       "Two-Factor Authentication",
			Description: "Enter your authentication code",
			Page:        "auth/2fa",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		Error:          errorMsg,
		SessionID:      sessionID,
		UseRecoveryKey: useRecovery,
	}

	if err := s.renderer.Render(w, "auth/2fa", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

func (s *Server) process2FA(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.render2FAPage(w, r, "Invalid form data", false)
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.render2FAPage(w, r, "Invalid request. Please try again.", false)
		return
	}

	sessionID := r.FormValue("session_id")
	code := strings.TrimSpace(r.FormValue("code"))
	useRecovery := r.FormValue("use_recovery") == "1"

	if sessionID == "" || code == "" {
		s.render2FAPage(w, r, "Please enter your authentication code", false)
		return
	}

	// Validate session
	user, session, err := s.userAuthManager.ValidateSession(r.Context(), sessionID)
	if err != nil {
		http.Redirect(w, r, "/auth/login?expired=1", http.StatusSeeOther)
		return
	}

	if useRecovery {
		// Validate recovery key
		if s.recoveryManager == nil {
			s.render2FAPage(w, r, "Recovery keys are not available", true)
			return
		}

		err = s.recoveryManager.Validate(r.Context(), user.ID, code)
		if err != nil {
			s.render2FAPage(w, r, "Invalid recovery key", true)
			return
		}

		// Disable 2FA after using recovery key
		if s.totpManager != nil {
			_ = s.totpManager.Disable(r.Context(), user.ID)
		}
	} else {
		// Validate TOTP code
		if s.totpManager == nil {
			s.render2FAPage(w, r, "Two-factor authentication is not configured", false)
			return
		}

		err = s.totpManager.Verify(r.Context(), user.ID, code)
		if err != nil {
			s.render2FAPage(w, r, "Invalid authentication code", false)
			return
		}
	}

	// Set session cookie
	s.userAuthManager.SetSessionCookie(w, session.Token)

	// Redirect to home
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleRecoveryLogin handles login with recovery key
func (s *Server) handleRecoveryLogin(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Users.Enabled {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Redirect to 2FA page with recovery flag
	sessionID := r.URL.Query().Get("session")
	http.Redirect(w, r, "/auth/2fa?session="+sessionID+"&recovery=1", http.StatusSeeOther)
}

// getSSOProviders returns configured SSO providers
func (s *Server) getSSOProviders() []SSOProvider {
	var providers []SSOProvider

	// Add OIDC providers
	for id, p := range s.config.Server.Users.SSO.OIDC {
		providers = append(providers, SSOProvider{
			Name:    p.Name,
			ID:      id,
			IconURL: p.IconURL,
			URL:     "/auth/sso/" + id,
		})
	}

	// Add LDAP if configured
	if s.config.Server.Users.SSO.LDAP.Enabled {
		providers = append(providers, SSOProvider{
			Name:    "LDAP",
			ID:      "ldap",
			IconURL: "/static/icons/ldap.svg",
			URL:     "/auth/sso/ldap",
		})
	}

	return providers
}

// getClientIPSimple extracts the client IP address from a request (simple version)
func getClientIPSimple(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}
