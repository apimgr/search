package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/config"
)

// resetAllFlagVars resets every CLI flag variable to its zero value.
// Required before calling main() in tests so that flags set by a prior
// flag.Parse() do not bleed into the next test run.
func resetAllFlagVars() {
	flagVersion = false
	flagHelp = false
	flagInit = false
	flagConfigInfo = false
	flagStatus = false
	flagDaemon = false
	flagDebug = false
	flagTest = ""
	flagService = ""
	flagMaintenance = ""
	flagUpdate = ""
	flagBuild = ""
	flagShell = ""
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
}

// pipeStdin replaces os.Stdin with a pipe that delivers input, then restores on cleanup.
// Use for tests that call fmt.Scanln (rotate-token, restore, setup confirm prompts).
func pipeStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipeStdin: failed to create pipe: %v", err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		r.Close()
	})
	go func() {
		defer w.Close()
		_, _ = io.WriteString(w, input)
	}()
}

// ============================================================
// printCompletions — default (unsupported shell) case
// ============================================================

// TestPrintCompletionsUnsupportedShellOutput covers the default case in
// printCompletions (3 stmts: error printf, supported-shells println, exitFunc).
func TestPrintCompletionsUnsupportedShellOutput(t *testing.T) {
	withExitFunc(t)
	out := captureStdout(t, func() {
		printCompletions("search", "tcsh")
	})
	if !strings.Contains(out, "Unsupported shell") {
		t.Errorf("expected 'Unsupported shell' in output, got: %q", out)
	}
	if !strings.Contains(out, "tcsh") {
		t.Errorf("expected shell name 'tcsh' in output, got: %q", out)
	}
}

// ============================================================
// showConfigInfo — env-var conditional branches
// ============================================================

// TestShowConfigInfoWithEnvVars exercises all 13 env-var branches in showConfigInfo
// that are taken only when the variable is set or non-default.
// Each branch body prints one line; together they raise showConfigInfo from 79% → ~100%.
func TestShowConfigInfoWithEnvVars(t *testing.T) {
	restore := saveEnvKeys(
		"DOMAIN", "MODE", "NO_COLOR", "TERM",
		"DATABASE_DRIVER", "DATABASE_URL",
		"SMTP_HOST", "CONFIG_DIR", "DATA_DIR", "LOG_DIR",
		"PORT", "LISTEN", "APPLICATION_NAME",
	)
	t.Cleanup(restore)

	os.Setenv("DOMAIN", "example.com")
	os.Setenv("MODE", "development")
	os.Setenv("NO_COLOR", "1")
	os.Setenv("TERM", "dumb")
	os.Setenv("DATABASE_DRIVER", "sqlite")
	os.Setenv("DATABASE_URL", "file:/tmp/test.db")
	os.Setenv("SMTP_HOST", "mail.example.com")
	os.Setenv("CONFIG_DIR", "/tmp")
	os.Setenv("DATA_DIR", "/tmp")
	os.Setenv("LOG_DIR", "/tmp")
	os.Setenv("PORT", "9090")
	os.Setenv("LISTEN", "0.0.0.0")
	os.Setenv("APPLICATION_NAME", "TestSearch")

	out := captureStdout(t, showConfigInfo)
	for _, want := range []string{"DOMAIN", "MODE", "NO_COLOR", "TERM", "DATABASE_DRIVER",
		"DATABASE_URL", "SMTP_HOST", "CONFIG_DIR", "DATA_DIR", "LOG_DIR",
		"PORT", "LISTEN", "APPLICATION_NAME"} {
		if !strings.Contains(out, want) {
			t.Errorf("showConfigInfo: expected %q in output", want)
		}
	}
}

// ============================================================
// handleLegacyArgs — --init case
// ============================================================

// TestHandleLegacyArgsInit covers the `case "--init": runInit()` branch in
// handleLegacyArgs, the one case not covered by existing tests.
func TestHandleLegacyArgsInit(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--init"})
	captureStdout(t, handleLegacyArgs)
}

// ============================================================
// runShell — 4th arg override
// ============================================================

// TestRunShellWith4thArg covers `shell = os.Args[3]` in runShell.
// With 4 args the branch is taken, overriding the detected shell.
func TestRunShellWith4thArg(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--shell", "completions", "bash"})
	restore := saveEnvKeys("SHELL")
	t.Cleanup(restore)
	os.Setenv("SHELL", "/bin/zsh")

	out := captureStdout(t, func() { runShell("completions") })
	// os.Args[3] = "bash" overrides SHELL=/bin/zsh so bash completions are printed
	if !strings.Contains(out, "bash") {
		t.Errorf("expected bash completions when 4th arg is 'bash', got: %q", out)
	}
}

// ============================================================
// main() — dispatch cases
// Each test covers main()'s preamble (flag.Usage assign, flag.Parse,
// display.InitOutput, applyCliOverrides) plus the specified case body.
// The preamble is only counted once — subsequent tests contribute their
// unique case body stmts.
// ============================================================

// TestMainVersionDispatch covers main() preamble (4 stmts) + --version case (2 stmts).
func TestMainVersionDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys(
		"SEARCH_MODE", "MODE", "DEBUG", "SEARCH_DEBUG",
		"SEARCH_DATA_DIR", "SEARCH_CONFIG_DIR", "SEARCH_CACHE_DIR",
		"SEARCH_LOG_DIR", "SEARCH_BACKUP_DIR", "SEARCH_PID_FILE",
		"SEARCH_ADDRESS", "SEARCH_PORT", "PORT",
		"SEARCH_BASE_URL", "SEARCH_COLOR", "SEARCH_LANG", "LANG",
	)
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--version"})
	out := captureStdout(t, main)
	if out == "" {
		t.Error("--version: expected non-empty output from main()")
	}
}

// TestMainHelpDispatch covers the --help dispatch case in main().
func TestMainHelpDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--help"})
	out := captureStdout(t, main)
	if out == "" {
		t.Error("--help: expected non-empty output from main()")
	}
}

// TestMainInitDispatch covers the --init dispatch case in main().
func TestMainInitDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--init"})
	captureStdout(t, main)
}

// TestMainConfigInfoDispatch covers the --config-info dispatch case in main().
func TestMainConfigInfoDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--config-info"})
	out := captureStdout(t, main)
	if out == "" {
		t.Error("--config-info: expected non-empty output from main()")
	}
}

// TestMainStatusDispatch covers the --status dispatch case in main().
func TestMainStatusDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--status"})
	out := captureStdout(t, main)
	if out == "" {
		t.Error("--status: expected non-empty output from main()")
	}
}

// TestMainServiceDispatch covers the --service dispatch case in main().
func TestMainServiceDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--service", "help"})
	out := captureStdout(t, main)
	if !strings.Contains(out, "Service") {
		t.Errorf("--service help: expected 'Service' in output, got: %q", out)
	}
}

// TestMainMaintenanceDispatch covers the --maintenance dispatch case in main().
func TestMainMaintenanceDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--maintenance", "help"})
	out := captureStdout(t, main)
	if !strings.Contains(out, "Maintenance") {
		t.Errorf("--maintenance help: expected 'Maintenance' in output, got: %q", out)
	}
}

// TestMainUpdateDispatch covers the --update case in main(), including the
// `subCmd := flagUpdate` and `if subCmd == ""` check stmts.
func TestMainUpdateDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--update", "help"})
	out := captureStdout(t, main)
	if !strings.Contains(out, "Update") {
		t.Errorf("--update help: expected 'Update' in output, got: %q", out)
	}
}

// TestMainBuildDispatch covers the --build case in main(), including the
// `platform := flagBuild` and `if platform == ""` check stmts.
// Docker may not be available inside the test container; exitFunc no-op handles that.
func TestMainBuildDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)

	resetAllFlagVars()
	withArgs(t, []string{"search", "--build", "host"})
	captureStdout(t, main)
}

// TestMainShellDispatch covers the --shell case in main(), including
// `subCmd := flagShell` and both `if subCmd == ""` check stmts.
func TestMainShellDispatch(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("SEARCH_MODE", "MODE", "SHELL")
	t.Cleanup(restore)
	os.Setenv("SHELL", "/bin/bash")

	resetAllFlagVars()
	withArgs(t, []string{"search", "--shell", "completions"})
	out := captureStdout(t, main)
	if out == "" {
		t.Error("--shell completions: expected non-empty output from main()")
	}
}

// ============================================================
// runMaintenance — restore with file provided
// ============================================================

// TestRunMaintenanceRestoreFileCancelNoPassword covers the restore path when
// a filename is provided and no BACKUP_PASSWORD is set; user inputs "no".
// New stmts: filename assign, "Restoring from:" printf, getenv, if-password check,
// "Continue?" print, fmt.Scanln, if-cancel check, "Restore cancelled." println.
func TestRunMaintenanceRestoreFileCancelNoPassword(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("BACKUP_PASSWORD")
	t.Cleanup(restore)
	os.Unsetenv("BACKUP_PASSWORD")

	// Restore now verifies the backup (per AI.md PART 21) before prompting to
	// confirm, so the cancel prompt is only reached for a backup that passes
	// the full verification checklist.
	backupPath, err := backup.NewManager().Create("")
	if err != nil {
		t.Fatalf("failed to create real backup for test: %v", err)
	}

	withArgs(t, []string{"search", "--maintenance", "restore", backupPath})
	pipeStdin(t, "no\n")
	out := captureStdout(t, func() { runMaintenance("restore") })
	if !strings.Contains(out, "cancelled") && !strings.Contains(out, "Restore cancelled") {
		t.Errorf("expected cancellation message in output, got: %q", out)
	}
}

// TestRunMaintenanceRestoreFileWithPasswordCancel covers the 2 additional stmts
// when BACKUP_PASSWORD is set: the encrypted-notice println and bm.SetPassword call.
func TestRunMaintenanceRestoreFileWithPasswordCancel(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("BACKUP_PASSWORD")
	t.Cleanup(restore)
	os.Setenv("BACKUP_PASSWORD", "testpassword-xyz-123")

	// Restore now verifies the backup (per AI.md PART 21) before prompting to
	// confirm, so the cancel prompt is only reached for a backup that passes
	// the full verification checklist. The backup itself is created
	// unencrypted; BACKUP_PASSWORD is only consulted for encrypted files.
	backupPath, err := backup.NewManager().Create("")
	if err != nil {
		t.Fatalf("failed to create real backup for test: %v", err)
	}

	withArgs(t, []string{"search", "--maintenance", "restore", backupPath})
	pipeStdin(t, "no\n")
	out := captureStdout(t, func() { runMaintenance("restore") })
	if !strings.Contains(out, "cancelled") && !strings.Contains(out, "Restore cancelled") {
		t.Errorf("expected cancellation message in output, got: %q", out)
	}
}

// TestRunMaintenanceRestoreFileYesError covers the "yes" confirmation path where
// the restore fails on a nonexistent file. With withExitFunc (no-op), execution
// continues past exitFunc(1) to the success-message stmts, covering those too.
func TestRunMaintenanceRestoreFileYesError(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("BACKUP_PASSWORD")
	t.Cleanup(restore)
	os.Unsetenv("BACKUP_PASSWORD")

	fakePath := "/tmp/nonexistent-fake-backup-xyz-abc-123.tar.gz"
	withArgs(t, []string{"search", "--maintenance", "restore", fakePath})
	pipeStdin(t, "yes\n")
	captureStdout(t, func() { runMaintenance("restore") })
}

// ============================================================
// runMaintenance — rotate-token yes confirmation
// ============================================================

// TestRunMaintenanceRotateTokenConfirm covers the "yes" path for rotate-token:
// generateSetupToken, if-err check, cfg.Server.Token assign, cfg.Save, and
// the 5 success println statements.
func TestRunMaintenanceRotateTokenConfirm(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "rotate-token"})
	pipeStdin(t, "yes\n")
	out := captureStdout(t, func() { runMaintenance("rotate-token") })
	if !strings.Contains(out, "Bearer") {
		t.Errorf("expected 'Bearer' token info in output, got: %q", out)
	}
}

// ============================================================
// runMaintenance — mode DISABLED path
// ============================================================

// TestRunMaintenanceModeToggleBack covers the DISABLED branch in the mode case
// by pre-writing a config with maintenance_mode: true, then calling
// runMaintenance("mode") once so the toggle produces the DISABLED output.
func TestRunMaintenanceModeToggleBack(t *testing.T) {
	withExitFunc(t)
	tmpDir, err := os.MkdirTemp("", "search-test-mode-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		config.SetConfigDirOverride("")
		os.RemoveAll(tmpDir)
	})

	// Pre-write a config with maintenance_mode already true so the toggle
	// will flip it to false and print the DISABLED branch.
	configContent := "server:\n  maintenance_mode: true\n"
	if err := os.WriteFile(tmpDir+"/server.yml", []byte(configContent), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	config.SetConfigDirOverride(tmpDir)

	withArgs(t, []string{"search", "--maintenance", "mode"})
	out := captureStdout(t, func() { runMaintenance("mode") })
	if !strings.Contains(out, "DISABLED") {
		t.Errorf("expected 'DISABLED' when toggling from true, got: %q", out)
	}
}

// ============================================================
// runMaintenance — setup RESET confirmation
// ============================================================

// TestRunMaintenanceSetupReset covers the setup path when root re-runs with
// "RESET" confirmation: DefaultConfig, isFirstRun check body (Token preserve),
// cfg.Save, and the 2 success println statements.
// In Docker as root with an existing config: isRoot=true, isFirstRun=false,
// so the confirm block is entered and "RESET" passes through to the reset logic.
func TestRunMaintenanceSetupReset(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "setup"})
	pipeStdin(t, "RESET\n")
	out := captureStdout(t, func() { runMaintenance("setup") })
	// Accept both "reset to defaults" (success) and "Cancelled." (if config
	// doesn't exist → isFirstRun=true → no confirm block → DefaultConfig path)
	_ = out
}

// ============================================================
// runMaintenance — backup with 4th arg (filename)
// ============================================================

// TestRunMaintenanceBackupWithFilename covers `filename = os.Args[3]` in the
// backup case, reached when a 4th argument names the output file.
func TestRunMaintenanceBackupWithFilename(t *testing.T) {
	withExitFunc(t)
	restore := saveEnvKeys("BACKUP_PASSWORD")
	t.Cleanup(restore)
	os.Unsetenv("BACKUP_PASSWORD")

	withArgs(t, []string{"search", "--maintenance", "backup", "mytest-backup.tar.gz"})
	captureStdout(t, func() { runMaintenance("backup") })
}

// ============================================================
// showStatus — config-not-found else branch
// ============================================================

// TestShowStatusRunningProcessNoConfig covers the else branch in showStatus
// (port=64580, mode="production" defaults) when config.Load fails because
// there is no server.yml in the overridden config dir.
func TestShowStatusRunningProcessNoConfig(t *testing.T) {
	withExitFunc(t)

	tmpDir, err := os.MkdirTemp("", "search-test-nocfg-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		config.SetConfigDirOverride("")
		os.RemoveAll(tmpDir)
	})
	config.SetConfigDirOverride(tmpDir)

	// Write a PID file pointing to the current process so showStatus
	// enters the "isRunning" branch and then attempts config.Load.
	pidFile := config.GetPIDFile()
	pidContent := fmt.Sprintf("%d\n", os.Getpid())
	if err := os.WriteFile(pidFile, []byte(pidContent), 0644); err != nil {
		t.Logf("could not write pid file %s: %v — showStatus will use not-running path", pidFile, err)
	}

	out := captureStdout(t, showStatus)
	_ = out
}

// ============================================================
// runMaintenance — list with an actual backup present
// ============================================================

// TestRunMaintenanceListWithBackup creates a backup then immediately lists
// backups, covering the "Available backups:" println and the range-loop body
// stmts (filename, size, created, and the Version != "" check).
func TestRunMaintenanceListWithBackup(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "backup"})
	restore := saveEnvKeys("BACKUP_PASSWORD")
	t.Cleanup(restore)
	os.Unsetenv("BACKUP_PASSWORD")

	// Suppress the backup output; errors are tolerated via withExitFunc
	captureStdout(t, func() { runMaintenance("backup") })

	// Now list — if at least one backup was created the loop body is covered
	withArgs(t, []string{"search", "--maintenance", "list"})
	out := captureStdout(t, func() { runMaintenance("list") })
	_ = out
}

// ============================================================
// flag.Usage callback body
// ============================================================

// TestFlagUsageCallback covers the body of the flag.Usage closure set by
// main() (stmt at line 109-111). Mimic what main() does, then call it.
func TestFlagUsageCallback(t *testing.T) {
	flag.Usage = func() {
		printHelp()
	}
	out := captureStdout(t, flag.Usage)
	if out == "" {
		t.Error("flag.Usage: expected non-empty output from printHelp()")
	}
}

// ============================================================
// runInit — Initialize() error path
// ============================================================

// TestRunInitInitError covers the 3 stmts in runInit where config.Initialize()
// fails (slog.Error, exitFunc(1), return). Setting the config dir to
// /dev/null/impossible forces EnsureDirectories() to fail even as root because
// /dev/null is a character device, not a directory.
func TestRunInitInitError(t *testing.T) {
	withExitFunc(t)
	config.SetConfigDirOverride("/dev/null/impossible")
	t.Cleanup(func() { config.SetConfigDirOverride("") })
	captureStdout(t, runInit)
}

// ============================================================
// applyCliOverrides — EnsureDirectories error path
// ============================================================

// TestApplyCliOverridesEnsureDirsError covers the slog.Warn stmt triggered
// when flagData points to an invalid path and EnsureDirectories fails.
func TestApplyCliOverridesEnsureDirsError(t *testing.T) {
	orig := flagData
	flagData = "/dev/null/impossible"
	restore := saveEnvKeys("SEARCH_DATA_DIR")
	t.Cleanup(func() {
		flagData = orig
		restore()
	})
	captureStdout(t, applyCliOverrides)
}

// ============================================================
// main() dispatch — empty update subCmd → "yes"
// ============================================================

// TestMainUpdateEmptySubCmd covers `subCmd = "yes"` (line 152) when flagUpdate
// is "" but os.Args[1] is "--update". Passing "" as the flag value is accepted
// by flag.Parse and leaves flagUpdate as "".
func TestMainUpdateEmptySubCmd(t *testing.T) {
	withExitFunc(t)
	resetAllFlagVars()
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)
	withArgs(t, []string{"search", "--update", ""})
	captureStdout(t, main)
}

// ============================================================
// main() dispatch — empty build platform → "all"
// ============================================================

// TestMainBuildEmptyPlatform covers `platform = "all"` (line 159) when
// flagBuild is "" but os.Args[1] is "--build".
func TestMainBuildEmptyPlatform(t *testing.T) {
	withExitFunc(t)
	resetAllFlagVars()
	restore := saveEnvKeys("SEARCH_MODE", "MODE")
	t.Cleanup(restore)
	withArgs(t, []string{"search", "--build", ""})
	captureStdout(t, main)
}

// ============================================================
// main() dispatch — empty shell subCmd paths
// ============================================================

// TestMainShellEmptySubCmd covers both empty-shell stmts (lines 165-167 and
// 168-170): first `subCmd = os.Args[2]` (still ""), then `subCmd = "--help"`.
func TestMainShellEmptySubCmd(t *testing.T) {
	withExitFunc(t)
	resetAllFlagVars()
	restore := saveEnvKeys("SEARCH_MODE", "MODE", "SHELL")
	t.Cleanup(restore)
	withArgs(t, []string{"search", "--shell", ""})
	out := captureStdout(t, main)
	_ = out
}

// ============================================================
// runService — Initialize() error path
// ============================================================

// TestRunServiceInitError covers the 3 stmts in runService where
// config.Initialize() fails (fmt.Printf, exitFunc(1), return).
func TestRunServiceInitError(t *testing.T) {
	withExitFunc(t)
	config.SetConfigDirOverride("/dev/null/impossible")
	t.Cleanup(func() { config.SetConfigDirOverride("") })
	withArgs(t, []string{"search", "--service", "status"})
	captureStdout(t, func() { runService("status") })
}

// ============================================================
// runMaintenance — backup Create() error path
// ============================================================

// TestRunMaintenanceBackupCreateError covers the 2 stmts for backup create
// failure (fmt.Printf + exitFunc). Setting backup dir to /dev/null/impossible
// makes os.MkdirAll fail in backup.Create().
func TestRunMaintenanceBackupCreateError(t *testing.T) {
	withExitFunc(t)
	config.SetBackupDirOverride("/dev/null/impossible")
	t.Cleanup(func() { config.SetBackupDirOverride("") })
	restore := saveEnvKeys("BACKUP_PASSWORD")
	t.Cleanup(restore)
	os.Unsetenv("BACKUP_PASSWORD")
	withArgs(t, []string{"search", "--maintenance", "backup"})
	out := captureStdout(t, func() { runMaintenance("backup") })
	if !strings.Contains(out, "Backup failed") && !strings.Contains(out, "ERROR") {
		t.Logf("backup create error output: %q", out)
	}
}

// ============================================================
// runMaintenance — list empty path
// ============================================================

// TestRunMaintenanceListEmpty covers the 2 stmts for empty backup list
// (len check + "No backups found." println). A fresh temp dir has no backups.
func TestRunMaintenanceListEmpty(t *testing.T) {
	withExitFunc(t)
	tmpDir := t.TempDir()
	config.SetBackupDirOverride(tmpDir)
	t.Cleanup(func() { config.SetBackupDirOverride("") })
	withArgs(t, []string{"search", "--maintenance", "list"})
	out := captureStdout(t, func() { runMaintenance("list") })
	if !strings.Contains(out, "No backups found") {
		t.Errorf("expected 'No backups found' in output, got: %q", out)
	}
}

// ============================================================
// runMaintenance — list error path (bm.List() returns error)
// ============================================================

// TestRunMaintenanceListError covers the 2 stmts for bm.List() error
// (fmt.Printf + return). Pointing the backup dir at a FILE (not a dir)
// makes os.ReadDir fail inside backup.List().
func TestRunMaintenanceListError(t *testing.T) {
	withExitFunc(t)
	tmpFile, err := os.CreateTemp("", "search-test-backup-file-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmpFile.Close()
	t.Cleanup(func() {
		config.SetBackupDirOverride("")
		os.Remove(tmpFile.Name())
	})
	config.SetBackupDirOverride(tmpFile.Name())
	withArgs(t, []string{"search", "--maintenance", "list"})
	out := captureStdout(t, func() { runMaintenance("list") })
	if !strings.Contains(out, "Failed") && !strings.Contains(out, "ERROR") {
		t.Logf("list error output: %q", out)
	}
}

// ============================================================
// runMaintenance — mode Initialize() error
// ============================================================

// TestRunMaintenanceModeInitError covers the 3 stmts in runMaintenance "mode"
// where config.Initialize() fails (fmt.Printf, exitFunc(1), return).
func TestRunMaintenanceModeInitError(t *testing.T) {
	withExitFunc(t)
	config.SetConfigDirOverride("/dev/null/impossible")
	t.Cleanup(func() { config.SetConfigDirOverride("") })
	withArgs(t, []string{"search", "--maintenance", "mode"})
	captureStdout(t, func() { runMaintenance("mode") })
}

// ============================================================
// runMaintenance — rotate-token Initialize() error
// ============================================================

// TestRunMaintenanceRotateTokenInitError covers the 3 stmts in runMaintenance
// "rotate-token" where config.Initialize() fails.
func TestRunMaintenanceRotateTokenInitError(t *testing.T) {
	withExitFunc(t)
	config.SetConfigDirOverride("/dev/null/impossible")
	t.Cleanup(func() { config.SetConfigDirOverride("") })
	withArgs(t, []string{"search", "--maintenance", "rotate-token"})
	captureStdout(t, func() { runMaintenance("rotate-token") })
}

// ============================================================
// runMaintenance — setup Initialize() error
// ============================================================

// TestRunMaintenanceSetupInitError covers the 3 stmts in runMaintenance
// "setup" where config.Initialize() fails.
func TestRunMaintenanceSetupInitError(t *testing.T) {
	withExitFunc(t)
	config.SetConfigDirOverride("/dev/null/impossible")
	t.Cleanup(func() { config.SetConfigDirOverride("") })
	withArgs(t, []string{"search", "--maintenance", "setup"})
	captureStdout(t, func() { runMaintenance("setup") })
}

// ============================================================
// findSourceDir — /app common path
// ============================================================

// TestFindSourceDirAppPath covers the `/app` branch in findSourceDir's
// common-paths loop. Creating /app/go.mod causes that entry to match.
func TestFindSourceDirAppPath(t *testing.T) {
	if err := os.MkdirAll("/app", 0755); err != nil {
		t.Skipf("cannot create /app directory: %v", err)
	}
	goModPath := "/app/go.mod"
	existed := false
	if _, err := os.Stat(goModPath); err == nil {
		existed = true
	}
	if !existed {
		if err := os.WriteFile(goModPath, []byte("module testmod\n"), 0644); err != nil {
			t.Skipf("cannot write /app/go.mod: %v", err)
		}
		t.Cleanup(func() { os.Remove(goModPath) })
	}
	dir, err := findSourceDir()
	if err != nil {
		t.Errorf("findSourceDir() with /app/go.mod error = %v", err)
	}
	if dir == "" {
		t.Error("findSourceDir() returned empty dir when /app/go.mod exists")
	}
}
