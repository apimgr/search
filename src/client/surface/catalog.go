// Package surface defines the API-facing command screens shared by the GUI
// and TUI. The order follows root CLI help order and nested usage order.
package surface

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/apimgr/search/src/client/api"
	"github.com/apimgr/search/src/config"
	"github.com/apimgr/search/src/common/i18n"
)

// Argument defines one structured value accepted by a command screen.
type Argument struct {
	Name        string
	Label       string
	Placeholder string
	Key         string
	Required    bool
	Multiple    bool
}

// Command describes one server-facing CLI leaf and its matching GUI screen
// and TUI view.
type Command struct {
	Path        []string
	Description string
	Arguments   []Argument
}

var commands = []Command{
	{Path: []string{"alerts", "create"}, Description: "client.surface.command_description", Arguments: []Argument{
		{Name: "query", Label: "client.surface.arguments.query.label", Placeholder: "client.surface.enter_value", Key: "query", Required: true},
		{Name: "category", Label: "client.surface.arguments.category.label", Placeholder: "client.surface.enter_value", Key: "category"},
		{Name: "language", Label: "client.surface.arguments.language.label", Placeholder: "client.surface.enter_value", Key: "language"},
		{Name: "region", Label: "client.surface.arguments.region.label", Placeholder: "client.surface.enter_value", Key: "region"},
		{Name: "engines", Label: "client.surface.arguments.engines.label", Placeholder: "client.surface.enter_value", Key: "engines"},
		{Name: "safe_search", Label: "client.surface.arguments.safe_search.label", Placeholder: "client.surface.enter_value", Key: "safe_search"},
		{Name: "frequency", Label: "client.surface.arguments.frequency.label", Placeholder: "client.surface.enter_value", Key: "frequency"},
		{Name: "email", Label: "client.surface.arguments.email.label", Placeholder: "client.surface.enter_value", Key: "email"},
		{Name: "deliver_email", Label: "client.surface.arguments.deliver_email.label", Placeholder: "client.surface.enter_value", Key: "deliver_email"},
		{Name: "deliver_rss", Label: "client.surface.arguments.deliver_rss.label", Placeholder: "client.surface.enter_value", Key: "deliver_rss"},
		{Name: "deliver_webhook", Label: "client.surface.arguments.deliver_webhook.label", Placeholder: "client.surface.enter_value", Key: "deliver_webhook"},
		{Name: "webhook_url", Label: "client.surface.arguments.webhook_url.label", Placeholder: "client.surface.enter_value", Key: "webhook_url"},
	}},
	{Path: []string{"alerts", "get"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "manage_token", Label: "client.surface.arguments.manage_token.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"alerts", "update"}, Description: "client.surface.command_description", Arguments: []Argument{
		{Name: "manage_token", Label: "client.surface.arguments.manage_token.label", Placeholder: "client.surface.enter_value", Required: true},
		{Name: "query", Label: "client.surface.arguments.query.label", Placeholder: "client.surface.enter_value", Key: "query"},
		{Name: "category", Label: "client.surface.arguments.category.label", Placeholder: "client.surface.enter_value", Key: "category"},
		{Name: "language", Label: "client.surface.arguments.language.label", Placeholder: "client.surface.enter_value", Key: "language"},
		{Name: "region", Label: "client.surface.arguments.region.label", Placeholder: "client.surface.enter_value", Key: "region"},
		{Name: "engines", Label: "client.surface.arguments.engines.label", Placeholder: "client.surface.enter_value", Key: "engines"},
		{Name: "safe_search", Label: "client.surface.arguments.safe_search.label", Placeholder: "client.surface.enter_value", Key: "safe_search"},
		{Name: "frequency", Label: "client.surface.arguments.frequency.label", Placeholder: "client.surface.enter_value", Key: "frequency"},
		{Name: "deliver_email", Label: "client.surface.arguments.deliver_email.label", Placeholder: "client.surface.enter_value", Key: "deliver_email"},
		{Name: "deliver_rss", Label: "client.surface.arguments.deliver_rss.label", Placeholder: "client.surface.enter_value", Key: "deliver_rss"},
		{Name: "deliver_webhook", Label: "client.surface.arguments.deliver_webhook.label", Placeholder: "client.surface.enter_value", Key: "deliver_webhook"},
		{Name: "webhook_url", Label: "client.surface.arguments.webhook_url.label", Placeholder: "client.surface.enter_value", Key: "webhook_url"},
	}},
	{Path: []string{"alerts", "delete"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "manage_token", Label: "client.surface.arguments.manage_token.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"answer"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "slug", Label: "client.surface.arguments.slug.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"autocomplete"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "prefix", Label: "client.surface.arguments.prefix.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"autodiscover"}, Description: "client.surface.command_description"},
	{Path: []string{"bangs"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "filter", Label: "client.surface.arguments.filter.label", Placeholder: "client.surface.enter_value"}}},
	{Path: []string{"categories"}, Description: "client.surface.command_description"},
	{Path: []string{"engines"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "id", Label: "client.surface.arguments.id.label", Placeholder: "client.surface.enter_value"}}},
	{Path: []string{"health"}, Description: "client.surface.command_description"},
	{Path: []string{"info"}, Description: "client.surface.command_description"},
	{Path: []string{"instant"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "query", Label: "client.surface.arguments.query.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"metrics"}, Description: "client.surface.command_description"},
	{Path: []string{"preferences"}, Description: "client.surface.command_description"},
	{Path: []string{"preferences", "set"}, Description: "client.surface.command_description", Arguments: []Argument{
		{Name: "storage", Label: "client.surface.arguments.storage.label", Placeholder: "client.surface.enter_value", Key: "storage"},
		{Name: "fields", Label: "client.surface.arguments.fields.label", Placeholder: "client.surface.enter_value", Key: "fields"},
	}},
	{Path: []string{"related"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "query", Label: "client.surface.arguments.query.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"scheduler", "list"}, Description: "client.surface.command_description"},
	{Path: []string{"scheduler", "show"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "task", Label: "client.surface.arguments.task.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"scheduler", "run"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "task", Label: "client.surface.arguments.task.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"scheduler", "history"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "task", Label: "client.surface.arguments.task.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"search"}, Description: "client.surface.command_description", Arguments: []Argument{{Name: "query", Label: "client.surface.arguments.query.label", Placeholder: "client.surface.enter_value", Required: true}}},
	{Path: []string{"server-config"}, Description: "client.surface.command_description"},
	{Path: []string{"status"}, Description: "client.surface.command_description"},
	{Path: []string{"widget"}, Description: "client.surface.command_description", Arguments: []Argument{
		{Name: "name", Label: "client.surface.arguments.widget.label", Placeholder: "client.surface.enter_value", Required: true},
		{Name: "parameters", Label: "client.surface.arguments.parameters.label", Placeholder: "client.surface.enter_value", Multiple: true},
	}},
	{Path: []string{"widgets"}, Description: "client.surface.command_description"},
}

// Text localizes client UI text using SEARCH_LANG or the system locale.
func Text(key string, args ...interface{}) string {
	lang := strings.TrimSpace(os.Getenv("SEARCH_LANG"))
	if lang == "" {
		lang = strings.TrimSpace(os.Getenv("LANG"))
	}
	manager, err := i18n.CachedDefaultManager()
	if err != nil || manager == nil {
		return key
	}
	return manager.T(manager.ResolveSupportedLanguage(lang), key, args...)
}

// Commands returns the ordered list of screens/views without exposing the
// package's backing slices.
func Commands() []Command {
	result := make([]Command, len(commands))
	for i, command := range commands {
		result[i] = Command{
			Path:        append([]string(nil), command.Path...),
			Description: command.Description,
			Arguments:   append([]Argument(nil), command.Arguments...),
		}
	}
	return result
}

// Name returns the CLI path that identifies this screen/view.
func (command Command) Name() string { return strings.Join(command.Path, " ") }

// DescriptionText returns a localized description of the command screen.
func DescriptionText(command Command) string {
	return Text("client.surface.command_description", command.Name())
}

// BuildArgs validates structured field values and converts them to discrete
// CLI arguments without splitting or interpreting user input as shell syntax.
func BuildArgs(command Command, values map[string]string) ([]string, error) {
	known := make(map[string]Argument, len(command.Arguments))
	for _, argument := range command.Arguments {
		known[argument.Name] = argument
	}
	for name := range values {
		if _, ok := known[name]; !ok {
			return nil, errors.New(Text("client.surface.unknown_argument", name, command.Name()))
		}
	}

	var result []string
	for _, argument := range command.Arguments {
		value := strings.TrimSpace(values[argument.Name])
		if value == "" {
			if argument.Required {
				return nil, errors.New(Text("client.surface.required_argument", Text(argument.Label)))
			}
			continue
		}
		parts := []string{value}
		if argument.Multiple {
			parts = strings.Split(value, ";")
		}
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if argument.Key != "" {
				part = argument.Key + "=" + part
			}
			result = append(result, part)
		}
	}
	return result, nil
}

// Run executes the corresponding existing CLI leaf command and returns its
// rendered output. Structured values become individual argv fields without
// shell parsing or splitting values that contain spaces.
func Run(ctx context.Context, client *api.Client, command Command, values map[string]string) (string, error) {
	if client == nil {
		return "", errors.New(Text("client.surface.api_client_missing"))
	}
	commandArgs, err := BuildArgs(command, values)
	if err != nil {
		return "", err
	}
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("%s: %w", Text("client.surface.locate_cli"), err)
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
	}

	args := []string{"--server", client.BaseURL, "--output", "plain"}
	if client.Token != "" {
		baseDir := filepath.Join(os.TempDir(), config.ProjectOrg)
		if err := os.MkdirAll(baseDir, 0755); err != nil {
			return "", fmt.Errorf("%s: %w", Text("client.surface.create_token_directory"), err)
		}
		tempDir, err := os.MkdirTemp(baseDir, config.InternalName+"-client-ui-")
		if err != nil {
			return "", fmt.Errorf("%s: %w", Text("client.surface.create_token_file_directory"), err)
		}
		defer os.RemoveAll(tempDir)
		if err := os.Chmod(tempDir, 0700); err != nil {
			return "", fmt.Errorf("%s: %w", Text("client.surface.secure_token_directory"), err)
		}
		tokenFile := filepath.Join(tempDir, "token")
		if err := os.WriteFile(tokenFile, []byte(client.Token), 0600); err != nil {
			return "", fmt.Errorf("%s: %w", Text("client.surface.write_token_file"), err)
		}
		args = append(args, "--token-file", tokenFile)
	}
	if client.UserContext != "" {
		args = append(args, "--user", client.UserContext)
	}
	args = append(args, command.Path...)
	args = append(args, commandArgs...)
	process := exec.CommandContext(ctx, executable, args...)
	process.Env = withoutTokenEnvironment(os.Environ())
	var stdout, stderr strings.Builder
	process.Stdout = &stdout
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return stdout.String(), fmt.Errorf("%s", message)
	}
	return stdout.String(), nil
}

func withoutTokenEnvironment(environment []string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		if strings.HasPrefix(entry, "SEARCH_TOKEN=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
