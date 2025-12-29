#!/usr/bin/env bash
# Per AI.md PART 9: Auto-detect runtime and run tests
set -euo pipefail

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
    echo ""
    echo "To run unit tests only:"
    echo "  go test -v ./src/..."
    exit 1
fi
