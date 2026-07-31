# Feature Rules (PART 17-22)

Read: `AI.md` PART 17 (Email & Notifications), PART 18 (Scheduler), PART 19 (GeoIP), PART 20 (Metrics), PART 21 (Backup & Restore), PART 22 (Update Command).

- Email/notifications live in `src/email` (templates in `src/email/templates`).
- Scheduling is built-in (`src/scheduler`) — never rely on external cron.
- GeoIP lookups live in `src/geoip`.
- Metrics live in `src/graphql`/metrics endpoints per PART 20 — see also `api-rules.md` for `/server/healthz` and `/metrics`.
- Backup/restore logic lives in `src/backup`.
- Self-update logic lives in `src/update`.
