#!/usr/bin/env bash
set -euo pipefail

KATA_SHIM_PATH="${MEMOH_KATA_SHIM_PATH:-/opt/kata/bin/containerd-shim-kata-v2}"
KATA_CONFIG_DIR="${MEMOH_KATA_CONFIG_DIR:-/etc/kata-containers}"
KATA_SHARE_DIR="${MEMOH_KATA_SHARE_DIR:-/usr/share/kata-containers}"
KATA_OPT_DIR="${MEMOH_KATA_OPT_DIR:-/opt/kata}"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

warn() {
  echo "WARN: $*" >&2
}

[ "$(uname -s)" = "Linux" ] || fail "Kata dev validation requires a Linux host with KVM."
[ -e /dev/kvm ] || fail "/dev/kvm is missing. Enable KVM or run on a VM/bare-metal host with nested virtualization."
[ -r /dev/kvm ] || warn "/dev/kvm exists but is not readable by the current user. Docker may still pass it through when privileged."
[ -f "$KATA_SHIM_PATH" ] || fail "Kata shim not found at $KATA_SHIM_PATH. Set MEMOH_KATA_SHIM_PATH to containerd-shim-kata-v2."
[ -d "$KATA_CONFIG_DIR" ] || fail "Kata config directory not found at $KATA_CONFIG_DIR. Set MEMOH_KATA_CONFIG_DIR if your install uses another path."

if [ ! -d "$KATA_OPT_DIR" ]; then
  warn "Kata opt directory not found at $KATA_OPT_DIR. This is OK only if your Kata config points elsewhere."
fi
if [ ! -d "$KATA_SHARE_DIR" ]; then
  warn "Kata share directory not found at $KATA_SHARE_DIR. This is OK only if your Kata config points elsewhere."
fi

echo "Kata dev host preflight passed:"
echo "  MEMOH_KATA_SHIM_PATH=$KATA_SHIM_PATH"
echo "  MEMOH_KATA_CONFIG_DIR=$KATA_CONFIG_DIR"
echo "  MEMOH_KATA_SHARE_DIR=$KATA_SHARE_DIR"
echo "  MEMOH_KATA_OPT_DIR=$KATA_OPT_DIR"
