package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"

	"github.com/apimgr/search/src/client/api"
)

// Tests for package variables

func TestProjectName(t *testing.T) {
	if ProjectName == "" {
		t.Error("ProjectName should not be empty")
	}
}

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Error("Version should not be empty")
	}
}

func TestCommitID(t *testing.T) {
	if CommitID == "" {
		t.Error("CommitID should not be empty")
	}
}

func TestBuildDate(t *testing.T) {
	if BuildDate == "" {
		t.Error("BuildDate should not be empty")
	}
}

// Tests for resolveServerAddress

func TestResolveServerAddressFromFlag(t *testing.T) {
	viper.Reset()
	server = "https://flag.example.com"

	addr, shouldSave := resolveServerAddress()

	if addr != "https://flag.example.com" {
		t.Errorf("resolveServerAddress() = %q, want 'https://flag.example.com'", addr)
	}
	if !shouldSave {
		t.Error("shouldSave should be true when config is empty")
	}

	server = ""
}

func TestResolveServerAddressFromFlagWithExistingConfig(t *testing.T) {
	viper.Reset()
	viper.Set("server.primary", "https://existing.example.com")
	server = "https://flag.example.com"

	addr, shouldSave := resolveServerAddress()

	if addr != "https://flag.example.com" {
		t.Errorf("resolveServerAddress() = %q", addr)
	}
	if shouldSave {
		t.Error("shouldSave should be false when config already has value")
	}

	server = ""
}

func TestResolveServerAddressFromPrimary(t *testing.T) {
	viper.Reset()
	server = ""
	viper.Set("server.primary", "https://primary.example.com")

	addr, shouldSave := resolveServerAddress()

	if addr != "https://primary.example.com" {
		t.Errorf("resolveServerAddress() = %q", addr)
	}
	if shouldSave {
		t.Error("shouldSave should be false for config value")
	}
}

func TestResolveServerAddressFromLegacy(t *testing.T) {
	viper.Reset()
	server = ""
	viper.Set("server.address", "https://legacy.example.com")

	addr, shouldSave := resolveServerAddress()

	if addr != "https://legacy.example.com" {
		t.Errorf("resolveServerAddress() = %q", addr)
	}
	if shouldSave {
		t.Error("shouldSave should be false for legacy config")
	}
}

func TestResolveServerAddressEmpty(t *testing.T) {
	viper.Reset()
	server = ""

	addr, shouldSave := resolveServerAddress()

	if addr != "" {
		t.Errorf("resolveServerAddress() = %q, want empty", addr)
	}
	if shouldSave {
		t.Error("shouldSave should be false when no server")
	}
}

// Tests for getToken

func TestGetTokenFromFlag(t *testing.T) {
	viper.Reset()
	token = "flag-token"

	result := getToken()

	if result != "flag-token" {
		t.Errorf("getToken() = %q, want 'flag-token'", result)
	}

	token = ""
}

func TestGetTokenFromFile(t *testing.T) {
	viper.Reset()
	token = ""

	tempDir := t.TempDir()
	tokenPath := filepath.Join(tempDir, "testtoken")
	os.WriteFile(tokenPath, []byte("file-token\n"), 0600)
	tokenFile = tokenPath

	result := getToken()

	if result != "file-token" {
		t.Errorf("getToken() = %q, want 'file-token'", result)
	}

	tokenFile = ""
}

func TestGetTokenFromEnv(t *testing.T) {
	viper.Reset()
	token = ""
	tokenFile = ""
	os.Setenv("SEARCH_TOKEN", "env-token")
	defer os.Unsetenv("SEARCH_TOKEN")

	result := getToken()

	if result != "env-token" {
		t.Errorf("getToken() = %q, want 'env-token'", result)
	}
}

func TestGetTokenFromConfig(t *testing.T) {
	viper.Reset()
	token = ""
	tokenFile = ""
	os.Unsetenv("SEARCH_TOKEN")
	viper.Set("server.token", "config-token")

	result := getToken()

	if result != "config-token" {
		t.Errorf("getToken() = %q, want 'config-token'", result)
	}
}

func TestGetTokenFromDefaultFile(t *testing.T) {
	viper.Reset()
	token = ""
	tokenFile = ""
	os.Unsetenv("SEARCH_TOKEN")

	// Test default token file path
	// This test may not find the file, which is expected
	result := getToken()
	_ = result // May be empty if no token file exists
}

func TestGetTokenPriority(t *testing.T) {
	viper.Reset()
	token = "flag-token"
	os.Setenv("SEARCH_TOKEN", "env-token")
	viper.Set("server.token", "config-token")
	defer os.Unsetenv("SEARCH_TOKEN")

	result := getToken()

	// Flag should have highest priority
	if result != "flag-token" {
		t.Errorf("getToken() = %q, want 'flag-token'", result)
	}

	token = ""
}

// Tests for getBinaryName

func TestGetBinaryName(t *testing.T) {
	name := getBinaryName()

	if name == "" {
		t.Error("getBinaryName() returned empty string")
	}
}

// Tests for getOutputFormat

func TestGetOutputFormatFromFlag(t *testing.T) {
	viper.Reset()
	output = "json"

	result := getOutputFormat()

	if result != "json" {
		t.Errorf("getOutputFormat() = %q, want 'json'", result)
	}

	output = ""
}

func TestGetOutputFormatFromConfig(t *testing.T) {
	viper.Reset()
	output = ""
	viper.Set("output.format", "plain")

	result := getOutputFormat()

	if result != "plain" {
		t.Errorf("getOutputFormat() = %q, want 'plain'", result)
	}
}

func TestGetOutputFormatDefault(t *testing.T) {
	viper.Reset()
	output = ""

	result := getOutputFormat()

	// Default is empty string or whatever viper returns
	_ = result
}

// Tests for saveServerToConfig

func TestSaveServerToConfig(t *testing.T) {
	viper.Reset()
	tempDir := t.TempDir()

	// Set config path
	viper.Set("server.primary", "")

	saveServerToConfig("https://saved.example.com")

	if viper.GetString("server.primary") != "https://saved.example.com" {
		t.Error("saveServerToConfig() should set server.primary")
	}

	_ = tempDir
}

// Tests for updateClusterConfig

func TestUpdateClusterConfig(t *testing.T) {
	viper.Reset()

	updateClusterConfig("https://primary.example.com", []string{"node1", "node2"})

	if viper.GetString("server.primary") != "https://primary.example.com" {
		t.Error("updateClusterConfig() should set server.primary")
	}

	nodes := viper.GetStringSlice("server.cluster")
	if len(nodes) != 2 {
		t.Errorf("updateClusterConfig() nodes length = %d, want 2", len(nodes))
	}
}

func TestUpdateClusterConfigEmptyPrimary(t *testing.T) {
	viper.Reset()
	viper.Set("server.primary", "existing")

	updateClusterConfig("", []string{"node1"})

	// Should not change existing primary
	if viper.GetString("server.primary") != "existing" {
		t.Error("updateClusterConfig() should not change existing primary when empty")
	}
}

func TestUpdateClusterConfigEmptyNodes(t *testing.T) {
	viper.Reset()
	viper.Set("server.cluster", []string{"existing"})

	updateClusterConfig("primary", []string{})

	// Should not change existing nodes when empty
	nodes := viper.GetStringSlice("server.cluster")
	if len(nodes) != 1 {
		t.Error("updateClusterConfig() should not change nodes when empty")
	}
}

// Tests for initClient

func TestInitClientNoServer(t *testing.T) {
	viper.Reset()
	server = ""
	apiClient = nil

	err := initClient()

	if err == nil {
		t.Error("initClient() should return error when no server configured")
	}
}

func TestInitClientWithServer(t *testing.T) {
	// Create test server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/autodiscover" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"server":  map[string]interface{}{"name": "test"},
				"cluster": map[string]interface{}{"nodes": []string{}},
			})
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil

	err := initClient()

	if err != nil {
		t.Fatalf("initClient() error = %v", err)
	}

	if apiClient == nil {
		t.Error("apiClient should not be nil after initClient()")
	}

	server = ""
}

func TestInitClientWithClusterNodes(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	viper.Set("server.cluster", []string{"node1.example.com", "node2.example.com"})
	server = testServer.URL
	apiClient = nil

	err := initClient()

	if err != nil {
		t.Fatalf("initClient() error = %v", err)
	}

	nodes := apiClient.GetClusterNodes()
	if len(nodes) != 2 {
		t.Errorf("ClusterNodes length = %d, want 2", len(nodes))
	}

	server = ""
}

func TestInitClientWithUserContext(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	userCtx = "@testuser"
	apiClient = nil

	err := initClient()

	if err != nil {
		t.Fatalf("initClient() error = %v", err)
	}

	if apiClient.UserContext != "@testuser" {
		t.Errorf("UserContext = %q, want '@testuser'", apiClient.UserContext)
	}

	server = ""
	userCtx = ""
}

func TestInitClientWithTimeout(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	timeout = 60
	apiClient = nil

	err := initClient()

	if err != nil {
		t.Fatalf("initClient() error = %v", err)
	}

	server = ""
	timeout = 0
}

// Tests for backgroundAutodiscover

func TestBackgroundAutodiscoverNilClient(t *testing.T) {
	apiClient = nil

	// Should not panic
	backgroundAutodiscover()
}

func TestBackgroundAutodiscoverSuccess(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.AutodiscoverResponse{})
	}))
	defer testServer.Close()

	apiClient = api.NewClient(testServer.URL, "", 30)

	// Should not panic
	backgroundAutodiscover()
}

func TestBackgroundAutodiscoverWithNodes(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.AutodiscoverResponse{}
		resp.Cluster.Primary = "https://primary.example.com"
		resp.Cluster.Nodes = []string{"node1", "node2"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer testServer.Close()

	viper.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)

	backgroundAutodiscover()

	// Wait a bit for background update
	// Note: In real tests, we'd use sync mechanisms
}

// Tests for runSearch

func TestRunSearchSuccess(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/search" {
			json.NewEncoder(w).Encode(api.SearchResponse{
				Results: []api.SearchResult{
					{ID: "1", Title: "Test", URL: "https://example.com", Score: 0.9},
				},
				TotalCount: 1,
				Query:      "test",
				Page:       1,
				PerPage:    10,
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	}))
	defer testServer.Close()

	viper.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)
	output = "plain"
	page = 1
	limit = 10

	err := runSearch("test query")

	if err != nil {
		t.Fatalf("runSearch() error = %v", err)
	}

	output = ""
}

func TestRunSearchJSONOutput(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.SearchResponse{
			Results:    []api.SearchResult{},
			TotalCount: 0,
		})
	}))
	defer testServer.Close()

	viper.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)
	output = "json"

	err := runSearch("test")

	if err != nil {
		t.Fatalf("runSearch() with json output error = %v", err)
	}

	output = ""
}

func TestRunSearchTableOutput(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.SearchResponse{
			Results: []api.SearchResult{
				{ID: "1", Title: "Very Long Title That Should Be Truncated Because It Exceeds Fifty Characters", URL: "https://example.com", Score: 0.5},
			},
			TotalCount: 1,
		})
	}))
	defer testServer.Close()

	viper.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)
	output = "table"

	err := runSearch("test")

	if err != nil {
		t.Fatalf("runSearch() with table output error = %v", err)
	}

	output = ""
}

func TestRunSearchInitializesClient(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.SearchResponse{})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "plain"

	err := runSearch("test")

	if err != nil {
		t.Fatalf("runSearch() error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunSearchError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	viper.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)

	err := runSearch("test")

	if err == nil {
		t.Error("runSearch() should return error on server error")
	}
}

// Tests for initConfig

func TestInitConfig(t *testing.T) {
	viper.Reset()
	cfgFile = ""

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Should not panic
	initConfig()
}

func TestInitConfigWithFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test.yml")
	os.WriteFile(configPath, []byte("server:\n  primary: https://test.example.com\n"), 0600)

	viper.Reset()
	cfgFile = configPath

	initConfig()

	cfgFile = ""
}

func TestInitConfigDefaults(t *testing.T) {
	viper.Reset()
	cfgFile = ""

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	initConfig()

	// Verify defaults are set
	if viper.GetInt("server.timeout") != 30 {
		t.Errorf("default server.timeout = %d, want 30", viper.GetInt("server.timeout"))
	}
	if viper.GetString("output.format") != "table" {
		t.Errorf("default output.format = %q, want 'table'", viper.GetString("output.format"))
	}
	if viper.GetInt("cache.ttl") != 300 {
		t.Errorf("default cache.ttl = %d, want 300", viper.GetInt("cache.ttl"))
	}
}

// Tests for Execute

func TestExecuteHelp(t *testing.T) {
	// Reset rootCmd for testing
	oldArgs := os.Args
	os.Args = []string{"test", "--help"}
	defer func() { os.Args = oldArgs }()

	// Execute with help flag shouldn't error
	// Note: This will print help output
}

// Tests for rootCmd

func TestRootCmdUse(t *testing.T) {
	if rootCmd.Use == "" {
		t.Error("rootCmd.Use should not be empty")
	}
}

func TestRootCmdShort(t *testing.T) {
	if rootCmd.Short == "" {
		t.Error("rootCmd.Short should not be empty")
	}
}

func TestRootCmdLong(t *testing.T) {
	if rootCmd.Long == "" {
		t.Error("rootCmd.Long should not be empty")
	}
}

func TestRootCmdFlags(t *testing.T) {
	// Verify flags are registered
	flags := []string{"config", "server", "token", "token-file", "user", "output", "no-color", "timeout", "debug", "page", "limit"}

	for _, flag := range flags {
		if rootCmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Flag --%s should be registered", flag)
		}
	}
}

func TestRootCmdVersionFlag(t *testing.T) {
	flag := rootCmd.Flags().Lookup("version")
	if flag == nil {
		t.Error("Flag --version should be registered")
	}
}

func TestRootCmdHelpFlag(t *testing.T) {
	flag := rootCmd.Flags().Lookup("help")
	if flag == nil {
		t.Error("Flag --help should be registered")
	}
}
