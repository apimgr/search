package alert

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	_ "modernc.org/sqlite"
)

type testEngine struct {
	*search.BaseEngine
	results   []model.Result
	searchErr error
	searches  int
	lastQuery *model.Query
}

func newTestEngine(name string, categories ...string) *testEngine {
	return &testEngine{
		BaseEngine: search.NewBaseEngine(&model.EngineConfig{
			Name:        name,
			DisplayName: name,
			Enabled:     true,
			Priority:    100,
			Categories:  categories,
		}),
	}
}

func (e *testEngine) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	e.searches++
	copyQuery := *query
	copyQuery.Engines = append([]string(nil), query.Engines...)
	e.lastQuery = &copyQuery
	if e.searchErr != nil {
		return nil, e.searchErr
	}
	return e.results, nil
}

func setupAlertTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	schema := `
		CREATE TABLE search_alerts (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			query TEXT NOT NULL,
			category TEXT NOT NULL,
			safe_search INTEGER NOT NULL DEFAULT 1,
			frequency TEXT NOT NULL,
			deliver_email INTEGER NOT NULL DEFAULT 0,
			deliver_rss INTEGER NOT NULL DEFAULT 1,
			deliver_webhook INTEGER NOT NULL DEFAULT 0,
			webhook_url TEXT,
			webhook_secret_encrypted TEXT,
			email_verified INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'pending',
			verify_token_hash TEXT,
			verify_token_expires DATETIME,
			manage_token_hash TEXT NOT NULL,
			rss_token_hash TEXT NOT NULL,
			manage_token_encrypted TEXT NOT NULL DEFAULT '',
			rss_token_encrypted TEXT NOT NULL DEFAULT '',
			base_url TEXT,
			last_checked_at DATETIME,
			last_sent_at DATETIME,
			last_error TEXT,
			created_from_ip TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			verified_at DATETIME,
			paused_at DATETIME,
			language TEXT NOT NULL DEFAULT 'en',
			region TEXT NOT NULL DEFAULT '',
			engines_json TEXT NOT NULL DEFAULT '[]'
		);
		CREATE UNIQUE INDEX idx_search_alerts_manage_token ON search_alerts(manage_token_hash);
		CREATE UNIQUE INDEX idx_search_alerts_rss_token ON search_alerts(rss_token_hash);
		CREATE INDEX idx_search_alerts_verify_token ON search_alerts(verify_token_hash);
		CREATE INDEX idx_search_alerts_status_frequency ON search_alerts(status, frequency);

		CREATE TABLE search_alert_results (
			id TEXT PRIMARY KEY,
			alert_id TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			title TEXT NOT NULL,
			url TEXT NOT NULL,
			content TEXT,
			engine TEXT,
			published_at DATETIME,
			first_seen_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			notified_email_at DATETIME,
			notified_webhook_at DATETIME,
			FOREIGN KEY (alert_id) REFERENCES search_alerts(id) ON DELETE CASCADE,
			UNIQUE(alert_id, fingerprint)
		);
		CREATE INDEX idx_search_alert_results_alert ON search_alert_results(alert_id, first_seen_at);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}

func newTestManager(t *testing.T, engines ...search.Engine) (*Manager, *sql.DB) {
	t.Helper()

	db := setupAlertTestDB(t)
	cfg := config.DefaultConfig()
	aggregator := search.NewAggregator(engines, search.AggregatorConfig{
		Timeout:       5 * time.Second,
		CacheEnabled:  false,
		MaxConcurrent: len(engines),
	})

	return NewManager(db, cfg, aggregator, nil), db
}

func TestCreateStoresAlertFilterContext(t *testing.T) {
	google := newTestEngine("google", "general", "news")
	bing := newTestEngine("bing", "general", "news")
	manager, db := newTestManager(t, google, bing)
	defer db.Close()

	created, err := manager.Create(context.Background(), CreateRequest{
		Query:         "privacy search",
		Category:      "news",
		Language:      "DE",
		Region:        "US",
		Engines:       []string{"Google", "bing"},
		SafeSearch:    2,
		Frequency:     FrequencyDaily,
		Email:         "alerts@example.com",
		DeliverRSS:    true,
		BaseURL:       "https://search.test",
		CreatedFromIP: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if created.Alert.Language != "de" {
		t.Fatalf("Language = %q, want de", created.Alert.Language)
	}
	if created.Alert.Region != "us" {
		t.Fatalf("Region = %q, want us", created.Alert.Region)
	}
	if len(created.Alert.Engines) != 2 || created.Alert.Engines[0] != "google" || created.Alert.Engines[1] != "bing" {
		t.Fatalf("Engines = %#v, want [google bing]", created.Alert.Engines)
	}

	stored, err := manager.GetByManageToken(context.Background(), created.ManageToken)
	if err != nil {
		t.Fatalf("GetByManageToken() error = %v", err)
	}
	if stored.Language != "de" || stored.Region != "us" {
		t.Fatalf("stored filters = %q/%q, want de/us", stored.Language, stored.Region)
	}
	if len(stored.Engines) != 2 {
		t.Fatalf("stored engines = %#v, want 2 entries", stored.Engines)
	}
}

func TestCreateRejectsUnknownAlertEngine(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:      "privacy",
		Category:   "general",
		Language:   "en",
		Engines:    []string{"unknown"},
		Frequency:  FrequencyDaily,
		Email:      "alerts@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err == nil {
		t.Fatal("Create() should reject unknown engine")
	}
}

func TestCreateStoresDistinctPrivateTokens(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	created, err := manager.Create(context.Background(), CreateRequest{
		Query:      "privacy search",
		Category:   "general",
		Language:   "en",
		Frequency:  FrequencyDaily,
		Email:      "alerts@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ManageToken == created.RSSToken {
		t.Fatal("Create() should generate distinct manage and RSS tokens")
	}

	rssToken, err := manager.RSSTokenForManageToken(context.Background(), created.ManageToken)
	if err != nil {
		t.Fatalf("RSSTokenForManageToken() error = %v", err)
	}
	if rssToken != created.RSSToken {
		t.Fatalf("RSSTokenForManageToken() = %q, want %q", rssToken, created.RSSToken)
	}
}

func TestUpdateReplacesAlertFilterContext(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"), newTestEngine("bing", "news"))
	defer db.Close()

	created, err := manager.Create(context.Background(), CreateRequest{
		Query:      "privacy",
		Category:   "general",
		Language:   "en",
		Frequency:  FrequencyDaily,
		Email:      "alerts@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := manager.Update(context.Background(), created.ManageToken, UpdateRequest{
		Query:      "federated search",
		Category:   "news",
		Language:   "fr",
		Region:     "ca",
		Engines:    []string{"bing"},
		Frequency:  FrequencyWeekly,
		SafeSearch: 0,
		DeliverRSS: true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.Query != "federated search" || updated.Category != "news" {
		t.Fatalf("updated query/category = %q/%q", updated.Query, updated.Category)
	}
	if updated.Language != "fr" || updated.Region != "ca" {
		t.Fatalf("updated language/region = %q/%q", updated.Language, updated.Region)
	}
	if len(updated.Engines) != 1 || updated.Engines[0] != "bing" {
		t.Fatalf("updated engines = %#v, want [bing]", updated.Engines)
	}
}

func TestProcessDueUsesStoredAlertFilters(t *testing.T) {
	google := newTestEngine("google", "general")
	google.results = []model.Result{{URL: "https://example.com/1", Title: "One", Engine: "google"}}
	bing := newTestEngine("bing", "general")
	bing.results = []model.Result{{URL: "https://example.com/2", Title: "Two", Engine: "bing"}}

	manager, db := newTestManager(t, google, bing)
	defer db.Close()

	created, err := manager.Create(context.Background(), CreateRequest{
		Query:      "privacy",
		Category:   "general",
		Language:   "fr",
		Region:     "ca",
		Engines:    []string{"google"},
		Frequency:  FrequencyDaily,
		Email:      "alerts@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Alert.Status != StatusActive {
		t.Fatalf("alert status = %q, want active", created.Alert.Status)
	}

	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue() error = %v", err)
	}

	if google.searches != 1 {
		t.Fatalf("google searches = %d, want 1", google.searches)
	}
	if bing.searches != 0 {
		t.Fatalf("bing searches = %d, want 0", bing.searches)
	}
	if google.lastQuery == nil {
		t.Fatal("expected google to receive a query")
	}
	if google.lastQuery.Language != "fr" || google.lastQuery.Region != "ca" {
		t.Fatalf("query language/region = %q/%q, want fr/ca", google.lastQuery.Language, google.lastQuery.Region)
	}
	if len(google.lastQuery.Engines) != 1 || google.lastQuery.Engines[0] != "google" {
		t.Fatalf("query engines = %#v, want [google]", google.lastQuery.Engines)
	}
}
