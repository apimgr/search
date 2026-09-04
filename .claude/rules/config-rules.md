# Config Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 5 (Configuration), PART 6 (Application Modes), PART 12 (Server Configuration).

## CRITICAL - NEVER DO
- Name the config file `server.yaml` or `server.json` — it is always `server.yml`
- Silently ignore an unknown config key — unknown keys are ERRORS
- Ship a config value without a sane default
- Use SIGHUP for reload — `src/signal/signal_unix.go` intentionally ignores SIGHUP; reload is driven by the file watcher
- Enable `/debug/*` endpoints (pprof, expvar) from a mode — only the debug flag enables them
- Enable debug endpoints in development mode
- Use a config library outside the PART 5 approved list
- Hardcode a config path — resolve it through the path package

## CRITICAL - ALWAYS DO
- Honor the precedence chain: CLI flags > env vars > config file > defaults
- Use the `{PROJECT_NAME}_` environment prefix
- Accept all boolean spellings: `true/false`, `yes/no`, `1/0`, `on/off`, `enable/disable`
- Validate the whole config on load and fail loudly on error
- Hot-reload safe settings via the config file watcher (`src/config/config.go`)
- Document clearly which settings require a restart
- Default to production mode; normalize `prod`/`production` and `dev`/`development`
- Keep every server setting in `server.yml` (PART 12), including Valkey/Redis when used

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Config filename? | `server.yml` (never `.yaml`) | PART 5 |
| Unknown keys? | ERROR, never ignored | PART 5 |
| Reload mechanism? | File watcher, NOT SIGHUP | PART 5, 12 |
| Default mode? | production | PART 6 |
| What enables `/debug/*`? | The debug flag only | PART 6 |
| Does `--mode debug` set debug? | Yes, it defaults the debug flag on | PART 6 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| mode | `production` \| `development` \| `debug` (`server.mode`, `MODE`/`SEARCH_MODE`) |
| debug flag | Independent switch that gates `/debug/*`; set by `--debug` or `DEBUG` |
| hot-reload | Applying a changed `server.yml` without restarting the process |

## QUICK REFERENCE
| Layer | Source | Wins over |
|-------|--------|-----------|
| 1 | CLI flags | everything |
| 2 | Env vars (`{PROJECT_NAME}_`) | file, defaults |
| 3 | `server.yml` | defaults |
| 4 | Built-in defaults | — |

- Mode dispatch: `src/mode/mode.go`, `src/config/env.go`
- Config load/watch: `src/config/config.go`

---

For complete details, see AI.md PART 5, 6, 12
