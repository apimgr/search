# Makefile Rules (PART 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER add extra Makefile targets beyond the six core ones
- NEVER build on the host — all builds run inside Docker (`casjaysdev/go:latest`)
- NEVER hardcode `PROJECT_NAME`/`PROJECT_ORG` — always derive from git remote or directory
- NEVER copy or symlink binaries (no `cp`/`ln -s` to PATH, `/usr/local/bin`, or between `binaries/`/`releases/`)
- NEVER add `v` prefix to text/timestamp versions (`vdev`, `vbeta`, `v20251218` are all wrong)
- NEVER double the `v` prefix (`vv1.2.3`)
- NEVER skip coverage enforcement or use `-short`/`-count=0` to dodge it
- NEVER treat `make release`/`make docker` as CI/CD replacements — `make release` is manual-local only; automated releases go through CI/CD

## CRITICAL - ALWAYS DO

- ALWAYS limit the Makefile to exactly six targets: `dev`, `local`, `build`, `test`, `release`, `docker`
- ALWAYS source version from `release.txt` first, then `VERSION` env var, then fall back to `devel`
- ALWAYS embed `Version`, `CommitID`, `BuildDate`, `OfficialSite` via `-ldflags` on real builds (not `dev`)
- ALWAYS use `CGO_ENABLED=0` for static binaries
- ALWAYS run `clean` before `build` and `local`
- ALWAYS output `dev` builds to an isolated tempdir: `${TMPDIR}/${PROJECT_ORG}/${PROJECT_NAME}-XXXXXX/`
- ALWAYS output `local`/`build`/`release` artifacts to `binaries/` (and `release`'s packaged output to `releases/`)
- ALWAYS strip binaries before packaging a release
- ALWAYS run tests and builds through Docker with cached Go module/build dirs

## Key Rules Summary

**Required Makefile targets (exactly six, no more):**

| Target | Purpose | Output |
|--------|---------|--------|
| `dev` | Quick dev build | tempdir |
| `local` | Prod test build | `binaries/` |
| `build` | Full release, 8 platforms | `binaries/` |
| `test` | Unit tests + coverage gate | coverage report |
| `release` | Local manual release w/ source archive | `releases/` |
| `docker` | Build + push container | `$REGISTRY` |

**Local-dev-only scope — the Makefile is NOT for CI/CD:**
- `make dev`/`make local`/`make build`/`make test` are local developer workflow only
- `make release` is for manual local releases only — automated stable/beta/daily releases run through CI/CD (GitHub Actions/Gitea Actions/GitLab CI), never `make`
- CI/CD pipelines implement their own build/release/test logic; they do not just shell out to these Makefile targets as their source of truth for release semantics

**Build/test/lint conventions:**
- Build matrix: linux/darwin/windows/freebsd × amd64/arm64 (8 platforms)
- Binary naming: `{project_name}` (server), `{project_name}-cli` (CLI); distributed as `{project_name}[-cli]-{os}-{arch}[.exe]`
- Coverage gate: >= 60% minimum (raise via `IDEA.md`, never lower); tests run in Docker via `go test -cover`
- Version tag `v` prefix: numeric semver only (`1.2.3` → `v1.2.3`); text/timestamp versions never get `v` (`dev`, `beta`, `20251218060432`)
- Directory rules: `binaries/` = build output (gitignored), `releases/` = packaged artifacts, both never manually copied elsewhere

For complete details, see AI.md PART 25.
