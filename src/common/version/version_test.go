package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()
	if info.Version == "" {
		t.Error("Version should not be empty")
	}
	if info.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", info.OS, runtime.GOOS)
	}
	if info.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", info.Arch, runtime.GOARCH)
	}
	if info.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
}

func TestInfoString(t *testing.T) {
	info := Get()
	s := info.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
	if !strings.Contains(s, info.Version) {
		t.Error("String() should contain version")
	}
}

func TestInfoFull(t *testing.T) {
	info := Get()
	full := info.Full()
	if full == "" {
		t.Error("Full() should not be empty")
	}
	if !strings.Contains(full, "Version:") {
		t.Error("Full() should contain 'Version:'")
	}
	if !strings.Contains(full, "Commit:") {
		t.Error("Full() should contain 'Commit:'")
	}
	if !strings.Contains(full, "Build Date:") {
		t.Error("Full() should contain 'Build Date:'")
	}
}

func TestInfoShort(t *testing.T) {
	info := Get()
	s := info.Short()
	if s != info.Version {
		t.Errorf("Short() = %q, want %q", s, info.Version)
	}
}

func TestInfoUserAgent(t *testing.T) {
	info := Get()
	ua := info.UserAgent("search")
	if !strings.HasPrefix(ua, "search/") {
		t.Errorf("UserAgent() = %q, should start with 'search/'", ua)
	}
}

func TestGetShort(t *testing.T) {
	s := GetShort()
	if s != Version {
		t.Errorf("GetShort() = %q, want %q", s, Version)
	}
}

func TestGetCommitShort(t *testing.T) {
	s := GetCommitShort()
	if len(Commit) >= 7 && len(s) != 7 {
		t.Errorf("GetCommitShort() = %q, should be 7 chars when commit is long enough", s)
	}
}

func TestIsDev(t *testing.T) {
	// Save original
	orig := Version
	defer func() { Version = orig }()

	tests := []struct {
		version string
		want    bool
	}{
		{"dev", true},
		{"", true},
		{"1.0.0-dev", true},
		{"1.0.0", false},
		{"v1.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			Version = tt.version
			got := IsDev()
			if got != tt.want {
				t.Errorf("IsDev() with version %q = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
