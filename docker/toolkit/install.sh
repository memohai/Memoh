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
#   NPM_MIRROR          Default: https://registry.npmjs.org
#   ALPINE_MIRROR       Default: https://dl-cdn.alpinelinux.org/alpine
#   UV_MIRROR           Default: https://github.com/astral-sh/uv/releases/latest/download
#
set -eu

ALPINE_VERSION=3.23
NODE_VERSION=24.14.0
NPM_VERSION=10.9.2

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
NPM_MIRROR="${NPM_MIRROR:-https://registry.npmjs.org}"
ALPINE_MIRROR="${ALPINE_MIRROR:-https://dl-cdn.alpinelinux.org/alpine}"
UV_MIRROR="${UV_MIRROR:-https://github.com/astral-sh/uv/releases/latest/download}"

case "$ARCH" in
  amd64) NODE_ARCH=x64;  UV_ARCH=x86_64;  APK_ARCH=x86_64 ;;
  arm64) NODE_ARCH=arm64; UV_ARCH=aarch64; APK_ARCH=aarch64 ;;
  *) echo "ERROR: unsupported arch: $ARCH" >&2; exit 1 ;;
esac

ALPINE_REPO="${ALPINE_MIRROR}/v${ALPINE_VERSION}/main/${APK_ARCH}"

TMPDIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMPDIR"
}
trap cleanup EXIT INT TERM

apk_index_path="$TMPDIR/APKINDEX.tar.gz"

apk_package_filename() {
  pkg="$1"
  tar -xzOf "$apk_index_path" APKINDEX | awk -v pkg="$pkg" '
    $0 == "P:" pkg { hit = 1; next }
    hit && /^V:/ { print pkg "-" substr($0, 3) ".apk"; exit }
    /^$/ { hit = 0 }
  '
}

install_musl_runtime_libs() {
  dest_dir="$OUTDIR/node-musl/runtime-lib"
  rm -rf "$dest_dir"
  mkdir -p "$dest_dir"

  echo "Downloading musl runtime libs (${APK_ARCH})..."
  wget -qO "$apk_index_path" "${ALPINE_REPO}/APKINDEX.tar.gz"

  for pkg in libgcc libstdc++; do
    apk_file="$(apk_package_filename "$pkg")"
    if [ -z "$apk_file" ]; then
      echo "ERROR: failed to resolve Alpine package for $pkg (${APK_ARCH})" >&2
      exit 1
    fi
    pkg_path="$TMPDIR/$apk_file"
    extract_dir="$TMPDIR/extract-$pkg"
    rm -rf "$extract_dir"
    mkdir -p "$extract_dir"
    wget -qO "$pkg_path" "${ALPINE_REPO}/$apk_file"
    tar -xzf "$pkg_path" -C "$extract_dir"
    cp -a "$extract_dir/usr/lib/." "$dest_dir/"
  done
}

install_pinned_npm() {
  node_dir="$1"
  dest_dir="$OUTDIR/$node_dir/lib/node_modules/npm"
  extract_dir="$TMPDIR/npm-$node_dir"

  rm -rf "$dest_dir" "$extract_dir"
  mkdir -p "$extract_dir" "$(dirname "$dest_dir")"
  tar -xzf "$npm_archive" -C "$extract_dir"
  mv "$extract_dir/package" "$dest_dir"
}

mkdir -p "$OUTDIR/node-glibc" "$OUTDIR/node-musl"

echo "Downloading Node.js v${NODE_VERSION} (glibc, ${NODE_ARCH})..."
wget -qO- "${NODEJS_MIRROR}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
  | tar -xJf - --strip-components=1 -C "$OUTDIR/node-glibc"

MUSL_URL="${NODEJS_MUSL_MIRROR}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}-musl.tar.xz"
echo "Downloading Node.js v${NODE_VERSION} (musl, ${NODE_ARCH})..."
musl_archive="$TMPDIR/node-musl.tar.xz"
if wget -qO "$musl_archive" "$MUSL_URL" 2>/dev/null; then
  tar -xJf "$musl_archive" --strip-components=1 -C "$OUTDIR/node-musl"
else
  echo "ERROR: failed to download musl Node.js build for ${NODE_ARCH}" >&2
  exit 1
fi

install_musl_runtime_libs

echo "Downloading npm v${NPM_VERSION}..."
npm_archive="$TMPDIR/npm.tgz"
wget -qO "$npm_archive" "${NPM_MIRROR}/npm/-/npm-${NPM_VERSION}.tgz"
install_pinned_npm node-glibc
install_pinned_npm node-musl

echo "Downloading uv (${UV_ARCH})..."
wget -qO- "${UV_MIRROR}/uv-${UV_ARCH}-unknown-linux-musl.tar.gz" \
  | tar -xzf - --strip-components=1 -C /tmp
mv /tmp/uv "$OUTDIR/uv"
chmod +x "$OUTDIR/uv"

echo "Toolkit installed to $OUTDIR"
