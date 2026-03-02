#!/bin/sh
# Build MCP binary, package as containerd image, and import.
# Called by air after server build — safe to skip outside dev container.
set -e

MCP_IMAGE="${MCP_IMAGE:-docker.io/memohai/mcp:latest}"
MCP_BINARY="/opt/memoh/data/.dev/mcp"
BASE_ROOTFS="/opt/images/memoh-mcp-rootfs.tar"

[ -f "$BASE_ROOTFS" ] || exit 0
command -v ctr >/dev/null 2>&1 || exit 0

mkdir -p "$(dirname "$MCP_BINARY")"

OLD_HASH=$(sha256sum "$MCP_BINARY" 2>/dev/null | cut -d' ' -f1)
go build -o "$MCP_BINARY" ./cmd/mcp || exit 0
NEW_HASH=$(sha256sum "$MCP_BINARY" | cut -d' ' -f1)

[ "$OLD_HASH" = "$NEW_HASH" ] && exit 0

echo "[mcp-dev] Binary changed, rebuilding MCP image..."

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

# Layer 1: base rootfs (symlink to avoid copying the large file)
LAYER1_SHA=$(sha256sum "$BASE_ROOTFS" | cut -d' ' -f1)
mkdir -p "$WORK/$LAYER1_SHA"
ln -s "$BASE_ROOTFS" "$WORK/$LAYER1_SHA/layer.tar"

# Layer 2: compiled binary overlay
mkdir -p "$WORK/overlay/opt"
cp "$MCP_BINARY" "$WORK/overlay/opt/mcp"
chmod +x "$WORK/overlay/opt/mcp"
tar -cf "$WORK/layer2.tar" -C "$WORK/overlay" opt
LAYER2_SHA=$(sha256sum "$WORK/layer2.tar" | cut -d' ' -f1)
mkdir -p "$WORK/$LAYER2_SHA"
mv "$WORK/layer2.tar" "$WORK/$LAYER2_SHA/layer.tar"

# OCI image config
ARCH=$(uname -m)
case "$ARCH" in aarch64|arm64) ARCH="arm64" ;; x86_64|amd64) ARCH="amd64" ;; esac

printf '{"architecture":"%s","os":"linux","created":"1970-01-01T00:00:00Z","config":{"Entrypoint":["/opt/entrypoint.sh"],"WorkingDir":"/app","Env":["PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"]},"rootfs":{"type":"layers","diff_ids":["sha256:%s","sha256:%s"]},"history":[{"created":"1970-01-01T00:00:00Z","comment":"memoh-mcp rootfs"},{"created":"1970-01-01T00:00:00Z","comment":"memoh-mcp binary"}]}' \
    "$ARCH" "$LAYER1_SHA" "$LAYER2_SHA" > "$WORK/config.json"

CONFIG_SHA=$(sha256sum "$WORK/config.json" | cut -d' ' -f1)
mv "$WORK/config.json" "$WORK/$CONFIG_SHA.json"

printf '[{"Config":"%s.json","RepoTags":["%s"],"Layers":["%s/layer.tar","%s/layer.tar"]}]' \
    "$CONFIG_SHA" "$MCP_IMAGE" "$LAYER1_SHA" "$LAYER2_SHA" > "$WORK/manifest.json"

# -h follows symlinks (layer 1 is symlinked to avoid copying)
tar -chf "$WORK/memoh-mcp.tar" -C "$WORK" manifest.json "$CONFIG_SHA.json" "$LAYER1_SHA/" "$LAYER2_SHA/"

# Replace image in containerd
ctr -n default images rm "$MCP_IMAGE" 2>/dev/null || true
ctr -n default images import --all-platforms "$WORK/memoh-mcp.tar" 2>&1 || true

# Clean old MCP containers so they recreate with new image
for c in $(ctr -n default containers ls -q 2>/dev/null | grep "^mcp-"); do
  ctr -n default tasks kill "$c" 2>/dev/null || true
  ctr -n default tasks delete "$c" 2>/dev/null || true
  ctr -n default containers delete "$c" 2>/dev/null || true
done

echo "[mcp-dev] Done. Containers will auto-recreate with new image."
