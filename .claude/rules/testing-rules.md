# Testing Rules (PART 28, 29, 30)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER run `go build`/`go test`/any binary directly on the local machine — local has no Go; build and run only inside Docker (`casjaysdev/go:latest`) or Incus
- NEVER use the project directory for runtime/test data — ALL temp/test data goes to `/tmp/{project_org}/{internal_name}-XXXXXX/`
- NEVER use bare `mktemp -d`, `/tmp/` root, or a generic path without the `{project_org}/{internal_name}-XXXXXX` structure
- NEVER use `docker/docker-compose.yml` or `docker/docker-compose.dev.yml` — AI may only use `docker/docker-compose.test.yml`, and only via `tests/` scripts (direct temp-dir copy is a fallback)
- NEVER commit config files (`server.yml`, `cli.yml`) — they are runtime-generated only, never in the repo
- NEVER skip `*_test.go` in favor of only `./tests/*.sh`, or vice versa — both phases are required and measure different things
- NEVER use `pkill -f`, `pkill {name}` without `-x`, `killall`, `kill -9` as first resort, or any Docker/rm wildcard prune (`docker system prune`, `rm -rf /`, `rm -rf *`, etc.)
- NEVER put non-ReadTheDocs files in `docs/` — it is exclusively MkDocs/ReadTheDocs content
- NEVER hardcode a user-facing string anywhere (web, API, Swagger/GraphQL, email, CLI/agent output, health page, cookie consent, legal pages) — every string MUST go through `t()`/`{{t .Lang key}}`
- NEVER let a missing translation key or unsupported language error/crash — fall back to English silently
- NEVER convey information by color alone in UI; NEVER skip skip-links, ARIA roles, or focus management on interactive components

## CRITICAL - ALWAYS DO

- ALWAYS run both test phases: Phase 1 `make test` (Go unit tests, ≥60% coverage, pre-commit gate) and Phase 2 `./tests/run_tests.sh` (compiled-binary integration tests, manual/developer-initiated)
- ALWAYS create/update the matching `*_test.go` in the same work pass when you add or change package logic — never defer
- ALWAYS test every route with ALL applicable Accept headers: frontend routes get `text/html` + `text/plain`; API routes get `application/json` + `text/plain`; every `.txt` endpoint tested explicitly
- ALWAYS provide `tests/run_tests.sh`, `tests/docker.sh` (Alpine), `tests/incus.sh` (Debian+systemd) — executable, using `trap` cleanup, `set -eo pipefail`, exit 0/nonzero
- ALWAYS identify exact PID/container name before kill/stop/remove; scope all Docker/process operations to `{project_name}` only
- ALWAYS ship `mkdocs.yml`, `.readthedocs.yaml`, `docs/requirements.txt`, and the required `docs/*.md` set (index, installation, configuration, api, security, integrations, development; cli.md if applicable)
- ALWAYS support all 7 languages (`en es zh fr ar de ja`) identically across server and CLI, embedded via `go:embed` from the single `src/common/i18n/locales/` source
- ALWAYS follow the language fallback chain: `?lang=` (sets cookie) → `lang` cookie → `Accept-Language` → `en`
- ALWAYS validate translation keys at build time (`make i18n-validate`) — identical key sets across all language files, no empty values, matching interpolation vars/plural categories
- ALWAYS meet WCAG 2.1 AA: 4.5:1 text contrast, visible focus indicators, 44x44px touch targets, full keyboard navigation, skip links as first focusable elements

## Key Rules Summary

**Test coverage requirements**
- Phase 1 (`go test -cover`): ≥60% code coverage, enforced in CI, fails build below threshold
- Phase 2 (`./tests/*.sh`): 100% endpoint/route coverage — every API route, frontend route, error path (400/401/403/404/429/500), and edge case
- Priority: always test auth/token validation, DB queries/migrations, API handlers; best-effort on getters/logging; skip third-party internals

**Test file conventions**
- `*_test.go` = package logic, pure functions, validation, parsing, mocked handlers — no running server required
- `./tests/*.sh` = full running binary behavior — routes, auth, systemd, CLI-against-server, container/Incus scenarios
- Required scripts: `tests/run_tests.sh` (auto-detect incus/docker), `tests/docker.sh`, `tests/incus.sh` — must build via Docker with host `GO_CACHE`/`GO_BUILD` mounts, install `curl bash file jq` in Alpine, test binary rename (`--help` shows renamed name), read `server.token` from `server.yml`, run full CLI functionality with a token
- All shell scripts: `# @@License : WTFPL` header

**ReadTheDocs/MkDocs structure**
- Engine: MkDocs Material, dark/light/auto toggle (dark default), theme CSS in `docs/stylesheets/{dark,light}.css`
- Root files: `mkdocs.yml`, `.readthedocs.yaml` (ubuntu-24.04, python 3.12)
- Required `docs/` pages: `index.md`, `installation.md`, `configuration.md`, `api.md`, `security.md`, `integrations.md`, `development.md`, `requirements.txt`; `cli.md` if the project has a CLI
- `docs/` must describe the product as it actually behaves — every operator/integrator/user-facing feature in code must appear here
- Custom theme colors allowed if WCAG AA contrast (4.5:1) is maintained in both themes, documented in AI.md

**i18n string conventions**
- Single source of truth: `src/common/i18n/locales/{lang}.json`, embedded in ALL binaries (server + CLI) via one shared `common/i18n` package
- Key naming: dot-separated lowercase (`health.status.title`); interpolation `{variable}`; plurals nested under CLDR categories (`zero one two few many other`)
- Every `http.Error()` and API JSON error/status message must use a translation key, never hardcoded English
- RTL: Arabic uses `dir="rtl"` from `meta.direction`; use CSS logical properties (`margin-inline-start`, not `margin-left`)
- Adding a language: copy `en.json`, translate all keys, add to `available_languages`, run `make i18n-validate`, rebuild all binaries

**Accessibility requirements**
- WCAG 2.1 AA mandatory: keyboard-only operation, screen reader support (NVDA/JAWS/VoiceOver), 4.5:1 text contrast / 3:1 for large text and UI components
- Skip-to-content and skip-to-navigation links as first two focusable elements on every page
- ARIA live regions for dynamic status/error content (`role="status"`/`role="alert"`, `aria-live`); modals get `role="dialog"` + `aria-modal` + focus trap + focus-return on close
- Forms: labels associated via `for`/`id`, `aria-required`, `aria-describedby` for hints/errors
- `.sr-only` class for screen-reader-only text; test with axe DevTools, WAVE, Lighthouse, and manual keyboard-only pass

For complete details, see AI.md PART 28, 29, 30.
