# Project SPEC

Project: search
Role: Efficient loader for AI.md

`AI.md` is the read-only source of implementation rules; `IDEA.md` defines product behavior and project variables. Read both before implementation, then read the relevant `.claude/rules/` and AI.md PARTs. Never guess when requirements are unclear; preserve user changes and keep docs in sync.

Terminology: server = `search`; required client = `search-cli`; operator = person managing the server through CLI and `server.yml`.

For complete implementation details, follow `AI.md`. Project-specific business logic belongs in `IDEA.md`. Keep this file a short loader; `CLAUDE.md` must be its byte-identical copy.
