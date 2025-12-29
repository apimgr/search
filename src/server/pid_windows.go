// +build windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// CheckPIDFile checks if PID file exists and if the process is still running
// Returns: (isRunning bool, pid int, err error)
// Per AI.md PART 8: Stale PID detection is REQUIRED
func CheckPIDFile(pidPath string) (bool, int, error) {
	data, err := os.ReadFile(pidPath)
	if os.IsNotExist(err) {
		return false, 0, nil // No PID file, not running
	}
	if err != nil {
		return false, 0, fmt.Errorf("reading pid file: %w", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		// Corrupt PID file - remove it
		os.Remove(pidPath)
		return false, 0, nil
	}

	// Check if process is running
	if !isProcessRunning(pid) {
		// Stale PID file - remove it
		os.Remove(pidPath)
		return false, 0, nil
	}

	// Process exists - verify it's actually our process (not PID reuse)
	if !isOurProcess(pid) {
		// PID was reused by another process - remove stale file
		os.Remove(pidPath)
		return false, 0, nil
	}

	return true, pid, nil
}

// isProcessRunning checks if a process with given PID exists (Windows)
func isProcessRunning(pid int) bool {
	// Use tasklist to check if process exists
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// If process exists, output will contain the PID
	return strings.Contains(string(output), strconv.Itoa(pid))
}

// isOurProcess verifies the process is actually our binary (Windows)
func isOurProcess(pid int) bool {
	// Use wmic to get process name
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("processid=%d", pid), "get", "name", "/value")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(output)), "search")
}
