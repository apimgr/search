package config

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSetDatabaseDirOverride covers the 0% SetDatabaseDirOverride function.
func TestSetDatabaseDirOverride(t *testing.T) {
	SetDatabaseDirOverride("/test/db")
	defer SetDatabaseDirOverride("")

	got := GetDatabaseDir()
	if got != "/test/db" {
		t.Errorf("GetDatabaseDir() with database override = %q, want /test/db", got)
	}
}

// TestSetDatabaseDirOverrideClear verifies clearing the override restores normal behavior.
func TestSetDatabaseDirOverrideClear(t *testing.T) {
	SetDatabaseDirOverride("")
	got := GetDatabaseDir()
	if got == "" {
		t.Error("GetDatabaseDir() after clearing override should not be empty")
	}
}

// TestIsNoColor covers both branches: NO_COLOR unset (false) and set (true).
func TestIsNoColor(t *testing.T) {
	tests := []struct {
		name   string
		setVal string
		unset  bool
		want   bool
	}{
		{"NO_COLOR not set", "", true, false},
		{"NO_COLOR set to 1", "1", false, true},
		{"NO_COLOR set to empty", "", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unset {
				os.Unsetenv("NO_COLOR")
			} else {
				os.Setenv("NO_COLOR", tt.setVal)
				defer os.Unsetenv("NO_COLOR")
			}
			got := IsNoColor()
			if got != tt.want {
				t.Errorf("IsNoColor() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsDumbTerminal covers both branches: TERM=xterm (false) and TERM=dumb (true).
func TestIsDumbTerminal(t *testing.T) {
	orig := os.Getenv("TERM")
	defer os.Setenv("TERM", orig)

	tests := []struct {
		name   string
		envVal string
		want   bool
	}{
		{"xterm terminal", "xterm", false},
		{"dumb terminal", "dumb", true},
		{"empty terminal", "", false},
		{"vt100 terminal", "vt100", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("TERM", tt.envVal)
			got := IsDumbTerminal()
			if got != tt.want {
				t.Errorf("IsDumbTerminal() with TERM=%q = %v, want %v", tt.envVal, got, tt.want)
			}
		})
	}
}

// TestSetDebugOverride covers the 0% SetDebugOverride function.
func TestSetDebugOverride(t *testing.T) {
	origOverride := debugOverride
	defer func() { debugOverride = origOverride }()

	SetDebugOverride(true)
	if !debugOverride {
		t.Error("SetDebugOverride(true) should set debugOverride=true")
	}

	SetDebugOverride(false)
	if debugOverride {
		t.Error("SetDebugOverride(false) should set debugOverride=false")
	}
}

// TestConfigSetDebug covers the 0% Config.SetDebug method.
func TestConfigSetDebug(t *testing.T) {
	origDebug := os.Getenv("DEBUG")
	os.Unsetenv("DEBUG")
	defer func() {
		if origDebug != "" {
			os.Setenv("DEBUG", origDebug)
		} else {
			os.Unsetenv("DEBUG")
		}
	}()

	cfg := DefaultConfig()

	cfg.SetDebug(true)
	if !cfg.IsDebug() {
		t.Error("IsDebug() after SetDebug(true) should return true")
	}

	cfg.SetDebug(false)
	if cfg.IsDebug() {
		t.Error("IsDebug() after SetDebug(false) should return false")
	}
}

// TestConfigOnReload covers the 0% Config.OnReload method; verifies all registered hooks fire.
func TestConfigOnReload(t *testing.T) {
	cfg := DefaultConfig()

	tmp, err := os.CreateTemp(t.TempDir(), "server.*.yml")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := yaml.NewEncoder(tmp).Encode(cfg); err != nil {
		tmp.Close()
		t.Fatalf("yaml.Encode: %v", err)
	}
	tmp.Close()
	cfg.SetPath(tmp.Name())

	var callCount int
	cfg.OnReload(func(_ *Config) { callCount++ })
	cfg.OnReload(func(_ *Config) { callCount++ })

	if err := cfg.Reload(); err != nil {
		t.Fatalf("Reload() error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("OnReload hooks called %d times, want 2", callCount)
	}
}

// TestGetConfigDirInContainer verifies GetConfigDir returns /config/search in a container.
func TestGetConfigDirInContainer(t *testing.T) {
	setContainerOverride(t, true)
	os.Unsetenv("SEARCH_CONFIG_DIR")
	os.Unsetenv("CONFIG_DIR")
	SetConfigDirOverride("")
	defer SetConfigDirOverride("")

	got := GetConfigDir()
	want := "/config/" + ProjectName
	if got != want {
		t.Errorf("GetConfigDir() in container = %q, want %q", got, want)
	}
}

// TestGetDataDirInContainer verifies GetDataDir returns /data/search in a container.
func TestGetDataDirInContainer(t *testing.T) {
	setContainerOverride(t, true)
	os.Unsetenv("SEARCH_DATA_DIR")
	os.Unsetenv("DATA_DIR")
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	got := GetDataDir()
	want := "/data/" + ProjectName
	if got != want {
		t.Errorf("GetDataDir() in container = %q, want %q", got, want)
	}
}

// TestGetLogDirInContainer verifies GetLogDir returns /data/log/search in a container.
func TestGetLogDirInContainer(t *testing.T) {
	setContainerOverride(t, true)
	os.Unsetenv("SEARCH_LOG_DIR")
	os.Unsetenv("LOG_DIR")
	SetLogDirOverride("")
	defer SetLogDirOverride("")

	got := GetLogDir()
	want := "/data/log/" + ProjectName
	if got != want {
		t.Errorf("GetLogDir() in container = %q, want %q", got, want)
	}
}

// TestGetCacheDirInContainer verifies GetCacheDir returns /data/search/cache in a container.
func TestGetCacheDirInContainer(t *testing.T) {
	setContainerOverride(t, true)
	os.Unsetenv("SEARCH_CACHE_DIR")
	SetCacheDirOverride("")
	defer SetCacheDirOverride("")

	got := GetCacheDir()
	want := "/data/" + ProjectName + "/cache"
	if got != want {
		t.Errorf("GetCacheDir() in container = %q, want %q", got, want)
	}
}

// TestGetBackupDirInContainer verifies GetBackupDir returns /data/backups/search in a container.
func TestGetBackupDirInContainer(t *testing.T) {
	setContainerOverride(t, true)
	os.Unsetenv("SEARCH_BACKUP_DIR")
	os.Unsetenv("BACKUP_DIR")
	SetBackupDirOverride("")
	defer SetBackupDirOverride("")

	got := GetBackupDir()
	want := "/data/backups/" + ProjectName
	if got != want {
		t.Errorf("GetBackupDir() in container = %q, want %q", got, want)
	}
}

// TestGetPIDFileInContainer verifies GetPIDFile returns /data/search.pid in a container.
func TestGetPIDFileInContainer(t *testing.T) {
	setContainerOverride(t, true)
	os.Unsetenv("SEARCH_PID_FILE")
	os.Unsetenv("PID_FILE")
	SetPIDFileOverride("")
	defer SetPIDFileOverride("")

	got := GetPIDFile()
	want := "/data/" + ProjectName + ".pid"
	if got != want {
		t.Errorf("GetPIDFile() in container = %q, want %q", got, want)
	}
}

// TestGetDatabaseDirInContainer verifies GetDatabaseDir returns /data/db in a container.
func TestGetDatabaseDirInContainer(t *testing.T) {
	setContainerOverride(t, true)
	os.Unsetenv("SEARCH_DATABASE_DIR")
	os.Unsetenv("DATABASE_DIR")
	SetDatabaseDirOverride("")
	defer SetDatabaseDirOverride("")

	got := GetDatabaseDir()
	if got != "/data/db" {
		t.Errorf("GetDatabaseDir() in container = %q, want /data/db", got)
	}
}

// TestEnsureSystemDirectories covers the 0% EnsureSystemDirectories and chownRecursive functions.
// Requires root (Docker test environment runs as root).
func TestEnsureSystemDirectories(t *testing.T) {
	if !IsPrivileged() {
		t.Skip("TestEnsureSystemDirectories requires root privileges")
	}
	if err := EnsureSystemDirectories("root"); err != nil {
		t.Errorf("EnsureSystemDirectories(\"root\") error = %v, want nil", err)
	}
}

// TestGetDirEnvVarPaths covers the env-var early-return branches in each Get*Dir function.
// These returns are not hit by the container tests because env vars are checked before container detection.
func TestGetDirEnvVarPaths(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		envVal  string
		fn      func() string
		cleanup func()
	}{
		{
			"GetConfigDir via SEARCH_CONFIG_DIR",
			"SEARCH_CONFIG_DIR", "/test/config/search",
			GetConfigDir,
			func() { SetConfigDirOverride(""); os.Unsetenv("SEARCH_CONFIG_DIR") },
		},
		{
			"GetConfigDir via CONFIG_DIR",
			"CONFIG_DIR", "/test/config2/search",
			GetConfigDir,
			func() { SetConfigDirOverride(""); os.Unsetenv("CONFIG_DIR") },
		},
		{
			"GetDataDir via SEARCH_DATA_DIR",
			"SEARCH_DATA_DIR", "/test/data/search",
			GetDataDir,
			func() { SetDataDirOverride(""); os.Unsetenv("SEARCH_DATA_DIR") },
		},
		{
			"GetDataDir via DATA_DIR",
			"DATA_DIR", "/test/data2/search",
			GetDataDir,
			func() { SetDataDirOverride(""); os.Unsetenv("DATA_DIR") },
		},
		{
			"GetLogDir via SEARCH_LOG_DIR",
			"SEARCH_LOG_DIR", "/test/log/search",
			GetLogDir,
			func() { SetLogDirOverride(""); os.Unsetenv("SEARCH_LOG_DIR") },
		},
		{
			"GetLogDir via LOG_DIR",
			"LOG_DIR", "/test/log2/search",
			GetLogDir,
			func() { SetLogDirOverride(""); os.Unsetenv("LOG_DIR") },
		},
		{
			"GetCacheDir via SEARCH_CACHE_DIR",
			"SEARCH_CACHE_DIR", "/test/cache/search",
			GetCacheDir,
			func() { SetCacheDirOverride(""); os.Unsetenv("SEARCH_CACHE_DIR") },
		},
		{
			"GetBackupDir via SEARCH_BACKUP_DIR",
			"SEARCH_BACKUP_DIR", "/test/backup/search",
			GetBackupDir,
			func() { SetBackupDirOverride(""); os.Unsetenv("SEARCH_BACKUP_DIR") },
		},
		{
			"GetBackupDir via BACKUP_DIR",
			"BACKUP_DIR", "/test/backup2/search",
			GetBackupDir,
			func() { SetBackupDirOverride(""); os.Unsetenv("BACKUP_DIR") },
		},
		{
			"GetPIDFile via SEARCH_PID_FILE",
			"SEARCH_PID_FILE", "/test/search.pid",
			GetPIDFile,
			func() { SetPIDFileOverride(""); os.Unsetenv("SEARCH_PID_FILE") },
		},
		{
			"GetPIDFile via PID_FILE",
			"PID_FILE", "/test/search2.pid",
			GetPIDFile,
			func() { SetPIDFileOverride(""); os.Unsetenv("PID_FILE") },
		},
		{
			"GetDatabaseDir via SEARCH_DATABASE_DIR",
			"SEARCH_DATABASE_DIR", "/test/db/search",
			GetDatabaseDir,
			func() { SetDatabaseDirOverride(""); os.Unsetenv("SEARCH_DATABASE_DIR") },
		},
		{
			"GetDatabaseDir via DATABASE_DIR",
			"DATABASE_DIR", "/test/db2/search",
			GetDatabaseDir,
			func() { SetDatabaseDirOverride(""); os.Unsetenv("DATABASE_DIR") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(tt.cleanup)
			os.Setenv(tt.envKey, tt.envVal)
			got := tt.fn()
			if got != tt.envVal {
				t.Errorf("%s: got %q, want %q", tt.name, got, tt.envVal)
			}
		})
	}
}
