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

func validMCPRequest(cfg *config.Config, path, body string) *http.Request {
	var payload mcpRequest
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		panic(err)
	}
	if payload.Params == nil {
		payload.Params = make(map[string]json.RawMessage)
	}
	meta, _ := json.Marshal(map[string]string{"io.modelcontextprotocol/protocolVersion": mcpProtocolVersion})
	payload.Params["_meta"], _ = json.Marshal(json.RawMessage(meta))
	encoded, _ := json.Marshal(payload)
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(encoded)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+cfg.Get().Token)
	request.Header.Set("MCP-Protocol-Version", mcpProtocolVersion)
	request.Header.Set("Mcp-Method", payload.Method)
	if raw, ok := payload.Params["name"]; ok {
		var name string
		if json.Unmarshal(raw, &name) == nil {
			request.Header.Set("Mcp-Name", name)
		}
	}
	if raw, ok := payload.Params["uri"]; ok {
		var uri string
		if json.Unmarshal(raw, &uri) == nil {
			request.Header.Set("Mcp-Name", uri)
		}
	}
	return request
}

func TestMCPToolsList(t *testing.T) {
	cfg := config.DefaultConfig()
	router := newMCPTestRouter(cfg)
	request := validMCPRequest(cfg, "/api/v1/server/mcp", `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
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

func TestMCPToolAnnotationsFollowHTTPMethod(t *testing.T) {
	tests := []struct {
		method string
		want   map[string]bool
	}{
		{method: http.MethodGet, want: map[string]bool{"readOnlyHint": true}},
		{method: http.MethodPost, want: map[string]bool{"readOnlyHint": false}},
		{method: http.MethodPatch, want: map[string]bool{"readOnlyHint": false}},
		{method: http.MethodPut, want: map[string]bool{"idempotentHint": true}},
		{method: http.MethodDelete, want: map[string]bool{"destructiveHint": true, "idempotentHint": true}},
	}
	for _, test := range tests {
		got := mcpToolAnnotations(test.method)
		if len(got) != len(test.want) {
			t.Fatalf("annotations for %s = %#v, want %#v", test.method, got, test.want)
		}
		for key, value := range test.want {
			if got[key] != value {
				t.Errorf("annotations for %s = %#v, want %s=%t", test.method, got, key, value)
			}
		}
	}
}

func TestMCPRejectsOriginAndMethodHeaderMismatch(t *testing.T) {
	tests := []struct {
		name           string
		origin, method string
		want           int
	}{
		{name: "origin", origin: "https://evil.example", want: http.StatusForbidden},
		{name: "header mismatch", method: "tools/call", want: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			body := `{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}`
			request := validMCPRequest(cfg, "/mcp", body)
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

func TestMCPEnforcesVersionHeadersAndUnknownMethodStatus(t *testing.T) {
	tests := []struct {
		name       string
		prepare    func(*http.Request)
		body       string
		wantStatus int
		wantCode   int
	}{
		{
			name:       "older protocol version is rejected",
			body:       `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   -32000,
		},
		{
			name:       "missing method header is rejected",
			body:       `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   -32020,
			prepare:    func(request *http.Request) { request.Header.Del("Mcp-Method") },
		},
		{
			name:       "unknown method is not found",
			body:       `{"jsonrpc":"2.0","id":3,"method":"unknown/method","params":{}}`,
			wantStatus: http.StatusNotFound,
			wantCode:   -32601,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			request := validMCPRequest(cfg, "/mcp", test.body)
			if test.name == "older protocol version is rejected" {
				request.Header.Set("MCP-Protocol-Version", "2025-11-25")
			}
			if test.prepare != nil {
				test.prepare(request)
			}
			response := httptest.NewRecorder()
			newMCPTestRouter(cfg).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			var payload struct {
				Error struct {
					Code int `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Error.Code != test.wantCode {
				t.Fatalf("error code = %d, want %d; body=%s", payload.Error.Code, test.wantCode, response.Body.String())
			}
		})
	}
}

func TestMCPRequiresBearerAuthentication(t *testing.T) {
	for _, credential := range []string{"", "Bearer invalid-token"} {
		t.Run(map[bool]string{true: "invalid", false: "missing"}[credential != ""], func(t *testing.T) {
			cfg := config.DefaultConfig()
			request := validMCPRequest(cfg, "/mcp", `{"jsonrpc":"2.0","id":9,"method":"tools/list","params":{}}`)
			request.Header.Set("Authorization", credential)
			response := httptest.NewRecorder()
			newMCPTestRouter(cfg).ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
			}
			if response.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("missing WWW-Authenticate challenge")
			}
			var payload struct {
				JSONRPC string `json:"jsonrpc"`
				ID      int    `json:"id"`
				Error   any    `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.JSONRPC != "2.0" || payload.ID != 9 || payload.Error == nil {
				t.Fatalf("authentication response = %#v", payload)
			}
		})
	}
}

func TestMCPLegacyInitializeIsRejectedWithoutNegotiation(t *testing.T) {
	cfg := config.DefaultConfig()
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":11,"method":"initialize","params":{"protocolVersion":"2025-11-25"}}`))
	request.Header.Set("Authorization", "Bearer "+cfg.Get().Token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	newMCPTestRouter(cfg).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	var payload struct {
		Error struct {
			Data struct {
				Name string `json:"name"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Data.Name != "UnsupportedProtocolVersionError" {
		t.Fatalf("legacy initialize error = %#v", payload.Error)
	}
}

func TestMCPUsesCanonicalErrorsUntilJSONRPCRequestParses(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		body       string
		origin     string
		wantStatus int
		wantRPC    bool
	}{
		{name: "malformed JSON uses canonical error", method: http.MethodPost, body: `{`, wantStatus: http.StatusBadRequest},
		{name: "oversized request uses canonical error", method: http.MethodPost, body: strings.Repeat("x", (1<<20)+1), wantStatus: http.StatusRequestEntityTooLarge},
		{name: "invalid origin uses JSON-RPC error after parse", method: http.MethodPost, body: `{"jsonrpc":"2.0","id":7,"method":"ping","params":{}}`, origin: "https://evil.example", wantStatus: http.StatusForbidden, wantRPC: true},
		{name: "unsupported method uses canonical error", method: http.MethodGet, wantStatus: http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			request := httptest.NewRequest(test.method, "/mcp", strings.NewReader(test.body))
			if test.wantRPC {
				request = validMCPRequest(cfg, "/mcp", test.body)
			}
			if test.method == http.MethodPost && !test.wantRPC && test.name != "malformed JSON uses canonical error" && test.name != "oversized request uses canonical error" {
				request.Header.Set("Authorization", "Bearer "+cfg.Get().Token)
			}
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			newMCPTestRouter(cfg).ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
				t.Fatalf("Content-Type = %q, want JSON", contentType)
			}
			if test.wantRPC {
				var payload struct {
					JSONRPC string `json:"jsonrpc"`
					ID      int    `json:"id"`
					Error   any    `json:"error"`
				}
				if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.JSONRPC != "2.0" || payload.ID != 7 || payload.Error == nil {
					t.Fatalf("JSON-RPC error response = %#v", payload)
				}
				return
			}
			var payload APIResponse
			if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.OK || payload.Error == "" || payload.Message == "" {
				t.Fatalf("canonical error response = %#v", payload)
			}
		})
	}
}
