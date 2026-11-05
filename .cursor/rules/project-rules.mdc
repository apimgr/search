# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 2 (License & Attribution), PART 3 (Project Structure), PART 4 (OS-Specific Paths).

## CRITICAL - NEVER DO
- Use a GPL, AGPL, or LGPL licensed dependency — MIT-compatible only
- Ship a direct `go.mod` dependency that is not attributed in `LICENSE.md`
- Create a root file that is not on the PART 3 § Allowed Root Files list
- Create a root directory that is not on the PART 3 § Allowed Root Directories list
- Create `pkg/`, `internal/`, `cmd/`, `vendor/`, `node_modules/`, `utils/`, `common/`, `lib/`, or `libs/`
- Put runtime output (`config/`, `data/`, `logs/`, `tmp/`, `build/`, `dist/`) in the repo tree
- Hardcode an OS path anywhere outside the path-resolution package
- Use `{project_org}`/`{project_name}` for filesystem paths — PART 4 paths key on `{internal_org}`/`{internal_name}`
- Use plural Go package directories — `src/` dirs are singular (`src/config`, `src/mode`, `src/service`)

## CRITICAL - ALWAYS DO
- Keep the license MIT and `LICENSE.md` in the repo root
- List every embedded library license (compact table is fine for 10+ deps)
- Keep `src/` laid out per PART 3: `src/config/config.go`, `src/mode/mode.go`, `src/ssl/ssl.go`, `src/scheduler/scheduler.go`, `src/service/service.go`, `src/server/` with subdirs
- Keep `docker/` for Docker config, `docs/` for MkDocs only, `tests/` for test files and scripts
- Resolve config/data/cache/log/backup dirs per the PART 4 matrix for Linux, macOS, BSD, and Windows
- Detect privileged vs non-privileged at runtime and pick the matching path set
- Support the Docker path layout: `/config/`, `/data/`, `/data/log/`, `/data/db/sqlite/`, `/data/backups/`

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| License? | MIT, in `LICENSE.md` | PART 2 |
| GPL/AGPL/LGPL deps? | NEVER | PART 2 |
| `pkg/`, `internal/`, `cmd/`? | NEVER | PART 3 |
| Go package dir naming? | Singular | PART 3 |
| Path key? | `{internal_org}/{internal_name}` | PART 4 |
| Docker DB dir? | `/data/db/sqlite/` | PART 4 |

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
