// Package cmd implements CLI commands for the search client
// Per AI.md PART 32: Shell completion support
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// shellCmd is the "shell" subcommand handling completions and init scripts.
var shellCmd = newShellCommand()

// newShellCommand constructs the shell subcommand. It dispatches to its own
// "completions" and "init" sub-actions based on the first positional argument,
// preserving the previous cobra command tree behavior.
func newShellCommand() *command {
	return &command{
		Use:   "shell [completions|init] [SHELL]",
		Short: "Shell integration commands",
		Long:  `Shell integration for completions and init scripts.`,
		run: func(args []string) error {
			if len(args) == 0 {
				printShellHelp()
				return nil
			}
			action := args[0]
			shell := detectShell()
			if len(args) > 1 {
				shell = args[1]
			}
			switch action {
			case "completions":
				return printCompletions(shell)
			case "init":
				return printInit(shell)
			case "--help", "-h", "help":
				printShellHelp()
				return nil
			default:
				return fmt.Errorf("unknown shell action: %s\nSupported: completions, init", action)
			}
		},
	}
}

func init() {
	rootCmd.addCommand("shell", shellCmd)
}

// printShellHelp prints usage for the shell subcommand.
func printShellHelp() {
	fmt.Println("Shell integration commands:")
	fmt.Println("")
	fmt.Println("  completions [SHELL]  Generate shell completions")
	fmt.Println("  init [SHELL]         Generate shell init command")
	fmt.Println("")
	fmt.Println("Supported shells: bash, zsh, fish, sh, dash, ksh, powershell, pwsh")
	fmt.Println("")
	fmt.Println("Examples:")
	fmt.Printf("  %s --shell completions bash > ~/.local/share/bash-completion/completions/%s\n", getBinaryName(), getBinaryName())
	fmt.Printf("  eval \"$(%s --shell init)\"\n", getBinaryName())
}

// detectShell auto-detects shell from $SHELL environment variable
// Per AI.md PART 32 line 43135-43141
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
	return strings.TrimSuffix(strings.ToLower(base), ".exe")
}

// printCompletions generates and prints a shell completion script.
// The scripts are hand-written (replacing cobra's generators) and cover the
// subcommands and flags exposed by the CLI.
// Per AI.md PART 32 line 43143-43159
func printCompletions(shell string) error {
	binaryName := getBinaryName()

	switch shell {
	case "bash":
		fmt.Print(bashCompletion(binaryName))
		return nil
	case "zsh":
		fmt.Print(zshCompletion(binaryName))
		return nil
	case "fish":
		fmt.Print(fishCompletion(binaryName))
		return nil
	case "powershell", "pwsh":
		fmt.Print(powershellCompletion(binaryName))
		return nil
	case "sh", "dash", "ksh":
		fmt.Print(posixCompletion(binaryName, shell))
		return nil
	default:
		return fmt.Errorf("unsupported shell: %s\nSupported: bash, zsh, fish, sh, dash, ksh, powershell, pwsh", shell)
	}
}

// completionFlags lists the flags offered for completion (without leading --).
var completionFlags = []string{
	"config", "server", "token", "token-file", "user", "output",
	"color", "shell", "lang", "timeout", "debug", "page", "limit",
	"maintenance", "update", "help", "version",
}

// completionSubcommands lists the subcommands offered for completion.
var completionSubcommands = []string{"alerts", "answer", "autocomplete", "autodiscover", "bangs", "categories", "engines", "health", "info", "instant", "metrics", "preferences", "scheduler", "search", "server-config", "shell", "widget", "widgets"}

// bashCompletion returns a hand-written bash completion script.
func bashCompletion(bin string) string {
	flags := ""
	for _, f := range completionFlags {
		flags += "--" + f + " "
	}
	subs := strings.Join(completionSubcommands, " ")
	return fmt.Sprintf(`# bash completion for %[1]s
_%[1]s_completions() {
    local cur prev
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    local subcommands="%[2]s"
    local flags="%[3]s"
    if [[ "$cur" == -* ]]; then
        COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
        return 0
    fi
    if [[ "$prev" == "--output" ]]; then
        COMPREPLY=( $(compgen -W "json table plain" -- "$cur") )
        return 0
    fi
    if [[ "$prev" == "--color" ]]; then
        COMPREPLY=( $(compgen -W "auto yes no" -- "$cur") )
        return 0
    fi
    if [[ "$prev" == "--update" ]]; then
        COMPREPLY=( $(compgen -W "check yes" -- "$cur") )
        return 0
    fi
    if [[ "$prev" == "--shell" ]]; then
        COMPREPLY=( $(compgen -W "completions init --help" -- "$cur") )
        return 0
    fi
    if [[ "${COMP_WORDS[COMP_CWORD-2]}" == "--shell" && ( "$prev" == "completions" || "$prev" == "init" ) ]]; then
        COMPREPLY=( $(compgen -W "bash zsh fish sh dash ksh powershell pwsh" -- "$cur") )
        return 0
    fi
    COMPREPLY=( $(compgen -W "$subcommands $flags" -- "$cur") )
    return 0
}
complete -F _%[1]s_completions %[1]s
`, bin, subs, strings.TrimSpace(flags))
}

// zshCompletion returns a hand-written zsh completion script.
func zshCompletion(bin string) string {
	flags := ""
	for _, f := range completionFlags {
		if f == "update" {
			continue
		}
		flags += "'--" + f + "' "
	}
	subs := strings.Join(completionSubcommands, " ")
	return fmt.Sprintf(`#compdef %[1]s
# zsh completion for %[1]s
_%[1]s() {
    local -a subcommands flags
    subcommands=(%[2]s)
    flags=(%[3]s)
    _arguments \
        '1: :->cmds' \
        '--update[check for or install a client update]:action:(check yes)' \
        '--shell[shell integration]:action:(completions init --help)' \
        '*: :->args'
    case $state in
        cmds)
            _describe 'command' subcommands
            compadd -- $flags
            ;;
        *)
            compadd -- $flags
            ;;
    esac
}
compdef _%[1]s %[1]s
`, bin, subs, strings.TrimSpace(flags))
}

// fishCompletion returns a hand-written fish completion script.
func fishCompletion(bin string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# fish completion for %s\n", bin)
	for _, s := range completionSubcommands {
		fmt.Fprintf(&b, "complete -c %s -f -n '__fish_use_subcommand' -a '%s'\n", bin, s)
	}
	for _, f := range completionFlags {
		fmt.Fprintf(&b, "complete -c %s -l %s\n", bin, f)
	}
	fmt.Fprintf(&b, "complete -c %s -n '__fish_seen_argument --update' -a 'check yes'\n", bin)
	fmt.Fprintf(&b, "complete -c %s -n '__fish_seen_argument --shell' -a 'completions init --help'\n", bin)
	return b.String()
}

// posixCompletion emits a portable candidate function for shells without a
// standardized programmable-completion interface.
func posixCompletion(bin, shell string) string {
	items := append([]string{}, completionSubcommands...)
	for _, flag := range completionFlags {
		items = append(items, "--"+flag)
	}
	items = append(items, "check", "yes")
	fn := completionFunctionName(bin)
	return fmt.Sprintf(`# POSIX completion candidates for %[1]s (%[3]s)
# Call %[2]s PREFIX [PREVIOUS-ARGUMENT] to list matching commands and flags.
%[2]s() {
    _completion_prefix=${1-}
    _completion_previous=${2-}
    case "$_completion_previous" in
        --update) _completion_items='check yes' ;;
        --shell) _completion_items='completions init --help' ;;
        *) _completion_items='%[4]s' ;;
    esac
    for _completion_item in $_completion_items; do
        case "$_completion_item" in
            "$_completion_prefix"*) printf '%%s\\n' "$_completion_item" ;;
        esac
    done
}
`, bin, fn, shell, strings.Join(items, " "))
}

func completionFunctionName(bin string) string {
	var name strings.Builder
	name.WriteByte('_')
	for _, char := range strings.ToLower(bin) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			name.WriteRune(char)
		} else {
			name.WriteByte('_')
		}
	}
	name.WriteString("_completion_candidates")
	return name.String()
}

// powershellCompletion returns a hand-written PowerShell completion script.
func powershellCompletion(bin string) string {
	items := append([]string{}, completionSubcommands...)
	for _, f := range completionFlags {
		items = append(items, "--"+f)
	}
	var quoted []string
	for _, it := range items {
		quoted = append(quoted, "'"+it+"'")
	}
	return fmt.Sprintf(`# PowerShell completion for %[1]s
Register-ArgumentCompleter -Native -CommandName %[1]s -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $items = @(%[2]s)
    if ($commandAst.ToString() -match '--update\s+\S*$') {
        $items = @('check', 'yes')
    } elseif ($commandAst.ToString() -match '--shell\s+\S*$') {
        $items = @('completions', 'init', '--help')
    }
    $items | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`, bin, strings.Join(quoted, ", "))
}

// printInit generates shell init command for eval
// Per AI.md PART 32 line 43161-43176
func printInit(shell string) error {
	binaryName := getBinaryName()

	switch shell {
	case "bash":
		fmt.Printf("source <(%s --shell completions bash)\n", binaryName)
	case "zsh":
		fmt.Printf("source <(%s --shell completions zsh)\n", binaryName)
	case "fish":
		fmt.Printf("%s --shell completions fish | source\n", binaryName)
	case "sh", "dash", "ksh":
		fmt.Printf("eval \"$(%s --shell completions %s)\"\n", binaryName, shell)
	case "powershell", "pwsh":
		fmt.Printf("Invoke-Expression (& %s --shell completions powershell)\n", binaryName)
	default:
		return fmt.Errorf("unsupported shell: %s\nSupported: bash, zsh, fish, sh, dash, ksh, powershell, pwsh", shell)
	}
	return nil
}

// HandleShellFlag processes the --shell flag when used as a flag instead of a
// subcommand. Called from main() before the main parse to support the
// "--shell completions|init" pattern per PART 8.
// Returns (true, nil) on success, (true, err) on failure, (false, nil) when not handled.
// The caller in main() is responsible for calling os.Exit based on the returned values.
func HandleShellFlag(args []string) (bool, error) {
	if len(args) < 2 {
		return false, nil
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
				return true, printCompletions(shell)
			case "init":
				return true, printInit(shell)
			case "--help":
				printShellHelp()
				return true, nil
			}
			return true, nil
		}
	}
	return false, nil
}
