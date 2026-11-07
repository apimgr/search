package server

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/owner"
	"github.com/go-chi/chi/v5"
)

const ownerTokenCookie = "search_owner_token_xJ5yCXLv"

type ownerTokenFlash struct {
	token     string
	tokenCard string
	message   string
	createdAt time.Time
}

const ownerTokenFlashLifetime = 2 * time.Minute

func (s *Server) ownerWebIdentity(w http.ResponseWriter, r *http.Request, requirePrimary bool) (owner.Identity, bool) {
	if s.ownerManager == nil {
		return owner.Identity{}, false
	}
	cookie, err := r.Cookie(ownerTokenCookie)
	if err != nil {
		return owner.Identity{}, false
	}
	identity, err := s.ownerManager.Resolve(r.Context(), cookie.Value)
	if err != nil {
		s.clearOwnerCookie(w)
		return owner.Identity{}, false
	}
	if requirePrimary && identity.Kind != "primary" {
		s.clearOwnerCookie(w)
		return owner.Identity{}, false
	}
	s.setOwnerCookie(w, cookie.Value)
	return identity, true
}

func (s *Server) ownerWebRequired(w http.ResponseWriter, r *http.Request) (owner.Identity, bool) {
	identity, ok := s.ownerWebIdentity(w, r, true)
	if !ok {
		http.Error(w, i18n.RequestString(r, "errors.forbidden"), http.StatusForbidden)
	}
	return identity, ok
}

func (s *Server) setOwnerCookie(w http.ResponseWriter, token string) {
	maxAge := 365 * 24 * time.Hour
	configured := strings.TrimSpace(s.config.Server.Tokens.CookieMaxAge)
	if configured == "1y" {
		configured = "8760h"
	}
	if parsed, err := time.ParseDuration(configured); err == nil && parsed > 0 {
		maxAge = parsed
	}
	http.SetCookie(w, &http.Cookie{Name: ownerTokenCookie, Value: token, Path: "/", MaxAge: int(maxAge.Seconds()), Expires: time.Now().Add(maxAge), HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func (s *Server) clearOwnerCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: ownerTokenCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}

func (s *Server) storeOwnerTokenFlash(flash ownerTokenFlash) (string, error) {
	var idBytes [24]byte
	if _, err := rand.Read(idBytes[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(idBytes[:])
	flash.createdAt = time.Now()
	s.ownerFlashMu.Lock()
	defer s.ownerFlashMu.Unlock()
	if s.ownerFlashes == nil {
		s.ownerFlashes = make(map[string]ownerTokenFlash)
	}
	for key, existing := range s.ownerFlashes {
		if time.Since(existing.createdAt) > ownerTokenFlashLifetime {
			delete(s.ownerFlashes, key)
		}
	}
	s.ownerFlashes[id] = flash
	return id, nil
}

func (s *Server) takeOwnerTokenFlash(id string) (ownerTokenFlash, bool) {
	if len(id) != 48 {
		return ownerTokenFlash{}, false
	}
	s.ownerFlashMu.Lock()
	defer s.ownerFlashMu.Unlock()
	flash, ok := s.ownerFlashes[id]
	delete(s.ownerFlashes, id)
	if !ok || time.Since(flash.createdAt) > ownerTokenFlashLifetime {
		return ownerTokenFlash{}, false
	}
	return flash, true
}

func (s *Server) redirectOwnerTokenAction(w http.ResponseWriter, r *http.Request, card string, token string, message string, tokenCard string) {
	id, err := s.storeOwnerTokenFlash(ownerTokenFlash{token: token, tokenCard: tokenCard, message: message})
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusInternalServerError)
		return
	}
	location := "/server/preferences/tokens?result=" + id + "#" + card
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, location, http.StatusSeeOther)
}

func (s *Server) handleOwnerTokensPage(w http.ResponseWriter, r *http.Request) {
	s.renderOwnerTokensPage(w, r)
}

func (s *Server) renderOwnerTokensPage(w http.ResponseWriter, r *http.Request) {
	page := s.newPageData(w, r, "", "owner_tokens")
	page.Title = i18n.RequestString(r, "preferences.owner_tokens.heading")
	data := map[string]interface{}{}
	page.Data = data
	flash, hasFlash := s.takeOwnerTokenFlash(r.URL.Query().Get("result"))
	if hasFlash {
		page.Flash = &FlashMessage{Type: "success", Message: i18n.RequestString(r, flash.message)}
		if flash.token != "" {
			w.Header().Set("Cache-Control", "no-store")
		}
	}
	if identity, ok := s.ownerWebIdentity(w, r, true); ok {
		tokens, err := s.ownerManager.List(r.Context(), identity)
		if err != nil {
			s.handleInternalError(w, r, "owner token list", err)
			return
		}
		data["HasOwner"] = true
		data["ownerPrimaryMasked"], _ = s.ownerManager.Mask(r.Context(), identity, identity.TokenID)
		data["PrimaryRevealed"] = hasFlash && flash.tokenCard == "primary" && flash.token != ""
		data["PrimaryRevealedValue"] = ""
		if data["PrimaryRevealed"] == true {
			data["PrimaryRevealedValue"] = flash.token
		}
		linked := make([]map[string]interface{}, 0, len(tokens))
		for _, token := range tokens {
			if token.Kind != "linked" {
				continue
			}
			masked, _ := s.ownerManager.Mask(r.Context(), identity, token.ID)
			status := "active"
			statusKey := "preferences.owner_tokens.active"
			if token.RevokedAt != nil {
				status = "revoked"
				statusKey = "preferences.owner_tokens.revoked"
			} else if token.ExpiresAt != nil && !token.ExpiresAt.After(time.Now()) {
				status = "expired"
				statusKey = "preferences.owner_tokens.expired"
			}
			revealed := hasFlash && flash.tokenCard == strconv.FormatInt(token.ID, 10) && flash.token != ""
			expiresOn := ""
			if token.ExpiresAt != nil {
				expiresOn = token.ExpiresAt.Format("2006-01-02")
			}
			expiryValue := "never"
			if token.ExpiresAt != nil {
				remaining := time.Until(*token.ExpiresAt)
				switch {
				case durationCloseTo(remaining, 7*24*time.Hour):
					expiryValue = "7d"
				case durationCloseTo(remaining, 30*24*time.Hour):
					expiryValue = "30d"
				case durationCloseTo(remaining, 90*24*time.Hour):
					expiryValue = "90d"
				case durationCloseTo(remaining, 365*24*time.Hour):
					expiryValue = "1y"
				default:
					expiryValue = "custom"
				}
			}
			linked = append(linked, map[string]interface{}{
				"ID": token.ID, "Name": token.Name, "Masked": masked,
				"CreatedAt":  token.CreatedAt.Format(time.RFC3339),
				"LastUsedAt": formatOwnerTime(token.LastUsedAt), "ExpiresAt": formatOwnerTime(token.ExpiresAt),
				"ExpiresOn": expiresOn, "ExpiryValue": expiryValue, "Status": status,
				"StatusLabel": i18n.RequestString(r, statusKey),
				"Revealed":    revealed, "RevealedValue": tokenFlashValue(flash, revealed),
			})
		}
		data["LinkedTokens"] = linked
		data["LinkedCount"] = len(linked)
	}
	if err := s.renderer.Render(w, "owner_tokens", page); err != nil {
		s.handleInternalError(w, r, "owner token render", err)
	}
}

func durationCloseTo(actual, expected time.Duration) bool {
	delta := actual - expected
	if delta < 0 {
		delta = -delta
	}
	return delta <= 2*24*time.Hour
}

func tokenFlashValue(flash ownerTokenFlash, revealed bool) string {
	if revealed {
		return flash.token
	}
	return ""
}

func (s *Server) handleOwnerPrimaryReveal(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	token, err := s.ownerManager.Reveal(r.Context(), identity, identity.TokenID)
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.not_found"), http.StatusNotFound)
		return
	}
	s.redirectOwnerTokenAction(w, r, "primary", token, "preferences.owner_tokens.primary_revealed", "primary")
}

func (s *Server) handleOwnerPrimaryRotate(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "rotate" {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
		return
	}
	token, err := s.ownerManager.Rotate(r.Context(), identity, identity.TokenID)
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusInternalServerError)
		return
	}
	s.setOwnerCookie(w, token)
	s.redirectOwnerTokenAction(w, r, "primary", token, "preferences.owner_tokens.primary_rotated", "primary")
}

func (s *Server) handleOwnerRestore(w http.ResponseWriter, r *http.Request) {
	if s.ownerManager == nil {
		http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusServiceUnavailable)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	if _, err := s.ownerManager.Restore(r.Context(), token); err != nil {
		s.clearOwnerCookie(w)
		http.Error(w, i18n.RequestString(r, "errors.invalid_token"), http.StatusUnauthorized)
		return
	}
	s.setOwnerCookie(w, token)
	s.redirectOwnerTokenAction(w, r, "primary", "", "preferences.owner_tokens.restore_success", "")
}

func (s *Server) handleOwnerTokenForget(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ownerWebRequired(w, r); !ok {
		return
	}
	s.clearOwnerCookie(w)
	s.redirectOwnerTokenAction(w, r, "restore", "", "preferences.owner_tokens.forgot_saved_token", "")
}

func (s *Server) handleOwnerLinkedCreate(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	expiry, err := parseWebOwnerExpiry(r.FormValue("expires_at"), r.FormValue("expires_on"))
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
		return
	}
	id, token, err := s.ownerManager.CreateLinked(r.Context(), identity, r.FormValue("name"), expiry)
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
		return
	}
	s.redirectOwnerTokenAction(w, r, "token-"+strconv.FormatInt(id, 10), token, "preferences.owner_tokens.linked_created", strconv.FormatInt(id, 10))
}

func (s *Server) handleOwnerLinkedReveal(w http.ResponseWriter, r *http.Request) {
	s.ownerLinkedAction(w, r, "reveal")
}

func (s *Server) handleOwnerLinkedRotate(w http.ResponseWriter, r *http.Request) {
	s.ownerLinkedAction(w, r, "rotate")
}

func (s *Server) handleOwnerLinkedRevoke(w http.ResponseWriter, r *http.Request) {
	s.ownerLinkedAction(w, r, "revoke")
}

func (s *Server) ownerLinkedAction(w http.ResponseWriter, r *http.Request, action string) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	card := "token-" + strconv.FormatInt(id, 10)
	tokenCard := strconv.FormatInt(id, 10)
	switch action {
	case "reveal":
		token, err := s.ownerManager.Reveal(r.Context(), identity, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.redirectOwnerTokenAction(w, r, card, token, "preferences.owner_tokens.linked_revealed", tokenCard)
	case "rotate":
		token, err := s.ownerManager.Rotate(r.Context(), identity, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.redirectOwnerTokenAction(w, r, card, token, "preferences.owner_tokens.linked_rotated", tokenCard)
	case "revoke":
		if err := s.ownerManager.RevokeLinked(r.Context(), identity, id); err != nil {
			http.NotFound(w, r)
			return
		}
		s.redirectOwnerTokenAction(w, r, card, "", "preferences.owner_tokens.linked_revoked", "")
	}
}

func (s *Server) handleOwnerLinkedUpdate(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		http.NotFound(w, r)
		return
	}
	expiry, err := parseWebOwnerExpiry(r.FormValue("expires_at"), r.FormValue("expires_on"))
	if err == nil {
		err = s.ownerManager.UpdateLinked(r.Context(), identity, id, r.FormValue("name"), expiry)
	}
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
		return
	}
	s.redirectOwnerTokenAction(w, r, "token-"+strconv.FormatInt(id, 10), "", "preferences.owner_tokens.linked_updated", "")
}

func parseWebOwnerExpiry(value, expiresOn string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "never" {
		return nil, nil
	}
	if value == "custom" {
		value = strings.TrimSpace(expiresOn)
		if value == "" {
			return nil, errors.New("custom expiry requires a date")
		}
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
			return nil, errors.New("expiry must be never, 7d, 30d, 90d, 1y, or a custom date")
		}
		expiry := parsed.UTC().Add(24*time.Hour - time.Second)
		if !expiry.After(time.Now()) {
			return nil, errors.New("expiry must be in the future")
		}
		return &expiry, nil
	}
	expiry := time.Now().UTC().Add(duration)
	return &expiry, nil
}
