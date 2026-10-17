// Package server: exported access to the fully wired HTTP handler.
//
// Per AI.md PART 28, the browser E2E suite lives in tests/e2e/ behind the
// `e2e` build tag. Those tests drive the real router rather than a
// reconstructed subset, so the handler has to be reachable from outside the
// package: they wrap it in an httptest.Server instead of shelling out to a
// compiled binary.
//
// setupRoutes is a pure registration pass — it builds a fresh chi router and
// wires handlers, touching no listener, database, or scheduler — so calling it
// more than once is safe and each caller gets an independent router.
package server

import "net/http"

// Handler returns the server's complete HTTP handler: every page, API, PWA,
// and well-known route with the full middleware chain already applied.
//
// It is intended for tests and for embedding the server in another process;
// a running service reaches the same handler through Start.
func (s *Server) Handler() http.Handler {
	return s.setupRoutes()
}
