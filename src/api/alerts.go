package api

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/alert"
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

func (h *Handler) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if h.alertManager == nil {
		h.writeError(w, "NOT_AVAILABLE", "Alert storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost {
		h.writeError(w, "METHOD_NOT_ALLOWED", "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req alertCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, "BAD_REQUEST", "Invalid JSON body", http.StatusBadRequest)
		return
	}

	created, err := h.alertManager.Create(r.Context(), alert.CreateRequest{
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
	})
	if err != nil {
		h.writeError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest)
		return
	}

	h.writeJSON(w, http.StatusCreated, APIResponse{
		OK: true,
		Data: map[string]interface{}{
			"alert":        created.Alert,
			"manage_url":   baseURLFromRequest(h, r) + "/alerts/manage/" + created.ManageToken,
			"rss_url":      baseURLFromRequest(h, r) + "/alerts/" + created.RSSToken + ".rss",
			"manage_token": created.ManageToken,
			"rss_token":    created.RSSToken,
		},
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
			h.writeError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.alertManager.SetPaused(r.Context(), token, paused); err != nil {
			h.writeError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest)
			return
		}
		h.writeJSON(w, http.StatusOK, APIResponse{OK: true, Data: map[string]bool{"paused": paused}})
	case strings.HasSuffix(path, "/rss") && r.Method == http.MethodGet:
		token := strings.TrimSuffix(path, "/rss")
		xmlData, err := h.alertManager.FeedXML(r.Context(), token, 50)
		if err != nil {
			h.writeError(w, "NOT_FOUND", err.Error(), http.StatusNotFound)
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
				h.writeError(w, "NOT_FOUND", err.Error(), http.StatusNotFound)
				return
			}
			data, err := alertManageResponseData(h, r, token, alertInfo)
			if err != nil {
				h.writeError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest)
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
				h.writeError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest)
				return
			}
			data, err := alertManageResponseData(h, r, token, alertInfo)
			if err != nil {
				h.writeError(w, "BAD_REQUEST", err.Error(), http.StatusBadRequest)
				return
			}
			h.writeJSON(w, http.StatusOK, APIResponse{OK: true, Data: data})
		case http.MethodDelete:
			if err := h.alertManager.Delete(r.Context(), token); err != nil {
				h.writeError(w, "NOT_FOUND", err.Error(), http.StatusNotFound)
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

func baseURLFromRequest(h *Handler, r *http.Request) string {
	if h.config.Server.BaseURL != "" {
		return strings.TrimRight(h.config.Server.BaseURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func clientIPForAPI(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

func decodeAlertPauseState(r *http.Request) (bool, error) {
	paused := true
	if r.Body != nil {
		defer r.Body.Close()
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
