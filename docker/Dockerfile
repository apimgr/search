# =============================================================================
# Build Stage - Compile Go binary
# =============================================================================
FROM casjaysdev/go:latest AS builder

# Install git and bash (git: required for go mod download; bash: for build scripts)
RUN apk add --no-cache git bash

ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_EPOCH
ARG COMMIT_ID
ARG OFFICIAL_SITE

WORKDIR /app

# Copy go.mod first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -buildvcs=false -trimpath \
    -ldflags "-s -w -X 'github.com/apimgr/search/src/config.Version=${VERSION}' -X 'github.com/apimgr/search/src/config.CommitID=${COMMIT_ID}' -X 'github.com/apimgr/search/src/config.BuildEpoch=${BUILD_EPOCH}' -X 'github.com/apimgr/search/src/config.OfficialSite=${OFFICIAL_SITE}' -X 'github.com/apimgr/search/src/version.Version=${VERSION}' -X 'github.com/apimgr/search/src/version.CommitID=${COMMIT_ID}' -X 'github.com/apimgr/search/src/version.BuildEpoch=${BUILD_EPOCH}'" \
    -o /app/binary/search ./src

# =============================================================================
# Runtime Stage - Minimal Alpine image
# =============================================================================
FROM alpine:latest

# ARGs for build-time values (set by docker build --build-arg)
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID
ARG LICENSE=MIT

# No LABEL blocks — all image metadata is applied as OCI labels/annotations at
# build time by CI via --label/--annotation flags (see AI.md PART 26).

# Pull in the latest patched package index before installing anything, so a
# security-fixed libcrypto3/libssl3 (or any other base-image package) isn't
# left pinned to whatever was baked into the alpine:latest layer
RUN apk upgrade --no-cache

# Install required packages
# NOTE: Tor binary installed but NOT configured here - binary handles all Tor setup (see PART 31)
RUN apk add --no-cache \
    git \
    curl \
    bash \
    tini \
    tor

# NO directory creation - binary handles ALL setup based on env vars
# This allows users to override CONFIG_DIR, DATA_DIR, etc.
# Docker volume mounts auto-create mount points

# Copy binary from builder stage (multi-stage build)
COPY --from=builder /app/binary/search /usr/local/bin/search

# Copy BUILD-TIME overlay (entrypoint.sh) from docker/rootfs/ into image
# Note: This is docker/rootfs/ (build context), NOT runtime ./volumes/ mounts
COPY docker/rootfs/ /

# Copy Dockerfile to image (for reference and backup)
COPY docker/Dockerfile /root/Dockerfile

# Make all binaries/scripts in /usr/local/bin executable (755)
RUN chmod 755 /usr/local/bin/*

# Non-root runtime user: documented exception (see IDEA.md § Docker Non-Root
# Exception) — binary binds privileged port 80 AND must create/chown
# /config and /data on first run for the zero-config Docker Volume Mapping
# contract (PART 26); entrypoint.sh is spec-prohibited from doing that setup
# itself, so privilege drop cannot happen before exec. Runtime stays root.

# Set environment
# Note: Tor available (binary installed) but server binary controls Tor startup (see PART 31)
# DATABASE_DIR: SQLite location (binary auto-detects container, but explicit is clearer)
# MODE is never baked into the image — binary defaults to production; compose sets MODE explicitly
ENV TZ=America/New_York \
    DATABASE_DIR=/data/db/sqlite

# Expose internal port (always 80)
EXPOSE 80

# Stop signal for graceful shutdown
STOPSIGNAL SIGRTMIN+3

# Health check (long start period for services that need initialization)
HEALTHCHECK --start-period=10m --interval=5m --timeout=15s --retries=3 \
    CMD /usr/local/bin/search --status || exit 1

# Use tini as init with signal propagation
# -p SIGTERM: propagate SIGTERM to child processes
ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]
