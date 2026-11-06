package server

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/go-chi/chi/v5"
)

const maxVideoPreviewRange = 8 << 20

var peerTubePreviewFilename = regexp.MustCompile(`^[A-Za-z0-9_-]+\.(mp4|webm)$`)

func (s *Server) handlePeerTubePreview(w http.ResponseWriter, r *http.Request) {
	if s.torRelayed(r) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	servePeerTubePreview(w, r, chi.URLParam(r, "*"), client)
}

func servePeerTubePreview(w http.ResponseWriter, r *http.Request, filename string, client *http.Client) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		localizedTextHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
		return
	}
	if !peerTubePreviewFilename.MatchString(filename) || client == nil {
		localizedTextHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}
	byteRange := r.Header.Get("Range")
	if byteRange == "" {
		byteRange = fmt.Sprintf("bytes=0-%d", maxVideoPreviewRange-1)
	}
	start, end, ok := parsePreviewRange(byteRange)
	if !ok {
		w.Header().Set("Content-Range", "bytes */*")
		localizedTextHTTPError(w, r, http.StatusRequestedRangeNotSatisfiable, "errors.bad_request")
		return
	}
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://peertube.tv/static/web-videos/"+filename, nil)
	if err != nil {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	upstream.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	upstream.Header.Set("Accept", "video/mp4, video/webm")
	upstream.Header.Set("User-Agent", "Search video preview")
	response, err := client.Do(upstream)
	if err != nil {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	defer closeutil.Close(response.Body)
	if response.StatusCode != http.StatusPartialContent {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (contentType != "video/mp4" && contentType != "video/webm") {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	actualStart, actualEnd, validRange := parseContentRange(response.Header.Get("Content-Range"))
	if !validRange || actualStart != start || actualEnd < actualStart || actualEnd > end || actualEnd-actualStart >= maxVideoPreviewRange || response.ContentLength != actualEnd-actualStart+1 {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, response.ContentLength+1))
	if err != nil || int64(len(body)) != response.ContentLength {
		localizedTextHTTPError(w, r, http.StatusBadGateway, "errors.server_error")
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Range", response.Header.Get("Content-Range"))
	w.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusPartialContent)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}

func parsePreviewRange(raw string) (int64, int64, bool) {
	if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(raw, "bytes="), "-")
	if len(parts) != 2 || parts[0] == "" {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 {
		return 0, 0, false
	}
	maxInt64 := int64(^uint64(0) >> 1)
	if start > maxInt64-maxVideoPreviewRange+1 {
		return 0, 0, false
	}
	end := start + maxVideoPreviewRange - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
	}
	if end < start {
		return 0, 0, false
	}
	if end-start >= maxVideoPreviewRange {
		end = start + maxVideoPreviewRange - 1
	}
	return start, end, true
}

func parseContentRange(raw string) (int64, int64, bool) {
	if !strings.HasPrefix(raw, "bytes ") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimPrefix(raw, "bytes "), "/")
	if len(parts) != 2 {
		return 0, 0, false
	}
	positions := strings.Split(parts[0], "-")
	if len(positions) != 2 {
		return 0, 0, false
	}
	start, startErr := strconv.ParseInt(positions[0], 10, 64)
	end, endErr := strconv.ParseInt(positions[1], 10, 64)
	total, totalErr := strconv.ParseInt(parts[1], 10, 64)
	return start, end, startErr == nil && endErr == nil && totalErr == nil && start >= 0 && end >= start && total > end
}
