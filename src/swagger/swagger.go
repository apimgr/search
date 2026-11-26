package swagger

//go:generate go run ./cmd/openapi-gen -output openapi.json

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
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

const debugMountedCondition = "--debug || DEBUG=true || MODE=debug"

// OpenAPISpec represents the OpenAPI 3.0 specification
type OpenAPISpec struct {
	OpenAPI    string              `json:"openapi"`
	Info       Info                `json:"info"`
	Servers    []Server            `json:"servers"`
	Paths      map[string]PathItem `json:"paths"`
	Components Components          `json:"components,omitempty"`
}

type Info struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	XDescription string   `json:"x-description,omitempty"`
	Version      string   `json:"version"`
	Contact      *Contact `json:"contact,omitempty"`
}

type Contact struct {
	Name  string `json:"name,omitempty"`
	URL   string `json:"url,omitempty"`
	Email string `json:"email,omitempty"`
}

type Server struct {
	URL          string `json:"url"`
	Description  string `json:"description,omitempty"`
	XDescription string `json:"x-description,omitempty"`
}

type PathItem struct {
	Get          *Operation `json:"get,omitempty"`
	Post         *Operation `json:"post,omitempty"`
	Put          *Operation `json:"put,omitempty"`
	Delete       *Operation `json:"delete,omitempty"`
	Patch        *Operation `json:"patch,omitempty"`
	XMountedWhen string     `json:"x-mounted-when,omitempty"`
}

type Operation struct {
	OperationID  string                `json:"operationId,omitempty"`
	Summary      string                `json:"summary,omitempty"`
	Description  string                `json:"description,omitempty"`
	XDescription string                `json:"x-description,omitempty"`
	Tags         []string              `json:"tags,omitempty"`
	Parameters   []Parameter           `json:"parameters,omitempty"`
	RequestBody  *RequestBody          `json:"requestBody,omitempty"`
	Responses    map[string]Response   `json:"responses"`
	Security     []map[string][]string `json:"security,omitempty"`
}

type Parameter struct {
	Name         string  `json:"name"`
	In           string  `json:"in"`
	Description  string  `json:"description,omitempty"`
	XDescription string  `json:"x-description,omitempty"`
	Required     bool    `json:"required,omitempty"`
	Schema       *Schema `json:"schema,omitempty"`
}

type RequestBody struct {
	Description  string               `json:"description,omitempty"`
	XDescription string               `json:"x-description,omitempty"`
	Required     bool                 `json:"required,omitempty"`
	Content      map[string]MediaType `json:"content"`
}

type Response struct {
	Description  string               `json:"description"`
	XDescription string               `json:"x-description,omitempty"`
	Content      map[string]MediaType `json:"content,omitempty"`
}

type MediaType struct {
	Schema *Schema `json:"schema,omitempty"`
}

type Schema struct {
	Type         string            `json:"type,omitempty"`
	Format       string            `json:"format,omitempty"`
	Nullable     bool              `json:"nullable,omitempty"`
	MinLength    int               `json:"minLength,omitempty"`
	MaxLength    int               `json:"maxLength,omitempty"`
	Description  string            `json:"description,omitempty"`
	XDescription string            `json:"x-description,omitempty"`
	Properties   map[string]Schema `json:"properties,omitempty"`
	Items        *Schema           `json:"items,omitempty"`
	Example      interface{}       `json:"example,omitempty"`
	Enum         []interface{}     `json:"enum,omitempty"`
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
	XDescription string `json:"x-description,omitempty"`
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

// openAPILocalizationKey returns the stable swagger.* catalog key for one
// English annotation. The readable prefix helps translators find the string;
// the digest prevents collisions between punctuation and spelling variants.
func openAPILocalizationKey(value string) string {
	var slug strings.Builder
	lastUnderscore := false
	for _, char := range strings.ToLower(value) {
		if slug.Len() >= 48 {
			break
		}
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			slug.WriteRune(char)
			lastUnderscore = false
			continue
		}
		if slug.Len() > 0 && !lastUnderscore {
			slug.WriteByte('_')
			lastUnderscore = true
		}
	}
	prefix := strings.Trim(slug.String(), "_")
	if prefix == "" {
		prefix = "text"
	}
	digest := sha256.Sum256([]byte(value))
	return "swagger.text." + prefix + "_" + hex.EncodeToString(digest[:6])
}

func translateOpenAPISpec(spec *OpenAPISpec, lang string) {
	translate := func(value *string) {
		if *value == "" {
			return
		}
		key := openAPILocalizationKey(*value)
		if translated := i18n.T(lang, key); translated != key {
			*value = translated
		}
	}
	translate(&spec.Info.Title)
	translate(&spec.Info.Description)
	translate(&spec.Info.XDescription)
	for i := range spec.Servers {
		translate(&spec.Servers[i].Description)
		translate(&spec.Servers[i].XDescription)
	}
	for path, item := range spec.Paths {
		for _, operation := range []*Operation{item.Get, item.Post, item.Put, item.Patch, item.Delete} {
			if operation == nil {
				continue
			}
			translate(&operation.Summary)
			translate(&operation.Description)
			translate(&operation.XDescription)
			for i := range operation.Tags {
				translate(&operation.Tags[i])
			}
			for i := range operation.Parameters {
				translate(&operation.Parameters[i].Description)
				translate(&operation.Parameters[i].XDescription)
				translateSchema(operation.Parameters[i].Schema, translate)
			}
			if operation.RequestBody != nil {
				translate(&operation.RequestBody.Description)
				translate(&operation.RequestBody.XDescription)
				for _, media := range operation.RequestBody.Content {
					translateSchema(media.Schema, translate)
				}
			}
			for code, response := range operation.Responses {
				translate(&response.Description)
				translate(&response.XDescription)
				for _, media := range response.Content {
					translateSchema(media.Schema, translate)
				}
				operation.Responses[code] = response
			}
		}
		spec.Paths[path] = item
	}
	for name, schema := range spec.Components.Schemas {
		translateSchema(&schema, translate)
		spec.Components.Schemas[name] = schema
	}
	for name, scheme := range spec.Components.SecuritySchemes {
		translate(&scheme.Description)
		translate(&scheme.XDescription)
		spec.Components.SecuritySchemes[name] = scheme
	}
}

func translateSchema(schema *Schema, translate func(*string)) {
	if schema == nil {
		return
	}
	translate(&schema.Description)
	translate(&schema.XDescription)
	schema.Example = translateExampleMessage(schema.Example, translate)
	for name, property := range schema.Properties {
		translateSchema(&property, translate)
		if name == "message" {
			if message, ok := property.Example.(string); ok {
				translate(&message)
				property.Example = message
			}
		}
		schema.Properties[name] = property
	}
	translateSchema(schema.Items, translate)
}

func translateExampleMessage(example interface{}, translate func(*string)) interface{} {
	values, ok := example.(map[string]interface{})
	if !ok {
		return example
	}
	for name, value := range values {
		if name == "message" {
			if message, ok := value.(string); ok {
				translate(&message)
				values[name] = message
			}
			continue
		}
		values[name] = translateExampleMessage(value, translate)
	}
	return values
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
		versionedAPIPath := strings.HasPrefix(path, version.APIPrefix+"/")
		healthAlias := path == "/api/healthz"
		if (!versionedAPIPath && !healthAlias) || strings.Contains(path, "/server/mcp") || strings.Contains(path, "/server/preferences/tokens") {
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

// mergeConditionalAnnotations adds conditional routes from the annotation
// source when an older embedded OpenAPI artifact does not yet contain them.
// The generated artifact remains the source for all unconditional operations.
func mergeConditionalAnnotations(spec *OpenAPISpec) {
	for path, item := range generatePaths() {
		if item.XMountedWhen != "" {
			spec.Paths[path] = item
		}
	}
}

// filterUnmountedPaths removes conditional paths that are unavailable for this
// process configuration before the OpenAPI document is served.
func filterUnmountedPaths(spec *OpenAPISpec, cfg *config.Config) {
	for path, item := range spec.Paths {
		if item.XMountedWhen == "" {
			continue
		}
		if !isMountedForRun(item.XMountedWhen, cfg) {
			delete(spec.Paths, path)
		}
	}
}

func isMountedForRun(condition string, cfg *config.Config) bool {
	switch condition {
	case debugMountedCondition:
		return cfg != nil && cfg.IsDebug()
	default:
		return false
	}
}

// SpecHandler serves the generated OpenAPI 3.0 document as JSON.
// Per AI.md PART 14 "Root-Level Endpoints": mounted at
// /api/{api_version}/server/swagger and at the /api/swagger alias (same
// handler, never a redirect). JSON only — no YAML, no .json path suffix.
func SpecHandler(cfg *config.Config) http.HandlerFunc {
	var spec OpenAPISpec
	prepareErr := json.Unmarshal(generatedOpenAPISpec, &spec)
	if prepareErr == nil {
		mergeConditionalAnnotations(&spec)
		filterUnmountedPaths(&spec, cfg)
	}
	preparedSpec, marshalErr := json.Marshal(spec)
	if prepareErr == nil {
		prepareErr = marshalErr
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if prepareErr != nil {
			http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusInternalServerError)
			return
		}
		var spec OpenAPISpec
		if err := json.Unmarshal(preparedSpec, &spec); err != nil {
			http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusInternalServerError)
			return
		}
		baseURL := buildBaseURL(r)
		spec.Servers = []Server{{URL: baseURL, Description: "Search API Server"}}
		spec.Info.Version = config.Version
		if spec.Info.Contact != nil {
			spec.Info.Contact.Email = "noreply@" + extractHost(baseURL)
		}
		lang, _ := i18n.DetectRequestLocale(r)
		translateOpenAPISpec(&spec, lang)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Add("Vary", "Cookie, Accept-Language")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		if err := json.NewEncoder(w).Encode(spec); err != nil {
			http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusInternalServerError)
		}
	}
}

// UIHandler serves the themed Swagger UI.
// Per AI.md PART 14: mounted at /server/docs/swagger and fetches the spec
// from /api/swagger.
func UIHandler(_ *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang, dir := i18n.DetectRequestLocale(r)
		theme := getTheme(r)
		specURL := html.EscapeString(buildBaseURL(r) + version.APIPrefix + "/server/swagger")
		title := html.EscapeString(i18n.RequestString(r, "swagger.ui.title"))
		specLink := html.EscapeString(i18n.RequestString(r, "swagger.ui.spec_link"))
		javascriptRequired := html.EscapeString(i18n.RequestString(r, "swagger.ui.javascript_required"))
		page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s" dir="%s" data-theme="%s">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>%s</title>
	<link rel="stylesheet" href="/static/vendor/swagger-ui/swagger-ui.css">
	<link rel="stylesheet" href="/static/css/swagger.css">
	<link rel="stylesheet" href="/static/css/swagger-theme.css">
</head>
<body>
	<main>
		<h1>%s</h1>
		<p><a href="%s">%s</a></p>
		<div id="swagger-ui" data-spec-url="%s" aria-live="polite"><p>%s</p></div>
		<script src="/static/vendor/swagger-ui/swagger-ui-bundle.js"></script>
		<script src="/static/vendor/swagger-ui/swagger-ui-standalone-preset.js"></script>
		<script src="/static/js/swagger.js"></script>
	</main>
</body>
</html>`, html.EscapeString(lang), html.EscapeString(dir), theme, title, title, specURL, specLink, specURL, javascriptRequired)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write([]byte(page)); err != nil {
			http.Error(w, i18n.RequestString(r, "errors.server_error"), http.StatusInternalServerError)
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
