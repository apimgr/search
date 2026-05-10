# Tasks for search

This file mirrors the current AI task state in the repository so it survives local/session migration.

## Pending / next candidate

- [ ] `i18n-translator-review` — Replace English placeholders in `src/i18n/locales/{zh,ja,ar,he,fa,ur}.json` for `help.*`, `auth.reset.*`, `auth.forgot.*`, `user.profile.*`, `user.security.*`, `user.tokens.*`, `user.twofa.*`, `user.recovery.*`, `user.nav.*`, and `admin.nav.*` keys with proper translations. Per AI.md PART 0 "Never guess or assume", AI used English placeholders for these locales because translation confidence was insufficient. Needs a human translator pass per locale. Cumulative key count across `help`, `auth`, `user`, `admin`, `common`, `form` namespaces: ~360 keys × 6 locales = ~2,160 strings to translate.

## Completed

- [x] `auth-user-admin-chrome-i18n` — Localized auth (reset, forgot), user (profile, security, tokens, 2fa-setup, recovery-keys), and admin chrome (layout/admin, partial/admin/sidebar) templates across all 15 locales. Fixed real pre-existing bug along the way: many templates already referenced keys (auth.forgot.*, user.profile.*, user.security.*, user.tokens.*, etc.) that did NOT exist in any locale, so those pages were rendering literal key strings. Restructured `user.profile`/`.security`/`.tokens` from flat nav-label strings to nested page-content namespaces, preserving locale-specific nav translations under new `user.nav.*` keys. ~110 new keys (commits `4ce99ae7b7f2` auth, `44bd05635b29` user, plus this admin commit).
- [x] `public-help-page-i18n` — Public help page is fully localized. All 7 phases shipped across 15 locales (proper translations for en/es/fr/de/it/pt/nl/pl/ru, English placeholders for zh/ja/ar/he/fa/ur tracked above for translator review). 255 `help.*` keys total.
  - Phase 1: page title + h1 + subtitle + TOC (commit `8666cd634d61`)
  - Phase 2: Getting Started + Search Categories (commit `c80ffcae6f06`)
  - Phase 3: Search Operators table + Keyboard Shortcuts table (commit `65f43b4c81c0`)
  - Phase 4: Privacy Features + Tor Access + FAQ + Need More Help (commit `2aeff104b089`)
  - Phase 5: API Documentation (commit `fdfd4b77a44f`)
  - Phase 6: Bang Commands (commit `5823daa76498`)
  - Phase 7: Direct Answers — h2 + intro + 9 group headings + 59 operator descriptions

(Historical completed work is preserved in git history; see commit `8e6b14655dec` for the `quotes-500-bug` fix.)
