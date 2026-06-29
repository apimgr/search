// Package cmd implements CLI commands for the search client
// Per AI.md PART 32: CLI Client implementation
package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/client/clicfg"
	"github.com/apimgr/search/src/client/path"
	"github.com/apimgr/search/src/client/tui"
)

// fatalVersionCh carries a non-nil error when the server mandates a newer CLI version.
// Buffered size 1 so backgroundAutodiscover never blocks; main.go drains it and exits.
var fatalVersionCh = make(chan error, 1)

// FatalVersionCh returns the channel that receives a fatal version error.
// main() must select on this channel and call os.Exit(1) when it fires.
func FatalVersionCh() <-chan error { return fatalVersionCh }

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

// command describes a CLI command in the stdlib-flag command tree.
// It replaces the previous cobra.Command usage while preserving the metadata
// fields the tests inspect (Use, Short, Long) and flag lookup behavior.
type command struct {
	// Use is the usage string (e.g. "status" or "search [query]").
	Use string
	// Short is a one-line description.
	Short string
	// Long is the full help text.
	Long string
	// run executes the command with the remaining positional args.
	run func(args []string) error
	// flags holds the persistent flags registered on the root command.
	flags *flag.FlagSet
	// localFlags holds command-local flags such as --version and --help.
	localFlags *flag.FlagSet
	// subcommands maps a subcommand name to its command.
	subcommands map[string]*command
}

// PersistentFlags returns the persistent flag set, mirroring the cobra method
// used by the tests to verify registered flags.
func (c *command) PersistentFlags() *flag.FlagSet {
	return c.flags
}

// Flags returns the command-local flag set, mirroring the cobra method used by
// the tests to verify --version and --help registration.
func (c *command) Flags() *flag.FlagSet {
	return c.localFlags
}

// Commands returns the registered subcommands, mirroring the cobra method used
// by the tests to verify command registration.
func (c *command) Commands() []*command {
	cmds := make([]*command, 0, len(c.subcommands))
	for _, sub := range c.subcommands {
		cmds = append(cmds, sub)
	}
	return cmds
}

// rootCmd is the top-level CLI command.
var rootCmd = newRootCommand()

// newRootCommand constructs the root command, registers all flags, and wires
// the search/TUI behavior. Per AI.md PART 32: bare args = search term,
// no args + interactive terminal = TUI mode.
func newRootCommand() *command {
	c := &command{
		Use:         getBinaryName() + " [query]",
		Short:       "CLI client for Search API",
		Long:        `search-cli is a command-line interface for interacting with the Search API server.`,
		subcommands: make(map[string]*command),
	}

	// Persistent flags (apply to the root command and subcommands).
	fs := flag.NewFlagSet(getBinaryName(), flag.ContinueOnError)
	// Suppress flag package's default error printing; we report errors ourselves.
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfgFile, "config", "", "config file path")
	fs.StringVar(&server, "server", "", "server address")
	fs.StringVar(&token, "token", "", "API token")
	fs.StringVar(&tokenFile, "token-file", "", "read token from file")
	fs.StringVar(&userCtx, "user", "", "user or org context (@user, +org, or auto-detect)")
	fs.StringVar(&output, "output", "table", "output format: json, table, plain")
	// Per AI.md PART 8: --color {auto|yes|no} (not --no-color bool); NO_COLOR env is also respected
	fs.StringVar(&colorMode, "color", "auto", "color output: auto, yes, no")
	// Per AI.md PART 8: --shell flag for shell completions (mirrors server binary)
	fs.StringVar(&shellFlag, "shell", "", "shell integration: completions|init|--help")
	// Per AI.md PART 8: --lang flag for output language
	fs.StringVar(&langFlag, "lang", "", "output language code (e.g. en, es, zh, fr, ar, de, ja)")
	fs.IntVar(&timeout, "timeout", 0, "request timeout in seconds")
	fs.BoolVar(&debugMode, "debug", false, "enable debug output")
	fs.IntVar(&page, "page", 1, "page number")
	fs.IntVar(&limit, "limit", 10, "results per page")
	c.flags = fs

	// Command-local flags: -h/--help and -v/--version.
	// Per AI.md PART 32 line 42627: -h and -v are FLAGS, not commands.
	lfs := flag.NewFlagSet("root-local", flag.ContinueOnError)
	lfs.Bool("version", false, "Show version")
	lfs.Bool("v", false, "Show version")
	lfs.Bool("help", false, "Show help")
	lfs.Bool("h", false, "Show help")
	c.localFlags = lfs

	c.run = func(args []string) error {
		// Per AI.md PART 32 line 40972-40978: Mode detection.
		// No args + interactive terminal = TUI mode; args provided = CLI search.
		if len(args) == 0 {
			if term.IsTerminal(int(os.Stdout.Fd())) {
				return runTUI()
			}
			// Non-interactive with no args = error
			return fmt.Errorf("no search query provided")
		}
		return runSearch(strings.Join(args, " "))
	}

	return c
}

// addCommand registers a subcommand under the root command.
func (c *command) addCommand(name string, sub *command) {
	c.subcommands[name] = sub
}

// versionString renders the version banner per the previous cobra template.
func versionString() string {
	return fmt.Sprintf("%s %s (%s) built %s\n", getBinaryName(), Version, CommitID, BuildDate)
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
		return fmt.Errorf("no server configured; run %s to complete setup", getBinaryName())
	}

	// Per AI.md PART 32 line 41436-41469: Token sources (priority order)
	tokenVal := getToken()

	timeoutVal := clicfg.GetInt("server.timeout")
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
		currentPrimary := clicfg.GetString("server.primary")
		if currentPrimary == "" {
			currentPrimary = clicfg.GetString("server.address")
		}
		return server, currentPrimary == ""
	}

	// 2. SEARCH_SERVER environment variable
	if envServer := os.Getenv("SEARCH_SERVER"); envServer != "" {
		return envServer, false
	}

	// 3. cli.yml → server.primary
	if primary := clicfg.GetString("server.primary"); primary != "" {
		return primary, false
	}
	// Also check legacy address key
	if addr := clicfg.GetString("server.address"); addr != "" {
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
	clicfg.Set("server.primary", serverAddr)
	configPath, _ := path.ConfigFile()
	_ = clicfg.WriteConfigAs(configPath)
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
			msg := fmt.Sprintf("this CLI is too old; the server requires %s — run '%s --update yes' to upgrade.",
				info.CLIMinVersion, getBinaryName())
			fmt.Fprintln(os.Stderr, msg)
			// Signal main() to call os.Exit(1); os.Exit is forbidden outside main per AI.md PART 7.
			fatalVersionCh <- fmt.Errorf("%s", msg)
			return
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
	if cfgToken := clicfg.GetString("server.token"); cfgToken != "" {
		return cfgToken
	}

	// 5. Default token file: {config_dir}/token
	configDir, _ := path.ConfigDir()
	if configDir != "" {
		tokenPath := filepath.Join(configDir, "token")
		if data, err := os.ReadFile(tokenPath); err == nil {
			return strings.TrimSpace(string(data))
		}
	}

	// No token (anonymous access if allowed)
	return ""
}

// ExecuteClientCLI is the CLI entry point. It parses os.Args, applies global
// configuration, dispatches to a subcommand if present, and otherwise runs the
// root search/TUI behavior. It returns an error on failure (handled by main).
func ExecuteClientCLI() error {
	return executeArgs(os.Args[1:])
}

// executeArgs runs the command tree against the provided argument slice.
// It is separated from ExecuteClientCLI so behavior can be exercised in tests.
func executeArgs(args []string) error {
	// Detect -h/--help and -v/--version before flag parsing so they work in any
	// position, mirroring the previous cobra short-flag handling.
	for _, a := range args {
		switch a {
		case "-h", "--help":
			printRootHelp()
			return nil
		case "-v", "--version":
			fmt.Print(versionString())
			return nil
		}
	}

	if err := rootCmd.flags.Parse(args); err != nil {
		return err
	}

	// Initialize configuration (defaults, file, color, lang).
	initConfig()

	rest := rootCmd.flags.Args()

	// Dispatch to a subcommand if the first positional matches one.
	if len(rest) > 0 {
		if sub, ok := rootCmd.subcommands[rest[0]]; ok {
			return sub.run(rest[1:])
		}
	}

	return rootCmd.run(rest)
}

// printRootHelp prints the root command usage and flag descriptions.
func printRootHelp() {
	fmt.Printf("%s\n\n%s\n\nUsage:\n  %s\n\nFlags:\n", rootCmd.Short, rootCmd.Long, rootCmd.Use)
	rootCmd.flags.SetOutput(os.Stdout)
	rootCmd.flags.PrintDefaults()
	if len(rootCmd.subcommands) > 0 {
		fmt.Printf("\nCommands:\n")
		for name, sub := range rootCmd.subcommands {
			fmt.Printf("  %-12s %s\n", name, sub.Short)
		}
	}
}

// initConfig sets defaults, loads the config file, and applies lang/color.
// Per AI.md PART 32: CLI Startup Sequence (NON-NEGOTIABLE).
func initConfig() {
	// Ensure all CLI directories exist with correct permissions.
	if err := path.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create CLI directories: %v\n", err)
	}

	// Per AI.md PART 8: apply --lang flag (set env so i18n layer picks it up).
	if langFlag != "" {
		os.Setenv("SEARCH_LANG", langFlag)
		os.Setenv("LANG", langFlag)
	}

	// Per AI.md PART 8: apply --color / NO_COLOR priority chain.
	// Priority: --color flag → config output.color → NO_COLOR env → TTY auto-detect.
	applyColorMode()

	if cfgFile != "" {
		// Resolve config path (handles relative/absolute paths and extensions).
		resolvedPath, err := path.ResolveConfigPath(cfgFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid config path: %v\n", err)
		} else {
			clicfg.SetConfigFile(resolvedPath)
		}
	} else {
		// Per AI.md PART 32 line 40579: Config at ~/.config/apimgr/search/cli.yml.
		configDir, _ := path.ConfigDir()
		clicfg.AddConfigPath(configDir)
		clicfg.SetConfigName("cli")
		clicfg.SetConfigType("yaml")
	}

	// Per AI.md PART 32 lines 42709-42772: Full cli.yml configuration.
	// Server connection defaults.
	clicfg.SetDefault("server.primary", "")
	clicfg.SetDefault("server.api_version", "v1")
	clicfg.SetDefault("server.timeout", 30)
	clicfg.SetDefault("server.retry", 3)
	clicfg.SetDefault("server.retry_delay", 1)

	// Authentication defaults (legacy keys for backwards compat).
	clicfg.SetDefault("server.address", "")
	clicfg.SetDefault("server.token", "")
	clicfg.SetDefault("auth.token", "")
	clicfg.SetDefault("auth.token_file", "")

	// Output preferences defaults.
	clicfg.SetDefault("output.format", "table")
	clicfg.SetDefault("output.color", "auto")
	clicfg.SetDefault("output.pager", "auto")
	clicfg.SetDefault("output.quiet", false)
	clicfg.SetDefault("output.verbose", false)

	// TUI preferences defaults.
	clicfg.SetDefault("tui.enabled", true)
	clicfg.SetDefault("tui.theme", "dark")
	clicfg.SetDefault("tui.mouse", true)
	clicfg.SetDefault("tui.unicode", true)

	// Logging defaults - Per AI.md PART 32 lines 42749-42755.
	clicfg.SetDefault("logging.level", "warn")
	clicfg.SetDefault("logging.file", "")
	clicfg.SetDefault("logging.max_size", 10)
	clicfg.SetDefault("logging.max_files", 5)

	// Cache defaults - Per AI.md PART 32 lines 42756-42760.
	clicfg.SetDefault("cache.enabled", true)
	clicfg.SetDefault("cache.ttl", 300)
	clicfg.SetDefault("cache.max_size", 100)

	// Debug default.
	clicfg.SetDefault("debug", false)

	clicfg.ReadInConfig()
}

func getBinaryName() string {
	return filepath.Base(os.Args[0])
}

func getOutputFormat() string {
	if output != "" {
		return output
	}
	return clicfg.GetString("output.format")
}

// applyColorMode applies the NO_COLOR/--color priority chain per AI.md PART 8.
// Priority: --color flag → config output.color → NO_COLOR env var → TTY auto-detect.
// Sets SEARCH_COLOR env so downstream packages (TUI, formatters) pick it up.
func applyColorMode() {
	resolved := colorMode

	// 1. --color flag (already in colorMode from flags).
	// Only override from config/env if flag is at its default "auto".
	if resolved == "auto" || resolved == "" {
		// 2. Config file: output.color.
		if cfgColor := clicfg.GetString("output.color"); cfgColor != "" && cfgColor != "auto" {
			resolved = cfgColor
		}
	}

	if resolved == "auto" || resolved == "" {
		// 3. NO_COLOR env var: non-empty = disable colors.
		if os.Getenv("NO_COLOR") != "" {
			resolved = "no"
		}
	}

	if resolved == "auto" || resolved == "" {
		// 4. TTY auto-detect.
		if !term.IsTerminal(int(os.Stdout.Fd())) || os.Getenv("TERM") == "dumb" {
			resolved = "no"
		} else {
			resolved = "yes"
		}
	}

	os.Setenv("SEARCH_COLOR", resolved)
}
