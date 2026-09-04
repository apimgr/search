# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 27 (CI/CD Workflows), and `~/.claude/memory/cicd_conventions.md`.

## CRITICAL - NEVER DO
- Pin a third-party Action to a tag or branch — full commit SHA only
- Call the Makefile from a workflow — CI/CD uses explicit commands (PART 25)
- Let `.github/workflows/` and `.gitea/workflows/` drift out of parity
- Create `ci.yml` or `release.yml` before the security-only workflows exist
- Skip a platform in a release build — all 8 are required
- Publish a release without running the test suite

## CRITICAL - ALWAYS DO
- Keep `release.yml` (stable), `beta.yml` (beta), `daily.yml` (nightly), and `docker.yml` (image builds)
- Mirror every workflow into `.gitea/workflows/` (Forgejo-compatible) and keep them in parity
- Maintain a `Jenkinsfile` in addition to the GitHub/Gitea workflows
- Build all 8 platforms and upload the release artifacts
- Run the automated test suite in CI
- Push Docker images to the registry
- Verify a staged workflow with `act --list -W {file}` before committing it
- Check the triggered run after every push and fix a red build immediately

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Action pinning? | Full commit SHA, never a tag | PART 27 |
| Makefile in CI? | NEVER | PART 25, 27 |
| Which providers? | GitHub + Gitea/Forgejo + Jenkins | PART 27 |
| Creation order? | Security workflows first, `ci.yml`/`release.yml` last | PART 27 |
| Platform coverage? | All 8 (4 OS × 2 arch) | PART 7, 27 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| parity | GitHub and Gitea workflows kept functionally identical |
| `act` | Local runner used to validate a workflow before commit |
| security-only workflow | Scanning/lint workflow created before build workflows |

## QUICK REFERENCE
| Workflow | Purpose |
|----------|---------|
| `release.yml` | Stable releases |
| `beta.yml` | Beta releases |
| `daily.yml` | Nightly builds |
| `docker.yml` | Docker image builds |
| `Jenkinsfile` | Jenkins pipeline (required) |

---

For complete details, see AI.md PART 27
