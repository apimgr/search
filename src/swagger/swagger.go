package swagger

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/apimgr/search/src/config"
)

// OpenAPISpec represents the OpenAPI 3.0 specification
type OpenAPISpec struct {
	OpenAPI string                 `json:"openapi"`
	Info    Info                   `json:"info"`
	Servers []Server               `json:"servers"`
	Paths   map[string]PathItem    `json:"paths"`
	Components Components           `json:"components,omitempty"`
}

type Info struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Version     string  `json:"version"`
	Contact     *Contact `json:"contact,omitempty"`
}

type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

type Server struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type PathItem struct {
	Get    *Operation `json:"get,omitempty"`
	Post   *Operation `json:"post,omitempty"`
	Put    *Operation `json:"put,omitempty"`
	Delete *Operation `json:"delete,omitempty"`
	Patch  *Operation `json:"patch,omitempty"`
}

type Operation struct {
	Summary     string              `json:"summary,omitempty"`
	Description string              `json:"description,omitempty"`
	Tags        []string            `json:"tags,omitempty"`
	Parameters  []Parameter         `json:"parameters,omitempty"`
	RequestBody *RequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]Response `json:"responses"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

type RequestBody struct {
	Description string             `json:"description,omitempty"`
	Required    bool               `json:"required,omitempty"`
	Content     map[string]MediaType `json:"content"`
}

type Response struct {
	Description string             `json:"description"`
	Content     map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type Schema struct {
	Type       string            `json:"type,omitempty"`
	Properties map[string]Schema `json:"properties,omitempty"`
	Items      *Schema           `json:"items,omitempty"`
	Example    interface{}       `json:"example,omitempty"`
}

type Components struct {
	Schemas map[string]Schema `json:"schemas,omitempty"`
}

// GenerateSpec generates the OpenAPI specification for the search API
func GenerateSpec(cfg *config.Config, baseURL string) *OpenAPISpec {
	spec := &OpenAPISpec{
		OpenAPI: "3.0.0",
		Info: Info{
			Title:       "Search API",
			Description: "Privacy-respecting metasearch engine API",
			Version:     config.Version,
			Contact: &Contact{
				Name:  "apimgr",
				URL:   "https://github.com/apimgr/search",
				Email: "noreply@" + extractHost(baseURL),
			},
		},
		Servers: []Server{
			{
				URL:         baseURL,
				Description: "Search API Server",
			},
		},
		Paths:      generatePaths(),
		Components: generateComponents(),
	}

	return spec
}

// generatePaths generates all API endpoint definitions
func generatePaths() map[string]PathItem {
	paths := make(map[string]PathItem)

	// Health check
	paths["/healthz"] = PathItem{
		Get: &Operation{
			Summary:     "Health check",
			Description: "Returns server health status",
			Tags:        []string{"System"},
			Responses: map[string]Response{
				"200": {
					Description: "Server is healthy",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type: "object",
								Properties: map[string]Schema{
									"status":  {Type: "string", Example: "healthy"},
									"version": {Type: "string", Example: "1.0.0"},
									"uptime":  {Type: "string", Example: "2d 5h 30m"},
								},
							},
						},
					},
				},
			},
		},
	}

	// API v1 health check
	paths["/api/v1/healthz"] = paths["/healthz"]

	// Search endpoint
	paths["/api/v1/search"] = PathItem{
		Get: &Operation{
			Summary:     "Search",
			Description: "Perform a metasearch query across multiple engines",
			Tags:        []string{"Search"},
			Parameters: []Parameter{
				{
					Name:        "q",
					In:          "query",
					Description: "Search query",
					Required:    true,
					Schema:      &Schema{Type: "string"},
				},
				{
					Name:        "category",
					In:          "query",
					Description: "Search category (general, images, videos, news, files)",
					Required:    false,
					Schema:      &Schema{Type: "string"},
				},
				{
					Name:        "page",
					In:          "query",
					Description: "Page number (default: 1)",
					Required:    false,
					Schema:      &Schema{Type: "integer"},
				},
				{
					Name:        "lang",
					In:          "query",
					Description: "Language code (e.g., en, es, fr)",
					Required:    false,
					Schema:      &Schema{Type: "string"},
				},
			},
			Responses: map[string]Response{
				"200": {
					Description: "Search results",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type: "object",
								Properties: map[string]Schema{
									"query":         {Type: "string"},
									"results":       {Type: "array", Items: &Schema{Type: "object"}},
									"total_results": {Type: "integer"},
									"search_time":   {Type: "number"},
									"engines":       {Type: "array", Items: &Schema{Type: "string"}},
								},
							},
						},
					},
				},
			},
		},
	}

	// Autocomplete endpoint
	paths["/api/v1/autocomplete"] = PathItem{
		Get: &Operation{
			Summary:     "Autocomplete suggestions",
			Description: "Get search query suggestions",
			Tags:        []string{"Search"},
			Parameters: []Parameter{
				{
					Name:        "q",
					In:          "query",
					Description: "Partial query",
					Required:    true,
					Schema:      &Schema{Type: "string"},
				},
			},
			Responses: map[string]Response{
				"200": {
					Description: "Autocomplete suggestions",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type:  "array",
								Items: &Schema{Type: "string"},
							},
						},
					},
				},
			},
		},
	}

	return paths
}

// generateComponents generates reusable component schemas
func generateComponents() Components {
	return Components{
		Schemas: map[string]Schema{
			"SearchResult": {
				Type: "object",
				Properties: map[string]Schema{
					"title":      {Type: "string"},
					"url":        {Type: "string"},
					"content":    {Type: "string"},
					"engine":     {Type: "string"},
					"score":      {Type: "number"},
					"image_url":  {Type: "string"},
					"thumbnail":  {Type: "string"},
					"published":  {Type: "string"},
				},
			},
			"HealthResponse": {
				Type: "object",
				Properties: map[string]Schema{
					"status":  {Type: "string"},
					"version": {Type: "string"},
					"uptime":  {Type: "string"},
				},
			},
		},
	}
}

// Handler serves the Swagger UI and OpenAPI spec
func Handler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get base URL from request
		baseURL := buildBaseURL(r)

		// Serve OpenAPI JSON spec
		if strings.HasSuffix(r.URL.Path, ".json") || r.Header.Get("Accept") == "application/json" {
			spec := GenerateSpec(cfg, baseURL)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(spec)
			return
		}

		// Serve Swagger UI
		serveSwaggerUI(w, r, baseURL)
	}
}

// serveSwaggerUI serves the Swagger UI HTML
func serveSwaggerUI(w http.ResponseWriter, r *http.Request, baseURL string) {
	// Get theme from cookie or default to dark
	theme := getTheme(r)

	html := `<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<title>Search API - Swagger UI</title>
	<link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
	<style>` + getSwaggerThemeCSS(theme) + `</style>
</head>
<body>
	<div id="swagger-ui"></div>
	<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
	<script>
		window.onload = function() {
			SwaggerUIBundle({
				url: "` + baseURL + `/openapi.json",
				dom_id: '#swagger-ui',
				deepLinking: true,
				presets: [
					SwaggerUIBundle.presets.apis,
					SwaggerUIBundle.SwaggerUIStandalonePreset
				]
			});
		};
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// buildBaseURL constructs the base URL from the request
// Per AI.md PART 5: URL Variables (NON-NEGOTIABLE)
func buildBaseURL(r *http.Request) string {
	proto := "http"
	if r.TLS != nil {
		proto = "https"
	}

	// Check X-Forwarded-Proto header (reverse proxy)
	if forwardedProto := r.Header.Get("X-Forwarded-Proto"); forwardedProto != "" {
		proto = forwardedProto
	}

	host := r.Host

	// Check X-Forwarded-Host header (reverse proxy)
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}

	return proto + "://" + host
}

// extractHost extracts hostname from URL
func extractHost(url string) string {
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	parts := strings.Split(url, "/")
	// strings.Split always returns at least one element (even for empty string)
	// so parts[0] is always safe to access
	return strings.Split(parts[0], ":")[0]
}

// getTheme gets the current theme from cookie or defaults to dark
// Per AI.md PART 16: Themes (NON-NEGOTIABLE - PROJECT-WIDE)
func getTheme(r *http.Request) string {
	if cookie, err := r.Cookie("theme"); err == nil {
		switch cookie.Value {
		case "light", "dark", "auto":
			return cookie.Value
		}
	}
	return "dark" // Default to dark theme
}
