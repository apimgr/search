package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/go-chi/chi/v5"
)

func newMCPTestRouter(cfg *config.Config) chi.Router {
	handler := NewHandler(cfg, nil, nil)
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	return router
}

func TestMCPInitializeAndToolsList(t *testing.T) {
	cfg := config.DefaultConfig()
	router := newMCPTestRouter(cfg)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/server/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var payload struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range payload.Result.Tools {
		if tool.Name == "getApiV1Search" {
			found = true
		}
	}
	if !found {
		t.Fatalf("search operation missing from MCP tools: %#v", payload.Result.Tools)
	}
	for _, tool := range payload.Result.Tools {
		if strings.Contains(tool.Name, "ServerPreferencesTokens") {
			t.Fatalf("token management leaked into MCP tools: %s", tool.Name)
		}
	}
}

func TestMCPRejectsOriginHeaderMismatchAndDisabledEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		configure      func(*config.Config)
		origin, method string
		want           int
	}{
		{name: "origin", origin: "https://evil.example", want: http.StatusForbidden},
		{name: "header mismatch", method: "tools/call", want: http.StatusBadRequest},
		{name: "disabled", configure: func(cfg *config.Config) { cfg.Server.MCP.Enabled = false }, want: http.StatusNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			if test.configure != nil {
				test.configure(cfg)
			}
			body := `{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}`
			request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.method != "" {
				request.Header.Set("Mcp-Method", test.method)
			}
			response := httptest.NewRecorder()
			newMCPTestRouter(cfg).ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}
