// Package theme provides color definitions for the application
// Per AI.md PART 17: Web Frontend
package theme

// Color represents a color value that can be used in templates and CSS
type Color struct {
	Hex  string // Hex color code (e.g., "#bd93f9")
	RGB  string // RGB value (e.g., "189, 147, 249")
	Name string // Human-readable name
}

// Theme represents a complete color theme
type Theme struct {
	Name        string
	Description string
	IsDark      bool
	Colors      ThemeColors
}

// ThemeColors contains all color values for a theme
type ThemeColors struct {
	// Backgrounds
	BGPrimary   Color
	BGSecondary Color
	BGTertiary  Color

	// Text
	TextPrimary   Color
	TextSecondary Color
	TextMuted     Color

	// Accents
	AccentPrimary   Color
	AccentSecondary Color
	AccentSuccess   Color
	AccentWarning   Color
	AccentError     Color
	AccentInfo      Color

	// UI Elements
	BorderColor Color
	ShadowColor Color
	InputBG     Color
	InputBorder Color
	InputFocus  Color
	LinkColor   Color
	LinkHover   Color
}

// Dark is the default dark theme (Dracula-based)
var Dark = Theme{
	Name:        "dark",
	Description: "Dark theme with Dracula color palette",
	IsDark:      true,
	Colors: ThemeColors{
		BGPrimary:       Color{Hex: "#282a36", RGB: "40, 42, 54", Name: "Background"},
		BGSecondary:     Color{Hex: "#44475a", RGB: "68, 71, 90", Name: "Current Line"},
		BGTertiary:      Color{Hex: "#1e1f29", RGB: "30, 31, 41", Name: "Dark Background"},
		TextPrimary:     Color{Hex: "#f8f8f2", RGB: "248, 248, 242", Name: "Foreground"},
		TextSecondary:   Color{Hex: "#6272a4", RGB: "98, 114, 164", Name: "Comment"},
		TextMuted:       Color{Hex: "#6272a4", RGB: "98, 114, 164", Name: "Muted"},
		AccentPrimary:   Color{Hex: "#bd93f9", RGB: "189, 147, 249", Name: "Purple"},
		AccentSecondary: Color{Hex: "#ff79c6", RGB: "255, 121, 198", Name: "Pink"},
		AccentSuccess:   Color{Hex: "#50fa7b", RGB: "80, 250, 123", Name: "Green"},
		AccentWarning:   Color{Hex: "#ffb86c", RGB: "255, 184, 108", Name: "Orange"},
		AccentError:     Color{Hex: "#ff5555", RGB: "255, 85, 85", Name: "Red"},
		AccentInfo:      Color{Hex: "#8be9fd", RGB: "139, 233, 253", Name: "Cyan"},
		BorderColor:     Color{Hex: "#44475a", RGB: "68, 71, 90", Name: "Border"},
		ShadowColor:     Color{Hex: "rgba(0, 0, 0, 0.3)", RGB: "0, 0, 0", Name: "Shadow"},
		InputBG:         Color{Hex: "#44475a", RGB: "68, 71, 90", Name: "Input Background"},
		InputBorder:     Color{Hex: "#6272a4", RGB: "98, 114, 164", Name: "Input Border"},
		InputFocus:      Color{Hex: "#bd93f9", RGB: "189, 147, 249", Name: "Input Focus"},
		LinkColor:       Color{Hex: "#8be9fd", RGB: "139, 233, 253", Name: "Link"},
		LinkHover:       Color{Hex: "#bd93f9", RGB: "189, 147, 249", Name: "Link Hover"},
	},
}

// Light is the light theme
var Light = Theme{
	Name:        "light",
	Description: "Light theme for bright environments",
	IsDark:      false,
	Colors: ThemeColors{
		BGPrimary:       Color{Hex: "#f8f8f2", RGB: "248, 248, 242", Name: "Background"},
		BGSecondary:     Color{Hex: "#ffffff", RGB: "255, 255, 255", Name: "Secondary Background"},
		BGTertiary:      Color{Hex: "#e8e8e8", RGB: "232, 232, 232", Name: "Tertiary Background"},
		TextPrimary:     Color{Hex: "#282a36", RGB: "40, 42, 54", Name: "Text"},
		TextSecondary:   Color{Hex: "#44475a", RGB: "68, 71, 90", Name: "Secondary Text"},
		TextMuted:       Color{Hex: "#6272a4", RGB: "98, 114, 164", Name: "Muted Text"},
		AccentPrimary:   Color{Hex: "#7c3aed", RGB: "124, 58, 237", Name: "Primary"},
		AccentSecondary: Color{Hex: "#db2777", RGB: "219, 39, 119", Name: "Secondary"},
		AccentSuccess:   Color{Hex: "#059669", RGB: "5, 150, 105", Name: "Success"},
		AccentWarning:   Color{Hex: "#d97706", RGB: "217, 119, 6", Name: "Warning"},
		AccentError:     Color{Hex: "#dc2626", RGB: "220, 38, 38", Name: "Error"},
		AccentInfo:      Color{Hex: "#0284c7", RGB: "2, 132, 199", Name: "Info"},
		BorderColor:     Color{Hex: "#e5e7eb", RGB: "229, 231, 235", Name: "Border"},
		ShadowColor:     Color{Hex: "rgba(0, 0, 0, 0.1)", RGB: "0, 0, 0", Name: "Shadow"},
		InputBG:         Color{Hex: "#ffffff", RGB: "255, 255, 255", Name: "Input Background"},
		InputBorder:     Color{Hex: "#d1d5db", RGB: "209, 213, 219", Name: "Input Border"},
		InputFocus:      Color{Hex: "#7c3aed", RGB: "124, 58, 237", Name: "Input Focus"},
		LinkColor:       Color{Hex: "#0284c7", RGB: "2, 132, 199", Name: "Link"},
		LinkHover:       Color{Hex: "#7c3aed", RGB: "124, 58, 237", Name: "Link Hover"},
	},
}

// Themes is a map of all available themes
var Themes = map[string]Theme{
	"dark":  Dark,
	"light": Light,
}

// GetTheme returns a theme by name, defaulting to dark if not found
func GetTheme(name string) Theme {
	if theme, ok := Themes[name]; ok {
		return theme
	}
	return Dark
}

// AvailableThemes returns a list of available theme names
func AvailableThemes() []string {
	names := make([]string, 0, len(Themes))
	for name := range Themes {
		names = append(names, name)
	}
	return names
}

// CSS Variable Names - these correspond to CSS custom properties in main.css
const (
	CSSVarBGPrimary       = "--bg-primary"
	CSSVarBGSecondary     = "--bg-secondary"
	CSSVarBGTertiary      = "--bg-tertiary"
	CSSVarTextPrimary     = "--text-primary"
	CSSVarTextSecondary   = "--text-secondary"
	CSSVarTextMuted       = "--text-muted"
	CSSVarAccentPrimary   = "--accent-primary"
	CSSVarAccentSecondary = "--accent-secondary"
	CSSVarAccentSuccess   = "--accent-success"
	CSSVarAccentWarning   = "--accent-warning"
	CSSVarAccentError     = "--accent-error"
	CSSVarAccentInfo      = "--accent-info"
	CSSVarBorderColor     = "--border-color"
	CSSVarShadowColor     = "--shadow-color"
	CSSVarInputBG         = "--input-bg"
	CSSVarInputBorder     = "--input-border"
	CSSVarInputFocus      = "--input-focus"
	CSSVarLinkColor       = "--link-color"
	CSSVarLinkHover       = "--link-hover"
)

// ToCSSVariables converts theme colors to CSS variable assignments
func (t Theme) ToCSSVariables() map[string]string {
	return map[string]string{
		CSSVarBGPrimary:       t.Colors.BGPrimary.Hex,
		CSSVarBGSecondary:     t.Colors.BGSecondary.Hex,
		CSSVarBGTertiary:      t.Colors.BGTertiary.Hex,
		CSSVarTextPrimary:     t.Colors.TextPrimary.Hex,
		CSSVarTextSecondary:   t.Colors.TextSecondary.Hex,
		CSSVarTextMuted:       t.Colors.TextMuted.Hex,
		CSSVarAccentPrimary:   t.Colors.AccentPrimary.Hex,
		CSSVarAccentSecondary: t.Colors.AccentSecondary.Hex,
		CSSVarAccentSuccess:   t.Colors.AccentSuccess.Hex,
		CSSVarAccentWarning:   t.Colors.AccentWarning.Hex,
		CSSVarAccentError:     t.Colors.AccentError.Hex,
		CSSVarAccentInfo:      t.Colors.AccentInfo.Hex,
		CSSVarBorderColor:     t.Colors.BorderColor.Hex,
		CSSVarShadowColor:     t.Colors.ShadowColor.Hex,
		CSSVarInputBG:         t.Colors.InputBG.Hex,
		CSSVarInputBorder:     t.Colors.InputBorder.Hex,
		CSSVarInputFocus:      t.Colors.InputFocus.Hex,
		CSSVarLinkColor:       t.Colors.LinkColor.Hex,
		CSSVarLinkHover:       t.Colors.LinkHover.Hex,
	}
}
