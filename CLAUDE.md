# Project SPEC

project_name:  search
project_org:   apimgr
internal_name: search

## Spec Files

| File | Purpose |
|------|---------|
| `AI.md` | Implementation patterns (HOW) — READ ONLY |
| `IDEA.md` | Business logic and features (WHAT) — update as needed |
| `.claude/rules/` | Extracted rule files for efficient context loading |

## Session Start

1. Read `.claude/rules/ai-rules.md` (always)
2. Read `.claude/rules/` files relevant to the current task
3. Read relevant IDEA.md sections for business logic
4. Only read full AI.md PARTs when rules files don't have enough detail

## Rule Files

| File | PARTs | Topic |
|------|-------|-------|
| `.claude/rules/ai-rules.md` | 0, 1 | AI behavior, naming conventions |
| `.claude/rules/project-rules.md` | 2, 3, 4 | License, structure, OS paths |
| `.claude/rules/config-rules.md` | 5, 6, 12 | Configuration, modes, server config |
| `.claude/rules/binary-rules.md` | 7, 8, 32 | Binary, server CLI, client |
| `.claude/rules/backend-rules.md` | 9, 10, 11, 31 | Errors, DB, security, Tor |
| `.claude/rules/api-rules.md` | 13, 14, 15 | Health, API structure, SSL/TLS |
| `.claude/rules/frontend-rules.md` | 16 | Web frontend |
| `.claude/rules/features-rules.md` | 17-22 | Email, scheduler, GeoIP, metrics, backup, update |
| `.claude/rules/service-rules.md` | 23, 24 | Privilege, service |
| `.claude/rules/makefile-rules.md` | 25 | Makefile |
| `.claude/rules/docker-rules.md` | 26 | Docker |
| `.claude/rules/cicd-rules.md` | 27 | CI/CD |
| `.claude/rules/testing-rules.md` | 28, 29, 30 | Testing, docs, i18n/a11y |
