package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/apimgr/search/src/common/theme"
)

// TUITheme maps semantic TUI roles to ANSI indices, never literal hex colors.
type TUITheme struct {
	Name       string
	Background lipgloss.Color
	Foreground lipgloss.Color
	Primary    lipgloss.Color
	Secondary  lipgloss.Color
	Accent     lipgloss.Color
	Error      lipgloss.Color
	Success    lipgloss.Color
	Warning    lipgloss.Color
	Muted      lipgloss.Color
}

func tuiThemeFromPalette(name string, palette theme.TerminalPalette) TUITheme {
	return TUITheme{
		Name:       name,
		Background: lipgloss.Color(palette.Foreground),
		Foreground: lipgloss.Color(palette.Foreground),
		Primary:    lipgloss.Color(palette.Primary),
		Secondary:  lipgloss.Color(palette.Muted),
		Accent:     lipgloss.Color(palette.Info),
		Error:      lipgloss.Color(palette.Error),
		Success:    lipgloss.Color(palette.Success),
		Warning:    lipgloss.Color(palette.Warning),
		Muted:      lipgloss.Color(palette.Muted),
	}
}

// TUIThemeDark is derived from the ANSI dark terminal palette.
var TUIThemeDark = tuiThemeFromPalette("dark", theme.TerminalPaletteDark)

// TUIThemeLight is derived from the ANSI light terminal palette.
var TUIThemeLight = tuiThemeFromPalette("light", theme.TerminalPaletteLight)

// CurrentTUITheme is the active theme selected from cli.yml.
var CurrentTUITheme = TUIThemeDark

// SetTUITheme selects the active TUI theme by name.
func SetTUITheme(name string) {
	if name == "light" {
		CurrentTUITheme = TUIThemeLight
		CurrentStyles = StylesFromTerminalPalette(theme.TerminalPaletteLight)
	} else {
		CurrentTUITheme = TUIThemeDark
		CurrentStyles = StylesFromTerminalPalette(theme.TerminalPaletteDark)
	}
	refreshStyles()
}
