# Infer PROJECTNAME and PROJECTORG from git remote or directory path (NEVER hardcode)
PROJECTNAME := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)(\.git)?$$|\1|' || basename "$$(pwd)")
PROJECTORG := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)/[^/]+(\.git)?$$|\1|' || basename "$$(dirname "$$(pwd)")")

# Version precedence: release.txt > env/default fallback
VERSION ?= $(shell cat release.txt 2>/dev/null || echo "devel")
# Per AI.md PART 25: add v prefix ONLY to numeric semver (e.g. 1.2.3 → v1.2.3)
# Text versions (dev, beta, devel) and timestamps get NO v prefix
TAG := $(shell echo "$(VERSION)" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]' && echo "v$(VERSION)" || echo "$(VERSION)")

# Build info - use TZ env var or system timezone
# Format: "Thu Dec 17, 2025 at 18:19:24 EST"
BUILD_DATE := $(shell date +"%a %b %d, %Y at %H:%M:%S %Z")
COMMIT_ID := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Official site URL (OPTIONAL - never guess or assume)
# Sources (in order of precedence):
#   1. File: site.txt in project root (single line, URL only)
#   2. Environment variable: OFFICIALSITE=https://example.com
#   3. Empty (self-hosted projects - users must use --server flag)
# NEVER infer from project name, domain, or any other source
OFFICIALSITE := $(shell [ -f site.txt ] && cat site.txt || echo "${OFFICIALSITE:-}")

# Linker flags to embed build info (per AI.md PART 25)
# Sets vars in both src/config and src/version packages (both declare build info vars)
LDFLAGS := -s -w \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/config.Version=$(VERSION)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/config.CommitID=$(COMMIT_ID)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/config.BuildDate=$(BUILD_DATE)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/config.OfficialSite=$(OFFICIALSITE)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/version.Version=$(VERSION)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/version.Commit=$(COMMIT_ID)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/version.BuildDate=$(BUILD_DATE)'

# CLI linker flags (per AI.md PART 25)
CLI_LDFLAGS := -s -w \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.ProjectName=$(PROJECTNAME)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.Version=$(VERSION)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.CommitID=$(COMMIT_ID)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.BuildDate=$(BUILD_DATE)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/cmd.OfficialSite=$(OFFICIALSITE)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/api.ProjectName=$(PROJECTNAME)' \
	-X 'github.com/$(PROJECTORG)/$(PROJECTNAME)/src/client/api.Version=$(VERSION)'

# Directories
BINDIR := binaries
RELDIR := releases

# Build targets (8 platforms minimum per AI.md PART 25)
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64 freebsd/arm64

# Docker (per AI.md PART 25: always use casjaysdev/go:latest; named volume for Go state)
REGISTRY ?= ghcr.io/$(PROJECTORG)/$(PROJECTNAME)
GO_DOCKER := docker run --rm \
	--name $(PROJECTNAME)-$$(tr -dc 'a-z0-9' </dev/urandom | head -c8) \
	-v $(PWD):/app \
	-v go-state:/usr/local/share/go \
	-w /app \
	-e CGO_ENABLED=0 \
	casjaysdev/go:latest

.PHONY: dev local build release docker test

# =============================================================================
# DEV - Quick dev build to temp directory (per AI.md PART 25)
# =============================================================================
# Outputs to /tmp/{project_org}/{project_name}-XXXXXX/ for quick testing
# ALWAYS uses Docker for building - host has NO Go installed
dev:
	@$(GO_DOCKER) go mod tidy
	@mkdir -p "$${TMPDIR:-/tmp}/$(PROJECTORG)" && \
	BUILD_DIR=$$(mktemp -d "$${TMPDIR:-/tmp}/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX") && \
	echo "Quick dev build to $$BUILD_DIR..." && \
	$(GO_DOCKER) go build -o $(BINDIR)/.dev-$(PROJECTNAME) ./src && \
	mv $(BINDIR)/.dev-$(PROJECTNAME) "$$BUILD_DIR/$(PROJECTNAME)" && \
	if [ -d "src/client" ]; then \
		$(GO_DOCKER) go build -o $(BINDIR)/.dev-$(PROJECTNAME)-cli ./src/client && \
		mv $(BINDIR)/.dev-$(PROJECTNAME)-cli "$$BUILD_DIR/$(PROJECTNAME)-cli"; \
	fi && \
	echo "Built: $$BUILD_DIR/$(PROJECTNAME)" && \
	echo "Test:  docker run --rm -it --name $(PROJECTNAME)-test -v $$BUILD_DIR:/app alpine:latest /app/$(PROJECTNAME) --help"

# =============================================================================
# LOCAL - Build for current OS/ARCH with version suffix (per AI.md PART 25)
# =============================================================================
# Outputs to binaries/search-VERSION for production testing
local:
	@rm -rf $(BINDIR) $(RELDIR)
	@mkdir -p $(BINDIR)
	@echo "Building local binaries version $(VERSION)..."
	@$(GO_DOCKER) go mod tidy
	@$(GO_DOCKER) go mod download
	@$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
		go build -ldflags \"$(LDFLAGS)\" -o $(BINDIR)/$(PROJECTNAME)-$(VERSION) ./src"
	@if [ -d "src/client" ]; then \
		echo "Building local CLI $(VERSION)..."; \
		$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
			go build -ldflags \"$(CLI_LDFLAGS)\" -o $(BINDIR)/$(PROJECTNAME)-cli-$(VERSION) ./src/client"; \
	fi
	@echo ""
	@echo "Built: $(BINDIR)/$(PROJECTNAME)-$(VERSION)"

# =============================================================================
# BUILD - Build all platforms + host binary (via Docker with cached modules)
# =============================================================================
build:
	@rm -rf $(BINDIR) $(RELDIR)
	@mkdir -p $(BINDIR)
	@echo "Building version $(VERSION)..."
	@echo "Tidying and downloading Go modules..."
	@$(GO_DOCKER) go mod tidy
	@$(GO_DOCKER) go mod download
	@echo "Building host binary..."
	@$(GO_DOCKER) sh -c "GOOS=\$$(go env GOOS) GOARCH=\$$(go env GOARCH) \
		go build -ldflags \"$(LDFLAGS)\" -o $(BINDIR)/$(PROJECTNAME) ./src"
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUTPUT=$(BINDIR)/$(PROJECTNAME)-$$OS-$$ARCH; \
		[ "$$OS" = "windows" ] && OUTPUT=$$OUTPUT.exe; \
		echo "Building server $$OS/$$ARCH..."; \
		$(GO_DOCKER) sh -c "GOOS=$$OS GOARCH=$$ARCH \
			go build -ldflags \"$(LDFLAGS)\" \
			-o $$OUTPUT ./src" || exit 1; \
	done
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
	@echo "$(VERSION)" > $(RELDIR)/version.txt
	@for f in $(BINDIR)/$(PROJECTNAME)-*; do \
		[ -f "$$f" ] || continue; \
		echo "$$f" | grep -q "\-cli" && continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done
	@for f in $(BINDIR)/$(PROJECTNAME)-cli-*; do \
		[ -f "$$f" ] || continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done
	@tar --exclude='.git' --exclude='.github' --exclude='.gitea' \
		--exclude='binaries' --exclude='releases' --exclude='*.tar.gz' \
		-czf $(RELDIR)/$(PROJECTNAME)-$(VERSION)-source.tar.gz .
	@gh release delete $(TAG) --yes 2>/dev/null || true
	@git tag -d $(TAG) 2>/dev/null || true
	@git push origin :refs/tags/$(TAG) 2>/dev/null || true
	@gh release create $(TAG) $(RELDIR)/* \
		--title "$(PROJECTNAME) $(VERSION)" \
		--notes "Release $(VERSION)" \
		--latest
	@echo "Release complete: $(TAG)"

# =============================================================================
# DOCKER - Build and push container to ghcr.io
# =============================================================================
# Uses multi-stage Dockerfile - Go compilation happens inside Docker
# No pre-built binaries needed
docker:
	@echo "Building Docker image $(VERSION)..."
	@docker buildx version > /dev/null 2>&1 || (echo "docker buildx required" && exit 1)
	@docker buildx create --name $(PROJECTNAME)-builder --use 2>/dev/null || \
		docker buildx use $(PROJECTNAME)-builder
	@docker buildx build \
		-f docker/Dockerfile \
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
# TEST - Run all tests with coverage enforcement (via Docker per AI.md PART 25)
# =============================================================================
# Server template projects: 80% minimum coverage threshold
# Per AI.md PART 28: test artifacts go to /tmp/apimgr/search-XXXXXX/, NEVER project dir
test:
	@echo "Running tests with coverage..."
	@mkdir -p "/tmp/$(PROJECTORG)" && \
	COVDIR=$$(mktemp -d "/tmp/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX") && \
	docker run --rm \
		--name $(PROJECTNAME)-test-$$(tr -dc 'a-z0-9' </dev/urandom | head -c8) \
		-v $(PWD):/app \
		-v go-state:/usr/local/share/go \
		-v $$COVDIR:/tmp/covout \
		-w /app \
		-e CGO_ENABLED=0 \
		casjaysdev/go:latest \
		ash -c 'set -e; PKGS=$$(go list ./... | grep -v "/src/service"); go mod download; go test -v -cover -coverprofile=/tmp/covout/coverage.out $$PKGS; COVERAGE=$$(go tool cover -func=/tmp/covout/coverage.out | grep total | awk "{print \$$3}" | sed "s/%//"); echo "Coverage: $$COVERAGE%"; if [ $$(echo "$$COVERAGE < 80" | bc -l) -eq 1 ]; then echo "ERROR: Coverage is $$COVERAGE%, must be >= 80%"; exit 1; fi'
	@echo "Tests complete"

