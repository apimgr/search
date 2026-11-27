# Testing & Docs Rules (PART 28, 29, 30)
<!-- Synchronized with AI.md revision c2f65bf274b5. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 28 (Testing & Development), PART 29 (ReadTheDocs Documentation), PART 30 (I18N & A11Y).

## CRITICAL - NEVER DO
- Write runtime data, coverage, test output, screenshots, logs, or failure artifacts into the project tree
- Commit with a failing test — every test must pass in CI
- Put anything but MkDocs content in `docs/`
- Hardcode a user-facing string in a template or handler — it belongs in `src/common/i18n`
- Ship UI that fails WCAG 2.1 AA
- Skip the documentation badge in `README.md`
- Run Go commands on the host, or run human-only production/development Compose files in automated testing
- Run `docker-compose.test.yml` from the repository tree; copy it into the project-scoped temporary workspace first
- Treat exploratory browser runs or E2E as a commit/CI gate; they are on-demand and every confirmed finding must become a deterministic test

## CRITICAL - ALWAYS DO
- Keep tests in `tests/` and alongside packages as `_test.go`
- Use two required test phases: the `make test` unit/route commit gate and manual binary validation through `tests/*.sh`; `tests/run_tests.sh` requires Docker and runs only the basic Docker tier
- Provide `tests/run_tests.sh` (Docker-only), `tests/docker.sh`, `tests/incus.sh`, `tests/e2e.sh` (browser E2E, on demand), and `tests/virt.sh` when a clean OS test is needed
- Keep `tests/run_tests.sh` Docker-only and delegating to `tests/docker.sh`; build available binaries in one Docker phase into its task-scoped temp mount, not the repository `binaries/` tree; use `tests/incus.sh` for Incus coverage
- Cover both unit and integration tests, including API testing, and measure coverage
- Enforce at least 60% statement coverage; Go coverage data is collected with `-test.gocoverdir` and converted with `go tool covdata textfmt`
- Document the beta testing procedures
- Build docs with `mkdocs.yml` and `.readthedocs.yaml` in the project root, with `docs/requirements.txt` for dependencies
- Keep ReadTheDocs hosted documentation and its README badge current; `docs/` must explain every shipped browser, CLI, API, config, discovery, and public integration surface
- Use the MkDocs Material theme with a light/dark/auto toggle, dark by default
- Keep the required pages: `docs/index.md`, `installation.md`, `configuration.md`, `api.md`, `security.md`, `integrations.md`, `development.md`, `translating.md`, plus `cli.md` when applicable
- Keep every human-readable string in `src/common/i18n`; all binaries use the same complete embedded locales and registry, with English fallback and consent-gated `?lang=` persistence
- Require exact key/plural parity across locales and the required language minimum; support RTL and test accessibility against WCAG 2.1 AA
- Test keyboard use, skip links, labels, focus, theme variants, RTL layout, reduced motion, and non-JavaScript behavior
- Render every registered page and themed HTTP error status in the page-conformance suite; include all required statuses from 400 through 503
- Keep docs aligned with every shipped browser, CLI, API, configuration, discovery, and public integration surface

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Test location? | `tests/` plus package `_test.go` | PART 28 |
| Coverage output? | Outside the project tree | PART 28 |
| `docs/` contents? | MkDocs only | PART 29 |
| Docs theme? | MkDocs Material, dark default | PART 29 |
| Accessibility bar? | WCAG 2.1 AA | PART 30 |
| I18N string location? | `src/common/i18n` | PART 30 |
| Test execution? | Container-only Go commands; temp output is cleaned after use | PART 28 |
| Automated Compose? | Copy test Compose to scoped temp, use a named bridge network and tmpfs cache, and invoke via test wrappers | PART 26, 28 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| E2E | Browser end-to-end test run on demand via `tests/e2e.sh` |
| A11Y | Accessibility (WCAG 2.1 AA) |
| I18N | Internationalization, including RTL support |

## QUICK REFERENCE
| Script | Environment |
|--------|-------------|
| `tests/run_tests.sh` | Auto-detect |
| `tests/docker.sh` | Docker |
| `tests/incus.sh` | Incus |
| `tests/e2e.sh` | Browser E2E (on demand) |

Root docs files: `mkdocs.yml`, `.readthedocs.yaml`, `docs/requirements.txt`

---

For complete details, see AI.md PART 28, 29, 30
