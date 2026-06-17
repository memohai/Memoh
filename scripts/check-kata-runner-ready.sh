#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_DIR="${1:-${MEMOH_KATA_EVIDENCE_DIR:-tmp/kata-evidence}}"
CHECK_CONTAINER="${MEMOH_KATA_RUNNER_CHECK_CONTAINER:-0}"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

validate_bool() {
  case "$2" in
    true|false|0|1)
      ;;
    *)
      fail "$1 must be true, false, 0, or 1; got: $2"
      ;;
  esac
}

validate_bool MEMOH_KATA_RUNNER_CHECK_CONTAINER "$CHECK_CONTAINER"

mkdir -p "$EVIDENCE_DIR"

{
  echo "run_id=${GITHUB_RUN_ID:-local}"
  echo "run_attempt=${GITHUB_RUN_ATTEMPT:-1}"
  echo "runner_name=${RUNNER_NAME:-$(hostname 2>/dev/null || echo unknown)}"
  echo "runner_os=${RUNNER_OS:-$(uname -s 2>/dev/null || echo unknown)}"
  echo "runner_arch=${RUNNER_ARCH:-$(uname -m 2>/dev/null || echo unknown)}"
  echo "uname=$(uname -a 2>/dev/null || echo unknown)"
  if command -v docker >/dev/null 2>&1; then
    echo "docker=$(docker --version)"
    echo "docker_compose=$(docker compose version 2>/dev/null || echo missing)"
  else
    echo "docker=missing"
    echo "docker_compose=missing"
  fi
  echo "kvm_present=$([ -e /dev/kvm ] && echo true || echo false)"
  echo "kata_shim=${MEMOH_KATA_SHIM_PATH:-/opt/kata/bin/containerd-shim-kata-v2}"
} >"$EVIDENCE_DIR/environment.txt"

echo "Kata runner target:"
echo "  evidence_dir=$EVIDENCE_DIR"
echo "  runner_name=${RUNNER_NAME:-local}"
echo "  runner_os=${RUNNER_OS:-$(uname -s 2>/dev/null || echo unknown)}"
echo "  runner_arch=${RUNNER_ARCH:-$(uname -m 2>/dev/null || echo unknown)}"
echo "  check_container=$CHECK_CONTAINER"

require_cmd curl
require_cmd docker
require_cmd jq

docker info >/dev/null
docker compose version >/dev/null

scripts/check-kata-dev-env.sh

if [ "$CHECK_CONTAINER" = "1" ] || [ "$CHECK_CONTAINER" = "true" ]; then
  MEMOH_KATA_CHECK_CONTAINER=1 scripts/check-kata-dev-env.sh
fi

grep -q '^runner_os=Linux$' "$EVIDENCE_DIR/environment.txt" || fail "environment evidence does not prove this is a Linux runner"
grep -q '^kvm_present=true$' "$EVIDENCE_DIR/environment.txt" || fail "environment evidence does not prove /dev/kvm is present"

echo "Kata runner readiness passed."
