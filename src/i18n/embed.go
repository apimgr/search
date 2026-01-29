package i18n

import (
	"embed"
)

//go:embed locales/*.json
var localesFS embed.FS

// DefaultManager creates a new i18n manager with embedded translations
// Uses the default supported languages and loads from embedded locales
func DefaultManager() (*Manager, error) {
	return newManagerFromFS(localesFS, "locales", "en", DefaultSupportedLanguages())
}

// newManagerFromFS creates a new i18n manager from the given filesystem
// This is an internal function used by DefaultManager and for testing
func newManagerFromFS(fs embed.FS, dir, defaultLang string, supported []string) (*Manager, error) {
	m := NewManager(defaultLang, supported)
	if err := m.LoadFromFS(fs, dir); err != nil {
		return nil, err
	}
	return m, nil
}

// LocalesFS returns the embedded locales filesystem for external use
func LocalesFS() embed.FS {
	return localesFS
}
