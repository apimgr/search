package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleServerDPOFallbackAndEmptyAddress(t *testing.T) {
	h := newTestHandler()
	h.config.Server.Contact.Admin.Email = "admin@example.test"
	h.config.Server.Contact.General.Email = "general@example.test"

	request := httptest.NewRequest(http.MethodGet, APIPrefix+"/server/dpo", nil)
	response := httptest.NewRecorder()
	h.handleServerDPO(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var got struct {
		OK   bool              `json:"ok"`
		Data ServerDPOResponse `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got.OK || got.Data.Email != "general@example.test" || got.Data.PrivacyURL != "http://example.com/server/privacy" {
		t.Fatalf("unexpected response: %+v", got)
	}

	h.config.Server.Contact.General.Email = ""
	response = httptest.NewRecorder()
	h.handleServerDPO(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("empty-contact status = %d, want 200", response.Code)
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode empty-contact response: %v", err)
	}
	if got.Data.Email != "" {
		t.Fatalf("email = %q, want empty; admin contact must remain private", got.Data.Email)
	}
}
