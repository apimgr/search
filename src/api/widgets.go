package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/widget"
)

// widgetFetchTimeout bounds how long a single widget data fetch may run.
// The weather fetcher can make up to two sequential upstream HTTP calls
// (geocode, then forecast), each with its own 10s client timeout, so an
// unbounded worst case (~20s) can exceed the HTTP server's 15s
// WriteTimeout (src/server/server.go) and cause the server to hard-reset
// the connection mid-response instead of returning a clean error. Capping
// the whole fetch at 10s here leaves headroom under WriteTimeout for the
// response to actually be written.
const widgetFetchTimeout = 10 * time.Second

// handleWidgets returns list of available widgets
// Widgets are always enabled - users control via localStorage
func (h *Handler) handleWidgets(w http.ResponseWriter, r *http.Request) {
	if h.widgetManager == nil {
		// Return basic widgets even without manager
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: map[string]interface{}{
				"enabled":  true,
				"widgets":  []interface{}{},
				"defaults": []string{"clock", "calculator", "quicklinks", "notes"},
			},
			Meta: &APIMeta{Version: APIVersion},
		})
		return
	}

	allWidgets := h.widgetManager.GetAllWidgets()
	defaults := h.widgetManager.GetDefaultWidgets()

	// Filter by category if requested
	category := r.URL.Query().Get("category")
	if category != "" {
		var filtered []*widget.Widget
		for _, w := range allWidgets {
			if string(w.Category) == category {
				filtered = append(filtered, w)
			}
		}
		allWidgets = filtered
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"enabled":  h.widgetManager.IsEnabled(),
			"widgets":  allWidgets,
			"defaults": defaults,
		},
		Meta: &APIMeta{Version: APIVersion},
	})
}

// handleWidgetData fetches data for a specific widget
func (h *Handler) handleWidgetData(w http.ResponseWriter, r *http.Request) {
	if h.widgetManager == nil {
		// Return empty data - tool widgets work client-side, data widgets need manager
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			OK: true,
			Data: &widget.WidgetData{
				Error: "Widget data not available - configure in widget settings",
			},
			Meta: &APIMeta{Version: APIVersion},
		})
		return
	}

	// Extract widget type from path: /api/{api_version}/widgets/{type}
	path := strings.TrimPrefix(r.URL.Path, APIPrefix+"/widgets/")
	widgetType := widget.WidgetType(strings.Split(path, "/")[0])

	if widgetType == "" {
		h.errorResponse(w, http.StatusBadRequest, "Widget type required", "")
		return
	}

	// Check if widget is enabled
	if !h.widgetManager.IsWidgetEnabled(widgetType) {
		h.errorResponse(w, http.StatusNotFound, "Widget not available", "")
		return
	}

	// Collect params from query string
	params := make(map[string]string)
	for key, values := range r.URL.Query() {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}

	// Fall back to the global "units" cookie (set from the Preferences page)
	// when the widget itself has no explicit per-widget override.
	if params["units"] == "" {
		if cookie, err := r.Cookie("units"); err == nil {
			if cookie.Value == "metric" || cookie.Value == "imperial" {
				params["units"] = cookie.Value
			}
		}
	}

	// Fetch widget data, bounded well under the server's WriteTimeout so a
	// slow upstream (e.g. weather geocode+forecast) fails cleanly with a
	// JSON error instead of the server hard-resetting the connection.
	ctx, cancel := context.WithTimeout(r.Context(), widgetFetchTimeout)
	defer cancel()
	data, err := h.widgetManager.FetchWidgetData(ctx, widgetType, params)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to fetch widget data", err.Error())
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK:   true,
		Data: data,
		Meta: &APIMeta{Version: APIVersion},
	})
}
