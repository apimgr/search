# AI Assistant Rules (PART 0, 1)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER guess or assume when a requirement is unclear — STOP and ASK
- NEVER fill in spec gaps by inventing behavior — ask for clarification
- NEVER claim "done" without reading, searching, testing, and verifying first
- NEVER skip reading the relevant AI.md PART before implementing a task
- NEVER modify AI.md — it is read-only; update IDEA.md instead
- NEVER create report files (AUDIT.md, COMPLIANCE.md, SUMMARY.md) — fix issues directly
- NEVER leave partial work without explicitly saying it's incomplete
- NEVER rush past "This is probably what they meant" / "I'll just assume" / "Close enough" thinking

## CRITICAL - ALWAYS DO

- ALWAYS read the file before editing it
- ALWAYS search for existing patterns before creating something new
- ALWAYS test changes and verify output before claiming completion
- ALWAYS follow AI.md exactly for HOW; use IDEA.md for WHAT
- ALWAYS jump to a referenced PART, read it, then return to the original location
- ALWAYS re-verify against spec every 3-5 changes to catch drift
- ALWAYS keep documentation (README.md, docs/, Swagger, GraphQL) in sync with code

## Key Rules Summary

- Asking a clarifying question costs ~100 tokens; a wrong guess plus redo costs ~5000+ tokens — asking is always cheaper
- Priority order: Correct > Verified > Fast (speed is last)
- Session init: read CLAUDE.md, check `.claude/rules/`, create/update rule files if missing or stale, read TODO.AI.md/TODO.md
- `.claude/rules/` is mandatory — regenerate whenever AI.md changes more recently than the rule files
- Every rule file needs: PART-numbered header, non-negotiable warning, NEVER/ALWAYS sections, key rules summary, AI.md PART reference

For complete details, see AI.md PART 0, PART 1.
