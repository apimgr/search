package graphql

import (
	"strconv"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"
)

// Schema represents the GraphQL schema
var Schema graphql.Schema

var jsonScalar = graphql.NewScalar(graphql.ScalarConfig{
	Name:         "JSON",
	Description:  "Arbitrary JSON object returned by the REST API",
	Serialize:    func(value interface{}) interface{} { return value },
	ParseValue:   func(value interface{}) interface{} { return value },
	ParseLiteral: parseJSONLiteral,
})

func parseJSONLiteral(value ast.Value) interface{} {
	switch value := value.(type) {
	case *ast.ObjectValue:
		object := make(map[string]interface{}, len(value.Fields))
		for _, field := range value.Fields {
			object[field.Name.Value] = parseJSONLiteral(field.Value)
		}
		return object
	case *ast.ListValue:
		list := make([]interface{}, len(value.Values))
		for index, item := range value.Values {
			list[index] = parseJSONLiteral(item)
		}
		return list
	case *ast.StringValue:
		return value.Value
	case *ast.IntValue:
		parsed, err := strconv.ParseInt(value.Value, 10, 64)
		if err != nil {
			return nil
		}
		return parsed
	case *ast.FloatValue:
		parsed, err := strconv.ParseFloat(value.Value, 64)
		if err != nil {
			return nil
		}
		return parsed
	case *ast.BooleanValue:
		return value.Value
	case *ast.EnumValue:
		if value.Value == "null" {
			return nil
		}
		return value.Value
	default:
		return nil
	}
}

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
			"imageWidth":  &graphql.Field{Type: graphql.Int, Description: "Full image width in pixels"},
			"imageHeight": &graphql.Field{Type: graphql.Int, Description: "Full image height in pixels"},
			"imageFormat": &graphql.Field{Type: graphql.String, Description: "Image format when known"},
			"fileSize":    &graphql.Field{Type: graphql.Int, Description: "Image file size in bytes when known"},
			"duration":    &graphql.Field{Type: graphql.Int, Description: "Video duration in seconds when known"},
			"viewCount":   &graphql.Field{Type: graphql.Float, Description: "Video view count when known"},
			"published":   &graphql.Field{Type: graphql.String, Description: "Publication date in RFC3339 format"},
			"author":      &graphql.Field{Type: graphql.String, Description: "Result author or video channel"},
			"embedUrl":    &graphql.Field{Type: graphql.String, Description: "Provider embed URL for video results when available"},
			"domain":      &graphql.Field{Type: graphql.String, Description: "Result domain"},
			"category":    &graphql.Field{Type: graphql.String, Description: "Result category"},
		},
	})

	// Define the SearchResponse type
	searchResponseType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "SearchResponse",
		Description: "Search query response",
		Fields: graphql.Fields{
			"query":       &graphql.Field{Type: graphql.String, Description: "The search query"},
			"category":    &graphql.Field{Type: graphql.String, Description: "Resolved search category"},
			"page":        &graphql.Field{Type: graphql.Int, Description: "Current result page"},
			"perPage":     &graphql.Field{Type: graphql.Int, Description: "Results per page"},
			"totalPages":  &graphql.Field{Type: graphql.Int, Description: "Number of result pages"},
			"suggestions": &graphql.Field{Type: graphql.NewList(graphql.String), Description: "Related search suggestions"},
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

	// Define the canonical health response shared with the REST API.
	projectInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "HealthProjectInfo",
		Fields: graphql.Fields{
			"name": &graphql.Field{Type: graphql.String}, "tagline": &graphql.Field{Type: graphql.String},
			"description": &graphql.Field{Type: graphql.String},
		},
	})
	buildInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name:   "HealthBuildInfo",
		Fields: graphql.Fields{"commit": &graphql.Field{Type: graphql.String}, "date": &graphql.Field{Type: graphql.String}},
	})
	torInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "HealthTorInfo",
		Fields: graphql.Fields{
			"enabled": &graphql.Field{Type: graphql.Boolean}, "running": &graphql.Field{Type: graphql.Boolean},
			"status": &graphql.Field{Type: graphql.String}, "hostname": &graphql.Field{Type: graphql.String},
		},
	})
	i2pInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "HealthI2PInfo",
		Fields: graphql.Fields{
			"enabled": &graphql.Field{Type: graphql.Boolean}, "running": &graphql.Field{Type: graphql.Boolean},
			"status": &graphql.Field{Type: graphql.String}, "hostname": &graphql.Field{Type: graphql.String},
			"provider": &graphql.Field{Type: graphql.String},
		},
	})
	featuresInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "HealthFeaturesInfo",
		Fields: graphql.Fields{
			"tor": &graphql.Field{Type: torInfoType}, "i2p": &graphql.Field{Type: i2pInfoType},
			"geoip": &graphql.Field{Type: graphql.Boolean},
		},
	})
	checksInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "HealthChecksInfo",
		Fields: graphql.Fields{
			"database": &graphql.Field{Type: graphql.String}, "cache": &graphql.Field{Type: graphql.String},
			"disk": &graphql.Field{Type: graphql.String}, "scheduler": &graphql.Field{Type: graphql.String},
			"tor": &graphql.Field{Type: graphql.String}, "i2p": &graphql.Field{Type: graphql.String},
		},
	})
	statsInfoType := graphql.NewObject(graphql.ObjectConfig{
		Name: "HealthStatsInfo",
		Fields: graphql.Fields{
			"requestsTotal": &graphql.Field{Type: graphql.Int}, "requests24h": &graphql.Field{Type: graphql.Int},
			"activeConns": &graphql.Field{Type: graphql.Int},
		},
	})
	healthResponseType := graphql.NewObject(graphql.ObjectConfig{
		Name:        "HealthResponse",
		Description: "Public server health response per AI.md PART 13",
		Fields: graphql.Fields{
			"project":        &graphql.Field{Type: projectInfoType, Description: "Public project identification"},
			"status":         &graphql.Field{Type: graphql.String, Description: "healthy, degraded, restart_required, unhealthy, maintenance, or shutting_down"},
			"pendingRestart": &graphql.Field{Type: graphql.Boolean},
			"restartReason":  &graphql.Field{Type: graphql.NewList(graphql.String)},
			"version":        &graphql.Field{Type: graphql.String},
			"goVersion":      &graphql.Field{Type: graphql.String},
			"build":          &graphql.Field{Type: buildInfoType},
			"uptime":         &graphql.Field{Type: graphql.String},
			"mode":           &graphql.Field{Type: graphql.String},
			"timestamp":      &graphql.Field{Type: graphql.String},
			"features":       &graphql.Field{Type: featuresInfoType},
			"checks":         &graphql.Field{Type: checksInfoType},
			"stats":          &graphql.Field{Type: statsInfoType},
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
			"found":   &graphql.Field{Type: graphql.Boolean, Description: "Whether the answer was found"},
			"details": &graphql.Field{Type: jsonScalar, Description: "Type-specific answer fields matching the REST API data object"},
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
			"found":   &graphql.Field{Type: graphql.Boolean, Description: "Whether an instant answer was found"},
			"details": &graphql.Field{Type: jsonScalar, Description: "Type-specific answer fields matching the REST API data object"},
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

	engineType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchEngine",
		Fields: graphql.Fields{
			"id":         &graphql.Field{Type: graphql.String},
			"name":       &graphql.Field{Type: graphql.String},
			"enabled":    &graphql.Field{Type: graphql.Boolean},
			"priority":   &graphql.Field{Type: graphql.Int},
			"categories": &graphql.Field{Type: graphql.NewList(graphql.String)},
			"health":     &graphql.Field{Type: jsonScalar},
		},
	})
	widgetType := graphql.NewObject(graphql.ObjectConfig{
		Name: "Widget",
		Fields: graphql.Fields{
			"type": &graphql.Field{Type: graphql.String}, "name": &graphql.Field{Type: graphql.String},
			"description": &graphql.Field{Type: graphql.String}, "icon": &graphql.Field{Type: graphql.String},
			"category": &graphql.Field{Type: graphql.String}, "order": &graphql.Field{Type: graphql.Int},
		},
	})
	widgetCatalogType := graphql.NewObject(graphql.ObjectConfig{
		Name: "WidgetCatalog",
		Fields: graphql.Fields{
			"enabled":  &graphql.Field{Type: graphql.Boolean},
			"widgets":  &graphql.Field{Type: graphql.NewList(widgetType)},
			"defaults": &graphql.Field{Type: graphql.NewList(graphql.String)},
		},
	})
	widgetDataType := graphql.NewObject(graphql.ObjectConfig{
		Name: "WidgetData",
		Fields: graphql.Fields{
			"type": &graphql.Field{Type: graphql.String}, "data": &graphql.Field{Type: jsonScalar},
			"updatedAt": &graphql.Field{Type: graphql.String}, "error": &graphql.Field{Type: graphql.String},
		},
	})
	categoryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "SearchCategory",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.String},
			"name":        &graphql.Field{Type: graphql.String},
			"description": &graphql.Field{Type: graphql.String},
			"icon":        &graphql.Field{Type: graphql.String},
		},
	})
	bangType := graphql.NewObject(graphql.ObjectConfig{
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

	// Define the root query
	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"engines": &graphql.Field{Type: graphql.NewList(engineType), Resolve: resolveEngines},
			"engine": &graphql.Field{
				Type:    engineType,
				Args:    graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}},
				Resolve: resolveEngine,
			},
			"categories": &graphql.Field{Type: graphql.NewList(categoryType), Resolve: resolveCategories},
			"widgets": &graphql.Field{
				Type:    widgetCatalogType,
				Args:    graphql.FieldConfigArgument{"category": &graphql.ArgumentConfig{Type: graphql.String}},
				Resolve: resolveWidgets,
			},
			"widgetData": &graphql.Field{
				Type: widgetDataType,
				Args: graphql.FieldConfigArgument{
					"type":   &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"params": &graphql.ArgumentConfig{Type: jsonScalar},
				},
				Resolve: resolveWidgetData,
			},
			"bangs": &graphql.Field{
				Type: graphql.NewList(bangType),
				Args: graphql.FieldConfigArgument{
					"category": &graphql.ArgumentConfig{Type: graphql.String},
					"search":   &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: resolveBangs,
			},
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
					"limit":      &graphql.ArgumentConfig{Type: graphql.Int, Description: "Results per page (1-100)"},
					"safeSearch": &graphql.ArgumentConfig{Type: graphql.Int, Description: "Safe search level (0 off, 1 moderate, 2 strict)"},
					"timeRange":  &graphql.ArgumentConfig{Type: graphql.String, Description: "Time range filter (any, hour, day, week, month, year, custom)"},
					"timeFrom":   &graphql.ArgumentConfig{Type: graphql.String, Description: "Start date for custom range (YYYY-MM-DD)"},
					"timeTo":     &graphql.ArgumentConfig{Type: graphql.String, Description: "End date for custom range (YYYY-MM-DD)"},
					"region":     &graphql.ArgumentConfig{Type: graphql.String, Description: "Two-letter region code"},
					"engines":    &graphql.ArgumentConfig{Type: graphql.NewList(graphql.String), Description: "Limit search to the named engines"},
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
			"relatedSearches": &graphql.Field{
				Type:        graphql.NewList(graphql.String),
				Description: "Get related search suggestions",
				Args: graphql.FieldConfigArgument{
					"q":     &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String), Description: "Search query"},
					"limit": &graphql.ArgumentConfig{Type: graphql.Int, Description: "Maximum suggestions (1-20; default 8)"},
				},
				Resolve: resolveRelatedSearches,
			},
			"serverStatus":      &graphql.Field{Type: jsonScalar, Description: "Operator-only status matching GET /api/v1/server/status", Resolve: resolveOperatorStatus},
			"serverConfig":      &graphql.Field{Type: jsonScalar, Description: "Operator-only redacted config matching GET /api/v1/server/config", Resolve: resolveOperatorConfig},
			"preferences":       &graphql.Field{Type: jsonScalar, Description: "Client-side preference schema matching GET /api/v1/server/preferences", Resolve: resolvePreferences},
			"ownerTokens":       &graphql.Field{Type: jsonScalar, Description: "Primary-token-only owner token inventory matching GET /api/v1/server/preferences/tokens", Resolve: resolveOwnerTokens},
			"preferencesExport": &graphql.Field{Type: jsonScalar, Description: "Export current preferences using the REST export handler", Resolve: resolveExportPreferences},
			"geoIPLookup":       &graphql.Field{Type: jsonScalar, Description: "Look up an IP using the public REST GeoIP service; omit ip to look up the caller", Args: graphql.FieldConfigArgument{"ip": &graphql.ArgumentConfig{Type: graphql.String}}, Resolve: resolveGeoIPLookup},
			"geoIPPresets":      &graphql.Field{Type: graphql.NewList(jsonScalar), Description: "Operator-only saved GeoIP country-list presets", Resolve: resolveGeoIPPresets},
			"geoIPPreset":       &graphql.Field{Type: jsonScalar, Description: "Operator-only saved GeoIP country-list preset", Args: graphql.FieldConfigArgument{"name": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveGeoIPPreset},
			"serverInfo":        &graphql.Field{Type: jsonScalar, Description: "Public runtime information matching GET /api/v1/info", Resolve: resolveServerInfo},
			"autodiscover":      &graphql.Field{Type: jsonScalar, Description: "Versionless service discovery matching GET /api/autodiscover", Resolve: resolveAutodiscover},
			"serverPage":        &graphql.Field{Type: jsonScalar, Description: "About, privacy, help, or terms page data from the matching REST handler", Args: graphql.FieldConfigArgument{"name": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveServerPage},
			"contactInfo":       &graphql.Field{Type: jsonScalar, Description: "Public contact form availability matching GET /api/v1/server/contact", Resolve: resolveContactInfo},
			"favicon":           &graphql.Field{Type: jsonScalar, Description: "Privacy-preserving proxied favicon as base64", Args: graphql.FieldConfigArgument{"url": &graphql.ArgumentConfig{Type: graphql.String}}, Resolve: resolveFavicon},
			"alerts":            &graphql.Field{Type: graphql.NewList(jsonScalar), Description: "Operator-only alert inventory matching GET /api/v1/alerts", Resolve: resolveAlerts},
			"alert":             &graphql.Field{Type: jsonScalar, Description: "Read an alert using its private manage token", Args: graphql.FieldConfigArgument{"manageToken": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveAlert},
			"alertFeed":         &graphql.Field{Type: jsonScalar, Description: "Read a private alert feed using its RSS token", Args: graphql.FieldConfigArgument{"rssToken": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveAlertFeed},
			"schedulerTasks":    &graphql.Field{Type: graphql.NewList(jsonScalar), Description: "Operator-only scheduler task inventory", Resolve: resolveSchedulerTasks},
			"schedulerTask":     &graphql.Field{Type: jsonScalar, Description: "Operator-only scheduler task detail", Args: graphql.FieldConfigArgument{"task": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveSchedulerTask},
			"schedulerHistory":  &graphql.Field{Type: graphql.NewList(jsonScalar), Description: "Operator-only recent task history", Args: graphql.FieldConfigArgument{"task": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveSchedulerHistory},
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

	rootMutation := graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"apiRequest":           &graphql.Field{Type: jsonScalar, Description: "Execute an API REST request through the same routes, authentication, and validation as HTTP", Args: graphql.FieldConfigArgument{"method": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "path": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "input": &graphql.ArgumentConfig{Type: jsonScalar}}, Resolve: resolveAPIRequest},
			"saveContactWebhook":   &graphql.Field{Type: jsonScalar, Description: "Save an operator contact webhook; a new signing secret is returned once", Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)}}, Resolve: resolveSaveContactWebhook},
			"savePreferences":      &graphql.Field{Type: jsonScalar, Description: "Acknowledge client-side preference saves, matching POST /api/v1/server/preferences", Resolve: resolveSavePreferences},
			"importPreferences":    &graphql.Field{Type: jsonScalar, Description: "Import preferences using the REST handler and apply response cookies", Args: graphql.FieldConfigArgument{"theme": &graphql.ArgumentConfig{Type: graphql.String}, "lang": &graphql.ArgumentConfig{Type: graphql.String}, "code": &graphql.ArgumentConfig{Type: graphql.String}}, Resolve: resolveImportPreferences},
			"submitContact":        &graphql.Field{Type: jsonScalar, Description: "Submit a public contact message through the REST handler", Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)}}, Resolve: resolveSubmitContact},
			"createAlert":          &graphql.Field{Type: jsonScalar, Description: "Create an accountless alert subscription", Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)}}, Resolve: resolveCreateAlert},
			"updateAlert":          &graphql.Field{Type: jsonScalar, Description: "Update an alert using its private manage token", Args: graphql.FieldConfigArgument{"manageToken": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)}}, Resolve: resolveUpdateAlert},
			"pauseAlert":           &graphql.Field{Type: jsonScalar, Description: "Pause or resume an alert using its private manage token", Args: graphql.FieldConfigArgument{"manageToken": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "paused": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.Boolean)}}, Resolve: resolvePauseAlert},
			"deleteAlert":          &graphql.Field{Type: jsonScalar, Description: "Delete an alert using its private manage token", Args: graphql.FieldConfigArgument{"manageToken": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveDeleteAlert},
			"runSchedulerTask":     &graphql.Field{Type: jsonScalar, Description: "Operator-only manual scheduler run; writes the standard audit event", Args: graphql.FieldConfigArgument{"task": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveRunSchedulerTask},
			"enableSchedulerTask":  &graphql.Field{Type: jsonScalar, Description: "Operator-only scheduler task enable operation", Args: graphql.FieldConfigArgument{"task": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveEnableSchedulerTask},
			"disableSchedulerTask": &graphql.Field{Type: jsonScalar, Description: "Operator-only scheduler task disable operation", Args: graphql.FieldConfigArgument{"task": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveDisableSchedulerTask},
			"saveGeoIPPreset":      &graphql.Field{Type: jsonScalar, Description: "Save a named operator-authored country list without applying it", Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)}}, Resolve: resolveSaveGeoIPPreset},
			"deleteGeoIPPreset":    &graphql.Field{Type: jsonScalar, Description: "Delete a saved GeoIP country-list preset", Args: graphql.FieldConfigArgument{"name": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveDeleteGeoIPPreset},
			"revealPrimaryToken":   &graphql.Field{Type: jsonScalar, Description: "Reveal the primary token; requires the primary bearer token", Resolve: resolveRevealPrimaryToken},
			"rotatePrimaryToken":   &graphql.Field{Type: jsonScalar, Description: "Rotate the primary token; requires the primary bearer token", Resolve: resolveRotatePrimaryToken},
			"createOwnerToken":     &graphql.Field{Type: jsonScalar, Description: "Create a linked token; requires the primary bearer token", Args: graphql.FieldConfigArgument{"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)}}, Resolve: resolveCreateOwnerToken},
			"revealOwnerToken":     &graphql.Field{Type: jsonScalar, Description: "Reveal a linked token; requires the primary bearer token", Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveRevealOwnerToken},
			"rotateOwnerToken":     &graphql.Field{Type: jsonScalar, Description: "Rotate a linked token; requires the primary bearer token", Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveRotateOwnerToken},
			"updateOwnerToken":     &graphql.Field{Type: jsonScalar, Description: "Update a linked token; requires the primary bearer token", Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}, "input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(jsonScalar)}}, Resolve: resolveUpdateOwnerToken},
			"revokeOwnerToken":     &graphql.Field{Type: jsonScalar, Description: "Revoke a linked token; requires the primary bearer token", Args: graphql.FieldConfigArgument{"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)}}, Resolve: resolveRevokeOwnerToken},
		},
	})

	// Create schema
	var err error
	Schema, err = graphql.NewSchema(graphql.SchemaConfig{
		Query:    rootQuery,
		Mutation: rootMutation,
	})

	return err
}
