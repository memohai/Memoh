#!/bin/sh
set -e

# Toolkit is volume-mounted from the host (.toolkit/).
# If missing, the user forgot to run the install script.
if [ ! -d /opt/memoh/runtime/toolkit/node-glibc ]; then
  echo "ERROR: Toolkit not found at /opt/memoh/runtime/toolkit/." >&2
  echo "       Run ./docker/toolkit/install.sh before starting the dev environment." >&2
  exit 1
fi

# Clean up stale CNI state from previous runs. After a container restart the
# cni0 bridge may linger with a zeroed MAC (00:00:00:00:00:00), causing the
# bridge plugin to fail with "could not set bridge's mac: invalid argument".
ip link delete cni0 2>/dev/null || true
rm -rf /var/lib/cni/networks/* /var/lib/cni/results/* 2>/dev/null || true

# Ensure IP forwarding and subnet MASQUERADE for CNI.
sysctl -w net.ipv4.ip_forward=1 2>/dev/null || true
iptables -t nat -C POSTROUTING -s 10.88.0.0/16 ! -o cni0 -j MASQUERADE 2>/dev/null || \
  iptables -t nat -A POSTROUTING -s 10.88.0.0/16 ! -o cni0 -j MASQUERADE 2>/dev/null || true

# Setup cgroup v2 delegation for nested containerd.
if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
  mkdir -p /sys/fs/cgroup/init
  while read -r pid; do
    echo "$pid" > /sys/fs/cgroup/init/cgroup.procs 2>/dev/null || true
  done < /sys/fs/cgroup/cgroup.procs

  sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers \
    > /sys/fs/cgroup/cgroup.subtree_control 2>/dev/null || true
fi

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

# Build bridge binary into runtime directory (first boot)
echo "Building bridge binary..."
(cd /workspace && go build -o /opt/memoh/runtime/bridge ./cmd/bridge)
echo "Bridge binary ready."

echo "Starting server..."

trap 'kill ${SERVER_PID:-0} 2>/dev/null || true; kill ${CONTAINERD_PID:-0} 2>/dev/null || true; wait' TERM INT

"$@" &
SERVER_PID=$!

wait $SERVER_PID
EXIT_CODE=$?

kill $CONTAINERD_PID 2>/dev/null || true
wait $CONTAINERD_PID 2>/dev/null || true

exit $EXIT_CODE
