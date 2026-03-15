#!/bin/sh
# Download Node.js (glibc + musl) and uv into a toolkit directory.
#
# Usage:
#   ./docker/toolkit/install.sh [output_dir] [arch]
#
# Arguments:
#   output_dir  Target directory (default: .toolkit)
#   arch        amd64 or arm64 (default: auto-detect from uname -m)
#
# Environment variables for mirrors (useful in mainland China):
#   NODEJS_MIRROR       Default: https://nodejs.org/dist
#   NODEJS_MUSL_MIRROR  Default: https://unofficial-builds.nodejs.org/download/release
#   UV_MIRROR           Default: https://github.com/astral-sh/uv/releases/latest/download
#
set -eu

NODE_VERSION=22.14.0

OUTDIR="${1:-.toolkit}"
ARCH="${2:-}"

if [ -z "$ARCH" ]; then
  case "$(uname -m)" in
    x86_64)  ARCH=amd64 ;;
    aarch64) ARCH=arm64 ;;
    arm64)   ARCH=arm64 ;;
    *) echo "ERROR: unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac
fi

NODEJS_MIRROR="${NODEJS_MIRROR:-https://nodejs.org/dist}"
NODEJS_MUSL_MIRROR="${NODEJS_MUSL_MIRROR:-https://unofficial-builds.nodejs.org/download/release}"
UV_MIRROR="${UV_MIRROR:-https://github.com/astral-sh/uv/releases/latest/download}"

case "$ARCH" in
  amd64) NODE_ARCH=x64;  UV_ARCH=x86_64  ;;
  arm64) NODE_ARCH=arm64; UV_ARCH=aarch64 ;;
  *) echo "ERROR: unsupported arch: $ARCH" >&2; exit 1 ;;
esac

mkdir -p "$OUTDIR/node-glibc" "$OUTDIR/node-musl"

echo "Downloading Node.js v${NODE_VERSION} (glibc, ${NODE_ARCH})..."
wget -qO- "${NODEJS_MIRROR}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
  | tar -xJf - --strip-components=1 -C "$OUTDIR/node-glibc"

# ARM64 musl builds are not available from unofficial-builds.nodejs.org.
# On ARM64 Alpine containers the glibc build works via musl's glibc compat layer.
MUSL_URL="${NODEJS_MUSL_MIRROR}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}-musl.tar.xz"
echo "Downloading Node.js v${NODE_VERSION} (musl, ${NODE_ARCH})..."
if wget -q --spider "$MUSL_URL" 2>/dev/null; then
  wget -qO- "$MUSL_URL" | tar -xJf - --strip-components=1 -C "$OUTDIR/node-musl"
else
  echo "  musl build not available for ${NODE_ARCH}, using glibc build as fallback"
  cp -a "$OUTDIR/node-glibc/." "$OUTDIR/node-musl/"
fi

echo "Downloading uv (${UV_ARCH})..."
wget -qO- "${UV_MIRROR}/uv-${UV_ARCH}-unknown-linux-musl.tar.gz" \
  | tar -xzf - --strip-components=1 -C /tmp
mv /tmp/uv "$OUTDIR/uv"
chmod +x "$OUTDIR/uv"

echo "Toolkit installed to $OUTDIR"
