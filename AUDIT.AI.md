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
- [x] src/common/i18n and src/server: `?lang=` persisted without Preferences consent and on POST; preference cookie was HttpOnly; Automatic selector state treated unsupported/empty values as pinned — consent-gated GET-only and JS-readable persistence; corrected pin detection and added regression coverage; remove after commit
- [x] tests/e2e.sh: PWA screenshot opt-in targeted an unmounted host path — mapped to the mounted scoped tempdir; documented
- [x] tests/docker.sh: runtime cleanup could fail on service-owned files — cleanup restores host ownership before removing the scoped tree
- [ ] tests/docker-image-check.sh: hosted cleanup permissions and temp path/name conventions — primary agent is fixing separately
- [ ] src/i2p/process_parent_linux_test.go: managed child did not stop after parent death in hosted CI — primary agent is fixing separately

## Pass 4: Documentation
- [x] README.md and server documentation: host-execution examples lacked the explicit host-yourself note — added
- [x] docs/configuration.md: database connection variables were mislabeled runtime and omitted CACHE_URL/secret storage/pool behavior — documented first-run precedence and connection setup; remove after commit
- [x] docs/cli.md: shell completion behavior and supported shell families were undocumented — added server/client usage and persistent shell setup; remove after commit

## Pass 5: Spec Compliance
- [x] src/client/command/shell.go and src/main.go: client completions omitted `--update`; neither binary generated usable sh/dash/ksh completions, and the server did not auto-detect those shell names — added context-sensitive update/shell candidates and POSIX candidate functions, init wrappers, and shell detection; remove after commit
- [x] src/server/middleware.go: emergency error response consulted theme/i18n and added normal chrome despite the dependency-free fallback requirement — now emits plain JSON or minimal constant-shape HTML; remove after commit
- [x] src/config and src/server: first-run DATABASE_DRIVER/DATABASE_URL/CACHE_URL were not applied and server ignored configured database driver/URL/pool/token — wired init-only settings, libSQL token and PART 10 pool; reject invalid URL schemes without SQLite fallback; remove after commit
- [x] Makefile and tests/incus.sh: random suffix generation used an early-exit pipe consumer — changed to the prescribed finite urandom pipeline
- [x] tests/docker.sh and tests/incus.sh: env overrides could supersede frozen IDEA internal identity — IDEA now takes precedence
- [x] CI container jobs: COMMIT_ID was read with git rev-parse inside casjaysdev/go despite provider SHA being available — changed to provider context SHA
- [x] CI coverage tempdirs used repository slug/project_name — changed GitHub, Gitea, Forgejo, and GitLab coverage jobs to derive project_org/internal_name from IDEA
- [ ] GUI cross-platform build failure — primary agent is fixing separately

## Pass 6: Code Flow Trace

## Completed
- tests/docker.sh, tests/incus.sh, tests/e2e.sh, Makefile: align identity resolution, temp paths, cleanup, generated container names, and screenshot output with AI.md
