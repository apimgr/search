# Project Rules (PART 2, 3, 4)
<!-- Synchronized with AI.md revision f5ea82acc67d. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 2 (License & Attribution), PART 3 (Project Structure), PART 4 (OS-Specific Paths).

## CRITICAL - NEVER DO
- Use a GPL, AGPL, or LGPL licensed dependency; use MPL 2.0 only as an unmodified dependency and preserve its required notices
- Ship a direct `go.mod` dependency that is not attributed in `LICENSE.md`
- Embed or ship third-party datasets/content without recorded provenance, redistribution-compatible terms, attribution, and any required per-record source mapping
- Create a root file that is not on the PART 3 § Allowed Root Files list
- Create a root directory that is not on the PART 3 § Allowed Root Directories list
- Create `pkg/`, `internal/`, `cmd/`, `vendor/`, `node_modules/`, `utils/`, `lib/`, or `libs/`; `src/common/` is required by PART 7, 30, and 32
- Put runtime output (`config/`, `data/`, `logs/`, `tmp/`, `build/`, `dist/`) in the repo tree
- Hardcode an OS path anywhere outside the path-resolution package
- Use `{project_org}`/`{project_name}` for filesystem paths — PART 4 paths key on `{internal_org}`/`{internal_name}`
- Use plural Go package directories — `src/` dirs are singular (`src/config`, `src/mode`, `src/service`)
- Treat IDEA.md as implementation instructions or derive project variables from a Git slug/path when IDEA.md already defines them

## CRITICAL - ALWAYS DO
- Keep the license MIT and `LICENSE.md` in the repo root
- Attribute every direct dependency and preserve required notices; use the PART 2 format (full license text only when required, such as an Apache NOTICE)
- Include the required Goldmark and bluemonday dependencies for content rendering and sanitization (PART 3; handling rules are in `frontend-rules.md` / PART 16)
- Generate dependency license reports only in a project-scoped OS temporary directory; never put reports in the repository root
- Keep `src/` laid out per PART 3: `src/config/config.go`, `src/mode/mode.go`, `src/ssl/ssl.go`, `src/scheduler/scheduler.go`, `src/service/service.go`, `src/server/` with subdirs
- Keep `docker/` for Docker config, `docs/` for MkDocs only, `tests/` for test files and scripts
- Treat `PROMPT.md` as an optional human-owned root file: read and follow it, never rewrite or delete it
- Resolve config/data/cache/log/backup dirs per the PART 4 matrix for Linux, macOS, BSD, and Windows
- Preserve a migrated frozen `internal_name` exactly as shipped, even when mixed case; never normalize it in a filesystem path or service name
- Detect privileged vs non-privileged at runtime and pick the matching path set
- Support the Docker path layout: `/config/`, `/data/`, `/data/log/`, `/data/db/sqlite/`, `/data/backups/`
- Set the Go module path to `github.com/{project_org}/{internal_name}`; use the current latest stable Go toolchain
- Resolve project variables in one order everywhere: IDEA.md first, then the Git remote, then the project path; preserve frozen `internal_name` and `internal_org`
- Require release builds for Linux, macOS, Windows, and FreeBSD on amd64 and arm64; OpenBSD and NetBSD path rules apply if someone builds there, but those OSes are not required release targets
- Build and test Go code only in the project Docker image with `CGO_ENABLED=0`; use `casjaysdev/go:latest`
- Keep `.gitignore` and `.dockerignore`; the Docker ignore file excludes docs, tests, CI metadata, and generated outputs while retaining build inputs

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| License? | MIT, in `LICENSE.md` | PART 2 |
| GPL/AGPL/LGPL deps? | NEVER | PART 2 |
| `pkg/`, `internal/`, `cmd/`? | NEVER | PART 3 |
| Go package dir naming? | Singular | PART 3 |
| Path key? | `{internal_org}/{internal_name}` | PART 4 |
| Docker DB dir? | `/data/db/sqlite/` | PART 4 |
| Go module path? | `github.com/{project_org}/{internal_name}` | PART 3 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| `{internal_name}` | Frozen internal identifier used in filesystem paths |
| `{internal_org}` | Frozen internal org identifier used in filesystem paths |
| privileged | Running as root/Administrator — uses system-wide paths |
| non-privileged | Running as a user — uses `$HOME`-relative paths |

## QUICK REFERENCE
| Platform (privileged) | Config | Data | Log |
|-----------------------|--------|------|-----|
| Linux | `/etc/{org}/{name}` | `/var/lib/{org}/{name}` | `/var/log/{org}/{name}` |
| BSD | `/usr/local/etc/{org}/{name}` | `/var/db/{org}/{name}` | `/var/log/{org}/{name}` |
| macOS | `/Library/Application Support/{org}/{name}` | `.../data` | `/Library/Logs/{org}/{name}` |
| Windows | `%ProgramData%\{org}\{name}` | `...\data` | `...\logs` |
| Docker | `/config/{name}` | `/data/{name}` | `/data/log/{name}` |

---

For complete details, see AI.md PART 2, 3, 4
