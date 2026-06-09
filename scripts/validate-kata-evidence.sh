#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_FILE="${1:-}"
EXPECTED_RUNTIME="${MEMOH_KATA_EVIDENCE_EXPECTED_RUNTIME:-io.containerd.kata.v2}"
EXPECTED_BACKEND="${MEMOH_KATA_EVIDENCE_EXPECTED_BACKEND:-containerd}"
EXPECTED_WORKSPACE_BACKEND="${MEMOH_KATA_EVIDENCE_EXPECTED_WORKSPACE_BACKEND:-container}"
EXPECTED_STORAGE_HARD_LIMIT="${MEMOH_KATA_EVIDENCE_EXPECT_STORAGE_HARD_LIMIT:-false}"
EXPECTED_STORAGE_SOFT_LIMIT="${MEMOH_KATA_EVIDENCE_EXPECT_STORAGE_SOFT_LIMIT:-true}"
EXPECTED_DATA_RESTORED="${MEMOH_KATA_EVIDENCE_EXPECT_DATA_RESTORED:-true}"
EXPECTED_CTR_RUNTIME="${MEMOH_KATA_EVIDENCE_EXPECT_CTR_RUNTIME:-true}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

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

assert_evidence() {
  local filter="$1"
  local message="$2"
  if ! jq -e "$filter" "$EVIDENCE_FILE" >/dev/null; then
    echo "ERROR: $message" >&2
    echo "Evidence summary:" >&2
    jq '{schema_version, target, containers, checks}' "$EVIDENCE_FILE" >&2 || cat "$EVIDENCE_FILE" >&2
    exit 1
  fi
}

if [ -z "$EVIDENCE_FILE" ]; then
  echo "usage: scripts/validate-kata-evidence.sh <evidence.json>" >&2
  exit 1
fi

require_cmd jq
validate_bool MEMOH_KATA_EVIDENCE_EXPECT_STORAGE_HARD_LIMIT "$EXPECTED_STORAGE_HARD_LIMIT"
validate_bool MEMOH_KATA_EVIDENCE_EXPECT_STORAGE_SOFT_LIMIT "$EXPECTED_STORAGE_SOFT_LIMIT"
validate_bool MEMOH_KATA_EVIDENCE_EXPECT_DATA_RESTORED "$EXPECTED_DATA_RESTORED"
validate_bool MEMOH_KATA_EVIDENCE_EXPECT_CTR_RUNTIME "$EXPECTED_CTR_RUNTIME"

[ -f "$EVIDENCE_FILE" ] || { echo "ERROR: evidence file not found: $EVIDENCE_FILE" >&2; exit 1; }

assert_evidence ".schema_version == 1" "evidence schema_version must be 1"
assert_evidence ".target.expected_backend == \"$EXPECTED_BACKEND\"" "expected backend must be $EXPECTED_BACKEND"
assert_evidence ".target.expected_workspace_backend == \"$EXPECTED_WORKSPACE_BACKEND\"" "expected workspace backend must be $EXPECTED_WORKSPACE_BACKEND"
assert_evidence ".target.expected_runtime == \"$EXPECTED_RUNTIME\"" "expected runtime must be $EXPECTED_RUNTIME"
assert_evidence ".checks.ping_status == \"ok\"" "ping status must be ok"
assert_evidence ".checks.ping_container_backend == \"$EXPECTED_BACKEND\"" "ping container backend must be $EXPECTED_BACKEND"
assert_evidence ".checks.runtime_backend_reported == true" "runtime backend must be reported by the API"
assert_evidence ".containers.initial.workspace_backend == \"$EXPECTED_WORKSPACE_BACKEND\"" "initial workspace backend mismatch"
assert_evidence ".containers.final.workspace_backend == \"$EXPECTED_WORKSPACE_BACKEND\"" "final workspace backend mismatch"
assert_evidence ".containers.initial.runtime_backend == \"$EXPECTED_RUNTIME\"" "initial runtime backend mismatch"
assert_evidence ".containers.final.runtime_backend == \"$EXPECTED_RUNTIME\"" "final runtime backend mismatch"
assert_evidence ".checks.resource_limit_status == \"applied\"" "resource limit status must be applied"
assert_evidence ".resource_limits.final.status == \"applied\"" "final resource limit status must be applied"
assert_evidence ".resource_limits.final.backend == \"$EXPECTED_BACKEND\"" "final resource limit backend mismatch"
assert_evidence ".resource_limits.final.workspace_backend == \"$EXPECTED_WORKSPACE_BACKEND\"" "final resource limit workspace backend mismatch"
assert_evidence ".resource_limits.final.runtime_backend == \"$EXPECTED_RUNTIME\"" "final resource limit runtime backend mismatch"
assert_evidence ".resource_limits.update_response.status == \"pending_recreate\"" "resource limit update response must require recreate"
assert_evidence ".resource_limits.update_response.backend == \"$EXPECTED_BACKEND\"" "update response resource limit backend mismatch"
assert_evidence ".resource_limits.update_response.workspace_backend == \"$EXPECTED_WORKSPACE_BACKEND\"" "update response resource limit workspace backend mismatch"
assert_evidence ".resource_limits.update_response.runtime_backend == \"$EXPECTED_RUNTIME\"" "update response resource limit runtime backend mismatch"
assert_evidence ".resource_limits.requested.cpu_millicores > 0" "requested CPU limit must be non-zero"
assert_evidence ".resource_limits.requested.memory_bytes > 0" "requested memory limit must be non-zero"
assert_evidence ".resource_limits.requested.storage_bytes > 0" "requested storage limit must be non-zero"
assert_evidence ".checks.cpu_limit_applied == true" "CPU limit must be applied"
assert_evidence ".checks.memory_limit_applied == true" "memory limit must be applied"
assert_evidence ".checks.storage_soft_limit_preserved == true" "storage soft limit must be preserved"
assert_evidence ".checks.storage_hard_limit_supported == $EXPECTED_STORAGE_HARD_LIMIT" "storage hard-limit support must be $EXPECTED_STORAGE_HARD_LIMIT"
assert_evidence ".checks.storage_soft_limit_supported == $EXPECTED_STORAGE_SOFT_LIMIT" "storage soft-limit support must be $EXPECTED_STORAGE_SOFT_LIMIT"
assert_evidence ".checks.create_runtime_backend_reported == true" "bot create stream must report runtime backend"
assert_evidence ".checks.container_deleted_before_recreate == true" "container must be deleted before recreate"
assert_evidence ".checks.recreate_stream_completed == true" "container recreate stream must complete"
assert_evidence ".checks.recreate_runtime_backend_reported == true" "container recreate stream must report runtime backend"

if [ "$EXPECTED_CTR_RUNTIME" = "true" ]; then
  assert_evidence ".target.verify_containerd_runtime == true" "containerd runtime verification must be enabled"
  assert_evidence ".checks.ctr_runtime_verified == true" "ctr runtime verification must pass"
  assert_evidence ".containers.initial.ctr_runtime == \"$EXPECTED_RUNTIME\"" "initial ctr runtime mismatch"
  assert_evidence ".containers.final.ctr_runtime == \"$EXPECTED_RUNTIME\"" "final ctr runtime mismatch"
fi

if [ "$EXPECTED_DATA_RESTORED" = "true" ]; then
  assert_evidence ".checks.data_preservation_checked == true" "data preservation must be checked"
  assert_evidence ".checks.data_restored == true" "data restore must pass"
fi

if jq -e '.. | objects | keys[]? | test("(?i)(access_token|authorization|bearer|password|secret|jwt)")' "$EVIDENCE_FILE" >/dev/null; then
  echo "ERROR: evidence contains sensitive-looking key names" >&2
  exit 1
fi
if jq -e '.. | strings | test("(?i)(access_token|authorization: bearer|bearer [A-Za-z0-9._-]+|admin123)")' "$EVIDENCE_FILE" >/dev/null; then
  echo "ERROR: evidence contains sensitive-looking values" >&2
  exit 1
fi

echo "Kata evidence validated:"
jq '{runtime: .target.expected_runtime, initial_container: .containers.initial.id, final_container: .containers.final.id, checks: .checks}' "$EVIDENCE_FILE"
