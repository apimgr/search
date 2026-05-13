# Tasks for search

This file mirrors the current AI task state in the repository so it survives local/session migration.

## Pending / next candidate

- [ ] `i18n-translator-review` — Replace English placeholders in `src/i18n/locales/{zh,ja,ar,he,fa,ur}.json` for `help.*` and surviving `auth.login.*` keys with proper translations. Per AI.md PART 0 "Never guess or assume", AI used English placeholders for these locales because translation confidence was insufficient. Needs a human translator pass per locale.

## Completed

- [x] `part34-cleanup-go-and-templates` — Removed all PART 34 (regular users) surface from the codebase. IDEA.md declares PART 34/35/36 not implemented. Deleted: `src/user/` package (11 files), `src/server/user.go`, `src/api/user.go`, `src/api/auth.go` (user-only routes), `src/server/template/auth/{register,forgot,reset,verify}.tmpl`, `src/server/template/user/*.tmpl` (5 files). Rewrote `src/server/auth.go` (~740 → ~120 lines, admin-only login/logout). Removed `Server.Users`/`UsersConfig` from `src/config/config.go`. Stripped 115 user/auth i18n keys × 15 locales. Removed 16 PART 34 paths from `src/api/openapi.json`. Admin auth (`/auth/login`, `/auth/logout`) preserved intact — PART 11 unified login endpoint.
- [x] `orphan-admin-chrome-cleanup` — Removed `src/server/template/layout/admin.tmpl` and `src/server/template/partial/admin/{header,footer,sidebar}.tmpl`. Verified never loaded: `embed.go:154` loads only `layout/base.tmpl`; the real admin panel uses `src/admin/templates.go` programmatic HTML, not these template files. Dropped `template/partial/admin/*.tmpl` from the `go:embed` directive. Removed `admin.nav.*` namespace (23 keys) from all 15 locale files. No Go behavior changes — entirely dead code.
- [x] `auth-user-admin-chrome-i18n` — Localized auth (reset, forgot), user (profile, security, tokens, 2fa-setup, recovery-keys), and admin chrome (layout/admin, partial/admin/sidebar) templates across all 15 locales. Fixed real pre-existing bug along the way: many templates already referenced keys (auth.forgot.*, user.profile.*, user.security.*, user.tokens.*, etc.) that did NOT exist in any locale, so those pages were rendering literal key strings. Restructured `user.profile`/`.security`/`.tokens` from flat nav-label strings to nested page-content namespaces, preserving locale-specific nav translations under new `user.nav.*` keys. ~110 new keys (commits `4ce99ae7b7f2` auth, `44bd05635b29` user, plus the admin chrome commit). NOTE: admin chrome localization targeted dead code (see `orphan-admin-chrome-cleanup`); the auth and user localizations target PART 34 templates that are scheduled for deletion (see `part34-cleanup-go-and-templates`).
- [x] `public-help-page-i18n` — Public help page is fully localized. All 7 phases shipped across 15 locales (proper translations for en/es/fr/de/it/pt/nl/pl/ru, English placeholders for zh/ja/ar/he/fa/ur tracked above for translator review). 255 `help.*` keys total.
  - Phase 1: page title + h1 + subtitle + TOC (commit `8666cd634d61`)
  - Phase 2: Getting Started + Search Categories (commit `c80ffcae6f06`)
  - Phase 3: Search Operators table + Keyboard Shortcuts table (commit `65f43b4c81c0`)
  - Phase 4: Privacy Features + Tor Access + FAQ + Need More Help (commit `2aeff104b089`)
  - Phase 5: API Documentation (commit `fdfd4b77a44f`)
  - Phase 6: Bang Commands (commit `5823daa76498`)
  - Phase 7: Direct Answers — h2 + intro + 9 group headings + 59 operator descriptions

(Historical completed work is preserved in git history; see commit `8e6b14655dec` for the `quotes-500-bug` fix.)
