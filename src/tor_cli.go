package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
)

// torCLIClient is a short-timeout HTTP client used only to reach the
// server's own loopback-only /server/tor/* control endpoints (AI.md PART 31).
var torCLIClient = &http.Client{Timeout: 10 * time.Second}

// runningServerPort resolves the running server's bind port using the same
// PID-file + config mechanism as --status (AI.md PART 31: "identical to how
// --status locates the running server — no new discovery mechanism").
// Returns (port, true) if a server appears to be running, else (0, false).
func runningServerPort() (int, bool) {
	pidFile := config.GetPIDFile()
	pidData, err := os.ReadFile(pidFile)
	if err != nil {
		return 0, false
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(pidData)), "%d", &pid); err != nil {
		return 0, false
	}
	if !isProcessRunning(pid) {
		return 0, false
	}

	port := 64580
	if cfg, err := config.Load(config.GetConfigPath()); err == nil && cfg.Server.Port != 0 {
		port = cfg.Server.Port
	}
	return port, true
}

// torInternalRequest issues an HTTP request to a /server/tor/* internal
// endpoint on the running server's loopback listener and returns the decoded
// JSON response body.
func torInternalRequest(method, path string, body io.Reader) (map[string]any, int, error) {
	port, running := runningServerPort()
	if !running {
		return nil, 0, fmt.Errorf("no running server detected")
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := torCLIClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

// requireRunningServer exits 1 with the exact spec-mandated message
// (AI.md PART 31) when no running server is detected, for the subcommands
// that mutate the Tor process the server owns.
func requireRunningServer() {
	if _, running := runningServerPort(); !running {
		fmt.Fprintln(os.Stderr, "Error: no running server detected — start the server first")
		os.Exit(1)
	}
}

// runTor dispatches `search tor <subcommand>` per AI.md PART 31.
func runTor(args []string) {
	if len(args) == 0 {
		printTorHelp()
		os.Exit(1)
	}

	switch args[0] {
	case "status":
		runTorStatus()
	case "validate":
		runTorValidate()
	case "restart":
		requireRunningServer()
		runTorRestart()
	case "regenerate":
		requireRunningServer()
		runTorRegenerate()
	case "vanity":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: search tor vanity {start|apply}")
			os.Exit(1)
		}
		requireRunningServer()
		switch args[1] {
		case "start":
			var prefix string
			if len(args) > 2 {
				prefix = args[2]
			}
			runTorVanityStart(prefix)
		case "apply":
			runTorVanityApply()
		default:
			fmt.Fprintln(os.Stderr, "Usage: search tor vanity {start|apply}")
			os.Exit(1)
		}
	case "import-keys":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: search tor import-keys <path>")
			os.Exit(1)
		}
		requireRunningServer()
		runTorImportKeys(args[1])
	case "--help", "-h", "help":
		printTorHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown tor subcommand: %s\n\n", args[0])
		printTorHelp()
		os.Exit(1)
	}
}

// printTorHelp prints the tor subcommand usage table (AI.md PART 31).
func printTorHelp() {
	fmt.Println("Usage: search tor <subcommand>")
	fmt.Println()
	fmt.Println("Subcommands:")
	fmt.Println("  status              View Tor hidden service status")
	fmt.Println("  validate            Validate Tor configuration")
	fmt.Println("  restart             Restart the Tor process")
	fmt.Println("  regenerate          Regenerate the .onion address")
	fmt.Println("  vanity start <pfx>  Start vanity address search")
	fmt.Println("  vanity apply        Apply the found vanity address")
	fmt.Println("  import-keys <path>  Import an existing Tor key file")
}

// runTorStatus handles `search tor status`, falling back to on-disk state
// when no server is running (AI.md PART 31: status/validate MAY fall back
// since they are read-only).
func runTorStatus() {
	out, status, err := torInternalRequest(http.MethodGet, "/server/tor/status", nil)
	if err == nil && status == http.StatusOK {
		printTorStatusJSON(out)
		return
	}

	// Fallback: read on-disk state directly.
	hostname := readTorHostnameFallback()
	torrcPath := config.GetConfigDir() + "/tor/torrc"
	_, torrcErr := os.Stat(torrcPath)

	fmt.Println("Tor Hidden Service (offline — no running server detected)")
	if hostname != "" {
		fmt.Printf("  Address: %s\n", hostname)
	} else {
		fmt.Println("  Address: unknown")
	}
	if torrcErr == nil {
		fmt.Printf("  Config: %s\n", torrcPath)
	} else {
		fmt.Println("  Config: not found")
	}
}

// runTorValidate handles `search tor validate`, with the same offline
// fallback as status.
func runTorValidate() {
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/validate", nil)
	if err == nil && status == http.StatusOK {
		if data, ok := out["data"].(map[string]any); ok {
			printTorValidation(data)
			return
		}
	}

	// Fallback: on-disk checks only.
	torrcPath := config.GetConfigDir() + "/tor/torrc"
	valid := true
	var issues []string
	if _, err := os.Stat(torrcPath); err != nil {
		valid = false
		issues = append(issues, "torrc not found at "+torrcPath)
	}
	if readTorHostnameFallback() == "" {
		valid = false
		issues = append(issues, "no onion hostname on disk")
	}
	printTorValidation(map[string]any{"valid": valid, "issues": toAnySlice(issues)})
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func printTorValidation(data map[string]any) {
	valid, _ := data["valid"].(bool)
	if valid {
		fmt.Println("Tor configuration: valid")
		return
	}
	fmt.Println("Tor configuration: invalid")
	if issues, ok := data["issues"].([]any); ok {
		for _, issue := range issues {
			fmt.Printf("  - %v\n", issue)
		}
	}
}

func readTorHostnameFallback() string {
	data, err := os.ReadFile(config.GetDataDir() + "/tor/site/hostname")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func runTorRestart() {
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/restart", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: %v\n", out["error"])
		os.Exit(1)
	}
	fmt.Println("Tor restarted.")
	printTorStatusJSON(out)
}

func runTorRegenerate() {
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/regenerate", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: %v\n", out["error"])
		os.Exit(1)
	}
	if data, ok := out["data"].(map[string]any); ok {
		fmt.Printf("New onion address: %v\n", data["onion_address"])
	}
}

func runTorVanityStart(prefix string) {
	body, _ := json.Marshal(map[string]string{"prefix": prefix})
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/vanity/start", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: %v\n", out["error"])
		os.Exit(1)
	}
	fmt.Println("Vanity address search started in the background.")
	fmt.Println("Check progress with: search tor status")
}

func runTorVanityApply() {
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/vanity/apply", nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: %v\n", out["error"])
		os.Exit(1)
	}
	if data, ok := out["data"].(map[string]any); ok {
		fmt.Printf("Applied vanity address: %v\n", data["onion_address"])
	}
}

func runTorImportKeys(path string) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to read key file: %v\n", err)
		os.Exit(1)
	}
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/import-keys", bytes.NewReader(keyData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintf(os.Stderr, "Error: %v\n", out["error"])
		os.Exit(1)
	}
	if data, ok := out["data"].(map[string]any); ok {
		fmt.Printf("Imported. Onion address: %v\n", data["onion_address"])
	}
}

// printTorStatusJSON renders the `data` object of a /server/tor/status (or
// /restart) response for CLI display.
func printTorStatusJSON(out map[string]any) {
	data, ok := out["data"].(map[string]any)
	if !ok {
		fmt.Println("Tor Hidden Service: unknown")
		return
	}
	running, _ := data["running"].(bool)
	enabled, _ := data["enabled"].(bool)
	if !enabled {
		fmt.Println("Tor Hidden Service: Disabled")
		return
	}
	if running {
		fmt.Println("Tor Hidden Service: Connected")
	} else {
		fmt.Println("Tor Hidden Service: Disconnected")
	}
	if addr, ok := data["onion_address"].(string); ok && addr != "" {
		fmt.Printf("  Address: %s\n", addr)
	}
	if circuits, ok := data["circuits"]; ok {
		fmt.Printf("  Circuits: %v\n", circuits)
	}
	if version, ok := data["version"]; ok {
		fmt.Printf("  Version: %v\n", version)
	}
	if outbound, ok := data["outbound_enabled"]; ok {
		fmt.Printf("  Outbound Enabled: %v\n", outbound)
	}
}
