package swagger

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
)

//go:embed openapi.json
var generatedOpenAPISpec []byte

// OpenAPISpec represents the OpenAPI 3.0 specification
type OpenAPISpec struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Servers    []Server            `json:"servers"`
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components,omitempty"`
}

type Info struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
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
	OperationID string                `json:"operationId,omitempty"`
	Summary     string                `json:"summary,omitempty"`
	Description string                `json:"description,omitempty"`
	Tags        []string              `json:"tags,omitempty"`
	Parameters  []Parameter           `json:"parameters,omitempty"`
	RequestBody *RequestBody          `json:"requestBody,omitempty"`
	Responses   map[string]Response   `json:"responses"`
	Security    []map[string][]string `json:"security,omitempty"`
}

type Parameter struct {
	Name        string  `json:"name"`
	In          string  `json:"in"`
	Description string  `json:"description,omitempty"`
	Required    bool    `json:"required,omitempty"`
	Schema      *Schema `json:"schema,omitempty"`
}

type RequestBody struct {
	Description string               `json:"description,omitempty"`
	Required    bool                 `json:"required,omitempty"`
	Content     map[string]MediaType `json:"content"`
}

type Response struct {
	Description string               `json:"description"`
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
	Schemas         map[string]Schema         `json:"schemas,omitempty"`
	SecuritySchemes map[string]SecurityScheme `json:"securitySchemes,omitempty"`
}

// SecurityScheme describes an OpenAPI 3.0 security scheme
type SecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme,omitempty"`
	BearerFormat string `json:"bearerFormat,omitempty"`
	Description  string `json:"description,omitempty"`
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
	assignOperationIDs(spec.Paths)

	return spec
}

func assignOperationIDs(paths map[string]PathItem) {
	pathNames := make([]string, 0, len(paths))
	for path := range paths {
		pathNames = append(pathNames, path)
	}
	sort.Strings(pathNames)
	methods := []struct {
		name string
		get  func(PathItem) *Operation
		set  func(*PathItem, *Operation)
	}{
		{"get", func(item PathItem) *Operation { return item.Get }, func(item *PathItem, operation *Operation) { item.Get = operation }},
		{"post", func(item PathItem) *Operation { return item.Post }, func(item *PathItem, operation *Operation) { item.Post = operation }},
		{"put", func(item PathItem) *Operation { return item.Put }, func(item *PathItem, operation *Operation) { item.Put = operation }},
		{"patch", func(item PathItem) *Operation { return item.Patch }, func(item *PathItem, operation *Operation) { item.Patch = operation }},
		{"delete", func(item PathItem) *Operation { return item.Delete }, func(item *PathItem, operation *Operation) { item.Delete = operation }},
	}
	for _, path := range pathNames {
		item := paths[path]
		if !strings.HasPrefix(path, version.APIPrefix+"/") || strings.Contains(path, "/server/mcp") || strings.Contains(path, "/server/preferences/tokens") {
			continue
		}
		for _, method := range methods {
			operation := method.get(item)
			if operation == nil || operation.OperationID != "" {
				continue
			}
			copy := *operation
			operation = &copy
			var name strings.Builder
			name.WriteString(method.name)
			upper := true
			for _, r := range path {
				if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
					upper = true
					continue
				}
				if upper {
					if r >= 'a' && r <= 'z' {
						r -= 'a' - 'A'
					}
					upper = false
				}
				name.WriteRune(r)
			}
			operation.OperationID = name.String()
			method.set(&item, operation)
		}
		paths[path] = item
	}
}

// SpecHandler serves the generated OpenAPI 3.0 document as JSON.
// Per AI.md PART 14 "Root-Level Endpoints": mounted at
// /api/{api_version}/server/swagger and at the /api/swagger alias (same
// handler, never a redirect). JSON only — no YAML, no .json path suffix.
func SpecHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var spec OpenAPISpec
		if err := json.Unmarshal(generatedOpenAPISpec, &spec); err != nil {
			http.Error(w, "embedded OpenAPI document is invalid", http.StatusInternalServerError)
			return
		}
		baseURL := buildBaseURL(r)
		spec.Servers = []Server{{URL: baseURL, Description: "Search API Server"}}
		spec.Info.Version = config.Version
		if spec.Info.Contact != nil {
			spec.Info.Contact.Email = "noreply@" + extractHost(baseURL)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if err := json.NewEncoder(w).Encode(spec); err != nil {
			http.Error(w, `{"ok":false,"error":"INTERNAL_ERROR","message":"failed to encode OpenAPI spec","details":{}}`, http.StatusInternalServerError)
		}
	}
}

// UIHandler serves the themed Swagger UI.
// Per AI.md PART 14: mounted at /server/docs/swagger and fetches the spec
// from /api/swagger.
func UIHandler(_ *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		theme := getTheme(r)
		lang, dir := i18n.DetectRequestLocale(r)

		html := `<!DOCTYPE html>
<html lang="` + lang + `" dir="` + dir + `" data-theme="` + theme + `">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
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
				url: "` + buildBaseURL(r) + `/api/swagger",
				dom_id: '#swagger-ui',
				deepLinking: true,
				displayRequestDuration: true,
				filter: true,
				tryItOutEnabled: true,
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
		if _, err := w.Write([]byte(html)); err != nil {
			http.Error(w, "failed to write Swagger UI", http.StatusInternalServerError)
		}
	}
}

// buildBaseURL constructs the base URL from the request.
// Per AI.md PART 12: reverse-proxy headers honored only from trusted proxies.
func buildBaseURL(r *http.Request) string {
	proto := httputil.GetProtoFromRequest(r)
	host := httputil.GetHostFromRequest(r)
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
	// Default to dark theme
	return "dark"
}
