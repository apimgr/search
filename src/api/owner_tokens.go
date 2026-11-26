package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/owner"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func (h *Handler) ownerIdentity(w http.ResponseWriter, r *http.Request) (owner.Identity, bool) {
	if h.ownerManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "OWNER_TOKENS_UNAVAILABLE", "owner token management is unavailable", r)
		return owner.Identity{}, false
	}
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(value, "Bearer ") {
		h.errorResponse(w, http.StatusUnauthorized, "UNAUTHORIZED", "primary owner token required", r)
		return owner.Identity{}, false
	}
	identity, err := h.ownerManager.Resolve(r.Context(), strings.TrimSpace(strings.TrimPrefix(value, "Bearer ")))
	if err != nil {
		h.errorResponse(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid owner token", r)
		return owner.Identity{}, false
	}
	if identity.Kind != "primary" {
		h.errorResponse(w, http.StatusForbidden, "FORBIDDEN", "primary owner token required", r)
		return owner.Identity{}, false
	}
	return identity, true
}

func (h *Handler) ownerPrimaryIssue(w http.ResponseWriter, r *http.Request) {
	if h.ownerManager == nil {
		h.errorResponse(w, http.StatusServiceUnavailable, "OWNER_TOKENS_UNAVAILABLE", "owner token management is unavailable", r)
		return
	}
	if bearer := bearerToken(r); bearer != "" {
		if _, err := h.ownerManager.Resolve(r.Context(), bearer); err == nil {
			h.errorResponse(w, http.StatusConflict, "CONFLICT", "a valid owner token already exists for this caller", r)
			return
		} else if !errors.Is(err, owner.ErrInvalidToken) {
			h.ownerTokenError(w, r, err)
			return
		}
	}
	_, token, err := h.ownerManager.IssueWithSource(r.Context(), "issue")
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusCreated, &APIResponse{OK: true, Data: map[string]string{"owner_token": token}})
}

func (h *Handler) ownerPrimaryRevoke(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	var input struct {
		Confirm string `json:"confirm"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", r)
		return
	}
	counts, err := h.ownerManager.RevokeOwner(r.Context(), identity, input.Confirm, "api")
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]int64{"tokens": counts.Tokens, "resources": counts.Resources}})
}

func (h *Handler) ownerTokensList(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	tokens, err := h.ownerManager.List(r.Context(), identity)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	var primary interface{}
	linked := make([]interface{}, 0, len(tokens))
	for _, token := range tokens {
		object, err := h.ownerTokenObject(identity, token)
		if err != nil {
			h.ownerTokenError(w, r, err)
			return
		}
		if token.Kind == "primary" {
			primary = object
		} else {
			linked = append(linked, object)
		}
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]interface{}{"primary": primary, "linked": linked, "max_linked": h.ownerManager.MaxLinked()}})
}

func (h *Handler) ownerPrimaryReveal(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	token, err := h.ownerManager.Reveal(r.Context(), identity, identity.TokenID)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"owner_token": token}})
}

func (h *Handler) ownerPrimaryRotate(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	token, err := h.ownerManager.Rotate(r.Context(), identity, identity.TokenID)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"owner_token": token}})
}

func (h *Handler) ownerLinkedCreate(w http.ResponseWriter, r *http.Request) {
	identity, ok := h.ownerIdentity(w, r)
	if !ok {
		return
	}
	var input struct {
		Name      string  `json:"name"`
		Expires   *string `json:"expires"`
		ExpiresOn *string `json:"expires_on"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", r)
		return
	}
	expiry, err := parseOwnerExpiryFields(input.Expires, input.ExpiresOn)
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), r)
		return
	}
	id, token, err := h.ownerManager.CreateLinked(r.Context(), identity, input.Name, expiry)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	created, err := h.ownerTokenByID(r, identity, id)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	object, err := h.ownerTokenObject(identity, created)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	data := object
	data["linked_token"] = token
	h.jsonResponse(w, http.StatusCreated, &APIResponse{OK: true, Data: data})
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
		h.ownerTokenError(w, r, err)
		return
	}
	item, err := h.ownerTokenByID(r, identity, id)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	data, err := h.ownerTokenObject(identity, item)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	data["linked_token"] = token
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: data})
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
		h.ownerTokenError(w, r, err)
		return
	}
	item, err := h.ownerTokenByID(r, identity, id)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	data, err := h.ownerTokenObject(identity, item)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	data["linked_token"] = token
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: data})
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
		Name      *string `json:"name"`
		Expires   *string `json:"expires"`
		ExpiresOn *string `json:"expires_on"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "invalid JSON body", r)
		return
	}
	tokens, err := h.ownerManager.List(r.Context(), identity)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	var current *owner.Token
	for i := range tokens {
		if tokens[i].ID == id {
			current = &tokens[i]
			break
		}
	}
	if current == nil || current.Kind != "linked" || current.RevokedAt != nil {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", "token not found", r)
		return
	}
	name := current.Name
	if input.Name != nil {
		name = *input.Name
	}
	expiry := current.ExpiresAt
	if input.Expires != nil || input.ExpiresOn != nil {
		expiry, err = parseOwnerExpiryFields(input.Expires, input.ExpiresOn)
	}
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "VALIDATION_FAILED", err.Error(), r)
		return
	}
	if err := h.ownerManager.UpdateLinked(r.Context(), identity, id, name, expiry); err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	updated, err := h.ownerTokenByID(r, identity, id)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	data, err := h.ownerTokenObject(identity, updated)
	if err != nil {
		h.ownerTokenError(w, r, err)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: data})
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
		h.ownerTokenError(w, r, err)
		return
	}
	h.jsonResponse(w, http.StatusOK, &APIResponse{OK: true, Data: map[string]string{"id": id, "status": "revoked"}})
}

func (h *Handler) ownerTokenByID(r *http.Request, identity owner.Identity, id string) (owner.Token, error) {
	tokens, err := h.ownerManager.List(r.Context(), identity)
	if err != nil {
		return owner.Token{}, err
	}
	for _, token := range tokens {
		if token.ID == id {
			return token, nil
		}
	}
	return owner.Token{}, owner.ErrNotFound
}

func (h *Handler) ownerTokenObject(identity owner.Identity, token owner.Token) (map[string]interface{}, error) {
	if len(token.Last4) != 4 || !strings.HasPrefix(token.Prefix, "own_") {
		return nil, errors.New("owner token display metadata is unavailable")
	}
	masked := "own_••••••••••••••••••••••••••••" + token.Last4
	status := "active"
	if token.RevokedAt != nil {
		status = "revoked"
	} else if token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now().UTC()) {
		status = "expired"
	}
	return map[string]interface{}{
		"id": token.ID, "name": token.Name, "kind": token.Kind,
		"token_masked": masked, "created_at": token.CreatedAt.UTC().Format(time.RFC3339),
		"last_used_at": formatOwnerTime(token.LastUsedAt), "expires_at": formatOwnerTime(token.ExpiresAt),
		"status": status,
	}, nil
}

func formatOwnerTime(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func (h *Handler) ownerTokenID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	parsed, err := uuid.Parse(id)
	if err != nil || parsed.Version() != 7 {
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", "token not found", r)
		return "", false
	}
	return parsed.String(), true
}

func parseOwnerExpiryFields(expires, expiresOn *string) (*time.Time, error) {
	if expires != nil && expiresOn != nil {
		return nil, errors.New("send either expires or expires_on, not both")
	}
	if expiresOn != nil {
		value := strings.TrimSpace(*expiresOn)
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil || parsed.Format("2006-01-02") != value {
			return nil, errors.New("expires_on must be a YYYY-MM-DD date")
		}
		today := time.Now().UTC().Format("2006-01-02")
		if value < today {
			return nil, errors.New("expires_on must be today or later in UTC")
		}
		expiry := parsed.UTC().AddDate(0, 0, 1)
		return &expiry, nil
	}
	if expires == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*expires)
	var duration time.Duration
	switch value {
	case "never":
		return nil, nil
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	case "90d":
		duration = 90 * 24 * time.Hour
	case "1y":
		duration = 365 * 24 * time.Hour
	default:
		return nil, errors.New("expires must be never, 7d, 30d, 90d, or 1y; use expires_on for a date")
	}
	expiry := time.Now().UTC().Add(duration)
	return &expiry, nil
}

func (h *Handler) ownerTokenError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, owner.ErrForbidden):
		h.errorResponse(w, http.StatusForbidden, "FORBIDDEN", "primary owner token required", r)
	case errors.Is(err, owner.ErrNotFound), errors.Is(err, owner.ErrInvalidToken):
		h.errorResponse(w, http.StatusNotFound, "NOT_FOUND", "token not found", r)
	case errors.Is(err, owner.ErrTokenLimit):
		h.errorResponse(w, http.StatusConflict, "TOKEN_LIMIT", err.Error(), r)
	case errors.Is(err, owner.ErrDuplicate):
		h.errorResponse(w, http.StatusConflict, "DUPLICATE", err.Error(), r)
	case errors.Is(err, owner.ErrConfirmation):
		h.errorResponse(w, http.StatusBadRequest, "VALIDATION_FAILED", "primary token confirmation did not match", r)
	default:
		h.errorResponse(w, http.StatusInternalServerError, "INTERNAL_ERROR", "owner token operation failed", r)
	}
}
