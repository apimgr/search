package logging

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)
	defer m.Close()

	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.Access() == nil {
		t.Error("Access() returned nil")
	}
	if m.Server() == nil {
		t.Error("Server() returned nil")
	}
	if m.Error() == nil {
		t.Error("Error() returned nil")
	}
	if m.Security() == nil {
		t.Error("Security() returned nil")
	}
	if m.Audit() == nil {
		t.Error("Audit() returned nil")
	}
	if m.Debug() == nil {
		t.Error("Debug() returned nil")
	}
}

func TestNewManagerDefaultDir(t *testing.T) {
	m := NewManager("")
	defer m.Close()

	if m == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestManagerClose(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	err := m.Close()
	if err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func TestManagerRotateAll(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)
	defer m.Close()

	// Write some logs first
	m.Server().Info("test message")

	err := m.RotateAll()
	if err != nil {
		t.Errorf("RotateAll() returned error: %v", err)
	}
}

func TestLogTypeConstants(t *testing.T) {
	tests := []struct {
		logType LogType
		want    string
	}{
		{LogTypeAccess, "access"},
		{LogTypeServer, "server"},
		{LogTypeError, "error"},
		{LogTypeSecurity, "security"},
		{LogTypeAudit, "audit"},
		{LogTypeDebug, "debug"},
	}

	for _, tt := range tests {
		t.Run(string(tt.logType), func(t *testing.T) {
			if string(tt.logType) != tt.want {
				t.Errorf("LogType = %q, want %q", tt.logType, tt.want)
			}
		})
	}
}

func TestFormatVariables(t *testing.T) {
	if len(AvailableFormatVariables) == 0 {
		t.Error("AvailableFormatVariables should not be empty")
	}

	// Check some expected variables exist
	found := make(map[string]bool)
	for _, v := range AvailableFormatVariables {
		found[v.Name] = true
		if v.Name == "" {
			t.Error("Variable name should not be empty")
		}
		if v.Description == "" {
			t.Errorf("Variable %s should have a description", v.Name)
		}
	}

	expectedVars := []string{"$remote_addr", "$request", "$status", "$http_user_agent"}
	for _, expected := range expectedVars {
		if !found[expected] {
			t.Errorf("Expected format variable %s not found", expected)
		}
	}
}

func TestAccessLogger(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "access.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	entry := AccessEntry{
		Timestamp: time.Now(),
		IP:        "192.168.1.1",
		Method:    "GET",
		Path:      "/test",
		Protocol:  "HTTP/1.1",
		Status:    200,
		Size:      1234,
		UserAgent: "TestAgent/1.0",
	}

	logger.Log(entry)

	// Verify file was written
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "192.168.1.1") {
		t.Error("Log should contain IP address")
	}
}

func TestAccessLoggerFormats(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []string{"combined", "common", "json"}
	for _, format := range tests {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(tmpDir, format+".log")
			logger := NewAccessLogger(path)
			logger.SetFormat(format)
			defer logger.Close()

			entry := AccessEntry{
				Timestamp: time.Now(),
				IP:        "10.0.0.1",
				Method:    "POST",
				Path:      "/api",
				Protocol:  "HTTP/2.0",
				Status:    201,
				Size:      500,
			}
			logger.Log(entry)

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("Failed to read log file: %v", err)
			}
			if len(content) == 0 {
				t.Error("Log file should not be empty")
			}
		})
	}
}

func TestAccessLoggerCustomFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "custom.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	customFormat := "$remote_addr - $status - $request_time_ms"
	logger.SetCustomFormat(customFormat)

	if logger.GetCustomFormat() != customFormat {
		t.Errorf("GetCustomFormat() = %q, want %q", logger.GetCustomFormat(), customFormat)
	}

	entry := AccessEntry{
		Timestamp: time.Now(),
		IP:        "1.2.3.4",
		Status:    200,
		Latency:   123,
	}
	logger.Log(entry)
}

func TestAccessLoggerLogRequest(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "request.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	req := &http.Request{
		Method:     "GET",
		URL:        &url.URL{Path: "/test", RawQuery: "q=search"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"User-Agent": []string{"TestAgent/1.0"}},
		RemoteAddr: "192.168.1.1:12345",
		Host:       "example.com",
	}

	logger.LogRequest(req, 200, 1024, 50*time.Millisecond)
}

func TestAccessLoggerRotate(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "rotate.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	logger.Log(AccessEntry{IP: "1.1.1.1", Status: 200})

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestValidateFormat(t *testing.T) {
	tests := []struct {
		format  string
		wantLen int
	}{
		{"$remote_addr - $status", 0},          // Valid
		{"$unknown_var", 1},                     // Unknown
		{"$remote_addr $unknown $status", 1},    // One unknown
		{"plain text", 0},                       // No variables
	}

	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			unknown := ValidateFormat(tt.format)
			if len(unknown) != tt.wantLen {
				t.Errorf("ValidateFormat(%q) returned %d unknown vars, want %d", tt.format, len(unknown), tt.wantLen)
			}
		})
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level LogLevel
		want  string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{LevelFatal, "FATAL"},
		{LogLevel(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.level.String()
			if got != tt.want {
				t.Errorf("LogLevel(%d).String() = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestServerLogger(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "server.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false) // Disable stdout for tests
	defer logger.Close()

	logger.Info("test info message")
	logger.Warn("test warning")
	logger.Error("test error")
	logger.Debug("test debug") // Shouldn't appear (level is INFO)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "INFO") {
		t.Error("Log should contain INFO")
	}
}

func TestServerLoggerLevels(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "levels.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	logger.SetLevel(LevelDebug)
	defer logger.Close()

	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warn message")
	logger.Error("error message")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "DEBUG") {
		t.Error("Log should contain DEBUG")
	}
	if !strings.Contains(logContent, "INFO") {
		t.Error("Log should contain INFO")
	}
}

func TestServerLoggerJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "json.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	logger.SetFormat("json")
	defer logger.Close()

	logger.Info("test message", map[string]interface{}{"key": "value"})

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), `"level":"INFO"`) {
		t.Error("JSON log should contain level field")
	}
}

func TestServerLoggerWriter(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "writer.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	writer := logger.Writer()
	writer.Write([]byte("test via writer"))

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "test via writer") {
		t.Error("Log should contain message written via Writer")
	}
}

func TestSecurityEventConstants(t *testing.T) {
	events := []SecurityEvent{
		SecurityEventLoginFailed,
		SecurityEventLoginSuccess,
		SecurityEventRateLimited,
		SecurityEventBlocked,
		SecurityEventSuspicious,
		SecurityEventBruteForce,
		SecurityEventInvalidToken,
		SecurityEventCSRFViolation,
	}

	for _, event := range events {
		if string(event) == "" {
			t.Error("Security event should not be empty")
		}
	}
}

func TestSecurityLogger(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "security.log")
	logger := NewSecurityLogger(path)
	defer logger.Close()

	logger.LogLoginFailed("192.168.1.1", "testuser", "/login")
	logger.LogLoginSuccess("192.168.1.1", "testuser")
	logger.LogRateLimited("192.168.1.1", "/api")
	logger.LogBlocked("192.168.1.1", "/admin", "IP blocked")
	logger.LogSuspicious("192.168.1.1", "/etc/passwd", "path traversal attempt")
	logger.LogBruteForce("192.168.1.1", "/login", 10)
	logger.LogInvalidToken("192.168.1.1", "/api")
	logger.LogCSRFViolation("192.168.1.1", "/form")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "LOGIN_FAILED") {
		t.Error("Log should contain LOGIN_FAILED")
	}
	if !strings.Contains(logContent, "RATE_LIMITED") {
		t.Error("Log should contain RATE_LIMITED")
	}
}

func TestAuditActionConstants(t *testing.T) {
	// Test a sample of audit actions
	actions := []AuditAction{
		AuditActionLogin,
		AuditActionLogout,
		AuditActionConfigChange,
		AuditActionUserCreate,
		AuditActionBackupCreate,
	}

	for _, action := range actions {
		if string(action) == "" {
			t.Error("Audit action should not be empty")
		}
	}
}

func TestAuditCategoryConstants(t *testing.T) {
	categories := []AuditCategory{
		AuditCategoryAuth,
		AuditCategoryAdmin,
		AuditCategoryConfig,
		AuditCategoryUser,
		AuditCategorySecurity,
		AuditCategoryData,
		AuditCategorySystem,
	}

	for _, cat := range categories {
		if string(cat) == "" {
			t.Error("Audit category should not be empty")
		}
	}
}

func TestAuditSeverityConstants(t *testing.T) {
	severities := []AuditSeverity{
		AuditSeverityInfo,
		AuditSeverityWarning,
		AuditSeverityError,
		AuditSeverityCritical,
	}

	expected := []string{"info", "warn", "error", "critical"}
	for i, sev := range severities {
		if string(sev) != expected[i] {
			t.Errorf("AuditSeverity = %q, want %q", sev, expected[i])
		}
	}
}

func TestAuditLogger(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogLogin("admin", "192.168.1.1", true)
	logger.LogLogin("baduser", "10.0.0.1", false)
	logger.LogLogout("admin", "192.168.1.1")
	logger.LogConfigChange("admin", "192.168.1.1", "engines", "enabled google")
	logger.LogEngineToggle("admin", "192.168.1.1", "google", true)
	logger.LogTokenCreate("admin", "192.168.1.1", "api-token-1")
	logger.LogTokenRevoke("admin", "192.168.1.1", "api-token-1")
	logger.LogReload("admin", "192.168.1.1", true, "config reloaded")
	logger.LogUserCreate("admin", "192.168.1.1", "newuser")
	logger.LogUserDelete("admin", "192.168.1.1", "olduser")
	logger.LogBackupCreate("admin", "192.168.1.1", "backup.tar.gz")
	logger.LogBackupRestore("admin", "192.168.1.1", "backup.tar.gz", true)
	logger.Log2FAEnable("admin", "192.168.1.1")
	logger.Log2FADisable("admin", "192.168.1.1", "testuser")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "admin.login") {
		t.Error("Log should contain admin.login event")
	}
}

func TestAuditLoggerGenerateID(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	id := logger.generateAuditID()
	if !strings.HasPrefix(id, "audit_") {
		t.Errorf("Audit ID %q should have prefix 'audit_'", id)
	}
	if len(id) < 10 {
		t.Errorf("Audit ID %q seems too short", id)
	}
}

func TestAuditEntryStruct(t *testing.T) {
	entry := AuditEntry{
		ID:       "audit_test123",
		Time:     time.Now(),
		Event:    AuditActionLogin,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityInfo,
		Actor: AuditActor{
			Username: "admin",
			IP:       "192.168.1.1",
		},
		Result: "success",
	}

	if entry.ID != "audit_test123" {
		t.Error("Entry ID should be set")
	}
	if entry.Event != AuditActionLogin {
		t.Error("Entry Event should be set")
	}
}

func TestErrorLogger(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error.log")
	logger := NewErrorLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	testErr := &testError{msg: "test error"}
	logger.Error("something went wrong", testErr)
	logger.ErrorWithStack("stack error", testErr, "stack trace here")
	logger.Fatal("fatal error", nil) // Won't exit in test

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "ERROR") {
		t.Error("Log should contain ERROR")
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestErrorLoggerJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error-json.log")
	logger := NewErrorLogger(path)
	logger.SetStdout(false)
	logger.SetFormat("json")
	defer logger.Close()

	logger.Error("json error", nil, map[string]interface{}{"code": 500})

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), `"level":"ERROR"`) {
		t.Error("JSON log should contain level field")
	}
}

func TestDebugLogger(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	// Debug logger is disabled by default
	if logger.IsEnabled() {
		t.Error("Debug logger should be disabled by default")
	}

	logger.Debug("should not appear")

	logger.Enable()
	if !logger.IsEnabled() {
		t.Error("Debug logger should be enabled after Enable()")
	}

	logger.Debug("should appear")
	logger.Trace("trace message")
	logger.DebugWithCaller("caller debug", "test.go", 100, "TestDebugLogger")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "should appear") {
		t.Error("Log should contain debug message after enabling")
	}

	logger.Disable()
	if logger.IsEnabled() {
		t.Error("Debug logger should be disabled after Disable()")
	}
}

func TestDebugLoggerJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-json.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	logger.Enable()
	logger.SetFormat("json")
	defer logger.Close()

	logger.Debug("json debug", map[string]interface{}{"key": "value"})

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), `"level":"DEBUG"`) {
		t.Error("JSON log should contain level field")
	}
}

func TestOrDash(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "-"},
		{"value", "value"},
		{"-", "-"},
	}

	for _, tt := range tests {
		got := orDash(tt.input)
		if got != tt.want {
			t.Errorf("orDash(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGetHostname(t *testing.T) {
	hostname := getHostname()
	if hostname == "" {
		t.Error("getHostname() should not return empty string")
	}
}

func TestEstimateHeaderSize(t *testing.T) {
	size := estimateHeaderSize(200)
	if size <= 0 {
		t.Errorf("estimateHeaderSize(200) = %d, should be positive", size)
	}
}

func TestTLSCipherSuiteName(t *testing.T) {
	tests := []struct {
		id   uint16
		want string
	}{
		{0x1301, "TLS_AES_128_GCM_SHA256"},
		{0x1302, "TLS_AES_256_GCM_SHA384"},
		{0x9999, "0x9999"}, // Unknown cipher
	}

	for _, tt := range tests {
		got := tlsCipherSuiteName(tt.id)
		if got != tt.want {
			t.Errorf("tlsCipherSuiteName(0x%04x) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

// ============================================================
// Additional tests for 100% coverage
// ============================================================

func TestAccessLoggerLogRequestWithID(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "request-id.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	req := &http.Request{
		Method:     "POST",
		URL:        &url.URL{Path: "/api/v1", RawQuery: "key=value"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{"User-Agent": []string{"TestAgent/2.0"}},
		RemoteAddr: "10.0.0.1:54321",
		Host:       "api.example.com",
	}

	logger.LogRequestWithID(req, 201, 2048, 100*time.Millisecond, "req-12345")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "10.0.0.1") {
		t.Error("Log should contain IP address")
	}
}

func TestAccessLoggerLogRequestWithXForwardedFor(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xff.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	req := &http.Request{
		Method:     "GET",
		URL:        &url.URL{Path: "/"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"X-Forwarded-For": []string{"203.0.113.1, 70.41.3.18"},
		},
		RemoteAddr: "127.0.0.1:8080",
	}

	logger.LogRequest(req, 200, 100, 10*time.Millisecond)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "203.0.113.1") {
		t.Error("Log should contain X-Forwarded-For IP")
	}
}

func TestAccessLoggerLogRequestWithXRealIP(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "xrealip.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	req := &http.Request{
		Method:     "GET",
		URL:        &url.URL{Path: "/"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			// Use canonical form (X-Real-Ip not X-Real-IP) for http.Header map keys
			"X-Real-Ip": []string{"198.51.100.1"},
		},
		RemoteAddr: "127.0.0.1:8080",
	}

	logger.LogRequest(req, 200, 100, 10*time.Millisecond)
	logger.LogRequestWithID(req, 200, 100, 10*time.Millisecond, "req-abc")

	// Ensure file is flushed
	logger.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "198.51.100.1") {
		t.Error("Log should contain X-Real-IP")
	}
}

func TestAccessLoggerLogRequestWithTLS(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "tls.log")
	logger := NewAccessLogger(path)
	logger.SetFormat("json")
	defer logger.Close()

	// Test all TLS versions
	tlsVersions := []struct {
		version  uint16
		expected string
	}{
		{0x0301, "TLSv1.0"},
		{0x0302, "TLSv1.1"},
		{0x0303, "TLSv1.2"},
		{0x0304, "TLSv1.3"},
	}

	for _, tv := range tlsVersions {
		req := &http.Request{
			Method:     "GET",
			URL:        &url.URL{Path: "/secure"},
			Proto:      "HTTP/2.0",
			ProtoMajor: 2,
			ProtoMinor: 0,
			Header:     http.Header{},
			RemoteAddr: "10.0.0.1:443",
			TLS: &tls.ConnectionState{
				Version:     tv.version,
				CipherSuite: 0x1301,
			},
		}
		logger.LogRequest(req, 200, 100, 5*time.Millisecond)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "TLSv1.3") {
		t.Error("Log should contain TLS version")
	}
}

func TestAccessLoggerLogRequestWithIDAndTLS(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "tls-id.log")
	logger := NewAccessLogger(path)
	logger.SetFormat("json")
	defer logger.Close()

	// Test all TLS versions with LogRequestWithID
	tlsVersions := []uint16{0x0301, 0x0302, 0x0303, 0x0304}

	for i, tv := range tlsVersions {
		req := &http.Request{
			Method:     "GET",
			URL:        &url.URL{Path: "/secure"},
			Proto:      "HTTP/2.0",
			ProtoMajor: 2,
			ProtoMinor: 0,
			Header: http.Header{
				"X-Forwarded-For": []string{"1.2.3.4"},
			},
			RemoteAddr: "10.0.0.1:443",
			TLS: &tls.ConnectionState{
				Version:     tv,
				CipherSuite: 0xc02f,
			},
		}
		logger.LogRequestWithID(req, 200, 100, 5*time.Millisecond, "req-"+string(rune('a'+i)))
	}
}

func TestAccessLoggerIPv6Handling(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "ipv6.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	// Test IPv6 address with port (should NOT strip the bracket part)
	req := &http.Request{
		Method:     "GET",
		URL:        &url.URL{Path: "/"},
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{},
		RemoteAddr: "[::1]:8080",
	}

	logger.LogRequest(req, 200, 100, 10*time.Millisecond)
	logger.LogRequestWithID(req, 200, 100, 10*time.Millisecond, "req-ipv6")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	// IPv6 address should be present (the port stripping logic handles IPv6)
	if len(content) == 0 {
		t.Error("Log file should not be empty")
	}
}

func TestAccessLoggerFormatWithVariablesEmptyCustomFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty-custom.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	// Set format to custom but leave customFormat empty - should fallback to combined
	logger.SetFormat("custom")
	// Note: customFormat is empty by default

	entry := AccessEntry{
		Timestamp: time.Now(),
		IP:        "5.6.7.8",
		Method:    "DELETE",
		Path:      "/resource",
		Protocol:  "HTTP/1.1",
		Status:    204,
		Size:      0,
		Referer:   "https://example.com",
		UserAgent: "CustomAgent/1.0",
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "5.6.7.8") {
		t.Error("Log should contain IP in fallback format")
	}
}

func TestAccessLoggerFormatWithVariablesAllVariables(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "all-vars.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	// Test comprehensive custom format with all variables
	customFormat := "$remote_addr $remote_user [$time_local] [$time_iso8601] [$time_unix] [$time_msec] \"$request\" " +
		"$request_method $request_uri $request_path $query_string $status $body_bytes_sent $bytes_sent " +
		"\"$http_referer\" \"$http_user_agent\" $http_host $http_x_forwarded_for $http_x_real_ip " +
		"$server_protocol $request_time $request_time_ms $request_id $connection $connection_requests " +
		"$ssl_protocol $ssl_cipher $hostname $pid"
	logger.SetCustomFormat(customFormat)

	entry := AccessEntry{
		Timestamp:          time.Now(),
		IP:                 "192.168.1.100",
		Method:             "PUT",
		Path:               "/api/resource",
		QueryString:        "id=123&action=update",
		Protocol:           "HTTP/2.0",
		Status:             200,
		Size:               1024,
		BytesSent:          1224,
		Referer:            "https://app.example.com/",
		UserAgent:          "Mozilla/5.0",
		Latency:            250,
		RequestID:          "uuid-12345",
		RemoteUser:         "admin",
		Host:               "api.example.com",
		XForwardedFor:      "10.0.0.1",
		XRealIP:            "10.0.0.2",
		SSLProtocol:        "TLSv1.3",
		SSLCipher:          "TLS_AES_256_GCM_SHA384",
		Connection:         999,
		ConnectionRequests: 5,
	}
	logger.Log(entry)

	// Ensure file is flushed
	logger.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "192.168.1.100") {
		t.Error("Log should contain IP")
	}
	if !strings.Contains(logContent, "uuid-12345") {
		t.Error("Log should contain request ID")
	}
	if !strings.Contains(logContent, "admin") {
		t.Error("Log should contain remote user")
	}
}

func TestAccessLoggerRotateWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "rotate-test.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	logger.Log(AccessEntry{IP: "1.1.1.1", Status: 200})

	// Create the rotated file that would be created by date
	timestamp := time.Now().Format("20060102")
	rotatedPath := path + "." + timestamp
	os.WriteFile(rotatedPath, []byte("existing"), 0644)

	// Now rotate - should create a file with time suffix
	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}

	// Check that a new rotated file was created with time suffix
	files, _ := filepath.Glob(path + ".*")
	if len(files) < 2 {
		t.Errorf("Expected at least 2 rotated files, got %d", len(files))
	}
}

func TestAccessLoggerEmptyPath(t *testing.T) {
	logger := NewAccessLogger("")
	defer logger.Close()

	// Should not panic when logging with no file
	logger.Log(AccessEntry{IP: "1.1.1.1", Status: 200})
}

func TestAccessLoggerRotateNilFile(t *testing.T) {
	logger := NewAccessLogger("")
	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() with nil file returned error: %v", err)
	}
	logger.Close()
}

func TestAccessLoggerCloseNilFile(t *testing.T) {
	logger := NewAccessLogger("")
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() with nil file returned error: %v", err)
	}
}

func TestAccessLoggerLogWithRefererEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "referer.log")
	logger := NewAccessLogger(path)
	defer logger.Close()

	// Combined format with empty referer should use "-"
	entry := AccessEntry{
		Timestamp: time.Now(),
		IP:        "1.2.3.4",
		Method:    "GET",
		Path:      "/",
		Protocol:  "HTTP/1.1",
		Status:    200,
		Size:      100,
		Referer:   "", // Empty referer
		UserAgent: "Agent",
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), `"-"`) {
		t.Error("Empty referer should be logged as dash")
	}
}

func TestServerLoggerRotate(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "server-rotate.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	logger.Info("test message")

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestServerLoggerRotateWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "server-rotate2.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	logger.Info("test message")

	// Create the rotated file
	timestamp := time.Now().Format("20060102")
	rotatedPath := path + "." + timestamp
	os.WriteFile(rotatedPath, []byte("existing"), 0644)

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestServerLoggerRotateNilFile(t *testing.T) {
	logger := NewServerLogger("")
	logger.SetStdout(false)
	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() with nil file returned error: %v", err)
	}
	logger.Close()
}

func TestServerLoggerCloseNilFile(t *testing.T) {
	logger := NewServerLogger("")
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() with nil file returned error: %v", err)
	}
}

func TestServerLoggerEmptyPath(t *testing.T) {
	logger := NewServerLogger("")
	logger.SetStdout(false)
	defer logger.Close()

	// Should not panic
	logger.Info("test")
}

func TestServerLoggerTextFormatWithFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "server-fields.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	logger.SetFormat("text")
	defer logger.Close()

	fields := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}
	logger.Info("message with fields", fields)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "key1=value1") {
		t.Error("Log should contain field key1")
	}
}

func TestServerLoggerLevelFiltering(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "server-level.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	logger.SetLevel(LevelWarn) // Only WARN and above
	defer logger.Close()

	logger.Debug("debug")
	logger.Info("info")
	logger.Warn("warn")
	logger.Error("error")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if strings.Contains(logContent, "debug") {
		t.Error("DEBUG should not be logged at WARN level")
	}
	if strings.Contains(logContent, "[INFO]") {
		t.Error("INFO should not be logged at WARN level")
	}
	if !strings.Contains(logContent, "WARN") {
		t.Error("WARN should be logged")
	}
}

func TestServerLoggerWriterEmptyMessage(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "server-writer-empty.log")
	logger := NewServerLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	writer := logger.Writer()

	// Write empty string (after trim should be empty)
	n, err := writer.Write([]byte("   \n\t  "))
	if err != nil {
		t.Errorf("Write returned error: %v", err)
	}
	if n != 7 {
		t.Errorf("Write returned %d, expected 7", n)
	}

	// Write non-empty
	writer.Write([]byte("valid message"))
}

func TestSecurityLoggerRotate(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "security-rotate.log")
	logger := NewSecurityLogger(path)
	defer logger.Close()

	logger.LogLoginFailed("1.1.1.1", "user", "/login")

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestSecurityLoggerRotateWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "security-rotate2.log")
	logger := NewSecurityLogger(path)
	defer logger.Close()

	logger.LogLoginFailed("1.1.1.1", "user", "/login")

	// Create the rotated file
	timestamp := time.Now().Format("20060102")
	rotatedPath := path + "." + timestamp
	os.WriteFile(rotatedPath, []byte("existing"), 0644)

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestSecurityLoggerRotateNilFile(t *testing.T) {
	logger := NewSecurityLogger("")
	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() with nil file returned error: %v", err)
	}
	logger.Close()
}

func TestSecurityLoggerCloseNilFile(t *testing.T) {
	logger := NewSecurityLogger("")
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() with nil file returned error: %v", err)
	}
}

func TestSecurityLoggerEmptyPath(t *testing.T) {
	logger := NewSecurityLogger("")
	defer logger.Close()

	// Should not panic
	logger.LogLoginFailed("1.1.1.1", "user", "/login")
}

func TestSecurityLoggerLogEntryDirectly(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "security-direct.log")
	logger := NewSecurityLogger(path)
	defer logger.Close()

	entry := SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventSuspicious,
		IP:        "192.168.1.1",
		User:      "suspect",
		Path:      "/admin",
		Details:   "unusual activity",
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "USER=suspect") {
		t.Error("Log should contain USER field")
	}
	if !strings.Contains(logContent, "PATH=/admin") {
		t.Error("Log should contain PATH field")
	}
	if !strings.Contains(logContent, "DETAILS=unusual activity") {
		t.Error("Log should contain DETAILS field")
	}
}

func TestAuditLoggerLogWithPresetIDAndTime(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit-preset.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	presetTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	entry := AuditEntry{
		ID:       "audit_PRESET123456",
		Time:     presetTime,
		Event:    AuditActionLogin,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: "admin", IP: "10.0.0.1"},
		Result:   "success",
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "audit_PRESET123456") {
		t.Error("Log should contain preset ID")
	}
	if !strings.Contains(logContent, "2024-01-15") {
		t.Error("Log should contain preset time")
	}
}

func TestAuditLoggerLogWithoutTarget(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit-notarget.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	entry := AuditEntry{
		Event:    AuditActionLogout,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: "user", IP: "1.2.3.4"},
		Result:   "success",
		Target:   nil, // No target
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if len(content) == 0 {
		t.Error("Log should not be empty")
	}
}

func TestAuditLoggerRotate(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit-rotate.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogLogin("admin", "1.1.1.1", true)

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestAuditLoggerRotateWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit-rotate2.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogLogin("admin", "1.1.1.1", true)

	// Create the rotated file
	timestamp := time.Now().Format("20060102")
	rotatedPath := path + "." + timestamp
	os.WriteFile(rotatedPath, []byte("existing"), 0644)

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestAuditLoggerRotateNilFile(t *testing.T) {
	logger := NewAuditLogger("")
	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() with nil file returned error: %v", err)
	}
	logger.Close()
}

func TestAuditLoggerCloseNilFile(t *testing.T) {
	logger := NewAuditLogger("")
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() with nil file returned error: %v", err)
	}
}

func TestAuditLoggerEmptyPath(t *testing.T) {
	logger := NewAuditLogger("")
	defer logger.Close()

	// Should not panic
	logger.LogLogin("admin", "1.1.1.1", true)
}

func TestResultFromBool(t *testing.T) {
	if resultFromBool(true) != "success" {
		t.Error("resultFromBool(true) should return 'success'")
	}
	if resultFromBool(false) != "failure" {
		t.Error("resultFromBool(false) should return 'failure'")
	}
}

func TestAuditLoggerBackupRestoreFailure(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit-backup-fail.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogBackupRestore("admin", "1.1.1.1", "backup.tar.gz", false)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "failure") {
		t.Error("Log should contain failure result")
	}
	if !strings.Contains(logContent, "critical") {
		t.Error("Failed backup restore should have critical severity")
	}
}

func TestAuditLoggerReloadFailure(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit-reload-fail.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogReload("admin", "1.1.1.1", false, "config error")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "failure") {
		t.Error("Log should contain failure result")
	}
}

func TestErrorLoggerRotate(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error-rotate.log")
	logger := NewErrorLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	logger.Error("test error", nil)

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestErrorLoggerRotateWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error-rotate2.log")
	logger := NewErrorLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	logger.Error("test error", nil)

	// Create the rotated file
	timestamp := time.Now().Format("20060102")
	rotatedPath := path + "." + timestamp
	os.WriteFile(rotatedPath, []byte("existing"), 0644)

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestErrorLoggerRotateNilFile(t *testing.T) {
	logger := NewErrorLogger("")
	logger.SetStdout(false)
	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() with nil file returned error: %v", err)
	}
	logger.Close()
}

func TestErrorLoggerCloseNilFile(t *testing.T) {
	logger := NewErrorLogger("")
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() with nil file returned error: %v", err)
	}
}

func TestErrorLoggerEmptyPath(t *testing.T) {
	logger := NewErrorLogger("")
	logger.SetStdout(false)
	defer logger.Close()

	// Should not panic
	logger.Error("test", nil)
}

func TestErrorLoggerLogEntryDirectly(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error-direct.log")
	logger := NewErrorLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	entry := ErrorEntry{
		Timestamp: time.Now(),
		Level:     "ERROR",
		Message:   "direct error",
		Error:     "some error",
		File:      "test.go",
		Line:      100,
		Stack:     "stack trace here",
		Fields:    map[string]interface{}{"key": "value"},
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "test.go:100") {
		t.Error("Log should contain file and line")
	}
	if !strings.Contains(logContent, "key=value") {
		t.Error("Log should contain fields")
	}
}

func TestErrorLoggerLogEntryDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error-defaults.log")
	logger := NewErrorLogger(path)
	logger.SetStdout(false)
	defer logger.Close()

	// Entry with empty Level and zero Timestamp - should use defaults
	entry := ErrorEntry{
		Message: "default entry",
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "ERROR") {
		t.Error("Default level should be ERROR")
	}
}

func TestErrorLoggerTextFormatWithAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "error-allfields.log")
	logger := NewErrorLogger(path)
	logger.SetStdout(false)
	logger.SetFormat("text")
	defer logger.Close()

	testErr := &testError{msg: "underlying error"}
	fields := map[string]interface{}{"code": 500, "service": "api"}
	logger.Error("main error", testErr, fields)
	logger.ErrorWithStack("stack error", testErr, "full stack trace", fields)
	logger.Fatal("fatal error", testErr, fields)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "error=underlying error") {
		t.Error("Log should contain error message")
	}
	if !strings.Contains(logContent, "FATAL") {
		t.Error("Log should contain FATAL level")
	}
}

func TestDebugLoggerRotate(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-rotate.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	logger.Enable()
	defer logger.Close()

	logger.Debug("test debug")

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestDebugLoggerRotateWithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-rotate2.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	logger.Enable()
	defer logger.Close()

	logger.Debug("test debug")

	// Create the rotated file
	timestamp := time.Now().Format("20060102")
	rotatedPath := path + "." + timestamp
	os.WriteFile(rotatedPath, []byte("existing"), 0644)

	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() returned error: %v", err)
	}
}

func TestDebugLoggerRotateNilFile(t *testing.T) {
	logger := NewDebugLogger("")
	logger.SetStdout(false)
	err := logger.Rotate()
	if err != nil {
		t.Errorf("Rotate() with nil file returned error: %v", err)
	}
	logger.Close()
}

func TestDebugLoggerCloseNilFile(t *testing.T) {
	logger := NewDebugLogger("")
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() with nil file returned error: %v", err)
	}
}

func TestDebugLoggerEmptyPath(t *testing.T) {
	logger := NewDebugLogger("")
	logger.SetStdout(false)
	logger.Enable()
	defer logger.Close()

	// Should not panic
	logger.Debug("test")
}

func TestDebugLoggerLogEntryDirectly(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-direct.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	logger.Enable()
	defer logger.Close()

	entry := DebugEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Message:   "direct debug",
		File:      "debug_test.go",
		Line:      50,
		Function:  "TestDebugLoggerLogEntryDirectly",
		Fields:    map[string]interface{}{"key": "value"},
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "debug_test.go:50") {
		t.Error("Log should contain file and line")
	}
	if !strings.Contains(logContent, "func=TestDebugLoggerLogEntryDirectly") {
		t.Error("Log should contain function name")
	}
}

func TestDebugLoggerLogEntryDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-defaults.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	logger.Enable()
	defer logger.Close()

	// Entry with empty Level and zero Timestamp - should use defaults
	entry := DebugEntry{
		Message: "default entry",
	}
	logger.Log(entry)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "DEBUG") {
		t.Error("Default level should be DEBUG")
	}
}

func TestDebugLoggerTextFormatWithAllFields(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-allfields.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	logger.Enable()
	logger.SetFormat("text")
	defer logger.Close()

	fields := map[string]interface{}{"module": "test", "count": 42}
	logger.Debug("debug with fields", fields)
	logger.DebugWithCaller("caller debug", "file.go", 123, "MyFunc", fields)
	logger.Trace("trace message", fields)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "module=test") {
		t.Error("Log should contain fields")
	}
	if !strings.Contains(logContent, "TRACE") {
		t.Error("Log should contain TRACE level")
	}
}

func TestDebugLoggerDisabledSkipsLogging(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-disabled.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	// NOT enabled - default is disabled
	defer logger.Close()

	logger.Debug("should not appear")
	logger.DebugWithCaller("should not appear", "file.go", 1, "Func")
	logger.Trace("should not appear")

	// File should not exist or be empty since logger never enabled
	_, err := os.Stat(path)
	if err == nil {
		content, _ := os.ReadFile(path)
		if len(content) > 0 {
			t.Error("Disabled debug logger should not write anything")
		}
	}
}

func TestDebugLoggerJSONFormatWithCallerInfo(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-json-caller.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	logger.Enable()
	logger.SetFormat("json")
	defer logger.Close()

	logger.DebugWithCaller("json caller debug", "myfile.go", 999, "TestFunction", map[string]interface{}{"data": "test"})

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, `"file":"myfile.go"`) {
		t.Error("JSON log should contain file field")
	}
	if !strings.Contains(logContent, `"line":999`) {
		t.Error("JSON log should contain line field")
	}
	if !strings.Contains(logContent, `"function":"TestFunction"`) {
		t.Error("JSON log should contain function field")
	}
}

func TestDebugLoggerOpenFileWhenDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "debug-openfile.log")
	logger := NewDebugLogger(path)
	logger.SetStdout(false)
	// Disable is default, call openFile directly (via Enable/Disable cycle)
	defer logger.Close()

	// openFile should return nil when disabled
	if logger.file != nil {
		t.Error("File should not be opened when disabled")
	}
}

func TestAllTLSCipherSuites(t *testing.T) {
	ciphers := []struct {
		id   uint16
		want string
	}{
		{0x1301, "TLS_AES_128_GCM_SHA256"},
		{0x1302, "TLS_AES_256_GCM_SHA384"},
		{0x1303, "TLS_CHACHA20_POLY1305_SHA256"},
		{0xc02f, "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"},
		{0xc030, "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
		{0xcca8, "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305"},
		{0xc02b, "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"},
		{0xc02c, "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384"},
		{0xcca9, "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305"},
	}

	for _, c := range ciphers {
		got := tlsCipherSuiteName(c.id)
		if got != c.want {
			t.Errorf("tlsCipherSuiteName(0x%04x) = %q, want %q", c.id, got, c.want)
		}
	}
}

func TestAuditCategoryTokensAndCluster(t *testing.T) {
	// Test additional audit categories
	categories := []struct {
		cat  AuditCategory
		want string
	}{
		{AuditCategoryTokens, "tokens"},
		{AuditCategoryCluster, "cluster"},
		{AuditCategoryOrganization, "organization"},
	}

	for _, c := range categories {
		if string(c.cat) != c.want {
			t.Errorf("AuditCategory = %q, want %q", c.cat, c.want)
		}
	}
}

func TestAuditActorStruct(t *testing.T) {
	actor := AuditActor{
		Type:      "admin",
		ID:        "user-123",
		Username:  "admin",
		IP:        "192.168.1.1",
		UserAgent: "Mozilla/5.0",
	}

	if actor.Type != "admin" {
		t.Error("Actor type should be set")
	}
	if actor.ID != "user-123" {
		t.Error("Actor ID should be set")
	}
	if actor.UserAgent != "Mozilla/5.0" {
		t.Error("Actor UserAgent should be set")
	}
}

func TestAuditTargetStruct(t *testing.T) {
	target := AuditTarget{
		Type: "user",
		ID:   "target-456",
		Name: "testuser",
	}

	if target.Type != "user" {
		t.Error("Target type should be set")
	}
	if target.ID != "target-456" {
		t.Error("Target ID should be set")
	}
	if target.Name != "testuser" {
		t.Error("Target name should be set")
	}
}

func TestAuditEntryWithAllFields(t *testing.T) {
	entry := AuditEntry{
		ID:       "audit_test",
		Time:     time.Now(),
		Event:    AuditActionConfigChange,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityCritical,
		Actor: AuditActor{
			Type:      "admin",
			ID:        "admin-1",
			Username:  "superadmin",
			IP:        "10.0.0.1",
			UserAgent: "AdminClient/1.0",
		},
		Target: &AuditTarget{
			Type: "config",
			ID:   "config-main",
			Name: "main_config",
		},
		Details: map[string]interface{}{
			"changed":  "ssl_enabled",
			"old":      false,
			"new":      true,
		},
		Result: "success",
		NodeID: "node-1",
		Reason: "security hardening",
	}

	if entry.NodeID != "node-1" {
		t.Error("NodeID should be set")
	}
	if entry.Reason != "security hardening" {
		t.Error("Reason should be set")
	}
}

func TestMoreAuditActions(t *testing.T) {
	// Test additional audit actions defined in the package
	actions := []struct {
		action AuditAction
		want   string
	}{
		{AuditActionLoginFailed, "admin.login_failed"},
		{AuditActionAdminCreated, "admin.created"},
		{AuditActionAdminDeleted, "admin.deleted"},
		{AuditActionPasswordChanged, "admin.password_changed"},
		{AuditActionMFAEnabled, "admin.mfa_enabled"},
		{AuditActionMFADisabled, "admin.mfa_disabled"},
		{AuditActionTokenRegenerated, "admin.token_regenerated"},
		{AuditActionSessionExpired, "admin.session_expired"},
		{AuditActionSessionRevoked, "admin.session_revoked"},
		{AuditActionUserRegistered, "user.registered"},
		{AuditActionUserLogin, "user.login"},
		{AuditActionUserLogout, "user.logout"},
		{AuditActionUserLoginFailed, "user.login_failed"},
		{AuditActionUserSuspended, "user.suspended"},
		{AuditActionUserUnsuspended, "user.unsuspended"},
		{AuditActionUserRoleChanged, "user.role_changed"},
		{AuditActionUserPasswordChanged, "user.password_changed"},
		{AuditActionUserPasswordResetReq, "user.password_reset_requested"},
		{AuditActionUserPasswordResetDone, "user.password_reset_completed"},
		{AuditActionUserEmailVerified, "user.email_verified"},
		{AuditActionUserMFAEnabled, "user.mfa_enabled"},
		{AuditActionUserMFADisabled, "user.mfa_disabled"},
		{AuditActionUserRecoveryKeyUsed, "user.recovery_key_used"},
		{AuditActionOrgCreated, "org.created"},
		{AuditActionOrgDeleted, "org.deleted"},
		{AuditActionOrgSettingsUpdated, "org.settings_updated"},
		{AuditActionOrgMemberInvited, "org.member_invited"},
		{AuditActionOrgMemberJoined, "org.member_joined"},
		{AuditActionOrgMemberRemoved, "org.member_removed"},
		{AuditActionOrgMemberLeft, "org.member_left"},
		{AuditActionOrgRoleChanged, "org.role_changed"},
		{AuditActionOrgRoleCreated, "org.role_created"},
		{AuditActionOrgRoleUpdated, "org.role_updated"},
		{AuditActionOrgRoleDeleted, "org.role_deleted"},
		{AuditActionOrgTokenCreated, "org.token_created"},
		{AuditActionOrgTokenRevoked, "org.token_revoked"},
		{AuditActionOrgOwnershipTransfer, "org.ownership_transferred"},
		{AuditActionOrgBillingUpdated, "org.billing_updated"},
		{AuditActionConfigSMTPUpdated, "config.smtp_updated"},
		{AuditActionConfigSSLUpdated, "config.ssl_updated"},
		{AuditActionConfigSSLExpired, "config.ssl_expired"},
		{AuditActionConfigTorRegen, "config.tor_address_regenerated"},
		{AuditActionConfigBrandingUpdate, "config.branding_updated"},
		{AuditActionConfigOIDCAdded, "config.oidc_provider_added"},
		{AuditActionConfigOIDCRemoved, "config.oidc_provider_removed"},
		{AuditActionConfigLDAPUpdated, "config.ldap_updated"},
		{AuditActionConfigAdminGroups, "config.admin_groups_updated"},
		{AuditActionRateLimitExceeded, "security.rate_limit_exceeded"},
		{AuditActionIPBlocked, "security.ip_blocked"},
		{AuditActionIPUnblocked, "security.ip_unblocked"},
		{AuditActionCountryBlocked, "security.country_blocked"},
		{AuditActionCSRFFailure, "security.csrf_failure"},
		{AuditActionInvalidToken, "security.invalid_token"},
		{AuditActionBruteForceDetected, "security.brute_force_detected"},
		{AuditActionSuspiciousActivity, "security.suspicious_activity"},
		{AuditActionTokenExpired, "token.expired"},
		{AuditActionTokenUsed, "token.used"},
		{AuditActionBackupDelete, "backup.deleted"},
		{AuditActionBackupFailed, "backup.failed"},
		{AuditActionServerStarted, "server.started"},
		{AuditActionServerStopped, "server.stopped"},
		{AuditActionMaintenanceEntered, "server.maintenance_entered"},
		{AuditActionMaintenanceExited, "server.maintenance_exited"},
		{AuditActionServerUpdated, "server.updated"},
		{AuditActionSchedulerTaskFail, "scheduler.task_failed"},
		{AuditActionSchedulerTaskRun, "scheduler.task_manual_run"},
		{AuditActionClusterNodeJoined, "cluster.node_joined"},
		{AuditActionClusterNodeRemoved, "cluster.node_removed"},
		{AuditActionClusterNodeFailed, "cluster.node_failed"},
		{AuditActionClusterTokenGen, "cluster.token_generated"},
		{AuditActionClusterModeChanged, "cluster.mode_changed"},
		{AuditActionPermissionChange, "user.permission_change"},
		{AuditActionAdminInvite, "admin.invite"},
	}

	for _, a := range actions {
		if string(a.action) != a.want {
			t.Errorf("AuditAction = %q, want %q", a.action, a.want)
		}
	}
}

func TestValidateFormatEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantLen int
	}{
		{"empty", "", 0},
		{"just dollar", "$", 0},
		{"dollar at end", "text$", 0},
		{"multiple unknowns", "$foo $bar $baz", 3},
		{"mixed valid invalid", "$remote_addr $invalid $status", 1},
		{"variable with numbers", "$var123", 1},
		{"consecutive dollars", "$$remote_addr", 0}, // First $ is skipped (no name), second is valid $remote_addr
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unknown := ValidateFormat(tt.format)
			if len(unknown) != tt.wantLen {
				t.Errorf("ValidateFormat(%q) returned %d unknown vars %v, want %d",
					tt.format, len(unknown), unknown, tt.wantLen)
			}
		})
	}
}

func TestAccessEntryStructFields(t *testing.T) {
	entry := AccessEntry{
		Timestamp:          time.Now(),
		IP:                 "1.2.3.4",
		Method:             "GET",
		Path:               "/api",
		QueryString:        "key=value",
		Protocol:           "HTTP/1.1",
		Status:             200,
		Size:               1024,
		BytesSent:          1224,
		Referer:            "http://example.com",
		UserAgent:          "TestAgent",
		Latency:            100,
		RequestID:          "req-123",
		RemoteUser:         "user",
		Host:               "api.example.com",
		XForwardedFor:      "10.0.0.1",
		XRealIP:            "10.0.0.2",
		SSLProtocol:        "TLSv1.3",
		SSLCipher:          "AES256",
		Connection:         1,
		ConnectionRequests: 5,
	}

	if entry.BytesSent != 1224 {
		t.Error("BytesSent should be set")
	}
	if entry.ConnectionRequests != 5 {
		t.Error("ConnectionRequests should be set")
	}
}

func TestServerEntryStruct(t *testing.T) {
	entry := ServerEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "test message",
		Fields:    map[string]interface{}{"key": "value"},
	}

	if entry.Level != "INFO" {
		t.Error("Level should be set")
	}
	if entry.Fields["key"] != "value" {
		t.Error("Fields should be set")
	}
}

func TestSecurityEntryStruct(t *testing.T) {
	entry := SecurityEntry{
		Timestamp: time.Now(),
		Event:     SecurityEventBlocked,
		IP:        "1.2.3.4",
		User:      "attacker",
		Path:      "/admin",
		Details:   "blocked",
	}

	if entry.Event != SecurityEventBlocked {
		t.Error("Event should be set")
	}
	if entry.Details != "blocked" {
		t.Error("Details should be set")
	}
}

func TestDebugEntryStruct(t *testing.T) {
	entry := DebugEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Message:   "debug message",
		File:      "file.go",
		Line:      100,
		Function:  "TestFunc",
		Fields:    map[string]interface{}{"data": 123},
	}

	if entry.Function != "TestFunc" {
		t.Error("Function should be set")
	}
	if entry.Fields["data"] != 123 {
		t.Error("Fields should be set")
	}
}

func TestErrorEntryStruct(t *testing.T) {
	entry := ErrorEntry{
		Timestamp: time.Now(),
		Level:     "ERROR",
		Message:   "error message",
		Error:     "underlying error",
		File:      "file.go",
		Line:      50,
		Stack:     "stack trace",
		Fields:    map[string]interface{}{"code": 500},
	}

	if entry.Stack != "stack trace" {
		t.Error("Stack should be set")
	}
	if entry.Error != "underlying error" {
		t.Error("Error should be set")
	}
}

func TestFormatVariableStruct(t *testing.T) {
	fv := FormatVariable{
		Name:        "$test_var",
		Description: "A test variable",
		Example:     "example_value",
	}

	if fv.Name != "$test_var" {
		t.Error("Name should be set")
	}
	if fv.Description != "A test variable" {
		t.Error("Description should be set")
	}
	if fv.Example != "example_value" {
		t.Error("Example should be set")
	}
}
