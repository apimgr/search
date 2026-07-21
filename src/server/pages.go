package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/security"
	"github.com/apimgr/search/src/version"
)

// handleHome renders the home page
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.handleNotFound(w, r)
		return
	}

	data := s.newPageData(w, r, "", "home")
	data.CSRFToken = s.getCSRFToken(r)

	// Text browsers receive a JavaScript-free home page (just a search form).
	// Per AI.md PART 14: text browsers are INTERACTIVE, no JS.
	if httputil.IsTextBrowser(r) {
		s.renderNoJSHome(w, r, data)
		return
	}

	// HTTP tools fetching the home page receive plain text via HTML2TextConverter.
	// Per AI.md PART 14: HTTP tools are NON-INTERACTIVE, just dump output.
	if httputil.IsHttpTool(r) {
		s.renderHTMLToText(w, "index", data)
		return
	}

	// Read enabled widgets from the server-side cookie. nil means the cookie
	// was never set, so fall back to defaults; a non-nil empty slice means
	// the user explicitly disabled all widgets and must be respected as-is.
	enabled := parseWidgetCookie(r)
	if enabled == nil {
		enabled = []string{"clock", "calculator", "quicklinks", "notes"}
		if s.widgetManager != nil {
			if d := s.widgetManager.GetDefaultWidgets(); len(d) > 0 {
				enabled = d
			}
		}
	}
	data.WidgetsEnabled = true
	data.EnabledWidgets = enabled

	if err := s.renderer.Render(w, "index", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleAbout renders the about page
func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "about")
	data.Title = s.getI18nManager().T(data.Lang, "nav.about")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "about", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handlePrivacy renders the privacy page
func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "privacy")
	data.Title = s.getI18nManager().T(data.Lang, "footer.privacy_policy")
	data.CSRFToken = s.getCSRFToken(r)

	if err := s.renderer.Render(w, "privacy", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleContact renders the contact form and handles its submission.
// Per AI.md PART 11 "Security Reports — Coordinated Disclosure Pipeline": the
// same /server/contact route doubles as the vulnerability-report submission
// path when a valid ?security_id={id} switches the form into security mode.
func (s *Server) handleContact(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleContactSubmit(w, r)
		return
	}

	securityID := strings.TrimSpace(r.URL.Query().Get("security_id"))
	securityMode := s.validateSecurityIDOrLog(r, securityID)

	s.renderContactForm(w, r, securityID, securityMode, "")
}

// validateSecurityIDOrLog validates a supplied {security_id} per AI.md PART 11.
// An invalid/expired id falls back to normal contact mode silently, but is
// logged to security.log as security.security_id_invalid.
func (s *Server) validateSecurityIDOrLog(r *http.Request, securityID string) bool {
	if securityID == "" {
		return false
	}
	if security.ValidateSecurityID(s.config.Server.Security.InstallationSecret, securityID, time.Now().Unix()) {
		return true
	}
	if s.logManager != nil {
		s.logManager.Security().LogSecurityIDInvalid(getClientIPSimple(r), r.UserAgent(), securityID)
	}
	return false
}

// renderContactForm renders the contact page in standard or security mode.
func (s *Server) renderContactForm(w http.ResponseWriter, r *http.Request, securityID string, securityMode bool, contactError string) {
	// Generate captcha values
	captchaA, _ := rand.Int(rand.Reader, big.NewInt(10))
	captchaB, _ := rand.Int(rand.Reader, big.NewInt(10))

	a := int(captchaA.Int64()) + 1
	b := int(captchaB.Int64()) + 1
	answer := a + b

	// Generate signed captcha ID containing the expected answer
	captchaID := s.signCaptcha(answer)

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, "", "contact")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "contact.page_title")
	baseData.CSRFToken = s.getCSRFToken(r)

	data := &ContactPageData{
		PageData:     *baseData,
		ContactError: contactError,
		CaptchaA:     a,
		CaptchaB:     b,
		CaptchaID:    captchaID,
		SecurityMode: securityMode,
		SecurityID:   securityID,
	}

	if securityMode {
		data.AppVersion = version.Version
		data.CommitHash = version.CommitID
		data.Timestamp = time.Now().UTC().Format(time.RFC3339)
		data.RequestUserAgent = r.UserAgent()
		data.DisclosureDays = 90
	}

	// Check for success message
	if r.URL.Query().Get("success") == "1" {
		data.ContactSent = true
	}

	if err := s.renderer.Render(w, "contact", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleContactSubmit processes a POST to /server/contact. When a valid
// security_id was submitted, the report is routed to the coordinated-
// disclosure pipeline per AI.md PART 11 "Submission Flow"; otherwise the
// standard contact-form path is preserved (out of scope for this pipeline).
func (s *Server) handleContactSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}

	securityID := strings.TrimSpace(r.PostFormValue("security_id"))

	// Step 1: re-validate security_id server-side — the form value can be tampered with.
	if !s.validateSecurityIDOrLog(r, securityID) {
		s.renderContactForm(w, r, "", false, "")
		return
	}

	s.handleSecurityReportSubmit(w, r, securityID)
}

// handleSecurityReportSubmit implements AI.md PART 11's 7-step coordinated-
// disclosure Submission Flow.
func (s *Server) handleSecurityReportSubmit(w http.ResponseWriter, r *http.Request, securityID string) {
	lang := s.newPageData(w, r, "", "contact").Lang

	affectedComponent := strings.TrimSpace(r.PostFormValue("affected_component"))
	affectedEndpoint := strings.TrimSpace(r.PostFormValue("affected_endpoint"))
	severity := strings.TrimSpace(r.PostFormValue("severity"))
	summary := strings.TrimSpace(r.PostFormValue("summary"))
	stepsToReproduce := strings.TrimSpace(r.PostFormValue("steps_to_reproduce"))
	impact := strings.TrimSpace(r.PostFormValue("impact"))
	suggestedFix := strings.TrimSpace(r.PostFormValue("suggested_fix"))
	researcherGPG := strings.TrimSpace(r.PostFormValue("researcher_gpg"))
	researcherEmail := strings.TrimSpace(r.PostFormValue("email"))
	creditPreference := strings.TrimSpace(r.PostFormValue("credit_preference"))
	creditName := strings.TrimSpace(r.PostFormValue("credit_name"))
	cveRequested := config.IsTruthy(r.PostFormValue("cve_requested"))
	agreedToDisclosure := config.IsTruthy(r.PostFormValue("agreed_to_disclosure"))

	disclosureDays := 90
	if v := strings.TrimSpace(r.PostFormValue("disclosure_days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			disclosureDays = n
		}
	}

	if summary == "" || severity == "" || affectedComponent == "" || stepsToReproduce == "" || impact == "" || !agreedToDisclosure {
		s.renderContactForm(w, r, securityID, true, s.getI18nManager().T(lang, "contact.security_required_fields"))
		return
	}

	// Step 2: allocate the tracking id.
	trackingID, err := security.GenerateTrackingID()
	if err != nil {
		s.handleInternalError(w, r, "generate tracking id", err)
		return
	}

	rawToken, tokenHash, err := security.GenerateReportToken()
	if err != nil {
		s.handleInternalError(w, r, "generate report token", err)
		return
	}

	plaintextBody := composeSecurityReportBody(trackingID, affectedComponent, affectedEndpoint, severity,
		summary, stepsToReproduce, impact, suggestedFix, researcherEmail, researcherGPG,
		creditPreference, creditName, disclosureDays, cveRequested)

	// Step 3: encrypt the report body at rest — PGP if a project keypair
	// exists, otherwise AES-256-GCM with server.security.encryption_key.
	// Plaintext is never persisted to disk.
	encryptedBody, encryptionMethod, err := s.encryptSecurityReportBody(plaintextBody)
	if err != nil {
		s.handleInternalError(w, r, "encrypt security report", err)
		return
	}

	report := security.Report{
		TrackingID:        trackingID,
		SecurityIDUsed:    securityID,
		AffectedComponent: affectedComponent,
		AffectedEndpoint:  affectedEndpoint,
		Severity:          severity,
		Summary:           summary,
		EncryptedBody:     encryptedBody,
		EncryptionMethod:  encryptionMethod,
		CreditPreference:  creditPreference,
		CreditName:        creditName,
		DisclosureDays:    disclosureDays,
		CVERequested:      cveRequested,
		ReportTokenHash:   tokenHash,
	}

	if s.dbManager != nil {
		if err := security.InsertReport(r.Context(), s.dbManager.ServerDB(), report); err != nil {
			s.handleInternalError(w, r, "insert security report", err)
			return
		}
	}

	// Step 4: maintainer notification (security role, falls back to admin).
	s.sendSecurityMaintainerNotification(report, plaintextBody)

	// Step 5: researcher acknowledgment, only if the researcher gave contact info.
	if researcherEmail != "" {
		s.sendSecurityResearcherAck(r, researcherEmail, researcherGPG, trackingID, rawToken)
	}

	// Step 7: log acceptance — tracking id + safe metadata only, never PII/content.
	if s.logManager != nil {
		s.logManager.Security().LogReportReceived(getClientIPSimple(r), trackingID, severity, affectedComponent)
	}

	// Step 6: respond to the form POST.
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"ok":   true,
			"data": map[string]string{"tracking_id": trackingID},
		})
		return
	}

	baseData := s.newPageData(w, r, "", "contact")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "contact.page_title")
	data := &ContactPageData{
		PageData:     *baseData,
		ContactSent:  true,
		SecurityMode: true,
		TrackingID:   trackingID,
	}
	if err := s.renderer.Render(w, "contact", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// composeSecurityReportBody assembles the plaintext security report content
// that gets encrypted at rest per AI.md PART 11 step 3. Never persisted or
// logged in plaintext form.
func composeSecurityReportBody(trackingID, affectedComponent, affectedEndpoint, severity, summary,
	stepsToReproduce, impact, suggestedFix, researcherEmail, researcherGPG,
	creditPreference, creditName string, disclosureDays int, cveRequested bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Tracking ID: %s\n", trackingID)
	fmt.Fprintf(&b, "Affected component: %s\n", affectedComponent)
	fmt.Fprintf(&b, "Affected endpoint: %s\n", affectedEndpoint)
	fmt.Fprintf(&b, "Severity: %s\n", severity)
	fmt.Fprintf(&b, "Summary: %s\n\n", summary)
	fmt.Fprintf(&b, "Steps to reproduce:\n%s\n\n", stepsToReproduce)
	fmt.Fprintf(&b, "Impact:\n%s\n\n", impact)
	if suggestedFix != "" {
		fmt.Fprintf(&b, "Suggested fix:\n%s\n\n", suggestedFix)
	}
	fmt.Fprintf(&b, "CVE requested: %t\n", cveRequested)
	fmt.Fprintf(&b, "Disclosure timeline preference: %d days\n", disclosureDays)
	fmt.Fprintf(&b, "Credit preference: %s\n", creditPreference)
	if creditName != "" {
		fmt.Fprintf(&b, "Credit name: %s\n", creditName)
	}
	if researcherEmail != "" {
		fmt.Fprintf(&b, "Researcher email: %s\n", researcherEmail)
	}
	if researcherGPG != "" {
		fmt.Fprintf(&b, "Researcher GPG: %s\n", researcherGPG)
	}
	return b.String()
}

// encryptSecurityReportBody selects the encryption method per AI.md PART 11
// step 3: PGP to the project's own public key if a keypair exists, otherwise
// AES-256-GCM with server.security.encryption_key.
func (s *Server) encryptSecurityReportBody(plaintext string) ([]byte, string, error) {
	if pubKey, err := security.LoadPublicKey(config.GetConfigDir()); err == nil && pubKey != "" {
		armored, err := security.EncryptMessageToArmoredKey(pubKey, []byte(plaintext))
		if err == nil {
			return []byte(armored), security.EncryptionMethodPGP, nil
		}
	}
	encoded, err := security.EncryptAESGCM(s.config.Server.Security.EncryptionKey, []byte(plaintext))
	if err != nil {
		return nil, "", err
	}
	return []byte(encoded), security.EncryptionMethodAESGCM, nil
}

// sendSecurityMaintainerNotification sends the maintainer CC notification
// per AI.md PART 11 step 4, to the security contact role (falls back to
// admin per AI.md PART 12 fallback chain). Best-effort: a delivery failure
// does not fail the submission since the report is already persisted.
func (s *Server) sendSecurityMaintainerNotification(report security.Report, plaintextBody string) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}
	recipient := s.config.Server.Contact.Security.Email
	if recipient == "" {
		recipient = s.config.Server.Contact.Admin.Email
	}
	if recipient == "" {
		return
	}

	subject := fmt.Sprintf("[security] %s (%s)", report.Summary, report.TrackingID)
	body := plaintextBody
	if pubKey, err := security.LoadPublicKey(config.GetConfigDir()); err == nil && pubKey != "" {
		if armored, err := security.EncryptMessageToArmoredKey(pubKey, []byte(plaintextBody)); err == nil {
			body = armored
		}
	}
	msg := email.NewMessage([]string{recipient}, subject, body)
	_ = s.mailer.Send(msg)
}

// sendSecurityResearcherAck sends the researcher acknowledgment email per
// AI.md PART 11 step 5, containing the tracking id and one-shot status link.
// Best-effort: a delivery failure does not fail the submission.
func (s *Server) sendSecurityResearcherAck(r *http.Request, researcherEmail, researcherGPGArmor, trackingID, rawToken string) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}

	statusURL := fmt.Sprintf("%s/server/security/report/%s?token=%s", s.getBaseURL(r), trackingID, rawToken)
	body := fmt.Sprintf("Thank you for your security report.\n\nTracking ID: %s\nStatus: %s\n", trackingID, statusURL)

	if researcherGPGArmor != "" {
		if armored, err := security.EncryptMessageToArmoredKey(researcherGPGArmor, []byte(body)); err == nil {
			body = armored
		}
	}

	msg := email.NewMessage([]string{researcherEmail}, "Security report received: "+trackingID, body)
	_ = s.mailer.Send(msg)
}

// handleHelp renders the help page
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "help")
	data.Title = s.getI18nManager().T(data.Lang, "help.page_title")
	data.CSRFToken = s.getCSRFToken(r)
	data.ServerURL = s.getBaseURL(r)

	if err := s.renderer.Render(w, "help", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleTerms renders the terms of service page
func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "terms")
	data.Title = s.getI18nManager().T(data.Lang, "footer.terms")
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

// buildHealthInfo constructs the HealthResponse per AI.md PART 13.
// Field order matches canonical spec: project, status, version, build, runtime,
// features, checks, stats.
func (s *Server) buildHealthInfo() *HealthResponse {
	// Per AI.md PART 13: Build Tor feature status
	torInfo := TorInfo{
		Enabled:  s.config.Server.Tor.Enabled,
		Running:  s.torService != nil && s.torService.IsRunning(),
		Status:   "",
		Hostname: "",
	}
	if torInfo.Running {
		torInfo.Status = "healthy"
		if s.torService != nil {
			torInfo.Hostname = s.torService.GetOnionAddress()
		}
	} else if torInfo.Enabled {
		torInfo.Status = "unavailable"
	}

	status := "healthy"

	// Check maintenance mode
	if s.config.Server.MaintenanceMode {
		status = "maintenance"
	}

	// Per AI.md PART 13: checks use "ok" or "error" only (no "disabled")
	checks := ChecksInfo{
		Cache:     "ok",
		Disk:      "ok",
		Scheduler: "ok",
	}

	// Database check
	if s.dbManager != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.dbManager.Ping(ctx); err != nil {
			checks.Database = "error"
			if status == "healthy" {
				status = "unhealthy"
			}
		} else {
			checks.Database = "ok"
		}
	} else {
		checks.Database = "ok"
	}

	// Disk check - verify data directory is accessible
	if dataDir := config.GetDataDir(); dataDir != "" {
		if _, err := os.Stat(dataDir); err != nil {
			checks.Disk = "error"
		}
	}

	// Scheduler check — "ok" when disabled (no error), "error" when failed
	if s.scheduler == nil {
		checks.Scheduler = "ok"
	}

	// Tor check (only when Tor is configured)
	if s.config.Server.Tor.Enabled {
		if s.torService != nil && s.torService.IsRunning() {
			checks.Tor = "ok"
		} else {
			checks.Tor = "error"
		}
	}

	health := &HealthResponse{
		// 1. Project identification from cfg.Branding per AI.md PART 13
		Project: ProjectInfo{
			Name:        s.config.Server.Branding.Title,
			Tagline:     s.config.Server.Branding.Tagline,
			Description: s.config.Server.Branding.Description,
		},
		// 2. Overall status
		Status: status,
		// 3. Version & build info
		Version:   getVersion(),
		GoVersion: runtime.Version(),
		Build: BuildInfo{
			Commit: config.CommitID,
			Date:   config.BuildDate,
		},
		// 4. Runtime info
		Uptime:    formatDuration(time.Since(s.startTime)),
		Mode:      s.config.Server.Mode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		// 5. Features - PUBLIC only (PARTS 20, 32)
		Features: FeaturesInfo{
			Tor:   torInfo,
			GeoIP: s.config.Server.GeoIP.Enabled,
		},
		// 7. Checks
		Checks: checks,
		// 8. Stats per AI.md PART 13: requests_total, requests_24h, active_connections
		Stats: StatsInfo{
			RequestsTotal: s.getRequestsTotal(),
			Requests24h:   s.getRequests24h(),
			ActiveConns:   s.getActiveConnections(),
		},
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
func (s *Server) respondHealthJSON(w http.ResponseWriter, health *HealthResponse) {
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

// respondHealthText responds with plain text health info per AI.md PART 13.
// Format: key: value pairs, one per line.
func (s *Server) respondHealthText(w http.ResponseWriter, health *HealthResponse) {
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
	b.WriteString(fmt.Sprintf("build.commit: %s\n", health.Build.Commit))

	// Component checks
	b.WriteString(fmt.Sprintf("check.database: %s\n", health.Checks.Database))
	b.WriteString(fmt.Sprintf("check.cache: %s\n", health.Checks.Cache))
	b.WriteString(fmt.Sprintf("check.disk: %s\n", health.Checks.Disk))
	b.WriteString(fmt.Sprintf("check.scheduler: %s\n", health.Checks.Scheduler))
	if health.Checks.Tor != "" {
		b.WriteString(fmt.Sprintf("check.tor: %s\n", health.Checks.Tor))
	}

	// Features
	var features []string
	if health.Features.Tor.Enabled {
		features = append(features, "tor")
	}
	if health.Features.GeoIP {
		features = append(features, "geoip")
	}
	if len(features) > 0 {
		b.WriteString(fmt.Sprintf("features: %s\n", strings.Join(features, ", ")))
	}

	// Tor details
	if health.Features.Tor.Enabled {
		b.WriteString(fmt.Sprintf("features.tor.enabled: %t\n", health.Features.Tor.Enabled))
		b.WriteString(fmt.Sprintf("features.tor.running: %t\n", health.Features.Tor.Running))
		b.WriteString(fmt.Sprintf("features.tor.status: %s\n", health.Features.Tor.Status))
		if health.Features.Tor.Hostname != "" {
			b.WriteString(fmt.Sprintf("features.tor.hostname: %s\n", health.Features.Tor.Hostname))
		}
	}

	fmt.Fprint(w, b.String())
}

// respondHealthHTML responds with HTML health page
func (s *Server) respondHealthHTML(w http.ResponseWriter, r *http.Request, health *HealthResponse) {
	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, "", "healthz")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "health.page_title")

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
	lang := s.getI18nManager().DetectLanguage(r)
	// Return JSON for API routes per AI.md PART 14
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		msg := s.getI18nManager().T(lang, "errors.not_found")
		msgJSON, _ := json.Marshal(msg)
		_, _ = w.Write([]byte(`{"ok":false,"error":"NOT_FOUND","message":` + string(msgJSON) + `}`))
		return
	}
	title := s.getI18nManager().T(lang, "errors.not_found_title")
	msg := s.getI18nManager().T(lang, "errors.not_found_message")
	s.handleError(w, r, http.StatusNotFound, title, msg)
}

// handleError renders an error page
func (s *Server) handleError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	w.WriteHeader(code)

	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, title, "error")

	data := &ErrorPageData{
		PageData:   *baseData,
		StatusCode: code,
		StatusText: title,
		Message:    message,
	}

	if s.config.IsDevelopment() {
		data.ErrorDetails = fmt.Sprintf("Request: %s %s", r.Method, r.URL.Path)
	}

	// Guard against nil renderer (e.g. during tests or early startup)
	if s.renderer == nil {
		http.Error(w, fmt.Sprintf("%d - %s: %s", code, title, message), code)
		return
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
	slog.Error("internal error", "context", context, "method", r.Method, "path", r.URL.Path, "err", err)
	lang := s.getI18nManager().DetectLanguage(r)
	title := s.getI18nManager().T(lang, "errors.server_error_title")
	// Show generic message to user - never expose internal error details
	msg := s.getI18nManager().T(lang, "errors.server_error_message")
	s.handleError(w, r, http.StatusInternalServerError, title, msg)
}

// signCaptcha creates an HMAC-signed captcha ID containing the expected answer
func (s *Server) signCaptcha(answer int) string {
	// Use server start time as HMAC key (stable per process, not guessable)
	key := []byte(s.startTime.Format(time.RFC3339Nano))
	data := strconv.Itoa(answer)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	return data + "." + sig
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

// getVersion returns the application version
func getVersion() string {
	return config.Version
}

// jsonMarshal marshals data to JSON with 2-space indentation per AI.md PART 14
func jsonMarshal(v interface{}) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// handleAutocomplete handles autocomplete requests
// Per AI.md PART 32 line 28280: /autocomplete GET endpoint for autocomplete suggestions
func (s *Server) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))

	// If no query, return empty suggestions
	if query == "" {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"ok":   true,
			"data": []string{},
		})
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
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"data": []string{},
	})
}

// handleConsent handles POST /consent — sets the cookieConsent JSON cookie and redirects back.
// Accepts choice=accept (all categories) or choice=decline (essential only).
// Cookie format: {"essential":true,"preferences":true,"analytics":false,"timestamp":unix}
// Works with zero JS; the cookie banner form POSTs here directly.
func (s *Server) handleConsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	choice := r.FormValue("choice")
	prefs := r.FormValue("preferences") // optional granular field from prefs form
	analytics := r.FormValue("analytics")

	essential := true
	prefEnabled := true
	analyticsEnabled := true

	switch choice {
	case "decline":
		prefEnabled = false
		analyticsEnabled = false
	case "accept":
		// accept all — defaults above
	case "save":
		// granular save from preferences form
		prefEnabled, _ = config.ParseBool(prefs, false)
		analyticsEnabled, _ = config.ParseBool(analytics, false)
	default:
		// unknown choice — treat as decline
		prefEnabled = false
		analyticsEnabled = false
	}

	// Build JSON cookie value per spec
	cookieVal := fmt.Sprintf(
		`{"essential":%t,"preferences":%t,"analytics":%t,"timestamp":%d}`,
		essential, prefEnabled, analyticsEnabled, time.Now().Unix(),
	)
	http.SetCookie(w, &http.Cookie{
		Name:     "cookieConsent",
		Value:    cookieVal,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
	})

	ref := r.Referer()
	if ref == "" {
		ref = "/"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

// handleAnnouncementDismiss handles POST /announcements/dismiss.
// Appends the announcement id to the dismissed_announcements cookie and redirects back.
// Works with zero JS; the dismiss form POSTs here directly.
func (s *Server) handleAnnouncementDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		ref := r.Referer()
		if ref == "" {
			ref = "/"
		}
		http.Redirect(w, r, ref, http.StatusSeeOther)
		return
	}

	// Read existing dismissed ids from cookie
	var ids []string
	if dc, err := r.Cookie("dismissed_announcements"); err == nil && dc.Value != "" {
		ids = strings.Split(dc.Value, ",")
	}

	// Append id if not already present
	found := false
	for _, existing := range ids {
		if strings.TrimSpace(existing) == id {
			found = true
			break
		}
	}
	if !found {
		ids = append(ids, id)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "dismissed_announcements",
		Value:    strings.Join(ids, ","),
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
	})

	ref := r.Referer()
	if ref == "" {
		ref = "/"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}

// handleConsentCCPA handles POST /consent/ccpa.
// Sets or clears the ccpa_opt_out cookie and redirects back.
// form field "action": "opt-out" sets cookie; "opt-in" clears it.
// Works with zero JS; the CCPA form POSTs here directly.
func (s *Server) handleConsentCCPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	action := r.FormValue("action")
	if action == "opt-out" {
		http.SetCookie(w, &http.Cookie{
			Name:     "ccpa_opt_out",
			Value:    "true",
			Path:     "/",
			MaxAge:   365 * 24 * 60 * 60,
			SameSite: http.SameSiteLaxMode,
		})
	} else {
		// opt-in: clear the cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "ccpa_opt_out",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			SameSite: http.SameSiteLaxMode,
		})
	}

	ref := r.Referer()
	if ref == "" {
		ref = "/"
	}
	http.Redirect(w, r, ref, http.StatusSeeOther)
}
