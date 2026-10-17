package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
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
	m.recordRequestAt(http.MethodGet, "/old", http.StatusOK, time.Millisecond, 0, 0, now.Add(-25*time.Hour))
	m.recordRequestAt(http.MethodGet, "/recent", http.StatusOK, time.Millisecond, 0, 0, now.Add(-23*time.Hour))
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
