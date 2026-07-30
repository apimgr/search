# Service Rules (PART 23, 24)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Never prompt for privilege escalation if the user cannot actually escalate
  (not in sudoers/wheel/admin) — show an informative error instead.
- Never reuse a reserved/well-known UID/GID (65534, 999-980, 101-110,
  170-179, etc.) even if it looks free on the current system.
- Never assign UID and GID different values — they MUST match.
- Never run the service permanently as root/Administrator unless IDEA.md
  explicitly approves it (and if so, the service file + docs must say why
  privilege drop is impossible).
- Never let `--service --uninstall` skip the confirmation prompt before
  deleting all data, configs, and the system user.
- Never leave the service running as root after binding privileged ports —
  drop privileges immediately after bind (Unix-like).
- Never use Local System, Administrator, or a logged-in user account for
  Windows services — Virtual Service Account (VSA) is the default.
- Never install more than one init system's service file on the same host
  (e.g. both OpenRC and SysVinit) — detect and pick exactly one.

## CRITICAL - ALWAYS DO

- Always check EUID==0 / admin token first; skip the escalation prompt
  entirely if already privileged.
- Always try escalation methods in OS-defined order (Linux: sudo, su,
  pkexec, doas; macOS: sudo, osascript; BSD: doas, sudo, su; Windows: UAC,
  runas).
- Always let the binary itself handle user/group creation, privilege
  escalation, directory setup, and permissions during normal startup —
  `--service --install` only installs, enables, and starts the service.
- Always fall back to a user-level service (systemd --user, launchctl
  user agent) when the caller cannot get system-level privileges.
- Always create a dedicated service user/group by default, in the safe
  UID/GID range, searching top-down and skipping reserved IDs.
- Always keep config/data/cache/log/user intact on `--service --disable`;
  only `--uninstall` deletes them.
- Always print "Binary remains - delete manually: rm {binary_path}" after
  uninstall — the binary itself is never removed automatically.

## Key Rules Summary

**Privilege escalation**
- Detection order is OS-specific (see NEVER/ALWAYS above); never invent a
  different order.
- Escalation is used only for install/start (privileged port bind,
  service file placement); the running server drops privileges afterward.

**Default scope / service commands**
- `--service --install`: install + enable + start (system if privileged,
  else user-level fallback).
- `--service --disable`: stop + disable, keep data/user/service file.
- `--service --uninstall`: stop + disable + remove service file + delete
  all data dirs + delete system user/group; requires y/N confirmation;
  binary is left in place.

**UID/GID conventions**
- Username = group = `{internal_name}`; UID == GID, matching value.
- Linux/BSD safe range: 200-899, search from 899 down; macOS: 200-399,
  search from 399 down.
- Shell: `/sbin/nologin` (or `/usr/sbin/nologin`, `/usr/bin/false` macOS).
- Home dir: config dir by default, or data dir if it needs user-writable
  content; directory must exist before user creation.
- Windows: no manual user — Virtual Service Account
  `NT SERVICE\{internal_name}`, auto-managed, minimal privilege.

**systemd / launchd / polkit conventions**
- systemd unit at `/etc/systemd/system/{internal_name}.service`, hardened
  (`ProtectSystem=strict`, `ProtectHome=yes`, `PrivateTmp=yes`,
  explicit `ReadWritePaths=` for config/data/cache/log dirs),
  `Restart=on-failure`, journal logging.
- launchd plist at `/Library/LaunchDaemons/{plist_name}.plist`, starts as
  root (no UserName/GroupName key), binary drops to `{project_name}` user
  after binding; managed via `launchctl load|unload`.
- OpenRC (`/etc/init.d/{internal_name}`), SysVinit (same path, mutually
  exclusive with OpenRC), runit (`/etc/sv/{internal_name}/`), and FreeBSD
  rc.d (`/usr/local/etc/rc.d/{internal_name}`) each get exactly one
  service file per host, chosen by init-system detection.
- PolicyKit (`pkexec`) is a Linux escalation fallback, tried after sudo/su
  and before doas.

**Service install/uninstall behavior**
- All projects must ship built-in support for every relevant init system
  and Windows Services — no external packaging step required.
- Server binary (not the `--service` flag) owns user creation, privilege
  drop, and directory/permission setup, every startup.
- Any port is usable without editing the service file — the binary binds
  as root/admin then drops privileges.

For complete details, see AI.md PART 23, 24.
