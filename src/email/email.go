package email

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Config holds email configuration
type Config struct {
	Enabled     bool   `yaml:"enabled"`
	SMTPHost    string `yaml:"smtp_host"`
	SMTPPort    int    `yaml:"smtp_port"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	FromAddress string `yaml:"from_address"`
	FromName    string `yaml:"from_name"`
	UseTLS      bool   `yaml:"use_tls"`
	UseSTARTTLS bool   `yaml:"use_starttls"`
	AdminEmails []string `yaml:"admin_emails"`
}

// DefaultConfig returns default email configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:     false,
		SMTPHost:    "localhost",
		SMTPPort:    25,
		FromAddress: "noreply@scour.li",
		FromName:    "Scour Search",
		UseTLS:      false,
		UseSTARTTLS: true,
		AdminEmails: []string{},
	}
}

// Mailer handles email sending
type Mailer struct {
	config *Config
}

// NewMailer creates a new mailer
func NewMailer(cfg *Config) *Mailer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Mailer{config: cfg}
}

// Message represents an email message
type Message struct {
	To          []string
	CC          []string
	BCC         []string
	Subject     string
	Body        string
	HTMLBody    string
	ContentType string
	Headers     map[string]string
}

// NewMessage creates a new email message
func NewMessage(to []string, subject, body string) *Message {
	return &Message{
		To:          to,
		Subject:     subject,
		Body:        body,
		ContentType: "text/plain",
		Headers:     make(map[string]string),
	}
}

// SetHTML sets the HTML body
func (m *Message) SetHTML(html string) {
	m.HTMLBody = html
	m.ContentType = "text/html"
}

// Send sends an email message
func (ml *Mailer) Send(msg *Message) error {
	if !ml.config.Enabled {
		return fmt.Errorf("email is not enabled")
	}

	if len(msg.To) == 0 {
		return fmt.Errorf("no recipients specified")
	}

	// Build email headers
	headers := make(map[string]string)
	headers["From"] = ml.formatAddress(ml.config.FromName, ml.config.FromAddress)
	headers["To"] = strings.Join(msg.To, ", ")
	headers["Subject"] = ml.encodeHeader(msg.Subject)
	headers["Date"] = time.Now().Format(time.RFC1123Z)
	headers["MIME-Version"] = "1.0"

	if len(msg.CC) > 0 {
		headers["Cc"] = strings.Join(msg.CC, ", ")
	}

	// Merge custom headers
	for k, v := range msg.Headers {
		headers[k] = v
	}

	// Build message body
	var body string
	if msg.HTMLBody != "" {
		headers["Content-Type"] = "text/html; charset=UTF-8"
		body = msg.HTMLBody
	} else {
		headers["Content-Type"] = "text/plain; charset=UTF-8"
		body = msg.Body
	}
	headers["Content-Transfer-Encoding"] = "base64"

	// Build raw message
	var rawMsg strings.Builder
	for k, v := range headers {
		rawMsg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	rawMsg.WriteString("\r\n")
	rawMsg.WriteString(base64.StdEncoding.EncodeToString([]byte(body)))

	// Get all recipients
	recipients := append([]string{}, msg.To...)
	recipients = append(recipients, msg.CC...)
	recipients = append(recipients, msg.BCC...)

	// Send email
	return ml.sendMail(recipients, []byte(rawMsg.String()))
}

// sendMail sends the raw email
func (ml *Mailer) sendMail(recipients []string, message []byte) error {
	addr := fmt.Sprintf("%s:%d", ml.config.SMTPHost, ml.config.SMTPPort)

	var conn net.Conn
	var err error

	if ml.config.UseTLS {
		// Direct TLS connection
		tlsConfig := &tls.Config{
			ServerName: ml.config.SMTPHost,
		}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 30*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, ml.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// STARTTLS if enabled and not already using TLS
	if ml.config.UseSTARTTLS && !ml.config.UseTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName: ml.config.SMTPHost,
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("failed to start TLS: %w", err)
			}
		}
	}

	// Authenticate if credentials provided
	if ml.config.Username != "" && ml.config.Password != "" {
		auth := smtp.PlainAuth("", ml.config.Username, ml.config.Password, ml.config.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(ml.config.FromAddress); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	// Set recipients
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("failed to set recipient %s: %w", rcpt, err)
		}
	}

	// Send message
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to get data writer: %w", err)
	}

	if _, err := writer.Write(message); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close data writer: %w", err)
	}

	return client.Quit()
}

// formatAddress formats an email address with name
func (ml *Mailer) formatAddress(name, address string) string {
	if name == "" {
		return address
	}
	return fmt.Sprintf("%s <%s>", ml.encodeHeader(name), address)
}

// encodeHeader encodes a header value for UTF-8
func (ml *Mailer) encodeHeader(value string) string {
	// Check if encoding is needed
	needsEncoding := false
	for _, r := range value {
		if r > 127 {
			needsEncoding = true
			break
		}
	}

	if !needsEncoding {
		return value
	}

	// Use Base64 encoding for UTF-8
	encoded := base64.StdEncoding.EncodeToString([]byte(value))
	return fmt.Sprintf("=?UTF-8?B?%s?=", encoded)
}

// IsEnabled returns whether email is enabled
func (ml *Mailer) IsEnabled() bool {
	return ml.config.Enabled
}

// SendToAdmins sends an email to all configured admin addresses
func (ml *Mailer) SendToAdmins(subject, body string) error {
	if len(ml.config.AdminEmails) == 0 {
		return fmt.Errorf("no admin emails configured")
	}

	msg := NewMessage(ml.config.AdminEmails, subject, body)
	return ml.Send(msg)
}

// SendAlert sends an alert email to admins
func (ml *Mailer) SendAlert(alertType, message string) error {
	subject := fmt.Sprintf("[Scour Alert] %s", alertType)
	body := fmt.Sprintf("Alert Type: %s\nTime: %s\n\nMessage:\n%s",
		alertType,
		time.Now().Format(time.RFC3339),
		message,
	)
	return ml.SendToAdmins(subject, body)
}

// SendSecurityAlert sends a security alert email
func (ml *Mailer) SendSecurityAlert(event, ip, details string) error {
	subject := fmt.Sprintf("[Scour Security] %s from %s", event, ip)
	body := fmt.Sprintf("Security Event: %s\nIP Address: %s\nTime: %s\n\nDetails:\n%s",
		event,
		ip,
		time.Now().Format(time.RFC3339),
		details,
	)
	return ml.SendToAdmins(subject, body)
}

// TestConnection tests the SMTP connection
func (ml *Mailer) TestConnection() error {
	if !ml.config.Enabled {
		return fmt.Errorf("email is not enabled")
	}

	addr := fmt.Sprintf("%s:%d", ml.config.SMTPHost, ml.config.SMTPPort)

	var conn net.Conn
	var err error

	if ml.config.UseTLS {
		tlsConfig := &tls.Config{
			ServerName: ml.config.SMTPHost,
		}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}

	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, ml.config.SMTPHost)
	if err != nil {
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer client.Close()

	// Test STARTTLS if needed
	if ml.config.UseSTARTTLS && !ml.config.UseTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName: ml.config.SMTPHost,
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		}
	}

	// Test authentication if credentials provided
	if ml.config.Username != "" && ml.config.Password != "" {
		auth := smtp.PlainAuth("", ml.config.Username, ml.config.Password, ml.config.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	return client.Quit()
}
