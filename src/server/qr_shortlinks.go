package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/security"
	"github.com/go-chi/chi/v5"
)

const maxQRShortlinkTargetBytes = 8192

func (s *Server) createQRShortlink(ctx context.Context, target string) (string, error) {
	parsed, err := url.ParseRequestURI(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || len(target) > maxQRShortlinkTargetBytes || strings.ContainsAny(target, "\r\n\x00") {
		return "", fmt.Errorf("QR short links require an absolute HTTP or HTTPS URL without credentials")
	}
	if s.dbManager == nil || s.dbManager.ServerDB() == nil || !s.dbManager.ServerDB().IsReady() {
		return "", fmt.Errorf("short-link storage is unavailable")
	}
	key := s.config.Get().Security.EncryptionKey
	if key == "" {
		return "", fmt.Errorf("short-link encryption is unavailable")
	}
	encryptedTarget, err := security.EncryptAESGCM(key, []byte(target))
	if err != nil {
		return "", fmt.Errorf("encrypt short-link destination: %w", err)
	}
	db := s.dbManager.ServerDB()
	table := database.ServerTableName(db, "qr_shortlinks")
	for attempt := 0; attempt < 3; attempt++ {
		secret := make([]byte, 16)
		if _, err := rand.Read(secret); err != nil {
			return "", fmt.Errorf("generate short-link code: %w", err)
		}
		code := base64.RawURLEncoding.EncodeToString(secret)
		result, err := db.Exec(ctx, fmt.Sprintf("INSERT OR IGNORE INTO %s (code, target_encrypted) VALUES (?, ?)", table), code, encryptedTarget)
		if err != nil {
			return "", fmt.Errorf("store short-link destination: %w", err)
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return "", fmt.Errorf("verify short-link creation: %w", err)
		}
		if inserted == 1 {
			return code, nil
		}
	}
	return "", fmt.Errorf("could not allocate a unique short-link code")
}

func (s *Server) handleQRShortlinkRedirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	code := chi.URLParam(r, "code")
	decoded, err := base64.RawURLEncoding.DecodeString(code)
	if err != nil || len(decoded) != 16 {
		http.NotFound(w, r)
		return
	}
	if s.dbManager == nil || s.dbManager.ServerDB() == nil || !s.dbManager.ServerDB().IsReady() {
		http.Error(w, "Short-link storage unavailable", http.StatusServiceUnavailable)
		return
	}
	table := database.ServerTableName(s.dbManager.ServerDB(), "qr_shortlinks")
	var encryptedTarget string
	err = s.dbManager.ServerDB().QueryRow(r.Context(), fmt.Sprintf("SELECT target_encrypted FROM %s WHERE code = ?", table), code).Scan(&encryptedTarget)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "Short-link lookup failed", http.StatusInternalServerError)
		return
	}
	key := s.config.Get().Security.EncryptionKey
	plaintext, err := security.DecryptAESGCM(key, encryptedTarget)
	if err != nil {
		http.Error(w, "Short-link destination is unavailable", http.StatusInternalServerError)
		return
	}
	target := string(plaintext)
	parsed, err := url.ParseRequestURI(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || len(target) > maxQRShortlinkTargetBytes || strings.ContainsAny(target, "\r\n\x00") {
		http.Error(w, "Short-link destination is invalid", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
	http.Redirect(w, r, target, http.StatusFound)
}
