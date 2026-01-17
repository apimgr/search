package i18n

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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
