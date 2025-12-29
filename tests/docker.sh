#!/usr/bin/env bash
# Per AI.md PART 9: Full integration testing in Docker Alpine container
set -euo pipefail

# Detect project info
PROJECTNAME="search"
PROJECTORG="apimgr"

# Create temp directory for build
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}.XXXXXX")
trap "rm -rf $BUILD_DIR" EXIT

echo "=== Building binary in Docker ==="
docker run --rm \
  -v "$(pwd):/build" \
  -w /build \
  -e CGO_ENABLED=0 \
  golang:alpine go build -o "$BUILD_DIR/$PROJECTNAME" ./src

echo "=== Testing in Docker (Alpine) ==="
docker run --rm \
  -v "$BUILD_DIR:/app" \
  alpine:latest sh -c "
    set -e
    apk add --no-cache curl file

    chmod +x /app/$PROJECTNAME

    echo '=== Version Check ==='
    /app/$PROJECTNAME --version

    echo '=== Help Check ==='
    /app/$PROJECTNAME --help

    echo '=== Binary Info ==='
    ls -lh /app/$PROJECTNAME
    file /app/$PROJECTNAME

    echo '=== Starting Server for API Tests ==='
    /app/$PROJECTNAME --port 64580 &
    SERVER_PID=\$!
    sleep 3

    echo '=== Health Endpoint Tests ==='
    # Test JSON response (default)
    curl -f http://localhost:64580/healthz || { echo 'FAILED: /healthz'; exit 1; }

    # Test .txt extension (plain text)
    curl -f http://localhost:64580/healthz.txt || { echo 'FAILED: /healthz.txt'; exit 1; }

    # Test Accept header: application/json
    curl -f -H 'Accept: application/json' http://localhost:64580/healthz || { echo 'FAILED: Accept JSON'; exit 1; }

    # Test Accept header: text/plain
    curl -f -H 'Accept: text/plain' http://localhost:64580/healthz || { echo 'FAILED: Accept text/plain'; exit 1; }

    echo '=== API Endpoint Tests ==='
    # Search API
    curl -f 'http://localhost:64580/api/v1/search?q=test' || { echo 'FAILED: /api/v1/search'; exit 1; }

    # Autocomplete API
    curl -f 'http://localhost:64580/api/v1/autocomplete?q=test' || { echo 'FAILED: /api/v1/autocomplete'; exit 1; }

    # Engines API
    curl -f 'http://localhost:64580/api/v1/engines' || { echo 'FAILED: /api/v1/engines'; exit 1; }

    # Config API
    curl -f 'http://localhost:64580/api/v1/config' || { echo 'FAILED: /api/v1/config'; exit 1; }

    echo '=== Frontend Tests ==='
    # Home page
    curl -f http://localhost:64580/ || { echo 'FAILED: Home page'; exit 1; }

    # Search page (HTML)
    curl -f -H 'Accept: text/html' 'http://localhost:64580/search?q=test' || { echo 'FAILED: Search page'; exit 1; }

    # Preferences page
    curl -f http://localhost:64580/preferences || { echo 'FAILED: Preferences'; exit 1; }

    # About page
    curl -f http://localhost:64580/server/about || { echo 'FAILED: About'; exit 1; }

    # robots.txt
    curl -f http://localhost:64580/robots.txt || { echo 'FAILED: robots.txt'; exit 1; }

    # OpenSearch
    curl -f http://localhost:64580/opensearch.xml || echo 'WARN: OpenSearch may be disabled'

    echo '=== Static Assets Test ==='
    # CSS
    curl -f http://localhost:64580/static/css/style.css || { echo 'FAILED: CSS'; exit 1; }

    # JS
    curl -f http://localhost:64580/static/js/app.js || { echo 'FAILED: JS'; exit 1; }

    echo '=== Stopping Server ==='
    kill \$SERVER_PID
    wait \$SERVER_PID 2>/dev/null || true

    echo '=== All tests passed ==='
"

echo "Docker tests completed successfully"
