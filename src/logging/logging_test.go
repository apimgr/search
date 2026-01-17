package logging

import (
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
