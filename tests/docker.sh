#!/usr/bin/env bash
# @@License : WTFPL
# Per AI.md PART 28: Integration testing in a Docker alpine container
set -eo pipefail

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
TMP_BASE=$TMPDIR
if [ -z "$TMP_BASE" ]; then
    TMP_BASE=/tmp
fi
TMP_ROOT="$TMP_BASE/$PROJECTORG"

random_suffix() {
    printf '%08x' "$(((RANDOM << 15) | RANDOM))"
}

# Build through the Makefile's development target, which writes to a scoped
# temporary directory instead of the repository.
echo "Building via 'make dev' (server + CLI, if present)..."
DEV_OUTPUT=$(make dev 2>&1)
echo "$DEV_OUTPUT"
BUILT_LINE=$(echo "$DEV_OUTPUT" | grep -- '^Built: ' | tail -1)
if [ -z "$BUILT_LINE" ]; then
    echo "FAILED: could not find 'Built: <path>' line in 'make dev' output"
    exit 1
fi
BUILD_PATH=$(printf '%s\n' "$BUILT_LINE" | cut -c8-)
BUILD_DIR=$(dirname "$BUILD_PATH")

cleanup() {
    case "$BUILD_DIR" in
        "$TMP_ROOT/$PROJECTNAME-"*)
            rm -rf -- "$BUILD_DIR"
            ;;
        *)
            printf 'Refusing to remove unexpected build directory: %s\n' "$BUILD_DIR" >&2
            return 1
            ;;
    esac
}
trap cleanup EXIT

echo "Testing in Docker (Alpine)..."
docker run --rm -i --name "$PROJECTNAME-$(random_suffix)" \
  -e PROJECTNAME="$PROJECTNAME" \
  -v "$BUILD_DIR:/app" \
  alpine:latest sh -s <<'ALPINE'
set -eu

apk add --no-cache curl bash file jq >/dev/null

chmod +x "/app/$PROJECTNAME"
if [ -f "/app/$PROJECTNAME-cli" ]; then
    chmod +x "/app/$PROJECTNAME-cli"
fi
if [ -f "/app/$PROJECTNAME-agent" ]; then
    chmod +x "/app/$PROJECTNAME-agent"
fi

echo '=== Version Check ==='
"/app/$PROJECTNAME" --version

echo '=== Help Check ==='
"/app/$PROJECTNAME" --help

echo '=== Binary Info ==='
ls -lh "/app/$PROJECTNAME"
file "/app/$PROJECTNAME"

mkdir -p /tmp/search-config
echo '=== Initializing Test Configuration ==='
"/app/$PROJECTNAME" --config /tmp/search-config --port 64580 >/tmp/search-bootstrap.log 2>&1 &
BOOTSTRAP_PID=$!
attempt=0
while ! grep -q 'Server listening' /tmp/search-bootstrap.log; do
    if ! kill -0 "$BOOTSTRAP_PID" 2>/dev/null; then
        sed -E 's/(Operator Token: ).*/\1[REDACTED]/' /tmp/search-bootstrap.log | tail -40
        echo 'FAILED: server exited before initial startup'
        exit 1
    fi
    if [ "$attempt" -ge 30 ]; then
        sed -E 's/(Operator Token: ).*/\1[REDACTED]/' /tmp/search-bootstrap.log | tail -40
        echo 'FAILED: initial startup exceeded 30 seconds'
        exit 1
    fi
    sleep 1
    attempt=$((attempt + 1))
done

API_TOKEN=$(awk '
    /^server:$/ { in_server = 1; next }
    in_server && /^  token:/ { sub(/^  token:[[:space:]]*/, ""); print; exit }
    in_server && /^[^ ]/ { exit }
' /tmp/search-config/server.yml)
if [ -z "$API_TOKEN" ]; then
    echo 'FAILED: generated operator token was not found in server.yml'
    exit 1
fi
kill "$BOOTSTRAP_PID"
wait "$BOOTSTRAP_PID" 2>/dev/null || true

awk '
    $0 == "    allowlist: []" {
        print "    allowlist:"
        print "      - cidr: 127.0.0.1/32"
        print "        description: Docker integration test loopback"
        print "      - cidr: ::1/128"
        print "        description: Docker integration test loopback"
        inserted = 1
        next
    }
    { print }
    END { if (!inserted) exit 1 }
' /tmp/search-config/server.yml > /tmp/server.yml
mv /tmp/server.yml /tmp/search-config/server.yml

echo '=== Starting Server for API Tests ==='
"/app/$PROJECTNAME" --config /tmp/search-config --port 64580 >/tmp/server.log 2>&1 &
SERVER_PID=$!
attempt=0
ready=false
while [ "$attempt" -lt 30 ]; do
    status=$(curl -q -sS --max-time 1 -o /dev/null -w '%{http_code}' http://127.0.0.1:64580/api/v1/server/healthz 2>/dev/null || true)
    case "$status" in
        2??)
            ready=true
            break
            ;;
    esac
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        sed -E 's/(Operator Token: ).*/\1[REDACTED]/' /tmp/server.log | tail -40
        echo 'FAILED: server exited before becoming ready'
        exit 1
    fi
    sleep 1
    attempt=$((attempt + 1))
done
if [ "$ready" != true ]; then
    sed -E 's/(Operator Token: ).*/\1[REDACTED]/' /tmp/server.log | tail -40
    echo 'FAILED: server did not become ready within 30 seconds'
    exit 1
fi

check_http() {
    label=$1
    url=$2
    shift 2
    status=$(curl -q -LSs --max-time 20 "$@" -o /dev/null -w '%{http_code}' "$url") || status=000
    case "$status" in
        2??) printf '✓ %s (%s)\n' "$label" "$status" ;;
        *) printf 'FAILED: %s returned HTTP %s\n' "$label" "$status" >&2; return 1 ;;
    esac
}

check_status() {
    label=$1
    expected=$2
    url=$3
    status=$(curl -q -sS --max-time 10 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
    if [ "$status" != "$expected" ]; then
        printf 'FAILED: %s returned HTTP %s, expected %s\n' "$label" "$status" "$expected" >&2
        return 1
    fi
    printf '✓ %s (%s)\n' "$label" "$status"
}

BASE_URL=http://127.0.0.1:64580
echo '=== API Endpoint Tests ==='
check_http 'API health JSON' "$BASE_URL/api/v1/server/healthz"
check_http 'API health plain text' "$BASE_URL/api/v1/server/healthz.txt"
check_http 'Frontend health JSON negotiation' "$BASE_URL/server/healthz" -H 'Accept: application/json'
check_http 'Frontend health text negotiation' "$BASE_URL/server/healthz" -H 'Accept: text/plain'

echo '=== Project-Specific Endpoint Tests ==='
check_http 'Search API' "$BASE_URL/api/v1/search?q=test"
check_http 'Autocomplete API' "$BASE_URL/api/v1/autocomplete?q=test"
check_http 'Engine catalog API' "$BASE_URL/api/v1/engines"
check_http 'Related search API' "$BASE_URL/api/v1/search/related?q=test"
check_http 'Category catalog API' "$BASE_URL/api/v1/categories"
check_http 'Bang catalog API' "$BASE_URL/api/v1/bangs"
check_http 'Instant answer API' "$BASE_URL/api/v1/instant?q=2+2"

echo '=== Frontend Endpoint Tests ==='
check_http 'Home page' "$BASE_URL/"
check_http 'Search page' "$BASE_URL/search?q=test"
check_http 'Preferences page' "$BASE_URL/server/preferences"
check_http 'About page' "$BASE_URL/server/about"
check_http 'robots.txt' "$BASE_URL/robots.txt"
check_http 'CSS asset' "$BASE_URL/static/css/public.css"
check_http 'JavaScript asset' "$BASE_URL/static/js/app.js"

echo '=== Operator Token Tests ==='
check_http 'Operator status with token' "$BASE_URL/server/status" -H "Authorization: Bearer $API_TOKEN"
check_status 'Operator status without token' 401 "$BASE_URL/server/status"

echo '=== Binary Rename Tests ==='
cp "/app/$PROJECTNAME" /app/renamed-server
chmod +x /app/renamed-server
if ! /app/renamed-server --help 2>&1 | grep -q -- 'renamed-server'; then
    echo 'FAILED: server help did not use the renamed binary name'
    exit 1
fi
echo '✓ Server binary rename'

echo '=== CLI Client Tests (if present) ==='
if [ -f "/app/$PROJECTNAME-cli" ]; then
    "/app/$PROJECTNAME-cli" --version
    "/app/$PROJECTNAME-cli" --help >/dev/null
    cp "/app/$PROJECTNAME-cli" /app/renamed-cli
    chmod +x /app/renamed-cli
    if ! /app/renamed-cli --help 2>&1 | grep -q -- 'renamed-cli'; then
        echo 'FAILED: CLI help did not use the renamed binary name'
        exit 1
    fi
    printf '%s' "$API_TOKEN" >/tmp/search-cli-token
    chmod 600 /tmp/search-cli-token
    "/app/$PROJECTNAME-cli" --server "$BASE_URL" --token-file /tmp/search-cli-token status
else
    echo 'CLI client not built - skipping'
fi

echo '=== Agent Tests (if present) ==='
if [ -f "/app/$PROJECTNAME-agent" ]; then
    "/app/$PROJECTNAME-agent" --version
    "/app/$PROJECTNAME-agent" --help >/dev/null
else
    echo 'Agent not built - skipping'
fi

echo '=== Stopping Server ==='
kill "$SERVER_PID"
wait "$SERVER_PID" 2>/dev/null || true
echo '=== All tests passed ==='
ALPINE

echo "Docker tests completed successfully"
