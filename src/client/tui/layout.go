package tui

import (
	"strings"

	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/common/terminal"
)

// LayoutConfig describes the responsive layout for a terminal size tier.
// Per AI.md PART 32: the TUI must adapt every element to the current breakpoint.
type LayoutConfig struct {
	ShowBorders    bool
	ShowHeader     bool
	ShowFooter     bool
	ShowSidebar    bool
	SidebarWidth   int
	MaxColumns     int
	TruncateAt     int
	UseAbbrev      bool
	VerticalScroll bool
	MultiPane      bool
	TileLayout     bool
}

// GetLayoutConfig returns the layout for a terminal size mode.
// Per AI.md PART 32 responsive layout table.
func GetLayoutConfig(mode terminal.SizeMode) LayoutConfig {
	switch mode {
	case terminal.SizeModeMicro:
		return LayoutConfig{
			ShowBorders:    false,
			ShowHeader:     false,
			ShowFooter:     false,
			ShowSidebar:    false,
			MaxColumns:     2,
			TruncateAt:     30,
			UseAbbrev:      true,
			VerticalScroll: true,
		}
	case terminal.SizeModeMinimal:
		return LayoutConfig{
			ShowBorders:    false,
			ShowHeader:     true,
			ShowFooter:     true,
			ShowSidebar:    false,
			MaxColumns:     3,
			TruncateAt:     40,
			UseAbbrev:      true,
			VerticalScroll: true,
		}
	case terminal.SizeModeCompact:
		return LayoutConfig{
			ShowBorders:    true,
			ShowHeader:     true,
			ShowFooter:     true,
			ShowSidebar:    false,
			MaxColumns:     4,
			TruncateAt:     60,
			UseAbbrev:      false,
			VerticalScroll: true,
		}
	case terminal.SizeModeWide:
		return LayoutConfig{
			ShowBorders:    true,
			ShowHeader:     true,
			ShowFooter:     true,
			ShowSidebar:    true,
			SidebarWidth:   30,
			MaxColumns:     8,
			TruncateAt:     120,
			UseAbbrev:      false,
			VerticalScroll: true,
		}
	case terminal.SizeModeUltrawide:
		return LayoutConfig{
			ShowBorders:    true,
			ShowHeader:     true,
			ShowFooter:     true,
			ShowSidebar:    true,
			SidebarWidth:   40,
			MaxColumns:     12,
			TruncateAt:     200,
			UseAbbrev:      false,
			VerticalScroll: false,
			MultiPane:      true,
		}
	case terminal.SizeModeMassive:
		return LayoutConfig{
			ShowBorders:    true,
			ShowHeader:     true,
			ShowFooter:     true,
			ShowSidebar:    true,
			SidebarWidth:   50,
			MaxColumns:     20,
			TruncateAt:     0,
			UseAbbrev:      false,
			VerticalScroll: false,
			MultiPane:      true,
			TileLayout:     true,
		}
	default:
		return LayoutConfig{
			ShowBorders:    true,
			ShowHeader:     true,
			ShowFooter:     true,
			ShowSidebar:    false,
			MaxColumns:     6,
			TruncateAt:     80,
			UseAbbrev:      false,
			VerticalScroll: true,
		}
	}
}

// Spacing constants used for responsive padding per AI.md PART 32.
const (
	SpaceXS = 1
	SpaceS  = 2
	SpaceM  = 4
	SpaceL  = 6
	SpaceXL = 8
)

// GetSpacingForMode returns the horizontal spacing unit for a size mode.
func GetSpacingForMode(mode terminal.SizeMode) int {
	switch mode {
	case terminal.SizeModeMicro:
		return 0
	case terminal.SizeModeMinimal:
		return SpaceXS
	case terminal.SizeModeCompact:
		return SpaceS
	case terminal.SizeModeStandard:
		return SpaceS
	case terminal.SizeModeWide:
		return SpaceM
	case terminal.SizeModeUltrawide:
		return SpaceL
	case terminal.SizeModeMassive:
		return SpaceXL
	default:
		return SpaceS
	}
}

// SymbolSet holds the glyphs used by the TUI.
type SymbolSet struct {
	Success   string
	Error     string
	Warning   string
	Info      string
	Arrow     string
	Checked   string
	Unchecked string
	Bullet    string
	Plus      string
	Spinner   []string
}

// TUISymbols is the Unicode symbol set per AI.md PART 32.
var TUISymbols = SymbolSet{
	Success:   "✓",
	Error:     "✗",
	Warning:   "⚠",
	Info:      "ℹ",
	Arrow:     "→",
	Checked:   "☑",
	Unchecked: "☒",
	Bullet:    "•",
	Plus:      "+",
	Spinner:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

// TUISymbolsASCII is the ASCII fallback symbol set per AI.md PART 32.
var TUISymbolsASCII = SymbolSet{
	Success:   "[OK]",
	Error:     "[ERR]",
	Warning:   "[WARN]",
	Info:      "[INFO]",
	Arrow:     "->",
	Checked:   "[x]",
	Unchecked: "[ ]",
	Bullet:    "*",
	Plus:      "+",
	Spinner:   []string{"|", "/", "-", "\\"},
}

// GetTUISymbols selects the Unicode or ASCII symbol set for the environment.
// Per AI.md PART 32: fall back to ASCII when the terminal cannot render
// Unicode or when emoji/rich output is disabled.
func GetTUISymbols(env display.Env) SymbolSet {
	if !env.IsTerminal || !display.EmojiEnabled() {
		return TUISymbolsASCII
	}
	return TUISymbols
}

// truncate shortens s to at most width runes, appending an ellipsis.
// A width of zero or less means no truncation.
func truncate(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 3 {
		return string(r[:width])
	}
	return string(r[:width-3]) + "..."
}

// truncateMiddle shortens s to at most width runes, removing the middle so
// both the start and end remain readable (useful for long URLs).
func truncateMiddle(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	if width <= 5 {
		return string(r[:width])
	}
	head := (width - 3) / 2
	tail := width - 3 - head
	return string(r[:head]) + "..." + string(r[len(r)-tail:])
}

// ensureVisible adjusts the scroll offset so index stays within the viewport.
func ensureVisible(index, offset, pageSize int) int {
	if pageSize <= 0 {
		return 0
	}
	if index < offset {
		return index
	}
	if index >= offset+pageSize {
		return index - pageSize + 1
	}
	return offset
}

// pageSize returns how many rows fit in the content area for a given height.
// chrome is the number of rows consumed by header, input, and footer.
func pageSize(height, chrome int) int {
	n := height - chrome
	if n < 1 {
		return 1
	}
	return n
}

// padTo right-pads s with spaces so it occupies at least width columns.
func padTo(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(r))
}
