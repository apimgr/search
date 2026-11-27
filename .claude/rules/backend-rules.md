# Backend Rules (PART 9, 10, 11, 31)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Leak internal errors, credentials, tokens, or sensitive request data.
- Use bcrypt for configuration/backup passwords or store those passwords in plaintext.
- Make Tor/I2P required for normal server startup.

## CRITICAL - ALWAYS DO
- Validate all input server-side, use typed errors, and apply the prescribed cache behavior.
- Use the canonical database schema, migrations, and transaction boundaries.
- Hash login passwords with Argon2id; encrypt backup passwords at rest; hash API tokens with SHA-256.
- Auto-enable Tor support when its binary is found; follow the specified I2P defaults.

## Key rules
- Follow PART 9 error handling and cache TTL/invalidation requirements.
- Keep database access behind the specified data layer and secure secret rotation.

For complete details, see AI.md PART 9, 10, 11, 31.
