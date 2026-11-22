package server

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/common/httputil"
)

func (s *Server) contentNegotiationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/.well-known/") || strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/locales/") || strings.HasSuffix(r.URL.Path, ".xml") || r.URL.Path == "/robots.txt" || r.URL.Path == "/llms.txt" {
			next.ServeHTTP(w, r)
			return
		}
		addFrontendVary(w.Header())
		if strings.HasSuffix(r.URL.Path, ".txt") {
			clone := r.Clone(r.Context())
			urlCopy := *r.URL
			urlCopy.Path = strings.TrimSuffix(urlCopy.Path, ".txt")
			urlCopy.RawPath = ""
			clone.URL = &urlCopy
			clone.Header = r.Header.Clone()
			clone.Header.Set("Accept", "text/plain")
			next.ServeHTTP(w, clone)
			return
		}

		format := httputil.GetPreferredFormat(r)
		if format == "application/json" {
			apiPath, ok := frontendAPIPath(r.URL.Path)
			if !ok {
				http.Error(w, "No JSON representation is available for this page", http.StatusNotAcceptable)
				return
			}
			clone := r.Clone(r.Context())
			urlCopy := *r.URL
			urlCopy.Path = apiPath
			urlCopy.RawPath = ""
			clone.URL = &urlCopy
			clone.Header = r.Header.Clone()
			clone.Header.Set("Accept", "application/json")
			s.router.ServeHTTP(w, clone)
			return
		}

		if format != "text/plain" {
			addFrontendVary(w.Header())
			next.ServeHTTP(w, r)
			return
		}
		capture := &frontendResponseCapture{header: make(http.Header)}
		next.ServeHTTP(capture, r)
		body := capture.body.Bytes()
		contentType := strings.ToLower(capture.header.Get("Content-Type"))
		if contentType == "" && len(body) > 0 {
			probe := body
			if len(probe) > 512 {
				probe = probe[:512]
			}
			contentType = strings.ToLower(http.DetectContentType(probe))
		}
		if !strings.HasPrefix(contentType, "text/html") {
			addFrontendVary(capture.header)
			copyFrontendResponse(w, capture, body)
			return
		}
		copyFrontendHeaders(w.Header(), capture.header)
		addFrontendVary(w.Header())
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(capture.statusCode())
		_, _ = w.Write([]byte(httputil.HTML2TextConverter(string(body), 80)))
	})
}

func addFrontendVary(header http.Header) {
	for _, wanted := range []string{"Accept", "User-Agent"} {
		present := false
		for _, value := range header.Values("Vary") {
			for _, field := range strings.Split(value, ",") {
				if strings.EqualFold(strings.TrimSpace(field), wanted) || strings.TrimSpace(field) == "*" {
					present = true
				}
			}
		}
		if !present {
			header.Add("Vary", wanted)
		}
	}
}

func frontendAPIPath(path string) (string, bool) {
	switch path {
	case "/":
		return api.APIPrefix + "/info", true
	case "/search":
		return api.APIPrefix + "/search", true
	case "/server/preferences/sync/import":
		return api.APIPrefix + "/server/preferences/sync/import", true
	case "/server/about", "/server/privacy", "/server/dpo", "/server/help", "/server/help/mcp", "/server/terms", "/server/contact", "/server/preferences", "/server/security", "/server/security/policy", "/server/security/thanks", "/server/healthz", "/server/preferences/tokens", "/server/preferences/tools", "/server/preferences/sync":
		return api.APIPrefix + path, true
	default:
		if strings.HasPrefix(path, "/direct/") {
			return api.APIPrefix + path, true
		}
		return "", false
	}
}

type frontendResponseCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *frontendResponseCapture) Header() http.Header { return c.header }

func (c *frontendResponseCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *frontendResponseCapture) Write(body []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(body)
}

func (c *frontendResponseCapture) statusCode() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

func copyFrontendHeaders(dst, src http.Header) {
	for key, values := range src {
		if strings.EqualFold(key, "Content-Length") || strings.EqualFold(key, "Content-Encoding") {
			continue
		}
		dst[key] = append([]string(nil), values...)
	}
}

func copyFrontendResponse(w http.ResponseWriter, capture *frontendResponseCapture, body []byte) {
	copyFrontendHeaders(w.Header(), capture.header)
	w.WriteHeader(capture.statusCode())
	_, _ = w.Write(body)
}
