//go:build darwin

package service

import (
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
