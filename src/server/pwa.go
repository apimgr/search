package server

import (
	"net/http"
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
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	// Allow the worker to claim the full root scope even though the file path is /sw.js
	w.Header().Set("Service-Worker-Allowed", "/")
	// Never cache the worker itself so new versions are picked up promptly
	w.Header().Set("Cache-Control", "no-cache")
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
