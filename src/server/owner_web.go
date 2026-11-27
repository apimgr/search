package server

import (
	"crypto/rand"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/owner"
	"github.com/go-chi/chi/v5"
)

const ownerTokenCookie = "search_owner_token_xJ5yCXLv"

const ownerTokenViewCookie = "search_token_view"
const ownerTokenNoticeCookie = "search_token_notice"

func (s *Server) strictRateLimit(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.strictLimiter != nil && !s.strictLimiter.Allow(getClientIPSimple(r)) {
			localizedHTTPError(w, r, http.StatusTooManyRequests, "errors.rate_limited")
			return
		}
		next(w, r)
	}
}

var ownerTokenNotices = map[string]struct{}{
	"preferences.owner_tokens.primary_created":  {},
	"preferences.owner_tokens.primary_revealed": {},
	"preferences.owner_tokens.primary_rotated":  {},
	"preferences.owner_tokens.primary_revoked":  {},
	"preferences.owner_tokens.restore_success":  {},
	"preferences.owner_tokens.forgot_saved_token": {},
	"preferences.owner_tokens.linked_created":   {},
	"preferences.owner_tokens.linked_revealed":   {},
	"preferences.owner_tokens.linked_rotated":   {},
	"preferences.owner_tokens.linked_revoked":   {},
	"preferences.owner_tokens.linked_updated":   {},
}

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
		s.clearOwnerCookie(w, r)
		return owner.Identity{}, false
	}
	if requirePrimary && identity.Kind != "primary" {
		s.clearOwnerCookie(w, r)
		return owner.Identity{}, false
	}
	s.setOwnerCookie(w, r, cookie.Value)
	return identity, true
}

func (s *Server) ownerWebRequired(w http.ResponseWriter, r *http.Request) (owner.Identity, bool) {
	identity, ok := s.ownerWebIdentity(w, r, true)
	if !ok {
		http.Error(w, i18n.RequestString(r, "errors.forbidden"), http.StatusForbidden)
	}
	return identity, ok
}

func (s *Server) setOwnerCookie(w http.ResponseWriter, r *http.Request, token string) {
	maxAge := 365 * 24 * time.Hour
	configured := strings.TrimSpace(s.config.Server.Tokens.CookieMaxAge)
	if configured == "1y" {
		configured = "8760h"
	}
	if parsed, err := time.ParseDuration(configured); err == nil && parsed > 0 {
		maxAge = parsed
	}
	http.SetCookie(w, &http.Cookie{Name: ownerTokenCookie, Value: token, Path: "/", MaxAge: int(maxAge.Seconds()), Expires: time.Now().Add(maxAge), HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) clearOwnerCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: ownerTokenCookie, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) setOwnerTokenViewCookie(w http.ResponseWriter, r *http.Request, card string) bool {
	value, err := s.signOwnerTokenView(r, card)
	if err != nil {
		s.handleInternalError(w, r, "owner token view signing", err)
		return false
	}
	http.SetCookie(w, &http.Cookie{Name: ownerTokenViewCookie, Value: value, Path: "/server/preferences/tokens", MaxAge: 60, HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
	return true
}

func (s *Server) clearOwnerTokenViewCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: ownerTokenViewCookie, Value: "", Path: "/server/preferences/tokens", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) takeOwnerTokenViewCookie(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(ownerTokenViewCookie)
	if err != nil {
		return ""
	}
	s.clearOwnerTokenViewCookie(w, r)
	return s.verifyOwnerTokenView(r, strings.TrimSpace(cookie.Value))
}

func (s *Server) signOwnerTokenView(r *http.Request, card string) (string, error) {
	if card == "" || strings.ContainsAny(card, ".\r\n") {
		return "", errors.New("invalid token-view card")
	}
	if s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return "", errors.New("server database unavailable for token-view signing")
	}
	secret, err := database.EnsureSecret(r.Context(), s.dbManager.ServerDB(), database.SecretCookieSigning)
	if err != nil {
		return "", err
	}
	key, err := base64.StdEncoding.DecodeString(secret.Value)
	if err != nil {
		return "", err
	}
	issued := time.Now().UTC().Unix()
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	nonceText := base64.RawURLEncoding.EncodeToString(nonce)
	payload := fmt.Sprintf("%s:%d:%s", card, issued, nonceText)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(payload))
	return fmt.Sprintf("%s.%d.%s.%s", card, issued, nonceText, base64.RawURLEncoding.EncodeToString(mac.Sum(nil))), nil
}

func (s *Server) verifyOwnerTokenView(r *http.Request, value string) string {
	parts := strings.Split(value, ".")
	if len(parts) != 4 || parts[0] == "" || strings.ContainsAny(parts[0], "\r\n") {
		return ""
	}
	issued, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return ""
	}
	now := time.Now().UTC().Unix()
	if issued > now || now-issued > 60 {
		return ""
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(nonce) != 16 {
		return ""
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[3])
	if err != nil || len(signature) != sha256.Size || s.dbManager == nil || s.dbManager.ServerDB() == nil {
		return ""
	}
	secret, err := database.GetSecret(r.Context(), s.dbManager.ServerDB(), database.SecretCookieSigning)
	if err != nil {
		return ""
	}
	payload := fmt.Sprintf("%s:%d:%s", parts[0], issued, parts[2])
	for _, encodedKey := range []string{secret.Value, secret.PreviousValue} {
		if encodedKey == "" {
			continue
		}
		key, decodeErr := base64.StdEncoding.DecodeString(encodedKey)
		if decodeErr != nil {
			continue
		}
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(payload))
		if hmac.Equal(signature, mac.Sum(nil)) {
			return parts[0]
		}
	}
	return ""
}

func (s *Server) setOwnerTokenNoticeCookie(w http.ResponseWriter, r *http.Request, message string) {
	if _, ok := ownerTokenNotices[message]; !ok {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: ownerTokenNoticeCookie, Value: message, Path: "/server/preferences/tokens", MaxAge: 60, HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) takeOwnerTokenNoticeCookie(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie(ownerTokenNoticeCookie)
	if err != nil {
		return ""
	}
	s.clearOwnerTokenNoticeCookie(w, r)
	if _, ok := ownerTokenNotices[cookie.Value]; !ok {
		return ""
	}
	return cookie.Value
}

func (s *Server) clearOwnerTokenNoticeCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: ownerTokenNoticeCookie, Value: "", Path: "/server/preferences/tokens", MaxAge: -1, Expires: time.Unix(1, 0), HttpOnly: true, Secure: s.cookieSecure(r), SameSite: http.SameSiteStrictMode})
}

func (s *Server) redirectOwnerTokenAction(w http.ResponseWriter, r *http.Request, card, message, tokenCard string) {
	if message != "" {
		s.setOwnerTokenNoticeCookie(w, r, message)
	} else {
		s.clearOwnerTokenNoticeCookie(w, r)
	}
	if tokenCard != "" {
		if !s.setOwnerTokenViewCookie(w, r, tokenCard) {
			return
		}
	} else {
		s.clearOwnerTokenViewCookie(w, r)
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, "/server/preferences/tokens#"+card, http.StatusSeeOther)
}

func (s *Server) handleOwnerTokensPage(w http.ResponseWriter, r *http.Request) {
	s.renderOwnerTokensPage(w, r)
}

func (s *Server) renderOwnerTokensPage(w http.ResponseWriter, r *http.Request) {
	page := s.newPageData(w, r, "", "owner_tokens")
	page.Title = i18n.RequestString(r, "preferences.owner_tokens.heading")
	data := map[string]interface{}{}
	page.Data = data
	viewCard := s.takeOwnerTokenViewCookie(w, r)
	if message := s.takeOwnerTokenNoticeCookie(w, r); message != "" {
		page.Flash = &FlashMessage{Type: "success", Message: i18n.RequestString(r, message)}
	}
	if identity, ok := s.ownerWebIdentity(w, r, true); ok {
		tokens, err := s.ownerManager.List(r.Context(), identity)
		if err != nil {
			s.handleInternalError(w, r, "owner token list", err)
			return
		}
		data["HasOwner"] = true
		data["ResourceLabelPlural"] = "alerts"
		data["ownerPrimaryMasked"], _ = s.ownerManager.Mask(r.Context(), identity, identity.TokenID)
		data["PrimaryRevealed"] = false
		data["PrimaryRevealedValue"] = ""
		if viewCard == "primary" {
			if token, revealErr := s.ownerManager.Reveal(r.Context(), identity, identity.TokenID); revealErr == nil {
				data["PrimaryRevealed"] = true
				data["PrimaryRevealedValue"] = token
				w.Header().Set("Cache-Control", "no-store")
			}
		}
		linked := make([]map[string]interface{}, 0, len(tokens))
		var primaryLast4 string
		for _, token := range tokens {
			if token.Kind != "linked" {
				if token.Kind == "primary" {
					primaryLast4 = token.Last4
				}
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
			revealed := false
			revealedValue := ""
			if viewCard == token.ID {
				if tokenValue, revealErr := s.ownerManager.Reveal(r.Context(), identity, token.ID); revealErr == nil {
					revealed = true
					revealedValue = tokenValue
					w.Header().Set("Cache-Control", "no-store")
				}
			}
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
				"Revealed":    revealed, "RevealedValue": revealedValue,
			})
		}
		data["LinkedTokens"] = linked
		data["LinkedCount"] = len(linked)
		data["PrimaryLast4"] = primaryLast4
		resourceCount := int64(0)
		if s.alertManager != nil {
			resourceCount, err = s.alertManager.CountOwnerResources(r.Context(), identity.OwnerID)
			if err != nil {
				s.handleInternalError(w, r, "owner resource count", err)
				return
			}
		}
		data["ResourceCount"] = resourceCount
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

func (s *Server) handleOwnerPrimaryReveal(w http.ResponseWriter, r *http.Request) {
	_, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	s.redirectOwnerTokenAction(w, r, "primary", "preferences.owner_tokens.primary_revealed", "primary")
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
	s.setOwnerCookie(w, r, token)
	s.redirectOwnerTokenAction(w, r, "primary", "preferences.owner_tokens.primary_rotated", "primary")
}

func (s *Server) handleOwnerPrimaryRevoke(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	if _, err := s.ownerManager.RevokeOwner(r.Context(), identity, r.FormValue("confirm"), "web"); err != nil {
		if errors.Is(err, owner.ErrConfirmation) {
			s.handleError(w, r, http.StatusBadRequest, i18n.RequestString(r, "errors.invalid_request_title"), i18n.RequestString(r, "errors.bad_request"))
			return
		}
		s.handleInternalError(w, r, "owner primary token revoke", err)
		return
	}
	s.clearOwnerCookie(w, r)
	w.Header().Set("Clear-Site-Data", `"storage"`)
	s.redirectOwnerTokenAction(w, r, "new-token", "preferences.owner_tokens.primary_revoked", "")
}

func (s *Server) handleOwnerPrimaryCreate(w http.ResponseWriter, r *http.Request) {
	if s.ownerManager == nil {
		http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusServiceUnavailable)
		return
	}
	if cookie, err := r.Cookie(ownerTokenCookie); err == nil {
		if _, resolveErr := s.ownerManager.Resolve(r.Context(), cookie.Value); resolveErr == nil {
			s.handleError(w, r, http.StatusConflict, i18n.RequestString(r, "errors.invalid_request_title"), i18n.RequestString(r, "errors.bad_request"))
			return
		} else if !errors.Is(resolveErr, owner.ErrInvalidToken) {
			s.handleInternalError(w, r, "owner primary token issue identity check", resolveErr)
			return
		}
		s.clearOwnerCookie(w, r)
	}
	_, token, err := s.ownerManager.IssueWithSource(r.Context(), "issue")
	if err != nil {
		s.handleInternalError(w, r, "owner primary token issue", err)
		return
	}
	s.setOwnerCookie(w, r, token)
	s.redirectOwnerTokenAction(w, r, "primary", "preferences.owner_tokens.primary_created", "primary")
}

func (s *Server) handleOwnerRestore(w http.ResponseWriter, r *http.Request) {
	if s.ownerManager == nil {
		http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusServiceUnavailable)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	if _, err := s.ownerManager.Restore(r.Context(), token); err != nil {
		s.clearOwnerCookie(w, r)
		http.Error(w, i18n.RequestString(r, "errors.invalid_token"), http.StatusUnauthorized)
		return
	}
	s.setOwnerCookie(w, r, token)
	s.redirectOwnerTokenAction(w, r, "primary", "preferences.owner_tokens.restore_success", "")
}

func (s *Server) handleOwnerTokenForget(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.ownerWebRequired(w, r); !ok {
		return
	}
	s.clearOwnerCookie(w, r)
	w.Header().Set("Clear-Site-Data", `"storage"`)
	s.redirectOwnerTokenAction(w, r, "restore", "preferences.owner_tokens.forgot_saved_token", "")
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
	id, _, err := s.ownerManager.CreateLinked(r.Context(), identity, r.FormValue("name"), expiry)
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.bad_request"), http.StatusBadRequest)
		return
	}
	s.redirectOwnerTokenAction(w, r, "token-"+id, "preferences.owner_tokens.linked_created", id)
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
	id := chi.URLParam(r, "id")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	card := "token-" + id
	tokenCard := id
	switch action {
	case "reveal":
		listed, err := s.ownerManager.List(r.Context(), identity)
		if err != nil {
			s.handleInternalError(w, r, "owner linked token list", err)
			return
		}
		owned := false
		for _, token := range listed {
			if token.ID == id && token.Kind == "linked" {
				owned = true
				break
			}
		}
		if !owned {
			http.NotFound(w, r)
			return
		}
		s.redirectOwnerTokenAction(w, r, card, "preferences.owner_tokens.linked_revealed", tokenCard)
	case "rotate":
		_, err := s.ownerManager.Rotate(r.Context(), identity, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		s.redirectOwnerTokenAction(w, r, card, "preferences.owner_tokens.linked_rotated", tokenCard)
	case "revoke":
		if err := s.ownerManager.RevokeLinked(r.Context(), identity, id); err != nil {
			http.NotFound(w, r)
			return
		}
		s.redirectOwnerTokenAction(w, r, card, "preferences.owner_tokens.linked_revoked", "")
	}
}

func (s *Server) handleOwnerLinkedUpdate(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.ownerWebRequired(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	if id == "" {
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
	s.redirectOwnerTokenAction(w, r, "token-"+id, "preferences.owner_tokens.linked_updated", "")
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
