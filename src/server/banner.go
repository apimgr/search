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

// PrintBanner prints a responsive startup banner per AI.md PART 15 spec.
// Adapts to terminal width: ≥80 full box drawing, 60-79 compact, 40-59 minimal,
// <40 micro single-line, NO_COLOR/TERM=dumb plain text (no emojis or box drawing).
// fmt.Fprintln(os.Stdout, ...) is used throughout this function: this is intentional
// terminal-only banner output, not application logging; log/slog must not be used here
// as it would prepend timestamps and log levels to visual border characters.
func PrintBanner(info *BannerInfo) {
	width := getTerminalWidth()

	// Plain mode: no emojis (NO_COLOR, TERM=dumb)
	if !display.ColorEnabled() || os.Getenv("TERM") == "dumb" {
		printBannerPlain(info)
		return
	}

	switch {
	case width >= 80:
		printBannerFull(info, width)
	case width >= 60:
		printBannerCompact(info)
	case width >= 40:
		printBannerMinimal(info)
	default:
		printBannerMicro(info)
	}
}

// getTerminalWidth returns the raw terminal width (unclamped).
// Returns 80 as default when the terminal size cannot be determined.
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 1 {
		return 80
	}
	return width
}

// printBannerFull renders the full box-drawing banner for terminals ≥80 columns.
// Per AI.md PART 15: Full branded banner with icons and URLs.
func printBannerFull(info *BannerInfo, width int) {
	// Cap width to avoid overly wide banners on very large terminals
	if width > 120 {
		width = 120
	}
	innerWidth := width - 4

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
	fmt.Fprintln(os.Stdout)
}

// printBannerCompact renders the compact banner for 60-79 column terminals.
// Per AI.md PART 15: Icons + text only, no box drawing.
func printBannerCompact(info *BannerInfo) {
	appName := info.AppName
	if appName == "" {
		appName = "Search"
	}

	rocket := display.Emoji("🚀", "*")
	pkg := display.Emoji("📦", "v")
	fmt.Fprintf(os.Stdout, "%s %s %s%s\n",
		color("\033[1;35m", rocket+" "+strings.ToUpper(appName)),
		"",
		color("\033[33m", pkg),
		info.Version)

	modeIcon, modeColor := bannerModeIcon(info.Mode)
	fmt.Fprintf(os.Stdout, "%s %s\n",
		modeIcon,
		color(modeColor, "Mode: "+strings.ToLower(info.Mode)))

	if info.HTTPSAddr != "" {
		lock := display.Emoji("🔐", "[SSL]")
		fmt.Fprintf(os.Stdout, "%s %s\n", lock, color("\033[32m", info.HTTPSAddr))
	}
	if info.HTTPAddr != "" && (info.HTTPSAddr == "" || info.HTTPSPort > 0) {
		globe := display.Emoji("🌐", "[WEB]")
		fmt.Fprintf(os.Stdout, "%s %s\n", globe, color("\033[34m", info.HTTPAddr))
	}
	if info.TorAddr != "" {
		onion := display.Emoji("🧅", "[TOR]")
		fmt.Fprintf(os.Stdout, "%s %s\n", onion, color("\033[35m", info.TorAddr))
	}

	proto := "http"
	if info.IsHTTPS {
		proto = "https"
	}
	listen := display.Emoji("📡", "[LISTEN]")
	fmt.Fprintf(os.Stdout, "%s %s\n",
		listen,
		color("\033[36m", fmt.Sprintf("Listening: %s://%s", proto, info.ListenAddr)))

	ok := display.Emoji("✅", "[OK]")
	ts := time.Now().Format("Mon Jan 02, 2006 at 15:04:05 MST")
	fmt.Fprintf(os.Stdout, "%s %s\n", ok, color("\033[32m", "Started: "+ts))
	fmt.Fprintln(os.Stdout)
}

// printBannerMinimal renders the minimal banner for 40-59 column terminals.
// Per AI.md PART 15: Abbreviated, no icons.
func printBannerMinimal(info *BannerInfo) {
	appName := info.AppName
	if appName == "" {
		appName = "Search"
	}

	fmt.Fprintf(os.Stdout, "%s %s\n", appName, info.Version)
	fmt.Fprintf(os.Stdout, "%s\n", strings.ToLower(info.Mode))

	addr := primaryURLHost(info)
	if addr != "" {
		fmt.Fprintf(os.Stdout, "%s\n", addr)
	}
	fmt.Fprintln(os.Stdout)
}

// printBannerMicro renders the micro banner for terminals <40 columns.
// Per AI.md PART 15: Single line only.
func printBannerMicro(info *BannerInfo) {
	appName := info.AppName
	if appName == "" {
		appName = "Search"
	}

	addr := primaryURLHost(info)
	if addr != "" {
		fmt.Fprintf(os.Stdout, "%s %s\n", appName, addr)
	} else {
		fmt.Fprintf(os.Stdout, "%s\n", appName)
	}
}

// printBannerPlain renders a plain text banner with no emojis or ANSI codes.
// Per AI.md PART 15: NO_COLOR / TERM=dumb output.
func printBannerPlain(info *BannerInfo) {
	appName := info.AppName
	if appName == "" {
		appName = "Search"
	}

	fmt.Fprintf(os.Stdout, "%s v%s\n", appName, info.Version)
	fmt.Fprintf(os.Stdout, "Mode: %s\n", strings.ToLower(info.Mode))

	if info.HTTPSAddr != "" {
		fmt.Fprintf(os.Stdout, "URL: %s\n", info.HTTPSAddr)
	}
	if info.HTTPAddr != "" && (info.HTTPSAddr == "" || info.HTTPSPort > 0) {
		fmt.Fprintf(os.Stdout, "URL: %s\n", info.HTTPAddr)
	}
	if info.TorAddr != "" {
		fmt.Fprintf(os.Stdout, "Tor: %s\n", info.TorAddr)
	}

	proto := "http"
	if info.IsHTTPS {
		proto = "https"
	}
	fmt.Fprintf(os.Stdout, "Listening: %s://%s\n", proto, info.ListenAddr)
	fmt.Fprintf(os.Stdout, "Started: %s\n", time.Now().Format("Mon Jan 02, 2006 at 15:04:05 MST"))
	fmt.Fprintln(os.Stdout)
}

// bannerModeIcon returns the emoji/icon and ANSI color for the current mode.
func bannerModeIcon(mode string) (string, string) {
	if strings.ToLower(mode) == "production" {
		return display.Emoji("🔒", "[PROD]"), "\033[32m"
	}
	return display.Emoji("🔧", "[DEV]"), "\033[33m"
}

// primaryURLHost returns the primary display URL (HTTPS preferred over HTTP).
func primaryURLHost(info *BannerInfo) string {
	if info.HTTPSAddr != "" {
		return info.HTTPSAddr
	}
	if info.HTTPAddr != "" {
		return info.HTTPAddr
	}
	return ""
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
	modeIcon, modeColor := bannerModeIcon(info.Mode)
	mode := strings.ToLower(info.Mode)

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
