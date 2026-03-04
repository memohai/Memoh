çç#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
TARGET_OS="${TARGET_OS:-$(go env GOOS)}"
TARGET_ARCH="${TARGET_ARCH:-$(go env GOARCH)}"
VERSION="${VERSION:-dev}"
COMMIT_HASH="${COMMIT_HASH:-unknown}"
BUILD_TIME="${BUILD_TIME:-$(date -u +"%Y-%m-%dT%H:%M:%SZ")}"
OUTPUT_DIR="${OUTPUT_DIR:-$ROOT_DIR/dist}"
PREPARE_ASSETS_ONLY="false"
UPX_COMPRESS_AGENT_BIN="${UPX_COMPRESS_AGENT_BIN:-false}"
UPX_ARGS="${UPX_ARGS:--3}"
UPX_ALLOW_DARWIN="${UPX_ALLOW_DARWIN:-false}"
AUTO_INSTALL_UPX="${AUTO_INSTALL_UPX:-}"
if [[ -z "$AUTO_INSTALL_UPX" ]]; then
  AUTO_INSTALL_UPX=$([[ "${GITHUB_ACTIONS:-}" == "true" ]] && echo "true" || echo "false")
fi

WEB_DIR="$ROOT_DIR/internal/embedded/web"
AGENT_DIR="$ROOT_DIR/internal/embedded/agent"
BUN_DIR="$ROOT_DIR/internal/embedded/bun"

log() {
  echo "[release] $*"
}

usage() {
  cat <<'EOF'
Usage: scripts/release.sh [options]

Options:
  --os <os>             Target OS (default: current GOOS)
  --arch <arch>         Target ARCH (default: current GOARCH)
  --version <version>   Version string injected into memoh binary
  --commit-hash <sha>   Commit hash injected into memoh binary
  --output-dir <dir>    Output directory for release artifacts
  --prepare-assets      Only prepare embedded assets, do not build archive

Compatibility options:
  --bun-version <v>     Deprecated; ignored (kept for backward compatibility)
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --os)
        TARGET_OS="$2"
        shift 2
        ;;
      --arch)
        TARGET_ARCH="$2"
        shift 2
        ;;
      --bun-version)
        # Bun runtime archives are no longer embedded; keep arg for compatibility.
        shift 2
        ;;
      --version)
        VERSION="$2"
        shift 2
        ;;
      --commit-hash)
        COMMIT_HASH="$2"
        shift 2
        ;;
      --output-dir)
        OUTPUT_DIR="$2"
        shift 2
        ;;
      --prepare-assets)
        PREPARE_ASSETS_ONLY="true"
        shift
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Unknown arg: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
  done
}

write_keep_gitignore() {
  local dir="$1"
  printf "*\n!.gitignore\n" > "$dir/.gitignore"
}

resolve_agent_compile_target() {
  case "${TARGET_OS}-${TARGET_ARCH}" in
    linux-amd64) echo "bun-linux-x64|agent-bin" ;;
    linux-arm64) echo "bun-linux-arm64|agent-bin" ;;
    darwin-amd64) echo "bun-darwin-x64|agent-bin" ;;
    darwin-arm64) echo "bun-darwin-arm64|agent-bin" ;;
    windows-amd64) echo "bun-windows-x64|agent-bin.exe" ;;
    *) echo "|" ;;
  esac
}

prepare_embed_dirs() {
  rm -rf "$WEB_DIR" "$AGENT_DIR" "$BUN_DIR"
  mkdir -p "$WEB_DIR" "$AGENT_DIR" "$BUN_DIR"
  write_keep_gitignore "$WEB_DIR"
  write_keep_gitignore "$AGENT_DIR"
  write_keep_gitignore "$BUN_DIR"
}

prepare_assets() {
  prepare_embed_dirs

  log "building web assets"
  pnpm --dir "$ROOT_DIR" web:build
  cp -R "$ROOT_DIR/packages/web/dist/." "$WEB_DIR/"
  gzip_embedded_web_assets "$WEB_DIR"

  local target_key="${TARGET_OS}-${TARGET_ARCH}"
  local resolved bun_compile_target agent_bin_name
  resolved="$(resolve_agent_compile_target)"
  bun_compile_target="${resolved%%|*}"
  agent_bin_name="${resolved##*|}"
  if [[ -z "$bun_compile_target" || -z "$agent_bin_name" ]]; then
    echo "agent-bin not available for ${target_key}" > "$AGENT_DIR/UNAVAILABLE"
    log "skipped agent-bin compile for unsupported target ${target_key}"
    return 0
  fi

  log "building agent executable (${bun_compile_target})"
  patch_jsdom_style_loader_for_compile
  trap 'restore_jsdom_style_loader_patch' RETURN
  (
    cd "$ROOT_DIR/agent"
    bun build src/index.ts --compile --target "$bun_compile_target" --outfile "$AGENT_DIR/$agent_bin_name"
  )
  restore_jsdom_style_loader_patch
  trap - RETURN
  chmod +x "$AGENT_DIR/$agent_bin_name" || true
  compress_agent_bin_if_enabled "$AGENT_DIR/$agent_bin_name" "$TARGET_OS"

  log "embedded assets prepared (${target_key})"
}

JSDOM_STYLE_RULES_FILE=""
JSDOM_STYLE_RULES_BACKUP=""
JSDOM_XHR_IMPL_FILE=""
JSDOM_XHR_IMPL_BACKUP=""

patch_jsdom_style_loader_for_compile() {
  local css_path css_json
  JSDOM_STYLE_RULES_FILE="$(node -e "try{process.stdout.write(require.resolve('jsdom/lib/jsdom/living/helpers/style-rules.js',{paths:['$ROOT_DIR/agent']}))}catch{process.exit(1)}" 2>/dev/null || true)"
  css_path="$(node -e "try{process.stdout.write(require.resolve('jsdom/lib/jsdom/browser/default-stylesheet.css',{paths:['$ROOT_DIR/agent']}))}catch{process.exit(1)}" 2>/dev/null || true)"
  JSDOM_XHR_IMPL_FILE="$(node -e "try{process.stdout.write(require.resolve('jsdom/lib/jsdom/living/xhr/XMLHttpRequest-impl.js',{paths:['$ROOT_DIR/agent']}))}catch{process.exit(1)}" 2>/dev/null || true)"

  if [[ -z "$JSDOM_STYLE_RULES_FILE" || -z "$css_path" || -z "$JSDOM_XHR_IMPL_FILE" ]]; then
    log "skip jsdom patch (jsdom sources not resolved)"
    return 0
  fi

  JSDOM_STYLE_RULES_BACKUP="${JSDOM_STYLE_RULES_FILE}.memoh.bak"
  JSDOM_XHR_IMPL_BACKUP="${JSDOM_XHR_IMPL_FILE}.memoh.bak"
  cp "$JSDOM_STYLE_RULES_FILE" "$JSDOM_STYLE_RULES_BACKUP"
  cp "$JSDOM_XHR_IMPL_FILE" "$JSDOM_XHR_IMPL_BACKUP"
  css_json="$(node -e "const fs=require('fs');process.stdout.write(JSON.stringify(fs.readFileSync(process.argv[1],'utf8')))" "$css_path")"

  node - "$JSDOM_STYLE_RULES_FILE" "$css_json" <<'NODE'
const fs = require("fs");
const file = process.argv[2];
const css = process.argv[3];
let src = fs.readFileSync(file, "utf8");
const pattern = /const defaultStyleSheet = fs\.readFileSync\([\s\S]*?\);\n/;
if (!pattern.test(src)) {
  console.error("[release] jsdom patch target not found");
  process.exit(1);
}
src = src.replace(pattern, `const defaultStyleSheet = ${css};\n`);
fs.writeFileSync(file, src, "utf8");
NODE

  node - "$JSDOM_XHR_IMPL_FILE" <<'NODE'
const fs = require("fs");
const file = process.argv[2];
let src = fs.readFileSync(file, "utf8");
const pattern = /const syncWorkerFile = require\.resolve \? require\.resolve\("\.\/xhr-sync-worker\.js"\) : null;/;
if (!pattern.test(src)) {
  console.error("[release] jsdom xhr patch target not found");
  process.exit(1);
}
src = src.replace(pattern, 'const syncWorkerFile = `${__dirname}/xhr-sync-worker.js`;');
fs.writeFileSync(file, src, "utf8");
NODE

  log "patched jsdom style loader for compile-time embedding"
}

restore_jsdom_style_loader_patch() {
  if [[ -n "$JSDOM_STYLE_RULES_BACKUP" && -f "$JSDOM_STYLE_RULES_BACKUP" && -n "$JSDOM_STYLE_RULES_FILE" ]]; then
    mv "$JSDOM_STYLE_RULES_BACKUP" "$JSDOM_STYLE_RULES_FILE"
  fi
  if [[ -n "$JSDOM_XHR_IMPL_BACKUP" && -f "$JSDOM_XHR_IMPL_BACKUP" && -n "$JSDOM_XHR_IMPL_FILE" ]]; then
    mv "$JSDOM_XHR_IMPL_BACKUP" "$JSDOM_XHR_IMPL_FILE"
  fi
  log "restored jsdom compile-time patches"
}

compress_agent_bin_if_enabled() {
  local bin_path="$1"
  local target_os="$2"

  if [[ "$UPX_COMPRESS_AGENT_BIN" != "true" ]]; then
    return 0
  fi
  ensure_upx_available
  if [[ "$target_os" == "darwin" && "$UPX_ALLOW_DARWIN" != "true" ]]; then
    log "skip upx on darwin (set UPX_ALLOW_DARWIN=true to force)"
    return 0
  fi

  local before_bytes after_bytes
  before_bytes="$(wc -c < "$bin_path" | tr -d ' ')"
  read -r -a upx_flags <<< "$UPX_ARGS"
  upx "${upx_flags[@]}" "$bin_path"
  after_bytes="$(wc -c < "$bin_path" | tr -d ' ')"
  log "upx compressed agent-bin: ${before_bytes} -> ${after_bytes} bytes"
}

ensure_upx_available() {
  if command -v upx >/dev/null 2>&1; then
    return 0
  fi
  if [[ "$AUTO_INSTALL_UPX" != "true" ]]; then
    echo "[release] UPX_COMPRESS_AGENT_BIN=true but upx not found in PATH" >&2
    echo "[release] install upx or set AUTO_INSTALL_UPX=true" >&2
    exit 1
  fi

  log "upx not found; attempting auto-install"
  if [[ "$OSTYPE" == linux* ]] && command -v apt-get >/dev/null 2>&1; then
    if command -v sudo >/dev/null 2>&1; then
      sudo apt-get update -y && sudo apt-get install -y upx-ucl
    else
      apt-get update -y && apt-get install -y upx-ucl
    fi
  elif [[ "$OSTYPE" == darwin* ]] && command -v brew >/dev/null 2>&1; then
    brew install upx
  fi

  if ! command -v upx >/dev/null 2>&1; then
    echo "[release] failed to auto-install upx" >&2
    exit 1
  fi
}

gzip_embedded_web_assets() {
  local web_dir="$1"
  log "precompressing web assets (.gz)"

  while IFS= read -r -d '' file_path; do
    if [[ "$(basename "$file_path")" == ".gitignore" ]]; then
      continue
    fi
    gzip -9 -c "$file_path" > "${file_path}.gz"
    rm -f "$file_path"
  done < <(find "$web_dir" -type f -print0)
}

build_archive() {
  mkdir -p "$OUTPUT_DIR"

  local ext=""
  if [[ "$TARGET_OS" == "windows" ]]; then
    ext=".exe"
  fi

  local binary_name="memoh${ext}"
  local target_dir="$OUTPUT_DIR/memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}"
  mkdir -p "$target_dir"

  log "building binary ${TARGET_OS}/${TARGET_ARCH}"
  CGO_ENABLED=0 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" \
    go build \
    -trimpath \
    -ldflags "-s -w -X github.com/memohai/memoh/internal/version.Version=${VERSION} -X github.com/memohai/memoh/internal/version.CommitHash=${COMMIT_HASH} -X github.com/memohai/memoh/internal/version.BuildTime=${BUILD_TIME}" \
    -o "$target_dir/$binary_name" \
    "$ROOT_DIR/cmd/memoh"

  if [[ "$TARGET_OS" == "windows" ]]; then
    (cd "$OUTPUT_DIR" && zip -q -r "memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}.zip" "memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}")
  else
    tar -C "$OUTPUT_DIR" -czf "$OUTPUT_DIR/memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}.tar.gz" "memoh_${VERSION}_${TARGET_OS}_${TARGET_ARCH}"
  fi

  log "archive created (${TARGET_OS}-${TARGET_ARCH})"
}

parse_args "$@"
prepare_assets
if [[ "$PREPARE_ASSETS_ONLY" == "true" ]]; then
  log "prepare-assets only mode completed"
  exit 0
fi

build_archive
