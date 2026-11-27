# Binary Rules (PART 7, 8, 32)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Enable CGO or build Go binaries on the host.
- Omit required server/client flags, version metadata, or supported platforms.
- Implement client-side application behavior that belongs in the server.

## CRITICAL - ALWAYS DO
- Build with `CGO_ENABLED=0` in the prescribed toolchain container.
- Provide the `search` server and required `search-cli` companion.
- Keep CLI behavior, help, errors, and completion surfaces consistent with the spec.

## Key rules
- Build all 8 OS/architecture combinations listed in PART 7.
- Use the canonical binary names and embed only allowed build metadata.
- Keep server processing authoritative; the client presents server results.

For complete details, see AI.md PART 7, 8, 32.
