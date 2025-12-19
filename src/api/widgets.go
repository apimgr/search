package api

import (
	"net/http"
	"strings"

	"github.com/apimgr/search/src/widgets"
)

// handleWidgets returns list of available widgets
func (h *Handler) handleWidgets(w http.ResponseWriter, r *http.Request) {
	if h.widgetManager == nil {
		h.jsonResponse(w, http.StatusOK, &APIResponse{
			Success: true,
			Data: map[string]interface{}{
				"enabled":  false,
				"widgets":  []interface{}{},
				"defaults": []string{},
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
		var filtered []*widgets.Widget
		for _, w := range allWidgets {
			if string(w.Category) == category {
				filtered = append(filtered, w)
			}
		}
		allWidgets = filtered
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
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
		h.errorResponse(w, http.StatusServiceUnavailable, "Widgets not enabled", "")
		return
	}

	// Extract widget type from path: /api/v1/widgets/{type}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/widgets/")
	widgetType := widgets.WidgetType(strings.Split(path, "/")[0])

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

	// Fetch widget data
	data, err := h.widgetManager.FetchWidgetData(r.Context(), widgetType, params)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to fetch widget data", err.Error())
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		Success: true,
		Data:    data,
		Meta:    &APIMeta{Version: APIVersion},
	})
}
