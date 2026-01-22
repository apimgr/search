package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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
		BaseURL:      "https://api.example.com",
		Token:        "test-token",
		UserContext:  "testuser",
		ClusterNodes: []string{"node1", "node2"},
		HTTPClient:   http.DefaultClient,
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
	if len(client.ClusterNodes) != 2 {
		t.Errorf("ClusterNodes length = %d", len(client.ClusterNodes))
	}
}

// Tests for SearchResult struct

func TestSearchResultStruct(t *testing.T) {
	result := SearchResult{
		ID:      "123",
		Title:   "Test Title",
		URL:     "https://example.com",
		Snippet: "Test snippet",
		Score:   0.95,
	}

	if result.ID != "123" {
		t.Errorf("ID = %q", result.ID)
	}
	if result.Title != "Test Title" {
		t.Errorf("Title = %q", result.Title)
	}
	if result.URL != "https://example.com" {
		t.Errorf("URL = %q", result.URL)
	}
	if result.Snippet != "Test snippet" {
		t.Errorf("Snippet = %q", result.Snippet)
	}
	if result.Score != 0.95 {
		t.Errorf("Score = %f", result.Score)
	}
}

func TestSearchResultJSON(t *testing.T) {
	result := SearchResult{
		ID:      "123",
		Title:   "Test",
		URL:     "https://example.com",
		Snippet: "Snippet",
		Score:   0.5,
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded SearchResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.ID != result.ID {
		t.Errorf("decoded.ID = %q, want %q", decoded.ID, result.ID)
	}
}

// Tests for SearchResponse struct

func TestSearchResponseStruct(t *testing.T) {
	response := SearchResponse{
		Results: []SearchResult{
			{ID: "1", Title: "Result 1"},
			{ID: "2", Title: "Result 2"},
		},
		TotalCount: 100,
		Query:      "test query",
		Page:       1,
		PerPage:    10,
	}

	if len(response.Results) != 2 {
		t.Errorf("Results length = %d", len(response.Results))
	}
	if response.TotalCount != 100 {
		t.Errorf("TotalCount = %d", response.TotalCount)
	}
	if response.Query != "test query" {
		t.Errorf("Query = %q", response.Query)
	}
	if response.Page != 1 {
		t.Errorf("Page = %d", response.Page)
	}
	if response.PerPage != 10 {
		t.Errorf("PerPage = %d", response.PerPage)
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
	response.Cluster.Primary = "https://primary.example.com"
	response.Cluster.Nodes = []string{"node1", "node2"}
	response.API.Version = "v1"
	response.API.BasePath = "/api/v1"

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
	if response.Cluster.Primary != "https://primary.example.com" {
		t.Errorf("Cluster.Primary = %q", response.Cluster.Primary)
	}
	if len(response.Cluster.Nodes) != 2 {
		t.Errorf("Cluster.Nodes length = %d", len(response.Cluster.Nodes))
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

// Tests for SetClusterNodes

func TestSetClusterNodes(t *testing.T) {
	client := NewClient("https://api.example.com", "", 30)

	nodes := []string{"node1.example.com", "node2.example.com", "node3.example.com"}
	client.SetClusterNodes(nodes)

	got := client.GetClusterNodes()
	if len(got) != 3 {
		t.Errorf("GetClusterNodes() length = %d, want 3", len(got))
	}
	for i, node := range nodes {
		if got[i] != node {
			t.Errorf("GetClusterNodes()[%d] = %q, want %q", i, got[i], node)
		}
	}
}

func TestSetClusterNodesEmpty(t *testing.T) {
	client := NewClient("https://api.example.com", "", 30)

	client.SetClusterNodes([]string{})

	got := client.GetClusterNodes()
	if len(got) != 0 {
		t.Errorf("GetClusterNodes() length = %d, want 0", len(got))
	}
}

func TestGetClusterNodesNil(t *testing.T) {
	client := NewClient("https://api.example.com", "", 30)

	got := client.GetClusterNodes()
	if got != nil && len(got) != 0 {
		t.Errorf("GetClusterNodes() should be nil or empty, got %v", got)
	}
}

// Tests for Search

func TestSearch(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			t.Errorf("Expected path /api/v1/search, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != "test query" {
			t.Errorf("Expected q=test+query, got %s", r.URL.Query().Get("q"))
		}

		response := SearchResponse{
			Results: []SearchResult{
				{ID: "1", Title: "Result 1", URL: "https://example.com/1"},
			},
			TotalCount: 1,
			Query:      "test query",
			Page:       1,
			PerPage:    10,
		}
		json.NewEncoder(w).Encode(response)
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
		if r.URL.Path != "/api/v1/healthz" {
			t.Errorf("Expected path /api/v1/healthz, got %s", r.URL.Path)
		}

		response := HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
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
		response := HealthResponse{
			Status:  "healthy",
			Version: "2.0.0",
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
		response.Cluster.Nodes = []string{"node1", "node2"}
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

	// Should have updated cluster nodes
	nodes := client.GetClusterNodes()
	if len(nodes) != 2 {
		t.Errorf("ClusterNodes should be updated, got length %d", len(nodes))
	}
}

// Tests for cluster failover

func TestGetWithFailover(t *testing.T) {
	// Primary fails, fallback succeeds
	failCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		response := HealthResponse{Status: "healthy"}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	resp, err := client.get("/api/v1/healthz")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	resp.Body.Close()

	if failCount != 1 {
		t.Errorf("Request count = %d, want 1", failCount)
	}
}

func TestGetWithFailoverClusterNode(t *testing.T) {
	// Create primary that fails
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()

	// Create fallback that works
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := HealthResponse{Status: "healthy"}
		json.NewEncoder(w).Encode(response)
	}))
	defer fallback.Close()

	client := NewClient(primary.URL, "", 30)
	client.SetClusterNodes([]string{fallback.URL})

	resp, err := client.getWithFailover("/api/v1/healthz")
	if err != nil {
		t.Fatalf("getWithFailover() error = %v", err)
	}
	resp.Body.Close()
}

func TestGetWithFailoverAllFail(t *testing.T) {
	// Create primary that fails
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()

	// Create fallback that also fails
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer fallback.Close()

	client := NewClient(primary.URL, "", 30)
	client.SetClusterNodes([]string{fallback.URL})

	_, err := client.getWithFailover("/api/v1/healthz")
	if err == nil {
		t.Error("getWithFailover() should return error when all nodes fail")
	}
}

// Tests for doRequestToServer

func TestDoRequestToServer(t *testing.T) {
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
	resp, err := client.doRequestToServer(server.URL, "GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequestToServer() error = %v", err)
	}
	resp.Body.Close()
}

func TestDoRequestToServerWithToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want 'Bearer test-token'", auth)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", 30)
	resp, err := client.doRequestToServer(server.URL, "GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequestToServer() error = %v", err)
	}
	resp.Body.Close()
}

func TestDoRequestToServerWithBody(t *testing.T) {
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
	resp, err := client.doRequestToServer(server.URL, "POST", "/test", body)
	if err != nil {
		t.Fatalf("doRequestToServer() error = %v", err)
	}
	resp.Body.Close()
}

func TestDoRequestToServer400Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error": "bad request"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.doRequestToServer(server.URL, "GET", "/test", nil)
	if err == nil {
		t.Error("doRequestToServer() should return error for 400 response")
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

// Tests for concurrent access

func TestSetClusterNodesConcurrent(t *testing.T) {
	client := NewClient("https://api.example.com", "", 30)

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			client.SetClusterNodes([]string{"node1", "node2"})
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			client.GetClusterNodes()
		}
		done <- true
	}()

	<-done
	<-done
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

// Test Autodiscover with empty cluster nodes (no update)
func TestAutodiscoverEmptyClusterNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := AutodiscoverResponse{}
		response.Server.Name = "search"
		response.Server.Version = "1.0.0"
		response.Cluster.Nodes = []string{} // Empty nodes
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	// Set some existing nodes first
	client.SetClusterNodes([]string{"existing-node"})

	result, err := client.Autodiscover()
	if err != nil {
		t.Fatalf("Autodiscover() error = %v", err)
	}
	if result.Server.Name != "search" {
		t.Errorf("result.Server.Name = %q", result.Server.Name)
	}

	// Cluster nodes should NOT be updated (still has existing-node)
	nodes := client.GetClusterNodes()
	if len(nodes) != 1 || nodes[0] != "existing-node" {
		t.Errorf("ClusterNodes should not be updated when response has empty nodes, got %v", nodes)
	}
}

// Test getWithFailover when cluster node equals BaseURL (skip logic)
func TestGetWithFailoverSkipPrimaryInNodes(t *testing.T) {
	// Create primary that fails
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()

	// Create fallback that works
	fallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := HealthResponse{Status: "healthy"}
		json.NewEncoder(w).Encode(response)
	}))
	defer fallback.Close()

	client := NewClient(primary.URL, "", 30)
	// Set cluster nodes including the primary URL (should be skipped)
	client.SetClusterNodes([]string{primary.URL, fallback.URL})

	resp, err := client.getWithFailover("/api/v1/healthz")
	if err != nil {
		t.Fatalf("getWithFailover() error = %v", err)
	}
	resp.Body.Close()
}

// Test doRequestToServer with network connection refused
func TestDoRequestToServerConnectionRefused(t *testing.T) {
	client := NewClient("http://127.0.0.1:59999", "", 1) // Invalid port, connection refused
	_, err := client.doRequestToServer("http://127.0.0.1:59999", "GET", "/test", nil)
	if err == nil {
		t.Error("doRequestToServer() should return error for connection refused")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("error = %q, want to contain 'request failed'", err.Error())
	}
}

// Test doRequestToServer with body containing unmarshalable type
func TestDoRequestToServerMarshalError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	// Create an unmarshalable body (channel cannot be marshaled to JSON)
	unmarshalableBody := make(chan int)
	_, err := client.doRequestToServer(server.URL, "POST", "/test", unmarshalableBody)
	if err == nil {
		t.Error("doRequestToServer() should return error for unmarshalable body")
	}
	if !strings.Contains(err.Error(), "failed to marshal body") {
		t.Errorf("error = %q, want to contain 'failed to marshal body'", err.Error())
	}
}

// Test getWithFailover with no cluster nodes
func TestGetWithFailoverNoClusterNodes(t *testing.T) {
	// Create primary that fails
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer primary.Close()

	client := NewClient(primary.URL, "", 30)
	// No cluster nodes set

	_, err := client.getWithFailover("/api/v1/healthz")
	if err == nil {
		t.Error("getWithFailover() should return error when primary fails and no cluster nodes")
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
		response := HealthResponse{
			Status:  "healthy",
			Version: "",
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

// Test doRequestToServer without token
func TestDoRequestToServerNoToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Authorization header = %q, want empty", auth)
		}
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30) // No token
	resp, err := client.doRequestToServer(server.URL, "GET", "/test", nil)
	if err != nil {
		t.Fatalf("doRequestToServer() error = %v", err)
	}
	resp.Body.Close()
}

// Test search with different pagination values
func TestSearchPagination(t *testing.T) {
	tests := []struct {
		name    string
		page    int
		perPage int
	}{
		{"first page", 1, 10},
		{"second page", 2, 10},
		{"large per page", 1, 100},
		{"zero values", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := SearchResponse{
					Results:    []SearchResult{},
					TotalCount: 0,
					Query:      "test",
					Page:       tt.page,
					PerPage:    tt.perPage,
				}
				json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			result, err := client.Search("test", tt.page, tt.perPage)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if result.Page != tt.page {
				t.Errorf("Page = %d, want %d", result.Page, tt.page)
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
	response.Cluster.Primary = "https://primary.example.com"
	response.Cluster.Nodes = []string{"node1", "node2"}
	response.API.Version = "v1"
	response.API.BasePath = "/api/v1"

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
	if len(decoded.Cluster.Nodes) != len(response.Cluster.Nodes) {
		t.Error("Cluster.Nodes length mismatch")
	}
}

// Test SearchResponse JSON serialization
func TestSearchResponseJSON(t *testing.T) {
	response := SearchResponse{
		Results: []SearchResult{
			{ID: "1", Title: "Result 1", URL: "https://example.com/1", Snippet: "snippet", Score: 0.9},
		},
		TotalCount: 1,
		Query:      "test",
		Page:       1,
		PerPage:    10,
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded SearchResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.TotalCount != response.TotalCount {
		t.Errorf("TotalCount = %d, want %d", decoded.TotalCount, response.TotalCount)
	}
	if len(decoded.Results) != 1 {
		t.Errorf("Results length = %d, want 1", len(decoded.Results))
	}
}

// Test get method (wrapper for getWithFailover)
func TestGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	resp, err := client.get("/test")
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}
	resp.Body.Close()
}

// Test doRequestToServer with invalid URL (should fail at http.NewRequest)
func TestDoRequestToServerInvalidMethod(t *testing.T) {
	client := NewClient("http://example.com", "", 30)
	// Invalid method with space should cause http.NewRequest to fail
	_, err := client.doRequestToServer("http://example.com", "INVALID METHOD", "/test", nil)
	if err == nil {
		t.Error("doRequestToServer() should return error for invalid method")
	}
	if !strings.Contains(err.Error(), "failed to create request") {
		t.Errorf("error = %q, want to contain 'failed to create request'", err.Error())
	}
}
