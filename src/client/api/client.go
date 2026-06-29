package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	searchapi "github.com/apimgr/search/src/api"
)

// ProjectName is set at build time - used for User-Agent
var ProjectName = "search"

// Version is set at build time
var Version = "dev"

// Client is the API client for the search server.
// Per AI.md PART 32: single-server client; no cluster failover (AI.md line 2055).
type Client struct {
	BaseURL string
	Token   string
	// Per AI.md PART 32: --user flag for user/org context
	UserContext string
	HTTPClient  *http.Client
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
	Features       map[string]bool   `json:"features,omitempty"`
	Checks         map[string]string `json:"checks,omitempty"`
	Stats          *HealthStats      `json:"stats,omitempty"`
	PendingRestart bool              `json:"pending_restart,omitempty"`
	RestartReason  []string          `json:"restart_reason,omitempty"`
}

// BuildInfo represents build information
type BuildInfo struct {
	Commit string `json:"commit"`
	Date   string `json:"date"`
}

// HealthStats represents health statistics
type HealthStats struct {
	RequestsTotal     int64 `json:"requests_total"`
	Requests24h       int64 `json:"requests_24h"`
	ActiveConnections int   `json:"active_connections"`
}

// InfoResponse represents server information from GET /api/v1/info
type InfoResponse struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Uptime      string          `json:"uptime"`
	Mode        string          `json:"mode"`
	Engines     EnginesSummary  `json:"engines"`
	Features    map[string]bool `json:"features"`
}

// EnginesSummary provides aggregate engine statistics
type EnginesSummary struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

// EngineStatus represents engine status information from /api/v1/engines
type EngineStatus struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Enabled     bool                   `json:"enabled"`
	Priority    int                    `json:"priority"`
	Categories  []string               `json:"categories"`
	Description string                 `json:"description,omitempty"`
	Homepage    string                 `json:"homepage,omitempty"`
	Health      map[string]interface{} `json:"health,omitempty"`
}

// Category represents a search category from /api/v1/categories
type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// Bang represents a bang search shortcut from /api/v1/bangs
type Bang struct {
	Shortcut    string   `json:"shortcut"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Category    string   `json:"category"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// Widget represents a widget definition from /api/v1/widgets
type Widget struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon"`
	Category    string `json:"category"`
	Order       int    `json:"order"`
}

// WidgetData represents data returned by a specific widget from /api/v1/widgets/{name}
type WidgetData struct {
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data,omitempty"`
	UpdatedAt string          `json:"updated_at"`
	Error     string          `json:"error,omitempty"`
}

// InstantAnswer represents an instant answer response from /api/v1/instant
type InstantAnswer struct {
	Query   string                 `json:"query"`
	Type    string                 `json:"type,omitempty"`
	Title   string                 `json:"title,omitempty"`
	Content string                 `json:"content,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Source  string                 `json:"source,omitempty"`
	Found   bool                   `json:"found"`
}

// DirectAnswer represents a direct answer response from /api/v1/direct/{slug}
type DirectAnswer struct {
	Type        string                 `json:"type"`
	Term        string                 `json:"term"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Content     string                 `json:"content"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Source      string                 `json:"source,omitempty"`
	SourceURL   string                 `json:"source_url,omitempty"`
	Found       bool                   `json:"found"`
}

// Preferences represents the user preferences schema from /api/v1/preferences
type Preferences struct {
	Storage string   `json:"storage,omitempty"`
	Fields  []string `json:"fields,omitempty"`
}

// Alert represents a search alert with management metadata from /api/v1/alerts
type Alert struct {
	// Core alert data nested under "alert" key in the API response
	AlertData   map[string]interface{} `json:"alert,omitempty"`
	ManageToken string                 `json:"manage_token,omitempty"`
	ManageURL   string                 `json:"manage_url,omitempty"`
	RSSToken    string                 `json:"rss_token,omitempty"`
	RSSURL      string                 `json:"rss_url,omitempty"`
}

// CreateAlertRequest represents a request to create a search alert via POST /api/v1/alerts
type CreateAlertRequest struct {
	Query          string   `json:"query"`
	Category       string   `json:"category"`
	Language       string   `json:"language"`
	Region         string   `json:"region"`
	Engines        []string `json:"engines"`
	SafeSearch     int      `json:"safe_search"`
	Frequency      string   `json:"frequency"`
	Email          string   `json:"email"`
	DeliverEmail   bool     `json:"deliver_email"`
	DeliverRSS     bool     `json:"deliver_rss"`
	DeliverWebhook bool     `json:"deliver_webhook"`
	WebhookURL     string   `json:"webhook_url,omitempty"`
}

// UpdateAlertRequest represents a request to update a search alert via PATCH /api/v1/alerts/{token}
type UpdateAlertRequest struct {
	Query          string   `json:"query"`
	Category       string   `json:"category"`
	Language       string   `json:"language"`
	Region         string   `json:"region"`
	Engines        []string `json:"engines"`
	Frequency      string   `json:"frequency"`
	SafeSearch     int      `json:"safe_search"`
	DeliverEmail   bool     `json:"deliver_email"`
	DeliverRSS     bool     `json:"deliver_rss"`
	DeliverWebhook bool     `json:"deliver_webhook"`
	WebhookURL     string   `json:"webhook_url,omitempty"`
}

// StatusResponse represents detailed server status from GET /server/status
type StatusResponse struct {
	Status  string                 `json:"status"`
	Version string                 `json:"version"`
	Mode    string                 `json:"mode"`
	Uptime  string                 `json:"uptime"`
	Checks  map[string]string      `json:"checks"`
	Stats   map[string]interface{} `json:"stats,omitempty"`
}

// ServerConfigResponse represents server configuration from GET /server/config
type ServerConfigResponse struct {
	Config map[string]interface{} `json:"config"`
}

// Search performs a search query
func (c *Client) Search(query string, page, perPage int) (*SearchResponse, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("limit", fmt.Sprintf("%d", perPage))

	resp, err := c.get(searchapi.APIPrefix + "/search?" + params.Encode())
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

// Health checks server health using the canonical /server/healthz endpoint per AI.md PART 13
func (c *Client) Health() (*HealthResponse, error) {
	resp, err := c.get("/server/healthz")
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

// GetInfo returns server information from GET /api/v1/info
func (c *Client) GetInfo() (*InfoResponse, error) {
	resp, err := c.get(searchapi.APIPrefix + "/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result InfoResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode info data: %w", err)
	}
	return &result, nil
}

// GetRelatedSearches returns related search suggestions from GET /api/v1/search/related
func (c *Client) GetRelatedSearches(query string) ([]string, error) {
	params := url.Values{}
	params.Set("q", query)

	resp, err := c.get(searchapi.APIPrefix + "/search/related?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var relatedData struct {
		Suggestions []string `json:"suggestions"`
	}
	if err := json.Unmarshal(apiResp.Data, &relatedData); err != nil {
		return nil, fmt.Errorf("failed to decode related searches data: %w", err)
	}
	return relatedData.Suggestions, nil
}

// GetAutocomplete returns autocomplete suggestions from GET /api/v1/autocomplete
func (c *Client) GetAutocomplete(query string) ([]string, error) {
	params := url.Values{}
	params.Set("q", query)

	resp, err := c.get(searchapi.APIPrefix + "/autocomplete?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var suggestions []string
	if err := json.Unmarshal(apiResp.Data, &suggestions); err != nil {
		return nil, fmt.Errorf("failed to decode autocomplete data: %w", err)
	}
	return suggestions, nil
}

// GetEngines returns all search engine statuses from GET /api/v1/engines
func (c *Client) GetEngines() ([]EngineStatus, error) {
	resp, err := c.get(searchapi.APIPrefix + "/engines")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var engines []EngineStatus
	if err := json.Unmarshal(apiResp.Data, &engines); err != nil {
		return nil, fmt.Errorf("failed to decode engines data: %w", err)
	}
	return engines, nil
}

// GetEngineByID returns status information for a specific engine from GET /api/v1/engines/{id}
func (c *Client) GetEngineByID(id string) (*EngineStatus, error) {
	resp, err := c.get(searchapi.APIPrefix + "/engines/" + url.PathEscape(id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var engine EngineStatus
	if err := json.Unmarshal(apiResp.Data, &engine); err != nil {
		return nil, fmt.Errorf("failed to decode engine data: %w", err)
	}
	return &engine, nil
}

// GetCategories returns all search categories from GET /api/v1/categories
func (c *Client) GetCategories() ([]Category, error) {
	resp, err := c.get(searchapi.APIPrefix + "/categories")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var categories []Category
	if err := json.Unmarshal(apiResp.Data, &categories); err != nil {
		return nil, fmt.Errorf("failed to decode categories data: %w", err)
	}
	return categories, nil
}

// GetBangs returns available bang shortcuts from GET /api/v1/bangs
func (c *Client) GetBangs() ([]Bang, error) {
	resp, err := c.get(searchapi.APIPrefix + "/bangs")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var bangsData struct {
		Bangs []Bang `json:"bangs"`
	}
	if err := json.Unmarshal(apiResp.Data, &bangsData); err != nil {
		return nil, fmt.Errorf("failed to decode bangs data: %w", err)
	}
	return bangsData.Bangs, nil
}

// GetWidgets returns available widget definitions from GET /api/v1/widgets
func (c *Client) GetWidgets() ([]Widget, error) {
	resp, err := c.get(searchapi.APIPrefix + "/widgets")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var widgetsData struct {
		Widgets []Widget `json:"widgets"`
	}
	if err := json.Unmarshal(apiResp.Data, &widgetsData); err != nil {
		return nil, fmt.Errorf("failed to decode widgets data: %w", err)
	}
	return widgetsData.Widgets, nil
}

// GetWidgetData fetches data for a specific widget from GET /api/v1/widgets/{name}
func (c *Client) GetWidgetData(name string, params url.Values) (*WidgetData, error) {
	path := searchapi.APIPrefix + "/widgets/" + url.PathEscape(name)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}

	resp, err := c.get(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result WidgetData
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode widget data: %w", err)
	}
	return &result, nil
}

// GetInstantAnswer fetches an instant answer for a query from GET /api/v1/instant
func (c *Client) GetInstantAnswer(query string) (*InstantAnswer, error) {
	params := url.Values{}
	params.Set("q", query)

	resp, err := c.get(searchapi.APIPrefix + "/instant?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result InstantAnswer
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode instant answer data: %w", err)
	}
	return &result, nil
}

// GetDirectAnswer fetches a direct answer for a slug from GET /api/v1/direct/{slug}
func (c *Client) GetDirectAnswer(slug string) (*DirectAnswer, error) {
	resp, err := c.get(searchapi.APIPrefix + "/direct/" + slug)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result DirectAnswer
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode direct answer data: %w", err)
	}
	return &result, nil
}

// GetPreferences returns the user preferences schema from GET /api/v1/preferences
func (c *Client) GetPreferences() (*Preferences, error) {
	resp, err := c.get(searchapi.APIPrefix + "/preferences")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result Preferences
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode preferences data: %w", err)
	}
	return &result, nil
}

// SetPreferences saves user preferences via PUT /api/v1/preferences
func (c *Client) SetPreferences(prefs *Preferences) error {
	resp, err := c.doRequest("PUT", searchapi.APIPrefix+"/preferences", prefs)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// CreateAlert creates a new search alert via POST /api/v1/alerts
func (c *Client) CreateAlert(req *CreateAlertRequest) (*Alert, error) {
	resp, err := c.doRequest("POST", searchapi.APIPrefix+"/alerts", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result Alert
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode alert data: %w", err)
	}
	return &result, nil
}

// GetAlert retrieves a search alert by manage token via GET /api/v1/alerts/{manageToken}
func (c *Client) GetAlert(manageToken string) (*Alert, error) {
	resp, err := c.get(searchapi.APIPrefix + "/alerts/" + url.PathEscape(manageToken))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result Alert
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode alert data: %w", err)
	}
	return &result, nil
}

// UpdateAlert updates a search alert by manage token via PATCH /api/v1/alerts/{manageToken}
func (c *Client) UpdateAlert(manageToken string, req *UpdateAlertRequest) (*Alert, error) {
	resp, err := c.doRequest("PATCH", searchapi.APIPrefix+"/alerts/"+url.PathEscape(manageToken), req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result Alert
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode alert data: %w", err)
	}
	return &result, nil
}

// DeleteAlert deletes a search alert by manage token via DELETE /api/v1/alerts/{manageToken}
func (c *Client) DeleteAlert(manageToken string) error {
	resp, err := c.doRequest("DELETE", searchapi.APIPrefix+"/alerts/"+url.PathEscape(manageToken), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// GetStatus returns detailed server status from GET /server/status (requires operator token)
func (c *Client) GetStatus() (*StatusResponse, error) {
	resp, err := c.get("/server/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result StatusResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode status data: %w", err)
	}
	return &result, nil
}

// GetServerConfig returns server configuration from GET /server/config (requires operator token)
func (c *Client) GetServerConfig() (*ServerConfigResponse, error) {
	resp, err := c.get("/server/config")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result ServerConfigResponse
	if err := json.Unmarshal(apiResp.Data, &result); err != nil {
		return nil, fmt.Errorf("failed to decode server config data: %w", err)
	}
	return &result, nil
}

// GetMetrics returns Prometheus-format metrics text from GET /server/metrics (requires operator token)
func (c *Client) GetMetrics() (string, error) {
	resp, err := c.get("/server/metrics")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metrics response: %w", err)
	}
	return string(body), nil
}

// SetUserContext sets the user or org context for API requests
// Per AI.md PART 32 line 41490-41498: --user flag for user/org context
// @name = force user context
// +name = force org context
// name = auto-detect
func (c *Client) SetUserContext(ctx string) {
	c.UserContext = ctx
}

// CLIBinaryInfo holds the version and SHA-256 checksum for a CLI binary platform.
// Per AI.md PART 32: cli_versions in autodiscover response, keyed by "{os}-{arch}".
type CLIBinaryInfo struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// AutodiscoverResponse represents /api/autodiscover response.
// Per AI.md PART 32: includes cli_versions and cli_min_version for auto-update checks.
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
	API struct {
		Version  string `json:"version"`
		BasePath string `json:"base_path"`
	} `json:"api"`
	// Per AI.md PART 32: CLI binary info per platform for auto-update.
	CLIVersions   map[string]CLIBinaryInfo `json:"cli_versions"`
	CLIMinVersion string                   `json:"cli_min_version"`
}

// Autodiscover fetches server settings from /api/autodiscover.
// Per AI.md PART 32: Non-versioned autodiscover endpoint.
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

	return &result, nil
}

// CurrentPlatform returns the "{os}-{arch}" key used in autodiscover cli_versions.
// Per AI.md PART 32: keys are "{os}-{arch}" e.g. "linux-amd64".
func CurrentPlatform() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// get performs a GET request to the primary server.
// Per AI.md line 2055: single-instance only; no cluster failover.
func (c *Client) get(path string) (*http.Response, error) {
	return c.doRequest("GET", path, nil)
}

// doRequest performs an HTTP request to the primary server (BaseURL).
func (c *Client) doRequest(method, path string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyReader = strings.NewReader(string(bodyBytes))
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Fixed User-Agent per AI.md PART 32
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
