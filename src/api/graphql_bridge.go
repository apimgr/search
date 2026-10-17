package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/apimgr/search/src/common/closeutil"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
)

var errGraphQLRouteUnavailable = errors.New("API route is unavailable")

// APIRequestForGraphQL routes an API REST request through the registered API mux.
func (h *Handler) APIRequestForGraphQL(ctx context.Context, request *http.Request, method, path string, input interface{}) (map[string]interface{}, error) {
	if request == nil || h.apiRoutes == nil {
		return nil, errGraphQLRouteUnavailable
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
	default:
		return nil, errGraphQLRouteUnavailable
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/api/") || strings.HasSuffix(parsed.Path, "/graphql") || strings.HasSuffix(parsed.Path, "/mcp") {
		return nil, errGraphQLRouteUnavailable
	}
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil || len(encoded) > 64<<10 {
			return nil, errGraphQLRouteUnavailable
		}
		body = bytes.NewReader(encoded)
	}
	routed := httptest.NewRequest(method, parsed.String(), body)
	routed = routed.WithContext(ctx)
	routed.Header = request.Header.Clone()
	routed.Host = request.Host
	routed.RemoteAddr = request.RemoteAddr
	routed.TLS = request.TLS
	if body != nil {
		routed.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	h.apiRoutes.ServeHTTP(recorder, routed)
	response := recorder.Result()
	defer closeutil.Close(response.Body)
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, errGraphQLRouteUnavailable
	}
	var payload interface{}
	if json.Unmarshal(data, &payload) != nil {
		payload = string(data)
	}
	return map[string]interface{}{"status": response.StatusCode, "body": payload}, nil
}

// ContactWebhookForGraphQL exposes the operator contact webhook config route through GraphQL.
func (h *Handler) ContactWebhookForGraphQL(ctx context.Context, request *http.Request, input map[string]interface{}) (interface{}, error) {
	if request == nil {
		return nil, errGraphQLRouteUnavailable
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, errGraphQLRouteUnavailable
	}
	secured := h.requireOperator(h.handleContactWebhookConfigSave)
	response, err := h.runGraphQLRoute(ctx, request, APIPrefix+"/server/config/contact/webhooks", http.MethodPost, body, secured)
	if err != nil {
		return nil, err
	}
	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.OK {
		return nil, errGraphQLRouteUnavailable
	}
	return envelope.Data, nil
}

// OwnerTokenForGraphQL delegates primary-token operations to the exact REST
// handlers so authorization, expiry parsing, and one-time token responses stay aligned.
func (h *Handler) OwnerTokenForGraphQL(ctx context.Context, request *http.Request, operation, id string, input map[string]interface{}) (interface{}, error) {
	if request == nil {
		return nil, errGraphQLRouteUnavailable
	}
	var method, path string
	var handler http.HandlerFunc
	switch operation {
	case "list":
		method, path, handler = http.MethodGet, APIPrefix+"/server/preferences/tokens", h.ownerTokensList
	case "revealPrimary":
		method, path, handler = http.MethodPost, APIPrefix+"/server/preferences/tokens/primary/reveal", h.ownerPrimaryReveal
	case "rotatePrimary":
		method, path, handler = http.MethodPost, APIPrefix+"/server/preferences/tokens/primary/rotate", h.ownerPrimaryRotate
	case "create":
		method, path, handler = http.MethodPost, APIPrefix+"/server/preferences/tokens", h.ownerLinkedCreate
	case "reveal", "rotate":
		if id == "" {
			return nil, errGraphQLRouteUnavailable
		}
		method, handler = http.MethodPost, h.ownerLinkedReveal
		if operation == "rotate" {
			handler = h.ownerLinkedRotate
		}
		path = APIPrefix + "/server/preferences/tokens/" + id + "/" + operation
	case "update":
		if id == "" {
			return nil, errGraphQLRouteUnavailable
		}
		method, path, handler = http.MethodPatch, APIPrefix+"/server/preferences/tokens/"+id, h.ownerLinkedUpdate
	case "revoke":
		if id == "" {
			return nil, errGraphQLRouteUnavailable
		}
		method, path, handler = http.MethodDelete, APIPrefix+"/server/preferences/tokens/"+id, h.ownerLinkedRevoke
	default:
		return nil, errGraphQLRouteUnavailable
	}
	var body []byte
	if method == http.MethodPost && operation == "create" || method == http.MethodPatch {
		encoded, err := json.Marshal(input)
		if err != nil {
			return nil, errGraphQLRouteUnavailable
		}
		body = encoded
	}
	clone := request.Clone(ctx)
	routeContext := chi.NewRouteContext()
	if id != "" {
		routeContext.URLParams.Add("id", id)
	}
	ctx = context.WithValue(ctx, chi.RouteCtxKey, routeContext)
	response, err := h.runGraphQLRoute(ctx, clone, path, method, body, handler)
	if err != nil {
		return nil, err
	}
	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.OK {
		return nil, errGraphQLRouteUnavailable
	}
	return envelope.Data, nil
}

func (h *Handler) runGraphQLRoute(ctx context.Context, request *http.Request, path, method string, body []byte, handler http.HandlerFunc) (*httptest.ResponseRecorder, error) {
	if request == nil || handler == nil {
		return nil, errGraphQLRouteUnavailable
	}
	clone := request.Clone(ctx)
	clone.Method = method
	clone.URL = new(url.URL)
	*clone.URL = *request.URL
	clone.URL.Path = path
	clone.Body = http.NoBody
	if len(body) != 0 {
		clone.Body = io.NopCloser(bytes.NewReader(body))
	}
	response := httptest.NewRecorder()
	handler(response, clone)
	if response.Code < http.StatusOK || response.Code >= http.StatusMultipleChoices {
		return nil, errGraphQLRouteUnavailable
	}
	return response, nil
}

// ServerPageForGraphQL calls the same handler used by the public REST page route.
func (h *Handler) ServerPageForGraphQL(ctx context.Context, request *http.Request, name string) (interface{}, error) {
	path := APIPrefix + "/server/" + name
	var handler http.HandlerFunc
	switch name {
	case "about":
		handler = h.handleServerAbout
	case "privacy":
		handler = h.handleServerPrivacy
	case "help":
		handler = h.handleServerHelp
	case "terms":
		handler = h.handleServerTerms
	default:
		return nil, errGraphQLRouteUnavailable
	}
	response, err := h.runGraphQLRoute(ctx, request, path, http.MethodGet, nil, handler)
	if err != nil {
		return nil, err
	}
	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		return nil, errGraphQLRouteUnavailable
	}
	return envelope.Data, nil
}

// ContactForGraphQL routes contact reads and submissions through the REST handler.
func (h *Handler) ContactForGraphQL(ctx context.Context, request *http.Request, method string, input map[string]interface{}) (interface{}, error) {
	var body []byte
	if method == http.MethodPost {
		encoded, err := json.Marshal(input)
		if err != nil {
			return nil, errGraphQLRouteUnavailable
		}
		body = encoded
	} else if method != http.MethodGet {
		return nil, errGraphQLRouteUnavailable
	}
	response, err := h.runGraphQLRoute(ctx, request, APIPrefix+"/server/contact", method, body, h.handleServerContact)
	if err != nil {
		return nil, err
	}
	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.OK {
		return nil, errGraphQLRouteUnavailable
	}
	return envelope.Data, nil
}

// FaviconForGraphQL returns the same proxied image bytes as REST in base64 form.
func (h *Handler) FaviconForGraphQL(ctx context.Context, request *http.Request, target string) (map[string]string, error) {
	if request == nil {
		return nil, errGraphQLRouteUnavailable
	}
	clone := request.Clone(ctx)
	clone.Method = http.MethodGet
	clone.URL = new(url.URL)
	*clone.URL = *request.URL
	clone.URL.Path = APIPrefix + "/favicon"
	query := clone.URL.Query()
	if target != "" {
		query.Set("url", target)
	}
	clone.URL.RawQuery = query.Encode()
	response := httptest.NewRecorder()
	h.handleFavicon(response, clone)
	contentType := response.Header().Get("Content-Type")
	if response.Code != http.StatusOK || !strings.HasPrefix(contentType, "image/") {
		return nil, errGraphQLRouteUnavailable
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errGraphQLRouteUnavailable
	}
	return map[string]string{"contentType": mediaType, "base64": base64.StdEncoding.EncodeToString(response.Body.Bytes())}, nil
}

// PreferencesExportForGraphQL uses the REST export handler so cookie fallback and URL encoding stay identical.
func (h *Handler) PreferencesExportForGraphQL(ctx context.Context, request *http.Request) (interface{}, error) {
	response, err := h.runGraphQLRoute(ctx, request, APIPrefix+"/server/preferences/export", http.MethodGet, nil, h.handleServerPreferencesExport)
	if err != nil {
		return nil, err
	}
	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.OK {
		return nil, errGraphQLRouteUnavailable
	}
	return envelope.Data, nil
}

// PreferencesImportForGraphQL uses the REST import handler and returns its cookies for the GraphQL HTTP response.
func (h *Handler) PreferencesImportForGraphQL(ctx context.Context, request *http.Request, values url.Values) (interface{}, []string, error) {
	if request == nil {
		return nil, nil, errGraphQLRouteUnavailable
	}
	clone := request.Clone(ctx)
	clone.Method = http.MethodGet
	clone.URL = new(url.URL)
	*clone.URL = *request.URL
	clone.URL.Path = APIPrefix + "/server/preferences/import"
	clone.URL.RawQuery = values.Encode()
	response := httptest.NewRecorder()
	h.handleServerPreferencesImport(response, clone)
	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.OK {
		return nil, nil, errGraphQLRouteUnavailable
	}
	return envelope.Data, response.Header().Values("Set-Cookie"), nil
}

// AutodiscoverForGraphQL uses the versionless discovery endpoint implementation.
func (h *Handler) AutodiscoverForGraphQL(ctx context.Context, request *http.Request) (interface{}, error) {
	response, err := h.runGraphQLRoute(ctx, request, "/api/autodiscover", http.MethodGet, nil, h.handleAutodiscover)
	if err != nil {
		return nil, err
	}
	var envelope APIResponse
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.OK {
		return nil, errGraphQLRouteUnavailable
	}
	return envelope.Data, nil
}
