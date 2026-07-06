# TODO.AI.md — Outstanding implementation items

## [ ] Email templates missing i18n (HIGH)

Read: AI.md PART 30

**Files affected:**
- `src/email/email.go:307` — `SendAlert()` uses hardcoded English strings
- `src/email/email.go:322` — `SendSecurityAlert()` uses hardcoded English strings
- `src/server/scheduler.go:237-256` — Task failure email body is hardcoded English

**Fix:** Import `github.com/apimgr/search/src/i18n` and use `i18n.T(lang, "email.alert_subject")` pattern for all user-facing email content. Add corresponding keys to `src/i18n/locales/en.json`.

---

## [ ] RepairDatabase path validation (MEDIUM)

Read: AI.md PART 10

**File:** `src/service/maintenance.go:569`

**Issue:** `RepairDatabase(dbPath string)` uses `fmt.Sprintf("VACUUM INTO '%s'", cleanPath)` without validating the path. If called with malicious input, this is a potential SQL injection.

**Fix:** Add path validation using `filepath.Clean()` and ensure path is within allowed directories before using in SQL.
