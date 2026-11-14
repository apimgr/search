package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/common/httputil"
)

// ContentNegotiationMiddleware normalizes API .txt aliases and returns API
// payloads as plain text for explicit text requests and non-interactive tools.
func (h *Handler) ContentNegotiationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}

		addAPIVaryHeaders(w.Header())
		format := httputil.GetPreferredFormat(r)
		if format != "text/plain" {
			next.ServeHTTP(w, r)
			return
		}

		request := r
		if strings.HasSuffix(request.URL.Path, ".txt") {
			clone := request.Clone(request.Context())
			urlCopy := *request.URL
			urlCopy.Path = stripTxtExtension(urlCopy.Path)
			urlCopy.RawPath = ""
			clone.URL = &urlCopy
			request = clone
		}

		capture := newAPIResponseCapture()
		next.ServeHTTP(capture, request)
		addAPIVaryHeaders(capture.header)
		body, contentType := capture.body.Bytes(), capture.header.Get("Content-Type")
		if !isJSONContentType(contentType) {
			copyAPIResponse(w, capture, body, contentType)
			return
		}

		textBody, err := apiResponseText(body)
		if err != nil {
			copyAPIResponse(w, capture, body, contentType)
			return
		}

		copyAPIResponseHeaders(w.Header(), capture.header, true)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(capture.statusCode())
		_, _ = w.Write(textBody)
	})
}

type apiResponseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newAPIResponseCapture() *apiResponseCapture {
	return &apiResponseCapture{header: make(http.Header)}
}

func (c *apiResponseCapture) Header() http.Header {
	return c.header
}

func (c *apiResponseCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *apiResponseCapture) Write(body []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(body)
}

func (c *apiResponseCapture) statusCode() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

func copyAPIResponse(w http.ResponseWriter, capture *apiResponseCapture, body []byte, contentType string) {
	copyAPIResponseHeaders(w.Header(), capture.header, false)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(capture.statusCode())
	_, _ = w.Write(body)
}

func copyAPIResponseHeaders(dst, src http.Header, transformed bool) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") || (transformed && strings.EqualFold(key, "Content-Encoding")) {
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
}

func addAPIVaryHeaders(header http.Header) {
	for _, desired := range []string{"Accept", "User-Agent"} {
		found := false
		for _, value := range header.Values("Vary") {
			for _, field := range strings.Split(value, ",") {
				if strings.EqualFold(strings.TrimSpace(field), desired) || strings.TrimSpace(field) == "*" {
					found = true
					break
				}
			}
		}
		if !found {
			header.Add("Vary", desired)
		}
	}
}

func isJSONContentType(contentType string) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	return mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func apiResponseText(body []byte) ([]byte, error) {
	var payload interface{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	if response, ok := payload.(map[string]interface{}); ok {
		if success, hasOK := response["ok"].(bool); hasOK && !success {
			code, _ := response["error"].(string)
			message, _ := response["message"].(string)
			if code != "" && message != "" {
				return []byte(fmt.Sprintf("ERROR: %s: %s\n", code, message)), nil
			}
		}
		if data, exists := response["data"]; exists {
			payload = data
		}
	}

	if value, ok := payload.(string); ok {
		return []byte(value + "\n"), nil
	}

	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}
