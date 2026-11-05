# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 27 (CI/CD Workflows), and `~/.claude/memory/cicd_conventions.md`.

## CRITICAL - NEVER DO
- Pin a third-party Action to a tag or branch — full commit SHA only
- Call the Makefile from a workflow — CI/CD uses explicit commands (PART 25)
- Let `.github/workflows/` and `.gitea/workflows/` drift out of parity
- Do not create a separate security-only workflow; keep the required security checks in `ci.yml`
- Skip a platform in a release build — all 8 are required
- Publish a release without running the test suite

## CRITICAL - ALWAYS DO
- Keep the workflows required by PART 27: `ci.yml` and `release.yml`; add `beta.yml`, `devel.yml`, and `docker.yml` only when the project requires them
- For this GitHub repository, maintain the corresponding Gitea and Forgejo workflows in parity
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
| Required workflows? | `ci.yml` and `release.yml`; others are project-specific | PART 27 |
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

---

For complete details, see AI.md PART 27
