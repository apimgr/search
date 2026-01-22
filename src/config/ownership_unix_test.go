//go:build !windows

package config

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsPrivileged(t *testing.T) {
	// This test just verifies the function returns a bool without panic
	result := IsPrivileged()

	// If running as root (euid=0), should return true
	if os.Geteuid() == 0 && !result {
		t.Error("IsPrivileged() should return true when running as root")
	}

	// If not running as root, should return false
	if os.Geteuid() != 0 && result {
		t.Error("IsPrivileged() should return false when not running as root")
	}
}

func TestGetCurrentUID(t *testing.T) {
	got := GetCurrentUID()
	want := os.Getuid()

	if got != want {
		t.Errorf("GetCurrentUID() = %d, want %d", got, want)
	}
}

func TestGetCurrentGID(t *testing.T) {
	got := GetCurrentGID()
	want := os.Getgid()

	if got != want {
		t.Errorf("GetCurrentGID() = %d, want %d", got, want)
	}
}

func TestSetOwnership(t *testing.T) {
	// Skip if not running as root (can only chown as root)
	if os.Getuid() != 0 {
		t.Skip("setOwnership test requires root privileges")
	}

	tmpDir, err := os.MkdirTemp("", "ownership-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := tmpDir + "/testfile"
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = setOwnership(testFile)
	if err != nil {
		t.Errorf("setOwnership() error = %v", err)
	}
}

func TestSetOwnershipNonRoot(t *testing.T) {
	// When not root, setOwnership should return nil without doing anything
	if os.Getuid() == 0 {
		t.Skip("This test is for non-root users")
	}

	tmpDir, err := os.MkdirTemp("", "ownership-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := tmpDir + "/testfile"
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Should return nil without error when not root
	err = setOwnership(testFile)
	if err != nil {
		t.Errorf("setOwnership() as non-root should return nil, got error = %v", err)
	}
}

func TestSetFileOwnership(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("SetFileOwnership test requires root privileges")
	}

	tmpDir, err := os.MkdirTemp("", "ownership-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := tmpDir + "/testfile"
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uid := os.Getuid()
	gid := os.Getgid()

	err = SetFileOwnership(testFile, uid, gid)
	if err != nil {
		t.Errorf("SetFileOwnership() error = %v", err)
	}
}

func TestGetFileOwnership(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "ownership-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testFile := tmpDir + "/testfile"
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	uid, gid, err := GetFileOwnership(testFile)
	if err != nil {
		t.Errorf("GetFileOwnership() error = %v", err)
	}

	// On Unix, newly created files should be owned by the current user
	expectedUID := os.Getuid()
	expectedGID := os.Getgid()

	if uid != expectedUID {
		t.Errorf("GetFileOwnership() uid = %d, want %d", uid, expectedUID)
	}
	if gid != expectedGID {
		t.Errorf("GetFileOwnership() gid = %d, want %d", gid, expectedGID)
	}
}

func TestGetFileOwnershipNonExistent(t *testing.T) {
	_, _, err := GetFileOwnership("/nonexistent/file/path")
	if err == nil {
		t.Error("GetFileOwnership() should error for nonexistent file")
	}
}

func TestChownPath(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("chownPath test requires root privileges")
	}

	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		t.Fatalf("Failed to get current user: %v", err)
	}

	tmpDir, err := os.MkdirTemp("", "chown-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file in the directory
	testFile := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err = chownPath(tmpDir, currentUser.Username)
	if err != nil {
		t.Errorf("chownPath() error = %v", err)
	}
}

func TestChownPathInvalidUser(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("chownPath test requires root privileges")
	}

	tmpDir, err := os.MkdirTemp("", "chown-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = chownPath(tmpDir, "nonexistent_user_12345")
	if err == nil {
		t.Error("chownPath() should error for nonexistent user")
	}
}

func TestPlatformSpecificFunctions(t *testing.T) {
	// Verify we're on Unix
	if runtime.GOOS == "windows" {
		t.Skip("These tests are Unix-specific")
	}

	// These functions should work without panicking
	_ = IsPrivileged()
	_ = GetCurrentUID()
	_ = GetCurrentGID()
}
