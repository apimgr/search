// Package main provides CLI initialization functions
// Per AI.md PART 32: CLI initialization order (NON-NEGOTIABLE)
package main

import (
	"fmt"
	"os"

	"github.com/apimgr/search/src/client/path"
	"github.com/apimgr/search/src/client/surface"
)

// InitCLI initializes the CLI environment.
// Per AI.md PART 32: CLI Startup Sequence (NON-NEGOTIABLE)
// 1. Ensure directories exist
// 2. Set correct permissions
// 3. Initialize logging (with rotation)
// 4. Initialize cache
func InitCLI() error {
	// Step 1: Create directories with correct permissions
	if err := path.EnsureDirs(); err != nil {
		return fmt.Errorf("%s: %w", surface.Text("client.init.directories_failed"), err)
	}

	// Step 2: Initialize logging with configuration
	// Per AI.md PART 32 lines 42749-42755: Comprehensive logging
	if err := InitLogging(); err != nil {
		// Non-fatal - log to stderr if file fails
		fmt.Fprintln(os.Stderr, surface.Text("client.init.log_warning", err))
	}

	// Step 3: Initialize cache
	// Per AI.md PART 32 lines 42756-42760: Cache configuration
	if err := InitCache(); err != nil {
		// Non-fatal - cache is optional
		LogWarn("could not initialize cache", "error", err)
	}

	return nil
}
