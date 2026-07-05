package server

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/apimgr/search/src/common/display"
	"github.com/apimgr/search/src/config"
	"golang.org/x/term"
)

// color wraps text in ANSI color code if colors are enabled
// Per AI.md PART 8: NO_COLOR disables colors
func color(code, text string) string {
	if display.ColorEnabled() {
		return code + text + "\033[0m"
	}
	return text
}

// BannerInfo holds information for the startup banner
type BannerInfo struct {
	AppName    string
	Version    string
	Mode       string
	Debug      bool
	HTTPPort   int
	HTTPSPort  int
	HTTPAddr   string
	HTTPSAddr  string
	TorAddr    string
	I2PAddr    string
	ListenAddr string
	IsHTTPS    bool
}

// PrintBanner prints a responsive startup banner per AI.md spec
// Uses box drawing characters and emojis for visual appeal
// fmt.Fprintln(os.Stdout, ...) is used throughout this function: this is intentional
// terminal-only banner output, not application logging; log/slog must not be used here
// as it would prepend timestamps and log levels to visual border characters.
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

	// Blank line before banner (terminal-only output, not application logging)
	fmt.Fprintln(os.Stdout)
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
	// Blank line after banner (terminal-only output, not application logging)
	fmt.Fprintln(os.Stdout)
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
// Per AI.md PART 8: Respects NO_COLOR for border colors
// fmt.Fprintln(os.Stdout) is intentional terminal banner output, not application logging
func printTopBorder(width int) {
	border := "╭" + strings.Repeat("─", width-2) + "╮"
	fmt.Fprintln(os.Stdout, color("\033[36m", border))
}

// printBottomBorder prints the bottom border
// Per AI.md PART 8: Respects NO_COLOR for border colors
// fmt.Fprintln(os.Stdout) is intentional terminal banner output, not application logging
func printBottomBorder(width int) {
	border := "╰" + strings.Repeat("─", width-2) + "╯"
	fmt.Fprintln(os.Stdout, color("\033[36m", border))
}

// printSeparator prints a separator line
// Per AI.md PART 8: Respects NO_COLOR for border colors
// fmt.Fprintln(os.Stdout) is intentional terminal banner output, not application logging
func printSeparator(width int) {
	separator := "├" + strings.Repeat("─", width-2) + "┤"
	fmt.Fprintln(os.Stdout, color("\033[36m", separator))
}

// printLine prints a padded line within the banner
// Per AI.md PART 8: Respects NO_COLOR for border colors
// fmt.Fprint/Fprintln(os.Stdout) is intentional terminal banner output, not application logging
func printLine(content string, innerWidth int) {
	border := color("\033[36m", "│")
	fmt.Fprint(os.Stdout, border+"  ")
	// Calculate visible length (ignoring ANSI codes)
	visibleLen := visibleLength(content)
	padding := innerWidth - visibleLen
	if padding < 0 {
		padding = 0
	}
	fmt.Fprint(os.Stdout, content)
	fmt.Fprint(os.Stdout, strings.Repeat(" ", padding))
	fmt.Fprintln(os.Stdout, border)
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
// Per AI.md PART 8: Uses display.Emoji() for NO_COLOR fallback
func printHeaderLine(info *BannerInfo, innerWidth int) {
	// Use project name from config, with emoji
	appName := info.AppName
	if appName == "" {
		appName = "Search"
	}

	rocket := display.Emoji("🚀", "[*]")
	pkg := display.Emoji("📦", "v")
	header := fmt.Sprintf("%s %s · %s%s",
		color("\033[1;35m", rocket+" "+strings.ToUpper(appName)),
		"",
		color("\033[33m", pkg),
		info.Version)
	printLine(header, innerWidth)
}

// printModeLine prints the mode and debug status
// Per AI.md PART 8: Uses display.Emoji() for NO_COLOR fallback
func printModeLine(info *BannerInfo, innerWidth int) {
	var modeIcon, modeColor string
	mode := strings.ToLower(info.Mode)

	if mode == "production" {
		modeIcon = display.Emoji("🔒", "[PROD]")
		// Green
		modeColor = "\033[32m"
	} else {
		modeIcon = display.Emoji("🔧", "[DEV]")
		// Yellow
		modeColor = "\033[33m"
	}

	modeLine := fmt.Sprintf("%s %s", modeIcon, color(modeColor, "Running in mode: "+mode))

	if info.Debug {
		modeLine += " " + color("\033[35m", "[debugging]")
	}

	printLine(modeLine, innerWidth)
}

// printURLLines prints the access URLs
// Per AI.md PART 8: Uses display.Emoji() for NO_COLOR fallback
func printURLLines(info *BannerInfo, innerWidth int) {
	// Tor URL (if available)
	if info.TorAddr != "" {
		torIcon := display.Emoji("🧅", "[TOR]")
		torLine := fmt.Sprintf("%s %s  %s", torIcon, color("\033[35m", "Tor"), info.TorAddr)
		printLine(torLine, innerWidth)
	}

	// I2P URL (if available)
	if info.I2PAddr != "" {
		i2pIcon := display.Emoji("🔗", "[I2P]")
		i2pLine := fmt.Sprintf("%s %s  %s", i2pIcon, color("\033[36m", "I2P"), info.I2PAddr)
		printLine(i2pLine, innerWidth)
	}

	// HTTPS URL
	if info.HTTPSAddr != "" {
		httpsIcon := display.Emoji("🔐", "[SSL]")
		httpsLine := fmt.Sprintf("%s %s  %s", httpsIcon, color("\033[32m", "HTTPS"), info.HTTPSAddr)
		printLine(httpsLine, innerWidth)
	}

	// HTTP URL (only show if no HTTPS or in dual port mode)
	if info.HTTPAddr != "" && (info.HTTPSAddr == "" || info.HTTPSPort > 0) {
		httpIcon := display.Emoji("🌐", "[WEB]")
		httpLine := fmt.Sprintf("%s %s   %s", httpIcon, color("\033[34m", "HTTP"), info.HTTPAddr)
		printLine(httpLine, innerWidth)
	}
}

// printListenLine prints the listening address
// Per AI.md PART 8: Uses display.Emoji() for NO_COLOR fallback
func printListenLine(info *BannerInfo, innerWidth int) {
	proto := "http"
	if info.IsHTTPS {
		proto = "https"
	}
	listenIcon := display.Emoji("📡", "[LISTEN]")
	listenLine := fmt.Sprintf("%s %s", listenIcon, color("\033[36m", fmt.Sprintf("Listening on %s://%s", proto, info.ListenAddr)))
	printLine(listenLine, innerWidth)
}

// printTimestampLine prints the startup timestamp
// Per AI.md PART 8: Uses display.Emoji() for NO_COLOR fallback
func printTimestampLine(innerWidth int) {
	// Format: Wed Jan 15, 2025 at 09:00:00 EST
	timestamp := time.Now().Format("Mon Jan 02, 2006 at 15:04:05 MST")
	okIcon := display.Emoji("✅", "[OK]")
	timeLine := fmt.Sprintf("%s %s", okIcon, color("\033[32m", "Server started on "+timestamp))
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
