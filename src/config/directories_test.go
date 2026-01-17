package config

import (
	"os"
	"runtime"
	"testing"
)

func TestSetAndGetConfigDirOverride(t *testing.T) {
	// Test that setting override changes GetConfigDir result
	SetConfigDirOverride("/test/config")
	defer SetConfigDirOverride("") // Reset

	got := GetConfigDir()
	if got != "/test/config" {
		t.Errorf("GetConfigDir() with override = %q, want %q", got, "/test/config")
	}

	// Test without override (returns system default)
	SetConfigDirOverride("")
	got = GetConfigDir()
	if got == "" {
		t.Error("GetConfigDir() without override should not be empty")
	}
}

func TestSetAndGetDataDirOverride(t *testing.T) {
	SetDataDirOverride("/test/data")
	defer SetDataDirOverride("")

	got := GetDataDir()
	if got != "/test/data" {
		t.Errorf("GetDataDir() with override = %q, want %q", got, "/test/data")
	}

	SetDataDirOverride("")
	got = GetDataDir()
	if got == "" {
		t.Error("GetDataDir() without override should not be empty")
	}
}

func TestSetAndGetLogDirOverride(t *testing.T) {
	SetLogDirOverride("/test/logs")
	defer SetLogDirOverride("")

	got := GetLogDir()
	if got != "/test/logs" {
		t.Errorf("GetLogDir() with override = %q, want %q", got, "/test/logs")
	}

	SetLogDirOverride("")
	got = GetLogDir()
	if got == "" {
		t.Error("GetLogDir() without override should not be empty")
	}
}

func TestSetAndGetPIDFileOverride(t *testing.T) {
	SetPIDFileOverride("/test/search.pid")
	defer SetPIDFileOverride("")

	got := GetPIDFile()
	if got != "/test/search.pid" {
		t.Errorf("GetPIDFile() with override = %q, want %q", got, "/test/search.pid")
	}

	SetPIDFileOverride("")
	got = GetPIDFile()
	if got == "" {
		t.Error("GetPIDFile() without override should not be empty")
	}
}

func TestSetAndGetCacheDirOverride(t *testing.T) {
	SetCacheDirOverride("/test/cache")
	defer SetCacheDirOverride("")

	got := GetCacheDir()
	if got != "/test/cache" {
		t.Errorf("GetCacheDir() with override = %q, want %q", got, "/test/cache")
	}

	SetCacheDirOverride("")
	got = GetCacheDir()
	if got == "" {
		t.Error("GetCacheDir() without override should not be empty")
	}
}

func TestSetAndGetBackupDirOverride(t *testing.T) {
	SetBackupDirOverride("/test/backup")
	defer SetBackupDirOverride("")

	got := GetBackupDir()
	if got != "/test/backup" {
		t.Errorf("GetBackupDir() with override = %q, want %q", got, "/test/backup")
	}

	SetBackupDirOverride("")
	got = GetBackupDir()
	if got == "" {
		t.Error("GetBackupDir() without override should not be empty")
	}
}

func TestGetOS(t *testing.T) {
	got := GetOS()
	if got != runtime.GOOS {
		t.Errorf("GetOS() = %q, want %q", got, runtime.GOOS)
	}
}

func TestGetArch(t *testing.T) {
	got := GetArch()
	if got != runtime.GOARCH {
		t.Errorf("GetArch() = %q, want %q", got, runtime.GOARCH)
	}
}

func TestIsRunningInContainer(t *testing.T) {
	// Just verify it doesn't panic and returns a bool
	result := IsRunningInContainer()
	_ = result
}

func TestGetSSLDir(t *testing.T) {
	got := GetSSLDir()
	if got == "" {
		t.Error("GetSSLDir() should not be empty")
	}
}

func TestGetDatabaseDir(t *testing.T) {
	got := GetDatabaseDir()
	if got == "" {
		t.Error("GetDatabaseDir() should not be empty")
	}
}

func TestGetGeoIPDir(t *testing.T) {
	got := GetGeoIPDir()
	if got == "" {
		t.Error("GetGeoIPDir() should not be empty")
	}
}

func TestGetSecurityDir(t *testing.T) {
	got := GetSecurityDir()
	if got == "" {
		t.Error("GetSecurityDir() should not be empty")
	}
}

func TestGetTorDir(t *testing.T) {
	got := GetTorDir()
	if got == "" {
		t.Error("GetTorDir() should not be empty")
	}
}

func TestGetTorKeysDir(t *testing.T) {
	got := GetTorKeysDir()
	if got == "" {
		t.Error("GetTorKeysDir() should not be empty")
	}
}

func TestGetTemplatesDir(t *testing.T) {
	got := GetTemplatesDir()
	if got == "" {
		t.Error("GetTemplatesDir() should not be empty")
	}
}

func TestGetEmailTemplatesDir(t *testing.T) {
	got := GetEmailTemplatesDir()
	if got == "" {
		t.Error("GetEmailTemplatesDir() should not be empty")
	}
}

func TestGetWebDataDir(t *testing.T) {
	got := GetWebDataDir()
	if got == "" {
		t.Error("GetWebDataDir() should not be empty")
	}
}

func TestGetWellKnownDir(t *testing.T) {
	got := GetWellKnownDir()
	if got == "" {
		t.Error("GetWellKnownDir() should not be empty")
	}
}

func TestGetDirectoryPermissions(t *testing.T) {
	got := GetDirectoryPermissions()
	if got == 0 {
		t.Error("GetDirectoryPermissions() should not be 0")
	}
	// Should be 0755 or 0750
	if got != 0755 && got != 0750 {
		t.Errorf("GetDirectoryPermissions() = %o, want 0755 or 0750", got)
	}
}

func TestGetSensitiveDirectoryPermissions(t *testing.T) {
	got := GetSensitiveDirectoryPermissions()
	if got == 0 {
		t.Error("GetSensitiveDirectoryPermissions() should not be 0")
	}
	// Should be 0700
	if got != 0700 {
		t.Errorf("GetSensitiveDirectoryPermissions() = %o, want 0700", got)
	}
}

func TestGetSensitiveFilePermissions(t *testing.T) {
	got := GetSensitiveFilePermissions()
	if got == 0 {
		t.Error("GetSensitiveFilePermissions() should not be 0")
	}
	// Should be 0600
	if got != 0600 {
		t.Errorf("GetSensitiveFilePermissions() = %o, want 0600", got)
	}
}

func TestEnsureDirectories(t *testing.T) {
	// Create temp directories for testing
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set overrides to temp locations
	SetConfigDirOverride(tmpDir + "/config")
	SetDataDirOverride(tmpDir + "/data")
	SetLogDirOverride(tmpDir + "/logs")
	SetCacheDirOverride(tmpDir + "/cache")
	defer func() {
		SetConfigDirOverride("")
		SetDataDirOverride("")
		SetLogDirOverride("")
		SetCacheDirOverride("")
	}()

	err = EnsureDirectories()
	if err != nil {
		t.Errorf("EnsureDirectories() error = %v", err)
	}

	// Verify directories were created (EnsureDirectories creates config, data, log, cache, but not backup)
	dirs := []string{
		tmpDir + "/config",
		tmpDir + "/data",
		tmpDir + "/logs",
		tmpDir + "/cache",
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
		}
	}
}

func TestGetServiceFile(t *testing.T) {
	got := GetServiceFile()
	// Just verify it returns something (varies by OS)
	_ = got
}

func TestGetBinaryPath(t *testing.T) {
	got := GetBinaryPath()
	if got == "" {
		t.Error("GetBinaryPath() should not be empty")
	}
}

func TestEnsureSensitiveFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "sensitive-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := tmpDir + "/secret.key"

	// Create the file first
	if err := os.WriteFile(testFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Ensure permissions
	err = EnsureSensitiveFile(testFile)
	if err != nil {
		t.Errorf("EnsureSensitiveFile() error = %v", err)
	}

	// Verify permissions (Unix only)
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(testFile)
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("File permissions = %o, want 0600", perm)
		}
	}
}
