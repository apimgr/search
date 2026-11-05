package graphql

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/common/httputil"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/widget"
	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/parser"
)

const (
	maxGraphQLRequestBytes = 64 << 10
	maxGraphQLQueryDepth   = 12
	maxGraphQLQueryFields  = 100
)

// localizedHTTPError writes a transport-level error (bad method, bad body,
// schema init failure) that occurs before a GraphQL query is executed.
// These are not GraphQL responses (no query ran), so they use the standard
// REST error envelope per AI.md PART 14: {"ok":false,"error":"CODE","message":"..."}.
// Once a query executes, graphql-go's own {data,errors} shape (GraphQL spec)
// is returned unmodified — that shape is mandated by the GraphQL spec itself.
func localizedHTTPError(w http.ResponseWriter, r *http.Request, status int, key string, args ...interface{}) {
	errorCode := strings.ReplaceAll(strings.ToUpper(http.StatusText(status)), " ", "_")
	body := map[string]interface{}{
		"ok":      false,
		"error":   errorCode,
		"message": i18n.RequestString(r, key, args...),
		"details": map[string]interface{}{},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Debug("failed to write GraphQL error response", "error", err)
	}
}

// Handler serves the GraphQL endpoint and GraphiQL UI
func Handler(cfg *config.Config, dependencies ...Dependencies) http.HandlerFunc {
	// Initialize schema on first call
	if Schema.QueryType() == nil {
		if err := InitSchema(); err != nil {
			return func(w http.ResponseWriter, r *http.Request) {
				localizedHTTPError(w, r, http.StatusInternalServerError, "errors.server_error")
			}
		}
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// Handle POST requests (GraphQL queries)
		if r.Method == http.MethodPost {
			if len(dependencies) > 0 {
				ctx := context.WithValue(r.Context(), dependenciesContextKey{}, dependencies[0])
				handleGraphQLQueryWithContext(w, r.WithContext(ctx))
			} else {
				handleGraphQLQuery(w, r)
			}
			return
		}

		// Handle GET requests (GraphiQL UI)
		if r.Method == http.MethodGet {
			serveGraphiQL(w, r, "Run a query to see results.")
			return
		}

		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
	}
}

// handleGraphQLQuery executes a GraphQL query
func handleGraphQLQuery(w http.ResponseWriter, r *http.Request) {
	handleGraphQLQueryWithContext(w, r)
}

func handleGraphQLQueryWithContext(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxGraphQLRequestBytes)
	var params graphQLRequestParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			localizedHTTPError(w, r, http.StatusRequestEntityTooLarge, "errors.bad_request")
			return
		}
		localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}
	if cost, depth, ok := graphQLQueryCost(params.Query); ok && (cost > maxGraphQLQueryFields || depth > maxGraphQLQueryDepth) {
		localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}

	result := executeGraphQLQuery(w, r, params)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(result); err != nil {
		slog.Debug("failed to write GraphQL response", "error", err)
	}
}

type graphQLRequestParams struct {
	Query         string                 `json:"query"`
	Variables     map[string]interface{} `json:"variables"`
	OperationName string                 `json:"operationName"`
}

func executeGraphQLQuery(w http.ResponseWriter, r *http.Request, params graphQLRequestParams) *graphql.Result {
	ctx := context.WithValue(r.Context(), graphqlRequestContextKey{}, r)
	ctx = context.WithValue(ctx, graphqlResponseWriterContextKey{}, w)
	return graphql.Do(graphql.Params{
		Schema:         Schema,
		RequestString:  params.Query,
		VariableValues: params.Variables,
		OperationName:  params.OperationName,
		Context:        ctx,
	})
}

func graphQLQueryCost(query string) (int, int, bool) {
	document, err := parser.Parse(parser.ParseParams{Source: query})
	if err != nil {
		return 0, 0, false
	}
	fragments := make(map[string]*ast.FragmentDefinition)
	for _, definition := range document.Definitions {
		if fragment, ok := definition.(*ast.FragmentDefinition); ok && fragment.Name != nil {
			fragments[fragment.Name.Value] = fragment
		}
	}
	maxCost, maxDepth := 0, 0
	for _, definition := range document.Definitions {
		operation, ok := definition.(*ast.OperationDefinition)
		if !ok {
			continue
		}
		cost, depth, valid := graphQLSelectionCost(operation.SelectionSet, fragments, make(map[string]bool), 0)
		if !valid {
			return 0, 0, true
		}
		if cost > maxCost {
			maxCost = cost
		}
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxCost, maxDepth, true
}

func graphQLSelectionCost(set *ast.SelectionSet, fragments map[string]*ast.FragmentDefinition, active map[string]bool, depth int) (int, int, bool) {
	if set == nil {
		return 0, depth, true
	}
	cost, maxDepth := 0, depth
	for _, selection := range set.Selections {
		switch item := selection.(type) {
		case *ast.Field:
			cost++
			childDepth := depth + 1
			if childDepth > maxDepth {
				maxDepth = childDepth
			}
			childCost, childMax, valid := graphQLSelectionCost(item.SelectionSet, fragments, active, childDepth)
			if !valid {
				return 0, 0, false
			}
			cost += childCost
			if childMax > maxDepth {
				maxDepth = childMax
			}
		case *ast.InlineFragment:
			childCost, childMax, valid := graphQLSelectionCost(item.SelectionSet, fragments, active, depth)
			if !valid {
				return 0, 0, false
			}
			cost += childCost
			if childMax > maxDepth {
				maxDepth = childMax
			}
		case *ast.FragmentSpread:
			if item.Name == nil || active[item.Name.Value] {
				return 0, 0, false
			}
			fragment := fragments[item.Name.Value]
			if fragment == nil {
				return 0, 0, false
			}
			active[item.Name.Value] = true
			childCost, childMax, valid := graphQLSelectionCost(fragment.SelectionSet, fragments, active, depth)
			delete(active, item.Name.Value)
			if !valid {
				return 0, 0, false
			}
			cost += childCost
			if childMax > maxDepth {
				maxDepth = childMax
			}
		}
		if cost > maxGraphQLQueryFields || maxDepth > maxGraphQLQueryDepth {
			return cost, maxDepth, true
		}
	}
	return cost, maxDepth, true
}

// serveGraphiQL serves a server-rendered GraphQL explorer UI.
// Per frontend rules: vanilla JS only, no CDN frameworks, CSS custom properties for theming.
func serveGraphiQL(w http.ResponseWriter, r *http.Request, response string) {
	theme := getTheme(r)
	lang, dir := i18n.DetectRequestLocale(r)
	themeCSS := getGraphiQLThemeCSS(theme)

	html := `<!DOCTYPE html>
<html lang="` + lang + `" dir="` + dir + `">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>Search API Explorer</title>
	<style>
` + themeCSS + `
		*, *::before, *::after { box-sizing: border-box; margin: 0; padding: 0; }
		body { font-family: monospace; background: var(--palette-background); color: var(--palette-foreground); min-height: 100vh; display: flex; flex-direction: column; }
		header { padding: 0.75rem 1rem; background: var(--palette-surface); border-bottom: 1px solid var(--palette-border); display: flex; align-items: center; gap: 1rem; }
		h1 { font-size: 1rem; font-weight: 600; color: var(--palette-primary); }
		main { display: grid; grid-template-columns: 1fr 1fr; grid-template-rows: auto 1fr; gap: 0; flex: 1; }
		.panel { display: flex; flex-direction: column; border-right: 1px solid var(--palette-border); }
		.panel:last-child { border-right: none; }
		.panel-header { padding: 0.5rem 1rem; background: var(--palette-surface); border-bottom: 1px solid var(--palette-border); font-size: 0.75rem; color: var(--palette-muted); display: flex; justify-content: space-between; align-items: center; }
		textarea { flex: 1; width: 100%; padding: 1rem; background: var(--palette-background); color: var(--palette-foreground); border: none; resize: none; font-family: monospace; font-size: 0.875rem; outline: none; min-height: 300px; }
		pre { flex: 1; padding: 1rem; overflow: auto; font-size: 0.875rem; white-space: pre-wrap; word-break: break-all; background: var(--palette-background); color: var(--palette-foreground); min-height: 300px; }
		button { padding: 0.375rem 0.875rem; background: var(--palette-primary); color: var(--palette-background); border: none; border-radius: 3px; cursor: pointer; font-size: 0.8rem; font-family: monospace; }
		button:hover { opacity: 0.85; }
		.error { color: var(--palette-error); }
		@media (max-width: 700px) { main { grid-template-columns: 1fr; } .panel { border-right: none; border-bottom: 1px solid var(--palette-border); } }
	</style>
</head>
<body>
	<header>
		<h1>GraphiQL API Explorer</h1>
		<p>JSON API endpoint: <code>/api/graphql</code></p>
	</header>
	<form id="query-form" method="post" action="/server/docs/graphql">
	<main>
		<div class="panel">
			<div class="panel-header">
				<span>Query</span>
				<button id="run-btn" type="submit">Run ▶</button>
			</div>
			<textarea id="query" name="query" spellcheck="false" placeholder="{ health { status version } }">{ health { status version mode uptime } }</textarea>
		</div>
		<div class="panel">
			<div class="panel-header"><span>Response</span></div>
			<pre id="response">` + html.EscapeString(response) + `</pre>
		</div>
	</main>
	</form>
	<details class="schema-panel">
		<summary>GraphQL schema</summary>
		<pre>` + html.EscapeString(GeneratedSchemaSDL) + `</pre>
	</details>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(html)); err != nil {
		slog.Debug("failed to write GraphQL UI", "error", err)
	}
}

// buildBaseURL constructs the base URL from the request.
// Per AI.md PART 12: reverse-proxy headers honored only from trusted proxies.
func buildBaseURL(r *http.Request) string {
	proto := httputil.GetProtoFromRequest(r)
	host := httputil.GetHostFromRequest(r)
	return proto + "://" + host
}

// UIHandler returns an http.HandlerFunc that serves the GraphiQL UI and its
// native read-only query form. Registered at /server/docs/graphql per AI.md
// PART 14.
func UIHandler(cfg *config.Config) http.HandlerFunc {
	return UIHandlerWithDependencies(cfg, Dependencies{})
}

func UIHandlerWithDependencies(cfg *config.Config, dependencies Dependencies) http.HandlerFunc {
	if dependencies.StartedAt.IsZero() {
		dependencies.StartedAt = time.Now()
	}
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			serveGraphiQL(w, r, "Run a query to see results.")
		case http.MethodPost:
			r.Body = http.MaxBytesReader(w, r.Body, maxGraphQLRequestBytes)
			if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/x-www-form-urlencoded") {
				localizedHTTPError(w, r, http.StatusUnsupportedMediaType, "errors.bad_request")
				return
			}
			if err := r.ParseForm(); err != nil {
				var tooLarge *http.MaxBytesError
				if errors.As(err, &tooLarge) {
					localizedHTTPError(w, r, http.StatusRequestEntityTooLarge, "errors.bad_request")
					return
				}
				localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
				return
			}
			params := graphQLRequestParams{Query: r.FormValue("query")}
			if !graphQLQueryReadOnly(params.Query) {
				localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
				return
			}
			if cost, depth, ok := graphQLQueryCost(params.Query); ok && (cost > maxGraphQLQueryFields || depth > maxGraphQLQueryDepth) {
				localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
				return
			}
			ctx := context.WithValue(r.Context(), dependenciesContextKey{}, dependencies)
			result := executeGraphQLQuery(w, r.WithContext(ctx), params)
			body, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				localizedHTTPError(w, r, http.StatusInternalServerError, "errors.server_error")
				return
			}
			serveGraphiQL(w, r, string(body))
		default:
			localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
			return
		}
	}
}

func graphQLQueryReadOnly(query string) bool {
	document, err := parser.Parse(parser.ParseParams{Source: query})
	if err != nil {
		return false
	}
	found := false
	for _, definition := range document.Definitions {
		operation, ok := definition.(*ast.OperationDefinition)
		if !ok {
			continue
		}
		found = true
		if operation.Operation != ast.OperationTypeQuery {
			return false
		}
	}
	return found
}

// QueryHandler returns an http.HandlerFunc that handles GraphQL POST queries only.
// Registered at POST /api/graphql and POST /api/v1/server/graphql per AI.md PART 14.
func QueryHandler(cfg *config.Config, dependencies ...Dependencies) http.HandlerFunc {
	if len(dependencies) > 0 {
		return QueryHandlerWithDependencies(cfg, dependencies[0])
	}
	return QueryHandlerWithDependencies(cfg, Dependencies{})
}

// Dependencies are the live application services used by GraphQL resolvers.
type Dependencies struct {
	Aggregator        *search.Aggregator
	Direct            *direct.Manager
	Instant           *instant.Manager
	Autocomplete      func(context.Context, string) []string
	RelatedSearches   func(context.Context, string, int) ([]string, error)
	Widgets           *widget.Manager
	API               *api.Handler
	ServerInfo        func() api.InfoResponse
	GeoIPLookup       func(string) (api.GeoIPLookupResponse, error)
	OperatorStatus    func(*http.Request) (interface{}, error)
	OperatorConfig    func(*http.Request) (interface{}, error)
	Preferences       func() interface{}
	SavePreferences   func() interface{}
	EngineInventory   func() []api.EngineInfo
	ThumbnailRewriter func(string) string
	StartedAt         time.Time
}

type dependenciesContextKey struct{}
type graphqlRequestContextKey struct{}
type graphqlResponseWriterContextKey struct{}

func responseWriterFromContext(ctx context.Context) http.ResponseWriter {
	writer, _ := ctx.Value(graphqlResponseWriterContextKey{}).(http.ResponseWriter)
	return writer
}

func requestFromContext(ctx context.Context) *http.Request {
	request, _ := ctx.Value(graphqlRequestContextKey{}).(*http.Request)
	return request
}

// QueryHandlerWithDependencies serves GraphQL against the running application services.
func QueryHandlerWithDependencies(cfg *config.Config, dependencies Dependencies) http.HandlerFunc {
	if dependencies.StartedAt.IsZero() {
		dependencies.StartedAt = time.Now()
	}
	if Schema.QueryType() == nil {
		if err := InitSchema(); err != nil {
			return func(w http.ResponseWriter, r *http.Request) {
				localizedHTTPError(w, r, http.StatusInternalServerError, "errors.server_error")
			}
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
			return
		}
		ctx := context.WithValue(r.Context(), dependenciesContextKey{}, dependencies)
		handleGraphQLQueryWithContext(w, r.WithContext(ctx))
	}
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
