package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/owner"
	"github.com/go-chi/chi/v5"
)

const ownerTokenCookie = "search_owner_token_xJ5yCXLv"

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
		http.Error(w, "owner token unavailable", http.StatusForbidden)
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

func (s *Server) renderOwnerTokenResult(w http.ResponseWriter, r *http.Request, token string, message string) {
	extra := map[string]interface{}{"ownerTokenMessage": message}
	if token != "" {
		extra["ownerTokenRevealed"] = token
	}
	s.renderPreferences(w, r, extra)
}

func (s *Server) handleOwnerPrimaryReveal(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	token, err := s.ownerManager.Reveal(r.Context(), identity, identity.TokenID)
	if err != nil {
		http.Error(w, "token unavailable", http.StatusNotFound)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.renderOwnerTokenResult(w, r, token, "Primary token revealed")
}

func (s *Server) handleOwnerPrimaryRotate(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	if r.FormValue("confirm") != "rotate" {
		http.Error(w, "confirmation required", http.StatusBadRequest)
		return
	}
	token, err := s.ownerManager.Rotate(r.Context(), identity, identity.TokenID)
	if err != nil {
		http.Error(w, "token operation failed", http.StatusInternalServerError)
		return
	}
	s.setOwnerCookie(w, token)
	w.Header().Set("Cache-Control", "no-store")
	s.renderOwnerTokenResult(w, r, token, "Primary token rotated")
}

func (s *Server) handleOwnerRestore(w http.ResponseWriter, r *http.Request) {
	if s.ownerManager == nil {
		http.Error(w, "owner token management unavailable", http.StatusServiceUnavailable)
		return
	}
	_, err := s.ownerManager.Restore(r.Context(), strings.TrimSpace(r.FormValue("token")))
	if err != nil {
		s.clearOwnerCookie(w)
		http.Error(w, "invalid primary token", http.StatusUnauthorized)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	s.setOwnerCookie(w, token)
	http.Redirect(w, r, "/server/preferences", http.StatusSeeOther)
}

func (s *Server) handleOwnerLinkedCreate(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	expiry, err := parseWebOwnerExpiry(r.FormValue("expires_at"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, token, err := s.ownerManager.CreateLinked(r.Context(), identity, r.FormValue("name"), expiry)
	if err != nil {
		http.Error(w, "linked token could not be created", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	s.renderOwnerTokenResult(w, r, token, "Linked token created")
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
	switch action {
	case "reveal":
		token, err := s.ownerManager.Reveal(r.Context(), identity, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		s.renderOwnerTokenResult(w, r, token, "Linked token revealed")
	case "rotate":
		token, err := s.ownerManager.Rotate(r.Context(), identity, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		s.renderOwnerTokenResult(w, r, token, "Linked token rotated")
	case "revoke":
		if err := s.ownerManager.RevokeLinked(r.Context(), identity, id); err != nil {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/server/preferences", http.StatusSeeOther)
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
	expiry, err := parseWebOwnerExpiry(r.FormValue("expires_at"))
	if err == nil {
		err = s.ownerManager.UpdateLinked(r.Context(), identity, id, r.FormValue("name"), expiry)
	}
	if err != nil {
		http.Error(w, "linked token could not be updated", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/server/preferences", http.StatusSeeOther)
}

func parseWebOwnerExpiry(value string) (*time.Time, error) {
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
		expiry := parsed.UTC().Add(24*time.Hour - time.Second)
		if !expiry.After(time.Now()) {
			return nil, errors.New("expiry must be in the future")
		}
		return &expiry, nil
	}
	expiry := time.Now().UTC().Add(duration)
	return &expiry, nil
}
