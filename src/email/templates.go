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
	TemplateWelcome          TemplateType = "welcome"
	TemplatePasswordReset    TemplateType = "password_reset"
	TemplatePasswordChanged  TemplateType = "password_changed"
	TemplateLoginNotification TemplateType = "login_notification"
	TemplateEmailVerification TemplateType = "email_verification"
	TemplateAccountLocked    TemplateType = "account_locked"
	TemplateAdminAlert       TemplateType = "admin_alert"
	TemplateWeeklyReport     TemplateType = "weekly_report"
	TemplateSecurityAlert    TemplateType = "security_alert"
	TemplateAPITokenCreated  TemplateType = "api_token_created"
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
}
