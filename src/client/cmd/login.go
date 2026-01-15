// Package cmd implements CLI commands for the search client
// Per AI.md PART 36: login command for interactive token storage
package cmd

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/apimgr/search/src/client/api"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login and save API token",
	Long: `Login by providing an API token and save it to the config directory.

The token is stored at ~/.config/apimgr/search/token (or the platform-appropriate location).

Examples:
  ` + getBinaryName() + ` login
  ` + getBinaryName() + ` login --token usr_abc123...`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogin()
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove saved API token",
	Long: `Remove the saved API token from the config directory.

Examples:
  ` + getBinaryName() + ` logout`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLogout()
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(logoutCmd)
}

// runLogin handles interactive token login
// Per AI.md PART 36 line 41481-41484
func runLogin() error {
	var tokenVal string

	// If token provided via flag, use it directly
	if token != "" {
		tokenVal = token
	} else {
		// Prompt for token interactively
		fmt.Print("Enter API token: ")

		// Read password without echo if terminal
		if term.IsTerminal(int(syscall.Stdin)) {
			tokenBytes, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return fmt.Errorf("failed to read token: %w", err)
			}
			fmt.Println() // newline after hidden input
			tokenVal = strings.TrimSpace(string(tokenBytes))
		} else {
			// Read from stdin (pipe)
			reader := bufio.NewReader(os.Stdin)
			line, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read token: %w", err)
			}
			tokenVal = strings.TrimSpace(line)
		}
	}

	if tokenVal == "" {
		return fmt.Errorf("token cannot be empty")
	}

	// Validate token format per AI.md PART 11
	// User tokens start with usr_, admin tokens with adm_
	if !strings.HasPrefix(tokenVal, "usr_") && !strings.HasPrefix(tokenVal, "adm_") {
		fmt.Println("Warning: Token does not have expected prefix (usr_ or adm_)")
	}

	// Get token file path
	tokenPath, err := getDefaultTokenPath()
	if err != nil {
		return fmt.Errorf("failed to get token path: %w", err)
	}

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(tokenPath), 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Write token to file with secure permissions
	if err := os.WriteFile(tokenPath, []byte(tokenVal+"\n"), 0600); err != nil {
		return fmt.Errorf("failed to save token: %w", err)
	}

	fmt.Printf("Token saved to %s\n", tokenPath)

	// Test connection if server is configured
	serverAddr := server
	if serverAddr == "" {
		serverAddr = getServerAddress()
	}
	if serverAddr != "" {
		fmt.Printf("Testing connection to %s...\n", serverAddr)
		if err := testToken(serverAddr, tokenVal); err != nil {
			fmt.Printf("Warning: Could not verify token: %v\n", err)
		} else {
			fmt.Println("Token verified successfully!")
		}
	}

	return nil
}

// runLogout removes the saved token
func runLogout() error {
	tokenPath, err := getDefaultTokenPath()
	if err != nil {
		return fmt.Errorf("failed to get token path: %w", err)
	}

	if _, err := os.Stat(tokenPath); os.IsNotExist(err) {
		fmt.Println("No saved token found")
		return nil
	}

	if err := os.Remove(tokenPath); err != nil {
		return fmt.Errorf("failed to remove token: %w", err)
	}

	fmt.Println("Token removed successfully")
	return nil
}

// getDefaultTokenPath returns the default token file path
// Per AI.md PART 36 line 41441, 41464
func getDefaultTokenPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "apimgr", "search", "token"), nil
}

// getServerAddress returns the configured server address
func getServerAddress() string {
	if server != "" {
		return server
	}
	// This is a simple fallback - full config loading happens in initConfig
	return ""
}

// testToken attempts to verify the token with the server
func testToken(serverAddr, tokenVal string) error {
	client := &api.Client{
		BaseURL: serverAddr,
		Token:   tokenVal,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	_, err := client.Health()
	return err
}
