package config

import (
	"os"
	"testing"
)

// TestSetColorMode verifies SetColorMode stores and GetColorMode retrieves the override.
func TestSetColorMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     string
		wantMode string
	}{
		{"set always", "always", "always"},
		{"set never", "never", "never"},
		{"set auto", "auto", "auto"},
		{"set empty clears", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetColorMode(tt.mode)
			defer SetColorMode("")

			if tt.mode != "" {
				got := GetColorMode()
				if got != tt.wantMode {
					t.Errorf("GetColorMode() after SetColorMode(%q) = %q, want %q", tt.mode, got, tt.wantMode)
				}
			}
		})
	}
}

// TestGetColorModeEnvPriority verifies the standard NO_COLOR override.
func TestGetColorModeEnvPriority(t *testing.T) {
	SetColorMode("")
	t.Setenv("NO_COLOR", "1")
	if got := GetColorMode(); got != "never" {
		t.Errorf("GetColorMode() with NO_COLOR = %q, want never", got)
	}
}

// TestGetColorModeCLIOverrideBeatsEnv verifies CLI flag takes priority over env.
func TestGetColorModeCLIOverrideBeatsEnv(t *testing.T) {
	defer SetColorMode("")
	t.Setenv("NO_COLOR", "1")
	SetColorMode("always")

	got := GetColorMode()
	if got != "always" {
		t.Errorf("GetColorMode() with CLI override = %q, want 'always' (CLI beats env)", got)
	}
}

// TestSetBaseURLOverride verifies SetBaseURLOverride stores and GetBaseURL retrieves it.
func TestSetBaseURLOverride(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"custom prefix", "/search", "/search"},
		{"nested path", "/app/search", "/app/search"},
		{"root", "/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetBaseURLOverride(tt.baseURL)
			defer SetBaseURLOverride("")

			got := GetBaseURL()
			if got != tt.want {
				t.Errorf("GetBaseURL() after SetBaseURLOverride(%q) = %q, want %q", tt.baseURL, got, tt.want)
			}
		})
	}
}

// TestGetBaseURLDefaultIsSlash verifies the default return value is "/".
func TestGetBaseURLDefaultIsSlash(t *testing.T) {
	SetBaseURLOverride("")

	got := GetBaseURL()
	if got != "/" {
		t.Errorf("GetBaseURL() default = %q, want '/'", got)
	}
}

// TestIsColorEnabled verifies color detection by mode.
func TestIsColorEnabled(t *testing.T) {
	tests := []struct {
		name      string
		colorMode string
		want      bool
	}{
		{"always enabled", "always", true},
		{"never disabled", "never", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetColorMode(tt.colorMode)
			defer SetColorMode("")

			got := IsColorEnabled()
			if got != tt.want {
				t.Errorf("IsColorEnabled() with mode %q = %v, want %v", tt.colorMode, got, tt.want)
			}
		})
	}
}

// TestIsColorEnabledAutoMode verifies auto-detect returns a bool without panicking.
func TestIsColorEnabledAutoMode(t *testing.T) {
	SetColorMode("auto")
	defer SetColorMode("")

	// Must not panic; return value depends on terminal detection
	_ = IsColorEnabled()
}

// TestIsTerminalNonInteractive verifies isTerminal returns false in CI-like environments.
func TestIsTerminalNonInteractive(t *testing.T) {
	originalCI := os.Getenv("CI")
	originalTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("CI", originalCI)
		os.Setenv("TERM", originalTERM)
	}()

	os.Setenv("CI", "true")
	os.Setenv("TERM", "")

	got := isTerminal()
	if got {
		t.Error("isTerminal() should return false when CI=true")
	}
}

// TestIsTerminalDumb verifies isTerminal returns false for dumb terminals.
func TestIsTerminalDumb(t *testing.T) {
	originalCI := os.Getenv("CI")
	originalTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("CI", originalCI)
		os.Setenv("TERM", originalTERM)
	}()

	os.Setenv("CI", "")
	os.Setenv("TERM", "dumb")

	got := isTerminal()
	if got {
		t.Error("isTerminal() should return false when TERM=dumb")
	}
}

// TestIsTerminalWithTERM verifies isTerminal returns true when TERM is set (non-CI).
func TestIsTerminalWithTERM(t *testing.T) {
	originalCI := os.Getenv("CI")
	originalTERM := os.Getenv("TERM")
	defer func() {
		os.Setenv("CI", originalCI)
		os.Setenv("TERM", originalTERM)
	}()

	os.Setenv("CI", "")
	os.Setenv("TERM", "xterm-256color")

	got := isTerminal()
	if !got {
		t.Error("isTerminal() should return true when TERM is set and CI is not set")
	}
}
