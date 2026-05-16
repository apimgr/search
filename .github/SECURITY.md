# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest (main/devel) | Yes |
| Tagged releases | Latest tag only |

Older releases receive no security patches. Always run the latest release.

## Reporting a Vulnerability

**Do NOT open a public GitHub issue for security vulnerabilities.**

Security bugs can expose user search queries, admin credentials, or server infrastructure. Please report them privately.

**Reporting paths (in order of preference):**

1. **GitHub private advisory** — [Submit a private advisory](https://github.com/apimgr/search/security/advisories/new)
2. **Email** — Send details to `apimgr@casjay.cc` with subject `[SECURITY] search - <brief description>`

**What to include:**
- Affected version or commit SHA
- Steps to reproduce
- Impact assessment (what can an attacker do?)
- Any proof-of-concept (attach, do not paste publicly)
- Suggested fix if known

**What to expect:**
- Acknowledgement within 72 hours
- Status update within 7 days
- Fix and coordinated disclosure within 90 days for valid reports

**Important:** search does not log user queries or IPs by design (privacy is the product). If you discover a path that leaks query data, that is a critical vulnerability.

## Security Design Notes

- No server-side query or IP logging for end users
- Admin credentials stored with Argon2id (never bcrypt)
- API tokens stored as SHA-256 hashes
- Image proxy strips Referer headers to prevent third-party tracking
- Tor integration for optional .onion access
- Rate limiting on all endpoints to prevent scraping and abuse
- SSRF defenses on webhook destinations (private CIDR block, scheme allowlist)
- Signed manage tokens (256-bit random) for accountless alert management

See `IDEA.md ## Threat model & abuse cases` and `IDEA.md ## Security decisions & exceptions` for the full threat model.
