package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/search/src/security"
	"gopkg.in/yaml.v3"
)

func TestEncryptDNSCredentialsAtRest(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.SSL.LetsEncrypt.DNSCredentials = map[string]string{"CF_DNS_API_TOKEN": "test-token-value"}
	if err := cfg.encryptDNSCredentials(); err != nil {
		t.Fatalf("encryptDNSCredentials() error = %v", err)
	}
	encoded := cfg.Server.SSL.LetsEncrypt.DNSCredentials["CF_DNS_API_TOKEN"]
	if !strings.HasPrefix(encoded, "enc:v1:") || strings.Contains(encoded, "test-token-value") {
		t.Fatalf("DNS credential was not encrypted: %q", encoded)
	}
	plaintext, err := security.DecryptAESGCM(cfg.Server.Security.EncryptionKey, strings.TrimPrefix(encoded, "enc:v1:"))
	if err != nil {
		t.Fatalf("decrypt encrypted DNS credential: %v", err)
	}
	if string(plaintext) != "test-token-value" || !cfg.needsSave {
		t.Fatalf("decrypted credential = %q, needsSave = %t", plaintext, cfg.needsSave)
	}
}

func TestPortPairConfigRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Server.Port = 8080
	cfg.Server.HTTPSPort = 8443
	path := filepath.Join(t.TempDir(), "server.yml")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `port: 8080,8443`) || strings.Contains(string(data), "https_port:") {
		t.Fatalf("saved port pair is not canonical: %s", data)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Server.Port != 8080 || loaded.Server.HTTPSPort != 8443 {
		t.Fatalf("loaded ports = %d,%d, want 8080,8443", loaded.Server.Port, loaded.Server.HTTPSPort)
	}
}

func TestConfigSaveMigratesGeneratedSecretsAndPreservesOperatorSecrets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	secretPath := filepath.Join(dir, "secrets.yml")
	generatedToken := strings.Repeat("a", 64)
	if err := os.WriteFile(path, []byte("server:\n  token: "+generatedToken+"\n  pages:\n    contact:\n      captcha_secret_key: operator-captcha-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte("server:\n  notifications:\n    email:\n      smtp:\n        password: smtp-password\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	mainConfig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	secretConfig, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mainConfig), generatedToken) || strings.Contains(string(mainConfig), "smtp-password") || strings.Contains(string(mainConfig), "operator-captcha-secret") {
		t.Fatalf("server.yml must not contain credentials: %s", mainConfig)
	}
	for _, expected := range []string{generatedToken, "smtp-password", "operator-captcha-secret"} {
		if !strings.Contains(string(secretConfig), expected) {
			t.Fatalf("secrets.yml missing preserved value %q: %s", expected, secretConfig)
		}
	}
	if mode := fileMode(t, secretPath); mode != 0600 {
		t.Fatalf("secrets.yml mode = %04o, want 0600", mode)
	}
}

func TestConfigSavePreservesOperatorTokenInServerConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	const operatorToken = "operator-managed-token"
	if err := os.WriteFile(path, []byte("server:\n  token: "+operatorToken+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	mainConfig, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mainConfig), operatorToken) {
		t.Fatalf("operator-managed token remains in server.yml: %s", mainConfig)
	}
	secretConfig, err := os.ReadFile(filepath.Join(dir, "secrets.yml"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read secrets.yml: %v", err)
	}
	if !strings.Contains(string(secretConfig), operatorToken) {
		t.Fatalf("operator-managed token was not moved to secrets.yml: %s", secretConfig)
	}
}

func TestConfigLoadCorrectsConfigAndDropInModes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  port: 8080\n"), 0644); err != nil {
		t.Fatal(err)
	}
	dropInDir := filepath.Join(dir, "yaml.d")
	if err := os.Mkdir(dropInDir, 0755); err != nil {
		t.Fatal(err)
	}
	dropIn := filepath.Join(dropInDir, "10-extra.yml")
	if err := os.WriteFile(dropIn, []byte("search:\n  results_per_page: 20\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	for _, filename := range []string{path, dropIn} {
		if mode := fileMode(t, filename); mode != 0600 {
			t.Errorf("%s mode = %04o, want 0600", filename, mode)
		}
	}
	if mode := fileMode(t, dropInDir); mode != 0700 {
		t.Errorf("yaml.d mode = %04o, want 0700", mode)
	}
}

func TestGeneratedLogConfigUsesSharedDefaultsAndOnlyOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yml")
	cfg := DefaultConfig()
	cfg.Server.Logs.Level = "debug"
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	logsPath := filepath.Join(filepath.Dir(path), "yaml.d", "logs.yml")
	data, err := os.ReadFile(logsPath)
	if err != nil {
		t.Fatalf("read generated logs area: %v", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse generated logs area: %v", err)
	}
	logs := mappingValue(mappingValue(document.Content[0], "server"), "logs")
	if got := mappingValue(logs, "level"); got == nil || got.Value != "debug" {
		t.Fatalf("logs override = %#v, want debug", got)
	}
	if mappingValue(logs, "defaults") != nil {
		t.Fatalf("default log settings should not be written: %#v", logs)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	if loaded.Server.Logs.Defaults.Rotate != "weekly,50MB" || loaded.Server.Logs.Defaults.Keep != "none" || loaded.Server.Logs.Access.Rotate != "monthly" || !loaded.Server.Logs.Audit.Enabled || loaded.Server.Logs.Audit.Rotate != "daily" {
		t.Fatalf("shared log defaults or overrides were not restored: %#v", loaded.Server.Logs)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestPeekServerPortReadsOnlyPortValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yml")
	content := "server:\n  port: \"8080,8443\"\n  address: 42\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	httpPort, httpsPort, found, err := PeekServerPort(path)
	if err != nil {
		t.Fatalf("PeekServerPort() error = %v", err)
	}
	if !found || httpPort != 8080 || httpsPort != 8443 {
		t.Fatalf("PeekServerPort() = %d,%d,%t, want 8080,8443,true", httpPort, httpsPort, found)
	}
}

func TestPeekServerAddressReadsOnlyAddressValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.yml")
	content := "server:\n  address: \"[::]\"\n  port: invalid\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	address, found, err := PeekServerAddress(path)
	if err != nil {
		t.Fatalf("PeekServerAddress() error = %v", err)
	}
	if !found || address != "[::]" {
		t.Fatalf("PeekServerAddress() = %q,%t, want [::],true", address, found)
	}
}

func TestPeekServerPortAndAddressMergeOrderedDropIns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.yml")
	if err := os.WriteFile(path, []byte("server:\n  port: \"8080,8443\"\n  address: 127.0.0.1\n"), 0600); err != nil {
		t.Fatalf("write server.yml: %v", err)
	}
	dropInDir := filepath.Join(dir, "yaml.d")
	if err := os.Mkdir(dropInDir, 0700); err != nil {
		t.Fatalf("create yaml.d: %v", err)
	}
	for name, content := range map[string]string{
		"10-listen.yml": "server:\n  port: \"8181,8444\"\n  address: \"::1\"\n",
		"20-port.yml":   "server:\n  port: \"8282,9443\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dropInDir, name), []byte(content), 0600); err != nil {
			t.Fatalf("write drop-in %s: %v", name, err)
		}
	}

	httpPort, httpsPort, found, err := PeekServerPort(path)
	if err != nil {
		t.Fatalf("PeekServerPort() error = %v", err)
	}
	if !found || httpPort != 8282 || httpsPort != 9443 {
		t.Fatalf("PeekServerPort() = %d,%d,%t, want 8282,9443,true", httpPort, httpsPort, found)
	}
	address, found, err := PeekServerAddress(path)
	if err != nil {
		t.Fatalf("PeekServerAddress() error = %v", err)
	}
	if !found || address != "::1" {
		t.Fatalf("PeekServerAddress() = %q,%t, want ::1,true", address, found)
	}
}

func TestPeekServerSkipsMalformedFilesUntilFullLoad(t *testing.T) {
	for _, malformedBase := range []bool{false, true} {
		name := "malformed drop-in"
		if malformedBase {
			name = "malformed base config"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "server.yml")
			base := "server:\n  port: 8080\n  address: 127.0.0.1\n"
			if malformedBase {
				base = "server: [\n"
			}
			if err := os.WriteFile(path, []byte(base), 0600); err != nil {
				t.Fatalf("write server.yml: %v", err)
			}
			dropInDir := filepath.Join(dir, "yaml.d")
			if err := os.Mkdir(dropInDir, 0700); err != nil {
				t.Fatalf("create yaml.d: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dropInDir, "10-invalid.yml"), []byte("server: [\n"), 0600); err != nil {
				t.Fatalf("write malformed drop-in: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dropInDir, "20-listen.yml"), []byte("server:\n  port: 9090\n  address: \"[::1]\"\n"), 0600); err != nil {
				t.Fatalf("write valid drop-in: %v", err)
			}

			httpPort, _, found, err := PeekServerPort(path)
			if err != nil {
				t.Fatalf("PeekServerPort() error = %v", err)
			}
			if !found || httpPort != 9090 {
				t.Fatalf("PeekServerPort() = %d,%t, want 9090,true", httpPort, found)
			}
			address, found, err := PeekServerAddress(path)
			if err != nil {
				t.Fatalf("PeekServerAddress() error = %v", err)
			}
			if !found || address != "[::1]" {
				t.Fatalf("PeekServerAddress() = %q,%t, want [::1],true", address, found)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("Load() error = nil, want malformed YAML to be reported by full load")
			}
		})
	}
}

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
	os.Unsetenv("CONFIG_DIR")
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
	os.Unsetenv("DATA_DIR")
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
	os.Unsetenv("LOG_DIR")
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
	os.Unsetenv("BACKUP_DIR")
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
	os.Unsetenv("DATABASE_DIR")
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
			"GetConfigDir via CONFIG_DIR",
			"CONFIG_DIR", "/test/config2/search",
			GetConfigDir,
			func() { SetConfigDirOverride(""); os.Unsetenv("CONFIG_DIR") },
		},
		{
			"GetDataDir via DATA_DIR",
			"DATA_DIR", "/test/data2/search",
			GetDataDir,
			func() { SetDataDirOverride(""); os.Unsetenv("DATA_DIR") },
		},
		{
			"GetLogDir via LOG_DIR",
			"LOG_DIR", "/test/log2/search",
			GetLogDir,
			func() { SetLogDirOverride(""); os.Unsetenv("LOG_DIR") },
		},
		{
			"GetBackupDir via BACKUP_DIR",
			"BACKUP_DIR", "/test/backup2/search",
			GetBackupDir,
			func() { SetBackupDirOverride(""); os.Unsetenv("BACKUP_DIR") },
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
