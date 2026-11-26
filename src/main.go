package main

import (
	"context"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/common/banner"
	"github.com/apimgr/search/src/common/closeutil"
	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/common/i18n"
	"github.com/apimgr/search/src/common/netutil"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/database"
	"github.com/apimgr/search/src/email"
	"github.com/apimgr/search/src/mode"
	"github.com/apimgr/search/src/server"
	"github.com/apimgr/search/src/service"
	sigsvc "github.com/apimgr/search/src/signal"
	"github.com/apimgr/search/src/update"

	_ "modernc.org/sqlite"
)

// CLI flags (per AI.md PART 8: SERVER BINARY CLI)
var (
	flagVersion     bool
	flagHelp        bool
	flagStatus      bool
	flagDaemon      bool
	flagDebug       bool
	flagI2P         bool
	flagService     string
	flagMaintenance string
	flagUpdate      string
	flagShell       string

	// Required flags per AI.md PART 6 (NON-NEGOTIABLE)
	flagMode    string
	flagData    string
	flagConfig  string
	flagCache   string
	flagLog     string
	flagBackup  string
	flagPID     string
	flagAddress string
	flagPort    string
	flagBaseURL string
	flagColor   string
	flagLang    string
	flagBackupIncludeSSL  bool
	flagBackupIncludeData bool
)

// exitFunc is the function used to exit the process.
// Overriding this in tests prevents os.Exit from terminating the test process.
var exitFunc = os.Exit

func init() {
	// Simple commands
	flag.BoolVar(&flagVersion, "version", false, "Show version information")
	flag.BoolVar(&flagVersion, "v", false, "Show version information (shorthand)")
	flag.BoolVar(&flagHelp, "help", false, "Show help message")
	flag.BoolVar(&flagHelp, "h", false, "Show help message (shorthand)")
	flag.BoolVar(&flagStatus, "status", false, "Show server status")
	flag.BoolVar(&flagDaemon, "daemon", false, "Daemonize (detach from terminal)")
	flag.BoolVar(&flagDebug, "debug", false, "Enable debug mode (verbose logging, debug endpoints)")
	flag.BoolVar(&flagI2P, "i2p", false, "Enable the I2P eepsite (default on in application containers)")

	// Commands with optional arguments
	flag.StringVar(&flagService, "service", "", "Service management: start|stop|restart|reload|status|--install|--uninstall|--disable|--help")
	flag.StringVar(&flagMaintenance, "maintenance", "", "Maintenance: backup|restore|update|mode|setup|pgp|security|secret|token|data|compliance|email|scheduler|--help")
	flag.BoolVar(&flagBackupIncludeSSL, "include-ssl", false, "Include optional SSL files in a maintenance backup")
	flag.BoolVar(&flagBackupIncludeData, "include-data", false, "Include optional data files in a maintenance backup")
	flag.StringVar(&flagUpdate, "update", "", "Update management: check|yes|branch")
	flag.StringVar(&flagShell, "shell", "", "Shell integration: completions|init|--help")

	// Configuration override flags (NON-NEGOTIABLE per AI.md PART 6)
	flag.StringVar(&flagMode, "mode", "", "Set application mode (production|development|debug)")
	flag.StringVar(&flagData, "data", "", "Set data directory")
	flag.StringVar(&flagConfig, "config", "", "Set config directory")
	flag.StringVar(&flagCache, "cache", "", "Set cache directory")
	flag.StringVar(&flagLog, "log", "", "Set log directory")
	flag.StringVar(&flagBackup, "backup", "", "Set backup directory")
	flag.StringVar(&flagPID, "pid", "", "Set PID file path")
	flag.StringVar(&flagAddress, "address", "", "Set listen address")
	flag.StringVar(&flagPort, "port", "", "Set listen port or HTTP,HTTPS ports")
	flag.StringVar(&flagBaseURL, "baseurl", "", "Set URL path prefix for reverse proxy (default: /)")
	// Per AI.md PART 8: --color {auto|yes|no}
	flag.StringVar(&flagColor, "color", "auto", "Set color output mode (auto|yes|no)")
	// Per AI.md PART 8: --lang sets language for output (default: auto, from LANG env)
	flag.StringVar(&flagLang, "lang", "", "Set language for output (default: auto)")
}

func main() {
	// Custom usage function
	flag.Usage = func() {
		printHelp()
	}
	if command, ok := commandHelp(os.Args[1:], "shell", "service", "maintenance", "update"); ok {
		printCommandHelp(command)
		return
	}

	// Parse flags
	flag.Parse()
	if (flagBackupIncludeSSL || flagBackupIncludeData) && flagMaintenance != "backup" {
		fmt.Fprintln(os.Stderr, "--include-ssl and --include-data are valid only with --maintenance backup")
		exitFunc(2)
		return
	}

	// Initialize display output (colors and emojis) based on NO_COLOR, TERM, and --color flag
	// Per AI.md PART 8: Must be called early, before any output
	display.InitOutput(flagColor)

	// Apply CLI overrides to config (before any other operations)
	applyCliOverrides()

	// Handle commands
	switch {
	case flagVersion:
		printVersion()
		return
	case flagHelp:
		printHelp()
		return
	case flagStatus:
		showStatus()
		return
	case flagService != "":
		runService(flagService)
		return
	case flagMaintenance != "":
		runMaintenance(flagMaintenance)
		return
	case flagUpdate != "" || (len(os.Args) > 1 && os.Args[1] == "--update"):
		subCmd := flagUpdate
		if subCmd == "" {
			subCmd = "yes"
		}
		runUpdate(subCmd)
		return
	case flagShell != "" || (len(os.Args) > 1 && os.Args[1] == "--shell"):
		subCmd := flagShell
		if subCmd == "" && len(os.Args) > 2 {
			subCmd = os.Args[2]
		}
		if subCmd == "" {
			subCmd = "--help"
		}
		runShell(subCmd)
		return
	case len(os.Args) > 1 && os.Args[1] == "tor":
		runTor(os.Args[2:])
		return
	}

	// Handle legacy argument style (for backwards compatibility)
	// Skip if runtime flags are set (--port, --address, --mode, etc. are handled by flag.Parse())
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "--") && !strings.Contains(os.Args[1], "=") {
		// Don't call handleLegacyArgs for runtime configuration flags
		// These are already handled by flag.Parse() and applyCliOverrides()
		runtimeFlags := map[string]bool{
			"--port": true, "--address": true, "--mode": true, "--data": true,
			"--config": true, "--cache": true, "--log": true, "--backup": true,
			"--pid": true, "--debug": true, "--daemon": true,
			"--baseurl": true, "--color": true, "--lang": true,
		}
		if !runtimeFlags[os.Args[1]] {
			handleLegacyArgs()
			return
		}
	}

	// Start server
	runServer()
}

// applyCliOverrides applies CLI flag overrides to the config system
// Per AI.md PART 6: Directory flags MUST create directories if they don't exist
func applyCliOverrides() {
	// Set mode from CLI flag or environment
	// Per AI.md PART 6: Mode Detection Priority
	if flagMode != "" {
		if err := os.Setenv("MODE", flagMode); err != nil {
			slog.Warn("failed to set MODE", "error", err)
		}
		mode.SetAppMode(flagMode)
	} else {
		// Initialize mode from environment
		mode.FromEnv()
	}

	// Set debug mode from CLI flag
	// Per AI.md PART 6: --debug enables debug mode (verbose logging, debug endpoints)
	if flagDebug {
		if err := os.Setenv("DEBUG", "true"); err != nil {
			slog.Warn("failed to set DEBUG", "error", err)
		}
		mode.SetDebugEnabled(true)
	}
	if err := validateCLIPathOverrides(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitFunc(2)
		return
	}

	if flagData != "" {
		config.SetDataDirOverride(flagData)
	}
	if flagConfig != "" {
		config.SetConfigDirOverride(flagConfig)
	}
	if flagCache != "" {
		config.SetCacheDirOverride(flagCache)
	}
	if flagLog != "" {
		config.SetLogDirOverride(flagLog)
	}
	if flagBackup != "" {
		config.SetBackupDirOverride(flagBackup)
	}
	if flagPID != "" {
		config.SetPIDFileOverride(flagPID)
	}
	if flagPort != "" {
		if err := os.Setenv("PORT", flagPort); err != nil {
			slog.Warn("failed to set PORT", "error", err)
		}
	}
	i2pFlagSet := false
	flag.Visit(func(parsed *flag.Flag) {
		if parsed.Name == "i2p" {
			i2pFlagSet = true
		}
	})
	if err := applyI2PFlagOverride(i2pFlagSet, flagI2P); err != nil {
		slog.Warn("failed to set I2P_ENABLED", "error", err)
	}
	if flagAddress != "" {
		if err := os.Setenv("LISTEN", flagAddress); err != nil {
			slog.Warn("failed to set LISTEN", "error", err)
		}
	}
	// Per AI.md PART 6: --baseurl sets URL path prefix for reverse proxy
	if flagBaseURL != "" {
		config.SetBaseURLOverride(flagBaseURL)
	}
	// Per AI.md PART 8: Color priority: CLI flag > Config > NO_COLOR env > Auto-detect
	if flagColor != "" {
		config.SetColorMode(flagColor)
	}
	// Per AI.md PART 8: --lang sets language for CLI output
	if flagLang != "" {
		if err := os.Setenv("SEARCH_LANG", flagLang); err != nil {
			slog.Warn("failed to set SEARCH_LANG", "error", err)
		}
		if err := os.Setenv("LANG", flagLang); err != nil {
			slog.Warn("failed to set LANG", "error", err)
		}
	}

	// Ensure directories exist after CLI overrides are applied
	// Per AI.md PART 6: All directory flags MUST create directories if they don't exist
	if flagData != "" || flagConfig != "" || flagCache != "" || flagLog != "" || flagBackup != "" || flagPID != "" {
		if err := config.EnsureDirectories(); err != nil {
			slog.Warn("Failed to create directories", "err", err)
		}
	}
}

func validateCLIPathOverrides() error {
	paths := []struct {
		name  string
		value *string
	}{
		{"--config", &flagConfig},
		{"--data", &flagData},
		{"--cache", &flagCache},
		{"--log", &flagLog},
		{"--backup", &flagBackup},
		{"--pid", &flagPID},
	}
	for _, path := range paths {
		if *path.value == "" {
			continue
		}
		safe, err := config.SafeDirPath(*path.value)
		if err != nil {
			return fmt.Errorf("invalid %s path: %w", path.name, err)
		}
		*path.value = safe
	}
	return nil
}

func applyI2PFlagOverride(explicit, enabled bool) error {
	if !explicit {
		return nil
	}
	return os.Setenv("I2P_ENABLED", strconv.FormatBool(enabled))
}

func commandHelp(args []string, commands ...string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		next := args[i+1]
		if next != "--help" && next != "-h" && next != "help" {
			continue
		}
		for _, command := range commands {
			if args[i] == "--"+command {
				return command, true
			}
		}
	}
	return "", false
}

func printCommandHelp(command string) {
	switch command {
	case "shell":
		printShellHelp(filepath.Base(os.Args[0]))
	case "service":
		printServiceHelp()
	case "maintenance":
		runMaintenance("--help")
	case "update":
		runUpdate("--help")
	}
}

// handleLegacyArgs handles old-style arguments for backwards compatibility
func handleLegacyArgs() {
	switch os.Args[1] {
	case "--version", "-v":
		printVersion()
	case "--help", "-h":
		printHelp()
	case "--status":
		showStatus()
	case "--service":
		if len(os.Args) > 2 {
			runService(os.Args[2])
		} else {
			slog.Error("Missing service subcommand", "usage", "search --service {start,stop,restart,reload,status,--install,--uninstall,--disable,--help}")
		}
	case "--maintenance":
		if len(os.Args) > 2 {
			runMaintenance(os.Args[2])
		} else {
			slog.Error("Missing maintenance subcommand", "usage", "search --maintenance {backup|restore|update|mode|setup|pgp|security|secret|token|data|compliance|email|scheduler|tor|--help}")
		}
	case "--update":
		subCmd := "yes"
		if len(os.Args) > 2 {
			subCmd = os.Args[2]
		}
		runUpdate(subCmd)
	case "--shell":
		subCmd := "--help"
		if len(os.Args) > 2 {
			subCmd = os.Args[2]
		}
		runShell(subCmd)
	case "--daemon":
		// Per AI.md PART 8: Only -h and -v may have short flags
		flagDaemon = true
		runServer()
	default:
		slog.Error("Unknown command", "cmd", os.Args[1], "hint", "Use --help for usage information")
	}
}

var startServerBody = runServerServiceBody

var (
	startupListeners        map[int]net.Listener
	startupRedirectListener net.Listener
)

func runServer() {
	// Per AI.md PART 24: when the Service Control Manager starts the process,
	// run the server under the SCM state machine instead of returning.
	// IsWindowsService is a no-op (false) on non-Windows builds.
	if service.IsWindowsService() {
		if err := service.RunAsWindowsService(func() error {
			startServerBody()
			return nil
		}, func() {
			// Trigger the same graceful shutdown path signal handling uses.
			if !sigsvc.IsShuttingDown() {
				if p, err := os.FindProcess(os.Getpid()); err == nil {
					_ = p.Signal(os.Interrupt) //nolint:errcheck
				}
			}
		}); err != nil {
			slog.Error("Windows service exited with error", "err", err)
			exitFunc(1)
		}
		return
	}

	// Unix-like platforms and ordinary Windows processes enter the same
	// long-running server startup path. Only the Windows Service Control
	// Manager wraps that path in its platform service state machine.
	startServerBody()
}

func bindPrivilegedListeners(cfg *config.Config) (map[int]net.Listener, net.Listener, error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("configuration is required")
	}

	ports := []int{cfg.Server.GetHTTPPort()}
	if cfg.Server.IsDualPortMode() {
		ports = append(ports, cfg.Server.GetHTTPSPort())
	}
	var redirectPort int
	if cfg.Server.SSL.AutoTLS && !cfg.Server.IsDualPortMode() {
		redirectPort = 80
	}

	listeners := make(map[int]net.Listener)
	closeListeners := func() {
		for _, listener := range listeners {
			_ = listener.Close()
		}
	}
	for _, port := range ports {
		if port <= 0 || port >= 1024 {
			continue
		}
		if _, exists := listeners[port]; exists {
			continue
		}
		addr := netutil.JoinHostPort(cfg.Server.Address, fmt.Sprint(port))
		listener, err := netutil.ListenTCP(cfg.Server.Address, port)
		if err != nil {
			closeListeners()
			return nil, nil, fmt.Errorf("bind %s: %w", addr, err)
		}
		listeners[port] = listener
	}

	var redirectListener net.Listener
	if redirectPort > 0 {
		addr := netutil.JoinHostPort(cfg.Server.Address, fmt.Sprint(redirectPort))
		listener, err := netutil.ListenTCP(cfg.Server.Address, redirectPort)
		if err != nil {
			closeListeners()
			return nil, nil, fmt.Errorf("bind AutoTLS redirect %s: %w", addr, err)
		}
		redirectListener = listener
	}
	return listeners, redirectListener, nil
}

func earlyPortConfig() (*config.Config, error) {
	cfg := config.DefaultConfig()
	env := config.LoadFromEnv()
	portValue := strings.TrimSpace(env.Port)
	if flagPort != "" {
		portValue = flagPort
	}
	var port, httpsPort int
	if portValue != "" {
		var err error
		port, httpsPort, err = config.ParsePortValue(portValue)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT value: %w", err)
		}
	} else {
		var found bool
		var err error
		port, httpsPort, found, err = config.PeekServerPort(config.GetConfigPath())
		if err != nil {
			return nil, err
		}
		if !found || port == 0 {
			port = config.GetRandomPort()
			if err := os.Setenv("PORT", strconv.Itoa(port)); err != nil {
				return nil, fmt.Errorf("persist early random port for config initialization: %w", err)
			}
		}
	}
	cfg.Server.Port = port
	cfg.Server.HTTPSPort = httpsPort
	listenAddress := strings.TrimSpace(flagAddress)
	if listenAddress == "" {
		listenAddress = strings.TrimSpace(env.Listen)
	}
	if listenAddress == "" {
		address, found, err := config.PeekServerAddress(config.GetConfigPath())
		if err != nil {
			return nil, err
		}
		if found {
			listenAddress = address
		}
	}
	if listenAddress != "" {
		cfg.Server.Address = listenAddress
	}
	return cfg, nil
}

func runServerServiceBody() {
	// Handle daemonization per AI.md PART 6
	// Read only server.daemonize here so re-execution happens before root binds.
	daemonizeFromConfig := false
	if !config.IsRunningInContainer() {
		if enabled, found, err := config.PeekServerDaemonize(config.GetConfigPath()); err != nil {
			slog.Warn("could not read early daemonize setting", "error", err)
		} else {
			daemonizeFromConfig = found && enabled
		}
	}
	if (flagDaemon || daemonizeFromConfig) && !config.IsRunningInContainer() && os.Getenv("_DAEMON_CHILD") != "1" {
		if err := daemonize(); err != nil {
			slog.Error("Failed to daemonize", "err", err)
			exitFunc(1)
		}
		// Parent exits in daemonize(), only child continues
	}

	// PART 8 step 7b: a root-started process applies service-user staged updates
	// before creating users, changing ownership, binding ports, or dropping
	// privileges. User-started and Windows processes never replace the binary.
	if config.StartedElevated() {
		applied, err := update.NewManager().ApplyStagedUpdate()
		if err != nil {
			slog.Error("staged update could not be applied; continuing with current binary", "err", err)
			if setErr := os.Setenv("_SEARCH_UPDATE_FAILED", err.Error()); setErr != nil {
				slog.Error("failed to record staged update error for notification", "err", setErr)
			}
			var failedUpdate *update.StagedUpdateFailure
			if errors.As(err, &failedUpdate) && failedUpdate.Version != "" {
				if setErr := os.Setenv("_SEARCH_UPDATE_FAILED_VERSION", failedUpdate.Version); setErr != nil {
					slog.Error("failed to record staged update version for notification", "err", setErr)
				}
			}
		} else if applied {
			return
		}
	}

	// Per AI.md PART 8 and PART 26: root performs privileged setup and then
	// drops to the dedicated service account on hosts and inside containers.
	if service.IsRunningAsRoot() {
		containerMode := config.IsRunningInContainer()
		slog.Info("Running as root - performing privileged setup")

		// Step 8a: Create system user
		svcUser, err := service.CreateSystemUser(config.InternalName)
		if err != nil {
			slog.Error("Failed to create system user", "err", err)
			exitFunc(1)
		}
		slog.Info("System user ready", "user", svcUser.Name, "uid", svcUser.UID, "gid", svcUser.GID)

		// Step 8b-d: Create directories and set ownership (while still root)
		if !containerMode {
			if err := config.EnsureSystemDirectories(config.InternalName); err != nil {
				slog.Error("Failed to create system directories", "err", err)
				exitFunc(1)
			}
			slog.Info("System directories created")
		}

		// Step 8b-c: Create and chown the ACTUAL resolved runtime directories
		// (these honor --data/--config/--cache/--log/--backup/--pid CLI flags
		// per PART 8 step 7; EnsureSystemDirectories above only covers the
		// hardcoded default system paths). Without this, directories created
		// via custom flags stay root-owned after the privilege drop below and
		// the service user cannot write to them (PID file, database, etc.)
		if err := config.EnsureDirectories(); err != nil {
			slog.Error("Failed to create runtime directories", "err", err)
			exitFunc(1)
		}
		if err := config.ChownRuntimeDirectories("search"); err != nil {
			slog.Error("Failed to chown runtime directories", "err", err)
			exitFunc(1)
		}

		// Set environment for the service user (HOME points to data dir)
		// This ensures config.Initialize() uses the correct paths after privilege drop
		// Only applies to the default system paths — custom overrides (already
		// applied via config.SetXDirOverride above) take precedence and are
		// left untouched here.
		if !containerMode {
			if flagData == "" {
				config.SetDataDirOverride("/var/lib/" + config.InternalOrg + "/" + config.InternalName)
			}
			if flagConfig == "" {
				config.SetConfigDirOverride("/etc/" + config.InternalOrg + "/" + config.InternalName)
			}
			if flagLog == "" {
				config.SetLogDirOverride("/var/log/" + config.InternalOrg + "/" + config.InternalName)
			}
			if flagCache == "" {
				config.SetCacheDirOverride("/var/cache/" + config.InternalOrg + "/" + config.InternalName)
			}
		}
		if err := os.Setenv("HOME", config.GetDataDir()); err != nil {
			slog.Warn("failed to set service HOME", "error", err)
		}

		// Read only the effective port before binding privileged sockets. The
		// full configuration load happens after PID handling below.
		cfg, err := earlyPortConfig()
		if err != nil {
			slog.Error("Early port resolution failed", "err", err)
			exitFunc(1)
			return
		}
		listeners, redirectListener, err := bindPrivilegedListeners(cfg)
		if err != nil {
			slog.Error("Failed to bind privileged ports", "err", err)
			exitFunc(1)
			return
		}
		startupListeners = listeners
		startupRedirectListener = redirectListener

		// Step 8g: Drop privileges to search user
		slog.Info("Dropping privileges", "user", svcUser.Name)
		if err := service.DropPrivileges(svcUser.Name); err != nil {
			slog.Error("Failed to drop privileges", "err", err)
			exitFunc(1)
		}

		// Step 8h: Verify privilege drop succeeded
		if err := service.VerifyPrivilegesDropped(); err != nil {
			slog.Error("Privilege verification failed", "err", err)
			exitFunc(1)
		}
		slog.Info("Privileges dropped successfully")
	}

	if err := config.EnsureDirectories(); err != nil {
		slog.Error("Failed to create runtime directories", "err", err)
		exitFunc(1)
		return
	}

	// Step 11: Check PID file for existing running instance (stale PID detection)
	// Per AI.md PART 8: Startup sequence step 11 — check before writing
	pidFile := config.GetPIDFile()
	if running, existingPID, err := server.CheckPIDFile(pidFile); err != nil {
		slog.Error("PID file check failed", "err", err)
		exitFunc(1)
	} else if running {
		slog.Error("Server already running", "pid", existingPID)
		exitFunc(1)
	}

	// Step 12: Write PID file
	// Per AI.md PART 8: Startup sequence step 12 — write after stale check passes
	if err := server.WritePIDFile(pidFile); err != nil {
		slog.Error("Failed to write PID file", "err", err)
		exitFunc(1)
	}
	defer func() {
		if err := server.RemovePIDFile(pidFile); err != nil {
			slog.Warn("failed to remove server PID file", "error", err)
		}
	}()

	// Full settings are loaded after PID ownership is established. This keeps
	// the privileged phase limited to the server.port peek and socket binding.
	cfg, err := config.Initialize()
	if err != nil {
		slog.Error("Configuration failed", "err", err)
		exitFunc(1)
		return
	}

	// Show the operator token once, at first creation, so it can be saved.
	showSetup := cfg.IsFirstRun()
	if cfg.Server.Backup.EncryptionPassword == "" && !cfg.Server.Compliance.AnyEnabled() {
		slog.Warn("backup encryption is not configured; backup archives are unencrypted", "setting", "server.backup.encryption_password")
	}

	// Create server (initialises DB, scheduler, etc. — steps 13-16 per AI.md PART 8)
	srv := server.NewServer(cfg)
	srv.SetPreboundListeners(startupListeners, startupRedirectListener)
	startupListeners = nil
	startupRedirectListener = nil

	// Step 19: Setup signal handling per AI.md PART 7.
	// Registered before the server goroutine so signals arriving during Tor startup
	// (step 17, up to 3 min) are handled correctly.
	shutdownDone := sigsvc.Setup(sigsvc.ShutdownConfig{
		ShutdownFunc: srv.Shutdown,
		PIDFile:      config.GetPIDFile(),
	})

	// Steps 17-18: Start Tor then bind HTTP socket in a goroutine.
	// readyCh is closed once the TCP socket is accepting connections.
	// Per AI.md PART 8: banner (step 20) must be printed after binding (step 18).
	readyCh := make(chan struct{})
	serverErrCh := make(chan error, 1)
	go func() {
		if err := srv.StartHTTPServer(readyCh); err != nil {
			serverErrCh <- err
		}
		close(serverErrCh)
	}()

	// Wait for server to signal readiness (timeout after 5 min to allow Tor bootstrap)
	select {
	case <-readyCh:
		// Socket is bound and accepting connections
	case err := <-serverErrCh:
		if err != nil {
			slog.Error("Server failed to start", "err", err)
		}
		exitFunc(1)
	case <-time.After(5 * time.Minute):
		slog.Error("Server failed to start within 5-minute timeout")
		exitFunc(1)
	}
	hostname, _ := os.Hostname()
	srv.NotifyStartup(hostname)
	previousVersion := os.Getenv("_SEARCH_UPDATE_PREVIOUS_VERSION")
	newVersion := os.Getenv("_SEARCH_UPDATE_NEW_VERSION")
	if previousVersion != "" && newVersion != "" {
		_ = os.Unsetenv("_SEARCH_UPDATE_PREVIOUS_VERSION")
		_ = os.Unsetenv("_SEARCH_UPDATE_NEW_VERSION")
		srv.NotifyUpdateInstalled(previousVersion, newVersion)
	}
	if updateFailure := os.Getenv("_SEARCH_UPDATE_FAILED"); updateFailure != "" {
		_ = os.Unsetenv("_SEARCH_UPDATE_FAILED")
		failedVersion := os.Getenv("_SEARCH_UPDATE_FAILED_VERSION")
		_ = os.Unsetenv("_SEARCH_UPDATE_FAILED_VERSION")
		srv.NotifyUpdateFailed(failedVersion, updateFailure)
	}
	if cfg.Server.Scheduler.Tasks.BlocklistUpdate.Enabled {
		srv.StartBlocklistUpdateIfMissing()
	}
	if cfg.Server.Scheduler.Tasks.CVEUpdate.Enabled {
		srv.StartCVEUpdateIfMissing()
	}

	// Step 20: Print responsive startup banner per AI.md PART 7 and PART 8.
	// The server is already listening; wait up to five seconds for overlay identity files.
	srv.WaitForOverlayAddresses(5 * time.Second)
	urls := buildListenURLs(cfg)
	if torAddr := srv.TorAddress(); torAddr != "" {
		// Add live Tor address — buildListenURLs reads cfg.Server.Tor.OnionAddress which
		// may be empty on first run before the address is persisted.
		alreadyHasOnion := false
		for _, u := range urls {
			if strings.Contains(u, ".onion") {
				alreadyHasOnion = true
				break
			}
		}
		if !alreadyHasOnion {
			urls = append(urls, "http://"+torAddr)
		}
	}
	if i2pAddr := srv.I2PAddress(); i2pAddr != "" {
		alreadyHasI2P := false
		for _, u := range urls {
			if strings.Contains(u, ".b32.i2p") {
				alreadyHasI2P = true
				break
			}
		}
		if !alreadyHasI2P {
			urls = append(urls, "http://"+i2pAddr)
		}
	}

	banner.Print(banner.Config{
		AppName:   "Search",
		Version:   config.Version,
		Mode:      cfg.Server.Mode,
		Debug:     serverBannerDebugEnabled(cfg),
		URLs:      urls,
		ShowSetup: showSetup,
		SetupToken: func() string {
			if showSetup {
				return cfg.Server.Token
			}
			return ""
		}(),
	})

	// Step 21: Block until graceful shutdown completes.
	// Per AI.md PART 7: os.Exit() belongs only in main().
	<-shutdownDone
	if err, ok := <-serverErrCh; ok && err != nil {
		slog.Error("Server stopped with an error", "err", err)
		exitFunc(1)
		return
	}
	exitFunc(sigsvc.ShutdownExitCode())
}

func serverBannerDebugEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.IsDebug()
}

func printVersion() {
	// Per AI.md PART 13: --version format
	// Format:
	//   {binary} {version}
	//   Built: {BUILD_DATE}
	//   Go: {GO_VERSION}
	//   OS/Arch: {GOOS}/{GOARCH}
	// Note: No v prefix in version string
	binaryName := filepath.Base(os.Args[0])

	fmt.Println(serverCLITf("server_cli.version.binary", "binary", binaryName, "version", config.Version))
	fmt.Println(serverCLITf("server_cli.version.built", "date", config.BuildDate))
	fmt.Println(serverCLITf("server_cli.version.go", "version", runtime.Version()))
	fmt.Println(serverCLITf("server_cli.version.os_arch", "os", runtime.GOOS, "arch", runtime.GOARCH))
}

func printHelp() {
	// Per AI.md PART 6: Use actual binary name in help.
	binaryName := filepath.Base(os.Args[0])
	fmt.Println(serverCLITf("server_cli.help.full", "binary", binaryName))
}
func showStatus() {
	// Per AI.md PART 31 - --status output format
	binaryName := filepath.Base(os.Args[0])

	// Check PID file and process status
	pidFile := config.GetPIDFile()
	isRunning := false
	var pid int
	var startTime time.Time

	if pidData, err := os.ReadFile(pidFile); err == nil {
		pidStr := strings.TrimSpace(string(pidData))
		if p, err := fmt.Sscanf(pidStr, "%d", &pid); err == nil && p == 1 {
			// Check if process is actually running
			if isProcessRunning(pid) {
				isRunning = true
				// Try to get process start time for uptime calculation
				startTime = getProcessStartTime(pid)
			}
		}
	}

	fmt.Println()

	if isRunning {
		fmt.Println(serverCLIText("server_cli.status.server_running"))
	} else {
		fmt.Println(serverCLIText("server_cli.status.server_not_running"))
		fmt.Println()
		fmt.Println(serverCLITf("server_cli.status.start_server", "binary", binaryName))
		fmt.Println(serverCLITf("server_cli.status.install_service", "binary", binaryName))
		// Per AI.md PART 8: --status exits 0 when healthy and 1 when unhealthy.
		// A server that is not running is unhealthy.
		exitFunc(1)
		return
	}

	// Load config to show settings
	var port int
	var mode string
	var torEnabled bool
	var torAddress string
	var i2pEnabled bool
	var baseurl string
	var fqdn string

	configPath := config.GetConfigPath()
	if cfg, err := config.Load(configPath); err == nil {
		port = cfg.Server.Port
		mode = cfg.Server.Mode
		torEnabled = cfg.Server.Tor.Enabled
		torAddress = cfg.Server.Tor.OnionAddress
		i2pEnabled = cfg.I2PEnabled()
		baseurl = cfg.Server.BaseURL
		fqdn = cfg.Server.FQDN
	} else {
		// Try to get from env or defaults
		port = 64580
		mode = "production"
	}

	// Calculate uptime
	uptime := "unknown"
	if !startTime.IsZero() {
		uptime = formatUptime(time.Since(startTime))
	}

	fmt.Println(serverCLITf("server_cli.status.pid", "pid", pid))
	fmt.Println(serverCLITf("server_cli.status.port", "port", port))
	fmt.Println(serverCLITf("server_cli.status.mode", "mode", mode))
	fmt.Println(serverCLITf("server_cli.status.uptime", "uptime", uptime))
	fmt.Println()

	// Instance mode (single-instance per AI.md line 2055)
	fmt.Println(serverCLIText("server_cli.status.mode_standalone"))
	fmt.Println()

	// Tor Hidden Service status
	if torEnabled {
		if torAddress != "" {
			fmt.Println(serverCLIText("server_cli.status.tor_connected"))
			// Truncate address for display
			displayAddr := torAddress
			if len(torAddress) > 16 {
				displayAddr = torAddress[:8] + "..." + torAddress[len(torAddress)-10:]
			}
			fmt.Println(serverCLITf("server_cli.status.tor_address", "address", displayAddr))
		} else {
			fmt.Println(serverCLIText("server_cli.status.tor_waiting"))
		}
	} else {
		fmt.Println(serverCLIText("server_cli.status.tor_disabled"))
	}

	healthy, health := queryInternalHealth(port, baseurl, fqdn)
	i2pState := "disabled"
	i2pProvider, i2pAddress := "none", ""
	if health != nil {
		i2pState = health.Features.I2P.State
		i2pProvider = health.Features.I2P.Provider
		i2pAddress = health.Features.I2P.Hostname
	}
	for _, line := range formatI2PStatusLines(i2pEnabled, i2pState, i2pProvider, i2pAddress) {
		fmt.Println(line)
	}

	// Per AI.md PART 8: healthy means running AND responding, and PART 13
	// defines "the internal health request made by --status" — query the
	// server's /server/healthz over loopback and exit on the response.
	if healthy {
		fmt.Println(serverCLIText("server_cli.status.health_healthy"))
		exitFunc(0)
	} else {
		fmt.Println(serverCLIText("server_cli.status.health_not_responding"))
		exitFunc(1)
	}
}

func formatI2PStatusLines(enabled bool, state, provider, address string) []string {
	if state == "" {
		if enabled {
			state = "no_provider"
		} else {
			state = "disabled"
		}
	}
	switch state {
	case "running":
		lines := []string{serverCLITf("server_cli.status.i2p_running", "provider", provider)}
		if address != "" {
			lines = append(lines, serverCLITf("server_cli.status.i2p_address", "address", address))
		}
		return lines
	case "disabled":
		return []string{serverCLIText("server_cli.status.i2p_disabled")}
	case "no_provider":
		return []string{serverCLIText("server_cli.status.i2p_no_provider")}
	case "starting", "building":
		return []string{serverCLIText("server_cli.status.i2p_starting")}
	default:
		return []string{serverCLIText("server_cli.status.i2p_error")}
	}
}

// queryInternalHealth performs the internal health request made by --status.
// It returns true only when the server answers /server/healthz with a 2xx
// status. The request goes over loopback, so the server certificate is
// verified against the configured FQDN (its actual subject) rather than
// 127.0.0.1. TLS verification is never disabled: a certificate-verification
// error means the TLS server answered, which satisfies "running and
// responding" per AI.md PART 8. A successful poll writes no server log
// entry (PART 13 log suppression).
type internalHealthStatus struct {
	Features struct {
		I2P struct {
			State    string `json:"state"`
			Provider string `json:"provider"`
			Hostname string `json:"hostname"`
		} `json:"i2p"`
	} `json:"features"`
}

func queryInternalHealth(port int, baseurl, fqdn string) (bool, *internalHealthStatus) {
	url := func(scheme string) string {
		return fmt.Sprintf("%s://127.0.0.1:%d%s/server/healthz", scheme, port, baseurl)
	}
	poll := func(client *http.Client, scheme string) (healthy, alive bool, status *internalHealthStatus) {
		req, err := http.NewRequest(http.MethodGet, url(scheme), nil)
		if err != nil {
			return false, false, nil
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Language", serverCLILanguage())
		resp, err := client.Do(req)
		if err != nil {
			// A certificate-verification error is proof the TLS server is
			// listening and completed a handshake with us.
			if strings.Contains(err.Error(), "certificate") ||
				strings.Contains(err.Error(), "x509") {
				return false, true, nil
			}
			return false, false, nil
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if readErr != nil {
			slog.Debug("failed to read server health response", "error", readErr)
		}
		closeutil.Close(resp.Body)
		var result internalHealthStatus
		if readErr == nil && json.Unmarshal(body, &result) == nil {
			status = &result
		}
		return resp.StatusCode >= 200 && resp.StatusCode < 300, true, status
	}

	// Plain HTTP first: dual-port mode always binds it, and non-SSL installs
	// have only it.
	httpClient := &http.Client{Timeout: 5 * time.Second}
	healthy, alive, status := poll(httpClient, "http")
	if healthy || alive {
		return healthy, status
	}

	// HTTPS with real verification against the FQDN.
	tlsClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: fqdn, MinVersion: tls.VersionTLS12},
		},
	}
	healthy, alive, status = poll(tlsClient, "https")
	if healthy {
		return true, status
	}
	return alive, status
}

// isProcessRunning checks if a process with given PID exists
func isProcessRunning(pid int) bool {
	if runtime.GOOS == "windows" {
		// On Windows, try to open the process
		process, err := os.FindProcess(pid)
		if err != nil {
			return false
		}
		// On Windows, FindProcess always succeeds, so we need another check
		// Try to send signal 0 (doesn't work on Windows, but process handle is valid)
		return process != nil
	}

	// On Unix, send signal 0 to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// getProcessStartTime attempts to get the start time of a process
func getProcessStartTime(pid int) time.Time {
	if runtime.GOOS == "linux" {
		// Read /proc/{pid}/stat to get start time
		statPath := fmt.Sprintf("/proc/%d/stat", pid)
		data, err := os.ReadFile(statPath)
		if err != nil {
			return time.Time{}
		}

		// Parse stat file - field 22 is starttime in clock ticks since boot
		fields := strings.Fields(string(data))
		if len(fields) < 22 {
			return time.Time{}
		}

		// Get system uptime
		uptimeData, err := os.ReadFile("/proc/uptime")
		if err != nil {
			return time.Time{}
		}
		uptimeFields := strings.Fields(string(uptimeData))
		if len(uptimeFields) == 0 {
			return time.Time{}
		}
		systemUptime, err := strconv.ParseFloat(uptimeFields[0], 64)
		if err != nil {
			return time.Time{}
		}

		// Parse process start time (in clock ticks)
		startTicks, err := strconv.ParseInt(fields[21], 10, 64)
		if err != nil {
			return time.Time{}
		}

		// Convert to seconds (assuming 100 Hz clock, sysconf(_SC_CLK_TCK) is usually 100)
		clkTck := int64(100)
		processStartSeconds := float64(startTicks) / float64(clkTck)

		// Calculate actual start time
		bootTime := time.Now().Add(-time.Duration(systemUptime * float64(time.Second)))
		return bootTime.Add(time.Duration(processStartSeconds * float64(time.Second)))
	}

	// For other platforms, return zero time (uptime will show as "unknown")
	return time.Time{}
}

// formatUptime formats a duration as human-readable uptime string
func formatUptime(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}

func runService(action string) {
	fmt.Print(display.Emoji("🔧", "[*]") + " " + serverCLITf("server_cli.main.service_heading", "action", action) + "\n\n")

	// Load configuration
	cfg, err := config.Initialize()
	if err != nil {
		fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_load_config", "error", err))
		exitFunc(1)
		return
	}

	// Create service manager
	sm := service.NewServiceManager(cfg)

	switch action {
	case "--install":
		// Per AI.md PART 23 Service Installation Logic: privileged → system
		// service; unprivileged → prompt via HandleEscalation (which re-execs
		// elevated or falls back), then a user service. HandleEscalation
		// returns nil both when already elevated and when the operator declined
		// or no escalation is available; execElevated replaces the process, so
		// a returned error means the elevation attempt itself failed.
		if !config.IsPrivileged() {
			fmt.Println(serverCLITf("server_cli.main.installing_service", "os", runtime.GOOS))
			if err := service.HandleEscalation(); err != nil {
				fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.privilege_escalation_failed", "error", err))
				exitFunc(1)
				return
			}
			if !config.IsPrivileged() {
				fmt.Println(serverCLIText("server_cli.main.installing_user_service"))
				if err := sm.InstallUserService(); err != nil {
					fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_install_user_service", "error", err))
					exitFunc(1)
					return
				}
				if err := sm.StartAllServices(); err != nil {
					fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_start_user_service", "error", err))
					exitFunc(1)
					return
				}
				fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.main.user_service_installed"))
				return
			}
		}
		fmt.Println(serverCLITf("server_cli.main.installing_service", "os", runtime.GOOS))
		if err := sm.Install(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_install_service", "error", err))
			exitFunc(1)
			return
		}
		if err := sm.StartAllServices(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_start_service", "error", err))
			exitFunc(1)
			return
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.main.service_installed"))

	case "--uninstall":
		// Per AI.md PART 23 Service Uninstall Logic: stop, disable, remove the
		// unit, delete ALL data (config/data/cache/log/backup dirs + PID file),
		// delete the system user and group, then point the operator at the
		// binary. Confirmation is required before the destructive action.
		if !config.IsPrivileged() {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.main.requires_elevated_privileges"))
			exitFunc(1)
			return
		}
		fmt.Print(i18n.TDefault("service.confirm_uninstall"))
		var answer string
		if _, err := fmt.Scanln(&answer); err != nil && !errors.Is(err, io.EOF) {
			slog.Warn("failed to read uninstall confirmation", "error", err)
		}
		answer = strings.TrimSpace(answer)
		if !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Println(i18n.TDefault("service.uninstall_cancelled"))
			return
		}
		fmt.Println(serverCLIText("server_cli.main.uninstalling_service"))
		if err := sm.Uninstall(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_uninstall_service", "error", err))
			exitFunc(1)
			return
		}
		// Account ownership is recorded in the configuration directory, so
		// remove an app-created service account before deleting that directory.
		var cleanupErrors []error
		if err := service.DeleteSystemUser(config.InternalName); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("%s: %w", serverCLIText("server_cli.main.delete_system_user"), err))
		}
		// Step 4: delete all data directories and the PID file.
		for _, dir := range []string{
			config.GetConfigDir(), config.GetDataDir(), config.GetCacheDir(),
			config.GetLogDir(), config.GetBackupDir(),
		} {
			if dir != "" {
				if err := os.RemoveAll(dir); err != nil {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("%s: %w", serverCLITf("server_cli.main.remove_directory", "path", dir), err))
				}
			}
		}
		if pidFile := config.GetPIDFile(); pidFile != "" {
			if err := os.Remove(pidFile); err != nil && !os.IsNotExist(err) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("%s: %w", serverCLITf("server_cli.main.remove_pid_file", "path", pidFile), err))
			}
		}
		if err := errors.Join(cleanupErrors...); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.service_uninstall_incomplete", "error", err))
			exitFunc(1)
			return
		}
		fmt.Println(i18n.TDefault("service.uninstalled_remove_binary", config.GetBinaryPath()))

	case "start":
		fmt.Println(serverCLIText("server_cli.main.starting_service"))
		if err := sm.StartAllServices(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_start_service", "error", err))
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.main.service_started"))

	case "stop":
		fmt.Println(serverCLIText("server_cli.main.stopping_service"))
		if err := sm.StopAllServices(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_stop_service", "error", err))
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.main.service_stopped"))

	case "status":
		status, err := sm.Status()
		if err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_service_status", "error", err))
			exitFunc(1)
		}
		fmt.Println(serverCLITf("server_cli.main.service_status", "status", status))

	case "restart":
		fmt.Println(serverCLIText("server_cli.main.restarting_service"))
		if err := sm.RestartAllServices(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_restart_service", "error", err))
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.main.service_restarted"))

	case "reload":
		fmt.Println(serverCLIText("server_cli.main.reloading_service"))
		if err := sm.Reload(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_reload_service", "error", err))
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.main.service_reloaded"))

	case "--disable":
		status, err := sm.Status()
		if err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_service_status", "error", err))
			exitFunc(1)
			return
		}
		if status == "active" || status == "running" || status == "started" {
			fmt.Println(serverCLIText("server_cli.main.stopping_service"))
			if err := sm.StopAllServices(); err != nil {
				fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_stop_service", "error", err))
				exitFunc(1)
				return
			}
		}
		fmt.Println(serverCLIText("server_cli.main.disabling_service"))
		if err := sm.Disable(); err != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.failed_disable_service", "error", err))
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " " + serverCLIText("server_cli.main.service_disabled"))

	case "--help":
		printServiceHelp()

	default:
		fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.main.unknown_service_action", "action", action))
		fmt.Println(serverCLIText("server_cli.main.valid_service_actions"))
	}
}

func printServiceHelp() {
	// Per AI.md PART 23 "Service Help Output": --install is what re-enables
	// a disabled service, so there is no separate --enable action.
	fmt.Println(serverCLIText("server_cli.main.service_help"))
}

// authorizeRestore implements the AI.md PART 21 restore authorization tiers:
// first-run (empty database) and root are always allowed; the service user
// must present a valid operator token; every other caller is denied. It is a
// pure function so the tiering can be unit tested without real root/user state.
func authorizeRestore(isFirstRun, isRoot, isServiceUser bool, expectedToken, presentedToken string) error {
	switch {
	case isFirstRun:
		return nil
	case isRoot:
		return nil
	case isServiceUser:
		if expectedToken == "" {
			return errors.New("no operator token is configured")
		}
		expectedSum := sha256.Sum256([]byte(expectedToken))
		presentedSum := sha256.Sum256([]byte(presentedToken))
		if presentedToken == "" || subtle.ConstantTimeCompare(expectedSum[:], presentedSum[:]) != 1 {
			return errors.New("invalid operator token")
		}
		return nil
	default:
		return errors.New("this operation requires root or the operator token")
	}
}

// readBackupPassword resolves an interactive restore password. Backup creation
// uses the configured secret and never prompts.
func printBackupEncryptionWarning() {
	fmt.Println("┌─────────────────────────────────────────────────────────────────────┐")
	fmt.Println("│  ⚠️  BACKUP ENCRYPTION NOT CONFIGURED                               │")
	fmt.Println("│                                                                      │")
	fmt.Println("│  Your backups are NOT encrypted. If someone gains access to your    │")
	fmt.Println("│  backup files, they can read all data including server configuration.│")
	fmt.Println("│                                                                      │")
	fmt.Println("│  Set server.backup.encryption_password in secrets.yml to fix this.  │")
	fmt.Println("└─────────────────────────────────────────────────────────────────────┘")
}

func readBackupPassword(prompt string) string {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return ""
	}
	fmt.Print(prompt)
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return ""
	}
	return string(passwordBytes)
}

// maintenanceArg returns os.Args[i] or "" when the operator omitted it, so a
// --maintenance subcommand can print its own help instead of panicking.
func maintenanceArg(i int) string {
	if len(os.Args) > i {
		return os.Args[i]
	}
	return ""
}

func maintenanceArgs(start int) []string {
	if len(os.Args) <= start {
		return nil
	}
	return append([]string(nil), os.Args[start:]...)
}

func runMaintenance(action string) {
	if action != "data" || !maintenanceDataWritesStdout(maintenanceArgs(4)) {
		fmt.Printf(display.Emoji("🔧", "[*]")+" Maintenance: %s\n\n", action)
	}

	bm := backup.NewManager()

	switch action {
	case "backup":
		backupArgs := maintenanceArgs(3)
		if len(backupArgs) > 0 && backupArgs[0] == "test" {
			if len(backupArgs) > 2 || (len(backupArgs) == 2 && strings.HasPrefix(backupArgs[1], "-")) {
				fmt.Println(display.Emoji("❌", "[ERROR]") + " Usage: search --maintenance backup test [backup-file]")
				exitFunc(1)
				return
			}
			filename := maintenanceArg(4)
			if err := runMaintenanceBackupTest(filename); err != nil {
				fmt.Printf(display.Emoji("❌", "[ERROR]")+" Backup test failed: %v\n", err)
				exitFunc(1)
				return
			}
			fmt.Println(display.Emoji("✅", "[OK]") + " Backup test passed")
			return
		}
		backupOptions, optionsErr := parseMaintenanceBackupOptions(backupArgs)
		if optionsErr != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" %v\n", optionsErr)
			fmt.Println("Usage: search --maintenance backup [filename] [--include-ssl] [--include-data]")
			exitFunc(1)
			return
		}
		backupOptions.IncludeSSL = backupOptions.IncludeSSL || flagBackupIncludeSSL
		backupOptions.IncludeData = backupOptions.IncludeData || flagBackupIncludeData
		filename := backupOptions.Filename

		// Per AI.md PART 21: configured backup key material enables encryption;
		// backup creation never prompts.
		cfg, cfgErr := maintenanceBackupConfig(config.Initialize)
		if cfgErr != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Could not load configuration for backup: %v\n", cfgErr)
			exitFunc(1)
			return
		}
		dbMgr, dbErr := database.NewDatabaseManager(&database.Config{
			Driver:   "sqlite",
			DataDir:  config.GetDatabaseDir(),
			MaxOpen:  10,
			MaxIdle:  5,
			Lifetime: 300,
		})
		if dbErr != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Could not open server database for backup: %v\n", dbErr)
			exitFunc(1)
			return
		}
		defer func() {
			if err := dbMgr.Close(); err != nil {
				slog.Warn("failed to close maintenance backup database", "err", err)
			}
		}()
		if err := database.InitSchema(context.Background(), dbMgr); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Could not initialize server database for backup: %v\n", err)
			exitFunc(1)
			return
		}
		bm.SetServerDatabaseSnapshotter(dbMgr.ServerDB())
		bm.SetIncludeSSL(backupOptions.IncludeSSL)
		bm.SetIncludeData(backupOptions.IncludeData)
		complianceEnabled := cfg.Server.Compliance.AnyEnabled()
		password := cfg.Server.Backup.EncryptionPassword
		if complianceEnabled && password == "" {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " Compliance mode requires backup encryption")
			fmt.Println("   Set server.backup.encryption_password in secrets.yml, and try again.")
			exitFunc(1)
			return
		}
		if password != "" {
			fmt.Println(display.Emoji("🔐", "[ENCRYPTED]") + " Backup encryption: ENABLED")
			bm.SetPassword(password)
		} else {
			fmt.Println(display.Emoji("🔓", "[PLAIN]") + " Backup encryption: DISABLED")
			printBackupEncryptionWarning()
		}

		fmt.Println("Creating backup...")
		// Per AI.md PART 21: every backup must pass the full verification
		// checklist immediately after creation — a failed backup is deleted
		// and any existing backups are preserved.
		var backupPath string
		var verifyResult *backup.VerificationResult
		var err error
		if password != "" {
			backupPath, verifyResult, err = bm.CreateEncryptedAndVerify(filename)
		} else {
			backupPath, verifyResult, err = bm.CreateAndVerify(filename)
		}
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Backup failed: %v\n", err)
			exitFunc(1)
			return
		}
		fmt.Printf(display.Emoji("✅", "[OK]")+" Backup created and verified: %s\n", backupPath)
		if verifyResult != nil {
			fmt.Printf("   Verified: file=%v size=%v checksum=%v manifest=%v decrypt=%v\n",
				verifyResult.FileExists, verifyResult.SizeValid, verifyResult.ChecksumValid,
				verifyResult.ManifestValid, verifyResult.DecryptValid)
		}

		// Show backup info. GetMetadata reads the manifest via a plain gzip
		// reader, so it cannot be used on an encrypted (.enc) archive.
		if !backup.IsEncrypted(backupPath) {
			metadata, err := bm.GetMetadata(backupPath)
			if err == nil {
				fmt.Printf("   Version: %s\n", metadata.Version)
				fmt.Printf("   Files:   %d\n", len(metadata.Contents))
				fmt.Println("   Encrypted: NO")
			}
		} else {
			fmt.Println("   Encrypted: YES (AES-256-GCM)")
		}

	case "restore":
		if len(os.Args) < 4 {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " Please specify backup file to restore")
			fmt.Println("Usage: search --maintenance restore <backup-file>")
			fmt.Println()

			// List available backups
			backups, _ := bm.List()
			if len(backups) > 0 {
				fmt.Println("Available backups:")
				for _, b := range backups {
					fmt.Printf("  - %s (%s, %s)\n", b.Filename, b.FormatSize(), b.CreatedAt.Format("2006-01-02 15:04"))
				}
			}
			return
		}
		filename := os.Args[3]

		// Per AI.md PART 21 Restore Authorization: restore is destructive and requires
		// authorization — database empty (first-run) or root is allowed; the service
		// user needs the operator token (server.token); any other user is denied.
		authCfg, err := config.Initialize()
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load config: %v\n", err)
			exitFunc(1)
			return
		}
		restoreLimits, limitsErr := backup.RestoreLimitsFromConfig(authCfg.Server.Restore)
		if limitsErr != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Invalid restore limits: %v\n", limitsErr)
			exitFunc(1)
			return
		}
		bm.SetRestoreLimits(restoreLimits)
		isFirstRun := authCfg.IsFirstRun()
		isRoot := config.IsPrivileged()
		isServiceUser := false
		if currentUser, userErr := user.Current(); userErr == nil {
			isServiceUser = currentUser.Username == "search"
		}

		// Only the service-user tier requires an interactive prompt; the other
		// tiers are decided without any further input.
		var presentedToken string
		if isServiceUser && !isFirstRun && !isRoot {
			presentedToken = readBackupPassword("Enter operator token: ")
		}

		if authErr := authorizeRestore(isFirstRun, isRoot, isServiceUser, authCfg.Server.Token, presentedToken); authErr != nil {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " Restore denied: " + authErr.Error())
			exitFunc(1)
			return
		}

		fmt.Printf("Restoring from: %s\n", filename)

		// Per AI.md PART 21: restore prompts for an encrypted archive password;
		// the configured backup password is never used to restore.
		isEncryptedFile := backup.IsEncrypted(filename)
		var password string
		if isEncryptedFile {
			password = readBackupPassword("Enter backup password: ")
			if password == "" {
				fmt.Println(display.Emoji("❌", "[ERROR]") + " This backup is encrypted — a password is required")
				fmt.Println("   Enter the backup password when prompted, and try again.")
				exitFunc(1)
				return
			}
			bm.SetPassword(password)
		}

		// Per AI.md PART 21: every check in the restore verification checklist
		// must pass before the restore proceeds.
		fmt.Println("Verifying backup integrity...")
		verifyResult, err := bm.VerifyBackup(filename)
		if err != nil || verifyResult == nil || !verifyResult.AllPassed {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " Backup verification failed")
			if err != nil {
				fmt.Printf("   %v\n", err)
			}
			if verifyResult != nil {
				for _, verifyErr := range verifyResult.Errors {
					fmt.Printf("   - %s\n", verifyErr)
				}
			}
			exitFunc(1)
			return
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Verifying backup integrity... OK")

		// Confirm restore
		fmt.Print("This will overwrite current configuration. Continue? (yes/no): ")
		var confirm string
		if _, err := fmt.Scanln(&confirm); err != nil && !errors.Is(err, io.EOF) {
			slog.Warn("failed to read restore confirmation", "error", err)
		}
		if confirm != "yes" {
			fmt.Println("Restore cancelled.")
			return
		}
		restoreDB, dbErr := database.NewDatabaseManager(&database.Config{
			Driver:   "sqlite",
			DataDir:  config.GetDatabaseDir(),
			MaxOpen:  10,
			MaxIdle:  5,
			Lifetime: 300,
		})
		if dbErr != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Could not open server database for pre-restore backup: %v\n", dbErr)
			exitFunc(1)
			return
		}
		bm.SetServerDatabaseSnapshotter(restoreDB.ServerDB())
		bm.SetBeforeRestore(func() error {
			err := restoreDB.Close()
			restoreDB = nil
			return err
		})
		defer func() {
			if restoreDB != nil {
				if err := restoreDB.Close(); err != nil {
					slog.Warn("failed to close restore database", "err", err)
				}
			}
		}()

		fmt.Println("Restoring...")
		if isEncryptedFile {
			err = bm.RestoreEncrypted(filename)
		} else {
			err = bm.Restore(filename)
		}
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Restore failed: %v\n", err)
			exitFunc(1)
			return
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Restore completed successfully")
		fmt.Println("   Please restart the server to apply changes.")

	// Per AI.md PART 22: --maintenance update [cmd] is an alias for
	// --update [cmd] and accepts the same subcommands, defaulting to "yes"
	case "update":
		sub := "yes"
		if len(os.Args) > 3 {
			sub = os.Args[3]
		}
		runUpdate(sub)

	case "mode":
		fmt.Println("Toggling maintenance mode...")
		cfg, err := config.Initialize()
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load config: %v\n", err)
			exitFunc(1)
			return
		}
		// Toggle maintenance mode
		cfg.Server.MaintenanceMode = !cfg.Server.MaintenanceMode
		if err := cfg.Save(config.GetConfigPath()); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save config: %v\n", err)
			exitFunc(1)
		}
		if cfg.Server.MaintenanceMode {
			fmt.Println(display.Emoji("✅", "[OK]") + " Maintenance mode: ENABLED")
			fmt.Println("   The server will show a maintenance page to users")
		} else {
			fmt.Println(display.Emoji("✅", "[OK]") + " Maintenance mode: DISABLED")
			fmt.Println("   The server is now accepting normal requests")
		}

	// Per AI.md PART 11 "Secret Rotation" (AI.md:14034):
	// --maintenance secret rotate {installation_secret|encryption_key}
	case "secret":
		runMaintenanceSecret(maintenanceArg(3), maintenanceArg(4))

	// Per AI.md PART 11 (AI.md:11864): --maintenance token list|revoke <prefix>
	case "token":
		runMaintenanceToken(maintenanceArg(3), maintenanceArg(4))

	// Per AI.md PART 12: owner-scoped data export and deletion use --owner.
	case "data":
		runMaintenanceData(maintenanceArg(3), maintenanceArgs(4))

	// Per AI.md PART 12 "Compliance Routes": --maintenance compliance report
	case "compliance":
		runMaintenanceCompliance(maintenanceArg(3))

	case "security":
		runMaintenanceSecurity(maintenanceArg(3), maintenanceArgs(4))

	case "email":
		runMaintenanceEmail(maintenanceArg(3), maintenanceArgs(4))

	case "scheduler":
		runMaintenanceScheduler(maintenanceArg(3), maintenanceArg(4))
	case "tor":
		runTor(maintenanceArgs(3))

	case "setup":
		// Per AI.md PART 8: --maintenance setup resets server configuration to defaults
		// Authorization: first-run only OR root
		fmt.Println(display.Emoji("🔧", "[*]") + " Setup / Reset Configuration")
		fmt.Println()

		// Check authorization
		cfg, err := config.Initialize()
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load config: %v\n", err)
			exitFunc(1)
			return
		}

		isFirstRun := cfg.IsFirstRun()
		isRoot := config.IsPrivileged()

		if !isFirstRun && !isRoot {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " Setup already completed.")
			fmt.Println()
			fmt.Println("To reconfigure:")
			fmt.Println("  1. Edit server.yml directly and restart the server")
			fmt.Println("  2. Run as root: sudo search --maintenance setup")
			exitFunc(1)
			return
		}

		if !isFirstRun {
			// Root user re-running setup - confirm
			fmt.Println(display.Emoji("⚠️", "[WARN]") + " This will reset configuration to defaults!")
			fmt.Print("Type 'RESET' to confirm: ")
			var confirm string
			if _, err := fmt.Scanln(&confirm); err != nil && !errors.Is(err, io.EOF) {
				slog.Warn("failed to read restore confirmation", "error", err)
			}
			if confirm != "RESET" {
				fmt.Println("Cancelled.")
				return
			}
		}

		// Reset to defaults
		newCfg := config.DefaultConfig()
		// Preserve token if regenerating (operator must have it)
		if !isFirstRun {
			newCfg.Server.Token = cfg.Server.Token
		}
		if err := newCfg.Save(config.GetConfigPath()); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to save config: %v\n", err)
			exitFunc(1)
			return
		}

		fmt.Println(display.Emoji("✅", "[OK]") + " Configuration reset to defaults")
		fmt.Println("   Config: " + config.GetConfigPath())

	case "pgp":
		// Per AI.md PART 8 and SECURITY.txt spec: PGP keypair management
		// Subcommands: generate, rotate, publish, export, import, delete
		pgpAction := ""
		if len(os.Args) > 3 {
			pgpAction = os.Args[3]
		}

		switch pgpAction {
		case "generate":
			runPGPGenerate()

		case "rotate":
			runPGPRotate()

		case "publish":
			runPGPPublish()

		case "export":
			keyType := ""
			if len(os.Args) > 4 {
				keyType = os.Args[4]
			}
			exportPath := ""
			if len(os.Args) > 5 {
				exportPath = os.Args[5]
			}
			switch keyType {
			case "public":
				runPGPExportPublic(exportPath)
			case "private":
				runPGPExportPrivate(exportPath)
			default:
				fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLIText("server_cli.pgp.export_key_type_required"))
				fmt.Println(serverCLITf("server_cli.pgp.export_usage", "project", config.ProjectName))
			}

		case "import":
			importFile := ""
			if len(os.Args) > 4 {
				importFile = os.Args[4]
			}
			runPGPImport(importFile)

		case "delete":
			runPGPDelete()

		case "help", "--help", "":
			fmt.Println(serverCLITf("server_cli.pgp.help", "project", config.ProjectName, "config_dir", config.GetConfigDir()))

		default:
			fmt.Println(display.Emoji("❌", "[ERROR]") + " " + serverCLITf("server_cli.pgp.unknown_action", "action", pgpAction))
			fmt.Println(serverCLIText("server_cli.pgp.valid_actions"))
		}

	// Help text follows AI.md PART 8 "Maintenance Help Output" (AI.md:31554)
	// verbatim, extended with the pgp/secret/token/data/compliance actions
	// that the frozen --maintenance flag list (AI.md:10204) also carries.
	case "--help":
		binaryName := filepath.Base(os.Args[0])
		fmt.Println("Maintenance commands:")
		fmt.Println()
		fmt.Println("backup [file] [--include-ssl] [--include-data] - Create a backup")
		fmt.Println("                                        Optional: include config SSL files and data files")
		fmt.Println("                                        Default: " + config.GetBackupDir() + "/" + binaryName + "_backup_YYYY-MM-DD_HHMMSS.tar.gz")
		fmt.Println()
		fmt.Println("restore <file>                        - Restore from backup file")
		fmt.Println("                                        Stops server, restores data, restarts server")
		fmt.Println()
		fmt.Println("update [cmd]                          - Manage updates")
		fmt.Println("  check                               - Check for available updates")
		fmt.Println("  yes                                 - Download and install update")
		fmt.Println("  branch <name>                       - Switch update branch (stable|beta|devel; aliases: daily, rolling, dev, development)")
		fmt.Println()
		fmt.Println("mode <mode>                           - Set application mode")
		fmt.Println("  production                          - Normal operation (default)")
		fmt.Println("  development                         - Verbose logging (debug endpoints require the debug flag)")
		fmt.Println()
		fmt.Println("setup                                 - Reset server configuration (first-run or root only)")
		fmt.Println()
		fmt.Println("pgp <action>                          - PGP keypair management (run 'pgp help' for details)")
		fmt.Println()
		fmt.Println("security list|show|export|update       - Manage coordinated-disclosure reports")
		fmt.Println()
		fmt.Println("secret rotate <name>                  - Rotate installation_secret or encryption_key")
		fmt.Println()
		fmt.Println("token list|revoke <prefix>            - List or revoke API tokens")
		fmt.Println()
		fmt.Println("data export --owner <prefix> [--output FILE] - Export one owner's data")
		fmt.Println("data delete --owner <prefix> [--yes]   - Delete one owner and its resources")
		fmt.Println()
		fmt.Println("compliance report                     - Print a compliance summary")
		fmt.Println("email test [--to ADDRESS] [--dry-run]  - Send or preview email templates")
		fmt.Println("scheduler list|show|run|enable|disable|history - Manage scheduled tasks")
		fmt.Println("tor <action>                           - Manage Tor hidden service")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  " + binaryName + " --maintenance backup")
		fmt.Println("  " + binaryName + " --maintenance backup --include-ssl --include-data")
		fmt.Println("  " + binaryName + " --maintenance backup /path/to/backup.tar.gz")
		fmt.Println("  " + binaryName + " --maintenance restore /path/to/backup.tar.gz")
		fmt.Println("  " + binaryName + " --maintenance update check")
		fmt.Println("  " + binaryName + " --maintenance update yes")
		fmt.Println("  " + binaryName + " --maintenance mode development")
		fmt.Println("  " + binaryName + " --maintenance setup")

	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown action: %s\n", action)
		fmt.Println("Valid actions: backup, restore, update, mode, setup, pgp, security, secret, token, data, compliance, email, scheduler, tor, help")
	}
}

// updateBranchArg returns the value following the "branch" subcommand token on
// the command line. It scans os.Args so both invocation forms work:
// "--update branch beta" and the "--maintenance update branch beta" alias.
func updateBranchArg() string {
	for i, arg := range os.Args {
		if arg == "branch" && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

// configuredUpdateBranch returns the release channel from server.yml.
// The config file is the single source of truth for the channel per AI.md
// PART 22; an unreadable config falls back to the stable default.
func configuredUpdateBranch() string {
	cfg, err := config.Load(config.GetConfigPath())
	if err != nil {
		return update.BranchStable
	}
	return update.NormalizeBranch(cfg.Server.Update.Branch)
}

func runUpdate(subCmd string) {
	fmt.Println(display.Emoji("🔄", "[UPDATE]") + " Update Management")
	fmt.Println()

	um := update.NewManager()

	switch subCmd {
	case "check":
		branch := configuredUpdateBranch()
		fmt.Println("Checking for updates...")
		fmt.Printf("Current version: %s\n", config.Version)
		fmt.Printf("Update channel:  %s\n", branch)
		fmt.Println()

		// Per AI.md PART 22: a manual check always sees the true latest
		// release; defer_days gates only the scheduled update_check task
		info, err := um.CheckForUpdates(branch)
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to check for updates: %v\n", err)
			exitFunc(1)
			return
		}

		if !info.Available {
			fmt.Println(display.Emoji("✅", "[OK]") + " You are running the latest version")
		} else {
			fmt.Printf(display.Emoji("🆕", "[NEW]")+" New version available: %s\n", info.LatestVersion)
			fmt.Printf("   Published: %s\n", info.PublishedAt.Format("Jan 2, 2006"))
			if info.AssetSize > 0 {
				fmt.Printf("   Size: %.1f MB\n", float64(info.AssetSize)/(1024*1024))
			}
			fmt.Println()
			fmt.Println("Release Notes:")
			fmt.Println(info.ReleaseNotes)
			fmt.Println()
			fmt.Println("Run 'search --update yes' to install the update")
		}

	case "yes", "":
		if !config.IsPrivileged() {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " Update installation requires elevated privileges")
			fmt.Println("   Use --update check to check without privileges")
			exitFunc(1)
			return
		}

		branch := configuredUpdateBranch()
		fmt.Println("Checking for updates...")
		fmt.Printf("Update channel:  %s\n", branch)
		info, err := um.CheckForUpdates(branch)
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to check for updates: %v\n", err)
			exitFunc(1)
			return
		}

		if !info.Available {
			fmt.Println(display.Emoji("✅", "[OK]") + " You are running the latest version")
			return
		}

		if info.DownloadURL == "" {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Release %s has no binary for %s/%s\n",
				info.LatestVersion, runtime.GOOS, runtime.GOARCH)
			exitFunc(1)
			return
		}

		fmt.Printf("Downloading version %s...\n", info.LatestVersion)

		downloadPath, err := um.DownloadUpdate(info.DownloadURL, func(downloaded, total int64) {
			if total > 0 {
				pct := float64(downloaded) / float64(total) * 100
				fmt.Printf("\r   Progress: %.1f%%", pct)
			}
		})
		fmt.Println()

		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Download failed: %v\n", err)
			exitFunc(1)
			return
		}

		fmt.Println("Installing update...")
		if err := um.InstallUpdate(downloadPath, info.AssetName, info.ChecksumURL); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Installation failed: %v\n", err)
			exitFunc(1)
			return
		}

		fmt.Println(display.Emoji("✅", "[OK]") + " Update installed successfully!")

		// Per AI.md PART 17 "Default Templates" -> update_installed and the
		// Operator Notifications matrix ("Update installed | INFO | ✓ |
		// Important change record"): notify admins on successful self-update.
		if cfg, cfgErr := config.Load(config.GetConfigPath()); cfgErr == nil {
			if mailer, ok := email.NewMailerFromConfig(cfg); ok {
				if sendErr := mailer.SendUpdateInstalled(config.Version, info.LatestVersion); sendErr != nil {
					slog.Error("failed to send update installed notification email", "err", sendErr)
				}
			}
		}

		// Per AI.md PART 22 "Service-Aware Update": the new binary only takes
		// effect once the running instance restarts. A supervised instance
		// restarts through its service manager; an unsupervised instance
		// re-executes itself so the CLI applies the update immediately.
		fmt.Println("Restarting...")
		if err := update.Restart(); err != nil {
			fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Restart failed: %v\n", err)
			fmt.Println("   Restart the service or running instance manually to apply the update")
			exitFunc(1)
			return
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Restarted on the new version")

	case "branch":
		branch := updateBranchArg()
		if branch == "" {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " Please specify branch name")
			fmt.Println("Usage: search --update branch <stable|beta|devel>")
			exitFunc(1)
			return
		}
		if !update.IsValidBranch(branch) {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Invalid branch: %s\n", branch)
			fmt.Println("Valid branches: stable, beta, devel (aliases: daily, rolling, dev, development)")
			exitFunc(1)
			return
		}

		// Per AI.md PART 22: the config file is the single source of truth for
		// the channel; there is no separate CLI-side state
		configPath := config.GetConfigPath()
		cfg, err := config.Load(configPath)
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load %s: %v\n", configPath, err)
			exitFunc(1)
			return
		}
		branch = update.NormalizeBranch(branch)
		cfg.Server.Update.Branch = branch
		if err := cfg.Save(configPath); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to write %s: %v\n", configPath, err)
			exitFunc(1)
			return
		}

		descriptions := map[string]string{
			"stable": "stable (full releases only)",
			"beta":   "beta (beta pre-releases plus all stable releases)",
			"devel":  "devel (nightly build plus beta and stable releases)",
		}
		fmt.Printf(display.Emoji("✅", "[OK]")+" Update branch set to: %s\n", descriptions[branch])
		fmt.Printf("   Written to %s (server.update.branch)\n", configPath)

	case "help", "--help":
		fmt.Println("Update Management Commands:")
		fmt.Println()
		fmt.Println("  check              Check for available updates")
		fmt.Println("  yes                Download and install update")
		fmt.Println("  branch <name>      Set update branch (stable|beta|devel; aliases: daily, rolling, dev, development)")
		fmt.Println("  help               Show this help")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  search --update check")
		fmt.Println("  search --update yes")
		fmt.Println("  search --update branch beta")

	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown subcommand: %s\n", subCmd)
		fmt.Println("Valid subcommands: check, yes, branch <stable|beta|devel>, help")
	}
}

func generateSetupToken() (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := cryptoRand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("read crypto/rand: %w", err)
	}
	return hex.EncodeToString(randomBytes), nil
}

// buildListenURLs builds the list of URLs the server is listening on
func buildListenURLs(cfg *config.Config) []string {
	var urls []string

	// Get listen address
	addr := cfg.Server.Address
	if addr == "" {
		addr = "0.0.0.0"
	}

	// Primary HTTP URL - use GetHTTPPort which resolves port 0 based on environment
	// Per AI.md PART 27: Container defaults to port 80, local dev to random 64xxx
	port := cfg.Server.GetHTTPPort()

	// Use localhost for display if binding to all interfaces
	displayAddr := addr
	if addr == "0.0.0.0" || addr == "" || addr == "::" {
		displayAddr = "localhost"
	}

	urls = append(urls, fmt.Sprintf("http://%s:%d", displayAddr, port))

	// Add HTTPS if configured
	if cfg.Server.SSL.Enabled && cfg.Server.HTTPSPort > 0 {
		urls = append(urls, fmt.Sprintf("https://%s:%d", displayAddr, cfg.Server.HTTPSPort))
	}

	// Add Tor onion address if available
	if cfg.Server.Tor.Enabled && cfg.Server.Tor.OnionAddress != "" {
		urls = append(urls, fmt.Sprintf("http://%s", cfg.Server.Tor.OnionAddress))
	}

	return urls
}

// ============================================================
// ============================================================
// Daemonize (per AI.md PART 8)
// ============================================================

func daemonize() error {
	// Windows doesn't support traditional Unix daemonization
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "Warning: --daemon is not supported on Windows")
		fmt.Fprintln(os.Stderr, "Use --service --install && --service start for Windows Service")
		// Continue in foreground on Windows
		return nil
	}

	// Already daemonized? Check if parent is init (PID 1)
	if os.Getppid() == 1 {
		return nil
	}

	// Check if we're already a daemon child
	if os.Getenv("_DAEMON_CHILD") == "1" {
		return nil
	}

	// Get executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Build command with same args (minus --daemon to prevent loop)
	args := filterDaemonFlag(os.Args[1:])

	cmd := exec.Command(execPath, args...)
	cmd.Env = append(os.Environ(), "_DAEMON_CHILD=1")

	// Detach from parent's file descriptors
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	// Start child process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting daemon: %w", err)
	}

	// Parent exits, child continues
	fmt.Printf("Daemon started with PID %d\n", cmd.Process.Pid)
	exitFunc(0)
	return nil
}

// filterDaemonFlag removes --daemon from args to prevent infinite loop
// Per AI.md PART 8: Only -h and -v may have short flags, so no -d
func filterDaemonFlag(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--daemon" {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

// ============================================================
// Shell Integration (per AI.md PART 8)
// ============================================================

// runShell handles shell integration commands
// Per AI.md PART 8: --shell completions [SHELL], --shell init [SHELL]
func runShell(subCmd string) {
	binaryName := filepath.Base(os.Args[0])

	// Determine shell type
	shell := detectShell()
	if len(os.Args) > 3 {
		shell = os.Args[3]
	}

	switch subCmd {
	case "completions":
		printCompletions(binaryName, shell)
	case "init":
		printShellInit(binaryName, shell)
	case "help", "--help":
		printShellHelp(binaryName)
	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown shell subcommand: %s\n", subCmd)
		fmt.Println("Valid subcommands: completions, init, --help")
		exitFunc(1)
	}
}

// detectShell detects the current shell from environment
func detectShell() string {
	// Check SHELL environment variable
	shell := os.Getenv("SHELL")
	if shell != "" {
		shell = filepath.Base(shell)
		switch shell {
		case "bash", "zsh", "fish", "powershell", "pwsh":
			return shell
		}
	}

	// Check parent process on Unix
	if runtime.GOOS != "windows" {
		// Default to bash
		return "bash"
	}

	// Windows default
	return "powershell"
}

// printCompletions prints shell completions script
func printCompletions(binaryName, shell string) {
	switch shell {
	case "bash":
		fmt.Printf(`# Bash completions for %s
# Add to ~/.bashrc: eval "$(%s --shell init bash)"

_%s_completions() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    if [[ "${COMP_WORDS[1]}" == "--maintenance" ]]; then
        case "${COMP_WORDS[2]}" in
            email)
                case "${prev}" in
                    email) COMPREPLY=( $(compgen -W "test" -- "${cur}") ); return 0 ;;
                    test) COMPREPLY=( $(compgen -W "--to --dry-run" -- "${cur}") ); return 0 ;;
                esac
                ;;
            backup)
                COMPREPLY=( $(compgen -W "--include-ssl --include-data test" -- "${cur}") )
                return 0
                ;;
            scheduler)
                if [[ "${prev}" == "scheduler" ]]; then
                    COMPREPLY=( $(compgen -W "list show run enable disable history" -- "${cur}") ); return 0
                fi
                ;;
            security)
                case "${prev}" in
                    security) COMPREPLY=( $(compgen -W "list show export update" -- "${cur}") ); return 0 ;;
                    list) COMPREPLY=( $(compgen -W "--status" -- "${cur}") ); return 0 ;;
                    export) COMPREPLY=( $(compgen -W "--output" -- "${cur}") ); return 0 ;;
                    update) COMPREPLY=( $(compgen -W "--status --comment --disclosure-date --severity" -- "${cur}") ); return 0 ;;
                    --status) COMPREPLY=( $(compgen -W "received triaged confirmed patching disclosed wont_fix" -- "${cur}") ); return 0 ;;
                    --severity) COMPREPLY=( $(compgen -W "critical high medium low informational" -- "${cur}") ); return 0 ;;
                esac
                ;;
            tor)
                case "${prev}" in
                    tor) COMPREPLY=( $(compgen -W "status validate restart regenerate vanity import-keys --help" -- "${cur}") ); return 0 ;;
                    vanity) COMPREPLY=( $(compgen -W "start stop apply" -- "${cur}") ); return 0 ;;
                esac
                ;;
        esac
    fi

    opts="--help --version --status --daemon --debug --i2p"
    opts="$opts --mode --config --data --cache --log --backup --pid --address --port"
    opts="$opts --service --maintenance --update --shell"

    case "${prev}" in
        vanity)
            COMPREPLY=( $(compgen -W "start apply" -- ${cur}) )
            return 0
            ;;
        --service)
            COMPREPLY=( $(compgen -W "start stop restart reload status --install --uninstall --disable --help" -- ${cur}) )
            return 0
            ;;
        --maintenance)
            COMPREPLY=( $(compgen -W "backup restore update mode setup pgp secret token data compliance email scheduler tor help" -- ${cur}) )
            return 0
            ;;
        --update)
            COMPREPLY=( $(compgen -W "check yes branch" -- ${cur}) )
            return 0
            ;;
        --shell)
            COMPREPLY=( $(compgen -W "completions init --help" -- ${cur}) )
            return 0
            ;;
        --mode)
            COMPREPLY=( $(compgen -W "production development" -- ${cur}) )
            return 0
            ;;
        --config|--data|--cache|--log|--backup)
            COMPREPLY=( $(compgen -d -- ${cur}) )
            return 0
            ;;
        --pid)
            COMPREPLY=( $(compgen -f -- ${cur}) )
            return 0
            ;;
    esac

    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
}
complete -F _%s_completions %s
`, binaryName, binaryName, binaryName, binaryName, binaryName)

	case "zsh":
		fmt.Printf(`#compdef %s
# Zsh completions for %s
# Add to ~/.zshrc: eval "$(%s --shell init zsh)"

_%s() {
    local -a commands
    local -a opts

    opts=(
        '--help[Show help message]'
        '-h[Show help message]'
        '--version[Show version]'
        '-v[Show version]'
        '--status[Show server status]'
        '--daemon[Run as daemon]'
        '--debug[Enable debug mode]'
        '--mode[Application mode]:mode:(production development)'
        '--config[Config directory]:directory:_files -/'
        '--data[Data directory]:directory:_files -/'
        '--cache[Cache directory]:directory:_files -/'
        '--log[Log directory]:directory:_files -/'
        '--backup[Backup directory]:directory:_files -/'
        '--pid[PID file]:file:_files'
        '--address[Listen address]:address:'
        '--port[Listen port(s), HTTP or HTTP,HTTPS]:port:'
        '--i2p[Enable the I2P eepsite (default on in application containers)]'
        '--service[Service management]:action:(start stop restart reload status --install --uninstall --disable --help)'
        '--maintenance[Maintenance]:action:(backup restore update mode setup pgp security secret token data compliance email scheduler tor help)'
        '--update[Update management]:action:(check yes branch)'
        '--shell[Shell integration]:subcommand:(completions init --help)'
    )

    if [[ $words[2] == --maintenance && $words[3] == backup ]]; then
        opts+=( '--include-ssl[Include config SSL files]' '--include-data[Include data files]' 'test[Verify a backup without restoring]' )
    elif [[ $words[2] == tor && $words[3] == vanity ]]; then
        opts+=( 'start[Start vanity search]' 'stop[Stop vanity search]' 'apply[Apply a found candidate]' )
    elif [[ $words[2] == tor ]]; then
        opts+=(
            'status[View Tor hidden service status]'
            'validate[Validate Tor configuration]'
            'restart[Restart the Tor process]'
            'regenerate[Regenerate the .onion address]'
            'vanity[Manage vanity address search]'
            'import-keys[Import an existing Tor key directory]'
            '--help[Show Tor maintenance help]'
        )
    elif [[ $words[2] == --maintenance && $words[3] == security && $words[4] == list ]]; then
        opts+=( '--status[Filter by status]:status:(received triaged confirmed patching disclosed wont_fix)' )
    elif [[ $words[2] == --maintenance && $words[3] == security && $words[4] == export ]]; then
        opts+=( '--output[Output path or - for stdout]:file:_files' )
    elif [[ $words[2] == --maintenance && $words[3] == security && $words[4] == update ]]; then
        opts+=( '--status[Set status]:status:(received triaged confirmed patching disclosed wont_fix)' '--comment[Set researcher-visible comment]:comment:' '--disclosure-date[Set expected disclosure date]:date:' '--severity[Set severity]:severity:(critical high medium low informational)' )
    elif [[ $words[2] == --maintenance && $words[3] == security ]]; then
		opts+=( 'list[List coordinated-disclosure reports]' 'show[Show report metadata]' 'export[Export encrypted report]' 'update[Update report triage]' )
    fi

    _arguments -s $opts
}

compdef _%s %s
`, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName)

	case "fish":
		fmt.Print(strings.ReplaceAll(`# Fish completions for %s
# Add to ~/.config/fish/config.fish: %s --shell init fish | source

complete -c %s -f
complete -c %s -s h -l help -d 'Show help message'
complete -c %s -s v -l version -d 'Show version'
complete -c %s -l status -d 'Show server status'
complete -c %s -l daemon -d 'Run as daemon'
complete -c %s -l debug -d 'Enable debug mode'
complete -c %s -l mode -d 'Application mode' -xa 'production development'
complete -c %s -l config -d 'Config directory' -xa '(__fish_complete_directories)'
complete -c %s -l data -d 'Data directory' -xa '(__fish_complete_directories)'
complete -c %s -l cache -d 'Cache directory' -xa '(__fish_complete_directories)'
complete -c %s -l log -d 'Log directory' -xa '(__fish_complete_directories)'
complete -c %s -l backup -d 'Backup directory' -xa '(__fish_complete_directories)'
complete -c %s -l pid -d 'PID file'
complete -c %s -l address -d 'Listen address'
complete -c %s -l port -d 'Listen port(s), HTTP or HTTP,HTTPS'
complete -c %s -l i2p -d 'Enable the I2P eepsite (default on in application containers)'
complete -c %s -l service -d 'Service management' -xa 'start stop restart reload status --install --uninstall --disable --help'
complete -c %s -l maintenance -d 'Maintenance' -xa 'backup restore update mode setup pgp security secret token data compliance email scheduler tor help'
complete -c %s -n '__fish_seen_subcommand_from backup' -l include-ssl -d 'Include optional config SSL files'
complete -c %s -n '__fish_seen_subcommand_from backup' -l include-data -d 'Include optional data files'
complete -c %s -l update -d 'Update management' -xa 'check yes branch'
complete -c %s -l shell -d 'Shell integration' -xa 'completions init --help'
complete -c %s -n '__fish_seen_subcommand_from tor' -a 'status validate restart regenerate vanity import-keys --help' -d 'Tor maintenance action'
complete -c %s -n '__fish_seen_subcommand_from vanity' -a 'start stop apply' -d 'Tor vanity action'
complete -c %s -n '__fish_seen_subcommand_from security' -a 'list show export update' -d 'Security report action'
complete -c %s -n '__fish_seen_subcommand_from security' -l status -d 'Filter or set report status' -xa 'received triaged confirmed patching disclosed wont_fix'
complete -c %s -n '__fish_seen_subcommand_from security' -l output -d 'Encrypted report output path'
complete -c %s -n '__fish_seen_subcommand_from security' -l comment -d 'Researcher-visible report comment'
complete -c %s -n '__fish_seen_subcommand_from security' -l disclosure-date -d 'Expected disclosure date'
complete -c %s -n '__fish_seen_subcommand_from security' -l severity -d 'Report severity' -xa 'critical high medium low informational'
`, "%s", binaryName))

	case "powershell", "pwsh":
		fmt.Printf(`# PowerShell completions for %s
# Add to $PROFILE: Invoke-Expression (&%s --shell init powershell)

Register-ArgumentCompleter -CommandName %s -ScriptBlock {
    param($commandName, $wordToComplete, $cursorPosition, $commandAst, $fakeBoundParameters)

    $commands = @(
        @{Name='--help'; Description='Show help message'}
        @{Name='-h'; Description='Show help message'}
        @{Name='--version'; Description='Show version'}
        @{Name='-v'; Description='Show version'}
        @{Name='--status'; Description='Show server status'}
        @{Name='--daemon'; Description='Run as daemon'}
        @{Name='--debug'; Description='Enable debug mode'}
        @{Name='--mode'; Description='Application mode'}
        @{Name='--config'; Description='Config directory'}
        @{Name='--data'; Description='Data directory'}
        @{Name='--cache'; Description='Cache directory'}
        @{Name='--log'; Description='Log directory'}
        @{Name='--backup'; Description='Backup directory'}
        @{Name='--pid'; Description='PID file'}
        @{Name='--address'; Description='Listen address'}
        @{Name='--port'; Description='Listen port(s), HTTP or HTTP,HTTPS'}
        @{Name='--i2p'; Description='Enable the I2P eepsite (default on in application containers)'}
        @{Name='--service'; Description='Service management'}
        @{Name='--maintenance'; Description='Maintenance'}
        @{Name='--update'; Description='Update management'}
        @{Name='--shell'; Description='Shell integration'}
    )

    if ($commandAst.ToString() -match '--maintenance backup\\s*$') {
        $commands = @(
            @{Name='--include-ssl'; Description='Include optional config SSL files'}
            @{Name='--include-data'; Description='Include optional data files'}
            @{Name='test'; Description='Verify a backup without restoring it'}
        )
    } elseif ($commandAst.ToString() -match '--maintenance tor vanity\\s*$') {
        $commands = @(
            @{Name='start'; Description='Start vanity address search'}
            @{Name='stop'; Description='Stop vanity address search'}
            @{Name='apply'; Description='Apply a found vanity candidate'}
        )
    } elseif ($commandAst.ToString() -match '--maintenance tor\\s*$') {
        $commands = @(
            @{Name='status'; Description='View Tor hidden service status'}
            @{Name='validate'; Description='Validate Tor configuration'}
            @{Name='restart'; Description='Restart the Tor process'}
            @{Name='regenerate'; Description='Regenerate the .onion address'}
            @{Name='vanity'; Description='Manage vanity address search'}
            @{Name='import-keys'; Description='Import a Tor key directory'}
            @{Name='--help'; Description='Show Tor maintenance help'}
        )
    }

    $commands | Where-Object { $_.Name -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_.Name, $_.Name, 'ParameterValue', $_.Description)
    }
}
`, binaryName, binaryName, binaryName)

	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unsupported shell: %s\n", shell)
		fmt.Println("Supported shells: bash, zsh, fish, powershell")
		exitFunc(1)
	}
}

// printShellInit prints the shell initialization command
func printShellInit(binaryName, shell string) {
	switch shell {
	case "bash":
		fmt.Printf("source <(%s --shell completions bash)\n", binaryName)
	case "zsh":
		fmt.Printf("source <(%s --shell completions zsh)\n", binaryName)
	case "fish":
		fmt.Printf("%s --shell completions fish | source\n", binaryName)
	case "powershell", "pwsh":
		fmt.Printf("Invoke-Expression (&%s --shell completions powershell)\n", binaryName)
	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unsupported shell: %s\n", shell)
		fmt.Println("Supported shells: bash, zsh, fish, powershell")
		exitFunc(1)
	}
}

// printShellHelp prints shell integration help
func printShellHelp(binaryName string) {
	fmt.Printf(`Shell Integration for %s

Usage:
  %s --shell completions [SHELL]   Print shell completions script
  %s --shell init [SHELL]          Print shell init command for eval
  %s --shell --help                Show this help

Supported Shells:
  bash        Bash shell (default on Linux)
  zsh         Zsh shell (default on macOS)
  fish        Fish shell
  powershell  PowerShell (Windows)

Setup Instructions:

  Bash (~/.bashrc):
    eval "$(%s --shell init bash)"

  Zsh (~/.zshrc):
    eval "$(%s --shell init zsh)"

  Fish (~/.config/fish/config.fish):
    %s --shell init fish | source

  PowerShell ($PROFILE):
    Invoke-Expression (&%s --shell init powershell)

The shell will be auto-detected if not specified.
`, binaryName, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName, binaryName)
}
