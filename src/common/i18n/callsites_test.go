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
				if strings.HasPrefix(d.Name(), "node_modules") {
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
