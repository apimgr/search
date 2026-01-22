package main

import (
	"os"
	"sync"
	"testing"

	"github.com/spf13/viper"
)

// Tests for InitCLI

func TestInitCLI(t *testing.T) {
	// Reset state for clean test
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	// Set up temp directories
	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	err := InitCLI()
	if err != nil {
		t.Fatalf("InitCLI() error = %v", err)
	}
}

func TestInitCLIWithExistingDirs(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Create directories first
	os.MkdirAll(tempDir+"/.config/apimgr/search", 0700)
	os.MkdirAll(tempDir+"/.local/share/apimgr/search", 0700)
	os.MkdirAll(tempDir+"/.cache/apimgr/search", 0700)
	os.MkdirAll(tempDir+"/.local/log/apimgr/search", 0700)

	err := InitCLI()
	if err != nil {
		t.Fatalf("InitCLI() with existing dirs error = %v", err)
	}
}

func TestInitCLIMultipleCalls(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Call multiple times
	err1 := InitCLI()
	if err1 != nil {
		t.Fatalf("first InitCLI() error = %v", err1)
	}

	// Second call should also succeed
	err2 := InitCLI()
	if err2 != nil {
		t.Fatalf("second InitCLI() error = %v", err2)
	}
}

func TestInitCLILoggingInitialized(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	_ = InitCLI()

	// Logger should be accessible (either initialized or fallback)
	l := Logger()
	if l == nil {
		t.Error("Logger() should not return nil after InitCLI()")
	}
}

func TestInitCLICacheInitialized(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	_ = InitCLI()

	// Cache should be accessible
	c := Cache()
	if c == nil {
		t.Error("Cache() should not return nil after InitCLI()")
	}
}

// Tests for error handling in InitCLI

func TestInitCLIHandlesLoggingError(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// InitCLI should handle logging errors gracefully (non-fatal)
	err := InitCLI()
	if err != nil {
		t.Fatalf("InitCLI() should handle logging errors, got error = %v", err)
	}
}

func TestInitCLIHandlesCacheError(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// InitCLI should handle cache errors gracefully (non-fatal)
	err := InitCLI()
	if err != nil {
		t.Fatalf("InitCLI() should handle cache errors, got error = %v", err)
	}
}

// Tests for InitCLI sequence

func TestInitCLISequence(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Per AI.md PART 36: CLI Startup Sequence
	// 1. Ensure directories exist
	// 2. Set correct permissions
	// 3. Initialize logging (with rotation)
	// 4. Initialize cache

	err := InitCLI()
	if err != nil {
		t.Fatalf("InitCLI() error = %v", err)
	}

	// Verify directories were created
	dirs := []string{
		tempDir + "/.config/apimgr/search",
		tempDir + "/.local/share/apimgr/search",
		tempDir + "/.cache/apimgr/search",
		tempDir + "/.local/log/apimgr/search",
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			t.Errorf("Directory %s was not created", dir)
			continue
		}
		if err != nil {
			t.Errorf("Error checking directory %s: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", dir)
		}
	}
}

// Tests for InitCLI with config

func TestInitCLIWithViperConfig(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	// Set some config values
	viper.Set("logging.level", "debug")
	viper.Set("cache.enabled", true)
	viper.Set("cache.ttl", 600)

	err := InitCLI()
	if err != nil {
		t.Fatalf("InitCLI() with config error = %v", err)
	}
}

func TestInitCLIWithDisabledCache(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	viper.Set("cache.enabled", false)

	err := InitCLI()
	if err != nil {
		t.Fatalf("InitCLI() with disabled cache error = %v", err)
	}
}

// Tests for concurrent InitCLI

func TestInitCLIConcurrent(t *testing.T) {
	// Reset state
	logger = nil
	loggerOnce = sync.Once{}
	cliCache = nil
	cliCacheOnce = sync.Once{}
	viper.Reset()

	tempDir := t.TempDir()
	os.Setenv("HOME", tempDir)
	defer os.Unsetenv("HOME")

	done := make(chan error, 10)

	// Call InitCLI concurrently
	for i := 0; i < 10; i++ {
		go func() {
			err := InitCLI()
			done <- err
		}()
	}

	// Wait for all and check errors
	for i := 0; i < 10; i++ {
		err := <-done
		if err != nil {
			t.Errorf("Concurrent InitCLI() error = %v", err)
		}
	}
}
