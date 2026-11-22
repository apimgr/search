# API implementation notes

## Health data freshness

- `data_nvd` uses a 48-hour maximum age. AI.md PART 13 requires a freshness limit for each data set but does not set one for NVD; PART 18 schedules `cve_update` daily. The 48-hour limit allows one missed daily update before stale NVD data reports `degraded`.
