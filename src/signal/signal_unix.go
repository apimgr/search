//go:build !windows
// +build !windows

// Per AI.md PART 7: Unix signal handling with build tags

package signal

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// setupSignals configures graceful shutdown (Unix)
// Per AI.md PART 7: Unix signals table
func setupSignals(cfg ShutdownConfig) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan,
		syscall.SIGTERM,  // 15 - kill (default), graceful shutdown
		syscall.SIGINT,   // 2 - Ctrl+C, graceful shutdown
		syscall.SIGQUIT,  // 3 - Ctrl+\, graceful shutdown
		syscall.SIGUSR1,  // 10 - Reopen logs (log rotation)
		syscall.SIGUSR2,  // 12 - Status dump to log
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
				log.Println("Received SIGUSR1, reopening logs...")
				reopenLogs(cfg)

			case syscall.SIGUSR2:
				log.Println("Received SIGUSR2, dumping status...")
				dumpStatus(cfg)

			default:
				// Graceful shutdown: SIGTERM, SIGINT, SIGQUIT, SIGRTMIN+3
				log.Printf("Received %v, starting graceful shutdown...", sig)
				gracefulShutdown(cfg)
			}
		}
	}()
}

// stopChildProcesses sends SIGTERM to children, SIGKILL after timeout (Unix)
// Per AI.md PART 7: Child process handling with graceful then forced termination
func stopChildProcesses(pids []int, timeout time.Duration) {
	// Send SIGTERM to all children (graceful)
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			log.Printf("Failed to find process %d: %v", pid, err)
			continue
		}
		if err := process.Signal(syscall.SIGTERM); err != nil {
			log.Printf("Failed to send SIGTERM to process %d: %v", pid, err)
		} else {
			log.Printf("Sent SIGTERM to process %d", pid)
		}
	}

	// Wait with timeout, then SIGKILL survivors
	deadline := time.Now().Add(timeout)
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			continue
		}

		// Poll until process exits or timeout
		for time.Now().Before(deadline) {
			// Signal 0 checks if process exists without sending anything
			if err := process.Signal(syscall.Signal(0)); err != nil {
				log.Printf("Process %d has exited", pid)
				break // Process exited
			}
			time.Sleep(100 * time.Millisecond)
		}

		// Force kill if still running (after timeout)
		if err := process.Signal(syscall.Signal(0)); err == nil {
			log.Printf("Process %d still running after timeout, sending SIGKILL", pid)
			process.Signal(syscall.SIGKILL)
		}
	}
}

// KillProcess sends signal to process (Unix)
// Per AI.md PART 7: killProcess helper
func KillProcess(pid int, graceful bool) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if graceful {
		return process.Signal(syscall.SIGTERM)
	}
	return process.Signal(syscall.SIGKILL)
}
