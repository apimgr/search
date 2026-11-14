package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/security"
	"github.com/go-chi/chi/v5"
)

// SecurityOverviewPageData extends PageData for /server/security.
type SecurityOverviewPageData struct {
	PageData
	ReportURL      string
	ContactURL     string
	MailtoContact  string
	Expires        string
	HasPGPKey      bool
	PGPKeyURL      string
	SecurityTxtURL string
}

type SecurityAcknowledgment struct {
	Credit   string `json:"credit"`
	Date     string `json:"date"`
	Severity string `json:"severity"`
}

func (s *Server) SecurityOverview(r *http.Request) interface{} {
	webSecurity := s.config.Server.Web.Security
	baseURL := s.getBaseURL(r)
	securityID := security.GenerateSecurityID(s.config.Server.Security.InstallationSecret, time.Now().Unix())
	contacts := make([]string, 0, 3)
	if webSecurity.ReportURL != "" {
		contacts = append(contacts, webSecurity.ReportURL)
	}
	contacts = append(contacts, baseURL+"/server/contact?security_id="+securityID)
	contact := webSecurity.Contact
	if contact == "" {
		contact = s.config.Server.Contact.Security.Email
	}
	if contact == "" {
		contact = s.config.Server.Contact.Admin.Email
	}
	if contact != "" {
		if !strings.HasPrefix(contact, "mailto:") && strings.Contains(contact, "@") {
			contact = "mailto:" + contact
		}
		contacts = append(contacts, contact)
	}
	pgp := map[string]interface{}{"available": false, "url": "", "fingerprint": ""}
	if webSecurity.PublishPGPKey {
		if armor, err := security.LoadPublicKey(config.GetConfigDir()); err == nil && armor != "" {
			fingerprint, _ := security.KeyFingerprint(armor)
			pgp = map[string]interface{}{"available": true, "url": baseURL + "/.well-known/pgp-key.asc", "fingerprint": fingerprint}
		}
	}
	expires := webSecurity.Expires
	if expires == "" {
		expires = time.Now().AddDate(1, 0, 0).UTC().Format(time.RFC3339)
	}
	return map[string]interface{}{
		"contacts": contacts, "expires": expires, "pgp": pgp,
		"policy_url": baseURL + "/server/security/policy", "thanks_url": baseURL + "/server/security/thanks",
	}
}

func (s *Server) SecurityPolicy(r *http.Request) interface{} {
	policy := s.config.Server.Web.Security.Policy
	if policy == "" {
		lang := s.getI18nManager().ResolveLanguage(nil, r)
		im := s.getI18nManager()
		policy = strings.Join([]string{
			"## " + im.T(lang, "security.policy_scope_heading"), im.T(lang, "security.policy_scope_description"),
			"## " + im.T(lang, "security.policy_disclosure_window_heading"), im.T(lang, "security.policy_disclosure_window_description"),
			"## " + im.T(lang, "security.policy_in_scope_heading"), "- " + im.T(lang, "security.policy_in_scope_item_deployed"), "- " + im.T(lang, "security.policy_in_scope_item_api"), "- " + im.T(lang, "security.policy_in_scope_item_auth"),
			"## " + im.T(lang, "security.policy_out_of_scope_heading"), "- " + im.T(lang, "security.policy_out_of_scope_item_dos"), "- " + im.T(lang, "security.policy_out_of_scope_item_social"), "- " + im.T(lang, "security.policy_out_of_scope_item_physical"), "- " + im.T(lang, "security.policy_out_of_scope_item_third_party"),
			"## " + im.T(lang, "security.policy_safe_harbor_heading"), im.T(lang, "security.policy_safe_harbor_description"),
		}, "\n\n")
	}
	return map[string]string{"policy": policy}
}

func (s *Server) SecurityThanks(ctx context.Context) (interface{}, error) {
	entries := make([]SecurityAcknowledgment, 0)
	if s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return entries, nil
	}
	table := database.ServerTableName(s.dbManager.ServerDB(), "security_reports")
	rows, err := s.dbManager.ServerDB().Query(ctx, fmt.Sprintf(
		`SELECT credit_preference, credit_name, credit_handle, created_at, severity FROM %s WHERE status = ? ORDER BY created_at`, table), "disclosed")
	if err != nil {
		return nil, err
	}
	anonymous := 0
	for rows.Next() {
		var preference, name, handle, created, severity sql.NullString
		if err := rows.Scan(&preference, &name, &handle, &created, &severity); err != nil {
			return nil, err
		}
		credit := ""
		switch preference.String {
		case "real_name":
			credit = name.String
		case "handle":
			credit = handle.String
		case "anonymous":
			anonymous++
			credit = fmt.Sprintf("Anonymous Researcher #%d", anonymous)
		}
		if credit == "" {
			continue
		}
		date := created.String
		if len(date) >= 4 {
			date = date[:4]
		}
		entries = append(entries, SecurityAcknowledgment{Credit: credit, Date: date, Severity: severity.String})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return entries, nil
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
	if s.config.Server.Web.Security.Policy != "" {
		custom := struct {
			PageData
			PolicyMarkdown string
		}{PageData: *data, PolicyMarkdown: s.config.Server.Web.Security.Policy}
		if err := s.renderer.Render(w, "security_policy_custom", custom); err != nil {
			s.handleInternalError(w, r, "template render", err)
		}
		return
	}

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
	entries, err := s.SecurityThanks(r.Context())
	if err != nil {
		s.handleInternalError(w, r, "load security acknowledgments", err)
		return
	}
	pageData := struct {
		PageData
		Entries []SecurityAcknowledgment
	}{PageData: *data}
	if values, ok := entries.([]SecurityAcknowledgment); ok {
		pageData.Entries = values
	}

	if err := s.renderer.Render(w, "security_thanks", pageData); err != nil {
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
// is accepted once per UTC calendar day and expires 30 days after report closure.
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

	now := time.Now()
	if status.TokenExpired(now) {
		s.handleNotFound(w, r)
		return
	}

	if !security.ReportTokenMatches(token, status.ReportTokenHash) {
		s.handleNotFound(w, r)
		return
	}
	accessDate := now.UTC().Format("2006-01-02")
	allowed, err := security.RecordReportStatusAccess(r.Context(), s.dbManager.ServerDB(), trackingID, accessDate)
	if err != nil {
		s.handleInternalError(w, r, "record security report status access", err)
		return
	}
	if !allowed {
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
