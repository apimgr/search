# Search - TODO List

**Project**: search
**Updated**: 2026-01-22
**Spec**: AI.md

## Status: AI.md UPDATED

AI.md has been copied from TEMPLATE.md and PARTS 0-5 have been read.

---

## Current Tasks

### In Progress
- None

### Pending
- Review project for AI.md compliance (PARTS 0-5)
- Ensure all NEVER/MUST rules are followed in codebase
- Verify path security implementation (SafePath)
- Verify Argon2id password hashing
- Verify config.ParseBool() usage
- Verify comment style (above code, not inline)

---

## Test Coverage Summary

| Package | Coverage | Status |
|---------|----------|--------|
| client/api | 100.0% | COMPLETE |
| common/httputil | 100.0% | COMPLETE |
| common/version | 100.0% | COMPLETE |
| mode | 100.0% | COMPLETE |
| model | 98.9% | Excellent |
| common/terminal | 98.6% | Excellent |
| swagger | 97.7% | Excellent |
| common/banner | 97.2% | Excellent |
| client/tui | 95.2% | Excellent |
| search | 94.9% | Excellent |
| graphql | 93.9% | Excellent |
| common/display | 92.9% | Excellent |
| signal | 92.3% | Excellent |
| client | 91.8% | Good |
| backup | 87.7% | Good |
| common/theme | 81.7% | Good |
| widget | 75.9% | Good |
| database | 74.0% | Good |
| cache | 72.0% | Good |
| tls | 71.0% | Good |
| update | 69.7% | Good |
| server | 16.7% | Needs improvement |

---

## Compliance Summary

| PARTS | Status | Notes |
|-------|--------|-------|
| 0-5 | REVIEWED | AI rules, critical rules, license, structure, paths, config |
| 6-10 | NEEDS REVIEW | Mode, CLI, errors, database |
| 11-15 | NEEDS REVIEW | Security headers, SSL/TLS |
| 16-20 | NEEDS REVIEW | Admin panel, email, scheduler |
| 21-25 | NEEDS REVIEW | Metrics, backup, service |
| 26-30 | NEEDS REVIEW | Build/CI, testing |
| 31-36 | NEEDS REVIEW | I18N, optional features |

---

## Notes

- AI.md is READ-ONLY (PARTS 0-36)
- Build passes, tests complete in reasonable time
- Organizations and Custom Domains NOT NEEDED for this project
