# Frontend Rules (PART 16)

Read: `AI.md` PART 16 (Web Frontend).

- Server-side rendering only — Go templates (`src/server/template`), never client-side rendering frameworks.
- Static assets live in `src/server/static`.
- Dark/light/auto theme support required — see `src/common/theme`, no hardcoded colors.
