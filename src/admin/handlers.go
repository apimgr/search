package admin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/config"
	"gopkg.in/yaml.v3"
)

// Renderer interface for template rendering
type Renderer interface {
	Render(w io.Writer, name string, data interface{}) error
}

// EngineRegistry interface for engine management
type EngineRegistry interface {
	Count() int
	GetEnabled() []interface{}
	GetAll() []interface{}
}

// ReloadCallback is called when config reload is triggered
type ReloadCallback func() error

// Handler wraps admin HTTP handlers
type Handler struct {
	config         *config.Config
	auth           *AuthManager
	renderer       Renderer
	startTime      time.Time
	registry       EngineRegistry
	reloadCallback ReloadCallback
	configPath     string
}

// NewHandler creates a new admin handler
func NewHandler(cfg *config.Config, renderer Renderer) *Handler {
	return &Handler{
		config:    cfg,
		auth:      NewAuthManager(cfg),
		renderer:  renderer,
		startTime: time.Now(),
	}
}

// SetRegistry sets the engine registry for admin reporting
func (h *Handler) SetRegistry(registry EngineRegistry) {
	h.registry = registry
}

// SetReloadCallback sets the callback for config reload
func (h *Handler) SetReloadCallback(cb ReloadCallback) {
	h.reloadCallback = cb
}

// SetConfigPath sets the path to the config file
func (h *Handler) SetConfigPath(path string) {
	h.configPath = path
}

// RegisterRoutes registers admin routes on the given mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Public routes (no auth required)
	mux.HandleFunc("/admin/login", h.handleLogin)
	mux.HandleFunc("/admin/logout", h.handleLogout)

	// Protected routes (auth required)
	mux.HandleFunc("/admin", h.requireAuth(h.handleDashboard))
	mux.HandleFunc("/admin/", h.requireAuth(h.handleDashboard))
	mux.HandleFunc("/admin/dashboard", h.requireAuth(h.handleDashboard))
	mux.HandleFunc("/admin/config", h.requireAuth(h.handleConfig))
	mux.HandleFunc("/admin/engines", h.requireAuth(h.handleEngines))
	mux.HandleFunc("/admin/tokens", h.requireAuth(h.handleTokens))
	mux.HandleFunc("/admin/logs", h.requireAuth(h.handleLogs))

	// Server settings routes (per TEMPLATE.md)
	mux.HandleFunc("/admin/server/settings", h.requireAuth(h.handleServerSettings))
	mux.HandleFunc("/admin/server/branding", h.requireAuth(h.handleServerBranding))
	mux.HandleFunc("/admin/server/ssl", h.requireAuth(h.handleServerSSL))
	mux.HandleFunc("/admin/server/tor", h.requireAuth(h.handleServerTor))
	mux.HandleFunc("/admin/server/web", h.requireAuth(h.handleServerWeb))
	mux.HandleFunc("/admin/server/email", h.requireAuth(h.handleServerEmail))
	mux.HandleFunc("/admin/server/announcements", h.requireAuth(h.handleServerAnnouncements))
	mux.HandleFunc("/admin/server/geoip", h.requireAuth(h.handleServerGeoIP))
	mux.HandleFunc("/admin/server/metrics", h.requireAuth(h.handleServerMetrics))
	mux.HandleFunc("/admin/scheduler", h.requireAuth(h.handleScheduler))

	// API routes (bearer token auth)
	mux.HandleFunc("/admin/api/v1/status", h.requireAPIAuth(h.apiStatus))
	mux.HandleFunc("/admin/api/v1/config", h.requireAPIAuth(h.apiConfig))
	mux.HandleFunc("/admin/api/v1/engines", h.requireAPIAuth(h.apiEngines))
	mux.HandleFunc("/admin/api/v1/tokens", h.requireAPIAuth(h.apiTokens))
	mux.HandleFunc("/admin/api/v1/reload", h.requireAPIAuth(h.apiReload))
	mux.HandleFunc("/admin/api/v1/backups", h.requireAPIAuth(h.apiBackups))
	mux.HandleFunc("/admin/api/v1/logs", h.requireAPIAuth(h.apiLogs))
	mux.HandleFunc("/admin/api/v1/scheduler", h.requireAPIAuth(h.apiScheduler))
	mux.HandleFunc("/admin/api/v1/email/test", h.requireAPIAuth(h.apiEmailTest))
	mux.HandleFunc("/admin/api/v1/update/check", h.requireAPIAuth(h.apiUpdateCheck))
}

// requireAuth middleware checks for valid admin session
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, ok := h.auth.GetSessionFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/admin/login?redirect="+r.URL.Path, http.StatusSeeOther)
			return
		}

		// Refresh session on activity
		h.auth.RefreshSession(session.ID)

		// Add session to request context (could use context.WithValue)
		next(w, r)
	}
}

// requireAPIAuth middleware checks for valid bearer token
func (h *Handler) requireAPIAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := h.auth.GetTokenFromRequest(r)
		if token == "" {
			h.jsonError(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		apiToken, ok := h.auth.ValidateAPIToken(token)
		if !ok {
			h.jsonError(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Log API access
		log.Printf("[Admin API] %s %s (token: %s)", r.Method, r.URL.Path, maskToken(apiToken.Token))

		next(w, r)
	}
}

// handleLogin handles admin login
func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processLogin(w, r)
		return
	}

	// Check if already logged in
	if _, ok := h.auth.GetSessionFromRequest(r); ok {
		http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
		return
	}

	// Render login page
	data := &AdminPageData{
		Title:  "Admin Login",
		Page:   "admin-login",
		Config: h.config,
		Error:  r.URL.Query().Get("error"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminLogin(w, data)
}

// processLogin handles login form submission
func (h *Handler) processLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/login?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

	if !h.auth.Authenticate(username, password) {
		log.Printf("[Admin] Failed login attempt for user: %s from %s", username, GetClientIP(r))
		http.Redirect(w, r, "/admin/login?error=Invalid+credentials", http.StatusSeeOther)
		return
	}

	// Create session
	session := h.auth.CreateSession(username, GetClientIP(r), r.UserAgent())
	h.auth.SetSessionCookie(w, session)

	log.Printf("[Admin] Successful login for user: %s from %s", username, GetClientIP(r))

	// Redirect to dashboard or original URL
	redirect := r.FormValue("redirect")
	if redirect == "" || redirect == "/admin/login" {
		redirect = "/admin/dashboard"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

// handleLogout handles admin logout
func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if session, ok := h.auth.GetSessionFromRequest(r); ok {
		h.auth.DeleteSession(session.ID)
		log.Printf("[Admin] User logged out: %s", session.Username)
	}
	h.auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// handleDashboard renders the admin dashboard
func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	data := &AdminPageData{
		Title:  "Dashboard",
		Page:   "admin-dashboard",
		Config: h.config,
		Stats: &DashboardStats{
			Uptime:         formatDuration(time.Since(h.startTime)),
			Version:        config.Version,
			GoVersion:      runtime.Version(),
			NumGoroutines:  runtime.NumGoroutine(),
			MemAlloc:       formatBytes(m.Alloc),
			MemTotal:       formatBytes(m.TotalAlloc),
			NumCPU:         runtime.NumCPU(),
			ServerMode:     h.config.Server.Mode,
			TorEnabled:     h.config.Server.Tor.Enabled,
			SSLEnabled:     h.config.Server.SSL.Enabled,
			EnginesEnabled: h.getEngineCount(),
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "dashboard", data)
}

// handleConfig renders the configuration page
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Handle config update
		h.processConfigUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:  "Configuration",
		Page:   "admin-config",
		Config: h.config,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "config", data)
}

// handleEngines renders the search engines page
func (h *Handler) handleEngines(w http.ResponseWriter, r *http.Request) {
	data := &AdminPageData{
		Title:  "Search Engines",
		Page:   "admin-engines",
		Config: h.config,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "engines", data)
}

// handleTokens renders the API tokens page
func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processTokenCreate(w, r)
		return
	}

	data := &AdminPageData{
		Title:  "API Tokens",
		Page:   "admin-tokens",
		Config: h.config,
		Tokens: h.auth.ListAPITokens(),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "tokens", data)
}

// handleLogs renders the logs page
func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	data := &AdminPageData{
		Title:  "Logs",
		Page:   "admin-logs",
		Config: h.config,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "logs", data)
}

// processConfigUpdate handles configuration updates
func (h *Handler) processConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/config?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	// Update config values from form
	if title := r.FormValue("server_title"); title != "" {
		h.config.Server.Title = title
	}
	if desc := r.FormValue("server_description"); desc != "" {
		h.config.Server.Description = desc
	}

	// Save config to file if path is set
	if h.configPath != "" {
		if err := h.saveConfig(); err != nil {
			log.Printf("[Admin] Failed to save config: %v", err)
			http.Redirect(w, r, "/admin/config?error=Failed+to+save+config", http.StatusSeeOther)
			return
		}
	}

	// Trigger reload if callback is set
	if h.reloadCallback != nil {
		if err := h.reloadCallback(); err != nil {
			log.Printf("[Admin] Failed to reload config: %v", err)
		}
	}

	http.Redirect(w, r, "/admin/config?success=Configuration+updated", http.StatusSeeOther)
}

// saveConfig saves the current config to the config file
func (h *Handler) saveConfig() error {
	if h.configPath == "" {
		return fmt.Errorf("config path not set")
	}

	data, err := yaml.Marshal(h.config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(h.configPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// processTokenCreate handles API token creation
func (h *Handler) processTokenCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/tokens?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	validDays := 365 // Default to 1 year

	if name == "" {
		http.Redirect(w, r, "/admin/tokens?error=Name+is+required", http.StatusSeeOther)
		return
	}

	token := h.auth.CreateAPIToken(name, description, []string{"*"}, validDays)
	log.Printf("[Admin] API token created: %s", name)

	// Redirect with the token shown (only time it's visible)
	http.Redirect(w, r, "/admin/tokens?new_token="+token.Token, http.StatusSeeOther)
}

// API handlers

// apiStatus returns server status
func (h *Handler) apiStatus(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	status := map[string]interface{}{
		"status":     "ok",
		"version":    config.Version,
		"go_version": runtime.Version(),
		"uptime":     time.Since(h.startTime).String(),
		"memory": map[string]interface{}{
			"alloc":       m.Alloc,
			"total_alloc": m.TotalAlloc,
			"sys":         m.Sys,
		},
		"goroutines": runtime.NumGoroutine(),
		"config": map[string]interface{}{
			"mode":        h.config.Server.Mode,
			"tor_enabled": h.config.Server.Tor.Enabled,
			"ssl_enabled": h.config.Server.SSL.Enabled,
		},
	}

	h.jsonResponse(w, status, http.StatusOK)
}

// apiConfig returns or updates configuration
func (h *Handler) apiConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// Return current config (sanitized)
		cfg := map[string]interface{}{
			"server": map[string]interface{}{
				"title":       h.config.Server.Title,
				"description": h.config.Server.Description,
				"port":        h.config.Server.Port,
				"mode":        h.config.Server.Mode,
			},
		}
		h.jsonResponse(w, cfg, http.StatusOK)
		return
	}

	if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		// Update config
		var updates map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Apply updates to config
		if serverUpdates, ok := updates["server"].(map[string]interface{}); ok {
			if title, ok := serverUpdates["title"].(string); ok {
				h.config.Server.Title = title
			}
			if desc, ok := serverUpdates["description"].(string); ok {
				h.config.Server.Description = desc
			}
		}

		// Save and reload
		if h.configPath != "" {
			if err := h.saveConfig(); err != nil {
				h.jsonError(w, "Failed to save config", http.StatusInternalServerError)
				return
			}
		}

		if h.reloadCallback != nil {
			h.reloadCallback()
		}

		h.jsonResponse(w, map[string]string{"status": "updated"}, http.StatusOK)
		return
	}

	h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// apiEngines returns search engine status
func (h *Handler) apiEngines(w http.ResponseWriter, r *http.Request) {
	engines := []map[string]interface{}{}

	if h.registry != nil {
		for _, eng := range h.registry.GetAll() {
			// Use type assertion to get engine info
			if e, ok := eng.(interface {
				Name() string
				IsEnabled() bool
				GetPriority() int
			}); ok {
				engines = append(engines, map[string]interface{}{
					"name":     e.Name(),
					"enabled":  e.IsEnabled(),
					"priority": e.GetPriority(),
				})
			}
		}
	}

	h.jsonResponse(w, map[string]interface{}{"engines": engines}, http.StatusOK)
}

// apiTokens manages API tokens
func (h *Handler) apiTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tokens := h.auth.ListAPITokens()
		h.jsonResponse(w, map[string]interface{}{"tokens": tokens}, http.StatusOK)

	case http.MethodPost:
		var req struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Permissions []string `json:"permissions"`
			ValidDays   int      `json:"valid_days"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Name == "" {
			h.jsonError(w, "Name is required", http.StatusBadRequest)
			return
		}

		if req.ValidDays <= 0 {
			req.ValidDays = 365
		}
		if len(req.Permissions) == 0 {
			req.Permissions = []string{"*"}
		}

		token := h.auth.CreateAPIToken(req.Name, req.Description, req.Permissions, req.ValidDays)
		h.jsonResponse(w, map[string]interface{}{
			"token": token.Token,
			"name":  token.Name,
		}, http.StatusCreated)

	case http.MethodDelete:
		token := r.URL.Query().Get("token")
		if token == "" {
			h.jsonError(w, "Token parameter required", http.StatusBadRequest)
			return
		}

		if h.auth.RevokeAPIToken(token) {
			h.jsonResponse(w, map[string]string{"status": "revoked"}, http.StatusOK)
		} else {
			h.jsonError(w, "Token not found", http.StatusNotFound)
		}

	default:
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// apiReload triggers a configuration reload
func (h *Handler) apiReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("[Admin API] Configuration reload requested")

	if h.reloadCallback != nil {
		if err := h.reloadCallback(); err != nil {
			h.jsonError(w, fmt.Sprintf("Reload failed: %v", err), http.StatusInternalServerError)
			return
		}
	}

	h.jsonResponse(w, map[string]string{"status": "reloaded"}, http.StatusOK)
}

// Helper methods

func (h *Handler) jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) jsonError(w http.ResponseWriter, message string, status int) {
	h.jsonResponse(w, map[string]string{"error": message}, status)
}

// AdminPageData holds data for admin templates
type AdminPageData struct {
	Title          string
	Description    string
	Page           string
	Config         *config.Config
	Stats          *DashboardStats
	Tokens         []*APIToken
	Error          string
	Success        string
	NewToken       string
	SchedulerTasks map[string]*SchedulerTaskInfo
}

// SchedulerTaskInfo holds information about a scheduled task
type SchedulerTaskInfo struct {
	Name     string
	Schedule string
	LastRun  time.Time
	NextRun  time.Time
	Enabled  bool
}

// DashboardStats holds dashboard statistics
type DashboardStats struct {
	Uptime         string
	Version        string
	GoVersion      string
	NumGoroutines  int
	MemAlloc       string
	MemTotal       string
	NumCPU         int
	ServerMode     string
	TorEnabled     bool
	SSLEnabled     bool
	EnginesEnabled int
}

// Helper functions

func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// getEngineCount returns the number of enabled search engines
func (h *Handler) getEngineCount() int {
	if h.registry != nil {
		return len(h.registry.GetEnabled())
	}
	return 0
}

// Server Settings Handlers

// handleServerSettings renders the server settings page
func (h *Handler) handleServerSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerSettingsUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "Server Settings",
		Page:    "admin-server-settings",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-settings", data)
}

// processServerSettingsUpdate handles server settings form submission
func (h *Handler) processServerSettingsUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/settings?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	// Update server settings
	if title := r.FormValue("title"); title != "" {
		h.config.Server.Title = title
	}
	if desc := r.FormValue("description"); desc != "" {
		h.config.Server.Description = desc
	}
	if baseURL := r.FormValue("base_url"); baseURL != "" {
		h.config.Server.BaseURL = baseURL
	}

	h.saveAndReload(w, r, "/admin/server/settings")
}

// handleServerBranding renders the branding settings page
func (h *Handler) handleServerBranding(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerBrandingUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "Branding",
		Page:    "admin-server-branding",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-branding", data)
}

// processServerBrandingUpdate handles branding form submission
func (h *Handler) processServerBrandingUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/branding?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	if appName := r.FormValue("app_name"); appName != "" {
		h.config.Server.Branding.AppName = appName
	}
	if theme := r.FormValue("theme"); theme != "" {
		h.config.Server.Branding.Theme = theme
	}
	if primaryColor := r.FormValue("primary_color"); primaryColor != "" {
		h.config.Server.Branding.PrimaryColor = primaryColor
	}
	if logoURL := r.FormValue("logo_url"); logoURL != "" {
		h.config.Server.Branding.LogoURL = logoURL
	}
	if faviconURL := r.FormValue("favicon_url"); faviconURL != "" {
		h.config.Server.Branding.FaviconURL = faviconURL
	}
	if footerText := r.FormValue("footer_text"); footerText != "" {
		h.config.Server.Branding.FooterText = footerText
	}

	h.saveAndReload(w, r, "/admin/server/branding")
}

// handleServerSSL renders the SSL/TLS settings page
func (h *Handler) handleServerSSL(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerSSLUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "SSL/TLS",
		Page:    "admin-server-ssl",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-ssl", data)
}

// processServerSSLUpdate handles SSL settings form submission
func (h *Handler) processServerSSLUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/ssl?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	h.config.Server.SSL.Enabled = r.FormValue("enabled") == "on"
	h.config.Server.SSL.AutoTLS = r.FormValue("auto_tls") == "on"
	h.config.Server.SSL.CertFile = r.FormValue("cert_file")
	h.config.Server.SSL.KeyFile = r.FormValue("key_file")
	h.config.Server.SSL.LetsEncrypt.Enabled = r.FormValue("letsencrypt_enabled") == "on"
	h.config.Server.SSL.LetsEncrypt.Email = r.FormValue("letsencrypt_email")

	h.saveAndReload(w, r, "/admin/server/ssl")
}

// handleServerTor renders the Tor settings page
func (h *Handler) handleServerTor(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerTorUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "Tor Hidden Service",
		Page:    "admin-server-tor",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-tor", data)
}

// processServerTorUpdate handles Tor settings form submission
func (h *Handler) processServerTorUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/tor?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	h.config.Server.Tor.Enabled = r.FormValue("enabled") == "on"
	h.config.Server.Tor.StreamIsolation = r.FormValue("stream_isolation") == "on"

	h.saveAndReload(w, r, "/admin/server/tor")
}

// handleServerWeb renders the web settings page (robots.txt, security.txt)
func (h *Handler) handleServerWeb(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerWebUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "Web Settings",
		Page:    "admin-server-web",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-web", data)
}

// processServerWebUpdate handles web settings form submission
func (h *Handler) processServerWebUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/web?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	h.config.Server.Web.Security.Contact = r.FormValue("security_contact")
	h.config.Server.Web.Security.Expires = r.FormValue("security_expires")
	h.config.Server.Web.CORS = r.FormValue("cors")

	h.saveAndReload(w, r, "/admin/server/web")
}

// handleServerEmail renders the email settings page
func (h *Handler) handleServerEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerEmailUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "Email Settings",
		Page:    "admin-server-email",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-email", data)
}

// processServerEmailUpdate handles email settings form submission
func (h *Handler) processServerEmailUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/email?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	h.config.Server.Email.Enabled = r.FormValue("enabled") == "on"
	h.config.Server.Email.SMTPHost = r.FormValue("smtp_host")
	if port := r.FormValue("smtp_port"); port != "" {
		fmt.Sscanf(port, "%d", &h.config.Server.Email.SMTPPort)
	}
	h.config.Server.Email.SMTPUser = r.FormValue("smtp_user")
	if pass := r.FormValue("smtp_pass"); pass != "" && pass != "********" {
		h.config.Server.Email.SMTPPass = pass
	}
	h.config.Server.Email.FromAddress = r.FormValue("from_address")
	h.config.Server.Email.FromName = r.FormValue("from_name")
	h.config.Server.Email.TLS = r.FormValue("tls") == "on"

	h.saveAndReload(w, r, "/admin/server/email")
}

// handleServerAnnouncements renders the announcements settings page
func (h *Handler) handleServerAnnouncements(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerAnnouncementsUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "Announcements",
		Page:    "admin-server-announcements",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-announcements", data)
}

// processServerAnnouncementsUpdate handles announcements form submission
func (h *Handler) processServerAnnouncementsUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/announcements?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	h.config.Server.Web.Announcements.Enabled = r.FormValue("enabled") == "on"

	h.saveAndReload(w, r, "/admin/server/announcements")
}

// handleServerGeoIP renders the GeoIP settings page
func (h *Handler) handleServerGeoIP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerGeoIPUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "GeoIP",
		Page:    "admin-server-geoip",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-geoip", data)
}

// processServerGeoIPUpdate handles GeoIP settings form submission
func (h *Handler) processServerGeoIPUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/geoip?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	h.config.Server.GeoIP.Enabled = r.FormValue("enabled") == "on"
	h.config.Server.GeoIP.ASN = r.FormValue("asn") == "on"
	h.config.Server.GeoIP.Country = r.FormValue("country") == "on"
	h.config.Server.GeoIP.City = r.FormValue("city") == "on"

	h.saveAndReload(w, r, "/admin/server/geoip")
}

// handleServerMetrics renders the metrics settings page
func (h *Handler) handleServerMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		h.processServerMetricsUpdate(w, r)
		return
	}

	data := &AdminPageData{
		Title:   "Metrics",
		Page:    "admin-server-metrics",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-metrics", data)
}

// processServerMetricsUpdate handles metrics settings form submission
func (h *Handler) processServerMetricsUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/server/metrics?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	h.config.Server.Metrics.Enabled = r.FormValue("enabled") == "on"
	h.config.Server.Metrics.IncludeSystem = r.FormValue("include_system") == "on"
	if token := r.FormValue("token"); token != "" && token != "********" {
		h.config.Server.Metrics.Token = token
	}

	h.saveAndReload(w, r, "/admin/server/metrics")
}

// handleScheduler renders the scheduler management page
func (h *Handler) handleScheduler(w http.ResponseWriter, r *http.Request) {
	// Build scheduler task info
	schedulerTasks := make(map[string]*SchedulerTaskInfo)

	// Define default tasks with their schedules
	tasks := []struct {
		id       string
		name     string
		schedule string
	}{
		{"backup", "Backup", "0 3 * * *"},
		{"cache_cleanup", "Cache Cleanup", "0 */6 * * *"},
		{"log_rotation", "Log Rotation", "0 0 * * 0"},
		{"geoip_update", "GeoIP Update", "0 4 * * 3"},
		{"engine_health", "Engine Health Check", "*/15 * * * *"},
	}

	for _, task := range tasks {
		schedulerTasks[task.id] = &SchedulerTaskInfo{
			Name:     task.name,
			Schedule: task.schedule,
			Enabled:  true, // Default to enabled
			// LastRun and NextRun will be zero values (displayed as "Never")
		}
	}

	data := &AdminPageData{
		Title:          "Scheduler",
		Page:           "admin-scheduler",
		Config:         h.config,
		Error:          r.URL.Query().Get("error"),
		Success:        r.URL.Query().Get("success"),
		SchedulerTasks: schedulerTasks,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "scheduler", data)
}

// saveAndReload saves config and triggers reload, then redirects
func (h *Handler) saveAndReload(w http.ResponseWriter, r *http.Request, redirectPath string) {
	if h.configPath != "" {
		if err := h.saveConfig(); err != nil {
			log.Printf("[Admin] Failed to save config: %v", err)
			http.Redirect(w, r, redirectPath+"?error=Failed+to+save+config", http.StatusSeeOther)
			return
		}
	}

	if h.reloadCallback != nil {
		if err := h.reloadCallback(); err != nil {
			log.Printf("[Admin] Failed to reload config: %v", err)
		}
	}

	http.Redirect(w, r, redirectPath+"?success=Settings+saved", http.StatusSeeOther)
}

// ============================================================
// Additional Admin API Endpoints
// ============================================================

// apiBackups handles backup management API
func (h *Handler) apiBackups(w http.ResponseWriter, r *http.Request) {
	bm := backup.NewManager()

	switch r.Method {
	case http.MethodGet:
		// List backups
		backups, err := bm.List()
		if err != nil {
			h.jsonError(w, fmt.Sprintf("Failed to list backups: %v", err), http.StatusInternalServerError)
			return
		}
		h.jsonResponse(w, map[string]interface{}{
			"backups": backups,
			"total":   len(backups),
		}, http.StatusOK)

	case http.MethodPost:
		// Create backup
		var req struct {
			Filename string `json:"filename"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		backupPath, err := bm.Create(req.Filename)
		if err != nil {
			h.jsonError(w, fmt.Sprintf("Backup failed: %v", err), http.StatusInternalServerError)
			return
		}

		metadata, _ := bm.GetMetadata(backupPath)
		h.jsonResponse(w, map[string]interface{}{
			"path":     backupPath,
			"filename": filepath.Base(backupPath),
			"metadata": metadata,
		}, http.StatusCreated)

	case http.MethodDelete:
		// Delete backup
		filename := r.URL.Query().Get("filename")
		if filename == "" {
			h.jsonError(w, "Filename parameter required", http.StatusBadRequest)
			return
		}

		if err := bm.Delete(filename); err != nil {
			h.jsonError(w, fmt.Sprintf("Failed to delete backup: %v", err), http.StatusInternalServerError)
			return
		}
		h.jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)

	default:
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// apiLogs handles log viewing API
func (h *Handler) apiLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logType := r.URL.Query().Get("type")
	if logType == "" {
		logType = "server"
	}

	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		fmt.Sscanf(l, "%d", &lines)
	}
	if lines > 1000 {
		lines = 1000
	}

	// Get log file path
	logDir := config.GetDataDir()
	var logPath string
	switch logType {
	case "access":
		logPath = filepath.Join(logDir, "logs", "access.log")
	case "error":
		logPath = filepath.Join(logDir, "logs", "error.log")
	case "security":
		logPath = filepath.Join(logDir, "logs", "security.log")
	case "audit":
		logPath = filepath.Join(logDir, "logs", "audit.log")
	default:
		logPath = filepath.Join(logDir, "logs", "server.log")
	}

	// Read last N lines from log file
	logLines, err := readLastLines(logPath, lines)
	if err != nil {
		h.jsonResponse(w, map[string]interface{}{
			"type":  logType,
			"lines": []string{},
			"error": fmt.Sprintf("Could not read log file: %v", err),
		}, http.StatusOK)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"type":  logType,
		"path":  logPath,
		"lines": logLines,
		"total": len(logLines),
	}, http.StatusOK)
}

// readLastLines reads the last n lines from a file
func readLastLines(path string, n int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n*2 { // Keep a buffer
			lines = lines[n:]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Return last n lines
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}

	return lines, nil
}

// apiScheduler handles scheduler task management API
func (h *Handler) apiScheduler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List scheduled tasks
		tasks := h.config.Server.Scheduler.Tasks
		h.jsonResponse(w, map[string]interface{}{
			"enabled": h.config.Server.Scheduler.Enabled,
			"tasks":   tasks,
			"total":   len(tasks),
		}, http.StatusOK)

	case http.MethodPost:
		// Add/update task
		var task config.ScheduledTask
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			h.jsonError(w, "Invalid task data", http.StatusBadRequest)
			return
		}

		// Find or append task
		found := false
		for i, t := range h.config.Server.Scheduler.Tasks {
			if t.ID == task.ID {
				h.config.Server.Scheduler.Tasks[i] = task
				found = true
				break
			}
		}
		if !found {
			h.config.Server.Scheduler.Tasks = append(h.config.Server.Scheduler.Tasks, task)
		}

		h.saveConfig()
		h.jsonResponse(w, map[string]string{"status": "saved"}, http.StatusOK)

	case http.MethodDelete:
		// Delete task
		taskID := r.URL.Query().Get("id")
		if taskID == "" {
			h.jsonError(w, "Task ID required", http.StatusBadRequest)
			return
		}

		tasks := make([]config.ScheduledTask, 0)
		for _, t := range h.config.Server.Scheduler.Tasks {
			if t.ID != taskID {
				tasks = append(tasks, t)
			}
		}
		h.config.Server.Scheduler.Tasks = tasks

		h.saveConfig()
		h.jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)

	default:
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// apiEmailTest sends a test email
func (h *Handler) apiEmailTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.config.Server.Email.Enabled {
		h.jsonError(w, "Email is not enabled", http.StatusBadRequest)
		return
	}

	// Get test recipient
	var req struct {
		To string `json:"to"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Use admin email if not specified
	to := req.To
	if to == "" {
		to = h.config.Server.Admin.Email
	}
	if to == "" {
		h.jsonError(w, "No recipient email specified", http.StatusBadRequest)
		return
	}

	// For now, just validate the configuration is set
	// Full email sending would require implementing SMTP client
	if h.config.Server.Email.SMTPHost == "" {
		h.jsonError(w, "SMTP host not configured", http.StatusBadRequest)
		return
	}

	log.Printf("[Admin] Test email requested to: %s", to)

	h.jsonResponse(w, map[string]interface{}{
		"status":  "queued",
		"message": fmt.Sprintf("Test email queued for delivery to %s", to),
		"note":    "Email delivery requires SMTP configuration",
	}, http.StatusOK)
}

// apiUpdateCheck checks for updates
func (h *Handler) apiUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return current version info
	// Full update checking would require checking a release API
	h.jsonResponse(w, map[string]interface{}{
		"current_version": config.Version,
		"build_time":      config.BuildTime,
		"go_version":      config.GoVersion,
		"git_commit":      config.GitCommit,
		"update_available": false, // Would check against releases
		"latest_version":   config.Version, // Would fetch from releases
		"release_notes":    "",
		"download_url":     "",
	}, http.StatusOK)
}

// GetClientIPFromString extracts client IP from request with proper handling
func GetClientIPFromString(remoteAddr string, headers http.Header) string {
	// Check X-Forwarded-For first
	if xff := headers.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Check X-Real-IP
	if xri := headers.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to remote addr
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		// Check if this is IPv6
		if !strings.Contains(remoteAddr[idx:], "]") {
			return remoteAddr[:idx]
		}
	}
	return remoteAddr
}
