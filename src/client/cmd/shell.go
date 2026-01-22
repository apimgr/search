// Package cmd implements CLI commands for the search client
// Per AI.md PART 36: Shell completion support
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Shell integration commands",
	Long:  `Shell integration for completions and init scripts.`,
}

var completionsCmd = &cobra.Command{
	Use:   "completions [bash|zsh|fish|powershell]",
	Short: "Generate shell completions",
	Long: `Generate shell completion script for the specified shell.
If no shell is specified, auto-detects from $SHELL environment variable.

Examples:
  # Auto-detect shell
  ` + getBinaryName() + ` shell completions > ~/.local/share/bash-completion/completions/` + getBinaryName() + `

  # Specific shell
  ` + getBinaryName() + ` shell completions bash > ~/.local/share/bash-completion/completions/` + getBinaryName() + `
  ` + getBinaryName() + ` shell completions zsh > ~/.zsh/completions/_` + getBinaryName() + `
  ` + getBinaryName() + ` shell completions fish > ~/.config/fish/completions/` + getBinaryName() + `.fish`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell", "pwsh"},
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := detectShell()
		if len(args) > 0 {
			shell = args[0]
		}
		return printCompletions(shell)
	},
}

var initCmd = &cobra.Command{
	Use:   "init [bash|zsh|fish|powershell]",
	Short: "Generate shell init command",
	Long: `Generate shell init command for eval.
If no shell is specified, auto-detects from $SHELL environment variable.

Add to your shell rc file:
  eval "$(` + getBinaryName() + ` shell init)"`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"bash", "zsh", "fish", "powershell", "pwsh"},
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := detectShell()
		if len(args) > 0 {
			shell = args[0]
		}
		return printInit(shell)
	},
}

func init() {
	shellCmd.AddCommand(completionsCmd)
	shellCmd.AddCommand(initCmd)
	rootCmd.AddCommand(shellCmd)
}

// detectShell auto-detects shell from $SHELL environment variable
// Per AI.md PART 36 line 43135-43141
func detectShell() string {
	shellPath := os.Getenv("SHELL")
	if shellPath == "" {
		// Default fallback
		return "bash"
	}
	// Handle both Unix forward slash and Windows backslash separators
	base := filepath.Base(shellPath)
	if idx := strings.LastIndex(base, "\\"); idx >= 0 {
		base = base[idx+1:]
	}
	return base
}

// printCompletions generates and prints shell completion script
// Per AI.md PART 36 line 43143-43159
func printCompletions(shell string) error {
	binaryName := getBinaryName()

	switch shell {
	case "bash":
		return rootCmd.GenBashCompletionV2(os.Stdout, true)
	case "zsh":
		return rootCmd.GenZshCompletion(os.Stdout)
	case "fish":
		return rootCmd.GenFishCompletion(os.Stdout, true)
	case "powershell", "pwsh":
		return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
	case "sh", "dash", "ksh":
		// Basic POSIX completions
		fmt.Printf("# POSIX shell completions for %s\n", binaryName)
		fmt.Printf("# Limited completion support for %s\n", shell)
		return nil
	default:
		return fmt.Errorf("unsupported shell: %s\nSupported: bash, zsh, fish, powershell", shell)
	}
}

// printInit generates shell init command for eval
// Per AI.md PART 36 line 43161-43176
func printInit(shell string) error {
	binaryName := getBinaryName()

	switch shell {
	case "bash":
		fmt.Printf("source <(%s shell completions bash)\n", binaryName)
	case "zsh":
		fmt.Printf("source <(%s shell completions zsh)\n", binaryName)
	case "fish":
		fmt.Printf("%s shell completions fish | source\n", binaryName)
	case "sh", "dash", "ksh":
		fmt.Printf("eval \"$(%s shell completions %s)\"\n", binaryName, shell)
	case "powershell", "pwsh":
		fmt.Printf("Invoke-Expression (& %s shell completions powershell)\n", binaryName)
	default:
		return fmt.Errorf("unsupported shell: %s\nSupported: bash, zsh, fish, powershell", shell)
	}
	return nil
}

// handleShellFlag processes --shell flag when used as a flag instead of subcommand
// This provides backwards compatibility with the --shell completions|init pattern
func handleShellFlag(args []string) bool {
	if len(args) < 2 {
		return false
	}

	// Check for --shell flag pattern
	for i, arg := range args {
		if arg == "--shell" && i+1 < len(args) {
			subCmd := args[i+1]
			shell := ""
			if i+2 < len(args) && !strings.HasPrefix(args[i+2], "-") {
				shell = args[i+2]
			} else {
				shell = detectShell()
			}

			switch subCmd {
			case "completions":
				if err := printCompletions(shell); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			case "init":
				if err := printInit(shell); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				os.Exit(0)
			case "--help":
				fmt.Println("Shell integration commands:")
				fmt.Println("")
				fmt.Println("  completions [SHELL]  Generate shell completions")
				fmt.Println("  init [SHELL]         Generate shell init command")
				fmt.Println("")
				fmt.Println("Supported shells: bash, zsh, fish, powershell")
				fmt.Println("")
				fmt.Println("Examples:")
				fmt.Printf("  %s --shell completions bash > ~/.local/share/bash-completion/completions/%s\n", getBinaryName(), getBinaryName())
				fmt.Printf("  eval \"$(%s --shell init)\"\n", getBinaryName())
				os.Exit(0)
			}
			return true
		}
	}
	return false
}
