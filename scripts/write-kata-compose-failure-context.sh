#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_DIR="${1:-}"
STATUS="${2:-}"
STARTED="${3:-}"
LABEL="${4:-Kata E2E}"

usage() {
  echo "usage: scripts/write-kata-compose-failure-context.sh <evidence-dir> <status> <started> <label> -- <compose-command...>" >&2
}

redact_sensitive() {
  sed -E \
    -e 's/([Pp]assword[[:space:]]*[:=][[:space:]]*)[^[:space:],;]+/\1[redacted]/g' \
    -e 's/([Jj][Ww][Tt][-_ ]?[Ss]ecret[[:space:]]*[:=][[:space:]]*)[^[:space:],;]+/\1[redacted]/g' \
    -e 's/([Aa]uthorization[[:space:]]*[:=][[:space:]]*)[Bb]earer[[:space:]]+[A-Za-z0-9._-]+/\1Bearer [redacted]/g' \
    -e 's/[Bb]earer[[:space:]]+[A-Za-z0-9._-]+/Bearer [redacted]/g'
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
logs_tmp="$logs_file.tmp"

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
    "$@" logs --no-color --tail="${MEMOH_KATA_FAILURE_LOG_TAIL:-300}" migrate server >"$logs_tmp" 2>&1 || true
    redact_sensitive <"$logs_tmp" >"$logs_file"
    rm -f "$logs_tmp"
    ;;
esac

echo "Wrote Kata failure context: $context_file"
if [ -f "$logs_file" ]; then
  echo "Wrote Kata compose logs: $logs_file"
fi
