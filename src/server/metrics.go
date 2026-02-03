package server

import (
	"bufio"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/apimgr/search/src/config"
)

// Metrics collects server metrics using Prometheus client library
// Per AI.md PART 29: MUST use github.com/prometheus/client_golang
type Metrics struct {
	config    *config.Config
	startTime time.Time
	registry  *prometheus.Registry

	// Per AI.md PART 13: Atomic counters for health endpoint stats
	// These are separate from Prometheus counters for easy access
	totalRequests     atomic.Int64
	activeConnections atomic.Int64

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
	schedulerTasksTotal    *prometheus.CounterVec
	schedulerTaskDuration  *prometheus.HistogramVec
	schedulerTasksRunning  *prometheus.GaugeVec
	schedulerLastRun       *prometheus.GaugeVec

	// Authentication metrics
	authAttempts       *prometheus.CounterVec
	authSessionsActive prometheus.Gauge

	// Business metrics
	usersTotal  prometheus.Gauge
	usersActive prometheus.Gauge

	// Search metrics
	searchesTotal    prometheus.Counter
	searchDuration   *prometheus.HistogramVec
	engineRequests   *prometheus.CounterVec
	engineErrors     *prometheus.CounterVec

	// System metrics
	uptimeSeconds    prometheus.Gauge
	goroutines       prometheus.Gauge
	memAlloc         prometheus.Gauge
	memSys           prometheus.Gauge
	cpuUsage         prometheus.Gauge
	memUsedPercent   prometheus.Gauge
	diskUsedBytes    prometheus.Gauge
	diskTotalBytes   prometheus.Gauge
	diskUsedPercent  prometheus.Gauge
}

// NewMetrics creates a new Prometheus metrics collector
// Per AI.md PART 29: Use github.com/prometheus/client_golang with promauto
func NewMetrics(cfg *config.Config) *Metrics {
	// Use default registry or create custom one
	reg := prometheus.DefaultRegisterer

	// Duration buckets per AI.md PART 29
	durationBuckets := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

	// Size buckets per AI.md PART 29
	sizeBuckets := []float64{100, 1000, 10000, 100000, 1000000, 10000000}

	// Query duration buckets
	queryBuckets := []float64{0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1}

	// Task duration buckets
	taskBuckets := []float64{0.1, 0.5, 1, 5, 10, 30, 60, 300, 600}

	m := &Metrics{
		config:    cfg,
		startTime: time.Now(),

		// HTTP metrics per AI.md PART 29
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

		// Database metrics per AI.md PART 29
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

		// Cache metrics per AI.md PART 29
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

		// Scheduler metrics per AI.md PART 29
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

		// Authentication metrics per AI.md PART 29
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

		// Business metrics per AI.md PART 29
		usersTotal: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_users_total",
				Help: "Total number of registered users",
			},
		),
		usersActive: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_users_active",
				Help: "Number of users active in last 24 hours",
			},
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

		// System metrics
		uptimeSeconds: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_uptime_seconds",
				Help: "Server uptime in seconds",
			},
		),
		goroutines: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_goroutines",
				Help: "Number of active goroutines",
			},
		),
		memAlloc: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_memory_alloc_bytes",
				Help: "Currently allocated memory in bytes",
			},
		),
		memSys: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_memory_sys_bytes",
				Help: "Total memory obtained from system",
			},
		),
		cpuUsage: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_cpu_usage_percent",
				Help: "CPU usage percentage",
			},
		),
		memUsedPercent: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_memory_used_percent",
				Help: "System memory usage percentage",
			},
		),
		diskUsedBytes: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_disk_used_bytes",
				Help: "Disk space used in bytes",
			},
		),
		diskTotalBytes: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_disk_total_bytes",
				Help: "Total disk space in bytes",
			},
		),
		diskUsedPercent: promauto.With(reg).NewGauge(
			prometheus.GaugeOpts{
				Name: "search_disk_used_percent",
				Help: "Disk space usage percentage",
			},
		),
	}

	// Start background goroutine to update system metrics
	go m.updateSystemMetrics()

	return m
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

// collectSystemMetrics collects current system metrics
func (m *Metrics) collectSystemMetrics() {
	// Uptime
	m.uptimeSeconds.Set(time.Since(m.startTime).Seconds())

	// Goroutines
	m.goroutines.Set(float64(runtime.NumGoroutine()))

	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	m.memAlloc.Set(float64(memStats.Alloc))
	m.memSys.Set(float64(memStats.Sys))

	// System metrics (if enabled)
	if m.config.Server.Metrics.IncludeSystem {
		m.cpuUsage.Set(getCPUUsage())
		m.memUsedPercent.Set(getMemoryUsagePercent())

		diskUsed, diskTotal := getDiskUsage()
		m.diskUsedBytes.Set(float64(diskUsed))
		m.diskTotalBytes.Set(float64(diskTotal))
		if diskTotal > 0 {
			m.diskUsedPercent.Set(float64(diskUsed) / float64(diskTotal) * 100)
		}
	}
}

// RecordRequest records an HTTP request
// Per AI.md PART 13: Also increments atomic counter for health endpoint stats
func (m *Metrics) RecordRequest(method, path string, statusCode int, duration time.Duration, reqSize, respSize int64) {
	// Increment atomic counter for health endpoint
	m.totalRequests.Add(1)

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

// SetActiveRequests sets the current number of active requests
func (m *Metrics) SetActiveRequests(n int) {
	m.httpActiveRequests.Set(float64(n))
}

// SetDBConnections sets the current database connection counts
func (m *Metrics) SetDBConnections(open, inUse int) {
	m.dbConnectionsOpen.Set(float64(open))
	m.dbConnectionsInUse.Set(float64(inUse))
}

// SetActiveSessions sets the current number of active sessions
func (m *Metrics) SetActiveSessions(n int) {
	m.authSessionsActive.Set(float64(n))
}

// SetUserCounts sets the user count metrics
func (m *Metrics) SetUserCounts(total, active int) {
	m.usersTotal.Set(float64(total))
	m.usersActive.Set(float64(active))
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

// GetActiveConnections returns current active connections for health endpoint
// Per AI.md PART 13: stats.active_connections must return actual count
func (m *Metrics) GetActiveConnections() int64 {
	return m.activeConnections.Load()
}

// Handler returns an HTTP handler for Prometheus metrics
// Per AI.md PART 29: Uses promhttp.Handler()
func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}

// AuthenticatedHandler returns an HTTP handler with optional Bearer token authentication
func (m *Metrics) AuthenticatedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check for Bearer token if configured
		token := m.config.Server.Metrics.Token
		if token != "" {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if len(auth) < 7 || auth[:7] != "Bearer " {
				http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
				return
			}
			if auth[7:] != token {
				http.Error(w, "Invalid token", http.StatusUnauthorized)
				return
			}
		}

		// Use Prometheus handler
		promhttp.Handler().ServeHTTP(w, r)
	}
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

		// Wrap response writer to capture status code and size
		wrapped := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		// Record request metrics
		duration := time.Since(start)
		path := normalizePath(r.URL.Path)
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
	defer file.Close()

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

// getMemoryUsagePercent returns the system memory usage percentage (Linux only)
func getMemoryUsagePercent() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

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
		return 0
	}

	return float64(memTotal-memAvailable) / float64(memTotal) * 100
}

// getDiskUsage returns the disk usage in bytes (used, total) for the data directory
func getDiskUsage() (used, total uint64) {
	if runtime.GOOS == "windows" {
		return 0, 0
	}

	return getDiskUsageUnix()
}
