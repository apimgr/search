package email

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	emailtemplates "github.com/apimgr/search/src/server/template/email"
)

// loadDefaultTemplate returns the embedded default template text for the given
// type, or false if no embedded default exists.
func loadDefaultTemplate(templateType TemplateType) (string, bool) {
	data, err := emailtemplates.Defaults.ReadFile(string(templateType) + ".txt")
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", false
	}
	return text, true
}

// TemplateType represents an email template type
type TemplateType string

const (
	TemplateSecurityAlert    TemplateType = "security_alert"
	TemplateBackupCompleted  TemplateType = "backup_complete"
	TemplateBackupFailed     TemplateType = "backup_failed"
	TemplateSSLExpiring      TemplateType = "ssl_expiring"
	TemplateSSLRenewed       TemplateType = "ssl_renewed"
	TemplateSSLRenewalFailed TemplateType = "ssl_renewal_failed"
	TemplateSchedulerError   TemplateType = "scheduler_error"
	TemplateStartup          TemplateType = "startup"
	TemplateShutdown         TemplateType = "shutdown"
	TemplateDiskLow          TemplateType = "disk_low"
	TemplateDatabaseError    TemplateType = "database_error"
	TemplateUpdateAvailable  TemplateType = "update_available"
	TemplateUpdateInstalled  TemplateType = "update_installed"
	TemplateUpdateFailed     TemplateType = "update_failed"
	TemplateTest             TemplateType = "test"
)

// AllTemplateTypes follows the AI.md PART 17 "Default Templates" table.
var AllTemplateTypes = []TemplateType{
	TemplateSecurityAlert,
	TemplateBackupCompleted,
	TemplateBackupFailed,
	TemplateSSLExpiring,
	TemplateSSLRenewed,
	TemplateSSLRenewalFailed,
	TemplateSchedulerError,
	TemplateStartup,
	TemplateShutdown,
	TemplateDiskLow,
	TemplateDatabaseError,
	TemplateUpdateAvailable,
	TemplateUpdateInstalled,
	TemplateUpdateFailed,
	TemplateTest,
}

// IsValidTemplateType reports whether t is one of the PART 17 templates.
func IsValidTemplateType(t TemplateType) bool {
	for _, known := range AllTemplateTypes {
		if known == t {
			return true
		}
	}
	return false
}

// TemplateData holds common template variables for constructing vars maps.
// Kept for backward compatibility in helpers that build vars maps.
type TemplateData struct {
	SiteName     string
	SiteURL      string
	Year         int
	SupportEmail string
}

// NewTemplateData creates template data with defaults
func NewTemplateData(siteName, siteURL, supportEmail string) *TemplateData {
	return &TemplateData{
		SiteName:     siteName,
		SiteURL:      siteURL,
		Year:         time.Now().Year(),
		SupportEmail: supportEmail,
	}
}

// VarsMap returns the common variables as a map for template substitution
func (td *TemplateData) VarsMap() map[string]string {
	return map[string]string{
		"app_name":  td.SiteName,
		"app_url":   td.SiteURL,
		"fqdn":      td.SiteURL,
		"year":      fmt.Sprintf("%d", td.Year),
		"timestamp": time.Now().Format("2006-01-02 15:04:05 UTC"),
	}
}

// EmailTemplate manages email template rendering.
// Templates use the format:
//
//	Subject: {subject text}
//	---
//	Plain text body with {variable} substitutions.
type EmailTemplate struct{}

// NewEmailTemplate creates a new email template manager
func NewEmailTemplate() *EmailTemplate {
	return &EmailTemplate{}
}

// Render renders a template with {variable} substitutions from vars.
// Returns the subject line and body text.
func (et *EmailTemplate) Render(templateType TemplateType, vars map[string]string) (subject string, body string, err error) {
	return et.RenderLocalized(templateType, "en", vars)
}

// RenderLocalized renders a configured or embedded email template using the
// requested locale for the embedded subject and body strings.
func (et *EmailTemplate) RenderLocalized(templateType TemplateType, lang string, vars map[string]string) (subject string, body string, err error) {
	text, ok := loadCustomTemplate(templateType)
	if ok {
		rendered := ReplaceVars(text, vars)
		if unresolvedTemplateVariable.MatchString(rendered) {
			ok = false
		} else if subject, body, err = parseTemplate(rendered); err == nil {
			return subject, body, nil
		} else {
			ok = false
		}
	}
	if !ok {
		text, ok = loadDefaultTemplate(templateType)
		if !ok {
			return "", "", fmt.Errorf("template %s not found", templateType)
		}
	}
	localizedVars := make(map[string]string, len(vars)+2)
	for key, value := range vars {
		localizedVars[key] = value
	}
	localizedVars["subject"] = localizedEmailString(lang, templateType, "subject", vars)
	localizedVars["body"] = localizedEmailString(lang, templateType, "body", vars)
	rendered := ReplaceVars(text, localizedVars)
	if unresolvedTemplateVariable.MatchString(rendered) {
		return "", "", fmt.Errorf("template %s has an unknown variable", templateType)
	}
	return parseTemplate(rendered)
}

func localizedEmailString(lang string, templateType TemplateType, field string, vars map[string]string) string {
	return ReplaceVars(i18n.T(lang, "email.templates."+string(templateType)+"."+field), vars)
}

// loadCustomTemplate returns an operator-provided template override from
// {config_dir}/template/email/{type}.txt if it exists and is readable.
// Per AI.md PART 17, custom templates take precedence over embedded defaults.
func loadCustomTemplate(templateType TemplateType) (string, bool) {
	configDir := config.GetConfigDir()
	if configDir == "" {
		return "", false
	}
	path := filepath.Join(configDir, "template", "email", string(templateType)+".txt")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", false
	}
	return text, true
}

// ReplaceVars replaces all {variable} placeholders in text with values from vars
func ReplaceVars(text string, vars map[string]string) string {
	for key, val := range vars {
		text = strings.ReplaceAll(text, "{"+key+"}", val)
	}
	return text
}

// parseTemplate splits the rendered template text into subject and body.
// Expects: first line "Subject: {subject}", then "---", then body.
func parseTemplate(text string) (subject, body string, err error) {
	lines := splitLines(text)
	if len(lines) == 0 {
		return "", "", fmt.Errorf("template is empty")
	}
	firstLine := strings.TrimSpace(lines[0])
	if strings.HasPrefix(firstLine, "Subject:") {
		subject = strings.TrimSpace(strings.TrimPrefix(firstLine, "Subject:"))
	} else {
		subject = firstLine
	}
	if subject == "" {
		return "", "", fmt.Errorf("subject cannot be empty")
	}
	bodyStart := 1
	separatorFound := false
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			bodyStart = i + 1
			separatorFound = true
			break
		}
	}
	if !separatorFound {
		return "", "", fmt.Errorf("invalid template syntax at line 1")
	}
	body = joinLines(lines[bodyStart:])
	if strings.TrimSpace(body) == "" {
		return "", "", fmt.Errorf("body cannot be empty")
	}
	return subject, body, nil
}

func splitLines(s string) []string {
	var lines []string
	var current bytes.Buffer
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, current.String())
			current.Reset()
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		lines = append(lines, current.String())
	}
	return lines
}

func joinLines(lines []string) string {
	var buf bytes.Buffer
	for i, line := range lines {
		if i > 0 {
			buf.WriteRune('\n')
		}
		buf.WriteString(line)
	}
	return buf.String()
}

// PreviewTemplate renders a template with sample data for UI preview
func (et *EmailTemplate) PreviewTemplate(templateType TemplateType, siteName, siteURL string, replyTo ...string) (subject string, body string, err error) {
	return et.PreviewTemplateLocalized(templateType, "en", siteName, siteURL, replyTo...)
}

// PreviewTemplateLocalized renders a preview using the selected email locale.
func (et *EmailTemplate) PreviewTemplateLocalized(templateType TemplateType, lang, siteName, siteURL string, replyTo ...string) (subject string, body string, err error) {
	vars := map[string]string{
		"app_name":              siteName,
		"app_url":               siteURL,
		"fqdn":                  siteURL,
		"timestamp":             time.Now().Format("2006-01-02 15:04:05 UTC"),
		"year":                  fmt.Sprintf("%d", time.Now().Year()),
		"event":                 "Rate limit exceeded",
		"ip":                    "192.168.1.100",
		"details":               "100 requests in 10 seconds",
		"severity":              "warning",
		"action_required":       "Review the activity",
		"filename":              siteName + "_backup_2006-01-02_150405.tar.gz",
		"size":                  "12.5 MB",
		"error":                 "sample error",
		"since":                 "2026-06-14 02:00:00 UTC",
		"expires_in":            "14 days",
		"expiry_date":           "2026-07-01",
		"valid_until":           "2027-07-01",
		"days_left":             "3",
		"next_retry":            "2026-06-14 03:00:00 UTC",
		"previous_version":      "1.0.0",
		"domain":                siteURL,
		"task_name":             "geoip_update",
		"hostname":              "search.example.com",
		"mode":                  "production",
		"path":                  "/var/lib/search",
		"used_percent":          "91.4",
		"free":                  "4.2 GB",
		"threshold_percent":     "85",
		"next_run":              "2026-06-14 03:00:00 UTC",
		"alert_type":            "System Warning",
		"alert_level":           "warning",
		"message":               "High CPU usage detected",
		"period_start":          "2026-06-06",
		"period_end":            "2026-06-13",
		"total_searches":        "15000",
		"error_count":           "12",
		"current_version":       "1.0.0",
		"new_version":           "1.1.0",
		"release_date":          "2026-06-13",
		"release_notes":         "Bug fixes and performance improvements.",
		"update_url":            siteURL + "/update",
		"scheduled_at":          "2026-06-14 02:00:00 UTC",
		"duration":              "30 minutes",
		"reason":                "Database maintenance",
		"affected_services":     "search, alerts",
		"breach_description":    "Unusual API access pattern detected",
		"affected_users":        "0",
		"ip_addresses":          "10.0.0.1, 10.0.0.2",
		"sent_at":               time.Now().Format("2006-01-02 15:04:05 UTC"),
		"notification_reply_to": "admin@" + siteURL,
		"onion_url":             "",
		"onion_address":         "",
		"i2p_url":               "",
		"i2p_address":           "",
	}
	if templateType == TemplateSecurityAlert {
		vars["subject_prefix"] = i18n.T(lang, "email.security_alert.subject_prefix")
		vars["heading"] = i18n.T(lang, "email.security_alert.heading")
		vars["label_source_ip"] = i18n.T(lang, "email.security_alert.label_source_ip")
		vars["label_details"] = i18n.T(lang, "email.security_alert.label_details")
		vars["label_from"] = i18n.T(lang, "email.common.label_from")
		vars["label_time"] = i18n.T(lang, "email.common.label_time")
	}
	for key, value := range previewVariableNames(siteName, siteURL) {
		if _, exists := vars[key]; !exists {
			vars[key] = value
		}
	}
	if len(replyTo) > 0 && replyTo[0] != "" {
		vars["notification_reply_to"] = replyTo[0]
	}
	return et.RenderLocalized(templateType, lang, vars)
}

var unresolvedTemplateVariable = regexp.MustCompile(`\{[^{}]+\}`)

// ValidateTemplates renders every built-in and configured template with the
// PART 17 preview sample and returns blocking validation errors.
func (et *EmailTemplate) ValidateTemplates(siteName, siteURL string, replyTo ...string) (map[TemplateType][2]string, []error, []string) {
	previews := make(map[TemplateType][2]string, len(AllTemplateTypes))
	var validationErrors []error
	var warnings []string
	for _, templateType := range AllTemplateTypes {
		if raw, ok := loadCustomTemplate(templateType); ok {
			known := previewVariableNames(siteName, siteURL)
			for _, variable := range unresolvedTemplateVariable.FindAllString(raw, -1) {
				name := strings.TrimSuffix(strings.TrimPrefix(variable, "{"), "}")
				if _, exists := known[name]; !exists {
					message := "unknown variable: " + variable
					if suggestion := closestTemplateVariable(name, known); suggestion != "" {
						message += ". Did you mean {" + suggestion + "}?"
					}
					validationErrors = append(validationErrors, fmt.Errorf("%s: %s", templateType, message))
					break
				}
			}
			if _, _, err := parseTemplate(raw); err != nil {
				validationErrors = append(validationErrors, fmt.Errorf("%s: %w", templateType, err))
			}
		}
		subject, body, err := et.PreviewTemplate(templateType, siteName, siteURL, replyTo...)
		if err != nil {
			validationErrors = append(validationErrors, fmt.Errorf("%s: %w", templateType, err))
			continue
		}
		previews[templateType] = [2]string{subject, body}
		if len([]rune(subject)) > 78 {
			warnings = append(warnings, fmt.Sprintf("%s: subject is longer than 78 characters", templateType))
		}
		if templateType == TemplateSecurityAlert && !strings.Contains(strings.ToLower(body), "contact") {
			warnings = append(warnings, fmt.Sprintf("%s: recommended contact information is missing", templateType))
		}
		for _, rendered := range []string{subject, body} {
			if unknown := unresolvedTemplateVariable.FindString(rendered); unknown != "" {
				validationErrors = append(validationErrors, fmt.Errorf("%s: unknown variable: %s", templateType, unknown))
				break
			}
		}
		if strings.TrimSpace(subject) == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%s: subject cannot be empty", templateType))
		}
		if strings.TrimSpace(body) == "" {
			validationErrors = append(validationErrors, fmt.Errorf("%s: body cannot be empty", templateType))
		}
	}
	return previews, validationErrors, warnings
}

func closestTemplateVariable(name string, known map[string]string) string {
	best, bestDistance := "", 3
	for candidate := range known {
		distance := templateEditDistance(name, candidate)
		if distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	return best
}

func templateEditDistance(left, right string) int {
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range left {
		current := make([]int, len(right)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range right {
			cost := 0
			if leftRune != rightRune {
				cost = 1
			}
			current[rightIndex+1] = min(current[rightIndex]+1, previous[rightIndex+1]+1, previous[rightIndex]+cost)
		}
		previous = current
	}
	return previous[len(right)]
}

func previewVariableNames(siteName, siteURL string) map[string]string {
	return map[string]string{
		"app_name": siteName, "app_url": siteURL, "fqdn": siteURL, "version": "1.0.0",
		"timestamp": time.Now().Format("2006-01-02 15:04:05 UTC"), "year": fmt.Sprint(time.Now().Year()),
		"event": "Rate limit exceeded", "ip": "192.168.1.100", "details": "Sample details",
		"severity": "warning", "action_required": "Review the activity", "filename": "sample.tar.gz",
		"heading": "Sample notification", "intro": "A sample notification is available.",
		"label_details": "Details", "label_from": "From", "label_source_ip": "Source IP",
		"label_time": "Time", "label_current_version": "Current version", "label_new_version": "New version",
		"label_release_date": "Release date", "label_release_notes": "Release notes", "label_view_update": "View update",
		"manual_update_command": "search --update yes", "subject_prefix": "[Search]",
		"size": "12.5 MB", "error": "sample error", "since": "2026-06-14 02:00:00 UTC",
		"expires_in": "14 days", "expiry_date": "2026-07-01", "valid_until": "2027-07-01",
		"days_left": "3", "next_retry": "2026-06-14 03:00:00 UTC", "previous_version": "1.0.0",
		"domain": siteURL, "task_name": "geoip_update", "hostname": "search.example.com",
		"mode": "production", "path": "/var/lib/search", "used_percent": "91.4",
		"free": "4.2 GB", "threshold_percent": "85", "next_run": "2026-06-14 03:00:00 UTC",
		"alert_type": "System Warning", "alert_level": "warning", "message": "Sample message",
		"period_start": "2026-06-06", "period_end": "2026-06-13", "total_searches": "15000",
		"error_count": "12", "current_version": "1.0.0", "new_version": "1.1.0",
		"release_date": "2026-06-13", "release_notes": "Sample release notes",
		"update_url": siteURL + "/update", "scheduled_at": "2026-06-14 02:00:00 UTC",
		"duration": "30 minutes", "reason": "Database maintenance", "affected_services": "search",
		"breach_description": "Sample security event", "affected_users": "0", "ip_addresses": "192.168.1.100",
		"sent_at": time.Now().Format("2006-01-02 15:04:05 UTC"), "notification_reply_to": "admin@" + siteURL,
		"onion_url": "", "onion_address": "", "i2p_url": "", "i2p_address": "",
	}
}

// TemplateInfo contains metadata about an email template
type TemplateInfo struct {
	Type           TemplateType `json:"type"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	IsAccountEmail bool         `json:"is_account_email"`
}

// GetAllTemplateTypes returns a list of all available email template types
func GetAllTemplateTypes() []TemplateInfo {
	return []TemplateInfo{
		{TemplateSecurityAlert, "Security Alert", "Security-related notifications", false},
		{TemplateBackupCompleted, "Backup Completed", "Backup completion notification", false},
		{TemplateBackupFailed, "Backup Failed", "Backup failure notification", false},
		{TemplateSSLExpiring, "SSL Expiring", "SSL certificate expiration warning", false},
		{TemplateSSLRenewed, "SSL Renewed", "SSL certificate renewal confirmation", false},
		{TemplateSSLRenewalFailed, "SSL Renewal Failed", "SSL certificate renewal failure notification", false},
		{TemplateSchedulerError, "Scheduler Error", "Scheduled task failure notification", false},
		{TemplateStartup, "Server Started", "Server startup notification", false},
		{TemplateShutdown, "Server Stopped", "Server shutdown notification", false},
		{TemplateDiskLow, "Disk Space Low", "Low disk space notification", false},
		{TemplateDatabaseError, "Database Connection Problem", "Database connection failure notification", false},
		{TemplateUpdateAvailable, "Update Available", "New version available notification", false},
		{TemplateUpdateInstalled, "Update Installed", "Self-update completion notification", false},
		{TemplateUpdateFailed, "Update Failed", "Self-update failure notification", false},
		{TemplateTest, "Test Email", "Test email to verify SMTP configuration", false},
	}
}

// IsAccountEmail returns true if the template is for account/security emails.
// This project has no user accounts; all templates are operator/system notifications.
func IsAccountEmail(_ TemplateType) bool {
	return false
}
