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
	"github.com/apimgr/search/src/client/surface"
	"github.com/apimgr/search/src/common/display"
)

// ErrNoTerminal is returned when the wizard is required but the process has no
// interactive terminal to prompt on (headless mode is an error per PART 32).
var ErrNoTerminal error = noTerminalError{}

type noTerminalError struct{}

func (noTerminalError) Error() string {
	return surface.Text("client.setup.no_terminal")
}

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
			return fmt.Errorf("%s", surface.Text("client.setup.gui_requires_local_display"))
		}
		return gui.Configure()
	}
	if override == "tui" {
		if !env.IsTerminal {
			return fmt.Errorf("%s", surface.Text("client.setup.tui_requires_terminal"))
		}
		if !clicfg.GetBool("tui.enabled") {
			return fmt.Errorf("%s", surface.Text("client.setup.tui_disabled"))
		}
		return runWizard(os.Stdin, os.Stdout)
	}
	if override != "" && override != "auto" && override != "tui" {
		return fmt.Errorf("%s", surface.Text("client.setup.invalid_display_mode", override))
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

	if _, err := fmt.Fprintln(out, surface.Text("client.setup.wizard_title")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, surface.Text("client.setup.wizard_intro")); err != nil {
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
		tokenVal, err = prompt(reader, out, surface.Text("client.setup.token_prompt"), "")
		if err != nil {
			return err
		}
		probe := api.NewClient(serverAddr, tokenVal, 10)
		if _, err := probe.GetVersion(); err == nil {
			if tokenVal != "" {
				kind, authErr := probe.CheckAuth()
				if authErr != nil {
					if _, writeErr := fmt.Fprintln(out, surface.Text("client.setup.auth_failed", authErr)); writeErr != nil {
						return writeErr
					}
					retry, promptErr := promptChoice(reader, out, surface.Text("client.setup.retry_server_token"), "yes", []string{"yes", "no"})
					if promptErr != nil {
						return promptErr
					}
					if retry == "no" {
						return fmt.Errorf("%s", surface.Text("client.setup.invalid_token_cancelled"))
					}
					continue
				}
				if _, writeErr := fmt.Fprintln(out, surface.Text("client.setup.auth_valid", kind)); writeErr != nil {
					return writeErr
				}
			}
			break
		} else {
			if _, err := fmt.Fprintln(out, surface.Text("client.setup.connection_failed", err)); err != nil {
				return err
			}
			retry, promptErr := promptChoice(reader, out, surface.Text("client.setup.retry_server"), "yes", []string{"yes", "no"})
			if promptErr != nil {
				return promptErr
			}
			if retry == "no" {
				return fmt.Errorf("%s", surface.Text("client.setup.server_unreachable_cancelled"))
			}
		}
	}
	clicfg.Set("server.primary", serverAddr)
	if tokenVal != "" {
		clicfg.Set("auth.token", tokenVal)
	}

	format, err := promptChoice(reader, out, surface.Text("client.setup.output_format"), "table",
		[]string{"table", "json", "yaml", "plain", "csv"})
	if err != nil {
		return err
	}
	clicfg.Set("output.format", format)

	themeName, err := promptChoice(reader, out, surface.Text("client.setup.tui_theme"), "dark", []string{"dark", "light"})
	if err != nil {
		return err
	}
	clicfg.Set("tui.theme", themeName)

	if err := path.EnsureDirs(); err != nil {
		return fmt.Errorf("%s: %w", surface.Text("client.setup.create_directories_failed"), err)
	}

	configPath, err := path.ConfigFile()
	if err != nil {
		return fmt.Errorf("%s: %w", surface.Text("client.setup.resolve_config_failed"), err)
	}
	if err := clicfg.WriteConfigAs(configPath); err != nil {
		return fmt.Errorf("%s: %w", surface.Text("client.setup.write_config_failed", configPath), err)
	}
	if err := os.Chmod(configPath, 0600); err != nil {
		return fmt.Errorf("%s: %w", surface.Text("client.setup.set_permissions_failed", configPath), err)
	}

	_, err = fmt.Fprint(out, "\n"+surface.Text("client.setup.config_saved", configPath)+"\n")
	return err
}

// promptServer asks for the server URL until a syntactically valid one is given.
func promptServer(reader *bufio.Reader, out io.Writer) (string, error) {
	for {
		answer, err := prompt(reader, out, surface.Text("client.setup.server_url_prompt"), "")
		if err != nil {
			return "", err
		}
		if answer == "" {
			if _, err := fmt.Fprintln(out, surface.Text("client.setup.server_url_required")); err != nil {
				return "", err
			}
			continue
		}
		if !strings.Contains(answer, "://") {
			answer = "https://" + answer
		}
		parsed, perr := url.Parse(answer)
		if perr != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			if _, err := fmt.Fprintln(out, surface.Text("client.setup.invalid_url_retry")); err != nil {
				return "", err
			}
			continue
		}
		return strings.TrimRight(answer, "/"), nil
	}
}

// promptChoice asks a question restricted to a set of allowed answers.
func promptChoice(reader *bufio.Reader, out io.Writer, question, def string, allowed []string) (string, error) {
	label := surface.Text("client.setup.choice_label", question, strings.Join(allowed, "/"))
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
		if _, err := fmt.Fprintln(out, surface.Text("client.setup.choose_one", strings.Join(allowed, ", "))); err != nil {
			return "", err
		}
	}
}

// prompt reads one line, returning def when the answer is empty.
func prompt(reader *bufio.Reader, out io.Writer, question, def string) (string, error) {
	if def != "" {
		if _, err := fmt.Fprint(out, surface.Text("client.setup.prompt_default", question, def)); err != nil {
			return "", err
		}
	} else {
		if _, err := fmt.Fprint(out, surface.Text("client.setup.prompt", question)); err != nil {
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
