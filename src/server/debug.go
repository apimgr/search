package server

import (
	"bytes"
	"encoding/json"
	"expvar"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"runtime"

	"github.com/go-chi/chi/v5"
)

// registerDebugRoutes registers debug endpoints under /debug/* (DEBUG=true only).
// Per AI.md PART 6: pprof, expvar, and custom debug endpoints.
func (s *Server) registerDebugRoutes(r chi.Router) {
	if !s.config.IsDebug() {
		return
	}

	r.Route("/debug", func(r chi.Router) {
		// Debug mode exposes process and configuration details, so every debug
		// endpoint still requires the operator credential.
		r.Use(func(next http.Handler) http.Handler {
			return s.RequireOperator(func(w http.ResponseWriter, req *http.Request) {
				next.ServeHTTP(w, req)
			})
		})

		r.Get("/pprof", pprof.Index)
		r.Get("/pprof/cmdline", s.handleDebugCommandLine)
		r.Get("/pprof/profile", pprof.Profile)
		r.Get("/pprof/symbol", pprof.Symbol)
		r.Get("/pprof/trace", pprof.Trace)
		r.Get("/pprof/heap", pprof.Handler("heap").ServeHTTP)
		r.Get("/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
		r.Get("/pprof/allocs", pprof.Handler("allocs").ServeHTTP)
		r.Get("/pprof/block", pprof.Handler("block").ServeHTTP)
		r.Get("/pprof/mutex", pprof.Handler("mutex").ServeHTTP)
		r.Get("/pprof/threadcreate", pprof.Handler("threadcreate").ServeHTTP)

		// expvar with its command-line value redacted to the executable name.
		r.Get("/vars", s.handleDebugVars)

		// Custom debug endpoints
		r.Get("/config", s.handleDebugConfig)
		r.Get("/routes", s.handleDebugRoutes)
		r.Get("/cache", s.handleDebugCache)
		r.Get("/db", s.handleDebugDB)
		r.Get("/scheduler", s.handleDebugScheduler)
		r.Get("/memory", s.handleDebugMemory)
		r.Get("/goroutines", s.handleDebugGoroutines)
		r.Get("/error500", func(w http.ResponseWriter, r *http.Request) {
			manager := s.getI18nManager()
			lang := manager.DetectLanguage(r)
			s.handleError(w, r, http.StatusInternalServerError,
				manager.T(lang, "errors.server_error_title"),
				manager.T(lang, "errors.server_error_message"))
		})
	})
}

// processBinaryName returns only the executable's base name for diagnostic
// output; process arguments may contain credentials and are never exposed.
func processBinaryName() string {
	if len(os.Args) == 0 {
		return "search"
	}
	name := filepath.Base(os.Args[0])
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "search"
	}
	return name
}

func (s *Server) handleDebugCommandLine(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writeResponseText(w, processBinaryName()+"\n")
}

func (s *Server) handleDebugVars(w http.ResponseWriter, _ *http.Request) {
	values := make(map[string]any)
	expvar.Do(func(value expvar.KeyValue) {
		if value.Key == "cmdline" {
			values[value.Key] = []string{processBinaryName()}
			return
		}
		var decoded any
		if err := json.Unmarshal([]byte(value.Value.String()), &decoded); err != nil {
			values[value.Key] = value.Value.String()
			return
		}
		values[value.Key] = decoded
	})

	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(values); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to encode runtime variables")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	writeResponseBytes(w, body.Bytes())
}

// handleDebugConfig returns sanitized configuration
func (s *Server) handleDebugConfig(w http.ResponseWriter, r *http.Request) {
	// Return config with sensitive values redacted
	sanitized := s.config.Sanitized()
	respondOK(w, http.StatusOK, sanitized)
}

// handleDebugRoutes returns all registered routes
func (s *Server) handleDebugRoutes(w http.ResponseWriter, r *http.Request) {
	routes := []map[string]string{}

	walkFunc := func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		routes = append(routes, map[string]string{
			"method": method,
			"route":  route,
		})
		return nil
	}

	if err := chi.Walk(s.router, walkFunc); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to walk routes")
		return
	}

	respondOK(w, http.StatusOK, map[string]any{
		"count":  len(routes),
		"routes": routes,
	})
}

// handleDebugMemory returns memory statistics
func (s *Server) handleDebugMemory(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	respondOK(w, http.StatusOK, map[string]any{
		"alloc_mb":       m.Alloc / 1024 / 1024,
		"total_alloc_mb": m.TotalAlloc / 1024 / 1024,
		"sys_mb":         m.Sys / 1024 / 1024,
		"num_gc":         m.NumGC,
		"heap_objects":   m.HeapObjects,
		"goroutines":     runtime.NumGoroutine(),
	})
}

// handleDebugGoroutines returns goroutine count and stack traces
func (s *Server) handleDebugGoroutines(w http.ResponseWriter, r *http.Request) {
	// 1MB buffer for stack traces
	buf := make([]byte, 1024*1024)
	// true = include all goroutines
	n := runtime.Stack(buf, true)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf[:n])
}

// handleDebugCache returns cache statistics
func (s *Server) handleDebugCache(w http.ResponseWriter, r *http.Request) {
	if s.cache == nil {
		respondOK(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	stats := s.cache.Stats()
	respondOK(w, http.StatusOK, stats)
}

// handleDebugDB returns database statistics
func (s *Server) handleDebugDB(w http.ResponseWriter, r *http.Request) {
	if s.db == nil {
		respondOK(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	stats := s.db.Stats()
	respondOK(w, http.StatusOK, map[string]any{
		"open_connections":    stats.OpenConnections,
		"in_use":              stats.InUse,
		"idle":                stats.Idle,
		"wait_count":          stats.WaitCount,
		"wait_duration_ms":    stats.WaitDuration.Milliseconds(),
		"max_idle_closed":     stats.MaxIdleClosed,
		"max_lifetime_closed": stats.MaxLifetimeClosed,
	})
}

// handleDebugScheduler returns scheduler task status
func (s *Server) handleDebugScheduler(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		respondOK(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	tasks := s.scheduler.Status()
	respondOK(w, http.StatusOK, tasks)
}
