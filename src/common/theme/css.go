// Package theme provides CSS variable generation
// Per AI.md PART 7: Common Go Modules - theme/css.go
package theme

import (
	"fmt"
	"strings"
)

// GenerateCSS generates CSS :root block with all theme variables
func (t Theme) GenerateCSS() string {
	var sb strings.Builder
	sb.WriteString(":root {\n")
	for varName, value := range t.ToCSSVariables() {
		sb.WriteString(fmt.Sprintf("  %s: %s;\n", varName, value))
	}
	sb.WriteString("}\n")
	return sb.String()
}

// GenerateCSSBlock generates a CSS block for a specific selector
func (t Theme) GenerateCSSBlock(selector string) string {
	var sb strings.Builder
	sb.WriteString(selector)
	sb.WriteString(" {\n")
	for varName, value := range t.ToCSSVariables() {
		sb.WriteString(fmt.Sprintf("  %s: %s;\n", varName, value))
	}
	sb.WriteString("}\n")
	return sb.String()
}

// GenerateThemeCSS generates CSS for both light and dark themes with data-theme selectors
func GenerateThemeCSS() string {
	var sb strings.Builder

	// Dark theme (default)
	sb.WriteString(":root,\n[data-theme=\"dark\"] {\n")
	for varName, value := range Dark.ToCSSVariables() {
		sb.WriteString(fmt.Sprintf("  %s: %s;\n", varName, value))
	}
	sb.WriteString("}\n\n")

	// Light theme
	sb.WriteString("[data-theme=\"light\"] {\n")
	for varName, value := range Light.ToCSSVariables() {
		sb.WriteString(fmt.Sprintf("  %s: %s;\n", varName, value))
	}
	sb.WriteString("}\n\n")

	// System preference media query
	sb.WriteString("@media (prefers-color-scheme: light) {\n")
	sb.WriteString("  :root:not([data-theme]) {\n")
	for varName, value := range Light.ToCSSVariables() {
		sb.WriteString(fmt.Sprintf("    %s: %s;\n", varName, value))
	}
	sb.WriteString("  }\n")
	sb.WriteString("}\n")

	return sb.String()
}

// GenerateLipglossStyles generates Go code for lipgloss terminal styles
func (t Theme) GenerateLipglossStyles() string {
	var sb strings.Builder
	sb.WriteString("// Auto-generated lipgloss styles for ")
	sb.WriteString(t.Name)
	sb.WriteString(" theme\n\n")

	sb.WriteString(fmt.Sprintf("var %sBackground = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.BGPrimary.Hex))
	sb.WriteString(fmt.Sprintf("var %sForeground = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.TextPrimary.Hex))
	sb.WriteString(fmt.Sprintf("var %sPrimary = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.AccentPrimary.Hex))
	sb.WriteString(fmt.Sprintf("var %sSuccess = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.AccentSuccess.Hex))
	sb.WriteString(fmt.Sprintf("var %sWarning = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.AccentWarning.Hex))
	sb.WriteString(fmt.Sprintf("var %sError = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.AccentError.Hex))
	sb.WriteString(fmt.Sprintf("var %sInfo = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.AccentInfo.Hex))
	sb.WriteString(fmt.Sprintf("var %sBorder = lipgloss.Color(\"%s\")\n", t.Name, t.Colors.BorderColor.Hex))

	return sb.String()
}

// ToJSON returns theme colors as JSON for JavaScript consumption
func (t Theme) ToJSON() string {
	vars := t.ToCSSVariables()
	var sb strings.Builder
	sb.WriteString("{\n")
	i := 0
	total := len(vars)
	for varName, value := range vars {
		// Convert CSS var name to camelCase for JS
		jsName := cssVarToJSName(varName)
		sb.WriteString(fmt.Sprintf("  \"%s\": \"%s\"", jsName, value))
		if i < total-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
		i++
	}
	sb.WriteString("}")
	return sb.String()
}

// cssVarToJSName converts --bg-primary to bgPrimary
func cssVarToJSName(cssVar string) string {
	// Remove leading --
	s := strings.TrimPrefix(cssVar, "--")
	// Split by -
	parts := strings.Split(s, "-")
	if len(parts) <= 1 {
		return s
	}
	// Capitalize all parts except first
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(string(parts[i][0])) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}
