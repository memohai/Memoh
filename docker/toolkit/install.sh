#!/bin/sh
# Download Node.js (glibc + musl), uv, and Xvnc runtime files into a workspace
# runtime assembly.
#
# Usage:
#   ./docker/toolkit/install.sh [toolkit_output_dir] [arch] [display_output_dir]
#
# Arguments:
#   toolkit_output_dir  Target toolkit directory (default: .toolkit)
#   arch                amd64 or arm64 (default: auto-detect from uname -m)
#   display_output_dir  Target display directory (default: toolkit/display)
#
# Environment variables for mirrors (useful in mainland China):
#   NODEJS_MIRROR       Default: https://nodejs.org/dist
#   NODEJS_MUSL_MIRROR  Default: https://unofficial-builds.nodejs.org/download/release
#   NPM_MIRROR          Default: https://registry.npmjs.org
#   ALPINE_MIRROR       Default: https://dl-cdn.alpinelinux.org/alpine
#   UV_MIRROR           Default: https://github.com/astral-sh/uv/releases/latest/download
#   MEMOH_DISPLAY_OUTDIR
#                       Optional override for display_output_dir.
#
set -eu

ALPINE_VERSION=3.23
NODE_VERSION=24.14.0
NPM_VERSION=10.9.2

OUTDIR="${1:-.toolkit}"
ARCH="${2:-}"
DISPLAY_OUTDIR="${MEMOH_DISPLAY_OUTDIR:-${3:-$OUTDIR/display}}"

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

ALPINE_MAIN_REPO="${ALPINE_MIRROR}/v${ALPINE_VERSION}/main"
ALPINE_COMMUNITY_REPO="${ALPINE_MIRROR}/v${ALPINE_VERSION}/community"
ALPINE_MAIN_ARCH_REPO="${ALPINE_MAIN_REPO}/${APK_ARCH}"
ALPINE_COMMUNITY_ARCH_REPO="${ALPINE_COMMUNITY_REPO}/${APK_ARCH}"

TMPDIR="$(mktemp -d)"
cleanup() {
  rm -rf "$TMPDIR"
}
trap cleanup EXIT INT TERM

apk_main_index_path="$TMPDIR/APKINDEX-main.tar.gz"
apk_community_index_path="$TMPDIR/APKINDEX-community.tar.gz"
apk_main_index_text="$TMPDIR/APKINDEX-main"
apk_community_index_text="$TMPDIR/APKINDEX-community"

ensure_apk_indexes() {
  if [ ! -f "$apk_main_index_path" ]; then
    wget -qO "$apk_main_index_path" "${ALPINE_MAIN_ARCH_REPO}/APKINDEX.tar.gz"
    tar -xzOf "$apk_main_index_path" APKINDEX > "$apk_main_index_text"
  fi
  if [ ! -f "$apk_community_index_path" ]; then
    wget -qO "$apk_community_index_path" "${ALPINE_COMMUNITY_ARCH_REPO}/APKINDEX.tar.gz"
    tar -xzOf "$apk_community_index_path" APKINDEX > "$apk_community_index_text"
  fi
}

apk_package_field() {
  pkg="$1"
  field="$2"
  for index_text in "$apk_main_index_text" "$apk_community_index_text"; do
    value="$(awk -v pkg="$pkg" -v field="$field" '
      $0 == "P:" pkg { hit = 1; next }
      hit && index($0, field ":") == 1 { print substr($0, length(field) + 2); exit }
      /^$/ { hit = 0 }
    ' "$index_text")"
    if [ -n "$value" ]; then
      echo "$value"
      return
    fi
  done
}

apk_package_repo() {
  pkg="$1"
  for repo in main community; do
    case "$repo" in
      main) index_text="$apk_main_index_text"; repo_url="$ALPINE_MAIN_ARCH_REPO" ;;
      community) index_text="$apk_community_index_text"; repo_url="$ALPINE_COMMUNITY_ARCH_REPO" ;;
    esac
    if awk -v pkg="$pkg" '
      $0 == "P:" pkg { found = 1; exit }
      END { exit found ? 0 : 1 }
    ' "$index_text"; then
      echo "$repo_url"
      return
    fi
  done
}

apk_package_filename() {
  pkg="$1"
  version="$(apk_package_field "$pkg" V)"
  if [ -n "$version" ]; then
    echo "$pkg-$version.apk"
  fi
}

apk_package_deps() {
  pkg="$1"
  apk_package_field "$pkg" D | tr ' ' '\n' | awk '
    /^$/ { next }
    /^!/ { next }
    {
      dep = $0
      if (dep !~ /^(so:|cmd:|pc:)/) sub(/[<>=~].*/, "", dep)
      if (dep != "") print dep
    }
  '
}

apk_package_provider() {
  dep="$1"
  for index_text in "$apk_main_index_text" "$apk_community_index_text"; do
    provider="$(awk -v dep="$dep" '
      /^P:/ { pkg = substr($0, 3); next }
      /^p:/ {
        split(substr($0, 3), provides, " ")
        for (i in provides) {
          item = provides[i]
          sub(/[<>=~].*/, "", item)
          if (item == dep) {
            print pkg
            exit
          }
        }
      }
    ' "$index_text")"
    if [ -n "$provider" ]; then
      echo "$provider"
      return
    fi
  done
}

resolve_apk_dependency() {
  dep="$1"
  if [ -n "$(apk_package_filename "$dep")" ]; then
    echo "$dep"
    return
  fi
  apk_package_provider "$dep"
}

install_apk_package() {
  pkg="$1"
  root="$2"
  case " $INSTALLED_APK_PACKAGES " in
    *" $pkg "*) return ;;
  esac

  for dep in $(apk_package_deps "$pkg"); do
    dep_pkg="$(resolve_apk_dependency "$dep")"
    if [ -n "$dep_pkg" ]; then
      install_apk_package "$dep_pkg" "$root"
    fi
  done

  apk_file="$(apk_package_filename "$pkg")"
  repo_url="$(apk_package_repo "$pkg")"
  if [ -z "$apk_file" ] || [ -z "$repo_url" ]; then
    echo "ERROR: failed to resolve Alpine package $pkg (${APK_ARCH})" >&2
    exit 1
  fi

  pkg_path="$TMPDIR/$apk_file"
  extract_dir="$TMPDIR/extract-$pkg"
  rm -rf "$extract_dir"
  mkdir -p "$extract_dir"
  if [ ! -f "$pkg_path" ]; then
    wget -qO "$pkg_path" "${repo_url}/$apk_file"
  fi
  tar -xzf "$pkg_path" -C "$extract_dir"
  cp -a "$extract_dir/." "$root/"
  INSTALLED_APK_PACKAGES="$INSTALLED_APK_PACKAGES $pkg"
}

apk_package_filename_from_index() {
  pkg="$1"
  index_text="$2"
  awk -v pkg="$pkg" '
    $0 == "P:" pkg { hit = 1; next }
    hit && /^V:/ { print pkg "-" substr($0, 3) ".apk"; exit }
    /^$/ { hit = 0 }
  ' "$index_text"
}

install_musl_runtime_libs() {
  dest_dir="$OUTDIR/node-musl/runtime-lib"
  if [ -f "$dest_dir/libgcc_s.so.1" ] && [ -f "$dest_dir/libstdc++.so.6" ]; then
    echo "musl runtime libs already installed; skipping download."
    return
  fi

  rm -rf "$dest_dir"
  mkdir -p "$dest_dir"

  echo "Downloading musl runtime libs (${APK_ARCH})..."
  ensure_apk_indexes

  for pkg in libgcc libstdc++; do
    apk_file="$(apk_package_filename_from_index "$pkg" "$apk_main_index_text")"
    if [ -z "$apk_file" ]; then
      echo "ERROR: failed to resolve Alpine package for $pkg (${APK_ARCH})" >&2
      exit 1
    fi
    pkg_path="$TMPDIR/$apk_file"
    extract_dir="$TMPDIR/extract-$pkg"
    rm -rf "$extract_dir"
    mkdir -p "$extract_dir"
    wget -qO "$pkg_path" "${ALPINE_MAIN_ARCH_REPO}/$apk_file"
    tar -xzf "$pkg_path" -C "$extract_dir"
    cp -a "$extract_dir/usr/lib/." "$dest_dir/"
  done
}

install_pinned_npm() {
  node_dir="$1"
  dest_dir="$OUTDIR/$node_dir/lib/node_modules/npm"
  extract_dir="$TMPDIR/npm-$node_dir"
  if [ -f "$dest_dir/bin/npm-cli.js" ]; then
    echo "npm v${NPM_VERSION} already installed for $node_dir; skipping download."
    return
  fi

  ensure_npm_archive

  rm -rf "$dest_dir" "$extract_dir"
  mkdir -p "$extract_dir" "$(dirname "$dest_dir")"
  tar -xzf "$npm_archive" -C "$extract_dir"
  mv "$extract_dir/package" "$dest_dir"
}

ensure_npm_archive() {
  npm_archive="$TMPDIR/npm.tgz"
  if [ -f "$npm_archive" ]; then
    return
  fi
  echo "Downloading npm v${NPM_VERSION}..."
  wget -qO "$npm_archive" "${NPM_MIRROR}/npm/-/npm-${NPM_VERSION}.tgz"
}

install_node_glibc() {
  dest_dir="$OUTDIR/node-glibc"
  if [ -x "$dest_dir/bin/node" ]; then
    echo "Node.js v${NODE_VERSION} (glibc, ${NODE_ARCH}) already installed; skipping download."
    return
  fi

  rm -rf "$dest_dir"
  mkdir -p "$dest_dir"
  echo "Downloading Node.js v${NODE_VERSION} (glibc, ${NODE_ARCH})..."
  wget -qO- "${NODEJS_MIRROR}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
    | tar -xJf - --strip-components=1 -C "$dest_dir"
}

install_node_musl() {
  dest_dir="$OUTDIR/node-musl"
  if [ -x "$dest_dir/bin/node" ]; then
    echo "Node.js v${NODE_VERSION} (musl, ${NODE_ARCH}) already installed; skipping download."
    return
  fi

  rm -rf "$dest_dir"
  mkdir -p "$dest_dir"
  MUSL_URL="${NODEJS_MUSL_MIRROR}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}-musl.tar.xz"
  echo "Downloading Node.js v${NODE_VERSION} (musl, ${NODE_ARCH})..."
  musl_archive="$TMPDIR/node-musl.tar.xz"
  if wget -qO "$musl_archive" "$MUSL_URL" 2>/dev/null; then
    tar -xJf "$musl_archive" --strip-components=1 -C "$dest_dir"
  else
    echo "ERROR: failed to download musl Node.js build for ${NODE_ARCH}" >&2
    exit 1
  fi
}

install_uv() {
  if [ -x "$OUTDIR/uv" ]; then
    echo "uv already installed; skipping download."
    return
  fi

  echo "Downloading uv (${UV_ARCH})..."
  extract_dir="$TMPDIR/uv"
  mkdir -p "$extract_dir"
  wget -qO- "${UV_MIRROR}/uv-${UV_ARCH}-unknown-linux-musl.tar.gz" \
    | tar -xzf - --strip-components=1 -C "$extract_dir"
  mv "$extract_dir/uv" "$OUTDIR/uv"
  chmod +x "$OUTDIR/uv"
}

write_display_wrappers() {
  mkdir -p "$DISPLAY_OUTDIR/bin"

  write_display_musl_wrapper() {
    name="$1"
    cat > "$DISPLAY_OUTDIR/bin/$name" <<EOF
#!/bin/sh
set -eu
SELF="\$0"
if command -v readlink >/dev/null 2>&1; then
  RESOLVED="\$(readlink -f "\$0" 2>/dev/null || true)"
  if [ -n "\$RESOLVED" ]; then
    SELF="\$RESOLVED"
  fi
fi
ROOT="\$(CDPATH= cd -- "\$(dirname -- "\$SELF")/../root" && pwd)"
ARCH="\$(uname -m)"
case "\$ARCH" in
  x86_64)  LOADER="\$ROOT/lib/ld-musl-x86_64.so.1" ;;
  aarch64|arm64) LOADER="\$ROOT/lib/ld-musl-aarch64.so.1" ;;
  *) echo "ERROR: unsupported architecture: \$ARCH" >&2; exit 1 ;;
esac
export PATH="\$ROOT/../bin:\$ROOT/usr/bin:\$PATH"
export XKB_CONFIG_ROOT="\$ROOT/usr/share/X11/xkb"
export FONTCONFIG_PATH="\$ROOT/etc/fonts"
export XDG_DATA_DIRS="\$ROOT/usr/share:\${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"
exec "\$LOADER" --library-path "\$ROOT/lib:\$ROOT/usr/lib" "\$ROOT/usr/bin/$name" "\$@"
EOF
    chmod +x "$DISPLAY_OUTDIR/bin/$name"
  }

  cat > "$DISPLAY_OUTDIR/bin/xkbcomp" <<'EOF'
#!/bin/sh
set -eu
SELF="$0"
if command -v readlink >/dev/null 2>&1; then
  RESOLVED="$(readlink -f "$0" 2>/dev/null || true)"
  if [ -n "$RESOLVED" ]; then
    SELF="$RESOLVED"
  fi
fi
ROOT="$(CDPATH= cd -- "$(dirname -- "$SELF")/../root" && pwd)"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  LOADER="$ROOT/lib/ld-musl-x86_64.so.1" ;;
  aarch64|arm64) LOADER="$ROOT/lib/ld-musl-aarch64.so.1" ;;
  *) echo "ERROR: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
exec "$LOADER" --library-path "$ROOT/lib:$ROOT/usr/lib" "$ROOT/usr/bin/xkbcomp" "$@"
EOF
  chmod +x "$DISPLAY_OUTDIR/bin/xkbcomp"

  cat > "$DISPLAY_OUTDIR/bin/Xvnc" <<'EOF'
#!/bin/sh
set -eu
SELF="$0"
if command -v readlink >/dev/null 2>&1; then
  RESOLVED="$(readlink -f "$0" 2>/dev/null || true)"
  if [ -n "$RESOLVED" ]; then
    SELF="$RESOLVED"
  fi
fi
ROOT="$(CDPATH= cd -- "$(dirname -- "$SELF")/../root" && pwd)"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  LOADER="$ROOT/lib/ld-musl-x86_64.so.1" ;;
  aarch64|arm64) LOADER="$ROOT/lib/ld-musl-aarch64.so.1" ;;
  *) echo "ERROR: unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
export PATH="$ROOT/../bin:$ROOT/usr/bin:$PATH"
export XKB_CONFIG_ROOT="$ROOT/usr/share/X11/xkb"
export FONTCONFIG_PATH="$ROOT/etc/fonts"
exec "$LOADER" --library-path "$ROOT/lib:$ROOT/usr/lib" "$ROOT/usr/bin/Xvnc" -xkbdir "$ROOT/usr/share/X11/xkb" "$@"
EOF
  chmod +x "$DISPLAY_OUTDIR/bin/Xvnc"

  write_display_musl_wrapper xsetroot
  write_display_musl_wrapper twm
  write_display_musl_wrapper xterm
}

install_display_bundle() {
  mkdir -p "$DISPLAY_OUTDIR"
	if [ -x "$DISPLAY_OUTDIR/root/usr/bin/Xvnc" ] && [ -x "$DISPLAY_OUTDIR/root/usr/bin/xkbcomp" ] && [ -x "$DISPLAY_OUTDIR/root/usr/bin/xsetroot" ] && [ -x "$DISPLAY_OUTDIR/root/usr/bin/twm" ] && [ -x "$DISPLAY_OUTDIR/root/usr/bin/xterm" ]; then
		write_display_wrappers
    echo "Display bundle already installed to $DISPLAY_OUTDIR; skipping download."
    return
  fi

  rm -rf "$DISPLAY_OUTDIR"
  mkdir -p "$DISPLAY_OUTDIR/bin"

  echo "Installing display runtime from Alpine packages (${APK_ARCH})..."
  if command -v apk >/dev/null 2>&1; then
    apk add \
      --root "$DISPLAY_OUTDIR/root" \
      --initdb \
      --no-cache \
      --no-scripts \
      --allow-untrusted \
      --repository "$ALPINE_MAIN_REPO" \
      --repository "$ALPINE_COMMUNITY_REPO" \
      tigervnc \
      xkeyboard-config \
      font-misc-misc \
      xsetroot \
      twm \
      xterm
  elif command -v docker >/dev/null 2>&1; then
    display_abs="$(cd "$DISPLAY_OUTDIR" && pwd)"
    docker run --rm \
      -v "$display_abs:/out" \
      "alpine:${ALPINE_VERSION}" \
      sh -eu -c 'apk add --root /out/root --initdb --no-cache --no-scripts --allow-untrusted --repository "$1" --repository "$2" tigervnc xkeyboard-config font-misc-misc xsetroot twm xterm' \
      sh "$ALPINE_MAIN_REPO" "$ALPINE_COMMUNITY_REPO"
  else
    echo "ERROR: installing the display runtime requires apk or docker." >&2
    exit 1
  fi

	write_display_wrappers

  if [ ! -x "$DISPLAY_OUTDIR/bin/Xvnc" ]; then
    echo "ERROR: display bundle does not contain executable bin/Xvnc" >&2
    exit 1
  fi

  echo "Display bundle installed to $DISPLAY_OUTDIR"
}

mkdir -p "$OUTDIR/node-glibc" "$OUTDIR/node-musl"

install_node_glibc
install_node_musl
install_musl_runtime_libs

install_pinned_npm node-glibc
install_pinned_npm node-musl

install_uv

echo "Toolkit installed to $OUTDIR"
install_display_bundle
