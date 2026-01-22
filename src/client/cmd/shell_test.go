package cmd

import (
	"bytes"
	"os"
	"testing"
)

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
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printCompletions("bash")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printCompletions('bash') error = %v", err)
	}

	output := buf.String()
	// Bash completion should contain certain keywords
	if len(output) == 0 {
		t.Error("printCompletions('bash') produced no output")
	}
}

func TestPrintCompletionsZsh(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printCompletions("zsh")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printCompletions('zsh') error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("printCompletions('zsh') produced no output")
	}
}

func TestPrintCompletionsFish(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printCompletions("fish")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printCompletions('fish') error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("printCompletions('fish') produced no output")
	}
}

func TestPrintCompletionsPowershell(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printCompletions("powershell")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printCompletions('powershell') error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("printCompletions('powershell') produced no output")
	}
}

func TestPrintCompletionsPwsh(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printCompletions("pwsh")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printCompletions('pwsh') error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("printCompletions('pwsh') produced no output")
	}
}

func TestPrintCompletionsPosixShells(t *testing.T) {
	shells := []string{"sh", "dash", "ksh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := printCompletions(shell)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			buf.ReadFrom(r)

			if err != nil {
				t.Fatalf("printCompletions(%q) error = %v", shell, err)
			}

			// POSIX shells should output a comment
			output := buf.String()
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
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printInit("bash")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printInit('bash') error = %v", err)
	}

	output := buf.String()
	if len(output) == 0 {
		t.Error("printInit('bash') produced no output")
	}
}

func TestPrintInitZsh(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printInit("zsh")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printInit('zsh') error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("printInit('zsh') produced no output")
	}
}

func TestPrintInitFish(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printInit("fish")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printInit('fish') error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("printInit('fish') produced no output")
	}
}

func TestPrintInitPosixShells(t *testing.T) {
	shells := []string{"sh", "dash", "ksh"}

	for _, shell := range shells {
		t.Run(shell, func(t *testing.T) {
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := printInit(shell)

			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			buf.ReadFrom(r)

			if err != nil {
				t.Fatalf("printInit(%q) error = %v", shell, err)
			}

			if buf.Len() == 0 {
				t.Errorf("printInit(%q) produced no output", shell)
			}
		})
	}
}

func TestPrintInitPowershell(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printInit("powershell")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printInit('powershell') error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("printInit('powershell') produced no output")
	}
}

func TestPrintInitPwsh(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := printInit("pwsh")

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("printInit('pwsh') error = %v", err)
	}

	if buf.Len() == 0 {
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

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := completionsCmd.RunE(completionsCmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("completionsCmd.RunE() auto-detect error = %v", err)
	}
}

func TestCompletionsCmdRunEWithArg(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := completionsCmd.RunE(completionsCmd, []string{"zsh"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("completionsCmd.RunE(['zsh']) error = %v", err)
	}
}

// Tests for initCmd.RunE

func TestInitCmdRunEAutoDetect(t *testing.T) {
	os.Setenv("SHELL", "/bin/bash")
	defer os.Unsetenv("SHELL")

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := initCmd.RunE(initCmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("initCmd.RunE() auto-detect error = %v", err)
	}
}

func TestInitCmdRunEWithArg(t *testing.T) {
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := initCmd.RunE(initCmd, []string{"fish"})

	w.Close()
	os.Stdout = oldStdout

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
