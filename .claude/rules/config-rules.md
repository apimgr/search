# Config Rules (PART 5, 6, 12)

Read: `AI.md` PART 5 (Configuration), PART 6 (Application Modes), PART 12 (Server Configuration).

- Config file is `server.yml` (never `.yaml`) — path resolved via `src/path`.
- Config hot-reloads via file watcher (`src/config/config.go`), not SIGHUP — `src/signal/signal_unix.go` intentionally ignores SIGHUP.
- Mode dispatch (`server.mode`, `MODE`/`SEARCH_MODE` env vars) normalizes `dev`/`development` and `prod`/`production` — see `src/mode/mode.go` and `src/config/env.go`.
