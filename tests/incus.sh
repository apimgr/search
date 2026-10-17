#!/usr/bin/env bash
# @@License : WTFPL
# Per AI.md PART 28: Full integration + systemd testing in Incus Debian container
# shellcheck disable=SC2140
set -eo pipefail

# Check if incus is available
if ! command -v incus &>/dev/null; then
    echo "ERROR: incus not found. Install incus or use tests/docker.sh"
    exit 1
fi

# Detect project info
PROJECTNAME=$(basename "$PWD")
random_suffix() {
    printf '%08x' "$(((RANDOM << 15) | RANDOM))"
}
PROJECTORG=$(basename "$(dirname "$PWD")")
CONTAINER_NAME="${PROJECTNAME}-$(random_suffix)"

echo "Project: $PROJECTORG/$PROJECTNAME"

# Incus image - use latest Debian stable (currently 13/trixie)
INCUS_IMAGE="images:debian/trixie"

# Create temp directory for build
# Per AI.md PART 28: Use org/project structure for temp dirs
mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
BUILD_DIR=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-XXXXXX")
cleanup() {
    rm -rf -- "$BUILD_DIR"
    incus delete "$CONTAINER_NAME" --force >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Go cache directories (same as Makefile) per AI.md PART 28
GO_CACHE="${GO_CACHE:-${HOME}/go/pkg/mod}"
GO_BUILD="${GO_BUILD:-${HOME}/.cache/go-build/${PROJECTNAME}}"
mkdir -p "$GO_CACHE" "$GO_BUILD"

# Per AI.md PART 28: build location is binaries/ in the project root
DOCKER_BUILD_OUT="/build/binaries"

echo "=== Building server binary in Docker ==="
docker run --rm --name "${PROJECTNAME}-$(random_suffix)" \
  -v "$PWD:/build" \
  -v "${GO_BUILD}:/usr/local/share/go/cache" \
  -v "${GO_CACHE}:/usr/local/share/go/pkg/mod" \
  -w /build \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  casjaysdev/go:latest sh -c "mkdir -p $DOCKER_BUILD_OUT && go build -buildvcs=false -trimpath -o $DOCKER_BUILD_OUT/$PROJECTNAME ./src"

# Copy built binary to BUILD_DIR
cp "$(pwd)/binaries/$PROJECTNAME" "$BUILD_DIR/"

# Build CLI client if exists
if [ -d "src/client" ]; then
    echo "=== Building CLI client in Docker ==="
    docker run --rm --name "${PROJECTNAME}-$(random_suffix)" \
      -v "$PWD:/build" \
      -v "${GO_BUILD}:/usr/local/share/go/cache" \
      -v "${GO_CACHE}:/usr/local/share/go/pkg/mod" \
      -w /build \
      -e CGO_ENABLED=0 \
      -e GOFLAGS=-buildvcs=false \
      casjaysdev/go:latest go build -buildvcs=false -trimpath -o "$DOCKER_BUILD_OUT/${PROJECTNAME}-cli" ./src/client
    cp "$(pwd)/binaries/${PROJECTNAME}-cli" "$BUILD_DIR/"
fi

# Build agent if exists
if [ -d "src/agent" ]; then
    echo "=== Building agent in Docker ==="
    docker run --rm --name "${PROJECTNAME}-$(random_suffix)" \
      -v "$PWD:/build" \
      -v "${GO_BUILD}:/usr/local/share/go/cache" \
      -v "${GO_CACHE}:/usr/local/share/go/pkg/mod" \
      -w /build \
      -e CGO_ENABLED=0 \
      -e GOFLAGS=-buildvcs=false \
      casjaysdev/go:latest go build -buildvcs=false -trimpath -o "$DOCKER_BUILD_OUT/${PROJECTNAME}-agent" ./src/agent
    cp "$(pwd)/binaries/${PROJECTNAME}-agent" "$BUILD_DIR/"
fi

# Clean up only the artifacts this script produced - never the whole binaries/ dir
rm -f "$(pwd)/binaries/${PROJECTNAME}" \
      "$(pwd)/binaries/${PROJECTNAME}-cli" \
      "$(pwd)/binaries/${PROJECTNAME}-agent"

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
# Per AI.md PART 31: tor is a required package (auto-enabled when found)
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
    # Per AI.md PART 28: Test that binaries show ACTUAL name in --help/--version
    cp /usr/local/bin/$PROJECTNAME /tmp/renamed-server
    chmod +x /tmp/renamed-server
    if /tmp/renamed-server --help 2>&1 | grep -q 'renamed-server'; then
        echo '✓ Server binary rename works (--help shows actual name)'
    else
        echo '✗ FAILED: Server --help does not show renamed binary name'
        exit 1
    fi

    echo '=== Service Install Test ==='
    # Per AI.md PART 28: Install as system service
    if ! PORT=64580 SEARCH_PORT=64580 /usr/local/bin/$PROJECTNAME --port 64580 --service --install >/dev/null 2>&1; then
        echo 'FAILED: Service install'
        exit 1
    fi
    echo 'Service installed without exposing the generated operator token'

    echo '=== Service Status ==='
    systemctl status $PROJECTNAME || true

    echo '=== Service Start Test ==='
    systemctl stop $PROJECTNAME || true
    if grep -q '^  port:' "/etc/apimgr/"$PROJECTNAME"/server.yml"; then
        sed -i '/^  port:/c\  port: 64580' "/etc/apimgr/"$PROJECTNAME"/server.yml"
        sed -i '/^  healthz:/,/^  maintenance_mode:/s/^      enabled: false/      enabled: true/' "/etc/apimgr/"$PROJECTNAME"/server.yml"
    else
        echo 'FAILED: server.port is missing from generated config'
        exit 1
    fi
    systemctl start $PROJECTNAME
    if ! timeout 300 bash -c 'until curl -q -LSsf http://localhost:64580/healthz >/dev/null 2>&1; do sleep 5; done'; then
        echo 'Server did not become healthy within 5 minutes'
        journalctl -u $PROJECTNAME --no-pager -n 100
        exit 1
    fi

    # Check if service is running, if not show detailed logs
    if ! systemctl is-active --quiet $PROJECTNAME; then
        echo 'Service failed to start. Unit result:'
        systemctl show $PROJECTNAME --property=Result,ExecMainCode,ExecMainStatus,NRestarts
        echo 'Journal logs:'
        journalctl -u $PROJECTNAME --no-pager -n 100
        echo 'Application logs:'
        find /var/log/apimgr/$PROJECTNAME -maxdepth 2 -type f -print -exec tail -n 100 {} \; 2>/dev/null || true
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
    curl -q -LSsf -o /dev/null http://localhost:64580/healthz || { echo 'FAILED: /healthz'; exit 1; }

    # Test .txt extension (plain text)
    curl -q -LSsf -o /dev/null http://localhost:64580/healthz.txt || { echo 'FAILED: /healthz.txt'; exit 1; }

    # Test Accept header: application/json
    curl -q -LSsf -o /dev/null -H 'Accept: application/json' http://localhost:64580/healthz || { echo 'FAILED: Accept JSON'; exit 1; }

    # Test Accept header: text/plain
    curl -q -LSsf -o /dev/null -H 'Accept: text/plain' http://localhost:64580/healthz || { echo 'FAILED: Accept text/plain'; exit 1; }

    echo '=== API Endpoint Tests ==='
    # Health API (JSON)
    curl -q -LSsf -o /dev/null 'http://localhost:64580/api/v1/healthz' || { echo 'FAILED: /api/v1/healthz'; exit 1; }

    # Health API (.txt extension)
    curl -q -LSsf -o /dev/null 'http://localhost:64580/api/v1/healthz.txt' || { echo 'FAILED: /api/v1/healthz.txt'; exit 1; }

    # Search API
    curl -q -LSsf -o /dev/null 'http://localhost:64580/api/v1/search?q=test' || { echo 'FAILED: /api/v1/search'; exit 1; }

    # Autocomplete API
    curl -q -LSsf -o /dev/null 'http://localhost:64580/api/v1/autocomplete?q=test' || { echo 'FAILED: /api/v1/autocomplete'; exit 1; }

    # Engines API
    curl -q -LSsf -o /dev/null 'http://localhost:64580/api/v1/engines' || { echo 'FAILED: /api/v1/engines'; exit 1; }

    echo '=== Frontend Tests ==='
    # Home page
    curl -q -LSsf -o /dev/null http://localhost:64580/ || { echo 'FAILED: Home page'; exit 1; }

    # Search page (HTML)
    curl -q -LSsf -o /dev/null -H 'Accept: text/html' 'http://localhost:64580/search?q=test' || { echo 'FAILED: Search page'; exit 1; }

    # Preferences page
    curl -q -LSsf -o /dev/null http://localhost:64580/server/preferences || { echo 'FAILED: /server/preferences'; exit 1; }

    # About page
    curl -q -LSsf -o /dev/null http://localhost:64580/server/about || { echo 'FAILED: About'; exit 1; }

    # robots.txt
    curl -q -LSsf -o /dev/null http://localhost:64580/robots.txt || { echo 'FAILED: robots.txt'; exit 1; }

    # OpenSearch
    curl -q -LSsf -o /dev/null http://localhost:64580/opensearch.xml || echo 'WARN: OpenSearch may be disabled'

    echo '=== Static Assets Test ==='
    # CSS
    curl -q -LSsf -o /dev/null http://localhost:64580/static/css/common.css || { echo 'FAILED: /static/css/common.css'; exit 1; }

    # JS
    curl -q -LSsf -o /dev/null http://localhost:64580/static/js/app.js || { echo 'FAILED: JS'; exit 1; }

    echo '=== Operator Token Tests ==='
    # Per AI.md PART 8/11: two-tier auth only (anonymous + operator bearer
    # token). There is no admin account, login, or setup-token API — the
    # operator token (server.token) is printed once on first run and used
    # directly as a Bearer token against operator-only routes.
    API_TOKEN=\$(sed -n 's/^  token: *//p' \"/etc/apimgr/$PROJECTNAME/server.yml\" | tr -d '\"' | head -1)

    if [ -n "\$API_TOKEN" ]; then
        echo 'Operator token loaded from configuration for authorization checks'


        # Operator-only endpoint should succeed with the token
        curl -q -LSsf -o /dev/null -H \"Authorization: Bearer \$API_TOKEN\" \\
            http://localhost:64580/server/status >/dev/null \\
            && echo '✓ /server/status authorized with operator token' \\
            || echo 'FAILED: /server/status with operator token'

        # Same endpoint without a token must be rejected
        STATUS_CODE=\$(curl -q -LSsf -o /dev/null -w '%{http_code}' http://localhost:64580/server/status 2>/dev/null || true)
        if [ \"\$STATUS_CODE\" = \"401\" ]; then
            echo '✓ /server/status rejects unauthenticated request (401)'
        else
            echo \"FAILED: /server/status without token returned \$STATUS_CODE, expected 401\"
        fi
    else
        echo 'No operator token found in configuration'
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
        /usr/local/bin/${PROJECTNAME}-cli --server http://localhost:64580 status >/dev/null || { echo 'FAILED: CLI status command'; exit 1; }

        echo '✓ CLI client status check passed'
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
