package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

const (
	// ProjectOrg is the organization name
	ProjectOrg = "apimgr"
	// ProjectName is the project name
	ProjectName = "search"
	// InternalOrg is the frozen on-disk organization identity (per AI.md
	// PART 0/23: set once at first install from ProjectOrg, then immutable
	// for the project's lifetime even if ProjectOrg is renamed later).
	InternalOrg = "apimgr"
	// InternalName is the frozen on-disk/service-account identity (per
	// AI.md PART 0/23: set once at first install from ProjectName, then
	// immutable for the project's lifetime). Used for the privilege-drop
	// system user/group and backup directory paths so a project rename
	// never orphans them.
	InternalName = "search"
)

// SafeDirPath validates a filesystem path while preserving absolute paths.
func SafeDirPath(input string) (string, error) {
	if input == "" {
		return "", errors.New("empty path")
	}
	for _, r := range input {
		if unicode.IsControl(r) {
			return "", errors.New("invalid control character")
		}
	}
	for _, segment := range strings.Split(filepath.ToSlash(input), "/") {
		if segment == ".." {
			return "", errors.New("path traversal")
		}
	}
	return filepath.Clean(input), nil
}

// CLI directory overrides (per AI.md PART 17)
var (
	cliOverrides    = make(map[string]string)
	cliOverrideMu   sync.RWMutex
	startedElevated = IsPrivileged()
	pidFileEnabled  = true
)

// StartedElevated reports whether this process began with elevated privileges.
// This value is captured before any service privilege drop and remains stable
// for the lifetime of the process.
func StartedElevated() bool {
	return startedElevated
}

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

// SetPIDFileEnabled controls whether startup manages the server PID file.
func SetPIDFileEnabled(enabled bool) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	pidFileEnabled = enabled
}

// PIDFileEnabled reports whether this process should manage a PID file.
func PIDFileEnabled() bool {
	if IsRunningInContainer() {
		return false
	}
	cliOverrideMu.RLock()
	defer cliOverrideMu.RUnlock()
	return pidFileEnabled
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

// SetDatabaseDirOverride sets a CLI override for the database directory
func SetDatabaseDirOverride(dir string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["database"] = dir
}

// SetColorMode sets the color output mode
// Per AI.md PART 8: --color {always|never|auto} flag
// Priority: CLI flag > Config > NO_COLOR env > Auto-detect
func SetColorMode(mode string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["color"] = mode
}

// SetBaseURLOverride sets a CLI override for the base URL path prefix
// Per AI.md PART 6: --baseurl flag for reverse proxy path prefix
func SetBaseURLOverride(url string) {
	cliOverrideMu.Lock()
	defer cliOverrideMu.Unlock()
	cliOverrides["base_url"] = url
}

// GetColorMode returns the color output mode
// Per AI.md PART 8: Priority: CLI flag > Config > NO_COLOR env > Auto-detect
func GetColorMode() string {
	// Check CLI override first (--color flag)
	if mode, ok := getOverride("color"); ok && mode != "" {
		return mode
	}

	// Check NO_COLOR environment variable (standard)
	if os.Getenv("NO_COLOR") != "" {
		return "never"
	}

	// Auto-detect based on terminal
	return "auto"
}

// GetBaseURL returns the base URL path prefix
// Per AI.md PART 6: Configurable base URL for reverse proxy, defaults to "/"
func GetBaseURL() string {
	// Check CLI override first (--baseurl flag)
	if url, ok := getOverride("base_url"); ok && url != "" {
		return url
	}

	// Default to "/"
	return "/"
}

// IsColorEnabled returns whether color output should be enabled
// Per AI.md PART 8: Honor NO_COLOR standard
func IsColorEnabled() bool {
	mode := GetColorMode()
	switch mode {
	case "always":
		return true
	case "never":
		return false
	// "auto"
	default:
		// Check if stdout is a terminal
		return isTerminal()
	}
}

// isTerminal checks if stdout is a terminal (platform-specific)
func isTerminal() bool {
	// Check for common CI/non-terminal indicators
	if os.Getenv("CI") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	// Check TERM is set (common terminal indicator)
	return os.Getenv("TERM") != ""
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

// containerDetectionOverride allows tests to override container detection without filesystem manipulation.
// Nil means use actual detection; non-nil overrides the result directly.
var containerDetectionOverride *bool

// IsRunningInContainer returns true if running inside a container
func IsRunningInContainer() bool {
	if containerDetectionOverride != nil {
		return *containerDetectionOverride
	}
	if os.Getenv("INVOCATION_ID") != "" || os.Getenv("RC_SVCNAME") != "" {
		return false
	}
	if comm, err := os.ReadFile("/proc/1/comm"); err == nil && strings.TrimSpace(string(comm)) == "systemd" {
		return false
	}
	if os.Getppid() == 1 {
		if _, err := os.Stat("/run/openrc"); err == nil {
			return false
		}
	}

	for _, indicator := range []string{"/.dockerenv", "/run/.containerenv", "/dev/lxc"} {
		if _, err := os.Stat(indicator); err == nil {
			return true
		}
	}
	if os.Getenv("container") != "" || os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return true
	}

	if parent, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", os.Getppid())); err == nil {
		switch strings.TrimSpace(string(parent)) {
		case "tini", "dumb-init", "s6-svscan", "runsv", "runsvdir", "catatonit", ProjectName:
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
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		return dir
	}

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/config/" + ProjectName
	}

	// Keep the path mode stable when the service later drops privileges.
	isRoot := startedElevated

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/etc/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.config/" + InternalOrg + "/" + InternalName)

	case "darwin":
		if isRoot {
			return "/Library/Application Support/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/Library/Application Support/" + InternalOrg + "/" + InternalName)

	case "windows":
		if isRoot {
			return filepath.Join(os.Getenv("ProgramData"), InternalOrg, InternalName)
		}
		return filepath.Join(os.Getenv("AppData"), InternalOrg, InternalName)

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/usr/local/etc/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.config/" + InternalOrg + "/" + InternalName)

	default:
		return os.ExpandEnv("$HOME/.config/" + InternalOrg + "/" + InternalName)
	}
}

// GetDataDir returns the OS-appropriate data directory
func GetDataDir() string {
	// Check CLI override first (--data flag)
	if dir, ok := getOverride("data"); ok && dir != "" {
		return dir
	}

	// Check environment variable
	if dir := os.Getenv("DATA_DIR"); dir != "" {
		return dir
	}

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/" + ProjectName
	}

	isRoot := startedElevated

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/lib/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.local/share/" + InternalOrg + "/" + InternalName)

	case "darwin":
		if isRoot {
			return "/Library/Application Support/" + InternalOrg + "/" + InternalName + "/data"
		}
		return os.ExpandEnv("$HOME/Library/Application Support/" + InternalOrg + "/" + InternalName)

	case "windows":
		if isRoot {
			return filepath.Join(os.Getenv("ProgramData"), InternalOrg, InternalName, "data")
		}
		return filepath.Join(os.Getenv("LocalAppData"), InternalOrg, InternalName)

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/db/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.local/share/" + InternalOrg + "/" + InternalName)

	default:
		return os.ExpandEnv("$HOME/.local/share/" + InternalOrg + "/" + InternalName)
	}
}

// GetLogDir returns the OS-appropriate log directory
func GetLogDir() string {
	// Check CLI override first (--log flag)
	if dir, ok := getOverride("logs"); ok && dir != "" {
		return dir
	}

	// Check environment variable
	if dir := os.Getenv("LOG_DIR"); dir != "" {
		return dir
	}

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/log/" + ProjectName
	}

	isRoot := startedElevated

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/log/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.local/log/" + InternalOrg + "/" + InternalName)

	case "darwin":
		if isRoot {
			return "/Library/Logs/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/Library/Logs/" + InternalOrg + "/" + InternalName)

	case "windows":
		if isRoot {
			return filepath.Join(os.Getenv("ProgramData"), InternalOrg, InternalName, "logs")
		}
		return filepath.Join(os.Getenv("LocalAppData"), InternalOrg, InternalName, "logs")

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/log/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.local/log/" + InternalOrg + "/" + InternalName)

	default:
		return os.ExpandEnv("$HOME/.local/log/" + InternalOrg + "/" + InternalName)
	}
}

// GetCacheDir returns the OS-appropriate cache directory
func GetCacheDir() string {
	// Check CLI override first (--cache flag)
	if dir, ok := getOverride("cache"); ok && dir != "" {
		return dir
	}
	if dir := os.Getenv("CACHE_DIR"); dir != "" {
		return dir
	}

	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/" + ProjectName + "/cache"
	}

	isRoot := startedElevated

	switch GetOS() {
	case "linux":
		if isRoot {
			return "/var/cache/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.cache/" + InternalOrg + "/" + InternalName)

	case "darwin":
		if isRoot {
			return "/Library/Caches/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/Library/Caches/" + InternalOrg + "/" + InternalName)

	case "windows":
		if isRoot {
			return filepath.Join(os.Getenv("ProgramData"), InternalOrg, InternalName, "cache")
		}
		return filepath.Join(os.Getenv("LocalAppData"), InternalOrg, InternalName, "cache")

	case "freebsd", "openbsd", "netbsd":
		if isRoot {
			return "/var/cache/" + InternalOrg + "/" + InternalName
		}
		return os.ExpandEnv("$HOME/.cache/" + InternalOrg + "/" + InternalName)

	default:
		return os.ExpandEnv("$HOME/.cache/" + InternalOrg + "/" + InternalName)
	}
}

// GetBackupDir resolves an explicit override first, then a writable system
// backup location, and finally a mode-appropriate fallback. The startup
// privilege state is captured before privilege dropping so paths stay stable.
func GetBackupDir() string {
	if dir, ok := getOverride("backup"); ok && dir != "" {
		return dir
	}
	if dir := os.Getenv("BACKUP_DIR"); dir != "" {
		return dir
	}
	if IsRunningInContainer() {
		return "/data/backups/" + InternalName
	}

	systemDir := systemBackupDir()
	if writableBackupDir(systemDir) {
		return systemDir
	}
	if startedElevated {
		return filepath.Join(GetDataDir(), "backup")
	}
	return userBackupDir()
}

func systemBackupDir() string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join("/Library/Backups", InternalOrg, InternalName)
	case "windows":
		return filepath.Join(os.Getenv("ProgramData"), "Backups", InternalOrg, InternalName)
	case "freebsd", "openbsd", "netbsd":
		return filepath.Join("/var/backups", InternalOrg, InternalName)
	default:
		return filepath.Join("/mnt/Backups", InternalOrg, InternalName)
	}
}

func userBackupDir() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library/Backups", InternalOrg, InternalName)
	case "windows":
		return filepath.Join(os.Getenv("LOCALAPPDATA"), "Backups", InternalOrg, InternalName)
	default:
		return filepath.Join(home, ".local/share/Backups", InternalOrg, InternalName)
	}
}

func writableBackupDir(path string) bool {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return false
	}
	test, err := os.CreateTemp(path, ".write-test-")
	if err != nil {
		return false
	}
	name := test.Name()
	if err := test.Close(); err != nil {
		_ = os.Remove(name)
		return false
	}
	return os.Remove(name) == nil
}

// GetPIDFile returns the PID file location for the startup privilege mode.
func GetPIDFile() string {
	if !PIDFileEnabled() {
		return ""
	}
	if path, ok := getOverride("pid"); ok && path != "" {
		return path
	}
	if path := os.Getenv("PID_FILE"); path != "" {
		return path
	}
	if startedElevated && (runtime.GOOS == "linux" || runtime.GOOS == "darwin" || runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd") {
		return filepath.Join("/var/run", InternalOrg, InternalName, InternalName+".pid")
	}
	return filepath.Join(GetDataDir(), InternalName+".pid")
}

// GetSSLDir returns the SSL certificate directory.
func GetSSLDir() string {
	return filepath.Join(GetConfigDir(), "ssl")
}

// GetDatabaseDir returns the database directory for this project.
func GetDatabaseDir() string {
	if dir, ok := getOverride("database"); ok && dir != "" {
		return dir
	}
	if dir := os.Getenv("DATABASE_DIR"); dir != "" {
		return dir
	}
	if IsRunningInContainer() {
		return "/data/db/sqlite"
	}
	if runtime.GOOS == "linux" && startedElevated {
		return "/var/lib/" + InternalOrg + "/" + InternalName + "/db"
	}
	if runtime.GOOS == "windows" && startedElevated {
		return filepath.Join(os.Getenv("ProgramData"), InternalOrg, InternalName, "db")
	}
	if runtime.GOOS == "freebsd" || runtime.GOOS == "openbsd" || runtime.GOOS == "netbsd" {
		if startedElevated {
			return "/var/db/" + InternalOrg + "/" + InternalName + "/db"
		}
	}
	return filepath.Join(GetDataDir(), "db")
}

// GetGeoIPDir returns the OS-appropriate GeoIP database directory
// Per AI.md PART 4: Security DBs are under the data directory (geoip, blocklists, cve, trivy)
func GetGeoIPDir() string {
	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/" + ProjectName + "/security/geoip"
	}

	return filepath.Join(GetDataDir(), "security", "geoip")
}

// GetSecurityDir returns the OS-appropriate security directory
// Per AI.md PART 4: Contains geoip/, blocklists/, cve/, trivy/ — lives under data directory
func GetSecurityDir() string {
	// Container paths (per AI.md PART 4)
	if IsRunningInContainer() {
		return "/data/" + ProjectName + "/security"
	}

	return filepath.Join(GetDataDir(), "security")
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

// GetTemplatesDir returns the customizable template directory.
func GetTemplatesDir() string {
	return filepath.Join(GetConfigDir(), "template")
}

// GetEmailTemplatesDir returns the customizable email templates directory.
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
	if startedElevated {
		return 0o755
	}
	return 0o700
}

// GetSensitiveDirectoryPermissions returns permissions for sensitive directories
// Per AI.md PART 7: Tor dirs, SSL dirs always 0700
func GetSensitiveDirectoryPermissions() os.FileMode {
	return 0o700
}

// GetSensitiveFilePermissions returns permissions for sensitive files
// Per AI.md PART 7: keys, config files 0600
func GetSensitiveFilePermissions() os.FileMode {
	return 0o600
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
		GetBackupDir(),
		GetEmailTemplatesDir(),
		GetWebDataDir(),
		GetWellKnownDir(),
	}

	// Sensitive directories always use 0700
	sensitiveDirs := []string{
		GetTorDir(),
		GetTorKeysDir(),
		GetSSLDir(),
		GetSecurityDir(),
	}

	pidFile := GetPIDFile()

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

	if pidFile != "" {
		if err := ensureDir(filepath.Dir(pidFile), dirPerm); err != nil {
			return err
		}
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
		"/etc/" + InternalOrg + "/" + InternalName,
		"/var/lib/" + InternalOrg + "/" + InternalName,
		"/var/lib/" + InternalOrg + "/" + InternalName + "/db",
		"/var/log/" + InternalOrg + "/" + InternalName,
		"/var/cache/" + InternalOrg + "/" + InternalName,
	}
	if PIDFileEnabled() {
		systemDirs = append(systemDirs, filepath.Join("/var/run", InternalOrg, InternalName))
	}

	// Sensitive directories with 0700 permissions
	sensitiveDirs := []string{
		"/etc/" + InternalOrg + "/" + InternalName + "/security",
		"/etc/" + InternalOrg + "/" + InternalName + "/ssl",
		"/etc/" + InternalOrg + "/" + InternalName + "/tor",
	}

	// Create standard system directories
	for _, dir := range systemDirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		// Set ownership to service user
		if err := chownRecursive(dir, userName); err != nil {
			return err
		}
	}

	// Create sensitive directories with restricted permissions
	for _, dir := range sensitiveDirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := chownRecursive(dir, userName); err != nil {
			return err
		}
	}

	return nil
}

// chownRecursive changes ownership of a directory and its contents
func chownRecursive(path, userName string) error {
	// This is implemented in ownership_unix.go and ownership_windows.go
	return chownPath(path, userName)
}

// ChownRuntimeDirectories chowns the actual resolved runtime directories
// (which honor --data/--config/--cache/--log/--backup/--pid CLI overrides,
// per AI.md PART 8 step 7) to the given service user. Must be called while
// still root, after EnsureDirectories has created them, but before
// DropPrivileges (step 8c) — otherwise directories created via custom flags
// stay root-owned and the dropped-privilege process cannot write to them
// (e.g. PID file, database, sessions, logs).
func ChownRuntimeDirectories(userName string) error {
	dirs := []string{
		GetConfigDir(),
		GetDataDir(),
		GetLogDir(),
		GetCacheDir(),
		GetBackupDir(),
		GetDatabaseDir(),
		GetGeoIPDir(),
		GetSecurityDir(),
		GetSSLDir(),
		GetTorDir(),
	}
	if pidFile := GetPIDFile(); pidFile != "" {
		dirs = append(dirs, filepath.Dir(pidFile))
	}

	for _, dir := range dirs {
		if err := chownRecursive(dir, userName); err != nil {
			return fmt.Errorf("chown %s: %w", dir, err)
		}
	}

	return nil
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
		return "/etc/systemd/system/" + InternalName + ".service"

	case "darwin":
		if startedElevated {
			return "/Library/LaunchDaemons/io.github." + InternalOrg + "." + InternalName + ".plist"
		}
		return os.ExpandEnv("$HOME/Library/LaunchAgents/io.github." + InternalOrg + "." + InternalName + ".plist")

	// Windows uses Service Manager, not a file
	case "windows":
		return ""

	case "freebsd", "openbsd", "netbsd":
		return "/usr/local/etc/rc.d/" + InternalName

	default:
		return ""
	}
}

// GetBinaryPath returns the expected installation path for the binary
func GetBinaryPath() string {
	switch GetOS() {
	case "linux":
		if startedElevated {
			return "/usr/local/bin/" + ProjectName
		}
		return os.ExpandEnv("$HOME/.local/bin/" + ProjectName)

	case "darwin":
		if startedElevated {
			return "/usr/local/bin/" + ProjectName
		}
		return os.ExpandEnv("$HOME/bin/" + ProjectName)

	case "windows":
		if startedElevated {
			return filepath.Join(os.Getenv("ProgramFiles"), InternalOrg, InternalName, ProjectName+".exe")
		}
		return filepath.Join(os.Getenv("LocalAppData"), InternalOrg, InternalName, ProjectName+".exe")

	case "freebsd", "openbsd", "netbsd":
		return "/usr/local/bin/" + ProjectName

	default:
		return "/usr/local/bin/" + ProjectName
	}
}
