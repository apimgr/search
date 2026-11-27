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

type SecurityPolicyDefaultPageData struct {
	PageData
	FQDN    string
	Domains []string
}

func (s *Server) SecurityOverview(r *http.Request) interface{} {
	webSecurity := s.config.Server.Web.Security
	baseURL := s.getBaseURL(r)
	securityID := security.GenerateSecurityID(s.config.Server.Security.InstallationSecret, time.Now().Unix())
	contacts := make([]string, 0, 3)
	if webSecurity.ReportURL != "" {
		contacts = append(contacts, webSecurity.ReportURL)
	}
	if s.config.Server.Pages.Contact.Enabled {
		contacts = append(contacts, baseURL+"/server/contact?security_id="+securityID)
	}
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
		lang := s.getI18nManager().ResolveLanguage(nil, r, false)
		im := s.getI18nManager()
		fqdn, domains := securityPolicyDomains(s.config.Server.FQDN, r.Host)
		onion, i2pAddress := s.securityPolicyOverlayAddresses()
		policy = defaultSecurityPolicyMarkdown(lang, im.T, fqdn, domains, onion, i2pAddress)
	}
	return map[string]string{"policy": policy}
}

func (s *Server) securityPolicyOverlayAddresses() (string, string) {
	onion := ""
	if s.torService != nil && s.torService.IsRunning() {
		onion = s.torService.GetOnionAddress()
	}
	i2pAddress := ""
	if s.i2pManager != nil {
		i2pAddress = s.i2pManager.EepsiteAddress()
	}
	return onion, i2pAddress
}

func securityPolicyDomains(configured, requestHost string) (string, []string) {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		configured = strings.Join(config.GetAllDomains(), ",")
	}
	domains := make([]string, 0)
	for _, domain := range strings.Split(configured, ",") {
		domain = strings.TrimSpace(domain)
		if domain != "" && !containsString(domains, domain) {
			domains = append(domains, domain)
		}
	}
	if len(domains) == 0 && strings.TrimSpace(requestHost) != "" {
		domains = append(domains, strings.TrimSpace(requestHost))
	}
	if len(domains) == 0 {
		return "", nil
	}
	return domains[0], domains[1:]
}

func defaultSecurityPolicyMarkdown(lang string, translate func(string, string, ...interface{}) string, fqdn string, domains []string, onion, i2p string) string {
	scope := translate(lang, "security.policy_scope_description", fqdn)
	sections := []string{
		"## " + translate(lang, "security.policy_scope_heading"), scope,
		"## " + translate(lang, "security.policy_disclosure_window_heading"), translate(lang, "security.policy_disclosure_window_description"),
		"## " + translate(lang, "security.policy_in_scope_heading"), "- " + translate(lang, "security.policy_in_scope_item_deployed"), "- " + translate(lang, "security.policy_in_scope_item_api"), "- " + translate(lang, "security.policy_in_scope_item_auth"),
	}
	for _, domain := range domains {
		sections = append(sections, "- "+translate(lang, "security.policy_in_scope_item_host", domain))
	}
	if onion != "" {
		sections = append(sections, "- "+translate(lang, "security.policy_in_scope_item_tor", onion))
	}
	if i2p != "" {
		sections = append(sections, "- "+translate(lang, "security.policy_in_scope_item_i2p", i2p))
	}
	sections = append(sections,
		"## "+translate(lang, "security.policy_out_of_scope_heading"),
		"- "+translate(lang, "security.policy_out_of_scope_item_dos"),
		"- "+translate(lang, "security.policy_out_of_scope_item_social"),
		"- "+translate(lang, "security.policy_out_of_scope_item_physical"),
		"- "+translate(lang, "security.policy_out_of_scope_item_third_party"),
		"- "+translate(lang, "security.policy_out_of_scope_item_scanner"),
		"- "+translate(lang, "security.policy_out_of_scope_item_headers"),
		"## "+translate(lang, "security.policy_safe_harbor_heading"),
		translate(lang, "security.policy_safe_harbor_description"),
		"## "+translate(lang, "security.policy_reporting_heading"),
		translate(lang, "security.policy_reporting_description")+" ["+translate(lang, "security.overview_title")+"](/server/security)",
	)
	return strings.Join(sections, "\n\n")
}

func (s *Server) SecurityThanks(ctx context.Context) (interface{}, error) {
	entries := make([]SecurityAcknowledgment, 0)
	if s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return entries, nil
	}
	table := database.ServerTableName(s.dbManager.ServerDB(), "security_thanks")
	rows, err := s.dbManager.ServerDB().Query(ctx, fmt.Sprintf(
		`SELECT credit, disclosed_on, severity FROM %s ORDER BY disclosed_on DESC, tracking_id`, table))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var credit, disclosedOn, severity sql.NullString
		if err := rows.Scan(&credit, &disclosedOn, &severity); err != nil {
			return nil, err
		}
		date := disclosedOn.String
		if len(date) >= 4 {
			date = date[:4]
		}
		entries = append(entries, SecurityAcknowledgment{Credit: credit.String, Date: date, Severity: severity.String})
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
		ContactURL:     "",
		MailtoContact:  mailto,
		Expires:        expires,
		HasPGPKey:      hasPGPKey,
		PGPKeyURL:      baseURL + "/.well-known/pgp-key.asc",
		SecurityTxtURL: baseURL + "/.well-known/security.txt",
	}
	if s.config.Server.Pages.Contact.Enabled {
		data.ContactURL = baseURL + "/server/contact?security_id=" + securityID
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
	fqdn, domains := securityPolicyDomains(s.config.Server.FQDN, r.Host)
	defaultData := SecurityPolicyDefaultPageData{PageData: *data, FQDN: fqdn, Domains: domains}
	if err := s.renderer.Render(w, "security_policy", defaultData); err != nil {
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
	TrackingID      string
	Status          string
	ResearcherComment string
	DisclosureDate  string
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
	w.Header().Set("Cache-Control", "no-store")

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
		PageData:         *baseData,
		TrackingID:       status.TrackingID,
		Status:           status.Status,
		ResearcherComment: status.ResearcherComment,
		DisclosureDate:   status.DisclosureDate,
	}

	if err := s.renderer.Render(w, "security_report_status", data); err != nil {
		s.handleInternalError(w, r, "template render", err)
	}
}
