//go:build windows

package config

import (
	"path/filepath"
	"testing"
)

func TestWindowsDirectoryPathsResolveKnownFolders(t *testing.T) {
	for _, name := range []string{"SEARCH_CONFIG_DIR", "CONFIG_DIR", "SEARCH_DATA_DIR", "DATA_DIR", "SEARCH_LOG_DIR", "LOG_DIR", "SEARCH_CACHE_DIR", "SEARCH_DATABASE_DIR", "DATABASE_DIR"} {
		t.Setenv(name, "")
	}
	t.Setenv("ProgramData", `C:\ProgramData`)
	t.Setenv("ProgramFiles", `C:\Program Files`)
	t.Setenv("AppData", `C:\Users\tester\AppData\Roaming`)
	t.Setenv("LocalAppData", `C:\Users\tester\AppData\Local`)

	wasElevated := startedElevated
	startedElevated = IsPrivileged()
	defer func() { startedElevated = wasElevated }()

	cliOverrideMu.Lock()
	oldOverrides := cliOverrides
	cliOverrides = make(map[string]string)
	cliOverrideMu.Unlock()
	defer func() {
		cliOverrideMu.Lock()
		cliOverrides = oldOverrides
		cliOverrideMu.Unlock()
	}()

	oldContainerOverride := containerDetectionOverride
	notContainer := false
	containerDetectionOverride = &notContainer
	defer func() { containerDetectionOverride = oldContainerOverride }()

	root := IsPrivileged()
	base := filepath.Join(`C:\ProgramData`, ProjectOrg, ProjectName)
	userBase := filepath.Join(`C:\Users\tester\AppData\Local`, ProjectOrg, ProjectName)
	wantConfig := filepath.Join(`C:\Users\tester\AppData\Roaming`, ProjectOrg, ProjectName)
	wantData := userBase
	wantLogs := filepath.Join(userBase, "logs")
	wantCache := filepath.Join(userBase, "cache")
	wantDatabase := filepath.Join(userBase, "db")
	wantBinary := filepath.Join(userBase, ProjectName+".exe")
	if root {
		wantConfig = base
		wantData = filepath.Join(base, "data")
		wantLogs = filepath.Join(base, "logs")
		wantCache = filepath.Join(base, "cache")
		wantDatabase = filepath.Join(base, "db")
		wantBinary = filepath.Join(`C:\Program Files`, ProjectOrg, ProjectName, ProjectName+".exe")
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"config", GetConfigDir(), wantConfig},
		{"data", GetDataDir(), wantData},
		{"logs", GetLogDir(), wantLogs},
		{"cache", GetCacheDir(), wantCache},
		{"database", GetDatabaseDir(), wantDatabase},
		{"binary", GetBinaryPath(), wantBinary},
	}
	for _, check := range checks {
		if check.got != check.want {
			t.Errorf("%s directory = %q, want %q", check.name, check.got, check.want)
		}
		if filepath.IsAbs(check.got) && filepath.VolumeName(check.got) == "" {
			t.Errorf("%s directory = %q, missing expanded Windows volume", check.name, check.got)
		}
	}
}
