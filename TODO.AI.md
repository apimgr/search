# TODO.AI.md

## Pending

- [ ] scripts/build.sh (lines 52-63): raw `docker run ... go build` bypasses `make build` — use `make build` instead
- [ ] scripts/test.sh (lines 79-86): raw `docker run ... go test` bypasses `make test` — use `make test` instead
- [ ] scripts/release.sh (line 75): calls `./scripts/test.sh` which contains raw docker commands — call `make test` instead
- [ ] scripts/release.sh (line 88): calls `./scripts/build.sh` which contains raw docker commands — call `make build` instead
- [ ] src/main.go (line 1850): accepts "macos" as an alias for "darwin" GOOS — binary-rules.md only allows linux/darwin/windows/freebsd; flag and reject macos/mac/osx aliases
