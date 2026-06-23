package cmd

// cmd_extra_test.go adds targeted coverage for versionLessThan, applyColorMode,
// backgroundAutodiscover (CLIVersions/CLIMinVersion paths), and HandleShellFlag
// (the return-true path that doesn't call os.Exit).
//
// Coverage targets:
//   - versionLessThan: table-driven — equal, a<b, a>b, v-prefix, missing segments
//   - applyColorMode: config-file path, NO_COLOR env path, TTY auto-detect paths
//   - backgroundAutodiscover: CLIVersions newer version available (stderr + return)
//   - HandleShellFlag: --shell with unknown sub-command (return true, no exit)

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/apimgr/search/src/client/clicfg"

	"github.com/apimgr/search/src/client/api"
)

// ---- versionLessThan ----

// TestVersionLessThan covers all comparison branches.
func TestVersionLessThan(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"equal versions", "1.0.0", "1.0.0", false},
		{"a less than b (major)", "1.0.0", "2.0.0", true},
		{"a greater than b (major)", "2.0.0", "1.0.0", false},
		{"a less than b (minor)", "1.0.0", "1.1.0", true},
		{"a greater than b (minor)", "1.1.0", "1.0.0", false},
		{"a less than b (patch)", "1.0.0", "1.0.1", true},
		{"a greater than b (patch)", "1.0.1", "1.0.0", false},
		{"v-prefix stripped", "v1.2.3", "v1.2.4", true},
		{"only major version", "1", "2", true},
		{"only major version equal", "1", "1", false},
		{"two segments", "1.5", "1.6", true},
		{"two segments equal", "1.5", "1.5", false},
		{"major greater minor less", "2.0.0", "1.9.9", false},
		{"zero versions equal", "0.0.0", "0.0.0", false},
		{"zero vs one", "0.0.0", "0.0.1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := versionLessThan(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("versionLessThan(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// ---- applyColorMode ----

// TestApplyColorModeNOCOLOR covers path 3: NO_COLOR env var → resolved = "no".
func TestApplyColorModeNOCOLOR(t *testing.T) {
	orig := colorMode
	defer func() {
		colorMode = orig
		os.Unsetenv("NO_COLOR")
		os.Unsetenv("SEARCH_COLOR")
	}()

	colorMode = "auto"
	os.Setenv("NO_COLOR", "1")
	clicfg.Reset()

	applyColorMode()

	got := os.Getenv("SEARCH_COLOR")
	if got != "no" {
		t.Errorf("applyColorMode() SEARCH_COLOR = %q, want 'no' (NO_COLOR set)", got)
	}
}

// TestApplyColorModeExplicitYes covers path 1: explicit --color yes flag.
func TestApplyColorModeExplicitYes(t *testing.T) {
	orig := colorMode
	defer func() {
		colorMode = orig
		os.Unsetenv("SEARCH_COLOR")
	}()

	colorMode = "yes"
	clicfg.Reset()

	applyColorMode()

	got := os.Getenv("SEARCH_COLOR")
	if got != "yes" {
		t.Errorf("applyColorMode() SEARCH_COLOR = %q, want 'yes' (explicit yes)", got)
	}
}

// TestApplyColorModeExplicitNo covers path 1: explicit --color no flag.
func TestApplyColorModeExplicitNo(t *testing.T) {
	orig := colorMode
	defer func() {
		colorMode = orig
		os.Unsetenv("SEARCH_COLOR")
	}()

	colorMode = "no"
	clicfg.Reset()

	applyColorMode()

	got := os.Getenv("SEARCH_COLOR")
	if got != "no" {
		t.Errorf("applyColorMode() SEARCH_COLOR = %q, want 'no' (explicit no)", got)
	}
}

// TestApplyColorModeFromConfig covers path 2: config file sets output.color.
func TestApplyColorModeFromConfig(t *testing.T) {
	orig := colorMode
	defer func() {
		colorMode = orig
		os.Unsetenv("NO_COLOR")
		os.Unsetenv("SEARCH_COLOR")
		clicfg.Reset()
	}()

	colorMode = "auto"
	os.Unsetenv("NO_COLOR")
	clicfg.Reset()
	clicfg.Set("output.color", "no")

	applyColorMode()

	got := os.Getenv("SEARCH_COLOR")
	if got != "no" {
		t.Errorf("applyColorMode() SEARCH_COLOR = %q, want 'no' (from config)", got)
	}
}

// TestApplyColorModeTTYAutoDetect covers path 4: TTY auto-detect when TERM=dumb.
func TestApplyColorModeTTYAutoDetect(t *testing.T) {
	orig := colorMode
	origTERM := os.Getenv("TERM")
	defer func() {
		colorMode = orig
		os.Setenv("TERM", origTERM)
		os.Unsetenv("NO_COLOR")
		os.Unsetenv("SEARCH_COLOR")
		clicfg.Reset()
	}()

	colorMode = "auto"
	os.Unsetenv("NO_COLOR")
	os.Setenv("TERM", "dumb")
	clicfg.Reset()

	applyColorMode()

	got := os.Getenv("SEARCH_COLOR")
	if got != "no" {
		t.Errorf("applyColorMode() SEARCH_COLOR = %q, want 'no' (TERM=dumb)", got)
	}
}

// ---- backgroundAutodiscover extra paths ----

// TestBackgroundAutodiscoverCLIVersionsAvailable covers the path where CLIVersions
// contains a newer version — should write to stderr but not exit.
func TestBackgroundAutodiscoverCLIVersionsAvailable(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return an autodiscover response with a newer CLI version.
		resp := api.AutodiscoverResponse{}
		resp.CLIVersions = map[string]api.CLIBinaryInfo{
			api.CurrentPlatform(): {Version: "999.0.0"},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":   true,
			"data": resp,
		})
	}))
	defer testServer.Close()

	savedVersion := Version
	defer func() { Version = savedVersion }()

	// Set Version to a real value (not "dev") so the version check runs.
	Version = "0.0.1"

	clicfg.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)
	defer func() { apiClient = nil }()

	// Should print to stderr about newer version but NOT call os.Exit.
	backgroundAutodiscover()
}

// TestBackgroundAutodiscoverDevVersionSkipsCheck covers the path where
// Version == "dev" — the CLIVersions check is skipped entirely.
func TestBackgroundAutodiscoverDevVersionSkipsCheck(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.AutodiscoverResponse{}
		resp.CLIVersions = map[string]api.CLIBinaryInfo{
			api.CurrentPlatform(): {Version: "999.0.0"},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":   true,
			"data": resp,
		})
	}))
	defer testServer.Close()

	savedVersion := Version
	defer func() { Version = savedVersion }()

	// Version == "dev" should skip the min-version and update checks.
	Version = "dev"

	clicfg.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)
	defer func() { apiClient = nil }()

	// Should return immediately after the CLIVersions check is skipped.
	backgroundAutodiscover()
}

// TestBackgroundAutodiscoverNoCLIVersionsForPlatform covers the case where
// CLIVersions is populated but the current platform is not in the map.
func TestBackgroundAutodiscoverNoCLIVersionsForPlatform(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := api.AutodiscoverResponse{}
		// Use a platform key that will never match.
		resp.CLIVersions = map[string]api.CLIBinaryInfo{
			"nonexistent-platform-xyz": {Version: "999.0.0"},
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":   true,
			"data": resp,
		})
	}))
	defer testServer.Close()

	savedVersion := Version
	defer func() { Version = savedVersion }()

	Version = "1.0.0"

	clicfg.Reset()
	apiClient = api.NewClient(testServer.URL, "", 30)
	defer func() { apiClient = nil }()

	backgroundAutodiscover()
}

// ---- HandleShellFlag: return-true path without os.Exit ----

// TestHandleShellFlagReturnsTrueForUnknownSubcmd covers the path where --shell
// is present but the subcommand doesn't match any case — the loop returns true
// without calling os.Exit.
func TestHandleShellFlagReturnsTrueForUnknownSubcmd(t *testing.T) {
	// "--shell unknownsubcmd" → falls through switch without matching, returns true.
	result := HandleShellFlag([]string{"search", "--shell", "unknownsubcmd"})
	if !result {
		t.Error("HandleShellFlag(['search', '--shell', 'unknownsubcmd']) should return true")
	}
}

// TestHandleShellFlagReturnsTrueWithShellArg covers the path where --shell is
// followed by a subcommand and a shell arg (3 args), and subcommand is unknown.
func TestHandleShellFlagReturnsTrueWithShellArg(t *testing.T) {
	// "--shell unknownsubcmd bash" → shell=bash, subCmd=unknownsubcmd, returns true.
	result := HandleShellFlag([]string{"search", "--shell", "unknownsubcmd", "bash"})
	if !result {
		t.Error("HandleShellFlag with 3 args and unknown subCmd should return true")
	}
}

// TestHandleShellFlagShellAutoDetect covers the path where --shell is present,
// subCmd is unknown, and no explicit shell arg → shell = detectShell().
func TestHandleShellFlagAutoDetectShell(t *testing.T) {
	os.Setenv("SHELL", "/bin/bash")
	defer os.Unsetenv("SHELL")

	// "--shell unknownsubcmd" with next arg being a flag → shell=detectShell()
	result := HandleShellFlag([]string{"search", "--shell", "unknownsubcmd", "--some-flag"})
	if !result {
		t.Error("HandleShellFlag with --shell flag arg and unknown subCmd should return true")
	}
}
