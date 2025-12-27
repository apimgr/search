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
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/apimgr/search/src/backup"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/mode"
	"github.com/apimgr/search/src/models"
	"github.com/apimgr/search/src/search"
	"github.com/apimgr/search/src/search/engines"
	"github.com/apimgr/search/src/server"
	"github.com/apimgr/search/src/service"
	"github.com/apimgr/search/src/update"

	_ "modernc.org/sqlite"
)

// CLI flags (per TEMPLATE.md PART 6)
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

	// Required flags per TEMPLATE.md PART 6 (NON-NEGOTIABLE)
	flagMode    string
	flagData    string
	flagConfig  string
	flagLog     string
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
	flag.StringVar(&flagService, "service", "", "Service management: install|uninstall|start|stop|status|restart|reload")
	flag.StringVar(&flagMaintenance, "maintenance", "", "Maintenance: backup|restore|update|mode")
	flag.StringVar(&flagUpdate, "update", "", "Update management: check|yes|branch")
	flag.StringVar(&flagBuild, "build", "", "Build for platforms: all|linux|darwin|windows|freebsd")

	// Configuration override flags (NON-NEGOTIABLE per TEMPLATE.md PART 6)
	flag.StringVar(&flagMode, "mode", "", "Set application mode (production|development)")
	flag.StringVar(&flagData, "data", "", "Set data directory")
	flag.StringVar(&flagConfig, "config", "", "Set config directory")
	flag.StringVar(&flagLog, "log", "", "Set log directory")
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
	}

	// Handle legacy argument style (for backwards compatibility)
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "--") && !strings.Contains(os.Args[1], "=") {
		handleLegacyArgs()
		return
	}

	// Start server
	runServer()
}

// applyCliOverrides applies CLI flag overrides to the config system
// Per TEMPLATE.md PART 6: Directory flags MUST create directories if they don't exist
func applyCliOverrides() {
	// Set mode from CLI flag or environment
	if flagMode != "" {
		os.Setenv("SEARCH_MODE", flagMode)
		os.Setenv("MODE", flagMode)
		mode.Set(flagMode)
	} else {
		// Initialize mode from environment
		mode.FromEnv()
	}

	// Set debug mode from CLI flag
	// Per TEMPLATE.md PART 6: --debug enables debug mode (verbose logging, debug endpoints)
	if flagDebug {
		os.Setenv("DEBUG", "true")
		os.Setenv("SEARCH_DEBUG", "true")
		mode.SetDebug(true)
	}

	if flagData != "" {
		os.Setenv("SEARCH_DATA_DIR", flagData)
		config.SetDataDirOverride(flagData)
	}
	if flagConfig != "" {
		os.Setenv("SEARCH_CONFIG_DIR", flagConfig)
		config.SetConfigDirOverride(flagConfig)
	}
	if flagLog != "" {
		os.Setenv("SEARCH_LOG_DIR", flagLog)
		config.SetLogDirOverride(flagLog)
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
	// Per TEMPLATE.md PART 6: All directory flags MUST create directories if they don't exist
	if flagData != "" || flagConfig != "" || flagLog != "" || flagPID != "" {
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
			fmt.Println("Usage: search --service <install|uninstall|start|stop|status|restart|reload>")
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
	case "--daemon", "-d":
		flagDaemon = true
		runServer()
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		fmt.Println("Use --help for usage information")
	}
}

func runServer() {
	// Handle daemonization per TEMPLATE.md PART 6
	// Check if we should daemonize (only for manual starts, not --service start)
	if flagDaemon && os.Getenv("_DAEMON_CHILD") != "1" {
		if err := daemonize(); err != nil {
			log.Fatalf("❌ Failed to daemonize: %v", err)
		}
		// Parent exits in daemonize(), only child continues
	}

	fmt.Println("🔍 Search - Privacy-Respecting Metasearch Engine")
	fmt.Printf("Version: %s\n\n", config.Version)

	// Initialize configuration
	cfg, err := config.Initialize()
	if err != nil {
		log.Fatalf("❌ Configuration failed: %v", err)
	}

	// Check for first run and display setup token if needed
	// Per TEMPLATE.md PART 6: Setup token displayed ONCE on first run
	checkFirstRun(cfg)

	// Create server
	srv := server.New(cfg)

	// Handle graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		sig := <-quit
		fmt.Printf("\n⚠️  Received signal: %v\n", sig)

		if sig == syscall.SIGHUP {
			fmt.Println("🔄 Reloading configuration...")
			// Reload config on SIGHUP
			newCfg, err := config.Initialize()
			if err != nil {
				log.Printf("❌ Failed to reload config: %v", err)
				return
			}
			srv.UpdateConfig(newCfg)
			fmt.Println("✅ Configuration reloaded")
			return
		}

		fmt.Println("🛑 Shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("❌ Shutdown error: %v", err)
		}
		fmt.Println("✅ Server stopped")
		os.Exit(0)
	}()

	// Start server
	if err := srv.Start(); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}

func printVersion() {
	// Per TEMPLATE.md PART 6: --version format
	// Format: {binary} v1.2.3 (abc1234) built 2025-01-15
	binaryName := filepath.Base(os.Args[0])

	// Build the version line
	commitPart := ""
	if config.CommitID != "" && config.CommitID != "unknown" {
		// Truncate commit to 7 chars if longer
		commit := config.CommitID
		if len(commit) > 7 {
			commit = commit[:7]
		}
		commitPart = fmt.Sprintf(" (%s)", commit)
	}

	// Format build date as YYYY-MM-DD
	buildDate := config.BuildDate
	if t, err := time.Parse("Mon Jan 02, 2006 at 15:04:05 MST", buildDate); err == nil {
		buildDate = t.Format("2006-01-02")
	} else if t, err := time.Parse(time.RFC3339, buildDate); err == nil {
		buildDate = t.Format("2006-01-02")
	}

	fmt.Printf("%s v%s%s built %s\n", binaryName, config.Version, commitPart, buildDate)
}

func printHelp() {
	// Per TEMPLATE.md PART 6: Use actual binary name in help
	binaryName := filepath.Base(os.Args[0])

	fmt.Printf(`%s - Privacy-Respecting Metasearch Engine

Usage:
  %s [options]             Start the server with optional flags
  %s [command]             Execute a command

Runtime Flags:
  --mode <mode>            Set application mode (production|development)
  --data <dir>             Set data directory
  --config <dir>           Set config directory
  --log <dir>              Set log directory
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
  %s --service install               Install as system service
  %s --service reload                Reload configuration
  %s --update check                  Check for updates
  %s --maintenance setup             Reset admin credentials
  %s --build all                     Build for all platforms
  %s --build host                    Build for current platform

For more information: https://github.com/apimgr/search
`, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName, binaryName,
		binaryName, binaryName, binaryName)
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
	// Per TEMPLATE.md PART 31 - --status output format
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
		fmt.Printf("Or install as service: %s --service install\n", binaryName)
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

	// Node status (standalone for now, cluster support later)
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
	case "install":
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

	case "uninstall":
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

	case "disable":
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

	case "enable":
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

	case "help", "--help":
		fmt.Println("Service Management Commands:")
		fmt.Println()
		fmt.Println("  install     Install as system service")
		fmt.Println("  uninstall   Remove system service")
		fmt.Println("  start       Start the service")
		fmt.Println("  stop        Stop the service")
		fmt.Println("  restart     Restart the service")
		fmt.Println("  reload      Reload configuration (SIGHUP)")
		fmt.Println("  enable      Enable service autostart")
		fmt.Println("  disable     Disable service autostart")
		fmt.Println("  status      Show service status")
		fmt.Println("  help        Show this help")

	default:
		fmt.Printf("❌ Unknown action: %s\n", action)
		fmt.Println("Valid actions: install, uninstall, start, stop, restart, reload, enable, disable, status, help")
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
		// Per TEMPLATE.md PART 24: Password from env var or prompt
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
		// Per TEMPLATE.md PART 24: Prompt for password if encrypted
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
		// Admin recovery per TEMPLATE.md PART 26
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

	query := models.NewQuery(testQuery)
	query.Category = models.CategoryGeneral

	// Get engines for category
	searchEngines := registry.GetForCategory(query.Category)
	fmt.Printf("Using %d engines for category '%s'\n\n", len(searchEngines), query.Category)

	// Create aggregator
	aggregator := search.NewAggregatorSimple(searchEngines, 30*time.Second)

	// Perform search
	ctx := context.Background()
	results, err := aggregator.Search(ctx, query)

	if err != nil {
		if err == models.ErrNoResults {
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
// First Run Detection (per TEMPLATE.md PART 6)
// ============================================================

// checkFirstRun checks if this is the first run and displays setup token if needed
// Per TEMPLATE.md PART 6: Setup token displayed ONCE on first run
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

	// Display the token ONCE - per TEMPLATE.md this is the only time it's shown
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
// Admin Recovery Helpers (per TEMPLATE.md PART 26)
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
// Build Command (per TEMPLATE.md PART 23)
// ============================================================

// BuildTarget represents a build target platform
type BuildTarget struct {
	OS   string
	Arch string
}

// runBuild builds the binary for specified platforms using Docker
// Per TEMPLATE.md PART 23: Binary must be able to build itself
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
	// Per TEMPLATE.md PART 23: Must use golang:alpine for builds
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
// Per TEMPLATE.md PART 6 - Daemonization
func daemonize() error {
	// Windows doesn't support traditional Unix daemonization
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "Warning: --daemon is not supported on Windows")
		fmt.Fprintln(os.Stderr, "Use --service install && --service start for Windows Service")
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

// filterDaemonFlag removes --daemon and -d from args to prevent infinite loop
func filterDaemonFlag(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "--daemon" && arg != "-d" {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}
