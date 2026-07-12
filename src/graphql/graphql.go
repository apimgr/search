package graphql

import (
	"encoding/json"
	"net/http"

	"github.com/apimgr/search/src/common/httputil"
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

// serveGraphiQL serves a server-rendered GraphQL explorer UI.
// Per frontend rules: vanilla JS only, no CDN frameworks, CSS custom properties for theming.
func serveGraphiQL(w http.ResponseWriter, r *http.Request) {
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
		body { font-family: monospace; background: var(--color-bg); color: var(--color-text); min-height: 100vh; display: flex; flex-direction: column; }
		header { padding: 0.75rem 1rem; background: var(--color-surface); border-bottom: 1px solid var(--color-border); display: flex; align-items: center; gap: 1rem; }
		h1 { font-size: 1rem; font-weight: 600; color: var(--color-primary); }
		main { display: grid; grid-template-columns: 1fr 1fr; grid-template-rows: auto 1fr; gap: 0; flex: 1; }
		.panel { display: flex; flex-direction: column; border-right: 1px solid var(--color-border); }
		.panel:last-child { border-right: none; }
		.panel-header { padding: 0.5rem 1rem; background: var(--color-surface); border-bottom: 1px solid var(--color-border); font-size: 0.75rem; color: var(--color-text-muted); display: flex; justify-content: space-between; align-items: center; }
		textarea { flex: 1; width: 100%; padding: 1rem; background: var(--color-bg); color: var(--color-text); border: none; resize: none; font-family: monospace; font-size: 0.875rem; outline: none; min-height: 300px; }
		pre { flex: 1; padding: 1rem; overflow: auto; font-size: 0.875rem; white-space: pre-wrap; word-break: break-all; background: var(--color-bg); min-height: 300px; }
		button { padding: 0.375rem 0.875rem; background: var(--color-primary); color: #fff; border: none; border-radius: 3px; cursor: pointer; font-size: 0.8rem; font-family: monospace; }
		button:hover { opacity: 0.85; }
		.error { color: #e74c3c; }
		@media (max-width: 700px) { main { grid-template-columns: 1fr; } .panel { border-right: none; border-bottom: 1px solid var(--color-border); } }
	</style>
</head>
<body>
	<header>
		<h1>Search API Explorer</h1>
	</header>
	<main>
		<div class="panel">
			<div class="panel-header">
				<span>Query</span>
				<button id="run-btn" onclick="runQuery()">Run ▶</button>
			</div>
			<textarea id="query" spellcheck="false" placeholder="{ health { status version } }">{ health { status version mode uptime } }</textarea>
		</div>
		<div class="panel">
			<div class="panel-header"><span>Response</span></div>
			<pre id="response">Run a query to see results.</pre>
		</div>
	</main>
	<script>
		function runQuery() {
			var q = document.getElementById('query').value.trim();
			var out = document.getElementById('response');
			out.textContent = 'Running…';
			out.className = '';
			fetch('/api/graphql', {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ query: q })
			})
			.then(function(r) { return r.json(); })
			.then(function(d) { out.textContent = JSON.stringify(d, null, 2); })
			.catch(function(e) { out.textContent = 'Error: ' + e.message; out.className = 'error'; });
		}
		document.getElementById('query').addEventListener('keydown', function(e) {
			if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') { e.preventDefault(); runQuery(); }
		});
	</script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

// buildBaseURL constructs the base URL from the request.
// Per AI.md PART 12: reverse-proxy headers honored only from trusted proxies.
func buildBaseURL(r *http.Request) string {
	proto := httputil.GetProtoFromRequest(r)
	host := httputil.GetHostFromRequest(r)
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
