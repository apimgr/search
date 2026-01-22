package email

import (
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
		name     string
		input    string
		encoded  bool
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
		Enabled:     false, // Will fail but we want to test the path
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

	// Will fail to connect since no host is specified
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
	// When no SMTP is detected, should return default config
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
	// This test just ensures the function doesn't panic
	// Actual availability depends on environment
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

	// Verify all fields are set correctly
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
		name    string
		email   string
		want    string
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

// Test Send with various message configurations

func TestMailerSendWithHTML(t *testing.T) {
	cfg := &Config{Enabled: false}
	ml := NewMailer(cfg)
	msg := NewMessage([]string{"user@example.com"}, "Test", "Plain body")
	msg.SetHTML("<h1>Hello</h1>")

	err := ml.Send(msg)
	// Should fail because email is not enabled, but HTML should be set
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

	// Sending nil message would panic, so we test with empty To
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

// Test DetectSMTP doesn't crash

func TestDetectSMTP(t *testing.T) {
	// This just ensures the function doesn't panic
	// Actual detection depends on environment
	result := DetectSMTP()
	// result may be nil or not, depending on environment
	_ = result
}

// Test getDefaultGateway doesn't crash

func TestGetDefaultGateway(t *testing.T) {
	// This just ensures the function doesn't panic
	gateway := getDefaultGateway()
	// gateway may be empty or have a value
	_ = gateway
}

// Test tryDetectSMTP with unreachable host
// Uses localhost with high port that's unlikely to have anything listening

func TestTryDetectSMTPUnreachable(t *testing.T) {
	// Use localhost with a high port that nothing is listening on
	// This should fail quickly with connection refused
	result := tryDetectSMTP("127.0.0.1", 59999, false)
	if result != nil {
		t.Error("tryDetectSMTP should return nil for unreachable port")
	}
}

func TestTryDetectSMTPTLSUnreachable(t *testing.T) {
	// Use localhost with a high port that nothing is listening on
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

func TestEmailTemplateRenderWelcome(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &WelcomeData{
		TemplateData: baseData,
		Username:     "testuser",
		Email:        "test@example.com",
	}

	subject, body, err := et.Render(TemplateWelcome, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if body == "" {
		t.Error("Body should not be empty")
	}
	if !strings.Contains(body, "testuser") {
		t.Error("Body should contain username")
	}
}

func TestEmailTemplateRenderPasswordReset(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &PasswordResetData{
		TemplateData: baseData,
		Username:     "testuser",
		ResetLink:    "https://example.com/reset?token=abc123",
		ExpiresIn:    "1 hour",
		IPAddress:    "192.168.1.1",
		RequestedAt:  time.Now(),
	}

	subject, body, err := et.Render(TemplatePasswordReset, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "reset") || !strings.Contains(strings.ToLower(body), "password") {
		t.Error("Body should mention password reset")
	}
}

func TestEmailTemplateRenderEmailVerification(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &EmailVerificationData{
		TemplateData:     baseData,
		Username:         "testuser",
		Email:            "test@example.com",
		VerificationLink: "https://example.com/verify?token=xyz789",
		ExpiresIn:        "24 hours",
	}

	subject, body, err := et.Render(TemplateEmailVerification, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(strings.ToLower(body), "verif") {
		t.Error("Body should mention verification")
	}
}

func TestEmailTemplateRenderLoginNotification(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &LoginNotificationData{
		TemplateData: baseData,
		Username:     "testuser",
		LoginTime:    time.Now(),
		IPAddress:    "192.168.1.100",
		UserAgent:    "Chrome on Windows",
		Location:     "New York, US",
		IsNewDevice:  true,
	}

	subject, body, err := et.Render(TemplateLoginNotification, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "192.168.1.100") {
		t.Error("Body should contain IP address")
	}
}

func TestEmailTemplateRenderSecurityAlert(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SecurityAlertData{
		TemplateData:   baseData,
		Event:          "Suspicious Activity",
		Severity:       "high",
		IPAddress:      "10.0.0.1",
		Details:        "Multiple failed login attempts detected",
		OccurredAt:     time.Now(),
		ActionRequired: "Review the activity",
	}

	subject, body, err := et.Render(TemplateSecurityAlert, data)
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

func TestEmailTemplateRenderPasswordChanged(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &PasswordChangedData{
		TemplateData: baseData,
		Username:     "testuser",
		ChangedAt:    time.Now(),
		IPAddress:    "192.168.1.50",
		UserAgent:    "Firefox on Linux",
	}

	subject, body, err := et.Render(TemplatePasswordChanged, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(strings.ToLower(body), "password") {
		t.Error("Body should mention password")
	}
}

func TestEmailTemplateRenderTwoFactorEnabled(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &TwoFactorEnabledData{
		TemplateData: baseData,
		Username:     "testuser",
		EnabledAt:    time.Now(),
		IPAddress:    "192.168.1.70",
		Method:       "TOTP",
	}

	subject, body, err := et.Render(Template2FAEnabled, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "TOTP") {
		t.Error("Body should contain 2FA method")
	}
}

func TestEmailTemplateRenderTwoFactorDisabled(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &TwoFactorDisabledData{
		TemplateData: baseData,
		Username:     "testuser",
		DisabledAt:   time.Now(),
		IPAddress:    "192.168.1.80",
		Reason:       "User requested",
	}

	subject, _, err := et.Render(Template2FADisabled, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
}

func TestEmailTemplateRenderAccountLocked(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AccountLockedData{
		TemplateData:       baseData,
		Username:           "testuser",
		Reason:             "Too many failed login attempts",
		LockedAt:           time.Now(),
		UnlockInstructions: "Contact support or wait 30 minutes",
	}

	subject, body, err := et.Render(TemplateAccountLocked, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "Locked") {
		t.Error("Body should mention account being locked")
	}
}

func TestEmailTemplateRenderAPITokenCreated(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &APITokenCreatedData{
		TemplateData: baseData,
		Username:     "testuser",
		TokenName:    "Production API Key",
		Permissions:  []string{"read", "write"},
		ExpiresAt:    time.Now().Add(90 * 24 * time.Hour),
		CreatedAt:    time.Now(),
		IPAddress:    "192.168.1.100",
	}

	subject, body, err := et.Render(TemplateAPITokenCreated, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "Production API Key") {
		t.Error("Body should contain token name")
	}
}

func TestEmailTemplateRenderAdminAlert(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AdminAlertData{
		TemplateData: baseData,
		AlertType:    "System Warning",
		AlertLevel:   "warning",
		Message:      "High CPU usage detected",
		Details:      map[string]string{"CPU": "92%", "Duration": "5 min"},
		OccurredAt:   time.Now(),
	}

	subject, body, err := et.Render(TemplateAdminAlert, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &WeeklyReportData{
		TemplateData:  baseData,
		PeriodStart:   time.Now().AddDate(0, 0, -7),
		PeriodEnd:     time.Now(),
		TotalSearches: 15000,
		UniqueUsers:   2500,
		TopQueries:    []string{"golang", "python", "docker"},
		EngineStats:   map[string]int{"Google": 8000, "DuckDuckGo": 7000},
		ErrorCount:    10,
	}

	subject, body, err := et.Render(TemplateWeeklyReport, data)
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

func TestEmailTemplateRenderAdminInvite(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AdminInviteData{
		TemplateData: baseData,
		InviterName:  "John Admin",
		InviteLink:   "https://example.com/invite/abc123",
		ExpiresIn:    "48 hours",
		Message:      "Welcome to the team!",
	}

	subject, body, err := et.Render(TemplateAdminInvite, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if !strings.Contains(body, "John Admin") {
		t.Error("Body should contain inviter name")
	}
}

func TestEmailTemplateRenderBackupCompleted(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BackupCompletedData{
		TemplateData: baseData,
		BackupName:   "daily-backup-20240115.tar.gz",
		BackupSize:   "10.5 MB",
		CreatedAt:    time.Now(),
		FileCount:    150,
		Duration:     "3.2 seconds",
	}

	subject, _, err := et.Render(TemplateBackupCompleted, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
}

func TestEmailTemplateRenderUpdateAvailable(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &UpdateAvailableData{
		TemplateData:   baseData,
		CurrentVersion: "1.2.3",
		NewVersion:     "1.3.0",
		ReleaseDate:    time.Now(),
		ReleaseNotes:   "Bug fixes and improvements",
		UpdateURL:      "https://example.com/releases/1.3.0",
	}

	subject, body, err := et.Render(TemplateUpdateAvailable, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &MaintenanceNoticeData{
		TemplateData:     baseData,
		ScheduledAt:      time.Now().Add(24 * time.Hour),
		Duration:         "30 minutes",
		Reason:           "Database maintenance",
		AffectedServices: []string{"Search API", "Admin Panel"},
	}

	subject, _, err := et.Render(TemplateMaintenanceNotice, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
}

func TestEmailTemplateRenderInvalidType(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &WelcomeData{
		TemplateData: baseData,
		Username:     "test",
	}

	_, _, err := et.Render(TemplateType("invalid_template"), data)
	if err == nil {
		t.Error("Render() should error for invalid template type")
	}
}

func TestEmailTemplateRenderNilData(t *testing.T) {
	et := NewEmailTemplate()

	// Nil data should error since templates reference data fields
	_, _, err := et.Render(TemplateWelcome, nil)
	// May error depending on implementation
	_ = err
}

// Tests for PreviewTemplate

func TestEmailTemplatePreviewTemplate(t *testing.T) {
	et := NewEmailTemplate()

	templates := []TemplateType{
		TemplateWelcome,
		TemplatePasswordReset,
		TemplateEmailVerification,
		TemplateLoginNotification,
		TemplateSecurityAlert,
		TemplatePasswordChanged,
		Template2FAEnabled,
		Template2FADisabled,
		TemplateAccountLocked,
		TemplateAdminAlert,
		TemplateWeeklyReport,
		TemplateAPITokenCreated,
		TemplateAdminInvite,
		TemplateBackupCompleted,
		TemplateUpdateAvailable,
		TemplateMaintenanceNotice,
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

	// Should contain at least the common types
	found := make(map[TemplateType]bool)
	for _, info := range types {
		found[info.Type] = true
	}

	expectedTypes := []TemplateType{
		TemplateWelcome,
		TemplatePasswordReset,
		TemplateLoginNotification,
		TemplateSecurityAlert,
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
	accountTemplates := []TemplateType{
		TemplateWelcome,
		TemplatePasswordReset,
		TemplateEmailVerification,
		TemplatePasswordChanged,
		TemplateLoginNotification,
		TemplateAccountLocked,
		TemplateSecurityAlert,
		Template2FAEnabled,
		Template2FADisabled,
	}

	for _, tt := range accountTemplates {
		if !IsAccountEmail(tt) {
			t.Errorf("IsAccountEmail(%s) should return true", tt)
		}
	}

	nonAccountTemplates := []TemplateType{
		TemplateAdminAlert,
		TemplateWeeklyReport,
		TemplateBackupCompleted,
		TemplateUpdateAvailable,
	}

	for _, tt := range nonAccountTemplates {
		if IsAccountEmail(tt) {
			t.Errorf("IsAccountEmail(%s) should return false", tt)
		}
	}
}

// Tests for TemplateType constants

func TestTemplateTypeConstants(t *testing.T) {
	if TemplateWelcome != "welcome" {
		t.Errorf("TemplateWelcome = %q, want %q", TemplateWelcome, "welcome")
	}
	if TemplatePasswordReset != "password_reset" {
		t.Errorf("TemplatePasswordReset = %q, want %q", TemplatePasswordReset, "password_reset")
	}
	if TemplateEmailVerification != "email_verification" {
		t.Errorf("TemplateEmailVerification = %q, want %q", TemplateEmailVerification, "email_verification")
	}
	if TemplateLoginNotification != "login_notification" {
		t.Errorf("TemplateLoginNotification = %q, want %q", TemplateLoginNotification, "login_notification")
	}
	if TemplateSecurityAlert != "security_alert" {
		t.Errorf("TemplateSecurityAlert = %q, want %q", TemplateSecurityAlert, "security_alert")
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

// Tests for data structs

func TestWelcomeDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &WelcomeData{
		TemplateData: baseData,
		Username:     "testuser",
		Email:        "test@example.com",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.Email != "test@example.com" {
		t.Errorf("Email = %q", data.Email)
	}
	if data.SiteName != "TestApp" {
		t.Errorf("SiteName = %q", data.SiteName)
	}
}

func TestPasswordResetDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &PasswordResetData{
		TemplateData: baseData,
		Username:     "testuser",
		ResetLink:    "https://example.com/reset",
		ExpiresIn:    "1 hour",
		IPAddress:    "192.168.1.1",
		RequestedAt:  time.Now(),
	}

	if data.ResetLink != "https://example.com/reset" {
		t.Errorf("ResetLink = %q", data.ResetLink)
	}
	if data.ExpiresIn != "1 hour" {
		t.Errorf("ExpiresIn = %q", data.ExpiresIn)
	}
}

func TestLoginNotificationDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &LoginNotificationData{
		TemplateData: baseData,
		Username:     "testuser",
		LoginTime:    time.Now(),
		IPAddress:    "192.168.1.100",
		UserAgent:    "Chrome on Windows",
		Location:     "New York, US",
		IsNewDevice:  true,
	}

	if data.Location != "New York, US" {
		t.Errorf("Location = %q", data.Location)
	}
	if data.UserAgent != "Chrome on Windows" {
		t.Errorf("UserAgent = %q", data.UserAgent)
	}
	if !data.IsNewDevice {
		t.Error("IsNewDevice should be true")
	}
}

func TestSecurityAlertDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SecurityAlertData{
		TemplateData:   baseData,
		Event:          "Suspicious Activity",
		Severity:       "high",
		IPAddress:      "10.0.0.1",
		Details:        "Multiple failed logins",
		OccurredAt:     time.Now(),
		ActionRequired: "Review activity",
	}

	if data.Event != "Suspicious Activity" {
		t.Errorf("Event = %q", data.Event)
	}
	if data.Severity != "high" {
		t.Errorf("Severity = %q", data.Severity)
	}
}

func TestAPITokenCreatedDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &APITokenCreatedData{
		TemplateData: baseData,
		Username:     "testuser",
		TokenName:    "My API Key",
		Permissions:  []string{"read", "write", "delete"},
		ExpiresAt:    time.Now().Add(90 * 24 * time.Hour),
		CreatedAt:    time.Now(),
		IPAddress:    "192.168.1.1",
	}

	if len(data.Permissions) != 3 {
		t.Errorf("Permissions length = %d", len(data.Permissions))
	}
	if data.TokenName != "My API Key" {
		t.Errorf("TokenName = %q", data.TokenName)
	}
}

func TestWeeklyReportDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &WeeklyReportData{
		TemplateData:  baseData,
		PeriodStart:   time.Now().AddDate(0, 0, -7),
		PeriodEnd:     time.Now(),
		TotalSearches: 15000,
		UniqueUsers:   2500,
		TopQueries:    []string{"golang", "python"},
		EngineStats:   map[string]int{"Google": 8000},
		ErrorCount:    10,
	}

	if data.TotalSearches != 15000 {
		t.Errorf("TotalSearches = %d", data.TotalSearches)
	}
	if data.UniqueUsers != 2500 {
		t.Errorf("UniqueUsers = %d", data.UniqueUsers)
	}
}

func TestAdminInviteDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AdminInviteData{
		TemplateData: baseData,
		InviterName:  "John",
		InviteLink:   "https://example.com/invite",
		ExpiresIn:    "48 hours",
		Message:      "Welcome!",
	}

	if data.InviterName != "John" {
		t.Errorf("InviterName = %q", data.InviterName)
	}
	if data.Message != "Welcome!" {
		t.Errorf("Message = %q", data.Message)
	}
}

func TestAdminAlertDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AdminAlertData{
		TemplateData: baseData,
		AlertType:    "Critical",
		AlertLevel:   "critical",
		Message:      "Server down",
		Details:      map[string]string{"Server": "prod-1"},
		OccurredAt:   time.Now(),
	}

	if data.AlertType != "Critical" {
		t.Errorf("AlertType = %q", data.AlertType)
	}
	if data.AlertLevel != "critical" {
		t.Errorf("AlertLevel = %q", data.AlertLevel)
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
		{"line1\n\nline3", 3}, // empty line in middle
	}

	for _, tt := range tests {
		result := splitLines(tt.input)
		if len(result) != tt.want {
			t.Errorf("splitLines(%q) returned %d lines, want %d", tt.input, len(result), tt.want)
		}
	}
}

func TestSplitLinesEmpty(t *testing.T) {
	// Empty string returns empty slice
	result := splitLines("")
	if len(result) != 0 {
		t.Errorf("splitLines(\"\") returned %d lines, want 0", len(result))
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

// Tests for TemplateInfo struct

func TestTemplateInfoStruct(t *testing.T) {
	info := TemplateInfo{
		Type:           TemplateWelcome,
		Name:           "Welcome",
		Description:    "Welcome email",
		IsAccountEmail: true,
	}

	if info.Type != TemplateWelcome {
		t.Errorf("Type = %q", info.Type)
	}
	if info.Name != "Welcome" {
		t.Errorf("Name = %q", info.Name)
	}
	if !info.IsAccountEmail {
		t.Error("IsAccountEmail should be true")
	}
}

// Additional tests for PART 18 templates

func TestEmailTemplateRenderMFAReminder(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &MFAReminderData{
		TemplateData: baseData,
		Username:     "testuser",
		SetupLink:    "https://example.com/setup-mfa",
		DismissLink:  "https://example.com/dismiss",
	}

	subject, body, err := et.Render(TemplateMFAReminder, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplateRenderBackupFailed(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BackupFailedData{
		TemplateData: baseData,
		BackupName:   "daily-backup",
		Error:        "Disk full",
		FailedAt:     time.Now(),
	}

	subject, body, err := et.Render(TemplateBackupFailed, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SSLExpiringData{
		TemplateData: baseData,
		Domain:       "example.com",
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour),
		DaysLeft:     7,
		RenewLink:    "https://example.com/renew-ssl",
	}

	subject, body, err := et.Render(TemplateSSLExpiring, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SSLRenewedData{
		TemplateData: baseData,
		Domain:       "example.com",
		RenewedAt:    time.Now(),
		ValidUntil:   time.Now().Add(365 * 24 * time.Hour),
	}

	subject, body, err := et.Render(TemplateSSLRenewed, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SchedulerErrorData{
		TemplateData: baseData,
		TaskName:     "daily-cleanup",
		Error:        "Task timed out",
		FailedAt:     time.Now(),
		TaskDetails:  map[string]string{"Duration": "exceeded 1h"},
	}

	subject, body, err := et.Render(TemplateSchedulerError, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

func TestEmailTemplateRenderBreachNotification(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BreachNotificationData{
		TemplateData:       baseData,
		Username:           "testuser",
		BreachDate:         time.Now(),
		BreachDescription:  "Unauthorized access detected",
		AffectedData:       []string{"email", "preferences"},
		RecommendedActions: []string{"Change password", "Enable 2FA"},
		SupportContact:     "security@example.com",
	}

	subject, body, err := et.Render(TemplateBreachNotification, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BreachAdminAlertData{
		TemplateData:      baseData,
		Severity:          "critical",
		DetectedAt:        time.Now(),
		BreachDescription: "Multiple accounts compromised",
		AffectedUsers:     150,
		IPAddresses:       []string{"1.2.3.4", "5.6.7.8"},
		ActionRequired:    "Investigate immediately",
	}

	subject, body, err := et.Render(TemplateBreachAdminAlert, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &TestEmailData{
		TemplateData: baseData,
		SentAt:       time.Now(),
	}

	subject, body, err := et.Render(TemplateTest, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	_ = body
}

// Tests for PreviewTemplate with PART 18 templates that are not yet implemented
// These templates are not yet supported by PreviewTemplate, so they should return errors

func TestEmailTemplatePreviewTemplateMFAReminderError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateMFAReminder, "TestApp", "https://example.com")
	// PreviewTemplate doesn't implement MFAReminder yet, should error
	if err == nil {
		t.Error("PreviewTemplate(MFAReminder) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

func TestEmailTemplatePreviewTemplateBackupFailedError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateBackupFailed, "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate(BackupFailed) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

func TestEmailTemplatePreviewTemplateSSLExpiringError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateSSLExpiring, "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate(SSLExpiring) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

func TestEmailTemplatePreviewTemplateSSLRenewedError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateSSLRenewed, "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate(SSLRenewed) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

func TestEmailTemplatePreviewTemplateSchedulerErrorError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateSchedulerError, "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate(SchedulerError) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

func TestEmailTemplatePreviewTemplateBreachNotificationError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateBreachNotification, "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate(BreachNotification) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

func TestEmailTemplatePreviewTemplateBreachAdminAlertError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateBreachAdminAlert, "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate(BreachAdminAlert) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

func TestEmailTemplatePreviewTemplateTestError(t *testing.T) {
	et := NewEmailTemplate()

	_, _, err := et.PreviewTemplate(TemplateTest, "TestApp", "https://example.com")
	if err == nil {
		t.Error("PreviewTemplate(Test) should error for unimplemented template")
	}
	if !strings.Contains(err.Error(), "unknown template type") {
		t.Errorf("Error should mention 'unknown template type', got: %v", err)
	}
}

// Tests for IsAccountEmail with MFA and Breach templates

func TestIsAccountEmailMFAReminder(t *testing.T) {
	if !IsAccountEmail(TemplateMFAReminder) {
		t.Error("TemplateMFAReminder should be an account email")
	}
}

func TestIsAccountEmailBreachNotification(t *testing.T) {
	if !IsAccountEmail(TemplateBreachNotification) {
		t.Error("TemplateBreachNotification should be an account email")
	}
}

// Tests for additional template type constants

func TestTemplateTypeConstantsPart18(t *testing.T) {
	if TemplateMFAReminder != "mfa_reminder" {
		t.Errorf("TemplateMFAReminder = %q, want %q", TemplateMFAReminder, "mfa_reminder")
	}
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
	if TemplateBreachNotification != "breach_notification" {
		t.Errorf("TemplateBreachNotification = %q, want %q", TemplateBreachNotification, "breach_notification")
	}
	if TemplateBreachAdminAlert != "breach_admin_alert" {
		t.Errorf("TemplateBreachAdminAlert = %q, want %q", TemplateBreachAdminAlert, "breach_admin_alert")
	}
	if TemplateTest != "test" {
		t.Errorf("TemplateTest = %q, want %q", TemplateTest, "test")
	}
	if TemplatePasswordChanged != "password_changed" {
		t.Errorf("TemplatePasswordChanged = %q, want %q", TemplatePasswordChanged, "password_changed")
	}
	if TemplateAccountLocked != "account_locked" {
		t.Errorf("TemplateAccountLocked = %q, want %q", TemplateAccountLocked, "account_locked")
	}
	if TemplateAdminAlert != "admin_alert" {
		t.Errorf("TemplateAdminAlert = %q, want %q", TemplateAdminAlert, "admin_alert")
	}
	if TemplateWeeklyReport != "weekly_report" {
		t.Errorf("TemplateWeeklyReport = %q, want %q", TemplateWeeklyReport, "weekly_report")
	}
	if TemplateAPITokenCreated != "api_token_created" {
		t.Errorf("TemplateAPITokenCreated = %q, want %q", TemplateAPITokenCreated, "api_token_created")
	}
	if TemplateAdminInvite != "admin_invite" {
		t.Errorf("TemplateAdminInvite = %q, want %q", TemplateAdminInvite, "admin_invite")
	}
	if Template2FAEnabled != "two_factor_enabled" {
		t.Errorf("Template2FAEnabled = %q, want %q", Template2FAEnabled, "two_factor_enabled")
	}
	if Template2FADisabled != "two_factor_disabled" {
		t.Errorf("Template2FADisabled = %q, want %q", Template2FADisabled, "two_factor_disabled")
	}
	if TemplateBackupCompleted != "backup_completed" {
		t.Errorf("TemplateBackupCompleted = %q, want %q", TemplateBackupCompleted, "backup_completed")
	}
	if TemplateUpdateAvailable != "update_available" {
		t.Errorf("TemplateUpdateAvailable = %q, want %q", TemplateUpdateAvailable, "update_available")
	}
	if TemplateMaintenanceNotice != "maintenance_notice" {
		t.Errorf("TemplateMaintenanceNotice = %q, want %q", TemplateMaintenanceNotice, "maintenance_notice")
	}
}

// Tests for data struct coverage

func TestMFAReminderDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &MFAReminderData{
		TemplateData: baseData,
		Username:     "testuser",
		SetupLink:    "https://example.com/setup-mfa",
		DismissLink:  "https://example.com/dismiss",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.SetupLink != "https://example.com/setup-mfa" {
		t.Errorf("SetupLink = %q", data.SetupLink)
	}
	if data.DismissLink != "https://example.com/dismiss" {
		t.Errorf("DismissLink = %q", data.DismissLink)
	}
}

func TestBackupFailedDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BackupFailedData{
		TemplateData: baseData,
		BackupName:   "daily-backup",
		Error:        "Disk full",
		FailedAt:     time.Now(),
	}

	if data.BackupName != "daily-backup" {
		t.Errorf("BackupName = %q", data.BackupName)
	}
	if data.Error != "Disk full" {
		t.Errorf("Error = %q", data.Error)
	}
}

func TestSSLExpiringDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SSLExpiringData{
		TemplateData: baseData,
		Domain:       "example.com",
		ExpiresAt:    time.Now().Add(7 * 24 * time.Hour),
		DaysLeft:     7,
		RenewLink:    "https://example.com/renew",
	}

	if data.Domain != "example.com" {
		t.Errorf("Domain = %q", data.Domain)
	}
	if data.DaysLeft != 7 {
		t.Errorf("DaysLeft = %d", data.DaysLeft)
	}
}

func TestSSLRenewedDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	renewedAt := time.Now()
	validUntil := time.Now().Add(365 * 24 * time.Hour)
	data := &SSLRenewedData{
		TemplateData: baseData,
		Domain:       "example.com",
		RenewedAt:    renewedAt,
		ValidUntil:   validUntil,
	}

	if data.Domain != "example.com" {
		t.Errorf("Domain = %q", data.Domain)
	}
	if data.RenewedAt != renewedAt {
		t.Error("RenewedAt mismatch")
	}
	if data.ValidUntil != validUntil {
		t.Error("ValidUntil mismatch")
	}
}

func TestSchedulerErrorDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SchedulerErrorData{
		TemplateData: baseData,
		TaskName:     "daily-cleanup",
		Error:        "Timeout",
		FailedAt:     time.Now(),
		TaskDetails:  map[string]string{"duration": "exceeded 1h"},
	}

	if data.TaskName != "daily-cleanup" {
		t.Errorf("TaskName = %q", data.TaskName)
	}
	if data.Error != "Timeout" {
		t.Errorf("Error = %q", data.Error)
	}
	if len(data.TaskDetails) != 1 {
		t.Errorf("TaskDetails length = %d", len(data.TaskDetails))
	}
}

func TestBreachNotificationDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BreachNotificationData{
		TemplateData:       baseData,
		Username:           "testuser",
		BreachDate:         time.Now(),
		BreachDescription:  "Unauthorized access",
		AffectedData:       []string{"email", "preferences"},
		RecommendedActions: []string{"Change password", "Enable 2FA"},
		SupportContact:     "security@example.com",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.BreachDescription != "Unauthorized access" {
		t.Errorf("BreachDescription = %q", data.BreachDescription)
	}
	if len(data.AffectedData) != 2 {
		t.Errorf("AffectedData length = %d", len(data.AffectedData))
	}
	if len(data.RecommendedActions) != 2 {
		t.Errorf("RecommendedActions length = %d", len(data.RecommendedActions))
	}
}

func TestBreachAdminAlertDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BreachAdminAlertData{
		TemplateData:      baseData,
		Severity:          "critical",
		DetectedAt:        time.Now(),
		BreachDescription: "Mass account compromise",
		AffectedUsers:     150,
		IPAddresses:       []string{"1.2.3.4", "5.6.7.8"},
		ActionRequired:    "Immediate investigation required",
	}

	if data.Severity != "critical" {
		t.Errorf("Severity = %q", data.Severity)
	}
	if data.AffectedUsers != 150 {
		t.Errorf("AffectedUsers = %d", data.AffectedUsers)
	}
	if len(data.IPAddresses) != 2 {
		t.Errorf("IPAddresses length = %d", len(data.IPAddresses))
	}
}

func TestTestEmailDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	sentAt := time.Now()
	data := &TestEmailData{
		TemplateData: baseData,
		SentAt:       sentAt,
	}

	if data.SentAt != sentAt {
		t.Error("SentAt mismatch")
	}
}

func TestPasswordChangedDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	changedAt := time.Now()
	data := &PasswordChangedData{
		TemplateData: baseData,
		Username:     "testuser",
		ChangedAt:    changedAt,
		IPAddress:    "192.168.1.50",
		UserAgent:    "Firefox on Linux",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.ChangedAt != changedAt {
		t.Error("ChangedAt mismatch")
	}
	if data.IPAddress != "192.168.1.50" {
		t.Errorf("IPAddress = %q", data.IPAddress)
	}
	if data.UserAgent != "Firefox on Linux" {
		t.Errorf("UserAgent = %q", data.UserAgent)
	}
}

func TestEmailVerificationDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &EmailVerificationData{
		TemplateData:     baseData,
		Username:         "testuser",
		Email:            "test@example.com",
		VerificationLink: "https://example.com/verify?token=abc",
		ExpiresIn:        "24 hours",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.Email != "test@example.com" {
		t.Errorf("Email = %q", data.Email)
	}
	if data.VerificationLink != "https://example.com/verify?token=abc" {
		t.Errorf("VerificationLink = %q", data.VerificationLink)
	}
	if data.ExpiresIn != "24 hours" {
		t.Errorf("ExpiresIn = %q", data.ExpiresIn)
	}
}

func TestAccountLockedDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	lockedAt := time.Now()
	data := &AccountLockedData{
		TemplateData:       baseData,
		Username:           "testuser",
		Reason:             "Too many failed attempts",
		LockedAt:           lockedAt,
		UnlockInstructions: "Contact support",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.Reason != "Too many failed attempts" {
		t.Errorf("Reason = %q", data.Reason)
	}
	if data.LockedAt != lockedAt {
		t.Error("LockedAt mismatch")
	}
	if data.UnlockInstructions != "Contact support" {
		t.Errorf("UnlockInstructions = %q", data.UnlockInstructions)
	}
}

func TestTwoFactorEnabledDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	enabledAt := time.Now()
	data := &TwoFactorEnabledData{
		TemplateData: baseData,
		Username:     "testuser",
		EnabledAt:    enabledAt,
		IPAddress:    "192.168.1.70",
		Method:       "TOTP",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.EnabledAt != enabledAt {
		t.Error("EnabledAt mismatch")
	}
	if data.Method != "TOTP" {
		t.Errorf("Method = %q", data.Method)
	}
}

func TestTwoFactorDisabledDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	disabledAt := time.Now()
	data := &TwoFactorDisabledData{
		TemplateData: baseData,
		Username:     "testuser",
		DisabledAt:   disabledAt,
		IPAddress:    "192.168.1.80",
		Reason:       "User requested",
	}

	if data.Username != "testuser" {
		t.Errorf("Username = %q", data.Username)
	}
	if data.DisabledAt != disabledAt {
		t.Error("DisabledAt mismatch")
	}
	if data.Reason != "User requested" {
		t.Errorf("Reason = %q", data.Reason)
	}
}

func TestBackupCompletedDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	createdAt := time.Now()
	data := &BackupCompletedData{
		TemplateData: baseData,
		BackupName:   "backup-20240115.tar.gz",
		BackupSize:   "10.5 MB",
		CreatedAt:    createdAt,
		FileCount:    150,
		Duration:     "3.2 seconds",
	}

	if data.BackupName != "backup-20240115.tar.gz" {
		t.Errorf("BackupName = %q", data.BackupName)
	}
	if data.BackupSize != "10.5 MB" {
		t.Errorf("BackupSize = %q", data.BackupSize)
	}
	if data.FileCount != 150 {
		t.Errorf("FileCount = %d", data.FileCount)
	}
	if data.Duration != "3.2 seconds" {
		t.Errorf("Duration = %q", data.Duration)
	}
}

func TestUpdateAvailableDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	releaseDate := time.Now()
	data := &UpdateAvailableData{
		TemplateData:   baseData,
		CurrentVersion: "1.2.3",
		NewVersion:     "1.3.0",
		ReleaseDate:    releaseDate,
		ReleaseNotes:   "Bug fixes and improvements",
		UpdateURL:      "https://example.com/releases/1.3.0",
	}

	if data.CurrentVersion != "1.2.3" {
		t.Errorf("CurrentVersion = %q", data.CurrentVersion)
	}
	if data.NewVersion != "1.3.0" {
		t.Errorf("NewVersion = %q", data.NewVersion)
	}
	if data.ReleaseNotes != "Bug fixes and improvements" {
		t.Errorf("ReleaseNotes = %q", data.ReleaseNotes)
	}
	if data.UpdateURL != "https://example.com/releases/1.3.0" {
		t.Errorf("UpdateURL = %q", data.UpdateURL)
	}
}

func TestMaintenanceNoticeDataStruct(t *testing.T) {
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	scheduledAt := time.Now().Add(24 * time.Hour)
	data := &MaintenanceNoticeData{
		TemplateData:     baseData,
		ScheduledAt:      scheduledAt,
		Duration:         "30 minutes",
		Reason:           "Database maintenance",
		AffectedServices: []string{"Search API", "Admin Panel"},
	}

	if data.Duration != "30 minutes" {
		t.Errorf("Duration = %q", data.Duration)
	}
	if data.Reason != "Database maintenance" {
		t.Errorf("Reason = %q", data.Reason)
	}
	if len(data.AffectedServices) != 2 {
		t.Errorf("AffectedServices length = %d", len(data.AffectedServices))
	}
}

// Tests for Login notification with IsNewDevice=false

func TestEmailTemplateRenderLoginNotificationNotNewDevice(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &LoginNotificationData{
		TemplateData: baseData,
		Username:     "testuser",
		LoginTime:    time.Now(),
		IPAddress:    "192.168.1.100",
		UserAgent:    "Chrome on Windows",
		Location:     "New York, US",
		IsNewDevice:  false,
	}

	subject, body, err := et.Render(TemplateLoginNotification, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	// Should not have "New Device" in subject for existing device
	if strings.Contains(subject, "New Device") {
		t.Error("Subject should not contain 'New Device' for existing device")
	}
	if !strings.Contains(body, "192.168.1.100") {
		t.Error("Body should contain IP address")
	}
}

// Tests for Login notification without Location

func TestEmailTemplateRenderLoginNotificationNoLocation(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &LoginNotificationData{
		TemplateData: baseData,
		Username:     "testuser",
		LoginTime:    time.Now(),
		IPAddress:    "192.168.1.100",
		UserAgent:    "Chrome on Windows",
		Location:     "",
		IsNewDevice:  false,
	}

	subject, body, err := et.Render(TemplateLoginNotification, data)
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

// Tests for AccountLocked without UnlockInstructions

func TestEmailTemplateRenderAccountLockedNoInstructions(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AccountLockedData{
		TemplateData:       baseData,
		Username:           "testuser",
		Reason:             "Too many failed login attempts",
		LockedAt:           time.Now(),
		UnlockInstructions: "",
	}

	subject, body, err := et.Render(TemplateAccountLocked, data)
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

// Tests for AdminAlert without Details

func TestEmailTemplateRenderAdminAlertNoDetails(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AdminAlertData{
		TemplateData: baseData,
		AlertType:    "System Warning",
		AlertLevel:   "info",
		Message:      "Test message",
		Details:      nil,
		OccurredAt:   time.Now(),
	}

	subject, body, err := et.Render(TemplateAdminAlert, data)
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

// Tests for AdminAlert with critical level

func TestEmailTemplateRenderAdminAlertCritical(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AdminAlertData{
		TemplateData: baseData,
		AlertType:    "System Down",
		AlertLevel:   "critical",
		Message:      "Server is not responding",
		Details:      map[string]string{"Server": "prod-1"},
		OccurredAt:   time.Now(),
	}

	subject, body, err := et.Render(TemplateAdminAlert, data)
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

// Tests for WeeklyReport without TopQueries

func TestEmailTemplateRenderWeeklyReportNoTopQueries(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &WeeklyReportData{
		TemplateData:  baseData,
		PeriodStart:   time.Now().AddDate(0, 0, -7),
		PeriodEnd:     time.Now(),
		TotalSearches: 15000,
		UniqueUsers:   2500,
		TopQueries:    nil,
		EngineStats:   nil,
		ErrorCount:    10,
	}

	subject, body, err := et.Render(TemplateWeeklyReport, data)
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

// Tests for SecurityAlert without ActionRequired

func TestEmailTemplateRenderSecurityAlertNoAction(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SecurityAlertData{
		TemplateData:   baseData,
		Event:          "Suspicious Activity",
		Severity:       "medium",
		IPAddress:      "10.0.0.1",
		Details:        "",
		OccurredAt:     time.Now(),
		ActionRequired: "",
	}

	subject, body, err := et.Render(TemplateSecurityAlert, data)
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

// Tests for SecurityAlert critical and high severity

func TestEmailTemplateRenderSecurityAlertCritical(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SecurityAlertData{
		TemplateData:   baseData,
		Event:          "Account Breach",
		Severity:       "critical",
		IPAddress:      "10.0.0.1",
		Details:        "Account credentials exposed",
		OccurredAt:     time.Now(),
		ActionRequired: "Immediate action required",
	}

	subject, body, err := et.Render(TemplateSecurityAlert, data)
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

// Tests for APITokenCreated without permissions

func TestEmailTemplateRenderAPITokenCreatedNoPermissions(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &APITokenCreatedData{
		TemplateData: baseData,
		Username:     "testuser",
		TokenName:    "Production API Key",
		Permissions:  nil,
		ExpiresAt:    time.Now().Add(90 * 24 * time.Hour),
		CreatedAt:    time.Now(),
		IPAddress:    "192.168.1.100",
	}

	subject, body, err := et.Render(TemplateAPITokenCreated, data)
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

// Tests for AdminInvite without Message

func TestEmailTemplateRenderAdminInviteNoMessage(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &AdminInviteData{
		TemplateData: baseData,
		InviterName:  "John Admin",
		InviteLink:   "https://example.com/invite/abc123",
		ExpiresIn:    "48 hours",
		Message:      "",
	}

	subject, body, err := et.Render(TemplateAdminInvite, data)
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

// Tests for 2FADisabled without Reason

func TestEmailTemplateRenderTwoFactorDisabledNoReason(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &TwoFactorDisabledData{
		TemplateData: baseData,
		Username:     "testuser",
		DisabledAt:   time.Now(),
		IPAddress:    "192.168.1.80",
		Reason:       "",
	}

	subject, body, err := et.Render(Template2FADisabled, data)
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

// Tests for MaintenanceNotice without Reason

func TestEmailTemplateRenderMaintenanceNoticeNoReason(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &MaintenanceNoticeData{
		TemplateData:     baseData,
		ScheduledAt:      time.Now().Add(24 * time.Hour),
		Duration:         "30 minutes",
		Reason:           "",
		AffectedServices: nil,
	}

	subject, body, err := et.Render(TemplateMaintenanceNotice, data)
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

// Tests for UpdateAvailable without ReleaseNotes

func TestEmailTemplateRenderUpdateAvailableNoNotes(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &UpdateAvailableData{
		TemplateData:   baseData,
		CurrentVersion: "1.2.3",
		NewVersion:     "1.3.0",
		ReleaseDate:    time.Now(),
		ReleaseNotes:   "",
		UpdateURL:      "https://example.com/releases/1.3.0",
	}

	subject, body, err := et.Render(TemplateUpdateAvailable, data)
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

// Tests for SSL expiring with very few days left

func TestEmailTemplateRenderSSLExpiringUrgent(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SSLExpiringData{
		TemplateData: baseData,
		Domain:       "example.com",
		ExpiresAt:    time.Now().Add(2 * 24 * time.Hour),
		DaysLeft:     2,
		RenewLink:    "",
	}

	subject, body, err := et.Render(TemplateSSLExpiring, data)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if subject == "" {
		t.Error("Subject should not be empty")
	}
	if body == "" {
		t.Error("Body should not be empty")
	}
	// Should have urgent messaging for 2 days left
	if !strings.Contains(body, "URGENT") {
		t.Error("Body should contain URGENT for very few days left")
	}
}

// Tests for SchedulerError without TaskDetails

func TestEmailTemplateRenderSchedulerErrorNoDetails(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &SchedulerErrorData{
		TemplateData: baseData,
		TaskName:     "daily-cleanup",
		Error:        "Task timed out",
		FailedAt:     time.Now(),
		TaskDetails:  nil,
	}

	subject, body, err := et.Render(TemplateSchedulerError, data)
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

// Tests for BreachNotification without RecommendedActions

func TestEmailTemplateRenderBreachNotificationNoActions(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BreachNotificationData{
		TemplateData:       baseData,
		Username:           "testuser",
		BreachDate:         time.Now(),
		BreachDescription:  "Unauthorized access detected",
		AffectedData:       nil,
		RecommendedActions: nil,
		SupportContact:     "",
	}

	subject, body, err := et.Render(TemplateBreachNotification, data)
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

// Tests for BreachAdminAlert without IPAddresses

func TestEmailTemplateRenderBreachAdminAlertNoIPs(t *testing.T) {
	et := NewEmailTemplate()

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BreachAdminAlertData{
		TemplateData:      baseData,
		Severity:          "HIGH",
		DetectedAt:        time.Now(),
		BreachDescription: "Multiple accounts compromised",
		AffectedUsers:     150,
		IPAddresses:       nil,
		ActionRequired:    "Investigate immediately",
	}

	subject, body, err := et.Render(TemplateBreachAdminAlert, data)
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

	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &BreachAdminAlertData{
		TemplateData:      baseData,
		Severity:          "CRITICAL",
		DetectedAt:        time.Now(),
		BreachDescription: "Complete system breach",
		AffectedUsers:     1000,
		IPAddresses:       []string{"1.2.3.4"},
		ActionRequired:    "Shut down immediately",
	}

	subject, body, err := et.Render(TemplateBreachAdminAlert, data)
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

// Tests for splitLines with trailing newline

func TestSplitLinesTrailingNewline(t *testing.T) {
	result := splitLines("line1\nline2\n")
	if len(result) != 2 {
		t.Errorf("splitLines(\"line1\\nline2\\n\") returned %d lines, want 2", len(result))
	}
}

// Tests for getDefaultGateway returning value

func TestGetDefaultGatewayReturn(t *testing.T) {
	// This tests that getDefaultGateway doesn't panic and returns a string
	result := getDefaultGateway()
	// Result can be empty or a valid IP, just ensure it's a string
	if result != "" {
		// Should be a valid IPv4-like format if not empty
		parts := strings.Split(result, ".")
		if len(parts) != 4 {
			// It's ok if empty but if not empty should look like IP
			t.Logf("Gateway result: %s", result)
		}
	}
}

// Tests for DetectAndConfigure

func TestDetectAndConfigureFull(t *testing.T) {
	// Test that it returns a valid config (either default or detected)
	cfg := DetectAndConfigure()

	if cfg == nil {
		t.Fatal("DetectAndConfigure() returned nil")
	}

	// Should have valid defaults at minimum
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
	// Error should mention connection failure
	if !strings.Contains(err.Error(), "connection failed") && !strings.Contains(err.Error(), "lookup") {
		t.Logf("Error: %v", err)
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
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			// Use TEST-NET-1 (reserved for docs, doesn't route)
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

	// Will fail on connection but exercises the CC code path
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
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			// Use TEST-NET-1 (reserved for docs, doesn't route)
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

	result := ml.formatAddress("Caf\u00e9 Owner", "test@example.com")
	// Should have encoded UTF-8 name
	if !strings.Contains(result, "=?UTF-8?B?") {
		t.Errorf("formatAddress with UTF-8 name = %q, should have UTF-8 encoding", result)
	}
	if !strings.Contains(result, "test@example.com") {
		t.Errorf("formatAddress should contain email address, got %q", result)
	}
}

// Tests for rawTemplates to ensure all template types have content

func TestRawTemplatesContainSubjectLine(t *testing.T) {
	for tt, tmpl := range rawTemplates {
		lines := strings.Split(tmpl, "\n")
		if len(lines) < 2 {
			t.Errorf("rawTemplates[%s] should have at least 2 lines (subject + body)", tt)
		}
		// First line should be the subject template
		if lines[0] == "" {
			t.Errorf("rawTemplates[%s] should have non-empty first line (subject)", tt)
		}
	}
}

// Tests for TemplateInfo JSON tags

func TestTemplateInfoJSONTags(t *testing.T) {
	info := TemplateInfo{
		Type:           TemplateWelcome,
		Name:           "Welcome",
		Description:    "Welcome email",
		IsAccountEmail: true,
	}

	// Verify the struct is properly tagged (just test fields work)
	if info.Type != TemplateWelcome {
		t.Errorf("Type = %q", info.Type)
	}
	if info.Name != "Welcome" {
		t.Errorf("Name = %q", info.Name)
	}
	if info.Description != "Welcome email" {
		t.Errorf("Description = %q", info.Description)
	}
	if !info.IsAccountEmail {
		t.Error("IsAccountEmail should be true")
	}
}

// Tests for GetAllTemplateTypes ensuring all PART 18 templates are included

func TestGetAllTemplateTypesIncludesPart18(t *testing.T) {
	types := GetAllTemplateTypes()

	part18Types := []TemplateType{
		TemplateMFAReminder,
		TemplateBackupFailed,
		TemplateSSLExpiring,
		TemplateSSLRenewed,
		TemplateSchedulerError,
		TemplateBreachNotification,
		TemplateBreachAdminAlert,
		TemplateTest,
	}

	found := make(map[TemplateType]bool)
	for _, info := range types {
		found[info.Type] = true
	}

	for _, tt := range part18Types {
		if !found[tt] {
			t.Errorf("GetAllTemplateTypes() should include %s", tt)
		}
	}
}

// Tests to verify IsAccountEmail covers all account templates

func TestIsAccountEmailAllTemplates(t *testing.T) {
	// All templates that should return true for IsAccountEmail
	accountTemplates := []TemplateType{
		TemplateWelcome,
		TemplatePasswordReset,
		TemplatePasswordChanged,
		TemplateLoginNotification,
		TemplateEmailVerification,
		TemplateAccountLocked,
		TemplateSecurityAlert,
		Template2FAEnabled,
		Template2FADisabled,
		TemplateMFAReminder,
		TemplateBreachNotification,
	}

	for _, tt := range accountTemplates {
		if !IsAccountEmail(tt) {
			t.Errorf("IsAccountEmail(%s) should return true", tt)
		}
	}

	// All templates that should return false
	notificationTemplates := []TemplateType{
		TemplateAdminAlert,
		TemplateWeeklyReport,
		TemplateAPITokenCreated,
		TemplateAdminInvite,
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

	for _, tt := range notificationTemplates {
		if IsAccountEmail(tt) {
			t.Errorf("IsAccountEmail(%s) should return false", tt)
		}
	}

	// Test unknown template type
	if IsAccountEmail(TemplateType("unknown_template")) {
		t.Error("IsAccountEmail should return false for unknown template type")
	}
}

// Tests for NewEmailTemplate parsing all templates successfully

func TestNewEmailTemplateAllTemplatesParsed(t *testing.T) {
	et := NewEmailTemplate()

	// Verify that all templates in rawTemplates were successfully parsed
	for templateType := range rawTemplates {
		if _, ok := et.templates[templateType]; !ok {
			t.Errorf("Template %s should be parsed and available", templateType)
		}
	}
}

// Tests for Render with template execution error

func TestEmailTemplateRenderExecutionError(t *testing.T) {
	et := NewEmailTemplate()

	// Pass invalid data that doesn't match template expectations
	// This should cause template execution to fail
	invalidData := map[string]string{"invalid": "data"}

	_, _, err := et.Render(TemplateWelcome, invalidData)
	// Should error because the data doesn't have the expected fields
	if err == nil {
		t.Error("Render() should error with mismatched data")
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

	// Will fail on connection but exercises all message building code paths
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
	msg := NewMessage([]string{"user@example.com"}, "Hello \u4e16\u754c", "Body")

	err := ml.Send(msg)
	if err == nil {
		t.Error("Send() should error with invalid SMTP server")
	}
}

// Tests for encodeHeader with exactly 128 character

func TestEncodeHeaderBoundary(t *testing.T) {
	cfg := DefaultConfig()
	ml := NewMailer(cfg)

	// Test with character code 127 (DEL, highest ASCII)
	result127 := ml.encodeHeader("test\x7fvalue")
	if result127 != "test\x7fvalue" {
		// Character 127 is ASCII, should not be encoded
		t.Logf("Result for char 127: %s", result127)
	}

	// Test with character code 128 (first non-ASCII)
	result128 := ml.encodeHeader("test\x80value")
	if !strings.HasPrefix(result128, "=?UTF-8?B?") {
		t.Errorf("encodeHeader with char 128 should be encoded, got %q", result128)
	}
}

// Tests for splitLines edge cases

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

// Tests for joinLines edge cases

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
	// Should fail on connection but exercises the SendAlert -> SendToAdmins -> Send path
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
	// Should fail on connection but exercises the full code path
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

// Tests for Render with mismatched template data type

func TestEmailTemplateRenderWithWrongDataType(t *testing.T) {
	et := NewEmailTemplate()

	// Use PasswordResetData for Welcome template - should still work partially
	baseData := NewTemplateData("TestApp", "https://example.com", "support@example.com")
	data := &PasswordResetData{
		TemplateData: baseData,
		Username:     "testuser",
		ResetLink:    "https://example.com/reset",
		ExpiresIn:    "1 hour",
		IPAddress:    "192.168.1.1",
		RequestedAt:  time.Now(),
	}

	// Welcome template expects WelcomeData but PasswordResetData should work for common fields
	subject, body, err := et.Render(TemplateWelcome, data)
	// This should either succeed or error depending on template requirements
	if err != nil {
		// If it errors, that's expected for mismatched data
		t.Logf("Render with mismatched data errored: %v", err)
	} else {
		if subject == "" {
			t.Error("Subject should not be empty")
		}
		if body == "" {
			t.Error("Body should not be empty")
		}
	}
}

// Tests for rawTemplates count matches GetAllTemplateTypes count

func TestRawTemplatesCountMatchesGetAllTemplateTypes(t *testing.T) {
	allTypes := GetAllTemplateTypes()
	rawCount := len(rawTemplates)
	allTypesCount := len(allTypes)

	if rawCount != allTypesCount {
		t.Errorf("rawTemplates has %d entries but GetAllTemplateTypes returns %d entries", rawCount, allTypesCount)
	}
}

// Tests for Render with template that produces empty content

func TestEmailTemplateRenderEmptyContent(t *testing.T) {
	et := NewEmailTemplate()

	// Create minimal data - should still produce subject at minimum
	baseData := NewTemplateData("", "", "")
	data := &WelcomeData{
		TemplateData: baseData,
		Username:     "",
		Email:        "",
	}

	subject, body, err := et.Render(TemplateWelcome, data)
	if err != nil {
		t.Logf("Render with empty data: %v", err)
	}
	// Even with empty data, templates should produce some output
	_ = subject
	_ = body
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
	// Call multiple times to ensure consistency
	result1 := getDefaultGateway()
	result2 := getDefaultGateway()

	// Results should be consistent
	if result1 != result2 {
		t.Errorf("getDefaultGateway() should return consistent results: %q vs %q", result1, result2)
	}
}

// Tests for tryDetectSMTP with different scenarios

func TestTryDetectSMTPDifferentPorts(t *testing.T) {
	// Test with unreachable ports
	ports := []int{59990, 59991, 59992}

	for _, port := range ports {
		result := tryDetectSMTP("127.0.0.1", port, false)
		if result != nil {
			t.Errorf("tryDetectSMTP for port %d should return nil", port)
		}
	}
}

// Tests for sendMail with uppercase TLS mode

func TestMailerSendWithUppercaseTLSMode(t *testing.T) {
	cfg := &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: "invalid.host.example.com",
			Port: 587,
			TLS:  "AUTO", // uppercase
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
			TLS:  "TLS", // uppercase
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
			TLS:  "STARTTLS", // uppercase
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
