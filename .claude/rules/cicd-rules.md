# CI/CD Rules (PART 27)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO

- Pin Actions to tags (must pin to full commit SHA)
- Create build-toolchain.yml for Go projects (no docker/Dockerfile.build needed)
- Add ci.yml or release.yml before all code is complete and tests pass
- Skip make test before any commit
- Write coverage output to the project tree in CI

## CRITICAL - ALWAYS DO

- Third-party Actions pinned to full commit SHA (never @v1, @main, @latest)
- Workflow creation order: security-only → ci.yml → release.yml
- Coverage output to $GITHUB_ENV COVDIR (separate step, mounted path)
- Run act --list -W {file} on workflow files before gitcommit
- Test gate: make test must pass before every commit

## Workflow Creation Order

1. Security workflows first (secret scan, SHA/digest policy, dependency audit)
2. ci.yml — only after all code complete, make test passes, lint clean
3. release.yml — only after ci.yml is proven working

## GitHub Actions SHA Pinning

```yaml
# WRONG
uses: actions/checkout@v4
uses: actions/setup-go@v5

# CORRECT
uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683  # v4.2.2
uses: actions/setup-go@f111f3307d8850f501ac008e886eec1fd1932a34  # v5.3.0
```

## CI Coverage Pattern

```yaml
- name: Run tests
  env:
    CGO_ENABLED: 0
  run: |
    mkdir -p /tmp/${{ github.repository_owner }}
    COVDIR=$(mktemp -d "/tmp/${{ github.repository_owner }}/search-XXXXXX")
    echo "COVDIR=$COVDIR" >> $GITHUB_ENV
    docker run --rm \
      -v "$PWD:/workspace" \
      -w /workspace \
      -e CGO_ENABLED=0 \
      casjaysdev/go:latest \
      sh -c "go test -coverprofile=$COVDIR/coverage.out ./... && go tool cover -func=$COVDIR/coverage.out"
```

## Required Workflows

| File | Purpose | When to Add |
|------|---------|-------------|
| .github/workflows/security.yml | Secret scan, SHA policy | First (safe anytime) |
| .github/workflows/ci.yml | Build + test on PR/push | After code complete |
| .github/workflows/release.yml | Build + publish release | After ci.yml proven |

## Test Gate

make test must pass before every commit — no exceptions.
If make test is absent: go test ./... must pass.
Never skip tests to "save time".

For complete details, see AI.md PART 27
