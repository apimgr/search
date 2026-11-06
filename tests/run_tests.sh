#!/usr/bin/env bash
# @@License : WTFPL
# Per AI.md PART 28: Auto-detect runtime and run tests
set -eo pipefail

cd "$(dirname "$0")/.."

# Detect available container runtime
if command -v incus &>/dev/null; then
    echo "Detected: incus"
    echo "Running full integration tests with systemd..."
    exec ./tests/incus.sh
elif command -v docker &>/dev/null; then
    echo "Detected: docker"
    echo "Running integration tests..."
    exec ./tests/docker.sh
else
    echo "ERROR: No container runtime found."
    echo "Please install docker or incus to run integration tests."
    exit 1
fi
