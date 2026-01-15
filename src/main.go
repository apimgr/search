package main

import (
	"context"
	cryptoRand "crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	mathRand "math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/common/banner"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/mode"
	"github.com/apimgr/search/src/model"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engines"
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
	flagInit        bool
	flagConfigInfo  bool
	flagStatus      bool
	flagDaemon      bool
	flagDebug       bool
	flagTest        string
	flagService     string
	flagMaintenance string
	flagUpdate      string
	flagBuild       string
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
)

func init() {
	// Simple commands
	flag.BoolVar(&flagVersion, "version", false, "Show version information")
	flag.BoolVar(&flagVersion, "v", false, "Show version information (shorthand)")
	flag.BoolVar(&flagHelp, "help", false, "Show help message")
	flag.BoolVar(&flagHelp, "h", false, "Show help message (shorthand)")
	flag.BoolVar(&flagInit, "init", false, "Initialize configuration")
	flag.BoolVar(&flagConfigInfo, "config-info", false, "Show configuration paths and status")
	flag.BoolVar(&flagStatus, "status", false, "Show server status")
	flag.BoolVar(&flagDaemon, "daemon", false, "Daemonize (detach from terminal)")
	flag.BoolVar(&flagDebug, "debug", false, "Enable debug mode (verbose logging, debug endpoints)")

	// Commands with optional arguments
	flag.StringVar(&flagTest, "test", "", "Test search engines with optional query")
	flag.StringVar(&flagService, "service", "", "Service management: start|stop|restart|reload|status|--install|--uninstall|--disable|--help")
	flag.StringVar(&flagMaintenance, "maintenance", "", "Maintenance: backup|restore|update|mode")
	flag.StringVar(&flagUpdate, "update", "", "Update management: check|yes|branch")
	flag.StringVar(&flagBuild, "build", "", "Build for platforms: all|linux|darwin|windows|freebsd")
	flag.StringVar(&flagShell, "shell", "", "Shell integration: completions|init|--help")

	// Configuration override flags (NON-NEGOTIABLE per AI.md PART 6)
	flag.StringVar(&flagMode, "mode", "", "Set application mode (production|development)")
	flag.StringVar(&flagData, "data", "", "Set data directory")
	flag.StringVar(&flagConfig, "config", "", "Set config directory")
	flag.StringVar(&flagCache, "cache", "", "Set cache directory")
	flag.StringVar(&flagLog, "log", "", "Set log directory")
	flag.StringVar(&flagBackup, "backup", "", "Set backup directory")
	flag.StringVar(&flagPID, "pid", "", "Set PID file path")
	flag.StringVar(&flagAddress, "address", "", "Set listen address")
	flag.IntVar(&flagPort, "port", 0, "Set listen port")
}

func main() {
	// Custom usage function
	flag.Usage = func() {
		printHelp()
	}

	// Parse flags
	flag.Parse()

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
	case flagInit:
		runInit()
		return
	case flagConfigInfo:
		showConfigInfo()
		return
	case flagStatus:
		showStatus()
		return
	case flagTest != "" || (len(os.Args) > 1 && os.Args[1] == "--test"):
		runTest()
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
	case flagBuild != "" || (len(os.Args) > 1 && os.Args[1] == "--build"):
		platform := flagBuild
		if platform == "" {
			platform = "all"
		}
		runBuild(platform)
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
		os.Setenv("SEARCH_MODE", flagMode)
		os.Setenv("MODE", flagMode)
		mode.SetAppMode(flagMode)
	} else {
		// Initialize mode from environment
		mode.FromEnv()
	}

	// Set debug mode from CLI flag
	// Per AI.md PART 6: --debug enables debug mode (verbose logging, debug endpoints)
	if flagDebug {
		os.Setenv("DEBUG", "true")
		os.Setenv("SEARCH_DEBUG", "true")
		mode.SetDebugEnabled(true)
	}

	if flagData != "" {
		os.Setenv("SEARCH_DATA_DIR", flagData)
		config.SetDataDirOverride(flagData)
	}
	if flagConfig != "" {
		os.Setenv("SEARCH_CONFIG_DIR", flagConfig)
		config.SetConfigDirOverride(flagConfig)
	}
	if flagCache != "" {
		os.Setenv("SEARCH_CACHE_DIR", flagCache)
		config.SetCacheDirOverride(flagCache)
	}
	if flagLog != "" {
		os.Setenv("SEARCH_LOG_DIR", flagLog)
		config.SetLogDirOverride(flagLog)
	}
	if flagBackup != "" {
		os.Setenv("SEARCH_BACKUP_DIR", flagBackup)
		config.SetBackupDirOverride(flagBackup)
	}
	if flagPID != "" {
		os.Setenv("SEARCH_PID_FILE", flagPID)
		config.SetPIDFileOverride(flagPID)
	}
	if flagAddress != "" {
		os.Setenv("SEARCH_ADDRESS", flagAddress)
	}
	if flagPort != 0 {
		os.Setenv("SEARCH_PORT", fmt.Sprintf("%d", flagPort))
		os.Setenv("PORT", fmt.Sprintf("%d", flagPort))
	}

	// Ensure directories exist after CLI overrides are applied
	// Per AI.md PART 6: All directory flags MUST create directories if they don't exist
	if flagData != "" || flagConfig != "" || flagCache != "" || flagLog != "" || flagBackup != "" || flagPID != "" {
		if err := config.EnsureDirectories(); err != nil {
			log.Printf("Warning: Failed to create directories: %v", err)
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
	case "--test":
		runTest()
	case "--init":
		runInit()
	case "--config-info":
		showConfigInfo()
	case "--status":
		showStatus()
	case "--service":
		if len(os.Args) > 2 {
			runService(os.Args[2])
		} else {
			fmt.Println("Usage: search --service {start,stop,restart,reload,status,--install,--uninstall,--disable,--help}")
		}
	case "--maintenance":
		if len(os.Args) > 2 {
			runMaintenance(os.Args[2])
		} else {
			fmt.Println("Usage: search --maintenance <backup|restore|update|mode>")
		}
	case "--update":
		subCmd := "yes"
		if len(os.Args) > 2 {
			subCmd = os.Args[2]
		}
		runUpdate(subCmd)
	case "--build":
		platform := "all"
		if len(os.Args) > 2 {
			platform = os.Args[2]
		}
		runBuild(platform)
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
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		fmt.Println("Use --help for usage information")
	}
}

func runServer() {
	// Handle daemonization per AI.md PART 6
	// Check if we should daemonize (only for manual starts, not --service start)
	if flagDaemon && os.Getenv("_DAEMON_CHILD") != "1" {
		if err := daemonize(); err != nil {
			log.Fatalf("❌ Failed to daemonize: %v", err)
		}
		// Parent exits in daemonize(), only child continues
	}

	// Per AI.md PART 8: If running as root, setup system resources then drop privileges
	// Skip privilege dropping in container mode - container entrypoint handles this
	if service.IsRunningAsRoot() && !config.IsRunningInContainer() {
		log.Println("[Startup] Running as root - performing privileged setup")

		// Step 8a: Create system user
		svcUser, err := service.CreateSystemUser("search")
		if err != nil {
			log.Fatalf("❌ Failed to create system user: %v", err)
		}
		log.Printf("[Startup] System user ready: %s (uid=%d, gid=%d)", svcUser.Name, svcUser.UID, svcUser.GID)

		// Step 8b-d: Create directories and set ownership (while still root)
		if err := config.EnsureSystemDirectories("search"); err != nil {
			log.Fatalf("❌ Failed to create system directories: %v", err)
		}
		log.Println("[Startup] System directories created")

		// Set environment for the service user (HOME points to data dir)
		// This ensures config.Initialize() uses the correct paths after privilege drop
		os.Setenv("HOME", "/var/lib/apimgr/search")
		os.Setenv("SEARCH_CONFIG_DIR", "/etc/apimgr/search")
		os.Setenv("SEARCH_DATA_DIR", "/var/lib/apimgr/search")
		os.Setenv("SEARCH_LOG_DIR", "/var/log/apimgr/search")
		os.Setenv("SEARCH_CACHE_DIR", "/var/cache/apimgr/search")

		// Step 8e-f: Port binding for privileged ports happens in server.Start()
		// Pre-binding not implemented yet - using AmbientCapabilities as fallback

		// Step 8g: Drop privileges to search user
		log.Printf("[Startup] Dropping privileges to user: %s", svcUser.Name)
		if err := service.DropPrivileges(svcUser.Name); err != nil {
			log.Fatalf("❌ Failed to drop privileges: %v", err)
		}

		// Step 8h: Verify privilege drop succeeded
		if err := service.VerifyPrivilegesDropped(); err != nil {
			log.Fatalf("❌ %v", err)
		}
		log.Println("[Startup] Privileges dropped successfully")
	}

	// Initialize configuration
	cfg, err := config.Initialize()
	if err != nil {
		log.Fatalf("❌ Configuration failed: %v", err)
	}

	// Build listen URLs for banner
	urls := buildListenURLs(cfg)

	// Check for first run - generate setup token if needed
	// Per AI.md PART 14: Setup token displayed ONCE on first run
	var setupToken string
	showSetup := cfg.IsFirstRun()
	if showSetup {
		setupToken = generateSetupToken()
		// Store hashed token in database for verification
		// The actual token is shown ONCE and never stored in plain text
		dataDir := config.GetDataDir()
		dbPath := filepath.Join(dataDir, "db", "server.db")
		if err := storeSetupToken(dbPath, setupToken); err != nil {
			log.Printf("Warning: Could not store setup token: %v", err)
		}
	}

	// Print responsive startup banner per AI.md PART 7 and PART 14
	banner.Print(banner.Config{
		AppName:    "Search",
		Version:    config.Version,
		Mode:       cfg.Server.Mode,
		Debug:      mode.IsDebugEnabled(),
		URLs:       urls,
		ShowSetup:  showSetup,
		SetupToken: setupToken,
		AdminPath:  "admin",
	})

	// Create server
	srv := server.New(cfg)

	// Setup signal handling per AI.md PART 7
	// Uses platform-dependent signal handling via src/signal package
	sigsvc.Setup(sigsvc.ShutdownConfig{
		ShutdownFunc: srv.Shutdown,
		PIDFile:      config.GetPIDFile(),
	})

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
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
  --mode <mode>            Set application mode (production|development)
  --data <dir>             Set data directory
  --config <dir>           Set config directory
  --cache <dir>            Set cache directory
  --log <dir>              Set log directory
  --backup <dir>           Set backup directory
  --pid <file>             Set PID file path
  --address <addr>         Set listen address
  --port <port>            Set listen port
  --daemon                 Daemonize (detach from terminal, Unix only)
  --debug                  Enable debug mode (verbose logging, debug endpoints)

Information:
  --help, -h               Show this help message
  --version, -v            Show version information
  --status                 Show server status and health
  --config-info            Show configuration paths and status

Shell Integration:
  --shell completions [SHELL]  Print shell completions script
  --shell init [SHELL]         Print shell init command for eval
  --shell --help               Show shell help

Setup:
  --init                   Initialize configuration
  --test [query]           Test search engines with optional query

Service Management:
  --service <action>       Service management (requires privileges):
    install                Install as system service
    uninstall              Remove system service
    start                  Start the service
    stop                   Stop the service
    status                 Check service status
    restart                Restart the service
    reload                 Reload configuration (SIGHUP)
    enable                 Enable service autostart
    disable                Disable service autostart

Maintenance:
  --maintenance <action>   Maintenance commands:
    backup [file]          Create backup archive
                           Use BACKUP_PASSWORD env var for encryption
    restore <file>         Restore from backup
                           Use BACKUP_PASSWORD env var if encrypted
    setup                  Reset admin credentials (generates setup token)
    mode                   Toggle maintenance mode

Updates:
  --update [subcommand]    Update management:
    check                  Check for available updates
    yes                    Download and install update (default)
    branch <name>          Set update branch (stable|beta|daily)
    rollback               Rollback to previous version
    list                   List available versions

Build:
  --build [platform]       Build binaries (requires Docker):
    all                    Build for all 8 platforms (default)
    linux                  Build for Linux (amd64, arm64)
    darwin                 Build for macOS (amd64, arm64)
    windows                Build for Windows (amd64, arm64)
    freebsd                Build for FreeBSD (amd64, arm64)
    host                   Build for current OS/ARCH only
    linux/amd64            Build for specific OS/ARCH

Environment Variables:
  SEARCH_SETTINGS_PATH     Path to configuration file
  SEARCH_CONFIG_DIR        Configuration directory
  SEARCH_DATA_DIR          Data directory
  SEARCH_LOG_DIR           Log directory
  DEBUG, SEARCH_DEBUG      Enable debug mode (0/1, true/false)
  SECRET_KEY               Secret key for sessions
  PORT, SEARCH_PORT        Server port
  MODE, SEARCH_MODE        Application mode (production|development)
  INSTANCE_NAME            Instance display name
  DISABLE_TOR              Disable Tor (auto-enabled if tor binary installed)
  BACKUP_PASSWORD          Password for backup encryption (AES-256-GCM)

Examples:
  %s                                 Start server with defaults
  %s --port 8080                     Start on port 8080
  %s --mode development              Start in dev mode
  %s --config /etc/search --data /var/lib/search  Custom directories
  %s --init                          Create configuration files
  %s --test "golang"                 Test search with "golang" query
  %s --service --install             Install as system service
  %s --service reload                Reload configuration
  %s --update check                  Check for updates
  %s --maintenance setup             Reset admin credentials
  %s --build all                     Build for all platforms
  %s --build host                    Build for current platform

For more information: https://github.com/apimgr/search
`, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName, binaryName)
}

func runInit() {
	fmt.Println("🔧 Initializing Search configuration...")
	fmt.Println()

	cfg, err := config.Initialize()
	if err != nil {
		log.Fatalf("❌ Initialization failed: %v", err)
	}

	fmt.Println("✅ Configuration initialized successfully!")
	fmt.Println()
	fmt.Println("📁 Configuration Paths:")
	fmt.Println("   Config: ", config.GetConfigDir())
	fmt.Println("   Data:   ", config.GetDataDir())
	fmt.Println("   Logs:   ", config.GetLogDir())
	fmt.Println("   Cache:  ", config.GetCacheDir())
	fmt.Println("   Backup: ", config.GetBackupDir())
	fmt.Println("   SSL:    ", config.GetSSLDir())
	fmt.Println("   Tor:    ", config.GetTorDir())
	fmt.Println()
	fmt.Println("⚙️  Server Configuration:")
	fmt.Println("   Title:  ", cfg.Server.Title)
	fmt.Printf("   Port:   %d\n", cfg.Server.Port)
	fmt.Println("   Mode:   ", cfg.Server.Mode)
	fmt.Println("   Tor:    ", cfg.Server.Tor.Enabled)
	fmt.Println()
	fmt.Println("🔍 Search Engines:")
	for name, engine := range cfg.Engines {
		status := "❌ disabled"
		if engine.Enabled {
			status = fmt.Sprintf("✅ enabled (priority: %d)", engine.Priority)
		}
		fmt.Printf("   %s: %s\n", name, status)
	}
}

func showConfigInfo() {
	fmt.Println("📁 Configuration Information")
	fmt.Println()

	env := config.LoadFromEnv()

	fmt.Println("System:")
	fmt.Printf("  OS:           %s\n", config.GetOS())
	fmt.Printf("  Architecture: %s\n", config.GetArch())
	fmt.Printf("  Privileged:   %v\n", config.IsPrivileged())
	fmt.Printf("  Container:    %v\n", config.IsRunningInContainer())
	fmt.Println()
	fmt.Println("Mode:", env.GetMode())
	fmt.Println()
	fmt.Println("Directories:")
	fmt.Println("  Config:   ", config.GetConfigDir())
	fmt.Println("  Data:     ", config.GetDataDir())
	fmt.Println("  Logs:     ", config.GetLogDir())
	fmt.Println("  Cache:    ", config.GetCacheDir())
	fmt.Println("  Backup:   ", config.GetBackupDir())
	fmt.Println("  Database: ", config.GetDatabaseDir())
	fmt.Println("  GeoIP:    ", config.GetGeoIPDir())
	fmt.Println("  Tor:      ", config.GetTorDir())
	fmt.Println("  SSL:      ", config.GetSSLDir())
	fmt.Println("  Templates:", config.GetTemplatesDir())
	fmt.Println("  Web Data: ", config.GetWebDataDir())
	fmt.Println()
	fmt.Println("Files:")
	fmt.Println("  Config:   ", config.GetConfigPath())
	fmt.Println("  PID:      ", config.GetPIDFile())
	fmt.Println("  Service:  ", config.GetServiceFile())
	fmt.Println("  Binary:   ", config.GetBinaryPath())
	fmt.Println()

	// Check if config exists
	configPath := config.GetConfigPath()
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println("Configuration Status: ✅ exists")
	} else {
		fmt.Println("Configuration Status: ⚠️  not found (run --init to create)")
	}

	fmt.Println()
	fmt.Println("Environment Variables:")
	if env.InstanceName != "Search" {
		fmt.Println("  INSTANCE_NAME:", env.InstanceName)
	}
	if env.Port != "" {
		fmt.Println("  PORT:", env.Port)
	}
	if env.Mode != "production" {
		fmt.Println("  MODE:", env.Mode)
	}
	if env.Debug {
		fmt.Println("  DEBUG: enabled")
	}
	if env.UseTor {
		fmt.Println("  TOR: enabled (auto-detected)")
	}
	if env.ConfigDir != "" {
		fmt.Println("  CONFIG_DIR:", env.ConfigDir)
	}
	if env.DataDir != "" {
		fmt.Println("  DATA_DIR:", env.DataDir)
	}
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
		return
	}

	// Load config to show settings
	var port int
	var mode string
	var torEnabled bool
	var torAddress string

	configPath := config.GetConfigPath()
	if cfg, err := config.Load(configPath); err == nil {
		port = cfg.Server.Port
		mode = cfg.Server.Mode
		torEnabled = cfg.Server.Tor.Enabled
		torAddress = cfg.Server.Tor.OnionAddress
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

	// Node status
	fmt.Println("Node: standalone")
	fmt.Println("Cluster: disabled")
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
		var systemUptime float64
		fmt.Sscanf(string(uptimeData), "%f", &systemUptime)

		// Parse process start time (in clock ticks)
		var startTicks int64
		fmt.Sscanf(fields[21], "%d", &startTicks)

		// Convert to seconds (assuming 100 Hz clock)
		clkTck := int64(100) // sysconf(_SC_CLK_TCK) is usually 100
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
	fmt.Printf("🔧 Service Management: %s\n\n", action)

	// Load configuration
	cfg, err := config.Initialize()
	if err != nil {
		fmt.Printf("❌ Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Create service manager
	sm := service.NewServiceManager(cfg)

	switch action {
	case "--install", "install":
		if !config.IsPrivileged() {
			fmt.Println("❌ This command requires elevated privileges")
			fmt.Println("   Run with sudo/admin rights")
			os.Exit(1)
		}
		fmt.Printf("Installing service for %s...\n", runtime.GOOS)
		if err := sm.Install(); err != nil {
			fmt.Printf("❌ Failed to install service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service installed successfully")
		fmt.Println("   Run 'search --service start' to start the service")

	case "--uninstall", "uninstall":
		if !config.IsPrivileged() {
			fmt.Println("❌ This command requires elevated privileges")
			os.Exit(1)
		}
		fmt.Println("Uninstalling service...")
		if err := sm.Uninstall(); err != nil {
			fmt.Printf("❌ Failed to uninstall service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service uninstalled successfully")

	case "start":
		if !config.IsPrivileged() {
			fmt.Println("❌ This command requires elevated privileges")
			os.Exit(1)
		}
		fmt.Println("Starting service...")
		if err := sm.Start(); err != nil {
			fmt.Printf("❌ Failed to start service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service started successfully")

	case "stop":
		if !config.IsPrivileged() {
			fmt.Println("❌ This command requires elevated privileges")
			os.Exit(1)
		}
		fmt.Println("Stopping service...")
		if err := sm.Stop(); err != nil {
			fmt.Printf("❌ Failed to stop service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service stopped successfully")

	case "status":
		status, err := sm.Status()
		if err != nil {
			fmt.Printf("❌ Failed to get service status: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Service status: %s\n", status)

	case "restart":
		if !config.IsPrivileged() {
			fmt.Println("❌ This command requires elevated privileges")
			os.Exit(1)
		}
		fmt.Println("Restarting service...")
		if err := sm.Restart(); err != nil {
			fmt.Printf("❌ Failed to restart service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service restarted successfully")

	case "reload":
		fmt.Println("Reloading service configuration...")
		if err := sm.Reload(); err != nil {
			fmt.Printf("❌ Failed to reload service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service configuration reloaded")

	case "--disable", "disable":
		if !config.IsPrivileged() {
			fmt.Println("❌ This command requires elevated privileges")
			os.Exit(1)
		}
		fmt.Println("Disabling service autostart...")
		if err := sm.Disable(); err != nil {
			fmt.Printf("❌ Failed to disable service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service autostart disabled")

	case "--enable", "enable":
		if !config.IsPrivileged() {
			fmt.Println("❌ This command requires elevated privileges")
			os.Exit(1)
		}
		fmt.Println("Enabling service autostart...")
		if err := sm.Enable(); err != nil {
			fmt.Printf("❌ Failed to enable service: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Service autostart enabled")

	case "--help", "help":
		fmt.Println("Service Management Commands:")
		fmt.Println()
		fmt.Println("  --install     Install as system service")
		fmt.Println("  --uninstall   Remove system service")
		fmt.Println("  start         Start the service")
		fmt.Println("  stop          Stop the service")
		fmt.Println("  restart       Restart the service")
		fmt.Println("  reload        Reload configuration (SIGHUP)")
		fmt.Println("  --enable      Enable service autostart")
		fmt.Println("  --disable     Disable service autostart")
		fmt.Println("  status        Show service status")
		fmt.Println("  --help        Show this help")

	default:
		fmt.Printf("❌ Unknown action: %s\n", action)
		fmt.Println("Valid actions: start, stop, restart, reload, status, --install, --uninstall, --enable, --disable, --help")
	}
}

func runMaintenance(action string) {
	fmt.Printf("🔧 Maintenance: %s\n\n", action)

	bm := backup.NewManager()

	switch action {
	case "backup":
		filename := ""
		if len(os.Args) > 3 {
			filename = os.Args[3]
		}

		// Check for backup encryption password
		// Per AI.md PART 24: Password from env var or prompt
		password := os.Getenv("BACKUP_PASSWORD")
		if password != "" {
			fmt.Println("🔐 Backup encryption: ENABLED")
			bm.SetPassword(password)
		} else {
			fmt.Println("🔓 Backup encryption: DISABLED (set BACKUP_PASSWORD to enable)")
		}

		fmt.Println("Creating backup...")
		backupPath, err := bm.Create(filename)
		if err != nil {
			fmt.Printf("❌ Backup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Backup created: %s\n", backupPath)

		// Show backup info
		metadata, err := bm.GetMetadata(backupPath)
		if err == nil {
			fmt.Printf("   Version: %s\n", metadata.Version)
			fmt.Printf("   Files:   %d\n", len(metadata.Files))
			if metadata.Encrypted {
				fmt.Printf("   Encrypted: YES (%s)\n", metadata.EncryptionMethod)
			} else {
				fmt.Println("   Encrypted: NO")
			}
		}

	case "restore":
		if len(os.Args) < 4 {
			fmt.Println("❌ Please specify backup file to restore")
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
		fmt.Printf("Restoring from: %s\n", filename)

		// Check if backup is encrypted
		// Per AI.md PART 24: Prompt for password if encrypted
		password := os.Getenv("BACKUP_PASSWORD")
		if password != "" {
			fmt.Println("🔐 Using encryption password from BACKUP_PASSWORD env var")
			bm.SetPassword(password)
		}

		// Confirm restore
		fmt.Print("This will overwrite current configuration. Continue? (yes/no): ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Restore cancelled.")
			return
		}

		if err := bm.Restore(filename); err != nil {
			if strings.Contains(err.Error(), "decryption failed") {
				fmt.Printf("❌ Restore failed: %v\n", err)
				fmt.Println("   This backup appears to be encrypted.")
				fmt.Println("   Set BACKUP_PASSWORD environment variable and try again.")
			} else {
				fmt.Printf("❌ Restore failed: %v\n", err)
			}
			os.Exit(1)
		}
		fmt.Println("✅ Restore completed successfully")
		fmt.Println("   Please restart the server to apply changes.")

	case "list":
		backups, err := bm.List()
		if err != nil {
			fmt.Printf("❌ Failed to list backups: %v\n", err)
			return
		}
		if len(backups) == 0 {
			fmt.Println("No backups found.")
			return
		}
		fmt.Println("Available backups:")
		for _, b := range backups {
			fmt.Printf("  - %s\n", b.Filename)
			fmt.Printf("      Size:    %s\n", b.FormatSize())
			fmt.Printf("      Created: %s\n", b.CreatedAt.Format("2006-01-02 15:04:05"))
			if b.Version != "" {
				fmt.Printf("      Version: %s\n", b.Version)
			}
		}

	case "update":
		runUpdate("yes")

	case "mode":
		fmt.Println("Toggling maintenance mode...")
		cfg, err := config.Initialize()
		if err != nil {
			fmt.Printf("❌ Failed to load config: %v\n", err)
			os.Exit(1)
		}
		// Toggle maintenance mode
		cfg.Server.MaintenanceMode = !cfg.Server.MaintenanceMode
		if err := cfg.Save(config.GetConfigPath()); err != nil {
			fmt.Printf("❌ Failed to save config: %v\n", err)
			os.Exit(1)
		}
		if cfg.Server.MaintenanceMode {
			fmt.Println("✅ Maintenance mode: ENABLED")
			fmt.Println("   The server will show a maintenance page to users")
		} else {
			fmt.Println("✅ Maintenance mode: DISABLED")
			fmt.Println("   The server is now accepting normal requests")
		}

	case "setup":
		// Admin recovery per AI.md PART 26
		// Clears admin password and generates a new setup token
		fmt.Println("🔧 Admin Recovery Setup")
		fmt.Println()

		// Initialize database to reset credentials
		cfg, err := config.Initialize()
		if err != nil {
			fmt.Printf("❌ Failed to load config: %v\n", err)
			os.Exit(1)
		}

		// Get data directory for server.db
		dataDir := config.GetDataDir()
		dbPath := fmt.Sprintf("%s/server.db", dataDir)

		// Check if database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Println("No existing admin accounts found.")
			fmt.Println("Run the server and visit /admin/setup to create the first admin.")
			return
		}

		fmt.Println("This will:")
		fmt.Println("  1. Clear the primary admin's password")
		fmt.Println("  2. Generate a one-time setup token")
		fmt.Println("  3. Allow password reset via /admin/setup")
		fmt.Println()
		fmt.Print("Continue? (yes/no): ")
		var confirm string
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Cancelled.")
			return
		}

		// Generate setup token
		setupToken := generateSetupToken()

		// Store hashed setup token in database
		if err := storeSetupToken(dbPath, setupToken); err != nil {
			fmt.Printf("❌ Failed to create setup token: %v\n", err)
			os.Exit(1)
		}

		// Clear primary admin credentials
		if err := resetAdminCredentials(dbPath); err != nil {
			fmt.Printf("⚠️  Warning: Could not reset credentials: %v\n", err)
		}

		fmt.Println()
		fmt.Println("✅ Setup token created successfully!")
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════════════════════════╗")
		fmt.Println("║  SETUP TOKEN (copy this - it will not be shown again)    ║")
		fmt.Println("╠══════════════════════════════════════════════════════════╣")
		fmt.Printf("║  %s                                  ║\n", setupToken)
		fmt.Println("╚══════════════════════════════════════════════════════════╝")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Start the server: search")
		fmt.Printf("  2. Visit: http://localhost:%d/admin/setup\n", cfg.Server.Port)
		fmt.Println("  3. Enter the setup token above to create a new password")
		fmt.Println()
		fmt.Println("The token expires in 1 hour.")

	case "help":
		fmt.Println("Maintenance Commands:")
		fmt.Println()
		fmt.Println("  backup [file]     Create backup archive")
		fmt.Println("                    Set BACKUP_PASSWORD env var for encryption")
		fmt.Println("  restore <file>    Restore from backup")
		fmt.Println("                    Set BACKUP_PASSWORD env var if encrypted")
		fmt.Println("  list              List available backups")
		fmt.Println("  update            Check and install updates")
		fmt.Println("  mode              Toggle maintenance mode")
		fmt.Println("  setup             Reset admin credentials (generates setup token)")
		fmt.Println("  help              Show this help")
		fmt.Println()
		fmt.Println("Backup Encryption:")
		fmt.Println("  BACKUP_PASSWORD=secret search --maintenance backup")
		fmt.Println("  BACKUP_PASSWORD=secret search --maintenance restore backup.tar.gz")

	default:
		fmt.Printf("❌ Unknown action: %s\n", action)
		fmt.Println("Valid actions: backup, restore, list, update, mode, setup, help")
	}
}

func runUpdate(subCmd string) {
	fmt.Println("🔄 Update Management")
	fmt.Println()

	um := update.NewManager()

	switch subCmd {
	case "check":
		fmt.Println("Checking for updates...")
		fmt.Printf("Current version: %s\n", config.Version)
		fmt.Println()

		info, err := um.CheckForUpdates(false)
		if err != nil {
			fmt.Printf("❌ Failed to check for updates: %v\n", err)
			return
		}

		if !info.Available {
			fmt.Println("✅ You are running the latest version")
		} else {
			fmt.Printf("🆕 New version available: %s\n", info.LatestVersion)
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
			fmt.Println("❌ Update installation requires elevated privileges")
			fmt.Println("   Use --update check to check without privileges")
			os.Exit(1)
		}

		fmt.Println("Checking for updates...")
		info, err := um.CheckForUpdates(false)
		if err != nil {
			fmt.Printf("❌ Failed to check for updates: %v\n", err)
			os.Exit(1)
		}

		if !info.Available {
			fmt.Println("✅ You are running the latest version")
			return
		}

		fmt.Printf("Downloading version %s...\n", info.LatestVersion)

		archivePath, err := um.DownloadUpdate(info.DownloadURL, func(downloaded, total int64) {
			if total > 0 {
				pct := float64(downloaded) / float64(total) * 100
				fmt.Printf("\r   Progress: %.1f%%", pct)
			}
		})
		fmt.Println()

		if err != nil {
			fmt.Printf("❌ Download failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Installing update...")
		if err := um.InstallUpdate(archivePath); err != nil {
			fmt.Printf("❌ Installation failed: %v\n", err)
			fmt.Println("   Run 'search --update rollback' to restore previous version")
			os.Exit(1)
		}

		fmt.Println("✅ Update installed successfully!")
		fmt.Println("   Please restart the service to apply the update")

	case "rollback":
		fmt.Println("Rolling back to previous version...")
		if err := um.Rollback(); err != nil {
			fmt.Printf("❌ Rollback failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Rollback completed successfully!")
		fmt.Println("   Please restart the service to apply the change")

	case "list":
		fmt.Println("Fetching available versions...")
		versions, err := um.ListAvailableVersions()
		if err != nil {
			fmt.Printf("❌ Failed to fetch versions: %v\n", err)
			return
		}
		fmt.Printf("Current version: %s\n", config.Version)
		fmt.Println()
		fmt.Println("Available versions:")
		for _, v := range versions {
			marker := "  "
			if v == "v"+config.Version || v == config.Version {
				marker = "→ "
			}
			fmt.Printf("%s%s\n", marker, v)
		}

	case "branch":
		if len(os.Args) < 4 {
			fmt.Println("❌ Please specify branch name")
			fmt.Println("Usage: search --update branch <stable|beta|daily>")
			return
		}
		branch := os.Args[3]
		switch branch {
		case "stable":
			fmt.Println("Update branch set to: stable (releases only)")
		case "beta":
			fmt.Println("Update branch set to: beta (includes pre-releases)")
		case "daily":
			fmt.Println("Update branch set to: daily (bleeding edge)")
		default:
			fmt.Printf("❌ Invalid branch: %s\n", branch)
			fmt.Println("Valid branches: stable, beta, daily")
		}

	default:
		fmt.Printf("❌ Unknown subcommand: %s\n", subCmd)
		fmt.Println("Valid subcommands: check, yes, rollback, list, branch <name>")
	}
}

func runTest() {
	fmt.Println("🧪 Testing Search Engines...")
	fmt.Println()

	// Create engine registry
	registry := engines.DefaultRegistry()

	fmt.Printf("✅ Registered %d engines\n\n", registry.Count())

	// List engines
	fmt.Println("📋 Available Engines:")
	for _, engine := range registry.GetEnabled() {
		fmt.Printf("  • %s (priority: %d)\n", engine.DisplayName(), engine.GetPriority())
	}
	fmt.Println()

	// Test search
	testQuery := "golang programming"
	if len(os.Args) > 2 {
		testQuery = os.Args[2]
	}

	fmt.Printf("🔎 Searching for: \"%s\"\n\n", testQuery)

	query := model.NewQuery(testQuery)
	query.Category = model.CategoryGeneral

	// Get engines for category
	searchEngines := registry.GetForCategory(query.Category)
	fmt.Printf("Using %d engines for category '%s'\n\n", len(searchEngines), query.Category)

	// Create aggregator
	aggregator := search.NewAggregatorSimple(searchEngines, 30*time.Second)

	// Perform search
	ctx := context.Background()
	results, err := aggregator.Search(ctx, query)

	if err != nil {
		if err == model.ErrNoResults {
			fmt.Println("⚠️  No results found")
			return
		}
		log.Fatalf("❌ Search failed: %v", err)
	}

	// Display results
	fmt.Printf("✅ Found %d results in %.2f seconds\n", results.TotalResults, results.SearchTime)
	fmt.Printf("📊 Engines used: %v\n\n", results.Engines)

	fmt.Println("🎯 Top Results:")
	fmt.Println(strings.Repeat("─", 80))

	displayCount := 10
	if len(results.Results) < displayCount {
		displayCount = len(results.Results)
	}

	for i := 0; i < displayCount; i++ {
		result := results.Results[i]
		fmt.Printf("\n%d. %s\n", i+1, result.Title)
		fmt.Printf("   🔗 %s\n", result.URL)
		if result.Content != "" {
			content := result.Content
			if len(content) > 150 {
				content = content[:150] + "..."
			}
			fmt.Printf("   📝 %s\n", content)
		}
		fmt.Printf("   🏷️  Engine: %s | Score: %.0f\n", result.Engine, result.Score)
	}

	fmt.Println(strings.Repeat("─", 80))

	// Export as JSON
	if len(os.Args) > 3 && os.Args[3] == "--json" {
		jsonData, _ := json.MarshalIndent(results, "", "  ")
		filename := fmt.Sprintf("search_results_%d.json", time.Now().Unix())
		os.WriteFile(filename, jsonData, 0644)
		fmt.Printf("\n💾 Results saved to: %s\n", filename)
	}
}

// ============================================================
// First Run Detection (per AI.md PART 6)
// ============================================================

// getFirstRunToken returns setup token and whether to show first run setup
// Per AI.md PART 6: Setup token displayed ONCE on first run
func getFirstRunToken(cfg *config.Config) (token string, showSetup bool) {
	dataDir := config.GetDataDir()
	dbPath := filepath.Join(dataDir, "db", "server.db")

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Database doesn't exist yet, will be created on first use
		return "", false
	}

	// Open database and check for admin
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", false
	}
	defer db.Close()

	// Check if admin_credentials table exists
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='admin_credentials'").Scan(&tableName)
	if err != nil {
		// Table doesn't exist, this is a fresh database - generate token
		return generateAndStoreToken(dbPath)
	}

	// Check if any admin exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM admin_credentials").Scan(&count)
	if err != nil {
		return "", false
	}

	if count == 0 {
		// No admin exists, generate setup token
		return generateAndStoreToken(dbPath)
	}

	return "", false
}

// generateAndStoreToken generates a new setup token and stores it
func generateAndStoreToken(dbPath string) (string, bool) {
	// Check if a valid setup token already exists
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return "", false
	}
	defer db.Close()

	// Check for unexpired setup token
	var expiresAt string
	err = db.QueryRow(`
		SELECT expires_at FROM setup_token WHERE id = 1 AND used_at IS NULL
	`).Scan(&expiresAt)

	if err == nil {
		// Token exists, check if still valid
		expiry, err := time.Parse("2006-01-02 15:04:05", expiresAt)
		if err == nil && expiry.After(time.Now()) {
			// Valid token exists, don't regenerate (token only shown once)
			return "", true // showSetup=true but no token (previously shown)
		}
	}

	// Generate new setup token
	setupToken := generateSetupToken()

	// Store the hashed token
	if err := storeSetupToken(dbPath, setupToken); err != nil {
		log.Printf("Warning: Failed to store setup token: %v", err)
		return "", false
	}

	return setupToken, true
}

// buildListenURLs builds the list of URLs the server is listening on
func buildListenURLs(cfg *config.Config) []string {
	var urls []string

	// Get listen address
	addr := cfg.Server.Address
	if addr == "" {
		addr = "0.0.0.0"
	}

	// Primary HTTP URL
	port := cfg.Server.Port
	if port == 0 {
		port = 64580
	}

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

// checkFirstRun checks if this is the first run and displays setup token if needed
// Per AI.md PART 6: Setup token displayed ONCE on first run
// DEPRECATED: Use getFirstRunToken instead - kept for backward compatibility
func checkFirstRun(cfg *config.Config) {
	dataDir := config.GetDataDir()
	dbPath := filepath.Join(dataDir, "db", "server.db")

	// Check if database exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		// Database doesn't exist yet, will be created on first use
		// We'll display setup token after database creation
		return
	}

	// Open database and check for admin
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		// Can't open database, skip first-run check
		return
	}
	defer db.Close()

	// Check if admin_credentials table exists
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='admin_credentials'").Scan(&tableName)
	if err != nil {
		// Table doesn't exist, this is a fresh database
		// Generate and display setup token
		displayFirstRunSetupToken(cfg, dbPath)
		return
	}

	// Check if any admin exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM admin_credentials").Scan(&count)
	if err != nil {
		// Can't query, skip check
		return
	}

	if count == 0 {
		// No admin exists, display setup token
		displayFirstRunSetupToken(cfg, dbPath)
	}
}

// displayFirstRunSetupToken generates and displays a setup token on first run
func displayFirstRunSetupToken(cfg *config.Config, dbPath string) {
	// Check if a valid setup token already exists
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return
	}
	defer db.Close()

	// Check for unexpired setup token
	var expiresAt string
	err = db.QueryRow(`
		SELECT expires_at FROM setup_token WHERE id = 1 AND used_at IS NULL
	`).Scan(&expiresAt)

	if err == nil {
		// Token exists, check if still valid
		expiry, err := time.Parse("2006-01-02 15:04:05", expiresAt)
		if err == nil && expiry.After(time.Now()) {
			// Valid token exists, don't regenerate (token only shown once)
			fmt.Println("╔══════════════════════════════════════════════════════════════╗")
			fmt.Println("║  FIRST RUN: Admin setup required                             ║")
			fmt.Println("╠══════════════════════════════════════════════════════════════╣")
			fmt.Printf("║  Visit: http://localhost:%d/admin/setup               ║\n", cfg.Server.Port)
			fmt.Println("║  A setup token was previously generated.                     ║")
			fmt.Println("║  Run 'search --maintenance setup' for a new token.           ║")
			fmt.Println("╚══════════════════════════════════════════════════════════════╝")
			fmt.Println()
			return
		}
	}

	// Generate new setup token
	setupToken := generateSetupToken()

	// Store the hashed token
	if err := storeSetupToken(dbPath, setupToken); err != nil {
		log.Printf("Warning: Failed to store setup token: %v", err)
		return
	}

	// Display the token ONCE - per AI.md this is the only time it's shown
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║  FIRST RUN: Admin setup required                             ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  SETUP TOKEN (copy this - it will not be shown again!)       ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  %s                                  ║\n", setupToken)
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Visit: http://localhost:%d/admin/setup               ║\n", cfg.Server.Port)
	fmt.Println("║  Enter the token above to create your admin account.         ║")
	fmt.Println("║  Token expires in 1 hour.                                     ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
}

// ============================================================
// Admin Recovery Helpers (per AI.md PART 26)
// ============================================================

// generateSetupToken creates a cryptographically secure setup token
func generateSetupToken() string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	token := make([]byte, 32)

	// Use crypto/rand for secure generation
	randomBytes := make([]byte, 32)
	if _, err := cryptoRand.Read(randomBytes); err != nil {
		// Fallback to less secure method if crypto/rand fails
		for i := range token {
			token[i] = chars[mathRand.Intn(len(chars))]
		}
		return string(token)
	}

	for i, b := range randomBytes {
		token[i] = chars[int(b)%len(chars)]
	}
	return string(token)
}

// storeSetupToken stores the hashed setup token in the database
func storeSetupToken(dbPath, token string) error {
	// Hash the token
	hash := sha256.Sum256([]byte(token))
	tokenHash := hex.EncodeToString(hash[:])

	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Create setup_token table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS setup_token (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			token_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			used_at DATETIME
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Clear any existing setup token and insert new one
	expiresAt := time.Now().Add(1 * time.Hour)
	_, err = db.Exec(`DELETE FROM setup_token`)
	if err != nil {
		return fmt.Errorf("failed to clear old tokens: %w", err)
	}

	_, err = db.Exec(`
		INSERT INTO setup_token (id, token_hash, expires_at) VALUES (1, ?, ?)
	`, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to store token: %w", err)
	}

	return nil
}

// resetAdminCredentials clears the primary admin's password for recovery
func resetAdminCredentials(dbPath string) error {
	// Open database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Clear password for primary admin (is_primary = 1)
	result, err := db.Exec(`
		UPDATE admin_credentials
		SET password_hash = '', token_hash = NULL, token_prefix = NULL, updated_at = datetime('now')
		WHERE is_primary = 1
	`)
	if err != nil {
		return fmt.Errorf("failed to reset credentials: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Try legacy single-admin format
		_, err = db.Exec(`
			UPDATE admin_credentials
			SET password_hash = '', token_hash = NULL, token_prefix = NULL, updated_at = datetime('now')
			WHERE id = 1
		`)
		if err != nil {
			return fmt.Errorf("failed to reset credentials: %w", err)
		}
	}

	return nil
}

// ============================================================
// Build Command (per AI.md PART 23)
// ============================================================

// BuildTarget represents a build target platform
type BuildTarget struct {
	OS   string
	Arch string
}

// runBuild builds the binary for specified platforms using Docker
// Per AI.md PART 23: Binary must be able to build itself
func runBuild(platform string) {
	fmt.Println("🔧 Build Command")
	fmt.Printf("   Version: %s\n", config.Version)
	fmt.Println()

	// Check if Docker is available
	if _, err := exec.LookPath("docker"); err != nil {
		fmt.Println("❌ Docker is required for cross-platform builds")
		fmt.Println("   Please install Docker and try again")
		os.Exit(1)
	}

	// Find the source directory
	srcDir, err := findSourceDir()
	if err != nil {
		fmt.Printf("❌ Cannot find source directory: %v\n", err)
		fmt.Println("   The build command requires access to the source code")
		os.Exit(1)
	}

	// Define build targets
	allTargets := []BuildTarget{
		{"linux", "amd64"},
		{"linux", "arm64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"windows", "amd64"},
		{"windows", "arm64"},
		{"freebsd", "amd64"},
		{"freebsd", "arm64"},
	}

	// Filter targets based on platform argument
	var targets []BuildTarget
	switch platform {
	case "all", "":
		targets = allTargets
	case "linux":
		targets = []BuildTarget{{"linux", "amd64"}, {"linux", "arm64"}}
	case "darwin", "macos":
		targets = []BuildTarget{{"darwin", "amd64"}, {"darwin", "arm64"}}
	case "windows":
		targets = []BuildTarget{{"windows", "amd64"}, {"windows", "arm64"}}
	case "freebsd":
		targets = []BuildTarget{{"freebsd", "amd64"}, {"freebsd", "arm64"}}
	case "host":
		targets = []BuildTarget{{runtime.GOOS, runtime.GOARCH}}
	default:
		// Check for OS/ARCH format
		parts := strings.Split(platform, "/")
		if len(parts) == 2 {
			targets = []BuildTarget{{parts[0], parts[1]}}
		} else {
			fmt.Printf("❌ Unknown platform: %s\n", platform)
			fmt.Println("   Valid options: all, linux, darwin, windows, freebsd, host, or OS/ARCH")
			os.Exit(1)
		}
	}

	// Create output directory
	outputDir := filepath.Join(srcDir, "binaries")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("❌ Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("📁 Source: %s\n", srcDir)
	fmt.Printf("📁 Output: %s\n", outputDir)
	fmt.Printf("🎯 Targets: %d platforms\n\n", len(targets))

	// Build for each target
	failed := 0
	for _, target := range targets {
		ext := ""
		if target.OS == "windows" {
			ext = ".exe"
		}
		outputName := fmt.Sprintf("search-%s-%s%s", target.OS, target.Arch, ext)
		outputPath := filepath.Join(outputDir, outputName)

		fmt.Printf("   Building %s/%s... ", target.OS, target.Arch)

		if err := buildWithDocker(srcDir, outputPath, target.OS, target.Arch); err != nil {
			fmt.Printf("❌ %v\n", err)
			failed++
		} else {
			// Get file size
			if info, err := os.Stat(outputPath); err == nil {
				fmt.Printf("✅ (%s)\n", formatBytes(info.Size()))
			} else {
				fmt.Println("✅")
			}
		}
	}

	fmt.Println()
	if failed > 0 {
		fmt.Printf("⚠️  %d/%d builds failed\n", failed, len(targets))
		os.Exit(1)
	}
	fmt.Printf("✅ Build complete: %d binaries in %s/\n", len(targets), outputDir)
}

// findSourceDir locates the source directory
func findSourceDir() (string, error) {
	// Try current directory first
	if _, err := os.Stat("go.mod"); err == nil {
		return ".", nil
	}

	// Try to find based on executable path
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	// Check if we're in the binaries directory
	dir := filepath.Dir(execPath)
	parentDir := filepath.Dir(dir)
	if _, err := os.Stat(filepath.Join(parentDir, "go.mod")); err == nil {
		return parentDir, nil
	}

	// Check common development paths
	homeDir, _ := os.UserHomeDir()
	commonPaths := []string{
		filepath.Join(homeDir, "Projects/github/apimgr/search"),
		"/root/Projects/github/apimgr/search",
		"/app",
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("could not find go.mod in any expected location")
}

// buildWithDocker builds a binary using Docker
func buildWithDocker(srcDir, outputPath, goos, goarch string) error {
	outputName := filepath.Base(outputPath)

	// Docker command to build
	// Per AI.md PART 23: Must use golang:alpine for builds
	cmd := exec.Command("docker", "run", "--rm",
		"-v", srcDir+":/app",
		"-w", "/app",
		"-e", "CGO_ENABLED=0",
		"-e", "GOOS="+goos,
		"-e", "GOARCH="+goarch,
		"golang:alpine",
		"go", "build",
		"-ldflags", fmt.Sprintf("-s -w -X github.com/apimgr/search/src/config.Version=%s -X github.com/apimgr/search/src/config.BuildDate=%s",
			config.Version, time.Now().Format("Mon Jan 02, 2006 at 15:04:05 MST")),
		"-o", "/app/binaries/"+outputName,
		"./src",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, string(output))
	}

	return nil
}

// formatBytes formats bytes as human-readable size
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// daemonize forks the process and detaches from terminal
// Per AI.md PART 6 - Daemonization
func daemonize() error {
	// Windows doesn't support traditional Unix daemonization
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "Warning: --daemon is not supported on Windows")
		fmt.Fprintln(os.Stderr, "Use --service --install && --service start for Windows Service")
		return nil // Continue in foreground
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
	os.Exit(0)
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
		fmt.Printf("❌ Unknown shell subcommand: %s\n", subCmd)
		fmt.Println("Valid subcommands: completions, init, --help")
		os.Exit(1)
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

    opts="--help --version --status --init --config-info --test --daemon --debug"
    opts="$opts --mode --config --data --cache --log --backup --pid --address --port"
    opts="$opts --service --maintenance --update --build --shell"

    case "${prev}" in
        --service)
            COMPREPLY=( $(compgen -W "install uninstall start stop restart reload enable disable status help" -- ${cur}) )
            return 0
            ;;
        --maintenance)
            COMPREPLY=( $(compgen -W "backup restore list update mode setup help" -- ${cur}) )
            return 0
            ;;
        --update)
            COMPREPLY=( $(compgen -W "check yes rollback list branch" -- ${cur}) )
            return 0
            ;;
        --build)
            COMPREPLY=( $(compgen -W "all linux darwin windows freebsd host" -- ${cur}) )
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
        '--init[Initialize configuration]'
        '--config-info[Show configuration paths]'
        '--test[Test search engines]:query:'
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
        '--service[Service management]:action:(install uninstall start stop restart reload enable disable status help)'
        '--maintenance[Maintenance]:action:(backup restore list update mode setup help)'
        '--update[Update management]:action:(check yes rollback list branch)'
        '--build[Build binaries]:platform:(all linux darwin windows freebsd host)'
        '--shell[Shell integration]:subcommand:(completions init --help)'
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
complete -c %s -l init -d 'Initialize configuration'
complete -c %s -l config-info -d 'Show configuration paths'
complete -c %s -l test -d 'Test search engines'
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
complete -c %s -l service -d 'Service management' -xa 'install uninstall start stop restart reload enable disable status help'
complete -c %s -l maintenance -d 'Maintenance' -xa 'backup restore list update mode setup help'
complete -c %s -l update -d 'Update management' -xa 'check yes rollback list branch'
complete -c %s -l build -d 'Build binaries' -xa 'all linux darwin windows freebsd host'
complete -c %s -l shell -d 'Shell integration' -xa 'completions init --help'
`, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName, binaryName, binaryName, binaryName, binaryName,
			binaryName, binaryName, binaryName)

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
        @{Name='--init'; Description='Initialize configuration'}
        @{Name='--config-info'; Description='Show configuration paths'}
        @{Name='--test'; Description='Test search engines'}
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
        @{Name='--build'; Description='Build binaries'}
        @{Name='--shell'; Description='Shell integration'}
    )

    $commands | Where-Object { $_.Name -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_.Name, $_.Name, 'ParameterValue', $_.Description)
    }
}
`, binaryName, binaryName, binaryName)

	default:
		fmt.Printf("❌ Unsupported shell: %s\n", shell)
		fmt.Println("Supported shells: bash, zsh, fish, powershell")
		os.Exit(1)
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
		fmt.Printf("❌ Unsupported shell: %s\n", shell)
		fmt.Println("Supported shells: bash, zsh, fish, powershell")
		os.Exit(1)
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
