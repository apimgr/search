# Project Audit

Started: 2026-09-03

Scope: full AI.md compliance audit, PART 0-32 plus FINAL COMPLIANCE CHECKLIST.
AI.md is read-only source of truth. No SPEC.md exists, so no overrides apply.

## Pass 5: Spec Compliance — PART 31 (Tor & I2P) — verified this session

- [x] `src/service/tor.go`: `CheckTorConnection()` was a hardwired
      `return true` stub (PART 1 forbids stubs; zero callers) — removed along
      with its two always-pass tests in `src/service/tor_test.go`.

## Pass 5: Spec Compliance — PART 8 / 32 (Server CLI & Client)

- [x] Maintenance subcommands `secret`, `token`, `data`, and `compliance` are
      implemented and dispatched from `src/main.go`.
- [x] Removed unsupported update `rollback` CLI handling and its completion
      entry. No `--init`, `--config-info`, `--test`, `--build`, service `enable`,
      maintenance `list`, `rotate-token`, or update `list` command exists.
      `tor` and `email test` are defined separately in PART 31 and PART 17;
      `--service status` is explicitly listed in the escalation table at AI.md
      8144.
- [x] `src/common/banner/banner.go` uses `terminal.GetSymbols()` for box
      characters; NO_COLOR does not disable box drawing per AI.md PART 7.
- [x] `showStatus()` calls `queryInternalHealth()` over loopback and exits
      according to the health response.
- [x] PART 32 client implementation includes responsive TUI layout/styles,
      keyboard help/navigation, YAML/CSV output, setup wizard, GUI, API
      subcommands, and extended version output.

## Pass 5: Spec Compliance — PART 17 (Email & Notifications)

- [x] Security alerts are wired from rate-limit and IP/domain-blocklist events
      through `email.NotifySecurityAlert`, matching the PART 17 event list.
- [x] `src/email/templates.go` exposes exactly the ten templates in the PART 17
      table; the former extra admin alert template is gone.
- [x] `Mailer.Send` uses the bounded retry queue when SMTP is configured;
      `Server` starts and drains that queue as part of its lifecycle.
- [x] The generic final checklist asks for SMTP mutation through an API, but
      AI.md PART 1 explicitly forbids runtime API configuration changes and
      the project IDEA states configuration is file-only. The API exposes only
      a redacted GET view, as required by the more specific rule; no SMTP write
      endpoint is permitted.

## Pass 5: Spec Compliance — PART 18 (Scheduler)

- [x] Scheduler tasks are exposed through operator-gated API endpoints and
      `search-cli scheduler` now implements list/show/run/enable/disable/history.
- [x] `GeoIPUpdate` calls the GeoIP updater. `TokenCleanup` now removes expired
      server API tokens; `LogRotation` now invokes the built-in log manager and
      returns failures so the scheduler records them.

## Pass 5: Spec Compliance — PART 19 (GeoIP)

- [x] No WHOIS pseudo-database or registrant lookup exists. City IPv4 and IPv6
      use the specified GitHub release assets; presets and the GeoIP lookup API
      are implemented.

## Pass 5: Spec Compliance — PART 20 (Metrics)

- [x] Metrics aliases, per-service bearer authentication, Grafana dashboard,
      Loki streams, and bounded Loki retention are implemented in
      `src/server/metrics_routes.go`, `metrics_grafana.go`, and `metrics_loki.go`.

## Pass 5: Spec Compliance — PART 22 (Update Command)

- [x] `--update branch <name>` writes `server.update.branch` to the config file.
- [x] Stable/beta/daily channels are cumulative; the rolling daily tag is
      compared by `buildEpoch`.
- [x] Scheduled auto-install uses the normal update installation flow and
      records one notification per version.
- [x] Release asset selection uses the canonical raw binary names.
- [x] Update installation restarts the running service or re-executes the
      unsupervised process according to the service manager.
- [x] Removed the CLI rollback subcommand because it is absent from PART 22's
      complete update command table.

## Pass 5: Spec Compliance — PART 23/24 (Privilege & Service)

- [x] Privilege escalation prompts use the `service.escalation_prompt` i18n
      key, with locale parity enforced by `TestKeyConsistency`.
- [x] `main.go` dispatches Windows SCM processes through
      `service.IsWindowsService()` and `service.RunAsWindowsService()`.
- [x] Linux service detection and installation cover runit, OpenRC, systemd,
      s6, and SysVinit.

## Pass 5: Spec Compliance — PART 28 (Testing)

- [x] `tests/e2e/` has no deterministic SSR/no-JS/full-browser scenario set for the direct DNS and WHOIS answers.
- [x] `tests/e2e/` has no deterministic SSR/no-JS/full-browser scenario set for hostname resolution and ASN lookups.
- [x] `tests/e2e/` has no deterministic SSR/no-JS/full-browser scenario set for certificate and HTTP-header inspection.
- [x] `tests/e2e/` has no deterministic SSR/no-JS/full-browser scenario set for directory discovery and archive/cache views.
- [x] `tests/e2e/` has no deterministic SSR/no-JS/full-browser scenario set for robots.txt and sitemap parsing.
- [x] `tests/e2e/` has no deterministic SSR/no-JS/full-browser scenario set for technology/feed detection.
- [x] `tests/e2e/` has no deterministic SSR/no-JS/full-browser scenario set for URL expansion and safety checks.


- E2E network-backed direct-answer coverage completed: DNS, WHOIS, hostname
  resolution, ASN, certificates, HTTP headers, directory discovery, archive
  cache, robots, sitemap, technology, feeds, URL expansion, and safety checks
  now use local deterministic fixtures in the SSR/no-JS/full-browser matrix.
  External network access remains blocked by the E2E network topology.


- Re-audited 2026-10-12: PART 13 traffic statistics now use a bounded rolling
  second-level counter; metrics middleware is mounted in Chi before routes,
  uses matched route templates to avoid token/path values in labels, and feeds
  both HTML and API health. API health now shares component status/checks from
  the running server. Focused server/API tests pass.

## NOT AUDITED — must be re-run

- [ ] Complete PART 13 health/versioning, PART 14 API/schema parity, and
      PART 15 TLS/ACME audits. The 2026-10-12 pass fixed and tested inaccurate
      traffic statistics and shared health checks/status; GraphQL/OpenAPI parity
      and the full certificate/challenge matrix remain to be reviewed.

## Pass 2: Code Quality

- [x] `src/binaries/` is not tracked; AI.md requires the root `binaries/` build-output directory, which is gitignored.
- [x] `src/i2p/` exists with its manager, backend, SAM, destination, config, and tests.

## Spec Conflicts — RESOLVED (operator ruling, applied)

- [x] PART 25 vs FINAL checklist — RULING: PART 25 (AI.md 32295-32309) wins.
      The Makefile has EXACTLY six targets: `dev`, `local`, `build`, `test`,
      `release`, `docker`. `all`, `lint`, `clean`, `build-arm64`,
      `docker-build` and `i18n-validate` were removed. FINAL-checklist lines
      AI.md:47593-47594 (`make clean`, `make all`) are superseded by this
      ruling and are NOT findings.
- [x] i18n validator location — RULING: no root `cmd/` directory is ever
      created (PART 3, AI.md:47348 wins over AI.md:41060). Validation lives in
      `src/common/i18n` as `TestKeyConsistency` / `TestLocalesFS` and runs as a
      normal part of `make test`. No `cmd/i18n-validate` path or
      `i18n-validate` target remains anywhere in the tree.

## Verified compliant — no change required

- `src/path/` (singular) is correct despite the FINAL checklist writing
  `src/paths/paths.go` at AI.md:47340. PART 3's normative body mandates
  singular Go package directories (AI.md:960, 1147, 1158); the plural spelling
  appears only in the checklist, never in the normative text.

## Completed

### PART 7/8/32
- src/client/api/client.go:821: added `acceptLanguage()` resolving
  SEARCH_LANG -> LC_ALL -> LANG, normalizing `en_US.UTF-8` -> `en-US`.
- src/client/cmd/shell.go:226-234: `--shell init` emits
  `{bin} --shell completions X` per AI.md 45865-45873, all 8 shells.
- src/client/cmd/shell.go:59-63: help lists all 8 shells (AI.md 45744-45755).
- src/main.go:311: `--maintenance` with no subcommand lists the implemented set.
- src/main.go:93,559,653: `--mode` corrected to
  `{production|development|debug}` (AI.md:10174).
- src/main.go:839: `--status` exits 1 when the server is not running
  (AI.md:10193).

### PART 23/24/25/26
- src/service/service_linux.go: `ensureSystemUser()` now delegates to
  `createLinuxSystemUser()` applying matching-UID==GID, 200-899, reserved-skip
  (AI.md 31599-31617).
- src/service/privilege.go: `reservedSystemIDs` gained the missing 980-992
  block; 993 label corrected (AI.md 31635-31647).
- src/service/privilege_linux.go:64: hardcoded `/etc/apimgr/` ->
  `"/etc/" + config.InternalOrg + "/"` (AI.md 31602).
- src/service/service_windows.go: `sc create` gained
  `obj= NT SERVICE\<internal_name>` (AI.md 31880-31895); implemented
  `windowsService.Execute`, `IsWindowsService()`, `RunAsWindowsService()`
  (AI.md 32261-32289).
- Makefile: added DOCKER_MEM/DOCKER_CPUS and --memory/--cpus (AI.md
  32523-32538); `test:` uses `$${TMPDIR:-/tmp}`; `release:` emits
  sha256.txt/sha512.txt (AI.md 32638, 33019-33020); removed the prohibited
  `install:` copy to /usr/local/bin (AI.md 32950).
- docker/Dockerfile: runtime stage gained `ARG BUILD_EPOCH` (AI.md 33363).
- docker/Dockerfile.dev: BuildDate ldflag -> BUILD_EPOCH (PART 25 32747-32748).
- docker/docker-compose.dev.yml / .test.yml: `DEBUG: 1` -> `true`,
  `MODE: dev` -> `development` (AI.md 33708-33709, 33940-33941);
  `:latest` -> `:devel` (AI.md 33557, 33932).
- docker/rootfs/usr/local/bin/entrypoint.sh: `set -e` -> `set -eo pipefail`;
  log timestamps now RFC-3339 (AI.md 33447, 33466).
- scripts/release.sh: rewritten as a spec-correct delegator (validate semver ->
  write release.txt -> make test -> make release); the old version walked
  platform subdirs `make build` never creates.
- scripts/build.sh, scripts/test.sh: `set -e` -> `set -eo pipefail`.

### PART 17-22
- src/update/update.go:147: checksum-asset lookup now tries canonical
  `sha256.txt` first (AI.md 31196-31204).
- src/update/update.go:271: `VerifyChecksum` no longer returns nil when no
  checksum asset exists — refuses to install an unverified binary
  (AI.md 31150). Test at src/update/update_network_test.go:506 inverted.
- src/geoip/geoip.go:312: added `isPrivateIP()` guard so RFC1918/RFC4193/
  loopback addresses are not geo-located or country-blocked (AI.md 28719).
- src/backup/encryption.go:24: comment cited PART 24 -> PART 21.

### Pass 0 build blocker (PART 2)
- go.mod / go.sum: `filippo.io/edwards25519 v1.1.0` and
  `github.com/pires/go-proxyproto v0.15.0` present and consistent;
  `go mod tidy && go build ./...` clean in `casjaysdev/go:latest`.
- LICENSE.md:29,44: both new direct deps attributed — edwards25519 is
  BSD-3-Clause, go-proxyproto is Apache-2.0 (verified from the module cache
  LICENSE files); neither is GPL/LGPL/AGPL.
- LICENSE.md:51-55: the 5 stale golang.org/x versions now match go.mod
  (crypto v0.55.0, net v0.58.0, sys v0.47.0, term v0.45.0, text v0.41.0).

### PART 0 (AI assistant configuration)
- `.claude/rules/*.md`: all 13 files carry the NON-NEGOTIABLE banner,
  `## CRITICAL - NEVER DO` / `## CRITICAL - ALWAYS DO`, and the trailing
  `For complete details, see AI.md PART …` line (AI.md 47157-47166).
- CLAUDE.md: expanded from 3 lines to the full ~105-line loader specified at
  AI.md 1531-1583.
- Generated the AI-tool config set required by AI.md 1450-1516:
  `.cursor/rules/*.mdc`, `.windsurf/rules/*.mdc`, `.ai/rules/*.md` (13 each),
  `.cursor/CURSOR.md`, `.windsurf/WINDSURF.md`, `.aider/AIDER.md`,
  `.ai/AI.md`, and `.aider/CONVENTIONS.md`.

### PART 25 six-target ruling sweep
- Makefile: reduced to exactly `dev`, `local`, `build`, `test`, `release`,
  `docker`; `clean` inlined as `@rm -rf $(BINDIR) $(RELDIR)` in `local:` and
  `build:` instead of a prerequisite target.
- scripts/build.sh: dropped `BUILD_ARM64` and the `make build-arm64` call —
  `make build` already covers linux/arm64 as one of the 8 platforms.
- docs/development.md:134-161: `make lint` / `make test-integration` /
  `make coverage` replaced with a Dockerized `go vet && staticcheck` command
  and `make test` as the single test entry point.
- .claude/rules/makefile-rules.md: rewritten around the six-target rule.

### PART 26/27 (Docker & CI/CD)
- `.github/workflows/beta.yml`, `.gitea/workflows/beta.yml`,
  `.forgejo/workflows/beta.yml`: did not exist -> created (PART 27, FINAL
  checklist AI.md:47614). Full 8-platform server+CLI matrix on
  `casjaysdev/go:latest`, version from `release.txt` else
  `$(date -u +%Y%m%d%H%M%S)-beta`, `sha256.txt`/`sha512.txt`, SBOM, prerelease
  on tag `beta`. GitHub adds build-provenance attestation; Gitea/Forgejo
  publish via `tea release create --prerelease`.
- `.forgejo/workflows/security.yml`: deleted — its four jobs belong in
  `ci.yml` per PART 27, and the separate file broke provider parity and
  double-ran vuln-scan.
- `.forgejo/workflows/ci.yml`, `.gitea/workflows/ci.yml`: regenerated from
  `.github/workflows/ci.yml`; Forgejo was missing `secret-scan`,
  `workflow-policy`, `image-scan` and the weekly `schedule` trigger.
- `.forgejo/workflows/{daily,release,docker}.yml`: used `${{ gitea.* }}` and
  `secrets.GITEA_TOKEN` inside the Forgejo tree -> `${{ forgejo.* }}` /
  `FORGEJO_TOKEN`.
- `.github|.gitea|.forgejo/workflows/docker.yml`: `:devel` never rebuilt
  without a push -> added `schedule: '0 4 * * *'` and split out a dedicated
  `build-devel` job per the PART 27 equivalence table. Gitea/Forgejo copies
  emitted a non-ISO local-time `BUILD_DATE` and never passed `BUILD_EPOCH`
  despite `docker/Dockerfile` declaring `ARG BUILD_EPOCH` -> both now correct.
- Verified compliant, no change: all 12 distinct Actions across all three
  providers pinned to full 40-char SHAs; zero `make` invocations in any
  workflow or the Jenkinsfile; release asset names are
  `search-{goos}-{goarch}[.exe]`, matching `getBinaryName()`
  (AI.md 31184-31191); all PART 26 Docker and compose files match the spec.
- Validation: `act --list -W <file>` passes for all five
  `.github/workflows/*.yml`; the 10 Gitea/Forgejo files were validated via
  context-substituted copies (act's schema is GitHub-only and rejects the
  `gitea`/`forgejo` expression contexts outright).
- Spec self-contradiction resolved: AI.md 37304-37305 (Jenkins section) says
  `MODE=devel` is baked into `Dockerfile.dev` via ENV; PART 26 (~AI.md 33166)
  says `ENV MODE` is not set. PART 26 is the authoritative Docker part and
  wins — `docker/Dockerfile.dev` sets no `MODE` ENV.

### PART 29 (documentation)
- README.md:6: added the required ReadTheDocs documentation badge
  (AI.md 4791, FINAL checklist AI.md:47652), linked per AI.md:4756.
