# Project SPEC

Project: search
Role: Efficient loader for AI.md

⚠️ **THIS FILE IS AUTO-LOADED EVERY CONVERSATION. FOLLOW IT EXACTLY.** ⚠️

Purpose:
- This file is a short loader for the most important rules
- `AI.md` is the full source of truth
- For complete details, read the referenced PARTs in `AI.md`

## Asking Questions

- **Default to continuing work** — do not stop just to ask whether you should continue; if the next step is implied by the spec, the current task, or the current findings, continue
- **Never guess** — if the answer cannot be determined from `AI.md`, `IDEA.md`, the codebase, or repo state **and** the missing information materially changes behavior, scope, or safety, ASK the user
- **Do NOT ask for permission to keep going** — continue until the current task is complete, blocked by a real decision, or the user explicitly asks to pause
- **Question mark = question** — when user ends with `?`, answer/clarify, don't execute

## Before ANY Code Change

1. Have I read the relevant PART in AI.md? (If no → read it)
2. Does this follow the spec EXACTLY? (If unsure → check spec)
3. Am I guessing or do I KNOW from the spec? (If guessing → read spec)

**WHEN IN DOUBT: READ THE SPEC. DO NOT GUESS.**

## Binary Terminology
- **server** = `search` (main binary, runs as service)
- **client** = `search-cli` (REQUIRED companion, CLI/TUI/GUI)

## Key Placeholders
- `{project_name}` = search
- `{internal_name}` = search (FROZEN — never changes after init)
- `{project_org}` = apimgr
- `{plist_name}` = io.github.apimgr.search

## NEVER Do (Top 19) - VIOLATIONS ARE BUGS
1. Use bcrypt for config/backup passwords → Use Argon2id
2. Put Dockerfile in root → `docker/Dockerfile`
3. Use CGO → CGO_ENABLED=0 always
4. Hardcode dev values → Detect at runtime
5. Use external cron → Internal scheduler (PART 18)
6. Store config/backup passwords plaintext → Argon2id (API tokens use SHA-256)
7. Create premium tiers → All features free, no paywalls
8. Use Makefile in CI/CD → Explicit Docker commands only
9. Guess or assume values that a command can produce → Run the command
10. Skip platforms → Build all 8 (linux/darwin/windows/freebsd × amd64/arm64)
11. Client-side rendering (React/Vue) → Server-side Go templates
12. Require JavaScript for core features → Progressive enhancement only
13. Let long strings break mobile → Use word-break CSS
14. Skip validation → Server validates EVERYTHING
15. Implement without reading spec → Read relevant PART first
16. Modify AI.md content → READ-ONLY SPEC
17. Edit `## Project variables` in IDEA.md without confirming with the user
18. Run Go/binaries on local host → ALWAYS use Docker `casjaysdev/go:latest`
19. Use forbidden DB drivers → Only `modernc.org/sqlite` and `libsql-client-go` allowed

## ALWAYS Do - NON-NEGOTIABLE
1. Read AI.md before implementing ANY feature
2. Server-side processing (server does the work, client displays)
3. Mobile-first responsive CSS with CSS custom properties
4. All features work without JavaScript
5. Tor hidden service support (auto-enabled if Tor found)
6. Built-in scheduler, GeoIP, metrics, email, backup, update
7. All settings configurable via config file, CLI flags, AND env vars
8. Client binary `search-cli` for ALL projects
9. Commit often via `gitcommit --dir /root/Projects/github/apimgr/search all`

## File Locations
- Config: `{config_dir}/server.yml`
- Data: `{data_dir}/`
- Logs: `{log_dir}/`
- Source: `src/`
- Docker: `docker/`

## Where to Find Details
- AI behavior + rules: `.claude/rules/ai-rules.md` (PART 0, 1)
- Project structure + paths: `.claude/rules/project-rules.md` (PART 2, 3, 4)
- Config + middleware + debug: `.claude/rules/config-rules.md` (PART 5, 6, 12)
- Binary + platforms + client: `.claude/rules/binary-rules.md` (PART 7, 8, 32)
- Error handling + DB + security + Tor: `.claude/rules/backend-rules.md` (PART 9, 10, 11, 31)
- API + health + TLS: `.claude/rules/api-rules.md` (PART 13, 14, 15)
- Frontend + templates: `.claude/rules/frontend-rules.md` (PART 16)
- Email + scheduler + GeoIP + metrics + backup + update: `.claude/rules/features-rules.md` (PART 17-22)
- Service installation + privilege escalation: `.claude/rules/service-rules.md` (PART 23, 24)
- Makefile + build: `.claude/rules/makefile-rules.md` (PART 25)
- Docker: `.claude/rules/docker-rules.md` (PART 26)
- CI/CD workflows: `.claude/rules/cicd-rules.md` (PART 27)
- Testing + docs + i18n: `.claude/rules/testing-rules.md` (PART 28, 29, 30)
- Full spec: `AI.md` (~55k lines) ← **SOURCE OF TRUTH**

## Current Project State
[AI updates this section as work progresses]
- Last read AI.md: 2026-06-12
- Current task: Regenerating .claude/rules/ files from spec
- Relevant PARTs: 0-32
