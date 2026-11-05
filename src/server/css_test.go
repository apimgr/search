package server

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// hardcodedColor matches a literal hex or rgb/rgba color in a stylesheet.
var hardcodedColor = regexp.MustCompile(`(?i)(#[0-9a-f]{3,8}\b|\brgba?\s*\()`)

// variableDeclaration matches a line that DECLARES a custom property, i.e.
// one whose first meaningful token is `--`. Those lines are the token
// definitions themselves and are exactly where a literal color belongs.
var variableDeclaration = regexp.MustCompile(`^\s*--[a-zA-Z0-9-]+\s*:`)

// TestNoHardcodedColorsInStylesheets enforces AI.md PART 16 "CSS Variable
// Reference": colors are referenced through semantic custom properties, not
// written literally at the point of use. A literal color in a component rule
// is how a theme stops being themeable — it silently wins over the light-mode
// and auto-mode blocks in common.css, so the page renders with dark-theme
// colors in light mode with nothing failing.
//
// common.css is exempt: it is where the tokens are defined, so a literal there
// is the definition rather than a violation.
func TestNoHardcodedColorsInStylesheets(t *testing.T) {
	files := []struct{ name, path string }{
		{"public.css", "static/css/public.css"},
		{"components.css", "static/css/components.css"},
	}

	for _, f := range files {
		t.Run(f.name, func(t *testing.T) {
			b, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatalf("cannot read %s: %v", f.path, err)
			}
			var violations []string
			for i, line := range strings.Split(string(b), "\n") {
				trimmed := strings.TrimSpace(line)
				// A custom-property declaration is where a literal belongs.
				if trimmed == "" || strings.HasPrefix(trimmed, "/*") ||
					trimmed == "*" || variableDeclaration.MatchString(line) {
					continue
				}
				if hardcodedColor.MatchString(line) {
					violations = append(violations,
						"  line "+strconv.Itoa(i+1)+": "+trimmed)
				}
			}
			if len(violations) > 0 {
				t.Errorf("%s has %d hardcoded color(s); use a var(--color-*) token instead:\n%s",
					f.name, len(violations), strings.Join(violations, "\n"))
			}
		})
	}
}

func TestCommonCSSUsesSharedThemePalettes(t *testing.T) {
	css, err := GetStaticFile("css/common.css")
	if err != nil {
		t.Fatalf("GetStaticFile(common.css) error = %v", err)
	}
	text := string(css)
	for _, expected := range []string{
		"--palette-background: #282a36",
		"--palette-accent: #ff79c6",
		"--palette-background: #ffffff",
		"--palette-primary: #0969da",
		"--color-bg: var(--palette-background)",
	} {
		if !strings.Contains(text, expected) {
			t.Errorf("generated common.css is missing %q", expected)
		}
	}
	if strings.Contains(text, "__DARK_") || strings.Contains(text, "__LIGHT_") {
		t.Fatal("generated common.css contains unresolved palette markers")
	}
}
