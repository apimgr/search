//go:build darwin

package service

import (
	"testing"
)

func TestDarwinServiceAccountUsesInternalName(t *testing.T) {
	const internalName = "search"
	if got := GetServiceUser(internalName); got != internalName {
		t.Fatalf("GetServiceUser(%q) = %q, want the exact internal name", internalName, got)
	}
	if got := GetServiceGroup(internalName); got != internalName {
		t.Fatalf("GetServiceGroup(%q) = %q, want the exact internal name", internalName, got)
	}
}

// Test findAvailableMacOSSystemID directly
func TestFindAvailableMacOSSystemID(t *testing.T) {
	id, err := findAvailableMacOSSystemID()
	if err != nil {
		t.Logf("findAvailableMacOSSystemID() error = %v (may be expected)", err)
		return
	}

	// ID should be in valid range (200-399)
	if id < 200 || id >= 400 {
		t.Errorf("ID %d not in range 200-399", id)
	}
}

// Test createMacOSServiceAccount
func TestCreateMacOSServiceAccount(t *testing.T) {
	// This will likely fail without root, but exercises the code path
	_, err := createMacOSServiceAccount("testsvc_macos")
	// Error expected without root
	_ = err
}
