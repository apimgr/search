package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/version"
)

// ============================================================
// getProcessStartTime
// ============================================================

func TestGetProcessStartTime(t *testing.T) {
	t.Run("current process returns non-zero on linux", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("proc filesystem only available on Linux")
		}
		pid := os.Getpid()
		got := getProcessStartTime(pid)
		if got.IsZero() {
			t.Error("getProcessStartTime(current pid) returned zero time, want non-zero on Linux")
		}
		// start time must be in the past
		if !got.Before(time.Now()) {
			t.Errorf("getProcessStartTime(current pid) = %v, want a time before now", got)
		}
	})

	t.Run("nonexistent PID returns zero time", func(t *testing.T) {
		got := getProcessStartTime(99999999)
		if !got.IsZero() {
			t.Errorf("getProcessStartTime(99999999) = %v, want zero time", got)
		}
	})

	t.Run("non-linux returns zero time", func(t *testing.T) {
		if runtime.GOOS == "linux" {
			t.Skip("this tests non-Linux fallback path")
		}
		got := getProcessStartTime(os.Getpid())
		if !got.IsZero() {
			t.Errorf("getProcessStartTime on %s = %v, want zero time", runtime.GOOS, got)
		}
	})

	t.Run("does not panic on PID 0", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("getProcessStartTime(0) panicked: %v", r)
			}
		}()
		getProcessStartTime(0)
	})
}

// ============================================================
// buildListenURLs
// ============================================================

func newMinimalConfig(addr string, port int) *config.Config {
	cfg := &config.Config{}
	cfg.Server.Address = addr
	cfg.Server.Port = port
	return cfg
}

func TestBuildListenURLs(t *testing.T) {
	t.Run("basic http with explicit address and port", func(t *testing.T) {
		cfg := newMinimalConfig("127.0.0.1", 8080)
		urls := buildListenURLs(cfg)
		if len(urls) == 0 {
			t.Fatal("buildListenURLs() returned empty slice")
		}
		want := "http://127.0.0.1:8080"
		if urls[0] != want {
			t.Errorf("urls[0] = %q, want %q", urls[0], want)
		}
	})

	t.Run("wildcard address 0.0.0.0 displays as localhost", func(t *testing.T) {
		cfg := newMinimalConfig("0.0.0.0", 9090)
		urls := buildListenURLs(cfg)
		if !strings.HasPrefix(urls[0], "http://localhost:") {
			t.Errorf("urls[0] = %q, want http://localhost:...", urls[0])
		}
	})

	t.Run("empty address defaults to 0.0.0.0 behaviour (localhost display)", func(t *testing.T) {
		cfg := newMinimalConfig("", 7070)
		urls := buildListenURLs(cfg)
		if !strings.HasPrefix(urls[0], "http://localhost:") {
			t.Errorf("urls[0] = %q, want http://localhost:... for empty address", urls[0])
		}
	})

	t.Run("ipv6 wildcard :: displays as localhost", func(t *testing.T) {
		cfg := newMinimalConfig("::", 6060)
		urls := buildListenURLs(cfg)
		if !strings.HasPrefix(urls[0], "http://localhost:") {
			t.Errorf("urls[0] = %q, want http://localhost:... for :: address", urls[0])
		}
	})

	t.Run("https url added when SSL enabled and HTTPSPort set", func(t *testing.T) {
		cfg := newMinimalConfig("127.0.0.1", 8080)
		cfg.Server.SSL.Enabled = true
		cfg.Server.HTTPSPort = 8443
		urls := buildListenURLs(cfg)
		found := false
		for _, u := range urls {
			if strings.HasPrefix(u, "https://") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("buildListenURLs() = %v, want an https:// URL when SSL enabled", urls)
		}
	})

	t.Run("https not added when SSL enabled but HTTPSPort is 0", func(t *testing.T) {
		cfg := newMinimalConfig("127.0.0.1", 8080)
		cfg.Server.SSL.Enabled = true
		cfg.Server.HTTPSPort = 0
		urls := buildListenURLs(cfg)
		for _, u := range urls {
			if strings.HasPrefix(u, "https://") {
				t.Errorf("buildListenURLs() produced %q but HTTPSPort is 0", u)
			}
		}
	})

	t.Run("tor onion address added when tor enabled and address set", func(t *testing.T) {
		cfg := newMinimalConfig("127.0.0.1", 8080)
		cfg.Server.Tor.Enabled = true
		cfg.Server.Tor.OnionAddress = "abc123fake.onion"
		urls := buildListenURLs(cfg)
		found := false
		for _, u := range urls {
			if strings.Contains(u, "abc123fake.onion") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("buildListenURLs() = %v, want onion address in list", urls)
		}
	})

	t.Run("tor url not added when onion address is empty", func(t *testing.T) {
		cfg := newMinimalConfig("127.0.0.1", 8080)
		cfg.Server.Tor.Enabled = true
		cfg.Server.Tor.OnionAddress = ""
		urls := buildListenURLs(cfg)
		for _, u := range urls {
			if strings.Contains(u, ".onion") {
				t.Errorf("buildListenURLs() produced %q but OnionAddress is empty", u)
			}
		}
	})

	t.Run("all three urls http https tor", func(t *testing.T) {
		cfg := newMinimalConfig("127.0.0.1", 8080)
		cfg.Server.SSL.Enabled = true
		cfg.Server.HTTPSPort = 8443
		cfg.Server.Tor.Enabled = true
		cfg.Server.Tor.OnionAddress = "xyz789fake.onion"
		urls := buildListenURLs(cfg)
		if len(urls) != 3 {
			t.Errorf("buildListenURLs() returned %d urls, want 3: %v", len(urls), urls)
		}
	})
}

// ============================================================
// printVersion — output smoke tests
// ============================================================

// captureStdout redirects os.Stdout during fn, returns captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	io.Copy(&buf, r)
	r.Close()
	return buf.String()
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	if err := w.Close(); err != nil {
		t.Errorf("close captured stderr: %v", err)
	}
	os.Stderr = old
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Errorf("read captured stderr: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Errorf("close captured stderr pipe: %v", err)
	}
	return buf.String()
}

func TestPrintVersion(t *testing.T) {
	out := captureStdout(t, printVersion)

	t.Run("contains Built: line", func(t *testing.T) {
		if !strings.Contains(out, version.LabelBuilt) {
			t.Errorf("printVersion() output missing %q\n%s", version.LabelBuilt, out)
		}
	})

	t.Run("contains Go: line", func(t *testing.T) {
		if !strings.Contains(out, version.LabelGo) {
			t.Errorf("printVersion() output missing %q\n%s", version.LabelGo, out)
		}
	})

	t.Run("contains OS/Arch: line", func(t *testing.T) {
		if !strings.Contains(out, version.LabelOSArch) {
			t.Errorf("printVersion() output missing %q\n%s", version.LabelOSArch, out)
		}
	})

	t.Run("output is non-empty", func(t *testing.T) {
		if strings.TrimSpace(out) == "" {
			t.Error("printVersion() produced no output")
		}
	})
}

// ============================================================
// printHelp — output smoke tests
// ============================================================

func TestPrintHelp(t *testing.T) {
	out := captureStdout(t, printHelp)

	mustContain := []string{
		"--version",
		"--help",
		"--mode",
		"--port",
		"--service",
		"--maintenance",
		"--update",
		"--shell",
		"--debug",
	}
	for _, keyword := range mustContain {
		if !strings.Contains(out, keyword) {
			t.Errorf("printHelp() output missing %q", keyword)
		}
	}
}

// ============================================================
// printCompletions — all four shells
// ============================================================

func TestPrintCompletions(t *testing.T) {
	tests := []struct {
		shell    string
		mustHave []string
	}{
		{
			shell:    "bash",
			mustHave: []string{"COMPREPLY", "--service", "--maintenance", "--update", "tor"},
		},
		{
			shell:    "zsh",
			mustHave: []string{"_arguments", "--service", "--update", "scheduler tor"},
		},
		{
			// fish uses -l <name> (no leading --) for long option names
			shell:    "fish",
			mustHave: []string{"complete", "-l service", "-l update", "scheduler tor"},
		},
		{
			shell:    "powershell",
			mustHave: []string{"Register-ArgumentCompleter", "--service", "--update", "--maintenance tor"},
		},
		{shell: "sh", mustHave: []string{"POSIX completion candidates", "_search_completion_candidates", "--update", "check yes branch"}},
		{shell: "dash", mustHave: []string{"POSIX completion candidates", "_search_completion_candidates", "--service", "--maintenance"}},
		{shell: "ksh", mustHave: []string{"POSIX completion candidates", "_search_completion_candidates", "--shell"}},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			out := captureStdout(t, func() {
				printCompletions("search", tt.shell)
			})
			for _, kw := range tt.mustHave {
				if !strings.Contains(out, kw) {
					t.Errorf("printCompletions(%q) missing %q", tt.shell, kw)
				}
			}
			if strings.Contains(out, "'tor[Tor hidden service management]") || strings.Contains(out, "-a 'tor' -d 'Tor hidden service") {
				t.Errorf("printCompletions(%q) exposes Tor as a top-level command", tt.shell)
			}
		})
	}
}

func TestMaintenanceTorHelpDispatch(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{"search", "--maintenance", "tor", "--help"}
	t.Cleanup(func() { os.Args = oldArgs })
	out := captureStdout(t, func() { runMaintenance("tor") })
	if !strings.Contains(out, "Usage: search --maintenance tor <subcommand>") {
		t.Fatalf("Tor maintenance dispatch help = %q", out)
	}
}

func TestPrintCompletionsUnsupportedShellExits(t *testing.T) {
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	oldStderr, oldExit := os.Stderr, exitFunc
	os.Stderr = writePipe
	var exitCode int
	exitFunc = func(code int) { exitCode = code }
	t.Cleanup(func() {
		os.Stderr, exitFunc = oldStderr, oldExit
		if err := writePipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close stderr writer: %v", err)
		}
		if err := readPipe.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("close stderr reader: %v", err)
		}
	})

	printCompletions("search", "unsupported")
	if err := writePipe.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}
	stderr, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}
	if exitCode != 2 {
		t.Fatalf("unsupported shell exit code = %d, want 2", exitCode)
	}
	if !strings.Contains(string(stderr), "Supported shells:") {
		t.Fatalf("unsupported shell stderr = %q, want supported shell usage", stderr)
	}
}

// ============================================================
// printShellInit
// ============================================================

func TestPrintShellInit(t *testing.T) {
	tests := []struct {
		shell   string
		contain string
	}{
		{"bash", "completions bash"},
		{"zsh", "completions zsh"},
		{"fish", "completions fish"},
		{"sh", "completions sh"},
		{"dash", "completions dash"},
		{"ksh", "completions ksh"},
		{"powershell", "completions powershell"},
		{"pwsh", "completions powershell"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			out := captureStdout(t, func() {
				printShellInit("search", tt.shell)
			})
			if !strings.Contains(out, tt.contain) {
				t.Errorf("printShellInit(%q) output %q does not contain %q", tt.shell, out, tt.contain)
			}
		})
	}
}

// ============================================================
// printShellHelp
// ============================================================

func TestPrintShellHelp(t *testing.T) {
	out := captureStdout(t, func() {
		printShellHelp("search")
	})

	mustContain := []string{"bash", "zsh", "fish", "powershell", "completions", "init"}
	for _, kw := range mustContain {
		if !strings.Contains(out, kw) {
			t.Errorf("printShellHelp() output missing %q", kw)
		}
	}
	if strings.TrimSpace(out) == "" {
		t.Error("printShellHelp() produced no output")
	}
}

// ============================================================
// formatUptime edge cases not in main_test.go
// ============================================================

func TestFormatUptimeAdditionalEdges(t *testing.T) {
	tests := []struct {
		name string
		d    time.Duration
		want string
	}{
		{
			name: "sub-minute duration rounds down to 0m",
			d:    30 * time.Second,
			want: "0m",
		},
		{
			name: "23 hours 59 minutes — just below 1 day",
			d:    23*time.Hour + 59*time.Minute,
			want: "23h 59m",
		},
		{
			name: "large uptime 365 days",
			d:    365 * 24 * time.Hour,
			want: fmt.Sprintf("%dd %dh %dm", 365, 0, 0),
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
// formatBytes edge cases not in main_test.go
// ============================================================

// ============================================================
// generateSetupToken — idempotency / entropy
// ============================================================

func TestGenerateSetupTokenEntropy(t *testing.T) {
	const runs = 20
	seen := make(map[string]bool, runs)
	for i := 0; i < runs; i++ {
		tok, err := generateSetupToken()
		if err != nil {
			t.Fatalf("generateSetupToken() run %d error = %v", i, err)
		}
		if seen[tok] {
			t.Errorf("generateSetupToken() produced duplicate token %q after %d calls", tok, i)
		}
		seen[tok] = true
	}
}

// ============================================================
// applyCliOverrides — color and cache/log/backup dir overrides
// ============================================================

func TestApplyCliOverridesColorAndDirs(t *testing.T) {
	t.Cleanup(func() {
		resetFlagVars()
		config.SetColorMode("")
		config.SetCacheDirOverride("")
		config.SetLogDirOverride("")
		config.SetBackupDirOverride("")
		config.SetPIDFileOverride("")
	})

	t.Run("flagColor sets the config override", func(t *testing.T) {
		resetFlagVars()

		flagColor = "never"
		applyCliOverrides()

		if got := config.GetColorMode(); got != "never" {
			t.Errorf("GetColorMode() = %q, want never", got)
		}
		config.SetColorMode("")
	})

	t.Run("flagCache sets the config override", func(t *testing.T) {
		resetFlagVars()

		dir := t.TempDir()
		flagCache = dir
		applyCliOverrides()

		if got := config.GetCacheDir(); got != dir {
			t.Errorf("GetCacheDir() = %q, want %q", got, dir)
		}
		config.SetCacheDirOverride("")
	})

	t.Run("flagLog sets the config override", func(t *testing.T) {
		resetFlagVars()

		dir := t.TempDir()
		flagLog = dir
		applyCliOverrides()

		if got := config.GetLogDir(); got != dir {
			t.Errorf("GetLogDir() = %q, want %q", got, dir)
		}
		config.SetLogDirOverride("")
	})

	t.Run("flagBackup sets the config override", func(t *testing.T) {
		resetFlagVars()

		dir := t.TempDir()
		flagBackup = dir
		applyCliOverrides()

		if got := config.GetBackupDir(); got != dir {
			t.Errorf("GetBackupDir() = %q, want %q", got, dir)
		}
		config.SetBackupDirOverride("")
	})

	t.Run("flagPID sets the config override", func(t *testing.T) {
		resetFlagVars()
		originalPIDEnabled := config.PIDFileEnabled()
		t.Cleanup(func() { config.SetPIDFileEnabled(originalPIDEnabled) })

		flagPID = "/tmp/test-search.pid"
		applyCliOverrides()

		if config.IsRunningInContainer() {
			if config.GetPIDFile() != "" {
				t.Errorf("GetPIDFile() = %q, want empty path in a container", config.GetPIDFile())
			}
		} else if got := config.GetPIDFile(); got != "/tmp/test-search.pid" {
			t.Errorf("GetPIDFile() = %q, want /tmp/test-search.pid", got)
		}
		config.SetPIDFileOverride("")
	})
}

// ============================================================
// isProcessRunning — additional coverage
// ============================================================

func TestIsProcessRunningParentProcess(t *testing.T) {
	ppid := os.Getppid()
	if ppid <= 0 {
		t.Skip("no valid parent PID available")
	}
	// Parent should be running too
	if !isProcessRunning(ppid) {
		t.Errorf("isProcessRunning(%d) = false, want true for parent process", ppid)
	}
}

// Note: runShell is not directly tested because it reads os.Args[3] raw, and
// test runner flags (e.g. -test.coverprofile=...) end up in os.Args[3], which
// hits the unsupported-shell branch and calls os.Exit(1). The functions it
// dispatches to (printCompletions, printShellInit, printShellHelp) are tested
// separately above with full branch coverage.

// ============================================================
// detectShell — pwsh path
// ============================================================

func TestDetectShellPwsh(t *testing.T) {
	orig, had := os.LookupEnv("SHELL")
	t.Cleanup(func() {
		if had {
			os.Setenv("SHELL", orig)
		} else {
			os.Unsetenv("SHELL")
		}
	})

	os.Setenv("SHELL", "/usr/bin/pwsh")
	got := detectShell()
	if got != "pwsh" {
		t.Errorf("detectShell() with SHELL=pwsh = %q, want %q", got, "pwsh")
	}
}
