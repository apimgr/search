// Package cmd implements CLI commands for the search client
// Per AI.md PART 32: --status command for health check
package cmd

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrUnhealthy is returned by runStatus when the server reports an unhealthy
// state or the health endpoint cannot be reached. main() converts this sentinel
// into os.Exit(1) so that os.Exit never appears outside of main().
var ErrUnhealthy = errors.New("server is unhealthy")

// statusCmd is the "status" subcommand, registered on the root command.
var statusCmd = newStatusCommand()

// newStatusCommand constructs the status subcommand using the stdlib-flag
// command tree, preserving the previous cobra metadata and behavior.
func newStatusCommand() *command {
	return &command{
		Use:   "status",
		Short: "Check server status and health",
		Long: `Check server status and health.
Exits with code 0 if healthy, 1 if unhealthy.

Examples:
  ` + getBinaryName() + ` status
  ` + getBinaryName() + ` status --output json`,
		run: func(args []string) error {
			return runStatus()
		},
	}
}

func init() {
	rootCmd.addCommand("status", statusCmd)
}

// statusView converts the status payload into the format-agnostic view used by
// every output format. Nested check maps are flattened into "check.NAME" rows
// so the table and csv formats stay one value per row.
func statusView(data map[string]interface{}) view {
	keys := make([]string, 0, len(data))
	for k := range data {
		if k != "checks" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	rows := make([][]string, 0, len(data))
	for _, k := range keys {
		rows = append(rows, []string{k, fmt.Sprintf("%v", data[k])})
	}
	if checks, ok := data["checks"].(map[string]interface{}); ok {
		flattenHealthFields(&rows, "check", checks)
	}

	plain := make([]string, 0, len(rows))
	for _, r := range rows {
		plain = append(plain, r[0]+"="+r[1])
	}
	return view{
		Data:    data,
		Headers: []string{"FIELD", "VALUE"},
		Rows:    rows,
		Plain:   plain,
	}
}

func flattenHealthFields(rows *[][]string, prefix string, values map[string]interface{}) {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		key := prefix + "." + name
		if nested, ok := values[name].(map[string]interface{}); ok {
			flattenHealthFields(rows, key, nested)
			continue
		}
		value := values[name]
		if enabled, ok := value.(bool); ok {
			value = boolCell(enabled)
		}
		*rows = append(*rows, []string{key, fmt.Sprintf("%v", value)})
	}
}

func runStatus() error {
	// Initialize client if not already done
	if apiClient == nil {
		if err := initClient(); err != nil {
			return err
		}
	}

	// Measure response time
	start := time.Now()
	health, err := apiClient.Health()
	elapsed := time.Since(start)

	if err != nil {
		renderErr := render(getOutputFormat(), statusView(map[string]interface{}{
			"status":        "error",
			"error":         err.Error(),
			"response_time": elapsed.Milliseconds(),
		}))
		if renderErr != nil {
			return renderErr
		}
		return ErrUnhealthy
	}

	data := map[string]interface{}{
		"status":        health.Status,
		"version":       health.Version,
		"uptime":        health.Uptime,
		"response_time": elapsed.Milliseconds(),
		"checks":        health.Checks,
	}
	if err := render(getOutputFormat(), statusView(data)); err != nil {
		return err
	}

	// Return ErrUnhealthy when status is not ok/healthy so main() can call os.Exit(1)
	if health.Status != "ok" && health.Status != "healthy" {
		return ErrUnhealthy
	}
	return nil
}
