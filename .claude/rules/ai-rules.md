# AI Assistant Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Guess or assume values — always search, read spec, or ask
- Say "done" without verifying output
- Skip reading a file before editing it
- Create patterns not in the spec
- Create report/analysis/audit files (fix directly instead)
- Edit AI.md or TEMPLATE.md (READ-ONLY)
- Rely on memory — read the relevant spec sections instead
- Use inline YAML comments (comments go above the key, never after the value)
- Hardcode English strings — all user-facing text uses i18n keys
- Use `strconv.ParseBool()` — always use `config.ParseBool()`
- Use generic names: Mode, Config, Status, Type — use intent-revealing names

## CRITICAL - ALWAYS DO

- Read relevant AI.md PARTs before each task
- Search before creating (check if it exists)
- Test changes and verify output before claiming completion
- Use intent-revealing names (SearchEngineMode, not Mode)
- Comments always ABOVE the line they describe, never inline
- One logical change per commit
- Use `config.ParseBool()` for all boolean parsing (accepts yes/no/enable/oui/etc.)
- All user-facing strings via i18n keys (never hardcoded English)
- `curl -q -LSsf` is the standard curl invocation

## Key Rules Summary

### Session Initialization
1. Read existing CLAUDE.md and .claude/CLAUDE.md
2. Check if .claude/rules/ directory exists — create/update if missing
3. Read TODO.AI.md and TODO.md for pending items
4. Commit all COMMIT, NEVER, and MUST rules to memory

### Naming Convention
- Intent-revealing names are MANDATORY
- Bad: `Mode`, `Config`, `Status`, `Type`, `Handler`, `Service`, `Manager`
- Good: `SearchEngineMode`, `ServerConfig`, `EngineHealthStatus`, `ResultType`
- Go directories: singular (handler/, model/, middleware/) — matches package names
- Tooling dirs: always plural (scripts/, tests/, completions/)

### Spec Compliance
- AI.md = HOW to implement (never modify)
- IDEA.md = WHAT the project does (update as project evolves)
- Task → PART mapping:
  - Config: PART 5 | CLI: PART 8 | API: PART 14 | Docker: PART 27
  - Tests: PART 28/29 | i18n: PART 30 | Frontend: PART 16

### Translation Rule
- All user-facing text MUST use i18n key lookups
- Pattern: `t(r, "category.key")` or `i18n.T(lang, "key")`
- Never: `"Search results for " + query`
- Always: `t(r, "search.results_for", query)`

### curl Standard
- Always: `curl -q -LSsf`
- Never: `curl`, `wget`, `curl -s`, `curl -L`

For complete details, see AI.md PART 0, 1
