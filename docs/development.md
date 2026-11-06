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

# Build local and all eight release platforms
make build

# Build a versioned local binary for production testing
make local

# Build Docker image
make docker
```

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
tests/run_tests.sh   # auto-detect environment
tests/docker.sh      # Docker
tests/incus.sh       # Incus
tests/e2e.sh         # browser end-to-end, on demand
```

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
