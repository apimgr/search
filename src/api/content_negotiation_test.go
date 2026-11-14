package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestContentNegotiationMiddlewareFormatsTxtAlias(t *testing.T) {
	var routedPath string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true,"data":"hello"}`))
	})
	middleware := (&Handler{}).ContentNegotiationMiddleware(next)

	request := httptest.NewRequest(http.MethodGet, APIPrefix+"/example.txt", nil)
	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, request)

	if routedPath != APIPrefix+"/example" {
		t.Fatalf("routed path = %q, want .txt-stripped API path", routedPath)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", contentType)
	}
	if response.Body.String() != "hello\n" {
		t.Fatalf("body = %q, want raw data with trailing newline", response.Body.String())
	}
}

func TestContentNegotiationMiddlewareConvertsStructuredDataToText(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":{"count":2,"name":"search"}}`))
	})
	middleware := (&Handler{}).ContentNegotiationMiddleware(next)

	request := httptest.NewRequest(http.MethodGet, APIPrefix+"/example", nil)
	request.Header.Set("Accept", "text/plain")
	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, request)

	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", contentType)
	}
	if !strings.Contains(response.Body.String(), `"count": 2`) || !strings.Contains(response.Body.String(), `"name": "search"`) {
		t.Fatalf("structured response lost data fields: %s", response.Body.String())
	}
}

func TestContentNegotiationMiddlewareExplicitJSONBeatsHTTPTool(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"data":"hello"}`))
	})
	middleware := (&Handler{}).ContentNegotiationMiddleware(next)

	request := httptest.NewRequest(http.MethodGet, APIPrefix+"/example", nil)
	request.Header.Set("User-Agent", "curl/8.0")
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	middleware.ServeHTTP(response, request)

	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	if response.Body.String() != `{"ok":true,"data":"hello"}` {
		t.Fatalf("JSON response changed: %s", response.Body.String())
	}
}
