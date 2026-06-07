#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_FILE="${1:-}"
EXPECTED_RUNTIME="${MEMOH_CONTAINERD_SMOKE_EVIDENCE_EXPECTED_RUNTIME:-${MEMOH_CONTAINERD_SMOKE_RUNTIME:-io.containerd.kata.v2}}"
EXPECTED_NAMESPACE="${MEMOH_CONTAINERD_SMOKE_EVIDENCE_EXPECTED_NAMESPACE:-${MEMOH_CONTAINERD_SMOKE_NAMESPACE:-default}}"
EXPECTED_IMAGE="${MEMOH_CONTAINERD_SMOKE_EVIDENCE_EXPECTED_IMAGE:-${MEMOH_CONTAINERD_SMOKE_IMAGE:-docker.io/library/alpine:3.22}}"
EXPECTED_SNAPSHOTTER="${MEMOH_CONTAINERD_SMOKE_EVIDENCE_EXPECTED_SNAPSHOTTER:-${MEMOH_CONTAINERD_SMOKE_SNAPSHOTTER:-overlayfs}}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

assert_evidence() {
  local filter="$1"
  local message="$2"
  if ! jq -e "$filter" "$EVIDENCE_FILE" >/dev/null; then
    echo "ERROR: $message" >&2
    echo "Smoke evidence summary:" >&2
    jq '{schema_version, target, checks}' "$EVIDENCE_FILE" >&2 || cat "$EVIDENCE_FILE" >&2
    exit 1
  fi
}

if [ -z "$EVIDENCE_FILE" ]; then
  echo "usage: scripts/validate-containerd-smoke-evidence.sh <smoke-evidence.json>" >&2
  exit 1
fi

require_cmd jq

[ -f "$EVIDENCE_FILE" ] || { echo "ERROR: smoke evidence file not found: $EVIDENCE_FILE" >&2; exit 1; }

assert_evidence ".schema_version == 1" "smoke evidence schema_version must be 1"
assert_evidence ".target.runtime == \"$EXPECTED_RUNTIME\"" "smoke runtime must be $EXPECTED_RUNTIME"
assert_evidence ".target.namespace == \"$EXPECTED_NAMESPACE\"" "smoke namespace must be $EXPECTED_NAMESPACE"
assert_evidence ".target.image == \"$EXPECTED_IMAGE\"" "smoke image must be $EXPECTED_IMAGE"
assert_evidence ".target.snapshotter == \"$EXPECTED_SNAPSHOTTER\"" "smoke snapshotter must be $EXPECTED_SNAPSHOTTER"
assert_evidence ".target.container_id | length > 0" "smoke container_id must be present"
assert_evidence ".checks.ctr_reachable == true" "ctr must be reachable"
assert_evidence ".checks.image_available == true" "smoke image must be available"
assert_evidence ".checks.runtime_started == true" "smoke runtime must start"
assert_evidence ".checks.command_output == \"runtime-smoke-ok\"" "smoke command output mismatch"

if jq -e '.. | objects | keys[]? | test("(?i)(access_token|authorization|bearer|password|secret|jwt)")' "$EVIDENCE_FILE" >/dev/null; then
  echo "ERROR: smoke evidence contains sensitive-looking key names" >&2
  exit 1
fi
if jq -e '.. | strings | test("(?i)(access_token|authorization: bearer|bearer [A-Za-z0-9._-]+|admin123)")' "$EVIDENCE_FILE" >/dev/null; then
  echo "ERROR: smoke evidence contains sensitive-looking values" >&2
  exit 1
fi

echo "Containerd smoke evidence validated:"
jq '{runtime: .target.runtime, image: .target.image, snapshotter: .target.snapshotter, checks: .checks}' "$EVIDENCE_FILE"
