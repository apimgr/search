package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/apimgr/search/src/version"
)

// metricsServiceNames are the three metrics services defined by AI.md PART 20.
// Each has its own bearer token so rotating one never breaks the others.
const (
	metricsServicePrometheus = "prometheus"
	metricsServiceGrafana    = "grafana"
	metricsServiceLoki       = "loki"
)

// metricsAuth wraps a metrics service handler with mandatory per-service
// bearer-token authentication per AI.md PART 20 "Authentication":
//   - auth.allow_unauthenticated (default false) skips the check entirely; it is
//     a firewalled-internal-network escape hatch only
//   - an empty token disables that service -> 403 with an empty body
//   - the token travels in the Authorization header only; query-string tokens
//     are FORBIDDEN because they leak into access logs and proxies
//   - comparison is constant-time and never logs or echoes the token
func (s *Server) metricsAuth(service string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := s.config.Server.Metrics.Auth

		// Firewalled-internal-network escape hatch only (default false)
		if auth.AllowUnauthenticated {
			next.ServeHTTP(w, r)
			return
		}

		token := metricsServiceToken(s, service)
		if token == "" {
			// Empty token = service disabled: 403 with an empty body
			w.WriteHeader(http.StatusForbidden)
			return
		}

		header := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			localizedHTTPError(w, r, http.StatusUnauthorized, "errors.unauthorized")
			return
		}

		// Compare over fixed-length SHA-256 digests so timing leaks neither the
		// token value nor its length.
		presented := sha256.Sum256([]byte(strings.TrimPrefix(header, prefix)))
		expected := sha256.Sum256([]byte(token))
		if subtle.ConstantTimeCompare(presented[:], expected[:]) != 1 {
			localizedHTTPError(w, r, http.StatusUnauthorized, "errors.invalid_token")
			return
		}

		next.ServeHTTP(w, r)
	}
}

// metricsServiceToken returns the configured bearer token for a metrics service.
// The value is never logged, echoed, or included in any response.
func metricsServiceToken(s *Server, service string) string {
	tokens := s.config.Server.Metrics.Auth.Tokens
	switch service {
	case metricsServicePrometheus:
		return tokens.Prometheus
	case metricsServiceGrafana:
		return tokens.Grafana
	case metricsServiceLoki:
		return tokens.Loki
	}
	return ""
}

// registerMetricsRoutes mounts the canonical metrics endpoint set per AI.md
// PART 20 "Endpoints". Every alias invokes the SAME handler and is never a
// redirect, because redirects break Prometheus scrapers.
func (s *Server) registerMetricsRoutes(r chi.Router) {
	s.logDisabledMetricsServices()

	prom := s.metricsAuth(metricsServicePrometheus, s.metrics.Handler())
	grafana := s.metricsAuth(metricsServiceGrafana, s.metricsGrafanaHandler())
	loki := s.metricsAuth(metricsServiceLoki, s.metricsLokiHandler())

	mount := func(prefix string) {
		// The bare path serves the SAME handler as /prometheus
		r.Method(http.MethodGet, prefix, prom)
		r.Method(http.MethodGet, prefix+"/prometheus", prom)
		r.Method(http.MethodGet, prefix+"/grafana", grafana)
		r.Method(http.MethodGet, prefix+"/loki", loki)
	}

	// Frontend namespace, mirroring the healthz layout
	mount("/server/metrics")
	// Versioned API path
	mount(version.APIPrefix + "/server/metrics")
	// Unversioned machine-friendly alias for the current api version
	mount("/api/metrics")
	// Root alias, gated on server.metrics.root.enabled (default true)
	if s.config.Server.Metrics.Root.Enabled {
		mount("/metrics")
	}
}

// logDisabledMetricsServices records once at startup, to the log files only,
// which metrics services are disabled because their token is empty. Per AI.md
// PART 20 the reason is logged; the token value never is.
func (s *Server) logDisabledMetricsServices() {
	if s.logManager == nil {
		return
	}

	if s.config.Server.Metrics.Auth.AllowUnauthenticated {
		s.logManager.Server().Warn("Metrics authentication disabled by server.metrics.auth.allow_unauthenticated - use only on a firewalled internal network")
		return
	}

	for _, service := range []string{metricsServicePrometheus, metricsServiceGrafana, metricsServiceLoki} {
		if metricsServiceToken(s, service) == "" {
			s.logManager.Server().Warn("Metrics service disabled: no bearer token configured", map[string]interface{}{
				"service":    service,
				"config_key": "server.metrics.auth.tokens." + service,
			})
		}
	}
}
