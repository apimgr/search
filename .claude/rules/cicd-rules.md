# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- NEVER use Makefile targets in CI/CD workflows — commands must be explicit and visible (opposite of local dev).
- NEVER reference local user paths (e.g. `~/.local/share/go`) in CI — use `/tmp/` or CI-native caching.
- NEVER depend on local Docker containers for builds on GitHub/Gitea Actions — use native runners with `container: image: casjaysdev/go:latest`.
- NEVER cross-cancel different release refs — auto-cancel only applies to older runs of the *exact same* branch/tag ref (e.g. a `v1.2.4` run must never cancel `v1.2.3`).
- NEVER install tools inline inside CI jobs — all jobs run inside `casjaysdev/go:latest`; no `ensure-build-image` gate, no `build-toolchain.yml` for Go projects.
- NEVER use `github.event.before`/`default_branch` incorrectly for secret-scan range — never use `default_branch` (it resolves to HEAD after push and silently skips the scan); use `github.event.before` / `github.event.after`.
- NEVER add `-musl` suffix to binary names.
- NEVER skip `ci.yml` or `release.yml` — both are required on every project.
- NEVER pin third-party Actions to a tag — always pin to a full commit SHA (with a `# vX.Y.Z` trailing comment for readability).

## CRITICAL - ALWAYS DO

- ALWAYS use explicit `go build` commands with all flags visible in CI (no Makefile).
- ALWAYS use CI-native caching, never host cache dirs.
- ALWAYS set `VERSION`, `COMMIT_ID`, `BUILD_DATE` explicitly via a "Set build info" step (not static `env:`).
- ALWAYS build all 8 platforms in the release/beta/daily matrix: linux/darwin/windows/freebsd × amd64/arm64 (windows adds `.exe`).
- ALWAYS add workflow concurrency (`concurrency: group / cancel-in-progress`) to any workflow triggered by pushes to `main`, `master`, `devel`, `dev`, or `beta`.
- ALWAYS scope tag-release concurrency groups per exact tag ref (e.g. `release-${{ github.ref }}`).
- ALWAYS build CLI binaries (with `-cli` suffix) only when `src/client/` exists (`if: hashFiles('src/client/') != ''`).
- ALWAYS run security jobs (`secret-scan`, `workflow-policy`, `vuln-scan`, `image-scan`) on push, PR, and a weekly cron (`0 6 * * 1`); skip non-security jobs on scheduled runs with `if: github.event_name != 'schedule'`.
- ALWAYS use truffleHog (Apache-2.0) for mandatory secret scanning on every public repo.
- ALWAYS enforce the coverage threshold (60%) in `ci.yml` and fail the build below it.
- ALWAYS build Docker images for `linux/amd64` and `linux/arm64` via buildx, pushed to `ghcr.io` (GitHub) or provider-equivalent registry.

## Key Rules Summary

**Required workflow files (per provider):**
| File | Trigger | Required? |
|------|---------|-----------|
| `ci.yml` | push/PR + weekly cron for security | Required, all projects |
| `release.yml` | tag push (`v*`, `*.*.*`) | Required, all projects |
| `beta.yml` | push to `beta` | Optional, project-specific |
| `daily.yml` | 3am UTC cron + push to main/master | Optional, project-specific |
| `docker.yml` | any branch push + version tags | Optional, project-specific |

Go projects never have `build-toolchain.yml` — `casjaysdev/go:latest` is externally maintained.

**Config locations by provider:**
- GitHub → `.github/workflows/*.yml` (github.com only, no self-hosted)
- Gitea → `.gitea/workflows/*.yml` (self-hosted or gitea.com)
- Forgejo → `.forgejo/workflows/*.yml` (self-hosted only; also reads `.gitea/workflows/` for compat)
- GitLab → single `.gitlab-ci.yml` with stages (self-hosted or gitlab.com)
- Jenkins → `Jenkinsfile` with `BUILD_TYPE` param (`release`/`beta`/`daily`)

**Third-party action SHA-pinning:** every `uses:` in the spec examples pins a full commit SHA with a `# vX.Y.Z` comment (e.g. `actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0  # v7.0.0`) — never a bare tag.

**Workflow creation order:** security-relevant jobs (secret-scan, workflow-policy, vuln-scan, image-scan) live inside `ci.yml` and must be considered/created first; `ci.yml` and `release.yml` are baseline; `beta.yml`/`daily.yml`/`docker.yml` are added only when the project needs them.

**Provider-specific conventions:**
- Variable mapping: `github.*`↔`gitea.*`↔`forgejo.*`; `GITHUB_ENV`↔`GITEA_ENV`↔`FORGEJO_ENV`; `github.token`↔`secrets.GITEA_TOKEN`↔`secrets.FORGEJO_TOKEN`.
- GitLab uses `$CI_*` auto-populated variables (e.g. `$CI_COMMIT_SHORT_SHA`, `$CI_REGISTRY_IMAGE`) instead of Actions context.
- Forgejo is a Gitea fork — workflows are cross-compatible; prefer `.forgejo/workflows/` but `.gitea/workflows/` also works.
- Docker image tags: `devel`/`{commit_id}` on any push; add `beta` tag on beta branch push; `{version}`/`latest`/`{YYMM}` on version tags.
- Daily builds delete and recreate the `daily` tag/release each run (`gh release delete daily --yes`).

For complete details, see AI.md PART 27.
