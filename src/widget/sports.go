package widget

import (
	"context"
	"net/http"
	"time"

	"github.com/apimgr/search/src/config"
)

// SportsFetcher fetches sports scores
type SportsFetcher struct {
	client *http.Client
	config *config.SportsWidgetConfig
}

// SportsData represents sports widget data
type SportsData struct {
	Games []GameData `json:"games"`
}

// GameData represents data for a single game
type GameData struct {
	League    string `json:"league"`
	HomeTeam  string `json:"home_team"`
	AwayTeam  string `json:"away_team"`
	HomeScore int    `json:"home_score"`
	AwayScore int    `json:"away_score"`
	Status    string `json:"status"` // "scheduled", "live", "final"
	StartTime string `json:"start_time,omitempty"`
}

// NewSportsFetcher creates a new sports fetcher
func NewSportsFetcher(cfg *config.SportsWidgetConfig) *SportsFetcher {
	return &SportsFetcher{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		config: cfg,
	}
}

// WidgetType returns the widget type
func (f *SportsFetcher) WidgetType() WidgetType {
	return WidgetSports
}

// CacheDuration returns how long to cache the data
func (f *SportsFetcher) CacheDuration() time.Duration {
	return 5 * time.Minute
}

// Fetch fetches sports scores
// Note: This is a placeholder implementation. To get real sports data,
// you would need to integrate with a sports API like:
// - ESPN API
// - TheSportsDB (free tier)
// - API-Football
// - SportRadar
func (f *SportsFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	// Sports API integration would go here
	// For now, return empty data since most free sports APIs require registration

	return &WidgetData{
		Type:      WidgetSports,
		Data:      &SportsData{Games: []GameData{}},
		UpdatedAt: time.Now(),
	}, nil
}
