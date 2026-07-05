package theme

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// goos is the operating system for detection (allows testing)
var goos = runtime.GOOS

// DetectSystemDark attempts to detect if the system prefers dark mode
func DetectSystemDark() bool {
	switch goos {
	case "darwin":
		return detectMacOSDark()
	case "linux":
		return detectLinuxDark()
	case "windows":
		return detectWindowsDark()
	default:
		// Default to dark for unknown systems
		return true
	}
}

// detectMacOSDark checks macOS dark mode preference
func detectMacOSDark() bool {
	cmd := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle")
	output, err := cmd.Output()
	if err != nil {
		// No AppleInterfaceStyle means light mode
		return false
	}
	return strings.TrimSpace(string(output)) == "Dark"
}

// detectLinuxDark checks Linux dark mode preference
func detectLinuxDark() bool {
	// Check GNOME/GTK dark mode
	cmd := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
	output, err := cmd.Output()
	if err == nil {
		scheme := strings.TrimSpace(string(output))
		if strings.Contains(scheme, "dark") {
			return true
		}
		if strings.Contains(scheme, "light") {
			return false
		}
	}

	// Check GTK theme name
	cmd = exec.Command("gsettings", "get", "org.gnome.desktop.interface", "gtk-theme")
	output, err = cmd.Output()
	if err == nil {
		theme := strings.ToLower(strings.TrimSpace(string(output)))
		if strings.Contains(theme, "dark") {
			return true
		}
	}

	// Check environment variable
	if gtkTheme := os.Getenv("GTK_THEME"); gtkTheme != "" {
		if strings.Contains(strings.ToLower(gtkTheme), "dark") {
			return true
		}
	}

	// Check KDE Plasma
	if os.Getenv("KDE_SESSION_VERSION") != "" {
		cmd = exec.Command("kreadconfig5", "--group", "General", "--key", "ColorScheme")
		output, err = cmd.Output()
		if err == nil {
			scheme := strings.ToLower(strings.TrimSpace(string(output)))
			if strings.Contains(scheme, "dark") {
				return true
			}
		}
	}

	// Default to dark
	return true
}

// detectWindowsDark checks Windows dark mode preference
func detectWindowsDark() bool {
	// Windows dark mode is stored in registry
	// HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize
	// AppsUseLightTheme = 0 means dark mode
	cmd := exec.Command("reg", "query",
		"HKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize",
		"/v", "AppsUseLightTheme")
	output, err := cmd.Output()
	if err != nil {
		// Default to dark if we can't read
		return true
	}

	// Output format: "    AppsUseLightTheme    REG_DWORD    0x0"
	// 0x0 = dark mode, 0x1 = light mode
	outputStr := string(output)
	if strings.Contains(outputStr, "0x0") {
		return true
	}
	if strings.Contains(outputStr, "0x1") {
		return false
	}

	return true
}

// GetSystemTheme returns the theme based on system preference
func GetSystemTheme() Theme {
	if DetectSystemDark() {
		return Dark
	}
	return Light
}
