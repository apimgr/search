# Binary Rules (PART 7, 8, 32)

Read: `AI.md` PART 7 (Binary Requirements), PART 8 (Server Binary CLI), PART 32 (Client).

- `CGO_ENABLED=0` always — no CGO, ever.
- Two binaries: `search` (server) and `search-cli` (client) — see `src/server`, `src/client`.
- Server CLI flags and subcommands must match PART 8 exactly.
- Client supports API mode, config mode, and TUI mode per PART 32.
