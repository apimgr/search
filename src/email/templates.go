package email

import (
	"bytes"
	"fmt"
	"html/template"
	"time"
)

// TemplateType represents an email template type
type TemplateType string

const (
	TemplateWelcome           TemplateType = "welcome"
	TemplatePasswordReset     TemplateType = "password_reset"
	TemplatePasswordChanged   TemplateType = "password_changed"
	TemplateLoginNotification TemplateType = "login_notification"
	TemplateEmailVerification TemplateType = "email_verification"
	TemplateAccountLocked     TemplateType = "account_locked"
	TemplateAdminAlert        TemplateType = "admin_alert"
	TemplateWeeklyReport      TemplateType = "weekly_report"
	TemplateSecurityAlert     TemplateType = "security_alert"
	TemplateAPITokenCreated   TemplateType = "api_token_created"
	// Additional templates per TEMPLATE.md PART 16
	TemplateAdminInvite       TemplateType = "admin_invite"
	Template2FAEnabled        TemplateType = "two_factor_enabled"
	Template2FADisabled       TemplateType = "two_factor_disabled"
	TemplateBackupCompleted   TemplateType = "backup_completed"
	TemplateUpdateAvailable   TemplateType = "update_available"
	TemplateMaintenanceNotice TemplateType = "maintenance_notice"
)

// TemplateData holds common template data
type TemplateData struct {
	SiteName    string
	SiteURL     string
	Year        int
	SupportEmail string
}

// NewTemplateData creates template data with defaults
func NewTemplateData(siteName, siteURL, supportEmail string) *TemplateData {
	return &TemplateData{
		SiteName:    siteName,
		SiteURL:     siteURL,
		Year:        time.Now().Year(),
		SupportEmail: supportEmail,
	}
}

// WelcomeData holds data for welcome email
type WelcomeData struct {
	*TemplateData
	Username string
	Email    string
}

// PasswordResetData holds data for password reset email
type PasswordResetData struct {
	*TemplateData
	Username   string
	ResetLink  string
	ExpiresIn  string
	IPAddress  string
	RequestedAt time.Time
}

// PasswordChangedData holds data for password changed notification
type PasswordChangedData struct {
	*TemplateData
	Username   string
	ChangedAt  time.Time
	IPAddress  string
	UserAgent  string
}

// LoginNotificationData holds data for login notification
type LoginNotificationData struct {
	*TemplateData
	Username  string
	LoginTime time.Time
	IPAddress string
	UserAgent string
	Location  string
	IsNewDevice bool
}

// EmailVerificationData holds data for email verification
type EmailVerificationData struct {
	*TemplateData
	Username       string
	Email          string
	VerificationLink string
	ExpiresIn      string
}

// AccountLockedData holds data for account locked notification
type AccountLockedData struct {
	*TemplateData
	Username    string
	Reason      string
	LockedAt    time.Time
	UnlockInstructions string
}

// AdminAlertData holds data for admin alert
type AdminAlertData struct {
	*TemplateData
	AlertType   string
	AlertLevel  string
	Message     string
	Details     map[string]string
	OccurredAt  time.Time
}

// WeeklyReportData holds data for weekly report
type WeeklyReportData struct {
	*TemplateData
	PeriodStart    time.Time
	PeriodEnd      time.Time
	TotalSearches  int
	UniqueUsers    int
	TopQueries     []string
	EngineStats    map[string]int
	ErrorCount     int
}

// SecurityAlertData holds data for security alert
type SecurityAlertData struct {
	*TemplateData
	Event      string
	Severity   string
	IPAddress  string
	Details    string
	OccurredAt time.Time
	ActionRequired string
}

// APITokenCreatedData holds data for API token creation notification
type APITokenCreatedData struct {
	*TemplateData
	Username    string
	TokenName   string
	Permissions []string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	IPAddress   string
}

// AdminInviteData holds data for admin invite email
type AdminInviteData struct {
	*TemplateData
	InviterName string
	InviteLink  string
	ExpiresIn   string
	Message     string
}

// TwoFactorEnabledData holds data for 2FA enabled notification
type TwoFactorEnabledData struct {
	*TemplateData
	Username    string
	EnabledAt   time.Time
	IPAddress   string
	Method      string
}

// TwoFactorDisabledData holds data for 2FA disabled notification
type TwoFactorDisabledData struct {
	*TemplateData
	Username    string
	DisabledAt  time.Time
	IPAddress   string
	Reason      string
}

// BackupCompletedData holds data for backup completion notification
type BackupCompletedData struct {
	*TemplateData
	BackupName  string
	BackupSize  string
	CreatedAt   time.Time
	FileCount   int
	Duration    string
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
	ScheduledAt  time.Time
	Duration     string
	Reason       string
	AffectedServices []string
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
	for templateType, tmpl := range rawTemplates {
		t, err := template.New(string(templateType)).Parse(tmpl)
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
	TemplateWelcome: `Welcome to {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Welcome to {{.SiteName}}!</h1>
        <p>Hello {{.Username}},</p>
        <p>Thank you for joining {{.SiteName}}, a privacy-respecting search engine.</p>
        <p>Your account has been created with the email: <strong>{{.Email}}</strong></p>
        <p>You can now:</p>
        <ul>
            <li>Save your search preferences</li>
            <li>Access your search history (stored locally)</li>
            <li>Customize your experience</li>
        </ul>
        <p><a href="{{.SiteURL}}" style="display: inline-block; background: #00d9ff; color: #1a1a2e; padding: 12px 24px; text-decoration: none; border-radius: 4px; font-weight: bold;">Start Searching</a></p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}. Your privacy matters.</p>
    </div>
</body>
</html>`,

	TemplatePasswordReset: `Password Reset Request - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Password Reset</h1>
        <p>Hello {{.Username}},</p>
        <p>We received a request to reset your password. Click the button below to create a new password:</p>
        <p><a href="{{.ResetLink}}" style="display: inline-block; background: #00d9ff; color: #1a1a2e; padding: 12px 24px; text-decoration: none; border-radius: 4px; font-weight: bold;">Reset Password</a></p>
        <p style="color: #888; font-size: 14px;">This link will expire in {{.ExpiresIn}}.</p>
        <p style="color: #888; font-size: 14px;">Request details:</p>
        <ul style="color: #888; font-size: 14px;">
            <li>IP Address: {{.IPAddress}}</li>
            <li>Time: {{.RequestedAt.Format "Jan 2, 2006 3:04 PM"}}</li>
        </ul>
        <p style="color: #ff6b6b;">If you didn't request this, please ignore this email or contact support.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplatePasswordChanged: `Password Changed - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Password Changed</h1>
        <p>Hello {{.Username}},</p>
        <p>Your password was successfully changed.</p>
        <p style="color: #888; font-size: 14px;">Change details:</p>
        <ul style="color: #888; font-size: 14px;">
            <li>Time: {{.ChangedAt.Format "Jan 2, 2006 3:04 PM"}}</li>
            <li>IP Address: {{.IPAddress}}</li>
            <li>Browser: {{.UserAgent}}</li>
        </ul>
        <p style="color: #ff6b6b;">If you didn't make this change, please reset your password immediately and contact support.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateLoginNotification: `{{if .IsNewDevice}}New Device Login{{else}}Login Notification{{end}} - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: {{if .IsNewDevice}}#ff6b6b{{else}}#00d9ff{{end}}; margin-top: 0;">{{if .IsNewDevice}}New Device Login{{else}}Login Notification{{end}}</h1>
        <p>Hello {{.Username}},</p>
        {{if .IsNewDevice}}
        <p style="color: #ff6b6b;">A new device was used to sign into your account.</p>
        {{else}}
        <p>A successful login was recorded for your account.</p>
        {{end}}
        <p style="color: #888; font-size: 14px;">Login details:</p>
        <ul style="color: #888; font-size: 14px;">
            <li>Time: {{.LoginTime.Format "Jan 2, 2006 3:04 PM"}}</li>
            <li>IP Address: {{.IPAddress}}</li>
            {{if .Location}}<li>Location: {{.Location}}</li>{{end}}
            <li>Browser: {{.UserAgent}}</li>
        </ul>
        <p style="color: #ff6b6b;">If this wasn't you, please change your password immediately.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateEmailVerification: `Verify Your Email - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Verify Your Email</h1>
        <p>Hello {{.Username}},</p>
        <p>Please verify your email address ({{.Email}}) by clicking the button below:</p>
        <p><a href="{{.VerificationLink}}" style="display: inline-block; background: #00d9ff; color: #1a1a2e; padding: 12px 24px; text-decoration: none; border-radius: 4px; font-weight: bold;">Verify Email</a></p>
        <p style="color: #888; font-size: 14px;">This link will expire in {{.ExpiresIn}}.</p>
        <p style="color: #888; font-size: 14px;">If you didn't create an account, please ignore this email.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateAccountLocked: `Account Locked - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ff6b6b; margin-top: 0;">Account Locked</h1>
        <p>Hello {{.Username}},</p>
        <p>Your account has been locked for security reasons.</p>
        <p><strong>Reason:</strong> {{.Reason}}</p>
        <p><strong>Locked at:</strong> {{.LockedAt.Format "Jan 2, 2006 3:04 PM"}}</p>
        {{if .UnlockInstructions}}
        <p><strong>To unlock your account:</strong></p>
        <p>{{.UnlockInstructions}}</p>
        {{end}}
        <p>If you believe this is an error, please contact support at {{.SupportEmail}}.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

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
            <tr><td style="padding: 8px; border-bottom: 1px solid #333;">Unique Users</td><td style="padding: 8px; border-bottom: 1px solid #333; text-align: right;"><strong>{{.UniqueUsers}}</strong></td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333;">Errors</td><td style="padding: 8px; border-bottom: 1px solid #333; text-align: right;"><strong>{{.ErrorCount}}</strong></td></tr>
        </table>

        {{if .TopQueries}}
        <h2 style="color: #00d9ff; font-size: 18px;">Top Queries</h2>
        <ol>
        {{range .TopQueries}}
            <li>{{.}}</li>
        {{end}}
        </ol>
        {{end}}

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

	TemplateAPITokenCreated: `New API Token Created - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">New API Token Created</h1>
        <p>Hello {{.Username}},</p>
        <p>A new API token has been created for your account.</p>

        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Token Name</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.TokenName}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Created</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.CreatedAt.Format "Jan 2, 2006 3:04 PM"}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Expires</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.ExpiresAt.Format "Jan 2, 2006"}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">IP Address</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.IPAddress}}</td></tr>
        </table>

        {{if .Permissions}}
        <p><strong>Permissions:</strong></p>
        <ul>
        {{range .Permissions}}
            <li>{{.}}</li>
        {{end}}
        </ul>
        {{end}}

        <p style="color: #ff6b6b;">If you didn't create this token, please revoke it immediately in your account settings.</p>

        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	TemplateAdminInvite: `You've Been Invited to Admin - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00d9ff; margin-top: 0;">Admin Invitation</h1>
        <p>You've been invited by <strong>{{.InviterName}}</strong> to become an administrator of {{.SiteName}}.</p>
        {{if .Message}}
        <p style="background: #0f0f1a; padding: 15px; border-radius: 4px; border-left: 4px solid #00d9ff;">
            "{{.Message}}"
        </p>
        {{end}}
        <p>Click the button below to accept the invitation and create your admin account:</p>
        <p><a href="{{.InviteLink}}" style="display: inline-block; background: #00d9ff; color: #1a1a2e; padding: 12px 24px; text-decoration: none; border-radius: 4px; font-weight: bold;">Accept Invitation</a></p>
        <p style="color: #888; font-size: 14px;">This invitation expires in {{.ExpiresIn}}.</p>
        <p style="color: #888; font-size: 14px;">If you weren't expecting this invitation, you can safely ignore this email.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	Template2FAEnabled: `Two-Factor Authentication Enabled - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #00ff88; margin-top: 0;">2FA Enabled</h1>
        <p>Hello {{.Username}},</p>
        <p>Two-factor authentication has been successfully enabled for your account.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Method</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.Method}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Enabled At</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.EnabledAt.Format "Jan 2, 2006 3:04 PM"}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">IP Address</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.IPAddress}}</td></tr>
        </table>
        <p style="background: #00ff8822; border-left: 4px solid #00ff88; padding: 15px; margin: 20px 0;">
            Your account is now more secure. Make sure to save your recovery codes in a safe place.
        </p>
        <p style="color: #ff6b6b;">If you didn't enable 2FA, please contact support immediately.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
    </div>
</body>
</html>`,

	Template2FADisabled: `Two-Factor Authentication Disabled - {{.SiteName}}
<!DOCTYPE html>
<html>
<head><meta charset="UTF-8"></head>
<body style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; max-width: 600px; margin: 0 auto; padding: 20px;">
    <div style="background: #1a1a2e; color: #ffffff; padding: 30px; border-radius: 8px;">
        <h1 style="color: #ffd93d; margin-top: 0;">2FA Disabled</h1>
        <p>Hello {{.Username}},</p>
        <p>Two-factor authentication has been disabled for your account.</p>
        <table style="width: 100%; border-collapse: collapse; margin: 20px 0;">
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Disabled At</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.DisabledAt.Format "Jan 2, 2006 3:04 PM"}}</td></tr>
            <tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">IP Address</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.IPAddress}}</td></tr>
            {{if .Reason}}<tr><td style="padding: 8px; border-bottom: 1px solid #333; color: #888;">Reason</td><td style="padding: 8px; border-bottom: 1px solid #333;">{{.Reason}}</td></tr>{{end}}
        </table>
        <p style="background: #ffd93d22; border-left: 4px solid #ffd93d; padding: 15px; margin: 20px 0;">
            Your account is now less secure. We recommend re-enabling 2FA as soon as possible.
        </p>
        <p style="color: #ff6b6b;">If you didn't disable 2FA, please change your password and contact support immediately.</p>
        <hr style="border: 1px solid #333; margin: 20px 0;">
        <p style="color: #888; font-size: 12px;">&copy; {{.Year}} {{.SiteName}}</p>
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
}

// TemplateInfo contains metadata about an email template
// Per TEMPLATE.md PART 31: Account emails vs Notification emails
type TemplateInfo struct {
	Type           TemplateType `json:"type"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	IsAccountEmail bool         `json:"is_account_email"`
}

// GetAllTemplateTypes returns a list of all available email template types
// Per TEMPLATE.md PART 16: Admin should be able to preview email templates
// Per TEMPLATE.md PART 31: Templates are marked as account (security) or notification
func GetAllTemplateTypes() []TemplateInfo {
	return []TemplateInfo{
		// Account emails (security-sensitive) - always sent to account email
		{TemplateWelcome, "Welcome", "Sent when a new user registers", true},
		{TemplatePasswordReset, "Password Reset", "Password reset request link", true},
		{TemplatePasswordChanged, "Password Changed", "Notification after password change", true},
		{TemplateLoginNotification, "Login Notification", "Alert for new login activity", true},
		{TemplateEmailVerification, "Email Verification", "Email address verification link", true},
		{TemplateAccountLocked, "Account Locked", "Notification when account is locked", true},
		{TemplateSecurityAlert, "Security Alert", "Security-related notifications", true},
		{Template2FAEnabled, "2FA Enabled", "Confirmation of 2FA activation", true},
		{Template2FADisabled, "2FA Disabled", "Notification of 2FA deactivation", true},

		// Notification emails (non-security) - sent to notification email if set
		{TemplateAdminAlert, "Admin Alert", "System alerts for administrators", false},
		{TemplateWeeklyReport, "Weekly Report", "Weekly usage statistics summary", false},
		{TemplateAPITokenCreated, "API Token Created", "Notification for new API token", false},
		{TemplateAdminInvite, "Admin Invite", "Invitation to become an admin", false},
		{TemplateBackupCompleted, "Backup Completed", "Backup completion notification", false},
		{TemplateUpdateAvailable, "Update Available", "New version available notification", false},
		{TemplateMaintenanceNotice, "Maintenance Notice", "Scheduled maintenance alert", false},
	}
}

// IsAccountEmail returns true if the template is for account/security emails
// Per TEMPLATE.md PART 31: Account emails go to user's account email only
func IsAccountEmail(templateType TemplateType) bool {
	switch templateType {
	case TemplateWelcome,
		TemplatePasswordReset,
		TemplatePasswordChanged,
		TemplateLoginNotification,
		TemplateEmailVerification,
		TemplateAccountLocked,
		TemplateSecurityAlert,
		Template2FAEnabled,
		Template2FADisabled:
		return true
	default:
		return false
	}
}

// PreviewTemplate renders a template with sample data for preview
// Per TEMPLATE.md PART 16: Template preview in admin panel
func (et *EmailTemplate) PreviewTemplate(templateType TemplateType, siteName, siteURL string) (subject string, body string, err error) {
	baseData := NewTemplateData(siteName, siteURL, "support@"+siteURL)
	sampleTime := time.Now()

	var data interface{}

	switch templateType {
	case TemplateWelcome:
		data = &WelcomeData{
			TemplateData: baseData,
			Username:     "john_doe",
			Email:        "john@example.com",
		}
	case TemplatePasswordReset:
		data = &PasswordResetData{
			TemplateData: baseData,
			Username:     "john_doe",
			ResetLink:    siteURL + "/auth/reset/sample-token-12345",
			ExpiresIn:    "1 hour",
			IPAddress:    "192.168.1.100",
			RequestedAt:  sampleTime,
		}
	case TemplatePasswordChanged:
		data = &PasswordChangedData{
			TemplateData: baseData,
			Username:     "john_doe",
			ChangedAt:    sampleTime,
			IPAddress:    "192.168.1.100",
			UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0",
		}
	case TemplateLoginNotification:
		data = &LoginNotificationData{
			TemplateData: baseData,
			Username:     "john_doe",
			LoginTime:    sampleTime,
			IPAddress:    "192.168.1.100",
			UserAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0",
			Location:     "New York, US",
			IsNewDevice:  true,
		}
	case TemplateEmailVerification:
		data = &EmailVerificationData{
			TemplateData:     baseData,
			Username:         "john_doe",
			Email:            "john@example.com",
			VerificationLink: siteURL + "/auth/verify/sample-token-12345",
			ExpiresIn:        "24 hours",
		}
	case TemplateAccountLocked:
		data = &AccountLockedData{
			TemplateData:       baseData,
			Username:           "john_doe",
			Reason:             "Too many failed login attempts",
			LockedAt:           sampleTime,
			UnlockInstructions: "Contact support or wait 30 minutes for automatic unlock.",
		}
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
			UniqueUsers:   2341,
			TopQueries:    []string{"golang tutorial", "python web framework", "docker compose", "kubernetes guide", "rust async"},
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
			Details:        "5 failed login attempts for user 'admin' in the last 10 minutes.",
			OccurredAt:     sampleTime,
			ActionRequired: "Review the activity and consider blocking the IP address.",
		}
	case TemplateAPITokenCreated:
		data = &APITokenCreatedData{
			TemplateData: baseData,
			Username:     "john_doe",
			TokenName:    "CI/CD Pipeline",
			Permissions:  []string{"read:search", "read:stats", "write:preferences"},
			ExpiresAt:    sampleTime.AddDate(0, 3, 0),
			CreatedAt:    sampleTime,
			IPAddress:    "192.168.1.100",
		}
	case TemplateAdminInvite:
		data = &AdminInviteData{
			TemplateData: baseData,
			InviterName:  "Primary Admin",
			InviteLink:   siteURL + "/admin/invite/sample-invite-token",
			ExpiresIn:    "48 hours",
			Message:      "Welcome to the admin team! Please set up your account.",
		}
	case Template2FAEnabled:
		data = &TwoFactorEnabledData{
			TemplateData: baseData,
			Username:     "john_doe",
			EnabledAt:    sampleTime,
			IPAddress:    "192.168.1.100",
			Method:       "TOTP (Authenticator App)",
		}
	case Template2FADisabled:
		data = &TwoFactorDisabledData{
			TemplateData: baseData,
			Username:     "john_doe",
			DisabledAt:   sampleTime,
			IPAddress:    "192.168.1.100",
			Reason:       "User requested",
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
			AffectedServices: []string{"Search API", "Admin Panel", "GraphQL Endpoint"},
		}
	default:
		return "", "", fmt.Errorf("unknown template type: %s", templateType)
	}

	return et.Render(templateType, data)
}
