//go:build darwin

package service

import (
	"strings"
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestServiceManagerGetLaunchdPath(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	path := sm.getLaunchdPath()
	// Per AI.md PART 24: macOS plist uses reverse DNS format: io.github.apimgr.search
	if path != "/Library/LaunchDaemons/io.github.apimgr.search.plist" {
		t.Errorf("getLaunchdPath() = %q, want %q", path, "/Library/LaunchDaemons/io.github.apimgr.search.plist")
	}
}

func TestLaunchdTemplate(t *testing.T) {
	if launchdTemplate == "" {
		t.Fatal("launchdTemplate is empty")
	}

	required := []string{
		"<?xml",
		"<plist",
		"apimgr.search",
		"/usr/local/bin/search",
		"RunAtLoad",
		"KeepAlive",
	}

	for _, req := range required {
		if !contains(launchdTemplate, req) {
			t.Errorf("launchdTemplate missing required element: %q", req)
		}
	}
}

func TestLaunchdUserAgentTemplateUsesUserLogPaths(t *testing.T) {
	if !contains(launchdUserAgentTemplate, "{stdout_path}") || !contains(launchdUserAgentTemplate, "{stderr_path}") {
		t.Fatal("LaunchAgent template must use configured user log paths")
	}
	if contains(launchdUserAgentTemplate, "/tmp/") {
		t.Fatal("LaunchAgent template must not write logs under /tmp")
	}
	if got := escapePlistText(`/Users/ops/Library/Logs/a&b/search`); got != `/Users/ops/Library/Logs/a&amp;b/search` {
		t.Errorf("escapePlistText() = %q", got)
	}
}

func TestLaunchdUserArgs(t *testing.T) {
	path := "/Users/ops/Library/LaunchAgents/io.github.apimgr.search.plist"
	tests := []struct {
		action string
		want   []string
	}{
		{action: "start", want: []string{"load", "-w", path}},
		{action: "stop", want: []string{"unload", path}},
		{action: "enable", want: []string{"load", "-w", path}},
		{action: "disable", want: []string{"unload", "-w", path}},
	}
	for _, tt := range tests {
		got := launchdUserArgs(tt.action, path)
		if strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("launchdUserArgs(%q) = %#v, want %#v", tt.action, got, tt.want)
		}
	}
}

func TestServiceManagerStatusLaunchd(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.statusLaunchd()
	_ = status
	_ = err
}

func TestServiceManagerInstallLaunchdError(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.installLaunchd()
	_ = err
}

func TestServiceManagerUninstallLaunchd(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.uninstallLaunchd()
	_ = err
}
