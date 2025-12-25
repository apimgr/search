PROJECT := search
ORG := apimgr

# Version: env var > release.txt > default
VERSION ?= $(shell cat release.txt 2>/dev/null || echo "0.1.0")

# Build info - use TZ env var or system timezone
# Format: "Thu Dec 17, 2025 at 18:19:24 EST"
BUILD_DATE := $(shell date +"%a %b %d, %Y at %H:%M:%S %Z")
COMMIT_ID := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
# COMMIT_ID used directly - no VCS_REF alias

# Linker flags to embed build info
LDFLAGS := -s -w \
	-X 'github.com/$(ORG)/$(PROJECT)/src/config.Version=$(VERSION)' \
	-X 'github.com/$(ORG)/$(PROJECT)/src/config.CommitID=$(COMMIT_ID)' \
	-X 'github.com/$(ORG)/$(PROJECT)/src/config.BuildDate=$(BUILD_DATE)'

# CLI linker flags
CLI_LDFLAGS := -s -w \
	-X 'github.com/$(ORG)/$(PROJECT)/src/client/cmd.ProjectName=$(PROJECT)' \
	-X 'github.com/$(ORG)/$(PROJECT)/src/client/cmd.Version=$(VERSION)' \
	-X 'github.com/$(ORG)/$(PROJECT)/src/client/cmd.CommitID=$(COMMIT_ID)' \
	-X 'github.com/$(ORG)/$(PROJECT)/src/client/cmd.BuildDate=$(BUILD_DATE)' \
	-X 'github.com/$(ORG)/$(PROJECT)/src/client/api.ProjectName=$(PROJECT)' \
	-X 'github.com/$(ORG)/$(PROJECT)/src/client/api.Version=$(VERSION)'

# Directories
BINDIR := ./binaries
RELDIR := ./releases

# Go module cache (persistent across builds)
GOCACHE := $(HOME)/.cache/go-build
GOMODCACHE := $(HOME)/go/pkg/mod

# Build targets
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64 freebsd/arm64

# Docker
REGISTRY := ghcr.io/$(ORG)/$(PROJECT)
GO_DOCKER := docker run --rm \
	-v $(PWD):/build \
	-v $(GOCACHE):/root/.cache/go-build \
	-v $(GOMODCACHE):/go/pkg/mod \
	-w /build \
	-e CGO_ENABLED=0 \
	golang:alpine

.PHONY: dev build release docker test clean

# =============================================================================
# DEV - Quick dev build to temp directory (per TEMPLATE.md PART 11)
# =============================================================================
# Outputs to /tmp/apimgr.XXXXXX/search for quick testing
# ALWAYS uses Docker for building - host has NO Go installed
# Test using Docker (quick) or Incus (full systemd environment)
dev:
	@mkdir -p $(GOCACHE) $(GOMODCACHE) $(BINDIR)
	@DEVDIR=$$(mktemp -d /tmp/$(ORG).XXXXXX); \
	echo "Building dev binary to $$DEVDIR/$(PROJECT) (Docker)..."; \
	$(GO_DOCKER) go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/.dev-$(PROJECT) ./src; \
	mv $(BINDIR)/.dev-$(PROJECT) "$$DEVDIR/$(PROJECT)"; \
	if [ -d "src/client" ]; then \
		echo "Building dev CLI to $$DEVDIR/$(PROJECT)-cli..."; \
		$(GO_DOCKER) go build -ldflags "$(CLI_LDFLAGS)" -o $(BINDIR)/.dev-$(PROJECT)-cli ./src/client; \
		mv $(BINDIR)/.dev-$(PROJECT)-cli "$$DEVDIR/$(PROJECT)-cli"; \
	fi; \
	echo ""; \
	echo "Built: $$DEVDIR/$(PROJECT)"; \
	echo ""; \
	echo "Test (Docker - quick):"; \
	echo "  docker run --rm -v $$DEVDIR:/app alpine:latest /app/$(PROJECT) --help"; \
	echo "  docker run --rm -v $$DEVDIR:/app alpine:latest /app/$(PROJECT) --version"; \
	echo "  docker run --rm -p 8080:80 -v $$DEVDIR:/app alpine:latest /app/$(PROJECT)"; \
	echo ""; \
	echo "Test (Incus - full systemd):"; \
	echo "  incus file push $$DEVDIR/$(PROJECT) <container>/usr/local/bin/$(PROJECT)"; \
	echo "  incus exec <container> -- $(PROJECT) --help"

# =============================================================================
# BUILD - Build all platforms + host binary (via Docker with cached modules)
# =============================================================================
build: clean
	@mkdir -p $(BINDIR)
	@echo "Building version $(VERSION)..."
	@mkdir -p $(GOCACHE) $(GOMODCACHE)

	# Download modules first (cached)
	@echo "Downloading Go modules..."
	@$(GO_DOCKER) go mod download

	# Build for host OS/ARCH
	@echo "Building host binary..."
	@$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
		go build -ldflags \"$(LDFLAGS)\" -o $(BINDIR)/$(PROJECT) ./src"

	# Build all platforms (server)
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUTPUT=$(BINDIR)/$(PROJECT)-$$OS-$$ARCH; \
		[ "$$OS" = "windows" ] && OUTPUT=$$OUTPUT.exe; \
		echo "Building $$OS/$$ARCH..."; \
		$(GO_DOCKER) sh -c "GOOS=$$OS GOARCH=$$ARCH \
			go build -ldflags \"$(LDFLAGS)\" \
			-o $$OUTPUT ./src" || exit 1; \
	done

	# Build CLI if src/client exists
	@if [ -d "src/client" ]; then \
		echo "Building CLI..."; \
		$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
			go build -ldflags \"$(CLI_LDFLAGS)\" -o $(BINDIR)/$(PROJECT)-cli ./src/client"; \
		for platform in $(PLATFORMS); do \
			OS=$${platform%/*}; \
			ARCH=$${platform#*/}; \
			OUTPUT=$(BINDIR)/$(PROJECT)-$$OS-$$ARCH-cli; \
			[ "$$OS" = "windows" ] && OUTPUT=$(BINDIR)/$(PROJECT)-$$OS-$$ARCH-cli.exe; \
			echo "Building CLI $$OS/$$ARCH..."; \
			$(GO_DOCKER) sh -c "GOOS=$$OS GOARCH=$$ARCH \
				go build -ldflags \"$(CLI_LDFLAGS)\" \
				-o $$OUTPUT ./src/client" || exit 1; \
		done; \
	fi

	@echo "Build complete: $(BINDIR)/"

# =============================================================================
# RELEASE - Manual local release (stable only)
# =============================================================================
release: build
	@mkdir -p $(RELDIR)
	@echo "Preparing release $(VERSION)..."

	# Create version.txt
	@echo "$(VERSION)" > $(RELDIR)/version.txt

	# Copy server binaries to releases
	@for f in $(BINDIR)/$(PROJECT)-*; do \
		[ -f "$$f" ] || continue; \
		echo "$$f" | grep -q "\-cli" && continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done

	# Copy CLI binaries to releases (if they exist)
	@for f in $(BINDIR)/$(PROJECT)-*-cli $(BINDIR)/$(PROJECT)-*-cli.exe; do \
		[ -f "$$f" ] || continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done

	# Create source archive (exclude VCS and build artifacts)
	@tar --exclude='.git' --exclude='.github' --exclude='.gitea' \
		--exclude='binaries' --exclude='releases' --exclude='*.tar.gz' \
		-czf $(RELDIR)/$(PROJECT)-$(VERSION)-source.tar.gz .

	# Delete existing release/tag if exists
	@gh release delete $(VERSION) --yes 2>/dev/null || true
	@git tag -d $(VERSION) 2>/dev/null || true
	@git push origin :refs/tags/$(VERSION) 2>/dev/null || true

	# Create new release (stable)
	@gh release create $(VERSION) $(RELDIR)/* \
		--title "$(PROJECT) $(VERSION)" \
		--notes "Release $(VERSION)" \
		--latest

	@echo "Release complete: $(VERSION)"

# =============================================================================
# DOCKER - Build and push container to ghcr.io
# =============================================================================
# Uses multi-stage Dockerfile - Go compilation happens inside Docker
# No pre-built binaries needed
docker:
	@echo "Building Docker image $(VERSION)..."

	# Ensure buildx is available
	@docker buildx version > /dev/null 2>&1 || (echo "docker buildx required" && exit 1)

	# Create/use builder
	@docker buildx create --name $(PROJECT)-builder --use 2>/dev/null || \
		docker buildx use $(PROJECT)-builder

	# Build and push multi-arch (multi-stage Dockerfile handles Go compilation)
	@docker buildx build \
		-f ./docker/Dockerfile \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--build-arg COMMIT_ID="$(COMMIT_ID)" \
		-t $(REGISTRY):$(VERSION) \
		-t $(REGISTRY):latest \
		--push \
		.

	@echo "Docker push complete: $(REGISTRY):$(VERSION)"

# =============================================================================
# TEST - Run all tests (via Docker with cached modules)
# =============================================================================
test:
	@echo "Running tests in Docker..."
	@mkdir -p $(GOCACHE) $(GOMODCACHE)
	@$(GO_DOCKER) go mod download
	@$(GO_DOCKER) go test -v -cover ./...
	@echo "Tests complete"

# =============================================================================
# CLEAN - Remove build artifacts
# =============================================================================
clean:
	@rm -rf $(BINDIR) $(RELDIR)
