package server

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// registerTorInternalRoutes registers the internal-only Tor control endpoints
// used by `search tor <subcommand>` to reach the running server's embedded
// Tor process. Per AI.md PART 31: "No REST API for Tor configuration" means
// no PUBLIC REST API — these endpoints are loopback-gated, never documented,
// versioned, or reachable through /api/{api_version}/**, and sit at the same
// internal tier as /server/metrics.
func (s *Server) registerTorInternalRoutes(r chi.Router) {
	r.Get("/server/tor/status", s.loopbackOnly(s.handleTorInternalStatus))
	r.Post("/server/tor/validate", s.loopbackOnly(s.handleTorInternalValidate))
	r.Post("/server/tor/restart", s.loopbackOnly(s.handleTorInternalRestart))
	r.Post("/server/tor/regenerate", s.loopbackOnly(s.handleTorInternalRegenerate))
	r.Post("/server/tor/vanity/start", s.loopbackOnly(s.handleTorInternalVanityStart))
	r.Post("/server/tor/vanity/apply", s.loopbackOnly(s.handleTorInternalVanityApply))
	r.Post("/server/tor/import-keys", s.loopbackOnly(s.handleTorInternalImportKeys))
}

// loopbackOnly rejects any request whose immediate TCP peer is not
// 127.0.0.1/::1 with a 404 (not 403 — the endpoint must not be
// discoverable), per AI.md PART 31. This checks r.RemoteAddr directly,
// never a forwarded-for header, since it is a security boundary rather
// than a display value — a proxy or a malicious client must never be
// able to spoof its way past it.
func (s *Server) loopbackOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

// torUnavailable responds when the server has no Tor service configured
// (Tor binary not found at startup, or the feature is disabled).
func torUnavailable(w http.ResponseWriter) {
	respondJSON(w, http.StatusServiceUnavailable, map[string]any{
		"ok":      false,
		"error":   "TOR_UNAVAILABLE",
		"message": "Tor service is not configured or unavailable",
		"details": map[string]any{},
	})
}

// torError responds with a Tor operation failure. These endpoints are
// loopback-only and consumed exclusively by the trusted `search tor`
// CLI operated by the same user running the server, so the underlying
// error text is genuinely operator-actionable diagnostic detail (e.g.
// "prefix too long", "tor process is not running") rather than a
// public-facing leak — it is surfaced in "message" (never in "error",
// which stays a machine-readable code) per AI.md PART 14/9.
func torError(w http.ResponseWriter, status int, code string, err error) {
	respondJSON(w, status, map[string]any{
		"ok":      false,
		"error":   code,
		"message": err.Error(),
		"details": map[string]any{},
	})
}

// handleTorInternalStatus handles GET /server/tor/status (loopback-only).
func (s *Server) handleTorInternalStatus(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	respondOK(w, http.StatusOK, s.torService.GetTorStatus())
}

// handleTorInternalValidate handles POST /server/tor/validate (loopback-only).
// Validates the running Tor configuration: binary found, service enabled,
// and the control connection responsive.
func (s *Server) handleTorInternalValidate(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	status := s.torService.GetTorStatus()
	valid := true
	var issues []string
	if enabled, _ := status["enabled"].(bool); !enabled {
		valid = false
		issues = append(issues, "tor is not enabled in server.yml")
	}
	if running, _ := status["running"].(bool); !running {
		valid = false
		issues = append(issues, "tor process is not running")
	}
	respondOK(w, http.StatusOK, map[string]any{
		"valid":  valid,
		"issues": issues,
		"status": status,
	})
}

// handleTorInternalRestart handles POST /server/tor/restart (loopback-only).
func (s *Server) handleTorInternalRestart(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	if err := s.torService.RestartTorService(); err != nil {
		torError(w, http.StatusInternalServerError, "TOR_RESTART_FAILED", err)
		return
	}
	respondOK(w, http.StatusOK, s.torService.GetTorStatus())
}

// handleTorInternalRegenerate handles POST /server/tor/regenerate (loopback-only).
func (s *Server) handleTorInternalRegenerate(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	addr, err := s.torService.RegenerateAddress()
	if err != nil {
		torError(w, http.StatusInternalServerError, "TOR_REGENERATE_FAILED", err)
		return
	}
	respondOK(w, http.StatusOK, map[string]any{"onion_address": addr})
}

// torVanityStartRequest is the JSON body for POST /server/tor/vanity/start.
type torVanityStartRequest struct {
	Prefix string `json:"prefix"`
}

// handleTorInternalVanityStart handles POST /server/tor/vanity/start (loopback-only).
func (s *Server) handleTorInternalVanityStart(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	var req torVanityStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"ok":      false,
			"error":   "BAD_REQUEST",
			"message": "Invalid request body",
			"details": map[string]any{},
		})
		return
	}
	if err := s.torService.GenerateVanity(strings.TrimSpace(req.Prefix)); err != nil {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", err)
		return
	}
	respondOK(w, http.StatusOK, map[string]any{"started": true})
}

// handleTorInternalVanityApply handles POST /server/tor/vanity/apply (loopback-only).
func (s *Server) handleTorInternalVanityApply(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	addr, err := s.torService.ApplyVanityAddress()
	if err != nil {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", err)
		return
	}
	respondOK(w, http.StatusOK, map[string]any{"onion_address": addr})
}

// handleTorInternalImportKeys handles POST /server/tor/import-keys (loopback-only).
// The CLI reads the key file itself and streams the raw private key bytes as
// the request body.
func (s *Server) handleTorInternalImportKeys(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"ok":      false,
			"error":   "BAD_REQUEST",
			"message": "Failed to read request body",
			"details": map[string]any{},
		})
		return
	}
	addr, err := s.torService.ImportKeys(body)
	if err != nil {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", err)
		return
	}
	respondOK(w, http.StatusOK, map[string]any{"onion_address": addr})
}
