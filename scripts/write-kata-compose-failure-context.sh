#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_DIR="${1:-}"
STATUS="${2:-}"
STARTED="${3:-}"
LABEL="${4:-Kata E2E}"

usage() {
  echo "usage: scripts/write-kata-compose-failure-context.sh <evidence-dir> <status> <started> <label> -- <compose-command...>" >&2
}

if [ -z "$EVIDENCE_DIR" ] || [ -z "$STATUS" ] || [ -z "$STARTED" ]; then
  usage
  exit 1
fi
shift 4 || {
  usage
  exit 1
}
if [ "${1:-}" != "--" ]; then
  usage
  exit 1
fi
shift
if [ "$#" -eq 0 ]; then
  usage
  exit 1
fi

mkdir -p "$EVIDENCE_DIR"
context_file="$EVIDENCE_DIR/failure-context.txt"
logs_file="$EVIDENCE_DIR/compose-logs.txt"

{
  printf 'label=%s\n' "$LABEL"
  printf 'status=%s\n' "$STATUS"
  printf 'started=%s\n' "$STARTED"
  printf 'generated_at=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  printf 'compose_command='
  printf '%q ' "$@"
  printf '\n'
} >"$context_file"

case "$STARTED" in
  1|true)
    "$@" logs --no-color --tail="${MEMOH_KATA_FAILURE_LOG_TAIL:-300}" migrate server >"$logs_file" 2>&1 || true
    ;;
esac

echo "Wrote Kata failure context: $context_file"
if [ -f "$logs_file" ]; then
  echo "Wrote Kata compose logs: $logs_file"
fi
