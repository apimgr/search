package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestHandleMCPHelpReturnsLiveToolsAndRequestURL(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Pages.Help.MCP = "Search the web with `getApiV1Search`."
	handler := NewHandler(cfg, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "https://search.example.test/api/v1/server/help/mcp", nil)
	response := httptest.NewRecorder()

	handler.handleMCPHelp(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
	}
	var body struct {
		OK   bool `json:"ok"`
		Data struct {
			Enabled bool     `json:"enabled"`
			URL     string   `json:"url"`
			Aliases []string `json:"aliases"`
			Tools   []struct {
				Name string `json:"name"`
			} `json:"tools"`
			Project string `json:"project"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.OK || !body.Data.Enabled {
		t.Fatal("MCP help response is not enabled")
	}
	if body.Data.URL != "https://search.example.test/api/v1/server/mcp" {
		t.Fatalf("MCP URL = %q", body.Data.URL)
	}
	if len(body.Data.Aliases) != 2 || body.Data.Aliases[0] != "https://search.example.test/api/mcp" || body.Data.Aliases[1] != "https://search.example.test/mcp" {
		t.Fatalf("aliases = %#v", body.Data.Aliases)
	}
	if body.Data.Project != cfg.Server.Pages.Help.MCP {
		t.Fatalf("project markdown = %q", body.Data.Project)
	}
	for _, tool := range body.Data.Tools {
		if tool.Name == "getApiV1Search" {
			return
		}
	}
	t.Fatalf("live MCP tool list omitted getApiV1Search: %s", response.Body.String())
}

func TestHandleMCPHelpReturnsNotFoundWhenDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.MCP.Enabled = false
	handler := NewHandler(cfg, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/server/help/mcp", nil)
	response := httptest.NewRecorder()

	handler.handleMCPHelp(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
	if strings.Contains(response.Body.String(), "tools") {
		t.Fatalf("disabled MCP response disclosed tools: %s", response.Body.String())
	}
}
