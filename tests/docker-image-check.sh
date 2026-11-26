#!/usr/bin/env bash
# @@License : WTFPL
# Verify the required runtime tools and Tor health contract for a built image.
set -euo pipefail

PROJECT_NAME=$(basename "$(git rev-parse --show-toplevel)")
PROJECT_ORG=$(basename "$(dirname "$(git rev-parse --show-toplevel)")")
IMAGE=${1:-}
if [ -z "$IMAGE" ]; then
  echo "Usage: $0 IMAGE" >&2
  exit 64
fi

random_suffix() {
  printf '%08x' "$(((RANDOM << 15) | RANDOM))"
}

command_check="${PROJECT_NAME}-$(random_suffix)"
app_container="${PROJECT_NAME}-$(random_suffix)"
tmp_base=${TMPDIR:-/tmp}
tmp_root="$tmp_base/$PROJECT_ORG"
mkdir -p "$tmp_root"
chmod 700 "$tmp_root"
runtime_dir=$(mktemp -d "$tmp_root/$PROJECT_NAME-image-check-XXXXXX")
chmod 700 "$runtime_dir"
mkdir -p "$runtime_dir/config" "$runtime_dir/data" "$runtime_dir/database"
chmod 700 "$runtime_dir/config" "$runtime_dir/data" "$runtime_dir/database"
cleanup() {
  docker rm -f "$app_container" >/dev/null 2>&1 || true
  case "$runtime_dir" in
    "$tmp_root/$PROJECT_NAME-image-check-"*) rm -rf -- "$runtime_dir" ;;
    *) echo "Refusing to remove unexpected image-check directory: $runtime_dir" >&2; return 1 ;;
  esac
}
trap cleanup EXIT

docker run --rm --name "$command_check" --entrypoint sh "$IMAGE" -c \
  'for c in bash curl tor i2pd tini; do command -v "$c" || exit 1; done'

docker run --rm --detach --name "$app_container" \
  --publish 127.0.0.1::80 \
  -v "$runtime_dir/config:/config/search" \
  -v "$runtime_dir/data:/data/search" \
  -v "$runtime_dir/database:/data/db" \
  "$IMAGE" >/dev/null
health_port=$(docker port "$app_container" 80/tcp | sed -n 's/.*://p')
if [ -z "$health_port" ]; then
  echo "FAILED: $IMAGE did not publish its health port" >&2
  exit 1
fi
health_url="http://127.0.0.1:$health_port/api/v1/server/healthz"

fetch_health() {
  curl -q -LSsf --max-time 10 -H 'Accept: application/json' "$health_url" 2>/dev/null
}

health=''
status=''
for _ in $(seq 1 "${HEALTH_WAIT:-30}"); do
  health=$(fetch_health || true)
  if [ -n "$health" ]; then
    compact_health=$(printf '%s' "$health" | tr -d '[:space:]')
    status=$(printf '%s' "$compact_health" | sed -n 's/.*"tor":{[^}]*"status":"\([^"]*\)".*/\1/p')
    if [ -n "$status" ]; then
      break
    fi
  fi
  if ! docker inspect --format '{{.State.Running}}' "$app_container" 2>/dev/null | grep -q true; then
    docker logs "$app_container" >&2 2>/dev/null || true
    echo "FAILED: $IMAGE exited before /api/v1/server/healthz became available" >&2
    exit 1
  fi
  sleep 1
done

if [ -z "$status" ]; then
  echo "FAILED: $IMAGE did not serve a Tor status at /api/v1/server/healthz within ${HEALTH_WAIT:-30}s" >&2
  exit 1
fi

tor_wait=${TOR_WAIT:-180}
case "$status" in
  healthy|starting|unavailable) ;;
  *)
    echo "FAILED: $IMAGE reported invalid Tor status '$status'" >&2
    exit 1
    ;;
esac

if [ "$status" != healthy ]; then
  for _ in $(seq 1 "$tor_wait"); do
    sleep 1
    health=$(fetch_health || true)
    compact_health=$(printf '%s' "$health" | tr -d '[:space:]')
    status=$(printf '%s' "$compact_health" | sed -n 's/.*"tor":{[^}]*"status":"\([^"]*\)".*/\1/p')
    case "$status" in
      healthy) break ;;
      starting|unavailable) ;;
      *)
        echo "FAILED: $IMAGE reported invalid Tor status '${status:-missing}'" >&2
        exit 1
        ;;
    esac
  done
fi

if [ "$status" = healthy ]; then
  echo "✓ $IMAGE Tor hidden service is healthy"
elif [ "${REQUIRE_OVERLAY:-0}" = 1 ]; then
  echo "FAILED: $IMAGE Tor did not become healthy within ${tor_wait}s (status: $status)" >&2
  exit 1
else
  echo "SKIP: no outbound network; $IMAGE Tor status is $status" >&2
fi
