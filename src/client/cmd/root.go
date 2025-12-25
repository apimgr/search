package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/apimgr/search/src/client/api"
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
	output    string
	noColor   bool
	timeout   int
	tuiMode   bool

	apiClient *api.Client
)

var rootCmd = &cobra.Command{
	Use:   getBinaryName(),
	Short: "CLI client for Search API",
	Long:  `search-cli is a command-line interface for interacting with the Search API server.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip client init for config and version commands
		if cmd.Name() == "config" || cmd.Name() == "version" || cmd.Parent().Name() == "config" {
			return nil
		}

		// Initialize API client
		serverAddr := viper.GetString("server.address")
		if server != "" {
			serverAddr = server
		}
		if serverAddr == "" {
			return fmt.Errorf("server address not configured. Use --server or run 'config set server.address <url>'")
		}

		tokenVal := viper.GetString("server.token")
		if token != "" {
			tokenVal = token
		}
		// Also check environment variable
		if envToken := os.Getenv("SEARCH_CLI_TOKEN"); envToken != "" && tokenVal == "" {
			tokenVal = envToken
		}

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
		return nil
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&server, "server", "s", "", "server address")
	rootCmd.PersistentFlags().StringVarP(&token, "token", "t", "", "API token")
	rootCmd.PersistentFlags().StringVarP(&output, "output", "o", "table", "output format: json, table, plain")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colored output")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 0, "request timeout in seconds")
	rootCmd.PersistentFlags().BoolVar(&tuiMode, "tui", false, "launch TUI mode")

	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(tuiCmd)
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		configDir := filepath.Join(home, ".config", "search")
		os.MkdirAll(configDir, 0755)
		viper.AddConfigPath(configDir)
		viper.SetConfigName("cli")
		viper.SetConfigType("yaml")
	}

	// Defaults
	viper.SetDefault("server.address", "")
	viper.SetDefault("server.token", "")
	viper.SetDefault("server.timeout", 30)
	viper.SetDefault("output.format", "table")
	viper.SetDefault("output.color", "auto")

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
