package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/apimgr/search/src/config"
)

// ServiceManager handles system service installation
type ServiceManager struct {
	config *config.Config
}

// NewServiceManager creates a new service manager
func NewServiceManager(cfg *config.Config) *ServiceManager {
	return &ServiceManager{config: cfg}
}

// mkdirAll is a wrapper for os.MkdirAll enabling test injection
var mkdirAll = os.MkdirAll

// runCmd is a wrapper for runCommand enabling test injection
var runCmd = runCommand

// createServiceDirectories creates directories needed by the service.
// Per AI.md PART 23: config, data, cache, log at 0755; security/, ssl/, tor/
// inside the config dir at 0700 (sensitive key material).
func (sm *ServiceManager) createServiceDirectories() error {
	configDir := config.GetConfigDir()

	// Standard directories at 0755
	dirs := []string{
		configDir,
		config.GetDataDir(),
		config.GetCacheDir(),
		config.GetLogDir(),
	}

	for _, dir := range dirs {
		if err := mkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
		// Set ownership to service user (non-fatal: may fail when not root)
		if err := runCmd("chown", "-R", "search:search", dir); err != nil {
			_ = err
		}
	}

	// Security-sensitive subdirectories at 0700 per AI.md PART 23
	secureDirs := []string{
		filepath.Join(configDir, "security"),
		filepath.Join(configDir, "ssl"),
		filepath.Join(configDir, "tor"),
	}

	for _, dir := range secureDirs {
		if err := mkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
		if err := runCmd("chown", "-R", "search:search", dir); err != nil {
			_ = err
		}
	}

	return nil
}

func (sm *ServiceManager) renderTemplate(tmpl string, data interface{}) (string, error) {
	t, err := template.New("service").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(content)
	return err
}

// GetServiceStatus returns formatted service status information
func GetServiceStatus() string {
	cfg := config.DefaultConfig()
	sm := NewServiceManager(cfg)

	status, err := sm.Status()
	if err != nil {
		return fmt.Sprintf("Service status: unknown (%v)", err)
	}

	return fmt.Sprintf("Service status: %s", status)
}
