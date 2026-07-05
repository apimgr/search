package server

import (
	"net/http"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
)

// handleLocale serves embedded locale JSON for WebUI JavaScript.
func (s *Server) handleLocale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	langFile := strings.TrimPrefix(r.URL.Path, "/locales/")
	if langFile == "" || strings.Contains(langFile, "/") || !strings.HasSuffix(langFile, ".json") {
		http.NotFound(w, r)
		return
	}

	lang := strings.TrimSuffix(langFile, ".json")
	lang = s.getI18nManager().ResolveSupportedLanguage(lang)

	body, err := i18n.LocalesFS().ReadFile("locales/" + lang + ".json")
	if err != nil {
		body, err = i18n.LocalesFS().ReadFile("locales/en.json")
		if err != nil {
			http.NotFound(w, r)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	_, _ = w.Write(body)
}
