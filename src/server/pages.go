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
	"net/mail"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/i2p"
	"github.com/apimgr/search/src/notify"
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
	captchaRequired := s.contactCaptchaRequired(r)
	var a, b int
	var captchaID string
	mode, siteKey, secretKey := s.configuredContactCaptcha()
	onionRequest := isOnionRequest(s.config, r)
	externalCaptcha := captchaRequired && !onionRequest && (mode == "recaptcha" || mode == "hcaptcha") && siteKey != "" && secretKey != ""
	if captchaRequired && (mode == "simple" || onionRequest) {
		captchaA, _ := rand.Int(rand.Reader, big.NewInt(10))
		captchaB, _ := rand.Int(rand.Reader, big.NewInt(10))
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
		SecurityMode:    securityMode,
		SecurityID:      securityID,
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
	securityMode := securityID != "" && s.validateSecurityIDOrLog(r, securityID)
	lang := s.newPageData(w, r, "", "contact").Lang
	if s.contactCaptchaRequired(r) && !s.verifyConfiguredContactCaptcha(r) {
		s.markContactCaptchaRequired(w, r)
		s.renderContactForm(w, r, securityID, securityMode, s.getI18nManager().T(lang, "errors.bad_request"))
		return
	}
	if strings.TrimSpace(r.PostFormValue("website")) != "" {
		s.clearContactCaptchaRequirement(w, r)
		if strings.Contains(r.Header.Get("Accept"), "application/json") {
			respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]string{"status": "received"}})
		} else {
			http.Redirect(w, r, "/server/contact?success=1", http.StatusSeeOther)
		}
		return
	}
	if securityMode {
		s.handleSecurityReportSubmit(w, r, securityID)
		return
	}

	name := strings.TrimSpace(r.PostFormValue("name"))
	address, addressErr := mail.ParseAddress(strings.TrimSpace(r.PostFormValue("email")))
	subject := strings.TrimSpace(r.PostFormValue("subject"))
	message := strings.TrimSpace(r.PostFormValue("message"))
	if name == "" || addressErr != nil || address.Address != strings.TrimSpace(r.PostFormValue("email")) || subject == "" || message == "" || len(name) > 200 || len(subject) > 300 || len(message) > 10000 {
		s.markContactCaptchaRequired(w, r)
		s.renderContactForm(w, r, "", false, s.getI18nManager().T(lang, "errors.bad_request"))
		return
	}
	if err := s.SubmitContact(r.Context(), api.ContactMessage{Name: name, Email: address.Address, Subject: subject, Message: message, SourceIP: getClientIPSimple(r)}); err != nil {
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
	recipient := s.config.Server.Contact.General.Email
	if recipient == "" {
		recipient = s.config.Server.Contact.Admin.Email
	}
	mailEnabled := recipient != "" && s.mailer != nil && s.mailer.CanSend()
	webhookEnabled := s.notifyManager != nil && s.notifyManager.HasWebhook("general")
	if !mailEnabled && !webhookEnabled {
		return fmt.Errorf("contact delivery is not configured")
	}
	if mailEnabled {
		mailMessage := email.NewMessage([]string{recipient}, "[contact] "+message.Subject,
			"Name: "+message.Name+"\nEmail: "+message.Email+"\n\n"+message.Message)
		mailMessage.Headers["Reply-To"] = message.Email
		if err := s.mailer.Send(mailMessage); err != nil {
			return err
		}
	}
	if webhookEnabled {
		body := "Name: " + message.Name + "\nEmail: " + message.Email + "\n\n" + message.Message
		if err := s.notifyManager.Dispatch(ctx, notify.Event{Role: "general", Name: "contact.received", Subject: message.Subject, Body: body, Severity: "info", SourceIP: message.SourceIP}); err != nil {
			return err
		}
	}
	return nil
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
		researcherEmail, researcherGPG, creditPreference, creditName, disclosureDays, cveRequested)

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
	creditPreference, creditName string, disclosureDays int, cveRequested bool) string {
	t := func(key string, args ...interface{}) string { return i18n.T(lang, key, args...) }
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", t("email.security_report.label_tracking_id"), trackingID)
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

	if err := s.renderer.Render(w, "help", data); err != nil {
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

	// I2P check (only when I2P is enabled — opt-in per AI.md PART 31.2)
	if s.config.Server.I2P.Enabled {
		if s.i2pManager != nil && s.i2pManager.IsRunning() {
			checks.I2P = "ok"
		} else {
			checks.I2P = "error"
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
			Tor: torInfo,
			// I2P status from the eepsite manager per AI.md PART 13 /
			// PART 31.2 — opt-in, so disabled is the default state.
			I2P:   buildI2PInfo(s.i2pManager),
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

	if health.Status == "healthy" && s.config.PendingRestart() {
		health.Status = "restart_required"
		health.PendingRestart = true
		health.RestartReason = s.config.RestartSettings()
	}

	return health
}

// HealthSnapshot exposes the same checks and status to the versioned and
// unversioned API health routes as the browser-facing endpoint.
func (s *Server) HealthSnapshot() api.HealthSnapshot {
	health := s.buildHealthInfo()
	return api.HealthSnapshot{
		Status:         health.Status,
		PendingRestart: health.PendingRestart,
		RestartReason:  append([]string(nil), health.RestartReason...),
		Checks: api.ChecksInfo{
			Database:  health.Checks.Database,
			Cache:     health.Checks.Cache,
			Disk:      health.Checks.Disk,
			Scheduler: health.Checks.Scheduler,
			Tor:       health.Checks.Tor,
			I2P:       health.Checks.I2P,
		},
	}
}

// buildI2PInfo derives the features.i2p payload from the eepsite manager,
// mirroring the src/api healthz mapping. Status uses the AI.md PART 13
// enum (healthy/disabled/starting/error), not the manager's human string.
// A nil manager (I2P not enabled) reports the disabled/none state per
// AI.md PART 31.2.
func buildI2PInfo(mgr *i2p.I2PManager) I2PInfo {
	if mgr == nil || !mgr.Enabled() {
		return I2PInfo{
			Enabled:  false,
			Running:  false,
			Status:   "disabled",
			Hostname: "",
			Provider: "none",
		}
	}
	if !mgr.IsRunning() {
		return I2PInfo{
			Enabled:  true,
			Running:  false,
			Status:   "error",
			Hostname: "",
			Provider: mgr.ProviderName(),
		}
	}
	return I2PInfo{
		Enabled:  true,
		Running:  true,
		Status:   "healthy",
		Hostname: mgr.EepsiteAddress(),
		Provider: mgr.ProviderName(),
	}
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
func (s *Server) respondHealthText(w http.ResponseWriter, health *HealthResponse) {
	w.Header().Set("Content-Type", "text/plain")

	statusCode := http.StatusOK
	if health.Status == "unhealthy" || health.Status == "maintenance" || health.Status == "shutting_down" {
		statusCode = http.StatusServiceUnavailable
	}
	w.WriteHeader(statusCode)

	var b strings.Builder

	// 1. Project (PART 16: branding)
	fmt.Fprintf(&b, "project.name: %s\n", health.Project.Name)
	fmt.Fprintf(&b, "project.tagline: %s\n", health.Project.Tagline)
	fmt.Fprintf(&b, "project.description: %s\n", health.Project.Description)

	// 2. Status
	fmt.Fprintf(&b, "status: %s\n", health.Status)

	// 3. Version & Build (PART 7)
	fmt.Fprintf(&b, "version: %s\n", health.Version)
	fmt.Fprintf(&b, "go_version: %s\n", health.GoVersion)
	fmt.Fprintf(&b, "build.commit: %s\n", health.Build.Commit)
	fmt.Fprintf(&b, "build.date: %s\n", health.Build.Date)

	// 4. Runtime (PART 6)
	fmt.Fprintf(&b, "uptime: %s\n", health.Uptime)
	fmt.Fprintf(&b, "mode: %s\n", health.Mode)
	fmt.Fprintf(&b, "timestamp: %s\n", health.Timestamp)

	// 5. Features - NON-NEGOTIABLE only (show actual status)
	fmt.Fprintf(&b, "features.tor.enabled: %t\n", health.Features.Tor.Enabled)
	fmt.Fprintf(&b, "features.tor.running: %t\n", health.Features.Tor.Running)
	fmt.Fprintf(&b, "features.tor.status: %s\n", health.Features.Tor.Status)
	fmt.Fprintf(&b, "features.tor.hostname: %s\n", health.Features.Tor.Hostname)
	fmt.Fprintf(&b, "features.i2p.enabled: %t\n", health.Features.I2P.Enabled)
	fmt.Fprintf(&b, "features.i2p.running: %t\n", health.Features.I2P.Running)
	fmt.Fprintf(&b, "features.i2p.status: %s\n", health.Features.I2P.Status)
	fmt.Fprintf(&b, "features.i2p.hostname: %s\n", health.Features.I2P.Hostname)
	fmt.Fprintf(&b, "features.i2p.provider: %s\n", health.Features.I2P.Provider)
	fmt.Fprintf(&b, "features.geoip: %t\n", health.Features.GeoIP)

	// 6. Checks
	fmt.Fprintf(&b, "checks.database: %s\n", health.Checks.Database)
	fmt.Fprintf(&b, "checks.cache: %s\n", health.Checks.Cache)
	fmt.Fprintf(&b, "checks.disk: %s\n", health.Checks.Disk)
	fmt.Fprintf(&b, "checks.scheduler: %s\n", health.Checks.Scheduler)
	if health.Checks.Tor != "" {
		fmt.Fprintf(&b, "checks.tor: %s\n", health.Checks.Tor)
	}

	// 7. Stats
	fmt.Fprintf(&b, "stats.requests_total: %d\n", health.Stats.RequestsTotal)
	fmt.Fprintf(&b, "stats.requests_24h: %d\n", health.Stats.Requests24h)
	fmt.Fprintf(&b, "stats.active_connections: %d\n", health.Stats.ActiveConns)

	writeResponseText(w, b.String())
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
		for _, name := range []string{
			"theme", "lang", widgetCookieName, "search_widget_weather_units",
			"search_pref_category", "search_pref_safe_search", "search_pref_results_per_page",
			"search_pref_units", "search_pref_new_tab", "search_pref_infinite_scroll",
			"search_pref_keyboard_shortcuts", "search_pref_save_history", "search_pref_video_previews",
		} {
			http.SetCookie(w, &http.Cookie{
				Name: name, Value: "", Path: "/", MaxAge: -1,
				Expires: time.Unix(1, 0), SameSite: http.SameSiteLaxMode,
				Secure: s.cookieSecure(r),
			})
		}
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
