package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
	gql "github.com/graphql-go/graphql"
)

// Tests for InitSchema

func TestInitSchema(t *testing.T) {
	err := InitSchema()
	if err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	// Schema should be initialized
	if Schema.QueryType() == nil {
		t.Error("Schema QueryType should not be nil after initialization")
	}
}

// Tests for resolvers

func TestResolveSearch(t *testing.T) {
	// Initialize schema first
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	params := gql.ResolveParams{
		Args: map[string]interface{}{"q": "test query"},
	}

	result, err := resolveSearch(params)
	if err != nil {
		t.Fatalf("resolveSearch() error = %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("resolveSearch() result type = %T, want map", result)
	}

	if resultMap["query"] != "test query" {
		t.Errorf("query = %v", resultMap["query"])
	}
}

func TestResolveAutocomplete(t *testing.T) {
	params := gql.ResolveParams{
		Args: map[string]interface{}{"q": "test"},
	}

	result, err := resolveAutocomplete(params)
	if err != nil {
		t.Fatalf("resolveAutocomplete() error = %v", err)
	}

	suggestions, ok := result.([]string)
	if !ok {
		t.Fatalf("resolveAutocomplete() result type = %T, want []string", result)
	}

	// Returns empty by design (use REST API for full functionality)
	if len(suggestions) != 0 {
		t.Errorf("Expected empty suggestions, got %v", suggestions)
	}
}

func TestResolveHealth(t *testing.T) {
	params := gql.ResolveParams{}

	result, err := resolveHealth(params)
	if err != nil {
		t.Fatalf("resolveHealth() error = %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("resolveHealth() result type = %T, want map", result)
	}

	if resultMap["status"] != "healthy" {
		t.Errorf("status = %v, want healthy", resultMap["status"])
	}
	if resultMap["mode"] != "production" {
		t.Errorf("mode = %v, want production", resultMap["mode"])
	}
}

// Tests for Handler

func TestHandlerPOST(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	// Create a GraphQL query request
	query := map[string]interface{}{
		"query": `{ health { status version } }`,
	}
	body, _ := json.Marshal(query)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	// Check response contains data
	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if _, hasData := response["data"]; !hasData {
		t.Error("Response should contain 'data' field")
	}
}

func TestHandlerGET(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}

	// Should serve GraphiQL UI
	body := rec.Body.String()
	if !strings.Contains(body, "GraphiQL") {
		t.Error("Response should contain GraphiQL")
	}
	if !strings.Contains(body, "graphiql") {
		t.Error("Response should contain graphiql element")
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodPut, "/graphql", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandlerInvalidJSON(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

// Tests for buildBaseURL

func TestBuildBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		headers   map[string]string
		wantProto string
		wantHost  string
	}{
		{
			name:      "default HTTP",
			host:      "localhost:8080",
			headers:   nil,
			wantProto: "http",
			wantHost:  "localhost:8080",
		},
		{
			name:      "forwarded HTTPS",
			host:      "localhost:8080",
			headers:   map[string]string{"X-Forwarded-Proto": "https"},
			wantProto: "https",
			wantHost:  "localhost:8080",
		},
		{
			name:      "forwarded host",
			host:      "localhost:8080",
			headers:   map[string]string{"X-Forwarded-Host": "example.com"},
			wantProto: "http",
			wantHost:  "example.com",
		},
		{
			name: "forwarded both",
			host: "localhost:8080",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
				"X-Forwarded-Host":  "example.com",
			},
			wantProto: "https",
			wantHost:  "example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			url := buildBaseURL(req)

			if !strings.HasPrefix(url, tt.wantProto+"://") {
				t.Errorf("buildBaseURL() = %q, want protocol %q", url, tt.wantProto)
			}
			if !strings.Contains(url, tt.wantHost) {
				t.Errorf("buildBaseURL() = %q, want host %q", url, tt.wantHost)
			}
		})
	}
}

// Tests for theme functions

func TestGetTheme(t *testing.T) {
	// Test default theme
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	theme := getTheme(req)

	if theme != "dark" && theme != "light" {
		t.Errorf("getTheme() = %q, want dark or light", theme)
	}

	// Test with cookie
	reqWithCookie := httptest.NewRequest(http.MethodGet, "/", nil)
	reqWithCookie.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	themeFromCookie := getTheme(reqWithCookie)

	if themeFromCookie != "light" {
		t.Errorf("getTheme() with cookie = %q, want light", themeFromCookie)
	}
}

func TestGetGraphiQLThemeCSS(t *testing.T) {
	// Test dark theme
	darkCSS := getGraphiQLThemeCSS("dark")
	if darkCSS == "" {
		t.Error("getGraphiQLThemeCSS('dark') returned empty")
	}

	// Test light theme
	lightCSS := getGraphiQLThemeCSS("light")
	if lightCSS == "" {
		t.Error("getGraphiQLThemeCSS('light') returned empty")
	}

	// Dark and light should be different
	if darkCSS == lightCSS {
		t.Error("Dark and light themes should be different")
	}
}

// Tests for search query via GraphQL

func TestGraphQLSearchQuery(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query": `{
			search(q: "test query") {
				query
				totalResults
				searchTime
			}
		}`,
	}
	body, _ := json.Marshal(query)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check data is present
	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should contain 'data' map")
	}

	search, ok := data["search"].(map[string]interface{})
	if !ok {
		t.Fatal("Data should contain 'search' map")
	}

	if search["query"] != "test query" {
		t.Errorf("search.query = %v, want 'test query'", search["query"])
	}
}
