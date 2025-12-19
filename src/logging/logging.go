package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogType represents different log types
type LogType string

const (
	LogTypeAccess   LogType = "access"
	LogTypeServer   LogType = "server"
	LogTypeSecurity LogType = "security"
	LogTypeAudit    LogType = "audit"
)

// Manager manages all log types
type Manager struct {
	mu       sync.RWMutex
	logDir   string
	access   *AccessLogger
	server   *ServerLogger
	security *SecurityLogger
	audit    *AuditLogger
}

// NewManager creates a new logging manager
func NewManager(logDir string) *Manager {
	if logDir == "" {
		logDir = "/data/logs/search"
	}

	// Ensure log directory exists
	os.MkdirAll(logDir, 0755)

	m := &Manager{
		logDir: logDir,
	}

	m.access = NewAccessLogger(filepath.Join(logDir, "access.log"))
	m.server = NewServerLogger(filepath.Join(logDir, "server.log"))
	m.security = NewSecurityLogger(filepath.Join(logDir, "security.log"))
	m.audit = NewAuditLogger(filepath.Join(logDir, "audit.log"))

	return m
}

// Access returns the access logger
func (m *Manager) Access() *AccessLogger {
	return m.access
}

// Server returns the server logger
func (m *Manager) Server() *ServerLogger {
	return m.server
}

// Security returns the security logger
func (m *Manager) Security() *SecurityLogger {
	return m.security
}

// Audit returns the audit logger
func (m *Manager) Audit() *AuditLogger {
	return m.audit
}

// Close closes all loggers
func (m *Manager) Close() error {
	var errs []error
	if err := m.access.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.server.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.security.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.audit.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors closing loggers: %v", errs)
	}
	return nil
}

// RotateAll rotates all log files
func (m *Manager) RotateAll() error {
	var errs []error
	if err := m.access.Rotate(); err != nil {
		errs = append(errs, err)
	}
	if err := m.server.Rotate(); err != nil {
		errs = append(errs, err)
	}
	if err := m.security.Rotate(); err != nil {
		errs = append(errs, err)
	}
	if err := m.audit.Rotate(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("errors rotating logs: %v", errs)
	}
	return nil
}

// ============================================================
// Access Logger - Apache/Nginx Combined Log Format with Custom Variables
// ============================================================

// FormatVariable represents a log format variable
type FormatVariable struct {
	Name        string
	Description string
	Example     string
}

// AvailableFormatVariables lists all supported format variables
var AvailableFormatVariables = []FormatVariable{
	{"$remote_addr", "Client IP address", "192.168.1.1"},
	{"$remote_user", "Client user name (from auth)", "-"},
	{"$time_local", "Local time in Common Log Format", "02/Jan/2006:15:04:05 -0700"},
	{"$time_iso8601", "ISO 8601 time format", "2006-01-02T15:04:05-07:00"},
	{"$time_unix", "Unix timestamp in seconds", "1704067200"},
	{"$time_msec", "Unix timestamp with milliseconds", "1704067200.123"},
	{"$request", "Full request line", "GET /search?q=test HTTP/1.1"},
	{"$request_method", "HTTP method", "GET"},
	{"$request_uri", "Full request URI with query string", "/search?q=test"},
	{"$request_path", "Request path only (no query string)", "/search"},
	{"$query_string", "Query string without ?", "q=test"},
	{"$status", "HTTP response status code", "200"},
	{"$body_bytes_sent", "Response body size in bytes", "1234"},
	{"$bytes_sent", "Total bytes sent (headers + body)", "1456"},
	{"$http_referer", "Referer header", "https://example.com/"},
	{"$http_user_agent", "User-Agent header", "Mozilla/5.0 ..."},
	{"$http_host", "Host header", "example.com"},
	{"$http_x_forwarded_for", "X-Forwarded-For header", "10.0.0.1"},
	{"$http_x_real_ip", "X-Real-IP header", "10.0.0.1"},
	{"$server_protocol", "Request protocol", "HTTP/1.1"},
	{"$request_time", "Request processing time in seconds with ms precision", "0.123"},
	{"$request_time_ms", "Request processing time in milliseconds", "123"},
	{"$request_id", "Unique request ID (if set)", "550e8400-e29b-41d4-a716-446655440000"},
	{"$connection", "Connection serial number", "12345"},
	{"$connection_requests", "Number of requests on this connection", "3"},
	{"$ssl_protocol", "SSL protocol (TLSv1.2, TLSv1.3, etc.)", "TLSv1.3"},
	{"$ssl_cipher", "SSL cipher used", "TLS_AES_128_GCM_SHA256"},
	{"$hostname", "Server hostname", "search.example.com"},
	{"$pid", "Process ID", "12345"},
}

// AccessLogger logs HTTP access in Combined Log Format
type AccessLogger struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	format       string // "combined", "common", "json", "custom"
	customFormat string // Custom format string with variables
}

// AccessEntry represents an access log entry with all fields for custom formatting
type AccessEntry struct {
	Timestamp          time.Time `json:"timestamp"`
	IP                 string    `json:"ip"`
	Method             string    `json:"method"`
	Path               string    `json:"path"`
	QueryString        string    `json:"query_string,omitempty"`
	Protocol           string    `json:"protocol"`
	Status             int       `json:"status"`
	Size               int64     `json:"size"`
	BytesSent          int64     `json:"bytes_sent"`
	Referer            string    `json:"referer"`
	UserAgent          string    `json:"user_agent"`
	Latency            int64     `json:"latency_ms"`
	RequestID          string    `json:"request_id,omitempty"`
	RemoteUser         string    `json:"remote_user,omitempty"`
	Host               string    `json:"host,omitempty"`
	XForwardedFor      string    `json:"x_forwarded_for,omitempty"`
	XRealIP            string    `json:"x_real_ip,omitempty"`
	SSLProtocol        string    `json:"ssl_protocol,omitempty"`
	SSLCipher          string    `json:"ssl_cipher,omitempty"`
	Connection         int64     `json:"connection,omitempty"`
	ConnectionRequests int       `json:"connection_requests,omitempty"`
}

// NewAccessLogger creates a new access logger
func NewAccessLogger(path string) *AccessLogger {
	l := &AccessLogger{
		path:   path,
		format: "combined",
	}
	l.openFile()
	return l
}

func (l *AccessLogger) openFile() error {
	if l.path == "" {
		return nil
	}

	dir := filepath.Dir(l.path)
	os.MkdirAll(dir, 0755)

	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

// Log logs an access entry
func (l *AccessLogger) Log(entry AccessEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	var line string

	switch l.format {
	case "json":
		data, _ := json.Marshal(entry)
		line = string(data)
	case "common":
		// Common Log Format: host ident authuser date request status bytes
		line = fmt.Sprintf("%s - - [%s] \"%s %s %s\" %d %d",
			entry.IP,
			entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700"),
			entry.Method,
			entry.Path,
			entry.Protocol,
			entry.Status,
			entry.Size,
		)
	case "custom":
		// Custom format with variable substitution
		line = l.formatWithVariables(entry)
	default: // combined
		// Combined Log Format: host ident authuser date request status bytes referer user-agent
		referer := entry.Referer
		if referer == "" {
			referer = "-"
		}
		line = fmt.Sprintf("%s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\"",
			entry.IP,
			entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700"),
			entry.Method,
			entry.Path,
			entry.Protocol,
			entry.Status,
			entry.Size,
			referer,
			entry.UserAgent,
		)
	}

	if l.file != nil {
		l.file.WriteString(line + "\n")
	}
}

// formatWithVariables formats an entry using the custom format string
func (l *AccessLogger) formatWithVariables(entry AccessEntry) string {
	if l.customFormat == "" {
		// Fall back to combined format
		return fmt.Sprintf("%s - - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\"",
			entry.IP,
			entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700"),
			entry.Method,
			entry.Path,
			entry.Protocol,
			entry.Status,
			entry.Size,
			orDash(entry.Referer),
			entry.UserAgent,
		)
	}

	result := l.customFormat

	// Build request URI
	requestURI := entry.Path
	if entry.QueryString != "" {
		requestURI += "?" + entry.QueryString
	}

	// Build full request line
	request := fmt.Sprintf("%s %s %s", entry.Method, requestURI, entry.Protocol)

	// Define all variable replacements
	replacements := map[string]string{
		"$remote_addr":           entry.IP,
		"$remote_user":           orDash(entry.RemoteUser),
		"$time_local":            entry.Timestamp.Format("02/Jan/2006:15:04:05 -0700"),
		"$time_iso8601":          entry.Timestamp.Format(time.RFC3339),
		"$time_unix":             fmt.Sprintf("%d", entry.Timestamp.Unix()),
		"$time_msec":             fmt.Sprintf("%d.%03d", entry.Timestamp.Unix(), entry.Timestamp.Nanosecond()/1000000),
		"$request":               request,
		"$request_method":        entry.Method,
		"$request_uri":           requestURI,
		"$request_path":          entry.Path,
		"$query_string":          entry.QueryString,
		"$status":                fmt.Sprintf("%d", entry.Status),
		"$body_bytes_sent":       fmt.Sprintf("%d", entry.Size),
		"$bytes_sent":            fmt.Sprintf("%d", entry.BytesSent),
		"$http_referer":          orDash(entry.Referer),
		"$http_user_agent":       entry.UserAgent,
		"$http_host":             orDash(entry.Host),
		"$http_x_forwarded_for":  orDash(entry.XForwardedFor),
		"$http_x_real_ip":        orDash(entry.XRealIP),
		"$server_protocol":       entry.Protocol,
		"$request_time":          fmt.Sprintf("%.3f", float64(entry.Latency)/1000.0),
		"$request_time_ms":       fmt.Sprintf("%d", entry.Latency),
		"$request_id":            orDash(entry.RequestID),
		"$connection":            fmt.Sprintf("%d", entry.Connection),
		"$connection_requests":   fmt.Sprintf("%d", entry.ConnectionRequests),
		"$ssl_protocol":          orDash(entry.SSLProtocol),
		"$ssl_cipher":            orDash(entry.SSLCipher),
		"$hostname":              getHostname(),
		"$pid":                   fmt.Sprintf("%d", os.Getpid()),
	}

	// Apply replacements (order by length descending to avoid partial matches)
	for _, v := range AvailableFormatVariables {
		if val, ok := replacements[v.Name]; ok {
			result = strings.ReplaceAll(result, v.Name, val)
		}
	}

	return result
}

// orDash returns the value or "-" if empty
func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// getHostname returns the system hostname
func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// LogRequest logs an HTTP request
func (l *AccessLogger) LogRequest(r *http.Request, status int, size int64, latency time.Duration) {
	ip := r.RemoteAddr
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	xRealIP := r.Header.Get("X-Real-IP")

	if xForwardedFor != "" {
		ip = strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	}
	if xRealIP != "" {
		ip = xRealIP
	}

	// Strip port from IP
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		if !strings.Contains(ip[idx:], "]") { // Handle IPv6
			ip = ip[:idx]
		}
	}

	protocol := fmt.Sprintf("HTTP/%d.%d", r.ProtoMajor, r.ProtoMinor)

	// Get TLS info if available
	sslProtocol := ""
	sslCipher := ""
	if r.TLS != nil {
		switch r.TLS.Version {
		case 0x0301:
			sslProtocol = "TLSv1.0"
		case 0x0302:
			sslProtocol = "TLSv1.1"
		case 0x0303:
			sslProtocol = "TLSv1.2"
		case 0x0304:
			sslProtocol = "TLSv1.3"
		}
		sslCipher = tlsCipherSuiteName(r.TLS.CipherSuite)
	}

	l.Log(AccessEntry{
		Timestamp:     time.Now(),
		IP:            ip,
		Method:        r.Method,
		Path:          r.URL.Path,
		QueryString:   r.URL.RawQuery,
		Protocol:      protocol,
		Status:        status,
		Size:          size,
		BytesSent:     size + estimateHeaderSize(status),
		Referer:       r.Header.Get("Referer"),
		UserAgent:     r.Header.Get("User-Agent"),
		Latency:       latency.Milliseconds(),
		Host:          r.Host,
		XForwardedFor: xForwardedFor,
		XRealIP:       xRealIP,
		SSLProtocol:   sslProtocol,
		SSLCipher:     sslCipher,
	})
}

// LogRequestWithID logs an HTTP request with a request ID
func (l *AccessLogger) LogRequestWithID(r *http.Request, status int, size int64, latency time.Duration, requestID string) {
	ip := r.RemoteAddr
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	xRealIP := r.Header.Get("X-Real-IP")

	if xForwardedFor != "" {
		ip = strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	}
	if xRealIP != "" {
		ip = xRealIP
	}

	// Strip port from IP
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		if !strings.Contains(ip[idx:], "]") { // Handle IPv6
			ip = ip[:idx]
		}
	}

	protocol := fmt.Sprintf("HTTP/%d.%d", r.ProtoMajor, r.ProtoMinor)

	// Get TLS info if available
	sslProtocol := ""
	sslCipher := ""
	if r.TLS != nil {
		switch r.TLS.Version {
		case 0x0301:
			sslProtocol = "TLSv1.0"
		case 0x0302:
			sslProtocol = "TLSv1.1"
		case 0x0303:
			sslProtocol = "TLSv1.2"
		case 0x0304:
			sslProtocol = "TLSv1.3"
		}
		sslCipher = tlsCipherSuiteName(r.TLS.CipherSuite)
	}

	l.Log(AccessEntry{
		Timestamp:     time.Now(),
		IP:            ip,
		Method:        r.Method,
		Path:          r.URL.Path,
		QueryString:   r.URL.RawQuery,
		Protocol:      protocol,
		Status:        status,
		Size:          size,
		BytesSent:     size + estimateHeaderSize(status),
		Referer:       r.Header.Get("Referer"),
		UserAgent:     r.Header.Get("User-Agent"),
		Latency:       latency.Milliseconds(),
		RequestID:     requestID,
		Host:          r.Host,
		XForwardedFor: xForwardedFor,
		XRealIP:       xRealIP,
		SSLProtocol:   sslProtocol,
		SSLCipher:     sslCipher,
	})
}

// estimateHeaderSize estimates the response header size
func estimateHeaderSize(status int) int64 {
	// Rough estimate: status line + common headers
	return 200
}

// tlsCipherSuiteName returns the name of a TLS cipher suite
func tlsCipherSuiteName(id uint16) string {
	// Common cipher suites
	names := map[uint16]string{
		0x1301: "TLS_AES_128_GCM_SHA256",
		0x1302: "TLS_AES_256_GCM_SHA384",
		0x1303: "TLS_CHACHA20_POLY1305_SHA256",
		0xc02f: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		0xc030: "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
		0xcca8: "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305",
		0xc02b: "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
		0xc02c: "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
		0xcca9: "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305",
	}
	if name, ok := names[id]; ok {
		return name
	}
	return fmt.Sprintf("0x%04x", id)
}

// Rotate rotates the access log file
func (l *AccessLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rotate()
}

func (l *AccessLogger) rotate() error {
	if l.file == nil {
		return nil
	}

	l.file.Close()

	timestamp := time.Now().Format("20060102")
	rotatedPath := fmt.Sprintf("%s.%s", l.path, timestamp)

	// If rotated file already exists, add time
	if _, err := os.Stat(rotatedPath); err == nil {
		rotatedPath = fmt.Sprintf("%s.%s", l.path, time.Now().Format("20060102-150405"))
	}

	os.Rename(l.path, rotatedPath)
	return l.openFile()
}

// Close closes the access logger
func (l *AccessLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// SetFormat sets the log format
func (l *AccessLogger) SetFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format
}

// SetCustomFormat sets a custom log format with variables
// Example: "$remote_addr - $remote_user [$time_local] \"$request\" $status $body_bytes_sent"
func (l *AccessLogger) SetCustomFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = "custom"
	l.customFormat = format
}

// GetCustomFormat returns the current custom format string
func (l *AccessLogger) GetCustomFormat() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.customFormat
}

// ValidateFormat validates a custom format string and returns any unknown variables
func ValidateFormat(format string) []string {
	var unknown []string
	// Find all $variable patterns
	i := 0
	for i < len(format) {
		if format[i] == '$' {
			// Find the end of the variable name
			j := i + 1
			for j < len(format) && (format[j] == '_' || (format[j] >= 'a' && format[j] <= 'z') || (format[j] >= '0' && format[j] <= '9')) {
				j++
			}
			if j > i+1 {
				varName := format[i:j]
				found := false
				for _, v := range AvailableFormatVariables {
					if v.Name == varName {
						found = true
						break
					}
				}
				if !found {
					unknown = append(unknown, varName)
				}
			}
			i = j
		} else {
			i++
		}
	}
	return unknown
}

// ============================================================
// Server Logger - Application logs
// ============================================================

// LogLevel represents log severity
type LogLevel int

const (
	LevelDebug LogLevel = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

func (l LogLevel) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ServerLogger logs application events
type ServerLogger struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	level    LogLevel
	format   string // "text", "json"
	stdout   bool
}

// ServerEntry represents a server log entry
type ServerEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// NewServerLogger creates a new server logger
func NewServerLogger(path string) *ServerLogger {
	l := &ServerLogger{
		path:   path,
		level:  LevelInfo,
		format: "text",
		stdout: true,
	}
	l.openFile()
	return l
}

func (l *ServerLogger) openFile() error {
	if l.path == "" {
		return nil
	}

	dir := filepath.Dir(l.path)
	os.MkdirAll(dir, 0755)

	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

// SetLevel sets the minimum log level
func (l *ServerLogger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetFormat sets the log format
func (l *ServerLogger) SetFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format
}

// SetStdout enables/disables stdout output
func (l *ServerLogger) SetStdout(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stdout = enabled
}

func (l *ServerLogger) log(level LogLevel, msg string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	entry := ServerEntry{
		Timestamp: time.Now(),
		Level:     level.String(),
		Message:   msg,
		Fields:    fields,
	}

	var line string
	if l.format == "json" {
		data, _ := json.Marshal(entry)
		line = string(data)
	} else {
		line = fmt.Sprintf("[%s] [%s] %s",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.Level,
			entry.Message,
		)
		if len(fields) > 0 {
			parts := make([]string, 0, len(fields))
			for k, v := range fields {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			}
			line += " " + strings.Join(parts, " ")
		}
	}

	if l.stdout {
		fmt.Println(line)
	}
	if l.file != nil {
		l.file.WriteString(line + "\n")
	}
}

// Debug logs a debug message
func (l *ServerLogger) Debug(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelDebug, msg, f)
}

// Info logs an info message
func (l *ServerLogger) Info(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelInfo, msg, f)
}

// Warn logs a warning message
func (l *ServerLogger) Warn(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelWarn, msg, f)
}

// Error logs an error message
func (l *ServerLogger) Error(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelError, msg, f)
}

// Fatal logs a fatal message and exits
func (l *ServerLogger) Fatal(msg string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	l.log(LevelFatal, msg, f)
	os.Exit(1)
}

// Rotate rotates the server log file
func (l *ServerLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	l.file.Close()

	timestamp := time.Now().Format("20060102")
	rotatedPath := fmt.Sprintf("%s.%s", l.path, timestamp)

	if _, err := os.Stat(rotatedPath); err == nil {
		rotatedPath = fmt.Sprintf("%s.%s", l.path, time.Now().Format("20060102-150405"))
	}

	os.Rename(l.path, rotatedPath)
	return l.openFile()
}

// Close closes the server logger
func (l *ServerLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Writer returns an io.Writer for use with standard log
func (l *ServerLogger) Writer() io.Writer {
	return &serverLogWriter{logger: l, level: LevelInfo}
}

type serverLogWriter struct {
	logger *ServerLogger
	level  LogLevel
}

func (w *serverLogWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		w.logger.log(w.level, msg, nil)
	}
	return len(p), nil
}

// ============================================================
// Security Logger - Fail2ban compatible
// ============================================================

// SecurityEvent represents a security event type
type SecurityEvent string

const (
	SecurityEventLoginFailed    SecurityEvent = "LOGIN_FAILED"
	SecurityEventLoginSuccess   SecurityEvent = "LOGIN_SUCCESS"
	SecurityEventRateLimited    SecurityEvent = "RATE_LIMITED"
	SecurityEventBlocked        SecurityEvent = "BLOCKED"
	SecurityEventSuspicious     SecurityEvent = "SUSPICIOUS"
	SecurityEventBruteForce     SecurityEvent = "BRUTE_FORCE"
	SecurityEventInvalidToken   SecurityEvent = "INVALID_TOKEN"
	SecurityEventCSRFViolation  SecurityEvent = "CSRF_VIOLATION"
)

// SecurityLogger logs security events (fail2ban compatible)
type SecurityLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// SecurityEntry represents a security log entry
type SecurityEntry struct {
	Timestamp time.Time     `json:"timestamp"`
	Event     SecurityEvent `json:"event"`
	IP        string        `json:"ip"`
	User      string        `json:"user,omitempty"`
	Path      string        `json:"path,omitempty"`
	Details   string        `json:"details,omitempty"`
}

// NewSecurityLogger creates a new security logger
func NewSecurityLogger(path string) *SecurityLogger {
	l := &SecurityLogger{
		path: path,
	}
	l.openFile()
	return l
}

func (l *SecurityLogger) openFile() error {
	if l.path == "" {
		return nil
	}

	dir := filepath.Dir(l.path)
	os.MkdirAll(dir, 0755)

	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

// Log logs a security event
func (l *SecurityLogger) Log(entry SecurityEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Fail2ban compatible format:
	// YYYY-MM-DD HH:MM:SS [EVENT] IP=x.x.x.x USER=xxx PATH=/xxx DETAILS=xxx
	line := fmt.Sprintf("%s [%s] IP=%s",
		entry.Timestamp.Format("2006-01-02 15:04:05"),
		entry.Event,
		entry.IP,
	)

	if entry.User != "" {
		line += fmt.Sprintf(" USER=%s", entry.User)
	}
	if entry.Path != "" {
		line += fmt.Sprintf(" PATH=%s", entry.Path)
	}
	if entry.Details != "" {
		line += fmt.Sprintf(" DETAILS=%s", entry.Details)
	}

	if l.file != nil {
		l.file.WriteString(line + "\n")
	}

	// Also print to stdout for visibility
	fmt.Println("[SECURITY] " + line)
}

// LogLoginFailed logs a failed login attempt
func (l *SecurityLogger) LogLoginFailed(ip, user, path string) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventLoginFailed,
		IP:        ip,
		User:      user,
		Path:      path,
	})
}

// LogLoginSuccess logs a successful login
func (l *SecurityLogger) LogLoginSuccess(ip, user string) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventLoginSuccess,
		IP:        ip,
		User:      user,
	})
}

// LogRateLimited logs a rate limit event
func (l *SecurityLogger) LogRateLimited(ip, path string) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventRateLimited,
		IP:        ip,
		Path:      path,
	})
}

// LogBlocked logs a blocked request
func (l *SecurityLogger) LogBlocked(ip, path, reason string) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventBlocked,
		IP:        ip,
		Path:      path,
		Details:   reason,
	})
}

// LogSuspicious logs a suspicious activity
func (l *SecurityLogger) LogSuspicious(ip, path, details string) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventSuspicious,
		IP:        ip,
		Path:      path,
		Details:   details,
	})
}

// LogBruteForce logs a brute force detection
func (l *SecurityLogger) LogBruteForce(ip, path string, attempts int) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventBruteForce,
		IP:        ip,
		Path:      path,
		Details:   fmt.Sprintf("attempts=%d", attempts),
	})
}

// LogInvalidToken logs an invalid token attempt
func (l *SecurityLogger) LogInvalidToken(ip, path string) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventInvalidToken,
		IP:        ip,
		Path:      path,
	})
}

// LogCSRFViolation logs a CSRF violation
func (l *SecurityLogger) LogCSRFViolation(ip, path string) {
	l.Log(SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventCSRFViolation,
		IP:        ip,
		Path:      path,
	})
}

// Rotate rotates the security log file
func (l *SecurityLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	l.file.Close()

	timestamp := time.Now().Format("20060102")
	rotatedPath := fmt.Sprintf("%s.%s", l.path, timestamp)

	if _, err := os.Stat(rotatedPath); err == nil {
		rotatedPath = fmt.Sprintf("%s.%s", l.path, time.Now().Format("20060102-150405"))
	}

	os.Rename(l.path, rotatedPath)
	return l.openFile()
}

// Close closes the security logger
func (l *SecurityLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// ============================================================
// Audit Logger - Admin actions and configuration changes
// ============================================================

// AuditAction represents an audit action type
type AuditAction string

const (
	AuditActionLogin         AuditAction = "LOGIN"
	AuditActionLogout        AuditAction = "LOGOUT"
	AuditActionConfigChange  AuditAction = "CONFIG_CHANGE"
	AuditActionEngineToggle  AuditAction = "ENGINE_TOGGLE"
	AuditActionTokenCreate   AuditAction = "TOKEN_CREATE"
	AuditActionTokenRevoke   AuditAction = "TOKEN_REVOKE"
	AuditActionReload        AuditAction = "RELOAD"
	AuditActionUserCreate    AuditAction = "USER_CREATE"
	AuditActionUserDelete    AuditAction = "USER_DELETE"
	AuditActionPermissionChange AuditAction = "PERMISSION_CHANGE"
)

// AuditLogger logs administrative actions
type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
	path string
}

// AuditEntry represents an audit log entry
type AuditEntry struct {
	Timestamp time.Time   `json:"timestamp"`
	Action    AuditAction `json:"action"`
	User      string      `json:"user"`
	IP        string      `json:"ip"`
	Resource  string      `json:"resource,omitempty"`
	Details   string      `json:"details,omitempty"`
	Success   bool        `json:"success"`
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(path string) *AuditLogger {
	l := &AuditLogger{
		path: path,
	}
	l.openFile()
	return l
}

func (l *AuditLogger) openFile() error {
	if l.path == "" {
		return nil
	}

	dir := filepath.Dir(l.path)
	os.MkdirAll(dir, 0755)

	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

// Log logs an audit event
func (l *AuditLogger) Log(entry AuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// JSON format for audit logs (easy to parse and analyze)
	data, _ := json.Marshal(entry)

	if l.file != nil {
		l.file.WriteString(string(data) + "\n")
	}

	// Also print to stdout for visibility
	status := "SUCCESS"
	if !entry.Success {
		status = "FAILED"
	}
	fmt.Printf("[AUDIT] %s %s user=%s ip=%s resource=%s\n",
		status, entry.Action, entry.User, entry.IP, entry.Resource)
}

// LogLogin logs a login attempt
func (l *AuditLogger) LogLogin(user, ip string, success bool) {
	l.Log(AuditEntry{
		Timestamp: time.Now(),
		Action:    AuditActionLogin,
		User:      user,
		IP:        ip,
		Success:   success,
	})
}

// LogLogout logs a logout
func (l *AuditLogger) LogLogout(user, ip string) {
	l.Log(AuditEntry{
		Timestamp: time.Now(),
		Action:    AuditActionLogout,
		User:      user,
		IP:        ip,
		Success:   true,
	})
}

// LogConfigChange logs a configuration change
func (l *AuditLogger) LogConfigChange(user, ip, resource, details string) {
	l.Log(AuditEntry{
		Timestamp: time.Now(),
		Action:    AuditActionConfigChange,
		User:      user,
		IP:        ip,
		Resource:  resource,
		Details:   details,
		Success:   true,
	})
}

// LogEngineToggle logs an engine enable/disable
func (l *AuditLogger) LogEngineToggle(user, ip, engine string, enabled bool) {
	action := "disabled"
	if enabled {
		action = "enabled"
	}
	l.Log(AuditEntry{
		Timestamp: time.Now(),
		Action:    AuditActionEngineToggle,
		User:      user,
		IP:        ip,
		Resource:  engine,
		Details:   action,
		Success:   true,
	})
}

// LogTokenCreate logs a token creation
func (l *AuditLogger) LogTokenCreate(user, ip, tokenName string) {
	l.Log(AuditEntry{
		Timestamp: time.Now(),
		Action:    AuditActionTokenCreate,
		User:      user,
		IP:        ip,
		Resource:  tokenName,
		Success:   true,
	})
}

// LogTokenRevoke logs a token revocation
func (l *AuditLogger) LogTokenRevoke(user, ip, tokenName string) {
	l.Log(AuditEntry{
		Timestamp: time.Now(),
		Action:    AuditActionTokenRevoke,
		User:      user,
		IP:        ip,
		Resource:  tokenName,
		Success:   true,
	})
}

// LogReload logs a configuration reload
func (l *AuditLogger) LogReload(user, ip string, success bool, details string) {
	l.Log(AuditEntry{
		Timestamp: time.Now(),
		Action:    AuditActionReload,
		User:      user,
		IP:        ip,
		Details:   details,
		Success:   success,
	})
}

// Rotate rotates the audit log file
func (l *AuditLogger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	l.file.Close()

	timestamp := time.Now().Format("20060102")
	rotatedPath := fmt.Sprintf("%s.%s", l.path, timestamp)

	if _, err := os.Stat(rotatedPath); err == nil {
		rotatedPath = fmt.Sprintf("%s.%s", l.path, time.Now().Format("20060102-150405"))
	}

	os.Rename(l.path, rotatedPath)
	return l.openFile()
}

// Close closes the audit logger
func (l *AuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
