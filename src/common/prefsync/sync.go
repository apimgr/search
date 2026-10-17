package prefsync

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/apimgr/search/src/common/httputil"
)

const CookieMaxAge = 31536000

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

var preferenceEnums = map[string]map[string]bool{
	"category":           {"general": true, "images": true, "videos": true, "news": true, "maps": true, "files": true, "music": true, "science": true, "it": true, "social": true},
	"safe_search":        {"0": true, "1": true, "2": true},
	"results_per_page":   {"10": true, "20": true, "30": true, "50": true, "100": true},
	"units":              {"metric": true, "imperial": true},
	"new_tab":            {"0": true, "1": true},
	"infinite_scroll":    {"0": true, "1": true},
	"keyboard_shortcuts": {"0": true, "1": true},
	"save_history":       {"0": true, "1": true},
}

type State struct {
	Theme string            `json:"theme"`
	Lang  string            `json:"lang"`
	Prefs map[string]string `json:"prefs"`
}

type ImportResult struct {
	State   State    `json:"-"`
	Applied []string `json:"applied"`
	Ignored []string `json:"ignored"`
}

func Current(r *http.Request, projectName, defaultLang string, supported []string) State {
	state := State{Theme: "dark", Lang: canonicalLanguage(defaultLang, supported), Prefs: map[string]string{}}
	if cookie, err := r.Cookie("theme"); err == nil && validTheme(cookie.Value) {
		state.Theme = cookie.Value
	}
	if cookie, err := r.Cookie("lang"); err == nil {
		if value := canonicalLanguage(cookie.Value, supported); value != "" {
			state.Lang = value
		}
	}
	prefix := projectName + "_pref_"
	for _, cookie := range r.Cookies() {
		if !strings.HasPrefix(cookie.Name, prefix) {
			continue
		}
		key := strings.TrimPrefix(cookie.Name, prefix)
		if keyPattern.MatchString(key) && validPref(key, cookie.Value) {
			state.Prefs[key] = cookie.Value
		}
	}
	return state
}

func Export(r *http.Request, projectName, defaultLang string, supported []string) (State, string, string) {
	state := Current(r, projectName, defaultLang, supported)
	values := url.Values{"theme": {state.Theme}, "lang": {state.Lang}}
	for key, value := range state.Prefs {
		values.Set(projectName+"_pref_"+key, value)
	}
	query := values.Encode()
	code := base64.RawURLEncoding.EncodeToString([]byte(query))
	base, err := url.Parse(httputil.BuildURL(r, "/server/preferences/import"))
	if err != nil {
		return state, "", code
	}
	base.RawQuery = query
	return state, base.String(), code
}

func Import(r *http.Request, projectName, defaultLang string, supported []string) ImportResult {
	state := Current(r, projectName, defaultLang, supported)
	query := r.URL.Query()
	decoded := url.Values{}
	code := strings.TrimSpace(query.Get("code"))
	codeTooLong := len(code) > 2048
	if len(code) <= 2048 && code != "" {
		if index := strings.Index(code, "?"); index >= 0 {
			code = code[index+1:]
		}
		var text string
		if strings.Contains(code, "=") {
			trimmed := strings.TrimRight(code, "=")
			if len(trimmed) > 0 && len(code)-len(trimmed) <= 2 && !strings.ContainsAny(trimmed, "&%") {
				if data, err := base64.URLEncoding.DecodeString(code); err == nil {
					text = string(data)
				}
			}
			if text == "" {
				text = code
			}
		} else {
			if data, err := base64.RawURLEncoding.DecodeString(code); err == nil {
				text = string(data)
			} else if data, err := base64.URLEncoding.DecodeString(code); err == nil {
				text = string(data)
			}
		}
		if text != "" {
			if values, err := url.ParseQuery(text); err == nil {
				decoded = values
			}
		}
	}
	keys := map[string]bool{}
	for key := range decoded {
		keys[key] = true
	}
	for key := range query {
		if key != "code" {
			keys[key] = true
		}
	}
	allKeys := make([]string, 0, len(keys))
	for key := range keys {
		allKeys = append(allKeys, key)
	}
	sort.Strings(allKeys)
	result := ImportResult{State: state, Applied: []string{}, Ignored: []string{}}
	if codeTooLong {
		result.Ignored = append(result.Ignored, "code")
	}
	for i, name := range allKeys {
		if i >= 32 {
			result.Ignored = append(result.Ignored, name)
			continue
		}
		if name == "code" {
			continue
		}
		values, direct := query[name]
		if !direct {
			values = decoded[name]
		}
		if len(values) == 0 {
			result.Ignored = append(result.Ignored, name)
			continue
		}
		value := values[0]
		if len(value) > 64 {
			result.Ignored = append(result.Ignored, name)
			continue
		}
		valid := false
		switch name {
		case "theme":
			valid = validTheme(value)
			if valid {
				result.State.Theme = value
			}
		case "lang":
			canonical := canonicalLanguage(value, supported)
			valid = canonical != ""
			if valid {
				result.State.Lang = canonical
			}
		default:
			prefix := projectName + "_pref_"
			if strings.HasPrefix(name, prefix) {
				key := strings.TrimPrefix(name, prefix)
				valid = keyPattern.MatchString(key) && validPref(key, value)
				if valid {
					result.State.Prefs[key] = value
				}
			}
		}
		if valid {
			result.Applied = append(result.Applied, name)
		} else {
			result.Ignored = append(result.Ignored, name)
		}
	}
	sort.Strings(result.Applied)
	sort.Strings(result.Ignored)
	return result
}

func Apply(w http.ResponseWriter, r *http.Request, projectName string, result ImportResult) {
	secure := strings.HasPrefix(strings.ToLower(httputil.BuildURL(r, "/")), "https://")
	set := func(name, value string) {
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: CookieMaxAge, SameSite: http.SameSiteLaxMode, Secure: secure})
	}
	for _, name := range result.Applied {
		switch name {
		case "theme":
			set("theme", result.State.Theme)
		case "lang":
			set("lang", result.State.Lang)
		default:
			prefix := projectName + "_pref_"
			if strings.HasPrefix(name, prefix) {
				set(name, result.State.Prefs[strings.TrimPrefix(name, prefix)])
			}
		}
	}
}

func validTheme(value string) bool { return value == "dark" || value == "light" || value == "auto" }

func validPref(key, value string) bool {
	options, ok := preferenceEnums[key]
	return ok && options[value]
}

func canonicalLanguage(value string, supported []string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, candidate := range supported {
		if strings.EqualFold(candidate, value) {
			return candidate
		}
	}
	if index := strings.IndexAny(value, "-_"); index >= 0 {
		base := value[:index]
		for _, candidate := range supported {
			if strings.EqualFold(candidate, base) {
				return candidate
			}
		}
	}
	return ""
}
