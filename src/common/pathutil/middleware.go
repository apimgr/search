// Package pathutil provides path security middleware
// Per AI.md PART 5: HTTP Request Path Middleware (NON-NEGOTIABLE)
package pathutil

import (
	"net/http"
	"path"
	"strings"
)

// PathSecurityMiddleware normalizes paths and blocks traversal attempts
// Per AI.md: This middleware MUST be first in the chain - before auth, before routing.
func PathSecurityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		original := r.URL.Path

		// Check both raw path and URL-decoded for traversal
		// Note: r.URL.Path is already decoded by net/http, but check RawPath too
		rawPath := r.URL.RawPath
		if rawPath == "" {
			rawPath = r.URL.Path
		}

		// Block path traversal attempts (encoded and decoded)
		// %2e = . so %2e%2e = ..
		if strings.Contains(original, "..") ||
			strings.Contains(rawPath, "..") ||
			strings.Contains(strings.ToLower(rawPath), "%2e") {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		// Normalize the path
		cleaned := path.Clean(original)

		// Ensure leading slash
		if !strings.HasPrefix(cleaned, "/") {
			cleaned = "/" + cleaned
		}

		// Preserve trailing slash for directory paths
		if original != "/" && strings.HasSuffix(original, "/") && !strings.HasSuffix(cleaned, "/") {
			cleaned += "/"
		}

		// Update request
		r.URL.Path = cleaned

		next.ServeHTTP(w, r)
	})
}
