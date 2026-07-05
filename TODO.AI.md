# TODO.AI.md — Outstanding implementation items

## Completed Items

### [x] Replace forbidden scheduler library
Completed. `src/scheduler/scheduler.go` was rewritten to use `github.com/go-co-op/gocron/v2` directly.

### [x] Remove spf13/viper from codebase
Completed. Replaced with `gopkg.in/yaml.v3` directly.

### [x] Move gocron/v2 and required libs from indirect to direct in go.mod
Completed.

### [x] Raise test coverage to spec minimum of 80%
Completed. `make test` passes with **82.9%** total coverage.

### [x] Fix Makefile coverage output path
Completed. Coverage output never touches the project tree.

### [x] Well-known routes implementation
Completed. All required `/.well-known/*` routes implemented per AI.md PART 11.

### [x] Dockerfile spec compliance
Completed. Uses `casjaysdev/alpine:latest`, includes OCI labels.

---

## IMPORTANT — Pending Implementation

### [x] Database srv_ prefix for remote/libsql (PART 10)

Completed. `src/database/migrations.go` now uses `{prefix}` placeholders that resolve to `srv_` for libsql driver and empty string for local sqlite.

---

### [x] Blocklist update scheduler handler (PART 18)

Completed. `src/security/blocklist.go` implements BlocklistManager with Update(), IsBlocked(), LoadFromDisk(). Wired up to scheduler in `src/server/scheduler.go`.

---

### [x] CVE update scheduler handler (PART 18)

Completed. `src/security/cve.go` implements CVEManager with Update(), Lookup(), Search(), LoadFromDisk(). Downloads from CISA KEV catalog. Wired up to scheduler in `src/server/scheduler.go`.

---

### [x] Comprehensive audit logging (PART 10)

Completed. AuditLogger exists in `src/logging/logging.go`. Operator auth attempts (success/failure) now logged via `RequireOperator` in `src/server/auth.go`.

---

## MINOR — Lower Priority

### [x] mkdocs.yml + docs/ directory (PART 30)

Already exists. `mkdocs.yml` at project root, `docs/` directory with api.md, cli.md, configuration.md, etc.

---

### [x] Redis/Valkey cache wire-up (PART 9)

Completed. `src/server/server.go` now initializes Redis/Valkey cache when `cache.type: redis` or `cache.type: valkey` in config. Falls back to memory cache on connection failure.

---

### [x] Shell completions generation (PART 8)

Already implemented. `--shell completions [bash|zsh|fish]` prints completions script. Implementation in `src/main.go` printCompletions() function.

---

## VIOLATIONS FOUND (2024-07-04 Audit)

### [ ] Email templates missing i18n (PART 30) — HIGH

**Files affected:**
- `src/email/email.go:307` — `SendAlert()` uses hardcoded English strings
- `src/email/email.go:322` — `SendSecurityAlert()` uses hardcoded English strings
- `src/server/scheduler.go:237-256` — Task failure email body is hardcoded English

**Fix:** Import `github.com/apimgr/search/src/i18n` and use `i18n.T(lang, "email.alert_subject")` pattern for all user-facing email content. Add corresponding keys to `src/i18n/locales/en.json`.

---

### [ ] RepairDatabase path validation (PART 10) — MEDIUM

**File:** `src/service/maintenance.go:569`

**Issue:** `RepairDatabase(dbPath string)` uses `fmt.Sprintf("VACUUM INTO '%s'", cleanPath)` without validating the path. If called with malicious input, this is a potential SQL injection.

**Fix:** Add path validation using `filepath.Clean()` and ensure path is within allowed directories before using in SQL.
