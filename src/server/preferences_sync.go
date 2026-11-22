package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/prefsync"
)

func (s *Server) handlePreferencesImport(w http.ResponseWriter, r *http.Request) {
	mgr := s.getI18nManager()
	result := prefsync.Import(r, "search", mgr.DefaultLanguage(), mgr.SupportedLanguageCodes())
	if CanSetPreferenceCookie(r) {
		prefsync.Apply(w, r, "search", result)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", preferenceRedirect(r))
	w.WriteHeader(http.StatusSeeOther)
}

func (s *Server) handleLegacyPreferencesSync(w http.ResponseWriter, r *http.Request) {
	target := "/server/preferences/sync"
	if strings.HasSuffix(r.URL.Path, "/import") {
		target += "/import"
	}
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}

func (s *Server) handlePreferencesSync(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		clone := r.Clone(r.Context())
		urlCopy := *r.URL
		urlCopy.RawQuery = r.Form.Encode()
		clone.URL = &urlCopy
		mgr := s.getI18nManager()
		result := prefsync.Import(clone, "search", mgr.DetectLanguage(r), mgr.SupportedLanguageCodes())
		if CanSetPreferenceCookie(r) {
			prefsync.Apply(w, r, "search", result)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Location", preferenceRedirect(r))
		w.WriteHeader(http.StatusSeeOther)
		return
	}
	if r.URL.Path != "/server/preferences/sync" {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "text/plain") {
		mgr := s.getI18nManager()
		_, fullURL, code := prefsync.Export(r, "search", mgr.DetectLanguage(r), mgr.SupportedLanguageCodes())
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("URL: " + fullURL + "\nCode: " + code + "\n"))
		return
	}
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		mgr := s.getI18nManager()
		state, fullURL, code := prefsync.Export(r, "search", mgr.DetectLanguage(r), mgr.SupportedLanguageCodes())
		w.Header().Set("Cache-Control", "no-store")
		respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": map[string]interface{}{"theme": state.Theme, "lang": state.Lang, "prefs": state.Prefs, "url": fullURL, "code": code}})
		return
	}
	mgr := s.getI18nManager()
	_, fullURL, code := prefsync.Export(r, "search", mgr.DetectLanguage(r), mgr.SupportedLanguageCodes())
	w.Header().Set("Cache-Control", "no-store")
	s.renderPreferencesSync(w, r, map[string]interface{}{"syncURL": fullURL, "syncCode": code})
}

func preferenceRedirect(r *http.Request) string {
	target := r.URL.Query().Get("redirect")
	if target == "" {
		target = r.FormValue("redirect")
	}
	if validPreferenceRedirect(target) && !preferenceRedirectLoop(target) {
		return target
	}
	if referer := r.Referer(); referer != "" {
		candidate, err := url.Parse(referer)
		current, currentErr := url.Parse(httputil.BuildURL(r, "/"))
		if err == nil && currentErr == nil && candidate.IsAbs() && samePreferenceOrigin(candidate, current) {
			target := candidate.RequestURI()
			if validPreferenceRedirect(target) && !preferenceRedirectLoop(target) {
				return target
			}
		}
	}
	if r.URL.Path == "/server/preferences/sync" {
		return "/server/preferences/sync"
	}
	return "/"
}

func validPreferenceRedirect(target string) bool {
	if !strings.HasPrefix(target, "/") || strings.HasPrefix(target, "//") || strings.HasPrefix(target, "/\\") || strings.ContainsAny(target, "\\\r\n") {
		return false
	}
	parsed, err := url.Parse(target)
	return err == nil && parsed.Scheme == "" && parsed.Host == "" && parsed.User == nil
}

func preferenceRedirectLoop(target string) bool {
	path := strings.SplitN(target, "?", 2)[0]
	return strings.HasPrefix(path, "/server/preferences/sync/import") || path == "/server/preferences" || path == "/server/preferences/sync"
}

func samePreferenceOrigin(a, b *url.URL) bool {
	return a != nil && b != nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
