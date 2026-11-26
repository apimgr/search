# Implementation Backlog

## Priority 1 — Hosted CI failures
- [ ] Fix hosted cleanup permissions and temp path/name conventions in `tests/docker-image-check.sh`.
- [ ] Fix `TestParentDeathSignalStopsManagedChild` in `src/i2p/process_parent_linux_test.go`.
- [ ] Handle unchecked cleanup and close errors in `src/client/surface/catalog.go`, `src/i2p/i2p.go`, `src/i2p/sam.go`, and `src/maintenance_security.go`.
- [ ] Fix the GUI cross-platform build failure while preserving the eight required targets.

## Priority 2 — Full AI.md alignment
- [ ] Finish reconciling all server pages and documentation against AI.md and the implemented behavior.
- [ ] Reconcile all `.claude/rules/` summaries with the current AI.md revision; AI.md is newer than the rule files, and only the binary/config/backend summaries have been refreshed so far.
- [ ] Run the required primary-session tests, build, lint, and CI verification after all changes are integrated.
