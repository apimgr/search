package api

import (
	"net/http"

	"github.com/apimgr/search/src/i18n"
)

func localizedHTTPError(w http.ResponseWriter, r *http.Request, status int, key string, args ...interface{}) {
	http.Error(w, i18n.RequestString(r, key, args...), status)
}
