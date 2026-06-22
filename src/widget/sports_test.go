package widget

import (
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestResolveTeamName(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"lakers alias", "lakers", "Los Angeles Lakers"},
		{"patriots alias", "patriots", "New England Patriots"},
		{"yankees alias", "yankees", "New York Yankees"},
		{"warriors alias", "warriors", "Golden State Warriors"},
		{"man utd alias", "man utd", "Manchester United"},
		{"barca alias", "barca", "FC Barcelona"},
		{"unknown query returned unchanged", "foobarteam", "foobarteam"},
		{"empty string returned unchanged", "", ""},
		{"case insensitive LAKERS", "LAKERS", "Los Angeles Lakers"},
		{"case insensitive mixed MaN uTd", "MaN uTd", "Manchester United"},
		{"leading and trailing whitespace trimmed", "  yankees  ", "New York Yankees"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTeamName(tt.query)
			if got != tt.want {
				t.Errorf("resolveTeamName(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestResolveLeagueName(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"nfl mapping", "nfl", "NFL"},
		{"nba mapping", "nba", "NBA"},
		{"epl mapping", "epl", "English Premier League"},
		{"f1 mapping", "f1", "Formula 1"},
		{"mls mapping", "mls", "MLS"},
		{"unknown returned unchanged", "unknownleague", "unknownleague"},
		{"empty returned unchanged", "", ""},
		{"case insensitive NFL uppercase", "NFL", "NFL"},
		{"premier league full name", "premier league", "English Premier League"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveLeagueName(tt.query)
			if got != tt.want {
				t.Errorf("resolveLeagueName(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

func TestParseScore(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"single digit", "3", 3},
		{"zero", "0", 0},
		{"two digits", "21", 21},
		{"empty string returns -1", "", -1},
		{"non-numeric returns -1", "abc", -1},
		{"large score", "100", 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseScore(tt.input)
			if got != tt.want {
				t.Errorf("parseScore(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseGameStatus(t *testing.T) {
	score := 3

	tests := []struct {
		name      string
		status    string
		progress  string
		homeScore *int
		want      GameStatus
	}{
		{"match finished is final", "Match Finished", "", nil, GameStatusFinal},
		{"live status is live", "live", "", nil, GameStatusLive},
		{"in progress is live", "In Progress", "", nil, GameStatusLive},
		{"HT progress is live", "", "HT", nil, GameStatusLive},
		{"FT progress is final", "", "FT", nil, GameStatusFinal},
		{"postponed", "Postponed", "", nil, GameStatusPostponed},
		{"canceled American spelling", "Canceled", "", nil, GameStatusCanceled},
		{"cancelled British spelling", "Cancelled", "", nil, GameStatusCanceled},
		{"score present no status is final", "", "", &score, GameStatusFinal},
		{"empty status nil score is scheduled", "", "", nil, GameStatusScheduled},
		{"ft in status is final", "FT", "", nil, GameStatusFinal},
		{"aet in status is final", "AET", "", nil, GameStatusFinal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGameStatus(tt.status, tt.progress, tt.homeScore)
			if got != tt.want {
				t.Errorf("parseGameStatus(%q, %q, homeScore=%v) = %q, want %q",
					tt.status, tt.progress, tt.homeScore, got, tt.want)
			}
		})
	}
}

func TestGetStatusDetail(t *testing.T) {
	tests := []struct {
		name   string
		evt    sportsDBEvent
		status GameStatus
		want   string
	}{
		{
			name:   "live with progress returns progress string",
			evt:    sportsDBEvent{StrProgress: "45'"},
			status: GameStatusLive,
			want:   "45'",
		},
		{
			name:   "final returns Final",
			evt:    sportsDBEvent{},
			status: GameStatusFinal,
			want:   "Final",
		},
		{
			name:   "postponed returns Postponed",
			evt:    sportsDBEvent{},
			status: GameStatusPostponed,
			want:   "Postponed",
		},
		{
			name:   "canceled returns Canceled",
			evt:    sportsDBEvent{},
			status: GameStatusCanceled,
			want:   "Canceled",
		},
		{
			name:   "scheduled with time returns the time",
			evt:    sportsDBEvent{StrTime: "7:00 PM"},
			status: GameStatusScheduled,
			want:   "7:00 PM",
		},
		{
			name:   "scheduled with no time returns Scheduled",
			evt:    sportsDBEvent{StrTime: ""},
			status: GameStatusScheduled,
			want:   "Scheduled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStatusDetail(tt.evt, tt.status)
			if got != tt.want {
				t.Errorf("getStatusDetail() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConvertEvent(t *testing.T) {
	homeScoreStr := "3"
	awayScoreStr := "1"

	t.Run("basic fields are copied to GameData", func(t *testing.T) {
		evt := sportsDBEvent{
			IDEvent:          "12345",
			StrLeague:        "NBA",
			IDLeague:         "4387",
			StrSeason:        "2023-2024",
			StrHomeTeam:      "Los Angeles Lakers",
			IDHomeTeam:       "134860",
			StrAwayTeam:      "Boston Celtics",
			IDAwayTeam:       "134861",
			StrVenue:         "Crypto.com Arena",
			IntRound:         "5",
			StrHomeTeamBadge: "https://example.com/lakers.png",
			StrAwayTeamBadge: "https://example.com/celtics.png",
		}
		game := convertEvent(evt)
		if game.ID != "12345" {
			t.Errorf("ID = %q, want %q", game.ID, "12345")
		}
		if game.League != "NBA" {
			t.Errorf("League = %q, want %q", game.League, "NBA")
		}
		if game.LeagueID != "4387" {
			t.Errorf("LeagueID = %q, want %q", game.LeagueID, "4387")
		}
		if game.HomeTeam != "Los Angeles Lakers" {
			t.Errorf("HomeTeam = %q, want %q", game.HomeTeam, "Los Angeles Lakers")
		}
		if game.AwayTeam != "Boston Celtics" {
			t.Errorf("AwayTeam = %q, want %q", game.AwayTeam, "Boston Celtics")
		}
		if game.Venue != "Crypto.com Arena" {
			t.Errorf("Venue = %q, want %q", game.Venue, "Crypto.com Arena")
		}
	})

	t.Run("scores parsed from string pointers", func(t *testing.T) {
		evt := sportsDBEvent{
			StrStatus:    "Match Finished",
			IntHomeScore: &homeScoreStr,
			IntAwayScore: &awayScoreStr,
		}
		game := convertEvent(evt)
		if game.HomeScore == nil || *game.HomeScore != 3 {
			t.Errorf("HomeScore = %v, want 3", game.HomeScore)
		}
		if game.AwayScore == nil || *game.AwayScore != 1 {
			t.Errorf("AwayScore = %v, want 1", game.AwayScore)
		}
	})

	t.Run("nil score pointers produce nil game scores", func(t *testing.T) {
		evt := sportsDBEvent{}
		game := convertEvent(evt)
		if game.HomeScore != nil {
			t.Errorf("HomeScore should be nil, got %v", game.HomeScore)
		}
		if game.AwayScore != nil {
			t.Errorf("AwayScore should be nil, got %v", game.AwayScore)
		}
	})

	t.Run("start time from StrTimestamp takes precedence over DateEvent", func(t *testing.T) {
		evt := sportsDBEvent{
			StrTimestamp: "2024-01-15T19:00:00",
			DateEvent:    "2024-01-15",
			StrTime:      "19:00:00",
		}
		game := convertEvent(evt)
		if game.StartTime != "2024-01-15T19:00:00" {
			t.Errorf("StartTime = %q, want %q", game.StartTime, "2024-01-15T19:00:00")
		}
	})

	t.Run("start time built from DateEvent and StrTime when no timestamp", func(t *testing.T) {
		evt := sportsDBEvent{
			DateEvent: "2024-01-15",
			StrTime:   "19:00:00",
		}
		game := convertEvent(evt)
		if game.StartTime != "2024-01-15T19:00:00Z" {
			t.Errorf("StartTime = %q, want %q", game.StartTime, "2024-01-15T19:00:00Z")
		}
	})

	t.Run("start time from DateEvent alone when no time field", func(t *testing.T) {
		evt := sportsDBEvent{
			DateEvent: "2024-01-15",
		}
		game := convertEvent(evt)
		if game.StartTime != "2024-01-15T00:00:00Z" {
			t.Errorf("StartTime = %q, want %q", game.StartTime, "2024-01-15T00:00:00Z")
		}
	})
}

func TestSportsFetcherCacheDuration(t *testing.T) {
	cfg := &config.SportsWidgetConfig{}

	tests := []struct {
		name       string
		lastStatus GameStatus
		want       time.Duration
	}{
		{"live game cache is 1 minute", GameStatusLive, 1 * time.Minute},
		{"scheduled game cache is 30 minutes", GameStatusScheduled, 30 * time.Minute},
		{"final game cache is 1 hour", GameStatusFinal, 1 * time.Hour},
		{"postponed falls through to default 1 hour", GameStatusPostponed, 1 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewSportsFetcher(cfg)
			f.lastStatus = tt.lastStatus
			got := f.CacheDuration()
			if got != tt.want {
				t.Errorf("CacheDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSportsFetcherUpdateLastStatus(t *testing.T) {
	cfg := &config.SportsWidgetConfig{}

	t.Run("nil data sets lastStatus to final", func(t *testing.T) {
		f := NewSportsFetcher(cfg)
		f.updateLastStatus(nil)
		if f.lastStatus != GameStatusFinal {
			t.Errorf("lastStatus = %q, want %q", f.lastStatus, GameStatusFinal)
		}
	})

	t.Run("empty games slice sets lastStatus to final", func(t *testing.T) {
		f := NewSportsFetcher(cfg)
		f.updateLastStatus(&SportsData{Games: []GameData{}})
		if f.lastStatus != GameStatusFinal {
			t.Errorf("lastStatus = %q, want %q", f.lastStatus, GameStatusFinal)
		}
	})

	t.Run("one live game sets lastStatus to live and HasLive true", func(t *testing.T) {
		f := NewSportsFetcher(cfg)
		data := &SportsData{
			Games: []GameData{
				{Status: GameStatusLive},
			},
		}
		f.updateLastStatus(data)
		if f.lastStatus != GameStatusLive {
			t.Errorf("lastStatus = %q, want %q", f.lastStatus, GameStatusLive)
		}
		if !data.HasLive {
			t.Error("HasLive should be true when a live game is present")
		}
	})

	t.Run("all scheduled games sets lastStatus to scheduled", func(t *testing.T) {
		f := NewSportsFetcher(cfg)
		data := &SportsData{
			Games: []GameData{
				{Status: GameStatusScheduled},
				{Status: GameStatusScheduled},
			},
		}
		f.updateLastStatus(data)
		if f.lastStatus != GameStatusScheduled {
			t.Errorf("lastStatus = %q, want %q", f.lastStatus, GameStatusScheduled)
		}
		if data.HasLive {
			t.Error("HasLive should be false when no live games present")
		}
	})

	t.Run("mix of final and scheduled sets lastStatus to final", func(t *testing.T) {
		f := NewSportsFetcher(cfg)
		data := &SportsData{
			Games: []GameData{
				{Status: GameStatusFinal},
				{Status: GameStatusScheduled},
			},
		}
		f.updateLastStatus(data)
		if f.lastStatus != GameStatusFinal {
			t.Errorf("lastStatus = %q, want %q", f.lastStatus, GameStatusFinal)
		}
	})

	t.Run("live game among others sets lastStatus to live immediately", func(t *testing.T) {
		f := NewSportsFetcher(cfg)
		data := &SportsData{
			Games: []GameData{
				{Status: GameStatusFinal},
				{Status: GameStatusLive},
				{Status: GameStatusScheduled},
			},
		}
		f.updateLastStatus(data)
		if f.lastStatus != GameStatusLive {
			t.Errorf("lastStatus = %q, want %q", f.lastStatus, GameStatusLive)
		}
		if !data.HasLive {
			t.Error("HasLive should be true when a live game is present")
		}
	})
}
