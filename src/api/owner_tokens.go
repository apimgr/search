package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/owner"
	"github.com/go-chi/chi/v5"
)

func (h *Handler) ownerIdentity(w http.ResponseWriter, r *http.Request) (owner.Identity, bool) {
	if h.ownerManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "OWNER_TOKENS_UNAVAILABLE", "owner token management is unavailable")
		return owner.Identity{}, false
	}
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") {
		h.errorResponse(w, http.StatusUnauthorized, "UNAUTHORIZED", "primary owner token required")
		return owner.Identity{}, false
	}
	identity, err := h.ownerManager.Resolve(r.Context(), strings.TrimSpace(strings.TrimPrefix(value, "Bearer ")))
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid owner token")
		return owner.Identity{}, false
	}
	if identity.Kind != "primary" {
		h.errorResponse(w, http.StatusForbidden, "FORBIDDEN", "primary owner token required")
		return owner.Identity{}, false
	}
	return identity, true
}

func (h *Handler) ownerTokensList(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	tokens, err := h.ownerManager.List(r.Context(), identity)
	if err != nil {
		h.ownerTokenError(w, err)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"tokens": tokens}})
}

func (h *Handler) ownerPrimaryReveal(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	token, err := h.ownerManager.Reveal(r.Context(), identity, identity.TokenID)
	if err != nil {
		h.ownerTokenError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"token": token}})
}

func (h *Handler) ownerPrimaryRotate(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	token, err := h.ownerManager.Rotate(r.Context(), identity, identity.TokenID)
	if err != nil {
		h.ownerTokenError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"token": token}})
}

func (h *Handler) ownerLinkedCreate(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	var input struct {
		Name      string `json:"name"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}
	expiry, err := parseOwnerExpiry(input.ExpiresAt)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	id, token, err := h.ownerManager.CreateLinked(r.Context(), identity, input.Name, expiry)
	if err != nil {
		h.ownerTokenError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusCreated, &APIResponse{OK: true, Data: map[string]interface{}{"id": id, "token": token}})
}

func (h *Handler) ownerLinkedReveal(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	id, valid := h.ownerTokenID(w, r)
	if !valid {
		return
	}
	token, err := h.ownerManager.Reveal(r.Context(), identity, id)
	if err != nil {
		h.ownerTokenError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"token": token}})
}

func (h *Handler) ownerLinkedRotate(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	id, valid := h.ownerTokenID(w, r)
	if !valid {
		return
	}
	token, err := h.ownerManager.Rotate(r.Context(), identity, id)
	if err != nil {
		h.ownerTokenError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"token": token}})
}

func (h *Handler) ownerLinkedUpdate(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	id, valid := h.ownerTokenID(w, r)
	if !valid {
		return
	}
	var input struct {
		Name      string `json:"name"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body")
		return
	}
	expiry, err := parseOwnerExpiry(input.ExpiresAt)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error())
		return
	}
	if err := h.ownerManager.UpdateLinked(r.Context(), identity, id, input.Name, expiry); err != nil {
		h.ownerTokenError(w, err)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]bool{"updated": true}})
}

func (h *Handler) ownerLinkedRevoke(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	id, valid := h.ownerTokenID(w, r)
	if !valid {
		return
	}
	if err := h.ownerManager.RevokeLinked(r.Context(), identity, id); err != nil {
		h.ownerTokenError(w, err)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]bool{"revoked": true}})
}

func (h *Handler) ownerTokenID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", "token not found")
		return 0, false
	}
	return id, true
}

func parseOwnerExpiry(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "never" {
		return nil, nil
	}
	var duration time.Duration
	switch value {
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	case "90d":
		duration = 90 * 24 * time.Hour
	case "1y":
		duration = 365 * 24 * time.Hour
	default:
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return nil, errors.New("expiry must be never, 7d, 30d, 90d, 1y, or YYYY-MM-DD")
		}
		expiry := parsed.UTC().Add(23*time.Hour + 59*time.Minute + 59*time.Second)
		if !expiry.After(time.Now()) {
			return nil, errors.New("expiry must be in the future")
		}
		return &expiry, nil
	}
	expiry := time.Now().UTC().Add(duration)
	return &expiry, nil
}

func (h *Handler) ownerTokenError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, owner.ErrForbidden):
		h.errorResponse(w, http.StatusForbidden, "FORBIDDEN", "primary owner token required")
	case errors.Is(err, owner.ErrNotFound), errors.Is(err, owner.ErrInvalidToken):
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", "token not found")
	case errors.Is(err, owner.ErrTokenLimit):
		h.errorResponse(w, http.StatusConflict, "TOKEN_LIMIT", err.Error())
	case errors.Is(err, owner.ErrDuplicate):
		h.errorResponse(w, http.StatusConflict, "DUPLICATE", err.Error())
	default:
		h.errorResponse(w, http.StatusInternalServerError, "INTERNAL_ERROR", "owner token operation failed")
	}
}
