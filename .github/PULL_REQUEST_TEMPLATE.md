## Summary

<!-- What does this PR do? One paragraph or bullet list. -->

## Why

<!-- Why is this change needed? What problem does it solve? -->

## Test Evidence

<!-- How was this tested? Attach logs, screenshots, or describe the test steps.
     Unit tests: `make test`
     Integration tests: `./tests/run_tests.sh` -->

## Documentation / Config Updates

- [ ] `IDEA.md` updated if features/data models changed
- [ ] `docs/` pages updated if user/admin/API/config behavior changed
- [ ] Swagger annotations updated if routes changed

## Breaking Change

- [ ] This is a breaking change
  - Describe migration path:
- [ ] No breaking changes

## Security / Privacy Impact

- [ ] This change affects authentication, session handling, or credential storage
- [ ] This change affects data retention, query logging, or user privacy
- [ ] This change modifies rate limiting, CSRF/XSS/path security, or input validation
- [ ] No security/privacy impact

## Checklist

- [ ] No placeholder/stub/TODO behavior was introduced
- [ ] No `strconv.ParseBool()` — used `config.ParseBool()` instead
- [ ] No `bcrypt` — used `Argon2id` for any new password hashing
- [ ] No server-side logging of user queries or IPs
- [ ] `CGO_ENABLED=0` — no CGO added
- [ ] All user-facing strings added to all 7 locale files (`en`, `es`, `fr`, `de`, `zh`, `ar`, `ja`)
