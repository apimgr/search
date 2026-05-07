# Tasks for search

This file mirrors the current AI task state in the repository so it survives local/session migration.

## Pending / next candidate

- [ ] `public-help-page-i18n` — Localize the fallback body in `src/server/template/page/help.tmpl` plus related public help page-title metadata in `src/server/pages.go`.

## Completed

- [x] `quotes-500-bug` — Fixed (uncommitted, 2026-05-06). Aggregator returns `model.ErrNoResults` for zero-result queries (including `q=""`); callers were treating that as fatal and returning HTTP 500. Per AI.md PART 9 (recoverable errors) and PART 14 (success envelope), no-results is a 200 with empty payload.
  - `src/server/server.go` — `handleSearch` now skips the error path when `errors.Is(err, model.ErrNoResults)` and renders the no-results page; added `errors` import.
  - `src/api/api.go` — REST `handleSearch` treats `ErrNoResults` as 200 OK with empty `SearchResponse`; added `errors` import.
  - `src/api/graphql.go` — GraphQL `search` resolver treats `ErrNoResults` as empty results, not a GraphQL error; added `errors` import.
  - `src/api/api_test.go` — added `TestSearchEndpointEmptyResultsNotFatal` regression test using `emptyResultEngine` mock; asserts 200/OK for `q=""`, `q=foo`, `q=""""`.

(Historical task list cleared 2026-05-06 during AI.md template refresh. Completed work is preserved in git history.)
