#!/usr/bin/env bash
# @@License : WTFPL
# Per AI.md PART 28 "Browser E2E Testing": Docker-wrapped entry point for the
# browser end-to-end suite (all three tiers). Standalone, developer-initiated
# (beta testing, pre-release, after major frontend work) — NOT invoked by
# run_tests.sh and NOT part of the commit gate. No Makefile target exists for
# it; the six-target set (PART 25) is fixed.
#
# The suite runs `go test -tags e2e ./tests/e2e/...` inside a casjaysdev/go
# container and drives Chromium through a chromedp/headless-shell sidecar:
#   - Tier 1 (SSR, net/http): needs no browser, runs regardless.
#   - Tiers 2/3 (chromedp): attach to the sidecar over CDP via CHROMEDP_WS.
# Chromium in the test container itself is not required — the browser lives in
# the sidecar image and the tests talk to it over the shared Docker network.
set -euo pipefail

PROJECTNAME=$(basename "$PWD")
PROJECTORG=$(basename "$(dirname "$PWD")")
random_suffix() {
  printf '%08x' "$(((RANDOM << 15) | RANDOM))"
}
NET="${PROJECTNAME}-e2e-$$-$(random_suffix)"
SIDECAR="${PROJECTNAME}-$(random_suffix)"
TEST_CONTAINER="${PROJECTNAME}-$(random_suffix)"

# Org-structured temp dir per AI.md PART 29 for the suite's ephemeral output.
mkdir -p "${TMPDIR:-/tmp}/${PROJECTORG}"
E2E_TMP=$(mktemp -d "${TMPDIR:-/tmp}/${PROJECTORG}/${PROJECTNAME}-e2e-XXXXXX")
trap 'docker rm -f "$SIDECAR" >/dev/null 2>&1 || true; docker network rm "$NET" >/dev/null 2>&1 || true; rm -rf "$E2E_TMP"' EXIT

# Resolve Go modules before creating the isolated network. The test process
# itself runs with GOPROXY=off and cannot reach external hosts.
docker run --rm --name "${PROJECTNAME}-e2e-modules-$(random_suffix)" \
  -v "$PWD:/app" \
  -v "${HOME}/go/pkg/mod:/usr/local/share/go/pkg/mod" \
  -w /app \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  casjaysdev/go:latest go mod download

# Sidecar: the internal-only network prevents both Chromium and the test
# process from reaching public hosts. The test joins the sidecar namespace, so
# CDP remains available over loopback without publishing a host port.
docker network create --internal "$NET" >/dev/null

echo "Starting chromedp/headless-shell sidecar..."
docker run --rm --name "$SIDECAR" --network "$NET" \
  chromedp/headless-shell:latest >/dev/null 2>&1 &
SIDECAR_PID=$!

# Wait until Docker can attach a probe to the sidecar namespace.
for _ in $(seq 1 30); do
  if [ "$(docker inspect --format '{{.State.Running}}' "$SIDECAR" 2>/dev/null || true)" = true ]; then
    break
  fi
  sleep 0.5
done

# Probe from a temporary container in the sidecar namespace because the
# headless-shell image intentionally has no shell or diagnostic utilities.
if ! docker run --rm --name "${PROJECTNAME}-$(random_suffix)" --network "container:${SIDECAR}" alpine:latest sh -c '
  for _ in $(seq 1 30); do
    wget -q -O /dev/null http://127.0.0.1:9222/json/version && exit 0
    sleep 0.5
  done
  exit 1
'; then
  echo "FAILED: headless-shell sidecar did not come up on port 9222"
  exit 1
fi

# Chrome rejects non-local Host headers, so the test container shares the
# sidecar namespace and reaches CDP through loopback.
export CHROMEDP_WS="ws://127.0.0.1:9222"

echo "Running browser E2E suite (Tiers 1-3) in Docker..."
EXIT=0
docker run --rm --name "$TEST_CONTAINER" \
  --network "container:${SIDECAR}" \
  -v "$PWD:/app" \
  -v "${HOME}/go/pkg/mod:/usr/local/share/go/pkg/mod" \
  -v "${HOME}/.cache/go-build/${PROJECTNAME}:/usr/local/share/go/cache" \
  -v "${E2E_TMP}:/out" \
  -w /app \
  -e CGO_ENABLED=0 \
  -e GOFLAGS=-buildvcs=false \
  -e GOPROXY=off \
  -e CHROMEDP_WS \
  casjaysdev/go:latest \
  go test -tags e2e -count=1 -v "$@" ./tests/e2e/... || EXIT=$?

kill "$SIDECAR_PID" 2>/dev/null || true
wait "$SIDECAR_PID" 2>/dev/null || true

if [ "$EXIT" -ne 0 ]; then
  echo "FAILED: browser E2E suite reported failures (see output above)"
fi
exit "$EXIT"
