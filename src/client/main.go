package main

import (
	"fmt"
	"os"

	"github.com/apimgr/search/src/client/cmd"
)

func main() {
	// Per AI.md PART 36: CLI Startup Sequence (NON-NEGOTIABLE)
	// Initialize CLI environment before executing commands
	if err := InitCLI(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
