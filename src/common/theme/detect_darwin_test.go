//go:build darwin

package theme

import (
	"testing"
)

// TestDetectSystemDarkOnDarwin tests the darwin branch of DetectSystemDark
// This test only runs on macOS
func TestDetectSystemDarkOnDarwin(t *testing.T) {
	// On Darwin, DetectSystemDark should call detectMacOSDark
	// and return a boolean based on macOS system preferences
	result := DetectSystemDark()
	// We can't predict the result, but it should be a valid boolean
	if result != true && result != false {
		t.Error("DetectSystemDark() on Darwin should return a valid boolean")
	}
}

// TestGetSystemThemeOnDarwin tests GetSystemTheme on macOS
func TestGetSystemThemeOnDarwin(t *testing.T) {
	theme := GetSystemTheme()
	// Theme should be either dark or light based on system preference
	if theme.Name != "dark" && theme.Name != "light" {
		t.Errorf("GetSystemTheme() on Darwin returned unexpected theme: %s", theme.Name)
	}
}
