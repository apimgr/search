package main

import (
	"errors"
	"fmt"
	"os"

	cmd "github.com/apimgr/search/src/client/command"
	clientupdate "github.com/apimgr/search/src/client/update"
)

func main() {
	// Per AI.md PART 32: CLI Startup Sequence (NON-NEGOTIABLE)
	// Initialize CLI environment before executing commands
	if err := InitCLI(); err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", cmd.LocalizedCLIText("Error:"), err)
		os.Exit(1)
	}

	// Per AI.md PART 8: --shell flag handled before cobra parses, same as server binary
	if handled, err := cmd.HandleShellFlag(os.Args); handled {
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", cmd.LocalizedCLIText("shell completion error:"), err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := cmd.ExecuteClientCLI(); err != nil {
		if errors.Is(err, clientupdate.ErrRestartStarted) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "%s %v\n", cmd.LocalizedCLIText("Error:"), err)
		os.Exit(cmd.ExitCodeForError(err))
	}
}
