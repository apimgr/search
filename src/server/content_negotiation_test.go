package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestFrontendAPIPath(t *testing.T) {
	tests := []struct {
		path string
		want string
		ok   bool
	}{
		{path: "/", want: "/api/v1/info", ok: true},
		{path: "/search", want: "/api/v1/search", ok: true},
		{path: "/server/security/policy", want: "/api/v1/server/security/policy", ok: true},
		{path: "/direct/math:2+2", want: "/api/v1/direct/math:2+2", ok: true},
		{path: "/server/security/report/sec_123", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got, ok := frontendAPIPath(tt.path)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("frontendAPIPath(%q) = (%q, %t), want (%q, %t)", tt.path, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestFrontendContentNegotiation(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/api/v1/server/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"title":"About"}}`))
	})
	s := &Server{router: router}
	page := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body><h1>About</h1><p>Search service</p></body></html>"))
	})
	handler := s.contentNegotiationMiddleware(page)

	jsonRequest := httptest.NewRequest(http.MethodGet, "/server/about", nil)
	jsonRequest.Header.Set("Accept", "application/json")
	jsonResponse := httptest.NewRecorder()
	handler.ServeHTTP(jsonResponse, jsonRequest)
	if jsonResponse.Code != http.StatusOK || jsonResponse.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("JSON negotiation response = %d %q", jsonResponse.Code, jsonResponse.Header().Get("Content-Type"))
	}

	textRequest := httptest.NewRequest(http.MethodGet, "/server/about", nil)
	textRequest.Header.Set("Accept", "text/plain")
	textResponse := httptest.NewRecorder()
	handler.ServeHTTP(textResponse, textRequest)
	if textResponse.Code != http.StatusOK || textResponse.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("text negotiation response = %d %q", textResponse.Code, textResponse.Header().Get("Content-Type"))
	}
	if textResponse.Body.String() == "" || textResponse.Body.String()[0] == '<' {
		t.Fatalf("text negotiation returned HTML: %q", textResponse.Body.String())
	}
}
