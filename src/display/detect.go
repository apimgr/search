package display

import (
	"os"
	"runtime"

	"golang.org/x/term"
)

// Package-level function variables for testing
// These can be replaced in tests to mock system calls
var (
	isTerminalFunc = term.IsTerminal
	getSizeFunc    = term.GetSize
)

// DisplayType represents the type of display available
type DisplayType string

const (
	DisplayTypeNone    DisplayType = "none"
	DisplayTypeX11     DisplayType = "x11"
	DisplayTypeWayland DisplayType = "wayland"
	DisplayTypeWindows DisplayType = "windows"
	DisplayTypeMacOS   DisplayType = "macos"
)

// Env represents the detected display environment
type Env struct {
	// Display availability
	HasDisplay  bool        // X11, Wayland, Windows, macOS display available
	DisplayType DisplayType // Type of display: x11, wayland, windows, macos, none

	// Terminal state
	IsTerminal bool // stdout is a TTY
	Cols       int  // Terminal columns (0 if no terminal)
	Rows       int  // Terminal rows (0 if no terminal)

	// Remote session detection
	IsSSH    bool // Running over SSH
	IsMosh   bool // Running over mosh
	IsScreen bool // Running in GNU screen
	IsTmux   bool // Running in tmux

	// Environment
	OS       string // Runtime OS (linux, darwin, windows, freebsd, etc.)
	HasColor bool   // Terminal supports colors
}

// Detect detects the current display environment
func Detect() Env {
	env := Env{
		OS:          runtime.GOOS,
		DisplayType: DisplayTypeNone,
	}

	// Terminal detection
	env.IsTerminal = isTerminalFunc(int(os.Stdout.Fd()))
	if env.IsTerminal {
		cols, rows, err := getSizeFunc(int(os.Stdout.Fd()))
		if err == nil {
			env.Cols = cols
			env.Rows = rows
		}
	}

	// Remote session detection
	env.IsSSH = os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != "" || os.Getenv("SSH_CONNECTION") != ""
	env.IsMosh = os.Getenv("MOSH") != "" || os.Getenv("MOSH_CONNECTION") != ""
	env.IsScreen = os.Getenv("STY") != ""
	env.IsTmux = os.Getenv("TMUX") != ""

	// Color support detection
	env.HasColor = detectColorSupport()

	// Platform-specific display detection
	env.detectPlatformDisplay()

	return env
}

// detectColorSupport checks if the terminal supports colors
func detectColorSupport() bool {
	// Check NO_COLOR environment variable (standard)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	// Check TERM environment variable
	termEnv := os.Getenv("TERM")
	if termEnv == "" || termEnv == "dumb" {
		return false
	}

	// Check COLORTERM for truecolor support
	if os.Getenv("COLORTERM") != "" {
		return true
	}

	// Check FORCE_COLOR
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}

	// Most modern terminals support color
	return true
}

// GetMode determines the appropriate display mode based on environment
func (e Env) GetMode() Mode {
	// If not a terminal and no display, we're headless
	if !e.IsTerminal && !e.HasDisplay {
		return ModeHeadless
	}

	// If we have a GUI display and not over SSH, GUI mode is available
	if e.HasDisplay && !e.IsSSH && !e.IsMosh {
		return ModeGUI
	}

	// If we have a terminal, use TUI mode
	if e.IsTerminal {
		return ModeTUI
	}

	// Default to CLI mode
	return ModeCLI
}

// IsRemote returns true if running in a remote session
func (e Env) IsRemote() bool {
	return e.IsSSH || e.IsMosh
}

// IsMultiplexed returns true if running in a terminal multiplexer
func (e Env) IsMultiplexed() bool {
	return e.IsScreen || e.IsTmux
}

// TerminalSize returns the terminal size or default values
func (e Env) TerminalSize() (cols, rows int) {
	if e.Cols > 0 && e.Rows > 0 {
		return e.Cols, e.Rows
	}
	// Default terminal size
	return 80, 24
}
