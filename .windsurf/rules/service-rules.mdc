# Service Rules (PART 23, 24)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 23 (Privilege Escalation & Service), PART 24 (Service Support).

## CRITICAL - NEVER DO
- Invoke `sudo` or escalate privilege at runtime unless PART 23 explicitly authorizes it for that operation
- Escalate silently — the prompt and the reason must be visible to the operator
- Assume privileged paths when running unprivileged, or vice versa
- Skip a supported platform's service integration
- Write a service unit outside `src/service`

## CRITICAL - ALWAYS DO
- Document the privilege requirements of every protected operation
- Prompt via `sudo`/`runas` only when the operation genuinely needs it
- Gate service operations behind the appropriate rights
- Implement `--service --install`, `--service --uninstall`, and `--service start|stop|restart|reload`
- Generate the correct unit for the host: systemd on Linux, launchd plist on macOS, Windows Service on Windows, `rc.d` script on BSD
- Use the default scope and path rules from PART 23-24 when choosing between system-wide and user-scoped installation

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Runtime `sudo`? | Only where PART 23 authorizes it | PART 23 |
| Unit file location? | `src/service` | PART 24 |
| macOS plist name? | `io.github.{project_org}.{internal_name}` | PART 23-24 |
| Service subcommands? | install, uninstall, start, stop, restart, reload | PART 24 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| Operator | Person deploying and managing the server via CLI and `server.yml` |
| privileged operation | Action requiring root/Administrator rights |
| scope | System-wide vs per-user service installation |

## QUICK REFERENCE
| Platform | Service mechanism |
|----------|-------------------|
| Linux | systemd unit |
| macOS | launchd plist |
| Windows | Windows Service |
| BSD | `rc.d` script |

---

For complete details, see AI.md PART 23, 24
