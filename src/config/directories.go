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

// SetCacheDirOverride sets a CLI override for the cache directory
func SetCacheDirOverride(dir string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["cache"] = dir
}

// SetBackupDirOverride sets a CLI override for the backup directory
func SetBackupDirOverride(dir string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["backup"] = dir
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

// IsPrivileged is implemented in ownership_unix.go and ownership_windows.go
// with platform-specific code using build tags

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

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/config/" + ProjectName
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

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/" + ProjectName
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

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/log/" + ProjectName
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/log/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/log/" + ProjectOrg + "/" + ProjectName)

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
		return os.ExpandEnv("$HOME/.local/log/" + ProjectOrg + "/" + ProjectName)

	default:
		return os.ExpandEnv("$HOME/.local/log/" + ProjectOrg + "/" + ProjectName)
	}
}

// GetCacheDir returns the OS-appropriate cache directory
func GetCacheDir() string {
	// Check CLI override first (--cache flag)
	if dir, ok := getOverride("cache"); ok && dir != "" {
		return dir
	}

	// Check environment variable
	if dir := os.Getenv("SEARCH_CACHE_DIR"); dir != "" {
		return dir
	}

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/" + ProjectName + "/cache"
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
	// Check CLI override first (--backup flag)
	if dir, ok := getOverride("backup"); ok && dir != "" {
		return dir
	}

	// Check environment variable
	if dir := os.Getenv("SEARCH_BACKUP_DIR"); dir != "" {
		return dir
	}
	if dir := os.Getenv("BACKUP_DIR"); dir != "" {
		return dir
	}

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/backups/" + ProjectName
	}

	isRoot := IsPrivileged()

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/mnt/Backups/" + ProjectOrg + "/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/share/Backups/" + ProjectOrg + "/" + ProjectName)

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
		return os.ExpandEnv("$HOME/.local/share/Backups/" + ProjectOrg + "/" + ProjectName)

	default:
		return os.ExpandEnv("$HOME/.local/share/Backups/" + ProjectOrg + "/" + ProjectName)
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
// Per AI.md PART 4: SSL is under config directory with letsencrypt/ and local/ subdirs
func GetSSLDir() string {
	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/config/" + ProjectName + "/ssl"
	}

	return filepath.Join(GetConfigDir(), "ssl")
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
// Per AI.md PART 4: Security DBs are under config/security/ (geoip, blocklists, cve, trivy)
func GetGeoIPDir() string {
	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/config/" + ProjectName + "/security/geoip"
	}

	return filepath.Join(GetConfigDir(), "security", "geoip")
}

// GetSecurityDir returns the OS-appropriate security directory
// Per AI.md PART 4: Contains geoip/, blocklists/, cve/, trivy/
func GetSecurityDir() string {
	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/config/" + ProjectName + "/security"
	}

	return filepath.Join(GetConfigDir(), "security")
}

// GetTorDir returns the OS-appropriate Tor data directory
func GetTorDir() string {
	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/" + ProjectName + "/tor"
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

// GetDirectoryPermissions returns the appropriate directory permissions
// Per AI.md PART 7: root: 0755, user: 0700
func GetDirectoryPermissions() os.FileMode {
	if IsPrivileged() {
		return 0755
	}
	return 0700
}

// GetSensitiveDirectoryPermissions returns permissions for sensitive directories
// Per AI.md PART 7: Tor dirs, SSL dirs always 0700
func GetSensitiveDirectoryPermissions() os.FileMode {
	return 0700
}

// GetSensitiveFilePermissions returns permissions for sensitive files
// Per AI.md PART 7: keys, config files 0600
func GetSensitiveFilePermissions() os.FileMode {
	return 0600
}

// EnsureDirectories creates all required directories if they don't exist
// Per AI.md PART 7: Server Startup Sequence - setup directories with proper permissions
func EnsureDirectories() error {
	// Get permission mode based on context (root vs user)
	dirPerm := GetDirectoryPermissions()
	sensitivePerm := GetSensitiveDirectoryPermissions()

	// Standard directories use context-based permissions
	standardDirs := []string{
		GetConfigDir(),
		GetDataDir(),
		GetLogDir(),
		GetCacheDir(),
		GetDatabaseDir(),
		GetGeoIPDir(),
		GetEmailTemplatesDir(),
		GetWebDataDir(),
		GetWellKnownDir(),
	}

	// Sensitive directories always use 0700
	sensitiveDirs := []string{
		GetTorDir(),
		GetTorKeysDir(),
		GetSSLDir(),
		filepath.Join(GetConfigDir(), "security"),
	}

	// PID file directory
	pidDir := filepath.Dir(GetPIDFile())

	// Create standard directories with appropriate permissions
	for _, dir := range standardDirs {
		if err := ensureDir(dir, dirPerm); err != nil {
			return err
		}
	}

	// Create sensitive directories with restricted permissions
	for _, dir := range sensitiveDirs {
		if err := ensureDir(dir, sensitivePerm); err != nil {
			return err
		}
	}

	// PID directory uses standard permissions
	if err := ensureDir(pidDir, dirPerm); err != nil {
		return err
	}

	return nil
}

// ensureDir creates a directory with specific permissions and sets ownership
// Per AI.md PART 7: Set ownership to current user/group
func ensureDir(path string, perm os.FileMode) error {
	// Create directory with proper permissions
	if err := os.MkdirAll(path, perm); err != nil {
		return err
	}

	// Fix permissions if directory already existed with different perms
	if err := os.Chmod(path, perm); err != nil {
		return err
	}

	// Set ownership to current user/group (Unix only)
	if err := setOwnership(path); err != nil {
		// Log but don't fail - may not have permission to chown
		// This is common when running in containers
		return nil
	}

	return nil
}

// EnsureSystemDirectories creates system directories and sets ownership
// Per AI.md PART 8: Step 8b-d - Create directories while running as root
// This is called BEFORE privilege dropping when running as root
func EnsureSystemDirectories(userName string) error {
	// System directories with 0755 permissions
	systemDirs := []string{
		"/etc/" + ProjectOrg + "/" + ProjectName,
		"/var/lib/" + ProjectOrg + "/" + ProjectName,
		"/var/lib/" + ProjectOrg + "/" + ProjectName + "/db",
		"/var/log/" + ProjectOrg + "/" + ProjectName,
		"/var/cache/" + ProjectOrg + "/" + ProjectName,
	}

	// Sensitive directories with 0700 permissions
	sensitiveDirs := []string{
		"/etc/" + ProjectOrg + "/" + ProjectName + "/security",
		"/etc/" + ProjectOrg + "/" + ProjectName + "/ssl",
		"/etc/" + ProjectOrg + "/" + ProjectName + "/tor",
	}

	// Create standard system directories
	for _, dir := range systemDirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
		// Set ownership to service user
		if err := chownRecursive(dir, userName); err != nil {
			return err
		}
	}

	// Create sensitive directories with restricted permissions
	for _, dir := range sensitiveDirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err := chownRecursive(dir, userName); err != nil {
			return err
		}
	}

	// Create PID directory
	pidDir := "/var/run/" + ProjectOrg
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		return err
	}
	if err := chownRecursive(pidDir, userName); err != nil {
		return err
	}

	return nil
}

// chownRecursive changes ownership of a directory and its contents
func chownRecursive(path, userName string) error {
	// This is implemented in ownership_unix.go and ownership_windows.go
	return chownPath(path, userName)
}

// EnsureSensitiveFile ensures a file has proper sensitive permissions (0600)
// Per AI.md PART 7: Tor files, key files, config files
func EnsureSensitiveFile(path string) error {
	if err := os.Chmod(path, GetSensitiveFilePermissions()); err != nil {
		return err
	}
	return setOwnership(path)
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
