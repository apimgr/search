package main

import (
	"os"
	"testing"
)

func TestValidateRuntimeDirectoryEnvironmentPaths(t *testing.T) {
	originalCache, originalPID := flagCache, flagPID
	flagCache, flagPID = "", ""
	t.Cleanup(func() { flagCache, flagPID = originalCache, originalPID })
	t.Setenv("CACHE_DIR", "relative/store")
	t.Setenv("PID_FILE", "relative/search.pid")

	if err := validateCLIPathOverrides(); err != nil {
		t.Fatalf("validateCLIPathOverrides() error = %v", err)
	}
	if got := os.Getenv("CACHE_DIR"); got != "relative/store" {
		t.Errorf("CACHE_DIR = %q, want relative/store", got)
	}
	if got := os.Getenv("PID_FILE"); got != "relative/search.pid" {
		t.Errorf("PID_FILE = %q, want relative/search.pid", got)
	}
}

func TestValidateRuntimeDirectoryEnvironmentRejectsTraversal(t *testing.T) {
	originalCache, originalPID := flagCache, flagPID
	flagCache, flagPID = "", ""
	t.Cleanup(func() { flagCache, flagPID = originalCache, originalPID })
	t.Setenv("CACHE_DIR", "../outside")
	t.Setenv("PID_FILE", "")

	if err := validateCLIPathOverrides(); err == nil {
		t.Fatal("validateCLIPathOverrides() error = nil, want unsafe CACHE_DIR rejection")
	}
}
