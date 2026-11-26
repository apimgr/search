package surface

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/apimgr/search/src/common/i18n"
)

func TestCommandScreensAndViewsCoverEachAPIFacingLeaf(t *testing.T) {
	want := []string{
		"alerts create", "alerts get", "alerts update", "alerts delete",
		"answer", "autocomplete", "autodiscover", "bangs", "categories",
		"engines", "health", "info", "instant", "metrics", "preferences",
		"preferences set", "related", "scheduler list", "scheduler show",
		"scheduler run", "scheduler history", "search", "server-config", "status", "widget",
		"widgets",
	}
	commands := Commands()
	if len(commands) != len(want) {
		t.Fatalf("surface count = %d, want %d", len(commands), len(want))
	}
	seen := make(map[string]bool, len(commands))
	for i, command := range commands {
		name := command.Name()
		if name != want[i] {
			t.Errorf("screen/view %d = %q, want %q", i, name, want[i])
		}
		if seen[name] {
			t.Errorf("duplicate screen/view for %q", name)
		}
		seen[name] = true
		if len(command.Path) == 0 || strings.TrimSpace(command.Description) == "" {
			t.Errorf("screen/view %q has no path or description", name)
		}
	}
}

func TestBuildArgsKeepsStructuredValuesTogether(t *testing.T) {
	command := Command{Path: []string{"search"}, Arguments: []Argument{{Name: "query", Label: "Query", Required: true}}}
	args, err := BuildArgs(command, map[string]string{"query": `privacy tools "quoted"`})
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 || args[0] != `privacy tools "quoted"` {
		t.Fatalf("BuildArgs() = %#v, want one untouched argument", args)
	}
}

func TestBuildArgsValidatesRequiredAndUnknownFields(t *testing.T) {
	command := Command{Path: []string{"answer"}, Arguments: []Argument{{Name: "slug", Label: "Slug", Required: true}}}
	if _, err := BuildArgs(command, nil); err == nil {
		t.Fatal("BuildArgs() accepted a missing required value")
	}
	if _, err := BuildArgs(command, map[string]string{"unknown": "value"}); err == nil {
		t.Fatal("BuildArgs() accepted an unknown field")
	}
}

func TestSurfaceAndOwnerTokenTranslationsExistForEveryLocale(t *testing.T) {
	keys := []string{
		"client.surface.title", "client.surface.menu_title", "client.surface.choose_command",
		"client.surface.command_description", "client.surface.instructions", "client.surface.run",
		"client.surface.run_accessibility", "client.surface.argument_accessibility", "client.surface.enter_value",
		"client.surface.required", "client.surface.running", "client.surface.running_command",
		"client.surface.command_failed", "client.surface.no_commands", "client.surface.no_selection",
		"client.surface.error_label", "client.surface.no_results", "client.surface.help.title",
		"client.surface.help.return", "client.surface.help.menu_move", "client.surface.help.menu_previous",
		"client.surface.help.open_run", "client.surface.help.focus", "client.surface.help.back",
		"client.surface.help.help", "client.surface.help.quit", "client.surface.footer.menu",
		"client.surface.footer.form", "client.surface.required_argument", "client.surface.unknown_argument",
		"client.surface.locate_cli", "client.surface.create_token_directory",
		"client.surface.create_token_file_directory", "client.surface.secure_token_directory",
		"client.surface.write_token_file", "client.surface.api_client_missing",
		"client.surface.setup.initial_status", "client.surface.setup.invalid_url",
		"client.surface.setup.connection_failed", "client.surface.setup.config_directory_failed",
		"client.surface.setup.config_file_failed", "client.surface.setup.save_failed",
		"client.surface.setup.secure_failed", "client.surface.setup.connected", "client.surface.setup.title",
		"client.surface.setup.instructions", "client.surface.setup.server_label",
		"client.surface.setup.server_placeholder", "client.surface.setup.token_label",
		"client.surface.setup.token_placeholder", "client.surface.setup.connect_button",
		"client.surface.setup.connect_accessibility", "client.surface.setup.window_title",
		"client.surface.setup.closed", "client.surface.display_init_failed",
		"client.surface.display_resize_failed", "client.surface.display_render_failed",
		"client.surface.gui_unavailable",
		"client.setup.no_terminal", "client.setup.gui_requires_local_display",
		"client.setup.tui_requires_terminal", "client.setup.tui_disabled",
		"client.setup.invalid_display_mode", "client.setup.wizard_title", "client.setup.wizard_intro",
		"client.setup.token_prompt", "client.setup.auth_failed", "client.setup.retry_server_token",
		"client.setup.invalid_token_cancelled", "client.setup.auth_valid", "client.setup.connection_failed",
		"client.setup.retry_server", "client.setup.server_unreachable_cancelled",
		"client.setup.output_format", "client.setup.tui_theme", "client.setup.create_directories_failed",
		"client.setup.resolve_config_failed", "client.setup.write_config_failed",
		"client.setup.set_permissions_failed", "client.setup.config_saved",
		"client.setup.server_url_prompt", "client.setup.server_url_required",
		"client.setup.invalid_url_retry", "client.setup.choice_label", "client.setup.choose_one",
		"client.setup.prompt_default", "client.setup.prompt", "client.init.log_warning",
		"client.init.directories_failed",
		"swagger.text.missing_or_invalid_owner_token_e7687308b918",
		"swagger.text.valid_owner_or_linked_token_lacks_operator_scope_42a78f769c1c",
		"swagger.text.primary_owner_bearer_token_required_send_either_7aa2bb6abc65",
		"swagger.text.send_any_of_name_expires_or_expires_on_expiry_fi_e88d9e35db97",
		"swagger.text.invalid_linked_token_expiry_1e6b76bfcb72",
	}
	for _, command := range Commands() {
		keys = append(keys, command.Description)
		for _, argument := range command.Arguments {
			keys = append(keys, argument.Label, argument.Placeholder)
		}
	}
	unique := make(map[string]bool, len(keys))
	for _, key := range keys {
		unique[key] = true
	}
	for _, language := range i18n.DefaultSupportedLanguages() {
		data, err := i18n.LocalesFS().ReadFile("locales/" + language + ".json")
		if err != nil {
			t.Fatalf("read %s locale: %v", language, err)
		}
		var root map[string]interface{}
		if err := json.Unmarshal(data, &root); err != nil {
			t.Fatalf("parse %s locale: %v", language, err)
		}
		translations := make(map[string]string)
		flattenLocale("", root, translations)
		for key := range unique {
			if value := strings.TrimSpace(translations[key]); value == "" || value == key {
				t.Errorf("%s locale is missing a translation for %q", language, key)
			}
		}
	}
}

func TestTextUsesSearchLanguageBeforeSystemLocale(t *testing.T) {
	t.Setenv("SEARCH_LANG", "es")
	t.Setenv("LANG", "en_US.UTF-8")
	if got := Text("client.surface.title"); got != "CLI de Search" {
		t.Fatalf("Text() = %q, want Spanish translation", got)
	}
}

func TestTorCLITranslationsExistInEveryLocale(t *testing.T) {
	english, err := i18n.LocalesFS().ReadFile("locales/en.json")
	if err != nil {
		t.Fatalf("read English locale: %v", err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(english, &root); err != nil {
		t.Fatalf("parse English locale: %v", err)
	}
	englishTranslations := make(map[string]string)
	flattenLocale("", root, englishTranslations)
	torKeys := make([]string, 0, 63)
	for key := range englishTranslations {
		if strings.HasPrefix(key, "server_cli.tor.") {
			torKeys = append(torKeys, key)
		}
	}
	if len(torKeys) != 63 {
		t.Fatalf("English Tor CLI catalog has %d keys, want 63", len(torKeys))
	}
	for _, language := range i18n.DefaultSupportedLanguages() {
		data, err := i18n.LocalesFS().ReadFile("locales/" + language + ".json")
		if err != nil {
			t.Fatalf("read %s locale: %v", language, err)
		}
		var locale map[string]interface{}
		if err := json.Unmarshal(data, &locale); err != nil {
			t.Fatalf("parse %s locale: %v", language, err)
		}
		translations := make(map[string]string)
		flattenLocale("", locale, translations)
		for _, key := range torKeys {
			if value := strings.TrimSpace(translations[key]); value == "" || value == key {
				t.Errorf("%s locale is missing a translation for %q", language, key)
			}
		}
	}
}

func flattenLocale(prefix string, value interface{}, result map[string]string) {
	switch item := value.(type) {
	case map[string]interface{}:
		for key, nested := range item {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flattenLocale(name, nested, result)
		}
	case string:
		if prefix != "" {
			result[prefix] = item
		}
	}
}
