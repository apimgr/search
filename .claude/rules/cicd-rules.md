# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use Makefile targets in CI/CD.
- Skip supported provider/platform workflows, security checks, or release verification.
- Embed credentials or use mutable third-party actions without the prescribed pinning.

## CRITICAL - ALWAYS DO
- Use explicit toolchain-container commands in workflows.
- Keep CI behavior aligned across supported providers and use least-privilege permissions.
- Build/test all required platforms and publish only through the documented workflows.

## Key rules
- Workflows must match current project commands and generated artifact names.
- Check CI runs after a push and resolve failures before reporting completion.

For complete details, see AI.md PART 27.
