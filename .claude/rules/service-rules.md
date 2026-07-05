# Service Rules (PART 23, 24)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Hard-code privilege assumptions (always check isElevated() dynamically)
- Run permanently as root unless IDEA.md documents a specific exception
- Skip privilege drop after binding privileged ports (Unix)
- Create system user manually (binary auto-creates on first root startup)
- Use password authentication for the search system user (nologin shell)
- Call sudo directly in business logic (use handleEscalation() wrapper)

## CRITICAL - ALWAYS DO

- Service user: search (system user, nologin shell, auto-created by binary)
- Privilege drop: root → search user after binding ports (Unix)
- Platform detection: use build tags (privilege_unix.go, privilege_windows.go)
- Smart escalation: check canEscalate() before prompting
- Service type detection: check actual files before assuming system vs user service

## Service Files

| Platform | System Service Path | User Service Path |
|----------|--------------------|--------------------|
| Linux (systemd) | /etc/systemd/system/search.service | ~/.config/systemd/user/search.service |
| macOS | /Library/LaunchDaemons/io.github.apimgr.search.plist | ~/Library/LaunchAgents/io.github.apimgr.search.plist |
| FreeBSD | /usr/local/etc/rc.d/search | N/A |
| Windows | NT SERVICE\search (Virtual Service Account) | N/A |

## System User Properties

| Property | Value |
|----------|-------|
| Username | search |
| Group | search |
| Shell | /usr/sbin/nologin |
| Home | /var/lib/apimgr/search |
| UID/GID | Auto-assigned (system user, <1000 on Linux) |

## Directory Ownership (Set by root before privilege drop)

```bash
chown -R search:search /etc/apimgr/search/
chown -R search:search /var/lib/apimgr/search/
chown -R search:search /var/cache/apimgr/search/
chown -R search:search /var/log/apimgr/search/
chmod 755 /etc/apimgr/search/
chmod 700 /etc/apimgr/search/security/
chmod 700 /etc/apimgr/search/ssl/
chmod 700 /etc/apimgr/search/tor/
```

## Privilege Detection Pattern

```go
// privilege_unix.go (build tag: !windows)
func isElevated() bool { return os.Geteuid() == 0 }

// privilege_windows.go (build tag: windows)
func isElevated() bool { /* windows.Token admin check */ }
```

## Escalation Flow

```
--service install called
├── Already root? → Install system service
├── canEscalate() (sudo/wheel/admin group)?
│   ├── YES → Prompt "Install system service? Requires sudo. [Y/n]"
│   │         Y → Re-exec with sudo
│   │         N → Install user service
│   └── NO → "No admin access, installing user service..."
└── Done
```

## systemd Unit Template

```ini
[Unit]
Description=Search - Privacy-respecting self-hosted metasearch engine
After=network.target

[Service]
Type=simple
User=search
Group=search
ExecStart=/usr/local/bin/search
Restart=always
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/lib/apimgr/search /var/log/apimgr/search /etc/apimgr/search

[Install]
WantedBy=multi-user.target
```

For complete details, see AI.md PART 23, 24
