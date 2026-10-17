package server

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/prefsync"
)

func (s *Server) handlePreferencesExport(w http.ResponseWriter, r *http.Request) {
	mgr := s.getI18nManager()
	state, fullURL, code := prefsync.Export(r, "search", mgr.DefaultLanguage(), mgr.SupportedLanguageCodes())
	w.Header().Set("Cache-Control", "no-store")
	data := map[string]interface{}{"theme": state.Theme, "lang": state.Lang, "prefs": state.Prefs, "url": fullURL, "code": code}
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/plain") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("URL: " + fullURL + "\nCode: " + code + "\n"))
		return
	}
	if strings.Contains(accept, "application/json") {
		respondJSON(w, http.StatusOK, map[string]interface{}{"ok": true, "data": data})
		return
	}
	page := s.newPageData(w, r, "Preferences", "preferences")
	page.Prefs = resolveGeneralPrefs(r)
	page.Data = map[string]interface{}{"bangs": s.bangManager.GetAll(), "categories": s.bangManager.GetCategories(), "builtins": s.bangManager.GetBuiltins(), "syncURL": fullURL, "syncCode": code}
	if err := s.renderer.Render(w, "preferences", page); err != nil {
		s.handleInternalError(w, r, "preferences export render", err)
	}
}

func (s *Server) handlePreferencesImport(w http.ResponseWriter, r *http.Request) {
	mgr := s.getI18nManager()
	result := prefsync.Import(r, "search", mgr.DefaultLanguage(), mgr.SupportedLanguageCodes())
	prefsync.Apply(w, r, "search", result)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Location", preferenceRedirect(r))
	w.WriteHeader(http.StatusSeeOther)
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
	return strings.HasPrefix(path, "/server/preferences/import") || strings.HasPrefix(path, "/server/preferences/export") || path == "/server/preferences"
}

func samePreferenceOrigin(a, b *url.URL) bool {
	return a != nil && b != nil && strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}
