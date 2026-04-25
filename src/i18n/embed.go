package i18n

import (
	"embed"
	"net/http"
	"sync"
)

//go:embed locales/*.json
var localesFS embed.FS

var (
	defaultManagerOnce sync.Once
	defaultManager     *Manager
	defaultManagerErr  error
)

// DefaultManager creates a new i18n manager with embedded translations
// Uses the default supported languages and loads from embedded locales
func DefaultManager() (*Manager, error) {
	return newManagerFromFS(localesFS, "locales", "en", DefaultSupportedLanguages())
}

// CachedDefaultManager returns a singleton default manager loaded from the
// embedded locale files.
func CachedDefaultManager() (*Manager, error) {
	defaultManagerOnce.Do(func() {
		defaultManager, defaultManagerErr = DefaultManager()
	})
	return defaultManager, defaultManagerErr
}

// RequestString translates a key for the current request language using the
// cached embedded locale manager.
func RequestString(r *http.Request, key string, args ...interface{}) string {
	manager, err := CachedDefaultManager()
	if err != nil || manager == nil {
		return key
	}
	return manager.T(manager.DetectLanguage(r), key, args...)
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
