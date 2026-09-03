package api

// api_extra_test.go adds targeted coverage for handleWidgets and handleWidgetData
// with a real widget.Manager, and for ServeSwaggerUI with TLS/forwarded-proto paths.
//
// Coverage targets:
//   - handleWidgets: real manager, no category filter
//   - handleWidgets: real manager, with category filter that matches widgets
//   - handleWidgets: real manager, with category filter that matches nothing
//   - handleWidgetData: real manager, valid widget type (no fetcher → ok response)
//   - handleWidgetData: real manager, with query params populated
//   - handleWidgetData: real manager, empty type (400)
//   - handleWidgetData: real manager with registered fetcher (cache + fetch path)
//   - handleWidgetData: real manager with fetcher returning error (WidgetData.Error set)
//   - ServeSwaggerUI: with r.TLS set (https scheme path)

import (
	"context"
	cryptotls "crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/widget"
)

// newWidgetManager builds a widget.Manager with widgets enabled.
func newWidgetManager() *widget.Manager {
	cfg := &config.WidgetsConfig{
		Enabled:        true,
		DefaultWidgets: []string{"clock", "calculator"},
		CacheTTL:       60,
	}
	return widget.NewManager(cfg)
}

// newHandlerWithWidgets returns a Handler with a real widget.Manager set.
func newHandlerWithWidgets() *Handler {
	h := newTestHandler()
	h.SetWidgetManager(newWidgetManager())
	return h
}

// ---- handleWidgets with real manager ----

// TestHandleWidgetsWithManager covers the path where widgetManager != nil and
// no category filter is applied.
func TestHandleWidgetsWithManager(t *testing.T) {
	handler := newHandlerWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets", nil)
	w := httptest.NewRecorder()

	handler.handleWidgets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWidgets() status = %d, want 200", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !resp.OK {
		t.Error("handleWidgets() OK = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("handleWidgets() data should be a map")
	}
	if _, hasWidgets := data["widgets"]; !hasWidgets {
		t.Error("handleWidgets() data should contain 'widgets' key")
	}
	if _, hasDefaults := data["defaults"]; !hasDefaults {
		t.Error("handleWidgets() data should contain 'defaults' key")
	}
}

// TestHandleWidgetsWithManagerCategoryFilter covers the category filter path
// using "tool" which has several widgets (clock, calculator, etc.).
func TestHandleWidgetsWithManagerCategoryFilter(t *testing.T) {
	handler := newHandlerWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets?category=tool", nil)
	w := httptest.NewRecorder()

	handler.handleWidgets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWidgets(category=tool) status = %d, want 200", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !resp.OK {
		t.Error("handleWidgets(category=tool) OK = false, want true")
	}
}

// TestHandleWidgetsWithManagerUnknownCategory covers the category filter path
// where no widgets match — filtered list should be nil/empty but still return 200.
func TestHandleWidgetsWithManagerUnknownCategory(t *testing.T) {
	handler := newHandlerWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets?category=nosuchcategory", nil)
	w := httptest.NewRecorder()

	handler.handleWidgets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWidgets(category=nosuchcategory) status = %d, want 200", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !resp.OK {
		t.Error("handleWidgets(category=nosuchcategory) OK = false, want true")
	}
}

// ---- handleWidgetData with real manager ----

// TestHandleWidgetDataWithManagerValidType covers the path where manager is set,
// widgetType is valid, IsWidgetEnabled returns true, and FetchWidgetData is called
// (no fetcher registered → returns ok WidgetData with Error field).
func TestHandleWidgetDataWithManagerValidType(t *testing.T) {
	handler := newHandlerWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/clock", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWidgetData(clock) status = %d, want 200", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !resp.OK {
		t.Errorf("handleWidgetData(clock) OK = false, want true")
	}
}

// TestHandleWidgetDataWithManagerWeatherType exercises the data widget path
// (weather, no fetcher registered → returns ok with "widget not available" error).
func TestHandleWidgetDataWithManagerWeatherType(t *testing.T) {
	handler := newHandlerWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/weather", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWidgetData(weather) status = %d, want 200", w.Code)
	}
}

// TestHandleWidgetDataWithManagerQueryParams covers params collection — query
// parameters are collected into the params map passed to FetchWidgetData.
func TestHandleWidgetDataWithManagerQueryParams(t *testing.T) {
	handler := newHandlerWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/weather?city=london&units=metric", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWidgetData(weather?city=london) status = %d, want 200", w.Code)
	}
}

// TestHandleWidgetDataWithManagerEmptyType covers the empty-type 400 branch
// when a real manager is set.
func TestHandleWidgetDataWithManagerEmptyType(t *testing.T) {
	handler := newHandlerWithWidgets()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("handleWidgetData(empty type, real manager) status = %d, want 400", w.Code)
	}
}

// TestHandleWidgetDataWithFetcher covers FetchWidgetData with a real fetcher
// registered — exercises the cache-check and fetch code path.
func TestHandleWidgetDataWithFetcher(t *testing.T) {
	handler := newHandlerWithWidgets()

	handler.widgetManager.RegisterFetcher(&mockWidgetFetcher{
		widgetType: widget.WidgetClock,
		data: &widget.WidgetData{
			Type: widget.WidgetClock,
			Data: map[string]interface{}{"time": "12:00"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/clock", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleWidgetData(clock with fetcher) status = %d, want 200", w.Code)
	}
	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if !resp.OK {
		t.Error("handleWidgetData(clock with fetcher) OK = false, want true")
	}
}

// TestHandleWidgetDataFetcherError covers the fetcher error path — the manager
// converts fetch errors to WidgetData.Error and returns 200.
func TestHandleWidgetDataFetcherError(t *testing.T) {
	handler := newHandlerWithWidgets()

	handler.widgetManager.RegisterFetcher(&mockWidgetFetcher{
		widgetType: widget.WidgetWeather,
		fetchErr:   fmt.Errorf("API key not configured"),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/weather", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	// Manager converts fetcher error to WidgetData.Error — still 200 ok
	if w.Code != http.StatusOK {
		t.Errorf("handleWidgetData(weather fetch error) status = %d, want 200", w.Code)
	}
}

// ---- ServeSwaggerUI with TLS ----

// TestServeSwaggerUITLSScheme covers the r.TLS != nil branch that sets scheme to "https".
func TestServeSwaggerUITLSScheme(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/openapi", nil)
	req.TLS = &cryptotls.ConnectionState{}
	req.Host = "secure.example.com"
	w := httptest.NewRecorder()

	handler.ServeSwaggerUI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ServeSwaggerUI(TLS) status = %d, want 200", w.Code)
	}

	// html/template encodes the URL inside <script> context, so forward slashes
	// become \/ — check for the scheme prefix in escaped form.
	body := w.Body.String()
	if !strings.Contains(body, "https:") {
		n := len(body)
		if n > 300 {
			n = 300
		}
		t.Errorf("ServeSwaggerUI(TLS) body should contain https scheme, got (first 300 chars): %q", body[:n])
	}
}

// ---- mockWidgetFetcher ----

// mockWidgetFetcher implements widget.Fetcher for use in tests.
type mockWidgetFetcher struct {
	widgetType widget.WidgetType
	data       *widget.WidgetData
	fetchErr   error
}

func (f *mockWidgetFetcher) WidgetType() widget.WidgetType {
	return f.widgetType
}

func (f *mockWidgetFetcher) Fetch(ctx context.Context, params map[string]string) (*widget.WidgetData, error) {
	if f.fetchErr != nil {
		return nil, f.fetchErr
	}
	if f.data != nil {
		return f.data, nil
	}
	return &widget.WidgetData{Type: f.widgetType}, nil
}

func (f *mockWidgetFetcher) CacheDuration() time.Duration {
	return 60 * time.Second
}
