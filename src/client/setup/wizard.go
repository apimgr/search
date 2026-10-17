// Package setup implements the first-run configuration wizard for the client.
// Per AI.md PART 32: the CLI must be usable immediately after install; when no
// server is configured the wizard collects the minimum settings and writes
// cli.yml with 0600 permissions.
package setup

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/client/clicfg"
	"github.com/apimgr/search/src/client/gui"
	"github.com/apimgr/search/src/client/path"
	"github.com/apimgr/search/src/common/display"
)

// ErrNoTerminal is returned when the wizard is required but the process has no
// interactive terminal to prompt on (headless mode is an error per PART 32).
var ErrNoTerminal = fmt.Errorf("setup required but no interactive terminal is available")

// Mode names the interface the wizard runs in.
type Mode string

const (
	// ModeTUI is the terminal wizard, used on SSH/Mosh sessions and local TTYs.
	ModeTUI Mode = "tui"
	// ModeGUI is the graphical wizard, only reachable in gui-tagged builds.
	ModeGUI Mode = "gui"
	// ModeNone means no interactive interface is available.
	ModeNone Mode = "none"
)

// SelectSetupMode picks the wizard interface for the current environment.
// Per AI.md PART 32: remote sessions always use the TUI even when a display is
// advertised, a local display prefers the GUI, and a bare TTY uses the TUI.
func SelectSetupMode(env display.Env) Mode {
	if env.IsSSH || env.IsMosh {
		return ModeTUI
	}
	if env.HasDisplay {
		return ModeGUI
	}
	if env.IsTerminal {
		return ModeTUI
	}
	return ModeNone
}

// IsConfigured reports whether a server address is already known.
func IsConfigured() bool {
	if strings.TrimSpace(clicfg.GetString("server.primary")) != "" {
		return true
	}
	return strings.TrimSpace(clicfg.GetString("server.address")) != ""
}

// EnsureConfigured runs the setup wizard when no server is configured.
// It is a no-op once a server address is present.
func EnsureConfigured() error {
	if IsConfigured() {
		return nil
	}
	return RunSetupWizard()
}

// RunSetupWizard prompts for the client settings and persists them to cli.yml.
func RunSetupWizard() error {
	env := display.Detect()
	override := strings.ToLower(strings.TrimSpace(clicfg.GetString("display.mode")))
	if override == "gui" {
		if !env.HasDisplay || env.IsSSH || env.IsMosh || !gui.Available() {
			return fmt.Errorf("GUI setup requires a local display and a GUI-enabled build")
		}
		return gui.Configure()
	}
	if override == "tui" {
		if !env.IsTerminal {
			return fmt.Errorf("TUI setup requires an interactive terminal")
		}
		if !clicfg.GetBool("tui.enabled") {
			return fmt.Errorf("TUI mode is disabled by tui.enabled in cli.yml")
		}
		return runWizard(os.Stdin, os.Stdout)
	}
	if override != "" && override != "auto" && override != "tui" {
		return fmt.Errorf("invalid display.mode %q; use auto, gui, or tui", override)
	}
	switch SelectSetupMode(env) {
	case ModeGUI:
		if gui.Available() {
			return gui.Configure()
		}
		if !env.IsTerminal {
			return ErrNoTerminal
		}
	case ModeNone:
		return ErrNoTerminal
	}
	return runWizard(os.Stdin, os.Stdout)
}

// runWizard drives the prompt loop against explicit streams so the flow is
// testable without a controlling terminal.
func runWizard(in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)

	if _, err := fmt.Fprintln(out, "First-run setup"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "Answer a few questions to create cli.yml."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}

	var serverAddr, tokenVal string
	for {
		serverAddr, err := promptServer(reader, out)
		if err != nil {
			return err
		}
		tokenVal, err = prompt(reader, out, "API token (leave blank for anonymous access)", "")
		if err != nil {
			return err
		}
		probe := api.NewClient(serverAddr, tokenVal, 10)
		if _, err := probe.GetVersion(); err == nil {
			break
		} else {
			if _, err := fmt.Fprintf(out, "Connection failed: %v\n", err); err != nil {
				return err
			}
			retry, promptErr := promptChoice(reader, out, "Try another server", "yes", []string{"yes", "no"})
			if promptErr != nil {
				return promptErr
			}
			if retry == "no" {
				return fmt.Errorf("setup canceled because the server could not be reached")
			}
		}
	}
	clicfg.Set("server.primary", serverAddr)
	if tokenVal != "" {
		clicfg.Set("auth.token", tokenVal)
	}

	format, err := promptChoice(reader, out, "Default output format", "table",
		[]string{"table", "json", "yaml", "plain", "csv"})
	if err != nil {
		return err
	}
	clicfg.Set("output.format", format)

	themeName, err := promptChoice(reader, out, "TUI theme", "dark", []string{"dark", "light"})
	if err != nil {
		return err
	}
	clicfg.Set("tui.theme", themeName)

	if err := path.EnsureDirs(); err != nil {
		return fmt.Errorf("failed to create client directories: %w", err)
	}

	configPath, err := path.ConfigFile()
	if err != nil {
		return fmt.Errorf("failed to resolve config path: %w", err)
	}
	if err := clicfg.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("failed to write %s: %w", configPath, err)
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", configPath, err)
	}

	_, err = fmt.Fprintf(out, "\nConfiguration saved to %s\n", configPath)
	return err
}

// promptServer asks for the server URL until a syntactically valid one is given.
func promptServer(reader *bufio.Reader, out io.Writer) (string, error) {
	for {
		answer, err := prompt(reader, out, "Server URL (e.g. https://scour.li)", "")
		if err != nil {
			return "", err
		}
		if answer == "" {
			if _, err := fmt.Fprintln(out, "A server URL is required."); err != nil {
				return "", err
			}
			continue
		}
		if !strings.Contains(answer, "://") {
			answer = "https://" + answer
		}
		parsed, perr := url.Parse(answer)
		if perr != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			if _, err := fmt.Fprintln(out, "That is not a valid URL; try again."); err != nil {
				return "", err
			}
			continue
		}
		return strings.TrimRight(answer, "/"), nil
	}
}

// promptChoice asks a question restricted to a set of allowed answers.
func promptChoice(reader *bufio.Reader, out io.Writer, question, def string, allowed []string) (string, error) {
	label := fmt.Sprintf("%s (%s)", question, strings.Join(allowed, "/"))
	for {
		answer, err := prompt(reader, out, label, def)
		if err != nil {
			return "", err
		}
		for _, a := range allowed {
			if answer == a {
				return answer, nil
			}
		}
		if _, err := fmt.Fprintf(out, "Choose one of: %s\n", strings.Join(allowed, ", ")); err != nil {
			return "", err
		}
	}
}

// prompt reads one line, returning def when the answer is empty.
func prompt(reader *bufio.Reader, out io.Writer, question, def string) (string, error) {
	if def != "" {
		if _, err := fmt.Fprintf(out, "%s [%s]: ", question, def); err != nil {
			return "", err
		}
	} else {
		if _, err := fmt.Fprintf(out, "%s: ", question); err != nil {
			return "", err
		}
	}
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		if err == io.EOF {
			return "", ErrNoTerminal
		}
		return "", err
	}
	answer := strings.TrimSpace(line)
	if answer == "" {
		return def, nil
	}
	return answer, nil
}
