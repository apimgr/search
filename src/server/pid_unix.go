//go:build !windows
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

// isProcessRunning checks if a process with the given PID exists (Unix).
// Uses signal 0 to test process existence without actually sending a signal.
func isProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds — send signal 0 to check existence
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// isOurProcess verifies the process is actually our binary (not a PID reuse).
// On Linux reads /proc/{pid}/exe; on macOS/BSD falls back to ps.
func isOurProcess(pid int) bool {
	// Linux: read /proc/{pid}/exe symlink
	exePath, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		// macOS/BSD: use ps command as fallback
		if runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" ||
			runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" {
			return isOurProcessPS(pid)
		}
		return false
	}
	return strings.Contains(filepath.Base(exePath), "search")
}

// isOurProcessPS checks process identity on macOS/BSD using ps.
func isOurProcessPS(pid int) bool {
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "search")
}
