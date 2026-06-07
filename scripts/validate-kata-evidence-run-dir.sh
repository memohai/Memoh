#!/usr/bin/env bash
set -euo pipefail

API_EVIDENCE_FILE="${1:-}"
SMOKE_EVIDENCE_FILE="${2:-}"
EXPECT_KVM="${MEMOH_KATA_EVIDENCE_EXPECT_KVM:-}"
tmpdir=""

usage() {
  echo "usage: scripts/validate-kata-evidence-run-dir.sh <api-evidence.json> <smoke-evidence.json>" >&2
}

if [ -z "$API_EVIDENCE_FILE" ] || [ -z "$SMOKE_EVIDENCE_FILE" ]; then
  usage
  exit 1
fi

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

cleanup() {
  local status=$?
  if [ -n "$tmpdir" ]; then
    rm -rf "$tmpdir"
  fi
  exit "$status"
}

require_cmd jq
[ -f "$API_EVIDENCE_FILE" ] || { echo "ERROR: API evidence file not found: $API_EVIDENCE_FILE" >&2; exit 1; }
[ -f "$SMOKE_EVIDENCE_FILE" ] || { echo "ERROR: smoke evidence file not found: $SMOKE_EVIDENCE_FILE" >&2; exit 1; }

api_name="$(basename "$API_EVIDENCE_FILE")"
case "$api_name" in
  *.json)
    ;;
  *)
    echo "ERROR: API evidence file must end with .json: $API_EVIDENCE_FILE" >&2
    exit 1
    ;;
esac

source_dir="$(dirname "$API_EVIDENCE_FILE")"
tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/memoh-kata-evidence-run.XXXXXX")"
trap cleanup EXIT

if [ -z "$EXPECT_KVM" ]; then
  api_runtime="$(jq -er '.target.expected_runtime' "$API_EVIDENCE_FILE")"
  case "$api_runtime" in
    *kata*)
      EXPECT_KVM=true
      ;;
    *)
      EXPECT_KVM=false
      ;;
  esac
fi

cp "$API_EVIDENCE_FILE" "$tmpdir/$api_name"
cp "$SMOKE_EVIDENCE_FILE" "$tmpdir/${api_name%.json}.smoke.json"
if [ -f "$source_dir/environment.txt" ]; then
  cp "$source_dir/environment.txt" "$tmpdir/environment.txt"
fi

MEMOH_KATA_EVIDENCE_EXPECT_KVM="$EXPECT_KVM" \
MEMOH_KATA_EVIDENCE_EXPECTED_RUNS=1 \
  scripts/validate-kata-evidence-dir.sh "$tmpdir"
