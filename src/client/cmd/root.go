// Package cmd implements CLI commands for the search client
// Per AI.md PART 36: CLI Client implementation
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/client/paths"
	"github.com/apimgr/search/src/client/tui"
)

var (
	// Build info - set via -ldflags at build time
	ProjectName = "search"
	Version     = "dev"
	CommitID    = "unknown"
	BuildDate   = "unknown"

	cfgFile   string
	server    string
	token     string
	tokenFile string // Per AI.md PART 36: --token-file flag
	userCtx   string // Per AI.md PART 36: --user flag for user/org context
	output    string
	noColor   bool
	timeout   int
	debugMode bool
	page      int
	limit     int

	apiClient *api.Client
)

var rootCmd = &cobra.Command{
	Use:   getBinaryName() + " [query]",
	Short: "CLI client for Search API",
	Long:  `search-cli is a command-line interface for interacting with the Search API server.`,
	// Per AI.md PART 36 line 42776: Bare args = search term
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Per AI.md PART 36 line 40972-40978: Mode detection
		// No args + interactive terminal = TUI mode
		// Args provided = CLI mode (search)
		if len(args) == 0 {
			if term.IsTerminal(int(os.Stdout.Fd())) {
				// TUI mode - launch TUI
				return runTUI()
			}
			// Non-interactive with no args = error
			return fmt.Errorf("no search query provided")
		}

		// CLI mode - perform search
		return runSearch(strings.Join(args, " "))
	},
}

// runTUI launches the TUI interface
// Per AI.md PART 36: TUI mode when no args in interactive terminal
func runTUI() error {
	// Initialize client before launching TUI
	if err := initClient(); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}
	return tui.Run(apiClient)
}

// runSearch performs a search with the given query
func runSearch(query string) error {
	// Initialize client if not already done
	if apiClient == nil {
		if err := initClient(); err != nil {
			return err
		}
	}

	result, err := apiClient.Search(query, page, limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	switch getOutputFormat() {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "plain":
		for _, r := range result.Results {
			fmt.Printf("%s\n%s\n\n", r.Title, r.URL)
		}
	default:
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "TITLE\tURL\tSCORE\n")
		for _, r := range result.Results {
			title := r.Title
			if len(title) > 50 {
				title = title[:47] + "..."
			}
			fmt.Fprintf(w, "%s\t%s\t%.2f\n", title, r.URL, r.Score)
		}
		w.Flush()
		fmt.Printf("\nTotal: %d results\n", result.TotalCount)
	}
	return nil
}

// initClient initializes the API client with cluster failover support
// Per AI.md PART 36 line 42791-42856: Cluster failover and server resolution
func initClient() error {
	// Per AI.md PART 36 line 42820-42856: Server address resolution priority
	// 1. --server flag (if provided)
	// 2. server.primary in cli.yml (if set)
	// 3. server.cluster nodes (if primary fails) - handled by client
	// 4. Error with setup instructions (no official site)
	serverAddr, shouldSave := resolveServerAddress()
	if serverAddr == "" {
		return fmt.Errorf(`no server configured

To configure a server, run:
  %s --server https://your-server.example.com list

This will save the server address for future commands.
Or edit ~/.config/apimgr/%s/cli.yml directly.`, getBinaryName(), ProjectName)
	}

	// Per AI.md PART 36 line 41436-41469: Token sources (priority order)
	tokenVal := getToken()

	timeoutVal := viper.GetInt("server.timeout")
	if timeout > 0 {
		timeoutVal = timeout
	}
	if timeoutVal == 0 {
		timeoutVal = 30
	}

	api.ProjectName = ProjectName
	api.Version = Version
	apiClient = api.NewClient(serverAddr, tokenVal, timeoutVal)

	// Load cluster nodes from config
	clusterNodes := viper.GetStringSlice("server.cluster")
	if len(clusterNodes) > 0 {
		apiClient.SetClusterNodes(clusterNodes)
	}

	// Set user context if provided
	if userCtx != "" {
		apiClient.SetUserContext(userCtx)
	}

	// Per AI.md PART 36 line 42834: Save --server flag to config if empty
	if shouldSave {
		saveServerToConfig(serverAddr)
	}

	// Per AI.md PART 36 line 42797-42811: Background autodiscover
	go backgroundAutodiscover()

	return nil
}

// resolveServerAddress resolves server address with priority order
// Per AI.md PART 36 line 42820-42856: Server address resolution
// Returns server address and whether to save it to config
func resolveServerAddress() (string, bool) {
	// 1. --server flag (if provided)
	if server != "" {
		// Per AI.md PART 36 line 42834: Save to config if current is empty
		currentPrimary := viper.GetString("server.primary")
		if currentPrimary == "" {
			// Also check legacy address key
			currentPrimary = viper.GetString("server.address")
		}
		return server, currentPrimary == ""
	}

	// 2. server.primary in cli.yml
	if primary := viper.GetString("server.primary"); primary != "" {
		return primary, false
	}
	// Also check legacy address key
	if addr := viper.GetString("server.address"); addr != "" {
		return addr, false
	}

	// 3. No server configured
	return "", false
}

// saveServerToConfig saves server address to config file
// Per AI.md PART 36 line 42834: Save --server to server.primary
func saveServerToConfig(serverAddr string) {
	viper.Set("server.primary", serverAddr)
	configPath := paths.ConfigFile()
	_ = viper.WriteConfigAs(configPath)
}

// backgroundAutodiscover performs autodiscovery in background
// Per AI.md PART 36 line 42797-42811: Background node discovery
func backgroundAutodiscover() {
	if apiClient == nil {
		return
	}

	// Call /api/autodiscover in background
	discover, err := apiClient.Autodiscover()
	if err != nil {
		// Silent failure - autodiscover is best-effort
		return
	}

	// Update config with discovered cluster nodes (async, non-blocking)
	if len(discover.Cluster.Nodes) > 0 {
		updateClusterConfig(discover.Cluster.Primary, discover.Cluster.Nodes)
	}
}

// updateClusterConfig updates cluster configuration
// Per AI.md PART 36 line 42811: Update server.primary and server.cluster in cli.yml
func updateClusterConfig(primary string, nodes []string) {
	// Update in-memory config
	if primary != "" {
		viper.Set("server.primary", primary)
	}
	if len(nodes) > 0 {
		viper.Set("server.cluster", nodes)
	}

	// Save to config file (non-blocking)
	configPath := paths.ConfigFile()
	_ = viper.WriteConfigAs(configPath)
}

// getToken returns the API token following PART 36 priority order
// Per AI.md PART 36 line 41436-41469
func getToken() string {
	// 1. --token flag (explicit)
	if token != "" {
		return token
	}

	// 2. --token-file flag (file path)
	if tokenFile != "" {
		if data, err := os.ReadFile(tokenFile); err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	// 3. Environment variable: SEARCH_TOKEN
	if envToken := os.Getenv("SEARCH_TOKEN"); envToken != "" {
		return envToken
	}

	// 4. Config file: cli.yml -> token
	if cfgToken := viper.GetString("server.token"); cfgToken != "" {
		return cfgToken
	}

	// 5. Default token file: {config_dir}/token
	tokenPath := filepath.Join(paths.ConfigDir(), "token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		return strings.TrimSpace(string(data))
	}

	return "" // No token (anonymous access if allowed)
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Per AI.md PART 36 line 42627: -h and -v are FLAGS, not commands
	// Per AI.md PART 36 line 42563-42564: Only -h and -v have short flags
	rootCmd.Flags().BoolP("version", "v", false, "Show version")
	rootCmd.Flags().BoolP("help", "h", false, "Show help")

	// Long-form flags only (no short flags per PART 36)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().StringVar(&server, "server", "", "server address")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "API token")
	rootCmd.PersistentFlags().StringVar(&tokenFile, "token-file", "", "read token from file")
	rootCmd.PersistentFlags().StringVar(&userCtx, "user", "", "user or org context (@user, +org, or auto-detect)")
	rootCmd.PersistentFlags().StringVar(&output, "output", "table", "output format: json, table, plain")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 0, "request timeout in seconds")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug output")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "page number")
	rootCmd.PersistentFlags().IntVar(&limit, "limit", 10, "results per page")

	// Custom version handling
	rootCmd.SetVersionTemplate(fmt.Sprintf("%s %s (%s) built %s\n", getBinaryName(), Version, CommitID, BuildDate))
	rootCmd.Version = Version
}

func initConfig() {
	// Per AI.md PART 36: CLI Startup Sequence (NON-NEGOTIABLE)
	// Ensure all CLI directories exist with correct permissions
	if err := paths.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create CLI directories: %v\n", err)
	}

	if cfgFile != "" {
		// Resolve config path (handles relative/absolute paths and extensions)
		resolvedPath, err := paths.ResolveConfigPath(cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid config path: %v\n", err)
		} else {
			viper.SetConfigFile(resolvedPath)
		}
	} else {
		// Per AI.md PART 36 line 40579: Config at ~/.config/apimgr/search/cli.yml
		configDir := paths.ConfigDir()
		viper.AddConfigPath(configDir)
		viper.SetConfigName("cli")
		viper.SetConfigType("yaml")
	}

	// Per AI.md PART 36 lines 42709-42772: Full cli.yml configuration
	// Server connection defaults
	viper.SetDefault("server.primary", "")
	viper.SetDefault("server.cluster", []string{})
	viper.SetDefault("server.api_version", "v1")
	viper.SetDefault("server.admin_path", "admin")
	viper.SetDefault("server.timeout", 30)
	viper.SetDefault("server.retry", 3)
	viper.SetDefault("server.retry_delay", 1)

	// Authentication defaults (legacy keys for backwards compat)
	viper.SetDefault("server.address", "")
	viper.SetDefault("server.token", "")
	viper.SetDefault("auth.token", "")
	viper.SetDefault("auth.token_file", "")

	// Output preferences defaults
	viper.SetDefault("output.format", "table")
	viper.SetDefault("output.color", "auto")
	viper.SetDefault("output.pager", "auto")
	viper.SetDefault("output.quiet", false)
	viper.SetDefault("output.verbose", false)

	// TUI preferences defaults
	viper.SetDefault("tui.enabled", true)
	viper.SetDefault("tui.theme", "dark")
	viper.SetDefault("tui.mouse", true)
	viper.SetDefault("tui.unicode", true)

	// Logging defaults - Per AI.md PART 36 lines 42749-42755
	viper.SetDefault("logging.level", "warn")
	viper.SetDefault("logging.file", "")
	viper.SetDefault("logging.max_size", 10)
	viper.SetDefault("logging.max_files", 5)

	// Cache defaults - Per AI.md PART 36 lines 42756-42760
	viper.SetDefault("cache.enabled", true)
	viper.SetDefault("cache.ttl", 300)
	viper.SetDefault("cache.max_size", 100)

	// Debug default
	viper.SetDefault("debug", false)

	viper.ReadInConfig()
}

func getBinaryName() string {
	return filepath.Base(os.Args[0])
}

func getOutputFormat() string {
	if output != "" {
		return output
	}
	return viper.GetString("output.format")
}
