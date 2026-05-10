# Tasks for search

This file mirrors the current AI task state in the repository so it survives local/session migration.

## Pending / next candidate

- [ ] `public-help-i18n-translator-review` — Replace English placeholders in `src/i18n/locales/{zh,ja,ar,he,fa,ur}.json` `help.*` keys with proper translations. Per AI.md PART 0 "Never guess or assume", AI used English placeholders for these locales because translation confidence was insufficient. Needs a human translator pass per locale. Total key count: 255 keys × 6 locales = 1,530 strings to translate.

## Completed

- [x] `public-help-page-i18n` — Public help page is fully localized. All 7 phases shipped across 15 locales (proper translations for en/es/fr/de/it/pt/nl/pl/ru, English placeholders for zh/ja/ar/he/fa/ur tracked above for translator review). 255 `help.*` keys total.
  - Phase 1: page title + h1 + subtitle + TOC (commit `8666cd634d61`)
  - Phase 2: Getting Started + Search Categories (commit `c80ffcae6f06`)
  - Phase 3: Search Operators table + Keyboard Shortcuts table (commit `65f43b4c81c0`)
  - Phase 4: Privacy Features + Tor Access + FAQ + Need More Help (commit `2aeff104b089`)
  - Phase 5: API Documentation (commit `fdfd4b77a44f`)
  - Phase 6: Bang Commands (commit `5823daa76498`)
  - Phase 7: Direct Answers — h2 + intro + 9 group headings + 59 operator descriptions

(Historical completed work is preserved in git history; see commit `8e6b14655dec` for the `quotes-500-bug` fix.)
