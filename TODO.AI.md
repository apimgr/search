# Tasks for search

This file mirrors the current AI task state in the repository so it survives local/session migration.

## Pending / next candidate

- [ ] `public-help-page-i18n` — Localize the rest of the fallback body in `src/server/template/page/help.tmpl`. Phase 1 (page title + h1/subtitle + TOC) shipped in commit `8666cd634d61`. Remaining sections to migrate (each as its own commit, all 15 locales per phase):
  - [ ] Getting Started + Search Categories (~25 keys)
  - [ ] Bang Commands (~50 keys)
  - [ ] Direct Answers (~55 keys, mostly operator descriptions)
  - [ ] Search Operators table (~22 keys)
  - [ ] Keyboard Shortcuts (~10 keys)
  - [ ] API Documentation (~25 keys)
  - [ ] Privacy Features + Tor Access + FAQ + Need More Help (~25 keys)
- [ ] `public-help-i18n-translator-review` — Replace English placeholders in `src/i18n/locales/{zh,ja,ar,he,fa,ur}.json` `help.*` keys with proper translations. Phase 1 added 13 keys; subsequent phases will add ~150 more keys. Per AI.md PART 0 "Never guess or assume", AI used English placeholders for these locales because translation confidence was insufficient. Needs a human translator pass per locale.

(Historical completed work is preserved in git history; see commit `8e6b14655dec` for the `quotes-500-bug` fix and commit `8666cd634d61` for help page i18n phase 1.)
