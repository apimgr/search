# Development Guide

This guide covers how to contribute to Search development.

## Prerequisites

- Go 1.21 or later
- Git
- Docker (optional, for testing)
- Make

## Getting Started

### Clone the Repository

```bash
git clone https://github.com/apimgr/search.git
cd search
```

### Build

```bash
# Build for current platform
make build

# Build all platforms
make release

# Build Docker image
make docker
```

### Run in Development Mode

```bash
# Run directly
make dev

# Or run the binary
./binaries/search --mode development
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
│   ├── admin/           # Admin panel handlers
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
│   ├── models/          # Data models
│   ├── scheduler/       # Task scheduler
│   ├── search/          # Search aggregation
│   │   ├── engines/     # Search engine implementations
│   │   └── bangs/       # Bang command handling
│   ├── server/          # HTTP server
│   ├── service/         # Service management
│   ├── tls/             # TLS/SSL handling
│   ├── users/           # User management
│   ├── widgets/         # Dashboard widgets
│   └── main.go          # Entry point
├── tests/               # Integration tests
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

## Adding a New Search Engine

1. Create a new file in `src/search/engines/`:

```go
package engines

type MyEngine struct {
    BaseEngine
}

func NewMyEngine() *MyEngine {
    return &MyEngine{
        BaseEngine: BaseEngine{
            name:       "myengine",
            categories: []Category{CategoryGeneral},
            enabled:    true,
            priority:   50,
        },
    }
}

func (e *MyEngine) Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
    // Implement search logic
    return results, nil
}
```

2. Register the engine in `src/search/engines/registry.go`:

```go
func DefaultRegistry() *Registry {
    r := NewRegistry()
    // ... existing engines ...
    r.Register(NewMyEngine())
    return r
}
```

## Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Add comments for exported functions
- Keep functions small and focused

### Linting

```bash
make lint
```

## Testing

### Unit Tests

```bash
go test ./src/...
```

### Integration Tests

```bash
make test-integration
```

### Coverage

```bash
make coverage
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

### Build Image

```bash
docker build -t search:dev -f docker/Dockerfile .
```

### Run Container

```bash
docker run -p 64580:80 search:dev
```

### Docker Compose

```bash
docker compose -f docker/docker-compose.yml up -d
```

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
