# Makefile Rules (PART 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Run go/cargo/npm directly on host (always use Docker)
- Write coverage output to project tree (use /tmp/apimgr/search-XXXXXX/)
- Use $(pwd) in docker -v flags (use $(PWD) in Makefiles — Makefile variable, not subshell)
- Skip CGO_ENABLED=0 in build commands
- Use docker/Dockerfile.build for Go projects
- Hardcode architecture (target linux/amd64 + linux/arm64)

## CRITICAL - ALWAYS DO

- All Go commands inside Docker: docker run --rm -v "$(PWD):/workspace" -w /workspace casjaysdev/go:latest
- Coverage to temp: mkdir -p /tmp/$(PROJECTORG) && COVDIR=$$(mktemp -d /tmp/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX)
- CGO_ENABLED=0 on all build commands
- PROJECTNAME, PROJECTORG, BINARY, VERSION variables at top of Makefile
- make build, make test, make lint, make clean as minimum targets
- make install copies binary to /usr/local/bin/

## Required Makefile Variables

```makefile
PROJECTNAME := search
PROJECTORG  := apimgr
BINARY      := search
VERSION     := $(shell cat release.txt 2>/dev/null || echo "0.0.1")
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
```

## Docker Build Pattern

```makefile
build:
	docker run --rm \
		-v "$(PWD):/workspace" \
		-w /workspace \
		-e CGO_ENABLED=0 \
		-e GOOS=linux \
		-e GOARCH=amd64 \
		casjaysdev/go:latest \
		go build -ldflags="-s -w \
			-X github.com/apimgr/search/src/version.Version=$(VERSION) \
			-X github.com/apimgr/search/src/version.BuildDate=$(BUILD_DATE) \
			-X github.com/apimgr/search/src/version.Commit=$(COMMIT)" \
		-o binaries/$(BINARY) ./src/
```

## Coverage Pattern

```makefile
test:
	@mkdir -p "/tmp/$(PROJECTORG)"
	@COVDIR=$$(mktemp -d "/tmp/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX") && \
	docker run --rm \
		-v "$(PWD):/workspace" \
		-w /workspace \
		-e CGO_ENABLED=0 \
		casjaysdev/go:latest \
		sh -c "go test -coverprofile=$$COVDIR/coverage.out ./... && \
		       go tool cover -func=$$COVDIR/coverage.out"
```

## Required Targets

| Target | Description |
|--------|-------------|
| build | Compile binary for linux/amd64 |
| build-arm64 | Compile binary for linux/arm64 |
| test | Run tests with coverage |
| lint | Run golangci-lint |
| clean | Remove binaries/ artifacts |
| install | Copy binary to /usr/local/bin/ |
| docker-build | Build Docker image |
| release | Build all platforms + checksums |

For complete details, see AI.md PART 25
