package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/apimgr/search/src/config"
)

// Metrics collects server metrics
type Metrics struct {
	config    *config.Config
	startTime time.Time

	// Request metrics
	requestsTotal     atomic.Int64
	requestsSuccess   atomic.Int64
	requestsError     atomic.Int64
	requestLatencySum atomic.Int64
	requestLatencyN   atomic.Int64

	// Search metrics
	searchesTotal    atomic.Int64
	searchLatencySum atomic.Int64
	searchLatencyN   atomic.Int64

	// Engine metrics
	engineRequests map[string]*atomic.Int64
	engineErrors   map[string]*atomic.Int64
	engineMu       sync.RWMutex

	// Rate limit metrics
	rateLimitHits atomic.Int64

	// Custom metrics
	custom   map[string]*CustomMetric
	customMu sync.RWMutex
}

// CustomMetric represents a custom metric
type CustomMetric struct {
	Name        string
	Description string
	Type        string // counter, gauge, histogram
	Value       atomic.Int64
	Labels      map[string]string
}

// NewMetrics creates a new metrics collector
func NewMetrics(cfg *config.Config) *Metrics {
	return &Metrics{
		config:         cfg,
		startTime:      time.Now(),
		engineRequests: make(map[string]*atomic.Int64),
		engineErrors:   make(map[string]*atomic.Int64),
		custom:         make(map[string]*CustomMetric),
	}
}

// RecordRequest records an HTTP request
func (m *Metrics) RecordRequest(statusCode int, latency time.Duration) {
	m.requestsTotal.Add(1)

	if statusCode >= 200 && statusCode < 400 {
		m.requestsSuccess.Add(1)
	} else {
		m.requestsError.Add(1)
	}

	m.requestLatencySum.Add(latency.Microseconds())
	m.requestLatencyN.Add(1)
}

// RecordSearch records a search operation
func (m *Metrics) RecordSearch(latency time.Duration) {
	m.searchesTotal.Add(1)
	m.searchLatencySum.Add(latency.Microseconds())
	m.searchLatencyN.Add(1)
}

// RecordEngineRequest records a request to a search engine
func (m *Metrics) RecordEngineRequest(engine string) {
	m.engineMu.Lock()
	defer m.engineMu.Unlock()

	if _, ok := m.engineRequests[engine]; !ok {
		m.engineRequests[engine] = &atomic.Int64{}
	}
	m.engineRequests[engine].Add(1)
}

// RecordEngineError records an error from a search engine
func (m *Metrics) RecordEngineError(engine string) {
	m.engineMu.Lock()
	defer m.engineMu.Unlock()

	if _, ok := m.engineErrors[engine]; !ok {
		m.engineErrors[engine] = &atomic.Int64{}
	}
	m.engineErrors[engine].Add(1)
}

// RecordRateLimitHit records a rate limit hit
func (m *Metrics) RecordRateLimitHit() {
	m.rateLimitHits.Add(1)
}

// GetStats returns current metrics stats
func (m *Metrics) GetStats() *MetricsStats {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate average latencies
	avgRequestLatency := float64(0)
	if n := m.requestLatencyN.Load(); n > 0 {
		avgRequestLatency = float64(m.requestLatencySum.Load()) / float64(n) / 1000 // ms
	}

	avgSearchLatency := float64(0)
	if n := m.searchLatencyN.Load(); n > 0 {
		avgSearchLatency = float64(m.searchLatencySum.Load()) / float64(n) / 1000 // ms
	}

	// Get engine stats
	engineStats := make(map[string]EngineStats)
	m.engineMu.RLock()
	for engine, requests := range m.engineRequests {
		errors := int64(0)
		if e, ok := m.engineErrors[engine]; ok {
			errors = e.Load()
		}
		engineStats[engine] = EngineStats{
			Requests: requests.Load(),
			Errors:   errors,
		}
	}
	m.engineMu.RUnlock()

	stats := &MetricsStats{
		Uptime:            time.Since(m.startTime),
		RequestsTotal:     m.requestsTotal.Load(),
		RequestsSuccess:   m.requestsSuccess.Load(),
		RequestsError:     m.requestsError.Load(),
		AvgRequestLatency: avgRequestLatency,
		SearchesTotal:     m.searchesTotal.Load(),
		AvgSearchLatency:  avgSearchLatency,
		RateLimitHits:     m.rateLimitHits.Load(),
		EngineStats:       engineStats,
		MemAlloc:          memStats.Alloc,
		MemTotalAlloc:     memStats.TotalAlloc,
		MemSys:            memStats.Sys,
		NumGoroutine:      runtime.NumGoroutine(),
	}

	// Add system metrics if enabled
	if m.config.Server.Metrics.IncludeSystem {
		stats.CPUUsage = getCPUUsage()
		stats.MemUsedPercent = getMemoryUsagePercent()
		diskUsed, diskTotal := getDiskUsage()
		stats.DiskUsedBytes = diskUsed
		stats.DiskTotalBytes = diskTotal
		if diskTotal > 0 {
			stats.DiskUsedPct = float64(diskUsed) / float64(diskTotal) * 100
		}
	}

	return stats
}

// MetricsStats holds metrics statistics
type MetricsStats struct {
	Uptime            time.Duration          `json:"uptime"`
	RequestsTotal     int64                  `json:"requests_total"`
	RequestsSuccess   int64                  `json:"requests_success"`
	RequestsError     int64                  `json:"requests_error"`
	AvgRequestLatency float64                `json:"avg_request_latency_ms"`
	SearchesTotal     int64                  `json:"searches_total"`
	AvgSearchLatency  float64                `json:"avg_search_latency_ms"`
	RateLimitHits     int64                  `json:"rate_limit_hits"`
	EngineStats       map[string]EngineStats `json:"engine_stats"`
	MemAlloc          uint64                 `json:"mem_alloc"`
	MemTotalAlloc     uint64                 `json:"mem_total_alloc"`
	MemSys            uint64                 `json:"mem_sys"`
	NumGoroutine      int                    `json:"num_goroutine"`
	// System metrics (when include_system is enabled)
	CPUUsage       float64 `json:"cpu_usage_percent,omitempty"`
	MemUsedPercent float64 `json:"mem_used_percent,omitempty"`
	DiskUsedBytes  uint64  `json:"disk_used_bytes,omitempty"`
	DiskTotalBytes uint64  `json:"disk_total_bytes,omitempty"`
	DiskUsedPct    float64 `json:"disk_used_percent,omitempty"`
}

// EngineStats holds per-engine statistics
type EngineStats struct {
	Requests int64 `json:"requests"`
	Errors   int64 `json:"errors"`
}

// Handler returns an HTTP handler for metrics
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		format := r.URL.Query().Get("format")

		if format == "prometheus" {
			m.writePrometheus(w)
		} else {
			m.writeJSON(w)
		}
	}
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

		// Default to Prometheus format for /metrics endpoint
		m.writePrometheus(w)
	}
}

// writeJSON writes metrics in JSON format
func (m *Metrics) writeJSON(w http.ResponseWriter) {
	stats := m.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"version": config.Version,
		"metrics": stats,
	})
}

// writePrometheus writes metrics in Prometheus format
func (m *Metrics) writePrometheus(w http.ResponseWriter) {
	stats := m.GetStats()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	// Write metrics in Prometheus exposition format
	fmt.Fprintf(w, "# HELP search_uptime_seconds Server uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE search_uptime_seconds gauge\n")
	fmt.Fprintf(w, "search_uptime_seconds %.2f\n", stats.Uptime.Seconds())

	fmt.Fprintf(w, "# HELP search_requests_total Total number of HTTP requests\n")
	fmt.Fprintf(w, "# TYPE search_requests_total counter\n")
	fmt.Fprintf(w, "search_requests_total %d\n", stats.RequestsTotal)

	fmt.Fprintf(w, "# HELP search_requests_success_total Total number of successful HTTP requests\n")
	fmt.Fprintf(w, "# TYPE search_requests_success_total counter\n")
	fmt.Fprintf(w, "search_requests_success_total %d\n", stats.RequestsSuccess)

	fmt.Fprintf(w, "# HELP search_requests_error_total Total number of failed HTTP requests\n")
	fmt.Fprintf(w, "# TYPE search_requests_error_total counter\n")
	fmt.Fprintf(w, "search_requests_error_total %d\n", stats.RequestsError)

	fmt.Fprintf(w, "# HELP search_request_latency_avg_ms Average request latency in milliseconds\n")
	fmt.Fprintf(w, "# TYPE search_request_latency_avg_ms gauge\n")
	fmt.Fprintf(w, "search_request_latency_avg_ms %.2f\n", stats.AvgRequestLatency)

	fmt.Fprintf(w, "# HELP search_searches_total Total number of search queries\n")
	fmt.Fprintf(w, "# TYPE search_searches_total counter\n")
	fmt.Fprintf(w, "search_searches_total %d\n", stats.SearchesTotal)

	fmt.Fprintf(w, "# HELP search_search_latency_avg_ms Average search latency in milliseconds\n")
	fmt.Fprintf(w, "# TYPE search_search_latency_avg_ms gauge\n")
	fmt.Fprintf(w, "search_search_latency_avg_ms %.2f\n", stats.AvgSearchLatency)

	fmt.Fprintf(w, "# HELP search_rate_limit_hits_total Total number of rate limit hits\n")
	fmt.Fprintf(w, "# TYPE search_rate_limit_hits_total counter\n")
	fmt.Fprintf(w, "search_rate_limit_hits_total %d\n", stats.RateLimitHits)

	fmt.Fprintf(w, "# HELP search_engine_requests_total Total requests per search engine\n")
	fmt.Fprintf(w, "# TYPE search_engine_requests_total counter\n")
	for engine, es := range stats.EngineStats {
		fmt.Fprintf(w, "search_engine_requests_total{engine=\"%s\"} %d\n", engine, es.Requests)
	}

	fmt.Fprintf(w, "# HELP search_engine_errors_total Total errors per search engine\n")
	fmt.Fprintf(w, "# TYPE search_engine_errors_total counter\n")
	for engine, es := range stats.EngineStats {
		fmt.Fprintf(w, "search_engine_errors_total{engine=\"%s\"} %d\n", engine, es.Errors)
	}

	fmt.Fprintf(w, "# HELP search_memory_alloc_bytes Currently allocated memory in bytes\n")
	fmt.Fprintf(w, "# TYPE search_memory_alloc_bytes gauge\n")
	fmt.Fprintf(w, "search_memory_alloc_bytes %d\n", stats.MemAlloc)

	fmt.Fprintf(w, "# HELP search_memory_sys_bytes Total memory obtained from system\n")
	fmt.Fprintf(w, "# TYPE search_memory_sys_bytes gauge\n")
	fmt.Fprintf(w, "search_memory_sys_bytes %d\n", stats.MemSys)

	fmt.Fprintf(w, "# HELP search_goroutines Number of active goroutines\n")
	fmt.Fprintf(w, "# TYPE search_goroutines gauge\n")
	fmt.Fprintf(w, "search_goroutines %d\n", stats.NumGoroutine)

	// System metrics (when include_system is enabled)
	if m.config.Server.Metrics.IncludeSystem {
		fmt.Fprintf(w, "# HELP search_cpu_usage_percent CPU usage percentage\n")
		fmt.Fprintf(w, "# TYPE search_cpu_usage_percent gauge\n")
		fmt.Fprintf(w, "search_cpu_usage_percent %.2f\n", stats.CPUUsage)

		fmt.Fprintf(w, "# HELP search_memory_used_percent System memory usage percentage\n")
		fmt.Fprintf(w, "# TYPE search_memory_used_percent gauge\n")
		fmt.Fprintf(w, "search_memory_used_percent %.2f\n", stats.MemUsedPercent)

		fmt.Fprintf(w, "# HELP search_disk_used_bytes Disk space used in bytes\n")
		fmt.Fprintf(w, "# TYPE search_disk_used_bytes gauge\n")
		fmt.Fprintf(w, "search_disk_used_bytes %d\n", stats.DiskUsedBytes)

		fmt.Fprintf(w, "# HELP search_disk_total_bytes Total disk space in bytes\n")
		fmt.Fprintf(w, "# TYPE search_disk_total_bytes gauge\n")
		fmt.Fprintf(w, "search_disk_total_bytes %d\n", stats.DiskTotalBytes)

		fmt.Fprintf(w, "# HELP search_disk_used_percent Disk space usage percentage\n")
		fmt.Fprintf(w, "# TYPE search_disk_used_percent gauge\n")
		fmt.Fprintf(w, "search_disk_used_percent %.2f\n", stats.DiskUsedPct)
	}
}

// MetricsMiddleware creates middleware for recording request metrics
func (m *Metrics) MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		m.RecordRequest(wrapped.statusCode, time.Since(start))
	})
}

// metricsResponseWriter wraps http.ResponseWriter to capture status code for metrics
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *metricsResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// getCPUUsage returns the current CPU usage percentage (Linux only)
func getCPUUsage() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	// Read /proc/stat for CPU stats
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

	// Parse CPU times: user, nice, system, idle
	user, _ := strconv.ParseFloat(fields[1], 64)
	nice, _ := strconv.ParseFloat(fields[2], 64)
	system, _ := strconv.ParseFloat(fields[3], 64)
	idle, _ := strconv.ParseFloat(fields[4], 64)

	total := user + nice + system + idle
	if total == 0 {
		return 0
	}

	// Return non-idle percentage
	return ((total - idle) / total) * 100
}

// getMemoryUsagePercent returns the system memory usage percentage (Linux only)
func getMemoryUsagePercent() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}

	// Read /proc/meminfo
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

		// Exit early if we have what we need
		if memTotal > 0 && memAvailable > 0 {
			break
		}
	}

	if memTotal == 0 {
		return 0
	}

	// Return used percentage
	return float64(memTotal-memAvailable) / float64(memTotal) * 100
}

// getDiskUsage returns the disk usage in bytes (used, total) for the data directory
func getDiskUsage() (used, total uint64) {
	if runtime.GOOS == "windows" {
		return 0, 0
	}

	// Use syscall.Statfs to get disk stats
	// This is done via the unix-specific file
	return getDiskUsageUnix()
}
