package theme

import "testing"

func TestThemePaletteMatchesSpecification(t *testing.T) {
	dark := ThemePaletteDark
	if dark.Background != "#282a36" || dark.Foreground != "#f8f8f2" || dark.Primary != "#bd93f9" || dark.Secondary != "#50fa7b" || dark.Accent != "#ff79c6" || dark.Success != "#50fa7b" || dark.Warning != "#ffb86c" || dark.Error != "#ff5555" || dark.Info != "#8be9fd" || dark.Surface != "#2b2d3a" || dark.SurfaceAlt != "#21222c" || dark.Border != "#44475a" || dark.Muted != "#6272a4" {
		t.Fatalf("dark theme palette does not match AI.md: %#v", dark)
	}
	light := ThemePaletteLight
	if light.Background != "#ffffff" || light.Foreground != "#1f2328" || light.Primary != "#0969da" || light.Secondary != "#1a7f37" || light.Accent != "#8250df" || light.Success != "#1a7f37" || light.Warning != "#9a6700" || light.Error != "#d1242f" || light.Info != "#0969da" || light.Surface != "#f6f8fa" || light.SurfaceAlt != "#eff2f5" || light.Border != "#d1d9e0" || light.Muted != "#59636e" {
		t.Fatalf("light theme palette does not match AI.md: %#v", light)
	}
}

func TestPaletteSelection(t *testing.T) {
	if GetThemePalette("light") != ThemePaletteLight || GetThemePalette("dark") != ThemePaletteDark || GetThemePalette("auto") != ThemePaletteDark {
		t.Fatal("palette selection did not use light or dark defaults")
	}
	if GetTerminalPalette("light") != TerminalPaletteLight || GetTerminalPalette("dark") != TerminalPaletteDark {
		t.Fatal("terminal palette selection did not match the selected theme")
	}
}

func TestTerminalPalettesUseSpecANSIIndices(t *testing.T) {
	if TerminalPaletteDark != (TerminalPalette{Foreground: "15", Muted: "7", Primary: "13", Success: "10", Warning: "11", Error: "9", Info: "12", Border: "13"}) {
		t.Fatalf("dark terminal palette does not match AI.md: %#v", TerminalPaletteDark)
	}
	if TerminalPaletteLight != (TerminalPalette{Foreground: "0", Muted: "8", Primary: "4", Success: "2", Warning: "3", Error: "1", Info: "4", Border: "4"}) {
		t.Fatalf("light terminal palette does not match AI.md: %#v", TerminalPaletteLight)
	}
}

func TestCSSPaletteVariables(t *testing.T) {
	css := CSSPaletteVariables(":root", ThemePaletteDark)
	for _, expected := range []string{"--palette-background: #282a36", "--palette-accent: #ff79c6", "--palette-surface-alt: #21222c"} {
		if !contains(css, expected) {
			t.Errorf("CSS palette is missing %q", expected)
		}
	}
}

func contains(text, value string) bool {
	for i := 0; i+len(value) <= len(text); i++ {
		if text[i:i+len(value)] == value {
			return true
		}
	}
	return false
}
