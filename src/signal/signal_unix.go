//go:build !windows
// +build !windows

// Per AI.md PART 7: Unix signal handling with build tags

package signal

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// findProcessFunc is the function used to find processes
// This is a variable to allow testing with mock implementations
var findProcessFunc = os.FindProcess

// setupSignals configures graceful shutdown (Unix)
// Per AI.md PART 7: Unix signals table
func setupSignals(cfg ShutdownConfig, done chan struct{}) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		// 15 - kill (default), graceful shutdown
		syscall.SIGTERM,
		// 2 - Ctrl+C, graceful shutdown
		syscall.SIGINT,
		// 3 - Ctrl+\, graceful shutdown
		syscall.SIGQUIT,
		// 10 - Reopen logs (log rotation)
		syscall.SIGUSR1,
		// 12 - Status dump to log
		syscall.SIGUSR2,
	)

	// Handle SIGRTMIN+3 (signal 37) - Docker STOPSIGNAL
	signal.Notify(sigChan, syscall.Signal(37))

	// Per AI.md PART 7: SIGHUP should be IGNORED
	// Config auto-reloads via file watcher, not SIGHUP
	signal.Ignore(syscall.SIGHUP)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGUSR1:
				slog.Info("Received SIGUSR1, reopening logs")
				reopenLogs(cfg)

			case syscall.SIGUSR2:
				slog.Info("Received SIGUSR2, dumping status")
				dumpStatus(cfg)

			default:
				// Graceful shutdown: SIGTERM, SIGINT, SIGQUIT, SIGRTMIN+3
				slog.Info("Starting graceful shutdown", "signal", sig)
				gracefulShutdown(cfg, done)
			}
		}
	}()
}

// stopChildProcesses sends SIGTERM to children, SIGKILL after timeout (Unix)
// Per AI.md PART 7: Child process handling with graceful then forced termination
func stopChildProcesses(pids []int, timeout time.Duration) {
	// Send SIGTERM to all children (graceful)
	for _, pid := range pids {
		process, err := findProcessFunc(pid)
		if err != nil {
			slog.Error("Failed to find process", "pid", pid, "err", err)
			continue
		}
		if err := process.Signal(syscall.SIGTERM); err != nil {
			slog.Error("Failed to send SIGTERM", "pid", pid, "err", err)
		} else {
			slog.Info("Sent SIGTERM", "pid", pid)
		}
	}

	// Wait with timeout, then SIGKILL survivors
	deadline := time.Now().Add(timeout)
	for _, pid := range pids {
		process, err := findProcessFunc(pid)
		if err != nil {
			continue
		}

		// Poll until process exits or timeout
		for time.Now().Before(deadline) {
			// Signal 0 checks if process exists without sending anything
			if err := process.Signal(syscall.Signal(0)); err != nil {
				slog.Info("Process has exited", "pid", pid)
				// Process exited
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Force kill if still running (after timeout)
		if err := process.Signal(syscall.Signal(0)); err == nil {
			slog.Warn("Process still running after timeout, sending SIGKILL", "pid", pid)
			process.Signal(syscall.SIGKILL)
		}
	}
}

// KillProcess sends signal to process (Unix)
// Per AI.md PART 7: killProcess helper
func KillProcess(pid int, graceful bool) error {
	process, err := findProcessFunc(pid)
	if err != nil {
		return err
	}
	if graceful {
		return process.Signal(syscall.SIGTERM)
	}
	return process.Signal(syscall.SIGKILL)
}
