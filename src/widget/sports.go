package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
)

// SportsFetcher fetches sports scores from TheSportsDB API
type SportsFetcher struct {
	client     *http.Client
	config     *config.SportsWidgetConfig
	lastStatus GameStatus // Track last fetched game status for cache duration
}

// GameStatus represents the status of a game
type GameStatus string

const (
	GameStatusScheduled GameStatus = "scheduled"
	GameStatusLive      GameStatus = "live"
	GameStatusFinal     GameStatus = "final"
	GameStatusPostponed GameStatus = "postponed"
	GameStatusCanceled  GameStatus = "canceled"
)

// SportsData represents sports widget data
type SportsData struct {
	Games      []GameData `json:"games"`
	League     string     `json:"league,omitempty"`
	Team       string     `json:"team,omitempty"`
	QueryType  string     `json:"query_type"` // "team", "league", or "live"
	HasLive    bool       `json:"has_live"`   // Whether any games are currently live
	LastUpdate string     `json:"last_update"`
}

// GameData represents data for a single game
type GameData struct {
	ID            string     `json:"id"`
	League        string     `json:"league"`
	LeagueID      string     `json:"league_id,omitempty"`
	Season        string     `json:"season,omitempty"`
	HomeTeam      string     `json:"home_team"`
	HomeTeamID    string     `json:"home_team_id,omitempty"`
	HomeTeamBadge string     `json:"home_team_badge,omitempty"`
	AwayTeam      string     `json:"away_team"`
	AwayTeamID    string     `json:"away_team_id,omitempty"`
	AwayTeamBadge string     `json:"away_team_badge,omitempty"`
	HomeScore     *int       `json:"home_score"` // Pointer to distinguish 0 from not played
	AwayScore     *int       `json:"away_score"`
	Status        GameStatus `json:"status"`
	StatusDetail  string     `json:"status_detail,omitempty"` // e.g., "Q3 5:30", "Final", "7:00 PM ET"
	StartTime     string     `json:"start_time,omitempty"`    // ISO 8601 format
	Venue         string     `json:"venue,omitempty"`
	Round         string     `json:"round,omitempty"`
	// Additional stats if available
	HomeStats *TeamStats `json:"home_stats,omitempty"`
	AwayStats *TeamStats `json:"away_stats,omitempty"`
}

// TeamStats represents basic team statistics for a game
type TeamStats struct {
	Shots       int `json:"shots,omitempty"`
	ShotsOnGoal int `json:"shots_on_goal,omitempty"`
	Possession  int `json:"possession,omitempty"` // Percentage
	Corners     int `json:"corners,omitempty"`
	Fouls       int `json:"fouls,omitempty"`
	YellowCards int `json:"yellow_cards,omitempty"`
	RedCards    int `json:"red_cards,omitempty"`
}

// TheSportsDB API response structures
type sportsDBEventsResponse struct {
	Events []sportsDBEvent `json:"events"`
}

type sportsDBEvent struct {
	IDEvent          string  `json:"idEvent"`
	StrEvent         string  `json:"strEvent"`
	StrLeague        string  `json:"strLeague"`
	IDLeague         string  `json:"idLeague"`
	StrSeason        string  `json:"strSeason"`
	StrHomeTeam      string  `json:"strHomeTeam"`
	IDHomeTeam       string  `json:"idHomeTeam"`
	StrAwayTeam      string  `json:"strAwayTeam"`
	IDAwayTeam       string  `json:"idAwayTeam"`
	IntHomeScore     *string `json:"intHomeScore"`
	IntAwayScore     *string `json:"intAwayScore"`
	StrStatus        string  `json:"strStatus"`
	DateEvent        string  `json:"dateEvent"`
	StrTime          string  `json:"strTime"`
	StrTimestamp     string  `json:"strTimestamp"`
	StrVenue         string  `json:"strVenue"`
	IntRound         string  `json:"intRound"`
	StrHomeTeamBadge string  `json:"strHomeTeamBadge"`
	StrAwayTeamBadge string  `json:"strAwayTeamBadge"`
	// Stats
	IntHomeShots *string `json:"intHomeShots"`
	IntAwayShots *string `json:"intAwayShots"`
	StrProgress  string  `json:"strProgress"` // Live match progress
}

type sportsDBTeamsResponse struct {
	Teams []sportsDBTeam `json:"teams"`
}

type sportsDBTeam struct {
	IDTeam       string `json:"idTeam"`
	StrTeam      string `json:"strTeam"`
	StrLeague    string `json:"strLeague"`
	IDLeague     string `json:"idLeague"`
	StrTeamBadge string `json:"strTeamBadge"`
	StrStadium   string `json:"strStadium"`
	StrCountry   string `json:"strCountry"`
}

type sportsDBLeaguesResponse struct {
	Leagues []sportsDBLeague `json:"leagues"`
}

type sportsDBLeague struct {
	IDLeague   string `json:"idLeague"`
	StrLeague  string `json:"strLeague"`
	StrSport   string `json:"strSport"`
	StrCountry string `json:"strCountry"`
	StrBadge   string `json:"strBadge"`
}

type sportsDBLiveScoresResponse struct {
	Events []sportsDBLiveEvent `json:"events"`
}

type sportsDBLiveEvent struct {
	IDEvent      string `json:"idEvent"`
	StrEvent     string `json:"strEvent"`
	StrLeague    string `json:"strLeague"`
	IDLeague     string `json:"idLeague"`
	StrHomeTeam  string `json:"strHomeTeam"`
	IDHomeTeam   string `json:"idHomeTeam"`
	StrAwayTeam  string `json:"strAwayTeam"`
	IDAwayTeam   string `json:"idAwayTeam"`
	IntHomeScore string `json:"intHomeScore"`
	IntAwayScore string `json:"intAwayScore"`
	StrProgress  string `json:"strProgress"`
	StrStatus    string `json:"strStatus"`
	StrVenue     string `json:"strVenue"`
}

// League name mappings for common queries
var leagueNameMappings = map[string]string{
	// American Football
	"nfl":              "NFL",
	"football":         "NFL",
	"ncaa":             "NCAA Football",
	"college football": "NCAA Football",

	// Basketball
	"nba":             "NBA",
	"basketball":      "NBA",
	"wnba":            "WNBA",
	"ncaa basketball": "NCAA Basketball",

	// Baseball
	"mlb":      "MLB",
	"baseball": "MLB",

	// Hockey
	"nhl":    "NHL",
	"hockey": "NHL",

	// Soccer/Football
	"premier league":   "English Premier League",
	"epl":              "English Premier League",
	"english premier":  "English Premier League",
	"la liga":          "Spanish La Liga",
	"serie a":          "Italian Serie A",
	"bundesliga":       "German Bundesliga",
	"ligue 1":          "French Ligue 1",
	"mls":              "MLS",
	"champions league": "UEFA Champions League",
	"ucl":              "UEFA Champions League",
	"world cup":        "FIFA World Cup",
	"soccer":           "English Premier League",

	// Other
	"ufc":         "UFC",
	"mma":         "UFC",
	"f1":          "Formula 1",
	"formula 1":   "Formula 1",
	"formula one": "Formula 1",
	"tennis":      "ATP",
	"golf":        "PGA",
	"pga":         "PGA",
}

// Team name aliases for common queries
var teamAliases = map[string]string{
	// NFL teams
	"lakers":   "Los Angeles Lakers",
	"celtics":  "Boston Celtics",
	"warriors": "Golden State Warriors",
	"bulls":    "Chicago Bulls",
	"heat":     "Miami Heat",
	"knicks":   "New York Knicks",
	"nets":     "Brooklyn Nets",
	"sixers":   "Philadelphia 76ers",
	"76ers":    "Philadelphia 76ers",

	// NFL teams
	"patriots": "New England Patriots",
	"cowboys":  "Dallas Cowboys",
	"packers":  "Green Bay Packers",
	"chiefs":   "Kansas City Chiefs",
	"eagles":   "Philadelphia Eagles",
	"49ers":    "San Francisco 49ers",
	"niners":   "San Francisco 49ers",
	"giants":   "New York Giants",
	"jets":     "New York Jets",
	"ravens":   "Baltimore Ravens",
	"steelers": "Pittsburgh Steelers",
	"dolphins": "Miami Dolphins",
	"bills":    "Buffalo Bills",
	"bears":    "Chicago Bears",
	"lions":    "Detroit Lions",

	// MLB teams
	"yankees":   "New York Yankees",
	"red sox":   "Boston Red Sox",
	"redsox":    "Boston Red Sox",
	"dodgers":   "Los Angeles Dodgers",
	"cubs":      "Chicago Cubs",
	"mets":      "New York Mets",
	"astros":    "Houston Astros",
	"braves":    "Atlanta Braves",
	"cardinals": "St. Louis Cardinals",

	// NHL teams
	"bruins":      "Boston Bruins",
	"rangers":     "New York Rangers",
	"penguins":    "Pittsburgh Penguins",
	"blackhawks":  "Chicago Blackhawks",
	"canadiens":   "Montreal Canadiens",
	"maple leafs": "Toronto Maple Leafs",
	"leafs":       "Toronto Maple Leafs",
	"oilers":      "Edmonton Oilers",

	// Soccer teams
	"man united":        "Manchester United",
	"man utd":           "Manchester United",
	"manchester united": "Manchester United",
	"man city":          "Manchester City",
	"manchester city":   "Manchester City",
	"liverpool":         "Liverpool",
	"chelsea":           "Chelsea",
	"arsenal":           "Arsenal",
	"tottenham":         "Tottenham Hotspur",
	"spurs":             "Tottenham Hotspur",
	"barcelona":         "FC Barcelona",
	"barca":             "FC Barcelona",
	"real madrid":       "Real Madrid",
	"bayern":            "Bayern Munich",
	"bayern munich":     "Bayern Munich",
	"psg":               "Paris Saint-Germain",
	"juventus":          "Juventus",
	"inter":             "Inter Milan",
	"inter milan":       "Inter Milan",
	"ac milan":          "AC Milan",
	"milan":             "AC Milan",
}

// NewSportsFetcher creates a new sports fetcher
func NewSportsFetcher(cfg *config.SportsWidgetConfig) *SportsFetcher {
	return &SportsFetcher{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		config:     cfg,
		lastStatus: GameStatusFinal, // Default to completed games cache duration
	}
}

// WidgetType returns the widget type
func (f *SportsFetcher) WidgetType() WidgetType {
	return WidgetSports
}

// CacheDuration returns how long to cache the data
// Live games: 1 minute, Completed/Scheduled: 1 hour
func (f *SportsFetcher) CacheDuration() time.Duration {
	switch f.lastStatus {
	case GameStatusLive:
		return 1 * time.Minute
	case GameStatusScheduled:
		return 30 * time.Minute
	default:
		return 1 * time.Hour
	}
}

// Fetch fetches sports scores based on query parameters
// Supported params:
//   - team: Team name (e.g., "lakers", "yankees", "Manchester United")
//   - league: League name (e.g., "nfl", "premier league", "nba")
//   - live: "true" to fetch only live games
func (f *SportsFetcher) Fetch(ctx context.Context, params map[string]string) (*WidgetData, error) {
	team := strings.TrimSpace(params["team"])
	league := strings.TrimSpace(params["league"])
	liveOnly := strings.ToLower(params["live"]) == "true"

	var data *SportsData
	var err error

	// Determine query type and fetch appropriate data
	if liveOnly {
		data, err = f.fetchLiveScores(ctx, league)
	} else if team != "" {
		data, err = f.fetchTeamGames(ctx, team)
	} else if league != "" {
		data, err = f.fetchLeagueGames(ctx, league)
	} else {
		// Default: fetch from configured default leagues or show popular live games
		data, err = f.fetchDefaultGames(ctx)
	}

	if err != nil {
		return &WidgetData{
			Type:      WidgetSports,
			Error:     err.Error(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Update last status based on fetched data for cache duration
	f.updateLastStatus(data)

	return &WidgetData{
		Type:      WidgetSports,
		Data:      data,
		UpdatedAt: time.Now(),
	}, nil
}

// updateLastStatus updates the cache duration hint based on game statuses
func (f *SportsFetcher) updateLastStatus(data *SportsData) {
	if data == nil || len(data.Games) == 0 {
		f.lastStatus = GameStatusFinal
		return
	}

	// If any game is live, use short cache
	for _, game := range data.Games {
		if game.Status == GameStatusLive {
			f.lastStatus = GameStatusLive
			data.HasLive = true
			return
		}
	}

	// If all games are scheduled, use medium cache
	allScheduled := true
	for _, game := range data.Games {
		if game.Status != GameStatusScheduled {
			allScheduled = false
			break
		}
	}
	if allScheduled {
		f.lastStatus = GameStatusScheduled
		return
	}

	f.lastStatus = GameStatusFinal
}

// fetchTeamGames fetches recent and upcoming games for a specific team
func (f *SportsFetcher) fetchTeamGames(ctx context.Context, teamQuery string) (*SportsData, error) {
	// Resolve team alias
	teamName := resolveTeamName(teamQuery)

	// First, search for the team
	team, err := f.searchTeam(ctx, teamName)
	if err != nil {
		return nil, fmt.Errorf("team not found: %s", teamQuery)
	}

	// Fetch last 5 events for this team
	lastEvents, _ := f.fetchTeamLastEvents(ctx, team.IDTeam)

	// Fetch next 5 events for this team
	nextEvents, _ := f.fetchTeamNextEvents(ctx, team.IDTeam)

	// Combine and convert events
	games := make([]GameData, 0, len(lastEvents)+len(nextEvents))

	for _, evt := range lastEvents {
		games = append(games, convertEvent(evt))
	}
	for _, evt := range nextEvents {
		games = append(games, convertEvent(evt))
	}

	return &SportsData{
		Games:      games,
		Team:       team.StrTeam,
		QueryType:  "team",
		LastUpdate: time.Now().Format(time.RFC3339),
	}, nil
}

// fetchLeagueGames fetches recent and upcoming games for a specific league
func (f *SportsFetcher) fetchLeagueGames(ctx context.Context, leagueQuery string) (*SportsData, error) {
	// Resolve league name
	leagueName := resolveLeagueName(leagueQuery)

	// Search for the league
	league, err := f.searchLeague(ctx, leagueName)
	if err != nil {
		return nil, fmt.Errorf("league not found: %s", leagueQuery)
	}

	// Fetch events for this league
	events, err := f.fetchLeagueEvents(ctx, league.IDLeague)
	if err != nil {
		return nil, err
	}

	games := make([]GameData, 0, len(events))
	for _, evt := range events {
		games = append(games, convertEvent(evt))
	}

	return &SportsData{
		Games:      games,
		League:     league.StrLeague,
		QueryType:  "league",
		LastUpdate: time.Now().Format(time.RFC3339),
	}, nil
}

// fetchLiveScores fetches currently live games
func (f *SportsFetcher) fetchLiveScores(ctx context.Context, leagueFilter string) (*SportsData, error) {
	// TheSportsDB v2 endpoint for live scores (requires paid API in production)
	// For free tier, we'll fetch today's events and filter by status

	events, err := f.fetchTodaysEvents(ctx, leagueFilter)
	if err != nil {
		return nil, err
	}

	// Filter to only live games
	games := make([]GameData, 0)
	for _, evt := range events {
		game := convertEvent(evt)
		if game.Status == GameStatusLive {
			games = append(games, game)
		}
	}

	return &SportsData{
		Games:      games,
		QueryType:  "live",
		HasLive:    len(games) > 0,
		LastUpdate: time.Now().Format(time.RFC3339),
	}, nil
}

// fetchDefaultGames fetches games from default configured leagues
func (f *SportsFetcher) fetchDefaultGames(ctx context.Context) (*SportsData, error) {
	defaultLeagues := f.config.DefaultLeagues
	if len(defaultLeagues) == 0 {
		defaultLeagues = []string{"NFL", "NBA", "MLB", "NHL"}
	}

	allGames := make([]GameData, 0)

	for _, leagueName := range defaultLeagues {
		league, err := f.searchLeague(ctx, leagueName)
		if err != nil {
			continue
		}

		events, err := f.fetchLeagueEvents(ctx, league.IDLeague)
		if err != nil {
			continue
		}

		for _, evt := range events {
			allGames = append(allGames, convertEvent(evt))
		}

		// Limit total games
		if len(allGames) >= 15 {
			break
		}
	}

	return &SportsData{
		Games:      allGames,
		QueryType:  "default",
		LastUpdate: time.Now().Format(time.RFC3339),
	}, nil
}

// searchTeam searches for a team by name
func (f *SportsFetcher) searchTeam(ctx context.Context, teamName string) (*sportsDBTeam, error) {
	apiURL := fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/searchteams.php?t=%s",
		url.QueryEscape(teamName))

	var resp sportsDBTeamsResponse
	if err := f.doRequest(ctx, apiURL, &resp); err != nil {
		return nil, err
	}

	if len(resp.Teams) == 0 {
		return nil, fmt.Errorf("no team found")
	}

	return &resp.Teams[0], nil
}

// searchLeague searches for a league by name
func (f *SportsFetcher) searchLeague(ctx context.Context, leagueName string) (*sportsDBLeague, error) {
	apiURL := fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/search_all_leagues.php?s=%s",
		url.QueryEscape(leagueName))

	var resp sportsDBLeaguesResponse
	if err := f.doRequest(ctx, apiURL, &resp); err != nil {
		return nil, err
	}

	if len(resp.Leagues) == 0 {
		// Try alternate search
		apiURL = fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/all_leagues.php")
		if err := f.doRequest(ctx, apiURL, &resp); err != nil {
			return nil, err
		}

		// Find best match
		leagueNameLower := strings.ToLower(leagueName)
		for _, league := range resp.Leagues {
			if strings.Contains(strings.ToLower(league.StrLeague), leagueNameLower) {
				return &league, nil
			}
		}
		return nil, fmt.Errorf("no league found")
	}

	return &resp.Leagues[0], nil
}

// fetchTeamLastEvents fetches recent completed events for a team
func (f *SportsFetcher) fetchTeamLastEvents(ctx context.Context, teamID string) ([]sportsDBEvent, error) {
	apiURL := fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/eventslast.php?id=%s", teamID)

	var resp sportsDBEventsResponse
	if err := f.doRequest(ctx, apiURL, &resp); err != nil {
		return nil, err
	}

	return resp.Events, nil
}

// fetchTeamNextEvents fetches upcoming events for a team
func (f *SportsFetcher) fetchTeamNextEvents(ctx context.Context, teamID string) ([]sportsDBEvent, error) {
	apiURL := fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/eventsnext.php?id=%s", teamID)

	var resp sportsDBEventsResponse
	if err := f.doRequest(ctx, apiURL, &resp); err != nil {
		return nil, err
	}

	return resp.Events, nil
}

// fetchLeagueEvents fetches events for a league (combines past and future)
func (f *SportsFetcher) fetchLeagueEvents(ctx context.Context, leagueID string) ([]sportsDBEvent, error) {
	// Note: TheSportsDB free tier uses league ID for past/next events,
	// season parameter is only used in premium tier endpoints

	// Fetch past events (last round)
	pastURL := fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/eventspastleague.php?id=%s", leagueID)
	var pastResp sportsDBEventsResponse
	_ = f.doRequest(ctx, pastURL, &pastResp)

	// Fetch next events
	nextURL := fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/eventsnextleague.php?id=%s", leagueID)
	var nextResp sportsDBEventsResponse
	_ = f.doRequest(ctx, nextURL, &nextResp)

	// Combine, preferring past events first (most recent results)
	events := make([]sportsDBEvent, 0, len(pastResp.Events)+len(nextResp.Events))
	events = append(events, pastResp.Events...)
	events = append(events, nextResp.Events...)

	// Limit to reasonable number
	if len(events) > 20 {
		events = events[:20]
	}

	return events, nil
}

// fetchTodaysEvents fetches all events happening today
func (f *SportsFetcher) fetchTodaysEvents(ctx context.Context, leagueFilter string) ([]sportsDBEvent, error) {
	today := time.Now().Format("2006-01-02")
	apiURL := fmt.Sprintf("https://www.thesportsdb.com/api/v1/json/3/eventsday.php?d=%s", today)

	if leagueFilter != "" {
		leagueName := resolveLeagueName(leagueFilter)
		apiURL += "&s=" + url.QueryEscape(leagueName)
	}

	var resp sportsDBEventsResponse
	if err := f.doRequest(ctx, apiURL, &resp); err != nil {
		return nil, err
	}

	return resp.Events, nil
}

// doRequest performs an HTTP request and decodes the JSON response
func (f *SportsFetcher) doRequest(ctx context.Context, apiURL string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("User-Agent", version.BrowserUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

// convertEvent converts a TheSportsDB event to our GameData structure
func convertEvent(evt sportsDBEvent) GameData {
	game := GameData{
		ID:            evt.IDEvent,
		League:        evt.StrLeague,
		LeagueID:      evt.IDLeague,
		Season:        evt.StrSeason,
		HomeTeam:      evt.StrHomeTeam,
		HomeTeamID:    evt.IDHomeTeam,
		HomeTeamBadge: evt.StrHomeTeamBadge,
		AwayTeam:      evt.StrAwayTeam,
		AwayTeamID:    evt.IDAwayTeam,
		AwayTeamBadge: evt.StrAwayTeamBadge,
		Venue:         evt.StrVenue,
		Round:         evt.IntRound,
	}

	// Parse scores
	if evt.IntHomeScore != nil && *evt.IntHomeScore != "" {
		if score := parseScore(*evt.IntHomeScore); score >= 0 {
			game.HomeScore = &score
		}
	}
	if evt.IntAwayScore != nil && *evt.IntAwayScore != "" {
		if score := parseScore(*evt.IntAwayScore); score >= 0 {
			game.AwayScore = &score
		}
	}

	// Parse status
	game.Status = parseGameStatus(evt.StrStatus, evt.StrProgress, game.HomeScore)
	game.StatusDetail = getStatusDetail(evt, game.Status)

	// Parse start time
	if evt.StrTimestamp != "" {
		game.StartTime = evt.StrTimestamp
	} else if evt.DateEvent != "" && evt.StrTime != "" {
		game.StartTime = evt.DateEvent + "T" + evt.StrTime + "Z"
	} else if evt.DateEvent != "" {
		game.StartTime = evt.DateEvent + "T00:00:00Z"
	}

	return game
}

// parseScore safely parses a score string to int
func parseScore(s string) int {
	var score int
	_, err := fmt.Sscanf(s, "%d", &score)
	if err != nil {
		return -1
	}
	return score
}

// parseGameStatus determines the game status from API fields
func parseGameStatus(status, progress string, homeScore *int) GameStatus {
	statusLower := strings.ToLower(status)
	progressLower := strings.ToLower(progress)

	// Check for live indicators
	if strings.Contains(statusLower, "live") ||
		strings.Contains(statusLower, "in progress") ||
		strings.Contains(progressLower, "ht") ||
		strings.Contains(progressLower, "'") ||
		strings.Contains(progressLower, "q1") ||
		strings.Contains(progressLower, "q2") ||
		strings.Contains(progressLower, "q3") ||
		strings.Contains(progressLower, "q4") ||
		strings.Contains(progressLower, "1st") ||
		strings.Contains(progressLower, "2nd") ||
		strings.Contains(progressLower, "3rd") ||
		(progress != "" && !strings.Contains(progressLower, "ft")) {
		return GameStatusLive
	}

	// Check for final
	if strings.Contains(statusLower, "match finished") ||
		strings.Contains(statusLower, "final") ||
		strings.Contains(statusLower, "ft") ||
		strings.Contains(statusLower, "aet") ||
		strings.Contains(progressLower, "ft") ||
		strings.Contains(progressLower, "aet") {
		return GameStatusFinal
	}

	// Check for postponed/canceled
	if strings.Contains(statusLower, "postponed") {
		return GameStatusPostponed
	}
	if strings.Contains(statusLower, "canceled") || strings.Contains(statusLower, "cancelled") {
		return GameStatusCanceled
	}

	// If score is present, likely final
	if homeScore != nil {
		return GameStatusFinal
	}

	// Default to scheduled
	return GameStatusScheduled
}

// getStatusDetail returns human-readable status detail
func getStatusDetail(evt sportsDBEvent, status GameStatus) string {
	if evt.StrProgress != "" && status == GameStatusLive {
		return evt.StrProgress
	}

	switch status {
	case GameStatusFinal:
		return "Final"
	case GameStatusPostponed:
		return "Postponed"
	case GameStatusCanceled:
		return "Canceled"
	case GameStatusScheduled:
		if evt.StrTime != "" {
			return evt.StrTime
		}
		return "Scheduled"
	default:
		return ""
	}
}

// resolveTeamName resolves a team alias to full name
func resolveTeamName(query string) string {
	queryLower := strings.ToLower(strings.TrimSpace(query))

	if fullName, ok := teamAliases[queryLower]; ok {
		return fullName
	}

	// Return original query with proper casing
	return query
}

// resolveLeagueName resolves a league alias to the proper name
func resolveLeagueName(query string) string {
	queryLower := strings.ToLower(strings.TrimSpace(query))

	if fullName, ok := leagueNameMappings[queryLower]; ok {
		return fullName
	}

	return query
}
