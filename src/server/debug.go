package server

import (
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"
	"net/http/pprof"
	"runtime"

	"github.com/apimgr/search/src/config"
)

// registerDebugRoutes registers debug endpoints (DEBUG=true only)
// Per AI.md PART 7: Debug endpoints disabled by default, enabled with DEBUG=true
func (s *Server) registerDebugRoutes(mux *http.ServeMux) {
	if !s.config.IsDebug() {
		return // No debug routes unless DEBUG=true
	}

	// pprof endpoints
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	// expvar
	mux.Handle("/debug/vars", expvar.Handler())

	// Custom debug endpoints
	mux.HandleFunc("/debug/config", s.handleDebugConfig)
	mux.HandleFunc("/debug/routes", s.handleDebugRoutes)
	mux.HandleFunc("/debug/memory", s.handleDebugMemory)
	mux.HandleFunc("/debug/goroutines", s.handleDebugGoroutines)
	mux.HandleFunc("/debug/cache", s.handleDebugCache)
	mux.HandleFunc("/debug/db", s.handleDebugDB)
	mux.HandleFunc("/debug/scheduler", s.handleDebugScheduler)
}

// handleDebugConfig returns sanitized configuration
func (s *Server) handleDebugConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		"users": map[string]interface{}{
			"enabled": s.config.Server.Users.Enabled,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleDebugRoutes lists all registered routes
func (s *Server) handleDebugRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Per AI.md PART 17: Admin path is configurable (default: "admin")
	adminPath := config.GetAdminPath()
	routes := []string{
		"GET /",
		"GET /search",
		"GET /autocomplete",
		"GET /healthz",
		"GET /preferences",
		"GET /static/*",
		fmt.Sprintf("GET /%s/*", adminPath),
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":    runtime.NumGoroutine(),
		"gomaxprocs": runtime.GOMAXPROCS(0),
		"numcpu":   runtime.NumCPU(),
	})
}

// handleDebugCache returns cache statistics
func (s *Server) handleDebugCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
