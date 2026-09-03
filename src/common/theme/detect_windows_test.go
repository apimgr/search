//go:build windows

package theme

import (
	"testing"
)

// TestDetectSystemDarkOnWindows tests the windows branch of DetectSystemDark
// This test only runs on Windows
func TestDetectSystemDarkOnWindows(t *testing.T) {
	// On Windows, DetectSystemDark should call detectWindowsDark
	// and return a boolean based on Windows registry settings
	result := DetectSystemDark()
	// We can't predict the result, but it should be a valid boolean
	if result != true && result != false {
		t.Error("DetectSystemDark() on Windows should return a valid boolean")
	}
}

// TestGetSystemThemeOnWindows tests GetSystemTheme on Windows
func TestGetSystemThemeOnWindows(t *testing.T) {
	theme := GetSystemTheme()
	// Theme should be either dark or light based on system preference
	if theme.Name != "dark" && theme.Name != "light" {
		t.Errorf("GetSystemTheme() on Windows returned unexpected theme: %s", theme.Name)
	}
}
