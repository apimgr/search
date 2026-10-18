package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/common/prefsync"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/geoip"
	"github.com/apimgr/search/src/i2p"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/notify"
	"github.com/apimgr/search/src/owner"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engine"
	"github.com/apimgr/search/src/service"
	"github.com/apimgr/search/src/version"
	"github.com/apimgr/search/src/widget"
	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
)

// APIVersion is the current API version. Derived from version.APIVersion.
const APIVersion = version.APIVersion

// APIPrefix is the versioned API base path (e.g. "/api/v1"). Derived from version.APIPrefix.
const APIPrefix = version.APIPrefix

// APIBasePath returns the versioned API base path (e.g. "/api/v1").
// Always use this or APIPrefix in code — never hardcode "v1" directly.
func APIBasePath() string {
	return version.APIPrefix
}

// HealthMetrics supplies live server counters for the public health response.
type HealthMetrics interface {
	GetTotalRequests() int64
	GetRequestsLast24Hours() int64
	GetActiveConnections() int64
}

type HealthSnapshot struct {
	Status         string
	PendingRestart bool
	RestartReason  []string
	Checks         ChecksInfo
}

type HealthProvider interface {
	HealthSnapshot() HealthSnapshot
}

type ContactMessage struct {
	Name     string
	Email    string
	Subject  string
	Message  string
	SourceIP string `json:"-"`
}

type ContactSubmitter interface {
	SubmitContact(context.Context, ContactMessage) error
}

// Handler handles API requests
type Handler struct {
	config          *config.Config
	registry        *engine.Registry
	aggregator      *search.Aggregator
	widgetManager   *widget.Manager
	instantManager  *instant.Manager
	directManager   *direct.Manager
	relatedSearches *search.RelatedSearches
	// Per AI.md PART 32: Tor service for health status
	torService *service.TorService
	// Per AI.md PART 31.2: I2P manager for health status
	i2pManager        *i2p.I2PManager
	geoipLookup       *geoip.Lookup
	startTime         time.Time
	healthMetrics     HealthMetrics
	healthProvider    HealthProvider
	contactSubmitter  ContactSubmitter
	alertManager      *alert.Manager
	ownerManager      *owner.Manager
	apiRoutes         http.Handler
	thumbnailRewriter func(string) string
	// scheduler backs the /server/scheduler/* endpoints per AI.md PART 18
	scheduler *scheduler.Scheduler
	// auditLogger writes the scheduler.task_manual_run audit event per PART 11
	auditLogger *logging.AuditLogger
	// validate is the input validator per AI.md PART 3 requirement
	validate           *validator.Validate
	autocompleteMu     sync.RWMutex
	autocompleteCache  map[string]autocompleteCacheEntry
	autocompleteClient *http.Client
}

type autocompleteCacheEntry struct {
	suggestions []string
	expiresAt   time.Time
}

const (
	maximumAutocompleteQueryBytes       = 2048
	maximumAutocompleteSuggestions      = 10
	maximumAutocompleteSuggestionLength = 200
	maximumAutocompleteResponseBytes    = 1 << 20
	maximumAutocompleteCacheEntries     = 500
)

// SetOwnerManager connects anonymous resource-token management to the server database.
func (h *Handler) SetOwnerManager(manager *owner.Manager) { h.ownerManager = manager }

// SetScheduler sets the scheduler for the API handler (PART 18 endpoints)
func (h *Handler) SetScheduler(s *scheduler.Scheduler) {
	h.scheduler = s
}

// SetHealthMetrics connects the server's live request counters to health responses.
func (h *Handler) SetHealthMetrics(metrics HealthMetrics) {
	h.healthMetrics = metrics
}

// SetHealthProvider shares the server's component checks with API health routes.
func (h *Handler) SetHealthProvider(provider HealthProvider) {
	h.healthProvider = provider
}

func (h *Handler) SetContactSubmitter(submitter ContactSubmitter) {
	h.contactSubmitter = submitter
}

// SetAuditLogger sets the audit logger for the API handler
func (h *Handler) SetAuditLogger(al *logging.AuditLogger) {
	h.auditLogger = al
}

// NewHandler creates a new API handler
func NewHandler(cfg *config.Config, registry *engine.Registry, aggregator *search.Aggregator) *Handler {
	return &Handler{
		config:            cfg,
		registry:          registry,
		aggregator:        aggregator,
		startTime:         time.Now(),
		validate:          validator.New(),
		autocompleteCache: make(map[string]autocompleteCacheEntry),
		autocompleteClient: &http.Client{
			Timeout: 2 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
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

// SetI2PManager sets the I2P manager for the API handler
// Per AI.md PART 31.2: eepsite status is checked via the manager, not hardcoded
func (h *Handler) SetI2PManager(im *i2p.I2PManager) {
	h.i2pManager = im
}

func (h *Handler) SetAlertManager(am *alert.Manager) {
	h.alertManager = am
}

// SetThumbnailRewriter applies the server's privacy-preserving image proxy to
// thumbnails returned by the JSON search API.
func (h *Handler) SetThumbnailRewriter(rewrite func(string) string) {
	h.thumbnailRewriter = rewrite
}

// RewriteThumbnail returns a privacy-preserving URL when enabled by the live
// server configuration, and the source URL otherwise.
func (h *Handler) RewriteThumbnail(thumbnail string) string {
	if h.thumbnailRewriter == nil || thumbnail == "" {
		return thumbnail
	}
	return h.thumbnailRewriter(thumbnail)
}

// SetGeoIPLookup sets the GeoIP lookup service for the API handler.
// Instant answer handlers use it to enrich IP responses with geo data.
func (h *Handler) SetGeoIPLookup(g *geoip.Lookup) {
	h.geoipLookup = g
}

// RegisterRoutes registers API routes
func (h *Handler) RegisterRoutes(r chi.Router) {
	h.apiRoutes = r
	// Autodiscover - non-versioned per AI.md PART 32 line 38077-38157
	r.Get("/api/autodiscover", h.handleAutodiscover)

	// Health and info - Per AI.md PART 13/14
	// Canonical API route: /api/v1/server/healthz (JSON default, text via .txt or Accept header)
	// Unversioned operational alias per PART 14 mounts the same handler.
	r.Get(APIPrefix+"/server/healthz", h.handleHealthz)
	r.Get(APIPrefix+"/server/healthz.txt", h.handleHealthz)
	r.Get("/api/healthz", h.handleHealthz)
	r.Get(APIPrefix+"/info", h.handleInfo)
	// Per AI.md PART 14
	r.Get(APIPrefix+"/info.txt", h.handleInfo)

	// Search
	r.Get(APIPrefix+"/search", h.handleSearch)
	r.Post(APIPrefix+"/search", h.handleSearch)
	r.Get(APIPrefix+"/search/related", h.handleRelatedSearches)
	r.Get(APIPrefix+"/autocomplete", h.handleAutocomplete)

	// Engines
	r.Get(APIPrefix+"/engines", h.handleEngines)
	r.Get(APIPrefix+"/engines/*", h.handleEngineByID)

	// Categories
	r.Get(APIPrefix+"/categories", h.handleCategories)

	// Bangs
	r.Get(APIPrefix+"/bangs", h.handleBangs)

	// Widgets
	r.Get(APIPrefix+"/widgets", h.handleWidgets)
	r.Get(APIPrefix+"/widgets/*", h.handleWidgetData)

	// Instant Answers
	r.Get(APIPrefix+"/instant", h.handleInstantAnswer)

	// Direct Answers (full-page results per IDEA.md)
	r.Get(APIPrefix+"/direct/*", h.handleDirectAnswer)

	// Server info pages (per AI.md PART 16)
	r.Get(APIPrefix+"/server/about", h.handleServerAbout)
	r.Get(APIPrefix+"/server/privacy", h.handleServerPrivacy)
	r.Get(APIPrefix+"/server/help", h.handleServerHelp)
	r.Get(APIPrefix+"/server/terms", h.handleServerTerms)
	r.Get(APIPrefix+"/server/contact", h.handleServerContact)
	r.Post(APIPrefix+"/server/contact", h.handleServerContact)
	r.Get(APIPrefix+"/server/preferences", h.handlePreferences)
	r.Post(APIPrefix+"/server/preferences", h.handlePreferences)
	r.Get(APIPrefix+"/server/preferences/export", h.handleServerPreferencesExport)
	r.Get(APIPrefix+"/server/preferences/import", h.handleServerPreferencesImport)
	r.Post(APIPrefix+"/server/mcp", h.handleMCP)
	r.Post("/api/mcp", h.handleMCP)
	r.Post("/mcp", h.handleMCP)
	r.Get(APIPrefix+"/server/preferences/tokens", h.ownerTokensList)
	r.Post(APIPrefix+"/server/preferences/tokens/primary/reveal", h.ownerPrimaryReveal)
	r.Post(APIPrefix+"/server/preferences/tokens/primary/rotate", h.ownerPrimaryRotate)
	r.Post(APIPrefix+"/server/preferences/tokens", h.ownerLinkedCreate)
	r.Post(APIPrefix+"/server/preferences/tokens/{id}/reveal", h.ownerLinkedReveal)
	r.Post(APIPrefix+"/server/preferences/tokens/{id}/rotate", h.ownerLinkedRotate)
	r.Patch(APIPrefix+"/server/preferences/tokens/{id}", h.ownerLinkedUpdate)
	r.Delete(APIPrefix+"/server/preferences/tokens/{id}", h.ownerLinkedRevoke)

	// Favicon proxy - privacy-preserving favicon fetching
	// Per AI.md PART 16: NO external requests from client, server proxies content
	r.Get(APIPrefix+"/favicon", h.handleFavicon)
	r.Get(APIPrefix+"/alerts", h.handleAlerts)
	r.Post(APIPrefix+"/alerts", h.handleAlerts)
	r.Get(APIPrefix+"/alerts/{token}", h.handleAlertByToken)
	r.Patch(APIPrefix+"/alerts/{token}", h.handleAlertByToken)
	r.Delete(APIPrefix+"/alerts/{token}", h.handleAlertByToken)
	r.Post(APIPrefix+"/alerts/{token}/pause", h.handleAlertByToken)
	r.Get(APIPrefix+"/alerts/{token}/rss", h.handleAlertByToken)

	// Operator-gated server status and config — per AI.md PART 14
	r.Get(APIPrefix+"/server/status", h.requireOperator(h.handleServerStatus))
	r.Get(APIPrefix+"/server/config", h.requireOperator(h.handleServerConfig))
	r.Post(APIPrefix+"/server/config/contact/webhooks", h.requireOperator(h.handleContactWebhookConfigSave))

	// GeoIP lookup and operator-authored country-list presets — per AI.md PART 19.
	// Lookup is public (it only exposes the requester's own or an explicitly
	// supplied address); presets live in server.yml and are operator-gated.
	r.Get(APIPrefix+"/server/geoip/ips", h.handleGeoIPLookup)
	r.Get(APIPrefix+"/server/geoip/ips/{ip}", h.handleGeoIPLookup)
	r.Get(APIPrefix+"/server/geoip/presets", h.requireOperator(h.handleGeoIPPresets))
	r.Post(APIPrefix+"/server/geoip/presets", h.requireOperator(h.handleGeoIPPresetSave))
	r.Get(APIPrefix+"/server/geoip/presets/{name}", h.requireOperator(h.handleGeoIPPreset))
	r.Delete(APIPrefix+"/server/geoip/presets/{name}", h.requireOperator(h.handleGeoIPPresetDelete))

	// Scheduler — per AI.md PART 18: the scheduler is always running, task
	// visibility and external triggering go through these endpoints (operator
	// gated; manual runs are audit events per the PART 11 audit table).
	r.Get(APIPrefix+"/server/scheduler/tasks", h.requireOperator(h.handleSchedulerTasks))
	r.Get(APIPrefix+"/server/scheduler/tasks/{task}", h.requireOperator(h.handleSchedulerTask))
	r.Get(APIPrefix+"/server/scheduler/tasks/{task}/history", h.requireOperator(h.handleSchedulerTaskHistory))
	r.Post(APIPrefix+"/server/scheduler/tasks/{task}/run", h.requireOperator(h.handleSchedulerTaskRun))
	r.Post(APIPrefix+"/server/scheduler/tasks/{task}/enable", h.requireOperator(h.handleSchedulerTaskEnable))
	r.Post(APIPrefix+"/server/scheduler/tasks/{task}/disable", h.requireOperator(h.handleSchedulerTaskDisable))
}

// handleSchedulerTasks handles GET /api/v1/server/scheduler/tasks.
// Returns all registered tasks with their last/next run status per docs/api.md.
func (h *Handler) handleSchedulerTasks(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running")
		return
	}
	tasks := h.scheduler.GetTasks()
	if tasks == nil {
		tasks = []*scheduler.TaskInfo{}
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"tasks": tasks}})
}

// handleSchedulerTask returns details for one scheduled task.
func (h *Handler) handleSchedulerTask(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running")
		return
	}
	task, err := h.scheduler.GetTask(scheduler.TaskID(chi.URLParam(r, "task")))
	if err != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: task})
}

// handleSchedulerTaskHistory returns recent persisted runs for one task.
func (h *Handler) handleSchedulerTaskHistory(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running")
		return
	}
	taskID := scheduler.TaskID(chi.URLParam(r, "task"))
	if _, err := h.scheduler.GetTask(taskID); err != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	history, err := h.scheduler.GetHistory(taskID, 20)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "HISTORY_UNAVAILABLE", err.Error())
		return
	}
	if history == nil {
		history = []*scheduler.TaskHistoryEntry{}
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"history": history}})
}

func (h *Handler) handleSchedulerTaskEnable(w http.ResponseWriter, r *http.Request) {
	h.setSchedulerTaskEnabled(w, r, true)
}

func (h *Handler) handleSchedulerTaskDisable(w http.ResponseWriter, r *http.Request) {
	h.setSchedulerTaskEnabled(w, r, false)
}

func (h *Handler) setSchedulerTaskEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running")
		return
	}
	taskID := scheduler.TaskID(chi.URLParam(r, "task"))
	var err error
	if enabled {
		err = h.scheduler.Enable(taskID)
	} else {
		err = h.scheduler.Disable(taskID)
	}
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "TASK_STATE_REJECTED", err.Error())
		return
	}
	task, err := h.scheduler.GetTask(taskID)
	if err != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: task})
}

// handleSchedulerTaskRun handles POST /api/v1/server/scheduler/tasks/{task}/run.
// Triggers an immediate run of the named task, per docs/api.md.
func (h *Handler) handleSchedulerTaskRun(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task")
	if taskID == "" {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "missing task id")
		return
	}
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running")
		return
	}
	if err := h.scheduler.RunNow(scheduler.TaskID(taskID)); err != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("task %q cannot be run: %v", taskID, err))
		return
	}
	// Per AI.md PART 11 audit table: a manual run is audit event
	// scheduler.task_manual_run. Operator-triggered over loopback/API with a
	// bearer token, so the actor is the operator token subject ("-": privacy
	// per PART 11, no client IP is logged).
	if al := h.auditLogger; al != nil {
		al.LogSchedulerTaskManualRun("operator", "-", taskID, true)
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"task": taskID, "triggered": true}})
}

// Response types

// APIResponse is the base response structure
// Per AI.md PART 16: Unified Response Format (NON-NEGOTIABLE)
// Success: {"ok": true, "data": {...}}
// Error: {"ok": false, "error": "ERROR_CODE", "message": "Human readable message"}
type APIResponse struct {
	OK   bool        `json:"ok"`
	Data interface{} `json:"data,omitempty"`
	// Error code (e.g., "BAD_REQUEST", "NOT_FOUND")
	Error string `json:"error,omitempty"`
	// Human-readable error message
	Message string `json:"message,omitempty"`
	// Optional structured error context (e.g., {"field":"email","rule":"format"}); omitted when empty per AI.md PART 14
	Details map[string]interface{} `json:"details,omitempty"`
	// Pagination is a sibling of data (never nested inside it) per AI.md PART 14
	Pagination *Pagination `json:"pagination,omitempty"`
	// Optional metadata (request_id, process_time_ms)
	Meta *APIMeta `json:"meta,omitempty"`
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
	// "healthy", "unhealthy", "degraded"
	Status string `json:"status"`
	// true if restart needed
	PendingRestart bool `json:"pending_restart,omitempty"`
	// settings that changed
	RestartReason []string `json:"restart_reason,omitempty"`

	// 3. Version & build info (PART 7: binary requirements)
	// SemVer "1.0.0"
	Version string `json:"version"`
	// "go1.23.0"
	GoVersion string    `json:"go_version"`
	Build     BuildInfo `json:"build"`

	// 4. Runtime info (PART 6: application modes)
	// human readable "2d 5h 30m"
	Uptime string `json:"uptime"`
	// "production" or "development"
	Mode string `json:"mode"`
	// current UTC time ISO 8601
	Timestamp string `json:"timestamp"`

	// 5. Features - PUBLIC only (PARTS 20, 32)
	Features FeaturesInfo `json:"features"`

	// 6. Component health checks (per AI.md PART 13)
	Checks ChecksInfo `json:"checks"`

	// 7. Statistics (public-safe aggregates)
	Stats StatsInfo `json:"stats"`
}

// ProjectInfo represents project identification per AI.md PART 13
type ProjectInfo struct {
	// branding.app_name
	Name string `json:"name"`
	// branding.tagline (short slogan)
	Tagline string `json:"tagline"`
	// server.description (longer)
	Description string `json:"description"`
}

// BuildInfo represents build information per AI.md PART 13
// Note: Fields are "commit" and "date" per spec, not "commit_id" and "build_date"
type BuildInfo struct {
	// git short hash (7 chars)
	Commit string `json:"commit"`
	// ISO 8601 build timestamp
	Date string `json:"date"`
}

// StatsInfo represents health statistics per AI.md PART 13 (line 16310-16317)
type StatsInfo struct {
	// Total HTTP requests (lifetime)
	RequestsTotal int64 `json:"requests_total"`
	// Requests in last 24 hours
	Requests24h int64 `json:"requests_24h"`
	// Current active connections
	ActiveConns int `json:"active_connections"`
}

// ChecksInfo represents component health per AI.md PART 13
type ChecksInfo struct {
	// PART 10: "ok" or "error"
	Database string `json:"database"`
	// PART 10: "ok" or "error"
	Cache string `json:"cache"`
	// Disk space check
	Disk string `json:"disk"`
	// PART 18: "ok" or "error"
	Scheduler string `json:"scheduler"`
	// PART 31: "ok" or "error" (if enabled)
	Tor string `json:"tor,omitempty"`
	// PART 31.2: "ok" or "error" (if enabled)
	I2P string `json:"i2p,omitempty"`
}

// FeaturesInfo represents PUBLIC feature status per AI.md PART 13.
// Only non-optional features are listed; optional features absent until implemented.
type FeaturesInfo struct {
	// PART 32: Tor Hidden Service
	Tor TorInfo `json:"tor"`
	// PART 31.2: I2P eepsite
	I2P I2PInfo `json:"i2p"`
	// PART 20: GeoIP enabled
	GeoIP bool `json:"geoip"`
}

// TorInfo represents Tor status per AI.md PART 13 (line 16290-16296)
type TorInfo struct {
	// Tor binary found and config enabled
	Enabled bool `json:"enabled"`
	// Hidden service active
	Running bool `json:"running"`
	// "healthy", "starting", "error"
	Status string `json:"status"`
	// "abc123...xyz.onion" (56 chars, v3)
	Hostname string `json:"hostname"`
}

// I2PInfo represents I2P eepsite status per AI.md PART 13 (line 17428-17438)
type I2PInfo struct {
	// I2P enabled in config
	Enabled bool `json:"enabled"`
	// Eepsite tunnel active
	Running bool `json:"running"`
	// "healthy", "disabled", "starting", "error"
	Status string `json:"status"`
	// ".b32.i2p" eepsite address
	Hostname string `json:"hostname"`
	// "i2pd", "sam", or "none"
	Provider string `json:"provider"`
}

// AutodiscoverResponse represents /api/autodiscover response
// Per AI.md PART 32: Autodiscover endpoint for CLI/agent
// CLIBinaryInfo holds the version and SHA-256 checksum for a CLI binary platform.
// Per AI.md PART 32: autodiscover response must include cli_versions per platform.
type CLIBinaryInfo struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

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
	// Per AI.md PART 32: CLI version info for auto-update checks.
	// Keys are "{os}-{arch}" e.g. "linux-amd64".
	CLIVersions   map[string]CLIBinaryInfo `json:"cli_versions"`
	CLIMinVersion string                   `json:"cli_min_version"`
}

// InfoResponse represents server info response
type InfoResponse struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Uptime      string          `json:"uptime"`
	Mode        string          `json:"mode"`
	Engines     EnginesSummary  `json:"engines"`
	System      SystemInfo      `json:"system"`
	Features    map[string]bool `json:"features"`
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
	Query      string   `json:"query"                    validate:"required,min=1,max=500"`
	Category   string   `json:"category"                 validate:"omitempty,max=50"`
	Page       int      `json:"page"                     validate:"omitempty,min=1,max=1000"`
	Limit      int      `json:"limit"                    validate:"omitempty,min=1,max=100"`
	Engines    []string `json:"engines,omitempty"        validate:"omitempty,max=50,dive,min=1,max=50"`
	SafeSearch *int     `json:"safe_search,omitempty"   validate:"omitempty,min=0,max=2"`
	TimeRange  string   `json:"time_range,omitempty"     validate:"omitempty,oneof=any hour day week month year custom"`
	TimeFrom   string   `json:"time_from,omitempty"      validate:"omitempty,len=10"`
	TimeTo     string   `json:"time_to,omitempty"        validate:"omitempty,len=10"`
	Language   string   `json:"language,omitempty"       validate:"omitempty,max=10"`
	Region     string   `json:"region,omitempty"         validate:"omitempty,max=10"`
}

// Pagination represents standard pagination info per AI.md PART 14
type Pagination struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
	Total int `json:"total"`
	Pages int `json:"pages"`
}

// SearchResponse represents the search API `data` payload.
// Pagination is carried as a sibling of `data` on APIResponse per AI.md PART 14.
type SearchResponse struct {
	Query      string         `json:"query"`
	Category   string         `json:"category"`
	Results    []SearchResult `json:"results"`
	SearchTime float64        `json:"search_time_ms"`
	Engines    []string       `json:"engines_used"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Description string  `json:"description"`
	Engine      string  `json:"engine"`
	Score       float64 `json:"score"`
	Category    string  `json:"category"`
	Thumbnail   string  `json:"thumbnail,omitempty"`
	ImageURL    string  `json:"image_url,omitempty"`
	ImageWidth  int     `json:"image_width,omitempty"`
	ImageHeight int     `json:"image_height,omitempty"`
	ImageFormat string  `json:"image_format,omitempty"`
	FileSize    int64   `json:"file_size,omitempty"`
	Duration    int     `json:"duration,omitempty"`
	ViewCount   int64   `json:"view_count,omitempty"`
	Author      string  `json:"author,omitempty"`
	EmbedURL    string  `json:"embed_url,omitempty"`
	Date        string  `json:"date,omitempty"`
	Domain      string  `json:"domain,omitempty"`
}

// EngineInfo represents engine information
type EngineInfo struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Enabled     bool                 `json:"enabled"`
	Priority    int                  `json:"priority"`
	Categories  []string             `json:"categories"`
	Description string               `json:"description,omitempty"`
	Homepage    string               `json:"homepage,omitempty"`
	Health      *search.EngineHealth `json:"health,omitempty"`
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
	health := h.buildHealthResponse()
	statusCode := http.StatusOK
	if health.Status == "unhealthy" || health.Status == "maintenance" || health.Status == "shutting_down" {
		statusCode = http.StatusServiceUnavailable
	}
	// Health probes require the bare canonical health payload in JSON and the
	// same complete field set in text format; the normal API envelope is an
	// intentional exception for these routes.
	if strings.HasSuffix(r.URL.Path, ".txt") ||
		(strings.Contains(r.Header.Get("Accept"), "text/plain") && !strings.Contains(r.Header.Get("Accept"), "application/json")) ||
		(r.URL.Query().Get("format") == "text" || r.URL.Query().Get("format") == "txt" || r.URL.Query().Get("format") == "plain") {
		h.textResponse(w, statusCode, formatHealthText(health))
		return
	}
	h.jsonResponse(w, statusCode, health)
}

func formatHealthText(health HealthResponse) string {
	var text strings.Builder
	fmt.Fprintf(&text, "project.name: %s\nproject.tagline: %s\nproject.description: %s\nstatus: %s\n", health.Project.Name, health.Project.Tagline, health.Project.Description, health.Status)
	if health.PendingRestart {
		fmt.Fprintln(&text, "pending_restart: true")
		for _, reason := range health.RestartReason {
			fmt.Fprintf(&text, "restart_reason: %s\n", reason)
		}
	}
	fmt.Fprintf(&text, "version: %s\ngo_version: %s\nbuild.commit: %s\nbuild.date: %s\nuptime: %s\nmode: %s\ntimestamp: %s\nfeatures.tor.enabled: %t\nfeatures.tor.running: %t\nfeatures.tor.status: %s\nfeatures.tor.hostname: %s\nfeatures.i2p.enabled: %t\nfeatures.i2p.running: %t\nfeatures.i2p.status: %s\nfeatures.i2p.hostname: %s\nfeatures.i2p.provider: %s\nfeatures.geoip: %t\nchecks.database: %s\nchecks.cache: %s\nchecks.disk: %s\nchecks.scheduler: %s\n",
		health.Version, health.GoVersion, health.Build.Commit, health.Build.Date, health.Uptime,
		health.Mode, health.Timestamp, health.Features.Tor.Enabled, health.Features.Tor.Running,
		health.Features.Tor.Status, health.Features.Tor.Hostname, health.Features.I2P.Enabled,
		health.Features.I2P.Running, health.Features.I2P.Status, health.Features.I2P.Hostname,
		health.Features.I2P.Provider, health.Features.GeoIP, health.Checks.Database,
		health.Checks.Cache, health.Checks.Disk, health.Checks.Scheduler)
	if health.Checks.Tor != "" {
		fmt.Fprintf(&text, "checks.tor: %s\n", health.Checks.Tor)
	}
	if health.Checks.I2P != "" {
		fmt.Fprintf(&text, "checks.i2p: %s\n", health.Checks.I2P)
	}
	fmt.Fprintf(&text, "stats.requests_total: %d\nstats.requests_24h: %d\nstats.active_connections: %d\n", health.Stats.RequestsTotal, health.Stats.Requests24h, health.Stats.ActiveConns)
	return text.String()
}

// HealthForGraphQL returns the same public health payload as the REST endpoint.
func (h *Handler) HealthForGraphQL() HealthResponse {
	return h.buildHealthResponse()
}

func (h *Handler) buildHealthResponse() HealthResponse {
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

	// I2P status per AI.md PART 13 and PART 31.2
	i2pInfo := I2PInfo{
		Enabled:  h.config.Server.I2P.Enabled,
		Running:  false,
		Status:   "disabled",
		Hostname: "",
		Provider: "none",
	}
	i2pCheck := ""
	if h.config.Server.I2P.Enabled {
		i2pInfo.Status = "error"
		i2pCheck = "error"
		if h.i2pManager != nil && h.i2pManager.IsRunning() {
			i2pInfo.Running = true
			i2pInfo.Status = "healthy"
			i2pInfo.Hostname = h.i2pManager.EepsiteAddress()
			i2pInfo.Provider = h.i2pManager.ProviderName()
			i2pCheck = "ok"
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
		// 5. Features per AI.md PART 13
		Features: FeaturesInfo{
			Tor: TorInfo{
				Enabled:  torEnabled,
				Running:  torRunning,
				Status:   torStatus,
				Hostname: torHostname,
			},
			I2P:   i2pInfo,
			GeoIP: h.config.Server.GeoIP.Enabled,
		},
		// 6. Checks per AI.md PART 13
		Checks: ChecksInfo{
			Database:  "ok",
			Cache:     h.checkCacheHealth(),
			Disk:      h.checkDiskHealth(),
			Scheduler: h.checkSchedulerHealth(),
			Tor:       torCheck,
			I2P:       i2pCheck,
		},
		// 7. Stats
		Stats: StatsInfo{
			RequestsTotal: h.getRequestsTotal(),
			Requests24h:   h.getRequests24h(),
			ActiveConns:   h.getActiveConnections(),
		},
	}

	if h.healthProvider != nil {
		snapshot := h.healthProvider.HealthSnapshot()
		health.Status = snapshot.Status
		health.PendingRestart = snapshot.PendingRestart
		health.RestartReason = snapshot.RestartReason
		health.Checks = snapshot.Checks
	} else if health.Status != "maintenance" && h.config.PendingRestart() {
		health.Status = "restart_required"
		health.PendingRestart = true
		health.RestartReason = h.config.RestartSettings()
	}

	return health
}

// handleAutodiscover handles /api/autodiscover endpoint
// Per AI.md PART 32 line 38077-38157: Non-versioned autodiscover for CLI/agent
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
	// PART 32 is the CLIENT spec. Regular-user accounts are not part of this project.
	resp.Server.Features.Auth = false
	resp.Server.Features.Search = true
	resp.Server.Features.Register = false

	// API info
	resp.API.Version = APIVersion
	resp.API.BasePath = APIPrefix

	// Per AI.md PART 32: include CLI version info for auto-update checks.
	// The map is populated when CLI binaries are published alongside the server release.
	// An empty map signals "no CLI binary available via this server" (CLI uses GitHub Releases).
	resp.CLIVersions = map[string]CLIBinaryInfo{}
	resp.CLIMinVersion = ""

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: resp,
	})
}

func (h *Handler) Info() InfoResponse {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	enabled := 0
	total := 0
	if h.registry != nil {
		enabled = len(h.registry.GetEnabled())
		total = h.registry.Count()
	}
	return InfoResponse{
		Name: h.config.Server.Title, Version: config.Version,
		Description: h.config.Server.Description,
		Uptime:      h.formatDuration(time.Since(h.startTime)), Mode: h.config.Server.Mode,
		Engines: EnginesSummary{Total: total, Enabled: enabled},
		System:  SystemInfo{GoVersion: runtime.Version(), NumCPU: runtime.NumCPU(), NumGoroutine: runtime.NumGoroutine(), MemAlloc: h.formatBytes(memory.Alloc)},
		Features: map[string]bool{
			"ssl": h.config.Server.SSL.Enabled, "tor": h.config.Server.Tor.Enabled,
			"rate_limit": h.config.Server.RateLimit.Enabled, "image_proxy": h.config.Server.ImageProxy.Enabled,
		},
	}
}

func (h *Handler) handleInfo(w http.ResponseWriter, _ *http.Request) {
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true, Data: h.Info(), Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Parse request
	var req SearchRequest

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
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
		if safeSearch := strings.TrimSpace(r.URL.Query().Get("safe_search")); safeSearch != "" {
			value, err := strconv.Atoi(safeSearch)
			if err != nil {
				invalid := -1
				req.SafeSearch = &invalid
			} else {
				req.SafeSearch = &value
			}
		}
		req.Language = strings.TrimSpace(r.URL.Query().Get("lang"))
		req.Region = strings.TrimSpace(r.URL.Query().Get("region"))
		req.TimeRange = strings.TrimSpace(r.URL.Query().Get("time_range"))
		req.TimeFrom = strings.TrimSpace(r.URL.Query().Get("time_from"))
		req.TimeTo = strings.TrimSpace(r.URL.Query().Get("time_to"))
		req.Engines = splitSearchEngineFilters(r.URL.Query()["engines"])
	}

	// Validate all request fields per AI.md PART 3 using go-playground/validator
	if err := h.validate.Struct(req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request parameters", err.Error())
		return
	}
	if err := validateSearchDateRange(req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request parameters", err.Error())
		return
	}

	// Set defaults
	req.Category = model.ParseCategory(req.Category).String()
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	// Perform search
	query := model.NewQuery(req.Query)
	query.Category = model.ParseCategory(req.Category)
	query.Page = req.Page
	query.PerPage = req.Limit
	if req.SafeSearch != nil {
		query.SafeSearch = *req.SafeSearch
	}
	if req.TimeRange != "" {
		query.TimeRange = req.TimeRange
	}
	query.DateAfter = req.TimeFrom
	query.DateBefore = req.TimeTo
	query.Region = req.Region
	query.Engines = append([]string(nil), req.Engines...)
	if req.Language != "" {
		query.Language = req.Language
	}

	ctx := r.Context()
	results, err := h.aggregator.Search(ctx, query)
	if err != nil && !errors.Is(err, model.ErrNoResults) {
		// Per AI.md canonical status table: validation errors are 400, no
		// configured/available engines is 503 (maintenance/overloaded), and
		// any other search failure is 502 (upstream search-engine failure)
		// rather than a generic 500 - the aggregator itself has no other
		// local failure mode at this point.
		switch {
		case errors.Is(err, model.ErrEmptyQuery), errors.Is(err, model.ErrInvalidCategory):
			h.errorResponse(w, http.StatusBadRequest, "Search failed", err.Error())
		case errors.Is(err, model.ErrNoEngines):
			h.errorResponse(w, http.StatusServiceUnavailable, "Search failed", err.Error())
		default:
			h.errorResponse(w, http.StatusBadGateway, "Search failed", err.Error())
		}
		return
	}

	// Convert results
	apiResults := make([]SearchResult, 0, len(results.Results))
	for _, result := range results.GetPage(req.Page) {
		thumbnail := h.RewriteThumbnail(result.Thumbnail)
		imageURL := result.ImageURL
		if imageURL == "" && result.Category == model.CategoryImages {
			if fullImage, ok := result.Metadata["full_image"].(string); ok {
				imageURL = fullImage
			}
		}
		imageURL = h.RewriteThumbnail(imageURL)
		apiResults = append(apiResults, SearchResult{
			Title:       result.Title,
			URL:         result.URL,
			Description: result.Content,
			Engine:      result.Engine,
			Score:       result.Score,
			Category:    string(result.Category),
			Thumbnail:   thumbnail,
			ImageURL:    imageURL,
			ImageWidth:  result.ImageWidth,
			ImageHeight: result.ImageHeight,
			ImageFormat: result.ImageFormat,
			FileSize:    result.FileSize,
			Duration:    result.Duration,
			ViewCount:   result.ViewCount,
			Author:      result.Author,
			EmbedURL:    result.EmbedURL,
			Domain:      extractDomain(result.URL),
		})
	}

	// Calculate total pages per AI.md PART 14 pagination format
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: SearchResponse{
			Query:      req.Query,
			Category:   req.Category,
			Results:    apiResults,
			SearchTime: float64(time.Since(start).Microseconds()) / 1000,
			Engines:    results.Engines,
		},
		Pagination: &Pagination{
			Page:  results.Page,
			Limit: results.PerPage,
			Total: results.TotalResults,
			Pages: results.TotalPages,
		},
		Meta: &APIMeta{
			Version:     APIVersion,
			ProcessTime: float64(time.Since(start).Microseconds()) / 1000,
		},
	})
}

func splitSearchEngineFilters(values []string) []string {
	var engines []string
	for _, value := range values {
		for _, engine := range strings.Split(value, ",") {
			if engine = strings.TrimSpace(engine); engine != "" {
				engines = append(engines, engine)
			}
		}
	}
	return engines
}

func validateSearchDateRange(req SearchRequest) error {
	parseDate := func(name, value string) (time.Time, error) {
		if value == "" {
			return time.Time{}, nil
		}
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return time.Time{}, fmt.Errorf("%s must use YYYY-MM-DD format", name)
		}
		return parsed, nil
	}
	from, err := parseDate("time_from", req.TimeFrom)
	if err != nil {
		return err
	}
	to, err := parseDate("time_to", req.TimeTo)
	if err != nil {
		return err
	}
	if req.TimeRange == "custom" && (req.TimeFrom == "" || req.TimeTo == "") {
		return fmt.Errorf("custom time_range requires time_from and time_to")
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		return fmt.Errorf("time_from must not be after time_to")
	}
	return nil
}

// HandleAutocomplete is the public method for autocomplete suggestions
func (h *Handler) HandleAutocomplete(w http.ResponseWriter, r *http.Request) {
	h.handleAutocomplete(w, r)
}

// AutocompleteSuggestions returns provider suggestions for use by non-HTTP APIs.
func (h *Handler) AutocompleteSuggestions(ctx context.Context, query string) []string {
	return h.fetchAutocompleteSuggestions(ctx, strings.TrimSpace(query))
}

func (h *Handler) handleAutocomplete(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK:   true,
			Data: []string{},
			Meta: &APIMeta{Version: APIVersion},
		})
		return
	}

	// Fetch suggestions from DuckDuckGo's autocomplete API
	suggestions := h.fetchAutocompleteSuggestions(r.Context(), query)

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: suggestions,
		Meta: &APIMeta{Version: APIVersion},
	})
}

// fetchAutocompleteSuggestions fetches search suggestions from DuckDuckGo
func (h *Handler) fetchAutocompleteSuggestions(ctx context.Context, query string) []string {
	query = strings.TrimSpace(query)
	if query == "" || len(query) > maximumAutocompleteQueryBytes {
		return []string{}
	}
	cacheKey := strings.ToLower(query)
	if suggestions, ok := h.cachedAutocomplete(cacheKey); ok {
		return suggestions
	}

	// DuckDuckGo autocomplete API
	apiURL := fmt.Sprintf("https://duckduckgo.com/ac/?q=%s&type=list", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return []string{}
	}

	req.Header.Set("User-Agent", engine.UserAgent)

	client := h.autocompleteClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	resp, err := client.Do(req)
	if err != nil {
		return []string{}
	}
	defer closeutil.Close(resp.Body)

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return []string{}
	}

	// DuckDuckGo returns: ["query", ["suggestion1", "suggestion2", ...]]
	body, err := io.ReadAll(io.LimitReader(resp.Body, maximumAutocompleteResponseBytes+1))
	if err != nil || len(body) > maximumAutocompleteResponseBytes {
		return []string{}
	}
	var result []interface{}
	if err := json.Unmarshal(body, &result); err != nil {
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
			str = cleanAutocompleteSuggestion(str)
			if str != "" {
				suggestions = append(suggestions, str)
			}
			if len(suggestions) == maximumAutocompleteSuggestions {
				break
			}
		}
	}

	h.cacheAutocomplete(cacheKey, suggestions)
	return suggestions
}

func cleanAutocompleteSuggestion(suggestion string) string {
	suggestion = strings.TrimSpace(strings.Map(func(character rune) rune {
		if character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, suggestion))
	if len(suggestion) > maximumAutocompleteSuggestionLength {
		suggestion = suggestion[:maximumAutocompleteSuggestionLength]
		for !utf8.ValidString(suggestion) {
			suggestion = suggestion[:len(suggestion)-1]
		}
		suggestion = strings.TrimSpace(suggestion)
	}
	return suggestion
}

func (h *Handler) cachedAutocomplete(key string) ([]string, bool) {
	h.autocompleteMu.RLock()
	entry, ok := h.autocompleteCache[key]
	h.autocompleteMu.RUnlock()
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return append([]string(nil), entry.suggestions...), true
}

func (h *Handler) cacheAutocomplete(key string, suggestions []string) {
	h.autocompleteMu.Lock()
	defer h.autocompleteMu.Unlock()
	if h.autocompleteCache == nil {
		h.autocompleteCache = make(map[string]autocompleteCacheEntry)
	}
	now := time.Now()
	for cachedKey, entry := range h.autocompleteCache {
		if now.After(entry.expiresAt) {
			delete(h.autocompleteCache, cachedKey)
		}
	}
	if _, exists := h.autocompleteCache[key]; !exists && len(h.autocompleteCache) >= maximumAutocompleteCacheEntries {
		oldestKey := ""
		var oldest time.Time
		for cachedKey, entry := range h.autocompleteCache {
			if oldestKey == "" || entry.expiresAt.Before(oldest) {
				oldestKey, oldest = cachedKey, entry.expiresAt
			}
		}
		delete(h.autocompleteCache, oldestKey)
	}
	if len(suggestions) > maximumAutocompleteSuggestions {
		suggestions = suggestions[:maximumAutocompleteSuggestions]
	}
	h.autocompleteCache[key] = autocompleteCacheEntry{
		suggestions: append([]string(nil), suggestions...),
		expiresAt:   now.Add(time.Hour),
	}
}

func (h *Handler) handleEngines(w http.ResponseWriter, r *http.Request) {
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true, Data: h.EngineInventory(), Meta: &APIMeta{Version: APIVersion},
	})
}

// EngineInventory returns the current engine data used by REST and GraphQL.
func (h *Handler) EngineInventory() []EngineInfo {
	if h.registry == nil {
		return []EngineInfo{}
	}
	allEngines := h.registry.GetAll()
	engineList := make([]EngineInfo, 0, len(allEngines))
	for _, eng := range allEngines {
		categories := make([]string, 0)
		if cfg := eng.GetConfig(); cfg != nil {
			categories = append(categories, cfg.Categories...)
		}
		engineList = append(engineList, EngineInfo{
			ID: eng.Name(), Name: eng.DisplayName(), Enabled: eng.IsEnabled(),
			Priority: eng.GetPriority(), Categories: categories, Health: engineHealth(eng),
		})
	}
	return engineList
}

func (h *Handler) handleEngineByID(w http.ResponseWriter, r *http.Request) {
	// Extract engine ID from path
	path := r.URL.Path
	id := strings.TrimPrefix(path, APIPrefix+"/engines/")
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
			Health:     engineHealth(engine),
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

func engineHealth(engine search.Engine) *search.EngineHealth {
	tracker, ok := engine.(interface{ GetHealth() search.EngineHealth })
	if !ok {
		return nil
	}

	health := tracker.GetHealth()
	return &health
}

func (h *Handler) handleCategories(w http.ResponseWriter, r *http.Request) {
	categories := AvailableCategories()

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: categories,
		Meta: &APIMeta{Version: APIVersion},
	})
}

// AvailableCategories returns the public search category catalog shared by REST and GraphQL.
func AvailableCategories() []CategoryInfo {
	return []CategoryInfo{
		{ID: "general", Name: "Web", Description: "General web search", Icon: "🌐"},
		{ID: "images", Name: "Images", Description: "Image search", Icon: "🖼️"},
		{ID: "videos", Name: "Videos", Description: "Video search", Icon: "🎥"},
		{ID: "news", Name: "News", Description: "News search", Icon: "📰"},
		{ID: "maps", Name: "Maps", Description: "Map and location search", Icon: "🗺️"},
		{ID: "files", Name: "Files", Description: "Document and downloadable file search", Icon: "📁"},
		{ID: "music", Name: "Music", Description: "Music tracks, artists, albums, and videos", Icon: "🎵"},
		{ID: "science", Name: "Science", Description: "Scientific papers and research search", Icon: "🔬"},
		{ID: "it", Name: "IT", Description: "Developer, code, and technical search", Icon: "💻"},
		{ID: "social", Name: "Social", Description: "Social media and community search", Icon: "💬"},
	}
}

// Helper methods

// jsonResponse sends JSON response with 2-space indentation per AI.md PART 14
func (h *Handler) jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-API-Version", APIVersion)
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		slog.Debug("failed to encode API response", "error", err)
	}
}

// textResponse sends plain text response per AI.md PART 14
// Ensures response ends with exactly one newline
func (h *Handler) textResponse(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-API-Version", APIVersion)
	w.WriteHeader(status)
	// Per AI.md PART 14: All text responses must end with single newline
	text = strings.TrimRight(text, "\n") + "\n"
	if _, err := w.Write([]byte(text)); err != nil {
		slog.Debug("failed to write API text response", "error", err)
	}
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
func (h *Handler) errorResponse(w http.ResponseWriter, status int, message, detail string) {
	// Get request ID from response header (set by middleware)
	requestID := w.Header().Get("X-Request-ID")

	// Log the internal detail for operators — never exposed in the API response
	// (Tier 3 per AI.md PART 11: internal error details are debug-only)
	if detail != "" && status >= 500 {
		slog.Error("API error", "request_id", requestID, "message", message, "detail", detail)
	}

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

// checkCacheHealth verifies the cache backend responds per AI.md PART 13.
// The aggregator result cache is the active cache; Ping exercises the backend.
func (h *Handler) checkCacheHealth() string {
	if h.aggregator == nil {
		return "error"
	}
	rc := h.aggregator.Cache()
	if rc == nil {
		return "error"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rc.Ping(ctx); err != nil {
		return "error"
	}
	return "ok"
}

// checkDiskHealth verifies the data directory is accessible
func (h *Handler) checkDiskHealth() string {
	dataDir := config.GetDataDir()
	if dataDir == "" {
		return "error"
	}
	if _, err := os.Stat(dataDir); err != nil {
		return "error"
	}
	return "ok"
}

// checkSchedulerHealth verifies the scheduler is running per AI.md PART 18.
// The scheduler is always running — nil or stopped is an error.
func (h *Handler) checkSchedulerHealth() string {
	if h.scheduler == nil || !h.scheduler.IsRunning() {
		return "error"
	}
	return "ok"
}

// getRequestsTotal returns total requests since startup.
func (h *Handler) getRequestsTotal() int64 {
	if h.healthMetrics == nil {
		return 0
	}
	return h.healthMetrics.GetTotalRequests()
}

// getRequests24h returns requests in the trailing 24-hour window.
func (h *Handler) getRequests24h() int64 {
	if h.healthMetrics == nil {
		return 0
	}
	return h.healthMetrics.GetRequestsLast24Hours()
}

// getActiveConnections returns the current number of active connections.
func (h *Handler) getActiveConnections() int {
	if h.healthMetrics == nil {
		return 0
	}
	return int(h.healthMetrics.GetActiveConnections())
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
// AvailableBangs returns the same built-in bang catalog exposed by the REST API.
func AvailableBangs() []BangInfo { return getBuiltinBangs() }

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

	// Inject client IP so IP-related instant handlers return the requester's address.
	ctx = instant.WithClientIP(ctx, httputil.GetClientIP(r))

	// Inject GeoIP lookup so instant handlers can enrich IP answers with geo data.
	ctx = instant.WithGeoIPLookup(ctx, h.geoipLookup)
	units := "imperial"
	if h.config != nil && (h.config.Search.Widgets.Weather.Units == "metric" || h.config.Search.Widgets.Weather.Units == "imperial") {
		units = h.config.Search.Widgets.Weather.Units
	}
	if c, err := r.Cookie("search_pref_units"); err == nil && (c.Value == "metric" || c.Value == "imperial") {
		units = c.Value
	}
	weatherUnits := "auto"
	if c, err := r.Cookie("search_widget_weather_units"); err == nil && (c.Value == "auto" || c.Value == "metric" || c.Value == "imperial") {
		weatherUnits = c.Value
	}
	ctx = instant.WithUnits(ctx, units)
	ctx = instant.WithWeatherUnits(ctx, weatherUnits)

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

	// Parse path: /api/{api_version}/direct/{type}/{term}
	path := strings.TrimPrefix(r.URL.Path, APIPrefix+"/direct/")
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

	// Per AI.md PART 19: the CC BY 4.0 notices for the ip-location-db
	// databases are a license condition and must appear verbatim on
	// /server/about whenever the data is in use
	if h.config.Server.GeoIP.Enabled {
		sections = append(sections, PageSection{
			ID:      "geoip-attribution",
			Title:   "IP Geolocation Data",
			Content: geoIPAttributionHTML + " " + geoIPAttributionText,
		})
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
			Content: appName + " is a privacy-respecting metasearch engine. Type a query in the search box, refine it with category tabs, or use bang shortcuts and direct answers for targeted lookups.",
		},
		{
			ID:      "categories",
			Title:   "Search Categories",
			Content: "Available categories are general, images, videos, news, maps, files, it, science, and social. Each category narrows the enabled engines and result types for the query.",
		},
		{
			ID:      "bangs",
			Title:   "Bang Commands",
			Content: "Bang commands send searches directly to another site. Use !shortcut terms or terms !shortcut. Examples include !g, !ddg, !b, !br, !sp, !w, !yt, !gh, !so, !rd, !az, !gm, !osm, !wa, !mdn, !npm, !pypi, !gopkg, !scholar, !arxiv, !archive, and !spot.",
		},
		{
			ID:      "answers",
			Title:   "Instant and Direct Answers",
			Content: "Direct answers use the type:term format and open as full-page results, for example dns:example.com, whois:example.com, http:404, regex:[a-z]+, jwt:decode token, useragent:my, and rule:34. Instant answers also work from the normal search box for queries such as 2 + 2, convert 10 km to miles, time in tokyo, what is my ip, or #ff5733.",
		},
		{
			ID:      "search_tips",
			Title:   "Search Tips",
			Content: "Use quotes for exact phrases, minus to exclude terms, OR and AND for boolean searches, wildcards with *, and filters such as site:, -site:, filetype:, intitle:, inurl:, intext:, related:, cache:, info:, before:, after:, define:, weather:, stocks:, map:, movie:, and source:.",
		},
		{
			ID:      "keyboard_shortcuts",
			Title:   "Keyboard Shortcuts",
			Content: "Use / to focus the search box, Escape to clear or close dialogs, t to cycle theme (dark, light, auto), ? to show shortcuts, j and k to move through results, Enter or o/O to open a result, h/l or the arrow keys to change pages, gg or G to jump, and 1-9 to open a specific result.",
		},
		{
			ID:      "api_documentation",
			Title:   "API Documentation",
			Content: "REST, direct-answer, GraphQL, and OpenAPI interfaces are available. Key endpoints include /api/v1/search, /api/v1/search/related, /api/v1/autocomplete, /api/v1/instant, /api/v1/direct/{type}/{term}, /api/v1/engines, /api/v1/categories, /api/v1/bangs, /api/v1/widgets, /api/v1/server/help, /api/v1/server/about, /api/v1/server/privacy, /api/v1/server/contact, /api/v1/server/terms, /api/graphql, /server/docs/graphql, /server/docs/swagger (Swagger UI), and /api/swagger (OpenAPI spec).",
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
		slog.Debug("favicon error", "domain", domain, "err", err)
		h.serveFaviconFallback(w)
		return
	}

	// Set a generic user agent
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FaviconFetcher/1.0)")
	req.Header.Set("Accept", "image/*")

	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("favicon error", "domain", domain, "err", err)
		h.serveFaviconFallback(w)
		return
	}
	defer closeutil.Close(resp.Body)

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
		slog.Debug("favicon error", "domain", domain, "err", err)
		h.serveFaviconFallback(w)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", contentType)
	// Cache for 24 hours
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(body); err != nil {
		slog.Debug("failed to write API binary response", "error", err)
	}
}

// serveFaviconFallback serves a 1x1 transparent PNG as fallback
// This allows the client-side JS to detect load failure and show placeholder
// handleServerContact handles GET|POST /api/v1/server/contact per AI.md PART 1.
// GET: returns contact form availability and config.
// POST: submits a contact message (JSON body); same validation as web form.
func (h *Handler) handleServerContact(w http.ResponseWriter, r *http.Request) {
	accept := r.Header.Get("Accept")
	wantsJSON := strings.Contains(accept, "application/json") || !strings.Contains(accept, "text/html")
	contactEnabled := h.config.Server.Pages.Contact.Enabled &&
		(h.config.Server.Contact.General.Email != "" || h.config.Server.Contact.Admin.Email != "" || notify.HasConfiguredWebhook(h.config.Server.Contact, "general"))

	if r.Method == http.MethodGet {
		if !wantsJSON {
			http.Redirect(w, r, "/server/contact", http.StatusSeeOther)
			return
		}
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK:   true,
			Data: map[string]interface{}{"enabled": contactEnabled, "title": "Contact"},
		})
		return
	}

	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", r.Method)
		return
	}
	if !contactEnabled {
		h.errorResponse(w, http.StatusNotFound, "Contact form is not enabled", "")
		return
	}
	var body struct {
		Name    string `json:"name"`
		Email   string `json:"email"`
		Subject string `json:"subject"`
		Message string `json:"message"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.writeError(w, "BAD_REQUEST", "Invalid JSON body", http.StatusBadRequest)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.TrimSpace(body.Email)
	body.Subject = strings.TrimSpace(body.Subject)
	body.Message = strings.TrimSpace(body.Message)
	address, err := mail.ParseAddress(body.Email)
	if body.Name == "" || err != nil || address.Address != body.Email || body.Subject == "" || body.Message == "" || len(body.Name) > 200 || len(body.Subject) > 300 || len(body.Message) > 10000 {
		h.writeError(w, "BAD_REQUEST", "Name, valid email, subject, and message are required", http.StatusBadRequest)
		return
	}
	if h.contactSubmitter == nil {
		h.writeError(w, "NOT_AVAILABLE", "Contact delivery is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := h.contactSubmitter.SubmitContact(r.Context(), ContactMessage{Name: body.Name, Email: address.Address, Subject: body.Subject, Message: body.Message, SourceIP: httputil.GetClientIP(r)}); err != nil {
		h.writeError(w, "DELIVERY_UNAVAILABLE", "Contact message could not be delivered", http.StatusServiceUnavailable)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"status": "received"}})
}

// handlePreferences handles GET|POST /api/v1/server/preferences per AI.md PART 1.
// Preferences are stored client-side (localStorage/cookies). The API endpoint
// provides the schema and acknowledges client-submitted preference saves.
func (h *Handler) PreferenceSchema() map[string]interface{} {
	return map[string]interface{}{"storage": "client-side", "fields": []string{"theme", "language", "safe_search", "per_page", "default_category", "engines"}}
}

func (h *Handler) SavePreferences() map[string]string {
	return map[string]string{"status": "saved"}
}

func (h *Handler) handlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		manager := h.syncI18nManager()
		state := prefsync.Current(r, "search", manager.DefaultLanguage(), manager.SupportedLanguageCodes())
		h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: state, Meta: &APIMeta{Version: APIVersion}})
		return
	}

	if r.Method == http.MethodPost {
		values := url.Values{}
		if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			var input map[string]json.RawMessage
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
				h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
				return
			}
			for key, raw := range input {
				var text string
				if json.Unmarshal(raw, &text) != nil {
					var scalar interface{}
					if json.Unmarshal(raw, &scalar) == nil {
						text = fmt.Sprint(scalar)
					}
				}
				values.Set(key, text)
			}
		} else {
			if err := r.ParseForm(); err != nil {
				h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid form body")
				return
			}
			values = r.Form
		}
		hasCode := values.Has("code")
		values.Del("code")
		clone := r.Clone(r.Context())
		urlCopy := *r.URL
		urlCopy.RawQuery = values.Encode()
		clone.URL = &urlCopy
		manager := h.syncI18nManager()
		result := prefsync.Import(clone, "search", manager.DefaultLanguage(), manager.SupportedLanguageCodes())
		if hasCode {
			result.Ignored = append(result.Ignored, "code")
			sort.Strings(result.Ignored)
		}
		if len(result.Applied) == 0 {
			h.errorResponse(w, http.StatusBadRequest, "VALIDATION_FAILED", "no valid preferences were supplied")
			return
		}
		prefsync.Apply(w, r, "search", result)
		h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"theme": result.State.Theme, "lang": result.State.Lang, "prefs": result.State.Prefs, "applied": result.Applied, "ignored": result.Ignored}, Meta: &APIMeta{Version: APIVersion}})
		return
	}

	h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", r.Method)
}

// syncI18nManager returns an i18n manager sufficient for language-code
// validation. Loaded translations aren't needed here - only the supported
// language list, which NewManager populates without loading locale files.
func (h *Handler) syncI18nManager() *i18n.Manager {
	return i18n.NewManager("en", i18n.DefaultSupportedLanguages())
}

// handleServerPreferencesExport handles GET /api/v1/server/preferences/export
// per AI.md "Cross-device preference sync": reads the current theme/lang
// cookies and returns the full import URL plus a base64url short code for
// the same state. Stateless - nothing is written or looked up server-side.
func (h *Handler) handleServerPreferencesExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported")
		return
	}

	manager := h.syncI18nManager()
	state, fullURL, shortCode := prefsync.Export(r, "search", manager.DefaultLanguage(), manager.SupportedLanguageCodes())

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: map[string]interface{}{"theme": state.Theme, "lang": state.Lang, "prefs": state.Prefs, "url": fullURL, "code": shortCode},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerPreferencesImport handles GET /api/v1/server/preferences/import
// per AI.md "Cross-device preference sync": accepts theme/lang as a plain
// query string, or a pasted base64url short code (in the "code" param,
// optionally still carrying a "https://.../import?" prefix which is
// stripped). Each value is validated against its normal enum/BCP-47
// allowlist - unknown or malformed values are dropped rather than erroring.
// Valid values are written as cookies and acknowledged in the JSON response;
// unlike the WebUI mirror this never redirects, since API clients expect a
// JSON body back.
func (h *Handler) handleServerPreferencesImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported")
		return
	}

	manager := h.syncI18nManager()
	result := prefsync.Import(r, "search", manager.DefaultLanguage(), manager.SupportedLanguageCodes())
	prefsync.Apply(w, r, "search", result)
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"theme": result.State.Theme, "lang": result.State.Lang, "prefs": result.State.Prefs, "applied": result.Applied, "ignored": result.Ignored}, Meta: &APIMeta{Version: APIVersion}})
}

func (h *Handler) serveFaviconFallback(w http.ResponseWriter) {
	// 1x1 transparent PNG (smallest valid PNG)
	transparentPNG := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
	data, _ := base64.StdEncoding.DecodeString(transparentPNG)

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		slog.Debug("failed to write API fallback response", "error", err)
	}
}

// requireOperator wraps a handler and rejects requests without a valid operator
// bearer token. Per AI.md PART 14: operator-gated endpoints use Bearer auth.
// Token comparison is constant-time over SHA-256 digests to prevent timing leaks.
func (h *Handler) operatorAuthError(r *http.Request) error {
	expected := h.config.Get().Token
	if expected == "" {
		return errors.New("operator token not configured")
	}
	hdr := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(hdr) <= len(prefix) || !strings.EqualFold(hdr[:len(prefix)], prefix) {
		return errors.New("operator token required")
	}
	presented := strings.TrimSpace(hdr[len(prefix):])
	expectedSum := sha256.Sum256([]byte(expected))
	presentedSum := sha256.Sum256([]byte(presented))
	if subtle.ConstantTimeCompare(expectedSum[:], presentedSum[:]) != 1 {
		return errors.New("invalid operator token")
	}
	return nil
}

// OperatorServerStatus and OperatorServerConfig preserve the REST operator-token requirement for GraphQL fields.
func (h *Handler) OperatorServerStatus(r *http.Request) (interface{}, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	return h.serverStatusData(), nil
}

func (h *Handler) serverStatusData() map[string]interface{} {
	cfg := h.config.Get()
	status := "healthy"
	if cfg.MaintenanceMode {
		status = "maintenance"
	}
	return map[string]interface{}{
		"status": status, "version": APIVersion, "mode": cfg.Mode,
		"uptime": h.formatDuration(time.Since(h.startTime)), "go_version": runtime.Version(),
		"tor": map[string]interface{}{"enabled": cfg.Tor.Enabled, "running": h.torService != nil && h.torService.IsRunning()},
	}
}

func (h *Handler) OperatorServerConfig(r *http.Request) (interface{}, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	return h.serverConfigData(), nil
}

func (h *Handler) serverConfigData() map[string]interface{} {
	cfg := h.config.Get()
	return map[string]interface{}{
		"mode": cfg.Mode, "port": cfg.Port, "https_port": cfg.HTTPSPort,
		"address": cfg.Address, "base_url": cfg.BaseURL, "maintenance": cfg.MaintenanceMode,
		"branding": map[string]string{"title": cfg.Branding.Title, "tagline": cfg.Branding.Tagline, "description": cfg.Branding.Description},
		"features": map[string]bool{"tor": cfg.Tor.Enabled, "geoip": cfg.GeoIP.Enabled, "metrics": cfg.Metrics.Enabled},
	}
}

func (h *Handler) requireOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.operatorAuthError(r); err != nil {
			if err.Error() != "operator token not configured" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="operator"`)
			}
			message := "Operator token not configured"
			if err.Error() == "operator token required" {
				message = "Operator token required"
			}
			if err.Error() == "invalid operator token" {
				message = "Invalid operator token"
			}
			h.errorResponse(w, http.StatusUnauthorized, message, "")
			return
		}
		next(w, r)
	}
}

// handleServerStatus handles GET /api/v1/server/status (operator token required).
// Per AI.md PART 14: operator-gated JSON status endpoint mirrors /server/status HTML page.
func (h *Handler) handleServerStatus(w http.ResponseWriter, _ *http.Request) {
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: h.serverStatusData()})
}

// handleServerConfig handles GET /api/v1/server/config (operator token required).
// Per AI.md PART 14: returns a redacted view of the current server configuration.
// Sensitive fields (token, secret_key) are never included in the response.
func (h *Handler) handleServerConfig(w http.ResponseWriter, _ *http.Request) {
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: h.serverConfigData()})
}

// GeoIP attribution notices required verbatim by AI.md PART 19. Both are
// required together: the DB-IP notice does not cover the NRO-sourced country
// data, and the NRO notice does not cover the DB-IP city data.
const (
	geoIPAttributionHTML = `<a href="https://db-ip.com/">IP Geolocation by DB-IP</a>`
	geoIPAttributionText = "Country and ASN data licensed CC BY 4.0 by the Number Resource Organization (NRO)."
)

// GeoIPLookupResponse is the payload of a GeoIP IP lookup.
// Organization data comes from the ASN database's autonomous_system_organization
// field — BGP/RIR data, never an RDAP/WHOIS registrant record (AI.md PART 19).
type GeoIPLookupResponse struct {
	IP          string  `json:"ip"`
	Found       bool    `json:"found"`
	Private     bool    `json:"private"`
	CountryCode string  `json:"country_code,omitempty"`
	CountryName string  `json:"country_name,omitempty"`
	Continent   string  `json:"continent,omitempty"`
	City        string  `json:"city,omitempty"`
	Region      string  `json:"region,omitempty"`
	PostalCode  string  `json:"postal_code,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
	ASN         uint    `json:"asn,omitempty"`
	ASNOrg      string  `json:"asn_org,omitempty"`
	Attribution string  `json:"attribution"`
}

// GeoIPPresetResponse is a single named country-list preset.
type GeoIPPresetResponse struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
}

// geoIPPresetRequest is the body accepted when saving a preset.
// Either codes or from is supplied: from copies the current deny_countries or
// allow_countries selection, which is the "save the current selection as a
// named preset" flow required by AI.md PART 19.
type geoIPPresetRequest struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
	From  string   `json:"from"`
}

// handleGeoIPLookup handles GET /api/v1/server/geoip/ips and
// GET /api/v1/server/geoip/ips/{ip}. With no {ip} the requesting client's
// resolved IP is used. Per AI.md PART 19 private addresses are never
// looked up and the response always carries the required attribution.
func (h *Handler) handleGeoIPLookup(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimSuffix(chi.URLParam(r, "ip"), ".txt")
	if target == "" {
		target = httputil.GetClientIP(r)
	}
	payload, err := h.LookupGeoIP(target)
	if err != nil {
		if h.geoipLookup == nil || !h.geoipLookup.IsLoaded() {
			h.errorResponse(w, http.StatusServiceUnavailable, "GeoIP database is not available", "")
			return
		}
		h.errorResponse(w, http.StatusBadRequest, "Invalid IP address", "")
		return
	}

	h.respondWithFormat(w, r, http.StatusOK, &APIResponse{
		OK:   true,
		Data: payload,
		Meta: &APIMeta{Version: APIVersion},
	}, func() string {
		var b strings.Builder
		fmt.Fprintf(&b, "IP: %s\n", payload.IP)
		if payload.Private {
			b.WriteString("Private: yes\n")
		}
		if payload.CountryCode != "" {
			fmt.Fprintf(&b, "Country: %s (%s)\n", payload.CountryName, payload.CountryCode)
		}
		if payload.City != "" {
			fmt.Fprintf(&b, "City: %s\n", payload.City)
		}
		if payload.Region != "" {
			fmt.Fprintf(&b, "Region: %s\n", payload.Region)
		}
		if payload.Timezone != "" {
			fmt.Fprintf(&b, "Timezone: %s\n", payload.Timezone)
		}
		if payload.ASN != 0 {
			fmt.Fprintf(&b, "ASN: AS%d %s\n", payload.ASN, payload.ASNOrg)
		}
		b.WriteString(geoIPAttributionText)
		return b.String()
	})
}

// LookupGeoIP returns the same attributed payload used by the REST lookup endpoint.
func (h *Handler) LookupGeoIP(target string) (GeoIPLookupResponse, error) {
	if h.geoipLookup == nil || !h.geoipLookup.IsLoaded() {
		return GeoIPLookupResponse{}, errors.New("GeoIP database is not available")
	}
	if net.ParseIP(target) == nil {
		return GeoIPLookupResponse{}, errors.New("invalid IP address")
	}
	result := h.geoipLookup.Lookup(target)
	return GeoIPLookupResponse{
		IP: result.IP, Found: result.Found, Private: result.Private,
		CountryCode: result.CountryCode, CountryName: result.CountryName, Continent: result.Continent,
		City: result.City, Region: result.Region, PostalCode: result.PostalCode,
		Latitude: result.Latitude, Longitude: result.Longitude, Timezone: result.Timezone,
		ASN: result.ASN, ASNOrg: result.ASNOrg, Attribution: geoIPAttributionText,
	}, nil
}

// handleGeoIPPresets handles GET /api/v1/server/geoip/presets (operator token
// required) and lists the operator-authored country lists from server.yml.
func (h *Handler) handleGeoIPPresets(w http.ResponseWriter, r *http.Request) {
	cfg := h.config.Get()
	presets := make([]GeoIPPresetResponse, 0, len(cfg.GeoIP.Presets))
	for _, name := range cfg.GeoIP.PresetNames() {
		codes, _ := cfg.GeoIP.Preset(name)
		presets = append(presets, GeoIPPresetResponse{Name: name, Codes: codes})
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: map[string]interface{}{"presets": presets},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleGeoIPPreset handles GET /api/v1/server/geoip/presets/{name} (operator
// token required). Per AI.md PART 19 resolving a preset only returns the codes
// for the operator to review — it never applies them to deny_countries or
// allow_countries.
func (h *Handler) handleGeoIPPreset(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	cfg := h.config.Get()
	codes, ok := cfg.GeoIP.Preset(name)
	if !ok {
		h.errorResponse(w, http.StatusNotFound, "Preset not found", "")
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: GeoIPPresetResponse{Name: name, Codes: codes},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleGeoIPPresetSave handles POST /api/v1/server/geoip/presets (operator
// token required) and persists a named country list to server.yml. Saving a
// preset never changes the enforced deny_countries/allow_countries values.
func (h *Handler) handleGeoIPPresetSave(w http.ResponseWriter, r *http.Request) {
	var req geoIPPresetRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64*1024)).Decode(&req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid JSON body", err.Error())
		return
	}

	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !config.IsValidPresetName(name) {
		h.errorResponse(w, http.StatusBadRequest, "Invalid preset name (a-z, 0-9, hyphen, underscore)", "")
		return
	}

	cfg := h.config.Get()
	codes := req.Codes
	switch strings.TrimSpace(req.From) {
	case "deny_countries":
		codes = cfg.GeoIP.DenyCountries
	case "allow_countries":
		codes = cfg.GeoIP.AllowedCountries
	case "":
	default:
		h.errorResponse(w, http.StatusBadRequest, "Invalid from value (deny_countries or allow_countries)", "")
		return
	}

	normalized := config.NormalizeCountryCodes(codes)
	if len(normalized) == 0 {
		h.errorResponse(w, http.StatusBadRequest, "No valid ISO 3166-1 alpha-2 country codes supplied", "")
		return
	}

	h.config.Update(func(s *config.ServerConfig) {
		s.GeoIP.SetPreset(name, normalized)
	})
	if err := h.saveConfig(); err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to persist preset", err.Error())
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: GeoIPPresetResponse{Name: name, Codes: normalized},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleGeoIPPresetDelete handles DELETE /api/v1/server/geoip/presets/{name}
// (operator token required) and removes a stored preset from server.yml.
func (h *Handler) handleGeoIPPresetDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	var removed bool
	h.config.Update(func(s *config.ServerConfig) {
		removed = s.GeoIP.DeletePreset(name)
	})
	if !removed {
		h.errorResponse(w, http.StatusNotFound, "Preset not found", "")
		return
	}
	if err := h.saveConfig(); err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to persist preset removal", err.Error())
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: map[string]interface{}{"name": name, "deleted": true},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// saveConfig writes the in-memory configuration back to server.yml.
func (h *Handler) saveConfig() error {
	path := h.config.GetPath()
	if path == "" {
		return errors.New("config path is not set")
	}
	return h.config.Save(path)
}

// SchedulerTasksForGraphQL returns scheduler tasks after the REST operator check.
func (h *Handler) SchedulerTasksForGraphQL(r *http.Request) ([]*scheduler.TaskInfo, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	if h.scheduler == nil {
		return nil, errors.New("scheduler is unavailable")
	}
	tasks := h.scheduler.GetTasks()
	if tasks == nil {
		tasks = []*scheduler.TaskInfo{}
	}
	return tasks, nil
}

// SchedulerTaskForGraphQL returns one registered task after the REST operator check.
func (h *Handler) SchedulerTaskForGraphQL(r *http.Request, id string) (*scheduler.TaskInfo, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	if h.scheduler == nil {
		return nil, errors.New("scheduler is unavailable")
	}
	return h.scheduler.GetTask(scheduler.TaskID(id))
}

// SchedulerHistoryForGraphQL returns the same bounded history as the REST endpoint.
func (h *Handler) SchedulerHistoryForGraphQL(r *http.Request, id string) ([]*scheduler.TaskHistoryEntry, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	if h.scheduler == nil {
		return nil, errors.New("scheduler is unavailable")
	}
	if _, err := h.scheduler.GetTask(scheduler.TaskID(id)); err != nil {
		return nil, err
	}
	history, err := h.scheduler.GetHistory(scheduler.TaskID(id), 20)
	if err != nil {
		return nil, err
	}
	if history == nil {
		history = []*scheduler.TaskHistoryEntry{}
	}
	return history, nil
}

// SetSchedulerTaskEnabledForGraphQL applies the REST enable/disable operation.
func (h *Handler) SetSchedulerTaskEnabledForGraphQL(r *http.Request, id string, enabled bool) (*scheduler.TaskInfo, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	if h.scheduler == nil {
		return nil, errors.New("scheduler is unavailable")
	}
	taskID := scheduler.TaskID(id)
	var err error
	if enabled {
		err = h.scheduler.Enable(taskID)
	} else {
		err = h.scheduler.Disable(taskID)
	}
	if err != nil {
		return nil, err
	}
	return h.scheduler.GetTask(taskID)
}

// RunSchedulerTaskForGraphQL triggers and audits the same manual operation as REST.
func (h *Handler) RunSchedulerTaskForGraphQL(r *http.Request, id string) (map[string]interface{}, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	if h.scheduler == nil {
		return nil, errors.New("scheduler is unavailable")
	}
	if err := h.scheduler.RunNow(scheduler.TaskID(id)); err != nil {
		return nil, errors.New("task cannot be run")
	}
	if h.auditLogger != nil {
		h.auditLogger.LogSchedulerTaskManualRun("operator", "-", id, true)
	}
	return map[string]interface{}{"task": id, "triggered": true}, nil
}

// GeoIPPresetSaveInput is the JSON input for saving an operator-authored preset.
type GeoIPPresetSaveInput struct {
	Name  string   `json:"name"`
	Codes []string `json:"codes"`
	From  string   `json:"from"`
}

// GeoIPPresetsForGraphQL returns the configured presets after operator authorization.
func (h *Handler) GeoIPPresetsForGraphQL(r *http.Request) ([]GeoIPPresetResponse, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	cfg := h.config.Get()
	presets := make([]GeoIPPresetResponse, 0, len(cfg.GeoIP.Presets))
	for _, name := range cfg.GeoIP.PresetNames() {
		codes, _ := cfg.GeoIP.Preset(name)
		presets = append(presets, GeoIPPresetResponse{Name: name, Codes: codes})
	}
	return presets, nil
}

// GeoIPPresetForGraphQL returns one configured preset after operator authorization.
func (h *Handler) GeoIPPresetForGraphQL(r *http.Request, name string) (GeoIPPresetResponse, error) {
	if err := h.operatorAuthError(r); err != nil {
		return GeoIPPresetResponse{}, err
	}
	cfg := h.config.Get()
	codes, ok := cfg.GeoIP.Preset(name)
	if !ok {
		return GeoIPPresetResponse{}, errors.New("preset not found")
	}
	return GeoIPPresetResponse{Name: name, Codes: codes}, nil
}

// SaveGeoIPPresetForGraphQL persists a named country list without applying it to enforcement.
func (h *Handler) SaveGeoIPPresetForGraphQL(r *http.Request, input GeoIPPresetSaveInput) (GeoIPPresetResponse, error) {
	if err := h.operatorAuthError(r); err != nil {
		return GeoIPPresetResponse{}, err
	}
	name := strings.ToLower(strings.TrimSpace(input.Name))
	if !config.IsValidPresetName(name) {
		return GeoIPPresetResponse{}, errors.New("invalid preset name")
	}
	cfg := h.config.Get()
	codes := input.Codes
	switch strings.TrimSpace(input.From) {
	case "deny_countries":
		codes = cfg.GeoIP.DenyCountries
	case "allow_countries":
		codes = cfg.GeoIP.AllowedCountries
	case "":
	default:
		return GeoIPPresetResponse{}, errors.New("invalid preset source")
	}
	normalized := config.NormalizeCountryCodes(codes)
	if len(normalized) == 0 {
		return GeoIPPresetResponse{}, errors.New("preset has no valid country codes")
	}
	h.config.Update(func(server *config.ServerConfig) { server.GeoIP.SetPreset(name, normalized) })
	if err := h.saveConfig(); err != nil {
		return GeoIPPresetResponse{}, errors.New("preset persistence failed")
	}
	return GeoIPPresetResponse{Name: name, Codes: normalized}, nil
}

// DeleteGeoIPPresetForGraphQL removes and persists a preset after operator authorization.
func (h *Handler) DeleteGeoIPPresetForGraphQL(r *http.Request, name string) (map[string]interface{}, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	var removed bool
	h.config.Update(func(server *config.ServerConfig) { removed = server.GeoIP.DeletePreset(name) })
	if !removed {
		return nil, errors.New("preset not found")
	}
	if err := h.saveConfig(); err != nil {
		return nil, errors.New("preset persistence failed")
	}
	return map[string]interface{}{"name": name, "deleted": true}, nil
}
