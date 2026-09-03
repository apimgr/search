// Package path provides CLI directory and file path resolution
// Per AI.md PART 32: CLI paths follow XDG on Linux, standard locations on Windows
package path

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	projectOrg  = "apimgr"
	projectName = "search"
)

// osGetter returns the current OS. Abstracted for testing.
var osGetter = func() string { return runtime.GOOS }

// chmodFunc wraps os.Chmod for testing.
var chmodFunc = os.Chmod

// ConfigDir returns the CLI config directory.
// Linux: ~/.config/apimgr/search/
// Windows: %APPDATA%\apimgr\search\
func ConfigDir() (string, error) {
	if osGetter() == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), projectOrg, projectName), nil
	}
	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", projectOrg, projectName), nil
}

// getHomeDir returns the user's home directory.
// Uses HOME env var first (respects test overrides), falls back to os.UserHomeDir().
// Returns an error if no home directory can be determined.
func getHomeDir() (string, error) {
	// Check HOME env var first (allows test overrides)
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}
	// Fall back to os.UserHomeDir()
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return home, nil
	}
	return "", fmt.Errorf("cannot determine home directory: HOME not set and UserHomeDir failed")
}

// DataDir returns the CLI data directory.
// Linux: ~/.local/share/apimgr/search/
// Windows: %LOCALAPPDATA%\apimgr\search\data\
func DataDir() (string, error) {
	if osGetter() == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "data"), nil
	}
	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", projectOrg, projectName), nil
}

// CacheDir returns the CLI cache directory.
// Linux: ~/.cache/apimgr/search/
// Windows: %LOCALAPPDATA%\apimgr\search\cache\
func CacheDir() (string, error) {
	if osGetter() == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "cache"), nil
	}
	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", projectOrg, projectName), nil
}

// LogDir returns the CLI log directory.
// Linux: ~/.local/log/apimgr/search/
// Windows: %LOCALAPPDATA%\apimgr\search\log\
func LogDir() (string, error) {
	if osGetter() == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "log"), nil
	}
	home, err := getHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "log", projectOrg, projectName), nil
}

// ConfigFile returns the CLI config file path.
func ConfigFile() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli.yml"), nil
}

// LogFile returns the CLI log file path.
func LogFile() (string, error) {
	dir, err := LogDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cli.log"), nil
}

// EnsureDirs creates all CLI directories with correct permissions.
// Called on every startup before any file operations.
// Per AI.md PART 32: CLI Startup Sequence (NON-NEGOTIABLE)
func EnsureDirs() error {
	configDir, err := ConfigDir()
	if err != nil {
		return fmt.Errorf("resolve config dir: %w", err)
	}
	dataDir, err := DataDir()
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	cacheDir, err := CacheDir()
	if err != nil {
		return fmt.Errorf("resolve cache dir: %w", err)
	}
	logDir, err := LogDir()
	if err != nil {
		return fmt.Errorf("resolve log dir: %w", err)
	}

	dirs := []string{configDir, dataDir, cacheDir, logDir}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
		// Ensure permissions even if dir existed
		if err := chmodFunc(dir, 0700); err != nil {
			return fmt.Errorf("chmod dir %s: %w", dir, err)
		}
	}
	return nil
}

// EnsureFile creates parent dirs and sets permissions before writing.
// MUST be called before any file creation.
func EnsureFile(path string, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	return nil
}

// ResolveConfigPath resolves --config flag to absolute path
// Per AI.md PART 32: --config Flag (Config File Selection)
func ResolveConfigPath(configFlag string) (string, error) {
	if configFlag == "" {
		// Default: cli.yml
		return ConfigFile()
	}

	// Expand ~ to home directory
	if strings.HasPrefix(configFlag, "~/") {
		home, err := getHomeDir()
		if err != nil {
			return "", err
		}
		configFlag = filepath.Join(home, configFlag[2:])
	}

	// If absolute path - use as-is
	if filepath.IsAbs(configFlag) {
		return addExtIfNeeded(configFlag)
	}

	// Relative path - resolve from config dir
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	fullPath := filepath.Join(dir, configFlag)
	return addExtIfNeeded(fullPath)
}

// addExtIfNeeded adds .yml extension if no extension provided
func addExtIfNeeded(path string) (string, error) {
	ext := filepath.Ext(path)
	if ext == ".yml" || ext == ".yaml" {
		return path, nil
	}

	// No extension - try .yml first, then .yaml
	if ext == "" {
		ymlPath := path + ".yml"
		if _, err := os.Stat(ymlPath); err == nil {
			return ymlPath, nil
		}
		yamlPath := path + ".yaml"
		if _, err := os.Stat(yamlPath); err == nil {
			return yamlPath, nil
		}
		// Default to .yml for new files
		return ymlPath, nil
	}

	// Unknown extension - use as-is
	return path, nil
}
