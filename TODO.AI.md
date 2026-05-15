# Tasks for search

This file mirrors the current AI task state in the repository so it survives local/session migration.

## Pending / next candidate

- [ ] `i18n-translator-review` — Replace English placeholders in `src/i18n/locales/{zh,ja,ar,he,fa,ur}.json` for `help.*` and surviving `auth.login.*` keys with proper translations. Per AI.md PART 0 "Never guess or assume", AI used English placeholders for these locales because translation confidence was insufficient. Needs a human translator pass per locale.

- [ ] `instant-utils-split` — `src/instant/utils.go` (762 lines) violates the "no `utils.go`" naming rule. File is a grab bag of Time / Hash / Base64 / URL handlers. Split into `src/instant/time.go`, `src/instant/hash.go`, `src/instant/base64.go`, `src/instant/url.go` (preserving package and exported names). Mechanical refactor — no behavior change — but touches enough surface to deserve its own commit.

(Historical completed work is preserved in git history.)
