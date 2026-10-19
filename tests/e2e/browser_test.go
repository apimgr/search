//go:build e2e

// Tier 3 — full browser, JavaScript enabled. Per AI.md PART 28, this tier
// must pass with zero console errors and zero failed asset/XHR requests on
// every page visited; a feature that only works here and not in nojs_test.go
// is a PART 14 progressive-enhancement violation, not a passing test.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// TestBrowserHomeSearchFlow drives the real JS-enhanced search box end to
// end: type a query, submit, and assert the results page that comes back
// carries the query the user actually typed with no console errors along
// the way.
func TestBrowserHomeSearchFlow(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	var pageURL, html string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#searchQuery`, chromedp.ByQuery),
		chromedp.SendKeys(`#searchQuery`, "golang"),
		chromedp.Click(`.search-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}

	if !strings.Contains(pageURL, "q=golang") {
		t.Errorf("search flow landed on %q, want q=golang in the URL", pageURL)
	}
	requireNoFailures(t, pf, "/ -> /search")
}

func TestBrowserRelatedSearchSuggestions(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	failures := watchForFailures(ctx)
	query := "browser related fixture"
	var body string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/api/v1/search/related?q="+url.QueryEscape(query)),
		chromedp.Text("body", &body, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load related-search response: %v", err)
	}
	assertRelatedSuggestions(t, body, query)
	requireNoFailures(t, failures, "related-search API response")
}

func TestBrowserAutocompleteSuggestions(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	failures := watchForFailures(ctx)
	query := "browser autocomplete"
	var body string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/autocomplete?q="+url.QueryEscape(query)),
		chromedp.Text("body", &body, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("load autocomplete response: %v", err)
	}
	assertAutocompleteSuggestions(t, body, query)
	requireNoFailures(t, failures, "autocomplete API response")
}

func TestBrowserSearchBarWidgetGETForm(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var pageURL string
	form := fmt.Sprintf(`(() => {
		const widget = document.createElement('form');
		widget.action = %q + '/search';
		widget.method = 'get';
		widget.setAttribute('role', 'search');
		widget.innerHTML = '<label for="widget-query">Search</label><input id="widget-query" type="search" name="q" required><button type="submit">Search</button>';
		document.body.appendChild(widget);
		widget.querySelector('input').value = 'widget integration';
		widget.requestSubmit();
	})()`, site.URL)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.Evaluate(form, nil),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
	); err != nil {
		t.Fatalf("Search Bar widget form submission: %v", err)
	}
	if !strings.Contains(pageURL, "/search?q=widget+integration") {
		t.Fatalf("Search Bar widget landed on %q, want its native GET query", pageURL)
	}
	requireNoFailures(t, pf, "Search Bar widget GET form")
}

func TestBrowserKeyboardSearchAndHelpShortcuts(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var searchFocused, searchCleared, helpOpened, helpClosed bool
	var searchAfterEscape, pageURL string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.Evaluate(`document.dispatchEvent(new KeyboardEvent('keydown', {key: '/', bubbles: true}))`, nil),
		chromedp.Evaluate(`document.activeElement === document.querySelector('#searchQuery')`, &searchFocused),
		chromedp.SetValue(`#searchQuery`, "clear this search", chromedp.ByQuery),
		chromedp.Evaluate(`document.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}))`, nil),
		chromedp.Evaluate(`JSON.stringify({value: document.querySelector('#searchQuery').value, blurred: document.activeElement !== document.querySelector('#searchQuery')})`, &searchAfterEscape),
		chromedp.Evaluate(`document.dispatchEvent(new KeyboardEvent('keydown', {key: 's', bubbles: true}))`, nil),
		chromedp.WaitVisible(`.preferences-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
		chromedp.Navigate(site.URL+"/"),
		chromedp.Evaluate(`document.activeElement.blur(); document.dispatchEvent(new KeyboardEvent('keydown', {key: '?', bubbles: true}))`, nil),
		chromedp.WaitVisible(`#keyboard-help-modal`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#keyboard-help-modal')?.open === true`, &helpOpened),
		chromedp.Evaluate(`document.querySelector('#keyboard-help-modal')?.dispatchEvent(new KeyboardEvent('keydown', {key: 'Escape', bubbles: true}))`, nil),
		chromedp.WaitNotPresent(`#keyboard-help-modal`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#keyboard-help-modal') === null`, &helpClosed),
	); err != nil {
		t.Fatalf("keyboard shortcut flow: %v", err)
	}
	searchCleared = strings.Contains(searchAfterEscape, `"value":""`) && strings.Contains(searchAfterEscape, `"blurred":true`)
	if !searchFocused || !searchCleared || !strings.Contains(pageURL, "/server/preferences") || !helpOpened || !helpClosed {
		t.Fatalf("keyboard shortcut results: focused=%t searchCleared=%t preferencesURL=%q helpOpened=%t helpClosed=%t", searchFocused, searchCleared, pageURL, helpOpened, helpClosed)
	}
	requireNoFailures(t, pf, "keyboard search and help shortcuts")
}

func TestBrowserKeyboardResultNavigationShortcuts(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var stateJSON string
	setup := `
		window.__keyboardClicks = [];
		window.__keyboardOpened = [];
		window.open = function(href, target, features) {
			window.__keyboardOpened.push({href: href, target: target, features: features});
		};
		document.body.insertAdjacentHTML('beforeend', ` + "`" + `
			<div class="search-result" data-keyboard-result="one"><h3>One</h3><a href="/keyboard/one">One</a></div>
			<div class="search-result" data-keyboard-result="two"><h3>Two</h3><a href="/keyboard/two">Two</a></div>
			<div class="search-result" data-keyboard-result="three"><h3>Three</h3><a href="/keyboard/three">Three</a></div>
			<a class="pagination-prev" href="/keyboard/prev" data-keyboard-page="prev">Previous</a>
			<a class="pagination-next" href="/keyboard/next" data-keyboard-page="next">Next</a>
		` + "`" + `);
		document.querySelectorAll('[data-keyboard-result] a, [data-keyboard-page]').forEach(function(link) {
			link.addEventListener('click', function(event) {
				event.preventDefault();
				var result = link.closest('[data-keyboard-result]');
				window.__keyboardClicks.push((result && result.dataset.keyboardResult) || link.dataset.keyboardPage);
			});
		});
		window.__dispatchKeyboard = function(key, shiftKey) {
			document.dispatchEvent(new KeyboardEvent('keydown', {key: key, shiftKey: !!shiftKey, bubbles: true}));
		};
	`
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/search?q=keyboard"),
		chromedp.Evaluate(setup, nil),
		chromedp.Evaluate(`__dispatchKeyboard('j'); __dispatchKeyboard('j'); __dispatchKeyboard('k'); __dispatchKeyboard('g'); __dispatchKeyboard('g'); __dispatchKeyboard('G'); __dispatchKeyboard('k')`, nil),
		chromedp.Evaluate(`JSON.stringify({selected: Array.from(document.querySelectorAll('.search-result')).map(function(node) { return node.classList.contains('keyboard-selected'); }), clicks: window.__keyboardClicks.slice(), opened: window.__keyboardOpened.slice()})`, &stateJSON),
	); err != nil {
		t.Fatalf("keyboard result navigation: %v", err)
	}
	var state struct {
		Selected []bool   `json:"selected"`
		Clicks   []string `json:"clicks"`
		Theme    string   `json:"theme"`
		Opened   []struct {
			Href     string `json:"href"`
			Target   string `json:"target"`
			Features string `json:"features"`
		} `json:"opened"`
	}
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		t.Fatalf("decode keyboard navigation state %q: %v", stateJSON, err)
	}
	if len(state.Selected) != 3 || state.Selected[1] != true || state.Selected[0] || state.Selected[2] {
		t.Fatalf("j/k, gg, G did not leave the second result selected: %s", stateJSON)
	}
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`__dispatchKeyboard('Enter'); __dispatchKeyboard('o'); __dispatchKeyboard('O'); __dispatchKeyboard('1'); __dispatchKeyboard('2', true); __dispatchKeyboard('h'); __dispatchKeyboard('l'); __dispatchKeyboard('t')`, nil),
		chromedp.Evaluate(`JSON.stringify({clicks: window.__keyboardClicks, opened: window.__keyboardOpened, theme: document.documentElement.getAttribute('data-theme-mode')})`, &stateJSON),
	); err != nil {
		t.Fatalf("keyboard result actions: %v", err)
	}
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		t.Fatalf("decode keyboard action state %q: %v", stateJSON, err)
	}
	wantClicks := []string{"two", "two", "one", "prev", "next"}
	if strings.Join(state.Clicks, ",") != strings.Join(wantClicks, ",") {
		t.Errorf("Enter/o/quick jump/pagination clicks = %v, want %v", state.Clicks, wantClicks)
	}
	if len(state.Opened) != 2 {
		t.Errorf("O and shifted quick jump should each open a new tab, got %+v", state.Opened)
	} else {
		for _, opened := range state.Opened {
			if opened.Href != site.URL+"/keyboard/two" || opened.Target != "_blank" || !strings.Contains(opened.Features, "noopener") {
				t.Errorf("new-tab shortcut should safely open the selected result, got %+v", opened)
			}
		}
	}
	if state.Theme != "light" {
		t.Errorf("t should toggle the default dark theme to light, got %q", state.Theme)
	}
	requireNoFailures(t, pf, "keyboard result navigation shortcuts")
}

func TestBrowserCustomBangCanBeCreatedSuggestedAndUsed(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var pageURL string
	var bangCount, invalidBangCount int
	var urlValid bool
	var autocompleteText, saveStatus string
	runStage := func(name string, actions ...chromedp.Action) {
		t.Helper()
		if err := chromedp.Run(ctx, actions...); err != nil {
			t.Fatalf("custom bang %s: %v", name, err)
		}
	}
	runStage("create preferences",
		chromedp.Navigate(site.URL+"/server/preferences"),
	)
	runStage("store custom shortcut",
		chromedp.SetValue(`#bang-shortcut`, "fixture", chromedp.ByQuery),
		chromedp.SetValue(`#bang-name`, "Fixture Search", chromedp.ByQuery),
		chromedp.SetValue(`#bang-url`, site.URL+"/search?q={query}", chromedp.ByQuery),
		chromedp.SetValue(`#bang-category`, "testing", chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#bang-url').validity.valid`, &urlValid),
		chromedp.Click(`#add-custom-bang button[type=submit]`, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('search_custom_bangs') || '[]').length`, &bangCount),
		chromedp.Evaluate(`document.querySelector('#save-status')?.textContent || ''`, &saveStatus),
	)
	if bangCount != 1 || !urlValid {
		t.Fatalf("custom bang was not stored: count=%d valid=%t status=%q", bangCount, urlValid, saveStatus)
	}
	runStage("reject unsafe custom target",
		chromedp.SetValue(`#bang-shortcut`, "unsafe", chromedp.ByQuery),
		chromedp.SetValue(`#bang-name`, "Unsafe Search", chromedp.ByQuery),
		chromedp.SetValue(`#bang-url`, "javascript:alert(1)", chromedp.ByQuery),
		chromedp.Click(`#add-custom-bang button[type=submit]`, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.parse(localStorage.getItem('search_custom_bangs') || '[]').length`, &invalidBangCount),
	)
	if invalidBangCount != 1 {
		t.Fatalf("unsafe custom bang target was stored or valid bang was lost: count=%d", invalidBangCount)
	}
	runStage("show autocomplete",
		chromedp.Navigate(site.URL+"/"),
		chromedp.SendKeys(`#searchQuery`, "!", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`document.querySelector('.autocomplete-bang')?.innerText || ''`, &autocompleteText),
	)
	if !strings.Contains(autocompleteText, "!fixture") || !strings.Contains(autocompleteText, "Fixture Search") {
		t.Fatalf("custom bang suggestion for ! = %q", autocompleteText)
	}
	runStage("filter autocomplete by shortcut",
		chromedp.SendKeys(`#searchQuery`, "fixture", chromedp.ByQuery),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(`document.querySelector('.autocomplete-bang')?.innerText || ''`, &autocompleteText),
	)
	if !strings.Contains(autocompleteText, "!fixture") || !strings.Contains(autocompleteText, "Fixture Search") {
		t.Fatalf("custom bang shortcut autocomplete suggestion = %q", autocompleteText)
	}
	runStage("submit custom shortcut",
		chromedp.Click(`.autocomplete-bang`, chromedp.ByQuery),
		chromedp.SendKeys(`#searchQuery`, "cats & dogs", chromedp.ByQuery),
		chromedp.Click(`.search-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
	)
	parsed, err := url.Parse(pageURL)
	if err != nil {
		t.Fatalf("parse custom bang destination %q: %v", pageURL, err)
	}
	if parsed.Path != "/search" || parsed.Query().Get("q") != "cats & dogs" {
		t.Fatalf("custom bang destination=%q, want local search for cats & dogs", pageURL)
	}
	runStage("submit suffix custom shortcut",
		chromedp.Navigate(site.URL+"/"),
		chromedp.SetValue(`#searchQuery`, "cats & dogs !fixture", chromedp.ByQuery),
		chromedp.Click(`.search-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
	)
	parsed, err = url.Parse(pageURL)
	if err != nil {
		t.Fatalf("parse suffix custom bang destination %q: %v", pageURL, err)
	}
	if parsed.Path != "/search" || parsed.Query().Get("q") != "cats & dogs" {
		t.Fatalf("suffix custom bang destination=%q, want local search for cats & dogs", pageURL)
	}
	requireNoFailures(t, pf, "custom bang create, autocomplete, and redirect")
}

func TestBrowserAdvancedSearchSubmitsFilters(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var pageURL string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.Click(`#advancedSearchBtn`, chromedp.ByQuery),
		chromedp.WaitVisible(`#as-allwords`, chromedp.ByQuery),
		chromedp.SetValue(`#as-allwords`, "privacy search"),
		chromedp.SetValue(`#as-exactphrase`, "private results"),
		chromedp.SetValue(`#as-anywords`, "engine source"),
		chromedp.SetValue(`#as-nonewords`, "tracking ads"),
		chromedp.SetValue(`#as-site`, "example.org"),
		chromedp.SetValue(`#as-filetype`, "pdf"),
		chromedp.SetValue(`#as-time`, "week"),
		chromedp.SetValue(`#as-lang`, "de"),
		chromedp.SetValue(`#as-region`, "de"),
		chromedp.Click(`#advancedSearchSubmit`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Location(&pageURL),
	); err != nil {
		t.Fatalf("submit advanced search: %v", err)
	}
	parsed, err := url.Parse(pageURL)
	if err != nil {
		t.Fatalf("parse advanced search URL: %v", err)
	}
	params := parsed.Query()
	for name, want := range map[string]string{
		"q":          `privacy search "private results" (engine OR source) -tracking -ads site:example.org filetype:pdf`,
		"time_range": "week",
		"lang":       "de",
		"region":     "de",
	} {
		if got := params.Get(name); got != want {
			t.Errorf("advanced search parameter %s = %q, want %q", name, got, want)
		}
	}
	requireNoFailures(t, pf, "advanced search form")
}

func TestBrowserCrossDevicePreferenceSync(t *testing.T) {
	site := newTestSite(t)
	source := newBrowserContext(t)
	if err := chromedp.Run(source,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := grantPreferenceConsent(ctx, site.URL); err != nil {
				return err
			}
			for _, cookie := range []struct{ name, value string }{
				{"theme", "light"},
				{"lang", "de"},
				{"search_pref_category", "images"},
				{"search_pref_infinite_scroll", "0"},
				{"search_pref_font_size", "large"},
				{"search_pref_results_density", "compact"},
				{"search_pref_accent_color", "#456def"},
			} {
				if err := network.SetCookie(cookie.name, cookie.value).WithURL(site.URL).WithPath("/").Do(ctx); err != nil {
					return err
				}
			}
			return nil
		}),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.Click(`#copy-sync-prefs`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#sync-url').value.length > 0`, nil, chromedp.WithPollingInterval(25*time.Millisecond), chromedp.WithPollingTimeout(5*time.Second)),
	); err != nil {
		t.Fatalf("export preferences from source device: %v", err)
	}
	var syncURL string
	if err := chromedp.Run(source, chromedp.Value(`#sync-url`, &syncURL, chromedp.ByQuery)); err != nil {
		t.Fatalf("read exported preference URL: %v", err)
	}
	if syncURL == "" {
		t.Fatal("copy preferences did not populate the export URL")
	}

	receiver := newBrowserContext(t)
	pf := watchForFailures(receiver)
	var homepage string
	var infiniteScrollEnabled bool
	if err := chromedp.Run(receiver,
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.SetValue(`#sync-paste`, syncURL, chromedp.ByQuery),
		chromedp.Click(`#apply-sync-prefs`, chromedp.ByQuery),
		chromedp.WaitVisible(`#searchQuery`, chromedp.ByQuery),
		chromedp.OuterHTML("html", &homepage),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.Evaluate(`document.querySelector('#infinite-scroll').checked`, &infiniteScrollEnabled),
	); err != nil {
		t.Fatalf("import preferences on receiver device: %v", err)
	}
	if !strings.Contains(homepage, "theme-light") || !strings.Contains(homepage, `<html lang="de"`) {
		t.Errorf("receiver did not apply imported theme and language: %s", snippet(homepage, `<html`))
	}
	for _, marker := range []string{`font-size-large`, `results-density-compact`, `--color-accent: #456def`} {
		if !strings.Contains(homepage, marker) {
			t.Errorf("receiver did not apply imported visual preference %q: %s", marker, snippet(homepage, `<html`))
		}
	}
	if infiniteScrollEnabled {
		t.Error("receiver did not apply disabled infinite-scroll preference")
	}
	requireNoFailures(t, pf, "cross-device preference import")
}

func TestBrowserInfiniteScrollPreservesSearchFilters(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	requests := make(chan url.Values, 1)
	document := fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Search fixture</title></head><body><div class="search-results-page" data-query="filter fixture" data-category="general" data-page="1" data-per-page="1" data-safe-search="1" data-lang="de" data-region="fr" data-time-range="week" data-time-from="2025-01-01" data-time-to="2025-01-31"></div><div class="results-list" id="results-container"></div><div id="scroll-trigger"></div><div id="loading-indicator" class="hidden"></div><div id="end-indicator" class="hidden"></div><script src="%s/static/js/app.js"></script></body></html>`, site.URL)
	fixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintln(writer, document)
		case request.URL.Path == "/api/v1/search":
			select {
			case requests <- request.URL.Query():
			default:
			}
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = fmt.Fprintln(writer, `{"ok":true,"data":{"results":[{"title":"Loaded fixture result","url":"https://fixture.example/page-two","description":"Fixture result loaded by infinite scroll","engine":"fixture"}],"pagination":{"page":2,"limit":1,"total":2,"pages":2}}}`)
		case strings.HasPrefix(request.URL.Path, "/locales/"):
			writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = fmt.Fprintln(writer, `{}`)
		case request.URL.Path == "/api/v1/favicon":
			writer.Header().Set("Content-Type", "image/svg+xml")
			_, _ = fmt.Fprintln(writer, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1 1"><circle cx="0.5" cy="0.5" r="0.5"/></svg>`)
		default:
			http.NotFound(writer, request)
		}
	}))
	t.Cleanup(fixture.Close)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(fixture.URL),
		chromedp.WaitVisible(`#scroll-trigger`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#scroll-trigger').scrollIntoView()`, nil),
		chromedp.WaitVisible(`.result-item`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("trigger infinite-scroll request: %v", err)
	}
	var params url.Values
	select {
	case params = <-requests:
	case <-time.After(5 * time.Second):
		t.Fatal("infinite scroll did not request the next result page")
	}
	for name, want := range map[string]string{
		"lang":       "de",
		"region":     "fr",
		"time_range": "week",
		"time_from":  "2025-01-01",
		"time_to":    "2025-01-31",
	} {
		if got := params.Get(name); got != want {
			t.Errorf("infinite-scroll parameter %s = %q, want %q", name, got, want)
		}
	}
	requireNoFailures(t, pf, "infinite-scroll filter preservation")
}

func TestBrowserInfiniteScrollVideoResultsRetainPreviewMetadata(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	document := fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Video search fixture</title></head><body><div class="search-results-page" data-query="video fixture" data-category="videos" data-page="1" data-per-page="1" data-safe-search="1"></div><div class="results-list" id="results-container"></div><div id="scroll-trigger"></div><div id="loading-indicator" class="hidden"></div><div id="end-indicator" class="hidden"></div><script src="%s/static/js/app.js"></script></body></html>`, site.URL)
	fixture := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/" {
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = fmt.Fprintln(writer, document)
			return
		}
		if request.URL.Path != "/api/v1/search" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = fmt.Fprintln(writer, `{"ok":true,"data":{"results":[{"title":"Loaded fixture video","url":"https://youtube.com/watch?v=abc123XYZ_9","description":"Video loaded by infinite scroll","engine":"fixture","duration":120,"embed_url":"https://www.youtube-nocookie.com/embed/abc123XYZ_9"}],"pagination":{"page":2,"limit":1,"total":2,"pages":2}}}`)
	}))
	t.Cleanup(fixture.Close)
	var embedURL, duration string
	steps := []struct {
		name   string
		action chromedp.Action
	}{
		{name: "navigate to video fixture", action: chromedp.Navigate(fixture.URL)},
		{name: "trigger infinite scroll", action: chromedp.Evaluate(`document.querySelector('#scroll-trigger').scrollIntoView()`, nil)},
		{name: "wait for paginated video card", action: chromedp.WaitVisible(`.video-result[data-video-embed]`, chromedp.ByQuery)},
		{name: "read video embed", action: chromedp.Evaluate(`document.querySelector('.video-result').dataset.videoEmbed`, &embedURL)},
		{name: "read video duration", action: chromedp.Evaluate(`document.querySelector('.video-result').dataset.videoDuration`, &duration)},
	}
	for _, step := range steps {
		if err := chromedp.Run(ctx, step.action); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	if embedURL != "https://www.youtube-nocookie.com/embed/abc123XYZ_9" || duration != "120" {
		t.Errorf("paginated video metadata embed=%q duration=%q", embedURL, duration)
	}
	requireNoFailures(t, pf, "infinite-scroll video metadata")
}

func TestBrowserSearchHistoryIsLocalAndClearable(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var stored, query string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.SendKeys(`#searchQuery`, "local history example"),
		chromedp.Click(`.search-btn`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Evaluate(`localStorage.getItem('search_history')`, &stored),
	); err != nil {
		t.Fatalf("record browser-local search history: %v", err)
	}
	if !strings.Contains(stored, `"query":"local history example"`) {
		t.Fatalf("localStorage search_history = %q, want the submitted query", stored)
	}

	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#search-history-list a`, chromedp.ByQuery),
		chromedp.Text(`#search-history-list a`, &query, chromedp.ByQuery),
		chromedp.Click(`#search-history-list a`, chromedp.ByQuery),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open a recent search: %v", err)
	}
	if query != "local history example" {
		t.Fatalf("recent search link text = %q", query)
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#clear-search-history`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector("#clear-search-history").addEventListener("click", () => { window.__clearClicked = true; })`, nil),
		chromedp.Click(`#clear-search-history`, chromedp.ByQuery),
		chromedp.WaitNotPresent(`#search-history-list a`, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.stringify({value:localStorage.getItem('search_history'), clicked:window.__clearClicked})`, &stored),
	); err != nil {
		t.Fatalf("clear browser-local search history: %v", err)
	}
	if stored != `{"value":null,"clicked":true}` {
		t.Fatalf("search_history after clear = %q, want null", stored)
	}
	requireNoFailures(t, pf, "local search history create, read, and clear")
}

func TestBrowserAlertLifecycle(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var managePath, value, pageURL string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/alerts/new?q=browser%20alert%20original&category=general"),
		chromedp.WaitVisible("form.alert-form", chromedp.ByQuery),
		chromedp.SetValue("#email", "alerts@example.test", chromedp.ByQuery),
		chromedp.Click("form.alert-form button[type=submit]", chromedp.ByQuery),
		chromedp.WaitVisible("a[href*='/alerts/manage/']", chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector("a[href*='/alerts/manage/']").getAttribute('href')`, &managePath),
	); err != nil {
		t.Fatalf("create accountless alert in browser: %v", err)
	}
	if managePath == "" {
		t.Fatal("created alert page did not expose a manage link")
	}
	manageURL, err := url.Parse(managePath)
	if err != nil {
		t.Fatalf("parse alert manage link: %v", err)
	}
	if manageURL.IsAbs() {
		managePath = manageURL.RequestURI()
	}

	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+managePath),
		chromedp.WaitVisible(".alert-page form.alert-form", chromedp.ByQuery),
		chromedp.Value("#query", &value, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open alert manage page: %v", err)
	}
	if value != "browser alert original" {
		t.Fatalf("managed alert query = %q, want original query", value)
	}

	if err := chromedp.Run(ctx,
		chromedp.SetValue("#query", "browser alert updated", chromedp.ByQuery),
		chromedp.SetValue("#frequency", "weekly", chromedp.ByQuery),
		chromedp.Click(".alert-page form.alert-form button[type=submit]", chromedp.ByQuery),
		chromedp.WaitVisible(".search-info", chromedp.ByQuery),
		chromedp.Value("#query", &value, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("update alert in browser: %v", err)
	}
	if value != "browser alert updated" {
		t.Fatalf("updated alert query = %q", value)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click("form.alert-inline-form[action$='/pause'] button", chromedp.ByQuery),
		chromedp.WaitVisible(".search-info", chromedp.ByQuery),
		chromedp.Value("form.alert-inline-form[action$='/pause'] input[name=paused]", &value, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("pause alert in browser: %v", err)
	}
	if value != "0" {
		t.Fatalf("paused alert next-action value = %q, want resume value 0", value)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click("form.alert-inline-form[action$='/pause'] button", chromedp.ByQuery),
		chromedp.WaitVisible(".search-info", chromedp.ByQuery),
		chromedp.Value("form.alert-inline-form[action$='/pause'] input[name=paused]", &value, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("resume alert in browser: %v", err)
	}
	if value != "1" {
		t.Fatalf("resumed alert next-action value = %q, want pause value 1", value)
	}

	if err := chromedp.Run(ctx,
		chromedp.Click("form.alert-inline-form[action$='/delete'] button", chromedp.ByQuery),
		chromedp.WaitVisible("form.alert-form", chromedp.ByQuery),
		chromedp.Location(&pageURL),
	); err != nil {
		t.Fatalf("delete alert in browser: %v", err)
	}
	if !strings.Contains(pageURL, "/alerts/new") {
		t.Fatalf("deleted alert redirected to %q, want alert creation page", pageURL)
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+managePath),
		chromedp.WaitVisible(".error-page", chromedp.ByQuery),
		chromedp.Text(".error-code", &value, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("verify deleted alert token is revoked: %v", err)
	}
	if value != "404" {
		t.Fatalf("deleted alert manage link returned status %q, want 404", value)
	}
	requireNoFailures(t, pf, "alert create, update, pause, resume, delete, and revoked manage link")
}

func TestBrowserOwnerLinkedTokenManagement(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var primary, linked, rotated, status string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/alerts/new?q=browser%20owner%20fixture&category=general"),
		chromedp.SetValue("#email", "alerts@example.test", chromedp.ByQuery),
		chromedp.Click("form.alert-form button[type=submit]", chromedp.ByQuery),
		chromedp.WaitVisible(".owner-token-issued .token-value", chromedp.ByQuery),
		chromedp.Text(".owner-token-issued .token-value", &primary, chromedp.ByQuery),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.SetValue(`form[action="/server/preferences/tokens"] input[name=name]`, "browser-linked", chromedp.ByQuery),
		chromedp.SetValue(`form[action="/server/preferences/tokens"] select[name=expires_at]`, "7d", chromedp.ByQuery),
		chromedp.Click(`form[action="/server/preferences/tokens"] button[type=submit]`, chromedp.ByQuery),
		chromedp.WaitVisible(".token-reveal .token-value", chromedp.ByQuery),
		chromedp.Text(".token-reveal .token-value", &linked, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("create owner and linked token in browser: %v", err)
	}
	if primary == "" || linked == "" || primary == linked {
		t.Fatal("owner and linked credentials were not issued distinctly")
	}
	var linkedName string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.SetValue(`.owner-token-item form[action$="/update"] input[name=name]`, "browser-renamed", chromedp.ByQuery),
		chromedp.SetValue(`.owner-token-item form[action$="/update"] select[name=expires_at]`, "30d", chromedp.ByQuery),
		chromedp.Click(`.owner-token-item form[action$="/update"] button[type=submit]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.owner-token-item h4`, chromedp.ByQuery),
		chromedp.Text(`.owner-token-item h4`, &linkedName, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("update linked token in browser: %v", err)
	}
	if linkedName != "browser-renamed" {
		t.Fatalf("linked-token name = %q after update", linkedName)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`.owner-token-item form[action$="/reveal"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(".token-reveal .token-value", chromedp.ByQuery),
		chromedp.Text(".token-reveal .token-value", &status, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("reveal linked token in browser: %v", err)
	}
	if status != linked {
		t.Fatal("revealed linked token differs from issued token")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.Click(`.owner-token-item form[action$="/rotate"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(".token-reveal .token-value", chromedp.ByQuery),
		chromedp.Text(".token-reveal .token-value", &rotated, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("rotate linked token in browser: %v", err)
	}
	if rotated == "" || rotated == linked {
		t.Fatal("linked token rotation did not replace the credential")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.Evaluate(`window.confirm = () => true`, nil),
		chromedp.Click(`.owner-token-item form[action$="/revoke"] button`, chromedp.ByQuery),
		chromedp.WaitVisible(`#owner-tokens-heading`, chromedp.ByQuery),
		chromedp.Text(`.owner-token-item p`, &status, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("revoke linked token in browser: %v", err)
	}
	if !strings.Contains(strings.ToLower(status), "revoked") {
		t.Fatalf("linked token status after revoke = %q", status)
	}
	requireNoFailures(t, pf, "owner-token create, update, reveal, rotate, revoke")
}

func TestBrowserImagePreviewAndDownloadActions(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var state string
	var open bool
	fixture := `<section class="image-result" data-full-url="https://source.example/photo" data-preview-url="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==" data-image-title="Fixture image" data-image-width="800" data-image-height="600" data-image-format="gif"><a href="https://source.example/photo" data-image-preview><img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///ywAAAAAAQABAAACAUwAOw==" alt="Fixture image"></a></section><dialog id="image-preview-dialog" aria-labelledby="image-preview-title"><h2 data-image-preview-title></h2><form method="dialog"><button type="submit">Close</button></form><figure><img data-image-preview-image alt=""><figcaption data-image-preview-dimensions></figcaption></figure><a href="#" data-image-preview-source>Visit source page</a><a href="#" download data-image-preview-download>Download image</a></dialog>`
	fixtureJSON, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("encode image-preview fixture: %v", err)
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#searchQuery`, chromedp.ByQuery),
		chromedp.Evaluate(`document.body.insertAdjacentHTML('beforeend', `+string(fixtureJSON)+`)`, nil),
		chromedp.Click(`.image-result [data-image-preview] img`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('#image-preview-dialog').open`, &open, chromedp.WithPollingInterval(20*time.Millisecond), chromedp.WithPollingTimeout(5*time.Second)),
		chromedp.Evaluate(`JSON.stringify({title:document.querySelector('[data-image-preview-title]').textContent,alt:document.querySelector('[data-image-preview-image]').alt,src:document.querySelector('[data-image-preview-image]').src,dimensions:document.querySelector('[data-image-preview-dimensions]').textContent,source:document.querySelector('[data-image-preview-source]').href,download:document.querySelector('[data-image-preview-download]').download})`, &state),
	); err != nil {
		t.Fatalf("open image preview: %v", err)
	}
	for _, expected := range []string{`"title":"Fixture image"`, `"alt":"Fixture image"`, `"dimensions":"Image dimensions: 800 × 600 px"`, `"source":"https://source.example/photo"`, `"download":"Fixture image.gif"`} {
		if !strings.Contains(state, expected) {
			t.Errorf("preview state %s does not contain %s", state, expected)
		}
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`#image-preview-dialog button[type=submit]`, chromedp.ByQuery),
		chromedp.Poll(`!document.querySelector('#image-preview-dialog').open`, &open, chromedp.WithPollingInterval(20*time.Millisecond), chromedp.WithPollingTimeout(5*time.Second)),
	); err != nil {
		t.Fatalf("close image preview: %v", err)
	}
	requireNoFailures(t, pf, "image preview, metadata, source link, download, and close")
}

// TestBrowserThemeToggleChangesComputedStyle exercises PART 16's theme rule
// that switching themes must actually change computed styles, not just an
// attribute nobody reads. It drives the JS toggle button (the enhancement
// nojs_test.go's form-based path deliberately does not exercise) and reads
// back the background color the browser actually painted.
func TestBrowserThemeToggleChangesComputedStyle(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	var before, after string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`.theme-toggle`, chromedp.ByQuery),
		chromedp.Evaluate(`getComputedStyle(document.body).backgroundColor`, &before),
		chromedp.Click(`.theme-toggle`, chromedp.ByQuery),
		// Give the click handler's DOM mutation one tick to flush before reading.
		chromedp.Sleep(0),
		chromedp.Evaluate(`getComputedStyle(document.body).backgroundColor`, &after),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}

	if before == after {
		t.Errorf("theme toggle click did not change body background-color (still %q)", before)
	}
	requireNoFailures(t, pf, "/ (theme toggle)")
}

func TestBrowserAutoThemeTracksSystemColorScheme(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	var light, dark, updatedLight string
	if err := chromedp.Run(ctx,
		emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "light"}}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie("theme", "auto").WithURL(site.URL).WithPath("/").Do(ctx)
		}),
		chromedp.Navigate(site.URL+"/"),
		chromedp.Evaluate(`getComputedStyle(document.body).backgroundColor`, &light),
		emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "dark"}}),
		chromedp.Evaluate(`getComputedStyle(document.body).backgroundColor`, &dark),
		emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-color-scheme", Value: "light"}}),
		chromedp.Evaluate(`getComputedStyle(document.body).backgroundColor`, &updatedLight),
	); err != nil {
		t.Fatalf("auto theme color-scheme emulation: %v", err)
	}
	if light == dark || light != updatedLight {
		t.Errorf("auto theme did not follow system scheme and return: light=%q dark=%q light-again=%q", light, dark, updatedLight)
	}
}

func TestBrowserLanguagePreferenceChangesRenderedStrings(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var language, searchLabel, visualClass, accentStyle, fontSize string
	var videoPreviewsEnabled bool
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.WaitVisible(`#general-settings`, chromedp.ByQuery),
		chromedp.SetValue(`#lang`, "de", chromedp.ByQuery),
		chromedp.SetValue(`#font-size`, "small", chromedp.ByQuery),
		chromedp.SetValue(`#results-density`, "spacious", chromedp.ByQuery),
		chromedp.SetValue(`#accent-color`, "#a1b2c3", chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('#video-previews').checked = false`, nil),
		chromedp.Click(`#save-preferences`, chromedp.ByQuery),
		chromedp.WaitVisible(`#searchQuery`, chromedp.ByQuery),
		chromedp.Evaluate(`document.documentElement.lang`, &language),
		chromedp.Evaluate(`document.querySelector('#searchQuery').getAttribute('aria-label')`, &searchLabel),
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.Evaluate(`document.querySelector('#video-previews').checked`, &videoPreviewsEnabled),
		chromedp.Evaluate(`document.documentElement.className`, &visualClass),
		chromedp.Evaluate(`document.documentElement.style.getPropertyValue('--color-primary').trim()`, &accentStyle),
		chromedp.Evaluate(`getComputedStyle(document.documentElement).fontSize`, &fontSize),
	); err != nil {
		t.Fatalf("save language preference in browser: %v", err)
	}
	if language != "de" || searchLabel != "Suchen" {
		t.Fatalf("language preference rendered lang=%q search label=%q, want de/Suchen", language, searchLabel)
	}
	if videoPreviewsEnabled {
		t.Fatal("JavaScript preferences save did not persist the disabled video preview preference")
	}
	if !strings.Contains(visualClass, "font-size-small") || !strings.Contains(visualClass, "results-density-spacious") || accentStyle != "#a1b2c3" || fontSize != "14px" {
		t.Fatalf("JavaScript visual preferences did not persist: class=%q accent=%q font_size=%q", visualClass, accentStyle, fontSize)
	}
	requireNoFailures(t, pf, "language preference save and localized homepage")
}

func TestBrowserDisabledVideoPreviewsSkipPreviewSetup(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	if err := chromedp.Run(ctx, network.SetCookie("search_pref_video_previews", "0").WithURL(site.URL).WithPath("/")); err != nil {
		t.Fatalf("set disabled video preview preference: %v", err)
	}
	var previewContainerMissing bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL),
		chromedp.Evaluate(`(() => { const result = document.createElement('a'); result.className = 'video-result'; result.dataset.videoEmbed = 'https://www.youtube-nocookie.com/embed/abc123XYZ_9'; document.body.appendChild(result); result.dispatchEvent(new MouseEvent('mouseenter', {bubbles: true})); })()`, nil),
		chromedp.Sleep(700*time.Millisecond),
		chromedp.Evaluate(`document.querySelector('#video-preview-container') === null`, &previewContainerMissing),
	); err != nil {
		t.Fatalf("disabled video preview setup: %v", err)
	}
	if !previewContainerMissing {
		t.Fatal("disabled video previews initialized the preview container")
	}
}

func TestBrowserVideoPreviewFallsBackWithoutLocalClip(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	var previewVisible bool
	var localPreviewState string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL),
		chromedp.Evaluate(`(() => {
            const result = document.createElement('article');
            result.className = 'video-result';
            result.dataset.videoEmbed = 'https://www.youtube-nocookie.com/embed/abc123XYZ_9';
            result.dataset.videoDuration = '120';
            result.dataset.videoPreviewUrl = 'https://media.example.test/clip.mp4';
            result.innerHTML = '<div class="video-thumbnail-container"><img class="video-thumbnail" alt="Static fallback"></div><h3 class="video-title">Fixture clip</h3>';
            document.body.appendChild(result);
            window.__videoPreviewAccepted = window.VideoPreview.show(result);
        })()`, nil),
		chromedp.Evaluate(`window.__videoPreviewAccepted && !!document.querySelector('#video-preview-container.visible')`, &previewVisible),
	); err != nil {
		t.Fatalf("video preview fallback behavior: %v", err)
	}
	if previewVisible {
		t.Error("preview without a same-origin WebM/MP4 clip did not fall back to the static thumbnail")
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
        HTMLMediaElement.prototype.play = function() { return Promise.resolve(); };
        const result = document.createElement('article');
        result.className = 'video-result';
        result.dataset.videoPreviewUrl = new URL('/fixtures/preview.webm', location.href).href;
        result.dataset.videoDuration = '90';
        result.innerHTML = '<div class="video-thumbnail-container"></div><h3 class="video-title">Local clip</h3>';
        document.body.appendChild(result);
        const accepted = window.VideoPreview.show(result);
        const video = document.querySelector('#video-preview-container video');
        return [accepted, video && video.src.startsWith(location.origin), video && video.muted, !!document.querySelector('#video-preview-container iframe')].join(':');
    })()`, &localPreviewState)); err != nil {
		t.Fatalf("same-origin video preview setup: %v", err)
	}
	if localPreviewState != "true:true:true:false" {
		t.Errorf("same-origin clip preview state = %q, want accepted/local/muted/no iframe", localPreviewState)
	}
}

// TestBrowserHomepageWidgets covers the dashboard's collapse, removal, and
// persistence interactions through the actual homepage UI.
func TestBrowserHomepageWidgets(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var state string
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#widget-grid [data-widget="clock"] .widget-collapse`, chromedp.ByQuery),
		chromedp.Click(`#widget-grid [data-widget="clock"] .widget-collapse`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => { const w=document.querySelector('[data-widget="clock"]'); return w.classList.contains('widget-collapsed') + ':' + w.querySelector('.widget-collapse').getAttribute('aria-expanded'); })()`, &state),
	); err != nil {
		t.Fatalf("homepage widget collapse: %v", err)
	}
	if state != "true:false" {
		t.Errorf("collapsed widget state = %q, want true:false", state)
	}
	if err := chromedp.Run(ctx,
		chromedp.Click(`#widget-grid [data-widget="clock"] .widget-menu`, chromedp.ByQuery),
		chromedp.WaitVisible(`.widget-dropdown-menu [data-menu-action="remove"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open homepage widget menu: %v", err)
	}
	var saved bool
	if err := chromedp.Run(ctx,
		chromedp.Click(`.widget-dropdown-menu [data-menu-action="remove"]`, chromedp.ByQuery),
		chromedp.WaitNotPresent(`#widget-grid [data-widget="clock"]`, chromedp.ByQuery),
		chromedp.Poll(`(() => { const item = document.cookie.split('; ').find(value => value.startsWith('search_widgets=')); return !!item && !decodeURIComponent(item.split('=')[1]).split(',').includes('clock'); })()`, &saved, chromedp.WithPollingInterval(25*time.Millisecond), chromedp.WithPollingTimeout(5*time.Second)),
	); err != nil {
		t.Fatalf("remove homepage widget: %v", err)
	}
	if !saved {
		t.Errorf("widget removal did not update the preference cookie")
	}
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/"),
		chromedp.Evaluate(`(document.querySelector('#widget-grid [data-widget="clock"]') === null).toString()`, &state),
	); err != nil {
		t.Fatalf("reload widget dashboard: %v", err)
	}
	if state != "true" {
		t.Errorf("removed widget did not persist after reload: %s", state)
	}
	requireNoFailures(t, pf, "homepage widget controls")
}

func TestBrowserHomepageWidgetReorderPersists(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var initialJSON, reorderedJSON, storedOrder, persistedJSON string
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error { return grantPreferenceConsent(ctx, site.URL) }),
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#widget-grid [data-widget]`, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.stringify(Array.from(document.querySelectorAll('#widget-grid [data-widget]'), function(widget) { return widget.dataset.widget; }))`, &initialJSON),
		chromedp.Evaluate(`(() => {
			const grid = document.getElementById('widget-grid');
			const widget = grid.querySelector('[data-widget]');
			const dataTransfer = new DataTransfer();
			widget.dispatchEvent(new DragEvent('dragstart', {bubbles: true, dataTransfer: dataTransfer}));
			grid.dispatchEvent(new DragEvent('dragover', {bubbles: true, cancelable: true, dataTransfer: dataTransfer, clientY: 1000000}));
			grid.dispatchEvent(new DragEvent('drop', {bubbles: true, cancelable: true, dataTransfer: dataTransfer}));
			widget.dispatchEvent(new DragEvent('dragend', {bubbles: true, dataTransfer: dataTransfer}));
			return JSON.stringify(Array.from(grid.querySelectorAll('[data-widget]'), function(item) { return item.dataset.widget; }));
		})()`, &reorderedJSON),
		chromedp.Poll(`(() => {
			const cookie = document.cookie.split('; ').find(function(value) { return value.startsWith('search_widgets='); });
			return cookie ? decodeURIComponent(cookie.slice('search_widgets='.length)) : '';
		})()`, &storedOrder, chromedp.WithPollingInterval(25*time.Millisecond), chromedp.WithPollingTimeout(5*time.Second)),
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`#widget-grid [data-widget]`, chromedp.ByQuery),
		chromedp.Evaluate(`JSON.stringify(Array.from(document.querySelectorAll('#widget-grid [data-widget]'), function(widget) { return widget.dataset.widget; }))`, &persistedJSON),
	); err != nil {
		t.Fatalf("homepage widget reorder: %v", err)
	}
	var initial, reordered, persisted []string
	if err := json.Unmarshal([]byte(initialJSON), &initial); err != nil {
		t.Fatalf("decode initial widget order %q: %v", initialJSON, err)
	}
	if err := json.Unmarshal([]byte(reorderedJSON), &reordered); err != nil {
		t.Fatalf("decode reordered widget order %q: %v", reorderedJSON, err)
	}
	if err := json.Unmarshal([]byte(persistedJSON), &persisted); err != nil {
		t.Fatalf("decode persisted widget order %q: %v", persistedJSON, err)
	}
	if len(initial) < 2 {
		t.Fatalf("widget reorder requires at least two enabled widgets, got %v", initial)
	}
	want := append(append([]string{}, initial[1:]...), initial[0])
	if strings.Join(reordered, ",") != strings.Join(want, ",") {
		t.Errorf("dragged widget order = %v, want %v", reordered, want)
	}
	if strings.Trim(storedOrder, `"`) != strings.Join(want, ",") {
		t.Errorf("widget order cookie = %q, want %q", storedOrder, strings.Join(want, ","))
	}
	if strings.Join(persisted, ",") != strings.Join(want, ",") {
		t.Errorf("reordered widgets after reload = %v, want %v", persisted, want)
	}
	requireNoFailures(t, pf, "homepage widget reorder")
}

// TestBrowserSwaggerDocsRender loads the Swagger UI page (PART 14 API docs
// requirement) and asserts the JS-rendered widget actually mounts, with no
// console errors — a common failure mode for embedded doc viewers is a CSP
// or asset-path break that only shows up as a silent blank page.
func TestBrowserSwaggerDocsRender(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/server/docs/swagger"),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}

	requireNoFailures(t, pf, "/server/docs/swagger")
}

// TestBrowserDirectQRDownloadsRender checks the full-page QR downloads in the
// real browser and guards against frontend asset or console failures.
func TestBrowserDirectQRDownloadsRender(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	var downloads int64
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/direct/qr/hello%20world"),
		chromedp.WaitVisible(`.direct-answer-page[data-type="qr"]`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelectorAll('.qr-content a[download]').length`, &downloads),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}
	if downloads < 7 {
		t.Errorf("full-page QR download links = %d, want PNG sizes and SVG variants", downloads)
	}
	requireNoFailures(t, pf, "/direct/qr/hello world")
}

// TestBrowserQRCustomizationSubmitsAndRendersInBrowser covers the full-JS
// tier for the QR generator's server-backed option controls.
func TestBrowserQRCustomizationSubmitsAndRendersInBrowser(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var pageURL, imageWidth, csrfState, formAction string
	if err := chromedp.Run(ctx, chromedp.Navigate(site.URL+"/direct/qr/hello%20world"), chromedp.AttributeValue(`form.qr-options`, "action", &formAction, nil), chromedp.Evaluate(`(() => { const field = document.querySelector('input[name="csrf_token"]')?.value || ''; const cookie = document.cookie.split('; ').find(v => v.startsWith('csrf_token='))?.slice(11) || ''; return field && cookie && field === cookie ? 'match' : 'mismatch field=' + !!field + ' cookie=' + !!cookie })()`, &csrfState)); err != nil {
		t.Fatalf("load QR page for CSRF check: %v", err)
	}
	if csrfState != "match" {
		t.Fatalf("QR browser CSRF state = %q", csrfState)
	}
	if !strings.HasPrefix(formAction, "/direct/qr?url=") {
		t.Fatalf("QR form action = %q, want query-based URL target", formAction)
	}
	if err := chromedp.Run(ctx,
		chromedp.Evaluate(`(() => {
			const size = document.querySelector('select[name="size"]');
			const level = document.querySelector('select[name="level"]');
			const background = document.querySelector('input[name="background"]');
			size.value = "XL";
			level.value = "H";
			background.value = "#ABCDEF";
			size.dispatchEvent(new Event("change", {bubbles: true}));
			level.dispatchEvent(new Event("change", {bubbles: true}));
			background.dispatchEvent(new Event("input", {bubbles: true}));
		})()`, nil),
		chromedp.Click(`.qr-options button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.direct-answer-page[data-type="qr"]`, chromedp.ByQuery),
		chromedp.Location(&pageURL), chromedp.AttributeValue(`.qr-code`, "width", &imageWidth, nil),
	); err != nil {
		t.Fatalf("browser QR customization: %v", err)
	}
	parsedURL, err := url.Parse(pageURL)
	if err != nil || parsedURL.Path != "/direct/qr" || parsedURL.Query().Get("url") != "hello world" {
		t.Errorf("QR customization navigated to unexpected URL %q", pageURL)
	}
	if imageWidth != "1024" {
		t.Errorf("selected XL QR image width = %q, want 1024", imageWidth)
	}
	requireNoFailures(t, pf, "/direct/qr customization")
}

// TestBrowserLocalDirectAnswerFeatures exercises every deterministic
// direct-answer page in a full browser and rejects frontend errors.
func TestBrowserLocalInstantAnswerFeatures(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	for _, tc := range localInstantAnswerPages {
		t.Run(tc.name, func(t *testing.T) {
			var html string
			if err := chromedp.Run(ctx,
				chromedp.Navigate(site.URL+"/search?q="+url.QueryEscape(tc.query)),
				chromedp.WaitVisible(`.instant-answer-box[data-type="`+tc.typeName+`"]`, chromedp.ByQuery),
				chromedp.OuterHTML("html", &html),
			); err != nil {
				t.Fatalf("browser %s: %v", tc.name, err)
			}
			if !strings.Contains(html, `class="instant-answer-content"`) {
				t.Errorf("browser %s returned no answer content", tc.name)
			}
			assertInstantAnswerFixture(t, tc.typeName, html)
		})
	}
	requireNoFailures(t, pf, "deterministic instant-answer scenarios")
}

func TestBrowserLocalDirectAnswerFeatures(t *testing.T) {
	site := newTestSite(t)
	for _, tc := range localDirectAnswerPages {
		t.Run(tc.name, func(t *testing.T) {
			ctx := newBrowserContext(t)
			pf := watchForFailures(ctx)
			var answerText string
			if err := chromedp.Run(ctx, chromedp.Navigate(site.URL+tc.path)); err != nil {
				t.Fatalf("browser %s navigation: %v", tc.name, err)
			}
			var pageHTML string
			if err := chromedp.Run(ctx,
				chromedp.OuterHTML("html", &pageHTML),
				chromedp.Evaluate(`document.querySelector('.direct-content')?.innerText || ''`, &answerText),
			); err != nil {
				t.Fatalf("browser %s page: %v", tc.name, err)
			}
			if !strings.Contains(pageHTML, `direct-answer-page`) || !strings.Contains(pageHTML, `data-type="`+tc.typeName+`"`) {
				t.Fatalf("browser %s unexpected page: %s", tc.name, snippet(pageHTML, "<body"))
			}
			if len(strings.TrimSpace(answerText)) < 20 {
				t.Errorf("browser %s answer is empty or incomplete: %q", tc.name, answerText)
			}
			assertDirectAnswerFixture(t, tc.typeName, tc.path, answerText)
			assertFormatterHighlighting(t, tc.typeName, tc.path, pageHTML)
			if tc.typeName == "man" {
				assertManPageControls(t, pageHTML)
				var seeAlsoHidden, detailsOpen, detailsCanCollapse bool
				var modernCopy, fallbackCopy string
				if err := chromedp.Run(ctx,
					chromedp.Evaluate(`(() => { const field = document.querySelector('[data-man-search]'); field.value = 'SEE ALSO'; field.dispatchEvent(new Event('input', {bubbles:true})); return document.querySelector('#man-name-1').closest('.man-section').hidden; })()`, &seeAlsoHidden),
					chromedp.Evaluate(`document.querySelector('#man-see-also-3').closest('details').open`, &detailsOpen),
					chromedp.Evaluate(`(() => { window.__manCopied = ''; Object.defineProperty(navigator, 'clipboard', {configurable:true, value:{writeText: async text => {window.__manCopied = text;}}}); document.querySelector('#man-see-also-3').closest('details').querySelector('[data-man-copy]').click(); return true; })()`, nil),
					chromedp.Evaluate(`window.__manCopied`, &modernCopy),
					chromedp.Evaluate(`(() => { window.__manCopiedFallback = ''; Object.defineProperty(navigator, 'clipboard', {configurable:true, value:undefined}); document.execCommand = command => {window.__manCopiedFallback = document.activeElement.value; return command === 'copy';}; document.querySelector('#man-see-also-3').closest('details').querySelector('[data-man-copy]').click(); return window.__manCopiedFallback; })()`, &fallbackCopy),
					chromedp.Evaluate(`(() => { const details = document.querySelector('#man-see-also-3').closest('details'); details.querySelector('summary').click(); return !details.open; })()`, &detailsCanCollapse),
				); err != nil {
					t.Fatalf("man page controls: %v", err)
				}
				if !seeAlsoHidden || !detailsOpen || !detailsCanCollapse || modernCopy != `echo "hi" --quiet` || fallbackCopy != modernCopy {
					t.Errorf("man page interaction state = hidden %v, search-open %v, native-collapse %v, modern-copy %q, fallback-copy %q", seeAlsoHidden, detailsOpen, detailsCanCollapse, modernCopy, fallbackCopy)
				}
			}
			if strings.Contains(tc.name, "Wikipedia") {
				assertFullWikipediaFixture(t, pageHTML)
				if strings.Contains(tc.name, "localized") && !strings.Contains(pageHTML, `value="de" selected`) {
					t.Error("localized Wikipedia answer did not preserve the selected language")
				}
			}
			requireNoFailures(t, pf, tc.path)
		})
	}
}

func TestBrowserChmodPermissionGridUpdatesOutput(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var octal, symbolic, command string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/direct/chmod/700"),
		chromedp.WaitVisible(`[data-chmod-calculator]`, chromedp.ByQuery),
		chromedp.Click(`.chmod-toggle[data-principal="g"][data-permission="read"]`, chromedp.ByQuery),
		chromedp.Text(`[data-chmod-octal]`, &octal, chromedp.ByQuery),
		chromedp.Text(`[data-chmod-symbolic]`, &symbolic, chromedp.ByQuery),
		chromedp.Text(`[data-chmod-command]`, &command, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("update chmod grid: %v", err)
	}
	if octal != "740" || symbolic != "rwxr-----" || command != "chmod 740 filename" {
		t.Errorf("chmod grid outputs = octal %q symbolic %q command %q, want 740 rwxr----- chmod 740 filename", octal, symbolic, command)
	}
	for _, selector := range []string{
		`.chmod-toggle[data-principal="g"][data-permission="write"]`,
		`.chmod-toggle[data-principal="g"][data-permission="execute"]`,
		`.chmod-toggle[data-principal="o"][data-permission="read"]`,
		`.chmod-toggle[data-principal="o"][data-permission="write"]`,
		`.chmod-toggle[data-principal="o"][data-permission="execute"]`,
	} {
		if err := chromedp.Run(ctx, chromedp.Click(selector, chromedp.ByQuery)); err != nil {
			t.Fatalf("enable permission %s: %v", selector, err)
		}
	}
	var warningVisible bool
	if err := chromedp.Run(ctx,
		chromedp.Text(`[data-chmod-octal]`, &octal, chromedp.ByQuery),
		chromedp.Evaluate(`!document.querySelector('[data-chmod-warning]').hidden`, &warningVisible),
	); err != nil {
		t.Fatalf("read chmod security warning: %v", err)
	}
	if octal != "777" || !warningVisible {
		t.Errorf("chmod mode = %q with warning visible=%t, want 777 with warning", octal, warningVisible)
	}
	requireNoFailures(t, pf, "interactive chmod calculator")
}

func TestBrowserArchiveTimelineSliderUpdatesSelection(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	var initial, updated, content string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/direct/cache/8.8.8.8"),
		chromedp.WaitVisible(`.archive-timeline input[type="range"]`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.archive-timeline output').textContent`, &initial),
		chromedp.Evaluate(`(() => { const range = document.querySelector('.archive-timeline input[type="range"]'); range.value = '1'; range.dispatchEvent(new Event('input', {bubbles: true})); return document.querySelector('.archive-timeline output').textContent + '|' + document.querySelector('.archive-timeline select[name="snapshot_id"]').selectedIndex; })()`, &updated),
		chromedp.Click(`.archive-timeline button[type="submit"]`, chromedp.ByQuery),
		chromedp.WaitVisible(`.direct-content`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.direct-content')?.innerText || ''`, &content),
	); err != nil {
		t.Fatalf("archive timeline interaction: %v", err)
	}
	if initial == updated || !strings.Contains(updated, "2020-01-01T00:00:00Z · Wayback Machine|1") {
		t.Errorf("timeline label did not update to the selected capture: initial=%q updated=%q", initial, updated)
	}
	if !strings.Contains(content, "Archived fixture page older capture") {
		t.Errorf("submitted timeline did not render the selected capture: %q", content)
	}
}

// TestBrowserResponsiveViewportNoHorizontalScroll covers PART 16's mobile
// rule at the 375x812 viewport the spec names explicitly: the document must
// not overflow horizontally, which is the most common broken-responsive
// symptom (a fixed-width element wider than the viewport).
func TestBrowserResponsiveViewportNoHorizontalScroll(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	var scrollWidth, clientWidth int64
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Evaluate(`document.documentElement.scrollWidth`, &scrollWidth),
		chromedp.Evaluate(`document.documentElement.clientWidth`, &clientWidth),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}

	if scrollWidth > clientWidth {
		t.Errorf("375px viewport: scrollWidth=%d > clientWidth=%d, page overflows horizontally", scrollWidth, clientWidth)
	}
	requireNoFailures(t, pf, "/ (375x812 viewport)")
}

func TestBrowserMobileSearchAndNavigationControls(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var overflow, searchWidth, viewportWidth, minTarget float64
	var navToggle, categoriesAvailable, resultLayout, preferencesSheetOpen bool
	var sheetBottom, viewportHeight float64
	if err := chromedp.Run(ctx,
		chromedp.EmulateViewport(375, 812),
		chromedp.Navigate(site.URL+"/"),
		chromedp.WaitVisible(`.search-form`, chromedp.ByQuery),
		chromedp.Evaluate(`document.documentElement.scrollWidth - document.documentElement.clientWidth`, &overflow),
		chromedp.Evaluate(`document.querySelector('.search-form')?.getBoundingClientRect().width || 0`, &searchWidth),
		chromedp.Evaluate(`document.documentElement.clientWidth`, &viewportWidth),
		chromedp.Evaluate(`!!document.querySelector('label[for="nav-toggle"]')`, &navToggle),
		chromedp.Evaluate(`!!document.querySelector('.category-tabs')`, &categoriesAvailable),
		chromedp.Evaluate(`(() => { const els = Array.from(document.querySelectorAll('.header-mobile label, .search-form button, .category-tab')); const sizes = els.map(e => e.getBoundingClientRect()).filter(r => r.width > 0 && r.height > 0).map(r => Math.min(r.width, r.height)); return sizes.length ? Math.min(...sizes) : 0 })()`, &minTarget),
		chromedp.Click(`[data-mobile-preferences-trigger]`, chromedp.ByQuery),
		chromedp.WaitVisible(`#mobile-preferences-dialog[open]`, chromedp.ByQuery),
		chromedp.Evaluate(`!!document.querySelector('#mobile-preferences-dialog[open]')`, &preferencesSheetOpen),
		chromedp.Evaluate(`document.querySelector('#mobile-preferences-dialog').getBoundingClientRect().bottom`, &sheetBottom),
		chromedp.Evaluate(`window.innerHeight`, &viewportHeight),
		chromedp.Click(`#mobile-preferences-dialog form[method="dialog"] button`, chromedp.ByQuery),
		chromedp.Navigate(site.URL+"/search?q=mobile+layout"),
		chromedp.WaitVisible(`.search-results-page`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => { const list = document.querySelector('.results-list'); return !!list && getComputedStyle(list).flexDirection !== 'row' })()`, &resultLayout),
	); err != nil {
		t.Fatalf("mobile search layout: %v", err)
	}
	if overflow > 0 || searchWidth < viewportWidth*0.8 {
		t.Errorf("mobile layout overflow=%g search width=%g viewport=%g", overflow, searchWidth, viewportWidth)
	}
	if !navToggle || !categoriesAvailable {
		t.Error("mobile header navigation toggle or category navigation is missing")
	}
	if !preferencesSheetOpen || sheetBottom < viewportHeight-2 {
		t.Errorf("mobile preferences did not open as a bottom sheet: open=%t bottom=%g viewport height=%g", preferencesSheetOpen, sheetBottom, viewportHeight)
	}
	if minTarget > 0 && minTarget < 44 {
		t.Errorf("smallest visible interactive target is %gpx, want at least 44px", minTarget)
	}
	if !resultLayout {
		t.Error("mobile results list is missing or laid out horizontally")
	}
	requireNoFailures(t, pf, "/search (375x812 mobile layout)")
}

// TestBrowserErrorPageNoConsoleErrors loads a 404 to make sure the themed
// error page (PART 16) itself doesn't throw — error pages are frequently
// undertested because the happy-path crawl never hits them.
func TestBrowserErrorPageNoConsoleErrors(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/this-route-does-not-exist"),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}

	// The 404 status itself is asserted at Tier 1 (ssr_test.go); this test's
	// job is only the console/asset cleanliness of the rendered error page.
	requireNoFailures(t, pf, "/this-route-does-not-exist (404 page)")
}

func TestBrowserRecoveredPanicShowsThemedErrorPage(t *testing.T) {
	site := newPanicTestSite(t)
	t.Cleanup(site.Close)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var html string
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie("theme", "light").WithURL(site.URL).WithPath("/").Do(ctx)
		}),
		chromedp.Navigate(site.URL+"/__e2e/panic"),
		chromedp.WaitVisible(`.error-page`, chromedp.ByQuery),
		chromedp.OuterHTML("html", &html),
	); err != nil {
		t.Fatalf("browser recovery page: %v", err)
	}
	for _, marker := range []string{`class="theme-light"`, "Something went wrong", `href="/server/about"`} {
		if !strings.Contains(html, marker) {
			t.Errorf("browser recovery page is missing %q", marker)
		}
	}
	if strings.Contains(html, "e2e panic sentinel") || strings.Contains(html, "goroutine ") {
		t.Fatal("browser recovery page leaked panic details or a stack trace")
	}
	requireNoFailures(t, pf, "/__e2e/panic (500 page)")
}

// TestBrowserInstantAnswersInteract exercises generated instant-answer HTML
// through the same delegated browser controls users click on the search page.
func TestBrowserInstantAnswersInteract(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	var beforeUUID, afterUUID, stopwatchTime string
	var lapCount int64
	var timerFinished bool
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/search?q=stopwatch"),
		chromedp.WaitVisible(`.instant-answer-content .stopwatch-widget`, chromedp.ByQuery),
		chromedp.Click(`.stopwatch-start`, chromedp.ByQuery),
		chromedp.Sleep(250*time.Millisecond),
		chromedp.Click(`.stopwatch-lap`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.stopwatch-laps').children.length`, &lapCount),
		chromedp.Click(`.stopwatch-stop`, chromedp.ByQuery),
		chromedp.Click(`.stopwatch-reset`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.stopwatch-display').textContent`, &stopwatchTime),
		chromedp.Navigate(site.URL+"/search?q=timer+1+second"),
		chromedp.WaitVisible(`.instant-answer-content .timer-widget`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => { const timer = document.querySelector('.timer-widget'); timer.addEventListener('timercomplete', () => { timer.dataset.completed = 'true'; }); })()`, nil),
		chromedp.Click(`.timer-start`, chromedp.ByQuery),
		chromedp.Poll(`document.querySelector('.timer-widget').dataset.completed === 'true'`, &timerFinished, chromedp.WithPollingInterval(50*time.Millisecond), chromedp.WithPollingTimeout(5*time.Second)),
		chromedp.Navigate(site.URL+"/search?q=uuid"),
		chromedp.WaitVisible(`.instant-answer-content .uuid-result`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('[data-uuid-format="standard"]').textContent`, &beforeUUID),
		chromedp.Click(`[data-action="uuid-regenerate"]`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('[data-uuid-format="standard"]').textContent`, &afterUUID),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}
	if lapCount != 1 {
		t.Errorf("stopwatch lap count = %d, want 1", lapCount)
	}
	if stopwatchTime != "00:00:00" {
		t.Errorf("stopwatch reset display = %q, want 00:00:00", stopwatchTime)
	}
	if !timerFinished {
		t.Error("countdown timer did not dispatch its completion event")
	}
	if beforeUUID == "" || afterUUID == "" || beforeUUID == afterUUID {
		t.Errorf("UUID regenerate did not produce a different value: before=%q after=%q", beforeUUID, afterUUID)
	}
	requireNoFailures(t, pf, "instant-answer stopwatch and UUID controls")
}

// TestBrowserInstantAnswerCopyControls verifies the delegated copy buttons
// write each answer value through the browser Clipboard API.
func TestBrowserInstantAnswerCopyControls(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	for _, query := range []string{
		"sqrt(144)", "sha256 hello", "base64 encode hello", "url encode hello world",
		"generate uuid", "generate password", "#ff0000", "lorem ipsum 3 paragraphs",
	} {
		t.Run(query, func(t *testing.T) {
			var expected, copied string
			if err := chromedp.Run(ctx,
				chromedp.Navigate(site.URL+"/search?q="+url.QueryEscape(query)),
				chromedp.WaitVisible(`.instant-answer-content .copy-btn`, chromedp.ByQuery),
				chromedp.Evaluate(`(() => { Object.defineProperty(navigator, 'clipboard', {configurable: true, value: {writeText: value => {window.__e2eCopied = value; return Promise.resolve();}}}); return document.querySelector('.instant-answer-content .copy-btn').dataset.copy; })()`, &expected),
				chromedp.Click(`.instant-answer-content .copy-btn`, chromedp.ByQuery),
				chromedp.Evaluate(`window.__e2eCopied || ''`, &copied),
			); err != nil {
				t.Fatalf("copy control for %q: %v", query, err)
			}
			if expected == "" || copied != expected {
				t.Errorf("copy control wrote %q, want %q", copied, expected)
			}
		})
	}
	requireNoFailures(t, pf, "instant-answer copy controls")
}

// TestBrowserInstantAnswerControls exercises additional in-page widgets with
// JavaScript enabled while the E2E Docker network blocks all public egress.
func TestBrowserInstantAnswerControls(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)

	var initialColor, changedColor, oldPassword, newPassword, calculatorText string
	var qrImageCount, qrDownloadCount, randomBefore, randomAfter int64
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/search?q=%23ff5733"),
		chromedp.WaitVisible(`.color-result`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('[data-color-value="hex"]').textContent`, &initialColor),
		chromedp.Evaluate(`(() => { const input = document.querySelector('.color-preview'); const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set; setter.call(input, '#00ff00'); input.dispatchEvent(new Event('input', {bubbles:true})); return document.querySelector('[data-color-value="hex"]').textContent; })()`, &changedColor),
		chromedp.Navigate(site.URL+"/search?q=password+16"),
		chromedp.WaitVisible(`.password-result`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.password-value').textContent`, &oldPassword),
		chromedp.Click(`[data-action="regenerate-password"]`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.password-value').textContent`, &newPassword),
		chromedp.Navigate(site.URL+"/search?q=qr+offline+test"),
		chromedp.WaitVisible(`.instant-answer-content .qr-code-preview`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelectorAll('.instant-answer-content .qr-code-preview').length`, &qrImageCount),
		chromedp.Evaluate(`document.querySelectorAll('.instant-answer-content a[download]').length`, &qrDownloadCount),
		chromedp.Navigate(site.URL+"/search?q=2%2B2"),
		chromedp.WaitVisible(`.instant-answer-content`, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector('.instant-answer-content').innerText`, &calculatorText),
		chromedp.Navigate(site.URL+"/search?q=roll+d20"),
		chromedp.WaitVisible(`.random-result`, chromedp.ByQuery),
		chromedp.Evaluate(`parseInt(document.querySelector('.random-value').textContent, 10)`, &randomBefore),
		chromedp.Click(`[data-action="roll-again"]`, chromedp.ByQuery),
		chromedp.Evaluate(`parseInt(document.querySelector('.random-value').textContent, 10)`, &randomAfter),
	); err != nil {
		t.Fatalf("chromedp run: %v", err)
	}
	if initialColor != "#FF5733" || changedColor != "#00FF00" {
		t.Errorf("color picker values = %q then %q, want #FF5733 then #00FF00", initialColor, changedColor)
	}
	if oldPassword == "" || newPassword == "" || oldPassword == newPassword {
		t.Errorf("password regenerate did not produce a new value: before=%q after=%q", oldPassword, newPassword)
	}
	if qrImageCount != 1 || qrDownloadCount < 2 {
		t.Errorf("QR output has %d preview images and %d download links, want one image and at least two downloads", qrImageCount, qrDownloadCount)
	}
	if !strings.Contains(calculatorText, "4") {
		t.Errorf("calculator answer does not show 4: %q", calculatorText)
	}
	if randomBefore < 1 || randomBefore > 20 || randomAfter < 1 || randomAfter > 20 {
		t.Errorf("d20 roll values = %d and %d, want values in [1,20]", randomBefore, randomAfter)
	}
	requireNoFailures(t, pf, "instant-answer calculator, color, password, random, and QR controls")
}

func TestBrowserSearchHistoryImportRespectsOptOut(t *testing.T) {
	site := newTestSite(t)
	ctx := newBrowserContext(t)
	pf := watchForFailures(ctx)
	var stored string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(site.URL+"/server/preferences"),
		chromedp.WaitVisible(`#export-prefs`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => {
			const payload = JSON.stringify({preferences: {save_history: false}, search_history: [{query: "must be removed", category: "general"}]});
			const file = new File([payload], "preferences.json", {type: "application/json"});
			const transfer = new DataTransfer();
			transfer.items.add(file);
			const input = document.querySelector("#import-file");
			input.files = transfer.files;
			input.dispatchEvent(new Event("change", {bubbles: true}));
			return true;
		})()`, nil),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`JSON.stringify({saveHistory: document.cookie.match(/(?:^|;\s*)search_pref_save_history=([^;]*)/)?.[1] === '1', history: localStorage.getItem('search_history')})`, &stored),
	); err != nil {
		t.Fatalf("import preferences with history disabled: %v", err)
	}
	var importedState struct {
		SaveHistory bool   `json:"saveHistory"`
		History     string `json:"history"`
	}
	if err := json.Unmarshal([]byte(stored), &importedState); err != nil {
		t.Fatalf("decode imported local state: %v", err)
	}
	if importedState.SaveHistory || importedState.History != "" {
		t.Fatalf("imported local state = %s, want history disabled and cleared", stored)
	}
	requireNoFailures(t, pf, "history import respects local opt-out")
}
