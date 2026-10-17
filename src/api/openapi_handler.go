package api

import (
	"net/http"

	"github.com/apimgr/search/src/swagger"
	"github.com/go-chi/chi/v5"
)

// RegisterOpenAPIRoutes registers the OpenAPI/Swagger routes.
// Per AI.md PART 14 "Root-Level Endpoints":
//   - /server/docs/swagger              — Swagger UI (fetches /api/swagger)
//   - /api/{api_version}/server/swagger — OpenAPI JSON spec (versioned)
//   - /api/swagger                      — same handler, unversioned alias
//
// The old /openapi and /openapi.json paths are removed with no redirect
// (AI.md PART 14, "Old paths removed"). The spec is generated from code —
// never hand-maintained.
func (h *Handler) RegisterOpenAPIRoutes(r chi.Router) {
	specHandler := swagger.SpecHandler(h.config)

	r.Get("/server/docs/swagger", swagger.UIHandler(h.config))
	r.Get(APIPrefix+"/server/swagger", specHandler)
	r.Get("/api/swagger", specHandler)
}

// ServeOpenAPISpec serves the generated OpenAPI JSON specification.
// Exposed for callers that need the spec handler directly.
func (h *Handler) ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	swagger.SpecHandler(h.config)(w, r)
}

// ServeSwaggerUI serves the themed Swagger UI page.
func (h *Handler) ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	swagger.UIHandler(h.config)(w, r)
}
