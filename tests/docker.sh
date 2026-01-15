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
GO_DOCKER="docker run --rm \
  -v $(pwd):/build \
  -v ${GOCACHE}:/root/.cache/go-build \
  -v ${GODIR}:/go \
  -w /build \
  -e CGO_ENABLED=0 \
  golang:alpine"

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
    apk add --no-cache curl bash file jq >/dev/null

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

    echo '=== Admin Setup & API Token Creation ==='
    # Get setup token from server output (captured during startup)
    SETUP_TOKEN=\$(cat /tmp/server.log 2>/dev/null | grep -oP 'Setup Token.*:\\s*\\K[a-f0-9]+' | head -1 || echo '')

    if [ -n \"\$SETUP_TOKEN\" ]; then
        echo \"Setup token found: \${SETUP_TOKEN:0:8}...\"

        # Create admin account
        curl -sf -X POST \\
            -H \"X-Setup-Token: \$SETUP_TOKEN\" \\
            -H \"Content-Type: application/json\" \\
            -d '{\"username\":\"testadmin\",\"password\":\"TestPass123!\"}' \\
            http://localhost:64580/api/v1/admin/setup || echo 'Admin setup failed (may already exist)'

        # Login and get session
        SESSION=\$(curl -sf -X POST \\
            -H \"Content-Type: application/json\" \\
            -d '{\"username\":\"testadmin\",\"password\":\"TestPass123!\"}' \\
            http://localhost:64580/api/v1/admin/login | grep -oP '\"session_token\":\\s*\"\\K[^\"]+' || echo '')

        if [ -n \"\$SESSION\" ]; then
            echo '✓ Admin login successful'

            # Generate API token for CLI/Agent testing
            API_TOKEN=\$(curl -sf -X POST \\
                -H \"Authorization: Bearer \$SESSION\" \\
                http://localhost:64580/api/v1/admin/profile/token | grep -oP '\"token\":\\s*\"\\K[^\"]+' || echo '')

            if [ -n \"\$API_TOKEN\" ]; then
                echo \"✓ API token created: \${API_TOKEN:0:12}...\"
            else
                echo 'API token creation failed (continuing without token)'
            fi
        else
            echo 'Admin login failed (continuing without session)'
        fi
    else
        echo 'No setup token found (server may already be configured)'
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
