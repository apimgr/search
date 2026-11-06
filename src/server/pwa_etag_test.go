package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHTMLResponsesUseBuildScopedETagWithoutReplacingPrivatePolicy(t *testing.T) {
	for _, cacheControl := range []string{"", "private, no-store"} {
		render := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if cacheControl != "" {
				w.Header().Set("Cache-Control", cacheControl)
			}
			_, _ = w.Write([]byte("<html>page</html>"))
		})
		wrapped := HTMLCacheHeaders(render)
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept", "text/html")
		first := httptest.NewRecorder()
		wrapped.ServeHTTP(first, r)

		wantCacheControl := cacheControl
		if wantCacheControl == "" {
			wantCacheControl = "no-store"
		}
		if got := first.Header().Get("Cache-Control"); got != wantCacheControl {
			t.Errorf("Cache-Control = %q, want %q", got, wantCacheControl)
		}
		etag := first.Header().Get("ETag")
		if !strings.Contains(etag, buildAssetStamp()) {
			t.Fatalf("ETag %q does not include build stamp %q", etag, buildAssetStamp())
		}

		conditional := httptest.NewRequest(http.MethodGet, "/", nil)
		conditional.Header.Set("Accept", "text/html")
		conditional.Header.Set("If-None-Match", "W/"+etag)
		second := httptest.NewRecorder()
		wrapped.ServeHTTP(second, conditional)
		if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
			t.Errorf("conditional response = %d with %d body bytes, want empty 304", second.Code, second.Body.Len())
		}
	}
}

func TestHTMLCacheMiddlewareLeavesNonHTMLResponsesAlone(t *testing.T) {
	handler := HTMLCacheHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	r := httptest.NewRequest(http.MethodGet, "/api", nil)
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Header().Get("Cache-Control") != "" || w.Header().Get("ETag") != "" {
		t.Errorf("non-HTML headers were changed: Cache-Control=%q ETag=%q", w.Header().Get("Cache-Control"), w.Header().Get("ETag"))
	}
}

func TestHTMLCacheMiddlewareRetainsHeadersWhenResponseExceedsBufferLimit(t *testing.T) {
	handler := HTMLCacheHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html>" + strings.Repeat("x", maxHTMLResponseBuffer)))
	}))
	r := httptest.NewRequest(http.MethodGet, "/large", nil)
	r.Header.Set("Accept", "text/html")
	r.Header.Set("If-None-Match", `"html-`+buildAssetStamp()+`"`)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("large conditional response status = %d, want 200 because the streaming validator cannot prove body equality", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("large HTML Cache-Control = %q, want no-store", got)
	}
	if got := w.Header().Get("ETag"); got != `"html-`+buildAssetStamp()+`"` {
		t.Errorf("large HTML ETag = %q, want build-scoped validator", got)
	}
}

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

func TestAssetURLAddsBuildStamp(t *testing.T) {
	got := assetURL("/static/css/common.css")
	req, err := url.Parse(got)
	if err != nil {
		t.Fatalf("assetURL() returned invalid URL %q: %v", got, err)
	}
	if req.Path != "/static/css/common.css" || req.Query().Get("v") != buildAssetStamp() {
		t.Errorf("assetURL() = %q, want static path with build stamp %q", got, buildAssetStamp())
	}
}

func TestStaticAssetImmutableCacheRequiresMatchingBuildStamp(t *testing.T) {
	handler := StaticFileServer()
	for _, tc := range []struct {
		name, query, want string
	}{
		{"missing stamp", "", "no-cache"},
		{"stale stamp", "?v=old", "no-cache"},
		{"matching stamp", "?v=" + buildAssetStamp(), "public, max-age=31536000, immutable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/css/common.css"+tc.query, nil))
			if got := w.Header().Get("Cache-Control"); got != tc.want {
				t.Errorf("Cache-Control = %q, want %q", got, tc.want)
			}
		})
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
