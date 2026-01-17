package mode

import (
	"testing"
)

func TestAppModeString(t *testing.T) {
	tests := []struct {
		mode AppMode
		want string
	}{
		{Production, "production"},
		{Development, "development"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("AppMode.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetAppMode(t *testing.T) {
	// Save original
	orig := currentMode
	defer func() { currentMode = orig }()

	tests := []struct {
		input string
		want  AppMode
	}{
		{"dev", Development},
		{"development", Development},
		{"Development", Development},
		{"DEV", Development},
		{"prod", Production},
		{"production", Production},
		{"Production", Production},
		{"PRODUCTION", Production},
		{"", Production},
		{"invalid", Production},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			SetAppMode(tt.input)
			got := GetCurrentAppMode()
			if got != tt.want {
				t.Errorf("SetAppMode(%q) -> GetCurrentAppMode() = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSetDebugEnabled(t *testing.T) {
	// Save original
	orig := debugEnabled
	defer func() { debugEnabled = orig }()

	SetDebugEnabled(true)
	if !IsDebugEnabled() {
		t.Error("SetDebugEnabled(true) -> IsDebugEnabled() = false, want true")
	}

	SetDebugEnabled(false)
	if IsDebugEnabled() {
		t.Error("SetDebugEnabled(false) -> IsDebugEnabled() = true, want false")
	}
}

func TestGetCurrentAppMode(t *testing.T) {
	// Save original
	orig := currentMode
	defer func() { currentMode = orig }()

	currentMode = Development
	if GetCurrentAppMode() != Development {
		t.Error("GetCurrentAppMode() should return Development")
	}

	currentMode = Production
	if GetCurrentAppMode() != Production {
		t.Error("GetCurrentAppMode() should return Production")
	}
}

func TestIsAppModeDev(t *testing.T) {
	orig := currentMode
	defer func() { currentMode = orig }()

	currentMode = Development
	if !IsAppModeDev() {
		t.Error("IsAppModeDev() should return true in development mode")
	}

	currentMode = Production
	if IsAppModeDev() {
		t.Error("IsAppModeDev() should return false in production mode")
	}
}

func TestIsAppModeProd(t *testing.T) {
	orig := currentMode
	defer func() { currentMode = orig }()

	currentMode = Production
	if !IsAppModeProd() {
		t.Error("IsAppModeProd() should return true in production mode")
	}

	currentMode = Development
	if IsAppModeProd() {
		t.Error("IsAppModeProd() should return false in development mode")
	}
}

func TestGetAppModeString(t *testing.T) {
	origMode := currentMode
	origDebug := debugEnabled
	defer func() {
		currentMode = origMode
		debugEnabled = origDebug
	}()

	currentMode = Production
	debugEnabled = false
	got := GetAppModeString()
	if got != "production" {
		t.Errorf("GetAppModeString() = %q, want %q", got, "production")
	}

	debugEnabled = true
	got = GetAppModeString()
	if got != "production [debugging]" {
		t.Errorf("GetAppModeString() with debug = %q, want %q", got, "production [debugging]")
	}

	currentMode = Development
	debugEnabled = false
	got = GetAppModeString()
	if got != "development" {
		t.Errorf("GetAppModeString() = %q, want %q", got, "development")
	}
}

func TestFromEnv(t *testing.T) {
	// Just verify it doesn't panic
	FromEnv()
}
