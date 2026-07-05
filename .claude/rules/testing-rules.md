# Testing Rules (PART 28, 29, 30)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Write coverage output to project tree (use /tmp/apimgr/search-XXXXXX/)
- Skip tests before commit (make test must pass)
- Hardcode English strings in tests (use i18n key lookups or constants)
- Test implementation details (test behavior, not internals)
- Leave TODO/FIXME/stub functions in committed test code

## CRITICAL - ALWAYS DO

- Coverage target: ≥80% across all packages
- Table-driven tests (Go idiomatic)
- Test all error paths, not just happy path
- Coverage output to /tmp/apimgr/search-XXXXXX/coverage.out
- i18n: all user-facing text through i18n key lookups (t(r, "key"))
- Accessibility: WCAG AA minimum on all web UI

## Coverage Requirements

| Package | Minimum Coverage |
|---------|-----------------|
| All packages | ≥80% |
| Currently below target | src/main, src/database, src/instant, src/server, src/direct, src/geoip |

## Test Patterns

```go
// Table-driven test (required pattern)
func TestSearchQuery(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"empty query", "", "", true},
        {"valid query", "golang", "golang", false},
        {"xss attempt", "<script>", "", true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseSearchQuery(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ParseSearchQuery() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if got != tt.want {
                t.Errorf("ParseSearchQuery() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Coverage Command (In Docker)

```bash
mkdir -p "/tmp/apimgr"
COVDIR=$(mktemp -d "/tmp/apimgr/search-XXXXXX")
docker run --rm \
    -v "$PWD:/workspace" \
    -w /workspace \
    -e CGO_ENABLED=0 \
    casjaysdev/go:latest \
    sh -c "go test -coverprofile=$COVDIR/coverage.out ./... && go tool cover -func=$COVDIR/coverage.out"
```

## i18n Rules (PART 30)

- All user-facing text via i18n key lookups — NO hardcoded English
- Pattern: t(r, "category.key") for HTTP handlers, i18n.T(lang, "key") for other code
- Key format: category.action_noun (search.results_title, errors.not_found)
- Translation files: src/common/i18n/locales/{lang}.json
- Default language: en (English)
- Fallback: if key missing in lang → fall back to "en"

```go
// WRONG
w.Write([]byte("Search results for " + query))

// CORRECT
msg := t(r, "search.results_for", query)
w.Write([]byte(msg))
```

## Accessibility Rules (PART 30)

- WCAG AA minimum compliance
- All images: alt text required
- All forms: labels associated with inputs
- Color contrast: 4.5:1 minimum for normal text
- Keyboard navigation: all interactive elements reachable
- Screen reader: semantic HTML5 (nav, main, section, article)
- Focus management: visible focus indicator
- ARIA: only when semantic HTML is insufficient

## ReadTheDocs (PART 29)

- mkdocs.yml must exist at project root
- docs/ directory for documentation source
- All public API endpoints documented
- Configuration options documented in docs/configuration.md

For complete details, see AI.md PART 28, 29, 30
