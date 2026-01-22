package graphql

import (
	"bytes"
	"crypto/tls"
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

// Tests for GraphQL autocomplete query via handler

func TestGraphQLAutocompleteQuery(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query": `{
			autocomplete(q: "test")
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

	// Autocomplete returns empty array by design
	autocomplete, ok := data["autocomplete"].([]interface{})
	if !ok {
		t.Fatal("Data should contain 'autocomplete' array")
	}

	if len(autocomplete) != 0 {
		t.Errorf("autocomplete should return empty array, got %v", autocomplete)
	}
}

// Tests for GraphQL query with variables and operationName

func TestGraphQLQueryWithVariablesAndOperationName(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query": `query SearchQuery($q: String!) {
			search(q: $q) {
				query
				totalResults
			}
		}`,
		"variables": map[string]interface{}{
			"q": "variable test",
		},
		"operationName": "SearchQuery",
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

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should contain 'data' map")
	}

	search, ok := data["search"].(map[string]interface{})
	if !ok {
		t.Fatal("Data should contain 'search' map")
	}

	if search["query"] != "variable test" {
		t.Errorf("search.query = %v, want 'variable test'", search["query"])
	}
}

// Tests for buildBaseURL with TLS

func TestBuildBaseURLWithTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8443"
	// Simulate TLS connection
	req.TLS = &tls.ConnectionState{}

	url := buildBaseURL(req)

	if !strings.HasPrefix(url, "https://") {
		t.Errorf("buildBaseURL() with TLS = %q, want https:// prefix", url)
	}
	if !strings.Contains(url, "localhost:8443") {
		t.Errorf("buildBaseURL() = %q, want host localhost:8443", url)
	}
}

func TestBuildBaseURLWithTLSAndForwardedProto(t *testing.T) {
	// When X-Forwarded-Proto is set, it should override TLS detection
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8443"
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("X-Forwarded-Proto", "http") // Override to http

	url := buildBaseURL(req)

	// X-Forwarded-Proto should override TLS detection
	if !strings.HasPrefix(url, "http://") {
		t.Errorf("buildBaseURL() with X-Forwarded-Proto override = %q, want http:// prefix", url)
	}
}

// Additional tests for getTheme

func TestGetThemeWithDarkCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})

	theme := getTheme(req)
	if theme != "dark" {
		t.Errorf("getTheme() with dark cookie = %q, want dark", theme)
	}
}

func TestGetThemeWithAutoCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "auto"})

	theme := getTheme(req)
	if theme != "auto" {
		t.Errorf("getTheme() with auto cookie = %q, want auto", theme)
	}
}

func TestGetThemeWithInvalidCookie(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "invalid"})

	theme := getTheme(req)
	if theme != "dark" {
		t.Errorf("getTheme() with invalid cookie = %q, want dark (default)", theme)
	}
}

func TestGetThemeDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	theme := getTheme(req)
	if theme != "dark" {
		t.Errorf("getTheme() without cookie = %q, want dark (default)", theme)
	}
}

// Additional tests for getGraphiQLThemeCSS

func TestGetGraphiQLThemeCSSDefault(t *testing.T) {
	// Test with a value that's neither "light" nor "dark" - should return dark
	css := getGraphiQLThemeCSS("auto")
	if css != graphiqlDarkTheme {
		t.Error("getGraphiQLThemeCSS('auto') should return dark theme by default")
	}
}

func TestGetGraphiQLThemeCSSEmptyString(t *testing.T) {
	// Test with empty string - should return dark
	css := getGraphiQLThemeCSS("")
	if css != graphiqlDarkTheme {
		t.Error("getGraphiQLThemeCSS('') should return dark theme by default")
	}
}

// Tests for GraphiQL serving with different themes

func TestServeGraphiQLWithLightTheme(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	// Light theme should have specific CSS
	if !strings.Contains(body, "background: #ffffff") {
		t.Error("Light theme should contain white background CSS")
	}
}

func TestServeGraphiQLWithDarkTheme(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	req.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	// Dark theme should have specific CSS
	if !strings.Contains(body, "background: #282a36") {
		t.Error("Dark theme should contain dark background CSS")
	}
}

// Test GraphQL health query with all fields

func TestGraphQLHealthQueryAllFields(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query": `{
			health {
				status
				version
				uptime
				mode
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

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should contain 'data' map")
	}

	health, ok := data["health"].(map[string]interface{})
	if !ok {
		t.Fatal("Data should contain 'health' map")
	}

	if health["status"] != "healthy" {
		t.Errorf("health.status = %v, want healthy", health["status"])
	}
	if health["mode"] != "production" {
		t.Errorf("health.mode = %v, want production", health["mode"])
	}
	if health["uptime"] != "0s" {
		t.Errorf("health.uptime = %v, want 0s", health["uptime"])
	}
	if health["version"] != config.Version {
		t.Errorf("health.version = %v, want %s", health["version"], config.Version)
	}
}

// Test search query with all optional arguments

func TestGraphQLSearchQueryWithAllArgs(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query": `{
			search(q: "test", category: "images", page: 2, lang: "en") {
				query
				results {
					title
					url
					content
					engine
					score
					imageUrl
					thumbnail
					published
				}
				totalResults
				searchTime
				engines
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

	// Should not have errors
	if errors, ok := response["errors"]; ok {
		t.Errorf("Unexpected errors: %v", errors)
	}

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should contain 'data' map")
	}

	search, ok := data["search"].(map[string]interface{})
	if !ok {
		t.Fatal("Data should contain 'search' map")
	}

	if search["query"] != "test" {
		t.Errorf("search.query = %v, want 'test'", search["query"])
	}
}

// Test DELETE method (another method not allowed case)

func TestHandlerDELETEMethodNotAllowed(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodDelete, "/graphql", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// Test PATCH method not allowed

func TestHandlerPATCHMethodNotAllowed(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodPatch, "/graphql", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// Test GraphQL introspection query

func TestGraphQLIntrospectionQuery(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	// Basic introspection query
	query := map[string]interface{}{
		"query": `{
			__schema {
				queryType {
					name
				}
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

	data, ok := response["data"].(map[string]interface{})
	if !ok {
		t.Fatal("Response should contain 'data' map")
	}

	schema, ok := data["__schema"].(map[string]interface{})
	if !ok {
		t.Fatal("Data should contain '__schema' map")
	}

	queryType, ok := schema["queryType"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema should contain 'queryType' map")
	}

	if queryType["name"] != "Query" {
		t.Errorf("queryType.name = %v, want Query", queryType["name"])
	}
}

// Test resolvers with additional edge cases

func TestResolveSearchResultFields(t *testing.T) {
	params := gql.ResolveParams{
		Args: map[string]interface{}{"q": "edge case test"},
	}

	result, err := resolveSearch(params)
	if err != nil {
		t.Fatalf("resolveSearch() error = %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("resolveSearch() result type = %T, want map", result)
	}

	// Check all fields are present
	if resultMap["query"] != "edge case test" {
		t.Errorf("query = %v, want 'edge case test'", resultMap["query"])
	}
	if resultMap["totalResults"] != 0 {
		t.Errorf("totalResults = %v, want 0", resultMap["totalResults"])
	}
	if resultMap["searchTime"] != 0.0 {
		t.Errorf("searchTime = %v, want 0.0", resultMap["searchTime"])
	}

	results, ok := resultMap["results"].([]interface{})
	if !ok {
		t.Errorf("results type = %T, want []interface{}", resultMap["results"])
	}
	if len(results) != 0 {
		t.Errorf("results length = %d, want 0", len(results))
	}

	engines, ok := resultMap["engines"].([]string)
	if !ok {
		t.Errorf("engines type = %T, want []string", resultMap["engines"])
	}
	if len(engines) != 0 {
		t.Errorf("engines length = %d, want 0", len(engines))
	}
}

// Test empty query body

func TestHandlerEmptyQuery(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query": "",
	}
	body, _ := json.Marshal(query)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	// GraphQL should still return 200 OK even for empty query (but with errors in response)
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

// Test GraphQL syntax error in query

func TestHandlerGraphQLSyntaxError(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query": `{ invalid syntax here {{`,
	}
	body, _ := json.Marshal(query)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	// GraphQL returns 200 OK even for syntax errors (errors are in the response body)
	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have errors
	if _, hasErrors := response["errors"]; !hasErrors {
		t.Error("Response should contain 'errors' field for syntax error")
	}
}

// Table-driven tests for getTheme

func TestGetThemeTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		cookie    *http.Cookie
		wantTheme string
	}{
		{
			name:      "no cookie - defaults to dark",
			cookie:    nil,
			wantTheme: "dark",
		},
		{
			name:      "light theme cookie",
			cookie:    &http.Cookie{Name: "theme", Value: "light"},
			wantTheme: "light",
		},
		{
			name:      "dark theme cookie",
			cookie:    &http.Cookie{Name: "theme", Value: "dark"},
			wantTheme: "dark",
		},
		{
			name:      "auto theme cookie",
			cookie:    &http.Cookie{Name: "theme", Value: "auto"},
			wantTheme: "auto",
		},
		{
			name:      "invalid theme cookie - defaults to dark",
			cookie:    &http.Cookie{Name: "theme", Value: "blue"},
			wantTheme: "dark",
		},
		{
			name:      "empty theme cookie - defaults to dark",
			cookie:    &http.Cookie{Name: "theme", Value: ""},
			wantTheme: "dark",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}

			theme := getTheme(req)
			if theme != tt.wantTheme {
				t.Errorf("getTheme() = %q, want %q", theme, tt.wantTheme)
			}
		})
	}
}

// Table-driven tests for getGraphiQLThemeCSS

func TestGetGraphiQLThemeCSSTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		theme     string
		wantCSS   string
		wantDark  bool
		wantLight bool
	}{
		{
			name:     "dark theme",
			theme:    "dark",
			wantDark: true,
		},
		{
			name:      "light theme",
			theme:     "light",
			wantLight: true,
		},
		{
			name:     "auto theme - defaults to dark",
			theme:    "auto",
			wantDark: true,
		},
		{
			name:     "empty string - defaults to dark",
			theme:    "",
			wantDark: true,
		},
		{
			name:     "unknown value - defaults to dark",
			theme:    "unknown",
			wantDark: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			css := getGraphiQLThemeCSS(tt.theme)

			if tt.wantDark && css != graphiqlDarkTheme {
				t.Error("Expected dark theme CSS")
			}
			if tt.wantLight && css != graphiqlLightTheme {
				t.Error("Expected light theme CSS")
			}
		})
	}
}

// Test resolveHealth returns correct version from config

func TestResolveHealthVersion(t *testing.T) {
	params := gql.ResolveParams{}

	result, err := resolveHealth(params)
	if err != nil {
		t.Fatalf("resolveHealth() error = %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("resolveHealth() result type = %T, want map", result)
	}

	// Version should match config.Version
	if resultMap["version"] != config.Version {
		t.Errorf("version = %v, want %s", resultMap["version"], config.Version)
	}

	// All expected fields should be present
	expectedFields := []string{"status", "version", "uptime", "mode"}
	for _, field := range expectedFields {
		if _, ok := resultMap[field]; !ok {
			t.Errorf("Missing expected field: %s", field)
		}
	}
}

// Test schema has all expected query fields

func TestSchemaQueryFields(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	queryType := Schema.QueryType()
	if queryType == nil {
		t.Fatal("Schema.QueryType() should not be nil")
	}

	// Check expected query fields exist
	expectedFields := []string{"search", "autocomplete", "health"}
	for _, fieldName := range expectedFields {
		field := queryType.Fields()[fieldName]
		if field == nil {
			t.Errorf("Schema missing expected query field: %s", fieldName)
		}
	}
}

// Test Handler returns same handler for subsequent calls (schema already initialized)

func TestHandlerReusesSchema(t *testing.T) {
	// Initialize schema first
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}

	// Get handler twice
	handler1 := Handler(cfg)
	handler2 := Handler(cfg)

	// Both should work correctly (schema should be initialized once)
	for i, handler := range []http.HandlerFunc{handler1, handler2} {
		req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
		rec := httptest.NewRecorder()

		handler(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Handler %d: Status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}
}

// Test GraphQL query with nil variables

func TestGraphQLQueryWithNilVariables(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	query := map[string]interface{}{
		"query":     `{ health { status } }`,
		"variables": nil,
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

	if _, hasData := response["data"]; !hasData {
		t.Error("Response should contain 'data' field")
	}
}

// Test multiple GraphQL queries in sequence

func TestMultipleGraphQLQueries(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	queries := []string{
		`{ health { status } }`,
		`{ autocomplete(q: "test") }`,
		`{ search(q: "query") { query } }`,
	}

	for i, q := range queries {
		query := map[string]interface{}{
			"query": q,
		}
		body, _ := json.Marshal(query)

		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Query %d: Status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}

		var response map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("Query %d: Failed to decode response: %v", i+1, err)
		}

		if _, hasData := response["data"]; !hasData {
			t.Errorf("Query %d: Response should contain 'data' field", i+1)
		}
	}
}

// Test GraphiQL HTML structure

func TestGraphiQLHTMLStructure(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	body := rec.Body.String()

	// Check essential HTML elements
	requiredElements := []string{
		"<!DOCTYPE html>",
		"<html",
		"<head>",
		"<title>Search API - GraphiQL</title>",
		"graphiql.min.css",
		"<body>",
		`<div id="graphiql">`,
		"react.production.min.js",
		"react-dom.production.min.js",
		"graphiql.min.js",
		"GraphiQL.createFetcher",
		"</html>",
	}

	for _, element := range requiredElements {
		if !strings.Contains(body, element) {
			t.Errorf("GraphiQL HTML should contain %q", element)
		}
	}
}

// Test buildBaseURL constructs correct URL format

func TestBuildBaseURLFormat(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
	req.Host = "example.com:8080"

	url := buildBaseURL(req)

	// Should be a valid URL format
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		t.Errorf("buildBaseURL() = %q, should start with http:// or https://", url)
	}

	// Should contain the host
	if !strings.Contains(url, "example.com:8080") {
		t.Errorf("buildBaseURL() = %q, should contain host example.com:8080", url)
	}

	// Should NOT contain the path
	if strings.Contains(url, "/some/path") {
		t.Errorf("buildBaseURL() = %q, should not contain request path", url)
	}
}
