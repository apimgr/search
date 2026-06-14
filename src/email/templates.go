package email

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"github.com/apimgr/search/src/version"
)

// TemplateType represents an email template type
type TemplateType string

const (
	TemplateAdminAlert TemplateType = "admin_alert"
	TemplateWeeklyReport      TemplateType = "weekly_report"
	TemplateSecurityAlert     TemplateType = "security_alert"
	TemplateBackupCompleted   TemplateType = "backup_complete"
	TemplateUpdateAvailable   TemplateType = "update_available"
	TemplateMaintenanceNotice TemplateType = "maintenance_notice"
	// Operator/system notification templates per AI.md PART 18
	TemplateBackupFailed     TemplateType = "backup_failed"
	TemplateSSLExpiring      TemplateType = "ssl_expiring"
	TemplateSSLRenewed       TemplateType = "ssl_renewed"
	TemplateSchedulerError   TemplateType = "scheduler_error"
	TemplateBreachAdminAlert TemplateType = "breach_admin_alert"
	TemplateTest             TemplateType = "test"
)

// TemplateData holds common template data
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

// AdminAlertData holds data for admin alert
type AdminAlertData struct {
	*TemplateData
	AlertType  string
	AlertLevel string
	Message    string
	Details    map[string]string
	OccurredAt time.Time
}

// WeeklyReportData holds data for weekly report
type WeeklyReportData struct {
	*TemplateData
	PeriodStart   time.Time
	PeriodEnd     time.Time
	TotalSearches int
	EngineStats   map[string]int
	ErrorCount    int
}

// SecurityAlertData holds data for security alert
type SecurityAlertData struct {
	*TemplateData
	Event          string
	Severity       string
	IPAddress      string
	Details        string
	OccurredAt     time.Time
	ActionRequired string
}

// BackupCompletedData holds data for backup completion notification
type BackupCompletedData struct {
	*TemplateData
	BackupName string
	BackupSize string
	CreatedAt  time.Time
	FileCount  int
	Duration   string
}

// UpdateAvailableData holds data for update available notification
type UpdateAvailableData struct {
	*TemplateData
	CurrentVersion string
	NewVersion     string
	ReleaseDate    time.Time
	ReleaseNotes   string
	UpdateURL      string
}

// MaintenanceNoticeData holds data for maintenance notice email
type MaintenanceNoticeData struct {
	*TemplateData
	ScheduledAt      time.Time
	Duration         string
	Reason           string
	AffectedServices []string
}

// BackupFailedData holds data for backup failure notification
// Per AI.md PART 18: backup_failed template
type BackupFailedData struct {
	*TemplateData
	BackupName string
	Error      string
	FailedAt   time.Time
}

// SSLExpiringData holds data for SSL expiration warning
// Per AI.md PART 18: ssl_expiring template
type SSLExpiringData struct {
	*TemplateData
	Domain    string
	ExpiresAt time.Time
	DaysLeft  int
	RenewLink string
}

// SSLRenewedData holds data for SSL renewal confirmation
// Per AI.md PART 18: ssl_renewed template
type SSLRenewedData struct {
	*TemplateData
	Domain     string
	RenewedAt  time.Time
	ValidUntil time.Time
}

// SchedulerErrorData holds data for scheduler error notification
// Per AI.md PART 18: scheduler_error template
type SchedulerErrorData struct {
	*TemplateData
	TaskName    string
	Error       string
	FailedAt    time.Time
	TaskDetails map[string]string
}

// BreachAdminAlertData holds data for breach alert to admins
// Per AI.md PART 18: breach_admin_alert template
type BreachAdminAlertData struct {
	*TemplateData
	Severity          string
	DetectedAt        time.Time
	BreachDescription string
	AffectedUsers     int
	IPAddresses       []string
	ActionRequired    string
}

// TestEmailData holds data for test email
// Per AI.md PART 18: test template
type TestEmailData struct {
	*TemplateData
	SentAt time.Time
}

// EmailTemplate manages email template rendering
type EmailTemplate struct {
	templates map[TemplateType]*template.Template
}

// NewEmailTemplate creates a new email template manager
func NewEmailTemplate() *EmailTemplate {
	et := &EmailTemplate{
		templates: make(map[TemplateType]*template.Template),
	}

	// Parse all templates
	// Option missingkey=error ensures templates fail on missing data fields
	for templateType, tmpl := range rawTemplates {
		t, err := template.New(string(templateType)).Option("missingkey=error").Parse(tmpl)
		if err != nil {
			// Log error but don't fail - use empty template
			continue
		}
		et.templates[templateType] = t
	}

	return et
}

// Render renders a template with the given data
func (et *EmailTemplate) Render(templateType TemplateType, data interface{}) (subject string, body string, err error) {
	t, ok := et.templates[templateType]
	if !ok {
		return "", "", fmt.Errorf("template %s not found", templateType)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", "", err
	}

	// Templates include subject on first line
	content := buf.String()
	lines := splitLines(content)
	if len(lines) > 0 {
		subject = lines[0]
		body = joinLines(lines[1:])
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

// rawTemplates contains the raw template strings
var rawTemplates = map[TemplateType]string{
	TemplateAdminAlert: `[{{.AlertLevel}}] {{.AlertType}} - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: {{if eq .AlertLevel "critical"}}#ff6b6b{{else if eq .AlertLevel "warning"}}#ffd93d{{else}}#00d9ff{{end}}; margin-top: 0;">Admin Alert: {{.AlertType}}</h1>
        <p><strong>Level:</strong> {{.AlertLevel}}</p>
        <p><strong>Time:</strong> {{.OccurredAt.Format "Jan 2, 2006 3:04:05 PM"}}</p>
        <p><strong>Message:</strong></p>
        <p style="background: #0f0f1a; padding: 15px; border-radius: 4px; font-family: monospace;">{{.Message}}</p>
        {{if .Details}}
        <p><strong>Details:</strong></p>
        <ul>
        {{range $key, $value := .Details}}
            <li><strong>{{$key}}:</strong> {{$value}}</li>
        {{end}}
        </ul>
        {{end}}
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">{{.SiteName}} Admin Alert System</p>
    </div>
</body>
</html>`,

	TemplateWeeklyReport: `Weekly Report - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Weekly Report</h1>
        <p><strong>Period:</strong> {{.PeriodStart.Format "Jan 2"}} - {{.PeriodEnd.Format "Jan 2, 2006"}}</p>

        <h2 style="color: #00d9ff; font-size: 18px;">Statistics</h2>
        <table style="width: 100%; border-collapse: collapse;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333;">Total Searches</td><td style="padding: 8px; border-bottom: 1px solid #333; text-align: right;"><strong>{{.TotalSearches}}</strong></td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333;">Errors</td><td style="padding: 8px; border-bottom: 1px solid #333; text-align: right;"><strong>{{.ErrorCount}}</strong></td></tr>
        </table>

        {{if .EngineStats}}
        <h2 style="color: #00d9ff; font-size: 18px;">Engine Usage</h2>
        <table style="width: 100%; border-collapse: collapse;">
        {{range $engine, $count := .EngineStats}}
            <tr><td style="padding: 8px; border-bottom: 1px solid #333;">{{$engine}}</td><td style="padding: 8px; border-bottom: 1px solid #333; text-align: right;">{{$count}}</td></tr>
        {{end}}
        </table>
        {{end}}

        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateSecurityAlert: `[Security Alert] {{.Event}} - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ff6b6b; margin-top: 0;">Security Alert</h1>
        <p><strong>Event:</strong> {{.Event}}</p>
        <p><strong>Severity:</strong> <span style="color: {{if eq .Severity "critical"}}#ff6b6b{{else if eq .Severity "high"}}#ffd93d{{else}}#00d9ff{{end}};">{{.Severity}}</span></p>
        <p><strong>Time:</strong> {{.OccurredAt.Format "Jan 2, 2006 3:04:05 PM"}}</p>
        <p><strong>IP Address:</strong> {{.IPAddress}}</p>

        {{if .Details}}
        <p><strong>Details:</strong></p>
        <pre style="background: #0f0f1a; padding: 15px; border-radius: 4px; overflow-x: auto;">{{.Details}}</pre>
        {{end}}

        {{if .ActionRequired}}
        <p style="background: #ff6b6b22; border-left: 4px solid #ff6b6b; padding: 15px; margin: 20px 0;">
            <strong>Action Required:</strong> {{.ActionRequired}}
        </p>
        {{end}}

        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">{{.SiteName}} Security System</p>
    </div>
</body>
</html>`,

	TemplateBackupCompleted: `Backup Completed - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00ff88; margin-top: 0;">Backup Completed</h1>
        <p>A backup of your {{.SiteName}} data has been successfully created.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Backup Name</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.BackupName}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Size</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.BackupSize}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Files</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.FileCount}} files</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Duration</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.Duration}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Created At</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</td></tr>
        </table>
        <p style="color: #888; font-size: 14px;">This is an automated notification from the {{.SiteName}} backup system.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateUpdateAvailable: `Update Available - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Update Available</h1>
        <p>A new version of {{.SiteName}} is available!</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Current Version</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.CurrentVersion}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">New Version</td><td style="padding: 8px; border-bottom: 1px solid #333; color: #00ff88;"><strong>{{.NewVersion}}</strong></td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Release Date</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.ReleaseDate.Format "Jan 2, 2006"}}</td></tr>
        </table>
        {{if .ReleaseNotes}}
        <h2 style="color: #00d9ff; font-size: 16px;">What's New</h2>
        <div style="background: #0f0f1a; padding: 15px; border-radius: 4px; white-space: pre-wrap;">{{.ReleaseNotes}}</div>
        {{end}}
        <p style="margin-top: 20px;"><a href="{{.UpdateURL}}" style="display: inline-block; background: #00d9ff; color: #1a1a2e; padding: 12px 24px; text-decoration: none; border-radius: 4px; font-weight: bold;">View Update</a></p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateMaintenanceNotice: `Scheduled Maintenance - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ffd93d; margin-top: 0;">Scheduled Maintenance</h1>
        <p>We have scheduled maintenance for {{.SiteName}}.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Scheduled Time</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.ScheduledAt.Format "Jan 2, 2006 3:04 PM MST"}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Expected Duration</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.Duration}}</td></tr>
            {{if .Reason}}<tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Reason</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.Reason}}</td></tr>{{end}}
        </table>
        {{if .AffectedServices}}
        <h2 style="color: #ffd93d; font-size: 16px;">Affected Services</h2>
        <ul>
        {{range .AffectedServices}}
            <li>{{.}}</li>
        {{end}}
        </ul>
        {{end}}
        <p style="background: #ffd93d22; border-left: 4px solid #ffd93d; padding: 15px; margin: 20px 0;">
            During maintenance, the service may be temporarily unavailable. We apologize for any inconvenience.
        </p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateBackupFailed: `Backup Failed - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ff6b6b; margin-top: 0;">Backup Failed</h1>
        <p>A backup of your {{.SiteName}} data has failed.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Backup Name</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.BackupName}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Failed At</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.FailedAt.Format "Jan 2, 2006 3:04 PM"}}</td></tr>
        </table>
        <p><strong>Error:</strong></p>
        <pre style="background: #0f0f1a; padding: 15px; border-radius: 4px; overflow-x: auto; color: #ff6b6b;">{{.Error}}</pre>
        <p style="background: #ff6b6b22; border-left: 4px solid #ff6b6b; padding: 15px; margin: 20px 0;">
            Please check your backup configuration and ensure sufficient disk space is available.
        </p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateSSLExpiring: `SSL Certificate Expiring - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ffd93d; margin-top: 0;">SSL Certificate Expiring</h1>
        <p>Your SSL certificate is expiring soon and needs to be renewed.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Domain</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.Domain}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Expires</td><td style="padding: 8px; border-bottom: 1px solid #333; color: {{if le .DaysLeft 7}}#ff6b6b{{else}}#ffd93d{{end}};">{{.ExpiresAt.Format "Jan 2, 2006"}} ({{.DaysLeft}} days)</td></tr>
        </table>
        <p style="background: #ffd93d22; border-left: 4px solid #ffd93d; padding: 15px; margin: 20px 0;">
            {{if le .DaysLeft 3}}URGENT: Your certificate expires very soon. Renew immediately to avoid service disruption.{{else}}Please renew your certificate before it expires to ensure uninterrupted secure connections.{{end}}
        </p>
        {{if .RenewLink}}<p><a href="{{.RenewLink}}" style="display: inline-block; background: #00d9ff; color: #1a1a2e; padding: 12px 24px; text-decoration: none; border-radius: 4px; font-weight: bold;">Renew Certificate</a></p>{{end}}
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateSSLRenewed: `SSL Certificate Renewed - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00ff88; margin-top: 0;">SSL Certificate Renewed</h1>
        <p>Your SSL certificate has been successfully renewed.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Domain</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.Domain}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Renewed At</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.RenewedAt.Format "Jan 2, 2006 3:04 PM"}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Valid Until</td><td style="padding: 8px; border-bottom: 1px solid #333; color: #00ff88;">{{.ValidUntil.Format "Jan 2, 2006"}}</td></tr>
        </table>
        <p style="background: #00ff8822; border-left: 4px solid #00ff88; padding: 15px; margin: 20px 0;">
            Your secure connections will continue without interruption.
        </p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateSchedulerError: `Scheduled Task Failed - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ff6b6b; margin-top: 0;">Scheduled Task Failed</h1>
        <p>A scheduled task has failed to execute successfully.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Task Name</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.TaskName}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Failed At</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.FailedAt.Format "Jan 2, 2006 3:04 PM"}}</td></tr>
        </table>
        <p><strong>Error:</strong></p>
        <pre style="background: #0f0f1a; padding: 15px; border-radius: 4px; overflow-x: auto; color: #ff6b6b;">{{.Error}}</pre>
        {{if .TaskDetails}}
        <p><strong>Task Details:</strong></p>
        <ul>
        {{range $key, $value := .TaskDetails}}
            <li><strong>{{$key}}:</strong> {{$value}}</li>
        {{end}}
        </ul>
        {{end}}
        <p style="background: #ff6b6b22; border-left: 4px solid #ff6b6b; padding: 15px; margin: 20px 0;">
            Please review the task configuration and check system logs for more details.
        </p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">{{.SiteName}} Scheduler System</p>
    </div>
</body>
</html>`,

	TemplateBreachAdminAlert: `[{{.Severity}}] Security Breach Detected - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ff6b6b; margin-top: 0;">Security Breach Detected</h1>
        <p><strong>Severity:</strong> <span style="color: {{if eq .Severity "CRITICAL"}}#ff6b6b{{else if eq .Severity "HIGH"}}#ffd93d{{else}}#00d9ff{{end}}; font-weight: bold;">{{.Severity}}</span></p>
        <p><strong>Detected:</strong> {{.DetectedAt.Format "Jan 2, 2006 3:04:05 PM"}}</p>
        <p><strong>Description:</strong></p>
        <div style="background: #0f0f1a; padding: 15px; border-radius: 4px;">{{.BreachDescription}}</div>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Affected Users</td><td style="padding: 8px; border-bottom: 1px solid #333; color: #ff6b6b;"><strong>{{.AffectedUsers}}</strong></td></tr>
        </table>
        {{if .IPAddresses}}
        <p><strong>Source IP Addresses:</strong></p>
        <ul style="font-family: monospace;">
        {{range .IPAddresses}}
            <li>{{.}}</li>
        {{end}}
        </ul>
        {{end}}
        <p style="background: #ff6b6b; color: #ffffff; padding: 15px; margin: 20px 0; border-radius: 4px;">
            <strong>ACTION REQUIRED:</strong> {{.ActionRequired}}
        </p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">{{.SiteName}} Security Alert System</p>
    </div>
</body>
</html>`,

	TemplateTest: `Test Email - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Test Email</h1>
        <p>This is a test email from {{.SiteName}}.</p>
        <p>If you received this email, your email configuration is working correctly.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Sent At</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.SentAt.Format "Jan 2, 2006 3:04:05 PM"}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Server URL</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.SiteURL}}</td></tr>
        </table>
        <p style="background: #00ff8822; border-left: 4px solid #00ff88; padding: 15px; margin: 20px 0;">
            Email delivery is working properly.
        </p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,
}

// TemplateInfo contains metadata about an email template
// Per AI.md PART 31: Account emails vs Notification emails
type TemplateInfo struct {
	Type           TemplateType `json:"type"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	IsAccountEmail bool         `json:"is_account_email"`
}

// GetAllTemplateTypes returns a list of all available email template types
func GetAllTemplateTypes() []TemplateInfo {
	return []TemplateInfo{
		// Operator/system notification emails
		{TemplateAdminAlert, "Admin Alert", "System alerts for administrators", false},
		{TemplateWeeklyReport, "Weekly Report", "Weekly usage statistics summary", false},
		{TemplateSecurityAlert, "Security Alert", "Security-related notifications", false},
		{TemplateBackupCompleted, "Backup Completed", "Backup completion notification", false},
		{TemplateUpdateAvailable, "Update Available", "New version available notification", false},
		{TemplateMaintenanceNotice, "Maintenance Notice", "Scheduled maintenance alert", false},
		// Additional operator notification templates per AI.md PART 18
		{TemplateBackupFailed, "Backup Failed", "Backup failure notification", false},
		{TemplateSSLExpiring, "SSL Expiring", "SSL certificate expiration warning", false},
		{TemplateSSLRenewed, "SSL Renewed", "SSL certificate renewal confirmation", false},
		{TemplateSchedulerError, "Scheduler Error", "Scheduled task failure notification", false},
		{TemplateBreachAdminAlert, "Breach Admin Alert", "Security breach alert for admins", false},
		{TemplateTest, "Test Email", "Test email to verify SMTP configuration", false},
	}
}

// IsAccountEmail returns true if the template is for account/security emails.
// This project has no user accounts; all templates are operator/system notifications.
func IsAccountEmail(_ TemplateType) bool {
	return false
}

// PreviewTemplate renders a template with sample data for preview
func (et *EmailTemplate) PreviewTemplate(templateType TemplateType, siteName, siteURL string) (subject string, body string, err error) {
	baseData := NewTemplateData(siteName, siteURL, "support@"+siteURL)
	sampleTime := time.Now()

	var data interface{}

	switch templateType {
	case TemplateAdminAlert:
		data = &AdminAlertData{
			TemplateData: baseData,
			AlertType:    "High CPU Usage",
			AlertLevel:   "warning",
			Message:      "Server CPU usage exceeded 90% threshold.",
			Details: map[string]string{
				"Current Usage": "92%",
				"Threshold":     "90%",
				"Duration":      "5 minutes",
			},
			OccurredAt: sampleTime,
		}
	case TemplateWeeklyReport:
		data = &WeeklyReportData{
			TemplateData:  baseData,
			PeriodStart:   sampleTime.AddDate(0, 0, -7),
			PeriodEnd:     sampleTime,
			TotalSearches: 15423,
			EngineStats: map[string]int{
				"Google":     8234,
				"DuckDuckGo": 4521,
				"Brave":      2668,
			},
			ErrorCount: 12,
		}
	case TemplateSecurityAlert:
		data = &SecurityAlertData{
			TemplateData:   baseData,
			Event:          "Multiple Failed Login Attempts",
			Severity:       "high",
			IPAddress:      "45.33.32.156",
			Details:        "5 failed login attempts in the last 10 minutes.",
			OccurredAt:     sampleTime,
			ActionRequired: "Review the activity and consider blocking the IP address.",
		}
	case TemplateBackupCompleted:
		data = &BackupCompletedData{
			TemplateData: baseData,
			BackupName:   "search-backup-20251220-153045.tar.gz",
			BackupSize:   "12.5 MB",
			CreatedAt:    sampleTime,
			FileCount:    247,
			Duration:     "2.3 seconds",
		}
	case TemplateUpdateAvailable:
		data = &UpdateAvailableData{
			TemplateData:   baseData,
			CurrentVersion: "1.2.3",
			NewVersion:     "1.3.0",
			ReleaseDate:    sampleTime,
			ReleaseNotes:   "• New search engine integrations\n• Improved caching performance\n• Bug fixes and security updates",
			UpdateURL:      "https://github.com/apimgr/search/releases/v1.3.0",
		}
	case TemplateMaintenanceNotice:
		data = &MaintenanceNoticeData{
			TemplateData:     baseData,
			ScheduledAt:      sampleTime.Add(24 * time.Hour),
			Duration:         "30 minutes",
			Reason:           "Database maintenance and performance optimization",
			AffectedServices: []string{"Search API", "GraphQL Endpoint"},
		}
	case TemplateBackupFailed:
		data = &BackupFailedData{
			TemplateData: baseData,
			BackupName:   "search-backup-daily",
			Error:        "disk full: no space left on device",
			FailedAt:     sampleTime,
		}
	case TemplateSSLExpiring:
		data = &SSLExpiringData{
			TemplateData: baseData,
			Domain:       siteURL,
			ExpiresAt:    sampleTime.Add(14 * 24 * time.Hour),
			DaysLeft:     14,
			RenewLink:    siteURL + version.APIPrefix + "/server/ssl/renew",
		}
	case TemplateSSLRenewed:
		data = &SSLRenewedData{
			TemplateData: baseData,
			Domain:       siteURL,
			RenewedAt:    sampleTime,
			ValidUntil:   sampleTime.Add(90 * 24 * time.Hour),
		}
	case TemplateSchedulerError:
		data = &SchedulerErrorData{
			TemplateData: baseData,
			TaskName:     "geoip_update",
			Error:        "connection timeout after 30s",
			FailedAt:     sampleTime,
			TaskDetails:  map[string]string{"Retries": "3", "Next attempt": "in 1 hour"},
		}
	case TemplateBreachAdminAlert:
		data = &BreachAdminAlertData{
			TemplateData:      baseData,
			Severity:          "HIGH",
			DetectedAt:        sampleTime,
			BreachDescription: "Suspicious access pattern detected from multiple IPs.",
			AffectedUsers:     0,
			IPAddresses:       []string{"45.33.32.156", "198.51.100.42"},
			ActionRequired:    "Review logs and consider blocking source IP ranges.",
		}
	case TemplateTest:
		data = &TestEmailData{
			TemplateData: baseData,
			SentAt:       sampleTime,
		}
	default:
		return "", "", fmt.Errorf("unknown template type: %s", templateType)
	}

	return et.Render(templateType, data)
}
