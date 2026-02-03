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

// Tests for handleShellFlag

func TestHandleShellFlagNoArgs(t *testing.T) {
	result := handleShellFlag([]string{})

	if result {
		t.Error("handleShellFlag([]) should return false")
	}
}

func TestHandleShellFlagSingleArg(t *testing.T) {
	result := handleShellFlag([]string{"test"})

	if result {
		t.Error("handleShellFlag(['test']) should return false")
	}
}

func TestHandleShellFlagNoShellFlag(t *testing.T) {
	result := handleShellFlag([]string{"test", "command", "arg"})

	if result {
		t.Error("handleShellFlag without --shell should return false")
	}
}

func TestHandleShellFlagWithOtherFlags(t *testing.T) {
	result := handleShellFlag([]string{"--config", "file.yml", "--server", "url"})

	if result {
		t.Error("handleShellFlag without --shell should return false")
	}
}

// Tests for shellCmd

func TestShellCmdUse(t *testing.T) {
	if shellCmd.Use != "shell" {
		t.Errorf("shellCmd.Use = %q, want 'shell'", shellCmd.Use)
	}
}

func TestShellCmdShort(t *testing.T) {
	if shellCmd.Short == "" {
		t.Error("shellCmd.Short should not be empty")
	}
}

// Tests for completionsCmd

func TestCompletionsCmdUse(t *testing.T) {
	if completionsCmd.Use == "" {
		t.Error("completionsCmd.Use should not be empty")
	}
}

func TestCompletionsCmdValidArgs(t *testing.T) {
	validArgs := completionsCmd.ValidArgs

	if len(validArgs) == 0 {
		t.Error("completionsCmd.ValidArgs should not be empty")
	}

	expectedArgs := []string{"bash", "zsh", "fish", "powershell", "pwsh"}
	for _, expected := range expectedArgs {
		found := false
		for _, arg := range validArgs {
			if arg == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("completionsCmd.ValidArgs should contain %q", expected)
		}
	}
}

// Tests for initCmd

func TestInitCmdUse(t *testing.T) {
	if initCmd.Use == "" {
		t.Error("initCmd.Use should not be empty")
	}
}

func TestInitCmdValidArgs(t *testing.T) {
	validArgs := initCmd.ValidArgs

	if len(validArgs) == 0 {
		t.Error("initCmd.ValidArgs should not be empty")
	}

	expectedArgs := []string{"bash", "zsh", "fish", "powershell", "pwsh"}
	for _, expected := range expectedArgs {
		found := false
		for _, arg := range validArgs {
			if arg == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("initCmd.ValidArgs should contain %q", expected)
		}
	}
}

// Tests for completionsCmd.RunE

func TestCompletionsCmdRunEAutoDetect(t *testing.T) {
	os.Setenv("SHELL", "/bin/bash")
	defer os.Unsetenv("SHELL")

	_, err := captureStdout(func() error {
		return completionsCmd.RunE(completionsCmd, []string{})
	})

	if err != nil {
		t.Fatalf("completionsCmd.RunE() auto-detect error = %v", err)
	}
}

func TestCompletionsCmdRunEWithArg(t *testing.T) {
	_, err := captureStdout(func() error {
		return completionsCmd.RunE(completionsCmd, []string{"zsh"})
	})

	if err != nil {
		t.Fatalf("completionsCmd.RunE(['zsh']) error = %v", err)
	}
}

// Tests for initCmd.RunE

func TestInitCmdRunEAutoDetect(t *testing.T) {
	os.Setenv("SHELL", "/bin/bash")
	defer os.Unsetenv("SHELL")

	_, err := captureStdout(func() error {
		return initCmd.RunE(initCmd, []string{})
	})

	if err != nil {
		t.Fatalf("initCmd.RunE() auto-detect error = %v", err)
	}
}

func TestInitCmdRunEWithArg(t *testing.T) {
	_, err := captureStdout(func() error {
		return initCmd.RunE(initCmd, []string{"fish"})
	})

	if err != nil {
		t.Fatalf("initCmd.RunE(['fish']) error = %v", err)
	}
}

// Test shell command is added to root

func TestShellCommandRegistered(t *testing.T) {
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "shell" {
			found = true
			break
		}
	}

	if !found {
		t.Error("shell command should be registered with rootCmd")
	}
}

// Test completions and init subcommands are registered

func TestShellSubcommandsRegistered(t *testing.T) {
	subCommands := shellCmd.Commands()

	hasCompletions := false
	hasInit := false

	for _, cmd := range subCommands {
		if cmd.Use == "completions [bash|zsh|fish|powershell]" {
			hasCompletions = true
		}
		if cmd.Use == "init [bash|zsh|fish|powershell]" {
			hasInit = true
		}
	}

	if !hasCompletions {
		t.Error("completions subcommand should be registered with shellCmd")
	}
	if !hasInit {
		t.Error("init subcommand should be registered with shellCmd")
	}
}
