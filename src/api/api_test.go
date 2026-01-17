package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	if data["enabled"] != false {
		t.Error("Expected enabled to be false when widget manager is nil")
	}
}

// Tests for Widget data endpoint

func TestWidgetDataNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/weather", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
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
		Checks:    map[string]string{"search": "ok"},
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
		Query:        "test",
		Category:     "general",
		Results:      []SearchResult{{Title: "Test", URL: "https://example.com"}},
		TotalResults: 1,
		Page:         1,
		Limit:        20,
		HasMore:      false,
		SearchTime:   100.5,
		Engines:      []string{"google"},
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
