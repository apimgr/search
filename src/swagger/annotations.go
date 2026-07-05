package swagger

import (
	"github.com/apimgr/search/src/api"
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
							Schema: &Schema{
								Type: "object",
								Properties: map[string]Schema{
									"status":  {Type: "string", Example: "healthy"},
									"version": {Type: "string", Example: "1.0.0"},
									"uptime":  {Type: "string", Example: "2d 5h 30m"},
								},
							},
						},
					},
				},
			},
		},
	}

	// API v1 health check - canonical route is /api/v1/server/healthz per AI.md PART 13/14
	paths[api.APIPrefix+"/server/healthz"] = paths["/server/healthz"]

	// Search endpoint
	paths[api.APIPrefix+"/search"] = PathItem{
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
									"query":         {Type: "string"},
									"results":       {Type: "array", Items: &Schema{Type: "object"}},
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
	}

	// Autocomplete endpoint
	paths[api.APIPrefix+"/autocomplete"] = PathItem{
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
	paths[api.APIPrefix+"/direct/{type}/{term}"] = PathItem{
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
	paths[api.APIPrefix+"/instant"] = PathItem{
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

	paths[api.APIPrefix+"/alerts"] = PathItem{
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

	paths[api.APIPrefix+"/alerts/{token}"] = PathItem{
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

	paths[api.APIPrefix+"/alerts/{token}/pause"] = PathItem{
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

	paths[api.APIPrefix+"/alerts/{token}/rss"] = PathItem{
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

	return paths
}

// generateComponents generates reusable component schemas
func generateComponents() Components {
	return Components{
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
			"HealthResponse": {
				Type: "object",
				Properties: map[string]Schema{
					"status":  {Type: "string"},
					"version": {Type: "string"},
					"uptime":  {Type: "string"},
				},
			},
		},
	}
}
