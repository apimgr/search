# CI/CD Rules (PART 27)

Read: `AI.md` PART 27 (CI/CD Workflows), and `~/.claude/memory/cicd_conventions.md`.

- Workflows exist for both `.github/workflows/` and `.gitea/workflows/` (Forgejo-compatible) — keep them in parity.
- Third-party Actions pinned to a full commit SHA, never a tag.
- Security-only workflows are created first; `ci.yml`/`release.yml` last.
- `Jenkinsfile` is required in addition to GitHub/Gitea workflows.
