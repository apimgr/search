package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/alert"
	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/config"
)

type alertCreateRequest struct {
	Query          string   `json:"query"`
	Category       string   `json:"category"`
	Language       string   `json:"language"`
	Region         string   `json:"region"`
	Engines        []string `json:"engines"`
	SafeSearch     int      `json:"safe_search"`
	Frequency      string   `json:"frequency"`
	Email          string   `json:"email"`
	DeliverEmail   bool     `json:"deliver_email"`
	DeliverRSS     bool     `json:"deliver_rss"`
	DeliverWebhook bool     `json:"deliver_webhook"`
	WebhookURL     string   `json:"webhook_url"`
}

type alertUpdateRequest struct {
	Query          string   `json:"query"`
	Category       string   `json:"category"`
	Language       string   `json:"language"`
	Region         string   `json:"region"`
	Engines        []string `json:"engines"`
	Frequency      string   `json:"frequency"`
	SafeSearch     int      `json:"safe_search"`
	DeliverEmail   bool     `json:"deliver_email"`
	DeliverRSS     bool     `json:"deliver_rss"`
	DeliverWebhook bool     `json:"deliver_webhook"`
	WebhookURL     string   `json:"webhook_url"`
}

type alertPauseRequest struct {
	Paused *bool `json:"paused"`
}

var errAlertStorageUnavailable = errors.New("alert storage is unavailable")

// AlertCreateForGraphQL creates an alert with the same request metadata and response fields as REST.
func (h *Handler) AlertCreateForGraphQL(ctx context.Context, r *http.Request, req alert.CreateRequest) (map[string]interface{}, error) {
	if h.alertManager == nil {
		return nil, errAlertStorageUnavailable
	}
	if h.ownerManager == nil {
		return nil, errors.New("owner token service is unavailable")
	}
	req.BaseURL = baseURLFromRequest(h, r)
	req.CreatedFromIP = clientIPForAPI(r)
	if err := h.alertManager.ValidateCreate(req); err != nil {
		return nil, err
	}
	ownerToken := ""
	if header := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(header, "Bearer ") {
		ownerToken = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	identity, issuedToken, err := h.ownerManager.IssueOrResolve(ctx, ownerToken)
	if err != nil {
		return nil, err
	}
	req.OwnerID = identity.OwnerID
	created, err := h.alertManager.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	baseURL := baseURLFromRequest(h, r)
	data := map[string]interface{}{"alert": created.Alert, "manage_url": baseURL + "/alerts/manage/" + created.ManageToken, "rss_url": baseURL + "/alerts/" + created.RSSToken + ".rss", "manage_token": created.ManageToken, "rss_token": created.RSSToken}
	if issuedToken != "" {
		data["owner_token"] = issuedToken
	}
	return data, nil
}

// AlertListForGraphQL returns the operator-only alert inventory.
func (h *Handler) AlertListForGraphQL(ctx context.Context, r *http.Request) ([]*alert.Alert, error) {
	if err := h.operatorAuthError(r); err != nil {
		return nil, err
	}
	if h.alertManager == nil {
		return nil, errAlertStorageUnavailable
	}
	return h.alertManager.ListAll(ctx)
}

// AlertGetForGraphQL loads an alert through its private manage token.
func (h *Handler) AlertGetForGraphQL(ctx context.Context, r *http.Request, token string) (map[string]interface{}, error) {
	if h.alertManager == nil {
		return nil, errAlertStorageUnavailable
	}
	item, err := h.alertManager.GetByManageToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return alertManageResponseData(h, r, token, item)
}

// AlertUpdateForGraphQL updates an alert through its private manage token.
func (h *Handler) AlertUpdateForGraphQL(ctx context.Context, r *http.Request, token string, req alert.UpdateRequest) (map[string]interface{}, error) {
	if h.alertManager == nil {
		return nil, errAlertStorageUnavailable
	}
	item, err := h.alertManager.Update(ctx, token, req)
	if err != nil {
		return nil, err
	}
	return alertManageResponseData(h, r, token, item)
}

// AlertPauseForGraphQL changes an alert's paused state through its manage token.
func (h *Handler) AlertPauseForGraphQL(ctx context.Context, token string, paused bool) error {
	if h.alertManager == nil {
		return errAlertStorageUnavailable
	}
	return h.alertManager.SetPaused(ctx, token, paused)
}

// AlertDeleteForGraphQL deletes an alert through its private manage token.
func (h *Handler) AlertDeleteForGraphQL(ctx context.Context, token string) error {
	if h.alertManager == nil {
		return errAlertStorageUnavailable
	}
	return h.alertManager.Delete(ctx, token)
}

// AlertFeedForGraphQL returns a bounded private RSS feed through its token.
func (h *Handler) AlertFeedForGraphQL(ctx context.Context, token string) (*alert.Feed, error) {
	if h.alertManager == nil {
		return nil, errAlertStorageUnavailable
	}
	return h.alertManager.Feed(ctx, token, 50)
}

func (h *Handler) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		h.writeError(w, "NOT_AVAILABLE", "Alert storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		// Operator-only: list all alerts per AI.md PART 14.
		h.requireOperator(h.handleListAlerts)(w, r)
	case http.MethodPost:
		h.handleCreateAlert(w, r)
	default:
		h.writeError(w, "METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListAlerts handles GET /api/v1/alerts (operator token required).
func (h *Handler) handleListAlerts(w http.ResponseWriter, r *http.Request) {
	alerts, err := h.alertManager.ListAll(r.Context())
	if err != nil {
		h.writeError(w, "INTERNAL_ERROR", "Failed to list alerts", http.StatusInternalServerError)
		return
	}
	h.writeJSON(w, http.StatusOK, APIResponse{
		OK:   true,
		Data: map[string]interface{}{"alerts": alerts},
	})
}

// handleCreateAlert handles POST /api/v1/alerts (anonymous — rate limited by middleware).
func (h *Handler) handleCreateAlert(w http.ResponseWriter, r *http.Request) {
	var req alertCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "BAD_REQUEST", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	if h.ownerManager == nil {
		h.writeError(w, "OWNER_TOKENS_UNAVAILABLE", "Owner token service is unavailable", http.StatusServiceUnavailable)
		return
	}
	createRequest := alert.CreateRequest{
		Query:          req.Query,
		Category:       req.Category,
		Language:       req.Language,
		Region:         req.Region,
		Engines:        req.Engines,
		SafeSearch:     req.SafeSearch,
		Frequency:      alert.Frequency(req.Frequency),
		Email:          req.Email,
		DeliverEmail:   req.DeliverEmail,
		DeliverRSS:     req.DeliverRSS,
		DeliverWebhook: req.DeliverWebhook,
		WebhookURL:     req.WebhookURL,
		BaseURL:        baseURLFromRequest(h, r),
		CreatedFromIP:  clientIPForAPI(r),
	}
	if err := h.alertManager.ValidateCreate(createRequest); err != nil {
		h.writeAlertError(w, err)
		return
	}
	ownerToken := ""
	if header := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(header, "Bearer ") {
		ownerToken = strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	identity, issuedToken, err := h.ownerManager.IssueOrResolve(r.Context(), ownerToken)
	if err != nil {
		h.writeError(w, "INTERNAL_ERROR", "Owner identity could not be created", http.StatusInternalServerError)
		return
	}
	createRequest.OwnerID = identity.OwnerID
	created, err := h.alertManager.Create(r.Context(), createRequest)
	if err != nil {
		h.writeAlertError(w, err)
		return
	}

	data := map[string]interface{}{
		"alert":        created.Alert,
		"manage_url":   baseURLFromRequest(h, r) + "/alerts/manage/" + created.ManageToken,
		"rss_url":      baseURLFromRequest(h, r) + "/alerts/" + created.RSSToken + ".rss",
		"manage_token": created.ManageToken,
		"rss_token":    created.RSSToken,
	}
	if issuedToken != "" {
		data["owner_token"] = issuedToken
	}
	h.writeJSON(w, http.StatusCreated, APIResponse{
		OK:   true,
		Data: data,
	})
}

func (h *Handler) handleAlertByToken(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		h.writeError(w, "NOT_AVAILABLE", "Alert storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, APIPrefix+"/alerts/")
	switch {
	case strings.HasSuffix(path, "/pause") && r.Method == http.MethodPost:
		token := strings.TrimSuffix(path, "/pause")
		paused, err := decodeAlertPauseState(r)
		if err != nil {
			h.writeError(w, "BAD_REQUEST", "Invalid request body", http.StatusBadRequest)
			return
		}
		if err := h.alertManager.SetPaused(r.Context(), token, paused); err != nil {
			h.writeAlertError(w, err)
			return
		}
		h.writeJSON(w, http.StatusOK, APIResponse{OK: true, Data: map[string]bool{"paused": paused}})
	case strings.HasSuffix(path, "/rss") && r.Method == http.MethodGet:
		token := strings.TrimSuffix(path, "/rss")
		xmlData, err := h.alertManager.FeedXML(r.Context(), token, 50)
		if err != nil {
			h.writeAlertError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(xmlData)
	default:
		token := path
		if idx := strings.IndexRune(token, '/'); idx >= 0 {
			token = token[:idx]
		}
		switch r.Method {
		case http.MethodGet:
			alertInfo, err := h.alertManager.GetByManageToken(r.Context(), token)
			if err != nil {
				h.writeAlertError(w, err)
				return
			}
			data, err := alertManageResponseData(h, r, token, alertInfo)
			if err != nil {
				h.writeAlertError(w, err)
				return
			}
			h.writeJSON(w, http.StatusOK, APIResponse{OK: true, Data: data})
		case http.MethodPatch:
			var req alertUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				h.writeError(w, "BAD_REQUEST", "Invalid JSON body", http.StatusBadRequest)
				return
			}
			alertInfo, err := h.alertManager.Update(r.Context(), token, alert.UpdateRequest{
				Query:          req.Query,
				Category:       req.Category,
				Language:       req.Language,
				Region:         req.Region,
				Engines:        req.Engines,
				Frequency:      alert.Frequency(req.Frequency),
				SafeSearch:     req.SafeSearch,
				DeliverEmail:   req.DeliverEmail,
				DeliverRSS:     req.DeliverRSS,
				DeliverWebhook: req.DeliverWebhook,
				WebhookURL:     req.WebhookURL,
			})
			if err != nil {
				h.writeAlertError(w, err)
				return
			}
			data, err := alertManageResponseData(h, r, token, alertInfo)
			if err != nil {
				h.writeAlertError(w, err)
				return
			}
			h.writeJSON(w, http.StatusOK, APIResponse{OK: true, Data: data})
		case http.MethodDelete:
			if err := h.alertManager.Delete(r.Context(), token); err != nil {
				h.writeAlertError(w, err)
				return
			}
			h.writeJSON(w, http.StatusOK, APIResponse{OK: true, Data: map[string]bool{"deleted": true}})
		default:
			h.writeError(w, "METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload APIResponse) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) writeError(w http.ResponseWriter, code, message string, status int) {
	h.writeJSON(w, status, APIResponse{
		OK:      false,
		Error:   code,
		Message: message,
	})
}

// writeAlertError maps an error from the alert package to a safe API error
// response. Sentinel errors (ErrNotFound, ErrInvalidInput, ErrEmailRequired)
// carry curated, user-safe messages and are returned as-is. Any other error
// (DB failures, etc.) is never exposed to the client — it is logged and the
// client receives a generic SERVER_ERROR, per AI.md PART 11: error messages
// must never leak Go error strings, stack traces, or internal paths.
func (h *Handler) writeAlertError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, alert.ErrNotFound):
		h.writeError(w, "NOT_FOUND", "Alert not found", http.StatusNotFound)
	case errors.Is(err, alert.ErrInvalidInput), errors.Is(err, alert.ErrEmailRequired):
		h.writeError(w, "VALIDATION_FAILED", err.Error(), http.StatusBadRequest)
	default:
		slog.Error("alert API error", "error", err)
		h.writeError(w, "SERVER_ERROR", "Internal server error", http.StatusInternalServerError)
	}
}

func baseURLFromRequest(h *Handler, r *http.Request) string {
	if h.config.Server.BaseURL != "" {
		return strings.TrimRight(h.config.Server.BaseURL, "/")
	}
	scheme := httputil.GetProtoFromRequest(r)
	host := httputil.GetHostFromRequest(r)
	return scheme + "://" + host
}

func clientIPForAPI(r *http.Request) string {
	// Delegates to httputil which gates header trust on the package-level proxy state.
	return httputil.GetClientIP(r)
}

func decodeAlertPauseState(r *http.Request) (bool, error) {
	paused := true
	if r.Body != nil {
		defer closeutil.Close(r.Body)
		var req alertPauseRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		switch {
		case err == nil && req.Paused != nil:
			return *req.Paused, nil
		case err == nil:
			return paused, nil
		case errors.Is(err, io.EOF):
		default:
			return false, err
		}
	}

	if raw := strings.TrimSpace(r.URL.Query().Get("paused")); raw != "" {
		return config.ParseBool(raw, true)
	}
	return paused, nil
}

func alertManageResponseData(h *Handler, r *http.Request, manageToken string, alertInfo *alert.Alert) (map[string]interface{}, error) {
	rssToken, err := h.alertManager.RSSTokenForManageToken(r.Context(), manageToken)
	if err != nil {
		return nil, err
	}
	baseURL := baseURLFromRequest(h, r)
	return map[string]interface{}{
		"alert":        alertInfo,
		"manage_token": manageToken,
		"manage_url":   baseURL + "/alerts/manage/" + manageToken,
		"rss_token":    rssToken,
		"rss_url":      baseURL + "/alerts/" + rssToken + ".rss",
	}, nil
}
