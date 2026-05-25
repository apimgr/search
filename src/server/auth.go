// Package server: bearer-token authentication for the operator API.
//
// Per AI.md: there is NO admin web UI, NO sessions, NO user accounts.
// Two-tier auth:
//
//  1. Operator/server token — configured via server.token in server.yml
//     (auto-generated on first run). Sent as: Authorization: Bearer <token>
//     Compared via SHA-256 + subtle.ConstantTimeCompare.
//
//  2. Per-resource owner tokens — stored as SHA-256 in the api_tokens table.
//     (Handled at the resource layer, not here.)
//
// All API mutations that require operator privilege must call ValidateOperatorToken.
package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/config"
)

// ValidateOperatorToken returns true when the request carries a valid operator
// token in the Authorization header. Comparison is constant-time over the
// SHA-256 digests so that timing leaks do not reveal token bytes.
//
// Returns false (without error) if:
//   - server.token is empty in the configuration (no operator configured)
//   - the Authorization header is missing or malformed
//   - the token does not match
func ValidateOperatorToken(r *http.Request, cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	expected := cfg.Get().Token
	if expected == "" {
		return false
	}

	presented, ok := extractBearerToken(r)
	if !ok {
		return false
	}

	expectedSum := sha256.Sum256([]byte(expected))
	presentedSum := sha256.Sum256([]byte(presented))
	return subtle.ConstantTimeCompare(expectedSum[:], presentedSum[:]) == 1
}

// extractBearerToken pulls the token value from an Authorization header of the
// form `Bearer <token>`. Whitespace around the token is trimmed.
func extractBearerToken(r *http.Request) (string, bool) {
	hdr := r.Header.Get("Authorization")
	if hdr == "" {
		return "", false
	}
	const prefix = "Bearer "
	if len(hdr) <= len(prefix) || !strings.EqualFold(hdr[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(hdr[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// RequireOperator wraps an http.HandlerFunc and rejects requests that do not
// present a valid operator token.
func (s *Server) RequireOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ValidateOperatorToken(r, s.config) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="operator"`)
			localizedHTTPError(w, r, http.StatusUnauthorized, "errors.unauthorized")
			return
		}
		next(w, r)
	}
}

// getClientIPSimple extracts the client IP address from a request.
// Used by src/server/alerts.go and others.
func getClientIPSimple(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	ip := r.RemoteAddr
	if colonIdx := strings.LastIndex(ip, ":"); colonIdx != -1 {
		ip = ip[:colonIdx]
	}
	return ip
}
