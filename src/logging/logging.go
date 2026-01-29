package logging

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Testable variables for error path testing
var (
	osHostname = os.Hostname
	osExit     = os.Exit
)

// LogType represents different log types
type LogType string

const (
	LogTypeAccess   LogType = "access"
	LogTypeServer   LogType = "server"
	LogTypeError    LogType = "error"
	LogTypeSecurity LogType = "security"
	LogTypeAudit    LogType = "audit"
	LogTypeDebug    LogType = "debug"
)

// Manager manages all log types
type Manager struct {
	mu       sync.RWMutex
	logDir   string
	access   *AccessLogger
	server   *ServerLogger
	errorLog *ErrorLogger
	security *SecurityLogger
	audit    *AuditLogger
	debug    *DebugLogger
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
	m.errorLog = NewErrorLogger(filepath.Join(logDir, "error.log"))
	m.security = NewSecurityLogger(filepath.Join(logDir, "security.log"))
	m.audit = NewAuditLogger(filepath.Join(logDir, "audit.log"))
	m.debug = NewDebugLogger(filepath.Join(logDir, "debug.log"))

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

// Error returns the error logger
func (m *Manager) Error() *ErrorLogger {
	return m.errorLog
}

// Security returns the security logger
func (m *Manager) Security() *SecurityLogger {
	return m.security
}

// Audit returns the audit logger
func (m *Manager) Audit() *AuditLogger {
	return m.audit
}

// Debug returns the debug logger
func (m *Manager) Debug() *DebugLogger {
	return m.debug
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
	if err := m.errorLog.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.security.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.audit.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := m.debug.Close(); err != nil {
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
	if err := m.errorLog.Rotate(); err != nil {
		errs = append(errs, err)
	}
	if err := m.security.Rotate(); err != nil {
		errs = append(errs, err)
	}
	if err := m.audit.Rotate(); err != nil {
		errs = append(errs, err)
	}
	if err := m.debug.Rotate(); err != nil {
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
	// Sort keys by length descending
	keys := make([]string, 0, len(replacements))
	for k := range replacements {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return len(keys[i]) > len(keys[j])
	})

	for _, k := range keys {
		result = strings.ReplaceAll(result, k, replacements[k])
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
	hostname, err := osHostname()
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
	osExit(1)
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
// Per AI.md PART 25: ULID format IDs, Actor/Target, Categories, Severity
// ============================================================

// AuditAction represents an audit event type per AI.md PART 11 lines 11772-11936
// Uses dot notation: category.action (e.g., admin.login, user.created)
type AuditAction string

const (
	// Admin events (PART 11 lines 11772-11786)
	AuditActionLogin            AuditAction = "admin.login"
	AuditActionLogout           AuditAction = "admin.logout"
	AuditActionLoginFailed      AuditAction = "admin.login_failed"
	AuditActionAdminCreated     AuditAction = "admin.created"
	AuditActionAdminDeleted     AuditAction = "admin.deleted"
	AuditActionPasswordChanged  AuditAction = "admin.password_changed"
	AuditActionMFAEnabled       AuditAction = "admin.mfa_enabled"
	AuditActionMFADisabled      AuditAction = "admin.mfa_disabled"
	AuditActionTokenRegenerated AuditAction = "admin.token_regenerated"
	AuditActionSessionExpired   AuditAction = "admin.session_expired"
	AuditActionSessionRevoked   AuditAction = "admin.session_revoked"

	// User events (PART 11 lines 11788-11807)
	AuditActionUserRegistered        AuditAction = "user.registered"
	AuditActionUserLogin             AuditAction = "user.login"
	AuditActionUserLogout            AuditAction = "user.logout"
	AuditActionUserLoginFailed       AuditAction = "user.login_failed"
	AuditActionUserCreate            AuditAction = "user.created"
	AuditActionUserDelete            AuditAction = "user.deleted"
	AuditActionUserSuspended         AuditAction = "user.suspended"
	AuditActionUserUnsuspended       AuditAction = "user.unsuspended"
	AuditActionUserRoleChanged       AuditAction = "user.role_changed"
	AuditActionUserPasswordChanged   AuditAction = "user.password_changed"
	AuditActionUserPasswordResetReq  AuditAction = "user.password_reset_requested"
	AuditActionUserPasswordResetDone AuditAction = "user.password_reset_completed"
	AuditActionUserEmailVerified     AuditAction = "user.email_verified"
	AuditActionUserMFAEnabled        AuditAction = "user.mfa_enabled"
	AuditActionUserMFADisabled       AuditAction = "user.mfa_disabled"
	AuditActionUserRecoveryKeyUsed   AuditAction = "user.recovery_key_used"

	// Organization events (PART 11 lines 11809-11827)
	AuditActionOrgCreated             AuditAction = "org.created"
	AuditActionOrgDeleted             AuditAction = "org.deleted"
	AuditActionOrgSettingsUpdated     AuditAction = "org.settings_updated"
	AuditActionOrgMemberInvited       AuditAction = "org.member_invited"
	AuditActionOrgMemberJoined        AuditAction = "org.member_joined"
	AuditActionOrgMemberRemoved       AuditAction = "org.member_removed"
	AuditActionOrgMemberLeft          AuditAction = "org.member_left"
	AuditActionOrgRoleChanged         AuditAction = "org.role_changed"
	AuditActionOrgRoleCreated         AuditAction = "org.role_created"
	AuditActionOrgRoleUpdated         AuditAction = "org.role_updated"
	AuditActionOrgRoleDeleted         AuditAction = "org.role_deleted"
	AuditActionOrgTokenCreated        AuditAction = "org.token_created"
	AuditActionOrgTokenRevoked        AuditAction = "org.token_revoked"
	AuditActionOrgOwnershipTransfer   AuditAction = "org.ownership_transferred"
	AuditActionOrgBillingUpdated      AuditAction = "org.billing_updated"

	// Configuration events (PART 11 lines 11884-11897)
	AuditActionConfigChange         AuditAction = "config.updated"
	AuditActionConfigSMTPUpdated    AuditAction = "config.smtp_updated"
	AuditActionConfigSSLUpdated     AuditAction = "config.ssl_updated"
	AuditActionConfigSSLExpired     AuditAction = "config.ssl_expired"
	AuditActionConfigTorRegen       AuditAction = "config.tor_address_regenerated"
	AuditActionConfigBrandingUpdate AuditAction = "config.branding_updated"
	AuditActionConfigOIDCAdded      AuditAction = "config.oidc_provider_added"
	AuditActionConfigOIDCRemoved    AuditAction = "config.oidc_provider_removed"
	AuditActionConfigLDAPUpdated    AuditAction = "config.ldap_updated"
	AuditActionConfigAdminGroups    AuditAction = "config.admin_groups_updated"

	// Security events (PART 11 lines 11899-11910)
	AuditActionRateLimitExceeded   AuditAction = "security.rate_limit_exceeded"
	AuditActionIPBlocked           AuditAction = "security.ip_blocked"
	AuditActionIPUnblocked         AuditAction = "security.ip_unblocked"
	AuditActionCountryBlocked      AuditAction = "security.country_blocked"
	AuditActionCSRFFailure         AuditAction = "security.csrf_failure"
	AuditActionInvalidToken        AuditAction = "security.invalid_token"
	AuditActionBruteForceDetected  AuditAction = "security.brute_force_detected"
	AuditActionSuspiciousActivity  AuditAction = "security.suspicious_activity"

	// Token events (PART 11 lines 11912-11919)
	AuditActionTokenCreate  AuditAction = "token.created"
	AuditActionTokenRevoke  AuditAction = "token.revoked"
	AuditActionTokenExpired AuditAction = "token.expired"
	AuditActionTokenUsed    AuditAction = "token.used"

	// Backup & System events (PART 11 lines 11921-11935)
	AuditActionBackupCreate       AuditAction = "backup.created"
	AuditActionBackupRestore      AuditAction = "backup.restored"
	AuditActionBackupDelete       AuditAction = "backup.deleted"
	AuditActionBackupFailed       AuditAction = "backup.failed"
	AuditActionServerStarted      AuditAction = "server.started"
	AuditActionServerStopped      AuditAction = "server.stopped"
	AuditActionMaintenanceEntered AuditAction = "server.maintenance_entered"
	AuditActionMaintenanceExited  AuditAction = "server.maintenance_exited"
	AuditActionServerUpdated      AuditAction = "server.updated"
	AuditActionSchedulerTaskFail  AuditAction = "scheduler.task_failed"
	AuditActionSchedulerTaskRun   AuditAction = "scheduler.task_manual_run"

	// Cluster events (PART 11 lines 11937-11945)
	AuditActionClusterNodeJoined   AuditAction = "cluster.node_joined"
	AuditActionClusterNodeRemoved  AuditAction = "cluster.node_removed"
	AuditActionClusterNodeFailed   AuditAction = "cluster.node_failed"
	AuditActionClusterTokenGen     AuditAction = "cluster.token_generated"
	AuditActionClusterModeChanged  AuditAction = "cluster.mode_changed"

	// Legacy aliases for backward compatibility
	AuditActionReload        AuditAction = "config.reload"
	AuditActionEngineToggle  AuditAction = "config.engine_toggle"
	AuditActionAdminInvite   AuditAction = "admin.invite"
	AuditActionPermissionChange AuditAction = "user.permission_change"
)

// AuditCategory represents audit event categories per AI.md PART 11
type AuditCategory string

const (
	AuditCategoryAuth           AuditCategory = "authentication" // Authentication events
	AuditCategoryAdmin          AuditCategory = "admin"          // Admin panel actions
	AuditCategoryConfig         AuditCategory = "configuration"  // Configuration changes
	AuditCategoryUser           AuditCategory = "users"          // User management
	AuditCategorySecurity       AuditCategory = "security"       // Security-related events
	AuditCategoryData           AuditCategory = "backup"         // Backup/data operations
	AuditCategorySystem         AuditCategory = "server"         // Server/system operations
	AuditCategoryTokens         AuditCategory = "tokens"         // Token events
	AuditCategoryCluster        AuditCategory = "cluster"        // Cluster events
	AuditCategoryOrganization   AuditCategory = "organization"   // Organization events
)

// AuditSeverity represents audit event severity per AI.md PART 11 lines 11998-12005
type AuditSeverity string

const (
	AuditSeverityInfo     AuditSeverity = "info"     // Successful normal operations
	AuditSeverityWarning  AuditSeverity = "warn"     // Failed attempts, recoverable issues
	AuditSeverityError    AuditSeverity = "error"    // Failures requiring attention
	AuditSeverityCritical AuditSeverity = "critical" // Security incidents, server failures
)

// AuditActor represents who performed the action per AI.md PART 11
type AuditActor struct {
	Type      string `json:"type,omitempty"`       // Actor type: admin, user, system
	ID        string `json:"id,omitempty"`         // Actor's user ID
	Username  string `json:"username,omitempty"`   // Actor's username (for display)
	IP        string `json:"ip"`                   // IP address
	UserAgent string `json:"user_agent,omitempty"` // User agent string
}

// AuditTarget represents what the action was performed on per AI.md PART 11
type AuditTarget struct {
	Type string `json:"type"`           // Target type (session, user, config, token, etc.)
	ID   string `json:"id,omitempty"`   // Target ID
	Name string `json:"name,omitempty"` // Target name
}

// AuditLogger logs administrative actions
type AuditLogger struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	entropy io.Reader
}

// AuditEntry represents an audit log entry per AI.md PART 11 lines 11947-11997
// Uses ULID format IDs: audit_01HQXYZ123ABC
type AuditEntry struct {
	ID       string        `json:"id"`                   // ULID format: audit_01HQXYZ...
	Time     time.Time     `json:"time"`                 // ISO 8601 timestamp with milliseconds, UTC
	Event    AuditAction   `json:"event"`                // Event type (e.g., admin.login)
	Category AuditCategory `json:"category"`             // Event category
	Severity AuditSeverity `json:"severity"`             // info, warn, error, critical
	Actor    AuditActor    `json:"actor"`                // Who performed the action
	Target   *AuditTarget  `json:"target,omitempty"`     // What was acted upon
	Details  map[string]interface{} `json:"details,omitempty"` // Event-specific details
	Result   string        `json:"result"`               // "success" or "failure"
	NodeID   string        `json:"node_id,omitempty"`    // Node ID (cluster mode)
	Reason   string        `json:"reason,omitempty"`     // Reason for action (if provided)
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(path string) *AuditLogger {
	l := &AuditLogger{
		path:    path,
		entropy: rand.Reader,
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

// generateAuditID generates a ULID-based audit ID per AI.md PART 25
// Format: audit_01HQXYZ123ABC
func (l *AuditLogger) generateAuditID() string {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), l.entropy)
	return "audit_" + id.String()
}

// Log logs an audit event per AI.md PART 11
func (l *AuditLogger) Log(entry AuditEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Generate ULID if not set
	if entry.ID == "" {
		entry.ID = l.generateAuditID()
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}

	// JSON format for audit logs (easy to parse and analyze)
	data, _ := json.Marshal(entry)

	if l.file != nil {
		l.file.WriteString(string(data) + "\n")
	}

	// Also print to stdout for visibility (pretty is OK for console per PART 11)
	status := strings.ToUpper(entry.Result)
	targetInfo := ""
	if entry.Target != nil {
		targetInfo = fmt.Sprintf(" target=%s:%s", entry.Target.Type, entry.Target.Name)
	}
	fmt.Printf("[AUDIT] %s [%s] %s actor=%s ip=%s%s\n",
		entry.ID, entry.Category, status, entry.Actor.Username, entry.Actor.IP, targetInfo)
}

// resultFromBool converts bool to result string per AI.md PART 11
func resultFromBool(success bool) string {
	if success {
		return "success"
	}
	return "failure"
}

// LogLogin logs a login attempt per AI.md PART 11 line 11776
func (l *AuditLogger) LogLogin(user, ip string, success bool) {
	severity := AuditSeverityInfo
	if !success {
		severity = AuditSeverityWarning
	}
	l.Log(AuditEntry{
		Event:    AuditActionLogin,
		Category: AuditCategoryAuth,
		Severity: severity,
		Actor:    AuditActor{Username: user, IP: ip},
		Result:   resultFromBool(success),
	})
}

// LogLogout logs a logout per AI.md PART 11 line 11777
func (l *AuditLogger) LogLogout(user, ip string) {
	l.Log(AuditEntry{
		Event:    AuditActionLogout,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Result:   "success",
	})
}

// LogConfigChange logs a configuration change per AI.md PART 11 line 11888
func (l *AuditLogger) LogConfigChange(user, ip, resource, details string) {
	l.Log(AuditEntry{
		Event:    AuditActionConfigChange,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "config", Name: resource},
		Details:  map[string]interface{}{"changed": details},
		Result:   "success",
	})
}

// LogEngineToggle logs an engine enable/disable
func (l *AuditLogger) LogEngineToggle(user, ip, engine string, enabled bool) {
	action := "disabled"
	if enabled {
		action = "enabled"
	}
	l.Log(AuditEntry{
		Event:    AuditActionEngineToggle,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "engine", Name: engine},
		Details:  map[string]interface{}{"action": action},
		Result:   "success",
	})
}

// LogTokenCreate logs a token creation per AI.md PART 11 line 11916
func (l *AuditLogger) LogTokenCreate(user, ip, tokenName string) {
	l.Log(AuditEntry{
		Event:    AuditActionTokenCreate,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "token", Name: tokenName},
		Result:   "success",
	})
}

// LogTokenRevoke logs a token revocation per AI.md PART 11 line 11917
func (l *AuditLogger) LogTokenRevoke(user, ip, tokenName string) {
	l.Log(AuditEntry{
		Event:    AuditActionTokenRevoke,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "token", Name: tokenName},
		Result:   "success",
	})
}

// LogReload logs a configuration reload
func (l *AuditLogger) LogReload(user, ip string, success bool, details string) {
	severity := AuditSeverityInfo
	if !success {
		severity = AuditSeverityWarning
	}
	l.Log(AuditEntry{
		Event:    AuditActionReload,
		Category: AuditCategorySystem,
		Severity: severity,
		Actor:    AuditActor{Username: user, IP: ip},
		Details:  map[string]interface{}{"info": details},
		Result:   resultFromBool(success),
	})
}

// LogUserCreate logs a user creation per AI.md PART 11 line 11796
func (l *AuditLogger) LogUserCreate(actor, ip, targetUser string) {
	l.Log(AuditEntry{
		Event:    AuditActionUserCreate,
		Category: AuditCategoryUser,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "user", Name: targetUser},
		Result:   "success",
	})
}

// LogUserDelete logs a user deletion per AI.md PART 11 line 11797
func (l *AuditLogger) LogUserDelete(actor, ip, targetUser string) {
	l.Log(AuditEntry{
		Event:    AuditActionUserDelete,
		Category: AuditCategoryUser,
		Severity: AuditSeverityCritical,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "user", Name: targetUser},
		Result:   "success",
	})
}

// LogBackupCreate logs a backup creation per AI.md PART 11 line 11925
func (l *AuditLogger) LogBackupCreate(user, ip, filename string) {
	l.Log(AuditEntry{
		Event:    AuditActionBackupCreate,
		Category: AuditCategoryData,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "backup", Name: filename},
		Result:   "success",
	})
}

// LogBackupRestore logs a backup restoration per AI.md PART 11 line 11926
func (l *AuditLogger) LogBackupRestore(user, ip, filename string, success bool) {
	severity := AuditSeverityInfo
	if !success {
		severity = AuditSeverityCritical
	}
	l.Log(AuditEntry{
		Event:    AuditActionBackupRestore,
		Category: AuditCategoryData,
		Severity: severity,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "backup", Name: filename},
		Result:   resultFromBool(success),
	})
}

// Log2FAEnable logs 2FA enablement per AI.md PART 11 line 11805
func (l *AuditLogger) Log2FAEnable(user, ip string) {
	l.Log(AuditEntry{
		Event:    AuditActionMFAEnabled,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Details:  map[string]interface{}{"method": "totp"},
		Result:   "success",
	})
}

// Log2FADisable logs 2FA disablement per AI.md PART 11 line 11806
func (l *AuditLogger) Log2FADisable(actor, ip, targetUser string) {
	l.Log(AuditEntry{
		Event:    AuditActionMFADisabled,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityCritical,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "user", Name: targetUser},
		Details:  map[string]interface{}{"method": "totp"},
		Result:   "success",
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

// ============================================================
// Audit Query and Export Functions per AI.md PART 11
// ============================================================

// AuditQueryOptions defines filtering options for querying audit logs
type AuditQueryOptions struct {
	// Filter by category (authentication, admin, configuration, etc.)
	Category AuditCategory
	// Filter by event type (admin.login, user.created, etc.)
	Event AuditAction
	// Filter by actor username
	ActorUsername string
	// Filter by actor IP
	ActorIP string
	// Filter by target type
	TargetType string
	// Filter by target name
	TargetName string
	// Filter by result (success/failure)
	Result string
	// Filter by severity
	Severity AuditSeverity
	// Start time for range query (inclusive)
	StartTime time.Time
	// End time for range query (inclusive)
	EndTime time.Time
	// Maximum number of entries to return (0 = unlimited)
	Limit int
	// Number of entries to skip (for pagination)
	Offset int
}

// AuditQueryResult contains the results of an audit log query
type AuditQueryResult struct {
	// Total number of matching entries (before limit/offset)
	Total int `json:"total"`
	// Number of entries returned
	Count int `json:"count"`
	// The audit entries
	Entries []AuditEntry `json:"entries"`
	// Query duration in milliseconds
	DurationMs int64 `json:"duration_ms"`
}

// AuditExportFormat defines the export format
type AuditExportFormat string

const (
	AuditExportJSON AuditExportFormat = "json"
	AuditExportCSV  AuditExportFormat = "csv"
)

// QueryAuditLogs queries audit logs with filtering options
// Reads the audit log file and returns matching entries
func (l *AuditLogger) QueryAuditLogs(opts AuditQueryOptions) (*AuditQueryResult, error) {
	start := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Open the log file for reading
	file, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AuditQueryResult{
				Total:      0,
				Count:      0,
				Entries:    []AuditEntry{},
				DurationMs: time.Since(start).Milliseconds(),
			}, nil
		}
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}
	defer file.Close()

	var allMatching []AuditEntry
	scanner := NewLineScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if matchesAuditFilter(entry, opts) {
			allMatching = append(allMatching, entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading audit log: %w", err)
	}

	// Sort by time descending (newest first)
	sort.Slice(allMatching, func(i, j int) bool {
		return allMatching[i].Time.After(allMatching[j].Time)
	})

	total := len(allMatching)

	// Apply offset
	if opts.Offset > 0 && opts.Offset < len(allMatching) {
		allMatching = allMatching[opts.Offset:]
	} else if opts.Offset >= len(allMatching) {
		allMatching = []AuditEntry{}
	}

	// Apply limit
	if opts.Limit > 0 && opts.Limit < len(allMatching) {
		allMatching = allMatching[:opts.Limit]
	}

	return &AuditQueryResult{
		Total:      total,
		Count:      len(allMatching),
		Entries:    allMatching,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// matchesAuditFilter checks if an entry matches the query options
func matchesAuditFilter(entry AuditEntry, opts AuditQueryOptions) bool {
	// Filter by category
	if opts.Category != "" && entry.Category != opts.Category {
		return false
	}

	// Filter by event
	if opts.Event != "" && entry.Event != opts.Event {
		return false
	}

	// Filter by actor username
	if opts.ActorUsername != "" && entry.Actor.Username != opts.ActorUsername {
		return false
	}

	// Filter by actor IP
	if opts.ActorIP != "" && entry.Actor.IP != opts.ActorIP {
		return false
	}

	// Filter by target type
	if opts.TargetType != "" {
		if entry.Target == nil || entry.Target.Type != opts.TargetType {
			return false
		}
	}

	// Filter by target name
	if opts.TargetName != "" {
		if entry.Target == nil || entry.Target.Name != opts.TargetName {
			return false
		}
	}

	// Filter by result
	if opts.Result != "" && entry.Result != opts.Result {
		return false
	}

	// Filter by severity
	if opts.Severity != "" && entry.Severity != opts.Severity {
		return false
	}

	// Filter by start time
	if !opts.StartTime.IsZero() && entry.Time.Before(opts.StartTime) {
		return false
	}

	// Filter by end time
	if !opts.EndTime.IsZero() && entry.Time.After(opts.EndTime) {
		return false
	}

	return true
}

// ExportAuditLogs exports audit logs to the specified format
func (l *AuditLogger) ExportAuditLogs(opts AuditQueryOptions, format AuditExportFormat, w io.Writer) error {
	result, err := l.QueryAuditLogs(opts)
	if err != nil {
		return err
	}

	switch format {
	case AuditExportJSON:
		return exportAuditJSON(result.Entries, w)
	case AuditExportCSV:
		return exportAuditCSV(result.Entries, w)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

// exportAuditJSON exports audit entries to JSON format
func exportAuditJSON(entries []AuditEntry, w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(entries)
}

// exportAuditCSV exports audit entries to CSV format
func exportAuditCSV(entries []AuditEntry, w io.Writer) error {
	// Write CSV header
	header := "id,time,event,category,severity,actor_username,actor_ip,target_type,target_name,result,reason\n"
	if _, err := w.Write([]byte(header)); err != nil {
		return err
	}

	for _, e := range entries {
		targetType := ""
		targetName := ""
		if e.Target != nil {
			targetType = e.Target.Type
			targetName = e.Target.Name
		}

		line := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s\n",
			csvEscape(e.ID),
			e.Time.Format(time.RFC3339),
			csvEscape(string(e.Event)),
			csvEscape(string(e.Category)),
			csvEscape(string(e.Severity)),
			csvEscape(e.Actor.Username),
			csvEscape(e.Actor.IP),
			csvEscape(targetType),
			csvEscape(targetName),
			csvEscape(e.Result),
			csvEscape(e.Reason),
		)
		if _, err := w.Write([]byte(line)); err != nil {
			return err
		}
	}

	return nil
}

// csvEscape escapes a string for CSV output
func csvEscape(s string) string {
	if strings.ContainsAny(s, ",\"\n\r") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// LineScanner wraps bufio.Scanner for reading lines
type LineScanner struct {
	scanner *bufio.Scanner
}

// NewLineScanner creates a new line scanner
func NewLineScanner(r io.Reader) *LineScanner {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for potentially long JSON lines
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	return &LineScanner{scanner: scanner}
}

// Scan advances the scanner
func (s *LineScanner) Scan() bool {
	return s.scanner.Scan()
}

// Text returns the current line
func (s *LineScanner) Text() string {
	return s.scanner.Text()
}

// Err returns any scanning error
func (s *LineScanner) Err() error {
	return s.scanner.Err()
}

// ============================================================
// Additional Audit Logging Helper Methods per AI.md PART 11
// ============================================================

// LogEvent logs a generic audit event with full control over all fields
func (l *AuditLogger) LogEvent(event AuditAction, category AuditCategory, severity AuditSeverity, actor AuditActor, target *AuditTarget, result string, details map[string]interface{}, reason string) {
	l.Log(AuditEntry{
		Event:    event,
		Category: category,
		Severity: severity,
		Actor:    actor,
		Target:   target,
		Result:   result,
		Details:  details,
		Reason:   reason,
	})
}

// LogLoginFailed logs a failed login attempt
func (l *AuditLogger) LogLoginFailed(user, ip, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionLoginFailed,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{Username: user, IP: ip},
		Result:   "failure",
		Reason:   reason,
	})
}

// LogAdminCreated logs admin account creation
func (l *AuditLogger) LogAdminCreated(actor, ip, newAdmin string) {
	l.Log(AuditEntry{
		Event:    AuditActionAdminCreated,
		Category: AuditCategoryAdmin,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "admin", Name: newAdmin},
		Result:   "success",
	})
}

// LogAdminDeleted logs admin account deletion
func (l *AuditLogger) LogAdminDeleted(actor, ip, deletedAdmin string) {
	l.Log(AuditEntry{
		Event:    AuditActionAdminDeleted,
		Category: AuditCategoryAdmin,
		Severity: AuditSeverityCritical,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "admin", Name: deletedAdmin},
		Result:   "success",
	})
}

// LogPasswordChanged logs password change
func (l *AuditLogger) LogPasswordChanged(user, ip string, selfChange bool) {
	target := &AuditTarget{Type: "user", Name: user}
	if !selfChange {
		target = &AuditTarget{Type: "admin", Name: user}
	}
	l.Log(AuditEntry{
		Event:    AuditActionPasswordChanged,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   target,
		Result:   "success",
		Details:  map[string]interface{}{"self_change": selfChange},
	})
}

// LogTokenRegenerated logs API token regeneration
func (l *AuditLogger) LogTokenRegenerated(user, ip, tokenName string) {
	l.Log(AuditEntry{
		Event:    AuditActionTokenRegenerated,
		Category: AuditCategoryTokens,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "token", Name: tokenName},
		Result:   "success",
	})
}

// LogSessionExpired logs session expiration
func (l *AuditLogger) LogSessionExpired(user, ip, sessionID string) {
	l.Log(AuditEntry{
		Event:    AuditActionSessionExpired,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: user, IP: ip},
		Target:   &AuditTarget{Type: "session", ID: sessionID},
		Result:   "success",
	})
}

// LogSessionRevoked logs session revocation
func (l *AuditLogger) LogSessionRevoked(actor, ip, targetUser, sessionID string) {
	l.Log(AuditEntry{
		Event:    AuditActionSessionRevoked,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "session", ID: sessionID, Name: targetUser},
		Result:   "success",
	})
}

// LogUserSuspended logs user suspension
func (l *AuditLogger) LogUserSuspended(actor, ip, targetUser, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionUserSuspended,
		Category: AuditCategoryUser,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "user", Name: targetUser},
		Result:   "success",
		Reason:   reason,
	})
}

// LogUserUnsuspended logs user unsuspension
func (l *AuditLogger) LogUserUnsuspended(actor, ip, targetUser string) {
	l.Log(AuditEntry{
		Event:    AuditActionUserUnsuspended,
		Category: AuditCategoryUser,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "user", Name: targetUser},
		Result:   "success",
	})
}

// LogUserRoleChanged logs user role change
func (l *AuditLogger) LogUserRoleChanged(actor, ip, targetUser, oldRole, newRole string) {
	l.Log(AuditEntry{
		Event:    AuditActionUserRoleChanged,
		Category: AuditCategoryUser,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "user", Name: targetUser},
		Result:   "success",
		Details:  map[string]interface{}{"old_role": oldRole, "new_role": newRole},
	})
}

// LogIPBlocked logs IP blocking
func (l *AuditLogger) LogIPBlocked(actor, ip, blockedIP, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionIPBlocked,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "ip", Name: blockedIP},
		Result:   "success",
		Reason:   reason,
	})
}

// LogIPUnblocked logs IP unblocking
func (l *AuditLogger) LogIPUnblocked(actor, ip, unblockedIP string) {
	l.Log(AuditEntry{
		Event:    AuditActionIPUnblocked,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "ip", Name: unblockedIP},
		Result:   "success",
	})
}

// LogRateLimitExceeded logs rate limit exceeded
func (l *AuditLogger) LogRateLimitExceeded(ip, path string, limit int) {
	l.Log(AuditEntry{
		Event:    AuditActionRateLimitExceeded,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{IP: ip},
		Target:   &AuditTarget{Type: "endpoint", Name: path},
		Result:   "blocked",
		Details:  map[string]interface{}{"limit": limit},
	})
}

// LogBruteForceDetected logs brute force detection
func (l *AuditLogger) LogBruteForceDetected(ip, path string, attempts int) {
	l.Log(AuditEntry{
		Event:    AuditActionBruteForceDetected,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityCritical,
		Actor:    AuditActor{IP: ip},
		Target:   &AuditTarget{Type: "endpoint", Name: path},
		Result:   "detected",
		Details:  map[string]interface{}{"attempts": attempts},
	})
}

// LogCSRFFailure logs CSRF token failure
func (l *AuditLogger) LogCSRFFailure(ip, path string) {
	l.Log(AuditEntry{
		Event:    AuditActionCSRFFailure,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{IP: ip},
		Target:   &AuditTarget{Type: "endpoint", Name: path},
		Result:   "failure",
	})
}

// LogInvalidToken logs invalid token usage
func (l *AuditLogger) LogInvalidToken(ip, path, tokenType string) {
	l.Log(AuditEntry{
		Event:    AuditActionInvalidToken,
		Category: AuditCategorySecurity,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{IP: ip},
		Target:   &AuditTarget{Type: "endpoint", Name: path},
		Result:   "failure",
		Details:  map[string]interface{}{"token_type": tokenType},
	})
}

// LogBackupDelete logs backup deletion
func (l *AuditLogger) LogBackupDelete(actor, ip, filename string) {
	l.Log(AuditEntry{
		Event:    AuditActionBackupDelete,
		Category: AuditCategoryData,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "backup", Name: filename},
		Result:   "success",
	})
}

// LogBackupFailed logs backup failure
func (l *AuditLogger) LogBackupFailed(actor, ip, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionBackupFailed,
		Category: AuditCategoryData,
		Severity: AuditSeverityError,
		Actor:    AuditActor{Username: actor, IP: ip},
		Result:   "failure",
		Reason:   reason,
	})
}

// LogServerStarted logs server startup
func (l *AuditLogger) LogServerStarted(version, nodeID string) {
	l.Log(AuditEntry{
		Event:    AuditActionServerStarted,
		Category: AuditCategorySystem,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Type: "system", Username: "system"},
		Result:   "success",
		Details:  map[string]interface{}{"version": version},
		NodeID:   nodeID,
	})
}

// LogServerStopped logs server shutdown
func (l *AuditLogger) LogServerStopped(reason, nodeID string) {
	l.Log(AuditEntry{
		Event:    AuditActionServerStopped,
		Category: AuditCategorySystem,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Type: "system", Username: "system"},
		Result:   "success",
		Reason:   reason,
		NodeID:   nodeID,
	})
}

// LogMaintenanceEntered logs entering maintenance mode
func (l *AuditLogger) LogMaintenanceEntered(actor, ip, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionMaintenanceEntered,
		Category: AuditCategorySystem,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{Username: actor, IP: ip},
		Result:   "success",
		Reason:   reason,
	})
}

// LogMaintenanceExited logs exiting maintenance mode
func (l *AuditLogger) LogMaintenanceExited(actor, ip string) {
	l.Log(AuditEntry{
		Event:    AuditActionMaintenanceExited,
		Category: AuditCategorySystem,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Result:   "success",
	})
}

// LogConfigUpdated logs configuration update with specific field
func (l *AuditLogger) LogConfigUpdated(actor, ip, configSection, field string, oldValue, newValue interface{}) {
	l.Log(AuditEntry{
		Event:    AuditActionConfigChange,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "config", Name: configSection + "." + field},
		Result:   "success",
		Details:  map[string]interface{}{"field": field, "old_value": oldValue, "new_value": newValue},
	})
}

// LogSMTPUpdated logs SMTP configuration update
func (l *AuditLogger) LogSMTPUpdated(actor, ip string) {
	l.Log(AuditEntry{
		Event:    AuditActionConfigSMTPUpdated,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "config", Name: "smtp"},
		Result:   "success",
	})
}

// LogSSLUpdated logs SSL configuration update
func (l *AuditLogger) LogSSLUpdated(actor, ip, domain string) {
	l.Log(AuditEntry{
		Event:    AuditActionConfigSSLUpdated,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "ssl", Name: domain},
		Result:   "success",
	})
}

// LogClusterNodeJoined logs node joining cluster
func (l *AuditLogger) LogClusterNodeJoined(nodeID, nodeAddress string) {
	l.Log(AuditEntry{
		Event:    AuditActionClusterNodeJoined,
		Category: AuditCategoryCluster,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Type: "system", Username: "system"},
		Target:   &AuditTarget{Type: "node", ID: nodeID, Name: nodeAddress},
		Result:   "success",
		NodeID:   nodeID,
	})
}

// LogClusterNodeRemoved logs node removal from cluster
func (l *AuditLogger) LogClusterNodeRemoved(actor, ip, nodeID, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionClusterNodeRemoved,
		Category: AuditCategoryCluster,
		Severity: AuditSeverityWarning,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "node", ID: nodeID},
		Result:   "success",
		Reason:   reason,
	})
}

// LogClusterNodeFailed logs node failure in cluster
func (l *AuditLogger) LogClusterNodeFailed(nodeID, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionClusterNodeFailed,
		Category: AuditCategoryCluster,
		Severity: AuditSeverityCritical,
		Actor:    AuditActor{Type: "system", Username: "system"},
		Target:   &AuditTarget{Type: "node", ID: nodeID},
		Result:   "failure",
		Reason:   reason,
	})
}

// LogSchedulerTaskFailed logs scheduler task failure
func (l *AuditLogger) LogSchedulerTaskFailed(taskName, reason string) {
	l.Log(AuditEntry{
		Event:    AuditActionSchedulerTaskFail,
		Category: AuditCategorySystem,
		Severity: AuditSeverityError,
		Actor:    AuditActor{Type: "system", Username: "scheduler"},
		Target:   &AuditTarget{Type: "task", Name: taskName},
		Result:   "failure",
		Reason:   reason,
	})
}

// LogSchedulerTaskManualRun logs manual scheduler task execution
func (l *AuditLogger) LogSchedulerTaskManualRun(actor, ip, taskName string, success bool) {
	severity := AuditSeverityInfo
	if !success {
		severity = AuditSeverityError
	}
	l.Log(AuditEntry{
		Event:    AuditActionSchedulerTaskRun,
		Category: AuditCategorySystem,
		Severity: severity,
		Actor:    AuditActor{Username: actor, IP: ip},
		Target:   &AuditTarget{Type: "task", Name: taskName},
		Result:   resultFromBool(success),
	})
}

// ============================================================
// Audit Log Retention and Cleanup per AI.md PART 11
// ============================================================

// AuditRetentionPolicy defines the retention policy for audit logs
type AuditRetentionPolicy struct {
	// Maximum age of audit entries (0 = no limit)
	MaxAge time.Duration
	// Maximum number of entries to keep (0 = no limit)
	MaxEntries int
	// Keep critical events regardless of age
	PreserveCritical bool
}

// DefaultAuditRetentionPolicy returns the default retention policy
// 90 days retention, no entry limit, preserve critical events
func DefaultAuditRetentionPolicy() AuditRetentionPolicy {
	return AuditRetentionPolicy{
		MaxAge:           90 * 24 * time.Hour,
		MaxEntries:       0,
		PreserveCritical: true,
	}
}

// CleanupAuditLogs removes old audit entries based on retention policy
// Returns the number of entries removed
func (l *AuditLogger) CleanupAuditLogs(policy AuditRetentionPolicy) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Read all entries
	file, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to open audit log: %w", err)
	}

	var entries []AuditEntry
	scanner := NewLineScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	file.Close()

	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("error reading audit log: %w", err)
	}

	cutoff := time.Now().Add(-policy.MaxAge)
	var keepEntries []AuditEntry

	for _, e := range entries {
		keep := false

		// Keep if within retention period
		if policy.MaxAge == 0 || e.Time.After(cutoff) {
			keep = true
		}

		// Keep critical events if policy says so
		if policy.PreserveCritical && e.Severity == AuditSeverityCritical {
			keep = true
		}

		if keep {
			keepEntries = append(keepEntries, e)
		}
	}

	// Sort by time (oldest first for proper log ordering)
	sort.Slice(keepEntries, func(i, j int) bool {
		return keepEntries[i].Time.Before(keepEntries[j].Time)
	})

	// Apply max entries limit (keep newest, but always preserve critical if policy says so)
	if policy.MaxEntries > 0 && len(keepEntries) > policy.MaxEntries {
		if policy.PreserveCritical {
			// Separate critical entries from non-critical
			var criticalEntries, nonCriticalEntries []AuditEntry
			for _, e := range keepEntries {
				if e.Severity == AuditSeverityCritical {
					criticalEntries = append(criticalEntries, e)
				} else {
					nonCriticalEntries = append(nonCriticalEntries, e)
				}
			}
			// Keep all critical plus as many non-critical as possible up to limit
			remaining := policy.MaxEntries - len(criticalEntries)
			if remaining <= 0 {
				// No room for non-critical entries, keep only critical
				nonCriticalEntries = nil
			} else if len(nonCriticalEntries) > remaining {
				// Keep only the newest non-critical entries up to remaining
				nonCriticalEntries = nonCriticalEntries[len(nonCriticalEntries)-remaining:]
			}
			keepEntries = append(criticalEntries, nonCriticalEntries...)
			// Re-sort by time
			sort.Slice(keepEntries, func(i, j int) bool {
				return keepEntries[i].Time.Before(keepEntries[j].Time)
			})
		} else {
			keepEntries = keepEntries[len(keepEntries)-policy.MaxEntries:]
		}
	}

	removed := len(entries) - len(keepEntries)

	// Close current file before rewriting
	if l.file != nil {
		l.file.Close()
	}

	// Rewrite the file with kept entries
	file, err = os.Create(l.path)
	if err != nil {
		return 0, fmt.Errorf("failed to rewrite audit log: %w", err)
	}

	for _, e := range keepEntries {
		data, _ := json.Marshal(e)
		file.WriteString(string(data) + "\n")
	}
	file.Close()

	// Reopen for appending
	l.openFile()

	return removed, nil
}

// GetAuditStats returns statistics about the audit log
func (l *AuditLogger) GetAuditStats() (*AuditStats, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	stats := &AuditStats{
		ByCategory: make(map[AuditCategory]int),
		BySeverity: make(map[AuditSeverity]int),
		ByResult:   make(map[string]int),
	}

	file, err := os.Open(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, nil
		}
		return nil, fmt.Errorf("failed to open audit log: %w", err)
	}
	defer file.Close()

	scanner := NewLineScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry AuditEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		stats.TotalEntries++
		stats.ByCategory[entry.Category]++
		stats.BySeverity[entry.Severity]++
		stats.ByResult[entry.Result]++

		if stats.OldestEntry.IsZero() || entry.Time.Before(stats.OldestEntry) {
			stats.OldestEntry = entry.Time
		}
		if entry.Time.After(stats.NewestEntry) {
			stats.NewestEntry = entry.Time
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading audit log: %w", err)
	}

	return stats, nil
}

// AuditStats contains audit log statistics
type AuditStats struct {
	TotalEntries int                      `json:"total_entries"`
	OldestEntry  time.Time                `json:"oldest_entry"`
	NewestEntry  time.Time                `json:"newest_entry"`
	ByCategory   map[AuditCategory]int    `json:"by_category"`
	BySeverity   map[AuditSeverity]int    `json:"by_severity"`
	ByResult     map[string]int           `json:"by_result"`
}

// ============================================================
// Error Logger - Error messages only per PART 21
// ============================================================

// ErrorLogger logs error messages to error.log
type ErrorLogger struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	format string // "text", "json"
	stdout bool
}

// ErrorEntry represents an error log entry
type ErrorEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Error     string                 `json:"error,omitempty"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Stack     string                 `json:"stack,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// NewErrorLogger creates a new error logger
func NewErrorLogger(path string) *ErrorLogger {
	l := &ErrorLogger{
		path:   path,
		format: "text",
		stdout: true,
	}
	l.openFile()
	return l
}

func (l *ErrorLogger) openFile() error {
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

// SetFormat sets the log format
func (l *ErrorLogger) SetFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format
}

// SetStdout enables/disables stdout output
func (l *ErrorLogger) SetStdout(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stdout = enabled
}

// Log logs an error entry
func (l *ErrorLogger) Log(entry ErrorEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Level == "" {
		entry.Level = "ERROR"
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
		if entry.Error != "" {
			line += fmt.Sprintf(" error=%s", entry.Error)
		}
		if entry.File != "" {
			line += fmt.Sprintf(" (%s:%d)", entry.File, entry.Line)
		}
		if len(entry.Fields) > 0 {
			for k, v := range entry.Fields {
				line += fmt.Sprintf(" %s=%v", k, v)
			}
		}
	}

	if l.file != nil {
		l.file.WriteString(line + "\n")
	}
	if l.stdout {
		fmt.Println("[ERROR] " + line)
	}
}

// Error logs an error message
func (l *ErrorLogger) Error(msg string, err error, fields ...map[string]interface{}) {
	entry := ErrorEntry{
		Timestamp: time.Now(),
		Level:     "ERROR",
		Message:   msg,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

// ErrorWithStack logs an error with stack trace
func (l *ErrorLogger) ErrorWithStack(msg string, err error, stack string, fields ...map[string]interface{}) {
	entry := ErrorEntry{
		Timestamp: time.Now(),
		Level:     "ERROR",
		Message:   msg,
		Stack:     stack,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

// Fatal logs a fatal error
func (l *ErrorLogger) Fatal(msg string, err error, fields ...map[string]interface{}) {
	entry := ErrorEntry{
		Timestamp: time.Now(),
		Level:     "FATAL",
		Message:   msg,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

// Rotate rotates the error log file
func (l *ErrorLogger) Rotate() error {
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

// Close closes the error logger
func (l *ErrorLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// ============================================================
// Debug Logger - Debug messages (dev mode only) per PART 21
// ============================================================

// DebugLogger logs debug messages to debug.log
type DebugLogger struct {
	mu      sync.Mutex
	file    *os.File
	path    string
	format  string // "text", "json"
	enabled bool
	stdout  bool
}

// DebugEntry represents a debug log entry
type DebugEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	File      string                 `json:"file,omitempty"`
	Line      int                    `json:"line,omitempty"`
	Function  string                 `json:"function,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// NewDebugLogger creates a new debug logger
func NewDebugLogger(path string) *DebugLogger {
	l := &DebugLogger{
		path:    path,
		format:  "text",
		enabled: false, // Disabled by default per PART 21
		stdout:  true,
	}
	// Don't open file if disabled
	return l
}

func (l *DebugLogger) openFile() error {
	if l.path == "" || !l.enabled {
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

// Enable enables the debug logger and opens the file
func (l *DebugLogger) Enable() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = true
	l.openFile()
}

// Disable disables the debug logger and closes the file
func (l *DebugLogger) Disable() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.enabled = false
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// IsEnabled returns whether debug logging is enabled
func (l *DebugLogger) IsEnabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enabled
}

// SetFormat sets the log format
func (l *DebugLogger) SetFormat(format string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.format = format
}

// SetStdout enables/disables stdout output
func (l *DebugLogger) SetStdout(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.stdout = enabled
}

// Log logs a debug entry
func (l *DebugLogger) Log(entry DebugEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.enabled {
		return
	}

	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now()
	}
	if entry.Level == "" {
		entry.Level = "DEBUG"
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
		if entry.File != "" {
			line += fmt.Sprintf(" (%s:%d)", entry.File, entry.Line)
		}
		if entry.Function != "" {
			line += fmt.Sprintf(" func=%s", entry.Function)
		}
		if len(entry.Fields) > 0 {
			for k, v := range entry.Fields {
				line += fmt.Sprintf(" %s=%v", k, v)
			}
		}
	}

	if l.file != nil {
		l.file.WriteString(line + "\n")
	}
	if l.stdout {
		fmt.Println("[DEBUG] " + line)
	}
}

// Debug logs a debug message
func (l *DebugLogger) Debug(msg string, fields ...map[string]interface{}) {
	if !l.enabled {
		return
	}
	entry := DebugEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Message:   msg,
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

// DebugWithCaller logs a debug message with caller info
func (l *DebugLogger) DebugWithCaller(msg string, file string, line int, function string, fields ...map[string]interface{}) {
	if !l.enabled {
		return
	}
	entry := DebugEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Message:   msg,
		File:      file,
		Line:      line,
		Function:  function,
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

// Trace logs a trace-level debug message
func (l *DebugLogger) Trace(msg string, fields ...map[string]interface{}) {
	if !l.enabled {
		return
	}
	entry := DebugEntry{
		Timestamp: time.Now(),
		Level:     "TRACE",
		Message:   msg,
	}
	if len(fields) > 0 {
		entry.Fields = fields[0]
	}
	l.Log(entry)
}

// Rotate rotates the debug log file
func (l *DebugLogger) Rotate() error {
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

// Close closes the debug logger
func (l *DebugLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
