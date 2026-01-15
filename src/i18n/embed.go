package i18n

import (
	"embed"
)

//go:embed locales/*.json
var localesFS embed.FS

// DefaultManager creates a new i18n manager with embedded translations
// Uses the default supported languages and loads from embedded locales
func DefaultManager() (*Manager, error) {
	m := NewManager("en", DefaultSupportedLanguages())
	if err := m.LoadFromFS(localesFS, "locales"); err != nil {
		return nil, err
	}
	return m, nil
}

// LocalesFS returns the embedded locales filesystem for external use
func LocalesFS() embed.FS {
	return localesFS
}
