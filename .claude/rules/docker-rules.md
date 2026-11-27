# Docker Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Place a Dockerfile in the repository root.
- Run as root in the final application image or use an unpinned, unsupported build flow.
- Put build/runtime secrets in image layers.

## CRITICAL - ALWAYS DO
- Store Docker build files under `docker/` and follow the multi-stage image requirements.
- Use a minimal runtime image, non-root user, health checks, and only required files.
- Keep compose, image labels, build args, and docs consistent with the actual image.

## Key rules
- Build locally without publishing; CI owns image publication.
- Verify the produced image using the project image checks.

For complete details, see AI.md PART 26.
