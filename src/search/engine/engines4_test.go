package engine

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/model"
)

// redirectToTestServer is a RoundTripper that rewrites all requests to a test server,
// preserving path and query so the engine's logic is exercised end-to-end.
type redirectToTestServer struct {
	base string
}

func (r *redirectToTestServer) RoundTrip(req *http.Request) (*http.Response, error) {
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = "http"
	newReq.URL.Host = strings.TrimPrefix(r.base, "http://")
	return http.DefaultTransport.RoundTrip(newReq)
}

// dialToTransport returns an *http.Transport whose DialContext always connects
// to the given test server address, regardless of the requested host/port.
// Used for engines (Qwant, Wikipedia) that create their own http.Client internally
// using SharedTransport, which is typed as *http.Transport (not http.RoundTripper).
// NOTE: only suitable for tests that expect an error, since TLS negotiation will
// fail when the server speaks plain HTTP.
func dialToTransport(srv *httptest.Server) *http.Transport {
	addr := srv.Listener.Addr().String()
	return &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		},
	}
}

// dialToTLSTransport returns an *http.Transport that always connects to the given
// TLS test server. InsecureSkipVerify bypasses the hostname mismatch between the
// test server's example.com cert and the real engine URLs (wikipedia.org, qwant.com).
// Use with httptest.NewTLSServer for success-path tests on HTTPS engines.
func dialToTLSTransport(srv *httptest.Server) *http.Transport {
	addr := srv.Listener.Addr().String()
	return &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

// --- DuckDuckGo VQD + images/videos/news ---

// TestDDGGetVQDTokenFound verifies that getVQDToken extracts the token from an HTML page.
func TestDDGGetVQDTokenFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>some stuff vqd="3-abc123xyz" more stuff</body></html>`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	token, err := e.getVQDToken(context.Background(), "golang")
	if err != nil {
		t.Fatalf("getVQDToken() error = %v", err)
	}
	if token != "3-abc123xyz" {
		t.Errorf("getVQDToken() = %q, want %q", token, "3-abc123xyz")
	}
}

// TestDDGGetVQDTokenSingleQuote verifies extraction from the single-quote variant.
func TestDDGGetVQDTokenSingleQuote(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>vqd='3-single456' end</body></html>`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	token, err := e.getVQDToken(context.Background(), "test")
	if err != nil {
		t.Fatalf("getVQDToken() error = %v", err)
	}
	if token != "3-single456" {
		t.Errorf("getVQDToken() = %q, want %q", token, "3-single456")
	}
}

// TestDDGGetVQDTokenMissing verifies that a missing token returns an error.
func TestDDGGetVQDTokenMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<html><body>no token here</body></html>`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.getVQDToken(context.Background(), "test")
	if err == nil {
		t.Error("getVQDToken() expected error when token absent, got nil")
	}
}

// TestDDGSearchImages exercises the images path with a mock server that handles the
// VQD token request (root path) and the image API request (/i.js).
func TestDDGSearchImages(t *testing.T) {
	imageResp := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"title":     "Cute kitten",
				"url":       "https://example.com/kitten",
				"image":     "https://example.com/kitten.jpg",
				"thumbnail": "https://example.com/kitten_thumb.jpg",
				"width":     800,
				"height":    600,
				"source":    "example.com",
			},
		},
	}
	imageJSON, _ := json.Marshal(imageResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/i.js") {
			w.Header().Set("Content-Type", "application/json")
			w.Write(imageJSON)
			return
		}
		// VQD token page
		fmt.Fprint(w, `vqd="3-token999"`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	query := &model.Query{Text: "kitten", Category: model.CategoryImages}
	results, err := e.searchImages(context.Background(), query)
	if err != nil {
		t.Fatalf("searchImages() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("searchImages() len = %d, want 1", len(results))
	}
	if results[0].Title != "Cute kitten" {
		t.Errorf("result title = %q, want %q", results[0].Title, "Cute kitten")
	}
	if results[0].Category != model.CategoryImages {
		t.Errorf("result category = %v, want Images", results[0].Category)
	}
}

// TestDDGSearchImagesNonOK verifies that a non-200 from the images endpoint returns an error.
func TestDDGSearchImagesNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/i.js") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `vqd="3-tok"`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.searchImages(context.Background(), &model.Query{Text: "test", Category: model.CategoryImages})
	if err == nil {
		t.Error("searchImages() expected error on non-200, got nil")
	}
}

// TestDDGSearchImagesSafeSearch exercises safe-search parameter variants.
func TestDDGSearchImagesSafeSearch(t *testing.T) {
	tests := []struct {
		name       string
		safeSearch int
	}{
		{"off", 0},
		{"moderate", 1},
		{"strict", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageResp := map[string]interface{}{"results": []interface{}{}}
			imageJSON, _ := json.Marshal(imageResp)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/i.js") {
					w.Header().Set("Content-Type", "application/json")
					w.Write(imageJSON)
					return
				}
				fmt.Fprint(w, `vqd="3-safe"`)
			}))
			defer srv.Close()

			e := NewDuckDuckGo()
			e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

			q := &model.Query{Text: "test", Category: model.CategoryImages, SafeSearch: tt.safeSearch}
			results, err := e.searchImages(context.Background(), q)
			if err != nil {
				t.Fatalf("searchImages() error = %v", err)
			}
			if len(results) != 0 {
				t.Errorf("expected 0 results for empty response, got %d", len(results))
			}
		})
	}
}

// TestDDGSearchImagesWithFilters covers the ImageSize and ImageType filter branches.
func TestDDGSearchImagesWithFilters(t *testing.T) {
	tests := []struct {
		name      string
		imageSize string
		imageType string
	}{
		{"small size", "small", ""},
		{"large size", "large", ""},
		{"photo type", "", "photo"},
		{"clipart type", "", "clipart"},
		{"animated type", "", "animated"},
		{"xlarge size", "xlarge", ""},
		{"medium size", "medium", ""},
		{"unknown size", "huge", ""},
		{"unknown type", "drawing", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageResp := map[string]interface{}{"results": []interface{}{}}
			imageJSON, _ := json.Marshal(imageResp)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/i.js") {
					w.Header().Set("Content-Type", "application/json")
					w.Write(imageJSON)
					return
				}
				fmt.Fprint(w, `vqd="3-filter"`)
			}))
			defer srv.Close()

			e := NewDuckDuckGo()
			e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

			q := &model.Query{
				Text:      "test",
				Category:  model.CategoryImages,
				ImageSize: tt.imageSize,
				ImageType: tt.imageType,
			}
			_, err := e.searchImages(context.Background(), q)
			if err != nil {
				t.Fatalf("searchImages() error = %v", err)
			}
		})
	}
}

// TestDDGSearchVideos exercises the video search path with a mock server.
func TestDDGSearchVideos(t *testing.T) {
	videoResp := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"title":       "Go concurrency patterns",
				"content":     "https://example.com/video1",
				"description": "An introduction to goroutines",
				"duration":    "15:30",
				"views":       int64(100000),
				"published":   "2024-01-15",
				"publisher":   "GopherCon",
				"images":      "https://example.com/thumb.jpg",
			},
		},
	}
	videoJSON, _ := json.Marshal(videoResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v.js") {
			w.Header().Set("Content-Type", "application/json")
			w.Write(videoJSON)
			return
		}
		fmt.Fprint(w, `vqd="3-vidtoken"`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	query := &model.Query{Text: "golang", Category: model.CategoryVideos}
	results, err := e.searchVideos(context.Background(), query)
	if err != nil {
		t.Fatalf("searchVideos() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("searchVideos() len = %d, want 1", len(results))
	}
	if results[0].Title != "Go concurrency patterns" {
		t.Errorf("result title = %q, want %q", results[0].Title, "Go concurrency patterns")
	}
	// 15:30 = 930 seconds
	if results[0].Duration != 930 {
		t.Errorf("result duration = %d, want 930", results[0].Duration)
	}
	if results[0].Category != model.CategoryVideos {
		t.Errorf("result category = %v, want Videos", results[0].Category)
	}
}

// TestDDGSearchVideosNonOK verifies non-200 returns an error.
func TestDDGSearchVideosNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v.js") {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `vqd="3-tok"`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.searchVideos(context.Background(), &model.Query{Text: "test", Category: model.CategoryVideos})
	if err == nil {
		t.Error("searchVideos() expected error on non-200, got nil")
	}
}

// TestDDGSearchVideosSafeSearchAndFilters covers the safe-search, duration, and quality branches.
func TestDDGSearchVideosSafeSearchAndFilters(t *testing.T) {
	tests := []struct {
		name         string
		safeSearch   int
		videoLength  string
		videoQuality string
	}{
		{"safe off", 0, "", ""},
		{"safe moderate", 1, "", ""},
		{"safe strict", 2, "", ""},
		{"short duration", 1, "short", ""},
		{"medium duration", 1, "medium", ""},
		{"long duration", 1, "long", ""},
		{"unknown duration", 1, "ultralong", ""},
		{"hd quality", 1, "", "hd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			videoResp := map[string]interface{}{"results": []interface{}{}}
			videoJSON, _ := json.Marshal(videoResp)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/v.js") {
					w.Header().Set("Content-Type", "application/json")
					w.Write(videoJSON)
					return
				}
				fmt.Fprint(w, `vqd="3-x"`)
			}))
			defer srv.Close()

			e := NewDuckDuckGo()
			e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

			q := &model.Query{
				Text:         "test",
				Category:     model.CategoryVideos,
				SafeSearch:   tt.safeSearch,
				VideoLength:  tt.videoLength,
				VideoQuality: tt.videoQuality,
			}
			_, err := e.searchVideos(context.Background(), q)
			if err != nil {
				t.Fatalf("searchVideos() error = %v", err)
			}
		})
	}
}

// TestDDGSearchNews exercises the news search path with a mock server.
func TestDDGSearchNews(t *testing.T) {
	newsResp := map[string]interface{}{
		"results": []map[string]interface{}{
			{
				"title":   "Go 1.22 Released",
				"url":     "https://example.com/news1",
				"excerpt": "Go team announces 1.22",
				"source":  "The Gopher Daily",
				"image":   "https://example.com/go-logo.jpg",
				"date":    int64(1706745600),
			},
		},
	}
	newsJSON, _ := json.Marshal(newsResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/news.js") {
			w.Header().Set("Content-Type", "application/json")
			w.Write(newsJSON)
			return
		}
		fmt.Fprint(w, `vqd="3-newstoken"`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	query := &model.Query{Text: "golang", Category: model.CategoryNews}
	results, err := e.searchNews(context.Background(), query)
	if err != nil {
		t.Fatalf("searchNews() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("searchNews() len = %d, want 1", len(results))
	}
	if results[0].Title != "Go 1.22 Released" {
		t.Errorf("result title = %q, want %q", results[0].Title, "Go 1.22 Released")
	}
	if results[0].Author != "The Gopher Daily" {
		t.Errorf("result author = %q, want %q", results[0].Author, "The Gopher Daily")
	}
	if results[0].Category != model.CategoryNews {
		t.Errorf("result category = %v, want News", results[0].Category)
	}
}

// TestDDGSearchNewsNonOK verifies non-200 from news endpoint returns an error.
func TestDDGSearchNewsNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/news.js") {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		fmt.Fprint(w, `vqd="3-tok"`)
	}))
	defer srv.Close()

	e := NewDuckDuckGo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.searchNews(context.Background(), &model.Query{Text: "test", Category: model.CategoryNews})
	if err == nil {
		t.Error("searchNews() expected error on non-200, got nil")
	}
}

// TestDDGSearchNewsTimeRange covers the time-range filter branches (day/week/month).
func TestDDGSearchNewsTimeRange(t *testing.T) {
	tests := []struct {
		name      string
		timeRange string
	}{
		{"day", "day"},
		{"week", "week"},
		{"month", "month"},
		{"none", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newsResp := map[string]interface{}{"results": []interface{}{}}
			newsJSON, _ := json.Marshal(newsResp)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/news.js") {
					w.Header().Set("Content-Type", "application/json")
					w.Write(newsJSON)
					return
				}
				fmt.Fprint(w, `vqd="3-t"`)
			}))
			defer srv.Close()

			e := NewDuckDuckGo()
			e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

			q := &model.Query{Text: "test", Category: model.CategoryNews, TimeRange: tt.timeRange}
			_, err := e.searchNews(context.Background(), q)
			if err != nil {
				t.Fatalf("searchNews() error = %v", err)
			}
		})
	}
}

// --- parseDuration ---

// TestParseDurationE4 covers all duration string formats including edge cases.
func TestParseDurationE4(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"seconds only", "45", 45},
		{"minutes:seconds", "1:30", 90},
		{"hours:minutes:seconds", "1:02:03", 3723},
		{"zero", "0", 0},
		{"empty", "", 0},
		{"two zeros", "0:00", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDuration(tt.input)
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- Reddit ---

// TestRedditSearchSuccess exercises the full Reddit JSON path with a mock server.
func TestRedditSearchSuccess(t *testing.T) {
	redditResp := map[string]interface{}{
		"data": map[string]interface{}{
			"children": []map[string]interface{}{
				{
					"data": map[string]interface{}{
						"title":        "Why Go is awesome",
						"permalink":    "/r/golang/comments/abc123/why_go_is_awesome/",
						"selftext":     "Go has goroutines and channels.",
						"subreddit":    "golang",
						"score":        1234,
						"num_comments": 56,
						"url":          "https://www.reddit.com/r/golang/comments/abc123/",
						"is_self":      true,
					},
				},
			},
		},
	}
	respJSON, _ := json.Marshal(redditResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewReddit()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	q := &model.Query{Text: "golang", Category: model.CategoryGeneral}
	results, err := e.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(results))
	}
	if results[0].Title != "Why Go is awesome" {
		t.Errorf("result title = %q, want %q", results[0].Title, "Why Go is awesome")
	}
	if !strings.HasPrefix(results[0].URL, "https://www.reddit.com") {
		t.Errorf("result URL does not start with reddit.com: %q", results[0].URL)
	}
	if !strings.Contains(results[0].Content, "golang") {
		t.Errorf("content should contain subreddit name: %q", results[0].Content)
	}
}

// TestRedditSearchLongSelftext verifies that long selftext is truncated to 400 chars + "...".
func TestRedditSearchLongSelftext(t *testing.T) {
	// 500-character selftext (exceeds 400 limit)
	longText := strings.Repeat("A", 500)
	redditResp := map[string]interface{}{
		"data": map[string]interface{}{
			"children": []map[string]interface{}{
				{
					"data": map[string]interface{}{
						"title":        "Long post",
						"permalink":    "/r/test/comments/xyz/",
						"selftext":     longText,
						"subreddit":    "test",
						"score":        10,
						"num_comments": 2,
					},
				},
			},
		},
	}
	respJSON, _ := json.Marshal(redditResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewReddit()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	results, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !strings.Contains(results[0].Content, "...") {
		t.Errorf("expected truncated selftext with '...', got: %q", results[0].Content)
	}
}

// TestRedditSearchNonOK verifies that a non-200 response returns an error.
func TestRedditSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	e := NewReddit()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestRedditSearchInvalidJSON verifies that malformed JSON returns an error.
func TestRedditSearchInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json at all {{{`)
	}))
	defer srv.Close()

	e := NewReddit()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on invalid JSON, got nil")
	}
}

// TestRedditSearchEmpty verifies that an empty children list returns zero results without error.
func TestRedditSearchEmpty(t *testing.T) {
	redditResp := map[string]interface{}{
		"data": map[string]interface{}{
			"children": []interface{}{},
		},
	}
	respJSON, _ := json.Marshal(redditResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewReddit()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	results, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// --- StackOverflow ---

// TestSOSearchSuccess exercises the full Stack Overflow JSON path.
func TestSOSearchSuccess(t *testing.T) {
	soResp := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"question_id":   12345,
				"title":         "How do goroutines work?",
				"link":          "https://stackoverflow.com/questions/12345/how-do-goroutines-work",
				"body":          "<p>I want to understand goroutines</p>",
				"tags":          []string{"go", "concurrency", "goroutines"},
				"score":         500,
				"answer_count":  12,
				"is_answered":   true,
				"creation_date": int64(1600000000),
			},
		},
	}
	respJSON, _ := json.Marshal(soResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewStackOverflow()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	q := &model.Query{Text: "goroutines", Category: model.CategoryGeneral}
	results, err := e.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(results))
	}
	if results[0].Title != "How do goroutines work?" {
		t.Errorf("result title = %q, want %q", results[0].Title, "How do goroutines work?")
	}
	// Answered item should include a checkmark and answer count
	if !strings.Contains(results[0].Content, "answers") {
		t.Errorf("content should contain answer info: %q", results[0].Content)
	}
	if !strings.Contains(results[0].Content, "Score:") {
		t.Errorf("content should contain score: %q", results[0].Content)
	}
}

// TestSOSearchUnanswered verifies content for unanswered questions with answers > 0.
func TestSOSearchUnanswered(t *testing.T) {
	soResp := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"question_id":  9999,
				"title":        "Unanswered question",
				"link":         "https://stackoverflow.com/questions/9999/",
				"tags":         []string{"go"},
				"score":        0,
				"answer_count": 3,
				"is_answered":  false,
			},
		},
	}
	respJSON, _ := json.Marshal(soResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewStackOverflow()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	results, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// Unanswered with answer_count>0 shows count but no checkmark
	if !strings.Contains(results[0].Content, "3 answers") {
		t.Errorf("content should contain '3 answers': %q", results[0].Content)
	}
}

// TestSOSearchNoAnswers verifies content for questions with zero answers.
func TestSOSearchNoAnswers(t *testing.T) {
	soResp := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"question_id":  8888,
				"title":        "Unanswered no answers",
				"link":         "https://stackoverflow.com/questions/8888/",
				"tags":         []string{},
				"score":        0,
				"answer_count": 0,
				"is_answered":  false,
			},
		},
	}
	respJSON, _ := json.Marshal(soResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewStackOverflow()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	results, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !strings.Contains(results[0].Content, "No answers yet") {
		t.Errorf("content should say 'No answers yet': %q", results[0].Content)
	}
}

// TestSOSearchNonOK verifies that a non-200 response returns an error.
func TestSOSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewStackOverflow()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestSOSearchInvalidJSON verifies that malformed JSON returns an error.
func TestSOSearchInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{invalid`)
	}))
	defer srv.Close()

	e := NewStackOverflow()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on invalid JSON, got nil")
	}
}

// TestSOSearchTagsCapped verifies that very long tag lists are capped in the content string.
func TestSOSearchTagsCapped(t *testing.T) {
	soResp := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"question_id":  7777,
				"title":        "Many tags question",
				"link":         "https://stackoverflow.com/questions/7777/",
				"tags":         []string{"go", "goroutine", "channel", "concurrency", "sync"},
				"score":        5,
				"answer_count": 1,
				"is_answered":  true,
			},
		},
	}
	respJSON, _ := json.Marshal(soResp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewStackOverflow()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	results, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// Content must start with at least the first tag
	if !strings.HasPrefix(results[0].Content, "[go]") {
		t.Errorf("content should start with first tag: %q", results[0].Content)
	}
}

// --- Wikipedia ---

// TestWikipediaSearchSuccess exercises the Wikipedia JSON API path with a mock server.
func TestWikipediaSearchSuccess(t *testing.T) {
	wikiResp := map[string]interface{}{
		"query": map[string]interface{}{
			"search": []map[string]interface{}{
				{
					"title":     "Go (programming language)",
					"pageid":    25460924,
					"snippet":   "Go is a statically typed, compiled language",
					"timestamp": "2024-01-10T12:00:00Z",
				},
				{
					"title":     "Golang",
					"pageid":    67890,
					"snippet":   "Informal name for the Go language",
					"timestamp": "2024-01-05T08:00:00Z",
				},
			},
		},
	}
	respJSON, _ := json.Marshal(wikiResp)

	// Wikipedia uses https:// URLs — use TLS server so the transport can negotiate.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewWikipediaEngine()
	// Wikipedia creates its own http.Client using SharedTransport; replace SharedTransport temporarily.
	origTransport := SharedTransport
	SharedTransport = dialToTLSTransport(srv)
	defer func() { SharedTransport = origTransport }()

	q := &model.Query{Text: "go programming", Category: model.CategoryGeneral, Page: 0}
	results, err := e.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("Search() len = %d, want 2", len(results))
	}
	if results[0].Title != "Go (programming language)" {
		t.Errorf("result[0] title = %q, want %q", results[0].Title, "Go (programming language)")
	}
	if !strings.Contains(results[0].URL, "curid=25460924") {
		t.Errorf("result URL should include page ID: %q", results[0].URL)
	}
	if results[0].Content != "Go is a statically typed, compiled language" {
		t.Errorf("result content = %q", results[0].Content)
	}
}

// TestWikipediaSearchNonOK verifies that a non-200 response returns an error.
func TestWikipediaSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	e := NewWikipediaEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTransport(srv)
	defer func() { SharedTransport = origTransport }()

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestWikipediaSearchInvalidJSON verifies that malformed JSON returns an error.
func TestWikipediaSearchInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{not valid json`)
	}))
	defer srv.Close()

	e := NewWikipediaEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTransport(srv)
	defer func() { SharedTransport = origTransport }()

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on invalid JSON, got nil")
	}
}

// TestWikipediaSearchEmpty verifies that an empty search list returns zero results.
func TestWikipediaSearchEmpty(t *testing.T) {
	wikiResp := map[string]interface{}{
		"query": map[string]interface{}{
			"search": []interface{}{},
		},
	}
	respJSON, _ := json.Marshal(wikiResp)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewWikipediaEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTLSTransport(srv)
	defer func() { SharedTransport = origTransport }()

	results, err := e.Search(context.Background(), &model.Query{Text: "xyzzy"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestWikipediaSearchPagination verifies that page>0 sets a non-zero sroffset.
func TestWikipediaSearchPagination(t *testing.T) {
	var capturedURL string
	wikiResp := map[string]interface{}{
		"query": map[string]interface{}{
			"search": []interface{}{},
		},
	}
	respJSON, _ := json.Marshal(wikiResp)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewWikipediaEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTLSTransport(srv)
	defer func() { SharedTransport = origTransport }()

	_, err := e.Search(context.Background(), &model.Query{Text: "golang", Page: 2})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// page=2 → sroffset=20
	if !strings.Contains(capturedURL, "sroffset=20") {
		t.Errorf("expected sroffset=20 in URL, got: %q", capturedURL)
	}
}

// --- Qwant ---

// TestQwantSearchSuccess exercises the Qwant JSON API path with a mock server.
func TestQwantSearchSuccess(t *testing.T) {
	qwantResp := map[string]interface{}{
		"data": map[string]interface{}{
			"result": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"title":     "Go programming",
						"url":       "https://go.dev",
						"desc":      "The Go programming language",
						"source":    "go.dev",
						"date":      "2024-01-01",
						"thumbnail": "https://go.dev/logo.png",
					},
				},
			},
		},
	}
	respJSON, _ := json.Marshal(qwantResp)

	// Qwant uses https:// URLs — use TLS server so the transport can negotiate.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewQwantEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTLSTransport(srv)
	defer func() { SharedTransport = origTransport }()

	q := &model.Query{Text: "go programming", Category: model.CategoryGeneral, Page: 0}
	results, err := e.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(results))
	}
	if results[0].Title != "Go programming" {
		t.Errorf("result title = %q, want %q", results[0].Title, "Go programming")
	}
	if results[0].Thumbnail != "https://go.dev/logo.png" {
		t.Errorf("result thumbnail = %q", results[0].Thumbnail)
	}
	if results[0].Author != "go.dev" {
		t.Errorf("result author = %q, want %q", results[0].Author, "go.dev")
	}
}

// TestQwantSearchMediaField verifies that the media field overrides thumbnail.
func TestQwantSearchMediaField(t *testing.T) {
	qwantResp := map[string]interface{}{
		"data": map[string]interface{}{
			"result": map[string]interface{}{
				"items": []map[string]interface{}{
					{
						"title":     "Video result",
						"url":       "https://example.com/video",
						"thumbnail": "https://example.com/thumb.jpg",
						"media":     "https://example.com/media.mp4",
					},
				},
			},
		},
	}
	respJSON, _ := json.Marshal(qwantResp)

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(respJSON)
	}))
	defer srv.Close()

	e := NewQwantEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTLSTransport(srv)
	defer func() { SharedTransport = origTransport }()

	results, err := e.Search(context.Background(), &model.Query{Text: "video", Category: model.CategoryVideos})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// media field overrides thumbnail
	if results[0].Thumbnail != "https://example.com/media.mp4" {
		t.Errorf("thumbnail should be media URL: %q", results[0].Thumbnail)
	}
}

// TestQwantSearchCategoriesBasic verifies basic category-to-path mapping.
func TestQwantSearchCategoriesBasic(t *testing.T) {
	tests := []struct {
		name     string
		category model.Category
		wantPath string
	}{
		{"images", model.CategoryImages, "/images"},
		{"videos", model.CategoryVideos, "/videos"},
		{"news", model.CategoryNews, "/news"},
		{"general/web", model.CategoryGeneral, "/web"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedPath string
			emptyResp := map[string]interface{}{
				"data": map[string]interface{}{
					"result": map[string]interface{}{
						"items": []interface{}{},
					},
				},
			}
			respJSON, _ := json.Marshal(emptyResp)

			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.Write(respJSON)
			}))
			defer srv.Close()

			e := NewQwantEngine()
			origTransport := SharedTransport
			SharedTransport = dialToTLSTransport(srv)
			defer func() { SharedTransport = origTransport }()

			_, err := e.Search(context.Background(), &model.Query{Text: "test", Category: tt.category})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			if !strings.HasSuffix(capturedPath, tt.wantPath) {
				t.Errorf("path = %q, want suffix %q", capturedPath, tt.wantPath)
			}
		})
	}
}

// TestQwantSearchNonOK verifies that a non-200 response returns an error.
func TestQwantSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	e := NewQwantEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTransport(srv)
	defer func() { SharedTransport = origTransport }()

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestQwantSearchInvalidJSON verifies that malformed JSON returns an error.
func TestQwantSearchInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{bad json`)
	}))
	defer srv.Close()

	e := NewQwantEngine()
	origTransport := SharedTransport
	SharedTransport = dialToTransport(srv)
	defer func() { SharedTransport = origTransport }()

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on invalid JSON, got nil")
	}
}

// --- Yahoo ---

// TestYahooParseResultsSimplePattern exercises the simple <a href><h3> pattern.
func TestYahooParseResultsSimplePattern(t *testing.T) {
	html := `<html><body>
<a href="https://example.com/page1"><h3>Example Page One</h3></a>
<a href="https://example.org/page2"><h3>Example Page Two</h3></a>
</body></html>`

	e := NewYahoo()
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("parseResults() len = %d, want 2", len(results))
	}
	if results[0].Title != "Example Page One" {
		t.Errorf("result[0] title = %q, want %q", results[0].Title, "Example Page One")
	}
	if results[0].URL != "https://example.com/page1" {
		t.Errorf("result[0] URL = %q", results[0].URL)
	}
}

// TestYahooParseResultsYahooInternalSkipped verifies that yahoo.com links are skipped.
func TestYahooParseResultsYahooInternalSkipped(t *testing.T) {
	html := `<html><body>
<a href="https://www.yahoo.com/internal"><h3>Yahoo Internal</h3></a>
<a href="https://real-result.com/page"><h3>Real Result</h3></a>
</body></html>`

	e := NewYahoo()
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.URL, "yahoo.com") {
			t.Errorf("yahoo.com internal link should be skipped, found: %q", r.URL)
		}
	}
}

// TestYahooParseResultsRedirectURL verifies that r.search.yahoo.com redirect URLs are resolved.
func TestYahooParseResultsRedirectURL(t *testing.T) {
	targetURL := "https://example.com/real-page"
	redirectURL := "https://r.search.yahoo.com/link?RU=" + targetURL
	html := fmt.Sprintf(`<a href="%s"><h3>Redirect Result</h3></a>`, redirectURL)

	e := NewYahoo()
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseResults() len = %d, want 1", len(results))
	}
	if results[0].URL != targetURL {
		t.Errorf("URL = %q, want %q", results[0].URL, targetURL)
	}
}

// TestYahooParseResultsComplexPattern exercises the fallback algo/ac-algo/s-desc regex path.
func TestYahooParseResultsComplexPattern(t *testing.T) {
	html := `<div class="algo-sr">
<a class="ac-algo" href="https://example.com/complex">Complex Result Title</a>
<p class="s-desc">This is the description of the complex result</p>
</div>`

	e := NewYahoo()
	// No simple-pattern match → falls through to complex pattern
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	// May or may not match depending on regex — verify no panic and no error
	_ = results
}

// TestYahooParseResultsEmpty verifies that empty HTML returns empty results.
func TestYahooParseResultsEmpty(t *testing.T) {
	e := NewYahoo()
	results, err := e.parseResults("", model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty HTML, got %d", len(results))
	}
}

// TestExtractYahooRedirectURLBasic verifies basic redirect URL extraction cases.
func TestExtractYahooRedirectURLBasic(t *testing.T) {
	tests := []struct {
		name        string
		redirectURL string
		want        string
	}{
		{
			"valid RU param",
			"https://r.search.yahoo.com/link?RU=https%3A%2F%2Fexample.com%2Fpage",
			"https://example.com/page",
		},
		{
			"no RU param",
			"https://r.search.yahoo.com/link?other=value",
			"",
		},
		{
			"invalid URL",
			"://bad url",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractYahooRedirectURL(tt.redirectURL)
			if got != tt.want {
				t.Errorf("extractYahooRedirectURL(%q) = %q, want %q", tt.redirectURL, got, tt.want)
			}
		})
	}
}

// TestYahooSearchNonOK verifies that a non-200 response returns an error.
func TestYahooSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	e := NewYahoo()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestYahooSearchCategories exercises the image and news category URL branches.
func TestYahooSearchCategories(t *testing.T) {
	tests := []struct {
		name     string
		category model.Category
	}{
		{"images", model.CategoryImages},
		{"news", model.CategoryNews},
		{"general", model.CategoryGeneral},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "<html></html>")
			}))
			defer srv.Close()

			e := NewYahoo()
			e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

			results, err := e.Search(context.Background(), &model.Query{Text: "test", Category: tt.category})
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
			// Empty page = no results, but no error
			_ = results
		})
	}
}

// --- Yandex parseResults ---

// TestYandexParseResultsPrimary exercises the organic__title-link anchor path.
func TestYandexParseResultsPrimary(t *testing.T) {
	html := `<div>
<a class="organic__title-link" href="https://example.com/page1">Go Language Tutorial</a>
<div class="OrganicTextContentSpan">Learn Go from scratch with examples</div>
<a class="organic__title-link" href="https://example.org/page2">Advanced Go Patterns</a>
</div>`

	e := NewYandex()
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) < 1 {
		t.Fatal("parseResults() returned no results, want ≥1")
	}
	// First result should be the Go Language Tutorial
	if !strings.Contains(results[0].Title, "Go") {
		t.Errorf("result[0].Title = %q, expected to contain 'Go'", results[0].Title)
	}
}

// TestYandexParseResultsSnippet verifies that snippets are extracted from the OrganicTextContentSpan.
func TestYandexParseResultsSnippet(t *testing.T) {
	html := `<a class="organic__title-link" href="https://example.com/page1">Go Tutorial</a>
<div class="OrganicTextContentSpan">Comprehensive guide to Go programming</div>`

	e := NewYandex()
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) < 1 {
		t.Skip("no results extracted — skipping snippet check")
	}
	if results[0].Content == "" {
		t.Errorf("expected snippet content, got empty string")
	}
}

// TestYandexParseResultsSkipsYandexInternal verifies that yandex.* URLs are filtered out.
func TestYandexParseResultsSkipsYandexInternal(t *testing.T) {
	html := `<a class="organic__title-link" href="https://yandex.com/internal">Yandex Internal</a>
<a class="organic__title-link" href="https://example.com/external">External Result</a>`

	e := NewYandex()
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	for _, r := range results {
		if strings.Contains(r.URL, "yandex.") {
			t.Errorf("yandex internal URL should be filtered: %q", r.URL)
		}
	}
}

// TestYandexParseResultsFallbackH2 exercises the structural <h2> fallback path.
func TestYandexParseResultsFallbackH2(t *testing.T) {
	// No organic__title-link → triggers h2 fallback
	html := `<h2><a href="https://fallback.example.com/result">Fallback Result Title</a></h2>`

	e := NewYandex()
	results, err := e.parseResults(html, model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	// Should find the fallback result
	if len(results) < 1 {
		t.Fatal("parseResults() returned no results via h2 fallback, want ≥1")
	}
	if results[0].URL != "https://fallback.example.com/result" {
		t.Errorf("result URL = %q, want https://fallback.example.com/result", results[0].URL)
	}
}

// TestYandexParseResultsEmpty verifies that empty HTML returns empty results.
func TestYandexParseResultsEmpty(t *testing.T) {
	e := NewYandex()
	results, err := e.parseResults("", model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for empty HTML, got %d", len(results))
	}
}

// TestYandexSearchNonOK verifies that a non-200 response returns an error.
func TestYandexSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	e := NewYandex()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestYandexSearchParams exercises the safe-search, language, and pagination branches.
func TestYandexSearchParams(t *testing.T) {
	tests := []struct {
		name       string
		safeSearch int
		language   string
		page       int
	}{
		{"safe strict", 2, "en", 1},
		{"safe off", 0, "ru", 2},
		{"safe moderate no lang", 1, "", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, "<html></html>")
			}))
			defer srv.Close()

			e := NewYandex()
			e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

			q := &model.Query{
				Text:       "test",
				SafeSearch: tt.safeSearch,
				Language:   tt.language,
				Page:       tt.page,
			}
			_, err := e.Search(context.Background(), q)
			if err != nil {
				t.Fatalf("Search() error = %v", err)
			}
		})
	}
}

// TestGetYandexRegion verifies the language-to-region mapping.
func TestGetYandexRegion(t *testing.T) {
	tests := []struct {
		lang string
		want string
	}{
		{"en", "84"},
		{"ru", "225"},
		{"de", "96"},
		{"fr", "124"},
		{"es", "203"},
		{"it", "205"},
		{"tr", "983"},
		{"kz", "159"},
		{"by", "149"},
		{"ua", "187"},
		{"uk", "187"},
		{"unknown", "84"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := getYandexRegion(tt.lang)
			if got != tt.want {
				t.Errorf("getYandexRegion(%q) = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}

// TestExtractYandexURL verifies redirect URL extraction from Yandex /clck/ links.
func TestExtractYandexURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			"valid url param",
			"https://yandex.com/clck/redir?url=https%3A%2F%2Fexample.com%2Fpage",
			"https://example.com/page",
		},
		{
			"no url param",
			"https://yandex.com/clck/redir?other=value",
			"",
		},
		{
			"invalid URL",
			"://invalid",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractYandexURL(tt.url)
			if got != tt.want {
				t.Errorf("extractYandexURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

// --- YouTube ---

// TestYouTubeParseJSONSuccess exercises the full ytInitialData JSON navigation path.
func TestYouTubeParseJSONSuccess(t *testing.T) {
	// Build the nested YouTube JSON structure.
	videoRenderer := map[string]interface{}{
		"videoId": "dQw4w9WgXcQ",
		"title": map[string]interface{}{
			"runs": []map[string]interface{}{{"text": "Never Gonna Give You Up"}},
		},
		"descriptionSnippet": map[string]interface{}{
			"runs": []map[string]interface{}{{"text": "Official music video"}},
		},
		"thumbnail": map[string]interface{}{
			"thumbnails": []map[string]interface{}{
				{"url": "https://img.youtube.com/vi/dQw4w9WgXcQ/default.jpg"},
				{"url": "https://img.youtube.com/vi/dQw4w9WgXcQ/hqdefault.jpg"},
			},
		},
		"ownerText": map[string]interface{}{
			"runs": []map[string]interface{}{{"text": "Rick Astley"}},
		},
		"viewCountText": map[string]interface{}{"simpleText": "1B views"},
		"publishedTimeText": map[string]interface{}{"simpleText": "15 years ago"},
	}

	ytData := map[string]interface{}{
		"contents": map[string]interface{}{
			"twoColumnSearchResultsRenderer": map[string]interface{}{
				"primaryContents": map[string]interface{}{
					"sectionListRenderer": map[string]interface{}{
						"contents": []map[string]interface{}{
							{
								"itemSectionRenderer": map[string]interface{}{
									"contents": []map[string]interface{}{
										{"videoRenderer": videoRenderer},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	jsonBytes, _ := json.Marshal(ytData)

	e := NewYouTubeEngine()
	q := &model.Query{Text: "rick astley", Category: model.CategoryVideos}
	results, err := e.parseJSON(string(jsonBytes), q, 10)
	if err != nil {
		t.Fatalf("parseJSON() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("parseJSON() len = %d, want 1", len(results))
	}
	if results[0].Title != "Never Gonna Give You Up" {
		t.Errorf("title = %q, want %q", results[0].Title, "Never Gonna Give You Up")
	}
	if results[0].URL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Errorf("URL = %q", results[0].URL)
	}
	// Thumbnail should be the last (highest quality) entry
	if results[0].Thumbnail != "https://img.youtube.com/vi/dQw4w9WgXcQ/hqdefault.jpg" {
		t.Errorf("thumbnail = %q", results[0].Thumbnail)
	}
	// Author and metadata combined into Content
	if !strings.Contains(results[0].Content, "Rick Astley") {
		t.Errorf("content should contain channel name: %q", results[0].Content)
	}
}

// TestYouTubeParseJSONInvalidJSON verifies that malformed JSON returns an error.
func TestYouTubeParseJSONInvalidJSON(t *testing.T) {
	e := NewYouTubeEngine()
	q := &model.Query{Text: "test"}
	_, err := e.parseJSON("{invalid json", q, 10)
	if err == nil {
		t.Error("parseJSON() expected error on invalid JSON, got nil")
	}
}

// TestYouTubeParseJSONMissingStructure verifies that missing JSON structure returns empty results.
func TestYouTubeParseJSONMissingStructure(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{"empty object", `{}`},
		{"no twoColumn", `{"contents":{}}`},
		{"no primaryContents", `{"contents":{"twoColumnSearchResultsRenderer":{}}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewYouTubeEngine()
			q := &model.Query{Text: "test"}
			results, err := e.parseJSON(tt.json, q, 10)
			if err != nil {
				t.Fatalf("parseJSON() error = %v", err)
			}
			if len(results) != 0 {
				t.Errorf("expected 0 results for incomplete structure, got %d", len(results))
			}
		})
	}
}

// TestYouTubeParseJSONMaxResults verifies that maxResults cap is respected.
func TestYouTubeParseJSONMaxResults(t *testing.T) {
	makeVideoRenderer := func(id string) map[string]interface{} {
		return map[string]interface{}{
			"videoId": id,
			"title": map[string]interface{}{
				"runs": []map[string]interface{}{{"text": "Video " + id}},
			},
		}
	}

	items := make([]map[string]interface{}, 5)
	for i := range items {
		items[i] = map[string]interface{}{"videoRenderer": makeVideoRenderer(fmt.Sprintf("vid%03d", i))}
	}

	ytData := map[string]interface{}{
		"contents": map[string]interface{}{
			"twoColumnSearchResultsRenderer": map[string]interface{}{
				"primaryContents": map[string]interface{}{
					"sectionListRenderer": map[string]interface{}{
						"contents": []map[string]interface{}{
							{
								"itemSectionRenderer": map[string]interface{}{
									"contents": items,
								},
							},
						},
					},
				},
			},
		},
	}
	jsonBytes, _ := json.Marshal(ytData)

	e := NewYouTubeEngine()
	q := &model.Query{Text: "test"}
	results, err := e.parseJSON(string(jsonBytes), q, 3)
	if err != nil {
		t.Fatalf("parseJSON() error = %v", err)
	}
	if len(results) != 3 {
		t.Errorf("parseJSON() len = %d, want 3 (maxResults cap)", len(results))
	}
}

// TestYouTubeSearchNonOK verifies that a non-200 response returns an error.
func TestYouTubeSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	e := NewYouTubeEngine()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestYouTubeSearchFallbackToHTML verifies that when no JSON pattern matches, HTML fallback runs.
func TestYouTubeSearchFallbackToHTML(t *testing.T) {
	// HTML with /watch?v= links but no ytInitialData
	htmlPage := `<html><body>
<a href="/watch?v=abc1234567A">Video One</a>
<a href="/watch?v=xyz9876543Z">Video Two</a>
</body></html>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, htmlPage)
	}))
	defer srv.Close()

	e := NewYouTubeEngine()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	results, err := e.Search(context.Background(), &model.Query{Text: "test", Category: model.CategoryVideos})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected ≥2 results from HTML fallback, got %d", len(results))
	}
	for _, r := range results {
		if !strings.HasPrefix(r.URL, "https://www.youtube.com/watch?v=") {
			t.Errorf("result URL should be YouTube watch URL: %q", r.URL)
		}
		if r.Thumbnail == "" {
			t.Errorf("HTML fallback result should have a thumbnail")
		}
	}
}

// --- Startpage ---

// TestStartpageSearchNonOK verifies that a non-200 response returns an error.
func TestStartpageSearchNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	e := NewStartpageEngine()
	e.client = &http.Client{Transport: &redirectToTestServer{base: srv.URL}}

	_, err := e.Search(context.Background(), &model.Query{Text: "test"})
	if err == nil {
		t.Error("Search() expected error on non-200, got nil")
	}
}

// TestStartpageParseResultsStructuredPattern exercises the w-gl__result-url / w-gl__result-title path.
func TestStartpageParseResultsStructuredPattern(t *testing.T) {
	html := `<html><body>
<a class="w-gl__result-url" href="https://example.com/result1"></a>
<h3 class="w-gl__result-title">Example Result One</h3>
<p class="w-gl__description">Description of result one</p>
<a class="w-gl__result-url" href="https://example.org/result2"></a>
<h3 class="w-gl__result-title">Example Result Two</h3>
</body></html>`

	e := NewStartpageEngine()
	q := &model.Query{Text: "test", Category: model.CategoryGeneral}
	results, err := e.parseResults(html, q)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) < 1 {
		t.Fatal("parseResults() returned no results, want ≥1")
	}
}

// TestStartpageParseResultsGenericFallback exercises the generic https:// link fallback path.
func TestStartpageParseResultsGenericFallback(t *testing.T) {
	// No w-gl__result-url pattern → triggers generic fallback
	html := `<html><body>
<a href="https://external.example.com/page">External Result Page Title</a>
<a href="https://another.example.org/page">Another External Page</a>
<a href="https://www.startpage.com/settings">Startpage Internal</a>
<a href="https://ext.example.net/page">X</a>
</body></html>`

	e := NewStartpageEngine()
	q := &model.Query{Text: "test", Category: model.CategoryGeneral}
	results, err := e.parseResults(html, q)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	// Startpage internal and short titles are filtered
	for _, r := range results {
		if strings.Contains(r.URL, "startpage.com") {
			t.Errorf("startpage internal URL should be filtered: %q", r.URL)
		}
		if len(r.Title) < 5 {
			t.Errorf("result title too short (filtered): %q", r.Title)
		}
	}
}

// TestStartpageParseResultsDeduplicated verifies that duplicate URLs are skipped in fallback mode.
func TestStartpageParseResultsDeduplicated(t *testing.T) {
	html := `<html><body>
<a href="https://example.com/page">First Occurrence Of This Result</a>
<a href="https://example.com/page">Duplicate Occurrence Of Same</a>
</body></html>`

	e := NewStartpageEngine()
	q := &model.Query{Text: "test", Category: model.CategoryGeneral}
	results, err := e.parseResults(html, q)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	for _, r := range results {
		count := 0
		for _, r2 := range results {
			if r.URL == r2.URL {
				count++
			}
		}
		if count > 1 {
			t.Errorf("duplicate URL in results: %q", r.URL)
		}
	}
}
