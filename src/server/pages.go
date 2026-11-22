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
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/health"
	"github.com/apimgr/search/src/i2p"
	"github.com/apimgr/search/src/notify"
	"github.com/apimgr/search/src/security"
	"github.com/apimgr/search/src/version"
	"github.com/go-chi/chi/v5"
)

// handleHome renders the home page
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		s.handleNotFound(w, r)
		return
	}

	data := s.newPageData(w, r, "", "home")
	format := httputil.GetPreferredFormat(r)
	if format == "application/json" {
		respondJSON(w, http.StatusOK, api.APIResponse{
			OK:   true,
			Data: s.apiHandler.Info(),
			Meta: &api.APIMeta{Version: api.APIVersion},
		})
		return
	}

	if httputil.IsTextBrowser(r) && format == "text/html" {
		s.renderNoJSHome(w, r, data)
		return
	}

	if format == "text/plain" {
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

	if err := s.renderer.Render(w, "about", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handlePrivacy renders the privacy page
func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "privacy")
	data.Title = s.getI18nManager().T(data.Lang, "footer.privacy_policy")

	if err := s.renderer.Render(w, "privacy", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

type dpoPageData struct {
	PageData
	DPOName  string
	DPOEmail string
}

type dpoResponse struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	PrivacyURL string `json:"privacy_url"`
}

func (s *Server) dpoContact(r *http.Request) dpoResponse {
	name := strings.TrimSpace(s.config.Server.Contact.DPO.Name)
	email := strings.TrimSpace(s.config.Server.Contact.DPO.Email)
	if email == "" {
		email = strings.TrimSpace(s.config.Server.Contact.General.Email)
	}
	return dpoResponse{Name: name, Email: email, PrivacyURL: httputil.BuildURL(r, "/server/privacy")}
}

func (s *Server) handleDPO(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/server/dpo" {
		s.handleNotFound(w, r)
		return
	}
	contact := s.dpoContact(r)
	data := s.newPageData(w, r, "", "dpo")
	data.Title = s.getI18nManager().T(data.Lang, "dpo.title")
	pageData := dpoPageData{PageData: *data, DPOName: contact.Name, DPOEmail: contact.Email}
	if err := s.renderer.Render(w, "dpo", &pageData); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleContact renders the contact form and handles its submission.
// Per AI.md PART 11 "Security Reports — Coordinated Disclosure Pipeline": the
// same /server/contact route doubles as the vulnerability-report submission
// path when a valid ?security_id={id} switches the form into security mode.
func (s *Server) handleContact(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Pages.Contact.Enabled {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		s.handleContactSubmit(w, r)
		return
	}

	securityID := strings.TrimSpace(r.URL.Query().Get("security_id"))
	securityMode := s.validateSecurityIDOrLog(r, securityID)

	s.renderContactForm(w, r, securityID, securityMode, "")
}

func (s *Server) ValidContactSecurityID(r *http.Request, securityID string) bool {
	return s.validateSecurityIDOrLog(r, securityID)
}

func (s *Server) SubmitSecurityContact(original *http.Request, message api.SecurityContactMessage) (string, error) {
	form := url.Values{
		"security_id": {message.SecurityID}, "name": {message.Name}, "email": {message.Email},
		"researcher_gpg": {message.ResearcherGPG}, "affected_component": {message.AffectedComponent},
		"affected_endpoint": {message.AffectedEndpoint}, "severity": {message.Severity}, "summary": {message.Summary},
		"steps_to_reproduce": {message.StepsToReproduce}, "impact": {message.Impact}, "suggested_fix": {message.SuggestedFix},
		"cve_requested": {strconv.FormatBool(message.CVERequested)}, "disclosure_days": {strconv.Itoa(message.DisclosureDays)},
		"credit_preference": {message.CreditPreference}, "credit_handle": {message.CreditHandle},
		"agreed_to_disclosure": {strconv.FormatBool(message.AgreedToDisclosure)},
	}
	req := httptest.NewRequest(http.MethodPost, "/server/contact", strings.NewReader(form.Encode())).WithContext(original.Context())
	req.Header = original.Header.Clone()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.PostForm = form
	req.Form = form
	req.RemoteAddr = original.RemoteAddr
	recorder := httptest.NewRecorder()
	s.handleSecurityReportSubmit(recorder, req, message.SecurityID)
	if recorder.Code != http.StatusOK {
		return "", fmt.Errorf("security report handler returned HTTP %d", recorder.Code)
	}
	var response struct {
		Data struct {
			TrackingID string `json:"tracking_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		return "", fmt.Errorf("decode security report response: %w", err)
	}
	if response.Data.TrackingID == "" {
		return "", fmt.Errorf("security report handler returned no tracking id")
	}
	return response.Data.TrackingID, nil
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
	// Per AI.md PART 11 "Secret Rotation": the previous installation_secret
	// stays valid for 7 days after rotation so in-flight {security_id} URLs
	// still validate.
	if prev := s.previousInstallationSecret(); prev != "" &&
		security.ValidateSecurityID(prev, securityID, time.Now().Unix()) {
		return true
	}
	if s.logManager != nil {
		s.logManager.Security().LogSecurityIDInvalid(getClientIPSimple(r), r.UserAgent(), securityID)
	}
	return false
}

// previousInstallationSecret returns the pre-rotation installation_secret when
// it is still inside its 7-day grace window (AI.md PART 11 "Secret Rotation"),
// or "" when the secret has never been rotated or the window has closed.
func (s *Server) previousInstallationSecret() string {
	sec := s.config.Server.Security
	if sec.InstallationSecretPrevious == "" || sec.InstallationSecretPreviousExpires == "" {
		return ""
	}
	expires, err := time.Parse(time.RFC3339, sec.InstallationSecretPreviousExpires)
	if err != nil || time.Now().UTC().After(expires) {
		return ""
	}
	return sec.InstallationSecretPrevious
}

// renderContactForm renders the contact page in standard or security mode.
func (s *Server) renderContactForm(w http.ResponseWriter, r *http.Request, securityID string, securityMode bool, contactError string) {
	s.renderContactFormValues(w, r, securityID, securityMode, contactError, nil)
}

func (s *Server) renderContactFormValues(w http.ResponseWriter, r *http.Request, securityID string, securityMode bool, contactError string, values url.Values) {
	if values == nil && !securityMode {
		values = make(url.Values)
		requestedType := strings.TrimSpace(r.URL.Query().Get("type"))
		if requestedType == "abuse" || requestedType == "dmca" || requestedType == "privacy" {
			values.Set("type", requestedType)
		}
	}
	captchaRequired := true
	var a, b int
	var captchaID string
	mode, siteKey, secretKey := s.configuredContactCaptcha()
	onionRequest := isOnionRequest(s.config, r)
	externalCaptcha := captchaRequired && !onionRequest && (mode == "recaptcha" || mode == "hcaptcha") && siteKey != "" && secretKey != ""
	if captchaRequired && (mode == "simple" || onionRequest) {
		captchaA, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			s.handleInternalError(w, r, "contact captcha challenge", err)
			return
		}
		captchaB, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			s.handleInternalError(w, r, "contact captcha challenge", err)
			return
		}
		a = int(captchaA.Int64()) + 1
		b = int(captchaB.Int64()) + 1
		captchaID = s.signCaptcha(a + b)
	}
	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, "", "contact")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "contact.page_title")
	if captchaRequired && !externalCaptcha && mode != "simple" && !onionRequest && contactError == "" {
		contactError = s.getI18nManager().T(baseData.Lang, "errors.server_error_message")
	}

	data := &ContactPageData{
		PageData:        *baseData,
		ContactError:    contactError,
		CaptchaA:        a,
		CaptchaB:        b,
		CaptchaID:       captchaID,
		CaptchaRequired: captchaRequired,
		CaptchaExternal: externalCaptcha,
		CaptchaLocal:    captchaRequired && (mode == "simple" || onionRequest),
		CaptchaProvider: mode,
		CaptchaSiteKey:  siteKey,
		FormToken:       s.ContactFormToken(),
		SecurityMode:    securityMode,
		SecurityID:      securityID,
		ReviewFields:    make(map[string]string),
	}
	for key, entries := range values {
		if len(entries) > 0 {
			data.ReviewFields[key] = entries[0]
		}
	}

	if securityMode {
		data.AppVersion = version.Version
		data.CommitHash = version.CommitID
		data.Timestamp = time.Now().UTC().Format(time.RFC3339)
		data.RequestUserAgent = r.UserAgent()
		data.DisclosureDays = 90
		data.ResearcherGPG = values.Get("researcher_gpg")
		data.AffectedComponent = values.Get("affected_component")
		data.AffectedEndpoint = values.Get("affected_endpoint")
		data.Severity = values.Get("severity")
		data.Summary = values.Get("summary")
		data.StepsToReproduce = values.Get("steps_to_reproduce")
		data.Impact = values.Get("impact")
		data.SuggestedFix = values.Get("suggested_fix")
		data.CVERequested = config.IsTruthy(values.Get("cve_requested"))
		data.CreditPreference = values.Get("credit_preference")
		data.CreditHandle = values.Get("credit_handle")
		data.AgreedToDisclosure = config.IsTruthy(values.Get("agreed_to_disclosure"))
	}

	// Check for success message
	if r.URL.Query().Get("success") == "1" {
		data.ContactSent = true
	}

	if err := s.renderer.Render(w, "contact", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

func (s *Server) renderContactReview(w http.ResponseWriter, r *http.Request, securityID string, securityMode bool, values url.Values, flags, contactError string) {
	baseData := s.newPageData(w, r, "", "contact")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "contact.page_title")
	fields := make(map[string]string, len(values))
	for key, entries := range values {
		if len(entries) > 0 && key != "csrf_token" && key != "form_token" && key != "captcha" && key != "captcha_id" && key != "captcha_answer" && key != "captcha_response" && key != "g-recaptcha-response" && key != "h-captcha-response" && key != "website" && key != "step" && key != "review_token" && key != "confirmed" {
			fields[key] = entries[0]
		}
	}
	settings := s.contactSpamSettings()
	minSeconds := settings.ReviewMinSeconds
	if minSeconds <= 0 {
		minSeconds = 2
	}
	reviewSubject := fields["subject"]
	if securityMode {
		reviewSubject = fields["summary"]
	}
	data := &ContactPageData{
		PageData: *baseData, ContactError: contactError, FormToken: s.ContactFormToken(),
		ReviewMode: true, ReviewType: fields["type"], ReviewEmail: fields["email"],
		ReviewSubject: reviewSubject, ReviewToken: s.ContactReviewToken(values, flags),
		ConfirmAfter: minSeconds, ReviewFields: fields, SecurityMode: securityMode, SecurityID: securityID,
	}
	if err := s.renderer.Render(w, "contact", data); err != nil {
		s.handleInternalError(w, r, "contact review render", err)
	}
}

// handleContactSubmit processes a POST to /server/contact. When a valid
// security_id was submitted, the report is routed to the coordinated-
// disclosure pipeline per AI.md PART 11 "Submission Flow"; otherwise the
// standard contact-form path is preserved (out of scope for this pipeline).
func (s *Server) handleContactSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.config.Server.Pages.Contact.Enabled {
		http.NotFound(w, r)
		return
	}
	if s.ContactBanned(r) {
		s.RecordContactBlock("banned", r)
		w.Header().Set("Retry-After", "86400")
		localizedHTTPError(w, r, http.StatusTooManyRequests, "errors.rate_limited")
		return
	}
	if !s.ValidateContactOrigin(r) {
		s.RecordContactBlock("origin", r)
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			respondJSON(w, http.StatusBadRequest, api.APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: "Please submit the form from this site"})
		} else {
			http.Error(w, "Please submit the form from this site", http.StatusBadRequest)
		}
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxContactBodyBytes)
	if err := r.ParseForm(); err != nil {
		localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}

	securityID := strings.TrimSpace(r.PostFormValue("security_id"))
	securityMode := securityID != "" && s.validateSecurityIDOrLog(r, securityID)
	lang := s.newPageData(w, r, "", "contact").Lang
	if reason := s.ValidateContactShape(r, r.PostForm, securityMode, false); reason != "" {
		s.RecordContactBlock(reason, r)
		localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}
	step := r.PostFormValue("step")
	if step == "edit" {
		s.renderContactFormValues(w, r, securityID, securityMode, "", r.PostForm)
		return
	}
	honeypot := s.ContactHoneypotFilled(r.PostForm)
	if step != "confirm" {
		if !s.ValidateContactFormToken(r.PostFormValue("form_token")) {
			s.RecordContactBlock(s.ContactTokenFailureReason(r.PostFormValue("form_token")), r)
			w.WriteHeader(http.StatusBadRequest)
			s.renderContactFormValues(w, r, securityID, securityMode, s.getI18nManager().T(lang, "errors.bad_request"), r.PostForm)
			return
		}
		if !s.verifyConfiguredContactCaptcha(r) {
			s.RecordContactBlock("captcha", r)
			s.markContactCaptchaRequired(w, r)
			s.renderContactFormValues(w, r, securityID, securityMode, s.getI18nManager().T(lang, "errors.bad_request"), r.PostForm)
			return
		}
	} else if !s.ValidateContactReviewToken(r.PostForm, r.PostFormValue("review_token"), config.IsTruthy(r.PostFormValue("confirmed"))) {
		s.RecordContactBlock("review", r)
		flags := ""
		if ContactReviewHoneypot(r.PostFormValue("review_token")) {
			flags = "d"
		}
		s.renderContactReview(w, r, securityID, securityMode, r.PostForm, flags, "Please read the details and confirm")
		return
	}
	if securityMode {
		r.PostForm.Set("type", "security")
		r.Form.Set("type", "security")
	}
	if !securityMode && !s.validContactReportFields(r, r.PostForm) {
		s.RecordContactBlock("content", r)
		s.renderContactFormValues(w, r, "", false, "Your message could not be sent", r.PostForm)
		return
	}
	if step != "confirm" {
		address, err := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
		if err != nil {
			localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
			return
		}
		if reason, retryAfter := s.CheckContactSubmissionLimit(r, getClientIPSimple(r), address.Address); reason != "" {
			s.RecordContactBlock(reason, r)
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
			localizedHTTPError(w, r, http.StatusTooManyRequests, "errors.rate_limited")
			return
		}
		flags := ""
		if honeypot {
			flags = "d"
			s.RecordContactBlock("honeypot", r)
		}
		if honeypot {
			r.PostForm.Set("website", "")
		}
		s.renderContactReview(w, r, securityID, securityMode, r.PostForm, flags, "")
		return
	}
	if ContactReviewHoneypot(r.PostFormValue("review_token")) {
		s.clearContactCaptchaRequirement(w, r)
		s.respondContactSpamSuccess(w, r)
		return
	}
	if securityMode {
		address, err := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
		if err != nil {
			localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
			return
		}
		if reason, retryAfter := s.AcceptContactSubmission(r, getClientIPSimple(r), address.Address, r.PostFormValue("name"), r.PostFormValue("summary"), r.PostFormValue("steps_to_reproduce"), true); reason != "" {
			s.RecordContactBlock(reason, r)
			if retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
				localizedHTTPError(w, r, http.StatusTooManyRequests, "errors.rate_limited")
				return
			}
			if reason == "content" {
				s.renderContactForm(w, r, securityID, securityMode, "Your message could not be sent")
				return
			}
			localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
			return
		}
		s.handleSecurityReportSubmit(w, r, securityID)
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	address, addressErr := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
	subject := strings.TrimSpace(r.PostFormValue("subject"))
	message := strings.TrimSpace(r.PostFormValue("message"))
	for _, field := range []struct {
		name, value string
		max         int
	}{{"name", name, 100}, {"email", strings.TrimSpace(r.PostFormValue("email")), 254}, {"subject", subject, 200}, {"message", message, 5000}} {
		if utf8.RuneCountInString(field.value) <= field.max {
			continue
		}
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			respondJSON(w, http.StatusBadRequest, api.APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: "Field exceeds maximum length", Details: map[string]interface{}{"field": field.name, "rule": "max_length", "max_length": field.max}})
		} else {
			s.markContactCaptchaRequired(w, r)
			s.renderContactForm(w, r, "", false, s.getI18nManager().T(lang, "errors.bad_request"))
		}
		return
	}
	if name == "" || addressErr != nil || address.Address != strings.TrimSpace(r.PostFormValue("email")) || subject == "" || message == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(address.Address) > 254 || utf8.RuneCountInString(subject) > 200 || utf8.RuneCountInString(message) > 5000 {
		s.markContactCaptchaRequired(w, r)
		s.renderContactForm(w, r, "", false, s.getI18nManager().T(lang, "errors.bad_request"))
		return
	}
	if reason, retryAfter := s.AcceptContactSubmission(r, getClientIPSimple(r), address.Address, name, subject, message, false); reason != "" {
		s.RecordContactBlock(reason, r)
		if reason == "duplicate" {
			s.clearContactCaptchaRequirement(w, r)
			s.respondContactSpamSuccess(w, r)
			return
		}
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
			localizedHTTPError(w, r, http.StatusTooManyRequests, "errors.rate_limited")
			return
		}
		if reason == "content" {
			if strings.Contains(r.Header.Get("Accept"), "application/json") {
				respondJSON(w, http.StatusBadRequest, api.APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: "Your message could not be sent", Details: map[string]interface{}{"field": "contact", "rule": "invalid"}})
			} else {
				s.markContactCaptchaRequired(w, r)
				s.renderContactForm(w, r, "", false, "Your message could not be sent")
			}
			return
		}
		localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}
	if err := s.SubmitContact(r.Context(), api.ContactMessage{
		Name: name, Email: address.Address, Subject: subject, Message: message, Type: r.PostFormValue("type"),
		ReportedURL: r.PostFormValue("reported_url"), Reason: r.PostFormValue("reason"),
		CopyrightedWork:    r.PostFormValue("copyrighted_work"),
		GoodFaithStatement: config.IsTruthy(r.PostFormValue("good_faith_statement")),
		AccuracyStatement:  config.IsTruthy(r.PostFormValue("accuracy_statement")),
		Signature:          r.PostFormValue("signature"), RequestKind: r.PostFormValue("request_kind"),
		SourceIP: getClientIPSimple(r),
	}); err != nil {
		s.ForgetContactSubmission(r, getClientIPSimple(r), address.Address, name, subject, message, false)
		slog.Error("contact message delivery failed")
		s.markContactCaptchaRequired(w, r)
		s.renderContactForm(w, r, "", false, s.getI18nManager().T(lang, "errors.server_error_message"))
		return
	}
	s.clearContactCaptchaRequirement(w, r)
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "received"}})
		return
	}
	http.Redirect(w, r, "/server/contact?success=1", http.StatusSeeOther)
}

func (s *Server) validContactReportFields(r *http.Request, values url.Values) bool {
	typeName := values.Get("type")
	if typeName == "abuse" {
		switch values.Get("reason") {
		case "illegal_content", "terms_violation", "harassment", "malware_phishing", "other":
		default:
			return false
		}
	}
	if typeName == "dmca" && (!config.IsTruthy(values.Get("good_faith_statement")) || !config.IsTruthy(values.Get("accuracy_statement"))) {
		return false
	}
	if typeName == "abuse" || typeName == "dmca" {
		if !s.validReportedURL(r, values.Get("reported_url")) {
			return false
		}
	}
	settings := s.contactSpamSettings()
	fields := []string{values.Get("message")}
	if typeName != "security" {
		for _, field := range fields {
			if !s.contactLinksAllowed(r, field, settings.MaxLinks, settings.BlockedLinkDomains) {
				return false
			}
		}
	}
	return true
}

func (s *Server) ValidateContactReport(r *http.Request, values url.Values) bool {
	return s.validContactReportFields(r, values)
}

func (s *Server) validReportedURL(r *http.Request, raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}
	if u.Path == "" || u.Path == "/" || u.Path == "/server/contact" || strings.HasPrefix(u.Path, "/api/") || strings.HasPrefix(u.Path, "/server/admin") {
		return false
	}
	if !s.contactHostAllowed(r, u.Hostname(), nil) {
		return false
	}
	if s.router == nil {
		return false
	}
	ctx := chi.NewRouteContext()
	return s.router.Match(ctx, http.MethodGet, u.Path)
}

func (s *Server) contactLinksAllowed(r *http.Request, content string, maxLinks int, blockedDomains []string) bool {
	matches := contactLinkPattern.FindAllStringIndex(content, -1)
	count := 0
	for _, match := range matches {
		if match[0] > 0 && content[match[0]-1] == '@' {
			continue
		}
		candidate := strings.TrimRight(content[match[0]:match[1]], ".,;:!?)\"'")
		if candidate == "" {
			continue
		}
		count++
		raw := candidate
		if !strings.Contains(raw, "://") {
			raw = "https://" + strings.TrimPrefix(strings.TrimPrefix(raw, "www."), "www.")
		}
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || !s.contactHostAllowed(r, u.Hostname(), blockedDomains) {
			return false
		}
	}
	return maxLinks <= 0 || count <= maxLinks
}

func (s *Server) contactHostAllowed(r *http.Request, host string, blockedDomains []string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	allowed := make(map[string]bool)
	if base, err := url.Parse(s.getBaseURL(r)); err == nil && base.Hostname() != "" {
		allowed[strings.ToLower(base.Hostname())] = true
	}
	for _, domain := range strings.Split(os.Getenv("DOMAIN"), ",") {
		if domain = strings.TrimSpace(strings.ToLower(domain)); domain != "" {
			allowed[strings.TrimSuffix(domain, ".")] = true
		}
	}
	requestHost := strings.ToLower(r.Host)
	if hostOnly, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = hostOnly
	}
	allowed[strings.TrimSuffix(requestHost, ".")] = true
	if !allowed[host] {
		return false
	}
	for _, blocked := range blockedDomains {
		blocked = strings.Trim(strings.ToLower(strings.TrimSpace(blocked)), ".")
		if blocked != "" && (host == blocked || strings.HasSuffix(host, "."+blocked)) {
			return false
		}
	}
	return true
}

func (s *Server) respondContactSpamSuccess(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "received"}})
		return
	}
	http.Redirect(w, r, "/server/contact?success=1", http.StatusSeeOther)
}

func (s *Server) validCaptcha(id, answerText string) bool {
	parts := strings.SplitN(id, ".", 2)
	if len(parts) != 2 {
		return false
	}
	answer, err := strconv.Atoi(answerText)
	if err != nil || answer < 2 || answer > 20 {
		return false
	}
	expectedAnswer, err := strconv.Atoi(parts[0])
	if err != nil || expectedAnswer < 2 || expectedAnswer > 20 || answer != expectedAnswer {
		return false
	}
	mac := hmac.New(sha256.New, s.contactCaptchaSigningKey())
	_, _ = mac.Write([]byte(parts[0]))
	expectedSignature := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expectedSignature))
}

func (s *Server) contactCaptchaRequired(r *http.Request) bool {
	cookie, err := r.Cookie("contact_captcha_required")
	if err != nil {
		return false
	}
	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || timestamp > time.Now().Unix() || time.Since(time.Unix(timestamp, 0)) > time.Hour {
		return false
	}
	mac := hmac.New(sha256.New, s.contactCaptchaSigningKey())
	_, _ = mac.Write([]byte(parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

func (s *Server) markContactCaptchaRequired(w http.ResponseWriter, r *http.Request) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, s.contactCaptchaSigningKey())
	_, _ = mac.Write([]byte(timestamp))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	cookie := &http.Cookie{Name: "contact_captcha_required", Value: timestamp + "." + sig, Path: "/server/contact", MaxAge: 3600, HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode}
	http.SetCookie(w, cookie)
	r.AddCookie(cookie)
}

func (s *Server) clearContactCaptchaRequirement(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "contact_captcha_required", Value: "", Path: "/server/contact", MaxAge: -1, HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
}

// SubmitContact sends the contact message through the configured queued mailer.
func (s *Server) SubmitContact(ctx context.Context, message api.ContactMessage) error {
	role := strings.ToLower(strings.TrimSpace(message.Type))
	if role == "" {
		role = "general"
	}
	if role == "dmca" {
		role = "abuse"
	}
	if role != "abuse" && role != "privacy" {
		role = "general"
	}
	recipient := ""
	switch role {
	case "abuse":
		recipient = s.config.Server.Contact.Abuse.Email
	case "privacy":
		recipient = s.config.Server.Contact.DPO.Email
	}
	if recipient == "" {
		recipient = s.config.Server.Contact.General.Email
	}
	if recipient == "" {
		recipient = s.config.Server.Contact.Admin.Email
	}
	mailEnabled := recipient != "" && s.mailer != nil && s.mailer.CanSend()
	webhookRole := role
	if role == "privacy" && s.notifyManager != nil && !s.notifyManager.HasWebhook(webhookRole) {
		webhookRole = "general"
	}
	webhookEnabled := s.notifyManager != nil && s.notifyManager.HasWebhook(webhookRole)
	if !mailEnabled && !webhookEnabled {
		return fmt.Errorf("contact delivery is not configured")
	}
	if mailEnabled {
		appName := strings.TrimSpace(s.config.Server.Branding.Title)
		if appName == "" {
			appName = "Search"
		}
		mailMessage := email.NewMessage([]string{recipient}, "["+appName+" contact] "+message.Subject,
			contactMessageBody(message))
		mailMessage.Headers["Reply-To"] = message.Email
		if err := s.mailer.Send(mailMessage); err != nil {
			return err
		}
	}
	if webhookEnabled {
		if err := s.notifyManager.Dispatch(ctx, notify.Event{Role: webhookRole, Name: "contact.received", Subject: message.Subject, Body: contactMessageBody(message), Severity: "info", SourceIP: message.SourceIP}); err != nil {
			return err
		}
	}
	return nil
}

func contactMessageBody(message api.ContactMessage) string {
	var body strings.Builder
	fmt.Fprintf(&body, "Type: %s\n", message.Type)
	fmt.Fprintf(&body, "Name: %s\nEmail: %s\n", message.Name, message.Email)
	if message.ReportedURL != "" {
		fmt.Fprintf(&body, "Reported URL: %s\n", message.ReportedURL)
	}
	if message.Reason != "" {
		fmt.Fprintf(&body, "Reason: %s\n", message.Reason)
	}
	if message.CopyrightedWork != "" {
		fmt.Fprintf(&body, "Copyrighted work: %s\n", message.CopyrightedWork)
	}
	if message.GoodFaithStatement {
		fmt.Fprintf(&body, "Good faith statement: %t\n", message.GoodFaithStatement)
	}
	if message.AccuracyStatement {
		fmt.Fprintf(&body, "Accuracy statement: %t\n", message.AccuracyStatement)
	}
	if message.Signature != "" {
		fmt.Fprintf(&body, "Signature: %s\n", message.Signature)
	}
	if message.RequestKind != "" {
		fmt.Fprintf(&body, "Privacy request: %s\n", message.RequestKind)
	}
	body.WriteString("\n")
	body.WriteString(message.Message)
	return body.String()
}

func (s *Server) dispatchNotification(event notify.Event) {
	if s.notifyManager == nil || !s.notifyManager.HasWebhook(event.Role) {
		return
	}
	if err := s.notifyManager.Dispatch(context.Background(), event); err != nil {
		slog.Error("notification dispatch failed", "event", "notify.webhook_failed", "role", event.Role)
	}
}

func (s *Server) dispatchExactNotification(event notify.Event) {
	if s.notifyManager == nil || !notify.HasExactWebhook(s.config.Server.Contact, event.Role) {
		return
	}
	if err := s.notifyManager.DispatchExact(context.Background(), event); err != nil {
		slog.Error("notification dispatch failed", "event", "notify.webhook_failed", "role", event.Role)
	}
}

// handleSecurityReportSubmit implements AI.md PART 11's 7-step coordinated-
// disclosure Submission Flow.
func (s *Server) handleSecurityReportSubmit(w http.ResponseWriter, r *http.Request, securityID string) {
	accepted := false
	defer func() {
		if !accepted {
			s.ForgetContactSubmission(r, getClientIPSimple(r), r.PostFormValue("email"), r.PostFormValue("name"), r.PostFormValue("summary"), r.PostFormValue("steps_to_reproduce"), true)
		}
	}()
	lang := s.newPageData(w, r, "", "contact").Lang
	if strings.TrimSpace(r.PostFormValue("website")) != "" {
		s.clearContactCaptchaRequirement(w, r)
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "received"}})
		} else {
			http.Redirect(w, r, "/server/contact?success=1", http.StatusSeeOther)
		}
		return
	}

	affectedComponent := strings.TrimSpace(r.PostFormValue("affected_component"))
	affectedEndpoint := strings.TrimSpace(r.PostFormValue("affected_endpoint"))
	severity := strings.TrimSpace(r.PostFormValue("severity"))
	summary := strings.TrimSpace(r.PostFormValue("summary"))
	stepsToReproduce := strings.TrimSpace(r.PostFormValue("steps_to_reproduce"))
	impact := strings.TrimSpace(r.PostFormValue("impact"))
	suggestedFix := strings.TrimSpace(r.PostFormValue("suggested_fix"))
	researcherGPG := strings.TrimSpace(r.PostFormValue("researcher_gpg"))
	researcherEmail := strings.TrimSpace(r.PostFormValue("email"))
	researcherName := strings.TrimSpace(r.PostFormValue("name"))
	creditPreference := strings.TrimSpace(r.PostFormValue("credit_preference"))
	creditName := ""
	creditHandle := strings.TrimSpace(r.PostFormValue("credit_handle"))
	switch creditPreference {
	case "handle":
		if creditHandle == "" || utf8.RuneCountInString(creditHandle) > 100 {
			s.markContactCaptchaRequired(w, r)
			s.renderContactForm(w, r, securityID, true, s.getI18nManager().T(lang, "contact.security_required_fields"))
			return
		}
	case "real_name":
		creditName = researcherName
	}
	cveRequested := config.IsTruthy(r.PostFormValue("cve_requested"))
	agreedToDisclosure := config.IsTruthy(r.PostFormValue("agreed_to_disclosure"))

	disclosureDays := 90
	if v := strings.TrimSpace(r.PostFormValue("disclosure_days")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			disclosureDays = n
		}
	}

	gpgLimit := 16384
	if strings.HasPrefix(strings.ToLower(researcherGPG), "https://") {
		gpgLimit = 2048
	}
	for _, field := range []struct {
		name, value string
		max         int
	}{{"researcher_gpg", researcherGPG, gpgLimit}, {"affected_component", affectedComponent, 255}, {"affected_endpoint", affectedEndpoint, 2048}, {"summary", summary, 200}, {"steps_to_reproduce", stepsToReproduce, 10000}, {"impact", impact, 5000}, {"suggested_fix", suggestedFix, 5000}} {
		if utf8.RuneCountInString(field.value) <= field.max {
			continue
		}
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			respondJSON(w, http.StatusBadRequest, api.APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: "Field exceeds maximum length", Details: map[string]interface{}{"field": field.name, "rule": "max_length", "max_length": field.max}})
		} else {
			s.markContactCaptchaRequired(w, r)
			s.renderContactForm(w, r, securityID, true, s.getI18nManager().T(lang, "errors.bad_request"))
		}
		return
	}
	parsedEmail, emailErr := mail.ParseAddress(researcherEmail)
	if researcherName == "" || utf8.RuneCountInString(researcherName) > 100 || emailErr != nil || parsedEmail.Address != researcherEmail || utf8.RuneCountInString(researcherEmail) > 254 || summary == "" || severity == "" || affectedComponent == "" || stepsToReproduce == "" || impact == "" || !agreedToDisclosure {
		s.markContactCaptchaRequired(w, r)
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

	plaintextBody := composeSecurityReportBody(s.notificationLang(), trackingID, affectedComponent,
		affectedEndpoint, severity, summary, stepsToReproduce, impact, suggestedFix,
		researcherEmail, researcherGPG, creditPreference, creditName, disclosureDays, cveRequested,
		researcherName, config.Version, config.CommitID, time.Now().UTC().Format(time.RFC3339), r.UserAgent())

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
		CreditHandle:      creditHandle,
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
	accepted = true

	// Step 4: maintainer notification (security role, falls back to admin).
	s.sendSecurityMaintainerNotification(report, plaintextBody, researcherGPG, getClientIPSimple(r))

	// Step 5: researcher acknowledgment, only if the researcher gave contact info.
	if researcherEmail != "" {
		s.sendSecurityResearcherAck(r, researcherEmail, researcherGPG, trackingID, rawToken)
	}

	// Step 7: log acceptance — tracking id + safe metadata only, never PII/content.
	if s.logManager != nil {
		s.logManager.Security().LogReportReceived(getClientIPSimple(r), trackingID, severity, affectedComponent)
	}

	// Step 6: respond to the form POST.
	s.clearContactCaptchaRequirement(w, r)
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
// logged in plaintext form. Per AI.md PART 30 "Email Template Translation",
// every label resolves from an email.security_report.* i18n key via i18n.T
// before rendering — no hardcoded English strings.
func composeSecurityReportBody(lang, trackingID, affectedComponent, affectedEndpoint, severity, summary,
	stepsToReproduce, impact, suggestedFix, researcherEmail, researcherGPG,
	creditPreference, creditName string, disclosureDays int, cveRequested bool, researcherNames ...string) string {
	t := func(key string, args ...interface{}) string { return i18n.T(lang, key, args...) }
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_tracking_id"), trackingID)
	if len(researcherNames) > 0 && researcherNames[0] != "" {
		fmt.Fprintf(&b, "Name: %s\n", researcherNames[0])
	}
	if len(researcherNames) > 4 {
		fmt.Fprintf(&b, "app_version: %s\ncommit_hash: %s\ntimestamp: %s\nrequest_user_agent: %s\n",
			researcherNames[1], researcherNames[2], researcherNames[3], researcherNames[4])
	}
	fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_component"), affectedComponent)
	fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_endpoint"), affectedEndpoint)
	fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_severity"), severity)
	fmt.Fprintf(&b, "%s %s\n\n", t("email.security_report.label_summary"), summary)
	fmt.Fprintf(&b, "%s\n%s\n\n", t("email.security_report.label_steps"), stepsToReproduce)
	fmt.Fprintf(&b, "%s\n%s\n\n", t("email.security_report.label_impact"), impact)
	if suggestedFix != "" {
		fmt.Fprintf(&b, "%s\n%s\n\n", t("email.security_report.label_fix"), suggestedFix)
	}
	fmt.Fprintf(&b, "%s %t\n", t("email.security_report.label_cve"), cveRequested)
	fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_disclosure"),
		t("email.security_report.days", disclosureDays))
	fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_credit"), creditPreference)
	if creditName != "" {
		fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_credit_name"), creditName)
	}
	if researcherEmail != "" {
		fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_email"), researcherEmail)
	}
	if researcherGPG != "" {
		fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_gpg"), researcherGPG)
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

// notificationLang returns the language for server notification emails:
// the server i18n default when a manager is configured, else "en".
// Per AI.md PART 30 "Email Template Translation".
func (s *Server) notificationLang() string {
	if s.i18nManager != nil {
		if dl := s.i18nManager.DefaultLanguage(); dl != "" {
			return dl
		}
	}
	return "en"
}

// sendSecurityMaintainerNotification sends the maintainer CC notification
// per AI.md PART 11 step 4, to the security contact role (falls back to
// admin per AI.md PART 12 fallback chain). Best-effort: a delivery failure
// does not fail the submission since the report is already persisted.
func (s *Server) sendSecurityMaintainerNotification(report security.Report, plaintextBody, researcherGPG, sourceIP string) {
	recipient := s.config.Server.Contact.Security.Email
	if recipient == "" {
		recipient = s.config.Server.Contact.Admin.Email
	}
	var pgpBody string
	if researcherGPG != "" {
		if encrypted, err := security.EncryptMessageToArmoredKey(researcherGPG, []byte(plaintextBody)); err == nil {
			pgpBody = encrypted
		}
	}
	if pgpBody == "" {
		if pubKey, err := security.LoadPublicKey(config.GetConfigDir()); err == nil && pubKey != "" {
			if encrypted, err := security.EncryptMessageToArmoredKey(pubKey, []byte(plaintextBody)); err == nil {
				pgpBody = encrypted
			}
		}
	}
	if recipient != "" && s.mailer != nil && s.mailer.IsEnabled() {
		subject := fmt.Sprintf("[security] %s (%s)", report.Summary, report.TrackingID)
		body := pgpBody
		if body == "" {
			body = string(report.EncryptedBody)
			body = "WARNING: This report is encrypted with the server's AES-256-GCM key because no PGP key is configured. Configure and securely share the key before attempting to decrypt it.\n\n" + body
		}
		msg := email.NewMessage([]string{recipient}, subject, body)
		_ = s.mailer.Send(msg)
	}
	if pgpBody != "" {
		s.dispatchExactNotification(notify.Event{Role: "security", Name: "security.report_received", Subject: "Security report received", Body: pgpBody, Severity: report.Severity, TrackingID: report.TrackingID, SourceIP: sourceIP})
	}
	s.dispatchNotification(notify.Event{Role: "admin", Name: "security.report_received", Subject: "Security report received", Body: "A security report was received. Tracking ID: " + report.TrackingID + ". Severity: " + report.Severity + ".", Severity: report.Severity, TrackingID: report.TrackingID, SourceIP: sourceIP})
}

// sendSecurityResearcherAck sends the researcher acknowledgment email per
// AI.md PART 11 step 5, containing the tracking id and one-shot status link.
// Best-effort: a delivery failure does not fail the submission.
func (s *Server) sendSecurityResearcherAck(r *http.Request, researcherEmail, researcherGPGArmor, trackingID, rawToken string) {
	if s.mailer == nil || !s.mailer.IsEnabled() {
		return
	}

	statusURL := fmt.Sprintf("%s/server/security/report/%s?token=%s", s.getBaseURL(r), trackingID, rawToken)
	// Per AI.md PART 30 "Email Template Translation": translated strings are
	// resolved server-side via i18n.T before rendering — no hardcoded English.
	lang := s.notificationLang()
	body := fmt.Sprintf("%s\n\n%s %s\n%s %s\n",
		i18n.T(lang, "email.security_report.ack_heading"),
		i18n.T(lang, "email.security_report.label_tracking_id"), trackingID,
		i18n.T(lang, "email.security_report.label_status"), statusURL)
	subject := i18n.T(lang, "email.security_report.subject", trackingID)

	if researcherGPGArmor != "" {
		if armored, err := security.EncryptMessageToArmoredKey(researcherGPGArmor, []byte(body)); err == nil {
			body = armored
		}
	}

	msg := email.NewMessage([]string{researcherEmail}, subject, body)
	_ = s.mailer.Send(msg)
}

// handleHelp renders the help page
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "help")
	data.Title = s.getI18nManager().T(data.Lang, "help.page_title")
	data.Data = map[string]interface{}{"helpPages": s.helpPages(r), "bangs": s.bangManager.GetAll()}

	if err := s.renderer.Render(w, "help", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

type MCPHelpPageData struct {
	PageData
	MCPURL                         string
	MCPAliases                     []string
	MCPProtocolVersion             string
	MCPProtocolVersions            []string
	MCPTools                       []api.MCPHelpTool
	MCPAuthenticatedToolsAvailable bool
	MCPProjectMarkdown             string
}

func (s *Server) handleMCPHelp(w http.ResponseWriter, r *http.Request) {
	if s.apiHandler == nil {
		http.NotFound(w, r)
		return
	}
	info := s.apiHandler.MCPHelpInfo(r)
	if !info.Enabled {
		http.NotFound(w, r)
		return
	}
	baseData := s.newPageData(w, r, "", "help_mcp")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "mcp_help.title")
	data := &MCPHelpPageData{
		PageData:                       *baseData,
		MCPURL:                         info.URL,
		MCPAliases:                     info.Aliases,
		MCPProtocolVersion:             info.ProtocolVersion,
		MCPProtocolVersions:            info.ProtocolVersions,
		MCPTools:                       info.Tools,
		MCPAuthenticatedToolsAvailable: info.AuthenticatedToolsAvailable,
		MCPProjectMarkdown:             info.ProjectMarkdown,
	}
	w.Header().Set("Cache-Control", "no-store")
	if err := s.renderer.Render(w, "help_mcp", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleTerms renders the terms of service page
func (s *Server) handleTerms(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "terms")
	data.Title = s.getI18nManager().T(data.Lang, "footer.terms")

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
		s.respondHealthText(w, health, s.getI18nManager().DetectLanguage(r))
	default:
		s.respondHealthHTML(w, r, health)
	}
}

// handleReadyz handles Kubernetes readiness probe
// Per AI.md PART 11/13: /readyz endpoint for readiness
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	health := s.buildHealthInfo()

	// Return 503 if not ready (unhealthy or maintenance)
	if health.Status == "unhealthy" || health.Status == "maintenance" || health.Status == "shutting_down" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		writeResponseText(w, fmt.Sprintf("NOT READY: %s\n", health.Status))
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	writeResponseText(w, "READY\n")
}

// handleLivez handles Kubernetes liveness probe
// Per AI.md PART 11/13: /livez endpoint for liveness
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	// Liveness probe is simpler - just check if server can respond
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	writeResponseText(w, "ALIVE\n")
}

// buildHealthInfo constructs the HealthResponse per AI.md PART 13.
// Field order matches canonical spec: project, status, version, build, runtime,
// features, checks, stats.
func (s *Server) buildHealthInfo() *HealthResponse {
	// Per AI.md PART 13: Build Tor feature status
	torState := health.TorState(s.config.Server.Tor.Enabled, s.torService != nil && s.torService.IsStarting(), s.torService != nil && s.torService.IsRunning(), func() string {
		if s.torService != nil {
			return s.torService.GetOnionAddress()
		}
		return ""
	}())
	torInfo := TorInfo{Enabled: torState.Enabled, Running: torState.Running, Status: torState.Status, Hostname: torState.Hostname}
	i2pInfo := buildI2PInfo(s.i2pManager)

	status := "healthy"
	if s.shuttingDown.Load() {
		status = "shutting_down"
	}

	// Check maintenance mode
	if s.config.Server.MaintenanceMode && status != "shutting_down" {
		status = "maintenance"
	}

	// Per AI.md PART 13: checks use "ok" or "error" only (no "disabled")
	checks := ChecksInfo{
		Cache:     "ok",
		Disk:      "ok",
		Scheduler: "ok",
	}

	// Cache check — Ping exercises the backend; nil backend (caching
	// disabled) is a valid configured state and healthy per AI.md PART 9.
	if s.cache != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		if err := s.cache.Ping(ctx); err != nil {
			checks.Cache = "error"
			if status == "healthy" {
				status = "unhealthy"
			}
		}
		cancel()
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

	// The built-in scheduler is required by AI.md PART 18.
	if s.scheduler == nil || !s.scheduler.IsRunning() {
		checks.Scheduler = "error"
	}

	// Tor check (only when Tor is configured)
	if torInfo.Enabled {
		if torInfo.Status == "healthy" || torInfo.Status == "starting" {
			checks.Tor = "ok"
		} else {
			checks.Tor = "degraded"
			if status == "healthy" {
				status = "degraded"
			}
		}
	}

	// I2P check (only when I2P is enabled — opt-in per AI.md PART 31.2)
	if i2pInfo.Enabled {
		if i2pInfo.Status == "healthy" || i2pInfo.Status == "starting" {
			checks.I2P = "ok"
		} else {
			checks.I2P = "degraded"
			if status == "healthy" {
				status = "degraded"
			}
		}
	}
	var dynamicCriticalFailure, dynamicDegraded bool
	healthRegistry := s.currentHealthRegistry()
	if healthRegistry != nil {
		checks.Extra = healthRegistry.Checks()
		dynamicCriticalFailure, dynamicDegraded = aggregateDynamicHealth(checks.Extra)
	}

	// Resolve status once, following the canonical precedence from PART 13.
	switch {
	case s.shuttingDown.Load():
		status = "shutting_down"
	case s.config.Server.MaintenanceMode:
		status = "maintenance"
	case checks.Database == "error" || checks.Disk == "error" || checks.Scheduler == "error":
		status = "unhealthy"
	case dynamicCriticalFailure:
		status = "unhealthy"
	case checks.Cache == "error":
		status = "degraded"
	case checks.Tor == "degraded" || checks.I2P == "degraded":
		status = "degraded"
	case dynamicDegraded:
		status = "degraded"
	default:
		status = "healthy"
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
			Date:   version.HealthBuildDate(),
		},
		// 4. Runtime info
		Uptime:    formatDuration(time.Since(s.startTime)),
		Mode:      s.config.Server.Mode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Cluster:   ClusterInfo{Enabled: false},
		// 6. Features - PUBLIC only (PARTS 20, 32)
		Features: FeaturesInfo{
			Tor: torInfo,
			// I2P status from the eepsite manager per AI.md PART 13 /
			// PART 31.2 — opt-in, so disabled is the default state.
			I2P:   i2pInfo,
			GeoIP: s.config.Server.GeoIP.Enabled,
		},
		// 7. Checks
		Checks: checks,
		// 8. Stats per AI.md PART 13: requests_total, requests_24h, active_connections
		Stats: StatsInfo{
			RequestsTotal: s.getRequestsTotal(),
			Requests24h:   s.getRequests24h(),
			ActiveConns:   s.getActiveConnections(),
			Extra:         healthRegistryStats(healthRegistry),
		},
	}

	if health.Status == "healthy" && s.config.PendingRestart() {
		health.Status = "restart_required"
		health.PendingRestart = true
		health.RestartReason = s.config.RestartSettings()
	}

	return health
}

func aggregateDynamicHealth(checks []health.Check) (criticalFailure, degraded bool) {
	for _, check := range checks {
		if check.Status == "ok" {
			continue
		}
		if check.Critical {
			criticalFailure = true
		} else {
			degraded = true
		}
	}
	return criticalFailure, degraded
}

// HealthSnapshot exposes the same checks and status to the versioned and
// unversioned API health routes as the browser-facing endpoint.
func (s *Server) HealthSnapshot() api.HealthSnapshot {
	healthInfo := s.buildHealthInfo()
	return api.HealthSnapshot{
		Status:         healthInfo.Status,
		PendingRestart: healthInfo.PendingRestart,
		RestartReason:  append([]string(nil), healthInfo.RestartReason...),
		Checks: api.ChecksInfo{
			Database:  healthInfo.Checks.Database,
			Cache:     healthInfo.Checks.Cache,
			Disk:      healthInfo.Checks.Disk,
			Scheduler: healthInfo.Checks.Scheduler,
			Tor:       healthInfo.Checks.Tor,
			I2P:       healthInfo.Checks.I2P,
			Extra:     append([]health.Check(nil), healthInfo.Checks.Extra...),
		},
		StatsExtra: append([]health.Stat(nil), healthInfo.Stats.Extra...),
	}
}

func healthRegistryStats(registry *health.Registry) []health.Stat {
	if registry == nil {
		return nil
	}
	return registry.Stats()
}

func (s *Server) refreshHealthRegistry() {
	if registry := s.currentHealthRegistry(); registry != nil {
		registry.Refresh()
	}
}

func (s *Server) currentHealthRegistry() *health.Registry {
	if s == nil {
		return nil
	}
	s.healthRegistryMu.RLock()
	defer s.healthRegistryMu.RUnlock()
	return s.healthRegistry
}

func (s *Server) rebuildHealthRegistry() {
	if s == nil {
		return
	}
	registry := newHealthRegistry(s)
	registry.Refresh()
	s.healthRegistryMu.Lock()
	s.healthRegistry = registry
	s.healthRegistryMu.Unlock()
}

// buildI2PInfo derives the features.i2p payload from the eepsite manager,
// mirroring the src/api healthz mapping. Status uses the AI.md PART 13
// enum: disabled, starting, healthy, or unavailable. The separate state field
// preserves I2P's full disabled/no_provider/starting/building/running/error
// lifecycle. When disabled, every
// field remains at its zero value; provider details are only present when an
// active provider was found.
func buildI2PInfo(mgr *i2p.I2PManager) I2PInfo {
	if mgr == nil || !mgr.Enabled() {
		return I2PInfo{Status: "disabled", State: "disabled"}
	}
	state := health.I2PState(mgr.LifecycleState(), mgr.ProviderName(), mgr.EepsiteAddress())
	return I2PInfo{Enabled: state.Enabled, Running: state.Running, Status: state.Status, State: state.State, Hostname: state.Hostname, Provider: state.Provider}
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
// getRequests24h returns the request count from the trailing 24-hour window.
func (s *Server) getRequests24h() int64 {
	if s.metrics != nil {
		return s.metrics.GetRequestsLast24Hours()
	}
	return 0
}

// getActiveConnections returns current active connections
// Per AI.md PART 13: stats.active_connections must return actual count
func (s *Server) getActiveConnections() int {
	if s.metrics != nil {
		if active := int(s.metrics.GetActiveConnections()); active > 0 {
			return active
		}
		return 1
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
	if health.Status == "unhealthy" || health.Status == "maintenance" || health.Status == "shutting_down" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)

	data, _ := jsonMarshal(health)
	writeResponseBytes(w, data)
	// Per AI.md PART 14: Single trailing newline
	writeResponseText(w, "\n")
}

// respondHealthText responds with plain text health info per AI.md PART 13.
// Format: key: value pairs, one per line.
func (s *Server) respondHealthText(w http.ResponseWriter, health *HealthResponse, languages ...string) {
	w.Header().Set("Content-Type", "text/plain")

	statusCode := http.StatusOK
	if health.Status == "unhealthy" || health.Status == "maintenance" || health.Status == "shutting_down" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)

	var b strings.Builder
	lang := "en"
	if len(languages) > 0 && languages[0] != "" {
		lang = languages[0]
	}
	translate := func(key string) string { return s.getI18nManager().T(lang, key) }

	// Frontend text is intended for people using curl, so keep labels and values
	// human-readable instead of exposing the API's dotted machine format.
	projectName := health.Project.Name
	if projectName == "" {
		projectName = s.config.Server.Title
	}
	fmt.Fprintf(&b, "%s - %s\n\n", projectName, health.Project.Tagline)
	statusLabel := translate("health." + health.Status)
	if statusLabel == "health."+health.Status {
		statusLabel = humanizeHealthStatus(health.Status)
	}
	fmt.Fprintf(&b, "%s:    %s\n%s:   %s\nBuild:     %s (%s)\nGo:        %s\n%s:    %s\n%s:      %s\nChecked:   %s\n",
		translate("health.status"), statusLabel, translate("health.version"), health.Version, health.Build.Commit,
		formatHealthTimestamp(health.Build.Date), health.GoVersion,
		translate("health.uptime"), formatDurationForHuman(health.Uptime), translate("health.mode"), health.Mode,
		formatHealthTimestamp(health.Timestamp))
	fmt.Fprintf(&b, "%s: %s\n", translate("health.section.cluster"), translate("common.disabled"))
	fmt.Fprintf(&b, "\n%s\n", translate("health.features_heading"))
	fmt.Fprintf(&b, "  %s:     %s\n", translate("health.components.tor"), healthFeatureText(health.Features.Tor.Status, health.Features.Tor.Hostname))
	fmt.Fprintf(&b, "  I2P:     %s\n", healthFeatureText(health.Features.I2P.Status, health.Features.I2P.Hostname))
	if health.Features.GeoIP {
		fmt.Fprintf(&b, "  %s:   %s\n", translate("health.geoip"), translate("common.enabled"))
	} else {
		fmt.Fprintf(&b, "  %s:   %s\n", translate("health.geoip"), translate("common.disabled"))
	}
	fmt.Fprintf(&b, "\n%s\n", translate("health.component_status_heading"))
	fmt.Fprintf(&b, "  %s:   %s\n  %s:      %s\n  %s:       %s\n  %s:  %s\n", translate("health.components.database"), health.Checks.Database, translate("health.components.cache"), health.Checks.Cache, translate("health.components.disk_space"), health.Checks.Disk, translate("health.components.scheduler"), health.Checks.Scheduler)
	if health.Checks.Tor != "" {
		fmt.Fprintf(&b, "  %s:        %s\n", translate("health.components.tor"), health.Checks.Tor)
	}
	if health.Checks.I2P != "" {
		fmt.Fprintf(&b, "  I2P:        %s\n", health.Checks.I2P)
	}
	for _, check := range health.Checks.Extra {
		labelKey := "health.check." + check.Key
		label := translate(labelKey)
		if label == labelKey {
			label = humanizeHealthKey(check.Key)
		}
		fmt.Fprintf(&b, "  %s: %s\n", label, check.Status)
	}
	fmt.Fprintf(&b, "\n%s\n  %s:      %s\n  %s: %s\n  %s:  %s\n", translate("health.statistics_heading"), translate("health.total_requests"),
		formatHealthCount(health.Stats.RequestsTotal), translate("health.requests_24h"),
		formatHealthCount(health.Stats.Requests24h), translate("health.active_connections"), formatHealthCount(int64(health.Stats.ActiveConns)))
	for _, stat := range health.Stats.Extra {
		labelKey := "health.stat." + stat.Key
		label := translate(labelKey)
		if label == labelKey {
			label = humanizeHealthStatKey(stat.Key)
		}
		fmt.Fprintf(&b, "  %s: %s\n", label, formatHealthCount(stat.Value))
	}

	writeResponseText(w, b.String())
}

func humanizeHealthStatus(status string) string {
	if status == "" {
		return "Unknown"
	}
	return strings.ToUpper(status[:1]) + strings.ReplaceAll(status[1:], "_", " ")
}

func humanizeHealthKey(key string) string {
	parts := strings.Split(strings.Trim(key, "_"), "_")
	if len(parts) == 0 || parts[0] == "" {
		return key
	}
	if parts[0] == "blocklist" && len(parts) > 1 {
		return "Blocklist: " + strings.Join(parts[1:], " ")
	}
	labels := map[string]string{"geoip": "GeoIP", "i2p": "I2P", "tor": "Tor", "api": "API", "cve": "CVE"}
	for i, part := range parts {
		if label, ok := labels[part]; ok {
			parts[i] = label
		} else if part != "" {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}

func humanizeHealthStatKey(key string) string {
	parts := strings.Split(strings.Trim(key, "_"), "_")
	if len(parts) > 1 && (parts[len(parts)-1] == "total" || parts[len(parts)-1] == "active" || parts[len(parts)-1] == "24h") {
		return humanizeHealthKey(strings.Join(parts[:len(parts)-1], "_")) + " " + parts[len(parts)-1]
	}
	return humanizeHealthKey(key)
}

func formatDurationForHuman(value string) string {
	var days, hours, minutes, seconds int64
	for _, part := range strings.Fields(value) {
		if len(part) < 2 {
			return value
		}
		amount, err := strconv.ParseInt(part[:len(part)-1], 10, 64)
		if err != nil {
			return value
		}
		switch part[len(part)-1] {
		case 'd':
			days = amount
		case 'h':
			hours = amount
		case 'm':
			minutes = amount
		case 's':
			seconds = amount
		default:
			return value
		}
	}
	if days == 0 && hours == 0 && minutes == 0 {
		return fmt.Sprintf("%d seconds", seconds)
	}
	var human []string
	for _, part := range []struct {
		value int64
		unit  string
	}{{days, "day"}, {hours, "hour"}, {minutes, "minute"}} {
		if part.value == 0 {
			continue
		}
		unit := part.unit
		if part.value != 1 {
			unit += "s"
		}
		human = append(human, fmt.Sprintf("%d %s", part.value, unit))
	}
	return strings.Join(human, " ")
}

func healthFeatureText(status, hostname string) string {
	if status == "" {
		status = "disabled"
	}
	if hostname != "" {
		return status + " - " + hostname
	}
	return status
}

func formatHealthTimestamp(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.UTC().Format("January 02, 2006 at 15:04:05 UTC")
}

func formatHealthCount(value interface{}) string {
	var count int64
	switch number := value.(type) {
	case int:
		count = int64(number)
	case int64:
		count = number
	case uint64:
		count = int64(number)
	default:
		return "0"
	}
	text := strconv.FormatInt(count, 10)
	for i := len(text) - 3; i > 0; i -= 3 {
		text = text[:i] + "," + text[i:]
	}
	return text
}

// respondHealthHTML responds with HTML health page
func (s *Server) respondHealthHTML(w http.ResponseWriter, r *http.Request, health *HealthResponse) {
	// Use newPageData for TorAddress support per AI.md PART 32
	baseData := s.newPageData(w, r, "", "healthz")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "health.page_title")
	if health.Status == "unhealthy" || health.Status == "maintenance" || health.Status == "shutting_down" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

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
		_, _ = w.Write([]byte(`{"ok":false,"error":"NOT_FOUND","message":` + string(msgJSON) + `,"details":{}}`))
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
	// Never echo the original request path/query back into an error page body -
	// it may carry sensitive path segments or one-shot tokens (e.g. security
	// report tracking IDs). The no-JS theme-switch form falls back to "/" when
	// RequestPath is empty (see handleThemeSet).
	baseData.RequestPath = ""

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

func (s *Server) contactCaptchaSigningKey() []byte {
	secret := ""
	if s.config != nil {
		serverConfig := s.config.Get()
		secret = serverConfig.Security.InstallationSecret
		if secret == "" {
			secret = serverConfig.SecretKey
		}
	}
	if secret == "" {
		secret = s.startTime.Format(time.RFC3339Nano)
	}
	return []byte("contact-captcha:" + secret)
}

// signCaptcha creates an HMAC-signed captcha ID containing the expected answer.
func (s *Server) signCaptcha(answer int) string {
	key := s.contactCaptchaSigningKey()
	data := strconv.Itoa(answer)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	sig := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	return data + "." + sig
}

// getCSRFToken returns the CSRF token for the request and ensures the matching
// csrf_token cookie is set, using the stateless double-submit pattern per AI.md
// PART 16. The returned value is echoed into the hidden csrf_token form field so
// mutating POSTs can be validated against the cookie.
func (s *Server) getCSRFToken(w http.ResponseWriter, r *http.Request) string {
	if s.csrf == nil {
		s.csrf = NewCSRFMiddleware(s.config)
	}
	return s.csrf.IssueToken(w, r)
}

// formatDuration formats a duration in a human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	return strings.Join(parts, " ")
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

// setClearSiteData emits the Clear-Site-Data response header per AI.md PART 11
// Privacy Signal Headers. Default directive set is "cache", "cookies", "storage";
// "executionContexts" is added only when web.headers.clear_site_data.execution_contexts
// is true. Callers that set an essential cookie in the SAME response pass
// includeCookies=false so the header does not clobber the cookie just written.
func (s *Server) setClearSiteData(w http.ResponseWriter, includeCookies bool) {
	csd := s.config.Server.Security.Headers.ClearSiteData
	parts := []string{`"cache"`}
	if includeCookies {
		parts = append(parts, `"cookies"`)
	}
	parts = append(parts, `"storage"`)
	if csd.ExecutionContexts {
		parts = append(parts, `"executionContexts"`)
	}
	w.Header().Set("Clear-Site-Data", strings.Join(parts, ", "))
}

// safeRedirectPath returns a same-site redirect target, or "/" when the input
// is anything else.
//
// The consent, announcement-dismiss, and CCPA handlers all finish by
// redirecting the browser back to the page it came from. Taking that target
// straight from the Referer header is an open redirect: Referer is
// attacker-controlled, so a victim arriving from an evil site gets a 303 to an
// arbitrary URL immediately after posting consent on our own domain — a
// credible phishing primitive that launders the hop through our origin.
//
// AI.md PART 16 requires "same-site relative paths only" for `return_to`. Only
// a root-relative path qualifies; a protocol-relative "//host" reference, a
// value with a scheme, and a backslash (which some user agents normalize to
// "/") are all rejected, as is anything carrying a control character that
// could split the Location header. Callers pass the request's own
// RequestURI() rather than the form field, so a tampered `return_to` cannot
// widen the target either — the form value is only a hint.
func safeRedirectPath(candidate string) string {
	c := strings.TrimSpace(candidate)
	if c == "" {
		return "/"
	}
	if strings.ContainsAny(c, "\r\n\x00") {
		return "/"
	}
	if strings.Contains(c, "\\") {
		return "/"
	}
	if !strings.HasPrefix(c, "/") {
		// A bare relative path, an absolute URL with a scheme, or a bare host.
		return "/"
	}
	if strings.HasPrefix(c, "//") {
		// Cross-origin reference, not a same-site path.
		return "/"
	}
	if u, err := url.Parse(c); err == nil && u.IsAbs() {
		return "/"
	}
	return c
}

// cookieSecure reports whether consent-related cookies should carry the Secure
// attribute. It mirrors CSRFMiddleware.cookieSecure so a single
// server.security.csrf.secure setting ("auto" by default) governs every cookie
// the app sets, rather than each handler hardcoding a value.
//
// The consent, dismissed-announcement, and CCPA cookies previously set no
// Secure flag at all, which meant the client-side JS path (which sets
// SameSite=Lax; Secure) and the no-JS server path disagreed. The server path is
// authoritative, so the omission is what had to change.
func (s *Server) cookieSecure(r *http.Request) bool {
	if s == nil || s.config == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(s.config.Server.Security.CSRF.Secure)) {
	case "true":
		return true
	case "false":
		return false
	default:
		// "auto": secure whenever the request itself is, and whenever the
		// server is configured to terminate TLS in front of us.
		if r != nil && r.TLS != nil {
			return true
		}
		return s.config.Server.SSL.Enabled
	}
}

// handleConsent handles POST /server/consent — sets the cookie_consent JSON cookie and redirects back.
// Accepts choice=accept (all categories) or choice=decline (essential only).
// Cookie format: {"essential":true,"preferences":true,"analytics":false,"timestamp":unix}
// Works with zero JS; the cookie banner form POSTs here directly.
func (s *Server) handleConsent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, i18n.RequestString(r, "errors.method_not_allowed"), http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
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

	// Build JSON cookie value per spec. The value is percent-encoded on the
	// wire: net/http sanitizes cookie values and silently drops the double
	// quotes out of raw JSON, which would leave the client unable to
	// JSON.parse what it receives. Encoding matches the JS contract in
	// AI.md PART 16 (encodeURIComponent on write, decodeURIComponent on
	// read) and PathEscape/decodeURIComponent agree on every byte JSON uses.
	cookieVal := url.PathEscape(fmt.Sprintf(
		`{"essential":%t,"preferences":%t,"analytics":%t,"timestamp":%d}`,
		essential, prefEnabled, analyticsEnabled, time.Now().Unix(),
	))
	http.SetCookie(w, &http.Cookie{
		Name:     "cookie_consent",
		Value:    cookieVal,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure(r),
	})

	// Consent withdrawal (decline / unknown-treated-as-decline): clear cached and
	// stored non-essential data per AI.md PART 11. "cookies" is excluded so the
	// essential cookie_consent cookie set above is not clobbered in the same response.
	withdrawal := !prefEnabled && !analyticsEnabled
	if !prefEnabled {
		s.clearPreferenceCookies(w, r)
	}
	if withdrawal && s.config.Server.Security.Headers.ClearSiteData.OnConsentWithdrawal {
		s.setClearSiteData(w, false)
	}

	// return_to is the same-site relative path the form submits (AI.md PART 16);
	// the Referer is only a fallback for a hand-rolled POST. Both are
	// attacker-controlled, so both go through safeRedirectPath.
	redirectTo := r.FormValue("return_to")
	if strings.TrimSpace(redirectTo) == "" {
		redirectTo = r.Referer()
	}
	http.Redirect(w, r, safeRedirectPath(redirectTo), http.StatusSeeOther)
}

func (s *Server) clearPreferenceCookies(w http.ResponseWriter, r *http.Request) {
	for _, name := range []string{
		"theme", "lang", widgetCookieName, "search_pref_weather_units", "search_widget_weather_units",
		"search_pref_category", "search_pref_safe_search", "search_pref_results_per_page",
		"search_pref_units", "search_pref_new_tab", "search_pref_infinite_scroll",
		"search_pref_keyboard_shortcuts", "search_pref_save_history", "search_pref_video_previews",
		"search_pref_font_size", "search_pref_results_density", "search_pref_accent_color",
		enginePreferenceCookie, engineOrderPreferenceCookie,
	} {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: "", Path: "/", MaxAge: -1,
			Expires: time.Unix(1, 0), SameSite: http.SameSiteLaxMode,
			Secure: s.cookieSecure(r),
		})
	}
}

// handleAnnouncementDismiss handles POST /announcements/dismiss.
// Appends the announcement id to the dismissed_announcements cookie and redirects back.
// Works with zero JS; the dismiss form POSTs here directly.
func (s *Server) handleAnnouncementDismiss(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, i18n.RequestString(r, "errors.method_not_allowed"), http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
		return
	}
	id := strings.TrimSpace(r.FormValue("id"))
	if id == "" {
		// Nothing to record, so just go back where the visitor came from.
		redirectTo := r.FormValue("return_to")
		if strings.TrimSpace(redirectTo) == "" {
			redirectTo = r.Referer()
		}
		http.Redirect(w, r, safeRedirectPath(redirectTo), http.StatusSeeOther)
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
		Secure:   s.cookieSecure(r),
	})

	// return_to is the same-site relative path the form submits (AI.md PART 16);
	// the Referer is only a fallback for a hand-rolled POST. Both are
	// attacker-controlled, so both go through safeRedirectPath.
	redirectTo := r.FormValue("return_to")
	if strings.TrimSpace(redirectTo) == "" {
		redirectTo = r.Referer()
	}
	http.Redirect(w, r, safeRedirectPath(redirectTo), http.StatusSeeOther)
}

// handleConsentCCPA handles POST /server/ccpa.
// Sets or clears the ccpa_opt_out cookie and redirects back.
// form field "action": "opt-out" sets cookie; "opt-in" clears it.
// Works with zero JS; the CCPA form POSTs here directly.
func (s *Server) handleConsentCCPA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, i18n.RequestString(r, "errors.method_not_allowed"), http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
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
			Secure:   s.cookieSecure(r),
		})
		// CCPA opt-out is a consent withdrawal (opt-out of sale/sharing) per AI.md
		// PART 11. Clear cached and stored non-essential data; exclude "cookies" so
		// the ccpa_opt_out cookie set above survives.
		if s.config.Server.Security.Headers.ClearSiteData.OnConsentWithdrawal {
			s.setClearSiteData(w, false)
		}
	} else {
		// opt-in: clear the cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "ccpa_opt_out",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			SameSite: http.SameSiteLaxMode,
			Secure:   s.cookieSecure(r),
		})
	}

	// return_to is the same-site relative path the form submits (AI.md PART 16);
	// the Referer is only a fallback for a hand-rolled POST. Both are
	// attacker-controlled, so both go through safeRedirectPath.
	redirectTo := r.FormValue("return_to")
	if strings.TrimSpace(redirectTo) == "" {
		redirectTo = r.Referer()
	}
	http.Redirect(w, r, safeRedirectPath(redirectTo), http.StatusSeeOther)
}
