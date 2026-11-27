# Makefile Rules (PART 25)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Add Makefile targets beyond the six specified targets.
- Call Makefile targets from CI/CD workflows.
- Build Go binaries on the host.

## CRITICAL - ALWAYS DO
- Keep `help` as the default goal and implement only `build`, `test`, `release`, `docker`, `clean`, `help`.
- Run builds and tests through the prescribed Docker Go toolchain with CGO disabled.
- Keep outputs in the documented locations and ensure cleanup is targeted.

## Key rules
- CI uses explicit commands; Makefile is for local development only.
- `test` performs the prescribed lint, vulnerability, coverage, and binary checks.

For complete details, see AI.md PART 25.
