## [ ] Add `.gitea/workflows/daily.yml` and `.gitea/workflows/docker.yml` to match `.github/workflows/`
Read: AI.md PART 27
Project already opted into `daily.yml` and `docker.yml` on GitHub — Gitea/Forgejo workflows are missing the equivalents, breaking provider parity. Must use Gitea Actions syntax (see AI.md PART 27 `.gitea/workflows/daily.yml` and `.gitea/workflows/docker.yml` reference sections), SHA-pinned third-party actions, and pass `act --list -W {file}` before commit.

## [ ] Decide whether `beta.yml` is needed (GitHub and Gitea)
Read: AI.md PART 27
No `beta` branch currently exists in the repo. `beta.yml` is optional/project-specific per AI.md PART 27 — only add if a beta release channel is actually adopted. If adopted, add to both `.github/workflows/beta.yml` and `.gitea/workflows/beta.yml` with workflow concurrency per the branch-push auto-cancel policy.

## [ ] Add `image-scan` (Trivy) job to all providers' security workflows
Read: AI.md PART 27 (lines ~815, ~818)
AI.md mandates an `image-scan` (Trivy) job conditional on a Dockerfile being present, running after the image build — `docker/Dockerfile` exists, so it is required. No provider currently runs Trivy. Substantive addition: the job must build the production image from `docker/Dockerfile`, run a Trivy scan (fail on HIGH/CRITICAL), and pass `act --list -W {file}` before commit. Add to `.github`, `.gitea`, and `.forgejo` `security.yml` (or `ci.yml` per the PART 27 job-ordering table), SHA-pinning any new third-party action. Deferred from the 2026-07-30 audit because it needs a CI docker-build step that could not be validated in the audit environment.
