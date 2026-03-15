#!/bin/sh
# Build bridge binary and place in runtime directory.
# Called by air after server build — safe to skip outside dev container.
set -e

RUNTIME_DIR="/opt/memoh/runtime"
BRIDGE_BINARY="$RUNTIME_DIR/bridge"
STAGING="${BRIDGE_BINARY}.new"

[ -d "$RUNTIME_DIR" ] || exit 0
command -v ctr >/dev/null 2>&1 || exit 0

OLD_HASH=$(sha256sum "$BRIDGE_BINARY" 2>/dev/null | cut -d' ' -f1)
go build -o "$STAGING" ./cmd/bridge || exit 0
NEW_HASH=$(sha256sum "$STAGING" | cut -d' ' -f1)

if [ "$OLD_HASH" = "$NEW_HASH" ]; then
  rm -f "$STAGING"
  exit 0
fi

# Atomic replace avoids "text busy" when the old binary is running.
mv -f "$STAGING" "$BRIDGE_BINARY"
chmod +x "$BRIDGE_BINARY"

echo "[bridge-dev] Done. Containers will restart with new binary on next access."
