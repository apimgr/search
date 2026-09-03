# Backend Rules (PART 9, 10, 11, 31)

Read: `AI.md` PART 9 (Error Handling & Caching), PART 10 (Database), PART 11 (Security & Logging), PART 31 (Tor Hidden Service).

- Password/backup hashing is Argon2id — never bcrypt.
- Database access lives in `src/database` — follow the schema and migration pattern in PART 10.
- Security middleware, structured logging, and secret handling live in `src/security` / `src/logging` per PART 11.
- Tor hidden service support is REQUIRED and auto-enables when the Tor binary is found — see `src/` Tor integration and PART 31.
