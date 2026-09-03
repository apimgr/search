package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/apimgr/search/src/common/theme"
)

// TUITheme defines lipgloss colors for TUI rendering.
// Per AI.md PART 32: colors must match ThemePalette from common/theme (see PART 16).
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

// tuiThemeFromPalette derives a TUITheme from a shared common/theme.Theme
// so the TUI palette never drifts from the server frontend palette.
func tuiThemeFromPalette(t theme.Theme) TUITheme {
	return TUITheme{
		Name:       t.Name,
		Background: lipgloss.Color(t.Colors.BGPrimary.Hex),
		Foreground: lipgloss.Color(t.Colors.TextPrimary.Hex),
		Primary:    lipgloss.Color(t.Colors.AccentPrimary.Hex),
		Secondary:  lipgloss.Color(t.Colors.TextSecondary.Hex),
		Accent:     lipgloss.Color(t.Colors.AccentInfo.Hex),
		Error:      lipgloss.Color(t.Colors.AccentError.Hex),
		Success:    lipgloss.Color(t.Colors.AccentSuccess.Hex),
		Warning:    lipgloss.Color(t.Colors.AccentWarning.Hex),
		Muted:      lipgloss.Color(t.Colors.BorderColor.Hex),
	}
}

// TUIThemeDark is the dark theme (default) - matches server frontend ThemePaletteDark.
var TUIThemeDark = tuiThemeFromPalette(theme.Dark)

// TUIThemeLight is the light theme - matches server frontend ThemePaletteLight.
var TUIThemeLight = tuiThemeFromPalette(theme.Light)

// CurrentTUITheme is the active theme (set from cli.yml tui.theme).
var CurrentTUITheme = TUIThemeDark

// SetTUITheme selects the active TUI theme by name.
// Per AI.md PART 32 cli.yml reference: supports "dark", "light", or "system".
// "system" and any unrecognized value fall back to the dark theme (documented default).
func SetTUITheme(name string) {
	switch name {
	case "light":
		CurrentTUITheme = TUIThemeLight
	default:
		CurrentTUITheme = TUIThemeDark
	}
}
