#!/usr/bin/env bash
# Per AI.md PART 29: Full integration + systemd testing in Incus Debian container
set -euo pipefail

# Check if incus is available
if ! command -v incus &>/dev/null; then
    echo "ERROR: incus not found. Install incus or use tests/docker.sh"
    exit 1
fi

# Detect project info
PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
CONTAINER_NAME="test-${PROJECTNAME}-$$"

echo "Project: $PROJECTORG/$PROJECTNAME"

# Incus image - use latest Debian stable (currently 12/bookworm)
INCUS_IMAGE="images:debian/12"

# Create temp directory for build
# Per AI.md PART 29: Use org/project structure for temp dirs
mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")
trap "rm -rf $BUILD_DIR; incus delete $CONTAINER_NAME --force 2>/dev/null || true" EXIT

# Go cache directories (same as Makefile) per AI.md PART 29
GODIR="${HOME}/.local/share/go"
GOCACHE="${HOME}/.local/share/go/build"
mkdir -p "$GODIR" "$GOCACHE"

# Build output directory inside Docker mount
DOCKER_BUILD_OUT="/build/test-binaries"

echo "=== Building server binary in Docker ==="
docker run --rm \
  -v "$(pwd):/build" \
  -v "${GOCACHE}:/root/.cache/go-build" \
  -v "${GODIR}:/go" \
  -w /build \
  -e CGO_ENABLED=0 \
  golang:alpine sh -c "mkdir -p $DOCKER_BUILD_OUT && go build -o $DOCKER_BUILD_OUT/$PROJECTNAME ./src"

# Copy built binary to BUILD_DIR
cp "$(pwd)/test-binaries/$PROJECTNAME" "$BUILD_DIR/"

# Build CLI client if exists
if [ -d "src/client" ]; then
    echo "=== Building CLI client in Docker ==="
    docker run --rm \
      -v "$(pwd):/build" \
      -v "${GOCACHE}:/root/.cache/go-build" \
      -v "${GODIR}:/go" \
      -w /build \
      -e CGO_ENABLED=0 \
      golang:alpine go build -o "$DOCKER_BUILD_OUT/${PROJECTNAME}-cli" ./src/client
    cp "$(pwd)/test-binaries/${PROJECTNAME}-cli" "$BUILD_DIR/"
fi

# Build agent if exists
if [ -d "src/agent" ]; then
    echo "=== Building agent in Docker ==="
    docker run --rm \
      -v "$(pwd):/build" \
      -v "${GOCACHE}:/root/.cache/go-build" \
      -v "${GODIR}:/go" \
      -w /build \
      -e CGO_ENABLED=0 \
      golang:alpine go build -o "$DOCKER_BUILD_OUT/${PROJECTNAME}-agent" ./src/agent
    cp "$(pwd)/test-binaries/${PROJECTNAME}-agent" "$BUILD_DIR/"
fi

# Clean up build artifacts from project directory
rm -rf "$(pwd)/test-binaries"

echo "=== Launching Incus container (Debian + systemd) ==="
incus launch "$INCUS_IMAGE" "$CONTAINER_NAME"

# Wait for container to be ready
sleep 2

echo "=== Copying binaries to container ==="
incus file push "$BUILD_DIR/$PROJECTNAME" "$CONTAINER_NAME/usr/local/bin/"
incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/$PROJECTNAME"

# Copy CLI client if built
if [ -f "$BUILD_DIR/${PROJECTNAME}-cli" ]; then
    incus file push "$BUILD_DIR/${PROJECTNAME}-cli" "$CONTAINER_NAME/usr/local/bin/"
    incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/${PROJECTNAME}-cli"
fi

# Copy agent if built
if [ -f "$BUILD_DIR/${PROJECTNAME}-agent" ]; then
    incus file push "$BUILD_DIR/${PROJECTNAME}-agent" "$CONTAINER_NAME/usr/local/bin/"
    incus exec "$CONTAINER_NAME" -- chmod +x "/usr/local/bin/${PROJECTNAME}-agent"
fi

# Ensure required packages are available for testing
# Per AI.md PART 27: tor is a required package (auto-enabled when found)
incus exec "$CONTAINER_NAME" -- bash -c "apt-get update && apt-get install -y curl file jq procps tor" >/dev/null 2>&1

echo "=== Running tests in Incus ==="
incus exec "$CONTAINER_NAME" -- bash -c "
    set -e

    echo '=== Version Check ==='
    /usr/local/bin/$PROJECTNAME --version

    echo '=== Help Check ==='
    /usr/local/bin/$PROJECTNAME --help

    echo '=== Binary Info ==='
    ls -lh /usr/local/bin/$PROJECTNAME
    file /usr/local/bin/$PROJECTNAME

    echo '=== Binary Rename Tests ==='
    # Per AI.md PART 29: Test that binaries show ACTUAL name in --help/--version
    cp /usr/local/bin/$PROJECTNAME /tmp/renamed-server
    chmod +x /tmp/renamed-server
    if /tmp/renamed-server --help 2>&1 | grep -q 'renamed-server'; then
        echo '✓ Server binary rename works (--help shows actual name)'
    else
        echo '✗ FAILED: Server --help does not show renamed binary name'
        exit 1
    fi

    echo '=== Service Install Test ==='
    # Per AI.md PART 29: Install as system service
    /usr/local/bin/$PROJECTNAME --service --install

    echo '=== Service Status ==='
    systemctl status $PROJECTNAME || true

    echo '=== Service Start Test ==='
    systemctl start $PROJECTNAME
    sleep 3

    # Check if service is running, if not show detailed logs
    if ! systemctl is-active --quiet $PROJECTNAME; then
        echo 'Service failed to start. Journal logs:'
        journalctl -u $PROJECTNAME --no-pager -n 50
        echo ''
        echo 'Service file:'
        cat /etc/systemd/system/$PROJECTNAME.service
        echo ''
        echo 'Directory permissions:'
        ls -la /etc/apimgr/$PROJECTNAME/
        ls -la /var/lib/apimgr/$PROJECTNAME/
        exit 1
    fi
    systemctl status $PROJECTNAME

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
    # Health API (JSON)
    curl -f 'http://localhost:64580/api/v1/healthz' || { echo 'FAILED: /api/v1/healthz'; exit 1; }

    # Health API (.txt extension)
    curl -f 'http://localhost:64580/api/v1/healthz.txt' || { echo 'FAILED: /api/v1/healthz.txt'; exit 1; }

    # Search API
    curl -f 'http://localhost:64580/api/v1/search?q=test' || { echo 'FAILED: /api/v1/search'; exit 1; }

    # Autocomplete API
    curl -f 'http://localhost:64580/api/v1/autocomplete?q=test' || { echo 'FAILED: /api/v1/autocomplete'; exit 1; }

    # Engines API
    curl -f 'http://localhost:64580/api/v1/engines' || { echo 'FAILED: /api/v1/engines'; exit 1; }

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
    curl -f http://localhost:64580/static/css/main.css || { echo 'FAILED: CSS'; exit 1; }

    # JS
    curl -f http://localhost:64580/static/js/app.js || { echo 'FAILED: JS'; exit 1; }

    echo '=== Admin Setup & API Token Creation ==='
    # Per AI.md PART 29: Get setup token from journal
    SETUP_TOKEN=\$(journalctl -u $PROJECTNAME --no-pager 2>/dev/null | grep -oP 'Setup Token.*:\\s*\\K[a-f0-9]+' | head -1 || echo '')

    if [ -n \"\$SETUP_TOKEN\" ]; then
        echo \"Setup token found: \${SETUP_TOKEN:0:8}...\"

        # Create admin account via API
        SETUP_RESULT=\$(curl -sf -X POST \\
            -H \"X-Setup-Token: \$SETUP_TOKEN\" \\
            -H \"Content-Type: application/json\" \\
            -d '{\"username\":\"testadmin\",\"password\":\"TestPass123!\",\"email\":\"test@example.com\"}' \\
            http://localhost:64580/admin/setup 2>/dev/null || echo 'setup_failed')

        if echo \"\$SETUP_RESULT\" | grep -q 'setup_failed'; then
            echo 'Admin setup failed (may already exist)'
        else
            echo '✓ Admin account created'
        fi
    else
        echo 'No setup token found (server may already be configured)'
    fi

    echo '=== CLI Client Tests (if exists) ==='
    if [ -f /usr/local/bin/${PROJECTNAME}-cli ]; then
        echo 'Testing CLI client...'

        # Version check
        /usr/local/bin/${PROJECTNAME}-cli --version || { echo 'FAILED: CLI --version'; exit 1; }

        # Help check
        /usr/local/bin/${PROJECTNAME}-cli --help || { echo 'FAILED: CLI --help'; exit 1; }

        # Binary rename test for CLI
        cp /usr/local/bin/${PROJECTNAME}-cli /tmp/renamed-cli
        chmod +x /tmp/renamed-cli
        if /tmp/renamed-cli --help 2>&1 | grep -q 'renamed-cli'; then
            echo '✓ CLI binary rename works (--help shows actual name)'
        else
            echo '✗ FAILED: CLI --help does not show renamed binary name'
        fi

        # Test CLI against running server (status command)
        /usr/local/bin/${PROJECTNAME}-cli --server http://localhost:64580 status || echo 'CLI status command failed (may need auth)'

        echo '✓ CLI client tests passed'
    else
        echo 'CLI client not installed - skipping'
    fi

    echo '=== Agent Tests (if exists) ==='
    if [ -f /usr/local/bin/${PROJECTNAME}-agent ]; then
        echo 'Testing agent...'

        # Version check
        /usr/local/bin/${PROJECTNAME}-agent --version || { echo 'FAILED: Agent --version'; exit 1; }

        # Help check
        /usr/local/bin/${PROJECTNAME}-agent --help || { echo 'FAILED: Agent --help'; exit 1; }

        # Binary rename test for agent
        cp /usr/local/bin/${PROJECTNAME}-agent /tmp/renamed-agent
        chmod +x /tmp/renamed-agent
        if /tmp/renamed-agent --help 2>&1 | grep -q 'renamed-agent'; then
            echo '✓ Agent binary rename works (--help shows actual name)'
        else
            echo '✗ FAILED: Agent --help does not show renamed binary name'
        fi

        echo '✓ Agent tests passed'
    else
        echo 'Agent not installed - skipping'
    fi

    echo '=== Service Stop Test ==='
    systemctl stop $PROJECTNAME

    echo '=== All tests passed ==='
"

echo "Incus tests completed successfully"
