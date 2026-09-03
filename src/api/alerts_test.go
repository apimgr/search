package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	_ "modernc.org/sqlite"
)

type alertTestEngine struct {
	*search.BaseEngine
}

func newAlertTestEngine(name string, categories ...string) *alertTestEngine {
	return &alertTestEngine{
		BaseEngine: search.NewBaseEngine(&model.EngineConfig{
			Name:        name,
			DisplayName: name,
			Enabled:     true,
			Priority:    100,
			Categories:  categories,
		}),
	}
}

func (e *alertTestEngine) Search(ctx context.Context, query *model.Query) ([]model.Result, error) {
	return []model.Result{}, nil
}

func setupAlertAPITestDB(t *testing.T) *sql.DB {
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

func newAlertAPIHandler(t *testing.T) (*Handler, *alert.Manager, *sql.DB) {
	t.Helper()

	db := setupAlertAPITestDB(t)
	cfg := config.DefaultConfig()
	engines := []search.Engine{newAlertTestEngine("google", "general")}
	aggregator := search.NewAggregator(engines, search.AggregatorConfig{
		Timeout:       5 * time.Second,
		CacheEnabled:  false,
		MaxConcurrent: len(engines),
	})

	handler := NewHandler(cfg, nil, aggregator)
	manager := alert.NewManager(db, cfg, aggregator, nil)
	handler.SetAlertManager(manager)

	return handler, manager, db
}

func TestHandleAlertPauseCanResume(t *testing.T) {
	handler, manager, db := newAlertAPIHandler(t)
	defer db.Close()

	created, err := manager.Create(context.Background(), alert.CreateRequest{
		Query:      "privacy search",
		Category:   "general",
		Language:   "en",
		Frequency:  alert.FrequencyDaily,
		Email:      "alerts@example.com",
		DeliverRSS: true,
		BaseURL:    "https://search.test",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := manager.SetPaused(context.Background(), created.ManageToken, true); err != nil {
		t.Fatalf("SetPaused(true) error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, APIPrefix+"/alerts/"+created.ManageToken+"/pause", bytes.NewBufferString(`{"paused":false}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK {
		t.Fatal("response OK = false, want true")
	}

	alertInfo, err := manager.GetByManageToken(context.Background(), created.ManageToken)
	if err != nil {
		t.Fatalf("GetByManageToken() error = %v", err)
	}
	if alertInfo.Status != alert.StatusActive {
		t.Fatalf("Status = %q, want %q", alertInfo.Status, alert.StatusActive)
	}
}
