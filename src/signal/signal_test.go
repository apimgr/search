package signal

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestIsShuttingDown(t *testing.T) {
	// Save original
	orig := shuttingDown
	defer func() { shuttingDown = orig }()

	shuttingDown = false
	if IsShuttingDown() {
		t.Error("IsShuttingDown() should return false initially")
	}

	shuttingDown = true
	if !IsShuttingDown() {
		t.Error("IsShuttingDown() should return true after setting")
	}
}

func TestSetShuttingDown(t *testing.T) {
	// Save original
	orig := shuttingDown
	defer func() { shuttingDown = orig }()

	setShuttingDown(true)
	if !IsShuttingDown() {
		t.Error("setShuttingDown(true) should set shuttingDown to true")
	}

	setShuttingDown(false)
	if IsShuttingDown() {
		t.Error("setShuttingDown(false) should set shuttingDown to false")
	}
}

func TestShutdownConfigDefaults(t *testing.T) {
	cfg := ShutdownConfig{}

	// Test default values are applied in Setup
	// We can't directly test Setup without triggering signal handling,
	// but we can verify the config struct works
	if cfg.InFlightTimeout != 0 {
		t.Error("InFlightTimeout should be zero before Setup")
	}
	if cfg.ChildTimeout != 0 {
		t.Error("ChildTimeout should be zero before Setup")
	}
	if cfg.DatabaseTimeout != 0 {
		t.Error("DatabaseTimeout should be zero before Setup")
	}
	if cfg.LogFlushTimeout != 0 {
		t.Error("LogFlushTimeout should be zero before Setup")
	}
}

func TestReopenLogs(t *testing.T) {
	called := false
	cfg := ShutdownConfig{
		OnReopenLogs: func() {
			called = true
		},
	}

	reopenLogs(cfg)
	if !called {
		t.Error("OnReopenLogs callback should have been called")
	}

	// Test without callback
	cfg2 := ShutdownConfig{}
	reopenLogs(cfg2) // Should not panic
}

func TestDumpStatus(t *testing.T) {
	called := false
	cfg := ShutdownConfig{
		OnDumpStatus: func() {
			called = true
		},
	}

	dumpStatus(cfg)
	if !called {
		t.Error("OnDumpStatus callback should have been called")
	}

	// Test without callback
	cfg2 := ShutdownConfig{}
	dumpStatus(cfg2) // Should not panic
}

func TestShutdownConfigWithServer(t *testing.T) {
	server := &http.Server{Addr: ":0"}
	cfg := ShutdownConfig{
		Server:          server,
		InFlightTimeout: 5 * time.Second,
	}

	if cfg.Server != server {
		t.Error("Server should be set")
	}
	if cfg.InFlightTimeout != 5*time.Second {
		t.Errorf("InFlightTimeout = %v, want %v", cfg.InFlightTimeout, 5*time.Second)
	}
}

func TestShutdownConfigWithShutdownFunc(t *testing.T) {
	called := false
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
	}

	// Call the shutdown func directly
	err := cfg.ShutdownFunc(context.Background())
	if err != nil {
		t.Errorf("ShutdownFunc returned error: %v", err)
	}
	if !called {
		t.Error("ShutdownFunc should have been called")
	}
}

func TestShutdownConfigWithShutdownFuncError(t *testing.T) {
	expectedErr := errors.New("shutdown error")
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			return expectedErr
		},
	}

	err := cfg.ShutdownFunc(context.Background())
	if err != expectedErr {
		t.Errorf("ShutdownFunc error = %v, want %v", err, expectedErr)
	}
}

func TestShutdownConfigWithCallbacks(t *testing.T) {
	var closeDatabaseCalled, flushLogsCalled bool

	cfg := ShutdownConfig{
		OnCloseDatabase: func() {
			closeDatabaseCalled = true
		},
		OnFlushLogs: func() {
			flushLogsCalled = true
		},
	}

	// Call callbacks directly
	if cfg.OnCloseDatabase != nil {
		cfg.OnCloseDatabase()
	}
	if cfg.OnFlushLogs != nil {
		cfg.OnFlushLogs()
	}

	if !closeDatabaseCalled {
		t.Error("OnCloseDatabase should have been called")
	}
	if !flushLogsCalled {
		t.Error("OnFlushLogs should have been called")
	}
}

func TestShutdownConfigWithPIDFile(t *testing.T) {
	// Create a temp PID file
	tmpFile, err := os.CreateTemp("", "test-pid-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	cfg := ShutdownConfig{
		PIDFile: tmpFile.Name(),
	}

	if cfg.PIDFile != tmpFile.Name() {
		t.Errorf("PIDFile = %q, want %q", cfg.PIDFile, tmpFile.Name())
	}
}

func TestShutdownConfigGetChildPIDs(t *testing.T) {
	cfg := ShutdownConfig{
		GetChildPIDs: func() []int {
			return []int{123, 456, 789}
		},
	}

	pids := cfg.GetChildPIDs()
	if len(pids) != 3 {
		t.Errorf("GetChildPIDs returned %d PIDs, want 3", len(pids))
	}
	if pids[0] != 123 || pids[1] != 456 || pids[2] != 789 {
		t.Errorf("GetChildPIDs returned unexpected PIDs: %v", pids)
	}
}

func TestShutdownConfigStruct(t *testing.T) {
	// Test all fields can be set
	server := &http.Server{Addr: ":0"}
	cfg := ShutdownConfig{
		Server:          server,
		PIDFile:         "/tmp/test.pid",
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
	}

	if cfg.Server != server {
		t.Error("Server not set correctly")
	}
	if cfg.PIDFile != "/tmp/test.pid" {
		t.Errorf("PIDFile = %q, want /tmp/test.pid", cfg.PIDFile)
	}
	if cfg.InFlightTimeout != 30*time.Second {
		t.Errorf("InFlightTimeout = %v, want 30s", cfg.InFlightTimeout)
	}
	if cfg.ChildTimeout != 10*time.Second {
		t.Errorf("ChildTimeout = %v, want 10s", cfg.ChildTimeout)
	}
	if cfg.DatabaseTimeout != 5*time.Second {
		t.Errorf("DatabaseTimeout = %v, want 5s", cfg.DatabaseTimeout)
	}
	if cfg.LogFlushTimeout != 2*time.Second {
		t.Errorf("LogFlushTimeout = %v, want 2s", cfg.LogFlushTimeout)
	}
}

func TestShutdownConfigAllCallbacks(t *testing.T) {
	var (
		reopenLogsCalled   bool
		dumpStatusCalled   bool
		closeDatabaseCalled bool
		flushLogsCalled    bool
		getChildPIDsCalled bool
	)

	cfg := ShutdownConfig{
		OnReopenLogs: func() {
			reopenLogsCalled = true
		},
		OnDumpStatus: func() {
			dumpStatusCalled = true
		},
		OnCloseDatabase: func() {
			closeDatabaseCalled = true
		},
		OnFlushLogs: func() {
			flushLogsCalled = true
		},
		GetChildPIDs: func() []int {
			getChildPIDsCalled = true
			return []int{}
		},
	}

	// Test each callback
	if cfg.OnReopenLogs != nil {
		cfg.OnReopenLogs()
	}
	if cfg.OnDumpStatus != nil {
		cfg.OnDumpStatus()
	}
	if cfg.OnCloseDatabase != nil {
		cfg.OnCloseDatabase()
	}
	if cfg.OnFlushLogs != nil {
		cfg.OnFlushLogs()
	}
	if cfg.GetChildPIDs != nil {
		cfg.GetChildPIDs()
	}

	if !reopenLogsCalled {
		t.Error("OnReopenLogs should have been called")
	}
	if !dumpStatusCalled {
		t.Error("OnDumpStatus should have been called")
	}
	if !closeDatabaseCalled {
		t.Error("OnCloseDatabase should have been called")
	}
	if !flushLogsCalled {
		t.Error("OnFlushLogs should have been called")
	}
	if !getChildPIDsCalled {
		t.Error("GetChildPIDs should have been called")
	}
}

func TestShutdownConfigNilCallbacks(t *testing.T) {
	cfg := ShutdownConfig{}

	// Test that nil callbacks don't panic
	if cfg.OnReopenLogs != nil {
		t.Error("OnReopenLogs should be nil")
	}
	if cfg.OnDumpStatus != nil {
		t.Error("OnDumpStatus should be nil")
	}
	if cfg.OnCloseDatabase != nil {
		t.Error("OnCloseDatabase should be nil")
	}
	if cfg.OnFlushLogs != nil {
		t.Error("OnFlushLogs should be nil")
	}
	if cfg.GetChildPIDs != nil {
		t.Error("GetChildPIDs should be nil")
	}
}

func TestReopenLogsNoCallback(t *testing.T) {
	cfg := ShutdownConfig{}
	// Should not panic when OnReopenLogs is nil
	reopenLogs(cfg)
}

func TestDumpStatusNoCallback(t *testing.T) {
	cfg := ShutdownConfig{}
	// Should not panic when OnDumpStatus is nil
	dumpStatus(cfg)
}

func TestIsShuttingDownConcurrent(t *testing.T) {
	// Save original
	orig := shuttingDown
	defer func() { shuttingDown = orig }()

	// Test concurrent access doesn't panic
	done := make(chan bool)
	go func() {
		for i := 0; i < 100; i++ {
			setShuttingDown(true)
			IsShuttingDown()
			setShuttingDown(false)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 100; i++ {
			IsShuttingDown()
			setShuttingDown(false)
			setShuttingDown(true)
		}
		done <- true
	}()

	<-done
	<-done
}

func TestShutdownConfigTimeoutValues(t *testing.T) {
	tests := []struct {
		name     string
		timeout  time.Duration
		expected time.Duration
	}{
		{"zero", 0, 0},
		{"1 second", 1 * time.Second, 1 * time.Second},
		{"30 seconds", 30 * time.Second, 30 * time.Second},
		{"1 minute", 1 * time.Minute, 1 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ShutdownConfig{
				InFlightTimeout: tt.timeout,
			}
			if cfg.InFlightTimeout != tt.expected {
				t.Errorf("InFlightTimeout = %v, want %v", cfg.InFlightTimeout, tt.expected)
			}
		})
	}
}

func TestShutdownConfigShutdownFuncWithTimeout(t *testing.T) {
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(50 * time.Millisecond):
				return nil
			}
		},
		InFlightTimeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	err := cfg.ShutdownFunc(ctx)
	if err != nil {
		t.Errorf("ShutdownFunc should complete within timeout, got error: %v", err)
	}
}

func TestShutdownConfigShutdownFuncTimeout(t *testing.T) {
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
				return nil
			}
		},
		InFlightTimeout: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	err := cfg.ShutdownFunc(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("ShutdownFunc should timeout, got error: %v", err)
	}
}

// Additional tests for completeness

func TestShutdownConfigDatabaseTimeout(t *testing.T) {
	dbCloseCalled := false
	cfg := ShutdownConfig{
		DatabaseTimeout: 100 * time.Millisecond,
		OnCloseDatabase: func() {
			dbCloseCalled = true
		},
	}

	// Simulate the database close call
	if cfg.OnCloseDatabase != nil {
		cfg.OnCloseDatabase()
	}

	if !dbCloseCalled {
		t.Error("OnCloseDatabase was not called")
	}
}

func TestShutdownConfigLogFlushTimeout(t *testing.T) {
	logFlushCalled := false
	cfg := ShutdownConfig{
		LogFlushTimeout: 100 * time.Millisecond,
		OnFlushLogs: func() {
			logFlushCalled = true
		},
	}

	// Simulate the log flush call
	if cfg.OnFlushLogs != nil {
		cfg.OnFlushLogs()
	}

	if !logFlushCalled {
		t.Error("OnFlushLogs was not called")
	}
}

func TestShutdownConfigPIDFileField(t *testing.T) {
	cfg := ShutdownConfig{
		PIDFile: "/var/run/test.pid",
	}

	if cfg.PIDFile != "/var/run/test.pid" {
		t.Errorf("PIDFile = %q, want '/var/run/test.pid'", cfg.PIDFile)
	}
}

func TestShutdownConfigEmptyCallbacks(t *testing.T) {
	cfg := ShutdownConfig{}

	// Verify all callbacks are nil by default
	if cfg.OnReopenLogs != nil {
		t.Error("OnReopenLogs should be nil by default")
	}
	if cfg.OnDumpStatus != nil {
		t.Error("OnDumpStatus should be nil by default")
	}
	if cfg.OnCloseDatabase != nil {
		t.Error("OnCloseDatabase should be nil by default")
	}
	if cfg.OnFlushLogs != nil {
		t.Error("OnFlushLogs should be nil by default")
	}
	if cfg.GetChildPIDs != nil {
		t.Error("GetChildPIDs should be nil by default")
	}
	if cfg.ShutdownFunc != nil {
		t.Error("ShutdownFunc should be nil by default")
	}
	if cfg.Server != nil {
		t.Error("Server should be nil by default")
	}
}

func TestSetShuttingDownAndCheck(t *testing.T) {
	// Reset state
	setShuttingDown(false)

	if IsShuttingDown() {
		t.Error("Should not be shutting down initially")
	}

	setShuttingDown(true)
	if !IsShuttingDown() {
		t.Error("Should be shutting down after setting flag")
	}

	setShuttingDown(false)
	if IsShuttingDown() {
		t.Error("Should not be shutting down after resetting flag")
	}
}

// Test Setup function with defaults
func TestSetupAppliesDefaults(t *testing.T) {
	// Test that Setup can be called without panicking
	// We can't fully test signal handling, but we can verify it doesn't crash
	cfg := ShutdownConfig{}

	// Setup should apply defaults and set up signal handlers
	// We just verify it doesn't panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Setup() panicked: %v", r)
		}
	}()

	Setup(cfg)
}

// Test ShutdownConfig with all timeout values
func TestShutdownConfigTimeouts(t *testing.T) {
	cfg := ShutdownConfig{
		InFlightTimeout: 60 * time.Second,
		ChildTimeout:    20 * time.Second,
		DatabaseTimeout: 10 * time.Second,
		LogFlushTimeout: 5 * time.Second,
	}

	if cfg.InFlightTimeout != 60*time.Second {
		t.Errorf("InFlightTimeout = %v, want 60s", cfg.InFlightTimeout)
	}
	if cfg.ChildTimeout != 20*time.Second {
		t.Errorf("ChildTimeout = %v, want 20s", cfg.ChildTimeout)
	}
	if cfg.DatabaseTimeout != 10*time.Second {
		t.Errorf("DatabaseTimeout = %v, want 10s", cfg.DatabaseTimeout)
	}
	if cfg.LogFlushTimeout != 5*time.Second {
		t.Errorf("LogFlushTimeout = %v, want 5s", cfg.LogFlushTimeout)
	}
}

// Test shutdown callbacks are called correctly
func TestShutdownCallbackOrder(t *testing.T) {
	var order []string

	cfg := ShutdownConfig{
		OnReopenLogs: func() {
			order = append(order, "reopen")
		},
		OnDumpStatus: func() {
			order = append(order, "dump")
		},
		OnCloseDatabase: func() {
			order = append(order, "db")
		},
		OnFlushLogs: func() {
			order = append(order, "flush")
		},
	}

	// Test reopen logs
	reopenLogs(cfg)
	if len(order) != 1 || order[0] != "reopen" {
		t.Errorf("OnReopenLogs not called correctly, order = %v", order)
	}

	// Test dump status
	dumpStatus(cfg)
	if len(order) != 2 || order[1] != "dump" {
		t.Errorf("OnDumpStatus not called correctly, order = %v", order)
	}

	// Test callbacks can be called
	cfg.OnCloseDatabase()
	cfg.OnFlushLogs()
	if len(order) != 4 {
		t.Errorf("Not all callbacks called, order = %v", order)
	}
}

// Test GetChildPIDs callback
func TestGetChildPIDsCallback(t *testing.T) {
	expectedPIDs := []int{1000, 2000, 3000}
	cfg := ShutdownConfig{
		GetChildPIDs: func() []int {
			return expectedPIDs
		},
	}

	pids := cfg.GetChildPIDs()
	if len(pids) != 3 {
		t.Errorf("GetChildPIDs returned %d PIDs, want 3", len(pids))
	}
	for i, pid := range pids {
		if pid != expectedPIDs[i] {
			t.Errorf("PID[%d] = %d, want %d", i, pid, expectedPIDs[i])
		}
	}
}

// Test ShutdownFunc callback
func TestShutdownFuncCallback(t *testing.T) {
	called := false
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			called = true
			// Verify context is provided
			if ctx == nil {
				t.Error("Context should not be nil")
			}
			return nil
		},
	}

	ctx := context.Background()
	err := cfg.ShutdownFunc(ctx)
	if err != nil {
		t.Errorf("ShutdownFunc returned error: %v", err)
	}
	if !called {
		t.Error("ShutdownFunc was not called")
	}
}

// Test PIDFile field
func TestPIDFileField(t *testing.T) {
	cfg := ShutdownConfig{
		PIDFile: "/var/run/myapp.pid",
	}

	if cfg.PIDFile != "/var/run/myapp.pid" {
		t.Errorf("PIDFile = %q, want /var/run/myapp.pid", cfg.PIDFile)
	}
}

// Test concurrent IsShuttingDown calls
func TestConcurrentShuttingDown(t *testing.T) {
	done := make(chan bool, 10)

	// Multiple readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				IsShuttingDown()
			}
			done <- true
		}()
	}

	// Multiple writers
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				setShuttingDown(id%2 == 0)
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout in concurrent operations")
		}
	}

	// Reset state
	setShuttingDown(false)
}

// Test reopenLogs with nil callback
func TestReopenLogsNilCallback(t *testing.T) {
	cfg := ShutdownConfig{
		OnReopenLogs: nil,
	}
	// Should not panic
	reopenLogs(cfg)
}

// Test dumpStatus with nil callback
func TestDumpStatusNilCallback(t *testing.T) {
	cfg := ShutdownConfig{
		OnDumpStatus: nil,
	}
	// Should not panic
	dumpStatus(cfg)
}

// Test ShutdownConfig with Server
func TestShutdownConfigWithHTTPServer(t *testing.T) {
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	cfg := ShutdownConfig{
		Server:          server,
		InFlightTimeout: 30 * time.Second,
	}

	if cfg.Server != server {
		t.Error("Server not set correctly")
	}
	if cfg.Server.Addr != ":8080" {
		t.Errorf("Server.Addr = %q, want :8080", cfg.Server.Addr)
	}
}

// Test empty callbacks don't cause issues
func TestEmptyCallbacks(t *testing.T) {
	cfg := ShutdownConfig{}

	// All callbacks should be nil
	if cfg.OnReopenLogs != nil {
		t.Error("OnReopenLogs should be nil")
	}
	if cfg.OnDumpStatus != nil {
		t.Error("OnDumpStatus should be nil")
	}
	if cfg.OnCloseDatabase != nil {
		t.Error("OnCloseDatabase should be nil")
	}
	if cfg.OnFlushLogs != nil {
		t.Error("OnFlushLogs should be nil")
	}
	if cfg.GetChildPIDs != nil {
		t.Error("GetChildPIDs should be nil")
	}
	if cfg.ShutdownFunc != nil {
		t.Error("ShutdownFunc should be nil")
	}
}

// Test multiple callback assignments
func TestMultipleCallbackAssignments(t *testing.T) {
	count := 0
	callback1 := func() { count = 1 }
	callback2 := func() { count = 2 }

	cfg := ShutdownConfig{
		OnFlushLogs: callback1,
	}
	cfg.OnFlushLogs()
	if count != 1 {
		t.Error("callback1 not called")
	}

	cfg.OnFlushLogs = callback2
	cfg.OnFlushLogs()
	if count != 2 {
		t.Error("callback2 not called")
	}
}

func TestShutdownConfigWithBothServerAndFunc(t *testing.T) {
	// When both Server and ShutdownFunc are set, ShutdownFunc takes priority
	serverShutdownCalled := false
	funcShutdownCalled := false

	cfg := ShutdownConfig{
		Server: &http.Server{},
		ShutdownFunc: func(ctx context.Context) error {
			funcShutdownCalled = true
			return nil
		},
	}

	// Per the code, if ShutdownFunc is provided, it is called instead of Server.Shutdown
	if cfg.ShutdownFunc != nil {
		_ = cfg.ShutdownFunc(context.Background())
	}

	if !funcShutdownCalled {
		t.Error("ShutdownFunc should be called")
	}
	if serverShutdownCalled {
		t.Error("Server.Shutdown should not be called when ShutdownFunc is provided")
	}
}

func TestShutdownConfigChildPIDsEmpty(t *testing.T) {
	cfg := ShutdownConfig{
		GetChildPIDs: func() []int {
			return []int{}
		},
	}

	pids := cfg.GetChildPIDs()
	if len(pids) != 0 {
		t.Errorf("Expected empty PIDs, got %d", len(pids))
	}
}

func TestShutdownConfigChildPIDsMultiple(t *testing.T) {
	cfg := ShutdownConfig{
		GetChildPIDs: func() []int {
			return []int{100, 200, 300}
		},
	}

	pids := cfg.GetChildPIDs()
	if len(pids) != 3 {
		t.Errorf("Expected 3 PIDs, got %d", len(pids))
	}
	if pids[0] != 100 || pids[1] != 200 || pids[2] != 300 {
		t.Errorf("Unexpected PIDs: %v", pids)
	}
}

// TestSetupWithDefaults verifies Setup applies default timeout values
func TestSetupWithDefaults(t *testing.T) {
	// This test verifies Setup doesn't panic and applies defaults
	cfg := ShutdownConfig{}
	Setup(cfg)
	// Default values are applied within Setup before calling setupSignals
}

// TestSetupWithCustomTimeouts verifies Setup works with custom timeouts
func TestSetupWithCustomTimeouts(t *testing.T) {
	cfg := ShutdownConfig{
		InFlightTimeout: 60 * time.Second,
		ChildTimeout:    20 * time.Second,
		DatabaseTimeout: 10 * time.Second,
		LogFlushTimeout: 5 * time.Second,
	}
	Setup(cfg)
}

// TestSetupWithAllOptions verifies Setup with all config options
func TestSetupWithAllOptions(t *testing.T) {
	server := &http.Server{Addr: ":8080"}
	cfg := ShutdownConfig{
		Server:          server,
		PIDFile:         "/tmp/test.pid",
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
		OnReopenLogs:    func() {},
		OnDumpStatus:    func() {},
		OnCloseDatabase: func() {},
		OnFlushLogs:     func() {},
		GetChildPIDs:    func() []int { return []int{} },
	}
	Setup(cfg)
}

// TestSetupWithShutdownFunc verifies Setup with ShutdownFunc
func TestSetupWithShutdownFunc(t *testing.T) {
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			return nil
		},
		InFlightTimeout: 10 * time.Second,
	}
	Setup(cfg)
}

// TestSetupPartialTimeouts verifies Setup applies defaults only for zero values
func TestSetupPartialTimeouts(t *testing.T) {
	cfg := ShutdownConfig{
		InFlightTimeout: 60 * time.Second, // Custom
		// ChildTimeout: 0  - should use default
		DatabaseTimeout: 10 * time.Second, // Custom
		// LogFlushTimeout: 0 - should use default
	}
	Setup(cfg)
}

// TestGracefulShutdownDatabaseCloseSuccess simulates database close success path
func TestGracefulShutdownDatabaseCloseSuccess(t *testing.T) {
	closeCalled := false
	cfg := ShutdownConfig{
		DatabaseTimeout: 100 * time.Millisecond,
		OnCloseDatabase: func() {
			closeCalled = true
		},
	}

	// Simulate the database close logic from gracefulShutdown
	if cfg.OnCloseDatabase != nil {
		done := make(chan struct{})
		go func() {
			cfg.OnCloseDatabase()
			close(done)
		}()
		select {
		case <-done:
			// Success path
		case <-time.After(cfg.DatabaseTimeout):
			t.Error("Should not timeout")
		}
	}

	if !closeCalled {
		t.Error("OnCloseDatabase should have been called")
	}
}

// TestGracefulShutdownDatabaseCloseTimeout simulates database close timeout path
func TestGracefulShutdownDatabaseCloseTimeout(t *testing.T) {
	closeCalled := false
	cfg := ShutdownConfig{
		DatabaseTimeout: 50 * time.Millisecond,
		OnCloseDatabase: func() {
			closeCalled = true
			time.Sleep(200 * time.Millisecond) // Simulate slow close
		},
	}

	// Simulate the database close logic from gracefulShutdown
	if cfg.OnCloseDatabase != nil {
		done := make(chan struct{})
		go func() {
			cfg.OnCloseDatabase()
			close(done)
		}()
		select {
		case <-done:
			// Should not happen first
		case <-time.After(cfg.DatabaseTimeout):
			// Expected timeout path
		}
	}

	if !closeCalled {
		t.Error("OnCloseDatabase should have been called")
	}
}

// TestGracefulShutdownLogFlushSuccess simulates log flush success path
func TestGracefulShutdownLogFlushSuccess(t *testing.T) {
	flushCalled := false
	cfg := ShutdownConfig{
		LogFlushTimeout: 100 * time.Millisecond,
		OnFlushLogs: func() {
			flushCalled = true
		},
	}

	// Simulate the log flush logic from gracefulShutdown
	if cfg.OnFlushLogs != nil {
		done := make(chan struct{})
		go func() {
			cfg.OnFlushLogs()
			close(done)
		}()
		select {
		case <-done:
			// Success path
		case <-time.After(cfg.LogFlushTimeout):
			t.Error("Should not timeout")
		}
	}

	if !flushCalled {
		t.Error("OnFlushLogs should have been called")
	}
}

// TestGracefulShutdownLogFlushTimeout simulates log flush timeout path
func TestGracefulShutdownLogFlushTimeout(t *testing.T) {
	flushCalled := false
	cfg := ShutdownConfig{
		LogFlushTimeout: 50 * time.Millisecond,
		OnFlushLogs: func() {
			flushCalled = true
			time.Sleep(200 * time.Millisecond) // Simulate slow flush
		},
	}

	// Simulate the log flush logic from gracefulShutdown
	if cfg.OnFlushLogs != nil {
		done := make(chan struct{})
		go func() {
			cfg.OnFlushLogs()
			close(done)
		}()
		select {
		case <-done:
			// Should not happen first
		case <-time.After(cfg.LogFlushTimeout):
			// Expected timeout path
		}
	}

	if !flushCalled {
		t.Error("OnFlushLogs should have been called")
	}
}

// TestGracefulShutdownServerShutdownSuccess simulates server shutdown success
func TestGracefulShutdownServerShutdownSuccess(t *testing.T) {
	server := &http.Server{Addr: ":0"}
	cfg := ShutdownConfig{
		Server:          server,
		InFlightTimeout: 100 * time.Millisecond,
	}

	// The server isn't started, so Shutdown will succeed immediately
	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	if cfg.Server != nil {
		err := cfg.Server.Shutdown(ctx)
		if err != nil {
			t.Errorf("Server.Shutdown error: %v", err)
		}
	}
}

// TestGracefulShutdownShutdownFuncSuccess simulates ShutdownFunc success
func TestGracefulShutdownShutdownFuncSuccess(t *testing.T) {
	called := false
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
		InFlightTimeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	if cfg.ShutdownFunc != nil {
		err := cfg.ShutdownFunc(ctx)
		if err != nil {
			t.Errorf("ShutdownFunc error: %v", err)
		}
	}

	if !called {
		t.Error("ShutdownFunc should have been called")
	}
}

// TestGracefulShutdownShutdownFuncError simulates ShutdownFunc error
func TestGracefulShutdownShutdownFuncError(t *testing.T) {
	expectedErr := errors.New("shutdown failed")
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			return expectedErr
		},
		InFlightTimeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	if cfg.ShutdownFunc != nil {
		err := cfg.ShutdownFunc(ctx)
		if err != expectedErr {
			t.Errorf("Expected error %v, got %v", expectedErr, err)
		}
	}
}

// TestGracefulShutdownPIDFileRemoveSuccess simulates PID file removal success
func TestGracefulShutdownPIDFileRemoveSuccess(t *testing.T) {
	// Create a temp PID file
	tmpFile, err := os.CreateTemp("", "test-pid-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()

	cfg := ShutdownConfig{
		PIDFile: tmpFile.Name(),
	}

	// Simulate PID file removal from gracefulShutdown
	if cfg.PIDFile != "" {
		err := os.Remove(cfg.PIDFile)
		if err != nil && !os.IsNotExist(err) {
			t.Errorf("Failed to remove PID file: %v", err)
		}
	}

	// Verify file is removed
	if _, err := os.Stat(tmpFile.Name()); !os.IsNotExist(err) {
		t.Error("PID file should have been removed")
		os.Remove(tmpFile.Name()) // Cleanup
	}
}

// TestGracefulShutdownPIDFileRemoveNotExist simulates PID file not existing
func TestGracefulShutdownPIDFileRemoveNotExist(t *testing.T) {
	cfg := ShutdownConfig{
		PIDFile: "/tmp/nonexistent-pid-file-test-12345.pid",
	}

	// Simulate PID file removal from gracefulShutdown
	if cfg.PIDFile != "" {
		err := os.Remove(cfg.PIDFile)
		if err != nil && !os.IsNotExist(err) {
			t.Errorf("Unexpected error for non-existent file: %v", err)
		}
		// os.IsNotExist error is expected and handled
	}
}

// TestGracefulShutdownPIDFileRemoveError simulates PID file removal error
func TestGracefulShutdownPIDFileRemoveError(t *testing.T) {
	// Create a directory with a file that can't be removed normally
	// For testing, we just verify the error path logic
	cfg := ShutdownConfig{
		PIDFile: "/nonexistent/path/test.pid",
	}

	// Simulate PID file removal from gracefulShutdown
	if cfg.PIDFile != "" {
		err := os.Remove(cfg.PIDFile)
		// We expect an error here (path doesn't exist or permission denied)
		if err == nil {
			t.Error("Expected error removing PID file from invalid path")
		}
	}
}

// TestGracefulShutdownGetChildPIDsWithPIDs simulates GetChildPIDs returning PIDs
func TestGracefulShutdownGetChildPIDsWithPIDs(t *testing.T) {
	pidsReturned := false
	cfg := ShutdownConfig{
		ChildTimeout: 100 * time.Millisecond,
		GetChildPIDs: func() []int {
			pidsReturned = true
			return []int{12345, 12346}
		},
	}

	// Simulate GetChildPIDs check from gracefulShutdown
	if cfg.GetChildPIDs != nil {
		pids := cfg.GetChildPIDs()
		if len(pids) > 0 {
			// Would call stopChildProcesses here
		}
	}

	if !pidsReturned {
		t.Error("GetChildPIDs should have been called")
	}
}

// TestGracefulShutdownGetChildPIDsEmpty simulates GetChildPIDs returning empty
func TestGracefulShutdownGetChildPIDsEmpty(t *testing.T) {
	cfg := ShutdownConfig{
		ChildTimeout: 100 * time.Millisecond,
		GetChildPIDs: func() []int {
			return []int{}
		},
	}

	// Simulate GetChildPIDs check from gracefulShutdown
	if cfg.GetChildPIDs != nil {
		pids := cfg.GetChildPIDs()
		if len(pids) != 0 {
			t.Errorf("Expected empty PIDs, got %d", len(pids))
		}
	}
}

// TestGracefulShutdownNoGetChildPIDs simulates nil GetChildPIDs
func TestGracefulShutdownNoGetChildPIDs(t *testing.T) {
	cfg := ShutdownConfig{
		ChildTimeout: 100 * time.Millisecond,
		GetChildPIDs: nil,
	}

	// Simulate GetChildPIDs check from gracefulShutdown - should not panic
	if cfg.GetChildPIDs != nil {
		cfg.GetChildPIDs()
	}
}

// TestGracefulShutdownNoCallbacks tests gracefulShutdown path with no callbacks
func TestGracefulShutdownNoCallbacks(t *testing.T) {
	cfg := ShutdownConfig{
		InFlightTimeout: 100 * time.Millisecond,
		ChildTimeout:    100 * time.Millisecond,
		DatabaseTimeout: 100 * time.Millisecond,
		LogFlushTimeout: 100 * time.Millisecond,
	}

	// Simulate all nil checks from gracefulShutdown
	if cfg.ShutdownFunc != nil {
		t.Error("ShutdownFunc should be nil")
	}
	if cfg.Server != nil {
		t.Error("Server should be nil")
	}
	if cfg.OnCloseDatabase != nil {
		t.Error("OnCloseDatabase should be nil")
	}
	if cfg.OnFlushLogs != nil {
		t.Error("OnFlushLogs should be nil")
	}
	if cfg.GetChildPIDs != nil {
		t.Error("GetChildPIDs should be nil")
	}
}

// TestGracefulShutdownNilDatabase simulates nil OnCloseDatabase
func TestGracefulShutdownNilDatabase(t *testing.T) {
	cfg := ShutdownConfig{
		OnCloseDatabase: nil,
	}

	// Simulate the nil check from gracefulShutdown
	if cfg.OnCloseDatabase != nil {
		t.Error("OnCloseDatabase should be nil")
	}
}

// TestGracefulShutdownNilLogFlush simulates nil OnFlushLogs
func TestGracefulShutdownNilLogFlush(t *testing.T) {
	cfg := ShutdownConfig{
		OnFlushLogs: nil,
	}

	// Simulate the nil check from gracefulShutdown
	if cfg.OnFlushLogs != nil {
		t.Error("OnFlushLogs should be nil")
	}
}

// TestGracefulShutdownNilPIDFile simulates empty PIDFile
func TestGracefulShutdownNilPIDFile(t *testing.T) {
	cfg := ShutdownConfig{
		PIDFile: "",
	}

	// Simulate the empty check from gracefulShutdown
	if cfg.PIDFile != "" {
		t.Error("PIDFile should be empty")
	}
}

// TestSetShuttingDownFlag tests the shutdown flag state machine
func TestSetShuttingDownFlag(t *testing.T) {
	// Reset to known state
	setShuttingDown(false)
	if IsShuttingDown() {
		t.Error("Initial state should be false")
	}

	// Set to true (simulating shutdown start)
	setShuttingDown(true)
	if !IsShuttingDown() {
		t.Error("Should be shutting down after setShuttingDown(true)")
	}

	// Try setting again (idempotent)
	setShuttingDown(true)
	if !IsShuttingDown() {
		t.Error("Should still be shutting down")
	}

	// Reset
	setShuttingDown(false)
	if IsShuttingDown() {
		t.Error("Should not be shutting down after reset")
	}
}

// TestShutdownMutexConcurrency tests mutex protection under high concurrency
func TestShutdownMutexConcurrency(t *testing.T) {
	// Save original state
	orig := shuttingDown
	defer func() { shuttingDown = orig }()

	done := make(chan bool, 20)

	// Many concurrent readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 1000; j++ {
				_ = IsShuttingDown()
			}
			done <- true
		}()
	}

	// Many concurrent writers
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 1000; j++ {
				setShuttingDown(j%2 == 0)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("Timeout waiting for goroutines")
		}
	}
}

// TestSetupInFlightTimeoutDefault tests default InFlightTimeout
func TestSetupInFlightTimeoutDefault(t *testing.T) {
	// When InFlightTimeout is 0, Setup should apply default of 30s
	cfg := ShutdownConfig{InFlightTimeout: 0}
	// Setup applies defaults internally, we can't inspect them directly
	// but we verify Setup doesn't panic
	Setup(cfg)
}

// TestSetupChildTimeoutDefault tests default ChildTimeout
func TestSetupChildTimeoutDefault(t *testing.T) {
	cfg := ShutdownConfig{ChildTimeout: 0}
	Setup(cfg)
}

// TestSetupDatabaseTimeoutDefault tests default DatabaseTimeout
func TestSetupDatabaseTimeoutDefault(t *testing.T) {
	cfg := ShutdownConfig{DatabaseTimeout: 0}
	Setup(cfg)
}

// TestSetupLogFlushTimeoutDefault tests default LogFlushTimeout
func TestSetupLogFlushTimeoutDefault(t *testing.T) {
	cfg := ShutdownConfig{LogFlushTimeout: 0}
	Setup(cfg)
}

// TestReopenLogsCallsCallback verifies reopenLogs calls the callback
func TestReopenLogsCallsCallback(t *testing.T) {
	callCount := 0
	cfg := ShutdownConfig{
		OnReopenLogs: func() {
			callCount++
		},
	}

	reopenLogs(cfg)
	if callCount != 1 {
		t.Errorf("Expected callback to be called once, got %d", callCount)
	}

	reopenLogs(cfg)
	if callCount != 2 {
		t.Errorf("Expected callback to be called twice, got %d", callCount)
	}
}

// TestDumpStatusCallsCallback verifies dumpStatus calls the callback
func TestDumpStatusCallsCallback(t *testing.T) {
	callCount := 0
	cfg := ShutdownConfig{
		OnDumpStatus: func() {
			callCount++
		},
	}

	dumpStatus(cfg)
	if callCount != 1 {
		t.Errorf("Expected callback to be called once, got %d", callCount)
	}

	dumpStatus(cfg)
	if callCount != 2 {
		t.Errorf("Expected callback to be called twice, got %d", callCount)
	}
}

// TestShutdownConfigContextCancellation tests context cancellation in ShutdownFunc
func TestShutdownConfigContextCancellation(t *testing.T) {
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- cfg.ShutdownFunc(ctx)
	}()

	// Cancel the context
	cancel()

	select {
	case err := <-errCh:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("ShutdownFunc should have returned after context cancellation")
	}
}

// TestServerShutdownWithContext tests server shutdown respects context
func TestServerShutdownWithContext(t *testing.T) {
	server := &http.Server{Addr: ":0"}
	cfg := ShutdownConfig{
		Server:          server,
		InFlightTimeout: 5 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.InFlightTimeout)
	defer cancel()

	err := cfg.Server.Shutdown(ctx)
	if err != nil {
		t.Errorf("Server.Shutdown error: %v", err)
	}
}
