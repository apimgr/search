# Docker Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER place Dockerfile or docker-compose.yml in project root — always `docker/`
- NEVER include `build:` or `version:` in any docker-compose file
- NEVER modify `ENTRYPOINT` or `CMD` — all customization goes in `entrypoint.sh`
- NEVER use `${VAR}`/`${VAR:-default}` syntax requiring `.env`; never create `.env`, `.env.example`, `.env.sample`
- NEVER bake `MODE` into the image; production compose sets neither `DEBUG` nor `MODE`
- NEVER run `docker compose` from the project directory or mount volumes to `{project_root}/volumes/`
- NEVER commit runtime `./volumes/` content — only `docker/rootfs/` (build overlay) is committed
- NEVER add `LABEL` blocks to the Dockerfile — CI applies OCI labels/annotations at build time
- NEVER push `:dev` or `:test` tags to the production registry
- AI assistants must NEVER use `docker/docker-compose.yml` or `docker/docker-compose.dev.yml` directly — HUMAN USE ONLY

## CRITICAL - ALWAYS DO

- ALWAYS use multi-stage builds: builder (`casjaysdev/go:latest`) + runtime (`alpine:latest`)
- ALWAYS build context = project root; `-f docker/Dockerfile`
- ALWAYS use `entrypoint.sh` for container startup, ending with `exec "$@"` (or `exec <binary> ... "$@"`)
- ALWAYS switch to a non-root runtime user (`app`) unless binding a privileged port or managing system services (document exception in IDEA.md)
- ALWAYS use `tini` as init: `ENTRYPOINT [ "tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh" ]`
- ALWAYS set `STOPSIGNAL SIGRTMIN+3`
- ALWAYS set `HEALTHCHECK --start-period=10m --interval=5m --timeout=15s --retries=3`
- ALWAYS run docker compose from a temp dir (`mktemp -d "${TMPDIR:-/tmp}/{project_org}/{internal_name}-XXXXXX"`), never the repo
- ALWAYS hardcode environment variables with sane defaults directly in compose files

## Key Rules Summary

**Required Dockerfile stages**
- Builder: `casjaysdev/go:latest`, `CGO_ENABLED=0`, output to `/app/binary/{project_name}`
- Runtime: `alpine:latest`, installs `git curl bash tini tor`, `COPY --from=builder`, `COPY docker/rootfs/ /`
- Internal port always `80`; binary at `/usr/local/bin/{project_name}`

**Dockerfile.dev**
- `docker/Dockerfile.dev` — same as release but binary runs in debug mode
- Tagged `:devel`; built with `docker build -t .../{internal_name}:devel -f docker/Dockerfile.dev .`

**docker-compose files (all in `docker/`)**
- `docker-compose.yml` — production, `:latest` tag, Valkey cache (persistent volume), no DEBUG/MODE, port `172.17.0.1:64580:80`
- `docker-compose.dev.yml` — dev, `:devel` tag, no cache service, `DEBUG: 1`/`MODE: dev`, port `64580:80` — HUMAN USE ONLY
- `docker-compose.test.yml` — test, `:latest` tag, ephemeral `tmpfs` Valkey cache, `DEBUG: 1`/`MODE: dev` — AI/automated testing (prefer `tests/` scripts)
- Every compose file needs `name:`, `x-logging` anchor used by every service, network `{project_name}` with `external: false`
- Volumes: only `./volumes/config:/config:z` and `./volumes/data:/data:z` (`:z` on production/test, omitted on dev)

**Image naming/tagging**
- Release: `{PLATFORM_CONTAINER_REGISTRY}/{project_org}/{internal_name}:latest|{version}|{YYMM}|{commit}`
- Dev-local only: `{project_name}:dev`, `{project_name}:test` — never pushed
- Release images build for `linux/amd64` and `linux/arm64`

**OCI labels**
- No `LABEL` in Dockerfile — CI applies all labels via `--label` flags and manifest `annotations:` (via `docker/metadata-action`)
- Required set includes `maintainer`, `org.opencontainers.image.{vendor,authors,title,base.name,description,licenses,created,version,schema-version,revision,url,source,documentation,vcs-type}`, `com.github.containers.toolbox=false`

For complete details, see AI.md PART 26.
