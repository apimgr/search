package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
)

func localizedHTTPError(w http.ResponseWriter, r *http.Request, status int, key string, args ...interface{}) {
	errorCode := strings.ReplaceAll(strings.ToUpper(http.StatusText(status)), " ", "_")
	body := map[string]interface{}{
		"ok":      false,
		"error":   errorCode,
		"message": i18n.RequestString(r, key, args...),
		"details": map[string]interface{}{},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
