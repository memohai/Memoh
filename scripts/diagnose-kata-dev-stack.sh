#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${MEMOH_VERIFY_BASE_URL:-http://127.0.0.1:${MEMOH_DEV_SERVER_PORT:-18080}}"
SERVER_CONTAINER="${MEMOH_KATA_DEV_SERVER_CONTAINER:-memoh-dev-server}"
EXPECTED_CONFIG="${MEMOH_KATA_DEV_EXPECTED_CONFIG:-/workspace/devenv/app.kata.dev.toml}"
EXPECTED_RUNTIME="${MEMOH_VERIFY_EXPECTED_RUNTIME:-io.containerd.kata.v2}"
PING_JSON="$(mktemp "${TMPDIR:-/tmp}/memoh-kata-dev-ping.XXXXXX.json")"
CONTAINER_JSON="$(mktemp "${TMPDIR:-/tmp}/memoh-kata-dev-container.XXXXXX.json")"

cleanup() {
  rm -f "$PING_JSON" "$CONTAINER_JSON"
}
trap cleanup EXIT

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "ERROR: missing required command: $1" >&2
    exit 1
  fi
}

warn() {
  echo "WARN: $*" >&2
}

require_cmd curl
require_cmd docker
require_cmd jq

echo "Inspecting Memoh Kata dev stack:"
echo "  base_url=$BASE_URL"
echo "  server_container=$SERVER_CONTAINER"
echo "  expected_config=$EXPECTED_CONFIG"
echo "  expected_runtime=$EXPECTED_RUNTIME"

if ! curl -fsS "$BASE_URL/ping" >"$PING_JSON" 2>/dev/null; then
  echo "ERROR: Memoh server is not reachable at $BASE_URL." >&2
  echo "Start the Kata dev stack with: mise run dev:kata" >&2
  exit 1
fi

container_backend="$(jq -r '.container_backend // "unknown"' "$PING_JSON")"
local_workspace_enabled="$(jq -r '.local_workspace_enabled // "unknown"' "$PING_JSON")"
snapshot_supported="$(jq -r '.snapshot_supported // "unknown"' "$PING_JSON")"
echo "Server ping:"
echo "  container_backend=$container_backend"
echo "  local_workspace_enabled=$local_workspace_enabled"
echo "  snapshot_supported=$snapshot_supported"

if [ "$container_backend" != "containerd" ]; then
  echo "ERROR: server container_backend is $container_backend, expected containerd for Kata." >&2
  exit 1
fi

if ! docker container inspect "$SERVER_CONTAINER" >"$CONTAINER_JSON" 2>/dev/null; then
  echo "ERROR: Docker container $SERVER_CONTAINER was not found." >&2
  echo "Start the Kata dev stack with: mise run dev:kata" >&2
  exit 1
fi

image="$(jq -r '.[0].Config.Image // "unknown"' "$CONTAINER_JSON")"
config_path="$(jq -r '.[0].Config.Env[]? | select(startswith("CONFIG_PATH=")) | ltrimstr("CONFIG_PATH=")' "$CONTAINER_JSON")"
has_kvm_device="$(
  jq -r '
    any(.[0].HostConfig.Devices[]?; .PathOnHost == "/dev/kvm" and .PathInContainer == "/dev/kvm")
  ' "$CONTAINER_JSON"
)"
has_kata_shim_mount="$(
  jq -r '
    any(.[0].Mounts[]?; .Destination == "/usr/local/bin/containerd-shim-kata-v2")
  ' "$CONTAINER_JSON"
)"
has_kata_config_mount="$(
  jq -r '
    any(.[0].Mounts[]?; .Destination == "/etc/kata-containers")
  ' "$CONTAINER_JSON"
)"

echo "Server container:"
echo "  image=$image"
echo "  CONFIG_PATH=${config_path:-missing}"
echo "  /dev/kvm device=$has_kvm_device"
echo "  Kata shim mount=$has_kata_shim_mount"
echo "  Kata config mount=$has_kata_config_mount"

if [ "${config_path:-}" != "$EXPECTED_CONFIG" ]; then
  echo "ERROR: this dev server is using ${config_path:-missing}, not $EXPECTED_CONFIG." >&2
  echo "It is the normal dev stack, not the Kata dev stack. Restart with:" >&2
  echo "  docker compose -f devenv/docker-compose.yml -f devenv/docker-compose.kata.yml down --remove-orphans" >&2
  echo "  mise run dev:kata" >&2
  exit 1
fi

if [ "$has_kvm_device" != "true" ]; then
  echo "ERROR: $SERVER_CONTAINER does not have /dev/kvm mounted." >&2
  echo "Kata requires a Linux/KVM host and the Kata compose override." >&2
  exit 1
fi

if [ "$has_kata_shim_mount" != "true" ] || [ "$has_kata_config_mount" != "true" ]; then
  echo "ERROR: $SERVER_CONTAINER is missing Kata shim/config mounts." >&2
  echo "Check MEMOH_KATA_SHIM_PATH and MEMOH_KATA_CONFIG_DIR, then restart with mise run dev:kata." >&2
  exit 1
fi

warn "Kata uses container_backend=containerd; verify runtime on a bot workspace with: mise run test:kata"
echo "Kata dev stack shape looks correct."
