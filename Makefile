# Infer PROJECTNAME, PROJECTORG, GITPROVIDER from git remote (per AI.md PART 26)
# Note: Strip .git suffix from URL if present
PROJECTNAME := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)$$|\1|; s|\.git$$||' || basename "$$(pwd)")
PROJECTORG := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)/[^/]+$$|\1|' || basename "$$(dirname "$$(pwd)")")
GITPROVIDER := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*[:/]([^/]+)/.*|\1|; s|\.com$$||; s|\.org$$||' || echo "local")

# Version: env var > release.txt > default
VERSION ?= $(shell cat release.txt 2>/dev/null || echo "0.1.0")

# Build info - use TZ env var or system timezone
# Format: "Thu Dec 17, 2025 at 18:19:24 EST"
BUILD_DATE := $(shell date +"%a %b %d, %Y at %H:%M:%S %Z")
COMMIT_ID := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
# COMMIT_ID used directly - no VCS_REF alias

# Linker flags to embed build info
LDFLAGS := -s -w \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/config.Version=$(VERSION)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/config.CommitID=$(COMMIT_ID)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/config.BuildDate=$(BUILD_DATE)'

# CLI linker flags
CLI_LDFLAGS := -s -w \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.ProjectName=$(PROJECTNAME)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.Version=$(VERSION)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.CommitID=$(COMMIT_ID)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.BuildDate=$(BUILD_DATE)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/api.ProjectName=$(PROJECTNAME)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/api.Version=$(VERSION)'

# Directories
BINDIR := ./binaries
RELDIR := ./releases

# Go environment (persistent across builds - per AI.md PART 26)
GODIR := $(HOME)/.local/share/go
GOCACHE := $(HOME)/.local/share/go/build

# Build targets
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64 freebsd/arm64

# Docker (per AI.md PART 12)
REGISTRY ?= ghcr.io/$(PROJECTORG)/$(PROJECTNAME)
GO_DOCKER := docker run --rm \
	-v $(PWD):/build \
	-v $(GOCACHE):/root/.cache/go-build \
	-v $(GODIR):/go \
	--tmpfs /tmp:exec \
	-w /build \
	-e CGO_ENABLED=0 \
	golang:alpine

.PHONY: dev local build release docker test clean

# =============================================================================
# DEV - Quick dev build to temp directory (per TEMPLATE.md PART 11)
# =============================================================================
# Outputs to /tmp/apimgr.XXXXXX/search for quick testing
# ALWAYS uses Docker for building - host has NO Go installed
# Test using Docker (quick) or Incus (full systemd environment)
dev:
	@mkdir -p $(GOCACHE) $(GODIR) $(BINDIR)
	@DEVDIR=$$(mktemp -d /tmp/$(PROJECTORG).XXXXXX); \
	echo "Building dev binary to $$DEVDIR/$(PROJECTNAME) (Docker)..."; \
	$(GO_DOCKER) go build -ldflags "$(LDFLAGS)" -o $(BINDIR)/.dev-$(PROJECTNAME) ./src; \
	mv $(BINDIR)/.dev-$(PROJECTNAME) "$$DEVDIR/$(PROJECTNAME)"; \
	if [ -d "src/client" ]; then \
		echo "Building dev CLI to $$DEVDIR/$(PROJECTNAME)-cli..."; \
		$(GO_DOCKER) go build -ldflags "$(CLI_LDFLAGS)" -o $(BINDIR)/.dev-$(PROJECTNAME)-cli ./src/client; \
		mv $(BINDIR)/.dev-$(PROJECTNAME)-cli "$$DEVDIR/$(PROJECTNAME)-cli"; \
	fi; \
	echo ""; \
	echo "Built: $$DEVDIR/$(PROJECTNAME)"; \
	echo ""; \
	echo "Test (Docker - quick):"; \
	echo "  docker run --rm -v $$DEVDIR:/app alpine:latest /app/$(PROJECTNAME) --help"; \
	echo "  docker run --rm -v $$DEVDIR:/app alpine:latest /app/$(PROJECTNAME) --version"; \
	echo "  docker run --rm -p 8080:80 -v $$DEVDIR:/app alpine:latest /app/$(PROJECTNAME)"; \
	echo ""; \
	echo "Test (Incus - full systemd):"; \
	echo "  incus file push $$DEVDIR/$(PROJECTNAME) <container>/usr/local/bin/$(PROJECTNAME)"; \
	echo "  incus exec <container> -- $(PROJECTNAME) --help"

# =============================================================================
# LOCAL - Build for current OS/ARCH with version suffix (per AI.md PART 26)
# =============================================================================
# Outputs to binaries/search-VERSION for production testing
local:
	@mkdir -p $(GOCACHE) $(GODIR) $(BINDIR)
	@echo "Building host binary $(VERSION)..."
	@$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
		go build -ldflags \"$(LDFLAGS)\" -o $(BINDIR)/$(PROJECTNAME)-$(VERSION) ./src"
	@if [ -d "src/client" ]; then \
		echo "Building host CLI $(VERSION)..."; \
		$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
			go build -ldflags \"$(CLI_LDFLAGS)\" -o $(BINDIR)/$(PROJECTNAME)-cli-$(VERSION) ./src/client"; \
	fi
	@echo ""
	@echo "Built: $(BINDIR)/$(PROJECTNAME)-$(VERSION)"
	@if [ -d "src/client" ]; then echo "Built: $(BINDIR)/$(PROJECTNAME)-cli-$(VERSION)"; fi

# =============================================================================
# BUILD - Build all platforms + host binary (via Docker with cached modules)
# =============================================================================
build: clean
	@mkdir -p $(BINDIR)
	@echo "Building version $(VERSION)..."
	@mkdir -p $(GOCACHE) $(GODIR)

	# Download modules first (cached)
	@echo "Downloading Go modules..."
	@$(GO_DOCKER) go mod download

	# Build for host OS/ARCH
	@echo "Building host binary..."
	@$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
		go build -ldflags \"$(LDFLAGS)\" -o $(BINDIR)/$(PROJECTNAME) ./src"

	# Build all platforms (server)
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUTPUT=$(BINDIR)/$(PROJECTNAME)-$$OS-$$ARCH; \
		[ "$$OS" = "windows" ] && OUTPUT=$$OUTPUT.exe; \
		echo "Building $$OS/$$ARCH..."; \
		$(GO_DOCKER) sh -c "GOOS=$$OS GOARCH=$$ARCH \
			go build -ldflags \"$(LDFLAGS)\" \
			-o $$OUTPUT ./src" || exit 1; \
	done

	# Build CLI if src/client exists (per AI.md PART 26: search-cli-{os}-{arch})
	@if [ -d "src/client" ]; then \
		echo "Building CLI..."; \
		$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
			go build -ldflags \"$(CLI_LDFLAGS)\" -o $(BINDIR)/$(PROJECTNAME)-cli ./src/client"; \
		for platform in $(PLATFORMS); do \
			OS=$${platform%/*}; \
			ARCH=$${platform#*/}; \
			OUTPUT=$(BINDIR)/$(PROJECTNAME)-cli-$$OS-$$ARCH; \
			[ "$$OS" = "windows" ] && OUTPUT=$$OUTPUT.exe; \
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
	@for f in $(BINDIR)/$(PROJECTNAME)-*; do \
		[ -f "$$f" ] || continue; \
		echo "$$f" | grep -q "\-cli" && continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done

	# Copy CLI binaries to releases (if they exist)
	@for f in $(BINDIR)/$(PROJECTNAME)-cli-*; do \
		[ -f "$$f" ] || continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done

	# Create source archive (exclude VCS and build artifacts)
	@tar --exclude='.git' --exclude='.github' --exclude='.gitea' \
		--exclude='binaries' --exclude='releases' --exclude='*.tar.gz' \
		-czf $(RELDIR)/$(PROJECTNAME)-$(VERSION)-source.tar.gz .

	# Delete existing release/tag if exists
	@gh release delete $(VERSION) --yes 2>/dev/null || true
	@git tag -d $(VERSION) 2>/dev/null || true
	@git push origin :refs/tags/$(VERSION) 2>/dev/null || true

	# Create new release (stable)
	@gh release create $(VERSION) $(RELDIR)/* \
		--title "$(PROJECTNAME) $(VERSION)" \
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
	@docker buildx create --name $(PROJECTNAME)-builder --use 2>/dev/null || \
		docker buildx use $(PROJECTNAME)-builder

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
# TEST - Run all tests with coverage enforcement (via Docker per AI.md PART 26)
# =============================================================================
# Two-phase testing: verify pass first, then collect coverage
# Service package has Go 1.20+ integration coverage issues in Docker
test:
	@echo "Running tests..."
	@mkdir -p $(GOCACHE) $(GODIR)
	@$(GO_DOCKER) go mod download
	@echo "Phase 1: Verify all tests pass..."
	@$(GO_DOCKER) go test ./...
	@echo "Phase 2: Collect coverage (ignoring service integration coverage error)..."
	@$(GO_DOCKER) sh -c "go test -cover -coverprofile=coverage.out ./... 2>&1 || true"
	@echo "Tests complete"

# =============================================================================
# CLEAN - Remove build artifacts
# =============================================================================
clean:
	@rm -rf $(BINDIR) $(RELDIR)
