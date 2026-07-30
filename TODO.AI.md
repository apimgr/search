# TODO.AI.md

## Pending

- [ ] src/main.go (line 108) / src/client/cmd/root.go: `--color` flag default is empty string (auto) instead of the explicit string `"auto"` — functionally correct but ambiguous per go-lint convention check
- [ ] src/main_final_test.go: `TestRunMaintenanceBackupWithFilename` (line 548) and `TestRunMaintenanceListWithBackup` (line 597) never call `config.SetBackupDirOverride()`/cleanup like their sibling tests do — they write real backup files into the actual OS-default backup directory (e.g. `/mnt/Backups/apimgr/search`), polluting it across repeated local `go test` runs and causing `TestRunMaintenanceListEmpty` to intermittently fail with leftover files when run after them in the same process; pre-existing (last touched by commit 2f6dcaae24d9, not the recent backup-spec work) — add `config.SetBackupDirOverride(t.TempDir())` + `t.Cleanup` to both tests
- [ ] Makefile (line 97, `local` target): first recipe line is `@rm -rf` (line 98), not `@mkdir -p $(GO_CACHE) $(GO_BUILD)` — mkdir guard must be the absolute first recipe line per CasjaysDev convention
- [ ] Makefile (line 116, `build` target): first recipe line is `@rm -rf` (line 117), not `@mkdir -p $(GO_CACHE) $(GO_BUILD)` — mkdir guard must be the absolute first recipe line per CasjaysDev convention
