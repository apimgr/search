//go:build !linux && !darwin && !windows

package theme

import (
	"testing"
)

// TestDetectSystemDarkOnOtherOS tests the default branch of DetectSystemDark
// This test only runs on platforms that are not Linux, Darwin, or Windows
func TestDetectSystemDarkOnOtherOS(t *testing.T) {
	// On unknown platforms, DetectSystemDark should return true (default to dark)
	result := DetectSystemDark()
	if !result {
		t.Error("DetectSystemDark() on unknown OS should default to true (dark mode)")
	}
}

// TestGetSystemThemeOnOtherOS tests GetSystemTheme on unknown platforms
func TestGetSystemThemeOnOtherOS(t *testing.T) {
	theme := GetSystemTheme()
	// Theme should be dark since unknown platforms default to dark
	if theme.Name != "dark" {
		t.Errorf("GetSystemTheme() on unknown OS should return dark theme, got: %s", theme.Name)
	}
	if !theme.IsDark {
		t.Error("GetSystemTheme().IsDark should be true on unknown OS")
	}
}
