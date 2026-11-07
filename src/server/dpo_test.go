package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestDPOContactUsesPublicFallbackOnly(t *testing.T) {
	s := &Server{config: &config.Config{}}
	s.config.Server.Contact.Admin.Email = "admin@example.test"
	s.config.Server.Contact.General.Email = "general@example.test"

	request := httptest.NewRequest(http.MethodGet, "https://search.example.test/server/dpo", nil)
	got := s.dpoContact(request)
	if got.Email != "general@example.test" || got.Name != "" || got.PrivacyURL != "https://search.example.test/server/privacy" {
		t.Fatalf("dpoContact() = %+v", got)
	}

	s.config.Server.Contact.DPO.Name = "Privacy Officer"
	s.config.Server.Contact.DPO.Email = "dpo@example.test"
	got = s.dpoContact(request)
	if got.Email != "dpo@example.test" || got.Name != "Privacy Officer" {
		t.Fatalf("configured DPO was not preferred: %+v", got)
	}

	s.config.Server.Contact.General.Email = ""
	s.config.Server.Contact.DPO.Email = ""
	got = s.dpoContact(request)
	if got.Email != "" || got.Name != "Privacy Officer" {
		t.Fatalf("empty public contacts should remain empty: %+v", got)
	}
}
