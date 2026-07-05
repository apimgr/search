# Project Rules (PART 2, 3, 4)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Use GPL, AGPL, or LGPL dependencies
- Use mattn/go-sqlite3 (CGO required — forbidden)
- Use spf13/viper (must use gopkg.in/yaml.v3 directly)
- Use robfig/cron (must use github.com/go-co-op/gocron/v2)
- Use gorilla/mux (archived) or dgrijalva/jwt-go
- Enable CGO (CGO_ENABLED must be 0)
- Run go/cargo/npm on host — always use Docker
- Create docker/Dockerfile.build for Go projects (casjaysdev/go:latest is comprehensive)
- Use $(pwd) in docker -v flags (use $PWD instead)
- Write coverage/test output to the project tree

## CRITICAL - ALWAYS DO

- License: MIT (with embedded third-party attribution for 10+ deps)
- CGO_ENABLED=0 — all libraries must be pure Go
- Build in Docker: docker run --rm -v "$PWD:/workspace" -w /workspace casjaysdev/go:latest
- Coverage output to /tmp/apimgr/search-XXXXXX/ never to project tree
- Target linux/amd64 + linux/arm64
- Use $PWD (not $(pwd)) in shell docker -v flags; $(PWD) is correct in Makefiles

## Required Libraries

| Library | Purpose |
|---------|---------|
| github.com/go-chi/chi/v5 | HTTP router |
| modernc.org/sqlite | Pure Go SQLite |
| github.com/go-co-op/gocron/v2 | Task scheduler |
| gopkg.in/yaml.v3 | Config parsing |
| github.com/go-playground/validator/v10 | Input validation |
| github.com/rs/cors | CORS handling |
| golang.org/x/time/rate | Rate limiting |

## Forbidden Libraries

| Library | Reason | Replace With |
|---------|--------|-------------|
| mattn/go-sqlite3 | Requires CGO | modernc.org/sqlite |
| spf13/viper | Must use yaml.v3 directly | gopkg.in/yaml.v3 |
| robfig/cron | Wrong scheduler | github.com/go-co-op/gocron/v2 |
| gorilla/mux | Archived | github.com/go-chi/chi/v5 |
| dgrijalva/jwt-go | Security issues | golang-jwt/jwt |

## Project Structure

```
search/
├── binaries/       # Compiled binaries (gitignored)
├── scripts/        # Shell scripts and tooling
├── tests/          # Test files and fixtures
├── docker/         # Docker configs (no Dockerfile.build for Go)
├── src/            # Go source code
│   ├── main.go     # Entry point
│   ├── config/     # Configuration (singular — Go package names)
│   ├── server/     # HTTP server
│   ├── mode/       # Application mode detection
│   └── ...
└── docs/           # Documentation
```

## OS-Specific Paths

### Linux (Privileged / Service)
- Config: /etc/apimgr/search/server.yml
- Data: /var/lib/apimgr/search/
- Cache: /var/cache/apimgr/search/
- Logs: /var/log/apimgr/search/
- Backups: /var/lib/Backups/apimgr/search/

### Linux (User)
- Config: ~/.config/apimgr/search/server.yml
- Data: ~/.local/share/apimgr/search/
- Cache: ~/.cache/apimgr/search/

### Docker
- Config: /config/search/server.yml
- Data: /data/search/

## .gitignore Header Format

Line 1: `# gitignore created on MM/DD/YY at HH:MM`
Line 2: `ignoredirmessage`

## Official Site

https://search.apimgr.us

For complete details, see AI.md PART 2, 3, 4
