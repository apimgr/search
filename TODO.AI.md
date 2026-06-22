# TODO.AI.md — Outstanding bootstrap items

## [ ] Replace forbidden scheduler library

`src/scheduler/scheduler.go` and its test file import `github.com/robfig/cron/v3`, which is explicitly forbidden by the spec. Replace all usage with `github.com/go-co-op/gocron/v2` (already added to go.mod). The scheduler package must be rewritten to use gocron/v2's API. All tests must continue to pass after the migration.

Read: AI.md PART 18

## [ ] Remove spf13/viper from codebase

`github.com/spf13/viper` is forbidden by the spec; config must use direct YAML parsing with `gopkg.in/yaml.v3` only. The following files import viper and must be migrated:
- `src/client/cmd/root.go`
- `src/client/cmd/root_test.go`
- `src/client/init_test.go`
- `src/client/logging.go`
- `src/client/cache.go`
- `src/client/cache_test.go`
- `src/client/logging_test.go`
- `src/client/cmd/status_test.go`

After migration, remove `github.com/spf13/viper` from `go.mod` with `go mod tidy`.

Read: AI.md PART 5

## [ ] Move gocron/v2 and required libs from indirect to direct in go.mod

After the scheduler and client migrations above are complete, run `go mod tidy` inside Docker to promote `github.com/go-co-op/gocron/v2`, `github.com/go-playground/validator/v10`, `github.com/rs/cors`, and `golang.org/x/time` from indirect to direct dependencies (once they are imported in source code).

Read: AI.md PART 3

## [ ] Raise test coverage to spec minimum of 80%

Current total coverage is ~39.6% (measured across `./src/...`, excluding `src/service` which drops privileges mid-test). The spec (AI.md PART 28) requires ≥80%. The CI threshold is currently set to 38% as a floor.

Packages needing significant improvement: `src/main` (0%), `src/database` (0%), `src/instant` (14%), `src/server` (18%), `src/direct` (24.5%), `src/geoip` (30.3%).

Read: AI.md PART 28

## [ ] Fix Makefile coverage output path

`make test` writes `coverage.out` to the project tree (project root) at line 203 of the Makefile. The spec requires coverage output to go to `/tmp/apimgr/search-XXXXXX/coverage.out` (never to project tree). Update the `test` target in `Makefile`: create a temp dir with `mktemp -d "/tmp/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX"` and pass that path as `-coverprofile=$$COVDIR/coverage.out`.

Read: AI.md PART 26
