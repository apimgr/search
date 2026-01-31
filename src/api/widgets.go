package api

import (
	"net/http"
	"strings"

	"github.com/apimgr/search/src/widget"
)

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

	// Extract widget type from path: /api/v1/widgets/{type}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/widgets/")
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

	// Fetch widget data
	data, err := h.widgetManager.FetchWidgetData(r.Context(), widgetType, params)
	if err != nil {
		h.errorResponse(w, http.StatusInternalServerError, "Failed to fetch widget data", err.Error())
		return
	}

	h.jsonResponse(w, http.StatusOK, &APIResponse{
		OK: true,
		Data:    data,
		Meta:    &APIMeta{Version: APIVersion},
	})
}
