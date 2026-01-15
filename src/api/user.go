package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/model"
	userpkg "github.com/apimgr/search/src/user"
)

// UserHandler handles user API requests
type UserHandler struct {
	config          *config.Config
	authManager     *userpkg.AuthManager
	totpManager     *userpkg.TOTPManager
	recoveryManager *userpkg.RecoveryManager
	tokenManager    *userpkg.TokenManager
	db              *sql.DB
}

// NewUserHandler creates a new user API handler
func NewUserHandler(cfg *config.Config, db *sql.DB, authManager *userpkg.AuthManager, totpManager *userpkg.TOTPManager, recoveryManager *userpkg.RecoveryManager, tokenManager *userpkg.TokenManager) *UserHandler {
	return &UserHandler{
		config:          cfg,
		db:              db,
		authManager:     authManager,
		totpManager:     totpManager,
		recoveryManager: recoveryManager,
		tokenManager:    tokenManager,
	}
}

// RegisterRoutes registers user API routes
// Per AI.md PART 14: All resource names MUST be plural
func (h *UserHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/users/profile", h.handleProfile)
	mux.HandleFunc("/api/v1/users/password", h.handlePassword)
	mux.HandleFunc("/api/v1/users/sessions", h.handleSessions)
	mux.HandleFunc("/api/v1/users/sessions/", h.handleSessionByID)
	mux.HandleFunc("/api/v1/users/tokens", h.handleTokens)
	mux.HandleFunc("/api/v1/users/tokens/", h.handleTokenByID)
	mux.HandleFunc("/api/v1/users/2fa/status", h.handle2FAStatus)
	mux.HandleFunc("/api/v1/users/2fa/setup", h.handle2FASetup)
	mux.HandleFunc("/api/v1/users/2fa/enable", h.handle2FAEnable)
	mux.HandleFunc("/api/v1/users/2fa/disable", h.handle2FADisable)
	mux.HandleFunc("/api/v1/users/recovery-keys", h.handleRecoveryKeys)
}

// Request/Response types

// UpdateProfileRequest represents a profile update request
type UpdateProfileRequest struct {
	DisplayName       string `json:"display_name"`
	Bio               string `json:"bio"`
	AvatarURL         string `json:"avatar_url"`
	NotificationEmail string `json:"notification_email,omitempty"`
}

// ChangePasswordRequest represents a password change request
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// CreateTokenRequest represents a token creation request
type CreateTokenRequest struct {
	Name        string   `json:"name"`
	Permissions []string `json:"permissions"`
	ExpiresIn   int      `json:"expires_in_days"`
}

// Setup2FARequest represents a 2FA setup request
type Setup2FARequest struct {
	Password string `json:"password"`
}

// Enable2FARequest represents a 2FA enable request
type Enable2FARequest struct {
	Code string `json:"code"`
}

// Disable2FARequest represents a 2FA disable request
type Disable2FARequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

// SessionInfo represents session information for API
type SessionInfo struct {
	ID         int64     `json:"id"`
	DeviceName string    `json:"device_name"`
	IPAddress  string    `json:"ip_address"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsed   time.Time `json:"last_used"`
	ExpiresAt  time.Time `json:"expires_at"`
	IsCurrent  bool      `json:"is_current"`
}

// Handler implementations

func (h *UserHandler) handleProfile(w http.ResponseWriter, r *http.Request) {
	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getProfile(w, r, user)
	case http.MethodPut, http.MethodPatch:
		h.updateProfile(w, r, user)
	default:
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use GET or PUT/PATCH")
	}
}

func (h *UserHandler) getProfile(w http.ResponseWriter, r *http.Request, user *userpkg.User) {
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
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
			"email_info": user.GetEmailInfo(),
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) updateProfile(w http.ResponseWriter, r *http.Request, user *userpkg.User) {
	var req UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Update profile fields
	err := h.authManager.UpdateProfile(r.Context(), user.ID, req.DisplayName, req.Bio, req.AvatarURL)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to update profile", "")
		return
	}

	// Handle notification email update if provided
	// Notification email is a separate email for notifications, distinct from primary account email
	// Verification of notification emails is handled separately via user preferences
	if req.NotificationEmail != "" && req.NotificationEmail != user.NotificationEmail {
		if err := user.SetNotificationEmail(req.NotificationEmail); err != nil {
			h.errorResponse(w, http.StatusBadRequest, "Invalid notification email", err.Error())
			return
		}
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    map[string]string{"message": "Profile updated successfully"},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handlePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use PUT or POST")
		return
	}

	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		h.errorResponse(w, http.StatusBadRequest, "Current and new passwords are required", "")
		return
	}

	// Verify current password
	if !userpkg.CheckPassword(req.CurrentPassword, user.PasswordHash) {
		h.errorResponse(w, http.StatusUnauthorized, "Current password is incorrect", "")
		return
	}

	// Update password
	err = h.authManager.UpdatePassword(r.Context(), user.ID, req.NewPassword, h.config.Server.Users.Auth.PasswordMinLength)
	if err != nil {
		switch err {
		case userpkg.ErrPasswordTooShort, userpkg.ErrPasswordTooWeak, userpkg.ErrPasswordWhitespace:
			h.errorResponse(w, http.StatusBadRequest, err.Error(), "")
		default:
			h.errorResponse(w, http.StatusInternalServerError, "Failed to update password", "")
		}
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    map[string]string{"message": "Password updated successfully"},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handleSessions(w http.ResponseWriter, r *http.Request) {
	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	currentToken := h.authManager.GetSessionToken(r)

	switch r.Method {
	case http.MethodGet:
		// List all sessions
		sessions, err := h.authManager.GetUserSessions(r.Context(), user.ID)
		if err != nil {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to get sessions", "")
			return
		}

		sessionInfos := make([]SessionInfo, len(sessions))
		for i, s := range sessions {
			sessionInfos[i] = SessionInfo{
				ID:         s.ID,
				DeviceName: s.DeviceName,
				IPAddress:  s.IPAddress,
				CreatedAt:  s.CreatedAt,
				LastUsed:   s.LastUsed,
				ExpiresAt:  s.ExpiresAt,
				IsCurrent:  s.Token == currentToken,
			}
		}

		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data:    map[string]interface{}{"sessions": sessionInfos},
			Meta:    &APIMeta{Version: APIVersion},
		})

	case http.MethodDelete:
		// Revoke all other sessions
		err := h.authManager.LogoutAll(r.Context(), user.ID, currentToken)
		if err != nil {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to revoke sessions", "")
			return
		}

		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data:    map[string]string{"message": "All other sessions have been revoked"},
			Meta:    &APIMeta{Version: APIVersion},
		})

	default:
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use GET or DELETE")
	}
}

func (h *UserHandler) handleSessionByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use DELETE")
		return
	}

	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	// Extract session ID from path
	path := r.URL.Path
	idStr := strings.TrimPrefix(path, "/api/v1/users/sessions/")
	idStr = strings.TrimSuffix(idStr, "/")
	sessionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid session ID", "")
		return
	}

	// Revoke the session
	err = h.authManager.RevokeSession(r.Context(), user.ID, sessionID)
	if err != nil {
		if err == userpkg.ErrSessionNotFound {
			h.errorResponse(w, http.StatusNotFound, "Session not found", "")
		} else {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to revoke session", "")
		}
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    map[string]string{"message": "Session revoked successfully"},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handleTokens(w http.ResponseWriter, r *http.Request) {
	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	if h.tokenManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "Token management not available", "")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// List all tokens
		tokens, err := h.tokenManager.List(r.Context(), user.ID)
		if err != nil {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to get tokens", "")
			return
		}

		tokenInfos := make([]userpkg.TokenInfo, len(tokens))
		for i, t := range tokens {
			tokenInfos[i] = t.ToInfo()
		}

		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data:    map[string]interface{}{"tokens": tokenInfos},
			Meta:    &APIMeta{Version: APIVersion},
		})

	case http.MethodPost:
		// Create new token
		var req CreateTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
			return
		}

		if req.Name == "" {
			h.errorResponse(w, http.StatusBadRequest, "Token name is required", "")
			return
		}

		var expiresIn time.Duration
		if req.ExpiresIn > 0 {
			expiresIn = time.Duration(req.ExpiresIn) * 24 * time.Hour
		}

		createReq := userpkg.CreateTokenRequest{
			Name:        req.Name,
			Permissions: req.Permissions,
			ExpiresIn:   expiresIn,
		}

		result, err := h.tokenManager.Create(r.Context(), user.ID, createReq)
		if err != nil {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to create token", "")
			return
		}

		h.jsonResponse(w, http.StatusCreated, &APIResponse{
			OK: true,
			Data: map[string]interface{}{
				"token":   result.Token,
				"id":      result.ID,
				"name":    result.Name,
				"prefix":  result.Prefix,
				"expires": result.Expiry,
				"message": "Token created. Save this token now - it won't be shown again.",
			},
			Meta: &APIMeta{Version: APIVersion},
		})

	default:
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use GET or POST")
	}
}

func (h *UserHandler) handleTokenByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use DELETE")
		return
	}

	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	if h.tokenManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "Token management not available", "")
		return
	}

	// Extract token ID from path
	path := r.URL.Path
	idStr := strings.TrimPrefix(path, "/api/v1/users/tokens/")
	idStr = strings.TrimSuffix(idStr, "/")
	tokenID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid token ID", "")
		return
	}

	// Revoke the token
	err = h.tokenManager.Revoke(r.Context(), user.ID, tokenID)
	if err != nil {
		if err == userpkg.ErrTokenNotFound {
			h.errorResponse(w, http.StatusNotFound, "Token not found", "")
		} else {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to revoke token", "")
		}
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    map[string]string{"message": "Token revoked successfully"},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handle2FAStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use GET")
		return
	}

	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	if h.totpManager == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data:    map[string]interface{}{"enabled": false, "available": false},
			Meta:    &APIMeta{Version: APIVersion},
		})
		return
	}

	status := h.totpManager.GetStatus(r.Context(), user.ID)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"enabled":    status.Enabled,
			"verified":   status.Verified,
			"enabled_at": status.EnabledAt,
			"available":  true,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handle2FASetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	if h.totpManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "2FA not available", "")
		return
	}

	var req Setup2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Verify password before allowing 2FA setup
	if !userpkg.CheckPassword(req.Password, user.PasswordHash) {
		h.errorResponse(w, http.StatusUnauthorized, "Invalid password", "")
		return
	}

	// Setup 2FA
	result, err := h.totpManager.Setup(r.Context(), user)
	if err != nil {
		if err == userpkg.ErrTOTPAlreadySetup {
			h.errorResponse(w, http.StatusConflict, "2FA is already set up", "")
		} else {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to setup 2FA", "")
		}
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"secret":      result.Secret,
			"qr_code_url": result.QRCodeURL,
			"issuer":      result.Issuer,
			"account":     result.Account,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handle2FAEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	if h.totpManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "2FA not available", "")
		return
	}

	var req Enable2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	if req.Code == "" {
		h.errorResponse(w, http.StatusBadRequest, "Verification code is required", "")
		return
	}

	// Verify and enable 2FA
	err = h.totpManager.VerifySetup(r.Context(), user.ID, req.Code)
	if err != nil {
		if err == userpkg.ErrTOTPInvalidCode {
			h.errorResponse(w, http.StatusBadRequest, "Invalid verification code", "")
		} else if err == userpkg.ErrTOTPAlreadySetup {
			h.errorResponse(w, http.StatusConflict, "2FA is already enabled", "")
		} else {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to enable 2FA", "")
		}
		return
	}

	// Generate recovery keys
	var recoveryKeys []string
	if h.recoveryManager != nil {
		recoveryKeys, err = h.recoveryManager.Generate(r.Context(), user.ID)
		if err != nil {
			// Log error but don't fail - 2FA is still enabled
			recoveryKeys = nil
		}
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"message":       "2FA has been enabled",
			"recovery_keys": recoveryKeys,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use POST")
		return
	}

	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	if h.totpManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "2FA not available", "")
		return
	}

	var req Disable2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
		return
	}

	// Verify password
	if !userpkg.CheckPassword(req.Password, user.PasswordHash) {
		h.errorResponse(w, http.StatusUnauthorized, "Invalid password", "")
		return
	}

	// Verify TOTP code
	if err := h.totpManager.Verify(r.Context(), user.ID, req.Code); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid 2FA code", "")
		return
	}

	// Disable 2FA
	err = h.totpManager.Disable(r.Context(), user.ID)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to disable 2FA", "")
		return
	}

	// Revoke recovery keys
	if h.recoveryManager != nil {
		_ = h.recoveryManager.RevokeAll(r.Context(), user.ID)
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    map[string]string{"message": "2FA has been disabled"},
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *UserHandler) handleRecoveryKeys(w http.ResponseWriter, r *http.Request) {
	user, err := h.requireAuth(r)
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "Not authenticated", "")
		return
	}

	if h.recoveryManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "Recovery keys not available", "")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Get recovery key stats
		stats, err := h.recoveryManager.GetUsageStats(r.Context(), user.ID)
		if err != nil {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to get recovery key stats", "")
			return
		}

		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: map[string]interface{}{
				"total":     stats.Total,
				"used":      stats.Used,
				"remaining": stats.Remaining,
			},
			Meta: &APIMeta{Version: APIVersion},
		})

	case http.MethodPost:
		// Regenerate recovery keys
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.errorResponse(w, http.StatusBadRequest, "Invalid request body", err.Error())
			return
		}

		// Verify password
		if !userpkg.CheckPassword(req.Password, user.PasswordHash) {
			h.errorResponse(w, http.StatusUnauthorized, "Invalid password", "")
			return
		}

		// Generate new recovery keys
		keys, err := h.recoveryManager.Generate(r.Context(), user.ID)
		if err != nil {
			h.errorResponse(w, http.StatusInternalServerError, "Failed to generate recovery keys", "")
			return
		}

		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: map[string]interface{}{
				"recovery_keys": keys,
				"message":       "New recovery keys generated. Save these keys - they won't be shown again.",
			},
			Meta: &APIMeta{Version: APIVersion},
		})

	default:
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Use GET or POST")
	}
}

// Helper methods

func (h *UserHandler) requireAuth(r *http.Request) (*userpkg.User, error) {
	token := h.authManager.GetSessionToken(r)
	if token == "" {
		return nil, userpkg.ErrSessionNotFound
	}

	user, _, err := h.authManager.ValidateSession(r.Context(), token)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (h *UserHandler) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-API-Version", APIVersion)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// errorResponse sends a unified error response per AI.md PART 16
func (h *UserHandler) errorResponse(w http.ResponseWriter, status int, message, _ string) {
	h.jsonResponse(w, status, &APIResponse{
		OK:      false,
		Error:   model.ErrorCodeFromHTTP(status),
		Message: message,
	})
}
