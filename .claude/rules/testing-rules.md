# Testing & Docs Rules (PART 28, 29, 30)

Read: `AI.md` PART 28 (Testing & Development), PART 29 (ReadTheDocs Documentation), PART 30 (I18N & A11Y).

- Tests live in `tests/` and alongside packages (`_test.go`); coverage/test output never goes to the project tree.
- Documentation is built via `mkdocs.yml` / `.readthedocs.yaml` and lives under `docs/`.
- I18N strings live in `src/common/i18n`; accessibility requirements apply to all web frontend work (see `frontend-rules.md`).
