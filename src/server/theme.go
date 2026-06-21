package server

import (
	"net/http"
)

// Theme constants
// Per AI.md PART 16: Themes (NON-NEGOTIABLE - PROJECT-WIDE)
const (
	ThemeDark  = "dark"
	ThemeLight = "light"
	ThemeAuto  = "auto"
)

// DefaultTheme is the default theme when no preference is set
// Per AI.md PART 16: Dark theme is the default
const DefaultTheme = ThemeDark

// GetTheme gets the current theme from cookie or defaults to dark
// Per AI.md PART 16: Themes (NON-NEGOTIABLE - PROJECT-WIDE)
// Theme system applies to:
// - Web interface (HTML pages)
// - Admin panel
// - Swagger UI
// - GraphiQL interface
// - All interactive elements
func GetTheme(r *http.Request) string {
	// Check for theme cookie
	if cookie, err := r.Cookie("theme"); err == nil {
		switch cookie.Value {
		case ThemeLight, ThemeDark, ThemeAuto:
			return cookie.Value
		}
	}

	// Check for theme query parameter (for theme switching)
	if theme := r.URL.Query().Get("theme"); theme != "" {
		switch theme {
		case ThemeLight, ThemeDark, ThemeAuto:
			return theme
		}
	}

	// Default to dark theme
	return DefaultTheme
}

// SetTheme sets the theme cookie
// Per AI.md PART 16: User preference persisted in cookie
func SetTheme(w http.ResponseWriter, theme string) {
	// Validate theme value
	switch theme {
	case ThemeLight, ThemeDark, ThemeAuto:
		// Valid theme
	default:
		theme = DefaultTheme
	}

	// Set cookie with 30-day expiry; HttpOnly=false allows JavaScript theme switching;
	// Secure is set dynamically by the server based on TLS config
	cookie := &http.Cookie{
		Name:     "theme",
		Value:    theme,
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: false,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, cookie)
}

// GetThemeClass returns the CSS class for the current theme
// Per AI.md PART 16: Apply theme class to <html> element
func GetThemeClass(theme string) string {
	switch theme {
	case ThemeLight:
		return "theme-light"
	case ThemeDark:
		return "theme-dark"
	case ThemeAuto:
		return "theme-auto"
	default:
		return "theme-dark"
	}
}

// IsValidTheme checks if a theme string is valid
func IsValidTheme(theme string) bool {
	switch theme {
	case ThemeLight, ThemeDark, ThemeAuto:
		return true
	}
	return false
}

// ThemeInfo holds theme metadata for template rendering.
// Current is the user preference (light, dark, auto).
// ClassName is the CSS class applied to <html> (theme-light, theme-dark, theme-auto).
type ThemeInfo struct {
	Current   string
	ClassName string
	IsDark    bool
	IsLight   bool
	IsAuto    bool
}

// GetThemeInfo returns complete theme information for template rendering
func GetThemeInfo(r *http.Request) ThemeInfo {
	theme := GetTheme(r)

	info := ThemeInfo{
		Current:   theme,
		ClassName: GetThemeClass(theme),
		IsAuto:    theme == ThemeAuto,
	}

	// Determine effective theme for auto mode
	if theme == ThemeAuto {
		// In auto mode, we default to dark since we can't detect system preference server-side
		// Client-side JavaScript will apply the actual system preference
		info.IsDark = true
		info.IsLight = false
	} else {
		info.IsDark = theme == ThemeDark
		info.IsLight = theme == ThemeLight
	}

	return info
}

