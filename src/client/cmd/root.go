// Package cmd implements CLI commands for the search client
// Per AI.md PART 32: CLI Client implementation
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
	"github.com/apimgr/search/src/client/path"
	"github.com/apimgr/search/src/client/tui"
)

var (
	// Build info - set via -ldflags at build time
	// Per AI.md PART 26: LDFLAGS must include Version, CommitID, BuildDate, OfficialSite
	ProjectName = "search"
	Version     = "dev"
	CommitID    = "unknown"
	BuildDate   = "unknown"
	// Default server URL
	OfficialSite = "https://scour.li"

	cfgFile string
	server  string
	token   string
	// Per AI.md PART 32: --token-file flag
	tokenFile string
	// Per AI.md PART 32: --user flag for user/org context
	userCtx string
	output  string
	// Per AI.md PART 8: --color {auto|yes|no} (not --no-color bool)
	colorMode string
	// Per AI.md PART 8: --shell flag for shell completions
	shellFlag string
	// Per AI.md PART 8: --lang flag for output language
	langFlag  string
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
	// Per AI.md PART 32 line 42776: Bare args = search term
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Per AI.md PART 32 line 40972-40978: Mode detection
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
// Per AI.md PART 32: TUI mode when no args in interactive terminal
func runTUI() error {
	// Initialize client before launching TUI
	if err := initClient(); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}
	return tui.RunTUIApp(apiClient)
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
		fmt.Printf("\nTotal: %d results (page %d of %d)\n", result.Pagination.Total, result.Pagination.Page, result.Pagination.Pages)
	}
	return nil
}

// initClient initializes the API client
func initClient() error {
	// Server address resolution priority:
	// 1. --server flag (if provided)
	// 2. server.primary in cli.yml (if set)
	// 3. Error with setup instructions
	serverAddr, shouldSave := resolveServerAddress()
	if serverAddr == "" {
		return fmt.Errorf(`no server configured

To configure a server, run:
  //your-server.example.com list
  %s --server https:

This will save the server address for future commands.
Or edit ~/.config/apimgr/%s/cli.yml directly`, getBinaryName(), ProjectName)
	}

	// Per AI.md PART 32 line 41436-41469: Token sources (priority order)
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

	// Set user context if provided
	if userCtx != "" {
		apiClient.SetUserContext(userCtx)
	}

	// Per AI.md PART 32 line 42834: Save --server flag to config if empty
	if shouldSave {
		saveServerToConfig(serverAddr)
	}

	// Per AI.md PART 32 line 42797-42811: Background autodiscover
	go backgroundAutodiscover()

	return nil
}

// resolveServerAddress resolves server address with priority order.
// Per AI.md PART 32: priority is --server flag → SEARCH_SERVER env var → cli.yml → compiled default.
// Returns server address and whether to save it to config.
func resolveServerAddress() (string, bool) {
	// 1. --server flag (explicit override)
	if server != "" {
		// Per AI.md PART 32: save to config if current config is empty
		currentPrimary := viper.GetString("server.primary")
		if currentPrimary == "" {
			currentPrimary = viper.GetString("server.address")
		}
		return server, currentPrimary == ""
	}

	// 2. SEARCH_SERVER environment variable
	if envServer := os.Getenv("SEARCH_SERVER"); envServer != "" {
		return envServer, false
	}

	// 3. cli.yml → server.primary
	if primary := viper.GetString("server.primary"); primary != "" {
		return primary, false
	}
	// Also check legacy address key
	if addr := viper.GetString("server.address"); addr != "" {
		return addr, false
	}

	// 4. Compiled default (if set via ldflags)
	if OfficialSite != "" {
		return OfficialSite, false
	}

	return "", false
}

// saveServerToConfig saves server address to config file
// Per AI.md PART 32 line 42834: Save --server to server.primary
func saveServerToConfig(serverAddr string) {
	viper.Set("server.primary", serverAddr)
	configPath := path.ConfigFile()
	_ = viper.WriteConfigAs(configPath)
}

// backgroundAutodiscover performs autodiscovery and checks for CLI updates.
// Per AI.md PART 32: CLI checks cli_versions on every start; if current < available, notify user.
func backgroundAutodiscover() {
	if apiClient == nil {
		return
	}

	info, err := apiClient.Autodiscover()
	if err != nil || info == nil {
		return
	}

	// Per AI.md PART 32: check cli_min_version — refuse further requests if too old
	if info.CLIMinVersion != "" && Version != "dev" {
		if versionLessThan(Version, info.CLIMinVersion) {
			fmt.Fprintf(os.Stderr, "this CLI is too old; the server requires %s — run '%s --update yes' to upgrade.\n",
				info.CLIMinVersion, getBinaryName())
			os.Exit(1)
		}
	}

	// Per AI.md PART 32: notify user if a newer version is available for this platform
	if len(info.CLIVersions) == 0 || Version == "dev" {
		return
	}
	platformKey := api.CurrentPlatform()
	if latest, ok := info.CLIVersions[platformKey]; ok {
		if versionLessThan(Version, latest.Version) {
			fmt.Fprintf(os.Stderr, "A newer version of %s is available: %s (you have %s). Run '%s --update yes' to upgrade.\n",
				getBinaryName(), latest.Version, Version, getBinaryName())
		}
	}
}

// versionLessThan returns true when a is strictly less than b using semver-style comparison.
// Handles simple "major.minor.patch" strings; returns false on parse failure.
func versionLessThan(a, b string) bool {
	parse := func(v string) [3]int {
		v = strings.TrimPrefix(v, "v")
		parts := strings.SplitN(v, ".", 3)
		var nums [3]int
		for i, p := range parts {
			if i >= 3 {
				break
			}
			fmt.Sscanf(p, "%d", &nums[i])
		}
		return nums
	}
	va, vb := parse(a), parse(b)
	for i := range va {
		if va[i] < vb[i] {
			return true
		}
		if va[i] > vb[i] {
			return false
		}
	}
	return false
}

// getToken returns the API token following PART 32 priority order
// Per AI.md PART 32 line 41436-41469
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
	tokenPath := filepath.Join(path.ConfigDir(), "token")
	if data, err := os.ReadFile(tokenPath); err == nil {
		return strings.TrimSpace(string(data))
	}

	// No token (anonymous access if allowed)
	return ""
}

func ExecuteClientCLI() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Per AI.md PART 32 line 42627: -h and -v are FLAGS, not commands
	// Per AI.md PART 32 line 42563-42564: Only -h and -v have short flags
	rootCmd.Flags().BoolP("version", "v", false, "Show version")
	rootCmd.Flags().BoolP("help", "h", false, "Show help")

	// Long-form flags only (no short flags per PART 32)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file path")
	rootCmd.PersistentFlags().StringVar(&server, "server", "", "server address")
	rootCmd.PersistentFlags().StringVar(&token, "token", "", "API token")
	rootCmd.PersistentFlags().StringVar(&tokenFile, "token-file", "", "read token from file")
	rootCmd.PersistentFlags().StringVar(&userCtx, "user", "", "user or org context (@user, +org, or auto-detect)")
	rootCmd.PersistentFlags().StringVar(&output, "output", "table", "output format: json, table, plain")
	// Per AI.md PART 8: --color {auto|yes|no} (not --no-color bool); NO_COLOR env is also respected
	rootCmd.PersistentFlags().StringVar(&colorMode, "color", "auto", "color output: auto, yes, no")
	// Per AI.md PART 8: --shell flag for shell completions (mirrors server binary)
	rootCmd.PersistentFlags().StringVar(&shellFlag, "shell", "", "shell integration: completions|init|--help")
	// Per AI.md PART 8: --lang flag for output language
	rootCmd.PersistentFlags().StringVar(&langFlag, "lang", "", "output language code (e.g. en, es, zh, fr, ar, de, ja)")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 0, "request timeout in seconds")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "enable debug output")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "page number")
	rootCmd.PersistentFlags().IntVar(&limit, "limit", 10, "results per page")

	// Custom version handling
	rootCmd.SetVersionTemplate(fmt.Sprintf("%s %s (%s) built %s\n", getBinaryName(), Version, CommitID, BuildDate))
	rootCmd.Version = Version
}

func initConfig() {
	// Per AI.md PART 32: CLI Startup Sequence (NON-NEGOTIABLE)
	// Ensure all CLI directories exist with correct permissions
	if err := path.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create CLI directories: %v\n", err)
	}

	// Per AI.md PART 8: apply --lang flag (set env so i18n layer picks it up)
	if langFlag != "" {
		os.Setenv("SEARCH_LANG", langFlag)
		os.Setenv("LANG", langFlag)
	}

	// Per AI.md PART 8: apply --color / NO_COLOR priority chain
	// Priority: --color flag → config output.color → NO_COLOR env → TTY auto-detect
	applyColorMode()

	if cfgFile != "" {
		// Resolve config path (handles relative/absolute paths and extensions)
		resolvedPath, err := path.ResolveConfigPath(cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid config path: %v\n", err)
		} else {
			viper.SetConfigFile(resolvedPath)
		}
	} else {
		// Per AI.md PART 32 line 40579: Config at ~/.config/apimgr/search/cli.yml
		configDir := path.ConfigDir()
		viper.AddConfigPath(configDir)
		viper.SetConfigName("cli")
		viper.SetConfigType("yaml")
	}

	// Per AI.md PART 32 lines 42709-42772: Full cli.yml configuration
	// Server connection defaults
	viper.SetDefault("server.primary", "")
	viper.SetDefault("server.api_version", "v1")
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

	// Logging defaults - Per AI.md PART 32 lines 42749-42755
	viper.SetDefault("logging.level", "warn")
	viper.SetDefault("logging.file", "")
	viper.SetDefault("logging.max_size", 10)
	viper.SetDefault("logging.max_files", 5)

	// Cache defaults - Per AI.md PART 32 lines 42756-42760
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

// applyColorMode applies the NO_COLOR/--color priority chain per AI.md PART 8.
// Priority: --color flag → config output.color → NO_COLOR env var → TTY auto-detect.
// Sets SEARCH_COLOR env so downstream packages (TUI, formatters) pick it up.
func applyColorMode() {
	resolved := colorMode

	// 1. --color flag (already in colorMode from PersistentFlags)
	// Only override from config/env if flag is at its default "auto"
	if resolved == "auto" || resolved == "" {
		// 2. Config file: output.color
		if cfgColor := viper.GetString("output.color"); cfgColor != "" && cfgColor != "auto" {
			resolved = cfgColor
		}
	}

	if resolved == "auto" || resolved == "" {
		// 3. NO_COLOR env var: non-empty = disable colors
		if os.Getenv("NO_COLOR") != "" {
			resolved = "no"
		}
	}

	if resolved == "auto" || resolved == "" {
		// 4. TTY auto-detect
		if !term.IsTerminal(int(os.Stdout.Fd())) || os.Getenv("TERM") == "dumb" {
			resolved = "no"
		} else {
			resolved = "yes"
		}
	}

	os.Setenv("SEARCH_COLOR", resolved)
}
