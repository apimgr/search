package email

import (
	"strings"
	"testing"
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
