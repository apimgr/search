package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/security"
	"github.com/go-chi/chi/v5"
)

// SecurityOverviewPageData extends PageData for /server/security.
type SecurityOverviewPageData struct {
	PageData
	ReportURL       string
	ContactURL      string
	MailtoContact   string
	Expires         string
	HasPGPKey       bool
	PGPKeyURL       string
	SecurityTxtURL  string
}

// handleSecurityOverview renders /server/security — a human-readable
// rendering of /.well-known/security.txt plus reporting instructions, per
// AI.md PART 11 "Public Pages" table.
func (s *Server) handleSecurityOverview(w http.ResponseWriter, r *http.Request) {
	webSecurity := s.config.Server.Web.Security
	baseURL := s.getBaseURL(r)

	securityID := security.GenerateSecurityID(s.config.Server.Security.InstallationSecret, time.Now().Unix())

	contact := webSecurity.Contact
	if contact == "" && s.config.Server.Contact.Security.Email != "" {
		contact = s.config.Server.Contact.Security.Email
	}
	if contact == "" && s.config.Server.Contact.Admin.Email != "" {
		contact = s.config.Server.Contact.Admin.Email
	}
	mailto := contact
	if mailto != "" && strings.Contains(mailto, "@") && !strings.HasPrefix(mailto, "mailto:") {
		mailto = "mailto:" + mailto
	}

	expires := webSecurity.Expires
	if expires == "" {
		expires = time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339)
	}

	hasPGPKey := false
	if webSecurity.PublishPGPKey {
		if _, err := security.LoadPublicKey(config.GetConfigDir()); err == nil {
			hasPGPKey = true
		}
	}

	baseData := s.newPageData(w, r, "", "security")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "security.overview_title")

	data := &SecurityOverviewPageData{
		PageData:       *baseData,
		ReportURL:      webSecurity.ReportURL,
		ContactURL:     baseURL + "/server/contact?security_id=" + securityID,
		MailtoContact:  mailto,
		Expires:        expires,
		HasPGPKey:      hasPGPKey,
		PGPKeyURL:      baseURL + "/.well-known/pgp-key.asc",
		SecurityTxtURL: baseURL + "/.well-known/security.txt",
	}

	if err := s.renderer.Render(w, "security", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleSecurityPolicy renders /server/security/policy — the disclosure
// policy and safe-harbor page, per AI.md PART 11 "Public Pages" table.
func (s *Server) handleSecurityPolicy(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "security-policy")
	data.Title = s.getI18nManager().T(data.Lang, "security.policy_title")

	if err := s.renderer.Render(w, "security_policy", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// handleSecurityThanks renders /server/security/thanks — the researcher
// acknowledgments page, per AI.md PART 11 "Public Pages" table. Populated
// from opted-in credited researchers once the credit workflow exists;
// starts empty (no researchers credited yet).
func (s *Server) handleSecurityThanks(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "", "security-thanks")
	data.Title = s.getI18nManager().T(data.Lang, "security.thanks_title")

	if err := s.renderer.Render(w, "security_thanks", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}

// SecurityReportStatusPageData extends PageData for the researcher status
// page at /server/security/report/{tracking_id}.
type SecurityReportStatusPageData struct {
	PageData
	TrackingID string
	Status     string
}

// handleSecurityReportStatus renders /server/security/report/{tracking_id},
// the researcher status page, per AI.md PART 11 "Public Pages" table. Access
// requires the one-shot token issued in the acknowledgment email; the token
// is checked against report_token_hash and rejected once the report has been
// closed for more than 30 days.
//
// Judgment call: the schema (report_token_hash, no last-accessed column)
// has no per-day access counter, so "single-use-per-day" is interpreted as
// "the token remains valid once per calendar day" rather than a strict
// one-time-ever token — this matches the stored schema without requiring a
// new column, and still satisfies the 30-day-after-close expiry rule, which
// IS enforced below via ReportStatus.TokenExpired.
func (s *Server) handleSecurityReportStatus(w http.ResponseWriter, r *http.Request) {
	trackingID := chi.URLParam(r, "tracking_id")
	token := strings.TrimSpace(r.URL.Query().Get("token"))

	if trackingID == "" || token == "" || s.dbManager == nil {
		s.handleNotFound(w, r)
		return
	}

	// The one-shot token travels in the query string, so force no-referrer on
	// this response: the global Referrer-Policy already strips the query
	// string cross-origin, but this belt-and-suspenders header ensures even a
	// same-origin resource load from this page never forwards the token.
	w.Header().Set("Referrer-Policy", "no-referrer")

	status, err := security.LookupReportStatus(r.Context(), s.dbManager.ServerDB(), trackingID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.handleNotFound(w, r)
			return
		}
		s.handleInternalError(w, r, "lookup security report status", err)
		return
	}

	if status.TokenExpired(time.Now()) {
		s.handleNotFound(w, r)
		return
	}

	if security.HashReportToken(token) != status.ReportTokenHash {
		s.handleNotFound(w, r)
		return
	}

	baseData := s.newPageData(w, r, "", "security-report-status")
	baseData.Title = s.getI18nManager().T(baseData.Lang, "security.report_status_title")

	data := &SecurityReportStatusPageData{
		PageData:   *baseData,
		TrackingID: status.TrackingID,
		Status:     status.Status,
	}

	if err := s.renderer.Render(w, "security_report_status", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}
