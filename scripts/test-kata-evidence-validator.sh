#!/usr/bin/env bash
set -euo pipefail

TMPDIR="$(mktemp -d "${TMPDIR:-/tmp}/memoh-kata-evidence-test.XXXXXX")"

cleanup() {
  rm -rf "$TMPDIR"
}

trap cleanup EXIT

write_evidence() {
  local file="$1"
  local runtime="$2"

  jq -n \
    --arg runtime "$runtime" \
    '{
      schema_version: 1,
      generated_at: "2026-06-07T00:00:00Z",
      target: {
        base_url: "http://127.0.0.1:8080",
        expected_backend: "containerd",
        expected_workspace_backend: "container",
        expected_runtime: $runtime,
        verify_containerd_runtime: true,
        ctr_command: "ctr",
        ctr_namespace: "default"
      },
      bot: {
        id: "bot-id",
        name: "kata-runtime-verify-test"
      },
      containers: {
        initial: {
          id: "workspace-bot-id",
          workspace_backend: "container",
          runtime_backend: $runtime,
          ctr_runtime: $runtime
        },
        final: {
          id: "workspace-bot-id",
          workspace_backend: "container",
          runtime_backend: $runtime,
          ctr_runtime: $runtime
        }
      },
      checks: {
        ping_status: "ok",
        ping_container_backend: "containerd",
        runtime_backend_reported: true,
        ctr_runtime_verified: true,
        resource_limit_status: "applied",
        cpu_limit_applied: true,
        memory_limit_applied: true,
        storage_soft_limit_preserved: true,
        storage_hard_limit_supported: false,
        storage_soft_limit_supported: true,
        data_preservation_checked: true,
        data_restored: true
      },
      resource_limits: {
        requested: {
          cpu_millicores: 500,
          memory_bytes: 134217728,
          storage_bytes: 33554432
        },
        initial: {
          status: "applied",
          backend: "containerd",
          workspace_backend: "container",
          runtime_backend: $runtime
        },
        update_response: {
          status: "pending_recreate",
          backend: "containerd",
          workspace_backend: "container",
          runtime_backend: $runtime
        },
        final: {
          desired: {
            cpu_millicores: 500,
            memory_bytes: 134217728,
            storage_bytes: 33554432
          },
          applied: {
            cpu_millicores: 500,
            memory_bytes: 134217728,
            storage_bytes: 0
          },
          capabilities: {
            cpu: {
              hard_limit_supported: true,
              soft_limit_supported: false
            },
            memory: {
              hard_limit_supported: true,
              soft_limit_supported: false
            },
            storage: {
              hard_limit_supported: false,
              soft_limit_supported: true
            }
          },
          observed: {
            cpu_usage_percent: 0,
            memory_usage_bytes: 1048576,
            memory_limit_bytes: 134217728,
            storage_used_bytes: 67108864,
            storage_over_soft_limit: true
          },
          status: "applied",
          requires_recreate: false,
          backend: "containerd",
          workspace_backend: "container",
          runtime_backend: $runtime
        }
      }
    }' >"$file"
}

expect_failure() {
  local message="$1"
  shift

  set +e
  "$@" >/dev/null 2>&1
  local status=$?
  set -e
  if [ "$status" -eq 0 ]; then
    echo "ERROR: expected failure: $message" >&2
    exit 1
  fi
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

require_cmd jq

KATA_EVIDENCE="$TMPDIR/kata.json"
RUNC_EVIDENCE="$TMPDIR/runc.json"
BROKEN_EVIDENCE="$TMPDIR/broken.json"
SENSITIVE_EVIDENCE="$TMPDIR/sensitive.json"

write_evidence "$KATA_EVIDENCE" "io.containerd.kata.v2"
scripts/validate-kata-evidence.sh "$KATA_EVIDENCE" >/dev/null

write_evidence "$RUNC_EVIDENCE" "io.containerd.runc.v2"
MEMOH_KATA_EVIDENCE_EXPECTED_RUNTIME=io.containerd.runc.v2 \
  scripts/validate-kata-evidence.sh "$RUNC_EVIDENCE" >/dev/null

jq '.checks.cpu_limit_applied = false' "$KATA_EVIDENCE" >"$BROKEN_EVIDENCE"
expect_failure "CPU limit evidence must be enforced" \
  scripts/validate-kata-evidence.sh "$BROKEN_EVIDENCE"

jq '.debug.password = "admin123"' "$KATA_EVIDENCE" >"$SENSITIVE_EVIDENCE"
expect_failure "sensitive evidence must be rejected" \
  scripts/validate-kata-evidence.sh "$SENSITIVE_EVIDENCE"

echo "Kata evidence validator regression passed."
