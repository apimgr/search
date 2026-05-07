# Tasks for search

This file mirrors the current AI task state in the repository so it survives local/session migration.

## Pending / next candidate

- [ ] `public-help-page-i18n` — Localize the rest of the fallback body in `src/server/template/page/help.tmpl`. Phases 1-4 shipped. Remaining sections to migrate (each as its own commit, all 15 locales per phase):
  - [x] Phase 1: page title + h1 + subtitle + TOC (commit `8666cd634d61`)
  - [x] Phase 2: Getting Started + Search Categories (commit `c80ffcae6f06`)
  - [x] Phase 3: Search Operators table + Keyboard Shortcuts table (commit `65f43b4c81c0`)
  - [x] Phase 4: Privacy Features + Tor Access + FAQ + Need More Help
  - [ ] Phase 5: API Documentation (~25 keys, including endpoint descriptions and 3 example block headings)
  - [ ] Phase 6: Bang Commands (~50 keys: heading + intro + Popular Bangs + By Category table + Examples list)
  - [ ] Phase 7: Direct Answers (~55 keys: 7 sub-section headings + ~50 operator descriptions + Instant answer examples)
- [ ] `public-help-i18n-translator-review` — Replace English placeholders in `src/i18n/locales/{zh,ja,ar,he,fa,ur}.json` `help.*` keys with proper translations. Per AI.md PART 0 "Never guess or assume", AI used English placeholders for these locales because translation confidence was insufficient. Needs a human translator pass per locale. Cumulative key count after phase 4: ~120 keys (will grow to ~250 once phases 5-7 ship).

(Historical completed work is preserved in git history; see commit `8e6b14655dec` for the `quotes-500-bug` fix and commits `8666cd634d61`, `c80ffcae6f06`, `65f43b4c81c0` for help-page i18n phases 1-3.)
