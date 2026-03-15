#!/bin/sh

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
echo "containerd is ready"

wait $CONTAINERD_PID
