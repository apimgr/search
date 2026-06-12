package server

import (
	"bytes"
	"io"
	"net/http"
	"time"
)

// debugMiddleware logs detailed request/response info (--debug/DEBUG=true only)
func (s *Server) debugMiddleware(next http.Handler) http.Handler {
	// No-op unless debug enabled
	if !s.config.IsDebug() {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Capture request body for logging (limit to 10KB)
		if r.Body != nil && r.ContentLength > 0 && r.ContentLength < 10*1024 {
			body, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(body))
		}

		// Wrap response writer to capture status and size
		// responseWriter is defined in middleware.go (statusCode, bytesWritten fields)
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next.ServeHTTP(rw, r)

		// Log after request completes
		s.debugLog(r, rw.statusCode, time.Since(start), rw.bytesWritten)
	})
}
