package graphql

import (
	"context"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/direct"
	"github.com/apimgr/search/src/instant"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/widget"
	gql "github.com/graphql-go/graphql"
)

type graphqlTestEngine struct{ config *model.EngineConfig }

func (e graphqlTestEngine) Name() string                                  { return e.config.Name }
func (e graphqlTestEngine) DisplayName() string                           { return e.config.DisplayName }
func (e graphqlTestEngine) IsEnabled() bool                               { return true }
func (e graphqlTestEngine) GetPriority() int                              { return 1 }
func (e graphqlTestEngine) SupportsCategory(category model.Category) bool { return true }
func (e graphqlTestEngine) GetConfig() *model.EngineConfig                { return e.config }
func (e graphqlTestEngine) Search(_ context.Context, query *model.Query) ([]model.Result, error) {
	return []model.Result{{Title: "Fixture result", URL: "https://example.test/", Content: query.Text, Engine: e.config.Name, Category: query.Category, Thumbnail: "https://images.example.test/thumb.jpg", ImageURL: "https://images.example.test/full.jpg", ImageWidth: 800, ImageHeight: 600, ImageFormat: "jpg", FileSize: 4096, Duration: 634, ViewCount: 1000, Author: "Fixture Channel", EmbedURL: "https://www.youtube-nocookie.com/embed/fixture"}}, nil
}

func testGraphQLDependencies() Dependencies {
	config := model.NewEngineConfig("fixture")
	return Dependencies{
		Aggregator: search.NewAggregatorSimple([]search.Engine{graphqlTestEngine{config: config}}, 0),
		Direct:     direct.NewManager(),
		Instant:    instant.NewManager(nil),
		Widgets:    widget.NewManager(nil),
		ServerInfo: func() api.InfoResponse {
			return api.InfoResponse{Name: "Fixture Search", Version: "test", Mode: "test"}
		},
		GeoIPLookup: func(ip string) (api.GeoIPLookupResponse, error) {
			return api.GeoIPLookupResponse{IP: ip, Found: true, Attribution: "fixture attribution"}, nil
		},
		Preferences: func() interface{} {
			return map[string]interface{}{"storage": "client-side", "fields": []string{"theme"}}
		},
		SavePreferences: func() interface{} { return map[string]string{"status": "saved"} },
		Autocomplete: func(_ context.Context, query string) []string {
			return []string{query + " fixture"}
		},
		RelatedSearches: func(_ context.Context, query string, limit int) ([]string, error) {
			return []string{query + " guide", query + " examples"}[:min(limit, 2)], nil
		},
		EngineInventory: func() []api.EngineInfo {
			return []api.EngineInfo{{ID: "fixture", Name: "Fixture", Enabled: true, Categories: []string{"general"}}}
		},
	}
}

func testResolverParams(args map[string]interface{}) gql.ResolveParams {
	ctx := context.WithValue(context.Background(), dependenciesContextKey{}, testGraphQLDependencies())
	return gql.ResolveParams{Context: ctx, Args: args}
}
