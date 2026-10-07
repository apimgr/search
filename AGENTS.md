# Project SPEC

Project: search
Role: Efficient loader for AI.md

⚠️ **THIS FILE IS AUTO-LOADED EVERY CONVERSATION. FOLLOW IT EXACTLY.** ⚠️

Purpose:
- This file is a short loader for the most important rules
- `AI.md` is the full source of truth (THE HOW); `IDEA.md` is the product definition (THE WHAT)
- For complete details, read the referenced PARTs in `AI.md`

## Asking Questions

- **Default to continuing work** - do not stop just to ask whether you should continue
- **Never guess** - if the answer is not in `AI.md`, `IDEA.md`, the codebase, or repo state and it materially changes behavior, scope, or safety, ASK
- **Do NOT ask for permission to keep going** - continue until the task is complete or genuinely blocked
- **Question mark = question** - when the user ends with `?`, answer, don't execute

Ask only when: a business/product decision is missing; two implementations differ materially; the action is destructive or irreversible; the spec says to ask; or the user asked for a plan/checkpoint.

## Before ANY Code Change

1. Have I read the relevant PART in AI.md? (If no → read it)
2. Does this follow the spec EXACTLY? (If unsure → check spec)
3. Am I guessing or do I KNOW from the spec? (If guessing → read spec)
4. Would this pass the compliance checklist? (AI.md FINAL section)

**WHEN IN DOUBT: READ THE SPEC. DO NOT GUESS.**

## Binary Terminology

- **server** = `search` (main binary, runs as a service)
- **client** = `search-cli` (REQUIRED companion, CLI/TUI)
- **Operator** = person who deploys and manages the server via CLI and `server.yml`

## Key Placeholders

- `{project_name}` = `search` · `{project_org}` = `apimgr`
- `{internal_name}` = `search` (FROZEN) · `{internal_org}` = `apimgr`
- `{official_site}` = `https://scour.li`
- Full list: `IDEA.md` → `## Project variables` (never edit without confirming with the user)

## NEVER Do - VIOLATIONS ARE BUGS

1. Use bcrypt for config/backup passwords → Argon2id (PART 11)
2. Put a Dockerfile in the repo root → `docker/Dockerfile` (PART 26)
3. Use CGO → `CGO_ENABLED=0` always (PART 7)
4. Hardcode dev values → detect at runtime
5. Use external cron → built-in scheduler (PART 18)
6. Store config/backup passwords in plaintext → Argon2id (API tokens use SHA-256)
7. Create premium tiers → all features free, no paywalls
8. Use the Makefile in CI/CD → explicit commands only (PART 25, 27)
9. Guess a value a command can produce → run the command
10. Skip platforms → build all 8 (linux/darwin/windows/freebsd × amd64/arm64)
11. Client-side rendering (React/Vue) → server-side Go templates (PART 16)
12. Add JavaScript for anything HTML5+CSS already does → JS is a LAST RESORT (PART 16)
13. Let long strings break mobile → use word-break CSS
14. Skip validation → the server validates EVERYTHING
15. Implement without reading the spec → read the relevant PART first
16. Modify `AI.md` content → READ-ONLY spec; project changes go in `IDEA.md`
17. Edit `## Project variables` in `IDEA.md` without confirming with the user
18. Leave TODO/stub/partial code → implement 100% or don't implement
19. Use a `.yaml` config file → the config file is always `server.yml` (PART 5)

## ALWAYS Do - NON-NEGOTIABLE

1. Read `AI.md` before implementing ANY feature
2. Server-side processing (server does the work, client displays)
3. Mobile-first responsive CSS; dark/light/auto theme, no hardcoded colors
4. All features work without JavaScript
5. Tor hidden service support (auto-enabled when the Tor binary is found)
6. Built-in scheduler, GeoIP, metrics, email, backup, update
7. All settings configurable via `server.yml`, hot-reloaded by file watcher
8. Client binary (`search-cli`) is REQUIRED
9. Commit via `gitcommit --dir {project_dir} all` with a fresh `.git/COMMIT_MESS`. **Subagents never commit** — they edit and report back; only the parent reviews the diff and commits.

## File Locations

- Config: `{config_dir}/server.yml` · Data: `{data_dir}/` · Logs: `{log_dir}/`
- Source: `src/` · Docker: `docker/` · Docs: `docs/` · Tests: `tests/`
- OS path resolution: `src/config/directories.go` (PART 4)

## Where to Find Details

- AI behavior: `.claude/rules/ai-rules.md` (PART 0, 1)
- Project structure: `.claude/rules/project-rules.md` (PART 2, 3, 4)
- Config/modes: `.claude/rules/config-rules.md` (PART 5, 6, 12)
- Binaries/CLI: `.claude/rules/binary-rules.md` (PART 7, 8, 32)
- Backend: `.claude/rules/backend-rules.md` (PART 9, 10, 11, 31)
- API: `.claude/rules/api-rules.md` (PART 13, 14, 15)
- Frontend/WebUI: `.claude/rules/frontend-rules.md` (PART 16)
- Features: `.claude/rules/features-rules.md` (PART 17-22)
- Service: `.claude/rules/service-rules.md` (PART 23, 24)
- Makefile: `.claude/rules/makefile-rules.md` (PART 25)
- Docker: `.claude/rules/docker-rules.md` (PART 26)
- CI/CD: `.claude/rules/cicd-rules.md` (PART 27)
- Testing/docs/i18n: `.claude/rules/testing-rules.md` (PART 28, 29, 30)
- Full spec: `AI.md` ← **SOURCE OF TRUTH**

## Current Project State

- Last read AI.md: see session start
- Current task: see `TODO.AI.md`
- Relevant PARTs: determined per task
