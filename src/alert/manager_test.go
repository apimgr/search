package alert

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// --- Create validation edge cases ---

func TestCreateRequiresQuery(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:      "",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err == nil {
		t.Fatal("Create() should reject empty query")
	}
}

func TestCreateRequiresDeliveryChannel(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:     "test",
		Category:  "general",
		Frequency: FrequencyDaily,
		Email:     "test@example.com",
		BaseURL:   "https://search.test",
	})
	if err == nil {
		t.Fatal("Create() should reject no delivery channel")
	}
}

func TestCreateRequiresEmail(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err == nil {
		t.Fatal("Create() should reject empty email")
	}
}

func TestCreateRejectsInvalidCategory(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	// ParseCategory normalizes unknown values to CategoryGeneral — no error expected
	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "INVALID_CATEGORY_XYZ",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() should normalize invalid category to general, got error: %v", err)
	}
	if resp.Alert.Category != "general" {
		t.Fatalf("Category = %q, want general", resp.Alert.Category)
	}
}

func TestCreateWebhookDeliveryRequiresURL(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:          "test",
		Category:       "general",
		Frequency:      FrequencyDaily,
		Email:          "test@example.com",
		DeliverWebhook: true,
		WebhookURL:     "",
		BaseURL:        "https://search.test",
	})
	if err == nil {
		t.Fatal("Create() should reject webhook with empty URL")
	}
}

func TestCreateWebhookDeliveryRejectsInvalidURL(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:          "test",
		Category:       "general",
		Frequency:      FrequencyDaily,
		Email:          "test@example.com",
		DeliverWebhook: true,
		WebhookURL:     "not-a-url",
		BaseURL:        "https://search.test",
	})
	if err == nil {
		t.Fatal("Create() should reject invalid webhook URL")
	}
}

func TestCreateNilDBReturnsError(t *testing.T) {
	cfg := config.DefaultConfig()
	manager := NewManager(nil, cfg, nil, nil)

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
	})
	if err == nil {
		t.Fatal("Create() with nil db should return error")
	}
}

func TestCreateNormalizesFrequency(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "general",
		Frequency:  Frequency("garbage"),
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() with bad frequency should normalize, got error: %v", err)
	}
	if resp.Alert.Frequency != FrequencyDaily {
		t.Fatalf("frequency = %q, want daily", resp.Alert.Frequency)
	}
}

func TestCreateHashesCreatedFromIP(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:         "test",
		Category:      "general",
		Frequency:     FrequencyDaily,
		Email:         "test@example.com",
		DeliverRSS:    true,
		BaseURL:       "https://search.test",
		CreatedFromIP: "192.168.1.1",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if resp.Alert.CreatedFromIP == "192.168.1.1" {
		t.Fatal("raw IP must not be stored; should be hashed")
	}
	if resp.Alert.CreatedFromIP == "" {
		t.Fatal("hashed IP must not be empty")
	}
}

// --- SetPaused ---

func TestSetPausedAndResume(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := manager.SetPaused(context.Background(), resp.ManageToken, true); err != nil {
		t.Fatalf("SetPaused(true) error: %v", err)
	}

	stored, err := manager.GetByManageToken(context.Background(), resp.ManageToken)
	if err != nil {
		t.Fatalf("GetByManageToken() error: %v", err)
	}
	if stored.Status != StatusPaused {
		t.Fatalf("status = %q after pause, want %q", stored.Status, StatusPaused)
	}
	if stored.PausedAt == nil {
		t.Fatal("PausedAt must be set after pause")
	}

	if err := manager.SetPaused(context.Background(), resp.ManageToken, false); err != nil {
		t.Fatalf("SetPaused(false) error: %v", err)
	}

	resumed, err := manager.GetByManageToken(context.Background(), resp.ManageToken)
	if err != nil {
		t.Fatalf("GetByManageToken() after resume error: %v", err)
	}
	if resumed.Status != StatusActive {
		t.Fatalf("status = %q after resume, want %q", resumed.Status, StatusActive)
	}
}

// --- Delete ---

func TestDeleteRemovesAlert(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := manager.Delete(context.Background(), resp.ManageToken); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	_, err = manager.GetByManageToken(context.Background(), resp.ManageToken)
	if err == nil {
		t.Fatal("GetByManageToken() should return error after delete")
	}
}

func TestDeleteUnknownTokenReturnsNotFound(t *testing.T) {
	manager, db := newTestManager(t)
	defer db.Close()

	err := manager.Delete(context.Background(), "does-not-exist-token")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Delete() on missing token should return ErrNotFound, got %v", err)
	}
}

// --- GetByRSSToken ---

func TestGetByRSSToken(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "privacy",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	byRSS, err := manager.GetByRSSToken(context.Background(), resp.RSSToken)
	if err != nil {
		t.Fatalf("GetByRSSToken() error: %v", err)
	}
	if byRSS.ID != resp.Alert.ID {
		t.Fatalf("GetByRSSToken() returned wrong alert ID")
	}
}

func TestGetByRSSTokenNotFound(t *testing.T) {
	manager, db := newTestManager(t)
	defer db.Close()

	_, err := manager.GetByRSSToken(context.Background(), "bad-rss-token")
	if err == nil {
		t.Fatal("GetByRSSToken() should return error for unknown token")
	}
}

// --- Feed and FeedXML ---

func TestFeedReturnsItems(t *testing.T) {
	google := newTestEngine("google", "general")
	google.results = []model.Result{
		{URL: "https://example.com/a", Title: "Alpha", Engine: "google"},
		{URL: "https://example.com/b", Title: "Beta", Engine: "google"},
	}

	manager, db := newTestManager(t, google)
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue() error: %v", err)
	}

	feed, err := manager.Feed(context.Background(), resp.RSSToken, 10)
	if err != nil {
		t.Fatalf("Feed() error: %v", err)
	}
	if len(feed.Items) != 2 {
		t.Fatalf("Feed() returned %d items, want 2", len(feed.Items))
	}
}

func TestFeedXMLIsValidXML(t *testing.T) {
	google := newTestEngine("google", "general")
	google.results = []model.Result{
		{URL: "https://example.com/rss-item", Title: "RSS Item", Engine: "google"},
	}

	manager, db := newTestManager(t, google)
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "rss test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue() error: %v", err)
	}

	xmlData, err := manager.FeedXML(context.Background(), resp.RSSToken, 50)
	if err != nil {
		t.Fatalf("FeedXML() error: %v", err)
	}
	if !strings.HasPrefix(string(xmlData), "<?xml") {
		t.Fatal("FeedXML() must begin with XML declaration")
	}
	if !strings.Contains(string(xmlData), "<rss") {
		t.Fatal("FeedXML() must contain <rss> element")
	}
	if !strings.Contains(string(xmlData), "rss test") {
		t.Fatal("FeedXML() must contain the alert query")
	}
}

func TestFeedXMLBadTokenReturnsError(t *testing.T) {
	manager, db := newTestManager(t)
	defer db.Close()

	_, err := manager.FeedXML(context.Background(), "bad-token", 50)
	if err == nil {
		t.Fatal("FeedXML() should return error for unknown token")
	}
}

func TestFeedNegativeLimitDefaultsTo50(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// negative limit should not error
	_, err = manager.Feed(context.Background(), resp.RSSToken, -1)
	if err != nil {
		t.Fatalf("Feed() with -1 limit should not error: %v", err)
	}
}

// --- ProcessDue edge cases ---

func TestProcessDueWithNoAlerts(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue() with no alerts should not error: %v", err)
	}
}

func TestProcessDueInvalidFrequencyIsNoop(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	if err := manager.ProcessDue(context.Background(), Frequency("never")); err != nil {
		t.Fatalf("ProcessDue() with invalid frequency should be a no-op, got: %v", err)
	}
}

func TestProcessDueWithNilAggregator(t *testing.T) {
	db := setupAlertTestDB(t)
	defer db.Close()
	cfg := config.DefaultConfig()
	manager := NewManager(db, cfg, nil, nil)

	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue() with nil aggregator should not error: %v", err)
	}
}

func TestProcessDueWithSearchError(t *testing.T) {
	google := newTestEngine("google", "general")
	google.searchErr = model.ErrEngineUnavailable

	manager, db := newTestManager(t, google)
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:      "error test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// The aggregator converts all engine errors to ErrNoResults when no results are found.
	// processAlert treats ErrNoResults as "no results" (not a failure), so ProcessDue returns nil.
	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue() should not return error when engine returns ErrNoResults: %v", err)
	}
}

// --- CleanupResults ---

func TestCleanupResultsWithNilDB(t *testing.T) {
	cfg := config.DefaultConfig()
	manager := NewManager(nil, cfg, nil, nil)

	if err := manager.CleanupResults(context.Background()); err != nil {
		t.Fatalf("CleanupResults() with nil db should not error: %v", err)
	}
}

func TestCleanupResultsRunsWithoutError(t *testing.T) {
	google := newTestEngine("google", "general")
	google.results = []model.Result{
		{URL: "https://example.com/cleanup", Title: "Cleanup", Engine: "google"},
	}

	manager, db := newTestManager(t, google)
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:      "cleanup",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue() error: %v", err)
	}

	if err := manager.CleanupResults(context.Background()); err != nil {
		t.Fatalf("CleanupResults() error: %v", err)
	}
}

// --- Update validation ---

func TestUpdateRejectsUnknownEngine(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"), newTestEngine("bing", "news"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "update test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	_, err = manager.Update(context.Background(), resp.ManageToken, UpdateRequest{
		Query:      "updated",
		Category:   "general",
		Frequency:  FrequencyWeekly,
		Engines:    []string{"unknown_engine"},
		DeliverRSS: true,
	})
	if err == nil {
		t.Fatal("Update() should reject unknown engine")
	}
}

func TestUpdateRejectsNoDeliveryChannel(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:      "channel test",
		Category:   "general",
		Frequency:  FrequencyDaily,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	_, err = manager.Update(context.Background(), resp.ManageToken, UpdateRequest{
		Query:          "updated",
		Category:       "general",
		Frequency:      FrequencyDaily,
		DeliverEmail:   false,
		DeliverRSS:     false,
		DeliverWebhook: false,
	})
	if err == nil {
		t.Fatal("Update() should reject when no delivery channel selected")
	}
}

func TestUpdateWithBadManageTokenReturnsError(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.Update(context.Background(), "bad-token", UpdateRequest{
		Query:      "x",
		Category:   "general",
		DeliverRSS: true,
	})
	if err == nil {
		t.Fatal("Update() with bad token should return error")
	}
}

// --- Rate limiting ---

func TestCreateRateLimitExceeded(t *testing.T) {
	db := setupAlertTestDB(t)
	defer db.Close()
	cfg := config.DefaultConfig()
	// Set a tight rate limit of 1 per hour
	cfg.Search.Alerts.CreateRateLimitPerHour = 1

	aggregator := search.NewAggregator(
		[]search.Engine{newTestEngine("google", "general")},
		search.AggregatorConfig{Timeout: 5 * time.Second, MaxConcurrent: 1},
	)
	manager := NewManager(db, cfg, aggregator, nil)

	req := CreateRequest{
		Query:         "rate limit test",
		Category:      "general",
		Frequency:     FrequencyDaily,
		Email:         "rate@example.com",
		DeliverRSS:    true,
		BaseURL:       "https://search.test",
		CreatedFromIP: "10.0.0.1",
	}

	_, err := manager.Create(context.Background(), req)
	if err != nil {
		t.Fatalf("first Create() should succeed: %v", err)
	}

	// IP is hashed after first call; must hash manually for the same bucket
	_, err = manager.Create(context.Background(), req)
	if err == nil {
		t.Fatal("second Create() from same IP should be rate-limited")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Fatalf("expected rate-limit error, got: %v", err)
	}
}

// --- normalizeAlertEngines corner cases ---

func TestNormalizeAlertEnginesEmptyInput(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	result, err := manager.normalizeAlertEngines(nil)
	if err != nil {
		t.Fatalf("normalizeAlertEngines(nil) error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty slice, got %v", result)
	}
}

func TestNormalizeAlertEnginesDeduplicates(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	result, err := manager.normalizeAlertEngines([]string{"google", "google", "google"})
	if err != nil {
		t.Fatalf("normalizeAlertEngines error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 deduped engine, got %d: %v", len(result), result)
	}
}

func TestNormalizeAlertEnginesCommaSeparated(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"), newTestEngine("bing", "general"))
	defer db.Close()

	result, err := manager.normalizeAlertEngines([]string{"google,bing"})
	if err != nil {
		t.Fatalf("normalizeAlertEngines error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 engines from comma-separated, got %d: %v", len(result), result)
	}
}

// --- Utility functions ---

func TestNormalizeAlertLanguageEmpty(t *testing.T) {
	if got := normalizeAlertLanguage(""); got != "en" {
		t.Fatalf("normalizeAlertLanguage('') = %q, want 'en'", got)
	}
}

func TestNormalizeAlertLanguageLowercases(t *testing.T) {
	if got := normalizeAlertLanguage("FR"); got != "fr" {
		t.Fatalf("normalizeAlertLanguage('FR') = %q, want 'fr'", got)
	}
}

func TestNormalizeAlertRegionLowercases(t *testing.T) {
	if got := normalizeAlertRegion("US"); got != "us" {
		t.Fatalf("normalizeAlertRegion('US') = %q, want 'us'", got)
	}
}

func TestNormalizeSafeSearchClampsRange(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{-1, 1},
		{0, 0},
		{1, 1},
		{2, 2},
		{3, 1},
	}
	for _, tt := range tests {
		if got := normalizeSafeSearch(tt.input); got != tt.want {
			t.Errorf("normalizeSafeSearch(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestNormalizePerPage(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0, 20},
		{-5, 20},
		{50, 50},
		{101, 100},
		{1, 1},
	}
	for _, tt := range tests {
		if got := normalizePerPage(tt.input); got != tt.want {
			t.Errorf("normalizePerPage(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestIsValidFrequency(t *testing.T) {
	tests := []struct {
		input Frequency
		want  bool
	}{
		{FrequencyImmediate, true},
		{FrequencyDaily, true},
		{FrequencyWeekly, true},
		{Frequency("monthly"), false},
		{Frequency(""), false},
	}
	for _, tt := range tests {
		if got := isValidFrequency(tt.input); got != tt.want {
			t.Errorf("isValidFrequency(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestBoolToInt(t *testing.T) {
	if boolToInt(true) != 1 {
		t.Error("boolToInt(true) should be 1")
	}
	if boolToInt(false) != 0 {
		t.Error("boolToInt(false) should be 0")
	}
}

func TestHashTokenHexIsDeterministic(t *testing.T) {
	h1 := hashTokenHex("my-token")
	h2 := hashTokenHex("my-token")
	if h1 != h2 {
		t.Fatal("hashTokenHex must be deterministic")
	}
	if hashTokenHex("a") == hashTokenHex("b") {
		t.Fatal("hashTokenHex must differ for different inputs")
	}
}

func TestMarshalUnmarshalAlertEngines(t *testing.T) {
	tests := []struct {
		engines []string
	}{
		{nil},
		{[]string{}},
		{[]string{"google"}},
		{[]string{"google", "bing", "ddg"}},
	}
	for _, tt := range tests {
		raw := marshalAlertEngines(tt.engines)
		got := unmarshalAlertEngines(raw)
		// nil and empty are both acceptable as "no engines"
		if len(got) != len(tt.engines) {
			t.Errorf("marshal/unmarshal round-trip: input=%v got=%v", tt.engines, got)
		}
	}
}

func TestUnmarshalAlertEnginesCommaFallback(t *testing.T) {
	// Non-JSON comma-separated fallback
	got := unmarshalAlertEngines("google,bing")
	if len(got) != 2 || got[0] != "google" || got[1] != "bing" {
		t.Fatalf("comma fallback = %v, want [google bing]", got)
	}
}

func TestNullableTime(t *testing.T) {
	if nullableTime(time.Time{}) != nil {
		t.Fatal("nullableTime(zero) should return nil")
	}
	now := time.Now()
	if nullableTime(now) == nil {
		t.Fatal("nullableTime(non-zero) should return non-nil")
	}
}

func TestPtrTime(t *testing.T) {
	now := time.Now()
	p := ptrTime(now)
	if p == nil {
		t.Fatal("ptrTime should not return nil")
	}
	if !p.Equal(now) {
		t.Fatalf("ptrTime: got %v, want %v", p, now)
	}
}

// --- Webhook delivery (via httptest server) ---

func TestSendWebhookSucceeds(t *testing.T) {
	allowLoopbackWebhooks = true
	defer func() { allowLoopbackWebhooks = false }()

	received := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	google := newTestEngine("google", "general")
	google.results = []model.Result{{URL: "https://example.com", Title: "Result", Engine: "google"}}

	manager, db := newTestManager(t, google)
	defer db.Close()

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:          "webhook",
		Category:       "general",
		Frequency:      FrequencyDaily,
		Email:          "test@example.com",
		DeliverWebhook: true,
		WebhookURL:     srv.URL,
		BaseURL:        "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	alertRow, err := manager.GetByManageToken(context.Background(), resp.ManageToken)
	if err != nil {
		t.Fatalf("GetByManageToken() error: %v", err)
	}

	// Insert a result so webhook has something to deliver
	result := model.Result{URL: "https://example.com/new", Title: "New", Engine: "google"}
	if err := manager.insertResult(context.Background(), alertRow.ID, result); err != nil {
		t.Fatalf("insertResult() error: %v", err)
	}

	if err := manager.processAlert(context.Background(), alertRow); err != nil {
		t.Fatalf("processAlert() error: %v", err)
	}

	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("webhook server never received a request")
	}
}

func TestSendWebhookFailsAndReturnsError(t *testing.T) {
	allowLoopbackWebhooks = true
	defer func() { allowLoopbackWebhooks = false }()

	// Server that always returns 500
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	google := newTestEngine("google", "general")
	manager, db := newTestManager(t, google)
	defer db.Close()

	cfg := config.DefaultConfig()
	cfg.Search.Alerts.WebhookMaxRetries = 1
	cfg.Search.Alerts.WebhookRetryDelayMinutes = 0

	resp, err := manager.Create(context.Background(), CreateRequest{
		Query:          "webhook fail",
		Category:       "general",
		Frequency:      FrequencyDaily,
		Email:          "test@example.com",
		DeliverWebhook: true,
		WebhookURL:     srv.URL,
		BaseURL:        "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	alertRow, err := manager.GetByManageToken(context.Background(), resp.ManageToken)
	if err != nil {
		t.Fatalf("GetByManageToken() error: %v", err)
	}

	result := model.Result{URL: "https://example.com/fail", Title: "Fail", Engine: "google"}
	if err := manager.insertResult(context.Background(), alertRow.ID, result); err != nil {
		t.Fatalf("insertResult() error: %v", err)
	}

	// Override client retries to avoid slow sleep
	manager.serverConfig.Search.Alerts.WebhookMaxRetries = 1
	manager.serverConfig.Search.Alerts.WebhookRetryDelayMinutes = 0

	err = manager.sendWebhook(context.Background(), alertRow, []AlertResult{
		{ID: "r1", Title: "Fail", URL: "https://example.com/fail", Engine: "google"},
	})
	if err == nil {
		t.Fatal("sendWebhook() to failing server should return error")
	}
}

// --- markResultsDelivered corner cases ---

func TestMarkResultsDeliveredInvalidColumnReturnsError(t *testing.T) {
	manager, db := newTestManager(t)
	defer db.Close()

	err := manager.markResultsDelivered(context.Background(), []AlertResult{{ID: "x"}}, "invalid_column")
	if err == nil {
		t.Fatal("markResultsDelivered() with invalid column should return error")
	}
}

func TestMarkResultsDeliveredEmptySliceIsNoop(t *testing.T) {
	manager, db := newTestManager(t)
	defer db.Close()

	if err := manager.markResultsDelivered(context.Background(), nil, "notified_email_at"); err != nil {
		t.Fatalf("markResultsDelivered() with nil slice should not error: %v", err)
	}
}

// --- fingerprintResult ---

func TestFingerprintResultIsDeterministic(t *testing.T) {
	r := model.Result{URL: "https://example.com", Title: "Test", Engine: "google", PublishedAt: time.Time{}}
	f1 := fingerprintResult(r)
	f2 := fingerprintResult(r)
	if f1 != f2 {
		t.Fatal("fingerprintResult must be deterministic")
	}
	r2 := model.Result{URL: "https://other.com", Title: "Other", Engine: "bing", PublishedAt: time.Time{}}
	if fingerprintResult(r) == fingerprintResult(r2) {
		t.Fatal("fingerprintResult must differ for distinct results")
	}
}

// --- alertToken / manageURL (private methods exercised via public surface) ---

func TestAlertTokenInvalidColumnReturnsError(t *testing.T) {
	manager, db := newTestManager(t, newTestEngine("google", "general"))
	defer db.Close()

	_, err := manager.alertToken(context.Background(), "any-id", "invalid_column")
	if err == nil {
		t.Fatal("alertToken() with invalid column should return error")
	}
}

// --- encryptAlertToken / decryptAlertToken ---

func TestEncryptDecryptAlertTokenRoundTrip(t *testing.T) {
	key := "testsecretkey1234567890123456789012"
	token := "my-plain-token"

	enc, err := encryptAlertToken(token, key)
	if err != nil {
		t.Fatalf("encryptAlertToken() error: %v", err)
	}
	if enc == token {
		t.Fatal("encrypted value must differ from plaintext")
	}

	dec, err := decryptAlertToken(enc, key)
	if err != nil {
		t.Fatalf("decryptAlertToken() error: %v", err)
	}
	if dec != token {
		t.Fatalf("decryptAlertToken() = %q, want %q", dec, token)
	}
}

func TestEncryptAlertTokenEmptyReturnsEmpty(t *testing.T) {
	enc, err := encryptAlertToken("", "key")
	if err != nil {
		t.Fatalf("encryptAlertToken('') error: %v", err)
	}
	if enc != "" {
		t.Fatalf("encryptAlertToken('') must return '', got %q", enc)
	}
}

func TestDecryptAlertTokenEmptyReturnsEmpty(t *testing.T) {
	dec, err := decryptAlertToken("", "key")
	if err != nil {
		t.Fatalf("decryptAlertToken('') error: %v", err)
	}
	if dec != "" {
		t.Fatalf("decryptAlertToken('') must return '', got %q", dec)
	}
}

// --- randomToken ---

func TestRandomTokenLength(t *testing.T) {
	tok, err := randomToken(16)
	if err != nil {
		t.Fatalf("randomToken(16) error: %v", err)
	}
	// hex encoding: 16 bytes → 32 hex chars
	if len(tok) != 32 {
		t.Fatalf("randomToken(16) length = %d, want 32", len(tok))
	}
}

func TestRandomTokenIsUnique(t *testing.T) {
	t1, _ := randomToken(16)
	t2, _ := randomToken(16)
	if t1 == t2 {
		t.Fatal("randomToken must return unique values")
	}
}

// --- ProcessDue with all frequencies ---

func TestProcessDueWeeklyFrequency(t *testing.T) {
	google := newTestEngine("google", "general")
	google.results = []model.Result{{URL: "https://weekly.example.com", Title: "Weekly", Engine: "google"}}

	manager, db := newTestManager(t, google)
	defer db.Close()

	_, err := manager.Create(context.Background(), CreateRequest{
		Query:      "weekly",
		Category:   "general",
		Frequency:  FrequencyWeekly,
		Email:      "test@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	// ProcessDue with weekly — only weekly alerts are queried
	if err := manager.ProcessDue(context.Background(), FrequencyWeekly); err != nil {
		t.Fatalf("ProcessDue(weekly) error: %v", err)
	}
	if google.searches != 1 {
		t.Fatalf("google.searches = %d, want 1", google.searches)
	}

	// ProcessDue with daily — should NOT pick up a weekly alert
	google.searches = 0
	if err := manager.ProcessDue(context.Background(), FrequencyDaily); err != nil {
		t.Fatalf("ProcessDue(daily) error: %v", err)
	}
	if google.searches != 0 {
		t.Fatalf("ProcessDue(daily) should not run weekly alert, searches = %d", google.searches)
	}
}

// --- JSON round-trip sanity on AlertResult ---

func TestAlertResultJSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ar := AlertResult{
		ID:          "test-id",
		Title:       "Test Title",
		URL:         "https://example.com",
		Content:     "some content",
		Engine:      "google",
		FirstSeenAt: now,
	}
	data, err := json.Marshal(ar)
	if err != nil {
		t.Fatalf("json.Marshal(AlertResult) error: %v", err)
	}
	var got AlertResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal(AlertResult) error: %v", err)
	}
	if got.ID != ar.ID || got.Title != ar.Title || got.URL != ar.URL {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}
