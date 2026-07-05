package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

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

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{
			name:  "zero bytes",
			bytes: 0,
			want:  "0 B",
		},
		{
			name:  "one byte",
			bytes: 1,
			want:  "1 B",
		},
		{
			name:  "1023 bytes stays in B",
			bytes: 1023,
			want:  "1023 B",
		},
		{
			name:  "exactly 1 KB",
			bytes: 1024,
			want:  "1.0 KB",
		},
		{
			name:  "1.5 KB",
			bytes: 1536,
			want:  "1.5 KB",
		},
		{
			name:  "exactly 1 MB",
			bytes: 1024 * 1024,
			want:  "1.0 MB",
		},
		{
			name:  "exactly 1 GB",
			bytes: 1024 * 1024 * 1024,
			want:  "1.0 GB",
		},
		{
			name:  "2.5 MB",
			bytes: int64(2.5 * 1024 * 1024),
			want:  "2.5 MB",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatBytes(tt.bytes)
			if got != tt.want {
				t.Errorf("formatBytes(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

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
			name:     "unknown shell falls back to platform default",
			shellEnv: "/bin/tcsh",
			wantFn: func(got string) bool {
				if runtime.GOOS == "windows" {
					return got == "powershell"
				}
				return got == "bash"
			},
			wantDescr: "bash (linux/darwin) or powershell (windows)",
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
	flagPort = 0
	flagBaseURL = ""
	flagColor = ""
	flagLang = ""
	flagDebug = false
}

func TestApplyCliOverrides(t *testing.T) {
	// Capture and restore all env vars that applyCliOverrides may write
	restore := saveEnvKeys(
		"SEARCH_MODE", "MODE",
		"DEBUG", "SEARCH_DEBUG",
		"SEARCH_DATA_DIR", "SEARCH_CONFIG_DIR",
		"SEARCH_CACHE_DIR", "SEARCH_LOG_DIR",
		"SEARCH_BACKUP_DIR", "SEARCH_PID_FILE",
		"SEARCH_ADDRESS", "SEARCH_PORT", "PORT",
		"SEARCH_BASE_URL", "SEARCH_COLOR",
		"SEARCH_LANG", "LANG",
	)
	t.Cleanup(restore)

	t.Run("flagMode production sets SEARCH_MODE and MODE", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_MODE", "MODE")()
		resetFlagVars()

		flagMode = "production"
		applyCliOverrides()

		if got := os.Getenv("SEARCH_MODE"); got != "production" {
			t.Errorf("SEARCH_MODE = %q, want %q", got, "production")
		}
		if got := os.Getenv("MODE"); got != "production" {
			t.Errorf("MODE = %q, want %q", got, "production")
		}
	})

	t.Run("flagDebug true sets DEBUG and SEARCH_DEBUG", func(t *testing.T) {
		defer saveEnvKeys("DEBUG", "SEARCH_DEBUG")()
		resetFlagVars()

		flagDebug = true
		applyCliOverrides()

		if got := os.Getenv("DEBUG"); got != "true" {
			t.Errorf("DEBUG = %q, want %q", got, "true")
		}
		if got := os.Getenv("SEARCH_DEBUG"); got != "true" {
			t.Errorf("SEARCH_DEBUG = %q, want %q", got, "true")
		}
	})

	t.Run("flagPort sets SEARCH_PORT and PORT", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_PORT", "PORT")()
		resetFlagVars()

		flagPort = 8080
		applyCliOverrides()

		if got := os.Getenv("SEARCH_PORT"); got != "8080" {
			t.Errorf("SEARCH_PORT = %q, want %q", got, "8080")
		}
		if got := os.Getenv("PORT"); got != "8080" {
			t.Errorf("PORT = %q, want %q", got, "8080")
		}
	})

	t.Run("flagBaseURL sets SEARCH_BASE_URL", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_BASE_URL")()
		resetFlagVars()

		flagBaseURL = "/app"
		applyCliOverrides()

		if got := os.Getenv("SEARCH_BASE_URL"); got != "/app" {
			t.Errorf("SEARCH_BASE_URL = %q, want %q", got, "/app")
		}
	})

	t.Run("flagLang sets SEARCH_LANG and LANG", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_LANG", "LANG")()
		resetFlagVars()

		flagLang = "es"
		applyCliOverrides()

		if got := os.Getenv("SEARCH_LANG"); got != "es" {
			t.Errorf("SEARCH_LANG = %q, want %q", got, "es")
		}
		if got := os.Getenv("LANG"); got != "es" {
			t.Errorf("LANG = %q, want %q", got, "es")
		}
	})

	t.Run("flagAddress sets SEARCH_ADDRESS", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_ADDRESS")()
		resetFlagVars()

		flagAddress = "127.0.0.1"
		applyCliOverrides()

		if got := os.Getenv("SEARCH_ADDRESS"); got != "127.0.0.1" {
			t.Errorf("SEARCH_ADDRESS = %q, want %q", got, "127.0.0.1")
		}
	})

	t.Run("flagData with temp dir sets SEARCH_DATA_DIR", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_DATA_DIR")()
		resetFlagVars()

		dir := t.TempDir()
		flagData = dir
		applyCliOverrides()

		if got := os.Getenv("SEARCH_DATA_DIR"); got != dir {
			t.Errorf("SEARCH_DATA_DIR = %q, want %q", got, dir)
		}
	})

	t.Run("flagConfig with temp dir sets SEARCH_CONFIG_DIR", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_CONFIG_DIR")()
		resetFlagVars()

		dir := t.TempDir()
		flagConfig = dir
		applyCliOverrides()

		if got := os.Getenv("SEARCH_CONFIG_DIR"); got != dir {
			t.Errorf("SEARCH_CONFIG_DIR = %q, want %q", got, dir)
		}
	})

	t.Run("zero flagPort leaves SEARCH_PORT unchanged", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_PORT", "PORT")()
		os.Unsetenv("SEARCH_PORT")
		os.Unsetenv("PORT")
		resetFlagVars()

		applyCliOverrides()

		if got := os.Getenv("SEARCH_PORT"); got != "" {
			t.Errorf("SEARCH_PORT = %q, want empty when flagPort == 0", got)
		}
	})

	t.Run("empty flagMode does not force-overwrite existing SEARCH_MODE", func(t *testing.T) {
		defer saveEnvKeys("SEARCH_MODE", "MODE")()
		resetFlagVars()

		// Pre-set SEARCH_MODE to a non-default value
		os.Setenv("SEARCH_MODE", "development")
		// flagMode is empty — applyCliOverrides must call mode.FromEnv(), not
		// os.Setenv("SEARCH_MODE", ...), so the pre-set value must survive
		applyCliOverrides()

		if got := os.Getenv("SEARCH_MODE"); got != "development" {
			t.Errorf("SEARCH_MODE = %q, want %q (empty flagMode must not overwrite)", got, "development")
		}
	})
}
