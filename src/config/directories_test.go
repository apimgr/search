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

func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"contains at start", "docker123", "docker", true},
		{"contains at end", "123docker", "docker", true},
		{"contains in middle", "abc-docker-xyz", "docker", true},
		{"exact match", "docker", "docker", true},
		{"not contains", "container", "docker", false},
		{"empty string", "", "docker", false},
		{"empty substr", "docker", "", true},
		{"both empty", "", "", true},
		{"longer substr", "abc", "abcdefg", false},
		{"case sensitive", "Docker", "docker", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestGetConfigDirWithEnvOverride(t *testing.T) {
	// Save and restore env var
	originalConfigDir := os.Getenv("SEARCH_CONFIG_DIR")
	originalConfigDir2 := os.Getenv("CONFIG_DIR")
	defer func() {
		os.Setenv("SEARCH_CONFIG_DIR", originalConfigDir)
		os.Setenv("CONFIG_DIR", originalConfigDir2)
	}()

	// Clear override first
	SetConfigDirOverride("")

	// Test SEARCH_CONFIG_DIR env var
	os.Setenv("SEARCH_CONFIG_DIR", "/env/config")
	os.Setenv("CONFIG_DIR", "")
	got := GetConfigDir()
	if got != "/env/config" {
		t.Errorf("GetConfigDir() with SEARCH_CONFIG_DIR = %q, want %q", got, "/env/config")
	}

	// Test CONFIG_DIR fallback
	os.Setenv("SEARCH_CONFIG_DIR", "")
	os.Setenv("CONFIG_DIR", "/env/config2")
	got = GetConfigDir()
	if got != "/env/config2" {
		t.Errorf("GetConfigDir() with CONFIG_DIR = %q, want %q", got, "/env/config2")
	}

	// Clear env vars
	os.Setenv("SEARCH_CONFIG_DIR", "")
	os.Setenv("CONFIG_DIR", "")
}

func TestGetDataDirWithEnvOverride(t *testing.T) {
	// Save and restore env var
	originalDataDir := os.Getenv("SEARCH_DATA_DIR")
	originalDataDir2 := os.Getenv("DATA_DIR")
	defer func() {
		os.Setenv("SEARCH_DATA_DIR", originalDataDir)
		os.Setenv("DATA_DIR", originalDataDir2)
	}()

	// Clear override first
	SetDataDirOverride("")

	// Test SEARCH_DATA_DIR env var
	os.Setenv("SEARCH_DATA_DIR", "/env/data")
	os.Setenv("DATA_DIR", "")
	got := GetDataDir()
	if got != "/env/data" {
		t.Errorf("GetDataDir() with SEARCH_DATA_DIR = %q, want %q", got, "/env/data")
	}

	// Test DATA_DIR fallback
	os.Setenv("SEARCH_DATA_DIR", "")
	os.Setenv("DATA_DIR", "/env/data2")
	got = GetDataDir()
	if got != "/env/data2" {
		t.Errorf("GetDataDir() with DATA_DIR = %q, want %q", got, "/env/data2")
	}

	// Clear env vars
	os.Setenv("SEARCH_DATA_DIR", "")
	os.Setenv("DATA_DIR", "")
}

func TestGetLogDirWithEnvOverride(t *testing.T) {
	// Save and restore env var
	originalLogDir := os.Getenv("SEARCH_LOG_DIR")
	originalLogDir2 := os.Getenv("LOG_DIR")
	defer func() {
		os.Setenv("SEARCH_LOG_DIR", originalLogDir)
		os.Setenv("LOG_DIR", originalLogDir2)
	}()

	// Clear override first
	SetLogDirOverride("")

	// Test SEARCH_LOG_DIR env var
	os.Setenv("SEARCH_LOG_DIR", "/env/logs")
	os.Setenv("LOG_DIR", "")
	got := GetLogDir()
	if got != "/env/logs" {
		t.Errorf("GetLogDir() with SEARCH_LOG_DIR = %q, want %q", got, "/env/logs")
	}

	// Test LOG_DIR fallback
	os.Setenv("SEARCH_LOG_DIR", "")
	os.Setenv("LOG_DIR", "/env/logs2")
	got = GetLogDir()
	if got != "/env/logs2" {
		t.Errorf("GetLogDir() with LOG_DIR = %q, want %q", got, "/env/logs2")
	}

	// Clear env vars
	os.Setenv("SEARCH_LOG_DIR", "")
	os.Setenv("LOG_DIR", "")
}

func TestGetCacheDirWithEnvOverride(t *testing.T) {
	// Save and restore env var
	originalCacheDir := os.Getenv("SEARCH_CACHE_DIR")
	defer os.Setenv("SEARCH_CACHE_DIR", originalCacheDir)

	// Clear override first
	SetCacheDirOverride("")

	// Test SEARCH_CACHE_DIR env var
	os.Setenv("SEARCH_CACHE_DIR", "/env/cache")
	got := GetCacheDir()
	if got != "/env/cache" {
		t.Errorf("GetCacheDir() with SEARCH_CACHE_DIR = %q, want %q", got, "/env/cache")
	}

	// Clear env vars
	os.Setenv("SEARCH_CACHE_DIR", "")
}

func TestGetBackupDirWithEnvOverride(t *testing.T) {
	// Save and restore env var
	originalBackupDir := os.Getenv("SEARCH_BACKUP_DIR")
	originalBackupDir2 := os.Getenv("BACKUP_DIR")
	defer func() {
		os.Setenv("SEARCH_BACKUP_DIR", originalBackupDir)
		os.Setenv("BACKUP_DIR", originalBackupDir2)
	}()

	// Clear override first
	SetBackupDirOverride("")

	// Test SEARCH_BACKUP_DIR env var
	os.Setenv("SEARCH_BACKUP_DIR", "/env/backup")
	os.Setenv("BACKUP_DIR", "")
	got := GetBackupDir()
	if got != "/env/backup" {
		t.Errorf("GetBackupDir() with SEARCH_BACKUP_DIR = %q, want %q", got, "/env/backup")
	}

	// Test BACKUP_DIR fallback
	os.Setenv("SEARCH_BACKUP_DIR", "")
	os.Setenv("BACKUP_DIR", "/env/backup2")
	got = GetBackupDir()
	if got != "/env/backup2" {
		t.Errorf("GetBackupDir() with BACKUP_DIR = %q, want %q", got, "/env/backup2")
	}

	// Clear env vars
	os.Setenv("SEARCH_BACKUP_DIR", "")
	os.Setenv("BACKUP_DIR", "")
}

func TestGetPIDFileWithEnvOverride(t *testing.T) {
	// Save and restore env var
	originalPIDFile := os.Getenv("SEARCH_PID_FILE")
	originalPIDFile2 := os.Getenv("PID_FILE")
	defer func() {
		os.Setenv("SEARCH_PID_FILE", originalPIDFile)
		os.Setenv("PID_FILE", originalPIDFile2)
	}()

	// Clear override first
	SetPIDFileOverride("")

	// Test SEARCH_PID_FILE env var
	os.Setenv("SEARCH_PID_FILE", "/env/search.pid")
	os.Setenv("PID_FILE", "")
	got := GetPIDFile()
	if got != "/env/search.pid" {
		t.Errorf("GetPIDFile() with SEARCH_PID_FILE = %q, want %q", got, "/env/search.pid")
	}

	// Test PID_FILE fallback
	os.Setenv("SEARCH_PID_FILE", "")
	os.Setenv("PID_FILE", "/env/search2.pid")
	got = GetPIDFile()
	if got != "/env/search2.pid" {
		t.Errorf("GetPIDFile() with PID_FILE = %q, want %q", got, "/env/search2.pid")
	}

	// Clear env vars
	os.Setenv("SEARCH_PID_FILE", "")
	os.Setenv("PID_FILE", "")
}

func TestGetDatabaseDirWithEnvOverride(t *testing.T) {
	// Save and restore env var
	originalDBDir := os.Getenv("SEARCH_DATABASE_DIR")
	originalDBDir2 := os.Getenv("DATABASE_DIR")
	defer func() {
		os.Setenv("SEARCH_DATABASE_DIR", originalDBDir)
		os.Setenv("DATABASE_DIR", originalDBDir2)
	}()

	// Test SEARCH_DATABASE_DIR env var
	os.Setenv("SEARCH_DATABASE_DIR", "/env/db")
	os.Setenv("DATABASE_DIR", "")
	got := GetDatabaseDir()
	if got != "/env/db" {
		t.Errorf("GetDatabaseDir() with SEARCH_DATABASE_DIR = %q, want %q", got, "/env/db")
	}

	// Test DATABASE_DIR fallback
	os.Setenv("SEARCH_DATABASE_DIR", "")
	os.Setenv("DATABASE_DIR", "/env/db2")
	got = GetDatabaseDir()
	if got != "/env/db2" {
		t.Errorf("GetDatabaseDir() with DATABASE_DIR = %q, want %q", got, "/env/db2")
	}

	// Clear env vars
	os.Setenv("SEARCH_DATABASE_DIR", "")
	os.Setenv("DATABASE_DIR", "")
}

func TestEnsureDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ensure-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	newDir := tmpDir + "/newdir/subdir"

	err = ensureDir(newDir, 0755)
	if err != nil {
		t.Errorf("ensureDir() error = %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(newDir)
	if os.IsNotExist(err) {
		t.Error("ensureDir() did not create directory")
	}
	if !info.IsDir() {
		t.Error("ensureDir() did not create a directory")
	}

	// Verify permissions (Unix only)
	if runtime.GOOS != "windows" {
		perm := info.Mode().Perm()
		if perm != 0755 {
			t.Errorf("Directory permissions = %o, want 0755", perm)
		}
	}
}

func TestEnsureDirAlreadyExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ensure-dir-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create directory with different permissions
	existingDir := tmpDir + "/existing"
	if err := os.Mkdir(existingDir, 0777); err != nil {
		t.Fatalf("Failed to create existing dir: %v", err)
	}

	// ensureDir should fix permissions
	err = ensureDir(existingDir, 0755)
	if err != nil {
		t.Errorf("ensureDir() on existing dir error = %v", err)
	}

	// Verify permissions were fixed (Unix only)
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(existingDir)
		perm := info.Mode().Perm()
		if perm != 0755 {
			t.Errorf("Directory permissions = %o, want 0755", perm)
		}
	}
}

func TestGetDirectoryPermissionsNonRoot(t *testing.T) {
	// When not running as root, should return 0700
	if runtime.GOOS != "windows" && !IsPrivileged() {
		got := GetDirectoryPermissions()
		if got != 0700 {
			t.Errorf("GetDirectoryPermissions() for non-root = %o, want 0700", got)
		}
	}
}

func TestProjectConstants(t *testing.T) {
	if ProjectOrg != "apimgr" {
		t.Errorf("ProjectOrg = %q, want %q", ProjectOrg, "apimgr")
	}
	if ProjectName != "search" {
		t.Errorf("ProjectName = %q, want %q", ProjectName, "search")
	}
}

func TestGetOverride(t *testing.T) {
	// Clear all overrides
	cliOverrideMu.Lock()
	cliOverrides = make(map[string]string)
	cliOverrideMu.Unlock()

	// Test getting non-existent override
	val, ok := getOverride("nonexistent")
	if ok || val != "" {
		t.Errorf("getOverride(nonexistent) = %q, %v, want \"\", false", val, ok)
	}

	// Set an override
	SetConfigDirOverride("/test/config")

	// Test getting existing override
	val, ok = getOverride("config")
	if !ok || val != "/test/config" {
		t.Errorf("getOverride(config) = %q, %v, want %q, true", val, ok, "/test/config")
	}

	// Clear override
	SetConfigDirOverride("")
}
