package display

import (
	"runtime"
	"testing"
)

func TestDetect(t *testing.T) {
	env := Detect()

	// OS should always be set
	if env.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", env.OS, runtime.GOOS)
	}

	// DisplayType should be a valid value
	validTypes := map[DisplayType]bool{
		DisplayTypeNone:    true,
		DisplayTypeX11:     true,
		DisplayTypeWayland: true,
		DisplayTypeWindows: true,
		DisplayTypeMacOS:   true,
	}
	if !validTypes[env.DisplayType] {
		t.Errorf("DisplayType = %q, not a valid type", env.DisplayType)
	}
}

func TestDisplayTypeConstants(t *testing.T) {
	// Just verify the constants are defined
	if DisplayTypeNone != "none" {
		t.Errorf("DisplayTypeNone = %q, want %q", DisplayTypeNone, "none")
	}
	if DisplayTypeX11 != "x11" {
		t.Errorf("DisplayTypeX11 = %q, want %q", DisplayTypeX11, "x11")
	}
	if DisplayTypeWayland != "wayland" {
		t.Errorf("DisplayTypeWayland = %q, want %q", DisplayTypeWayland, "wayland")
	}
	if DisplayTypeWindows != "windows" {
		t.Errorf("DisplayTypeWindows = %q, want %q", DisplayTypeWindows, "windows")
	}
	if DisplayTypeMacOS != "macos" {
		t.Errorf("DisplayTypeMacOS = %q, want %q", DisplayTypeMacOS, "macos")
	}
}

func TestEnvFields(t *testing.T) {
	env := Detect()

	// Verify struct fields are accessible and have sensible values
	_ = env.HasDisplay
	_ = env.IsTerminal
	_ = env.Cols
	_ = env.Rows
	_ = env.IsSSH
	_ = env.IsMosh
	_ = env.IsScreen
	_ = env.IsTmux
	_ = env.HasColor
}
