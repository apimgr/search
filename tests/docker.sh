#!/usr/bin/env bash
# @@License : WTFPL
# Per AI.md PART 28: Integration testing in a Docker alpine container
set -eo pipefail

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
if ! command -v docker >/dev/null 2>&1; then
    echo 'ERROR: Docker is required for tests/docker.sh.' >&2
    exit 69
fi
TMP_BASE=$TMPDIR
if [ -z "$TMP_BASE" ]; then
    TMP_BASE=/tmp
fi
TMP_ROOT="$TMP_BASE/$PROJECTORG"

random_suffix() {
    printf '%08x' "$(((RANDOM << 15) | RANDOM))"
}

mkdir -p "$TMP_ROOT"
RUNTIME_DIR=$(mktemp -d "$TMP_ROOT/$PROJECTNAME-XXXXXX")
mkdir -p "$RUNTIME_DIR/config" "$RUNTIME_DIR/data"
chmod 700 "$RUNTIME_DIR"
chmod 755 "$RUNTIME_DIR/config" "$RUNTIME_DIR/data"
# Keep test binaries with the runtime data so the scoped cleanup removes both.
BIN_DIR="${BINDIR:-$RUNTIME_DIR/bin}"
echo "Building all available binaries with 'make build' into $BIN_DIR..."
make build BINDIR="$BIN_DIR"
case "$BIN_DIR" in
    /*) BIN_PATH="$BIN_DIR" ;;
    *) BIN_PATH="$PWD/$BIN_DIR" ;;
esac
TEST_SUFFIX=$(random_suffix)
CONTAINER_NAME="$PROJECTNAME-$TEST_SUFFIX"
NETWORK_NAME="$PROJECTNAME-test-$TEST_SUFFIX"

cleanup() {
    docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
    docker network rm "$NETWORK_NAME" >/dev/null 2>&1 || true
    case "$RUNTIME_DIR" in
        "$TMP_ROOT/$PROJECTNAME-"*) rm -rf -- "$RUNTIME_DIR" ;;
        *) printf 'Refusing to remove unexpected runtime directory: %s\n' "$RUNTIME_DIR" >&2; return 1 ;;
    esac
}
trap cleanup EXIT INT TERM
docker network create "$NETWORK_NAME" >/dev/null

echo "Testing in Docker (Alpine)..."
timeout -k 30 "${DOCKER_TIMEOUT:-3600}" docker run --rm -i \
  --name "$CONTAINER_NAME" --memory=4g --cpus=2 \
  --network "$NETWORK_NAME" \
  -e PROJECTNAME="$PROJECTNAME" \
  -e PROJECTORG="$PROJECTORG" \
  -e TOR_WAIT="${TOR_WAIT:-300}" \
  -e I2P_WAIT="${I2P_WAIT:-600}" \
  -e REQUIRE_OVERLAY="${REQUIRE_OVERLAY:-0}" \
  -v "$BIN_PATH:/app" \
  -v "$RUNTIME_DIR/config:/config" \
  -v "$RUNTIME_DIR/data:/data" \
  alpine:latest sh -s <<'ALPINE'
set -eu

apk add --no-cache curl bash file jq tor i2pd tzdata ca-certificates >/dev/null

mkdir -p "${TMPDIR:-/tmp}/$PROJECTORG"
TEST_TMP=$(mktemp -d "${TMPDIR:-/tmp}/$PROJECTORG/$PROJECTNAME-XXXXXX")
CONFIG_DIR=/config/$PROJECTNAME
SERVER_LOG="$TEST_TMP/server.log"
BOOTSTRAP_LOG="$TEST_TMP/bootstrap.log"
trap 'kill "${SERVER_PID:-}" "${BOOTSTRAP_PID:-}" 2>/dev/null || true; rm -rf -- "$TEST_TMP"' EXIT

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

echo '=== Initializing Test Configuration ==='
"/app/$PROJECTNAME" --port 64580 >"$BOOTSTRAP_LOG" 2>&1 &
BOOTSTRAP_PID=$!
attempt=0
while [ ! -f "$BOOTSTRAP_LOG" ] || ! grep -q 'Server listening' "$BOOTSTRAP_LOG"; do
    if ! kill -0 "$BOOTSTRAP_PID" 2>/dev/null; then
        sed -E 's/(Operator Token: ).*/\1[REDACTED]/' "$BOOTSTRAP_LOG" | tail -40
        echo 'FAILED: server exited before initial startup'
        exit 1
    fi
    if [ "$attempt" -ge "$TOR_WAIT" ]; then
        sed -E 's/(Operator Token: ).*/\1[REDACTED]/' "$BOOTSTRAP_LOG" | tail -40
        echo "FAILED: initial startup exceeded ${TOR_WAIT}s"
        exit 1
    fi
    sleep 1
    attempt=$((attempt + 1))
done

if [ ! -r "$CONFIG_DIR/secrets.yml" ]; then
    ls -ld "$CONFIG_DIR" 2>&1 || true
    echo 'FAILED: secrets.yml was not created in the mounted config directory'
    sed -E 's/(Operator Token: ).*/\1[REDACTED]/' "$BOOTSTRAP_LOG" | tail -40
    exit 1
fi
API_TOKEN=$(awk '
    /^server:$/ { in_server = 1; next }
    in_server && /^  token:/ { sub(/^  token:[[:space:]]*/, ""); print; exit }
    in_server && /^[^ ]/ { exit }
' "$CONFIG_DIR/secrets.yml" 2>/dev/null || true)
if [ -z "$API_TOKEN" ]; then
    echo 'FAILED: generated operator token was not found in secrets.yml'
    exit 1
fi
kill "$BOOTSTRAP_PID"
wait "$BOOTSTRAP_PID" 2>/dev/null || true

mkdir -p "$CONFIG_DIR/yaml.d"
cat > "$CONFIG_DIR/yaml.d/50-integration-test.yml" <<'YAML'
server:
  security:
    allowlist:
      - cidr: 127.0.0.1/32
        description: Docker integration test loopback
      - cidr: ::1/128
        description: Docker integration test loopback
YAML
chown -R search:search "$CONFIG_DIR/yaml.d"
chmod 0700 "$CONFIG_DIR/yaml.d"
chmod 0600 "$CONFIG_DIR/yaml.d/50-integration-test.yml"

echo '=== Starting Server for API Tests ==='
"/app/$PROJECTNAME" --port 64580 >"$SERVER_LOG" 2>&1 &
SERVER_PID=$!
attempt=0
ready=false
while [ "$attempt" -lt "$TOR_WAIT" ]; do
    status=$(curl -q -LSsf --max-time 1 -o /dev/null -w '%{http_code}' http://127.0.0.1:64580/api/v1/server/healthz 2>/dev/null || true)
    case "$status" in
        2??)
            ready=true
            break
            ;;
    esac
    if ! kill -0 "$SERVER_PID" 2>/dev/null; then
        sed -E 's/(Operator Token: ).*/\1[REDACTED]/' "$SERVER_LOG" | tail -40
        echo 'FAILED: server exited before becoming ready'
        exit 1
    fi
    sleep 1
    attempt=$((attempt + 1))
done
if [ "$ready" != true ]; then
    sed -E 's/(Operator Token: ).*/\1[REDACTED]/' "$SERVER_LOG" | tail -40
    echo "FAILED: server did not become ready within ${TOR_WAIT}s"
    exit 1
fi

check_http() {
    label=$1
    url=$2
    shift 2
    status=$(curl -q -LSsf --max-time 20 "$@" -o /dev/null -w '%{http_code}' "$url") || status=000
    case "$status" in
        2??) printf '✓ %s (%s)\n' "$label" "$status" ;;
        *) printf 'FAILED: %s returned HTTP %s\n' "$label" "$status" >&2; return 1 ;;
    esac
}

check_status() {
    label=$1
    expected=$2
    url=$3
    shift 3
    status=$(curl -q -LSs --max-time 10 "$@" -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
    if [ "$status" != "$expected" ]; then
        printf 'FAILED: %s returned HTTP %s, expected %s\n' "$label" "$status" "$expected" >&2
        return 1
    fi
    printf '✓ %s (%s)\n' "$label" "$status"
}

check_overlay() {
    feature=$1
    timeout_seconds=$2
    suffix=$3
    hostname=
    status=
    attempt=0
    while [ "$attempt" -lt "$timeout_seconds" ]; do
        health=$(curl -q -LSsf --max-time 10 "$BASE_URL/api/v1/server/healthz" 2>/dev/null || true)
        status=$(printf '%s' "$health" | jq -r ".features.$feature.status // empty" 2>/dev/null || true)
        if printf '%s' "$health" | grep -Eq '/(etc|var|home|config|data)/'; then
            printf 'FAILED: %s health response leaked a filesystem path\n' "$feature" >&2
            return 1
        fi
        if [ "$status" = healthy ]; then
            hostname=$(printf '%s' "$health" | jq -r ".features.$feature.hostname // empty")
            break
        fi
        case "$status" in
            starting|unavailable) ;;
            *)
                printf 'FAILED: %s reported unexpected status %s\n' "$feature" "${status:-missing}" >&2
                return 1
                ;;
        esac
        sleep 1
        attempt=$((attempt + 1))
    done

    if [ "$status" != healthy ]; then
        if [ "$REQUIRE_OVERLAY" = 1 ]; then
            printf 'FAILED: %s did not bootstrap within %ss (status: %s)\n' "$feature" "$timeout_seconds" "${status:-missing}" >&2
            return 1
        fi
        if [ "$status" = starting ] || [ "$status" = unavailable ]; then
            echo 'SKIP: no outbound network' >&2
            printf 'SKIP: %s bootstrap did not complete (status: %s)\n' "$feature" "$status" >&2
            return 0
        fi
        printf 'FAILED: %s did not become healthy\n' "$feature" >&2
        return 1
    fi

    if ! printf '%s' "$hostname" | grep -Eq "$suffix$"; then
        printf 'FAILED: %s returned invalid hostname %s\n' "$feature" "${hostname:-missing}" >&2
        return 1
    fi
    if [ "$(printf '%s' "$health" | jq -r ".features.$feature.enabled")" != true ] || \
        [ "$(printf '%s' "$health" | jq -r ".checks.$feature // empty")" != ok ]; then
        printf 'FAILED: %s health contract is incomplete\n' "$feature" >&2
        return 1
    fi
    if [ "$feature" = i2p ] && [ "$(printf '%s' "$health" | jq -r '.features.i2p.provider')" != i2pd ]; then
        printf 'FAILED: I2P provider is not i2pd\n' >&2
        return 1
    fi
    case "$feature" in
        tor) key_file="/data/$PROJECTNAME/tor/site/hostname" ;;
        i2p) key_file="/data/$PROJECTNAME/i2p/site/site-keys.dat" ;;
    esac
    if [ ! -s "$key_file" ]; then
        printf 'FAILED: %s identity was not persisted to %s\n' "$feature" "$key_file" >&2
        return 1
    fi
    printf '✓ %s healthy (%s)\n' "$feature" "$hostname"
}

BASE_URL=http://127.0.0.1:64580
echo '=== API Endpoint Tests ==='
check_http 'API health JSON' "$BASE_URL/api/v1/server/healthz"
check_http 'API health plain text' "$BASE_URL/api/v1/server/healthz.txt"
check_http 'Frontend health JSON negotiation' "$BASE_URL/server/healthz" -H 'Accept: application/json'
check_http 'Frontend health text negotiation' "$BASE_URL/server/healthz" -H 'Accept: text/plain'
HTML_HEADERS=$(curl -q -LSs --max-time 20 -H 'Accept: text/html' -D - -o /dev/null "$BASE_URL/" 2>/dev/null || true)
if printf '%s\n' "$HTML_HEADERS" | grep -i '^Content-Type: text/html' >/dev/null; then
    echo '✓ Browser Accept header receives HTML'
else
    echo 'FAILED: browser Accept header did not receive HTML' >&2
    exit 1
fi

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

echo '=== Overlay Tests ==='
check_overlay tor "$TOR_WAIT" '\.onion'
check_overlay i2p "$I2P_WAIT" '\.b32\.i2p'

echo '=== Operator Token Tests ==='
OPERATOR_ROUTE="$BASE_URL/api/v1/server/status"
check_status 'Operator route without token' 401 "$OPERATOR_ROUTE"
check_status 'Operator route with wrong token' 401 "$OPERATOR_ROUTE" -H 'Authorization: Bearer adm_invalid_test_token'
check_status 'Operator route with server token' 200 "$OPERATOR_ROUTE" -H "Authorization: Bearer $API_TOKEN"
PRIMARY_RESULT=$(curl -q -LSs --max-time 15 -H 'Accept: application/json' -H 'Content-Type: application/json' -d '{"query":"docker operator auth test","email":"docker-test@example.invalid","frequency":"daily","deliver_rss":true}' -w '\n%{http_code}' "$BASE_URL/api/v1/alerts" 2>/dev/null || true)
PRIMARY_STATUS=$(printf '%s\n' "$PRIMARY_RESULT" | tail -n 1)
PRIMARY_BODY=$(printf '%s\n' "$PRIMARY_RESULT" | sed '$d')
OWNER_TOKEN=$(printf '%s' "$PRIMARY_BODY" | jq -r '.data.owner_token // empty' 2>/dev/null || true)
if [ "$PRIMARY_STATUS" = 201 ] && [ -n "$OWNER_TOKEN" ]; then
    printf '✓ Owner token issued for operator-auth coverage (%s)\n' "$PRIMARY_STATUS"
    check_status 'Operator route with valid owner token' 403 "$OPERATOR_ROUTE" -H "Authorization: Bearer $OWNER_TOKEN"
    LINKED_RESULT=$(curl -q -LSs --max-time 15 -X POST -H 'Accept: application/json' -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' -d '{"name":"docker-test","expires":"never"}' -w '\n%{http_code}' "$BASE_URL/api/v1/server/preferences/tokens" 2>/dev/null || true)
    LINKED_STATUS=$(printf '%s\n' "$LINKED_RESULT" | tail -n 1)
    LINKED_BODY=$(printf '%s\n' "$LINKED_RESULT" | sed '$d')
    LINKED_TOKEN=$(printf '%s' "$LINKED_BODY" | jq -r '.data.linked_token // empty' 2>/dev/null || true)
    if [ "$LINKED_STATUS" = 201 ] && [ -n "$LINKED_TOKEN" ]; then
        printf '✓ Linked token issued for operator-auth coverage (%s)\n' "$LINKED_STATUS"
        check_status 'Operator route with valid linked token' 403 "$OPERATOR_ROUTE" -H "Authorization: Bearer $LINKED_TOKEN"
    else
        printf 'FAILED: linked token creation returned HTTP %s\n' "$LINKED_STATUS" >&2
        exit 1
    fi
else
    printf 'FAILED: owner token creation returned HTTP %s\n' "$PRIMARY_STATUS" >&2
    exit 1
fi
QUERY_STATUS=$(curl -q -LSs --max-time 10 -o /dev/null -w '%{http_code}' "${OPERATOR_ROUTE}?token=${API_TOKEN}" 2>/dev/null || true)
if [ "$QUERY_STATUS" = 401 ]; then
    echo '✓ Query-string operator token rejected (401)'
else
    printf 'FAILED: query-string operator token returned HTTP %s\n' "$QUERY_STATUS" >&2
    exit 1
fi

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
    printf '%s' "$LINKED_TOKEN" >"$TEST_TMP/cli-token"
    chmod 600 "$TEST_TMP/cli-token"
    expected_health=$(curl -q -LSsf --max-time 10 "$BASE_URL/api/v1/server/healthz" | jq -r '.status')
    set +e
    cli_status_output=$("/app/$PROJECTNAME-cli" --server "$BASE_URL" --token-file "$TEST_TMP/cli-token" status 2>&1)
    cli_status_code=$?
    set -e
    if [ "$cli_status_code" -gt 1 ] || ! printf '%s\n' "$cli_status_output" | grep -Eq "status[[:space:]]+$expected_health"; then
        printf 'FAILED: CLI status did not report server health %s (exit %s)\n' "$expected_health" "$cli_status_code" >&2
        printf '%s\n' "$cli_status_output" >&2
        exit 1
    fi
    if [ "$cli_status_code" -eq 1 ] && ! printf '%s\n' "$cli_status_output" | grep -q '^Error:'; then
        echo 'FAILED: CLI status exited unsuccessfully without a reason' >&2
        printf '%s\n' "$cli_status_output" >&2
        exit 1
    fi
    printf '✓ CLI health status (%s, exit %s)\n' "$expected_health" "$cli_status_code"
    if "/app/$PROJECTNAME-cli" --server "$BASE_URL" --token-file "$TEST_TMP/cli-token" engines >"$TEST_TMP/cli-read" 2>&1 && [ -s "$TEST_TMP/cli-read" ]; then
        echo '✓ CLI read command returns engine data with linked token'
    else
        echo 'FAILED: CLI read command failed with linked token' >&2
        exit 1
    fi
    if "/app/$PROJECTNAME-cli" --server "$BASE_URL" --token-file "$TEST_TMP/cli-token" alerts create query="docker-cli-auth-test" email="docker-cli@example.invalid" deliver_rss=true >"$TEST_TMP/cli-write" 2>&1; then
        echo '✓ CLI write command creates an RSS alert with linked token'
    else
        echo 'FAILED: CLI write command failed with linked token' >&2
        sed -E 's/((manage_token|rss_token|owner_token|webhook_secret)["= :]+)[^",[:space:]]+/\1[REDACTED]/Ig' "$TEST_TMP/cli-write" >&2
        exit 1
    fi
    if "/app/$PROJECTNAME-cli" --server "$BASE_URL" --token-file "$TEST_TMP/cli-token" server-config >"$TEST_TMP/cli-linked-denied" 2>&1; then
        echo 'FAILED: CLI operator command accepted an owner linked token' >&2
        exit 1
    elif ! grep -Eqi 'operator|forbidden|authori[sz]ation|authenticat|credential' "$TEST_TMP/cli-linked-denied"; then
        echo 'FAILED: CLI linked-token failure has no authorization message' >&2
        sed -E 's/(Bearer[[:space:]]+)[^[:space:]]+/\1[REDACTED]/Ig; s/((token|secret|password)[^=:]*[=:][[:space:]]*)[^[:space:],}]*/\1[REDACTED]/Ig' "$TEST_TMP/cli-linked-denied" >&2
        exit 1
    fi
    printf 'adm_invalid_test_token' >"$TEST_TMP/cli-invalid-token"
    chmod 600 "$TEST_TMP/cli-invalid-token"
    if "/app/$PROJECTNAME-cli" --server "$BASE_URL" --token-file "$TEST_TMP/cli-invalid-token" server-config >"$TEST_TMP/cli-invalid-denied" 2>&1; then
        echo 'FAILED: CLI accepted an invalid operator token' >&2
        exit 1
    elif ! grep -Eqi 'operator|forbidden|authori[sz]ation|authenticat|credential' "$TEST_TMP/cli-invalid-denied"; then
        echo 'FAILED: CLI invalid-token failure has no authorization message' >&2
        sed -E 's/(Bearer[[:space:]]+)[^[:space:]]+/\1[REDACTED]/Ig; s/((token|secret|password)[^=:]*[=:][[:space:]]*)[^[:space:],}]*/\1[REDACTED]/Ig' "$TEST_TMP/cli-invalid-denied" >&2
        exit 1
    fi
    echo '✓ CLI reports clear authorization errors for linked and invalid tokens'
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
