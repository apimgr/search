# Makefile Rules (PART 25)
<!-- Synchronized with AI.md revision f5ea82acc67d. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 25 (Makefile — local dev only, NOT CI/CD).

## CRITICAL - NEVER DO
- Call the Makefile from CI/CD — workflows use explicit commands (PART 27)
- Run `go` directly on the host — every toolchain command goes through the `GO_DOCKER` wrapper
- Hardcode project identity — derive `PROJECT_NAME` and `PROJECT_ORG` from IDEA.md first, then the git remote or directory as specified
- Add a `v` prefix to a text version (dev/beta/devel) or a timestamp version
- Guess a version — read `release.txt`
- Add targets beyond the seven core targets and the two permitted helpers — `dev`, `local`, `build`, `test`, `release`, `beta`, `docker`, plus only `clean` and `release-check`
- Use `.env` files or silently infer the official site from a domain; `site.txt` or `OFFICIAL_SITE` is explicit input
- Copy or symlink generated binaries out of `binaries/` except during the defined release-artifact step

## CRITICAL - ALWAYS DO
- Provide exactly seven core targets: `dev`, `local`, `build`, `test`, `release`, `beta`, `docker`; only `clean` and `release-check` may be added as helpers
- Make `test` delegate to the Docker-only test runner; build/test commands must not invoke Go on the host
- Provide cross-compilation targets covering all 8 platforms
- Inject the version at build time
- Set `CGO_ENABLED=0` and use the maintained `casjaysdev/go:latest` toolchain image for every Go command
- For local builds, honor version precedence: explicitly supplied `VERSION` > non-empty `release.txt` > short commit ID; CI never reads `release.txt`
- Prefix a numeric semver with `v` on the release tag
- Use `$(PWD)` in Makefile docker `-v` flags
- Keep persistent build output only in `binaries/` and release archives in `releases/`; temporary output and coverage/test artifacts go under the project-scoped OS temp directory
- Keep generated dependency-license reports and other command-produced working files in the project-scoped OS temp directory
- Build the server and required CLI for linux, darwin, windows, and freebsd on amd64 and arm64
- Embed version, commit ID, build epoch, and official site only from their defined explicit sources; never infer `OFFICIAL_SITE` from the domain or IDEA.md when operational input is unset

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Makefile in CI/CD? | NEVER — local dev only | PART 25, 27 |
| Host `go` builds? | NEVER — Docker via `GO_DOCKER` | PART 25 |
| Version source? | `release.txt` first | PART 25 |
| `v` prefix? | Numeric semver only | PART 25 |
| Project identity source? | `IDEA.md` first, then git remote or directory; never hardcoded | PART 25 |
| How many targets? | Seven core targets plus only `clean` and `release-check` helpers | PART 25 |
| i18n key validation? | Runs inside `make test`, no separate target | PART 30 |
| Build output? | `binaries/`/`releases/`; temporary build and test output goes under the project-scoped OS temp directory | PART 25, 28 |
| Official site? | `site.txt`, then `OFFICIAL_SITE`, otherwise empty; never guessed or inferred from `IDEA.md`/domain | PART 25 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| `GO_DOCKER` | Makefile wrapper that runs Go toolchain commands in a container |
| `INTERNAL_NAME` | Frozen runtime identity from `IDEA.md`; defaults to `PROJECT_NAME` before IDEA exists |
| version injection | Build-time `-ldflags` stamping of version data |

## QUICK REFERENCE
| Target | Purpose |
|--------|---------|
| `make dev` | Quick dev build to a temp dir |
| `make local` | Versioned build for the host platform |
| `make build` | Build all 8 platforms |
| `make test` | Run tests (includes i18n key validation) with coverage |
| `make release` | Create a release |
| `make beta` | Tag the current `main` commit with the required timestamped beta tag and push it |
| `make docker` | Build the host-platform image and load it into the local image store; CI/CD handles publication |

---

For complete details, see AI.md PART 25
