#!/bin/sh
set -e

MCP_IMAGE="${MCP_IMAGE:-docker.io/library/memoh-mcp:latest}"

# ---- Setup cgroup v2 delegation for nested containerd ----
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  echo "Setting up cgroup v2 delegation..."
  mkdir -p /sys/fs/cgroup/init
  # Move existing processes out of root cgroup to allow subtree control
  while read -r pid; do
    echo "$pid" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
  done < /sys/fs/cgroup/cgroup.procs
  # Enable all available controllers for subtree delegation
  sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers \
    > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
fi

# ---- Start containerd in background ----
mkdir -p /run/containerd
containerd &
CONTAINERD_PID=$!

echo "Waiting for containerd..."
for i in $(seq 1 30); do
  if ctr version >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! ctr version >/dev/null 2>&1; then
  echo "ERROR: containerd not responsive after 30s"
  exit 1
fi
echo "containerd is running (pid $CONTAINERD_PID)"

# ---- Import MCP image if not already present ----
if ! ctr -n default images check "name==${MCP_IMAGE}" 2>/dev/null | grep -q "${MCP_IMAGE}"; then
  echo "Importing MCP image into containerd..."
  for tar in /opt/images/*.tar; do
    if [ -f "$tar" ]; then
      ctr -n default images import --all-platforms "$tar" 2>&1 || true
    fi
  done
  if ctr -n default images check "name==${MCP_IMAGE}" 2>/dev/null | grep -q "${MCP_IMAGE}"; then
    echo "MCP image ready: ${MCP_IMAGE}"
  else
    echo "WARNING: MCP image not available after import, will try pull at runtime"
  fi
else
  echo "MCP image already present: ${MCP_IMAGE}"
fi

echo "containerd is ready, starting memoh-server..."

# ---- Start server (foreground, trap signals for graceful shutdown) ----
trap 'echo "Shutting down..."; kill $SERVER_PID 2>/dev/null; kill $CONTAINERD_PID 2>/dev/null; wait' TERM INT

/app/memoh-server serve &
SERVER_PID=$!

wait $SERVER_PID
EXIT_CODE=$?

kill $CONTAINERD_PID 2>/dev/null || true
wait $CONTAINERD_PID 2>/dev/null || true

exit $EXIT_CODE
