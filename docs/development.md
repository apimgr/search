# Development Guide

This guide covers how to contribute to Search development.

## Prerequisites

- Git
- Docker (required; Go commands run in `casjaysdev/go:latest`)
- Make

## Getting Started

### Clone the Repository

```bash
git clone https://github.com/apimgr/search.git
cd search
```

### Build

```bash
# Quick current-platform development build (writes to the OS temp directory)
make dev

# Build a versioned local binary for production testing
make local

# Build the local binaries plus all eight release platforms
make build

# Prepare and publish a stable release (requires X.Y.Z)
make release

# Regenerate the embedded OpenAPI JSON from src/swagger/annotations.go
docker run --rm --no-healthcheck \
  -v "$PWD:/app" \
  -w /app \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  casjaysdev/go:latest go generate ./src/swagger/...

# Build Docker image
make docker
```

OpenAPI and GraphQL artifacts are generated from their source annotations or
schema during build steps. Do not edit `src/swagger/openapi.json` by hand; run
`go generate ./src/swagger/...` after changing the Swagger annotations. CI runs the
same Go generator explicitly before tests, linting, and binary builds.

### Run in Development Mode

```bash
# Build into an OS temp directory for a quick check
make dev

# Build outputs are disposable; use the generated `Built:` path printed by make dev.
```

### Run Tests

```bash
make test
```

## Project Structure

```
search/
├── docker/              # Docker configuration
│   ├── Dockerfile
│   ├── docker-compose.yml
│   └── rootfs/          # Container filesystem overlay
├── docs/                # Documentation
├── scripts/             # Build and install scripts
├── src/                 # Go source code
│   ├── api/             # REST API handlers
│   ├── backup/          # Backup/restore functionality
│   ├── client/          # CLI client
│   ├── config/          # Configuration handling
│   ├── database/        # Database layer
│   ├── email/           # Email functionality
│   ├── geoip/           # GeoIP lookup
│   ├── graphql/         # GraphQL handlers
│   ├── instant/         # Instant answers
│   ├── logging/         # Logging system
│   ├── model/           # Data models
│   ├── scheduler/       # Task scheduler
│   ├── search/          # Search aggregation
│   │   ├── engine/      # Search engine implementations
│   │   └── bang/        # Bang command handling
│   ├── server/          # HTTP server
│   ├── service/         # Service management
│   ├── ssl/             # TLS/SSL handling
│   ├── widget/          # Search widgets
│   └── main.go          # Server entry point
├── tests/               # Integration tests
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Adding a New Search Engine

1. Add the implementation under `src/search/engine/` and satisfy the `search.Engine` interface in `src/search/engine.go`.
2. Register it in `src/search/engine/registry.go` and connect any operator-configurable settings through the existing engine configuration path.
3. Add parser tests using local fixtures in `src/search/engine/testdata/`; tests must not depend on live search providers.

## Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Add comments for exported functions
- Keep functions small and focused

### Linting

CI runs `go vet` and `staticcheck` inside the Go toolchain image. Run `make test` for the local Docker-based test suite.

## Testing

`make test` runs the Go package test suite in the maintained Go toolchain
container and enforces the project coverage threshold. It includes unit and
route tests, including locale consistency checks. It does not run the separate
system integration or browser E2E scripts.

```bash
make test
```

Environment-specific runners are available in `tests/`:

```bash
tests/run_tests.sh   # Docker integration suite
tests/docker.sh      # Docker
tests/incus.sh       # explicit Incus systemd integration suite
tests/e2e.sh         # browser end-to-end, on demand
```

`tests/docker.sh` creates a project-scoped temporary directory for its config,
data, and build output, plus an isolated Docker network, then removes them on
exit. Set `BINDIR` to retain build output in a caller-selected directory. It
checks Tor and I2P bootstrap;
when outbound network access is unavailable, those checks are skipped unless
`REQUIRE_OVERLAY=1` is set. `TOR_WAIT` and `I2P_WAIT` set the bootstrap wait
limits in seconds (defaults: 300 and 600).

## Pull Request Process

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes
4. Run tests: `make test`
5. Commit with a descriptive message
6. Push to your fork
7. Open a pull request

### Commit Message Format

```
type: short description

Longer description if needed.

Fixes #123
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

## Release Process

Releases are automated via GitHub Actions:

1. Update version in `release.txt`
2. Create a tag: `git tag v1.0.0`
3. Push the tag: `git push origin v1.0.0`
4. GitHub Actions will build and publish releases

## Docker Development

Use the Makefile targets for image builds. Development and test commands use
the maintained Go toolchain image; they do not require a local Go installation.
For the production and development image workflows, follow the repository's
Makefile targets and Docker documentation rather than invoking `docker build`
directly.

Both final images include `bash`, `curl`, `tor`, `i2pd`, `tini`, `tzdata`, and
CA certificates. `make docker` and CI run `tests/docker-image-check.sh` for each
production and development image. The check creates temporary config, data,
and database mounts, verifies required commands, then probes
`/api/v1/server/healthz` for JSON through a loopback-published port. It waits
up to `HEALTH_WAIT` seconds (30 by default) for the health endpoint and up to
`TOR_WAIT` seconds (180 by default)
for Tor health; without outbound network it reports a skip unless
`REQUIRE_OVERLAY=1`. `Dockerfile.dev` sets
`MODE=development`; the production image leaves `MODE` unset and uses the
server's production default.

For local builds, `site.txt` takes precedence as the explicit source for the
embedded official site URL; otherwise set `OFFICIAL_SITE` in the environment.
If neither is provided, the URL remains empty and the CLI requires `--server`.
Set `REGISTRY` to publish `make docker` output outside the default GHCR path.

| Make variable | Purpose | Default |
|---------------|---------|---------|
| `VERSION` | Local binary version; explicit environment value, then non-empty `release.txt`, then the short Git commit ID | Derived |
| `OFFICIAL_SITE` | Explicit embedded site URL when `site.txt` is absent | Empty |
| `REGISTRY` | Container image destination used by `make docker` | `ghcr.io/apimgr/search` |
| `BINDIR` | Local and cross-platform build output directory | `binaries/` |

CI/CD definitions are kept for GitHub Actions (`.github/workflows/`), GitLab
(`.gitlab-ci.yml`), Gitea (`.gitea/workflows/`), Forgejo
(`.forgejo/workflows/`), and Jenkins (`Jenkinsfile`). Each provider builds the
supported server and CLI platforms, runs the project checks, and publishes its
corresponding release or container artifacts.

## Documentation

Documentation is built with MkDocs:

```bash
# Install dependencies
pip install -r docs/requirements.txt

# Serve locally
mkdocs serve

# Build
mkdocs build
```

## Getting Help

- Open an issue on GitHub
- Check existing issues and discussions
- Read the documentation
