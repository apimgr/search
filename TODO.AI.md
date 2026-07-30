# TODO.AI.md

## Pending

- [ ] scripts/build.sh (lines 52-63): raw `docker run ... go build` bypasses `make build` — use `make build` instead
- [ ] scripts/test.sh (lines 79-86): raw `docker run ... go test` bypasses `make test` — use `make test` instead
- [ ] scripts/release.sh (line 75): calls `./scripts/test.sh` which contains raw docker commands — call `make test` instead
- [ ] scripts/release.sh (line 88): calls `./scripts/build.sh` which contains raw docker commands — call `make build` instead
- [ ] src/main.go (line 1850): accepts "macos" as an alias for "darwin" GOOS — binary-rules.md only allows linux/darwin/windows/freebsd; flag and reject macos/mac/osx aliases
- [ ] src/main.go (line 108) / src/client/cmd/root.go: `--color` flag default is empty string (auto) instead of the explicit string `"auto"` — functionally correct but ambiguous per go-lint convention check
- [ ] src/main_final_test.go: `TestRunMaintenanceBackupWithFilename` (line 548) and `TestRunMaintenanceListWithBackup` (line 597) never call `config.SetBackupDirOverride()`/cleanup like their sibling tests do — they write real backup files into the actual OS-default backup directory (e.g. `/mnt/Backups/apimgr/search`), polluting it across repeated local `go test` runs and causing `TestRunMaintenanceListEmpty` to intermittently fail with leftover files when run after them in the same process; pre-existing (last touched by commit 2f6dcaae24d9, not the recent backup-spec work) — add `config.SetBackupDirOverride(t.TempDir())` + `t.Cleanup` to both tests
