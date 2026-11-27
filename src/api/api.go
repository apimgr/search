package api

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
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
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/hubpages"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/common/prefsync"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/geoip"
	"github.com/apimgr/search/src/health"
	"github.com/apimgr/search/src/i2p"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/logging"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/notify"
	"github.com/apimgr/search/src/owner"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/search"
	searchbang "github.com/apimgr/search/src/search/bang"
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

var errOwnerTokenCannotAuthorizeOperator = errors.New("owner token cannot authorize operator route")

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
	StatsExtra     []health.Stat
}

type HealthProvider interface {
	HealthSnapshot() HealthSnapshot
}

type ContactMessage struct {
	Name               string
	Email              string
	Subject            string
	Message            string
	Type               string
	ReportedURL        string
	Reason             string
	CopyrightedWork    string
	GoodFaithStatement bool
	AccuracyStatement  bool
	Signature          string
	RequestKind        string
	SourceIP           string `json:"-"`
}

type ContactSubmitter interface {
	SubmitContact(context.Context, ContactMessage) error
}

type ContactCaptchaProvider interface {
	NewContactChallenge(*http.Request) (string, string)
	VerifyContactChallenge(*http.Request, string, string) bool
}

type ContactProofVerifier interface {
	VerifyContactProof(*http.Request, string, string, string) bool
}

type SecurityContactMessage struct {
	SecurityID, Name, Email, ResearcherGPG, AffectedComponent, AffectedEndpoint string
	Severity, Summary, StepsToReproduce, Impact, SuggestedFix                   string
	CVERequested, AgreedToDisclosure                                            bool
	DisclosureDays                                                              int
	CreditPreference, CreditHandle                                              string
}

type SecurityContactSubmitter interface {
	ValidContactSecurityID(*http.Request, string) bool
	SubmitSecurityContact(*http.Request, SecurityContactMessage) (string, error)
}

type SecurityPagesProvider interface {
	SecurityOverview(*http.Request) interface{}
	SecurityPolicy(*http.Request) interface{}
	SecurityThanks(context.Context) (interface{}, error)
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
	contactCaptcha    ContactCaptchaProvider
	securityContact   SecurityContactSubmitter
	securityPages     SecurityPagesProvider
	alertManager      *alert.Manager
	ownerManager      *owner.Manager
	strictMu          sync.Mutex
	strictAttempts    map[string][]time.Time
	mcpOriginAllowed  func(*http.Request, *url.URL) bool
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

// SetMCPOriginAllowlist shares the server's fully resolved CORS/public-origin
// policy with the MCP DNS-rebinding guard.
func (h *Handler) SetMCPOriginAllowlist(check func(*http.Request, *url.URL) bool) {
	h.mcpOriginAllowed = check
}

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

func (h *Handler) SetContactCaptchaProvider(provider ContactCaptchaProvider) {
	h.contactCaptcha = provider
}

func (h *Handler) SetSecurityContactSubmitter(submitter SecurityContactSubmitter) {
	h.securityContact = submitter
}

func (h *Handler) SetSecurityPagesProvider(provider SecurityPagesProvider) {
	h.securityPages = provider
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
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		h.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed", req.Method, req)
	})
	// Autodiscover - non-versioned per AI.md PART 32 line 38077-38157
	r.Get("/api/autodiscover", h.handleAutodiscover)
	r.Get("/cli/binaries/{filename}", h.handleCLIBinary)

	// Health and info - Per AI.md PART 13/14
	// Canonical API route: /api/v1/server/healthz (JSON default, text via .txt or Accept header)
	// Unversioned operational alias per PART 14 mounts the same handler.
	r.Get(APIPrefix+"/server/healthz", h.handleHealthz)
	r.Get(APIPrefix+"/server/healthz.txt", h.handleHealthz)
	r.Get("/api/healthz", h.handleHealthz)
	r.Get(APIPrefix+"/info", h.handleInfo)
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
	r.Get(APIPrefix+"/server/dpo", h.handleServerDPO)
	r.Get(APIPrefix+"/server/help", h.handleServerHelp)
	r.Get(APIPrefix+"/server/help/mcp", h.handleMCPHelp)
	r.Get(APIPrefix+"/server/terms", h.handleServerTerms)
	r.Get(APIPrefix+"/server/contact", h.handleServerContact)
	r.Post(APIPrefix+"/server/contact", h.handleServerContact)
	r.Get(APIPrefix+"/server/security", h.handleServerSecurity)
	r.Get(APIPrefix+"/server/security/policy", h.handleServerSecurityPolicy)
	r.Get(APIPrefix+"/server/security/thanks", h.handleServerSecurityThanks)
	r.Get(APIPrefix+"/server/auth/check", h.handleAuthCheck)
	r.Get(APIPrefix+"/server/preferences", h.handlePreferences)
	r.Post(APIPrefix+"/server/preferences", h.handlePreferences)
	r.Get(APIPrefix+"/server/preferences/sync", h.handleServerPreferencesExport)
	r.Get(APIPrefix+"/server/preferences/sync/import", h.handleServerPreferencesImport)
	r.Post(APIPrefix+"/server/mcp", h.handleMCP)
	r.Post("/api/mcp", h.handleMCP)
	r.Post("/mcp", h.handleMCP)
	r.Get(APIPrefix+"/server/preferences/tokens", h.ownerTokensList)
	r.Post(APIPrefix+"/server/preferences/tokens", h.ownerLinkedCreate)
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
	r.Get(APIPrefix+"/server/geoip/ips.txt", h.handleGeoIPLookup)
	r.Get(APIPrefix+"/server/geoip/ips/{ip}", h.handleGeoIPLookup)
	r.Get(APIPrefix+"/server/geoip/ips/{ip}.txt", h.handleGeoIPLookup)
	r.Get(APIPrefix+"/server/geoip/presets", h.requireOperator(h.handleGeoIPPresets))
	r.Post(APIPrefix+"/server/geoip/presets", h.requireOperator(h.handleGeoIPPresetSave))
	r.Get(APIPrefix+"/server/geoip/presets/{name}", h.requireOperator(h.handleGeoIPPreset))
	r.Delete(APIPrefix+"/server/geoip/presets/{name}", h.requireOperator(h.handleGeoIPPresetDelete))

	// Scheduler status is read-only; manual runs are operator-triggered and audited.
	r.Get(APIPrefix+"/server/scheduler/tasks", h.requireOperator(h.handleSchedulerTasks))
	r.Get(APIPrefix+"/server/scheduler/tasks/{task}", h.requireOperator(h.handleSchedulerTask))
	r.Get(APIPrefix+"/server/scheduler/tasks/{task}/history", h.requireOperator(h.handleSchedulerTaskHistory))
	r.Post(APIPrefix+"/server/scheduler/tasks/{task}/run", h.requireOperator(h.handleSchedulerTaskRun))

	// Owner-token issue/reveal/rotate/revoke operations consume the strict limiter,
	// including attempts that fail authentication or confirmation.
	r.Post(APIPrefix+"/server/preferences/tokens/primary", h.strictOwnerAction(h.ownerPrimaryIssue))
	r.Post(APIPrefix+"/server/preferences/tokens/primary/reveal", h.strictOwnerAction(h.ownerPrimaryReveal))
	r.Post(APIPrefix+"/server/preferences/tokens/primary/rotate", h.strictOwnerAction(h.ownerPrimaryRotate))
	r.Delete(APIPrefix+"/server/preferences/tokens/primary", h.strictOwnerAction(h.ownerPrimaryRevoke))
	r.Post(APIPrefix+"/server/preferences/tokens/{id}/reveal", h.strictOwnerAction(h.ownerLinkedReveal))
	r.Post(APIPrefix+"/server/preferences/tokens/{id}/rotate", h.strictOwnerAction(h.ownerLinkedRotate))
}

// handleSchedulerTasks handles GET /api/v1/server/scheduler/tasks.
// Returns all registered tasks with their last/next run status per docs/api.md.
func (h *Handler) handleSchedulerTasks(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running", r)
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
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running", r)
		return
	}
	task, err := h.scheduler.GetTask(scheduler.TaskID(chi.URLParam(r, "task")))
	if err != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", err.Error(), r)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: task})
}

// handleSchedulerTaskHistory returns recent persisted runs for one task.
func (h *Handler) handleSchedulerTaskHistory(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running", r)
		return
	}
	taskID := scheduler.TaskID(chi.URLParam(r, "task"))
	if _, err := h.scheduler.GetTask(taskID); err != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", err.Error(), r)
		return
	}
	history, err := h.scheduler.GetHistory(taskID, 20)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "HISTORY_UNAVAILABLE", err.Error(), r)
		return
	}
	if history == nil {
		history = []*scheduler.TaskHistoryEntry{}
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"history": history}})
}

// handleSchedulerTaskRun handles POST /api/v1/server/scheduler/tasks/{task}/run.
// Triggers an immediate run of the named task, per docs/api.md.
func (h *Handler) handleSchedulerTaskRun(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "task")
	if taskID == "" {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "missing task id", r)
		return
	}
	if h.scheduler == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "SCHEDULER_UNAVAILABLE", "scheduler is not running", r)
		return
	}
	if err := h.scheduler.RunNow(scheduler.TaskID(taskID)); err != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", fmt.Sprintf("task %q cannot be run: %v", taskID, err), r)
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

	// 5. Features - PUBLIC only (PARTS 19, 31)
	Features FeaturesInfo `json:"features"`

	// 6. Component health checks (per AI.md PART 13)
	Checks ChecksInfo `json:"checks"`

	// 7. Statistics (public-safe aggregates)
	Stats StatsInfo `json:"stats"`
}

// MarshalJSON fixes API health output to the canonical PART 13 field order.
func (health HealthResponse) MarshalJSON() ([]byte, error) {
	type orderedHealth HealthResponse
	return json.Marshal(orderedHealth(health))
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
	ActiveConns int           `json:"active_connections"`
	Extra       []health.Stat `json:"-"`
}

func (stats StatsInfo) MarshalJSON() ([]byte, error) {
	type ordered StatsInfo
	base, err := json.Marshal(ordered(stats))
	if err != nil {
		return nil, err
	}
	fields := make([]health.Field, 0, len(stats.Extra))
	for _, stat := range stats.Extra {
		fields = append(fields, health.Field{Key: stat.Key, Value: stat.Value})
	}
	return health.AppendFields(base, fields)
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
	I2P   string         `json:"i2p,omitempty"`
	Extra []health.Check `json:"-"`
}

func (checks ChecksInfo) MarshalJSON() ([]byte, error) {
	type ordered ChecksInfo
	base, err := json.Marshal(ordered(checks))
	if err != nil {
		return nil, err
	}
	fields := make([]health.Field, 0, len(checks.Extra))
	for _, check := range checks.Extra {
		fields = append(fields, health.Field{Key: check.Key, Value: check.Status})
	}
	return health.AppendFields(base, fields)
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
	// Full operator lifecycle state per AI.md PART 13 / PART 31.1.
	State string `json:"state"`
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
	State  string `json:"state"`
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
	Name          string                   `json:"name"`
	Version       string                   `json:"version"`
	API           AutodiscoverAPIInfo      `json:"api"`
	URLs          []string                 `json:"urls"`
	Onion         string                   `json:"onion,omitempty"`
	I2P           string                   `json:"i2p,omitempty"`
	CLIVersions   map[string]CLIBinaryInfo `json:"cli_versions"`
	CLIMinVersion string                   `json:"cli_min_version"`
}

// AutodiscoverAPIInfo describes the version prefix served by the API.
type AutodiscoverAPIInfo struct {
	CurrentVersion string `json:"current_version"`
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
	fmt.Fprintf(&text, "version: %s\ngo_version: %s\nbuild.commit: %s\nbuild.date: %s\nuptime: %s\nmode: %s\ntimestamp: %s\nfeatures.tor: %s\nfeatures.i2p: %s\nfeatures.geoip: %s\nchecks.database: %s\nchecks.cache: %s\nchecks.disk: %s\nchecks.scheduler: %s\n",
		health.Version, health.GoVersion, health.Build.Commit, health.Build.Date, health.Uptime,
		health.Mode, health.Timestamp,
		healthFeatureText(health.Features.Tor.Status, health.Features.Tor.Hostname),
		healthFeatureText(health.Features.I2P.Status, health.Features.I2P.Hostname),
		map[bool]string{true: "enabled", false: "disabled"}[health.Features.GeoIP], health.Checks.Database,
		health.Checks.Cache, health.Checks.Disk, health.Checks.Scheduler)
	if health.Checks.Tor != "" {
		fmt.Fprintf(&text, "checks.tor: %s\n", health.Checks.Tor)
	}
	if health.Checks.I2P != "" {
		fmt.Fprintf(&text, "checks.i2p: %s\n", health.Checks.I2P)
	}
	for _, check := range health.Checks.Extra {
		fmt.Fprintf(&text, "checks.%s: %s\n", check.Key, check.Status)
	}
	fmt.Fprintf(&text, "stats.requests_total: %d\nstats.requests_24h: %d\nstats.active_connections: %d\n", health.Stats.RequestsTotal, health.Stats.Requests24h, health.Stats.ActiveConns)
	for _, stat := range health.Stats.Extra {
		fmt.Fprintf(&text, "stats.%s: %d\n", stat.Key, stat.Value)
	}
	return text.String()
}

func healthFeatureText(status, hostname string) string {
	if status == "" {
		status = "disabled"
	}
	if hostname != "" {
		return status + " " + hostname
	}
	return status
}

// HealthForGraphQL returns the same public health payload as the REST endpoint.
func (h *Handler) HealthForGraphQL() HealthResponse {
	return h.buildHealthResponse()
}

func (h *Handler) buildHealthResponse() HealthResponse {
	// Tor status per AI.md PART 13 and PART 32
	torEnabled := h.config.Server.Tor.Enabled
	torRunning := false
	torStatus := "disabled"
	torHostname := ""
	torCheck := ""
	torState := health.TorState(false, false, false, "")

	// Check actual Tor service status per AI.md PART 32
	if torEnabled {
		torStarting := h.torService != nil && h.torService.IsStarting()
		actualRunning, hostname := false, ""
		if h.torService != nil && !torStarting {
			actualRunning = h.torService.IsRunning()
			if actualRunning {
				hostname = h.torService.GetOnionAddress()
			}
		}
		torState = health.TorState(true, torStarting, actualRunning, hostname)
		torEnabled, torRunning, torStatus, torHostname = torState.Enabled, torState.Running, torState.Status, torState.Hostname
		if torStatus == "unavailable" {
			torCheck = "degraded"
		} else {
			torCheck = "ok"
		}
	}

	// I2P status per AI.md PART 13 and PART 31.2
	i2pInfo := I2PInfo{
		Enabled:  false,
		Running:  false,
		Status:   "disabled",
		State:    "disabled",
		Hostname: "",
		Provider: "none",
	}
	i2pCheck := ""
	if h.config.I2PEnabled() && h.i2pManager != nil {
		lifecycle := h.i2pManager.LifecycleState()
		provider, hostname := "", ""
		if lifecycle != "starting" && lifecycle != "building" {
			provider = h.i2pManager.ProviderName()
			hostname = h.i2pManager.EepsiteAddress()
		}
		i2pState := health.I2PState(lifecycle, provider, hostname)
		i2pInfo = I2PInfo{Enabled: i2pState.Enabled, Running: i2pState.Running, Status: i2pState.Status, State: i2pState.State, Hostname: i2pState.Hostname, Provider: i2pState.Provider}
		if i2pInfo.Enabled && i2pInfo.Status == "unavailable" {
			i2pCheck = "degraded"
		} else if i2pInfo.Enabled {
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
			Date:   version.HealthBuildDate(),
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
				State:    torState.State,
				Status:   torStatus,
				Hostname: torHostname,
			},
			I2P:   i2pInfo,
			GeoIP: h.config.Server.GeoIP.Enabled,
		},
		// 7. Checks per AI.md PART 13
		Checks: ChecksInfo{
			Database:  "ok",
			Cache:     h.checkCacheHealth(),
			Disk:      h.checkDiskHealth(),
			Scheduler: h.checkSchedulerHealth(),
			Tor:       torCheck,
			I2P:       i2pCheck,
		},
		// 8. Stats
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
		health.Stats.Extra = append(health.Stats.Extra[:0], snapshot.StatsExtra...)
	} else {
		switch {
		case h.config.Server.MaintenanceMode:
			health.Status = "maintenance"
		case health.Checks.Database == "error" || health.Checks.Disk == "error" || health.Checks.Scheduler == "error":
			health.Status = "unhealthy"
		case health.Checks.Cache == "error" || health.Checks.Tor == "degraded" || health.Checks.I2P == "degraded":
			health.Status = "degraded"
		case h.config.PendingRestart():
			health.Status = "restart_required"
			health.PendingRestart = true
			health.RestartReason = h.config.RestartSettings()
		default:
			health.Status = "healthy"
		}
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
			Message: i18n.RequestString(r, "errors.method_not_allowed"),
		})
		return
	}

	serverConfig := h.config.Get()
	resp := AutodiscoverResponse{
		Name:          config.ProjectName,
		Version:       config.Version,
		API:           AutodiscoverAPIInfo{CurrentVersion: APIVersion},
		URLs:          []string{httputil.BuildURL(r, "")},
		CLIVersions:   h.cliBinaryVersions(),
		CLIMinVersion: serverConfig.Update.CLIMinVersion,
	}
	if serverConfig.Tor.Expose && h.torService != nil {
		resp.Onion = h.torService.GetOnionAddress()
	}
	if serverConfig.I2P.Expose && h.config.I2PEnabled() && h.i2pManager != nil {
		resp.I2P = h.i2pManager.EepsiteAddressFromDisk()
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: resp,
	})
}

func (h *Handler) cliBinaryDirectory() string {
	dir := strings.TrimSpace(h.config.CLI.BinaryDownload.Dir)
	if dir == "" {
		return filepath.Join(config.GetDataDir(), "cli")
	}
	return filepath.Clean(strings.ReplaceAll(dir, "{data_dir}", config.GetDataDir()))
}

func (h *Handler) cliBinaryVersions() map[string]CLIBinaryInfo {
	versions := make(map[string]CLIBinaryInfo)
	dir := h.cliBinaryDirectory()
	version, checksums, err := readCLIReleaseMetadata(dir)
	if err != nil {
		return versions
	}
	for _, platform := range []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64", "freebsd-amd64", "freebsd-arm64"} {
		filename := cliBinaryFilename(platform)
		file, _, digest, err := openVerifiedCLIBinary(dir, filename, checksums)
		if err != nil {
			if errors.Is(err, errCLIBinaryChecksumMismatch) {
				slog.Warn("CLI binary checksum does not match release metadata", "filename", filename)
			}
			continue
		}
		if err := file.Close(); err != nil {
			continue
		}
		versions[platform] = CLIBinaryInfo{Version: version, SHA256: digest}
	}
	return versions
}

var errCLIBinaryChecksumMismatch = errors.New("CLI binary checksum mismatch")

func cliBinaryFilename(platform string) string {
	filename := "search-cli-" + platform
	if strings.HasPrefix(platform, "windows-") {
		filename += ".exe"
	}
	return filename
}

func readCLIReleaseMetadata(dir string) (string, map[string]string, error) {
	versionBytes, err := os.ReadFile(filepath.Join(dir, "version.txt"))
	if err != nil {
		return "", nil, err
	}
	versionFields := strings.Fields(string(versionBytes))
	if len(versionFields) != 1 {
		return "", nil, errors.New("invalid CLI version metadata")
	}
	checksums, err := readCLIChecksums(filepath.Join(dir, "sha256.txt"))
	if err != nil {
		return "", nil, err
	}
	return versionFields[0], checksums, nil
}

func openVerifiedCLIBinary(dir, filename string, checksums map[string]string) (*os.File, os.FileInfo, string, error) {
	file, err := os.Open(filepath.Join(dir, filename))
	if err != nil {
		return nil, nil, "", err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, "", errors.Join(err, file.Close())
	}
	if !info.Mode().IsRegular() {
		return nil, nil, "", errors.Join(errors.New("CLI binary is not a regular file"), file.Close())
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, nil, "", errors.Join(err, file.Close())
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(checksums[filename], digest) {
		return nil, nil, "", errors.Join(errCLIBinaryChecksumMismatch, file.Close())
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, nil, "", errors.Join(err, file.Close())
	}
	return file, info, digest, nil
}

func readCLIChecksums(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer closeutil.Close(file)
	checksums := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || len(fields[0]) != sha256.Size*2 {
			continue
		}
		filename := filepath.Base(fields[len(fields)-1])
		if filename == fields[len(fields)-1] {
			checksums[filename] = fields[0]
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return checksums, nil
}

func (h *Handler) handleCLIBinary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "method not allowed", "", r)
		return
	}
	if h.config.CLI.BinaryDownload.RequireAuth {
		if err := h.operatorAuthError(r); err != nil {
			h.writeOperatorAuthError(w, r, err)
			return
		}
	}
	filename := chi.URLParam(r, "filename")
	if strings.Contains(filename, "/") || strings.Contains(filename, `\\`) {
		h.errorResponse(w, http.StatusNotFound, "not found", "", r)
		return
	}
	platformKey := ""
	for _, platform := range []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64", "freebsd-amd64", "freebsd-arm64"} {
		if filename == cliBinaryFilename(platform) {
			platformKey = platform
			break
		}
	}
	if platformKey == "" {
		h.errorResponse(w, http.StatusNotFound, "not found", "", r)
		return
	}
	dir := h.cliBinaryDirectory()
	version, checksums, err := readCLIReleaseMetadata(dir)
	if err != nil {
		h.errorResponse(w, http.StatusNotFound, "not found", "", r)
		return
	}
	file, info, digest, err := openVerifiedCLIBinary(dir, filename, checksums)
	if err != nil {
		if errors.Is(err, errCLIBinaryChecksumMismatch) {
			slog.Warn("CLI binary checksum does not match release metadata", "filename", filename)
		}
		h.errorResponse(w, http.StatusNotFound, "not found", "", r)
		return
	}
	defer closeutil.Close(file)
	if h.auditLogger != nil {
		h.auditLogger.LogEventQuiet(logging.AuditAction("cli.update_started"), logging.AuditCategorySystem, logging.AuditSeverityInfo,
			logging.AuditActor{Type: "anonymous", IP: httputil.GetClientIP(r)}, &logging.AuditTarget{Type: "cli_binary", ID: filename}, "success",
			map[string]interface{}{"platform": platformKey, "version": version, "sha256": digest}, "")
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, info.ModTime(), file)
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
			"rate_limit": true, "image_proxy": h.config.Server.ImageProxy.Enabled,
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
			h.errorResponse(w, http.StatusBadRequest, "Invalid JSON body", err.Error(), r)
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
		h.errorResponse(w, http.StatusBadRequest, "Invalid request parameters", err.Error(), r)
		return
	}
	if err := validateSearchDateRange(req); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request parameters", err.Error(), r)
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
			h.errorResponse(w, http.StatusBadRequest, "Search failed", err.Error(), r)
		case errors.Is(err, model.ErrNoEngines):
			h.errorResponse(w, http.StatusServiceUnavailable, "Search failed", err.Error(), r)
		default:
			h.errorResponse(w, http.StatusBadGateway, "Search failed", err.Error(), r)
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

	// Offer local search features before forwarding a query to the suggestion provider.
	suggestions := h.localAutocompleteSuggestions(query)
	if len(suggestions) < maximumAutocompleteSuggestions {
		for _, suggestion := range h.fetchAutocompleteSuggestions(r.Context(), query) {
			if !containsString(suggestions, suggestion) {
				suggestions = append(suggestions, suggestion)
			}
			if len(suggestions) == maximumAutocompleteSuggestions {
				break
			}
		}
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: suggestions,
		Meta: &APIMeta{Version: APIVersion},
	})
}

func (h *Handler) localAutocompleteSuggestions(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || strings.HasPrefix(query, "!") {
		return nil
	}
	features := []string{
		"define: ", "synonym: ", "antonym: ", "calculate: ",
		"weather ", "time ", "currency ", "what is ",
	}
	if h != nil && h.directManager != nil {
		for _, answerType := range h.directManager.GetHandlerTypes() {
			features = append(features, string(answerType)+": ")
		}
	}
	sort.Strings(features)
	result := make([]string, 0, maximumAutocompleteSuggestions)
	seen := make(map[string]bool)
	for _, feature := range features {
		if strings.HasPrefix(strings.ToLower(feature), query) && !seen[feature] {
			result = append(result, feature)
			seen[feature] = true
			if len(result) == maximumAutocompleteSuggestions {
				break
			}
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
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
		if unicode.IsControl(character) {
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
		h.errorResponse(w, http.StatusNotFound, "Engine not found", fmt.Sprintf("No engine with ID: %s", id), r)
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
func (h *Handler) errorResponse(w http.ResponseWriter, status int, message, detail string, r *http.Request) {
	// Get request ID from response header (set by middleware)
	requestID := w.Header().Get("X-Request-ID")

	// Log the internal detail for operators — never exposed in the API response
	// (Tier 3 per AI.md PART 11: internal error details are debug-only)
	if detail != "" && status >= 500 {
		slog.Error("API error", "request_id", requestID, "message", message, "detail", detail)
	}

	localizedMessage := i18n.RequestString(r, apiErrorTranslationKey(status, message))

	h.jsonResponse(w, status, &APIResponse{
		OK:      false,
		Error:   model.ErrorCodeFromHTTP(status),
		Message: localizedMessage,
		Meta: &APIMeta{
			RequestID: requestID,
			Version:   APIVersion,
		},
	})
}

func apiErrorTranslationKey(status int, messages ...string) string {
	if len(messages) > 0 {
		message := strings.TrimSpace(messages[0])
		code := strings.ToUpper(message)
		switch code {
		case "VALIDATION_FAILED":
			return "errors.validation_failed"
		case "REQUEST_TOO_LARGE":
			return "errors.request_too_large"
		case "SCHEDULER_UNAVAILABLE":
			return "errors.scheduler_unavailable"
		case "HISTORY_UNAVAILABLE":
			return "errors.history_unavailable"
		case "OWNER_TOKENS_UNAVAILABLE":
			return "errors.owner_tokens_unavailable"
		case "TOKEN_LIMIT":
			return "errors.token_limit"
		case "DUPLICATE":
			return "errors.duplicate"
		case "DELIVERY_UNAVAILABLE":
			return "errors.delivery_unavailable"
		}
		switch strings.ToLower(message) {
		case "search failed":
			return "search.error_message"
		case "invalid credentials":
			return "errors.invalid_credentials"
		case "widget type required", "query parameter is required", "term is required":
			return "errors.required"
		case "widget not available", "engine not found", "preset not found", "not found", "token not found":
			return "errors.not_found"
		case "method not allowed":
			return "errors.method_not_allowed"
		case "invalid ip address":
			return "errors.bad_request"
		case "too many contact submissions":
			return "errors.rate_limit"
		}
	}

	switch status {
	case http.StatusUnauthorized:
		return "errors.unauthorized"
	case http.StatusForbidden:
		return "errors.forbidden"
	case http.StatusNotFound, http.StatusGone:
		return "errors.not_found"
	case http.StatusMethodNotAllowed:
		return "errors.method_not_allowed"
	case http.StatusTooManyRequests:
		return "errors.rate_limit"
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return "errors.server_error_message"
	default:
		return "errors.bad_request"
	}
}

func (h *Handler) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	return strings.Join(parts, " ")
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
		return 1
	}
	if active := int(h.healthMetrics.GetActiveConnections()); active > 0 {
		return active
	}
	return 1
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
	URL         string   `json:"url,omitempty"`
	Engine      string   `json:"engine,omitempty"`
	Category    string   `json:"category"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

func (h *Handler) handleBangs(w http.ResponseWriter, r *http.Request) {
	// Return built-in bangs list
	// Custom bangs are stored client-side in localStorage

	bangs := h.AvailableBangs()

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

// AvailableBangs returns built-in bangs whose mapped local engines can serve searches.
func (h *Handler) AvailableBangs() []BangInfo {
	all := getBuiltinBangs()
	available := make([]BangInfo, 0, len(all))
	for _, item := range all {
		if item.Engine != "" && !h.localEngineAvailable(item.Engine) {
			continue
		}
		available = append(available, item)
	}
	return available
}

func (h *Handler) localEngineAvailable(name string) bool {
	if h == nil || h.registry == nil {
		return false
	}
	configured, err := h.registry.Get(name)
	if err != nil || !configured.IsEnabled() {
		return false
	}
	if health, ok := configured.(interface{ CanSearch(time.Time) bool }); ok && !health.CanSearch(time.Now()) {
		return false
	}
	return true
}

func getBuiltinBangs() []BangInfo {
	definitions := searchbang.Builtins()
	bangs := make([]BangInfo, 0, len(definitions))
	for _, item := range definitions {
		bangs = append(bangs, BangInfo{
			Shortcut:    item.Shortcut,
			Name:        item.Name,
			URL:         item.URL,
			Engine:      item.Engine,
			Category:    item.Category,
			Description: item.Description,
			Icon:        item.Icon,
			Aliases:     append([]string(nil), item.Aliases...),
		})
	}
	return bangs
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
		h.errorResponse(w, http.StatusBadRequest, "Query parameter is required", "", r)
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
	if c, err := r.Cookie("search_pref_weather_units"); err == nil && (c.Value == "auto" || c.Value == "metric" || c.Value == "imperial") {
		weatherUnits = c.Value
	}
	ctx = instant.WithUnits(ctx, units)
	ctx = instant.WithWeatherUnits(ctx, weatherUnits)

	answer, err := h.instantManager.Process(ctx, query)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to process instant answer", err.Error(), r)
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
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET requests are supported", r)
		return
	}

	// Parse path: /api/{api_version}/direct/{type}/{term}
	path := strings.TrimPrefix(r.URL.Path, APIPrefix+"/direct/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request", "URL format: /api/v1/direct/{type}/{term}", r)
		return
	}

	answerType := direct.AnswerType(parts[0])
	term, err := url.PathUnescape(parts[1])
	if err != nil {
		term = parts[1]
	}
	term = strings.TrimSpace(term)

	if term == "" {
		h.errorResponse(w, http.StatusBadRequest, "Invalid request", "Term is required", r)
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
		h.errorResponse(w, http.StatusInternalServerError, "Failed to process direct answer", err.Error(), r)
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
		h.errorResponse(w, http.StatusBadRequest, "Query parameter is required", "", r)
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
		h.errorResponse(w, http.StatusInternalServerError, "Failed to fetch related searches", err.Error(), r)
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
	Format      string            `json:"format,omitempty"`
	Sections    []PageSection     `json:"sections,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Pages       []HelpPageLink    `json:"pages,omitempty"`
}

func resolveBrandingPageContent(entry config.BrandingPageContent, language string) string {
	switch body := entry.Body.(type) {
	case string:
		return body
	case map[string]any:
		if value, ok := body[language].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
		if value, ok := body["default"].(string); ok {
			return value
		}
	case map[any]any:
		if value, ok := body[language].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
		if value, ok := body["default"].(string); ok {
			return value
		}
	}
	return ""
}

func (h *Handler) expandBrandingPageVariables(body string) string {
	values := map[string]string{
		"{current_year}":    fmt.Sprint(time.Now().Year()),
		"{project_name}":    "search",
		"{project_org}":     "apimgr",
		"{project_version}": version.Version,
		"{build_datetime}":  version.HealthBuildDate(),
	}
	if h.torService != nil {
		values["{onion_address}"] = h.torService.GetOnionAddress()
	}
	if h.i2pManager != nil {
		values["{i2p_address}"] = h.i2pManager.EepsiteAddressFromDisk()
	}
	for placeholder, value := range values {
		body = strings.ReplaceAll(body, placeholder, value)
	}
	return body
}

func (h *Handler) brandingPageContent(name, builtIn string, r *http.Request) (string, string) {
	entry, ok := h.config.Server.Branding.Content[name]
	if !ok {
		return builtIn, "markdown"
	}
	body := h.expandBrandingPageVariables(resolveBrandingPageContent(entry, i18n.RequestLanguage(r)))
	format := strings.ToLower(strings.TrimSpace(entry.Format))
	switch format {
	case "html":
		return body, "html"
	case "txt", "text":
		return body, "text"
	case "", "md", "markdown":
		return body, "markdown"
	default:
		return builtIn, "markdown"
	}
}

type HelpPageLink struct {
	Title string `json:"title"`
	Path  string `json:"path"`
}

func helpPageLinks() []HelpPageLink {
	pages := hubpages.Help(true)
	links := make([]HelpPageLink, 0, len(pages))
	for _, page := range pages {
		links = append(links, HelpPageLink{Title: page.Title, Path: page.Path})
	}
	return links
}

type ServerDPOResponse struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	PrivacyURL string `json:"privacy_url"`
}

// handleServerDPO returns the configured public DPO contact, falling back to
// the general public contact address and never exposing the admin address.
func (h *Handler) handleServerDPO(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported", r)
		return
	}
	name := strings.TrimSpace(h.config.Server.Contact.DPO.Name)
	email := strings.TrimSpace(h.config.Server.Contact.DPO.Email)
	if email == "" {
		email = strings.TrimSpace(h.config.Server.Contact.General.Email)
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerDPOResponse{
			Name:       name,
			Email:      email,
			PrivacyURL: httputil.BuildURL(r, "/server/privacy"),
		},
		Meta: &APIMeta{Version: APIVersion},
	})
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
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported", r)
		return
	}

	appName := strings.TrimSpace(h.config.Server.Branding.Title)
	if appName == "" {
		appName = strings.TrimSpace(h.config.Server.Title)
	}
	if appName == "" {
		appName = "Search"
	}
	tagline := strings.TrimSpace(h.config.Server.Branding.Tagline)
	if tagline == "" {
		tagline = "Privacy-respecting metasearch"
	}
	description := strings.TrimSpace(h.config.Server.Branding.Description)
	if description == "" {
		description = "Search aggregates results directly from primary and specialized search engines without tracking users."
	}
	builtIn := fmt.Sprintf("# %s\n\n%s\n\n## About\n\n%s\n\n## Features\n\n- Private metasearch across primary and specialized sources.\n- Search categories, bangs, instant answers and direct answers.\n- Server-rendered pages that work without JavaScript.\n- Optional accountless alerts and private RSS feeds.\n\n## Links\n\n- [Project site](https://scour.li)\n- [Source code](https://github.com/apimgr/search)\n- [Documentation](https://search.readthedocs.io)\n", appName, tagline, description)
	if h.config.Server.GeoIP.Enabled {
		builtIn += "\n## Third-party attribution\n\n[IP Geolocation by DB-IP](https://db-ip.com/)\n\n" + geoIPAttributionText + "\n"
	}
	content, format := h.brandingPageContent("about", builtIn, r)
	metadata := map[string]string{
		"version":    config.Version,
		"build_date": config.BuildDate,
		"commit":     config.CommitID,
	}
	if h.config.Server.Tor.Enabled {
		metadata["tor_enabled"] = "true"
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerPageResponse{
			Title:    "About " + appName,
			Content:  content,
			Format:   format,
			Metadata: metadata,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerPrivacy handles GET /api/v1/server/privacy
// Per AI.md PART 16: Returns privacy policy as JSON
func (h *Handler) handleServerPrivacy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported", r)
		return
	}

	privacy := h.config.Server.Privacy
	analyticsDescription := privacy.AnalyticsDescription()
	retentionPeriod := privacy.Retention.PeriodForLanguage(i18n.RequestLanguage(r))
	if strings.TrimSpace(retentionPeriod) == "" {
		retentionPeriod = i18n.RequestString(r, "privacy.retention.period")
	}
	trackingType := strings.TrimSpace(h.config.Server.Tracking.Type)
	trackingEnabled := trackingType != "" && !strings.EqualFold(trackingType, "none")
	trackingNames := map[string]string{
		"google": "Google Analytics", "matomo": "Matomo", "piwik": "Piwik",
		"owa": "Open Web Analytics", "fathom": "Fathom", "plausible": "Plausible",
		"umami": "Umami", "simple": "Simple Analytics", "cloudflare": "Cloudflare Web Analytics",
	}
	trackingName := trackingNames[strings.ToLower(trackingType)]
	if trackingName == "" && trackingEnabled {
		trackingName = trackingType
	}
	type sharingItem struct {
		Condition string `json:"condition"`
		When      string `json:"when"`
		Data      string `json:"data"`
	}
	type serviceItem struct {
		Name      string `json:"name"`
		Purpose   string `json:"purpose"`
		DataSent  string `json:"data_sent"`
		PolicyURL string `json:"policy_url"`
	}
	services := make([]serviceItem, 0, len(privacy.ThirdParty.Services))
	for _, service := range privacy.ThirdParty.Services {
		services = append(services, serviceItem{
			Name: service.Name, Purpose: service.Purpose, DataSent: service.DataSent, PolicyURL: service.PolicyURL,
		})
	}
	if services == nil {
		services = []serviceItem{}
	}
	sharing := make([]sharingItem, 0, len(privacy.Data.Sharing))
	for _, item := range privacy.Data.Sharing {
		sharing = append(sharing, sharingItem{Condition: item.Condition, When: item.When, Data: item.Data})
	}
	if sharing == nil {
		sharing = []sharingItem{}
	}
	userOptedOut := false
	if cookie, err := r.Cookie("ccpa_opt_out"); err == nil && cookie.Value == "true" {
		userOptedOut = true
	}
	data := map[string]any{
		"summary": map[string]any{
			"data_stored_on_server": privacy.Data.StoredOnServer,
			"data_sold":            privacy.Data.Sold,
			"user_control":         true,
		},
		"cookies": map[string]any{
			"essential": map[string]any{"enabled": privacy.Cookies.Essential.Enabled, "description": privacy.Cookies.Essential.Description},
			"preferences": map[string]any{"enabled": privacy.Cookies.Preferences.Enabled, "description": privacy.Cookies.Preferences.Description},
			"analytics": map[string]any{"enabled": privacy.Cookies.Analytics.Enabled, "description": analyticsDescription},
		},
		"data": map[string]any{
			"sold": privacy.Data.Sold, "stored_on_server": privacy.Data.StoredOnServer, "sharing": sharing,
		},
		"tracking": map[string]any{
			"enabled": trackingEnabled, "type": trackingType, "type_name": trackingName,
		},
		"retention": map[string]any{
			"period": retentionPeriod,
			"export_available": privacy.Retention.ExportAvailable,
			"deletion_available": privacy.Retention.DeletionAvailable,
		},
		"third_party": map[string]any{"services": services},
		"ccpa": map[string]any{
			"applicable": privacy.IsCCPAApplicable(), "opt_out_url": "/server/privacy#ccpa-opt-out", "user_opted_out": userOptedOut,
		},
		"content": map[string]any{
			"consent_message": privacy.ConsentMessage(), "data_usage": privacy.DataUsageContent(),
		},
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: data,
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerHelp handles GET /api/v1/server/help
// Per AI.md PART 16: Returns help documentation as JSON
func (h *Handler) handleServerHelp(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported", r)
		return
	}

	appName := h.config.Server.Branding.Title
	if appName == "" {
		appName = h.config.Server.Title
	}

	sections := []PageSection{
		{
			ID:      "getting_started",
			Title:   i18n.RequestString(r, "help.getting_started.heading"),
			Content: i18n.RequestFormat(r, "help.getting_started.intro_prefix", "app_name", appName) + " Enter " + i18n.RequestString(r, "help.getting_started.intro_suffix"),
		},
		{
			ID:    "categories",
			Title: i18n.RequestString(r, "help.categories.heading"),
			Content: i18n.RequestString(r, "help.categories.intro") + " " + strings.Join([]string{
				i18n.RequestString(r, "common.categories.general"),
				i18n.RequestString(r, "common.categories.images"),
				i18n.RequestString(r, "common.categories.videos"),
				i18n.RequestString(r, "common.categories.news"),
				i18n.RequestString(r, "common.categories.maps"),
				i18n.RequestString(r, "common.categories.files"),
				i18n.RequestString(r, "common.categories.it"),
				i18n.RequestString(r, "common.categories.science"),
				i18n.RequestString(r, "common.categories.social"),
			}, ", ") + ".",
		},
		{
			ID:      "bangs",
			Title:   i18n.RequestString(r, "help.toc.bang_commands"),
			Content: i18n.RequestString(r, "help.bangs.intro_prefix") + " `!shortcut terms` " + i18n.RequestString(r, "help.bangs.intro_or") + " `terms !shortcut`" + i18n.RequestString(r, "help.bangs.intro_suffix"),
		},
		{
			ID:      "answers",
			Title:   i18n.RequestString(r, "help.direct.heading"),
			Content: i18n.RequestString(r, "help.direct.intro_da_label") + " " + i18n.RequestString(r, "help.direct.intro_da_text_prefix") + " `type:term` " + i18n.RequestString(r, "help.direct.intro_da_text_suffix") + ". " + i18n.RequestString(r, "help.direct.intro_ia_label") + " " + i18n.RequestString(r, "help.direct.intro_ia_text"),
		},
		{
			ID:      "search_tips",
			Title:   i18n.RequestString(r, "help.operators.heading"),
			Content: i18n.RequestString(r, "help.operators.intro") + " `\"phrase\"`, `-term`, `OR`, `AND`, `*`, `site:`, `-site:`, `filetype:`, `intitle:`, `inurl:`, `intext:`, `related:`, `cache:`, `info:`, `before:`, `after:`, `define:`, `weather:`, `stocks:`, `map:`, `movie:`, `source:`.",
		},
		{
			ID:      "keyboard_shortcuts",
			Title:   i18n.RequestString(r, "help.shortcuts.heading"),
			Content: i18n.RequestString(r, "help.shortcuts.intro") + " `/`, `Escape`, `t`, `?`, `j`, `k`, `Enter`, `o`, `h`, `l`, `gg`, `G`, `1-9`.",
		},
		{
			ID:    "api_documentation",
			Title: i18n.RequestString(r, "help.toc.api_docs"),
			Content: i18n.RequestString(r, "about.api_docs_description") + " " + strings.Join([]string{
				"`" + APIPrefix + "/search`", "`" + APIPrefix + "/instant`", "`" + APIPrefix + "/direct/{type}/{term}`",
				"`" + APIPrefix + "/server/help`", "`" + APIPrefix + "/server/about`", "`/api/graphql`", "`/api/swagger`",
			}, ", ") + ".",
		},
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerPageResponse{
			Title:       i18n.RequestString(r, "help.title"),
			Description: i18n.RequestFormat(r, "help.subtitle", "app_name", appName),
			Content:     h.config.Server.Pages.Help.Content,
			Sections:    sections,
			Pages:       helpPageLinks(),
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerTerms handles GET /api/v1/server/terms
// Per AI.md PART 16: Returns terms of service as JSON
func (h *Handler) handleServerTerms(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported", r)
		return
	}

	appName := strings.TrimSpace(h.config.Server.Branding.Title)
	if appName == "" {
		appName = strings.TrimSpace(h.config.Server.Title)
	}
	if appName == "" {
		appName = "Search"
	}
	builtIn := fmt.Sprintf("## Acceptance\n\nUsing %s means accepting these terms. Stop using it if you do not agree.\n\n## Acceptable use\n\nDo not use the service unlawfully, abuse other people, bypass rate limits or access controls, or test beyond the security policy at [/server/security/policy](/server/security/policy).\n\n## Liability\n\nThe service is provided as is, without warranty of any kind. To the extent the law allows, the operator is not liable for loss arising from its use.\n\n## Changes\n\nThese terms may change. The current text on this page applies from the moment it is published.\n\n## Governing law\n\nThe laws of the United States apply, without regard to conflict-of-law rules.\n", appName)
	content, format := h.brandingPageContent("terms", builtIn, r)
	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: ServerPageResponse{
			Title:   "Terms of Service",
			Content: content,
			Format:  format,
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
	contactEnabled := h.config.Server.Pages.Contact.Enabled &&
		(h.config.Server.Contact.General.Email != "" || h.config.Server.Contact.Admin.Email != "" ||
			h.config.Server.Contact.Abuse.Email != "" || h.config.Server.Contact.DPO.Email != "" ||
			notify.HasConfiguredWebhook(h.config.Server.Contact, "general") || notify.HasConfiguredWebhook(h.config.Server.Contact, "abuse"))
	if !h.config.Server.Pages.Contact.Enabled {
		h.errorResponse(w, http.StatusNotFound, "Contact form is not enabled", "", r)
		return
	}

	if r.Method == http.MethodGet {
		cfg := h.config.Get()
		abuseEmail := cfg.Contact.Abuse.Email
		if abuseEmail == "" {
			abuseEmail = cfg.Contact.General.Email
		}
		fields := []map[string]interface{}{
			{"name": "name", "type": "text", "required": true, "max_length": 100},
			{"name": "email", "type": "email", "required": true, "max_length": 254},
			{"name": "captcha", "type": "captcha", "required": true, "max_length": nil},
			{"name": "form_token", "type": "hidden", "required": true, "max_length": 100},
		}
		securityID := strings.TrimSpace(r.URL.Query().Get("security_id"))
		securityMode := securityID != "" && h.securityContact != nil && h.securityContact.ValidContactSecurityID(r, securityID)
		if securityMode {
			fields = append(fields,
				map[string]interface{}{"name": "type", "type": "enum", "required": true, "values": []string{"security"}},
				map[string]interface{}{"name": "researcher_gpg", "type": "text", "required": false, "max_length": 16384, "max_length_when": []map[string]interface{}{{"prefix": "https://", "max_length": 2048}}},
				map[string]interface{}{"name": "affected_component", "type": "text", "required": true, "max_length": 255},
				map[string]interface{}{"name": "affected_endpoint", "type": "text", "required": false, "max_length": 2048},
				map[string]interface{}{"name": "severity", "type": "enum", "required": true, "max_length": nil, "values": []string{"critical", "high", "medium", "low", "informational"}},
				map[string]interface{}{"name": "summary", "type": "text", "required": true, "max_length": 200},
				map[string]interface{}{"name": "steps_to_reproduce", "type": "textarea", "required": true, "max_length": 10000},
				map[string]interface{}{"name": "impact", "type": "textarea", "required": true, "max_length": 5000},
				map[string]interface{}{"name": "suggested_fix", "type": "textarea", "required": false, "max_length": 5000},
				map[string]interface{}{"name": "cve_requested", "type": "checkbox", "required": false, "max_length": nil},
				map[string]interface{}{"name": "disclosure_days", "type": "number", "required": false, "max_length": nil},
				map[string]interface{}{"name": "credit_preference", "type": "enum", "required": true, "max_length": nil, "values": []string{"real_name", "handle", "no", "anonymous"}},
				map[string]interface{}{"name": "credit_handle", "type": "text", "required": false, "max_length": 100, "required_when": map[string]string{"field": "credit_preference", "equals": "handle"}},
				map[string]interface{}{"name": "agreed_to_disclosure", "type": "checkbox", "required": true, "max_length": nil},
			)
		} else {
			fields = append(fields,
				map[string]interface{}{"name": "type", "type": "enum", "required": true, "values": []string{"abuse", "dmca", "privacy"}},
				map[string]interface{}{"name": "reported_url", "type": "url", "required": false, "required_when": []map[string]string{{"field": "type", "in": "abuse,dmca"}}, "max_length": 2048},
				map[string]interface{}{"name": "reason", "type": "enum", "required": false, "required_when": []map[string]string{{"field": "type", "equals": "abuse"}}, "values": []string{"illegal_content", "terms_violation", "harassment", "malware_phishing", "other"}},
				map[string]interface{}{"name": "copyrighted_work", "type": "text", "required": false, "required_when": []map[string]string{{"field": "type", "equals": "dmca"}}},
				map[string]interface{}{"name": "good_faith_statement", "type": "checkbox", "required": false, "required_when": []map[string]string{{"field": "type", "equals": "dmca"}}},
				map[string]interface{}{"name": "accuracy_statement", "type": "checkbox", "required": false, "required_when": []map[string]string{{"field": "type", "equals": "dmca"}}},
				map[string]interface{}{"name": "signature", "type": "text", "required": false, "required_when": []map[string]string{{"field": "type", "equals": "dmca"}}},
				map[string]interface{}{"name": "request_kind", "type": "enum", "required": false, "required_when": []map[string]string{{"field": "type", "equals": "privacy"}}, "values": []string{"access", "export", "deletion", "correction", "opt_out", "other"}},
				map[string]interface{}{"name": "subject", "type": "text", "required": true, "max_length": 200}, map[string]interface{}{"name": "message", "type": "textarea", "required": true, "max_length": 5000})
		}
		captcha := h.contactCaptchaInfo(r)
		fields = append(fields, map[string]interface{}{"name": "website", "type": "text", "required": false, "max_length": 200, "must_be_empty": true})
		types := []string{"abuse", "dmca", "privacy"}
		if securityMode {
			types = []string{"security"}
		}
		formToken := h.contactFormToken(r)
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: map[string]interface{}{
				"enabled": contactEnabled, "title": "Contact", "fields": fields, "captcha": captcha,
				"security_url": httputil.BuildURL(r, "/server/security"), "abuse_email": abuseEmail,
				"security_mode": securityMode, "form_token": formToken, "types": types,
			},
		})
		return
	}

	if r.Method != http.MethodPost {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", r.Method, r)
		return
	}
	if !contactEnabled {
		h.errorResponse(w, http.StatusNotFound, "Contact form is not enabled", "", r)
		return
	}
	var body struct {
		Name               string `json:"name"`
		Email              string `json:"email"`
		Subject            string `json:"subject"`
		Message            string `json:"message"`
		Type               string `json:"type"`
		ReportedURL        string `json:"reported_url"`
		Reason             string `json:"reason"`
		CopyrightedWork    string `json:"copyrighted_work"`
		GoodFaithStatement bool   `json:"good_faith_statement"`
		AccuracyStatement  bool   `json:"accuracy_statement"`
		Signature          string `json:"signature"`
		RequestKind        string `json:"request_kind"`
		Step               string `json:"step"`
		ReviewToken        string `json:"review_token"`
		Confirmed          bool   `json:"confirmed"`
		FormToken          string `json:"form_token"`
		CaptchaID          string `json:"captcha_id"`
		CaptchaAnswer      string `json:"captcha_answer"`
		CaptchaResponse    string `json:"captcha_response"`
		SecurityID         string `json:"security_id"`
		ResearcherGPG      string `json:"researcher_gpg"`
		AffectedComponent  string `json:"affected_component"`
		AffectedEndpoint   string `json:"affected_endpoint"`
		Severity           string `json:"severity"`
		Summary            string `json:"summary"`
		StepsToReproduce   string `json:"steps_to_reproduce"`
		Impact             string `json:"impact"`
		SuggestedFix       string `json:"suggested_fix"`
		CVERequested       bool   `json:"cve_requested"`
		DisclosureDays     int    `json:"disclosure_days"`
		CreditPreference   string `json:"credit_preference"`
		CreditHandle       string `json:"credit_handle"`
		AgreedToDisclosure bool   `json:"agreed_to_disclosure"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256<<10)
	requestBody, readErr := io.ReadAll(r.Body)
	if readErr != nil || !utf8.Valid(requestBody) {
		h.recordContactBlock("shape", r)
		h.writeError(w, r, "BAD_REQUEST", i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), http.StatusBadRequest)
		return
	}
	var suppliedFields map[string]json.RawMessage
	if err := json.Unmarshal(requestBody, &suppliedFields); err != nil {
		h.recordContactBlock("shape", r)
		h.writeError(w, r, "BAD_REQUEST", i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), http.StatusBadRequest)
		return
	}
	allowedFields := map[string]bool{"name": true, "email": true, "subject": true, "message": true, "type": true, "reported_url": true, "reason": true, "copyrighted_work": true, "good_faith_statement": true, "accuracy_statement": true, "signature": true, "request_kind": true, "step": true, "review_token": true, "confirmed": true, "form_token": true, "captcha_id": true, "captcha_answer": true, "captcha_response": true, "g-recaptcha-response": true, "h-captcha-response": true, "security_id": true, "researcher_gpg": true, "affected_component": true, "affected_endpoint": true, "severity": true, "summary": true, "steps_to_reproduce": true, "impact": true, "suggested_fix": true, "cve_requested": true, "disclosure_days": true, "credit_preference": true, "credit_handle": true, "agreed_to_disclosure": true}
	for name := range suppliedFields {
		if !allowedFields[name] {
			h.recordContactBlock("shape", r)
			h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": name, "rule": "unknown"}})
			return
		}
	}
	if err := json.Unmarshal(requestBody, &body); err != nil {
		h.writeError(w, r, "BAD_REQUEST", i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), http.StatusBadRequest)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.TrimSpace(body.Email)
	body.FormToken = strings.TrimSpace(body.FormToken)
	body.Subject = strings.TrimSpace(body.Subject)
	body.Message = strings.TrimSpace(body.Message)
	body.Type = strings.ToLower(strings.TrimSpace(body.Type))
	body.ReportedURL = strings.TrimSpace(body.ReportedURL)
	body.Reason = strings.ToLower(strings.TrimSpace(body.Reason))
	body.CopyrightedWork = strings.TrimSpace(body.CopyrightedWork)
	body.Signature = strings.TrimSpace(body.Signature)
	body.RequestKind = strings.ToLower(strings.TrimSpace(body.RequestKind))
	body.Step = strings.ToLower(strings.TrimSpace(body.Step))
	body.ReviewToken = strings.TrimSpace(body.ReviewToken)
	body.SecurityID = strings.TrimSpace(body.SecurityID)
	body.ResearcherGPG = strings.TrimSpace(body.ResearcherGPG)
	body.AffectedComponent = strings.TrimSpace(body.AffectedComponent)
	body.AffectedEndpoint = strings.TrimSpace(body.AffectedEndpoint)
	body.Severity = strings.ToLower(strings.TrimSpace(body.Severity))
	body.Summary = strings.TrimSpace(body.Summary)
	body.StepsToReproduce = strings.TrimSpace(body.StepsToReproduce)
	body.Impact = strings.TrimSpace(body.Impact)
	body.SuggestedFix = strings.TrimSpace(body.SuggestedFix)
	body.CreditPreference = strings.ToLower(strings.TrimSpace(body.CreditPreference))
	body.CreditHandle = strings.TrimSpace(body.CreditHandle)
	securityMode := body.SecurityID != "" && h.securityContact != nil && h.securityContact.ValidContactSecurityID(r, body.SecurityID)
	step := body.Step
	if step == "" {
		step = "submit"
	}
	if step != "submit" && step != "confirm" {
		h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED"))})
		return
	}
	formValues := make(url.Values)
	for name, raw := range suppliedFields {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			value = strings.Trim(string(raw), "\"")
		}
		formValues.Set(name, value)
	}
	if antispam, ok := h.contactSubmitter.(interface {
		ValidateContactShape(*http.Request, url.Values, bool, bool) string
	}); ok {
		if reason := antispam.ValidateContactShape(r, formValues, securityMode, true); reason != "" {
			if blocked, ok := h.contactSubmitter.(interface{ RecordContactBlock(string, *http.Request) }); ok {
				blocked.RecordContactBlock(reason, r)
			}
			h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": "contact", "rule": reason}})
			return
		}
	} else if body.Name == "" || body.Email == "" || body.FormToken == "" || (!securityMode && (body.Subject == "" || body.Message == "")) {
		h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": "contact", "rule": "required"}})
		return
	}
	if !securityMode {
		if validator, ok := h.contactSubmitter.(interface {
			ValidateContactReport(*http.Request, url.Values) bool
		}); ok && !validator.ValidateContactReport(r, formValues) {
			h.recordContactBlock("content", r)
			h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": "contact", "rule": "invalid"}})
			return
		}
	}
	if step == "confirm" {
		validator, ok := h.contactSubmitter.(interface {
			ValidateContactReviewToken(url.Values, string, bool) bool
		})
		if !ok || !validator.ValidateContactReviewToken(formValues, body.ReviewToken, body.Confirmed) {
			h.recordContactBlock("review", r)
			reviewToken := ""
			if provider, ok := h.contactSubmitter.(interface {
				ContactReviewToken(url.Values, string) string
			}); ok {
				reviewToken = provider.ContactReviewToken(formValues, "")
			}
			h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Data: map[string]interface{}{"review": map[string]string{"type": body.Type, "email": body.Email, "subject": body.Subject}, "review_token": reviewToken}})
			return
		}
	} else if !h.validateContactFormToken(r, body.FormToken) {
		reason := "fill_time"
		if provider, ok := h.contactSubmitter.(interface{ ContactTokenFailureReason(string) string }); ok {
			reason = provider.ContactTokenFailureReason(body.FormToken)
		}
		h.recordContactBlock(reason, r)
		h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": "form_token", "rule": "invalid", "form_token": h.contactFormToken(r)}})
		return
	}
	providerProof := strings.TrimSpace(body.CaptchaResponse)
	if step != "confirm" {
		for _, name := range []string{"g-recaptcha-response", "h-captcha-response"} {
			var value string
			_ = json.Unmarshal(suppliedFields[name], &value)
			if strings.TrimSpace(value) == "" {
				continue
			}
			if providerProof != "" {
				h.recordContactBlock("captcha", r)
				h.contactCaptchaFailure(w, r)
				return
			}
			providerProof = strings.TrimSpace(value)
		}
	}
	validCaptcha := false
	if step != "confirm" {
		if verifier, ok := h.contactSubmitter.(ContactProofVerifier); ok {
			validCaptcha = verifier.VerifyContactProof(r, providerProof, body.CaptchaID, body.CaptchaAnswer)
		} else if providerProof == "" && h.contactCaptcha != nil {
			validCaptcha = h.contactCaptcha.VerifyContactChallenge(r, body.CaptchaID, body.CaptchaAnswer)
		}
	}
	if step != "confirm" && !validCaptcha {
		h.recordContactBlock("captcha", r)
		h.contactCaptchaFailure(w, r)
		return
	}
	address, err := mail.ParseAddress(body.Email)
	field, maxLength := "", 0
	limits := []struct {
		name, value string
		max         int
	}{{"name", body.Name, 100}, {"email", body.Email, 254}}
	if securityMode {
		gpgMax := 16384
		if strings.HasPrefix(strings.ToLower(body.ResearcherGPG), "https://") {
			gpgMax = 2048
		}
		limits = append(limits,
			struct {
				name, value string
				max         int
			}{"researcher_gpg", body.ResearcherGPG, gpgMax},
			struct {
				name, value string
				max         int
			}{"affected_component", body.AffectedComponent, 255},
			struct {
				name, value string
				max         int
			}{"affected_endpoint", body.AffectedEndpoint, 2048},
			struct {
				name, value string
				max         int
			}{"summary", body.Summary, 200},
			struct {
				name, value string
				max         int
			}{"steps_to_reproduce", body.StepsToReproduce, 10000},
			struct {
				name, value string
				max         int
			}{"impact", body.Impact, 5000},
			struct {
				name, value string
				max         int
			}{"suggested_fix", body.SuggestedFix, 5000},
			struct {
				name, value string
				max         int
			}{"credit_handle", body.CreditHandle, 100},
		)
	} else {
		limits = append(limits, struct {
			name, value string
			max         int
		}{"subject", body.Subject, 200}, struct {
			name, value string
			max         int
		}{"message", body.Message, 5000})
	}
	for _, item := range limits {
		if utf8.RuneCountInString(item.value) > item.max {
			field, maxLength = item.name, item.max
			break
		}
	}
	if field != "" {
		rule := "max_length"
		if field == "researcher_gpg" && strings.HasPrefix(strings.ToLower(body.ResearcherGPG), "https://") {
			rule = "url_max_length"
		}
		h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": field, "rule": rule, "max_length": maxLength}})
		return
	}
	if body.Name == "" || err != nil || address.Address != body.Email || (!securityMode && (body.Subject == "" || body.Message == "")) {
		h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": "contact", "rule": "required_or_format"}})
		return
	}
	if securityMode && body.CreditPreference == "handle" && body.CreditHandle == "" {
		h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": "credit_handle", "rule": "required"}})
		return
	}
	if securityMode {
		validSeverity := map[string]bool{"critical": true, "high": true, "medium": true, "low": true, "informational": true}
		validCredit := map[string]bool{"real_name": true, "handle": true, "no": true, "anonymous": true}
		if !validSeverity[body.Severity] || !validCredit[body.CreditPreference] || body.AffectedComponent == "" || body.Summary == "" || body.StepsToReproduce == "" || body.Impact == "" || !body.AgreedToDisclosure || body.DisclosureDays < 0 {
			h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")), Details: map[string]interface{}{"field": "security_report", "rule": "required_or_format"}})
			return
		}
		if body.DisclosureDays == 0 {
			body.DisclosureDays = 90
		}
		if step == "submit" {
			if checker, ok := h.contactSubmitter.(interface {
				CheckContactSubmissionLimit(*http.Request, string, string) (string, time.Duration)
			}); ok {
				if reason, retryAfter := checker.CheckContactSubmissionLimit(r, httputil.GetClientIP(r), address.Address); reason != "" {
					w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
					h.jsonResponse(w, http.StatusTooManyRequests, &APIResponse{OK: false, Error: "RATE_LIMITED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusTooManyRequests))})
					return
				}
			}
			reviewType := body.Type
			if reviewType == "" {
				reviewType = "security"
				formValues.Set("type", reviewType)
			}
			token := ""
			if provider, ok := h.contactSubmitter.(interface {
				ContactReviewToken(url.Values, string) string
			}); ok {
				token = provider.ContactReviewToken(formValues, "")
			}
			minSeconds := h.config.Get().Pages.Contact.Antispam.ReviewMinSeconds
			if minSeconds <= 0 {
				minSeconds = 2
			}
			h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{
				"review":       map[string]string{"type": reviewType, "email": address.Address, "subject": body.Summary},
				"review_token": token, "confirm_after_seconds": minSeconds,
			}})
			return
		}
		if reason, retryAfter := h.acceptContactSubmission(r, address.Address, body.Name, body.Summary, body.StepsToReproduce, true); reason != "" {
			h.recordContactBlock(reason, r)
			if reason == "duplicate" {
				h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"status": "received"}})
				return
			}
			if retryAfter > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
				h.jsonResponse(w, http.StatusTooManyRequests, &APIResponse{OK: false, Error: "RATE_LIMITED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusTooManyRequests))})
				return
			}
			h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, "errors.validation_failed"), Details: map[string]interface{}{"field": "contact", "rule": reason}})
			return
		}
		trackingID, submitErr := h.securityContact.SubmitSecurityContact(r, SecurityContactMessage{SecurityID: body.SecurityID, Name: body.Name, Email: address.Address, ResearcherGPG: body.ResearcherGPG, AffectedComponent: body.AffectedComponent, AffectedEndpoint: body.AffectedEndpoint, Severity: body.Severity, Summary: body.Summary, StepsToReproduce: body.StepsToReproduce, Impact: body.Impact, SuggestedFix: body.SuggestedFix, CVERequested: body.CVERequested, DisclosureDays: body.DisclosureDays, CreditPreference: body.CreditPreference, CreditHandle: body.CreditHandle, AgreedToDisclosure: body.AgreedToDisclosure})
		if submitErr != nil {
			h.writeError(w, r, "DELIVERY_UNAVAILABLE", i18n.RequestString(r, apiErrorTranslationKey(http.StatusServiceUnavailable)), http.StatusServiceUnavailable)
			return
		}
		h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"tracking_id": trackingID}})
		return
	}
	if h.contactSubmitter == nil {
		h.writeError(w, r, "NOT_AVAILABLE", i18n.RequestString(r, apiErrorTranslationKey(http.StatusServiceUnavailable)), http.StatusServiceUnavailable)
		return
	}
	if step == "submit" {
		if checker, ok := h.contactSubmitter.(interface {
			CheckContactSubmissionLimit(*http.Request, string, string) (string, time.Duration)
		}); ok {
			if reason, retryAfter := checker.CheckContactSubmissionLimit(r, httputil.GetClientIP(r), address.Address); reason != "" {
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
				h.jsonResponse(w, http.StatusTooManyRequests, &APIResponse{OK: false, Error: "RATE_LIMITED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusTooManyRequests))})
				return
			}
		}
		token := ""
		if provider, ok := h.contactSubmitter.(interface {
			ContactReviewToken(url.Values, string) string
		}); ok {
			token = provider.ContactReviewToken(formValues, "")
		}
		minSeconds := h.config.Get().Pages.Contact.Antispam.ReviewMinSeconds
		if minSeconds <= 0 {
			minSeconds = 2
		}
		h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{
			"review":       map[string]string{"type": body.Type, "email": address.Address, "subject": body.Subject},
			"review_token": token, "confirm_after_seconds": minSeconds,
		}})
		return
	}
	if reason, retryAfter := h.acceptContactSubmission(r, address.Address, body.Name, body.Subject, body.Message, false); reason != "" {
		h.recordContactBlock(reason, r)
		if reason == "duplicate" {
			h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"status": "received"}})
			return
		}
		if retryAfter > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
			h.jsonResponse(w, http.StatusTooManyRequests, &APIResponse{OK: false, Error: "RATE_LIMITED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusTooManyRequests))})
			return
		}
		h.jsonResponse(w, http.StatusBadRequest, &APIResponse{OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, "errors.validation_failed"), Details: map[string]interface{}{"field": "contact", "rule": reason}})
		return
	}
	if err := h.contactSubmitter.SubmitContact(r.Context(), ContactMessage{
		Name: body.Name, Email: address.Address, Subject: body.Subject, Message: body.Message, Type: body.Type,
		ReportedURL: body.ReportedURL, Reason: body.Reason, CopyrightedWork: body.CopyrightedWork,
		GoodFaithStatement: body.GoodFaithStatement, AccuracyStatement: body.AccuracyStatement,
		Signature: body.Signature, RequestKind: body.RequestKind, SourceIP: httputil.GetClientIP(r),
	}); err != nil {
		h.forgetContactSubmission(r, address.Address, body.Name, body.Subject, body.Message, false)
		h.writeError(w, r, "DELIVERY_UNAVAILABLE", i18n.RequestString(r, apiErrorTranslationKey(http.StatusServiceUnavailable)), http.StatusServiceUnavailable)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"status": "received"}})
}

func (h *Handler) contactCaptchaInfo(r *http.Request) map[string]interface{} {
	provider := h.contactCaptcha
	if provider == nil {
		provider, _ = h.contactSubmitter.(ContactCaptchaProvider)
	}
	fallback := map[string]interface{}{"provider": "simple"}
	if provider != nil {
		if id, question := provider.NewContactChallenge(r); id != "" {
			fallback["challenge_id"], fallback["question"] = id, question
		}
	}
	cfg := h.config.Get().Pages.Contact
	name := strings.ToLower(strings.TrimSpace(cfg.Captcha))
	if name == "recaptcha" || name == "hcaptcha" {
		return map[string]interface{}{"provider": name, "site_key": cfg.CaptchaSiteKey, "fallback": fallback}
	}
	return fallback
}

func (h *Handler) contactCaptchaFailure(w http.ResponseWriter, r *http.Request) {
	h.jsonResponse(w, http.StatusBadRequest, &APIResponse{
		OK: false, Error: "VALIDATION_FAILED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusBadRequest, "VALIDATION_FAILED")),
		Details: map[string]interface{}{
			"field": "captcha", "rule": "invalid", "form_token": h.contactFormToken(r), "captcha": h.contactCaptchaInfo(r),
		},
	})
}

func (h *Handler) contactFormToken(r *http.Request) string {
	if provider, ok := h.contactSubmitter.(interface{ ContactFormToken() string }); ok {
		return provider.ContactFormToken()
	}
	issued := time.Now().Unix()
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return ""
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	mac := hmac.New(sha256.New, h.contactFormSigningKey())
	_, _ = fmt.Fprintf(mac, "contact-form|%d|%s", issued, nonce)
	return fmt.Sprintf("%d.%s.%s", issued, nonce, hex.EncodeToString(mac.Sum(nil)))
}

func (h *Handler) validateContactFormToken(r *http.Request, token string) bool {
	if provider, ok := h.contactSubmitter.(interface{ ValidateContactFormToken(string) bool }); ok {
		return provider.ValidateContactFormToken(token)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	issued, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || issued <= 0 || issued > time.Now().Unix() {
		return false
	}
	settings := h.config.Get().Pages.Contact.Antispam
	maxAge := settings.MaxAgeSeconds
	if maxAge <= 0 {
		maxAge = 43200
	}
	minFill := settings.MinFillSeconds
	if time.Now().Unix()-issued > int64(maxAge) || time.Now().Unix()-issued < int64(minFill) {
		return false
	}
	mac := hmac.New(sha256.New, h.contactFormSigningKey())
	_, _ = mac.Write([]byte("contact-form|" + parts[0] + "|" + parts[1]))
	provided, err := hex.DecodeString(parts[2])
	return err == nil && hmac.Equal(provided, mac.Sum(nil))
}

func (h *Handler) contactFormSigningKey() []byte {
	if h.config != nil {
		cfg := h.config.Get()
		if cfg.Security.InstallationSecret != "" {
			return []byte(cfg.Security.InstallationSecret)
		}
		if cfg.SecretKey != "" {
			return []byte(cfg.SecretKey)
		}
	}
	return []byte("search-contact-form")
}

func (h *Handler) recordContactBlock(reason string, r *http.Request) {
	if provider, ok := h.contactSubmitter.(interface{ RecordContactBlock(string, *http.Request) }); ok {
		provider.RecordContactBlock(reason, r)
	}
}

func (h *Handler) acceptContactSubmission(r *http.Request, email, name, subject, message string, securityMode bool) (string, time.Duration) {
	provider, ok := h.contactSubmitter.(interface {
		AcceptContactSubmission(*http.Request, string, string, string, string, string, bool) (string, time.Duration)
	})
	if !ok {
		return "", 0
	}
	return provider.AcceptContactSubmission(r, httputil.GetClientIP(r), email, name, subject, message, securityMode)
}

func (h *Handler) forgetContactSubmission(r *http.Request, email, name, subject, message string, securityMode bool) {
	provider, ok := h.contactSubmitter.(interface {
		ForgetContactSubmission(*http.Request, string, string, string, string, string, bool)
	})
	if ok {
		provider.ForgetContactSubmission(r, httputil.GetClientIP(r), email, name, subject, message, securityMode)
	}
}

func (h *Handler) handleServerSecurity(w http.ResponseWriter, r *http.Request) {
	if h.securityPages == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "Security page unavailable", "Security page data is not initialized", r)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: h.securityPages.SecurityOverview(r), Meta: &APIMeta{Version: APIVersion}})
}

func (h *Handler) handleServerSecurityPolicy(w http.ResponseWriter, r *http.Request) {
	if h.securityPages == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "Security policy unavailable", "Security page data is not initialized", r)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: h.securityPages.SecurityPolicy(r), Meta: &APIMeta{Version: APIVersion}})
}

func (h *Handler) handleServerSecurityThanks(w http.ResponseWriter, r *http.Request) {
	if h.securityPages == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "Security acknowledgments unavailable", "Security page data is not initialized", r)
		return
	}
	data, err := h.securityPages.SecurityThanks(r.Context())
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Security acknowledgments unavailable", "Security acknowledgment data could not be loaded", r)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: data, Meta: &APIMeta{Version: APIVersion}})
}

func (h *Handler) handleAuthCheck(w http.ResponseWriter, r *http.Request) {
	kind := ""
	if h.config != nil {
		expected := h.config.Get().Token
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if expected != "" && len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
			presented := strings.TrimSpace(header[len(prefix):])
			expectedHash := sha256.Sum256([]byte(expected))
			presentedHash := sha256.Sum256([]byte(presented))
			if subtle.ConstantTimeCompare(expectedHash[:], presentedHash[:]) == 1 {
				kind = "operator"
			}
		}
		if kind == "" && h.ownerManager != nil {
			if _, err := h.ownerManager.Resolve(r.Context(), bearerToken(r)); err == nil {
				kind = "owner"
			}
		}
	}
	if kind == "" {
		w.Header().Set("WWW-Authenticate", `Bearer realm="server"`)
		h.jsonResponse(w, http.StatusUnauthorized, &APIResponse{OK: false, Error: "UNAUTHORIZED", Message: i18n.RequestString(r, apiErrorTranslationKey(http.StatusUnauthorized))})
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"kind": kind}})
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
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
		state := prefsync.Current(r, "search", manager.DetectLanguage(r), manager.SupportedLanguageCodes())
		h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: state, Meta: &APIMeta{Version: APIVersion}})
		return
	}

	if r.Method == http.MethodPost {
		values := r.URL.Query()
		if strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			var input map[string]json.RawMessage
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input); err != nil {
				h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", r)
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
				h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid form body", r)
				return
			}
			values = r.Form
		}
		values.Del("code")
		clone := r.Clone(r.Context())
		urlCopy := *r.URL
		urlCopy.RawQuery = values.Encode()
		clone.URL = &urlCopy
		manager := h.syncI18nManager()
		result := prefsync.Import(clone, "search", manager.DetectLanguage(r), manager.SupportedLanguageCodes())
		if len(result.Applied) == 0 {
			h.errorResponse(w, http.StatusBadRequest, "VALIDATION_FAILED", "no valid preferences were supplied", r)
			return
		}
		prefsync.Apply(w, r, "search", result)
		h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"theme": result.State.Theme, "lang": result.State.Lang, "prefs": result.State.Prefs, "applied": result.Applied, "ignored": result.Ignored}, Meta: &APIMeta{Version: APIVersion}})
		return
	}

	h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", r.Method, r)
}

// syncI18nManager returns an i18n manager sufficient for language-code
// validation. Loaded translations aren't needed here - only the supported
// language list, which NewManager populates without loading locale files.
func (h *Handler) syncI18nManager() *i18n.Manager {
	if h != nil && h.config != nil {
		manager, err := i18n.NewConfiguredManager(h.config.Server.I18n.DefaultLanguage, h.config.Server.I18n.SupportedLanguages)
		if err == nil {
			return manager
		}
	}
	return i18n.NewManager("en", i18n.DefaultSupportedLanguages())
}

// handleServerPreferencesExport handles GET /api/v1/server/preferences/sync
// per AI.md "Cross-device preference sync": reads the current theme/lang
// cookies and returns the full import URL plus a base64url short code for
// the same state. Stateless - nothing is written or looked up server-side.
func (h *Handler) handleServerPreferencesExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported", r)
		return
	}

	manager := h.syncI18nManager()
	state, fullURL, shortCode := prefsync.Export(r, "search", manager.DetectLanguage(r), manager.SupportedLanguageCodes())

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: map[string]interface{}{"theme": state.Theme, "lang": state.Lang, "prefs": state.Prefs, "url": fullURL, "code": shortCode},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleServerPreferencesImport handles GET /api/v1/server/preferences/sync/import
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
		h.errorResponse(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET is supported", r)
		return
	}

	manager := h.syncI18nManager()
	result := prefsync.Import(r, "search", manager.DetectLanguage(r), manager.SupportedLanguageCodes())
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
	started := time.Now()
	finishFailure := func(reason string, cause error) error {
		slog.Debug("operator authentication rejected", "reason", reason)
		if elapsed := time.Since(started); elapsed < 100*time.Millisecond {
			time.Sleep(100*time.Millisecond - elapsed)
		}
		return cause
	}
	hdr := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(hdr) <= len(prefix) || !strings.EqualFold(hdr[:len(prefix)], prefix) {
		return finishFailure("missing_token", errors.New("operator authorization required"))
	}
	presented := strings.TrimSpace(hdr[len(prefix):])
	if expected := h.config.Get().Token; expected != "" {
		expectedSum := sha256.Sum256([]byte(expected))
		presentedSum := sha256.Sum256([]byte(presented))
		if subtle.ConstantTimeCompare(expectedSum[:], presentedSum[:]) == 1 {
			return nil
		}
	}
	if h.ownerManager != nil {
		if _, err := h.ownerManager.Resolve(r.Context(), presented); err == nil {
			return finishFailure("owner_token_insufficient_scope", errOwnerTokenCannotAuthorizeOperator)
		}
	}
	return finishFailure("invalid_token", errors.New("operator authorization required"))
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
	torRunning := false
	if h.torService != nil && !h.torService.IsStarting() {
		torRunning = h.torService.IsRunning()
	}
	return map[string]interface{}{
		"status": status, "version": APIVersion, "mode": cfg.Mode,
		"uptime": h.formatDuration(time.Since(h.startTime)), "go_version": runtime.Version(),
		"tor": map[string]interface{}{"enabled": cfg.Tor.Enabled, "running": torRunning},
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
			h.writeOperatorAuthError(w, r, err)
			return
		}
		next(w, r)
	}
}

func (h *Handler) writeOperatorAuthError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errOwnerTokenCannotAuthorizeOperator) {
		w.Header().Set("WWW-Authenticate", `Bearer error="insufficient_scope"`)
		h.errorResponse(w, http.StatusForbidden, "Insufficient permissions", "", r)
		return
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="operator"`)
	h.errorResponse(w, http.StatusUnauthorized, "Invalid credentials", "", r)
}

// handleServerStatus handles GET /api/v1/server/status (operator token required).
// Per AI.md PART 14: operator-gated machine-only JSON status endpoint.
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
	target := stripTxtExtension(chi.URLParam(r, "ip"))
	if target == "" {
		target = httputil.GetClientIP(r)
	}
	payload, err := h.LookupGeoIP(target)
	if err != nil {
		if h.geoipLookup == nil || !h.geoipLookup.IsLoaded() {
			h.errorResponse(w, http.StatusServiceUnavailable, "GeoIP database is not available", "", r)
			return
		}
		h.errorResponse(w, http.StatusBadRequest, "Invalid IP address", "", r)
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
		h.errorResponse(w, http.StatusNotFound, "Preset not found", "", r)
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
		h.errorResponse(w, http.StatusBadRequest, "Invalid JSON body", err.Error(), r)
		return
	}

	name := strings.ToLower(strings.TrimSpace(req.Name))
	if !config.IsValidPresetName(name) {
		h.errorResponse(w, http.StatusBadRequest, "Invalid preset name (a-z, 0-9, hyphen, underscore)", "", r)
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
		h.errorResponse(w, http.StatusBadRequest, "Invalid from value (deny_countries or allow_countries)", "", r)
		return
	}

	normalized := config.NormalizeCountryCodes(codes)
	if len(normalized) == 0 {
		h.errorResponse(w, http.StatusBadRequest, "No valid ISO 3166-1 alpha-2 country codes supplied", "", r)
		return
	}

	h.config.Update(func(s *config.ServerConfig) {
		s.GeoIP.SetPreset(name, normalized)
	})
	if err := h.saveConfig(); err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to persist preset", err.Error(), r)
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
		h.errorResponse(w, http.StatusNotFound, "Preset not found", "", r)
		return
	}
	if err := h.saveConfig(); err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to persist preset removal", err.Error(), r)
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
