# Testing Rules (PART 28, 29, 30)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Run Go tests, builds, or binaries on the host.
- Let tests make uncontrolled outbound network requests or write artifacts into tracked source.
- Leave docs, translations, accessibility, or server-rendered pages out of compliance checks.

## CRITICAL - ALWAYS DO
- Run unit tests through the official Docker toolchain and enforce the configured coverage floor.
- Use local test servers for outbound dependencies and clean project-scoped temporary artifacts.
- Keep ReadTheDocs documentation, translations, accessibility, and rendered pages aligned with behavior.
- Run required generators before checking generated API/schema artifacts.

## Key rules
- CI spells out commands; local `make test` follows PART 25.
- Test actual behavior and report command results truthfully.

For complete details, see AI.md PART 28, 29, 30.
