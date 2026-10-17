package swagger

import (
	"github.com/apimgr/search/src/version"
)

// This file contains the OpenAPI/Swagger annotation definitions:
// endpoint paths and reusable component schemas generated from the API.

// generatePaths generates all API endpoint definitions
func generatePaths() map[string]PathItem {
	paths := make(map[string]PathItem)

	// Health check - canonical route is /server/healthz per AI.md PART 13
	paths["/server/healthz"] = PathItem{
		Get: &Operation{
			Summary:     "Health check",
			Description: "Returns server health status. Canonical route: /server/healthz",
			Tags:        []string{"System"},
			Responses: map[string]Response{
				"200": {
					Description: "Server is healthy",
					Content: map[string]MediaType{
						"application/json": {
							Schema: healthResponseSchema(),
						},
					},
				},
				"503": {
					Description: "Server is unhealthy, in maintenance, or shutting down",
					Content: map[string]MediaType{
						"application/json": {Schema: healthResponseSchema()},
						"text/plain":       {Schema: &Schema{Type: "string"}},
					},
				},
			},
		},
	}

	// API v1 health check - canonical route is /api/v1/server/healthz per AI.md PART 13/14
	paths[version.APIPrefix+"/server/healthz"] = paths["/server/healthz"]
	paths[version.APIPrefix+"/server/healthz.txt"] = paths[version.APIPrefix+"/server/healthz"]
	paths["/api/healthz"] = paths[version.APIPrefix+"/server/healthz"]

	// Search endpoint
	paths[version.APIPrefix+"/search"] = PathItem{
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
				{Name: "limit", In: "query", Description: "Results per page (default: 20, maximum: 100)", Schema: &Schema{Type: "integer"}},
				{Name: "safe_search", In: "query", Description: "Safe search level: 0 off, 1 moderate, 2 strict", Schema: &Schema{Type: "integer"}},
				{Name: "time_range", In: "query", Description: "Time range: any, hour, day, week, month, year, or custom", Schema: &Schema{Type: "string"}},
				{Name: "time_from", In: "query", Description: "Start date for custom range (YYYY-MM-DD)", Schema: &Schema{Type: "string"}},
				{Name: "time_to", In: "query", Description: "End date for custom range (YYYY-MM-DD)", Schema: &Schema{Type: "string"}},
				{Name: "region", In: "query", Description: "ISO country or region code", Schema: &Schema{Type: "string"}},
				{Name: "engines", In: "query", Description: "Comma-separated or repeated engine names", Schema: &Schema{Type: "array", Items: &Schema{Type: "string"}}},
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
									"query": {Type: "string"},
									"results": {Type: "array", Items: &Schema{Type: "object", Properties: map[string]Schema{
										"title": {Type: "string"}, "url": {Type: "string"}, "description": {Type: "string"},
										"engine": {Type: "string"}, "score": {Type: "number"}, "category": {Type: "string"},
										"thumbnail": {Type: "string"}, "image_url": {Type: "string"},
										"image_width": {Type: "integer"}, "image_height": {Type: "integer"},
										"image_format": {Type: "string"}, "file_size": {Type: "integer"},
										"duration": {Type: "integer"}, "view_count": {Type: "integer"},
										"author": {Type: "string"}, "embed_url": {Type: "string"},
									}}},
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
		Post: &Operation{
			Summary:     "Search with a JSON request body",
			Description: "Perform a metasearch query without placing search terms in the request URL",
			Tags:        []string{"Search"},
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{"application/json": {Schema: &Schema{
					Type: "object",
					Properties: map[string]Schema{
						"query":       {Type: "string", Example: "golang"},
						"category":    {Type: "string"},
						"page":        {Type: "integer"},
						"limit":       {Type: "integer"},
						"language":    {Type: "string"},
						"safe_search": {Type: "integer"},
						"time_range":  {Type: "string"},
						"time_from":   {Type: "string"},
						"time_to":     {Type: "string"},
						"region":      {Type: "string"},
						"engines":     {Type: "array", Items: &Schema{Type: "string"}},
					},
				}}},
			},
			Responses: map[string]Response{
				"200": okResponse("Search results", &Schema{Type: "object"}),
				"400": errorResponse("Invalid search request"),
			},
		},
	}

	// Autocomplete endpoint
	paths[version.APIPrefix+"/autocomplete"] = PathItem{
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

	// Direct answer endpoint
	paths[version.APIPrefix+"/direct/{type}/{term}"] = PathItem{
		Get: &Operation{
			Summary:     "Direct answer lookup",
			Description: "Get a direct answer for a specific type and term (e.g., tldr:git, dns:example.com, http:404)",
			Tags:        []string{"Direct Answers"},
			Parameters: []Parameter{
				{
					Name:        "type",
					In:          "path",
					Description: "Answer type (tldr, man, dns, whois, wiki, http, port, chmod, cron, etc.)",
					Required:    true,
					Schema:      &Schema{Type: "string"},
				},
				{
					Name:        "term",
					In:          "path",
					Description: "Term to look up",
					Required:    true,
					Schema:      &Schema{Type: "string"},
				},
			},
			Responses: map[string]Response{
				"200": {
					Description: "Direct answer result",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type: "object",
								Properties: map[string]Schema{
									"ok": {Type: "boolean"},
									"data": {
										Type: "object",
										Properties: map[string]Schema{
											"type":              {Type: "string"},
											"term":              {Type: "string"},
											"title":             {Type: "string"},
											"description":       {Type: "string"},
											"content":           {Type: "string"},
											"source":            {Type: "string"},
											"source_url":        {Type: "string"},
											"cache_ttl_seconds": {Type: "integer"},
											"found":             {Type: "boolean"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Instant answer endpoint
	paths[version.APIPrefix+"/instant"] = PathItem{
		Get: &Operation{
			Summary:     "Instant answer",
			Description: "Get an instant answer widget for a query (calculator, converter, dictionary, etc.)",
			Tags:        []string{"Instant Answers"},
			Parameters: []Parameter{
				{
					Name:        "q",
					In:          "query",
					Description: "Query to process for instant answer",
					Required:    true,
					Schema:      &Schema{Type: "string"},
				},
			},
			Responses: map[string]Response{
				"200": {
					Description: "Instant answer result",
					Content: map[string]MediaType{
						"application/json": {
							Schema: &Schema{
								Type: "object",
								Properties: map[string]Schema{
									"ok": {Type: "boolean"},
									"data": {
										Type: "object",
										Properties: map[string]Schema{
											"query":   {Type: "string"},
											"type":    {Type: "string"},
											"title":   {Type: "string"},
											"content": {Type: "string"},
											"source":  {Type: "string"},
											"found":   {Type: "boolean"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	paths[version.APIPrefix+"/alerts"] = PathItem{
		Get: &Operation{
			Summary:     "List search alerts",
			Description: "Return the alert inventory to an operator",
			Tags:        []string{"Alerts", "Operator"},
			Security:    operatorSecurity(),
			Responses: map[string]Response{
				"200": okResponse("Alert inventory", &Schema{Type: "object"}),
				"401": errorResponse("Missing or invalid operator token"),
			},
		},
		Post: &Operation{
			Summary:     "Create search alert",
			Description: "Create an accountless search alert with email, private RSS, and/or webhook delivery",
			Tags:        []string{"Alerts"},
			RequestBody: &RequestBody{
				Description: "Alert creation payload",
				Required:    true,
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{
							Type: "object",
							Properties: map[string]Schema{
								"query":           {Type: "string"},
								"category":        {Type: "string"},
								"language":        {Type: "string"},
								"region":          {Type: "string"},
								"engines":         {Type: "array", Items: &Schema{Type: "string"}},
								"safe_search":     {Type: "integer"},
								"frequency":       {Type: "string"},
								"email":           {Type: "string"},
								"deliver_email":   {Type: "boolean"},
								"deliver_rss":     {Type: "boolean"},
								"deliver_webhook": {Type: "boolean"},
								"webhook_url":     {Type: "string"},
							},
						},
					},
				},
			},
			Responses: map[string]Response{
				"201": {Description: "Alert created"},
			},
		},
	}

	paths[version.APIPrefix+"/alerts/{token}"] = PathItem{
		Get: &Operation{
			Summary:     "Get alert details",
			Description: "Get alert details for a manage token",
			Tags:        []string{"Alerts"},
			Parameters: []Parameter{
				{Name: "token", In: "path", Description: "Alert manage token", Required: true, Schema: &Schema{Type: "string"}},
			},
			Responses: map[string]Response{
				"200": {Description: "Alert details"},
			},
		},
		Patch: &Operation{
			Summary:     "Update alert",
			Description: "Update an alert using its manage token",
			Tags:        []string{"Alerts"},
			Parameters: []Parameter{
				{Name: "token", In: "path", Description: "Alert manage token", Required: true, Schema: &Schema{Type: "string"}},
			},
			RequestBody: &RequestBody{
				Description: "Alert update payload",
				Required:    true,
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{
							Type: "object",
							Properties: map[string]Schema{
								"query":           {Type: "string"},
								"category":        {Type: "string"},
								"language":        {Type: "string"},
								"region":          {Type: "string"},
								"engines":         {Type: "array", Items: &Schema{Type: "string"}},
								"safe_search":     {Type: "integer"},
								"frequency":       {Type: "string"},
								"deliver_email":   {Type: "boolean"},
								"deliver_rss":     {Type: "boolean"},
								"deliver_webhook": {Type: "boolean"},
								"webhook_url":     {Type: "string"},
							},
						},
					},
				},
			},
			Responses: map[string]Response{
				"200": {Description: "Alert updated"},
			},
		},
		Delete: &Operation{
			Summary:     "Delete alert",
			Description: "Delete an alert using its manage token",
			Tags:        []string{"Alerts"},
			Parameters: []Parameter{
				{Name: "token", In: "path", Description: "Alert manage token", Required: true, Schema: &Schema{Type: "string"}},
			},
			Responses: map[string]Response{
				"200": {Description: "Alert deleted"},
			},
		},
	}

	paths[version.APIPrefix+"/alerts/{token}/pause"] = PathItem{
		Post: &Operation{
			Summary:     "Pause or resume alert",
			Description: "Pause or resume an alert using its manage token",
			Tags:        []string{"Alerts"},
			Parameters: []Parameter{
				{Name: "token", In: "path", Description: "Alert manage token", Required: true, Schema: &Schema{Type: "string"}},
			},
			RequestBody: &RequestBody{
				Description: "Pause state payload",
				Required:    false,
				Content: map[string]MediaType{
					"application/json": {
						Schema: &Schema{
							Type: "object",
							Properties: map[string]Schema{
								"paused": {Type: "boolean"},
							},
						},
					},
				},
			},
			Responses: map[string]Response{
				"200": {Description: "Alert pause state updated"},
			},
		},
	}

	paths[version.APIPrefix+"/alerts/{token}/rss"] = PathItem{
		Get: &Operation{
			Summary:     "Get alert RSS feed",
			Description: "Return the private RSS feed for an alert",
			Tags:        []string{"Alerts"},
			Parameters: []Parameter{
				{Name: "token", In: "path", Description: "Alert RSS token", Required: true, Schema: &Schema{Type: "string"}},
			},
			Responses: map[string]Response{
				"200": {Description: "RSS feed payload"},
			},
		},
	}

	// Autodiscover (non-versioned) per AI.md PART 32
	paths["/api/autodiscover"] = PathItem{
		Get: &Operation{
			Summary:     "Autodiscover server settings",
			Description: "Returns server settings, config schema, and options for the CLI client",
			Tags:        []string{"System"},
			Responses: map[string]Response{
				"200": okResponse("Server settings", &Schema{Type: "object"}),
			},
		},
	}

	paths[version.APIPrefix+"/info"] = PathItem{
		Get: &Operation{
			Summary:     "Server information",
			Description: "Returns public server information (name, version, capabilities)",
			Tags:        []string{"System"},
			Responses: map[string]Response{
				"200": okResponse("Server information", &Schema{Type: "object"}),
			},
		},
	}
	paths[version.APIPrefix+"/info.txt"] = paths[version.APIPrefix+"/info"]

	paths[version.APIPrefix+"/search/related"] = PathItem{
		Get: &Operation{
			Summary:     "Related searches",
			Description: "Return related search suggestions for a query",
			Tags:        []string{"Search"},
			Parameters: []Parameter{
				{Name: "q", In: "query", Description: "Search query", Required: true, Schema: &Schema{Type: "string", Example: "golang"}},
			},
			Responses: map[string]Response{
				"200": okResponse("Related searches", &Schema{Type: "array", Items: &Schema{Type: "string"}}),
				"400": errorResponse("Missing or invalid query"),
			},
		},
	}

	paths[version.APIPrefix+"/engines"] = PathItem{
		Get: &Operation{
			Summary:     "List search engines",
			Description: "Return every configured search engine",
			Tags:        []string{"Engines"},
			Parameters:  paginationParameters(),
			Responses: map[string]Response{
				"200": okResponse("Engine list", &Schema{Type: "array", Items: &Schema{Type: "object"}}),
			},
		},
	}

	paths[version.APIPrefix+"/engines/{engine_id}"] = PathItem{
		Get: &Operation{
			Summary:     "Get search engine",
			Description: "Return a single search engine by identifier",
			Tags:        []string{"Engines"},
			Parameters: []Parameter{
				{Name: "engine_id", In: "path", Description: "Engine identifier", Required: true, Schema: &Schema{Type: "string", Example: "duckduckgo"}},
			},
			Responses: map[string]Response{
				"200": okResponse("Engine", &Schema{Type: "object"}),
				"404": errorResponse("Engine not found"),
			},
		},
	}

	paths[version.APIPrefix+"/categories"] = PathItem{
		Get: &Operation{
			Summary:     "List categories",
			Description: "Return every search category",
			Tags:        []string{"Engines"},
			Responses: map[string]Response{
				"200": okResponse("Category list", &Schema{Type: "array", Items: &Schema{Type: "object"}}),
			},
		},
	}

	paths[version.APIPrefix+"/bangs"] = PathItem{
		Get: &Operation{
			Summary:     "List bang shortcuts",
			Description: "Return the available bang shortcuts",
			Tags:        []string{"Bangs"},
			Parameters:  paginationParameters(),
			Responses: map[string]Response{
				"200": okResponse("Bang list", &Schema{Type: "array", Items: &Schema{Type: "object"}}),
			},
		},
	}

	paths[version.APIPrefix+"/widgets"] = PathItem{
		Get: &Operation{
			Summary:     "List widgets",
			Description: "Return every available widget",
			Tags:        []string{"Widgets"},
			Responses: map[string]Response{
				"200": okResponse("Widget list", &Schema{Type: "array", Items: &Schema{Type: "object"}}),
			},
		},
	}

	paths[version.APIPrefix+"/widgets/{widget_id}"] = PathItem{
		Get: &Operation{
			Summary:     "Get widget data",
			Description: "Return the rendered data for a single widget",
			Tags:        []string{"Widgets"},
			Parameters: []Parameter{
				{Name: "widget_id", In: "path", Description: "Widget identifier", Required: true, Schema: &Schema{Type: "string", Example: "weather"}},
			},
			Responses: map[string]Response{
				"200": okResponse("Widget data", &Schema{Type: "object"}),
				"404": errorResponse("Widget not found"),
			},
		},
	}

	paths[version.APIPrefix+"/favicon"] = PathItem{
		Get: &Operation{
			Summary:     "Favicon proxy",
			Description: "Proxy a site favicon so the browser never contacts a third party",
			Tags:        []string{"System"},
			Parameters: []Parameter{
				{Name: "url", In: "query", Description: "Target site URL", Required: true, Schema: &Schema{Type: "string", Example: "https://example.com"}},
			},
			Responses: map[string]Response{
				"200": {Description: "Favicon image bytes"},
				"400": errorResponse("Missing or invalid url"),
			},
		},
	}

	for _, page := range []struct {
		name    string
		summary string
	}{
		{"about", "About page content"},
		{"privacy", "Privacy policy content"},
		{"help", "Help page content"},
		{"terms", "Terms of service content"},
	} {
		paths[version.APIPrefix+"/server/"+page.name] = PathItem{
			Get: &Operation{
				Summary:     page.summary,
				Description: page.summary + " as structured data",
				Tags:        []string{"Server"},
				Responses: map[string]Response{
					"200": okResponse(page.summary, &Schema{Type: "object"}),
				},
			},
		}
	}

	paths[version.APIPrefix+"/server/contact"] = PathItem{
		Get: &Operation{
			Summary:     "Contact information",
			Description: "Return the contact form definition and operator contact details",
			Tags:        []string{"Server"},
			Responses: map[string]Response{
				"200": okResponse("Contact information", &Schema{Type: "object"}),
			},
		},
		Post: &Operation{
			Summary:     "Submit contact message",
			Description: "Submit a message to the server operator",
			Tags:        []string{"Server"},
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{
						Type: "object",
						Properties: map[string]Schema{
							"name":    {Type: "string", Example: "Ada Lovelace"},
							"email":   {Type: "string", Example: "ada@example.com"},
							"subject": {Type: "string", Example: "Feedback"},
							"message": {Type: "string", Example: "Great service"},
						},
					}},
				},
			},
			Responses: map[string]Response{
				"200": okResponse("Message accepted", &Schema{Type: "object"}),
				"400": errorResponse("Validation failed"),
			},
		},
	}

	paths[version.APIPrefix+"/server/preferences"] = PathItem{
		Get: &Operation{
			Summary:     "Get preferences",
			Description: "Return the current client preferences",
			Tags:        []string{"Preferences"},
			Responses: map[string]Response{
				"200": okResponse("Preferences", &Schema{Type: "object"}),
			},
		},
		Post: &Operation{
			Summary:     "Update preferences",
			Description: "Replace the current client preferences",
			Tags:        []string{"Preferences"},
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{Type: "object", Example: map[string]string{"theme": "dark"}}},
				},
			},
			Responses: map[string]Response{
				"200": okResponse("Updated preferences", &Schema{Type: "object"}),
				"400": errorResponse("Validation failed"),
			},
		},
	}

	paths[version.APIPrefix+"/server/preferences/export"] = PathItem{
		Get: &Operation{
			Summary:     "Export preferences",
			Description: "Return the current preferences as a portable document",
			Tags:        []string{"Preferences"},
			Responses: map[string]Response{
				"200": okResponse("Exported preferences", &Schema{Type: "object"}),
			},
		},
	}

	paths[version.APIPrefix+"/server/preferences/import"] = PathItem{
		Get: &Operation{
			Summary:     "Import preferences",
			Description: "Apply validated preferences from query values or a portable short code",
			Tags:        []string{"Preferences"},
			Parameters: []Parameter{
				{Name: "theme", In: "query", Schema: &Schema{Type: "string", Example: "dark"}},
				{Name: "lang", In: "query", Schema: &Schema{Type: "string", Example: "en"}},
				{Name: "code", In: "query", Description: "Base64url portable preference code", Schema: &Schema{Type: "string"}},
			},
			Responses: map[string]Response{
				"200": okResponse("Imported preferences", &Schema{Type: "object"}),
			},
		},
	}

	paths[version.APIPrefix+"/server/status"] = PathItem{
		Get: &Operation{
			Summary:     "Operator server status",
			Description: "Detailed server status. Requires the operator bearer token (AI.md PART 11)",
			Tags:        []string{"Operator"},
			Security:    operatorSecurity(),
			Responses: map[string]Response{
				"200": okResponse("Server status", &Schema{Type: "object"}),
				"401": errorResponse("Missing or invalid operator token"),
			},
		},
	}

	paths[version.APIPrefix+"/server/config"] = PathItem{
		Get: &Operation{
			Summary:     "Operator server config",
			Description: "Effective server configuration with secrets redacted. Requires the operator bearer token (AI.md PART 11)",
			Tags:        []string{"Operator"},
			Security:    operatorSecurity(),
			Responses: map[string]Response{
				"200": okResponse("Server configuration", &Schema{Type: "object"}),
				"401": errorResponse("Missing or invalid operator token"),
			},
		},
	}

	geoIPLookup := &Operation{
		Summary: "GeoIP lookup", Description: "Look up the requester address or an explicit public IP address", Tags: []string{"GeoIP"},
		Parameters: []Parameter{{Name: "ip", In: "path", Description: "IP address to look up", Required: true, Schema: &Schema{Type: "string"}}},
		Responses:  map[string]Response{"200": okResponse("GeoIP result with data-source attribution", &Schema{Type: "object"}), "400": errorResponse("Invalid IP address"), "503": errorResponse("GeoIP database unavailable")},
	}
	paths[version.APIPrefix+"/server/geoip/ips"] = PathItem{Get: &Operation{Summary: geoIPLookup.Summary, Description: geoIPLookup.Description, Tags: geoIPLookup.Tags, Responses: geoIPLookup.Responses}}
	paths[version.APIPrefix+"/server/geoip/ips/{ip}"] = PathItem{Get: geoIPLookup}

	presetList := &Operation{Summary: "List GeoIP presets", Description: "List saved country-list presets; operator bearer token required", Tags: []string{"GeoIP", "Operator"}, Security: operatorSecurity(), Responses: map[string]Response{"200": okResponse("Saved presets", &Schema{Type: "array", Items: &Schema{Type: "object"}}), "401": errorResponse("Missing or invalid operator token")}}
	presetCreate := &Operation{Summary: "Save GeoIP preset", Description: "Save a named country list without applying it; operator bearer token required", Tags: []string{"GeoIP", "Operator"}, Security: operatorSecurity(), RequestBody: &RequestBody{Required: true, Content: map[string]MediaType{"application/json": {Schema: &Schema{Type: "object", Properties: map[string]Schema{"name": {Type: "string"}, "codes": {Type: "array", Items: &Schema{Type: "string"}}, "from": {Type: "string", Example: "deny_countries"}}}}}}, Responses: map[string]Response{"200": okResponse("Saved preset", &Schema{Type: "object"}), "400": errorResponse("Invalid preset input"), "401": errorResponse("Missing or invalid operator token")}}
	paths[version.APIPrefix+"/server/geoip/presets"] = PathItem{Get: presetList, Post: presetCreate}
	paths[version.APIPrefix+"/server/geoip/presets/{name}"] = PathItem{
		Get:    &Operation{Summary: "Get GeoIP preset", Description: "Read a saved country-list preset without applying it; operator bearer token required", Tags: []string{"GeoIP", "Operator"}, Security: operatorSecurity(), Parameters: []Parameter{{Name: "name", In: "path", Required: true, Schema: &Schema{Type: "string"}}}, Responses: map[string]Response{"200": okResponse("Saved preset", &Schema{Type: "object"}), "401": errorResponse("Missing or invalid operator token"), "404": errorResponse("Preset not found")}},
		Delete: &Operation{Summary: "Delete GeoIP preset", Description: "Delete a saved country-list preset; operator bearer token required", Tags: []string{"GeoIP", "Operator"}, Security: operatorSecurity(), Parameters: []Parameter{{Name: "name", In: "path", Required: true, Schema: &Schema{Type: "string"}}}, Responses: map[string]Response{"200": okResponse("Deleted preset", &Schema{Type: "object"}), "401": errorResponse("Missing or invalid operator token"), "404": errorResponse("Preset not found")}},
	}

	taskList := &Operation{Summary: "List scheduler tasks", Description: "Operator bearer token required", Tags: []string{"Scheduler", "Operator"}, Security: operatorSecurity(), Responses: map[string]Response{"200": okResponse("Scheduler tasks", &Schema{Type: "array", Items: &Schema{Type: "object"}}), "401": errorResponse("Missing or invalid operator token"), "503": errorResponse("Scheduler unavailable")}}
	taskParam := []Parameter{{Name: "task", In: "path", Required: true, Schema: &Schema{Type: "string"}}}
	paths[version.APIPrefix+"/server/scheduler/tasks"] = PathItem{Get: taskList}
	paths[version.APIPrefix+"/server/scheduler/tasks/{task}"] = PathItem{Get: &Operation{Summary: "Get scheduler task", Tags: []string{"Scheduler", "Operator"}, Security: operatorSecurity(), Parameters: taskParam, Responses: map[string]Response{"200": okResponse("Scheduler task", &Schema{Type: "object"}), "401": errorResponse("Missing or invalid operator token"), "404": errorResponse("Task not found")}}}
	paths[version.APIPrefix+"/server/scheduler/tasks/{task}/history"] = PathItem{Get: &Operation{Summary: "Get scheduler task history", Tags: []string{"Scheduler", "Operator"}, Security: operatorSecurity(), Parameters: taskParam, Responses: map[string]Response{"200": okResponse("Recent task history", &Schema{Type: "array", Items: &Schema{Type: "object"}}), "401": errorResponse("Missing or invalid operator token"), "404": errorResponse("Task not found")}}}
	for action, summary := range map[string]string{"run": "Run scheduler task", "enable": "Enable scheduler task", "disable": "Disable scheduler task"} {
		paths[version.APIPrefix+"/server/scheduler/tasks/{task}/"+action] = PathItem{Post: &Operation{Summary: summary, Description: "Operator bearer token required", Tags: []string{"Scheduler", "Operator"}, Security: operatorSecurity(), Parameters: taskParam, Responses: map[string]Response{"200": okResponse("Scheduler task action result", &Schema{Type: "object"}), "401": errorResponse("Missing or invalid operator token"), "404": errorResponse("Task not found"), "503": errorResponse("Scheduler unavailable")}}}
	}

	swaggerSpec := PathItem{
		Get: &Operation{
			Summary:     "OpenAPI specification",
			Description: "Return this OpenAPI 3.0 document as JSON",
			Tags:        []string{"Docs"},
			Responses: map[string]Response{
				"200": {
					Description: "OpenAPI document",
					Content:     map[string]MediaType{"application/json": {Schema: &Schema{Type: "object"}}},
				},
			},
		},
	}
	paths[version.APIPrefix+"/server/swagger"] = swaggerSpec
	paths["/api/swagger"] = swaggerSpec

	graphqlQuery := PathItem{
		Post: &Operation{
			Summary:     "GraphQL query",
			Description: "Execute a GraphQL query or mutation",
			Tags:        []string{"Docs"},
			RequestBody: &RequestBody{
				Required: true,
				Content: map[string]MediaType{
					"application/json": {Schema: &Schema{
						Type: "object",
						Properties: map[string]Schema{
							"query":         {Type: "string", Example: "{ search(q: \"golang\") { title url } }"},
							"variables":     {Type: "object"},
							"operationName": {Type: "string"},
						},
					}},
				},
			},
			Responses: map[string]Response{
				"200": {
					Description: "GraphQL result",
					Content:     map[string]MediaType{"application/json": {Schema: &Schema{Type: "object"}}},
				},
				"400": errorResponse("Malformed GraphQL request"),
			},
		},
	}
	paths[version.APIPrefix+"/server/graphql"] = graphqlQuery
	paths["/api/graphql"] = graphqlQuery
	mcpEndpoint := PathItem{Post: &Operation{Summary: "MCP Streamable HTTP endpoint", Description: "JSON-RPC 2.0 tools over Streamable HTTP using the same API bearer authentication", Tags: []string{"Docs"}, Responses: map[string]Response{"200": okResponse("MCP JSON-RPC response", &Schema{Type: "object"}), "400": errorResponse("Invalid MCP request"), "403": errorResponse("Origin rejected"), "405": errorResponse("Method not allowed")}}}
	paths[version.APIPrefix+"/server/mcp"] = mcpEndpoint
	paths["/api/mcp"] = mcpEndpoint
	ownerAuth := ownerSecurity()
	ownerParam := []Parameter{{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "integer"}}}
	ownerBody := func(properties map[string]Schema) *RequestBody {
		return &RequestBody{Required: true, Content: map[string]MediaType{"application/json": {Schema: &Schema{Type: "object", Properties: properties}}}}
	}
	paths[version.APIPrefix+"/server/preferences/tokens"] = PathItem{
		Get:  &Operation{Summary: "List owner tokens", Description: "Primary owner bearer token required; token values are masked", Tags: []string{"Owner Tokens"}, Security: ownerAuth, Responses: map[string]Response{"200": okResponse("Owner tokens", &Schema{Type: "array", Items: &Schema{Type: "object"}}), "403": errorResponse("Primary owner token required")}},
		Post: &Operation{Summary: "Create linked owner token", Description: "Primary owner bearer token required", Tags: []string{"Owner Tokens"}, Security: ownerAuth, RequestBody: ownerBody(map[string]Schema{"name": {Type: "string"}, "expires_at": {Type: "string"}}), Responses: map[string]Response{"201": okResponse("Created linked token", &Schema{Type: "object"}), "403": errorResponse("Primary owner token required")}},
	}
	for suffix, summary := range map[string]string{"primary/reveal": "Reveal primary owner token", "primary/rotate": "Rotate primary owner token"} {
		paths[version.APIPrefix+"/server/preferences/tokens/"+suffix] = PathItem{Post: &Operation{Summary: summary, Tags: []string{"Owner Tokens"}, Security: ownerAuth, Responses: map[string]Response{"200": okResponse("Owner token", &Schema{Type: "object"}), "403": errorResponse("Primary owner token required")}}}
	}
	paths[version.APIPrefix+"/server/preferences/tokens/{id}/reveal"] = PathItem{Post: &Operation{Summary: "Reveal linked owner token", Tags: []string{"Owner Tokens"}, Security: ownerAuth, Parameters: ownerParam, Responses: map[string]Response{"200": okResponse("Owner token", &Schema{Type: "object"}), "404": errorResponse("Token not found")}}}
	paths[version.APIPrefix+"/server/preferences/tokens/{id}/rotate"] = PathItem{Post: &Operation{Summary: "Rotate linked owner token", Tags: []string{"Owner Tokens"}, Security: ownerAuth, Parameters: ownerParam, Responses: map[string]Response{"200": okResponse("Owner token", &Schema{Type: "object"}), "404": errorResponse("Token not found")}}}
	paths[version.APIPrefix+"/server/preferences/tokens/{id}"] = PathItem{
		Patch:  &Operation{Summary: "Update linked owner token", Tags: []string{"Owner Tokens"}, Security: ownerAuth, Parameters: ownerParam, RequestBody: ownerBody(map[string]Schema{"name": {Type: "string"}, "expires_at": {Type: "string"}}), Responses: map[string]Response{"200": okResponse("Updated token", &Schema{Type: "object"}), "404": errorResponse("Token not found")}},
		Delete: &Operation{Summary: "Revoke linked owner token", Tags: []string{"Owner Tokens"}, Security: ownerAuth, Parameters: ownerParam, Responses: map[string]Response{"200": okResponse("Revoked token", &Schema{Type: "object"}), "404": errorResponse("Token not found")}},
	}
	paths[version.APIPrefix+"/server/config/contact/webhooks"] = PathItem{Post: &Operation{
		Summary:     "Configure a contact notification webhook",
		Description: "Operator bearer token required. A newly generated signing secret is returned once when a URL is first configured or changed.",
		Tags:        []string{"Configuration", "Notifications"},
		Security:    operatorSecurity(),
		RequestBody: &RequestBody{Required: true, Content: map[string]MediaType{"application/json": {Schema: &Schema{Type: "object", Example: map[string]string{"role": "general", "transport": "generic", "url": "https://example.com/webhook"}, Properties: map[string]Schema{"role": {Type: "string", Example: "admin|security|abuse|general"}, "transport": {Type: "string", Example: "telegram|discord|slack|mattermost|pushover|gotify|generic"}, "url": {Type: "string", Example: "https://example.com/webhook"}}}}}},
		Responses:   map[string]Response{"200": okResponse("Webhook configuration updated; a new signing secret may be included once", &Schema{Type: "object", Properties: map[string]Schema{"role": {Type: "string"}, "transport": {Type: "string"}, "configured": {Type: "boolean"}, "secret": {Type: "string"}}}), "400": errorResponse("Invalid webhook configuration"), "401": errorResponse("Operator bearer token required"), "500": errorResponse("Webhook configuration could not be persisted")},
	}}

	return paths
}

func healthResponseSchema() *Schema {
	return &Schema{Type: "object", Properties: map[string]Schema{
		"project": {Type: "object", Properties: map[string]Schema{
			"name": {Type: "string"}, "tagline": {Type: "string"}, "description": {Type: "string"},
		}},
		"status":          {Type: "string", Example: "healthy"},
		"pending_restart": {Type: "boolean"},
		"restart_reason":  {Type: "array", Items: &Schema{Type: "string"}},
		"version":         {Type: "string", Example: "1.0.0"}, "go_version": {Type: "string"},
		"build":  {Type: "object", Properties: map[string]Schema{"commit": {Type: "string"}, "date": {Type: "string"}}},
		"uptime": {Type: "string"}, "mode": {Type: "string"}, "timestamp": {Type: "string"},
		"features": {Type: "object", Properties: map[string]Schema{
			"tor":   {Type: "object", Properties: map[string]Schema{"enabled": {Type: "boolean"}, "running": {Type: "boolean"}, "status": {Type: "string"}, "hostname": {Type: "string"}}},
			"i2p":   {Type: "object", Properties: map[string]Schema{"enabled": {Type: "boolean"}, "running": {Type: "boolean"}, "status": {Type: "string"}, "hostname": {Type: "string"}, "provider": {Type: "string"}}},
			"geoip": {Type: "boolean"},
		}},
		"checks": {Type: "object", Properties: map[string]Schema{
			"database": {Type: "string"}, "cache": {Type: "string"}, "disk": {Type: "string"},
			"scheduler": {Type: "string"}, "tor": {Type: "string"}, "i2p": {Type: "string"},
		}},
		"stats": {Type: "object", Properties: map[string]Schema{
			"requests_total": {Type: "integer"}, "requests_24h": {Type: "integer"}, "active_connections": {Type: "integer"},
		}},
	}}
}

// okResponse builds a success response documented with the AI.md PART 14
// envelope: {"ok": true, "data": ...}.
func okResponse(description string, data *Schema) Response {
	props := map[string]Schema{"ok": {Type: "boolean", Example: true}}
	if data != nil {
		props["data"] = *data
	}
	return Response{
		Description: description,
		Content: map[string]MediaType{
			"application/json": {Schema: &Schema{Type: "object", Properties: props}},
			"text/plain":       {Schema: &Schema{Type: "string"}},
		},
	}
}

// errorResponse builds an error response documented with the AI.md PART 14
// canonical error envelope.
func errorResponse(description string) Response {
	return Response{
		Description: description,
		Content: map[string]MediaType{
			"application/json": {Schema: &Schema{
				Type: "object",
				Properties: map[string]Schema{
					"ok":      {Type: "boolean", Example: false},
					"error":   {Type: "string", Example: "BAD_REQUEST"},
					"message": {Type: "string", Example: "Human readable message"},
					"details": {Type: "object"},
				},
			}},
		},
	}
}

// paginationParameters returns the standard limit/offset/sort/order query
// parameters described in AI.md PART 14.
func paginationParameters() []Parameter {
	return []Parameter{
		{Name: "limit", In: "query", Description: "Maximum items to return (default 250)", Schema: &Schema{Type: "integer", Example: 250}},
		{Name: "offset", In: "query", Description: "Number of items to skip", Schema: &Schema{Type: "integer", Example: 0}},
		{Name: "sort", In: "query", Description: "Field to sort by", Schema: &Schema{Type: "string"}},
		{Name: "order", In: "query", Description: "Sort direction: asc or desc", Schema: &Schema{Type: "string", Example: "asc"}},
	}
}

// operatorSecurity returns the security requirement for operator-token routes.
func operatorSecurity() []map[string][]string {
	return []map[string][]string{{"operatorToken": {}}}
}

func ownerSecurity() []map[string][]string { return []map[string][]string{{"ownerBearer": {}}} }

// generateComponents generates reusable component schemas
func generateComponents() Components {
	return Components{
		SecuritySchemes: map[string]SecurityScheme{
			"operatorToken": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "opaque",
				Description:  "Operator bearer token (AI.md PART 11)",
			},
			"ownerBearer": {Type: "http", Scheme: "bearer", BearerFormat: "opaque", Description: "Primary anonymous owner bearer token"},
		},
		Schemas: map[string]Schema{
			"SearchResult": {
				Type: "object",
				Properties: map[string]Schema{
					"title":     {Type: "string"},
					"url":       {Type: "string"},
					"content":   {Type: "string"},
					"engine":    {Type: "string"},
					"score":     {Type: "number"},
					"image_url": {Type: "string"},
					"thumbnail": {Type: "string"},
					"published": {Type: "string"},
				},
			},
			"HealthResponse": *healthResponseSchema(),
		},
	}
}
