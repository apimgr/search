package api

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/model"
	"github.com/graphql-go/graphql"
)

// GraphQL types

var healthType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Health",
	Fields: graphql.Fields{
		"status":    &graphql.Field{Type: graphql.String},
		"version":   &graphql.Field{Type: graphql.String},
		"uptime":    &graphql.Field{Type: graphql.String},
		"timestamp": &graphql.Field{Type: graphql.String},
	},
})

var systemInfoType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SystemInfo",
	Fields: graphql.Fields{
		"goVersion":    &graphql.Field{Type: graphql.String},
		"numCpu":       &graphql.Field{Type: graphql.Int},
		"numGoroutine": &graphql.Field{Type: graphql.Int},
		"memAlloc":     &graphql.Field{Type: graphql.String},
	},
})

var enginesSummaryType = graphql.NewObject(graphql.ObjectConfig{
	Name: "EnginesSummary",
	Fields: graphql.Fields{
		"total":   &graphql.Field{Type: graphql.Int},
		"enabled": &graphql.Field{Type: graphql.Int},
	},
})

var infoType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Info",
	Fields: graphql.Fields{
		"name":        &graphql.Field{Type: graphql.String},
		"version":     &graphql.Field{Type: graphql.String},
		"description": &graphql.Field{Type: graphql.String},
		"uptime":      &graphql.Field{Type: graphql.String},
		"mode":        &graphql.Field{Type: graphql.String},
		"engines":     &graphql.Field{Type: enginesSummaryType},
		"system":      &graphql.Field{Type: systemInfoType},
	},
})

var searchResultType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SearchResult",
	Fields: graphql.Fields{
		"title":       &graphql.Field{Type: graphql.String},
		"url":         &graphql.Field{Type: graphql.String},
		"description": &graphql.Field{Type: graphql.String},
		"engine":      &graphql.Field{Type: graphql.String},
		"score":       &graphql.Field{Type: graphql.Float},
		"category":    &graphql.Field{Type: graphql.String},
		"thumbnail":   &graphql.Field{Type: graphql.String},
		"date":        &graphql.Field{Type: graphql.String},
		"domain":      &graphql.Field{Type: graphql.String},
	},
})

var searchResponseType = graphql.NewObject(graphql.ObjectConfig{
	Name: "SearchResponse",
	Fields: graphql.Fields{
		"query":        &graphql.Field{Type: graphql.String},
		"category":     &graphql.Field{Type: graphql.String},
		"results":      &graphql.Field{Type: graphql.NewList(searchResultType)},
		"totalResults": &graphql.Field{Type: graphql.Int},
		"page":         &graphql.Field{Type: graphql.Int},
		"limit":        &graphql.Field{Type: graphql.Int},
		"hasMore":      &graphql.Field{Type: graphql.Boolean},
		"searchTimeMs": &graphql.Field{Type: graphql.Float},
		"enginesUsed":  &graphql.Field{Type: graphql.NewList(graphql.String)},
	},
})

var engineType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Engine",
	Fields: graphql.Fields{
		"id":          &graphql.Field{Type: graphql.String},
		"name":        &graphql.Field{Type: graphql.String},
		"enabled":     &graphql.Field{Type: graphql.Boolean},
		"priority":    &graphql.Field{Type: graphql.Int},
		"categories":  &graphql.Field{Type: graphql.NewList(graphql.String)},
		"description": &graphql.Field{Type: graphql.String},
		"homepage":    &graphql.Field{Type: graphql.String},
	},
})

var categoryType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Category",
	Fields: graphql.Fields{
		"id":          &graphql.Field{Type: graphql.String},
		"name":        &graphql.Field{Type: graphql.String},
		"description": &graphql.Field{Type: graphql.String},
		"icon":        &graphql.Field{Type: graphql.String},
	},
})

var bangType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Bang",
	Fields: graphql.Fields{
		"shortcut":    &graphql.Field{Type: graphql.String},
		"name":        &graphql.Field{Type: graphql.String},
		"url":         &graphql.Field{Type: graphql.String},
		"category":    &graphql.Field{Type: graphql.String},
		"description": &graphql.Field{Type: graphql.String},
		"aliases":     &graphql.Field{Type: graphql.NewList(graphql.String)},
	},
})

var bangsResponseType = graphql.NewObject(graphql.ObjectConfig{
	Name: "BangsResponse",
	Fields: graphql.Fields{
		"bangs":      &graphql.Field{Type: graphql.NewList(bangType)},
		"total":      &graphql.Field{Type: graphql.Int},
		"categories": &graphql.Field{Type: graphql.NewList(graphql.String)},
	},
})

var widgetInfoType = graphql.NewObject(graphql.ObjectConfig{
	Name: "WidgetInfo",
	Fields: graphql.Fields{
		"type":        &graphql.Field{Type: graphql.String},
		"name":        &graphql.Field{Type: graphql.String},
		"description": &graphql.Field{Type: graphql.String},
		"icon":        &graphql.Field{Type: graphql.String},
		"category":    &graphql.Field{Type: graphql.String},
	},
})

var instantAnswerType = graphql.NewObject(graphql.ObjectConfig{
	Name: "InstantAnswer",
	Fields: graphql.Fields{
		"query":   &graphql.Field{Type: graphql.String},
		"type":    &graphql.Field{Type: graphql.String},
		"title":   &graphql.Field{Type: graphql.String},
		"content": &graphql.Field{Type: graphql.String},
		"source":  &graphql.Field{Type: graphql.String},
		"found":   &graphql.Field{Type: graphql.Boolean},
	},
})

// GraphQLHandler handles GraphQL requests
type GraphQLHandler struct {
	schema  graphql.Schema
	handler *Handler
}

// NewGraphQLHandler creates a new GraphQL handler
func NewGraphQLHandler(h *Handler) (*GraphQLHandler, error) {
	gqlHandler := &GraphQLHandler{handler: h}

	// Build schema
	schema, err := gqlHandler.buildSchema()
	if err != nil {
		return nil, fmt.Errorf("failed to build GraphQL schema: %w", err)
	}

	gqlHandler.schema = schema
	return gqlHandler, nil
}

// buildSchema builds the GraphQL schema
func (g *GraphQLHandler) buildSchema() (graphql.Schema, error) {
	queryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			// Health check
			"healthz": &graphql.Field{
				Type:        healthType,
				Description: "Get health status",
				Resolve:     g.resolveHealthz,
			},

			// Server info
			"info": &graphql.Field{
				Type:        infoType,
				Description: "Get server information",
				Resolve:     g.resolveInfo,
			},

			// Search
			"search": &graphql.Field{
				Type:        searchResponseType,
				Description: "Perform a search query",
				Args: graphql.FieldConfigArgument{
					"query": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Search query string",
					},
					"category": &graphql.ArgumentConfig{
						Type:         graphql.String,
						Description:  "Search category (general, images, videos, news, maps)",
						DefaultValue: "general",
					},
					"page": &graphql.ArgumentConfig{
						Type:         graphql.Int,
						Description:  "Page number",
						DefaultValue: 1,
					},
					"limit": &graphql.ArgumentConfig{
						Type:         graphql.Int,
						Description:  "Results per page (max 100)",
						DefaultValue: 20,
					},
				},
				Resolve: g.resolveSearch,
			},

			// Autocomplete
			"autocomplete": &graphql.Field{
				Type:        graphql.NewList(graphql.String),
				Description: "Get search suggestions",
				Args: graphql.FieldConfigArgument{
					"query": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Query for suggestions",
					},
				},
				Resolve: g.resolveAutocomplete,
			},

			// Engines
			"engines": &graphql.Field{
				Type:        graphql.NewList(engineType),
				Description: "Get all search engines",
				Resolve:     g.resolveEngines,
			},

			// Engine by ID
			"engine": &graphql.Field{
				Type:        engineType,
				Description: "Get engine by ID",
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Engine ID",
					},
				},
				Resolve: g.resolveEngine,
			},

			// Categories
			"categories": &graphql.Field{
				Type:        graphql.NewList(categoryType),
				Description: "Get all search categories",
				Resolve:     g.resolveCategories,
			},

			// Bangs
			"bangs": &graphql.Field{
				Type:        bangsResponseType,
				Description: "Get bang shortcuts",
				Args: graphql.FieldConfigArgument{
					"category": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Filter by category",
					},
					"search": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Search filter",
					},
				},
				Resolve: g.resolveBangs,
			},

			// Widgets
			"widgets": &graphql.Field{
				Type:        graphql.NewList(widgetInfoType),
				Description: "Get available widgets",
				Resolve:     g.resolveWidgets,
			},

			// Instant answer
			"instant": &graphql.Field{
				Type:        instantAnswerType,
				Description: "Get instant answer for query",
				Args: graphql.FieldConfigArgument{
					"query": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Query for instant answer",
					},
				},
				Resolve: g.resolveInstant,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{
		Query: queryType,
	})
}

// Resolvers

func (g *GraphQLHandler) resolveHealthz(p graphql.ResolveParams) (interface{}, error) {
	return map[string]interface{}{
		"status":    "ok",
		"version":   config.Version,
		"uptime":    g.handler.formatDuration(time.Since(g.handler.startTime)),
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (g *GraphQLHandler) resolveInfo(p graphql.ResolveParams) (interface{}, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	enabled := len(g.handler.registry.GetEnabled())

	return map[string]interface{}{
		"name":        g.handler.config.Server.Title,
		"version":     config.Version,
		"description": g.handler.config.Server.Description,
		"uptime":      g.handler.formatDuration(time.Since(g.handler.startTime)),
		"mode":        g.handler.config.Server.Mode,
		"engines": map[string]interface{}{
			"total":   g.handler.registry.Count(),
			"enabled": enabled,
		},
		"system": map[string]interface{}{
			"goVersion":    runtime.Version(),
			"numCpu":       runtime.NumCPU(),
			"numGoroutine": runtime.NumGoroutine(),
			"memAlloc":     g.handler.formatBytes(m.Alloc),
		},
	}, nil
}

func (g *GraphQLHandler) resolveSearch(p graphql.ResolveParams) (interface{}, error) {
	start := time.Now()

	query := p.Args["query"].(string)
	category := "general"
	if cat, ok := p.Args["category"].(string); ok && cat != "" {
		category = cat
	}
	page := 1
	if pg, ok := p.Args["page"].(int); ok && pg > 0 {
		page = pg
	}
	limit := 20
	if lim, ok := p.Args["limit"].(int); ok && lim > 0 && lim <= 100 {
		limit = lim
	}

	// Map category
	var cat model.Category
	switch category {
	case "images":
		cat = model.CategoryImages
	case "videos":
		cat = model.CategoryVideos
	case "news":
		cat = model.CategoryNews
	case "maps":
		cat = model.CategoryMaps
	default:
		cat = model.CategoryGeneral
	}

	// Perform search
	q := model.NewQuery(query)
	q.Category = cat

	ctx := p.Context
	if ctx == nil {
		ctx = context.Background()
	}

	results, err := g.handler.aggregator.Search(ctx, q)
	if err != nil {
		return nil, err
	}

	// Convert results
	apiResults := make([]map[string]interface{}, 0, len(results.Results))
	for i, result := range results.Results {
		if i >= limit {
			break
		}
		apiResults = append(apiResults, map[string]interface{}{
			"title":       result.Title,
			"url":         result.URL,
			"description": result.Content,
			"engine":      result.Engine,
			"score":       result.Score,
			"category":    string(result.Category),
			"thumbnail":   result.Thumbnail,
			"domain":      extractDomain(result.URL),
		})
	}

	return map[string]interface{}{
		"query":        query,
		"category":     category,
		"results":      apiResults,
		"totalResults": results.TotalResults,
		"page":         page,
		"limit":        limit,
		"hasMore":      len(results.Results) > limit,
		"searchTimeMs": float64(time.Since(start).Microseconds()) / 1000,
		"enginesUsed":  results.Engines,
	}, nil
}

func (g *GraphQLHandler) resolveAutocomplete(p graphql.ResolveParams) (interface{}, error) {
	query := p.Args["query"].(string)
	if query == "" {
		return []string{}, nil
	}

	ctx := p.Context
	if ctx == nil {
		ctx = context.Background()
	}

	return g.handler.fetchAutocompleteSuggestions(ctx, query), nil
}

func (g *GraphQLHandler) resolveEngines(p graphql.ResolveParams) (interface{}, error) {
	allEngines := g.handler.registry.GetAll()
	engineList := make([]map[string]interface{}, 0, len(allEngines))

	for _, eng := range allEngines {
		categories := make([]string, 0)
		cfg := eng.GetConfig()
		if cfg != nil {
			for _, cat := range cfg.Categories {
				categories = append(categories, string(cat))
			}
		}

		engineList = append(engineList, map[string]interface{}{
			"id":         eng.Name(),
			"name":       eng.DisplayName(),
			"enabled":    eng.IsEnabled(),
			"priority":   eng.GetPriority(),
			"categories": categories,
		})
	}

	return engineList, nil
}

func (g *GraphQLHandler) resolveEngine(p graphql.ResolveParams) (interface{}, error) {
	id := p.Args["id"].(string)

	engine, err := g.handler.registry.Get(id)
	if err != nil {
		return nil, fmt.Errorf("engine not found: %s", id)
	}

	categories := make([]string, 0)
	cfg := engine.GetConfig()
	if cfg != nil {
		for _, cat := range cfg.Categories {
			categories = append(categories, string(cat))
		}
	}

	return map[string]interface{}{
		"id":         engine.Name(),
		"name":       engine.DisplayName(),
		"enabled":    engine.IsEnabled(),
		"priority":   engine.GetPriority(),
		"categories": categories,
	}, nil
}

func (g *GraphQLHandler) resolveCategories(p graphql.ResolveParams) (interface{}, error) {
	return []map[string]interface{}{
		{"id": "general", "name": "General", "description": "General web search", "icon": "🌐"},
		{"id": "images", "name": "Images", "description": "Image search", "icon": "🖼️"},
		{"id": "videos", "name": "Videos", "description": "Video search", "icon": "🎥"},
		{"id": "news", "name": "News", "description": "News search", "icon": "📰"},
		{"id": "maps", "name": "Maps", "description": "Map and location search", "icon": "🗺️"},
	}, nil
}

func (g *GraphQLHandler) resolveBangs(p graphql.ResolveParams) (interface{}, error) {
	bangs := getBuiltinBangs()

	// Filter by category
	if category, ok := p.Args["category"].(string); ok && category != "" {
		filtered := make([]BangInfo, 0)
		for _, b := range bangs {
			if b.Category == category {
				filtered = append(filtered, b)
			}
		}
		bangs = filtered
	}

	// Search filter
	if search, ok := p.Args["search"].(string); ok && search != "" {
		search = strings.ToLower(search)
		filtered := make([]BangInfo, 0)
		for _, b := range bangs {
			if strings.Contains(strings.ToLower(b.Shortcut), search) ||
				strings.Contains(strings.ToLower(b.Name), search) {
				filtered = append(filtered, b)
			}
		}
		bangs = filtered
	}

	// Convert to map format for GraphQL
	bangMaps := make([]map[string]interface{}, 0, len(bangs))
	for _, b := range bangs {
		bangMaps = append(bangMaps, map[string]interface{}{
			"shortcut":    b.Shortcut,
			"name":        b.Name,
			"url":         b.URL,
			"category":    b.Category,
			"description": b.Description,
			"aliases":     b.Aliases,
		})
	}

	return map[string]interface{}{
		"bangs": bangMaps,
		"total": len(bangMaps),
		"categories": []string{
			"general", "images", "video", "maps", "news",
			"knowledge", "social", "code", "shopping", "files",
			"music", "science", "translate", "privacy", "misc",
		},
	}, nil
}

func (g *GraphQLHandler) resolveWidgets(p graphql.ResolveParams) (interface{}, error) {
	if g.handler.widgetManager == nil {
		return []map[string]interface{}{}, nil
	}

	widgets := g.handler.widgetManager.GetAllWidgets()
	widgetList := make([]map[string]interface{}, 0, len(widgets))

	for _, w := range widgets {
		widgetList = append(widgetList, map[string]interface{}{
			"type":        string(w.Type),
			"name":        w.Name,
			"description": w.Description,
			"icon":        w.Icon,
			"category":    string(w.Category),
		})
	}

	return widgetList, nil
}

func (g *GraphQLHandler) resolveInstant(p graphql.ResolveParams) (interface{}, error) {
	query := p.Args["query"].(string)

	if g.handler.instantManager == nil {
		return map[string]interface{}{
			"query": query,
			"found": false,
		}, nil
	}

	ctx := p.Context
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	answer, err := g.handler.instantManager.Process(ctx, query)
	if err != nil {
		return nil, err
	}

	if answer == nil {
		return map[string]interface{}{
			"query": query,
			"found": false,
		}, nil
	}

	return map[string]interface{}{
		"query":   query,
		"type":    string(answer.Type),
		"title":   answer.Title,
		"content": answer.Content,
		"source":  answer.Source,
		"found":   true,
	}, nil
}

// ServeHTTP handles GraphQL HTTP requests
func (g *GraphQLHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse request
	var params struct {
		Query         string                 `json:"query"`
		OperationName string                 `json:"operationName"`
		Variables     map[string]interface{} `json:"variables"`
	}

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
	} else if r.Method == http.MethodGet {
		params.Query = r.URL.Query().Get("query")
		params.OperationName = r.URL.Query().Get("operationName")
		if varsStr := r.URL.Query().Get("variables"); varsStr != "" {
			json.Unmarshal([]byte(varsStr), &params.Variables)
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Execute query
	result := graphql.Do(graphql.Params{
		Schema:         g.schema,
		RequestString:  params.Query,
		OperationName:  params.OperationName,
		VariableValues: params.Variables,
		Context:        r.Context(),
	})

	// Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GraphiQL HTML template with Dracula theme
const graphiqlHTML = `<!DOCTYPE html>
<html>
<head>
  <title>GraphiQL - Search API</title>
  <link href="https://unpkg.com/graphiql@3/graphiql.min.css" rel="stylesheet" />
  <style>
    /* Dracula Theme for GraphiQL */
    body {
      margin: 0;
      overflow: hidden;
    }
    #graphiql {
      height: 100vh;
    }
    /* Dracula colors */
    .graphiql-container {
      --color-base: #282a36;
      --color-primary: #bd93f9;
      --color-secondary: #6272a4;
      --color-tertiary: #44475a;
      --color-info: #8be9fd;
      --color-success: #50fa7b;
      --color-warning: #f1fa8c;
      --color-error: #ff5555;
      --color-neutral: #f8f8f2;
      --font-family: 'Fira Code', 'Source Code Pro', monospace;
      --font-family-mono: 'Fira Code', 'Source Code Pro', monospace;
    }
    .graphiql-container,
    .graphiql-container .graphiql-editors,
    .graphiql-container .graphiql-response,
    .graphiql-container .graphiql-editor-tools,
    .graphiql-container .CodeMirror {
      background: #282a36 !important;
      color: #f8f8f2 !important;
    }
    .graphiql-container .graphiql-sidebar {
      background: #21222c !important;
    }
    .graphiql-container .graphiql-sessions {
      background: #282a36 !important;
    }
    .graphiql-container .graphiql-session-header {
      background: #44475a !important;
    }
    .graphiql-container button {
      background: #44475a !important;
      color: #f8f8f2 !important;
      border-color: #6272a4 !important;
    }
    .graphiql-container button:hover {
      background: #6272a4 !important;
    }
    .graphiql-container .execute-button {
      background: #bd93f9 !important;
      border-color: #bd93f9 !important;
    }
    .graphiql-container .execute-button:hover {
      background: #ff79c6 !important;
      border-color: #ff79c6 !important;
    }
    .CodeMirror-gutters {
      background: #282a36 !important;
      border-color: #44475a !important;
    }
    .CodeMirror-linenumber {
      color: #6272a4 !important;
    }
    .cm-keyword { color: #ff79c6 !important; }
    .cm-def { color: #50fa7b !important; }
    .cm-variable { color: #f8f8f2 !important; }
    .cm-string { color: #f1fa8c !important; }
    .cm-number { color: #bd93f9 !important; }
    .cm-atom { color: #bd93f9 !important; }
    .cm-property { color: #66d9ef !important; }
    .cm-punctuation { color: #f8f8f2 !important; }
    .cm-ws { color: #6272a4 !important; }
    .cm-invalidchar { color: #ff5555 !important; }
  </style>
</head>
<body>
  <div id="graphiql"></div>
  <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/graphiql@3/graphiql.min.js"></script>
  <script>
    const root = ReactDOM.createRoot(document.getElementById('graphiql'));
    root.render(
      React.createElement(GraphiQL, {
        fetcher: GraphiQL.createFetcher({
          url: '{{.Endpoint}}',
        }),
        defaultEditorToolsVisibility: true,
      })
    );
  </script>
</body>
</html>`

// ServeGraphiQL serves the GraphiQL UI
func (g *GraphQLHandler) ServeGraphiQL(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("graphiql").Parse(graphiqlHTML)
	if err != nil {
		http.Error(w, "Template error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	tmpl.Execute(w, map[string]string{
		"Endpoint": "/graphql",
	})
}

// RegisterGraphQLRoutes registers GraphQL routes per AI.md spec
// /graphql GET  → GraphiQL interface
// /graphql POST → GraphQL queries
func (h *Handler) RegisterGraphQLRoutes(mux *http.ServeMux) error {
	gqlHandler, err := NewGraphQLHandler(h)
	if err != nil {
		return err
	}

	// Combined /graphql endpoint: GET serves GraphiQL, POST handles queries
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gqlHandler.ServeGraphiQL(w, r)
			return
		}
		gqlHandler.ServeHTTP(w, r)
	})

	return nil
}
