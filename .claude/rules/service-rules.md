# Service Rules (PART 23, 24)

Read: `AI.md` PART 23 (Privilege Escalation & Service), PART 24 (Service Support).

- Never invoke `sudo` or escalate privilege at runtime unless PART 23 explicitly authorizes it for that specific operation.
- Service unit files (systemd, launchd, etc.) live under `src/service` and follow the default scope/path rules in PART 23-24.
