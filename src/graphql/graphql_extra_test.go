package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/graphql-go/graphql/language/ast"
)

func TestParseJSONLiteralPreservesNestedInput(t *testing.T) {
	literal := &ast.ObjectValue{Fields: []*ast.ObjectField{
		{Name: &ast.Name{Value: "enabled"}, Value: &ast.BooleanValue{Value: true}},
		{Name: &ast.Name{Value: "filters"}, Value: &ast.ListValue{Values: []ast.Value{
			&ast.StringValue{Value: "web"},
			&ast.IntValue{Value: "3"},
		}}},
	}}
	parsed, ok := parseJSONLiteral(literal).(map[string]interface{})
	if !ok {
		t.Fatalf("parsed literal type = %T, want object", parseJSONLiteral(literal))
	}
	if parsed["enabled"] != true {
		t.Fatalf("enabled = %#v, want true", parsed["enabled"])
	}
	filters, ok := parsed["filters"].([]interface{})
	if !ok || len(filters) != 2 || filters[0] != "web" || filters[1] != int64(3) {
		t.Fatalf("filters = %#v, want [\"web\", 3]", parsed["filters"])
	}
}

// Tests for resolveDirectAnswer

func TestResolveDirectAnswer(t *testing.T) {
	tests := []struct {
		name       string
		answerType string
		term       string
		wantType   string
		wantTerm   string
		wantFound  bool
	}{
		{"unknown answer type", "notregistered", "example.test", "notregistered", "example.test", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := testResolverParams(map[string]interface{}{
				"type": tt.answerType,
				"term": tt.term,
			})

			result, err := resolveDirectAnswer(params)
			if err != nil {
				t.Fatalf("resolveDirectAnswer() error = %v", err)
			}

			m, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("resolveDirectAnswer() result is not map[string]interface{}")
			}

			okVal, _ := m["ok"].(bool)
			if okVal != tt.wantFound {
				t.Errorf("resolveDirectAnswer() ok = %v, want %v", m["ok"], tt.wantFound)
			}

			data, ok := m["data"].(map[string]interface{})
			if !ok {
				t.Fatalf("resolveDirectAnswer() data is not map[string]interface{}")
			}

			if data["type"] != tt.wantType {
				t.Errorf("data[type] = %q, want %q", data["type"], tt.wantType)
			}
			if data["term"] != tt.wantTerm {
				t.Errorf("data[term] = %q, want %q", data["term"], tt.wantTerm)
			}
			if data["found"] != tt.wantFound {
				t.Errorf("data[found] = %v, want %v", data["found"], tt.wantFound)
			}
		})
	}
}

// TestResolveDirectAnswerRequiredFields verifies all expected fields are present.
func TestResolveDirectAnswerRequiredFields(t *testing.T) {
	params := testResolverParams(map[string]interface{}{"type": "notregistered", "term": "example.test"})
	result, err := resolveDirectAnswer(params)
	if err != nil {
		t.Fatalf("resolveDirectAnswer() error = %v", err)
	}

	m := result.(map[string]interface{})
	data := m["data"].(map[string]interface{})

	requiredFields := []string{"type", "term", "title", "description", "content", "source", "sourceUrl", "cacheTtlSeconds", "found"}
	for _, field := range requiredFields {
		if _, ok := data[field]; !ok {
			t.Errorf("resolveDirectAnswer() missing field %q in data", field)
		}
	}
}

// Tests for resolveInstant

func TestResolveInstant(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantQuery string
		wantFound bool
	}{
		{"math expression", "2+2", "2+2", true},
		{"unmatched natural language", "this phrase has no instant answer", "this phrase has no instant answer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := testResolverParams(map[string]interface{}{"q": tt.query})

			result, err := resolveInstant(params)
			if err != nil {
				t.Fatalf("resolveInstant() error = %v", err)
			}

			m, ok := result.(map[string]interface{})
			if !ok {
				t.Fatalf("resolveInstant() result is not map[string]interface{}")
			}

			okVal, _ := m["ok"].(bool)
			if !okVal {
				t.Errorf("resolveInstant() ok = %v, want true", m["ok"])
			}

			data, ok := m["data"].(map[string]interface{})
			if !ok {
				t.Fatalf("resolveInstant() data is not map[string]interface{}")
			}

			if data["query"] != tt.wantQuery {
				t.Errorf("data[query] = %q, want %q", data["query"], tt.wantQuery)
			}
			if data["found"] != tt.wantFound {
				t.Errorf("data[found] = %v, want %v", data["found"], tt.wantFound)
			}
		})
	}
}

// TestResolveInstantRequiredFields verifies all expected fields are present.
func TestResolveInstantRequiredFields(t *testing.T) {
	params := testResolverParams(map[string]interface{}{"q": "2+2"})
	result, err := resolveInstant(params)
	if err != nil {
		t.Fatalf("resolveInstant() error = %v", err)
	}

	m := result.(map[string]interface{})
	data := m["data"].(map[string]interface{})

	requiredFields := []string{"query", "type", "title", "content", "source", "found"}
	for _, field := range requiredFields {
		if _, ok := data[field]; !ok {
			t.Errorf("resolveInstant() missing field %q in data", field)
		}
	}
}

// Tests for UIHandler

func TestUIHandlerGET(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := UIHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/server/docs/graphql", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("UIHandler GET status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, "GraphiQL") {
		t.Error("UIHandler GET response should contain GraphiQL")
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("UIHandler GET Content-Type = %q, want text/html", ct)
	}
}

func TestUIHandlerNonGETReturns405(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := UIHandler(cfg)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/server/docs/graphql", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("UIHandler %s status = %d, want %d", method, resp.StatusCode, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestUIHandlerGETRendersTheme(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := UIHandler(cfg)

	tests := []struct {
		name        string
		cookieValue string
		wantBg      string
	}{
		{"dark theme (default)", "", "#282a36"},
		{"light theme", "light", "#ffffff"},
		{"dark theme explicit", "dark", "#282a36"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/server/docs/graphql", nil)
			if tt.cookieValue != "" {
				req.AddCookie(&http.Cookie{Name: "theme", Value: tt.cookieValue})
			}
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("UIHandler status = %d, want 200", resp.StatusCode)
			}
			body := w.Body.String()
			if !strings.Contains(body, tt.wantBg) {
				t.Errorf("UIHandler body does not contain expected bg color %q for theme %q", tt.wantBg, tt.cookieValue)
			}
		})
	}
}

// Tests for QueryHandler

func TestQueryHandlerPOST(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := QueryHandler(cfg, testGraphQLDependencies())

	body := `{"query":"{ health { status } }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("QueryHandler POST status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("QueryHandler POST response is not valid JSON: %v", err)
	}
}

func TestQueryHandlerNonPOSTReturns405(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := QueryHandler(cfg, testGraphQLDependencies())

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/graphql", nil)
			w := httptest.NewRecorder()

			handler(w, req)

			resp := w.Result()
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("QueryHandler %s status = %d, want %d", method, resp.StatusCode, http.StatusMethodNotAllowed)
			}
		})
	}
}

func TestQueryHandlerInvalidJSONReturns400(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := QueryHandler(cfg, testGraphQLDependencies())

	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewBufferString("not-json{"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("QueryHandler invalid JSON status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
}

func TestQueryHandlerDirectAnswerQuery(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := QueryHandler(cfg, testGraphQLDependencies())

	body := `{"query":"{ directAnswer(type: \"notregistered\", term: \"example.test\") { ok } }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("QueryHandler directAnswer status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestQueryHandlerInstantQuery(t *testing.T) {
	if err := InitSchema(); err != nil {
		t.Fatalf("InitSchema() error = %v", err)
	}

	cfg := config.DefaultConfig()
	handler := QueryHandler(cfg, testGraphQLDependencies())

	body := `{"query":"{ instant(q: \"2+2\") { ok } }"}`
	req := httptest.NewRequest(http.MethodPost, "/api/graphql", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("QueryHandler instant status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}
