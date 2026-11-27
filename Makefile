# Resolve project identity from IDEA.md first, then the git remote, then the directory path.
PROJECT_NAME := $(shell sed -n 's/^project_name:[[:space:]]*//p' IDEA.md 2>/dev/null | head -n1 | tr -d '[:space:]')
ifeq ($(PROJECT_NAME),)
PROJECT_NAME := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)$$|\1|; s|\.git$$||' || basename "$$(pwd)")
endif
PROJECT_ORG := $(shell sed -n 's/^project_org:[[:space:]]*//p' IDEA.md 2>/dev/null | head -n1 | tr -d '[:space:]')
ifeq ($(PROJECT_ORG),)
PROJECT_ORG := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)/[^/]+(\.git)?$$|\1|' || basename "$$(dirname "$$(pwd)")")
endif
INTERNAL_NAME := $(shell sed -n 's/^internal_name:[[:space:]]*//p' IDEA.md 2>/dev/null | head -n1 | tr -d '[:space:]')
ifeq ($(INTERNAL_NAME),)
INTERNAL_NAME := $(PROJECT_NAME)
endif

# Local builds only: VERSION env (user override, AI never sets it) > non-empty release.txt > devel rule (short commit id, "devel" outside git)
VERSION ?= $(shell v=; [ -r release.txt ] && v=$$(tr -d '[:space:]' < release.txt); if [ -n "$$v" ]; then echo "$$v"; else git rev-parse --short=7 HEAD 2>/dev/null || echo devel; fi)

# Build info - BUILD_EPOCH is the single captured time source
# Unix build timestamp (seconds, UTC) - used by the updater's devel-channel check
BUILD_EPOCH := $(shell date -u +%s)
# Derived from BUILD_EPOCH - ISO 8601 UTC, e.g. "2025-12-04T13:05:13Z"
# Used only for Docker OCI labels (org.opencontainers.image.created); not an ldflag
BUILD_DATE := $(shell date -u -d @$(BUILD_EPOCH) +"%Y-%m-%dT%H:%M:%SZ")
COMMIT_ID := $(shell git rev-parse --short=7 HEAD 2>/dev/null || echo "N/A")
# COMMIT_ID used directly - no VCS_REF alias

# Official site URL (OPTIONAL - never guess or assume)
# Sources (in order of precedence):
#   1. File: site.txt in project root (single line, URL only)
#   2. Environment variable: OFFICIAL_SITE=<url>
#   3. Empty (self-hosted projects - users must use --server flag)
# NEVER infer from project name, domain, or any other source
OFFICIAL_SITE := $(shell [ -f site.txt ] && cat site.txt || echo "${OFFICIAL_SITE:-}")

# Linker flags to embed build info
# BuildDate is NOT embedded - it is derived from BuildEpoch at process start
LDFLAGS := -s -w \
	-X 'main.Version=$(VERSION)' \
	-X 'main.CommitID=$(COMMIT_ID)' \
	-X 'main.BuildEpoch=$(BUILD_EPOCH)' \
	-X 'main.OfficialSite=$(OFFICIAL_SITE)'

# Directories
BINDIR := binaries
RELDIR := releases

# Go directories (persistent across builds)
# GO_CACHE: module download cache — safe to share across concurrent builds (Go file-locks writes)
# GO_BUILD: compile cache — scoped per project to prevent corruption when multiple projects build concurrently
GO_CACHE  ?= $(HOME)/go/pkg/mod
GO_BUILD  ?= $(HOME)/.cache/go-build/$(PROJECT_NAME)

# Build targets - all 8 platforms
PLATFORMS ?= linux/amd64,linux/arm64,darwin/amd64,darwin/arm64,windows/amd64,windows/arm64,freebsd/amd64,freebsd/arm64

# Docker - Set REGISTRY based on your platform (ghcr.io, registry.gitlab.com, git.example.com)
REGISTRY ?= ghcr.io/$(PROJECT_ORG)/$(PROJECT_NAME)
PLATFORM_REPO_URL ?= $(shell git remote get-url origin 2>/dev/null | sed -E 's#^git@([^:]+):#https://\1/#; s#^ssh://git@([^/]+)/#https://\1/#; s#^(https?://)[^/@]+@#\1#; s#\.git$$##')

# Resource limits for build containers
DOCKER_MEM  ?= 4g
DOCKER_CPUS ?= 2

# CPU weight under contention (Docker's default is 1024): a build never starves the host's other work
DOCKER_CPU_SHARES ?= 256

# Hard ceiling in seconds for ONE container run, so a hung step can never hold a container for hours (raise it for a very long cross-compile)
DOCKER_TIMEOUT ?= 3600

# GO_DOCKER_RUN: shared docker run prefix (no image) so targets can add mounts before the image
# --rm: never leave an exited container behind; --no-healthcheck: the toolchain image's service HEALTHCHECK must not poll a one-shot build container;
# timeout: a hung run is stopped after DOCKER_TIMEOUT seconds (the signal is forwarded to the container, which stops)
GO_DOCKER_RUN := timeout -k 30 $(DOCKER_TIMEOUT) docker run --rm --no-healthcheck \
	--name $(PROJECT_NAME)-$$(head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n' | cut -c1-8) \
	--memory=$(DOCKER_MEM) --cpus=$(DOCKER_CPUS) --cpu-shares=$(DOCKER_CPU_SHARES) \
	-v $(PWD):/app \
	-v $(abspath $(BINDIR)):/build-output \
	-v $(GO_CACHE):/usr/local/share/go/pkg/mod \
	-v $(GO_BUILD):/usr/local/share/go/cache \
	-w /app \
	-e CGO_ENABLED=0 \
	-e GOFLAGS=-buildvcs=false
GO_DOCKER := $(GO_DOCKER_RUN) casjaysdev/go:latest
# CGO_ENABLED=0 and GOFLAGS=-buildvcs=false are casjaysdev/go:latest defaults; set explicitly for clarity

.PHONY: build local release release-check beta docker test dev clean

# =============================================================================
# BUILD - Build all platforms + local binary (via Docker with cached modules)
# =============================================================================
build: clean
	@mkdir -p $(BINDIR) $(GO_CACHE) $(GO_BUILD)
	@echo "Building version $(VERSION)..."

	# Tidy, download, local build and every platform in ONE container (one sh -c loop, never a container per platform)
	@$(GO_DOCKER_RUN) -e BUILD_LDFLAGS="$(LDFLAGS)" casjaysdev/go:latest sh -c ' \
		set -e; \
		go mod tidy; \
		go mod download; \
		go generate ./src/swagger/...; \
		echo "Building $(PROJECT_NAME) (local platform)..."; \
		GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) go build -buildvcs=false -trimpath -ldflags "$$BUILD_LDFLAGS" -o /build-output/$(PROJECT_NAME) ./src; \
		if [ -d src/client ]; then \
			echo "Building $(PROJECT_NAME)-cli (local platform)..."; \
			GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) go build -buildvcs=false -trimpath -ldflags "$$BUILD_LDFLAGS" -o /build-output/$(PROJECT_NAME)-cli ./src/client; \
		fi; \
		for platform in $$(echo "$(PLATFORMS)" | tr "," " "); do \
			OS=$${platform%/*}; \
			ARCH=$${platform#*/}; \
			EXT=""; \
			if [ "$$OS" = "windows" ]; then EXT=".exe"; fi; \
			echo "Building server $$OS/$$ARCH..."; \
			GOOS=$$OS GOARCH=$$ARCH go build -buildvcs=false -trimpath -ldflags "$$BUILD_LDFLAGS" -o /build-output/$(PROJECT_NAME)-$$OS-$$ARCH$$EXT ./src; \
			if [ -d src/client ]; then \
				echo "Building CLI $$OS/$$ARCH..."; \
				GOOS=$$OS GOARCH=$$ARCH go build -buildvcs=false -trimpath -ldflags "$$BUILD_LDFLAGS" -o /build-output/$(PROJECT_NAME)-cli-$$OS-$$ARCH$$EXT ./src/client; \
			fi; \
		done'

	@echo "Build complete: $(BINDIR)/"

# =============================================================================
# LOCAL - Build local binaries only (fast development builds)
# =============================================================================
local: clean
	@mkdir -p $(BINDIR) $(GO_CACHE) $(GO_BUILD)
	@echo "Building local binaries version $(VERSION)..."

	# Tidy, download and build every local binary in ONE container
	@$(GO_DOCKER_RUN) -e BUILD_LDFLAGS="$(LDFLAGS)" casjaysdev/go:latest sh -c ' \
		set -e; \
		go mod tidy; \
		go mod download; \
		go generate ./src/swagger/...; \
		echo "Building $(PROJECT_NAME)..."; \
		GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) go build -buildvcs=false -trimpath -ldflags "$$BUILD_LDFLAGS" -o /build-output/$(PROJECT_NAME) ./src; \
		if [ -d src/client ]; then \
			echo "Building $(PROJECT_NAME)-cli..."; \
			GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) go build -buildvcs=false -trimpath -ldflags "$$BUILD_LDFLAGS" -o /build-output/$(PROJECT_NAME)-cli ./src/client; \
		fi'

	@echo "Local build complete: $(BINDIR)/"

# =============================================================================
# RELEASE - Manual local release (stable only)
# =============================================================================
# Stable SemVer only: a devel version (short commit id or "devel") and any prerelease (-beta, -rc) are published by CI, never by this target
release-check:
	@echo "$(VERSION)" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$$' >/dev/null || { echo "ERROR: make release needs a stable SemVer X.Y.Z in release.txt (got '$(VERSION)')" >&2; exit 78; }

# Tag the current main commit with a UTC timestamp; CI publishes the beta from this tag.
beta:
	@[ "$$(git branch --show-current)" = main ] || { echo "ERROR: make beta must run on main" >&2; exit 2; }
	@[ -z "$$(git status --porcelain)" ] || { echo "ERROR: make beta requires a clean working tree" >&2; exit 2; }
	@tag=$$(date -u +%Y%m%d%H%M%S)-beta; git tag "$$tag" && git push origin "refs/tags/$$tag" && echo "Pushed beta tag $$tag"

release: release-check build
	@mkdir -p $(RELDIR)
	@echo "Preparing release $(VERSION)..."

	# Create version.txt
	@echo "$(VERSION)" > $(RELDIR)/version.txt

	# Copy binaries to releases (strip if needed)
	@for f in $(BINDIR)/$(PROJECT_NAME)-*; do \
		[ -f "$$f" ] || continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done

	# Create source archive (exclude VCS and build artifacts)
	@tar --exclude='.git' --exclude='.github' --exclude='.gitea' \
		--exclude='binaries' --exclude='releases' --exclude='*.tar.gz' \
		-czf $(RELDIR)/$(PROJECT_NAME)-$(VERSION)-source.tar.gz .

	# Generate checksums
	@cd $(RELDIR) && FILES="$$(ls)" && sha256sum $$FILES > sha256.txt && sha512sum $$FILES > sha512.txt

	# Delete existing release/tag if exists
	# Tag and release name are v{version}; version.txt stays unprefixed (numeric SemVer)
	@gh release delete v$(VERSION) --yes 2>/dev/null || true
	@git tag -d v$(VERSION) 2>/dev/null || true
	@git push origin :refs/tags/v$(VERSION) 2>/dev/null || true

	# Create new release (stable)
	@gh release create v$(VERSION) $(RELDIR)/* \
		--title "v$(VERSION)" \
		--notes "Release v$(VERSION)" \
		--latest

	@echo "Release complete: v$(VERSION)"

# =============================================================================
# DOCKER - Build and push container to registry (set REGISTRY env var)
# =============================================================================
# Uses multi-stage Dockerfile - Go compilation happens inside Docker
# No pre-built binaries needed
docker:
	@echo "Building Docker image $(VERSION)..."

	# Ensure buildx is available
	@docker buildx version > /dev/null 2>&1 || (echo "docker buildx required" && exit 1)

	# Create/use the ONE shared builder (never one per project: every builder is a long-lived buildkit container)
	@docker buildx create --name multiarch-builder --use 2>/dev/null || \
		docker buildx use multiarch-builder

	# Build multi-arch and push (buildx multi-arch images must be pushed; set REGISTRY first)
	@docker buildx build \
		-f docker/Dockerfile \
		--platform linux/amd64,linux/arm64 \
		--push \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--build-arg BUILD_EPOCH="$(BUILD_EPOCH)" \
		--build-arg COMMIT_ID="$(COMMIT_ID)" \
		--build-arg OFFICIAL_SITE="$(OFFICIAL_SITE)" \
		--label "maintainer=casjay <casjay@yahoo.com>" \
		--label "org.opencontainers.image.vendor=$(PROJECT_ORG)" \
		--label "org.opencontainers.image.authors=$(PROJECT_ORG)" \
		--label "org.opencontainers.image.title=$(PROJECT_NAME)" \
		--label "org.opencontainers.image.base.name=$(PROJECT_NAME)" \
		--label "org.opencontainers.image.description=$(PROJECT_NAME) - Privacy-respecting self-hosted metasearch engine" \
		--label "org.opencontainers.image.licenses=MIT" \
		--label "org.opencontainers.image.created=$(BUILD_DATE)" \
		--label "org.opencontainers.image.version=$(VERSION)" \
		--label "org.opencontainers.image.schema-version=$(VERSION)" \
		--label "org.opencontainers.image.revision=$(COMMIT_ID)" \
		--label "org.opencontainers.image.url=$(PLATFORM_REPO_URL)" \
		--label "org.opencontainers.image.source=$(PLATFORM_REPO_URL)" \
		--label "org.opencontainers.image.documentation=$(PLATFORM_REPO_URL)" \
		--label "org.opencontainers.image.vcs-type=Git" \
		--label "com.github.containers.toolbox=false" \
		--annotation "manifest:maintainer=casjay <casjay@yahoo.com>" \
		--annotation "manifest:org.opencontainers.image.vendor=$(PROJECT_ORG)" \
		--annotation "manifest:org.opencontainers.image.authors=$(PROJECT_ORG)" \
		--annotation "manifest:org.opencontainers.image.title=$(PROJECT_NAME)" \
		--annotation "manifest:org.opencontainers.image.base.name=$(PROJECT_NAME)" \
		--annotation "manifest:org.opencontainers.image.description=$(PROJECT_NAME) - Privacy-respecting self-hosted metasearch engine" \
		--annotation "manifest:org.opencontainers.image.licenses=MIT" \
		--annotation "manifest:org.opencontainers.image.created=$(BUILD_DATE)" \
		--annotation "manifest:org.opencontainers.image.version=$(VERSION)" \
		--annotation "manifest:org.opencontainers.image.schema-version=$(VERSION)" \
		--annotation "manifest:org.opencontainers.image.revision=$(COMMIT_ID)" \
		--annotation "manifest:org.opencontainers.image.url=$(PLATFORM_REPO_URL)" \
		--annotation "manifest:org.opencontainers.image.source=$(PLATFORM_REPO_URL)" \
		--annotation "manifest:org.opencontainers.image.documentation=$(PLATFORM_REPO_URL)" \
		--annotation "manifest:org.opencontainers.image.vcs-type=Git" \
		--annotation "manifest:com.github.containers.toolbox=false" \
		-t $(REGISTRY):$(VERSION) \
		-t $(REGISTRY):latest \
		.
	@docker pull $(REGISTRY):$(VERSION)
	@bash tests/docker-image-check.sh $(REGISTRY):$(VERSION)

	@echo "Docker build complete: $(REGISTRY):$(VERSION)"

# =============================================================================
# TEST - Run all tests with coverage enforcement (via Docker)
# =============================================================================
test:
	@mkdir -p $(GO_CACHE) $(GO_BUILD)
	@echo "Running tests with coverage..."
	@$(GO_DOCKER) sh -c " \
		mkdir -p \"\$${TMPDIR:-/tmp}/$(PROJECT_ORG)\" && \
		COVDIR=\$$(mktemp -d \"\$${TMPDIR:-/tmp}/$(PROJECT_ORG)/$(INTERNAL_NAME)-XXXXXX\") && \
		go mod download && \
		go generate ./src/swagger/... && \
		mkdir -p \$$COVDIR/data && \
		go test -v -cover ./... -args -test.gocoverdir=\$$COVDIR/data && \
		go tool covdata textfmt -i=\$$COVDIR/data -o=\$$COVDIR/coverage.out && \
		COVERAGE=\$$(go tool cover -func=\$$COVDIR/coverage.out | grep '^total:' | awk '{print \$$3}' | sed 's/%//') && \
		echo \"Coverage: \$$COVERAGE%\" && \
		if [ \$$(echo \"\$$COVERAGE < 60\" | bc -l) -eq 1 ]; then \
			echo \"ERROR: Coverage is \$$COVERAGE%, must be >= 60%\"; exit 1; \
		fi && \
		echo \"Tests complete - Coverage: \$$COVERAGE% (>= 60% required) ✓\""

# =============================================================================
# Coverage gates:
#   - All Go projects: 60% minimum (go test -cover must report >= 60.0%)
#   - Override upward in IDEA.md (## Project variables -> coverage_minimum: 80)
#     when higher coverage is appropriate. Never override downward.
#   - CLI tools and libraries: same 60% floor.
# Coverage runs in CI on every push and in `make test` locally.
# Never skip with -short or -count=0. No //go:coverage ignore workarounds.
# =============================================================================

# =============================================================================
# DEV - Quick build for local development/testing (to random temp dir)
# =============================================================================
# Fast: local platform only, no ldflags, random temp dir for isolation
# Builds server + CLI (if they exist)
dev:
	@mkdir -p $(GO_CACHE) $(GO_BUILD)
	@mkdir -p "$${TMPDIR:-/tmp}/$(PROJECT_ORG)" && \
		BUILD_DIR=$$(mktemp -d "$${TMPDIR:-/tmp}/$(PROJECT_ORG)/$(INTERNAL_NAME)-XXXXXX") && \
		echo "Quick dev build to $$BUILD_DIR..." && \
		$(GO_DOCKER_RUN) -v $$BUILD_DIR:/build casjaysdev/go:latest sh -c ' \
			set -e; \
			go mod tidy; \
			go generate ./src/swagger/...; \
			go build -buildvcs=false -o /build/$(PROJECT_NAME) ./src; \
			if [ -d src/client ]; then go build -buildvcs=false -o /build/$(PROJECT_NAME)-cli ./src/client; fi' && \
		echo "Built: $$BUILD_DIR/$(PROJECT_NAME)" && \
		if [ -d "src/client" ]; then echo "Built: $$BUILD_DIR/$(PROJECT_NAME)-cli"; fi && \
		TEST_CONTAINER="$(PROJECT_NAME)-$$(head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n' | cut -c1-8)" && \
		echo "Test:  docker run --rm --name $$TEST_CONTAINER -v $$BUILD_DIR:/app alpine:latest /app/$(PROJECT_NAME) --help"

# =============================================================================
# CLEAN - Remove build artifacts
# =============================================================================
clean:
	@rm -rf $(BINDIR) $(RELDIR)
