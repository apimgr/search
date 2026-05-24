package server

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CheckPIDFile checks if PID file exists and if the process is still running.
// Returns (isRunning bool, pid int, err error).
// Per AI.md PART 8: Stale PID detection is REQUIRED on every startup.
func CheckPIDFile(pidPath string) (bool, int, error) {
	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		// No PID file — not running
		return false, 0, nil
	}
	if err != nil {
		return false, 0, fmt.Errorf("reading pid file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		// Corrupt PID file — remove it and continue
		os.Remove(pidPath)
		return false, 0, nil
	}

	// Check if process is running
	if !isProcessRunning(pid) {
		// Stale PID file — remove it and continue
		os.Remove(pidPath)
		return false, 0, nil
	}

	// Process exists — verify it's actually our binary (guards against PID reuse)
	if !isOurProcess(pid) {
		// PID reused by another process — remove stale file and continue
		os.Remove(pidPath)
		return false, 0, nil
	}

	return true, pid, nil
}

// WritePIDFile writes the current process PID to the given file.
// Checks for an existing running instance first and returns an error if found.
// Per AI.md PART 8: Step 12 in server startup sequence.
func WritePIDFile(pidPath string) error {
	// Check for existing running instance first
	running, existingPID, err := CheckPIDFile(pidPath)
	if err != nil {
		return err
	}
	if running {
		return fmt.Errorf("already running (pid %d)", existingPID)
	}

	// Write our PID to the file
	pid := os.Getpid()
	return os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644)
}

// RemovePIDFile removes the PID file on shutdown.
// Per AI.md PART 8: Must be called in signal handlers and defer.
func RemovePIDFile(pidPath string) error {
	return os.Remove(pidPath)
}
