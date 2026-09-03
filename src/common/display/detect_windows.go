//go:build windows
// +build windows

package display

import (
	"os"
	"strings"
)

// detectPlatformDisplay detects display availability on Windows
func (e *Env) detectPlatformDisplay() {
	// Check if running as a Windows Service
	// Services typically have no interactive session
	if isWindowsService() {
		e.HasDisplay = false
		e.DisplayType = DisplayTypeNone
		return
	}

	// Check if running over SSH (OpenSSH for Windows)
	if e.IsSSH || e.IsMosh {
		e.HasDisplay = false
		e.DisplayType = DisplayTypeNone
		return
	}

	// Windows always has a display in interactive sessions
	e.HasDisplay = true
	e.DisplayType = DisplayTypeWindows
}

// isWindowsService attempts to detect if running as a Windows Service
func isWindowsService() bool {
	// Check for common service indicators
	// Services typically don't have SESSIONNAME or it's "Services"
	sessionName := os.Getenv("SESSIONNAME")
	if sessionName == "" || strings.EqualFold(sessionName, "Services") {
		// Could be a service, check for interactive
		// If no USERDOMAIN_ROAMINGPROFILE, likely a service
		if os.Getenv("USERDOMAIN_ROAMINGPROFILE") == "" {
			return true
		}
	}

	// Check parent process - services are typically spawned by services.exe
	// This is a heuristic check
	if os.Getppid() == 1 {
		return true
	}

	return false
}
