package email

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
)

// Testable variables for dependency injection in tests
var (
	// netDialTimeout allows tests to mock net.DialTimeout
	netDialTimeout = net.DialTimeout
	// tlsDial allows tests to mock tls.Dial
	tlsDial = tls.Dial
	// smtpNewClient allows tests to mock smtp.NewClient
	smtpNewClient = smtp.NewClient
	// netInterfaces allows tests to mock net.Interfaces
	netInterfaces = net.Interfaces
	// tlsDialWithDialer allows tests to mock tls.DialWithDialer
	tlsDialWithDialer = tls.DialWithDialer
)

// Config holds email configuration
// Per AI.md PART 18: Nested SMTP and From blocks
type Config struct {
	// Auto-set based on SMTP availability
	Enabled     bool
	SMTP        SMTPConfig
	From        FromConfig
	AdminEmails []string
	// AppTitle is the configured application title used in email subjects
	AppTitle string
	// FQDN is the server fully-qualified domain name used in email templates
	FQDN string
	// AppURL is the full application URL used in email templates
	AppURL string
	// ReplyTo is the Reply-To address from server.notifications.email.reply_to.
	// Per AI.md PART 17 "Default Sender" the Reply-To header is omitted when
	// this is empty.
	ReplyTo string
	// OnionAddress is the Tor hidden service address (AI.md PART 31.1), used
	// for the {onion_url}/{onion_address} global template variables. Empty
	// when Tor is not running.
	OnionAddress string
	// I2PAddress is the I2P eepsite address (AI.md PART 31.2), used for the
	// {i2p_url}/{i2p_address} global template variables. Empty when I2P is
	// disabled, which is the default.
	I2PAddress string
	// Lang is the BCP 47 language tag used to translate email body/subject
	// strings before rendering (see AI.md PART 30 "Email Template Translation").
	// Defaults to "en" when empty.
	Lang string
	// Events holds the per-event email enable/disable switches
	// (server.notifications.email.events per AI.md PART 17). A false value
	// means the corresponding Send* call is a silent no-op.
	Events config.EmailEventsConfig
}

// SMTPConfig represents SMTP server configuration
// Per AI.md PART 18: SMTP configuration
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// auto, starttls, tls, none
	TLS string
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
			// Default: app title (set at runtime)
			Name: "",
			// Default: no-reply@{fqdn} (set at runtime)
			Email: "",
		},
		AdminEmails: []string{},
	}
}

// Mailer handles email sending
type Mailer struct {
	config *Config
	// queueMu guards queue, which is nil until StartQueue is called.
	queueMu sync.RWMutex
	queue   *Queue
}

// NewMailer creates a new mailer
func NewMailer(cfg *Config) *Mailer {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	return &Mailer{config: cfg}
}

// NewMailerFromConfig builds a Mailer from the application's global
// server.yml config, resolving the same from-address/from-name/language
// defaults and admin recipient (AI.md PART 12 "Contact Configuration":
// server.contact.admin.email) used by every mailer construction site in
// this project (the running server, the `search email test` CLI
// subcommand, and the `search --update yes` self-update completion
// notification). Returns (nil, false) when SMTP is not configured, per
// AI.md PART 17: "Email-dependent features disabled until SMTP configured."
func NewMailerFromConfig(cfg *config.Config) (*Mailer, bool) {
	if cfg == nil || cfg.Server.Notifications.Email.SMTP.Host == "" {
		return nil, false
	}

	fromEmail := cfg.Server.Notifications.Email.From.Email
	if fromEmail == "" {
		fqdn := "localhost"
		if cfg.Server.BaseURL != "" {
			if u, err := url.Parse(cfg.Server.BaseURL); err == nil && u.Host != "" {
				fqdn = u.Host
			}
		}
		fromEmail = "no-reply@" + fqdn
	}
	fromName := cfg.Server.Notifications.Email.From.Name
	if fromName == "" {
		fromName = cfg.Server.Branding.Title
		if fromName == "" {
			fromName = "Search"
		}
	}
	emailLang := cfg.Server.I18n.DefaultLanguage
	if emailLang == "" {
		emailLang = "en"
	}

	var adminEmails []string
	if cfg.Server.Contact.Admin.Email != "" {
		adminEmails = []string{cfg.Server.Contact.Admin.Email}
	}

	return NewMailer(&Config{
		Enabled: true,
		SMTP: SMTPConfig{
			Host:     cfg.Server.Notifications.Email.SMTP.Host,
			Port:     cfg.Server.Notifications.Email.SMTP.Port,
			Username: cfg.Server.Notifications.Email.SMTP.Username,
			Password: cfg.Server.Notifications.Email.SMTP.Password,
			TLS:      cfg.Server.Notifications.Email.SMTP.TLS,
		},
		From: FromConfig{
			Name:  fromName,
			Email: fromEmail,
		},
		AppTitle:     fromName,
		FQDN:         cfg.Server.FQDN,
		AppURL:       cfg.Server.BaseURL,
		Lang:         emailLang,
		AdminEmails:  adminEmails,
		Events:       cfg.Server.Notifications.Email.Events,
		OnionAddress: cfg.Server.Tor.OnionAddress,
	}), true
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

// Send delivers an email message. When a delivery queue is running (see
// StartQueue) the message is handed to the queue, which retries with
// exponential backoff and logs every success and failure per the AI.md
// PART 17 compliance checklist. Without a running queue the message is
// delivered synchronously in a single attempt.
func (ml *Mailer) Send(msg *Message) error {
	if !ml.config.Enabled {
		return fmt.Errorf("email is not enabled")
	}

	if len(msg.To) == 0 {
		return fmt.Errorf("no recipients specified")
	}

	if q := ml.activeQueue(); q != nil {
		return q.enqueue(msg)
	}

	return ml.SendNow(msg)
}

// SendNow delivers an email message synchronously in a single attempt,
// bypassing the delivery queue. Used by the queue workers themselves and by
// operator-facing actions (the test email) that must report the SMTP result
// immediately.
func (ml *Mailer) SendNow(msg *Message) error {
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

	// Per AI.md PART 17 "Default Sender": Reply-To is included only when
	// server.notifications.email.reply_to is set, otherwise omitted.
	if ml.config.ReplyTo != "" {
		headers["Reply-To"] = ml.config.ReplyTo
	}

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
		fmt.Fprintf(&rawMsg, "%s: %s\r\n", k, v)
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
		conn, err = tlsDial("tcp", addr, tlsConfig)
	} else {
		conn, err = netDialTimeout("tcp", addr, 30*time.Second)
	}

	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer closeutil.Close(conn)

	client, err := smtpNewClient(conn, ml.config.SMTP.Host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Warn("failed to close SMTP client", "error", err)
		}
	}()

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

// CanSend reports whether notification email may be sent at all, implementing
// the AI.md PART 17 "SMTP Check Before Sending" predicate: email must be
// enabled AND an SMTP host must be configured. When it returns false the
// notification helpers degrade gracefully — they log nothing to SMTP, attempt
// nothing, and return nil rather than an error.
func (ml *Mailer) CanSend() bool {
	if ml == nil || ml.config == nil {
		return false
	}
	return ml.config.Enabled && ml.config.SMTP.Host != ""
}

// SendToAdmins sends an email to all configured admin addresses
func (ml *Mailer) SendToAdmins(subject, body string) error {
	if len(ml.config.AdminEmails) == 0 {
		return fmt.Errorf("no admin emails configured")
	}

	msg := NewMessage(ml.config.AdminEmails, subject, body)
	return ml.Send(msg)
}

// lang returns the configured notification language, defaulting to "en"
// when unset (see AI.md PART 30 "Email Template Translation").
func (ml *Mailer) lang() string {
	if ml.config.Lang == "" {
		return "en"
	}
	return ml.config.Lang
}

// baseVars returns the global template variables available in every email
// template (see AI.md PART 17 → Global Variables).
func (ml *Mailer) baseVars() map[string]string {
	appName := ml.config.AppTitle
	if appName == "" {
		appName = i18n.TDefault("common.app_name")
	}
	now := time.Now()
	lang := ml.lang()
	onionURL := ""
	if ml.config.OnionAddress != "" {
		onionURL = "http://" + ml.config.OnionAddress
	}
	i2pURL := ""
	if ml.config.I2PAddress != "" {
		i2pURL = "http://" + ml.config.I2PAddress
	}
	return map[string]string{
		"app_name":              appName,
		"app_url":               ml.config.AppURL,
		"fqdn":                  ml.config.FQDN,
		"onion_url":             onionURL,
		"onion_address":         ml.config.OnionAddress,
		"i2p_url":               i2pURL,
		"i2p_address":           ml.config.I2PAddress,
		"notification_reply_to": ml.config.ReplyTo,
		"timestamp":             now.Format("2006-01-02 15:04:05 UTC"),
		"year":                  fmt.Sprintf("%d", now.Year()),
		"label_from":            i18n.T(lang, "email.common.label_from"),
		"label_time":            i18n.T(lang, "email.common.label_time"),
	}
}

// SendSecurityAlert sends a security alert email.
// Per AI.md PART 30 "Email Template Translation": translated strings are
// resolved server-side via i18n.T before being passed as plain {variable}
// values to the {variable}-substitution template engine (AI.md PART 17).
func (ml *Mailer) SendSecurityAlert(event, ip, details string) error {
	if !ml.CanSend() || !ml.config.Events.SecurityAlert {
		return nil
	}
	lang := ml.lang()
	vars := ml.baseVars()
	vars["event"] = event
	vars["ip"] = ip
	vars["details"] = details
	vars["subject_prefix"] = i18n.T(lang, "email.security_alert.subject_prefix")
	vars["heading"] = i18n.T(lang, "email.security_alert.heading")
	vars["label_source_ip"] = i18n.T(lang, "email.security_alert.label_source_ip")
	vars["label_details"] = i18n.T(lang, "email.security_alert.label_details")
	subject, body, err := NewEmailTemplate().Render(TemplateSecurityAlert, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendUpdateAvailable notifies admins that a newer release is available.
// Per AI.md PART 22: the update_check task is notify-only; this email is the
// notification. Per AI.md PART 30 "Email Template Translation": translated
// strings are resolved server-side via i18n.T before being passed as plain
// {variable} values.
func (ml *Mailer) SendUpdateAvailable(currentVersion, newVersion, releaseDate, releaseNotes, updateURL string) error {
	if !ml.CanSend() || !ml.config.Events.UpdateAvailable {
		return nil
	}
	lang := ml.lang()
	vars := ml.baseVars()
	vars["current_version"] = currentVersion
	vars["new_version"] = newVersion
	vars["release_date"] = releaseDate
	vars["release_notes"] = releaseNotes
	vars["update_url"] = updateURL
	vars["subject_prefix"] = i18n.T(lang, "email.update_available.subject_prefix")
	vars["heading"] = i18n.T(lang, "email.update_available.heading")
	vars["intro"] = i18n.T(lang, "email.update_available.intro", vars["app_name"])
	vars["label_current_version"] = i18n.T(lang, "email.update_available.label_current_version")
	vars["label_new_version"] = i18n.T(lang, "email.update_available.label_new_version")
	vars["label_release_date"] = i18n.T(lang, "email.update_available.label_release_date")
	vars["label_release_notes"] = i18n.T(lang, "email.update_available.label_release_notes")
	vars["label_view_update"] = i18n.T(lang, "email.update_available.label_view_update")
	subject, body, err := NewEmailTemplate().Render(TemplateUpdateAvailable, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendUpdateInstalled notifies admins that a self-update completed. Per
// AI.md PART 17 "Default Templates" → update_installed ("Includes previous
// version and new version") and the Operator Notifications matrix
// ("Update installed | INFO | ✓ | Important change record") and PART 22
// (Update Command): fired after `search --update yes` successfully installs
// a new version.
func (ml *Mailer) SendUpdateInstalled(previousVersion, newVersion string) error {
	if !ml.CanSend() || !ml.config.Events.UpdateInstalled {
		return nil
	}
	vars := ml.baseVars()
	vars["previous_version"] = previousVersion
	vars["new_version"] = newVersion
	subject, body, err := NewEmailTemplate().Render(TemplateUpdateInstalled, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendBackupCompleted notifies admins that a scheduled or manual backup
// completed successfully. Per AI.md PART 17 "Default Templates" →
// backup_complete and PART 21 (Backup & Restore).
func (ml *Mailer) SendBackupCompleted(filename, size string) error {
	if !ml.CanSend() || !ml.config.Events.BackupComplete {
		return nil
	}
	vars := ml.baseVars()
	vars["filename"] = filename
	vars["size"] = size
	subject, body, err := NewEmailTemplate().Render(TemplateBackupCompleted, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendBackupFailed notifies admins that a scheduled or manual backup failed.
// Per AI.md PART 17 "Default Templates" → backup_failed and PART 21
// (Backup & Restore). Per PART 17 "Suppression", this notification suppresses
// the generic scheduler_error email for the same backup task execution.
func (ml *Mailer) SendBackupFailed(filename, errMsg string) error {
	if !ml.CanSend() || !ml.config.Events.BackupFailed {
		return nil
	}
	vars := ml.baseVars()
	vars["filename"] = filename
	vars["error"] = errMsg
	subject, body, err := NewEmailTemplate().Render(TemplateBackupFailed, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendSSLExpiring notifies admins that a managed SSL certificate is
// approaching expiry. Per AI.md PART 17 "Default Templates" → ssl_expiring
// and PART 15 (SSL/TLS & Let's Encrypt): sent 7, 3, and 1 days before expiry.
func (ml *Mailer) SendSSLExpiring(domain, expiresIn, expiryDate string) error {
	if !ml.CanSend() || !ml.config.Events.SSLExpiring {
		return nil
	}
	vars := ml.baseVars()
	// Per AI.md PART 17 "ssl_expiring / ssl_renewed" variables, {fqdn} in an
	// SSL template is the domain name on the certificate.
	vars["fqdn"] = domain
	vars["domain"] = domain
	vars["expires_in"] = expiresIn
	vars["expiry_date"] = expiryDate
	subject, body, err := NewEmailTemplate().Render(TemplateSSLExpiring, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendSSLRenewed notifies admins that a managed SSL certificate was
// successfully renewed. Per AI.md PART 17 "Default Templates" → ssl_renewed
// and PART 15 (SSL/TLS & Let's Encrypt).
func (ml *Mailer) SendSSLRenewed(domain, validUntil string) error {
	if !ml.CanSend() || !ml.config.Events.SSLRenewed {
		return nil
	}
	vars := ml.baseVars()
	// Per AI.md PART 17 "ssl_expiring / ssl_renewed" variables, {fqdn} in an
	// SSL template is the domain name on the certificate.
	vars["fqdn"] = domain
	vars["domain"] = domain
	vars["valid_until"] = validUntil
	subject, body, err := NewEmailTemplate().Render(TemplateSSLRenewed, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendSSLRenewalFailed notifies admins that a managed SSL certificate's
// renewal attempt failed. Per AI.md PART 17 "Default Templates" →
// ssl_renewal_failed ("Includes domain, error, days until expiry, next
// retry") and the Operator Notifications matrix ("SSL renewal failed | ERROR
// | ✓ | Critical - needs attention"). Also suppresses the generic
// scheduler_error notification for the same task execution (PART 17
// Suppression rule).
func (ml *Mailer) SendSSLRenewalFailed(domain, errMsg string, daysLeft int, nextRetry string) error {
	if !ml.CanSend() || !ml.config.Events.SSLRenewalFailed {
		return nil
	}
	vars := ml.baseVars()
	// Per AI.md PART 17 "ssl_renewal_failed" variables: {fqdn} is the domain
	// whose cert failed to renew, {expires_in} the days until the current cert
	// expires and {expiry_date} its absolute expiry date.
	vars["fqdn"] = domain
	vars["domain"] = domain
	vars["error"] = errMsg
	vars["days_left"] = fmt.Sprintf("%d", daysLeft)
	vars["expires_in"] = fmt.Sprintf("%d days", daysLeft)
	vars["expiry_date"] = time.Now().AddDate(0, 0, daysLeft).Format("2006-01-02")
	vars["next_retry"] = nextRetry
	subject, body, err := NewEmailTemplate().Render(TemplateSSLRenewalFailed, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendSchedulerError notifies admins that a scheduled task failed and has no
// dedicated failure notification of its own. Per AI.md PART 17 "Default
// Templates" → scheduler_error and its Suppression rule: fires only for
// tasks with no dedicated failure event; backup_failed and ssl_renewal_failed
// suppress it for their respective task executions.
func (ml *Mailer) SendSchedulerError(taskName, errMsg, nextRun string) error {
	if !ml.CanSend() || !ml.config.Events.SchedulerError {
		return nil
	}
	vars := ml.baseVars()
	vars["task_name"] = taskName
	vars["error"] = errMsg
	vars["next_run"] = nextRun
	subject, body, err := NewEmailTemplate().Render(TemplateSchedulerError, vars)
	if err != nil {
		return err
	}
	return ml.SendToAdmins(subject, body)
}

// SendTest sends a test email to a specific recipient (not the admin list).
// Per AI.md PART 17 "Send Test Email": the operator-facing test action (and
// the `{project_name} email test` CLI command) targets a chosen recipient
// rather than every configured admin.
func (ml *Mailer) SendTest(to string) error {
	if to == "" {
		return fmt.Errorf("no recipient specified")
	}
	vars := ml.baseVars()
	vars["sent_at"] = vars["timestamp"]
	subject, body, err := NewEmailTemplate().Render(TemplateTest, vars)
	if err != nil {
		return err
	}
	// Delivered synchronously: the operator is waiting on the result, so the
	// SMTP outcome must be reported instead of being retried in the queue.
	if err := ml.SendNow(NewMessage([]string{to}, subject, body)); err != nil {
		slog.Error("Test email failed", "to", to, "template", string(TemplateTest), "err", err)
		return err
	}
	slog.Info("Test email sent", "to", to, "template", string(TemplateTest), "subject", subject)
	return nil
}

// SendTemplateTest renders templateType with the PART 17 preview sample data
// and sends it to a single recipient. Per AI.md PART 17 "Send Test Email":
// requires SMTP, uses sample data, subject prefixed `[TEST]`, and the send is
// logged. This is the service-layer entry point behind the operator's
// "Send Test" action for a specific template.
func (ml *Mailer) SendTemplateTest(to string, templateType TemplateType) error {
	if to == "" {
		return fmt.Errorf("no recipient specified")
	}
	if !ml.CanSend() {
		return fmt.Errorf("email features require SMTP configuration")
	}
	appName := ml.config.AppTitle
	if appName == "" {
		appName = i18n.TDefault("common.app_name")
	}
	subject, body, err := NewEmailTemplate().PreviewTemplate(templateType, appName, ml.config.AppURL)
	if err != nil {
		return err
	}
	subject = "[TEST] " + subject
	if err := ml.SendNow(NewMessage([]string{to}, subject, body)); err != nil {
		slog.Error("Test email failed", "to", to, "template", string(templateType), "err", err)
		return err
	}
	slog.Info("Test email sent", "to", to, "template", string(templateType), "subject", subject)
	return nil
}

// TestConnection tests the SMTP connection
// Per AI.md PART 18: Connection test on startup
func (ml *Mailer) TestConnection() error {
	if !ml.config.Enabled {
		return fmt.Errorf("email is not enabled")
	}

	if ml.config.SMTP.Host == "" {
		return fmt.Errorf("SMTP host is not configured")
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
		conn, err = tlsDial("tcp", addr, tlsConfig)
	} else {
		conn, err = netDialTimeout("tcp", addr, 10*time.Second)
	}

	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer closeutil.Close(conn)

	client, err := smtpNewClient(conn, ml.config.SMTP.Host)
	if err != nil {
		return fmt.Errorf("SMTP client creation failed: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Warn("failed to close SMTP client", "error", err)
		}
	}()

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
	Host         string
	Port         int
	TLS          bool
	STARTTLS     bool
	AuthRequired bool
}

// DetectSMTP auto-detects SMTP servers on the local system.
// Per AI.md PART 17: SMTP auto-detection on first run.
// Probe order: localhost → 127.0.0.1 → fqdn → globalIPv4 → mail.fqdn → smtp.fqdn → Docker host (172.17.0.1) → gateway IP.
// fqdn and globalIPv4 are the server's configured FQDN and global IPv4 address; pass empty strings if unknown.
// Ports: 25, 587, 465.
func DetectSMTP(fqdn, globalIPv4 string) *DetectedSMTP {
	// Per AI.md PART 17: Hosts to check in priority order
	hosts := []string{
		"localhost",
		"127.0.0.1",
	}

	if fqdn != "" {
		hosts = append(hosts, fqdn)
	}

	if globalIPv4 != "" {
		hosts = append(hosts, globalIPv4)
	}

	if fqdn != "" {
		hosts = append(hosts, "mail."+fqdn, "smtp."+fqdn)
	}

	// Docker bridge host
	hosts = append(hosts, "172.17.0.1")

	// Try to get gateway IP
	if gateway := getDefaultGateway(); gateway != "" {
		hosts = append(hosts, gateway)
	}

	// Per AI.md PART 17: Ports to check (25, 587, 465)
	ports := []struct {
		port     int
		tls      bool
		starttls bool
	}{
		// Standard SMTP
		{25, false, true},
		// Submission with STARTTLS
		{587, false, true},
		// SMTPS (TLS)
		{465, true, false},
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
	interfaces, err := netInterfaces()
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
		conn, err = tlsDialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr, tlsConfig)
	} else {
		conn, err = netDialTimeout("tcp", addr, 2*time.Second)
	}

	if err != nil {
		return nil
	}
	defer closeutil.Close(conn)

	client, err := smtpNewClient(conn, host)
	if err != nil {
		return nil
	}
	defer func() {
		if err := client.Close(); err != nil {
			slog.Warn("failed to close SMTP client", "error", err)
		}
	}()

	// Check if STARTTLS is supported
	hasSTARTTLS := false
	if !useTLS {
		hasSTARTTLS, _ = client.Extension("STARTTLS")
	}

	// Check if AUTH is required
	hasAuth, _ := client.Extension("AUTH")

	if err := client.Quit(); err != nil {
		slog.Warn("failed to close SMTP session", "error", err)
	}

	return &DetectedSMTP{
		Host:         host,
		Port:         port,
		TLS:          useTLS,
		STARTTLS:     hasSTARTTLS,
		AuthRequired: hasAuth,
	}
}

// DetectAndConfigure detects SMTP and returns a configured Config.
// Per AI.md PART 17: SMTP auto-detection on first run.
// fqdn and globalIPv4 are passed to DetectSMTP for extended host probing; pass empty strings if unknown.
func DetectAndConfigure(fqdn, globalIPv4 string) *Config {
	detected := DetectSMTP(fqdn, globalIPv4)
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
	return DetectSMTP("", "") != nil
}

// defaultMailer holds the process-wide mailer used by the package-level
// Notify* helpers, so that call sites which detect an operator event (rate
// limiting, authentication failure, blocklist hit) do not have to thread a
// *Mailer through their own construction.
var defaultMailer atomic.Pointer[Mailer]

// SetDefaultMailer registers the process-wide mailer used by the package-level
// Notify* helpers. Passing nil clears it, which makes every helper log-only.
func SetDefaultMailer(ml *Mailer) {
	defaultMailer.Store(ml)
}

// DefaultMailer returns the process-wide mailer registered with
// SetDefaultMailer, or nil when none has been registered.
func DefaultMailer() *Mailer {
	return defaultMailer.Load()
}

// NotifySecurityAlert records a security event and emails the operator when
// SMTP is available. It implements the AI.md PART 17 "Log vs Email Decision
// Matrix" for security events: the event is ALWAYS logged, and email is added
// only when SMTP is configured and `events.security_alert` is enabled. It is
// safe to call from any request path — it never blocks on SMTP (delivery goes
// through the queue when one is running) and never returns an error, so a
// failing mail server can never break the guarded request.
//
// Call sites are the abuse-detection points named in the PART 17 Operator
// Notifications matrix: rate limit exceeded, authentication failure and IP
// block (blocklist hit).
func NotifySecurityAlert(event, ip, details string) {
	slog.Error("Security alert", "event", event, "ip", ip, "details", details)
	ml := DefaultMailer()
	if ml == nil {
		return
	}
	if err := ml.SendSecurityAlert(event, ip, details); err != nil {
		slog.Error("Security alert email failed", "event", event, "err", err)
	}
}

// NotifySchedulerError records a scheduled task failure and emails the
// operator when SMTP is available. Per AI.md PART 17 "Suppression",
// callers must NOT invoke this for a backup or SSL renewal task that already
// emitted backup_failed or ssl_renewal_failed for the same execution.
func NotifySchedulerError(taskName, errMsg, nextRun string) {
	slog.Error("Scheduled task failed", "task", taskName, "err", errMsg, "next_run", nextRun)
	ml := DefaultMailer()
	if ml == nil {
		return
	}
	if err := ml.SendSchedulerError(taskName, errMsg, nextRun); err != nil {
		slog.Error("Scheduler error email failed", "task", taskName, "err", err)
	}
}
