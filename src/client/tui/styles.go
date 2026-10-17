package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/apimgr/search/src/common/theme"
)

// TUIStyles holds the lipgloss styles used across the TUI.
// Per AI.md PART 32: styles are derived from the ANSI TerminalPalette so the
// TUI renders correctly on terminals without truecolor support.
type TUIStyles struct {
	Base     lipgloss.Style
	Title    lipgloss.Style
	Selected lipgloss.Style
	Error    lipgloss.Style
	Success  lipgloss.Style
	Warning  lipgloss.Style
	Muted    lipgloss.Style
	Border   lipgloss.Style
}

// StylesFromTerminalPalette builds the TUI style set from an ANSI palette.
func StylesFromTerminalPalette(p theme.TerminalPalette) TUIStyles {
	return TUIStyles{
		Base: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Foreground)),
		Title: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Primary)).
			Bold(true).
			Padding(0, 1),
		Selected: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Background)).
			Background(lipgloss.Color(p.Primary)).
			Bold(true),
		Error: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Error)),
		Success: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Success)),
		Warning: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Warning)),
		Muted: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Muted)),
		Border: lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(p.Border)).
			Padding(0, 1),
	}
}

// CurrentStyles is the active style set, rebuilt whenever the theme changes.
var CurrentStyles = StylesFromTerminalPalette(theme.TerminalDark)
