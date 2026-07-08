package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHandleServerStatus verifies the 0% handler returns 200 with ok=true and status=healthy.
func TestHandleServerStatus(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/server/status", nil)
	w := httptest.NewRecorder()

	handler.handleServerStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleServerStatus() status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	if !resp.OK {
		t.Error("handleServerStatus() response.OK = false, want true")
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("handleServerStatus() response.Data type = %T, want map[string]interface{}", resp.Data)
	}
	if data["status"] != "healthy" {
		t.Errorf("handleServerStatus() status = %v, want healthy", data["status"])
	}
	if _, hasGoVersion := data["go_version"]; !hasGoVersion {
		t.Error("handleServerStatus() response should contain go_version field")
	}
}

// TestHandleServerStatusMaintenance verifies maintenance mode is reported in the status field.
func TestHandleServerStatusMaintenance(t *testing.T) {
	handler := newTestHandler()
	handler.config.Server.MaintenanceMode = true

	req := httptest.NewRequest(http.MethodGet, "/server/status", nil)
	w := httptest.NewRecorder()

	handler.handleServerStatus(w, req)

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("response.Data type = %T, want map[string]interface{}", resp.Data)
	}
	if data["status"] != "maintenance" {
		t.Errorf("handleServerStatus() maintenance status = %v, want maintenance", data["status"])
	}
}

// TestHandleServerConfig verifies the 0% handler returns 200 with ok=true and expected config fields.
func TestHandleServerConfig(t *testing.T) {
	handler := newTestHandler()
	req := httptest.NewRequest(http.MethodGet, "/server/config", nil)
	w := httptest.NewRecorder()

	handler.handleServerConfig(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleServerConfig() status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	if !resp.OK {
		t.Error("handleServerConfig() response.OK = false, want true")
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("handleServerConfig() response.Data type = %T, want map[string]interface{}", resp.Data)
	}
	for _, field := range []string{"mode", "maintenance", "branding", "features"} {
		if _, exists := data[field]; !exists {
			t.Errorf("handleServerConfig() response.Data missing field %q", field)
		}
	}
}

// TestHandleHealthzMaintenanceMode verifies 503 is returned and status=maintenance when in maintenance.
func TestHandleHealthzMaintenanceMode(t *testing.T) {
	handler := newTestHandler()
	handler.config.Server.MaintenanceMode = true

	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("handleHealthz() maintenance status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("response.Data type = %T, want map[string]interface{}", resp.Data)
	}
	if data["status"] != "maintenance" {
		t.Errorf("handleHealthz() maintenance status = %v, want maintenance", data["status"])
	}
}

// TestHandleHealthzTorEnabledNilService verifies tor.status=unavailable when tor is enabled but service is nil.
func TestHandleHealthzTorEnabledNilService(t *testing.T) {
	handler := newTestHandler()
	handler.config.Server.Tor.Enabled = true

	req := httptest.NewRequest(http.MethodGet, "/server/healthz", nil)
	w := httptest.NewRecorder()

	handler.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("handleHealthz() status = %d, want 200", w.Code)
	}

	var resp APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("response.Data type = %T, want map[string]interface{}", resp.Data)
	}
	features, ok := data["features"].(map[string]interface{})
	if !ok {
		t.Fatalf("features type = %T, want map[string]interface{}", data["features"])
	}
	tor, ok := features["tor"].(map[string]interface{})
	if !ok {
		t.Fatalf("tor type = %T, want map[string]interface{}", features["tor"])
	}
	if tor["status"] != "unavailable" {
		t.Errorf("tor.status = %v, want unavailable", tor["status"])
	}
	if tor["enabled"] != true {
		t.Errorf("tor.enabled = %v, want true", tor["enabled"])
	}
}

