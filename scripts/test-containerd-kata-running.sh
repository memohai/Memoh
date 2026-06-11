#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MEMOH_VERIFY_BASE_URL:-http://127.0.0.1:${MEMOH_DEV_SERVER_PORT:-18080}}"
EXPECTED_RUNTIME="${MEMOH_VERIFY_EXPECTED_RUNTIME:-io.containerd.kata.v2}"
EVIDENCE_DIR="${MEMOH_KATA_EVIDENCE_DIR:-tmp/kata-evidence}"
EVIDENCE_FILE="${MEMOH_VERIFY_EVIDENCE_FILE:-}"
SMOKE_EVIDENCE_FILE="${MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE:-}"
COMPOSE_CMD="${MEMOH_KATA_RUNNING_COMPOSE_CMD:-docker compose -f devenv/docker-compose.yml -f devenv/docker-compose.kata.yml}"
CTR_COMMAND="${MEMOH_VERIFY_CTR_COMMAND:-$COMPOSE_CMD exec -T server ctr}"
SMOKE_CTR_COMMAND="${MEMOH_CONTAINERD_SMOKE_CTR_COMMAND:-$CTR_COMMAND}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

runtime_slug() {
  printf "%s" "$EXPECTED_RUNTIME" | tr -c "[:alnum:]" "-" | sed -e "s/^-*//" -e "s/-*$//"
}

require_cmd curl
require_cmd jq

if [ -z "$EVIDENCE_FILE" ]; then
  mkdir -p "$EVIDENCE_DIR"
  EVIDENCE_FILE="$EVIDENCE_DIR/$(runtime_slug)-running-$(date -u +%Y%m%dT%H%M%SZ)-$$.json"
fi
if [ -z "$SMOKE_EVIDENCE_FILE" ]; then
  SMOKE_EVIDENCE_FILE="${EVIDENCE_FILE%.json}.smoke.json"
fi
scripts/write-kata-evidence-environment.sh "$(dirname "$EVIDENCE_FILE")"

echo "Running containerd runtime verification against an existing Memoh stack:"
echo "  base_url=$BASE_URL"
echo "  expected_runtime=$EXPECTED_RUNTIME"
echo "  ctr_command=$CTR_COMMAND"
echo "  api_evidence=$EVIDENCE_FILE"
echo "  smoke_evidence=$SMOKE_EVIDENCE_FILE"

MEMOH_CONTAINERD_SMOKE_RUNTIME="$EXPECTED_RUNTIME" \
MEMOH_CONTAINERD_SMOKE_CTR_COMMAND="$SMOKE_CTR_COMMAND" \
MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE="$SMOKE_EVIDENCE_FILE" \
  scripts/smoke-containerd-runtime.sh
MEMOH_CONTAINERD_SMOKE_EVIDENCE_EXPECTED_RUNTIME="$EXPECTED_RUNTIME" \
  scripts/validate-containerd-smoke-evidence.sh "$SMOKE_EVIDENCE_FILE"

MEMOH_VERIFY_BASE_URL="$BASE_URL" \
MEMOH_VERIFY_EXPECTED_RUNTIME="$EXPECTED_RUNTIME" \
MEMOH_VERIFY_CONTAINERD_RUNTIME=true \
MEMOH_VERIFY_CTR_COMMAND="$CTR_COMMAND" \
MEMOH_VERIFY_EVIDENCE_FILE="$EVIDENCE_FILE" \
  scripts/verify-containerd-kata.sh
MEMOH_KATA_EVIDENCE_EXPECTED_RUNTIME="$EXPECTED_RUNTIME" \
  scripts/validate-kata-evidence.sh "$EVIDENCE_FILE"
MEMOH_KATA_EVIDENCE_EXPECTED_RUNTIME="$EXPECTED_RUNTIME" \
  scripts/validate-kata-evidence-run-dir.sh "$EVIDENCE_FILE" "$SMOKE_EVIDENCE_FILE"

echo "Containerd runtime verification passed."
echo "API evidence: $EVIDENCE_FILE"
echo "Smoke evidence: $SMOKE_EVIDENCE_FILE"
