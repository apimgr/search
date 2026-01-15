package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ProjectName is set at build time - used for User-Agent
var ProjectName = "search"

// Version is set at build time
var Version = "dev"

// Client is the API client for the search server
// Per AI.md PART 36 line 42791-42818: Cluster failover support
type Client struct {
	BaseURL      string
	Token        string
	UserContext  string   // Per AI.md PART 36: --user flag for user/org context
	ClusterNodes []string // Per AI.md PART 36: cluster nodes for failover
	HTTPClient   *http.Client
	mu           sync.RWMutex
}

// NewClient creates a new API client
func NewClient(baseURL, token string, timeout int) *Client {
	return &Client{
		BaseURL: baseURL,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

// SearchResult represents a search result
type SearchResult struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// SearchResponse represents the API search response
type SearchResponse struct {
	Results    []SearchResult `json:"results"`
	TotalCount int            `json:"total_count"`
	Query      string         `json:"query"`
	Page       int            `json:"page"`
	PerPage    int            `json:"per_page"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string            `json:"status"`
	Version   string            `json:"version,omitempty"`
	Uptime    string            `json:"uptime,omitempty"`
	Timestamp string            `json:"timestamp,omitempty"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// Search performs a search query
func (c *Client) Search(query string, page, perPage int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("per_page", fmt.Sprintf("%d", perPage))

	resp, err := c.get("/api/v1/search?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// Health checks server health
func (c *Client) Health() (*HealthResponse, error) {
	resp, err := c.get("/api/v1/healthz")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &result, nil
}

// GetVersion returns server version info
func (c *Client) GetVersion() (string, error) {
	health, err := c.Health()
	if err != nil {
		return "", err
	}
	return health.Version, nil
}

// SetUserContext sets the user or org context for API requests
// Per AI.md PART 36 line 41490-41498: --user flag for user/org context
// @name = force user context
// +name = force org context
// name = auto-detect
func (c *Client) SetUserContext(ctx string) {
	c.UserContext = ctx
}

// AutodiscoverResponse represents /api/autodiscover response
// Per AI.md PART 36 line 38077-38157: Autodiscover endpoint
type AutodiscoverResponse struct {
	Server struct {
		Name     string `json:"name"`
		Version  string `json:"version"`
		URL      string `json:"url"`
		Features struct {
			Auth     bool `json:"auth"`
			Search   bool `json:"search"`
			Register bool `json:"register"`
		} `json:"features"`
	} `json:"server"`
	Cluster struct {
		Primary string   `json:"primary"`
		Nodes   []string `json:"nodes"`
	} `json:"cluster"`
	API struct {
		Version  string `json:"version"`
		BasePath string `json:"base_path"`
	} `json:"api"`
}

// SetClusterNodes sets the cluster nodes for failover
// Per AI.md PART 36 line 42791-42818: Cluster failover
func (c *Client) SetClusterNodes(nodes []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ClusterNodes = nodes
}

// GetClusterNodes returns current cluster nodes
func (c *Client) GetClusterNodes() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ClusterNodes
}

// Autodiscover fetches server settings from /api/autodiscover
// Per AI.md PART 36 line 38077-38157: Non-versioned autodiscover endpoint
func (c *Client) Autodiscover() (*AutodiscoverResponse, error) {
	resp, err := c.doRequest("GET", "/api/autodiscover", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result AutodiscoverResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode autodiscover response: %w", err)
	}

	// Update cluster nodes from response
	if len(result.Cluster.Nodes) > 0 {
		c.SetClusterNodes(result.Cluster.Nodes)
	}

	return &result, nil
}

// get performs a GET request with automatic cluster failover
// Per AI.md PART 36 line 42802-42812: Failover behavior
func (c *Client) get(path string) (*http.Response, error) {
	return c.getWithFailover(path)
}

// getWithFailover tries primary server first, then cluster nodes
// Per AI.md PART 36 line 42802-42812: Cluster failover
func (c *Client) getWithFailover(path string) (*http.Response, error) {
	// 1. Try primary server (BaseURL)
	resp, err := c.doRequest("GET", path, nil)
	if err == nil {
		return resp, nil
	}

	// 2. If fails, try cluster nodes (silent failover)
	c.mu.RLock()
	nodes := c.ClusterNodes
	c.mu.RUnlock()

	for _, node := range nodes {
		if node == c.BaseURL {
			continue // Skip primary, already tried
		}
		// Try this node
		resp, nodeErr := c.doRequestToServer(node, "GET", path, nil)
		if nodeErr == nil {
			return resp, nil
		}
	}

	// All nodes failed, return original error
	return nil, err
}

// doRequestToServer performs HTTP request to specific server
// Per AI.md PART 36: Cluster failover support
func (c *Client) doRequestToServer(serverURL, method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, serverURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Fixed User-Agent per AI.md PART 36
	req.Header.Set("User-Agent", fmt.Sprintf("%s-cli/%s", ProjectName, Version))
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		bodyData, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("server error %d: %s", resp.StatusCode, string(bodyData))
	}

	return resp, nil
}
