// +build !windows

package server

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
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

// isProcessRunning checks if a process with given PID exists (Unix)
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds - need to send signal 0
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// isOurProcess verifies the process is actually our binary (Unix)
func isOurProcess(pid int) bool {
	// Read /proc/{pid}/exe symlink (Linux)
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		// On macOS/BSD, use ps command
		if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" {
			return isOurProcessDarwin(pid)
		}
		return false
	}
	return strings.Contains(filepath.Base(exePath), "search")
}

// isOurProcessDarwin checks process on macOS/BSD
func isOurProcessDarwin(pid int) bool {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "search")
}
