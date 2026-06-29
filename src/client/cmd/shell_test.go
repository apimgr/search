package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// captureStdout captures stdout during function execution
// Uses a goroutine to read concurrently to avoid pipe buffer deadlock
func captureStdout(fn func() error) (string, error) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w

	// Read from pipe concurrently to avoid buffer deadlock
	outputChan := make(chan string)
	go func() {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		outputChan <- buf.String()
	}()

	// Run the function
	fnErr := fn()

	// Restore stdout and close writer to signal EOF to reader
	w.Close()
	os.Stdout = oldStdout

	// Wait for reader goroutine to finish
	output := <-outputChan
	r.Close()

	return output, fnErr
}

// Tests for detectShell

func TestDetectShellFromEnv(t *testing.T) {
	tests := []struct {
		shellEnv string
		expected string
	}{
		{"/bin/bash", "bash"},
		{"/bin/zsh", "zsh"},
		{"/usr/bin/fish", "fish"},
		{"/bin/sh", "sh"},
		{"/usr/local/bin/zsh", "zsh"},
		{"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe", "powershell.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.shellEnv, func(t *testing.T) {
			os.Setenv("SHELL", tt.shellEnv)
			defer os.Unsetenv("SHELL")

			result := detectShell()
			if result != tt.expected {
				t.Errorf("detectShell() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestDetectShellNoEnv(t *testing.T) {
	os.Unsetenv("SHELL")

	result := detectShell()

	if result != "bash" {
		t.Errorf("detectShell() = %q, want 'bash' as default", result)
	}
}

func TestDetectShellEmptyEnv(t *testing.T) {
	os.Setenv("SHELL", "")
	defer os.Unsetenv("SHELL")

	result := detectShell()

	if result != "bash" {
		t.Errorf("detectShell() = %q, want 'bash' as default", result)
	}
}

// Tests for printCompletions

func TestPrintCompletionsBash(t *testing.T) {
	output, err := captureStdout(func() error {
		return printCompletions("bash")
	})

	if err != nil {
		t.Fatalf("printCompletions('bash') error = %v", err)
	}

	// Bash completion should contain certain keywords
	if len(output) == 0 {
		t.Error("printCompletions('bash') produced no output")
	}
}

func TestPrintCompletionsZsh(t *testing.T) {
	output, err := captureStdout(func() error {
		return printCompletions("zsh")
	})

	if err != nil {
		t.Fatalf("printCompletions('zsh') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printCompletions('zsh') produced no output")
	}
}

func TestPrintCompletionsFish(t *testing.T) {
	output, err := captureStdout(func() error {
		return printCompletions("fish")
	})

	if err != nil {
		t.Fatalf("printCompletions('fish') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printCompletions('fish') produced no output")
	}
}

func TestPrintCompletionsPowershell(t *testing.T) {
	output, err := captureStdout(func() error {
		return printCompletions("powershell")
	})

	if err != nil {
		t.Fatalf("printCompletions('powershell') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printCompletions('powershell') produced no output")
	}
}

func TestPrintCompletionsPwsh(t *testing.T) {
	output, err := captureStdout(func() error {
		return printCompletions("pwsh")
	})

	if err != nil {
		t.Fatalf("printCompletions('pwsh') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printCompletions('pwsh') produced no output")
	}
}

func TestPrintCompletionsPosixShells(t *testing.T) {
	shells := []string{"sh", "dash", "ksh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			output, err := captureStdout(func() error {
				return printCompletions(shell)
			})

			if err != nil {
				t.Fatalf("printCompletions(%q) error = %v", shell, err)
			}

			// POSIX shells should output a comment
			if len(output) == 0 {
				t.Errorf("printCompletions(%q) produced no output", shell)
			}
		})
	}
}

func TestPrintCompletionsUnsupported(t *testing.T) {
	err := printCompletions("unsupported")

	if err == nil {
		t.Error("printCompletions('unsupported') should return error")
	}
}

// Tests for printInit

func TestPrintInitBash(t *testing.T) {
	output, err := captureStdout(func() error {
		return printInit("bash")
	})

	if err != nil {
		t.Fatalf("printInit('bash') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printInit('bash') produced no output")
	}
}

func TestPrintInitZsh(t *testing.T) {
	output, err := captureStdout(func() error {
		return printInit("zsh")
	})

	if err != nil {
		t.Fatalf("printInit('zsh') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printInit('zsh') produced no output")
	}
}

func TestPrintInitFish(t *testing.T) {
	output, err := captureStdout(func() error {
		return printInit("fish")
	})

	if err != nil {
		t.Fatalf("printInit('fish') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printInit('fish') produced no output")
	}
}

func TestPrintInitPosixShells(t *testing.T) {
	shells := []string{"sh", "dash", "ksh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			output, err := captureStdout(func() error {
				return printInit(shell)
			})

			if err != nil {
				t.Fatalf("printInit(%q) error = %v", shell, err)
			}

			if len(output) == 0 {
				t.Errorf("printInit(%q) produced no output", shell)
			}
		})
	}
}

func TestPrintInitPowershell(t *testing.T) {
	output, err := captureStdout(func() error {
		return printInit("powershell")
	})

	if err != nil {
		t.Fatalf("printInit('powershell') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printInit('powershell') produced no output")
	}
}

func TestPrintInitPwsh(t *testing.T) {
	output, err := captureStdout(func() error {
		return printInit("pwsh")
	})

	if err != nil {
		t.Fatalf("printInit('pwsh') error = %v", err)
	}

	if len(output) == 0 {
		t.Error("printInit('pwsh') produced no output")
	}
}

func TestPrintInitUnsupported(t *testing.T) {
	err := printInit("unsupported")

	if err == nil {
		t.Error("printInit('unsupported') should return error")
	}
}

// Tests for HandleShellFlag

func TestHandleShellFlagNoArgs(t *testing.T) {
	handled, err := HandleShellFlag([]string{})

	if err != nil {
		t.Errorf("HandleShellFlag([]) unexpected error = %v", err)
	}
	if handled {
		t.Error("HandleShellFlag([]) should return false")
	}
}

func TestHandleShellFlagSingleArg(t *testing.T) {
	handled, err := HandleShellFlag([]string{"test"})

	if err != nil {
		t.Errorf("HandleShellFlag(['test']) unexpected error = %v", err)
	}
	if handled {
		t.Error("HandleShellFlag(['test']) should return false")
	}
}

func TestHandleShellFlagNoShellFlag(t *testing.T) {
	handled, err := HandleShellFlag([]string{"test", "command", "arg"})

	if err != nil {
		t.Errorf("HandleShellFlag without --shell unexpected error = %v", err)
	}
	if handled {
		t.Error("HandleShellFlag without --shell should return false")
	}
}

func TestHandleShellFlagWithOtherFlags(t *testing.T) {
	handled, err := HandleShellFlag([]string{"--config", "file.yml", "--server", "url"})

	if err != nil {
		t.Errorf("HandleShellFlag without --shell unexpected error = %v", err)
	}
	if handled {
		t.Error("HandleShellFlag without --shell should return false")
	}
}

// Tests for shellCmd

func TestShellCmdUse(t *testing.T) {
	// The shell command's usage string documents its actions and shells.
	if shellCmd.Use == "" {
		t.Error("shellCmd.Use should not be empty")
	}
}

func TestShellCmdShort(t *testing.T) {
	if shellCmd.Short == "" {
		t.Error("shellCmd.Short should not be empty")
	}
}

// Tests for the "completions" action of the shell command.

func TestCompletionsActionAutoDetect(t *testing.T) {
	os.Setenv("SHELL", "/bin/bash")
	defer os.Unsetenv("SHELL")

	out, err := captureStdout(func() error {
		return shellCmd.run([]string{"completions"})
	})

	if err != nil {
		t.Fatalf("shell completions auto-detect error = %v", err)
	}
	if len(out) == 0 {
		t.Error("shell completions auto-detect produced no output")
	}
}

func TestCompletionsActionWithArg(t *testing.T) {
	out, err := captureStdout(func() error {
		return shellCmd.run([]string{"completions", "zsh"})
	})

	if err != nil {
		t.Fatalf("shell completions zsh error = %v", err)
	}
	if len(out) == 0 {
		t.Error("shell completions zsh produced no output")
	}
}

// Tests for the "init" action of the shell command.

func TestInitActionAutoDetect(t *testing.T) {
	os.Setenv("SHELL", "/bin/bash")
	defer os.Unsetenv("SHELL")

	out, err := captureStdout(func() error {
		return shellCmd.run([]string{"init"})
	})

	if err != nil {
		t.Fatalf("shell init auto-detect error = %v", err)
	}
	if len(out) == 0 {
		t.Error("shell init auto-detect produced no output")
	}
}

func TestInitActionWithArg(t *testing.T) {
	out, err := captureStdout(func() error {
		return shellCmd.run([]string{"init", "fish"})
	})

	if err != nil {
		t.Fatalf("shell init fish error = %v", err)
	}
	if len(out) == 0 {
		t.Error("shell init fish produced no output")
	}
}

// TestShellActionUnknown verifies an unknown action returns an error.
func TestShellActionUnknown(t *testing.T) {
	err := shellCmd.run([]string{"bogus"})
	if err == nil {
		t.Error("shell run with unknown action should return error")
	}
}

// TestShellActionHelp verifies the help action prints usage without error.
func TestShellActionHelp(t *testing.T) {
	out, err := captureStdout(func() error {
		return shellCmd.run([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("shell --help error = %v", err)
	}
	if len(out) == 0 {
		t.Error("shell --help produced no output")
	}
}

// Test shell command is added to root

func TestShellCommandRegistered(t *testing.T) {
	if _, ok := rootCmd.subcommands["shell"]; !ok {
		t.Error("shell command should be registered with rootCmd")
	}
}
