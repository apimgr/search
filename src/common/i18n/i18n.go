package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var printfDirective = regexp.MustCompile(`%[#+\- 0]*[0-9]*(\.[0-9]+)?[a-zA-Z]`)

// Manager handles translations and language detection
type Manager struct {
	mu sync.RWMutex
	// lang -> key -> value
	translations map[string]map[string]string
	defaultLang  string
	supported    []string
	rtlLangs     map[string]bool
}

// NewManager creates a new i18n manager
func NewManager(defaultLang string, supported []string) *Manager {
	rtl := make(map[string]bool)
	for code, language := range Languages {
		if language.RTL {
			rtl[code] = true
		}
	}

	return &Manager{
		translations: make(map[string]map[string]string),
		defaultLang:  defaultLang,
		supported:    supported,
		rtlLangs:     rtl,
	}
}

// LoadFromFS loads translations from an embedded filesystem
// Supports both flat and nested JSON structures
// Nested keys are flattened with dot notation: {"auth": {"login": "Log In"}} -> "auth.login" = "Log In"
func (m *Manager) LoadFromFS(fs embed.FS, dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, lang := range m.supported {
		path := fmt.Sprintf("%s/%s.json", dir, lang)
		data, err := fs.ReadFile(path)
		if err != nil {
			// Skip missing languages, just use default
			continue
		}

		// Parse as generic interface to handle nested structures
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("failed to parse %s: %w", path, err)
		}

		// Flatten nested structure
		translations := make(map[string]string)
		flattenTranslations("", raw, translations)

		m.translations[lang] = translations
	}

	// Ensure default language is loaded
	if _, ok := m.translations[m.defaultLang]; !ok {
		return fmt.Errorf("default language %s not found", m.defaultLang)
	}

	return nil
}

// flattenTranslations recursively flattens nested JSON into dot-notation keys
func flattenTranslations(prefix string, value interface{}, result map[string]string) {
	switch v := value.(type) {
	case map[string]interface{}:
		for key, val := range v {
			newPrefix := key
			if prefix != "" {
				newPrefix = prefix + "." + key
			}
			flattenTranslations(newPrefix, val, result)
		}
	case string:
		if prefix != "" {
			result[prefix] = v
		}
	}
}

// LoadFromMap loads translations from a map (useful for testing)
func (m *Manager) LoadFromMap(lang string, translations map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.translations[lang] = translations
}

// T translates a key and applies the legacy printf-style arguments.
func (m *Manager) T(lang, key string, args ...interface{}) string {
	m.mu.RLock()
	value := key
	if translations, ok := m.translations[lang]; ok {
		if translated, ok := translations[key]; ok {
			value = translated
		}
	}
	if value == key {
		if translations, ok := m.translations[m.defaultLang]; ok {
			if translated, ok := translations[key]; ok {
				value = translated
			}
		}
	}
	m.mu.RUnlock()
	if len(args) == 0 {
		return value
	}
	if printfDirective.MatchString(value) {
		return fmt.Sprintf(value, args...)
	}
	for _, arg := range args {
		start := strings.IndexByte(value, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(value[start+1:], '}')
		if end < 0 {
			break
		}
		end += start + 1
		value = value[:start] + fmt.Sprint(arg) + value[end+1:]
	}
	return value
}

// Tf translates a key and replaces named {token} placeholders from key/value
// pairs. Values without a matching token are ignored.
func (m *Manager) Tf(lang, key string, args ...interface{}) string {
	m.mu.RLock()
	value := key

	// Try requested language
	if trans, ok := m.translations[lang]; ok {
		if val, ok := trans[key]; ok {
			value = val
		}
	}

	// Fallback to default language
	if value == key {
		if trans, ok := m.translations[m.defaultLang]; ok {
			if val, ok := trans[key]; ok {
				value = val
			}
		}
	}

	// Return key if not found
	m.mu.RUnlock()
	for i := 0; i+1 < len(args); i += 2 {
		name := fmt.Sprint(args[i])
		replacement := fmt.Sprint(args[i+1])
		value = strings.ReplaceAll(value, "{"+name+"}", replacement)
	}
	return value
}

// DetectLanguage detects the preferred language from the request
// Priority: 1. Query param 2. Cookie 3. Accept-Language header 4. Default
func (m *Manager) DetectLanguage(r *http.Request) string {
	// Check query param first
	if raw := strings.TrimSpace(r.URL.Query().Get("lang")); raw != "" {
		if lang := m.normalizeSupportedLanguage(raw); lang != "" {
			return lang
		}
		return m.defaultLang
	}

	// Check cookie first
	if cookie, err := r.Cookie("lang"); err == nil {
		lang := normalizeLanguageCode(cookie.Value)
		if m.IsSupported(lang) {
			return lang
		}
	}

	// Parse Accept-Language header
	accept := r.Header.Get("Accept-Language")
	if accept != "" {
		lang := m.parseAcceptLanguage(accept)
		if lang != "" {
			return lang
		}
	}

	return m.defaultLang
}

// ResolveLanguage detects the preferred language and persists a supported query
// parameter value to the language cookie.
func (m *Manager) ResolveLanguage(w http.ResponseWriter, r *http.Request) string {
	if raw := strings.TrimSpace(r.URL.Query().Get("lang")); raw != "" {
		if lang := m.normalizeSupportedLanguage(raw); lang != "" {
			if w != nil {
				SetLanguageCookie(w, lang)
			}
			return lang
		}
		return m.defaultLang
	}

	return m.DetectLanguage(r)
}

// ResolveSupportedLanguage normalizes a language code and falls back to the
// manager default if the value is unsupported.
func (m *Manager) ResolveSupportedLanguage(value string) string {
	if lang := m.normalizeSupportedLanguage(value); lang != "" {
		return lang
	}
	return m.defaultLang
}

// DetectRequestLocale resolves the request language and matching text
// direction using the default supported languages.
func DetectRequestLocale(r *http.Request) (string, string) {
	manager := NewManager("en", DefaultSupportedLanguages())
	lang := manager.DetectLanguage(r)
	if manager.IsRTL(lang) {
		return lang, "rtl"
	}
	return lang, "ltr"
}

// parseAcceptLanguage parses the Accept-Language header and returns the best match
func (m *Manager) parseAcceptLanguage(header string) string {
	// Parse header like "en-US,en;q=0.9,de;q=0.8"
	type langQuality struct {
		lang    string
		quality float64
	}

	var langs []langQuality

	parts := strings.Split(header, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Split by semicolon for quality value
		langParts := strings.Split(part, ";")
		lang := strings.TrimSpace(langParts[0])
		quality := 1.0

		if len(langParts) > 1 {
			qPart := strings.TrimSpace(langParts[1])
			qPart = strings.TrimPrefix(qPart, "q=")
			parsedQuality, err := strconv.ParseFloat(qPart, 64)
			if err != nil || parsedQuality < 0 || parsedQuality > 1 {
				continue
			}
			quality = parsedQuality
		}

		// Normalize language code (en-US -> en)
		if idx := strings.Index(lang, "-"); idx != -1 {
			lang = lang[:idx]
		}
		lang = strings.ToLower(lang)

		langs = append(langs, langQuality{lang: lang, quality: quality})
	}

	// Sort by quality descending
	sort.Slice(langs, func(i, j int) bool {
		return langs[i].quality > langs[j].quality
	})

	// Find first supported language
	for _, lq := range langs {
		if m.IsSupported(lq.lang) {
			return lq.lang
		}
	}

	return ""
}

func (m *Manager) normalizeSupportedLanguage(value string) string {
	lang := normalizeLanguageCode(value)
	if lang == "" || !m.IsSupported(lang) {
		return ""
	}
	return lang
}

func normalizeLanguageCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if idx := strings.IndexAny(value, "-_"); idx != -1 {
		value = value[:idx]
	}
	return value
}

// IsSupported checks if a language is supported
func (m *Manager) IsSupported(lang string) bool {
	for _, l := range m.supported {
		if l == lang {
			return true
		}
	}
	return false
}

// IsRTL checks if a language is right-to-left
func (m *Manager) IsRTL(lang string) bool {
	return m.rtlLangs[lang]
}

// SupportedLanguages returns the list of supported languages
func (m *Manager) SupportedLanguages() []Language {
	var result []Language
	for _, code := range m.supported {
		if lang, ok := Languages[code]; ok {
			result = append(result, lang)
		}
	}
	return result
}

// SupportedLanguageCodes returns the list of supported language codes
// Used for security.txt Preferred-Languages field per RFC 9116
func (m *Manager) SupportedLanguageCodes() []string {
	// Return a copy to prevent external modification
	result := make([]string, len(m.supported))
	copy(result, m.supported)
	return result
}

// DefaultLanguage returns the default language code
func (m *Manager) DefaultLanguage() string {
	return m.defaultLang
}

// Translator is a language-specific translator for use in templates
type Translator struct {
	manager *Manager
	lang    string
}

// NewTranslator creates a translator for a specific language
func (m *Manager) NewTranslator(lang string) *Translator {
	if !m.IsSupported(lang) {
		lang = m.defaultLang
	}
	return &Translator{
		manager: m,
		lang:    lang,
	}
}

// T translates a key
func (t *Translator) T(key string, args ...interface{}) string {
	return t.manager.T(t.lang, key, args...)
}

// Tf translates a key with named literal replacements.
func (t *Translator) Tf(key string, args ...interface{}) string {
	return t.manager.Tf(t.lang, key, args...)
}

// Lang returns the current language code
func (t *Translator) Lang() string {
	return t.lang
}

// IsRTL returns true if the current language is right-to-left
func (t *Translator) IsRTL() bool {
	return t.manager.IsRTL(t.lang)
}

// Dir returns "rtl" or "ltr" based on language direction
func (t *Translator) Dir() string {
	if t.IsRTL() {
		return "rtl"
	}
	return "ltr"
}

// Languages returns all supported languages
func (t *Translator) Languages() []Language {
	return t.manager.SupportedLanguages()
}

// TemplateFuncs returns template functions for i18n
func (m *Manager) TemplateFuncs(lang string) template.FuncMap {
	t := m.NewTranslator(lang)
	return template.FuncMap{
		"t": func(key string, args ...interface{}) string {
			return t.T(key, args...)
		},
		"tf": func(key string, kv ...interface{}) string {
			return t.Tf(key, kv...)
		},
		"lang": func() string {
			return t.Lang()
		},
		"isRTL": func() bool {
			return t.IsRTL()
		},
		"dir": func() string {
			return t.Dir()
		},
		"languages": func() []Language {
			return t.Languages()
		},
	}
}

// SetLanguageCookie sets the language preference cookie
func SetLanguageCookie(w http.ResponseWriter, lang string) {
	http.SetCookie(w, &http.Cookie{
		Name:  "lang",
		Value: lang,
		Path:  "/",
		// 1 year
		MaxAge:   365 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
