## [ ] Add `.gitea/workflows/daily.yml` and `.gitea/workflows/docker.yml` to match `.github/workflows/`
Read: AI.md PART 27
Project already opted into `daily.yml` and `docker.yml` on GitHub — Gitea/Forgejo workflows are missing the equivalents, breaking provider parity. Must use Gitea Actions syntax (see AI.md PART 27 `.gitea/workflows/daily.yml` and `.gitea/workflows/docker.yml` reference sections), SHA-pinned third-party actions, and pass `act --list -W {file}` before commit.

## [ ] Decide whether `beta.yml` is needed (GitHub and Gitea)
Read: AI.md PART 27
No `beta` branch currently exists in the repo. `beta.yml` is optional/project-specific per AI.md PART 27 — only add if a beta release channel is actually adopted. If adopted, add to both `.github/workflows/beta.yml` and `.gitea/workflows/beta.yml` with workflow concurrency per the branch-push auto-cancel policy.

## [ ] `desmos` bang has no functional search/query URL
Read: `src/search/bang/defaults.go` (line ~630)
The `desmos` bang entry (`{Shortcut: "desmos", Name: "Desmos", URL: "https://www.desmos.com/calculator?lang=en", Category: "misc"}`) has no `{query}`/`%s` placeholder — it always opens the bare calculator, ignoring the user's search terms. Desmos has no public search/query URL endpoint to redirect to, so this can't be fixed by changing the URL template alone. Decide: (a) leave as a "quick launch" bang with updated `Description` clarifying it ignores query text, (b) remove the bang entirely, or (c) find/document an alternate Desmos endpoint that accepts a query param.
