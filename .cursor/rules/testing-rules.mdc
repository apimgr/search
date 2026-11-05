# Testing & Docs Rules (PART 28, 29, 30)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 28 (Testing & Development), PART 29 (ReadTheDocs Documentation), PART 30 (I18N & A11Y).

## CRITICAL - NEVER DO
- Write coverage or test output into the project tree
- Commit with a failing test — every test must pass in CI
- Put anything but MkDocs content in `docs/`
- Hardcode a user-facing string in a template or handler — it belongs in `src/common/i18n`
- Ship UI that fails WCAG 2.1 AA
- Skip the documentation badge in `README.md`

## CRITICAL - ALWAYS DO
- Keep tests in `tests/` and alongside packages as `_test.go`
- Provide `tests/run_tests.sh` (auto-detects environment), `tests/docker.sh`, `tests/incus.sh`, and `tests/e2e.sh` (browser E2E, on demand)
- Cover both unit and integration tests, including API testing, and measure coverage
- Document the beta testing procedures
- Build docs with `mkdocs.yml` and `.readthedocs.yaml` in the project root, with `docs/requirements.txt` for dependencies
- Use the MkDocs Material theme with a light/dark/auto toggle, dark by default
- Keep the required pages: `docs/index.md`, `installation.md`, `configuration.md`, `api.md`, `security.md`, `integrations.md`, `development.md`
- Keep I18N strings in `src/common/i18n`, support RTL where applicable, and test accessibility

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Test location? | `tests/` plus package `_test.go` | PART 28 |
| Coverage output? | Outside the project tree | PART 28 |
| `docs/` contents? | MkDocs only | PART 29 |
| Docs theme? | MkDocs Material, dark default | PART 29 |
| Accessibility bar? | WCAG 2.1 AA | PART 30 |
| I18N string location? | `src/common/i18n` | PART 30 |

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
