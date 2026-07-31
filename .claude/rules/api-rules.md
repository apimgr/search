# API Rules (PART 13, 14, 15)

Read: `AI.md` PART 13 (Health & Versioning), PART 14 (API Structure), PART 15 (SSL/TLS & Let's Encrypt).

- Health/version endpoints follow the exact paths and payload shape in PART 13.
- API routes live under `src/api` and `src/graphql`, following the structure in PART 14.
- SSL/TLS and Let's Encrypt integration lives in `src/ssl` per PART 15 — never hand-roll cert handling elsewhere.
- Public URLs in docs/README must use `official_site` (`https://scour.li`), never `localhost`.
