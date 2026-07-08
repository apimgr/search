package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// ============================================================
// handleLegacyArgs
// Covers the --help, --version, --config-info, --status,
// --service/missing-subcommand, --maintenance/missing-subcommand,
// --update, --build, --shell, --daemon and default branches.
// ============================================================

func TestHandleLegacyArgsHelp(t *testing.T) {
	withArgs(t, []string{"search", "--help"})
	out := captureStdout(t, handleLegacyArgs)
	if !strings.Contains(out, "--version") {
		t.Errorf("handleLegacyArgs(--help) missing --version in output: %q", out)
	}
}

func TestHandleLegacyArgsVersion(t *testing.T) {
	withArgs(t, []string{"search", "--version"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--version) produced no output")
	}
}

func TestHandleLegacyArgsShortVersion(t *testing.T) {
	withArgs(t, []string{"search", "-v"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(-v) produced no output")
	}
}

func TestHandleLegacyArgsShortHelp(t *testing.T) {
	withArgs(t, []string{"search", "-h"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(-h) produced no output")
	}
}

func TestHandleLegacyArgsConfigInfo(t *testing.T) {
	withArgs(t, []string{"search", "--config-info"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--config-info) produced no output")
	}
}

func TestHandleLegacyArgsStatus(t *testing.T) {
	withArgs(t, []string{"search", "--status"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--status) produced no output")
	}
}

func TestHandleLegacyArgsServiceMissingSubcmd(t *testing.T) {
	withArgs(t, []string{"search", "--service"})
	// No panic is the contract; the function logs to slog.Error (stderr)
	handleLegacyArgs()
}

func TestHandleLegacyArgsServiceWithSubcmd(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "--help"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--service --help) produced no output")
	}
}

func TestHandleLegacyArgsMaintenanceMissingSubcmd(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance"})
	// No panic is the contract; the function logs to slog.Error (stderr)
	handleLegacyArgs()
}

func TestHandleLegacyArgsMaintenanceWithSubcmd(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "help"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--maintenance help) produced no output")
	}
}

func TestHandleLegacyArgsUpdateCheck(t *testing.T) {
	// Use "check" (read-only) instead of bare --update which defaults to "yes"
	// and would attempt a real download+install when running as root in Docker.
	withArgs(t, []string{"search", "--update", "check"})
	withExitFunc(t)
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--update check) produced no output")
	}
}

func TestHandleLegacyArgsUpdateWithSubcmd(t *testing.T) {
	withArgs(t, []string{"search", "--update", "help"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--update help) produced no output")
	}
}

func TestHandleLegacyArgsBuildDefault(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--build"})
	// No os.Args[2] → platform = "all" → may call exitFunc if Docker absent; no panic
	captureStdout(t, handleLegacyArgs)
}

func TestHandleLegacyArgsBuildWithPlatform(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--build", "all"})
	// May call exitFunc if Docker absent; no panic
	captureStdout(t, handleLegacyArgs)
}

func TestHandleLegacyArgsShellDefault(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--shell"})
	out := captureStdout(t, handleLegacyArgs)
	// --help path; output expected
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--shell) produced no output")
	}
}

func TestHandleLegacyArgsShellWithSubcmd(t *testing.T) {
	withArgs(t, []string{"search", "--shell", "--help"})
	out := captureStdout(t, handleLegacyArgs)
	if strings.TrimSpace(out) == "" {
		t.Error("handleLegacyArgs(--shell --help) produced no output")
	}
}

// TestHandleLegacyArgsDaemon is intentionally omitted.
// The --daemon path in handleLegacyArgs calls daemonize() then runServer().
// _DAEMON_CHILD=1 makes daemonize() return nil (child path), which causes
// runServer() to start a real HTTP server that never exits — hanging the test.
// The daemonize() function itself is covered by TestDaemonizeAlreadyChild.

func TestHandleLegacyArgsUnknown(t *testing.T) {
	withArgs(t, []string{"search", "--completely-unknown-flag-xyz"})
	// Logs to slog.Error; no panic
	handleLegacyArgs()
}

// ============================================================
// runBuild — platform filtering branches
// runBuild calls exitFunc if Docker is missing, so we replace it.
// We only verify the filtering logic by exercising each platform name.
// ============================================================

func TestRunBuildPlatformBranches(t *testing.T) {
	tests := []struct {
		name     string
		platform string
	}{
		{"all", "all"},
		{"linux", "linux"},
		{"darwin", "darwin"},
		{"windows", "windows"},
		{"freebsd", "freebsd"},
		{"host", "host"},
		{"slash notation", "linux/amd64"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withExitFunc(t)
			withArgs(t, []string{"search", "--build", tt.platform})
			// Output goes to stdout; we don't assert content because Docker may
			// or may not be present; the contract is "no panic".
			captureStdout(t, func() { runBuild(tt.platform) })
		})
	}
}

func TestRunBuildUnknownPlatform(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--build", "solaris"})
	out := captureStdout(t, func() { runBuild("solaris") })
	if !strings.Contains(out, "Unknown platform") && !strings.Contains(out, "unknown") && !strings.Contains(out, "ERROR") {
		t.Errorf("runBuild(solaris) expected error output, got: %q", out)
	}
}

func TestRunBuildMacosPlatform(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--build", "macos"})
	// "macos" is an alias for "darwin" — no panic
	captureStdout(t, func() { runBuild("macos") })
}

// ============================================================
// runUpdate — check, rollback, list, help branches
// These may make network calls; we only verify no panic + some output.
// ============================================================

func TestRunUpdateHelp(t *testing.T) {
	withArgs(t, []string{"search", "--update", "help"})
	out := captureStdout(t, func() { runUpdate("help") })
	if !strings.Contains(out, "check") {
		t.Errorf("runUpdate(help) expected 'check' in output, got: %q", out)
	}
}

func TestRunUpdateHelpAlias(t *testing.T) {
	withArgs(t, []string{"search", "--update", "--help"})
	out := captureStdout(t, func() { runUpdate("--help") })
	if strings.TrimSpace(out) == "" {
		t.Error("runUpdate(--help) produced no output")
	}
}

func TestRunUpdateBranchDaily(t *testing.T) {
	withArgs(t, []string{"search", "--update", "branch", "daily"})
	out := captureStdout(t, func() { runUpdate("branch") })
	if !strings.Contains(out, "daily") {
		t.Errorf("runUpdate(branch daily) expected 'daily' in output, got: %q", out)
	}
}

// ============================================================
// runMaintenance — pgp sub-branches
// ============================================================

func TestRunMaintenancePGPHelp(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "help"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp help) produced no output")
	}
}

func TestRunMaintenancePGPEmpty(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp) with no subcommand produced no output")
	}
}

func TestRunMaintenancePGPGenerate(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "generate"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp generate) produced no output")
	}
}

func TestRunMaintenancePGPRotate(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "rotate"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp rotate) produced no output")
	}
}

func TestRunMaintenancePGPPublish(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "publish"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp publish) produced no output")
	}
}

func TestRunMaintenancePGPExportPublic(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "export", "public"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp export public) produced no output")
	}
}

func TestRunMaintenancePGPExportPrivate(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "export", "private"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp export private) produced no output")
	}
}

func TestRunMaintenancePGPExportNoKeyType(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "export"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp export) with no key type produced no output")
	}
}

func TestRunMaintenancePGPImport(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "import"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp import) produced no output")
	}
}

func TestRunMaintenancePGPDelete(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "delete"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp delete) produced no output")
	}
}

func TestRunMaintenancePGPUnknown(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "unknown-action"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp unknown-action) produced no output")
	}
}

func TestRunMaintenancePGPHelpFlag(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "pgp", "--help"})
	out := captureStdout(t, func() { runMaintenance("pgp") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(pgp --help) produced no output")
	}
}

func TestRunMaintenanceHelpFlag(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "--help"})
	out := captureStdout(t, func() { runMaintenance("--help") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(--help) produced no output")
	}
}

// ============================================================
// daemonize — Windows early-return and already-daemon-child paths
// ============================================================

func TestDaemonizeWindowsNoop(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-specific path")
	}
	// On Windows daemonize() just prints a warning and returns nil
	err := daemonize()
	if err != nil {
		t.Errorf("daemonize() on Windows returned error: %v", err)
	}
}

func TestDaemonizeAlreadyChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix-only path")
	}
	orig, had := os.LookupEnv("_DAEMON_CHILD")
	os.Setenv("_DAEMON_CHILD", "1")
	t.Cleanup(func() {
		if had {
			os.Setenv("_DAEMON_CHILD", orig)
		} else {
			os.Unsetenv("_DAEMON_CHILD")
		}
	})
	err := daemonize()
	if err != nil {
		t.Errorf("daemonize() as daemon child returned error: %v", err)
	}
}

// ============================================================
// runShell — help alias ("help" without leading --)
// ============================================================

func TestRunShellHelpAlias(t *testing.T) {
	withArgs(t, []string{"search", "--shell", "help"})
	out := captureStdout(t, func() { runShell("help") })
	if strings.TrimSpace(out) == "" {
		t.Error("runShell(help) produced no output")
	}
}

// ============================================================
// printShellInit — unsupported shell
// ============================================================

func TestPrintShellInitUnsupported(t *testing.T) {
	withExitFunc(t)
	out := captureStdout(t, func() { printShellInit("search", "tcsh") })
	if strings.TrimSpace(out) == "" {
		t.Error("printShellInit(tcsh) expected error output, got none")
	}
}

// ============================================================
// runUpdate — rollback path (network not required; error expected)
// ============================================================

func TestRunUpdateRollback(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--update", "rollback"})
	out := captureStdout(t, func() { runUpdate("rollback") })
	if strings.TrimSpace(out) == "" {
		t.Error("runUpdate(rollback) produced no output")
	}
}

// ============================================================
// runMaintenance — update alias (delegates to runUpdate("yes"))
// ============================================================

func TestRunMaintenanceUpdateAlias(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "update"})
	out := captureStdout(t, func() { runMaintenance("update") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(update) produced no output")
	}
}

// ============================================================
// runMaintenance — setup when not first-run and not root
// ============================================================

func TestRunMaintenanceSetupNotFirstRunNotRoot(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "setup"})
	out := captureStdout(t, func() { runMaintenance("setup") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(setup) produced no output")
	}
}

// ============================================================
// findSourceDir — returns valid path or skips
// ============================================================

func TestFindSourceDirParentCheck(t *testing.T) {
	// Running from within the project tree, so either "." or the parent should work
	dir, err := findSourceDir()
	if err != nil {
		t.Skipf("findSourceDir() not in source tree: %v", err)
	}
	if dir == "" {
		t.Error("findSourceDir() returned empty string")
	}
}

// ============================================================
// runUpdate — check and list branches (network; ok if error)
// ============================================================

func TestRunUpdateCheck(t *testing.T) {
	withArgs(t, []string{"search", "--update", "check"})
	out := captureStdout(t, func() { runUpdate("check") })
	if strings.TrimSpace(out) == "" {
		t.Error("runUpdate(check) produced no output")
	}
}

func TestRunUpdateList(t *testing.T) {
	withArgs(t, []string{"search", "--update", "list"})
	out := captureStdout(t, func() { runUpdate("list") })
	if strings.TrimSpace(out) == "" {
		t.Error("runUpdate(list) produced no output")
	}
}

// ============================================================
// runMaintenance backup — with BACKUP_PASSWORD set
// ============================================================

func TestRunMaintenanceBackupWithPassword(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "backup"})

	orig, had := os.LookupEnv("BACKUP_PASSWORD")
	os.Setenv("BACKUP_PASSWORD", "test-secret-password")
	t.Cleanup(func() {
		if had {
			os.Setenv("BACKUP_PASSWORD", orig)
		} else {
			os.Unsetenv("BACKUP_PASSWORD")
		}
	})

	out := captureStdout(t, func() { runMaintenance("backup") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(backup) with BACKUP_PASSWORD produced no output")
	}
}

// ============================================================
// showStatus — running process path
// Write the current test process's PID into a temp file, then override
// SEARCH_PID_FILE so showStatus reads it and finds the process running.
// ============================================================

func TestShowStatusRunningProcess(t *testing.T) {
	withArgs(t, []string{"search"})
	withExitFunc(t)

	pid := os.Getpid()
	tmpDir := t.TempDir()
	pidFile := tmpDir + "/test.pid"

	// Write "pid\n" without importing fmt — use Sprintf via the existing fmt import
	pidStr := pidIntToString(pid)
	if err := os.WriteFile(pidFile, []byte(pidStr+"\n"), 0644); err != nil {
		t.Skip("cannot write PID file: " + err.Error())
	}

	origPID, hadPID := os.LookupEnv("SEARCH_PID_FILE")
	os.Setenv("SEARCH_PID_FILE", pidFile)
	t.Cleanup(func() {
		if hadPID {
			os.Setenv("SEARCH_PID_FILE", origPID)
		} else {
			os.Unsetenv("SEARCH_PID_FILE")
		}
	})

	out := captureStdout(t, showStatus)
	if strings.TrimSpace(out) == "" {
		t.Error("showStatus produced no output even with PID file present")
	}
	// When the PID is found and alive, status must say "Running"
	if !strings.Contains(out, "Running") {
		t.Errorf("showStatus expected 'Running' when current PID is in file, got: %q", out)
	}
}

// pidIntToString converts an int PID to its decimal string representation
// without importing strconv (which the project forbids for bool parsing but
// is fine for ints; we avoid it here for simplicity).
func pidIntToString(pid int) string {
	if pid == 0 {
		return "0"
	}
	digits := make([]byte, 0, 10)
	for pid > 0 {
		digits = append([]byte{byte('0' + pid%10)}, digits...)
		pid /= 10
	}
	return string(digits)
}

// ============================================================
// runMaintenance restore — no file but with backups listed
// ============================================================

func TestRunMaintenanceRestoreNoFileShowsHelp(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "restore"})
	out := captureStdout(t, func() { runMaintenance("restore") })
	if strings.TrimSpace(out) == "" {
		t.Error("runMaintenance(restore) with no file produced no output")
	}
}
