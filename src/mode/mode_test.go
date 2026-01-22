package mode

import (
	"os"
	"testing"
)

func TestAppModeString(t *testing.T) {
	tests := []struct {
		mode AppMode
		want string
	}{
		{Production, "production"},
		{Development, "development"},
		{AppMode(99), "production"}, // Invalid mode should return default "production"
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

	tests := []struct {
		name  string
		mode  AppMode
		debug bool
		want  string
	}{
		{"production without debug", Production, false, "production"},
		{"production with debug", Production, true, "production [debugging]"},
		{"development without debug", Development, false, "development"},
		{"development with debug", Development, true, "development [debugging]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentMode = tt.mode
			debugEnabled = tt.debug
			got := GetAppModeString()
			if got != tt.want {
				t.Errorf("GetAppModeString() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFromEnv(t *testing.T) {
	// Save original state
	origMode := currentMode
	origDebug := debugEnabled
	origModeEnv := os.Getenv("MODE")
	origDebugEnv := os.Getenv("DEBUG")
	defer func() {
		currentMode = origMode
		debugEnabled = origDebug
		os.Setenv("MODE", origModeEnv)
		os.Setenv("DEBUG", origDebugEnv)
	}()

	tests := []struct {
		name      string
		modeEnv   string
		debugEnv  string
		wantMode  AppMode
		wantDebug bool
	}{
		{
			name:      "no environment variables",
			modeEnv:   "",
			debugEnv:  "",
			wantMode:  Production,
			wantDebug: false,
		},
		{
			name:      "MODE=dev",
			modeEnv:   "dev",
			debugEnv:  "",
			wantMode:  Development,
			wantDebug: false,
		},
		{
			name:      "MODE=development",
			modeEnv:   "development",
			debugEnv:  "",
			wantMode:  Development,
			wantDebug: false,
		},
		{
			name:      "MODE=prod",
			modeEnv:   "prod",
			debugEnv:  "",
			wantMode:  Production,
			wantDebug: false,
		},
		{
			name:      "MODE=production",
			modeEnv:   "production",
			debugEnv:  "",
			wantMode:  Production,
			wantDebug: false,
		},
		{
			name:      "DEBUG=true",
			modeEnv:   "",
			debugEnv:  "true",
			wantMode:  Production,
			wantDebug: true,
		},
		{
			name:      "DEBUG=1",
			modeEnv:   "",
			debugEnv:  "1",
			wantMode:  Production,
			wantDebug: true,
		},
		{
			name:      "DEBUG=yes",
			modeEnv:   "",
			debugEnv:  "yes",
			wantMode:  Production,
			wantDebug: true,
		},
		{
			name:      "DEBUG=false (not truthy)",
			modeEnv:   "",
			debugEnv:  "false",
			wantMode:  Production,
			wantDebug: false,
		},
		{
			name:      "MODE=dev and DEBUG=true",
			modeEnv:   "dev",
			debugEnv:  "true",
			wantMode:  Development,
			wantDebug: true,
		},
		{
			name:      "MODE=production and DEBUG=1",
			modeEnv:   "production",
			debugEnv:  "1",
			wantMode:  Production,
			wantDebug: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset state before each test
			currentMode = Production
			debugEnabled = false

			// Set environment variables
			os.Setenv("MODE", tt.modeEnv)
			os.Setenv("DEBUG", tt.debugEnv)

			// Call FromEnv
			FromEnv()

			// Verify results
			if currentMode != tt.wantMode {
				t.Errorf("FromEnv() currentMode = %v, want %v", currentMode, tt.wantMode)
			}
			if debugEnabled != tt.wantDebug {
				t.Errorf("FromEnv() debugEnabled = %v, want %v", debugEnabled, tt.wantDebug)
			}
		})
	}
}

func TestFromEnvModeNotSet(t *testing.T) {
	// Save original state
	origMode := currentMode
	origDebug := debugEnabled
	origModeEnv := os.Getenv("MODE")
	origDebugEnv := os.Getenv("DEBUG")
	defer func() {
		currentMode = origMode
		debugEnabled = origDebug
		os.Setenv("MODE", origModeEnv)
		os.Setenv("DEBUG", origDebugEnv)
	}()

	// Test that MODE="" doesn't change existing mode
	currentMode = Development
	debugEnabled = false
	os.Setenv("MODE", "")
	os.Setenv("DEBUG", "")

	FromEnv()

	// Mode should remain unchanged when MODE env is empty
	if currentMode != Development {
		t.Errorf("FromEnv() with empty MODE should not change currentMode, got %v want %v", currentMode, Development)
	}
}

func TestUpdateAppModeProfilingSettings(t *testing.T) {
	// Save original
	origDebug := debugEnabled
	defer func() {
		debugEnabled = origDebug
		// Reset profiling to default state
		updateAppModeProfilingSettings()
	}()

	// Test debug enabled path - profiling should be enabled
	debugEnabled = true
	updateAppModeProfilingSettings()
	// Function sets runtime.SetBlockProfileRate(1) and runtime.SetMutexProfileFraction(1)
	// We can't easily verify the runtime state, but we verify the function doesn't panic

	// Test debug disabled path - profiling should be disabled
	debugEnabled = false
	updateAppModeProfilingSettings()
	// Function sets runtime.SetBlockProfileRate(0) and runtime.SetMutexProfileFraction(0)
}
