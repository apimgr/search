# Docker Rules (PART 26)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

Read: `AI.md` PART 26 (Docker).

## CRITICAL - NEVER DO
- Put a Dockerfile in the repo root — `docker/Dockerfile` is the only location
- Ship a single-stage Dockerfile — builds are multi-stage
- Use a non-Alpine runtime base
- Omit `tini` as the init process
- Omit OCI labels or the healthcheck
- Bake config or data into the image — they are volume mounts

## CRITICAL - ALWAYS DO
- Keep `docker/Dockerfile`, `docker/Dockerfile.dev`, `docker/docker-compose.yml`, `docker/docker-compose.dev.yml`, and `docker/docker-compose.test.yml`
- Use a multi-stage build on an Alpine base with `tini` as PID 1
- Run as root in the container — the app manages users and permissions at runtime
- Apply the standard OCI labels and the PART 26 binary naming conventions exactly
- Declare a `HEALTHCHECK` in the Dockerfile
- Mount volumes for config and data; listen on internal port 80
- Use the container path layout: `/config/{project_name}/`, `/data/{project_name}/`, `/data/log/{project_name}/`, `/data/db/sqlite/`, `/data/backups/{project_name}/`

## KEY DECISIONS (pre-answered)
| Question | Answer | Reference |
|----------|--------|-----------|
| Where is the Dockerfile? | `docker/Dockerfile` (NEVER root) | PART 26 |
| Base image? | Alpine | PART 26 |
| Init process? | `tini` | PART 26 |
| Container user? | root — the app manages users at runtime | PART 26 |
| Internal port? | 80 | PART 4, 26 |
| Container DB dir? | `/data/db/sqlite/` | PART 4 |

## TERMINOLOGY
| Term | Meaning |
|------|---------|
| multi-stage | Separate build and runtime stages in one Dockerfile |
| toolchain image | Image used to compile; distinct from the runtime image |
| OCI labels | `org.opencontainers.image.*` metadata |

## QUICK REFERENCE
| File | Purpose |
|------|---------|
| `docker/Dockerfile` | Production image |
| `docker/Dockerfile.dev` | Development image |
| `docker/docker-compose.yml` | Production compose |
| `docker/docker-compose.dev.yml` | Development compose |
| `docker/docker-compose.test.yml` | Test compose |

---

For complete details, see AI.md PART 26
