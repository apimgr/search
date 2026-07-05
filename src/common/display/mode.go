// Package display provides display environment detection for all binaries
// Per AI.md PART 7: BINARY REQUIREMENTS - Display Environment Detection
package display

// Mode represents the display mode of the application
type Mode int

const (
	// ModeHeadless - No display, no TTY (daemon, service, cron)
	ModeHeadless Mode = iota
	// ModeCLI - Command provided or piped output
	ModeCLI
	// ModeTUI - Terminal available, interactive (TTY, SSH, mosh, screen, tmux)
	ModeTUI
	// ModeGUI - Native display available (X11, Wayland, Windows, macOS)
	ModeGUI
)

// String returns the string representation of the mode
func (m Mode) String() string {
	switch m {
	case ModeHeadless:
		return "headless"
	case ModeCLI:
		return "cli"
	case ModeTUI:
		return "tui"
	case ModeGUI:
		return "gui"
	default:
		return "unknown"
	}
}

// SupportsInteraction returns true if the mode supports user interaction
func (m Mode) SupportsInteraction() bool {
	return m >= ModeTUI
}

// SupportsColors returns true if the mode supports colored output
func (m Mode) SupportsColors() bool {
	return m >= ModeCLI
}

// SupportsRichOutput returns true if the mode supports rich formatted output
func (m Mode) SupportsRichOutput() bool {
	return m >= ModeTUI
}

// IsInteractive returns true if running interactively
func (m Mode) IsInteractive() bool {
	return m == ModeTUI || m == ModeGUI
}

// IsDaemon returns true if running as daemon/service
func (m Mode) IsDaemon() bool {
	return m == ModeHeadless
}
