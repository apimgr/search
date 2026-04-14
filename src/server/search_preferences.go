package server

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/apimgr/search/src/model"
)

type searchPreferences struct {
	Theme             string
	DefaultCategory   model.Category
	SafeSearch        int
	ResultsPerPage    int
	NewTab            bool
	InfiniteScroll    bool
	KeyboardShortcuts bool
}

func parseSearchPreferences(raw string) searchPreferences {
	prefs := searchPreferences{
		Theme:             "",
		DefaultCategory:   model.CategoryGeneral,
		SafeSearch:        1,
		ResultsPerPage:    20,
		NewTab:            false,
		InfiniteScroll:    false,
		KeyboardShortcuts: true,
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return prefs
	}

	if decoded, err := base64.RawURLEncoding.DecodeString(raw); err == nil && len(decoded) > 0 {
		raw = strings.TrimSpace(string(decoded))
	}

	if strings.HasPrefix(raw, "{") {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			if theme, ok := payload["theme"].(string); ok {
				prefs.Theme = normalizeThemePreference(theme)
			}
			if category, ok := payload["default_category"].(string); ok {
				prefs.DefaultCategory = model.ParseCategory(category)
			}
			if safeSearch, ok := payload["safe_search"].(float64); ok {
				prefs.SafeSearch = normalizeSafeSearch(int(safeSearch))
			}
			if resultsPerPage, ok := payload["results_per_page"].(float64); ok {
				prefs.ResultsPerPage = normalizeResultsPerPage(int(resultsPerPage))
			}
			if newTab, ok := payload["new_tab"].(bool); ok {
				prefs.NewTab = newTab
			}
			if infiniteScroll, ok := payload["infinite_scroll"].(bool); ok {
				prefs.InfiniteScroll = infiniteScroll
			}
			if keyboardShortcuts, ok := payload["keyboard_shortcuts"].(bool); ok {
				prefs.KeyboardShortcuts = keyboardShortcuts
			}
			return prefs
		}
	}

	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		key, value, found := strings.Cut(part, "=")
		if !found {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		switch key {
		case "t":
			prefs.Theme = normalizeThemePreference(value)
		case "c":
			prefs.DefaultCategory = model.ParseCategory(value)
		case "s":
			prefs.SafeSearch = normalizeSafeSearchAlias(value)
		case "r":
			if parsed, err := strconv.Atoi(value); err == nil {
				prefs.ResultsPerPage = normalizeResultsPerPage(parsed)
			}
		case "n":
			prefs.NewTab = value == "1"
		case "p":
			prefs.InfiniteScroll = strings.EqualFold(value, "i")
		case "k":
			prefs.KeyboardShortcuts = value != "0"
		}
	}

	return prefs
}

func normalizeThemePreference(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "d", "dark":
		return ThemeDark
	case "l", "light":
		return ThemeLight
	case "a", "auto", "system":
		return ThemeAuto
	default:
		return ""
	}
}

func normalizeSafeSearchAlias(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "o", "off", "0":
		return 0
	case "m", "moderate", "1":
		return 1
	case "s", "strict", "2":
		return 2
	default:
		return 1
	}
}

func normalizeSafeSearch(value int) int {
	if value < 0 || value > 2 {
		return 1
	}
	return value
}

func normalizeResultsPerPage(value int) int {
	if value < 1 {
		return 20
	}
	if value > 100 {
		return 100
	}
	return value
}
