package i18n

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// usedKeyPattern matches a translation key at a call site in a Go template
// ({{t "some.key"}}) or in client JavaScript (t('some.key') / t("some.key")).
// It deliberately requires the quotes, so prose in the surrounding markup is
// not mistaken for a key.
var usedKeyPattern = regexp.MustCompile(`(?:\bt\(|\{\{t\s+)"([a-zA-Z0-9_]+(?:\.[a-zA-Z0-9_]+)+)"`)

// TestTemplateTranslationKeysExist is the guard against the widgets_ui
// regression: a translation key referenced from a Go template or client
// JavaScript file but absent from en.json. The failure mode is silent and
// visible only in the browser — the JS t() calls have inline English
// fallbacks, and a Go template renders the raw dotted key because Go has no
// such fallback.
//
// TestKeyConsistency does not catch this: it compares en.json against the
// other locale files, so a key that is missing everywhere is consistent and
// passes. This walks the actual call sites instead.
func TestTemplateTranslationKeysExist(t *testing.T) {
	enData, err := fs.ReadFile(LocalesFS(), "locales/en.json")
	if err != nil {
		t.Fatalf("cannot read en.json: %v", err)
	}
	var enRaw interface{}
	if err := json.Unmarshal(enData, &enRaw); err != nil {
		t.Fatalf("cannot parse en.json: %v", err)
	}
	known := make(map[string]string)
	flattenTranslations("", enRaw, known)

	// Roots are relative to src/, resolved from the package directory
	// (src/common/i18n) so the test does not depend on the working directory.
	const (
		templateRoot = "../../server/template"
		staticRoot   = "../../server/static"
	)

	// Maps a key to the call sites that reference it, for the error message.
	used := make(map[string][]string)

	collect := func(root, exts string) {
		if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// A missing root is not this test's failure to report.
				return nil
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), "node_modules") || d.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, exts) {
				return nil
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(filepath.Join("..", ".."), path)
			for _, m := range usedKeyPattern.FindAllStringSubmatch(string(b), -1) {
				used[m[1]] = append(used[m[1]], filepath.ToSlash(rel))
			}
			return nil
		}); err != nil {
			t.Errorf("walk translation call-site root %s: %v", root, err)
		}
	}
	collect(templateRoot, ".tmpl")
	collect(staticRoot, ".js")

	if len(used) == 0 {
		t.Fatal("no translation call sites were found; the scan patterns are stale")
	}

	keys := make([]string, 0, len(used))
	for k := range used {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var missing []string
	for _, k := range keys {
		if _, ok := known[k]; !ok {
			missing = append(missing, k+" (used in "+strings.Join(used[k], ", ")+")")
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d translation key(s) are referenced but absent from en.json:\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func TestOwnerTokenTranslationsExistInEveryLocale(t *testing.T) {
	keys := []string{
		"preferences.owner_tokens.heading",
		"preferences.owner_tokens.primary_token",
		"preferences.owner_tokens.linked_tokens",
		"preferences.owner_tokens.reveal_notice",
		"preferences.owner_tokens.reveal_primary",
		"preferences.owner_tokens.rotate_primary",
		"preferences.owner_tokens.rotate_primary_confirm",
		"preferences.owner_tokens.rotate_linked_confirm",
		"preferences.owner_tokens.create_linked",
		"preferences.owner_tokens.restore_description",
		"preferences.owner_tokens.restore_existing",
		"preferences.owner_tokens.restore_owner",
		"preferences.owner_tokens.revoke_linked_confirm",
		"preferences.owner_tokens.primary_revealed",
		"preferences.owner_tokens.primary_created",
		"preferences.owner_tokens.primary_rotated",
		"preferences.owner_tokens.linked_created",
		"preferences.owner_tokens.linked_revealed",
		"preferences.owner_tokens.linked_rotated",
		"preferences.owner_tokens.expiry_7d",
		"preferences.owner_tokens.expiry_30d",
		"preferences.owner_tokens.expiry_90d",
		"preferences.owner_tokens.expiry_1y",
	}
	locales, err := fs.ReadDir(LocalesFS(), "locales")
	if err != nil {
		t.Fatalf("cannot read locales: %v", err)
	}
	for _, locale := range locales {
		if locale.IsDir() || !strings.HasSuffix(locale.Name(), ".json") {
			continue
		}
		data, err := fs.ReadFile(LocalesFS(), "locales/"+locale.Name())
		if err != nil {
			t.Errorf("%s: cannot read locale: %v", locale.Name(), err)
			continue
		}
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Errorf("%s: cannot parse locale: %v", locale.Name(), err)
			continue
		}
		translations := make(map[string]string)
		flattenTranslations("", raw, translations)
		for _, key := range keys {
			value, ok := translations[key]
			if !ok {
				t.Errorf("%s: missing owner-token translation %q", locale.Name(), key)
			} else if locale.Name() != "en.json" && value == key {
				t.Errorf("%s: owner-token translation %q is untranslated", locale.Name(), key)
			}
		}
	}
}

func TestServerTorCLITranslationsExistInEveryLocale(t *testing.T) {
	keys := []string{
		"server_cli.tor.address", "server_cli.tor.address_unknown", "server_cli.tor.applied_vanity",
		"server_cli.tor.bootstrap", "server_cli.tor.check_progress", "server_cli.tor.circuit_established",
		"server_cli.tor.config_invalid", "server_cli.tor.config_missing", "server_cli.tor.config_path",
		"server_cli.tor.config_valid", "server_cli.tor.confirm_apply_vanity", "server_cli.tor.confirm_import",
		"server_cli.tor.confirm_regenerate", "server_cli.tor.error", "server_cli.tor.help_import_keys",
		"server_cli.tor.help_regenerate", "server_cli.tor.help_restart", "server_cli.tor.help_status",
		"server_cli.tor.help_subcommands", "server_cli.tor.help_usage", "server_cli.tor.help_validate",
		"server_cli.tor.help_vanity_apply", "server_cli.tor.help_vanity_start", "server_cli.tor.help_vanity_stop",
		"server_cli.tor.hostname_missing", "server_cli.tor.imported_address", "server_cli.tor.issue",
		"server_cli.tor.new_address", "server_cli.tor.no_running_server", "server_cli.tor.offline_heading",
		"server_cli.tor.path_absolute", "server_cli.tor.pid", "server_cli.tor.prefix_duplicate",
		"server_cli.tor.prefix_required", "server_cli.tor.prefix_value_required", "server_cli.tor.restarted",
		"server_cli.tor.status_connected", "server_cli.tor.status_disconnected", "server_cli.tor.status_unknown",
		"server_cli.tor.torrc_missing", "server_cli.tor.unknown_option", "server_cli.tor.unknown_subcommand",
		"server_cli.tor.usage_import_keys", "server_cli.tor.usage_vanity", "server_cli.tor.usage_vanity_stop",
		"server_cli.tor.vanity_candidates", "server_cli.tor.vanity_not_running", "server_cli.tor.vanity_started",
		"server_cli.tor.vanity_state", "server_cli.tor.vanity_stopped", "server_cli.tor.version",
		"server_cli.tor.workers_duplicate", "server_cli.tor.workers_integer", "server_cli.tor.workers_minimum",
		"server_cli.tor.workers_value_required", "server_cli.tor.vanity_prefix", "server_cli.tor.vanity_workers",
		"server_cli.tor.vanity_attempts", "server_cli.tor.vanity_rate", "server_cli.tor.vanity_elapsed_seconds",
		"server_cli.tor.vanity_state_idle", "server_cli.tor.vanity_state_running", "server_cli.tor.vanity_state_found",
	}
	if len(keys) != 63 {
		t.Fatalf("Tor CLI translation inventory has %d keys, want 63", len(keys))
	}
	locales, err := fs.ReadDir(LocalesFS(), "locales")
	if err != nil {
		t.Fatalf("cannot read locales: %v", err)
	}
	for _, locale := range locales {
		if locale.IsDir() || !strings.HasSuffix(locale.Name(), ".json") {
			continue
		}
		data, err := fs.ReadFile(LocalesFS(), "locales/"+locale.Name())
		if err != nil {
			t.Errorf("%s: cannot read locale: %v", locale.Name(), err)
			continue
		}
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Errorf("%s: cannot parse locale: %v", locale.Name(), err)
			continue
		}
		translations := make(map[string]string)
		flattenTranslations("", raw, translations)
		for _, key := range keys {
			value, ok := translations[key]
			if !ok {
				t.Errorf("%s: missing Tor CLI translation %q", locale.Name(), key)
				continue
			}
			if locale.Name() != "en.json" && value == key {
				t.Errorf("%s: Tor CLI translation %q is an untranslated key", locale.Name(), key)
			}
		}
	}
}

func TestServerCLICatalogCallsitesExistInEveryLocale(t *testing.T) {
	pattern := regexp.MustCompile(`"(server_cli\.(?:tor|pgp|maintenance|status|help|version|main)\.[a-zA-Z0-9_]+)"`)
	keys := make(map[string]struct{})
	for _, path := range []string{"../../tor_cli.go", "../../main_pgp.go", "../../maintenance_cli.go", "../../maintenance_email.go", "../../maintenance_scheduler.go", "../../main.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("cannot read server CLI source %s: %v", path, err)
		}
		for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
			keys[match[1]] = struct{}{}
		}
	}
	delete(keys, "server_cli.tor.vanity_")
	delete(keys, "server_cli.tor.vanity_state_")
	delete(keys, "server_cli.maintenance.token_status_")
	delete(keys, "server_cli.maintenance.scheduler_status_")
	for _, state := range []string{"active", "revoked", "expired"} {
		keys["server_cli.maintenance.token_status_"+state] = struct{}{}
	}
	for _, state := range []string{"unknown", "success", "failed", "skipped", "running", "retrying"} {
		keys["server_cli.maintenance.scheduler_status_"+state] = struct{}{}
	}
	if len(keys) == 0 {
		t.Fatal("no server CLI translation keys were found; the source scan is stale")
	}
	locales, err := fs.ReadDir(LocalesFS(), "locales")
	if err != nil {
		t.Fatalf("cannot read locales: %v", err)
	}
	for _, locale := range locales {
		if locale.IsDir() || !strings.HasSuffix(locale.Name(), ".json") {
			continue
		}
		data, err := fs.ReadFile(LocalesFS(), "locales/"+locale.Name())
		if err != nil {
			t.Errorf("%s: cannot read locale: %v", locale.Name(), err)
			continue
		}
		var raw interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Errorf("%s: cannot parse locale: %v", locale.Name(), err)
			continue
		}
		translations := make(map[string]string)
		flattenTranslations("", raw, translations)
		for key := range keys {
			value, ok := translations[key]
			if !ok {
				t.Errorf("%s: missing server CLI translation %q", locale.Name(), key)
				continue
			}
			if locale.Name() != "en.json" && value == key {
				t.Errorf("%s: server CLI translation %q is an untranslated key", locale.Name(), key)
			}
		}
	}
}
