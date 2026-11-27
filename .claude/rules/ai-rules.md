# AI Rules (PART 0, 1)
<!-- Synchronized with AI.md revision c2f65bf274b5. -->

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
- Treat AI.md as read-only and IDEA.md as product intent; a project feature with no AI.md contract needs an explicit SPEC.md contract, while SPEC.md exceptions require the user's approval
- Stop and ask before changing product behavior, overriding AI.md, or guessing; do not place implementation exceptions in IDEA.md
- Treat security as the first requirement: validate input type, length, format, and range on the server; reject unknown fields/keys; sanitize for the output context; never weaken security to make a feature work
- Preserve an existing mixed-case `internal_name` during migration; lowercase applies only to newly chosen values, because changing a frozen name moves persisted paths and service identities
- Keep secrets and internal details out of client errors and ordinary logs; use the audience-appropriate error detail required by PART 1
- Keep README content in the PART 1 order, use `https://scour.li` for public URLs, and use `curl -q -LSsf` in documented curl examples
- Do not add AI attribution, generated-by text, or co-author trailers
- Follow the explicit question rule: an interrogative or a message ending in `?` is a question to answer, not an action to execute
- Use the repo's human-owned `PROMPT.md` as instructions when present; never rewrite or delete it
- Keep all ordinary work on `main`; temporary branches are local only, never pushed. Before commit/push, verify `git branch --show-current` is `main`

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Config/backup password hash? | Argon2id (NEVER bcrypt) | PART 11 |
| Where is Dockerfile? | `docker/Dockerfile` (NEVER root) | PART 26 |
| CGO enabled? | NEVER (CGO_ENABLED=0 always) | PART 7 |
| Premium features? | NEVER (all features free) | PART 1 |
| External cron? | NEVER (built-in scheduler) | PART 18 |
| Client-side rendering? | NEVER (server-side Go templates) | PART 16 |
| SQL wildcard selection? | NEVER use `SELECT *`; name the required columns | PART 1 |
| A feature requires behavior not specified by AI.md? | Use IDEA.md for WHAT and, only after approval, SPEC.md for its technical contract or an explicit AI.md exception | PART 0 |
| How do ordinary code changes reach the remote? | Via `gitcommit --dir {project_dir} all`; maintainers/AI push to `main` and open no PR (release workflows handle tags separately) | PART 0 |

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
| `TODO.AI.md` | AI-owned implementation backlog; remove completed items in the same successful commit that resolves them |
| `TODO.md`, `PLAN.md`, `PROMPT.md` | Human-owned files: mark TODO/PLAN items done in place where permitted; read PROMPT; never rewrite or delete |

Reading a PART: `grep -n "^# PART N" AI.md`, then read only that slice.

## COMPLIANCE CHECK
Before completing ANY task:
- [ ] Read relevant PART(s) in AI.md
- [ ] Implementation matches spec EXACTLY
- [ ] No guessing - all decisions from spec
- [ ] Docs updated if code changed
- [ ] Review the complete diff; run test and lint gates once in the primary session
- [ ] Confirm branch is `main`; write and re-read `.git/COMMIT_MESS`; finish only after successful `gitcommit` push and green CI

---

For complete details, see AI.md PART 0, 1
