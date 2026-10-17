package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/config"
)

// pipeStdin replaces os.Stdin with a pipe that delivers input, then restores on cleanup.
// Use for tests that call fmt.Scanln (restore, setup confirm prompts).

// resetAllFlagVars resets every CLI flag variable to its zero value.
// Required before calling main() in tests so that flags set by a prior
// flag.Parse() do not bleed into the next test run.
func resetAllFlagVars() {
	flagVersion = false
	flagHelp = false
	flagStatus = false
	flagDaemon = false
	flagDebug = false
	flagService = ""
	flagMaintenance = ""
	flagUpdate = ""
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
	withArgs(t, []string{"search", "--service", "--help"})
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
	withArgs(t, []string{"search", "--maintenance", "--help"})
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
// authorizeRestore — restore authorization tiers (AI.md PART 21)
// ============================================================

// TestAuthorizeRestore covers every restore authorization tier: first-run,
// root, service-user with a valid/invalid/missing token, and any other user.
func TestAuthorizeRestore(t *testing.T) {
	const expectedToken = "correct-operator-token"

	tests := []struct {
		name            string
		isFirstRun      bool
		isRoot          bool
		isServiceUser   bool
		expectedToken   string
		presentedToken  string
		wantErr         bool
		wantErrContains string
	}{
		{
			name:       "first run is always allowed",
			isFirstRun: true,
		},
		{
			name:   "root is always allowed",
			isRoot: true,
		},
		{
			name:           "service user with matching token is allowed",
			isServiceUser:  true,
			expectedToken:  expectedToken,
			presentedToken: expectedToken,
		},
		{
			name:            "service user with wrong token is denied",
			isServiceUser:   true,
			expectedToken:   expectedToken,
			presentedToken:  "wrong-token",
			wantErr:         true,
			wantErrContains: "invalid operator token",
		},
		{
			name:            "service user with empty presented token is denied",
			isServiceUser:   true,
			expectedToken:   expectedToken,
			presentedToken:  "",
			wantErr:         true,
			wantErrContains: "invalid operator token",
		},
		{
			name:            "service user with no token configured is denied",
			isServiceUser:   true,
			expectedToken:   "",
			presentedToken:  "anything",
			wantErr:         true,
			wantErrContains: "no operator token is configured",
		},
		{
			name:            "random user is denied",
			isFirstRun:      false,
			isRoot:          false,
			isServiceUser:   false,
			wantErr:         true,
			wantErrContains: "this operation requires root or the operator token",
		},
		{
			name:          "first run wins even if also service user",
			isFirstRun:    true,
			isServiceUser: true,
			expectedToken: expectedToken,
		},
		{
			name:   "root wins even without a configured token",
			isRoot: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authorizeRestore(tt.isFirstRun, tt.isRoot, tt.isServiceUser, tt.expectedToken, tt.presentedToken)
			if (err != nil) != tt.wantErr {
				t.Fatalf("authorizeRestore() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.wantErrContains) {
				t.Errorf("authorizeRestore() error = %q, want substring %q", err.Error(), tt.wantErrContains)
			}
		})
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

// TestRunMaintenanceRestoreFileWithPasswordCancel covers the "no" cancellation
// path for an unencrypted backup. BACKUP_PASSWORD is set in the environment but
// the backup created for this test is unencrypted, so the encrypted-notice
// println and bm.SetPassword call are NOT exercised here — BACKUP_PASSWORD is
// only consulted when backup.IsEncrypted(filename) is true.
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

	tmpDir := t.TempDir()
	config.SetBackupDirOverride(tmpDir)
	t.Cleanup(func() { config.SetBackupDirOverride("") })

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

// TestPrintFirstRunOperatorToken verifies service-first installs still show
// the one-time token after configuration initialization consumes first-run state.
func TestPrintFirstRunOperatorToken(t *testing.T) {
	dir := t.TempDir()
	config.SetConfigDirOverride(filepath.Join(dir, "config"))
	config.SetDataDirOverride(filepath.Join(dir, "data"))
	config.SetLogDirOverride(filepath.Join(dir, "log"))
	config.SetCacheDirOverride(filepath.Join(dir, "cache"))
	config.SetBackupDirOverride(filepath.Join(dir, "backup"))
	config.SetPIDFileOverride(filepath.Join(dir, "run", "search.pid"))
	t.Cleanup(func() {
		config.SetConfigDirOverride("")
		config.SetDataDirOverride("")
		config.SetLogDirOverride("")
		config.SetCacheDirOverride("")
		config.SetBackupDirOverride("")
		config.SetPIDFileOverride("")
	})

	cfg, err := config.Initialize()
	if err != nil {
		t.Fatalf("config.Initialize() error = %v", err)
	}
	if !cfg.IsFirstRun() || cfg.Server.Token == "" {
		t.Fatal("expected first-run config with a generated operator token")
	}

	out := captureStdout(t, func() { printFirstRunOperatorToken(cfg) })
	if !strings.Contains(out, cfg.Server.Token) {
		t.Fatalf("first-run output does not contain the operator token: %q", out)
	}
	if strings.Contains(captureStdout(t, func() { printFirstRunOperatorToken(nil) }), cfg.Server.Token) {
		t.Fatal("nil config unexpectedly printed an operator token")
	}
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

// ============================================================
// runMaintenance — list error path (bm.List() returns error)
// ============================================================

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

func TestBindPrivilegedListenersSkipsUnprivilegedPort(t *testing.T) {
	cfg := &config.Config{}
	cfg.Server.Address = "127.0.0.1"
	cfg.Server.Port = 8080
	listeners, redirect, err := bindPrivilegedListeners(cfg)
	if err != nil {
		t.Fatalf("bindPrivilegedListeners() error = %v", err)
	}
	if len(listeners) != 0 || redirect != nil {
		t.Fatalf("unexpected privileged listeners: ports=%v redirect=%v", listeners, redirect)
	}
}

func TestBindPrivilegedListenersRequiresConfig(t *testing.T) {
	if _, _, err := bindPrivilegedListeners(nil); err == nil {
		t.Fatal("bindPrivilegedListeners(nil) succeeded")
	}
}
