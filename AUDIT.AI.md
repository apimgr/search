# Project Audit

Started: 2026-11-27

## Pass 1: Security
- [x] Branding assets: image decode allowed sources larger than the specified 16 MP ceiling — reduced the pre-decode limit to 16 MP.
- [x] Branding assets: generated variant cache reads were unbounded — bounded reads to the configured cache limit.
- [x] Footer sanitization: strict operator content accepted HTTP links and data images — restricted it to HTTPS and relative URLs.

## Pass 2: Code Quality

## Pass 3: Logic
- [x] Cookie consent: the legacy `enabled: false` setting disabled an always-required banner — render consent data unconditionally.
- [x] Cookie banner: stacking token and layout diverged from the specified order — set `--z-banner` to 250 and use it in the banner styles.

## Pass 4: Documentation
- [x] Help API docs: the endpoint contract did not describe localized Markdown or the registry page list — documented the exact response fields and fallback.
- [x] About API: built-in GeoIP attribution was conditional on the GeoIP setting despite the required always-present DB-IP notice — made it unconditional and covered disabled GeoIP.

## Pass 5: Spec Compliance
- [x] Backup manifest: PART 21 requires `files` metadata entries with Unix-second `mtime` and a single canonical `sha256`, but current generation used `file_manifest`/`modified_at` plus `checksums` and `checksum` fields — now emits the canonical schema, verifies it, and continues accepting legacy manifests.
- [x] Makefile release: the `release` target deleted and recreated stable tags despite the immutable-tag rule — reject an existing local/remote tag and remove deletion steps.
- [x] Client GUI build: the CI release workflow produced the optional GUI artifact, but Makefile builds omitted it and CLI docs said the default static CLI contained it — build the separate GUI artifact on supported platforms and correct the build/installation docs.
- [x] Tor process ownership: PART 31 forbids bine's `tor.Start` lifecycle, but service startup used it — spawn Tor directly, discover/authenticate the runtime control port, handle bootstrap events, and shut down the owned child process explicitly.
- [x] Help API/config: `/server/help` omitted the required response shape and language-map handling — added localized configuration resolution, built-in Markdown fallback, and registry-backed page titles.
- [x] MCP help page: the live tool registry rendered as a list instead of the specified Tool/Description table, and resolved alias URLs lacked copy controls — changed to the shared data table and copyable code blocks.
- [x] MCP help text: defaults and the 401 explanation claimed anonymous public tools and exposed operator-only use, contradicting the required Bearer token and operator-token prohibition — corrected the default copy and all locale strings.
- [x] CORS: native CORS allowed `X-Api-Key`, although AI.md's allowed header list excludes it and native auth ignores it — removed it and updated the test.
- [x] Footer sanitizer: strict content allowed `<h1>` despite the specified footer allow-list — disallowed the tag and added sanitizer coverage.
- [x] SMTP configuration: invalid ports were silently ignored or passed to the network layer — validate the range, warn once and restore the default port; malformed `SMTP_PORT` now warns and uses `587`.
- [x] Toast durations: the CSS progress bar stayed at fixed defaults while configured timers changed — serve config-derived CSS variables and remove the unsupported per-toast duration override.
- [x] Email customization: maintenance-recovery notification bypassed the disk-template engine — registered it as an embedded/customizable template and included it in preview validation.

## Pass 6: Code Flow Trace

## Pass 7: Additional AI.md Crosswalk Findings
- [x] Homepage fallback SVG had no explicit dimensions and could dominate `/`; bounded bare SVGs and made the configured logo responsive at the required mobile/desktop sizes.
- [x] Makefile exposed unsupported targets, stripped debug builds, and published releases; aligned it to six targets, debug builds, and local-only release artifacts, with docs updated.
- [x] Server and CLI build metadata lived outside `main` while CI linker flags targeted stale packages; restored main-package metadata and synchronized runtime consumers across CI and Docker builds.
- [ ] Component gallery linked to four contexts but rendered only one at a time; also lacked several catalog component variants and the strong-border token.
- [ ] Service selection defaulted to systemd without detection and did not follow container or no-user-manager behavior on every supported platform.
