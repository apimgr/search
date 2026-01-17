package path

import (
	"runtime"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	p := Get("apimgr", "search", false)
	if p == nil {
		t.Fatal("Get() returned nil")
	}
	if p.ConfigDir == "" {
		t.Error("ConfigDir should not be empty")
	}
	if p.DataDir == "" {
		t.Error("DataDir should not be empty")
	}
	if p.LogDir == "" {
		t.Error("LogDir should not be empty")
	}
	if p.BackupDir == "" {
		t.Error("BackupDir should not be empty")
	}
}

func TestGetPrivileged(t *testing.T) {
	p := Get("apimgr", "search", true)
	if p == nil {
		t.Fatal("Get() with privileged returned nil")
	}
	if p.ConfigDir == "" {
		t.Error("ConfigDir should not be empty")
	}
}

func TestGetLinuxPaths(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Skipping Linux-specific test")
	}

	// Test unprivileged paths
	p := getLinuxPaths("apimgr", "search", false)
	if p == nil {
		t.Fatal("getLinuxPaths() returned nil")
	}
	if !strings.Contains(p.ConfigDir, ".config") {
		t.Errorf("Unprivileged ConfigDir should contain .config, got %s", p.ConfigDir)
	}

	// Test privileged paths
	p = getLinuxPaths("apimgr", "search", true)
	if !strings.HasPrefix(p.ConfigDir, "/etc") {
		t.Errorf("Privileged ConfigDir should start with /etc, got %s", p.ConfigDir)
	}
}

func TestGetDarwinPaths(t *testing.T) {
	// Test unprivileged paths
	p := getDarwinPaths("apimgr", "search", false)
	if p == nil {
		t.Fatal("getDarwinPaths() returned nil")
	}
	if !strings.Contains(p.ConfigDir, "Application Support") {
		t.Errorf("macOS ConfigDir should contain 'Application Support', got %s", p.ConfigDir)
	}

	// Test privileged paths
	p = getDarwinPaths("apimgr", "search", true)
	if !strings.HasPrefix(p.ConfigDir, "/Library") {
		t.Errorf("Privileged macOS ConfigDir should start with /Library, got %s", p.ConfigDir)
	}
}

func TestGetBSDPaths(t *testing.T) {
	// Test unprivileged paths
	p := getBSDPaths("apimgr", "search", false)
	if p == nil {
		t.Fatal("getBSDPaths() returned nil")
	}

	// Test privileged paths
	p = getBSDPaths("apimgr", "search", true)
	if !strings.HasPrefix(p.ConfigDir, "/usr/local/etc") {
		t.Errorf("Privileged BSD ConfigDir should start with /usr/local/etc, got %s", p.ConfigDir)
	}
}

func TestGetWindowsPaths(t *testing.T) {
	// Test unprivileged paths
	p := getWindowsPaths("apimgr", "search", false)
	if p == nil {
		t.Fatal("getWindowsPaths() returned nil")
	}

	// Test privileged paths
	p = getWindowsPaths("apimgr", "search", true)
	if p.ConfigDir == "" {
		t.Error("Windows ConfigDir should not be empty")
	}
}

func TestIsPrivileged(t *testing.T) {
	// Just verify it doesn't panic
	result := IsPrivileged()
	_ = result
}

func TestPathsGetConfigPath(t *testing.T) {
	p := Get("apimgr", "search", false)
	path := p.GetConfigPath()
	if path == "" {
		t.Error("GetConfigPath() should not be empty")
	}
	if !strings.HasSuffix(path, "server.yml") {
		t.Errorf("GetConfigPath() should end with server.yml, got %s", path)
	}
}

func TestPathsEnsureDirs(t *testing.T) {
	// Create paths with temp directories for testing
	p := &Paths{
		ConfigDir:   "/tmp/paths-test-config",
		DataDir:     "/tmp/paths-test-data",
		LogDir:      "/tmp/paths-test-logs",
		BackupDir:   "/tmp/paths-test-backup",
		PIDFile:     "/tmp/paths-test-search.pid",
		SSLDir:      "/tmp/paths-test-ssl",
		SecurityDir: "/tmp/paths-test-security",
		DBDir:       "/tmp/paths-test-db",
	}

	err := p.EnsureDirs()
	if err != nil {
		t.Errorf("EnsureDirs() error = %v", err)
	}
}
