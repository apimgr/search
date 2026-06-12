package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/version"
)

// Tests for NewClient

func TestNewClient(t *testing.T) {
	client := NewClient("https://api.example.com", "test-token", 30)

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
	if client.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q, want %q", client.BaseURL, "https://api.example.com")
	}
	if client.Token != "test-token" {
		t.Errorf("Token = %q, want %q", client.Token, "test-token")
	}
	if client.HTTPClient == nil {
		t.Error("HTTPClient should be initialized")
	}
	if client.HTTPClient.Timeout != 30*time.Second {
		t.Errorf("HTTPClient.Timeout = %v, want %v", client.HTTPClient.Timeout, 30*time.Second)
	}
}

func TestNewClientZeroTimeout(t *testing.T) {
	client := NewClient("https://api.example.com", "", 0)

	if client.HTTPClient.Timeout != 0 {
		t.Errorf("HTTPClient.Timeout = %v, want 0", client.HTTPClient.Timeout)
	}
}

// Tests for Client struct fields

func TestClientStruct(t *testing.T) {
	client := &Client{
		BaseURL:     "https://api.example.com",
		Token:       "test-token",
		UserContext: "testuser",
		HTTPClient:  http.DefaultClient,
	}

	if client.BaseURL != "https://api.example.com" {
		t.Errorf("BaseURL = %q", client.BaseURL)
	}
	if client.Token != "test-token" {
		t.Errorf("Token = %q", client.Token)
	}
	if client.UserContext != "testuser" {
		t.Errorf("UserContext = %q", client.UserContext)
	}
}

// Tests for SearchResult struct

func TestSearchResultStruct(t *testing.T) {
	result := SearchResult{
		Title:       "Test Title",
		URL:         "https://example.com",
		Description: "Test description",
		Engine:      "google",
		Score:       0.95,
		Category:    "general",
	}

	if result.Title != "Test Title" {
		t.Errorf("Title = %q", result.Title)
	}
	if result.URL != "https://example.com" {
		t.Errorf("URL = %q", result.URL)
	}
	if result.Description != "Test description" {
		t.Errorf("Description = %q", result.Description)
	}
	if result.Score != 0.95 {
		t.Errorf("Score = %f", result.Score)
	}
}

func TestSearchResultJSON(t *testing.T) {
	result := SearchResult{
		Title:       "Test",
		URL:         "https://example.com",
		Description: "Description",
		Engine:      "google",
		Score:       0.5,
		Category:    "general",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded SearchResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Title != result.Title {
		t.Errorf("decoded.Title = %q, want %q", decoded.Title, result.Title)
	}
}

// Tests for SearchResponse struct per AI.md PART 14 pagination format

func TestSearchResponseStruct(t *testing.T) {
	response := SearchResponse{
		Query:    "test query",
		Category: "general",
		Results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1"},
			{Title: "Result 2", URL: "https://example.com/2"},
		},
		Pagination: SearchPagination{
			Page:  1,
			Limit: 10,
			Total: 100,
			Pages: 10,
		},
		SearchTime: 100.5,
		Engines:    []string{"google"},
	}

	if len(response.Results) != 2 {
		t.Errorf("Results length = %d", len(response.Results))
	}
	if response.Pagination.Total != 100 {
		t.Errorf("Pagination.Total = %d", response.Pagination.Total)
	}
	if response.Query != "test query" {
		t.Errorf("Query = %q", response.Query)
	}
	if response.Pagination.Page != 1 {
		t.Errorf("Pagination.Page = %d", response.Pagination.Page)
	}
	if response.Pagination.Limit != 10 {
		t.Errorf("Pagination.Limit = %d", response.Pagination.Limit)
	}
}

// Tests for HealthResponse struct

func TestHealthResponseStruct(t *testing.T) {
	response := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		Uptime:    "24h30m",
		Timestamp: "2024-01-01T00:00:00Z",
		Checks: map[string]string{
			"database": "ok",
			"cache":    "ok",
		},
	}

	if response.Status != "healthy" {
		t.Errorf("Status = %q", response.Status)
	}
	if response.Version != "1.0.0" {
		t.Errorf("Version = %q", response.Version)
	}
	if response.Uptime != "24h30m" {
		t.Errorf("Uptime = %q", response.Uptime)
	}
	if len(response.Checks) != 2 {
		t.Errorf("Checks length = %d", len(response.Checks))
	}
}

func TestHealthResponseJSON(t *testing.T) {
	response := HealthResponse{
		Status:  "healthy",
		Version: "1.0.0",
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded HealthResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Status != response.Status {
		t.Errorf("decoded.Status = %q, want %q", decoded.Status, response.Status)
	}
}

// Tests for AutodiscoverResponse struct

func TestAutodiscoverResponseStruct(t *testing.T) {
	response := AutodiscoverResponse{}
	response.Server.Name = "search"
	response.Server.Version = "1.0.0"
	response.Server.URL = "https://api.example.com"
	response.Server.Features.Auth = true
	response.Server.Features.Search = true
	response.Server.Features.Register = false
	response.API.Version = "v1"
	response.API.BasePath = version.APIPrefix

	if response.Server.Name != "search" {
		t.Errorf("Server.Name = %q", response.Server.Name)
	}
	if response.Server.Version != "1.0.0" {
		t.Errorf("Server.Version = %q", response.Server.Version)
	}
	if !response.Server.Features.Auth {
		t.Error("Server.Features.Auth should be true")
	}
	if !response.Server.Features.Search {
		t.Error("Server.Features.Search should be true")
	}
	if response.Server.Features.Register {
		t.Error("Server.Features.Register should be false")
	}
	if response.API.Version != "v1" {
		t.Errorf("API.Version = %q", response.API.Version)
	}
}

// Tests for SetUserContext

func TestSetUserContext(t *testing.T) {
	client := NewClient("https://api.example.com", "", 30)

	client.SetUserContext("testuser")
	if client.UserContext != "testuser" {
		t.Errorf("UserContext = %q, want 'testuser'", client.UserContext)
	}

	client.SetUserContext("@forced")
	if client.UserContext != "@forced" {
		t.Errorf("UserContext = %q, want '@forced'", client.UserContext)
	}

	client.SetUserContext("+orgname")
	if client.UserContext != "+orgname" {
		t.Errorf("UserContext = %q, want '+orgname'", client.UserContext)
	}
}

// Tests for Search

func TestSearch(t *testing.T) {
	// Create test server that returns wrapped API response per AI.md
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/search" {
			t.Errorf("Expected path %s, got %s", version.APIPrefix+"/search", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "test query" {
			t.Errorf("Expected q=test+query, got %s", r.URL.Query().Get("q"))
		}

		// Return wrapped response per AI.md API format
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true,"data":{"query":"test query","category":"general","results":[{"title":"Result 1","url":"https://example.com/1","description":"","engine":"google","score":0.9}],"pagination":{"page":1,"limit":10,"total":1,"pages":1},"search_time_ms":100,"engines_used":["google"]}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 30)
	result, err := client.Search("test query", 1, 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if result.Query != "test query" {
		t.Errorf("result.Query = %q", result.Query)
	}
	if len(result.Results) != 1 {
		t.Errorf("result.Results length = %d", len(result.Results))
	}
}

func TestSearchServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Search("test", 1, 10)
	if err == nil {
		t.Error("Search() should return error for 500 response")
	}
}

// Tests for Health

func TestHealth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/healthz" {
			t.Errorf("Expected path %s, got %s", version.APIPrefix+"/healthz", r.URL.Path)
		}

		// Per AI.md PART 14: Wrapped response format {"ok": true, "data": {...}}
		response := map[string]interface{}{
			"ok": true,
			"data": HealthResponse{
				Status:  "healthy",
				Version: "1.0.0",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	result, err := client.Health()
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if result.Status != "healthy" {
		t.Errorf("result.Status = %q", result.Status)
	}
	if result.Version != "1.0.0" {
		t.Errorf("result.Version = %q", result.Version)
	}
}

// Tests for GetVersion

func TestGetVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Per AI.md PART 14: Wrapped response format {"ok": true, "data": {...}}
		response := map[string]interface{}{
			"ok": true,
			"data": HealthResponse{
				Status:  "healthy",
				Version: "2.0.0",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	version, err := client.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}

	if version != "2.0.0" {
		t.Errorf("GetVersion() = %q, want '2.0.0'", version)
	}
}

// Tests for Autodiscover

func TestAutodiscover(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/autodiscover" {
			t.Errorf("Expected path /api/autodiscover, got %s", r.URL.Path)
		}

		response := AutodiscoverResponse{}
		response.Server.Name = "search"
		response.Server.Version = "1.0.0"
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	result, err := client.Autodiscover()
	if err != nil {
		t.Fatalf("Autodiscover() error = %v", err)
	}

	if result.Server.Name != "search" {
		t.Errorf("result.Server.Name = %q", result.Server.Name)
	}
}

func TestGet(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		response := HealthResponse{Status: "healthy"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	resp, err := client.get(version.APIPrefix + "/healthz")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	resp.Body.Close()

	if requestCount != 1 {
		t.Errorf("Request count = %d, want 1", requestCount)
	}
}

// Tests for doRequest

func TestDoRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept header = %q", r.Header.Get("Accept"))
		}
		userAgent := r.Header.Get("User-Agent")
		if userAgent == "" {
			t.Error("User-Agent header should be set")
		}

		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	resp, err := client.doRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

func TestDoRequestWithToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want 'Bearer test-token'", auth)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 30)
	resp, err := client.doRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

func TestDoRequestWithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type header = %q, want 'application/json'", contentType)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	body := map[string]string{"key": "value"}
	resp, err := client.doRequest("POST", "/test", body)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

func TestDoRequest400Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.doRequest("GET", "/test", nil)
	if err == nil {
		t.Error("doRequest() should return error for 400 response")
	}
}

// Tests for package variables

func TestProjectName(t *testing.T) {
	if ProjectName == "" {
		t.Error("ProjectName should not be empty")
	}
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
}


// Additional tests for 100% coverage

// Test Search with invalid JSON response
func TestSearchInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 30)
	_, err := client.Search("test query", 1, 10)
	if err == nil {
		t.Error("Search() should return error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "failed to decode") {
		t.Errorf("error = %q, want to contain 'failed to decode'", err.Error())
	}
}

// Test Health with invalid JSON response
func TestHealthInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Health()
	if err == nil {
		t.Error("Health() should return error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "failed to decode") {
		t.Errorf("error = %q, want to contain 'failed to decode'", err.Error())
	}
}

// Test Health with server error
func TestHealthServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Health()
	if err == nil {
		t.Error("Health() should return error for 500 response")
	}
}

// Test GetVersion with error from Health
func TestGetVersionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetVersion()
	if err == nil {
		t.Error("GetVersion() should return error when Health() fails")
	}
}

// Test Autodiscover with invalid JSON response
func TestAutodiscoverInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Autodiscover()
	if err == nil {
		t.Error("Autodiscover() should return error for invalid JSON response")
	}
	if !strings.Contains(err.Error(), "failed to decode autodiscover") {
		t.Errorf("error = %q, want to contain 'failed to decode autodiscover'", err.Error())
	}
}

// Test Autodiscover with server error
func TestAutodiscoverServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Autodiscover()
	if err == nil {
		t.Error("Autodiscover() should return error for 500 response")
	}
}

// Test doRequest with network connection refused
func TestDoRequestConnectionRefused(t *testing.T) {
	// Invalid port, connection refused
	client := NewClient("http://127.0.0.1:59999", "", 1)
	_, err := client.doRequest("GET", "/test", nil)
	if err == nil {
		t.Error("doRequest() should return error for connection refused")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %q, want to contain 'request failed'", err.Error())
	}
}

// Test doRequest with body containing unmarshalable type
func TestDoRequestMarshalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	// Create an unmarshalable body (channel cannot be marshaled to JSON)
	unmarshalableBody := make(chan int)
	_, err := client.doRequest("POST", "/test", unmarshalableBody)
	if err == nil {
		t.Error("doRequest() should return error for unmarshalable body")
	}
	if !strings.Contains(err.Error(), "failed to marshal body") {
		t.Errorf("error = %q, want to contain 'failed to marshal body'", err.Error())
	}
}

// Test Search with connection error
func TestSearchConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "", 1)
	_, err := client.Search("test", 1, 10)
	if err == nil {
		t.Error("Search() should return error for connection failure")
	}
}

// Test Health with connection error
func TestHealthConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "", 1)
	_, err := client.Health()
	if err == nil {
		t.Error("Health() should return error for connection failure")
	}
}

// Test GetVersion with empty version in response
func TestGetVersionEmptyVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Per AI.md PART 14: Wrapped response format {"ok": true, "data": {...}}
		response := map[string]interface{}{
			"ok": true,
			"data": HealthResponse{
				Status:  "healthy",
				Version: "",
			},
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	version, err := client.GetVersion()
	if err != nil {
		t.Fatalf("GetVersion() error = %v", err)
	}
	if version != "" {
		t.Errorf("GetVersion() = %q, want empty string", version)
	}
}

// Test SetUserContext with empty string
func TestSetUserContextEmpty(t *testing.T) {
	client := NewClient("https://api.example.com", "", 30)

	client.SetUserContext("")
	if client.UserContext != "" {
		t.Errorf("UserContext = %q, want empty string", client.UserContext)
	}
}

// Test doRequest without token
func TestDoRequestNoToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Authorization header = %q, want empty", auth)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	// No token
	client := NewClient(server.URL, "", 30)
	resp, err := client.doRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequest() error = %v", err)
	}
	resp.Body.Close()
}

// Test search with different pagination values
// Per AI.md PART 14: Pagination format {page, limit, total, pages}
func TestSearchPagination(t *testing.T) {
	tests := []struct {
		name  string
		page  int
		limit int
	}{
		{"first page", 1, 10},
		{"second page", 2, 10},
		{"large limit", 1, 100},
		{"zero values", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Per AI.md PART 14: Wrapped response format {"ok": true, "data": {...}}
				response := map[string]interface{}{
					"ok": true,
					"data": SearchResponse{
						Results: []SearchResult{},
						Query:   "test",
						Pagination: SearchPagination{
							Page:  tt.page,
							Limit: tt.limit,
							Total: 0,
							Pages: 0,
						},
					},
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			result, err := client.Search("test", tt.page, tt.limit)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result.Pagination.Page != tt.page {
				t.Errorf("Pagination.Page = %d, want %d", result.Pagination.Page, tt.page)
			}
		})
	}
}

// Test AutodiscoverResponse JSON serialization
func TestAutodiscoverResponseJSON(t *testing.T) {
	response := AutodiscoverResponse{}
	response.Server.Name = "search"
	response.Server.Version = "1.0.0"
	response.Server.URL = "https://api.example.com"
	response.Server.Features.Auth = true
	response.Server.Features.Search = true
	response.Server.Features.Register = false
	response.API.Version = "v1"
	response.API.BasePath = version.APIPrefix

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded AutodiscoverResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Server.Name != response.Server.Name {
		t.Errorf("decoded.Server.Name = %q, want %q", decoded.Server.Name, response.Server.Name)
	}
	if decoded.Server.Features.Auth != response.Server.Features.Auth {
		t.Error("Server.Features.Auth mismatch")
	}
	if decoded.API.Version != response.API.Version {
		t.Errorf("API.Version = %q, want %q", decoded.API.Version, response.API.Version)
	}
}

// Test SearchResponse JSON serialization
// Per AI.md PART 14: Pagination format {page, limit, total, pages}
func TestSearchResponseJSON(t *testing.T) {
	response := SearchResponse{
		Results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Description: "snippet", Score: 0.9},
		},
		Query: "test",
		Pagination: SearchPagination{
			Page:  1,
			Limit: 10,
			Total: 1,
			Pages: 1,
		},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded SearchResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Pagination.Total != response.Pagination.Total {
		t.Errorf("Pagination.Total = %d, want %d", decoded.Pagination.Total, response.Pagination.Total)
	}
	if len(decoded.Results) != 1 {
		t.Errorf("Results length = %d, want 1", len(decoded.Results))
	}
}

// Test doRequest with invalid HTTP method (should fail at http.NewRequest)
func TestDoRequestInvalidMethod(t *testing.T) {
	client := NewClient("http://example.com", "", 30)
	// Invalid method with space should cause http.NewRequest to fail
	_, err := client.doRequest("INVALID METHOD", "/test", nil)
	if err == nil {
		t.Error("doRequest() should return error for invalid method")
	}
	if !strings.Contains(err.Error(), "failed to create request") {
		t.Errorf("error = %q, want to contain 'failed to create request'", err.Error())
	}
}
