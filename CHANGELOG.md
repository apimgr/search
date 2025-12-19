# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-12-17

### Added

#### Search Engines
- Google, Bing, DuckDuckGo, Yahoo, Brave Search, Startpage, Qwant
- Wikipedia for knowledge results
- YouTube for video search
- Reddit for social/news
- GitHub and Stack Overflow for code search

#### Privacy Features
- No tracking, no logging, no data collection
- Full Tor integration with hidden service support
- Image proxying for privacy
- GeoIP country blocking/allowing
- Referrer stripping

#### User Interface
- Dark (Dracula) and light theme toggle
- Mobile-responsive design
- Infinite scroll
- Category tabs (Web, Images, Videos, News)
- Widget system (weather, clock, calculator, etc.)
- Instant answers (math, conversions, definitions)
- Custom bangs (!g, !ddg, etc.)

#### Backend
- SQLite database (pure Go, no CGO)
- Redis cache with in-memory fallback
- SSL/TLS with Let's Encrypt auto-renewal
- Email notifications via SMTP
- Task scheduler with periodic jobs
- Comprehensive logging (access, audit, security)

#### Deployment
- Docker multi-stage build (Alpine-based)
- Service management (systemd, launchd, Windows Service, BSD rc.d)
- Health checks and monitoring endpoints
- REST API with OpenAPI documentation

### Security
- CSRF protection
- Rate limiting
- Security headers (HSTS, CSP, X-Frame-Options)
- Fail2ban compatible security logs
- Input sanitization

---

**Note**: This changelog follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/) conventions.
