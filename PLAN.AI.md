# Public i18n continuation plan

## Problem

The public i18n cleanup is mostly complete, but a few public-facing fallback surfaces still emit hardcoded English. The current active slice is the direct-answer page; after that, the largest remaining public fallback target is the help page.

## Current status

- Completed: recovered alert/privacy/preferences backlog, language resolution, locale delivery, fallback `lang`/`dir` cleanup, admin i18n backlog, public JS i18n backlog, shared public layout/footer/partials, alert manage flow, home/contact, search results, and privacy/terms pages.
- Completed: the direct-answer page template and inline fallback labels are now localized with embedded locale parity and focused coverage.
- Next active slice: localize `src/server/template/page/help.tmpl` and the related page-title metadata in `src/server/pages.go`.

## Approach

1. Localize the remaining public help-page fallback body and help page-title metadata as the next bounded public i18n slice.
2. Audit the remaining public fallback pages after the help page is stable.
3. Keep the remaining public i18n work incremental and verification-driven: one coherent user-facing surface at a time, locale parity in embedded JSON, and focused existing test coverage after each slice.

## Notes

- `PLAN.AI.md` is now the repo-local source of truth for the active implementation plan.
- `TODO.AI.md` mirrors the current SQL/session task state for migration continuity.
- When this plan is fully implemented and verified, replace this file with the AI.md completion message instead of deleting it.
