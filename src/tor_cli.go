package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/i18n"
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
		return nil, 0, errors.New(serverCLIText("server_cli.tor.no_running_server"))
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", port, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept-Language", serverCLILanguage())
	resp, err := torCLIClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer closeutil.Close(resp.Body)

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
		fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.no_running_server"))
		os.Exit(1)
	}
}

// runTor dispatches `search --maintenance tor <subcommand>` per AI.md PART 31.
func runTor(args []string) {
	yes := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--yes" {
			yes = true
			continue
		}
		filtered = append(filtered, arg)
	}
	args = filtered
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", "a Tor action is required"))
		printTorHelpTo(os.Stderr)
		exitFunc(2)
		return
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		printTorHelp()
		return
	}
	if pgpRequireAuthorized() == nil {
		return
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
		runTorRegenerate(yes)
	case "vanity":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", "a vanity action is required"))
			fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.usage_vanity"))
			exitFunc(2)
			return
		}
		requireRunningServer()
		switch args[1] {
		case "start":
			prefix, workers, err := parseTorVanityStartArgs(args[2:])
			if err != nil {
				fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", err))
				exitFunc(2)
				return
			}
			runTorVanityStart(prefix, workers)
		case "apply":
			address := ""
			if len(args) > 2 {
				address = args[2]
			}
			if len(args) > 3 {
				fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.usage_vanity"))
				exitFunc(2)
				return
			}
			runTorVanityApply(address, yes)
		case "stop":
			if len(args) > 2 {
				fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.usage_vanity_stop"))
				exitFunc(2)
				return
			}
			runTorVanityStop()
		default:
			fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.unknown_subcommand", "command", args[1]))
			fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.usage_vanity"))
			exitFunc(2)
			return
		}
	case "import-keys":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", "a key directory path is required"))
			fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.usage_import_keys"))
			exitFunc(2)
			return
		}
		requireRunningServer()
		if len(args) > 2 {
			fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.usage_import_keys"))
			exitFunc(2)
			return
		}
		runTorImportKeys(args[1], yes)
	case "--help", "-h", "help":
		printTorHelp()
	default:
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.unknown_subcommand", "command", args[0]))
		printTorHelpTo(os.Stderr)
		exitFunc(2)
	}
}

// printTorHelp prints the tor subcommand usage table (AI.md PART 31).
func printTorHelp() {
	printTorHelpTo(os.Stdout)
}

func printTorHelpTo(w io.Writer) {
	for _, line := range []string{
		serverCLIText("server_cli.tor.help_usage"),
		"",
		serverCLIText("server_cli.tor.help_subcommands"),
		serverCLIText("server_cli.tor.help_status"),
		serverCLIText("server_cli.tor.help_validate"),
		serverCLIText("server_cli.tor.help_restart"),
		serverCLIText("server_cli.tor.help_regenerate"),
		serverCLIText("server_cli.tor.help_vanity_start"),
		serverCLIText("server_cli.tor.help_vanity_stop"),
		serverCLIText("server_cli.tor.help_vanity_apply"),
		serverCLIText("server_cli.tor.help_import_keys"),
	} {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return
		}
	}
}

// runTorStatus handles `search --maintenance tor status`, falling back to on-disk state
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

	fmt.Println(serverCLIText("server_cli.tor.offline_heading"))
	if hostname != "" {
		fmt.Print(serverCLITf("server_cli.tor.address", "address", hostname))
	} else {
		fmt.Println(serverCLIText("server_cli.tor.address_unknown"))
	}
	if torrcErr == nil {
		fmt.Print(serverCLITf("server_cli.tor.config_path", "path", torrcPath))
	} else {
		fmt.Println(serverCLIText("server_cli.tor.config_missing"))
	}
}

// runTorValidate handles `search --maintenance tor validate`, with the same offline
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
		issues = append(issues, serverCLITf("server_cli.tor.torrc_missing", "path", torrcPath))
	}
	if readTorHostnameFallback() == "" {
		valid = false
		issues = append(issues, serverCLIText("server_cli.tor.hostname_missing"))
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
		fmt.Println(serverCLIText("server_cli.tor.config_valid"))
		return
	}
	fmt.Println(serverCLIText("server_cli.tor.config_invalid"))
	if issues, ok := data["issues"].([]any); ok {
		for _, issue := range issues {
			fmt.Print(serverCLITf("server_cli.tor.issue", "issue", issue))
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
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", err))
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", out["error"]))
		os.Exit(1)
	}
	fmt.Println(serverCLIText("server_cli.tor.restarted"))
	printTorStatusJSON(out)
}

func runTorRegenerate(yes bool) {
	if !confirmTorAction(yes, serverCLIText("server_cli.tor.confirm_regenerate")) {
		return
	}
	body, _ := json.Marshal(map[string]any{"confirm": true})
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/regenerate", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", err))
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", out["error"]))
		os.Exit(1)
	}
	if data, ok := out["data"].(map[string]any); ok {
		fmt.Print(serverCLITf("server_cli.tor.new_address", "address", data["onion_address"]))
	}
}

func parseTorVanityStartArgs(args []string) (string, int, error) {
	prefix := ""
	workers := 0
	workersSet := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--workers":
			if index+1 >= len(args) {
				return "", 0, errors.New(serverCLIText("server_cli.tor.workers_value_required"))
			}
			if workersSet {
				return "", 0, errors.New(serverCLIText("server_cli.tor.workers_duplicate"))
			}
			index++
			value, err := strconv.Atoi(args[index])
			if err != nil {
				return "", 0, errors.New(serverCLIText("server_cli.tor.workers_integer"))
			}
			workers = value
			workersSet = true
		case strings.HasPrefix(arg, "--workers="):
			if workersSet {
				return "", 0, errors.New(serverCLIText("server_cli.tor.workers_duplicate"))
			}
			value, err := strconv.Atoi(strings.TrimPrefix(arg, "--workers="))
			if err != nil {
				return "", 0, errors.New(serverCLIText("server_cli.tor.workers_integer"))
			}
			workers = value
			workersSet = true
		case arg == "--prefix":
			if index+1 >= len(args) {
				return "", 0, errors.New(serverCLIText("server_cli.tor.prefix_value_required"))
			}
			index++
			if prefix != "" {
				return "", 0, errors.New(serverCLIText("server_cli.tor.prefix_duplicate"))
			}
			prefix = args[index]
		case strings.HasPrefix(arg, "--"):
			return "", 0, errors.New(serverCLITf("server_cli.tor.unknown_option", "option", arg))
		default:
			if prefix != "" {
				return "", 0, errors.New(serverCLIText("server_cli.tor.prefix_duplicate"))
			}
			prefix = arg
		}
	}
	if prefix == "" {
		return "", 0, errors.New(serverCLIText("server_cli.tor.prefix_required"))
	}
	if workersSet && workers < 1 {
		return "", 0, errors.New(serverCLIText("server_cli.tor.workers_minimum"))
	}
	return prefix, workers, nil
}

func runTorVanityStart(prefix string, workers int) {
	request := map[string]any{"prefix": prefix}
	if workers > 0 {
		request["workers"] = workers
	}
	body, _ := json.Marshal(request)
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/vanity/start", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", err))
		os.Exit(1)
	}
	if status != http.StatusAccepted {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", out["error"]))
		os.Exit(1)
	}
	fmt.Println(serverCLIText("server_cli.tor.vanity_started"))
	fmt.Println(serverCLIText("server_cli.tor.check_progress"))
}

func runTorVanityStop() {
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/vanity/stop", nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", err))
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", out["error"]))
		os.Exit(1)
	}
	if data, ok := out["data"].(map[string]any); ok {
		if stopped, _ := data["stopped"].(bool); stopped {
			fmt.Println(serverCLIText("server_cli.tor.vanity_stopped"))
		} else {
			fmt.Println(serverCLIText("server_cli.tor.vanity_not_running"))
		}
	}
}

func runTorVanityApply(address string, yes bool) {
	if !confirmTorAction(yes, serverCLIText("server_cli.tor.confirm_apply_vanity")) {
		return
	}
	body, _ := json.Marshal(map[string]any{"address": address, "confirm": true})
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/vanity/apply", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", err))
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", out["error"]))
		os.Exit(1)
	}
	if data, ok := out["data"].(map[string]any); ok {
		fmt.Print(serverCLITf("server_cli.tor.applied_vanity", "address", data["onion_address"]))
	}
}

func runTorImportKeys(path string, yes bool) {
	if !filepath.IsAbs(path) {
		fmt.Fprintln(os.Stderr, serverCLIText("server_cli.tor.path_absolute"))
		exitFunc(2)
		return
	}
	if !confirmTorAction(yes, serverCLIText("server_cli.tor.confirm_import")) {
		return
	}
	body, _ := json.Marshal(map[string]any{"path": filepath.Clean(path), "confirm": true})
	out, status, err := torInternalRequest(http.MethodPost, "/server/tor/import-keys", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", err))
		os.Exit(1)
	}
	if status != http.StatusOK {
		fmt.Fprintln(os.Stderr, serverCLITf("server_cli.tor.error", "error", out["error"]))
		os.Exit(1)
	}
	if data, ok := out["data"].(map[string]any); ok {
		fmt.Print(serverCLITf("server_cli.tor.imported_address", "address", data["onion_address"]))
	}
}

func confirmTorAction(yes bool, prompt string) bool {
	if yes {
		return true
	}
	if !torConfirmationAccepted(pgpReadLine(prompt)) {
		fmt.Println(serverCLIText("admin.status.cancelled"))
		return false
	}
	return true
}

func torConfirmationAccepted(answer string) bool {
	return strings.EqualFold(strings.TrimSpace(answer), "yes")
}

// printTorStatusJSON renders the `data` object of a /server/tor/status (or
// /restart) response for CLI display.
func printTorStatusJSON(out map[string]any) {
	data, ok := out["data"].(map[string]any)
	if !ok {
		fmt.Println(serverCLIText("server_cli.tor.status_unknown"))
		return
	}
	running, _ := data["running"].(bool)
	if running {
		fmt.Println(serverCLIText("server_cli.tor.status_connected"))
	} else {
		fmt.Println(serverCLIText("server_cli.tor.status_disconnected"))
	}
	if pid, ok := data["pid"]; ok {
		fmt.Print(serverCLITf("server_cli.tor.pid", "pid", pid))
	}
	if addr, ok := data["onion_address"].(string); ok && addr != "" {
		fmt.Print(serverCLITf("server_cli.tor.address", "address", addr))
	}
	if percent, ok := data["bootstrap_percent"]; ok {
		fmt.Print(serverCLITf("server_cli.tor.bootstrap", "percent", percent))
	}
	if established, ok := data["circuit_established"].(bool); ok {
		value := i18n.T(serverCLILanguage(), "common.no")
		if established {
			value = i18n.T(serverCLILanguage(), "common.yes")
		}
		fmt.Print(serverCLITf("server_cli.tor.circuit_established", "state", value))
	}
	if version, ok := data["version"]; ok {
		fmt.Print(serverCLITf("server_cli.tor.version", "version", version))
	}
	if vanity, ok := data["vanity"].(map[string]any); ok {
		state, _ := vanity["state"].(string)
		fmt.Print(serverCLITf("server_cli.tor.vanity_state", "state", serverCLIText("server_cli.tor.vanity_state_"+state)))
		for _, field := range []string{"prefix", "workers", "attempts", "rate", "elapsed_seconds"} {
			if value, exists := vanity[field]; exists && value != "" && value != float64(0) && value != int(0) {
				fmt.Print(serverCLITf("server_cli.tor.vanity_"+field, "value", value))
			}
		}
		if candidates, ok := vanity["candidates"].([]any); ok && len(candidates) > 0 {
			fmt.Println(serverCLIText("server_cli.tor.vanity_candidates"))
			for _, candidate := range candidates {
				fmt.Printf("  %v\n", candidate)
			}
		}
	}
}
