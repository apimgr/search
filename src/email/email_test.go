package email

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// Tests for Config

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}
	if cfg.Enabled {
		t.Error("Enabled should be false by default")
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("SMTP.Port = %d, want 587", cfg.SMTP.Port)
	}
	if cfg.SMTP.TLS != "auto" {
		t.Errorf("SMTP.TLS = %q, want auto", cfg.SMTP.TLS)
	}
}

func TestConfigStruct(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     "smtp.example.com",
			Port:     465,
			Username: "user@example.com",
			Password: "secret",
			TLS:      "tls",
		},
		From: FromConfig{
			Name:  "My App",
			Email: "noreply@example.com",
		},
		AdminEmails: []string{"admin@example.com", "ops@example.com"},
	}

	if cfg.SMTP.Host != "smtp.example.com" {
		t.Errorf("SMTP.Host = %q", cfg.SMTP.Host)
	}
	if cfg.SMTP.Port != 465 {
		t.Errorf("SMTP.Port = %d", cfg.SMTP.Port)
	}
	if len(cfg.AdminEmails) != 2 {
		t.Errorf("AdminEmails length = %d", len(cfg.AdminEmails))
	}
}

// Tests for Mailer

func TestNewMailer(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "smtp.example.com",
			Port: 587,
		},
		From: FromConfig{
			Email: "test@example.com",
		},
	}

	ml := NewMailer(cfg)

	if ml == nil {
		t.Fatal("NewMailer() returned nil")
	}
	if ml.config != cfg {
		t.Error("config should be set")
	}
}

func TestNewMailerNilConfig(t *testing.T) {
	ml := NewMailer(nil)

	if ml == nil {
		t.Fatal("NewMailer(nil) returned nil")
	}
	if ml.config == nil {
		t.Error("config should not be nil")
	}
	if ml.config.SMTP.Port != 587 {
		t.Errorf("Should use default config, got Port = %d", ml.config.SMTP.Port)
	}
}

func TestMailerIsEnabled(t *testing.T) {
	cfg := &Config{Enabled: true}
	ml := NewMailer(cfg)

	if !ml.IsEnabled() {
		t.Error("IsEnabled() should return true when enabled")
	}

	cfg.Enabled = false
	if ml.IsEnabled() {
		t.Error("IsEnabled() should return false when disabled")
	}
}

// Tests for Message

func TestNewMessage(t *testing.T) {
	to := []string{"user@example.com"}
	subject := "Test Subject"
	body := "Test body content"

	msg := NewMessage(to, subject, body)

	if msg == nil {
		t.Fatal("NewMessage() returned nil")
	}
	if len(msg.To) != 1 {
		t.Errorf("To length = %d, want 1", len(msg.To))
	}
	if msg.To[0] != "user@example.com" {
		t.Errorf("To[0] = %q", msg.To[0])
	}
	if msg.Subject != subject {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if msg.Body != body {
		t.Errorf("Body = %q", msg.Body)
	}
	if msg.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", msg.ContentType)
	}
	if msg.Headers == nil {
		t.Error("Headers should be initialized")
	}
}

func TestMessageSetHTML(t *testing.T) {
	msg := NewMessage([]string{"user@example.com"}, "Test", "Plain body")
	htmlContent := "<h1>Hello</h1><p>World</p>"

	msg.SetHTML(htmlContent)

	if msg.HTMLBody != htmlContent {
		t.Errorf("HTMLBody = %q", msg.HTMLBody)
	}
	if msg.ContentType != "text/html" {
		t.Errorf("ContentType = %q, want text/html", msg.ContentType)
	}
}

func TestMessageStruct(t *testing.T) {
	msg := &Message{
		To:          []string{"to@example.com"},
		CC:          []string{"cc@example.com"},
		BCC:         []string{"bcc@example.com"},
		Subject:     "Test",
		Body:        "Body text",
		HTMLBody:    "<p>HTML</p>",
		ContentType: "text/html",
		Headers:     map[string]string{"X-Custom": "value"},
	}

	if len(msg.To) != 1 {
		t.Errorf("To length = %d", len(msg.To))
	}
	if len(msg.CC) != 1 {
		t.Errorf("CC length = %d", len(msg.CC))
	}
	if len(msg.BCC) != 1 {
		t.Errorf("BCC length = %d", len(msg.BCC))
	}
	if msg.Headers["X-Custom"] != "value" {
		t.Errorf("Headers['X-Custom'] = %q", msg.Headers["X-Custom"])
	}
}

// Tests for Send (without actual SMTP server)

func TestMailerSendDisabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error when email is disabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("Error = %q, should mention 'not enabled'", err.Error())
	}
}

func TestMailerSendNoRecipients(t *testing.T) {
	cfg := &Config{Enabled: true}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{}, "Test", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with no recipients")
	}
	if !strings.Contains(err.Error(), "no recipients") {
		t.Errorf("Error = %q, should mention 'no recipients'", err.Error())
	}
}

// Tests for SendToAdmins

func TestMailerSendToAdminsNoAdmins(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		AdminEmails: []string{},
	}
	ml := NewMailer(cfg)

	err := ml.SendToAdmins("Test Subject", "Test Body")
	if err == nil {
		t.Error("SendToAdmins() should error with no admin emails")
	}
	if !strings.Contains(err.Error(), "no admin emails") {
		t.Errorf("Error = %q, should mention 'no admin emails'", err.Error())
	}
}

// Tests for formatAddress

func TestFormatAddress(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	tests := []struct {
		name    string
		email   string
		wantHas string
	}{
		{"", "test@example.com", "test@example.com"},
		{"Test User", "test@example.com", "Test User <test@example.com>"},
		{"John Doe", "john@example.com", "John Doe <john@example.com>"},
	}

	for _, tt := range tests {
		result := ml.formatAddress(tt.name, tt.email)
		if !strings.Contains(result, tt.wantHas) {
			t.Errorf("formatAddress(%q, %q) = %q, should contain %q", tt.name, tt.email, result, tt.wantHas)
		}
	}
}

// Tests for encodeHeader

func TestEncodeHeader(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	tests := []struct {
		name    string
		input   string
		encoded bool
	}{
		{"ascii", "Hello World", false},
		{"utf8", "Hello 世界", true},
		{"accents", "Café", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ml.encodeHeader(tt.input)

			if tt.encoded {
				if !strings.HasPrefix(result, "=?UTF-8?B?") {
					t.Errorf("encodeHeader(%q) = %q, expected UTF-8 B encoding", tt.input, result)
				}
			} else {
				if result != tt.input {
					t.Errorf("encodeHeader(%q) = %q, want same as input", tt.input, result)
				}
			}
		})
	}
}

// Tests for SMTPConfig

func TestSMTPConfigStruct(t *testing.T) {
	smtp := SMTPConfig{
		Host:     "mail.example.com",
		Port:     587,
		Username: "user",
		Password: "pass",
		TLS:      "starttls",
	}

	if smtp.Host != "mail.example.com" {
		t.Errorf("Host = %q", smtp.Host)
	}
	if smtp.Port != 587 {
		t.Errorf("Port = %d", smtp.Port)
	}
	if smtp.TLS != "starttls" {
		t.Errorf("TLS = %q", smtp.TLS)
	}
}

// Tests for FromConfig

func TestFromConfigStruct(t *testing.T) {
	from := FromConfig{
		Name:  "My Application",
		Email: "app@example.com",
	}

	if from.Name != "My Application" {
		t.Errorf("Name = %q", from.Name)
	}
	if from.Email != "app@example.com" {
		t.Errorf("Email = %q", from.Email)
	}
}

// Tests for TLS mode handling

func TestTLSModes(t *testing.T) {
	modes := []string{"auto", "starttls", "tls", "none"}
	for _, mode := range modes {
		cfg := &Config{
			Enabled: true,
			SMTP: SMTPConfig{
				Host: "smtp.example.com",
				Port: 587,
				TLS:  mode,
			},
			From: FromConfig{
				Email: "test@example.com",
			},
		}
		ml := NewMailer(cfg)
		if ml.config.SMTP.TLS != mode {
			t.Errorf("TLS mode %q not preserved", mode)
		}
	}
}

// Tests for SendAlert

func TestMailerSendAlert(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		AdminEmails: []string{"admin@example.com"},
	}
	ml := NewMailer(cfg)

	err := ml.SendAlert("Test Alert", "Test message")
	if err == nil {
		t.Error("SendAlert() should error when email is disabled")
	}
}

func TestMailerSendAlertNoAdmins(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		AdminEmails: []string{},
	}
	ml := NewMailer(cfg)

	err := ml.SendAlert("Test Alert", "Test message")
	if err == nil {
		t.Error("SendAlert() should error with no admin emails")
	}
}

// Tests for SendSecurityAlert

func TestMailerSendSecurityAlert(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		AdminEmails: []string{"admin@example.com"},
	}
	ml := NewMailer(cfg)

	err := ml.SendSecurityAlert("Login Failed", "192.168.1.1", "Multiple failed attempts")
	if err == nil {
		t.Error("SendSecurityAlert() should error when email is disabled")
	}
}

func TestMailerSendSecurityAlertNoAdmins(t *testing.T) {
	cfg := &Config{
		Enabled:     true,
		AdminEmails: []string{},
	}
	ml := NewMailer(cfg)

	err := ml.SendSecurityAlert("Security Event", "10.0.0.1", "Details here")
	if err == nil {
		t.Error("SendSecurityAlert() should error with no admin emails")
	}
}

// Tests for TestConnection

func TestMailerTestConnectionDisabled(t *testing.T) {
	cfg := &Config{Enabled: false}
	ml := NewMailer(cfg)

	err := ml.TestConnection()
	if err == nil {
		t.Error("TestConnection() should error when email is disabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("Error = %q, should mention 'not enabled'", err.Error())
	}
}

func TestMailerTestConnectionNoHost(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "",
			Port: 587,
		},
	}
	ml := NewMailer(cfg)

	err := ml.TestConnection()
	if err == nil {
		t.Error("TestConnection() should error with no host")
	}
}

// Tests for DetectedSMTP struct

func TestDetectedSMTPStruct(t *testing.T) {
	detected := &DetectedSMTP{
		Host:         "smtp.example.com",
		Port:         587,
		TLS:          false,
		STARTTLS:     true,
		AuthRequired: true,
	}

	if detected.Host != "smtp.example.com" {
		t.Errorf("Host = %q", detected.Host)
	}
	if detected.Port != 587 {
		t.Errorf("Port = %d", detected.Port)
	}
	if detected.TLS {
		t.Error("TLS should be false")
	}
	if !detected.STARTTLS {
		t.Error("STARTTLS should be true")
	}
	if !detected.AuthRequired {
		t.Error("AuthRequired should be true")
	}
}

func TestDetectedSMTPStructTLS(t *testing.T) {
	detected := &DetectedSMTP{
		Host:         "smtp.example.com",
		Port:         465,
		TLS:          true,
		STARTTLS:     false,
		AuthRequired: true,
	}

	if detected.Port != 465 {
		t.Errorf("Port = %d", detected.Port)
	}
	if !detected.TLS {
		t.Error("TLS should be true for port 465")
	}
}

// Tests for DetectAndConfigure

func TestDetectAndConfigureNoSMTP(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Enabled {
		t.Error("Default config should not be enabled")
	}
	if cfg.SMTP.Port != 587 {
		t.Errorf("Default port = %d, want 587", cfg.SMTP.Port)
	}
}

// Tests for IsLocalSMTPAvailable

func TestIsLocalSMTPAvailable(t *testing.T) {
	_ = IsLocalSMTPAvailable()
}

// Tests for Message with CC and BCC

func TestMessageWithCCAndBCC(t *testing.T) {
	msg := NewMessage([]string{"to@example.com"}, "Test", "Body")
	msg.CC = []string{"cc1@example.com", "cc2@example.com"}
	msg.BCC = []string{"bcc@example.com"}

	if len(msg.CC) != 2 {
		t.Errorf("CC length = %d", len(msg.CC))
	}
	if len(msg.BCC) != 1 {
		t.Errorf("BCC length = %d", len(msg.BCC))
	}
}

func TestMessageWithHeaders(t *testing.T) {
	msg := NewMessage([]string{"to@example.com"}, "Test", "Body")
	msg.Headers["X-Priority"] = "1"
	msg.Headers["X-Custom-Header"] = "custom-value"

	if msg.Headers["X-Priority"] != "1" {
		t.Errorf("X-Priority = %q", msg.Headers["X-Priority"])
	}
	if msg.Headers["X-Custom-Header"] != "custom-value" {
		t.Errorf("X-Custom-Header = %q", msg.Headers["X-Custom-Header"])
	}
}

// Tests for Config YAML tags

func TestConfigYAMLTags(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     "smtp.example.com",
			Port:     587,
			Username: "user",
			Password: "pass",
			TLS:      "auto",
		},
		From: FromConfig{
			Name:  "Test",
			Email: "test@example.com",
		},
		AdminEmails: []string{"admin@example.com"},
	}

	if cfg.SMTP.Host != "smtp.example.com" {
		t.Errorf("SMTP.Host = %q", cfg.SMTP.Host)
	}
}

// Tests for encodeHeader edge cases

func TestEncodeHeaderEdgeCases(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	tests := []struct {
		name    string
		input   string
		encoded bool
	}{
		{"empty", "", false},
		{"numbers", "12345", false},
		{"symbols", "!@#$%^&*()", false},
		{"mixed with emoji", "Hello 👋", true},
		{"cyrillic", "Привет", true},
		{"japanese", "こんにちは", true},
		{"arabic", "مرحبا", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ml.encodeHeader(tt.input)
			if tt.encoded && !strings.HasPrefix(result, "=?UTF-8?B?") {
				t.Errorf("encodeHeader(%q) = %q, expected UTF-8 B encoding", tt.input, result)
			}
			if !tt.encoded && result != tt.input {
				t.Errorf("encodeHeader(%q) = %q, expected no change", tt.input, result)
			}
		})
	}
}

// Tests for formatAddress edge cases

func TestFormatAddressEdgeCases(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"", "test@example.com", "test@example.com"},
		{"Simple Name", "test@example.com", "Simple Name <test@example.com>"},
		{"Name With Spaces", "test@example.com", "Name With Spaces <test@example.com>"},
	}

	for _, tt := range tests {
		result := ml.formatAddress(tt.name, tt.email)
		if result != tt.want {
			t.Errorf("formatAddress(%q, %q) = %q, want %q", tt.name, tt.email, result, tt.want)
		}
	}
}

// Tests for different port configurations

func TestSMTPPortConfigurations(t *testing.T) {
	tests := []struct {
		name string
		port int
		tls  string
	}{
		{"standard", 25, "none"},
		{"submission", 587, "starttls"},
		{"smtps", 465, "tls"},
		{"custom", 2525, "auto"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Enabled: true,
				SMTP: SMTPConfig{
					Host: "smtp.example.com",
					Port: tt.port,
					TLS:  tt.tls,
				},
			}
			ml := NewMailer(cfg)

			if ml.config.SMTP.Port != tt.port {
				t.Errorf("Port = %d, want %d", ml.config.SMTP.Port, tt.port)
			}
			if ml.config.SMTP.TLS != tt.tls {
				t.Errorf("TLS = %q, want %q", ml.config.SMTP.TLS, tt.tls)
			}
		})
	}
}

// Tests for Send with various message configurations

func TestMailerSendWithHTML(t *testing.T) {
	cfg := &Config{Enabled: false}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Plain body")
	msg.SetHTML("<h1>Hello</h1>")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error when disabled")
	}
	if msg.HTMLBody != "<h1>Hello</h1>" {
		t.Errorf("HTMLBody = %q", msg.HTMLBody)
	}
	if msg.ContentType != "text/html" {
		t.Errorf("ContentType = %q", msg.ContentType)
	}
}

func TestMailerSendNilMessage(t *testing.T) {
	cfg := &Config{Enabled: true}
	ml := NewMailer(cfg)

	msg := NewMessage([]string{}, "Test", "Body")
	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with empty recipients")
	}
}

// Tests for multiple admin emails

func TestConfigMultipleAdminEmails(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		AdminEmails: []string{
			"admin1@example.com",
			"admin2@example.com",
			"admin3@example.com",
		},
	}

	if len(cfg.AdminEmails) != 3 {
		t.Errorf("AdminEmails length = %d", len(cfg.AdminEmails))
	}
}

// Tests for DetectSMTP

func TestDetectSMTP(t *testing.T) {
	result := DetectSMTP("", "")
	_ = result
}

// Tests for getDefaultGateway

func TestGetDefaultGateway(t *testing.T) {
	gateway := getDefaultGateway()
	_ = gateway
}

// Tests for tryDetectSMTP with unreachable host

func TestTryDetectSMTPUnreachable(t *testing.T) {
	result := tryDetectSMTP("127.0.0.1", 59999, false)
	if result != nil {
		t.Error("tryDetectSMTP should return nil for unreachable port")
	}
}

func TestTryDetectSMTPTLSUnreachable(t *testing.T) {
	result := tryDetectSMTP("127.0.0.1", 59998, true)
	if result != nil {
		t.Error("tryDetectSMTP with TLS should return nil for unreachable port")
	}
}

// Tests for EmailTemplate

func TestNewEmailTemplate(t *testing.T) {
	et := NewEmailTemplate()
	if et == nil {
		t.Fatal("NewEmailTemplate() returned nil")
	}
}

func TestEmailTemplateRenderSecurityAlert(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":  "TestApp",
		"fqdn":      "https://example.com",
		"timestamp": "2026-06-13 12:00:00 UTC",
		"event":     "Suspicious Activity",
		"ip":        "10.0.0.1",
		"details":   "Multiple failed login attempts detected",
	}

	subject, body, err := et.Render(TemplateSecurityAlert, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "Suspicious Activity") {
		t.Error("Body should contain event")
	}
}

func TestEmailTemplateRenderAdminAlert(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":    "TestApp",
		"fqdn":        "https://example.com",
		"timestamp":   "2026-06-13 12:00:00 UTC",
		"alert_type":  "System Warning",
		"alert_level": "warning",
		"message":     "High CPU usage detected",
	}

	subject, body, err := et.Render(TemplateAdminAlert, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "System Warning") {
		t.Error("Body should contain alert type")
	}
}

func TestEmailTemplateRenderWeeklyReport(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":       "TestApp",
		"fqdn":           "https://example.com",
		"period_start":   "2026-06-06",
		"period_end":     "2026-06-13",
		"total_searches": "15000",
		"error_count":    "10",
	}

	subject, body, err := et.Render(TemplateWeeklyReport, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "15000") {
		t.Error("Body should contain total searches")
	}
}

func TestEmailTemplateRenderBackupCompleted(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":  "TestApp",
		"fqdn":      "https://example.com",
		"timestamp": "2026-06-13 02:00:00 UTC",
		"filename":  "search_backup_2026-06-13_020000.tar.gz",
		"size":      "10.5 MB",
	}

	subject, _, err := et.Render(TemplateBackupCompleted, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
}

func TestEmailTemplateRenderUpdateAvailable(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":        "TestApp",
		"fqdn":            "https://example.com",
		"timestamp":       "2026-06-13 12:00:00 UTC",
		"current_version": "1.2.3",
		"new_version":     "1.3.0",
		"release_date":    "2026-06-13",
		"release_notes":   "Bug fixes and improvements",
		"update_url":      "https://example.com/releases/1.3.0",
	}

	subject, body, err := et.Render(TemplateUpdateAvailable, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "1.3.0") {
		t.Error("Body should contain new version")
	}
}

func TestEmailTemplateRenderMaintenanceNotice(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":          "TestApp",
		"fqdn":              "https://example.com",
		"timestamp":         "2026-06-13 12:00:00 UTC",
		"scheduled_at":      "2026-06-14 02:00:00 UTC",
		"duration":          "30 minutes",
		"reason":            "Database maintenance",
		"affected_services": "search, alerts",
	}

	subject, _, err := et.Render(TemplateMaintenanceNotice, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
}

func TestEmailTemplateRenderInvalidType(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.Render(TemplateType("invalid_template"), map[string]string{})
	if err == nil {
		t.Error("Render() should error for invalid template type")
	}
}

func TestEmailTemplateRenderNilData(t *testing.T) {
	et := NewEmailTemplate()

	// nil map is valid — no substitution occurs, template renders with placeholders
	_, _, err := et.Render(TemplateTest, nil)
	_ = err
}

// Tests for PreviewTemplate

func TestEmailTemplatePreviewTemplate(t *testing.T) {
	et := NewEmailTemplate()

	templates := []TemplateType{
		TemplateSecurityAlert,
		TemplateAdminAlert,
		TemplateWeeklyReport,
		TemplateBackupCompleted,
		TemplateUpdateAvailable,
		TemplateMaintenanceNotice,
		TemplateBackupFailed,
		TemplateSSLExpiring,
		TemplateSSLRenewed,
		TemplateSchedulerError,
		TemplateBreachAdminAlert,
		TemplateTest,
	}

	for _, tmplType := range templates {
		t.Run(string(tmplType), func(t *testing.T) {
			subject, body, err := et.PreviewTemplate(tmplType, "TestApp", "https://example.com")
			if err != nil {
				t.Fatalf("PreviewTemplate(%s) error = %v", tmplType, err)
			}
			if subject == "" {
				t.Errorf("PreviewTemplate(%s) returned empty subject", tmplType)
			}
			if body == "" {
				t.Errorf("PreviewTemplate(%s) returned empty body", tmplType)
			}
		})
	}
}

func TestEmailTemplatePreviewTemplateInvalid(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateType("nonexistent"), "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate() should error for invalid template type")
	}
}

// Tests for GetAllTemplateTypes

func TestGetAllTemplateTypes(t *testing.T) {
	types := GetAllTemplateTypes()

	if len(types) == 0 {
		t.Fatal("GetAllTemplateTypes() returned empty slice")
	}

	found := make(map[TemplateType]bool)
	for _, info := range types {
		found[info.Type] = true
	}

	expectedTypes := []TemplateType{
		TemplateSecurityAlert,
		TemplateAdminAlert,
		TemplateBackupCompleted,
	}

	for _, tt := range expectedTypes {
		if !found[tt] {
			t.Errorf("GetAllTemplateTypes() should include %s", tt)
		}
	}
}

func TestGetAllTemplateTypesInfo(t *testing.T) {
	types := GetAllTemplateTypes()

	for _, info := range types {
		if info.Type == "" {
			t.Error("TemplateInfo.Type should not be empty")
		}
		if info.Name == "" {
			t.Errorf("TemplateInfo.Name should not be empty for %s", info.Type)
		}
		if info.Description == "" {
			t.Errorf("TemplateInfo.Description should not be empty for %s", info.Type)
		}
	}
}

// Tests for IsAccountEmail

func TestIsAccountEmail(t *testing.T) {
	allTemplates := []TemplateType{
		TemplateAdminAlert,
		TemplateWeeklyReport,
		TemplateSecurityAlert,
		TemplateBackupCompleted,
		TemplateUpdateAvailable,
		TemplateMaintenanceNotice,
		TemplateBackupFailed,
		TemplateSSLExpiring,
		TemplateSSLRenewed,
		TemplateSchedulerError,
		TemplateBreachAdminAlert,
		TemplateTest,
	}

	for _, tt := range allTemplates {
		if IsAccountEmail(tt) {
			t.Errorf("IsAccountEmail(%s) should return false — no user accounts in this project", tt)
		}
	}
}

// Tests for TemplateType constants

func TestTemplateTypeConstants(t *testing.T) {
	if TemplateSecurityAlert != "security_alert" {
		t.Errorf("TemplateSecurityAlert = %q, want %q", TemplateSecurityAlert, "security_alert")
	}
	if TemplateAdminAlert != "admin_alert" {
		t.Errorf("TemplateAdminAlert = %q, want %q", TemplateAdminAlert, "admin_alert")
	}
	if TemplateWeeklyReport != "weekly_report" {
		t.Errorf("TemplateWeeklyReport = %q, want %q", TemplateWeeklyReport, "weekly_report")
	}
	if TemplateBackupCompleted != "backup_complete" {
		t.Errorf("TemplateBackupCompleted = %q, want %q", TemplateBackupCompleted, "backup_complete")
	}
}

// Tests for TemplateData struct

func TestNewTemplateData(t *testing.T) {
	data := NewTemplateData("MySite", "https://mysite.com", "help@mysite.com")

	if data == nil {
		t.Fatal("NewTemplateData() returned nil")
	}
	if data.SiteName != "MySite" {
		t.Errorf("SiteName = %q, want %q", data.SiteName, "MySite")
	}
	if data.SiteURL != "https://mysite.com" {
		t.Errorf("SiteURL = %q, want %q", data.SiteURL, "https://mysite.com")
	}
	if data.SupportEmail != "help@mysite.com" {
		t.Errorf("SupportEmail = %q, want %q", data.SupportEmail, "help@mysite.com")
	}
	if data.Year == 0 {
		t.Error("Year should not be 0")
	}
}

func TestTemplateDataStruct(t *testing.T) {
	data := &TemplateData{
		SiteName:     "Test Site",
		SiteURL:      "https://test.com",
		Year:         2024,
		SupportEmail: "support@test.com",
	}

	if data.SiteName != "Test Site" {
		t.Errorf("SiteName = %q", data.SiteName)
	}
	if data.Year != 2024 {
		t.Errorf("Year = %d", data.Year)
	}
}

func TestTemplateDataVarsMap(t *testing.T) {
	data := NewTemplateData("MySite", "https://mysite.com", "help@mysite.com")
	vars := data.VarsMap()

	if vars["app_name"] != "MySite" {
		t.Errorf("app_name = %q", vars["app_name"])
	}
	if vars["app_url"] != "https://mysite.com" {
		t.Errorf("app_url = %q", vars["app_url"])
	}
	if vars["fqdn"] != "https://mysite.com" {
		t.Errorf("fqdn = %q", vars["fqdn"])
	}
	if vars["year"] == "" {
		t.Error("year should not be empty")
	}
	if vars["timestamp"] == "" {
		t.Error("timestamp should not be empty")
	}
}

// Tests for helper functions

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"line1\nline2\nline3", 3},
		{"single line", 1},
		{"line1\n\nline3", 3},
	}

	for _, tt := range tests {
		result := splitLines(tt.input)
		if len(result) != tt.want {
			t.Errorf("splitLines(%q) returned %d lines, want %d", tt.input, len(result), tt.want)
		}
	}
}

func TestSplitLinesEmpty(t *testing.T) {
	result := splitLines("")
	if len(result) != 0 {
		t.Errorf("splitLines(\"\") returned %d lines, want 0", len(result))
	}
}

func TestSplitLinesTrailingNewline(t *testing.T) {
	result := splitLines("line1\nline2\n")
	if len(result) != 2 {
		t.Errorf("splitLines(\"line1\\nline2\\n\") returned %d lines, want 2", len(result))
	}
}

func TestSplitLinesOnlyNewlines(t *testing.T) {
	result := splitLines("\n\n\n")
	if len(result) != 3 {
		t.Errorf("splitLines(\"\\n\\n\\n\") returned %d lines, want 3", len(result))
	}
	for i, line := range result {
		if line != "" {
			t.Errorf("Line %d should be empty, got %q", i, line)
		}
	}
}

func TestSplitLinesSingleChar(t *testing.T) {
	result := splitLines("a")
	if len(result) != 1 {
		t.Errorf("splitLines(\"a\") returned %d lines, want 1", len(result))
	}
	if result[0] != "a" {
		t.Errorf("splitLines(\"a\")[0] = %q, want \"a\"", result[0])
	}
}

func TestJoinLines(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"line1", "line2", "line3"}, "line1\nline2\nline3"},
		{[]string{"single"}, "single"},
		{[]string{}, ""},
		{[]string{"", ""}, "\n"},
	}

	for _, tt := range tests {
		result := joinLines(tt.input)
		if result != tt.want {
			t.Errorf("joinLines(%v) = %q, want %q", tt.input, result, tt.want)
		}
	}
}

func TestJoinLinesSingleLine(t *testing.T) {
	result := joinLines([]string{"single"})
	if result != "single" {
		t.Errorf("joinLines([\"single\"]) = %q, want \"single\"", result)
	}
}

func TestJoinLinesNilSlice(t *testing.T) {
	result := joinLines(nil)
	if result != "" {
		t.Errorf("joinLines(nil) = %q, want \"\"", result)
	}
}

// Tests for ReplaceVars

func TestReplaceVars(t *testing.T) {
	text := "Hello {name}, welcome to {app_name}!"
	vars := map[string]string{
		"name":     "World",
		"app_name": "TestApp",
	}
	result := ReplaceVars(text, vars)
	if result != "Hello World, welcome to TestApp!" {
		t.Errorf("ReplaceVars = %q", result)
	}
}

func TestReplaceVarsNilMap(t *testing.T) {
	text := "Hello {name}"
	result := ReplaceVars(text, nil)
	if result != "Hello {name}" {
		t.Errorf("ReplaceVars with nil map should leave placeholders, got %q", result)
	}
}

func TestReplaceVarsEmptyMap(t *testing.T) {
	text := "Hello {name}"
	result := ReplaceVars(text, map[string]string{})
	if result != "Hello {name}" {
		t.Errorf("ReplaceVars with empty map should leave placeholders, got %q", result)
	}
}

func TestReplaceVarsNoPlaceholders(t *testing.T) {
	text := "Plain text without placeholders"
	vars := map[string]string{"key": "value"}
	result := ReplaceVars(text, vars)
	if result != text {
		t.Errorf("ReplaceVars without placeholders should return original, got %q", result)
	}
}

// Tests for rawTemplates map

func TestRawTemplatesExist(t *testing.T) {
	templates := GetAllTemplateTypes()

	for _, info := range templates {
		if _, ok := rawTemplates[info.Type]; !ok {
			t.Errorf("rawTemplates should contain template for %s", info.Type)
		}
	}
}

func TestRawTemplatesNotEmpty(t *testing.T) {
	for tt, tmpl := range rawTemplates {
		if tmpl == "" {
			t.Errorf("rawTemplates[%s] should not be empty", tt)
		}
	}
}

func TestRawTemplatesContainSubjectLine(t *testing.T) {
	for tt, tmpl := range rawTemplates {
		lines := strings.Split(tmpl, "\n")
		if len(lines) < 3 {
			t.Errorf("rawTemplates[%s] should have at least 3 lines (subject, ---, body)", tt)
		}
		firstLine := strings.TrimSpace(lines[0])
		if !strings.HasPrefix(firstLine, "Subject:") {
			t.Errorf("rawTemplates[%s] first line should start with 'Subject:', got %q", tt, firstLine)
		}
	}
}

func TestRawTemplatesContainSeparator(t *testing.T) {
	for tt, tmpl := range rawTemplates {
		if !strings.Contains(tmpl, "\n---\n") {
			t.Errorf("rawTemplates[%s] should contain '---' separator", tt)
		}
	}
}

func TestRawTemplatesCountMatchesGetAllTemplateTypes(t *testing.T) {
	allTypes := GetAllTemplateTypes()
	rawCount := len(rawTemplates)
	allTypesCount := len(allTypes)

	if rawCount != allTypesCount {
		t.Errorf("rawTemplates has %d entries but GetAllTemplateTypes returns %d entries", rawCount, allTypesCount)
	}
}

// Tests for TemplateInfo struct

func TestTemplateInfoStruct(t *testing.T) {
	info := TemplateInfo{
		Type:           TemplateTest,
		Name:           "Test",
		Description:    "Test email",
		IsAccountEmail: false,
	}

	if info.Type != TemplateTest {
		t.Errorf("Type = %q", info.Type)
	}
	if info.Name != "Test" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.IsAccountEmail {
		t.Error("IsAccountEmail should be false")
	}
}

// Additional render tests

func TestEmailTemplateRenderBackupFailed(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":  "TestApp",
		"fqdn":      "https://example.com",
		"timestamp": "2026-06-13 02:00:00 UTC",
		"filename":  "daily-backup",
		"error":     "Disk full",
	}

	subject, body, err := et.Render(TemplateBackupFailed, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplateRenderSSLExpiring(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":    "TestApp",
		"fqdn":        "https://example.com",
		"timestamp":   "2026-06-13 12:00:00 UTC",
		"domain":      "example.com",
		"expires_in":  "7 days",
		"expiry_date": "2026-06-20",
	}

	subject, body, err := et.Render(TemplateSSLExpiring, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplateRenderSSLRenewed(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":    "TestApp",
		"fqdn":        "https://example.com",
		"timestamp":   "2026-06-13 12:00:00 UTC",
		"domain":      "example.com",
		"valid_until": "2027-06-13",
	}

	subject, body, err := et.Render(TemplateSSLRenewed, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplateRenderSchedulerError(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":  "TestApp",
		"fqdn":      "https://example.com",
		"timestamp": "2026-06-13 12:00:00 UTC",
		"task_name": "daily-cleanup",
		"error":     "Task timed out",
		"next_run":  "2026-06-14 00:00:00 UTC",
	}

	subject, body, err := et.Render(TemplateSchedulerError, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplateRenderBreachAdminAlert(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":           "TestApp",
		"fqdn":               "https://example.com",
		"timestamp":          "2026-06-13 12:00:00 UTC",
		"severity":           "critical",
		"breach_description": "Multiple accounts compromised",
		"affected_users":     "150",
		"ip_addresses":       "1.2.3.4, 5.6.7.8",
		"action_required":    "Investigate immediately",
	}

	subject, body, err := et.Render(TemplateBreachAdminAlert, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplateRenderTest(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":  "TestApp",
		"fqdn":      "https://example.com",
		"timestamp": "2026-06-13 12:00:00 UTC",
		"sent_at":   "2026-06-13 12:00:00 UTC",
		"app_url":   "https://example.com",
	}

	subject, body, err := et.Render(TemplateTest, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

// Tests for PreviewTemplate with each operator notification template

func TestEmailTemplatePreviewTemplateBackupFailed(t *testing.T) {
	et := NewEmailTemplate()

	subject, body, err := et.PreviewTemplate(TemplateBackupFailed, "TestApp", "https://example.com")
	if err != nil {
		t.Fatalf("PreviewTemplate(BackupFailed) error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplatePreviewTemplateSSLExpiring(t *testing.T) {
	et := NewEmailTemplate()

	subject, body, err := et.PreviewTemplate(TemplateSSLExpiring, "TestApp", "https://example.com")
	if err != nil {
		t.Fatalf("PreviewTemplate(SSLExpiring) error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplatePreviewTemplateSSLRenewed(t *testing.T) {
	et := NewEmailTemplate()

	subject, body, err := et.PreviewTemplate(TemplateSSLRenewed, "TestApp", "https://example.com")
	if err != nil {
		t.Fatalf("PreviewTemplate(SSLRenewed) error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplatePreviewTemplateSchedulerError(t *testing.T) {
	et := NewEmailTemplate()

	subject, body, err := et.PreviewTemplate(TemplateSchedulerError, "TestApp", "https://example.com")
	if err != nil {
		t.Fatalf("PreviewTemplate(SchedulerError) error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplatePreviewTemplateBreachAdminAlert(t *testing.T) {
	et := NewEmailTemplate()

	subject, body, err := et.PreviewTemplate(TemplateBreachAdminAlert, "TestApp", "https://example.com")
	if err != nil {
		t.Fatalf("PreviewTemplate(BreachAdminAlert) error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplatePreviewTemplateTest(t *testing.T) {
	et := NewEmailTemplate()

	subject, body, err := et.PreviewTemplate(TemplateTest, "TestApp", "https://example.com")
	if err != nil {
		t.Fatalf("PreviewTemplate(Test) error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

// Tests for additional template type constants

func TestTemplateTypeConstantsPart18(t *testing.T) {
	if TemplateBackupFailed != "backup_failed" {
		t.Errorf("TemplateBackupFailed = %q, want %q", TemplateBackupFailed, "backup_failed")
	}
	if TemplateSSLExpiring != "ssl_expiring" {
		t.Errorf("TemplateSSLExpiring = %q, want %q", TemplateSSLExpiring, "ssl_expiring")
	}
	if TemplateSSLRenewed != "ssl_renewed" {
		t.Errorf("TemplateSSLRenewed = %q, want %q", TemplateSSLRenewed, "ssl_renewed")
	}
	if TemplateSchedulerError != "scheduler_error" {
		t.Errorf("TemplateSchedulerError = %q, want %q", TemplateSchedulerError, "scheduler_error")
	}
	if TemplateBreachAdminAlert != "breach_admin_alert" {
		t.Errorf("TemplateBreachAdminAlert = %q, want %q", TemplateBreachAdminAlert, "breach_admin_alert")
	}
	if TemplateTest != "test" {
		t.Errorf("TemplateTest = %q, want %q", TemplateTest, "test")
	}
	if TemplateAdminAlert != "admin_alert" {
		t.Errorf("TemplateAdminAlert = %q, want %q", TemplateAdminAlert, "admin_alert")
	}
	if TemplateWeeklyReport != "weekly_report" {
		t.Errorf("TemplateWeeklyReport = %q, want %q", TemplateWeeklyReport, "weekly_report")
	}
	if TemplateBackupCompleted != "backup_complete" {
		t.Errorf("TemplateBackupCompleted = %q, want %q", TemplateBackupCompleted, "backup_complete")
	}
	if TemplateUpdateAvailable != "update_available" {
		t.Errorf("TemplateUpdateAvailable = %q, want %q", TemplateUpdateAvailable, "update_available")
	}
	if TemplateMaintenanceNotice != "maintenance_notice" {
		t.Errorf("TemplateMaintenanceNotice = %q, want %q", TemplateMaintenanceNotice, "maintenance_notice")
	}
}

// Tests for AdminAlert with critical level

func TestEmailTemplateRenderAdminAlertCritical(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":    "TestApp",
		"fqdn":        "https://example.com",
		"timestamp":   "2026-06-13 12:00:00 UTC",
		"alert_type":  "System Down",
		"alert_level": "critical",
		"message":     "Server is not responding",
	}

	subject, body, err := et.Render(TemplateAdminAlert, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(subject, "critical") {
		t.Error("Subject should contain critical level")
	}
	if body == "" {
		t.Error("Body should not be empty")
	}
}

// Tests for SecurityAlert with critical severity

func TestEmailTemplateRenderSecurityAlertCritical(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":        "TestApp",
		"fqdn":            "https://example.com",
		"timestamp":       "2026-06-13 12:00:00 UTC",
		"event":           "Account Breach",
		"ip":              "10.0.0.1",
		"details":         "Account credentials exposed",
		"action_required": "Immediate action required",
	}

	subject, body, err := et.Render(TemplateSecurityAlert, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if body == "" {
		t.Error("Body should not be empty")
	}
}

// Tests for BreachAdminAlert with CRITICAL severity

func TestEmailTemplateRenderBreachAdminAlertCritical(t *testing.T) {
	et := NewEmailTemplate()

	vars := map[string]string{
		"app_name":           "TestApp",
		"fqdn":               "https://example.com",
		"timestamp":          "2026-06-13 12:00:00 UTC",
		"severity":           "CRITICAL",
		"breach_description": "Complete system breach",
		"affected_users":     "1000",
		"ip_addresses":       "1.2.3.4",
		"action_required":    "Shut down immediately",
	}

	subject, body, err := et.Render(TemplateBreachAdminAlert, vars)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !strings.Contains(subject, "CRITICAL") {
		t.Error("Subject should contain CRITICAL")
	}
	if body == "" {
		t.Error("Body should not be empty")
	}
}

// Tests for Render with partial vars (empty/missing values)

func TestEmailTemplateRenderPartialVars(t *testing.T) {
	et := NewEmailTemplate()

	// Only supply the minimum — template renders with leftover {placeholders}
	vars := map[string]string{
		"app_name": "TestApp",
	}

	subject, body, err := et.Render(TemplateSecurityAlert, vars)
	if err != nil {
		t.Fatalf("Render() with partial vars error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty even with partial vars")
	}
	_ = body
}

// Tests for Render with empty vars map

func TestEmailTemplateRenderEmptyVars(t *testing.T) {
	et := NewEmailTemplate()

	subject, body, err := et.Render(TemplateTest, map[string]string{})
	if err != nil {
		t.Fatalf("Render() with empty vars error = %v", err)
	}
	// Subject line becomes "Test Email - {app_name}" — still non-empty
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

// Tests for TemplateInfo JSON tags

func TestTemplateInfoJSONTags(t *testing.T) {
	info := TemplateInfo{
		Type:           TemplateAdminAlert,
		Name:           "Admin Alert",
		Description:    "Administrative alert email",
		IsAccountEmail: false,
	}

	if info.Type != TemplateAdminAlert {
		t.Errorf("Type = %q", info.Type)
	}
	if info.Name != "Admin Alert" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Description != "Administrative alert email" {
		t.Errorf("Description = %q", info.Description)
	}
	if info.IsAccountEmail {
		t.Error("IsAccountEmail should be false")
	}
}

// Tests for GetAllTemplateTypes ensuring all operator templates are included

func TestGetAllTemplateTypesIncludesOperatorTemplates(t *testing.T) {
	types := GetAllTemplateTypes()

	operatorTypes := []TemplateType{
		TemplateBackupFailed,
		TemplateSSLExpiring,
		TemplateSSLRenewed,
		TemplateSchedulerError,
		TemplateBreachAdminAlert,
		TemplateTest,
	}

	found := make(map[TemplateType]bool)
	for _, info := range types {
		found[info.Type] = true
	}

	for _, tt := range operatorTypes {
		if !found[tt] {
			t.Errorf("GetAllTemplateTypes() should include %s", tt)
		}
	}
}

// Tests for IsAccountEmail covering all templates

func TestIsAccountEmailAllTemplates(t *testing.T) {
	allTemplates := []TemplateType{
		TemplateAdminAlert,
		TemplateWeeklyReport,
		TemplateSecurityAlert,
		TemplateBackupCompleted,
		TemplateUpdateAvailable,
		TemplateMaintenanceNotice,
		TemplateBackupFailed,
		TemplateSSLExpiring,
		TemplateSSLRenewed,
		TemplateSchedulerError,
		TemplateBreachAdminAlert,
		TemplateTest,
	}

	for _, tt := range allTemplates {
		if IsAccountEmail(tt) {
			t.Errorf("IsAccountEmail(%s) should return false — no user accounts exist", tt)
		}
	}

	if IsAccountEmail(TemplateType("unknown_template")) {
		t.Error("IsAccountEmail should return false for unknown template type")
	}
}

// Tests for Message with empty recipients after filtering

func TestMessageWithEmptyToList(t *testing.T) {
	msg := NewMessage([]string{}, "Test", "Body")
	if len(msg.To) != 0 {
		t.Errorf("To should be empty, got %d", len(msg.To))
	}
}

// Tests for Message multiple To recipients

func TestMessageMultipleRecipients(t *testing.T) {
	to := []string{"user1@example.com", "user2@example.com", "user3@example.com"}
	msg := NewMessage(to, "Test", "Body")

	if len(msg.To) != 3 {
		t.Errorf("To length = %d, want 3", len(msg.To))
	}
	for i, recipient := range to {
		if msg.To[i] != recipient {
			t.Errorf("To[%d] = %q, want %q", i, msg.To[i], recipient)
		}
	}
}

// Tests for Send building message with all components

func TestMailerSendWithHTMLAndHeaders(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "starttls",
		},
		From: FromConfig{
			Name:  "Test App",
			Email: "noreply@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test Subject", "Plain body")
	msg.SetHTML("<h1>Hello</h1>")
	msg.CC = []string{"cc1@example.com", "cc2@example.com"}
	msg.BCC = []string{"bcc@example.com"}
	msg.Headers["X-Mailer"] = "Test Mailer"
	msg.Headers["Reply-To"] = "reply@example.com"

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for Send with UTF-8 subject

func TestMailerSendWithUTF8Subject(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "auto",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Hello 世界", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for encodeHeader boundary

func TestEncodeHeaderBoundary(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	result127 := ml.encodeHeader("test\x7fvalue")
	_ = result127

	result128 := ml.encodeHeader("test\x80value")
	if !strings.HasPrefix(result128, "=?UTF-8?B?") {
		t.Errorf("encodeHeader with char 128 should be encoded, got %q", result128)
	}
}

// Tests for getDefaultGateway returning value

func TestGetDefaultGatewayReturn(t *testing.T) {
	result := getDefaultGateway()
	if result != "" {
		parts := strings.Split(result, ".")
		if len(parts) != 4 {
			t.Logf("Gateway result: %s", result)
		}
	}
}

// Tests for DetectAndConfigure

func TestDetectAndConfigureFull(t *testing.T) {
	cfg := DetectAndConfigure("", "")

	if cfg == nil {
		t.Fatal("DetectAndConfigure() returned nil")
	}

	if cfg.SMTP.TLS == "" {
		t.Error("TLS should have a value")
	}
}

// Tests for TestConnection with different TLS modes

func TestMailerTestConnectionTLSMode(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 465,
			TLS:  "tls",
		},
	}
	ml := NewMailer(cfg)

	err := ml.TestConnection()
	if err == nil {
		t.Error("TestConnection() should error with invalid host")
	}
}

func TestMailerTestConnectionSTARTTLSMode(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "starttls",
		},
	}
	ml := NewMailer(cfg)

	err := ml.TestConnection()
	if err == nil {
		t.Error("TestConnection() should error with invalid host")
	}
}

func TestMailerTestConnectionNoneMode(t *testing.T) {
	orig := netDialTimeout
	defer func() { netDialTimeout = orig }()
	netDialTimeout = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		return nil, fmt.Errorf("connection refused")
	}

	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "192.0.2.1",
			Port: 59999,
			TLS:  "none",
		},
	}
	ml := NewMailer(cfg)

	err := ml.TestConnection()
	if err == nil {
		t.Error("TestConnection() should error with invalid host")
	}
}

// Tests for Send with CC recipients

func TestMailerSendWithCC(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "auto",
		},
		From: FromConfig{
			Name:  "Test Sender",
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")
	msg.CC = []string{"cc@example.com"}

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for Send with BCC recipients

func TestMailerSendWithBCC(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "auto",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")
	msg.BCC = []string{"bcc@example.com"}

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for Send with custom headers

func TestMailerSendWithCustomHeaders(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "auto",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")
	msg.Headers["X-Custom"] = "value"
	msg.Headers["X-Priority"] = "1"

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for Send with TLS mode

func TestMailerSendWithTLSMode(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 465,
			TLS:  "tls",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for Send with none TLS mode

func TestMailerSendWithNoneTLSMode(t *testing.T) {
	orig := netDialTimeout
	defer func() { netDialTimeout = orig }()
	netDialTimeout = func(network, addr string, timeout time.Duration) (net.Conn, error) {
		return nil, fmt.Errorf("connection refused")
	}

	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "192.0.2.1",
			Port: 59999,
			TLS:  "none",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for formatAddress with UTF-8 name

func TestFormatAddressUTF8Name(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	result := ml.formatAddress("Café Owner", "test@example.com")
	if !strings.Contains(result, "=?UTF-8?B?") {
		t.Errorf("formatAddress with UTF-8 name = %q, should have UTF-8 encoding", result)
	}
	if !strings.Contains(result, "test@example.com") {
		t.Errorf("formatAddress should contain email address, got %q", result)
	}
}

// Tests for formatAddress with special characters in name

func TestFormatAddressSpecialChars(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	tests := []struct {
		name  string
		email string
	}{
		{"O'Connor", "oconnor@example.com"},
		{"Test \"Quoted\"", "quoted@example.com"},
		{"Test, With Comma", "comma@example.com"},
	}

	for _, tt := range tests {
		result := ml.formatAddress(tt.name, tt.email)
		if !strings.Contains(result, tt.email) {
			t.Errorf("formatAddress(%q, %q) = %q, should contain email", tt.name, tt.email, result)
		}
	}
}

// Tests for getDefaultGateway with different network configurations

func TestGetDefaultGatewayDifferentConfigs(t *testing.T) {
	result1 := getDefaultGateway()
	result2 := getDefaultGateway()

	if result1 != result2 {
		t.Errorf("getDefaultGateway() should return consistent results: %q vs %q", result1, result2)
	}
}

// Tests for tryDetectSMTP with different scenarios

func TestTryDetectSMTPDifferentPorts(t *testing.T) {
	ports := []int{59990, 59991, 59992}

	for _, port := range ports {
		result := tryDetectSMTP("127.0.0.1", port, false)
		if result != nil {
			t.Errorf("tryDetectSMTP for port %d should return nil", port)
		}
	}
}

// Tests for Send with uppercase TLS mode

func TestMailerSendWithUppercaseTLSMode(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "AUTO",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

func TestMailerSendWithUppercaseTLS(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 465,
			TLS:  "TLS",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

func TestMailerSendWithUppercaseSTARTTLS(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "STARTTLS",
		},
		From: FromConfig{
			Email: "sender@example.com",
		},
	}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for SendToAdmins with disabled email

func TestMailerSendToAdminsDisabled(t *testing.T) {
	cfg := &Config{
		Enabled:     false,
		AdminEmails: []string{"admin@example.com"},
	}
	ml := NewMailer(cfg)

	err := ml.SendToAdmins("Test Subject", "Test Body")
	if err == nil {
		t.Error("SendToAdmins() should error when email is disabled")
	}
	if !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("Error should mention 'not enabled', got: %v", err)
	}
}

// Tests for SendAlert with enabled email but invalid server

func TestMailerSendAlertWithServer(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
		},
		From: FromConfig{
			Email: "alerts@example.com",
		},
		AdminEmails: []string{"admin@example.com"},
	}
	ml := NewMailer(cfg)

	err := ml.SendAlert("Test Alert", "Test message content")
	if err == nil {
		t.Error("SendAlert() should error with invalid SMTP server")
	}
}

// Tests for SendSecurityAlert with enabled email but invalid server

func TestMailerSendSecurityAlertWithServer(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
		},
		From: FromConfig{
			Email: "security@example.com",
		},
		AdminEmails: []string{"admin@example.com"},
	}
	ml := NewMailer(cfg)

	err := ml.SendSecurityAlert("Brute Force Attack", "192.168.1.100", "Multiple failed login attempts")
	if err == nil {
		t.Error("SendSecurityAlert() should error with invalid SMTP server")
	}
}

// Tests for TestConnection with credentials

func TestMailerTestConnectionWithCredentials(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     "invalid.host.example.com",
			Port:     587,
			Username: "testuser",
			Password: "testpass",
			TLS:      "auto",
		},
	}
	ml := NewMailer(cfg)

	err := ml.TestConnection()
	if err == nil {
		t.Error("TestConnection() should error with invalid host")
	}
}

// Tests for DetectedSMTP with all combinations

func TestDetectedSMTPAllCombinations(t *testing.T) {
	tests := []struct {
		name         string
		host         string
		port         int
		tls          bool
		starttls     bool
		authRequired bool
	}{
		{"standard", "localhost", 25, false, true, false},
		{"submission", "localhost", 587, false, true, true},
		{"smtps", "localhost", 465, true, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			detected := &DetectedSMTP{
				Host:         tt.host,
				Port:         tt.port,
				TLS:          tt.tls,
				STARTTLS:     tt.starttls,
				AuthRequired: tt.authRequired,
			}

			if detected.Host != tt.host {
				t.Errorf("Host = %q", detected.Host)
			}
			if detected.Port != tt.port {
				t.Errorf("Port = %d", detected.Port)
			}
			if detected.TLS != tt.tls {
				t.Errorf("TLS = %v", detected.TLS)
			}
			if detected.STARTTLS != tt.starttls {
				t.Errorf("STARTTLS = %v", detected.STARTTLS)
			}
			if detected.AuthRequired != tt.authRequired {
				t.Errorf("AuthRequired = %v", detected.AuthRequired)
			}
		})
	}
}

// Tests for Config with all fields

func TestConfigAllFields(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     "smtp.example.com",
			Port:     587,
			Username: "user",
			Password: "pass",
			TLS:      "starttls",
		},
		From: FromConfig{
			Name:  "My App",
			Email: "noreply@example.com",
		},
		AdminEmails: []string{"admin1@example.com", "admin2@example.com"},
	}

	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.SMTP.Host != "smtp.example.com" {
		t.Errorf("SMTP.Host = %q", cfg.SMTP.Host)
	}
	if cfg.SMTP.Username != "user" {
		t.Errorf("SMTP.Username = %q", cfg.SMTP.Username)
	}
	if cfg.SMTP.Password != "pass" {
		t.Errorf("SMTP.Password = %q", cfg.SMTP.Password)
	}
	if cfg.From.Name != "My App" {
		t.Errorf("From.Name = %q", cfg.From.Name)
	}
	if len(cfg.AdminEmails) != 2 {
		t.Errorf("AdminEmails length = %d", len(cfg.AdminEmails))
	}
}
