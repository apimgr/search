package server

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
)

// normalizeSyncLanguage lowercases and strips any region/script subtag
// (e.g. "fr-CA" -> "fr") to match the base language codes tracked by
// i18n.Manager.SupportedLanguageCodes(). Mirrors the private
// normalizeLanguageCode helper in src/common/i18n since that package does
// not export it.
func normalizeSyncLanguage(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if idx := strings.IndexAny(value, "-_"); idx != -1 {
		value = value[:idx]
	}
	return value
}

// currentSyncPrefs reads the theme/lang cookies for the exportable preference
// state, falling back to defaults per AI.md "Client-Side Preferences".
func (s *Server) currentSyncPrefs(r *http.Request) (theme, lang string) {
	theme = DefaultTheme
	if c, err := r.Cookie("theme"); err == nil && IsValidTheme(c.Value) {
		theme = c.Value
	}

	mgr := s.getI18nManager()
	lang = mgr.DefaultLanguage()
	if c, err := r.Cookie("lang"); err == nil {
		if l := normalizeSyncLanguage(c.Value); l != "" && mgr.IsSupported(l) {
			lang = l
		}
	}

	return theme, lang
}

// handlePreferencesExport handles GET /server/preferences/export. Per AI.md
// "Cross-device preference sync": reads the current theme/lang cookies and
// returns the full import URL plus a base64url short code for the same
// state. Stateless - nothing is written or looked up server-side.
func (s *Server) handlePreferencesExport(w http.ResponseWriter, r *http.Request) {
	theme, lang := s.currentSyncPrefs(r)

	query := url.Values{"theme": {theme}, "lang": {lang}}.Encode()

	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
		scheme = "http"
	}
	fullURL := scheme + "://" + r.Host + "/server/preferences/import?" + query
	shortCode := base64.RawURLEncoding.EncodeToString([]byte(query))

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"ok": true,
		"data": map[string]string{
			"theme":      theme,
			"lang":       lang,
			"full_url":   fullURL,
			"short_code": shortCode,
		},
	})
}

// handlePreferencesImport handles GET /server/preferences/import. Per AI.md
// "Cross-device preference sync": accepts theme/lang as a plain query
// string, or a pasted base64url short code (in the "code" param, optionally
// still carrying a "https://.../import?" prefix which is stripped). Each
// value is validated against its normal enum/BCP-47 allowlist - unknown or
// malformed values are dropped rather than erroring. Valid values are
// written as cookies, then it redirects with 303 so the code never lingers
// in the visible URL or browser history.
func (s *Server) handlePreferencesImport(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	// A pasted short code takes precedence when present; it may still carry
	// a "https://host/server/preferences/import?" prefix if the user copied
	// the full URL into the code field instead of the code alone.
	if code := query.Get("code"); code != "" {
		if idx := strings.Index(code, "?"); idx != -1 {
			code = code[idx+1:]
		}
		if decoded, err := base64.RawURLEncoding.DecodeString(code); err == nil {
			// The stripped text decoded as base64 - treat it as the short code form.
			if decodedQuery, err := url.ParseQuery(string(decoded)); err == nil {
				query = decodedQuery
			}
		} else if decodedQuery, err := url.ParseQuery(code); err == nil {
			// Not valid base64 (e.g. the user pasted the plain "theme=dark&lang=fr"
			// query string rather than the short code) - use it directly.
			query = decodedQuery
		}
	}

	if theme := query.Get("theme"); theme != "" && IsValidTheme(theme) {
		SetTheme(w, theme)
	}

	if lang := normalizeSyncLanguage(query.Get("lang")); lang != "" && s.getI18nManager().IsSupported(lang) {
		i18n.SetLanguageCookie(w, lang)
	}

	redirectTo := "/"
	if p := r.FormValue("redirect"); strings.HasPrefix(p, "/") && !strings.HasPrefix(p, "//") {
		redirectTo = p
	}

	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}
