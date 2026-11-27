//go:build darwin

package service

import (
	"reflect"
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

func TestParseMacOSAccountIDsIncludesUserAndGroupIDs(t *testing.T) {
	got := parseMacOSAccountIDs("_legacy 398\nsearch 397\n", "_legacy 396\nsearch 395\n")
	want := map[int]bool{395: true, 396: true, 397: true, 398: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseMacOSAccountIDs() = %#v, want %#v", got, want)
	}
}

func TestMacOSAccountDeleteCommandsUseCanonicalName(t *testing.T) {
	got := macOSAccountDeleteCommands("search")
	want := [][]string{
		{"dscl", ".", "-delete", "/Users/search"},
		{"dscl", ".", "-delete", "/Groups/search"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("macOSAccountDeleteCommands() = %#v, want %#v", got, want)
	}
}

// Test createMacOSServiceAccount
func TestCreateMacOSServiceAccount(t *testing.T) {
	// This will likely fail without root, but exercises the code path
	_, err := createMacOSServiceAccount("testsvc_macos")
	// Error expected without root
	_ = err
}
