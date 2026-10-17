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
	Debug
)

func (m AppMode) String() string {
	switch m {
	case Development:
		return "development"
	case Debug:
		return "debug"
	default:
		return "production"
	}
}

// SetAppMode sets the application mode
// Per AI.md PART 6: Mode shortcuts (dev/devel/development, prod/production, debug)
func SetAppMode(m string) {
	switch strings.ToLower(m) {
	case "dev", "devel", "development":
		currentMode = Development
	case "debug":
		// Debug mode — explicit opt-in only; defaults the debug flag on
		// (an explicit --debug flag or DEBUG env var still wins)
		currentMode = Debug
		SetDebugEnabled(true)
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

// FromEnv sets mode and debug from environment variables.
// Per AI.md PART 6: MODE=debug selects debug mode and defaults the debug flag
// to on, but an explicitly set DEBUG env var (truthy OR falsy) always wins over
// that default — MODE=debug DEBUG=false runs debug mode with debug off.
// The --debug CLI flag (applied after this) wins over both.
func FromEnv() {
	if m := os.Getenv("MODE"); m != "" {
		SetAppMode(m)
	}
	// LookupEnv distinguishes "explicitly set" from "unset": an unset
	// DEBUG leaves the mode-defaulted value alone; a set DEBUG overrides it
	if v, set := os.LookupEnv("DEBUG"); set {
		SetDebugEnabled(config.IsTruthy(v))
	}
}
