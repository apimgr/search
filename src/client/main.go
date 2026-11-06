package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/apimgr/search/src/client/cmd"
	clientupdate "github.com/apimgr/search/src/client/update"
)

func main() {
	// Per AI.md PART 32: CLI Startup Sequence (NON-NEGOTIABLE)
	// Initialize CLI environment before executing commands
	if err := InitCLI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Per AI.md PART 8: --shell flag handled before cobra parses, same as server binary
	if handled, err := cmd.HandleShellFlag(os.Args); handled {
		if err != nil {
			fmt.Fprintf(os.Stderr, "shell completion error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if err := cmd.ExecuteClientCLI(); err != nil {
		if errors.Is(err, clientupdate.ErrRestartStarted) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(cmd.ExitCodeForError(err))
	}
}
