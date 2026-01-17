package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests for ConfigDir

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()

	if dir == "" {
		t.Error("ConfigDir() returned empty string")
	}

	// Should contain project org and name
	if !strings.Contains(dir, projectOrg) {
		t.Errorf("ConfigDir() = %q, should contain %q", dir, projectOrg)
	}
	if !strings.Contains(dir, projectName) {
		t.Errorf("ConfigDir() = %q, should contain %q", dir, projectName)
	}
}

func TestConfigDirPlatformSpecific(t *testing.T) {
	dir := ConfigDir()

	if runtime.GOOS == "windows" {
		// Should use APPDATA on Windows
		appdata := os.Getenv("APPDATA")
		if appdata != "" && !strings.HasPrefix(dir, appdata) {
			t.Errorf("ConfigDir() on Windows should use APPDATA, got %q", dir)
		}
	} else {
		// Should use .config on non-Windows
		if !strings.Contains(dir, ".config") {
			t.Errorf("ConfigDir() on %s should use .config, got %q", runtime.GOOS, dir)
		}
	}
}

// Tests for DataDir

func TestDataDir(t *testing.T) {
	dir := DataDir()

	if dir == "" {
		t.Error("DataDir() returned empty string")
	}

	if !strings.Contains(dir, projectOrg) {
		t.Errorf("DataDir() = %q, should contain %q", dir, projectOrg)
	}
	if !strings.Contains(dir, projectName) {
		t.Errorf("DataDir() = %q, should contain %q", dir, projectName)
	}
}

func TestDataDirPlatformSpecific(t *testing.T) {
	dir := DataDir()

	if runtime.GOOS == "windows" {
		// Should use LOCALAPPDATA on Windows
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" && !strings.HasPrefix(dir, localAppData) {
			t.Errorf("DataDir() on Windows should use LOCALAPPDATA, got %q", dir)
		}
		// Should have 'data' subdirectory
		if !strings.HasSuffix(dir, "data") {
			t.Errorf("DataDir() on Windows should end with 'data', got %q", dir)
		}
	} else {
		// Should use .local/share on non-Windows
		if !strings.Contains(dir, ".local/share") && !strings.Contains(dir, ".local"+string(filepath.Separator)+"share") {
			t.Errorf("DataDir() on %s should use .local/share, got %q", runtime.GOOS, dir)
		}
	}
}

// Tests for CacheDir

func TestCacheDir(t *testing.T) {
	dir := CacheDir()

	if dir == "" {
		t.Error("CacheDir() returned empty string")
	}

	if !strings.Contains(dir, projectOrg) {
		t.Errorf("CacheDir() = %q, should contain %q", dir, projectOrg)
	}
	if !strings.Contains(dir, projectName) {
		t.Errorf("CacheDir() = %q, should contain %q", dir, projectName)
	}
}

func TestCacheDirPlatformSpecific(t *testing.T) {
	dir := CacheDir()

	if runtime.GOOS == "windows" {
		// Should use LOCALAPPDATA on Windows
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData != "" && !strings.HasPrefix(dir, localAppData) {
			t.Errorf("CacheDir() on Windows should use LOCALAPPDATA, got %q", dir)
		}
		// Should have 'cache' subdirectory
		if !strings.HasSuffix(dir, "cache") {
			t.Errorf("CacheDir() on Windows should end with 'cache', got %q", dir)
		}
	} else {
		// Should use .cache on non-Windows
		if !strings.Contains(dir, ".cache") {
			t.Errorf("CacheDir() on %s should use .cache, got %q", runtime.GOOS, dir)
		}
	}
}

// Tests for LogDir

func TestLogDir(t *testing.T) {
	dir := LogDir()

	if dir == "" {
		t.Error("LogDir() returned empty string")
	}

	if !strings.Contains(dir, projectOrg) {
		t.Errorf("LogDir() = %q, should contain %q", dir, projectOrg)
	}
	if !strings.Contains(dir, projectName) {
		t.Errorf("LogDir() = %q, should contain %q", dir, projectName)
	}
}

func TestLogDirPlatformSpecific(t *testing.T) {
	dir := LogDir()

	if runtime.GOOS == "windows" {
		// Should have 'log' subdirectory
		if !strings.HasSuffix(dir, "log") {
			t.Errorf("LogDir() on Windows should end with 'log', got %q", dir)
		}
	} else {
		// Should use .local/log on non-Windows
		if !strings.Contains(dir, ".local/log") && !strings.Contains(dir, ".local"+string(filepath.Separator)+"log") {
			t.Errorf("LogDir() on %s should use .local/log, got %q", runtime.GOOS, dir)
		}
	}
}

// Tests for ConfigFile

func TestConfigFile(t *testing.T) {
	file := ConfigFile()

	if file == "" {
		t.Error("ConfigFile() returned empty string")
	}

	// Should end with cli.yml
	if !strings.HasSuffix(file, "cli.yml") {
		t.Errorf("ConfigFile() = %q, should end with cli.yml", file)
	}

	// Should be under ConfigDir
	if !strings.HasPrefix(file, ConfigDir()) {
		t.Errorf("ConfigFile() = %q, should be under ConfigDir() = %q", file, ConfigDir())
	}
}

// Tests for LogFile

func TestLogFile(t *testing.T) {
	file := LogFile()

	if file == "" {
		t.Error("LogFile() returned empty string")
	}

	// Should end with cli.log
	if !strings.HasSuffix(file, "cli.log") {
		t.Errorf("LogFile() = %q, should end with cli.log", file)
	}

	// Should be under LogDir
	if !strings.HasPrefix(file, LogDir()) {
		t.Errorf("LogFile() = %q, should be under LogDir() = %q", file, LogDir())
	}
}

// Tests for EnsureDirs

func TestEnsureDirs(t *testing.T) {
	// Create temp dir to avoid modifying real dirs
	tempDir := t.TempDir()

	// Temporarily change home for testing
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	err := EnsureDirs()
	if err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}

	// Verify directories were created
	// Note: The actual directories depend on the HOME env var
}

// Tests for EnsureFile

func TestEnsureFile(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "subdir", "test.txt")

	err := EnsureFile(testFile, 0600)
	if err != nil {
		t.Fatalf("EnsureFile() error = %v", err)
	}

	// Parent directory should exist
	parentDir := filepath.Dir(testFile)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		t.Error("EnsureFile() should create parent directory")
	}
}

func TestEnsureFileExistingDir(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := EnsureFile(testFile, 0600)
	if err != nil {
		t.Fatalf("EnsureFile() error = %v", err)
	}
}

// Tests for ResolveConfigPath

func TestResolveConfigPathEmpty(t *testing.T) {
	path, err := ResolveConfigPath("")
	if err != nil {
		t.Fatalf("ResolveConfigPath('') error = %v", err)
	}

	// Should return default ConfigFile
	if path != ConfigFile() {
		t.Errorf("ResolveConfigPath('') = %q, want %q", path, ConfigFile())
	}
}

func TestResolveConfigPathWithExtension(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "custom.yml")

	// Create the file
	os.WriteFile(testFile, []byte{}, 0600)

	path, err := ResolveConfigPath(testFile)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	if path != testFile {
		t.Errorf("ResolveConfigPath() = %q, want %q", path, testFile)
	}
}

func TestResolveConfigPathWithYamlExtension(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "custom.yaml")

	// Create the file
	os.WriteFile(testFile, []byte{}, 0600)

	path, err := ResolveConfigPath(testFile)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	if path != testFile {
		t.Errorf("ResolveConfigPath() = %q, want %q", path, testFile)
	}
}

func TestResolveConfigPathTildeExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	input := "~/custom.yml"

	path, err := ResolveConfigPath(input)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	expected := filepath.Join(home, "custom.yml")
	if path != expected {
		t.Errorf("ResolveConfigPath('~/custom.yml') = %q, want %q", path, expected)
	}
}

func TestResolveConfigPathRelative(t *testing.T) {
	path, err := ResolveConfigPath("myconfig")
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	// Should be resolved relative to ConfigDir with .yml extension
	expected := filepath.Join(ConfigDir(), "myconfig.yml")
	if path != expected {
		t.Errorf("ResolveConfigPath('myconfig') = %q, want %q", path, expected)
	}
}

// Tests for addExtIfNeeded

func TestAddExtIfNeededYml(t *testing.T) {
	input := "/path/to/config.yml"
	result, err := addExtIfNeeded(input)
	if err != nil {
		t.Fatalf("addExtIfNeeded() error = %v", err)
	}
	if result != input {
		t.Errorf("addExtIfNeeded(%q) = %q, want %q", input, result, input)
	}
}

func TestAddExtIfNeededYaml(t *testing.T) {
	input := "/path/to/config.yaml"
	result, err := addExtIfNeeded(input)
	if err != nil {
		t.Fatalf("addExtIfNeeded() error = %v", err)
	}
	if result != input {
		t.Errorf("addExtIfNeeded(%q) = %q, want %q", input, result, input)
	}
}

func TestAddExtIfNeededNoExt(t *testing.T) {
	tempDir := t.TempDir()

	// Create a .yml file
	ymlFile := filepath.Join(tempDir, "config.yml")
	os.WriteFile(ymlFile, []byte{}, 0600)

	input := filepath.Join(tempDir, "config")
	result, err := addExtIfNeeded(input)
	if err != nil {
		t.Fatalf("addExtIfNeeded() error = %v", err)
	}

	if result != ymlFile {
		t.Errorf("addExtIfNeeded(%q) = %q, want %q", input, result, ymlFile)
	}
}

func TestAddExtIfNeededNoExtYamlExists(t *testing.T) {
	tempDir := t.TempDir()

	// Create only a .yaml file (not .yml)
	yamlFile := filepath.Join(tempDir, "config.yaml")
	os.WriteFile(yamlFile, []byte{}, 0600)

	input := filepath.Join(tempDir, "config")
	result, err := addExtIfNeeded(input)
	if err != nil {
		t.Fatalf("addExtIfNeeded() error = %v", err)
	}

	if result != yamlFile {
		t.Errorf("addExtIfNeeded(%q) = %q, want %q", input, result, yamlFile)
	}
}

func TestAddExtIfNeededNoExtNeitherExists(t *testing.T) {
	tempDir := t.TempDir()

	input := filepath.Join(tempDir, "newconfig")
	result, err := addExtIfNeeded(input)
	if err != nil {
		t.Fatalf("addExtIfNeeded() error = %v", err)
	}

	// Should default to .yml
	expected := input + ".yml"
	if result != expected {
		t.Errorf("addExtIfNeeded(%q) = %q, want %q", input, result, expected)
	}
}

func TestAddExtIfNeededOtherExtension(t *testing.T) {
	input := "/path/to/config.json"
	result, err := addExtIfNeeded(input)
	if err != nil {
		t.Fatalf("addExtIfNeeded() error = %v", err)
	}

	// Unknown extension - use as-is
	if result != input {
		t.Errorf("addExtIfNeeded(%q) = %q, want %q", input, result, input)
	}
}

// Tests for directory structure consistency

func TestAllDirsAreDifferent(t *testing.T) {
	configDir := ConfigDir()
	dataDir := DataDir()
	cacheDir := CacheDir()
	logDir := LogDir()

	if configDir == dataDir {
		t.Error("ConfigDir and DataDir should be different")
	}
	if configDir == cacheDir {
		t.Error("ConfigDir and CacheDir should be different")
	}
	if configDir == logDir {
		t.Error("ConfigDir and LogDir should be different")
	}
	if dataDir == cacheDir {
		t.Error("DataDir and CacheDir should be different")
	}
	if dataDir == logDir {
		t.Error("DataDir and LogDir should be different")
	}
	if cacheDir == logDir {
		t.Error("CacheDir and LogDir should be different")
	}
}

func TestConstantsValues(t *testing.T) {
	if projectOrg != "apimgr" {
		t.Errorf("projectOrg = %q, want 'apimgr'", projectOrg)
	}
	if projectName != "search" {
		t.Errorf("projectName = %q, want 'search'", projectName)
	}
}
