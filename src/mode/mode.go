package mode

import (
	"os"
	"runtime"
	"strings"

	"github.com/apimgr/search/src/config"
)

var (
	currentMode  = Production
	debugEnabled = false
)

// AppMode represents the application operational mode
// Per AI.md PART 6: Application Modes
type AppMode int

const (
	Production AppMode = iota
	Development
)

func (m AppMode) String() string {
	switch m {
	case Development:
		return "development"
	default:
		return "production"
	}
}

// SetAppMode sets the application mode
// Per AI.md PART 6: Mode shortcuts (dev/development, prod/production)
func SetAppMode(m string) {
	switch strings.ToLower(m) {
	case "dev", "development":
		currentMode = Development
	default:
		currentMode = Production
	}
	updateAppModeProfilingSettings()
}

// SetDebugEnabled enables or disables debug mode
// Per AI.md PART 6: Debug Flag
func SetDebugEnabled(enabled bool) {
	debugEnabled = enabled
	updateAppModeProfilingSettings()
}

// updateAppModeProfilingSettings enables/disables profiling based on debug flag
func updateAppModeProfilingSettings() {
	if debugEnabled {
		// Enable profiling when debug is on
		runtime.SetBlockProfileRate(1)
		runtime.SetMutexProfileFraction(1)
	} else {
		// Disable profiling when debug is off
		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(0)
	}
}

// GetCurrentAppMode returns the current application mode
func GetCurrentAppMode() AppMode {
	return currentMode
}

// IsAppModeDev returns true if in development mode
// Per AI.md PART 1: Intent-revealing names required
func IsAppModeDev() bool {
	return currentMode == Development
}

// IsAppModeProd returns true if in production mode
// Per AI.md PART 1: Intent-revealing names required
func IsAppModeProd() bool {
	return currentMode == Production
}

// IsDebugEnabled returns true if debug mode is enabled (--debug or DEBUG=true)
// Per AI.md PART 6: Debug Flag
func IsDebugEnabled() bool {
	return debugEnabled
}

// GetAppModeString returns mode string with debug suffix if enabled
func GetAppModeString() string {
	s := currentMode.String()
	if debugEnabled {
		s += " [debugging]"
	}
	return s
}

// FromEnv sets mode and debug from environment variables
// Per AI.md PART 6: Mode and Debug Detection Priority
func FromEnv() {
	if m := os.Getenv("MODE"); m != "" {
		SetAppMode(m)
	}
	if config.IsTruthy(os.Getenv("DEBUG")) {
		SetDebugEnabled(true)
	}
}
