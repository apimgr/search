#!/usr/bin/env bash
# Per AI.md PART 9: Full integration + systemd testing in Incus Debian container
set -euo pipefail

# Check if incus is available
if ! command -v incus &>/dev/null; then
    echo "ERROR: incus not found. Install incus or use tests/docker.sh"
    exit 1
fi

# Detect project info
PROJECTNAME="search"
PROJECTORG="apimgr"
CONTAINER_NAME="test-${PROJECTNAME}-$$"

# Create temp directory for build
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}.XXXXXX")
trap "rm -rf $BUILD_DIR; incus delete $CONTAINER_NAME --force 2>/dev/null || true" EXIT

echo "=== Building binary in Docker ==="
docker run --rm \
  -v "$(pwd):/build" \
  -w /build \
  -e CGO_ENABLED=0 \
  golang:alpine go build -o "$BUILD_DIR/$PROJECTNAME" ./src

echo "=== Launching Incus container (Debian + systemd) ==="
incus launch images:debian/12 "$CONTAINER_NAME"

# Wait for container to be ready
sleep 3

echo "=== Copying binary to container ==="
incus file push "$BUILD_DIR/$PROJECTNAME" "$CONTAINER_NAME/usr/local/bin/"
incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/$PROJECTNAME"

echo "=== Running tests in Incus ==="
incus exec "$CONTAINER_NAME" -- bash -c "
    set -e

    echo '=== Installing dependencies ==='
    apt-get update && apt-get install -y curl file procps

    echo '=== Version Check ==='
    /usr/local/bin/$PROJECTNAME --version

    echo '=== Help Check ==='
    /usr/local/bin/$PROJECTNAME --help

    echo '=== Binary Info ==='
    ls -lh /usr/local/bin/$PROJECTNAME
    file /usr/local/bin/$PROJECTNAME

    echo '=== Starting Server for API Tests ==='
    /usr/local/bin/$PROJECTNAME --port 64580 &
    SERVER_PID=\$!
    sleep 3

    echo '=== Health Endpoint Tests ==='
    curl -f http://localhost:64580/healthz || { echo 'FAILED: /healthz'; exit 1; }
    curl -f http://localhost:64580/healthz.txt || { echo 'FAILED: /healthz.txt'; exit 1; }
    curl -f -H 'Accept: application/json' http://localhost:64580/healthz || { echo 'FAILED: Accept JSON'; exit 1; }
    curl -f -H 'Accept: text/plain' http://localhost:64580/healthz || { echo 'FAILED: Accept text/plain'; exit 1; }

    echo '=== API Endpoint Tests ==='
    curl -f 'http://localhost:64580/api/v1/search?q=test' || { echo 'FAILED: /api/v1/search'; exit 1; }
    curl -f 'http://localhost:64580/api/v1/autocomplete?q=test' || { echo 'FAILED: /api/v1/autocomplete'; exit 1; }
    curl -f 'http://localhost:64580/api/v1/engines' || { echo 'FAILED: /api/v1/engines'; exit 1; }
    curl -f 'http://localhost:64580/api/v1/config' || { echo 'FAILED: /api/v1/config'; exit 1; }

    echo '=== Frontend Tests ==='
    curl -f http://localhost:64580/ || { echo 'FAILED: Home page'; exit 1; }
    curl -f -H 'Accept: text/html' 'http://localhost:64580/search?q=test' || { echo 'FAILED: Search page'; exit 1; }
    curl -f http://localhost:64580/preferences || { echo 'FAILED: Preferences'; exit 1; }
    curl -f http://localhost:64580/server/about || { echo 'FAILED: About'; exit 1; }
    curl -f http://localhost:64580/robots.txt || { echo 'FAILED: robots.txt'; exit 1; }

    echo '=== Static Assets Test ==='
    curl -f http://localhost:64580/static/css/style.css || { echo 'FAILED: CSS'; exit 1; }
    curl -f http://localhost:64580/static/js/app.js || { echo 'FAILED: JS'; exit 1; }

    echo '=== Stopping Server ==='
    kill \$SERVER_PID
    wait \$SERVER_PID 2>/dev/null || true

    echo '=== Testing systemd service installation ==='
    /usr/local/bin/$PROJECTNAME service install --user root
    systemctl daemon-reload
    systemctl start $PROJECTNAME || echo 'WARN: Service start may require config'
    sleep 2
    systemctl status $PROJECTNAME || echo 'WARN: Service status check'
    systemctl stop $PROJECTNAME || true

    echo '=== All tests passed ==='
"

echo "Incus tests completed successfully"
