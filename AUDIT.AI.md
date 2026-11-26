# Project Audit

Started: 2026-11-26

## Pass 1: Security
- [x] src/server/server.go: database driver errors were logged verbatim and could contain connection URL credentials — startup log now includes only the driver; remove after commit

## Pass 2: Code Quality
- [x] AGENTS.md and CLAUDE.md: contained forbidden task/progress state and exceeded the short-loader rule — fixed; remove after commit
- [ ] src/client/surface/catalog.go: unchecked RemoveAll result — primary agent is fixing separately
- [ ] src/i2p/i2p.go: unchecked Close result — primary agent is fixing separately
- [ ] src/i2p/sam.go: unchecked Remove result — primary agent is fixing separately
- [ ] src/maintenance_security.go: unchecked Close and rows.Close results — primary agent is fixing separately

## Pass 3: Logic
- [x] src/main.go, src/config/config.go, and src/mode/mode.go: explicit `--debug=false` was indistinguishable from an omitted flag, and empty `DEBUG` incorrectly disabled the `MODE=debug` default — preserve explicit false CLI overrides and treat empty DEBUG as unset; remove after commit
- [x] src/common/i18n and src/server: `?lang=` persisted without Preferences consent and on POST; preference cookie was HttpOnly; Automatic selector state treated unsupported/empty values as pinned — consent-gated GET-only and JS-readable persistence; corrected pin detection and added regression coverage; remove after commit
- [x] tests/e2e.sh: PWA screenshot opt-in targeted an unmounted host path — mapped to the mounted scoped tempdir; documented
- [x] tests/docker.sh: runtime cleanup could fail on service-owned files — cleanup restores host ownership before removing the scoped tree
- [ ] tests/docker-image-check.sh: hosted cleanup permissions and temp path/name conventions — primary agent is fixing separately
- [ ] src/i2p/process_parent_linux_test.go: managed child did not stop after parent death in hosted CI — primary agent is fixing separately

## Pass 4: Documentation
- [x] README.md and server documentation: host-execution examples lacked the explicit host-yourself note — added
- [x] docs/configuration.md: database connection variables were mislabeled runtime and omitted CACHE_URL/secret storage/pool behavior — documented first-run precedence and connection setup; remove after commit
- [x] docs/cli.md: shell completion behavior and supported shell families were undocumented — added server/client usage and persistent shell setup; remove after commit
- [x] IDEA.md: operator token storage and config-source description contradicted AI.md PART 11, 12 — corrected both descriptions to use `secrets.yml` and the three config areas; remove after commit

## Pass 5: Spec Compliance
- [x] `src/config/config.go` and `src/config/config_test.go`: config validation warnings were printed to stdout despite AI.md:10600–10608 requiring warnings on stderr — moved warnings to stderr and added stream-routing coverage; remove after commit
- [ ] src/database/database.go and database consumers: AI.md:14434 requires private read/write handles with store-owned `Read`/`Write`, but public SQL/ReadSQL/Begin exposed pooled handles and production managers depended on them — replaced handle accessors with read-only Reader and migrated consumers; primary review and validation pending
- [ ] src/common/display/detect.go, src/main.go, src/main_pgp.go, src/maintenance_email.go, src/maintenance_security.go, src/tor_cli.go, relevant tests, and src/common/i18n/locales: AI.md:10600–10608 requires per-terminal color/emoji behavior, stderr-only diagnostics, and usage errors on stderr while explicit help stays stdout — added TTY gating, main help routing, service/maintenance/shell/Tor/security usage handling, corrected specific exit codes, and moved failure details to stderr; primary review and validation pending
- [ ] src/config/config.go: AI.md:30770–30783 sets update_failed/update_staged email event defaults true, but config default omitted update_staged and set update_failed false — corrected defaults and added YAML mapping coverage; primary review and validation pending
- [x] `src/config/config.go` and `src/config/config_test.go`: invalid `server.notifications.email.smtp.tls` values (including `SMTP_TLS`) were not falling back to the configured default as required by AI.md:16978–17004 and 30221 — validation now warns and uses `auto`; focused coverage added; remove after commit
- [x] `src/server/metrics.go` and `src/server/metrics_test.go`: `search_db_write_wait_seconds` used Prometheus default buckets instead of AI.md:31641's specified latency buckets — installed the required bucket list and added exposition assertions; remove after commit
- [x] `src/server/template/page/privacy.tmpl` and `src/server/page_conformance_test.go`: the CCPA opt-out section rendered a no-sale notice when `server.privacy.data.sold` was false, contrary to AI.md:29248 and 29412–29416 — render the section only when sales are enabled and cover both config states; remove after commit
- [ ] src/scheduler/scheduler.go and src/server/scheduler.go: AI.md:30901 requires one INFO task inventory and successful runs at DEBUG, and backup_hourly disabled by default; startup omitted inventory, success logged INFO, and default persisted enabled across restart — corrected logging and disabled-state persistence/config application; primary review and validation pending
- [ ] src/server/template/page/privacy.tmpl and src/server/template/partial/cookie_consent.tmpl: AI.md:29237 and 28693–28710 require a privacy-page Manage Preferences control and shared consent dialog, but neither was rendered and app.js only searched inside a banner that disappears after consent — added shared dialog, zero-JS POST form, privacy-page opener and fallback link; remove after commit
- [ ] src/server/server.go and src/server/wellknown.go: AI.md:15455 limits `/.well-known/**` to GET/HEAD, but explicit security.txt, PGP-key, llms.txt, and change-password handlers accepted other methods — added shared method guard and regression coverage; remove after commit
- [x] src/client/command/shell.go and src/main.go: client completions omitted `--update`; neither binary generated usable sh/dash/ksh completions, and the server did not auto-detect those shell names — added context-sensitive update/shell candidates and POSIX candidate functions, init wrappers, and shell detection; remove after commit
- [x] src/server/middleware.go: emergency error response consulted theme/i18n and added normal chrome despite the dependency-free fallback requirement — now emits plain JSON or minimal constant-shape HTML; remove after commit
- [x] src/config and src/server: first-run DATABASE_DRIVER/DATABASE_URL/CACHE_URL were not applied and server ignored configured database driver/URL/pool/token — wired init-only settings, libSQL token and PART 10 pool; reject invalid URL schemes without SQLite fallback; remove after commit
- [x] Makefile and tests/incus.sh: random suffix generation used an early-exit pipe consumer — changed to the prescribed finite urandom pipeline
- [x] `.github/workflows/ci.yml`, `.gitea/workflows/ci.yml`, and `.forgejo/workflows/ci.yml`: push triggers accepted every branch despite AI.md:37090–37093 and 38153–38156 requiring `main` — restricted each push trigger to `main`; remove after commit
- [x] GitHub, Gitea, Forgejo and GitLab devel workflows: branch handling still included `master` or non-`main` branch refs despite AI.md:37650–37656, 38674–38680, and 39595–39628 — restricted devel branch rules and cancellation filters to `main`, preserving schedules and release tags; remove after commit
- [x] tests/docker.sh and tests/incus.sh: env overrides could supersede frozen IDEA internal identity — IDEA now takes precedence
- [x] CI container jobs: COMMIT_ID was read with git rev-parse inside casjaysdev/go despite provider SHA being available — changed to provider context SHA
- [x] CI coverage tempdirs used repository slug/project_name — changed GitHub, Gitea, Forgejo, and GitLab coverage jobs to derive project_org/internal_name from IDEA
- [x] `.claude/rules/binary-rules.md`, `config-rules.md`, `backend-rules.md`: stale summaries omitted current all-shell support and first-run database connection behavior — refreshed those summaries; remove after commit
- [ ] GUI cross-platform build failure — primary agent is fixing separately

## Pass 6: Code Flow Trace

## Completed
- tests/docker.sh, tests/incus.sh, tests/e2e.sh, Makefile: align identity resolution, temp paths, cleanup, generated container names, and screenshot output with AI.md
