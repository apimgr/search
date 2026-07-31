# Project Rules (PART 2, 3, 4)

Read: `AI.md` PART 2 (License & Attribution), PART 3 (Project Structure), PART 4 (OS-Specific Paths).

- License is MIT — every direct go.mod dependency must be attributed in `LICENSE.md`; no GPL/AGPL/LGPL dependencies.
- Root files are an exhaustive allowed list (`AI.md` PART 3 § Allowed Root Files) — never add a root file not on that list.
- `src/` layout follows Go package-per-concern structure — no `utils/`, `common/`, `lib/`, `libs/`, `vendor/`, `node_modules/`.
- OS-specific paths (config/data/cache/log dirs) come from `src/path/paths.go` — Linux/Darwin/BSD/Windows, privileged vs non-privileged variants. Never hardcode a path outside that package.
