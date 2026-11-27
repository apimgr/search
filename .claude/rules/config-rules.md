# Configuration Rules (PART 5, 6, 12)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Use a YAML config filename other than `server.yml`.
- Hardcode developer defaults or silently accept invalid configuration.
- Store secrets in plaintext or bypass the canonical secret handling.

## CRITICAL - ALWAYS DO
- Use the exact `server.*` configuration keys and documented defaults.
- Validate configuration at startup and on reload; keep all settings configurable.
- Support hot reload through the specified file watcher and preserve safe migration behavior.

## Key rules
- File config is `{config_dir}/server.yml`; split config follows the specified drop-in rules.
- Environment variables and CLI overrides follow the precedence defined in PART 5.
- Keep sample config, docs, migrations, and runtime schema aligned.

For complete details, see AI.md PART 5, 6, 12.
