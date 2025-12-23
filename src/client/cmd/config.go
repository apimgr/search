package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage CLI configuration",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		settings := viper.AllSettings()
		out, err := yaml.Marshal(settings)
		if err != nil {
			return err
		}
		fmt.Print(string(out))
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := args[1]

		viper.Set(key, value)

		configPath := getConfigPath()
		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return err
		}

		if err := viper.WriteConfigAs(configPath); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Set %s = %s\n", key, value)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := viper.Get(key)
		if value == nil {
			return fmt.Errorf("key not found: %s", key)
		}
		fmt.Println(value)
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		configPath := getConfigPath()

		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("config already exists: %s", configPath)
		}

		if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
			return err
		}

		defaultConfig := `# Search CLI configuration
server:
  address: ""
  token: ""
  timeout: 30

output:
  format: table
  color: auto

tui:
  theme: default
  show_hints: true
`
		if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
			return err
		}

		fmt.Printf("Created config file: %s\n", configPath)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configInitCmd)
}

func getConfigPath() string {
	if cfgFile != "" {
		return cfgFile
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "search", "cli.yml")
}

func formatKey(key string) string {
	return strings.ReplaceAll(key, ".", "_")
}
