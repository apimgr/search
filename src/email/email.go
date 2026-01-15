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
// Per AI.md PART 18: Nested SMTP and From blocks
type Config struct {
	Enabled     bool   // Auto-set based on SMTP availability
	SMTP        SMTPConfig
	From        FromConfig
	AdminEmails []string
}

// SMTPConfig represents SMTP server configuration
// Per AI.md PART 18: SMTP configuration
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	TLS      string // auto, starttls, tls, none
}

// FromConfig represents the from address configuration
// Per AI.md PART 18: From name and email defaults
type FromConfig struct {
	Name  string
	Email string
}

// DefaultConfig returns default email configuration
// Per AI.md PART 18: Sane defaults
func DefaultConfig() *Config {
	return &Config{
		Enabled: false,
		SMTP: SMTPConfig{
			Host: "",
			Port: 587,
			TLS:  "auto",
		},
		From: FromConfig{
			Name:  "",  // Default: app title (set at runtime)
			Email: "",  // Default: no-reply@{fqdn} (set at runtime)
		},
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
	headers["From"] = ml.formatAddress(ml.config.From.Name, ml.config.From.Email)
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
// Per AI.md PART 18: TLS mode handling (auto, starttls, tls, none)
func (ml *Mailer) sendMail(recipients []string, message []byte) error {
	addr := net.JoinHostPort(ml.config.SMTP.Host, fmt.Sprintf("%d", ml.config.SMTP.Port))

	var conn net.Conn
	var err error

	// Per AI.md PART 18: TLS mode handling
	tlsMode := strings.ToLower(ml.config.SMTP.TLS)
	useTLS := tlsMode == "tls"
	useSTARTTLS := tlsMode == "starttls" || tlsMode == "auto"

	if useTLS {
		// Direct TLS connection
		tlsConfig := &tls.Config{
			ServerName: ml.config.SMTP.Host,
		}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 30*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, ml.config.SMTP.Host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// STARTTLS if enabled and not already using TLS
	if useSTARTTLS && !useTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName: ml.config.SMTP.Host,
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("failed to start TLS: %w", err)
			}
		}
	}

	// Authenticate if credentials provided
	if ml.config.SMTP.Username != "" && ml.config.SMTP.Password != "" {
		auth := smtp.PlainAuth("", ml.config.SMTP.Username, ml.config.SMTP.Password, ml.config.SMTP.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	// Set sender
	if err := client.Mail(ml.config.From.Email); err != nil {
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
// Per AI.md PART 18: Connection test on startup
func (ml *Mailer) TestConnection() error {
	if !ml.config.Enabled {
		return fmt.Errorf("email is not enabled")
	}

	addr := net.JoinHostPort(ml.config.SMTP.Host, fmt.Sprintf("%d", ml.config.SMTP.Port))

	// Per AI.md PART 18: TLS mode handling
	tlsMode := strings.ToLower(ml.config.SMTP.TLS)
	useTLS := tlsMode == "tls"
	useSTARTTLS := tlsMode == "starttls" || tlsMode == "auto"

	var conn net.Conn
	var err error

	if useTLS {
		tlsConfig := &tls.Config{
			ServerName: ml.config.SMTP.Host,
		}
		conn, err = tls.Dial("tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 10*time.Second)
	}

	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, ml.config.SMTP.Host)
	if err != nil {
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer client.Close()

	// Test STARTTLS if needed
	if useSTARTTLS && !useTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			tlsConfig := &tls.Config{
				ServerName: ml.config.SMTP.Host,
			}
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("STARTTLS failed: %w", err)
			}
		}
	}

	// Test authentication if credentials provided
	if ml.config.SMTP.Username != "" && ml.config.SMTP.Password != "" {
		auth := smtp.PlainAuth("", ml.config.SMTP.Username, ml.config.SMTP.Password, ml.config.SMTP.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	return client.Quit()
}

// DetectedSMTP represents a detected SMTP server
type DetectedSMTP struct {
	Host        string
	Port        int
	TLS         bool
	STARTTLS    bool
	AuthRequired bool
}

// DetectSMTP auto-detects SMTP servers on the local system
// Per AI.md PART 18: SMTP auto-detection on first run
// Check order: localhost, 127.0.0.1, Docker host (172.17.0.1), Gateway IP
// Ports: 25, 587, 465
func DetectSMTP() *DetectedSMTP {
	// Per AI.md PART 18: Hosts to check in order
	hosts := []string{
		"localhost",
		"127.0.0.1",
		"172.17.0.1", // Docker host
	}

	// Try to get gateway IP
	if gateway := getDefaultGateway(); gateway != "" {
		hosts = append(hosts, gateway)
	}

	// Per AI.md PART 18: Ports to check (25, 587, 465)
	ports := []struct {
		port     int
		tls      bool
		starttls bool
	}{
		{25, false, true},   // Standard SMTP
		{587, false, true},  // Submission with STARTTLS
		{465, true, false},  // SMTPS (TLS)
	}

	for _, host := range hosts {
		for _, p := range ports {
			if detected := tryDetectSMTP(host, p.port, p.tls); detected != nil {
				detected.STARTTLS = p.starttls
				return detected
			}
		}
	}

	return nil
}

// getDefaultGateway attempts to get the default gateway IP
func getDefaultGateway() string {
	// Get default route interface
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() {
				continue
			}
			if ipNet.IP.To4() != nil {
				// Return first non-loopback IPv4's likely gateway
				// Common gateway patterns: x.x.x.1 or x.x.x.254
				ip := ipNet.IP.To4()
				gateway := net.IPv4(ip[0], ip[1], ip[2], 1)
				return gateway.String()
			}
		}
	}
	return ""
}

// tryDetectSMTP attempts to connect to an SMTP server
func tryDetectSMTP(host string, port int, useTLS bool) *DetectedSMTP {
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	var conn net.Conn
	var err error

	if useTLS {
		tlsConfig := &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true,
		}
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, tlsConfig)
	} else {
		conn, err = net.DialTimeout("tcp", addr, 2*time.Second)
	}

	if err != nil {
		return nil
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return nil
	}
	defer client.Close()

	// Check if STARTTLS is supported
	hasSTARTTLS := false
	if !useTLS {
		hasSTARTTLS, _ = client.Extension("STARTTLS")
	}

	// Check if AUTH is required
	hasAuth, _ := client.Extension("AUTH")

	client.Quit()

	return &DetectedSMTP{
		Host:         host,
		Port:         port,
		TLS:          useTLS,
		STARTTLS:     hasSTARTTLS,
		AuthRequired: hasAuth,
	}
}

// DetectAndConfigure detects SMTP and returns a configured Config
// Per AI.md PART 18: SMTP auto-detection on first run
func DetectAndConfigure() *Config {
	detected := DetectSMTP()
	if detected == nil {
		return DefaultConfig()
	}

	// Determine TLS mode based on detection
	tlsMode := "auto"
	if detected.TLS {
		tlsMode = "tls"
	} else if detected.STARTTLS {
		tlsMode = "starttls"
	}

	return &Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host: detected.Host,
			Port: detected.Port,
			TLS:  tlsMode,
		},
		// Per AI.md PART 18: From defaults (set at runtime with app_name/fqdn)
		From:        FromConfig{},
		AdminEmails: []string{},
	}
}

// IsLocalSMTPAvailable checks if a local SMTP server is available
func IsLocalSMTPAvailable() bool {
	return DetectSMTP() != nil
}
