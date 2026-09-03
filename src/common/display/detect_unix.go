//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly
// +build linux darwin freebsd openbsd netbsd dragonfly

package display

import (
	"os"
	"runtime"
)

// Package-level function variables for testing
var (
	getPpidFunc = os.Getppid
	getGOOSFunc = func() string { return runtime.GOOS }
)

// detectPlatformDisplay detects display availability on Unix-like systems
func (e *Env) detectPlatformDisplay() {
	switch getGOOSFunc() {
	case "darwin":
		e.detectMacOSDisplay()
	default:
		e.detectUnixDisplay()
	}
}

// detectMacOSDisplay detects display on macOS
func (e *Env) detectMacOSDisplay() {
	// macOS always has a display unless running headless/SSH
	if e.IsSSH || e.IsMosh {
		e.HasDisplay = false
		e.DisplayType = DisplayTypeNone
		return
	}

	// Check if running as a LaunchDaemon (system service)
	if os.Getenv("XPC_SERVICE_NAME") != "" && getPpidFunc() == 1 {
		e.HasDisplay = false
		e.DisplayType = DisplayTypeNone
		return
	}

	// macOS has native Cocoa display
	e.HasDisplay = true
	e.DisplayType = DisplayTypeMacOS
}

// detectUnixDisplay detects display on Linux/BSD systems
func (e *Env) detectUnixDisplay() {
	// Check for Wayland first (preferred over X11)
	if waylandDisplay := os.Getenv("WAYLAND_DISPLAY"); waylandDisplay != "" {
		e.HasDisplay = true
		e.DisplayType = DisplayTypeWayland
		return
	}

	// Check for X11
	if xDisplay := os.Getenv("DISPLAY"); xDisplay != "" {
		e.HasDisplay = true
		e.DisplayType = DisplayTypeX11
		return
	}

	// No display available
	e.HasDisplay = false
	e.DisplayType = DisplayTypeNone
}
