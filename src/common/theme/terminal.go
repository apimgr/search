package theme

// TerminalPalette holds ANSI 256-color indices (as strings) for terminal
// rendering. Per AI.md PART 32: CLI and TUI must use this ANSI-mapped palette
// rather than the literal hex values in ThemePalette, because terminals cannot
// render truecolor reliably across every emulator.
type TerminalPalette struct {
	Name       string
	Foreground string
	Background string
	Primary    string
	Secondary  string
	Accent     string
	Success    string
	Warning    string
	Error      string
	Muted      string
	Border     string
}

// TerminalDark is the ANSI mapping of the Dark (Dracula) theme.
var TerminalDark = TerminalPalette{
	Name:       "dark",
	Foreground: "255",
	Background: "235",
	Primary:    "141",
	Secondary:  "61",
	Accent:     "117",
	Success:    "84",
	Warning:    "215",
	Error:      "203",
	Muted:      "61",
	Border:     "238",
}

// TerminalLight is the ANSI mapping of the Light theme.
var TerminalLight = TerminalPalette{
	Name:       "light",
	Foreground: "235",
	Background: "255",
	Primary:    "99",
	Secondary:  "238",
	Accent:     "32",
	Success:    "29",
	Warning:    "172",
	Error:      "160",
	Muted:      "61",
	Border:     "254",
}

// TerminalPalettes maps a theme name to its ANSI palette.
var TerminalPalettes = map[string]TerminalPalette{
	"dark":  TerminalDark,
	"light": TerminalLight,
}

// GetTerminalPalette returns the ANSI palette for a theme name.
// Unknown names fall back to the dark palette, which is the documented default.
func GetTerminalPalette(name string) TerminalPalette {
	if p, ok := TerminalPalettes[name]; ok {
		return p
	}
	return TerminalDark
}
