# Binary Rules (PART 7, 8, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 7 (Binary Requirements), PART 8 (Server Binary CLI), PART 32 (Client).

## CRITICAL - NEVER DO
- Enable CGO — `CGO_ENABLED=0` always, no exceptions
- Require an external runtime dependency, setup script, or installer
- Skip a build platform — all 8 (linux/darwin/windows/freebsd × amd64/arm64)
- Ship assets outside the binary — everything is embedded
- Require privileges for `--help` or `--version`
- Add, rename, or drop a PART 8 flag or subcommand
- Skip the client — `search-cli` is REQUIRED for every project, there is no skip case

## CRITICAL - ALWAYS DO
- Build two binaries: `search` (server) and `search-cli` (client), from `src/server` and `src/client`
- Let the binary handle ALL initialization: create directories on first run, set permissions from run context, work as root or user
- Implement the PART 8 flags exactly: `--help`, `--version`, `--config`, `--data`, `--cache`, `--log`, `--backup`, `--pid`, `--address`, `--port`, `--baseurl`, `--mode`, `--status`, `--daemon`, `--debug`, `--service`, `--maintenance`, `--update`
- Return exit 0 (healthy) / 1 (unhealthy) from `--status`
- Keep the client version identical to the server version
- Ship client CLI mode, TUI mode, and config mode, plus shell completions for bash, zsh, fish, and powershell
- Expose every server API operation through the client
- Store client config at `~/.config/{internal_org}/{internal_name}/cli.yml`, dark theme by default

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| CGO? | NEVER — `CGO_ENABLED=0` | PART 7 |
| Build platforms? | All 8 (4 OS × 2 arch) | PART 7 |
| Setup scripts? | NEVER — binary self-initializes | PART 7 |
| Client optional? | NEVER — REQUIRED for all projects | PART 32 |
| Client config file? | `cli.yml` (not `server.yml`) | PART 32 |
| Renaming the binary? | Allowed — the user may rename it | PART 8 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| server | `search` — main binary, runs as a service |
| client | `search-cli` — REQUIRED companion CLI/TUI |
| TUI mode | Interactive terminal UI mode of the client |

## QUICK REFERENCE
| Flag | Purpose |
|------|---------|
| `--config/--data/--cache/--log/--backup/--pid` | Path overrides |
| `--address/--port/--baseurl` | Listener + URL prefix (default `/`) |
| `--mode {production\|development\|debug}` | Shortcuts: prod, dev, devel |
| `--status` | Running status; exit 0/1 |
| `--daemon` / `--debug` | Detach / enable debug |
| `--service` / `--maintenance` / `--update` | Subcommand groups |

---

For complete details, see AI.md PART 7, 8, 32
