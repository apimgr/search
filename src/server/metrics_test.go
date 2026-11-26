package server

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/version"
	"github.com/go-chi/chi/v5"
)

// TestMetricsHandlerServesOwnRegistry is the regression guard for the private
// registry in NewMetrics. Registering on the process-wide default made a
// second Server in one process panic on duplicate collector registration;
// scoping each Metrics to its own registry fixes that, but it would silently
// serve an empty /server/metrics if Handler() still gathered from the default
// gatherer. This asserts the served output actually contains this instance's
// collectors.
func TestMetricsHandlerServesOwnRegistry(t *testing.T) {
	m := NewMetrics(config.DefaultConfig())
	m.RecordRequest(http.MethodGet, "/", http.StatusOK, 10*time.Millisecond, 100, 200)
	m.RecordSearch("web", 20*time.Millisecond)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/server/metrics", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("metrics handler status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{"search_http_requests_total", "search_searches_total"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output is missing %q — the handler is not gathering from the instance registry", want)
		}
	}
}

func TestMetricsRequestStatsPersistAndRefreshFromSharedDatabase(t *testing.T) {
	db, err := database.NewDB(&database.Config{Driver: "sqlite", DSN: ":memory:", MaxOpen: 1, MaxIdle: 1, Lifetime: 60})
	if err != nil {
		t.Fatalf("database.NewDB(): %v", err)
	}
	defer db.Close()
	table := db.RequestStatsTable()
	if _, err := db.Exec(context.Background(), `CREATE TABLE `+table+` (hour INTEGER PRIMARY KEY, request_count INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create request stats table: %v", err)
	}
	m := NewMetrics(config.DefaultConfig())
	if err := m.SetRequestStatsDB(context.Background(), db); err != nil {
		t.Fatalf("SetRequestStatsDB(): %v", err)
	}
	now := time.Now().UTC().Truncate(time.Hour)
	m.recordRequestAt(http.MethodGet, "/search", http.StatusOK, time.Millisecond, 0, 0, now, true)
	if err := m.PersistRequestStats(context.Background()); err != nil {
		t.Fatalf("PersistRequestStats(): %v", err)
	}
	if got := m.GetTotalRequests(); got != 1 {
		t.Fatalf("persisted total = %d, want 1", got)
	}
	if got := m.GetRequestsLast24Hours(); got != 1 {
		t.Fatalf("persisted trailing count = %d, want 1", got)
	}

	// Simulate another process writing the shared hourly row. A scheduled
	// refresh with no local requests must still observe it.
	hour := now.Unix()
	if err := db.Write(context.Background(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(context.Background(), `INSERT INTO `+table+` (hour, request_count) VALUES (?, 2) ON CONFLICT(hour) DO UPDATE SET request_count = request_count + excluded.request_count`, hour)
		return err
	}); err != nil {
		t.Fatalf("simulate external writer update: %v", err)
	}
	if err := m.PersistRequestStats(context.Background()); err != nil {
		t.Fatalf("refresh shared request stats: %v", err)
	}
	if got := m.GetTotalRequests(); got != 3 {
		t.Fatalf("shared total = %d, want 3", got)
	}
	if got := m.GetRequestsLast24Hours(); got != 3 {
		t.Fatalf("shared trailing count = %d, want 3", got)
	}
}

func TestHealthAndMetricsScrapesAreExcludedFromRequestTotals(t *testing.T) {
	for _, path := range []string{
		"/server/healthz",
		"/api/v1/server/healthz.txt",
		"/server/metrics",
		"/server/metrics/prometheus",
		"/server/metrics/grafana",
		"/server/metrics/loki",
		version.APIPrefix + "/server/metrics/prometheus",
		"/api/metrics/grafana",
		"/metrics/loki",
	} {
		if !isHealthOrMetricsPath(path) {
			t.Errorf("isHealthOrMetricsPath(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"/search", "/server/metrics-extra", "/metrics-old"} {
		if isHealthOrMetricsPath(path) {
			t.Errorf("isHealthOrMetricsPath(%q) = true, want false", path)
		}
	}
}

func TestCPUUsageFromCounterDelta(t *testing.T) {
	tests := []struct {
		name                        string
		previousTotal, previousIdle uint64
		total, idle                 uint64
		want                        float64
		valid                       bool
	}{
		{name: "half busy", previousTotal: 100, previousIdle: 50, total: 200, idle: 100, want: 50, valid: true},
		{name: "all idle", previousTotal: 100, previousIdle: 50, total: 200, idle: 150, want: 0, valid: true},
		{name: "counter reset", previousTotal: 200, previousIdle: 50, total: 100, idle: 60, valid: false},
		{name: "invalid idle delta", previousTotal: 100, previousIdle: 10, total: 200, idle: 150, valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, valid := cpuUsageFromCounterDelta(test.previousTotal, test.previousIdle, test.total, test.idle)
			if valid != test.valid || got != test.want {
				t.Fatalf("cpuUsageFromCounterDelta() = (%v, %v), want (%v, %v)", got, valid, test.want, test.valid)
			}
		})
	}
}

// TestMetricsRegistryIsPerInstance pins the isolation the private registry
// exists for: two Metrics in one process must both register cleanly and must
// not see each other's series.
func TestMetricsRegistryIsPerInstance(t *testing.T) {
	a := NewMetrics(config.DefaultConfig())
	b := NewMetrics(config.DefaultConfig())

	a.RecordRequest(http.MethodGet, "/a", http.StatusOK, time.Millisecond, 1, 1)

	rec := httptest.NewRecorder()
	b.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/server/metrics", nil))

	if strings.Contains(rec.Body.String(), `path="/a"`) {
		t.Error("instance b served instance a's series; the registries are shared")
	}
}

func TestMetricsRequestsLast24Hours(t *testing.T) {
	m := NewMetrics(config.DefaultConfig())
	now := time.Now()
	m.recordRequestAt(http.MethodGet, "/old", http.StatusOK, time.Millisecond, 0, 0, now.Add(-25*time.Hour), true)
	m.recordRequestAt(http.MethodGet, "/recent", http.StatusOK, time.Millisecond, 0, 0, now.Add(-23*time.Hour), true)
	m.RecordRequest(http.MethodGet, "/current", http.StatusOK, time.Millisecond, 0, 0)

	if got := m.GetTotalRequests(); got != 3 {
		t.Fatalf("GetTotalRequests() = %d, want 3", got)
	}
	if got := m.GetRequestsLast24Hours(); got != 2 {
		t.Fatalf("GetRequestsLast24Hours() = %d, want 2", got)
	}
}

func TestMetricsMiddlewareCountsRequestsWithoutRecordingPathTokens(t *testing.T) {
	m := NewMetrics(config.DefaultConfig())
	router := chi.NewRouter()
	router.Use(m.MetricsMiddleware)
	router.Get("/alerts/{token}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/alerts/secret-token-value", nil))

	if got := m.GetTotalRequests(); got != 1 {
		t.Fatalf("GetTotalRequests() = %d, want 1", got)
	}
	if got := m.GetRequestsLast24Hours(); got != 1 {
		t.Fatalf("GetRequestsLast24Hours() = %d, want 1", got)
	}
	if got := m.GetActiveConnections(); got != 0 {
		t.Fatalf("GetActiveConnections() = %d after request, want 0", got)
	}

	metrics := httptest.NewRecorder()
	m.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/server/metrics", nil))
	if strings.Contains(metrics.Body.String(), "secret-token-value") {
		t.Fatal("metrics exposed a path token as a label")
	}
	if !strings.Contains(metrics.Body.String(), `path="/alerts/{token}"`) {
		t.Fatal("metrics did not use the router's low-cardinality route pattern")
	}
}

func TestMetricsMiddlewareExcludesHealthAndScrapeFromPersistedCounts(t *testing.T) {
	m := NewMetrics(config.DefaultConfig())
	router := chi.NewRouter()
	router.Use(m.MetricsMiddleware)
	router.Get("/server/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	router.Get("/server/metrics", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	router.Get("/static/app.css", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, path := range []string{"/server/healthz", "/server/metrics", "/static/app.css"} {
		router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if got := m.GetTotalRequests(); got != 1 {
		t.Fatalf("GetTotalRequests() = %d, want only the static request counted", got)
	}
}
