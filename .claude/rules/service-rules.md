# Service Rules (PART 23, 24)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Require root for routine server operation or continue root-only work after privilege drop.
- Guess service manager behavior or modify host service configuration as a workaround.

## CRITICAL - ALWAYS DO
- Complete required privileged initialization before the final privilege drop.
- Detect supported service managers and use exact platform-specific user, unit, and path rules.
- Keep service install/status/start/stop behavior safe, reversible, and documented.

## Key rules
- Preserve the rule that nothing after privilege drop requires root.
- Test service lifecycle in the prescribed full OS environment.

For complete details, see AI.md PART 23, 24.
