package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"

	"github.com/apimgr/search/src/client/api"
)

// Tests for statusCmd

func TestStatusCmdUse(t *testing.T) {
	if statusCmd.Use != "status" {
		t.Errorf("statusCmd.Use = %q, want 'status'", statusCmd.Use)
	}
}

func TestStatusCmdShort(t *testing.T) {
	if statusCmd.Short == "" {
		t.Error("statusCmd.Short should not be empty")
	}
}

func TestStatusCmdLong(t *testing.T) {
	if statusCmd.Long == "" {
		t.Error("statusCmd.Long should not be empty")
	}
}

// Test status command is registered

func TestStatusCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "status" {
			found = true
			break
		}
	}

	if !found {
		t.Error("status command should be registered with rootCmd")
	}
}

// Tests for runStatus - note: these tests need to handle os.Exit behavior

func TestRunStatusHealthy(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/healthz" {
			json.NewEncoder(w).Encode(api.HealthResponse{
				Status:  "healthy",
				Version: "1.0.0",
				Uptime:  "24h",
				Checks: map[string]string{
					"database": "ok",
					"cache":    "ok",
				},
			})
		} else {
			json.NewEncoder(w).Encode(map[string]interface{}{})
		}
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	// Note: runStatus calls os.Exit on unhealthy, so we need to be careful
	// For healthy status, it should not exit
	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() healthy error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusHealthyJSON(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "ok",
			Version: "2.0.0",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "json"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() JSON error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusWithUptime(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
			Uptime:  "72h30m",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() with uptime error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusWithChecks(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
			Checks: map[string]string{
				"database": "healthy",
				"cache":    "healthy",
				"search":   "healthy",
			},
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() with checks error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusNoVersion(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status: "ok",
			// No version
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() no version error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusNoUptime(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
			// No uptime
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() no uptime error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusNoChecks(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
			// No checks
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() no checks error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusInitClientError(t *testing.T) {
	viper.Reset()
	server = ""
	apiClient = nil

	err := runStatus()

	if err == nil {
		t.Error("runStatus() should return error when no server configured")
	}
}

// Tests for statusCmd.RunE

func TestStatusCmdRunE(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := statusCmd.RunE(statusCmd, []string{})

	if err != nil {
		t.Fatalf("statusCmd.RunE() error = %v", err)
	}

	server = ""
	output = ""
}

// Tests for output format variations

func TestRunStatusOutputFormats(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:    "healthy",
			Version:   "1.0.0",
			Uptime:    "1h",
			Timestamp: "2024-01-01T00:00:00Z",
			Checks: map[string]string{
				"db": "ok",
			},
		})
	}))
	defer testServer.Close()

	formats := []string{"table", "json", "plain"}

	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			viper.Reset()
			server = testServer.URL
			apiClient = nil
			output = format

			err := runStatus()

			if err != nil {
				t.Fatalf("runStatus() with format %q error = %v", format, err)
			}
		})
	}

	server = ""
	output = ""
}

// Tests for response time measurement

func TestRunStatusResponseTime(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status: "healthy",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	// Response time is measured but not directly testable without capturing output
	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() error = %v", err)
	}

	server = ""
	output = ""
}

// Tests for status values

func TestRunStatusOkStatus(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status: "ok",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() with 'ok' status error = %v", err)
	}

	server = ""
	output = ""
}

func TestRunStatusHealthyStatus(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status: "healthy",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() with 'healthy' status error = %v", err)
	}

	server = ""
	output = ""
}

// Tests for existing client

func TestRunStatusExistingClient(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status: "healthy",
		})
	}))
	defer testServer.Close()

	viper.Reset()
	// Pre-initialize client
	apiClient = api.NewClient(testServer.URL, "", 30)
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() with existing client error = %v", err)
	}

	apiClient = nil
	output = ""
}

// Tests for multiple health checks display

func TestRunStatusMultipleChecks(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
			Checks: map[string]string{
				"database":    "connected",
				"cache":       "ready",
				"search":      "indexing",
				"email":       "configured",
				"storage":     "available",
				"scheduler":   "running",
				"auth":        "enabled",
			},
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "table"

	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() with multiple checks error = %v", err)
	}

	server = ""
	output = ""
}

// Tests for JSON output structure

func TestRunStatusJSONOutput(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.HealthResponse{
			Status:  "healthy",
			Version: "1.0.0",
			Uptime:  "1h30m",
			Checks: map[string]string{
				"db": "ok",
			},
		})
	}))
	defer testServer.Close()

	viper.Reset()
	server = testServer.URL
	apiClient = nil
	output = "json"

	// JSON output includes response_time, status, version, uptime, checks
	err := runStatus()

	if err != nil {
		t.Fatalf("runStatus() JSON output error = %v", err)
	}

	server = ""
	output = ""
}
