package path

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths represents OS-specific paths for the application
type Paths struct {
	// Runtime paths
	ConfigDir  string
	DataDir    string
	LogDir     string
	BackupDir  string
	PIDFile    string

	// Specific subdirectories
	SSLDir      string
	SecurityDir string
	DBDir       string
}

// goos is used for testing - allows overriding runtime.GOOS
var goos = runtime.GOOS

// Get returns OS-specific paths based on OS and privilege level
// Per AI.md PART 4 OS-SPECIFIC PATHS (NON-NEGOTIABLE)
func Get(projectOrg, projectName string, privileged bool) *Paths {
	switch goos {
	case "linux":
		return getLinuxPaths(projectOrg, projectName, privileged)
	case "darwin":
		return getDarwinPaths(projectOrg, projectName, privileged)
	case "freebsd", "openbsd", "netbsd":
		return getBSDPaths(projectOrg, projectName, privileged)
	case "windows":
		return getWindowsPaths(projectOrg, projectName, privileged)
	default:
		return getLinuxPaths(projectOrg, projectName, privileged)
	}
}

// getLinuxPaths returns Linux paths
func getLinuxPaths(org, name string, privileged bool) *Paths {
	if privileged {
		// Privileged (root/sudo)
		baseConfig := filepath.Join("/etc", org, name)
		baseData := filepath.Join("/var/lib", org, name)
		baseLog := filepath.Join("/var/log", org, name)
		baseBackup := filepath.Join("/mnt/Backups", org, name)

		return &Paths{
			ConfigDir:   baseConfig,
			DataDir:     baseData,
			LogDir:      baseLog,
			BackupDir:   baseBackup,
			PIDFile:     filepath.Join("/var/run", org, name+".pid"),
			SSLDir:      filepath.Join(baseConfig, "ssl"),
			SecurityDir: filepath.Join(baseConfig, "security"),
			DBDir:       filepath.Join(baseData, "db"),
		}
	}

	// User (non-privileged)
	homeDir, _ := os.UserHomeDir()
	baseConfig := filepath.Join(homeDir, ".config", org, name)
	baseData := filepath.Join(homeDir, ".local/share", org, name)
	baseBackup := filepath.Join(homeDir, ".local/share/Backups", org, name)

	return &Paths{
		ConfigDir:   baseConfig,
		DataDir:     baseData,
		LogDir:      filepath.Join(homeDir, ".local/log", org, name),
		BackupDir:   baseBackup,
		PIDFile:     filepath.Join(baseData, name+".pid"),
		SSLDir:      filepath.Join(baseConfig, "ssl"),
		SecurityDir: filepath.Join(baseConfig, "security"),
		DBDir:       filepath.Join(baseData, "db"),
	}
}

// getDarwinPaths returns macOS paths
func getDarwinPaths(org, name string, privileged bool) *Paths {
	if privileged {
		// Privileged (root/sudo)
		baseConfig := filepath.Join("/Library/Application Support", org, name)
		baseData := filepath.Join(baseConfig, "data")
		baseLog := filepath.Join("/Library/Logs", org, name)
		baseBackup := filepath.Join("/Library/Backups", org, name)

		return &Paths{
			ConfigDir:   baseConfig,
			DataDir:     baseData,
			LogDir:      baseLog,
			BackupDir:   baseBackup,
			PIDFile:     filepath.Join("/var/run", org, name+".pid"),
			SSLDir:      filepath.Join(baseConfig, "ssl"),
			SecurityDir: filepath.Join(baseConfig, "security"),
			DBDir:       filepath.Join(baseConfig, "db"),
		}
	}

	// User (non-privileged)
	homeDir, _ := os.UserHomeDir()
	baseConfig := filepath.Join(homeDir, "Library/Application Support", org, name)
	baseLog := filepath.Join(homeDir, "Library/Logs", org, name)
	baseBackup := filepath.Join(homeDir, "Library/Backups", org, name)

	return &Paths{
		ConfigDir:   baseConfig,
		DataDir:     baseConfig,
		LogDir:      baseLog,
		BackupDir:   baseBackup,
		PIDFile:     filepath.Join(baseConfig, name+".pid"),
		SSLDir:      filepath.Join(baseConfig, "ssl"),
		SecurityDir: filepath.Join(baseConfig, "security"),
		DBDir:       filepath.Join(baseConfig, "db"),
	}
}

// getBSDPaths returns BSD paths (FreeBSD, OpenBSD, NetBSD)
func getBSDPaths(org, name string, privileged bool) *Paths {
	if privileged {
		// Privileged (root/sudo/doas)
		baseConfig := filepath.Join("/usr/local/etc", org, name)
		baseData := filepath.Join("/var/db", org, name)
		baseLog := filepath.Join("/var/log", org, name)
		baseBackup := filepath.Join("/var/backups", org, name)

		return &Paths{
			ConfigDir:   baseConfig,
			DataDir:     baseData,
			LogDir:      baseLog,
			BackupDir:   baseBackup,
			PIDFile:     filepath.Join("/var/run", org, name+".pid"),
			SSLDir:      filepath.Join(baseConfig, "ssl"),
			SecurityDir: filepath.Join(baseConfig, "security"),
			DBDir:       filepath.Join(baseData, "db"),
		}
	}

	// User (non-privileged)
	homeDir, _ := os.UserHomeDir()
	baseConfig := filepath.Join(homeDir, ".config", org, name)
	baseData := filepath.Join(homeDir, ".local/share", org, name)
	baseBackup := filepath.Join(homeDir, ".local/share/Backups", org, name)

	return &Paths{
		ConfigDir:   baseConfig,
		DataDir:     baseData,
		LogDir:      filepath.Join(homeDir, ".local/log", org, name),
		BackupDir:   baseBackup,
		PIDFile:     filepath.Join(baseData, name+".pid"),
		SSLDir:      filepath.Join(baseConfig, "ssl"),
		SecurityDir: filepath.Join(baseConfig, "security"),
		DBDir:       filepath.Join(baseData, "db"),
	}
}

// getWindowsPaths returns Windows paths
func getWindowsPaths(org, name string, privileged bool) *Paths {
	if privileged {
		// Privileged (Administrator)
		programData := os.Getenv("ProgramData")
		if programData == "" {
			programData = "C:\\ProgramData"
		}

		baseConfig := filepath.Join(programData, org, name)
		baseData := filepath.Join(baseConfig, "data")
		baseLog := filepath.Join(baseConfig, "logs")
		baseBackup := filepath.Join(programData, "Backups", org, name)

		return &Paths{
			ConfigDir:   baseConfig,
			DataDir:     baseData,
			LogDir:      baseLog,
			BackupDir:   baseBackup,
			PIDFile:     filepath.Join(baseConfig, name+".pid"),
			SSLDir:      filepath.Join(baseConfig, "ssl"),
			SecurityDir: filepath.Join(baseConfig, "security"),
			DBDir:       filepath.Join(baseConfig, "db"),
		}
	}

	// User (non-privileged)
	appData := os.Getenv("AppData")
	localAppData := os.Getenv("LocalAppData")
	if appData == "" {
		homeDir, _ := os.UserHomeDir()
		appData = filepath.Join(homeDir, "AppData", "Roaming")
		localAppData = filepath.Join(homeDir, "AppData", "Local")
	}

	baseConfig := filepath.Join(appData, org, name)
	baseData := filepath.Join(localAppData, org, name)
	baseLog := filepath.Join(baseData, "logs")
	baseBackup := filepath.Join(localAppData, "Backups", org, name)

	return &Paths{
		ConfigDir:   baseConfig,
		DataDir:     baseData,
		LogDir:      baseLog,
		BackupDir:   baseBackup,
		PIDFile:     filepath.Join(baseData, name+".pid"),
		SSLDir:      filepath.Join(baseConfig, "ssl"),
		SecurityDir: filepath.Join(baseConfig, "security"),
		DBDir:       filepath.Join(baseData, "db"),
	}
}

// IsPrivileged returns true if running with elevated privileges
func IsPrivileged() bool {
	switch goos {
	case "windows":
		// On Windows, check if running as administrator
		// This is simplified - proper check would use Windows API
		return os.Getenv("USERPROFILE") == "" || os.Getuid() == 0
	default:
		// On Unix-like systems, check if UID is 0 (root)
		return os.Getuid() == 0
	}
}

// GetConfigPath returns the path to server.yml
func (p *Paths) GetConfigPath() string {
	return filepath.Join(p.ConfigDir, "server.yml")
}

// EnsureDirs creates all necessary directories
func (p *Paths) EnsureDirs() error {
	dirs := []string{
		p.ConfigDir,
		p.DataDir,
		p.LogDir,
		p.BackupDir,
		p.SSLDir,
		filepath.Join(p.SSLDir, "letsencrypt"),
		filepath.Join(p.SSLDir, "local"),
		p.SecurityDir,
		filepath.Join(p.SecurityDir, "geoip"),
		filepath.Join(p.SecurityDir, "blocklists"),
		filepath.Join(p.SecurityDir, "cve"),
		filepath.Join(p.SecurityDir, "trivy"),
		p.DBDir,
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	return nil
}
