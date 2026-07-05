package graphql

import (
	"github.com/graphql-go/graphql"
)

// Schema represents the GraphQL schema
var Schema graphql.Schema

// initSchemaFunc is used for testing - allows injecting errors
// By default, this is set to initSchemaImpl
var initSchemaFunc = initSchemaImpl

// InitSchema initializes the GraphQL schema
func InitSchema() error {
	return initSchemaFunc()
}

// initSchemaImpl is the actual schema initialization implementation
// Per AI.md PART 19: GraphQL must be in sync with REST API
func initSchemaImpl() error {
	// Define the SearchResult type
	searchResultType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "SearchResult",
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
		Name:        "SearchResponse",
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
		Name:        "HealthResponse",
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

	// Define the DirectAnswerData type
	directAnswerDataType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "DirectAnswerData",
		Description: "Direct answer data payload",
		Fields: graphql.Fields{
			"type": &graphql.Field{
				Type:        graphql.String,
				Description: "Answer type (tldr, man, dns, whois, wiki, http, port, chmod, cron, etc.)",
			},
			"term": &graphql.Field{
				Type:        graphql.String,
				Description: "Term that was looked up",
			},
			"title": &graphql.Field{
				Type:        graphql.String,
				Description: "Title of the answer",
			},
			"description": &graphql.Field{
				Type:        graphql.String,
				Description: "Brief description",
			},
			"content": &graphql.Field{
				Type:        graphql.String,
				Description: "Main content (HTML or text)",
			},
			"source": &graphql.Field{
				Type:        graphql.String,
				Description: "Source of the information",
			},
			"sourceUrl": &graphql.Field{
				Type:        graphql.String,
				Description: "URL to the source",
			},
			"cacheTtlSeconds": &graphql.Field{
				Type:        graphql.Int,
				Description: "Cache TTL in seconds",
			},
			"found": &graphql.Field{
				Type:        graphql.Boolean,
				Description: "Whether the answer was found",
			},
		},
	})

	// Define the DirectAnswer type
	directAnswerType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "DirectAnswer",
		Description: "Direct answer response",
		Fields: graphql.Fields{
			"ok": &graphql.Field{
				Type:        graphql.Boolean,
				Description: "Whether the request was successful",
			},
			"data": &graphql.Field{
				Type:        directAnswerDataType,
				Description: "Answer data",
			},
		},
	})

	// Define the InstantAnswerData type
	instantAnswerDataType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "InstantAnswerData",
		Description: "Instant answer data payload",
		Fields: graphql.Fields{
			"query": &graphql.Field{
				Type:        graphql.String,
				Description: "Original query",
			},
			"type": &graphql.Field{
				Type:        graphql.String,
				Description: "Answer type (calculator, converter, dictionary, etc.)",
			},
			"title": &graphql.Field{
				Type:        graphql.String,
				Description: "Title of the answer",
			},
			"content": &graphql.Field{
				Type:        graphql.String,
				Description: "Main content",
			},
			"source": &graphql.Field{
				Type:        graphql.String,
				Description: "Source of the information",
			},
			"found": &graphql.Field{
				Type:        graphql.Boolean,
				Description: "Whether an instant answer was found",
			},
		},
	})

	// Define the InstantAnswer type
	instantAnswerType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "InstantAnswer",
		Description: "Instant answer response",
		Fields: graphql.Fields{
			"ok": &graphql.Field{
				Type:        graphql.Boolean,
				Description: "Whether the request was successful",
			},
			"data": &graphql.Field{
				Type:        instantAnswerDataType,
				Description: "Answer data",
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
			"directAnswer": &graphql.Field{
				Type:        directAnswerType,
				Description: "Get a direct answer for a specific type and term (e.g., tldr:git, dns:example.com, http:404)",
				Args: graphql.FieldConfigArgument{
					"type": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Answer type (tldr, man, dns, whois, wiki, http, port, chmod, cron, etc.)",
					},
					"term": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Term to look up",
					},
				},
				Resolve: resolveDirectAnswer,
			},
			"instant": &graphql.Field{
				Type:        instantAnswerType,
				Description: "Get an instant answer widget for a query (calculator, converter, dictionary, etc.)",
				Args: graphql.FieldConfigArgument{
					"q": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Query to process for instant answer",
					},
				},
				Resolve: resolveInstant,
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
