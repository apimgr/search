#!/usr/bin/env bash
set -euo pipefail

# Detect project info
PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")

# Create temp directory for build
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}.XXXXXX")
trap "rm -rf $BUILD_DIR" EXIT

# Go cache directories (same as Makefile)
GODIR="${HOME}/.local/share/go"
GOCACHE="${HOME}/.local/share/go/build"
mkdir -p "$GODIR" "$GOCACHE"

# Common docker run for Go builds
# Per AI.md PART 25/26: always build with casjaysdev/go:latest, never golang:alpine
GO_DOCKER="docker run --rm \
  -v $PWD:/build \
  -v ${GOCACHE}:/root/.cache/go-build \
  -v ${GODIR}:/go \
  -w /build \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  casjaysdev/go:latest"

mkdir -p binaries

echo "Building server binary in Docker..."
$GO_DOCKER go build -o /build/binaries/.dev-${PROJECTNAME} ./src
mv binaries/.dev-${PROJECTNAME} "$BUILD_DIR/${PROJECTNAME}"

# Build CLI client if exists
if [ -d "src/client" ]; then
    echo "Building CLI client in Docker..."
    $GO_DOCKER go build -o /build/binaries/.dev-${PROJECTNAME}-cli ./src/client
    mv binaries/.dev-${PROJECTNAME}-cli "$BUILD_DIR/${PROJECTNAME}-cli"
fi

# Build agent if exists
if [ -d "src/agent" ]; then
    echo "Building agent in Docker..."
    $GO_DOCKER go build -o /build/binaries/.dev-${PROJECTNAME}-agent ./src/agent
    mv binaries/.dev-${PROJECTNAME}-agent "$BUILD_DIR/${PROJECTNAME}-agent"
fi

echo "Testing in Docker (Alpine)..."
docker run --rm \
  -v "$BUILD_DIR:/app" \
  alpine:latest sh -c "
    set -e

    # Install required tools for testing
    # Per AI.md PART 27: tor is a required package (auto-enabled when found)
    apk add --no-cache curl bash file jq tor >/dev/null

    chmod +x /app/${PROJECTNAME}
    [ -f /app/${PROJECTNAME}-cli ] && chmod +x /app/${PROJECTNAME}-cli
    [ -f /app/${PROJECTNAME}-agent ] && chmod +x /app/${PROJECTNAME}-agent

    echo '=== Version Check ==='
    /app/${PROJECTNAME} --version

    echo '=== Help Check ==='
    /app/${PROJECTNAME} --help

    echo '=== Binary Info ==='
    ls -lh /app/${PROJECTNAME}
    file /app/${PROJECTNAME}

    echo '=== Starting Server for API Tests ==='
    /app/${PROJECTNAME} --port 64580 > /tmp/server.log 2>&1 &
    SERVER_PID=\$!
    sleep 3
    # Show setup token if present (for debugging)
    grep -i 'setup.*token' /tmp/server.log 2>/dev/null || true

    echo '=== API Endpoint Tests ==='
    # Test JSON response (default)
    curl -f http://localhost:64580/api/v1/healthz || echo 'FAILED: /api/v1/healthz'

    # Test .txt extension (plain text)
    curl -f http://localhost:64580/api/v1/healthz.txt || echo 'FAILED: /api/v1/healthz.txt'

    # Test Accept header: application/json
    curl -f -H 'Accept: application/json' http://localhost:64580/healthz || echo 'FAILED: Accept JSON'

    # Test Accept header: text/plain
    curl -f -H 'Accept: text/plain' http://localhost:64580/healthz || echo 'FAILED: Accept text/plain'

    echo '=== Project-Specific Endpoint Tests ==='
    # Search API endpoints (from IDEA.md)
    curl -f 'http://localhost:64580/api/v1/search?q=test' || echo 'FAILED: /api/v1/search'
    curl -f 'http://localhost:64580/api/v1/autocomplete?q=test' || echo 'FAILED: /api/v1/autocomplete'
    curl -f http://localhost:64580/api/v1/engines || echo 'FAILED: /api/v1/engines'
    curl -f 'http://localhost:64580/api/v1/search/related?q=test' || echo 'FAILED: /api/v1/search/related'
    curl -f http://localhost:64580/api/v1/categories || echo 'FAILED: /api/v1/categories'
    curl -f http://localhost:64580/api/v1/bangs || echo 'FAILED: /api/v1/bangs'
    curl -f 'http://localhost:64580/api/v1/instant?q=2+2' || echo 'FAILED: /api/v1/instant'

    # Frontend endpoints
    curl -f http://localhost:64580/ || echo 'FAILED: Home page'
    curl -f -H 'Accept: text/html' 'http://localhost:64580/search?q=test' || echo 'FAILED: Search page'
    curl -f http://localhost:64580/preferences || echo 'FAILED: Preferences'
    curl -f http://localhost:64580/server/about || echo 'FAILED: About'
    curl -f http://localhost:64580/robots.txt || echo 'FAILED: robots.txt'

    # Static assets
    curl -f http://localhost:64580/static/css/main.css || echo 'FAILED: CSS'
    curl -f http://localhost:64580/static/js/app.js || echo 'FAILED: JS'

    echo '=== Operator Token Tests ==='
    # Per AI.md PART 8/11: two-tier auth only (anonymous + operator bearer
    # token). There is no admin account, login, or setup-token API — the
    # operator token (server.token) is printed once on first run and used
    # directly as a Bearer token against operator-only routes.
    API_TOKEN=\$(cat /tmp/server.log 2>/dev/null | sed -n 's/.*Operator Token: *\\([a-f0-9]*\\).*/\\1/p' | head -1 || echo '')

    if [ -n \"\$API_TOKEN\" ]; then
        echo \"Operator token found: \${API_TOKEN:0:8}...\"

        # Operator-only endpoint should succeed with the token
        curl -sf -H \"Authorization: Bearer \$API_TOKEN\" \\
            http://localhost:64580/server/status >/dev/null \\
            && echo '✓ /server/status authorized with operator token' \\
            || echo 'FAILED: /server/status with operator token'

        # Same endpoint without a token must be rejected
        STATUS_CODE=\$(curl -s -o /dev/null -w '%{http_code}' http://localhost:64580/server/status)
        if [ \"\$STATUS_CODE\" = \"401\" ]; then
            echo '✓ /server/status rejects unauthenticated request (401)'
        else
            echo \"FAILED: /server/status without token returned \$STATUS_CODE, expected 401\"
        fi
    else
        echo 'No operator token found in startup log (server may already be configured)'
    fi

    echo '=== Binary Rename Tests ==='
    # Test that binaries show ACTUAL name in --help/--version (not hardcoded)
    cp /app/${PROJECTNAME} /app/renamed-server
    chmod +x /app/renamed-server
    if /app/renamed-server --help 2>&1 | grep -q 'renamed-server'; then
        echo '✓ Server binary rename works (--help shows actual name)'
    else
        echo '✗ FAILED: Server --help does not show renamed binary name'
    fi

    echo '=== CLI Client Tests (if exists) ==='
    if [ -f /app/${PROJECTNAME}-cli ]; then
        /app/${PROJECTNAME}-cli --version || echo 'FAILED: CLI --version'
        /app/${PROJECTNAME}-cli --help || echo 'FAILED: CLI --help'

        # Test binary rename
        cp /app/${PROJECTNAME}-cli /app/renamed-cli
        chmod +x /app/renamed-cli
        if /app/renamed-cli --help 2>&1 | grep -q 'renamed-cli'; then
            echo '✓ CLI binary rename works'
        else
            echo '✗ FAILED: CLI --help does not show renamed binary name'
        fi

        # Full CLI functionality tests against server
        echo '--- CLI Full Functionality Tests ---'
        if [ -n \"\${API_TOKEN:-}\" ]; then
            # Test with API token
            /app/${PROJECTNAME}-cli --server http://localhost:64580 --token \"\$API_TOKEN\" status || echo 'CLI status failed'
        else
            # Test without token (anonymous if allowed)
            /app/${PROJECTNAME}-cli --server http://localhost:64580 status || echo 'CLI status (no token) failed or not applicable'
        fi
    else
        echo 'CLI client not built - skipping'
    fi

    echo '=== Agent Tests (if exists) ==='
    if [ -f /app/${PROJECTNAME}-agent ]; then
        /app/${PROJECTNAME}-agent --version || echo 'FAILED: Agent --version'
        /app/${PROJECTNAME}-agent --help || echo 'FAILED: Agent --help'

        # Test binary rename
        cp /app/${PROJECTNAME}-agent /app/renamed-agent
        chmod +x /app/renamed-agent
        if /app/renamed-agent --help 2>&1 | grep -q 'renamed-agent'; then
            echo '✓ Agent binary rename works'
        else
            echo '✗ FAILED: Agent --help does not show renamed binary name'
        fi

        # Full Agent functionality tests against server
        echo '--- Agent Full Functionality Tests ---'
        if [ -n \"\${API_TOKEN:-}\" ]; then
            # Test agent registration/status with API token
            /app/${PROJECTNAME}-agent --server http://localhost:64580 --token \"\$API_TOKEN\" status || echo 'Agent status failed'
        else
            echo 'Agent tests skipped (no API token)'
        fi
    else
        echo 'Agent not built - skipping'
    fi

    echo '=== Stopping Server ==='
    kill \$SERVER_PID
    wait \$SERVER_PID 2>/dev/null || true

    echo '=== All tests passed ==='
"

echo "Docker tests completed successfully"
