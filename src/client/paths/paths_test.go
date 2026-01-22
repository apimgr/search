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

// Table-driven tests for ResolveConfigPath

func TestResolveConfigPathTableDriven(t *testing.T) {
	home, _ := os.UserHomeDir()
	tempDir := t.TempDir()

	// Create test files
	ymlFile := filepath.Join(tempDir, "exists.yml")
	yamlFile := filepath.Join(tempDir, "existsyaml.yaml")
	os.WriteFile(ymlFile, []byte{}, 0600)
	os.WriteFile(yamlFile, []byte{}, 0600)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input returns default",
			input:    "",
			expected: ConfigFile(),
		},
		{
			name:     "tilde expansion",
			input:    "~/test.yml",
			expected: filepath.Join(home, "test.yml"),
		},
		{
			name:     "absolute path with yml",
			input:    "/abs/path/config.yml",
			expected: "/abs/path/config.yml",
		},
		{
			name:     "absolute path with yaml",
			input:    "/abs/path/config.yaml",
			expected: "/abs/path/config.yaml",
		},
		{
			name:     "relative path without extension",
			input:    "myconfig",
			expected: filepath.Join(ConfigDir(), "myconfig.yml"),
		},
		{
			name:     "relative path with yml extension",
			input:    "myconfig.yml",
			expected: filepath.Join(ConfigDir(), "myconfig.yml"),
		},
		{
			name:     "relative path with yaml extension",
			input:    "myconfig.yaml",
			expected: filepath.Join(ConfigDir(), "myconfig.yaml"),
		},
		{
			name:     "relative path with other extension",
			input:    "myconfig.json",
			expected: filepath.Join(ConfigDir(), "myconfig.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ResolveConfigPath(tt.input)
			if err != nil {
				t.Fatalf("ResolveConfigPath(%q) error = %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("ResolveConfigPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Table-driven tests for addExtIfNeeded

func TestAddExtIfNeededTableDriven(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files for detection tests
	ymlExists := filepath.Join(tempDir, "hasyml")
	os.WriteFile(ymlExists+".yml", []byte{}, 0600)

	yamlExists := filepath.Join(tempDir, "hasyaml")
	os.WriteFile(yamlExists+".yaml", []byte{}, 0600)

	bothExist := filepath.Join(tempDir, "hasboth")
	os.WriteFile(bothExist+".yml", []byte{}, 0600)
	os.WriteFile(bothExist+".yaml", []byte{}, 0600)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "path with .yml extension",
			input:    "/some/path/config.yml",
			expected: "/some/path/config.yml",
		},
		{
			name:     "path with .yaml extension",
			input:    "/some/path/config.yaml",
			expected: "/some/path/config.yaml",
		},
		{
			name:     "path with .json extension",
			input:    "/some/path/config.json",
			expected: "/some/path/config.json",
		},
		{
			name:     "path with .toml extension",
			input:    "/some/path/config.toml",
			expected: "/some/path/config.toml",
		},
		{
			name:     "no extension - yml exists",
			input:    ymlExists,
			expected: ymlExists + ".yml",
		},
		{
			name:     "no extension - yaml exists",
			input:    yamlExists,
			expected: yamlExists + ".yaml",
		},
		{
			name:     "no extension - both exist (prefers yml)",
			input:    bothExist,
			expected: bothExist + ".yml",
		},
		{
			name:     "no extension - neither exists (defaults yml)",
			input:    filepath.Join(tempDir, "nonexistent"),
			expected: filepath.Join(tempDir, "nonexistent.yml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := addExtIfNeeded(tt.input)
			if err != nil {
				t.Fatalf("addExtIfNeeded(%q) error = %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("addExtIfNeeded(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Error path tests for EnsureDirs

func TestEnsureDirsErrorMkdirAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	// Create a temp directory
	tempDir := t.TempDir()

	// Create a file that we'll try to use as a directory
	// This will cause MkdirAll to fail even as root
	blockingFile := filepath.Join(tempDir, "blocking")
	if err := os.WriteFile(blockingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create blocking file: %v", err)
	}

	// Save original HOME
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	// Set HOME to the file, which will cause ConfigDir() to return a path
	// that can't be created (file is not a directory)
	os.Setenv("HOME", blockingFile)

	// EnsureDirs should fail when it tries to create directories
	err := EnsureDirs()
	if err == nil {
		t.Error("EnsureDirs() should return error when MkdirAll fails")
	}
	if err != nil && !strings.Contains(err.Error(), "create dir") {
		t.Errorf("EnsureDirs() error = %v, should contain 'create dir'", err)
	}
}

func TestEnsureDirsErrorChmod(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping chmod test on Windows")
	}

	// This test is difficult to trigger reliably since Chmod rarely fails
	// on directories we own. We'll verify the success path with actual
	// directory creation and permission setting.

	tempDir := t.TempDir()

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempDir)

	// First call should succeed
	err := EnsureDirs()
	if err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}

	// Verify directories were created with correct permissions
	configDir := filepath.Join(tempDir, ".config", projectOrg, projectName)
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", configDir, err)
	}
	// Check permissions (0700 = drwx------)
	if info.Mode().Perm() != 0700 {
		t.Errorf("ConfigDir permissions = %o, want 0700", info.Mode().Perm())
	}

	// Second call should also succeed (directory exists, chmod still works)
	err = EnsureDirs()
	if err != nil {
		t.Errorf("EnsureDirs() second call error = %v", err)
	}
}

// Error path tests for EnsureFile

func TestEnsureFileErrorMkdirAll(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	// Create a temp directory
	tempDir := t.TempDir()

	// Create a file that we'll try to use as a directory
	// This will cause MkdirAll to fail even as root
	blockingFile := filepath.Join(tempDir, "blocking")
	if err := os.WriteFile(blockingFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create blocking file: %v", err)
	}

	// Try to create a file in a subdirectory of the file (impossible)
	testFile := filepath.Join(blockingFile, "subdir", "test.txt")

	err := EnsureFile(testFile, 0600)
	if err == nil {
		t.Error("EnsureFile() should return error when MkdirAll fails")
	}
	if err != nil && !strings.Contains(err.Error(), "create parent dir") {
		t.Errorf("EnsureFile() error = %v, should contain 'create parent dir'", err)
	}
}

// Edge cases for EnsureFile

func TestEnsureFileDeepNesting(t *testing.T) {
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "a", "b", "c", "d", "e", "test.txt")

	err := EnsureFile(testFile, 0600)
	if err != nil {
		t.Fatalf("EnsureFile() error = %v", err)
	}

	// All parent directories should exist
	parentDir := filepath.Dir(testFile)
	if _, err := os.Stat(parentDir); os.IsNotExist(err) {
		t.Error("EnsureFile() should create deeply nested parent directories")
	}
}

func TestEnsureFileRootPath(t *testing.T) {
	// Test with a file directly in temp directory (minimal nesting)
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")

	err := EnsureFile(testFile, 0600)
	if err != nil {
		t.Fatalf("EnsureFile() error = %v", err)
	}
}

// Edge cases for ResolveConfigPath

func TestResolveConfigPathAbsoluteNoExtension(t *testing.T) {
	tempDir := t.TempDir()

	// Create a yml file
	ymlFile := filepath.Join(tempDir, "absconfig.yml")
	os.WriteFile(ymlFile, []byte{}, 0600)

	// Pass absolute path without extension
	input := filepath.Join(tempDir, "absconfig")
	path, err := ResolveConfigPath(input)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	// Should find the existing .yml file
	if path != ymlFile {
		t.Errorf("ResolveConfigPath(%q) = %q, want %q", input, path, ymlFile)
	}
}

func TestResolveConfigPathAbsoluteYamlOnly(t *testing.T) {
	tempDir := t.TempDir()

	// Create only a yaml file
	yamlFile := filepath.Join(tempDir, "absconfig.yaml")
	os.WriteFile(yamlFile, []byte{}, 0600)

	// Pass absolute path without extension
	input := filepath.Join(tempDir, "absconfig")
	path, err := ResolveConfigPath(input)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	// Should find the existing .yaml file
	if path != yamlFile {
		t.Errorf("ResolveConfigPath(%q) = %q, want %q", input, path, yamlFile)
	}
}

func TestResolveConfigPathTildeWithSubdirs(t *testing.T) {
	home, _ := os.UserHomeDir()
	input := "~/configs/custom.yml"

	path, err := ResolveConfigPath(input)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	expected := filepath.Join(home, "configs", "custom.yml")
	if path != expected {
		t.Errorf("ResolveConfigPath(%q) = %q, want %q", input, path, expected)
	}
}

func TestResolveConfigPathTildeNoExtension(t *testing.T) {
	home, _ := os.UserHomeDir()
	tempDir := t.TempDir()

	// Create the test file structure
	testDir := filepath.Join(tempDir, "tildetest")
	os.MkdirAll(testDir, 0755)

	// Temporarily change HOME to use our temp directory
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tempDir)
	defer os.Setenv("HOME", origHome)

	// Create a yaml file for detection
	yamlFile := filepath.Join(testDir, "config.yaml")
	os.WriteFile(yamlFile, []byte{}, 0600)

	input := "~/tildetest/config"
	path, err := ResolveConfigPath(input)
	if err != nil {
		t.Fatalf("ResolveConfigPath() error = %v", err)
	}

	// Should find the existing .yaml file
	expected := filepath.Join(tempDir, "tildetest", "config.yaml")
	if path != expected {
		t.Errorf("ResolveConfigPath(%q) = %q, want %q (home=%s)", input, path, expected, home)
	}
}

// Verify path separators are correct for current OS

func TestPathSeparators(t *testing.T) {
	sep := string(filepath.Separator)

	configDir := ConfigDir()
	if !strings.Contains(configDir, sep) {
		t.Errorf("ConfigDir() = %q, should contain path separator %q", configDir, sep)
	}

	dataDir := DataDir()
	if !strings.Contains(dataDir, sep) {
		t.Errorf("DataDir() = %q, should contain path separator %q", dataDir, sep)
	}

	cacheDir := CacheDir()
	if !strings.Contains(cacheDir, sep) {
		t.Errorf("CacheDir() = %q, should contain path separator %q", cacheDir, sep)
	}

	logDir := LogDir()
	if !strings.Contains(logDir, sep) {
		t.Errorf("LogDir() = %q, should contain path separator %q", logDir, sep)
	}
}

// Verify directories are absolute paths

func TestDirsAreAbsolute(t *testing.T) {
	dirs := []struct {
		name string
		fn   func() string
	}{
		{"ConfigDir", ConfigDir},
		{"DataDir", DataDir},
		{"CacheDir", CacheDir},
		{"LogDir", LogDir},
	}

	for _, d := range dirs {
		t.Run(d.name, func(t *testing.T) {
			path := d.fn()
			if !filepath.IsAbs(path) {
				t.Errorf("%s() = %q, should be absolute path", d.name, path)
			}
		})
	}
}

// Verify files are absolute paths

func TestFilesAreAbsolute(t *testing.T) {
	files := []struct {
		name string
		fn   func() string
	}{
		{"ConfigFile", ConfigFile},
		{"LogFile", LogFile},
	}

	for _, f := range files {
		t.Run(f.name, func(t *testing.T) {
			path := f.fn()
			if !filepath.IsAbs(path) {
				t.Errorf("%s() = %q, should be absolute path", f.name, path)
			}
		})
	}
}

// Test EnsureDirs creates all directories with correct permissions

func TestEnsureDirsCreatesAllDirs(t *testing.T) {
	tempDir := t.TempDir()

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempDir)

	err := EnsureDirs()
	if err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}

	expectedDirs := []string{
		filepath.Join(tempDir, ".config", projectOrg, projectName),
		filepath.Join(tempDir, ".local", "share", projectOrg, projectName),
		filepath.Join(tempDir, ".cache", projectOrg, projectName),
		filepath.Join(tempDir, ".local", "log", projectOrg, projectName),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			t.Errorf("EnsureDirs() should create %q", dir)
			continue
		}
		if err != nil {
			t.Errorf("Stat(%q) error = %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%q should be a directory", dir)
		}
		if info.Mode().Perm() != 0700 {
			t.Errorf("%q permissions = %o, want 0700", dir, info.Mode().Perm())
		}
	}
}

// Test EnsureDirs handles existing directories with wrong permissions

func TestEnsureDirsFixesPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	tempDir := t.TempDir()

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tempDir)

	// Pre-create config directory with wrong permissions
	configDir := filepath.Join(tempDir, ".config", projectOrg, projectName)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	// Call EnsureDirs - should fix permissions
	err := EnsureDirs()
	if err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}

	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", configDir, err)
	}
	if info.Mode().Perm() != 0700 {
		t.Errorf("EnsureDirs() should fix permissions, got %o, want 0700", info.Mode().Perm())
	}
}

// Test addExtIfNeeded with various path formats

func TestAddExtIfNeededEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "path ending with dot",
			input:    "/path/to/config.",
			expected: "/path/to/config.",
		},
		{
			name:     "path with multiple dots",
			input:    "/path/to/my.config.yml",
			expected: "/path/to/my.config.yml",
		},
		{
			name:     "path with unknown extension",
			input:    "/path/to/my.config",
			expected: "/path/to/my.config", // unknown extension - use as-is
		},
		{
			name:     "empty base name with yml",
			input:    "/path/to/.yml",
			expected: "/path/to/.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := addExtIfNeeded(tt.input)
			if err != nil {
				t.Fatalf("addExtIfNeeded(%q) error = %v", tt.input, err)
			}
			if result != tt.expected {
				t.Errorf("addExtIfNeeded(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// Test ConfigFile and LogFile composition

func TestConfigFileComposition(t *testing.T) {
	configDir := ConfigDir()
	configFile := ConfigFile()

	expectedFile := filepath.Join(configDir, "cli.yml")
	if configFile != expectedFile {
		t.Errorf("ConfigFile() = %q, want %q", configFile, expectedFile)
	}
}

func TestLogFileComposition(t *testing.T) {
	logDir := LogDir()
	logFile := LogFile()

	expectedFile := filepath.Join(logDir, "cli.log")
	if logFile != expectedFile {
		t.Errorf("LogFile() = %q, want %q", logFile, expectedFile)
	}
}

// Verify directory hierarchy is correct

func TestDirectoryHierarchy(t *testing.T) {
	home, _ := os.UserHomeDir()

	if runtime.GOOS != "windows" {
		// On non-Windows, config should be under .config
		configDir := ConfigDir()
		expectedPrefix := filepath.Join(home, ".config")
		if !strings.HasPrefix(configDir, expectedPrefix) {
			t.Errorf("ConfigDir() = %q, should start with %q", configDir, expectedPrefix)
		}

		// Data should be under .local/share
		dataDir := DataDir()
		expectedPrefix = filepath.Join(home, ".local", "share")
		if !strings.HasPrefix(dataDir, expectedPrefix) {
			t.Errorf("DataDir() = %q, should start with %q", dataDir, expectedPrefix)
		}

		// Cache should be under .cache
		cacheDir := CacheDir()
		expectedPrefix = filepath.Join(home, ".cache")
		if !strings.HasPrefix(cacheDir, expectedPrefix) {
			t.Errorf("CacheDir() = %q, should start with %q", cacheDir, expectedPrefix)
		}

		// Log should be under .local/log
		logDir := LogDir()
		expectedPrefix = filepath.Join(home, ".local", "log")
		if !strings.HasPrefix(logDir, expectedPrefix) {
			t.Errorf("LogDir() = %q, should start with %q", logDir, expectedPrefix)
		}
	}
}
