# AI Rules (PART 0, 1)

Read: `AI.md` PART 0 (AI Assistant Rules), PART 1 (Critical Rules).

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Guess or assume - READ THE SPEC or ASK
- Implement without reading relevant PART first
- Modify AI.md PART content (read-only spec)
- Add features not in spec without asking
- Use "I think" or "probably" - KNOW from spec or ASK
- Use generic placeholder content ("Your app name", "Feature 1")
- Leave TODO comments in code - implement fully or don't implement
- Create stub functions or "future" placeholders
- Partial implementations - every feature must be 100% complete

## CRITICAL - ALWAYS DO
- Read relevant PART before implementing ANY feature
- Search AI.md before asking questions (answer is likely there)
- Follow spec EXACTLY - no "improvements" without approval
- Update IDEA.md when features change
- Keep all docs in sync with code
- When unsure, ASK - never guess or assume
- Implement features 100% complete - no stubs, no TODOs, no "future"
- Return after cross-references - a "See PART X" jump never replaces the rest of the PART/section you were reading; read it, then continue

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Config/backup password hash? | Argon2id (NEVER bcrypt) | PART 11 |
| Where is Dockerfile? | `docker/Dockerfile` (NEVER root) | PART 26 |
| CGO enabled? | NEVER (CGO_ENABLED=0 always) | PART 7 |
| Premium features? | NEVER (all features free) | PART 1 |
| External cron? | NEVER (built-in scheduler) | PART 18 |
| Client-side rendering? | NEVER (server-side Go templates) | PART 16 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| server | Main binary `search` - runs as service |
| client | CLI binary `search-cli` - REQUIRED |
| Operator | Person who deploys and manages the server via CLI and `server.yml` |

## QUICK REFERENCE
| File | Role |
|------|------|
| `AI.md` | THE HOW — read-only implementation spec, source of truth |
| `IDEA.md` | THE WHAT — product definition and project variables |
| `CLAUDE.md` | Short loader, auto-read every conversation |
| `.claude/rules/*.md` | Per-topic condensed rules pointing back at PARTs |
| `TODO.AI.md` | Current implementation backlog |

Reading a PART: `grep -n "^# PART N" AI.md`, then read only that slice.

## COMPLIANCE CHECK
Before completing ANY task:
- [ ] Read relevant PART(s) in AI.md
- [ ] Implementation matches spec EXACTLY
- [ ] No guessing - all decisions from spec
- [ ] Docs updated if code changed

---

For complete details, see AI.md PART 0, 1
