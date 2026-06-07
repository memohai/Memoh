#!/usr/bin/env bash
set -euo pipefail

CTR_COMMAND="${MEMOH_CONTAINERD_SMOKE_CTR_COMMAND:-ctr}"
NAMESPACE="${MEMOH_CONTAINERD_SMOKE_NAMESPACE:-default}"
RUNTIME="${MEMOH_CONTAINERD_SMOKE_RUNTIME:-io.containerd.kata.v2}"
IMAGE="${MEMOH_CONTAINERD_SMOKE_IMAGE:-docker.io/library/alpine:3.22}"
SNAPSHOTTER="${MEMOH_CONTAINERD_SMOKE_SNAPSHOTTER:-overlayfs}"
PULL_IMAGE="${MEMOH_CONTAINERD_SMOKE_PULL:-true}"
CONTAINER_ID="${MEMOH_CONTAINERD_SMOKE_ID:-memoh-runtime-smoke-$$}"

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

run_ctr() {
  local cmd="$CTR_COMMAND -n $(quote_shell "$NAMESPACE")"
  for arg in "$@"; do
    cmd="$cmd $(quote_shell "$arg")"
  done
  echo "+ $cmd" >&2
  bash -lc "$cmd"
}

cleanup() {
  run_ctr tasks kill "$CONTAINER_ID" >/dev/null 2>&1 || true
  run_ctr tasks rm "$CONTAINER_ID" >/dev/null 2>&1 || true
  run_ctr containers rm "$CONTAINER_ID" >/dev/null 2>&1 || true
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
  if ! run_ctr images info "$IMAGE" >/dev/null 2>&1; then
    pull_log="$(mktemp "${TMPDIR:-/tmp}/memoh-runtime-smoke-pull.XXXXXX")"
    if ! run_ctr images pull --snapshotter "$SNAPSHOTTER" "$IMAGE" >"$pull_log" 2>&1; then
      cat "$pull_log" >&2
      rm -f "$pull_log"
      exit 1
    fi
    rm -f "$pull_log"
  fi
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

echo "Verified containerd runtime $RUNTIME can start $IMAGE."
