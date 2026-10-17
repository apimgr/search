package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/security"
	"github.com/go-chi/chi/v5"
)

func TestCreateQRShortlinkRejectsInvalidDestinations(t *testing.T) {
	server := &Server{}
	credentialURL := (&url.URL{Scheme: "https", User: url.UserPassword("test-user", "test-password"), Host: "example.test", Path: "/"}).String()
	for _, target := range []string{
		"",
		"javascript:alert(1)",
		"data:text/html,hello",
		credentialURL,
		"https:///missing-host",
		"https://example.test/\r\nLocation:https://evil.test",
	} {
		if _, err := server.createQRShortlink(context.Background(), target); err == nil {
			t.Errorf("createQRShortlink(%q) succeeded, want rejection", target)
		}
	}
}

func TestQRShortlinkEncryptsDestinationAtRest(t *testing.T) {
	server := newTestServer(t)
	target := "https://example.test/private?token=secret"
	code, err := server.createQRShortlink(context.Background(), target)
	if err != nil {
		t.Fatalf("create QR short link: %v", err)
	}
	table := database.ServerTableName(server.dbManager.ServerDB(), "qr_shortlinks")
	var encrypted string
	if err := server.dbManager.ServerDB().QueryRow(context.Background(), "SELECT target_encrypted FROM "+table+" WHERE code = ?", code).Scan(&encrypted); err != nil {
		t.Fatalf("read encrypted QR target: %v", err)
	}
	if encrypted == target {
		t.Fatal("QR target was stored in plaintext")
	}
	decrypted, err := security.DecryptAESGCM(server.config.Get().Security.EncryptionKey, encrypted)
	if err != nil {
		t.Fatalf("decrypt stored QR target: %v", err)
	}
	if string(decrypted) != target {
		t.Fatalf("decrypted target = %q, want original target", decrypted)
	}
}

func TestQRShortlinkRedirectRejectsInvalidCodesAndMethods(t *testing.T) {
	router := chi.NewRouter()
	router.Get("/s/{code}", (&Server{}).handleQRShortlinkRedirect)
	for _, test := range []struct {
		method string
		target string
		status int
	}{
		{method: http.MethodGet, target: "/s/not-a-code", status: http.StatusNotFound},
		{method: http.MethodPost, target: "/s/AAAAAAAAAAAAAAAAAAAAAA", status: http.StatusMethodNotAllowed},
	} {
		t.Run(test.method+test.target, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.target, nil)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}
