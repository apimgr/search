# Project SPEC

Project: search
Org: apimgr
Internal name: search (FROZEN — never edit)
Role: Efficient loader for AI.md

⚠️ **THIS FILE IS AUTO-LOADED EVERY CONVERSATION. FOLLOW IT EXACTLY.** ⚠️

Purpose:
- This file is a short loader for the most important rules
- `AI.md` is the full source of truth (~60k lines, ~2.0MB)
- For complete details, read the referenced PARTs in `AI.md`

## FIRST TURN — MANDATORY

On EVERY new conversation or after "context compacted" message:
1. **READ** `AI.md` PART 0 and PART 1 before doing ANYTHING
2. **READ** the relevant `.claude/rules/*.md` for your current task
3. **NEVER** assume or guess — verify against AI.md before implementing

If you haven't read AI.md this session → STOP → read it NOW.

## Asking Questions

- **Default to continuing work** — do not stop just to ask whether to continue
- **Never guess** — if not in `AI.md`, `IDEA.md`, codebase, or repo state, ASK
- **Question mark = question** — when user ends with `?`, answer/clarify, don't execute
- **Use AskUserQuestion wizard** for multi-question prompts

## Before ANY Code Change

1. Have I read the relevant PART in AI.md? If no → read it
2. Does this follow the spec EXACTLY? If unsure → check spec
3. Am I guessing or do I KNOW from the spec? If guessing → read spec
4. Would this pass the FINAL compliance checklist?

**WHEN IN DOUBT: READ THE SPEC. DO NOT GUESS.**

## Binary Terminology

- **server** = `search` (main binary, runs as service)
- **client** = `search-cli` (REQUIRED companion, CLI/TUI/GUI)
- **agent** = `search-agent` (optional, monitoring/management projects only)

## Key Placeholders

- `{project_name}` = `search`
- `{project_org}` = `apimgr`
- `{internal_name}` = `search` (FROZEN — used for all on-disk identifiers)
- `{admin_path}` = `admin` (default; configurable)
- `{plist_name}` = `io.github.apimgr.search` (derived)

## Account Types (CRITICAL)

- **Server Admin** = manages the app (NOT a privileged OS user)
- **Primary Admin** = first admin, cannot be deleted
- **Regular User** = end-user (PART 34, optional feature)
- Server Admins ≠ Regular Users (separate DB tables)

## Cluster vs Managed Nodes (CRITICAL)

- **Cluster Node** = another instance of THIS app (horizontal scaling)
- **Managed Node** = EXTERNAL resource app controls/monitors (Docker hosts, etc.)
- Most apps only have cluster nodes

## NEVER Do (Top Hits — VIOLATIONS ARE BUGS)

1. Use bcrypt → Use Argon2id
2. Put Dockerfile in root → `docker/Dockerfile`
3. Use CGO → CGO_ENABLED=0 always
4. Hardcode dev values → Detect at runtime
5. Use external cron → Internal scheduler (PART 19)
6. Store passwords plaintext → Argon2id (tokens use SHA-256)
7. Create premium tiers → All features free, no paywalls
8. Use Makefile in CI/CD → Explicit commands only
9. Guess values that a command can produce — run the command (`date`, `basename "$PWD"`, `git config user.email`, `git rev-parse --short HEAD`, `uname -m`)
10. Skip platforms → Build all 8 (linux/darwin/windows/freebsd × amd64/arm64)
11. Client-side rendering (React/Vue) → Server-side Go templates
12. Require JavaScript for core features → Progressive enhancement only
13. Let long strings break mobile → Use word-break CSS
14. Skip validation → Server validates EVERYTHING
15. Implement without reading spec → Read relevant PART first
16. Modify AI.md PART 0-33 content → READ-ONLY SPEC. Project changes go in IDEA.md
17. Edit `## Project variables` in IDEA.md without confirming with the user
18. Read images larger than 1000×1000 directly → Resize first
19. Run plain `git commit`/`git push` → Use `gitcommit <command>` (signs + pushes in one step)

## ALWAYS Do — NON-NEGOTIABLE

1. Read AI.md before implementing ANY feature
2. Server-side processing (server does the work, client displays)
3. Mobile-first responsive CSS
4. All features work without JavaScript
5. Tor hidden service support (auto-enabled if Tor found)
6. Built-in scheduler, GeoIP, metrics, email, backup, update
7. Full admin panel with ALL settings
8. Client binary for ALL projects
9. Commit often via `gitcommit <command>` — small, focused, with a fresh accurate `.git/COMMIT_MESS` each time

## Commit Workflow (MANDATORY)

- Plain `git commit` and `git push` are DENIED
- AI commits via `gitcommit <command>` — signs + pushes in one step
- Pre-commit sequence:
  1. `git status --porcelain` and `git diff --stat` — verify changes
  2. Write `.git/COMMIT_MESS` (file is the contract)
  3. **Re-read** `.git/COMMIT_MESS` — push is irreversible
  4. Run `gitcommit <command>` (`new`/`improved`/`fixes`/`docs`/`test`/`all`/etc.)
- Commit message format: `{emoji} Title (max 64 chars) {emoji}` then blank line, then bulleted file-level details
- Emojis: ✨ feat / 🐛 fix / 📝 docs / 🎨 style / ♻️ refactor / ⚡ perf / ✅ test / 🔧 chore / 🔒 security / 🗑️ remove / 🚀 deploy / 📦 deps
- NEVER include AI/vendor attribution anywhere

## File Locations

- Config: `{config_dir}/server.yml`
- Data: `{data_dir}/`
- Logs: `{log_dir}/`
- Source: `src/`
- Docker: `docker/` (Dockerfile + compose + `docker/rootfs/` build-time overlay)
- Runtime volumes: `volumes/` (gitignored)

## Where to Find Details

- AI behavior: `.claude/rules/ai-rules.md` (PART 0, 1)
- Project structure: `.claude/rules/project-rules.md` (PART 2, 3, 4)
- Configuration: `.claude/rules/config-rules.md` (PART 5, 6, 12)
- Frontend/WebUI: `.claude/rules/frontend-rules.md` (PART 16, 17)
- Full spec: `AI.md` ← **SOURCE OF TRUTH**

## Current Project State

- AI.md last refreshed: 2026-05-06 (from `~/Templates/go/TEMPLATE.md`)
- IDEA.md format: three-section (migrated 2026-05-06; backup at `IDEA.md.preMigration.bak`)
- TODO.AI.md: 1 pending (`i18n-translator-review` — zh/ja/ar/he/fa/ur `help.*` keys need human translator)
- All CI/CD workflows passing: SHA-pinned, container builds, concurrency, govulncheck, go vet, docker build
- `src/instant/utils.go` split into per-concern files (time/hash/base64/url/color/uuid/random/password/ip)
- go-redis upgraded to v9.7.3 (CVE GO-2025-3540 remediated)
- PART 34 test leftovers removed from src/api/api_test.go; getClientIP restored to src/api/api.go
- go.mod tidy'd: skip2/go-qrcode and golang.org/x/text promoted to direct deps, pquerna/otp removed
