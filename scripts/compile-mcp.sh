#!/usr/bin/env sh
set -e

APP_DIR=${APP_DIR:-/app}
BIN_NAME=${BIN_NAME:-mcp}
STOP_SIGNAL=${STOP_SIGNAL:-TERM}
CONTAINER_NAME=${CONTAINER_NAME:-}
TARGET_OS=${TARGET_OS:-linux}
TARGET_ARCH=${TARGET_ARCH:-}

if [ -z "$TARGET_ARCH" ]; then
  case "$(uname -m)" in
    arm64|aarch64)
      TARGET_ARCH=arm64
      ;;
    x86_64|amd64)
      TARGET_ARCH=amd64
      ;;
    *)
      TARGET_ARCH=amd64
      ;;
  esac
fi

mkdir -p "$APP_DIR"

GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" go build -trimpath -ldflags "-s -w" -o "${APP_DIR}/${BIN_NAME}.new" ./cmd/mcp
mv -f "${APP_DIR}/${BIN_NAME}.new" "${APP_DIR}/${BIN_NAME}"

if [ -n "$CONTAINER_NAME" ]; then
  nerdctl kill -s "$STOP_SIGNAL" "$CONTAINER_NAME"
else
  echo "CONTAINER_NAME is empty; skip sending stop signal."
fi
