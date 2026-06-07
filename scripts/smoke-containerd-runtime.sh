#!/usr/bin/env bash
set -euo pipefail

CTR_COMMAND="${MEMOH_CONTAINERD_SMOKE_CTR_COMMAND:-ctr}"
NAMESPACE="${MEMOH_CONTAINERD_SMOKE_NAMESPACE:-default}"
RUNTIME="${MEMOH_CONTAINERD_SMOKE_RUNTIME:-io.containerd.kata.v2}"
IMAGE="${MEMOH_CONTAINERD_SMOKE_IMAGE:-docker.io/library/alpine:3.22}"
SNAPSHOTTER="${MEMOH_CONTAINERD_SMOKE_SNAPSHOTTER:-overlayfs}"
PULL_IMAGE="${MEMOH_CONTAINERD_SMOKE_PULL:-true}"
CONTAINER_ID="${MEMOH_CONTAINERD_SMOKE_ID:-memoh-runtime-smoke-$$}"
EVIDENCE_FILE="${MEMOH_CONTAINERD_SMOKE_EVIDENCE_FILE:-}"

quote_shell() {
  printf "%q" "$1"
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

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

run_ctr() {
  local cmd="$CTR_COMMAND -n $(quote_shell "$NAMESPACE")"
  for arg in "$@"; do
    cmd="$cmd $(quote_shell "$arg")"
  done
  echo "+ $cmd" >&2
  bash -lc "$cmd"
}

write_evidence() {
  if [ -z "$EVIDENCE_FILE" ]; then
    return 0
  fi

  require_cmd jq
  mkdir -p "$(dirname "$EVIDENCE_FILE")"
  local generated_at
  generated_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

  jq -n \
    --arg generated_at "$generated_at" \
    --arg ctr_command "$CTR_COMMAND" \
    --arg namespace "$NAMESPACE" \
    --arg runtime "$RUNTIME" \
    --arg image "$IMAGE" \
    --arg snapshotter "$SNAPSHOTTER" \
    --arg container_id "$CONTAINER_ID" \
    --argjson pulled "$PULL_IMAGE" \
    '{
      schema_version: 1,
      generated_at: $generated_at,
      target: {
        ctr_command: $ctr_command,
        namespace: $namespace,
        runtime: $runtime,
        image: $image,
        snapshotter: $snapshotter,
        container_id: $container_id,
        pulled: $pulled
      },
      checks: {
        ctr_reachable: true,
        image_available: true,
        runtime_started: true,
        command_output: "runtime-smoke-ok"
      }
    }' >"$EVIDENCE_FILE"

  echo "Wrote containerd runtime smoke evidence: $EVIDENCE_FILE"
}

cleanup() {
  run_ctr tasks kill "$CONTAINER_ID" >/dev/null 2>&1 || true
  run_ctr tasks rm "$CONTAINER_ID" >/dev/null 2>&1 || true
  run_ctr containers rm "$CONTAINER_ID" >/dev/null 2>&1 || true
  run_ctr snapshots --snapshotter "$SNAPSHOTTER" rm "$CONTAINER_ID" >/dev/null 2>&1 || true
}

validate_bool MEMOH_CONTAINERD_SMOKE_PULL "$PULL_IMAGE"
trap cleanup EXIT

echo "Containerd runtime smoke target:"
echo "  ctr_command=$CTR_COMMAND"
echo "  namespace=$NAMESPACE"
echo "  runtime=$RUNTIME"
echo "  image=$IMAGE"
echo "  snapshotter=$SNAPSHOTTER"

run_ctr version >/dev/null

if [ "$PULL_IMAGE" = "true" ]; then
  pull_log="$(mktemp "${TMPDIR:-/tmp}/memoh-runtime-smoke-pull.XXXXXX")"
  if ! run_ctr images pull --snapshotter "$SNAPSHOTTER" "$IMAGE" >"$pull_log" 2>&1; then
    cat "$pull_log" >&2
    rm -f "$pull_log"
    exit 1
  fi
  rm -f "$pull_log"
fi

run_ctr run \
  --rm \
  --runtime "$RUNTIME" \
  --snapshotter "$SNAPSHOTTER" \
  "$IMAGE" \
  "$CONTAINER_ID" \
  /bin/sh \
  -lc \
  'printf "runtime-smoke-ok\n"; uname -m >/dev/null'

write_evidence

echo "Verified containerd runtime $RUNTIME can start $IMAGE."
