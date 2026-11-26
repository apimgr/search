//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestPublicPagesContentNegotiation(t *testing.T) {
	site := newTestSite(t)
	paths := []string{
		"/",
		"/search?q=negotiation",
		"/server/about",
		"/server/privacy",
		"/server/dpo",
		"/server/help",
		"/server/help/mcp",
		"/server/terms",
		"/server/contact",
		"/server/security",
		"/server/security/policy",
		"/server/security/thanks",
		"/server/healthz",
		"/server/preferences",
	}
	for _, path := range paths {
		for _, format := range []struct {
			name, accept, contentType string
		}{
			{name: "html", accept: "text/html", contentType: "text/html"},
			{name: "json", accept: "application/json", contentType: "application/json"},
			{name: "text", accept: "text/plain", contentType: "text/plain"},
		} {
			t.Run(fmt.Sprintf("%s/%s", path, format.name), func(t *testing.T) {
				request, err := http.NewRequest(http.MethodGet, site.URL+path, nil)
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Accept", format.accept)
				response, err := http.DefaultClient.Do(request)
				if err != nil {
					t.Fatalf("GET %s: %v", path, err)
				}
				defer response.Body.Close()
				wantStatus := http.StatusOK
				if path == "/server/healthz" {
					wantStatus = http.StatusServiceUnavailable
				}
				if response.StatusCode != wantStatus {
					t.Fatalf("GET %s with Accept %q: status %d, want %d", path, format.accept, response.StatusCode, wantStatus)
				}
				if got := response.Header.Get("Content-Type"); !strings.HasPrefix(got, format.contentType) {
					t.Errorf("GET %s with Accept %q: Content-Type %q, want prefix %q", path, format.accept, got, format.contentType)
				}
			})
		}
	}
}
