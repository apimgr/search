// Package cmd implements CLI commands for the search client
// Per AI.md PART 36: --status command for health check
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check server status and health",
	Long: `Check server status and health.
Exits with code 0 if healthy, 1 if unhealthy.

Examples:
  ` + getBinaryName() + ` status
  ` + getBinaryName() + ` status --output json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runStatus()
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus() error {
	// Initialize client
	if err := initClient(); err != nil {
		return err
	}

	// Measure response time
	start := time.Now()
	health, err := apiClient.Health()
	elapsed := time.Since(start)

	if err != nil {
		// Output error in requested format
		switch getOutputFormat() {
		case "json":
			resp := map[string]interface{}{
				"status":        "error",
				"error":         err.Error(),
				"response_time": elapsed.Milliseconds(),
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(resp)
		default:
			fmt.Printf("Status: ERROR\n")
			fmt.Printf("Error: %v\n", err)
			fmt.Printf("Response time: %dms\n", elapsed.Milliseconds())
		}
		os.Exit(1)
		return nil
	}

	// Output health in requested format
	switch getOutputFormat() {
	case "json":
		resp := map[string]interface{}{
			"status":        health.Status,
			"version":       health.Version,
			"uptime":        health.Uptime,
			"response_time": elapsed.Milliseconds(),
			"checks":        health.Checks,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(resp)
	default:
		fmt.Printf("Status: %s\n", health.Status)
		if health.Version != "" {
			fmt.Printf("Version: %s\n", health.Version)
		}
		if health.Uptime != "" {
			fmt.Printf("Uptime: %s\n", health.Uptime)
		}
		fmt.Printf("Response time: %dms\n", elapsed.Milliseconds())
		if len(health.Checks) > 0 {
			fmt.Println("\nHealth checks:")
			for name, status := range health.Checks {
				fmt.Printf("  %s: %s\n", name, status)
			}
		}
	}

	// Exit code based on status
	if health.Status != "ok" && health.Status != "healthy" {
		os.Exit(1)
	}
	return nil
}
