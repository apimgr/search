package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/health"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/owner"
	"github.com/apimgr/search/src/scheduler"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engine"
	"github.com/apimgr/search/src/swagger"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type testContactCaptcha struct{}

func (testContactCaptcha) NewContactChallenge(*http.Request) (string, string) {
	return "test-id", "1 + 1"
}

func (testContactCaptcha) VerifyContactChallenge(_ *http.Request, id, answer string) bool {
	return id == "test-id" && answer == "2"
}

type testContactSubmitter struct {
	calls int
	last  ContactMessage
	err   error
}

type autocompleteTestRoundTripper func(*http.Request) (*http.Response, error)

func (transport autocompleteTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func autocompleteTestResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: request}
}

type captureAPISearchEngine struct {
	query *model.Query
}

func (e *captureAPISearchEngine) Name() string                         { return "capture" }
func (e *captureAPISearchEngine) DisplayName() string                  { return "Capture" }
func (e *captureAPISearchEngine) IsEnabled() bool                      { return true }
func (e *captureAPISearchEngine) GetPriority() int                     { return 1 }
func (e *captureAPISearchEngine) SupportsCategory(model.Category) bool { return true }
func (e *captureAPISearchEngine) GetConfig() *model.EngineConfig       { return nil }
func (e *captureAPISearchEngine) Search(_ context.Context, query *model.Query) ([]model.Result, error) {
	copy := *query
	e.query = &copy
	return []model.Result{{Title: "Captured", URL: "https://example.test/result", Engine: "capture", Category: query.Category}}, nil
}

func (s *testContactSubmitter) SubmitContact(_ context.Context, message ContactMessage) error {
	s.calls++
	s.last = message
	return s.err
}

func (*testContactSubmitter) ContactReviewToken(url.Values, string) string {
	return "test-review-token"
}

func (*testContactSubmitter) ValidateContactReviewToken(_ url.Values, token string, confirmed bool) bool {
	return token == "test-review-token" && confirmed
}

func newTestHandler() *Handler {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title:       "Test Search",
			Description: "Test Description",
			Mode:        "development",
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	return NewHandler(cfg, registry, aggregator)
}

func TestOperatorRouteDistinguishesOwnerTokensFromInvalidCredentials(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()
	handler.config.Server.Token = "operator-test-token"

	primary, primaryToken, err := handler.ownerManager.Issue(context.Background())
	if err != nil {
		t.Fatalf("issue primary owner token: %v", err)
	}
	_, linkedToken, err := handler.ownerManager.CreateLinked(context.Background(), primary, "automation", nil)
	if err != nil {
		t.Fatalf("create linked owner token: %v", err)
	}

	tests := []struct {
		name       string
		token      string
		statusCode int
	}{
		{name: "missing", statusCode: http.StatusUnauthorized},
		{name: "invalid", token: "invalid-token", statusCode: http.StatusUnauthorized},
		{name: "primary owner", token: primaryToken, statusCode: http.StatusForbidden},
		{name: "linked owner", token: linkedToken, statusCode: http.StatusForbidden},
		{name: "operator", token: handler.config.Server.Token, statusCode: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/server/status", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			response := httptest.NewRecorder()
			handler.requireOperator(handler.handleServerStatus)(response, req)
			if response.Code != tc.statusCode {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tc.statusCode, response.Body.String())
			}
		})
	}
}

func TestOwnerLinkedCreateReturnsLinkedTokenField(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()
	_, primaryToken, err := handler.ownerManager.Issue(context.Background())
	if err != nil {
		t.Fatalf("issue primary owner token: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/server/preferences/tokens", strings.NewReader(`{"name":"automation","expires":"never"}`))
	request.Header.Set("Authorization", "Bearer "+primaryToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ownerLinkedCreate(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusCreated, response.Body.String())
	}
	var result struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	linkedToken, ok := result.Data["linked_token"].(string)
	if !ok || linkedToken == "" {
		t.Fatalf("data.linked_token = %#v, want non-empty string", result.Data["linked_token"])
	}
	if _, exists := result.Data["token"]; exists {
		t.Fatal("response contains obsolete data.token field")
	}
	id, ok := result.Data["id"].(string)
	parsedID, parseErr := uuid.Parse(id)
	if !ok || parseErr != nil || parsedID.Version() != 7 {
		t.Fatalf("data.id = %#v, want UUIDv7", result.Data["id"])
	}
	if result.Data["status"] != "active" || result.Data["token_masked"] == "" || result.Data["name"] != "automation" || result.Data["kind"] != "linked" {
		t.Fatalf("created token object is incomplete: %#v", result.Data)
	}
	if _, exists := result.Data["token_enc"]; exists {
		t.Fatal("response exposed encrypted token data")
	}
}

func TestOwnerPrimaryIssueAPIRejectsExistingValidCredential(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	issue := func(token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/server/preferences/tokens/primary", nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	first := issue("")
	if first.Code != http.StatusCreated {
		t.Fatalf("issue status = %d, want %d; body=%s", first.Code, http.StatusCreated, first.Body.String())
	}
	var issued struct { Data map[string]string `json:"data"` }
	if err := json.NewDecoder(first.Body).Decode(&issued); err != nil {
		t.Fatalf("decode issue response: %v", err)
	}
	ownerToken := issued.Data["owner_token"]
	if ownerToken == "" {
		t.Fatal("issue response omitted data.owner_token")
	}
	conflict := issue(ownerToken)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("valid existing owner token status = %d, want %d; body=%s", conflict.Code, http.StatusConflict, conflict.Body.String())
	}
	var details string
	if err := db.QueryRow(`SELECT details FROM audit_log WHERE action='token.created' ORDER BY timestamp DESC LIMIT 1`).Scan(&details); err != nil || !strings.Contains(details, `"source":"issue"`) {
		t.Fatalf("issue audit details = %q, error=%v; want source=issue", details, err)
	}
}

func TestOwnerPrimaryRevokeRequiresConfirmationAndDeletesOwnerData(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	identity, primaryToken, err := handler.ownerManager.Issue(context.Background())
	if err != nil {
		t.Fatalf("issue primary owner token: %v", err)
	}
	linkedID, _, err := handler.ownerManager.CreateLinked(context.Background(), identity, "automation", nil)
	if err != nil {
		t.Fatalf("create linked token: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO search_alerts (id,owner_id,email,query,category,frequency,manage_token_hash,rss_token_hash) VALUES ('alert-1',?,'a@example.test','q','general','daily','manage-hash','rss-hash')`, identity.OwnerID); err != nil {
		t.Fatalf("insert owner resource: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO search_alert_results (id,alert_id,fingerprint,title,url) VALUES ('result-1','alert-1','f','t','https://example.test')`); err != nil {
		t.Fatalf("insert owner resource child: %v", err)
	}
	post := func(confirm string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"confirm": confirm})
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/server/preferences/tokens/primary", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+primaryToken)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		return response
	}
	if response := post("wrong"); response.Code != http.StatusBadRequest {
		t.Fatalf("wrong confirmation status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
	var ownersBefore int
	if err := db.QueryRow(`SELECT COUNT(*) FROM owners WHERE id=?`, identity.OwnerID).Scan(&ownersBefore); err != nil || ownersBefore != 1 {
		t.Fatalf("wrong confirmation deleted owner: count=%d error=%v", ownersBefore, err)
	}
	response := post(primaryToken[len(primaryToken)-4:])
	if response.Code != http.StatusOK {
		t.Fatalf("revoke status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var result struct { Data map[string]float64 `json:"data"` }
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode revoke response: %v", err)
	}
	if result.Data["tokens"] != 2 || result.Data["resources"] != 2 {
		t.Fatalf("revoke counts = %#v, want two tokens and two resource rows", result.Data)
	}
	var owners, tokens, alerts, results int
	for query, target := range map[string]*int{
		`SELECT COUNT(*) FROM owners WHERE id=?`: &owners,
		`SELECT COUNT(*) FROM api_tokens WHERE owner_id=?`: &tokens,
		`SELECT COUNT(*) FROM search_alerts WHERE owner_id=?`: &alerts,
		`SELECT COUNT(*) FROM search_alert_results WHERE id='result-1'`: &results,
	} {
		args := []interface{}{}
		if strings.Contains(query, "?") { args = append(args, identity.OwnerID) }
		if err := db.QueryRow(query, args...).Scan(target); err != nil {
			t.Fatalf("check owner revocation: %v", err)
		}
	}
	if owners+tokens+alerts+results != 0 {
		t.Fatalf("owner revocation left rows: owners=%d tokens=%d alerts=%d results=%d", owners, tokens, alerts, results)
	}
	var action, details string
	if err := db.QueryRow(`SELECT action,details FROM audit_log WHERE resource_id=? ORDER BY timestamp DESC LIMIT 1`, identity.OwnerID).Scan(&action, &details); err != nil || action != "token.revoked" {
		t.Fatalf("owner revoke audit action=%q error=%v", action, err)
	}
	var auditDetails map[string]interface{}
	if err := json.Unmarshal([]byte(details), &auditDetails); err != nil || auditDetails["scope"] != "owner" || auditDetails["source"] != "api" || auditDetails["tokens_deleted"] != float64(2) || auditDetails["resources_deleted"] != float64(2) {
		t.Fatalf("owner revoke audit details=%q error=%v", details, err)
	}
	if _, err := handler.ownerManager.Resolve(context.Background(), primaryToken); !errors.Is(err, owner.ErrInvalidToken) {
		t.Fatalf("primary token remains valid after revoke: %v (linked id %s)", err, linkedID)
	}
}

func TestOwnerTokenAPIResponsesUseMaskedTokenObjects(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()
	identity, primaryToken, err := handler.ownerManager.Issue(context.Background())
	if err != nil {
		t.Fatalf("issue primary owner token: %v", err)
	}
	primaryAction := func(call http.HandlerFunc) map[string]string {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/server/preferences/tokens/primary", nil)
		req.Header.Set("Authorization", "Bearer "+primaryToken)
		response := httptest.NewRecorder()
		call(response, req)
		var envelope struct { Data map[string]string `json:"data"` }
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode primary token response: %v", err)
		}
		return envelope.Data
	}
	if data := primaryAction(handler.ownerPrimaryReveal); data["owner_token"] != primaryToken || data["token"] != "" {
		t.Errorf("primary reveal response = %#v", data)
	}
	rotatedPrimary := primaryAction(handler.ownerPrimaryRotate)["owner_token"]
	if rotatedPrimary == "" || rotatedPrimary == primaryToken {
		t.Fatalf("primary rotate response did not return a new owner_token")
	}
	primaryToken = rotatedPrimary
	id, linkedToken, err := handler.ownerManager.CreateLinked(context.Background(), identity, "automation", nil)
	if err != nil {
		t.Fatalf("create linked owner token: %v", err)
	}
	withID := func(method, body string, call http.HandlerFunc) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/api/v1/server/preferences/tokens/"+id, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+primaryToken)
		req.Header.Set("Content-Type", "application/json")
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("id", id)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
		response := httptest.NewRecorder()
		call(response, req)
		return response
	}
	decodeData := func(t *testing.T, response *httptest.ResponseRecorder) map[string]interface{} {
		t.Helper()
		var envelope struct { Data map[string]interface{} `json:"data"` }
		if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		return envelope.Data
	}
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/server/preferences/tokens", nil)
	listReq.Header.Set("Authorization", "Bearer "+primaryToken)
	listResponse := httptest.NewRecorder()
	handler.ownerTokensList(listResponse, listReq)
	var listEnvelope struct { Data struct { Primary map[string]interface{} `json:"primary"`; Linked []map[string]interface{} `json:"linked"`; MaxLinked int `json:"max_linked"` } `json:"data"` }
	if err := json.NewDecoder(listResponse.Body).Decode(&listEnvelope); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if listResponse.Code != http.StatusOK || listEnvelope.Data.Primary["kind"] != "primary" || len(listEnvelope.Data.Linked) != 1 || listEnvelope.Data.MaxLinked != handler.ownerManager.MaxLinked() {
		t.Fatalf("list response = %#v, status %d", listEnvelope.Data, listResponse.Code)
	}
	for _, token := range append([]map[string]interface{}{listEnvelope.Data.Primary}, listEnvelope.Data.Linked...) {
		if token["token_masked"] == "" || token["status"] != "active" {
			t.Errorf("token projection missing mask/status: %#v", token)
		}
		for _, forbidden := range []string{"token", "linked_token", "token_enc", "token_hash"} {
			if _, exists := token[forbidden]; exists {
				t.Errorf("list response exposed %q", forbidden)
			}
		}
	}
	if data := decodeData(t, withID(http.MethodPost, "", handler.ownerLinkedReveal)); data["linked_token"] != linkedToken || data["id"] != id {
		t.Errorf("reveal response = %#v", data)
	}
	if data := decodeData(t, withID(http.MethodPost, "", handler.ownerLinkedRotate)); data["linked_token"] == linkedToken || data["id"] != id || data["name"] != "automation" {
		t.Errorf("rotate response = %#v", data)
	}
	if data := decodeData(t, withID(http.MethodPatch, `{"name":"renamed"}`, handler.ownerLinkedUpdate)); data["name"] != "renamed" || data["token_masked"] == "" {
		t.Errorf("update response = %#v", data)
	}
	if data := decodeData(t, withID(http.MethodDelete, "", handler.ownerLinkedRevoke)); data["id"] != id || data["status"] != "revoked" {
		t.Errorf("revoke response = %#v", data)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/preferences/tokens", nil)
	req.Header.Set("Authorization", "Bearer "+primaryToken)
	response := httptest.NewRecorder()
	handler.ownerTokensList(response, req)
	var afterRevoke struct { Data struct { Linked []map[string]interface{} `json:"linked"` } `json:"data"` }
	if err := json.NewDecoder(response.Body).Decode(&afterRevoke); err != nil {
		t.Fatalf("decode list after revoke: %v", err)
	}
	if len(afterRevoke.Data.Linked) != 1 || afterRevoke.Data.Linked[0]["status"] != "revoked" || afterRevoke.Data.Linked[0]["token_masked"] == "" {
		t.Errorf("revoked token was not retained as a masked list item: %#v", afterRevoke.Data.Linked)
	}
}

func TestOwnerLinkedCreateRejectsInvalidExpiryShape(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()
	_, primaryToken, err := handler.ownerManager.Issue(context.Background())
	if err != nil {
		t.Fatalf("issue primary owner token: %v", err)
	}
	for _, body := range []string{
		`{"name":"automation","expires":"never","expires_on":"2027-01-31"}`,
		`{"name":"automation","expires_at":"7d"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/server/preferences/tokens", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+primaryToken)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ownerLinkedCreate(response, request)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want %d; response=%s", body, response.Code, http.StatusBadRequest, response.Body.String())
		}
	}
}

func TestOwnerExpiryDateRemainsValidThroughUTCDate(t *testing.T) {
	date := time.Now().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	expiry, err := parseOwnerExpiryFields(nil, &date)
	if err != nil {
		t.Fatalf("parse expires_on: %v", err)
	}
	parsedDate, _ := time.Parse("2006-01-02", date)
	want := parsedDate.UTC().AddDate(0, 0, 1)
	if expiry == nil || !expiry.Equal(want) {
		t.Fatalf("expires_on %s produced %v, want UTC midnight %s", date, expiry, want)
	}
}

func TestOwnerLinkedUpdateKeepsOmittedFields(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()
	identity, primaryToken, err := handler.ownerManager.Issue(context.Background())
	if err != nil {
		t.Fatalf("issue primary owner token: %v", err)
	}
	expiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	id, _, err := handler.ownerManager.CreateLinked(context.Background(), identity, "automation", &expiry)
	if err != nil {
		t.Fatalf("create linked token: %v", err)
	}
	update := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/server/preferences/tokens/"+id, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+primaryToken)
		req.Header.Set("Content-Type", "application/json")
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("id", id)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
		response := httptest.NewRecorder()
		handler.ownerLinkedUpdate(response, req)
		return response
	}
	if response := update(`{"name":"renamed"}`); response.Code != http.StatusOK {
		t.Fatalf("rename status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	tokens, err := handler.ownerManager.List(context.Background(), identity)
	if err != nil {
		t.Fatalf("list linked tokens after rename: %v", err)
	}
	var current *owner.Token
	for i := range tokens {
		if tokens[i].ID == id {
			current = &tokens[i]
		}
	}
	if current == nil || current.Name != "renamed" || current.ExpiresAt == nil || !current.ExpiresAt.Equal(expiry) {
		t.Fatalf("token after name-only update = %#v, expiry should be preserved", current)
	}
	if response := update(`{"expires":"never"}`); response.Code != http.StatusOK {
		t.Fatalf("expiry update status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	tokens, err = handler.ownerManager.List(context.Background(), identity)
	if err != nil {
		t.Fatalf("list linked tokens after expiry update: %v", err)
	}
	for i := range tokens {
		if tokens[i].ID == id {
			if tokens[i].Name != "renamed" || tokens[i].ExpiresAt != nil {
				t.Fatalf("token after expiry-only update = %#v, name should persist and expiry should be nil", tokens[i])
			}
			return
		}
	}
	t.Fatal("linked token disappeared after partial updates")
}

func cliBinaryTestRequest(filename string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/cli/binaries/"+filename, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("filename", filename)
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}

func TestHealthzEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, APIPrefix+"/server/healthz", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d for a missing required scheduler, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "unhealthy" || response.Checks.Scheduler != "error" {
		t.Errorf("Expected missing scheduler to make health unhealthy, got status %q and scheduler check %q", response.Status, response.Checks.Scheduler)
	}
}

func TestInfoEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	w := httptest.NewRecorder()

	handler.handleInfo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}
}

func TestAPIRequestForGraphQLUsesRegisteredRESTRoutes(t *testing.T) {
	handler := newTestHandler()
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	request := httptest.NewRequest(http.MethodPost, APIPrefix+"/server/graphql", nil)
	result, err := handler.APIRequestForGraphQL(request.Context(), request, http.MethodGet, "/api/v1/info", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result["status"] != http.StatusOK {
		t.Fatalf("REST bridge status = %v, want 200", result["status"])
	}
	body, ok := result["body"].(map[string]interface{})
	if !ok || body["ok"] != true {
		t.Fatalf("REST bridge returned unexpected payload: %#v", result["body"])
	}
	for _, test := range []struct{ method, path string }{
		{http.MethodGet, "https://example.com/api/v1/info"},
		{http.MethodPost, "/api/v1/server/graphql"},
		{http.MethodPost, "/api/v1/server/mcp"},
		{http.MethodOptions, "/api/v1/info"},
	} {
		if _, err := handler.APIRequestForGraphQL(request.Context(), request, test.method, test.path, nil); err == nil {
			t.Errorf("expected REST bridge to reject %s %s", test.method, test.path)
		}
	}
}

func TestOpenAPIDocumentsRegisteredAPIPaths(t *testing.T) {
	handler := newTestHandler()
	router := chi.NewRouter()
	handler.RegisterRoutes(router)
	spec := swagger.GenerateSpec(handler.config, "https://search.example.test")
	for _, route := range router.Routes() {
		path := openAPITestPath(route.Pattern)
		if !strings.HasPrefix(path, "/api/") {
			continue
		}
		item, exists := spec.Paths[path]
		if !exists {
			t.Errorf("registered REST path %q is missing from OpenAPI", path)
			continue
		}
		for method := range route.Handlers {
			if !openAPIMethod(item, method) {
				t.Errorf("OpenAPI omits registered %s %s", method, path)
			}
		}
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if openAPIMethod(item, method) {
				if _, registered := route.Handlers[method]; !registered {
					t.Errorf("OpenAPI documents unregistered %s %s", method, path)
				}
			}
		}
	}
	request := httptest.NewRequest(http.MethodPost, APIPrefix+"/server/graphql", nil)
	pathParameters := strings.NewReplacer(
		"{id}", "test", "{token}", "test", "{engine_id}", "test", "{widget_id}", "test",
		"{type}", "unknown", "{term}", "example.test", "{ip}", "203.0.113.1", "{name}", "test", "{task}", "test",
	)
	for path, item := range spec.Paths {
		if !strings.HasPrefix(path, "/api/") || strings.HasSuffix(path, "/graphql") || strings.HasSuffix(path, "/mcp") {
			continue
		}
		path = pathParameters.Replace(path)
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			if !openAPIMethod(item, method) {
				continue
			}
			bridge, err := handler.APIRequestForGraphQL(request.Context(), request, method, path, nil)
			if err != nil {
				t.Errorf("GraphQL apiRequest cannot dispatch documented REST operation %s %s: %v", method, path, err)
				continue
			}
			directRequest := httptest.NewRequest(method, path, nil)
			directRequest.RemoteAddr = request.RemoteAddr
			directRecorder := httptest.NewRecorder()
			router.ServeHTTP(directRecorder, directRequest)
			if bridge["status"] != directRecorder.Code {
				t.Errorf("GraphQL apiRequest %s %s status = %v, direct REST status = %d", method, path, bridge["status"], directRecorder.Code)
				continue
			}
			var directBody interface{}
			if json.Unmarshal(directRecorder.Body.Bytes(), &directBody) != nil {
				directBody = directRecorder.Body.String()
			}
			normalizeAPIVolatileMetadata(bridge["body"])
			normalizeAPIVolatileMetadata(directBody)
			if !reflect.DeepEqual(bridge["body"], directBody) {
				t.Errorf("GraphQL apiRequest %s %s body differs from direct REST response: bridge=%#v direct=%#v", method, path, bridge["body"], directBody)
			}
		}
	}
}

func normalizeAPIVolatileMetadata(value interface{}) {
	switch value := value.(type) {
	case map[string]interface{}:
		for _, key := range []string{"process_time_ms", "mem_alloc", "num_goroutine", "uptime"} {
			delete(value, key)
		}
		for _, child := range value {
			normalizeAPIVolatileMetadata(child)
		}
	case []interface{}:
		for _, child := range value {
			normalizeAPIVolatileMetadata(child)
		}
	}
}

func openAPITestPath(pattern string) string {
	switch pattern {
	case APIPrefix + "/alerts/*":
		return APIPrefix + "/alerts/{token}"
	case APIPrefix + "/direct/*":
		return APIPrefix + "/direct/{type}/{term}"
	case APIPrefix + "/engines/*":
		return APIPrefix + "/engines/{engine_id}"
	case APIPrefix + "/widgets/*":
		return APIPrefix + "/widgets/{widget_id}"
	default:
		return pattern
	}
}

func openAPIMethod(item swagger.PathItem, method string) bool {
	switch method {
	case http.MethodGet:
		return item.Get != nil
	case http.MethodPost:
		return item.Post != nil
	case http.MethodPut:
		return item.Put != nil
	case http.MethodPatch:
		return item.Patch != nil
	case http.MethodDelete:
		return item.Delete != nil
	default:
		return false
	}
}

func TestCategoriesEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)
	w := httptest.NewRecorder()

	handler.handleCategories(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.([]interface{})
	if !ok {
		t.Fatal("Expected data to be an array")
	}

	// Should expose the full IDEA category set
	if len(data) != 10 {
		t.Errorf("Expected 10 categories, got %d", len(data))
	}
}

func TestSearchEndpointMissingQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.OK {
		t.Error("Expected success to be false for missing query")
	}

	if response.Error == "" {
		t.Error("Expected error to be present")
	}
}

func TestAutocompleteEmptyQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete", nil)
	w := httptest.NewRecorder()

	handler.handleAutocomplete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	// Empty query should return empty array
	data, ok := response.Data.([]interface{})
	if !ok {
		t.Fatal("Expected data to be an array")
	}

	if len(data) != 0 {
		t.Errorf("Expected empty array for empty query, got %d items", len(data))
	}
}

func TestBangsEndpoint(t *testing.T) {
	handler := newTestHandler()
	handler.registry = engine.DefaultRegistry()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bangs", nil)
	w := httptest.NewRecorder()

	handler.handleBangs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	bangs, ok := data["bangs"].([]interface{})
	if !ok {
		t.Fatal("Expected bangs to be an array")
	}

	// Should have multiple built-in bangs
	if len(bangs) == 0 {
		t.Error("Expected at least one bang")
	}
}

func TestEnginesEndpoint(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines", nil)
	w := httptest.NewRecorder()

	handler.handleEngines(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}
}

func TestFormatDuration(t *testing.T) {
	handler := &Handler{}

	// Test zero duration
	result := handler.formatDuration(0)
	if result != "0s" {
		t.Errorf("formatDuration(0) = %q, want %q", result, "0s")
	}

	// Test minutes
	result = handler.formatDuration(5 * time.Minute)
	if result != "5m" {
		t.Errorf("formatDuration(5m) = %q, want %q", result, "5m")
	}

	// Test hours and minutes
	result = handler.formatDuration(2*time.Hour + 30*time.Minute)
	if result != "2h 30m" {
		t.Errorf("formatDuration(2h30m) = %q, want %q", result, "2h 30m")
	}

	// Test days
	result = handler.formatDuration(25 * time.Hour)
	if result != "1d 1h" {
		t.Errorf("formatDuration(25h) = %q, want %q", result, "1d 1h")
	}
}

func TestFormatBytes(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		bytes    uint64
		expected string
	}{
		{0, "0 B"},
		{100, "100 B"},
		{1024, "1.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := handler.formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://example.com/path", "example.com"},
		{"http://example.com/path", "example.com"},
		{"https://www.example.com/path/to/page", "www.example.com"},
		{"example.com/path", "example.com"},
		{"https://example.com", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

func TestAPIResponseStructure(t *testing.T) {
	// Test that APIResponse serializes correctly
	response := APIResponse{
		OK:   true,
		Data: map[string]string{"key": "value"},
		Meta: &APIMeta{Version: "v1"},
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal APIResponse: %v", err)
	}

	var unmarshaled APIResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal APIResponse: %v", err)
	}

	if unmarshaled.OK != response.OK {
		t.Errorf("Success mismatch: got %v, want %v", unmarshaled.OK, response.OK)
	}
}

// TestAPIErrorResponse tests the unified error response format per AI.md PART 16
func TestAPIErrorResponse(t *testing.T) {
	response := APIResponse{
		OK:      false,
		Error:   "BAD_REQUEST",
		Message: "Invalid request format",
	}

	data, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal error response: %v", err)
	}

	var unmarshaled APIResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}

	if unmarshaled.Error == "" {
		t.Fatal("Expected error code to be present")
	}

	if unmarshaled.Error != "BAD_REQUEST" {
		t.Errorf("Error code mismatch: got %q, want %q", unmarshaled.Error, "BAD_REQUEST")
	}

	if unmarshaled.Message != "Invalid request format" {
		t.Errorf("Error message mismatch: got %q, want %q", unmarshaled.Message, "Invalid request format")
	}
}

// Tests for Autodiscover endpoint

func TestAutodiscoverEndpoint(t *testing.T) {
	handler := newTestHandler()
	handler.config.Update(func(server *config.ServerConfig) {
		server.Tor.Expose = true
		server.I2P.Expose = true
	})

	req := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	w := httptest.NewRecorder()

	handler.handleAutodiscover(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["name"] != config.ProjectName || data["version"] == nil {
		t.Errorf("Expected top-level name and version, got %#v", data)
	}
	api, ok := data["api"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected api to be a map")
	}

	if api["current_version"] != "v1" {
		t.Errorf("Expected API current_version 'v1', got %v", api["current_version"])
	}
	if _, legacy := data["server"]; legacy {
		t.Error("Legacy server object must not be included")
	}
	if urls, ok := data["urls"].([]interface{}); !ok || len(urls) != 1 {
		t.Errorf("Expected one resolved public URL, got %#v", data["urls"])
	}
	for _, key := range []string{"onion", "i2p", "server", "config", "settings"} {
		if _, exists := data[key]; exists {
			t.Errorf("unexpected autodiscover field %q", key)
		}
	}
	if len(data) != 6 {
		t.Errorf("Expected six autodiscover fields including CLI metadata, got %#v", data)
	}
}

func TestAutodiscoverEndpointMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/autodiscover", nil)
	w := httptest.NewRecorder()

	handler.handleAutodiscover(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestCLIBinaryMetadataAndDownload(t *testing.T) {
	dir := t.TempDir()
	filename := "search-cli-linux-amd64"
	binary := []byte("verified cli fixture")
	if err := os.WriteFile(filepath.Join(dir, filename), binary, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "version.txt"), []byte("1.2.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(binary)
	checksums := hex.EncodeToString(digest[:]) + "  " + filename + "\n"
	if err := os.WriteFile(filepath.Join(dir, "sha256.txt"), []byte(checksums), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newTestHandler()
	handler.config.CLI.BinaryDownload.Dir = dir
	handler.config.Server.Update.CLIMinVersion = "1.0.0"
	if got := handler.cliBinaryVersions()["linux-amd64"]; got.Version != "1.2.3" || got.SHA256 != hex.EncodeToString(digest[:]) {
		t.Fatalf("published metadata = %#v", got)
	}
	req := cliBinaryTestRequest(filename)
	response := httptest.NewRecorder()
	handler.handleCLIBinary(response, req)
	if response.Code != http.StatusOK || response.Body.String() != string(binary) {
		t.Fatalf("download status/body = %d/%q", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q", got)
	}
	handler.config.CLI.BinaryDownload.RequireAuth = true
	handler.config.Server.Token = "operator-token"
	unauthorized := httptest.NewRecorder()
	handler.handleCLIBinary(unauthorized, cliBinaryTestRequest(filename))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated download status = %d, want 401", unauthorized.Code)
	}
	authorizedRequest := cliBinaryTestRequest(filename)
	authorizedRequest.Header.Set("Authorization", "Bearer operator-token")
	authorized := httptest.NewRecorder()
	handler.handleCLIBinary(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Errorf("authenticated download status = %d, want 200", authorized.Code)
	}
}

func TestCLIBinaryMetadataRejectsChecksumMismatch(t *testing.T) {
	dir := t.TempDir()
	filename := "search-cli-linux-amd64"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte("unverified"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "version.txt"), []byte("1.2.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sha256.txt"), []byte(strings.Repeat("0", 64)+"  "+filename+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := newTestHandler()
	handler.config.CLI.BinaryDownload.Dir = dir
	if _, exists := handler.cliBinaryVersions()["linux-amd64"]; exists {
		t.Fatal("checksum-mismatched CLI binary was published")
	}
	req := cliBinaryTestRequest(filename)
	response := httptest.NewRecorder()
	handler.handleCLIBinary(response, req)
	if response.Code != http.StatusNotFound {
		t.Errorf("download status = %d, want 404", response.Code)
	}
}

func TestOpenVerifiedCLIBinaryReturnsHashedRegularFile(t *testing.T) {
	dir := t.TempDir()
	filename := "search-cli-linux-amd64"
	content := []byte("descriptor-specific cli content")
	if err := os.WriteFile(filepath.Join(dir, filename), content, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	wantDigest := hex.EncodeToString(digest[:])
	file, info, gotDigest, err := openVerifiedCLIBinary(dir, filename, map[string]string{filename: wantDigest})
	if err != nil {
		t.Fatalf("openVerifiedCLIBinary() error = %v", err)
	}
	t.Cleanup(func() {
		if err := file.Close(); err != nil {
			t.Errorf("close verified descriptor: %v", err)
		}
	})
	if !info.Mode().IsRegular() {
		t.Fatalf("opened mode = %v, want regular file", info.Mode())
	}
	if gotDigest != wantDigest {
		t.Fatalf("digest = %q, want %q", gotDigest, wantDigest)
	}
	gotContent, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read verified descriptor: %v", err)
	}
	if !bytes.Equal(gotContent, content) {
		t.Fatalf("verified descriptor content = %q, want %q", gotContent, content)
	}
}

// Tests for Search endpoint with valid query

func TestSearchEndpointWithQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	// Should return OK, or a discriminated error status if no engines are
	// configured in this test handler (503) or another search failure
	// occurs (502) - per AI.md canonical status table / PART 9.
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable && w.Code != http.StatusBadGateway {
		t.Errorf("Expected status %d, %d, or %d, got %d", http.StatusOK, http.StatusServiceUnavailable, http.StatusBadGateway, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// The search might fail without engines, but response format should be valid
	if response.OK {
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be a map")
		}
		if data["query"] != "test" {
			t.Errorf("Expected query 'test', got %v", data["query"])
		}
	}
}

func TestSearchEndpointWithCategory(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&category=images", nil)
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.OK {
		data, ok := response.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected data to be a map")
		}
		if data["category"] != "images" {
			t.Errorf("Expected category 'images', got %v", data["category"])
		}
	}
}

func TestSearchEndpointPassesFiltersToEngine(t *testing.T) {
	engine := &captureAPISearchEngine{}
	handler := newTestHandler()
	handler.aggregator = search.NewAggregatorSimple([]search.Engine{engine}, time.Second)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=privacy&category=news&page=2&limit=30&lang=fr&region=ca&safe_search=2&time_range=custom&time_from=2025-01-01&time_to=2025-01-31&engines=capture", nil)
	w := httptest.NewRecorder()
	handler.handleSearch(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("search status = %d, body %s", w.Code, w.Body.String())
	}
	if engine.query == nil {
		t.Fatal("search engine did not receive a query")
	}
	query := engine.query
	if query.Text != "privacy" || query.Category != model.CategoryNews || query.Page != 2 || query.PerPage != 30 {
		t.Errorf("query core fields = %#v", query)
	}
	if query.Language != "fr" || query.Region != "ca" || query.SafeSearch != 2 || query.TimeRange != "custom" || query.DateAfter != "2025-01-01" || query.DateBefore != "2025-01-31" {
		t.Errorf("query filters = %#v", query)
	}
	if len(query.Engines) != 1 || query.Engines[0] != "capture" {
		t.Errorf("query engines = %#v", query.Engines)
	}
}

func TestValidateSearchDateRange(t *testing.T) {
	tests := []struct {
		name string
		req  SearchRequest
		want bool
	}{
		{name: "complete custom range", req: SearchRequest{TimeRange: "custom", TimeFrom: "2025-01-01", TimeTo: "2025-01-31"}},
		{name: "custom range requires both dates", req: SearchRequest{TimeRange: "custom", TimeFrom: "2025-01-01"}, want: true},
		{name: "invalid date", req: SearchRequest{TimeFrom: "2025-02-30"}, want: true},
		{name: "reversed range", req: SearchRequest{TimeFrom: "2025-02-01", TimeTo: "2025-01-01"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSearchDateRange(test.req)
			if (err != nil) != test.want {
				t.Fatalf("validateSearchDateRange() error = %v, want error %v", err, test.want)
			}
		})
	}
}

// Tests for Autocomplete with query

func TestAutocompleteWithQuery(t *testing.T) {
	handler := newTestHandler()
	handler.autocompleteClient.Transport = autocompleteTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		return autocompleteTestResponse(request, http.StatusOK, `["test",["test suggestion"]]`), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleAutocomplete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	// Autocomplete may return empty array if external API fails
	_, ok := response.Data.([]interface{})
	if !ok {
		t.Error("Expected data to be an array")
	}
}

func TestAutocompleteProviderCachesAndSanitizesSuggestions(t *testing.T) {
	handler := newTestHandler()
	requests := 0
	handler.autocompleteClient.Transport = autocompleteTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		if request.URL.Host != "duckduckgo.com" {
			t.Errorf("autocomplete host = %q, want duckduckgo.com", request.URL.Host)
		}
		if request.URL.Query().Get("q") != "test query" {
			t.Errorf("autocomplete query = %q, want escaped query value", request.URL.Query().Get("q"))
		}
		return autocompleteTestResponse(request, http.StatusOK, `["test query",["alpha","line\nbreak","line\u0085break",""] ]`), nil
	})

	first := handler.fetchAutocompleteSuggestions(context.Background(), " test query ")
	if len(first) != 3 || first[1] != "linebreak" || first[2] != "linebreak" {
		t.Fatalf("first suggestions = %q, want sanitized provider values", first)
	}
	first[0] = "caller mutation"
	second := handler.fetchAutocompleteSuggestions(context.Background(), "TEST QUERY")
	if requests != 1 || len(second) != 3 || second[0] != "alpha" {
		t.Fatalf("cached suggestions = %q after %d requests, want immutable one-hour cache", second, requests)
	}
	handler.autocompleteMu.Lock()
	entry := handler.autocompleteCache["test query"]
	if time.Until(entry.expiresAt) < 59*time.Minute {
		handler.autocompleteMu.Unlock()
		t.Fatalf("autocomplete cache TTL = %s, want one hour", time.Until(entry.expiresAt))
	}
	entry.expiresAt = time.Now().Add(-time.Second)
	handler.autocompleteCache["test query"] = entry
	handler.autocompleteMu.Unlock()
	handler.fetchAutocompleteSuggestions(context.Background(), "test query")
	if requests != 2 {
		t.Fatalf("expired cache made %d provider requests, want refetch", requests)
	}
}

func TestAutocompleteProviderRejectsFailuresAndOversizedInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "upstream failure", status: http.StatusServiceUnavailable, body: `[]`},
		{name: "redirect", status: http.StatusFound, body: `[]`},
		{name: "malformed JSON", status: http.StatusOK, body: `[{`},
		{name: "oversized JSON", status: http.StatusOK, body: `[]` + strings.Repeat(" ", maximumAutocompleteResponseBytes)},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			handler := newTestHandler()
			handler.autocompleteClient.Transport = autocompleteTestRoundTripper(func(request *http.Request) (*http.Response, error) {
				requests++
				response := autocompleteTestResponse(request, test.status, test.body)
				if test.status == http.StatusFound {
					response.Header.Set("Location", "https://redirect.example/next")
				}
				return response, nil
			})
			if suggestions := handler.fetchAutocompleteSuggestions(context.Background(), "test"); len(suggestions) != 0 {
				t.Fatalf("invalid provider response produced suggestions: %v", suggestions)
			}
			if requests != 1 {
				t.Fatalf("invalid response caused %d requests, want one without following redirects", requests)
			}
		})
	}

	handler := newTestHandler()
	requests := 0
	handler.autocompleteClient.Transport = autocompleteTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		requests++
		return autocompleteTestResponse(request, http.StatusOK, `["test",["ok"]]`), nil
	})
	if suggestions := handler.fetchAutocompleteSuggestions(context.Background(), strings.Repeat("x", maximumAutocompleteQueryBytes+1)); len(suggestions) != 0 {
		t.Fatalf("oversized query produced suggestions: %v", suggestions)
	}
	if requests != 0 {
		t.Fatalf("oversized query caused %d upstream requests", requests)
	}
}

func TestAutocompleteCacheHasBoundedEntriesAndResults(t *testing.T) {
	handler := newTestHandler()
	for index := range maximumAutocompleteCacheEntries + 1 {
		handler.cacheAutocomplete(fmt.Sprintf("query-%03d", index), []string{"one", "two"})
	}
	if got := len(handler.autocompleteCache); got != maximumAutocompleteCacheEntries {
		t.Fatalf("autocomplete cache size = %d, want %d", got, maximumAutocompleteCacheEntries)
	}
	handler.cacheAutocomplete("bounded-results", make([]string, maximumAutocompleteSuggestions+1))
	if got := len(handler.autocompleteCache["bounded-results"].suggestions); got != maximumAutocompleteSuggestions {
		t.Fatalf("cached autocomplete results = %d, want %d", got, maximumAutocompleteSuggestions)
	}
}

// Tests for Engine by ID endpoint

func TestEngineByIDEmpty(t *testing.T) {
	handler := newTestHandler()

	// Empty ID should fall back to engines list
	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines/", nil)
	w := httptest.NewRecorder()

	handler.handleEngineByID(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestEngineByIDNotFound(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.handleEngineByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status %d, got %d", http.StatusNotFound, w.Code)
	}
}

// Tests for Widgets endpoint

func TestWidgetsEndpointNoManager(t *testing.T) {
	handler := newTestHandler()
	// widgetManager is nil by default

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets", nil)
	w := httptest.NewRecorder()

	handler.handleWidgets(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	// When widget manager is nil, still returns enabled: true with basic widgets
	if data["enabled"] != true {
		t.Error("Expected enabled to be true (basic widgets available)")
	}
}

// Tests for Widget data endpoint

func TestWidgetDataNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/weather", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	// Returns 200 OK with error message (graceful degradation)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// Tests for Instant Answer endpoint

func TestInstantAnswerEndpointNoQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instant", nil)
	w := httptest.NewRecorder()

	handler.handleInstantAnswer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestInstantAnswerEndpointNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/instant?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleInstantAnswer(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["found"] != false {
		t.Error("Expected found to be false when instant manager is nil")
	}
}

// Tests for Related Searches endpoint

func TestRelatedSearchesEndpointNoQuery(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related", nil)
	w := httptest.NewRecorder()

	handler.handleRelatedSearches(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestRelatedSearchesEndpointNoProvider(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related?q=test", nil)
	w := httptest.NewRecorder()

	handler.handleRelatedSearches(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	if data["count"].(float64) != 0 {
		t.Error("Expected count to be 0 when related searches is nil")
	}
}

// Tests for Set methods

func TestSetWidgetManager(t *testing.T) {
	handler := newTestHandler()

	if handler.widgetManager != nil {
		t.Error("Widget manager should be nil initially")
	}

	// We can't easily create a widget manager without config, but we can test the method exists
	handler.SetWidgetManager(nil)

	if handler.widgetManager != nil {
		t.Error("Widget manager should still be nil after setting to nil")
	}
}

func TestSetInstantManager(t *testing.T) {
	handler := newTestHandler()

	if handler.instantManager != nil {
		t.Error("Instant manager should be nil initially")
	}

	handler.SetInstantManager(nil)

	if handler.instantManager != nil {
		t.Error("Instant manager should still be nil after setting to nil")
	}
}

func TestSetRelatedSearches(t *testing.T) {
	handler := newTestHandler()

	if handler.relatedSearches != nil {
		t.Error("Related searches should be nil initially")
	}

	handler.SetRelatedSearches(nil)

	if handler.relatedSearches != nil {
		t.Error("Related searches should still be nil after setting to nil")
	}
}

// Tests for text response format

func TestRespondWithFormatText(t *testing.T) {
	handler := newTestHandler()

	// Test .txt extension
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz.txt", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "status: unhealthy\n") {
		t.Errorf("Expected complete health text, got %q", body)
	}
}

func TestRespondWithFormatAcceptHeader(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, APIPrefix+"/server/healthz", nil)
	req.Header.Set("Accept", "text/plain")
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

func TestRespondWithFormatQueryParam(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz?format=text", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

// Tests for helper functions

func TestStripTxtExtension(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/v1/server/healthz.txt", "/api/v1/server/healthz"},
		{"/api/v1/info.txt", "/api/v1/info"},
		{"/api/v1/server/healthz", "/api/v1/server/healthz"},
		{"/path/to/file", "/path/to/file"},
		{"file.txt", "file"},
		{"", ""},
	}

	for _, tt := range tests {
		result := stripTxtExtension(tt.input)
		if result != tt.expected {
			t.Errorf("stripTxtExtension(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCheckDiskHealth(t *testing.T) {
	handler := newTestHandler()

	// checkDiskHealth stats config.GetDataDir(), which resolves from CLI
	// overrides/env — not from the handler's config. Point it at a temp dir
	// so the test passes regardless of the host environment.
	tmpDir := t.TempDir()
	config.SetDataDirOverride(tmpDir)
	defer config.SetDataDirOverride("")

	result := handler.checkDiskHealth()
	if result != "ok" {
		t.Errorf("checkDiskHealth() = %q, want 'ok'", result)
	}
}

func TestGetRequestsTotal(t *testing.T) {
	handler := newTestHandler()

	// With metrics disabled, should return 0
	result := handler.getRequestsTotal()
	if result != 0 {
		t.Errorf("getRequestsTotal() = %d, want 0", result)
	}
}

type fakeHealthMetrics struct {
	total   int64
	last24h int64
	active  int64
}

func (m fakeHealthMetrics) GetTotalRequests() int64       { return m.total }
func (m fakeHealthMetrics) GetRequestsLast24Hours() int64 { return m.last24h }
func (m fakeHealthMetrics) GetActiveConnections() int64   { return m.active }

type fakeHealthProvider struct {
	snapshot HealthSnapshot
}

func (p fakeHealthProvider) HealthSnapshot() HealthSnapshot { return p.snapshot }

func TestHealthProviderOverridesChecksAndStatus(t *testing.T) {
	handler := newTestHandler()
	handler.SetHealthProvider(fakeHealthProvider{snapshot: HealthSnapshot{
		Status: "unhealthy",
		Checks: ChecksInfo{Database: "error", Cache: "ok", Disk: "ok", Scheduler: "ok"},
	}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()
	handler.handleHealthz(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("health response status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	var body HealthResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body.Status != "unhealthy" || body.Checks.Database != "error" {
		t.Fatalf("health response did not use provider snapshot: %+v", body)
	}
}

func TestHealthMetricsIntegration(t *testing.T) {
	handler := newTestHandler()
	handler.SetHealthMetrics(fakeHealthMetrics{total: 12, last24h: 7, active: 3})

	if got := handler.getRequestsTotal(); got != 12 {
		t.Errorf("getRequestsTotal() = %d, want 12", got)
	}
	if got := handler.getRequests24h(); got != 7 {
		t.Errorf("getRequests24h() = %d, want 7", got)
	}
	if got := handler.getActiveConnections(); got != 3 {
		t.Errorf("getActiveConnections() = %d, want 3", got)
	}
}

func TestGetRequests24h(t *testing.T) {
	handler := newTestHandler()

	result := handler.getRequests24h()
	if result != 0 {
		t.Errorf("getRequests24h() = %d, want 0", result)
	}
}

func TestGetActiveConnections(t *testing.T) {
	handler := newTestHandler()

	result := handler.getActiveConnections()
	if result != 1 {
		t.Errorf("getActiveConnections() = %d, want 1 for the active health request", result)
	}
}

// Tests for builtin bangs

func TestGetBuiltinBangs(t *testing.T) {
	bangs := getBuiltinBangs()

	if len(bangs) == 0 {
		t.Error("Expected at least one builtin bang")
	}

	// Check that Google bang exists
	var foundGoogle bool
	for _, b := range bangs {
		if b.Shortcut == "g" {
			foundGoogle = true
			if b.Name != "Google" {
				t.Errorf("Google bang Name = %q, want 'Google'", b.Name)
			}
			if b.Category != "general" {
				t.Errorf("Google bang Category = %q, want 'general'", b.Category)
			}
			break
		}
	}

	if !foundGoogle {
		t.Error("Expected to find Google bang with shortcut 'g'")
	}
}

func TestAvailableBangsFollowLocalEngineConfiguration(t *testing.T) {
	handler := newTestHandler()
	handler.registry = engine.DefaultRegistry()
	hasBang := func(shortcut string) bool {
		for _, item := range handler.AvailableBangs() {
			if item.Shortcut == shortcut {
				return true
			}
		}
		return false
	}
	if !hasBang("gh") {
		t.Fatal("enabled GitHub engine's bang is not available")
	}
	github, err := handler.registry.Get("github")
	if err != nil {
		t.Fatal(err)
	}
	github.GetConfig().Enabled = false
	if hasBang("gh") {
		t.Fatal("GitHub bang remained available after its local engine was disabled")
	}
	if !hasBang("sp") {
		t.Fatal("external Startpage shortcut was affected by local engine configuration")
	}
	tracker, ok := github.(interface {
		RecordFailure(error)
		RecordSuccess(time.Duration)
	})
	if !ok {
		t.Fatal("GitHub engine does not expose its health tracker")
	}
	for range 3 {
		tracker.RecordFailure(model.ErrEngineUnavailable)
	}
	if hasBang("gh") {
		t.Fatal("GitHub bang remained available while its local engine was in cooldown")
	}
	github.GetConfig().Enabled = true
	tracker.RecordSuccess(time.Millisecond)
	if !hasBang("gh") {
		t.Fatal("GitHub bang did not return after the local engine recovered")
	}
}

func TestBangsEndpointWithCategory(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bangs?category=code", nil)
	w := httptest.NewRecorder()

	handler.handleBangs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	bangs, ok := data["bangs"].([]interface{})
	if !ok {
		t.Fatal("Expected bangs to be an array")
	}

	// All bangs should be from code category
	for _, b := range bangs {
		bangMap := b.(map[string]interface{})
		if bangMap["category"] != "code" {
			t.Errorf("Expected category 'code', got %v", bangMap["category"])
		}
	}
}

func TestBangsEndpointWithSearch(t *testing.T) {
	handler := newTestHandler()
	handler.registry = engine.DefaultRegistry()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/bangs?search=google", nil)
	w := httptest.NewRecorder()

	handler.handleBangs(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.OK {
		t.Error("Expected success to be true")
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	bangs, ok := data["bangs"].([]interface{})
	if !ok {
		t.Fatal("Expected bangs to be an array")
	}

	// Should find at least Google bang
	if len(bangs) == 0 {
		t.Error("Expected to find at least one bang matching 'google'")
	}
}

// Tests for response types JSON serialization

func TestHealthResponseSerialization(t *testing.T) {
	health := HealthResponse{
		Status:    "healthy",
		Version:   "1.0.0",
		GoVersion: "go1.21",
		Mode:      "production",
		Uptime:    "1h 30m",
		Timestamp: "2024-01-01T00:00:00Z",
		Checks: ChecksInfo{
			Database:  "ok",
			Cache:     "ok",
			Disk:      "ok",
			Scheduler: "ok",
		},
	}

	data, err := json.Marshal(health)
	if err != nil {
		t.Fatalf("Failed to marshal HealthResponse: %v", err)
	}
	if strings.Contains(string(data), `"cluster"`) {
		t.Errorf("HealthResponse must not advertise an unsupported cluster field: %s", data)
	}

	var unmarshaled HealthResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal HealthResponse: %v", err)
	}

	if unmarshaled.Status != health.Status {
		t.Errorf("Status mismatch: got %q, want %q", unmarshaled.Status, health.Status)
	}
	orderedKeys := []string{`"project"`, `"status"`, `"version"`, `"go_version"`, `"build"`, `"uptime"`, `"mode"`, `"timestamp"`, `"features"`, `"checks"`, `"stats"`}
	last := -1
	for _, key := range orderedKeys {
		index := strings.Index(string(data), key)
		if index <= last {
			t.Errorf("health JSON key %s was missing or out of order: %s", key, data)
		}
		last = index
	}
}

func TestHealthResponseNilI2PProviderIsNone(t *testing.T) {
	handler := newTestHandler()
	health := handler.buildHealthResponse()
	if got := health.Features.Tor.State; got != "disabled" {
		t.Fatalf("nil Tor provider state = %q, want disabled", got)
	}
	if got := health.Features.I2P.Provider; got != "none" {
		t.Fatalf("nil I2P manager provider = %q, want none", got)
	}
}

func TestFormatHealthTextEmitsI2PCheckOnce(t *testing.T) {
	text := formatHealthText(HealthResponse{
		Status: "degraded",
		Checks: ChecksInfo{I2P: "degraded"},
	})
	if got := strings.Count(text, "checks.i2p:"); got != 1 {
		t.Fatalf("formatHealthText emitted checks.i2p %d times, want 1", got)
	}
}

func TestAPIHealthTextGoldenFieldOrder(t *testing.T) {
	body := formatHealthText(HealthResponse{
		Project: ProjectInfo{Name: "Search", Tagline: "Private", Description: "Metasearch"},
		Status:  "healthy", Version: "1.2.3", GoVersion: "go1.x",
		Build: BuildInfo{Commit: "abc123", Date: "2026-01-02T03:04:05Z"}, Uptime: "2h 5m", Mode: "production", Timestamp: "2026-01-02T03:04:05Z",
		Features: FeaturesInfo{Tor: TorInfo{Status: "disabled"}, I2P: I2PInfo{Status: "disabled"}},
		Checks:   ChecksInfo{Database: "ok", Cache: "ok", Disk: "ok", Scheduler: "ok", Extra: []health.Check{{Key: "engine_alpha", Status: "ok"}}},
		Stats:    StatsInfo{RequestsTotal: 1, Requests24h: 1, ActiveConns: 1, Extra: []health.Stat{{Key: "engines_total", Value: 2}}},
	})
	last := -1
	for _, marker := range []string{"project.name:", "project.tagline:", "project.description:", "status:", "version:", "go_version:", "build.commit:", "build.date:", "uptime:", "mode:", "timestamp:", "features.tor:", "features.i2p:", "features.geoip:", "checks.database:", "checks.cache:", "checks.disk:", "checks.scheduler:", "checks.engine_alpha:", "stats.requests_total:", "stats.requests_24h:", "stats.active_connections:", "stats.engines_total:"} {
		index := strings.Index(body, marker)
		if index <= last {
			t.Fatalf("API text marker %q missing or out of order in:\n%s", marker, body)
		}
		last = index
	}
}

func TestSearchResponseSerialization(t *testing.T) {
	resp := SearchResponse{
		Query:      "test",
		Category:   "general",
		Results:    []SearchResult{{Title: "Test", URL: "https://example.com"}},
		SearchTime: 100.5,
		Engines:    []string{"google"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal SearchResponse: %v", err)
	}

	var unmarshaled SearchResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SearchResponse: %v", err)
	}

	if unmarshaled.Query != resp.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, resp.Query)
	}
	if len(unmarshaled.Results) != 1 {
		t.Errorf("Results count mismatch: got %d, want 1", len(unmarshaled.Results))
	}
}

func TestBangInfoSerialization(t *testing.T) {
	bang := BangInfo{
		Shortcut:    "g",
		Name:        "Google",
		URL:         "https://google.com?q={query}",
		Category:    "general",
		Description: "Google Search",
		Aliases:     []string{"google"},
	}

	data, err := json.Marshal(bang)
	if err != nil {
		t.Fatalf("Failed to marshal BangInfo: %v", err)
	}

	var unmarshaled BangInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal BangInfo: %v", err)
	}

	if unmarshaled.Shortcut != bang.Shortcut {
		t.Errorf("Shortcut mismatch: got %q, want %q", unmarshaled.Shortcut, bang.Shortcut)
	}
}

// Tests for constants

func TestAPIConstants(t *testing.T) {
	if APIVersion != "v1" {
		t.Errorf("APIVersion = %q, want 'v1'", APIVersion)
	}
	if APIPrefix != "/api/v1" {
		t.Errorf("APIPrefix = %q, want '/api/v1'", APIPrefix)
	}
}

// Tests for client IP resolution via httputil.GetClientIP.
// Per AI.md PART 12: headers honored only from trusted proxies (private ranges + configured list).
// Priority (from trusted proxy): CF-Connecting-IP → True-Client-IP → X-Real-IP → X-Forwarded-For → X-Client-IP → RemoteAddr

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		// Untrusted peer: all forwarded headers must be ignored.
		{
			name:       "untrusted peer ignores X-Forwarded-For",
			remoteAddr: "203.0.113.99:1234",
			headers:    map[string]string{"X-Forwarded-For": "1.2.3.4"},
			want:       "203.0.113.99",
		},
		{
			name:       "untrusted peer ignores X-Real-IP",
			remoteAddr: "203.0.113.99:1234",
			headers:    map[string]string{"X-Real-IP": "1.2.3.4"},
			want:       "203.0.113.99",
		},
		// Trusted peer (RFC1918): headers honored in correct priority order.
		{
			name:       "trusted peer uses CF-Connecting-IP first",
			remoteAddr: "10.0.0.1:1234",
			headers: map[string]string{
				"CF-Connecting-IP": "203.0.113.1",
				"True-Client-IP":   "203.0.113.2",
				"X-Real-IP":        "203.0.113.3",
				"X-Forwarded-For":  "203.0.113.4",
			},
			want: "203.0.113.1",
		},
		{
			name:       "trusted peer uses True-Client-IP when no CF header",
			remoteAddr: "10.0.0.1:1234",
			headers: map[string]string{
				"True-Client-IP":  "203.0.113.2",
				"X-Real-IP":       "203.0.113.3",
				"X-Forwarded-For": "203.0.113.4",
			},
			want: "203.0.113.2",
		},
		{
			name:       "trusted peer X-Real-IP beats X-Forwarded-For",
			remoteAddr: "10.0.0.1:1234",
			headers: map[string]string{
				"X-Real-IP":       "203.0.113.3",
				"X-Forwarded-For": "203.0.113.4, 198.51.100.1",
			},
			want: "203.0.113.3",
		},
		{
			name:       "trusted peer X-Forwarded-For leftmost IP",
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.4, 198.51.100.1"},
			want:       "203.0.113.4",
		},
		{
			name:       "trusted peer X-Forwarded-For with spaces",
			remoteAddr: "172.16.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "  203.0.113.5  , 198.51.100.1"},
			want:       "203.0.113.5",
		},
		{
			name:       "trusted peer X-Client-IP when no other headers",
			remoteAddr: "192.168.1.1:1234",
			headers:    map[string]string{"X-Client-IP": "203.0.113.6"},
			want:       "203.0.113.6",
		},
		{
			name:       "trusted peer falls back to RemoteAddr with no headers",
			remoteAddr: "10.0.0.1:8080",
			want:       "10.0.0.1",
		},
		{
			name:       "loopback IPv6 falls back to RemoteAddr",
			remoteAddr: "[::1]:12345",
			want:       "::1",
		},
		{
			name:       "public RemoteAddr no headers",
			remoteAddr: "192.0.2.1:1234",
			want:       "192.0.2.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			got := httputil.GetClientIP(req)
			if got != tt.want {
				t.Errorf("GetClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Tests for EngineInfo serialization

func TestEngineInfoSerialization(t *testing.T) {
	engine := EngineInfo{
		ID:          "google",
		Name:        "Google",
		Enabled:     true,
		Priority:    1,
		Categories:  []string{"general", "images"},
		Description: "Google Search Engine",
		Homepage:    "https://google.com",
	}

	data, err := json.Marshal(engine)
	if err != nil {
		t.Fatalf("Failed to marshal EngineInfo: %v", err)
	}

	var unmarshaled EngineInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal EngineInfo: %v", err)
	}

	if unmarshaled.ID != engine.ID {
		t.Errorf("ID mismatch: got %q, want %q", unmarshaled.ID, engine.ID)
	}
	if unmarshaled.Name != engine.Name {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, engine.Name)
	}
	if len(unmarshaled.Categories) != len(engine.Categories) {
		t.Errorf("Categories length mismatch: got %d, want %d", len(unmarshaled.Categories), len(engine.Categories))
	}
	if unmarshaled.Enabled != engine.Enabled {
		t.Errorf("Enabled mismatch: got %v, want %v", unmarshaled.Enabled, engine.Enabled)
	}
	if unmarshaled.Priority != engine.Priority {
		t.Errorf("Priority mismatch: got %d, want %d", unmarshaled.Priority, engine.Priority)
	}
}

// Tests for SearchResult serialization

func TestSearchResultSerialization(t *testing.T) {
	result := SearchResult{
		Title:       "Test Result",
		URL:         "https://example.com/page",
		Description: "This is a test result description",
		Engine:      "google",
		Score:       0.95,
		Category:    "general",
		Thumbnail:   "https://example.com/thumb.png",
		Duration:    634,
		ViewCount:   1000,
		Author:      "Fixture Channel",
		EmbedURL:    "https://www.youtube-nocookie.com/embed/abc123",
		Date:        "2024-01-01",
		Domain:      "example.com",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal SearchResult: %v", err)
	}

	var unmarshaled SearchResult
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SearchResult: %v", err)
	}

	if unmarshaled.Title != result.Title {
		t.Errorf("Title mismatch: got %q, want %q", unmarshaled.Title, result.Title)
	}
	if unmarshaled.URL != result.URL {
		t.Errorf("URL mismatch: got %q, want %q", unmarshaled.URL, result.URL)
	}
	if unmarshaled.Score != result.Score {
		t.Errorf("Score mismatch: got %f, want %f", unmarshaled.Score, result.Score)
	}
	if unmarshaled.Domain != result.Domain {
		t.Errorf("Domain mismatch: got %q, want %q", unmarshaled.Domain, result.Domain)
	}
	if unmarshaled.Duration != result.Duration || unmarshaled.ViewCount != result.ViewCount || unmarshaled.Author != result.Author || unmarshaled.EmbedURL != result.EmbedURL {
		t.Errorf("video metadata mismatch: got duration=%d views=%d author=%q embed=%q", unmarshaled.Duration, unmarshaled.ViewCount, unmarshaled.Author, unmarshaled.EmbedURL)
	}
}

// Tests for APIMeta serialization

func TestAPIMetaSerialization(t *testing.T) {
	meta := APIMeta{
		RequestID:   "req_123abc",
		ProcessTime: 15.5,
		Version:     "v1",
	}

	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("Failed to marshal APIMeta: %v", err)
	}

	var unmarshaled APIMeta
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal APIMeta: %v", err)
	}

	if unmarshaled.Version != meta.Version {
		t.Errorf("Version mismatch: got %q, want %q", unmarshaled.Version, meta.Version)
	}
	if unmarshaled.RequestID != meta.RequestID {
		t.Errorf("RequestID mismatch: got %q, want %q", unmarshaled.RequestID, meta.RequestID)
	}
	if unmarshaled.ProcessTime != meta.ProcessTime {
		t.Errorf("ProcessTime mismatch: got %f, want %f", unmarshaled.ProcessTime, meta.ProcessTime)
	}
}

// Tests for EnginesSummary serialization

func TestEnginesSummarySerialization(t *testing.T) {
	summary := EnginesSummary{
		Total:   10,
		Enabled: 5,
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Failed to marshal EnginesSummary: %v", err)
	}

	var unmarshaled EnginesSummary
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal EnginesSummary: %v", err)
	}

	if unmarshaled.Total != summary.Total {
		t.Errorf("Total mismatch: got %d, want %d", unmarshaled.Total, summary.Total)
	}
	if unmarshaled.Enabled != summary.Enabled {
		t.Errorf("Enabled mismatch: got %d, want %d", unmarshaled.Enabled, summary.Enabled)
	}
}

// Tests for SystemInfo serialization

func TestSystemInfoSerialization(t *testing.T) {
	info := SystemInfo{
		GoVersion:    "go1.21.0",
		NumCPU:       8,
		NumGoroutine: 100,
		MemAlloc:     "256 MB",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Failed to marshal SystemInfo: %v", err)
	}

	var unmarshaled SystemInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SystemInfo: %v", err)
	}

	if unmarshaled.GoVersion != info.GoVersion {
		t.Errorf("GoVersion mismatch: got %q, want %q", unmarshaled.GoVersion, info.GoVersion)
	}
	if unmarshaled.NumCPU != info.NumCPU {
		t.Errorf("NumCPU mismatch: got %d, want %d", unmarshaled.NumCPU, info.NumCPU)
	}
	if unmarshaled.NumGoroutine != info.NumGoroutine {
		t.Errorf("NumGoroutine mismatch: got %d, want %d", unmarshaled.NumGoroutine, info.NumGoroutine)
	}
	if unmarshaled.MemAlloc != info.MemAlloc {
		t.Errorf("MemAlloc mismatch: got %q, want %q", unmarshaled.MemAlloc, info.MemAlloc)
	}
}

// Tests for SearchRequest serialization

func TestSearchRequestSerialization(t *testing.T) {
	safeSearch := 1
	req := SearchRequest{
		Query:      "golang testing",
		Category:   "general",
		Page:       1,
		Limit:      20,
		Engines:    []string{"google", "bing"},
		SafeSearch: &safeSearch,
		TimeRange:  "week",
		Language:   "en",
		Region:     "us",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal SearchRequest: %v", err)
	}

	var unmarshaled SearchRequest
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal SearchRequest: %v", err)
	}

	if unmarshaled.Query != req.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, req.Query)
	}
	if unmarshaled.Category != req.Category {
		t.Errorf("Category mismatch: got %q, want %q", unmarshaled.Category, req.Category)
	}
	if unmarshaled.Page != req.Page {
		t.Errorf("Page mismatch: got %d, want %d", unmarshaled.Page, req.Page)
	}
	if unmarshaled.Limit != req.Limit {
		t.Errorf("Limit mismatch: got %d, want %d", unmarshaled.Limit, req.Limit)
	}
	if len(unmarshaled.Engines) != len(req.Engines) {
		t.Errorf("Engines length mismatch: got %d, want %d", len(unmarshaled.Engines), len(req.Engines))
	}
	if unmarshaled.SafeSearch == nil || *unmarshaled.SafeSearch != *req.SafeSearch {
		t.Errorf("SafeSearch mismatch: got %v, want %v", unmarshaled.SafeSearch, req.SafeSearch)
	}
	if unmarshaled.Region != req.Region {
		t.Errorf("Region mismatch: got %q, want %q", unmarshaled.Region, req.Region)
	}
}

func TestSplitSearchEngineFilters(t *testing.T) {
	got := splitSearchEngineFilters([]string{"google,bing", " duckduckgo ", "", "brave,,"})
	want := []string{"google", "bing", "duckduckgo", "brave"}
	if len(got) != len(want) {
		t.Fatalf("engine filters = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("engine filters[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Tests for CategoryInfo serialization

func TestCategoryInfoSerialization(t *testing.T) {
	category := CategoryInfo{
		ID:          "general",
		Name:        "General",
		Description: "General web search",
		Icon:        "search",
	}

	data, err := json.Marshal(category)
	if err != nil {
		t.Fatalf("Failed to marshal CategoryInfo: %v", err)
	}

	var unmarshaled CategoryInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal CategoryInfo: %v", err)
	}

	if unmarshaled.ID != category.ID {
		t.Errorf("ID mismatch: got %q, want %q", unmarshaled.ID, category.ID)
	}
	if unmarshaled.Name != category.Name {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, category.Name)
	}
	if unmarshaled.Description != category.Description {
		t.Errorf("Description mismatch: got %q, want %q", unmarshaled.Description, category.Description)
	}
	if unmarshaled.Icon != category.Icon {
		t.Errorf("Icon mismatch: got %q, want %q", unmarshaled.Icon, category.Icon)
	}
}

// Tests for BuildInfo serialization

func TestBuildInfoSerialization(t *testing.T) {
	build := BuildInfo{
		Commit: "abc123d",
		Date:   "2024-01-15T10:00:00Z",
	}

	data, err := json.Marshal(build)
	if err != nil {
		t.Fatalf("Failed to marshal BuildInfo: %v", err)
	}

	var unmarshaled BuildInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal BuildInfo: %v", err)
	}

	if unmarshaled.Commit != build.Commit {
		t.Errorf("Commit mismatch: got %q, want %q", unmarshaled.Commit, build.Commit)
	}
	if unmarshaled.Date != build.Date {
		t.Errorf("Date mismatch: got %q, want %q", unmarshaled.Date, build.Date)
	}
}

// Tests for StatsInfo serialization

func TestStatsInfoSerialization(t *testing.T) {
	stats := StatsInfo{
		RequestsTotal: 100000,
		Requests24h:   5000,
		ActiveConns:   42,
	}

	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("Failed to marshal StatsInfo: %v", err)
	}

	var unmarshaled StatsInfo
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal StatsInfo: %v", err)
	}

	if unmarshaled.RequestsTotal != stats.RequestsTotal {
		t.Errorf("RequestsTotal mismatch: got %d, want %d", unmarshaled.RequestsTotal, stats.RequestsTotal)
	}
	if unmarshaled.Requests24h != stats.Requests24h {
		t.Errorf("Requests24h mismatch: got %d, want %d", unmarshaled.Requests24h, stats.Requests24h)
	}
	if unmarshaled.ActiveConns != stats.ActiveConns {
		t.Errorf("ActiveConns mismatch: got %d, want %d", unmarshaled.ActiveConns, stats.ActiveConns)
	}
}

// Tests for InfoResponse serialization

func TestInfoResponseSerialization(t *testing.T) {
	resp := InfoResponse{
		Name:        "Search API",
		Version:     "1.0.0",
		Description: "A search API server",
		Uptime:      "24h 30m",
		Mode:        "production",
		Engines: EnginesSummary{
			Total:   10,
			Enabled: 5,
		},
		System: SystemInfo{
			GoVersion:    "go1.21.0",
			NumCPU:       8,
			NumGoroutine: 50,
			MemAlloc:     "128 MB",
		},
		Features: map[string]bool{
			"multi_user": true,
			"tor":        false,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal InfoResponse: %v", err)
	}

	var unmarshaled InfoResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal InfoResponse: %v", err)
	}

	if unmarshaled.Name != resp.Name {
		t.Errorf("Name mismatch: got %q, want %q", unmarshaled.Name, resp.Name)
	}
	if unmarshaled.Version != resp.Version {
		t.Errorf("Version mismatch: got %q, want %q", unmarshaled.Version, resp.Version)
	}
	if unmarshaled.Mode != resp.Mode {
		t.Errorf("Mode mismatch: got %q, want %q", unmarshaled.Mode, resp.Mode)
	}
}

// Tests for formatDuration edge cases

func TestFormatDurationDaysOnly(t *testing.T) {
	handler := &Handler{}

	// Test exactly 24 hours
	result := handler.formatDuration(24 * time.Hour)
	if result != "1d" {
		t.Errorf("formatDuration(24h) = %q, want %q", result, "1d")
	}

	// Test multiple days
	result = handler.formatDuration(72 * time.Hour)
	if result != "3d" {
		t.Errorf("formatDuration(72h) = %q, want %q", result, "3d")
	}
}

func TestFormatDurationHoursOnly(t *testing.T) {
	handler := &Handler{}

	// Test exactly 1 hour
	result := handler.formatDuration(1 * time.Hour)
	if result != "1h" {
		t.Errorf("formatDuration(1h) = %q, want %q", result, "1h")
	}

	// Test 23 hours
	result = handler.formatDuration(23 * time.Hour)
	if result != "23h" {
		t.Errorf("formatDuration(23h) = %q, want %q", result, "23h")
	}
}

// Tests for formatBytes edge cases

func TestFormatBytesEdgeCases(t *testing.T) {
	handler := &Handler{}

	tests := []struct {
		bytes    uint64
		expected string
	}{
		{500, "500 B"},
		{1023, "1023 B"},
		{1536, "1.5 KB"},
		{1572864, "1.5 MB"},
		{1610612736, "1.5 GB"},
		{1099511627776, "1.0 TB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := handler.formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, result, tt.expected)
			}
		})
	}
}

// Tests for extractDomain edge cases

func TestExtractDomainEdgeCases(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"", ""},
		{"not-a-url", "not-a-url"},
		// Only http/https schemes are parsed fully
		{"ftp://ftp.example.com/file", "ftp:"},
		{"https://sub.domain.example.com/path/to/page?q=test", "sub.domain.example.com"},
		{"http://localhost:8080/api", "localhost:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := extractDomain(tt.url)
			if result != tt.expected {
				t.Errorf("extractDomain(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// Tests for NewHandler

func TestNewHandlerInitialization(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Server",
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)

	handler := NewHandler(cfg, registry, aggregator)

	if handler == nil {
		t.Fatal("NewHandler returned nil")
	}
	if handler.config != cfg {
		t.Error("Config not set correctly")
	}
	if handler.registry != registry {
		t.Error("Registry not set correctly")
	}
	if handler.aggregator != aggregator {
		t.Error("Aggregator not set correctly")
	}
	if handler.startTime.IsZero() {
		t.Error("startTime should be set")
	}
}

// Tests for AutodiscoverResponse serialization

func TestAutodiscoverResponseSerialization(t *testing.T) {
	resp := AutodiscoverResponse{
		Name: "Search Server", Version: "1.0.0",
		API:         AutodiscoverAPIInfo{CurrentVersion: "v1"},
		URLs:        []string{"https://search.example.com"},
		CLIVersions: map[string]CLIBinaryInfo{},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal AutodiscoverResponse: %v", err)
	}

	var unmarshaled AutodiscoverResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal AutodiscoverResponse: %v", err)
	}

	if unmarshaled.Name != resp.Name || unmarshaled.Version != resp.Version {
		t.Errorf("identity mismatch: got %q/%q, want %q/%q", unmarshaled.Name, unmarshaled.Version, resp.Name, resp.Version)
	}
	if unmarshaled.API.CurrentVersion != resp.API.CurrentVersion {
		t.Errorf("API.CurrentVersion = %q, want %q", unmarshaled.API.CurrentVersion, resp.API.CurrentVersion)
	}
	if !reflect.DeepEqual(unmarshaled.URLs, resp.URLs) {
		t.Errorf("URLs = %#v, want %#v", unmarshaled.URLs, resp.URLs)
	}
}

// ============================================================================
// Tests for OpenAPI/Swagger handlers (openapi_handler.go)
// ============================================================================

func TestServeOpenAPISpec(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, APIPrefix+"/server/swagger", nil)
	w := httptest.NewRecorder()

	handler.ServeOpenAPISpec(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected Content-Type application/json, got %q", contentType)
	}

	// Check CORS header
	cors := w.Header().Get("Access-Control-Allow-Origin")
	if cors != "*" {
		t.Errorf("Expected CORS header '*', got %q", cors)
	}

	// Check Cache-Control header
	cache := w.Header().Get("Cache-Control")
	if cache != "public, max-age=3600" {
		t.Errorf("Expected Cache-Control 'public, max-age=3600', got %q", cache)
	}
}

func TestServeSwaggerUI(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/server/docs/swagger", nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()

	handler.ServeSwaggerUI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Expected Content-Type text/html, got %q", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("Expected Swagger UI HTML content")
	}
}

// TestServeSwaggerUISpecURL verifies the UI fetches the spec from the
// canonical /api/swagger alias per AI.md PART 14 Root-Level Endpoints.
func TestServeSwaggerUISpecURL(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/server/docs/swagger", nil)
	req.Host = "api.example.com"
	w := httptest.NewRecorder()

	handler.ServeSwaggerUI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "http://api.example.com/api/v1/server/swagger") {
		t.Error("Expected spec URL pointing at the canonical versioned OpenAPI route")
	}
	if strings.Contains(body, "/openapi.json") {
		t.Error("Swagger UI must not reference the removed /openapi.json path")
	}
}

func TestRegisterOpenAPIRoutes(t *testing.T) {
	handler := newTestHandler()
	mux := chi.NewRouter()

	handler.RegisterOpenAPIRoutes(mux)

	// Test that routes are registered by making requests
	tests := []struct {
		path   string
		status int
	}{
		{"/server/docs/swagger", http.StatusNotFound},
		{APIPrefix + "/server/swagger", http.StatusOK},
		{"/api/swagger", http.StatusOK},
		{"/openapi", http.StatusNotFound},
		{"/openapi.json", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Host = "localhost"
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.status {
				t.Errorf("Expected status %d for %s, got %d", tt.status, tt.path, w.Code)
			}
		})
	}
}

// ============================================================================
// Additional api.go tests for edge cases
// ============================================================================

func TestSearchEndpointPOST(t *testing.T) {
	handler := newTestHandler()

	body := `{"query": "test", "category": "images", "page": 1, "limit": 10}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	// Response should be valid JSON
	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
}

func TestSearchEndpointPOSTInvalidJSON(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/search", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestSearchEndpointAllCategories(t *testing.T) {
	handler := newTestHandler()

	categories := []string{"general", "images", "videos", "news", "maps", "files", "music", "science", "it", "social", "unknown"}
	for _, cat := range categories {
		t.Run(cat, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&category="+cat, nil)
			w := httptest.NewRecorder()
			handler.handleSearch(w, req)

			var response APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
		})
	}
}

func TestSearchEndpointLimitBounds(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		limit    string
		expected int
	}{
		// Should default to 20
		{"0", 20},
		// Should default to 20
		{"-1", 20},
		// Should cap to 20 (>100 triggers default)
		{"200", 20},
		// Valid limit
		{"50", 50},
	}

	for _, tt := range tests {
		t.Run(tt.limit, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&limit="+tt.limit, nil)
			w := httptest.NewRecorder()
			handler.handleSearch(w, req)

			var response APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.OK {
				data, ok := response.Data.(map[string]interface{})
				if ok && data["limit"] != nil {
					if int(data["limit"].(float64)) != tt.expected {
						t.Errorf("Expected limit %d, got %v", tt.expected, data["limit"])
					}
				}
			}
		})
	}
}

func TestSearchEndpointPageBounds(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		page     string
		expected int
	}{
		// Should default to 1
		{"0", 1},
		// Should default to 1
		{"-1", 1},
		// Valid page
		{"5", 5},
	}

	for _, tt := range tests {
		t.Run(tt.page, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test&page="+tt.page, nil)
			w := httptest.NewRecorder()
			handler.handleSearch(w, req)

			var response APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.OK {
				data, ok := response.Data.(map[string]interface{})
				if ok && data["page"] != nil {
					if int(data["page"].(float64)) != tt.expected {
						t.Errorf("Expected page %d, got %v", tt.expected, data["page"])
					}
				}
			}
		})
	}
}

func TestSearchEndpointQueryTrimming(t *testing.T) {
	handler := newTestHandler()

	// Test that whitespace-only query is rejected
	// Use URL-encoded spaces (%20) to ensure proper parsing
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=%20%20%20", nil)
	w := httptest.NewRecorder()
	handler.handleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d for whitespace query, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHealthzMaintenanceMode(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title:           "Test Search",
			Description:     "Test Description",
			Mode:            "production",
			MaintenanceMode: true,
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d for maintenance mode, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != "maintenance" {
		t.Errorf("Expected status 'maintenance', got %v", response.Status)
	}
}

func TestHealthzTextFormatError(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title:           "Test Search",
			MaintenanceMode: true,
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz.txt", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "status: maintenance\n") {
		t.Errorf("Expected full maintenance health text, got %q", body)
	}
}

func TestRespondWithFormatTxtParam(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz?format=txt", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

func TestRespondWithFormatPlainParam(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz?format=plain", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain", contentType)
	}
}

func TestRespondWithFormatJSONOverride(t *testing.T) {
	handler := newTestHandler()

	// Accept header with both text/plain and application/json should use JSON
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/healthz", nil)
	req.Header.Set("Accept", "text/plain, application/json")
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json when both are accepted", contentType)
	}
}

func TestInfoEndpointDetails(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	w := httptest.NewRecorder()

	handler.handleInfo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatal("Expected data to be a map")
	}

	// Check system info is present
	system, ok := data["system"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected system info")
	}

	if system["go_version"] == nil {
		t.Error("Expected go_version in system info")
	}
	if system["num_cpu"] == nil {
		t.Error("Expected num_cpu in system info")
	}
}

func TestRelatedSearchesWithLimit(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related?q=test&limit=5", nil)
	w := httptest.NewRecorder()

	handler.handleRelatedSearches(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRelatedSearchesInvalidLimit(t *testing.T) {
	handler := newTestHandler()

	// Test with invalid limit values
	tests := []string{"-1", "0", "100", "abc"}
	for _, limit := range tests {
		t.Run(limit, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search/related?q=test&limit="+limit, nil)
			w := httptest.NewRecorder()

			handler.handleRelatedSearches(w, req)

			// Should still return OK (uses default limit)
			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
			}
		})
	}
}

func TestAutodiscoverHTTPS(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/autodiscover", nil)
	req.Host = "search.example.com"
	// Simulate TLS by setting TLS field (would need to mock properly for full test)
	w := httptest.NewRecorder()

	handler.handleAutodiscover(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestRegisterRoutes(t *testing.T) {
	handler := newTestHandler()
	mux := chi.NewRouter()

	handler.RegisterRoutes(mux)

	// Test all registered routes
	routes := []struct {
		path   string
		method string
	}{
		{"/api/autodiscover", http.MethodGet},
		{"/api/v1/server/healthz", http.MethodGet},
		{"/api/v1/server/healthz.txt", http.MethodGet},
		{"/api/v1/server/auth/check", http.MethodGet},
		{"/api/v1/info", http.MethodGet},
		{"/api/v1/categories", http.MethodGet},
		{"/api/v1/engines", http.MethodGet},
		{"/api/v1/bangs", http.MethodGet},
		{"/api/v1/autocomplete", http.MethodGet},
		{"/api/v1/widgets", http.MethodGet},
		{"/api/v1/instant", http.MethodGet},
		{"/api/v1/search/related", http.MethodGet},
		{APIPrefix + "/server/geoip/ips.txt", http.MethodGet},
		{APIPrefix + "/server/geoip/ips/203.0.113.1.txt", http.MethodGet},
	}

	for _, route := range routes {
		t.Run(route.path, func(t *testing.T) {
			req := httptest.NewRequest(route.method, route.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// All routes should return either 200, 400, or 503 (but not 404)
			if w.Code == http.StatusNotFound {
				t.Errorf("Route %s not registered", route.path)
			}
		})
	}
}

func TestVersionedHealthzTxtAliasReturnsPlainText(t *testing.T) {
	handler := newTestHandler()
	mux := chi.NewRouter()
	handler.RegisterRoutes(mux)

	for _, path := range []string{APIPrefix + "/server/healthz.txt"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			recorder := httptest.NewRecorder()
			mux.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("health request status = %d, want %d when the required scheduler is missing", recorder.Code, http.StatusServiceUnavailable)
			}
			if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
				t.Fatalf("Content-Type = %q, want text/plain", contentType)
			}
			if !strings.Contains(recorder.Body.String(), "status: ") {
				t.Fatalf("health text response missing status field: %s", recorder.Body.String())
			}
		})
	}
}

func TestHandleAutocompletePublicMethod(t *testing.T) {
	handler := newTestHandler()
	handler.autocompleteClient.Transport = autocompleteTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		return autocompleteTestResponse(request, http.StatusOK, `["test",["test suggestion"]]`), nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/autocomplete?q=test", nil)
	w := httptest.NewRecorder()

	// Test the public HandleAutocomplete method
	handler.HandleAutocomplete(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

func TestGetRequestsTotalMetricsEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
			Metrics: config.MetricsConfig{
				Enabled: true,
			},
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	// Even with metrics enabled, returns 0 (no actual metrics implementation)
	result := handler.getRequestsTotal()
	if result != 0 {
		t.Errorf("Expected 0, got %d", result)
	}

	result24h := handler.getRequests24h()
	if result24h != 0 {
		t.Errorf("Expected 0 for 24h, got %d", result24h)
	}
}

func TestWidgetDataEmptyType(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)
	// Leave widgetManager as nil

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/", nil)
	w := httptest.NewRecorder()

	handler.handleWidgetData(w, req)

	// Returns 200 OK with error message (graceful degradation)
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}
}

// ============================================================================
// Tests for InstantAnswerResponse serialization
// ============================================================================

func TestInstantAnswerResponseSerialization(t *testing.T) {
	resp := InstantAnswerResponse{
		Query:   "weather",
		Type:    "weather",
		Title:   "Weather Forecast",
		Content: "Sunny, 72F",
		Data:    map[string]interface{}{"temp": 72, "condition": "sunny"},
		Source:  "weather_api",
		Found:   true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal InstantAnswerResponse: %v", err)
	}

	var unmarshaled InstantAnswerResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal InstantAnswerResponse: %v", err)
	}

	if unmarshaled.Query != resp.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, resp.Query)
	}
	if unmarshaled.Found != resp.Found {
		t.Errorf("Found mismatch: got %v, want %v", unmarshaled.Found, resp.Found)
	}
}

// ============================================================================
// Tests for RelatedSearchResponse serialization
// ============================================================================

func TestRelatedSearchResponseSerialization(t *testing.T) {
	resp := RelatedSearchResponse{
		Query:       "golang",
		Suggestions: []string{"golang tutorial", "golang vs python", "golang web framework"},
		Count:       3,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal RelatedSearchResponse: %v", err)
	}

	var unmarshaled RelatedSearchResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal RelatedSearchResponse: %v", err)
	}

	if unmarshaled.Query != resp.Query {
		t.Errorf("Query mismatch: got %q, want %q", unmarshaled.Query, resp.Query)
	}
	if unmarshaled.Count != resp.Count {
		t.Errorf("Count mismatch: got %d, want %d", unmarshaled.Count, resp.Count)
	}
	if len(unmarshaled.Suggestions) != len(resp.Suggestions) {
		t.Errorf("Suggestions length mismatch: got %d, want %d", len(unmarshaled.Suggestions), len(resp.Suggestions))
	}
}

// ============================================================================
// Tests for auth.go handlers
// ============================================================================

// ============================================================================
// Tests for getHostname
// ============================================================================

func TestGetHostname(t *testing.T) {
	hostname, err := getHostname()
	if err != nil {
		t.Errorf("getHostname() returned error: %v", err)
	}
	if hostname == "" {
		t.Error("getHostname() returned empty string")
	}
}

// emptyResultEngine is a minimal search.Engine that always returns no results.
// Used to drive aggregator into model.ErrNoResults in regression tests.
type emptyResultEngine struct {
	cfg *model.EngineConfig
}

type thumbnailResultEngine struct {
	cfg *model.EngineConfig
}

func (e *thumbnailResultEngine) Name() string        { return e.cfg.Name }
func (e *thumbnailResultEngine) DisplayName() string { return e.cfg.DisplayName }
func (e *thumbnailResultEngine) Search(_ context.Context, _ *model.Query) ([]model.Result, error) {
	return []model.Result{{Title: "Image", URL: "https://example.test/page", Engine: e.cfg.Name, Category: model.CategoryImages, Thumbnail: "https://images.example.test/thumb.jpg", ImageURL: "https://images.example.test/full.jpg", ImageWidth: 800, ImageHeight: 600, ImageFormat: "jpg", FileSize: 4096}}, nil
}
func (e *thumbnailResultEngine) IsEnabled() bool  { return e.cfg.Enabled }
func (e *thumbnailResultEngine) GetPriority() int { return e.cfg.Priority }
func (e *thumbnailResultEngine) SupportsCategory(c model.Category) bool {
	return e.cfg.SupportsCategory(c)
}
func (e *thumbnailResultEngine) GetConfig() *model.EngineConfig { return e.cfg }

func TestSearchEndpointRewritesThumbnailWhenConfigured(t *testing.T) {
	cfg := &config.Config{Server: config.ServerConfig{Mode: "development"}}
	resultEngine := &thumbnailResultEngine{cfg: model.NewEngineConfig("images")}
	resultEngine.cfg.Enabled = true
	resultEngine.cfg.Categories = []string{string(model.CategoryImages)}
	handler := NewHandler(cfg, engine.NewRegistry(), search.NewAggregatorSimple([]search.Engine{resultEngine}, 5*time.Second))
	handler.SetThumbnailRewriter(func(string) string { return "/image-proxy?url=signed" })

	request := httptest.NewRequest(http.MethodGet, APIPrefix+"/search?q=example&category=images", nil)
	response := httptest.NewRecorder()
	handler.handleSearch(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d, want 200: %s", response.Code, response.Body.String())
	}
	var envelope struct {
		Data SearchResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if len(envelope.Data.Results) != 1 || envelope.Data.Results[0].Thumbnail != "/image-proxy?url=signed" || envelope.Data.Results[0].ImageURL != "/image-proxy?url=signed" || envelope.Data.Results[0].ImageWidth != 800 || envelope.Data.Results[0].ImageHeight != 600 || envelope.Data.Results[0].ImageFormat != "jpg" || envelope.Data.Results[0].FileSize != 4096 {
		t.Fatalf("API thumbnail = %#v, want rewritten same-origin URL", envelope.Data.Results)
	}
}

func (e *emptyResultEngine) Name() string        { return e.cfg.Name }
func (e *emptyResultEngine) DisplayName() string { return e.cfg.DisplayName }
func (e *emptyResultEngine) Search(ctx context.Context, q *model.Query) ([]model.Result, error) {
	return []model.Result{}, nil
}
func (e *emptyResultEngine) IsEnabled() bool                        { return e.cfg.Enabled }
func (e *emptyResultEngine) GetPriority() int                       { return e.cfg.Priority }
func (e *emptyResultEngine) SupportsCategory(c model.Category) bool { return e.cfg.SupportsCategory(c) }
func (e *emptyResultEngine) GetConfig() *model.EngineConfig         { return e.cfg }

func newHandlerWithEmptyResults() *Handler {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title:       "Test Search",
			Description: "Test Description",
			Mode:        "development",
		},
	}
	registry := engine.NewRegistry()
	eng := &emptyResultEngine{cfg: model.NewEngineConfig("empty")}
	aggregator := search.NewAggregatorSimple([]search.Engine{eng}, 5*time.Second)
	return NewHandler(cfg, registry, aggregator)
}

// TestSearchEndpointEmptyResultsNotFatal regresses the quotes-500 bug:
// when a query yields zero results (e.g. q="") the aggregator returns
// model.ErrNoResults; the API must still respond 200 OK with an empty
// result set rather than 500 (per AI.md PART 9: recoverable errors and
// PART 14: API success envelope).
func TestSearchEndpointEmptyResultsNotFatal(t *testing.T) {
	handler := newHandlerWithEmptyResults()

	cases := []string{`q=%22%22`, `q=foo`, `q=%22%22%22%22`}
	for _, qs := range cases {
		t.Run(qs, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/search?"+qs, nil)
			w := httptest.NewRecorder()

			handler.handleSearch(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("Expected status %d for empty-results query, got %d", http.StatusOK, w.Code)
			}

			var response APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}
			if !response.OK {
				t.Errorf("Expected OK=true for empty-results query, got OK=false (error=%q)", response.Error)
			}
		})
	}
}

// ============================================================================
// Tests for server info page handlers (handleServerAbout, Privacy, Help, Terms)
// ============================================================================

func TestHandleServerAbout(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/about", nil)
	w := httptest.NewRecorder()

	handler.handleServerAbout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	responseBody := w.Body.Bytes()
	var resp APIResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["title"] == nil {
		t.Error("expected title in response")
	}
	if data["sections"] == nil {
		t.Error("expected sections in response")
	}
}

func TestHandleServerAboutMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/about", nil)
	w := httptest.NewRecorder()

	handler.handleServerAbout(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleServerAboutFallbackTitle(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "FallbackTitle",
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/about", nil)
	w := httptest.NewRecorder()

	handler.handleServerAbout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "FallbackTitle") {
		preview := body
		if len(preview) > 200 {
			preview = preview[:200]
		}
		t.Errorf("expected FallbackTitle in body, got %q", preview)
	}
}

func TestHandleServerPrivacy(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/privacy", nil)
	w := httptest.NewRecorder()

	handler.handleServerPrivacy(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	responseBody := w.Body.Bytes()
	var resp APIResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["title"] != "Privacy Policy" {
		t.Errorf("title = %v, want 'Privacy Policy'", data["title"])
	}
	sections, ok := data["sections"].([]interface{})
	if !ok || len(sections) == 0 {
		t.Error("expected sections slice with at least one entry")
	}
}

func TestHandleServerPrivacyMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/server/privacy", nil)
	w := httptest.NewRecorder()

	handler.handleServerPrivacy(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleServerHelp(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/help", nil)
	w := httptest.NewRecorder()

	handler.handleServerHelp(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["title"] != "Help & Documentation" {
		t.Errorf("title = %v, want 'Help & Documentation'", data["title"])
	}
}

func TestHandleServerHelpUsesAcceptLanguage(t *testing.T) {
	handler := newTestHandler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/server/help", nil)
	request.Header.Set("Accept-Language", "es")
	response := httptest.NewRecorder()
	handler.handleServerHelp(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var payload struct {
		Data struct {
			Title    string `json:"title"`
			Sections []struct {
				Title string `json:"title"`
			} `json:"sections"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data.Title != "Ayuda y documentación" {
		t.Errorf("title = %q, want Spanish help title", payload.Data.Title)
	}
	if len(payload.Data.Sections) == 0 || payload.Data.Sections[0].Title != "Primeros pasos" {
		t.Errorf("first help section = %#v, want Spanish getting-started title", payload.Data.Sections)
	}
}

func TestHandleServerHelpMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/server/help", nil)
	w := httptest.NewRecorder()

	handler.handleServerHelp(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleServerTerms(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/terms", nil)
	w := httptest.NewRecorder()

	handler.handleServerTerms(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["title"] != "Terms of Service" {
		t.Errorf("title = %v, want 'Terms of Service'", data["title"])
	}
	sections, ok := data["sections"].([]interface{})
	if !ok || len(sections) == 0 {
		t.Error("expected sections with entries")
	}
}

func TestHandleServerTermsMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/server/terms", nil)
	w := httptest.NewRecorder()

	handler.handleServerTerms(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// ============================================================================
// Tests for handleServerContact
// ============================================================================

func TestHandleServerContactGETJSON(t *testing.T) {
	handler := newTestHandler()
	handler.config.Server.Pages.Contact.Enabled = true

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/contact", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	handler.handleServerContact(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	responseBody := w.Body.Bytes()
	var resp APIResponse
	if err := json.Unmarshal(responseBody, &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}
}

func TestHandleServerContactGETHTMLAcceptStillReturnsAPIData(t *testing.T) {
	handler := newTestHandler()
	handler.config.Server.Pages.Contact.Enabled = true

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/contact", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	handler.handleServerContact(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleServerContactMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()
	handler.config.Server.Pages.Contact.Enabled = true

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/server/contact", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	handler.handleServerContact(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleServerContactPOSTDisabled(t *testing.T) {
	handler := newTestHandler()

	body := `{"name":"Test","email":"test@example.com","subject":"Hello","message":"World"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	handler.handleServerContact(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (contact disabled by default)", http.StatusNotFound, w.Code)
	}
}

func TestHandleServerContactPOSTEnabled(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
			Contact: config.ContactConfig{
				General: config.ContactRoleConfig{Email: "contact@example.com"},
			},
			Pages: config.PagesConfig{Contact: config.ContactPageConfig{Enabled: true}},
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)
	submitter := &testContactSubmitter{}
	handler.SetContactSubmitter(submitter)
	handler.SetContactCaptchaProvider(testContactCaptcha{})

	cfg.Server.Pages.Contact.Antispam.MinFillSeconds = 0
	bodyFields := map[string]interface{}{
		"name": "Alice", "email": "alice@example.com", "type": "privacy", "request_kind": "other", "subject": "Bug Report", "message": "Found a bug",
		"form_token": handler.contactFormToken(nil), "captcha_id": "test-id", "captcha_answer": "2",
	}
	bodyBytes, err := json.Marshal(bodyFields)
	if err != nil {
		t.Fatal(err)
	}
	body := string(bodyBytes)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleServerContact(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Errorf("OK = false, want true; message = %q", resp.Message)
	}
	if submitter.calls != 0 {
		t.Fatalf("first contact step delivered %d messages, want none", submitter.calls)
	}
	reviewData, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("review response data = %#v", resp.Data)
	}
	reviewToken, _ := reviewData["review_token"].(string)
	bodyFields["step"] = "confirm"
	bodyFields["review_token"] = reviewToken
	bodyFields["confirmed"] = true
	confirmBody, err := json.Marshal(bodyFields)
	if err != nil {
		t.Fatal(err)
	}
	confirmRequest := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader(string(confirmBody)))
	confirmRequest.Header.Set("Content-Type", "application/json")
	confirmResponse := httptest.NewRecorder()
	handler.handleServerContact(confirmResponse, confirmRequest)
	if confirmResponse.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, want %d: %s", confirmResponse.Code, http.StatusOK, confirmResponse.Body.String())
	}
	if submitter.calls != 1 || submitter.last.Email != "alice@example.com" || submitter.last.Subject != "Bug Report" {
		t.Errorf("contact submission = %#v, calls=%d", submitter.last, submitter.calls)
	}
}

func TestHandleServerContactPOSTMissingFields(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
			Contact: config.ContactConfig{
				General: config.ContactRoleConfig{Email: "contact@example.com"},
			},
			Pages: config.PagesConfig{Contact: config.ContactPageConfig{Enabled: true}},
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	tests := []struct {
		name string
		body string
	}{
		{"missing name", `{"email":"a@b.com","subject":"Hi","message":"Hello"}`},
		{"missing email", `{"name":"Alice","subject":"Hi","message":"Hello"}`},
		{"missing subject", `{"name":"Alice","email":"a@b.com","message":"Hello"}`},
		{"missing message", `{"name":"Alice","email":"a@b.com","subject":"Hi"}`},
		{"all empty", `{"name":"","email":"","subject":"","message":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.handleServerContact(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d for %s", w.Code, http.StatusBadRequest, tt.name)
			}
		})
	}
}

func TestHandleServerContactPOSTInvalidJSON(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
			Contact: config.ContactConfig{
				General: config.ContactRoleConfig{Email: "contact@example.com"},
			},
			Pages: config.PagesConfig{Contact: config.ContactPageConfig{Enabled: true}},
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/contact", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleServerContact(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", http.StatusBadRequest, w.Code)
	}
}

// ============================================================================
// Tests for handlePreferences
// ============================================================================

func TestHandlePreferencesGET(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/server/preferences", nil)
	w := httptest.NewRecorder()

	handler.handlePreferences(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["theme"] != "dark" {
		t.Errorf("theme = %v, want dark", data["theme"])
	}
	if data["lang"] != "en" {
		t.Errorf("lang = %v, want en", data["lang"])
	}
	if prefs, ok := data["prefs"].(map[string]interface{}); !ok || prefs == nil {
		t.Errorf("prefs = %T, want a preference map", data["prefs"])
	}
}

func TestHandlePreferencesPOST(t *testing.T) {
	handler := newTestHandler()

	body := `{"theme":"dark","language":"en"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/preferences", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handlePreferences(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}
}

func TestHandlePreferencesPOSTIgnoresWebSyncCode(t *testing.T) {
	handler := newTestHandler()
	body := `{"theme":"dark","code":"theme=light&lang=fr"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/server/preferences", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handlePreferences(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var response APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data = %T, want object", response.Data)
	}
	if data["lang"] != "en" {
		t.Errorf("lang = %v, want en because code is a web-form-only control", data["lang"])
	}
	if data["theme"] != "dark" {
		t.Errorf("direct theme = %v, want dark to take precedence over code", data["theme"])
	}
}

func TestHandlePreferencesMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/server/preferences", nil)
	w := httptest.NewRecorder()

	handler.handlePreferences(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// ============================================================================
// Tests for handleFavicon
// ============================================================================

func TestHandleFaviconNoURL(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favicon", nil)
	w := httptest.NewRecorder()

	handler.handleFavicon(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("Content-Type = %q, want 'image/png' (fallback PNG)", ct)
	}
}

func TestHandleFaviconEmptyURL(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favicon?url=", nil)
	w := httptest.NewRecorder()

	handler.handleFavicon(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("Content-Type = %q, want 'image/png' (fallback)", ct)
	}
}

func TestHandleFaviconRemoteError(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favicon?url=https://nonexistent.invalid/page", nil)
	w := httptest.NewRecorder()

	handler.handleFavicon(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (fallback on error)", http.StatusOK, w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("Content-Type = %q, want 'image/png' (fallback on fetch error)", ct)
	}
}

func TestHandleFaviconFromTLSTestServer(t *testing.T) {
	faviconContent := []byte("\x89PNG\r\n\x1a\n" + strings.Repeat("x", 50))
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		w.WriteHeader(http.StatusOK)
		w.Write(faviconContent)
	}))
	defer ts.Close()

	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/favicon?url="+ts.URL+"/page", nil)
	w := httptest.NewRecorder()

	handler.handleFavicon(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "image/png" && ct != "image/x-icon" {
		t.Errorf("Content-Type = %q, want image/png (fallback) or image/x-icon (direct)", ct)
	}
}

// TestServeFaviconFallback directly tests the fallback PNG helper
func TestServeFaviconFallback(t *testing.T) {
	handler := newTestHandler()
	w := httptest.NewRecorder()

	handler.serveFaviconFallback(w)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", http.StatusOK, w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "image/png" {
		t.Errorf("Content-Type = %q, want 'image/png'", ct)
	}

	if w.Body.Len() == 0 {
		t.Error("expected non-empty body for fallback PNG")
	}

	cc := w.Header().Get("Cache-Control")
	if cc != "public, max-age=3600" {
		t.Errorf("Cache-Control = %q, want 'public, max-age=3600'", cc)
	}
}

// ============================================================================
// Tests for handleDirectAnswer
// ============================================================================

func TestHandleDirectAnswerMethodNotAllowed(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/direct/dns/example.com", nil)
	w := httptest.NewRecorder()

	handler.handleDirectAnswer(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleDirectAnswerMissingType(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/direct/", nil)
	w := httptest.NewRecorder()

	handler.handleDirectAnswer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleDirectAnswerMissingTerm(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/direct/dns/", nil)
	w := httptest.NewRecorder()

	handler.handleDirectAnswer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleDirectAnswerNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/direct/dns/example.com", nil)
	w := httptest.NewRecorder()

	handler.handleDirectAnswer(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (graceful degradation)", http.StatusOK, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Error("OK = false, want true")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if data["found"] != false {
		t.Errorf("found = %v, want false (no direct manager)", data["found"])
	}
	if data["type"] != "dns" {
		t.Errorf("type = %v, want 'dns'", data["type"])
	}
	if data["term"] != "example.com" {
		t.Errorf("term = %v, want 'example.com'", data["term"])
	}
}

func TestHandleDirectAnswerTableDriven(t *testing.T) {
	handler := newTestHandler()

	tests := []struct {
		name           string
		method         string
		path           string
		wantStatus     int
		wantOK         bool
		wantFoundFalse bool
	}{
		{"GET dns valid", http.MethodGet, "/api/v1/direct/dns/example.com", http.StatusOK, true, true},
		{"GET whois valid", http.MethodGet, "/api/v1/direct/whois/example.com", http.StatusOK, true, true},
		{"GET http code", http.MethodGet, "/api/v1/direct/http/404", http.StatusOK, true, true},
		{"GET regex", http.MethodGet, "/api/v1/direct/regex/%5Ba-z%5D%2B", http.StatusOK, true, true},
		{"POST not allowed", http.MethodPost, "/api/v1/direct/dns/example.com", http.StatusMethodNotAllowed, false, false},
		{"GET no type or term", http.MethodGet, "/api/v1/direct/", http.StatusBadRequest, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			handler.handleDirectAnswer(w, req)

			if w.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// ============================================================================
// Tests for http_errors.go: localizedHTTPError
// ============================================================================

func TestLocalizedHTTPError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	localizedHTTPError(w, req, http.StatusBadRequest, "errors.bad_request")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if w.Body.Len() == 0 {
		t.Error("expected non-empty body from localizedHTTPError")
	}
}

func TestLocalizedHTTPErrorWithArgs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	localizedHTTPError(w, req, http.StatusNotFound, "errors.not_found", "test-resource")

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestLocalizedHTTPErrorInternalServer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	localizedHTTPError(w, req, http.StatusInternalServerError, "errors.internal")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

// ============================================================================
// Tests for baseURLFromRequest and additional alerts.go coverage
// ============================================================================

func TestBaseURLFromRequestWithConfigBaseURL(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			BaseURL: "https://search.example.com/",
		},
	}
	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	result := baseURLFromRequest(handler, req)

	if result != "https://search.example.com" {
		t.Errorf("baseURLFromRequest = %q, want 'https://search.example.com'", result)
	}
}

func TestBaseURLFromRequestHTTP(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "localhost:8080"
	result := baseURLFromRequest(handler, req)

	if result != "http://localhost:8080" {
		t.Errorf("baseURLFromRequest = %q, want 'http://localhost:8080'", result)
	}
}

func TestBaseURLFromRequestHTTPS(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Loopback is always a trusted proxy — X-Forwarded-Proto is honored.
	req.RemoteAddr = "127.0.0.1:1234"
	req.Host = "secure.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	result := baseURLFromRequest(handler, req)

	if result != "https://secure.example.com" {
		t.Errorf("baseURLFromRequest = %q, want 'https://secure.example.com'", result)
	}
}

func TestClientIPForAPIWithXFF(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// Loopback is always a trusted proxy — XFF header is honored.
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")

	ip := clientIPForAPI(req)
	if ip != "10.0.0.1" {
		t.Errorf("clientIPForAPI = %q, want '10.0.0.1'", ip)
	}
}

func TestClientIPForAPIWithRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.50:4321"

	ip := clientIPForAPI(req)
	if ip != "192.168.1.50" {
		t.Errorf("clientIPForAPI = %q, want '192.168.1.50'", ip)
	}
}

func TestClientIPForAPIBareAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.99"

	ip := clientIPForAPI(req)
	if ip != "192.168.1.99" {
		t.Errorf("clientIPForAPI = %q, want '192.168.1.99'", ip)
	}
}

// ============================================================================
// Tests for decodeAlertPauseState
// ============================================================================

func TestDecodeAlertPauseStateTrueExplicit(t *testing.T) {
	body := strings.NewReader(`{"paused":true}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")

	paused, err := decodeAlertPauseState(req)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if !paused {
		t.Error("paused = false, want true")
	}
}

func TestDecodeAlertPauseStateFalseExplicit(t *testing.T) {
	body := strings.NewReader(`{"paused":false}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")

	paused, err := decodeAlertPauseState(req)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if paused {
		t.Error("paused = true, want false")
	}
}

func TestDecodeAlertPauseStateEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", nil)

	paused, err := decodeAlertPauseState(req)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if !paused {
		t.Error("paused = false, want true (default)")
	}
}

func TestDecodeAlertPauseStateQueryParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/?paused=false", nil)

	paused, err := decodeAlertPauseState(req)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if paused {
		t.Error("paused = true, want false (from query param)")
	}
}

func TestDecodeAlertPauseStateQueryParamTrue(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/?paused=true", nil)

	paused, err := decodeAlertPauseState(req)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if !paused {
		t.Error("paused = false, want true (from query param)")
	}
}

func TestDecodeAlertPauseStateEmptyBodyNoPauseField(t *testing.T) {
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")

	paused, err := decodeAlertPauseState(req)
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
	if !paused {
		t.Error("paused = false, want true (default when field absent)")
	}
}

// ============================================================================
// Tests for handleAlerts (alerts.go POST handler)
// ============================================================================

func TestHandleAlertsNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleAlerts(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (no alert manager)", http.StatusServiceUnavailable, w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.OK {
		t.Error("OK = true, want false (no alert manager)")
	}
	if resp.Error != "NOT_AVAILABLE" {
		t.Errorf("error code = %q, want 'NOT_AVAILABLE'", resp.Error)
	}
}

func TestHandleAlertsMethodNotAllowed(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	// DELETE is not a supported method on /api/v1/alerts (only GET and POST are)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/alerts", nil)
	w := httptest.NewRecorder()

	handler.handleAlerts(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleAlertsGetRequiresOperator(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	// GET /api/v1/alerts is operator-only; no token → 401
	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts", nil)
	w := httptest.NewRecorder()

	handler.handleAlerts(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", http.StatusUnauthorized, w.Code)
	}
}

func TestHandleAlertsInvalidJSON(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleAlerts(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleAlertsInvalidCreateDoesNotIssueOwner(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(`{"category":"general","email":"user@example.com","deliver_rss":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.handleAlerts(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	var owners int
	if err := db.QueryRow(`SELECT COUNT(*) FROM owners`).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if owners != 0 {
		t.Fatalf("owners = %d after invalid create, want 0", owners)
	}
}

func TestHandleAlertsValidCreate(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	body := `{
		"query": "privacy news",
		"category": "general",
		"language": "en",
		"frequency": "daily",
		"email": "user@example.com",
		"deliver_rss": true
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()

	handler.handleAlerts(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.OK {
		t.Errorf("OK = false, want true; message = %q", resp.Message)
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatal("data is not a map")
	}
	if ownerToken, ok := data["owner_token"].(string); !ok || ownerToken == "" {
		t.Errorf("owner_token = %#v, want token for newly issued owner", data["owner_token"])
	}
	if data["manage_token"] == nil || data["manage_token"] == "" {
		t.Error("expected manage_token in response")
	}
	if data["rss_token"] == nil || data["rss_token"] == "" {
		t.Error("expected rss_token in response")
	}
	if data["manage_url"] == nil {
		t.Error("expected manage_url in response")
	}
	if data["rss_url"] == nil {
		t.Error("expected rss_url in response")
	}
}

// ============================================================================
// Tests for handleAlertByToken (alert GET/PATCH/DELETE/pause/rss)
// ============================================================================

func TestHandleAlertByTokenNoManager(t *testing.T) {
	handler := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/sometoken", nil)
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHandleAlertByTokenGetNotFound(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/nonexistenttoken", nil)
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleAlertByTokenDeleteNotFound(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/alerts/nonexistenttoken", nil)
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", http.StatusNotFound, w.Code)
	}
}

func TestHandleAlertByTokenMethodNotAllowed(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/v1/alerts/sometoken", nil)
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleAlertByTokenGetSuccess(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	body := `{
		"query": "golang news",
		"category": "general",
		"language": "en",
		"frequency": "daily",
		"email": "godev@example.com",
		"deliver_rss": true
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Host = "localhost:8080"
	createW := httptest.NewRecorder()
	handler.handleAlerts(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("create: status = %d; body = %s", createW.Code, createW.Body.String())
	}

	var createResp APIResponse
	if err := json.NewDecoder(createW.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	manageToken := createResp.Data.(map[string]interface{})["manage_token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/"+manageToken, nil)
	req.Host = "localhost:8080"
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET alert: status = %d; body = %s", w.Code, w.Body.String())
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if !resp.OK {
		t.Errorf("OK = false, want true")
	}

	data := resp.Data.(map[string]interface{})
	if data["manage_token"] == nil {
		t.Error("expected manage_token in GET response")
	}
}

func TestHandleAlertByTokenDeleteSuccess(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	body := `{
		"query": "delete me",
		"category": "general",
		"language": "en",
		"frequency": "weekly",
		"email": "delete@example.com",
		"deliver_rss": true
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Host = "localhost:8080"
	createW := httptest.NewRecorder()
	handler.handleAlerts(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("create: status = %d", createW.Code)
	}

	var createResp APIResponse
	json.NewDecoder(createW.Body).Decode(&createResp)
	manageToken := createResp.Data.(map[string]interface{})["manage_token"].(string)

	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/alerts/"+manageToken, nil)
	delW := httptest.NewRecorder()

	handler.handleAlertByToken(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("DELETE: status = %d; body = %s", delW.Code, delW.Body.String())
	}

	var delResp APIResponse
	if err := json.NewDecoder(delW.Body).Decode(&delResp); err != nil {
		t.Fatalf("decode DELETE: %v", err)
	}
	if !delResp.OK {
		t.Error("OK = false on DELETE, want true")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/"+manageToken, nil)
	getW := httptest.NewRecorder()
	handler.handleAlertByToken(getW, getReq)

	if getW.Code != http.StatusNotFound {
		t.Fatalf("GET after DELETE: status = %d, want %d", getW.Code, http.StatusNotFound)
	}
}

func TestHandleAlertByTokenPatchSuccess(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	body := `{
		"query": "original query",
		"category": "general",
		"language": "en",
		"frequency": "daily",
		"email": "patch@example.com",
		"deliver_rss": true
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", strings.NewReader(body))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Host = "localhost:8080"
	createW := httptest.NewRecorder()
	handler.handleAlerts(createW, createReq)

	if createW.Code != http.StatusCreated {
		t.Fatalf("create: status = %d", createW.Code)
	}

	var createResp APIResponse
	json.NewDecoder(createW.Body).Decode(&createResp)
	manageToken := createResp.Data.(map[string]interface{})["manage_token"].(string)

	patchBody := `{
		"query": "updated query",
		"category": "news",
		"language": "en",
		"frequency": "weekly",
		"deliver_rss": true
	}`
	patchReq := httptest.NewRequest(http.MethodPatch, "/api/v1/alerts/"+manageToken, strings.NewReader(patchBody))
	patchReq.Header.Set("Content-Type", "application/json")
	patchReq.Host = "localhost:8080"
	patchW := httptest.NewRecorder()

	handler.handleAlertByToken(patchW, patchReq)

	if patchW.Code != http.StatusOK {
		t.Fatalf("PATCH: status = %d; body = %s", patchW.Code, patchW.Body.String())
	}

	var patchResp APIResponse
	if err := json.NewDecoder(patchW.Body).Decode(&patchResp); err != nil {
		t.Fatalf("decode PATCH: %v", err)
	}
	if !patchResp.OK {
		t.Errorf("OK = false on PATCH, want true; message = %q", patchResp.Message)
	}
}

func TestHandleAlertByTokenPatchInvalidJSON(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/alerts/anytoken", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAlertByTokenPauseNoBody(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/nonexistent/pause", nil)
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	// Missing body defaults to paused=true, so SetPaused runs against an
	// unknown token and correctly reports 404 (alert.ErrNotFound), matching
	// the sibling /rss unknown-token test below.
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (unknown token)", w.Code, http.StatusNotFound)
	}
}

func TestHandleAlertByTokenRSSNotFound(t *testing.T) {
	handler, _, db := newAlertAPIHandler(t)
	defer db.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/alerts/badtoken/rss", nil)
	w := httptest.NewRecorder()

	handler.handleAlertByToken(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ============================================================================
// Tests for DirectAnswerResponse serialization
// ============================================================================

func TestDirectAnswerResponseSerialization(t *testing.T) {
	resp := DirectAnswerResponse{
		Type:        "dns",
		Term:        "example.com",
		Title:       "DNS Records for example.com",
		Description: "DNS lookup results",
		Content:     "A 93.184.216.34",
		Data:        map[string]interface{}{"a": []string{"93.184.216.34"}},
		Source:      "dns_lookup",
		SourceURL:   "https://example.com",
		CacheTTL:    3600,
		Found:       true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var unmarshaled DirectAnswerResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if unmarshaled.Type != resp.Type {
		t.Errorf("Type = %q, want %q", unmarshaled.Type, resp.Type)
	}
	if unmarshaled.Term != resp.Term {
		t.Errorf("Term = %q, want %q", unmarshaled.Term, resp.Term)
	}
	if unmarshaled.Found != resp.Found {
		t.Errorf("Found = %v, want %v", unmarshaled.Found, resp.Found)
	}
	if unmarshaled.CacheTTL != resp.CacheTTL {
		t.Errorf("CacheTTL = %d, want %d", unmarshaled.CacheTTL, resp.CacheTTL)
	}
}

// ============================================================================
// Tests for ServerPageResponse serialization
// ============================================================================

func TestServerPageResponseSerialization(t *testing.T) {
	resp := ServerPageResponse{
		Title:       "About Us",
		Description: "About page description",
		Content:     "Long about content here",
		Sections: []PageSection{
			{ID: "intro", Title: "Introduction", Content: "Welcome to our service"},
		},
		Metadata: map[string]string{"version": "1.0.0"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var unmarshaled ServerPageResponse
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if unmarshaled.Title != resp.Title {
		t.Errorf("Title = %q, want %q", unmarshaled.Title, resp.Title)
	}
	if len(unmarshaled.Sections) != 1 {
		t.Errorf("Sections len = %d, want 1", len(unmarshaled.Sections))
	}
	if unmarshaled.Sections[0].ID != "intro" {
		t.Errorf("Section[0].ID = %q, want 'intro'", unmarshaled.Sections[0].ID)
	}
}

// ============================================================================
// Tests for SetTorService and SetDirectManager
// ============================================================================

func TestSetTorService(t *testing.T) {
	handler := newTestHandler()

	if handler.torService != nil {
		t.Error("torService should be nil initially")
	}

	handler.SetTorService(nil)

	if handler.torService != nil {
		t.Error("torService should still be nil after setting to nil")
	}
}

func TestSetDirectManager(t *testing.T) {
	handler := newTestHandler()

	if handler.directManager != nil {
		t.Error("directManager should be nil initially")
	}

	handler.SetDirectManager(nil)

	if handler.directManager != nil {
		t.Error("directManager should still be nil after setting to nil")
	}
}

func TestSetAlertManager(t *testing.T) {
	handler := newTestHandler()

	if handler.alertManager != nil {
		t.Error("alertManager should be nil initially")
	}

	handler.SetAlertManager(nil)

	if handler.alertManager != nil {
		t.Error("alertManager should still be nil after setting to nil")
	}
}

// ============================================================================
// Test APIBasePath helper
// ============================================================================

func TestAPIBasePath(t *testing.T) {
	path := APIBasePath()
	if path != "/api/v1" {
		t.Errorf("APIBasePath() = %q, want '/api/v1'", path)
	}
}

// ============================================================================
// Tests for handleEngines with real engine registered
// ============================================================================

func TestHandleEnginesWithRegisteredEngine(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
		},
	}
	registry := engine.NewRegistry()

	eng := &emptyResultEngine{cfg: model.NewEngineConfig("testengine")}
	eng.cfg.DisplayName = "Test Engine"
	eng.cfg.Enabled = true
	eng.cfg.Priority = 100
	registry.Register(eng)

	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines", nil)
	w := httptest.NewRecorder()

	handler.handleEngines(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	engines, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatal("data is not a slice")
	}
	if len(engines) != 1 {
		t.Fatalf("len(engines) = %d, want 1", len(engines))
	}

	eng0 := engines[0].(map[string]interface{})
	if eng0["id"] != "testengine" {
		t.Errorf("id = %v, want 'testengine'", eng0["id"])
	}
	if eng0["enabled"] != true {
		t.Errorf("enabled = %v, want true", eng0["enabled"])
	}
}

func TestHandleEngineByIDFound(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "Test Search",
		},
	}
	registry := engine.NewRegistry()

	eng := &emptyResultEngine{cfg: model.NewEngineConfig("myengine")}
	eng.cfg.DisplayName = "My Engine"
	eng.cfg.Enabled = true
	registry.Register(eng)

	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/engines/myengine", nil)
	w := httptest.NewRecorder()

	handler.handleEngineByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data := resp.Data.(map[string]interface{})
	if data["id"] != "myengine" {
		t.Errorf("id = %v, want 'myengine'", data["id"])
	}
}

// ============================================================================
// Tests for handleInfo with nil registry (edge case)
// ============================================================================

func TestHandleInfoNilRegistry(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Title: "NilRegistryTest",
		},
	}

	registry := engine.NewRegistry()
	aggregator := search.NewAggregatorSimple(nil, 30*time.Second)
	handler := NewHandler(cfg, registry, aggregator)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/info", nil)
	w := httptest.NewRecorder()

	handler.handleInfo(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ============================================================================
// Test for errorResponse with 5xx detail logging path
// ============================================================================

func TestErrorResponseWith5xxDetail(t *testing.T) {
	handler := newTestHandler()

	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "req-abc123")

	handler.errorResponse(w, http.StatusInternalServerError, "Internal error", "some internal detail", httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.OK {
		t.Error("OK = true, want false for error response")
	}
	if resp.Error == "" {
		t.Error("expected error code")
	}
}

func TestErrorResponseWith4xx(t *testing.T) {
	handler := newTestHandler()

	w := httptest.NewRecorder()

	handler.errorResponse(w, http.StatusBadRequest, "Bad request", "detail not logged for 4xx", httptest.NewRequest(http.MethodGet, "/", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.OK {
		t.Error("OK = true, want false")
	}
}

func TestErrorResponseUsesRequestLanguage(t *testing.T) {
	handler := newTestHandler()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=test", nil)
	request.Header.Set("Accept-Language", "es")
	response := httptest.NewRecorder()

	handler.errorResponse(response, http.StatusBadRequest, "Invalid request parameters", "", request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	var payload APIResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Error != "BAD_REQUEST" {
		t.Errorf("error code = %q, want BAD_REQUEST", payload.Error)
	}
	if payload.Message != "Solicitud no válida" {
		t.Errorf("message = %q, want Spanish bad-request translation", payload.Message)
	}
}

func TestWriteErrorTranslatesByErrorCode(t *testing.T) {
	handler := newTestHandler()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/alerts", nil)
	request.Header.Set("Accept-Language", "es")
	response := httptest.NewRecorder()

	handler.writeError(response, request, "VALIDATION_FAILED", "English-only detail", http.StatusBadRequest)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	var payload APIResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Error != "VALIDATION_FAILED" {
		t.Errorf("error code = %q, want VALIDATION_FAILED", payload.Error)
	}
	if payload.Message != "La información enviada no es válida." {
		t.Errorf("message = %q, want Spanish validation translation", payload.Message)
	}
}

func TestSchedulerOperatorEndpoints(t *testing.T) {
	handler := newTestHandler()
	handler.config.Server.Token = "scheduler-test-token"
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	sched := scheduler.NewScheduler(db, "test")
	if err := sched.Register(&scheduler.Task{
		ID: "test_task", Name: "Test task", Description: "test", Schedule: "@hourly",
		Skippable: true, Run: func(context.Context) error { return nil },
	}); err != nil {
		t.Fatal(err)
	}
	handler.SetScheduler(sched)
	router := chi.NewRouter()
	handler.RegisterRoutes(router)

	request := func(method, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("Authorization", "Bearer scheduler-test-token")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}

	for _, endpoint := range []struct{ method, path string }{
		{http.MethodGet, APIPrefix + "/server/scheduler/tasks"},
		{http.MethodGet, APIPrefix + "/server/scheduler/tasks/test_task"},
		{http.MethodGet, APIPrefix + "/server/scheduler/tasks/test_task/history"},
	} {
		response := request(endpoint.method, endpoint.path)
		if response.Code != http.StatusOK {
			t.Errorf("%s %s status = %d, body = %s", endpoint.method, endpoint.path, response.Code, response.Body.String())
		}
	}

	if response := request(http.MethodPost, APIPrefix+"/server/scheduler/tasks/test_task/run"); response.Code != http.StatusOK {
		t.Errorf("scheduler run status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, action := range []string{"enable", "disable"} {
		response := request(http.MethodPost, APIPrefix+"/server/scheduler/tasks/test_task/"+action)
		if response.Code != http.StatusNotFound {
			t.Errorf("scheduler %s mutation status = %d, want 404", action, response.Code)
		}
	}
}
