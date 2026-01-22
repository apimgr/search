//go:build !windows
// +build !windows

package signal

import (
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestKillProcessGraceful tests sending SIGTERM (graceful shutdown)
func TestKillProcessGraceful(t *testing.T) {
	// Start a child process that sleeps
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Send graceful signal (SIGTERM)
	err := KillProcess(pid, true)
	if err != nil {
		t.Errorf("KillProcess(graceful=true) failed: %v", err)
	}

	// Wait for process to terminate
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated as expected
	case <-time.After(2 * time.Second):
		t.Error("Process did not terminate after SIGTERM")
	}
}

// TestKillProcessForced tests sending SIGKILL (forced termination)
func TestKillProcessForced(t *testing.T) {
	// Start a child process that sleeps
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Send forced signal (SIGKILL)
	err := KillProcess(pid, false)
	if err != nil {
		t.Errorf("KillProcess(graceful=false) failed: %v", err)
	}

	// Wait for process to terminate
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-done:
		// Process terminated as expected
	case <-time.After(2 * time.Second):
		t.Error("Process did not terminate after SIGKILL")
	}
}

// TestKillProcessInvalidPID tests sending signal to non-existent process
func TestKillProcessInvalidPID(t *testing.T) {
	// Use a PID that's unlikely to exist
	invalidPID := 999999

	// Check if this PID exists first
	proc, err := os.FindProcess(invalidPID)
	if err == nil {
		// On Unix, FindProcess always succeeds, so check with Signal(0)
		if proc.Signal(syscall.Signal(0)) == nil {
			t.Skip("PID 999999 exists, skipping test")
		}
	}

	err = KillProcess(invalidPID, true)
	if err == nil {
		t.Error("KillProcess should fail for non-existent process")
	}
}

// TestKillProcessNegativePID tests handling of negative PIDs
func TestKillProcessNegativePID(t *testing.T) {
	err := KillProcess(-1, true)
	if err == nil {
		t.Error("KillProcess should fail for negative PID")
	}
}

// TestKillProcessZeroPID tests handling of zero PID
func TestKillProcessZeroPID(t *testing.T) {
	// PID 0 refers to the process group, this should fail or behave differently
	err := KillProcess(0, true)
	// This may or may not error depending on permissions
	// Just ensure it doesn't panic
	_ = err
}

// TestStopChildProcessesEmpty tests stopping with empty PID list
func TestStopChildProcessesEmpty(t *testing.T) {
	// Should not panic with empty list
	stopChildProcesses([]int{}, 100*time.Millisecond)
}

// TestStopChildProcessesSingleProcess tests stopping a single child process
func TestStopChildProcessesSingleProcess(t *testing.T) {
	// Start a child process that sleeps
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	pid := cmd.Process.Pid

	// Stop the child process with a timeout
	stopChildProcesses([]int{pid}, 2*time.Second)

	// Wait for process to be reaped
	cmd.Wait()
}

// TestStopChildProcessesMultiple tests stopping multiple child processes
func TestStopChildProcessesMultiple(t *testing.T) {
	// Start multiple child processes
	cmds := make([]*exec.Cmd, 3)
	pids := make([]int, 3)

	for i := 0; i < 3; i++ {
		cmds[i] = exec.Command("sleep", "60")
		if err := cmds[i].Start(); err != nil {
			t.Fatalf("Failed to start child process %d: %v", i, err)
		}
		pids[i] = cmds[i].Process.Pid
	}

	defer func() {
		for _, cmd := range cmds {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Stop all child processes
	stopChildProcesses(pids, 2*time.Second)

	// Wait for processes to be reaped
	for _, cmd := range cmds {
		cmd.Wait()
	}
}

// TestStopChildProcessesNonExistent tests stopping non-existent PIDs
func TestStopChildProcessesNonExistent(t *testing.T) {
	// Should not panic with non-existent PIDs
	stopChildProcesses([]int{999999, 999998, 999997}, 100*time.Millisecond)
}

// TestStopChildProcessesTimeout tests process termination with timeout
func TestStopChildProcessesTimeout(t *testing.T) {
	// Start a process that ignores SIGTERM (trap handler)
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Give the shell time to set up the trap
	time.Sleep(200 * time.Millisecond)

	// Stop with very short timeout - should escalate to SIGKILL
	stopChildProcesses([]int{pid}, 300*time.Millisecond)

	// Wait for SIGKILL to take effect and be processed
	time.Sleep(500 * time.Millisecond)

	// Wait for the process to actually be reaped
	cmd.Wait()
}

// TestStopChildProcessesMixed tests mix of valid and invalid PIDs
func TestStopChildProcessesMixed(t *testing.T) {
	// Start one valid process
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	validPID := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Mix of valid and invalid PIDs
	pids := []int{999999, validPID, 999998}

	// Should not panic and should terminate valid process
	stopChildProcesses(pids, 2*time.Second)

	// Wait for process to be reaped
	cmd.Wait()
}

// TestStopChildProcessesAlreadyExited tests stopping already-exited processes
func TestStopChildProcessesAlreadyExited(t *testing.T) {
	// Start a process that exits immediately
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	cmd.Wait() // Wait for it to exit

	// Should not panic for already-exited process
	stopChildProcesses([]int{pid}, 100*time.Millisecond)
}

// TestSetupSignalsDoesNotPanic tests that setupSignals doesn't panic
func TestSetupSignalsDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("setupSignals panicked: %v", r)
		}
	}()

	cfg := ShutdownConfig{
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
	}

	setupSignals(cfg)
}

// TestSetupSignalsWithCallbacks tests setupSignals with all callbacks
func TestSetupSignalsWithCallbacks(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("setupSignals panicked: %v", r)
		}
	}()

	cfg := ShutdownConfig{
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
		OnReopenLogs:    func() {},
		OnDumpStatus:    func() {},
		OnCloseDatabase: func() {},
		OnFlushLogs:     func() {},
		GetChildPIDs:    func() []int { return nil },
	}

	setupSignals(cfg)
}

// TestKillProcessSelfProcess tests trying to kill self (current process)
func TestKillProcessSelfProcess(t *testing.T) {
	// Don't actually kill ourselves - just verify FindProcess works
	pid := os.Getpid()
	proc, err := os.FindProcess(pid)
	if err != nil {
		t.Errorf("Failed to find current process: %v", err)
	}
	if proc == nil {
		t.Error("Process should not be nil")
	}
}

// TestKillProcessTableDriven runs table-driven tests for KillProcess
func TestKillProcessTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		pid       int
		graceful  bool
		wantError bool
	}{
		{
			name:      "negative pid graceful",
			pid:       -1,
			graceful:  true,
			wantError: true,
		},
		{
			name:      "negative pid forced",
			pid:       -1,
			graceful:  false,
			wantError: true,
		},
		{
			name:      "large non-existent pid graceful",
			pid:       9999999,
			graceful:  true,
			wantError: true,
		},
		{
			name:      "large non-existent pid forced",
			pid:       9999999,
			graceful:  false,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := KillProcess(tt.pid, tt.graceful)
			if tt.wantError && err == nil {
				t.Errorf("KillProcess(%d, %v) expected error, got nil", tt.pid, tt.graceful)
			}
		})
	}
}

// TestStopChildProcessesWithZeroTimeout tests zero timeout behavior
func TestStopChildProcessesWithZeroTimeout(t *testing.T) {
	// Start a child process
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	pid := cmd.Process.Pid

	// Zero timeout means it should immediately escalate to SIGKILL
	stopChildProcesses([]int{pid}, 0)

	// Wait for process to be reaped
	time.Sleep(500 * time.Millisecond)
	cmd.Wait()
}

// TestStopChildProcessesWithVeryShortTimeout tests very short timeout
func TestStopChildProcessesWithVeryShortTimeout(t *testing.T) {
	// Start a process
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Very short timeout
	stopChildProcesses([]int{pid}, 1*time.Millisecond)

	// Wait for signals to be processed
	time.Sleep(500 * time.Millisecond)
}

// TestFindProcessAlwaysSucceedsOnUnix verifies Unix behavior of FindProcess
func TestFindProcessAlwaysSucceedsOnUnix(t *testing.T) {
	// On Unix, os.FindProcess always succeeds even for non-existent PIDs
	proc, err := os.FindProcess(999999)
	if err != nil {
		t.Errorf("FindProcess should always succeed on Unix, got error: %v", err)
	}
	if proc == nil {
		t.Error("Process should not be nil")
	}
}

// TestStopChildProcessesPollingLoop tests the polling loop behavior
func TestStopChildProcessesPollingLoop(t *testing.T) {
	// Start a process that responds to SIGTERM
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	pid := cmd.Process.Pid

	// Use a long timeout to test the polling loop completes before timeout
	stopChildProcesses([]int{pid}, 5*time.Second)

	// Wait for process to be reaped
	cmd.Wait()
}

// TestStopChildProcessesSIGTERMError tests SIGTERM send failure
func TestStopChildProcessesSIGTERMError(t *testing.T) {
	// Use PIDs that don't exist - SIGTERM send should fail
	stopChildProcesses([]int{-1}, 100*time.Millisecond)
	// Should not panic
}

// TestStopChildProcessesSIGKILLPath tests the SIGKILL escalation path
func TestStopChildProcessesSIGKILLPath(t *testing.T) {
	// Start a process that ignores SIGTERM
	cmd := exec.Command("sh", "-c", "trap '' TERM; sleep 60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()
	pid := cmd.Process.Pid

	// Give time for trap to be set
	time.Sleep(200 * time.Millisecond)

	// Use very short timeout to trigger SIGKILL path
	stopChildProcesses([]int{pid}, 200*time.Millisecond)

	// Wait for SIGKILL to take effect and process to be reaped
	time.Sleep(500 * time.Millisecond)
	cmd.Wait()
}

// TestKillProcessGracefulTableDriven uses table-driven tests for graceful mode
func TestKillProcessGracefulTableDriven(t *testing.T) {
	tests := []struct {
		name     string
		graceful bool
	}{
		{"graceful true", true},
		{"graceful false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command("sleep", "60")
			if err := cmd.Start(); err != nil {
				t.Fatalf("Failed to start child process: %v", err)
			}
			pid := cmd.Process.Pid
			defer func() {
				cmd.Process.Kill()
				cmd.Wait()
			}()

			err := KillProcess(pid, tt.graceful)
			if err != nil {
				t.Errorf("KillProcess(pid, %v) failed: %v", tt.graceful, err)
			}

			// Wait for process to terminate
			done := make(chan error, 1)
			go func() {
				done <- cmd.Wait()
			}()

			select {
			case <-done:
				// Process terminated
			case <-time.After(2 * time.Second):
				t.Errorf("Process did not terminate with graceful=%v", tt.graceful)
			}
		})
	}
}

// TestStopChildProcessesFindProcessLoop tests the second FindProcess call in the loop
func TestStopChildProcessesFindProcessLoop(t *testing.T) {
	// Start multiple processes
	cmds := make([]*exec.Cmd, 2)
	pids := make([]int, 2)

	for i := 0; i < 2; i++ {
		cmds[i] = exec.Command("sleep", "60")
		if err := cmds[i].Start(); err != nil {
			t.Fatalf("Failed to start child process %d: %v", i, err)
		}
		pids[i] = cmds[i].Process.Pid
	}

	defer func() {
		for _, cmd := range cmds {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()

	// Stop processes with timeout
	stopChildProcesses(pids, 2*time.Second)

	// Wait for processes to be reaped
	for _, cmd := range cmds {
		cmd.Wait()
	}
}

// TestSetupSignalsNoPanic ensures setupSignals handles various configs without panic
func TestSetupSignalsNoPanic(t *testing.T) {
	testCases := []struct {
		name string
		cfg  ShutdownConfig
	}{
		{
			name: "empty config",
			cfg:  ShutdownConfig{},
		},
		{
			name: "with all timeouts",
			cfg: ShutdownConfig{
				InFlightTimeout: 30 * time.Second,
				ChildTimeout:    10 * time.Second,
				DatabaseTimeout: 5 * time.Second,
				LogFlushTimeout: 2 * time.Second,
			},
		},
		{
			name: "with callbacks",
			cfg: ShutdownConfig{
				OnReopenLogs:    func() {},
				OnDumpStatus:    func() {},
				OnCloseDatabase: func() {},
				OnFlushLogs:     func() {},
			},
		},
		{
			name: "with GetChildPIDs",
			cfg: ShutdownConfig{
				GetChildPIDs: func() []int { return nil },
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("setupSignals panicked: %v", r)
				}
			}()
			setupSignals(tc.cfg)
		})
	}
}

// TestKillProcessWithRealProcess tests KillProcess with actual process management
func TestKillProcessWithRealProcess(t *testing.T) {
	// Start a process
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}
	pid := cmd.Process.Pid

	// Kill it gracefully
	err := KillProcess(pid, true)
	if err != nil {
		t.Errorf("KillProcess failed: %v", err)
	}

	// Wait for it
	cmd.Wait()

	// Try to kill again (should fail since process is dead)
	err = KillProcess(pid, true)
	// This may succeed or fail depending on OS behavior
	// Just ensure no panic
}

// TestStopChildProcessesWithProcessExitDuringPolling tests process exit during polling
func TestStopChildProcessesWithProcessExitDuringPolling(t *testing.T) {
	// Start a process that will exit on its own
	cmd := exec.Command("sh", "-c", "sleep 0.2")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start child process: %v", err)
	}
	pid := cmd.Process.Pid
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Stop with longer timeout - process should exit during polling
	stopChildProcesses([]int{pid}, 5*time.Second)
}

// TestStopChildProcessesFindProcessError tests error handling in FindProcess
func TestStopChildProcessesFindProcessError(t *testing.T) {
	// On Unix, FindProcess always succeeds, but we can test with edge cases
	// Start a process and immediately kill it
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}
	pid := cmd.Process.Pid
	cmd.Wait() // Wait for process to exit

	// Now try to stop it - the process has already exited
	stopChildProcesses([]int{pid}, 100*time.Millisecond)
	// Should not panic
}

// TestSIGKILLSendFailure tests behavior when SIGKILL send would fail
func TestSIGKILLSendFailure(t *testing.T) {
	// Use an invalid PID that will cause signal send to fail
	// This tests the error path in the SIGKILL section
	// The code doesn't check for SIGKILL errors, but this tests coverage
	stopChildProcesses([]int{-1}, 0)
}

// TestSetupSignalsSIGUSR1 tests SIGUSR1 handling by sending actual signal
func TestSetupSignalsSIGUSR1(t *testing.T) {
	// Create a flag to verify callback was called
	callbackCalled := make(chan bool, 1)

	cfg := ShutdownConfig{
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
		OnReopenLogs: func() {
			callbackCalled <- true
		},
	}

	// Set up signal handlers
	setupSignals(cfg)

	// Give signal handler time to be set up
	time.Sleep(50 * time.Millisecond)

	// Send SIGUSR1 to self
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find self process: %v", err)
	}
	if err := proc.Signal(syscall.SIGUSR1); err != nil {
		t.Fatalf("Failed to send SIGUSR1: %v", err)
	}

	// Wait for callback to be called
	select {
	case <-callbackCalled:
		// Success - callback was called
	case <-time.After(2 * time.Second):
		t.Error("OnReopenLogs callback was not called after SIGUSR1")
	}
}

// TestSetupSignalsSIGUSR2 tests SIGUSR2 handling by sending actual signal
func TestSetupSignalsSIGUSR2(t *testing.T) {
	// Create a flag to verify callback was called
	callbackCalled := make(chan bool, 1)

	cfg := ShutdownConfig{
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
		OnDumpStatus: func() {
			callbackCalled <- true
		},
	}

	// Set up signal handlers
	setupSignals(cfg)

	// Give signal handler time to be set up
	time.Sleep(50 * time.Millisecond)

	// Send SIGUSR2 to self
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find self process: %v", err)
	}
	if err := proc.Signal(syscall.SIGUSR2); err != nil {
		t.Fatalf("Failed to send SIGUSR2: %v", err)
	}

	// Wait for callback to be called
	select {
	case <-callbackCalled:
		// Success - callback was called
	case <-time.After(2 * time.Second):
		t.Error("OnDumpStatus callback was not called after SIGUSR2")
	}
}

// TestSetupSignalsSIGUSR1NoCallback tests SIGUSR1 with nil callback
func TestSetupSignalsSIGUSR1NoCallback(t *testing.T) {
	cfg := ShutdownConfig{
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
		OnReopenLogs:    nil, // No callback configured
	}

	// Set up signal handlers
	setupSignals(cfg)

	// Give signal handler time to be set up
	time.Sleep(50 * time.Millisecond)

	// Send SIGUSR1 to self - should not panic even without callback
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find self process: %v", err)
	}
	if err := proc.Signal(syscall.SIGUSR1); err != nil {
		t.Fatalf("Failed to send SIGUSR1: %v", err)
	}

	// Give time for signal to be processed
	time.Sleep(100 * time.Millisecond)
}

// TestSetupSignalsSIGUSR2NoCallback tests SIGUSR2 with nil callback
func TestSetupSignalsSIGUSR2NoCallback(t *testing.T) {
	cfg := ShutdownConfig{
		InFlightTimeout: 30 * time.Second,
		ChildTimeout:    10 * time.Second,
		DatabaseTimeout: 5 * time.Second,
		LogFlushTimeout: 2 * time.Second,
		OnDumpStatus:    nil, // No callback configured
	}

	// Set up signal handlers
	setupSignals(cfg)

	// Give signal handler time to be set up
	time.Sleep(50 * time.Millisecond)

	// Send SIGUSR2 to self - should not panic even without callback
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find self process: %v", err)
	}
	if err := proc.Signal(syscall.SIGUSR2); err != nil {
		t.Fatalf("Failed to send SIGUSR2: %v", err)
	}

	// Give time for signal to be processed
	time.Sleep(100 * time.Millisecond)
}

// TestSetupSignalsMultipleSIGUSR1 tests multiple SIGUSR1 signals
func TestSetupSignalsMultipleSIGUSR1(t *testing.T) {
	callCount := 0
	mu := &sync.Mutex{}

	cfg := ShutdownConfig{
		OnReopenLogs: func() {
			mu.Lock()
			callCount++
			mu.Unlock()
		},
	}

	setupSignals(cfg)
	time.Sleep(50 * time.Millisecond)

	proc, _ := os.FindProcess(os.Getpid())

	// Send multiple signals
	for i := 0; i < 3; i++ {
		proc.Signal(syscall.SIGUSR1)
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if callCount < 1 {
		t.Errorf("Expected at least 1 callback call, got %d", callCount)
	}
	mu.Unlock()
}

// TestSetupSignalsMultipleSIGUSR2 tests multiple SIGUSR2 signals
func TestSetupSignalsMultipleSIGUSR2(t *testing.T) {
	callCount := 0
	mu := &sync.Mutex{}

	cfg := ShutdownConfig{
		OnDumpStatus: func() {
			mu.Lock()
			callCount++
			mu.Unlock()
		},
	}

	setupSignals(cfg)
	time.Sleep(50 * time.Millisecond)

	proc, _ := os.FindProcess(os.Getpid())

	// Send multiple signals
	for i := 0; i < 3; i++ {
		proc.Signal(syscall.SIGUSR2)
		time.Sleep(50 * time.Millisecond)
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if callCount < 1 {
		t.Errorf("Expected at least 1 callback call, got %d", callCount)
	}
	mu.Unlock()
}
