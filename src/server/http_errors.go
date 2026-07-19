package server

import (
	"encoding/json"
	"net/http"

	"github.com/apimgr/search/src/common/i18n"
)

// localizedHTTPError writes a canonical JSON error response with the given HTTP status code.
// Per AI.md PART 9: error field is a machine-readable code; message is the localized string.
// Body format: {ok: false, error: <ERROR_CODE>, message: <localized string>, details: {}}
func localizedHTTPError(w http.ResponseWriter, r *http.Request, status int, key string, args ...interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := struct {
		OK      bool              `json:"ok"`
		Error   string            `json:"error"`
		Message string            `json:"message"`
		Details map[string]string `json:"details"`
	}{
		OK:      false,
		Error:   mapHTTPStatusToCode(status),
		Message: i18n.RequestString(r, key, args...),
		Details: map[string]string{},
	}
	_ = json.NewEncoder(w).Encode(body)
}
