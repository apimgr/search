package server

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/apimgr/search/src/config"
	"golang.org/x/term"
)

// BannerInfo holds information for the startup banner
type BannerInfo struct {
	AppName     string
	Version     string
	Mode        string
	Debug       bool
	HTTPPort    int
	HTTPSPort   int
	HTTPAddr    string
	HTTPSAddr   string
	TorAddr     string
	I2PAddr     string
	ListenAddr  string
	IsHTTPS     bool
}

// PrintBanner prints a responsive startup banner per AI.md spec
// Uses box drawing characters and emojis for visual appeal
func PrintBanner(info *BannerInfo) {
	// Get terminal width for responsive layout
	width := getTerminalWidth()
	if width < 50 {
		width = 60
	}
	if width > 80 {
		width = 80
	}

	// Inner width (excluding borders)
	innerWidth := width - 4

	// Print banner
	fmt.Println()
	printTopBorder(width)
	printHeaderLine(info, innerWidth)
	printSeparator(width)
	printModeLine(info, innerWidth)
	printSeparator(width)
	printURLLines(info, innerWidth)
	printSeparator(width)
	printListenLine(info, innerWidth)
	printTimestampLine(innerWidth)
	printBottomBorder(width)
	fmt.Println()
}

// getTerminalWidth returns the terminal width or a default
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 40 {
		return 60
	}
	return width
}

// printTopBorder prints the top border
func printTopBorder(width int) {
	fmt.Print("\033[36m") // Cyan
	fmt.Print("╭")
	fmt.Print(strings.Repeat("─", width-2))
	fmt.Println("╮")
	fmt.Print("\033[0m") // Reset
}

// printBottomBorder prints the bottom border
func printBottomBorder(width int) {
	fmt.Print("\033[36m") // Cyan
	fmt.Print("╰")
	fmt.Print(strings.Repeat("─", width-2))
	fmt.Println("╯")
	fmt.Print("\033[0m") // Reset
}

// printSeparator prints a separator line
func printSeparator(width int) {
	fmt.Print("\033[36m") // Cyan
	fmt.Print("├")
	fmt.Print(strings.Repeat("─", width-2))
	fmt.Println("┤")
	fmt.Print("\033[0m") // Reset
}

// printLine prints a padded line within the banner
func printLine(content string, innerWidth int) {
	fmt.Print("\033[36m│\033[0m  ") // Cyan border
	// Calculate visible length (ignoring ANSI codes)
	visibleLen := visibleLength(content)
	padding := innerWidth - visibleLen
	if padding < 0 {
		padding = 0
	}
	fmt.Print(content)
	fmt.Print(strings.Repeat(" ", padding))
	fmt.Println("\033[36m│\033[0m") // Cyan border
}

// visibleLength calculates the visible length of a string (ignoring ANSI escape codes)
func visibleLength(s string) int {
	length := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		length++
	}
	return length
}

// printHeaderLine prints the app name and version
func printHeaderLine(info *BannerInfo, innerWidth int) {
	// Use project name from config, with emoji
	appName := info.AppName
	if appName == "" {
		appName = "Search"
	}

	header := fmt.Sprintf("\033[1;35m🚀 %s\033[0m · \033[33m📦 v%s\033[0m",
		strings.ToUpper(appName), info.Version)
	printLine(header, innerWidth)
}

// printModeLine prints the mode and debug status
func printModeLine(info *BannerInfo, innerWidth int) {
	var modeIcon, modeColor string
	mode := strings.ToLower(info.Mode)

	if mode == "production" {
		modeIcon = "🔒"
		modeColor = "\033[32m" // Green
	} else {
		modeIcon = "🔧"
		modeColor = "\033[33m" // Yellow
	}

	modeLine := fmt.Sprintf("%s %sRunning in mode: %s\033[0m", modeIcon, modeColor, mode)

	if info.Debug {
		modeLine += " \033[35m[debugging]\033[0m"
	}

	printLine(modeLine, innerWidth)
}

// printURLLines prints the access URLs
func printURLLines(info *BannerInfo, innerWidth int) {
	// Tor URL (if available)
	if info.TorAddr != "" {
		torLine := fmt.Sprintf("🧅 \033[35mTor\033[0m    %s", info.TorAddr)
		printLine(torLine, innerWidth)
	}

	// I2P URL (if available)
	if info.I2PAddr != "" {
		i2pLine := fmt.Sprintf("🔗 \033[36mI2P\033[0m    %s", info.I2PAddr)
		printLine(i2pLine, innerWidth)
	}

	// HTTPS URL
	if info.HTTPSAddr != "" {
		httpsLine := fmt.Sprintf("🔐 \033[32mHTTPS\033[0m  %s", info.HTTPSAddr)
		printLine(httpsLine, innerWidth)
	}

	// HTTP URL (only show if no HTTPS or in dual port mode)
	if info.HTTPAddr != "" && (info.HTTPSAddr == "" || info.HTTPSPort > 0) {
		httpLine := fmt.Sprintf("🌐 \033[34mHTTP\033[0m   %s", info.HTTPAddr)
		printLine(httpLine, innerWidth)
	}
}

// printListenLine prints the listening address
func printListenLine(info *BannerInfo, innerWidth int) {
	proto := "http"
	if info.IsHTTPS {
		proto = "https"
	}
	listenLine := fmt.Sprintf("📡 \033[36mListening on %s://%s\033[0m", proto, info.ListenAddr)
	printLine(listenLine, innerWidth)
}

// printTimestampLine prints the startup timestamp
func printTimestampLine(innerWidth int) {
	// Format: Wed Jan 15, 2025 at 09:00:00 EST
	timestamp := time.Now().Format("Mon Jan 02, 2006 at 15:04:05 MST")
	timeLine := fmt.Sprintf("✅ \033[32mServer started on %s\033[0m", timestamp)
	printLine(timeLine, innerWidth)
}

// BuildBannerInfo creates banner info from server configuration
func BuildBannerInfo(cfg *config.Config, torAddr string) *BannerInfo {
	info := &BannerInfo{
		AppName: "Search",
		Version: config.Version,
		Mode:    cfg.Server.Mode,
		Debug:   cfg.IsDebug(),
	}

	// Determine ports and addresses
	port := cfg.Server.Port
	httpsPort := cfg.Server.HTTPSPort

	// Get display hostname
	host := cfg.Server.Address
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = getDisplayHost(cfg)
	}

	// Single port mode
	if httpsPort == 0 {
		if cfg.Server.SSL.Enabled || port == 443 {
			info.HTTPSAddr = formatBannerURL(host, port, true)
			info.HTTPSPort = port
			info.IsHTTPS = true
		} else {
			info.HTTPAddr = formatBannerURL(host, port, false)
			info.HTTPPort = port
		}
		info.ListenAddr = fmt.Sprintf("%s:%d", cfg.Server.Address, port)
	} else {
		// Dual port mode
		info.HTTPAddr = formatBannerURL(host, port, false)
		info.HTTPSAddr = formatBannerURL(host, httpsPort, true)
		info.HTTPPort = port
		info.HTTPSPort = httpsPort
		info.IsHTTPS = true
		info.ListenAddr = fmt.Sprintf("%s:%d,%d", cfg.Server.Address, port, httpsPort)
	}

	// Tor address
	if torAddr != "" {
		// Tor uses HTTP by default unless HTTPS-only mode
		if port == 443 && httpsPort == 0 {
			info.TorAddr = "https://" + torAddr
		} else {
			info.TorAddr = "http://" + torAddr
		}
	}

	return info
}

// getDisplayHost returns the best hostname for display
func getDisplayHost(cfg *config.Config) string {
	// Try to get FQDN from hostname
	hostname, err := os.Hostname()
	if err == nil && hostname != "" && hostname != "localhost" {
		return hostname
	}

	// Default to localhost
	return "localhost"
}

// formatBannerURL formats a URL for the banner
func formatBannerURL(host string, port int, isHTTPS bool) string {
	proto := "http"
	if isHTTPS || port == 443 {
		proto = "https"
	}

	// Strip default ports
	if port == 80 || port == 443 {
		return fmt.Sprintf("%s://%s", proto, host)
	}

	return fmt.Sprintf("%s://%s:%d", proto, host, port)
}
