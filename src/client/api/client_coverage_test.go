package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"testing"

	"github.com/apimgr/search/src/version"
)

// apiOKResp wraps data in the canonical {"ok":true,"data":...} envelope.
func apiOKResp(t *testing.T, data interface{}) []byte {
	t.Helper()
	inner, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("apiOKResp marshal error: %v", err)
	}
	return []byte(fmt.Sprintf(`{"ok":true,"data":%s}`, string(inner)))
}

// Tests for CurrentPlatform

func TestCurrentPlatform(t *testing.T) {
	got := CurrentPlatform()
	want := runtime.GOOS + "-" + runtime.GOARCH
	if got != want {
		t.Errorf("CurrentPlatform() = %q, want %q", got, want)
	}
	if !strings.Contains(got, "-") {
		t.Errorf("CurrentPlatform() = %q, expected to contain '-'", got)
	}
}

// Tests for Search missing branch (ok=false)

func TestSearchAPINotOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":false,"error":"QUERY_REQUIRED","message":"query is required"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Search("", 1, 10)
	if err == nil {
		t.Error("Search() should return error when ok=false")
	}
}

// Test Search with valid outer JSON but invalid inner data JSON

func TestSearchInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// ok=true but data is a string, not a SearchResponse object
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Search("test", 1, 10)
	if err == nil {
		t.Error("Search() should return error when data cannot be decoded")
	}
	if !strings.Contains(err.Error(), "failed to decode search data") {
		t.Errorf("error = %q, want to contain 'failed to decode search data'", err.Error())
	}
}

// Test Health with valid outer JSON but invalid inner data JSON

func TestHealthInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// ok=true but data is a string, not a HealthResponse object
		w.Write([]byte(`{"ok":true,"data":"not-a-health-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.Health()
	if err == nil {
		t.Error("Health() should return error when data cannot be decoded")
	}
	if !strings.Contains(err.Error(), "failed to decode health data") {
		t.Errorf("error = %q, want to contain 'failed to decode health data'", err.Error())
	}
}

// Tests for GetInfo

func TestGetInfo(t *testing.T) {
	tests := []struct {
		name       string
		serverName string
		version    string
		wantErr    bool
	}{
		{"success with data", "search", "1.2.3", false},
		{"empty fields", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != version.APIPrefix+"/info" {
					t.Errorf("GetInfo path = %q, want %q", r.URL.Path, version.APIPrefix+"/info")
				}
				info := InfoResponse{Name: tt.serverName, Version: tt.version}
				w.Write(apiOKResp(t, info))
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			result, err := client.GetInfo()
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetInfo() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				if result.Name != tt.serverName {
					t.Errorf("GetInfo().Name = %q, want %q", result.Name, tt.serverName)
				}
				if result.Version != tt.version {
					t.Errorf("GetInfo().Version = %q, want %q", result.Version, tt.version)
				}
			}
		})
	}
}

func TestGetInfoServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetInfo()
	if err == nil {
		t.Error("GetInfo() should return error for 500 response")
	}
}

func TestGetInfoInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetInfo()
	if err == nil {
		t.Error("GetInfo() should return error for invalid JSON")
	}
}

func TestGetInfoInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetInfo()
	if err == nil {
		t.Error("GetInfo() should return error when data field cannot be decoded")
	}
	if !strings.Contains(err.Error(), "failed to decode info data") {
		t.Errorf("error = %q, want 'failed to decode info data'", err.Error())
	}
}

// Tests for GetRelatedSearches

func TestGetRelatedSearches(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		suggestions []string
	}{
		{"with results", "golang", []string{"go programming", "golang tutorial"}},
		{"empty results", "xyzzy123", []string{}},
		{"single result", "test", []string{"test driven development"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != version.APIPrefix+"/search/related" {
					t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/search/related")
				}
				if r.URL.Query().Get("q") != tt.query {
					t.Errorf("q = %q, want %q", r.URL.Query().Get("q"), tt.query)
				}
				data := map[string]interface{}{"suggestions": tt.suggestions}
				w.Write(apiOKResp(t, data))
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			got, err := client.GetRelatedSearches(tt.query)
			if err != nil {
				t.Fatalf("GetRelatedSearches() error = %v", err)
			}
			if len(got) != len(tt.suggestions) {
				t.Errorf("len(suggestions) = %d, want %d", len(got), len(tt.suggestions))
			}
		})
	}
}

func TestGetRelatedSearchesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetRelatedSearches("test")
	if err == nil {
		t.Error("GetRelatedSearches() should return error for 500")
	}
}

func TestGetRelatedSearchesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetRelatedSearches("test")
	if err == nil {
		t.Error("GetRelatedSearches() should return error for invalid JSON")
	}
}

func TestGetRelatedSearchesInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetRelatedSearches("test")
	if err == nil {
		t.Error("GetRelatedSearches() should return error when data field invalid")
	}
	if !strings.Contains(err.Error(), "failed to decode related searches data") {
		t.Errorf("error = %q, want 'failed to decode related searches data'", err.Error())
	}
}

// Tests for GetAutocomplete

func TestGetAutocomplete(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		suggestions []string
	}{
		{"with results", "go", []string{"golang", "google", "go programming"}},
		{"no results", "xyzzy", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != version.APIPrefix+"/autocomplete" {
					t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/autocomplete")
				}
				if r.URL.Query().Get("q") != tt.query {
					t.Errorf("q = %q, want %q", r.URL.Query().Get("q"), tt.query)
				}
				w.Write(apiOKResp(t, tt.suggestions))
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			got, err := client.GetAutocomplete(tt.query)
			if err != nil {
				t.Fatalf("GetAutocomplete() error = %v", err)
			}
			if len(got) != len(tt.suggestions) {
				t.Errorf("len(got) = %d, want %d", len(got), len(tt.suggestions))
			}
		})
	}
}

func TestGetAutocompleteServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetAutocomplete("test")
	if err == nil {
		t.Error("GetAutocomplete() should return error for 503")
	}
}

func TestGetAutocompleteInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{bad json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetAutocomplete("test")
	if err == nil {
		t.Error("GetAutocomplete() should return error for invalid JSON")
	}
}

func TestGetAutocompleteInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// data is an object, not an array — causes unmarshal to []string to fail
		w.Write([]byte(`{"ok":true,"data":{"not":"array"}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetAutocomplete("test")
	if err == nil {
		t.Error("GetAutocomplete() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode autocomplete data") {
		t.Errorf("error = %q, want 'failed to decode autocomplete data'", err.Error())
	}
}

// Tests for GetEngines

func TestGetEngines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/engines" {
			t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/engines")
		}
		engines := []EngineStatus{
			{ID: "google", Name: "Google", Enabled: true, Priority: 1, Categories: []string{"general"}},
			{ID: "bing", Name: "Bing", Enabled: false, Priority: 2, Categories: []string{"general", "images"}},
		}
		w.Write(apiOKResp(t, engines))
	}))
	defer server.Close()

	client := NewClient(server.URL, "operator-token", 30)
	engines, err := client.GetEngines()
	if err != nil {
		t.Fatalf("GetEngines() error = %v", err)
	}
	if len(engines) != 2 {
		t.Errorf("len(engines) = %d, want 2", len(engines))
	}
	if engines[0].ID != "google" {
		t.Errorf("engines[0].ID = %q, want 'google'", engines[0].ID)
	}
	if !engines[0].Enabled {
		t.Error("engines[0].Enabled should be true")
	}
}

func TestGetEnginesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetEngines()
	if err == nil {
		t.Error("GetEngines() should return error for 500")
	}
}

func TestGetEnginesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bad json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetEngines()
	if err == nil {
		t.Error("GetEngines() should return error for invalid JSON")
	}
}

func TestGetEnginesInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-array"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetEngines()
	if err == nil {
		t.Error("GetEngines() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode engines data") {
		t.Errorf("error = %q, want 'failed to decode engines data'", err.Error())
	}
}

// Tests for GetEngineByID

func TestGetEngineByID(t *testing.T) {
	tests := []struct {
		name     string
		engineID string
		enabled  bool
	}{
		{"enabled engine", "google", true},
		{"disabled engine", "bing", false},
		{"engine with special chars", "duck-go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := version.APIPrefix + "/engines/" + url.PathEscape(tt.engineID)
				if r.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
				}
				engine := EngineStatus{ID: tt.engineID, Name: tt.engineID, Enabled: tt.enabled}
				w.Write(apiOKResp(t, engine))
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			engine, err := client.GetEngineByID(tt.engineID)
			if err != nil {
				t.Fatalf("GetEngineByID(%q) error = %v", tt.engineID, err)
			}
			if engine.ID != tt.engineID {
				t.Errorf("engine.ID = %q, want %q", engine.ID, tt.engineID)
			}
			if engine.Enabled != tt.enabled {
				t.Errorf("engine.Enabled = %v, want %v", engine.Enabled, tt.enabled)
			}
		})
	}
}

func TestGetEngineByIDServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ok":false,"error":"NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetEngineByID("nonexistent")
	if err == nil {
		t.Error("GetEngineByID() should return error for 404")
	}
}

func TestGetEngineByIDInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetEngineByID("google")
	if err == nil {
		t.Error("GetEngineByID() should return error for invalid JSON")
	}
}

func TestGetEngineByIDInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetEngineByID("google")
	if err == nil {
		t.Error("GetEngineByID() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode engine data") {
		t.Errorf("error = %q, want 'failed to decode engine data'", err.Error())
	}
}

// Tests for GetCategories

func TestGetCategories(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/categories" {
			t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/categories")
		}
		cats := []Category{
			{ID: "general", Name: "General", Description: "General web search", Icon: "search"},
			{ID: "images", Name: "Images", Description: "Image search", Icon: "image"},
		}
		w.Write(apiOKResp(t, cats))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	cats, err := client.GetCategories()
	if err != nil {
		t.Fatalf("GetCategories() error = %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("len(cats) = %d, want 2", len(cats))
	}
	if cats[0].ID != "general" {
		t.Errorf("cats[0].ID = %q, want 'general'", cats[0].ID)
	}
}

func TestGetCategoriesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetCategories()
	if err == nil {
		t.Error("GetCategories() should return error for 500")
	}
}

func TestGetCategoriesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{{bad"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetCategories()
	if err == nil {
		t.Error("GetCategories() should return error for invalid JSON")
	}
}

func TestGetCategoriesInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-array"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetCategories()
	if err == nil {
		t.Error("GetCategories() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode categories data") {
		t.Errorf("error = %q, want 'failed to decode categories data'", err.Error())
	}
}

// Tests for GetBangs

func TestGetBangs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/bangs" {
			t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/bangs")
		}
		bangs := []Bang{
			{Shortcut: "!g", Name: "Google", URL: "https://google.com/search?q=%s", Category: "general"},
			{Shortcut: "!gh", Name: "GitHub", URL: "https://github.com/search?q=%s", Category: "code"},
		}
		w.Write(apiOKResp(t, map[string]interface{}{"bangs": bangs}))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	bangs, err := client.GetBangs()
	if err != nil {
		t.Fatalf("GetBangs() error = %v", err)
	}
	if len(bangs) != 2 {
		t.Errorf("len(bangs) = %d, want 2", len(bangs))
	}
	if bangs[0].Shortcut != "!g" {
		t.Errorf("bangs[0].Shortcut = %q, want '!g'", bangs[0].Shortcut)
	}
}

func TestGetBangsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetBangs()
	if err == nil {
		t.Error("GetBangs() should return error for 500")
	}
}

func TestGetBangsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetBangs()
	if err == nil {
		t.Error("GetBangs() should return error for invalid JSON")
	}
}

func TestGetBangsInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetBangs()
	if err == nil {
		t.Error("GetBangs() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode bangs data") {
		t.Errorf("error = %q, want 'failed to decode bangs data'", err.Error())
	}
}

// Tests for GetWidgets

func TestGetWidgets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/widgets" {
			t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/widgets")
		}
		widgets := []Widget{
			{Type: "weather", Name: "Weather", Icon: "cloud", Category: "instant", Order: 1},
			{Type: "calculator", Name: "Calculator", Icon: "calc", Category: "instant", Order: 2},
		}
		w.Write(apiOKResp(t, map[string]interface{}{"widgets": widgets}))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	widgets, err := client.GetWidgets()
	if err != nil {
		t.Fatalf("GetWidgets() error = %v", err)
	}
	if len(widgets) != 2 {
		t.Errorf("len(widgets) = %d, want 2", len(widgets))
	}
	if widgets[0].Type != "weather" {
		t.Errorf("widgets[0].Type = %q, want 'weather'", widgets[0].Type)
	}
}

func TestGetWidgetsServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetWidgets()
	if err == nil {
		t.Error("GetWidgets() should return error for 500")
	}
}

func TestGetWidgetsInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetWidgets()
	if err == nil {
		t.Error("GetWidgets() should return error for invalid JSON")
	}
}

func TestGetWidgetsInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetWidgets()
	if err == nil {
		t.Error("GetWidgets() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode widgets data") {
		t.Errorf("error = %q, want 'failed to decode widgets data'", err.Error())
	}
}

// Tests for GetWidgetData

func TestGetWidgetData(t *testing.T) {
	tests := []struct {
		name       string
		widgetName string
		params     url.Values
	}{
		{"no params", "weather", nil},
		{"with params", "weather", url.Values{"location": []string{"New York"}}},
		{"empty params", "calculator", url.Values{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := version.APIPrefix + "/widgets/" + url.PathEscape(tt.widgetName)
				if r.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
				}
				wd := WidgetData{Type: tt.widgetName, UpdatedAt: "2024-01-01T00:00:00Z"}
				w.Write(apiOKResp(t, wd))
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			result, err := client.GetWidgetData(tt.widgetName, tt.params)
			if err != nil {
				t.Fatalf("GetWidgetData(%q) error = %v", tt.widgetName, err)
			}
			if result.Type != tt.widgetName {
				t.Errorf("result.Type = %q, want %q", result.Type, tt.widgetName)
			}
		})
	}
}

func TestGetWidgetDataServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ok":false,"error":"NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetWidgetData("nonexistent", nil)
	if err == nil {
		t.Error("GetWidgetData() should return error for 404")
	}
}

func TestGetWidgetDataInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bad json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetWidgetData("weather", nil)
	if err == nil {
		t.Error("GetWidgetData() should return error for invalid JSON")
	}
}

func TestGetWidgetDataInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetWidgetData("weather", nil)
	if err == nil {
		t.Error("GetWidgetData() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode widget data") {
		t.Errorf("error = %q, want 'failed to decode widget data'", err.Error())
	}
}

// Tests for GetInstantAnswer

func TestGetInstantAnswer(t *testing.T) {
	tests := []struct {
		name  string
		query string
		found bool
		itype string
	}{
		{"found answer", "2+2", true, "calculator"},
		{"not found", "gibberish query xyz", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != version.APIPrefix+"/instant" {
					t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/instant")
				}
				if r.URL.Query().Get("q") != tt.query {
					t.Errorf("q = %q, want %q", r.URL.Query().Get("q"), tt.query)
				}
				ans := InstantAnswer{Query: tt.query, Found: tt.found, Type: tt.itype}
				w.Write(apiOKResp(t, ans))
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			result, err := client.GetInstantAnswer(tt.query)
			if err != nil {
				t.Fatalf("GetInstantAnswer(%q) error = %v", tt.query, err)
			}
			if result.Found != tt.found {
				t.Errorf("result.Found = %v, want %v", result.Found, tt.found)
			}
			if result.Query != tt.query {
				t.Errorf("result.Query = %q, want %q", result.Query, tt.query)
			}
		})
	}
}

func TestGetInstantAnswerServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetInstantAnswer("test")
	if err == nil {
		t.Error("GetInstantAnswer() should return error for 500")
	}
}

func TestGetInstantAnswerInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetInstantAnswer("test")
	if err == nil {
		t.Error("GetInstantAnswer() should return error for invalid JSON")
	}
}

func TestGetInstantAnswerInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetInstantAnswer("test")
	if err == nil {
		t.Error("GetInstantAnswer() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode instant answer data") {
		t.Errorf("error = %q, want 'failed to decode instant answer data'", err.Error())
	}
}

// Tests for GetDirectAnswer

func TestGetDirectAnswer(t *testing.T) {
	tests := []struct {
		name  string
		slug  string
		found bool
		title string
	}{
		{"found", "golang", true, "Go (programming language)"},
		{"not found", "nonexistent-slug-xyz", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				wantPath := version.APIPrefix + "/direct/" + tt.slug
				if r.URL.Path != wantPath {
					t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
				}
				ans := DirectAnswer{Term: tt.slug, Found: tt.found, Title: tt.title, Type: "wiki", Content: ""}
				w.Write(apiOKResp(t, ans))
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			result, err := client.GetDirectAnswer(tt.slug)
			if err != nil {
				t.Fatalf("GetDirectAnswer(%q) error = %v", tt.slug, err)
			}
			if result.Found != tt.found {
				t.Errorf("result.Found = %v, want %v", result.Found, tt.found)
			}
		})
	}
}

func TestGetDirectAnswerServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ok":false,"error":"NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetDirectAnswer("nonexistent")
	if err == nil {
		t.Error("GetDirectAnswer() should return error for 404")
	}
}

func TestGetDirectAnswerInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("bad json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetDirectAnswer("golang")
	if err == nil {
		t.Error("GetDirectAnswer() should return error for invalid JSON")
	}
}

func TestGetDirectAnswerInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetDirectAnswer("golang")
	if err == nil {
		t.Error("GetDirectAnswer() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode direct answer data") {
		t.Errorf("error = %q, want 'failed to decode direct answer data'", err.Error())
	}
}

// Tests for GetPreferences

func TestGetPreferences(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/preferences" {
			t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/preferences")
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		prefs := Preferences{Storage: "local", Fields: []string{"theme", "language"}}
		w.Write(apiOKResp(t, prefs))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	prefs, err := client.GetPreferences()
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if prefs.Storage != "local" {
		t.Errorf("prefs.Storage = %q, want 'local'", prefs.Storage)
	}
	if len(prefs.Fields) != 2 {
		t.Errorf("len(prefs.Fields) = %d, want 2", len(prefs.Fields))
	}
}

func TestGetPreferencesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetPreferences()
	if err == nil {
		t.Error("GetPreferences() should return error for 500")
	}
}

func TestGetPreferencesInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetPreferences()
	if err == nil {
		t.Error("GetPreferences() should return error for invalid JSON")
	}
}

func TestGetPreferencesInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetPreferences()
	if err == nil {
		t.Error("GetPreferences() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode preferences data") {
		t.Errorf("error = %q, want 'failed to decode preferences data'", err.Error())
	}
}

// Tests for SetPreferences

func TestSetPreferences(t *testing.T) {
	tests := []struct {
		name  string
		prefs *Preferences
	}{
		{"with fields", &Preferences{Storage: "local", Fields: []string{"theme"}}},
		{"empty preferences", &Preferences{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != version.APIPrefix+"/preferences" {
					t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/preferences")
				}
				if r.Method != http.MethodPut {
					t.Errorf("method = %q, want PUT", r.Method)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			client := NewClient(server.URL, "", 30)
			err := client.SetPreferences(tt.prefs)
			if err != nil {
				t.Fatalf("SetPreferences() error = %v", err)
			}
		})
	}
}

func TestSetPreferencesServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	err := client.SetPreferences(&Preferences{Storage: "local"})
	if err == nil {
		t.Error("SetPreferences() should return error for 500")
	}
}

// Tests for CreateAlert

func TestCreateAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != version.APIPrefix+"/alerts" {
			t.Errorf("path = %q, want %q", r.URL.Path, version.APIPrefix+"/alerts")
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		alert := Alert{ManageToken: "manage-tok-123", ManageURL: "https://example.com/alerts/manage-tok-123"}
		w.WriteHeader(http.StatusCreated)
		w.Write(apiOKResp(t, alert))
	}))
	defer server.Close()

	req := &CreateAlertRequest{
		Query:      "golang",
		Category:   "general",
		Language:   "en",
		Email:      "user@example.com",
		DeliverRSS: true,
		Frequency:  "daily",
	}

	client := NewClient(server.URL, "", 30)
	result, err := client.CreateAlert(req)
	if err != nil {
		t.Fatalf("CreateAlert() error = %v", err)
	}
	if result.ManageToken != "manage-tok-123" {
		t.Errorf("result.ManageToken = %q, want 'manage-tok-123'", result.ManageToken)
	}
}

func TestCreateAlertServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"ok":false,"error":"INVALID_EMAIL"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.CreateAlert(&CreateAlertRequest{Query: "test"})
	if err == nil {
		t.Error("CreateAlert() should return error for 400")
	}
}

func TestCreateAlertInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.CreateAlert(&CreateAlertRequest{Query: "test"})
	if err == nil {
		t.Error("CreateAlert() should return error for invalid JSON")
	}
}

func TestCreateAlertInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.CreateAlert(&CreateAlertRequest{Query: "test"})
	if err == nil {
		t.Error("CreateAlert() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode alert data") {
		t.Errorf("error = %q, want 'failed to decode alert data'", err.Error())
	}
}

// Tests for GetAlert

func TestGetAlert(t *testing.T) {
	const manageToken = "manage-token-abc"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := version.APIPrefix + "/alerts/" + url.PathEscape(manageToken)
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method = %q, want GET", r.Method)
		}
		alert := Alert{ManageToken: manageToken, RSSURL: "https://example.com/rss/123"}
		w.Write(apiOKResp(t, alert))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	result, err := client.GetAlert(manageToken)
	if err != nil {
		t.Fatalf("GetAlert() error = %v", err)
	}
	if result.ManageToken != manageToken {
		t.Errorf("result.ManageToken = %q, want %q", result.ManageToken, manageToken)
	}
}

func TestGetAlertServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ok":false,"error":"NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetAlert("nonexistent-token")
	if err == nil {
		t.Error("GetAlert() should return error for 404")
	}
}

func TestGetAlertInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetAlert("token")
	if err == nil {
		t.Error("GetAlert() should return error for invalid JSON")
	}
}

func TestGetAlertInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetAlert("token")
	if err == nil {
		t.Error("GetAlert() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode alert data") {
		t.Errorf("error = %q, want 'failed to decode alert data'", err.Error())
	}
}

// Tests for UpdateAlert

func TestUpdateAlert(t *testing.T) {
	const manageToken = "update-token-xyz"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := version.APIPrefix + "/alerts/" + url.PathEscape(manageToken)
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodPatch {
			t.Errorf("method = %q, want PATCH", r.Method)
		}
		alert := Alert{ManageToken: manageToken}
		w.Write(apiOKResp(t, alert))
	}))
	defer server.Close()

	req := &UpdateAlertRequest{
		Query:     "updated query",
		Frequency: "weekly",
	}
	client := NewClient(server.URL, "", 30)
	result, err := client.UpdateAlert(manageToken, req)
	if err != nil {
		t.Fatalf("UpdateAlert() error = %v", err)
	}
	if result.ManageToken != manageToken {
		t.Errorf("result.ManageToken = %q, want %q", result.ManageToken, manageToken)
	}
}

func TestUpdateAlertServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ok":false,"error":"NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.UpdateAlert("nonexistent", &UpdateAlertRequest{})
	if err == nil {
		t.Error("UpdateAlert() should return error for 404")
	}
}

func TestUpdateAlertInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.UpdateAlert("token", &UpdateAlertRequest{})
	if err == nil {
		t.Error("UpdateAlert() should return error for invalid JSON")
	}
}

func TestUpdateAlertInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.UpdateAlert("token", &UpdateAlertRequest{})
	if err == nil {
		t.Error("UpdateAlert() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode alert data") {
		t.Errorf("error = %q, want 'failed to decode alert data'", err.Error())
	}
}

// Tests for DeleteAlert

func TestDeleteAlert(t *testing.T) {
	const manageToken = "delete-token-abc"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := version.APIPrefix + "/alerts/" + url.PathEscape(manageToken)
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("method = %q, want DELETE", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	err := client.DeleteAlert(manageToken)
	if err != nil {
		t.Fatalf("DeleteAlert() error = %v", err)
	}
}

func TestDeleteAlertServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"ok":false,"error":"NOT_FOUND"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	err := client.DeleteAlert("nonexistent-token")
	if err == nil {
		t.Error("DeleteAlert() should return error for 404")
	}
}

// Tests for GetStatus

func TestGetStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server/status" {
			t.Errorf("path = %q, want '/server/status'", r.URL.Path)
		}
		// Operator token should be present
		auth := r.Header.Get("Authorization")
		if auth != "Bearer operator-token" {
			t.Errorf("Authorization = %q, want 'Bearer operator-token'", auth)
		}
		status := StatusResponse{
			Status:  "ok",
			Version: "1.0.0",
			Mode:    "production",
			Uptime:  "1d2h",
			Checks:  map[string]string{"database": "ok"},
		}
		w.Write(apiOKResp(t, status))
	}))
	defer server.Close()

	client := NewClient(server.URL, "operator-token", 30)
	result, err := client.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("result.Status = %q, want 'ok'", result.Status)
	}
	if result.Version != "1.0.0" {
		t.Errorf("result.Version = %q, want '1.0.0'", result.Version)
	}
}

func TestGetStatusUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"error":"UNAUTHORIZED"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetStatus()
	if err == nil {
		t.Error("GetStatus() should return error for 401")
	}
}

func TestGetStatusInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.GetStatus()
	if err == nil {
		t.Error("GetStatus() should return error for invalid JSON")
	}
}

func TestGetStatusInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.GetStatus()
	if err == nil {
		t.Error("GetStatus() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode status data") {
		t.Errorf("error = %q, want 'failed to decode status data'", err.Error())
	}
}

// Tests for GetServerConfig

func TestGetServerConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server/config" {
			t.Errorf("path = %q, want '/server/config'", r.URL.Path)
		}
		cfg := ServerConfigResponse{
			Config: map[string]interface{}{
				"server.port": 8080,
				"server.mode": "production",
			},
		}
		w.Write(apiOKResp(t, cfg))
	}))
	defer server.Close()

	client := NewClient(server.URL, "operator-token", 30)
	result, err := client.GetServerConfig()
	if err != nil {
		t.Fatalf("GetServerConfig() error = %v", err)
	}
	if result.Config == nil {
		t.Error("result.Config should not be nil")
	}
	if len(result.Config) != 2 {
		t.Errorf("len(result.Config) = %d, want 2", len(result.Config))
	}
}

func TestGetServerConfigUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"error":"UNAUTHORIZED"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetServerConfig()
	if err == nil {
		t.Error("GetServerConfig() should return error for 401")
	}
}

func TestGetServerConfigInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.GetServerConfig()
	if err == nil {
		t.Error("GetServerConfig() should return error for invalid JSON")
	}
}

func TestGetServerConfigInvalidDataJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true,"data":"not-an-object"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", 30)
	_, err := client.GetServerConfig()
	if err == nil {
		t.Error("GetServerConfig() should return error when data is wrong type")
	}
	if !strings.Contains(err.Error(), "failed to decode server config data") {
		t.Errorf("error = %q, want 'failed to decode server config data'", err.Error())
	}
}

// Tests for GetMetrics

func TestGetMetrics(t *testing.T) {
	metricsBody := "# HELP search_requests_total Total HTTP requests\n# TYPE search_requests_total counter\nsearch_requests_total 1234\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/server/metrics" {
			t.Errorf("path = %q, want '/server/metrics'", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Write([]byte(metricsBody))
	}))
	defer server.Close()

	client := NewClient(server.URL, "operator-token", 30)
	result, err := client.GetMetrics()
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if result != metricsBody {
		t.Errorf("GetMetrics() = %q, want %q", result, metricsBody)
	}
}

func TestGetMetricsUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"ok":false,"error":"UNAUTHORIZED"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "", 30)
	_, err := client.GetMetrics()
	if err == nil {
		t.Error("GetMetrics() should return error for 401")
	}
}

func TestGetMetricsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(server.URL, "operator-token", 30)
	result, err := client.GetMetrics()
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if result != "" {
		t.Errorf("GetMetrics() = %q, want empty string", result)
	}
}

// TestGetMetricsBodyReadError covers the io.ReadAll error path in GetMetrics.
// It uses HTTP hijacking to close the connection after headers are sent but before
// the body is written, causing the client's io.ReadAll to return "unexpected EOF".
func TestGetMetricsBodyReadError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Announce Content-Length larger than what we'll actually send, then hijack
		// the connection and drop it. This forces an "unexpected EOF" on the client.
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.Header().Set("Content-Length", "10000")
		w.WriteHeader(http.StatusOK)

		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("ResponseWriter does not implement http.Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("Hijack() error = %v", err)
			return
		}
		// Close the raw connection immediately — client will get unexpected EOF.
		conn.(*net.TCPConn).SetLinger(0)
		conn.Close()
	}))
	defer server.Close()

	client := NewClient(server.URL, "operator-token", 30)
	_, err := client.GetMetrics()
	if err == nil {
		t.Error("GetMetrics() should return error when body read fails (connection dropped)")
	}
}
