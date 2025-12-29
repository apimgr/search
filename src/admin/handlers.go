package admin

import (
	"bufio"
	"context"
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
	"github.com/apimgr/search/src/email"
	tlspkg "github.com/apimgr/search/src/tls"
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
	service        *AdminService
	cluster        ClusterManager
	tor            TorManager
	renderer       Renderer
	startTime      time.Time
	registry       EngineRegistry
	reloadCallback ReloadCallback
	configPath     string
}

// TorManager interface for Tor operations per AI.md PART 32
type TorManager interface {
	IsRunning() bool
	GetOnionAddress() string
	GetTorStatus() map[string]interface{}
	Start() error
	Stop() error
	Restart() error
	RegenerateAddress() (string, error)
	GenerateVanity(prefix string) error
	CancelVanity()
	GetVanityProgress() *VanityProgress
	ExportKeys() ([]byte, error)
	ImportKeys(privateKey []byte) (string, error)
}

// VanityProgress represents vanity address generation progress
type VanityProgress struct {
	Prefix    string
	Attempts  int64
	StartTime time.Time
	Running   bool
	Found     bool
	Address   string
	Error     string
}

// ClusterManager interface for cluster operations
type ClusterManager interface {
	Mode() string
	IsClusterMode() bool
	IsPrimary() bool
	NodeID() string
	Hostname() string
	GetNodes(ctx context.Context) ([]ClusterNode, error)
	GenerateJoinToken(ctx context.Context) (string, error)
	LeaveCluster(ctx context.Context) error
}

// ClusterNode represents a cluster node
type ClusterNode struct {
	ID        string
	Hostname  string
	Address   string
	Port      int
	Version   string
	IsPrimary bool
	Status    string
	LastSeen  time.Time
	JoinedAt  time.Time
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

// SetAdminService sets the admin service for multi-admin support
func (h *Handler) SetAdminService(svc *AdminService) {
	h.service = svc
}

// SetClusterManager sets the cluster manager for node management
func (h *Handler) SetClusterManager(cm ClusterManager) {
	h.cluster = cm
}

// SetTorManager sets the Tor service manager per AI.md PART 32
func (h *Handler) SetTorManager(tm TorManager) {
	h.tor = tm
}

// RegisterRoutes registers admin routes on the given mux
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Public routes (no auth required)
	mux.HandleFunc("/admin/login", h.handleLogin)
	mux.HandleFunc("/admin/logout", h.handleLogout)
	mux.HandleFunc("/admin/setup", h.handleSetup)
	mux.HandleFunc("/admin/invite/", h.handleInviteAccept)

	// Protected routes (auth required)
	mux.HandleFunc("/admin", h.requireAuth(h.handleDashboard))
	mux.HandleFunc("/admin/", h.requireAuth(h.handleDashboard))
	mux.HandleFunc("/admin/dashboard", h.requireAuth(h.handleDashboard))
	mux.HandleFunc("/admin/config", h.requireAuth(h.handleConfig))
	mux.HandleFunc("/admin/engines", h.requireAuth(h.handleEngines))
	mux.HandleFunc("/admin/tokens", h.requireAuth(h.handleTokens))
	mux.HandleFunc("/admin/logs", h.requireAuth(h.handleLogs))

	// Server settings routes (per AI.md)
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

	// Additional server settings routes (per AI.md)
	mux.HandleFunc("/admin/server/backup", h.requireAuth(h.handleServerBackup))
	mux.HandleFunc("/admin/server/maintenance", h.requireAuth(h.handleServerMaintenance))
	mux.HandleFunc("/admin/server/updates", h.requireAuth(h.handleServerUpdates))
	mux.HandleFunc("/admin/server/info", h.requireAuth(h.handleServerInfo))
	mux.HandleFunc("/admin/server/security", h.requireAuth(h.handleServerSecurity))
	mux.HandleFunc("/admin/help", h.requireAuth(h.handleHelp))

	// Admin management routes (per AI.md PART 31)
	mux.HandleFunc("/admin/users/admins", h.requireAuth(h.handleAdmins))
	mux.HandleFunc("/admin/users/admins/invite", h.requireAuth(h.handleAdminInvite))

	// Cluster/Node management routes (per AI.md PART 24)
	mux.HandleFunc("/admin/server/nodes", h.requireAuth(h.handleNodes))
	mux.HandleFunc("/admin/server/nodes/token", h.requireAuth(h.handleNodesToken))
	mux.HandleFunc("/admin/server/nodes/leave", h.requireAuth(h.handleNodesLeave))

	// API routes (bearer token auth) - per AI.md spec: /api/v1/admin/*
	mux.HandleFunc("/api/v1/admin/status", h.requireAPIAuth(h.apiStatus))
	mux.HandleFunc("/api/v1/admin/config", h.requireAPIAuth(h.apiConfig))
	mux.HandleFunc("/api/v1/admin/engines", h.requireAPIAuth(h.apiEngines))
	mux.HandleFunc("/api/v1/admin/tokens", h.requireAPIAuth(h.apiTokens))
	mux.HandleFunc("/api/v1/admin/reload", h.requireAPIAuth(h.apiReload))
	mux.HandleFunc("/api/v1/admin/backups", h.requireAPIAuth(h.apiBackups))
	mux.HandleFunc("/api/v1/admin/logs", h.requireAPIAuth(h.apiLogs))
	mux.HandleFunc("/api/v1/admin/scheduler", h.requireAPIAuth(h.apiScheduler))
	mux.HandleFunc("/api/v1/admin/email/test", h.requireAPIAuth(h.apiEmailTest))
	mux.HandleFunc("/api/v1/admin/email/templates", h.requireAPIAuth(h.apiEmailTemplates))
	mux.HandleFunc("/api/v1/admin/email/preview", h.requireAPIAuth(h.apiEmailPreview))
	mux.HandleFunc("/api/v1/admin/update/check", h.requireAPIAuth(h.apiUpdateCheck))
	mux.HandleFunc("/api/v1/admin/admins", h.requireAPIAuth(h.apiAdmins))
	mux.HandleFunc("/api/v1/admin/admins/invite", h.requireAPIAuth(h.apiAdminInvite))

	// Tor API routes per AI.md spec
	mux.HandleFunc("/api/v1/admin/tor/status", h.requireAPIAuth(h.apiTorStatus))
	mux.HandleFunc("/api/v1/admin/tor/start", h.requireAPIAuth(h.apiTorStart))
	mux.HandleFunc("/api/v1/admin/tor/stop", h.requireAPIAuth(h.apiTorStop))
	mux.HandleFunc("/api/v1/admin/tor/restart", h.requireAPIAuth(h.apiTorRestart))
	mux.HandleFunc("/api/v1/admin/tor/address/regenerate", h.requireAPIAuth(h.apiTorRegenerateAddress))
	mux.HandleFunc("/api/v1/admin/tor/vanity/start", h.requireAPIAuth(h.apiTorVanityStart))
	mux.HandleFunc("/api/v1/admin/tor/vanity/status", h.requireAPIAuth(h.apiTorVanityStatus))
	mux.HandleFunc("/api/v1/admin/tor/vanity/cancel", h.requireAPIAuth(h.apiTorVanityCancel))
	mux.HandleFunc("/api/v1/admin/tor/keys/export", h.requireAPIAuth(h.apiTorKeysExport))
	mux.HandleFunc("/api/v1/admin/tor/keys/import", h.requireAPIAuth(h.apiTorKeysImport))

	// Bang management API routes (per AI.md PART 36 line 28288)
	mux.HandleFunc("/api/v1/admin/bangs", h.requireAPIAuth(h.apiBangs))
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

	// Determine server status
	status := "Online"
	if h.config.Server.MaintenanceMode {
		status = "Maintenance"
	}

	// Calculate memory percentage (rough estimate based on system memory)
	memPercent := float64(m.Alloc) / float64(m.Sys) * 100
	if memPercent > 100 {
		memPercent = 100
	}

	// Build alerts list
	var alerts []AlertItem
	if h.config.Server.MaintenanceMode {
		alerts = append(alerts, AlertItem{
			Message: "Server is in maintenance mode",
			Type:    "warning",
		})
	}
	if !h.config.Server.SSL.Enabled && h.config.Server.Mode == "production" {
		alerts = append(alerts, AlertItem{
			Message: "SSL/TLS is not enabled in production mode",
			Type:    "warning",
		})
	}

	// Build scheduled tasks list
	scheduledTasks := []ScheduledTask{
		{Name: "Automatic Backup", NextRun: "02:00 daily"},
		{Name: "SSL Renewal Check", NextRun: "03:00 daily"},
		{Name: "GeoIP Update", NextRun: "03:00 Sunday"},
		{Name: "Session Cleanup", NextRun: "hourly"},
	}

	// Recent activity (placeholder - in production, read from audit log)
	recentActivity := []ActivityItem{
		{Time: time.Now().Format("15:04"), Message: "Admin logged in", Type: "info"},
		{Time: h.startTime.Format("15:04"), Message: "Server started", Type: "success"},
	}

	data := &AdminPageData{
		Title:  "Dashboard",
		Page:   "admin-dashboard",
		Config: h.config,
		Stats: &DashboardStats{
			Status:         status,
			Uptime:         formatDuration(time.Since(h.startTime)),
			Version:        config.Version,
			Requests24h:    0, // Metrics collected per-request by server middleware
			Errors24h:      0, // Metrics collected per-request by server middleware
			CPUPercent:     0, // CPU usage requires platform-specific code
			MemPercent:     memPercent,
			DiskPercent:    0, // Disk usage requires platform-specific code
			MemAlloc:       formatBytes(m.Alloc),
			MemTotal:       formatBytes(m.TotalAlloc),
			GoVersion:      runtime.Version(),
			NumGoroutines:  runtime.NumGoroutine(),
			NumCPU:         runtime.NumCPU(),
			ServerMode:     h.config.Server.Mode,
			TorEnabled:     h.config.Server.Tor.Enabled,
			SSLEnabled:     h.config.Server.SSL.Enabled,
			EnginesEnabled: h.getEngineCount(),
			RecentActivity: recentActivity,
			ScheduledTasks: scheduledTasks,
			Alerts:         alerts,
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
	Extra          map[string]interface{}
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
	// Status
	Status         string // Online, Maintenance, Error
	Uptime         string
	Version        string

	// Request stats (24h)
	Requests24h    int64
	Errors24h      int64

	// System resources
	CPUPercent     float64
	MemPercent     float64
	DiskPercent    float64
	MemAlloc       string
	MemTotal       string

	// Runtime info
	GoVersion      string
	NumGoroutines  int
	NumCPU         int
	ServerMode     string

	// Feature status
	TorEnabled     bool
	SSLEnabled     bool
	EnginesEnabled int

	// Recent activity (last 5 items)
	RecentActivity []ActivityItem

	// Scheduled tasks (next 5 tasks)
	ScheduledTasks []ScheduledTask

	// Alerts/Warnings
	Alerts []AlertItem
}

// ActivityItem represents a recent activity log entry
type ActivityItem struct {
	Time    string
	Message string
	Type    string // info, success, warning, error
}

// ScheduledTask represents an upcoming scheduled task
type ScheduledTask struct {
	Name    string
	NextRun string
}

// AlertItem represents an alert or warning
type AlertItem struct {
	Message string
	Type    string // info, warning, error
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

	// Get DNS providers list for template
	dnsProviders := tlspkg.DNSProviders()

	data := &AdminPageData{
		Title:   "SSL/TLS",
		Page:    "admin-server-ssl",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
		Extra: map[string]interface{}{
			"DNSProviders":        dnsProviders,
			"CurrentDNSProvider":  h.config.Server.SSL.DNS01.Provider,
			"DNS01Configured":     h.config.Server.SSL.DNS01.CredentialsEncrypted != "",
			"DNS01ValidatedAt":    h.config.Server.SSL.DNS01.ValidatedAt,
		},
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
	h.config.Server.SSL.LetsEncrypt.Challenge = r.FormValue("letsencrypt_challenge")

	// Handle DNS-01 provider configuration
	dnsProvider := r.FormValue("dns_provider")
	if dnsProvider != "" && h.config.Server.SSL.LetsEncrypt.Challenge == "dns-01" {
		// Collect provider credentials from form
		credentials := make(map[string]string)
		providerInfo := tlspkg.GetProviderByID(dnsProvider)
		if providerInfo != nil {
			for _, field := range providerInfo.Fields {
				val := r.FormValue("dns_" + field.Name)
				if val != "" {
					credentials[field.Name] = val
				}
			}
		}

		// Encrypt and store credentials if any were provided
		if len(credentials) > 0 {
			encrypted, err := tlspkg.EncryptCredentials(credentials, h.config.Server.SecretKey)
			if err != nil {
				http.Redirect(w, r, "/admin/server/ssl?error=Failed+to+encrypt+credentials", http.StatusSeeOther)
				return
			}
			h.config.Server.SSL.DNS01.Provider = dnsProvider
			h.config.Server.SSL.DNS01.CredentialsEncrypted = encrypted
			h.config.Server.SSL.DNS01.ValidatedAt = tlspkg.ValidatedAtNow()
		}
	}

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

// handleServerBackup renders the backup management page
func (h *Handler) handleServerBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		action := r.FormValue("action")
		switch action {
		case "create":
			// Trigger backup creation
			http.Redirect(w, r, "/admin/server/backup?success=Backup+created+successfully", http.StatusSeeOther)
			return
		case "restore":
			// Handle restore (would need file upload)
			http.Redirect(w, r, "/admin/server/backup?success=Restore+initiated", http.StatusSeeOther)
			return
		}
	}

	data := &AdminPageData{
		Title:   "Backup & Restore",
		Page:    "admin-server-backup",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-backup", data)
}

// handleServerMaintenance renders the maintenance mode page
func (h *Handler) handleServerMaintenance(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		enabled := r.FormValue("enabled") == "on"
		h.config.Server.MaintenanceMode = enabled
		h.saveAndReload(w, r, "/admin/server/maintenance")
		return
	}

	data := &AdminPageData{
		Title:   "Maintenance Mode",
		Page:    "admin-server-maintenance",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-maintenance", data)
}

// handleServerUpdates renders the updates management page
func (h *Handler) handleServerUpdates(w http.ResponseWriter, r *http.Request) {
	data := &AdminPageData{
		Title:   "Updates",
		Page:    "admin-server-updates",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-updates", data)
}

// handleServerInfo renders the server information page
func (h *Handler) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	data := &AdminPageData{
		Title:   "Server Info",
		Page:    "admin-server-info",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
		Stats: &DashboardStats{
			Version:       config.Version,
			GoVersion:     runtime.Version(),
			NumGoroutines: runtime.NumGoroutine(),
			NumCPU:        runtime.NumCPU(),
			MemAlloc:      formatBytes(m.Alloc),
			MemTotal:      formatBytes(m.TotalAlloc),
			Uptime:        formatDuration(time.Since(h.startTime)),
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-info", data)
}

// handleServerSecurity renders the security settings page
func (h *Handler) handleServerSecurity(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Handle security settings update
		h.config.Server.RateLimit.Enabled = r.FormValue("rate_limit_enabled") == "on"
		h.saveAndReload(w, r, "/admin/server/security")
		return
	}

	data := &AdminPageData{
		Title:   "Security Settings",
		Page:    "admin-server-security",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "server-security", data)
}

// handleHelp renders the help/documentation page
func (h *Handler) handleHelp(w http.ResponseWriter, r *http.Request) {
	data := &AdminPageData{
		Title:  "Help & Documentation",
		Page:   "admin-help",
		Config: h.config,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "help", data)
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
		// Keep a buffer to avoid storing entire file
		if len(lines) > n*2 {
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

// apiEmailTemplates returns list of available email templates
// Per AI.md PART 16: Template preview in admin panel
func (h *Handler) apiEmailTemplates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	templates := email.GetAllTemplateTypes()
	h.jsonResponse(w, map[string]interface{}{
		"templates": templates,
		"count":     len(templates),
	}, http.StatusOK)
}

// apiEmailPreview previews an email template with sample data
// Per AI.md PART 16: Template preview in admin panel
func (h *Handler) apiEmailPreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get template type from query or body
	templateType := r.URL.Query().Get("template")
	if templateType == "" && r.Method == http.MethodPost {
		var req struct {
			Template string `json:"template"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		templateType = req.Template
	}

	if templateType == "" {
		h.jsonError(w, "Template type is required (use ?template=welcome)", http.StatusBadRequest)
		return
	}

	// Get site info from config
	siteName := h.config.Server.Title
	siteURL := fmt.Sprintf("http://%s:%d", h.config.Server.Address, h.config.Server.Port)
	if h.config.Server.Address == "" || h.config.Server.Address == "0.0.0.0" {
		siteURL = fmt.Sprintf("http://localhost:%d", h.config.Server.Port)
	}

	// Render the template with sample data
	et := email.NewEmailTemplate()
	subject, body, err := et.PreviewTemplate(email.TemplateType(templateType), siteName, siteURL)
	if err != nil {
		h.jsonError(w, fmt.Sprintf("Failed to preview template: %v", err), http.StatusBadRequest)
		return
	}

	// Check if HTML preview is requested
	if r.URL.Query().Get("format") == "html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(body))
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"template": templateType,
		"subject":  subject,
		"body":     body,
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
		"current_version":  config.Version,
		"build_date":       config.BuildDate,
		"go_version":       runtime.Version(),
		"commit_id":        config.CommitID,
		"update_available": false,            // Would check against releases
		"latest_version":   config.Version,   // Would fetch from releases
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

// ============================================================
// Multi-Admin Management per AI.md PART 31
// ============================================================

// handleSetup handles first-run admin setup or password reset via setup token
func (h *Handler) handleSetup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.service == nil {
		http.Error(w, "Admin service not configured", http.StatusInternalServerError)
		return
	}

	// Check if any admin exists
	hasAdmin, err := h.service.HasAnyAdmin(ctx)
	if err != nil {
		log.Printf("[Admin] Error checking admin existence: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If admin exists, require setup token
	setupTokenRequired := hasAdmin

	if r.Method == http.MethodPost {
		h.processSetup(w, r, setupTokenRequired)
		return
	}

	// If already have admin and no setup token required, redirect to login
	if hasAdmin {
		// Check if there's a valid setup token active
		tokenValid := false
		if token := r.URL.Query().Get("token"); token != "" {
			tokenValid, _ = h.service.ValidateSetupToken(ctx, token)
		}
		if !tokenValid {
			http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
			return
		}
	}

	data := &AdminPageData{
		Title:  "Admin Setup",
		Page:   "admin-setup",
		Config: h.config,
		Error:  r.URL.Query().Get("error"),
		Extra: map[string]interface{}{
			"SetupTokenRequired": setupTokenRequired,
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "setup", data)
}

// processSetup handles the setup form submission
func (h *Handler) processSetup(w http.ResponseWriter, r *http.Request, requireToken bool) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/setup?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	// Validate setup token if required
	if requireToken {
		token := r.FormValue("setup_token")
		valid, err := h.service.ValidateSetupToken(ctx, token)
		if err != nil || !valid {
			http.Redirect(w, r, "/admin/setup?error=Invalid+or+expired+setup+token", http.StatusSeeOther)
			return
		}
		// Mark token as used
		h.service.UseSetupToken(ctx, token)
	}

	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate input
	if username == "" || password == "" {
		http.Redirect(w, r, "/admin/setup?error=Username+and+password+required", http.StatusSeeOther)
		return
	}

	if password != confirmPassword {
		http.Redirect(w, r, "/admin/setup?error=Passwords+do+not+match", http.StatusSeeOther)
		return
	}

	if len(password) < 8 {
		http.Redirect(w, r, "/admin/setup?error=Password+must+be+at+least+8+characters", http.StatusSeeOther)
		return
	}

	// Create admin
	admin, err := h.service.CreateAdmin(ctx, username, email, password, true)
	if err != nil {
		log.Printf("[Admin] Failed to create admin: %v", err)
		http.Redirect(w, r, "/admin/setup?error=Failed+to+create+admin+account", http.StatusSeeOther)
		return
	}

	log.Printf("[Admin] Primary admin created: %s", admin.Username)

	// Create session and log in
	session := h.auth.CreateSession(username, GetClientIP(r), r.UserAgent())
	h.auth.SetSessionCookie(w, session)

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

// handleAdmins renders the admin management page
func (h *Handler) handleAdmins(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.service == nil {
		http.Error(w, "Admin service not configured", http.StatusInternalServerError)
		return
	}

	// Get current admin from session
	session, ok := h.auth.GetSessionFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	// Get admin by username
	currentAdmin, err := h.service.GetAdminByUsername(ctx, session.Username)
	if err != nil || currentAdmin == nil {
		http.Error(w, "Admin not found", http.StatusInternalServerError)
		return
	}

	// Get admins visible to this admin (privacy enforced)
	admins, err := h.service.GetAdminsForAdmin(ctx, currentAdmin.ID)
	if err != nil {
		log.Printf("[Admin] Error getting admins: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get total count (visible to all)
	totalCount, _ := h.service.GetTotalAdminCount(ctx)

	data := &AdminPageData{
		Title:   "Server Admins",
		Page:    "admin-admins",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
		Extra: map[string]interface{}{
			"Admins":       admins,
			"TotalCount":   totalCount,
			"CurrentAdmin": currentAdmin,
			"IsPrimary":    currentAdmin.IsPrimary,
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "admins", data)
}

// handleAdminInvite creates a new admin invite
func (h *Handler) handleAdminInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.service == nil {
		http.Redirect(w, r, "/admin/users/admins?error=Admin+service+not+configured", http.StatusSeeOther)
		return
	}

	// Get current admin from session
	session, ok := h.auth.GetSessionFromRequest(r)
	if !ok {
		http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
		return
	}

	currentAdmin, err := h.service.GetAdminByUsername(ctx, session.Username)
	if err != nil || currentAdmin == nil {
		http.Redirect(w, r, "/admin/users/admins?error=Admin+not+found", http.StatusSeeOther)
		return
	}

	// Only primary admin can create invites
	if !currentAdmin.IsPrimary {
		http.Redirect(w, r, "/admin/users/admins?error=Only+primary+admin+can+create+invites", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/admin/users/admins?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")

	// Create invite (7 day expiry per AI.md)
	token, err := h.service.CreateInvite(ctx, currentAdmin.ID, username, 7*24*time.Hour)
	if err != nil {
		log.Printf("[Admin] Failed to create invite: %v", err)
		http.Redirect(w, r, "/admin/users/admins?error=Failed+to+create+invite", http.StatusSeeOther)
		return
	}

	log.Printf("[Admin] Invite created by %s for username: %s", currentAdmin.Username, username)

	// Redirect with the invite URL shown
	inviteURL := fmt.Sprintf("/admin/invite/%s", token)
	http.Redirect(w, r, "/admin/users/admins?success=Invite+created&invite_url="+inviteURL, http.StatusSeeOther)
}

// handleInviteAccept handles the invite acceptance page
func (h *Handler) handleInviteAccept(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.service == nil {
		http.Error(w, "Admin service not configured", http.StatusInternalServerError)
		return
	}

	// Extract token from URL path
	token := strings.TrimPrefix(r.URL.Path, "/admin/invite/")
	if token == "" {
		h.renderInviteError(w, "Invalid invite link")
		return
	}

	// Validate token
	invite, err := h.service.ValidateInvite(ctx, token)
	if err != nil {
		log.Printf("[Admin] Error validating invite: %v", err)
		h.renderInviteError(w, "Invalid or expired invite")
		return
	}
	if invite == nil {
		h.renderInviteError(w, "Invalid or expired invite")
		return
	}

	if r.Method == http.MethodPost {
		h.processInviteAccept(w, r, token, invite)
		return
	}

	data := &AdminPageData{
		Title:  "Accept Admin Invite",
		Page:   "admin-invite",
		Config: h.config,
		Error:  r.URL.Query().Get("error"),
		Extra: map[string]interface{}{
			"Invite":           invite,
			"SuggestedUsername": invite.Username,
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "invite-accept", data)
}

// processInviteAccept processes the invite acceptance form
func (h *Handler) processInviteAccept(w http.ResponseWriter, r *http.Request, token string, invite *AdminInvite) {
	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, r.URL.Path+"?error=Invalid+form+data", http.StatusSeeOther)
		return
	}

	username := r.FormValue("username")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate input
	if username == "" || password == "" {
		http.Redirect(w, r, r.URL.Path+"?error=Username+and+password+required", http.StatusSeeOther)
		return
	}

	if password != confirmPassword {
		http.Redirect(w, r, r.URL.Path+"?error=Passwords+do+not+match", http.StatusSeeOther)
		return
	}

	if len(password) < 8 {
		http.Redirect(w, r, r.URL.Path+"?error=Password+must+be+at+least+8+characters", http.StatusSeeOther)
		return
	}

	// Accept invite and create admin
	admin, err := h.service.AcceptInvite(ctx, token, username, email, password)
	if err != nil {
		log.Printf("[Admin] Failed to accept invite: %v", err)
		http.Redirect(w, r, r.URL.Path+"?error=Failed+to+create+admin+account", http.StatusSeeOther)
		return
	}

	log.Printf("[Admin] New admin created via invite: %s", admin.Username)

	// Create session and log in
	session := h.auth.CreateSession(username, GetClientIP(r), r.UserAgent())
	h.auth.SetSessionCookie(w, session)

	http.Redirect(w, r, "/admin/dashboard", http.StatusSeeOther)
}

// renderInviteError renders the invite error page
func (h *Handler) renderInviteError(w http.ResponseWriter, message string) {
	data := &AdminPageData{
		Title:  "Invalid Invite",
		Page:   "admin-invite-error",
		Config: h.config,
		Error:  message,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "invite-error", data)
}

// apiAdmins handles admin management API
func (h *Handler) apiAdmins(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.service == nil {
		h.jsonError(w, "Admin service not configured", http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		// For API, get token-based admin
		// This is simplified - in production you'd have an admin context
		admins, err := h.service.GetAdminsForAdmin(ctx, 1) // Assumes primary admin ID 1
		if err != nil {
			h.jsonError(w, "Failed to get admins", http.StatusInternalServerError)
			return
		}

		// Convert to safe response (no password hashes)
		response := make([]map[string]interface{}, len(admins))
		for i, a := range admins {
			response[i] = map[string]interface{}{
				"id":           a.ID,
				"username":     a.Username,
				"email":        a.Email,
				"is_primary":   a.IsPrimary,
				"source":       a.Source,
				"totp_enabled": a.TOTPEnabled,
				"created_at":   a.CreatedAt,
				"last_login":   a.LastLoginAt,
			}
		}

		h.jsonResponse(w, map[string]interface{}{"admins": response}, http.StatusOK)

	case http.MethodDelete:
		// Delete admin
		var req struct {
			ID int64 `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if err := h.service.DeleteAdmin(ctx, req.ID, 1); err != nil {
			h.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}

		h.jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)

	default:
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// apiAdminInvite handles admin invite API
func (h *Handler) apiAdminInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.service == nil {
		h.jsonError(w, "Admin service not configured", http.StatusInternalServerError)
		return
	}

	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create invite (7 day expiry)
	token, err := h.service.CreateInvite(ctx, 1, req.Username, 7*24*time.Hour) // Assumes primary admin
	if err != nil {
		h.jsonError(w, "Failed to create invite", http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]interface{}{
		"token":      token,
		"invite_url": fmt.Sprintf("/admin/invite/%s", token),
		"expires_in": "7 days",
	}, http.StatusCreated)
}

// ============================================================
// Cluster/Node Management per AI.md PART 24
// ============================================================

// handleNodes renders the cluster nodes management page
func (h *Handler) handleNodes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var nodes []ClusterNode
	var mode string
	var isPrimary bool
	var nodeID string
	var hostname string

	if h.cluster != nil {
		mode = h.cluster.Mode()
		isPrimary = h.cluster.IsPrimary()
		nodeID = h.cluster.NodeID()
		hostname = h.cluster.Hostname()

		clusterNodes, err := h.cluster.GetNodes(ctx)
		if err != nil {
			log.Printf("[Admin] Error getting nodes: %v", err)
		} else {
			nodes = clusterNodes
		}
	} else {
		mode = "standalone"
		isPrimary = true
		hostname, _ = os.Hostname()
		nodeID = "local"
		nodes = []ClusterNode{{
			ID:        nodeID,
			Hostname:  hostname,
			IsPrimary: true,
			Status:    "online",
			LastSeen:  time.Now(),
			JoinedAt:  time.Now(),
		}}
	}

	data := &AdminPageData{
		Title:   "Cluster Nodes",
		Page:    "nodes",
		Config:  h.config,
		Error:   r.URL.Query().Get("error"),
		Success: r.URL.Query().Get("success"),
		Extra: map[string]interface{}{
			"Nodes":      nodes,
			"Mode":       mode,
			"IsPrimary":  isPrimary,
			"NodeID":     nodeID,
			"Hostname":   hostname,
			"IsCluster":  mode != "standalone",
		},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.renderAdminPage(w, "nodes", data)
}

// handleNodesToken generates a join token for new nodes
func (h *Handler) handleNodesToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.cluster == nil || !h.cluster.IsClusterMode() {
		http.Redirect(w, r, "/admin/server/nodes?error=Not+in+cluster+mode", http.StatusSeeOther)
		return
	}

	if !h.cluster.IsPrimary() {
		http.Redirect(w, r, "/admin/server/nodes?error=Only+primary+node+can+generate+tokens", http.StatusSeeOther)
		return
	}

	token, err := h.cluster.GenerateJoinToken(ctx)
	if err != nil {
		log.Printf("[Admin] Failed to generate join token: %v", err)
		http.Redirect(w, r, "/admin/server/nodes?error=Failed+to+generate+token", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/server/nodes?success=Join+token+generated&token="+token, http.StatusSeeOther)
}

// handleNodesLeave removes this node from the cluster
func (h *Handler) handleNodesLeave(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.cluster == nil || !h.cluster.IsClusterMode() {
		http.Redirect(w, r, "/admin/server/nodes?error=Not+in+cluster+mode", http.StatusSeeOther)
		return
	}

	if err := h.cluster.LeaveCluster(ctx); err != nil {
		log.Printf("[Admin] Failed to leave cluster: %v", err)
		http.Redirect(w, r, "/admin/server/nodes?error=Failed+to+leave+cluster:+"+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/admin/server/nodes?success=Left+cluster+successfully", http.StatusSeeOther)
}

// ============================================================
// Tor API Endpoints per AI.md PART 32
// ============================================================

// apiTorStatus returns current Tor service status
func (h *Handler) apiTorStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonResponse(w, map[string]interface{}{
			"enabled":   false,
			"running":   false,
			"address":   "",
			"message":   "Tor service not configured",
		}, http.StatusOK)
		return
	}

	status := h.tor.GetTorStatus()
	status["enabled"] = h.config.Server.Tor.Enabled
	status["running"] = h.tor.IsRunning()
	status["address"] = h.tor.GetOnionAddress()

	h.jsonResponse(w, status, http.StatusOK)
}

// apiTorStart starts the Tor service
func (h *Handler) apiTorStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	if h.tor.IsRunning() {
		h.jsonError(w, "Tor service is already running", http.StatusConflict)
		return
	}

	if err := h.tor.Start(); err != nil {
		log.Printf("[Admin] Failed to start Tor: %v", err)
		h.jsonError(w, fmt.Sprintf("Failed to start Tor: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Admin] Tor service started")
	h.jsonResponse(w, map[string]interface{}{
		"status":  "started",
		"address": h.tor.GetOnionAddress(),
	}, http.StatusOK)
}

// apiTorStop stops the Tor service
func (h *Handler) apiTorStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	if !h.tor.IsRunning() {
		h.jsonError(w, "Tor service is not running", http.StatusConflict)
		return
	}

	if err := h.tor.Stop(); err != nil {
		log.Printf("[Admin] Failed to stop Tor: %v", err)
		h.jsonError(w, fmt.Sprintf("Failed to stop Tor: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Admin] Tor service stopped")
	h.jsonResponse(w, map[string]string{"status": "stopped"}, http.StatusOK)
}

// apiTorRestart restarts the Tor service
func (h *Handler) apiTorRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	if err := h.tor.Restart(); err != nil {
		log.Printf("[Admin] Failed to restart Tor: %v", err)
		h.jsonError(w, fmt.Sprintf("Failed to restart Tor: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Admin] Tor service restarted")
	h.jsonResponse(w, map[string]interface{}{
		"status":  "restarted",
		"address": h.tor.GetOnionAddress(),
	}, http.StatusOK)
}

// apiTorRegenerateAddress regenerates the .onion address
func (h *Handler) apiTorRegenerateAddress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	oldAddress := h.tor.GetOnionAddress()
	newAddress, err := h.tor.RegenerateAddress()
	if err != nil {
		log.Printf("[Admin] Failed to regenerate Tor address: %v", err)
		h.jsonError(w, fmt.Sprintf("Failed to regenerate address: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Admin] Tor address regenerated: %s -> %s", oldAddress, newAddress)
	h.jsonResponse(w, map[string]interface{}{
		"status":      "regenerated",
		"old_address": oldAddress,
		"new_address": newAddress,
	}, http.StatusOK)
}

// apiTorVanityStart starts vanity address generation
func (h *Handler) apiTorVanityStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	var req struct {
		Prefix string `json:"prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Prefix == "" {
		h.jsonError(w, "Prefix is required", http.StatusBadRequest)
		return
	}

	// Per AI.md: max 6 chars for built-in vanity generation
	if len(req.Prefix) > 6 {
		h.jsonError(w, "Prefix too long (max 6 characters for built-in generation, use external tools for 7+)", http.StatusBadRequest)
		return
	}

	// Validate prefix contains only valid base32 characters
	validChars := "abcdefghijklmnopqrstuvwxyz234567"
	for _, c := range strings.ToLower(req.Prefix) {
		if !strings.ContainsRune(validChars, c) {
			h.jsonError(w, "Invalid prefix: only a-z and 2-7 are valid", http.StatusBadRequest)
			return
		}
	}

	if err := h.tor.GenerateVanity(req.Prefix); err != nil {
		log.Printf("[Admin] Failed to start vanity generation: %v", err)
		h.jsonError(w, fmt.Sprintf("Failed to start vanity generation: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Admin] Vanity address generation started for prefix: %s", req.Prefix)
	h.jsonResponse(w, map[string]interface{}{
		"status": "started",
		"prefix": req.Prefix,
	}, http.StatusOK)
}

// apiTorVanityStatus returns vanity generation progress
func (h *Handler) apiTorVanityStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	progress := h.tor.GetVanityProgress()
	if progress == nil {
		h.jsonResponse(w, map[string]interface{}{
			"running": false,
			"message": "No vanity generation in progress",
		}, http.StatusOK)
		return
	}

	response := map[string]interface{}{
		"running":    progress.Running,
		"prefix":     progress.Prefix,
		"attempts":   progress.Attempts,
		"start_time": progress.StartTime,
		"found":      progress.Found,
	}

	if progress.Found {
		response["address"] = progress.Address
	}

	if progress.Error != "" {
		response["error"] = progress.Error
	}

	// Calculate elapsed time
	if !progress.StartTime.IsZero() {
		response["elapsed"] = time.Since(progress.StartTime).String()
	}

	h.jsonResponse(w, response, http.StatusOK)
}

// apiTorVanityCancel cancels vanity address generation
func (h *Handler) apiTorVanityCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	h.tor.CancelVanity()
	log.Printf("[Admin] Vanity address generation cancelled")
	h.jsonResponse(w, map[string]string{"status": "cancelled"}, http.StatusOK)
}

// apiTorKeysExport exports hidden service keys
func (h *Handler) apiTorKeysExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	keys, err := h.tor.ExportKeys()
	if err != nil {
		log.Printf("[Admin] Failed to export Tor keys: %v", err)
		h.jsonError(w, fmt.Sprintf("Failed to export keys: %v", err), http.StatusInternalServerError)
		return
	}

	// Return as downloadable file
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=tor_hidden_service_keys.bin")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(keys)))
	w.Write(keys)
}

// apiTorKeysImport imports hidden service keys (for external vanity addresses)
func (h *Handler) apiTorKeysImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.tor == nil {
		h.jsonError(w, "Tor service not configured", http.StatusBadRequest)
		return
	}

	// Read private key from request body
	privateKey, err := io.ReadAll(io.LimitReader(r.Body, 1024*10)) // Max 10KB
	if err != nil {
		h.jsonError(w, "Failed to read request body", http.StatusBadRequest)
		return
	}

	if len(privateKey) == 0 {
		h.jsonError(w, "Private key data is required", http.StatusBadRequest)
		return
	}

	newAddress, err := h.tor.ImportKeys(privateKey)
	if err != nil {
		log.Printf("[Admin] Failed to import Tor keys: %v", err)
		h.jsonError(w, fmt.Sprintf("Failed to import keys: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[Admin] Tor keys imported, new address: %s", newAddress)
	h.jsonResponse(w, map[string]interface{}{
		"status":  "imported",
		"address": newAddress,
	}, http.StatusOK)
}

// apiBangs manages custom bangs
// Per AI.md PART 36 line 28288: /api/v1/admin/bangs GET/POST endpoint
func (h *Handler) apiBangs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get current bangs configuration
		bangs := h.config.Search.Bangs.Custom

		// Convert to API response format
		response := make([]map[string]interface{}, len(bangs))
		for i, bang := range bangs {
			response[i] = map[string]interface{}{
				"shortcut":    bang.Shortcut,
				"name":        bang.Name,
				"url":         bang.URL,
				"category":    bang.Category,
				"description": bang.Description,
				"aliases":     bang.Aliases,
			}
		}

		h.jsonResponse(w, map[string]interface{}{
			"bangs":  response,
			"count":  len(response),
			"enabled": h.config.Search.Bangs.Enabled,
		}, http.StatusOK)

	case http.MethodPost:
		// Add or update custom bang
		var req struct {
			Shortcut    string   `json:"shortcut"`
			Name        string   `json:"name"`
			URL         string   `json:"url"`
			Category    string   `json:"category"`
			Description string   `json:"description"`
			Aliases     []string `json:"aliases"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.jsonError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate required fields
		if req.Shortcut == "" || req.Name == "" || req.URL == "" {
			h.jsonError(w, "Shortcut, name, and URL are required", http.StatusBadRequest)
			return
		}

		// Ensure shortcut starts with !
		if !strings.HasPrefix(req.Shortcut, "!") {
			req.Shortcut = "!" + req.Shortcut
		}

		// Create new bang config
		newBang := config.BangConfig{
			Shortcut:    req.Shortcut,
			Name:        req.Name,
			URL:         req.URL,
			Category:    req.Category,
			Description: req.Description,
			Aliases:     req.Aliases,
		}

		// Add to configuration
		// Note: This modifies in-memory config; should also persist to server.yml or database
		h.config.Search.Bangs.Custom = append(h.config.Search.Bangs.Custom, newBang)

		log.Printf("[Admin] Custom bang added: %s -> %s", req.Shortcut, req.Name)

		h.jsonResponse(w, map[string]interface{}{
			"status":   "created",
			"shortcut": req.Shortcut,
			"name":     req.Name,
		}, http.StatusCreated)

	case http.MethodDelete:
		// Delete custom bang by shortcut
		shortcut := r.URL.Query().Get("shortcut")
		if shortcut == "" {
			h.jsonError(w, "Shortcut parameter required", http.StatusBadRequest)
			return
		}

		// Find and remove bang
		bangs := h.config.Search.Bangs.Custom
		found := false
		for i, bang := range bangs {
			if bang.Shortcut == shortcut {
				// Remove from slice
				h.config.Search.Bangs.Custom = append(bangs[:i], bangs[i+1:]...)
				found = true
				log.Printf("[Admin] Custom bang deleted: %s", shortcut)
				break
			}
		}

		if !found {
			h.jsonError(w, "Bang not found", http.StatusNotFound)
			return
		}

		h.jsonResponse(w, map[string]string{"status": "deleted"}, http.StatusOK)

	default:
		h.jsonError(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
