// Package banner provides responsive startup banner printing
// Per AI.md PART 7: BINARY REQUIREMENTS - Common Go Modules
package banner

import (
	"fmt"
	"strings"

	"github.com/apimgr/search/src/display"
	"github.com/apimgr/search/src/terminal"
)

const boxWidth = 72 // Per AI.md PART 14 console output format

// Config holds banner configuration
type Config struct {
	AppName     string
	Version     string
	Mode        string   // production/development
	Debug       bool
	URLs        []string // Listen URLs
	ShowSetup   bool     // Show setup token (server only, first run)
	SetupToken  string
	AdminPath   string
	Description string
	SMTPStatus  string // SMTP status (e.g., "Auto-detected (localhost:25)")
}

// Print prints the banner based on terminal size
// Per AI.md PART 7 and PART 14: Console Output (First Run)
func Print(cfg Config) {
	size := terminal.GetSize()
	printWithSize(cfg, size.Mode)
}

// printWithSize prints the banner for the given size mode (internal, testable)
func printWithSize(cfg Config, mode terminal.SizeMode) {
	switch {
	case mode >= terminal.SizeModeStandard:
		printFull(cfg)
	case mode >= terminal.SizeModeCompact:
		printCompact(cfg)
	case mode >= terminal.SizeModeMinimal:
		printMinimal(cfg)
	default:
		printMicro(cfg)
	}
}

// printFull prints full boxed banner per AI.md PART 14
func printFull(cfg Config) {
	// Box characters
	topLeft := "╔"
	topRight := "╗"
	bottomLeft := "╚"
	bottomRight := "╝"
	horizontal := "═"
	vertical := "║"
	midLeft := "╠"
	midRight := "╣"

	hLine := strings.Repeat(horizontal, boxWidth-2)

	// Top border
	fmt.Println(topLeft + hLine + topRight)

	// Empty line
	printBoxLine(vertical, "", boxWidth)

	// App name and version
	title := fmt.Sprintf("%s %s", strings.ToUpper(cfg.AppName), cfg.Version)
	printBoxLine(vertical, "   "+title, boxWidth)

	// Empty line
	printBoxLine(vertical, "", boxWidth)

	// Status line
	status := "Status: Running"
	if cfg.ShowSetup {
		status = "Status: Running (first run - setup available)"
	}
	printBoxLine(vertical, "   "+status, boxWidth)

	// Empty line
	printBoxLine(vertical, "", boxWidth)

	// Mid separator
	fmt.Println(midLeft + hLine + midRight)

	// Empty line
	printBoxLine(vertical, "", boxWidth)

	// Web Interface URLs
	if len(cfg.URLs) > 0 {
		printBoxLine(vertical, "   "+display.Emoji("🌐", "[WEB]")+" Web Interface:", boxWidth)
		for _, url := range cfg.URLs {
			printBoxLine(vertical, "      "+url, boxWidth)
		}
		printBoxLine(vertical, "", boxWidth)
	}

	// Admin Panel
	if cfg.AdminPath != "" && len(cfg.URLs) > 0 {
		printBoxLine(vertical, "   "+display.Emoji("🔧", "[ADM]")+" Admin Panel:", boxWidth)
		adminURL := cfg.URLs[0] + "/" + cfg.AdminPath
		printBoxLine(vertical, "      "+adminURL, boxWidth)
		printBoxLine(vertical, "", boxWidth)
	}

	// Setup Token (first run only)
	if cfg.ShowSetup && cfg.SetupToken != "" {
		printBoxLine(vertical, "   "+display.Emoji("🔑", "[KEY]")+" Setup Token (use at /"+cfg.AdminPath+"):", boxWidth)
		printBoxLine(vertical, "      "+cfg.SetupToken, boxWidth)
		printBoxLine(vertical, "", boxWidth)
	}

	// SMTP Status
	if cfg.SMTPStatus != "" {
		printBoxLine(vertical, "   "+display.Emoji("📧", "[MAIL]")+" SMTP: "+cfg.SMTPStatus, boxWidth)
		printBoxLine(vertical, "", boxWidth)
	}

	// Warning for first run
	if cfg.ShowSetup {
		printBoxLine(vertical, "   "+display.Emoji("⚠️", "[!]")+"  Save the setup token! It will not be shown again.", boxWidth)
		printBoxLine(vertical, "", boxWidth)
	}

	// Bottom border
	fmt.Println(bottomLeft + hLine + bottomRight)
	fmt.Println()
}

// printBoxLine prints a line within the box with padding
func printBoxLine(border, content string, width int) {
	// Calculate padding: width - 2 borders - content length
	contentLen := len(content)
	// Account for emoji width (emojis are typically 2 chars wide but take 1 rune)
	// Simple approach: just pad to fill
	padding := width - 2 - contentLen
	if padding < 0 {
		padding = 0
		content = content[:width-2]
	}
	fmt.Printf("%s%s%s%s\n", border, content, strings.Repeat(" ", padding), border)
}

// printCompact prints compact banner (60-79 cols)
// Per AI.md PART 8: Uses display.Emoji() for NO_COLOR fallback
func printCompact(cfg Config) {
	fmt.Println()
	fmt.Printf("%s %s v%s\n", display.Emoji("🚀", "[*]"), cfg.AppName, cfg.Version)

	if cfg.Mode != "" {
		var icon string
		if cfg.Mode == "development" {
			icon = display.Emoji("🔧", "[DEV]")
		} else {
			icon = display.Emoji("🔒", "[PROD]")
		}
		fmt.Printf("%s Running in mode: %s\n", icon, cfg.Mode)
	}

	for _, url := range cfg.URLs {
		fmt.Printf("%s %s\n", display.Emoji("🌐", "[WEB]"), url)
	}

	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Println()
		fmt.Printf("%s Setup Token: %s\n", display.Emoji("🔑", "[KEY]"), cfg.SetupToken)
		fmt.Printf("%s  Save this token! Shown only once.\n", display.Emoji("⚠️", "[!]"))
	}
	fmt.Println()
}

// printMinimal prints minimal banner (40-59 cols)
func printMinimal(cfg Config) {
	fmt.Printf("%s %s\n", cfg.AppName, cfg.Version)
	if len(cfg.URLs) > 0 {
		// Extract just host:port
		fmt.Println(extractHostPort(cfg.URLs[0]))
	}
	if cfg.ShowSetup && cfg.SetupToken != "" {
		fmt.Printf("Token: %s\n", cfg.SetupToken)
	}
}

// printMicro prints micro banner (<40 cols)
func printMicro(cfg Config) {
	if len(cfg.URLs) > 0 {
		fmt.Printf("%s %s\n", cfg.AppName, extractHostPort(cfg.URLs[0]))
	} else {
		fmt.Println(cfg.AppName)
	}
}

// extractHostPort extracts host:port from URL
func extractHostPort(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "https://")
	// Remove path
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}
	return url
}

// PrintShutdown prints shutdown message
func PrintShutdown(appName string) {
	fmt.Printf("\n[INFO] %s shutting down...\n", appName)
}

// PrintError prints error message
func PrintError(err error) {
	fmt.Printf("[ERROR] %v\n", err)
}

// PrintSuccess prints success message
func PrintSuccess(msg string) {
	fmt.Printf("[INFO] %s\n", msg)
}

// PrintWarning prints warning message
func PrintWarning(msg string) {
	fmt.Printf("[WARN] %s\n", msg)
}

// PrintInfo prints info message
func PrintInfo(msg string) {
	fmt.Printf("[INFO] %s\n", msg)
}
