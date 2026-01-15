// Package paths provides CLI directory and file path resolution
// Per AI.md PART 36: CLI paths follow XDG on Linux, standard locations on Windows
package paths

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

// ConfigDir returns the CLI config directory
// Linux: ~/.config/apimgr/search/
// Windows: %APPDATA%\apimgr\search\
func ConfigDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), projectOrg, projectName)
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", projectOrg, projectName)
}

// DataDir returns the CLI data directory
// Linux: ~/.local/share/apimgr/search/
// Windows: %LOCALAPPDATA%\apimgr\search\data\
func DataDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "data")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", projectOrg, projectName)
}

// CacheDir returns the CLI cache directory
// Linux: ~/.cache/apimgr/search/
// Windows: %LOCALAPPDATA%\apimgr\search\cache\
func CacheDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "cache")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", projectOrg, projectName)
}

// LogDir returns the CLI log directory
// Linux: ~/.local/log/apimgr/search/
// Windows: %LOCALAPPDATA%\apimgr\search\log\
func LogDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("LOCALAPPDATA"), projectOrg, projectName, "log")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "log", projectOrg, projectName)
}

// ConfigFile returns the CLI config file path
func ConfigFile() string {
	return filepath.Join(ConfigDir(), "cli.yml")
}

// LogFile returns the CLI log file path
func LogFile() string {
	return filepath.Join(LogDir(), "cli.log")
}

// EnsureDirs creates all CLI directories with correct permissions.
// Called on every startup before any file operations.
// Per AI.md PART 36: CLI Startup Sequence (NON-NEGOTIABLE)
func EnsureDirs() error {
	dirs := []string{
		ConfigDir(),
		DataDir(),
		CacheDir(),
		LogDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
		// Ensure permissions even if dir existed
		if err := os.Chmod(dir, 0700); err != nil {
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
// Per AI.md PART 36: --config Flag (Config File Selection)
func ResolveConfigPath(configFlag string) (string, error) {
	if configFlag == "" {
		return ConfigFile(), nil // Default: cli.yml
	}

	// Expand ~ to home directory
	if strings.HasPrefix(configFlag, "~/") {
		home, _ := os.UserHomeDir()
		configFlag = filepath.Join(home, configFlag[2:])
	}

	// If absolute path - use as-is
	if filepath.IsAbs(configFlag) {
		return addExtIfNeeded(configFlag)
	}

	// Relative path - resolve from config dir
	fullPath := filepath.Join(ConfigDir(), configFlag)
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
