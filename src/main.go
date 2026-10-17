package main

import (
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
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
	"github.com/apimgr/search/src/config"
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
	flagPort    int
	flagBaseURL string
	flagColor   string
	flagLang    string
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

	// Commands with optional arguments
	flag.StringVar(&flagService, "service", "", "Service management: start|stop|restart|reload|status|--install|--uninstall|--disable|--help")
	flag.StringVar(&flagMaintenance, "maintenance", "", "Maintenance: backup|restore|update|mode|setup|pgp|secret|token|data|compliance|--help")
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
	flag.IntVar(&flagPort, "port", 0, "Set listen port")
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

	// Parse flags
	flag.Parse()

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
	case len(os.Args) > 1 && os.Args[1] == "email":
		runEmail(os.Args[2:])
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
		if err := os.Setenv("SEARCH_MODE", flagMode); err != nil {
			slog.Warn("failed to set SEARCH_MODE", "error", err)
		}
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
		if err := os.Setenv("SEARCH_DEBUG", "true"); err != nil {
			slog.Warn("failed to set SEARCH_DEBUG", "error", err)
		}
		mode.SetDebugEnabled(true)
	}

	if flagData != "" {
		if err := os.Setenv("SEARCH_DATA_DIR", flagData); err != nil {
			slog.Warn("failed to set SEARCH_DATA_DIR", "error", err)
		}
		config.SetDataDirOverride(flagData)
	}
	if flagConfig != "" {
		if err := os.Setenv("SEARCH_CONFIG_DIR", flagConfig); err != nil {
			slog.Warn("failed to set SEARCH_CONFIG_DIR", "error", err)
		}
		config.SetConfigDirOverride(flagConfig)
	}
	if flagCache != "" {
		if err := os.Setenv("SEARCH_CACHE_DIR", flagCache); err != nil {
			slog.Warn("failed to set SEARCH_CACHE_DIR", "error", err)
		}
		config.SetCacheDirOverride(flagCache)
	}
	if flagLog != "" {
		if err := os.Setenv("SEARCH_LOG_DIR", flagLog); err != nil {
			slog.Warn("failed to set SEARCH_LOG_DIR", "error", err)
		}
		config.SetLogDirOverride(flagLog)
	}
	if flagBackup != "" {
		if err := os.Setenv("SEARCH_BACKUP_DIR", flagBackup); err != nil {
			slog.Warn("failed to set SEARCH_BACKUP_DIR", "error", err)
		}
		config.SetBackupDirOverride(flagBackup)
	}
	if flagPID != "" {
		if err := os.Setenv("SEARCH_PID_FILE", flagPID); err != nil {
			slog.Warn("failed to set SEARCH_PID_FILE", "error", err)
		}
		config.SetPIDFileOverride(flagPID)
	}
	if flagAddress != "" {
		if err := os.Setenv("SEARCH_ADDRESS", flagAddress); err != nil {
			slog.Warn("failed to set SEARCH_ADDRESS", "error", err)
		}
	}
	if flagPort != 0 {
		if err := os.Setenv("SEARCH_PORT", fmt.Sprintf("%d", flagPort)); err != nil {
			slog.Warn("failed to set SEARCH_PORT", "error", err)
		}
		if err := os.Setenv("PORT", fmt.Sprintf("%d", flagPort)); err != nil {
			slog.Warn("failed to set PORT", "error", err)
		}
	}
	// Per AI.md PART 6: --baseurl sets URL path prefix for reverse proxy
	if flagBaseURL != "" {
		if err := os.Setenv("SEARCH_BASE_URL", flagBaseURL); err != nil {
			slog.Warn("failed to set SEARCH_BASE_URL", "error", err)
		}
		config.SetBaseURLOverride(flagBaseURL)
	}
	// Per AI.md PART 8: Color priority: CLI flag > Config > NO_COLOR env > Auto-detect
	if flagColor != "" {
		if err := os.Setenv("SEARCH_COLOR", flagColor); err != nil {
			slog.Warn("failed to set SEARCH_COLOR", "error", err)
		}
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
			slog.Error("Missing maintenance subcommand", "usage", "search --maintenance {backup|restore|update|mode|setup|pgp|secret|token|data|compliance|--help}")
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
	startupConfig           *config.Config
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
	tlsEnabled := cfg.Server.SSL.Enabled || cfg.Server.SSL.LetsEncrypt.Enabled
	if cfg.Server.IsDualPortMode() && tlsEnabled {
		ports = append(ports, cfg.Server.GetHTTPSPort())
	}
	var redirectPort int
	if tlsEnabled && cfg.Server.SSL.AutoTLS && !cfg.Server.IsDualPortMode() {
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
		addr := net.JoinHostPort(cfg.Server.Address, fmt.Sprint(port))
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			closeListeners()
			return nil, nil, fmt.Errorf("bind %s: %w", addr, err)
		}
		listeners[port] = listener
	}

	var redirectListener net.Listener
	if redirectPort > 0 {
		addr := net.JoinHostPort(cfg.Server.Address, fmt.Sprint(redirectPort))
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			closeListeners()
			return nil, nil, fmt.Errorf("bind AutoTLS redirect %s: %w", addr, err)
		}
		redirectListener = listener
	}
	return listeners, redirectListener, nil
}

func runServerServiceBody() {
	// Handle daemonization per AI.md PART 6
	// Check if we should daemonize (only for manual starts, not --service start)
	if flagDaemon && os.Getenv("_DAEMON_CHILD") != "1" {
		if err := daemonize(); err != nil {
			slog.Error("Failed to daemonize", "err", err)
			exitFunc(1)
		}
		// Parent exits in daemonize(), only child continues
	}

	// Per AI.md PART 8: If running as root, setup system resources then drop privileges
	// Skip privilege dropping in container mode - container entrypoint handles this
	if service.IsRunningAsRoot() && !config.IsRunningInContainer() {
		slog.Info("Running as root - performing privileged setup")

		// Step 8a: Create system user
		svcUser, err := service.CreateSystemUser(config.InternalName)
		if err != nil {
			slog.Error("Failed to create system user", "err", err)
			exitFunc(1)
		}
		slog.Info("System user ready", "user", svcUser.Name, "uid", svcUser.UID, "gid", svcUser.GID)

		// Step 8b-d: Create directories and set ownership (while still root)
		if err := config.EnsureSystemDirectories("search"); err != nil {
			slog.Error("Failed to create system directories", "err", err)
			exitFunc(1)
		}
		slog.Info("System directories created")

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
		if flagData == "" {
			if err := os.Setenv("SEARCH_DATA_DIR", "/var/lib/apimgr/search"); err != nil {
				slog.Warn("failed to set service SEARCH_DATA_DIR", "error", err)
			}
		}
		if flagConfig == "" {
			if err := os.Setenv("SEARCH_CONFIG_DIR", "/etc/apimgr/search"); err != nil {
				slog.Warn("failed to set service SEARCH_CONFIG_DIR", "error", err)
			}
		}
		if flagLog == "" {
			if err := os.Setenv("SEARCH_LOG_DIR", "/var/log/apimgr/search"); err != nil {
				slog.Warn("failed to set service SEARCH_LOG_DIR", "error", err)
			}
		}
		if flagCache == "" {
			if err := os.Setenv("SEARCH_CACHE_DIR", "/var/cache/apimgr/search"); err != nil {
				slog.Warn("failed to set service SEARCH_CACHE_DIR", "error", err)
			}
		}
		if err := os.Setenv("HOME", config.GetDataDir()); err != nil {
			slog.Warn("failed to set service HOME", "error", err)
		}

		// Resolve the persisted runtime configuration before binding privileged
		// sockets so CLI, environment, and server.yml port rules stay identical.
		cfg, err := config.Initialize()
		if err != nil {
			slog.Error("Configuration failed", "err", err)
			exitFunc(1)
			return
		}
		if err := config.ChownRuntimeDirectories("search"); err != nil {
			slog.Error("Failed to set runtime directory ownership", "err", err)
			exitFunc(1)
			return
		}
		listeners, redirectListener, err := bindPrivilegedListeners(cfg)
		if err != nil {
			slog.Error("Failed to bind privileged ports", "err", err)
			exitFunc(1)
			return
		}
		startupConfig = cfg
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

	// Configuration initialization creates the runtime directories, including
	// the parent directory of the PID file. It must complete before PID handling
	// so fresh installs and containers can start with empty data volumes.
	cfg := startupConfig
	if cfg == nil {
		var err error
		cfg, err = config.Initialize()
		if err != nil {
			slog.Error("Configuration failed", "err", err)
			exitFunc(1)
			return
		}
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

	// On first run, display the auto-generated operator token (server.token)
	// so the operator can save it. The token lives in server.yml and is the
	// only credential the application has — there is no admin web UI.
	var setupToken string
	showSetup := cfg.IsFirstRun()
	if showSetup {
		setupToken = cfg.Server.Token
	}

	// Create server (initialises DB, scheduler, etc. — steps 13-16 per AI.md PART 8)
	srv := server.NewServer(cfg)
	srv.SetPreboundListeners(startupListeners, startupRedirectListener)
	startupConfig = nil
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
	if cfg.Server.Scheduler.Tasks.BlocklistUpdate.Enabled {
		srv.StartBlocklistUpdateIfMissing()
	}
	if cfg.Server.Scheduler.Tasks.CVEUpdate.Enabled {
		srv.StartCVEUpdateIfMissing()
	}

	// Step 20: Print responsive startup banner per AI.md PART 7 and PART 8.
	// Now the server is listening; Tor address (if any) is available from srv.TorAddress().
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

	banner.Print(banner.Config{
		AppName:    "Search",
		Version:    config.Version,
		Mode:       cfg.Server.Mode,
		Debug:      mode.IsDebugEnabled(),
		URLs:       urls,
		ShowSetup:  showSetup,
		SetupToken: setupToken,
	})

	// Step 21: Block until graceful shutdown completes.
	// Per AI.md PART 7: os.Exit() belongs only in main().
	<-shutdownDone
	<-serverErrCh
	exitFunc(0)
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

	fmt.Printf("%s %s\n", binaryName, config.Version)
	fmt.Printf("Built: %s\n", config.BuildDate)
	fmt.Printf("Go: %s\n", runtime.Version())
	fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

func printHelp() {
	// Per AI.md PART 6: Use actual binary name in help
	binaryName := filepath.Base(os.Args[0])

	fmt.Printf(`%s - Privacy-Respecting Metasearch Engine

Usage:
  %s [options]             Start the server with optional flags
  %s [command]             Execute a command

Runtime Flags:
  --mode <mode>            Set application mode (production|development|debug)
  --data <dir>             Set data directory
  --config <dir>           Set config directory
  --cache <dir>            Set cache directory
  --log <dir>              Set log directory
  --backup <dir>           Set backup directory
  --pid <file>             Set PID file path
  --address <addr>         Set listen address
  --port <port>            Set listen port
  --baseurl <path>         Set URL path prefix for reverse proxy (default: /)
  --color <mode>           Set color output mode (auto|yes|no)
  --lang <code>            Set language for output (default: auto, from LANG env)
  --daemon                 Daemonize (detach from terminal, Unix only)
  --debug                  Enable debug mode (verbose logging, debug endpoints)

Information:
  --help, -h               Show this help message
  --version, -v            Show version information
  --status                 Show server status and health

Shell Integration:
  --shell completions [SHELL]  Print shell completions script
  --shell init [SHELL]         Print shell init command for eval
  --shell --help               Show shell help

Service Management:
  --service <action>       Service management (requires privileges):
    --install              Install, enable, and start system service
    --uninstall            Stop, disable, and remove system service
    start                  Start the service
    stop                   Stop the service
    status                 Check service status
    restart                Restart the service
    reload                 Reload configuration (SIGHUP)
    --disable              Stop and disable service autostart
    --help                 Show service commands and current status

Maintenance:
  --maintenance <action>   Maintenance commands:
    backup [file]          Create backup archive
                           Use BACKUP_PASSWORD env var for encryption
    restore <file>         Restore from backup
                           Use BACKUP_PASSWORD env var if encrypted
    update                 Alias for --update yes
    mode                   Toggle maintenance mode
    setup                  Reset configuration to defaults
    pgp <action>           PGP keypair management (generate/export/import)
    secret rotate <name>   Rotate an installation or encryption secret
    token list|revoke <prefix> Manage API tokens
    data export|delete <subject> GDPR/CCPA data access and erasure
    compliance report     Print a compliance summary
    --help                 Show maintenance commands

Updates:
  --update [subcommand]    Update management:
    check                  Check for available updates
    yes                    Download and install update (default)
    branch <name>          Set update branch (stable|beta|devel; aliases: daily, rolling, dev, development)

Tor Hidden Service:
  tor <subcommand>         Tor hidden service management (talks to a running
                           server over its loopback control endpoint):
    status                 View Tor hidden service status
    validate               Validate Tor configuration
    restart                Restart the Tor process
    regenerate             Regenerate the .onion address
    vanity start <prefix>  Start vanity address search
    vanity apply           Apply the found vanity address
    import-keys <path>     Import an existing Tor key file

Email:
  email test <address>     Send a test email to verify SMTP configuration

Environment Variables:
  SEARCH_SETTINGS_PATH     Path to configuration file
  SEARCH_CONFIG_DIR        Configuration directory
  SEARCH_DATA_DIR          Data directory
  SEARCH_LOG_DIR           Log directory
  DEBUG, SEARCH_DEBUG      Enable debug mode (0/1, true/false)
  SECRET_KEY               Secret key for in-memory caches (not sessions)
  PORT, SEARCH_PORT        Server port
  MODE, SEARCH_MODE        Application mode (production|development|debug)
  INSTANCE_NAME            Instance display name
  SEARCH_COLOR             Color output mode (always|never|auto)
  NO_COLOR                 Disable colors when set (standard)
  DISABLE_TOR              Disable Tor (auto-enabled if tor binary installed)
  BACKUP_PASSWORD          Password for backup encryption (AES-256-GCM)

Examples:
  %s                                 Start server with defaults
  %s --port 8080                     Start on port 8080
  %s --mode development              Start in dev mode
  %s --config /etc/search --data /var/lib/search  Custom directories
  %s --service --install             Install as system service
  %s --service reload                Reload configuration
  %s --update check                  Check for updates

For more information: https://github.com/apimgr/search
`, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName)
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
		fmt.Println("Server Status: Running")
	} else {
		fmt.Println("Server Status: Not Running")
		fmt.Println()
		fmt.Printf("Start the server with: %s\n", binaryName)
		fmt.Printf("Or install as service: %s --service --install\n", binaryName)
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
	var baseurl string
	var fqdn string

	configPath := config.GetConfigPath()
	if cfg, err := config.Load(configPath); err == nil {
		port = cfg.Server.Port
		mode = cfg.Server.Mode
		torEnabled = cfg.Server.Tor.Enabled
		torAddress = cfg.Server.Tor.OnionAddress
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

	fmt.Printf("  PID: %d\n", pid)
	fmt.Printf("  Port: %d\n", port)
	fmt.Printf("  Mode: %s\n", mode)
	fmt.Printf("  Uptime: %s\n", uptime)
	fmt.Println()

	// Instance mode (single-instance per AI.md line 2055)
	fmt.Println("Mode: standalone")
	fmt.Println()

	// Tor Hidden Service status
	if torEnabled {
		if torAddress != "" {
			fmt.Println("Tor Hidden Service: Connected")
			// Truncate address for display
			displayAddr := torAddress
			if len(torAddress) > 16 {
				displayAddr = torAddress[:8] + "..." + torAddress[len(torAddress)-10:]
			}
			fmt.Printf("  Address: %s\n", displayAddr)
		} else {
			fmt.Println("Tor Hidden Service: Enabled (waiting for address)")
		}
	} else {
		fmt.Println("Tor Hidden Service: Disabled")
	}

	// Per AI.md PART 8: healthy means running AND responding, and PART 13
	// defines "the internal health request made by --status" — query the
	// server's /server/healthz over loopback and exit on the response.
	healthy := queryInternalHealth(port, baseurl, fqdn)
	if healthy {
		fmt.Println("Health: Healthy")
		exitFunc(0)
	} else {
		fmt.Println("Health: Not Responding")
		exitFunc(1)
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
func queryInternalHealth(port int, baseurl, fqdn string) bool {
	url := func(scheme string) string {
		return fmt.Sprintf("%s://127.0.0.1:%d%s/server/healthz", scheme, port, baseurl)
	}
	poll := func(client *http.Client, scheme string) (healthy, alive bool) {
		resp, err := client.Get(url(scheme))
		if err != nil {
			// A certificate-verification error is proof the TLS server is
			// listening and completed a handshake with us.
			if strings.Contains(err.Error(), "certificate") ||
				strings.Contains(err.Error(), "x509") {
				return false, true
			}
			return false, false
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			slog.Debug("failed to drain server health response", "error", err)
		}
		closeutil.Close(resp.Body)
		return resp.StatusCode >= 200 && resp.StatusCode < 300, true
	}

	// Plain HTTP first: dual-port mode always binds it, and non-SSL installs
	// have only it.
	httpClient := &http.Client{Timeout: 5 * time.Second}
	healthy, alive := poll(httpClient, "http")
	if healthy || alive {
		return healthy
	}

	// HTTPS with real verification against the FQDN.
	tlsClient := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: fqdn, MinVersion: tls.VersionTLS12},
		},
	}
	healthy, alive = poll(tlsClient, "https")
	if healthy {
		return true
	}
	return alive
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

func printFirstRunOperatorToken(cfg *config.Config) {
	if cfg == nil || !cfg.IsFirstRun() || cfg.Server.Token == "" {
		return
	}
	fmt.Printf("First-run operator token (save it now; it will not be shown again): %s\n", cfg.Server.Token)
}

func runService(action string) {
	fmt.Printf(display.Emoji("🔧", "[*]")+" Service Management: %s\n\n", action)

	// Load configuration
	cfg, err := config.Initialize()
	if err != nil {
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to load config: %v\n", err)
		exitFunc(1)
		return
	}

	// Service management can initialize first-run configuration before the
	// server process starts, so display its one-time operator token here too.
	printFirstRunOperatorToken(cfg)

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
			fmt.Printf("Installing service for %s...\n", runtime.GOOS)
			if err := service.HandleEscalation(); err != nil {
				fmt.Printf(display.Emoji("❌", "[ERROR]")+" Privilege escalation failed: %v\n", err)
				exitFunc(1)
				return
			}
			if !config.IsPrivileged() {
				fmt.Println("Installing user service (no admin privileges)...")
				if err := sm.InstallUserService(); err != nil {
					fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to install user service: %v\n", err)
					exitFunc(1)
					return
				}
				if err := sm.StartAllServices(); err != nil {
					fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to start user service: %v\n", err)
				}
				fmt.Println(display.Emoji("✅", "[OK]") + " User service installed and started")
				return
			}
		}
		fmt.Printf("Installing service for %s...\n", runtime.GOOS)
		if err := sm.Install(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to install service: %v\n", err)
			exitFunc(1)
			return
		}
		if err := sm.StartAllServices(); err != nil {
			fmt.Printf(display.Emoji("⚠️", "[WARN]")+" Failed to start service: %v\n", err)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Service installed and started")

	case "--uninstall":
		// Per AI.md PART 23 Service Uninstall Logic: stop, disable, remove the
		// unit, delete ALL data (config/data/cache/log/backup dirs + PID file),
		// delete the system user and group, then point the operator at the
		// binary. Confirmation is required before the destructive action.
		if !config.IsPrivileged() {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " This command requires elevated privileges")
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
		fmt.Println("Uninstalling service...")
		if err := sm.Uninstall(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to uninstall service: %v\n", err)
			exitFunc(1)
			return
		}
		// Step 4: delete all data directories and the PID file.
		for _, dir := range []string{
			config.GetConfigDir(), config.GetDataDir(), config.GetCacheDir(),
			config.GetLogDir(), config.GetBackupDir(),
		} {
			if dir != "" {
				if err := os.RemoveAll(dir); err != nil {
					slog.Warn("Failed to remove directory during uninstall", "dir", dir, "err", err)
				}
			}
		}
		if pidFile := config.GetPIDFile(); pidFile != "" {
			os.Remove(pidFile) //nolint:errcheck
		}
		// Step 5: delete the system user and group created by the server.
		if err := service.DeleteSystemUser(config.InternalName); err != nil {
			slog.Warn("Failed to delete system user during uninstall", "err", err)
		}
		fmt.Println(i18n.TDefault("service.uninstalled_remove_binary", config.GetBinaryPath()))

	case "start":
		if !config.IsPrivileged() {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " This command requires elevated privileges")
			exitFunc(1)
		}
		fmt.Println("Starting service...")
		if err := sm.StartAllServices(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to start service: %v\n", err)
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Service started successfully")

	case "stop":
		if !config.IsPrivileged() {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " This command requires elevated privileges")
			exitFunc(1)
		}
		fmt.Println("Stopping service...")
		if err := sm.StopAllServices(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to stop service: %v\n", err)
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Service stopped successfully")

	case "status":
		status, err := sm.Status()
		if err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to get service status: %v\n", err)
			exitFunc(1)
		}
		fmt.Printf("Service status: %s\n", status)

	case "restart":
		if !config.IsPrivileged() {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " This command requires elevated privileges")
			exitFunc(1)
		}
		fmt.Println("Restarting service...")
		if err := sm.RestartAllServices(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to restart service: %v\n", err)
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Service restarted successfully")

	case "reload":
		fmt.Println("Reloading service configuration...")
		if err := sm.Reload(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to reload service: %v\n", err)
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Service configuration reloaded")

	case "--disable":
		if !config.IsPrivileged() {
			fmt.Println(display.Emoji("❌", "[ERROR]") + " This command requires elevated privileges")
			exitFunc(1)
		}
		fmt.Println("Disabling service autostart...")
		if err := sm.Disable(); err != nil {
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Failed to disable service: %v\n", err)
			exitFunc(1)
		}
		fmt.Println(display.Emoji("✅", "[OK]") + " Service autostart disabled")

	case "--help":
		// Per AI.md PART 23 "Service Help Output": --install is what re-enables
		// a disabled service, so there is no separate --enable action
		fmt.Println("Service management commands:")
		fmt.Println()
		fmt.Println("  start         Start the service")
		fmt.Println("  stop          Stop the service")
		fmt.Println("  restart       Restart the service")
		fmt.Println("  reload        Reload configuration without restart")
		fmt.Println("  status        Show service status")
		fmt.Println()
		fmt.Println("  --install     Install, enable, and start service")
		fmt.Println("  --disable     Stop and disable service (keeps data)")
		fmt.Println("  --uninstall   Stop, disable, and remove everything (keeps binary)")
		fmt.Println("  --help        Show this help")

	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown action: %s\n", action)
		fmt.Println("Valid actions: start, stop, restart, reload, status, --install, --uninstall, --disable, --help")
	}
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

// readBackupPassword resolves the backup encryption password for CLI use.
// Per AI.md PART 21: the CLI has no password flag (a flag would leak the
// password via shell history and process lists) — BACKUP_PASSWORD remains
// honored for scripted/non-interactive use, and the CLI otherwise falls back
// to the documented interactive masked prompt ("Enter backup password:").
func readBackupPassword(prompt string) string {
	if password := os.Getenv("BACKUP_PASSWORD"); password != "" {
		return password
	}
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

func runMaintenance(action string) {
	fmt.Printf(display.Emoji("🔧", "[*]")+" Maintenance: %s\n\n", action)

	bm := backup.NewManager()

	switch action {
	case "backup":
		filename := ""
		if len(os.Args) > 3 {
			filename = os.Args[3]
		}

		// Per AI.md PART 21: encryption is required under compliance mode and
		// otherwise applies only if a backup password was configured during
		// setup (server.backup.encryption.enabled) — the password itself is
		// never stored and is resolved via BACKUP_PASSWORD or an interactive
		// prompt.
		cfg, cfgErr := config.Initialize()
		complianceEnabled := cfgErr == nil && cfg.Server.Compliance.Enabled
		encryptionEnabled := (cfgErr == nil && cfg.Server.Backup.Encryption.Enabled) || complianceEnabled

		var password string
		if encryptionEnabled {
			password = readBackupPassword("Enter backup password: ")
			if password == "" && complianceEnabled {
				fmt.Println(display.Emoji("❌", "[ERROR]") + " Compliance mode requires backup encryption")
				fmt.Println("   Set BACKUP_PASSWORD or enter a password when prompted, and try again.")
				exitFunc(1)
				return
			}
		}
		if password != "" {
			fmt.Println(display.Emoji("🔐", "[ENCRYPTED]") + " Backup encryption: ENABLED")
			bm.SetPassword(password)
		} else {
			fmt.Println(display.Emoji("🔓", "[PLAIN]") + " Backup encryption: DISABLED (set BACKUP_PASSWORD to enable)")
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

		// Per AI.md PART 21: the CLI prompts "Enter backup password:" when the
		// backup is encrypted and no password was provided via BACKUP_PASSWORD.
		isEncryptedFile := backup.IsEncrypted(filename)
		var password string
		if isEncryptedFile {
			password = readBackupPassword("Enter backup password: ")
			if password == "" {
				fmt.Println(display.Emoji("❌", "[ERROR]") + " This backup is encrypted — a password is required")
				fmt.Println("   Set BACKUP_PASSWORD or enter a password when prompted, and try again.")
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

	// Per AI.md PART 12 GDPR/CCPA: --maintenance data export|delete <subject>
	case "data":
		runMaintenanceData(maintenanceArg(3), maintenanceArg(4))

	// Per AI.md PART 12 "Compliance Routes": --maintenance compliance report
	case "compliance":
		runMaintenanceCompliance(maintenanceArg(3))

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
				fmt.Println(display.Emoji("❌", "[ERROR]") + " Please specify key type: public or private")
				fmt.Println("Usage: search --maintenance pgp export <public|private> [path]")
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
			fmt.Println("PGP Keypair Management:")
			fmt.Println()
			fmt.Println("  generate              Generate a new Ed25519/Curve25519 keypair")
			fmt.Println("  rotate                Rotate keypair (signs new with old)")
			fmt.Println("  publish               Publish public key to configured keyservers")
			fmt.Println("  export public [path]  Export public key to stdout or file")
			fmt.Println("  export private <path> Export private key (requires confirmation)")
			fmt.Println("  import <file>         Import existing keypair from file")
			fmt.Println("  delete                Delete keypair (requires confirmation)")
			fmt.Println()
			fmt.Println("Keys are stored in: " + config.GetConfigDir() + "/security/")
			fmt.Println("Public key served at: /.well-known/pgp-key.asc")

		default:
			fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown PGP action: %s\n", pgpAction)
			fmt.Println("Valid actions: generate, rotate, publish, export, import, delete, help")
		}

	// Help text follows AI.md PART 8 "Maintenance Help Output" (AI.md:31554)
	// verbatim, extended with the pgp/secret/token/data/compliance actions
	// that the frozen --maintenance flag list (AI.md:10204) also carries.
	case "--help":
		binaryName := filepath.Base(os.Args[0])
		fmt.Println("Maintenance commands:")
		fmt.Println()
		fmt.Println("backup [file]                         - Create backup of all data")
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
		fmt.Println("secret rotate <name>                  - Rotate installation_secret or encryption_key")
		fmt.Println()
		fmt.Println("token list|revoke <prefix>            - List or revoke API tokens")
		fmt.Println()
		fmt.Println("data export|delete <subject>          - GDPR/CCPA personal data export or erasure")
		fmt.Println()
		fmt.Println("compliance report                     - Print a compliance summary")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  " + binaryName + " --maintenance backup")
		fmt.Println("  " + binaryName + " --maintenance backup /path/to/backup.tar.gz")
		fmt.Println("  " + binaryName + " --maintenance restore /path/to/backup.tar.gz")
		fmt.Println("  " + binaryName + " --maintenance update check")
		fmt.Println("  " + binaryName + " --maintenance update yes")
		fmt.Println("  " + binaryName + " --maintenance mode development")
		fmt.Println("  " + binaryName + " --maintenance setup")

	default:
		fmt.Printf(display.Emoji("❌", "[ERROR]")+" Unknown action: %s\n", action)
		fmt.Println("Valid actions: backup, restore, update, mode, setup, pgp, secret, token, data, compliance, help")
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

    opts="--help --version --status --daemon --debug"
    opts="$opts --mode --config --data --cache --log --backup --pid --address --port"
    opts="$opts --service --maintenance --update --shell tor email"

    case "${prev}" in
        tor)
            COMPREPLY=( $(compgen -W "status validate restart regenerate vanity import-keys" -- ${cur}) )
            return 0
            ;;
        vanity)
            COMPREPLY=( $(compgen -W "start apply" -- ${cur}) )
            return 0
            ;;
        email)
            COMPREPLY=( $(compgen -W "test" -- ${cur}) )
            return 0
            ;;
        --service)
            COMPREPLY=( $(compgen -W "start stop restart reload status --install --uninstall --disable --help" -- ${cur}) )
            return 0
            ;;
        --maintenance)
            COMPREPLY=( $(compgen -W "backup restore update mode setup pgp secret token data compliance help" -- ${cur}) )
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
        '--port[Listen port]:port:'
        '--service[Service management]:action:(start stop restart reload status --install --uninstall --disable --help)'
        '--maintenance[Maintenance]:action:(backup restore update mode setup pgp secret token data compliance help)'
        '--update[Update management]:action:(check yes branch)'
        '--shell[Shell integration]:subcommand:(completions init --help)'
        'tor[Tor hidden service management]:subcommand:(status validate restart regenerate vanity import-keys)'
        'email[Email management]:subcommand:(test)'
    )

    _arguments -s $opts
}

compdef _%s %s
`, binaryName, binaryName, binaryName, binaryName, binaryName, binaryName)

	case "fish":
		fmt.Printf(`# Fish completions for %s
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
complete -c %s -l port -d 'Listen port'
complete -c %s -l service -d 'Service management' -xa 'start stop restart reload status --install --uninstall --disable --help'
complete -c %s -l maintenance -d 'Maintenance' -xa 'backup restore update mode setup pgp secret token data compliance help'
complete -c %s -l update -d 'Update management' -xa 'check yes branch'
complete -c %s -l shell -d 'Shell integration' -xa 'completions init --help'
complete -c %s -n '__fish_use_subcommand' -a 'tor' -d 'Tor hidden service management' -xa 'status validate restart regenerate vanity import-keys'
complete -c %s -n '__fish_use_subcommand' -a 'email' -d 'Email management' -xa 'test'
`, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName)

	case "powershell", "pwsh":
		fmt.Printf(`# PowerShell completions for %s
# Add to $PROFILE: Invoke-Expression (&%s --shell init powershell)

Register-ArgumentCompleter -CommandName %s -ScriptBlock {
    param($commandName, $wordToComplete, $cursorPosition)

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
        @{Name='--port'; Description='Listen port'}
        @{Name='--service'; Description='Service management'}
        @{Name='--maintenance'; Description='Maintenance'}
        @{Name='--update'; Description='Update management'}
        @{Name='--shell'; Description='Shell integration'}
        @{Name='tor'; Description='Tor hidden service management'}
        @{Name='email'; Description='Email management'}
    )

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
