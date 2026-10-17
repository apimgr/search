package server

import (
	"bufio"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
)

type requestSecond struct {
	second int64
	count  int64
}

// Metrics collects server metrics using Prometheus client library.
// Per AI.md PART 20: MUST use github.com/prometheus/client_golang.
type Metrics struct {
	config    *config.Config
	startTime time.Time

	// registry is this instance's private collector registry. Handler gathers
	// from it, so the served output covers exactly the collectors below.
	registry *prometheus.Registry

	// Per AI.md PART 13: Atomic counters for health endpoint stats
	// These are separate from Prometheus counters for easy access
	totalRequests     atomic.Int64
	activeConnections atomic.Int64
	requestWindowMu   sync.Mutex
	requestWindow     [24 * 60 * 60]requestSecond

	// HTTP metrics
	httpRequestsTotal   *prometheus.CounterVec
	httpRequestDuration *prometheus.HistogramVec
	httpRequestSize     *prometheus.HistogramVec
	httpResponseSize    *prometheus.HistogramVec
	httpActiveRequests  prometheus.Gauge

	// Database metrics
	dbQueriesTotal     *prometheus.CounterVec
	dbQueryDuration    *prometheus.HistogramVec
	dbConnectionsOpen  prometheus.Gauge
	dbConnectionsInUse prometheus.Gauge
	dbErrors           *prometheus.CounterVec

	// Cache metrics
	cacheHits      *prometheus.CounterVec
	cacheMisses    *prometheus.CounterVec
	cacheEvictions *prometheus.CounterVec
	cacheSize      *prometheus.GaugeVec
	cacheBytes     *prometheus.GaugeVec

	// Scheduler metrics
	schedulerTasksTotal   *prometheus.CounterVec
	schedulerTaskDuration *prometheus.HistogramVec
	schedulerTasksRunning *prometheus.GaugeVec
	schedulerLastRun      *prometheus.GaugeVec

	// Authentication metrics
	authAttempts       *prometheus.CounterVec
	authSessionsActive prometheus.Gauge

	// Rate limiting metrics
	ratelimitRequestsTotal *prometheus.CounterVec
	ratelimitBlockedTotal  *prometheus.CounterVec

	// Business metrics (search domain)
	searchesTotal  prometheus.Counter
	searchDuration *prometheus.HistogramVec
	engineRequests *prometheus.CounterVec
	engineErrors   *prometheus.CounterVec

	// Application info metrics
	appInfo           *prometheus.GaugeVec
	appUptimeSeconds  prometheus.Gauge
	appStartTimestamp prometheus.Gauge

	// Go runtime metrics
	goGoroutines   prometheus.Gauge
	goMemAlloc     prometheus.Gauge
	goMemSys       prometheus.Gauge
	goGCRuns       prometheus.Counter
	goGCPauseTotal prometheus.Counter

	// System metrics
	systemCPUUsage  prometheus.Gauge
	systemMemUsage  prometheus.Gauge
	systemMemUsed   prometheus.Gauge
	systemMemTotal  prometheus.Gauge
	systemDiskUsage *prometheus.GaugeVec
	systemDiskUsed  *prometheus.GaugeVec
	systemDiskTotal *prometheus.GaugeVec

	// dataDir is the path reported by the system_disk_* metrics' "path" label
	dataDir string

	// Tor metrics
	torEnabled            prometheus.Gauge
	torRunning            prometheus.Gauge
	torCircuitEstablished prometheus.Gauge
	torRequestsTotal      prometheus.Counter

	// Runtime GC deltas, only touched by the collector goroutine
	lastGC    uint32
	lastPause uint64

	// torBackendPort is the loopback port the Tor hidden service forwards to;
	// 0 when Tor is not running. Set via SetTorBackendPort.
	torBackendPort atomic.Int64
}

// NewMetrics creates a new Prometheus metrics collector
// Per AI.md PART 20: Use github.com/prometheus/client_golang with promauto
func NewMetrics(cfg *config.Config) *Metrics {
	// Each Metrics owns a private registry rather than registering on the
	// process-wide default. Registering globally makes a second Server in the
	// same process panic on duplicate collector registration, which is only
	// ever correct because a deployed service runs exactly one. A private
	// registry keeps the /server/metrics output identical (Handler gathers
	// from this same registry) while letting tests and tools host more than
	// one instance at a time.
	reg := prometheus.NewRegistry()

	// Duration and size buckets come from server.yml (server.metrics.
	// duration_buckets / size_buckets) per AI.md PART 20 "Configuration",
	// falling back to the spec defaults when unset or empty.
	durationBuckets := cfg.Server.Metrics.DurationBuckets
	if len(durationBuckets) == 0 {
		durationBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	}

	// Size buckets per AI.md PART 20
	sizeBuckets := cfg.Server.Metrics.SizeBuckets
	if len(sizeBuckets) == 0 {
		sizeBuckets = []float64{100, 1000, 10000, 100000, 1000000, 10000000}
	}

	// Query duration buckets
	queryBuckets := []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}

	// Task duration buckets
	taskBuckets := []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600}

	m := &Metrics{
		config:    cfg,
		startTime: time.Now(),
		registry:  reg,

		// HTTP metrics per AI.md PART 20
		httpRequestsTotal: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_http_requests_total",
				Help: "Total number of HTTP requests",
			},
			[]string{"method", "path", "status"},
		),
		httpRequestDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "search_http_request_duration_seconds",
				Help:    "HTTP request duration in seconds",
				Buckets: durationBuckets,
			},
			[]string{"method", "path"},
		),
		httpRequestSize: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "search_http_request_size_bytes",
				Help:    "HTTP request size in bytes",
				Buckets: sizeBuckets,
			},
			[]string{"method", "path"},
		),
		httpResponseSize: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "search_http_response_size_bytes",
				Help:    "HTTP response size in bytes",
				Buckets: sizeBuckets,
			},
			[]string{"method", "path"},
		),
		httpActiveRequests: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_http_active_requests",
				Help: "Number of active HTTP requests",
			},
		),

		// Database metrics per AI.md PART 20
		dbQueriesTotal: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_db_queries_total",
				Help: "Total number of database queries",
			},
			[]string{"operation", "table"},
		),
		dbQueryDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "search_db_query_duration_seconds",
				Help:    "Database query duration in seconds",
				Buckets: queryBuckets,
			},
			[]string{"operation", "table"},
		),
		dbConnectionsOpen: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_db_connections_open",
				Help: "Number of open database connections",
			},
		),
		dbConnectionsInUse: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_db_connections_in_use",
				Help: "Number of database connections in use",
			},
		),
		dbErrors: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_db_errors_total",
				Help: "Total number of database errors",
			},
			[]string{"operation", "error_type"},
		),

		// Cache metrics per AI.md PART 20
		cacheHits: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_cache_hits_total",
				Help: "Total number of cache hits",
			},
			[]string{"cache"},
		),
		cacheMisses: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_cache_misses_total",
				Help: "Total number of cache misses",
			},
			[]string{"cache"},
		),
		cacheEvictions: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_cache_evictions_total",
				Help: "Total number of cache evictions",
			},
			[]string{"cache"},
		),
		cacheSize: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_cache_size",
				Help: "Current cache size (items)",
			},
			[]string{"cache"},
		),
		cacheBytes: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_cache_bytes",
				Help: "Current cache size (bytes)",
			},
			[]string{"cache"},
		),

		// Scheduler metrics per AI.md PART 20
		schedulerTasksTotal: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_scheduler_tasks_total",
				Help: "Total number of scheduled tasks executed",
			},
			[]string{"task", "status"},
		),
		schedulerTaskDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "search_scheduler_task_duration_seconds",
				Help:    "Scheduled task duration in seconds",
				Buckets: taskBuckets,
			},
			[]string{"task"},
		),
		schedulerTasksRunning: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_scheduler_tasks_running",
				Help: "Number of currently running scheduled tasks",
			},
			[]string{"task"},
		),
		schedulerLastRun: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_scheduler_last_run_timestamp",
				Help: "Timestamp of last task run",
			},
			[]string{"task"},
		),

		// Authentication metrics per AI.md PART 20
		authAttempts: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_auth_attempts_total",
				Help: "Total authentication attempts",
			},
			[]string{"method", "status"},
		),
		authSessionsActive: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_auth_sessions_active",
				Help: "Number of active sessions",
			},
		),

		// Rate limiting metrics per AI.md PART 20
		ratelimitRequestsTotal: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_ratelimit_requests_total",
				Help: "Total rate-limited requests",
			},
			[]string{"limit", "status"},
		),
		ratelimitBlockedTotal: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_ratelimit_blocked_total",
				Help: "Requests blocked by rate limiter",
			},
			[]string{"limit"},
		),

		// Search metrics
		searchesTotal: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "search_searches_total",
				Help: "Total number of search queries",
			},
		),
		searchDuration: promauto.With(reg).NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "search_search_duration_seconds",
				Help:    "Search query duration in seconds",
				Buckets: durationBuckets,
			},
			[]string{"category"},
		),
		engineRequests: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_engine_requests_total",
				Help: "Total requests per search engine",
			},
			[]string{"engine"},
		),
		engineErrors: promauto.With(reg).NewCounterVec(
			prometheus.CounterOpts{
				Name: "search_engine_errors_total",
				Help: "Total errors per search engine",
			},
			[]string{"engine"},
		),

		// Application info metrics per AI.md PART 20
		appInfo: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_app_info",
				Help: "Application information",
			},
			[]string{"version", "commit", "build_date", "go_version"},
		),
		appUptimeSeconds: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_app_uptime_seconds",
				Help: "Application uptime in seconds",
			},
		),
		appStartTimestamp: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_app_start_timestamp",
				Help: "Application start timestamp",
			},
		),

		// Go runtime metrics per AI.md PART 20
		goGoroutines: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_go_goroutines",
				Help: "Number of goroutines",
			},
		),
		goMemAlloc: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_go_mem_alloc_bytes",
				Help: "Bytes allocated and in use",
			},
		),
		goMemSys: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_go_mem_sys_bytes",
				Help: "Bytes obtained from system",
			},
		),
		goGCRuns: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "search_go_gc_runs_total",
				Help: "Total number of GC runs",
			},
		),
		goGCPauseTotal: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "search_go_gc_pause_total_seconds",
				Help: "Total GC pause time in seconds",
			},
		),

		// System metrics per AI.md PART 20
		systemCPUUsage: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_system_cpu_usage_percent",
				Help: "Current CPU usage percentage",
			},
		),
		systemMemUsage: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_system_memory_usage_percent",
				Help: "Current memory usage percentage",
			},
		),
		systemMemUsed: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_system_memory_used_bytes",
				Help: "Memory used in bytes",
			},
		),
		systemMemTotal: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_system_memory_total_bytes",
				Help: "Total memory in bytes",
			},
		),
		systemDiskUsage: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_system_disk_usage_percent",
				Help: "Disk usage percentage",
			},
			[]string{"path"},
		),
		systemDiskUsed: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_system_disk_used_bytes",
				Help: "Disk used in bytes",
			},
			[]string{"path"},
		),
		systemDiskTotal: promauto.With(reg).NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "search_system_disk_total_bytes",
				Help: "Total disk in bytes",
			},
			[]string{"path"},
		),

		// Tor metrics per AI.md PART 20 / PART 31
		torEnabled: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_tor_enabled",
				Help: "1 if Tor is enabled",
			},
		),
		torRunning: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_tor_running",
				Help: "1 if Tor process is running",
			},
		),
		torCircuitEstablished: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_tor_circuit_established",
				Help: "1 if a Tor circuit is established",
			},
		),
		torRequestsTotal: promauto.With(reg).NewCounter(
			prometheus.CounterOpts{
				Name: "search_tor_requests_total",
				Help: "Total requests received via the Tor hidden service",
			},
		),
	}

	m.dataDir = config.GetDataDir()

	// Application info is constant for the process lifetime, so it is set once here.
	info := version.Get()
	m.appInfo.WithLabelValues(info.Version, info.Commit, info.BuildDate, info.GoVersion).Set(1)
	m.appStartTimestamp.Set(float64(m.startTime.Unix()))

	// Collect once immediately so the first scrape is never all-zero.
	m.collectSystemMetrics()

	// Start background goroutine to update system metrics
	go m.updateSystemMetrics()

	return m
}

// SetTorBackendPort records the loopback port the Tor hidden service forwards
// to, so the metrics middleware can attribute requests to the onion service.
// Pass 0 when Tor is not running.
func (m *Metrics) SetTorBackendPort(port int) {
	m.torBackendPort.Store(int64(port))
}

// SetTorState updates the Tor state gauges per AI.md PART 20 "Tor Metrics".
func (m *Metrics) SetTorState(enabled, running, circuitEstablished bool) {
	m.torEnabled.Set(boolGauge(enabled))
	m.torRunning.Set(boolGauge(running))
	m.torCircuitEstablished.Set(boolGauge(circuitEstablished))
}

// SetActiveSessions sets the number of active sessions.
func (m *Metrics) SetActiveSessions(n int) {
	m.authSessionsActive.Set(float64(n))
}

// boolGauge maps a boolean to the Prometheus 1/0 gauge convention.
func boolGauge(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// updateSystemMetrics periodically updates system metrics
func (m *Metrics) updateSystemMetrics() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		m.collectSystemMetrics()
	}
}

// collectSystemMetrics collects current system, runtime and uptime metrics.
// Runtime and system groups are gated on server.metrics.include_runtime and
// server.metrics.include_system per AI.md PART 20.
func (m *Metrics) collectSystemMetrics() {
	// Uptime is part of the always-required application metric group
	m.appUptimeSeconds.Set(time.Since(m.startTime).Seconds())

	// Go runtime metrics (if enabled)
	if m.config.Server.Metrics.IncludeRuntime {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)

		m.goGoroutines.Set(float64(runtime.NumGoroutine()))
		m.goMemAlloc.Set(float64(memStats.Alloc))
		m.goMemSys.Set(float64(memStats.Sys))

		// Counters only ever move forward, so feed them the delta since last collection
		if memStats.NumGC > m.lastGC {
			m.goGCRuns.Add(float64(memStats.NumGC - m.lastGC))
			m.lastGC = memStats.NumGC
		}
		if memStats.PauseTotalNs > m.lastPause {
			m.goGCPauseTotal.Add(float64(memStats.PauseTotalNs-m.lastPause) / 1e9)
			m.lastPause = memStats.PauseTotalNs
		}
	}

	// System metrics (if enabled)
	if m.config.Server.Metrics.IncludeSystem {
		m.systemCPUUsage.Set(getCPUUsage())

		memUsed, memTotal := getMemoryUsage()
		m.systemMemUsed.Set(float64(memUsed))
		m.systemMemTotal.Set(float64(memTotal))
		if memTotal > 0 {
			m.systemMemUsage.Set(float64(memUsed) / float64(memTotal) * 100)
		}

		diskUsed, diskTotal := getDiskUsage(m.dataDir)
		m.systemDiskUsed.WithLabelValues(m.dataDir).Set(float64(diskUsed))
		m.systemDiskTotal.WithLabelValues(m.dataDir).Set(float64(diskTotal))
		if diskTotal > 0 {
			m.systemDiskUsage.WithLabelValues(m.dataDir).Set(float64(diskUsed) / float64(diskTotal) * 100)
		}
	}
}

// RecordRequest records an HTTP request
// Per AI.md PART 13: Also increments atomic counter for health endpoint stats
func (m *Metrics) RecordRequest(method, path string, statusCode int, duration time.Duration, reqSize, respSize int64) {
	m.recordRequestAt(method, path, statusCode, duration, reqSize, respSize, time.Now())
}

func (m *Metrics) recordRequestAt(method, path string, statusCode int, duration time.Duration, reqSize, respSize int64, at time.Time) {
	m.totalRequests.Add(1)
	second := at.Unix()
	m.requestWindowMu.Lock()
	bucket := &m.requestWindow[second%int64(len(m.requestWindow))]
	if bucket.second != second {
		bucket.second = second
		bucket.count = 0
	}
	bucket.count++
	m.requestWindowMu.Unlock()

	status := strconv.Itoa(statusCode)
	m.httpRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.httpRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
	m.httpRequestSize.WithLabelValues(method, path).Observe(float64(reqSize))
	m.httpResponseSize.WithLabelValues(method, path).Observe(float64(respSize))
}

// RecordSearch records a search operation
func (m *Metrics) RecordSearch(category string, duration time.Duration) {
	m.searchesTotal.Inc()
	m.searchDuration.WithLabelValues(category).Observe(duration.Seconds())
}

// RecordEngineRequest records a request to a search engine
func (m *Metrics) RecordEngineRequest(engine string) {
	m.engineRequests.WithLabelValues(engine).Inc()
}

// RecordEngineError records an error from a search engine
func (m *Metrics) RecordEngineError(engine string) {
	m.engineErrors.WithLabelValues(engine).Inc()
}

// RecordDBQuery records a database query
func (m *Metrics) RecordDBQuery(operation, table string, duration time.Duration) {
	m.dbQueriesTotal.WithLabelValues(operation, table).Inc()
	m.dbQueryDuration.WithLabelValues(operation, table).Observe(duration.Seconds())
}

// RecordDBError records a database error
func (m *Metrics) RecordDBError(operation, errorType string) {
	m.dbErrors.WithLabelValues(operation, errorType).Inc()
}

// RecordCacheHit records a cache hit
func (m *Metrics) RecordCacheHit(cache string) {
	m.cacheHits.WithLabelValues(cache).Inc()
}

// RecordCacheMiss records a cache miss
func (m *Metrics) RecordCacheMiss(cache string) {
	m.cacheMisses.WithLabelValues(cache).Inc()
}

// RecordSchedulerTask records a scheduler task execution
func (m *Metrics) RecordSchedulerTask(task, status string, duration time.Duration) {
	m.schedulerTasksTotal.WithLabelValues(task, status).Inc()
	m.schedulerTaskDuration.WithLabelValues(task).Observe(duration.Seconds())
	m.schedulerLastRun.WithLabelValues(task).SetToCurrentTime()
}

// RecordAuthAttempt records an authentication attempt
func (m *Metrics) RecordAuthAttempt(method, status string) {
	m.authAttempts.WithLabelValues(method, status).Inc()
}

// RecordRateLimit records a rate limiter decision. limit must be one of
// "global", "per_ip", "per_user", "per_endpoint" (never a raw client IP -
// see AI.md PART 20 "Rate Limiting Metrics" cardinality note). status is
// "allowed" or "limited".
func (m *Metrics) RecordRateLimit(limit, status string) {
	m.ratelimitRequestsTotal.WithLabelValues(limit, status).Inc()
	if status == "limited" {
		m.ratelimitBlockedTotal.WithLabelValues(limit).Inc()
	}
}

// SetActiveRequests sets the current number of active requests
func (m *Metrics) SetActiveRequests(n int) {
	m.httpActiveRequests.Set(float64(n))
}

// SetDBConnections sets the current database connection counts
func (m *Metrics) SetDBConnections(open, inUse int) {
	m.dbConnectionsOpen.Set(float64(open))
	m.dbConnectionsInUse.Set(float64(inUse))
}

// SetCacheStats sets cache statistics
func (m *Metrics) SetCacheStats(cache string, size int, bytes int64) {
	m.cacheSize.WithLabelValues(cache).Set(float64(size))
	m.cacheBytes.WithLabelValues(cache).Set(float64(bytes))
}

// GetTotalRequests returns total requests for health endpoint
// Per AI.md PART 13: stats.requests_total must return actual count
func (m *Metrics) GetTotalRequests() int64 {
	return m.totalRequests.Load()
}

// GetRequestsLast24Hours returns the request count from the trailing 24 hours.
func (m *Metrics) GetRequestsLast24Hours() int64 {
	now := time.Now().Unix()
	cutoff := now - int64(len(m.requestWindow)) + 1
	var total int64
	m.requestWindowMu.Lock()
	for i := range m.requestWindow {
		bucket := &m.requestWindow[i]
		if bucket.second >= cutoff && bucket.second <= now {
			total += bucket.count
		}
	}
	m.requestWindowMu.Unlock()
	return total
}

// GetActiveConnections returns current active connections for health endpoint
// Per AI.md PART 13: stats.active_connections must return actual count
func (m *Metrics) GetActiveConnections() int64 {
	return m.activeConnections.Load()
}

// Handler returns an HTTP handler for Prometheus metrics
// Per AI.md PART 20: Uses promhttp.Handler()
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// MetricsMiddleware creates middleware for recording request metrics
// Per AI.md PART 13: Tracks active connections for health endpoint stats
func (m *Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Increment active requests (both Prometheus gauge and atomic counter)
		m.httpActiveRequests.Inc()
		m.activeConnections.Add(1)
		defer func() {
			m.httpActiveRequests.Dec()
			m.activeConnections.Add(-1)
		}()

		// Attribute the request to the Tor hidden service when it arrived on the
		// loopback port the onion service forwards to (AI.md PART 20 tor_requests_total)
		if port := m.torBackendPort.Load(); port > 0 {
			if local, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
				if _, p, err := net.SplitHostPort(local.String()); err == nil && p == strconv.FormatInt(port, 10) {
					m.torRequestsTotal.Inc()
				}
			}
		}

		// Wrap response writer to capture status code and size
		wrapped := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// Record request metrics
		duration := time.Since(start)
		path := chi.RouteContext(r.Context()).RoutePattern()
		if path == "" {
			path = normalizePath(r.URL.Path)
		}
		m.RecordRequest(r.Method, path, wrapped.statusCode, duration, r.ContentLength, int64(wrapped.bytesWritten))
	})
}

// metricsResponseWriter wraps http.ResponseWriter to capture status code and bytes written
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *metricsResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *metricsResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += n
	return n, err
}

// normalizePath normalizes URL paths to reduce cardinality
func normalizePath(path string) string {
	// Normalize common patterns to reduce cardinality
	// Replace UUIDs with placeholder
	// This prevents high cardinality labels
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if len(p) == 36 && strings.Count(p, "-") == 4 {
			parts[i] = ":id"
		}
		// Replace numeric IDs
		if _, err := strconv.Atoi(p); err == nil && len(p) > 0 {
			parts[i] = ":id"
		}
	}
	return strings.Join(parts, "/")
}

// getCPUUsage returns the current CPU usage percentage (Linux only)
func getCPUUsage() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	file, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer closeutil.Close(file)

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return 0
	}

	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return 0
	}

	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0
	}

	user, _ := strconv.ParseFloat(fields[1], 64)
	nice, _ := strconv.ParseFloat(fields[2], 64)
	system, _ := strconv.ParseFloat(fields[3], 64)
	idle, _ := strconv.ParseFloat(fields[4], 64)

	total := user + nice + system + idle
	if total == 0 {
		return 0
	}

	return ((total - idle) / total) * 100
}

// getMemoryUsage returns system memory (used, total) in bytes (Linux only)
func getMemoryUsage() (used, total uint64) {
	if runtime.GOOS != "linux" {
		return 0, 0
	}

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer closeutil.Close(file)

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		value, _ := strconv.ParseUint(fields[1], 10, 64)
		switch fields[0] {
		case "MemTotal:":
			memTotal = value
		case "MemAvailable:":
			memAvailable = value
		}

		if memTotal > 0 && memAvailable > 0 {
			break
		}
	}

	if memTotal == 0 {
		return 0, 0
	}

	// /proc/meminfo reports kibibytes
	total = memTotal * 1024
	used = (memTotal - memAvailable) * 1024
	return used, total
}

// getDiskUsage returns the disk usage in bytes (used, total) for the given path
func getDiskUsage(path string) (used, total uint64) {
	if runtime.GOOS == "windows" {
		return 0, 0
	}

	return getDiskUsageUnix(path)
}
