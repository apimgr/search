package graphql

import (
	"github.com/apimgr/search/src/config"
	"github.com/graphql-go/graphql"
)

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

// resolveDirectAnswer handles direct answer queries
// GraphQL directAnswer returns stub data by design - use REST API /api/v1/direct/{type}/{term} for full functionality
func resolveDirectAnswer(p graphql.ResolveParams) (interface{}, error) {
	answerType, _ := p.Args["type"].(string)
	term, _ := p.Args["term"].(string)
	return map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"type":            answerType,
			"term":            term,
			"title":           "",
			"description":     "",
			"content":         "",
			"source":          "",
			"sourceUrl":       "",
			"cacheTtlSeconds": 0,
			"found":           false,
		},
	}, nil
}

// resolveInstant handles instant answer queries
// GraphQL instant returns stub data by design - use REST API /api/v1/instant for full functionality
func resolveInstant(p graphql.ResolveParams) (interface{}, error) {
	query, _ := p.Args["q"].(string)
	return map[string]interface{}{
		"ok": true,
		"data": map[string]interface{}{
			"query":   query,
			"type":    "",
			"title":   "",
			"content": "",
			"source":  "",
			"found":   false,
		},
	}, nil
}
