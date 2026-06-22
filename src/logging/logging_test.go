package logging

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
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

	// Privacy: $remote_addr, $http_user_agent and other identifying vars
	// are intentionally excluded from AvailableFormatVariables. Only check
	// that non-identifying vars are still present.
	expectedVars := []string{"$request", "$status", "$time_local", "$request_id"}
	for _, expected := range expectedVars {
		if !found[expected] {
			t.Errorf("Expected format variable %s not found", expected)
		}
	}

	// And explicitly assert the identifying vars are NOT present.
	forbiddenVars := []string{"$remote_addr", "$http_user_agent", "$http_referer", "$http_host", "$http_x_forwarded_for", "$http_x_real_ip", "$query_string"}
	for _, forbidden := range forbiddenVars {
		if found[forbidden] {
			t.Errorf("Privacy violation: %s must not be in AvailableFormatVariables", forbidden)
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
		// $remote_addr is no longer in AvailableFormatVariables (privacy),
		// so it is now reported as unknown alongside any other unknown var.
		{"$remote_addr - $status", 1},
		// Unknown
		{"$unknown_var", 1},
		// $remote_addr (privacy-excluded) + $unknown = 2 unknown.
		{"$remote_addr $unknown $status", 2},
		// No variables
		{"plain text", 0},
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
	// Disable stdout for tests
	logger.SetStdout(false)
	defer logger.Close()

	logger.Info("test info message")
	logger.Warn("test warning")
	logger.Error("test error")
	// Shouldn't appear (level is INFO)
	logger.Debug("test debug")

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
		AuditActionConfigChange,
		AuditActionRateLimitExceeded,
		AuditActionTokenCreate,
		AuditActionBackupCreate,
		AuditActionServerStarted,
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
		AuditCategoryConfig,
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

	logger.LogConfigChange("operator", "192.168.1.1", "engines", "enabled google")
	logger.LogTokenCreate("operator", "192.168.1.1", "api-token-1")
	logger.LogTokenRevoke("operator", "192.168.1.1", "api-token-1")
	logger.LogBackupCreate("operator", "192.168.1.1", "backup.tar.gz")
	logger.LogBackupRestore("operator", "192.168.1.1", "backup.tar.gz", true)
	logger.LogIPBlocked("operator", "192.168.1.1", "10.0.0.5", "brute force")
	logger.LogRateLimitExceeded("10.0.0.6", "/api/v1/search", 100)

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "config.updated") {
		t.Error("Log should contain config.updated event")
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
		Event:    AuditActionConfigChange,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor: AuditActor{
			Username: "operator",
			IP:       "192.168.1.1",
		},
		Result: "success",
	}

	if entry.ID != "audit_test123" {
		t.Error("Entry ID should be set")
	}
	if entry.Event != AuditActionConfigChange {
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
	// Won't exit in test
	logger.Fatal("fatal error", nil)

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
		// Unknown cipher
		{0x9999, "0x9999"},
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
	// Privacy: server-side logs must NEVER contain client IPs (CLAUDE.md rule #10).
	if strings.Contains(string(content), "10.0.0.1") {
		t.Error("Log must NOT contain IP address — privacy is the product")
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
	// Privacy: X-Forwarded-For IPs must never appear in server-side logs.
	if strings.Contains(string(content), "203.0.113.1") || strings.Contains(string(content), "70.41.3.18") {
		t.Error("Log must NOT contain X-Forwarded-For IP — privacy is the product")
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
	// Privacy: X-Real-IP must never appear in server-side logs.
	if strings.Contains(string(content), "198.51.100.1") {
		t.Error("Log must NOT contain X-Real-IP — privacy is the product")
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
	// Privacy: IP, query string, referer, user-agent, host, X-Forwarded-For,
	// X-Real-IP must NOT appear in the log even if requested in the format
	// string. The format-rendering layer must drop these identifying values.
	forbiddenFragments := []string{
		"192.168.1.100",
		"id=123",
		"app.example.com",
		"Mozilla/5.0",
		"api.example.com",
		"10.0.0.1",
		"10.0.0.2",
	}
	for _, frag := range forbiddenFragments {
		if strings.Contains(logContent, frag) {
			t.Errorf("Privacy violation: log contains %q", frag)
		}
	}
	// Non-identifying fields are still expected to render.
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
		// Empty referer
		Referer:   "",
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
	// Only WARN and above
	logger.SetLevel(LevelWarn)
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
		Event:    AuditActionConfigChange,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: "operator", IP: "10.0.0.1"},
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
		Event:    AuditActionRateLimitExceeded,
		Category: AuditCategoryAuth,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{IP: "1.2.3.4"},
		Result:   "blocked",
		// No target
		Target: nil,
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

	logger.LogConfigChange("operator", "1.1.1.1", "test", "init")

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

	logger.LogConfigChange("operator", "1.1.1.1", "test", "init")

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
	logger.LogConfigChange("operator", "1.1.1.1", "test", "init")
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

func TestAuditLoggerBackupFailure(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit-backup-fail2.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogBackupFailed("operator", "1.1.1.1", "disk full")

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	logContent := string(content)
	if !strings.Contains(logContent, "backup.failed") {
		t.Error("Log should contain backup.failed event")
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

func TestAuditCategoryTokens(t *testing.T) {
	// Test token audit category
	if string(AuditCategoryTokens) != "tokens" {
		t.Errorf("AuditCategoryTokens = %q, want %q", AuditCategoryTokens, "tokens")
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
			"changed": "ssl_enabled",
			"old":     false,
			"new":     true,
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
	// Test all spec-defined audit actions (AI.md PART 11)
	// Two-tier auth: no sessions, no user accounts, no admin login (AI.md line 11398)
	actions := []struct {
		action AuditAction
		want   string
	}{
		{AuditActionConfigChange, "config.updated"},
		{AuditActionConfigSMTPUpdated, "config.smtp_updated"},
		{AuditActionConfigSSLUpdated, "config.ssl_updated"},
		{AuditActionConfigSSLExpired, "config.ssl_expired"},
		{AuditActionConfigTorRegen, "config.tor_address_regenerated"},
		{AuditActionConfigBrandingUpdate, "config.branding_updated"},
		{AuditActionRateLimitExceeded, "security.rate_limit_exceeded"},
		{AuditActionIPBlocked, "security.ip_blocked"},
		{AuditActionIPUnblocked, "security.ip_unblocked"},
		{AuditActionCountryBlocked, "security.country_blocked"},
		{AuditActionCSRFFailure, "security.csrf_failure"},
		{AuditActionInvalidToken, "security.invalid_token"},
		{AuditActionBruteForceDetected, "security.brute_force_detected"},
		{AuditActionSuspiciousActivity, "security.suspicious_activity"},
		{AuditActionTokenCreate, "token.created"},
		{AuditActionTokenRevoke, "token.revoked"},
		{AuditActionTokenExpired, "token.expired"},
		{AuditActionBackupCreate, "backup.created"},
		{AuditActionBackupRestore, "backup.restored"},
		{AuditActionBackupDelete, "backup.deleted"},
		{AuditActionBackupFailed, "backup.failed"},
		{AuditActionServerStarted, "server.started"},
		{AuditActionServerStopped, "server.stopped"},
		{AuditActionMaintenanceEntered, "server.maintenance_entered"},
		{AuditActionMaintenanceExited, "server.maintenance_exited"},
		{AuditActionServerUpdated, "server.updated"},
		{AuditActionSchedulerTaskFail, "scheduler.task_failed"},
		{AuditActionSchedulerTaskRun, "scheduler.task_manual_run"},
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
		// $remote_addr is intentionally rejected — privacy is the product
		// (see AvailableFormatVariables in logging.go).
		{"mixed valid invalid", "$remote_addr $invalid $status", 2},
		{"variable with numbers", "$var123", 1},
		// First $ is skipped (no name), second is $remote_addr which is
		// rejected for privacy reasons, so still counts as unknown.
		{"consecutive dollars", "$$remote_addr", 1},
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

// ============================================================
// csvEscape tests
// ============================================================

func TestCSVEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain string", "hello", "hello"},
		{"empty string", "", ""},
		{"string with comma", "hello,world", `"hello,world"`},
		{"string with double quote", `say "hi"`, `"say ""hi"""`},
		{"string with newline", "line\none", "\"line\none\""},
		{"string with carriage return", "line\rone", "\"line\rone\""},
		{"all special chars", `a,b"c`, `"a,b""c"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := csvEscape(tt.input)
			if got != tt.want {
				t.Errorf("csvEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ============================================================
// matchesAuditFilter tests
// ============================================================

func TestMatchesAuditFilter(t *testing.T) {
	now := time.Now().UTC()
	base := AuditEntry{
		Event:    AuditActionConfigChange,
		Category: AuditCategoryConfig,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: "operator", IP: "10.0.0.1"},
		Target:   &AuditTarget{Type: "config", Name: "ssl"},
		Result:   "success",
		Time:     now,
	}

	tests := []struct {
		name string
		opts AuditQueryOptions
		want bool
	}{
		{
			name: "empty opts matches all",
			opts: AuditQueryOptions{},
			want: true,
		},
		{
			name: "category match",
			opts: AuditQueryOptions{Category: AuditCategoryConfig},
			want: true,
		},
		{
			name: "category no match",
			opts: AuditQueryOptions{Category: AuditCategorySecurity},
			want: false,
		},
		{
			name: "severity match",
			opts: AuditQueryOptions{Severity: AuditSeverityInfo},
			want: true,
		},
		{
			name: "severity no match",
			opts: AuditQueryOptions{Severity: AuditSeverityCritical},
			want: false,
		},
		{
			name: "result match",
			opts: AuditQueryOptions{Result: "success"},
			want: true,
		},
		{
			name: "result no match",
			opts: AuditQueryOptions{Result: "failure"},
			want: false,
		},
		{
			name: "actor username match",
			opts: AuditQueryOptions{ActorUsername: "operator"},
			want: true,
		},
		{
			name: "actor username no match",
			opts: AuditQueryOptions{ActorUsername: "attacker"},
			want: false,
		},
		{
			name: "actor IP match",
			opts: AuditQueryOptions{ActorIP: "10.0.0.1"},
			want: true,
		},
		{
			name: "actor IP no match",
			opts: AuditQueryOptions{ActorIP: "192.168.1.1"},
			want: false,
		},
		{
			name: "target type match",
			opts: AuditQueryOptions{TargetType: "config"},
			want: true,
		},
		{
			name: "target type no match",
			opts: AuditQueryOptions{TargetType: "token"},
			want: false,
		},
		{
			name: "target name match",
			opts: AuditQueryOptions{TargetName: "ssl"},
			want: true,
		},
		{
			name: "target name no match",
			opts: AuditQueryOptions{TargetName: "smtp"},
			want: false,
		},
		{
			name: "event match",
			opts: AuditQueryOptions{Event: AuditActionConfigChange},
			want: true,
		},
		{
			name: "event no match",
			opts: AuditQueryOptions{Event: AuditActionIPBlocked},
			want: false,
		},
		{
			name: "start time before entry",
			opts: AuditQueryOptions{StartTime: now.Add(-time.Hour)},
			want: true,
		},
		{
			name: "start time after entry",
			opts: AuditQueryOptions{StartTime: now.Add(time.Hour)},
			want: false,
		},
		{
			name: "end time after entry",
			opts: AuditQueryOptions{EndTime: now.Add(time.Hour)},
			want: true,
		},
		{
			name: "end time before entry",
			opts: AuditQueryOptions{EndTime: now.Add(-time.Hour)},
			want: false,
		},
		{
			name: "time range contains entry",
			opts: AuditQueryOptions{StartTime: now.Add(-time.Hour), EndTime: now.Add(time.Hour)},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesAuditFilter(base, tt.opts)
			if got != tt.want {
				t.Errorf("matchesAuditFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchesAuditFilterNilTarget(t *testing.T) {
	entry := AuditEntry{
		Category: AuditCategorySystem,
		Severity: AuditSeverityInfo,
		Actor:    AuditActor{Username: "system"},
		Result:   "success",
		Time:     time.Now().UTC(),
		Target:   nil,
	}

	// Target type filter with nil target should not match
	if matchesAuditFilter(entry, AuditQueryOptions{TargetType: "config"}) {
		t.Error("nil target should not match TargetType filter")
	}

	// Target name filter with nil target should not match
	if matchesAuditFilter(entry, AuditQueryOptions{TargetName: "ssl"}) {
		t.Error("nil target should not match TargetName filter")
	}

	// No target filter with nil target should match
	if !matchesAuditFilter(entry, AuditQueryOptions{}) {
		t.Error("entry with nil target should match empty filter")
	}
}

// ============================================================
// DefaultAuditRetentionPolicy tests
// ============================================================

func TestDefaultAuditRetentionPolicy(t *testing.T) {
	policy := DefaultAuditRetentionPolicy()

	if policy.MaxAge == 0 {
		t.Error("DefaultAuditRetentionPolicy MaxAge should be non-zero")
	}
	if policy.MaxAge < 24*time.Hour {
		t.Error("DefaultAuditRetentionPolicy MaxAge should be at least one day")
	}
	if !policy.PreserveCritical {
		t.Error("DefaultAuditRetentionPolicy should preserve critical events")
	}
}

// ============================================================
// QueryAuditLogs tests
// ============================================================

func TestQueryAuditLogsEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 0 {
		t.Errorf("QueryAuditLogs() Total = %d, want 0", result.Total)
	}
	if result.Count != 0 {
		t.Errorf("QueryAuditLogs() Count = %d, want 0", result.Count)
	}
	if len(result.Entries) != 0 {
		t.Errorf("QueryAuditLogs() len(Entries) = %d, want 0", len(result.Entries))
	}
}

func TestQueryAuditLogsNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "no-such-audit.log")
	// Create logger but remove file immediately
	logger := NewAuditLogger(path)
	logger.Close()
	os.Remove(path)

	// Re-open with no backing file
	logger2 := &AuditLogger{path: path}

	result, err := logger2.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() on missing file error = %v", err)
	}
	if result.Total != 0 {
		t.Errorf("QueryAuditLogs() Total = %d, want 0", result.Total)
	}
}

func TestQueryAuditLogsReturnsEntries(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigChange("operator", "10.0.0.1", "engines", "enabled google")
	logger.LogIPBlocked("operator", "10.0.0.1", "1.2.3.4", "spam")
	logger.LogServerStarted("1.0.0", "node-1")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 3 {
		t.Errorf("QueryAuditLogs() Total = %d, want 3", result.Total)
	}
	if result.Count != 3 {
		t.Errorf("QueryAuditLogs() Count = %d, want 3", result.Count)
	}
}

func TestQueryAuditLogsFilterByCategory(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigChange("operator", "10.0.0.1", "engines", "change")
	logger.LogIPBlocked("operator", "10.0.0.1", "1.2.3.4", "spam")
	logger.LogServerStarted("1.0.0", "node-1")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{Category: AuditCategoryConfig})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Errorf("QueryAuditLogs() filtered Total = %d, want 1", result.Total)
	}
	if result.Entries[0].Category != AuditCategoryConfig {
		t.Errorf("QueryAuditLogs() entry category = %q, want %q", result.Entries[0].Category, AuditCategoryConfig)
	}
}

func TestQueryAuditLogsFilterBySeverity(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigChange("operator", "10.0.0.1", "engines", "change")
	logger.LogBruteForceDetected("10.0.0.2", "/api/v1/search", 10)

	result, err := logger.QueryAuditLogs(AuditQueryOptions{Severity: AuditSeverityCritical})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Errorf("QueryAuditLogs() critical Total = %d, want 1", result.Total)
	}
}

func TestQueryAuditLogsPagination(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	for i := 0; i < 5; i++ {
		logger.LogConfigChange("operator", "10.0.0.1", "field", "change")
	}

	// First page: limit 2
	r1, err := logger.QueryAuditLogs(AuditQueryOptions{Limit: 2})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if r1.Total != 5 {
		t.Errorf("Total = %d, want 5", r1.Total)
	}
	if r1.Count != 2 {
		t.Errorf("Count = %d, want 2", r1.Count)
	}
	if len(r1.Entries) != 2 {
		t.Errorf("len(Entries) = %d, want 2", len(r1.Entries))
	}

	// Second page: limit 2, offset 2
	r2, err := logger.QueryAuditLogs(AuditQueryOptions{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if r2.Count != 2 {
		t.Errorf("Page 2 Count = %d, want 2", r2.Count)
	}

	// Offset past end
	r3, err := logger.QueryAuditLogs(AuditQueryOptions{Offset: 10})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if r3.Count != 0 {
		t.Errorf("Offset past end Count = %d, want 0", r3.Count)
	}
}

func TestQueryAuditLogsTimeRange(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	past := time.Now().UTC().Add(-2 * time.Hour)
	future := time.Now().UTC().Add(2 * time.Hour)

	logger.LogConfigChange("operator", "10.0.0.1", "ssl", "updated")

	// Entry is recent — querying for last hour should include it
	r, err := logger.QueryAuditLogs(AuditQueryOptions{
		StartTime: past,
		EndTime:   future,
	})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if r.Total != 1 {
		t.Errorf("QueryAuditLogs() with wide time range Total = %d, want 1", r.Total)
	}

	// Query far in the past — should return nothing
	r2, err := logger.QueryAuditLogs(AuditQueryOptions{
		EndTime: past,
	})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if r2.Total != 0 {
		t.Errorf("QueryAuditLogs() past range Total = %d, want 0", r2.Total)
	}
}

// ============================================================
// ExportAuditLogs tests
// ============================================================

func TestExportAuditLogsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigChange("operator", "10.0.0.1", "engines", "enabled google")
	logger.LogIPBlocked("operator", "10.0.0.1", "5.6.7.8", "spam")

	var buf bytes.Buffer
	err := logger.ExportAuditLogs(AuditQueryOptions{}, AuditExportJSON, &buf)
	if err != nil {
		t.Fatalf("ExportAuditLogs(JSON) error = %v", err)
	}

	var entries []AuditEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("ExportAuditLogs(JSON) produced invalid JSON: %v\nOutput: %s", err, buf.String())
	}
	if len(entries) != 2 {
		t.Errorf("ExportAuditLogs(JSON) entries count = %d, want 2", len(entries))
	}
}

func TestExportAuditLogsCSV(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigChange("operator", "10.0.0.1", "engines", "enabled google")

	var buf bytes.Buffer
	err := logger.ExportAuditLogs(AuditQueryOptions{}, AuditExportCSV, &buf)
	if err != nil {
		t.Fatalf("ExportAuditLogs(CSV) error = %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Header + 1 data row
	if len(lines) < 2 {
		t.Errorf("ExportAuditLogs(CSV) produced %d lines, want at least 2", len(lines))
	}
	if !strings.HasPrefix(lines[0], "id,time,event,category") {
		t.Errorf("ExportAuditLogs(CSV) header = %q, expected CSV header", lines[0])
	}
	if !strings.Contains(lines[1], "config.updated") {
		t.Errorf("ExportAuditLogs(CSV) data row = %q, expected config.updated event", lines[1])
	}
}

func TestExportAuditLogsUnsupportedFormat(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigChange("operator", "10.0.0.1", "engines", "change")

	var buf bytes.Buffer
	err := logger.ExportAuditLogs(AuditQueryOptions{}, AuditExportFormat("xml"), &buf)
	if err == nil {
		t.Error("ExportAuditLogs(unsupported format) should return error")
	}
}

func TestExportAuditLogsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	var buf bytes.Buffer
	err := logger.ExportAuditLogs(AuditQueryOptions{}, AuditExportJSON, &buf)
	if err != nil {
		t.Fatalf("ExportAuditLogs(JSON) on empty log error = %v", err)
	}
	var entries []AuditEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("ExportAuditLogs(JSON) empty produced invalid JSON: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("ExportAuditLogs(JSON) empty entries = %d, want 0", len(entries))
	}
}

// ============================================================
// CleanupAuditLogs tests
// ============================================================

func TestCleanupAuditLogsNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "no-such.log")
	logger := &AuditLogger{path: path}

	removed, err := logger.CleanupAuditLogs(DefaultAuditRetentionPolicy())
	if err != nil {
		t.Fatalf("CleanupAuditLogs() on missing file error = %v", err)
	}
	if removed != 0 {
		t.Errorf("CleanupAuditLogs() removed = %d, want 0", removed)
	}
}

func TestCleanupAuditLogsMaxEntries(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	for i := 0; i < 10; i++ {
		logger.LogConfigChange("operator", "10.0.0.1", "field", "change")
	}

	policy := AuditRetentionPolicy{
		MaxAge:           0,
		MaxEntries:       5,
		PreserveCritical: false,
	}

	removed, err := logger.CleanupAuditLogs(policy)
	if err != nil {
		t.Fatalf("CleanupAuditLogs() error = %v", err)
	}
	if removed != 5 {
		t.Errorf("CleanupAuditLogs() removed = %d, want 5", removed)
	}

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() after cleanup error = %v", err)
	}
	if result.Total != 5 {
		t.Errorf("After cleanup Total = %d, want 5", result.Total)
	}
}

func TestCleanupAuditLogsPreservesCritical(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	// Write 3 normal + 1 critical
	for i := 0; i < 3; i++ {
		logger.LogConfigChange("operator", "10.0.0.1", "field", "change")
	}
	logger.LogBruteForceDetected("1.2.3.4", "/api/v1/search", 50)

	policy := AuditRetentionPolicy{
		MaxAge:           0,
		MaxEntries:       2,
		PreserveCritical: true,
	}

	_, err := logger.CleanupAuditLogs(policy)
	if err != nil {
		t.Fatalf("CleanupAuditLogs() error = %v", err)
	}

	result, err := logger.QueryAuditLogs(AuditQueryOptions{Severity: AuditSeverityCritical})
	if err != nil {
		t.Fatalf("QueryAuditLogs() after cleanup error = %v", err)
	}
	if result.Total < 1 {
		t.Error("CleanupAuditLogs() should preserve critical entries")
	}
}

func TestCleanupAuditLogsMaxAge(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	// Write a current entry
	logger.LogConfigChange("operator", "10.0.0.1", "ssl", "change")

	// Cleanup with zero MaxAge means no age filtering
	policy := AuditRetentionPolicy{
		MaxAge:           0,
		MaxEntries:       0,
		PreserveCritical: false,
	}

	removed, err := logger.CleanupAuditLogs(policy)
	if err != nil {
		t.Fatalf("CleanupAuditLogs() error = %v", err)
	}
	if removed != 0 {
		t.Errorf("CleanupAuditLogs() with MaxAge=0 should remove 0, got %d", removed)
	}
}

// ============================================================
// GetAuditStats tests
// ============================================================

func TestGetAuditStatsEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	stats, err := logger.GetAuditStats()
	if err != nil {
		t.Fatalf("GetAuditStats() error = %v", err)
	}
	if stats.TotalEntries != 0 {
		t.Errorf("GetAuditStats() TotalEntries = %d, want 0", stats.TotalEntries)
	}
	if len(stats.ByCategory) != 0 {
		t.Errorf("GetAuditStats() ByCategory len = %d, want 0", len(stats.ByCategory))
	}
}

func TestGetAuditStatsCounts(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigChange("operator", "10.0.0.1", "ssl", "change")
	logger.LogConfigChange("operator", "10.0.0.1", "smtp", "change")
	logger.LogIPBlocked("operator", "10.0.0.1", "1.2.3.4", "spam")
	logger.LogServerStarted("1.0.0", "node-1")

	stats, err := logger.GetAuditStats()
	if err != nil {
		t.Fatalf("GetAuditStats() error = %v", err)
	}
	if stats.TotalEntries != 4 {
		t.Errorf("GetAuditStats() TotalEntries = %d, want 4", stats.TotalEntries)
	}
	if stats.ByCategory[AuditCategoryConfig] != 2 {
		t.Errorf("GetAuditStats() config count = %d, want 2", stats.ByCategory[AuditCategoryConfig])
	}
	if stats.ByCategory[AuditCategorySecurity] != 1 {
		t.Errorf("GetAuditStats() security count = %d, want 1", stats.ByCategory[AuditCategorySecurity])
	}
	if stats.ByCategory[AuditCategorySystem] != 1 {
		t.Errorf("GetAuditStats() system count = %d, want 1", stats.ByCategory[AuditCategorySystem])
	}
	if stats.ByResult["success"] != 4 {
		t.Errorf("GetAuditStats() success result count = %d, want 4", stats.ByResult["success"])
	}
	if stats.OldestEntry.IsZero() {
		t.Error("GetAuditStats() OldestEntry should be set")
	}
	if stats.NewestEntry.IsZero() {
		t.Error("GetAuditStats() NewestEntry should be set")
	}
}

func TestGetAuditStatsNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "no-such.log")
	logger := &AuditLogger{path: path}

	stats, err := logger.GetAuditStats()
	if err != nil {
		t.Fatalf("GetAuditStats() on missing file error = %v", err)
	}
	if stats.TotalEntries != 0 {
		t.Errorf("GetAuditStats() on missing file TotalEntries = %d, want 0", stats.TotalEntries)
	}
}

// ============================================================
// Audit helper method tests
// ============================================================

func TestLogServerStarted(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogServerStarted("1.2.3", "node-42")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogServerStarted() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionServerStarted {
		t.Errorf("LogServerStarted() Event = %q, want %q", entry.Event, AuditActionServerStarted)
	}
	if entry.Category != AuditCategorySystem {
		t.Errorf("LogServerStarted() Category = %q, want %q", entry.Category, AuditCategorySystem)
	}
}

func TestLogServerStopped(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogServerStopped("graceful shutdown", "node-1")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogServerStopped() wrote %d entries, want 1", result.Total)
	}
	if result.Entries[0].Event != AuditActionServerStopped {
		t.Errorf("LogServerStopped() Event = %q, want %q", result.Entries[0].Event, AuditActionServerStopped)
	}
}

func TestLogMaintenanceEnteredExited(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogMaintenanceEntered("operator", "10.0.0.1", "db unavailable")
	logger.LogMaintenanceExited("operator", "10.0.0.1")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{Category: AuditCategorySystem})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("Maintenance enter+exit wrote %d entries, want 2", result.Total)
	}

	// Results are sorted newest-first
	events := make(map[AuditAction]bool)
	for _, e := range result.Entries {
		events[e.Event] = true
	}
	if !events[AuditActionMaintenanceEntered] {
		t.Error("LogMaintenanceEntered event not found")
	}
	if !events[AuditActionMaintenanceExited] {
		t.Error("LogMaintenanceExited event not found")
	}
}

func TestLogConfigUpdated(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogConfigUpdated("operator", "10.0.0.1", "server", "port", 8080, 9090)

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogConfigUpdated() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionConfigChange {
		t.Errorf("LogConfigUpdated() Event = %q, want %q", entry.Event, AuditActionConfigChange)
	}
	if entry.Target == nil || entry.Target.Name != "server.port" {
		t.Errorf("LogConfigUpdated() Target.Name = %v, want server.port", entry.Target)
	}
}

func TestLogIPBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogIPBlocked("operator", "10.0.0.1", "5.6.7.8", "brute force")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogIPBlocked() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionIPBlocked {
		t.Errorf("LogIPBlocked() Event = %q, want %q", entry.Event, AuditActionIPBlocked)
	}
	if entry.Target == nil || entry.Target.Name != "5.6.7.8" {
		t.Errorf("LogIPBlocked() Target.Name = %v, want 5.6.7.8", entry.Target)
	}
	if entry.Reason != "brute force" {
		t.Errorf("LogIPBlocked() Reason = %q, want brute force", entry.Reason)
	}
}

func TestLogIPUnblocked(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogIPUnblocked("operator", "10.0.0.1", "5.6.7.8")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogIPUnblocked() wrote %d entries, want 1", result.Total)
	}
	if result.Entries[0].Event != AuditActionIPUnblocked {
		t.Errorf("LogIPUnblocked() Event = %q, want %q", result.Entries[0].Event, AuditActionIPUnblocked)
	}
}

func TestLogRateLimitExceeded(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogRateLimitExceeded("1.2.3.4", "/api/v1/search", 100)

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogRateLimitExceeded() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionRateLimitExceeded {
		t.Errorf("LogRateLimitExceeded() Event = %q, want %q", entry.Event, AuditActionRateLimitExceeded)
	}
	if entry.Result != "blocked" {
		t.Errorf("LogRateLimitExceeded() Result = %q, want blocked", entry.Result)
	}
}

func TestLogBruteForceDetected(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogBruteForceDetected("9.8.7.6", "/api/v1/search", 50)

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogBruteForceDetected() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionBruteForceDetected {
		t.Errorf("LogBruteForceDetected() Event = %q, want %q", entry.Event, AuditActionBruteForceDetected)
	}
	if entry.Severity != AuditSeverityCritical {
		t.Errorf("LogBruteForceDetected() Severity = %q, want critical", entry.Severity)
	}
}

func TestLogCSRFFailure(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogCSRFFailure("1.2.3.4", "/api/v1/preferences")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogCSRFFailure() wrote %d entries, want 1", result.Total)
	}
	if result.Entries[0].Event != AuditActionCSRFFailure {
		t.Errorf("LogCSRFFailure() Event = %q, want %q", result.Entries[0].Event, AuditActionCSRFFailure)
	}
}

func TestLogInvalidToken(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogInvalidToken("1.2.3.4", "/server/status", "bearer")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogInvalidToken() wrote %d entries, want 1", result.Total)
	}
	if result.Entries[0].Event != AuditActionInvalidToken {
		t.Errorf("LogInvalidToken() Event = %q, want %q", result.Entries[0].Event, AuditActionInvalidToken)
	}
}

func TestLogBackupDelete(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogBackupDelete("operator", "10.0.0.1", "backup-20240101.tar.gz")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogBackupDelete() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionBackupDelete {
		t.Errorf("LogBackupDelete() Event = %q, want %q", entry.Event, AuditActionBackupDelete)
	}
	if entry.Target == nil || entry.Target.Name != "backup-20240101.tar.gz" {
		t.Errorf("LogBackupDelete() Target.Name = %v, want backup-20240101.tar.gz", entry.Target)
	}
}

func TestLogBackupFailed(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogBackupFailed("operator", "10.0.0.1", "disk full")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogBackupFailed() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionBackupFailed {
		t.Errorf("LogBackupFailed() Event = %q, want %q", entry.Event, AuditActionBackupFailed)
	}
	if entry.Result != "failure" {
		t.Errorf("LogBackupFailed() Result = %q, want failure", entry.Result)
	}
	if entry.Reason != "disk full" {
		t.Errorf("LogBackupFailed() Reason = %q, want disk full", entry.Reason)
	}
}

func TestLogSMTPUpdated(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogSMTPUpdated("operator", "10.0.0.1")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogSMTPUpdated() wrote %d entries, want 1", result.Total)
	}
	if result.Entries[0].Event != AuditActionConfigSMTPUpdated {
		t.Errorf("LogSMTPUpdated() Event = %q, want %q", result.Entries[0].Event, AuditActionConfigSMTPUpdated)
	}
}

func TestLogSSLUpdated(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogSSLUpdated("operator", "10.0.0.1", "search.example.com")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogSSLUpdated() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionConfigSSLUpdated {
		t.Errorf("LogSSLUpdated() Event = %q, want %q", entry.Event, AuditActionConfigSSLUpdated)
	}
	if entry.Target == nil || entry.Target.Name != "search.example.com" {
		t.Errorf("LogSSLUpdated() Target.Name = %v, want search.example.com", entry.Target)
	}
}

func TestLogSchedulerTaskFailed(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogSchedulerTaskFailed("geoip_update", "connection timeout")

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogSchedulerTaskFailed() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionSchedulerTaskFail {
		t.Errorf("LogSchedulerTaskFailed() Event = %q, want %q", entry.Event, AuditActionSchedulerTaskFail)
	}
	if entry.Target == nil || entry.Target.Name != "geoip_update" {
		t.Errorf("LogSchedulerTaskFailed() Target.Name = %v, want geoip_update", entry.Target)
	}
}

func TestLogSchedulerTaskManualRun(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	logger.LogSchedulerTaskManualRun("operator", "10.0.0.1", "blocklist_update", true)
	logger.LogSchedulerTaskManualRun("operator", "10.0.0.1", "cve_update", false)

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("LogSchedulerTaskManualRun() wrote %d entries, want 2", result.Total)
	}

	// Check success result
	var successEntry, failEntry AuditEntry
	for _, e := range result.Entries {
		if e.Result == "success" {
			successEntry = e
		} else {
			failEntry = e
		}
	}
	if successEntry.Event != AuditActionSchedulerTaskRun {
		t.Errorf("LogSchedulerTaskManualRun success Event = %q, want %q", successEntry.Event, AuditActionSchedulerTaskRun)
	}
	if failEntry.Result != "failure" {
		t.Errorf("LogSchedulerTaskManualRun failure Result = %q, want failure", failEntry.Result)
	}
}

func TestLogEvent(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "audit.log")
	logger := NewAuditLogger(path)
	defer logger.Close()

	actor := AuditActor{Type: "operator", Username: "admin", IP: "10.0.0.1"}
	target := &AuditTarget{Type: "config", Name: "tor"}
	details := map[string]interface{}{"regenerated": true}

	logger.LogEvent(
		AuditActionConfigTorRegen,
		AuditCategoryConfig,
		AuditSeverityWarning,
		actor,
		target,
		"success",
		details,
		"user requested regen",
	)

	result, err := logger.QueryAuditLogs(AuditQueryOptions{})
	if err != nil {
		t.Fatalf("QueryAuditLogs() error = %v", err)
	}
	if result.Total != 1 {
		t.Fatalf("LogEvent() wrote %d entries, want 1", result.Total)
	}
	entry := result.Entries[0]
	if entry.Event != AuditActionConfigTorRegen {
		t.Errorf("LogEvent() Event = %q, want %q", entry.Event, AuditActionConfigTorRegen)
	}
	if entry.Category != AuditCategoryConfig {
		t.Errorf("LogEvent() Category = %q, want %q", entry.Category, AuditCategoryConfig)
	}
	if entry.Severity != AuditSeverityWarning {
		t.Errorf("LogEvent() Severity = %q, want %q", entry.Severity, AuditSeverityWarning)
	}
	if entry.Reason != "user requested regen" {
		t.Errorf("LogEvent() Reason = %q, want user requested regen", entry.Reason)
	}
	if entry.Target == nil || entry.Target.Name != "tor" {
		t.Errorf("LogEvent() Target.Name = %v, want tor", entry.Target)
	}
}
