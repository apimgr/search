package prefsync

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

var testLanguages = []string{"en", "fr", "de"}

func TestImportDirectValuesWinAndRemainAdditive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/server/preferences/import?code=theme%3Dlight%26lang%3Dfr%26search_pref_units%3Dmetric&theme=dark&cookie_consent=no&search_owner_token=secret&search_pref_video_previews=0", nil)
	req.AddCookie(&http.Cookie{Name: "search_pref_category", Value: "news"})
	result := Import(req, "search", "en", testLanguages)
	if result.State.Theme != "dark" || result.State.Lang != "fr" || result.State.Prefs["units"] != "metric" || result.State.Prefs["category"] != "news" {
		t.Fatalf("unexpected merged state: %#v", result.State)
	}
	if !contains(result.Applied, "theme") || !contains(result.Applied, "lang") || !contains(result.Applied, "search_pref_units") {
		t.Fatalf("applied=%v", result.Applied)
	}
	if !contains(result.Ignored, "cookie_consent") || !contains(result.Ignored, "search_owner_token") || !contains(result.Ignored, "search_pref_video_previews") {
		t.Fatalf("ignored=%v", result.Ignored)
	}
	w := httptest.NewRecorder()
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-Proto", "https")
	Apply(w, req, "search", result)
	if len(w.Result().Cookies()) != 3 {
		t.Fatalf("set cookies=%v", w.Result().Cookies())
	}
	for _, cookie := range w.Result().Cookies() {
		if !cookie.Secure || cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != CookieMaxAge {
			t.Errorf("cookie attributes: %#v", cookie)
		}
	}
	if !strings.Contains(strings.Join(w.Header().Values("Set-Cookie"), ";"), "search_pref_units") {
		t.Fatal("app preference cookie not set")
	}
}

func TestImportPaddedCodeAndUnsupportedRegionFallback(t *testing.T) {
	encoded := base64.URLEncoding.EncodeToString([]byte("lang=fr-CA&search_pref_safe_search=2"))
	req := httptest.NewRequest(http.MethodGet, "/server/preferences/import?code="+url.QueryEscape(encoded), nil)
	result := Import(req, "search", "en", testLanguages)
	if result.State.Lang != "fr" || result.State.Prefs["safe_search"] != "2" {
		t.Fatalf("state=%#v", result.State)
	}
}

func TestImportInvalidAndOversizedValuesAreIgnored(t *testing.T) {
	oversized := strings.Repeat("x", 2049)
	req := httptest.NewRequest(http.MethodGet, "/server/preferences/import?theme=neon&lang=zz&search_pref_units=stones&search_pref_private=secret&code="+oversized, nil)
	result := Import(req, "search", "en", testLanguages)
	if result.State.Theme != "dark" || result.State.Lang != "en" || len(result.State.Prefs) != 0 {
		t.Fatalf("state=%#v", result.State)
	}
	for _, name := range []string{"theme", "lang", "search_pref_units", "search_pref_private", "code"} {
		if !contains(result.Ignored, name) {
			t.Errorf("ignored list missing %s: %v", name, result.Ignored)
		}
	}
}

func TestExportCanonicalIncludesOnlyPreferences(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://search.example/server/preferences/export", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	req.AddCookie(&http.Cookie{Name: "lang", Value: "de"})
	req.AddCookie(&http.Cookie{Name: "search_pref_units", Value: "metric"})
	req.AddCookie(&http.Cookie{Name: "search_pref_save_history", Value: "0"})
	req.AddCookie(&http.Cookie{Name: "search_pref_font_size", Value: "large"})
	req.AddCookie(&http.Cookie{Name: "search_pref_results_density", Value: "compact"})
	req.AddCookie(&http.Cookie{Name: "search_pref_accent_color", Value: "#a1b2c3"})
	req.AddCookie(&http.Cookie{Name: "search_pref_video_previews", Value: "0"})
	req.AddCookie(&http.Cookie{Name: "cookie_consent", Value: "secret"})
	state, fullURL, code := Export(req, "search", "en", testLanguages)
	if state.Theme != "light" || state.Lang != "de" || state.Prefs["units"] != "metric" || state.Prefs["save_history"] != "0" || state.Prefs["font_size"] != "large" || state.Prefs["results_density"] != "compact" || state.Prefs["accent_color"] != "#a1b2c3" {
		t.Fatalf("state=%#v", state)
	}
	parsed, err := url.Parse(fullURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("search_pref_units") != "metric" || parsed.Query().Get("search_pref_save_history") != "0" || parsed.Query().Get("search_pref_font_size") != "large" || parsed.Query().Get("search_pref_results_density") != "compact" || parsed.Query().Get("search_pref_accent_color") != "#a1b2c3" || parsed.Query().Get("search_pref_video_previews") != "" || parsed.Query().Get("cookie_consent") != "" {
		t.Fatalf("query=%s", parsed.RawQuery)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(code)
	if err != nil || string(decoded) != parsed.RawQuery {
		t.Fatalf("code decode=%q err=%v query=%q", decoded, err, parsed.RawQuery)
	}
}

func TestImportValidatesVisualPreferences(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/server/preferences/import?search_pref_font_size=large&search_pref_results_density=spacious&search_pref_accent_color=%23ABCDEF", nil)
	result := Import(req, "search", "en", testLanguages)
	if result.State.Prefs["font_size"] != "large" || result.State.Prefs["results_density"] != "spacious" || result.State.Prefs["accent_color"] != "#ABCDEF" {
		t.Fatalf("valid values not imported: %#v", result.State.Prefs)
	}
	invalid := httptest.NewRequest(http.MethodGet, "/server/preferences/import?search_pref_font_size=huge&search_pref_results_density=tight&search_pref_accent_color=red", nil)
	result = Import(invalid, "search", "en", testLanguages)
	if len(result.State.Prefs) != 0 || !contains(result.Ignored, "search_pref_font_size") || !contains(result.Ignored, "search_pref_results_density") || !contains(result.Ignored, "search_pref_accent_color") {
		t.Fatalf("invalid visual values were not rejected: %#v", result)
	}
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
