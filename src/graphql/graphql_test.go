package graphql

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/search/engine"
	"github.com/go-chi/chi/v5"
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

	params := testResolverParams(map[string]interface{}{"q": "test query"})

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

func TestResolveSearchCustomDateRange(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	params := testResolverParams(map[string]interface{}{
		"q": "privacy", "timeRange": "custom", "timeFrom": "2025-01-01", "timeTo": "2025-01-31",
	})
	if _, err := resolveSearch(params); err != nil {
		t.Fatalf("resolveSearch() rejected valid custom date range: %v", err)
	}
	for _, args := range []map[string]interface{}{
		{"q": "privacy", "timeRange": "custom", "timeFrom": "2025-01-01"},
		{"q": "privacy", "timeFrom": "2025-02-30", "timeTo": "2025-03-01"},
		{"q": "privacy", "timeFrom": "2025-02-01", "timeTo": "2025-01-01"},
	} {
		if _, err := resolveSearch(testResolverParams(args)); err == nil {
			t.Errorf("resolveSearch(%#v) accepted an invalid date range", args)
		}
	}
}

func TestResolveAutocomplete(t *testing.T) {
	params := testResolverParams(map[string]interface{}{"q": "test"})

	result, err := resolveAutocomplete(params)
	if err != nil {
		t.Fatalf("resolveAutocomplete() error = %v", err)
	}

	suggestions, ok := result.([]string)
	if !ok {
		t.Fatalf("resolveAutocomplete() result type = %T, want []string", result)
	}

	// The resolver returns the configured provider suggestions.
	if len(suggestions) != 1 || suggestions[0] != "test fixture" {
		t.Errorf("suggestions = %v, want the configured fixture", suggestions)
	}
}

func TestResolveHealth(t *testing.T) {
	params := testResolverParams(nil)

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

	req := httptest.NewRequest(http.MethodPut, "/graphql?lang=de", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Errorf("ok = %v, want false", resp["ok"])
	}
	if resp["error"] != "METHOD_NOT_ALLOWED" {
		t.Errorf("error = %q, want %q", resp["error"], "METHOD_NOT_ALLOWED")
	}
	if resp["message"] != "Methode nicht erlaubt" {
		t.Errorf("message = %q, want %q", resp["message"], "Methode nicht erlaubt")
	}
}

func TestHandlerInvalidJSON(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg, testGraphQLDependencies())

	req := httptest.NewRequest(http.MethodPost, "/graphql?lang=de", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal body: %v", err)
	}
	if ok, _ := resp["ok"].(bool); ok {
		t.Errorf("ok = %v, want false", resp["ok"])
	}
	if resp["error"] != "BAD_REQUEST" {
		t.Errorf("error = %q, want %q", resp["error"], "BAD_REQUEST")
	}
	if resp["message"] != "Ungültige Anfrage" {
		t.Errorf("message = %q, want %q", resp["message"], "Ungültige Anfrage")
	}
}

func TestGraphQLQueryComplexityLimits(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	handler := Handler(&config.Config{}, testGraphQLDependencies())
	t.Run("excessive field count", func(t *testing.T) {
		var query strings.Builder
		query.WriteString("query { ")
		for index := 0; index <= maxGraphQLQueryFields; index++ {
			fmt.Fprintf(&query, "field%d: health { status } ", index)
		}
		query.WriteString("}")
		recorder := httptest.NewRecorder()
		body := fmt.Sprintf(`{"query":%s}`, strconv.Quote(query.String()))
		handler(recorder, httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	})
	t.Run("excessive nesting", func(t *testing.T) {
		query := "query { " + strings.Repeat("unknown { ", maxGraphQLQueryDepth) + "health" + strings.Repeat(" }", maxGraphQLQueryDepth) + " }"
		recorder := httptest.NewRecorder()
		handler(recorder, httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":`+strconv.Quote(query)+`}`)))
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
		}
	})
	t.Run("oversized request", func(t *testing.T) {
		body := `{"query":"` + strings.Repeat("x", maxGraphQLRequestBytes) + `"}`
		recorder := httptest.NewRecorder()
		handler(recorder, httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(body)))
		if recorder.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
		}
	})
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
			// Loopback is always a trusted proxy — X-Forwarded-* headers are honored.
			req.RemoteAddr = "127.0.0.1:1234"
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
	handler := Handler(cfg, testGraphQLDependencies())

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

func TestGraphQLSearchRewritesImageThumbnails(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	dependencies := testGraphQLDependencies()
	dependencies.ThumbnailRewriter = func(string) string { return "/image-proxy?url=signed" }
	handler := Handler(&config.Config{}, dependencies)
	body, err := json.Marshal(map[string]interface{}{"query": `{ search(q: "test", category: "images") { results { thumbnail imageUrl imageWidth imageHeight imageFormat fileSize } } }`})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GraphQL status = %d: %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Search struct {
				Results []struct {
					Thumbnail   string `json:"thumbnail"`
					ImageURL    string `json:"imageUrl"`
					ImageWidth  int    `json:"imageWidth"`
					ImageHeight int    `json:"imageHeight"`
					ImageFormat string `json:"imageFormat"`
					FileSize    int    `json:"fileSize"`
				} `json:"results"`
			} `json:"search"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode GraphQL response: %v", err)
	}
	if len(payload.Data.Search.Results) != 1 || payload.Data.Search.Results[0].Thumbnail != "/image-proxy?url=signed" || payload.Data.Search.Results[0].ImageURL != "/image-proxy?url=signed" || payload.Data.Search.Results[0].ImageWidth != 800 || payload.Data.Search.Results[0].ImageHeight != 600 || payload.Data.Search.Results[0].ImageFormat != "jpg" || payload.Data.Search.Results[0].FileSize != 4096 {
		t.Fatalf("GraphQL thumbnail = %#v, want rewritten same-origin URL", payload.Data.Search.Results)
	}
}

// Tests for GraphQL autocomplete query via handler

func TestGraphQLAutocompleteQuery(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg, testGraphQLDependencies())

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

	// Autocomplete returns provider-backed suggestions
	autocomplete, ok := data["autocomplete"].([]interface{})
	if !ok {
		t.Fatal("Data should contain 'autocomplete' array")
	}

	if len(autocomplete) != 1 || autocomplete[0] != "test fixture" {
		t.Errorf("autocomplete = %v, want the provider fixture", autocomplete)
	}
}

// Tests for GraphQL query with variables and operationName

func TestGraphQLQueryWithVariablesAndOperationName(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg, testGraphQLDependencies())

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
	// Loopback is always a trusted proxy — X-Forwarded-Proto is honored.
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "localhost:8443"
	req.TLS = &tls.ConnectionState{}
	// Override to http
	req.Header.Set("X-Forwarded-Proto", "http")

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	cfg.Server.Mode = "production"
	dependencies := testGraphQLDependencies()
	dependencies.API = api.NewHandler(cfg, nil, nil)
	handler := Handler(cfg, dependencies)

	query := map[string]interface{}{
		"query": `{
			health {
				project { name tagline description }
				status pendingRestart restartReason
				version goVersion
				build { commit date }
				uptime mode timestamp
				features { geoip tor { enabled running status hostname } i2p { enabled running status hostname provider } }
				checks { database cache disk scheduler tor i2p }
				stats { requestsTotal requests24h activeConns }
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
	if health["uptime"] == "" {
		t.Error("health.uptime is empty")
	}
	if health["version"] != config.Version {
		t.Errorf("health.version = %v, want %s", health["version"], config.Version)
	}
	for _, field := range []string{"project", "build", "features", "checks", "stats"} {
		if _, ok := health[field].(map[string]interface{}); !ok {
			t.Errorf("health.%s = %T, want nested object; response=%v", field, health[field], response)
		}
	}
}

func TestGraphQLPreferenceImportAppliesRESTCookies(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	dependencies := testGraphQLDependencies()
	dependencies.API = api.NewHandler(cfg, nil, nil)
	handler := Handler(cfg, dependencies)
	query := map[string]interface{}{"query": `mutation { importPreferences(theme: "light", lang: "fr") }`}
	body, _ := json.Marshal(query)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "theme=light") {
		t.Errorf("Set-Cookie = %v, want theme=light", rec.Header().Values("Set-Cookie"))
	}
	if !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), "\n"), "lang=fr") {
		t.Errorf("Set-Cookie = %v, want lang=fr", rec.Header().Values("Set-Cookie"))
	}
	if strings.Contains(rec.Body.String(), `"errors"`) {
		t.Errorf("GraphQL response has errors: %s", rec.Body.String())
	}
}

func TestGraphQLInformationalRoutesUseRESTHandlers(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.Server.Title = "Search"
	dependencies := testGraphQLDependencies()
	dependencies.API = api.NewHandler(cfg, nil, nil)
	handler := Handler(cfg, dependencies)
	query := map[string]interface{}{"query": `{ serverPage(name: "about") contactInfo favicon }`}
	body, _ := json.Marshal(query)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var response struct {
		Data   map[string]interface{} `json:"data"`
		Errors []interface{}          `json:"errors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Errors) != 0 {
		t.Fatalf("GraphQL errors: %v", response.Errors)
	}
	page, ok := response.Data["serverPage"].(map[string]interface{})
	if !ok || page["title"] != "About Search" {
		t.Errorf("serverPage = %v, want REST about-page data", response.Data["serverPage"])
	}
	contact, ok := response.Data["contactInfo"].(map[string]interface{})
	if !ok || contact["title"] != "Contact" {
		t.Errorf("contactInfo = %v, want REST contact metadata", response.Data["contactInfo"])
	}
	icon, ok := response.Data["favicon"].(map[string]interface{})
	if !ok || icon["contentType"] != "image/png" || icon["base64"] == "" {
		t.Errorf("favicon = %v, want base64 PNG fallback", response.Data["favicon"])
	}
}

// Test search query with all optional arguments

func TestGraphQLSearchQueryWithAllArgs(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	params := testResolverParams(map[string]interface{}{"q": "edge case test"})

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
	if resultMap["totalResults"] != 1 {
		t.Errorf("totalResults = %v, want one fixture result", resultMap["totalResults"])
	}
	if resultMap["searchTime"].(float64) <= 0 {
		t.Errorf("searchTime = %v, want a measured duration", resultMap["searchTime"])
	}

	results, ok := resultMap["results"].([]map[string]interface{})
	if !ok {
		t.Fatalf("results type = %T, want []map[string]interface{}", resultMap["results"])
	}
	if len(results) != 1 || results[0]["title"] != "Fixture result" {
		t.Errorf("results = %#v, want the fixture result", results)
	}
	if results[0]["duration"] != 634 || results[0]["viewCount"] != float64(1000) || results[0]["author"] != "Fixture Channel" || results[0]["embedUrl"] != "https://www.youtube-nocookie.com/embed/fixture" {
		t.Errorf("video result fields = %#v", results[0])
	}

	engines, ok := resultMap["engines"].([]string)
	if !ok {
		t.Fatalf("engines type = %T, want []string", resultMap["engines"])
	}
	if len(engines) != 1 || engines[0] != "fixture" {
		t.Errorf("engines = %v, want [fixture]", engines)
	}
}

// Test empty query body

func TestHandlerEmptyQuery(t *testing.T) {
	// Initialize schema
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler1 := Handler(cfg, testGraphQLDependencies())
	handler2 := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

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
	handler := Handler(cfg, testGraphQLDependencies())

	req := httptest.NewRequest(http.MethodGet, "/graphql", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	body := rec.Body.String()

	// Check essential HTML elements. Per frontend rules, GraphiQL is a
	// self-contained server-rendered vanilla-JS explorer — no CDN frameworks.
	requiredElements := []string{
		"<!DOCTYPE html>",
		"<html",
		"<head>",
		"<title>Search API Explorer</title>",
		"<body>",
		`<textarea id="query"`,
		`<pre id="response">`,
		`<summary>GraphQL schema</summary>`,
		"function runQuery()",
		"/api/graphql",
		"</html>",
	}

	for _, element := range requiredElements {
		if !strings.Contains(body, element) {
			t.Errorf("GraphiQL HTML should contain %q", element)
		}
	}
	if !strings.Contains(body, "type Query {") {
		t.Error("GraphiQL HTML should include the generated GraphQL schema")
	}
}

func TestServeGraphiQLLanguageAttributes(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/graphql?lang=de", nil)
	rec := httptest.NewRecorder()

	serveGraphiQL(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `<html lang="de" dir="ltr">`) {
		t.Fatalf("GraphiQL should use request locale, got body: %s", body)
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

func TestGraphQLPublicCatalogsUseRESTSources(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.DefaultConfig(), testGraphQLDependencies())
	requestBody := `{"query":"{ categories { id } engines { id enabled } bangs(search: \"google\") { shortcut name } }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("GraphQL status = %d, want 200", response.Code)
	}
	var payload struct {
		Data struct {
			Categories []struct {
				ID string `json:"id"`
			} `json:"categories"`
			Engines []struct {
				ID      string `json:"id"`
				Enabled bool   `json:"enabled"`
			} `json:"engines"`
			Bangs []struct {
				Shortcut string `json:"shortcut"`
			} `json:"bangs"`
		} `json:"data"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("GraphQL returned errors: %v", payload.Errors)
	}
	if len(payload.Data.Categories) != len(api.AvailableCategories()) {
		t.Errorf("categories = %d, want REST catalog size %d", len(payload.Data.Categories), len(api.AvailableCategories()))
	}
	if len(payload.Data.Engines) != 1 || payload.Data.Engines[0].ID != "fixture" || !payload.Data.Engines[0].Enabled {
		t.Errorf("engines = %#v, want enabled fixture engine", payload.Data.Engines)
	}
	if len(payload.Data.Bangs) == 0 {
		t.Error("bangs(search: google) returned no matches")
	}
}

func TestGraphQLRelatedSearchesUsesSharedProvider(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.DefaultConfig(), testGraphQLDependencies())
	requestBody := `{"query":"{ relatedSearches(q: \"rust\", limit: 1) }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, req)
	var payload struct {
		Data struct {
			Related []string `json:"relatedSearches"`
		} `json:"data"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("GraphQL errors: %v", payload.Errors)
	}
	if len(payload.Data.Related) != 1 || payload.Data.Related[0] != "rust guide" {
		t.Fatalf("relatedSearches = %v, want [rust guide]", payload.Data.Related)
	}
}

func TestGraphQLWidgetsExposeRESTCatalogAndData(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.DefaultConfig(), testGraphQLDependencies())
	requestBody := `{"query":"{ widgets(category: \"data\") { enabled widgets { type category } defaults } widgetData(type: \"weather\") { type error } }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, req)
	var payload struct {
		Data struct {
			Widgets struct {
				Enabled bool `json:"enabled"`
				Widgets []struct {
					Type     string `json:"type"`
					Category string `json:"category"`
				} `json:"widgets"`
				Defaults []string `json:"defaults"`
			} `json:"widgets"`
			WidgetData struct {
				Type  string `json:"type"`
				Error string `json:"error"`
			} `json:"widgetData"`
		} `json:"data"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("GraphQL errors: %v", payload.Errors)
	}
	if !payload.Data.Widgets.Enabled || len(payload.Data.Widgets.Widgets) == 0 || len(payload.Data.Widgets.Defaults) == 0 {
		t.Fatalf("widget catalog = %#v, expected enabled catalog with widgets and defaults", payload.Data.Widgets)
	}
	if payload.Data.WidgetData.Type != "weather" || payload.Data.WidgetData.Error == "" {
		t.Fatalf("widget data = %#v, want unavailable fixture fetcher error", payload.Data.WidgetData)
	}
}

func TestGraphQLServerInfoUsesRESTDataSource(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.DefaultConfig(), testGraphQLDependencies())
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ serverInfo }"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, req)
	var payload struct {
		Data struct {
			ServerInfo map[string]interface{} `json:"serverInfo"`
		} `json:"data"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("GraphQL errors: %v", payload.Errors)
	}
	if payload.Data.ServerInfo["name"] != "Fixture Search" || payload.Data.ServerInfo["version"] != "test" {
		t.Fatalf("serverInfo = %#v, want REST fixture fields", payload.Data.ServerInfo)
	}
}

func TestGraphQLGeoIPLookupUsesRESTDataSource(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.DefaultConfig(), testGraphQLDependencies())
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ geoIPLookup(ip: \"8.8.8.8\") }"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, req)
	var payload struct {
		Data struct {
			GeoIP map[string]interface{} `json:"geoIPLookup"`
		} `json:"data"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("GraphQL errors: %v", payload.Errors)
	}
	if payload.Data.GeoIP["ip"] != "8.8.8.8" || payload.Data.GeoIP["attribution"] != "fixture attribution" {
		t.Fatalf("geoIPLookup = %#v, want explicit IP with source attribution", payload.Data.GeoIP)
	}
}

func TestGraphQLGeoIPLookupDefaultsToCallerIP(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	dependencies := testGraphQLDependencies()
	var lookedUp string
	dependencies.GeoIPLookup = func(ip string) (api.GeoIPLookupResponse, error) {
		lookedUp = ip
		return api.GeoIPLookupResponse{IP: ip, Found: true, Attribution: "fixture attribution"}, nil
	}
	handler := Handler(config.DefaultConfig(), dependencies)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ geoIPLookup }"}`))
	req.RemoteAddr = "203.0.113.9:4242"
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler(response, req)
	if lookedUp != "203.0.113.9" {
		t.Fatalf("lookup IP = %q, want caller IP", lookedUp)
	}
}

func TestGraphQLOperatorFieldsKeepBearerAuthorization(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Token: "fixture-operator-token"}}
	apiHandler := api.NewHandler(cfg, engine.NewRegistry(), nil)
	dependencies := testGraphQLDependencies()
	dependencies.OperatorStatus = apiHandler.OperatorServerStatus
	dependencies.OperatorConfig = apiHandler.OperatorServerConfig
	handler := Handler(cfg, dependencies)
	query := `{"query":"{ serverStatus serverConfig }"}`

	unauthorized := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(query))
	unauthorized.Header.Set("Content-Type", "application/json")
	unauthorizedResponse := httptest.NewRecorder()
	handler(unauthorizedResponse, unauthorized)
	var rejected struct {
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(unauthorizedResponse.Body).Decode(&rejected); err != nil {
		t.Fatal(err)
	}
	if len(rejected.Errors) == 0 {
		t.Fatal("operator-only fields resolved without bearer authentication")
	}

	authorized := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(query))
	authorized.Header.Set("Content-Type", "application/json")
	authorized.Header.Set("Authorization", "Bearer fixture-operator-token")
	authorizedResponse := httptest.NewRecorder()
	handler(authorizedResponse, authorized)
	var accepted struct {
		Data struct {
			Status map[string]interface{} `json:"serverStatus"`
			Config map[string]interface{} `json:"serverConfig"`
		} `json:"data"`
		Errors []interface{} `json:"errors"`
	}
	if err := json.NewDecoder(authorizedResponse.Body).Decode(&accepted); err != nil {
		t.Fatal(err)
	}
	if len(accepted.Errors) != 0 {
		t.Fatalf("authorized GraphQL errors: %v", accepted.Errors)
	}
	if accepted.Data.Status["status"] == nil || accepted.Data.Config["mode"] == nil {
		t.Fatalf("operator data missing: %#v", accepted.Data)
	}
	if _, leaked := accepted.Data.Config["token"]; leaked {
		t.Fatal("redacted config leaked operator token")
	}
}

func TestGraphQLOwnerTokenFieldsRequirePrimaryAuthorization(t *testing.T) {
	cfg := config.DefaultConfig()
	dependencies := testGraphQLDependencies()
	dependencies.API = api.NewHandler(cfg, engine.NewRegistry(), nil)
	handler := Handler(cfg, dependencies)
	for _, body := range []string{`{"query":"{ ownerTokens }"}`, `{"query":"mutation { revealPrimaryToken }"}`} {
		request := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler(response, request)
		var result struct {
			Errors []interface{} `json:"errors"`
		}
		if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		if len(result.Errors) != 1 {
			t.Fatalf("owner token field errors = %v, want operation rejected without primary bearer auth", result.Errors)
		}
	}
}

func TestGraphQLAlertInventoryUsesOperatorAuthorization(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Server: config.ServerConfig{Token: "fixture-operator-token"}}
	apiHandler := api.NewHandler(cfg, engine.NewRegistry(), nil)
	dependencies := testGraphQLDependencies()
	dependencies.API = apiHandler
	handler := Handler(cfg, dependencies)
	query := `{"query":"{ alerts }"}`
	for _, test := range []struct {
		name, authorization string
		wantUnauthorized    bool
	}{
		{name: "missing token", wantUnauthorized: true},
		{name: "invalid token", authorization: "Bearer wrong-token", wantUnauthorized: true},
		{name: "valid token", authorization: "Bearer fixture-operator-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(query))
			req.Header.Set("Content-Type", "application/json")
			if test.authorization != "" {
				req.Header.Set("Authorization", test.authorization)
			}
			rec := httptest.NewRecorder()
			handler(rec, req)
			var payload struct {
				Errors []map[string]interface{} `json:"errors"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if test.wantUnauthorized && len(payload.Errors) == 0 {
				t.Fatal("unauthorized alert inventory request succeeded")
			}
			if !test.wantUnauthorized {
				if len(payload.Errors) == 0 {
					t.Fatal("missing alert storage should fail closed")
				}
				message, _ := payload.Errors[0]["message"].(string)
				if message != "alert operation failed" {
					t.Fatalf("error message = %q, want sanitized failure", message)
				}
			}
			if test.wantUnauthorized && len(payload.Errors) > 0 {
				message, _ := payload.Errors[0]["message"].(string)
				if message != "operator authorization required" {
					t.Fatalf("authorization error = %q", message)
				}
			}
		})
	}
}

func TestGraphQLOperatorGeoIPAndSchedulerFieldsRequireBearerToken(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Token: "fixture-operator-token"}}
	dependencies := testGraphQLDependencies()
	dependencies.API = api.NewHandler(cfg, engine.NewRegistry(), nil)
	handler := Handler(cfg, dependencies)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"{ geoIPPresets schedulerTasks }"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	var payload struct {
		Errors []map[string]interface{} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 2 {
		t.Fatalf("unauthorized GraphQL errors = %v, want both operator-only fields rejected", payload.Errors)
	}
	for _, item := range payload.Errors {
		if item["message"] != "operator authorization required" {
			t.Fatalf("unsafe authorization error: %v", item["message"])
		}
	}
}

func TestGraphQLRESTBridgeUsesRegisteredAPIRouter(t *testing.T) {
	cfg := config.DefaultConfig()
	apiHandler := api.NewHandler(cfg, engine.NewRegistry(), nil)
	router := chi.NewRouter()
	apiHandler.RegisterRoutes(router)
	dependencies := testGraphQLDependencies()
	dependencies.API = apiHandler
	handler := Handler(cfg, dependencies)
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(`{"query":"mutation { apiRequest(method: \"GET\", path: \"/api/v1/info\") }"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler(rec, req)
	var payload struct {
		Data   map[string]interface{}   `json:"data"`
		Errors []map[string]interface{} `json:"errors"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Errors) != 0 {
		t.Fatalf("GraphQL REST bridge errors: %v", payload.Errors)
	}
	result, ok := payload.Data["apiRequest"].(map[string]interface{})
	if !ok || result["status"] != float64(http.StatusOK) {
		t.Fatalf("REST bridge response = %#v", payload.Data["apiRequest"])
	}
}

func TestGraphQLPreferencesQueryAndMutation(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatal(err)
	}
	handler := Handler(config.DefaultConfig(), testGraphQLDependencies())
	for _, query := range []string{`{"query":"{ preferences }"}`, `{"query":"mutation { savePreferences }"}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/graphql", strings.NewReader(query))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler(response, req)
		var payload struct {
			Data   map[string]interface{} `json:"data"`
			Errors []interface{}          `json:"errors"`
		}
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Errors) != 0 {
			t.Fatalf("query %s errors: %v", query, payload.Errors)
		}
		if len(payload.Data) != 1 {
			t.Fatalf("query %s data = %#v", query, payload.Data)
		}
	}
}
