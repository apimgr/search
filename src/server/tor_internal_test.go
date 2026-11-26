package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/service"
	"github.com/go-chi/chi/v5"
)

func TestDecodeTorJSONRejectsUnknownFields(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/server/tor/regenerate", strings.NewReader(`{"confirm":true,"secret":"must-not-be-accepted"}`))
	response := httptest.NewRecorder()
	var body torConfirmRequest

	if decodeTorJSON(response, request, &body) {
		t.Fatal("decodeTorJSON accepted an unknown field")
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestTorInternalMethodMismatchIsNotFound(t *testing.T) {
	router := chi.NewRouter()
	(&Server{}).registerTorInternalRoutes(router)
	request := httptest.NewRequest(http.MethodGet, "/server/tor/vanity/stop", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestTorInternalVanityStopReturnsStateAndCandidates(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("CONFIG_DIR", t.TempDir())
	s := &Server{torService: service.NewTorService(config.DefaultConfig())}
	request := httptest.NewRequest(http.MethodPost, "/server/tor/vanity/stop", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	router := chi.NewRouter()
	s.registerTorInternalRoutes(router)

	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Stopped    bool     `json:"stopped"`
			Candidates []string `json:"candidates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.Stopped {
		t.Error("stopped = true with no running vanity search, want false")
	}
	if len(envelope.Data.Candidates) != 0 {
		t.Errorf("candidates = %v, want empty", envelope.Data.Candidates)
	}
}

func TestTorInternalValidateReturnsStructuredErrors(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("CONFIG_DIR", t.TempDir())
	cfg := config.DefaultConfig()
	cfg.Server.Tor.Enabled = false
	s := &Server{torService: service.NewTorService(cfg)}
	request := httptest.NewRequest(http.MethodPost, "/server/tor/validate", nil)
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	router := chi.NewRouter()
	s.registerTorInternalRoutes(router)

	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	var envelope struct {
		Data struct {
			Valid  bool `json:"valid"`
			Errors []struct {
				Field   string `json:"field"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.Valid || len(envelope.Data.Errors) != 2 {
		t.Fatalf("validation data = %+v, want invalid with two errors", envelope.Data)
	}
	if envelope.Data.Errors[0].Field == "" || envelope.Data.Errors[0].Message == "" {
		t.Errorf("first validation error = %+v, want field and message", envelope.Data.Errors[0])
	}
}

func TestTorInternalVanityApplyMapsMissingCandidateToNotFound(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("DATA_DIR", dataDir)
	t.Setenv("CONFIG_DIR", t.TempDir())
	s := &Server{torService: service.NewTorService(config.DefaultConfig())}
	request := httptest.NewRequest(http.MethodPost, "/server/tor/vanity/apply", strings.NewReader(`{"address":"missing","confirm":true}`))
	request.RemoteAddr = "127.0.0.1:12345"
	response := httptest.NewRecorder()
	router := chi.NewRouter()
	s.registerTorInternalRoutes(router)

	router.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, http.StatusNotFound, response.Body.String())
	}
	var envelope struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Error != "NOT_FOUND" {
		t.Errorf("error = %q, want NOT_FOUND", envelope.Error)
	}
}

func TestDecodeTorJSONRejectsTrailingDocuments(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/server/tor/regenerate", strings.NewReader(`{"confirm":true} {"confirm":false}`))
	response := httptest.NewRecorder()
	var body torConfirmRequest

	if decodeTorJSON(response, request, &body) {
		t.Fatal("decodeTorJSON accepted multiple JSON documents")
	}
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestDecodeTorJSONReadsConfirmation(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/server/tor/regenerate", strings.NewReader(`{"confirm":true}`))
	response := httptest.NewRecorder()
	var body torConfirmRequest

	if !decodeTorJSON(response, request, &body) {
		t.Fatalf("decodeTorJSON rejected valid confirmation: %s", response.Body.String())
	}
	if !body.Confirm {
		t.Fatal("confirmation was not decoded")
	}
}

func TestReadTorImportKeyReadsOnlySecretKeyFile(t *testing.T) {
	dir := t.TempDir()
	want := []byte("tor secret key fixture")
	if err := os.WriteFile(filepath.Join(dir, "hs_ed25519_secret_key"), want, 0o600); err != nil {
		t.Fatalf("write key fixture: %v", err)
	}
	got, err := readTorImportKey(dir)
	if err != nil {
		t.Fatalf("readTorImportKey() error = %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("readTorImportKey() = %q, want %q", got, want)
	}
}

func TestReadTorImportKeyRequiresAbsoluteDirectory(t *testing.T) {
	if _, err := readTorImportKey("relative/path"); err == nil {
		t.Fatal("readTorImportKey accepted a relative path")
	}
}
