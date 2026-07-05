package email

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// TemplateType represents an email template type
type TemplateType string

const (
	TemplateSecurityAlert     TemplateType = "security_alert"
	TemplateBackupCompleted   TemplateType = "backup_complete"
	TemplateBackupFailed      TemplateType = "backup_failed"
	TemplateSSLExpiring       TemplateType = "ssl_expiring"
	TemplateSSLRenewed        TemplateType = "ssl_renewed"
	TemplateSchedulerError    TemplateType = "scheduler_error"
	TemplateTest              TemplateType = "test"
	TemplateAdminAlert        TemplateType = "admin_alert"
	TemplateWeeklyReport      TemplateType = "weekly_report"
	TemplateUpdateAvailable   TemplateType = "update_available"
	TemplateMaintenanceNotice TemplateType = "maintenance_notice"
	TemplateBreachAdminAlert  TemplateType = "breach_admin_alert"
)

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
	text, ok := loadCustomTemplate(templateType)
	if !ok {
		text, ok = rawTemplates[templateType]
		if !ok {
			return "", "", fmt.Errorf("template %s not found", templateType)
		}
	}
	rendered := ReplaceVars(text, vars)
	return parseTemplate(rendered)
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
		return "", "", fmt.Errorf("template subject is empty")
	}
	bodyStart := 1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			bodyStart = i + 1
			break
		}
	}
	body = joinLines(lines[bodyStart:])
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
func (et *EmailTemplate) PreviewTemplate(templateType TemplateType, siteName, siteURL string) (subject string, body string, err error) {
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
		"error":                 "disk space exhausted",
		"expires_in":            "14 days",
		"expiry_date":           "2026-07-01",
		"valid_until":           "2027-07-01",
		"domain":                siteURL,
		"task_name":             "geoip_update",
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
	return et.Render(templateType, vars)
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
		{TemplateSchedulerError, "Scheduler Error", "Scheduled task failure notification", false},
		{TemplateTest, "Test Email", "Test email to verify SMTP configuration", false},
		{TemplateAdminAlert, "Admin Alert", "System alerts for administrators", false},
		{TemplateWeeklyReport, "Weekly Report", "Weekly usage statistics summary", false},
		{TemplateUpdateAvailable, "Update Available", "New version available notification", false},
		{TemplateMaintenanceNotice, "Maintenance Notice", "Scheduled maintenance alert", false},
		{TemplateBreachAdminAlert, "Breach Admin Alert", "Security breach alert for admins", false},
	}
}

// IsAccountEmail returns true if the template is for account/security emails.
// This project has no user accounts; all templates are operator/system notifications.
func IsAccountEmail(_ TemplateType) bool {
	return false
}

// rawTemplates contains the built-in plain text templates.
// Format: "Subject: {subject}\n---\nbody with {variable} substitutions"
// Custom templates override these by placing files in {config_dir}/template/email/.
var rawTemplates = map[TemplateType]string{
	TemplateSecurityAlert: `Subject: Security Alert - {app_name}
---
SECURITY ALERT

From: {app_name} ({fqdn})
Time: {timestamp}

{event}

Details:
  Source IP: {ip}
  {details}

--
{app_name}
{app_url}`,

	TemplateBackupCompleted: `Subject: Backup Complete - {app_name}
---
BACKUP COMPLETE

From: {app_name} ({fqdn})
Time: {timestamp}

Your backup completed successfully.

Filename: {filename}
Size: {size}

--
{app_name}
{app_url}`,

	TemplateBackupFailed: `Subject: Backup Failed - {app_name}
---
BACKUP FAILED

From: {app_name} ({fqdn})
Time: {timestamp}

A backup has failed.

Filename: {filename}
Error: {error}

Please check your backup configuration and ensure sufficient disk space is available.

--
{app_name}
{app_url}`,

	TemplateSSLExpiring: `Subject: SSL Certificate Expiring - {app_name}
---
SSL CERTIFICATE EXPIRING

From: {app_name} ({fqdn})
Time: {timestamp}

Your SSL certificate for {domain} is expiring soon.

Domain:      {domain}
Expires in:  {expires_in}
Expiry date: {expiry_date}

Please renew your certificate before it expires to ensure uninterrupted secure connections.

--
{app_name}
{app_url}`,

	TemplateSSLRenewed: `Subject: SSL Certificate Renewed - {app_name}
---
SSL CERTIFICATE RENEWED

From: {app_name} ({fqdn})
Time: {timestamp}

Your SSL certificate for {domain} has been successfully renewed.

Domain:      {domain}
Valid until: {valid_until}

Your secure connections will continue without interruption.

--
{app_name}
{app_url}`,

	TemplateSchedulerError: `Subject: Scheduled Task Failed - {app_name}
---
SCHEDULED TASK FAILED

From: {app_name} ({fqdn})
Time: {timestamp}

The scheduled task "{task_name}" failed.

Error:    {error}
Next run: {next_run}

Please review the task configuration and check system logs for more details.

--
{app_name}
{app_url}`,

	TemplateTest: `Subject: Test Email - {app_name}
---
TEST EMAIL

From: {app_name} ({fqdn})
Time: {timestamp}

This is a test email from {app_name}.

If you received this email, your email configuration is working correctly.

Sent at:    {sent_at}
Server URL: {app_url}

--
{app_name}
{app_url}`,

	TemplateAdminAlert: `Subject: [{alert_level}] {alert_type} - {app_name}
---
ADMIN ALERT

From: {app_name} ({fqdn})
Time: {timestamp}

Level:   {alert_level}
Type:    {alert_type}
Message: {message}

--
{app_name}
{app_url}`,

	TemplateWeeklyReport: `Subject: Weekly Report - {app_name}
---
WEEKLY REPORT

From: {app_name} ({fqdn})
Period: {period_start} to {period_end}

Total searches: {total_searches}
Errors:         {error_count}

--
{app_name}
{app_url}`,

	TemplateUpdateAvailable: `Subject: Update Available - {app_name}
---
UPDATE AVAILABLE

From: {app_name} ({fqdn})
Time: {timestamp}

A new version of {app_name} is available.

Current version: {current_version}
New version:     {new_version}
Release date:    {release_date}

Release notes:
{release_notes}

View update: {update_url}

--
{app_name}
{app_url}`,

	TemplateMaintenanceNotice: `Subject: Scheduled Maintenance - {app_name}
---
SCHEDULED MAINTENANCE

From: {app_name} ({fqdn})
Time: {timestamp}

Scheduled maintenance has been planned for {app_name}.

Scheduled at:      {scheduled_at}
Expected duration: {duration}
Reason:            {reason}
Affected services: {affected_services}

During maintenance, the service may be temporarily unavailable.

--
{app_name}
{app_url}`,

	TemplateBreachAdminAlert: `Subject: [{severity}] Security Breach Detected - {app_name}
---
SECURITY BREACH DETECTED

From: {app_name} ({fqdn})
Time: {timestamp}

Severity:        {severity}
Description:     {breach_description}
Affected users:  {affected_users}
Source IPs:      {ip_addresses}

ACTION REQUIRED: {action_required}

--
{app_name}
{app_url}`,
}
