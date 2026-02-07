#!/usr/bin/env sh
set -e

IMAGE="memoh-mcp:dev"

if [ "$(uname -s)" = "Darwin" ]; then
  limactl shell default -- nerdctl build -f cmd/mcp/Dockerfile -t "$IMAGE" .
  # Import into rootful containerd so the Go agent can find the image
  limactl shell default -- sh -c "nerdctl save $IMAGE | sudo nerdctl load"
  exit $?
fi

if ! command -v nerdctl >/dev/null 2>&1; then
  echo "nerdctl not found. Install nerdctl to build images."
  exit 1
fi

nerdctl build -f cmd/mcp/Dockerfile -t "$IMAGE" .
