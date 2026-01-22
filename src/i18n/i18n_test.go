package i18n

import (
	"embed"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

//go:embed testdata/*.json
var testdataFS embed.FS

func TestNewManager(t *testing.T) {
	supported := []string{"en", "de", "fr"}
	m := NewManager("en", supported)

	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
	if m.defaultLang != "en" {
		t.Errorf("defaultLang = %q, want %q", m.defaultLang, "en")
	}
	if len(m.supported) != 3 {
		t.Errorf("supported length = %d, want %d", len(m.supported), 3)
	}
	if m.translations == nil {
		t.Error("translations map should not be nil")
	}
	if m.rtlLangs == nil {
		t.Error("rtlLangs map should not be nil")
	}
}

func TestManagerRTLLangs(t *testing.T) {
	m := NewManager("en", []string{"en", "ar", "he", "fa", "ur"})

	rtlLangs := []string{"ar", "he", "fa", "ur"}
	for _, lang := range rtlLangs {
		if !m.rtlLangs[lang] {
			t.Errorf("rtlLangs[%q] should be true", lang)
		}
	}

	if m.rtlLangs["en"] {
		t.Error("rtlLangs[en] should be false")
	}
}

func TestManagerLoadFromMap(t *testing.T) {
	m := NewManager("en", []string{"en"})

	translations := map[string]string{
		"hello":   "Hello",
		"goodbye": "Goodbye",
	}

	m.LoadFromMap("en", translations)

	if m.translations["en"] == nil {
		t.Fatal("translations[en] should not be nil")
	}
	if m.translations["en"]["hello"] != "Hello" {
		t.Errorf("translations[en][hello] = %q, want %q", m.translations["en"]["hello"], "Hello")
	}
}

func TestManagerT(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	m.LoadFromMap("en", map[string]string{
		"hello":   "Hello",
		"welcome": "Welcome, %s!",
	})
	m.LoadFromMap("de", map[string]string{
		"hello":   "Hallo",
		"welcome": "Willkommen, %s!",
	})

	tests := []struct {
		lang string
		key  string
		args []interface{}
		want string
	}{
		{"en", "hello", nil, "Hello"},
		{"de", "hello", nil, "Hallo"},
		{"en", "welcome", []interface{}{"User"}, "Welcome, User!"},
		{"de", "welcome", []interface{}{"Benutzer"}, "Willkommen, Benutzer!"},
		{"en", "missing", nil, "missing"},               // Missing key returns key
		{"xx", "hello", nil, "Hello"},                   // Unknown lang falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.lang+":"+tt.key, func(t *testing.T) {
			var got string
			if len(tt.args) > 0 {
				got = m.T(tt.lang, tt.key, tt.args...)
			} else {
				got = m.T(tt.lang, tt.key)
			}
			if got != tt.want {
				t.Errorf("T(%q, %q) = %q, want %q", tt.lang, tt.key, got, tt.want)
			}
		})
	}
}

func TestManagerTMissingInBothLangs(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	m.LoadFromMap("en", map[string]string{})
	m.LoadFromMap("de", map[string]string{})

	result := m.T("de", "nonexistent")
	if result != "nonexistent" {
		t.Errorf("T() for missing key should return key, got %q", result)
	}
}

func TestManagerDetectLanguage(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	tests := []struct {
		name           string
		cookie         string
		acceptLanguage string
		want           string
	}{
		{"cookie en", "en", "", "en"},
		{"cookie de", "de", "", "de"},
		{"cookie invalid", "xx", "de,en;q=0.9", "de"},
		{"header only", "", "de,en;q=0.9", "de"},
		{"header with quality", "", "en;q=0.5,de;q=0.9", "de"},
		{"no preference", "", "", "en"},
		{"unsupported in header", "", "xx,yy", "en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "lang", Value: tt.cookie})
			}
			if tt.acceptLanguage != "" {
				req.Header.Set("Accept-Language", tt.acceptLanguage)
			}

			got := m.DetectLanguage(req)
			if got != tt.want {
				t.Errorf("DetectLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestManagerParseAcceptLanguage(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	tests := []struct {
		header string
		want   string
	}{
		{"en-US,en;q=0.9,de;q=0.8", "en"},
		{"de-DE,de;q=0.9,en;q=0.8", "de"},
		{"fr;q=1.0,de;q=0.5", "fr"},
		{"xx,yy,zz", ""},
		{"", ""},
		{"en-GB", "en"}, // Regional variant normalized
		{"de-AT,de;q=0.9", "de"},
	}

	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := m.parseAcceptLanguage(tt.header)
			if got != tt.want {
				t.Errorf("parseAcceptLanguage(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestManagerIsSupported(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	tests := []struct {
		lang string
		want bool
	}{
		{"en", true},
		{"de", true},
		{"fr", true},
		{"es", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := m.IsSupported(tt.lang)
			if got != tt.want {
				t.Errorf("IsSupported(%q) = %v, want %v", tt.lang, got, tt.want)
			}
		})
	}
}

func TestManagerIsRTL(t *testing.T) {
	m := NewManager("en", []string{"en", "ar", "he"})

	tests := []struct {
		lang string
		want bool
	}{
		{"ar", true},
		{"he", true},
		{"fa", true},
		{"ur", true},
		{"en", false},
		{"de", false},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := m.IsRTL(tt.lang)
			if got != tt.want {
				t.Errorf("IsRTL(%q) = %v, want %v", tt.lang, got, tt.want)
			}
		})
	}
}

func TestManagerSupportedLanguages(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})

	langs := m.SupportedLanguages()
	if len(langs) != 2 {
		t.Errorf("SupportedLanguages() returned %d languages, want 2", len(langs))
	}

	// Check that returned languages have correct data
	for _, lang := range langs {
		if lang.Code != "en" && lang.Code != "de" {
			t.Errorf("Unexpected language code: %q", lang.Code)
		}
	}
}

func TestManagerDefaultLanguage(t *testing.T) {
	m := NewManager("de", []string{"en", "de"})

	if m.DefaultLanguage() != "de" {
		t.Errorf("DefaultLanguage() = %q, want %q", m.DefaultLanguage(), "de")
	}
}

func TestManagerNewTranslator(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	m.LoadFromMap("en", map[string]string{"hello": "Hello"})
	m.LoadFromMap("de", map[string]string{"hello": "Hallo"})

	// Supported language
	tr := m.NewTranslator("de")
	if tr.lang != "de" {
		t.Errorf("Translator lang = %q, want %q", tr.lang, "de")
	}

	// Unsupported language falls back to default
	tr2 := m.NewTranslator("xx")
	if tr2.lang != "en" {
		t.Errorf("Translator lang = %q, want %q (default)", tr2.lang, "en")
	}
}

func TestTranslatorT(t *testing.T) {
	m := NewManager("en", []string{"en"})
	m.LoadFromMap("en", map[string]string{
		"hello":   "Hello",
		"welcome": "Welcome, %s!",
	})

	tr := m.NewTranslator("en")

	if tr.T("hello") != "Hello" {
		t.Errorf("T(hello) = %q, want %q", tr.T("hello"), "Hello")
	}
	if tr.T("welcome", "User") != "Welcome, User!" {
		t.Errorf("T(welcome, User) = %q, want %q", tr.T("welcome", "User"), "Welcome, User!")
	}
}

func TestTranslatorLang(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	tr := m.NewTranslator("de")

	if tr.Lang() != "de" {
		t.Errorf("Lang() = %q, want %q", tr.Lang(), "de")
	}
}

func TestTranslatorIsRTL(t *testing.T) {
	m := NewManager("en", []string{"en", "ar"})

	tr := m.NewTranslator("en")
	if tr.IsRTL() {
		t.Error("IsRTL() should be false for English")
	}

	tr2 := m.NewTranslator("ar")
	if !tr2.IsRTL() {
		t.Error("IsRTL() should be true for Arabic")
	}
}

func TestTranslatorDir(t *testing.T) {
	m := NewManager("en", []string{"en", "ar"})

	tr := m.NewTranslator("en")
	if tr.Dir() != "ltr" {
		t.Errorf("Dir() = %q, want %q", tr.Dir(), "ltr")
	}

	tr2 := m.NewTranslator("ar")
	if tr2.Dir() != "rtl" {
		t.Errorf("Dir() = %q, want %q", tr2.Dir(), "rtl")
	}
}

func TestTranslatorLanguages(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	tr := m.NewTranslator("en")

	langs := tr.Languages()
	if len(langs) != 2 {
		t.Errorf("Languages() returned %d, want 2", len(langs))
	}
}

func TestManagerTemplateFuncs(t *testing.T) {
	m := NewManager("en", []string{"en"})
	m.LoadFromMap("en", map[string]string{"hello": "Hello"})

	funcs := m.TemplateFuncs("en")

	if funcs["t"] == nil {
		t.Error("TemplateFuncs should include 't' function")
	}
	if funcs["lang"] == nil {
		t.Error("TemplateFuncs should include 'lang' function")
	}
	if funcs["isRTL"] == nil {
		t.Error("TemplateFuncs should include 'isRTL' function")
	}
	if funcs["dir"] == nil {
		t.Error("TemplateFuncs should include 'dir' function")
	}
	if funcs["languages"] == nil {
		t.Error("TemplateFuncs should include 'languages' function")
	}

	// Test t function
	tFunc := funcs["t"].(func(string, ...interface{}) string)
	if tFunc("hello") != "Hello" {
		t.Error("t function should translate")
	}

	// Test lang function
	langFunc := funcs["lang"].(func() string)
	if langFunc() != "en" {
		t.Error("lang function should return language code")
	}

	// Test isRTL function
	isRTLFunc := funcs["isRTL"].(func() bool)
	if isRTLFunc() {
		t.Error("isRTL should be false for English")
	}

	// Test dir function
	dirFunc := funcs["dir"].(func() string)
	if dirFunc() != "ltr" {
		t.Error("dir should be 'ltr' for English")
	}
}

func TestSetLanguageCookie(t *testing.T) {
	w := httptest.NewRecorder()
	SetLanguageCookie(w, "de")

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != "lang" {
		t.Errorf("Cookie name = %q, want %q", cookie.Name, "lang")
	}
	if cookie.Value != "de" {
		t.Errorf("Cookie value = %q, want %q", cookie.Value, "de")
	}
	if cookie.Path != "/" {
		t.Errorf("Cookie path = %q, want %q", cookie.Path, "/")
	}
	if !cookie.HttpOnly {
		t.Error("Cookie should be HttpOnly")
	}
	if cookie.MaxAge != 365*24*60*60 {
		t.Errorf("Cookie MaxAge = %d, want %d", cookie.MaxAge, 365*24*60*60)
	}
}

func TestFlattenTranslations(t *testing.T) {
	tests := []struct {
		name   string
		input  interface{}
		want   map[string]string
	}{
		{
			name: "flat structure",
			input: map[string]interface{}{
				"hello":   "Hello",
				"goodbye": "Goodbye",
			},
			want: map[string]string{
				"hello":   "Hello",
				"goodbye": "Goodbye",
			},
		},
		{
			name: "nested structure",
			input: map[string]interface{}{
				"auth": map[string]interface{}{
					"login":  "Log In",
					"logout": "Log Out",
				},
			},
			want: map[string]string{
				"auth.login":  "Log In",
				"auth.logout": "Log Out",
			},
		},
		{
			name: "deeply nested",
			input: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"key": "value",
					},
				},
			},
			want: map[string]string{
				"level1.level2.key": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]string)
			flattenTranslations("", tt.input, result)

			for key, wantVal := range tt.want {
				if result[key] != wantVal {
					t.Errorf("result[%q] = %q, want %q", key, result[key], wantVal)
				}
			}
		})
	}
}

// TestFlattenTranslationsEdgeCases tests edge cases for flattenTranslations
func TestFlattenTranslationsEdgeCases(t *testing.T) {
	t.Run("non-string non-map values are ignored", func(t *testing.T) {
		result := make(map[string]string)
		input := map[string]interface{}{
			"number":  42,
			"boolean": true,
			"array":   []string{"a", "b"},
			"nil":     nil,
			"float":   3.14,
			"valid":   "valid value",
		}
		flattenTranslations("", input, result)

		// Only the string value should be in result
		if len(result) != 1 {
			t.Errorf("result should have 1 entry, got %d", len(result))
		}
		if result["valid"] != "valid value" {
			t.Errorf("result[valid] = %q, want %q", result["valid"], "valid value")
		}
	})

	t.Run("string at root level with empty prefix is ignored", func(t *testing.T) {
		result := make(map[string]string)
		flattenTranslations("", "root string", result)
		if len(result) != 0 {
			t.Errorf("root string with empty prefix should be ignored, got %d entries", len(result))
		}
	})

	t.Run("string with prefix is included", func(t *testing.T) {
		result := make(map[string]string)
		flattenTranslations("key", "value", result)
		if result["key"] != "value" {
			t.Errorf("result[key] = %q, want %q", result["key"], "value")
		}
	})

	t.Run("mixed nested with non-string values", func(t *testing.T) {
		result := make(map[string]string)
		input := map[string]interface{}{
			"section": map[string]interface{}{
				"text":   "hello",
				"number": 123,
				"nested": map[string]interface{}{
					"deep": "value",
					"num":  456,
				},
			},
		}
		flattenTranslations("", input, result)

		if result["section.text"] != "hello" {
			t.Errorf("result[section.text] = %q, want %q", result["section.text"], "hello")
		}
		if result["section.nested.deep"] != "value" {
			t.Errorf("result[section.nested.deep] = %q, want %q", result["section.nested.deep"], "value")
		}
		if len(result) != 2 {
			t.Errorf("result should have 2 entries, got %d", len(result))
		}
	})
}

// TestLoadFromFSMissingLanguageFileIsSkipped tests that missing language files are skipped
func TestLoadFromFSMissingLanguageFileIsSkipped(t *testing.T) {
	fs := LocalesFS()
	// Include a language that doesn't have a file
	m := NewManager("en", []string{"en", "nonexistent_lang"})

	err := m.LoadFromFS(fs, "locales")
	if err != nil {
		t.Errorf("LoadFromFS should not fail for missing non-default languages: %v", err)
	}

	// English should still work
	if m.T("en", "common.save") != "Save" {
		t.Error("English should still be loaded")
	}

	// Nonexistent language should fall back to default
	if m.T("nonexistent_lang", "common.save") != "Save" {
		t.Error("Nonexistent language should fall back to default")
	}
}

// TestLoadFromFSInvalidJSON tests LoadFromFS with invalid JSON
func TestLoadFromFSInvalidJSON(t *testing.T) {
	// testdataFS contains an invalid en.json file
	m := NewManager("en", []string{"en"})

	err := m.LoadFromFS(testdataFS, "testdata")
	if err == nil {
		t.Error("LoadFromFS should return error for invalid JSON")
	}

	if !strings.Contains(err.Error(), "failed to parse") {
		t.Errorf("error should mention 'failed to parse', got: %v", err)
	}
}

// TestParseAcceptLanguageEdgeCases tests edge cases in parseAcceptLanguage
func TestParseAcceptLanguageEdgeCases(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"empty parts in header", "en,,de", "en"},
		{"spaces around parts", "  de  ,  en  ", "de"},
		{"quality without q= prefix", "en;0.5,de;q=0.9", "de"},
		{"multiple semicolons", "en;q=0.5;extra,de;q=0.9", "de"},
		{"uppercase language", "DE,EN", "de"},
		{"mixed case with region", "De-DE,En-US;q=0.8", "de"},
		{"only whitespace", "   ", ""},
		{"quality value 0", "en;q=0,de;q=0.5", "de"},
		{"same quality different order", "de;q=0.9,fr;q=0.9", "de"}, // First one wins in stable sort
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.parseAcceptLanguage(tt.header)
			if got != tt.want {
				t.Errorf("parseAcceptLanguage(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

// TestDetectLanguageWithUppercaseCookie tests cookie value normalization
func TestDetectLanguageWithUppercaseCookie(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "lang", Value: "DE"})

	got := m.DetectLanguage(req)
	if got != "de" {
		t.Errorf("DetectLanguage() = %q, want %q", got, "de")
	}
}

// TestDetectLanguageInvalidCookieFallbackToHeader tests fallback from invalid cookie to header
func TestDetectLanguageInvalidCookieFallbackToHeader(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "lang", Value: "invalid"})
	req.Header.Set("Accept-Language", "fr")

	got := m.DetectLanguage(req)
	if got != "fr" {
		t.Errorf("DetectLanguage() = %q, want %q", got, "fr")
	}
}

// TestDetectLanguageNoCookieNoHeaderFallbackToDefault tests fallback to default
func TestDetectLanguageNoCookieNoHeaderFallbackToDefault(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	req := httptest.NewRequest("GET", "/", nil)

	got := m.DetectLanguage(req)
	if got != "en" {
		t.Errorf("DetectLanguage() = %q, want %q", got, "en")
	}
}

// TestDetectLanguageEmptyHeader tests empty Accept-Language header
func TestDetectLanguageEmptyHeader(t *testing.T) {
	m := NewManager("en", []string{"en", "de", "fr"})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "")

	got := m.DetectLanguage(req)
	if got != "en" {
		t.Errorf("DetectLanguage() with empty header = %q, want %q", got, "en")
	}
}

// TestTemplateFuncsLanguages tests the languages function in TemplateFuncs
func TestTemplateFuncsLanguages(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})

	funcs := m.TemplateFuncs("en")

	// Test languages function
	languagesFunc := funcs["languages"].(func() []Language)
	langs := languagesFunc()
	if len(langs) != 2 {
		t.Errorf("languages() returned %d, want 2", len(langs))
	}

	// Verify the returned languages have correct data
	found := make(map[string]bool)
	for _, lang := range langs {
		found[lang.Code] = true
	}
	if !found["en"] || !found["de"] {
		t.Error("languages() should return en and de")
	}
}

// TestTemplateFuncsWithRTL tests template functions with RTL language
func TestTemplateFuncsWithRTL(t *testing.T) {
	m := NewManager("en", []string{"en", "ar"})
	m.LoadFromMap("ar", map[string]string{"hello": "مرحبا"})

	funcs := m.TemplateFuncs("ar")

	// Test t function
	tFunc := funcs["t"].(func(string, ...interface{}) string)
	if tFunc("hello") != "مرحبا" {
		t.Error("t function should translate for Arabic")
	}

	// Test lang function
	langFunc := funcs["lang"].(func() string)
	if langFunc() != "ar" {
		t.Error("lang function should return 'ar'")
	}

	// Test isRTL function
	isRTLFunc := funcs["isRTL"].(func() bool)
	if !isRTLFunc() {
		t.Error("isRTL should be true for Arabic")
	}

	// Test dir function
	dirFunc := funcs["dir"].(func() string)
	if dirFunc() != "rtl" {
		t.Error("dir should be 'rtl' for Arabic")
	}
}

// TestTemplateFuncsTWithArgs tests the t template function with format arguments
func TestTemplateFuncsTWithArgs(t *testing.T) {
	m := NewManager("en", []string{"en"})
	m.LoadFromMap("en", map[string]string{
		"welcome": "Welcome, %s!",
		"count":   "You have %d items",
	})

	funcs := m.TemplateFuncs("en")
	tFunc := funcs["t"].(func(string, ...interface{}) string)

	// Test with string argument
	result := tFunc("welcome", "John")
	if result != "Welcome, John!" {
		t.Errorf("t(welcome, John) = %q, want %q", result, "Welcome, John!")
	}

	// Test with int argument
	result = tFunc("count", 5)
	if result != "You have 5 items" {
		t.Errorf("t(count, 5) = %q, want %q", result, "You have 5 items")
	}
}

// TestSupportedLanguagesWithUnknownCode tests SupportedLanguages when a code is not in Languages map
func TestSupportedLanguagesWithUnknownCode(t *testing.T) {
	// Create manager with a code that doesn't exist in Languages map
	m := NewManager("en", []string{"en", "xx", "de"})

	langs := m.SupportedLanguages()
	// Should only return en and de since xx doesn't exist in Languages map
	if len(langs) != 2 {
		t.Errorf("SupportedLanguages() returned %d languages, want 2", len(langs))
	}

	for _, lang := range langs {
		if lang.Code != "en" && lang.Code != "de" {
			t.Errorf("Unexpected language code: %q", lang.Code)
		}
	}
}

// TestTWithFormatArgsInFallback tests T with format args falling back to default
func TestTWithFormatArgsInFallback(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	m.LoadFromMap("en", map[string]string{
		"welcome": "Welcome, %s!",
	})
	// de translations don't have "welcome" key

	// Should fall back to English and use format args
	got := m.T("de", "welcome", "User")
	if got != "Welcome, User!" {
		t.Errorf("T with fallback = %q, want %q", got, "Welcome, User!")
	}
}

// TestTWithNoTranslationsLoaded tests T when no translations are loaded
func TestTWithNoTranslationsLoaded(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	// Don't load any translations

	got := m.T("en", "hello")
	if got != "hello" {
		t.Errorf("T with no translations = %q, want %q", got, "hello")
	}

	got = m.T("de", "hello")
	if got != "hello" {
		t.Errorf("T with no translations = %q, want %q", got, "hello")
	}
}

// TestTFallbackWithoutArgs tests T fallback to default language without args
func TestTFallbackWithoutArgs(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	m.LoadFromMap("en", map[string]string{
		"hello": "Hello",
	})
	// de translations are empty

	// Should fall back to English without args
	got := m.T("de", "hello")
	if got != "Hello" {
		t.Errorf("T fallback without args = %q, want %q", got, "Hello")
	}
}

// TestTKeyNotInRequestedButInDefault tests key exists in default but not requested language
func TestTKeyNotInRequestedButInDefault(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	m.LoadFromMap("en", map[string]string{
		"hello":   "Hello",
		"goodbye": "Goodbye",
	})
	m.LoadFromMap("de", map[string]string{
		"hello": "Hallo",
		// "goodbye" is missing in German
	})

	// "goodbye" should fall back to English
	got := m.T("de", "goodbye")
	if got != "Goodbye" {
		t.Errorf("T for missing key in requested lang = %q, want %q", got, "Goodbye")
	}
}

// TestConcurrentAccess tests thread safety of the Manager
func TestConcurrentAccess(t *testing.T) {
	m := NewManager("en", []string{"en", "de"})
	m.LoadFromMap("en", map[string]string{"hello": "Hello"})
	m.LoadFromMap("de", map[string]string{"hello": "Hallo"})

	done := make(chan bool)

	// Concurrent reads
	for range 10 {
		go func() {
			for range 100 {
				_ = m.T("en", "hello")
				_ = m.T("de", "hello")
				_ = m.IsSupported("en")
				_ = m.IsRTL("ar")
				_ = m.SupportedLanguages()
			}
			done <- true
		}()
	}

	// Concurrent writes
	for range 5 {
		go func() {
			for range 50 {
				m.LoadFromMap("en", map[string]string{"hello": "Hello", "test": "Test"})
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for range 15 {
		<-done
	}
}

// TestDefaultManager tests the DefaultManager function from embed.go
func TestDefaultManager(t *testing.T) {
	manager, err := DefaultManager()
	if err != nil {
		t.Fatalf("DefaultManager() error = %v", err)
	}

	if manager == nil {
		t.Fatal("DefaultManager() returned nil")
	}

	// Verify it loaded English translations
	result := manager.T("en", "common.save")
	if result != "Save" {
		t.Errorf("T(en, common.save) = %q, want %q", result, "Save")
	}

	// Verify nested translations work
	result = manager.T("en", "auth.login")
	if result != "Log In" {
		t.Errorf("T(en, auth.login) = %q, want %q", result, "Log In")
	}
}

// TestLocalesFS tests the LocalesFS function from embed.go
func TestLocalesFS(t *testing.T) {
	fs := LocalesFS()

	// Verify we can read a file from the embedded FS
	data, err := fs.ReadFile("locales/en.json")
	if err != nil {
		t.Fatalf("Failed to read locales/en.json: %v", err)
	}

	if len(data) == 0 {
		t.Error("locales/en.json should not be empty")
	}
}

// TestLoadFromFSWithRealEmbedFS tests LoadFromFS with the actual embedded filesystem
func TestLoadFromFSWithRealEmbedFS(t *testing.T) {
	fs := LocalesFS()
	m := NewManager("en", []string{"en", "de", "fr"})

	err := m.LoadFromFS(fs, "locales")
	if err != nil {
		t.Fatalf("LoadFromFS() error = %v", err)
	}

	// Verify English loaded
	if m.T("en", "common.save") != "Save" {
		t.Error("English translations not loaded correctly")
	}

	// Verify German loaded
	result := m.T("de", "common.save")
	if result == "common.save" {
		t.Error("German translations not loaded")
	}
}

// TestLoadFromFSMissingDefaultLanguage tests LoadFromFS when default language file is missing
func TestLoadFromFSMissingDefaultLanguage(t *testing.T) {
	fs := LocalesFS()
	// Use a default language that doesn't exist
	m := NewManager("xx", []string{"xx", "yy"})

	err := m.LoadFromFS(fs, "locales")
	if err == nil {
		t.Error("LoadFromFS should return error when default language is missing")
	}

	expectedErr := "default language xx not found"
	if err.Error() != expectedErr {
		t.Errorf("error = %q, want %q", err.Error(), expectedErr)
	}
}
