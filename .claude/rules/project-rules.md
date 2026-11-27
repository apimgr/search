# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Put source, generated documentation, or runtime artifacts in the wrong project locations.
- Guess license attribution, project variables, or official-site values.
- Store runtime state, credentials, or build artifacts in tracked source directories.

## CRITICAL - ALWAYS DO
- Follow the required project structure and exact OS-specific config/data/cache/log/backup paths.
- Keep third-party provenance and required attribution with shipped data and assets.
- Keep product requirements in IDEA.md and implementation rules in AI.md.

## Key rules
- Server binary: `search`; required companion client: `search-cli`.
- Source belongs in `src/`, documentation in `docs/`, tests in `tests/`, Docker files in `docker/`.
- Resolve paths with the shared directory implementation; do not hardcode development values.

For complete details, see AI.md PART 2, 3, 4.
