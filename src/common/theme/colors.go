// Package theme defines the shared semantic color palettes.
package theme

// ThemePalette contains the literal web color values shared by CSS, Swagger,
// and GraphiQL. Terminal interfaces use TerminalPalette instead.
type ThemePalette struct {
	Background string `json:"background"`
	Foreground string `json:"foreground"`
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Accent     string `json:"accent"`
	Success    string `json:"success"`
	Warning    string `json:"warning"`
	Error      string `json:"error"`
	Info       string `json:"info"`
	Surface    string `json:"surface"`
	SurfaceAlt string `json:"surface_alt"`
	Border     string `json:"border"`
	Muted      string `json:"muted"`
}

// ThemePaletteDark is the literal dark-mode palette defined by AI.md PART 16.
var ThemePaletteDark = ThemePalette{
	Background: "#282a36", Foreground: "#f8f8f2",
	Primary: "#bd93f9", Secondary: "#50fa7b", Accent: "#ff79c6",
	Success: "#50fa7b", Warning: "#ffb86c", Error: "#ff5555", Info: "#8be9fd",
	Surface: "#2b2d3a", SurfaceAlt: "#21222c", Border: "#44475a", Muted: "#6272a4",
}

// ThemePaletteLight is the literal light-mode palette defined by AI.md PART 16.
var ThemePaletteLight = ThemePalette{
	Background: "#ffffff", Foreground: "#1f2328",
	Primary: "#0969da", Secondary: "#1a7f37", Accent: "#8250df",
	Success: "#1a7f37", Warning: "#9a6700", Error: "#d1242f", Info: "#0969da",
	Surface: "#f6f8fa", SurfaceAlt: "#eff2f5", Border: "#d1d9e0", Muted: "#59636e",
}

// GetThemePalette returns a palette by mode. Unknown modes use the dark default.
func GetThemePalette(mode string) ThemePalette {
	if mode == "light" {
		return ThemePaletteLight
	}
	return ThemePaletteDark
}

// CSSPaletteVariables returns CSS custom properties for the shared palette.
func CSSPaletteVariables(selector string, palette ThemePalette) string {
	return selector + " {\n" +
		"  --palette-background: " + palette.Background + ";\n" +
		"  --palette-foreground: " + palette.Foreground + ";\n" +
		"  --palette-primary: " + palette.Primary + ";\n" +
		"  --palette-secondary: " + palette.Secondary + ";\n" +
		"  --palette-accent: " + palette.Accent + ";\n" +
		"  --palette-success: " + palette.Success + ";\n" +
		"  --palette-warning: " + palette.Warning + ";\n" +
		"  --palette-error: " + palette.Error + ";\n" +
		"  --palette-info: " + palette.Info + ";\n" +
		"  --palette-surface: " + palette.Surface + ";\n" +
		"  --palette-surface-alt: " + palette.SurfaceAlt + ";\n" +
		"  --palette-border: " + palette.Border + ";\n" +
		"  --palette-muted: " + palette.Muted + ";\n" +
		"}\n"
}
