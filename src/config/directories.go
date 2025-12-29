package config

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const (
	// ProjectOrg is the organization name
	ProjectOrg = "apimgr"
	// ProjectName is the project name
	ProjectName = "search"
)

// CLI directory overrides (per AI.md PART 17)
var (
	cliOverrides   = make(map[string]string)
	cliOverrideMu  sync.RWMutex
)

// SetConfigDirOverride sets a CLI override for the config directory
func SetConfigDirOverride(dir string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["config"] = dir
}

// SetDataDirOverride sets a CLI override for the data directory
func SetDataDirOverride(dir string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["data"] = dir
}

// SetLogDirOverride sets a CLI override for the log directory
func SetLogDirOverride(dir string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["logs"] = dir
}

// SetPIDFileOverride sets a CLI override for the PID file path
func SetPIDFileOverride(path string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["pid"] = path
}

// getOverride returns a CLI override if set
func getOverride(key string) (string, bool) {
	cliOverrideMu.RLock()
	defer cliOverrideMu.RUnlock()
	val, ok := cliOverrides[key]
	return val, ok
}

// GetOS returns the current operating system
func GetOS() string {
	return runtime.GOOS
}

// GetArch returns the current architecture
func GetArch() string {
	return runtime.GOARCH
}

// IsPrivileged returns true if running with elevated privileges
func IsPrivileged() bool {
	switch GetOS() {
	case "windows":
		// On Windows, check if running as Administrator
		// Simplified check - in production would use Windows API
		return os.Getenv("PROCESSOR_ARCHITECTURE") != "" && os.Geteuid() == 0
	default:
		// On Unix-like systems, check EUID
		return os.Geteuid() == 0
	}
}

// IsRunningInContainer returns true if running inside a container
func IsRunningInContainer() bool {
	// Check for Docker/container indicators
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}

	// Check if init process is tini (common container init)
	if data, err := os.ReadFile("/proc/1/comm"); err == nil {
		comm := string(data)
		if comm == "tini\n" || comm == "dumb-init\n" {
			return true
		}
	}

	// Check cgroup for container indicators
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if len(content) > 0 && (contains(content, "docker") || contains(content, "kubepods") || contains(content, "lxc")) {
			return true
		}
	}

	return false
}

// contains checks if s contains substr (simple helper to avoid strings import overhead)
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// GetConfigDir returns the OS-appropriate configuration directory
func GetConfigDir() string {
	// Check CLI override first (--config flag)
	if dir, ok := getOverride("config"); ok && dir != "" {
		return dir
	}

	// Check environment variable
	if dir := os.Getenv("SEARCH_CONFIG_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		return dir
	}

	// Container paths
	if IsRunningInContainer() {
		return "/config"
	}

	// Check if running as root/admin
	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/etc/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.config/" + ProjectOrg + "/" + ProjectName)

	case "darwin":
		if isRoot {
			return "/Library/Application Support/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/Library/Application Support/" + ProjectOrg + "/" + ProjectName)

	case "windows":
		if isRoot {
			return os.ExpandEnv("%ProgramData%\\" + ProjectOrg + "\\" + ProjectName)
		}
		return os.ExpandEnv("%AppData%\\" + ProjectOrg + "\\" + ProjectName)

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/usr/local/etc/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.config/" + ProjectOrg + "/" + ProjectName)

	default:
		return os.ExpandEnv("$HOME/.config/" + ProjectOrg + "/" + ProjectName)
	}
}

// GetDataDir returns the OS-appropriate data directory
func GetDataDir() string {
	// Check CLI override first (--data flag)
	if dir, ok := getOverride("data"); ok && dir != "" {
		return dir
	}

	// Check environment variable
	if dir := os.Getenv("SEARCH_DATA_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}

	// Container paths
	if IsRunningInContainer() {
		return "/data"
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/lib/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/share/" + ProjectOrg + "/" + ProjectName)

	case "darwin":
		if isRoot {
			return "/Library/Application Support/" + ProjectOrg + "/" + ProjectName + "/data"
		}
		return os.ExpandEnv("$HOME/Library/Application Support/" + ProjectOrg + "/" + ProjectName)

	case "windows":
		if isRoot {
			return os.ExpandEnv("%ProgramData%\\" + ProjectOrg + "\\" + ProjectName + "\\data")
		}
		return os.ExpandEnv("%LocalAppData%\\" + ProjectOrg + "\\" + ProjectName)

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/db/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/share/" + ProjectOrg + "/" + ProjectName)

	default:
		return os.ExpandEnv("$HOME/.local/share/" + ProjectOrg + "/" + ProjectName)
	}
}

// GetLogDir returns the OS-appropriate log directory
func GetLogDir() string {
	// Check CLI override first (--log flag)
	if dir, ok := getOverride("logs"); ok && dir != "" {
		return dir
	}

	// Check environment variable
	if dir := os.Getenv("SEARCH_LOG_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("LOG_DIR"); dir != "" {
		return dir
	}

	// Container paths
	if IsRunningInContainer() {
		return "/data/logs/" + ProjectName
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/log/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/share/" + ProjectOrg + "/" + ProjectName + "/logs")

	case "darwin":
		if isRoot {
			return "/Library/Logs/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/Library/Logs/" + ProjectOrg + "/" + ProjectName)

	case "windows":
		if isRoot {
			return os.ExpandEnv("%ProgramData%\\" + ProjectOrg + "\\" + ProjectName + "\\logs")
		}
		return os.ExpandEnv("%LocalAppData%\\" + ProjectOrg + "\\" + ProjectName + "\\logs")

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/log/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/share/" + ProjectOrg + "/" + ProjectName + "/logs")

	default:
		return os.ExpandEnv("$HOME/.local/share/" + ProjectOrg + "/" + ProjectName + "/logs")
	}
}

// GetCacheDir returns the OS-appropriate cache directory
func GetCacheDir() string {
	// Container paths
	if IsRunningInContainer() {
		return "/data/cache"
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/cache/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.cache/" + ProjectOrg + "/" + ProjectName)

	case "darwin":
		return os.ExpandEnv("$HOME/Library/Caches/" + ProjectOrg + "/" + ProjectName)

	case "windows":
		return os.ExpandEnv("%LocalAppData%\\" + ProjectOrg + "\\" + ProjectName + "\\cache")

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/cache/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.cache/" + ProjectOrg + "/" + ProjectName)

	default:
		return os.ExpandEnv("$HOME/.cache/" + ProjectOrg + "/" + ProjectName)
	}
}

// GetBackupDir returns the OS-appropriate backup directory
func GetBackupDir() string {
	// Check environment variable first
	if dir := os.Getenv("SEARCH_BACKUP_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("BACKUP_DIR"); dir != "" {
		return dir
	}

	// Container paths
	if IsRunningInContainer() {
		return "/data/backups"
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/mnt/Backups/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/backups/" + ProjectOrg + "/" + ProjectName)

	case "darwin":
		if isRoot {
			return "/Library/Backups/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/Library/Backups/" + ProjectOrg + "/" + ProjectName)

	case "windows":
		if isRoot {
			return os.ExpandEnv("%ProgramData%\\Backups\\" + ProjectOrg + "\\" + ProjectName)
		}
		return os.ExpandEnv("%LocalAppData%\\Backups\\" + ProjectOrg + "\\" + ProjectName)

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/backups/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/backups/" + ProjectOrg + "/" + ProjectName)

	default:
		return os.ExpandEnv("$HOME/.local/backups/" + ProjectOrg + "/" + ProjectName)
	}
}

// GetPIDFile returns the OS-appropriate PID file path
func GetPIDFile() string {
	// Check CLI override first (--pid flag)
	if path, ok := getOverride("pid"); ok && path != "" {
		return path
	}

	// Check environment variable
	if path := os.Getenv("SEARCH_PID_FILE"); path != "" {
		return path
	}
	if path := os.Getenv("PID_FILE"); path != "" {
		return path
	}

	// Container paths
	if IsRunningInContainer() {
		return "/data/" + ProjectName + ".pid"
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/run/" + ProjectOrg + "/" + ProjectName + ".pid"
		}
		return filepath.Join(GetDataDir(), ProjectName+".pid")

	case "darwin":
		if isRoot {
			return "/var/run/" + ProjectOrg + "/" + ProjectName + ".pid"
		}
		return filepath.Join(GetDataDir(), ProjectName+".pid")

	case "windows":
		return filepath.Join(GetDataDir(), ProjectName+".pid")

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/run/" + ProjectOrg + "/" + ProjectName + ".pid"
		}
		return filepath.Join(GetDataDir(), ProjectName+".pid")

	default:
		return filepath.Join(GetDataDir(), ProjectName+".pid")
	}
}

// GetSSLDir returns the OS-appropriate SSL certificates directory
func GetSSLDir() string {
	// Container paths
	if IsRunningInContainer() {
		return "/config/ssl/certs"
	}

	return filepath.Join(GetConfigDir(), "ssl", "certs")
}

// GetDatabaseDir returns the OS-appropriate database directory
func GetDatabaseDir() string {
	// Check environment variable first
	if dir := os.Getenv("SEARCH_DATABASE_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("DATABASE_DIR"); dir != "" {
		return dir
	}

	// Container paths
	if IsRunningInContainer() {
		return "/data/db"
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/lib/" + ProjectOrg + "/" + ProjectName + "/db"
		}
		return filepath.Join(GetDataDir(), "db")

	case "darwin":
		return filepath.Join(GetDataDir(), "db")

	case "windows":
		return filepath.Join(GetDataDir(), "db")

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/db/" + ProjectOrg + "/" + ProjectName + "/db"
		}
		return filepath.Join(GetDataDir(), "db")

	default:
		return filepath.Join(GetDataDir(), "db")
	}
}

// GetGeoIPDir returns the OS-appropriate GeoIP database directory
func GetGeoIPDir() string {
	// Container paths
	if IsRunningInContainer() {
		return "/data/geoip"
	}

	return filepath.Join(GetDataDir(), "geoip")
}

// GetTorDir returns the OS-appropriate Tor data directory
func GetTorDir() string {
	// Container paths
	if IsRunningInContainer() {
		return "/data/tor"
	}

	return filepath.Join(GetDataDir(), "tor")
}

// GetTorKeysDir returns the directory for Tor hidden service keys
func GetTorKeysDir() string {
	return filepath.Join(GetTorDir(), "site")
}

// GetTemplatesDir returns the email templates directory (customizable)
func GetTemplatesDir() string {
	return filepath.Join(GetConfigDir(), "templates")
}

// GetEmailTemplatesDir returns the email templates directory
func GetEmailTemplatesDir() string {
	return filepath.Join(GetTemplatesDir(), "email")
}

// GetWebDataDir returns the web data directory for custom assets
func GetWebDataDir() string {
	return filepath.Join(GetDataDir(), "web")
}

// GetWellKnownDir returns the .well-known directory for custom files
func GetWellKnownDir() string {
	return filepath.Join(GetWebDataDir(), ".well-known")
}

// EnsureDirectories creates all required directories if they don't exist
func EnsureDirectories() error {
	dirs := []string{
		GetConfigDir(),
		GetDataDir(),
		GetLogDir(),
		GetCacheDir(),
		GetDatabaseDir(),
		GetGeoIPDir(),
		GetTorDir(),
		GetSSLDir(),
		GetEmailTemplatesDir(),
		GetWebDataDir(),
		GetWellKnownDir(),
	}

	// PID file directory (may need special handling)
	pidDir := filepath.Dir(GetPIDFile())
	dirs = append(dirs, pidDir)

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}

// GetServiceFile returns the OS-appropriate service file path
func GetServiceFile() string {
	switch GetOS() {
	case "linux":
		return "/etc/systemd/system/" + ProjectName + ".service"

	case "darwin":
		if IsPrivileged() {
			return "/Library/LaunchDaemons/com." + ProjectOrg + "." + ProjectName + ".plist"
		}
		return os.ExpandEnv("$HOME/Library/LaunchAgents/com." + ProjectOrg + "." + ProjectName + ".plist")

	// Windows uses Service Manager, not a file
	case "windows":
		return ""

	case "freebsd", "openbsd", "netbsd":
		return "/usr/local/etc/rc.d/" + ProjectName

	default:
		return ""
	}
}

// GetBinaryPath returns the expected installation path for the binary
func GetBinaryPath() string {
	switch GetOS() {
	case "linux":
		if IsPrivileged() {
			return "/usr/local/bin/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/bin/" + ProjectName)

	case "darwin":
		if IsPrivileged() {
			return "/usr/local/bin/" + ProjectName
		}
		return os.ExpandEnv("$HOME/bin/" + ProjectName)

	case "windows":
		if IsPrivileged() {
			return os.ExpandEnv("%ProgramFiles%\\" + ProjectOrg + "\\" + ProjectName + "\\" + ProjectName + ".exe")
		}
		return os.ExpandEnv("%LocalAppData%\\" + ProjectOrg + "\\" + ProjectName + "\\" + ProjectName + ".exe")

	case "freebsd", "openbsd", "netbsd":
		return "/usr/local/bin/" + ProjectName

	default:
		return "/usr/local/bin/" + ProjectName
	}
}
