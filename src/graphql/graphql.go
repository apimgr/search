package graphql

import (
	"encoding/json"
	"net/http"

	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/config"
	"github.com/graphql-go/graphql"
)

func localizedHTTPError(w http.ResponseWriter, r *http.Request, status int, key string, args ...interface{}) {
	http.Error(w, i18n.RequestString(r, key, args...), status)
}

// Handler serves the GraphQL endpoint and GraphiQL UI
func Handler(cfg *config.Config) http.HandlerFunc {
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
			handleGraphQLQuery(w, r)
			return
		}

		// Handle GET requests (GraphiQL UI)
		if r.Method == http.MethodGet {
			serveGraphiQL(w, r)
			return
		}

		localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
	}
}

// handleGraphQLQuery executes a GraphQL query
func handleGraphQLQuery(w http.ResponseWriter, r *http.Request) {
	var params struct {
		Query         string                 `json:"query"`
		Variables     map[string]interface{} `json:"variables"`
		OperationName string                 `json:"operationName"`
	}

	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		localizedHTTPError(w, r, http.StatusBadRequest, "errors.bad_request")
		return
	}

	result := graphql.Do(graphql.Params{
		Schema:         Schema,
		RequestString:  params.Query,
		VariableValues: params.Variables,
		OperationName:  params.OperationName,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// serveGraphiQL serves the GraphiQL interface
func serveGraphiQL(w http.ResponseWriter, r *http.Request) {
	// Get theme from cookie or default to dark
	theme := getTheme(r)
	lang, dir := i18n.DetectRequestLocale(r)

	html := `<!DOCTYPE html>
<html lang="` + lang + `" dir="` + dir + `">
<head>
	<meta charset="UTF-8">
	<title>Search API - GraphiQL</title>
	<link rel="stylesheet" href="https://unpkg.com/graphiql@3/graphiql.min.css">
	<style>` + getGraphiQLThemeCSS(theme) + `</style>
</head>
<body>
	<div id="graphiql"></div>
	<script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
	<script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
	<script src="https://unpkg.com/graphiql@3/graphiql.min.js"></script>
	<script>
		const fetcher = GraphiQL.createFetcher({ url: '/api/graphql' });
		ReactDOM.render(
			React.createElement(GraphiQL, { fetcher: fetcher }),
			document.getElementById('graphiql')
		);
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

// UIHandler returns an http.HandlerFunc that serves the GraphiQL UI (GET only).
// Registered at GET /server/docs/graphql per AI.md PART 14.
func UIHandler(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			localizedHTTPError(w, r, http.StatusMethodNotAllowed, "errors.method_not_allowed")
			return
		}
		serveGraphiQL(w, r)
	}
}

// QueryHandler returns an http.HandlerFunc that handles GraphQL POST queries only.
// Registered at POST /api/graphql and POST /api/v1/server/graphql per AI.md PART 14.
func QueryHandler(cfg *config.Config) http.HandlerFunc {
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
		handleGraphQLQuery(w, r)
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
