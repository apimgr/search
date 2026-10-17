package server

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/version"
)

// handleServiceWorker serves the service worker at the site root so its scope
// covers the entire origin (a worker served from /static/ could only control
// /static/*). Per AI.md PART 16: PWA Support.
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("sw.js")
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.not_found"), http.StatusNotFound)
		return
	}
	// Stamp the running build's version into the cache name so every deploy
	// gets its own cache and the worker's `activate` handler evicts the
	// previous one. Per AI.md PART 16 line 22493-22494: "Cache name MUST
	// embed {project_version}". Without this a browser that installed the
	// worker before a fix shipped keeps serving the stale cached app.js
	// forever, no matter how many times the server is redeployed.
	data = bytes.ReplaceAll(data, []byte("__CACHE_VERSION__"), []byte(version.Version))
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// Allow the worker to claim the full root scope even though the file path is /sw.js
	w.Header().Set("Service-Worker-Allowed", "/")
	// Never cache the worker itself so new versions are picked up promptly.
	// Per AI.md PART 16 line 13211/13229: no-cache + a build-stamp ETag so
	// the browser always revalidates and sees the new worker on its next
	// update check - a cached service worker script delays every other
	// update mechanism.
	w.Header().Set("Cache-Control", "no-cache")
	etag := "sw-" + version.Version
	w.Header().Set("ETag", fmt.Sprintf("%q", etag))
	writeIfNotModified(w, r, etag, data)
}

// handleManifest serves the PWA manifest at the site root per AI.md PART 16.
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("manifest.json")
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.not_found"), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	// AI.md line 13270 puts /manifest.json in the same bucket as /sw.js:
	// no-cache + ETag, never long-cached. This used to send
	// "public, max-age=86400", which let a browser keep an old manifest for a
	// full day after a release changed the app name, theme color, or icon set.
	// A stale manifest is how an update ships but the installed app still
	// looks like the previous one.
	w.Header().Set("Cache-Control", "no-cache")
	etag := "manifest-" + version.Version
	w.Header().Set("ETag", fmt.Sprintf("%q", etag))
	writeIfNotModified(w, r, etag, data)
}

// writeIfNotModified implements the conditional GET half of the no-cache +
// ETag pairing AI.md line 13270 requires. Without it the ETag is decorative:
// the client is told to revalidate, the full body comes back every time, and
// the stamp only ever documents the version to a human reading the response.
//
// The comparison is weak, as RFC 7232 section 3.2 requires for If-None-Match.
// A client may send W/"sw-1.0.0" for a strong validator and still get a 304,
// because on a cache hit the two representations are byte-identical.
func writeIfNotModified(w http.ResponseWriter, r *http.Request, etag string, data []byte) {
	if etagMatches(r.Header.Get("If-None-Match"), etag) {
		// 304 carries no body and no Content-Type; the stored copy supplies
		// both. The validators and caching directives are repeated so the
		// client refreshes them even when it only had the headers cached.
		w.WriteHeader(http.StatusNotModified)
		return
	}
	writeResponseBytes(w, data)
}

// etagMatches reports whether an If-None-Match value covers etag. The header
// may carry a comma-separated list, of which any match wins.
func etagMatches(ifNoneMatch, etag string) bool {
	// Normalize by stripping the weak prefix then the quotes, so W/"sw-1.0.0"
	// and "sw-1.0.0" compare equal.
	normalize := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "W/")
		return strings.Trim(s, `"`)
	}

	header := strings.TrimSpace(ifNoneMatch)
	if header == "" {
		return false
	}
	// "*" matches any current representation (RFC 7232 section 3.2).
	if header == "*" {
		return true
	}

	want := normalize(etag)
	for _, candidate := range strings.Split(header, ",") {
		if normalize(candidate) == want {
			return true
		}
	}
	return false
}

// handleOfflinePage serves the offline fallback page at the site root per
// AI.md PART 16: PWA Support.
func (s *Server) handleOfflinePage(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("offline.html")
	if err != nil {
		http.Error(w, i18n.RequestString(r, "errors.not_found"), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	writeResponseBytes(w, data)
}
