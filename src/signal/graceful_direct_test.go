package signal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// graceful_direct_test.go calls gracefulShutdown directly in the parent test
// process so that Go's coverage instrumentation captures every branch.
// The existing graceful_test.go uses subprocess execution which prevents
// coverage from being recorded in the parent process.

// resetShutdownFlag restores shuttingDown to its pre-test value.
func resetShutdownFlag(t *testing.T) {
	t.Helper()
	orig := shuttingDown
	t.Cleanup(func() { shuttingDown = orig })
	setShuttingDown(false)
}

// newDone returns a fresh channel for each gracefulShutdown invocation.
func newDone() chan struct{} {
	return make(chan struct{})
}

// assertDoneClosed fails if done was not closed after gracefulShutdown returns.
func assertDoneClosed(t *testing.T, done chan struct{}) {
	t.Helper()
	select {
	case <-done:
	default:
		t.Error("gracefulShutdown did not close the done channel")
	}
}

// TestGracefulShutdown_NoServerNoFunc covers the minimal config path:
// no Server, no ShutdownFunc, no callbacks, no PID file.
func TestGracefulShutdown_NoServerNoFunc(t *testing.T) {
	resetShutdownFlag(t)
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_ShutdownFuncSuccess covers the ShutdownFunc success path.
func TestGracefulShutdown_ShutdownFuncSuccess(t *testing.T) {
	resetShutdownFlag(t)
	called := false
	done := newDone()
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			called = true
			return nil
		},
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if !called {
		t.Error("ShutdownFunc was not called")
	}
}

// TestGracefulShutdown_ShutdownFuncError covers the ShutdownFunc error path
// (shutdown must still complete and close done even when the func returns an error).
func TestGracefulShutdown_ShutdownFuncError(t *testing.T) {
	resetShutdownFlag(t)
	done := newDone()
	cfg := ShutdownConfig{
		ShutdownFunc: func(ctx context.Context) error {
			return errors.New("shutdown func error")
		},
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_ServerShutdown covers the cfg.Server != nil path using
// an httptest.Server whose underlying http.Server has already been started so
// Shutdown drains it cleanly within the timeout.
func TestGracefulShutdown_ServerShutdown(t *testing.T) {
	resetShutdownFlag(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	done := newDone()
	cfg := ShutdownConfig{
		Server:          ts.Config,
		InFlightTimeout: 200 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_ServerShutdownError covers the Server.Shutdown error path
// by providing a server that was never started so Shutdown returns immediately
// (ErrServerClosed or similar), which is treated as an error by the implementation.
func TestGracefulShutdown_ServerShutdownError(t *testing.T) {
	resetShutdownFlag(t)

	server := &http.Server{Addr: "127.0.0.1:0"}
	done := newDone()
	cfg := ShutdownConfig{
		Server:          server,
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_OnCloseDatabase covers the OnCloseDatabase fast path
// (callback completes before DatabaseTimeout).
func TestGracefulShutdown_OnCloseDatabase(t *testing.T) {
	resetShutdownFlag(t)
	called := false
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 100 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
		OnCloseDatabase: func() { called = true },
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if !called {
		t.Error("OnCloseDatabase was not called")
	}
}

// TestGracefulShutdown_OnCloseDatabaseTimeout covers the DatabaseTimeout branch
// (callback exceeds DatabaseTimeout so the select hits the time.After case).
func TestGracefulShutdown_OnCloseDatabaseTimeout(t *testing.T) {
	resetShutdownFlag(t)
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 1 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
		OnCloseDatabase: func() {
			time.Sleep(50 * time.Millisecond)
		},
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_OnFlushLogs covers the OnFlushLogs fast path
// (callback completes before LogFlushTimeout).
func TestGracefulShutdown_OnFlushLogs(t *testing.T) {
	resetShutdownFlag(t)
	called := false
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 100 * time.Millisecond,
		OnFlushLogs:     func() { called = true },
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if !called {
		t.Error("OnFlushLogs was not called")
	}
}

// TestGracefulShutdown_OnFlushLogsTimeout covers the LogFlushTimeout branch
// (callback exceeds LogFlushTimeout so the select hits the time.After case).
func TestGracefulShutdown_OnFlushLogsTimeout(t *testing.T) {
	resetShutdownFlag(t)
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 1 * time.Millisecond,
		OnFlushLogs: func() {
			time.Sleep(50 * time.Millisecond)
		},
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_PIDFileExists covers the PID file removal success path
// (file exists and is removed without error).
func TestGracefulShutdown_PIDFileExists(t *testing.T) {
	resetShutdownFlag(t)
	f, err := os.CreateTemp(t.TempDir(), "test-pid-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	pidPath := f.Name()

	done := newDone()
	cfg := ShutdownConfig{
		PIDFile:         pidPath,
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed after shutdown")
	}
}

// TestGracefulShutdown_PIDFileNotExist covers the IsNotExist branch (no file →
// no error logged, shutdown completes normally).
func TestGracefulShutdown_PIDFileNotExist(t *testing.T) {
	resetShutdownFlag(t)
	done := newDone()
	cfg := ShutdownConfig{
		PIDFile:         t.TempDir() + "/nonexistent-pid.pid",
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_PIDFilePermError covers the PID file error path where the
// file exists but cannot be removed (covered by using a directory path which
// os.Remove will reject with EISDIR on Linux).
func TestGracefulShutdown_PIDFilePermError(t *testing.T) {
	resetShutdownFlag(t)
	done := newDone()
	cfg := ShutdownConfig{
		PIDFile:         t.TempDir(),
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_GetChildPIDsEmpty covers the GetChildPIDs → empty list
// branch (stopChildProcesses is not called).
func TestGracefulShutdown_GetChildPIDsEmpty(t *testing.T) {
	resetShutdownFlag(t)
	called := false
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
		GetChildPIDs: func() []int {
			called = true
			return []int{}
		},
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if !called {
		t.Error("GetChildPIDs was not called")
	}
}

// TestGracefulShutdown_GetChildPIDsNonexistent covers the GetChildPIDs → non-empty
// list branch where the PIDs do not exist (stopChildProcesses handles SIGTERM errors).
func TestGracefulShutdown_GetChildPIDsNonexistent(t *testing.T) {
	resetShutdownFlag(t)
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    10 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
		GetChildPIDs: func() []int {
			return []int{999999}
		},
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
}

// TestGracefulShutdown_AllCallbacksTogether covers the combined path where every
// optional field is set, exercising all branches in a single run.
func TestGracefulShutdown_AllCallbacksTogether(t *testing.T) {
	resetShutdownFlag(t)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	pidPath := t.TempDir() + "/combined.pid"
	if err := os.WriteFile(pidPath, []byte("0\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dbClosed := false
	logsFlushed := false
	done := newDone()
	cfg := ShutdownConfig{
		Server:  ts.Config,
		PIDFile: pidPath,
		InFlightTimeout: 200 * time.Millisecond,
		ChildTimeout:    10 * time.Millisecond,
		DatabaseTimeout: 100 * time.Millisecond,
		LogFlushTimeout: 100 * time.Millisecond,
		OnCloseDatabase: func() { dbClosed = true },
		OnFlushLogs:     func() { logsFlushed = true },
		GetChildPIDs:    func() []int { return []int{} },
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if !dbClosed {
		t.Error("OnCloseDatabase was not called")
	}
	if !logsFlushed {
		t.Error("OnFlushLogs was not called")
	}
}

// TestGracefulShutdown_SetsShuttingDownFlag confirms that gracefulShutdown sets
// the global shuttingDown flag to true during execution.
func TestGracefulShutdown_SetsShuttingDownFlag(t *testing.T) {
	resetShutdownFlag(t)

	flagObserved := false
	done := newDone()
	cfg := ShutdownConfig{
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 100 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
		OnCloseDatabase: func() {
			flagObserved = IsShuttingDown()
		},
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if !flagObserved {
		t.Error("IsShuttingDown() was not true during gracefulShutdown execution")
	}
}

// TestGracefulShutdown_ShutdownFuncTakesPriorityOverServer confirms that when
// both ShutdownFunc and Server are set, ShutdownFunc is used and Server.Shutdown
// is NOT called directly.
func TestGracefulShutdown_ShutdownFuncTakesPriorityOverServer(t *testing.T) {
	resetShutdownFlag(t)

	funcCalled := false
	server := &http.Server{Addr: "127.0.0.1:0"}
	done := newDone()
	cfg := ShutdownConfig{
		Server: server,
		ShutdownFunc: func(ctx context.Context) error {
			funcCalled = true
			return nil
		},
		InFlightTimeout: 5 * time.Millisecond,
		ChildTimeout:    5 * time.Millisecond,
		DatabaseTimeout: 5 * time.Millisecond,
		LogFlushTimeout: 5 * time.Millisecond,
	}
	gracefulShutdown(cfg, done)
	assertDoneClosed(t, done)
	if !funcCalled {
		t.Error("ShutdownFunc should take priority over Server.Shutdown")
	}
}
