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

### [ ] Comprehensive audit logging (PART 10)

**Spec:** `audit_log` table should capture config changes, security events, auth attempts.

**Location:** `src/database/audit.go` (needs creation) or expand `src/logging/audit.go`

**Task:**
- Create audit service that wraps DB writes
- Log: config changes, operator auth attempts, maintenance mode enter/exit
- Integrate with existing handlers

---

## MINOR — Lower Priority

### [ ] mkdocs.yml + docs/ directory (PART 30)

**Spec:** Documentation infrastructure for ReadTheDocs.

**Task:** Create `mkdocs.yml` and `docs/` directory with configuration docs.

---

### [ ] Redis/Valkey cache wire-up (PART 9)

**Status:** Code exists in `src/cache/redis.go` but not wired up in server init.

**Task:** Enable Redis cache backend when `cache.redis.enabled: true` in config.

---

### [ ] Shell completions generation (PART 8)

**Task:** Verify `--shell completions` generates bash/zsh/fish completions to `completions/` directory.
