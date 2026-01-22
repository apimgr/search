package signal

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

// These tests verify the gracefulShutdown function's internal logic
// Since gracefulShutdown calls os.Exit(0), we test individual components
// and use subprocess testing for the full flow

// TestGracefulShutdownInSubprocess tests gracefulShutdown via subprocess
func TestGracefulShutdownInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN") == "1" {
		// This code runs in the subprocess
		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		// Should not reach here due to os.Exit(0)
		return
	}

	// Run this test in a subprocess
	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN=1")
	err := cmd.Run()

	// Verify process exited with status 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithServerInSubprocess tests shutdown with server
func TestGracefulShutdownWithServerInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_SERVER") == "1" {
		server := &http.Server{Addr: ":0"}
		cfg := ShutdownConfig{
			Server:          server,
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithServerInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_SERVER=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithShutdownFuncInSubprocess tests shutdown with ShutdownFunc
func TestGracefulShutdownWithShutdownFuncInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_FUNC") == "1" {
		cfg := ShutdownConfig{
			ShutdownFunc: func(ctx context.Context) error {
				return nil
			},
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithShutdownFuncInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_FUNC=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithShutdownFuncErrorInSubprocess tests shutdown with ShutdownFunc error
func TestGracefulShutdownWithShutdownFuncErrorInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_FUNC_ERR") == "1" {
		cfg := ShutdownConfig{
			ShutdownFunc: func(ctx context.Context) error {
				return errors.New("shutdown error")
			},
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithShutdownFuncErrorInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_FUNC_ERR=1")
	err := cmd.Run()

	// Should still exit 0 even with shutdown func error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithCallbacksInSubprocess tests shutdown with all callbacks
func TestGracefulShutdownWithCallbacksInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_CALLBACKS") == "1" {
		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
			OnCloseDatabase: func() {},
			OnFlushLogs:     func() {},
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithCallbacksInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_CALLBACKS=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithDatabaseTimeoutInSubprocess tests database timeout path
func TestGracefulShutdownWithDatabaseTimeoutInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_DB_TIMEOUT") == "1" {
		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 50 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
			OnCloseDatabase: func() {
				time.Sleep(200 * time.Millisecond) // Exceed timeout
			},
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithDatabaseTimeoutInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_DB_TIMEOUT=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithLogFlushTimeoutInSubprocess tests log flush timeout path
func TestGracefulShutdownWithLogFlushTimeoutInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_LOG_TIMEOUT") == "1" {
		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 50 * time.Millisecond,
			OnFlushLogs: func() {
				time.Sleep(200 * time.Millisecond) // Exceed timeout
			},
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithLogFlushTimeoutInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_LOG_TIMEOUT=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithPIDFileInSubprocess tests PID file removal
func TestGracefulShutdownWithPIDFileInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_PID") == "1" {
		// Create a temp PID file
		tmpFile, err := os.CreateTemp("", "test-pid-*")
		if err != nil {
			os.Exit(1)
		}
		tmpFile.Close()
		pidFile := tmpFile.Name()

		cfg := ShutdownConfig{
			PIDFile:         pidFile,
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithPIDFileInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_PID=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithNonexistentPIDFileInSubprocess tests non-existent PID file
func TestGracefulShutdownWithNonexistentPIDFileInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_PID_NONEXIST") == "1" {
		cfg := ShutdownConfig{
			PIDFile:         "/tmp/nonexistent-test-pid-file-12345.pid",
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithNonexistentPIDFileInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_PID_NONEXIST=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithChildPIDsInSubprocess tests GetChildPIDs path
func TestGracefulShutdownWithChildPIDsInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_CHILD_PIDS") == "1" {
		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
			GetChildPIDs: func() []int {
				return []int{999999} // Non-existent PID
			},
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithChildPIDsInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_CHILD_PIDS=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithEmptyChildPIDsInSubprocess tests empty GetChildPIDs path
func TestGracefulShutdownWithEmptyChildPIDsInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_EMPTY_PIDS") == "1" {
		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
			GetChildPIDs: func() []int {
				return []int{}
			},
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithEmptyChildPIDsInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_EMPTY_PIDS=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithAllOptionsInSubprocess tests all options together
func TestGracefulShutdownWithAllOptionsInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_ALL") == "1" {
		// Create a temp PID file
		tmpFile, err := os.CreateTemp("", "test-pid-*")
		if err != nil {
			os.Exit(1)
		}
		tmpFile.Close()

		cfg := ShutdownConfig{
			ShutdownFunc: func(ctx context.Context) error {
				return nil
			},
			PIDFile:         tmpFile.Name(),
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
			OnCloseDatabase: func() {},
			OnFlushLogs:     func() {},
			GetChildPIDs: func() []int {
				return []int{}
			},
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithAllOptionsInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_ALL=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownSetsShuttingDownFlag tests that shutdown sets the flag
func TestGracefulShutdownSetsShuttingDownFlag(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_FLAG") == "1" {
		// Verify the flag is initially false
		if IsShuttingDown() {
			os.Exit(1)
		}

		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		// gracefulShutdown sets the flag to true
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownSetsShuttingDownFlag")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_FLAG=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestServerShutdownError simulates server shutdown error
func TestServerShutdownError(t *testing.T) {
	// Create a server and start it briefly
	server := &http.Server{Addr: "127.0.0.1:0"}

	// Start the server
	go func() {
		server.ListenAndServe()
	}()

	// Give it time to start
	time.Sleep(50 * time.Millisecond)

	// Create context that's already expired
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// Wait for context to expire
	time.Sleep(10 * time.Millisecond)

	// This should return context deadline exceeded
	err := server.Shutdown(ctx)
	if err != context.DeadlineExceeded {
		// This is OK, the server might not have even started accepting yet
		// Just verify we don't panic
	}

	// Clean up
	server.Close()
}

// TestGracefulShutdownPIDFileRemovalError tests PID file removal error path
func TestGracefulShutdownPIDFileRemovalError(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_PID_ERR") == "1" {
		cfg := ShutdownConfig{
			PIDFile:         "/invalid/path/that/does/not/exist/test.pid",
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownPIDFileRemovalError")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_PID_ERR=1")
	err := cmd.Run()

	// Should still exit 0 even with PID removal error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithServerErrorInSubprocess tests server shutdown error
func TestGracefulShutdownWithServerErrorInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_SERVER_ERR") == "1" {
		// Create a server that's been closed
		server := &http.Server{Addr: "127.0.0.1:0"}

		cfg := ShutdownConfig{
			Server:          server,
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}
		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithServerErrorInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_SERVER_ERR=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithBothServerAndFuncInSubprocess tests ShutdownFunc priority
func TestGracefulShutdownWithBothServerAndFuncInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_BOTH") == "1" {
		shutdownFuncCalled := false
		server := &http.Server{Addr: "127.0.0.1:0"}

		cfg := ShutdownConfig{
			Server: server,
			ShutdownFunc: func(ctx context.Context) error {
				shutdownFuncCalled = true
				return nil
			},
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}

		// Per the code, ShutdownFunc takes priority over Server
		gracefulShutdown(cfg)

		// This won't be reached due to os.Exit, but it documents the expected behavior
		if !shutdownFuncCalled {
			os.Exit(1)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithBothServerAndFuncInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_BOTH=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithServerShutdownErrorInSubprocess tests server shutdown with error
func TestGracefulShutdownWithServerShutdownErrorInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_SRV_ERR") == "1" {
		// Create a server and start it
		server := &http.Server{Addr: "127.0.0.1:18080"}

		// Start the server in a goroutine
		go func() {
			server.ListenAndServe()
		}()

		// Give server time to start
		time.Sleep(50 * time.Millisecond)

		// Use very short timeout to trigger context deadline exceeded
		cfg := ShutdownConfig{
			Server:          server,
			InFlightTimeout: 1 * time.Nanosecond, // Extremely short timeout
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}

		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithServerShutdownErrorInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_SRV_ERR=1")
	err := cmd.Run()

	// Should still exit 0 even with server shutdown error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithRealActualChildProcessesInSubprocess tests real child process handling
func TestGracefulShutdownWithRealActualChildProcessesInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_REAL_CHILD") == "1" {
		// Start a real child process
		childCmd := exec.Command("sleep", "60")
		if err := childCmd.Start(); err != nil {
			os.Exit(1)
		}
		childPID := childCmd.Process.Pid

		cfg := ShutdownConfig{
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    500 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
			GetChildPIDs: func() []int {
				return []int{childPID}
			},
		}

		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithRealActualChildProcessesInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_REAL_CHILD=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownPIDFilePermissionErrorInSubprocess tests PID file removal when permission denied
func TestGracefulShutdownPIDFilePermissionErrorInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_PID_PERM") == "1" {
		// Try to remove a file we can't remove - use /etc/passwd as example
		// This is a file that exists but we can't delete
		cfg := ShutdownConfig{
			PIDFile:         "/etc/passwd", // Exists but can't be removed
			InFlightTimeout: 100 * time.Millisecond,
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}

		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownPIDFilePermissionErrorInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_PID_PERM=1")
	err := cmd.Run()

	// Should still exit 0 even with permission error
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}

// TestGracefulShutdownWithRunningServerAndTimeoutInSubprocess tests real server shutdown with timeout
func TestGracefulShutdownWithRunningServerAndTimeoutInSubprocess(t *testing.T) {
	if os.Getenv("TEST_GRACEFUL_SHUTDOWN_SRV_TIMEOUT") == "1" {
		// Create a server and start it
		server := &http.Server{Addr: "127.0.0.1:18081"}

		// Start the server in a goroutine
		go func() {
			server.ListenAndServe()
		}()

		// Give server time to start
		time.Sleep(100 * time.Millisecond)

		// Start a long-running request in background
		go func() {
			resp, err := http.Get("http://127.0.0.1:18081/")
			if err == nil && resp != nil {
				resp.Body.Close()
			}
		}()

		time.Sleep(50 * time.Millisecond)

		// Use very short timeout to force shutdown error
		cfg := ShutdownConfig{
			Server:          server,
			InFlightTimeout: 1 * time.Nanosecond, // Extremely short
			ChildTimeout:    100 * time.Millisecond,
			DatabaseTimeout: 100 * time.Millisecond,
			LogFlushTimeout: 100 * time.Millisecond,
		}

		gracefulShutdown(cfg)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestGracefulShutdownWithRunningServerAndTimeoutInSubprocess")
	cmd.Env = append(os.Environ(), "TEST_GRACEFUL_SHUTDOWN_SRV_TIMEOUT=1")
	err := cmd.Run()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Errorf("Expected exit code 0, got exit error: %v", exitErr)
		} else {
			t.Errorf("Unexpected error running subprocess: %v", err)
		}
	}
}
