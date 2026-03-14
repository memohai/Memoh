#!/bin/sh
# Build MCP binary and place in runtime directory.
# Called by air after server build — safe to skip outside dev container.
set -e

RUNTIME_DIR="/opt/memoh/runtime"
MCP_BINARY="$RUNTIME_DIR/mcp"
STAGING="${MCP_BINARY}.new"

[ -d "$RUNTIME_DIR" ] || exit 0
command -v ctr >/dev/null 2>&1 || exit 0

OLD_HASH=$(sha256sum "$MCP_BINARY" 2>/dev/null | cut -d' ' -f1)
go build -o "$STAGING" ./cmd/mcp || exit 0
NEW_HASH=$(sha256sum "$STAGING" | cut -d' ' -f1)

if [ "$OLD_HASH" = "$NEW_HASH" ]; then
  rm -f "$STAGING"
  exit 0
fi

# Atomic replace avoids "text busy" when the old binary is running.
mv -f "$STAGING" "$MCP_BINARY"
chmod +x "$MCP_BINARY"

echo "[mcp-dev] Binary updated, stopping MCP container tasks..."

# Stop running MCP container tasks so they restart with new binary on next access.
for c in $(ctr -n default containers ls -q 2>/dev/null | grep "^mcp-"); do
  ctr -n default tasks kill "$c" 2>/dev/null || true
  ctr -n default tasks delete "$c" 2>/dev/null || true
done

echo "[mcp-dev] Done. Containers will restart with new binary on next access."
