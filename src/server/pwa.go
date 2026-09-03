package server

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/apimgr/search/src/version"
)

// handleServiceWorker serves the service worker at the site root so its scope
// covers the entire origin (a worker served from /static/ could only control
// /static/*). Per AI.md PART 16: PWA Support.
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("sw.js")
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
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
	w.Header().Set("ETag", fmt.Sprintf("%q", "sw-"+version.Version))
	w.Write(data)
}

// handleManifest serves the PWA manifest at the site root per AI.md PART 16.
func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("manifest.json")
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/manifest+json; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}

// handleOfflinePage serves the offline fallback page at the site root per
// AI.md PART 16: PWA Support.
func (s *Server) handleOfflinePage(w http.ResponseWriter, r *http.Request) {
	data, err := GetStaticFile("offline.html")
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(data)
}
