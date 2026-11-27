package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
)

func TestExplicitHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "long help", args: []string{"--help"}, want: true},
		{name: "short help", args: []string{"-h"}, want: true},
		{name: "no help", args: []string{"--status"}},
		{name: "helpful is a different flag", args: []string{"--helpful"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := explicitHelp(tt.args); got != tt.want {
				t.Fatalf("explicitHelp(%q) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestMissingLegacyCommandUsesUsageExit(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want string
	}{
		{name: "service", arg: "--service", want: "service action is required"},
		{name: "maintenance", arg: "--maintenance", want: "maintenance action is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldArgs, oldStderr, oldExit := os.Args, os.Stderr, exitFunc
			readPipe, writePipe, err := os.Pipe()
			if err != nil {
				t.Fatalf("create stderr pipe: %v", err)
			}
			os.Args = []string{"search", tt.arg}
			os.Stderr = writePipe
			var exitCode int
			exitFunc = func(code int) { exitCode = code }
			t.Cleanup(func() {
				os.Args, os.Stderr, exitFunc = oldArgs, oldStderr, oldExit
				if err := writePipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
					t.Errorf("close stderr writer: %v", err)
				}
				if err := readPipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
					t.Errorf("close stderr reader: %v", err)
				}
			})

			handleLegacyArgs()
			if err := writePipe.Close(); err != nil {
				t.Fatalf("close stderr writer: %v", err)
			}
			stderr, err := io.ReadAll(readPipe)
			if err != nil {
				t.Fatalf("read stderr: %v", err)
			}
			if exitCode != 2 {
				t.Fatalf("exit code = %d, want 2", exitCode)
			}
			if !strings.Contains(string(stderr), tt.want) || !strings.Contains(string(stderr), "Usage:") {
				t.Fatalf("stderr = %q, want localized error and usage hint", stderr)
			}
		})
	}
}

func TestUnknownServiceActionIsUsageError(t *testing.T) {
	oldArgs, oldStderr, oldExit := os.Args, os.Stderr, exitFunc
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Args = []string{"renamed-search"}
	os.Stderr = writePipe
	var exitCode int
	exitFunc = func(code int) { exitCode = code }
	t.Cleanup(func() {
		os.Args, os.Stderr, exitFunc = oldArgs, oldStderr, oldExit
		if err := writePipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close stderr writer: %v", err)
		}
		if err := readPipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close stderr reader: %v", err)
		}
	})

	stdout := captureStdout(t, func() { runService("invalid") })
	if err := writePipe.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	stderr, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("unknown service action exit code = %d, want 2", exitCode)
	}
	if stdout != "" {
		t.Fatalf("unknown service action wrote to stdout: %q", stdout)
	}
	if !strings.Contains(string(stderr), "Unknown service action") || !strings.Contains(string(stderr), "Usage: renamed-search --service --help") {
		t.Fatalf("stderr = %q, want error and usage hint", stderr)
	}
}

func TestMaintenanceBackupConfigRejectsInitializationFailure(t *testing.T) {
	wantErr := errors.New("invalid server.yml")
	cfg, err := maintenanceBackupConfig(func() (*config.Config, error) {
		return nil, wantErr
	})
	if cfg != nil {
		t.Fatal("maintenanceBackupConfig() returned config after initialization failure")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("maintenanceBackupConfig() error = %v, want wrapped %v", err, wantErr)
	}
}

func TestMaintenanceBackupConfigRejectsNilConfiguration(t *testing.T) {
	cfg, err := maintenanceBackupConfig(func() (*config.Config, error) { return nil, nil })
	if cfg != nil || err == nil {
		t.Fatalf("maintenanceBackupConfig() = (%v, %v), want (nil, error)", cfg, err)
	}
}

func TestParseMaintenanceBackupOptions(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    maintenanceBackupOptions
		wantErr bool
	}{
		{name: "defaults", args: nil, want: maintenanceBackupOptions{}},
		{name: "optional contents", args: []string{"snapshot", "--include-ssl", "--include-data"}, want: maintenanceBackupOptions{Filename: "snapshot", IncludeSSL: true, IncludeData: true}},
		{name: "unknown option", args: []string{"--other"}, wantErr: true},
		{name: "multiple filenames", args: []string{"first", "second"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMaintenanceBackupOptions(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseMaintenanceBackupOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("parseMaintenanceBackupOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestRunServerStartsBodyOnRegularProcess ensures non-service invocations
// enter the long-running server path instead of returning successfully.
func TestRunServerStartsBodyOnRegularProcess(t *testing.T) {
	original := startServerBody
	called := false
	startServerBody = func() { called = true }
	t.Cleanup(func() { startServerBody = original })

	runServer()
	if !called {
		t.Fatal("runServer() did not start the server body")
	}
}

func TestServerBannerDebugStateUsesConfigMode(t *testing.T) {
	debugValue, debugSet := os.LookupEnv("DEBUG")
	if err := os.Unsetenv("DEBUG"); err != nil {
		t.Fatalf("unset DEBUG: %v", err)
	}
	t.Cleanup(func() {
		if debugSet {
			if err := os.Setenv("DEBUG", debugValue); err != nil {
				t.Errorf("restore DEBUG: %v", err)
			}
			return
		}
		if err := os.Unsetenv("DEBUG"); err != nil {
			t.Errorf("restore unset DEBUG: %v", err)
		}
	})

	cfg := config.DefaultConfig()
	cfg.Server.Mode = "debug"
	if !serverBannerDebugEnabled(cfg) {
		t.Fatal("server banner did not enable debug state from server.mode=debug")
	}
}

// ============================================================
// formatUptime
// ============================================================

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "days hours minutes",
			d:    (2*24+5)*time.Hour + 30*time.Minute,
			want: "2d 5h 30m",
		},
		{
			name: "days no extra minutes",
			d:    3 * 24 * time.Hour,
			want: "3d 0h 0m",
		},
		{
			name: "hours and minutes no days",
			d:    4*time.Hour + 15*time.Minute,
			want: "4h 15m",
		},
		{
			name: "hours only",
			d:    2 * time.Hour,
			want: "2h 0m",
		},
		{
			name: "minutes only",
			d:    45 * time.Minute,
			want: "45m",
		},
		{
			name: "zero duration",
			d:    0,
			want: "0m",
		},
		{
			name: "one minute",
			d:    time.Minute,
			want: "1m",
		},
		{
			name: "exactly one day",
			d:    24 * time.Hour,
			want: "1d 0h 0m",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatUptime(tt.d)
			if got != tt.want {
				t.Errorf("formatUptime(%v) = %q, want %q", tt.d, got, tt.want)
			}
		})
	}
}

// ============================================================
// filterDaemonFlag
// ============================================================

func TestFilterDaemonFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "empty slice",
			args: []string{},
			want: []string{},
		},
		{
			name: "only daemon flag",
			args: []string{"--daemon"},
			want: []string{},
		},
		{
			name: "daemon flag removed from start",
			args: []string{"--daemon", "--port", "8080"},
			want: []string{"--port", "8080"},
		},
		{
			name: "daemon flag removed from middle",
			args: []string{"--mode", "production", "--daemon", "--port", "8080"},
			want: []string{"--mode", "production", "--port", "8080"},
		},
		{
			name: "daemon flag removed from end",
			args: []string{"--port", "8080", "--daemon"},
			want: []string{"--port", "8080"},
		},
		{
			name: "multiple daemon flags removed",
			args: []string{"--daemon", "--port", "8080", "--daemon"},
			want: []string{"--port", "8080"},
		},
		{
			name: "no daemon flag leaves args untouched",
			args: []string{"--port", "8080", "--mode", "production"},
			want: []string{"--port", "8080", "--mode", "production"},
		},
		{
			name: "partial match not removed",
			args: []string{"--daemonize", "--port", "9090"},
			want: []string{"--daemonize", "--port", "9090"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterDaemonFlag(tt.args)
			if len(got) != len(tt.want) {
				t.Errorf("filterDaemonFlag(%v) = %v, want %v", tt.args, got, tt.want)
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("filterDaemonFlag(%v)[%d] = %q, want %q", tt.args, i, v, tt.want[i])
				}
			}
		})
	}
}

// ============================================================
// formatBytes
// ============================================================

// ============================================================
// detectShell
// ============================================================

func TestDetectShell(t *testing.T) {
	// Save original SHELL env var and restore it after all sub-tests
	origShell, origHad := os.LookupEnv("SHELL")
	t.Cleanup(func() {
		if origHad {
			os.Setenv("SHELL", origShell)
		} else {
			os.Unsetenv("SHELL")
		}
	})

	tests := []struct {
		name       string
		shellEnv   string
		unsetShell bool
		wantFn     func(got string) bool
		wantDescr  string
	}{
		{
			name:      "bash from full path",
			shellEnv:  "/bin/bash",
			wantFn:    func(got string) bool { return got == "bash" },
			wantDescr: "bash",
		},
		{
			name:      "zsh from full path",
			shellEnv:  "/usr/bin/zsh",
			wantFn:    func(got string) bool { return got == "zsh" },
			wantDescr: "zsh",
		},
		{
			name:      "fish from full path",
			shellEnv:  "/usr/bin/fish",
			wantFn:    func(got string) bool { return got == "fish" },
			wantDescr: "fish",
		},
		{
			name:      "POSIX shell from path",
			shellEnv:  "/bin/dash",
			wantFn:    func(got string) bool { return got == "dash" },
			wantDescr: "dash",
		},
		{
			name:      "Korn shell from path",
			shellEnv:  "/bin/ksh",
			wantFn:    func(got string) bool { return got == "ksh" },
			wantDescr: "ksh",
		},
		{
			name:      "powershell bare name",
			shellEnv:  "powershell",
			wantFn:    func(got string) bool { return got == "powershell" },
			wantDescr: "powershell",
		},
		{
			name:      "pwsh bare name",
			shellEnv:  "pwsh",
			wantFn:    func(got string) bool { return got == "pwsh" },
			wantDescr: "pwsh",
		},
		{
			name:      "unknown shell remains available for unsupported-shell error",
			shellEnv:  "/bin/tcsh",
			wantFn:    func(got string) bool { return got == "tcsh" },
			wantDescr: "tcsh",
		},
		{
			name:       "empty SHELL falls back to platform default",
			unsetShell: true,
			wantFn: func(got string) bool {
				if runtime.GOOS == "windows" {
					return got == "powershell"
				}
				return got == "bash"
			},
			wantDescr: "bash (linux/darwin) or powershell (windows)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unsetShell {
				os.Unsetenv("SHELL")
			} else {
				os.Setenv("SHELL", tt.shellEnv)
			}
			got := detectShell()
			if !tt.wantFn(got) {
				t.Errorf("detectShell() = %q, want %s", got, tt.wantDescr)
			}
		})
	}
}

// ============================================================
// generateSetupToken
// ============================================================

func TestGenerateSetupToken(t *testing.T) {
	t.Run("returns 32-char lowercase hex string", func(t *testing.T) {
		tok, err := generateSetupToken()
		if err != nil {
			t.Fatalf("generateSetupToken() error = %v", err)
		}
		if len(tok) != 32 {
			t.Errorf("generateSetupToken() len = %d, want 32", len(tok))
		}
		for _, ch := range tok {
			if !strings.ContainsRune("0123456789abcdef", ch) {
				t.Errorf("generateSetupToken() contains non-hex char %q in %q", ch, tok)
				break
			}
		}
	})

	t.Run("never returns empty string", func(t *testing.T) {
		tok, err := generateSetupToken()
		if err != nil {
			t.Fatalf("generateSetupToken() error = %v", err)
		}
		if tok == "" {
			t.Error("generateSetupToken() returned empty string")
		}
	})

	t.Run("two calls produce different tokens", func(t *testing.T) {
		tok1, err := generateSetupToken()
		if err != nil {
			t.Fatalf("generateSetupToken() first call error = %v", err)
		}
		tok2, err := generateSetupToken()
		if err != nil {
			t.Fatalf("generateSetupToken() second call error = %v", err)
		}
		if tok1 == tok2 {
			t.Errorf("generateSetupToken() returned identical tokens on consecutive calls: %q", tok1)
		}
	})
}

// ============================================================
// isProcessRunning
// ============================================================

func TestIsProcessRunning(t *testing.T) {
	t.Run("current process is running", func(t *testing.T) {
		pid := os.Getpid()
		if !isProcessRunning(pid) {
			t.Errorf("isProcessRunning(%d) = false, want true for current process", pid)
		}
	})

	t.Run("nonexistent PID returns false", func(t *testing.T) {
		// PID 99999999 is astronomically unlikely to exist on any real system
		if isProcessRunning(99999999) {
			t.Error("isProcessRunning(99999999) = true, want false for nonexistent PID")
		}
	})

	t.Run("PID 0 does not panic", func(t *testing.T) {
		// We only verify no panic — the actual true/false is OS-defined
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("isProcessRunning(0) panicked: %v", r)
			}
		}()
		isProcessRunning(0)
	})
}

// ============================================================
// applyCliOverrides
// ============================================================

// saveEnvKeys captures the current values of a list of env-var names and
// returns a restore function that resets them all to their original state.
func saveEnvKeys(keys ...string) func() {
	saved := make(map[string]string, len(keys))
	unset := make(map[string]bool, len(keys))
	for _, k := range keys {
		v, ok := os.LookupEnv(k)
		if ok {
			saved[k] = v
		} else {
			unset[k] = true
		}
	}
	return func() {
		for _, k := range keys {
			if unset[k] {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, saved[k])
			}
		}
	}
}

// resetFlagVars zeroes all CLI flag variables to their default (zero) values
// so each sub-test starts from a clean state.
func resetFlagVars() {
	flagMode = ""
	flagData = ""
	flagConfig = ""
	flagCache = ""
	flagLog = ""
	flagBackup = ""
	flagPID = ""
	flagAddress = ""
	flagPort = ""
	flagBaseURL = ""
	flagColor = ""
	flagLang = ""
	flagBackupIncludeSSL = false
	flagBackupIncludeData = false
	flagDebug = false
	flagI2P = false
}

func TestApplyCliOverrides(t *testing.T) {
	t.Setenv("MODE", "")
	t.Setenv("DEBUG", "")
	t.Setenv("PORT", "")
	t.Setenv("LISTEN", "")
	t.Setenv("LANG", "")
	t.Run("mode uses canonical environment variable", func(t *testing.T) {
		defer saveEnvKeys("MODE")()
		resetFlagVars()
		flagMode = "development"
		applyCliOverrides()
		if got := os.Getenv("MODE"); got != "development" {
			t.Errorf("MODE = %q, want development", got)
		}
	})
	t.Run("debug uses canonical environment variable", func(t *testing.T) {
		defer saveEnvKeys("DEBUG")()
		resetFlagVars()
		flagDebug = true
		applyCliOverrides()
		if got := os.Getenv("DEBUG"); got != "true" {
			t.Errorf("DEBUG = %q, want true", got)
		}
	})
	t.Run("port uses canonical environment variable", func(t *testing.T) {
		defer saveEnvKeys("PORT")()
		resetFlagVars()
		flagPort = "8080"
		applyCliOverrides()
		if got := os.Getenv("PORT"); got != "8080" {
			t.Errorf("PORT = %q, want 8080", got)
		}
	})
	t.Run("address uses canonical environment variable", func(t *testing.T) {
		defer saveEnvKeys("LISTEN")()
		resetFlagVars()
		flagAddress = "127.0.0.1"
		applyCliOverrides()
		if got := os.Getenv("LISTEN"); got != "127.0.0.1" {
			t.Errorf("LISTEN = %q, want 127.0.0.1", got)
		}
	})
	t.Run("directory flags set config overrides", func(t *testing.T) {
		resetFlagVars()
		dir := t.TempDir()
		flagData = dir
		applyCliOverrides()
		if got := config.GetDataDir(); got != dir {
			t.Errorf("GetDataDir() = %q, want %q", got, dir)
		}
		config.SetDataDirOverride("")
	})
	t.Run("base URL flag sets config override", func(t *testing.T) {
		resetFlagVars()
		flagBaseURL = "/app"
		applyCliOverrides()
		if got := config.GetBaseURL(); got != "/app" {
			t.Errorf("GetBaseURL() = %q, want /app", got)
		}
		config.SetBaseURLOverride("")
	})
	t.Run("color flag sets config override", func(t *testing.T) {
		resetFlagVars()
		flagColor = "never"
		applyCliOverrides()
		if got := config.GetColorMode(); got != "never" {
			t.Errorf("GetColorMode() = %q, want never", got)
		}
		config.SetColorMode("")
	})
	t.Run("language flag sets locale", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_LANG", "LANG")()
		resetFlagVars()
		flagLang = "es"
		applyCliOverrides()
		if got := os.Getenv("SEARCH_LANG"); got != "es" {
			t.Errorf("SEARCH_LANG = %q, want es", got)
		}
		if got := os.Getenv("LANG"); got != "es" {
			t.Errorf("LANG = %q, want es", got)
		}
	})
}

func TestCommandHelpPreScan(t *testing.T) {
	for _, command := range []string{"shell", "service", "maintenance", "update"} {
		for _, help := range []string{"--help", "-h", "help"} {
			got, ok := commandHelp([]string{"--" + command, help}, "shell", "service", "maintenance", "update")
			if !ok || got != command {
				t.Errorf("commandHelp(--%s %s) = (%q, %v), want (%q, true)", command, help, got, ok, command)
			}
		}
	}
	if _, ok := commandHelp([]string{"--service", "restart"}, "service"); ok {
		t.Fatal("commandHelp(--service restart) unexpectedly matched help")
	}
}

func TestEarlyPortConfigAcceptsDualPortFlag(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("LISTEN", "")
	originalPort, originalAddress := flagPort, flagAddress
	flagPort, flagAddress = "8080,8443", ""
	t.Cleanup(func() { flagPort, flagAddress = originalPort, originalAddress })

	cfg, err := earlyPortConfig()
	if err != nil {
		t.Fatalf("earlyPortConfig() error = %v", err)
	}
	if cfg.Server.Port != 8080 || cfg.Server.HTTPSPort != 8443 {
		t.Fatalf("earlyPortConfig() ports = (%d, %d), want (8080, 8443)", cfg.Server.Port, cfg.Server.HTTPSPort)
	}
}

func TestServerFlagsIncludeI2P(t *testing.T) {
	if flag.Lookup("i2p") == nil {
		t.Fatal("--i2p flag is not registered")
	}
}

func TestI2PFlagOverridePrecedence(t *testing.T) {
	t.Setenv("I2P_ENABLED", "configured")
	if err := applyI2PFlagOverride(false, false); err != nil {
		t.Fatalf("unset flag override: %v", err)
	}
	if got := os.Getenv("I2P_ENABLED"); got != "configured" {
		t.Fatalf("unset --i2p changed I2P_ENABLED to %q", got)
	}
	if err := applyI2PFlagOverride(true, false); err != nil {
		t.Fatalf("explicit false flag override: %v", err)
	}
	if got := os.Getenv("I2P_ENABLED"); got != "false" {
		t.Fatalf("explicit --i2p=false set I2P_ENABLED to %q", got)
	}
}

func TestCLIPathOverrideValidation(t *testing.T) {
	originalData, originalPID := flagData, flagPID
	t.Cleanup(func() { flagData, flagPID = originalData, originalPID })
	flagData, flagPID = "/var/lib/search", ""
	if err := validateCLIPathOverrides(); err != nil {
		t.Fatalf("valid absolute path rejected: %v", err)
	}
	if flagData != "/var/lib/search" {
		t.Fatalf("absolute path changed to %q", flagData)
	}
	flagData = "../outside"
	if err := validateCLIPathOverrides(); err == nil {
		t.Fatal("path traversal override was accepted")
	}
}
