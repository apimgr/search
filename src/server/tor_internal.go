package server

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"github.com/apimgr/search/src/service"
	"github.com/go-chi/chi/v5"
)

// registerTorInternalRoutes registers the internal-only Tor control endpoints
// used by `search --maintenance tor <subcommand>` to reach the running server's embedded
// Tor process. Per AI.md PART 31: "No REST API for Tor configuration" means
// no PUBLIC REST API — these endpoints are loopback-gated, never documented,
// versioned, or reachable through /api/{api_version}/**, and sit at the same
// internal tier as /server/metrics.
func (s *Server) registerTorInternalRoutes(r chi.Router) {
	r.Handle("/server/tor/status", s.loopbackOnly(torMethodOnly(http.MethodGet, s.handleTorInternalStatus)))
	r.Handle("/server/tor/validate", s.loopbackOnly(torMethodOnly(http.MethodPost, s.handleTorInternalValidate)))
	r.Handle("/server/tor/restart", s.loopbackOnly(torMethodOnly(http.MethodPost, s.handleTorInternalRestart)))
	r.Handle("/server/tor/regenerate", s.loopbackOnly(torMethodOnly(http.MethodPost, s.handleTorInternalRegenerate)))
	r.Handle("/server/tor/vanity/start", s.loopbackOnly(torMethodOnly(http.MethodPost, s.handleTorInternalVanityStart)))
	r.Handle("/server/tor/vanity/stop", s.loopbackOnly(torMethodOnly(http.MethodPost, s.handleTorInternalVanityStop)))
	r.Handle("/server/tor/vanity/apply", s.loopbackOnly(torMethodOnly(http.MethodPost, s.handleTorInternalVanityApply)))
	r.Handle("/server/tor/import-keys", s.loopbackOnly(torMethodOnly(http.MethodPost, s.handleTorInternalImportKeys)))
}

func torMethodOnly(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

// loopbackOnly rejects any request whose immediate TCP peer is not
// 127.0.0.1/::1 with a 404 (not 403 — the endpoint must not be
// discoverable), per AI.md PART 31. This checks r.RemoteAddr directly,
// never a forwarded-for header, since it is a security boundary rather
// than a display value — a proxy or a malicious client must never be
// able to spoof its way past it.
//
// Requests relayed from the hidden service are also rejected: they present a
// loopback RemoteAddr (the local Tor daemon) but are externally controlled,
// so loopback-only endpoints must never serve them (AI.md PART 12 line
// 16097 — the peer address is meaningless for a Tor request).
func (s *Server) loopbackOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.torRelayed(r) {
			http.NotFound(w, r)
			return
		}
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
	torError(w, http.StatusInternalServerError, "SERVER_ERROR", errors.New("tor service is not configured or unavailable"))
}

// torError responds with a Tor operation failure. These endpoints are
// loopback-only and consumed exclusively by the trusted `search --maintenance tor`
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
		respondOK(w, http.StatusOK, map[string]any{
			"running":             false,
			"version":             "",
			"bootstrap_percent":   0,
			"circuit_established": false,
			"vanity": map[string]any{
				"state": "idle", "prefix": "", "workers": 0, "attempts": 0,
				"rate": 0, "elapsed_seconds": 0, "candidates": []string{},
			},
		})
		return
	}
	respondOK(w, http.StatusOK, s.torService.ControlStatus())
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
	errors := make([]map[string]string, 0)
	if enabled, _ := status["enabled"].(bool); !enabled {
		valid = false
		errors = append(errors, map[string]string{"field": "server.tor.enabled", "message": "Tor is not enabled in server.yml"})
	}
	if running, _ := status["running"].(bool); !running {
		valid = false
		errors = append(errors, map[string]string{"field": "server.tor.running", "message": "Tor process is not running"})
	}
	respondOK(w, http.StatusOK, map[string]any{
		"valid":  valid,
		"errors": errors,
	})
}

// handleTorInternalRestart handles POST /server/tor/restart (loopback-only).
func (s *Server) handleTorInternalRestart(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	if err := s.torService.RestartTorService(); err != nil {
		torError(w, http.StatusInternalServerError, "SERVER_ERROR", err)
		return
	}
	status := s.torService.ControlStatus()
	onionAddress, _ := status["onion_address"].(string)
	respondOK(w, http.StatusOK, map[string]any{"restarted": true, "onion_address": onionAddress})
}

// handleTorInternalRegenerate handles POST /server/tor/regenerate (loopback-only).
func (s *Server) handleTorInternalRegenerate(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	var req torConfirmRequest
	if !decodeTorJSON(w, r, &req) {
		return
	}
	if !req.Confirm {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", errTorConfirmationRequired)
		return
	}
	previousAddress, _ := s.torService.ControlStatus()["onion_address"].(string)
	addr, err := s.torService.RegenerateAddress()
	if err != nil {
		torError(w, http.StatusInternalServerError, "SERVER_ERROR", err)
		return
	}
	respondOK(w, http.StatusOK, map[string]any{"onion_address": addr, "previous_address": previousAddress})
}

type torConfirmRequest struct {
	Confirm bool `json:"confirm"`
}

var errTorConfirmationRequired = errors.New("confirm must be true")

func decodeTorJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": "BAD_REQUEST", "message": "Invalid request body", "details": map[string]any{},
		})
		return false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		respondJSON(w, http.StatusBadRequest, map[string]any{
			"ok": false, "error": "BAD_REQUEST", "message": "Invalid request body", "details": map[string]any{},
		})
		return false
	}
	return true
}

// torVanityStartRequest is the JSON body for POST /server/tor/vanity/start.
type torVanityStartRequest struct {
	Prefix  string `json:"prefix"`
	Workers *int   `json:"workers,omitempty"`
}

type torVanityApplyRequest struct {
	Address string `json:"address"`
	Confirm bool   `json:"confirm"`
}

// handleTorInternalVanityStart handles POST /server/tor/vanity/start (loopback-only).
func (s *Server) handleTorInternalVanityStart(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	var req torVanityStartRequest
	if !decodeTorJSON(w, r, &req) {
		return
	}
	workers := 0
	if req.Workers != nil {
		workers = *req.Workers
		if workers < 1 {
			torError(w, http.StatusBadRequest, "VALIDATION_FAILED", errors.New("workers must be at least 1"))
			return
		}
	}
	if err := s.torService.GenerateVanityWithWorkers(req.Prefix, workers); err != nil {
		var conflict *service.VanitySearchConflictError
		if errors.As(err, &conflict) {
			respondJSON(w, http.StatusConflict, map[string]any{
				"ok": false, "error": "CONFLICT", "message": err.Error(), "details": map[string]any{"prefix": conflict.Prefix},
			})
			return
		}
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", err)
		return
	}
	progress := s.torService.GetVanityProgress()
	respondOK(w, http.StatusAccepted, map[string]any{"state": "running", "prefix": progress.Prefix, "workers": progress.Workers})
}

// handleTorInternalVanityStop cancels the running search while retaining any
// candidates already written to disk.
func (s *Server) handleTorInternalVanityStop(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	wasRunning := s.torService.GetVanityProgress().Running
	if wasRunning {
		s.torService.CancelVanity()
	}
	progress := s.torService.GetVanityProgress()
	respondOK(w, http.StatusOK, map[string]any{
		"stopped":    wasRunning,
		"candidates": progress.Candidates,
	})
}

// handleTorInternalVanityApply handles POST /server/tor/vanity/apply (loopback-only).
func (s *Server) handleTorInternalVanityApply(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	var req torVanityApplyRequest
	if !decodeTorJSON(w, r, &req) {
		return
	}
	if !req.Confirm {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", errTorConfirmationRequired)
		return
	}
	addr, err := s.torService.ApplyVanityCandidate(req.Address)
	if err != nil {
		if errors.Is(err, service.ErrVanityCandidateNotFound) {
			torError(w, http.StatusNotFound, "NOT_FOUND", err)
			return
		}
		var ambiguous *service.AmbiguousVanityCandidateError
		if errors.As(err, &ambiguous) {
			respondJSON(w, http.StatusConflict, map[string]any{
				"ok": false, "error": "CONFLICT", "message": err.Error(), "details": map[string]any{"candidates": ambiguous.Candidates},
			})
			return
		}
		torError(w, http.StatusInternalServerError, "SERVER_ERROR", err)
		return
	}
	respondOK(w, http.StatusOK, map[string]any{"onion_address": addr})
}

// handleTorInternalImportKeys handles POST /server/tor/import-keys (loopback-only).
// The CLI sends a path only; private key bytes stay on disk and never cross
// the loopback control channel.
func (s *Server) handleTorInternalImportKeys(w http.ResponseWriter, r *http.Request) {
	if s.torService == nil {
		torUnavailable(w)
		return
	}
	var req struct {
		Path    string `json:"path"`
		Confirm bool   `json:"confirm"`
	}
	if !decodeTorJSON(w, r, &req) {
		return
	}
	if !req.Confirm {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", errTorConfirmationRequired)
		return
	}
	keyData, err := readTorImportKey(req.Path)
	if err != nil {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", err)
		return
	}
	if err := service.ValidateTorKeyMaterial(keyData); err != nil {
		torError(w, http.StatusBadRequest, "VALIDATION_FAILED", err)
		return
	}
	addr, err := s.torService.ImportKeys(keyData)
	if err != nil {
		torError(w, http.StatusInternalServerError, "SERVER_ERROR", err)
		return
	}
	respondOK(w, http.StatusOK, map[string]any{"onion_address": addr})
}

func readTorImportKey(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("key directory path must be absolute")
	}
	keyDir := filepath.Clean(path)
	dirInfo, err := os.Stat(keyDir)
	if err != nil {
		return nil, errors.New("key directory is not readable")
	}
	if !dirInfo.IsDir() {
		return nil, errors.New("key path must name a directory")
	}
	keyPath := filepath.Join(keyDir, "hs_ed25519_secret_key")
	keyInfo, err := os.Stat(keyPath)
	if err != nil || !keyInfo.Mode().IsRegular() || keyInfo.Size() <= 0 || keyInfo.Size() > 1<<20 {
		return nil, errors.New("key directory does not contain a readable Tor secret key")
	}
	keyData, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, errors.New("key directory does not contain a readable Tor secret key")
	}
	return keyData, nil
}
