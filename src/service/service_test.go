package service

import (
	"context"
	"os"
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

func TestServiceManagerHasRunit(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Test only runs on Linux")
	}

	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	// hasRunit should not panic
	result := sm.hasRunit()
	// We can't predict the result, just ensure it doesn't panic
	_ = result
}

func TestServiceManagerGetLaunchdPath(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	path := sm.getLaunchdPath()
	if path != "/Library/LaunchDaemons/apimgr.search.plist" {
		t.Errorf("getLaunchdPath() = %q, want %q", path, "/Library/LaunchDaemons/apimgr.search.plist")
	}
}

func TestServiceManagerGetRunitServiceDir(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	dir := sm.getRunitServiceDir()
	// Should be one of the expected paths
	validPaths := map[string]bool{
		"/etc/sv/search":       true,
		"/etc/runit/sv/search": true,
	}

	if !validPaths[dir] {
		t.Errorf("getRunitServiceDir() = %q, want one of %v", dir, validPaths)
	}
}

func TestServiceManagerGetRunitActiveDir(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	dir := sm.getRunitActiveDir()
	// Should be one of the expected paths
	validPaths := map[string]bool{
		"/var/service": true,
		"/service":     true,
	}

	if !validPaths[dir] {
		t.Errorf("getRunitActiveDir() = %q, want one of %v", dir, validPaths)
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

func TestSystemdTemplate(t *testing.T) {
	// Verify systemd template contains required elements per AI.md PART 25
	if systemdTemplate == "" {
		t.Fatal("systemdTemplate is empty")
	}

	required := []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"ExecStart=/usr/local/bin/search",
		"Restart=on-failure",
		"ProtectSystem=strict",
		"ProtectHome=yes",
		"PrivateTmp=yes",
	}

	for _, req := range required {
		if !contains(systemdTemplate, req) {
			t.Errorf("systemdTemplate missing required element: %q", req)
		}
	}
}

func TestRunitRunTemplate(t *testing.T) {
	if runitRunTemplate == "" {
		t.Fatal("runitRunTemplate is empty")
	}

	if !contains(runitRunTemplate, "#!/bin/sh") {
		t.Error("runitRunTemplate missing shebang")
	}
	if !contains(runitRunTemplate, "exec") {
		t.Error("runitRunTemplate missing exec")
	}
}

func TestLaunchdTemplate(t *testing.T) {
	if launchdTemplate == "" {
		t.Fatal("launchdTemplate is empty")
	}

	required := []string{
		"<?xml",
		"<plist",
		"apimgr.search",
		"/usr/local/bin/search",
		"RunAtLoad",
		"KeepAlive",
	}

	for _, req := range required {
		if !contains(launchdTemplate, req) {
			t.Errorf("launchdTemplate missing required element: %q", req)
		}
	}
}

func TestRcdTemplate(t *testing.T) {
	if rcdTemplate == "" {
		t.Fatal("rcdTemplate is empty")
	}

	required := []string{
		"#!/bin/sh",
		"# PROVIDE:",
		"# REQUIRE:",
		"search_enable",
		"/usr/local/bin/search",
	}

	for _, req := range required {
		if !contains(rcdTemplate, req) {
			t.Errorf("rcdTemplate missing required element: %q", req)
		}
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
	if err := ms.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait a bit for initialization
	time.Sleep(50 * time.Millisecond)

	// Stop should succeed
	ms.Stop()

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
	if err := ms.Start(); err != nil {
		t.Fatalf("First Start() error = %v", err)
	}

	// Second start should be no-op
	if err := ms.Start(); err != nil {
		t.Fatalf("Second Start() error = %v", err)
	}

	ms.Stop()
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

	ms.Stop()
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
	ms.Start()
	time.Sleep(50 * time.Millisecond)

	health := ms.GetHealthStatus()
	if health == nil {
		t.Fatal("GetHealthStatus() returned nil")
	}

	// Should have some components
	if len(health) == 0 {
		t.Error("GetHealthStatus() should return some components")
	}

	ms.Stop()
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
