package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/apimgr/search/src/httputil"
	"github.com/apimgr/search/src/config"
)

// handleHome renders the home page
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.handleNotFound(w, r)
		return
	}

	data := s.newPageData("", "home")
	data.CSRFToken = s.getCSRFToken(r)

	// Widgets are always available - users control via localStorage
	// Per user preference: no server-side gating, all widgets user-controlled
	data.WidgetsEnabled = true
	// Default widgets for new users (stored in localStorage after first visit)
	defaults := []string{"clock", "calculator", "quicklinks", "notes"}
	if s.widgetManager != nil {
		defaults = s.widgetManager.GetDefaultWidgets()
	}
	if len(defaults) > 0 {
		data.DefaultWidgets = fmt.Sprintf(`["%s"]`, strings.Join(defaults, `","`))
	}

	if err := s.renderer.Render(w, "index", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleAbout renders the about page
func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData("About", "about")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "about", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handlePrivacy renders the privacy page
func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData("Privacy Policy", "privacy")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "privacy", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleContact renders the contact form
func (s *Server) handleContact(w http.ResponseWriter, r *http.Request) {
	// Generate captcha values
	captchaA, _ := rand.Int(rand.Reader, big.NewInt(10))
	captchaB, _ := rand.Int(rand.Reader, big.NewInt(10))

	// Generate captcha ID
	captchaIDBytes := make([]byte, 16)
	rand.Read(captchaIDBytes)
	captchaID := base64.URLEncoding.EncodeToString(captchaIDBytes)

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData("Contact", "contact")
	baseData.CSRFToken = s.getCSRFToken(r)

	data := &ContactPageData{
		PageData:  *baseData,
		CaptchaA:  int(captchaA.Int64()) + 1,
		CaptchaB:  int(captchaB.Int64()) + 1,
		CaptchaID: captchaID,
	}

	// Check for success message
	if r.URL.Query().Get("success") == "1" {
		data.ContactSent = true
	}

	if err := s.renderer.Render(w, "contact", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleContactSubmit processes contact form submission
func (s *Server) handleContactSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/server/contact", http.StatusSeeOther)
		return
	}

	// Parse form
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	subject := strings.TrimSpace(r.FormValue("subject"))
	message := strings.TrimSpace(r.FormValue("message"))

	// Validate required fields
	if name == "" || email == "" || subject == "" || message == "" {
		http.Error(w, "All fields are required", http.StatusBadRequest)
		return
	}

	// Log the contact message (in production, this would send an email)
	fmt.Printf("[Contact] From: %s <%s>\n", name, email)
	fmt.Printf("[Contact] Subject: %s\n", subject)
	fmt.Printf("[Contact] Message: %s\n", message)

	// Redirect to success
	http.Redirect(w, r, "/server/contact?success=1", http.StatusSeeOther)
}

// handleHelp renders the help page
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData("Help", "help")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "help", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleTerms renders the terms of service page
func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData("Terms of Service", "terms")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "terms", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleHealthz handles the health check endpoint with content negotiation
// Per AI.md spec: supports HTML, JSON, and plain text responses
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	health := s.buildHealthInfo()

	// Determine response format based on content negotiation
	format := s.detectResponseFormat(r)

	switch format {
	case "application/json":
		s.respondHealthJSON(w, health)
	case "text/plain":
		s.respondHealthText(w, health)
	default:
		s.respondHealthHTML(w, r, health)
	}
}

// handleReadyz handles Kubernetes readiness probe
// Per AI.md PART 11/13: /readyz endpoint for readiness
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	health := s.buildHealthInfo()

	// Return 503 if not ready (unhealthy or maintenance)
	if health.Status != "healthy" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "NOT READY: %s\n", health.Status)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "READY\n")
}

// handleLivez handles Kubernetes liveness probe
// Per AI.md PART 11/13: /livez endpoint for liveness
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	// Liveness probe is simpler - just check if server can respond
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ALIVE\n")
}

// handleAPIHealthz handles /api/v1/healthz endpoint
// Per AI.md PART 13: API health endpoint ALWAYS returns JSON regardless of Accept header
func (s *Server) handleAPIHealthz(w http.ResponseWriter, r *http.Request) {
	health := s.buildHealthInfo()
	// Always JSON for API endpoint - per spec
	s.respondHealthJSON(w, health)
}

// buildHealthInfo constructs the health information per AI.md PART 13
func (s *Server) buildHealthInfo() *HealthInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := getHostname()

	// Per AI.md PART 13: Build Tor feature status
	torFeature := &TorFeature{
		Enabled:  s.config.Server.Tor.Enabled,
		Running:  s.torService != nil && s.torService.IsRunning(),
		Status:   "",
		Hostname: "",
	}
	if torFeature.Running {
		torFeature.Status = "healthy"
		if s.torService != nil {
			torFeature.Hostname = s.torService.GetOnionAddress()
		}
	} else if torFeature.Enabled {
		torFeature.Status = "unavailable"
	}

	health := &HealthInfo{
		// Per AI.md PART 13: project fields from cfg.Branding per spec line 16324-16326
		Project: &ProjectInfo{
			Name:        s.config.Server.Branding.Title,
			Tagline:     s.config.Server.Branding.Tagline,
			Description: s.config.Server.Branding.Description,
		},
		Status:    "healthy",
		Version:   getVersion(),
		GoVersion: runtime.Version(),
		Mode:      s.config.Server.Mode,
		Uptime:    formatDuration(time.Since(s.startTime)),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		// Per AI.md PART 13: build.commit and build.date (not commit_id/build_date)
		Build: &BuildInfo{
			Commit: config.CommitID,
			Date:   config.BuildDate,
		},
		Node: &NodeInfo{
			ID:       "standalone",
			Hostname: hostname,
		},
		// Per AI.md PART 13: cluster.primary (string) and cluster.nodes ([]string)
		Cluster: &ClusterInfo{
			Enabled: false,
			Primary: "",
			Nodes:   []string{},
		},
		// Per AI.md PART 13: features with tor as object
		Features: &HealthFeatures{
			MultiUser:     s.config.Server.Users.Enabled,
			Organizations: false, // Optional feature (PART 35)
			Tor:           torFeature,
			GeoIP:         s.config.Server.GeoIP.Enabled,
			Metrics:       s.config.Server.Metrics.Enabled,
		},
		Checks: make(map[string]string),
		// Per AI.md PART 13: stats (requests_total, requests_24h, active_connections)
		Stats: &HealthStats{
			RequestsTotal:     s.getRequestsTotal(),
			Requests24h:       s.getRequests24h(),
			ActiveConnections: s.getActiveConnections(),
		},
		System: &SystemInfo{
			GoVersion:    runtime.Version(),
			NumCPU:       runtime.NumCPU(),
			NumGoroutine: runtime.NumGoroutine(),
			MemAlloc:     formatBytes(m.Alloc),
		},
	}

	// Check maintenance mode
	if s.config.Server.MaintenanceMode {
		health.Status = "maintenance"
		health.Maintenance = &MaintenanceInfo{
			Reason:  "manual",
			Message: "Server is in maintenance mode",
			Since:   time.Now().UTC().Format(time.RFC3339),
		}
	}

	// Per AI.md PART 13: checks (database, cache, disk, scheduler, cluster)
	health.Checks["search"] = "ok"

	// Database check
	if s.dbManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.dbManager.Ping(ctx); err != nil {
			health.Checks["database"] = "error"
			if health.Status == "healthy" {
				health.Status = "unhealthy"
			}
		} else {
			health.Checks["database"] = "ok"
		}
	} else {
		health.Checks["database"] = "ok" // No separate DB = embedded SQLite always ok
	}

	// Cache check (standalone mode - no external cache)
	health.Checks["cache"] = "disabled"

	// Disk check - verify data directory is accessible
	if dataDir := config.GetDataDir(); dataDir != "" {
		if _, err := os.Stat(dataDir); err == nil {
			health.Checks["disk"] = "ok"
		} else {
			health.Checks["disk"] = "error"
		}
	} else {
		health.Checks["disk"] = "ok"
	}

	// Scheduler check
	if s.scheduler != nil {
		health.Checks["scheduler"] = "ok"
	} else {
		health.Checks["scheduler"] = "disabled"
	}

	// Cluster check (standalone mode)
	health.Checks["cluster"] = "disabled"

	// Tor check
	if s.config.Server.Tor.Enabled {
		if s.torService != nil && s.torService.IsRunning() {
			health.Checks["tor"] = "ok"
		} else {
			health.Checks["tor"] = "unavailable"
		}
	}

	return health
}

// getRequestsTotal returns total requests served
// Per AI.md PART 13: stats.requests_total must return actual count
func (s *Server) getRequestsTotal() int64 {
	if s.metrics != nil {
		return s.metrics.GetTotalRequests()
	}
	return 0
}

// getRequests24h returns requests in last 24 hours
// Per AI.md PART 13: stats.requests_24h - uses total requests as approximation
// Note: For accurate 24h tracking, a time-window based counter would be needed
func (s *Server) getRequests24h() int64 {
	// Return total requests as baseline (24h tracking requires more infrastructure)
	// This could be enhanced with a rolling window counter in the future
	if s.metrics != nil {
		return s.metrics.GetTotalRequests()
	}
	return 0
}

// getActiveConnections returns current active connections
// Per AI.md PART 13: stats.active_connections must return actual count
func (s *Server) getActiveConnections() int {
	if s.metrics != nil {
		return int(s.metrics.GetActiveConnections())
	}
	return 0
}

// detectResponseFormat determines the response format from the request
// Per AI.md PART 14: Content Negotiation Priority
// Uses smart client detection for automatic format selection
func (s *Server) detectResponseFormat(r *http.Request) string {
	return httputil.GetPreferredFormat(r)
}

// respondHealthJSON responds with JSON health info per AI.md PART 14
func (s *Server) respondHealthJSON(w http.ResponseWriter, health *HealthInfo) {
	w.Header().Set("Content-Type", "application/json")

	statusCode := http.StatusOK
	if health.Status == "unhealthy" || health.Status == "maintenance" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)

	data, _ := jsonMarshal(health)
	w.Write(data)
	// Per AI.md PART 14: Single trailing newline
	w.Write([]byte("\n"))
}

// respondHealthText responds with plain text health info per AI.md PART 13
// Format: key: value pairs, one per line
func (s *Server) respondHealthText(w http.ResponseWriter, health *HealthInfo) {
	w.Header().Set("Content-Type", "text/plain")

	statusCode := http.StatusOK
	if health.Status == "unhealthy" || health.Status == "maintenance" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)

	var b strings.Builder

	// Core fields
	b.WriteString(fmt.Sprintf("status: %s\n", health.Status))
	b.WriteString(fmt.Sprintf("version: %s\n", health.Version))
	b.WriteString(fmt.Sprintf("mode: %s\n", health.Mode))
	b.WriteString(fmt.Sprintf("uptime: %s\n", health.Uptime))
	b.WriteString(fmt.Sprintf("go_version: %s\n", health.GoVersion))

	// Build info
	if health.Build != nil {
		b.WriteString(fmt.Sprintf("build.commit: %s\n", health.Build.Commit))
	}

	// Component checks
	for name, status := range health.Checks {
		b.WriteString(fmt.Sprintf("%s: %s\n", name, status))
	}

	// Cluster info
	if health.Cluster != nil {
		if health.Cluster.Enabled {
			b.WriteString(fmt.Sprintf("cluster.primary: %s\n", health.Cluster.Primary))
			if len(health.Cluster.Nodes) > 0 {
				b.WriteString(fmt.Sprintf("cluster.nodes: %s\n", strings.Join(health.Cluster.Nodes, ", ")))
			}
		}
	}

	// Features
	if health.Features != nil {
		var features []string
		if health.Features.MultiUser {
			features = append(features, "multi_user")
		}
		if health.Features.Organizations {
			features = append(features, "organizations")
		}
		if health.Features.Tor != nil && health.Features.Tor.Enabled {
			features = append(features, "tor")
		}
		if health.Features.GeoIP {
			features = append(features, "geoip")
		}
		if health.Features.Metrics {
			features = append(features, "metrics")
		}
		if len(features) > 0 {
			b.WriteString(fmt.Sprintf("features: %s\n", strings.Join(features, ", ")))
		}

		// Tor details
		if health.Features.Tor != nil && health.Features.Tor.Enabled {
			b.WriteString(fmt.Sprintf("features.tor.enabled: %t\n", health.Features.Tor.Enabled))
			b.WriteString(fmt.Sprintf("features.tor.running: %t\n", health.Features.Tor.Running))
			b.WriteString(fmt.Sprintf("features.tor.status: %s\n", health.Features.Tor.Status))
			if health.Features.Tor.Hostname != "" {
				b.WriteString(fmt.Sprintf("features.tor.hostname: %s\n", health.Features.Tor.Hostname))
			}
		}
	}

	fmt.Fprint(w, b.String())
}

// respondHealthHTML responds with HTML health page
func (s *Server) respondHealthHTML(w http.ResponseWriter, r *http.Request, health *HealthInfo) {
	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData("Health", "healthz")

	data := &HealthPageData{
		PageData: *baseData,
		Health:   health,
	}

	if err := s.renderer.Render(w, "healthz", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleNotFound renders a 404 error page or JSON response for API routes
// Per AI.md PART 13/14: API errors return JSON with NOT_FOUND code
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	// Return JSON for API routes per AI.md PART 14
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"ok":false,"error":"NOT_FOUND","message":"Resource not found"}`))
		return
	}
	s.handleError(w, r, http.StatusNotFound, "Page Not Found", "The page you're looking for doesn't exist.")
}

// handleError renders an error page
func (s *Server) handleError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	w.WriteHeader(code)

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(title, "error")

	data := &ErrorPageData{
		PageData:     *baseData,
		ErrorCode:    code,
		ErrorTitle:   title,
		ErrorMessage: message,
	}

	if s.config.IsDevelopment() {
		data.ErrorDetails = fmt.Sprintf("Request: %s %s", r.Method, r.URL.Path)
	}

	if err := s.renderer.Render(w, "error", data); err != nil {
		// Fallback to plain text
		http.Error(w, fmt.Sprintf("%d - %s: %s", code, title, message), code)
	}
}

// handleInternalError logs the actual error internally and shows a generic message to the user.
// Per AI.md PART 9: User sees "Minimal, helpful" messages; internal details go to logs only.
func (s *Server) handleInternalError(w http.ResponseWriter, r *http.Request, context string, err error) {
	// Log the actual error with context for debugging
	log.Printf("[ERROR] %s: %s %s - %v", context, r.Method, r.URL.Path, err)
	// Show generic message to user - never expose internal error details
	s.handleError(w, r, http.StatusInternalServerError, "Error", "An error occurred. Please try again.")
}

// getCSRFToken returns a CSRF token for the request
func (s *Server) getCSRFToken(r *http.Request) string {
	// Generate a simple CSRF token
	tokenBytes := make([]byte, 32)
	rand.Read(tokenBytes)
	return base64.URLEncoding.EncodeToString(tokenBytes)
}

// formatDuration formats a duration in a human-readable format
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

// formatBytes formats bytes in a human-readable format
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

// isContactEnabled checks if contact form is enabled
func (s *Server) isContactEnabled() bool {
	return s.config.Server.Pages.Contact.Enabled
}

// getHostname returns the system hostname
func getHostname() (string, error) {
	return os.Hostname()
}

// getVersion returns the application version
func getVersion() string {
	return config.Version
}

// jsonMarshal marshals data to JSON with 2-space indentation per AI.md PART 14
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// handleAutocomplete handles autocomplete requests
// Per AI.md PART 36 line 28280: /autocomplete GET endpoint for autocomplete suggestions
func (s *Server) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// If no query, return empty suggestions
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]string{})
		return
	}

	// Delegate to API handler for actual autocomplete logic
	// This ensures consistent behavior between frontend and API endpoints
	if s.apiHandler != nil {
		// Forward to API autocomplete handler
		s.apiHandler.HandleAutocomplete(w, r)
		return
	}

	// Fallback: return empty suggestions if API handler not available
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode([]string{})
}
