package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/notify"
)

type contactWebhookSaveRequest struct {
	Role      string `json:"role"`
	Transport string `json:"transport"`
	URL       string `json:"url"`
}

func (h *Handler) handleContactWebhookConfigSave(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input contactWebhookSaveRequest
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		h.writeError(w, r, "BAD_REQUEST", "Invalid JSON body", http.StatusBadRequest)
		return
	}
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	input.Transport = strings.ToLower(strings.TrimSpace(input.Transport))
	input.URL = strings.TrimSpace(input.URL)
	if !validContactRole(input.Role) || !validWebhookTransport(input.Transport) {
		h.writeError(w, r, "VALIDATION_FAILED", "Role or webhook transport is invalid", http.StatusBadRequest)
		return
	}
	if input.URL != "" {
		parsed, err := url.ParseRequestURI(input.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
			h.writeError(w, r, "VALIDATION_FAILED", "Webhook URL must be a credential-free HTTPS URL", http.StatusBadRequest)
			return
		}
	}
	contact := h.config.Get().Contact
	roleConfig := contactRole(&contact, input.Role)
	previousURL := roleConfig.Webhooks[input.Transport]
	previousSecret := roleConfig.Webhooks[input.Transport+"_secret"]
	secret := previousSecret
	if input.URL == "" {
		secret = ""
	} else {
		if previousURL != input.URL || secret == "" {
			var err error
			secret, err = notify.GenerateWebhookSecret()
			if err != nil {
				h.writeError(w, r, "INTERNAL_ERROR", "Webhook secret generation failed", http.StatusInternalServerError)
				return
			}
		}
	}
	h.config.Update(func(server *config.ServerConfig) {
		role := contactRole(&server.Contact, input.Role)
		if role.Webhooks == nil {
			role.Webhooks = make(config.WebhookNotifyConfig)
		}
		if input.URL == "" {
			delete(role.Webhooks, input.Transport)
			delete(role.Webhooks, input.Transport+"_secret")
			return
		}
		role.Webhooks[input.Transport] = input.URL
		role.Webhooks[input.Transport+"_secret"] = secret
	})
	if err := h.saveConfig(); err != nil {
		slog.Error("failed to persist contact webhook configuration", "err", err)
		h.config.Update(func(server *config.ServerConfig) {
			role := contactRole(&server.Contact, input.Role)
			if role.Webhooks == nil {
				role.Webhooks = make(config.WebhookNotifyConfig)
			}
			if previousURL == "" {
				delete(role.Webhooks, input.Transport)
			} else {
				role.Webhooks[input.Transport] = previousURL
			}
			if previousSecret == "" {
				delete(role.Webhooks, input.Transport+"_secret")
			} else {
				role.Webhooks[input.Transport+"_secret"] = previousSecret
			}
		})
		h.writeError(w, r, "INTERNAL_ERROR", "Webhook configuration persistence failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	response := map[string]interface{}{"role": input.Role, "transport": input.Transport, "configured": input.URL != ""}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: response})
}

func validContactRole(role string) bool {
	switch role {
	case "admin", "security", "abuse", "general":
		return true
	default:
		return false
	}
}

func validWebhookTransport(transport string) bool {
	switch transport {
	case "telegram", "discord", "slack", "mattermost", "pushover", "gotify", "generic":
		return true
	default:
		return false
	}
}

func contactRole(contact *config.ContactConfig, role string) *config.ContactRoleConfig {
	switch role {
	case "admin":
		return &contact.Admin
	case "security":
		return &contact.Security
	case "abuse":
		return &contact.Abuse
	default:
		return &contact.General
	}
}
