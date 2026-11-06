package server

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"strings"
)

const maxHTMLResponseBuffer = 8 << 20

// HTMLCacheHeaders marks HTML documents as non-cacheable and gives the exact
// rendered response a build-scoped validator. The body hash keeps dynamic or
// authenticated pages from sharing a validator when their content changes.
func HTMLCacheHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/html") {
			next.ServeHTTP(w, r)
			return
		}
		capture := &htmlResponseCapture{header: make(http.Header), writer: w}
		next.ServeHTTP(capture, r)
		if capture.passthrough {
			return
		}

		contentType := capture.header.Get("Content-Type")
		if contentType == "" && capture.body.Len() > 0 {
			probe := capture.body.Bytes()
			if len(probe) > 512 {
				probe = probe[:512]
			}
			contentType = http.DetectContentType(probe)
			capture.header.Set("Content-Type", contentType)
		}
		if !strings.HasPrefix(strings.ToLower(contentType), "text/html") {
			copyHeader(w.Header(), capture.header)
			if capture.status != 0 {
				w.WriteHeader(capture.status)
			}
			if r.Method != http.MethodHead {
				_, _ = w.Write(capture.body.Bytes())
			}
			return
		}

		cacheControl := capture.header.Get("Cache-Control")
		if !hasCacheDirective(cacheControl, "no-store") {
			if hasCacheDirective(cacheControl, "private") {
				capture.header.Set("Cache-Control", cacheControl+", no-store")
			} else {
				capture.header.Set("Cache-Control", "no-store")
			}
		}
		if capture.header.Get("ETag") == "" {
			hash := sha256.Sum256(append([]byte(buildAssetStamp()+"\x00"), capture.body.Bytes()...))
			capture.header.Set("ETag", fmt.Sprintf("\"html-%s-%x\"", buildAssetStamp(), hash[:12]))
		}
		copyHeader(w.Header(), capture.header)
		status := capture.status
		if status == 0 {
			status = http.StatusOK
		}
		if status == http.StatusOK && htmlETagMatches(r.Header.Values("If-None-Match"), capture.header.Get("ETag")) {
			w.Header().Del("Content-Length")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(capture.body.Bytes())
		}
	})
}

type htmlResponseCapture struct {
	header      http.Header
	body        bytes.Buffer
	status      int
	passthrough bool
	writer      http.ResponseWriter
}

func (c *htmlResponseCapture) Header() http.Header { return c.header }

func (c *htmlResponseCapture) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

func (c *htmlResponseCapture) Write(p []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	if c.passthrough {
		return c.writer.Write(p)
	}
	if c.body.Len()+len(p) > maxHTMLResponseBuffer {
		c.flushToWriter()
		return c.writer.Write(p)
	}
	return c.body.Write(p)
}

func (c *htmlResponseCapture) flushToWriter() {
	contentType := c.header.Get("Content-Type")
	if contentType == "" && c.body.Len() > 0 {
		probe := c.body.Bytes()
		if len(probe) > 512 {
			probe = probe[:512]
		}
		contentType = http.DetectContentType(probe)
		c.header.Set("Content-Type", contentType)
	}
	if strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		cacheControl := c.header.Get("Cache-Control")
		if !hasCacheDirective(cacheControl, "no-store") {
			if hasCacheDirective(cacheControl, "private") {
				c.header.Set("Cache-Control", cacheControl+", no-store")
			} else {
				c.header.Set("Cache-Control", "no-store")
			}
		}
		if c.header.Get("ETag") == "" {
			c.header.Set("ETag", fmt.Sprintf("\"html-%s\"", buildAssetStamp()))
		}
	}
	copyHeader(c.writer.Header(), c.header)
	status := c.status
	if status == 0 {
		status = http.StatusOK
	}
	c.writer.WriteHeader(status)
	_, _ = c.writer.Write(c.body.Bytes())
	c.body.Reset()
	c.passthrough = true
}

func htmlETagMatches(values []string, etag string) bool {
	for _, value := range values {
		for _, candidate := range strings.Split(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
				return true
			}
		}
	}
	return false
}

func hasCacheDirective(value, directive string) bool {
	for _, item := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(item), directive) {
			return true
		}
	}
	return false
}

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
}
