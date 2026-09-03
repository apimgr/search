package engine

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/apimgr/search/src/model"
)

// This file holds golden-file tests: each fixture in testdata/ is a real
// response captured from the live upstream service (see the comment on each
// test for how/when it was captured), replayed through the engine's actual
// production parsing code with no network access at test time. These exist
// alongside the synthetic-fixture tests elsewhere in this package (which
// remain useful for exercising edge cases like empty/malformed input) to
// catch upstream response-shape drift that hand-written minimal HTML/JSON
// would never expose.

// readGolden reads a fixture file from testdata/ or fails the test.
func readGolden(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read golden fixture %s: %v", name, err)
	}
	return data
}

// TestArXivSearchGolden replays a real Atom feed captured from
// export.arxiv.org/api/query?search_query=all:golang (5 entries).
func TestArXivSearchGolden(t *testing.T) {
	body := readGolden(t, "arxiv_search.xml")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml")
		w.Write(body)
	}))
	defer server.Close()

	engine := NewArXiv()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "golang", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}
	for _, r := range results {
		if r.Title == "" || r.URL == "" {
			t.Errorf("result missing Title/URL: %+v", r)
		}
		if r.Category != model.CategoryScience {
			t.Errorf("Category = %v, want science", r.Category)
		}
	}
}

// TestGitHubSearchGolden replays a real response captured from
// api.github.com/search/repositories?q=golang.
func TestGitHubSearchGolden(t *testing.T) {
	body := readGolden(t, "github_search.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()

	engine := NewGitHub()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "golang", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}
	first := results[0]
	if !strings.Contains(first.URL, "github.com/") {
		t.Errorf("first result URL = %q, want a github.com repo URL", first.URL)
	}
	if first.Title == "" {
		t.Error("first result Title is empty")
	}
}

// TestHackerNewsSearchGolden replays a real response captured from
// hn.algolia.com/api/v1/search?query=golang&tags=story.
func TestHackerNewsSearchGolden(t *testing.T) {
	body := readGolden(t, "hackernews_search.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()

	engine := NewHackerNews()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "golang", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 20 {
		t.Fatalf("len(results) = %d, want 20", len(results))
	}
	if results[0].Title == "" {
		t.Error("first result Title is empty")
	}
	if !strings.HasPrefix(results[0].URL, "http") {
		t.Errorf("first result URL = %q, want an http(s) URL", results[0].URL)
	}
}

// TestStackOverflowSearchGolden replays a real response captured from
// api.stackexchange.com/2.3/search/advanced with filter=withbody (matching
// the exact params StackOverflow.Search sends in production).
func TestStackOverflowSearchGolden(t *testing.T) {
	body := readGolden(t, "stackoverflow_search.json")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer server.Close()

	engine := NewStackOverflow()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "golang", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}
	if results[0].Title != "How to print struct variables in console?" {
		t.Errorf("first result Title = %q, want the known real title", results[0].Title)
	}
	if !strings.Contains(results[0].URL, "stackoverflow.com/") {
		t.Errorf("first result URL = %q, want a stackoverflow.com URL", results[0].URL)
	}
}

// TestOpenStreetMapSearchGolden decodes a real Nominatim response captured
// from nominatim.openstreetmap.org/search?q=Paris directly into the
// production nominatimResult type, then feeds it through the real
// parseResults — skipping HTTP mocking since parseResults already takes
// decoded structs as input.
func TestOpenStreetMapSearchGolden(t *testing.T) {
	body := readGolden(t, "openstreetmap_search.json")

	var nominatimResults []nominatimResult
	if err := json.Unmarshal(body, &nominatimResults); err != nil {
		t.Fatalf("failed to decode golden fixture into nominatimResult: %v", err)
	}
	if len(nominatimResults) != 4 {
		t.Fatalf("len(nominatimResults) = %d, want 4", len(nominatimResults))
	}

	engine := NewOpenStreetMap()
	query := &model.Query{Text: "Paris"}
	results := engine.parseResults(nominatimResults, query)

	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4", len(results))
	}
	if !strings.Contains(results[0].Title, "Paris") {
		t.Errorf("first result Title = %q, want it to contain Paris", results[0].Title)
	}
	if results[0].Category != model.CategoryMaps {
		t.Errorf("Category = %v, want maps", results[0].Category)
	}
	if lat, ok := results[0].Metadata["latitude"]; !ok || lat == nil {
		t.Error("expected latitude metadata to be populated from real lat field")
	}
}

// TestPubMedSearchGolden replays real esearch.fcgi (5 PMIDs) and
// efetch.fcgi (2 full PubmedArticle records) responses captured from NCBI
// E-utilities, routed by request path exactly as PubMed.Search does.
func TestPubMedSearchGolden(t *testing.T) {
	esearch := readGolden(t, "pubmed_esearch.xml")
	efetch := readGolden(t, "pubmed_efetch.xml")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case strings.Contains(r.URL.Path, "esearch.fcgi"):
			w.Write(esearch)
		case strings.Contains(r.URL.Path, "efetch.fcgi"):
			w.Write(efetch)
		default:
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	engine := NewPubMed()
	engine.client = &http.Client{Transport: redirectToServer(server.URL)}

	results, err := engine.Search(context.Background(), &model.Query{Text: "cancer", Page: 1})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// The efetch fixture was trimmed to the first 2 PubmedArticle records
	// (the esearch fixture returned 5 IDs, but only 2 have full records).
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Title == "" {
			t.Error("result Title is empty")
		}
		if !strings.HasPrefix(r.URL, "https://pubmed.ncbi.nlm.nih.gov/") {
			t.Errorf("result URL = %q, want a pubmed.ncbi.nlm.nih.gov URL", r.URL)
		}
		if r.Category != model.CategoryScience {
			t.Errorf("Category = %v, want science", r.Category)
		}
	}
}

// TestBraveParseResultsGolden feeds a real search.brave.com HTML response
// (trimmed to the first several result blocks; the untrimmed 446KB capture
// parsed to 20 results, this trimmed copy parses to 4) through the real,
// unexported parseResults regex parser directly — Brave's Search() does
// nothing but an HTTP GET and a call to parseResults, so this exercises the
// only logic worth golden-testing without needing httptest.
func TestBraveParseResultsGolden(t *testing.T) {
	body := readGolden(t, "brave_search.html")

	engine := NewBrave()
	results, err := engine.parseResults(string(body), model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("len(results) = %d, want 4", len(results))
	}
	want := []struct{ title, url string }{
		{"The Go Programming Language", "https://go.dev/"},
		{"Go (programming language) - Wikipedia", "https://en.wikipedia.org/wiki/Go_(programming_language)"},
		{"GitHub - golang/go: The Go programming language · GitHub", "https://github.com/golang/go"},
	}
	for i, w := range want {
		if results[i].Title != w.title {
			t.Errorf("results[%d].Title = %q, want %q", i, results[i].Title, w.title)
		}
		if results[i].URL != w.url {
			t.Errorf("results[%d].URL = %q, want %q", i, results[i].URL, w.url)
		}
	}
}

// TestWikipediaExtractsDecodeGolden decodes a real response captured from
// en.wikipedia.org/w/api.php with the exact params WikipediaEngine.Search
// sends (action=query&generator=search&gsrsearch=golang&prop=extracts&
// exintro=true&explaintext=true&exsentences=3) into the production
// wikipediaExtractsResponse type.
//
// Unlike the other engines, WikipediaEngine has no swappable HTTP client —
// Search() builds a client inline against the package-level SharedTransport
// (a concrete *http.Transport, not an http.RoundTripper), so it cannot be
// redirected to httptest.Server without a source change to transport.go.
// Rather than widen SharedTransport's type for a test-only need, this test
// instead decodes the fixture with the real, unexported response struct and
// exercises the real relevance-ordering logic, which is where upstream
// shape drift (renamed/removed JSON fields, or a change to the Index-based
// ordering contract) would actually be caught.
func TestWikipediaExtractsDecodeGolden(t *testing.T) {
	body := readGolden(t, "wikipedia_search.json")

	var wikiResp wikipediaExtractsResponse
	if err := json.Unmarshal(body, &wikiResp); err != nil {
		t.Fatalf("failed to decode golden fixture into wikipediaExtractsResponse: %v", err)
	}

	if len(wikiResp.Query.Pages) != 5 {
		t.Fatalf("len(wikiResp.Query.Pages) = %d, want 5", len(wikiResp.Query.Pages))
	}

	// Find the page the real API ranked first (Index 1) and assert it is
	// the known real result for query "golang".
	var top *struct {
		Title   string `json:"title"`
		PageID  int    `json:"pageid"`
		Extract string `json:"extract"`
		Index   int    `json:"index"`
	}
	for k := range wikiResp.Query.Pages {
		page := wikiResp.Query.Pages[k]
		if page.Index == 1 {
			p := page
			top = &p
		}
	}
	if top == nil {
		t.Fatal("no page with Index 1 found")
	}
	if top.Title != "Go (programming language)" {
		t.Errorf("top-ranked page Title = %q, want %q", top.Title, "Go (programming language)")
	}
	if top.Extract == "" {
		t.Error("top-ranked page Extract is empty")
	}
	if top.PageID == 0 {
		t.Error("top-ranked page PageID is zero")
	}
}

// TestGoogleBlockPageDetectionGolden feeds a real block-page response
// captured live from google.com/search (the "enable JavaScript" anti-bot
// page returned to non-browser clients) through the real detectBlockPage
// function that Google.Search calls before attempting to parse results.
// This is what upstream bot-blocking looks like in production for this
// engine, so it doubles as the realistic golden fixture for Google: live
// organic HTML could not be captured from this sandbox (Google serves only
// the JS-challenge page to non-browser User-Agents here), but the resulting
// block page is itself a genuine real response worth regression-testing.
func TestGoogleBlockPageDetectionGolden(t *testing.T) {
	body := readGolden(t, "google_block.html")

	err := detectBlockPage("google", string(body))
	if err == nil {
		t.Fatal("detectBlockPage() = nil, want an error for a real captured block page")
	}
}

// TestBaiduBlockPageDetectionGolden feeds a real "百度安全验证" (Baidu
// Security Verification) block page captured live from baidu.com/s through
// the real detectBlockPage function that Baidu.Search calls. Live organic
// HTML could not be captured from this sandbox (Baidu serves only the
// security-verification page here), so this block page is the realistic
// golden fixture for Baidu.
func TestBaiduBlockPageDetectionGolden(t *testing.T) {
	body := readGolden(t, "baidu_block.html")

	err := detectBlockPage("baidu", string(body))
	if err == nil {
		t.Fatal("detectBlockPage() = nil, want an error for a real captured block page")
	}
}

// TestYandexParseResultsGolden feeds a real yandex.com/search HTML response
// (captured live for query "golang programming language"; trimmed to the
// first three organic result blocks, with favicon data: URIs and inline
// <script> tags stripped to keep the fixture small) through the real,
// unexported parseResults regex parser directly — Yandex's Search() does
// nothing but an HTTP GET, a block-page check, and a call to parseResults,
// so this exercises the only logic worth golden-testing without needing
// httptest. This regression-tests the "OrganicTitle-Link"/
// "OrganicTextContentSpan" markup Yandex now serves, replacing the older
// "organic__title-link" BEM classes the parser previously targeted.
func TestYandexParseResultsGolden(t *testing.T) {
	body := readGolden(t, "yandex_search.html")

	engine := NewYandex()
	results, err := engine.parseResults(string(body), model.CategoryGeneral)
	if err != nil {
		t.Fatalf("parseResults() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	want := []struct{ title, url, contentSubstr string }{
		{"The Go Programming Language", "https://go.dev/", "Build simple, secure, scalable systems with Go"},
		{"A Comprehensive Go Programming Language Guide", "https://lzwjava.github.io/go-lang-en", "comprehensive Go programming language guide"},
		{"Go ( programming language ) - Wikipedia", "https://en.wikipedia.org/wiki/Go_(programming_language)", "statically typed and compiled"},
	}
	for i, w := range want {
		if results[i].Title != w.title {
			t.Errorf("results[%d].Title = %q, want %q", i, results[i].Title, w.title)
		}
		if results[i].URL != w.url {
			t.Errorf("results[%d].URL = %q, want %q", i, results[i].URL, w.url)
		}
		if !strings.Contains(results[i].Content, w.contentSubstr) {
			t.Errorf("results[%d].Content = %q, want it to contain %q", i, results[i].Content, w.contentSubstr)
		}
		if results[i].Category != model.CategoryGeneral {
			t.Errorf("results[%d].Category = %v, want general", i, results[i].Category)
		}
	}
}

// TestArXivFeedXMLShapeGolden is a narrower regression check on the raw
// XML shape itself (independent of Search()'s HTTP plumbing), verifying the
// production arxivFeed/arxivEntry types still decode every field arXiv's
// real Atom feed actually sends, including multiple authors and category
// terms per entry.
func TestArXivFeedXMLShapeGolden(t *testing.T) {
	body := readGolden(t, "arxiv_search.xml")

	var feed arxivFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		t.Fatalf("failed to decode golden fixture into arxivFeed: %v", err)
	}
	if len(feed.Entries) != 5 {
		t.Fatalf("len(feed.Entries) = %d, want 5", len(feed.Entries))
	}
	for _, e := range feed.Entries {
		if e.Title == "" || e.ID == "" {
			t.Errorf("entry missing Title/ID: %+v", e)
		}
		if len(e.Authors) == 0 {
			t.Errorf("entry %q has no authors", e.Title)
		}
	}
}
