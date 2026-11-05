//go:build windows

package update

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceBinaryReportsFailedOldBinaryCleanupScheduling(t *testing.T) {
	dir := t.TempDir()
	currentPath := filepath.Join(dir, "search.exe")
	newBinaryPath := filepath.Join(dir, "search.new")
	if err := os.WriteFile(currentPath, []byte("old binary"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newBinaryPath, []byte("new binary"), 0600); err != nil {
		t.Fatal(err)
	}

	originalSchedule := scheduleOldBinaryForDeletion
	scheduleOldBinaryForDeletion = func(string) error {
		return errors.New("simulated scheduling failure")
	}
	t.Cleanup(func() {
		scheduleOldBinaryForDeletion = originalSchedule
	})

	err := replaceBinary(currentPath, newBinaryPath)
	if err == nil || !strings.Contains(err.Error(), "simulated scheduling failure") {
		t.Fatalf("replaceBinary() error = %v, want cleanup scheduling failure", err)
	}

	installed, err := os.ReadFile(currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != "new binary" {
		t.Errorf("installed binary = %q, want new binary", installed)
	}
	previous, err := os.ReadFile(currentPath + ".old")
	if err != nil {
		t.Fatal(err)
	}
	if string(previous) != "old binary" {
		t.Errorf("old binary = %q, want old binary", previous)
	}
}
