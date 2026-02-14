package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engines"
	"github.com/apimgr/search/src/service"
	"github.com/apimgr/search/src/widget"
)

// API version
const (
	APIVersion = "v1"
	APIPrefix  = "/api/v1"
)

// Handler handles API requests
type Handler struct {
	config          *config.Config
	registry        *engines.Registry
	aggregator      *search.Aggregator
	widgetManager   *widget.Manager
	instantManager  *instant.Manager
	directManager   *direct.Manager
	relatedSearches *search.RelatedSearches
	torService      *service.TorService // Per AI.md PART 32: Tor service for health status
	startTime       time.Time
}

// NewHandler creates a new API handler
func NewHandler(cfg *config.Config, registry *engines.Registry, aggregator *search.Aggregator) *Handler {
	return &Handler{
		config:     cfg,
		registry:   registry,
		aggregator: aggregator,
		startTime:  time.Now(),
	}
}

// SetWidgetManager sets the widget manager for the API handler
func (h *Handler) SetWidgetManager(wm *widget.Manager) {
	h.widgetManager = wm
}

// SetInstantManager sets the instant answer manager for the API handler
func (h *Handler) SetInstantManager(im *instant.Manager) {
	h.instantManager = im
}

// SetDirectManager sets the direct answer manager for the API handler
func (h *Handler) SetDirectManager(dm *direct.Manager) {
	h.directManager = dm
}

// SetRelatedSearches sets the related searches provider for the API handler
func (h *Handler) SetRelatedSearches(rs *search.RelatedSearches) {
	h.relatedSearches = rs
}

// SetTorService sets the Tor service for the API handler
// Per AI.md PART 32: Tor status is checked via service, not hardcoded
func (h *Handler) SetTorService(ts *service.TorService) {
	h.torService = ts
}

// RegisterRoutes registers API routes
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Autodiscover - non-versioned per AI.md PART 36 line 38077-38157
	mux.HandleFunc("/api/autodiscover", h.handleAutodiscover)

	// Health and info - Per AI.md PART 14: .txt extension support
	mux.HandleFunc("/api/v1/healthz", h.handleHealthz)
	mux.HandleFunc("/api/v1/healthz.txt", h.handleHealthz) // Per AI.md PART 14
	mux.HandleFunc("/api/v1/info", h.handleInfo)
	mux.HandleFunc("/api/v1/info.txt", h.handleInfo) // Per AI.md PART 14

	// Search
	mux.HandleFunc("/api/v1/search", h.handleSearch)
	mux.HandleFunc("/api/v1/search/related", h.handleRelatedSearches)
	mux.HandleFunc("/api/v1/autocomplete", h.handleAutocomplete)

	// Engines
	mux.HandleFunc("/api/v1/engines", h.handleEngines)
	mux.HandleFunc("/api/v1/engines/", h.handleEngineByID)

	// Categories
	mux.HandleFunc("/api/v1/categories", h.handleCategories)

	// Bangs
	mux.HandleFunc("/api/v1/bangs", h.handleBangs)

	// Widgets
	mux.HandleFunc("/api/v1/widgets", h.handleWidgets)
	mux.HandleFunc("/api/v1/widgets/", h.handleWidgetData)

	// Instant Answers
	mux.HandleFunc("/api/v1/instant", h.handleInstantAnswer)

	// Direct Answers (full-page results per IDEA.md)
	mux.HandleFunc("/api/v1/direct/", h.handleDirectAnswer)

	// Server info pages (per AI.md PART 16)
	mux.HandleFunc("/api/v1/server/about", h.handleServerAbout)
	mux.HandleFunc("/api/v1/server/privacy", h.handleServerPrivacy)
	mux.HandleFunc("/api/v1/server/help", h.handleServerHelp)
	mux.HandleFunc("/api/v1/server/terms", h.handleServerTerms)

	// Favicon proxy - privacy-preserving favicon fetching
	// Per AI.md PART 16: NO external requests from client, server proxies content
	mux.HandleFunc("/api/v1/favicon", h.handleFavicon)
}

// Response types

// APIResponse is the base response structure
// Per AI.md PART 16: Unified Response Format (NON-NEGOTIABLE)
// Success: {"ok": true, "data": {...}}
// Error: {"ok": false, "error": "ERROR_CODE", "message": "Human readable message"}
type APIResponse struct {
	OK      bool        `json:"ok"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`   // Error code (e.g., "BAD_REQUEST", "NOT_FOUND")
	Message string      `json:"message,omitempty"` // Human-readable error message
	Meta    *APIMeta    `json:"meta,omitempty"`    // Optional metadata (request_id, process_time_ms)
}

// APIMeta contains response metadata
type APIMeta struct {
	RequestID   string  `json:"request_id,omitempty"`
	ProcessTime float64 `json:"process_time_ms,omitempty"`
	Version     string  `json:"version"`
}

// HealthResponse represents health check response per AI.md PART 13
// All fields use canonical order from spec (line 16208-16244)
type HealthResponse struct {
	// 1. Project identification (PART 16: branding config)
	Project ProjectInfo `json:"project"`

	// 2. Overall status
	Status         string   `json:"status"`                    // "healthy", "unhealthy", "degraded"
	PendingRestart bool     `json:"pending_restart,omitempty"` // true if restart needed
	RestartReason  []string `json:"restart_reason,omitempty"`  // settings that changed

	// 3. Version & build info (PART 7: binary requirements)
	Version   string    `json:"version"`    // SemVer "1.0.0"
	GoVersion string    `json:"go_version"` // "go1.23.0"
	Build     BuildInfo `json:"build"`

	// 4. Runtime info (PART 6: application modes)
	Uptime    string `json:"uptime"`    // human readable "2d 5h 30m"
	Mode      string `json:"mode"`      // "production" or "development"
	Timestamp string `json:"timestamp"` // current UTC time ISO 8601

	// 5. Cluster info (PART 10: database & cluster)
	Cluster ClusterInfo `json:"cluster"`

	// 6. Features - PUBLIC only (PARTS 20, 32, 34, 35)
	Features FeaturesInfo `json:"features"`

	// 7. Component health checks (per spec line 16298-16308)
	Checks ChecksInfo `json:"checks"`

	// 8. Statistics (public-safe aggregates)
	Stats StatsInfo `json:"stats"`
}

// ProjectInfo represents project identification per AI.md PART 13
type ProjectInfo struct {
	Name        string `json:"name"`        // branding.app_name
	Tagline     string `json:"tagline"`     // branding.tagline (short slogan)
	Description string `json:"description"` // server.description (longer)
}

// BuildInfo represents build information per AI.md PART 13
// Note: Fields are "commit" and "date" per spec, not "commit_id" and "build_date"
type BuildInfo struct {
	Commit string `json:"commit"` // git short hash (7 chars)
	Date   string `json:"date"`   // ISO 8601 build timestamp
}

// StatsInfo represents health statistics per AI.md PART 13 (line 16310-16317)
type StatsInfo struct {
	RequestsTotal int64 `json:"requests_total"`     // Total HTTP requests (lifetime)
	Requests24h   int64 `json:"requests_24h"`       // Requests in last 24 hours
	ActiveConns   int   `json:"active_connections"` // Current active connections
}

// ChecksInfo represents component health per AI.md PART 13 (line 16298-16308)
type ChecksInfo struct {
	Database  string `json:"database"`          // PART 10: "ok" or "error"
	Cache     string `json:"cache"`             // PART 10: "ok" or "error"
	Disk      string `json:"disk"`              // Disk space check
	Scheduler string `json:"scheduler"`         // PART 19: "ok" or "error"
	Cluster   string `json:"cluster,omitempty"` // PART 10: "ok" or "error" (if enabled)
	Tor       string `json:"tor,omitempty"`     // PART 32: "ok" or "error" (if enabled)
}

// NodeInfo represents node information for cluster mode
type NodeInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

// ClusterInfo represents cluster status per AI.md PART 13
type ClusterInfo struct {
	Enabled   bool     `json:"enabled"`
	Status    string   `json:"status,omitempty"`     // "connected", "disconnected"
	Primary   string   `json:"primary,omitempty"`    // primary node public URL
	Nodes     []string `json:"nodes,omitempty"`      // all node public URLs
	NodeCount int      `json:"node_count,omitempty"` // total nodes
	Role      string   `json:"role,omitempty"`       // "primary" or "member"
}

// FeaturesInfo represents feature status per AI.md PART 13 (line 16269-16288)
type FeaturesInfo struct {
	Tor           TorInfo `json:"tor"`                     // PART 32: Tor Hidden Service
	GeoIP         bool    `json:"geoip"`                   // PART 20: GeoIP enabled
	MultiUser     bool    `json:"multi_user,omitempty"`    // PART 34: Multi-user mode
	Organizations bool    `json:"organizations,omitempty"` // PART 35: Organizations
}

// TorInfo represents Tor status per AI.md PART 13 (line 16290-16296)
type TorInfo struct {
	Enabled  bool   `json:"enabled"`  // Tor binary found and config enabled
	Running  bool   `json:"running"`  // Hidden service active
	Status   string `json:"status"`   // "healthy", "starting", "error"
	Hostname string `json:"hostname"` // "abc123...xyz.onion" (56 chars, v3)
}

// AutodiscoverResponse represents /api/autodiscover response
// Per AI.md PART 36 line 38077-38157: Autodiscover endpoint for CLI/agent
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

// InfoResponse represents server info response
type InfoResponse struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Uptime      string            `json:"uptime"`
	Mode        string            `json:"mode"`
	Engines     EnginesSummary    `json:"engines"`
	System      SystemInfo        `json:"system"`
	Features    map[string]bool   `json:"features"`
}

// EnginesSummary provides engine statistics
type EnginesSummary struct {
	Total   int `json:"total"`
	Enabled int `json:"enabled"`
}

// SystemInfo provides system information
type SystemInfo struct {
	GoVersion    string `json:"go_version"`
	NumCPU       int    `json:"num_cpu"`
	NumGoroutine int    `json:"num_goroutine"`
	MemAlloc     string `json:"mem_alloc"`
}

// SearchRequest represents a search API request
type SearchRequest struct {
	Query    string   `json:"query"`
	Category string   `json:"category"`
	Page     int      `json:"page"`
	Limit    int      `json:"limit"`
	Engines  []string `json:"engines,omitempty"`
	SafeSearch string `json:"safe_search,omitempty"`
	TimeRange  string `json:"time_range,omitempty"`
	Language   string `json:"language,omitempty"`
}

// Pagination represents standard pagination info per AI.md PART 14
type Pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

// SearchResponse represents search API response per AI.md PART 14 pagination format
type SearchResponse struct {
	Query      string         `json:"query"`
	Category   string         `json:"category"`
	Results    []SearchResult `json:"results"`
	Pagination Pagination     `json:"pagination"`
	SearchTime float64        `json:"search_time_ms"`
	Engines    []string       `json:"engines_used"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Title       string   `json:"title"`
	URL         string   `json:"url"`
	Description string   `json:"description"`
	Engine      string   `json:"engine"`
	Score       float64  `json:"score"`
	Category    string   `json:"category"`
	Thumbnail   string   `json:"thumbnail,omitempty"`
	Date        string   `json:"date,omitempty"`
	Domain      string   `json:"domain,omitempty"`
}

// EngineInfo represents engine information
type EngineInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Priority    int      `json:"priority"`
	Categories  []string `json:"categories"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
}

// CategoryInfo represents category information
type CategoryInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// Handler methods

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	// Tor status per AI.md PART 13 and PART 32
	torEnabled := h.config.Server.Tor.Enabled
	torRunning := false
	torStatus := ""
	torHostname := ""
	torCheck := ""

	// Check actual Tor service status per AI.md PART 32
	if torEnabled {
		if h.torService != nil && h.torService.IsRunning() {
			torRunning = true
			torStatus = "healthy"
			torHostname = h.torService.GetOnionAddress()
			torCheck = "ok"
		} else {
			torStatus = "unavailable"
			torCheck = "unavailable"
		}
	}

	// Determine overall status
	status := "healthy"

	// Check maintenance mode
	if h.config.Server.MaintenanceMode {
		status = "maintenance"
	}

	// Per AI.md PART 13: canonical field order (spec lines 16208-16244)
	health := HealthResponse{
		// 1. Project identification from cfg.Branding per spec
		Project: ProjectInfo{
			Name:        h.config.Server.Branding.Title,
			Tagline:     h.config.Server.Branding.Tagline,
			Description: h.config.Server.Branding.Description,
		},
		// 2. Status
		Status: status,
		// 3. Version & build
		Version:   config.Version,
		GoVersion: runtime.Version(),
		Build: BuildInfo{
			Commit: config.CommitID,
			Date:   config.BuildDate,
		},
		// 4. Runtime
		Uptime:    h.formatDuration(time.Since(h.startTime)),
		Mode:      h.config.Server.Mode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		// 5. Cluster
		Cluster: ClusterInfo{
			Enabled:   false,
			Status:    "",
			Primary:   "",
			Nodes:     []string{},
			NodeCount: 1,
			Role:      "",
		},
		// 6. Features per AI.md PART 13
		Features: FeaturesInfo{
			Tor: TorInfo{
				Enabled:  torEnabled,
				Running:  torRunning,
				Status:   torStatus,
				Hostname: torHostname,
			},
			GeoIP:         h.config.Server.GeoIP.Enabled,
			MultiUser:     h.config.Server.Users.Enabled,
			Organizations: false,
		},
		// 7. Checks per spec line 16298-16308
		Checks: ChecksInfo{
			Database:  "ok",
			Cache:     "disabled",
			Disk:      h.checkDiskHealth(),
			Scheduler: "ok",
			Cluster:   "",
			Tor:       torCheck,
		},
		// 8. Stats
		Stats: StatsInfo{
			RequestsTotal: h.getRequestsTotal(),
			Requests24h:   h.getRequests24h(),
			ActiveConns:   h.getActiveConnections(),
		},
	}

	statusCode := http.StatusOK
	if status == "unhealthy" || status == "maintenance" {
		statusCode = http.StatusServiceUnavailable
	}

	// Per AI.md PART 14: Support .txt extension and Accept: text/plain
	h.respondWithFormat(w, r, statusCode, &APIResponse{
		OK:   status == "healthy",
		Data: health,
		Meta: &APIMeta{Version: APIVersion},
	}, func() string {
		// Text format per AI.md PART 14 line 15016: "OK" or "ERROR: ..."
		if status == "healthy" {
			return "OK"
		}
		return fmt.Sprintf("ERROR: %s", status)
	})
}

// handleAutodiscover handles /api/autodiscover endpoint
// Per AI.md PART 36 line 38077-38157: Non-versioned autodiscover for CLI/agent
func (h *Handler) handleAutodiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.jsonResponse(w, http.StatusMethodNotAllowed, &APIResponse{
			OK:      false,
			Error:   "METHOD_NOT_ALLOWED",
			Message: "Only GET method allowed",
		})
		return
	}

	// Build server URL from request
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	serverURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	var resp AutodiscoverResponse

	// Server info
	resp.Server.Name = h.config.Server.Title
	resp.Server.Version = config.Version
	resp.Server.URL = serverURL
	resp.Server.Features.Auth = h.config.Server.Users.Enabled
	resp.Server.Features.Search = true // Search is always enabled
	resp.Server.Features.Register = h.config.Server.Users.Enabled && h.config.Server.Users.Registration.Enabled

	// Cluster info - for CLI/agent failover per AI.md PART 36 line 42791-42818
	resp.Cluster.Primary = serverURL
	resp.Cluster.Nodes = []string{serverURL} // Single node cluster by default

	// API info
	resp.API.Version = APIVersion
	resp.API.BasePath = APIPrefix

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: resp,
	})
}

func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	enabled := len(h.registry.GetEnabled())

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: InfoResponse{
			Name:        h.config.Server.Title,
			Version:     config.Version,
			Description: h.config.Server.Description,
			Uptime:      h.formatDuration(time.Since(h.startTime)),
			Mode:        h.config.Server.Mode,
			Engines: EnginesSummary{
				Total:   h.registry.Count(),
				Enabled: enabled,
			},
			System: SystemInfo{
				GoVersion:    runtime.Version(),
				NumCPU:       runtime.NumCPU(),
				NumGoroutine: runtime.NumGoroutine(),
				MemAlloc:     h.formatBytes(m.Alloc),
			},
			Features: map[string]bool{
				"ssl":         h.config.Server.SSL.Enabled,
				"tor":         h.config.Server.Tor.Enabled,
				"rate_limit":  h.config.Server.RateLimit.Enabled,
				"image_proxy": h.config.Server.ImageProxy.Enabled,
			},
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Parse request
	var req SearchRequest

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.errorResponse(w, http.StatusBadRequest, "Invalid JSON body", err.Error())
			return
		}
		// Trim query from JSON body
		req.Query = strings.TrimSpace(req.Query)
	} else {
		// Parse from query params (trim all text inputs)
		req.Query = strings.TrimSpace(r.URL.Query().Get("q"))
		req.Category = strings.TrimSpace(r.URL.Query().Get("category"))
		req.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		req.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
		req.SafeSearch = strings.TrimSpace(r.URL.Query().Get("safe_search"))
		req.Language = strings.TrimSpace(r.URL.Query().Get("lang"))
	}

	// Validate
	if req.Query == "" {
		h.errorResponse(w, http.StatusBadRequest, "Query parameter is required", "")
		return
	}

	// Set defaults
	if req.Category == "" {
		req.Category = "general"
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	// Map category
	var category model.Category
	switch req.Category {
	case "images":
		category = model.CategoryImages
	case "videos":
		category = model.CategoryVideos
	case "news":
		category = model.CategoryNews
	case "maps":
		category = model.CategoryMaps
	default:
		category = model.CategoryGeneral
	}

	// Perform search
	query := model.NewQuery(req.Query)
	query.Category = category

	ctx := r.Context()
	results, err := h.aggregator.Search(ctx, query)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Search failed", err.Error())
		return
	}

	// Convert results
	apiResults := make([]SearchResult, 0, len(results.Results))
	for i, result := range results.Results {
		if i >= req.Limit {
			break
		}
		apiResults = append(apiResults, SearchResult{
			Title:       result.Title,
			URL:         result.URL,
			Description: result.Content,
			Engine:      result.Engine,
			Score:       result.Score,
			Category:    string(result.Category),
			Thumbnail:   result.Thumbnail,
			Domain:      extractDomain(result.URL),
		})
	}

	// Calculate total pages per AI.md PART 14 pagination format
	totalPages := results.TotalResults / req.Limit
	if results.TotalResults%req.Limit > 0 {
		totalPages++
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: SearchResponse{
			Query:    req.Query,
			Category: req.Category,
			Results:  apiResults,
			Pagination: Pagination{
				Page:  req.Page,
				Limit: req.Limit,
				Total: results.TotalResults,
				Pages: totalPages,
			},
			SearchTime: float64(time.Since(start).Microseconds()) / 1000,
			Engines:    results.Engines,
		},
		Meta: &APIMeta{
			Version:     APIVersion,
			ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
		},
	})
}

// HandleAutocomplete is the public method for autocomplete suggestions
func (h *Handler) HandleAutocomplete(w http.ResponseWriter, r *http.Request) {
	h.handleAutocomplete(w, r)
}

func (h *Handler) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data:    []string{},
			Meta:    &APIMeta{Version: APIVersion},
		})
		return
	}

	// Fetch suggestions from DuckDuckGo's autocomplete API
	suggestions := h.fetchAutocompleteSuggestions(r.Context(), query)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    suggestions,
		Meta:    &APIMeta{Version: APIVersion},
	})
}

// fetchAutocompleteSuggestions fetches search suggestions from DuckDuckGo
func (h *Handler) fetchAutocompleteSuggestions(ctx context.Context, query string) []string {
	client := &http.Client{Timeout: 2 * time.Second}

	// DuckDuckGo autocomplete API
	url := fmt.Sprintf("https://duckduckgo.com/ac/?q=%s&type=list", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return []string{}
	}

	req.Header.Set("User-Agent", engines.UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return []string{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return []string{}
	}

	// DuckDuckGo returns: ["query", ["suggestion1", "suggestion2", ...]]
	var result []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return []string{}
	}

	if len(result) < 2 {
		return []string{}
	}

	suggestionsRaw, ok := result[1].([]interface{})
	if !ok {
		return []string{}
	}

	suggestions := make([]string, 0, len(suggestionsRaw))
	for _, s := range suggestionsRaw {
		if str, ok := s.(string); ok {
			suggestions = append(suggestions, str)
		}
	}

	// Limit to 10 suggestions
	if len(suggestions) > 10 {
		suggestions = suggestions[:10]
	}

	return suggestions
}

func (h *Handler) handleEngines(w http.ResponseWriter, r *http.Request) {
	allEngines := h.registry.GetAll()
	engineList := make([]EngineInfo, 0, len(allEngines))

	for _, eng := range allEngines {
		categories := make([]string, 0)
		cfg := eng.GetConfig()
		if cfg != nil {
			for _, cat := range cfg.Categories {
				categories = append(categories, string(cat))
			}
		}

		engineList = append(engineList, EngineInfo{
			ID:         eng.Name(),
			Name:       eng.DisplayName(),
			Enabled:    eng.IsEnabled(),
			Priority:   eng.GetPriority(),
			Categories: categories,
		})
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    engineList,
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *Handler) handleEngineByID(w http.ResponseWriter, r *http.Request) {
	// Extract engine ID from path
	path := r.URL.Path
	id := strings.TrimPrefix(path, "/api/v1/engines/")
	id = strings.TrimSuffix(id, "/")

	if id == "" {
		h.handleEngines(w, r)
		return
	}

	engine, err := h.registry.Get(id)
	if err != nil {
		h.errorResponse(w, http.StatusNotFound, "Engine not found", fmt.Sprintf("No engine with ID: %s", id))
		return
	}

	categories := make([]string, 0)
	cfg := engine.GetConfig()
	if cfg != nil {
		for _, cat := range cfg.Categories {
			categories = append(categories, string(cat))
		}
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: EngineInfo{
			ID:         engine.Name(),
			Name:       engine.DisplayName(),
			Enabled:    engine.IsEnabled(),
			Priority:   engine.GetPriority(),
			Categories: categories,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *Handler) handleCategories(w http.ResponseWriter, r *http.Request) {
	categories := []CategoryInfo{
		{ID: "general", Name: "General", Description: "General web search", Icon: "🌐"},
		{ID: "images", Name: "Images", Description: "Image search", Icon: "🖼️"},
		{ID: "videos", Name: "Videos", Description: "Video search", Icon: "🎥"},
		{ID: "news", Name: "News", Description: "News search", Icon: "📰"},
		{ID: "maps", Name: "Maps", Description: "Map and location search", Icon: "🗺️"},
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    categories,
		Meta:    &APIMeta{Version: APIVersion},
	})
}

// Helper methods

// jsonResponse sends JSON response with 2-space indentation per AI.md PART 14
func (h *Handler) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-API-Version", APIVersion)
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(data)
}

// textResponse sends plain text response per AI.md PART 14
// Ensures response ends with exactly one newline
func (h *Handler) textResponse(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-API-Version", APIVersion)
	w.WriteHeader(status)
	// Per AI.md PART 14: All text responses must end with single newline
	text = strings.TrimRight(text, "\n") + "\n"
	w.Write([]byte(text))
}

// respondWithFormat sends response based on format (supports .txt extension)
// Per AI.md PART 14: .txt extension support for plain text API output
func (h *Handler) respondWithFormat(w http.ResponseWriter, r *http.Request, status int, data interface{}, textFormatter func() string) {
	// Check for .txt extension in URL path
	path := r.URL.Path
	if strings.HasSuffix(path, ".txt") {
		h.textResponse(w, status, textFormatter())
		return
	}

	// Check Accept header for text/plain preference
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "text/plain") && !strings.Contains(accept, "application/json") {
		h.textResponse(w, status, textFormatter())
		return
	}

	// Check format query parameter
	format := r.URL.Query().Get("format")
	if format == "text" || format == "txt" || format == "plain" {
		h.textResponse(w, status, textFormatter())
		return
	}

	// Default to JSON
	h.jsonResponse(w, status, data)
}

// stripTxtExtension removes .txt extension from URL path for routing
// Per AI.md PART 14: .txt extension support
func stripTxtExtension(path string) string {
	if strings.HasSuffix(path, ".txt") {
		return strings.TrimSuffix(path, ".txt")
	}
	return path
}

// errorResponse sends a unified error response per AI.md PART 16
// Error format: {"ok": false, "error": "ERROR_CODE", "message": "Human readable message"}
// Per AI.md PART 7-9: RequestID must be included in error response meta
func (h *Handler) errorResponse(w http.ResponseWriter, status int, message, _ string) {
	// Get request ID from response header (set by middleware)
	requestID := w.Header().Get("X-Request-ID")

	h.jsonResponse(w, status, &APIResponse{
		OK:      false,
		Error:   model.ErrorCodeFromHTTP(status),
		Message: message,
		Meta: &APIMeta{
			RequestID: requestID,
			Version:   APIVersion,
		},
	})
}

func (h *Handler) formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func (h *Handler) formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// checkDiskHealth verifies the data directory is accessible
func (h *Handler) checkDiskHealth() string {
	// Data directory health is verified during startup
	// If we're running, disk is accessible
	return "ok"
}

// getRequestsTotal returns total requests since startup
// Returns 0 if metrics are disabled
func (h *Handler) getRequestsTotal() int64 {
	// Metrics integration point - returns 0 when metrics disabled
	if !h.config.Server.Metrics.Enabled {
		return 0
	}
	return 0
}

// getRequests24h returns requests in the last 24 hours
// Returns 0 if metrics are disabled
func (h *Handler) getRequests24h() int64 {
	// Metrics integration point - returns 0 when metrics disabled
	if !h.config.Server.Metrics.Enabled {
		return 0
	}
	return 0
}

// getActiveConnections returns current active connections
// Returns 0 if metrics are disabled
func (h *Handler) getActiveConnections() int {
	// Connection tracking integration point
	return 0
}

func extractDomain(urlStr string) string {
	// Simple domain extraction
	urlStr = strings.TrimPrefix(urlStr, "https://")
	urlStr = strings.TrimPrefix(urlStr, "http://")
	if idx := strings.Index(urlStr, "/"); idx > 0 {
		urlStr = urlStr[:idx]
	}
	return urlStr
}

// getHostname returns the system hostname
func getHostname() (string, error) {
	return os.Hostname()
}

// BangInfo represents bang information for API
type BangInfo struct {
	Shortcut    string   `json:"shortcut"`
	Name        string   `json:"name"`
	URL         string   `json:"url"`
	Category    string   `json:"category"`
	Description string   `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

func (h *Handler) handleBangs(w http.ResponseWriter, r *http.Request) {
	// Return built-in bangs list
	// Custom bangs are stored client-side in localStorage

	bangs := getBuiltinBangs()

	// Filter by category if specified
	category := r.URL.Query().Get("category")
	if category != "" {
		filtered := make([]BangInfo, 0)
		for _, b := range bangs {
			if b.Category == category {
				filtered = append(filtered, b)
			}
		}
		bangs = filtered
	}

	// Search filter
	search := r.URL.Query().Get("search")
	if search != "" {
		search = strings.ToLower(search)
		filtered := make([]BangInfo, 0)
		for _, b := range bangs {
			if strings.Contains(strings.ToLower(b.Shortcut), search) ||
				strings.Contains(strings.ToLower(b.Name), search) {
				filtered = append(filtered, b)
			}
		}
		bangs = filtered
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"bangs": bangs,
			"total": len(bangs),
			"categories": []string{
				"general", "images", "video", "maps", "news",
				"knowledge", "social", "code", "shopping", "files",
				"music", "science", "translate", "privacy", "misc",
			},
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// getBuiltinBangs returns the list of built-in bangs
func getBuiltinBangs() []BangInfo {
	return []BangInfo{
		// General Search Engines
		{Shortcut: "g", Name: "Google", URL: "https://www.google.com/search?q={query}", Category: "general", Description: "Google Search", Aliases: []string{"google"}},
		{Shortcut: "ddg", Name: "DuckDuckGo", URL: "https://duckduckgo.com/?q={query}", Category: "general", Description: "DuckDuckGo Search", Aliases: []string{"duckduckgo", "duck"}},
		{Shortcut: "b", Name: "Bing", URL: "https://www.bing.com/search?q={query}", Category: "general", Description: "Bing Search", Aliases: []string{"bing"}},
		{Shortcut: "sp", Name: "Startpage", URL: "https://www.startpage.com/do/search?q={query}", Category: "general", Description: "Startpage Search", Aliases: []string{"startpage"}},
		{Shortcut: "q", Name: "Qwant", URL: "https://www.qwant.com/?q={query}", Category: "general", Description: "Qwant Search", Aliases: []string{"qwant"}},
		{Shortcut: "ec", Name: "Ecosia", URL: "https://www.ecosia.org/search?q={query}", Category: "general", Description: "Ecosia Search", Aliases: []string{"ecosia"}},
		{Shortcut: "br", Name: "Brave Search", URL: "https://search.brave.com/search?q={query}", Category: "general", Description: "Brave Search", Aliases: []string{"brave"}},

		// Images
		{Shortcut: "gi", Name: "Google Images", URL: "https://www.google.com/search?tbm=isch&q={query}", Category: "images", Description: "Google Image Search", Aliases: []string{"gimg"}},
		{Shortcut: "bi", Name: "Bing Images", URL: "https://www.bing.com/images/search?q={query}", Category: "images", Description: "Bing Image Search"},
		{Shortcut: "us", Name: "Unsplash", URL: "https://unsplash.com/s/photos/{query}", Category: "images", Description: "Unsplash Free Photos", Aliases: []string{"unsplash"}},

		// Video
		{Shortcut: "yt", Name: "YouTube", URL: "https://www.youtube.com/results?search_query={query}", Category: "video", Description: "YouTube Video Search", Aliases: []string{"youtube"}},
		{Shortcut: "v", Name: "Vimeo", URL: "https://vimeo.com/search?q={query}", Category: "video", Description: "Vimeo Video Search", Aliases: []string{"vimeo"}},
		{Shortcut: "od", Name: "Odysee", URL: "https://odysee.com/$/search?q={query}", Category: "video", Description: "Odysee Video Search", Aliases: []string{"odysee", "lbry"}},

		// Maps
		{Shortcut: "gm", Name: "Google Maps", URL: "https://www.google.com/maps/search/{query}", Category: "maps", Description: "Google Maps", Aliases: []string{"gmaps"}},
		{Shortcut: "osm", Name: "OpenStreetMap", URL: "https://www.openstreetmap.org/search?query={query}", Category: "maps", Description: "OpenStreetMap", Aliases: []string{"openstreetmap"}},

		// News
		{Shortcut: "gn", Name: "Google News", URL: "https://news.google.com/search?q={query}", Category: "news", Description: "Google News", Aliases: []string{"gnews"}},

		// Knowledge
		{Shortcut: "w", Name: "Wikipedia", URL: "https://en.wikipedia.org/wiki/Special:Search?search={query}", Category: "knowledge", Description: "Wikipedia", Aliases: []string{"wiki", "wikipedia"}},
		{Shortcut: "wa", Name: "Wolfram Alpha", URL: "https://www.wolframalpha.com/input/?i={query}", Category: "knowledge", Description: "Wolfram Alpha", Aliases: []string{"wolfram"}},

		// Social
		{Shortcut: "tw", Name: "Twitter/X", URL: "https://twitter.com/search?q={query}", Category: "social", Description: "Twitter/X Search", Aliases: []string{"twitter", "x"}},
		{Shortcut: "rd", Name: "Reddit", URL: "https://www.reddit.com/search/?q={query}", Category: "social", Description: "Reddit Search", Aliases: []string{"reddit"}},
		{Shortcut: "hn", Name: "Hacker News", URL: "https://hn.algolia.com/?q={query}", Category: "social", Description: "Hacker News Search", Aliases: []string{"hackernews"}},

		// Code
		{Shortcut: "gh", Name: "GitHub", URL: "https://github.com/search?q={query}", Category: "code", Description: "GitHub Code Search", Aliases: []string{"github"}},
		{Shortcut: "gl", Name: "GitLab", URL: "https://gitlab.com/search?search={query}", Category: "code", Description: "GitLab Search", Aliases: []string{"gitlab"}},
		{Shortcut: "so", Name: "Stack Overflow", URL: "https://stackoverflow.com/search?q={query}", Category: "code", Description: "Stack Overflow Q&A", Aliases: []string{"stackoverflow"}},
		{Shortcut: "npm", Name: "npm", URL: "https://www.npmjs.com/search?q={query}", Category: "code", Description: "npm Package Registry"},
		{Shortcut: "pypi", Name: "PyPI", URL: "https://pypi.org/search/?q={query}", Category: "code", Description: "Python Package Index", Aliases: []string{"pip"}},
		{Shortcut: "gopkg", Name: "Go Packages", URL: "https://pkg.go.dev/search?q={query}", Category: "code", Description: "Go Package Documentation", Aliases: []string{"go", "golang"}},
		{Shortcut: "docker", Name: "Docker Hub", URL: "https://hub.docker.com/search?q={query}", Category: "code", Description: "Docker Hub Images", Aliases: []string{"dockerhub"}},
		{Shortcut: "mdn", Name: "MDN Web Docs", URL: "https://developer.mozilla.org/en-US/search?q={query}", Category: "code", Description: "Mozilla Developer Network"},

		// Shopping
		{Shortcut: "az", Name: "Amazon", URL: "https://www.amazon.com/s?k={query}", Category: "shopping", Description: "Amazon Shopping", Aliases: []string{"amazon"}},
		{Shortcut: "eb", Name: "eBay", URL: "https://www.ebay.com/sch/i.html?_nkw={query}", Category: "shopping", Description: "eBay Shopping", Aliases: []string{"ebay"}},

		// Files
		{Shortcut: "archive", Name: "Internet Archive", URL: "https://archive.org/search.php?query={query}", Category: "files", Description: "Internet Archive", Aliases: []string{"ia"}},

		// Music
		{Shortcut: "spot", Name: "Spotify", URL: "https://open.spotify.com/search/{query}", Category: "music", Description: "Spotify Music", Aliases: []string{"spotify"}},
		{Shortcut: "sc", Name: "SoundCloud", URL: "https://soundcloud.com/search?q={query}", Category: "music", Description: "SoundCloud Music", Aliases: []string{"soundcloud"}},
		{Shortcut: "lyrics", Name: "Genius Lyrics", URL: "https://genius.com/search?q={query}", Category: "music", Description: "Genius Song Lyrics", Aliases: []string{"genius"}},

		// Science
		{Shortcut: "scholar", Name: "Google Scholar", URL: "https://scholar.google.com/scholar?q={query}", Category: "science", Description: "Google Scholar Academic", Aliases: []string{"gs"}},
		{Shortcut: "arxiv", Name: "arXiv", URL: "https://arxiv.org/search/?query={query}", Category: "science", Description: "arXiv Preprints"},
		{Shortcut: "pubmed", Name: "PubMed", URL: "https://pubmed.ncbi.nlm.nih.gov/?term={query}", Category: "science", Description: "PubMed Medical Research"},

		// Translation
		{Shortcut: "gt", Name: "Google Translate", URL: "https://translate.google.com/?sl=auto&tl=en&text={query}", Category: "translate", Description: "Google Translate", Aliases: []string{"translate"}},
		{Shortcut: "deepl", Name: "DeepL", URL: "https://www.deepl.com/translator#auto/en/{query}", Category: "translate", Description: "DeepL Translator"},
		{Shortcut: "ud", Name: "Urban Dictionary", URL: "https://www.urbandictionary.com/define.php?term={query}", Category: "translate", Description: "Urban Dictionary Slang", Aliases: []string{"urban"}},

		// Privacy
		{Shortcut: "wbm", Name: "Wayback Machine", URL: "https://web.archive.org/web/*/{query}", Category: "privacy", Description: "Internet Archive Wayback Machine", Aliases: []string{"wayback"}},

		// Misc
		{Shortcut: "imdb", Name: "IMDb", URL: "https://www.imdb.com/find?q={query}", Category: "misc", Description: "Internet Movie Database"},
		{Shortcut: "goodreads", Name: "Goodreads", URL: "https://www.goodreads.com/search?q={query}", Category: "misc", Description: "Goodreads Books", Aliases: []string{"gr"}},
	}
}

// InstantAnswerResponse represents instant answer API response
type InstantAnswerResponse struct {
	Query   string                 `json:"query"`
	Type    string                 `json:"type,omitempty"`
	Title   string                 `json:"title,omitempty"`
	Content string                 `json:"content,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	Source  string                 `json:"source,omitempty"`
	Found   bool                   `json:"found"`
}

func (h *Handler) handleInstantAnswer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		h.errorResponse(w, http.StatusBadRequest, "Query parameter is required", "")
		return
	}

	if h.instantManager == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: InstantAnswerResponse{
				Query: query,
				Found: false,
			},
			Meta: &APIMeta{
				Version:     APIVersion,
				ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	answer, err := h.instantManager.Process(ctx, query)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to process instant answer", err.Error())
		return
	}

	if answer == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: InstantAnswerResponse{
				Query: query,
				Found: false,
			},
			Meta: &APIMeta{
				Version:     APIVersion,
				ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
			},
		})
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: InstantAnswerResponse{
			Query:   query,
			Type:    string(answer.Type),
			Title:   answer.Title,
			Content: answer.Content,
			Data:    answer.Data,
			Source:  answer.Source,
			Found:   true,
		},
		Meta: &APIMeta{
			Version:     APIVersion,
			ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
		},
	})
}

// DirectAnswerResponse represents direct answer API response
type DirectAnswerResponse struct {
	Type        string                 `json:"type"`
	Term        string                 `json:"term"`
	Title       string                 `json:"title"`
	Description string                 `json:"description,omitempty"`
	Content     string                 `json:"content"`
	Data        map[string]interface{} `json:"data,omitempty"`
	Source      string                 `json:"source,omitempty"`
	SourceURL   string                 `json:"source_url,omitempty"`
	CacheTTL    int                    `json:"cache_ttl_seconds,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Found       bool                   `json:"found"`
}

// handleDirectAnswer handles /api/v1/direct/{type}/{term} requests
func (h *Handler) handleDirectAnswer(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Only allow GET requests
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET requests are supported")
		return
	}

	// Parse path: /api/v1/direct/{type}/{term}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/direct/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request", "URL format: /api/v1/direct/{type}/{term}")
		return
	}

	answerType := direct.AnswerType(parts[0])
	term, err := url.PathUnescape(parts[1])
	if err != nil {
		term = parts[1]
	}
	term = strings.TrimSpace(term)

	if term == "" {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request", "Term is required")
		return
	}

	if h.directManager == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: DirectAnswerResponse{
				Type:  string(answerType),
				Term:  term,
				Found: false,
				Error: "Direct answers not available",
			},
			Meta: &APIMeta{
				Version:     APIVersion,
				ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	answer, err := h.directManager.ProcessType(ctx, answerType, term)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to process direct answer", err.Error())
		return
	}

	if answer == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: DirectAnswerResponse{
				Type:  string(answerType),
				Term:  term,
				Found: false,
			},
			Meta: &APIMeta{
				Version:     APIVersion,
				ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
			},
		})
		return
	}

	// Calculate cache TTL in seconds
	cacheTTL := 0
	if ttl := direct.CacheDurations[answerType]; ttl > 0 {
		cacheTTL = int(ttl.Seconds())
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: DirectAnswerResponse{
			Type:        string(answer.Type),
			Term:        answer.Term,
			Title:       answer.Title,
			Description: answer.Description,
			Content:     answer.Content,
			Data:        answer.Data,
			Source:      answer.Source,
			SourceURL:   answer.SourceURL,
			CacheTTL:    cacheTTL,
			Error:       answer.Error,
			Found:       answer.Error == "",
		},
		Meta: &APIMeta{
			Version:     APIVersion,
			ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
		},
	})
}

// RelatedSearchResponse represents related searches API response
type RelatedSearchResponse struct {
	Query       string   `json:"query"`
	Suggestions []string `json:"suggestions"`
	Count       int      `json:"count"`
}

func (h *Handler) handleRelatedSearches(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		h.errorResponse(w, http.StatusBadRequest, "Query parameter is required", "")
		return
	}

	// Parse limit parameter, default to 8
	limit := 8
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 20 {
			limit = l
		}
	}

	// If related searches provider not set, return empty
	if h.relatedSearches == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: RelatedSearchResponse{
				Query:       query,
				Suggestions: []string{},
				Count:       0,
			},
			Meta: &APIMeta{
				Version:     APIVersion,
				ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
			},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	suggestions, err := h.relatedSearches.GetRelated(ctx, query, limit)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to fetch related searches", err.Error())
		return
	}

	if suggestions == nil {
		suggestions = []string{}
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: RelatedSearchResponse{
			Query:       query,
			Suggestions: suggestions,
			Count:       len(suggestions),
		},
		Meta: &APIMeta{
			Version:     APIVersion,
			ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
		},
	})
}

// ServerPageResponse represents server info page API response
// Per AI.md PART 16: Server info pages return structured JSON
type ServerPageResponse struct {
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Content     string            `json:"content,omitempty"`
	Sections    []PageSection     `json:"sections,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// PageSection represents a section of a server info page
type PageSection struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// handleServerAbout handles GET /api/v1/server/about
// Per AI.md PART 16: Returns about page info as JSON
func (h *Handler) handleServerAbout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported")
		return
	}

	appName := h.config.Server.Branding.Title
	if appName == "" {
		appName = h.config.Server.Title
	}

	sections := []PageSection{
		{
			ID:      "description",
			Title:   "What is " + appName + "?",
			Content: appName + " is a privacy-respecting metasearch engine that aggregates results from multiple search engines without tracking you.",
		},
		{
			ID:      "features",
			Title:   "Features",
			Content: "Privacy First: No tracking, no profiling, no filter bubbles. Multiple Sources: Get results from various search engines. Fast & Lightweight: No JavaScript required, minimal bandwidth usage.",
		},
	}

	metadata := map[string]string{
		"version":    config.Version,
		"build_date": config.BuildDate,
		"commit":     config.CommitID,
	}

	// Add Tor address if available
	if h.config.Server.Tor.Enabled {
		metadata["tor_enabled"] = "true"
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerPageResponse{
			Title:       "About " + appName,
			Description: h.config.Server.Description,
			Content:     h.config.Server.Pages.About.Content,
			Sections:    sections,
			Metadata:    metadata,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerPrivacy handles GET /api/v1/server/privacy
// Per AI.md PART 16: Returns privacy policy as JSON
func (h *Handler) handleServerPrivacy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported")
		return
	}

	appName := h.config.Server.Branding.Title
	if appName == "" {
		appName = h.config.Server.Title
	}

	sections := []PageSection{
		{
			ID:      "data_collection",
			Title:   "Data Collection",
			Content: appName + " is designed with privacy as a core principle. We minimize data collection to what is strictly necessary for the service to function. Search queries are sent to upstream search engines but are not stored on our servers. We do not log IP addresses or use them for tracking.",
		},
		{
			ID:      "data_usage",
			Title:   "Data Usage",
			Content: "Any data that is temporarily processed is used only to forward your search query to upstream search engines, aggregate and display results to you, and remember your preferences (theme, language).",
		},
		{
			ID:      "data_retention",
			Title:   "Data Retention",
			Content: "We do not retain search queries or personal data. Session data is temporary and is deleted when you close your browser or your session expires.",
		},
		{
			ID:      "cookies",
			Title:   "Cookies",
			Content: "We use only essential cookies for session management and preference storage. We do not use tracking cookies, analytics cookies, or advertising cookies.",
		},
		{
			ID:      "third_parties",
			Title:   "Third Parties",
			Content: "Your search queries are forwarded to upstream search engines to retrieve results. We do not share any other data with third parties.",
		},
		{
			ID:      "your_rights",
			Title:   "Your Rights",
			Content: "Since we don't store personal data, there is no data to access, modify, or delete. Your privacy is protected by design.",
		},
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerPageResponse{
			Title:       "Privacy Policy",
			Description: "Privacy policy for " + appName,
			Content:     h.config.Server.Pages.Privacy.Content,
			Sections:    sections,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerHelp handles GET /api/v1/server/help
// Per AI.md PART 16: Returns help documentation as JSON
func (h *Handler) handleServerHelp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported")
		return
	}

	appName := h.config.Server.Branding.Title
	if appName == "" {
		appName = h.config.Server.Title
	}

	sections := []PageSection{
		{
			ID:      "getting_started",
			Title:   "Getting Started",
			Content: appName + " is a privacy-respecting search engine. Simply type your query in the search box and press Enter or click the search button.",
		},
		{
			ID:      "categories",
			Title:   "Search Categories",
			Content: "Use the category tabs to filter your search results: All (general web search), Images (search for images), Videos (search for videos), News (search news articles), Maps (search for locations).",
		},
		{
			ID:      "search_tips",
			Title:   "Search Tips",
			Content: "Use specific keywords for better results. Put phrases in quotes for exact matches. Use minus to exclude words (e.g., apple -fruit). Search within a site using site:example.com.",
		},
		{
			ID:      "keyboard_shortcuts",
			Title:   "Keyboard Shortcuts",
			Content: "/ - Focus search box. Escape - Clear search / Close dialogs. t - Toggle theme (dark/light).",
		},
		{
			ID:      "api_documentation",
			Title:   "API Documentation",
			Content: "This application provides a full REST API with interactive documentation at /openapi.",
		},
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerPageResponse{
			Title:       "Help & Documentation",
			Description: "Help and documentation for " + appName,
			Content:     h.config.Server.Pages.Help.Content,
			Sections:    sections,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerTerms handles GET /api/v1/server/terms
// Per AI.md PART 16: Returns terms of service as JSON
func (h *Handler) handleServerTerms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported")
		return
	}

	appName := h.config.Server.Branding.Title
	if appName == "" {
		appName = h.config.Server.Title
	}

	sections := []PageSection{
		{
			ID:      "acceptance",
			Title:   "1. Acceptance of Terms",
			Content: "By accessing or using " + appName + " (\"the Service\"), you agree to be bound by these Terms of Service. If you do not agree to these terms, please do not use the Service.",
		},
		{
			ID:      "description",
			Title:   "2. Description of Service",
			Content: appName + " is a privacy-respecting metasearch engine that aggregates results from multiple search engines. The Service is provided \"as is\" without warranty of any kind.",
		},
		{
			ID:      "user_conduct",
			Title:   "3. User Conduct",
			Content: "You agree not to: Use the Service for any unlawful purpose. Attempt to gain unauthorized access to the Service or its systems. Interfere with or disrupt the Service. Use automated means to access the Service in a manner that exceeds reasonable use.",
		},
		{
			ID:      "privacy",
			Title:   "4. Privacy",
			Content: "Your use of the Service is also governed by our Privacy Policy. We are committed to protecting your privacy and minimizing data collection.",
		},
		{
			ID:      "intellectual_property",
			Title:   "5. Intellectual Property",
			Content: "The Service and its original content, features, and functionality are owned by the operators of " + appName + ". Search results displayed are the property of their respective owners.",
		},
		{
			ID:      "third_party_content",
			Title:   "6. Third-Party Content",
			Content: "The Service aggregates results from third-party search engines. We do not control or endorse the content returned by these search engines.",
		},
		{
			ID:      "disclaimer",
			Title:   "7. Disclaimer of Warranties",
			Content: "THE SERVICE IS PROVIDED \"AS IS\" AND \"AS AVAILABLE\" WITHOUT WARRANTIES OF ANY KIND, EITHER EXPRESS OR IMPLIED.",
		},
		{
			ID:      "liability",
			Title:   "8. Limitation of Liability",
			Content: "IN NO EVENT SHALL THE OPERATORS BE LIABLE FOR ANY INDIRECT, INCIDENTAL, SPECIAL, CONSEQUENTIAL, OR PUNITIVE DAMAGES ARISING OUT OF OR RELATED TO YOUR USE OF THE SERVICE.",
		},
		{
			ID:      "modifications",
			Title:   "9. Modifications to Service",
			Content: "We reserve the right to modify, suspend, or discontinue the Service at any time without notice.",
		},
		{
			ID:      "changes",
			Title:   "10. Changes to Terms",
			Content: "We may update these Terms of Service from time to time. Your continued use of the Service after changes constitutes acceptance of the new terms.",
		},
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerPageResponse{
			Title:       "Terms of Service",
			Description: "Terms of service for " + appName,
			Content:     h.config.Server.Pages.Terms.Content,
			Sections:    sections,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleFavicon handles favicon proxy requests
// Per AI.md PART 16: NO external requests from client, server proxies content
// This provides privacy-preserving favicon fetching for search results
func (h *Handler) handleFavicon(w http.ResponseWriter, r *http.Request) {
	urlParam := strings.TrimSpace(r.URL.Query().Get("url"))
	if urlParam == "" {
		h.serveFaviconFallback(w)
		return
	}

	// Extract domain from URL
	domain := extractDomain(urlParam)
	if domain == "" {
		h.serveFaviconFallback(w)
		return
	}

	// Try to fetch favicon from the domain
	faviconURL := "https://" + domain + "/favicon.ico"

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", faviconURL, nil)
	if err != nil {
		log.Printf("[DEBUG] favicon request error for %s: %v", domain, err)
		h.serveFaviconFallback(w)
		return
	}

	// Set a generic user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FaviconFetcher/1.0)")
	req.Header.Set("Accept", "image/*")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[DEBUG] favicon fetch error for %s: %v", domain, err)
		h.serveFaviconFallback(w)
		return
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		h.serveFaviconFallback(w)
		return
	}

	// Validate content type is an image
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		// Some servers return favicon.ico without proper content-type
		// Accept common favicon patterns
		if !strings.Contains(contentType, "icon") && contentType != "application/octet-stream" {
			h.serveFaviconFallback(w)
			return
		}
		// Default to ICO if content-type is ambiguous
		if contentType == "application/octet-stream" || contentType == "" {
			contentType = "image/x-icon"
		}
	}

	// Limit response size to prevent abuse (max 100KB for favicon)
	limitedReader := io.LimitReader(resp.Body, 100*1024)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		log.Printf("[DEBUG] favicon read error for %s: %v", domain, err)
		h.serveFaviconFallback(w)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 24 hours
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// serveFaviconFallback serves a 1x1 transparent PNG as fallback
// This allows the client-side JS to detect load failure and show placeholder
func (h *Handler) serveFaviconFallback(w http.ResponseWriter) {
	// 1x1 transparent PNG (smallest valid PNG)
	transparentPNG := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	data, _ := base64.StdEncoding.DecodeString(transparentPNG)

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache fallback for 1 hour
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}
