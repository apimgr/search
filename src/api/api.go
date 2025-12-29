package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engines"
	"github.com/apimgr/search/src/widgets"
)

// API version
const (
	APIVersion = "v1"
	APIPrefix  = "/api/v1"
)

// Handler handles API requests
type Handler struct {
	config         *config.Config
	registry       *engines.Registry
	aggregator     *search.Aggregator
	widgetManager  *widgets.Manager
	instantManager *instant.Manager
	startTime      time.Time
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
func (h *Handler) SetWidgetManager(wm *widgets.Manager) {
	h.widgetManager = wm
}

// SetInstantManager sets the instant answer manager for the API handler
func (h *Handler) SetInstantManager(im *instant.Manager) {
	h.instantManager = im
}

// RegisterRoutes registers API routes
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Health and info
	mux.HandleFunc("/api/v1/healthz", h.handleHealthz)
	mux.HandleFunc("/api/v1/info", h.handleInfo)

	// Search
	mux.HandleFunc("/api/v1/search", h.handleSearch)
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
}

// Response types

// APIResponse is the base response structure
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
	Meta    *APIMeta    `json:"meta,omitempty"`
}

// APIError represents an API error
// Per AI.md PART 31: Standard error codes with HTTP status mapping
type APIError struct {
	Code    string `json:"code"`              // Standard error code (ERR_BAD_REQUEST, etc.)
	Status  int    `json:"status"`            // HTTP status code
	Message string `json:"message"`           // User-friendly error message
	Details string `json:"details,omitempty"` // Additional details (not in production)
}

// APIMeta contains response metadata
type APIMeta struct {
	RequestID   string  `json:"request_id,omitempty"`
	ProcessTime float64 `json:"process_time_ms,omitempty"`
	Version     string  `json:"version"`
}

// HealthResponse represents health check response per AI.md spec
type HealthResponse struct {
	Status         string            `json:"status"`
	Version        string            `json:"version"`
	Mode           string            `json:"mode"`
	Uptime         string            `json:"uptime"`
	Timestamp      string            `json:"timestamp"`
	Node           *NodeInfo         `json:"node,omitempty"`
	Cluster        *ClusterInfo      `json:"cluster,omitempty"`
	Checks         map[string]string `json:"checks"`
	PendingRestart bool              `json:"pending_restart,omitempty"`
	RestartReason  []string          `json:"restart_reason,omitempty"`
}

// NodeInfo represents node information for cluster mode
type NodeInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

// ClusterInfo represents cluster status
type ClusterInfo struct {
	Enabled bool   `json:"enabled"`
	Status  string `json:"status"`
	Nodes   int    `json:"nodes"`
	Role    string `json:"role,omitempty"`
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

// SearchResponse represents search API response
type SearchResponse struct {
	Query        string        `json:"query"`
	Category     string        `json:"category"`
	Results      []SearchResult `json:"results"`
	TotalResults int           `json:"total_results"`
	Page         int           `json:"page"`
	Limit        int           `json:"limit"`
	HasMore      bool          `json:"has_more"`
	SearchTime   float64       `json:"search_time_ms"`
	Engines      []string      `json:"engines_used"`
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
	hostname, _ := getHostname()

	// Build checks map
	checks := make(map[string]string)
	checks["search"] = "ok"

	// Determine overall status
	status := "healthy"

	// Check maintenance mode
	if h.config.Server.MaintenanceMode {
		status = "maintenance"
	}

	health := HealthResponse{
		Status:    status,
		Version:   config.Version,
		Mode:      h.config.Server.Mode,
		Uptime:    h.formatDuration(time.Since(h.startTime)),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Node: &NodeInfo{
			ID:       "standalone",
			Hostname: hostname,
		},
		Cluster: &ClusterInfo{
			Enabled: false,
			Status:  "disabled",
			Nodes:   1,
		},
		Checks: checks,
	}

	statusCode := http.StatusOK
	if status == "unhealthy" || status == "maintenance" {
		statusCode = http.StatusServiceUnavailable
	}

	h.jsonResponse(w, statusCode, &APIResponse{
		Success: status == "healthy",
		Data:    health,
		Meta:    &APIMeta{Version: APIVersion},
	})
}

func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	enabled := len(h.registry.GetEnabled())

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
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
	} else {
		// Parse from query params
		req.Query = r.URL.Query().Get("q")
		req.Category = r.URL.Query().Get("category")
		req.Page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		req.Limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
		req.SafeSearch = r.URL.Query().Get("safe_search")
		req.Language = r.URL.Query().Get("lang")
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
	var category models.Category
	switch req.Category {
	case "images":
		category = models.CategoryImages
	case "videos":
		category = models.CategoryVideos
	case "news":
		category = models.CategoryNews
	case "maps":
		category = models.CategoryMaps
	default:
		category = models.CategoryGeneral
	}

	// Perform search
	query := models.NewQuery(req.Query)
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

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data: SearchResponse{
			Query:        req.Query,
			Category:     req.Category,
			Results:      apiResults,
			TotalResults: results.TotalResults,
			Page:         req.Page,
			Limit:        req.Limit,
			HasMore:      len(results.Results) > req.Limit,
			SearchTime:   float64(time.Since(start).Microseconds()) / 1000,
			Engines:      results.Engines,
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
	query := r.URL.Query().Get("q")
	if query == "" {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			Success: true,
			Data:    []string{},
			Meta:    &APIMeta{Version: APIVersion},
		})
		return
	}

	// Fetch suggestions from DuckDuckGo's autocomplete API
	suggestions := h.fetchAutocompleteSuggestions(r.Context(), query)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
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

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0")

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
		Success: true,
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
		Success: true,
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
		Success: true,
		Data:    categories,
		Meta:    &APIMeta{Version: APIVersion},
	})
}

// Helper methods

func (h *Handler) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-API-Version", APIVersion)
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (h *Handler) errorResponse(w http.ResponseWriter, status int, message, details string) {
	h.jsonResponse(w, status, &APIResponse{
		Success: false,
		Error: &APIError{
			Code:    models.ErrorCodeFromHTTP(status),
			Status:  status,
			Message: message,
			Details: details,
		},
		Meta: &APIMeta{Version: APIVersion},
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
		Success: true,
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

	query := r.URL.Query().Get("q")
	if query == "" {
		h.errorResponse(w, http.StatusBadRequest, "Query parameter is required", "")
		return
	}

	if h.instantManager == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			Success: true,
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
			Success: true,
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
		Success: true,
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
