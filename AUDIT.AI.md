# Project Audit

Started: 2026-09-03

Scope: full AI.md compliance audit, PART 0-32 plus FINAL COMPLIANCE CHECKLIST.
AI.md is read-only source of truth. No SPEC.md exists, so no overrides apply.

## Pass 5: Spec Compliance — PART 8 / 32 (Server CLI & Client)

- [ ] `src/main.go`: 4 `--maintenance` subcommands missing (`secret`, `token`,
      `data`, `compliance`).
- [ ] `src/main.go`: non-spec flags present where AI.md:10160 says the command
      set "CANNOT BE CHANGED" — `--init`, `--config-info`, `--test`, `--build`,
      positional `tor`/`email`, `--service enable`, `--maintenance list`,
      `--maintenance rotate-token`, `--update rollback`, `--update list`.
- [ ] `src/common/banner/banner.go:56-90`: `printFull()` hardcodes box-drawing
      characters with no `EmojiEnabled()` guard (NO_COLOR / TERM=dumb must
      disable box drawing).
- [ ] `src/main.go` `showStatus()` never queries `/server/healthz`.
- [ ] PART 32 client gaps: no TUI help screen or keyboard navigation;
      `m.sizeMode` computed but unused in `View()`; missing
      `src/client/tui/layout.go`, `src/client/tui/styles.go`,
      `src/client/cli/output.go`, `src/client/setup/wizard.go`,
      `src/client/gui/`; missing `yaml` and `csv` output formats; only 2 of
      25+ API operations reachable as subcommands; `--version` not extended.

## Pass 5: Spec Compliance — PART 17 (Email & Notifications)

- [ ] `SendSecurityAlert` has no callers although `events.security_alert`
      defaults true — wire into rate-limit trip / auth failure / blocklist hit.
- [ ] `src/email/templates.go:48`: `TemplateAdminAlert` / `admin_alert` is an
      11th template not in the spec's 10-row table, and `SendAlert` is unused.
- [ ] `src/email/email.go:198`: `Mailer.Send` is a single synchronous attempt;
      PART 17 requires a send queue with retry.
- [ ] No SMTP configuration surface in `src/api/`; PART 17 requires SMTP config
      via both the config file and the API.

## Pass 5: Spec Compliance — PART 18 (Scheduler)

- [ ] No `scheduler` subcommand (`list|show|run|enable|disable|history`,
      AI.md 28543-28552); `GetSchedulerTasks`/`RunSchedulerTask`
      (`src/server/scheduler.go:309,317`) have zero callers. No
      `server.token`-protected scheduler status endpoint.
- [ ] Stub handlers (PART 1 violation): `GeoIPUpdate` logs success without
      calling `geoip.Update`; `TokenCleanup` (`:208-211`) and `LogRotation`
      (`:214-217`) are log-and-return no-ops.

## Pass 5: Spec Compliance — PART 19 (GeoIP)

- [ ] WHOIS pseudo-database is a hard NEVER (AI.md 28628) but is implemented:
      `src/geoip/geoip.go:20,30,60-62,78-79,95,178-191,296,367,456`,
      `src/geoip/mmdb.go:233-260 LookupWHOIS`, `src/config/config.go:1085`
      (`WHOIS bool`) and `:1861` (default true), `src/server/server.go:247`.
      Removal must be atomic across all three packages.
- [ ] `src/geoip/geoip.go:19`: City DB URL uses jsDelivr, not the
      GitHub-releases source at AI.md 28621-28626; no IPv6 city DB at all.
- [ ] `geoip.presets` (`src/config/config.go:1076`) is declared but never read —
      no way to save or apply a preset (AI.md 28700, 28721-28745).
- [ ] No IP-lookup API endpoint; `geoip.Lookup` is only wired into instant
      answers (`src/api/api.go:1228`).

## Pass 5: Spec Compliance — PART 20 (Metrics)

- [ ] `src/server/server.go:1019-1026` registers one metrics path. AI.md
      29979-30000 requires `/server/metrics{,/prometheus,/grafana,/loki}`,
      `/api/{version}/server/metrics*`, `/api/metrics*`, and root `/metrics*`
      gated on `server.metrics.root.enabled`, all bound to the same handler
      (never redirects).
- [ ] `src/server/metrics.go:516-543` + `src/config/config.go:1099-1109`: one
      shared `server.metrics.token`, and an empty token leaves metrics fully
      open. AI.md 29952-29975 / 30236-30239 require per-service tokens
      (`auth.tokens.prometheus|grafana|loki`), empty token = 403, and
      `server.metrics.auth.allow_unauthenticated` (default false) as the only
      bypass.
- [ ] No Grafana dashboard handler (AI.md 30152-30227); no Loki streams handler
      or `server.metrics.loki.max_entries` / `max_age` (AI.md 30242-30243).

## Pass 5: Spec Compliance — PART 22 (Update Command)

- [ ] `src/main.go:1683-1700`: `--update branch <name>` only prints; AI.md
      30823 requires writing `update.branch` to the config file.
- [ ] `src/main.go:1579,1608`: calls `CheckForUpdates(false)`, ignoring
      `server.update.branch`, so beta/daily are never honoured
      (AI.md 30827-30833).
- [ ] `src/update/update.go:117-121`: cumulative channels and the rolling
      `daily` tag unimplemented; `parseVersion("daily")` yields `[0]` so the
      daily channel can never see an update (AI.md 31088-31106 needs a
      `buildEpoch` comparison).
- [ ] `src/server/scheduler.go:185-189`: refuses to install even when
      `auto_install: true`; AI.md 30852-30853 requires the full `--update yes`
      flow. The code comment asserts the opposite of the spec.
- [ ] `src/server/scheduler.go:182-203`: WARN log + email re-fire every run;
      AI.md 30858 requires fire-once-per-version.
- [ ] `src/update/update.go:188-217`: `findAsset` expects
      `search_{os}_{arch}.tar.gz`; AI.md 31184-31191 names raw
      `{project_name}-{GOOS}-{GOARCH}[.exe]`. Cross-check `release.yml`.
- [ ] No restart after update (`src/main.go:1643` defers to the operator);
      AI.md 30915-31000 and 31255-31335 require `restartSelf`/`restartService`
      for systemd/launchd/rc.d/SCM.

## Pass 5: Spec Compliance — PART 23/24 (Privilege & Service)

- [ ] `src/service/privilege.go:270`: escalation prompt is hardcoded English;
      PART 30 requires an i18n key. Deferred to avoid racing concurrent edits
      to `src/common/i18n/locales/` (`TestKeyConsistency` enforces parity
      across every locale, so all locale files must change together).
- [ ] `src/main.go`: nothing calls the new `service.IsWindowsService()` /
      `service.RunAsWindowsService()`, so an `sc`-installed service is still
      killed by the SCM for not reporting status.
- [ ] `src/service/service_linux.go` `Install()` dispatches runit -> OpenRC ->
      systemd only; PART 24 also specifies SysVinit (AI.md 32089-32147) and
      PART 23 lists s6. Neither generator exists.

## Pass 5: Spec Compliance — PART 28 (Testing)

- [ ] `tests/e2e.sh` missing (FINAL checklist AI.md:47628).

## NOT AUDITED — must be re-run

- [ ] PART 13 (Health & Versioning), PART 14 (API Structure), PART 15 (SSL/TLS
      & Let's Encrypt): the assigned agent terminated early on an API session
      limit before producing any findings. These three PARTs have had no
      compliance review in this run.

## Pass 2: Code Quality

- [ ] `src/binaries/` is an empty tracked directory.
- [ ] `src/i2p/` does not exist despite I2P references in
      `src/server/pages.go`, `src/server/banner.go`, `src/server/embed.go`,
      `src/email/templates.go`, `src/config/validation.go` (PART 31.2).

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
