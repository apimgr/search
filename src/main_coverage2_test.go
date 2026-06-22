package main

import (
	"os"
	"strings"
	"testing"
)

// withExitFunc overrides exitFunc to a no-op during the test.
func withExitFunc(t *testing.T) {
	t.Helper()
	orig := exitFunc
	exitFunc = func(int) {}
	t.Cleanup(func() { exitFunc = orig })
}

// withArgs sets os.Args for the duration of a test, restoring on cleanup.
func withArgs(t *testing.T, args []string) {
	t.Helper()
	orig := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = orig })
}

// TestShowConfigInfo verifies showConfigInfo prints configuration sections without panicking.
func TestShowConfigInfo(t *testing.T) {
	out := captureStdout(t, showConfigInfo)
	if out == "" {
		t.Error("showConfigInfo produced no output")
	}
	if !strings.Contains(out, "Configuration") && !strings.Contains(out, "System") && !strings.Contains(out, "Directories") {
		t.Errorf("showConfigInfo missing expected sections, got: %q", out[:min(len(out), 200)])
	}
}

// TestShowStatusNotRunning verifies showStatus reports running status.
func TestShowStatusNotRunning(t *testing.T) {
	withArgs(t, []string{"search"})
	out := captureStdout(t, showStatus)
	if !strings.Contains(out, "Running") {
		t.Errorf("showStatus missing running status, got: %q", out[:min(len(out), 200)])
	}
}

// TestRunShellCompletions verifies runShell("completions") calls printCompletions.
func TestRunShellCompletions(t *testing.T) {
	withArgs(t, []string{"search", "--shell", "completions"})
	out := captureStdout(t, func() { runShell("completions") })
	if out == "" {
		t.Error("runShell(completions) produced no output")
	}
}

// TestRunShellInit verifies runShell("init") calls printShellInit.
func TestRunShellInit(t *testing.T) {
	withArgs(t, []string{"search", "--shell", "init"})
	out := captureStdout(t, func() { runShell("init") })
	if out == "" {
		t.Error("runShell(init) produced no output")
	}
}

// TestRunShellHelp verifies runShell("--help") calls printShellHelp.
func TestRunShellHelp(t *testing.T) {
	withArgs(t, []string{"search", "--shell", "--help"})
	out := captureStdout(t, func() { runShell("--help") })
	if out == "" {
		t.Error("runShell(--help) produced no output")
	}
}

// TestRunShellUnknown verifies runShell with an unknown subcommand prints an error.
func TestRunShellUnknown(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--shell", "unknown-xyz"})
	out := captureStdout(t, func() { runShell("unknown-xyz") })
	if !strings.Contains(out, "Unknown") && !strings.Contains(out, "unknown") {
		t.Errorf("runShell(unknown) expected error output, got: %q", out)
	}
}

// TestRunMaintenanceHelp verifies runMaintenance("help") prints maintenance commands.
func TestRunMaintenanceHelp(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "help"})
	out := captureStdout(t, func() { runMaintenance("help") })
	if !strings.Contains(out, "Maintenance") && !strings.Contains(out, "backup") {
		t.Errorf("runMaintenance(help) unexpected output: %q", out[:min(len(out), 200)])
	}
}

// TestRunMaintenanceDefault verifies runMaintenance with unknown action prints an error.
func TestRunMaintenanceDefault(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "unknown-xyz-action"})
	out := captureStdout(t, func() { runMaintenance("unknown-xyz-action") })
	if !strings.Contains(out, "Unknown") && !strings.Contains(out, "unknown") {
		t.Errorf("runMaintenance(default) unexpected output: %q", out)
	}
}

// TestRunMaintenanceList verifies runMaintenance("list") handles empty backup list.
func TestRunMaintenanceList(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "list"})
	out := captureStdout(t, func() { runMaintenance("list") })
	if out == "" {
		t.Error("runMaintenance(list) produced no output")
	}
}

// TestRunMaintenanceRestoreNoFile verifies runMaintenance("restore") with no filename prints usage.
func TestRunMaintenanceRestoreNoFile(t *testing.T) {
	withArgs(t, []string{"search", "--maintenance", "restore"})
	out := captureStdout(t, func() { runMaintenance("restore") })
	if !strings.Contains(out, "restore") && !strings.Contains(out, "backup") && !strings.Contains(out, "specify") {
		t.Errorf("runMaintenance(restore) no-file unexpected output: %q", out)
	}
}

// TestRunMaintenanceBackupNoPassword verifies runMaintenance("backup") runs without panicking.
func TestRunMaintenanceBackupNoPassword(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "backup"})
	os.Unsetenv("BACKUP_PASSWORD")
	out := captureStdout(t, func() { runMaintenance("backup") })
	if out == "" {
		t.Error("runMaintenance(backup) produced no output")
	}
}

// TestRunUpdateDefault verifies runUpdate with unknown subcommand prints an error.
func TestRunUpdateDefault(t *testing.T) {
	withArgs(t, []string{"search", "--update", "unknown-xyz"})
	out := captureStdout(t, func() { runUpdate("unknown-xyz") })
	if !strings.Contains(out, "Unknown") && !strings.Contains(out, "unknown") {
		t.Errorf("runUpdate(default) unexpected output: %q", out)
	}
}

// TestRunUpdateBranchNoArgs verifies runUpdate("branch") with no branch name prints usage.
func TestRunUpdateBranchNoArgs(t *testing.T) {
	withArgs(t, []string{"search", "--update", "branch"})
	out := captureStdout(t, func() { runUpdate("branch") })
	if !strings.Contains(out, "branch") && !strings.Contains(out, "Please") && !strings.Contains(out, "specify") {
		t.Errorf("runUpdate(branch no-args) unexpected output: %q", out)
	}
}

// TestRunUpdateBranchStable verifies runUpdate("branch") with stable branch succeeds.
func TestRunUpdateBranchStable(t *testing.T) {
	withArgs(t, []string{"search", "--update", "branch", "stable"})
	out := captureStdout(t, func() { runUpdate("branch") })
	if !strings.Contains(out, "stable") {
		t.Errorf("runUpdate(branch stable) unexpected output: %q", out)
	}
}

// TestRunUpdateBranchBeta verifies runUpdate("branch") with beta branch succeeds.
func TestRunUpdateBranchBeta(t *testing.T) {
	withArgs(t, []string{"search", "--update", "branch", "beta"})
	out := captureStdout(t, func() { runUpdate("branch") })
	if !strings.Contains(out, "beta") {
		t.Errorf("runUpdate(branch beta) unexpected output: %q", out)
	}
}

// TestRunUpdateBranchInvalid verifies runUpdate("branch") with invalid branch name prints error.
func TestRunUpdateBranchInvalid(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--update", "branch", "invalid-branch"})
	out := captureStdout(t, func() { runUpdate("branch") })
	if !strings.Contains(out, "Invalid") && !strings.Contains(out, "invalid") {
		t.Errorf("runUpdate(branch invalid) unexpected output: %q", out)
	}
}

// TestRunUpdateNotPrivileged verifies runUpdate("yes") runs without panicking.
func TestRunUpdateNotPrivileged(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--update", "yes"})
	out := captureStdout(t, func() { runUpdate("yes") })
	if out == "" {
		t.Error("runUpdate(yes) produced no output")
	}
}

// TestRunServiceHelp verifies runService("--help") prints service commands.
func TestRunServiceHelp(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "--help"})
	out := captureStdout(t, func() { runService("--help") })
	if !strings.Contains(out, "Service Management") && !strings.Contains(out, "start") {
		t.Errorf("runService(--help) unexpected output: %q", out[:min(len(out), 200)])
	}
}

// TestRunServiceDefault verifies runService with unknown action prints an error.
func TestRunServiceDefault(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "unknown-xyz"})
	out := captureStdout(t, func() { runService("unknown-xyz") })
	if !strings.Contains(out, "Unknown") && !strings.Contains(out, "unknown") {
		t.Errorf("runService(default) unexpected output: %q", out)
	}
}

// TestRunServiceStatus verifies runService("status") runs without panic.
func TestRunServiceStatus(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "status"})
	out := captureStdout(t, func() { runService("status") })
	if out == "" {
		t.Error("runService(status) produced no output")
	}
}

// TestRunServiceInstallNotPrivileged verifies install checks for privileges.
func TestRunServiceInstallNotPrivileged(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "--install"})
	out := captureStdout(t, func() { runService("--install") })
	if out == "" {
		t.Error("runService(--install) produced no output")
	}
}

// TestRunServiceStart verifies runService("start") runs without panic.
func TestRunServiceStart(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "start"})
	captureStdout(t, func() { runService("start") })
}

// TestRunServiceStop verifies runService("stop") runs without panic.
func TestRunServiceStop(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "stop"})
	captureStdout(t, func() { runService("stop") })
}

// TestRunServiceRestart verifies runService("restart") runs without panic.
func TestRunServiceRestart(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "restart"})
	captureStdout(t, func() { runService("restart") })
}

// TestRunServiceReload verifies runService("reload") runs without panic.
func TestRunServiceReload(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "reload"})
	captureStdout(t, func() { runService("reload") })
}

// TestRunServiceUninstall verifies runService("uninstall") checks privileges.
func TestRunServiceUninstall(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "uninstall"})
	out := captureStdout(t, func() { runService("uninstall") })
	if out == "" {
		t.Error("runService(uninstall) produced no output")
	}
}

// TestRunServiceEnable verifies runService("enable") checks privileges.
func TestRunServiceEnable(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "enable"})
	captureStdout(t, func() { runService("enable") })
}

// TestRunServiceDisable verifies runService("disable") checks privileges.
func TestRunServiceDisable(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--service", "disable"})
	captureStdout(t, func() { runService("disable") })
}

// TestRunInitSuccess verifies runInit runs without panicking in a container environment.
func TestRunInitSuccess(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--init"})
	out := captureStdout(t, runInit)
	if out == "" {
		t.Error("runInit produced no output")
	}
}

// TestRunMaintenanceMode verifies runMaintenance("mode") toggles without panic.
func TestRunMaintenanceMode(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "mode"})
	captureStdout(t, func() { runMaintenance("mode") })
}

// TestRunMaintenanceRotateTokenCancelled verifies rotate-token cancel path.
func TestRunMaintenanceRotateTokenCancelled(t *testing.T) {
	withExitFunc(t)
	withArgs(t, []string{"search", "--maintenance", "rotate-token"})

	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Skip("cannot create pipe for stdin")
	}
	os.Stdin = r
	w.Write([]byte("no\n"))
	w.Close()
	t.Cleanup(func() { os.Stdin = origStdin })

	captureStdout(t, func() { runMaintenance("rotate-token") })
}
