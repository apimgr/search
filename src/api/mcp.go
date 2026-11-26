package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/apimgr/search/src/common/closeutil"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/swagger"
	"github.com/apimgr/search/src/version"
)

const mcpProtocolVersion = "2026-07-28"

var mcpVersions = []string{mcpProtocolVersion}

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
	Annotations map[string]bool        `json:"annotations"`
	Path        string                 `json:"-"`
	Method      string                 `json:"-"`
	Operation   *swagger.Operation     `json:"-"`
}

func (h *Handler) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		h.errorResponse(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "", r)
		return
	}
	if r.ContentLength > 1<<20 {
		h.errorResponse(w, http.StatusRequestEntityTooLarge, "BAD_REQUEST", "", r)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "", r)
		return
	}
	var request mcpRequest
	if err := json.Unmarshal(body, &request); err != nil || request.JSONRPC != "2.0" || request.Method == "" {
		h.errorResponse(w, http.StatusBadRequest, "BAD_REQUEST", "", r)
		return
	}
	if !h.validMCPOrigin(r) {
		writeMCPError(w, http.StatusForbidden, request.ID, -32000, "Forbidden", i18n.RequestString(r, "errors.forbidden"))
		return
	}
	if !h.validMCPBearer(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="server"`)
		writeMCPError(w, http.StatusUnauthorized, request.ID, -32001, "Unauthorized", i18n.RequestString(r, "errors.unauthorized"))
		return
	}
	if request.Method == "initialize" && r.Header.Get("MCP-Protocol-Version") == "" && mcpRequestProtocolVersion(request) == "" {
		writeMCPUnsupportedVersion(w, r, request.ID)
		return
	}
	requestedVersion := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version"))
	if requestedVersion != "" && requestedVersion != mcpProtocolVersion {
		writeMCPUnsupportedVersion(w, r, request.ID)
		return
	}
	if mcpHeaderError(r, request) {
		writeMCPError(w, http.StatusBadRequest, request.ID, -32020, "HeaderMismatch", i18n.RequestString(r, "mcp.errors.header_mismatch"))
		return
	}
	w.Header().Set("MCP-Protocol-Version", mcpProtocolVersion)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	result, rpcErr := h.dispatchMCP(r, request)
	response := map[string]interface{}{"jsonrpc": "2.0", "id": json.RawMessage(request.ID)}
	if rpcErr != nil {
		code := -32601
		var protocolErr *mcpRPCError
		if errors.As(rpcErr, &protocolErr) {
			code = protocolErr.code
		}
		response["error"] = map[string]interface{}{"code": code, "message": rpcErr.Error()}
	} else {
		response["result"] = result
	}
	status := http.StatusOK
	var protocolErr *mcpRPCError
	if errors.As(rpcErr, &protocolErr) && protocolErr.code == -32601 {
		status = http.StatusNotFound
	}
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Debug("failed to write MCP response", "error", err)
	}
}

func mcpHeaderError(r *http.Request, request mcpRequest) bool {
	if version := strings.TrimSpace(r.Header.Get("MCP-Protocol-Version")); version == "" || version != mcpRequestProtocolVersion(request) {
		return true
	}
	if method := strings.TrimSpace(r.Header.Get("Mcp-Method")); method == "" || method != request.Method {
		return true
	}
	if request.Method == "tools/call" || request.Method == "resources/read" || request.Method == "prompts/get" {
		rawName := strings.TrimSpace(r.Header.Get("Mcp-Name"))
		for _, char := range rawName {
			if char > 127 {
				return true
			}
		}
		name, err := new(mime.WordDecoder).DecodeHeader(rawName)
		if err != nil || name == "" || !utf8.ValidString(name) {
			return true
		}
		var params struct {
			Name string `json:"name"`
			URI  string `json:"uri"`
		}
		if err := json.Unmarshal(rawParams(request.Params), &params); err != nil {
			return true
		}
		want := params.Name
		if request.Method == "resources/read" {
			want = params.URI
		}
		if want == "" || name != want {
			return true
		}
	}
	return false
}

type mcpRPCError struct {
	code    int
	message string
}

func (e *mcpRPCError) Error() string { return e.message }

func rawParams(params map[string]json.RawMessage) []byte {
	data, _ := json.Marshal(params)
	return data
}

func (h *Handler) dispatchMCP(r *http.Request, request mcpRequest) (interface{}, error) {
	switch request.Method {
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
			return nil, &mcpRPCError{code: -32602, message: i18n.RequestString(r, "mcp.errors.invalid_call")}
		}
		for _, tool := range h.mcpTools(r) {
			if tool.Name == params.Name {
				return h.callMCPTool(r, tool, params.Arguments), nil
			}
		}
		return mcpToolError(r, "NOT_FOUND", "mcp.errors.tool_not_found"), nil
	default:
		return nil, &mcpRPCError{code: -32601, message: i18n.RequestString(r, "mcp.errors.method_not_found")}
	}
}

func (h *Handler) validMCPBearer(r *http.Request) bool {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	if h.operatorAuthError(r) == nil {
		return true
	}
	if h.ownerManager == nil {
		return false
	}
	_, err := h.ownerManager.Resolve(r.Context(), strings.TrimSpace(header[len(prefix):]))
	return err == nil
}

func mcpRequestProtocolVersion(request mcpRequest) string {
	metaRaw, ok := request.Params["_meta"]
	if !ok {
		return ""
	}
	var meta map[string]json.RawMessage
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return ""
	}
	var value string
	if err := json.Unmarshal(meta["io.modelcontextprotocol/protocolVersion"], &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func writeMCPUnsupportedVersion(w http.ResponseWriter, r *http.Request, id json.RawMessage) {
	writeMCPError(w, http.StatusBadRequest, id, -32000, "UnsupportedProtocolVersionError", i18n.RequestFormat(r, "mcp.errors.unsupported_version", "version", mcpProtocolVersion))
}

func (h *Handler) mcpTools(r *http.Request) []mcpTool {
	spec := swagger.GenerateSpec(h.config, "")
	tools := make([]mcpTool, 0)
	for path, item := range spec.Paths {
		if !strings.HasPrefix(path, version.APIPrefix+"/") || strings.Contains(path, "/server/preferences") || strings.Contains(path, "/server/mcp") {
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
			tools = append(tools, mcpTool{Name: operation.OperationID, Description: firstNonEmpty(operation.Description, operation.Summary), InputSchema: mcpInputSchema(operation), Annotations: mcpToolAnnotations(method), Path: path, Method: method, Operation: operation})
		}
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	return tools
}

func mcpToolAnnotations(method string) map[string]bool {
	annotations := make(map[string]bool)
	switch method {
	case http.MethodGet:
		annotations["readOnlyHint"] = true
	case http.MethodDelete:
		annotations["destructiveHint"] = true
		annotations["idempotentHint"] = true
	case http.MethodPut:
		annotations["idempotentHint"] = true
	case http.MethodPost, http.MethodPatch:
		annotations["readOnlyHint"] = false
	}
	return annotations
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
		return mcpToolError(r, "BAD_REQUEST", "mcp.errors.path_argument_missing")
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
			return mcpToolError(r, "BAD_REQUEST", "mcp.errors.invalid_tool_arguments")
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
		return mcpToolError(r, "SERVER_ERROR", "mcp.errors.api_routes_unavailable")
	}
	h.apiRoutes.ServeHTTP(recorder, request)
	response := recorder.Result()
	defer closeutil.Close(response.Body)
	data, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return mcpToolError(r, "SERVER_ERROR", "mcp.errors.tool_response_unavailable")
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

func mcpToolError(r *http.Request, code, key string) map[string]interface{} {
	body := map[string]interface{}{"ok": false, "error": code, "message": i18n.RequestString(r, key)}
	encoded, _ := json.Marshal(body)
	return map[string]interface{}{"content": []map[string]string{{"type": "text", "text": string(encoded)}}, "structuredContent": body, "isError": true}
}

func writeMCPError(w http.ResponseWriter, status int, id json.RawMessage, code int, name, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"jsonrpc": "2.0", "id": id, "error": map[string]interface{}{"code": code, "message": message, "data": map[string]string{"name": name}}}); err != nil {
		slog.Debug("failed to write MCP error response", "error", err)
	}
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
	requestURL, err := url.Parse(httputil.BuildURL(r, "/"))
	if err == nil && sameOrigin(parsed, requestURL) {
		return true
	}
	domains := strings.Split(os.Getenv("DOMAIN"), ",")
	if os.Getenv("DOMAIN") == "" && h.config.Server.FQDN != "" {
		domains = strings.Split(h.config.Server.FQDN, ",")
	}
	for _, domain := range domains {
		domain = strings.TrimSpace(domain)
		if domain == "" {
			continue
		}
		allowed, err := url.Parse("https://" + domain)
		if err == nil && allowed.Host != "" && allowed.User == nil && allowed.Path == "" && sameOrigin(parsed, allowed) {
			return true
		}
	}
	if h.mcpOriginAllowed != nil && h.mcpOriginAllowed(r, parsed) {
		return true
	}
	for _, configured := range h.config.Server.CORS.AllowedOrigins {
		configured = strings.TrimSpace(configured)
		if configured == "" || configured == "*" {
			continue
		}
		allowed, err := url.Parse(configured)
		if err == nil && allowed.Scheme != "" && allowed.Host != "" && sameOrigin(parsed, allowed) {
			return true
		}
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
