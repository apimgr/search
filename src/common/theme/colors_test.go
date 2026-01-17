package theme

import (
	"strings"
	"testing"
)

func TestColorStruct(t *testing.T) {
	color := Color{
		Hex:  "#ff0000",
		RGB:  "255, 0, 0",
		Name: "Red",
	}

	if color.Hex != "#ff0000" {
		t.Errorf("Hex = %q, want %q", color.Hex, "#ff0000")
	}
	if color.RGB != "255, 0, 0" {
		t.Errorf("RGB = %q, want %q", color.RGB, "255, 0, 0")
	}
	if color.Name != "Red" {
		t.Errorf("Name = %q, want %q", color.Name, "Red")
	}
}

func TestThemeStruct(t *testing.T) {
	theme := Theme{
		Name:        "test",
		Description: "Test theme",
		IsDark:      true,
	}

	if theme.Name != "test" {
		t.Errorf("Name = %q, want %q", theme.Name, "test")
	}
	if !theme.IsDark {
		t.Error("IsDark should be true")
	}
}

func TestDarkTheme(t *testing.T) {
	if Dark.Name != "dark" {
		t.Errorf("Dark.Name = %q, want %q", Dark.Name, "dark")
	}
	if !Dark.IsDark {
		t.Error("Dark.IsDark should be true")
	}
	if Dark.Colors.BGPrimary.Hex == "" {
		t.Error("Dark.Colors.BGPrimary.Hex should not be empty")
	}
	if Dark.Colors.TextPrimary.Hex == "" {
		t.Error("Dark.Colors.TextPrimary.Hex should not be empty")
	}
	if Dark.Colors.AccentPrimary.Hex == "" {
		t.Error("Dark.Colors.AccentPrimary.Hex should not be empty")
	}
}

func TestLightTheme(t *testing.T) {
	if Light.Name != "light" {
		t.Errorf("Light.Name = %q, want %q", Light.Name, "light")
	}
	if Light.IsDark {
		t.Error("Light.IsDark should be false")
	}
	if Light.Colors.BGPrimary.Hex == "" {
		t.Error("Light.Colors.BGPrimary.Hex should not be empty")
	}
	if Light.Colors.TextPrimary.Hex == "" {
		t.Error("Light.Colors.TextPrimary.Hex should not be empty")
	}
}

func TestThemesMap(t *testing.T) {
	if len(Themes) < 2 {
		t.Errorf("Themes should have at least 2 themes, got %d", len(Themes))
	}

	if _, ok := Themes["dark"]; !ok {
		t.Error("Themes should contain 'dark'")
	}
	if _, ok := Themes["light"]; !ok {
		t.Error("Themes should contain 'light'")
	}
}

func TestGetTheme(t *testing.T) {
	tests := []struct {
		name     string
		themeName string
		want     string
	}{
		{"dark", "dark", "dark"},
		{"light", "light", "light"},
		{"unknown defaults to dark", "unknown", "dark"},
		{"empty defaults to dark", "", "dark"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := GetTheme(tt.themeName)
			if theme.Name != tt.want {
				t.Errorf("GetTheme(%q).Name = %q, want %q", tt.themeName, theme.Name, tt.want)
			}
		})
	}
}

func TestAvailableThemes(t *testing.T) {
	themes := AvailableThemes()
	if len(themes) < 2 {
		t.Errorf("AvailableThemes() should return at least 2 themes, got %d", len(themes))
	}

	// Check that dark and light are in the list
	hasDark := false
	hasLight := false
	for _, name := range themes {
		if name == "dark" {
			hasDark = true
		}
		if name == "light" {
			hasLight = true
		}
	}

	if !hasDark {
		t.Error("AvailableThemes() should include 'dark'")
	}
	if !hasLight {
		t.Error("AvailableThemes() should include 'light'")
	}
}

func TestCSSVariableConstants(t *testing.T) {
	// Test that CSS variable constants are defined correctly
	tests := []struct {
		varName string
		want    string
	}{
		{CSSVarBGPrimary, "--bg-primary"},
		{CSSVarBGSecondary, "--bg-secondary"},
		{CSSVarBGTertiary, "--bg-tertiary"},
		{CSSVarTextPrimary, "--text-primary"},
		{CSSVarTextSecondary, "--text-secondary"},
		{CSSVarTextMuted, "--text-muted"},
		{CSSVarAccentPrimary, "--accent-primary"},
		{CSSVarAccentSecondary, "--accent-secondary"},
		{CSSVarAccentSuccess, "--accent-success"},
		{CSSVarAccentWarning, "--accent-warning"},
		{CSSVarAccentError, "--accent-error"},
		{CSSVarAccentInfo, "--accent-info"},
		{CSSVarBorderColor, "--border-color"},
		{CSSVarShadowColor, "--shadow-color"},
		{CSSVarInputBG, "--input-bg"},
		{CSSVarInputBorder, "--input-border"},
		{CSSVarInputFocus, "--input-focus"},
		{CSSVarLinkColor, "--link-color"},
		{CSSVarLinkHover, "--link-hover"},
	}

	for _, tt := range tests {
		if tt.varName != tt.want {
			t.Errorf("CSS variable = %q, want %q", tt.varName, tt.want)
		}
	}
}

func TestToCSSVariables(t *testing.T) {
	vars := Dark.ToCSSVariables()

	if len(vars) == 0 {
		t.Error("ToCSSVariables() should return non-empty map")
	}

	// Check that required variables are present
	requiredVars := []string{
		CSSVarBGPrimary,
		CSSVarTextPrimary,
		CSSVarAccentPrimary,
		CSSVarBorderColor,
	}

	for _, varName := range requiredVars {
		if _, ok := vars[varName]; !ok {
			t.Errorf("ToCSSVariables() should include %q", varName)
		}
	}

	// Check that values are hex colors
	if vars[CSSVarBGPrimary] == "" {
		t.Errorf("ToCSSVariables()[%q] should not be empty", CSSVarBGPrimary)
	}
}

func TestThemeColorsStruct(t *testing.T) {
	colors := ThemeColors{
		BGPrimary:     Color{Hex: "#000000"},
		TextPrimary:   Color{Hex: "#ffffff"},
		AccentPrimary: Color{Hex: "#ff0000"},
	}

	if colors.BGPrimary.Hex != "#000000" {
		t.Errorf("BGPrimary.Hex = %q, want %q", colors.BGPrimary.Hex, "#000000")
	}
	if colors.TextPrimary.Hex != "#ffffff" {
		t.Errorf("TextPrimary.Hex = %q, want %q", colors.TextPrimary.Hex, "#ffffff")
	}
}

func TestDarkThemeColors(t *testing.T) {
	// Verify Dark theme has Dracula-based colors
	if Dark.Colors.BGPrimary.Hex != "#282a36" {
		t.Errorf("Dark.Colors.BGPrimary.Hex = %q, want %q", Dark.Colors.BGPrimary.Hex, "#282a36")
	}
	if Dark.Colors.AccentPrimary.Hex != "#bd93f9" {
		t.Errorf("Dark.Colors.AccentPrimary.Hex = %q, want %q", Dark.Colors.AccentPrimary.Hex, "#bd93f9")
	}
}

func TestLightThemeColors(t *testing.T) {
	// Verify Light theme has appropriate light colors
	if Light.Colors.BGPrimary.Hex != "#f8f8f2" {
		t.Errorf("Light.Colors.BGPrimary.Hex = %q, want %q", Light.Colors.BGPrimary.Hex, "#f8f8f2")
	}
}

// Tests for css.go

func TestGenerateCSS(t *testing.T) {
	css := Dark.GenerateCSS()

	if css == "" {
		t.Error("GenerateCSS() returned empty string")
	}
	if !strings.Contains(css, ":root {") {
		t.Error("GenerateCSS() should contain ':root {'")
	}
	if !strings.Contains(css, "--bg-primary:") {
		t.Error("GenerateCSS() should contain '--bg-primary:'")
	}
	if !strings.Contains(css, "--text-primary:") {
		t.Error("GenerateCSS() should contain '--text-primary:'")
	}
	if !strings.HasSuffix(css, "}\n") {
		t.Error("GenerateCSS() should end with '}\\n'")
	}
}

func TestGenerateCSSBlock(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		wantHas  string
	}{
		{"body selector", "body", "body {"},
		{"class selector", ".theme-dark", ".theme-dark {"},
		{"id selector", "#app", "#app {"},
		{"attribute selector", "[data-theme='dark']", "[data-theme='dark'] {"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			css := Dark.GenerateCSSBlock(tt.selector)
			if !strings.Contains(css, tt.wantHas) {
				t.Errorf("GenerateCSSBlock(%q) should contain %q", tt.selector, tt.wantHas)
			}
			if !strings.Contains(css, "--bg-primary:") {
				t.Errorf("GenerateCSSBlock(%q) should contain '--bg-primary:'", tt.selector)
			}
		})
	}
}

func TestGenerateThemeCSS(t *testing.T) {
	css := GenerateThemeCSS()

	if css == "" {
		t.Error("GenerateThemeCSS() returned empty string")
	}

	// Should have dark theme as default
	if !strings.Contains(css, ":root,") {
		t.Error("GenerateThemeCSS() should contain ':root,' for default dark theme")
	}
	if !strings.Contains(css, "[data-theme=\"dark\"]") {
		t.Error("GenerateThemeCSS() should contain '[data-theme=\"dark\"]'")
	}

	// Should have light theme
	if !strings.Contains(css, "[data-theme=\"light\"]") {
		t.Error("GenerateThemeCSS() should contain '[data-theme=\"light\"]'")
	}

	// Should have system preference media query
	if !strings.Contains(css, "@media (prefers-color-scheme: light)") {
		t.Error("GenerateThemeCSS() should contain '@media (prefers-color-scheme: light)'")
	}
	if !strings.Contains(css, ":root:not([data-theme])") {
		t.Error("GenerateThemeCSS() should contain ':root:not([data-theme])'")
	}
}

func TestGenerateLipglossStyles(t *testing.T) {
	styles := Dark.GenerateLipglossStyles()

	if styles == "" {
		t.Error("GenerateLipglossStyles() returned empty string")
	}

	// Should have auto-generated comment
	if !strings.Contains(styles, "// Auto-generated lipgloss styles for dark theme") {
		t.Error("GenerateLipglossStyles() should contain theme name comment")
	}

	// Should have color variables
	expectedVars := []string{
		"darkBackground",
		"darkForeground",
		"darkPrimary",
		"darkSuccess",
		"darkWarning",
		"darkError",
		"darkInfo",
		"darkBorder",
	}

	for _, varName := range expectedVars {
		if !strings.Contains(styles, varName) {
			t.Errorf("GenerateLipglossStyles() should contain %q", varName)
		}
	}

	// Should have lipgloss.Color format
	if !strings.Contains(styles, "lipgloss.Color(") {
		t.Error("GenerateLipglossStyles() should contain 'lipgloss.Color('")
	}
}

func TestGenerateLipglossStylesLight(t *testing.T) {
	styles := Light.GenerateLipglossStyles()

	if !strings.Contains(styles, "// Auto-generated lipgloss styles for light theme") {
		t.Error("GenerateLipglossStyles() for light theme should mention 'light'")
	}
	if !strings.Contains(styles, "lightBackground") {
		t.Error("GenerateLipglossStyles() for light theme should have 'lightBackground'")
	}
}

func TestToJSON(t *testing.T) {
	jsonStr := Dark.ToJSON()

	if jsonStr == "" {
		t.Error("ToJSON() returned empty string")
	}

	// Should be valid JSON structure
	if !strings.HasPrefix(jsonStr, "{") {
		t.Error("ToJSON() should start with '{'")
	}
	if !strings.HasSuffix(jsonStr, "}") {
		t.Error("ToJSON() should end with '}'")
	}

	// Should have camelCase variable names (not CSS var format)
	if strings.Contains(jsonStr, "--bg-primary") {
		t.Error("ToJSON() should convert to camelCase, not contain '--bg-primary'")
	}
	if !strings.Contains(jsonStr, "bgPrimary") {
		t.Error("ToJSON() should contain 'bgPrimary' in camelCase")
	}
}

func TestCssVarToJSName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"--bg-primary", "bgPrimary"},
		{"--text-secondary", "textSecondary"},
		{"--accent-primary", "accentPrimary"},
		{"--border-color", "borderColor"},
		{"--shadow-color", "shadowColor"},
		{"--input-bg", "inputBg"},
		{"--link-hover", "linkHover"},
		{"simple", "simple"},       // No dashes
		{"--single", "single"},     // Single word after --
		{"--a-b-c", "aBC"},         // Multiple parts
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cssVarToJSName(tt.input)
			if got != tt.want {
				t.Errorf("cssVarToJSName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Tests for detect.go

func TestDetectSystemDark(t *testing.T) {
	// DetectSystemDark should return a boolean without error
	// The actual value depends on the system, so we just test it doesn't panic
	result := DetectSystemDark()
	// Result should be either true or false (valid boolean)
	if result != true && result != false {
		t.Error("DetectSystemDark() should return a valid boolean")
	}
}

func TestGetSystemTheme(t *testing.T) {
	theme := GetSystemTheme()

	// Should return a valid theme
	if theme.Name == "" {
		t.Error("GetSystemTheme().Name should not be empty")
	}

	// Should be either dark or light
	if theme.Name != "dark" && theme.Name != "light" {
		t.Errorf("GetSystemTheme().Name = %q, want 'dark' or 'light'", theme.Name)
	}

	// Theme should have valid colors
	if theme.Colors.BGPrimary.Hex == "" {
		t.Error("GetSystemTheme().Colors.BGPrimary.Hex should not be empty")
	}
	if theme.Colors.TextPrimary.Hex == "" {
		t.Error("GetSystemTheme().Colors.TextPrimary.Hex should not be empty")
	}
}

func TestGetSystemThemeMatchesDetection(t *testing.T) {
	isDark := DetectSystemDark()
	theme := GetSystemTheme()

	if isDark && theme.Name != "dark" {
		t.Error("When DetectSystemDark() returns true, GetSystemTheme() should return dark theme")
	}
	if !isDark && theme.Name != "light" {
		t.Error("When DetectSystemDark() returns false, GetSystemTheme() should return light theme")
	}
}
