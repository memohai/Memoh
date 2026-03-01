#!/bin/sh
set -e

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

echo "containerd is ready, starting server command..."

trap 'kill ${SERVER_PID:-0} 2>/dev/null || true; kill ${CONTAINERD_PID:-0} 2>/dev/null || true; wait' TERM INT

"$@" &
SERVER_PID=$!

wait $SERVER_PID
EXIT_CODE=$?

kill $CONTAINERD_PID 2>/dev/null || true
wait $CONTAINERD_PID 2>/dev/null || true

exit $EXIT_CODE
