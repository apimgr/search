package api

import (
	"net/http"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/version"
)

type MCPHelpTool struct {
	Name        string
	Description string
}

type MCPHelpInfo struct {
	Enabled                     bool
	URL                         string
	Aliases                     []string
	ProtocolVersion             string
	ProtocolVersions            []string
	Tools                       []MCPHelpTool
	AuthenticatedToolsAvailable bool
	ProjectMarkdown             string
}

func (h *Handler) MCPHelpInfo(r *http.Request) MCPHelpInfo {
	tools := h.mcpTools(r)
	helpTools := make([]MCPHelpTool, 0, len(tools))
	for _, tool := range tools {
		helpTools = append(helpTools, MCPHelpTool{Name: tool.Name, Description: tool.Description})
	}
	authenticatedToolsAvailable := false
	if h.config.Server.Token != "" {
		authenticatedRequest := r.Clone(r.Context())
		authenticatedRequest.Header = r.Header.Clone()
		authenticatedRequest.Header.Set("Authorization", "Bearer "+h.config.Server.Token)
		for _, tool := range h.mcpTools(authenticatedRequest) {
			if !containsMCPTool(tools, tool.Name) {
				authenticatedToolsAvailable = true
				break
			}
		}
	}

	return MCPHelpInfo{
		Enabled:                     h.config.Server.MCP.Enabled,
		URL:                         httputil.BuildURL(r, version.APIPrefix+"/server/mcp"),
		Aliases:                     []string{httputil.BuildURL(r, "/api/mcp"), httputil.BuildURL(r, "/mcp")},
		ProtocolVersion:             mcpVersions[0],
		ProtocolVersions:            append([]string(nil), mcpVersions...),
		Tools:                       helpTools,
		AuthenticatedToolsAvailable: authenticatedToolsAvailable,
		ProjectMarkdown:             h.config.Server.Pages.Help.MCP,
	}
}

func containsMCPTool(tools []mcpTool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func (h *Handler) handleMCPHelp(w http.ResponseWriter, r *http.Request) {
	if !h.config.Server.MCP.Enabled {
		http.NotFound(w, r)
		return
	}
	info := h.MCPHelpInfo(r)
	tools := make([]map[string]string, 0, len(info.Tools))
	for _, tool := range info.Tools {
		tools = append(tools, map[string]string{"name": tool.Name, "description": tool.Description})
	}
	w.Header().Set("Cache-Control", "no-store")
	h.writeJSON(w, http.StatusOK, APIResponse{OK: true, Data: map[string]interface{}{
		"enabled": info.Enabled,
		"url":     info.URL,
		"aliases": info.Aliases,
		"auth":    map[string]string{"scheme": "Bearer"},
		"tools":   tools,
		"project": info.ProjectMarkdown,
	}})
}
