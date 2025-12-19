# Runtime stage
FROM alpine:latest

# ARGs for build-time values (set by docker build --build-arg)
ARG VERSION=dev
ARG BUILD_DATE
ARG VCS_REF
ARG TARGETARCH
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
      org.opencontainers.image.revision="${VCS_REF}"

# Install required packages including Tor
RUN apk add --no-cache \
    curl \
    bash \
    tini \
    tor

# Create directories with proper structure
RUN mkdir -p /config /data/db /data/logs /data/tor /data/geoip /data/backup

# Copy the arch-specific binary from ./binaries/ (project root)
COPY binaries/search-linux-${TARGETARCH} /usr/local/bin/search

# Copy entrypoint script from ./docker/rootfs/
COPY docker/rootfs/ /

# Make scripts executable
RUN chmod +x /usr/local/bin/search /usr/local/bin/entrypoint.sh

# Set environment
ENV MODE=development \
    TZ=America/New_York \
    ENABLE_TOR=false

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
