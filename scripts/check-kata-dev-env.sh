#!/usr/bin/env bash
set -euo pipefail

KATA_SHIM_PATH="${MEMOH_KATA_SHIM_PATH:-/opt/kata/bin/containerd-shim-kata-v2}"
KATA_CONFIG_DIR="${MEMOH_KATA_CONFIG_DIR:-/etc/kata-containers}"
KATA_CONFIG_FILE="$KATA_CONFIG_DIR/configuration.toml"
KATA_SHARE_DIR="${MEMOH_KATA_SHARE_DIR:-/usr/share/kata-containers}"
KATA_OPT_DIR="${MEMOH_KATA_OPT_DIR:-/opt/kata}"
KATA_CHECK_IMAGE="${MEMOH_KATA_CHECK_IMAGE:-${MEMOH_KATA_DEV_IMAGE:-memoh-dev-server-kata}}"
KATA_CHECK_BUILD_HINT="${MEMOH_KATA_CHECK_BUILD_HINT:-docker compose -f devenv/docker-compose.yml -f devenv/docker-compose.kata.yml build migrate server}"
CHECK_CONTAINER="${MEMOH_KATA_CHECK_CONTAINER:-0}"

fail() {
  echo "ERROR: $*" >&2
  exit 1
}

warn() {
  echo "WARN: $*" >&2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "missing required command: $1"
  fi
}

check_container_shim() {
  require_cmd docker
  if ! docker image inspect "$KATA_CHECK_IMAGE" >/dev/null 2>&1; then
    fail "Kata server image $KATA_CHECK_IMAGE is not built. Run: $KATA_CHECK_BUILD_HINT"
  fi

  docker_args=(
    run
    --rm
    --entrypoint
    /bin/bash
    --device
    /dev/kvm:/dev/kvm
    -v
    "$KATA_SHIM_PATH:/usr/local/bin/containerd-shim-kata-v2:ro"
    -v
    "$KATA_CONFIG_DIR:/etc/kata-containers:ro"
  )
  if [ -d "$KATA_SHARE_DIR" ]; then
    docker_args+=(-v "$KATA_SHARE_DIR:/usr/share/kata-containers:ro")
  fi
  if [ -d "$KATA_OPT_DIR" ]; then
    docker_args+=(-v "$KATA_OPT_DIR:/opt/kata:ro")
  fi

  "${docker_args[@]}" "$KATA_CHECK_IMAGE" -lc '
set -euo pipefail
config=/etc/kata-containers/configuration.toml
[ -x /usr/local/bin/containerd-shim-kata-v2 ] || { echo "ERROR: Kata shim is not executable inside the server image." >&2; exit 1; }
[ -f "$config" ] || { echo "ERROR: $config is missing inside the server image." >&2; exit 1; }
[ -e /dev/kvm ] || { echo "ERROR: /dev/kvm is missing inside the server image." >&2; exit 1; }
[ -r /dev/kvm ] || { echo "ERROR: /dev/kvm is not readable inside the server image." >&2; exit 1; }
[ -w /dev/kvm ] || { echo "ERROR: /dev/kvm is not writable inside the server image." >&2; exit 1; }
set +e
output="$(/usr/local/bin/containerd-shim-kata-v2 --version 2>&1)"
status=$?
set -e
if [ "$status" -ne 0 ]; then
  if printf "%s\n" "$output" | grep -Eiq "not found|no such file|permission denied|exec format|error while loading shared libraries|cannot execute"; then
    printf "%s\n" "$output" >&2
    exit "$status"
  fi
  printf "WARN: containerd-shim-kata-v2 returned %s for --version; binary executed, continuing.\n" "$status" >&2
else
  printf "%s\n" "$output" | sed -n "1,3p"
fi

paths="$(sed -E "/^[[:space:]]*#/d; s/[[:space:]]+#.*$//" "$config" | grep -Eo "\"/[^\"]+\"" | tr -d "\"" | sort -u || true)"
missing="$(mktemp)"
while IFS= read -r path; do
  [ -n "$path" ] || continue
  case "$path" in
    /dev/*|/proc/*|/run/*|/sys/*|/tmp/*)
      continue
      ;;
  esac
  if [ ! -e "$path" ]; then
    printf "%s\n" "$path" >>"$missing"
  fi
done <<EOF
$paths
EOF
if [ -s "$missing" ]; then
  echo "ERROR: Kata config references paths that are missing inside the server image:" >&2
  sed "s/^/  /" "$missing" >&2
  echo "Mount the referenced Kata runtime assets or use an official /opt/kata install." >&2
  exit 1
fi
'
}

[ "$(uname -s)" = "Linux" ] || fail "Kata validation requires a Linux host with KVM."
[ -e /dev/kvm ] || fail "/dev/kvm is missing. Enable KVM or run on a VM/bare-metal host with nested virtualization."
[ -r /dev/kvm ] || warn "/dev/kvm exists but is not readable by the current user. Docker may still pass it through when privileged."
if [ ! -f "$KATA_SHIM_PATH" ]; then
  detected_shim="$(command -v containerd-shim-kata-v2 || true)"
  if [ -n "$detected_shim" ]; then
    fail "Kata shim not found at $KATA_SHIM_PATH. Export MEMOH_KATA_SHIM_PATH=$detected_shim before running dev:kata."
  fi
  fail "Kata shim not found at $KATA_SHIM_PATH. Set MEMOH_KATA_SHIM_PATH to containerd-shim-kata-v2."
fi
[ -x "$KATA_SHIM_PATH" ] || fail "Kata shim at $KATA_SHIM_PATH is not executable."
[ -d "$KATA_CONFIG_DIR" ] || fail "Kata config directory not found at $KATA_CONFIG_DIR. Set MEMOH_KATA_CONFIG_DIR if your install uses another path."
[ -f "$KATA_CONFIG_FILE" ] || fail "Kata configuration not found at $KATA_CONFIG_FILE. Set MEMOH_KATA_CONFIG_DIR to a directory containing configuration.toml."

[ -d "$KATA_OPT_DIR" ] || fail "Kata opt directory not found at $KATA_OPT_DIR. Set MEMOH_KATA_OPT_DIR to an existing Kata directory used by your config."
[ -d "$KATA_SHARE_DIR" ] || fail "Kata share directory not found at $KATA_SHARE_DIR. Set MEMOH_KATA_SHARE_DIR to an existing Kata directory used by your config."

echo "Kata host preflight passed:"
echo "  MEMOH_KATA_SHIM_PATH=$KATA_SHIM_PATH"
echo "  MEMOH_KATA_CONFIG_DIR=$KATA_CONFIG_DIR"
echo "  MEMOH_KATA_CONFIG_FILE=$KATA_CONFIG_FILE"
echo "  MEMOH_KATA_SHARE_DIR=$KATA_SHARE_DIR"
echo "  MEMOH_KATA_OPT_DIR=$KATA_OPT_DIR"
echo "  MEMOH_KATA_CHECK_IMAGE=$KATA_CHECK_IMAGE"

if [ "$CHECK_CONTAINER" = "1" ]; then
  check_container_shim
  echo "Kata container preflight passed."
fi
