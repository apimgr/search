# Makefile Rules (PART 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 25 (Makefile — local dev only, NOT CI/CD).

## CRITICAL - NEVER DO
- Call the Makefile from CI/CD — workflows use explicit commands (PART 27)
- Run `go` directly on the host — every toolchain command goes through the `GO_DOCKER` wrapper
- Hardcode `PROJECTNAME`, `PROJECTORG`, or `BINARY` — always infer from the git remote or the directory
- Add a `v` prefix to a text version (dev/beta/devel) or a timestamp version
- Guess a version — read `release.txt`
- Add a seventh target — the six core targets are the complete set

## CRITICAL - ALWAYS DO
- Provide exactly six targets: `dev`, `local`, `build`, `test`, `release`, `docker`
- Provide cross-compilation targets covering all 8 platforms
- Inject the version at build time
- Honor version precedence: `release.txt` > env var > default fallback
- Prefix a numeric semver with `v` on the release tag
- Use `$(PWD)` in Makefile docker `-v` flags
- Keep build output out of the repo tree

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Makefile in CI/CD? | NEVER — local dev only | PART 25, 27 |
| Host `go` builds? | NEVER — Docker via `GO_DOCKER` | PART 25 |
| Version source? | `release.txt` first | PART 25 |
| `v` prefix? | Numeric semver only | PART 25 |
| Project name source? | git remote or directory, never hardcoded | PART 25 |
| How many targets? | Exactly six — `dev`, `local`, `build`, `test`, `release`, `docker` | PART 25 |
| i18n key validation? | Runs inside `make test`, no separate target | PART 30 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| `GO_DOCKER` | Makefile wrapper that runs Go toolchain commands in a container |
| `BINARY` | Inferred output binary name |
| version injection | Build-time `-ldflags` stamping of version data |

## QUICK REFERENCE
| Target | Purpose |
|--------|---------|
| `make dev` | Quick dev build to a temp dir |
| `make local` | Versioned build for the host platform |
| `make build` | Build all 8 platforms |
| `make test` | Run tests (includes i18n key validation) with coverage |
| `make release` | Create a release |
| `make docker` | Build and push the Docker image |

---

For complete details, see AI.md PART 25
