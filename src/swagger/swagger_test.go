package swagger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

// Tests for OpenAPI types

func TestOpenAPISpecStruct(t *testing.T) {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:       "Test API",
			Description: "Test description",
			Version:     "1.0.0",
		},
		Servers: []Server{{URL: "http://localhost:8080"}},
		Paths:   map[string]PathItem{},
	}

	if spec.OpenAPI != "3.0.0" {
		t.Errorf("OpenAPI = %q", spec.OpenAPI)
	}
	if spec.Info.Title != "Test API" {
		t.Errorf("Info.Title = %q", spec.Info.Title)
	}
}

func TestInfoStruct(t *testing.T) {
	info := Info{
		Title:       "Search API",
		Description: "A search API",
		Version:     "2.0.0",
		Contact: &Contact{
			Name:  "Developer",
			URL:   "https://example.com",
			Email: "dev@example.com",
		},
	}

	if info.Title != "Search API" {
		t.Errorf("Title = %q", info.Title)
	}
	if info.Contact == nil {
		t.Fatal("Contact should not be nil")
	}
	if info.Contact.Email != "dev@example.com" {
		t.Errorf("Contact.Email = %q", info.Contact.Email)
	}
}

func TestServerStruct(t *testing.T) {
	server := Server{
		URL:         "https://api.example.com",
		Description: "Production server",
	}

	if server.URL != "https://api.example.com" {
		t.Errorf("URL = %q", server.URL)
	}
}

func TestPathItemStruct(t *testing.T) {
	path := PathItem{
		Get: &Operation{
			Summary: "Get resource",
			Tags:    []string{"Resources"},
		},
		Post: &Operation{
			Summary: "Create resource",
		},
	}

	if path.Get == nil {
		t.Error("Get should not be nil")
	}
	if path.Post == nil {
		t.Error("Post should not be nil")
	}
	if path.Get.Summary != "Get resource" {
		t.Errorf("Get.Summary = %q", path.Get.Summary)
	}
}

func TestOperationStruct(t *testing.T) {
	op := Operation{
		Summary:     "Test operation",
		Description: "Test description",
		Tags:        []string{"test", "example"},
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true},
		},
		Responses: map[string]Response{
			"200": {Description: "Success"},
		},
	}

	if op.Summary != "Test operation" {
		t.Errorf("Summary = %q", op.Summary)
	}
	if len(op.Tags) != 2 {
		t.Errorf("Tags count = %d", len(op.Tags))
	}
	if len(op.Parameters) != 1 {
		t.Errorf("Parameters count = %d", len(op.Parameters))
	}
}

func TestParameterStruct(t *testing.T) {
	param := Parameter{
		Name:        "page",
		In:          "query",
		Description: "Page number",
		Required:    false,
		Schema:      &Schema{Type: "integer"},
	}

	if param.Name != "page" {
		t.Errorf("Name = %q", param.Name)
	}
	if param.In != "query" {
		t.Errorf("In = %q", param.In)
	}
	if param.Schema == nil {
		t.Error("Schema should not be nil")
	}
}

func TestRequestBodyStruct(t *testing.T) {
	body := RequestBody{
		Description: "Request body",
		Required:    true,
		Content: map[string]MediaType{
			"application/json": {Schema: &Schema{Type: "object"}},
		},
	}

	if !body.Required {
		t.Error("Required should be true")
	}
	if len(body.Content) != 1 {
		t.Errorf("Content count = %d", len(body.Content))
	}
}

func TestResponseStruct(t *testing.T) {
	resp := Response{
		Description: "Success response",
		Content: map[string]MediaType{
			"application/json": {Schema: &Schema{Type: "object"}},
		},
	}

	if resp.Description != "Success response" {
		t.Errorf("Description = %q", resp.Description)
	}
}

func TestSchemaStruct(t *testing.T) {
	schema := Schema{
		Type: "object",
		Properties: map[string]Schema{
			"id":   {Type: "integer"},
			"name": {Type: "string"},
		},
		Example: map[string]interface{}{"id": 1, "name": "test"},
	}

	if schema.Type != "object" {
		t.Errorf("Type = %q", schema.Type)
	}
	if len(schema.Properties) != 2 {
		t.Errorf("Properties count = %d", len(schema.Properties))
	}
}

func TestComponentsStruct(t *testing.T) {
	components := Components{
		Schemas: map[string]Schema{
			"User": {Type: "object"},
		},
	}

	if len(components.Schemas) != 1 {
		t.Errorf("Schemas count = %d", len(components.Schemas))
	}
}

// Tests for GenerateSpec

func TestGenerateSpec(t *testing.T) {
	cfg := &config.Config{}
	baseURL := "https://api.example.com"

	spec := GenerateSpec(cfg, baseURL)

	if spec == nil {
		t.Fatal("GenerateSpec() returned nil")
	}
	if spec.OpenAPI != "3.0.0" {
		t.Errorf("OpenAPI = %q, want 3.0.0", spec.OpenAPI)
	}
	if spec.Info.Title != "Search API" {
		t.Errorf("Info.Title = %q", spec.Info.Title)
	}
	if len(spec.Servers) != 1 {
		t.Errorf("Servers count = %d", len(spec.Servers))
	}
	if spec.Servers[0].URL != baseURL {
		t.Errorf("Server URL = %q, want %q", spec.Servers[0].URL, baseURL)
	}
}

func TestGenerateSpecHasRequiredPaths(t *testing.T) {
	cfg := &config.Config{}
	spec := GenerateSpec(cfg, "http://localhost:8080")

	requiredPaths := []string{"/healthz", "/api/v1/healthz", "/api/v1/search", "/api/v1/autocomplete"}
	for _, path := range requiredPaths {
		if _, exists := spec.Paths[path]; !exists {
			t.Errorf("Missing required path: %s", path)
		}
	}
}

func TestGenerateSpecSearchEndpoint(t *testing.T) {
	cfg := &config.Config{}
	spec := GenerateSpec(cfg, "http://localhost:8080")

	searchPath, exists := spec.Paths["/api/v1/search"]
	if !exists {
		t.Fatal("Search endpoint not found")
	}

	if searchPath.Get == nil {
		t.Fatal("Search endpoint should have GET operation")
	}

	// Should have required parameters
	var hasQ bool
	for _, p := range searchPath.Get.Parameters {
		if p.Name == "q" && p.Required {
			hasQ = true
		}
	}
	if !hasQ {
		t.Error("Search endpoint should have required 'q' parameter")
	}
}

// Tests for generatePaths

func TestGeneratePaths(t *testing.T) {
	paths := generatePaths()

	if len(paths) == 0 {
		t.Error("generatePaths() returned empty map")
	}

	// Health endpoint should exist
	if _, exists := paths["/healthz"]; !exists {
		t.Error("Health endpoint should exist")
	}

	// Search endpoint should exist with parameters
	search, exists := paths["/api/v1/search"]
	if !exists {
		t.Error("Search endpoint should exist")
	}
	if search.Get == nil {
		t.Error("Search should have GET operation")
	}
}

// Tests for generateComponents

func TestGenerateComponents(t *testing.T) {
	components := generateComponents()

	if len(components.Schemas) == 0 {
		t.Error("generateComponents() returned no schemas")
	}

	// Should have SearchResult schema
	if _, exists := components.Schemas["SearchResult"]; !exists {
		t.Error("SearchResult schema should exist")
	}

	// Should have HealthResponse schema
	if _, exists := components.Schemas["HealthResponse"]; !exists {
		t.Error("HealthResponse schema should exist")
	}
}

// Tests for Handler

func TestHandlerServesSwaggerUI(t *testing.T) {
	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Error("Response should contain swagger-ui")
	}
	if !strings.Contains(body, "SwaggerUIBundle") {
		t.Error("Response should contain SwaggerUIBundle")
	}
}

func TestHandlerServesJSON(t *testing.T) {
	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var spec OpenAPISpec
	if err := json.NewDecoder(rec.Body).Decode(&spec); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	if spec.OpenAPI != "3.0.0" {
		t.Errorf("OpenAPI = %q", spec.OpenAPI)
	}
}

func TestHandlerServesJSONWithAcceptHeader(t *testing.T) {
	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()

	handler(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

// Tests for buildBaseURL

func TestBuildBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		headers   map[string]string
		wantProto string
		wantHost  string
	}{
		{
			name:      "default HTTP",
			host:      "localhost:8080",
			headers:   nil,
			wantProto: "http",
			wantHost:  "localhost:8080",
		},
		{
			name:      "forwarded HTTPS",
			host:      "localhost:8080",
			headers:   map[string]string{"X-Forwarded-Proto": "https"},
			wantProto: "https",
			wantHost:  "localhost:8080",
		},
		{
			name:      "forwarded host",
			host:      "localhost:8080",
			headers:   map[string]string{"X-Forwarded-Host": "example.com"},
			wantProto: "http",
			wantHost:  "example.com",
		},
		{
			name: "both forwarded",
			host: "localhost:8080",
			headers: map[string]string{
				"X-Forwarded-Proto": "https",
				"X-Forwarded-Host":  "api.example.com",
			},
			wantProto: "https",
			wantHost:  "api.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			url := buildBaseURL(req)

			if !strings.HasPrefix(url, tt.wantProto+"://") {
				t.Errorf("buildBaseURL() = %q, want protocol %q", url, tt.wantProto)
			}
			if !strings.Contains(url, tt.wantHost) {
				t.Errorf("buildBaseURL() = %q, want host %q", url, tt.wantHost)
			}
		})
	}
}

// Tests for extractHost

func TestExtractHost(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"http://localhost:8080", "localhost"},
		{"https://example.com", "example.com"},
		{"https://api.example.com:443/path", "api.example.com"},
		{"http://127.0.0.1:9000/test", "127.0.0.1"},
		{"", ""},  // Empty string returns empty
	}

	for _, tt := range tests {
		result := extractHost(tt.url)
		if result != tt.expected {
			t.Errorf("extractHost(%q) = %q, want %q", tt.url, result, tt.expected)
		}
	}
}

// Tests for getTheme

func TestGetTheme(t *testing.T) {
	// Test default (no cookie)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	theme := getTheme(req)
	if theme != "dark" {
		t.Errorf("Default theme = %q, want dark", theme)
	}

	// Test with light theme cookie
	reqLight := httptest.NewRequest(http.MethodGet, "/", nil)
	reqLight.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	if getTheme(reqLight) != "light" {
		t.Error("Theme should be light")
	}

	// Test with dark theme cookie
	reqDark := httptest.NewRequest(http.MethodGet, "/", nil)
	reqDark.AddCookie(&http.Cookie{Name: "theme", Value: "dark"})
	if getTheme(reqDark) != "dark" {
		t.Error("Theme should be dark")
	}

	// Test with auto theme cookie
	reqAuto := httptest.NewRequest(http.MethodGet, "/", nil)
	reqAuto.AddCookie(&http.Cookie{Name: "theme", Value: "auto"})
	if getTheme(reqAuto) != "auto" {
		t.Error("Theme should be auto")
	}

	// Test with invalid theme cookie (should default to dark)
	reqInvalid := httptest.NewRequest(http.MethodGet, "/", nil)
	reqInvalid.AddCookie(&http.Cookie{Name: "theme", Value: "invalid"})
	if getTheme(reqInvalid) != "dark" {
		t.Error("Invalid theme should default to dark")
	}
}

// Tests for getSwaggerThemeCSS

func TestGetSwaggerThemeCSS(t *testing.T) {
	// Test dark theme
	darkCSS := getSwaggerThemeCSS("dark")
	if darkCSS == "" {
		t.Error("getSwaggerThemeCSS('dark') returned empty")
	}

	// Test light theme
	lightCSS := getSwaggerThemeCSS("light")
	if lightCSS == "" {
		t.Error("getSwaggerThemeCSS('light') returned empty")
	}

	// Dark and light should be different
	if darkCSS == lightCSS {
		t.Error("Dark and light CSS should be different")
	}
}
