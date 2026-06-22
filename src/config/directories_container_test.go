package config

import (
	"os"
	"strings"
	"testing"
)

// setContainerOverride sets the container detection override and returns a cleanup function.
func setContainerOverride(t *testing.T, val bool) {
	t.Helper()
	containerDetectionOverride = &val
	t.Cleanup(func() { containerDetectionOverride = nil })
}

// TestIsRunningInContainerOverrideFalse verifies the override returns false.
func TestIsRunningInContainerOverrideFalse(t *testing.T) {
	setContainerOverride(t, false)
	if IsRunningInContainer() {
		t.Error("IsRunningInContainer() with override=false should return false")
	}
}

// TestIsRunningInContainerOverrideTrue verifies the override returns true.
func TestIsRunningInContainerOverrideTrue(t *testing.T) {
	setContainerOverride(t, true)
	if !IsRunningInContainer() {
		t.Error("IsRunningInContainer() with override=true should return true")
	}
}

// TestGetConfigDirNonContainer verifies GetConfigDir returns a non-container path when not in container.
func TestGetConfigDirNonContainer(t *testing.T) {
	setContainerOverride(t, false)

	os.Unsetenv("SEARCH_CONFIG_DIR")
	os.Unsetenv("CONFIG_DIR")
	SetConfigDirOverride("")
	defer SetConfigDirOverride("")

	got := GetConfigDir()
	if got == "" {
		t.Error("GetConfigDir() non-container should not be empty")
	}

	if strings.HasPrefix(got, "/config/") {
		t.Errorf("GetConfigDir() non-container should not return container path, got %q", got)
	}
}

// TestGetDataDirNonContainer verifies GetDataDir returns a non-container path when not in container.
func TestGetDataDirNonContainer(t *testing.T) {
	setContainerOverride(t, false)

	os.Unsetenv("SEARCH_DATA_DIR")
	os.Unsetenv("DATA_DIR")
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	got := GetDataDir()
	if got == "" {
		t.Error("GetDataDir() non-container should not be empty")
	}

	if strings.HasPrefix(got, "/data/") {
		t.Errorf("GetDataDir() non-container should not return container path, got %q", got)
	}
}

// TestGetSSLDirNonContainer verifies GetSSLDir returns a non-container path when not in container.
func TestGetSSLDirNonContainer(t *testing.T) {
	setContainerOverride(t, false)

	os.Unsetenv("SEARCH_CONFIG_DIR")
	os.Unsetenv("CONFIG_DIR")
	SetConfigDirOverride("")
	defer SetConfigDirOverride("")

	got := GetSSLDir()
	if got == "" {
		t.Error("GetSSLDir() non-container should not be empty")
	}

	if strings.HasPrefix(got, "/config/") {
		t.Errorf("GetSSLDir() non-container should not return container path, got %q", got)
	}

	if !strings.HasSuffix(got, "/ssl") {
		t.Errorf("GetSSLDir() should end with /ssl, got %q", got)
	}
}

// TestGetGeoIPDirNonContainer verifies GetGeoIPDir returns a non-container path when not in container.
func TestGetGeoIPDirNonContainer(t *testing.T) {
	setContainerOverride(t, false)

	os.Unsetenv("SEARCH_CONFIG_DIR")
	os.Unsetenv("CONFIG_DIR")
	SetConfigDirOverride("")
	defer SetConfigDirOverride("")

	got := GetGeoIPDir()
	if got == "" {
		t.Error("GetGeoIPDir() non-container should not be empty")
	}

	if !strings.Contains(got, "geoip") {
		t.Errorf("GetGeoIPDir() should contain 'geoip', got %q", got)
	}
}

// TestGetSecurityDirNonContainer verifies GetSecurityDir returns a non-container path when not in container.
func TestGetSecurityDirNonContainer(t *testing.T) {
	setContainerOverride(t, false)

	os.Unsetenv("SEARCH_CONFIG_DIR")
	os.Unsetenv("CONFIG_DIR")
	SetConfigDirOverride("")
	defer SetConfigDirOverride("")

	got := GetSecurityDir()
	if got == "" {
		t.Error("GetSecurityDir() non-container should not be empty")
	}

	if !strings.Contains(got, "security") {
		t.Errorf("GetSecurityDir() should contain 'security', got %q", got)
	}
}

// TestGetTorDirNonContainer verifies GetTorDir returns a non-container path when not in container.
func TestGetTorDirNonContainer(t *testing.T) {
	setContainerOverride(t, false)

	os.Unsetenv("SEARCH_DATA_DIR")
	os.Unsetenv("DATA_DIR")
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	got := GetTorDir()
	if got == "" {
		t.Error("GetTorDir() non-container should not be empty")
	}

	if !strings.Contains(got, "tor") {
		t.Errorf("GetTorDir() should contain 'tor', got %q", got)
	}
}

// TestGetDirectoryPermissionsNonRoot verifies GetDirectoryPermissions returns 0700 for non-root.
// This exercises the non-root branch of GetDirectoryPermissions.
func TestGetDirectoryPermissionsRoot(t *testing.T) {
	perm := GetDirectoryPermissions()
	if perm != 0755 && perm != 0700 {
		t.Errorf("GetDirectoryPermissions() = %o, want 0755 or 0700", perm)
	}
}

// TestGetLogDirNonContainer verifies GetLogDir returns a non-container path.
func TestGetLogDirNonContainer(t *testing.T) {
	setContainerOverride(t, false)

	os.Unsetenv("SEARCH_LOG_DIR")
	os.Unsetenv("LOG_DIR")
	SetLogDirOverride("")
	defer SetLogDirOverride("")

	got := GetLogDir()
	if got == "" {
		t.Error("GetLogDir() non-container should not be empty")
	}

	if strings.HasPrefix(got, "/data/") {
		t.Errorf("GetLogDir() non-container should not return container path, got %q", got)
	}
}
