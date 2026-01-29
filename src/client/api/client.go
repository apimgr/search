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
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	Engine      string  `json:"engine"`
	Score       float64 `json:"score"`
	Category    string  `json:"category"`
	Thumbnail   string  `json:"thumbnail,omitempty"`
	Domain      string  `json:"domain,omitempty"`
}

// SearchPagination represents pagination info per AI.md PART 14
type SearchPagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

// SearchResponse represents the API search response per AI.md PART 14
type SearchResponse struct {
	Query      string           `json:"query"`
	Category   string           `json:"category"`
	Results    []SearchResult   `json:"results"`
	Pagination SearchPagination `json:"pagination"`
	SearchTime float64          `json:"search_time_ms"`
	Engines    []string         `json:"engines_used"`
}

// apiResponse wraps API responses with ok and data fields
type apiResponse struct {
	OK   bool            `json:"ok"`
	Data json.RawMessage `json:"data"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status         string            `json:"status"`
	Version        string            `json:"version,omitempty"`
	GoVersion      string            `json:"go_version,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	Uptime         string            `json:"uptime,omitempty"`
	Timestamp      string            `json:"timestamp,omitempty"`
	Build          *BuildInfo        `json:"build,omitempty"`
	Node           *NodeInfo         `json:"node,omitempty"`
	Cluster        *ClusterInfo      `json:"cluster,omitempty"`
	Features       map[string]bool   `json:"features,omitempty"`
	Checks         map[string]string `json:"checks,omitempty"`
	Stats          *HealthStats      `json:"stats,omitempty"`
	PendingRestart bool              `json:"pending_restart,omitempty"`
	RestartReason  []string          `json:"restart_reason,omitempty"`
}

// BuildInfo represents build information
type BuildInfo struct {
	CommitID  string `json:"commit_id"`
	BuildDate string `json:"build_date"`
}

// NodeInfo represents node information
type NodeInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

// ClusterInfo represents cluster information
type ClusterInfo struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Nodes   int    `json:"nodes"`
}

// HealthStats represents health statistics
type HealthStats struct {
	RequestsTotal     int64 `json:"requests_total"`
	Requests24h       int64 `json:"requests_24h"`
	ActiveConnections int   `json:"active_connections"`
}

// Search performs a search query
func (c *Client) Search(query string, page, perPage int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("limit", fmt.Sprintf("%d", perPage))

	resp, err := c.get("/api/v1/search?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// API returns {"ok": true, "data": {...}} - unwrap the data field
	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("API returned error")
	}

	var result SearchResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode search data: %w", err)
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

	// API returns {"ok": true, "data": {...}} - unwrap the data field
	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result HealthResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode health data: %w", err)
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
