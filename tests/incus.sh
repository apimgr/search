#!/usr/bin/env bash
set -eo pipefail

# Check if incus is available
if ! command -v incus &>/dev/null; then
    echo "ERROR: incus not found. Install incus or use tests/docker.sh" >&2
    exit 69
fi
if ! command -v docker >/dev/null 2>&1; then
    echo "ERROR: Docker is required to build binaries for Incus tests." >&2
    exit 69
fi

# Usage: tests/incus.sh [systemd|openrc|all]   (default: all)
# systemd runs in a Debian container, openrc in an Alpine container; each gets the full test run
MODE="${1:-all}"
case "$MODE" in
    systemd|openrc) MODES="$MODE" ;;
    all) MODES="systemd openrc" ;;
    *) echo "Usage: $0 [systemd|openrc|all]" >&2; exit 2 ;;
esac

# Resolve project identity from IDEA.md first, with the documented repository/path fallbacks.
read_idea_value() {
    [ -r IDEA.md ] || return 0
    sed -n "/^$1:[[:space:]]*/ { s/^$1:[[:space:]]*//; p; q; }" IDEA.md | tr -d '[:space:]'
}

ORIGIN_URL=$(git remote get-url origin 2>/dev/null || true)
PROJECT_NAME=$(read_idea_value project_name)
PROJECT_ORG=$(read_idea_value project_org)
if [ -z "$PROJECT_NAME" ]; then
    PROJECT_NAME=$(printf '%s\n' "$ORIGIN_URL" | sed -E 's|.*/([^/]+)$|\1|; s/\.git$//')
fi
if [ -z "$PROJECT_ORG" ]; then
    PROJECT_ORG=$(printf '%s\n' "$ORIGIN_URL" | sed -E 's|.*[:/]([^/:]+)/[^/]+(\.git)?$|\1|')
fi
PROJECT_NAME=${PROJECT_NAME:-$(basename "$PWD")}
PROJECT_ORG=${PROJECT_ORG:-$(basename "$(dirname "$PWD")")}
IDEA_INTERNAL_NAME=$(read_idea_value internal_name)
IDEA_INTERNAL_ORG=$(read_idea_value internal_org)
INTERNAL_NAME=${IDEA_INTERNAL_NAME:-${INTERNAL_NAME:-$PROJECT_NAME}}
INTERNAL_ORG=${IDEA_INTERNAL_ORG:-${INTERNAL_ORG:-$PROJECT_ORG}}
random_suffix() {
    head -c6 /dev/urandom | od -An -tx1 | tr -d ' \n' | cut -c1-8
}

CONTAINER_PREFIX="${PROJECT_NAME}-$(random_suffix)"
CONTAINERS=()
TMP_BASE="${TMPDIR:-/tmp}"
TMP_ROOT="$TMP_BASE/$PROJECT_ORG"
mkdir -p "$TMP_ROOT"
INCUS_TMP=$(mktemp -d "$TMP_ROOT/$INTERNAL_NAME-XXXXXX")
BIN_DIR="${BINDIR:-$INCUS_TMP/bin}"
INCUS_NETWORK="${PROJECT_NAME}-test-${CONTAINER_PREFIX##*-}"
NETWORK_CREATED=0

# Incus images, one per init system: systemd uses the latest Debian stable, OpenRC uses the
# latest stable Alpine release (update both when new stable releases ship)
INCUS_IMAGE_SYSTEMD="images:debian/trixie"
INCUS_IMAGE_OPENRC="images:alpine/3.24"

# Resource caps for every instance (the counterparts of DOCKER_CPUS and DOCKER_MEM); override from the environment
INCUS_CPUS="${INCUS_CPUS:-2}"
INCUS_MEM="${INCUS_MEM:-4GiB}"

# CPU priority for a container (0 lowest, 10 highest, the Incus default); only matters while CPU is contended
INCUS_CPU_PRIORITY="${INCUS_CPU_PRIORITY:-2}"

# Ceilings in seconds: one quick exec step, a package install, and the whole remote test run; override from the environment
INCUS_OP_TIMEOUT="${INCUS_OP_TIMEOUT:-60}"
INCUS_INSTALL_TIMEOUT="${INCUS_INSTALL_TIMEOUT:-900}"
INCUS_RUN_TIMEOUT="${INCUS_RUN_TIMEOUT:-3600}"

cleanup() {
    local c
    for c in "${CONTAINERS[@]}"; do incus delete "$c" --force 2>/dev/null || true; done
    if [ "$NETWORK_CREATED" -eq 1 ]; then
        incus network delete "$INCUS_NETWORK" 2>/dev/null || true
    fi
    case "$INCUS_TMP" in
        "$TMP_ROOT/$INTERNAL_NAME-"*) rm -rf -- "$INCUS_TMP" ;;
        *) printf 'Refusing to remove unexpected temp directory: %s\n' "$INCUS_TMP" >&2; return 1 ;;
    esac
}
trap cleanup EXIT INT TERM

# Build — use Makefile if present (standard for all bootstrapped projects)
# Output always lands in $BIN_DIR
if [ -f "Makefile" ]; then
    echo "Building with make build..."
    make build BINDIR="$BIN_DIR"
else
    # No Makefile yet — fallback to direct docker run
    echo "Building in Docker (no Makefile)..."
    GO_CACHE="${GO_CACHE:-$HOME/go/pkg/mod}"
    GO_BUILD="${GO_BUILD:-$HOME/.cache/go-build/${PROJECT_NAME}}"
    mkdir -p "$GO_CACHE" "$GO_BUILD" "$BIN_DIR"
    case "$BIN_DIR" in
        /*) BIN_PATH="$BIN_DIR" ;;
        *) BIN_PATH="$PWD/$BIN_DIR" ;;
    esac
    docker run --rm \
      --name "${PROJECT_NAME}-$(random_suffix)" \
      -v "$PWD:/app" \
      -v "$BIN_PATH:/out" \
      -v "$GO_CACHE:/usr/local/share/go/pkg/mod" \
      -v "$GO_BUILD:/usr/local/share/go/cache" \
      -w /app -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false \
      casjaysdev/go:latest go build -buildvcs=false -trimpath -ldflags "-s -w" -o "/out/${PROJECT_NAME}" ./src
    if [ -d "src/client" ]; then
        docker run --rm \
          --name "${PROJECT_NAME}-$(random_suffix)" \
          -v "$PWD:/app" \
          -v "$BIN_PATH:/out" \
          -v "$GO_CACHE:/usr/local/share/go/pkg/mod" \
          -v "$GO_BUILD:/usr/local/share/go/cache" \
          -w /app -e CGO_ENABLED=0 -e GOFLAGS=-buildvcs=false \
          casjaysdev/go:latest go build -buildvcs=false -trimpath -ldflags "-s -w" -o "/out/${PROJECT_NAME}-cli" ./src/client
    fi
fi

incus network create "$INCUS_NETWORK" >/dev/null
NETWORK_CREATED=1

REMOTE_LIB=$(cat <<'REMOTE_LIB_EOF'
INTERNAL_NAME="${INTERNAL_NAME:-$PROJECT_NAME}"
INTERNAL_ORG="${INTERNAL_ORG:-$PROJECT_ORG}"
FAILURES=0
fail() { echo "✗ FAILED: $*" >&2; FAILURES=$((FAILURES + 1)); }
pass() { echo "✓ $*"; }
check() { local d="$1"; shift; if "$@" >/dev/null 2>&1; then pass "$d"; else fail "$d"; fi; }
check_not() { local d="$1"; shift; if "$@" >/dev/null 2>&1; then fail "$d"; else pass "$d"; fi; }
wait_for() { local n="$1" i; shift; for i in $(seq 1 "$n"); do "$@" >/dev/null 2>&1 && return 0; sleep 1; done; return 1; }
TMP_ROOT="${TMPDIR:-/tmp}/${PROJECT_ORG}"
mkdir -p "$TMP_ROOT"
TEST_TMP=$(mktemp -d "$TMP_ROOT/${INTERNAL_NAME}-XXXXXX")
chmod 0700 "$TEST_TMP"
cleanup_test_tmp() {
    case "$TEST_TMP" in
        "$TMP_ROOT/$INTERNAL_NAME-"*) rm -rf -- "$TEST_TMP" ;;
        *) echo "Refusing to remove unexpected test directory: $TEST_TMP" >&2; return 1 ;;
    esac
}
trap cleanup_test_tmp EXIT INT TERM

# A native service listens on the port the server saved in server.yml (a random 64xxx port; 80 is only the container default), so read it there
CONFIG_FILE="/etc/${INTERNAL_ORG}/${INTERNAL_NAME}/server.yml"
# server.port is read inside the server: section at ANY indentation; a dual port such as "8090,8443" yields its first number
port() { awk '/^server:/ {s=1; next} /^[^[:space:]#]/ {s=0} s && /^[[:space:]]+port:/ {v=$2; gsub(/[^0-9,]/, "", v); sub(/,.*/, "", v); print v; exit}' "$CONFIG_FILE" 2>/dev/null; }
api() { echo "http://localhost:$(port)/api/v1/server/healthz"; }
api_base() { echo "http://localhost:$(port)/api/v1"; }
# The health helper asks for JSON: without an Accept header a curl client gets plain text and every jq check below would fail
healthz() { curl -q -LSsf --max-time 10 -H 'Accept: application/json' "$(api)"; }
healthz_up() { healthz | jq -e '.status' >/dev/null; }
overlay() { healthz | jq -r ".features.$1.$2"; }

# One set of service helpers per init system; the test steps are identical for both
if [ "$INIT_MODE" = "systemd" ]; then
    UNIT_FILE="/etc/systemd/system/${INTERNAL_NAME}.service"
    svc_active()  { systemctl is-active --quiet "$INTERNAL_NAME"; }
    svc_enabled() { systemctl is-enabled --quiet "$INTERNAL_NAME"; }
    svc_pid()     { systemctl show -p MainPID --value "$INTERNAL_NAME"; }
else
    UNIT_FILE="/etc/init.d/${INTERNAL_NAME}"
    svc_active()  { local service_status; service_status=$(rc-service "$INTERNAL_NAME" status 2>&1) || return 1; [[ "$service_status" == *started* ]]; }
    svc_enabled() { local enabled_services; enabled_services=$(rc-update show default) || return 1; [[ "$enabled_services" == *"$INTERNAL_NAME"* ]]; }
    svc_pid()     { cat "/var/run/${INTERNAL_ORG}/${INTERNAL_NAME}/${INTERNAL_NAME}.pid"; }
fi
svc_stopped() { ! svc_active; }
REMOTE_LIB_EOF
)

REMOTE_MAIN=$(cat <<'REMOTE_MAIN_EOF'
echo "=== [$INIT_MODE] Init system identity ==="
if [ "$INIT_MODE" = "systemd" ]; then
    check "PID 1 is systemd" test "$(cat /proc/1/comm)" = "systemd"
else
    check "OpenRC is running" test -d /run/openrc
    check "no systemd in this container" test ! -d /run/systemd/system
fi

echo "=== Version Check ==="
${PROJECT_NAME} --version

echo "=== Help Check ==="
${PROJECT_NAME} --help

echo "=== Binary Info ==="
ls -lh /usr/local/bin/${PROJECT_NAME}
file /usr/local/bin/${PROJECT_NAME}

echo "=== [$INIT_MODE] Service install (--service --install also enables and starts) ==="
${PROJECT_NAME} --service --install
check "service file installed at ${UNIT_FILE}" test -f "$UNIT_FILE"
if [ "$INIT_MODE" = "openrc" ]; then
    check "init script starts with #!/sbin/openrc-run" awk 'NR == 1 { if ($0 == "#!/sbin/openrc-run") exit 0; exit 1 }' "$UNIT_FILE"
    check "init script is executable" test -x "$UNIT_FILE"
    check_not "init script is not a SysV script" grep -q 'BEGIN INIT INFO' "$UNIT_FILE"
fi
check "service enabled at boot" svc_enabled
check "service active after install" wait_for 30 svc_active
check "health endpoint answers after install" wait_for 60 healthz_up
check "service account exists" getent passwd "$INTERNAL_NAME"
check "server runs as the service account after the privilege drop" test "$(stat -c %U /proc/$(svc_pid))" = "$INTERNAL_NAME"

echo "=== [$INIT_MODE] Service restart ==="
OLD_PID=$(svc_pid)
${PROJECT_NAME} --service restart
check "service active after restart" wait_for 30 svc_active
check "health endpoint answers after restart" wait_for 60 healthz_up
check "restart replaced the process" test "$(svc_pid)" != "$OLD_PID"

echo "=== [$INIT_MODE] Service stop and start ==="
${PROJECT_NAME} --service stop
check "service stopped" wait_for 30 svc_stopped
check_not "health endpoint is down while stopped" curl -q -LSsf --max-time 3 "$(api)"
${PROJECT_NAME} --service start
check "service active after start" wait_for 30 svc_active
check "health endpoint answers after start" wait_for 60 healthz_up

echo "=== API Endpoint Tests ==="
# JSON response (default)
curl -q -LSsf http://localhost:$(port)/api/v1/server/healthz || fail "/api/v1/server/healthz"
# .txt extension (plain text)
curl -q -LSsf http://localhost:$(port)/api/v1/server/healthz.txt || fail "/api/v1/server/healthz.txt"
# Accept: application/json
curl -q -LSsf -H 'Accept: application/json' http://localhost:$(port)/server/healthz || fail "Accept JSON"
# Accept: text/plain
curl -q -LSsf -H 'Accept: text/plain' http://localhost:$(port)/server/healthz || fail "Accept text/plain"

echo "=== Operator-token Authentication Tests ==="
SECRETS_FILE="/etc/${INTERNAL_ORG}/${INTERNAL_NAME}/secrets.yml"
OPERATOR_TOKEN=$(awk '/^server:/ {s=1; next} /^[^[:space:]#]/ {s=0} s && /^[[:space:]]+token:/ {sub(/^[[:space:]]+token:[[:space:]]*/, ""); gsub(/^\047|\047$|^\042|\042$/, ""); print; exit}' "$SECRETS_FILE")
OPERATOR_ROUTE="$(api_base)/server/status"
http_status() {
    local label="$1" want="$2" url="$3" token="${4:-}" status
    if [ -n "$token" ]; then
        status=$(curl -q -LSs --max-time 10 -o /dev/null -w '%{http_code}' -H "Authorization: Bearer $token" "$url" 2>/dev/null || true)
    else
        status=$(curl -q -LSs --max-time 10 -o /dev/null -w '%{http_code}' "$url" 2>/dev/null || true)
    fi
    printf '%s\n' "$status"
    if [ "$status" = "$want" ]; then pass "$label ($status)"; else fail "$label returned HTTP $status, expected $want"; fi
}
check "operator token was loaded from secrets.yml" test -n "$OPERATOR_TOKEN"
http_status "missing operator token rejected" 401 "$OPERATOR_ROUTE" >/dev/null
http_status "wrong operator token rejected" 401 "$OPERATOR_ROUTE" "adm_invalid_test_token" >/dev/null
http_status "operator token accepted" 200 "$OPERATOR_ROUTE" "$OPERATOR_TOKEN" >/dev/null
PRIMARY_RESULT=$(curl -q -LSs --max-time 15 -H 'Content-Type: application/json' -d '{"query":"incus operator auth test","email":"incus-test@example.invalid","frequency":"daily","deliver_rss":true}' -w '\n%{http_code}' "$(api_base)/alerts" 2>/dev/null || true)
PRIMARY_STATUS=${PRIMARY_RESULT##*$'\n'}
PRIMARY_BODY=${PRIMARY_RESULT%$'\n'*}
OWNER_TOKEN=$(printf '%s' "$PRIMARY_BODY" | jq -r '.data.owner_token // empty' 2>/dev/null || true)
if [ "$PRIMARY_STATUS" = "201" ] && [ -n "$OWNER_TOKEN" ]; then
    pass "owner token issued by the test alert for authorization coverage (201)"
    http_status "valid owner token forbidden on operator route" 403 "$OPERATOR_ROUTE" "$OWNER_TOKEN" >/dev/null
    LINKED_RESULT=$(curl -q -LSs --max-time 15 -X POST -H "Authorization: Bearer $OWNER_TOKEN" -H 'Content-Type: application/json' -d '{"name":"incus-test","expires":"never"}' -w '\n%{http_code}' "$(api_base)/server/preferences/tokens" 2>/dev/null || true)
    LINKED_STATUS=${LINKED_RESULT##*$'\n'}
    LINKED_BODY=${LINKED_RESULT%$'\n'*}
    LINKED_TOKEN=$(printf '%s' "$LINKED_BODY" | jq -r '.data.linked_token // empty' 2>/dev/null || true)
    if [ "$LINKED_STATUS" = "201" ] && [ -n "$LINKED_TOKEN" ]; then
        pass "linked token issued for authorization coverage (201)"
        http_status "valid linked token forbidden on operator route" 403 "$OPERATOR_ROUTE" "$LINKED_TOKEN" >/dev/null
    else
        fail "linked token creation returned HTTP $LINKED_STATUS"
    fi
else
    fail "primary owner token creation returned HTTP $PRIMARY_STATUS"
fi
QUERY_STATUS=$(curl -q -LSs --max-time 10 -o /dev/null -w '%{http_code}' "${OPERATOR_ROUTE}?token=${OPERATOR_TOKEN}" 2>/dev/null || true)
if [ "$QUERY_STATUS" = "401" ]; then pass "query-string operator token rejected (401)"; else fail "query-string operator token returned HTTP $QUERY_STATUS"; fi

echo "=== Project-Specific Endpoint Tests ==="
# The project's public search features are part of the required integration suite.
curl -q -LSsf --max-time 30 "$(api_base)/search?q=test" -o /dev/null || fail "Search API"
curl -q -LSsf --max-time 20 "$(api_base)/autocomplete?q=test" -o /dev/null || fail "Autocomplete API"
curl -q -LSsf --max-time 20 "$(api_base)/engines" -o /dev/null || fail "Engine catalog API"
curl -q -LSsf --max-time 20 "$(api_base)/search/related?q=test" -o /dev/null || fail "Related search API"
curl -q -LSsf --max-time 20 "$(api_base)/categories" -o /dev/null || fail "Category catalog API"
curl -q -LSsf --max-time 20 "$(api_base)/bangs" -o /dev/null || fail "Bang catalog API"
curl -q -LSsf --max-time 20 "$(api_base)/instant?q=2+2" -o /dev/null || fail "Instant answer API"

echo "=== Frontend Content Negotiation Tests ==="
HTML_HEADERS=$(curl -q -LSsf --max-time 20 -H 'Accept: text/html' -D - -o /dev/null "http://localhost:$(port)/")
if printf '%s\n' "$HTML_HEADERS" | grep -i '^Content-Type: text/html' >/dev/null; then pass "browser Accept header receives HTML"; else fail "browser Accept header did not receive HTML"; fi
curl -q -LSsf --max-time 20 "http://localhost:$(port)/search?q=test" -o /dev/null || fail "Search page"
curl -q -LSsf --max-time 20 "http://localhost:$(port)/server/preferences" -o /dev/null || fail "Preferences page"
curl -q -LSsf --max-time 20 "http://localhost:$(port)/server/about" -o /dev/null || fail "About page"
# MUST test ALL endpoints from IDEA.md - both API and frontend
# Test FULL CRUD if project has CRUD operations
#
# Example for jokes API (API routes with .txt extension):
#   curl -q -LSsf http://localhost:$(port)/api/v1/jokes/random || fail "API JSON"
#   curl -q -LSsf http://localhost:$(port)/api/v1/jokes/random.txt || fail "API .txt"
#   curl -q -LSsf -H 'Accept: text/plain' http://localhost:$(port)/api/v1/jokes/random || fail "API Accept text"
#
# Example for jokes frontend (smart detection, no .txt - test with text for simplicity):
#   JOKE=$(curl -q -LSsf http://localhost:$(port)/jokes/random)
#   if echo "$JOKE" | grep "Why" >/dev/null; then pass "Frontend text works"; else fail "Frontend text"; fi
#   curl -q -LSsfI -H 'Accept: text/html' http://localhost:$(port)/jokes/random | grep 'text/html' >/dev/null || fail "Frontend HTML"
#
# Example for open data CRUD (full test suite):
#   curl -q -LSsf http://localhost:$(port)/api/v1/items || fail "LIST items API"
#   curl -q -LSsf http://localhost:$(port)/api/v1/items/1 || fail "GET item API JSON"
#   curl -q -LSsf http://localhost:$(port)/api/v1/items/1.txt || fail "GET item API .txt"
#   curl -q -LSsf -X POST -H 'Content-Type: application/json' -d '{"name":"test"}' http://localhost:$(port)/api/v1/items || fail "CREATE item API"
#   ITEMS=$(curl -q -LSsf http://localhost:$(port)/items)
#
# Test ALL project-specific endpoints defined in IDEA.md

echo "=== Open API Smoke Test ==="
# No token needed for the public endpoints; MCP, metrics and token routes are checked separately (a missing token answers 401)
if curl -q -LSsf http://localhost:$(port)/server/healthz | grep '"status"' >/dev/null; then pass "Health endpoint works"; else fail "Health endpoint"; fi

echo "=== Binary Rename Tests ==="
# Binaries show the ACTUAL name in --help/--version (not hardcoded)
cp "/usr/local/bin/${PROJECT_NAME}" "$TEST_TMP/renamed-server"
chmod +x "$TEST_TMP/renamed-server"
if "$TEST_TMP/renamed-server" --help 2>&1 | grep 'renamed-server' >/dev/null; then pass "Server binary rename works (--help shows actual name)"; else fail "Server --help does not show renamed binary name"; fi

echo "=== Client Tests (if exists) ==="
if [ -f /usr/local/bin/${PROJECT_NAME}-cli ]; then
    ${PROJECT_NAME}-cli --version || fail "CLI --version"
    ${PROJECT_NAME}-cli --help || fail "CLI --help"

    cp "/usr/local/bin/${PROJECT_NAME}-cli" "$TEST_TMP/renamed-cli"
    chmod +x "$TEST_TMP/renamed-cli"
    if "$TEST_TMP/renamed-cli" --help 2>&1 | grep 'renamed-cli' >/dev/null; then pass "CLI binary rename works"; else fail "CLI --help does not show renamed binary name"; fi

    echo '--- CLI Full Functionality Tests ---'
    if [ -n "${LINKED_TOKEN:-}" ]; then
        CLI_TMP="$TEST_TMP/cli"
        mkdir -p "$CLI_TMP"
        chmod 0700 "$CLI_TMP"
        printf '%s' "$LINKED_TOKEN" > "$CLI_TMP/linked-token"
        chmod 0600 "$CLI_TMP/linked-token"
        cli_args=(--server "http://localhost:$(port)" --token-file "$CLI_TMP/linked-token")
        if ${PROJECT_NAME}-cli "${cli_args[@]}" status > "$CLI_TMP/status" 2>&1; then
            pass "CLI status command succeeds with linked token"
        else
            fail "CLI status command failed with linked token"
        fi
        if ${PROJECT_NAME}-cli "${cli_args[@]}" engines > "$CLI_TMP/read" 2>&1 && [ -s "$CLI_TMP/read" ]; then
            pass "CLI read command returns engine data with linked token"
        else
            fail "CLI read command failed with linked token"
        fi
        if ${PROJECT_NAME}-cli "${cli_args[@]}" alerts create query="incus-cli-auth-test" email="incus-cli@example.invalid" deliver_rss=true > "$CLI_TMP/write" 2>&1; then
            pass "CLI write command creates an RSS alert with linked token"
        else
            fail "CLI write command failed with linked token"
        fi
        if ${PROJECT_NAME}-cli --server "http://localhost:$(port)" --token-file "$CLI_TMP/linked-token" server-config > "$CLI_TMP/linked-denied" 2>&1; then
            fail "CLI operator command accepted an owner linked token"
        elif ! grep -Eqi 'operator|forbidden|authorization|credential' "$CLI_TMP/linked-denied"; then
            fail "CLI rejected linked token without a clear authorization message"
        else
            pass "CLI reports a clear authorization error for an owner linked token"
        fi
        printf '%s' 'adm_invalid_test_token' > "$CLI_TMP/invalid-token"
        chmod 0600 "$CLI_TMP/invalid-token"
        if ${PROJECT_NAME}-cli --server "http://localhost:$(port)" --token-file "$CLI_TMP/invalid-token" server-config > "$CLI_TMP/invalid-denied" 2>&1; then
            fail "CLI accepted an invalid operator token"
        elif ! grep -Eqi 'operator|forbidden|authorization|credential' "$CLI_TMP/invalid-denied"; then
            fail "CLI rejected invalid token without a clear authorization message"
        else
            pass "CLI reports a clear authorization error for an invalid token"
        fi
    else
        fail "CLI functional checks require the linked token created by the operator-auth test"
    fi
else
    echo 'client not installed - skipping'
fi

echo "=== Agent Tests (if exists) ==="

echo "=== [$INIT_MODE] Tor hidden service (tor is installed in this container) ==="
DATA_ROOT="/var/lib/${INTERNAL_ORG}/${INTERNAL_NAME}"
CONF_ROOT="/etc/${INTERNAL_ORG}/${INTERNAL_NAME}"
check "tor binary is installed" command -v tor
check "features.tor.enabled is true" test "$(overlay tor enabled)" = "true"
case "$(overlay tor status)" in
    disabled|starting|healthy|unavailable) pass "features.tor.status is one of the four contract words" ;;
    *) fail "features.tor.status is not one of the four contract words" ;;
esac
# Bootstrap needs outbound network. REQUIRE_OVERLAY=1 makes a missing network a failure instead of a SKIP.
for _ in $(seq 1 "${TOR_WAIT:-300}"); do [ "$(overlay tor status)" = "healthy" ] && break; sleep 1; done
if [ "$(overlay tor status)" = "healthy" ]; then
    if [[ "$(overlay tor hostname)" =~ ^[a-z2-7]{56}\.onion$ ]]; then pass "tor hostname is a v3 onion address"; else fail "tor hostname is not a v3 onion address"; fi
    check "features.tor.running is true" test "$(overlay tor running)" = "true"
    check "checks.tor is ok" test "$(healthz | jq -r '.checks.tor')" = "ok"
    check "onion hostname file persisted" test -s "${DATA_ROOT}/tor/site/hostname"
    for d in "${CONF_ROOT}/tor" "${DATA_ROOT}/tor" "${DATA_ROOT}/tor/site"; do
        check "$d is mode 700" test "$(stat -c %a "$d")" = "700"
    done
elif [ "${REQUIRE_OVERLAY:-0}" = "1" ]; then
    fail "Tor did not become healthy within ${TOR_WAIT:-300}s"
else
    echo "SKIP: Tor did not bootstrap within ${TOR_WAIT:-300}s (no outbound network?). The contract is still checked." >&2
    case "$(overlay tor status)" in
        starting) check "starting maps to checks.tor ok" test "$(healthz | jq -r '.checks.tor')" = "ok" ;;
        unavailable) check "unavailable maps to checks.tor degraded" test "$(healthz | jq -r '.checks.tor')" = "degraded" ;;
        *) fail "Tor is neither starting nor unavailable while not healthy" ;;
    esac
fi
if healthz | grep -Eq '/(etc|var|home|config|data)/'; then fail "health response leaks a filesystem path"; else pass "health response carries no filesystem path"; fi

echo "=== [$INIT_MODE] Tor missing binary (must be disabled, never an error) ==="
TOR_BIN=$(command -v tor)
mv "$TOR_BIN" "${TOR_BIN}.hidden"
${PROJECT_NAME} --service restart
wait_for 60 healthz_up
check "enabled is false without the binary" test "$(overlay tor enabled)" = "false"
check "running is false" test "$(overlay tor running)" = "false"
check "status is disabled" test "$(overlay tor status)" = "disabled"
check "hostname is empty" test -z "$(overlay tor hostname)"
check "checks.tor is omitted" test "$(healthz | jq '(.checks | has("tor"))')" = "false"
mv "${TOR_BIN}.hidden" "$TOR_BIN"
${PROJECT_NAME} --service restart
wait_for 60 healthz_up

echo "=== [$INIT_MODE] I2P (off by default in a system container, i2pd is installed) ==="
check "i2pd binary is installed" command -v i2pd
check "server.yml is 0600 after start" test "$(stat -c %a "${CONF_ROOT}/server.yml")" = "600"
check "features.i2p.enabled is false by default in a system container" test "$(overlay i2p enabled)" = "false"
check "status is disabled by default" test "$(overlay i2p status)" = "disabled"
check "checks.i2p is omitted by default" test "$(healthz | jq '(.checks | has("i2p"))')" = "false"

echo "=== [$INIT_MODE] I2P enabled by a drop-in (opt-in, i2pd is installed) ==="
mkdir -p "${CONF_ROOT}/yaml.d"
printf 'server:\n  i2p:\n    enabled: true\n' > "${CONF_ROOT}/yaml.d/50-i2p.yml"
chown "${INTERNAL_NAME}:${INTERNAL_NAME}" "${CONF_ROOT}/yaml.d" "${CONF_ROOT}/yaml.d/50-i2p.yml"
chmod 0700 "${CONF_ROOT}/yaml.d"
chmod 0600 "${CONF_ROOT}/yaml.d/50-i2p.yml"
${PROJECT_NAME} --service restart
wait_for 60 healthz_up
check "features.i2p.enabled is true after opting in" test "$(overlay i2p enabled)" = "true"
check "features.i2p.provider is i2pd" test "$(overlay i2p provider)" = "i2pd"
case "$(overlay i2p status)" in
    starting|healthy|unavailable) pass "features.i2p.status is a contract word for an enabled network" ;;
    *) fail "features.i2p.status is not valid for an enabled network" ;;
esac
for _ in $(seq 1 "${I2P_WAIT:-600}"); do [ "$(overlay i2p status)" = "healthy" ] && break; sleep 1; done
if [ "$(overlay i2p status)" = "healthy" ]; then
    if [[ "$(overlay i2p hostname)" =~ ^[a-z2-7]{52}\.b32\.i2p$ ]]; then pass "i2p hostname is a .b32.i2p address"; else fail "i2p hostname is not a .b32.i2p address"; fi
    check "features.i2p.running is true" test "$(overlay i2p running)" = "true"
    check "checks.i2p is ok" test "$(healthz | jq -r '.checks.i2p')" = "ok"
    check "destination key persisted" sh -c "find '${DATA_ROOT}/i2p/site' -type f -size +0 | grep . >/dev/null"
elif [ "${REQUIRE_OVERLAY:-0}" = "1" ]; then
    fail "I2P did not become healthy within ${I2P_WAIT:-600}s"
else
    echo "SKIP: I2P did not build tunnels within ${I2P_WAIT:-600}s (no outbound network?). The contract is still checked." >&2
fi
if healthz | grep -Eq '/(etc|var|home|config|data)/'; then fail "health response leaks a filesystem path"; else pass "health response carries no filesystem path"; fi

echo "=== [$INIT_MODE] I2P missing provider (enabled by the drop-in, no i2pd: disabled, never an error) ==="
I2PD_BIN=$(command -v i2pd)
mv "$I2PD_BIN" "${I2PD_BIN}.hidden"
${PROJECT_NAME} --service restart
wait_for 60 healthz_up
check "enabled is false without a provider" test "$(overlay i2p enabled)" = "false"
check "status is disabled" test "$(overlay i2p status)" = "disabled"
check "provider is none" test "$(overlay i2p provider)" = "none"
check "checks.i2p is omitted" test "$(healthz | jq '(.checks | has("i2p"))')" = "false"
mv "${I2PD_BIN}.hidden" "$I2PD_BIN"
rm -f "${CONF_ROOT}/yaml.d/50-i2p.yml"
${PROJECT_NAME} --service restart
wait_for 60 healthz_up

[ "$FAILURES" -eq 0 ] || { echo "$FAILURES check(s) failed" >&2; exit 1; }
echo "=== [$INIT_MODE] Main tests passed ==="
REMOTE_MAIN_EOF
)

REMOTE_AFTER=$(cat <<'REMOTE_AFTER_EOF'
echo "=== [$INIT_MODE] Boot persistence (container was restarted) ==="
check "service still enabled after reboot" svc_enabled
check "service came back after reboot" wait_for 120 svc_active
check "health endpoint answers after reboot" wait_for 120 healthz_up

echo "=== [$INIT_MODE] Service disable (keeps the service file) ==="
${PROJECT_NAME} --service --disable
check_not "service not enabled after --disable" svc_enabled
check "service not active after --disable" wait_for 30 svc_stopped
check "service file kept after --disable" test -f "$UNIT_FILE"

echo "=== [$INIT_MODE] Service re-install ==="
${PROJECT_NAME} --service --install
check "service enabled again" svc_enabled
check "service active again" wait_for 30 svc_active
check "health endpoint answers again" wait_for 60 healthz_up

echo "=== [$INIT_MODE] Service uninstall (confirmation prompt answered with y) ==="
printf 'y\n' | ${PROJECT_NAME} --service --uninstall
check_not "service file removed" test -e "$UNIT_FILE"
check_not "service not enabled" svc_enabled
check "service not active" wait_for 30 svc_stopped
check_not "data directory removed" test -d "/var/lib/${INTERNAL_ORG}/${INTERNAL_NAME}"

[ "$FAILURES" -eq 0 ] || { echo "$FAILURES check(s) failed" >&2; exit 1; }
echo "=== [$INIT_MODE] All tests passed ==="
REMOTE_AFTER_EOF
)

# Every exec step has a ceiling: iexec SECONDS CONTAINER COMMAND...
iexec() {
    local secs="$1" container="$2"
    shift 2
    timeout "$secs" incus exec "$container" -- "$@"
}

# Run a script inside the container with the init mode and project names
# Every script fragment is its own argument: printf puts a newline after each one, because $(cat <<EOF) strips the trailing newline of a heredoc
remote() {
    local container="$1" init="$2"
    shift 2
    printf '%s\n' "$@" | timeout "$INCUS_RUN_TIMEOUT" incus exec "$container" -- env INIT_MODE="$init" PROJECT_NAME="$PROJECT_NAME" PROJECT_ORG="$PROJECT_ORG" \
        INTERNAL_NAME="$INTERNAL_NAME" INTERNAL_ORG="$INTERNAL_ORG" REQUIRE_OVERLAY="${REQUIRE_OVERLAY:-0}" \
        TOR_WAIT="${TOR_WAIT:-300}" I2P_WAIT="${I2P_WAIT:-600}" bash -s
}

# Wait until the container accepts commands, its init system finished booting and the network is up
wait_boot() {
    local container="$1" init="$2" state
    for _ in $(seq 1 60); do iexec "$INCUS_OP_TIMEOUT" "$container" true 2>/dev/null && break; sleep 1; done
    if [ "$init" = "systemd" ]; then
        # "degraded" is normal in a container; only "starting" means boot is unfinished
        for _ in $(seq 1 90); do
            state=$(iexec "$INCUS_OP_TIMEOUT" "$container" systemctl is-system-running 2>/dev/null || true)
            [ -n "$state" ] && [ "$state" != "starting" ] && break
            sleep 1
        done
    else
        for _ in $(seq 1 60); do iexec "$INCUS_OP_TIMEOUT" "$container" test -d /run/openrc 2>/dev/null && break; sleep 1; done
    fi
    for _ in $(seq 1 60); do iexec "$INCUS_OP_TIMEOUT" "$container" sh -c 'ip route | grep "^default" >/dev/null' 2>/dev/null && break; sleep 1; done
}

run_mode() {
    local init="$1" image container
    if [ "$init" = "systemd" ]; then image="$INCUS_IMAGE_SYSTEMD"; else image="$INCUS_IMAGE_OPENRC"; fi
    container="${CONTAINER_PREFIX}-${init}"
    CONTAINERS+=("$container")

    echo "=== [$init] Launching Incus container ($image) ==="
    incus launch "$image" "$container" --network "$INCUS_NETWORK" -c limits.cpu="$INCUS_CPUS" -c limits.memory="$INCUS_MEM" -c limits.cpu.priority="$INCUS_CPU_PRIORITY"
    wait_boot "$container" "$init"

    echo "=== [$init] Installing test tools, tor and i2pd ==="
    if [ "$init" = "systemd" ]; then
        iexec "$INCUS_INSTALL_TIMEOUT" "$container" bash -c 'apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq curl jq file tor i2pd' >/dev/null
        # The distro tor/i2pd services are NOT what is tested: the server runs its own dedicated, isolated instances (PART 31)
        iexec "$INCUS_OP_TIMEOUT" "$container" systemctl disable --now tor.service tor@default.service i2pd.service 2>/dev/null || true
    else
        iexec "$INCUS_INSTALL_TIMEOUT" "$container" sh -c 'apk add --no-cache bash curl jq file tor i2pd tzdata ca-certificates' >/dev/null
        iexec "$INCUS_OP_TIMEOUT" "$container" sh -c 'rc-service tor stop; rc-service i2pd stop; rc-update del tor; rc-update del i2pd' 2>/dev/null || true
    fi

    echo "=== [$init] Copying binaries to container ==="
    incus file push "${BIN_DIR}/${PROJECT_NAME}" "$container/usr/local/bin/"
    iexec "$INCUS_OP_TIMEOUT" "$container" chmod +x "/usr/local/bin/${PROJECT_NAME}"
    if [ -f "${BIN_DIR}/${PROJECT_NAME}-cli" ]; then
        incus file push "${BIN_DIR}/${PROJECT_NAME}-cli" "$container/usr/local/bin/"
        iexec "$INCUS_OP_TIMEOUT" "$container" chmod +x "/usr/local/bin/${PROJECT_NAME}-cli"
    fi

    echo "=== [$init] Running tests in Incus ==="
    remote "$container" "$init" "$REMOTE_LIB" "$REMOTE_MAIN"

    echo "=== [$init] Restarting the container (boot persistence) ==="
    incus restart "$container"
    wait_boot "$container" "$init"
    remote "$container" "$init" "$REMOTE_LIB" "$REMOTE_AFTER"
    incus delete "$container" --force
}

# One container per init system, run one after the other (never in parallel)
run_mode_list() {
    local init
    for init in $MODES; do
        run_mode "$init"
    done
}
run_mode_list

echo "Incus tests completed successfully ($MODES)"
