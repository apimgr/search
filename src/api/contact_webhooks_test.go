package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestContactWebhookConfigSaveReturnsSecretOnlyWhenCreatedOrRotated(t *testing.T) {
	base := filepath.Join(os.TempDir(), "apimgr")
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	tempDir, err := os.MkdirTemp(base, "search-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tempDir, "server.yml")
	cfg := &config.Config{Server: config.ServerConfig{Token: "operator-token"}}
	cfg.SetPath(configPath)
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(cfg, nil, nil)
	secured := handler.requireOperator(handler.handleContactWebhookConfigSave)
	save := func(target string) (int, map[string]interface{}) {
		t.Helper()
		input, _ := json.Marshal(contactWebhookSaveRequest{Role: "general", Transport: "generic", URL: target})
		req := httptest.NewRequest(http.MethodPost, APIPrefix+"/server/config/contact/webhooks", bytes.NewReader(input))
		req.Header.Set("Authorization", "Bearer operator-token")
		response := httptest.NewRecorder()
		secured(response, req)
		var envelope APIResponse
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control = %q, want no-store", response.Header().Get("Cache-Control"))
		}
		data, _ := envelope.Data.(map[string]interface{})
		return response.Code, data
	}
	code, created := save("https://hooks.example/first")
	secret, ok := created["secret"].(string)
	if code != http.StatusOK || !ok || secret == "" {
		t.Fatalf("first save must return secret once: status=%d data=%#v", code, created)
	}
	if cfg.Get().Contact.General.Webhooks["generic_secret"] != secret {
		t.Fatal("signing secret was not persisted in server config")
	}
	code, unchanged := save("https://hooks.example/first")
	if code != http.StatusOK || unchanged["secret"] != nil {
		t.Fatalf("unchanged save must not return secret again: status=%d data=%#v", code, unchanged)
	}
	code, rotated := save("https://hooks.example/second")
	newSecret, ok := rotated["secret"].(string)
	if code != http.StatusOK || !ok || newSecret == secret {
		t.Fatalf("changed URL must return a new secret: status=%d data=%#v", code, rotated)
	}
	stored, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte(secret)) || !bytes.Contains(stored, []byte(newSecret)) {
		t.Fatal("config persistence did not rotate webhook secret")
	}
	if info, err := os.Stat(configPath); err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("config file permissions are not private: info=%v err=%v", info, err)
	}
}
