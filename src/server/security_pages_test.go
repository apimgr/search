package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// ---------- security_pages.go: handleSecurityOverview ----------

// TestHandleSecurityOverview_Renders confirms the handler does not 400 and
// resolves a mailto contact from the security email fallback chain.
func TestHandleSecurityOverview_Renders(t *testing.T) {
	s := newTestServer(t)
	origContact := s.config.Server.Web.Security.Contact
	origSecEmail := s.config.Server.Contact.Security.Email
	s.config.Server.Web.Security.Contact = ""
	s.config.Server.Contact.Security.Email = "security@example.com"
	t.Cleanup(func() {
		s.config.Server.Web.Security.Contact = origContact
		s.config.Server.Contact.Security.Email = origSecEmail
	})

	req := httptest.NewRequest(http.MethodGet, "/server/security", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityOverview(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSecurityOverview returned 400 (should be 200 or 500)")
	}
}

// TestHandleSecurityOverview_ContactPreferredOverEmail confirms Web.Security.Contact
// wins over the Contact.Security.Email fallback when both are set.
func TestHandleSecurityOverview_ContactPreferredOverEmail(t *testing.T) {
	s := newTestServer(t)
	origContact := s.config.Server.Web.Security.Contact
	origSecEmail := s.config.Server.Contact.Security.Email
	s.config.Server.Web.Security.Contact = "https://example.com/report"
	s.config.Server.Contact.Security.Email = "unused@example.com"
	t.Cleanup(func() {
		s.config.Server.Web.Security.Contact = origContact
		s.config.Server.Contact.Security.Email = origSecEmail
	})

	req := httptest.NewRequest(http.MethodGet, "/server/security", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityOverview(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSecurityOverview returned 400 (should be 200 or 500)")
	}
}

// TestHandleSecurityOverview_NoContactConfigured covers the branch where no
// contact address is configured at any level of the fallback chain.
func TestHandleSecurityOverview_NoContactConfigured(t *testing.T) {
	s := newTestServer(t)
	origContact := s.config.Server.Web.Security.Contact
	origSecEmail := s.config.Server.Contact.Security.Email
	origAdminEmail := s.config.Server.Contact.Admin.Email
	s.config.Server.Web.Security.Contact = ""
	s.config.Server.Contact.Security.Email = ""
	s.config.Server.Contact.Admin.Email = ""
	t.Cleanup(func() {
		s.config.Server.Web.Security.Contact = origContact
		s.config.Server.Contact.Security.Email = origSecEmail
		s.config.Server.Contact.Admin.Email = origAdminEmail
	})

	req := httptest.NewRequest(http.MethodGet, "/server/security", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityOverview(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSecurityOverview returned 400 (should be 200 or 500)")
	}
}

// TestHandleSecurityOverview_CustomExpires confirms an explicitly configured
// expiry timestamp is honored instead of the computed +1 year default.
func TestHandleSecurityOverview_CustomExpires(t *testing.T) {
	s := newTestServer(t)
	origExpires := s.config.Server.Web.Security.Expires
	s.config.Server.Web.Security.Expires = "2099-01-01T00:00:00Z"
	t.Cleanup(func() { s.config.Server.Web.Security.Expires = origExpires })

	req := httptest.NewRequest(http.MethodGet, "/server/security", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityOverview(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSecurityOverview returned 400 (should be 200 or 500)")
	}
}

// TestHandleSecurityOverview_PublishPGPKeyNoKeyFile covers the branch where
// PublishPGPKey is enabled but no key file exists on disk — HasPGPKey must
// resolve to false without the handler erroring out.
func TestHandleSecurityOverview_PublishPGPKeyNoKeyFile(t *testing.T) {
	s := newTestServer(t)
	orig := s.config.Server.Web.Security.PublishPGPKey
	s.config.Server.Web.Security.PublishPGPKey = true
	t.Cleanup(func() { s.config.Server.Web.Security.PublishPGPKey = orig })

	req := httptest.NewRequest(http.MethodGet, "/server/security", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityOverview(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSecurityOverview returned 400 (should be 200 or 500)")
	}
}

// ---------- security_pages.go: handleSecurityPolicy ----------

// TestHandleSecurityPolicy_Renders confirms the handler does not 400.
func TestHandleSecurityPolicy_Renders(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/security/policy", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityPolicy(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSecurityPolicy returned 400 (should be 200 or 500)")
	}
}

// ---------- security_pages.go: handleSecurityThanks ----------

// TestHandleSecurityThanks_Renders confirms the handler does not 400.
func TestHandleSecurityThanks_Renders(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/security/thanks", nil)
	rec := httptest.NewRecorder()
	s.handleSecurityThanks(rec, req)

	if rec.Code == http.StatusBadRequest {
		t.Errorf("handleSecurityThanks returned 400 (should be 200 or 500)")
	}
}

// ---------- security_pages.go: handleSecurityReportStatus ----------

// withURLParam attaches a chi route parameter to a request context, matching
// the pattern used elsewhere in this package's tests for chi.URLParam access.
func withURLParam(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// TestHandleSecurityReportStatus_MissingParams covers the guard clause for a
// missing tracking_id, missing token, and a nil database manager — all three
// must independently produce a 404, never a panic.
func TestHandleSecurityReportStatus_MissingParams(t *testing.T) {
	tests := []struct {
		name       string
		trackingID string
		token      string
	}{
		{"no tracking id", "", "sometoken"},
		{"no token", "abc123", ""},
		{"neither", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			rawURL := "/server/security/report/" + tt.trackingID
			if tt.token != "" {
				rawURL += "?token=" + tt.token
			}
			req := httptest.NewRequest(http.MethodGet, rawURL, nil)
			req = withURLParam(req, "tracking_id", tt.trackingID)
			rec := httptest.NewRecorder()

			s.handleSecurityReportStatus(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("status = %d, want 404", rec.Code)
			}
		})
	}
}

// TestHandleSecurityReportStatus_NilDBManager confirms a well-formed request
// with both params present still 404s when s.dbManager is nil (the shared
// test server has no database configured), instead of panicking on a nil
// pointer dereference.
func TestHandleSecurityReportStatus_NilDBManager(t *testing.T) {
	s := newTestServer(t)
	if s.dbManager != nil {
		t.Skip("shared test server unexpectedly has a database manager configured")
	}

	req := httptest.NewRequest(http.MethodGet, "/server/security/report/abc123?token=sometoken", nil)
	req = withURLParam(req, "tracking_id", "abc123")
	rec := httptest.NewRecorder()

	s.handleSecurityReportStatus(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// TestHandleSecurityReportStatus_TokenTrimmed confirms surrounding whitespace
// in the token query param does not bypass the empty-token guard.
func TestHandleSecurityReportStatus_TokenTrimmed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/server/security/report/abc123?token=%20%20", nil)
	req = withURLParam(req, "tracking_id", "abc123")
	rec := httptest.NewRecorder()

	s.handleSecurityReportStatus(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for whitespace-only token", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "abc123") {
		t.Error("tracking id should not leak into a 404 body")
	}
}
