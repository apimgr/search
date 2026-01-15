package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
