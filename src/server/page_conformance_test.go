package server

import (
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/security"
	"github.com/apimgr/search/src/version"
	xhtml "golang.org/x/net/html"
)

func newHTMLPageRequest(method, target string, body io.Reader) *http.Request {
	request := httptest.NewRequest(method, target, body)
	request.Header.Set("Accept", "text/html")
	return request
}

func TestServerPageConformance(t *testing.T) {
	s := newTestServer(t)
	previousCookieConsent := s.config.Server.Web.CookieConsent
	previousConsent := s.config.Server.Privacy.Consent
	s.config.Server.Web.CookieConsent.Enabled = true
	s.config.Server.Privacy.Consent.ShowPreferences = true
	t.Cleanup(func() {
		s.config.Server.Web.CookieConsent = previousCookieConsent
		s.config.Server.Privacy.Consent = previousConsent
	})
	previousAggregator := s.aggregator
	engineConfig := model.NewEngineConfig("page-conformance")
	engineConfig.Categories = []string{"general", "images", "videos", "news", "maps", "files", "music", "science", "it", "social"}
	capture := &queryCaptureEngine{BaseEngine: search.NewBaseEngine(engineConfig), queries: make(chan model.Query, 1)}
	s.aggregator = search.NewAggregator([]search.Engine{capture}, search.AggregatorConfig{})
	t.Cleanup(func() { s.aggregator = previousAggregator })
	router := s.setupRoutes()
	paths := []string{
		"/",
		"/search?q=page+conformance",
		"/direct/ascii/Hello",
		"/alerts/new",
		"/server/about",
		"/server/privacy",
		"/server/contact",
		"/server/dpo",
		"/server/help",
		"/server/terms",
		"/server/security",
		"/server/security/policy",
		"/server/security/thanks",
		"/server/preferences",
		"/server/healthz",
		"/server/docs/swagger",
		"/server/docs/graphql",
	}
	subpages := map[string]bool{
		"/server/security/policy": true,
		"/server/security/thanks": true,
	}
	for _, page := range s.preferencesPages() {
		paths = append(paths, page.Path)
		subpages[page.Path] = true
	}
	for _, page := range s.helpPages(newHTMLPageRequest(http.MethodGet, "/server/help", nil)) {
		paths = append(paths, page.Path)
		subpages[page.Path] = true
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			request := newHTMLPageRequest(http.MethodGet, path, nil)
			request.Header.Set("Accept", "text/html")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK && (path != "/server/healthz" || response.Code != http.StatusServiceUnavailable) {
				t.Fatalf("GET %s status = %d, want %d; body=%q", path, response.Code, http.StatusOK, response.Body.String())
			}
			assertServerPageConformance(t, path, response.Body.String(), subpages[path])
			assertInternalLinksResolve(t, router, path, response.Body.String())
			if path == "/server/healthz" {
				assertHealthPageStructure(t, response.Body.String())
			}
			assertSharedPageComponents(t, path, response.Body.String())
			if path == "/server/preferences" {
				assertPreferenceSectionsVisible(t, response.Body.String())
			}
			if path == "/server/preferences/tools" && !strings.Contains(response.Body.String(), `<div id="custom-bangs-empty-state" class="empty-state">`) {
				t.Error("custom-bangs empty state is not rendered visibly before JavaScript runs")
			}
			if path == "/server/preferences/tokens" {
				for _, required := range []string{
					`id="new-token"`, `action="/server/preferences/token/create"`,
					`id="restore"`, `action="/server/preferences/token/restore"`,
				} {
					if !strings.Contains(response.Body.String(), required) {
						t.Errorf("owner tokens page without an identity is missing %q", required)
					}
				}
			}
			if strings.Contains(response.Body.String(), `class="page-content`) {
				t.Errorf("GET %s rendered the nested page-content wrapper; server page content must use the shared section-card components", path)
			}
			if path == "/server/privacy" {
				for _, required := range []string{
					`<article class="privacy-policy">`, `class="last-updated"`,
					`datetime="` + version.HealthBuildDate() + `"`,
					`class="key-points"`, `class="key-point"`, `aria-hidden="true">🔒`,
					`id="cookie-preferences-modal"`,
					`data-action="cookie-preferences"`,
					`id="pref-preferences"`,
					`id="pref-analytics"`,
					`action="/server/consent"`,
				} {
					if !strings.Contains(response.Body.String(), required) {
						t.Errorf("privacy page is missing cookie preference control %q", required)
					}
				}
			}
			if path == "/server/security/thanks" {
				for _, required := range []string{`class="empty-state"`, `class="empty-state-icon"`, `href="/server/security/policy"`} {
					if !strings.Contains(response.Body.String(), required) {
						t.Errorf("security acknowledgments page is missing empty-state component %q", required)
					}
				}
			}
		})
	}

	t.Run("created alert page and management page", func(t *testing.T) {
		if s.alertManager == nil || s.ownerManager == nil {
			t.Fatal("alert and owner stores are required to create the page fixture")
		}

		csrfRequest := newHTMLPageRequest(http.MethodGet, "/alerts/new", nil)
		csrfResponse := httptest.NewRecorder()
		router.ServeHTTP(csrfResponse, csrfRequest)
		if csrfResponse.Code != http.StatusOK {
			t.Fatalf("GET /alerts/new fixture status = %d, want %d", csrfResponse.Code, http.StatusOK)
		}
		csrfToken := conformanceInputValue(t, csrfResponse.Body.String(), "csrf_token")
		csrfCookieName := s.config.Server.Security.CSRF.CookieName
		if csrfCookieName == "" {
			csrfCookieName = "csrf_token"
		}
		var csrfCookie *http.Cookie
		for _, cookie := range csrfResponse.Result().Cookies() {
			if cookie.Name == csrfCookieName {
				csrfCookie = cookie
				break
			}
		}
		if csrfCookie == nil {
			t.Fatalf("GET /alerts/new did not issue %q cookie", csrfCookieName)
		}

		form := url.Values{
			s.config.Server.Security.CSRF.FieldName: {csrfToken},
			"query":                                 {"page conformance"},
			"category":                              {"general"},
			"language":                              {"en"},
			"safe_search":                           {"1"},
			"frequency":                             {"daily"},
			"email":                                 {"conformance@example.test"},
			"deliver_rss":                           {"on"},
		}
		if s.config.Server.Security.CSRF.FieldName == "" {
			form.Set("csrf_token", csrfToken)
		}
		createRequest := httptest.NewRequest(http.MethodPost, "/alerts", strings.NewReader(form.Encode()))
		createRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		createRequest.RemoteAddr = "203.0.113.221:1234"
		createRequest.AddCookie(csrfCookie)
		createResponse := httptest.NewRecorder()
		router.ServeHTTP(createResponse, createRequest)
		if createResponse.Code != http.StatusSeeOther {
			t.Fatalf("POST /alerts fixture status = %d, want %d; body=%q", createResponse.Code, http.StatusSeeOther, createResponse.Body.String())
		}
		if got := createResponse.Header().Get("Location"); got != "/alerts/created" {
			t.Fatalf("POST /alerts redirect = %q, want /alerts/created", got)
		}
		var alertResultCookie *http.Cookie
		for _, cookie := range createResponse.Result().Cookies() {
			if cookie.Name == alertCreateResultCookie {
				alertResultCookie = cookie
				break
			}
		}
		if alertResultCookie == nil || strings.Contains(alertResultCookie.Value, "conformance@example.test") {
			t.Fatal("POST /alerts did not issue an encrypted one-use result cookie")
		}
		missingResult := httptest.NewRecorder()
		router.ServeHTTP(missingResult, newHTMLPageRequest(http.MethodGet, "/alerts/created", nil))
		if missingResult.Code != http.StatusSeeOther || missingResult.Header().Get("Location") != "/alerts/new" {
			t.Fatal("GET /alerts/created without its result cookie must redirect safely to /alerts/new")
		}
		createdRequest := newHTMLPageRequest(http.MethodGet, "/alerts/created", nil)
		createdRequest.Header.Set("Accept", "text/html")
		createdRequest.AddCookie(alertResultCookie)
		createdResponse := httptest.NewRecorder()
		router.ServeHTTP(createdResponse, createdRequest)
		if createdResponse.Code != http.StatusOK {
			t.Fatalf("GET /alerts/created fixture status = %d, want %d; body=%q", createdResponse.Code, http.StatusOK, createdResponse.Body.String())
		}
		if createdResponse.Header().Get("Cache-Control") != "no-store" || createdResponse.Header().Get("Referrer-Policy") != "no-referrer" {
			t.Fatal("GET /alerts/created must prevent caching and referrer disclosure")
		}
		managePath := conformanceLinkPath(t, createdResponse.Body.String(), "/alerts/manage/")
		manageToken := strings.TrimPrefix(managePath, "/alerts/manage/")
		if manageToken == "" {
			t.Fatal("created alert page has an empty manage token")
		}
		t.Cleanup(func() {
			if err := s.alertManager.Delete(context.Background(), manageToken); err != nil {
				t.Errorf("delete conformance alert fixture: %v", err)
			}
		})

		assertServerPageConformance(t, "/alerts/created", createdResponse.Body.String(), false)
		assertInternalLinksResolve(t, router, "/alerts/created", createdResponse.Body.String())
		for _, required := range []string{`aria-labelledby="alert-created-heading"`, `class="section-card"`} {
			if !strings.Contains(createdResponse.Body.String(), required) {
				t.Errorf("created alert page is missing shared card markup %q", required)
			}
		}
		clearedCookie := false
		for _, cookie := range createdResponse.Result().Cookies() {
			if cookie.Name == alertCreateResultCookie && cookie.MaxAge < 0 {
				clearedCookie = true
			}
		}
		if !clearedCookie {
			t.Fatal("GET /alerts/created did not clear the one-use result cookie")
		}

		manageRequest := newHTMLPageRequest(http.MethodGet, managePath, nil)
		manageRequest.Header.Set("Accept", "text/html")
		manageResponse := httptest.NewRecorder()
		router.ServeHTTP(manageResponse, manageRequest)
		if manageResponse.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d; body=%q", managePath, manageResponse.Code, http.StatusOK, manageResponse.Body.String())
		}
		assertServerPageConformance(t, managePath, manageResponse.Body.String(), false)
		assertInternalLinksResolve(t, router, managePath, manageResponse.Body.String())
		for _, required := range []string{`aria-labelledby="alert-manage-heading"`, `class="form-input"`, `class="btn btn-danger"`} {
			if !strings.Contains(manageResponse.Body.String(), required) {
				t.Errorf("alert management page is missing shared component %q", required)
			}
		}
		if !strings.Contains(manageResponse.Body.String(), `id="delete-alert-popover"`) ||
			!strings.Contains(manageResponse.Body.String(), `action="/alerts/`+manageToken+`/delete"`) {
			t.Fatalf("alert deletion must be behind a native confirmation popover with a CSRF-protected POST form: %s", manageResponse.Body.String())
		}
	})

	t.Run("CCPA section follows sale configuration", func(t *testing.T) {
		previous := s.config.Server.Privacy.Data.Sold
		defer func() { s.config.Server.Privacy.Data.Sold = previous }()

		s.config.Server.Privacy.Data.Sold = false
		request := newHTMLPageRequest(http.MethodGet, "/server/privacy", nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /server/privacy status = %d, want %d", response.Code, http.StatusOK)
		}
		if strings.Contains(response.Body.String(), `id="ccpa-opt-out"`) {
			t.Fatal("privacy page rendered CCPA opt-out while data.sold is false")
		}

		s.config.Server.Privacy.Data.Sold = true
		request = newHTMLPageRequest(http.MethodGet, "/server/privacy", nil)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /server/privacy with data.sold status = %d, want %d", response.Code, http.StatusOK)
		}
		if !strings.Contains(response.Body.String(), `id="ccpa-opt-out"`) {
			t.Fatal("privacy page omitted CCPA opt-out while data.sold is true")
		}
	})

	t.Run("privacy page reflects storage, sharing, and request security", func(t *testing.T) {
		privacy := &s.config.Server.Privacy
		previousStored := privacy.Data.StoredOnServer
		previousSharing := privacy.Data.Sharing
		defer func() {
			privacy.Data.StoredOnServer = previousStored
			privacy.Data.Sharing = previousSharing
		}()
		privacy.Data.StoredOnServer = false
		privacy.Data.Sharing = []config.PrivacySharingCondition{{Condition: "test", When: "an operator action occurs", Data: "the selected record"}}

		for _, test := range []struct {
			name       string
			url        string
			forwarded  string
			wantSecure string
			wantOther  string
		}{
			{name: "direct TLS", url: "https://example.test/server/privacy", wantSecure: "Connections are encrypted with HTTPS/TLS."},
			{name: "trusted proxy TLS", url: "http://example.test/server/privacy", forwarded: "https", wantSecure: "Connections are encrypted with HTTPS/TLS."},
			{name: "plain HTTP", url: "http://example.test/server/privacy", wantOther: "This site is currently served without TLS."},
		} {
			t.Run(test.name, func(t *testing.T) {
				request := newHTMLPageRequest(http.MethodGet, test.url, nil)
				if test.forwarded != "" {
					request.RemoteAddr = "127.0.0.1:1234"
					request.Header.Set("X-Forwarded-Proto", test.forwarded)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					t.Fatalf("GET %s status = %d, want %d", test.url, response.Code, http.StatusOK)
				}
				body := response.Body.String()
				if test.wantSecure != "" && !strings.Contains(body, test.wantSecure) {
					t.Errorf("privacy page omitted secure connection text %q", test.wantSecure)
				}
				if test.wantOther != "" && !strings.Contains(body, test.wantOther) {
					t.Errorf("privacy page omitted insecure connection text %q", test.wantOther)
				}
				for _, expected := range []string{"Some data is stored with third-party services", "an operator action occurs", "the selected record"} {
					if !strings.Contains(body, expected) {
						t.Errorf("privacy page omitted configured disclosure %q", expected)
					}
				}
			})
		}
	})

	t.Run("privacy contact uses the DPO address when configured", func(t *testing.T) {
		previous := s.config.Server.Contact.DPO.Email
		defer func() { s.config.Server.Contact.DPO.Email = previous }()
		request := newHTMLPageRequest(http.MethodGet, "/server/privacy", nil)
		response := httptest.NewRecorder()
		s.config.Server.Contact.DPO.Email = "privacy@example.test"
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /server/privacy with DPO contact status = %d", response.Code)
		}
		if !strings.Contains(response.Body.String(), `href="mailto:privacy@example.test"`) {
			t.Fatal("privacy page omitted configured DPO email")
		}

		s.config.Server.Contact.DPO.Email = ""
		request = newHTMLPageRequest(http.MethodGet, "/server/privacy", nil)
		response = httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if !strings.Contains(response.Body.String(), `href="/server/security"`) {
			t.Fatal("privacy page omitted security contact fallback")
		}
	})

	t.Run("configured header content is the final header child", func(t *testing.T) {
		original := s.config.Server.Branding.Content
		defer func() { s.config.Server.Branding.Content = original }()
		s.config.Server.Branding.Content = nil
		baselineRequest := newHTMLPageRequest(http.MethodGet, "/server/about", nil)
		baselineRequest.Header.Set("Accept", "text/html")
		baselineResponse := httptest.NewRecorder()
		router.ServeHTTP(baselineResponse, baselineRequest)
		if baselineResponse.Code != http.StatusOK {
			t.Fatalf("GET /server/about without configured header status = %d", baselineResponse.Code)
		}
		baselineDocument, err := xhtml.Parse(strings.NewReader(baselineResponse.Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		baselineHeader := findHTMLNode(baselineDocument, "header", "class", "header")
		if baselineHeader == nil {
			t.Fatal("public header not rendered without configured content")
		}
		baselineSignature := headerCoreSignature(baselineHeader)
		s.config.Server.Branding.Content = map[string]config.BrandingPageContent{
			"header": {Format: "html", Body: `<p id="configured-header-fragment">Configured notice</p>`},
		}
		request := newHTMLPageRequest(http.MethodGet, "/server/about", nil)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("GET /server/about with configured header status = %d", response.Code)
		}
		document, err := xhtml.Parse(strings.NewReader(response.Body.String()))
		if err != nil {
			t.Fatal(err)
		}
		assertHeaderConformance(t, "configured header page", document, true)
		header := findHTMLNode(document, "header", "class", "header")
		if header == nil {
			t.Fatal("public header not rendered")
		}
		if configuredSignature := headerCoreSignature(header); configuredSignature != baselineSignature {
			t.Errorf("configured header content changed shared header elements\nbefore: %s\nafter:  %s", baselineSignature, configuredSignature)
		}
		lastElement := (*xhtml.Node)(nil)
		for child := header.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == xhtml.ElementNode {
				lastElement = child
			}
		}
		if lastElement == nil || !hasHTMLAttribute(lastElement, "id", "configured-header-fragment") {
			t.Fatalf("last header element is not the configured fragment: %#v", lastElement)
		}
		if findHTMLNode(header, "a", "class", "site-brand") == nil || findHTMLNode(header, "nav", "class", "nav-links") == nil || findHTMLNode(header, "div", "class", "header-actions") == nil {
			t.Fatal("configured header replaced required shared header elements")
		}
	})

	for _, hub := range []struct {
		path  string
		pages []HubPage
	}{
		{path: "/server/preferences", pages: s.preferencesPages()},
		{path: "/server/help", pages: s.helpPages(newHTMLPageRequest(http.MethodGet, "/server/help", nil))},
	} {
		t.Run("hub registry "+hub.path, func(t *testing.T) {
			request := newHTMLPageRequest(http.MethodGet, hub.path, nil)
			request.Header.Set("Accept", "text/html")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", hub.path, response.Code, http.StatusOK)
			}
			for _, page := range hub.pages {
				if count := strings.Count(response.Body.String(), `href="`+page.Path+`"`); count != 1 {
					t.Errorf("hub %s links registered page %s %d times, want once", hub.path, page.Path, count)
				}
			}
			if hub.path == "/server/help" {
				request := httptest.NewRequest(http.MethodGet, "/api/"+version.APIVersion+"/server/help", nil)
				request.Header.Set("Accept", "application/json")
				apiResponse := httptest.NewRecorder()
				router.ServeHTTP(apiResponse, request)
				if apiResponse.Code != http.StatusOK {
					t.Fatalf("GET /api/%s/server/help status = %d, want %d", version.APIVersion, apiResponse.Code, http.StatusOK)
				}
				var payload struct {
					Data struct {
						Pages []struct {
							Path string `json:"path"`
						} `json:"pages"`
					} `json:"data"`
				}
				if err := json.Unmarshal(apiResponse.Body.Bytes(), &payload); err != nil {
					t.Fatalf("decode help API response: %v", err)
				}
				apiPaths := make(map[string]bool, len(payload.Data.Pages))
				for _, page := range payload.Data.Pages {
					apiPaths[page.Path] = true
				}
				for _, page := range hub.pages {
					if !apiPaths[page.Path] {
						t.Errorf("help API omits registered page %s", page.Path)
					}
				}
			}
		})
	}

	t.Run("unmatched route error page", func(t *testing.T) {
		request := newHTMLPageRequest(http.MethodGet, "/server/missing-page", nil)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("missing page status = %d, want %d", response.Code, http.StatusNotFound)
		}
		assertInternalLinksResolve(t, router, "error page", response.Body.String())
		assertServerPageConformance(t, "error page", response.Body.String(), false)
	})

	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
	} {
		t.Run("rendered error status "+http.StatusText(status), func(t *testing.T) {
			request := newHTMLPageRequest(http.MethodGet, "/server/error-test", nil)
			request.Header.Set("Accept", "text/html")
			response := httptest.NewRecorder()
			s.handleError(response, request, status, http.StatusText(status), "A safe, helpful error message.")
			if response.Code != status {
				t.Fatalf("rendered error status = %d, want %d", response.Code, status)
			}
			assertServerPageConformance(t, "error status "+http.StatusText(status), response.Body.String(), false)
			assertInternalLinksResolve(t, router, "error status "+http.StatusText(status), response.Body.String())
		})
	}

	t.Run("valid security report status page", func(t *testing.T) {
		if s.dbManager == nil || s.dbManager.ServerDB() == nil {
			t.Fatal("server database is unavailable")
		}
		trackingID, err := security.GenerateTrackingID()
		if err != nil {
			t.Fatal(err)
		}
		rawToken, tokenHash, err := security.GenerateReportToken()
		if err != nil {
			t.Fatal(err)
		}
		table := database.ServerTableName(s.dbManager.ServerDB(), "security_reports")
		if _, err := s.dbManager.ServerDB().Exec(context.Background(), fmt.Sprintf("INSERT INTO %s (tracking_id, id_window, severity, component, status, body_enc, body_alg, token_hash, researcher_comment, disclosure_date) VALUES (?, 0, 'low', 'test', 'received', X'', 'none', ?, ?, ?)", table), trackingID, tokenHash, "A researcher-visible update.", "2030-01-02"); err != nil {
			t.Fatalf("insert security report fixture: %v", err)
		}
		t.Cleanup(func() {
			_, _ = s.dbManager.ServerDB().Exec(context.Background(), fmt.Sprintf("DELETE FROM %s WHERE tracking_id = ?", table), trackingID)
		})
		request := newHTMLPageRequest(http.MethodGet, "/server/security/report/"+trackingID+"?token="+url.QueryEscape(rawToken), nil)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("valid security report status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
		}
		assertServerPageConformance(t, "security report status", response.Body.String(), true)
		assertInternalLinksResolve(t, router, "security report status", response.Body.String())
		if response.Header().Get("Referrer-Policy") != "no-referrer" || response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("security report status headers = Referrer-Policy %q, Cache-Control %q; want no-referrer/no-store", response.Header().Get("Referrer-Policy"), response.Header().Get("Cache-Control"))
		}
		for _, required := range []string{"A researcher-visible update.", "2030-01-02"} {
			if !strings.Contains(response.Body.String(), required) {
				t.Errorf("security report status omitted researcher-facing field %q", required)
			}
		}
	})

	t.Run("security report status page rejects invalid one-shot token", func(t *testing.T) {
		request := newHTMLPageRequest(http.MethodGet, "/server/security/report/unknown?token=invalid", nil)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("invalid security report token status = %d, want %d", response.Code, http.StatusNotFound)
		}
		assertServerPageConformance(t, "security report status denial", response.Body.String(), false)
	})
}

func TestSearchEmptyStatePageConformance(t *testing.T) {
	s := newTestServer(t)
	engineConfig := model.NewEngineConfig("empty-page-conformance")
	engineConfig.Categories = []string{"general"}
	base := &queryCaptureEngine{BaseEngine: search.NewBaseEngine(engineConfig), queries: make(chan model.Query, 1)}
	s.aggregator = search.NewAggregator([]search.Engine{emptyPageConformanceEngine{queryCaptureEngine: base}}, search.AggregatorConfig{})
	response := httptest.NewRecorder()
	s.setupRoutes().ServeHTTP(response, newHTMLPageRequest(http.MethodGet, "/search?q=missing+page", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /search empty state status = %d, want %d", response.Code, http.StatusOK)
	}
	html := response.Body.String()
	for _, required := range []string{
		`class="empty-state"`, `class="empty-state-icon"`, `class="empty-state-title"`,
		`class="empty-state-message"`, `class="btn btn-secondary"`,
	} {
		if !strings.Contains(html, required) {
			t.Errorf("empty search state is missing required markup %q", required)
		}
	}
	got := conformanceLinkPath(t, html, "/search")
	parsed, err := url.Parse(got)
	if err != nil || parsed.Path != "/search" || parsed.Query().Get("q") != "missing page" || parsed.Query().Get("engines") != "" || parsed.Query().Get("time_range") != "" {
		t.Errorf("empty search action = %q, want query-preserving link with filters cleared", got)
	}
	if !strings.Contains(html, `<h2 class="empty-state-title">`) {
		t.Error("empty search state must use a heading level directly below the page title")
	}
	if strings.Contains(html, `class="no-results`) {
		t.Error("empty search state uses obsolete page-specific no-results styles")
	}
}

func TestSearchResultCategoryRendering(t *testing.T) {
	categories := []string{"general", "images", "videos", "news", "maps", "files", "music", "science", "it", "social"}
	for _, category := range categories {
		t.Run(category, func(t *testing.T) {
			s := newTestServer(t)
			previousAggregator := s.aggregator
			t.Cleanup(func() { s.aggregator = previousAggregator })
			engineConfig := model.NewEngineConfig("category-page-conformance")
			engineConfig.Categories = categories
			result := model.Result{Title: "Fixture result", URL: "https://example.test/result", Content: "Safe fixture snippet", Engine: "fixture", Category: model.ParseCategory(category), Thumbnail: "https://example.test/image.png", ImageURL: "https://example.test/image.png", EmbedURL: "https://example.test/embed", PreviewURL: "/media/fixture.webm"}
			s.aggregator = search.NewAggregator([]search.Engine{fixedPageConformanceEngine{BaseEngine: search.NewBaseEngine(engineConfig), result: result}}, search.AggregatorConfig{})
			response := httptest.NewRecorder()
			s.setupRoutes().ServeHTTP(response, newHTMLPageRequest(http.MethodGet, "/search?q=fixture&category="+category, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET /search category=%s status = %d", category, response.Code)
			}
			body := response.Body.String()
			want := `class="results-list"`
			switch category {
			case "images":
				want = `class="image-results"`
			case "videos":
				want = `class="video-results"`
			}
			if !strings.Contains(body, want) || !strings.Contains(body, "Fixture result") {
				t.Fatalf("search category %s omitted its result layout %q: %s", category, want, body)
			}
			assertServerPageConformance(t, "/search?category="+category, body, false)
		})
	}
}

func TestInstantAnswerRenderingUsesStandardSearchShell(t *testing.T) {
	s := newTestServer(t)
	previousAggregator, previousInstantManager := s.aggregator, s.instantManager
	t.Cleanup(func() {
		s.aggregator = previousAggregator
		s.instantManager = previousInstantManager
	})
	s.instantManager = instant.NewManager(nil)
	engineConfig := model.NewEngineConfig("instant-page-conformance")
	engineConfig.Categories = []string{"general"}
	s.aggregator = search.NewAggregator([]search.Engine{fixedPageConformanceEngine{BaseEngine: search.NewBaseEngine(engineConfig), result: model.Result{Title: "Fixture result", URL: "https://example.test/result", Engine: "fixture", Category: model.CategoryGeneral}}}, search.AggregatorConfig{})
	response := httptest.NewRecorder()
	s.setupRoutes().ServeHTTP(response, newHTMLPageRequest(http.MethodGet, "/search?q=2+%2B+2", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `class="instant-answer-box"`) {
		t.Fatalf("instant answer did not render in the search page shell: status=%d body=%s", response.Code, response.Body.String())
	}
	assertServerPageConformance(t, "/search instant answer", response.Body.String(), false)
}

func TestDirectAnswerTypesRenderStandardShellAndHeadingHierarchy(t *testing.T) {
	manager := direct.NewManager()
	for _, answerType := range manager.GetHandlerTypes() {
		t.Run(string(answerType), func(t *testing.T) {
			s := newTestServer(t)
			answer := &direct.Answer{
				Type: answerType, Term: "fixture", Title: "Fixture " + string(answerType),
				Content: string(sanitizeOperatorHTML(`<h1>Fixture `+string(answerType)+`</h1><h3>Section</h3><h5>Nested section</h5><p><a href="https://example.test" rel="noopener noreferrer">Safe link</a></p><script>alert(1)</script>`, false)),
			}
			request := newHTMLPageRequest(http.MethodGet, "/direct/"+string(answerType)+"/fixture", nil)
			response := httptest.NewRecorder()
			s.renderDirectAnswer(response, request, answer)
			if response.Code != http.StatusOK {
				t.Fatalf("direct type %s status = %d", answerType, response.Code)
			}
			body := response.Body.String()
			assertServerPageConformance(t, "/direct/"+string(answerType)+"/fixture", body, false)
			assertInternalLinksResolve(t, s.setupRoutes(), "/direct/"+string(answerType)+"/fixture", body)
			if strings.Contains(body, "<script>alert(1)</script>") {
				t.Fatal("sanitized direct fixture retained script markup")
			}
			document, err := xhtml.Parse(strings.NewReader(body))
			if err != nil {
				t.Fatalf("parse direct page: %v", err)
			}
			if countDescendants(document, "h1") != 1 || countDescendants(document, "h2") < 1 || countDescendants(document, "h3") != 1 || countDescendants(document, "h4") != 1 {
				t.Fatalf("direct page %s heading counts: h1=%d h2=%d h3=%d h4=%d", answerType, countDescendants(document, "h1"), countDescendants(document, "h2"), countDescendants(document, "h3"), countDescendants(document, "h4"))
			}
		})
	}
}

type fixedPageConformanceEngine struct {
	*search.BaseEngine
	result model.Result
}

func (e fixedPageConformanceEngine) Search(context.Context, *model.Query) ([]model.Result, error) {
	return []model.Result{e.result}, nil
}

func TestCustomSecurityPolicyPageUsesSanitizedMarkdown(t *testing.T) {
	s := newTestServer(t)
	previousPolicy := s.config.Server.Web.Security.Policy
	defer func() { s.config.Server.Web.Security.Policy = previousPolicy }()
	s.config.Server.Web.Security.Policy = "Custom disclosure policy.\n\n<script>alert(1)</script>\n\n[Safe link](https://policy.example.test)"
	router := s.setupRoutes()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, newHTMLPageRequest(http.MethodGet, "/server/security/policy", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET custom security policy status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	assertServerPageConformance(t, "/server/security/policy custom", body, true)
	if strings.Contains(body, "<script>alert(1)</script>") || strings.Contains(body, "javascript:") || !strings.Contains(body, "Custom disclosure policy.") {
		t.Errorf("custom security policy render safety failed: custom=%v injected-script=%v javascript=%v", strings.Contains(body, "Custom disclosure policy."), strings.Contains(body, "<script>alert(1)</script>"), strings.Contains(body, "javascript:"))
	}
	if !strings.Contains(body, `href="https://policy.example.test" rel="noopener noreferrer nofollow ugc"`) {
		t.Errorf("custom security policy safe Markdown link is missing security attributes: %s", body)
	}
}

func TestPreferencesSyncMalformedFormUsesNoStoreRedirect(t *testing.T) {
	s := newTestServer(t)
	request := httptest.NewRequest(http.MethodPost, "/server/preferences/sync?redirect=%2Fserver%2Fpreferences%2Ftools", strings.NewReader("%"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	s.handlePreferencesSync(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("malformed preference sync POST status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if got := response.Header().Get("Location"); got != "/server/preferences/tools" {
		t.Errorf("malformed preference sync redirect = %q, want validated local path", got)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("malformed preference sync Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
	if strings.Contains(response.Body.String(), "invalid URL escape") || strings.Contains(response.Body.String(), "ParseForm") {
		t.Errorf("malformed preference sync response exposed a parser error: %q", response.Body.String())
	}
}

type emptyPageConformanceEngine struct {
	*queryCaptureEngine
}

func (emptyPageConformanceEngine) Search(context.Context, *model.Query) ([]model.Result, error) {
	return nil, nil
}

func TestPrivacyRetentionUsesResolvedLanguage(t *testing.T) {
	s := newTestServer(t)
	s.config.Server.Privacy.Retention.Period = map[string]any{
		"fr":      "Durée de conservation localisée.",
		"default": "Default retention period.",
	}
	router := s.setupRoutes()
	request := newHTMLPageRequest(http.MethodGet, "/server/privacy?lang=fr", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /server/privacy?lang=fr status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "Durée de conservation localisée.") {
		t.Fatal("privacy page did not use the resolved-language retention text")
	}
}

func TestDPOPageSecurityLinkOnlyWhenNoPublicAddress(t *testing.T) {
	s := newTestServer(t)
	router := s.setupRoutes()
	for _, test := range []struct {
		name       string
		email      string
		wantSecURL bool
	}{
		{name: "email configured", email: "privacy@example.test"},
		{name: "no public address", wantSecURL: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			s.config.Server.Contact.DPO.Email = test.email
			s.config.Server.Contact.General.Email = ""
			response := httptest.NewRecorder()
			router.ServeHTTP(response, newHTMLPageRequest(http.MethodGet, "/server/dpo", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("GET /server/dpo status = %d, want %d", response.Code, http.StatusOK)
			}
			got := strings.Contains(response.Body.String(), `href="/server/security"`)
			if got != test.wantSecURL {
				t.Errorf("security link present = %v, want %v", got, test.wantSecURL)
			}
			if !strings.Contains(response.Body.String(), `href="/server/privacy"`) {
				t.Error("DPO page must always link to the privacy page")
			}
		})
	}
}

func TestDefaultSecurityPolicyIncludesConfiguredScopeAndRequiredTerms(t *testing.T) {
	s := newTestServer(t)
	originalFQDN := s.config.Server.FQDN
	defer func() { s.config.Server.FQDN = originalFQDN }()
	s.config.Server.FQDN = "search.example.test,secondary.example.test"
	router := s.setupRoutes()
	request := newHTMLPageRequest(http.MethodGet, "http://search.example.test/server/security/policy", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /server/security/policy status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, required := range []string{
		"search.example.test", "secondary.example.test", "90 days", "Automated scanner output",
		"Missing best-practice headers", "stops once demonstrated", "/server/security",
	} {
		if !strings.Contains(response.Body.String(), required) {
			t.Errorf("default security policy omitted required content %q", required)
		}
	}
	manager := s.getI18nManager()
	markdown := defaultSecurityPolicyMarkdown("en", manager.T, "search.example.test", []string{"secondary.example.test"}, "example.onion", "example.b32.i2p")
	for _, required := range []string{"example.onion", "example.b32.i2p", "Automated scanner output", "Missing best-practice headers", "/server/security"} {
		if !strings.Contains(markdown, required) {
			t.Errorf("API default security policy omitted required content %q", required)
		}
	}
}

func TestAboutPageKeepsTaglineAndDescriptionDistinct(t *testing.T) {
	s := newTestServer(t)
	originalTagline := s.config.Server.Branding.Tagline
	originalBrandDescription := s.config.Server.Branding.Description
	originalDescription := s.config.Server.Description
	defer func() {
		s.config.Server.Branding.Tagline = originalTagline
		s.config.Server.Branding.Description = originalBrandDescription
		s.config.Server.Description = originalDescription
	}()
	s.config.Server.Branding.Tagline = "Concise tagline"
	s.config.Server.Branding.Description = "Long-form product description."
	s.config.Server.Description = "Fallback server description."
	response := httptest.NewRecorder()
	s.setupRoutes().ServeHTTP(response, newHTMLPageRequest(http.MethodGet, "/server/about", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /server/about status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	if !strings.Contains(body, `<p class="tagline">Concise tagline</p>`) || !strings.Contains(body, `<p class="page-lead">Long-form product description.</p>`) {
		t.Fatal("about page must render configured tagline and description in distinct elements")
	}
	if strings.Contains(body, `tagline">Long-form product description.`) || strings.Contains(body, `<p class="page-lead">Fallback server description.</p>`) {
		t.Fatal("about page reused or replaced the distinct configured branding fields")
	}
}

func TestTermsPageUsesRequiredFiveSectionDefault(t *testing.T) {
	s := newTestServer(t)
	response := httptest.NewRecorder()
	s.setupRoutes().ServeHTTP(response, newHTMLPageRequest(http.MethodGet, "/server/terms", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("GET /server/terms status = %d, want %d", response.Code, http.StatusOK)
	}
	body := response.Body.String()
	for _, required := range []string{
		`id="terms-acceptance-heading"`,
		`id="terms-acceptable-use-heading"`,
		`id="terms-liability-heading"`,
		`id="terms-changes-heading"`,
		`id="terms-governing-law-heading"`,
		`The current text on this page applies from the moment it is published.`,
		`/server/security/policy`,
		`The laws of the United States apply without regard to conflict-of-law rules`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("default terms page omitted required content %q", required)
		}
	}
	if strings.Contains(body, `id="terms-description-heading"`) || strings.Contains(body, `id="terms-user-conduct-heading"`) || strings.Contains(body, `Last updated:`) {
		t.Fatal("default terms page contains legacy sections or an unsupported last-updated claim")
	}
}

func TestHelpOverlayAccessSectionsRequirePublishedAddresses(t *testing.T) {
	s := newTestServer(t)
	originalTorEnabled := s.config.Server.Tor.Enabled
	defer func() { s.config.Server.Tor.Enabled = originalTorEnabled }()
	render := func(t *testing.T, withAddresses bool) string {
		t.Helper()
		request := newHTMLPageRequest(http.MethodGet, "/server/help", nil)
		response := httptest.NewRecorder()
		data := s.newPageData(response, request, "", "help")
		data.Title = s.getI18nManager().T(data.Lang, "help.page_title")
		data.Data = map[string]interface{}{"helpPages": s.helpPages(request), "bangs": s.bangManager.GetAll()}
		s.config.Server.Tor.Enabled = withAddresses
		if withAddresses {
			data.OnionAddress = "exampleonionaddress.onion"
			data.I2PAddress = "exampleb32address.b32.i2p"
		}
		if err := s.renderer.Render(response, "help", data); err != nil {
			t.Fatalf("render help page: %v", err)
		}
		return response.Body.String()
	}

	off := render(t, false)
	if strings.Contains(off, `id="tor-access"`) || strings.Contains(off, `id="i2p-access"`) {
		t.Fatal("overlay access sections must be hidden when no published address is available")
	}
	if strings.Contains(off, `</strong>`) == false || strings.Contains(off, `</strong></span>`) {
		t.Fatal("help privacy list markup must close its strong elements correctly")
	}
	on := render(t, true)
	for _, required := range []string{
		`id="tor-access"`, `id="i2p-access"`, `exampleonionaddress.onion`, `exampleb32address.b32.i2p`,
		`class="code-block"`, `data-copy="exampleonionaddress.onion"`, `data-copy="exampleb32address.b32.i2p"`,
		`id="troubleshooting"`,
	} {
		if !strings.Contains(on, required) {
			t.Errorf("help page with published overlay addresses omitted %q", required)
		}
	}
}

func TestOwnerTokenPageHasPrimaryRevokeConfirmation(t *testing.T) {
	s := newTestServer(t)
	_, token, err := s.ownerManager.IssueWithSource(context.Background(), "issue")
	if err != nil {
		t.Fatalf("IssueWithSource() error = %v", err)
	}
	if _, err := s.ownerManager.Resolve(context.Background(), token); err != nil {
		t.Fatalf("newly issued primary token did not resolve: %v", err)
	}
	router := s.setupRoutes()
	request := newHTMLPageRequest(http.MethodGet, "/server/preferences/tokens", nil)
	request.AddCookie(&http.Cookie{Name: ownerTokenCookie, Value: token})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /server/preferences/tokens status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, required := range []string{
		`id="revoke-primary"`, `id="revoke-primary-popover"`,
		`action="/server/preferences/token/revoke"`, `name="confirm"`,
		`class="responsive-grid responsive-grid-desktop"`,
		`class="responsive-grid responsive-grid-tablet"`,
		`class="section-card owner-token-card responsive-grid-left"`,
		`class="section-card owner-token-linked-column responsive-grid-right"`,
	} {
		if !strings.Contains(response.Body.String(), required) {
			t.Errorf("owner token page is missing %q", required)
		}
	}
	if !strings.Contains(response.Body.String(), token[len(token)-4:]) {
		t.Error("primary confirmation copy does not name the current token's last four characters")
	}
	if !strings.Contains(response.Body.String(), `class="empty-state"`) || !strings.Contains(response.Body.String(), `href="#create-token"`) {
		t.Error("owner token page is missing the linked-token empty state and next action")
	}
	body := response.Body.String()
	orderedCards := []string{`id="primary"`, `id="linked-tokens"`, `id="create-token"`, `id="this-browser"`, `id="revoke-primary"`}
	previous := -1
	for _, marker := range orderedCards {
		position := strings.Index(body, marker)
		if position < 0 || position <= previous {
			t.Errorf("owner token cards do not follow the required order at %s", marker)
			break
		}
		previous = position
	}
}

func TestOwnerTokenCreationUsesOneUseViewCookie(t *testing.T) {
	s := newTestServer(t)
	router := s.setupRoutes()
	initial := httptest.NewRecorder()
	router.ServeHTTP(initial, newHTMLPageRequest(http.MethodGet, "/server/preferences/tokens", nil))
	if initial.Code != http.StatusOK {
		t.Fatalf("GET /server/preferences/tokens status = %d, want %d", initial.Code, http.StatusOK)
	}
	csrfToken := conformanceInputValue(t, initial.Body.String(), "csrf_token")
	csrfCookieName := s.config.Server.Security.CSRF.CookieName
	if csrfCookieName == "" {
		csrfCookieName = "csrf_token"
	}
	var csrfCookie *http.Cookie
	for _, cookie := range initial.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfCookie = cookie
			break
		}
	}
	if csrfCookie == nil {
		t.Fatalf("GET /server/preferences/tokens did not set CSRF cookie %q", csrfCookieName)
	}
	form := url.Values{}
	csrfField := s.config.Server.Security.CSRF.FieldName
	if csrfField == "" {
		csrfField = "csrf_token"
	}
	form.Set(csrfField, csrfToken)
	request := httptest.NewRequest(http.MethodPost, "/server/preferences/token/create", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("POST /server/preferences/token/create status = %d, want %d; body=%q", response.Code, http.StatusSeeOther, response.Body.String())
	}
	if got := response.Header().Get("Location"); got != "/server/preferences/tokens#primary" {
		t.Fatalf("create redirect = %q, want token page primary anchor", got)
	}
	var ownerCookie, viewCookie *http.Cookie
	for _, cookie := range response.Result().Cookies() {
		switch cookie.Name {
		case ownerTokenCookie:
			ownerCookie = cookie
		case ownerTokenViewCookie:
			viewCookie = cookie
		}
	}
	if ownerCookie == nil || ownerCookie.Value == "" || viewCookie == nil {
		t.Fatalf("create response missing owner/view cookies: owner=%v view=%v", ownerCookie != nil, viewCookie != nil)
	}
	if !strings.HasPrefix(viewCookie.Value, "primary.") || viewCookie.Value == ownerCookie.Value || !viewCookie.HttpOnly || viewCookie.SameSite != http.SameSiteStrictMode || viewCookie.Path != "/server/preferences/tokens" || viewCookie.MaxAge != 60 {
		t.Fatalf("invalid one-use view cookie: %+v", viewCookie)
	}
	if strings.Contains(response.Header().Get("Location"), ownerCookie.Value) || strings.Contains(response.Body.String(), ownerCookie.Value) {
		t.Fatal("raw primary token appeared in the redirect or response body")
	}

	viewRequest := newHTMLPageRequest(http.MethodGet, "/server/preferences/tokens", nil)
	viewRequest.AddCookie(ownerCookie)
	viewRequest.AddCookie(viewCookie)
	viewResponse := httptest.NewRecorder()
	router.ServeHTTP(viewResponse, viewRequest)
	if viewResponse.Code != http.StatusOK || !strings.Contains(viewResponse.Body.String(), ownerCookie.Value) {
		t.Fatalf("one-use token page did not render the issued token: status=%d", viewResponse.Code)
	}
	if viewResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("revealed token page Cache-Control = %q, want no-store", viewResponse.Header().Get("Cache-Control"))
	}
	cleared := false
	for _, cookie := range viewResponse.Result().Cookies() {
		if cookie.Name == ownerTokenViewCookie && cookie.MaxAge <= 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("token page did not clear the one-use view cookie")
	}

	tamperedCookie := *viewCookie
	tamperedCookie.Value += "x"
	tamperedRequest := newHTMLPageRequest(http.MethodGet, "/server/preferences/tokens", nil)
	tamperedRequest.AddCookie(ownerCookie)
	tamperedRequest.AddCookie(&tamperedCookie)
	tamperedResponse := httptest.NewRecorder()
	router.ServeHTTP(tamperedResponse, tamperedRequest)
	if tamperedResponse.Code != http.StatusOK || strings.Contains(tamperedResponse.Body.String(), ownerCookie.Value) {
		t.Fatalf("tampered view cookie must not reveal the token: status=%d", tamperedResponse.Code)
	}

	reloadRequest := newHTMLPageRequest(http.MethodGet, "/server/preferences/tokens", nil)
	reloadRequest.AddCookie(ownerCookie)
	reloadResponse := httptest.NewRecorder()
	router.ServeHTTP(reloadResponse, reloadRequest)
	if reloadResponse.Code != http.StatusOK || strings.Contains(reloadResponse.Body.String(), ownerCookie.Value) {
		t.Fatalf("token reload must return to masked value: status=%d", reloadResponse.Code)
	}
}

func TestContactPageTypeSpecificForms(t *testing.T) {
	s := newTestServer(t)
	router := s.setupRoutes()

	for _, test := range []struct {
		path     string
		want     []string
		forbid   []string
		required []string
	}{
		{
			path:   "/server/contact",
			forbid: []string{"name", "email", "type", "reported_url", "reason", "copyrighted_work", "request_kind", "subject", "message", "captcha_answer"},
		},
		{
			path:     "/server/contact?type=abuse",
			want:     []string{"type", "name", "email", "reported_url", "reason", "subject", "message", "captcha_answer"},
			forbid:   []string{"copyrighted_work", "good_faith_statement", "accuracy_statement", "signature", "request_kind"},
			required: []string{"name", "email", "reported_url", "reason", "subject", "message", "captcha_answer"},
		},
		{
			path:     "/server/contact?type=dmca",
			want:     []string{"type", "name", "email", "reported_url", "copyrighted_work", "good_faith_statement", "accuracy_statement", "signature", "subject", "message", "captcha_answer"},
			forbid:   []string{"reason", "request_kind"},
			required: []string{"name", "email", "reported_url", "copyrighted_work", "good_faith_statement", "accuracy_statement", "signature", "subject", "message", "captcha_answer"},
		},
		{
			path:     "/server/contact?type=privacy",
			want:     []string{"type", "name", "email", "request_kind", "subject", "message", "captcha_answer"},
			forbid:   []string{"reported_url", "reason", "copyrighted_work", "good_faith_statement", "accuracy_statement", "signature"},
			required: []string{"name", "email", "request_kind", "subject", "message", "captcha_answer"},
		},
		{
			path:   "/server/contact?type=unknown",
			forbid: []string{"name", "email", "type", "reported_url", "reason", "copyrighted_work", "request_kind", "subject", "message", "captcha_answer"},
		},
	} {
		t.Run(test.path, func(t *testing.T) {
			request := newHTMLPageRequest(http.MethodGet, test.path, nil)
			request.Header.Set("Accept", "text/html")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", test.path, response.Code, http.StatusOK)
			}
			assertContactPageFormFields(t, response.Body.String(), test.want, test.forbid, test.required)
			if test.path == "/server/contact" || test.path == "/server/contact?type=unknown" {
				for _, chooserLink := range []string{
					`href="/server/contact?type=abuse"`,
					`href="/server/contact?type=dmca"`,
					`href="/server/contact?type=privacy"`,
				} {
					if !strings.Contains(response.Body.String(), chooserLink) {
						t.Errorf("contact chooser is missing %s", chooserLink)
					}
				}
				for _, sectionID := range []string{"contact-security-heading", "contact-abuse-heading"} {
					if got := strings.Count(response.Body.String(), `aria-labelledby="`+sectionID+`"`); got != 1 {
						t.Errorf("contact chooser section %q appears %d times, want exactly once", sectionID, got)
					}
				}
			}
			if strings.Contains(test.path, "type=abuse") || strings.Contains(test.path, "type=dmca") {
				if !strings.Contains(response.Body.String(), "https://example.com/...") {
					t.Errorf("GET %s did not show the request host in the reported-URL help text", test.path)
				}
			}
		})
	}
}

func TestContactExternalCaptchaHasNoJavaScriptFallback(t *testing.T) {
	s := newTestServer(t)
	previousCaptcha := s.config.Server.Pages.Contact.Captcha
	previousSiteKey := s.config.Server.Pages.Contact.CaptchaSiteKey
	previousSecret := s.config.Server.Pages.Contact.CaptchaSecret
	defer func() {
		s.config.Server.Pages.Contact.Captcha = previousCaptcha
		s.config.Server.Pages.Contact.CaptchaSiteKey = previousSiteKey
		s.config.Server.Pages.Contact.CaptchaSecret = previousSecret
	}()
	s.config.Server.Pages.Contact.Captcha = "recaptcha"
	s.config.Server.Pages.Contact.CaptchaSiteKey = "public-site-key"
	s.config.Server.Pages.Contact.CaptchaSecret = "server-secret"
	router := s.setupRoutes()
	request := newHTMLPageRequest(http.MethodGet, "/server/contact?type=privacy", nil)
	request.Header.Set("Accept", "text/html")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /server/contact with reCAPTCHA status = %d, want %d", response.Code, http.StatusOK)
	}
	for _, expected := range []string{
		`class="g-recaptcha"`,
		`<noscript>`,
		`name="captcha_id"`,
		`name="captcha_answer"`,
	} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Errorf("reCAPTCHA contact form is missing no-JavaScript fallback element %q", expected)
		}
	}

	form := url.Values{
		"captcha_id":     {conformanceInputValue(t, response.Body.String(), "captcha_id")},
		"captcha_answer": {conformanceCaptchaAnswer(t, response.Body.String())},
	}
	verifyRequest := httptest.NewRequest(http.MethodPost, "/server/contact", strings.NewReader(form.Encode()))
	verifyRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !s.verifyConfiguredContactCaptcha(verifyRequest) {
		t.Error("configured reCAPTCHA mode rejected a valid simple no-JavaScript fallback proof")
	}
}

func conformanceCaptchaAnswer(t *testing.T, body string) string {
	t.Helper()
	question := regexp.MustCompile(`What is (\d+) ([+-]) (\d+)\?`).FindStringSubmatch(stdhtml.UnescapeString(body))
	if len(question) != 4 {
		start := strings.Index(body, "<noscript")
		end := min(len(body), start+300)
		if start < 0 {
			start, end = 0, min(len(body), 300)
		}
		t.Fatalf("contact form has no parseable English arithmetic challenge: %q", body[start:end])
	}
	left, err := strconv.Atoi(question[1])
	if err != nil {
		t.Fatalf("parse CAPTCHA operand: %v", err)
	}
	right, err := strconv.Atoi(question[3])
	if err != nil {
		t.Fatalf("parse CAPTCHA operand: %v", err)
	}
	if question[2] == "-" {
		return strconv.Itoa(left - right)
	}
	return strconv.Itoa(left + right)
}

func TestContactTypeSurvivesReviewEdit(t *testing.T) {
	s := newTestServer(t)
	previousCaptcha := s.config.Server.Pages.Contact.Captcha
	s.config.Server.Pages.Contact.Captcha = "simple"
	defer func() { s.config.Server.Pages.Contact.Captcha = previousCaptcha }()
	s.config.Server.Pages.Contact.Antispam.MinFillSeconds = 0
	router := s.setupRoutes()
	get := httptest.NewRecorder()
	request := newHTMLPageRequest(http.MethodGet, "http://example.com/server/contact?type=dmca", nil)
	router.ServeHTTP(get, request)
	if get.Code != http.StatusOK {
		t.Fatalf("GET DMCA form status = %d, want %d", get.Code, http.StatusOK)
	}
	csrfToken := conformanceInputValue(t, get.Body.String(), "csrf_token")
	csrfCookieName := s.config.Server.Security.CSRF.CookieName
	if csrfCookieName == "" {
		csrfCookieName = "csrf_token"
	}
	var csrfCookie *http.Cookie
	for _, cookie := range get.Result().Cookies() {
		if cookie.Name == csrfCookieName {
			csrfCookie = cookie
			break
		}
	}
	if csrfCookie == nil {
		t.Fatalf("GET DMCA form did not issue %q cookie", csrfCookieName)
	}
	csrfField := s.config.Server.Security.CSRF.FieldName
	if csrfField == "" {
		csrfField = "csrf_token"
	}
	form := url.Values{
		csrfField:              {csrfToken},
		"website":              {""},
		"form_token":           {conformanceInputValue(t, get.Body.String(), "form_token")},
		"captcha_id":           {conformanceInputValue(t, get.Body.String(), "captcha_id")},
		"captcha_answer":       {conformanceCaptchaAnswer(t, get.Body.String())},
		"type":                 {"dmca"},
		"name":                 {"Example Reporter"},
		"email":                {"reporter@example.test"},
		"reported_url":         {"https://example.com/server/security"},
		"copyrighted_work":     {"Example work"},
		"good_faith_statement": {"true"},
		"accuracy_statement":   {"true"},
		"signature":            {"Example Reporter"},
		"subject":              {"Copyright notice"},
		"message":              {"The reported page contains this work."},
	}
	post := func(values url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://example.com/server/contact", strings.NewReader(values.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Accept", "text/html")
		req.AddCookie(csrfCookie)
		req.RemoteAddr = "203.0.113.223:1234"
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}
	review := post(form)
	if review.Code != http.StatusOK || !strings.Contains(review.Body.String(), "Review your report") {
		t.Fatalf("POST DMCA submit status/body = %d/%q, want rendered review", review.Code, review.Body.String())
	}
	editForm := url.Values{}
	for key, values := range form {
		editForm[key] = append([]string(nil), values...)
	}
	editForm.Set("step", "edit")
	edit := post(editForm)
	if edit.Code != http.StatusOK {
		t.Fatalf("POST DMCA edit status = %d, want %d; body=%q", edit.Code, http.StatusOK, edit.Body.String())
	}
	assertContactPageFormFields(t, edit.Body.String(), []string{"type", "name", "email", "reported_url", "copyrighted_work", "good_faith_statement", "accuracy_statement", "signature", "subject", "message", "captcha_answer"}, []string{"reason", "request_kind"}, []string{"name", "email", "reported_url", "copyrighted_work", "good_faith_statement", "accuracy_statement", "signature", "subject", "message", "captcha_answer"})
	if !strings.Contains(edit.Body.String(), `value="Example Reporter"`) {
		t.Error("edited DMCA form did not preserve submitted values")
	}
}

func assertContactPageFormFields(t *testing.T, body string, want, forbid, required []string) {
	t.Helper()
	document, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse contact page: %v", err)
	}
	main := findHTMLTag(document, "main")
	if main == nil {
		t.Fatal("contact page has no main element")
	}
	fields := make(map[string]bool)
	requiredFields := make(map[string]bool)
	missingSharedControls := make([]string, 0)
	forms := 0
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode {
			if node.Data == "form" {
				forms++
			}
			if node.Data == "input" || node.Data == "textarea" || node.Data == "select" {
				inputType := nodeAttribute(node, "type")
				if (node.Data != "input" || (inputType != "hidden" && inputType != "checkbox")) && !strings.Contains(" "+nodeAttribute(node, "class")+" ", " form-input ") {
					missingSharedControls = append(missingSharedControls, nodeAttribute(node, "name"))
				}
				for _, attribute := range node.Attr {
					if attribute.Key == "name" {
						fields[attribute.Val] = true
						if nodeHasAttribute(node, "required") {
							requiredFields[attribute.Val] = true
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(main)
	if len(want) == 0 && forms != 0 {
		t.Errorf("chooser rendered %d forms; want none", forms)
	}
	for _, name := range want {
		if !fields[name] {
			t.Errorf("contact form is missing required field %q", name)
		}
	}
	for _, name := range forbid {
		if fields[name] {
			t.Errorf("contact form rendered forbidden field %q", name)
		}
	}
	for _, name := range required {
		if !requiredFields[name] {
			t.Errorf("contact form field %q is missing the required attribute", name)
		}
	}
	if len(missingSharedControls) > 0 {
		t.Errorf("contact form controls lack the shared form-input class: %s", strings.Join(missingSharedControls, ", "))
	}
}

func findHTMLTag(root *xhtml.Node, tag string) *xhtml.Node {
	if root.Type == xhtml.ElementNode && root.Data == tag {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := findHTMLTag(child, tag); match != nil {
			return match
		}
	}
	return nil
}

func assertHealthPageStructure(t *testing.T, html string) {
	t.Helper()
	if got := strings.Count(html, `class="info-list"`); got != 3 {
		t.Errorf("health page has %d info-list blocks, want 3 for version, runtime and stats", got)
	}
	if !strings.Contains(html, `class="feature-list"`) {
		t.Error("health page is missing the required feature list")
	}
	if !strings.Contains(html, `class="table-wrapper"`) || !strings.Contains(html, `class="data-table"`) {
		t.Error("health checks must use the shared wrapped data table")
	}
	for _, forbidden := range []string{`class="component-list"`, `class="health-info"`, `class="info-row"`} {
		if strings.Contains(html, forbidden) {
			t.Errorf("health page still renders obsolete page-specific structure %q", forbidden)
		}
	}
}

func assertSharedPageComponents(t *testing.T, path, html string) {
	t.Helper()
	var required []string
	switch path {
	case "/":
		required = []string{`class="section-card"`, `class="search-form"`, `class="search-box"`}
	case "/server/about":
		required = []string{`class="section-card"`, `class="hub-list"`, `class="info-list"`}
	case "/server/help":
		required = []string{`class="section-card"`, `class="hub-list"`, `class="hub-link"`}
	case "/server/security/thanks":
		required = []string{`class="section-card"`, `class="empty-state"`, `class="btn btn-secondary"`}
	case "/server/preferences":
		required = []string{`class="form-input"`, `class="toggle"`, `popover-panel`, `class="hub-list"`}
	case "/server/preferences/tokens":
		required = []string{`class="section-card`}
	case "/server/preferences/tools":
		required = []string{`id="custom-bangs-empty-state"`, `class="empty-state-icon"`, `href="#add-custom-bang"`}
	case "/alerts/new":
		required = []string{`aria-labelledby="alert-create-heading"`, `class="form-input"`, `class="btn btn-primary"`}
	}
	for _, component := range required {
		if !strings.Contains(html, component) {
			t.Errorf("GET %s is missing shared component %q", path, component)
		}
	}
}

func assertPreferenceSectionsVisible(t *testing.T, html string) {
	t.Helper()
	document, err := xhtml.Parse(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse preferences page: %v", err)
	}
	sections := 0
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "section" && nodeHasAttribute(node, "data-preference-tab") {
			sections++
			if nodeHasAttribute(node, "hidden") {
				t.Errorf("preferences section %q is hidden in the rendered page", nodeAttribute(node, "aria-labelledby"))
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if sections < 3 {
		t.Errorf("preferences page renders %d tabbed sections, want at least 3 visible sections", sections)
	}
	if strings.Contains(html, `class="preferences-tabs`) {
		t.Error("preferences page still renders tabs that hide preference sections")
	}
}

func nodeAttribute(node *xhtml.Node, name string) string {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return attribute.Val
		}
	}
	return ""
}

func nodeHasAttribute(node *xhtml.Node, name string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == name {
			return true
		}
	}
	return false
}

func conformanceInputValue(t *testing.T, body, name string) string {
	t.Helper()
	document, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse page form: %v", err)
	}
	var value string
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if value != "" {
			return
		}
		if node.Type == xhtml.ElementNode && node.Data == "input" && hasHTMLAttribute(node, "name", name) {
			for _, attribute := range node.Attr {
				if attribute.Key == "value" {
					value = attribute.Val
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if value == "" {
		t.Fatalf("page has no input value for %q", name)
	}
	return value
}

func conformanceLinkPath(t *testing.T, body, prefix string) string {
	t.Helper()
	document, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse page links: %v", err)
	}
	var path string
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if path != "" {
			return
		}
		if node.Type == xhtml.ElementNode && node.Data == "a" {
			for _, attribute := range node.Attr {
				if attribute.Key != "href" {
					continue
				}
				parsed, parseErr := url.Parse(attribute.Val)
				if parseErr == nil && strings.HasPrefix(parsed.Path, prefix) {
					path = parsed.RequestURI()
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
	if path == "" {
		t.Fatalf("page has no link with path prefix %q", prefix)
	}
	return path
}

func TestDemoteDirectContentHeadings(t *testing.T) {
	content, err := demoteDirectContentHeadings(`<h1>Page title</h1><h1>Answer section</h1><h2>Subsection</h2><h5>Deep section</h5>`, "Page title")
	if err != nil {
		t.Fatalf("demote direct content headings: %v", err)
	}
	document, err := xhtml.Parse(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parse normalized direct content: %v", err)
	}
	if countDescendants(document, "h1") != 0 || countDescendants(document, "h2") != 0 {
		t.Fatalf("normalized direct content retained page-level headings: %s", content)
	}
	if countDescendants(document, "h3") != 1 || countDescendants(document, "h4") != 1 || countDescendants(document, "h5") != 1 {
		t.Fatalf("normalized direct content headings are incorrect: %s", content)
	}
	jumped, err := demoteDirectContentHeadings(`<h1>Answer</h1><h3>Section</h3><h5>Nested section</h5>`, "Different page title")
	if err != nil {
		t.Fatalf("normalize skipped direct headings: %v", err)
	}
	jumpedDocument, err := xhtml.Parse(strings.NewReader(jumped))
	if err != nil {
		t.Fatalf("parse normalized skipped headings: %v", err)
	}
	if countDescendants(jumpedDocument, "h3") != 1 || countDescendants(jumpedDocument, "h4") != 1 || countDescendants(jumpedDocument, "h5") != 1 {
		t.Fatalf("generated content heading jumps were not normalized: %s", jumped)
	}
	h2Only, err := demoteDirectContentHeadings(`<h2>Generated section</h2><p>Body</p>`, "Generated answer")
	if err != nil {
		t.Fatalf("demote h2-only direct content: %v", err)
	}
	h2Document, err := xhtml.Parse(strings.NewReader(h2Only))
	if err != nil {
		t.Fatalf("parse normalized h2-only content: %v", err)
	}
	if countDescendants(h2Document, "h2") != 0 || countDescendants(h2Document, "h3") != 1 {
		t.Fatalf("h2-only direct content must begin at h3 inside the card: %s", h2Only)
	}
}

func TestAppJSDoesNotHardcodeAPIVersion(t *testing.T) {
	data, err := EmbeddedFS.ReadFile("static/js/app.js")
	if err != nil {
		t.Fatalf("read embedded app.js: %v", err)
	}
	if strings.Contains(string(data), "/api/v1/") {
		t.Fatal("app.js hardcodes the API version instead of using the SSR API base path")
	}
}

func TestContactAndSecurityPagesUseTranslations(t *testing.T) {
	s := newTestServer(t)
	router := s.setupRoutes()

	for _, tc := range []struct {
		path string
		want []string
		bad  []string
	}{
		{
			path: "/server/contact?lang=es",
			want: []string{"Este formulario es solo", "Informar de abuso"},
			bad:  []string{"This form is only", "Report abuse or a terms violation"},
		},
		{
			path: "/server/security?lang=es",
			want: []string{"Informar de abuso o de una infracción", "Enviar un aviso DMCA", "Enviar una solicitud de privacidad"},
			bad:  []string{"Report abuse or a terms violation", "Send a DMCA notice", "Submit a privacy request"},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			request := newHTMLPageRequest(http.MethodGet, tc.path, nil)
			request.Header.Set("Accept", "text/html")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d, want %d", tc.path, response.Code, http.StatusOK)
			}
			for _, expected := range tc.want {
				if !strings.Contains(response.Body.String(), expected) {
					t.Errorf("GET %s is missing translated text %q", tc.path, expected)
				}
			}
			for _, unexpected := range tc.bad {
				if strings.Contains(response.Body.String(), unexpected) {
					t.Errorf("GET %s still contains untranslated text %q", tc.path, unexpected)
				}
			}
		})
	}
}

func assertInternalLinksResolve(t *testing.T, router http.Handler, sourcePath, body string) {
	t.Helper()
	document, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse links from %s: %v", sourcePath, err)
	}
	requestIndex := 0
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "a" {
			for _, attribute := range node.Attr {
				if attribute.Key != "href" || attribute.Val == "" {
					continue
				}
				target, parseErr := url.Parse(attribute.Val)
				if parseErr != nil || target.IsAbs() || target.Host != "" || strings.HasPrefix(attribute.Val, "#") || target.Path == "" {
					continue
				}
				request := newHTMLPageRequest(http.MethodGet, target.RequestURI(), nil)
				request.RemoteAddr = fmt.Sprintf("203.0.%d.%d:1234", requestIndex/256, requestIndex%256)
				requestIndex++
				request.Header.Set("Accept", "text/html")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				// This fixture's health check is intentionally degraded; its 503 confirms the linked route resolves.
				expectedDegradedHealth := target.Path == "/server/healthz" && response.Code == http.StatusServiceUnavailable
				expectedEmptySearchValidation := target.Path == "/search" && target.Query().Get("q") == "" && response.Code == http.StatusBadRequest
				if response.Code >= http.StatusBadRequest && !expectedDegradedHealth && !expectedEmptySearchValidation {
					t.Errorf("internal link %q from %s returned HTTP %d", attribute.Val, sourcePath, response.Code)
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(document)
}

func findHTMLNode(root *xhtml.Node, tag, attribute, value string) *xhtml.Node {
	if root.Type == xhtml.ElementNode && root.Data == tag && hasHTMLAttribute(root, attribute, value) {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := findHTMLNode(child, tag, attribute, value); match != nil {
			return match
		}
	}
	return nil
}

func hasHTMLAttribute(node *xhtml.Node, name, value string) bool {
	for _, attribute := range node.Attr {
		if attribute.Key == name && attribute.Val == value {
			return true
		}
	}
	return false
}

func assertServerPageConformance(t *testing.T, path, body string, subpage bool) {
	t.Helper()
	document, err := xhtml.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse %s HTML: %v", path, err)
	}
	if findHTMLNode(document, "html", "data-api-base", version.APIPrefix) == nil {
		t.Errorf("%s HTML does not expose the configured API base path %q", path, version.APIPrefix)
	}
	assertHeaderConformance(t, path, document, false)
	mainCount, h1Count, titleCount, containerCount, breadcrumbCount := 0, 0, 0, 0, 0
	csrfForms := make([]bool, 0)
	labelIDs := make(map[string]bool)
	tables := make([]*xhtml.Node, 0)
	cards := make([]*xhtml.Node, 0)
	styles, inlineHandlers := 0, 0
	inlineScripts := 0
	focusableLinks := make([]string, 0, 2)
	missingImageAlt := 0
	type formControl struct {
		id             string
		ariaLabel      string
		ariaLabelledBy string
		insideLabel    bool
	}
	controls := make([]formControl, 0)
	var walk func(*xhtml.Node, bool, bool)
	walk = func(node *xhtml.Node, inForm, insideLabel bool) {
		if node.Type == xhtml.ElementNode {
			insideLabel = insideLabel || node.Data == "label"
			switch node.Data {
			case "main":
				mainCount++
			case "h1":
				h1Count++
				for _, attr := range node.Attr {
					if attr.Key == "class" && hasClass(attr.Val, "page-title") {
						titleCount++
					}
				}
			case "style":
				styles++
			case "script":
				hasSource, isJSONLD := false, false
				for _, attr := range node.Attr {
					if attr.Key == "src" {
						hasSource = true
					}
					if attr.Key == "type" && attr.Val == "application/ld+json" {
						isJSONLD = true
					}
				}
				if !hasSource && !isJSONLD && strings.TrimSpace(textContent(node)) != "" {
					inlineScripts++
				}
			case "table":
				tables = append(tables, node)
			case "img":
				altPresent := false
				for _, attr := range node.Attr {
					if attr.Key == "alt" {
						altPresent = true
						break
					}
				}
				if !altPresent {
					missingImageAlt++
				}
			case "label":
				for _, attr := range node.Attr {
					if attr.Key == "for" {
						labelIDs[attr.Val] = true
					}
				}
			case "form":
				method := "get"
				for _, attr := range node.Attr {
					if attr.Key == "method" {
						method = strings.ToLower(attr.Val)
					}
				}
				csrfForms = append(csrfForms, method != http.MethodPost)
				inForm = true
			case "input", "select", "textarea":
				if inForm {
					name, inputType, id, ariaLabel, ariaLabelledBy := "", "", "", "", ""
					for _, attr := range node.Attr {
						switch attr.Key {
						case "name":
							name = attr.Val
						case "type":
							inputType = strings.ToLower(attr.Val)
						case "id":
							id = attr.Val
						case "aria-label":
							ariaLabel = attr.Val
						case "aria-labelledby":
							ariaLabelledBy = attr.Val
						}
					}
					if name == "csrf_token" {
						for index := range csrfForms {
							if !csrfForms[index] {
								csrfForms[index] = true
								break
							}
						}
					}
					if node.Data != "input" || (inputType != "hidden" && inputType != "submit" && inputType != "button") {
						controls = append(controls, formControl{id: id, ariaLabel: ariaLabel, ariaLabelledBy: ariaLabelledBy, insideLabel: insideLabel})
					}
				}
			}
			for _, attr := range node.Attr {
				if attr.Key != "class" {
					continue
				}
				if hasClass(attr.Val, "container") {
					containerCount++
					widths := 0
					for _, modifier := range []string{"container-sm", "container-md", "container-lg"} {
						if hasClass(attr.Val, modifier) {
							widths++
						}
					}
					if widths != 1 {
						t.Errorf("%s container has %d width modifiers, want one", path, widths)
					}
				}
				if hasClass(attr.Val, "breadcrumb") {
					breadcrumbCount++
				}
				if hasClass(attr.Val, "section-card") {
					cards = append(cards, node)
				}
			}
			for _, attr := range node.Attr {
				if attr.Key == "style" {
					styles++
				}
				if strings.HasPrefix(strings.ToLower(attr.Key), "on") {
					inlineHandlers++
				}
			}
			if node.Data == "a" {
				for _, attr := range node.Attr {
					if attr.Key == "href" && len(focusableLinks) < 2 {
						focusableLinks = append(focusableLinks, attr.Val)
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, inForm, insideLabel)
		}
	}
	walk(document, false, false)
	if mainCount != 1 {
		t.Errorf("%s has %d main elements, want one", path, mainCount)
	}
	if h1Count != 1 {
		t.Errorf("%s has %d h1 headings, want exactly one", path, h1Count)
	}
	if titleCount != 1 {
		t.Errorf("%s has %d page-title headings, want one", path, titleCount)
	}
	if containerCount != 1 {
		t.Errorf("%s has %d content containers, want one", path, containerCount)
	}
	wantBreadcrumbs := 0
	if subpage {
		wantBreadcrumbs = 1
	}
	if breadcrumbCount != wantBreadcrumbs {
		t.Errorf("%s has %d breadcrumbs, want %d", path, breadcrumbCount, wantBreadcrumbs)
	}
	if styles != 0 {
		t.Errorf("%s contains %d inline style attributes/elements", path, styles)
	}
	if inlineHandlers != 0 {
		t.Errorf("%s contains %d inline event handlers", path, inlineHandlers)
	}
	if inlineScripts != 0 {
		t.Errorf("%s contains %d executable inline scripts", path, inlineScripts)
	}
	if missingImageAlt != 0 {
		t.Errorf("%s contains %d images without alt attributes", path, missingImageAlt)
	}
	if len(focusableLinks) < 2 || focusableLinks[0] != "#main-content" || focusableLinks[1] != "#navigation" {
		t.Errorf("%s first focusable links = %v, want skip links to #main-content then #navigation", path, focusableLinks)
	}
	for index, formHasCSRF := range csrfForms {
		if !formHasCSRF {
			t.Errorf("%s POST form %d has no csrf_token", path, index+1)
		}
	}
	for _, control := range controls {
		if strings.TrimSpace(control.ariaLabel) == "" && strings.TrimSpace(control.ariaLabelledBy) == "" && !control.insideLabel && (control.id == "" || !labelIDs[control.id]) {
			t.Errorf("%s form control id %q has no accessible label", path, control.id)
		}
	}
	for _, table := range tables {
		if !hasDescendant(table, "thead") {
			t.Errorf("%s has a table without a thead", path)
		}
		if !hasClassAncestor(table, "table-wrapper") {
			t.Errorf("%s has a table outside .table-wrapper", path)
		}
	}
	for _, card := range cards {
		if countDescendants(card, "h2") != 1 {
			t.Errorf("%s section card has %d h2 headings, want exactly one", path, countDescendants(card, "h2"))
		}
	}
	if strings.HasPrefix(path, "/alerts") {
		for _, forbiddenClass := range []string{"page-section", "alert-page"} {
			if findHTMLClassNode(document, forbiddenClass) != nil {
				t.Errorf("%s rendered a page-specific wrapper class %q", path, forbiddenClass)
			}
		}
	}
}

func countDescendants(root *xhtml.Node, tag string) int {
	count := 0
	if root.Type == xhtml.ElementNode && root.Data == tag {
		count++
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		count += countDescendants(child, tag)
	}
	return count
}

func findHTMLClassNode(root *xhtml.Node, class string) *xhtml.Node {
	if root.Type == xhtml.ElementNode {
		for _, attribute := range root.Attr {
			if attribute.Key == "class" && hasClass(attribute.Val, class) {
				return root
			}
		}
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if match := findHTMLClassNode(child, class); match != nil {
			return match
		}
	}
	return nil
}

func assertHeaderConformance(t *testing.T, path string, document *xhtml.Node, hasConfiguredFragment bool) {
	t.Helper()
	header := findHTMLNode(document, "header", "class", "header")
	if header == nil {
		t.Errorf("%s has no shared public header", path)
		return
	}
	children := make([]*xhtml.Node, 0, 5)
	for child := header.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode {
			children = append(children, child)
		}
	}
	wantCount := 4
	if hasConfiguredFragment {
		wantCount++
	}
	if len(children) != wantCount {
		t.Errorf("%s header has %d element children, want %d", path, len(children), wantCount)
	}
	if len(children) < 4 {
		return
	}
	if children[0].Data != "a" || !hasHTMLAttribute(children[0], "class", "site-brand") {
		t.Errorf("%s header child 1 is not the brand link", path)
	}
	if children[1].Data != "nav" || !hasHTMLAttribute(children[1], "class", "nav-links") {
		t.Errorf("%s header child 2 is not the application navigation", path)
	}
	if children[2].Data != "label" || !hasHTMLAttribute(children[2], "class", "nav-toggle") {
		t.Errorf("%s header child 3 is not the navigation toggle label", path)
	}
	if children[3].Data != "div" || !hasHTMLAttribute(children[3], "class", "header-actions") {
		t.Errorf("%s header child 4 is not the header actions group", path)
	}
	actions := make([]*xhtml.Node, 0, 2)
	for child := children[3].FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode {
			actions = append(actions, child)
		}
	}
	if len(actions) != 2 || actions[0].Data != "a" || !hasHTMLAttribute(actions[0], "href", "/server/preferences") || actions[1].Data != "form" || !hasHTMLAttribute(actions[1], "class", "header-theme-form") || !hasHTMLAttribute(actions[1], "action", "/theme") {
		t.Errorf("%s header actions must contain only the preferences link followed by the theme form", path)
	}
	if hasConfiguredFragment && len(children) >= 5 && !hasHTMLAttribute(children[4], "id", "configured-header-fragment") {
		t.Errorf("%s configured branding content is not the last header child", path)
	}
}

func headerCoreSignature(header *xhtml.Node) string {
	var signature strings.Builder
	count := 0
	for child := header.FirstChild; child != nil && count < 4; child = child.NextSibling {
		if child.Type != xhtml.ElementNode {
			continue
		}
		appendHeaderNodeSignature(&signature, child)
		count++
	}
	return signature.String()
}

func appendHeaderNodeSignature(signature *strings.Builder, node *xhtml.Node) {
	if node.Type == xhtml.TextNode {
		signature.WriteString(node.Data)
		return
	}
	if node.Type != xhtml.ElementNode {
		return
	}
	signature.WriteByte('<')
	signature.WriteString(node.Data)
	for _, attribute := range node.Attr {
		if node.Data == "input" && attribute.Key == "value" && hasHTMLAttribute(node, "name", "csrf_token") {
			continue
		}
		signature.WriteByte(' ')
		signature.WriteString(attribute.Key)
		signature.WriteByte('=')
		signature.WriteString(attribute.Val)
	}
	signature.WriteByte('>')
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		appendHeaderNodeSignature(signature, child)
	}
	signature.WriteString("</")
	signature.WriteString(node.Data)
	signature.WriteByte('>')
}

func textContent(node *xhtml.Node) string {
	var content strings.Builder
	var walk func(*xhtml.Node)
	walk = func(current *xhtml.Node) {
		if current.Type == xhtml.TextNode {
			content.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return content.String()
}

func hasClass(value, class string) bool {
	for _, candidate := range strings.Fields(value) {
		if candidate == class {
			return true
		}
	}
	return false
}

func hasDescendant(node *xhtml.Node, tag string) bool {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == xhtml.ElementNode && child.Data == tag || hasDescendant(child, tag) {
			return true
		}
	}
	return false
}

func hasClassAncestor(node *xhtml.Node, class string) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type != xhtml.ElementNode {
			continue
		}
		for _, attr := range parent.Attr {
			if attr.Key == "class" && hasClass(attr.Val, class) {
				return true
			}
		}
	}
	return false
}
