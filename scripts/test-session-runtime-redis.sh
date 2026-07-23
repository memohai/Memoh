#!/usr/bin/env bash
set -euo pipefail

container=""
cleanup() {
  if [[ -n "$container" ]]; then
    docker rm -f "$container" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

runtime_url="${MEMOH_TEST_REDIS_URL:-}"
if [[ -z "$runtime_url" ]]; then
  container="memoh-runtime-test-$$"
  docker run --detach --rm \
    --name "$container" \
    --publish 127.0.0.1::6379 \
    redis:8-alpine >/dev/null

  for _ in {1..30}; do
    if docker exec "$container" redis-cli ping 2>/dev/null | grep -q '^PONG$'; then
      break
    fi
    sleep 1
  done
  if ! docker exec "$container" redis-cli ping 2>/dev/null | grep -q '^PONG$'; then
    echo "Redis test container did not become ready" >&2
    exit 1
  fi

  port="$(docker port "$container" 6379/tcp | awk -F: 'NR == 1 { print $NF }')"
  runtime_url="redis://127.0.0.1:${port}/0"
fi

export MEMOH_TEST_DISTRIBUTED_REQUIRED=1
export MEMOH_TEST_REDIS_URL="$runtime_url"

go test -v -race -count=1 -timeout 90s \
  ./internal/sessionruntime \
  ./internal/decisionruntime \
  ./internal/handlers \
  ./cmd/agent

go test -v -race -count=1 -timeout 90s \
  -run '^TestACPWorkspaceEffectsRejectStaleRedisOwner$' \
  ./internal/acpclient
