package terminal

import (
	"os"
	"strings"
)

// Symbols provides terminal symbols with Unicode/ASCII fallback
type Symbols struct {
	// Status indicators
	Success string
	Error   string
	Warning string
	Info    string
	Pending string

	// Progress
	Spinner    []string
	ProgressFG string
	ProgressBG string

	// Borders
	BoxTopLeft     string
	BoxTopRight    string
	BoxBottomLeft  string
	BoxBottomRight string
	BoxHorizontal  string
	BoxVertical    string

	// Arrows
	ArrowRight string
	ArrowLeft  string
	ArrowUp    string
	ArrowDown  string

	// Misc
	Bullet   string
	Check    string
	Cross    string
	Ellipsis string
}

// Unicode symbols (modern terminals)
var UnicodeSymbols = Symbols{
	Success: "✓",
	Error:   "✗",
	Warning: "⚠",
	Info:    "ℹ",
	Pending: "◌",

	Spinner:    []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
	ProgressFG: "█",
	ProgressBG: "░",

	BoxTopLeft:     "┌",
	BoxTopRight:    "┐",
	BoxBottomLeft:  "└",
	BoxBottomRight: "┘",
	BoxHorizontal:  "─",
	BoxVertical:    "│",

	ArrowRight: "→",
	ArrowLeft:  "←",
	ArrowUp:    "↑",
	ArrowDown:  "↓",

	Bullet:   "•",
	Check:    "✓",
	Cross:    "✗",
	Ellipsis: "…",
}

// ASCII symbols (fallback for limited terminals)
var ASCIISymbols = Symbols{
	Success: "[OK]",
	Error:   "[ERR]",
	Warning: "[WARN]",
	Info:    "[INFO]",
	Pending: "[...]",

	Spinner:    []string{"|", "/", "-", "\\"},
	ProgressFG: "#",
	ProgressBG: "-",

	BoxTopLeft:     "+",
	BoxTopRight:    "+",
	BoxBottomLeft:  "+",
	BoxBottomRight: "+",
	BoxHorizontal:  "-",
	BoxVertical:    "|",

	ArrowRight: "->",
	ArrowLeft:  "<-",
	ArrowUp:    "^",
	ArrowDown:  "v",

	Bullet:   "*",
	Check:    "[x]",
	Cross:    "[X]",
	Ellipsis: "...",
}

// GetSymbols returns the appropriate symbol set based on terminal capabilities
func GetSymbols() Symbols {
	if supportsUnicode() {
		return UnicodeSymbols
	}
	return ASCIISymbols
}

// supportsUnicode checks if the terminal likely supports Unicode
func supportsUnicode() bool {
	// Check LANG/LC_ALL for UTF-8
	for _, env := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		val := os.Getenv(env)
		if strings.Contains(strings.ToLower(val), "utf-8") ||
			strings.Contains(strings.ToLower(val), "utf8") {
			return true
		}
	}

	// Check TERM for known Unicode-capable terminals
	term := os.Getenv("TERM")
	unicodeTerms := []string{
		"xterm", "rxvt", "screen", "tmux", "vt100",
		"linux", "konsole", "gnome", "alacritty", "kitty",
	}
	for _, t := range unicodeTerms {
		if strings.Contains(term, t) {
			return true
		}
	}

	// Default to ASCII for safety
	return false
}
