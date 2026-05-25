package server

import (
	"encoding/json"
	"expvar"
	"net/http"
	"net/http/pprof"
	"runtime"

	"github.com/go-chi/chi/v5"
)

// registerDebugRoutes registers debug endpoints (DEBUG=true only)
// Per AI.md PART 7: Debug endpoints disabled by default, enabled with DEBUG=true
func (s *Server) registerDebugRoutes(r chi.Router) {
	if !s.config.IsDebug() {
		// No debug routes unless DEBUG=true
		return
	}

	// pprof endpoints
	// /debug/pprof/* catches index and named sub-profiles routed through pprof.Index
	r.HandleFunc("/debug/pprof/*", pprof.Index)
	r.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	r.HandleFunc("/debug/pprof/profile", pprof.Profile)
	r.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	r.HandleFunc("/debug/pprof/trace", pprof.Trace)
	r.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	r.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	r.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	r.Handle("/debug/pprof/block", pprof.Handler("block"))
	r.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	r.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	// expvar
	r.Handle("/debug/vars", expvar.Handler())

	// Custom debug endpoints
	r.HandleFunc("/debug/config", s.handleDebugConfig)
	r.HandleFunc("/debug/routes", s.handleDebugRoutes)
	r.HandleFunc("/debug/memory", s.handleDebugMemory)
	r.HandleFunc("/debug/goroutines", s.handleDebugGoroutines)
	r.HandleFunc("/debug/cache", s.handleDebugCache)
	r.HandleFunc("/debug/db", s.handleDebugDB)
	r.HandleFunc("/debug/scheduler", s.handleDebugScheduler)
}

// handleDebugConfig returns sanitized configuration
func (s *Server) handleDebugConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}

	// Return config with sensitive values redacted
	config := map[string]interface{}{
		"mode":    s.config.Server.Mode,
		"port":    s.config.Server.Port,
		"address": s.config.Server.Address,
		"title":   s.config.Server.Title,
		"ssl": map[string]interface{}{
			"enabled": s.config.Server.SSL.Enabled,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleDebugRoutes lists all registered routes
func (s *Server) handleDebugRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}

	routes := []string{
		"GET /",
		"GET /search",
		"GET /autocomplete",
		"GET /server/healthz",
		"GET /healthz",
		"GET /preferences",
		"GET /static/*",
		"GET /api/v1/*",
		"GET /debug/*",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"routes": routes,
	})
}

// handleDebugMemory returns memory statistics
func (s *Server) handleDebugMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := map[string]interface{}{
		"alloc":       m.Alloc,
		"total_alloc": m.TotalAlloc,
		"sys":         m.Sys,
		"num_gc":      m.NumGC,
		"heap_alloc":  m.HeapAlloc,
		"heap_sys":    m.HeapSys,
		"heap_idle":   m.HeapIdle,
		"heap_inuse":  m.HeapInuse,
		"stack_inuse": m.StackInuse,
		"stack_sys":   m.StackSys,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDebugGoroutines returns goroutine count
func (s *Server) handleDebugGoroutines(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":      runtime.NumGoroutine(),
		"gomaxprocs": runtime.GOMAXPROCS(0),
		"numcpu":     runtime.NumCPU(),
	})
}

// handleDebugCache returns cache statistics
func (s *Server) handleDebugCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}

	// Cache stats from aggregator if available
	stats := map[string]interface{}{
		"enabled": s.aggregator != nil,
	}

	if s.aggregator != nil {
		stats["cache_enabled"] = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDebugDB returns database statistics
// Per AI.md PART 6: Debug endpoint for database stats
func (s *Server) handleDebugDB(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}

	stats := map[string]interface{}{
		"enabled": s.dbManager != nil,
	}

	if s.dbManager != nil && s.dbManager.ServerDB() != nil {
		dbStats := s.dbManager.ServerDB().SQL().Stats()
		stats["open_connections"] = dbStats.OpenConnections
		stats["in_use"] = dbStats.InUse
		stats["idle"] = dbStats.Idle
		stats["wait_count"] = dbStats.WaitCount
		stats["wait_duration_ms"] = dbStats.WaitDuration.Milliseconds()
		stats["max_idle_closed"] = dbStats.MaxIdleClosed
		stats["max_lifetime_closed"] = dbStats.MaxLifetimeClosed
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleDebugScheduler returns scheduler task status
func (s *Server) handleDebugScheduler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}

	stats := map[string]interface{}{
		"enabled": s.scheduler != nil,
	}

	if s.scheduler != nil {
		stats["running"] = true
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
