# CI/CD Rules (PART 27)
<!-- Synchronized with AI.md revision c2f65bf274b5. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 27 (CI/CD Workflows), and `~/.claude/memory/cicd_conventions.md`.

## CRITICAL - NEVER DO
- Pin a third-party Action to a tag or branch — full commit SHA only
- Call the Makefile from a workflow — CI/CD uses explicit commands (PART 25)
- Let `.github/workflows/` and `.gitea/workflows/` drift out of parity
- Keep security jobs (`secret-scan`, `workflow-policy`, `vuln-scan`, `image-scan`) in `ci.yml`; they also run on the weekly schedule while build/test jobs skip scheduled runs
- Skip a platform in a release build — all 8 are required
- Publish a release without running the test suite
- Use floating third-party container images in GitLab or Jenkins; pin by digest and keep the maintained `casjaysdev/go:latest` toolchain as the sole floating exception

## CRITICAL - ALWAYS DO
- Keep all required PART 27 workflows: `ci.yml`, `release.yml`, `beta.yml`, `devel.yml`, and `docker.yml`
- For this GitHub repository, maintain GitHub Actions, Gitea Actions, Forgejo Actions, GitLab CI, and a Jenkinsfile
- Keep GitHub/Gitea/Forgejo workflows in parity, including triggers and provider-specific build metadata
- Build all 8 platforms and upload artifacts in release, beta, and devel workflows; the `ci.yml` build job is a compile gate for the runner platform only
- Run the automated test suite in CI
- Include SHA-pinned action enforcement, `govulncheck`, license verification, and container image scanning; coverage output goes under a scoped OS temp directory outside the repository
- Run TruffleHog with the shared exclude file and `--fail`; scan changed ranges for push/PR events, and scan verified-only full history for schedules and new branches
- Push Docker images to the registry
- Use provider-native caches when available; never mount local host cache paths into hosted jobs
- Set `VERSION`, `COMMIT_ID`, and `BUILD_EPOCH` explicitly only in artifact/image workflows; the compile-only CI job does not need artifact metadata
- Cancel only older runs for the same `main` branch ref or exact tag ref; never cancel another release tag's run
- Verify a staged workflow with `act --list -W {file}` before committing it
- Check the triggered run after every push and fix a red build immediately

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Action pinning? | Full commit SHA, never a tag | PART 27 |
| Makefile in CI? | NEVER | PART 25, 27 |
| Which providers? | GitHub, Gitea, Forgejo, GitLab, and Jenkins | PART 27 |
| Required workflows? | `ci.yml`, `release.yml`, `beta.yml`, `devel.yml`, and `docker.yml` | PART 27 |
| Platform coverage? | All 8 (4 OS × 2 arch) | PART 7, 27 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| parity | GitHub and Gitea workflows kept functionally identical |
| `act` | Local runner used to validate a workflow before commit |
| `devel.yml` | Rolling devel builds |

## QUICK REFERENCE
| Workflow | Purpose |
|----------|---------|
| `release.yml` | Stable releases |
| `beta.yml` | Beta releases |
| `devel.yml` | Rolling devel builds |
| `docker.yml` | Docker image builds |
| `Jenkinsfile` | Jenkins pipeline (required) |
| `.gitlab-ci.yml` | GitLab CI pipeline (required) |

---

For complete details, see AI.md PART 27
