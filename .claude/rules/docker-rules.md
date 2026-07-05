# Docker Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Create docker/Dockerfile.build for Go projects (casjaysdev/go:latest is comprehensive)
- Use $(pwd) in docker -v flags in shell scripts (use $PWD)
- Run the binary as root in containers (use tini + entrypoint that drops privileges)
- Hardcode image versions in CI/CD (pin to SHA)
- Use alpine for the runtime image without testing (prefer scratch or distroless for Go)
- Include source code or build tools in the runtime image

## CRITICAL - ALWAYS DO

- Build image: casjaysdev/go:latest (never custom Dockerfile.build for Go)
- Runtime image: scratch or casjaysdev/alpine:latest (static binary)
- Use tini as PID 1: ENTRYPOINT ["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]
- Multi-stage build: build stage (casjaysdev/go:latest) → runtime stage (scratch)
- OCI labels in Dockerfile
- $PWD in shell -v flags; $(PWD) in Makefile -v flags

## Dockerfile Pattern

```dockerfile
# Build stage
FROM casjaysdev/go:latest AS builder
WORKDIR /workspace
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X ..." \
    -o /search ./src/

# Runtime stage
FROM casjaysdev/alpine:latest
LABEL org.opencontainers.image.source="https://github.com/apimgr/search"
LABEL org.opencontainers.image.licenses="MIT"
COPY --from=builder /search /usr/local/bin/search
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh
ENTRYPOINT ["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]
```

## OCI Labels

Required labels:
- org.opencontainers.image.title
- org.opencontainers.image.description
- org.opencontainers.image.version
- org.opencontainers.image.source
- org.opencontainers.image.licenses
- org.opencontainers.image.created
- org.opencontainers.image.revision

## Volume Mounts

| Path | Purpose |
|------|---------|
| /config/search/ | Configuration (server.yml) |
| /data/search/ | Persistent data (database, etc.) |

## Docker-Specific Paths

| Setting | Path |
|---------|------|
| Config | /config/search/server.yml |
| Data | /data/search/ |
| Logs | stdout/stderr (Docker logs) |

## Environment Variables for Docker

```yaml
# docker-compose.yml
environment:
  - CONFIG_DIR=/config/search
  - DATA_DIR=/data/search
  - MODE=production
```

For complete details, see AI.md PART 26
