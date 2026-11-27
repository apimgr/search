package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestContactWebhookConfigSaveNeverReturnsSecret(t *testing.T) {
	base := filepath.Join(os.TempDir(), config.ProjectOrg)
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	tempDir, err := os.MkdirTemp(base, config.InternalName+"-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	if err := os.Chmod(tempDir, 0700); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(tempDir, "server.yml")
	cfg := config.DefaultConfig()
	cfg.Server.Token = "operator-token"
	cfg.SetPath(configPath)
	if err := cfg.Save(configPath); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(cfg, nil, nil)
	secured := handler.requireOperator(handler.handleContactWebhookConfigSave)
	save := func(target string) (int, map[string]interface{}, string) {
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
		return response.Code, data, response.Body.String()
	}
	code, created, createdBody := save("https://hooks.example/first")
	if code != http.StatusOK || created["secret"] != nil {
		t.Fatalf("first save exposed the signing secret: status=%d data=%#v body=%s", code, created, createdBody)
	}
	secret := cfg.Get().Contact.General.Webhooks["generic_secret"]
	if secret == "" {
		t.Fatal("signing secret was not generated")
	}
	if strings.Contains(createdBody, secret) {
		t.Fatal("first save response body contained the signing secret")
	}
	stored, err := os.ReadFile(filepath.Join(tempDir, "secrets.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stored, []byte(secret)) {
		t.Fatal("secrets.yml did not persist the signing secret")
	}
	code, unchanged, unchangedBody := save("https://hooks.example/first")
	if code != http.StatusOK || unchanged["secret"] != nil {
		t.Fatalf("unchanged save exposed the signing secret: status=%d data=%#v", code, unchanged)
	}
	if strings.Contains(unchangedBody, secret) {
		t.Fatal("unchanged save response body contained the signing secret")
	}
	code, rotated, rotatedBody := save("https://hooks.example/second")
	if code != http.StatusOK || rotated["secret"] != nil {
		t.Fatalf("changed URL exposed the new signing secret: status=%d data=%#v", code, rotated)
	}
	newSecret := cfg.Get().Contact.General.Webhooks["generic_secret"]
	if newSecret == "" || newSecret == secret {
		t.Fatal("changed URL did not rotate the signing secret")
	}
	if strings.Contains(rotatedBody, newSecret) {
		t.Fatal("rotated save response body contained the new signing secret")
	}
	stored, err = os.ReadFile(filepath.Join(tempDir, "secrets.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte(secret)) || !bytes.Contains(stored, []byte(newSecret)) {
		t.Fatal("secrets.yml did not persist the rotated webhook secret")
	}
	if info, err := os.Stat(configPath); err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatalf("config file permissions are not private: info=%v err=%v", info, err)
	}
	secretInfo, err := os.Stat(filepath.Join(tempDir, "secrets.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if secretInfo.Mode().Perm() != 0600 {
		t.Fatalf("secrets.yml permissions = %04o, want 0600", secretInfo.Mode().Perm())
	}
}
