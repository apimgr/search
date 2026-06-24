# TODO.AI.md — Outstanding bootstrap items

## [x] Replace forbidden scheduler library

Completed. `src/scheduler/scheduler.go` was rewritten to use `github.com/go-co-op/gocron/v2` directly. `github.com/robfig/cron/v3` has no direct imports in the codebase; it only appears as an indirect transitive dependency of gocron/v2.

## [x] Remove spf13/viper from codebase

Completed. `github.com/spf13/viper` has been fully removed from `go.mod` and all source files. The client configuration layer was replaced with a custom `src/client/clicfg` package that provides a compatible API using `gopkg.in/yaml.v3` directly. No direct viper imports remain anywhere in the codebase.

## [x] Move gocron/v2 and required libs from indirect to direct in go.mod

Completed. `github.com/go-co-op/gocron/v2` is now a direct dependency. `github.com/go-playground/validator/v10`, `github.com/rs/cors`, and `golang.org/x/time` are present in `go.mod` as required.

## [x] Raise test coverage to spec minimum of 80%

Completed. `make test` passes with **82.9%** total coverage (threshold ≥80% met). All packages except `src/service` (excluded — drops privileges mid-test) and `src/main` (practical ceiling ~19%) are at or above 80%. Committed in `cd9d4a2095f6`.

## [x] Fix Makefile coverage output path

Completed. `make test` now uses `mktemp -d "/tmp/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX"` and mounts it as `/tmp/covout` inside the container. Coverage output never touches the project tree. Committed in the same session.
