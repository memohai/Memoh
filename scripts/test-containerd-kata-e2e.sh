#!/usr/bin/env bash
set -euo pipefail

KEEP="${MEMOH_KATA_E2E_KEEP:-false}"
BASE_URL="${MEMOH_VERIFY_BASE_URL:-http://127.0.0.1:${MEMOH_DEV_SERVER_PORT:-18080}}"
EVIDENCE_DIR="${MEMOH_KATA_EVIDENCE_DIR:-tmp/kata-evidence}"
EVIDENCE_FILE="${MEMOH_VERIFY_EVIDENCE_FILE:-}"
SMOKE_EVIDENCE_FILE="${MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE:-}"
COMPOSE_CMD="docker compose -f devenv/docker-compose.yml -f devenv/docker-compose.kata.yml"
COMPOSE=(docker compose -f devenv/docker-compose.yml -f devenv/docker-compose.kata.yml)
STARTED=0

validate_bool() {
  case "$2" in
    true|false)
      ;;
    *)
      echo "ERROR: $1 must be true or false, got: $2" >&2
      exit 1
      ;;
  esac
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

dump_logs() {
  echo "Kata E2E failed; recent compose logs:" >&2
  "${COMPOSE[@]}" logs --no-color --tail=200 migrate server >&2 || true
}

cleanup() {
  if [ "$KEEP" = "true" ]; then
    echo "Keeping Kata E2E compose environment because MEMOH_KATA_E2E_KEEP=true."
    return
  fi
  "${COMPOSE[@]}" down --remove-orphans >/dev/null 2>&1 || true
}

on_exit() {
  local status=$?
  if [ "$status" -ne 0 ] && [ "$STARTED" = "1" ]; then
    dump_logs
  fi
  cleanup
  exit "$status"
}

wait_server_ready() {
  echo "Waiting for Memoh server at $BASE_URL..."
  for _ in $(seq 1 120); do
    if curl -fsSI "$BASE_URL/health" >/dev/null; then
      return
    fi
    sleep 1
  done
  echo "ERROR: Memoh server did not become healthy at $BASE_URL." >&2
  exit 1
}

validate_bool MEMOH_KATA_E2E_KEEP "$KEEP"
require_cmd curl
require_cmd docker
docker compose version >/dev/null
if [ -z "$EVIDENCE_FILE" ]; then
  mkdir -p "$EVIDENCE_DIR"
  EVIDENCE_FILE="$EVIDENCE_DIR/kata-dev-$(date -u +%Y%m%dT%H%M%SZ)-$$.json"
fi
if [ -z "$SMOKE_EVIDENCE_FILE" ]; then
  SMOKE_EVIDENCE_FILE="${EVIDENCE_FILE%.json}.smoke.json"
fi

trap on_exit EXIT

scripts/check-kata-dev-env.sh
"${COMPOSE[@]}" build migrate server
MEMOH_KATA_CHECK_CONTAINER=1 scripts/check-kata-dev-env.sh
"${COMPOSE[@]}" up -d --build server
STARTED=1
wait_server_ready

MEMOH_CONTAINERD_SMOKE_CTR_COMMAND="$COMPOSE_CMD exec -T server ctr" \
MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE="$SMOKE_EVIDENCE_FILE" \
  scripts/smoke-containerd-runtime.sh
scripts/validate-containerd-smoke-evidence.sh "$SMOKE_EVIDENCE_FILE"

MEMOH_VERIFY_BASE_URL="$BASE_URL" \
MEMOH_VERIFY_CONTAINERD_RUNTIME=true \
MEMOH_VERIFY_CTR_COMMAND="$COMPOSE_CMD exec -T server ctr" \
MEMOH_VERIFY_EVIDENCE_FILE="$EVIDENCE_FILE" \
  scripts/verify-containerd-kata.sh
scripts/validate-kata-evidence.sh "$EVIDENCE_FILE"

echo "Kata E2E verification passed."
echo "Kata E2E evidence: $EVIDENCE_FILE"
echo "Kata E2E smoke evidence: $SMOKE_EVIDENCE_FILE"
