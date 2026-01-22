package swagger

import (
	"crypto/tls"
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

// Additional tests for 100% coverage

// TestGetSwaggerThemeCSSTableDriven tests all theme variations using table-driven approach
func TestGetSwaggerThemeCSSTableDriven(t *testing.T) {
	tests := []struct {
		name      string
		theme     string
		wantDark  bool // true means should return dark theme
	}{
		{
			name:     "light theme returns light CSS",
			theme:    "light",
			wantDark: false,
		},
		{
			name:     "dark theme returns dark CSS",
			theme:    "dark",
			wantDark: true,
		},
		{
			name:     "auto theme defaults to dark CSS",
			theme:    "auto",
			wantDark: true,
		},
		{
			name:     "empty theme defaults to dark CSS",
			theme:    "",
			wantDark: true,
		},
		{
			name:     "invalid theme defaults to dark CSS",
			theme:    "invalid",
			wantDark: true,
		},
		{
			name:     "unknown theme defaults to dark CSS",
			theme:    "sepia",
			wantDark: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			css := getSwaggerThemeCSS(tt.theme)
			if css == "" {
				t.Error("getSwaggerThemeCSS returned empty CSS")
			}

			containsDarkBg := strings.Contains(css, "#282a36")
			containsLightBg := strings.Contains(css, "#ffffff")

			if tt.wantDark {
				if !containsDarkBg {
					t.Errorf("Expected dark theme CSS for theme %q", tt.theme)
				}
			} else {
				if !containsLightBg {
					t.Errorf("Expected light theme CSS for theme %q", tt.theme)
				}
			}
		})
	}
}

// TestBuildBaseURLWithTLS tests buildBaseURL when TLS is enabled
func TestBuildBaseURLWithTLS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "secure.example.com"
	// Simulate TLS by setting TLS field to non-nil
	req.TLS = &tls.ConnectionState{}

	url := buildBaseURL(req)

	if !strings.HasPrefix(url, "https://") {
		t.Errorf("buildBaseURL() with TLS = %q, want https://", url)
	}
	if !strings.Contains(url, "secure.example.com") {
		t.Errorf("buildBaseURL() = %q, want host secure.example.com", url)
	}
}

// TestBuildBaseURLTLSOverriddenByHeader tests that X-Forwarded-Proto overrides TLS detection
func TestBuildBaseURLTLSOverriddenByHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "example.com"
	req.TLS = &tls.ConnectionState{}
	// Even with TLS, X-Forwarded-Proto should take precedence
	req.Header.Set("X-Forwarded-Proto", "http")

	url := buildBaseURL(req)

	// X-Forwarded-Proto overrides TLS detection
	if !strings.HasPrefix(url, "http://") {
		t.Errorf("buildBaseURL() = %q, X-Forwarded-Proto should override TLS", url)
	}
}

// TestExtractHostEdgeCases tests additional edge cases for extractHost
func TestExtractHostEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "URL with only protocol",
			url:      "http://",
			expected: "",
		},
		{
			name:     "URL with only https protocol",
			url:      "https://",
			expected: "",
		},
		{
			name:     "plain hostname",
			url:      "example.com",
			expected: "example.com",
		},
		{
			name:     "hostname with port",
			url:      "example.com:8080",
			expected: "example.com",
		},
		{
			name:     "IP address with port and path",
			url:      "192.168.1.1:3000/api/v1",
			expected: "192.168.1.1",
		},
		{
			name:     "subdomain with multiple levels",
			url:      "https://api.v2.example.com:443/path/to/resource",
			expected: "api.v2.example.com",
		},
		{
			name:     "URL with trailing slash",
			url:      "http://example.com/",
			expected: "example.com",
		},
		{
			name:     "URL with query string",
			url:      "http://example.com?query=value",
			expected: "example.com?query=value",
		},
		{
			name:     "localhost URL",
			url:      "http://localhost",
			expected: "localhost",
		},
		{
			name:     "IPv6 localhost (simplified)",
			url:      "::1",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHost(tt.url)
			if result != tt.expected {
				t.Errorf("extractHost(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

// TestServeSwaggerUIWithDifferentThemes tests serveSwaggerUI with various theme cookies
func TestServeSwaggerUIWithDifferentThemes(t *testing.T) {
	tests := []struct {
		name           string
		themeCookie    string
		expectDarkCSS  bool
	}{
		{
			name:          "no theme cookie defaults to dark",
			themeCookie:   "",
			expectDarkCSS: true,
		},
		{
			name:          "light theme cookie",
			themeCookie:   "light",
			expectDarkCSS: false,
		},
		{
			name:          "dark theme cookie",
			themeCookie:   "dark",
			expectDarkCSS: true,
		},
		{
			name:          "auto theme cookie defaults to dark",
			themeCookie:   "auto",
			expectDarkCSS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
			if tt.themeCookie != "" {
				req.AddCookie(&http.Cookie{Name: "theme", Value: tt.themeCookie})
			}
			rec := httptest.NewRecorder()

			serveSwaggerUI(rec, req, "http://localhost:8080")

			body := rec.Body.String()

			if tt.expectDarkCSS {
				if !strings.Contains(body, "#282a36") {
					t.Error("Expected dark theme CSS in response")
				}
			} else {
				if !strings.Contains(body, "#ffffff") {
					t.Error("Expected light theme CSS in response")
				}
			}

			// Verify HTML structure
			if !strings.Contains(body, "<!DOCTYPE html>") {
				t.Error("Response should be valid HTML")
			}
			if !strings.Contains(body, "swagger-ui-bundle.js") {
				t.Error("Response should include Swagger UI bundle")
			}
		})
	}
}

// TestServeSwaggerUIBaseURLInjection tests that baseURL is correctly injected
func TestServeSwaggerUIBaseURLInjection(t *testing.T) {
	testURLs := []string{
		"http://localhost:8080",
		"https://api.example.com",
		"http://192.168.1.1:3000",
	}

	for _, baseURL := range testURLs {
		t.Run(baseURL, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/swagger", nil)
			rec := httptest.NewRecorder()

			serveSwaggerUI(rec, req, baseURL)

			body := rec.Body.String()
			expectedURL := baseURL + `/openapi.json`
			if !strings.Contains(body, expectedURL) {
				t.Errorf("Response should contain spec URL %q", expectedURL)
			}
		})
	}
}

// TestGenerateSpecContactEmail tests that contact email is correctly generated
func TestGenerateSpecContactEmail(t *testing.T) {
	tests := []struct {
		name          string
		baseURL       string
		expectedEmail string
	}{
		{
			name:          "localhost URL",
			baseURL:       "http://localhost:8080",
			expectedEmail: "noreply@localhost",
		},
		{
			name:          "domain URL",
			baseURL:       "https://api.example.com",
			expectedEmail: "noreply@api.example.com",
		},
		{
			name:          "IP address URL",
			baseURL:       "http://192.168.1.1:3000",
			expectedEmail: "noreply@192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{}
			spec := GenerateSpec(cfg, tt.baseURL)

			if spec.Info.Contact == nil {
				t.Fatal("Contact should not be nil")
			}
			if spec.Info.Contact.Email != tt.expectedEmail {
				t.Errorf("Contact.Email = %q, want %q", spec.Info.Contact.Email, tt.expectedEmail)
			}
		})
	}
}

// TestOpenAPISpecJSONSerialization tests complete JSON serialization
func TestOpenAPISpecJSONSerialization(t *testing.T) {
	cfg := &config.Config{}
	spec := GenerateSpec(cfg, "http://localhost:8080")

	jsonBytes, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Failed to marshal spec: %v", err)
	}

	var decoded OpenAPISpec
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal spec: %v", err)
	}

	// Verify round-trip
	if decoded.OpenAPI != spec.OpenAPI {
		t.Errorf("OpenAPI mismatch after round-trip")
	}
	if decoded.Info.Title != spec.Info.Title {
		t.Errorf("Info.Title mismatch after round-trip")
	}
	if len(decoded.Paths) != len(spec.Paths) {
		t.Errorf("Paths count mismatch: got %d, want %d", len(decoded.Paths), len(spec.Paths))
	}
}

// TestPathItemAllOperations tests PathItem with all HTTP methods
func TestPathItemAllOperations(t *testing.T) {
	pathItem := PathItem{
		Get: &Operation{
			Summary:   "Get resource",
			Tags:      []string{"Resources"},
			Responses: map[string]Response{"200": {Description: "OK"}},
		},
		Post: &Operation{
			Summary: "Create resource",
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{Type: "object"}},
				},
			},
			Responses: map[string]Response{"201": {Description: "Created"}},
		},
		Put: &Operation{
			Summary:   "Update resource",
			Responses: map[string]Response{"200": {Description: "Updated"}},
		},
		Delete: &Operation{
			Summary:   "Delete resource",
			Responses: map[string]Response{"204": {Description: "Deleted"}},
		},
		Patch: &Operation{
			Summary:   "Patch resource",
			Responses: map[string]Response{"200": {Description: "Patched"}},
		},
	}

	// Verify all operations exist
	if pathItem.Get == nil {
		t.Error("Get should not be nil")
	}
	if pathItem.Post == nil {
		t.Error("Post should not be nil")
	}
	if pathItem.Put == nil {
		t.Error("Put should not be nil")
	}
	if pathItem.Delete == nil {
		t.Error("Delete should not be nil")
	}
	if pathItem.Patch == nil {
		t.Error("Patch should not be nil")
	}

	// Test JSON serialization of all operations
	jsonBytes, err := json.Marshal(pathItem)
	if err != nil {
		t.Fatalf("Failed to marshal PathItem: %v", err)
	}

	var decoded PathItem
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal PathItem: %v", err)
	}

	if decoded.Post.RequestBody == nil {
		t.Error("RequestBody should be preserved after serialization")
	}
	if !decoded.Post.RequestBody.Required {
		t.Error("RequestBody.Required should be true")
	}
}

// TestSchemaWithItems tests Schema with Items field for array types
func TestSchemaWithItems(t *testing.T) {
	schema := Schema{
		Type: "array",
		Items: &Schema{
			Type: "object",
			Properties: map[string]Schema{
				"id":   {Type: "integer", Example: 1},
				"name": {Type: "string", Example: "test"},
			},
		},
	}

	if schema.Items == nil {
		t.Error("Items should not be nil")
	}
	if schema.Items.Type != "object" {
		t.Errorf("Items.Type = %q, want object", schema.Items.Type)
	}
	if len(schema.Items.Properties) != 2 {
		t.Errorf("Items.Properties count = %d, want 2", len(schema.Items.Properties))
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Failed to marshal Schema: %v", err)
	}

	var decoded Schema
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Schema: %v", err)
	}

	if decoded.Items == nil {
		t.Error("Items should be preserved after serialization")
	}
}

// TestMediaTypeStruct tests MediaType struct
func TestMediaTypeStruct(t *testing.T) {
	mt := MediaType{
		Schema: &Schema{
			Type: "object",
			Properties: map[string]Schema{
				"message": {Type: "string"},
			},
		},
	}

	if mt.Schema == nil {
		t.Error("Schema should not be nil")
	}

	jsonBytes, err := json.Marshal(mt)
	if err != nil {
		t.Fatalf("Failed to marshal MediaType: %v", err)
	}

	var decoded MediaType
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal MediaType: %v", err)
	}

	if decoded.Schema == nil {
		t.Error("Schema should be preserved after serialization")
	}
}

// TestContactStruct tests Contact struct fields
func TestContactStruct(t *testing.T) {
	contact := Contact{
		Name:  "API Support",
		URL:   "https://support.example.com",
		Email: "support@example.com",
	}

	if contact.Name != "API Support" {
		t.Errorf("Name = %q", contact.Name)
	}
	if contact.URL != "https://support.example.com" {
		t.Errorf("URL = %q", contact.URL)
	}
	if contact.Email != "support@example.com" {
		t.Errorf("Email = %q", contact.Email)
	}

	// Test JSON serialization
	jsonBytes, err := json.Marshal(contact)
	if err != nil {
		t.Fatalf("Failed to marshal Contact: %v", err)
	}

	var decoded Contact
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Contact: %v", err)
	}

	if decoded.Name != contact.Name {
		t.Error("Name mismatch after serialization")
	}
}

// TestGeneratePathsHealthEndpoint tests health endpoint details
func TestGeneratePathsHealthEndpoint(t *testing.T) {
	paths := generatePaths()

	health := paths["/healthz"]
	if health.Get == nil {
		t.Fatal("Health endpoint should have GET operation")
	}

	if health.Get.Summary != "Health check" {
		t.Errorf("Summary = %q", health.Get.Summary)
	}

	if len(health.Get.Tags) == 0 {
		t.Error("Tags should not be empty")
	}

	if _, exists := health.Get.Responses["200"]; !exists {
		t.Error("Should have 200 response")
	}
}

// TestGeneratePathsAutocompleteEndpoint tests autocomplete endpoint details
func TestGeneratePathsAutocompleteEndpoint(t *testing.T) {
	paths := generatePaths()

	autocomplete := paths["/api/v1/autocomplete"]
	if autocomplete.Get == nil {
		t.Fatal("Autocomplete endpoint should have GET operation")
	}

	// Should have required 'q' parameter
	var hasQ bool
	for _, p := range autocomplete.Get.Parameters {
		if p.Name == "q" && p.Required {
			hasQ = true
			break
		}
	}
	if !hasQ {
		t.Error("Autocomplete should have required 'q' parameter")
	}
}

// TestGenerateComponentsSearchResultSchema tests SearchResult schema details
func TestGenerateComponentsSearchResultSchema(t *testing.T) {
	components := generateComponents()

	searchResult, exists := components.Schemas["SearchResult"]
	if !exists {
		t.Fatal("SearchResult schema should exist")
	}

	expectedFields := []string{"title", "url", "content", "engine", "score", "image_url", "thumbnail", "published"}
	for _, field := range expectedFields {
		if _, exists := searchResult.Properties[field]; !exists {
			t.Errorf("SearchResult should have %q field", field)
		}
	}
}

// TestGenerateComponentsHealthResponseSchema tests HealthResponse schema details
func TestGenerateComponentsHealthResponseSchema(t *testing.T) {
	components := generateComponents()

	healthResp, exists := components.Schemas["HealthResponse"]
	if !exists {
		t.Fatal("HealthResponse schema should exist")
	}

	expectedFields := []string{"status", "version", "uptime"}
	for _, field := range expectedFields {
		if _, exists := healthResp.Properties[field]; !exists {
			t.Errorf("HealthResponse should have %q field", field)
		}
	}
}

// TestHandlerHTTPSViaForwardedProto tests handler with HTTPS via proxy
func TestHandlerHTTPSViaForwardedProto(t *testing.T) {
	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	req.Host = "api.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	handler(rec, req)

	var spec OpenAPISpec
	if err := json.NewDecoder(rec.Body).Decode(&spec); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	if len(spec.Servers) == 0 {
		t.Fatal("Servers should not be empty")
	}

	expectedURL := "https://api.example.com"
	if spec.Servers[0].URL != expectedURL {
		t.Errorf("Server URL = %q, want %q", spec.Servers[0].URL, expectedURL)
	}
}

// TestHandlerWithForwardedHost tests handler with forwarded host header
func TestHandlerWithForwardedHost(t *testing.T) {
	cfg := &config.Config{}
	handler := Handler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	req.Host = "localhost:8080"
	req.Header.Set("X-Forwarded-Host", "proxy.example.com")
	rec := httptest.NewRecorder()

	handler(rec, req)

	var spec OpenAPISpec
	if err := json.NewDecoder(rec.Body).Decode(&spec); err != nil {
		t.Fatalf("Failed to decode JSON: %v", err)
	}

	if !strings.Contains(spec.Servers[0].URL, "proxy.example.com") {
		t.Errorf("Server URL should contain forwarded host, got %q", spec.Servers[0].URL)
	}
}

// TestGenerateSpecInfoContact tests complete contact info in spec
func TestGenerateSpecInfoContact(t *testing.T) {
	cfg := &config.Config{}
	spec := GenerateSpec(cfg, "http://localhost:8080")

	if spec.Info.Contact == nil {
		t.Fatal("Contact should not be nil")
	}

	if spec.Info.Contact.Name != "apimgr" {
		t.Errorf("Contact.Name = %q, want apimgr", spec.Info.Contact.Name)
	}

	if spec.Info.Contact.URL != "https://github.com/apimgr/search" {
		t.Errorf("Contact.URL = %q", spec.Info.Contact.URL)
	}
}

// TestGenerateSpecServerDescription tests server description
func TestGenerateSpecServerDescription(t *testing.T) {
	cfg := &config.Config{}
	spec := GenerateSpec(cfg, "http://localhost:8080")

	if len(spec.Servers) == 0 {
		t.Fatal("Should have at least one server")
	}

	if spec.Servers[0].Description != "Search API Server" {
		t.Errorf("Server description = %q", spec.Servers[0].Description)
	}
}

// TestGenerateSpecComponents tests that components are included
func TestGenerateSpecComponents(t *testing.T) {
	cfg := &config.Config{}
	spec := GenerateSpec(cfg, "http://localhost:8080")

	if len(spec.Components.Schemas) == 0 {
		t.Error("Components.Schemas should not be empty")
	}
}

// TestOperationWithRequestBody tests Operation with RequestBody
func TestOperationWithRequestBody(t *testing.T) {
	op := Operation{
		Summary:     "Create resource",
		Description: "Creates a new resource",
		Tags:        []string{"Resources"},
		RequestBody: &RequestBody{
			Description: "Resource to create",
			Required:    true,
			Content: map[string]MediaType{
				"application/json": {
					Schema: &Schema{
						Type: "object",
						Properties: map[string]Schema{
							"name": {Type: "string"},
						},
					},
				},
			},
		},
		Responses: map[string]Response{
			"201": {
				Description: "Created",
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{Type: "object"}},
				},
			},
		},
	}

	if op.RequestBody == nil {
		t.Error("RequestBody should not be nil")
	}
	if !op.RequestBody.Required {
		t.Error("RequestBody.Required should be true")
	}

	// Verify JSON serialization
	jsonBytes, err := json.Marshal(op)
	if err != nil {
		t.Fatalf("Failed to marshal Operation: %v", err)
	}

	if !strings.Contains(string(jsonBytes), "requestBody") {
		t.Error("JSON should contain requestBody")
	}
}

// TestSearchEndpointAllParameters tests all parameters on search endpoint
func TestSearchEndpointAllParameters(t *testing.T) {
	cfg := &config.Config{}
	spec := GenerateSpec(cfg, "http://localhost:8080")

	searchPath := spec.Paths["/api/v1/search"]
	if searchPath.Get == nil {
		t.Fatal("Search endpoint should have GET operation")
	}

	expectedParams := map[string]bool{
		"q":        true,  // required
		"category": false, // optional
		"page":     false, // optional
		"lang":     false, // optional
	}

	foundParams := make(map[string]bool)
	for _, p := range searchPath.Get.Parameters {
		foundParams[p.Name] = p.Required
	}

	for name, required := range expectedParams {
		if foundRequired, exists := foundParams[name]; !exists {
			t.Errorf("Missing parameter: %s", name)
		} else if foundRequired != required {
			t.Errorf("Parameter %s required = %v, want %v", name, foundRequired, required)
		}
	}
}

// TestSwaggerThemeConstants verifies theme CSS constants are non-empty
func TestSwaggerThemeConstants(t *testing.T) {
	if swaggerDarkTheme == "" {
		t.Error("swaggerDarkTheme should not be empty")
	}
	if swaggerLightTheme == "" {
		t.Error("swaggerLightTheme should not be empty")
	}

	// Verify dark theme has expected CSS properties
	if !strings.Contains(swaggerDarkTheme, "background:") {
		t.Error("Dark theme should contain background styles")
	}
	if !strings.Contains(swaggerDarkTheme, ".swagger-ui") {
		t.Error("Dark theme should contain .swagger-ui selector")
	}

	// Verify light theme has expected CSS properties
	if !strings.Contains(swaggerLightTheme, "background:") {
		t.Error("Light theme should contain background styles")
	}
	if !strings.Contains(swaggerLightTheme, ".swagger-ui") {
		t.Error("Light theme should contain .swagger-ui selector")
	}
}

// TestAPIv1HealthzEndpoint tests that API v1 healthz matches root healthz
func TestAPIv1HealthzEndpoint(t *testing.T) {
	paths := generatePaths()

	rootHealth := paths["/healthz"]
	apiHealth := paths["/api/v1/healthz"]

	if rootHealth.Get == nil {
		t.Fatal("Root health endpoint should have GET")
	}
	if apiHealth.Get == nil {
		t.Fatal("API v1 health endpoint should have GET")
	}

	// They should be the same operation
	if rootHealth.Get.Summary != apiHealth.Get.Summary {
		t.Error("Both health endpoints should have same summary")
	}
}
