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
          ctr_runtime: $runtime,
          created_at: "2026-06-07T00:00:00Z",
          updated_at: "2026-06-07T00:00:00Z"
        },
        final: {
          id: "workspace-bot-id",
          workspace_backend: "container",
          runtime_backend: $runtime,
          ctr_runtime: $runtime,
          created_at: "2026-06-07T00:00:02Z",
          updated_at: "2026-06-07T00:00:02Z"
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
        create_runtime_backend_reported: true,
        container_deleted_before_recreate: true,
        recreate_stream_completed: true,
        recreate_runtime_backend_reported: true,
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

write_smoke_evidence() {
  local file="$1"
  local runtime="$2"

  jq -n \
    --arg runtime "$runtime" \
    '{
      schema_version: 1,
      generated_at: "2026-06-07T00:00:00Z",
      target: {
        ctr_command: "ctr",
        namespace: "default",
        runtime: $runtime,
        image: "docker.io/library/alpine:3.22",
        snapshotter: "overlayfs",
        container_id: "memoh-runtime-smoke-test",
        pulled: true
      },
      checks: {
        ctr_reachable: true,
        image_available: true,
        runtime_started: true,
        command_output: "runtime-smoke-ok"
      }
    }' >"$file"
}

write_environment_summary() {
  local file="$1"
  local kvm_present="${2:-true}"

  cat >"$file" <<EOF
run_id=12345
run_attempt=1
runner_name=kata-runner
runner_os=Linux
runner_arch=X64
uname=Linux kata-runner 6.8.0 #1 SMP x86_64 GNU/Linux
docker=Docker version 27.0.0
docker_compose=Docker Compose version v2.29.0
kvm_present=$kvm_present
kata_shim=/opt/kata/bin/containerd-shim-kata-v2
EOF
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
KATA_SMOKE_EVIDENCE="$TMPDIR/kata-smoke.json"
RUNC_SMOKE_EVIDENCE="$TMPDIR/runc-smoke.json"
BROKEN_SMOKE_EVIDENCE="$TMPDIR/broken-smoke.json"
SENSITIVE_SMOKE_EVIDENCE="$TMPDIR/sensitive-smoke.json"
CUSTOM_SMOKE_EVIDENCE="$TMPDIR/custom-smoke.json"
VALID_EVIDENCE_DIR="$TMPDIR/evidence-dir"
STALE_EVIDENCE_DIR="$TMPDIR/stale-evidence-dir"
RUNC_NO_KVM_RUN_DIR="$TMPDIR/runc-no-kvm-run-dir"
KATA_NO_KVM_RUN_DIR="$TMPDIR/kata-no-kvm-run-dir"
FAILURE_CONTEXT_DIR="$TMPDIR/failure-context-dir"
RUNC_EVIDENCE_DIR="$TMPDIR/runc-evidence-dir"
MISSING_PAIR_EVIDENCE_DIR="$TMPDIR/missing-pair-evidence-dir"
MISMATCH_EVIDENCE_DIR="$TMPDIR/mismatch-evidence-dir"
VALID_READINESS_DIR="$TMPDIR/valid-readiness-dir"
NO_KVM_READINESS_DIR="$TMPDIR/no-kvm-readiness-dir"
NO_DOCKER_READINESS_DIR="$TMPDIR/no-docker-readiness-dir"
SENSITIVE_READINESS_DIR="$TMPDIR/sensitive-readiness-dir"

write_evidence "$KATA_EVIDENCE" "io.containerd.kata.v2"
scripts/validate-kata-evidence.sh "$KATA_EVIDENCE" >/dev/null

write_evidence "$RUNC_EVIDENCE" "io.containerd.runc.v2"
MEMOH_KATA_EVIDENCE_EXPECTED_RUNTIME=io.containerd.runc.v2 \
  scripts/validate-kata-evidence.sh "$RUNC_EVIDENCE" >/dev/null

jq '.checks.cpu_limit_applied = false' "$KATA_EVIDENCE" >"$BROKEN_EVIDENCE"
expect_failure "CPU limit evidence must be enforced" \
  scripts/validate-kata-evidence.sh "$BROKEN_EVIDENCE"

jq '.checks.container_deleted_before_recreate = false' "$KATA_EVIDENCE" >"$BROKEN_EVIDENCE"
expect_failure "delete-before-recreate evidence must be enforced" \
  scripts/validate-kata-evidence.sh "$BROKEN_EVIDENCE"

jq '.resource_limits.update_response.status = "applied"' "$KATA_EVIDENCE" >"$BROKEN_EVIDENCE"
expect_failure "resource update response must require recreate" \
  scripts/validate-kata-evidence.sh "$BROKEN_EVIDENCE"

jq '.resource_limits.update_response.runtime_backend = "io.containerd.runc.v2"' "$KATA_EVIDENCE" >"$BROKEN_EVIDENCE"
expect_failure "resource update response runtime must match target runtime" \
  scripts/validate-kata-evidence.sh "$BROKEN_EVIDENCE"

jq '.checks.create_runtime_backend_reported = false' "$KATA_EVIDENCE" >"$BROKEN_EVIDENCE"
expect_failure "create stream runtime backend evidence must be enforced" \
  scripts/validate-kata-evidence.sh "$BROKEN_EVIDENCE"

jq '.checks.recreate_runtime_backend_reported = false' "$KATA_EVIDENCE" >"$BROKEN_EVIDENCE"
expect_failure "recreate stream runtime backend evidence must be enforced" \
  scripts/validate-kata-evidence.sh "$BROKEN_EVIDENCE"

jq '.debug.password = "admin123"' "$KATA_EVIDENCE" >"$SENSITIVE_EVIDENCE"
expect_failure "sensitive evidence must be rejected" \
  scripts/validate-kata-evidence.sh "$SENSITIVE_EVIDENCE"

write_smoke_evidence "$KATA_SMOKE_EVIDENCE" "io.containerd.kata.v2"
scripts/validate-containerd-smoke-evidence.sh "$KATA_SMOKE_EVIDENCE" >/dev/null

write_smoke_evidence "$RUNC_SMOKE_EVIDENCE" "io.containerd.runc.v2"
MEMOH_CONTAINERD_SMOKE_EVIDENCE_EXPECTED_RUNTIME=io.containerd.runc.v2 \
  scripts/validate-containerd-smoke-evidence.sh "$RUNC_SMOKE_EVIDENCE" >/dev/null

jq '.target.namespace = "testing" | .target.image = "example.local/alpine:custom" | .target.snapshotter = "native"' \
  "$KATA_SMOKE_EVIDENCE" >"$CUSTOM_SMOKE_EVIDENCE"
MEMOH_CONTAINERD_SMOKE_NAMESPACE=testing \
MEMOH_CONTAINERD_SMOKE_IMAGE=example.local/alpine:custom \
MEMOH_CONTAINERD_SMOKE_SNAPSHOTTER=native \
  scripts/validate-containerd-smoke-evidence.sh "$CUSTOM_SMOKE_EVIDENCE" >/dev/null

jq '.checks.runtime_started = false' "$KATA_SMOKE_EVIDENCE" >"$BROKEN_SMOKE_EVIDENCE"
expect_failure "smoke runtime_started must be enforced" \
  scripts/validate-containerd-smoke-evidence.sh "$BROKEN_SMOKE_EVIDENCE"

jq '.debug.password = "admin123"' "$KATA_SMOKE_EVIDENCE" >"$SENSITIVE_SMOKE_EVIDENCE"
expect_failure "sensitive smoke evidence must be rejected" \
  scripts/validate-containerd-smoke-evidence.sh "$SENSITIVE_SMOKE_EVIDENCE"

mkdir -p "$VALID_READINESS_DIR"
write_environment_summary "$VALID_READINESS_DIR/environment.txt"
scripts/validate-kata-runner-readiness.sh "$VALID_READINESS_DIR" >/dev/null

mkdir -p "$NO_KVM_READINESS_DIR"
write_environment_summary "$NO_KVM_READINESS_DIR/environment.txt" false
expect_failure "runner readiness evidence must require KVM" \
  scripts/validate-kata-runner-readiness.sh "$NO_KVM_READINESS_DIR"

mkdir -p "$NO_DOCKER_READINESS_DIR"
write_environment_summary "$NO_DOCKER_READINESS_DIR/environment.txt"
sed -i.bak 's/^docker=.*/docker=missing/' "$NO_DOCKER_READINESS_DIR/environment.txt"
expect_failure "runner readiness evidence must require Docker" \
  scripts/validate-kata-runner-readiness.sh "$NO_DOCKER_READINESS_DIR"

mkdir -p "$SENSITIVE_READINESS_DIR"
write_environment_summary "$SENSITIVE_READINESS_DIR/environment.txt"
printf 'jwt_secret=devsecret\n' >>"$SENSITIVE_READINESS_DIR/environment.txt"
expect_failure "runner readiness evidence must reject sensitive text" \
  scripts/validate-kata-runner-readiness.sh "$SENSITIVE_READINESS_DIR"

mkdir -p "$VALID_EVIDENCE_DIR"
write_environment_summary "$VALID_EVIDENCE_DIR/environment.txt"
cp "$KATA_EVIDENCE" "$VALID_EVIDENCE_DIR/kata-dev.json"
cp "$KATA_SMOKE_EVIDENCE" "$VALID_EVIDENCE_DIR/kata-dev.smoke.json"
MEMOH_KATA_EVIDENCE_EXPECTED_RUNS=1 \
  scripts/validate-kata-evidence-dir.sh "$VALID_EVIDENCE_DIR" >/dev/null

cp "$KATA_EVIDENCE" "$VALID_EVIDENCE_DIR/kata-compose.json"
cp "$KATA_SMOKE_EVIDENCE" "$VALID_EVIDENCE_DIR/kata-compose.smoke.json"
MEMOH_KATA_EVIDENCE_EXPECTED_RUNS=2 \
  scripts/validate-kata-evidence-dir.sh "$VALID_EVIDENCE_DIR" >/dev/null

mkdir -p "$STALE_EVIDENCE_DIR"
write_environment_summary "$STALE_EVIDENCE_DIR/environment.txt"
cp "$KATA_EVIDENCE" "$STALE_EVIDENCE_DIR/kata-dev.json"
cp "$KATA_SMOKE_EVIDENCE" "$STALE_EVIDENCE_DIR/kata-dev.smoke.json"
cp "$RUNC_EVIDENCE" "$STALE_EVIDENCE_DIR/stale-runc.json"
cp "$RUNC_SMOKE_EVIDENCE" "$STALE_EVIDENCE_DIR/stale-runc.smoke.json"
scripts/validate-kata-evidence-run-dir.sh \
  "$STALE_EVIDENCE_DIR/kata-dev.json" \
  "$STALE_EVIDENCE_DIR/kata-dev.smoke.json" >/dev/null

mkdir -p "$RUNC_NO_KVM_RUN_DIR"
write_environment_summary "$RUNC_NO_KVM_RUN_DIR/environment.txt" false
cp "$RUNC_EVIDENCE" "$RUNC_NO_KVM_RUN_DIR/runc.json"
cp "$RUNC_SMOKE_EVIDENCE" "$RUNC_NO_KVM_RUN_DIR/runc.smoke.json"
scripts/validate-kata-evidence-run-dir.sh \
  "$RUNC_NO_KVM_RUN_DIR/runc.json" \
  "$RUNC_NO_KVM_RUN_DIR/runc.smoke.json" >/dev/null

mkdir -p "$KATA_NO_KVM_RUN_DIR"
write_environment_summary "$KATA_NO_KVM_RUN_DIR/environment.txt" false
cp "$KATA_EVIDENCE" "$KATA_NO_KVM_RUN_DIR/kata-dev.json"
cp "$KATA_SMOKE_EVIDENCE" "$KATA_NO_KVM_RUN_DIR/kata-dev.smoke.json"
expect_failure "kata run bundle must require KVM evidence" \
  scripts/validate-kata-evidence-run-dir.sh \
  "$KATA_NO_KVM_RUN_DIR/kata-dev.json" \
  "$KATA_NO_KVM_RUN_DIR/kata-dev.smoke.json"

fake_compose="$TMPDIR/fake-compose.sh"
cat >"$fake_compose" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'fake compose command:'
printf ' %s' "$@"
printf '\n'
printf 'password=admin123 jwt_secret=devsecret Authorization: Bearer abc.def.ghi\n'
EOF
chmod +x "$fake_compose"
scripts/write-kata-compose-failure-context.sh \
  "$FAILURE_CONTEXT_DIR" \
  42 \
  1 \
  "Test Kata failure" \
  -- \
  "$fake_compose" >/dev/null
grep -q '^label=Test Kata failure$' "$FAILURE_CONTEXT_DIR/failure-context.txt" || { echo "ERROR: failure context missing label" >&2; exit 1; }
grep -q '^status=42$' "$FAILURE_CONTEXT_DIR/failure-context.txt" || { echo "ERROR: failure context missing status" >&2; exit 1; }
grep -q '^started=1$' "$FAILURE_CONTEXT_DIR/failure-context.txt" || { echo "ERROR: failure context missing started" >&2; exit 1; }
grep -q 'fake compose command: logs --no-color --tail=300 migrate server' "$FAILURE_CONTEXT_DIR/compose-logs.txt" || { echo "ERROR: failure context missing compose logs" >&2; exit 1; }
grep -q 'password=\[redacted\]' "$FAILURE_CONTEXT_DIR/compose-logs.txt" || { echo "ERROR: failure logs did not redact password" >&2; exit 1; }
grep -q 'jwt_secret=\[redacted\]' "$FAILURE_CONTEXT_DIR/compose-logs.txt" || { echo "ERROR: failure logs did not redact jwt_secret" >&2; exit 1; }
grep -q 'Authorization: Bearer \[redacted\]' "$FAILURE_CONTEXT_DIR/compose-logs.txt" || { echo "ERROR: failure logs did not redact bearer token" >&2; exit 1; }
if grep -Eq '(admin123|devsecret|abc\.def\.ghi)' "$FAILURE_CONTEXT_DIR/compose-logs.txt"; then
  echo "ERROR: failure logs contain unredacted sensitive text" >&2
  exit 1
fi

mkdir -p "$RUNC_EVIDENCE_DIR"
write_environment_summary "$RUNC_EVIDENCE_DIR/environment.txt"
cp "$RUNC_EVIDENCE" "$RUNC_EVIDENCE_DIR/runc.json"
cp "$RUNC_SMOKE_EVIDENCE" "$RUNC_EVIDENCE_DIR/runc.smoke.json"
MEMOH_KATA_EVIDENCE_EXPECTED_RUNS=1 \
  scripts/validate-kata-evidence-dir.sh "$RUNC_EVIDENCE_DIR" >/dev/null

mkdir -p "$MISSING_PAIR_EVIDENCE_DIR"
write_environment_summary "$MISSING_PAIR_EVIDENCE_DIR/environment.txt"
cp "$KATA_EVIDENCE" "$MISSING_PAIR_EVIDENCE_DIR/kata-dev.json"
expect_failure "directory evidence must require paired smoke evidence" \
  scripts/validate-kata-evidence-dir.sh "$MISSING_PAIR_EVIDENCE_DIR"

mkdir -p "$MISMATCH_EVIDENCE_DIR"
write_environment_summary "$MISMATCH_EVIDENCE_DIR/environment.txt"
cp "$KATA_EVIDENCE" "$MISMATCH_EVIDENCE_DIR/kata-dev.json"
cp "$RUNC_SMOKE_EVIDENCE" "$MISMATCH_EVIDENCE_DIR/kata-dev.smoke.json"
expect_failure "directory evidence must reject runtime mismatches" \
  scripts/validate-kata-evidence-dir.sh "$MISMATCH_EVIDENCE_DIR"

echo "Kata evidence validator regression passed."
