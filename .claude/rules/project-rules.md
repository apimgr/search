# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER use GPL/AGPL/LGPL licensed dependencies (copyleft) - avoid entirely
- NEVER omit LICENSE.md from project root
- NEVER hardcode `{project_name}` or `{project_org}` - always infer from git remote or path
- NEVER change `{internal_name}` after first-time setup - it is frozen forever
- NEVER assume current working directory is project root
- NEVER put user-editable config in `{data_dir}` or app-managed data in `{config_dir}`
- NEVER commit `binaries/`, `releases/`, `docker/rootfs/`, or `volumes/`
- NEVER use `.yaml` extension for the config file - it is always `server.yml`
- NEVER use `/data/**` or `/config/**` paths outside Docker containers
- NEVER put inline comments in YAML - comments go ABOVE the setting only

## CRITICAL - ALWAYS DO

- ALWAYS include `LICENSE.md` (MIT) in project root
- ALWAYS embed third-party dependency licenses in LICENSE.md
- ALWAYS include a license badge in README.md
- ALWAYS support all 4 OSes: Linux, BSD, macOS, Windows
- ALWAYS support both AMD64 and ARM64 architectures
- ALWAYS use latest stable Go version (never pin in docs/CI/Docker)
- ALWAYS use `modernc.org/sqlite` (pure Go), never `mattn/go-sqlite3` (CGO)
- ALWAYS keep all paths relative to project root, never `cwd`
- ALWAYS determine project root via `git rev-parse --show-toplevel` or programmatically

## Key Rules Summary

**License:**
- MIT License required; file = `LICENSE.md`; copyright holder = `{project_org}` or org/individual name
- MIT/Apache2.0/BSD/ISC deps: attribution required (compact table format recommended for 10+ deps)
- BSD-3-Clause: include the non-endorsement clause text
- Apache 2.0: include NOTICE file content if the library has one
- Docker license metadata via `--annotation "org.opencontainers.image.licenses=MIT"`, not a Dockerfile LABEL
- Automate license checks in CI with `go-licenses` (pre-installed in `casjaysdev/go:latest`)

**Variables:**
- `{project_name}` (lowercase, may change on rename) vs `{internal_name}` (lowercase, frozen forever)
- UPPER_SNAKE_CASE forms (`{PROJECT_NAME}`) = uppercase rendering, used for env vars/Makefile
- `{plist_name}` = `io.github.{project_org}.{internal_name}` (macOS)
- Recommended local path: `~/Projects/{gitprovider}/{project_org}/{internal_name}` (not required)

**Directory layout (required root items):**
- `.github/workflows/` or `.gitea/workflows/` (release.yml, beta.yml, daily.yml, docker.yml)
- `CLAUDE.md`, `.claude/rules/*.md` (12 rule files incl. this one), `.claude/settings.json`
- `docs/` (MkDocs: index, installation, configuration, api, cli, security, integrations, development)
- `src/`, `scripts/`, `tests/` (run_tests.sh, docker.sh, incus.sh - required)
- `docker/` (Dockerfile, Dockerfile.dev, docker-compose*.yml, `rootfs/` overlay - gitignored)
- `volumes/`, `binaries/`, `releases/` - all gitignored
- Root files: `README.md`, `LICENSE.md`, `AI.md`, `TODO.AI.md`, `TODO.md`, `PLAN.AI.md`, `PLAN.md`, `Jenkinsfile`, `release.txt`, `site.txt`
- `.gitignore` must start with `# gitignore created on MM/DD/YY at HH:MM` then literal `ignoredirmessage`
- `.dockerignore` must exclude `.git/`, CI dirs, `volumes/`, `binaries/`, `releases/`, `tests/`, `docs/`, `*.md`, `Makefile`, IDE files, AI config dirs; must NOT exclude `src/`, `go.mod`/`go.sum`, `docker/`

**Runtime directory purpose:**
- `{config_dir}` = user-editable · `{data_dir}` = app-managed · `{log_dir}` = logs · `{backup_dir}` = backups

**OS-specific paths (privileged / user):**
- Linux: `/etc/{internal_org}/{internal_name}/` · `~/.config/{internal_org}/{internal_name}/`
- macOS: `/Library/Application Support/{internal_org}/{internal_name}/` · `~/Library/Application Support/...`
- BSD: `/usr/local/etc/{internal_org}/{internal_name}/` · `~/.config/{internal_org}/{internal_name}/`
- Windows: `%ProgramData%\{internal_org}\{internal_name}\` · `%AppData%\{internal_org}\{internal_name}\`
- Docker (container-only): `/config/{project_name}/`, `/data/{project_name}/`, port `80`
- Config file is always named `server.yml` on every OS
- SQLite DB always lives under a `db/` subdir as `server.db`

**Go module rules:**
- `CGO_ENABLED=0` required for all libraries; forbidden: `mattn/go-sqlite3`, `lib/pq`, `ooni/go-libtor`, `dgrijalva/jwt-go`, `gorilla/mux`, `go-redis/redis` (old path)
- Build/test only in `casjaysdev/go:latest`; never `setup-go`, never pinned tags
- Never run `go` directly - use Makefile targets

For complete details, see AI.md PART 2, 3, 4.
