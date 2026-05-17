package path

import (
	"os"
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

func TestEnsureDirsError(t *testing.T) {
	// Test EnsureDirs error path by using an invalid path
	// Using /dev/null as a base causes MkdirAll to fail since it's not a directory
	p := &Paths{
		ConfigDir:   "/dev/null/invalid-path",
		DataDir:     "/tmp/paths-test-data",
		LogDir:      "/tmp/paths-test-logs",
		BackupDir:   "/tmp/paths-test-backup",
		PIDFile:     "/tmp/paths-test.pid",
		SSLDir:      "/tmp/paths-test-ssl",
		SecurityDir: "/tmp/paths-test-security",
		DBDir:       "/tmp/paths-test-db",
	}

	err := p.EnsureDirs()
	if err == nil {
		t.Error("EnsureDirs() should return error for invalid path")
	}
}

func TestGetAllFields(t *testing.T) {
	// Test that all fields are populated for both privileged and unprivileged
	tests := []struct {
		name       string
		privileged bool
	}{
		{"unprivileged", false},
		{"privileged", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Get("testorg", "testapp", tt.privileged)
			if p == nil {
				t.Fatal("Get() returned nil")
			}
			// Check all fields are populated
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
			if p.PIDFile == "" {
				t.Error("PIDFile should not be empty")
			}
			if p.SSLDir == "" {
				t.Error("SSLDir should not be empty")
			}
			if p.SecurityDir == "" {
				t.Error("SecurityDir should not be empty")
			}
			if p.DBDir == "" {
				t.Error("DBDir should not be empty")
			}
		})
	}
}

func TestLinuxPathsAllFields(t *testing.T) {
	// Test all fields for Linux paths - both privileged and unprivileged

	// Unprivileged
	p := getLinuxPaths("testorg", "testapp", false)
	if p.PIDFile == "" {
		t.Error("Unprivileged PIDFile should not be empty")
	}
	if p.SSLDir == "" {
		t.Error("Unprivileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Unprivileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Unprivileged DBDir should not be empty")
	}
	if p.BackupDir == "" {
		t.Error("Unprivileged BackupDir should not be empty")
	}

	// Privileged
	p = getLinuxPaths("testorg", "testapp", true)
	if !strings.HasPrefix(p.PIDFile, "/var/run") {
		t.Errorf("Privileged PIDFile should start with /var/run, got %s", p.PIDFile)
	}
	if p.SSLDir == "" {
		t.Error("Privileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Privileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Privileged DBDir should not be empty")
	}
}

func TestDarwinPathsAllFields(t *testing.T) {
	// Test all fields for Darwin paths

	// Unprivileged
	p := getDarwinPaths("testorg", "testapp", false)
	if p.PIDFile == "" {
		t.Error("Unprivileged PIDFile should not be empty")
	}
	if p.LogDir == "" {
		t.Error("Unprivileged LogDir should not be empty")
	}
	if p.BackupDir == "" {
		t.Error("Unprivileged BackupDir should not be empty")
	}
	if p.SSLDir == "" {
		t.Error("Unprivileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Unprivileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Unprivileged DBDir should not be empty")
	}

	// Privileged
	p = getDarwinPaths("testorg", "testapp", true)
	if !strings.HasPrefix(p.LogDir, "/Library/Logs") {
		t.Errorf("Privileged LogDir should start with /Library/Logs, got %s", p.LogDir)
	}
	if !strings.HasPrefix(p.BackupDir, "/Library/Backups") {
		t.Errorf("Privileged BackupDir should start with /Library/Backups, got %s", p.BackupDir)
	}
	if !strings.HasPrefix(p.PIDFile, "/var/run") {
		t.Errorf("Privileged PIDFile should start with /var/run, got %s", p.PIDFile)
	}
	if p.SSLDir == "" {
		t.Error("Privileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Privileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Privileged DBDir should not be empty")
	}
	if p.DataDir == "" {
		t.Error("Privileged DataDir should not be empty")
	}
}

func TestBSDPathsAllFields(t *testing.T) {
	// Test all fields for BSD paths

	// Unprivileged
	p := getBSDPaths("testorg", "testapp", false)
	if p.PIDFile == "" {
		t.Error("Unprivileged PIDFile should not be empty")
	}
	if p.LogDir == "" {
		t.Error("Unprivileged LogDir should not be empty")
	}
	if p.BackupDir == "" {
		t.Error("Unprivileged BackupDir should not be empty")
	}
	if p.SSLDir == "" {
		t.Error("Unprivileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Unprivileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Unprivileged DBDir should not be empty")
	}
	if !strings.Contains(p.ConfigDir, ".config") {
		t.Errorf("Unprivileged BSD ConfigDir should contain .config, got %s", p.ConfigDir)
	}

	// Privileged
	p = getBSDPaths("testorg", "testapp", true)
	if !strings.HasPrefix(p.DataDir, "/var/db") {
		t.Errorf("Privileged BSD DataDir should start with /var/db, got %s", p.DataDir)
	}
	if !strings.HasPrefix(p.LogDir, "/var/log") {
		t.Errorf("Privileged BSD LogDir should start with /var/log, got %s", p.LogDir)
	}
	if !strings.HasPrefix(p.BackupDir, "/var/backups") {
		t.Errorf("Privileged BSD BackupDir should start with /var/backups, got %s", p.BackupDir)
	}
	if !strings.HasPrefix(p.PIDFile, "/var/run") {
		t.Errorf("Privileged BSD PIDFile should start with /var/run, got %s", p.PIDFile)
	}
	if p.SSLDir == "" {
		t.Error("Privileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Privileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Privileged DBDir should not be empty")
	}
}

func TestWindowsPathsAllFields(t *testing.T) {
	// Test all fields for Windows paths

	// Unprivileged
	p := getWindowsPaths("testorg", "testapp", false)
	if p.PIDFile == "" {
		t.Error("Unprivileged PIDFile should not be empty")
	}
	if p.LogDir == "" {
		t.Error("Unprivileged LogDir should not be empty")
	}
	if p.BackupDir == "" {
		t.Error("Unprivileged BackupDir should not be empty")
	}
	if p.SSLDir == "" {
		t.Error("Unprivileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Unprivileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Unprivileged DBDir should not be empty")
	}
	if p.DataDir == "" {
		t.Error("Unprivileged DataDir should not be empty")
	}

	// Privileged
	p = getWindowsPaths("testorg", "testapp", true)
	if p.PIDFile == "" {
		t.Error("Privileged PIDFile should not be empty")
	}
	if p.LogDir == "" {
		t.Error("Privileged LogDir should not be empty")
	}
	if p.BackupDir == "" {
		t.Error("Privileged BackupDir should not be empty")
	}
	if p.SSLDir == "" {
		t.Error("Privileged SSLDir should not be empty")
	}
	if p.SecurityDir == "" {
		t.Error("Privileged SecurityDir should not be empty")
	}
	if p.DBDir == "" {
		t.Error("Privileged DBDir should not be empty")
	}
	if p.DataDir == "" {
		t.Error("Privileged DataDir should not be empty")
	}
}

func TestWindowsPathsWithEnvVars(t *testing.T) {
	// Save original env vars
	origProgramData := os.Getenv("ProgramData")
	origAppData := os.Getenv("AppData")
	origLocalAppData := os.Getenv("LocalAppData")

	defer func() {
		// Restore original env vars
		os.Setenv("ProgramData", origProgramData)
		os.Setenv("AppData", origAppData)
		os.Setenv("LocalAppData", origLocalAppData)
	}()

	// Test privileged with ProgramData set
	os.Setenv("ProgramData", "C:\\TestProgramData")
	p := getWindowsPaths("testorg", "testapp", true)
	if !strings.Contains(p.ConfigDir, "TestProgramData") {
		t.Errorf("Privileged Windows ConfigDir should use ProgramData env, got %s", p.ConfigDir)
	}

	// Test privileged with empty ProgramData (should fallback)
	os.Setenv("ProgramData", "")
	p = getWindowsPaths("testorg", "testapp", true)
	if !strings.Contains(p.ConfigDir, "ProgramData") {
		t.Errorf("Privileged Windows ConfigDir should fallback to C:\\ProgramData, got %s", p.ConfigDir)
	}

	// Test unprivileged with AppData set
	os.Setenv("AppData", "C:\\TestAppData")
	os.Setenv("LocalAppData", "C:\\TestLocalAppData")
	p = getWindowsPaths("testorg", "testapp", false)
	if !strings.Contains(p.ConfigDir, "TestAppData") {
		t.Errorf("Unprivileged Windows ConfigDir should use AppData env, got %s", p.ConfigDir)
	}
	if !strings.Contains(p.DataDir, "TestLocalAppData") {
		t.Errorf("Unprivileged Windows DataDir should use LocalAppData env, got %s", p.DataDir)
	}

	// Test unprivileged with empty AppData (should fallback to home dir)
	os.Setenv("AppData", "")
	os.Setenv("LocalAppData", "")
	p = getWindowsPaths("testorg", "testapp", false)
	// Should contain AppData/Roaming in the path (from home dir fallback)
	if !strings.Contains(p.ConfigDir, "AppData") {
		t.Errorf("Unprivileged Windows ConfigDir should fallback to home/AppData, got %s", p.ConfigDir)
	}
}

func TestIsPrivilegedResult(t *testing.T) {
	// Test IsPrivileged returns a boolean and doesn't panic
	result := IsPrivileged()

	// On most test environments, we're not running as root
	// Just verify it returns a valid boolean
	if result != true && result != false {
		t.Error("IsPrivileged should return a boolean")
	}

	// On Linux, check that UID 0 would be privileged
	if runtime.GOOS != "windows" {
		uid := os.Getuid()
		expected := uid == 0
		if result != expected {
			t.Errorf("IsPrivileged() = %v, expected %v for uid %d", result, expected, uid)
		}
	}
}

func TestGetConfigPathContent(t *testing.T) {
	tests := []struct {
		name       string
		org        string
		app        string
		privileged bool
	}{
		{"unprivileged", "testorg", "testapp", false},
		{"privileged", "testorg", "testapp", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Get(tt.org, tt.app, tt.privileged)
			configPath := p.GetConfigPath()

			// Verify it ends with server.yml
			if !strings.HasSuffix(configPath, "server.yml") {
				t.Errorf("GetConfigPath() should end with server.yml, got %s", configPath)
			}

			// Verify it's within ConfigDir
			if !strings.HasPrefix(configPath, p.ConfigDir) {
				t.Errorf("GetConfigPath() should be within ConfigDir, got %s", configPath)
			}
		})
	}
}

func TestGetWithDifferentOrgNames(t *testing.T) {
	// Test with various org and app name combinations
	tests := []struct {
		org  string
		name string
	}{
		{"myorg", "myapp"},
		{"company", "service"},
		{"test-org", "test-app"},
	}

	for _, tt := range tests {
		t.Run(tt.org+"/"+tt.name, func(t *testing.T) {
			p := Get(tt.org, tt.name, false)
			if p == nil {
				t.Fatal("Get() returned nil")
			}

			// Verify org and name appear in paths
			if !strings.Contains(p.ConfigDir, tt.org) || !strings.Contains(p.ConfigDir, tt.name) {
				t.Errorf("ConfigDir should contain org and name, got %s", p.ConfigDir)
			}
		})
	}
}
