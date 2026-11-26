package server

import (
	"net/http"

	"github.com/apimgr/search/src/api"
	"github.com/apimgr/search/src/common/httputil"
	graphqlpkg "github.com/apimgr/search/src/graphql"
	swaggerpkg "github.com/apimgr/search/src/swagger"
)

func (s *Server) renderGraphQLDocs(w http.ResponseWriter, r *http.Request, query, response string) {
	data := s.newPageData(w, r, "GraphQL Explorer", "graphql")
	data.Data = map[string]interface{}{
		"Query":    query,
		"Response": response,
		"Schema":   graphqlpkg.GeneratedSchemaSDL,
		"Action":   httputil.BuildURL(r, "/server/docs/graphql"),
		"APIURL":   httputil.BuildURL(r, api.APIPrefix+"/server/graphql"),
		"APIAlias": httputil.BuildURL(r, "/api/graphql"),
	}
	if err := s.renderer.Render(w, "graphql", data); err != nil {
		s.handleInternalError(w, r, "GraphQL documentation template", err)
	}
}

func (s *Server) handleSwaggerThemeCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(swaggerpkg.ThemeCSS(GetTheme(r))))
}

func (s *Server) handleGraphQLThemeCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(graphqlpkg.ThemeCSS(GetTheme(r))))
}

func (s *Server) handleSwaggerDocs(w http.ResponseWriter, r *http.Request) {
	data := s.newPageData(w, r, "API Documentation", "swagger")
	data.Data = map[string]interface{}{
		"SpecURL":  httputil.BuildURL(r, api.APIPrefix+"/server/swagger"),
		"APIAlias": httputil.BuildURL(r, "/api/swagger"),
	}
	if err := s.renderer.Render(w, "swagger", data); err != nil {
		s.handleInternalError(w, r, "Swagger documentation template", err)
	}
}
