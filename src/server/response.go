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

// respondError writes a JSON error response using the spec-required format.
// Per AI.md PART 9: {"ok":false,"error":"ERROR_CODE","message":"..."}
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]interface{}{
		"ok":      false,
		"error":   http.StatusText(status),
		"message": message,
	})
}
