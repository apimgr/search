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

### [ ] Database srv_ prefix for remote/libsql (PART 10)

**Spec:** Server tables must use `srv_` prefix when using libSQL/Turso remote database.

**Location:** `src/database/migrations.go`

**Task:**
- Add table prefix logic to `initServerSchema()` when driver is libsql
- Tables: `srv_scheduler_tasks`, `srv_audit_log`, `srv_config`, `srv_config_meta`, `srv_rate_limits`, `srv_backups`, `srv_api_tokens`
- Local SQLite: no prefix (current behavior)

---

### [ ] Blocklist update scheduler handler (PART 18)

**Spec:** `blocklist_update` task downloads/updates IP and domain blocklists daily at 04:00.

**Location:** `src/server/scheduler.go` line 74 (currently a stub)

**Task:**
- Create `src/security/blocklist.go` package
- Download blocklists from configured sources
- Parse and store in `{data_dir}/security/blocklists/`
- Update in-memory cache for `BlocklistMiddleware`

---

### [ ] CVE update scheduler handler (PART 18)

**Spec:** `cve_update` task downloads/updates CVE/security databases daily at 05:00.

**Location:** `src/server/scheduler.go` line 80 (currently a stub)

**Task:**
- Create `src/security/cve.go` package
- Download CVE database (NVD, GitHub Advisory, etc.)
- Store in `{data_dir}/security/cve/`

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
