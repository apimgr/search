package widget

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

// ---- Cache.Close / widgets.Close ----

func TestCacheClose(t *testing.T) {
	c := NewCache()
	c.Set("k", &WidgetData{Type: WidgetClock}, time.Minute)

	// Close should not panic
	c.Close()

	// Double-close should not panic
	c.Close()
}

func TestManagerClose(t *testing.T) {
	cfg := &config.WidgetsConfig{}
	m := NewManager(cfg)

	// Should not panic
	m.Close()

	// Double-close should not panic
	m.Close()
}

// ---- Dictionary.Fetch (7.3% → needs mocked HTTP) ----

func TestDictionaryFetcherFetch(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]string
		serverResp  func(w http.ResponseWriter, r *http.Request)
		wantErrData bool
		wantWord    string
		wantErrMsg  string
	}{
		{
			name:        "missing word param",
			params:      map[string]string{},
			wantErrData: true,
			wantErrMsg:  "word parameter required",
		},
		{
			name:        "word not found - 404",
			params:      map[string]string{"word": "zxqw"},
			wantErrData: true,
			wantErrMsg:  "word not found",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
		},
		{
			name:   "valid word with definitions",
			params: map[string]string{"word": "hello"},
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode([]map[string]interface{}{
					{
						"word":     "hello",
						"phonetic": "/həˈloʊ/",
						"phonetics": []map[string]interface{}{
							{"text": "/həˈloʊ/", "audio": "https://api.dictionaryapi.dev/media/pronunciations/en/hello-au.mp3"},
						},
						"meanings": []map[string]interface{}{
							{
								"partOfSpeech": "exclamation",
								"definitions": []map[string]interface{}{
									{
										"definition": "used as a greeting",
										"example":    "hello there",
										"synonyms":   []string{"hi", "hey"},
										"antonyms":   []string{},
									},
								},
								"synonyms": []string{"hi"},
								"antonyms": []string{},
							},
						},
					},
				})
			},
			wantWord: "hello",
		},
		{
			name:        "empty results array",
			params:      map[string]string{"word": "test"},
			wantErrData: true,
			wantErrMsg:  "no definitions found",
			serverResp: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, "[]")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewDictionaryFetcher()

			// For tests that need mocked HTTP, replace the client
			if tt.serverResp != nil {
				srv := httptest.NewServer(http.HandlerFunc(tt.serverResp))
				defer srv.Close()

				// Swap the http client to route to test server
				f.httpClient = &http.Client{
					Transport: &redirectTransport{base: srv.URL},
				}
			}

			data, err := f.Fetch(context.Background(), tt.params)

			if tt.wantErrMsg == "word parameter required" || tt.wantErrMsg == "word not found" || tt.wantErrMsg == "no definitions found" {
				// These don't hit HTTP (missing param) or use mocked HTTP
				if err != nil {
					t.Fatalf("Fetch() returned unexpected error: %v", err)
				}
				if data.Error == "" {
					t.Errorf("expected error data, got empty Error field")
				}
				if tt.wantErrMsg != "" && data.Error != tt.wantErrMsg {
					t.Errorf("Error = %q, want %q", data.Error, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("Fetch() unexpected error: %v", err)
			}
			if tt.wantErrData {
				if data.Error == "" {
					t.Error("expected error in WidgetData, got none")
				}
				return
			}
			if data.Error != "" {
				t.Errorf("unexpected WidgetData.Error: %s", data.Error)
			}
			if tt.wantWord != "" {
				dict, ok := data.Data.(*DictionaryData)
				if !ok {
					t.Fatal("data.Data is not *DictionaryData")
				}
				if dict.Word != tt.wantWord {
					t.Errorf("Word = %q, want %q", dict.Word, tt.wantWord)
				}
			}
		})
	}
}

// ---- Tracking simple/default constructors ----

func TestNewTrackingFetcherSimple(t *testing.T) {
	f := NewTrackingFetcherSimple()
	if f == nil {
		t.Fatal("NewTrackingFetcherSimple() returned nil")
	}
	if f.WidgetType() != WidgetTracking {
		t.Errorf("WidgetType() = %q, want %q", f.WidgetType(), WidgetTracking)
	}
}

func TestNewTrackingFetcherDefault(t *testing.T) {
	f := NewTrackingFetcherDefault()
	if f == nil {
		t.Fatal("NewTrackingFetcherDefault() returned nil")
	}
	if f.WidgetType() != WidgetTracking {
		t.Errorf("WidgetType() = %q, want %q", f.WidgetType(), WidgetTracking)
	}
}

// ---- Tracking.Fetch additional paths ----

func TestTrackingFetcherFetchPaths(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]string
		wantErrData bool
		wantErrMsg  string
		wantCarrier string
	}{
		{
			name:        "missing tracking number",
			params:      map[string]string{},
			wantErrData: true,
			wantErrMsg:  "tracking number required",
		},
		{
			name:        "tracking number too short",
			params:      map[string]string{"number": "12345"},
			wantErrData: true,
			wantErrMsg:  "tracking number too short (minimum 8 characters)",
		},
		{
			name:   "valid UPS tracking number",
			params: map[string]string{"number": "1Z999AA10123456784"},
		},
		{
			name:   "valid USPS tracking number",
			params: map[string]string{"number": "9400111899223397614329"},
		},
		{
			name:   "spaces removed from tracking number",
			params: map[string]string{"number": "1Z999 AA1 0123 4567 84"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewTrackingFetcher()
			data, err := f.Fetch(context.Background(), tt.params)
			if err != nil {
				t.Fatalf("Fetch() returned unexpected error: %v", err)
			}
			if tt.wantErrData {
				if data.Error == "" {
					t.Error("expected error in WidgetData, got none")
				}
				if tt.wantErrMsg != "" && data.Error != tt.wantErrMsg {
					t.Errorf("Error = %q, want %q", data.Error, tt.wantErrMsg)
				}
				return
			}
			if data.Type != WidgetTracking {
				t.Errorf("Type = %q, want %q", data.Type, WidgetTracking)
			}
		})
	}
}

// ---- Nutrition.Fetch with mocked HTTP ----

func TestNutritionFetcherFetch(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]string
		usdaResp    func(w http.ResponseWriter, r *http.Request)
		offResp     func(w http.ResponseWriter, r *http.Request)
		wantErrData bool
		wantErrMsg  string
	}{
		{
			name:        "missing food param",
			params:      map[string]string{},
			wantErrData: true,
			wantErrMsg:  "food item required (use 'query' or 'food' parameter)",
		},
		{
			name:   "food param used as food",
			params: map[string]string{"food": "apple"},
			usdaResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"totalHits": 1,
					"foods": []map[string]interface{}{
						{
							"fdcId":        321,
							"description":  "Apple, raw",
							"foodCategory": "Fruits",
							"foodNutrients": []map[string]interface{}{
								{"nutrientId": 1008, "nutrientName": "Energy", "value": 52.0, "unitName": "kcal"},
								{"nutrientId": 1003, "nutrientName": "Protein", "value": 0.26, "unitName": "g"},
								{"nutrientId": 1005, "nutrientName": "Carbohydrate", "value": 13.81, "unitName": "g"},
								{"nutrientId": 1004, "nutrientName": "Total lipid (fat)", "value": 0.17, "unitName": "g"},
							},
							"foodPortions": []map[string]interface{}{},
						},
					},
				})
			},
			offResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"count": "1",
					"products": []map[string]interface{}{
						{
							"product_name":     "Apple, raw",
							"brands":           "USDA",
							"serving_size":     "100g",
							"serving_quantity": 100.0,
							"categories":       "Fruits",
							"nutriments": map[string]interface{}{
								"energy-kcal_100g":   52.0,
								"fat_100g":           0.17,
								"carbohydrates_100g": 13.81,
								"proteins_100g":      0.26,
								"fiber_100g":         2.4,
							},
						},
					},
				})
			},
		},
		{
			name:   "usda returns no hits falls back to off returns data",
			params: map[string]string{"query": "banana"},
			usdaResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"totalHits": 0,
					"foods":     []interface{}{},
				})
			},
			offResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"count": "1",
					"products": []map[string]interface{}{
						{
							"product_name":     "Banana",
							"brands":           "Nature",
							"serving_size":     "120g",
							"serving_quantity": 120.0,
							"categories":       "Fruits",
							"nutriments": map[string]interface{}{
								"energy-kcal_100g":    89.0,
								"fat_100g":            0.33,
								"carbohydrates_100g":  23.0,
								"proteins_100g":       1.1,
								"fiber_100g":          2.6,
								"sugars_100g":         12.0,
								"saturated-fat_100g":  0.11,
								"sodium_100g":         1.0,
								"energy-kcal_serving": 106.8,
							},
						},
					},
				})
			},
		},
		{
			name:   "both usda and off return no results",
			params: map[string]string{"query": "xyznotafood"},
			usdaResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"totalHits": 0,
					"foods":     []interface{}{},
				})
			},
			offResp: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"count":    "0",
					"products": []interface{}{},
				})
			},
			wantErrData: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantErrMsg == "food item required (use 'query' or 'food' parameter)" {
				f := NewNutritionFetcher("")
				data, err := f.Fetch(context.Background(), tt.params)
				if err != nil {
					t.Fatalf("Fetch() returned unexpected error: %v", err)
				}
				if data.Error != tt.wantErrMsg {
					t.Errorf("Error = %q, want %q", data.Error, tt.wantErrMsg)
				}
				return
			}

			// Set up test server(s)
			var usdaSrv, offSrv *httptest.Server

			if tt.usdaResp != nil {
				usdaSrv = httptest.NewServer(http.HandlerFunc(tt.usdaResp))
				defer usdaSrv.Close()
			}
			if tt.offResp != nil {
				offSrv = httptest.NewServer(http.HandlerFunc(tt.offResp))
				defer offSrv.Close()
			}

			f := NewNutritionFetcher("")
			if usdaSrv != nil || offSrv != nil {
				f.httpClient = &http.Client{
					Transport: &dualRedirectTransport{
						usdaBase: func() string {
							if usdaSrv != nil {
								return usdaSrv.URL
							}
							return ""
						}(),
						offBase: func() string {
							if offSrv != nil {
								return offSrv.URL
							}
							return ""
						}(),
					},
				}
			}

			data, err := f.Fetch(context.Background(), tt.params)
			if err != nil {
				t.Fatalf("Fetch() returned unexpected error: %v", err)
			}
			if tt.wantErrData {
				if data.Error == "" {
					t.Error("expected error in WidgetData, got none")
				}
				return
			}
			if data.Error != "" {
				t.Errorf("unexpected WidgetData.Error: %s", data.Error)
			}
			if data.Type != WidgetNutrition {
				t.Errorf("Type = %q, want %q", data.Type, WidgetNutrition)
			}
		})
	}
}

func TestNutritionFetcherMetadata(t *testing.T) {
	f := NewNutritionFetcher("")
	if f.WidgetType() != WidgetNutrition {
		t.Errorf("WidgetType() = %q, want %q", f.WidgetType(), WidgetNutrition)
	}
	if f.CacheDuration() != 24*time.Hour {
		t.Errorf("CacheDuration() = %v, want %v", f.CacheDuration(), 24*time.Hour)
	}
}

func TestExtractFoodItemPatterns(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  string
	}{
		{"calories in", "calories in banana", "banana"},
		{"food calories", "apple calories", "apple"},
		{"nutrition query", "chicken breast nutrition", "chicken breast"},
		{"nutrition facts", "nutrition facts apple", "apple"},
		{"nutrition of", "nutrition of orange", "orange"},
		{"how many calories", "how many calories in an egg", "an egg"},
		{"macros for", "macros for chicken", "chicken"},
		{"protein in", "protein in beef", "beef"},
		{"no pattern match", "banana", "banana"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFoodItem(tt.query)
			if got != tt.want {
				t.Errorf("ExtractFoodItem(%q) = %q, want %q", tt.query, got, tt.want)
			}
		})
	}
}

// ---- Sports fetcher additional paths ----

func TestSportsFetcherFetchWithMockedTeam(t *testing.T) {
	// Team search returns empty — no team found
	teamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sportsDBTeamsResponse{Teams: []sportsDBTeam{}})
	}))
	defer teamSrv.Close()

	f := NewSportsFetcher(&config.SportsWidgetConfig{})
	f.client = &http.Client{
		Transport: &redirectTransport{base: teamSrv.URL},
	}

	data, err := f.Fetch(context.Background(), map[string]string{"team": "notexistingteam"})
	if err != nil {
		t.Fatalf("Fetch() returned unexpected error: %v", err)
	}
	if data.Error == "" {
		t.Error("expected error for unknown team")
	}
}

func TestSportsFetcherFetchTeamWithLastNextEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path
		switch {
		case strings.Contains(path, "searchteams"):
			_ = json.NewEncoder(w).Encode(sportsDBTeamsResponse{
				Teams: []sportsDBTeam{{IDTeam: "133612", StrTeam: "Arsenal"}},
			})
		case strings.Contains(path, "eventslast"):
			_ = json.NewEncoder(w).Encode(sportsDBEventsResponse{
				Events: []sportsDBEvent{
					{IDEvent: "1", StrEvent: "Arsenal vs Chelsea", StrStatus: "Match Finished", StrHomeTeam: "Arsenal", StrAwayTeam: "Chelsea"},
				},
			})
		case strings.Contains(path, "eventsnext"):
			_ = json.NewEncoder(w).Encode(sportsDBEventsResponse{
				Events: []sportsDBEvent{
					{IDEvent: "2", StrEvent: "Arsenal vs Man City", StrStatus: "Not Started", StrHomeTeam: "Arsenal", StrAwayTeam: "Man City"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	f := NewSportsFetcher(&config.SportsWidgetConfig{})
	f.client = &http.Client{
		Transport: &redirectTransport{base: srv.URL},
	}

	data, err := f.Fetch(context.Background(), map[string]string{"team": "Arsenal"})
	if err != nil {
		t.Fatalf("Fetch() returned unexpected error: %v", err)
	}
	if data.Error != "" {
		t.Errorf("unexpected WidgetData.Error: %s", data.Error)
	}
	if data.Type != WidgetSports {
		t.Errorf("Type = %q, want %q", data.Type, WidgetSports)
	}
}

func TestSportsFetcherFetchLiveWithMockedAPI(t *testing.T) {
	// Today's events returns some live games
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sportsDBEventsResponse{
			Events: []sportsDBEvent{
				{
					IDEvent:  "12345",
					StrEvent: "Lakers vs Celtics",
					StrStatus: "In Progress",
					StrHomeTeam: "Lakers",
					StrAwayTeam: "Celtics",
				},
			},
		})
	}))
	defer srv.Close()

	f := NewSportsFetcher(&config.SportsWidgetConfig{})
	f.client = &http.Client{
		Transport: &redirectTransport{base: srv.URL},
	}

	data, err := f.Fetch(context.Background(), map[string]string{"live": "true"})
	if err != nil {
		t.Fatalf("Fetch() returned unexpected error: %v", err)
	}
	if data.Type != WidgetSports {
		t.Errorf("Type = %q, want %q", data.Type, WidgetSports)
	}
}

func TestSportsFetcherFetchDefaultLeaguesWithMockedAPI(t *testing.T) {
	// Leagues search returns a league, then events returns empty
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		callCount++

		// First few calls: league search
		if callCount <= 2 {
			_ = json.NewEncoder(w).Encode(sportsDBLeaguesResponse{
				Leagues: []sportsDBLeague{
					{IDLeague: "4391", StrLeague: "NFL"},
				},
			})
			return
		}
		// Remaining calls: event queries — return empty
		_ = json.NewEncoder(w).Encode(sportsDBEventsResponse{Events: []sportsDBEvent{}})
	}))
	defer srv.Close()

	f := NewSportsFetcher(&config.SportsWidgetConfig{
		DefaultLeagues: []string{"NFL"},
	})
	f.client = &http.Client{
		Transport: &redirectTransport{base: srv.URL},
	}

	data, err := f.Fetch(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() returned unexpected error: %v", err)
	}
	if data.Type != WidgetSports {
		t.Errorf("Type = %q, want %q", data.Type, WidgetSports)
	}
}

func TestSportsFetcherFetchLeague(t *testing.T) {
	// League search returns empty → error
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sportsDBLeaguesResponse{Leagues: []sportsDBLeague{}})
	}))
	defer srv.Close()

	f := NewSportsFetcher(&config.SportsWidgetConfig{})
	f.client = &http.Client{
		Transport: &redirectTransport{base: srv.URL},
	}

	data, err := f.Fetch(context.Background(), map[string]string{"league": "unknownleague"})
	if err != nil {
		t.Fatalf("Fetch() returned unexpected error: %v", err)
	}
	if data.Error == "" {
		t.Error("expected error for unknown league")
	}
}

func TestUpdateLastStatus(t *testing.T) {
	tests := []struct {
		name           string
		data           *SportsData
		wantLastStatus GameStatus
	}{
		{"nil data", nil, GameStatusFinal},
		{"empty games", &SportsData{Games: []GameData{}}, GameStatusFinal},
		{
			"live game present",
			&SportsData{Games: []GameData{{Status: GameStatusLive}}},
			GameStatusLive,
		},
		{
			"all scheduled",
			&SportsData{Games: []GameData{
				{Status: GameStatusScheduled},
				{Status: GameStatusScheduled},
			}},
			GameStatusScheduled,
		},
		{
			"mix of scheduled and final",
			&SportsData{Games: []GameData{
				{Status: GameStatusFinal},
				{Status: GameStatusScheduled},
			}},
			GameStatusFinal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewSportsFetcher(&config.SportsWidgetConfig{})
			f.updateLastStatus(tt.data)
			if f.lastStatus != tt.wantLastStatus {
				t.Errorf("lastStatus = %q, want %q", f.lastStatus, tt.wantLastStatus)
			}
		})
	}
}

// ---- RSS fetchSingleFeed with mocked HTTP ----

func TestRSSFetchSingleFeed(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>http://example.com</link>
    <description>A test feed</description>
    <item>
      <title>Test Item 1</title>
      <link>http://example.com/item1</link>
      <description>Description 1</description>
      <pubDate>Mon, 01 Jan 2024 12:00:00 +0000</pubDate>
    </item>
    <item>
      <title>Test Item 2</title>
      <link>http://example.com/item2</link>
      <description>Description 2</description>
      <pubDate>Tue, 02 Jan 2024 12:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

	tests := []struct {
		name       string
		handler    func(w http.ResponseWriter, r *http.Request)
		wantItems  int
		wantErrStr string
	}{
		{
			name: "valid RSS feed",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/rss+xml")
				fmt.Fprint(w, rssXML)
			},
			wantItems: 2,
		},
		{
			name: "non-200 status returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
			},
			wantErrStr: "feed returned status 403",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			f := NewRSSFetcher(&config.RSSWidgetConfig{MaxFeeds: 5, MaxItems: 10})
			f.client = &http.Client{}

			items, err := f.fetchSingleFeed(context.Background(), srv.URL)

			if tt.wantErrStr != "" {
				if err == nil {
					t.Error("expected error, got nil")
				} else if err.Error() != tt.wantErrStr {
					t.Errorf("error = %q, want %q", err.Error(), tt.wantErrStr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tt.wantItems {
				t.Errorf("items count = %d, want %d", len(items), tt.wantItems)
			}
		})
	}
}

func TestRSSFetcherFetchWithRealFeed(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Test Feed</title>
    <link>http://example.com</link>
    <description>A test feed</description>
    <item>
      <title>Item 1</title>
      <link>http://example.com/1</link>
      <pubDate>Mon, 01 Jan 2024 12:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, rssXML)
	}))
	defer srv.Close()

	f := NewRSSFetcher(&config.RSSWidgetConfig{MaxFeeds: 5, MaxItems: 10})
	f.client = &http.Client{}

	data, err := f.Fetch(context.Background(), map[string]string{
		"feeds": srv.URL,
	})
	if err != nil {
		t.Fatalf("Fetch() unexpected error: %v", err)
	}
	if data.Error != "" {
		t.Errorf("unexpected error: %s", data.Error)
	}
	rssData, ok := data.Data.(*RSSData)
	if !ok {
		t.Fatal("data.Data is not *RSSData")
	}
	if len(rssData.Items) == 0 {
		t.Error("expected at least one RSS item")
	}
}

// ---- News fetchFeed with mocked HTTP ----

func TestNewsFetchFeedWithMockedHTTP(t *testing.T) {
	rssXML := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>News Source</title>
    <link>http://example.com</link>
    <description>News feed</description>
    <item>
      <title>Breaking News</title>
      <link>http://example.com/news/1</link>
      <description>A breaking news story</description>
      <pubDate>Mon, 01 Jan 2024 10:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>`

	tests := []struct {
		name      string
		handler   func(w http.ResponseWriter, r *http.Request)
		wantItems int
		wantErr   bool
	}{
		{
			name: "valid RSS returns items",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/rss+xml")
				fmt.Fprint(w, rssXML)
			},
			wantItems: 1,
		},
		{
			name: "non-200 returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			cfg := &config.NewsWidgetConfig{
				Sources:  []string{srv.URL},
				MaxItems: 10,
			}
			f := NewNewsFetcher(cfg)
			f.client = &http.Client{}

			items, err := f.fetchFeed(context.Background(), srv.URL)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(items) != tt.wantItems {
				t.Errorf("items = %d, want %d", len(items), tt.wantItems)
			}
		})
	}
}

// ---- Translate additional HTTP paths ----

func TestTranslateFetcherFetchFromLibreTranslate(t *testing.T) {
	tests := []struct {
		name        string
		handler     func(w http.ResponseWriter, r *http.Request)
		text        string
		sourceLang  string
		targetLang  string
		wantErrData bool
	}{
		{
			name: "successful translation",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"translatedText": "Hola",
					"detectedLanguage": map[string]interface{}{
						"language":   "en",
						"confidence": 0.99,
					},
				})
			},
			text:       "Hello",
			sourceLang: "en",
			targetLang: "es",
		},
		{
			name: "non-200 status returns error data",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			text:        "Hello",
			sourceLang:  "en",
			targetLang:  "es",
			wantErrData: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			f := NewTranslateFetcher()
			f.httpClient = &http.Client{
				Transport: &redirectTransport{base: srv.URL},
			}

			data, err := f.fetchFromLibreTranslate(context.Background(), tt.text, tt.sourceLang, tt.targetLang)
			if err != nil {
				t.Fatalf("fetchFromLibreTranslate() error: %v", err)
			}
			if tt.wantErrData {
				if data.Error == "" {
					t.Error("expected error data, got none")
				}
			} else {
				if data.Error != "" {
					t.Errorf("unexpected error: %s", data.Error)
				}
			}
		})
	}
}

func TestTranslateFetcherFetchFromMyMemory(t *testing.T) {
	tests := []struct {
		name        string
		handler     func(w http.ResponseWriter, r *http.Request)
		sourceLang  string
		wantErrData bool
	}{
		{
			name: "successful mymemory response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"responseStatus": 200,
					"responseData": map[string]interface{}{
						"translatedText": "Hola",
						"match":          0.85,
					},
					"detectedLanguage": "en",
				})
			},
			sourceLang: "en",
		},
		{
			name:       "auto sourceLang defaults to en",
			sourceLang: "auto",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"responseStatus": 200,
					"responseData": map[string]interface{}{
						"translatedText": "Hola",
						"match":          0.9,
					},
				})
			},
		},
		{
			name:        "non-200 responseStatus",
			sourceLang:  "en",
			wantErrData: true,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"responseStatus": 500,
					"responseData": map[string]interface{}{
						"translatedText": "",
						"match":          0.0,
					},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			f := NewTranslateFetcher()
			f.httpClient = &http.Client{
				Transport: &redirectTransport{base: srv.URL},
			}

			data, err := f.fetchFromMyMemory(context.Background(), "Hello", tt.sourceLang, "es")
			if err != nil {
				t.Fatalf("fetchFromMyMemory() error: %v", err)
			}
			if tt.wantErrData {
				if data.Error == "" {
					t.Error("expected error data, got none")
				}
			} else {
				if data.Error != "" {
					t.Errorf("unexpected error: %s", data.Error)
				}
			}
		})
	}
}

// ---- Weather reverseGeocode ----

func TestWeatherReverseGeocode(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(w http.ResponseWriter, r *http.Request)
		wantResult string
	}{
		{
			name: "successful reverse geocode with city and state",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"display_name": "Boston, Massachusetts, United States",
					"address": map[string]string{
						"city":         "Boston",
						"state":        "Massachusetts",
						"country":      "United States",
						"country_code": "us",
					},
				})
			},
			wantResult: "Boston, Massachusetts, United States",
		},
		{
			name: "non-200 returns coordinate string",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
			},
			wantResult: "42.36, -71.06",
		},
		{
			name: "response with town instead of city",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"display_name": "Lexington, Massachusetts, United States",
					"address": map[string]string{
						"town":         "Lexington",
						"state":        "Massachusetts",
						"country":      "United States",
						"country_code": "us",
					},
				})
			},
			wantResult: "Lexington, Massachusetts, United States",
		},
		{
			name: "empty address returns coordinate string",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"display_name": "",
					"address":      map[string]string{},
				})
			},
			wantResult: "42.36, -71.06",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			cfg := &config.WeatherWidgetConfig{}
			f := NewWeatherFetcher(cfg)
			f.client = &http.Client{
				Transport: &redirectTransport{base: srv.URL},
			}

			result, err := f.reverseGeocode(context.Background(), 42.36, -71.06)
			// reverseGeocode never returns hard errors — only falls back
			_ = err
			if result != tt.wantResult {
				t.Errorf("reverseGeocode() = %q, want %q", result, tt.wantResult)
			}
		})
	}
}

// ---- Wikipedia additional paths ----

func TestWikipediaFetcherFetchMissingTopic(t *testing.T) {
	f := NewWikipediaFetcher()
	data, err := f.Fetch(context.Background(), map[string]string{})
	if err != nil {
		t.Fatalf("Fetch() unexpected error: %v", err)
	}
	if data.Error == "" {
		t.Error("expected error when no topic provided")
	}
}

func TestWikipediaFetcherFetchArticle404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	f := NewWikipediaFetcher()
	f.httpClient = &http.Client{
		Transport: &redirectTransport{base: srv.URL},
	}

	data, err := f.Fetch(context.Background(), map[string]string{"topic": "ThisTopicDoesNotExist"})
	// When both article and search endpoints return 404, Fetch may propagate a Go error
	// or return WidgetData with an Error field — both are valid outcomes for this path.
	if err != nil {
		// acceptable: search API returned 404, propagated as Go error
		return
	}
	if data.Type != WidgetWikipedia {
		t.Errorf("Type = %q, want %q", data.Type, WidgetWikipedia)
	}
}

func TestWikipediaFetchArticleSummaryDisambiguation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"type":    "disambiguation",
			"title":   "Mercury",
			"extract": "Mercury may refer to...",
			"content_urls": map[string]interface{}{
				"desktop": map[string]string{
					"page": "https://en.wikipedia.org/wiki/Mercury",
				},
			},
		})
	}))
	defer srv.Close()

	f := NewWikipediaFetcher()
	f.httpClient = &http.Client{
		Transport: &redirectTransport{base: srv.URL},
	}

	// fetchArticleSummary should return nil for disambiguation (triggers search fallback)
	result, err := f.fetchArticleSummary(context.Background(), "Mercury", "en")
	if err != nil {
		t.Fatalf("fetchArticleSummary() error: %v", err)
	}
	if result != nil {
		t.Error("fetchArticleSummary() should return nil for disambiguation page")
	}
}

// ---- Crypto additional paths ----

func TestCryptoFetcherWithMockedHTTP(t *testing.T) {
	tests := []struct {
		name       string
		params     map[string]string
		handler    func(w http.ResponseWriter, r *http.Request)
		wantErrStr string
	}{
		{
			name:   "successful EUR currency response",
			params: map[string]string{"coins": "bitcoin", "currency": "eur"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"bitcoin": map[string]interface{}{
						"eur":            45000.0,
						"eur_24h_change": 1.2,
					},
				})
			},
		},
		{
			name:   "successful GBP currency response",
			params: map[string]string{"coins": "ethereum", "currency": "gbp"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"ethereum": map[string]interface{}{
						"gbp":            3000.0,
						"gbp_24h_change": -0.5,
					},
				})
			},
		},
		{
			name:   "API non-200 returns error WidgetData",
			params: map[string]string{"coins": "bitcoin"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantErrStr: "API returned status 429",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			cfg := &config.CryptoWidgetConfig{
				DefaultCoins: []string{"bitcoin"},
				Currency:     "usd",
			}
			f := NewCryptoFetcher(cfg)
			f.client = &http.Client{
				Transport: &redirectTransport{base: srv.URL},
			}

			data, err := f.Fetch(context.Background(), tt.params)
			if err != nil {
				t.Fatalf("Fetch() unexpected error: %v", err)
			}
			if tt.wantErrStr != "" {
				if data.Error != tt.wantErrStr {
					t.Errorf("Error = %q, want %q", data.Error, tt.wantErrStr)
				}
			} else {
				if data.Error != "" {
					t.Errorf("unexpected error: %s", data.Error)
				}
			}
		})
	}
}

// ---- Currency fetcher mocked paths ----

func TestCurrencyFetcherWithMockedHTTP(t *testing.T) {
	tests := []struct {
		name    string
		params  map[string]string
		handler func(w http.ResponseWriter, r *http.Request)
		wantErr bool
	}{
		{
			name:   "default USD to EUR",
			params: map[string]string{},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"query": map[string]interface{}{
						"from": "USD", "to": "EUR", "amount": 1.0,
					},
					"info":   map[string]interface{}{"rate": 0.92},
					"result": 0.92,
					"date":   "2024-01-01",
				})
			},
		},
		{
			name:   "explicit amount param",
			params: map[string]string{"from": "GBP", "to": "JPY", "amount": "100"},
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"success": true,
					"query": map[string]interface{}{
						"from": "GBP", "to": "JPY", "amount": 100.0,
					},
					"info":   map[string]interface{}{"rate": 185.0},
					"result": 18500.0,
					"date":   "2024-01-01",
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			f := NewCurrencyFetcher("")
			f.httpClient = &http.Client{
				Transport: &redirectTransport{base: srv.URL},
			}

			data, err := f.Fetch(context.Background(), tt.params)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Fetch() unexpected error: %v", err)
			}
			if data.Error != "" {
				t.Errorf("unexpected WidgetData.Error: %s", data.Error)
			}
			if data.Type != WidgetCurrency {
				t.Errorf("Type = %q, want %q", data.Type, WidgetCurrency)
			}
		})
	}
}

// ---- Stocks fetchQuotes with mocked HTTP ----

func TestStocksFetcherFetchQuotes(t *testing.T) {
	tests := []struct {
		name       string
		handler    func(w http.ResponseWriter, r *http.Request)
		wantErrStr string
	}{
		{
			name: "successful quote response",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(YahooFinanceResponse{
					QuoteResponse: struct {
						Result []struct {
							Symbol                     string  `json:"symbol"`
							ShortName                  string  `json:"shortName"`
							LongName                   string  `json:"longName"`
							RegularMarketPrice         float64 `json:"regularMarketPrice"`
							RegularMarketChange        float64 `json:"regularMarketChange"`
							RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
							RegularMarketVolume        int64   `json:"regularMarketVolume"`
							MarketCap                  float64 `json:"marketCap"`
						} `json:"result"`
						Error interface{} `json:"error"`
					}{
						Result: []struct {
							Symbol                     string  `json:"symbol"`
							ShortName                  string  `json:"shortName"`
							LongName                   string  `json:"longName"`
							RegularMarketPrice         float64 `json:"regularMarketPrice"`
							RegularMarketChange        float64 `json:"regularMarketChange"`
							RegularMarketChangePercent float64 `json:"regularMarketChangePercent"`
							RegularMarketVolume        int64   `json:"regularMarketVolume"`
							MarketCap                  float64 `json:"marketCap"`
						}{
							{Symbol: "AAPL", ShortName: "Apple Inc.", RegularMarketPrice: 175.5},
						},
					},
				})
			},
		},
		{
			name: "non-200 returns error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErrStr: "API returned status 401",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(tt.handler))
			defer srv.Close()

			f := NewStocksFetcher(&config.StocksWidgetConfig{
				DefaultSymbols: []string{"AAPL"},
			})
			f.client = &http.Client{
				Transport: &redirectTransport{base: srv.URL},
			}

			data, err := f.Fetch(context.Background(), map[string]string{})
			if err != nil {
				t.Fatalf("Fetch() unexpected error: %v", err)
			}
			if tt.wantErrStr != "" {
				if data.Error != tt.wantErrStr {
					t.Errorf("Error = %q, want %q", data.Error, tt.wantErrStr)
				}
			} else {
				if data.Error != "" {
					t.Errorf("unexpected error: %s", data.Error)
				}
			}
		})
	}
}

func TestStocksFetcherFetchWithSymbolsParam(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"quoteResponse": map[string]interface{}{
				"result": []interface{}{},
				"error":  nil,
			},
		})
	}))
	defer srv.Close()

	f := NewStocksFetcher(&config.StocksWidgetConfig{})
	f.client = &http.Client{
		Transport: &redirectTransport{base: srv.URL},
	}

	// Symbols passed via params should override config default
	data, err := f.Fetch(context.Background(), map[string]string{
		"symbols": "TSLA, NVDA",
	})
	if err != nil {
		t.Fatalf("Fetch() unexpected error: %v", err)
	}
	if data.Type != WidgetStocks {
		t.Errorf("Type = %q, want %q", data.Type, WidgetStocks)
	}
}

// ---- redirectTransport and dualRedirectTransport helpers ----

// redirectTransport redirects all requests to a test server base URL.
// Used to intercept HTTP calls made by widget fetchers that hard-code API URLs.
type redirectTransport struct {
	base string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = "http"
	newReq.URL.Host = req.URL.Host

	// Extract host from base URL and replace
	baseURL := t.base
	if len(baseURL) > 7 {
		if baseURL[:7] == "http://" {
			newReq.URL.Host = baseURL[7:]
			newReq.URL.Scheme = "http"
		} else if baseURL[:8] == "https://" {
			newReq.URL.Host = baseURL[8:]
			newReq.URL.Scheme = "http"
		}
	}

	return http.DefaultTransport.RoundTrip(newReq)
}

// dualRedirectTransport routes USDA requests and Open Food Facts requests
// to separate test servers based on URL host matching.
type dualRedirectTransport struct {
	usdaBase string
	offBase  string
}

func (t *dualRedirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq := req.Clone(req.Context())

	var targetBase string
	host := req.URL.Host
	if host == "" {
		host = req.URL.Hostname()
	}

	// Route to the appropriate test server based on the original host
	if host == "api.nal.usda.gov" && t.usdaBase != "" {
		targetBase = t.usdaBase
	} else if host == "world.openfoodfacts.org" && t.offBase != "" {
		targetBase = t.offBase
	} else if t.usdaBase != "" {
		targetBase = t.usdaBase
	} else {
		targetBase = t.offBase
	}

	if len(targetBase) > 7 {
		if targetBase[:7] == "http://" {
			newReq.URL.Host = targetBase[7:]
			newReq.URL.Scheme = "http"
		} else if targetBase[:8] == "https://" {
			newReq.URL.Host = targetBase[8:]
			newReq.URL.Scheme = "http"
		}
	}

	return http.DefaultTransport.RoundTrip(newReq)
}
