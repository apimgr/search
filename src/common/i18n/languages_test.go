package i18n

import (
	"io/fs"
	"strings"
	"testing"
)

func TestLanguageStruct(t *testing.T) {
	lang := Language{
		Code:             "en",
		Name:             "English",
		NativeName:       "English",
		RTL:              false,
		PluralCategories: []string{"one", "other"},
	}

	if lang.Code != "en" {
		t.Errorf("Code = %q, want %q", lang.Code, "en")
	}
	if lang.Name != "English" {
		t.Errorf("Name = %q, want %q", lang.Name, "English")
	}
	if lang.NativeName != "English" {
		t.Errorf("NativeName = %q, want %q", lang.NativeName, "English")
	}
	if lang.RTL {
		t.Error("RTL should be false for English")
	}
}

func TestLanguagesMap(t *testing.T) {
	for _, code := range []string{"en", "es", "zh", "fr", "ar", "de", "ja"} {
		if _, ok := Languages[code]; !ok {
			t.Errorf("Languages registry is missing required minimum code %q", code)
		}
	}
	if _, ok := Languages["ko"]; ok {
		t.Error("Korean must not be registered without an embedded locale file")
	}
}

func TestLanguagesData(t *testing.T) {
	tests := []struct {
		code       string
		name       string
		nativeName string
		rtl        bool
	}{
		{"en", "English", "English", false},
		{"de", "German", "Deutsch", false},
		{"fr", "French", "Français", false},
		{"ar", "Arabic", "العربية", true},
		{"he", "Hebrew", "עברית", true},
		{"fa", "Persian", "فارسی", true},
		{"ur", "Urdu", "اردو", true},
		{"ja", "Japanese", "日本語", false},
		{"zh", "Chinese", "中文", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			lang, ok := Languages[tt.code]
			if !ok {
				t.Fatalf("Language %q not found", tt.code)
			}
			if lang.Code != tt.code {
				t.Errorf("Code = %q, want %q", lang.Code, tt.code)
			}
			if lang.Name != tt.name {
				t.Errorf("Name = %q, want %q", lang.Name, tt.name)
			}
			if lang.NativeName != tt.nativeName {
				t.Errorf("NativeName = %q, want %q", lang.NativeName, tt.nativeName)
			}
			if lang.RTL != tt.rtl {
				t.Errorf("RTL = %v, want %v", lang.RTL, tt.rtl)
			}
		})
	}
}

func TestDefaultSupportedLanguages(t *testing.T) {
	defaults := DefaultSupportedLanguages()

	if len(defaults) != len(Languages) {
		t.Errorf("DefaultSupportedLanguages() returned %d, want every registry language (%d)", len(defaults), len(Languages))
	}

	// Check that English is first
	if defaults[0] != "en" {
		t.Errorf("First language should be 'en', got %q", defaults[0])
	}

	// Verify all codes are valid
	for _, code := range defaults {
		if _, ok := Languages[code]; !ok {
			t.Errorf("Invalid language code in defaults: %q", code)
		}
	}

	for _, code := range []string{"en", "es", "zh", "fr", "ar", "de", "ja"} {
		found := false
		for _, supported := range defaults {
			if supported == code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DefaultSupportedLanguages() is missing AI.md required language %q", code)
		}
	}
}

func TestAllLanguageCodes(t *testing.T) {
	codes := AllLanguageCodes()

	if len(codes) != len(Languages) {
		t.Errorf("AllLanguageCodes() returned %d, want %d", len(codes), len(Languages))
	}

	// Verify all codes are valid
	for _, code := range codes {
		if _, ok := Languages[code]; !ok {
			t.Errorf("Invalid language code: %q", code)
		}
	}
}

func TestGetLanguage(t *testing.T) {
	tests := []struct {
		code  string
		found bool
	}{
		{"en", true},
		{"de", true},
		{"ar", true},
		{"xx", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			lang, ok := GetLanguage(tt.code)
			if ok != tt.found {
				t.Errorf("GetLanguage(%q) found = %v, want %v", tt.code, ok, tt.found)
			}
			if tt.found && lang.Code != tt.code {
				t.Errorf("GetLanguage(%q) returned wrong code: %q", tt.code, lang.Code)
			}
		})
	}
}

func TestIsValidLanguageCode(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{"en", true},
		{"de", true},
		{"fr", true},
		{"ar", true},
		{"xx", false},
		{"", false},
		// Case sensitive
		{"EN", false},
		{"English", false},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := IsValidLanguageCode(tt.code)
			if got != tt.want {
				t.Errorf("IsValidLanguageCode(%q) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestRTLLanguages(t *testing.T) {
	rtl := RTLLanguages()

	if len(rtl) < 4 {
		t.Errorf("RTLLanguages() returned %d, want at least 4", len(rtl))
	}

	// Verify all returned languages are actually RTL
	for _, code := range rtl {
		lang, ok := Languages[code]
		if !ok {
			t.Errorf("Invalid code in RTLLanguages: %q", code)
			continue
		}
		if !lang.RTL {
			t.Errorf("Language %q is not RTL", code)
		}
	}

	// Verify specific RTL languages are included
	expected := map[string]bool{"ar": true, "he": true, "fa": true, "ur": true}
	found := make(map[string]bool)
	for _, code := range rtl {
		found[code] = true
	}

	for code := range expected {
		if !found[code] {
			t.Errorf("RTLLanguages() missing expected language: %q", code)
		}
	}
}

func TestLanguagesConsistency(t *testing.T) {
	// Ensure all languages have consistent data
	for code, lang := range Languages {
		if lang.Code != code {
			t.Errorf("Language %q has mismatched Code field: %q", code, lang.Code)
		}
		if lang.Name == "" {
			t.Errorf("Language %q has empty Name", code)
		}
		if lang.NativeName == "" {
			t.Errorf("Language %q has empty NativeName", code)
		}
		if len(lang.PluralCategories) == 0 {
			t.Errorf("Language %q has no CLDR plural categories", code)
		}
	}
}

func TestLanguageRegistryMatchesEmbeddedLocalesAndDefaults(t *testing.T) {
	entries, err := fs.ReadDir(LocalesFS(), "locales")
	if err != nil {
		t.Fatalf("read embedded locales: %v", err)
	}
	localeCodes := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		code := strings.TrimSuffix(entry.Name(), ".json")
		localeCodes[code] = true
		if _, ok := Languages[code]; !ok {
			t.Errorf("embedded locale %q has no registry entry", code)
		}
	}
	for code := range Languages {
		if !localeCodes[code] {
			t.Errorf("registry language %q has no embedded locale", code)
		}
	}
	defaults := DefaultSupportedLanguages()
	if len(defaults) != len(localeCodes) {
		t.Fatalf("runtime defaults have %d languages; embedded locales have %d", len(defaults), len(localeCodes))
	}
	for _, code := range defaults {
		if !localeCodes[code] {
			t.Errorf("runtime default language %q has no embedded locale", code)
		}
	}
	for code, language := range Languages {
		if language.Required && !containsCode(defaults, code) {
			t.Errorf("runtime defaults omit required minimum language %q", code)
		}
	}
}

func TestValidateSupportedLanguages(t *testing.T) {
	minimum := []string{"en", "es", "fr", "ar", "de", "it"}
	tests := []struct {
		name      string
		languages []string
		wantErr   bool
	}{
		{name: "spec minimum", languages: minimum},
		{name: "all registered defaults", languages: DefaultSupportedLanguages()},
		{name: "missing required language", languages: []string{"en", "es", "fr", "ar", "de"}, wantErr: true},
		{name: "unsupported locale", languages: append(append([]string{}, minimum...), "ko"), wantErr: true},
		{name: "duplicate locale", languages: append(append([]string{}, minimum...), "en"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSupportedLanguages(tt.languages)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateSupportedLanguages() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func containsCode(codes []string, target string) bool {
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
}
