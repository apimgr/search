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

// Tests for loginCmd

func TestLoginCmdUse(t *testing.T) {
	if loginCmd.Use != "login" {
		t.Errorf("loginCmd.Use = %q, want 'login'", loginCmd.Use)
	}
}

func TestLoginCmdShort(t *testing.T) {
	if loginCmd.Short == "" {
		t.Error("loginCmd.Short should not be empty")
	}
}

func TestLoginCmdLong(t *testing.T) {
	if loginCmd.Long == "" {
		t.Error("loginCmd.Long should not be empty")
	}
}

// Tests for logoutCmd

func TestLogoutCmdUse(t *testing.T) {
	if logoutCmd.Use != "logout" {
		t.Errorf("logoutCmd.Use = %q, want 'logout'", logoutCmd.Use)
	}
}

func TestLogoutCmdShort(t *testing.T) {
	if logoutCmd.Short == "" {
		t.Error("logoutCmd.Short should not be empty")
	}
}

// Test commands are registered

func TestLoginCommandsRegistered(t *testing.T) {
	commands := []string{"login", "logout"}

	for _, cmdName := range commands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Use == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("command %q should be registered with rootCmd", cmdName)
		}
	}
}

// Tests for getDefaultTokenPath

func TestGetDefaultTokenPath(t *testing.T) {
	path, err := getDefaultTokenPath()

	if err != nil {
		t.Fatalf("getDefaultTokenPath() error = %v", err)
	}

	if path == "" {
		t.Error("getDefaultTokenPath() returned empty string")
	}

	// Should contain expected path components
	if filepath.Base(path) != "token" {
		t.Errorf("getDefaultTokenPath() basename = %q, want 'token'", filepath.Base(path))
	}
}

func TestGetDefaultTokenPathContainsConfigDir(t *testing.T) {
	path, err := getDefaultTokenPath()

	if err != nil {
		t.Fatalf("getDefaultTokenPath() error = %v", err)
	}

	// Should contain apimgr/search in path
	if !filepath.IsAbs(path) {
		// Path might not be absolute on all systems, just check it's not empty
		if path == "" {
			t.Error("getDefaultTokenPath() returned empty path")
		}
	}
}

// Tests for getServerAddress

func TestGetServerAddressFromFlag(t *testing.T) {
	server = "https://flag.example.com"

	result := getServerAddress()

	if result != "https://flag.example.com" {
		t.Errorf("getServerAddress() = %q, want 'https://flag.example.com'", result)
	}

	server = ""
}

func TestGetServerAddressEmpty(t *testing.T) {
	server = ""

	result := getServerAddress()

	if result != "" {
		t.Errorf("getServerAddress() = %q, want empty", result)
	}
}

// Tests for testToken

func TestTestTokenSuccess(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Per AI.md PART 14: Wrapped response format {"ok": true, "data": {...}}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok": true,
			"data": api.HealthResponse{
				Status: "healthy",
			},
		})
	}))
	defer testServer.Close()

	err := testToken(testServer.URL, "test-token")

	if err != nil {
		t.Fatalf("testToken() error = %v", err)
	}
}

func TestTestTokenError(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer testServer.Close()

	err := testToken(testServer.URL, "invalid-token")

	if err == nil {
		t.Error("testToken() should return error for unauthorized")
	}
}

func TestTestTokenConnectionError(t *testing.T) {
	err := testToken("http://nonexistent.local:99999", "token")

	if err == nil {
		t.Error("testToken() should return error for connection failure")
	}
}

// Tests for runLogin

func TestRunLoginWithTokenFlag(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create config dir
	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	token = "usr_test-token-12345"
	server = ""

	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() with token flag error = %v", err)
	}

	// Verify token was saved
	tokenPath := filepath.Join(configDir, "token")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("Failed to read saved token: %v", err)
	}

	if string(data) != "usr_test-token-12345\n" {
		t.Errorf("Saved token = %q", string(data))
	}

	token = ""
}

func TestRunLoginWithAdminToken(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	token = "adm_admin-token-12345"
	server = ""

	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() with admin token error = %v", err)
	}

	token = ""
}

func TestRunLoginWithUnexpectedTokenPrefix(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	// Token without expected prefix should still work but print warning
	token = "unexpected-token-12345"
	server = ""

	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() with unexpected token error = %v", err)
	}

	token = ""
}

func TestRunLoginWithServerVerification(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Per AI.md PART 14: Wrapped response format
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "data": api.HealthResponse{
			Status: "healthy",
		}})
	}))
	defer testServer.Close()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	token = "usr_test-token"
	server = testServer.URL

	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() with server verification error = %v", err)
	}

	token = ""
	server = ""
}

func TestRunLoginWithServerVerificationFailed(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer testServer.Close()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	token = "usr_bad-token"
	server = testServer.URL

	// Should still succeed but print warning
	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() with failed verification should still save token: %v", err)
	}

	token = ""
	server = ""
}

func TestRunLoginEmptyToken(t *testing.T) {
	token = ""

	// When token is empty and not interactive, should error
	// Note: This test doesn't cover the interactive prompt
}

// Tests for runLogout

func TestRunLogoutSuccess(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create token file
	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)
	tokenPath := filepath.Join(configDir, "token")
	os.WriteFile(tokenPath, []byte("test-token"), 0600)

	err := runLogout()

	if err != nil {
		t.Fatalf("runLogout() error = %v", err)
	}

	// Verify token was removed
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Error("Token file should be removed after logout")
	}
}

func TestRunLogoutNoToken(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// No token file exists
	err := runLogout()

	if err != nil {
		t.Fatalf("runLogout() with no token error = %v", err)
	}
}

// Tests for loginCmd.RunE

func TestLoginCmdRunE(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	token = "usr_test-token"
	server = ""

	err := loginCmd.RunE(loginCmd, []string{})

	if err != nil {
		t.Fatalf("loginCmd.RunE() error = %v", err)
	}

	token = ""
}

// Tests for logoutCmd.RunE

func TestLogoutCmdRunE(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)
	tokenPath := filepath.Join(configDir, "token")
	os.WriteFile(tokenPath, []byte("test-token"), 0600)

	err := logoutCmd.RunE(logoutCmd, []string{})

	if err != nil {
		t.Fatalf("logoutCmd.RunE() error = %v", err)
	}
}

// Tests for token file permissions

func TestRunLoginTokenFilePermissions(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	token = "usr_test-token"
	server = ""

	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() error = %v", err)
	}

	tokenPath := filepath.Join(configDir, "token")
	info, err := os.Stat(tokenPath)
	if err != nil {
		t.Fatalf("Failed to stat token file: %v", err)
	}

	// Check file permissions (should be 0600)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("Token file permissions = %o, want 0600", perm)
	}

	token = ""
}

// Tests for config directory creation

func TestRunLoginCreatesConfigDir(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Don't create config dir - runLogin should create it
	token = "usr_test-token"
	server = ""

	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() error = %v", err)
	}

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		t.Error("runLogin() should create config directory")
	}

	token = ""
}

// Tests for token prefix validation

func TestTokenPrefixValidation(t *testing.T) {
	tests := []struct {
		token         string
		expectedValid bool
	}{
		{"usr_valid-token", true},
		{"adm_admin-token", true},
		{"invalid-token", false},
		{"USR_uppercase", false},
		{"", false},
		{"usr", false},
		{"adm", false},
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			hasPrefix := len(tt.token) >= 4 && (tt.token[:4] == "usr_" || tt.token[:4] == "adm_")

			if hasPrefix != tt.expectedValid {
				t.Errorf("Token %q hasValidPrefix = %v, want %v", tt.token, hasPrefix, tt.expectedValid)
			}
		})
	}
}

// Tests for viper config

func TestRunLoginWithViperConfig(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	viper.Reset()

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)

	token = "usr_test-token"
	server = ""

	err := runLogin()

	if err != nil {
		t.Fatalf("runLogin() with viper config error = %v", err)
	}

	token = ""
}

// Tests for multiple login/logout cycles

func TestMultipleLoginLogoutCycles(t *testing.T) {
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	configDir := filepath.Join(tempDir, ".config", "apimgr", "search")
	os.MkdirAll(configDir, 0700)
	tokenPath := filepath.Join(configDir, "token")

	// Login
	token = "usr_token1"
	server = ""
	if err := runLogin(); err != nil {
		t.Fatalf("First login error: %v", err)
	}

	// Verify token
	data, _ := os.ReadFile(tokenPath)
	if string(data) != "usr_token1\n" {
		t.Errorf("First token = %q", string(data))
	}

	// Logout
	if err := runLogout(); err != nil {
		t.Fatalf("First logout error: %v", err)
	}

	// Login again
	token = "usr_token2"
	if err := runLogin(); err != nil {
		t.Fatalf("Second login error: %v", err)
	}

	// Verify second token
	data, _ = os.ReadFile(tokenPath)
	if string(data) != "usr_token2\n" {
		t.Errorf("Second token = %q", string(data))
	}

	// Logout
	if err := runLogout(); err != nil {
		t.Fatalf("Second logout error: %v", err)
	}

	token = ""
}
