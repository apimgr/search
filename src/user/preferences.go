package user

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// PreferencesManager handles user search preferences
type PreferencesManager struct {
	db         *sql.DB
	cookieName string
}

// UserPreferences represents user search preferences
type UserPreferences struct {
	// Display settings
	Theme        string `json:"theme"`         // dark, light, auto
	ResultsPerPage int  `json:"results_per_page"`
	OpenInNewTab bool   `json:"open_in_new_tab"`

	// Search defaults
	DefaultCategory string   `json:"default_category"`
	DefaultLanguage string   `json:"default_language"`
	DefaultRegion   string   `json:"default_region"`
	SafeSearch      int      `json:"safe_search"` // 0, 1, 2
	DefaultSort     string   `json:"default_sort"` // relevance, date, popularity

	// Engine preferences
	EnabledEngines  []string `json:"enabled_engines,omitempty"`
	DisabledEngines []string `json:"disabled_engines,omitempty"`

	// UI preferences
	ShowThumbnails  bool `json:"show_thumbnails"`
	ShowEngineIcons bool `json:"show_engine_icons"`
	InfiniteScroll  bool `json:"infinite_scroll"`
	AutocompleteOn  bool `json:"autocomplete_on"`

	// Privacy
	SaveSearchHistory bool `json:"save_search_history"`
	AnonymizeResults  bool `json:"anonymize_results"` // Use proxy for images

	// Accessibility
	HighContrast bool `json:"high_contrast"`
	LargeFont    bool `json:"large_font"`
	ReduceMotion bool `json:"reduce_motion"`
}

// DefaultPreferences returns default user preferences
func DefaultPreferences() *UserPreferences {
	return &UserPreferences{
		Theme:           "auto",
		ResultsPerPage:  20,
		OpenInNewTab:    false,
		DefaultCategory: "general",
		DefaultLanguage: "en",
		DefaultRegion:   "",
		SafeSearch:      1, // Moderate
		DefaultSort:     "relevance",
		ShowThumbnails:  true,
		ShowEngineIcons: true,
		InfiniteScroll:  false,
		AutocompleteOn:  true,
		SaveSearchHistory: false,
		AnonymizeResults: true,
		HighContrast:    false,
		LargeFont:       false,
		ReduceMotion:    false,
	}
}

// Preferences errors
var (
	ErrPreferencesNotFound = errors.New("preferences not found")
)

// NewPreferencesManager creates a new preferences manager
func NewPreferencesManager(db *sql.DB) *PreferencesManager {
	return &PreferencesManager{
		db:         db,
		cookieName: "search_prefs",
	}
}

// GetForUser retrieves preferences for a logged-in user
func (pm *PreferencesManager) GetForUser(ctx context.Context, userID int64) (*UserPreferences, error) {
	var prefsJSON string
	err := pm.db.QueryRowContext(ctx, `
		SELECT preferences FROM user_preferences WHERE user_id = ?
	`, userID).Scan(&prefsJSON)

	if err == sql.ErrNoRows {
		return DefaultPreferences(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get preferences: %w", err)
	}

	var prefs UserPreferences
	if err := json.Unmarshal([]byte(prefsJSON), &prefs); err != nil {
		return DefaultPreferences(), nil
	}

	return &prefs, nil
}

// SaveForUser saves preferences for a logged-in user
func (pm *PreferencesManager) SaveForUser(ctx context.Context, userID int64, prefs *UserPreferences) error {
	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("failed to serialize preferences: %w", err)
	}

	_, err = pm.db.ExecContext(ctx, `
		INSERT INTO user_preferences (user_id, preferences, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET preferences = ?, updated_at = ?
	`, userID, string(prefsJSON), time.Now(), string(prefsJSON), time.Now())

	if err != nil {
		return fmt.Errorf("failed to save preferences: %w", err)
	}

	return nil
}

// GetFromCookie retrieves preferences from a cookie (for anonymous users)
func (pm *PreferencesManager) GetFromCookie(r *http.Request) *UserPreferences {
	cookie, err := r.Cookie(pm.cookieName)
	if err != nil {
		return DefaultPreferences()
	}

	var prefs UserPreferences
	if err := json.Unmarshal([]byte(cookie.Value), &prefs); err != nil {
		return DefaultPreferences()
	}

	return &prefs
}

// SetCookie sets preferences in a cookie (for anonymous users)
func (pm *PreferencesManager) SetCookie(w http.ResponseWriter, prefs *UserPreferences) error {
	prefsJSON, err := json.Marshal(prefs)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     pm.cookieName,
		Value:    string(prefsJSON),
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60, // 1 year
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// ClearCookie clears the preferences cookie
func (pm *PreferencesManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     pm.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// Merge merges new preferences with existing ones (only non-zero values)
func (prefs *UserPreferences) Merge(updates *UserPreferences) {
	if updates.Theme != "" {
		prefs.Theme = updates.Theme
	}
	if updates.ResultsPerPage > 0 {
		prefs.ResultsPerPage = updates.ResultsPerPage
	}
	if updates.DefaultCategory != "" {
		prefs.DefaultCategory = updates.DefaultCategory
	}
	if updates.DefaultLanguage != "" {
		prefs.DefaultLanguage = updates.DefaultLanguage
	}
	if updates.DefaultRegion != "" {
		prefs.DefaultRegion = updates.DefaultRegion
	}
	if updates.DefaultSort != "" {
		prefs.DefaultSort = updates.DefaultSort
	}
	if len(updates.EnabledEngines) > 0 {
		prefs.EnabledEngines = updates.EnabledEngines
	}
	if len(updates.DisabledEngines) > 0 {
		prefs.DisabledEngines = updates.DisabledEngines
	}

	// Boolean fields need explicit handling
	prefs.OpenInNewTab = updates.OpenInNewTab
	prefs.ShowThumbnails = updates.ShowThumbnails
	prefs.ShowEngineIcons = updates.ShowEngineIcons
	prefs.InfiniteScroll = updates.InfiniteScroll
	prefs.AutocompleteOn = updates.AutocompleteOn
	prefs.SaveSearchHistory = updates.SaveSearchHistory
	prefs.AnonymizeResults = updates.AnonymizeResults
	prefs.HighContrast = updates.HighContrast
	prefs.LargeFont = updates.LargeFont
	prefs.ReduceMotion = updates.ReduceMotion
	prefs.SafeSearch = updates.SafeSearch
}

// Validate validates preference values
func (prefs *UserPreferences) Validate() error {
	// Theme
	validThemes := map[string]bool{"dark": true, "light": true, "auto": true}
	if !validThemes[prefs.Theme] {
		prefs.Theme = "auto"
	}

	// Results per page
	if prefs.ResultsPerPage < 10 {
		prefs.ResultsPerPage = 10
	}
	if prefs.ResultsPerPage > 100 {
		prefs.ResultsPerPage = 100
	}

	// Safe search
	if prefs.SafeSearch < 0 || prefs.SafeSearch > 2 {
		prefs.SafeSearch = 1
	}

	// Default sort
	validSorts := map[string]bool{"relevance": true, "date": true, "popularity": true}
	if !validSorts[prefs.DefaultSort] {
		prefs.DefaultSort = "relevance"
	}

	// Default category
	validCategories := map[string]bool{
		"general": true, "images": true, "videos": true, "news": true,
		"maps": true, "files": true, "it": true, "science": true, "social": true,
	}
	if !validCategories[prefs.DefaultCategory] {
		prefs.DefaultCategory = "general"
	}

	return nil
}

// ToJSON converts preferences to JSON
func (prefs *UserPreferences) ToJSON() ([]byte, error) {
	return json.Marshal(prefs)
}

// FromJSON parses preferences from JSON
func FromJSON(data []byte) (*UserPreferences, error) {
	var prefs UserPreferences
	if err := json.Unmarshal(data, &prefs); err != nil {
		return nil, err
	}
	return &prefs, nil
}

// Preference String Encoding
// Compact URL-safe format: t=d;l=en;s=1;r=20;e=g,b,d
// Keys:
//   t = theme (d=dark, l=light, a=auto)
//   l = language (ISO code)
//   g = region (ISO code)
//   s = safe search (0, 1, 2)
//   r = results per page
//   c = category
//   o = sort (r=relevance, d=date, p=popularity)
//   e = enabled engines (comma-separated codes)
//   x = disabled engines (comma-separated codes)
//   n = new tab (1=true)
//   h = thumbnails (1=true)
//   i = engine icons (1=true)
//   f = infinite scroll (1=true)
//   a = autocomplete (1=true)
//   y = save history (1=true)
//   p = anonymize/proxy (1=true)
//   hc = high contrast (1=true)
//   lf = large font (1=true)
//   rm = reduce motion (1=true)

// Engine short codes for preference string
var engineCodes = map[string]string{
	"google":        "g",
	"bing":          "b",
	"duckduckgo":    "d",
	"brave":         "br",
	"startpage":     "sp",
	"qwant":         "q",
	"yahoo":         "y",
	"wikipedia":     "w",
	"reddit":        "r",
	"stackoverflow": "so",
	"github":        "gh",
	"youtube":       "yt",
	"mojeek":        "m",
	"yandex":        "ya",
	"baidu":         "ba",
}

// Reverse mapping for decoding
var engineNames = map[string]string{
	"g":  "google",
	"b":  "bing",
	"d":  "duckduckgo",
	"br": "brave",
	"sp": "startpage",
	"q":  "qwant",
	"y":  "yahoo",
	"w":  "wikipedia",
	"r":  "reddit",
	"so": "stackoverflow",
	"gh": "github",
	"yt": "youtube",
	"m":  "mojeek",
	"ya": "yandex",
	"ba": "baidu",
}

// ToPreferenceString encodes preferences to a compact URL-safe string
func (prefs *UserPreferences) ToPreferenceString() string {
	var parts []string

	// Theme
	switch prefs.Theme {
	case "dark":
		parts = append(parts, "t=d")
	case "light":
		parts = append(parts, "t=l")
	case "auto":
		parts = append(parts, "t=a")
	}

	// Language (only if set)
	if prefs.DefaultLanguage != "" && prefs.DefaultLanguage != "en" {
		parts = append(parts, "l="+prefs.DefaultLanguage)
	}

	// Region (only if set)
	if prefs.DefaultRegion != "" {
		parts = append(parts, "g="+prefs.DefaultRegion)
	}

	// Safe search (only if not default)
	if prefs.SafeSearch != 1 {
		parts = append(parts, fmt.Sprintf("s=%d", prefs.SafeSearch))
	}

	// Results per page (only if not default)
	if prefs.ResultsPerPage != 20 {
		parts = append(parts, fmt.Sprintf("r=%d", prefs.ResultsPerPage))
	}

	// Category (only if not default)
	if prefs.DefaultCategory != "" && prefs.DefaultCategory != "general" {
		parts = append(parts, "c="+prefs.DefaultCategory)
	}

	// Sort
	switch prefs.DefaultSort {
	case "date":
		parts = append(parts, "o=d")
	case "popularity":
		parts = append(parts, "o=p")
	// relevance is default, don't encode
	}

	// Enabled engines
	if len(prefs.EnabledEngines) > 0 {
		var codes []string
		for _, eng := range prefs.EnabledEngines {
			if code, ok := engineCodes[eng]; ok {
				codes = append(codes, code)
			}
		}
		if len(codes) > 0 {
			parts = append(parts, "e="+strings.Join(codes, ","))
		}
	}

	// Disabled engines
	if len(prefs.DisabledEngines) > 0 {
		var codes []string
		for _, eng := range prefs.DisabledEngines {
			if code, ok := engineCodes[eng]; ok {
				codes = append(codes, code)
			}
		}
		if len(codes) > 0 {
			parts = append(parts, "x="+strings.Join(codes, ","))
		}
	}

	// Boolean flags (only encode if true, except defaults)
	if prefs.OpenInNewTab {
		parts = append(parts, "n=1")
	}
	if !prefs.ShowThumbnails {
		parts = append(parts, "h=0")
	}
	if !prefs.ShowEngineIcons {
		parts = append(parts, "i=0")
	}
	if prefs.InfiniteScroll {
		parts = append(parts, "f=1")
	}
	if !prefs.AutocompleteOn {
		parts = append(parts, "a=0")
	}
	if prefs.SaveSearchHistory {
		parts = append(parts, "y=1")
	}
	if !prefs.AnonymizeResults {
		parts = append(parts, "p=0")
	}
	if prefs.HighContrast {
		parts = append(parts, "hc=1")
	}
	if prefs.LargeFont {
		parts = append(parts, "lf=1")
	}
	if prefs.ReduceMotion {
		parts = append(parts, "rm=1")
	}

	return strings.Join(parts, ";")
}

// ParsePreferenceString decodes a preference string into UserPreferences
func ParsePreferenceString(s string) *UserPreferences {
	prefs := DefaultPreferences()
	if s == "" {
		return prefs
	}

	// Parse key=value pairs separated by semicolons
	for _, part := range strings.Split(s, ";") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, value := kv[0], kv[1]

		switch key {
		case "t":
			switch value {
			case "d":
				prefs.Theme = "dark"
			case "l":
				prefs.Theme = "light"
			case "a":
				prefs.Theme = "auto"
			}
		case "l":
			prefs.DefaultLanguage = value
		case "g":
			prefs.DefaultRegion = value
		case "s":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 && v <= 2 {
				prefs.SafeSearch = v
			}
		case "r":
			if v, err := strconv.Atoi(value); err == nil && v >= 10 && v <= 100 {
				prefs.ResultsPerPage = v
			}
		case "c":
			prefs.DefaultCategory = value
		case "o":
			switch value {
			case "r":
				prefs.DefaultSort = "relevance"
			case "d":
				prefs.DefaultSort = "date"
			case "p":
				prefs.DefaultSort = "popularity"
			}
		case "e":
			prefs.EnabledEngines = parseEngineCodes(value)
		case "x":
			prefs.DisabledEngines = parseEngineCodes(value)
		case "n":
			prefs.OpenInNewTab = value == "1"
		case "h":
			prefs.ShowThumbnails = value != "0"
		case "i":
			prefs.ShowEngineIcons = value != "0"
		case "f":
			prefs.InfiniteScroll = value == "1"
		case "a":
			prefs.AutocompleteOn = value != "0"
		case "y":
			prefs.SaveSearchHistory = value == "1"
		case "p":
			prefs.AnonymizeResults = value != "0"
		case "hc":
			prefs.HighContrast = value == "1"
		case "lf":
			prefs.LargeFont = value == "1"
		case "rm":
			prefs.ReduceMotion = value == "1"
		}
	}

	return prefs
}

// parseEngineCodes converts comma-separated engine codes to full names
func parseEngineCodes(codes string) []string {
	var engines []string
	for _, code := range strings.Split(codes, ",") {
		code = strings.TrimSpace(code)
		if name, ok := engineNames[code]; ok {
			engines = append(engines, name)
		}
	}
	return engines
}

// GetShareableURL generates a shareable URL with the preference string
func (prefs *UserPreferences) GetShareableURL(baseURL string) string {
	prefStr := prefs.ToPreferenceString()
	if prefStr == "" {
		return baseURL + "/preferences"
	}
	return baseURL + "/preferences?p=" + prefStr
}
