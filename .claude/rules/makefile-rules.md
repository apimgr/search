# Makefile Rules (PART 25)

Read: `AI.md` PART 25 (Makefile — local dev only, NOT CI/CD).

- All toolchain commands run through Docker (`GO_DOCKER` wrapper in `Makefile`) — never `go` directly on host.
- Version precedence: `release.txt` > env/default fallback. Numeric semver gets a `v` prefix on the release tag; text versions (dev/beta/devel) and timestamps do not.
- `PROJECTNAME`/`PROJECTORG`/`BINARY` are always inferred from git remote or directory — never hardcoded.
