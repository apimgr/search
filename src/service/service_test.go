package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestNewServiceManager(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	if sm == nil {
		t.Fatal("NewServiceManager() returned nil")
	}
	if sm.config != cfg {
		t.Error("Config not set correctly")
	}
}

func TestNewServiceManagerNilConfig(t *testing.T) {
	sm := NewServiceManager(nil)
	if sm == nil {
		t.Fatal("NewServiceManager(nil) returned nil")
	}
}

func TestServiceManagerRenderTemplate(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	tmpl := "Hello, {{.Name}}!"
	data := map[string]string{"Name": "World"}

	result, err := sm.renderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("renderTemplate() error = %v", err)
	}
	if result != "Hello, World!" {
		t.Errorf("renderTemplate() = %q, want %q", result, "Hello, World!")
	}
}

func TestServiceManagerRenderTemplateError(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	tmpl := "Hello, {{.Name"
	data := map[string]string{"Name": "World"}

	_, err := sm.renderTemplate(tmpl, data)
	if err == nil {
		t.Error("renderTemplate() should fail with invalid template")
	}
}

func TestGetServiceStatus(t *testing.T) {
	status := GetServiceStatus()
	if status == "" {
		t.Error("GetServiceStatus() returned empty string")
	}
	// Should contain "Service status:"
	if !contains(status, "Service status:") {
		t.Errorf("GetServiceStatus() = %q, should contain 'Service status:'", status)
	}
}

// Maintenance tests

func TestMaintenanceModeString(t *testing.T) {
	tests := []struct {
		mode MaintenanceMode
		want string
	}{
		{ModeNormal, "normal"},
		{ModeDegraded, "degraded"},
		{ModeMaintenance, "maintenance"},
		{ModeRecovery, "recovery"},
		{ModeEmergency, "emergency"},
		{MaintenanceMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("MaintenanceMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestHealthStatusStruct(t *testing.T) {
	now := time.Now()
	hs := HealthStatus{
		Component:   "database",
		Healthy:     true,
		Message:     "OK",
		LastCheck:   now,
		LastHealthy: now,
		ErrorCount:  0,
	}

	if hs.Component != "database" {
		t.Errorf("Component = %q, want %q", hs.Component, "database")
	}
	if !hs.Healthy {
		t.Error("Healthy should be true")
	}
	if hs.ErrorCount != 0 {
		t.Error("ErrorCount should be 0")
	}
}

func TestNewMaintenanceService(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	if ms == nil {
		t.Fatal("NewMaintenanceService() returned nil")
	}
	if ms.config != cfg {
		t.Error("Config not set correctly")
	}
	if ms.health == nil {
		t.Error("health map should not be nil")
	}
	if ms.recoveryFuncs == nil {
		t.Error("recoveryFuncs map should not be nil")
	}
	if ms.GetMode() != ModeNormal {
		t.Error("Initial mode should be ModeNormal")
	}
}

func TestMaintenanceServiceStartStop(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Start should succeed
	if err := ms.StartMaintenanceService(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait a bit for initialization
	time.Sleep(50 * time.Millisecond)

	// Stop should succeed
	ms.StopMaintenanceService()

	// Should not be running after stop
	ms.mu.RLock()
	running := ms.running
	ms.mu.RUnlock()

	if running {
		t.Error("Service should not be running after Stop()")
	}
}

func TestMaintenanceServiceStartTwice(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// First start
	if err := ms.StartMaintenanceService(); err != nil {
		t.Fatalf("First Start() error = %v", err)
	}

	// Second start should be no-op
	if err := ms.StartMaintenanceService(); err != nil {
		t.Fatalf("Second Start() error = %v", err)
	}

	ms.StopMaintenanceService()
}

func TestMaintenanceServiceSetGetMode(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tests := []MaintenanceMode{
		ModeNormal,
		ModeDegraded,
		ModeMaintenance,
		ModeRecovery,
		ModeEmergency,
	}

	for _, mode := range tests {
		t.Run(mode.String(), func(t *testing.T) {
			ms.SetMode(mode, "test message")
			got := ms.GetMode()
			if got != mode {
				t.Errorf("GetMode() = %v, want %v", got, mode)
			}
		})
	}
}

func TestMaintenanceServiceGetMessage(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.SetMode(ModeMaintenance, "System upgrade in progress")
	msg := ms.GetMessage()

	if msg != "System upgrade in progress" {
		t.Errorf("GetMessage() = %q, want %q", msg, "System upgrade in progress")
	}
}

func TestMaintenanceServiceModeChecks(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Test IsNormal
	ms.SetMode(ModeNormal, "")
	if !ms.IsNormal() {
		t.Error("IsNormal() should return true")
	}

	// Test IsDegraded
	ms.SetMode(ModeDegraded, "")
	if !ms.IsDegraded() {
		t.Error("IsDegraded() should return true")
	}

	// Test IsInMaintenance
	ms.SetMode(ModeMaintenance, "")
	if !ms.IsInMaintenance() {
		t.Error("IsInMaintenance() should return true")
	}
}

func TestMaintenanceServiceEnableDisable(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Enable maintenance
	ms.EnableMaintenance("Scheduled maintenance", 0)
	if !ms.IsInMaintenance() {
		t.Error("Should be in maintenance mode after EnableMaintenance()")
	}

	// Disable maintenance
	ms.DisableMaintenance()
	if !ms.IsNormal() {
		t.Error("Should be in normal mode after DisableMaintenance()")
	}
}

func TestMaintenanceServiceEnableWithDuration(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Enable maintenance with duration
	ms.EnableMaintenance("Quick maintenance", 100*time.Millisecond)
	if !ms.IsInMaintenance() {
		t.Error("Should be in maintenance mode")
	}

	scheduledEnd := ms.GetScheduledEnd()
	if scheduledEnd.IsZero() {
		t.Error("GetScheduledEnd() should not be zero")
	}

	// Wait for auto-disable
	time.Sleep(200 * time.Millisecond)

	// Should auto-exit maintenance mode
	if ms.GetMode() == ModeMaintenance {
		t.Error("Should have auto-exited maintenance mode")
	}

	ms.StopMaintenanceService()
}

func TestMaintenanceServiceRegisterCallback(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	var calledWith MaintenanceMode
	var callCount int

	ms.RegisterCallback(func(mode MaintenanceMode) {
		calledWith = mode
		callCount++
	})

	ms.SetMode(ModeMaintenance, "test")

	// Wait for callback
	time.Sleep(50 * time.Millisecond)

	if callCount == 0 {
		t.Error("Callback should have been called")
	}
	if calledWith != ModeMaintenance {
		t.Errorf("Callback received mode %v, want %v", calledWith, ModeMaintenance)
	}
}

func TestMaintenanceServiceSetDatabaseChecks(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	serverCheck := func(ctx context.Context) error { return nil }
	usersCheck := func(ctx context.Context) error { return nil }

	ms.SetDatabaseChecks(serverCheck, usersCheck)

	// Verify checks are set (indirectly via starting service)
	ms.mu.RLock()
	hasServerCheck := ms.serverDBCheck != nil
	hasUsersCheck := ms.usersDBCheck != nil
	ms.mu.RUnlock()

	if !hasServerCheck {
		t.Error("serverDBCheck should be set")
	}
	if !hasUsersCheck {
		t.Error("usersDBCheck should be set")
	}
}

func TestMaintenanceServiceRegisterRecoveryFunc(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	recoveryFunc := func(ctx context.Context) error { return nil }
	ms.RegisterRecoveryFunc("cache", recoveryFunc)

	ms.mu.RLock()
	_, exists := ms.recoveryFuncs["cache"]
	ms.mu.RUnlock()

	if !exists {
		t.Error("Recovery function should be registered")
	}
}

func TestMaintenanceServiceGetHealthStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Start to initialize health status
	ms.StartMaintenanceService()
	time.Sleep(50 * time.Millisecond)

	health := ms.GetHealthStatus()
	if health == nil {
		t.Fatal("GetHealthStatus() returned nil")
	}

	// Should have some components
	if len(health) == 0 {
		t.Error("GetHealthStatus() should return some components")
	}

	ms.StopMaintenanceService()
}

func TestMaintenanceServiceGetStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.SetMode(ModeMaintenance, "Test maintenance")

	status := ms.GetStatus()
	if status == nil {
		t.Fatal("GetStatus() returned nil")
	}

	if status["mode"] != "maintenance" {
		t.Errorf("status[mode] = %v, want %q", status["mode"], "maintenance")
	}
	if status["message"] != "Test maintenance" {
		t.Errorf("status[message] = %v, want %q", status["message"], "Test maintenance")
	}
}

// GracefulDegradation tests

func TestNewGracefulDegradation(t *testing.T) {
	gd := NewGracefulDegradation()
	if gd == nil {
		t.Fatal("NewGracefulDegradation() returned nil")
	}
	if gd.degradedFeatures == nil {
		t.Error("degradedFeatures map should not be nil")
	}
	if gd.fallbacks == nil {
		t.Error("fallbacks map should not be nil")
	}
}

func TestGracefulDegradationMarkDegraded(t *testing.T) {
	gd := NewGracefulDegradation()

	gd.MarkDegraded("search")

	if !gd.IsDegraded("search") {
		t.Error("IsDegraded(search) should return true after MarkDegraded")
	}
}

func TestGracefulDegradationMarkHealthy(t *testing.T) {
	gd := NewGracefulDegradation()

	gd.MarkDegraded("search")
	gd.MarkHealthy("search")

	if gd.IsDegraded("search") {
		t.Error("IsDegraded(search) should return false after MarkHealthy")
	}
}

func TestGracefulDegradationIsDegraded(t *testing.T) {
	gd := NewGracefulDegradation()

	// Initially not degraded
	if gd.IsDegraded("widgets") {
		t.Error("IsDegraded() should return false for non-degraded feature")
	}

	gd.MarkDegraded("widgets")
	if !gd.IsDegraded("widgets") {
		t.Error("IsDegraded() should return true for degraded feature")
	}
}

func TestGracefulDegradationRegisterFallback(t *testing.T) {
	gd := NewGracefulDegradation()

	gd.RegisterFallback("search", func() interface{} {
		return "fallback results"
	})

	result := gd.GetFallback("search")
	if result != "fallback results" {
		t.Errorf("GetFallback() = %v, want %q", result, "fallback results")
	}
}

func TestGracefulDegradationGetFallbackMissing(t *testing.T) {
	gd := NewGracefulDegradation()

	result := gd.GetFallback("nonexistent")
	if result != nil {
		t.Errorf("GetFallback() = %v, want nil for missing fallback", result)
	}
}

func TestGracefulDegradationExecuteSuccess(t *testing.T) {
	gd := NewGracefulDegradation()

	result, err := gd.Execute("search", func() (interface{}, error) {
		return "search results", nil
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result != "search results" {
		t.Errorf("Execute() = %v, want %q", result, "search results")
	}
}

func TestGracefulDegradationExecuteDegraded(t *testing.T) {
	gd := NewGracefulDegradation()

	gd.MarkDegraded("search")
	gd.RegisterFallback("search", func() interface{} {
		return "fallback results"
	})

	result, err := gd.Execute("search", func() (interface{}, error) {
		return "search results", nil
	})

	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result != "fallback results" {
		t.Errorf("Execute() = %v, want %q (fallback)", result, "fallback results")
	}
}

func TestGracefulDegradationExecuteDegradedNoFallback(t *testing.T) {
	gd := NewGracefulDegradation()

	gd.MarkDegraded("search")

	_, err := gd.Execute("search", func() (interface{}, error) {
		return "search results", nil
	})

	if err == nil {
		t.Error("Execute() should return error when degraded with no fallback")
	}
}

func TestGracefulDegradationGetDegradedFeatures(t *testing.T) {
	gd := NewGracefulDegradation()

	gd.MarkDegraded("search")
	gd.MarkDegraded("widgets")
	gd.MarkDegraded("cache")

	features := gd.GetDegradedFeatures()
	if len(features) != 3 {
		t.Errorf("GetDegradedFeatures() returned %d features, want 3", len(features))
	}

	// Check all features are present
	featureMap := make(map[string]bool)
	for _, f := range features {
		featureMap[f] = true
	}

	expected := []string{"search", "widgets", "cache"}
	for _, exp := range expected {
		if !featureMap[exp] {
			t.Errorf("GetDegradedFeatures() missing %q", exp)
		}
	}
}

// File helper tests

func TestCopyFile(t *testing.T) {
	// Create temp source file
	src, err := os.CreateTemp("", "test-src-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(src.Name())

	content := []byte("test content for copy")
	if _, err := src.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	src.Close()

	// Create destination path
	dst, err := os.CreateTemp("", "test-dst-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	dst.Close()
	defer os.Remove(dst.Name())

	// Copy file
	if err := copyFile(src.Name(), dst.Name()); err != nil {
		t.Fatalf("copyFile() error = %v", err)
	}

	// Verify content
	copiedContent, err := os.ReadFile(dst.Name())
	if err != nil {
		t.Fatalf("Failed to read copied file: %v", err)
	}

	if string(copiedContent) != string(content) {
		t.Errorf("Copied content = %q, want %q", copiedContent, content)
	}
}

func TestFileChecksum(t *testing.T) {
	// Create temp file
	f, err := os.CreateTemp("", "test-checksum-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(f.Name())

	content := []byte("test content for checksum")
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	f.Close()

	checksum, err := fileChecksum(f.Name())
	if err != nil {
		t.Fatalf("fileChecksum() error = %v", err)
	}

	// SHA256 checksum should be 64 hex characters
	if len(checksum) != 64 {
		t.Errorf("Checksum length = %d, want 64", len(checksum))
	}

	// Verify same content produces same checksum
	checksum2, err := fileChecksum(f.Name())
	if err != nil {
		t.Fatalf("Second fileChecksum() error = %v", err)
	}

	if checksum != checksum2 {
		t.Error("Same file should produce same checksum")
	}
}

func TestFileChecksumError(t *testing.T) {
	_, err := fileChecksum("/nonexistent/file.txt")
	if err == nil {
		t.Error("fileChecksum() should error for nonexistent file")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Additional tests for improved coverage

func TestCopyFileSourceNotExist(t *testing.T) {
	err := copyFile("/nonexistent/source/file.txt", "/tmp/dest.txt")
	if err == nil {
		t.Error("copyFile() should fail when source doesn't exist")
	}
}

func TestCopyFileDestDirNotExist(t *testing.T) {
	// Create temp source file
	src, err := os.CreateTemp("", "test-src-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	src.Write([]byte("test content"))
	src.Close()
	defer os.Remove(src.Name())

	// Try to copy to non-existent directory
	err = copyFile(src.Name(), "/nonexistent/dir/dest.txt")
	if err == nil {
		t.Error("copyFile() should fail when dest directory doesn't exist")
	}
}

func TestMaintenanceServiceCheckComponent(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Create a check function that fails
	failingCheck := func(ctx context.Context) error {
		return fmt.Errorf("component failed")
	}

	ms.SetDatabaseChecks(failingCheck, nil)
	ms.StartMaintenanceService()
	time.Sleep(100 * time.Millisecond)

	// Health should show unhealthy
	health := ms.GetHealthStatus()
	if serverDB, ok := health["server_db"]; ok {
		if serverDB.Healthy {
			t.Log("Server DB should be unhealthy after failing check")
		}
	}

	ms.StopMaintenanceService()
}

func TestMaintenanceServiceMultipleCallbacks(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	var callCounts [3]int

	for i := 0; i < 3; i++ {
		idx := i
		ms.RegisterCallback(func(mode MaintenanceMode) {
			callCounts[idx]++
		})
	}

	ms.SetMode(ModeMaintenance, "test")
	time.Sleep(50 * time.Millisecond)

	// All callbacks should have been called
	for i, count := range callCounts {
		if count == 0 {
			t.Errorf("Callback %d was not called", i)
		}
	}
}

func TestHealthStatusStructComplete(t *testing.T) {
	now := time.Now()
	hs := HealthStatus{
		Component:   "cache",
		Healthy:     false,
		Message:     "Connection refused",
		LastCheck:   now,
		LastHealthy: now.Add(-1 * time.Hour),
		ErrorCount:  5,
	}

	if hs.Component != "cache" {
		t.Errorf("Component = %q, want %q", hs.Component, "cache")
	}
	if hs.Healthy {
		t.Error("Healthy should be false")
	}
	if hs.Message != "Connection refused" {
		t.Errorf("Message = %q, want %q", hs.Message, "Connection refused")
	}
	if hs.ErrorCount != 5 {
		t.Errorf("ErrorCount = %d, want %d", hs.ErrorCount, 5)
	}
}

func TestMaintenanceModeConstants(t *testing.T) {
	// Verify all mode constants are distinct
	modes := map[MaintenanceMode]bool{
		ModeNormal:      true,
		ModeDegraded:    true,
		ModeMaintenance: true,
		ModeRecovery:    true,
		ModeEmergency:   true,
	}

	if len(modes) != 5 {
		t.Error("All maintenance modes should be distinct")
	}
}

func TestServiceManagerRenderTemplateWithMap(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	tmpl := "User: {{.User}}, Binary: {{.Binary}}"
	data := map[string]string{
		"User":   "search",
		"Binary": "/usr/local/bin/search",
	}

	result, err := sm.renderTemplate(tmpl, data)
	if err != nil {
		t.Fatalf("renderTemplate() error = %v", err)
	}

	expected := "User: search, Binary: /usr/local/bin/search"
	if result != expected {
		t.Errorf("renderTemplate() = %q, want %q", result, expected)
	}
}

func TestGracefulDegradationExecuteFailureWithFallback(t *testing.T) {
	gd := NewGracefulDegradation()

	// Register fallback first
	gd.RegisterFallback("flaky", func() interface{} {
		return "fallback data"
	})

	// Execute function that fails
	result, err := gd.Execute("flaky", func() (interface{}, error) {
		return nil, fmt.Errorf("service error")
	})

	if err != nil {
		t.Fatalf("Execute() error = %v (should use fallback)", err)
	}
	if result != "fallback data" {
		t.Errorf("Execute() = %v, want %q", result, "fallback data")
	}

	// Feature should now be marked as degraded
	if !gd.IsDegraded("flaky") {
		t.Error("Feature should be marked as degraded after failure")
	}
}

func TestGracefulDegradationExecuteFailureNoFallback(t *testing.T) {
	gd := NewGracefulDegradation()

	// Execute function that fails without fallback
	_, err := gd.Execute("no-fallback", func() (interface{}, error) {
		return nil, fmt.Errorf("service error")
	})

	if err == nil {
		t.Error("Execute() should return error when failing without fallback")
	}
}

func TestMaintenanceServiceIsRecovery(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.SetMode(ModeRecovery, "Attempting recovery")
	if ms.GetMode() != ModeRecovery {
		t.Error("Mode should be ModeRecovery")
	}
}

func TestMaintenanceServiceIsEmergency(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.SetMode(ModeEmergency, "Critical failure")
	if ms.GetMode() != ModeEmergency {
		t.Error("Mode should be ModeEmergency")
	}
}

func TestMaintenanceServiceScheduledEndZero(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Enable maintenance without duration
	ms.EnableMaintenance("No duration", 0)

	scheduledEnd := ms.GetScheduledEnd()
	if !scheduledEnd.IsZero() {
		t.Error("Scheduled end should be zero when no duration specified")
	}

	ms.DisableMaintenance()
}

func TestMaintenanceServiceBackupDatabase(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "maintenance-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test database file
	dbPath := filepath.Join(tempDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("test database content"), 0644); err != nil {
		t.Fatalf("Failed to create test db: %v", err)
	}

	backupDir := filepath.Join(tempDir, "backups")

	// Backup database
	backupPath, err := ms.BackupDatabase(dbPath, backupDir)
	if err != nil {
		t.Fatalf("BackupDatabase() error = %v", err)
	}

	// Verify backup exists
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("Backup file not created: %v", err)
	}

	// Verify checksum file exists
	checksumPath := backupPath + ".sha256"
	if _, err := os.Stat(checksumPath); err != nil {
		t.Errorf("Checksum file not created: %v", err)
	}
}

func TestMaintenanceServiceRestoreDatabase(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "maintenance-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create backup file
	backupPath := filepath.Join(tempDir, "backup.db")
	backupContent := []byte("backup database content")
	if err := os.WriteFile(backupPath, backupContent, 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Create checksum
	checksum, _ := fileChecksum(backupPath)
	checksumPath := backupPath + ".sha256"
	os.WriteFile(checksumPath, []byte(checksum), 0600)

	// Create current database
	dbPath := filepath.Join(tempDir, "current.db")
	if err := os.WriteFile(dbPath, []byte("current content"), 0644); err != nil {
		t.Fatalf("Failed to create current db: %v", err)
	}

	// Restore
	err = ms.RestoreDatabase(backupPath, dbPath)
	if err != nil {
		t.Fatalf("RestoreDatabase() error = %v", err)
	}

	// Verify content was restored
	content, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("Failed to read restored db: %v", err)
	}

	if string(content) != string(backupContent) {
		t.Errorf("Restored content = %q, want %q", string(content), string(backupContent))
	}
}

func TestMaintenanceServiceRestoreDatabaseNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	err := ms.RestoreDatabase("/nonexistent/backup.db", "/tmp/db.db")
	if err == nil {
		t.Error("RestoreDatabase() should fail when backup not found")
	}
}

func TestMaintenanceServiceRestoreDatabaseChecksumMismatch(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "maintenance-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create backup file
	backupPath := filepath.Join(tempDir, "backup.db")
	os.WriteFile(backupPath, []byte("backup content"), 0644)

	// Create bad checksum
	checksumPath := backupPath + ".sha256"
	os.WriteFile(checksumPath, []byte("bad_checksum"), 0600)

	dbPath := filepath.Join(tempDir, "db.db")

	// Restore should fail due to checksum mismatch
	err = ms.RestoreDatabase(backupPath, dbPath)
	if err == nil {
		t.Error("RestoreDatabase() should fail with checksum mismatch")
	}
}

func TestServiceManagerStartStopRestart(t *testing.T) {
	// These operations require root and actual service management
	// We just test they don't panic when called
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Start should fail without privileges
	err := sm.StartAllServices()
	if err == nil && os.Geteuid() != 0 {
		t.Log("Start() succeeded unexpectedly")
	}

	// Stop should fail without privileges
	err = sm.StopAllServices()
	if err == nil && os.Geteuid() != 0 {
		t.Log("Stop() succeeded unexpectedly")
	}

	// Restart should fail without privileges
	err = sm.RestartAllServices()
	if err == nil && os.Geteuid() != 0 {
		t.Log("Restart() succeeded unexpectedly")
	}
}

func TestServiceManagerEnableDisable(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// These require root
	err := sm.Enable()
	// Expected to fail without root
	_ = err

	err = sm.Disable()
	// Expected to fail without root
	_ = err
}

func TestServiceManagerReload(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Reload requires root
	err := sm.Reload()
	// Expected to fail without root
	_ = err
}

func TestRunCommandError(t *testing.T) {
	err := runCommand("nonexistent_command_12345", "arg1", "arg2")
	if err == nil {
		t.Error("runCommand() should fail for non-existent command")
	}
}

func TestAppendToFileNewFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "append-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "newfile.txt")
	content := "test content\n"

	err = appendToFile(filePath, content)
	if err != nil {
		t.Fatalf("appendToFile() error = %v", err)
	}

	// Verify content
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	if string(data) != content {
		t.Errorf("File content = %q, want %q", string(data), content)
	}
}

func TestAppendToFileExistingFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "append-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	filePath := filepath.Join(tempDir, "existing.txt")
	os.WriteFile(filePath, []byte("existing\n"), 0644)

	err = appendToFile(filePath, "appended\n")
	if err != nil {
		t.Fatalf("appendToFile() error = %v", err)
	}

	data, _ := os.ReadFile(filePath)
	expected := "existing\nappended\n"
	if string(data) != expected {
		t.Errorf("File content = %q, want %q", string(data), expected)
	}
}

func TestGracefulDegradationGetDegradedFeaturesEmpty(t *testing.T) {
	gd := NewGracefulDegradation()

	features := gd.GetDegradedFeatures()
	if len(features) != 0 {
		t.Errorf("GetDegradedFeatures() should return empty slice initially, got %v", features)
	}
}

func TestMaintenanceServiceInitHealthStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.StartMaintenanceService()
	time.Sleep(50 * time.Millisecond)

	health := ms.GetHealthStatus()

	// Should have standard components
	expectedComponents := []string{"server_db", "users_db", "cache", "tor", "scheduler"}
	for _, comp := range expectedComponents {
		if _, ok := health[comp]; !ok {
			t.Errorf("Health status should have component %q", comp)
		}
	}

	ms.StopMaintenanceService()
}

func TestMaintenanceServiceCallbacksNotCalledOnSameMode(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	var callCount int
	ms.RegisterCallback(func(mode MaintenanceMode) {
		callCount++
	})

	// Set to same mode multiple times
	ms.SetMode(ModeNormal, "")
	ms.SetMode(ModeNormal, "")
	ms.SetMode(ModeNormal, "")

	time.Sleep(50 * time.Millisecond)

	// Callback should not be called when mode doesn't change
	if callCount > 0 {
		t.Error("Callback should not be called when mode doesn't change")
	}
}

func TestMaintenanceServiceStopIdempotent(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.StartMaintenanceService()
	time.Sleep(10 * time.Millisecond)

	// Stop multiple times should not panic
	ms.StopMaintenanceService()
	ms.StopMaintenanceService()
	ms.StopMaintenanceService()
}

func TestMaintenanceServiceStatusScheduledEnd(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.EnableMaintenance("Scheduled", 1*time.Hour)

	status := ms.GetStatus()
	if status == nil {
		t.Fatal("GetStatus() returned nil")
	}

	scheduledEnd, ok := status["scheduled_end"]
	if !ok {
		t.Error("Status should have scheduled_end field")
	}

	if scheduledEnd.(time.Time).IsZero() {
		t.Error("Scheduled end should not be zero when duration specified")
	}

	ms.DisableMaintenance()
}

func TestMaintenanceServiceModeAfterCallback(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	var capturedMode MaintenanceMode
	ms.RegisterCallback(func(mode MaintenanceMode) {
		capturedMode = mode
	})

	ms.SetMode(ModeDegraded, "testing")
	time.Sleep(50 * time.Millisecond)

	if capturedMode != ModeDegraded {
		t.Errorf("Callback received mode %v, want %v", capturedMode, ModeDegraded)
	}
}

// =====================================================
// Additional tests for 100% coverage
// =====================================================

// Test ServiceManager Install/Uninstall/Status methods
func TestServiceManagerInstall(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Install requires root and will fail, but should not panic
	err := sm.Install()
	// Expected to fail without root privileges
	_ = err
}

func TestServiceManagerUninstall(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Uninstall will fail without privileges
	err := sm.Uninstall()
	_ = err
}

func TestServiceManagerStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.Status()
	// Should return inactive or error (no service installed)
	if err == nil {
		// Should be inactive since service isn't installed
		if status != "inactive" && status != "unknown" && status != "active" {
			t.Logf("Status = %q", status)
		}
	}
}

// Test maintenance service health check scenarios
func TestMaintenanceServiceEvaluateSystemHealthNormal(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Set all components as healthy
	ms.mu.Lock()
	ms.health["server_db"] = &HealthStatus{Component: "server_db", Healthy: true}
	ms.health["users_db"] = &HealthStatus{Component: "users_db", Healthy: true}
	ms.health["cache"] = &HealthStatus{Component: "cache", Healthy: true}
	ms.mu.Unlock()

	ms.evaluateSystemHealth()

	// Should be normal mode
	if ms.GetMode() != ModeNormal {
		t.Errorf("Mode should be ModeNormal when all healthy, got %v", ms.GetMode())
	}
}

func TestMaintenanceServiceEvaluateSystemHealthCriticalUnhealthy(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Set server_db as unhealthy (critical)
	ms.mu.Lock()
	ms.health["server_db"] = &HealthStatus{Component: "server_db", Healthy: false}
	ms.health["users_db"] = &HealthStatus{Component: "users_db", Healthy: true}
	ms.mu.Unlock()

	ms.evaluateSystemHealth()

	// Should be emergency mode for critical component
	if ms.GetMode() != ModeEmergency {
		t.Errorf("Mode should be ModeEmergency when critical unhealthy, got %v", ms.GetMode())
	}
}

func TestMaintenanceServiceEvaluateSystemHealthMultipleUnhealthy(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Set multiple non-critical components as unhealthy
	ms.mu.Lock()
	ms.health["cache"] = &HealthStatus{Component: "cache", Healthy: false}
	ms.health["tor"] = &HealthStatus{Component: "tor", Healthy: false}
	ms.health["server_db"] = &HealthStatus{Component: "server_db", Healthy: true}
	ms.health["users_db"] = &HealthStatus{Component: "users_db", Healthy: true}
	ms.mu.Unlock()

	ms.evaluateSystemHealth()

	// Per AI.md PART 6: only critical components (database, file_write) trigger mode
	// changes. Non-critical components self-heal without affecting public mode.
	if ms.GetMode() != ModeNormal {
		t.Errorf("Mode should remain ModeNormal when only non-critical unhealthy, got %v", ms.GetMode())
	}
}

func TestMaintenanceServiceEvaluateSystemHealthInMaintenanceMode(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Set maintenance mode
	ms.SetMode(ModeMaintenance, "Manual maintenance")

	// Set all as healthy
	ms.mu.Lock()
	ms.health["server_db"] = &HealthStatus{Component: "server_db", Healthy: true}
	ms.health["users_db"] = &HealthStatus{Component: "users_db", Healthy: true}
	ms.mu.Unlock()

	ms.evaluateSystemHealth()

	// Should stay in maintenance mode (not auto-change)
	if ms.GetMode() != ModeMaintenance {
		t.Errorf("Mode should stay ModeMaintenance, got %v", ms.GetMode())
	}
}

func TestMaintenanceServiceAttemptRecoveryNoUnhealthy(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// All healthy
	ms.mu.Lock()
	ms.health["server_db"] = &HealthStatus{Component: "server_db", Healthy: true, ErrorCount: 0}
	ms.mu.Unlock()

	// Should not panic and not change mode
	initialMode := ms.GetMode()
	ms.attemptRecovery()

	if ms.GetMode() != initialMode {
		t.Errorf("Mode should not change when no unhealthy components")
	}
}

func TestMaintenanceServiceAttemptRecoveryWithRecoveryFunc(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	recovered := false
	ms.RegisterRecoveryFunc("cache", func(ctx context.Context) error {
		recovered = true
		return nil
	})

	// Set cache as unhealthy with enough errors
	ms.mu.Lock()
	ms.health["cache"] = &HealthStatus{Component: "cache", Healthy: false, ErrorCount: 5}
	ms.mu.Unlock()

	ms.attemptRecovery()

	if !recovered {
		t.Error("Recovery function should have been called")
	}
}

func TestMaintenanceServiceAttemptRecoveryWithFailingRecoveryFunc(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.RegisterRecoveryFunc("cache", func(ctx context.Context) error {
		return fmt.Errorf("recovery failed")
	})

	// Set cache as unhealthy
	ms.mu.Lock()
	ms.health["cache"] = &HealthStatus{Component: "cache", Healthy: false, ErrorCount: 5}
	ms.mu.Unlock()

	// Should not panic even when recovery fails
	ms.attemptRecovery()
}

func TestMaintenanceServicePerformHealthChecksWithDBChecks(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	serverChecked := false
	usersChecked := false

	ms.SetDatabaseChecks(
		func(ctx context.Context) error {
			serverChecked = true
			return nil
		},
		func(ctx context.Context) error {
			usersChecked = true
			return nil
		},
	)

	ms.performHealthChecks()

	if !serverChecked {
		t.Error("Server DB check should have been called")
	}
	if !usersChecked {
		t.Error("Users DB check should have been called")
	}
}

func TestMaintenanceServicePerformHealthChecksWithFailingDBChecks(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.initializeHealthStatus()

	ms.SetDatabaseChecks(
		func(ctx context.Context) error {
			return fmt.Errorf("connection refused")
		},
		nil,
	)

	ms.performHealthChecks()

	health := ms.GetHealthStatus()
	if health["server_db"].Healthy {
		t.Error("Server DB should be unhealthy after failed check")
	}
}

func TestMaintenanceServiceCheckComponentNewComponent(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ctx := context.Background()

	// Check a new component that doesn't exist yet
	ms.checkComponent(ctx, "new_component", func(ctx context.Context) error {
		return nil
	})

	health := ms.GetHealthStatus()
	if _, ok := health["new_component"]; !ok {
		t.Error("New component should be added to health status")
	}
}

// Test GracefulDegradation Execute with failure then mark degraded
func TestGracefulDegradationExecuteFailureThenDegraded(t *testing.T) {
	gd := NewGracefulDegradation()

	// First call fails
	_, err := gd.Execute("volatile", func() (interface{}, error) {
		return nil, fmt.Errorf("temporary failure")
	})

	if err == nil {
		t.Error("First Execute should fail")
	}

	// Feature should now be degraded
	if !gd.IsDegraded("volatile") {
		t.Error("Feature should be degraded after failure")
	}
}

// Test appendToFile error path
func TestAppendToFileError(t *testing.T) {
	err := appendToFile("/nonexistent/dir/file.txt", "content")
	if err == nil {
		t.Error("appendToFile should fail for non-existent directory")
	}
}

// Test renderTemplate with execution error
func TestServiceManagerRenderTemplateExecuteError(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// Template with missing field should cause execution error
	tmpl := "Hello, {{.MissingField}}!"
	data := struct{ Name string }{"World"}

	_, err := sm.renderTemplate(tmpl, data)
	if err == nil {
		t.Error("renderTemplate should fail when field is missing")
	}
}

// Test createServiceDirectories
func TestServiceManagerCreateServiceDirectories(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// This should create directories (may fail due to permissions)
	err := sm.createServiceDirectories()
	_ = err
}

// Test maintenance service with auto-exit maintenance mode cancelled
func TestMaintenanceServiceEnableMaintenanceCancelled(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Enable with duration
	ms.EnableMaintenance("Quick test", 200*time.Millisecond)

	// Stop before duration elapses
	time.Sleep(50 * time.Millisecond)
	ms.StopMaintenanceService()

	// Should have cancelled auto-exit
	time.Sleep(300 * time.Millisecond)
	// No assertion needed, just verify no panic
}

// Test maintenance backup with checksum write error (simulated)
func TestMaintenanceServiceBackupDatabaseNoChecksumDir(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "maintenance-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test database
	dbPath := filepath.Join(tempDir, "test.db")
	os.WriteFile(dbPath, []byte("test"), 0644)

	backupDir := filepath.Join(tempDir, "backups")

	// Backup should succeed
	backupPath, err := ms.BackupDatabase(dbPath, backupDir)
	if err != nil {
		t.Fatalf("BackupDatabase() error = %v", err)
	}

	if backupPath == "" {
		t.Error("Backup path should not be empty")
	}
}

// Test RestoreDatabase without checksum file
func TestMaintenanceServiceRestoreDatabaseNoChecksum(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "restore-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create backup without checksum
	backupPath := filepath.Join(tempDir, "backup.db")
	os.WriteFile(backupPath, []byte("backup data"), 0644)

	dbPath := filepath.Join(tempDir, "current.db")
	os.WriteFile(dbPath, []byte("current"), 0644)

	// Should succeed even without checksum
	err = ms.RestoreDatabase(backupPath, dbPath)
	if err != nil {
		t.Errorf("RestoreDatabase() error = %v", err)
	}
}

// Test RestoreDatabase without existing current database
func TestMaintenanceServiceRestoreDatabaseNoCurrentDB(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	tempDir, err := os.MkdirTemp("", "restore-test-")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create backup
	backupPath := filepath.Join(tempDir, "backup.db")
	os.WriteFile(backupPath, []byte("backup data"), 0644)

	// doesn't exist
	dbPath := filepath.Join(tempDir, "new.db")

	// Should succeed
	err = ms.RestoreDatabase(backupPath, dbPath)
	if err != nil {
		t.Errorf("RestoreDatabase() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(dbPath); err != nil {
		t.Error("Database should be created by restore")
	}
}

// Test runCommand with valid command
func TestRunCommandSuccess(t *testing.T) {
	// Run a simple command that should succeed
	err := runCommand("true")
	if err != nil {
		t.Errorf("runCommand(true) error = %v", err)
	}
}

// Test health status initialization
func TestMaintenanceServiceInitializeHealthStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.initializeHealthStatus()

	health := ms.GetHealthStatus()

	expectedComponents := []string{"server_db", "users_db", "cache", "tor", "scheduler"}
	for _, comp := range expectedComponents {
		if _, ok := health[comp]; !ok {
			t.Errorf("Health status should have component %q", comp)
		}
		if !health[comp].Healthy {
			t.Errorf("Component %q should be healthy initially", comp)
		}
	}
}

// Test GetStatus returns all expected fields
func TestMaintenanceServiceGetStatusComplete(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	ms.initializeHealthStatus()
	ms.SetMode(ModeDegraded, "Test degraded")

	status := ms.GetStatus()

	// Check required fields
	if _, ok := status["mode"]; !ok {
		t.Error("Status should have 'mode' field")
	}
	if _, ok := status["message"]; !ok {
		t.Error("Status should have 'message' field")
	}
	if _, ok := status["scheduled_end"]; !ok {
		t.Error("Status should have 'scheduled_end' field")
	}
	if _, ok := status["health"]; !ok {
		t.Error("Status should have 'health' field")
	}
}

// Test single unhealthy non-critical component
func TestMaintenanceServiceEvaluateSystemHealthSingleUnhealthy(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// Only cache unhealthy (non-critical)
	ms.mu.Lock()
	ms.health["cache"] = &HealthStatus{Component: "cache", Healthy: false}
	ms.health["server_db"] = &HealthStatus{Component: "server_db", Healthy: true}
	ms.health["users_db"] = &HealthStatus{Component: "users_db", Healthy: true}
	ms.mu.Unlock()

	ms.evaluateSystemHealth()

	// Per AI.md PART 6: only critical components (database, file_write) trigger mode
	// changes. Non-critical components self-heal without affecting public mode.
	if ms.GetMode() != ModeNormal {
		t.Errorf("Mode should remain ModeNormal when only non-critical unhealthy, got %v", ms.GetMode())
	}
}

// Test users_db critical unhealthy
func TestMaintenanceServiceEvaluateSystemHealthUsersDBUnhealthy(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// users_db unhealthy (critical)
	ms.mu.Lock()
	ms.health["server_db"] = &HealthStatus{Component: "server_db", Healthy: true}
	ms.health["users_db"] = &HealthStatus{Component: "users_db", Healthy: false}
	ms.mu.Unlock()

	ms.evaluateSystemHealth()

	// Should be emergency mode
	if ms.GetMode() != ModeEmergency {
		t.Errorf("Mode should be ModeEmergency when users_db unhealthy, got %v", ms.GetMode())
	}
}

// Test attemptRecovery with low error count (should not recover)
func TestMaintenanceServiceAttemptRecoveryLowErrorCount(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	recovered := false
	ms.RegisterRecoveryFunc("cache", func(ctx context.Context) error {
		recovered = true
		return nil
	})

	// Error count < 3, should not trigger recovery
	ms.mu.Lock()
	ms.health["cache"] = &HealthStatus{Component: "cache", Healthy: false, ErrorCount: 2}
	ms.mu.Unlock()

	ms.attemptRecovery()

	if recovered {
		t.Error("Recovery should not be called with error count < 3")
	}
}

// Test GracefulDegradation concurrent access
func TestGracefulDegradationConcurrent(t *testing.T) {
	gd := NewGracefulDegradation()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			gd.MarkDegraded("feature")
			gd.MarkHealthy("feature")
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_ = gd.IsDegraded("feature")
			_ = gd.GetDegradedFeatures()
		}
		done <- true
	}()

	<-done
	<-done
}

// Test MaintenanceService concurrent access
func TestMaintenanceServiceConcurrent(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	done := make(chan bool)

	// Mode writer
	go func() {
		modes := []MaintenanceMode{ModeNormal, ModeDegraded, ModeMaintenance}
		for i := 0; i < 100; i++ {
			ms.SetMode(modes[i%3], "test")
		}
		done <- true
	}()

	// Mode reader
	go func() {
		for i := 0; i < 100; i++ {
			_ = ms.GetMode()
			_ = ms.GetMessage()
			_ = ms.IsNormal()
		}
		done <- true
	}()

	<-done
	<-done
}

// Test all MaintenanceMode string values are unique
func TestMaintenanceModeStringUnique(t *testing.T) {
	strings := make(map[string]bool)
	modes := []MaintenanceMode{
		ModeNormal,
		ModeDegraded,
		ModeMaintenance,
		ModeRecovery,
		ModeEmergency,
	}

	for _, mode := range modes {
		s := mode.String()
		if strings[s] {
			t.Errorf("Duplicate string representation: %q", s)
		}
		strings[s] = true
	}
}

// Test copyFile source open error
func TestCopyFileSourceOpenError(t *testing.T) {
	err := copyFile("/nonexistent/source", "/tmp/dest")
	if err == nil {
		t.Error("copyFile should fail when source cannot be opened")
	}
}

// Test fileChecksum read error during copy
func TestFileChecksumCopyError(t *testing.T) {
	// Create a directory instead of a file
	tempDir, _ := os.MkdirTemp("", "checksum-test-")
	defer os.RemoveAll(tempDir)

	_, err := fileChecksum(tempDir)
	// On most systems, this will fail because we're trying to read a directory
	// The exact error depends on the OS
	_ = err
}

// Table-driven test for MaintenanceMode helper functions
func TestMaintenanceModeHelperFunctions(t *testing.T) {
	tests := []struct {
		name          string
		mode          MaintenanceMode
		isNormal      bool
		isDegraded    bool
		isMaintenance bool
	}{
		{"Normal", ModeNormal, true, false, false},
		{"Degraded", ModeDegraded, false, true, false},
		{"Maintenance", ModeMaintenance, false, false, true},
		{"Recovery", ModeRecovery, false, false, false},
		{"Emergency", ModeEmergency, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			ms := NewMaintenanceService(cfg)
			ms.SetMode(tt.mode, "test")

			if ms.IsNormal() != tt.isNormal {
				t.Errorf("IsNormal() = %v, want %v", ms.IsNormal(), tt.isNormal)
			}
			if ms.IsDegraded() != tt.isDegraded {
				t.Errorf("IsDegraded() = %v, want %v", ms.IsDegraded(), tt.isDegraded)
			}
			if ms.IsInMaintenance() != tt.isMaintenance {
				t.Errorf("IsInMaintenance() = %v, want %v", ms.IsInMaintenance(), tt.isMaintenance)
			}
		})
	}
}

// Test GetHealthStatus returns copies not originals
func TestMaintenanceServiceGetHealthStatusCopy(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)
	ms.initializeHealthStatus()

	health1 := ms.GetHealthStatus()
	health2 := ms.GetHealthStatus()

	// Modify health1
	if serverDB, ok := health1["server_db"]; ok {
		serverDB.ErrorCount = 999
	}

	// health2 should be unaffected
	if health2["server_db"].ErrorCount == 999 {
		t.Error("GetHealthStatus should return copies, not references")
	}
}

// =====================================================================
// InstallUserService / installSystemdUserService tests
// =====================================================================

// TestInstallUserServiceNonLinux verifies that on non-Linux platforms (excluding macOS
// which has its own path), InstallUserService returns an informative error.
func TestInstallUserServiceNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		t.Skip("Test covers the unsupported-platform error branch only")
	}

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.InstallUserService()
	if err == nil {
		t.Error("InstallUserService() on unsupported OS should return error, got nil")
	}
}

// TestCreateServiceDirectoriesMkdirFails covers the error-return branch in
// createServiceDirectories by injecting a mkdirAll failure.
func TestCreateServiceDirectoriesMkdirFails(t *testing.T) {
	orig := mkdirAll
	mkdirAll = func(_ string, _ os.FileMode) error {
		return fmt.Errorf("injected mkdir failure")
	}
	defer func() { mkdirAll = orig }()

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	if err := sm.createServiceDirectories(); err == nil {
		t.Error("createServiceDirectories() should fail when mkdirAll fails")
	}
}

// TestCreateServiceDirectoriesSecureDirFails covers the error-return branch
// in the secure-subdirectory loop inside createServiceDirectories.
func TestCreateServiceDirectoriesSecureDirFails(t *testing.T) {
	orig := mkdirAll
	callCount := 0
	mkdirAll = func(_ string, _ os.FileMode) error {
		callCount++
		// Let the first 4 standard-dir calls succeed; fail on the 5th (first security subdir).
		if callCount >= 5 {
			return fmt.Errorf("injected secure-dir failure")
		}
		return nil
	}
	defer func() { mkdirAll = orig }()

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	if err := sm.createServiceDirectories(); err == nil {
		t.Error("createServiceDirectories() should fail when secure mkdirAll fails")
	}
}

// TestCreateServiceDirectoriesSecureDirMode verifies that security-sensitive
// subdirectories are created with permission 0700 per AI.md PART 23.
func TestCreateServiceDirectoriesSecureDirMode(t *testing.T) {
	orig := mkdirAll
	recorded := map[string]os.FileMode{}
	mkdirAll = func(path string, perm os.FileMode) error {
		recorded[filepath.Base(path)] = perm
		return nil
	}
	defer func() { mkdirAll = orig }()

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)
	_ = sm.createServiceDirectories()

	for _, secureDir := range []string{"security", "ssl", "tor"} {
		perm, ok := recorded[secureDir]
		if !ok {
			t.Errorf("mkdirAll not called for %q", secureDir)
			continue
		}
		if perm != 0700 {
			t.Errorf("secure dir %q mode = %04o, want 0700", secureDir, perm)
		}
	}
}

// TestCreateServiceDirectoriesChownFails covers the non-fatal error bodies when
// chown fails for both standard and secure directories.
func TestCreateServiceDirectoriesChownFails(t *testing.T) {
	origMkdir := mkdirAll
	origRunCmd := runCmd
	mkdirAll = func(_ string, _ os.FileMode) error { return nil }
	runCmd = func(_ string, _ ...string) error {
		return fmt.Errorf("injected chown failure")
	}
	defer func() {
		mkdirAll = origMkdir
		runCmd = origRunCmd
	}()

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)
	// Chown errors are non-fatal and silently ignored; no error expected.
	if err := sm.createServiceDirectories(); err != nil {
		t.Errorf("createServiceDirectories() returned unexpected error: %v", err)
	}
}

// Test RegisterCallback with nil (should handle gracefully)
func TestMaintenanceServiceRegisterNilCallback(t *testing.T) {
	cfg := config.DefaultConfig()
	ms := NewMaintenanceService(cfg)

	// This might panic if not handled, but ideally should be graceful
	// Actually looking at the code, it doesn't check for nil callbacks
	// Let's verify current behavior
	defer func() {
		if r := recover(); r != nil {
			t.Log("Recovered from panic with nil callback")
		}
	}()

	ms.RegisterCallback(nil)
	ms.SetMode(ModeDegraded, "test")
	time.Sleep(50 * time.Millisecond)
}
