package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// handleHome renders the home page
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.handleNotFound(w, r)
		return
	}

	data := NewPageData(s.config, "", "home")
	data.CSRFToken = s.getCSRFToken(r)

	// Add widget data if widgets are enabled
	if s.widgetManager != nil && s.widgetManager.IsEnabled() {
		data.WidgetsEnabled = true
		// Convert default widgets to JSON array string for JavaScript
		defaults := s.widgetManager.GetDefaultWidgets()
		data.DefaultWidgets = fmt.Sprintf(`["%s"]`, strings.Join(defaults, `","`))
	}

	if err := s.renderer.Render(w, "index", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
	}
}

// handleAbout renders the about page
func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	data := NewPageData(s.config, "About", "about")
	data.CSRFToken = s.getCSRFToken(r)

	// Get Tor address if enabled
	if s.config.Server.Tor.Enabled && s.torService != nil {
		data.TorAddress = s.torService.GetOnionAddress()
	}

	if err := s.renderer.Render(w, "about", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
	}
}

// handlePrivacy renders the privacy page
func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	data := NewPageData(s.config, "Privacy Policy", "privacy")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "privacy", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
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

	data := &ContactPageData{
		PageData: PageData{
			Title:       "Contact",
			Description: s.config.Server.Description,
			Page:        "contact",
			Theme:       "dark",
			Config:      s.config,
			CSRFToken:   s.getCSRFToken(r),
			BuildDate:   time.Now().Format(time.RFC3339),
		},
		CaptchaA:  int(captchaA.Int64()) + 1,
		CaptchaB:  int(captchaB.Int64()) + 1,
		CaptchaID: captchaID,
	}

	// Check for success message
	if r.URL.Query().Get("success") == "1" {
		data.ContactSent = true
	}

	if err := s.renderer.Render(w, "contact", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
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
	data := NewPageData(s.config, "Help", "help")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "help", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
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

// buildHealthInfo constructs the health information per AI.md spec
func (s *Server) buildHealthInfo() *HealthInfo {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	hostname, _ := getHostname()

	health := &HealthInfo{
		Status:    "healthy",
		Version:   getVersion(),
		Mode:      s.config.Server.Mode,
		Uptime:    formatDuration(time.Since(s.startTime)),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    make(map[string]string),
		Node: &NodeInfo{
			ID:       "standalone",
			Hostname: hostname,
		},
		Cluster: &ClusterInfo{
			Enabled: false,
			Status:  "disabled",
			Nodes:   1,
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

	// Add service checks
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
	}

	// Tor check
	if s.config.Server.Tor.Enabled {
		if s.torService != nil && s.torService.IsRunning() {
			health.Checks["tor"] = "ok"
		} else {
			health.Checks["tor"] = "unavailable"
		}
	}

	// Scheduler check
	if s.scheduler != nil {
		health.Checks["scheduler"] = "ok"
	}

	return health
}

// detectResponseFormat determines the response format from the request
func (s *Server) detectResponseFormat(r *http.Request) string {
	// Check for .txt extension
	if strings.HasSuffix(r.URL.Path, ".txt") {
		return "text/plain"
	}

	// Check Accept header
	accept := r.Header.Get("Accept")

	switch {
	case strings.Contains(accept, "application/json"):
		return "application/json"
	case strings.Contains(accept, "text/plain"):
		return "text/plain"
	case strings.Contains(accept, "text/html"):
		return "text/html"
	default:
		return "text/html"
	}
}

// respondHealthJSON responds with JSON health info
func (s *Server) respondHealthJSON(w http.ResponseWriter, health *HealthInfo) {
	w.Header().Set("Content-Type", "application/json")

	statusCode := http.StatusOK
	if health.Status == "unhealthy" || health.Status == "maintenance" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)

	data, _ := jsonMarshal(health)
	w.Write(data)
}

// respondHealthText responds with plain text health info
func (s *Server) respondHealthText(w http.ResponseWriter, health *HealthInfo) {
	w.Header().Set("Content-Type", "text/plain")

	statusCode := http.StatusOK
	if health.Status == "unhealthy" || health.Status == "maintenance" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)

	if health.Status == "healthy" {
		fmt.Fprint(w, "OK")
	} else {
		fmt.Fprintf(w, "ERROR: %s", health.Status)
	}
}

// respondHealthHTML responds with HTML health page
func (s *Server) respondHealthHTML(w http.ResponseWriter, r *http.Request, health *HealthInfo) {
	data := &HealthPageData{
		PageData: PageData{
			Title:       "Health",
			Description: s.config.Server.Description,
			Page:        "healthz",
			Theme:       "dark",
			Config:      s.config,
			BuildDate:   time.Now().Format(time.RFC3339),
		},
		Health: health,
	}

	if err := s.renderer.Render(w, "healthz", data); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "Template Error", err.Error())
	}
}

// handleNotFound renders a 404 error page
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.handleError(w, r, http.StatusNotFound, "Page Not Found", "The page you're looking for doesn't exist.")
}

// handleError renders an error page
func (s *Server) handleError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	w.WriteHeader(code)

	data := &ErrorPageData{
		PageData: PageData{
			Title:       title,
			Description: s.config.Server.Description,
			Page:        "error",
			Theme:       "dark",
			Config:      s.config,
			BuildDate:   time.Now().Format(time.RFC3339),
		},
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

// jsonMarshal marshals data to JSON with indentation
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

// handleAutocomplete handles autocomplete requests
// Per AI.md PART 36 line 28280: /autocomplete GET endpoint for autocomplete suggestions
func (s *Server) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

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
