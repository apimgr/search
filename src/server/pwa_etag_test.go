package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestPWAAssetsAreRevalidatedNotCached pins AI.md line 13270: /sw.js and
// /manifest.json are "no-cache + ETag", never long-cached. The failure this
// catches is a browser holding a day-old manifest after a release changed the
// app name or icon set - the app installs, but looks like the previous build.
func TestPWAAssetsAreRevalidatedNotCached(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		path        string
		contentType string
	}{
		{"service worker", s.handleServiceWorker, "/sw.js", "application/javascript"},
		{"manifest", s.handleManifest, "/manifest.json", "application/manifest+json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serveOnce(tt.handler, tt.path)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if got := w.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
			}
			if w.Header().Get("ETag") == "" {
				t.Error("ETag is empty; the client has nothing to revalidate against")
			}
			if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, tt.contentType) {
				t.Errorf("Content-Type = %q, want it to mention %q", ct, tt.contentType)
			}
		})
	}
}

// TestPWAConditionalGETReturns304 is the reason the ETag exists. Sending the
// ETag back must produce a bodiless 304 rather than a fresh copy of the asset;
// a 200 here means every service-worker update check re-downloads the script.
func TestPWAConditionalGETReturns304(t *testing.T) {
	s := &Server{}

	tests := []struct {
		name    string
		handler http.HandlerFunc
		path    string
	}{
		{"service worker", s.handleServiceWorker, "/sw.js"},
		{"manifest", s.handleManifest, "/manifest.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := serveOnce(tt.handler, tt.path)
			etag := first.Header().Get("ETag")
			if etag == "" {
				t.Fatal("first response has no ETag")
			}

			// Send it back the way a browser would, with a weak validator:
			// RFC 7232 requires weak comparison for If-None-Match.
			r := httptest.NewRequest(http.MethodGet, tt.path, nil)
			r.Header.Set("If-None-Match", "W/"+etag)
			w := httptest.NewRecorder()
			tt.handler(w, r)

			if w.Code != http.StatusNotModified {
				t.Errorf("status = %d, want 304 (If-None-Match: W/%s)", w.Code, etag)
			}
			if body := w.Body.Len(); body != 0 {
				t.Errorf("304 body is %d bytes, want 0", body)
			}
		})
	}
}

// TestPWAConditionalGETStaleETagServesBody is the other direction: an ETag
// from a previous release must NOT short-circuit, or a client would keep the
// old asset forever and updates would never land.
func TestPWAConditionalGETStaleETagServesBody(t *testing.T) {
	s := &Server{}

	for _, path := range []string{"/sw.js", "/manifest.json"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Header.Set("If-None-Match", `"sw-0.0.0-previous-release"`)

			w := httptest.NewRecorder()
			if path == "/sw.js" {
				s.handleServiceWorker(w, r)
			} else {
				s.handleManifest(w, r)
			}

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 for a stale validator", w.Code)
			}
			if w.Body.Len() == 0 {
				t.Error("body is empty; the client would be left with the stale asset")
			}
		})
	}
}

// TestStaticAssetsReturn304 covers the same contract for /static/*. A CSS file
// is re-fetched on every page load; without a 304 the server re-sends the whole
// file to a browser that already has byte-identical content.
func TestStaticAssetsReturn304(t *testing.T) {
	handler := StaticFileServer()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/css/common.css", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("first request status = %d, want 200", w.Code)
	}
	etag := w.Header().Get("ETag")
	if etag == "" {
		t.Fatal("static asset has no ETag")
	}

	r := httptest.NewRequest(http.MethodGet, "/css/common.css", nil)
	r.Header.Set("If-None-Match", etag)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r)

	if w2.Code != http.StatusNotModified {
		t.Errorf("status = %d, want 304", w2.Code)
	}
	if body := w2.Body.Len(); body != 0 {
		t.Errorf("304 body is %d bytes, want 0", body)
	}
}

// TestStaticAssetETagIsContentDerived checks that two different assets get
// different validators. A single per-build value for every file would let a
// client's app.css request be answered by an app.js validation.
func TestStaticAssetETagIsContentDerived(t *testing.T) {
	handler := StaticFileServer()

	seen := make(map[string]string)
	for _, path := range []string{"/css/common.css", "/css/components.css", "/js/app.js"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", path, w.Code)
			continue
		}
		etag := w.Header().Get("ETag")
		if other, dup := seen[etag]; dup {
			t.Errorf("%s and %s share ETag %q; the validator does not identify the content", path, other, etag)
		}
		seen[etag] = path
	}
}

// TestStaticETagRejectsTraversal guards the path handling added for the ETag
// lookup: the URL path is attacker-controlled and is joined onto the embedded
// filesystem to read the bytes being hashed.
func TestStaticETagRejectsTraversal(t *testing.T) {
	for _, p := range []string{
		"/../../etc/passwd",
		"/css/../../go.mod",
		"/..",
		"/",
		"",
	} {
		t.Run(p, func(t *testing.T) {
			if _, ok := staticETag(p); ok {
				t.Errorf("staticETag(%q) produced a validator for a non-asset path", p)
			}
		})
	}
}

// TestEtagMatches covers the If-None-Match parsing rules the 304 path depends
// on. Each case is a distinct way a real client can be turned away from a body
// it does not have, or handed a 304 it should not get.
func TestEtagMatches(t *testing.T) {
	const etag = `"abc123"`

	tests := []struct {
		name   string
		header string
		want   bool
	}{
		{"empty header", "", false},
		{"exact strong match", `"abc123"`, true},
		{"weak match", `W/"abc123"`, true},
		{"star", "*", true},
		{"different tag", `"def456"`, false},
		{"weak different tag", `W/"def456"`, false},
		{"prefix is not a match", `"abc"`, false},
		{"match in a list", `"other", "abc123"`, true},
		{"weak match in a list", `"other", W/"abc123"`, true},
		{"no match in a list", `"other", "another"`, false},
		{"trailing comma tolerated", `"abc123",`, true},
		{"surrounding whitespace", `  "abc123"  `, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := etagMatches(tt.header, etag); got != tt.want {
				t.Errorf("etagMatches(%q, %q) = %v, want %v", tt.header, etag, got, tt.want)
			}
		})
	}
}

// serveOnce performs a plain GET against a handler and returns the recorder.
func serveOnce(h http.HandlerFunc, reqPath string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest(http.MethodGet, reqPath, nil))
	return w
}
