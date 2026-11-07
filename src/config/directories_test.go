package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSetAndGetConfigDirOverride(t *testing.T) {
	// Test that setting override changes GetConfigDir result
	SetConfigDirOverride("/test/config")
	// Reset
	defer SetConfigDirOverride("")

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
	if want := filepath.Join(GetConfigDir(), "template"); got != want {
		t.Errorf("GetTemplatesDir() = %q, want %q", got, want)
	}
}

func TestGetEmailTemplatesDir(t *testing.T) {
	got := GetEmailTemplatesDir()
	if want := filepath.Join(GetConfigDir(), "template", "email"); got != want {
		t.Errorf("GetEmailTemplatesDir() = %q, want %q", got, want)
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
	SetBackupDirOverride(tmpDir + "/backup")
	defer func() {
		SetConfigDirOverride("")
		SetDataDirOverride("")
		SetLogDirOverride("")
		SetCacheDirOverride("")
		SetBackupDirOverride("")
	}()

	err = EnsureDirectories()
	if err != nil {
		t.Errorf("EnsureDirectories() error = %v", err)
	}

	// Verify all runtime directories, including backups, were created.
	dirs := []string{
		tmpDir + "/config",
		tmpDir + "/data",
		tmpDir + "/logs",
		tmpDir + "/cache",
		tmpDir + "/backup",
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

func TestCanonicalDirectoryEnvironmentOverrides(t *testing.T) {
	t.Run("config directory", func(t *testing.T) {
		SetConfigDirOverride("")
		t.Setenv("CONFIG_DIR", "/env/config")
		if got := GetConfigDir(); got != "/env/config" {
			t.Fatalf("GetConfigDir() = %q, want /env/config", got)
		}
	})
	t.Run("data directory", func(t *testing.T) {
		SetDataDirOverride("")
		t.Setenv("DATA_DIR", "/env/data")
		if got := GetDataDir(); got != "/env/data" {
			t.Fatalf("GetDataDir() = %q, want /env/data", got)
		}
	})
	t.Run("log directory", func(t *testing.T) {
		SetLogDirOverride("")
		t.Setenv("LOG_DIR", "/env/log")
		if got := GetLogDir(); got != "/env/log" {
			t.Fatalf("GetLogDir() = %q, want /env/log", got)
		}
	})
	t.Run("backup directory", func(t *testing.T) {
		SetBackupDirOverride("")
		t.Setenv("BACKUP_DIR", "/env/backup")
		if got := GetBackupDir(); got != "/env/backup" {
			t.Fatalf("GetBackupDir() = %q, want /env/backup", got)
		}
	})
	t.Run("database directory", func(t *testing.T) {
		SetDatabaseDirOverride("")
		t.Setenv("DATABASE_DIR", "/env/database")
		if got := GetDatabaseDir(); got != "/env/database" {
			t.Fatalf("GetDatabaseDir() = %q, want /env/database", got)
		}
	})
}

func TestCacheAndPIDPathsUseCLIOverrides(t *testing.T) {
	t.Run("cache", func(t *testing.T) {
		SetCacheDirOverride("/flag/cache")
		t.Cleanup(func() { SetCacheDirOverride("") })
		if got := GetCacheDir(); got != "/flag/cache" {
			t.Fatalf("GetCacheDir() = %q, want /flag/cache", got)
		}
	})
	t.Run("pid file", func(t *testing.T) {
		SetPIDFileOverride("/flag/search.pid")
		t.Cleanup(func() { SetPIDFileOverride("") })
		if got := GetPIDFile(); got != "/flag/search.pid" {
			t.Fatalf("GetPIDFile() = %q, want /flag/search.pid", got)
		}
	})
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
