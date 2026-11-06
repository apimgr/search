package server

import (
	"github.com/apimgr/search/src/common/closeutil"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/direct"
)

const maxDictionaryAudioBytes = 5 << 20

func handleDictionaryAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		localizedTextHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	source := direct.SafeDictionaryAudioSource(r.URL.Query().Get("url"))
	if source == "" {
		localizedTextHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	serveDictionaryAudio(w, r, source, client)
}

func serveDictionaryAudio(w http.ResponseWriter, r *http.Request, source string, client *http.Client) {
	if direct.SafeDictionaryAudioSource(source) == "" || client == nil {
		localizedTextHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, source, nil)
	if err != nil {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	request.Header.Set("Accept", "audio/*")
	request.Header.Set("User-Agent", "Search pronunciation proxy")
	response, err := client.Do(request)
	if err != nil {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusOK {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !isSupportedAudioType(contentType) {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxDictionaryAudioBytes+1))
	if err != nil || len(body) > maxDictionaryAudioBytes {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", formatContentLength(len(body)))
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func isSupportedAudioType(contentType string) bool {
	switch strings.ToLower(contentType) {
	case "audio/mpeg", "audio/ogg", "audio/wav", "audio/x-wav", "audio/mp4", "audio/aac", "audio/webm":
		return true
	default:
		return false
	}
}

func formatContentLength(length int) string {
	return strconv.Itoa(length)
}
