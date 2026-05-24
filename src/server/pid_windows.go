//go:build windows
// +build windows

package server

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// isProcessRunning checks if a process with the given PID exists (Windows).
// Uses tasklist to avoid requiring Windows API CGO bindings.
func isProcessRunning(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), strconv.Itoa(pid))
}

// isOurProcess verifies the process is actually our binary (not a PID reuse) on Windows.
// Uses wmic to get the process image name.
func isOurProcess(pid int) bool {
	cmd := exec.Command("wmic", "process", "where",
		fmt.Sprintf("processid=%d", pid), "get", "name", "/value")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(output)), "search")
}
