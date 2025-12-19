package users

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
