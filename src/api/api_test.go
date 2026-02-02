package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engines"
)

func newTestHandler() *Handler {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title:       "Test Search",
			Description: "Test Description",
			Mode:        "development",
		},
	}
	registry := engines.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	return NewHandler(cfg, registry, aggregator)
}

func TestHealthzEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", data["status"])
	}
}

func TestInfoEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	w := httptest.NewRecorder()

	handler.handleInfo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}
}

func TestCategoriesEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
	w := httptest.NewRecorder()

	handler.handleCategories(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.([]interface{})
	if !ok {
		t.Fatal("Expected data to be an array")
	}

	// Should have 5 categories: general, images, videos, news, maps
	if len(data) != 5 {
		t.Errorf("Expected 5 categories, got %d", len(data))
	}
}

func TestSearchEndpointMissingQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.OK {
		t.Error("Expected success to be false for missing query")
	}

	if response.Error == "" {
		t.Error("Expected error to be present")
	}
}

func TestAutocompleteEmptyQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete", nil)
	w := httptest.NewRecorder()

	handler.handleAutocomplete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	// Empty query should return empty array
	data, ok := response.Data.([]interface{})
	if !ok {
		t.Fatal("Expected data to be an array")
	}

	if len(data) != 0 {
		t.Errorf("Expected empty array for empty query, got %d items", len(data))
	}
}

func TestBangsEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bangs", nil)
	w := httptest.NewRecorder()

	handler.handleBangs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	bangs, ok := data["bangs"].([]interface{})
	if !ok {
		t.Fatal("Expected bangs to be an array")
	}

	// Should have multiple built-in bangs
	if len(bangs) == 0 {
		t.Error("Expected at least one bang")
	}
}

func TestEnginesEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines", nil)
	w := httptest.NewRecorder()

	handler.handleEngines(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}
}

func TestFormatDuration(t *testing.T) {
	handler := &Handler{}

	// Test zero duration
	result := handler.formatDuration(0)
	if result != "0m" {
		t.Errorf("formatDuration(0) = %q, want %q", result, "0m")
	}

	// Test minutes
	result = handler.formatDuration(5 * time.Minute)
	if result != "5m" {
		t.Errorf("formatDuration(5m) = %q, want %q", result, "5m")
	}

	// Test hours and minutes
	result = handler.formatDuration(2*time.Hour + 30*time.Minute)
	if result != "2h 30m" {
		t.Errorf("formatDuration(2h30m) = %q, want %q", result, "2h 30m")
	}

	// Test days
	result = handler.formatDuration(25 * time.Hour)
	if result != "1d 1h 0m" {
		t.Errorf("formatDuration(25h) = %q, want %q", result, "1d 1h 0m")
	}
}

func TestFormatBytes(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := handler.formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/path", "example.com"},
		{"http://example.com/path", "example.com"},
		{"https://www.example.com/path/to/page", "www.example.com"},
		{"example.com/path", "example.com"},
		{"https://example.com", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

func TestAPIResponseStructure(t *testing.T) {
	// Test that APIResponse serializes correctly
	response := APIResponse{
		OK: true,
		Data:    map[string]string{"key": "value"},
		Meta:    &APIMeta{Version: "v1"},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal APIResponse: %v", err)
	}

	var unmarshaled APIResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal APIResponse: %v", err)
	}

	if unmarshaled.OK != response.OK {
		t.Errorf("Success mismatch: got %v, want %v", unmarshaled.OK, response.OK)
	}
}

// TestAPIErrorResponse tests the unified error response format per AI.md PART 16
func TestAPIErrorResponse(t *testing.T) {
	response := APIResponse{
		OK:      false,
		Error:   "BAD_REQUEST",
		Message: "Invalid request format",
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}

	var unmarshaled APIResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if unmarshaled.Error == "" {
		t.Fatal("Expected error code to be present")
	}

	if unmarshaled.Error != "BAD_REQUEST" {
		t.Errorf("Error code mismatch: got %q, want %q", unmarshaled.Error, "BAD_REQUEST")
	}

	if unmarshaled.Message != "Invalid request format" {
		t.Errorf("Error message mismatch: got %q, want %q", unmarshaled.Message, "Invalid request format")
	}
}

// Tests for Autodiscover endpoint

func TestAutodiscoverEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	w := httptest.NewRecorder()

	handler.handleAutodiscover(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	// Check server info
	server, ok := data["server"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected server to be a map")
	}

	if server["name"] == nil {
		t.Error("Expected server name to be present")
	}

	// Check API info
	api, ok := data["api"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected api to be a map")
	}

	if api["version"] != "v1" {
		t.Errorf("Expected API version 'v1', got %v", api["version"])
	}
}

func TestAutodiscoverEndpointMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/autodiscover", nil)
	w := httptest.NewRecorder()

	handler.handleAutodiscover(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// Tests for Search endpoint with valid query

func TestSearchEndpointWithQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	// Should return OK even if no results
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d or %d, got %d", http.StatusOK, http.StatusInternalServerError, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// The search might fail without engines, but response format should be valid
	if response.OK {
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be a map")
		}
		if data["query"] != "test" {
			t.Errorf("Expected query 'test', got %v", data["query"])
		}
	}
}

func TestSearchEndpointWithCategory(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&category=images", nil)
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.OK {
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be a map")
		}
		if data["category"] != "images" {
			t.Errorf("Expected category 'images', got %v", data["category"])
		}
	}
}

// Tests for Autocomplete with query

func TestAutocompleteWithQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleAutocomplete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	// Autocomplete may return empty array if external API fails
	_, ok := response.Data.([]interface{})
	if !ok {
		t.Error("Expected data to be an array")
	}
}

// Tests for Engine by ID endpoint

func TestEngineByIDEmpty(t *testing.T) {
	handler := newTestHandler()

	// Empty ID should fall back to engines list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines/", nil)
	w := httptest.NewRecorder()

	handler.handleEngineByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestEngineByIDNotFound(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.handleEngineByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// Tests for Widgets endpoint

func TestWidgetsEndpointNoManager(t *testing.T) {
	handler := newTestHandler()
	// widgetManager is nil by default

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets", nil)
	w := httptest.NewRecorder()

	handler.handleWidgets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	// When widget manager is nil, still returns enabled: true with basic widgets
	if data["enabled"] != true {
		t.Error("Expected enabled to be true (basic widgets available)")
	}
}

// Tests for Widget data endpoint

func TestWidgetDataNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/weather", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	// Returns 200 OK with error message (graceful degradation)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Tests for Instant Answer endpoint

func TestInstantAnswerEndpointNoQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instant", nil)
	w := httptest.NewRecorder()

	handler.handleInstantAnswer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestInstantAnswerEndpointNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instant?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleInstantAnswer(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["found"] != false {
		t.Error("Expected found to be false when instant manager is nil")
	}
}

// Tests for Related Searches endpoint

func TestRelatedSearchesEndpointNoQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related", nil)
	w := httptest.NewRecorder()

	handler.handleRelatedSearches(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRelatedSearchesEndpointNoProvider(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleRelatedSearches(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["count"].(float64) != 0 {
		t.Error("Expected count to be 0 when related searches is nil")
	}
}

// Tests for Set methods

func TestSetWidgetManager(t *testing.T) {
	handler := newTestHandler()

	if handler.widgetManager != nil {
		t.Error("Widget manager should be nil initially")
	}

	// We can't easily create a widget manager without config, but we can test the method exists
	handler.SetWidgetManager(nil)

	if handler.widgetManager != nil {
		t.Error("Widget manager should still be nil after setting to nil")
	}
}

func TestSetInstantManager(t *testing.T) {
	handler := newTestHandler()

	if handler.instantManager != nil {
		t.Error("Instant manager should be nil initially")
	}

	handler.SetInstantManager(nil)

	if handler.instantManager != nil {
		t.Error("Instant manager should still be nil after setting to nil")
	}
}

func TestSetRelatedSearches(t *testing.T) {
	handler := newTestHandler()

	if handler.relatedSearches != nil {
		t.Error("Related searches should be nil initially")
	}

	handler.SetRelatedSearches(nil)

	if handler.relatedSearches != nil {
		t.Error("Related searches should still be nil after setting to nil")
	}
}

// Tests for text response format

func TestRespondWithFormatText(t *testing.T) {
	handler := newTestHandler()

	// Test .txt extension
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz.txt", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}

	body := w.Body.String()
	if body != "OK\n" && !strings.HasPrefix(body, "ERROR:") {
		t.Errorf("Expected 'OK\\n' or 'ERROR:', got %q", body)
	}
}

func TestRespondWithFormatAcceptHeader(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.Header.Set("Accept", "text/plain")
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

func TestRespondWithFormatQueryParam(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz?format=text", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

// Tests for helper functions

func TestStripTxtExtension(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/v1/healthz.txt", "/api/v1/healthz"},
		{"/api/v1/info.txt", "/api/v1/info"},
		{"/api/v1/healthz", "/api/v1/healthz"},
		{"/path/to/file", "/path/to/file"},
		{"file.txt", "file"},
		{"", ""},
	}

	for _, tt := range tests {
		result := stripTxtExtension(tt.input)
		if result != tt.expected {
			t.Errorf("stripTxtExtension(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCheckDiskHealth(t *testing.T) {
	handler := newTestHandler()

	result := handler.checkDiskHealth()
	if result != "ok" {
		t.Errorf("checkDiskHealth() = %q, want 'ok'", result)
	}
}

func TestGetRequestsTotal(t *testing.T) {
	handler := newTestHandler()

	// With metrics disabled, should return 0
	result := handler.getRequestsTotal()
	if result != 0 {
		t.Errorf("getRequestsTotal() = %d, want 0", result)
	}
}

func TestGetRequests24h(t *testing.T) {
	handler := newTestHandler()

	result := handler.getRequests24h()
	if result != 0 {
		t.Errorf("getRequests24h() = %d, want 0", result)
	}
}

func TestGetActiveConnections(t *testing.T) {
	handler := newTestHandler()

	result := handler.getActiveConnections()
	if result != 0 {
		t.Errorf("getActiveConnections() = %d, want 0", result)
	}
}

// Tests for builtin bangs

func TestGetBuiltinBangs(t *testing.T) {
	bangs := getBuiltinBangs()

	if len(bangs) == 0 {
		t.Error("Expected at least one builtin bang")
	}

	// Check that Google bang exists
	var foundGoogle bool
	for _, b := range bangs {
		if b.Shortcut == "g" {
			foundGoogle = true
			if b.Name != "Google" {
				t.Errorf("Google bang Name = %q, want 'Google'", b.Name)
			}
			if b.Category != "general" {
				t.Errorf("Google bang Category = %q, want 'general'", b.Category)
			}
			break
		}
	}

	if !foundGoogle {
		t.Error("Expected to find Google bang with shortcut 'g'")
	}
}

func TestBangsEndpointWithCategory(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bangs?category=code", nil)
	w := httptest.NewRecorder()

	handler.handleBangs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	bangs, ok := data["bangs"].([]interface{})
	if !ok {
		t.Fatal("Expected bangs to be an array")
	}

	// All bangs should be from code category
	for _, b := range bangs {
		bangMap := b.(map[string]interface{})
		if bangMap["category"] != "code" {
			t.Errorf("Expected category 'code', got %v", bangMap["category"])
		}
	}
}

func TestBangsEndpointWithSearch(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bangs?search=google", nil)
	w := httptest.NewRecorder()

	handler.handleBangs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	bangs, ok := data["bangs"].([]interface{})
	if !ok {
		t.Fatal("Expected bangs to be an array")
	}

	// Should find at least Google bang
	if len(bangs) == 0 {
		t.Error("Expected to find at least one bang matching 'google'")
	}
}

// Tests for response types JSON serialization

func TestHealthResponseSerialization(t *testing.T) {
	health := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		GoVersion: "go1.21",
		Mode:      "production",
		Uptime:    "1h 30m",
		Timestamp: "2024-01-01T00:00:00Z",
		Checks: ChecksInfo{
			Database:  "ok",
			Cache:     "ok",
			Disk:      "ok",
			Scheduler: "ok",
		},
	}

	data, err := json.Marshal(health)
	if err != nil {
		t.Fatalf("Failed to marshal HealthResponse: %v", err)
	}

	var unmarshaled HealthResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal HealthResponse: %v", err)
	}

	if unmarshaled.Status != health.Status {
		t.Errorf("Status mismatch: got %q, want %q", unmarshaled.Status, health.Status)
	}
}

func TestSearchResponseSerialization(t *testing.T) {
	resp := SearchResponse{
		Query:    "test",
		Category: "general",
		Results:  []SearchResult{{Title: "Test", URL: "https://example.com"}},
		Pagination: Pagination{
			Page:  1,
			Limit: 20,
			Total: 1,
			Pages: 1,
		},
		SearchTime: 100.5,
		Engines:    []string{"google"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal SearchResponse: %v", err)
	}

	var unmarshaled SearchResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SearchResponse: %v", err)
	}

	if unmarshaled.Query != resp.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, resp.Query)
	}
	if len(unmarshaled.Results) != 1 {
		t.Errorf("Results count mismatch: got %d, want 1", len(unmarshaled.Results))
	}
}

func TestBangInfoSerialization(t *testing.T) {
	bang := BangInfo{
		Shortcut:    "g",
		Name:        "Google",
		URL:         "https://google.com?q={query}",
		Category:    "general",
		Description: "Google Search",
		Aliases:     []string{"google"},
	}

	data, err := json.Marshal(bang)
	if err != nil {
		t.Fatalf("Failed to marshal BangInfo: %v", err)
	}

	var unmarshaled BangInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal BangInfo: %v", err)
	}

	if unmarshaled.Shortcut != bang.Shortcut {
		t.Errorf("Shortcut mismatch: got %q, want %q", unmarshaled.Shortcut, bang.Shortcut)
	}
}

// Tests for constants

func TestAPIConstants(t *testing.T) {
	if APIVersion != "v1" {
		t.Errorf("APIVersion = %q, want 'v1'", APIVersion)
	}
	if APIPrefix != "/api/v1" {
		t.Errorf("APIPrefix = %q, want '/api/v1'", APIPrefix)
	}
}

// Tests for getClientIP function

func TestGetClientIPXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1, 192.0.2.1")

	ip := getClientIP(req)
	if ip != "203.0.113.1" {
		t.Errorf("getClientIP() = %q, want %q", ip, "203.0.113.1")
	}
}

func TestGetClientIPXForwardedForSingle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5")

	ip := getClientIP(req)
	if ip != "203.0.113.5" {
		t.Errorf("getClientIP() = %q, want %q", ip, "203.0.113.5")
	}
}

func TestGetClientIPXForwardedForWithSpaces(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "  203.0.113.2  , 198.51.100.1")

	ip := getClientIP(req)
	if ip != "203.0.113.2" {
		t.Errorf("getClientIP() = %q, want %q", ip, "203.0.113.2")
	}
}

func TestGetClientIPXRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "203.0.113.3")

	ip := getClientIP(req)
	if ip != "203.0.113.3" {
		t.Errorf("getClientIP() = %q, want %q", ip, "203.0.113.3")
	}
}

func TestGetClientIPXForwardedForPriority(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	req.Header.Set("X-Real-IP", "203.0.113.2")

	// X-Forwarded-For should take priority
	ip := getClientIP(req)
	if ip != "203.0.113.1" {
		t.Errorf("getClientIP() = %q, want %q (X-Forwarded-For should take priority)", ip, "203.0.113.1")
	}
}

func TestGetClientIPRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getClientIP() = %q, want %q", ip, "192.168.1.1")
	}
}

func TestGetClientIPRemoteAddrIPv6(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "[::1]:12345"

	ip := getClientIP(req)
	// LastIndex of ":" will find the port separator
	if ip != "[::1]" {
		t.Errorf("getClientIP() = %q, want %q", ip, "[::1]")
	}
}

func TestGetClientIPNoHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:8080"

	ip := getClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("getClientIP() = %q, want %q", ip, "10.0.0.1")
	}
}

// Tests for auth request/response struct serialization

func TestRegisterRequestSerialization(t *testing.T) {
	req := RegisterRequest{
		Username: "testuser",
		Email:    "test@example.com",
		Password: "SecurePass123!",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal RegisterRequest: %v", err)
	}

	var unmarshaled RegisterRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal RegisterRequest: %v", err)
	}

	if unmarshaled.Username != req.Username {
		t.Errorf("Username mismatch: got %q, want %q", unmarshaled.Username, req.Username)
	}
	if unmarshaled.Email != req.Email {
		t.Errorf("Email mismatch: got %q, want %q", unmarshaled.Email, req.Email)
	}
	if unmarshaled.Password != req.Password {
		t.Errorf("Password mismatch: got %q, want %q", unmarshaled.Password, req.Password)
	}
}

func TestLoginRequestSerialization(t *testing.T) {
	req := LoginRequest{
		Username:   "testuser",
		Password:   "SecurePass123!",
		RememberMe: true,
		TOTPCode:   "123456",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal LoginRequest: %v", err)
	}

	var unmarshaled LoginRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal LoginRequest: %v", err)
	}

	if unmarshaled.Username != req.Username {
		t.Errorf("Username mismatch: got %q, want %q", unmarshaled.Username, req.Username)
	}
	if unmarshaled.Password != req.Password {
		t.Errorf("Password mismatch: got %q, want %q", unmarshaled.Password, req.Password)
	}
	if unmarshaled.RememberMe != req.RememberMe {
		t.Errorf("RememberMe mismatch: got %v, want %v", unmarshaled.RememberMe, req.RememberMe)
	}
	if unmarshaled.TOTPCode != req.TOTPCode {
		t.Errorf("TOTPCode mismatch: got %q, want %q", unmarshaled.TOTPCode, req.TOTPCode)
	}
}

func TestLoginResponseSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	resp := LoginResponse{
		User: UserResponse{
			ID:       1,
			Username: "testuser",
			Email:    "test@example.com",
			Role:     "user",
		},
		SessionID:   "ses_test123",
		ExpiresAt:   now,
		Requires2FA: true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal LoginResponse: %v", err)
	}

	var unmarshaled LoginResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal LoginResponse: %v", err)
	}

	if unmarshaled.User.ID != resp.User.ID {
		t.Errorf("User.ID mismatch: got %d, want %d", unmarshaled.User.ID, resp.User.ID)
	}
	if unmarshaled.SessionID != resp.SessionID {
		t.Errorf("SessionID mismatch: got %q, want %q", unmarshaled.SessionID, resp.SessionID)
	}
	if unmarshaled.Requires2FA != resp.Requires2FA {
		t.Errorf("Requires2FA mismatch: got %v, want %v", unmarshaled.Requires2FA, resp.Requires2FA)
	}
}

func TestUserResponseSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	lastLogin := now.Add(-24 * time.Hour)
	resp := UserResponse{
		ID:            42,
		Username:      "testuser",
		Email:         "test@example.com",
		DisplayName:   "Test User",
		AvatarURL:     "https://example.com/avatar.png",
		Role:          "admin",
		EmailVerified: true,
		CreatedAt:     now,
		LastLogin:     &lastLogin,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal UserResponse: %v", err)
	}

	var unmarshaled UserResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal UserResponse: %v", err)
	}

	if unmarshaled.ID != resp.ID {
		t.Errorf("ID mismatch: got %d, want %d", unmarshaled.ID, resp.ID)
	}
	if unmarshaled.Username != resp.Username {
		t.Errorf("Username mismatch: got %q, want %q", unmarshaled.Username, resp.Username)
	}
	if unmarshaled.Email != resp.Email {
		t.Errorf("Email mismatch: got %q, want %q", unmarshaled.Email, resp.Email)
	}
	if unmarshaled.DisplayName != resp.DisplayName {
		t.Errorf("DisplayName mismatch: got %q, want %q", unmarshaled.DisplayName, resp.DisplayName)
	}
	if unmarshaled.AvatarURL != resp.AvatarURL {
		t.Errorf("AvatarURL mismatch: got %q, want %q", unmarshaled.AvatarURL, resp.AvatarURL)
	}
	if unmarshaled.Role != resp.Role {
		t.Errorf("Role mismatch: got %q, want %q", unmarshaled.Role, resp.Role)
	}
	if unmarshaled.EmailVerified != resp.EmailVerified {
		t.Errorf("EmailVerified mismatch: got %v, want %v", unmarshaled.EmailVerified, resp.EmailVerified)
	}
}

func TestUserResponseNilLastLogin(t *testing.T) {
	resp := UserResponse{
		ID:        1,
		Username:  "testuser",
		Email:     "test@example.com",
		Role:      "user",
		LastLogin: nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal UserResponse: %v", err)
	}

	// JSON should omit last_login when nil
	if strings.Contains(string(data), "last_login") {
		t.Error("Expected last_login to be omitted when nil")
	}
}

func TestForgotPasswordRequestSerialization(t *testing.T) {
	req := ForgotPasswordRequest{
		Email: "test@example.com",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ForgotPasswordRequest: %v", err)
	}

	var unmarshaled ForgotPasswordRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ForgotPasswordRequest: %v", err)
	}

	if unmarshaled.Email != req.Email {
		t.Errorf("Email mismatch: got %q, want %q", unmarshaled.Email, req.Email)
	}
}

func TestResetPasswordRequestSerialization(t *testing.T) {
	req := ResetPasswordRequest{
		Token:       "reset_token_123",
		NewPassword: "NewSecurePass456!",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ResetPasswordRequest: %v", err)
	}

	var unmarshaled ResetPasswordRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ResetPasswordRequest: %v", err)
	}

	if unmarshaled.Token != req.Token {
		t.Errorf("Token mismatch: got %q, want %q", unmarshaled.Token, req.Token)
	}
	if unmarshaled.NewPassword != req.NewPassword {
		t.Errorf("NewPassword mismatch: got %q, want %q", unmarshaled.NewPassword, req.NewPassword)
	}
}

func TestRecoveryKeyRequestSerialization(t *testing.T) {
	req := RecoveryKeyRequest{
		Username:    "testuser",
		RecoveryKey: "AAAAA-BBBBB-CCCCC-DDDDD",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal RecoveryKeyRequest: %v", err)
	}

	var unmarshaled RecoveryKeyRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal RecoveryKeyRequest: %v", err)
	}

	if unmarshaled.Username != req.Username {
		t.Errorf("Username mismatch: got %q, want %q", unmarshaled.Username, req.Username)
	}
	if unmarshaled.RecoveryKey != req.RecoveryKey {
		t.Errorf("RecoveryKey mismatch: got %q, want %q", unmarshaled.RecoveryKey, req.RecoveryKey)
	}
}

func TestVerifyEmailRequestSerialization(t *testing.T) {
	req := VerifyEmailRequest{
		Token: "verify_token_abc",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal VerifyEmailRequest: %v", err)
	}

	var unmarshaled VerifyEmailRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal VerifyEmailRequest: %v", err)
	}

	if unmarshaled.Token != req.Token {
		t.Errorf("Token mismatch: got %q, want %q", unmarshaled.Token, req.Token)
	}
}

func TestTwoFactorVerifyRequestSerialization(t *testing.T) {
	req := TwoFactorVerifyRequest{
		Code:      "123456",
		SessionID: "ses_partial_123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal TwoFactorVerifyRequest: %v", err)
	}

	var unmarshaled TwoFactorVerifyRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal TwoFactorVerifyRequest: %v", err)
	}

	if unmarshaled.Code != req.Code {
		t.Errorf("Code mismatch: got %q, want %q", unmarshaled.Code, req.Code)
	}
	if unmarshaled.SessionID != req.SessionID {
		t.Errorf("SessionID mismatch: got %q, want %q", unmarshaled.SessionID, req.SessionID)
	}
}

// Tests for EngineInfo serialization

func TestEngineInfoSerialization(t *testing.T) {
	engine := EngineInfo{
		ID:          "google",
		Name:        "Google",
		Enabled:     true,
		Priority:    1,
		Categories:  []string{"general", "images"},
		Description: "Google Search Engine",
		Homepage:    "https://google.com",
	}

	data, err := json.Marshal(engine)
	if err != nil {
		t.Fatalf("Failed to marshal EngineInfo: %v", err)
	}

	var unmarshaled EngineInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal EngineInfo: %v", err)
	}

	if unmarshaled.ID != engine.ID {
		t.Errorf("ID mismatch: got %q, want %q", unmarshaled.ID, engine.ID)
	}
	if unmarshaled.Name != engine.Name {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, engine.Name)
	}
	if len(unmarshaled.Categories) != len(engine.Categories) {
		t.Errorf("Categories length mismatch: got %d, want %d", len(unmarshaled.Categories), len(engine.Categories))
	}
	if unmarshaled.Enabled != engine.Enabled {
		t.Errorf("Enabled mismatch: got %v, want %v", unmarshaled.Enabled, engine.Enabled)
	}
	if unmarshaled.Priority != engine.Priority {
		t.Errorf("Priority mismatch: got %d, want %d", unmarshaled.Priority, engine.Priority)
	}
}

// Tests for SearchResult serialization

func TestSearchResultSerialization(t *testing.T) {
	result := SearchResult{
		Title:       "Test Result",
		URL:         "https://example.com/page",
		Description: "This is a test result description",
		Engine:      "google",
		Score:       0.95,
		Category:    "general",
		Thumbnail:   "https://example.com/thumb.png",
		Date:        "2024-01-01",
		Domain:      "example.com",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal SearchResult: %v", err)
	}

	var unmarshaled SearchResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SearchResult: %v", err)
	}

	if unmarshaled.Title != result.Title {
		t.Errorf("Title mismatch: got %q, want %q", unmarshaled.Title, result.Title)
	}
	if unmarshaled.URL != result.URL {
		t.Errorf("URL mismatch: got %q, want %q", unmarshaled.URL, result.URL)
	}
	if unmarshaled.Score != result.Score {
		t.Errorf("Score mismatch: got %f, want %f", unmarshaled.Score, result.Score)
	}
	if unmarshaled.Domain != result.Domain {
		t.Errorf("Domain mismatch: got %q, want %q", unmarshaled.Domain, result.Domain)
	}
}

// Tests for APIMeta serialization

func TestAPIMetaSerialization(t *testing.T) {
	meta := APIMeta{
		RequestID:   "req_123abc",
		ProcessTime: 15.5,
		Version:     "v1",
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Failed to marshal APIMeta: %v", err)
	}

	var unmarshaled APIMeta
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal APIMeta: %v", err)
	}

	if unmarshaled.Version != meta.Version {
		t.Errorf("Version mismatch: got %q, want %q", unmarshaled.Version, meta.Version)
	}
	if unmarshaled.RequestID != meta.RequestID {
		t.Errorf("RequestID mismatch: got %q, want %q", unmarshaled.RequestID, meta.RequestID)
	}
	if unmarshaled.ProcessTime != meta.ProcessTime {
		t.Errorf("ProcessTime mismatch: got %f, want %f", unmarshaled.ProcessTime, meta.ProcessTime)
	}
}

// Tests for EnginesSummary serialization

func TestEnginesSummarySerialization(t *testing.T) {
	summary := EnginesSummary{
		Total:   10,
		Enabled: 5,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Failed to marshal EnginesSummary: %v", err)
	}

	var unmarshaled EnginesSummary
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal EnginesSummary: %v", err)
	}

	if unmarshaled.Total != summary.Total {
		t.Errorf("Total mismatch: got %d, want %d", unmarshaled.Total, summary.Total)
	}
	if unmarshaled.Enabled != summary.Enabled {
		t.Errorf("Enabled mismatch: got %d, want %d", unmarshaled.Enabled, summary.Enabled)
	}
}

// Tests for SystemInfo serialization

func TestSystemInfoSerialization(t *testing.T) {
	info := SystemInfo{
		GoVersion:    "go1.21.0",
		NumCPU:       8,
		NumGoroutine: 100,
		MemAlloc:     "256 MB",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal SystemInfo: %v", err)
	}

	var unmarshaled SystemInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SystemInfo: %v", err)
	}

	if unmarshaled.GoVersion != info.GoVersion {
		t.Errorf("GoVersion mismatch: got %q, want %q", unmarshaled.GoVersion, info.GoVersion)
	}
	if unmarshaled.NumCPU != info.NumCPU {
		t.Errorf("NumCPU mismatch: got %d, want %d", unmarshaled.NumCPU, info.NumCPU)
	}
	if unmarshaled.NumGoroutine != info.NumGoroutine {
		t.Errorf("NumGoroutine mismatch: got %d, want %d", unmarshaled.NumGoroutine, info.NumGoroutine)
	}
	if unmarshaled.MemAlloc != info.MemAlloc {
		t.Errorf("MemAlloc mismatch: got %q, want %q", unmarshaled.MemAlloc, info.MemAlloc)
	}
}

// Tests for SearchRequest serialization

func TestSearchRequestSerialization(t *testing.T) {
	req := SearchRequest{
		Query:      "golang testing",
		Category:   "general",
		Page:       1,
		Limit:      20,
		Engines:    []string{"google", "bing"},
		SafeSearch: "moderate",
		TimeRange:  "week",
		Language:   "en",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal SearchRequest: %v", err)
	}

	var unmarshaled SearchRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SearchRequest: %v", err)
	}

	if unmarshaled.Query != req.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, req.Query)
	}
	if unmarshaled.Category != req.Category {
		t.Errorf("Category mismatch: got %q, want %q", unmarshaled.Category, req.Category)
	}
	if unmarshaled.Page != req.Page {
		t.Errorf("Page mismatch: got %d, want %d", unmarshaled.Page, req.Page)
	}
	if unmarshaled.Limit != req.Limit {
		t.Errorf("Limit mismatch: got %d, want %d", unmarshaled.Limit, req.Limit)
	}
	if len(unmarshaled.Engines) != len(req.Engines) {
		t.Errorf("Engines length mismatch: got %d, want %d", len(unmarshaled.Engines), len(req.Engines))
	}
}

// Tests for CategoryInfo serialization

func TestCategoryInfoSerialization(t *testing.T) {
	category := CategoryInfo{
		ID:          "general",
		Name:        "General",
		Description: "General web search",
		Icon:        "search",
	}

	data, err := json.Marshal(category)
	if err != nil {
		t.Fatalf("Failed to marshal CategoryInfo: %v", err)
	}

	var unmarshaled CategoryInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal CategoryInfo: %v", err)
	}

	if unmarshaled.ID != category.ID {
		t.Errorf("ID mismatch: got %q, want %q", unmarshaled.ID, category.ID)
	}
	if unmarshaled.Name != category.Name {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, category.Name)
	}
	if unmarshaled.Description != category.Description {
		t.Errorf("Description mismatch: got %q, want %q", unmarshaled.Description, category.Description)
	}
	if unmarshaled.Icon != category.Icon {
		t.Errorf("Icon mismatch: got %q, want %q", unmarshaled.Icon, category.Icon)
	}
}

// Tests for BuildInfo serialization

func TestBuildInfoSerialization(t *testing.T) {
	build := BuildInfo{
		Commit: "abc123d",
		Date:   "2024-01-15T10:00:00Z",
	}

	data, err := json.Marshal(build)
	if err != nil {
		t.Fatalf("Failed to marshal BuildInfo: %v", err)
	}

	var unmarshaled BuildInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal BuildInfo: %v", err)
	}

	if unmarshaled.Commit != build.Commit {
		t.Errorf("Commit mismatch: got %q, want %q", unmarshaled.Commit, build.Commit)
	}
	if unmarshaled.Date != build.Date {
		t.Errorf("Date mismatch: got %q, want %q", unmarshaled.Date, build.Date)
	}
}

// Tests for StatsInfo serialization

func TestStatsInfoSerialization(t *testing.T) {
	stats := StatsInfo{
		RequestsTotal: 100000,
		Requests24h:   5000,
		ActiveConns:   42,
	}

	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("Failed to marshal StatsInfo: %v", err)
	}

	var unmarshaled StatsInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal StatsInfo: %v", err)
	}

	if unmarshaled.RequestsTotal != stats.RequestsTotal {
		t.Errorf("RequestsTotal mismatch: got %d, want %d", unmarshaled.RequestsTotal, stats.RequestsTotal)
	}
	if unmarshaled.Requests24h != stats.Requests24h {
		t.Errorf("Requests24h mismatch: got %d, want %d", unmarshaled.Requests24h, stats.Requests24h)
	}
	if unmarshaled.ActiveConns != stats.ActiveConns {
		t.Errorf("ActiveConns mismatch: got %d, want %d", unmarshaled.ActiveConns, stats.ActiveConns)
	}
}

// Tests for NodeInfo serialization

func TestNodeInfoSerialization(t *testing.T) {
	node := NodeInfo{
		ID:       "node-1",
		Hostname: "search-node-1.example.com",
	}

	data, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("Failed to marshal NodeInfo: %v", err)
	}

	var unmarshaled NodeInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal NodeInfo: %v", err)
	}

	if unmarshaled.ID != node.ID {
		t.Errorf("ID mismatch: got %q, want %q", unmarshaled.ID, node.ID)
	}
	if unmarshaled.Hostname != node.Hostname {
		t.Errorf("Hostname mismatch: got %q, want %q", unmarshaled.Hostname, node.Hostname)
	}
}

// Tests for ClusterInfo serialization

func TestClusterInfoSerialization(t *testing.T) {
	cluster := ClusterInfo{
		Enabled:   true,
		Status:    "connected",
		Primary:   "node1.example.com",
		Nodes:     []string{"node1.example.com", "node2.example.com", "node3.example.com"},
		NodeCount: 3,
		Role:      "primary",
	}

	data, err := json.Marshal(cluster)
	if err != nil {
		t.Fatalf("Failed to marshal ClusterInfo: %v", err)
	}

	var unmarshaled ClusterInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ClusterInfo: %v", err)
	}

	if unmarshaled.Enabled != cluster.Enabled {
		t.Errorf("Enabled mismatch: got %v, want %v", unmarshaled.Enabled, cluster.Enabled)
	}
	if unmarshaled.Status != cluster.Status {
		t.Errorf("Status mismatch: got %q, want %q", unmarshaled.Status, cluster.Status)
	}
	if unmarshaled.NodeCount != cluster.NodeCount {
		t.Errorf("NodeCount mismatch: got %d, want %d", unmarshaled.NodeCount, cluster.NodeCount)
	}
	if len(unmarshaled.Nodes) != len(cluster.Nodes) {
		t.Errorf("Nodes length mismatch: got %d, want %d", len(unmarshaled.Nodes), len(cluster.Nodes))
	}
	if unmarshaled.Role != cluster.Role {
		t.Errorf("Role mismatch: got %q, want %q", unmarshaled.Role, cluster.Role)
	}
}

// Tests for InfoResponse serialization

func TestInfoResponseSerialization(t *testing.T) {
	resp := InfoResponse{
		Name:        "Search API",
		Version:     "1.0.0",
		Description: "A search API server",
		Uptime:      "24h 30m",
		Mode:        "production",
		Engines: EnginesSummary{
			Total:   10,
			Enabled: 5,
		},
		System: SystemInfo{
			GoVersion:    "go1.21.0",
			NumCPU:       8,
			NumGoroutine: 50,
			MemAlloc:     "128 MB",
		},
		Features: map[string]bool{
			"multi_user": true,
			"tor":        false,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal InfoResponse: %v", err)
	}

	var unmarshaled InfoResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal InfoResponse: %v", err)
	}

	if unmarshaled.Name != resp.Name {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, resp.Name)
	}
	if unmarshaled.Version != resp.Version {
		t.Errorf("Version mismatch: got %q, want %q", unmarshaled.Version, resp.Version)
	}
	if unmarshaled.Mode != resp.Mode {
		t.Errorf("Mode mismatch: got %q, want %q", unmarshaled.Mode, resp.Mode)
	}
}

// Tests for formatDuration edge cases

func TestFormatDurationDaysOnly(t *testing.T) {
	handler := &Handler{}

	// Test exactly 24 hours
	result := handler.formatDuration(24 * time.Hour)
	if result != "1d 0h 0m" {
		t.Errorf("formatDuration(24h) = %q, want %q", result, "1d 0h 0m")
	}

	// Test multiple days
	result = handler.formatDuration(72 * time.Hour)
	if result != "3d 0h 0m" {
		t.Errorf("formatDuration(72h) = %q, want %q", result, "3d 0h 0m")
	}
}

func TestFormatDurationHoursOnly(t *testing.T) {
	handler := &Handler{}

	// Test exactly 1 hour
	result := handler.formatDuration(1 * time.Hour)
	if result != "1h 0m" {
		t.Errorf("formatDuration(1h) = %q, want %q", result, "1h 0m")
	}

	// Test 23 hours
	result = handler.formatDuration(23 * time.Hour)
	if result != "23h 0m" {
		t.Errorf("formatDuration(23h) = %q, want %q", result, "23h 0m")
	}
}

// Tests for formatBytes edge cases

func TestFormatBytesEdgeCases(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		bytes    uint64
		expected string
	}{
		{500, "500 B"},
		{1023, "1023 B"},
		{1536, "1.5 KB"},
		{1572864, "1.5 MB"},
		{1610612736, "1.5 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := handler.formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

// Tests for extractDomain edge cases

func TestExtractDomainEdgeCases(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"", ""},
		{"not-a-url", "not-a-url"},
		{"ftp://ftp.example.com/file", "ftp:"},  // Only http/https schemes are parsed fully
		{"https://sub.domain.example.com/path/to/page?q=test", "sub.domain.example.com"},
		{"http://localhost:8080/api", "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// Tests for NewHandler

func TestNewHandlerInitialization(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Server",
		},
	}
	registry := engines.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)

	handler := NewHandler(cfg, registry, aggregator)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
	if handler.config != cfg {
		t.Error("Config not set correctly")
	}
	if handler.registry != registry {
		t.Error("Registry not set correctly")
	}
	if handler.aggregator != aggregator {
		t.Error("Aggregator not set correctly")
	}
	if handler.startTime.IsZero() {
		t.Error("startTime should be set")
	}
}

// Tests for AutodiscoverResponse serialization

func TestAutodiscoverResponseSerialization(t *testing.T) {
	var resp AutodiscoverResponse
	resp.Server.Name = "Search Server"
	resp.Server.Version = "1.0.0"
	resp.Server.URL = "https://search.example.com"
	resp.Server.Features.Auth = true
	resp.Server.Features.Search = true
	resp.Server.Features.Register = true
	resp.Cluster.Primary = "https://search.example.com"
	resp.Cluster.Nodes = []string{"https://search.example.com", "https://search2.example.com"}
	resp.API.Version = "v1"
	resp.API.BasePath = "/api/v1"

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal AutodiscoverResponse: %v", err)
	}

	var unmarshaled AutodiscoverResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal AutodiscoverResponse: %v", err)
	}

	if unmarshaled.Server.Name != resp.Server.Name {
		t.Errorf("Server.Name mismatch: got %q, want %q", unmarshaled.Server.Name, resp.Server.Name)
	}
	if unmarshaled.Server.Version != resp.Server.Version {
		t.Errorf("Server.Version mismatch: got %q, want %q", unmarshaled.Server.Version, resp.Server.Version)
	}
	if unmarshaled.API.Version != resp.API.Version {
		t.Errorf("API.Version mismatch: got %q, want %q", unmarshaled.API.Version, resp.API.Version)
	}
	if len(unmarshaled.Cluster.Nodes) != len(resp.Cluster.Nodes) {
		t.Errorf("Cluster.Nodes length mismatch: got %d, want %d", len(unmarshaled.Cluster.Nodes), len(resp.Cluster.Nodes))
	}
}

// ============================================================================
// Tests for OpenAPI/Swagger handlers (openapi_handler.go)
// ============================================================================

func TestServeOpenAPISpec(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()

	handler.ServeOpenAPISpec(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type application/json, got %q", contentType)
	}

	// Check CORS header
	cors := w.Header().Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("Expected CORS header '*', got %q", cors)
	}

	// Check Cache-Control header
	cache := w.Header().Get("Cache-Control")
	if cache != "public, max-age=3600" {
		t.Errorf("Expected Cache-Control 'public, max-age=3600', got %q", cache)
	}
}

func TestServeSwaggerUI(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/openapi", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()

	handler.ServeSwaggerUI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type text/html, got %q", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("Expected Swagger UI HTML content")
	}
}

func TestServeSwaggerUIWithXForwardedProto(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/openapi", nil)
	req.Host = "api.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	handler.ServeSwaggerUI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	// Check for the full spec URL with https scheme from X-Forwarded-Proto
	// html/template escapes forward slashes in JavaScript contexts
	expectedURL := `https:\/\/api.example.com\/openapi.json`
	if !strings.Contains(body, expectedURL) {
		t.Errorf("Expected spec URL %q in body, but it was not found", expectedURL)
	}
}

func TestRegisterOpenAPIRoutes(t *testing.T) {
	handler := newTestHandler()
	mux := http.NewServeMux()

	handler.RegisterOpenAPIRoutes(mux)

	// Test that routes are registered by making requests
	tests := []struct {
		path   string
		status int
	}{
		{"/openapi.json", http.StatusOK},
		{"/openapi", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Host = "localhost"
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.status {
				t.Errorf("Expected status %d for %s, got %d", tt.status, tt.path, w.Code)
			}
		})
	}
}

// ============================================================================
// Tests for GraphQL handlers (graphql.go)
// ============================================================================

func TestNewGraphQLHandler(t *testing.T) {
	handler := newTestHandler()

	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	if gqlHandler == nil {
		t.Fatal("NewGraphQLHandler returned nil")
	}

	if gqlHandler.handler != handler {
		t.Error("Handler not set correctly")
	}
}

func TestGraphQLHandlerServeHTTPPost(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	// Test health query
	query := `{"query": "{ healthz { status version } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["data"] == nil {
		t.Error("Expected data in response")
	}
}

func TestGraphQLHandlerServeHTTPGet(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	// Test GET with query parameter
	req := httptest.NewRequest(http.MethodGet, "/graphql?query={healthz{status}}", nil)
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGraphQLHandlerServeHTTPGetWithVariables(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	// Test GET with variables - URL encode the query parameters
	query := url.QueryEscape("{categories{id}}")
	variables := url.QueryEscape(`{"limit": 10}`)
	req := httptest.NewRequest(http.MethodGet, "/graphql?query="+query+"&variables="+variables, nil)
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGraphQLHandlerServeHTTPMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPut, "/graphql", nil)
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestGraphQLHandlerServeHTTPInvalidJSON(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestGraphQLServeGraphiQL(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	w := httptest.NewRecorder()

	gqlHandler.ServeGraphiQL(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type text/html, got %q", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "graphiql") {
		t.Error("Expected GraphiQL HTML content")
	}
}

func TestRegisterGraphQLRoutes(t *testing.T) {
	handler := newTestHandler()
	mux := http.NewServeMux()

	err := handler.RegisterGraphQLRoutes(mux)
	if err != nil {
		t.Fatalf("Failed to register GraphQL routes: %v", err)
	}

	// Test GET returns GraphiQL
	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d for GET /graphql, got %d", http.StatusOK, w.Code)
	}

	// Test POST handles queries
	query := `{"query": "{ categories { id } }"}`
	req = httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d for POST /graphql, got %d", http.StatusOK, w.Code)
	}
}

// GraphQL resolver tests

func TestGraphQLResolveInfo(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ info { name version description uptime mode engines { total enabled } system { goVersion numCpu numGoroutine memAlloc } } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok || data["info"] == nil {
		t.Error("Expected info data in response")
	}
}

func TestGraphQLResolveCategories(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ categories { id name description icon } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data in response")
	}

	categories, ok := data["categories"].([]interface{})
	if !ok {
		t.Fatal("Expected categories array")
	}

	if len(categories) != 5 {
		t.Errorf("Expected 5 categories, got %d", len(categories))
	}
}

func TestGraphQLResolveEngines(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ engines { id name enabled priority categories } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGraphQLResolveEngineNotFound(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ engine(id: \"nonexistent\") { id name } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have errors
	if result["errors"] == nil {
		t.Error("Expected errors for nonexistent engine")
	}
}

func TestGraphQLResolveBangs(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ bangs { bangs { shortcut name url category } total categories } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok || data["bangs"] == nil {
		t.Error("Expected bangs data in response")
	}
}

func TestGraphQLResolveBangsWithFilter(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ bangs(category: \"code\", search: \"github\") { bangs { shortcut name } total } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGraphQLResolveAutocomplete(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ autocomplete(query: \"test\") }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGraphQLResolveAutocompleteEmpty(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ autocomplete(query: \"\") }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data in response")
	}

	autocomplete, ok := data["autocomplete"].([]interface{})
	if !ok {
		t.Fatal("Expected autocomplete array")
	}

	if len(autocomplete) != 0 {
		t.Errorf("Expected empty autocomplete for empty query, got %d items", len(autocomplete))
	}
}

func TestGraphQLResolveWidgetsNoManager(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ widgets { type name description } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data in response")
	}

	widgets, ok := data["widgets"].([]interface{})
	if !ok {
		t.Fatal("Expected widgets array")
	}

	if len(widgets) != 0 {
		t.Errorf("Expected empty widgets when manager is nil, got %d", len(widgets))
	}
}

func TestGraphQLResolveInstantNoManager(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ instant(query: \"test\") { query found } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected data in response")
	}

	instant, ok := data["instant"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected instant object")
	}

	if instant["found"] != false {
		t.Error("Expected found to be false when instant manager is nil")
	}
}

func TestGraphQLResolveSearch(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ search(query: \"test\") { query category results { title url } totalResults page limit hasMore searchTimeMs enginesUsed } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	// May fail if aggregator fails, but response format should be valid
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGraphQLResolveSearchWithOptions(t *testing.T) {
	handler := newTestHandler()
	gqlHandler, err := NewGraphQLHandler(handler)
	if err != nil {
		t.Fatalf("Failed to create GraphQL handler: %v", err)
	}

	query := `{"query": "{ search(query: \"test\", category: \"images\", page: 2, limit: 10) { query category page limit } }"}`
	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(query))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gqlHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// ============================================================================
// Additional api.go tests for edge cases
// ============================================================================

func TestSearchEndpointPOST(t *testing.T) {
	handler := newTestHandler()

	body := `{"query": "test", "category": "images", "page": 1, "limit": 10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	// Response should be valid JSON
	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
}

func TestSearchEndpointPOSTInvalidJSON(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSearchEndpointAllCategories(t *testing.T) {
	handler := newTestHandler()

	categories := []string{"general", "images", "videos", "news", "maps", "unknown"}
	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&category="+cat, nil)
			w := httptest.NewRecorder()
			handler.handleSearch(w, req)

			var response APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
		})
	}
}

func TestSearchEndpointLimitBounds(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		limit    string
		expected int
	}{
		{"0", 20},    // Should default to 20
		{"-1", 20},   // Should default to 20
		{"200", 20},  // Should cap to 20 (>100 triggers default)
		{"50", 50},   // Valid limit
	}

	for _, tt := range tests {
		t.Run(tt.limit, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit="+tt.limit, nil)
			w := httptest.NewRecorder()
			handler.handleSearch(w, req)

			var response APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.OK {
				data, ok := response.Data.(map[string]interface{})
				if ok && data["limit"] != nil {
					if int(data["limit"].(float64)) != tt.expected {
						t.Errorf("Expected limit %d, got %v", tt.expected, data["limit"])
					}
				}
			}
		})
	}
}

func TestSearchEndpointPageBounds(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		page     string
		expected int
	}{
		{"0", 1},  // Should default to 1
		{"-1", 1}, // Should default to 1
		{"5", 5},  // Valid page
	}

	for _, tt := range tests {
		t.Run(tt.page, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&page="+tt.page, nil)
			w := httptest.NewRecorder()
			handler.handleSearch(w, req)

			var response APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.OK {
				data, ok := response.Data.(map[string]interface{})
				if ok && data["page"] != nil {
					if int(data["page"].(float64)) != tt.expected {
						t.Errorf("Expected page %d, got %v", tt.expected, data["page"])
					}
				}
			}
		})
	}
}

func TestSearchEndpointQueryTrimming(t *testing.T) {
	handler := newTestHandler()

	// Test that whitespace-only query is rejected
	// Use URL-encoded spaces (%20) to ensure proper parsing
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=%20%20%20", nil)
	w := httptest.NewRecorder()
	handler.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d for whitespace query, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHealthzMaintenanceMode(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title:           "Test Search",
			Description:     "Test Description",
			Mode:            "production",
			MaintenanceMode: true,
		},
	}
	registry := engines.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d for maintenance mode, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["status"] != "maintenance" {
		t.Errorf("Expected status 'maintenance', got %v", data["status"])
	}
}

func TestHealthzTextFormatError(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title:           "Test Search",
			MaintenanceMode: true,
		},
	}
	registry := engines.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz.txt", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	body := w.Body.String()
	if !strings.HasPrefix(body, "ERROR:") {
		t.Errorf("Expected 'ERROR:' prefix in maintenance mode, got %q", body)
	}
}

func TestRespondWithFormatTxtParam(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz?format=txt", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

func TestRespondWithFormatPlainParam(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz?format=plain", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

func TestRespondWithFormatJSONOverride(t *testing.T) {
	handler := newTestHandler()

	// Accept header with both text/plain and application/json should use JSON
	req := httptest.NewRequest(http.MethodGet, "/api/v1/healthz", nil)
	req.Header.Set("Accept", "text/plain, application/json")
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json when both are accepted", contentType)
	}
}

func TestInfoEndpointDetails(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	w := httptest.NewRecorder()

	handler.handleInfo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	// Check system info is present
	system, ok := data["system"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected system info")
	}

	if system["go_version"] == nil {
		t.Error("Expected go_version in system info")
	}
	if system["num_cpu"] == nil {
		t.Error("Expected num_cpu in system info")
	}
}

func TestRelatedSearchesWithLimit(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related?q=test&limit=5", nil)
	w := httptest.NewRecorder()

	handler.handleRelatedSearches(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRelatedSearchesInvalidLimit(t *testing.T) {
	handler := newTestHandler()

	// Test with invalid limit values
	tests := []string{"-1", "0", "100", "abc"}
	for _, limit := range tests {
		t.Run(limit, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related?q=test&limit="+limit, nil)
			w := httptest.NewRecorder()

			handler.handleRelatedSearches(w, req)

			// Should still return OK (uses default limit)
			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			}
		})
	}
}

func TestAutodiscoverHTTPS(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	req.Host = "search.example.com"
	// Simulate TLS by setting TLS field (would need to mock properly for full test)
	w := httptest.NewRecorder()

	handler.handleAutodiscover(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRegisterRoutes(t *testing.T) {
	handler := newTestHandler()
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test all registered routes
	routes := []struct {
		path   string
		method string
	}{
		{"/api/autodiscover", http.MethodGet},
		{"/api/v1/healthz", http.MethodGet},
		{"/api/v1/healthz.txt", http.MethodGet},
		{"/api/v1/info", http.MethodGet},
		{"/api/v1/info.txt", http.MethodGet},
		{"/api/v1/categories", http.MethodGet},
		{"/api/v1/engines", http.MethodGet},
		{"/api/v1/bangs", http.MethodGet},
		{"/api/v1/autocomplete", http.MethodGet},
		{"/api/v1/widgets", http.MethodGet},
		{"/api/v1/instant", http.MethodGet},
		{"/api/v1/search/related", http.MethodGet},
	}

	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// All routes should return either 200, 400, or 503 (but not 404)
			if w.Code == http.StatusNotFound {
				t.Errorf("Route %s not registered", route.path)
			}
		})
	}
}

func TestHandleAutocompletePublicMethod(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete?q=test", nil)
	w := httptest.NewRecorder()

	// Test the public HandleAutocomplete method
	handler.HandleAutocomplete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGetRequestsTotalMetricsEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
			Metrics: config.MetricsConfig{
				Enabled: true,
			},
		},
	}
	registry := engines.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	// Even with metrics enabled, returns 0 (no actual metrics implementation)
	result := handler.getRequestsTotal()
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}

	result24h := handler.getRequests24h()
	if result24h != 0 {
		t.Errorf("Expected 0 for 24h, got %d", result24h)
	}
}

func TestWidgetDataEmptyType(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
		},
	}
	registry := engines.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)
	// Leave widgetManager as nil

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	// Returns 200 OK with error message (graceful degradation)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// ============================================================================
// Tests for InstantAnswerResponse serialization
// ============================================================================

func TestInstantAnswerResponseSerialization(t *testing.T) {
	resp := InstantAnswerResponse{
		Query:   "weather",
		Type:    "weather",
		Title:   "Weather Forecast",
		Content: "Sunny, 72F",
		Data:    map[string]interface{}{"temp": 72, "condition": "sunny"},
		Source:  "weather_api",
		Found:   true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal InstantAnswerResponse: %v", err)
	}

	var unmarshaled InstantAnswerResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal InstantAnswerResponse: %v", err)
	}

	if unmarshaled.Query != resp.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, resp.Query)
	}
	if unmarshaled.Found != resp.Found {
		t.Errorf("Found mismatch: got %v, want %v", unmarshaled.Found, resp.Found)
	}
}

// ============================================================================
// Tests for RelatedSearchResponse serialization
// ============================================================================

func TestRelatedSearchResponseSerialization(t *testing.T) {
	resp := RelatedSearchResponse{
		Query:       "golang",
		Suggestions: []string{"golang tutorial", "golang vs python", "golang web framework"},
		Count:       3,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal RelatedSearchResponse: %v", err)
	}

	var unmarshaled RelatedSearchResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal RelatedSearchResponse: %v", err)
	}

	if unmarshaled.Query != resp.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, resp.Query)
	}
	if unmarshaled.Count != resp.Count {
		t.Errorf("Count mismatch: got %d, want %d", unmarshaled.Count, resp.Count)
	}
	if len(unmarshaled.Suggestions) != len(resp.Suggestions) {
		t.Errorf("Suggestions length mismatch: got %d, want %d", len(unmarshaled.Suggestions), len(resp.Suggestions))
	}
}

// ============================================================================
// Tests for auth.go handlers
// ============================================================================

func TestNewAuthHandler(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test",
		},
	}

	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)
	if handler == nil {
		t.Fatal("NewAuthHandler returned nil")
	}
	if handler.config != cfg {
		t.Error("Config not set correctly")
	}
}

func TestAuthHandlerRegisterRoutes(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test",
		},
	}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered
	routes := []string{
		"/api/v1/auth/register",
		"/api/v1/auth/login",
		"/api/v1/auth/logout",
		"/api/v1/auth/forgot",
		"/api/v1/auth/reset",
		"/api/v1/auth/recovery",
		"/api/v1/auth/verify",
		"/api/v1/auth/2fa/verify",
		"/api/v1/auth/session",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// Should not be 404
			if w.Code == http.StatusNotFound {
				t.Errorf("Route %s not registered", route)
			}
		})
	}
}

func TestAuthHandlerRegisterMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/register", nil)
	w := httptest.NewRecorder()

	handler.handleRegister(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandlerRegisterDisabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Users: config.UsersConfig{
				Registration: struct {
					Enabled                  bool     `yaml:"enabled"`
					RequireEmailVerification bool     `yaml:"require_email_verification"`
					RequireApproval          bool     `yaml:"require_approval"`
					AllowedDomains           []string `yaml:"allowed_domains"`
					BlockedDomains           []string `yaml:"blocked_domains"`
				}{
					Enabled: false,
				},
			},
		},
	}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader("{}"))
	w := httptest.NewRecorder()

	handler.handleRegister(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d for disabled registration, got %d", http.StatusForbidden, w.Code)
	}
}

func TestAuthHandlerRegisterInvalidJSON(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Users: config.UsersConfig{
				Registration: struct {
					Enabled                  bool     `yaml:"enabled"`
					RequireEmailVerification bool     `yaml:"require_email_verification"`
					RequireApproval          bool     `yaml:"require_approval"`
					AllowedDomains           []string `yaml:"allowed_domains"`
					BlockedDomains           []string `yaml:"blocked_domains"`
				}{
					Enabled: true,
				},
			},
		},
	}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()

	handler.handleRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerRegisterMissingFields(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Users: config.UsersConfig{
				Registration: struct {
					Enabled                  bool     `yaml:"enabled"`
					RequireEmailVerification bool     `yaml:"require_email_verification"`
					RequireApproval          bool     `yaml:"require_approval"`
					AllowedDomains           []string `yaml:"allowed_domains"`
					BlockedDomains           []string `yaml:"blocked_domains"`
				}{
					Enabled: true,
				},
			},
		},
	}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"username": "test"}`))
	w := httptest.NewRecorder()

	handler.handleRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !strings.Contains(response.Message, "required") {
		t.Error("Expected error message about required fields")
	}
}

func TestAuthHandlerRegisterDomainRestriction(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Users: config.UsersConfig{
				Registration: struct {
					Enabled                  bool     `yaml:"enabled"`
					RequireEmailVerification bool     `yaml:"require_email_verification"`
					RequireApproval          bool     `yaml:"require_approval"`
					AllowedDomains           []string `yaml:"allowed_domains"`
					BlockedDomains           []string `yaml:"blocked_domains"`
				}{
					Enabled:        true,
					AllowedDomains: []string{"allowed.com"},
				},
			},
		},
	}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	body := `{"username": "test", "email": "test@notallowed.com", "password": "password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler.handleRegister(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d for restricted domain, got %d", http.StatusForbidden, w.Code)
	}
}

func TestAuthHandlerRegisterInvalidEmail(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Users: config.UsersConfig{
				Registration: struct {
					Enabled                  bool     `yaml:"enabled"`
					RequireEmailVerification bool     `yaml:"require_email_verification"`
					RequireApproval          bool     `yaml:"require_approval"`
					AllowedDomains           []string `yaml:"allowed_domains"`
					BlockedDomains           []string `yaml:"blocked_domains"`
				}{
					Enabled:        true,
					AllowedDomains: []string{"allowed.com"},
				},
			},
		},
	}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	body := `{"username": "test", "email": "invalid-email", "password": "password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	w := httptest.NewRecorder()

	handler.handleRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d for invalid email, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerLoginMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/login", nil)
	w := httptest.NewRecorder()

	handler.handleLogin(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandlerLoginInvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader("invalid"))
	w := httptest.NewRecorder()

	handler.handleLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerLoginMissingCredentials(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username": "test"}`))
	w := httptest.NewRecorder()

	handler.handleLogin(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerLogoutMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()

	handler.handleLogout(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandlerForgotPasswordMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/forgot", nil)
	w := httptest.NewRecorder()

	handler.handleForgotPassword(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandlerForgotPasswordInvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot", strings.NewReader("invalid"))
	w := httptest.NewRecorder()

	handler.handleForgotPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerForgotPasswordEmptyEmail(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	handler.handleForgotPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerForgotPasswordSuccess(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot", strings.NewReader(`{"email": "test@example.com"}`))
	w := httptest.NewRecorder()

	handler.handleForgotPassword(w, req)

	// Should always return success to prevent email enumeration
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestAuthHandlerResetPasswordMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/reset", nil)
	w := httptest.NewRecorder()

	handler.handleResetPassword(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandlerResetPasswordInvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset", strings.NewReader("invalid"))
	w := httptest.NewRecorder()

	handler.handleResetPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerResetPasswordMissingFields(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset", strings.NewReader(`{"token": "abc"}`))
	w := httptest.NewRecorder()

	handler.handleResetPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerResetPasswordNoManager(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/reset", strings.NewReader(`{"token": "abc", "new_password": "newpass123"}`))
	w := httptest.NewRecorder()

	handler.handleResetPassword(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestAuthHandlerRecoveryKeyMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/recovery", nil)
	w := httptest.NewRecorder()

	handler.handleRecoveryKey(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandlerRecoveryKeyInvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery", strings.NewReader("invalid"))
	w := httptest.NewRecorder()

	handler.handleRecoveryKey(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerRecoveryKeyMissingFields(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/recovery", strings.NewReader(`{"username": "test"}`))
	w := httptest.NewRecorder()

	handler.handleRecoveryKey(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerVerifyEmailMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/verify", nil)
	w := httptest.NewRecorder()

	handler.handleVerifyEmail(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandlerVerifyEmailInvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", strings.NewReader("invalid"))
	w := httptest.NewRecorder()

	handler.handleVerifyEmail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerVerifyEmailMissingToken(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", strings.NewReader(`{}`))
	w := httptest.NewRecorder()

	handler.handleVerifyEmail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerVerifyEmailNoManager(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify", strings.NewReader(`{"token": "abc"}`))
	w := httptest.NewRecorder()

	handler.handleVerifyEmail(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestAuthHandler2FAVerifyMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/2fa/verify", nil)
	w := httptest.NewRecorder()

	handler.handle2FAVerify(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAuthHandler2FAVerifyInvalidJSON(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader("invalid"))
	w := httptest.NewRecorder()

	handler.handle2FAVerify(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandler2FAVerifyMissingFields(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/2fa/verify", strings.NewReader(`{"code": "123456"}`))
	w := httptest.NewRecorder()

	handler.handle2FAVerify(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAuthHandlerSessionMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewAuthHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/session", nil)
	w := httptest.NewRecorder()

	handler.handleSession(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// ============================================================================
// Tests for user.go handlers
// ============================================================================

func TestNewUserHandler(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test",
		},
	}

	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)
	if handler == nil {
		t.Fatal("NewUserHandler returned nil")
	}
	if handler.config != cfg {
		t.Error("Config not set correctly")
	}
}

func TestUserHandlerRegisterRoutes(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()

	handler.RegisterRoutes(mux)

	// Test that routes are registered
	routes := []string{
		"/api/v1/users/profile",
		"/api/v1/users/password",
		"/api/v1/users/sessions",
		"/api/v1/users/tokens",
		"/api/v1/users/2fa/status",
		"/api/v1/users/2fa/setup",
		"/api/v1/users/2fa/enable",
		"/api/v1/users/2fa/disable",
		"/api/v1/users/recovery-keys",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// Should not be 404
			if w.Code == http.StatusNotFound {
				t.Errorf("Route %s not registered", route)
			}
		})
	}
}

func TestUserHandlerProfileNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/profile", nil)
	w := httptest.NewRecorder()

	handler.handleProfile(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerProfileMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/profile", nil)
	w := httptest.NewRecorder()

	handler.handleProfile(w, req)

	// Without auth, should return unauthorized first
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerPasswordMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/password", nil)
	w := httptest.NewRecorder()

	handler.handlePassword(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserHandlerPasswordNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/users/password", nil)
	w := httptest.NewRecorder()

	handler.handlePassword(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerSessionsNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/sessions", nil)
	w := httptest.NewRecorder()

	handler.handleSessions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerSessionByIDMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/sessions/123", nil)
	w := httptest.NewRecorder()

	handler.handleSessionByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserHandlerSessionByIDNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/sessions/123", nil)
	w := httptest.NewRecorder()

	handler.handleSessionByID(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerTokensNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/tokens", nil)
	w := httptest.NewRecorder()

	handler.handleTokens(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerTokenByIDMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/tokens/123", nil)
	w := httptest.NewRecorder()

	handler.handleTokenByID(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserHandlerTokenByIDNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/tokens/123", nil)
	w := httptest.NewRecorder()

	handler.handleTokenByID(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandler2FAStatusMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/2fa/status", nil)
	w := httptest.NewRecorder()

	handler.handle2FAStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserHandler2FAStatusNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/2fa/status", nil)
	w := httptest.NewRecorder()

	handler.handle2FAStatus(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandler2FASetupMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/2fa/setup", nil)
	w := httptest.NewRecorder()

	handler.handle2FASetup(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserHandler2FASetupNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/2fa/setup", nil)
	w := httptest.NewRecorder()

	handler.handle2FASetup(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandler2FAEnableMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/2fa/enable", nil)
	w := httptest.NewRecorder()

	handler.handle2FAEnable(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserHandler2FAEnableNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/2fa/enable", nil)
	w := httptest.NewRecorder()

	handler.handle2FAEnable(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandler2FADisableMethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/2fa/disable", nil)
	w := httptest.NewRecorder()

	handler.handle2FADisable(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestUserHandler2FADisableNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/2fa/disable", nil)
	w := httptest.NewRecorder()

	handler.handle2FADisable(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

func TestUserHandlerRecoveryKeysNotAuthenticated(t *testing.T) {
	cfg := &config.Config{}
	handler := NewUserHandler(cfg, nil, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/recovery-keys", nil)
	w := httptest.NewRecorder()

	handler.handleRecoveryKeys(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
}

// ============================================================================
// Tests for request struct serialization (user.go)
// ============================================================================

func TestUpdateProfileRequestSerialization(t *testing.T) {
	req := UpdateProfileRequest{
		DisplayName:       "Test User",
		Bio:               "A test user bio",
		AvatarURL:         "https://example.com/avatar.png",
		NotificationEmail: "notify@example.com",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal UpdateProfileRequest: %v", err)
	}

	var unmarshaled UpdateProfileRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal UpdateProfileRequest: %v", err)
	}

	if unmarshaled.DisplayName != req.DisplayName {
		t.Errorf("DisplayName mismatch: got %q, want %q", unmarshaled.DisplayName, req.DisplayName)
	}
}

func TestChangePasswordRequestSerialization(t *testing.T) {
	req := ChangePasswordRequest{
		CurrentPassword: "oldpass123",
		NewPassword:     "newpass456",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal ChangePasswordRequest: %v", err)
	}

	var unmarshaled ChangePasswordRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal ChangePasswordRequest: %v", err)
	}

	if unmarshaled.CurrentPassword != req.CurrentPassword {
		t.Errorf("CurrentPassword mismatch")
	}
}

func TestCreateTokenRequestSerialization(t *testing.T) {
	req := CreateTokenRequest{
		Name:        "API Token",
		Permissions: []string{"read", "write"},
		ExpiresIn:   30,
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal CreateTokenRequest: %v", err)
	}

	var unmarshaled CreateTokenRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal CreateTokenRequest: %v", err)
	}

	if unmarshaled.Name != req.Name {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, req.Name)
	}
	if len(unmarshaled.Permissions) != len(req.Permissions) {
		t.Errorf("Permissions length mismatch")
	}
}

func TestSetup2FARequestSerialization(t *testing.T) {
	req := Setup2FARequest{
		Password: "securepass123",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal Setup2FARequest: %v", err)
	}

	var unmarshaled Setup2FARequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal Setup2FARequest: %v", err)
	}

	if unmarshaled.Password != req.Password {
		t.Errorf("Password mismatch")
	}
}

func TestEnable2FARequestSerialization(t *testing.T) {
	req := Enable2FARequest{
		Code: "123456",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal Enable2FARequest: %v", err)
	}

	var unmarshaled Enable2FARequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal Enable2FARequest: %v", err)
	}

	if unmarshaled.Code != req.Code {
		t.Errorf("Code mismatch: got %q, want %q", unmarshaled.Code, req.Code)
	}
}

func TestDisable2FARequestSerialization(t *testing.T) {
	req := Disable2FARequest{
		Password: "securepass123",
		Code:     "654321",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal Disable2FARequest: %v", err)
	}

	var unmarshaled Disable2FARequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal Disable2FARequest: %v", err)
	}

	if unmarshaled.Password != req.Password {
		t.Errorf("Password mismatch")
	}
	if unmarshaled.Code != req.Code {
		t.Errorf("Code mismatch")
	}
}

func TestSessionInfoSerialization(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	info := SessionInfo{
		ID:         123,
		DeviceName: "Chrome on Windows",
		IPAddress:  "192.168.1.1",
		CreatedAt:  now,
		LastUsed:   now,
		ExpiresAt:  now.Add(24 * time.Hour),
		IsCurrent:  true,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal SessionInfo: %v", err)
	}

	var unmarshaled SessionInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SessionInfo: %v", err)
	}

	if unmarshaled.ID != info.ID {
		t.Errorf("ID mismatch: got %d, want %d", unmarshaled.ID, info.ID)
	}
	if unmarshaled.DeviceName != info.DeviceName {
		t.Errorf("DeviceName mismatch: got %q, want %q", unmarshaled.DeviceName, info.DeviceName)
	}
	if unmarshaled.IsCurrent != info.IsCurrent {
		t.Errorf("IsCurrent mismatch: got %v, want %v", unmarshaled.IsCurrent, info.IsCurrent)
	}
}

// ============================================================================
// Tests for SwaggerUIData
// ============================================================================

func TestSwaggerUIDataSerialization(t *testing.T) {
	data := SwaggerUIData{
		SpecURL: "https://api.example.com/openapi.json",
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal SwaggerUIData: %v", err)
	}

	var unmarshaled SwaggerUIData
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SwaggerUIData: %v", err)
	}

	if unmarshaled.SpecURL != data.SpecURL {
		t.Errorf("SpecURL mismatch: got %q, want %q", unmarshaled.SpecURL, data.SpecURL)
	}
}

// ============================================================================
// Tests for getHostname
// ============================================================================

func TestGetHostname(t *testing.T) {
	hostname, err := getHostname()
	if err != nil {
		t.Errorf("getHostname() returned error: %v", err)
	}
	if hostname == "" {
		t.Error("getHostname() returned empty string")
	}
}
