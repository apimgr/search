# TODO.AI.md - AI Task Tracking

## Current Status

AI.md refreshed from TEMPLATE.md with project-specific placeholders replaced:
- projectname = search
- projectorg = apimgr

## Completed Tasks

- [x] Copy TEMPLATE.md to AI.md
- [x] Replace placeholders ({projectname} → search, {projectorg} → apimgr)
- [x] Read AI.md PART 0-5 and commit critical rules to memory
- [x] Create .claude/rules/ directory with all 14 rule files:
  - ai-rules.md (PART 0, 1)
  - project-rules.md (PART 2, 3, 4)
  - config-rules.md (PART 5, 6, 12)
  - binary-rules.md (PART 7, 8, 33)
  - backend-rules.md (PART 9, 10, 11, 32)
  - api-rules.md (PART 13, 14, 15)
  - frontend-rules.md (PART 16, 17)
  - features-rules.md (PART 18-23)
  - service-rules.md (PART 24, 25)
  - makefile-rules.md (PART 26)
  - docker-rules.md (PART 27)
  - cicd-rules.md (PART 28)
  - testing-rules.md (PART 29, 30, 31)
  - optional-rules.md (PART 34-36)

## Critical Rules Committed to Memory

### NEVER DO
- ❌ Use bcrypt (use Argon2id)
- ❌ Put Dockerfile in root (use docker/Dockerfile)
- ❌ Use CGO (CGO_ENABLED=0 always)
- ❌ Hardcode dev machine values
- ❌ Use external cron (use built-in scheduler)
- ❌ Use GPL/AGPL/LGPL dependencies
- ❌ Client-side rendering (use server-side Go templates)
- ❌ Run Go locally (use make dev/local/build/test)
- ❌ Guess or assume (read spec or ask)
- ❌ Use Makefile in CI/CD
- ❌ Skip any of 8 platforms
- ❌ Create forbidden files (SUMMARY.md, COMPLIANCE.md, etc.)

### ALWAYS DO
- ✅ Read AI.md PART 0, 1 at conversation start
- ✅ Read relevant PART before implementing
- ✅ Argon2id for password hashing
- ✅ CGO_ENABLED=0 for all builds
- ✅ All 8 platforms: linux, darwin, windows, freebsd × amd64, arm64
- ✅ MIT License only
- ✅ Server-side rendering with Go templates
- ✅ Mobile-first responsive CSS
- ✅ Follow spec EXACTLY

## Next Steps

See IDEA.md for project-specific features and implementation tasks.
