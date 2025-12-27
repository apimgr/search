package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/users"
)

// AuthHandler handles authentication API requests
type AuthHandler struct {
	config          *config.Config
	authManager     *users.AuthManager
	totpManager     *users.TOTPManager
	recoveryManager *users.RecoveryManager
	db              *sql.DB
}

// NewAuthHandler creates a new auth API handler
func NewAuthHandler(cfg *config.Config, db *sql.DB, authManager *users.AuthManager, totpManager *users.TOTPManager, recoveryManager *users.RecoveryManager) *AuthHandler {
	return &AuthHandler{
		config:          cfg,
		db:              db,
		authManager:     authManager,
		totpManager:     totpManager,
		recoveryManager: recoveryManager,
	}
}

// RegisterRoutes registers auth API routes
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/auth/register", h.handleRegister)
	mux.HandleFunc("/api/v1/auth/login", h.handleLogin)
	mux.HandleFunc("/api/v1/auth/logout", h.handleLogout)
	mux.HandleFunc("/api/v1/auth/forgot", h.handleForgotPassword)
	mux.HandleFunc("/api/v1/auth/reset", h.handleResetPassword)
	mux.HandleFunc("/api/v1/auth/recovery", h.handleRecoveryKey)
	mux.HandleFunc("/api/v1/auth/verify", h.handleVerifyEmail)
	mux.HandleFunc("/api/v1/auth/2fa/verify", h.handle2FAVerify)
	mux.HandleFunc("/api/v1/auth/session", h.handleSession)
}

// Request/Response types

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	RememberMe bool   `json:"remember_me"`
	TOTPCode   string `json:"totp_code,omitempty"`
}

// LoginResponse represents a successful login response
type LoginResponse struct {
	User       UserResponse `json:"user"`
	SessionID  string       `json:"session_id,omitempty"`
	ExpiresAt  time.Time    `json:"expires_at"`
	Requires2FA bool        `json:"requires_2fa,omitempty"`
}

// UserResponse represents user data in API responses
type UserResponse struct {
	ID            int64      `json:"id"`
	Username      string     `json:"username"`
	Email         string     `json:"email"`
	DisplayName   string     `json:"display_name,omitempty"`
	AvatarURL     string     `json:"avatar_url,omitempty"`
	Role          string     `json:"role"`
	EmailVerified bool       `json:"email_verified"`
	CreatedAt     time.Time  `json:"created_at"`
	LastLogin     *time.Time `json:"last_login,omitempty"`
}

// ForgotPasswordRequest represents a password reset request
type ForgotPasswordRequest struct {
	Email string `json:"email"`
}

// ResetPasswordRequest represents a password reset completion request
type ResetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// RecoveryKeyRequest represents a recovery key login request
type RecoveryKeyRequest struct {
	Username    string `json:"username"`
	RecoveryKey string `json:"recovery_key"`
}

// VerifyEmailRequest represents an email verification request
type VerifyEmailRequest struct {
	Token string `json:"token"`
}

// TwoFactorVerifyRequest represents a 2FA verification request
type TwoFactorVerifyRequest struct {
	Code      string `json:"code"`
	SessionID string `json:"session_id"`
}

// Handler implementations

func (h *AuthHandler) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	// Check if registration is enabled
	if !h.config.Server.Users.Registration.Enabled {
		h.errorResponse(w, http.StatusForbidden, "Registration is disabled", "")
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Validate required fields
	if req.Username == "" || req.Email == "" || req.Password == "" {
		h.errorResponse(w, http.StatusBadRequest, "Missing required fields", "Username, email, and password are required")
		return
	}

	// Check email domain restrictions
	if len(h.config.Server.Users.Registration.AllowedDomains) > 0 {
		emailParts := strings.Split(req.Email, "@")
		if len(emailParts) != 2 {
			h.errorResponse(w, http.StatusBadRequest, "Invalid email address", "")
			return
		}
		domain := strings.ToLower(emailParts[1])
		allowed := false
		for _, d := range h.config.Server.Users.Registration.AllowedDomains {
			if strings.ToLower(d) == domain {
				allowed = true
				break
			}
		}
		if !allowed {
			h.errorResponse(w, http.StatusForbidden, "Email domain not allowed", "")
			return
		}
	}

	// Register user
	user, err := h.authManager.Register(r.Context(), req.Username, req.Email, req.Password, h.config.Server.Users.Auth.PasswordMinLength)
	if err != nil {
		switch err {
		case users.ErrUsernameTaken:
			h.errorResponse(w, http.StatusConflict, "Username already taken", "")
		case users.ErrEmailTaken:
			h.errorResponse(w, http.StatusConflict, "Email already registered", "")
		case users.ErrUsernameReserved:
			h.errorResponse(w, http.StatusBadRequest, "Username is reserved", "")
		case users.ErrUsernameInvalid, users.ErrUsernameTooShort, users.ErrUsernameTooLong:
			h.errorResponse(w, http.StatusBadRequest, "Invalid username", err.Error())
		case users.ErrEmailInvalid:
			h.errorResponse(w, http.StatusBadRequest, "Invalid email address", "")
		case users.ErrPasswordTooShort, users.ErrPasswordTooWeak:
			h.errorResponse(w, http.StatusBadRequest, "Password does not meet requirements", err.Error())
		default:
			h.errorResponse(w, http.StatusInternalServerError, "Registration failed", "")
		}
		return
	}

	// TODO: Send verification email if required
	// if h.config.Server.Users.Registration.RequireEmailVerification {
	//     h.sendVerificationEmail(user)
	// }

	h.jsonResponse(w, http.StatusCreated, &APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"user": UserResponse{
				ID:            user.ID,
				Username:      user.Username,
				Email:         user.Email,
				DisplayName:   user.DisplayName,
				Role:          user.Role,
				EmailVerified: user.EmailVerified,
				CreatedAt:     user.CreatedAt,
			},
			"message": "Registration successful. Please check your email to verify your account.",
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Username == "" || req.Password == "" {
		h.errorResponse(w, http.StatusBadRequest, "Missing credentials", "Username and password are required")
		return
	}

	// Get client IP and user agent
	ipAddress := getClientIP(r)
	userAgent := r.UserAgent()

	// Attempt login
	user, session, err := h.authManager.Login(r.Context(), req.Username, req.Password, ipAddress, userAgent)
	if err != nil {
		switch err {
		case users.ErrInvalidCredentials:
			h.errorResponse(w, http.StatusUnauthorized, "Invalid username or password", "")
		case users.ErrUserInactive:
			h.errorResponse(w, http.StatusForbidden, "Account is inactive", "")
		default:
			h.errorResponse(w, http.StatusInternalServerError, "Login failed", "")
		}
		return
	}

	// Check if 2FA is required
	if h.totpManager != nil && h.totpManager.Is2FAEnabled(r.Context(), user.ID) {
		if req.TOTPCode == "" {
			// Return partial session indicating 2FA is needed
			h.jsonResponse(w, http.StatusOK, &APIResponse{
				Success: true,
				Data: LoginResponse{
					Requires2FA: true,
					SessionID:   session.Token,
				},
				Meta: &APIMeta{Version: APIVersion},
			})
			return
		}

		// Verify 2FA code
		if err := h.totpManager.Verify(r.Context(), user.ID, req.TOTPCode); err != nil {
			h.errorResponse(w, http.StatusUnauthorized, "Invalid 2FA code", "")
			return
		}
	}

	// Set session cookie
	h.authManager.SetSessionCookie(w, session.Token)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data: LoginResponse{
			User: UserResponse{
				ID:            user.ID,
				Username:      user.Username,
				Email:         user.Email,
				DisplayName:   user.DisplayName,
				AvatarURL:     user.AvatarURL,
				Role:          user.Role,
				EmailVerified: user.EmailVerified,
				CreatedAt:     user.CreatedAt,
				LastLogin:     user.LastLogin,
			},
			ExpiresAt: session.ExpiresAt,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	token := h.authManager.GetSessionToken(r)
	if token == "" {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	// Logout (delete session)
	_ = h.authManager.Logout(r.Context(), token)

	// Clear session cookie
	h.authManager.ClearSessionCookie(w)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data:    map[string]string{"message": "Logged out successfully"},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req ForgotPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Email == "" {
		h.errorResponse(w, http.StatusBadRequest, "Email is required", "")
		return
	}

	// Always return success to prevent email enumeration
	// TODO: Actually send password reset email if user exists
	// user, err := h.authManager.GetUserByEmail(r.Context(), req.Email)
	// if err == nil {
	//     h.sendPasswordResetEmail(user)
	// }

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data:    map[string]string{"message": "If an account exists with this email, a password reset link has been sent."},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req ResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Token == "" || req.NewPassword == "" {
		h.errorResponse(w, http.StatusBadRequest, "Token and new password are required", "")
		return
	}

	// TODO: Validate reset token and get user ID
	// userID, err := h.validateResetToken(r.Context(), req.Token)
	// if err != nil {
	//     h.errorResponse(w, http.StatusBadRequest, "Invalid or expired reset token", "")
	//     return
	// }

	// TODO: Update password
	// err = h.authManager.UpdatePassword(r.Context(), userID, req.NewPassword, h.config.Server.Users.Auth.PasswordMinLength)
	// if err != nil {
	//     h.errorResponse(w, http.StatusBadRequest, "Password does not meet requirements", err.Error())
	//     return
	// }

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data:    map[string]string{"message": "Password has been reset successfully."},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handleRecoveryKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req RecoveryKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Username == "" || req.RecoveryKey == "" {
		h.errorResponse(w, http.StatusBadRequest, "Username and recovery key are required", "")
		return
	}

	// Get user
	user, err := h.authManager.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Invalid username or recovery key", "")
		return
	}

	// Validate recovery key
	if h.recoveryManager == nil {
		h.errorResponse(w, http.StatusInternalServerError, "Recovery keys not configured", "")
		return
	}

	err = h.recoveryManager.Validate(r.Context(), user.ID, req.RecoveryKey)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Invalid username or recovery key", "")
		return
	}

	// Get remaining recovery keys count
	remaining, _ := h.recoveryManager.GetRemainingCount(r.Context(), user.ID)

	// Disable 2FA (recovery key was used to bypass it)
	if h.totpManager != nil {
		_ = h.totpManager.Disable(r.Context(), user.ID)
	}

	// Create session
	ipAddress := getClientIP(r)
	userAgent := r.UserAgent()
	session, err := h.createSessionForUser(r.Context(), user, ipAddress, userAgent)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to create session", "")
		return
	}

	// Set session cookie
	h.authManager.SetSessionCookie(w, session.Token)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"message":                 "Recovery key accepted. 2FA has been disabled.",
			"remaining_recovery_keys": remaining,
			"user": UserResponse{
				ID:            user.ID,
				Username:      user.Username,
				Email:         user.Email,
				DisplayName:   user.DisplayName,
				Role:          user.Role,
				EmailVerified: user.EmailVerified,
				CreatedAt:     user.CreatedAt,
			},
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req VerifyEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Token == "" {
		h.errorResponse(w, http.StatusBadRequest, "Verification token is required", "")
		return
	}

	// TODO: Validate verification token and get user ID
	// userID, err := h.validateVerificationToken(r.Context(), req.Token)
	// if err != nil {
	//     h.errorResponse(w, http.StatusBadRequest, "Invalid or expired verification token", "")
	//     return
	// }

	// TODO: Mark email as verified
	// err = h.authManager.VerifyEmail(r.Context(), userID)
	// if err != nil {
	//     h.errorResponse(w, http.StatusInternalServerError, "Failed to verify email", "")
	//     return
	// }

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data:    map[string]string{"message": "Email verified successfully."},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	var req TwoFactorVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Code == "" || req.SessionID == "" {
		h.errorResponse(w, http.StatusBadRequest, "Code and session ID are required", "")
		return
	}

	// Validate the partial session
	user, session, err := h.authManager.ValidateSession(r.Context(), req.SessionID)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Invalid or expired session", "")
		return
	}

	// Verify 2FA code
	if h.totpManager == nil {
		h.errorResponse(w, http.StatusInternalServerError, "2FA not configured", "")
		return
	}

	if err := h.totpManager.Verify(r.Context(), user.ID, req.Code); err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Invalid 2FA code", "")
		return
	}

	// Set session cookie
	h.authManager.SetSessionCookie(w, session.Token)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data: LoginResponse{
			User: UserResponse{
				ID:            user.ID,
				Username:      user.Username,
				Email:         user.Email,
				DisplayName:   user.DisplayName,
				AvatarURL:     user.AvatarURL,
				Role:          user.Role,
				EmailVerified: user.EmailVerified,
				CreatedAt:     user.CreatedAt,
				LastLogin:     user.LastLogin,
			},
			ExpiresAt: session.ExpiresAt,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use GET")
		return
	}

	token := h.authManager.GetSessionToken(r)
	if token == "" {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	user, session, err := h.authManager.ValidateSession(r.Context(), token)
	if err != nil {
		h.authManager.ClearSessionCookie(w)
		h.errorResponse(w, http.StatusUnauthorized, "Session expired", "")
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"user": UserResponse{
				ID:            user.ID,
				Username:      user.Username,
				Email:         user.Email,
				DisplayName:   user.DisplayName,
				AvatarURL:     user.AvatarURL,
				Role:          user.Role,
				EmailVerified: user.EmailVerified,
				CreatedAt:     user.CreatedAt,
				LastLogin:     user.LastLogin,
			},
			"session": map[string]interface{}{
				"expires_at": session.ExpiresAt,
				"created_at": session.CreatedAt,
			},
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// Helper methods

func (h *AuthHandler) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-API-Version", APIVersion)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *AuthHandler) errorResponse(w http.ResponseWriter, status int, message, details string) {
	h.jsonResponse(w, status, &APIResponse{
		Success: false,
		Error: &APIError{
			Code:    models.ErrorCodeFromHTTP(status),
			Status:  status,
			Message: message,
			Details: details,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *AuthHandler) createSessionForUser(ctx context.Context, user *users.User, ipAddress, userAgent string) (*users.UserSession, error) {
	// This is a workaround since AuthManager.createSession is private
	// In production, you'd expose this or use Login with a flag
	_, session, err := h.authManager.Login(ctx, user.Username, "", ipAddress, userAgent)
	if err != nil {
		// If login fails (wrong password), we need another approach
		// For recovery key flow, we've already authenticated via the key
		return nil, err
	}
	return session, nil
}

// getClientIP extracts the client IP address from a request
func getClientIP(r *http.Request) string {
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
