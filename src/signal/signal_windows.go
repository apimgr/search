//go:build windows
// +build windows

// Per AI.md PART 7: Windows signal handling with build tags
// Windows does NOT support SIGHUP, SIGUSR1, SIGUSR2, SIGQUIT

package signal

import (
	"log"
	"os"
	"os/signal"
	"time"
)

// setupSignals configures graceful shutdown (Windows)
// Per AI.md PART 7: Windows only supports os.Interrupt (Ctrl+C, Ctrl+Break)
func setupSignals(cfg ShutdownConfig) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		for sig := range sigChan {
			log.Printf("Received %v, starting graceful shutdown...", sig)
			gracefulShutdown(cfg)
		}
	}()
}

// stopChildProcesses terminates children (Windows)
// Per AI.md PART 7: Windows cannot send graceful signals - immediate termination only
func stopChildProcesses(pids []int, timeout time.Duration) {
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			log.Printf("Failed to find process %d: %v", pid, err)
			continue
		}
		// Windows: Kill() calls TerminateProcess - no graceful option
		if err := process.Kill(); err != nil {
			log.Printf("Failed to terminate process %d: %v", pid, err)
		} else {
			log.Printf("Terminated process %d", pid)
		}
	}
}

// KillProcess terminates process (Windows)
// Per AI.md PART 7: Windows doesn't have graceful signals - uses TerminateProcess
func KillProcess(pid int, graceful bool) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// Windows: Kill() is immediate termination (TerminateProcess)
	// graceful parameter ignored on Windows
	return process.Kill()
}

// NOTE: For Windows Services, use golang.org/x/sys/windows/svc
// to handle SERVICE_CONTROL_STOP properly
