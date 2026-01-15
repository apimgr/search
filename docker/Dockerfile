# =============================================================================
# Build Stage - Compile Go binary
# =============================================================================
FROM golang:alpine AS builder

# Install git and bash (git: required for go mod download; bash: for build scripts)
RUN apk add --no-cache git bash

ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID

WORKDIR /build

# Copy go.mod first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w -X 'github.com/apimgr/search/src/config.Version=${VERSION}' -X 'github.com/apimgr/search/src/config.CommitID=${COMMIT_ID}' -X 'github.com/apimgr/search/src/config.BuildDate=${BUILD_DATE}'" \
    -o /app ./src

# =============================================================================
# Runtime Stage - Minimal Alpine image
# =============================================================================
FROM alpine:latest

# ARGs for build-time values (set by docker build --build-arg)
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID
ARG LICENSE=MIT

# Static Labels
LABEL maintainer="apimgr <apimgr@casjay.cc>" \
      org.opencontainers.image.vendor="apimgr" \
      org.opencontainers.image.authors="apimgr" \
      org.opencontainers.image.title="search" \
      org.opencontainers.image.base.name="search" \
      org.opencontainers.image.description="Containerized version of search - privacy-respecting metasearch engine" \
      org.opencontainers.image.url="https://github.com/apimgr/search" \
      org.opencontainers.image.source="https://github.com/apimgr/search" \
      org.opencontainers.image.documentation="https://github.com/apimgr/search" \
      org.opencontainers.image.vcs-type="Git" \
      com.github.containers.toolbox="false"

# Dynamic Labels (from ARGs)
LABEL org.opencontainers.image.licenses="${LICENSE}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.schema-version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT_ID}"

# Install required packages (git, curl, bash, tini, tor - per AI.md PART 27)
RUN apk add --no-cache \
    git \
    curl \
    bash \
    tini \
    tor

# NO directory creation - binary handles ALL setup based on env vars
# This allows users to override CONFIG_DIR, DATA_DIR, etc.
# Docker volume mounts auto-create mount points
# Per AI.md PART 27: Binary handles directories, permissions, user/group, Tor, etc.

# Copy binary from builder stage (multi-stage build)
COPY --from=builder /app /usr/local/bin/search

# Copy entrypoint script from ./docker/rootfs/
COPY docker/rootfs/ /

# Copy Dockerfile to image (for reference and backup)
COPY docker/Dockerfile /root/Dockerfile

# Make all binaries/scripts in /usr/local/bin executable (755)
RUN chmod 755 /usr/local/bin/*

# Set environment
# Note: Tor is auto-detected (enabled if tor binary installed) per TEMPLATE.md PART 29
# No ENABLE_TOR env var needed - tor binary is always installed in this image
# PORT=80 forces app to listen on port 80 inside container
ENV MODE=development \
    TZ=America/New_York \
    PORT=80

# Expose internal port (80 inside container, map to 64080 on host)
EXPOSE 80

# Stop signal for graceful shutdown
STOPSIGNAL SIGRTMIN+3

# Health check (long start period for services that need initialization)
HEALTHCHECK --start-period=10m --interval=5m --timeout=15s --retries=3 \
    CMD /usr/local/bin/search --status || exit 1

# Use tini as init with signal propagation
# -p SIGTERM: propagate SIGTERM to child processes
ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]
