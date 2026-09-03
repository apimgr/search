//go:build freebsd || openbsd || netbsd

package service

import (
	"testing"

	"github.com/apimgr/search/src/config"
)

func TestRcdTemplate(t *testing.T) {
	if rcdTemplate == "" {
		t.Fatal("rcdTemplate is empty")
	}

	required := []string{
		"#!/bin/sh",
		"# PROVIDE:",
		"# REQUIRE:",
		"search_enable",
		"/usr/local/bin/search",
	}

	for _, req := range required {
		if !contains(rcdTemplate, req) {
			t.Errorf("rcdTemplate missing required element: %q", req)
		}
	}
}

func TestServiceManagerStatusRCd(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.statusRCd()
	_ = status
	_ = err
}

func TestServiceManagerInstallRCdError(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.installRCd()
	_ = err
}

func TestServiceManagerUninstallRCd(t *testing.T) {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	err := sm.uninstallRCd()
	_ = err
}
