package service

import (
	"fmt"
	"os"
	"os/exec"
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

// createServiceDirectories creates directories needed by the service.
// Per AI.md: config, data, cache, log directories
func (sm *ServiceManager) createServiceDirectories() error {
	dirs := []string{
		config.GetConfigDir(),
		config.GetDataDir(),
		config.GetCacheDir(),
		config.GetLogDir(),
	}

	for _, dir := range dirs {
		if err := mkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
		// Set ownership to service user (non-fatal: may fail when not root)
		if err := runCommand("chown", "-R", "search:search", dir); err != nil {
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
