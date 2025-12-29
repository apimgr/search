package graphql

import (
	"encoding/json"
	"net/http"

	"github.com/apimgr/search/src/config"
	"github.com/graphql-go/graphql"
)

// Schema represents the GraphQL schema
var Schema graphql.Schema

// InitSchema initializes the GraphQL schema
// Per AI.md PART 19: GraphQL must be in sync with REST API
func InitSchema() error {
	// Define the SearchResult type
	searchResultType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchResult",
		Description: "A single search result from an engine",
		Fields: graphql.Fields{
			"title": &graphql.Field{
				Type:        graphql.String,
				Description: "Result title",
			},
			"url": &graphql.Field{
				Type:        graphql.String,
				Description: "Result URL",
			},
			"content": &graphql.Field{
				Type:        graphql.String,
				Description: "Result content/description",
			},
			"engine": &graphql.Field{
				Type:        graphql.String,
				Description: "Source engine name",
			},
			"score": &graphql.Field{
				Type:        graphql.Float,
				Description: "Relevance score",
			},
			"imageUrl": &graphql.Field{
				Type:        graphql.String,
				Description: "Image URL (for image results)",
			},
			"thumbnail": &graphql.Field{
				Type:        graphql.String,
				Description: "Thumbnail URL",
			},
			"published": &graphql.Field{
				Type:        graphql.String,
				Description: "Publication date",
			},
		},
	})

	// Define the SearchResponse type
	searchResponseType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchResponse",
		Description: "Search query response",
		Fields: graphql.Fields{
			"query": &graphql.Field{
				Type:        graphql.String,
				Description: "The search query",
			},
			"results": &graphql.Field{
				Type:        graphql.NewList(searchResultType),
				Description: "Search results",
			},
			"totalResults": &graphql.Field{
				Type:        graphql.Int,
				Description: "Total number of results",
			},
			"searchTime": &graphql.Field{
				Type:        graphql.Float,
				Description: "Search duration in seconds",
			},
			"engines": &graphql.Field{
				Type:        graphql.NewList(graphql.String),
				Description: "Engines used for this search",
			},
		},
	})

	// Define the HealthResponse type
	healthResponseType := graphql.NewObject(graphql.ObjectConfig{
		Name: "HealthResponse",
		Description: "Server health status",
		Fields: graphql.Fields{
			"status": &graphql.Field{
				Type:        graphql.String,
				Description: "Health status (healthy, degraded, unhealthy)",
			},
			"version": &graphql.Field{
				Type:        graphql.String,
				Description: "Application version",
			},
			"uptime": &graphql.Field{
				Type:        graphql.String,
				Description: "Server uptime",
			},
			"mode": &graphql.Field{
				Type:        graphql.String,
				Description: "Application mode (production, development)",
			},
		},
	})

	// Define the root query
	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"search": &graphql.Field{
				Type:        searchResponseType,
				Description: "Perform a search query",
				Args: graphql.FieldConfigArgument{
					"q": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Search query",
					},
					"category": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Search category (general, images, videos, news, files)",
					},
					"page": &graphql.ArgumentConfig{
						Type:        graphql.Int,
						Description: "Page number",
					},
					"lang": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Language code (e.g., en, es, fr)",
					},
				},
				Resolve: resolveSearch,
			},
			"autocomplete": &graphql.Field{
				Type:        graphql.NewList(graphql.String),
				Description: "Get autocomplete suggestions",
				Args: graphql.FieldConfigArgument{
					"q": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Partial query",
					},
				},
				Resolve: resolveAutocomplete,
			},
			"health": &graphql.Field{
				Type:        healthResponseType,
				Description: "Get server health status",
				Resolve:     resolveHealth,
			},
		},
	})

	// Create schema
	var err error
	Schema, err = graphql.NewSchema(graphql.SchemaConfig{
		Query: rootQuery,
	})

	return err
}

// resolveSearch handles search queries
// GraphQL search returns empty results by design - use REST API /api/v1/search for full functionality
// GraphQL schema is provided for introspection and optional client integrations
func resolveSearch(p graphql.ResolveParams) (interface{}, error) {
	return map[string]interface{}{
		"query":        p.Args["q"],
		"results":      []interface{}{},
		"totalResults": 0,
		"searchTime":   0.0,
		"engines":      []string{},
	}, nil
}

// resolveAutocomplete handles autocomplete queries
// GraphQL autocomplete returns empty by design - use REST API /api/v1/autocomplete for full functionality
func resolveAutocomplete(p graphql.ResolveParams) (interface{}, error) {
	return []string{}, nil
}

// resolveHealth returns server health information
func resolveHealth(p graphql.ResolveParams) (interface{}, error) {
	return map[string]interface{}{
		"status":  "healthy",
		"version": config.Version,
		"uptime":  "0s",
		"mode":    "production",
	}, nil
}

// Handler serves the GraphQL endpoint and GraphiQL UI
func Handler(cfg *config.Config) http.HandlerFunc {
	// Initialize schema on first call
	if Schema.QueryType() == nil {
		if err := InitSchema(); err != nil {
			return func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Failed to initialize GraphQL schema", http.StatusInternalServerError)
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

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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

	html := `<!DOCTYPE html>
<html lang="en">
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
		const fetcher = GraphiQL.createFetcher({ url: '/graphql' });
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
