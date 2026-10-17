package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/apimgr/search/src/common/closeutil"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/swagger"
	"github.com/apimgr/search/src/version"
)

var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

type mcpRequest struct {
	JSONRPC string                     `json:"jsonrpc"`
	ID      json.RawMessage            `json:"id,omitempty"`
	Method  string                     `json:"method"`
	Params  map[string]json.RawMessage `json:"params,omitempty"`
}

type mcpTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	Path        string                 `json:"-"`
	Method      string                 `json:"-"`
	Operation   *swagger.Operation     `json:"-"`
}

func (h *Handler) handleMCP(w http.ResponseWriter, r *http.Request) {
	if !h.config.Server.MCP.Enabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.validMCPOrigin(r) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if r.ContentLength > 1<<20 {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	var request mcpRequest
	if err := json.Unmarshal(body, &request); err != nil || request.JSONRPC != "2.0" || request.Method == "" {
		http.Error(w, "invalid JSON-RPC request", http.StatusBadRequest)
		return
	}
	if mismatch := mcpHeaderError(r, request); mismatch != "" {
		writeMCPError(w, http.StatusBadRequest, request.ID, -32600, "HeaderMismatch", mismatch)
		return
	}
	var versionParam struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(rawParams(request.Params), &versionParam)
	if headerVersion := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")); headerVersion != "" && versionParam.ProtocolVersion != "" && headerVersion != versionParam.ProtocolVersion {
		writeMCPError(w, http.StatusBadRequest, request.ID, -32600, "HeaderMismatch", "MCP-Protocol-Version does not match JSON-RPC protocolVersion")
		return
	}
	protocol, err := mcpProtocol(r, request)
	if err != nil {
		writeMCPError(w, http.StatusBadRequest, request.ID, -32000, "UnsupportedProtocolVersionError", "supported versions: "+strings.Join(mcpVersions, ", "))
		return
	}
	if protocol != "" {
		w.Header().Set("MCP-Protocol-Version", protocol)
	}
	if len(request.ID) == 0 && strings.HasPrefix(request.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	result, rpcErr := h.dispatchMCP(r, request)
	response := map[string]interface{}{"jsonrpc": "2.0", "id": json.RawMessage(request.ID)}
	if rpcErr != nil {
		response["error"] = map[string]interface{}{"code": -32601, "message": rpcErr.Error()}
	} else {
		response["result"] = result
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func mcpHeaderError(r *http.Request, request mcpRequest) string {
	if method := r.Header.Get("Mcp-Method"); method != "" && method != request.Method {
		return "Mcp-Method does not match JSON-RPC method"
	}
	if name := r.Header.Get("Mcp-Name"); name != "" {
		var params struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(rawParams(request.Params), &params)
		if name != params.Name {
			return "Mcp-Name does not match JSON-RPC name"
		}
	}
	return ""
}

func mcpProtocol(r *http.Request, request mcpRequest) (string, error) {
	header := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version"))
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(rawParams(request.Params), &params)
	body := strings.TrimSpace(params.ProtocolVersion)
	if header != "" && body != "" && header != body {
		return "", errors.New("protocol version header and body differ")
	}
	version := body
	if version == "" {
		version = header
	}
	if version == "" && request.Method != "initialize" {
		version = mcpVersions[0]
	}
	if version != "" {
		found := false
		for _, supported := range mcpVersions {
			if version == supported {
				found = true
				break
			}
		}
		if !found {
			return "", errors.New("unsupported protocol version")
		}
	}
	return version, nil
}

func rawParams(params map[string]json.RawMessage) []byte {
	data, _ := json.Marshal(params)
	return data
}

func (h *Handler) dispatchMCP(r *http.Request, request mcpRequest) (interface{}, error) {
	switch request.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(rawParams(request.Params), &params)
		protocol := params.ProtocolVersion
		if protocol == "" {
			protocol = mcpVersions[0]
		}
		return map[string]interface{}{"protocolVersion": protocol, "capabilities": map[string]interface{}{"tools": map[string]interface{}{"listChanged": false}}, "serverInfo": map[string]string{"name": h.config.Server.Title, "version": configVersion()}}, nil
	case "ping":
		return map[string]interface{}{}, nil
	case "tools/list":
		tools := h.mcpTools(r)
		return map[string]interface{}{"tools": tools}, nil
	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(rawParams(request.Params), &params); err != nil || params.Name == "" {
			return nil, errors.New("invalid tools/call params")
		}
		for _, tool := range h.mcpTools(r) {
			if tool.Name == params.Name {
				return h.callMCPTool(r, tool, params.Arguments), nil
			}
		}
		return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": `{"ok":false,"error":"NOT_FOUND","message":"tool not found or unavailable"}`}}, "isError": true}, nil
	default:
		return nil, fmt.Errorf("method not found: %s", request.Method)
	}
}

func (h *Handler) mcpTools(r *http.Request) []mcpTool {
	spec := swagger.GenerateSpec(h.config, "")
	tools := make([]mcpTool, 0)
	for path, item := range spec.Paths {
		if !strings.HasPrefix(path, version.APIPrefix+"/") || strings.Contains(path, "/server/preferences/tokens") || strings.Contains(path, "/server/mcp") {
			continue
		}
		for method, operation := range map[string]*swagger.Operation{"GET": item.Get, "POST": item.Post, "PUT": item.Put, "PATCH": item.Patch, "DELETE": item.Delete} {
			if operation == nil {
				continue
			}
			if strings.Contains(path, "/server/scheduler/") || hasTag(operation.Tags, "Operator") {
				if h.operatorAuthError(r) != nil {
					continue
				}
			}
			if len(operation.Security) > 0 && h.operatorAuthError(r) != nil {
				continue
			}
			tools = append(tools, mcpTool{Name: operation.OperationID, Description: firstNonEmpty(operation.Description, operation.Summary), InputSchema: mcpInputSchema(operation), Path: path, Method: method, Operation: operation})
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func hasTag(tags []string, expected string) bool {
	for _, tag := range tags {
		if tag == expected {
			return true
		}
	}
	return false
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func mcpInputSchema(operation *swagger.Operation) map[string]interface{} {
	properties := map[string]interface{}{}
	required := make([]string, 0)
	for _, parameter := range operation.Parameters {
		if parameter.In != "path" && parameter.In != "query" {
			continue
		}
		schema := map[string]interface{}{"type": "string"}
		if parameter.Schema != nil {
			schema = swaggerSchema(parameter.Schema)
		}
		properties[parameter.Name] = schema
		if parameter.Required {
			required = append(required, parameter.Name)
		}
	}
	if operation.RequestBody != nil {
		if media, ok := operation.RequestBody.Content["application/json"]; ok && media.Schema != nil {
			for key, schema := range media.Schema.Properties {
				properties[key] = swaggerSchema(&schema)
			}
		}
	}
	if len(required) == 0 {
		required = nil
	}
	return map[string]interface{}{"type": "object", "properties": properties, "required": required}
}

func swaggerSchema(schema *swagger.Schema) map[string]interface{} {
	result := map[string]interface{}{}
	if schema.Type != "" {
		result["type"] = schema.Type
	}
	if schema.Example != nil {
		result["example"] = schema.Example
	}
	if len(schema.Properties) > 0 {
		properties := map[string]interface{}{}
		for key, property := range schema.Properties {
			properties[key] = swaggerSchema(&property)
		}
		result["properties"] = properties
	}
	if schema.Items != nil {
		result["items"] = swaggerSchema(schema.Items)
	}
	return result
}

func (h *Handler) callMCPTool(r *http.Request, tool mcpTool, arguments map[string]interface{}) map[string]interface{} {
	if arguments == nil {
		arguments = map[string]interface{}{}
	}
	path := tool.Path
	query := url.Values{}
	consumed := map[string]bool{}
	for _, parameter := range tool.Operation.Parameters {
		value, exists := arguments[parameter.Name]
		if !exists {
			continue
		}
		consumed[parameter.Name] = true
		text := fmt.Sprint(value)
		if parameter.In == "path" {
			path = strings.ReplaceAll(path, "{"+parameter.Name+"}", url.PathEscape(text))
		}
		if parameter.In == "query" {
			query.Set(parameter.Name, text)
		}
	}
	if strings.Contains(path, "{") {
		return mcpToolError("required path argument missing")
	}
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var body io.Reader
	if tool.Operation.RequestBody != nil {
		payload := map[string]interface{}{}
		if media, ok := tool.Operation.RequestBody.Content["application/json"]; ok && media.Schema != nil && len(media.Schema.Properties) > 0 {
			for key := range media.Schema.Properties {
				if value, exists := arguments[key]; exists {
					payload[key] = value
					consumed[key] = true
				}
			}
		} else {
			for key, value := range arguments {
				if !consumed[key] {
					payload[key] = value
				}
			}
		}
		data, err := json.Marshal(payload)
		if err != nil {
			return mcpToolError("invalid tool arguments")
		}
		body = bytes.NewReader(data)
	}
	request := httptest.NewRequest(tool.Method, path, body)
	request.Header = r.Header.Clone()
	request.Header.Set("X-Request-Source", "mcp")
	request.Host = r.Host
	request.RemoteAddr = r.RemoteAddr
	request.TLS = r.TLS
	recorder := httptest.NewRecorder()
	if h.apiRoutes == nil {
		return mcpToolError("API routes are unavailable")
	}
	h.apiRoutes.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer closeutil.Close(response.Body)
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return mcpToolError("tool response unavailable")
	}
	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		parsed = string(data)
	}
	result := map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(data)}}}
	if object, ok := parsed.(map[string]interface{}); ok {
		result["structuredContent"] = object
	}
	if response.StatusCode >= 400 {
		result["isError"] = true
	}
	return result
}

func mcpToolError(message string) map[string]interface{} {
	body := map[string]interface{}{"ok": false, "error": "BAD_REQUEST", "message": message}
	encoded, _ := json.Marshal(body)
	return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(encoded)}}, "structuredContent": body, "isError": true}
}

func writeMCPError(w http.ResponseWriter, status int, id json.RawMessage, code int, name, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": map[string]interface{}{"code": code, "message": message, "data": map[string]string{"name": name}}})
}

func (h *Handler) validMCPOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	requestURL, err := url.Parse(httputil.GetBaseURLFromRequest(r))
	if err == nil && sameOrigin(parsed, requestURL) {
		return true
	}
	for _, configured := range strings.Split(h.config.Server.Web.CORS, ",") {
		configured = strings.TrimSpace(configured)
		allowed, err := url.Parse(configured)
		if err == nil && allowed.Scheme != "" && allowed.Host != "" && sameOrigin(parsed, allowed) {
			return true
		}
	}
	return false
}

func sameOrigin(a, b *url.URL) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func configVersion() string { return version.Version }
