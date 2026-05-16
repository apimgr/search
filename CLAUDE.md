# Search

Project: search
Org: apimgr
Role: Efficient loader for AI.md

⚠️ **THIS FILE IS AUTO-LOADED EVERY CONVERSATION. FOLLOW IT EXACTLY.** ⚠️

Purpose:
- This file is a short loader for the most important rules
- `AI.md` is the full source of truth (~55k lines)
- `IDEA.md` is the project plan (features, data models, business rules)
- For complete details, read the referenced PARTs in `AI.md`

## FIRST TURN - MANDATORY

On EVERY new conversation or after "context compacted" message:
1. **READ** the relevant `.claude/rules/*.md` for your current task
2. **NEVER** assume or guess - verify against AI.md before implementing

## Before ANY Code Change

1. Have I read the relevant PART in AI.md? (If no → read it)
2. Does this follow the spec EXACTLY? (If unsure → check spec)
3. Am I guessing or do I KNOW from the spec? (If guessing → read spec)
4. Would this pass the compliance checklist? (AI.md FINAL section)

**WHEN IN DOUBT: READ THE SPEC. DO NOT GUESS.**

## Binary Terminology
- **server** = `search` (main binary, runs as service)
- **client** = `search-cli` (REQUIRED companion, CLI/TUI/GUI)
- **agent** = `search-agent` (optional, runs on remote machines)

## Key Placeholders
- `{project_name}` = search
- `{project_org}` = apimgr
- `{internal_name}` = search (FROZEN)
- `{admin_path}` = admin (default)
- `{plist_name}` = io.github.apimgr.search (derived)

## Account Types (CRITICAL)
- **Server Admin** = manages the app (NOT a privileged OS user)
- **Primary Admin** = first admin, cannot be deleted
- **No Regular Users** — PART 34 is NOT implemented (privacy is the product)

## NEVER Do (Top Violations)
1. Use bcrypt → Use Argon2id
2. Put Dockerfile in root → `docker/Dockerfile`
3. Use CGO → CGO_ENABLED=0 always
4. Hardcode dev values → Detect at runtime
5. Use external cron → Internal scheduler (PART 19)
6. Store passwords plaintext → Argon2id (tokens use SHA-256)
7. Create premium tiers → All features free, no paywalls
8. Use Makefile in CI/CD → Explicit commands only
9. Client-side rendering (React/Vue) → Server-side Go templates
10. Log user queries or IPs → privacy is the product, no server-side logs

## ALWAYS Do
1. Read AI.md before implementing ANY feature
2. Server-side rendering with Go templates
3. Mobile-first responsive CSS
4. All features work without JavaScript
5. Tor hidden service support (auto-enabled if Tor found)
6. Built-in scheduler, GeoIP, metrics, email, backup, update
7. Full admin panel at `/server/admin` with ALL settings
8. Client binary (search-cli) for ALL projects
9. Commit often — small, focused commits

## File Locations
- Config: `/etc/apimgr/search/server.yml` (Linux)
- Data: `/var/lib/apimgr/search/`
- Logs: `/var/log/apimgr/search/`
- Source: `src/`
- Docker: `docker/`

## Where to Find Details
- AI behavior: `.claude/rules/ai-rules.md` (PART 0, 1)
- Project structure: `.claude/rules/project-rules.md` (PART 2, 3, 4)
- Frontend/WebUI: `.claude/rules/frontend-rules.md` (PART 16, 17)
- Full spec: `AI.md` (~55k lines) ← **SOURCE OF TRUTH**

## Current Project State
- Last read AI.md: 2026-05-16
- Current task: bootstrap
- Relevant PARTs: 0-6
