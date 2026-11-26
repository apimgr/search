package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/apimgr/search/src/common/i18n"
)

func TestTorConfirmationAccepted(t *testing.T) {
	for _, input := range []string{"yes", " YES ", "Yes"} {
		if !torConfirmationAccepted(input) {
			t.Errorf("torConfirmationAccepted(%q) = false, want true", input)
		}
	}
	for _, input := range []string{"", "y", "no"} {
		if torConfirmationAccepted(input) {
			t.Errorf("torConfirmationAccepted(%q) = true, want false", input)
		}
	}
}

func TestServerCLILanguagePrecedence(t *testing.T) {
	oldFlagLang := flagLang
	t.Cleanup(func() { flagLang = oldFlagLang })
	configDir := t.TempDir()
	t.Setenv("CONFIG_DIR", configDir)
	t.Setenv("SEARCH_LANG", "fr_FR.UTF-8")
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	t.Setenv("LANG", "es_ES.UTF-8")
	flagLang = "ar"
	if got := serverCLILanguage(); got != "ar" {
		t.Fatalf("explicit language = %q, want ar", got)
	}
	flagLang = ""
	if got := serverCLILanguage(); got != "fr" {
		t.Fatalf("environment language = %q, want fr", got)
	}
	if err := os.WriteFile(filepath.Join(configDir, "server.yml"), []byte("server:\n  i18n:\n    default_language: it\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := serverCLILanguage(); got != "it" {
		t.Fatalf("configured language = %q, want it", got)
	}
	flagLang = "unsupported"
	if got := serverCLILanguage(); got != "en" {
		t.Fatalf("unsupported explicit language = %q, want en", got)
	}
}

func TestServerCLIOutputUsesSelectedLocale(t *testing.T) {
	oldFlagLang := flagLang
	t.Cleanup(func() { flagLang = oldFlagLang })
	flagLang = "es"

	got := serverCLIText("server_cli.tor.help_usage")
	want := i18n.T("es", "server_cli.tor.help_usage")
	if got != want {
		t.Fatalf("server CLI output = %q, want selected-locale translation %q", got, want)
	}
	if got == i18n.T("en", "server_cli.tor.help_usage") || strings.Contains(got, "server_cli.tor.help_usage") {
		t.Fatalf("server CLI output did not use a translated Spanish value: %q", got)
	}
}

func TestParseTorVanityStartArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantPrefix string
		wantWorker int
		wantErr    bool
	}{
		{name: "positional prefix", args: []string{"abc"}, wantPrefix: "abc"},
		{name: "prefix and workers", args: []string{"--prefix", "abc", "--workers", "3"}, wantPrefix: "abc", wantWorker: 3},
		{name: "workers before prefix", args: []string{"--workers=2", "abc"}, wantPrefix: "abc", wantWorker: 2},
		{name: "missing prefix", args: []string{}, wantErr: true},
		{name: "invalid workers", args: []string{"abc", "--workers", "many"}, wantErr: true},
		{name: "zero workers", args: []string{"abc", "--workers", "0"}, wantErr: true},
		{name: "duplicate workers", args: []string{"abc", "--workers", "2", "--workers=3"}, wantErr: true},
		{name: "duplicate prefix", args: []string{"abc", "--prefix", "def"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, workers, err := parseTorVanityStartArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseTorVanityStartArgs(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
			}
			if err == nil && (prefix != tt.wantPrefix || workers != tt.wantWorker) {
				t.Errorf("parseTorVanityStartArgs(%v) = (%q, %d), want (%q, %d)", tt.args, prefix, workers, tt.wantPrefix, tt.wantWorker)
			}
		})
	}
}

func TestPrintTorStatusJSONIncludesContractFields(t *testing.T) {
	previousLanguage := flagLang
	flagLang = "en"
	t.Cleanup(func() { flagLang = previousLanguage })
	out := captureStdout(t, func() {
		printTorStatusJSON(map[string]any{
			"data": map[string]any{
				"running":             true,
				"pid":                 1234,
				"version":             "0.4.8.12",
				"onion_address":       "example.onion",
				"bootstrap_percent":   87,
				"circuit_established": true,
				"vanity": map[string]any{
					"state":           "running",
					"prefix":          "abc",
					"workers":         2,
					"attempts":        99,
					"rate":            12.5,
					"elapsed_seconds": 8.0,
					"candidates":      []any{"candidate.onion"},
				},
			},
		})
	})
	for _, want := range []string{"Connected", "1234", "example.onion", "87", "Yes", "0.4.8.12", "abc", "99", "candidate.onion"} {
		if !strings.Contains(out, want) {
			t.Errorf("Tor status output is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Disabled") {
		t.Fatalf("Tor status inferred disabled from an absent field:\n%s", out)
	}
}
