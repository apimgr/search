# Service Rules (PART 23, 24)
<!-- Synchronized with AI.md revision f5ea82acc67d. -->

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 23 (Privilege Escalation & Service), PART 24 (Service Support).

## CRITICAL - NEVER DO
- Invoke `sudo` or escalate privilege at runtime unless PART 23 explicitly authorizes it for that operation
- Escalate silently — the prompt and the reason must be visible to the operator
- Assume privileged paths when running unprivileged, or vice versa
- Skip a supported platform's service integration
- Write a service unit outside `src/service`
- Use a reserved or colliding UID/GID for the dedicated service account; UID and GID must match
- Run as root permanently unless IDEA.md explicitly documents the exception

## CRITICAL - ALWAYS DO
- Document the privilege requirements of every protected operation
- Prompt via `sudo`/`runas` only when the operation genuinely needs it
- Gate service operations behind the appropriate rights
- Implement `--service --install`, `--service --uninstall`, and `--service start|stop|restart|reload`
- Detect and support systemd, OpenRC, SysVinit, runit, and s6 on Linux; launchd on macOS; Windows Service Control Manager; and rc.d on BSD
- Use the default scope and path rules from PART 23-24 when choosing between system-wide and user-scoped installation
- Create the dedicated `{internal_name}` account/group and owned runtime directories before dropping privileges; UID/GID match within the documented 200–899 range, with a nologin shell and config/data home
- Cache privilege/path mode at process start; later privilege changes must not silently switch the process between system and user directory trees
- Keep service start/stop/restart/reload operations aligned with the host manager and return command failures to the operator
- Detect the running host manager in the documented priority order (OpenRC before SysVinit); an installed but non-running systemd binary does not make the host systemd
- Preserve staged-update restart behavior: supervised services restart after exit `75`; OpenRC and SysVinit apply the staged update on the next start/restart and notify the operator

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Runtime `sudo`? | Only where PART 23 authorizes it | PART 23 |
| Unit file location? | `src/service` | PART 24 |
| macOS plist name? | `io.github.{project_org}.{internal_name}` | PART 23-24 |
| Service subcommands? | install, uninstall, start, stop, restart, reload | PART 24 |
| Linux managers? | systemd, OpenRC, SysVinit, runit, s6 | PART 24 |
| Service account identity? | Frozen `internal_name`; equal, non-reserved UID/GID where applicable | PART 23 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Operator | Person deploying and managing the server via CLI and `server.yml` |
| privileged operation | Action requiring root/Administrator rights |
| scope | System-wide vs per-user service installation |

## QUICK REFERENCE
| Platform | Service mechanism |
|----------|-------------------|
| Linux | systemd, OpenRC, SysVinit, runit, or s6 |
| macOS | launchd plist |
| Windows | Windows Service |
| BSD | `rc.d` script |

---

For complete details, see AI.md PART 23, 24
