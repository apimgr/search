#!/usr/bin/env bash
# @@License : WTFPL
# Per AI.md PART 28: the basic integration suite is Docker-only.
set -eo pipefail

cd "$(dirname "$0")/.."

# The systemd integration suite is available separately as tests/incus.sh.
if ! command -v docker &>/dev/null; then
    echo "ERROR: Docker is required for the integration test suite." >&2
    exit 1
fi

echo "Running Docker integration tests..."
exec ./tests/docker.sh
