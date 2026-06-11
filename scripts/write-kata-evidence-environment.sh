#!/usr/bin/env bash
set -euo pipefail

EVIDENCE_DIR="${1:-}"
KATA_SHIM_PATH="${MEMOH_KATA_SHIM_PATH:-/opt/kata/bin/containerd-shim-kata-v2}"

if [ -z "$EVIDENCE_DIR" ]; then
  echo "usage: scripts/write-kata-evidence-environment.sh <evidence-dir>" >&2
  exit 1
fi

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
  echo "kata_shim=$KATA_SHIM_PATH"
  if [ -x "$KATA_SHIM_PATH" ]; then
    "$KATA_SHIM_PATH" --version || true
  fi
} >"$EVIDENCE_DIR/environment.txt"

echo "Wrote Kata evidence environment summary: $EVIDENCE_DIR/environment.txt"
