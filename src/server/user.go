package server

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/users"
)

// UserPageData represents data for user pages
type UserPageData struct {
	PageData
	User           *users.User
	Error          string
	Success        string
	Sessions       []SessionDisplay
	Tokens         []TokenDisplay
	TwoFAEnabled   bool
	TwoFASetup     *users.TOTPSetupResponse
	RecoveryKeys   []string
	RecoveryStats  *users.RecoveryKeyStats
	CurrentSession int64
}

// SessionDisplay represents session info for display
type SessionDisplay struct {
	ID         int64
	DeviceName string
	IPAddress  string
	CreatedAt  string
	LastUsed   string
	IsCurrent  bool
}

// TokenDisplay represents token info for display
type TokenDisplay struct {
	ID          int64
	Name        string
	Prefix      string
	Permissions []string
	LastUsed    string
	ExpiresAt   string
	Expired     bool
}

// handleUserProfile renders the user profile page
func (s *Server) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUserAuth(r)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/user/profile", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderProfilePage(w, r, user, "", "")
	case http.MethodPost:
		s.processProfileUpdate(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) renderProfilePage(w http.ResponseWriter, r *http.Request, user *users.User, errorMsg, successMsg string) {
	data := &UserPageData{
		PageData: PageData{
			Title:       "Profile",
			Description: "Manage your profile",
			Page:        "user/profile",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		User:    user,
		Error:   errorMsg,
		Success: successMsg,
	}

	if err := s.renderer.Render(w, "user/profile", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
	}
}

func (s *Server) processProfileUpdate(w http.ResponseWriter, r *http.Request, user *users.User) {
	if err := r.ParseForm(); err != nil {
		s.renderProfilePage(w, r, user, "Invalid form data", "")
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.renderProfilePage(w, r, user, "Invalid request. Please try again.", "")
		return
	}

	displayName := strings.TrimSpace(r.FormValue("display_name"))
	bio := strings.TrimSpace(r.FormValue("bio"))
	avatarURL := strings.TrimSpace(r.FormValue("avatar_url"))

	// Update profile
	err := s.userAuthManager.UpdateProfile(r.Context(), user.ID, displayName, bio, avatarURL)
	if err != nil {
		s.renderProfilePage(w, r, user, "Failed to update profile", "")
		return
	}

	// Re-fetch user to show updated data
	user, _ = s.userAuthManager.GetUserByID(r.Context(), user.ID)
	s.renderProfilePage(w, r, user, "", "Profile updated successfully")
}

// handleUserSecurity renders the security settings page
func (s *Server) handleUserSecurity(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUserAuth(r)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/user/security", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderSecurityPage(w, r, user, "", "")
	case http.MethodPost:
		s.processSecurityUpdate(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) renderSecurityPage(w http.ResponseWriter, r *http.Request, user *users.User, errorMsg, successMsg string) {
	// Get current session token
	currentToken := s.userAuthManager.GetSessionToken(r)

	// Get user sessions
	sessions, _ := s.userAuthManager.GetUserSessions(r.Context(), user.ID)
	sessionDisplays := make([]SessionDisplay, len(sessions))
	var currentSessionID int64
	for i, sess := range sessions {
		isCurrent := sess.Token == currentToken
		if isCurrent {
			currentSessionID = sess.ID
		}
		sessionDisplays[i] = SessionDisplay{
			ID:         sess.ID,
			DeviceName: sess.DeviceName,
			IPAddress:  sess.IPAddress,
			CreatedAt:  formatTimeAgo(sess.CreatedAt),
			LastUsed:   formatTimeAgo(sess.LastUsed),
			IsCurrent:  isCurrent,
		}
	}

	// Check 2FA status
	var twoFAEnabled bool
	if s.totpManager != nil {
		twoFAEnabled = s.totpManager.Is2FAEnabled(r.Context(), user.ID)
	}

	// Get recovery key stats
	var recoveryStats *users.RecoveryKeyStats
	if s.recoveryManager != nil {
		recoveryStats, _ = s.recoveryManager.GetUsageStats(r.Context(), user.ID)
	}

	data := &UserPageData{
		PageData: PageData{
			Title:       "Security",
			Description: "Manage your security settings",
			Page:        "user/security",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		User:           user,
		Error:          errorMsg,
		Success:        successMsg,
		Sessions:       sessionDisplays,
		TwoFAEnabled:   twoFAEnabled,
		RecoveryStats:  recoveryStats,
		CurrentSession: currentSessionID,
	}

	if err := s.renderer.Render(w, "user/security", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
	}
}

func (s *Server) processSecurityUpdate(w http.ResponseWriter, r *http.Request, user *users.User) {
	if err := r.ParseForm(); err != nil {
		s.renderSecurityPage(w, r, user, "Invalid form data", "")
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.renderSecurityPage(w, r, user, "Invalid request. Please try again.", "")
		return
	}

	action := r.FormValue("action")

	switch action {
	case "change_password":
		s.processPasswordChange(w, r, user)
	case "revoke_session":
		s.processSessionRevoke(w, r, user)
	case "revoke_all_sessions":
		s.processRevokeAllSessions(w, r, user)
	default:
		s.renderSecurityPage(w, r, user, "Invalid action", "")
	}
}

func (s *Server) processPasswordChange(w http.ResponseWriter, r *http.Request, user *users.User) {
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if currentPassword == "" || newPassword == "" || confirmPassword == "" {
		s.renderSecurityPage(w, r, user, "All password fields are required", "")
		return
	}

	if newPassword != confirmPassword {
		s.renderSecurityPage(w, r, user, "New passwords do not match", "")
		return
	}

	// Verify current password
	if !users.CheckPassword(currentPassword, user.PasswordHash) {
		s.renderSecurityPage(w, r, user, "Current password is incorrect", "")
		return
	}

	// Update password
	err := s.userAuthManager.UpdatePassword(r.Context(), user.ID, newPassword, s.config.Server.Users.Auth.PasswordMinLength)
	if err != nil {
		switch err {
		case users.ErrPasswordTooShort:
			s.renderSecurityPage(w, r, user, "Password must be at least 8 characters", "")
		case users.ErrPasswordTooWeak:
			s.renderSecurityPage(w, r, user, "Password must contain at least one uppercase letter, one lowercase letter, and one number", "")
		default:
			s.renderSecurityPage(w, r, user, "Failed to update password", "")
		}
		return
	}

	s.renderSecurityPage(w, r, user, "", "Password updated successfully")
}

func (s *Server) processSessionRevoke(w http.ResponseWriter, r *http.Request, user *users.User) {
	sessionIDStr := r.FormValue("session_id")
	sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
	if err != nil {
		s.renderSecurityPage(w, r, user, "Invalid session ID", "")
		return
	}

	err = s.userAuthManager.RevokeSession(r.Context(), user.ID, sessionID)
	if err != nil {
		s.renderSecurityPage(w, r, user, "Failed to revoke session", "")
		return
	}

	s.renderSecurityPage(w, r, user, "", "Session revoked successfully")
}

func (s *Server) processRevokeAllSessions(w http.ResponseWriter, r *http.Request, user *users.User) {
	currentToken := s.userAuthManager.GetSessionToken(r)
	err := s.userAuthManager.LogoutAll(r.Context(), user.ID, currentToken)
	if err != nil {
		s.renderSecurityPage(w, r, user, "Failed to revoke sessions", "")
		return
	}

	s.renderSecurityPage(w, r, user, "", "All other sessions have been revoked")
}

// handleUserTokens renders the API tokens page
func (s *Server) handleUserTokens(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUserAuth(r)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/user/tokens", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.renderTokensPage(w, r, user, "", "", nil)
	case http.MethodPost:
		s.processTokenAction(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) renderTokensPage(w http.ResponseWriter, r *http.Request, user *users.User, errorMsg, successMsg string, newToken *users.CreateTokenResponse) {
	// Get user tokens
	var tokenDisplays []TokenDisplay
	if s.tokenManager != nil {
		tokens, _ := s.tokenManager.List(r.Context(), user.ID)
		tokenDisplays = make([]TokenDisplay, len(tokens))
		for i, t := range tokens {
			lastUsed := "Never"
			if t.LastUsed != nil {
				lastUsed = formatTimeAgo(*t.LastUsed)
			}
			expires := "Never"
			if t.ExpiresAt != nil {
				expires = t.ExpiresAt.Format("Jan 2, 2006")
			}
			tokenDisplays[i] = TokenDisplay{
				ID:          t.ID,
				Name:        t.Name,
				Prefix:      t.TokenPrefix,
				Permissions: t.GetPermissions(),
				LastUsed:    lastUsed,
				ExpiresAt:   expires,
				Expired:     t.IsExpired(),
			}
		}
	}

	data := &UserPageData{
		PageData: PageData{
			Title:       "API Tokens",
			Description: "Manage your API tokens",
			Page:        "user/tokens",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		User:    user,
		Error:   errorMsg,
		Success: successMsg,
		Tokens:  tokenDisplays,
	}

	// Add new token to template data if just created
	if newToken != nil {
		data.PageData.Extra = map[string]interface{}{
			"NewToken": newToken.Token,
		}
	}

	if err := s.renderer.Render(w, "user/tokens", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
	}
}

func (s *Server) processTokenAction(w http.ResponseWriter, r *http.Request, user *users.User) {
	if err := r.ParseForm(); err != nil {
		s.renderTokensPage(w, r, user, "Invalid form data", "", nil)
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.renderTokensPage(w, r, user, "Invalid request. Please try again.", "", nil)
		return
	}

	action := r.FormValue("action")

	switch action {
	case "create":
		s.processTokenCreate(w, r, user)
	case "revoke":
		s.processTokenRevoke(w, r, user)
	default:
		s.renderTokensPage(w, r, user, "Invalid action", "", nil)
	}
}

func (s *Server) processTokenCreate(w http.ResponseWriter, r *http.Request, user *users.User) {
	if s.tokenManager == nil {
		s.renderTokensPage(w, r, user, "Token management is not available", "", nil)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		s.renderTokensPage(w, r, user, "Token name is required", "", nil)
		return
	}

	// Get permissions
	permissions := r.Form["permissions"]

	// Get expiration
	expiresStr := r.FormValue("expires")
	var expiresIn time.Duration
	if expiresStr != "" {
		days, err := strconv.Atoi(expiresStr)
		if err == nil && days > 0 {
			expiresIn = time.Duration(days) * 24 * time.Hour
		}
	}

	req := users.CreateTokenRequest{
		Name:        name,
		Permissions: permissions,
		ExpiresIn:   expiresIn,
	}

	result, err := s.tokenManager.Create(r.Context(), user.ID, req)
	if err != nil {
		s.renderTokensPage(w, r, user, "Failed to create token", "", nil)
		return
	}

	s.renderTokensPage(w, r, user, "", "Token created successfully. Copy it now - it won't be shown again.", result)
}

func (s *Server) processTokenRevoke(w http.ResponseWriter, r *http.Request, user *users.User) {
	if s.tokenManager == nil {
		s.renderTokensPage(w, r, user, "Token management is not available", "", nil)
		return
	}

	tokenIDStr := r.FormValue("token_id")
	tokenID, err := strconv.ParseInt(tokenIDStr, 10, 64)
	if err != nil {
		s.renderTokensPage(w, r, user, "Invalid token ID", "", nil)
		return
	}

	err = s.tokenManager.Revoke(r.Context(), user.ID, tokenID)
	if err != nil {
		s.renderTokensPage(w, r, user, "Failed to revoke token", "", nil)
		return
	}

	s.renderTokensPage(w, r, user, "", "Token revoked successfully", nil)
}

// handle2FASetup handles 2FA setup page
func (s *Server) handle2FASetup(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUserAuth(r)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/user/security", http.StatusSeeOther)
		return
	}

	if s.totpManager == nil {
		s.renderSecurityPage(w, r, user, "Two-factor authentication is not available", "")
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.render2FASetupPage(w, r, user, "")
	case http.MethodPost:
		s.process2FASetup(w, r, user)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) render2FASetupPage(w http.ResponseWriter, r *http.Request, user *users.User, errorMsg string) {
	// Check if already enabled
	if s.totpManager.Is2FAEnabled(r.Context(), user.ID) {
		http.Redirect(w, r, "/user/security", http.StatusSeeOther)
		return
	}

	data := &UserPageData{
		PageData: PageData{
			Title:       "Setup Two-Factor Authentication",
			Description: "Enable 2FA for your account",
			Page:        "user/2fa-setup",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
		},
		User:  user,
		Error: errorMsg,
	}

	if err := s.renderer.Render(w, "user/2fa-setup", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
	}
}

func (s *Server) process2FASetup(w http.ResponseWriter, r *http.Request, user *users.User) {
	if err := r.ParseForm(); err != nil {
		s.render2FASetupPage(w, r, user, "Invalid form data")
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.render2FASetupPage(w, r, user, "Invalid request. Please try again.")
		return
	}

	step := r.FormValue("step")

	switch step {
	case "init":
		// Verify password and generate QR code
		password := r.FormValue("password")
		if !users.CheckPassword(password, user.PasswordHash) {
			s.render2FASetupPage(w, r, user, "Invalid password")
			return
		}

		setup, err := s.totpManager.Setup(r.Context(), user)
		if err != nil {
			s.render2FASetupPage(w, r, user, "Failed to setup 2FA")
			return
		}

		// Render setup page with QR code
		data := &UserPageData{
			PageData: PageData{
				Title:       "Setup Two-Factor Authentication",
				Description: "Scan the QR code",
				Page:        "user/2fa-setup",
				Theme:       "dark",
				Config:      s.config,
				CSRFToken:   s.getCSRFToken(r),
			},
			User:       user,
			TwoFASetup: setup,
		}

		if err := s.renderer.Render(w, "user/2fa-setup", data); err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
		}

	case "verify":
		// Verify the code and enable 2FA
		code := strings.TrimSpace(r.FormValue("code"))
		if code == "" {
			s.render2FASetupPage(w, r, user, "Please enter the verification code")
			return
		}

		err := s.totpManager.VerifySetup(r.Context(), user.ID, code)
		if err != nil {
			s.render2FASetupPage(w, r, user, "Invalid verification code")
			return
		}

		// Generate recovery keys
		var recoveryKeys []string
		if s.recoveryManager != nil {
			recoveryKeys, _ = s.recoveryManager.Generate(r.Context(), user.ID)
		}

		// Show recovery keys
		data := &UserPageData{
			PageData: PageData{
				Title:       "Save Your Recovery Keys",
				Description: "Store these keys safely",
				Page:        "user/recovery-keys",
				Theme:       "dark",
				Config:      s.config,
				CSRFToken:   s.getCSRFToken(r),
			},
			User:         user,
			RecoveryKeys: recoveryKeys,
			Success:      "Two-factor authentication has been enabled",
		}

		if err := s.renderer.Render(w, "user/recovery-keys", data); err != nil {
			s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
		}

	default:
		s.render2FASetupPage(w, r, user, "Invalid step")
	}
}

// handle2FADisable handles 2FA disabling
func (s *Server) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	user, err := s.requireUserAuth(r)
	if err != nil {
		http.Redirect(w, r, "/auth/login?redirect=/user/security", http.StatusSeeOther)
		return
	}

	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/user/security", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		s.renderSecurityPage(w, r, user, "Invalid form data", "")
		return
	}

	// Verify CSRF token
	if !s.csrf.ValidateToken(r) {
		s.renderSecurityPage(w, r, user, "Invalid request. Please try again.", "")
		return
	}

	password := r.FormValue("password")
	code := strings.TrimSpace(r.FormValue("code"))

	// Verify password
	if !users.CheckPassword(password, user.PasswordHash) {
		s.renderSecurityPage(w, r, user, "Invalid password", "")
		return
	}

	// Verify 2FA code
	if s.totpManager != nil {
		if err := s.totpManager.Verify(r.Context(), user.ID, code); err != nil {
			s.renderSecurityPage(w, r, user, "Invalid 2FA code", "")
			return
		}

		// Disable 2FA
		if err := s.totpManager.Disable(r.Context(), user.ID); err != nil {
			s.renderSecurityPage(w, r, user, "Failed to disable 2FA", "")
			return
		}
	}

	// Revoke recovery keys
	if s.recoveryManager != nil {
		_ = s.recoveryManager.RevokeAll(r.Context(), user.ID)
	}

	s.renderSecurityPage(w, r, user, "", "Two-factor authentication has been disabled")
}

// requireUserAuth validates user session and returns the user
func (s *Server) requireUserAuth(r *http.Request) (*users.User, error) {
	if s.userAuthManager == nil {
		return nil, users.ErrSessionNotFound
	}

	token := s.userAuthManager.GetSessionToken(r)
	if token == "" {
		return nil, users.ErrSessionNotFound
	}

	user, _, err := s.userAuthManager.ValidateSession(r.Context(), token)
	return user, err
}

// formatTimeAgo formats a time as relative (e.g., "2 hours ago")
func formatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return strconv.Itoa(mins) + " minutes ago"
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return strconv.Itoa(hours) + " hours ago"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return strconv.Itoa(days) + " days ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}
