package server

import (
	"encoding/json"
	"net/http"
)

// respondJSON writes a JSON response with the given status code.
// Per AI.md PART 6: used by debug endpoint handlers.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// mapHTTPStatusToCode returns the canonical machine-readable error code for an HTTP status.
// Per AI.md PART 9: error field must be a machine-readable code, not http.StatusText.
func mapHTTPStatusToCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "BAD_REQUEST"
	case http.StatusUnauthorized:
		return "UNAUTHORIZED"
	case http.StatusForbidden:
		return "FORBIDDEN"
	case http.StatusNotFound:
		return "NOT_FOUND"
	case http.StatusTooManyRequests:
		return "RATE_LIMITED"
	case http.StatusServiceUnavailable:
		return "MAINTENANCE"
	case http.StatusInternalServerError:
		return "SERVER_ERROR"
	default:
		return "SERVER_ERROR"
	}
}

// respondError writes a JSON error response using the spec-required format.
// Per AI.md PART 9: {"ok":false,"error":"ERROR_CODE","message":"...","details":{}}
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]interface{}{
		"ok":      false,
		"error":   mapHTTPStatusToCode(status),
		"message": message,
		"details": map[string]interface{}{},
	})
}
