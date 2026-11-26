package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/security"
	"github.com/apimgr/search/src/version"
	xhtml "golang.org/x/net/html"
)

func TestServerPageConformance(t *testing.T) {
	s := newTestServer(t)
	router := s.setupRoutes()
	paths := []string{
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
	for _, page := range s.helpPages(httptest.NewRequest(http.MethodGet, "/server/help", nil)) {
		paths = append(paths, page.Path)
		subpages[page.Path] = true
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			request.Header.Set("Accept", "text/html")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK && (path != "/server/healthz" || response.Code != http.StatusServiceUnavailable) {
				t.Fatalf("GET %s status = %d, want %d; body=%q", path, response.Code, http.StatusOK, response.Body.String())
			}
			assertServerPageConformance(t, path, response.Body.String(), subpages[path])
			assertInternalLinksResolve(t, router, path, response.Body.String())
		})
	}

	t.Run("configured header content is the final header child", func(t *testing.T) {
		original := s.config.Server.Branding.Content
		defer func() { s.config.Server.Branding.Content = original }()
		s.config.Server.Branding.Content = nil
		baselineRequest := httptest.NewRequest(http.MethodGet, "/server/about", nil)
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
		request := httptest.NewRequest(http.MethodGet, "/server/about", nil)
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
		{path: "/server/help", pages: s.helpPages(httptest.NewRequest(http.MethodGet, "/server/help", nil))},
	} {
		t.Run("hub registry "+hub.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, hub.path, nil)
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
		request := httptest.NewRequest(http.MethodGet, "/server/missing-page", nil)
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
			request := httptest.NewRequest(http.MethodGet, "/server/error-test", nil)
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
		if _, err := s.dbManager.ServerDB().Exec(context.Background(), fmt.Sprintf("INSERT INTO %s (tracking_id, id_window, severity, component, status, body_enc, body_alg, token_hash) VALUES (?, 0, 'low', 'test', 'received', X'', 'none', ?)", table), trackingID, tokenHash); err != nil {
			t.Fatalf("insert security report fixture: %v", err)
		}
		t.Cleanup(func() {
			_, _ = s.dbManager.ServerDB().Exec(context.Background(), fmt.Sprintf("DELETE FROM %s WHERE tracking_id = ?", table), trackingID)
		})
		request := httptest.NewRequest(http.MethodGet, "/server/security/report/"+trackingID+"?token="+url.QueryEscape(rawToken), nil)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("valid security report status = %d, want %d; body=%q", response.Code, http.StatusOK, response.Body.String())
		}
		assertServerPageConformance(t, "security report status", response.Body.String(), true)
		assertInternalLinksResolve(t, router, "security report status", response.Body.String())
	})

	t.Run("security report status page rejects invalid one-shot token", func(t *testing.T) {
		request := httptest.NewRequest(http.MethodGet, "/server/security/report/unknown?token=invalid", nil)
		request.Header.Set("Accept", "text/html")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("invalid security report token status = %d, want %d", response.Code, http.StatusNotFound)
		}
		assertServerPageConformance(t, "security report status denial", response.Body.String(), false)
	})
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
			want: []string{"Este formulario es solo", "Tipo", "Informar de abuso"},
			bad:  []string{"This form is only", "Report abuse or a terms violation"},
		},
		{
			path: "/server/security?lang=es",
			want: []string{"Informar de abuso o de una infracción", "Enviar un aviso DMCA", "Enviar una solicitud de privacidad"},
			bad:  []string{"Report abuse or a terms violation", "Send a DMCA notice", "Submit a privacy request"},
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tc.path, nil)
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
				request := httptest.NewRequest(http.MethodGet, target.RequestURI(), nil)
				request.RemoteAddr = fmt.Sprintf("203.0.%d.%d:1234", requestIndex/256, requestIndex%256)
				requestIndex++
				request.Header.Set("Accept", "text/html")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				// This fixture's health check is intentionally degraded; its 503 confirms the linked route resolves.
				expectedDegradedHealth := target.Path == "/server/healthz" && response.Code == http.StatusServiceUnavailable
				if response.Code >= http.StatusBadRequest && !expectedDegradedHealth {
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
	mainCount, titleCount, containerCount, breadcrumbCount := 0, 0, 0, 0
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
		if !hasDescendant(card, "h2") {
			t.Errorf("%s has a section card without an h2 heading", path)
		}
	}
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
