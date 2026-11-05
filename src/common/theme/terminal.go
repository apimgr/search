package theme

// TerminalPalette contains ANSI 16-color indices (0–15) for CLI/TUI output.
// Terminal colors intentionally map semantic roles instead of using hex RGB.
type TerminalPalette struct {
	Foreground string `json:"foreground"`
	Muted      string `json:"muted"`
	Primary    string `json:"primary"`
	Success    string `json:"success"`
	Warning    string `json:"warning"`
	Error      string `json:"error"`
	Info       string `json:"info"`
	Border     string `json:"border"`
}

// TerminalPaletteDark is the ANSI mapping for the dark theme.
var TerminalPaletteDark = TerminalPalette{
	Foreground: "15", Muted: "7", Primary: "13",
	Success: "10", Warning: "11", Error: "9", Info: "12", Border: "13",
}

// TerminalPaletteLight is the ANSI mapping for the light theme.
var TerminalPaletteLight = TerminalPalette{
	Foreground: "0", Muted: "8", Primary: "4",
	Success: "2", Warning: "3", Error: "1", Info: "4", Border: "4",
}

// GetTerminalPalette returns the ANSI palette for the selected theme.
func GetTerminalPalette(mode string) TerminalPalette {
	if mode == "light" {
		return TerminalPaletteLight
	}
	return TerminalPaletteDark
}
