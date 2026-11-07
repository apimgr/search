package i18n

import (
	"fmt"
	"sort"
	"strings"
)

// Language is one entry in the supported-language registry.
type Language struct {
	Code             string
	Name             string
	NativeName       string
	RTL              bool
	Required         bool
	PluralCategories []string
}

// Languages is the single registry for language names, direction and CLDR
// cardinal plural categories. Its keys must match the embedded locale files.
var Languages = map[string]Language{
	"ar": {Code: "ar", Name: "Arabic", NativeName: "العربية", RTL: true, Required: true, PluralCategories: []string{"zero", "one", "two", "few", "many", "other"}},
	"de": {Code: "de", Name: "German", NativeName: "Deutsch", Required: true, PluralCategories: []string{"one", "other"}},
	"en": {Code: "en", Name: "English", NativeName: "English", Required: true, PluralCategories: []string{"one", "other"}},
	"es": {Code: "es", Name: "Spanish", NativeName: "Español", Required: true, PluralCategories: []string{"one", "other"}},
	"fa": {Code: "fa", Name: "Persian", NativeName: "فارسی", RTL: true, PluralCategories: []string{"one", "other"}},
	"fr": {Code: "fr", Name: "French", NativeName: "Français", Required: true, PluralCategories: []string{"one", "other"}},
	"he": {Code: "he", Name: "Hebrew", NativeName: "עברית", RTL: true, PluralCategories: []string{"one", "two", "other"}},
	"it": {Code: "it", Name: "Italian", NativeName: "Italiano", Required: true, PluralCategories: []string{"one", "many", "other"}},
	"ja": {Code: "ja", Name: "Japanese", NativeName: "日本語", PluralCategories: []string{"other"}},
	"nl": {Code: "nl", Name: "Dutch", NativeName: "Nederlands", PluralCategories: []string{"one", "other"}},
	"pl": {Code: "pl", Name: "Polish", NativeName: "Polski", PluralCategories: []string{"one", "few", "many", "other"}},
	"pt": {Code: "pt", Name: "Portuguese", NativeName: "Português", PluralCategories: []string{"one", "many", "other"}},
	"ru": {Code: "ru", Name: "Russian", NativeName: "Русский", PluralCategories: []string{"one", "few", "many", "other"}},
	"ur": {Code: "ur", Name: "Urdu", NativeName: "اردو", RTL: true, PluralCategories: []string{"one", "other"}},
	"zh": {Code: "zh", Name: "Chinese", NativeName: "中文", PluralCategories: []string{"other"}},
}

// DefaultSupportedLanguages returns the complete registry in a stable order.
// English appears first so it remains the default and initial selector option.
func DefaultSupportedLanguages() []string {
	return AllLanguageCodes()
}

func containsLanguageCode(codes []string, target string) bool {
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
}

// ValidateSupportedLanguages checks an operator-selected subset against the
// registry and enforces every language marked as required by the spec.
func ValidateSupportedLanguages(supported []string) error {
	if len(supported) == 0 {
		return fmt.Errorf("supported languages cannot be empty")
	}
	selected := make(map[string]bool, len(supported))
	for _, code := range supported {
		code = strings.ToLower(strings.TrimSpace(code))
		if _, ok := Languages[code]; !ok {
			return fmt.Errorf("unsupported language %q", code)
		}
		if selected[code] {
			return fmt.Errorf("language %q is listed more than once", code)
		}
		selected[code] = true
	}
	for code, language := range Languages {
		if language.Required && !selected[code] {
			return fmt.Errorf("required language %q cannot be disabled", code)
		}
	}
	return nil
}

// AllLanguageCodes returns all registry language codes in stable order.
func AllLanguageCodes() []string {
	codes := make([]string, 0, len(Languages))
	for code := range Languages {
		if code != "en" {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	return append([]string{"en"}, codes...)
}

// GetLanguage returns language information by code.
func GetLanguage(code string) (Language, bool) {
	lang, ok := Languages[code]
	return lang, ok
}

// IsValidLanguageCode reports whether code is registered as supported.
func IsValidLanguageCode(code string) bool {
	_, ok := Languages[code]
	return ok
}

// RTLLanguages returns the registered right-to-left language codes.
func RTLLanguages() []string {
	var rtl []string
	for code, lang := range Languages {
		if lang.RTL {
			rtl = append(rtl, code)
		}
	}
	sort.Strings(rtl)
	return rtl
}
