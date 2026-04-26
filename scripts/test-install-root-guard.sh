#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

FAKEBIN="$TMPDIR/bin"
mkdir -p "$FAKEBIN"

cat > "$FAKEBIN/id" <<'EOF'
#!/bin/sh
if [ "$1" = "-u" ]; then
  printf '0\n'
  exit 0
fi
exit 1
EOF
chmod +x "$FAKEBIN/id"

cat > "$FAKEBIN/docker" <<'EOF'
#!/bin/sh
[ -n "${DOCKER_MARKER:-}" ] && echo invoked >> "$DOCKER_MARKER"
exit 42
EOF
chmod +x "$FAKEBIN/docker"

OUTPUT="$TMPDIR/output.txt"
DOCKER_MARKER="$TMPDIR/docker-marker.txt"
set +e
PATH="$FAKEBIN:/usr/bin:/bin" HOME="$TMPDIR/home" DOCKER_MARKER="$DOCKER_MARKER" sh "$ROOT/scripts/install.sh" --yes >"$OUTPUT" 2>&1
STATUS=$?
set -e

if [ "$STATUS" -eq 0 ]; then
  echo "expected root installer invocation to fail" >&2
  cat "$OUTPUT" >&2
  exit 1
fi

if [ -f "$DOCKER_MARKER" ]; then
  echo "installer reached Docker before rejecting root execution" >&2
  cat "$OUTPUT" >&2
  exit 1
fi

if ! grep -q "Do not run this installer as root" "$OUTPUT"; then
  echo "expected root execution warning was not printed" >&2
  cat "$OUTPUT" >&2
  exit 1
fi

OPT_IN_OUTPUT="$TMPDIR/opt-in-output.txt"
set +e
PATH="$FAKEBIN:/usr/bin:/bin" HOME="$TMPDIR/home" DOCKER_MARKER="$DOCKER_MARKER" MEMOH_ALLOW_ROOT_INSTALL=true sh "$ROOT/scripts/install.sh" --yes >"$OPT_IN_OUTPUT" 2>&1
OPT_IN_STATUS=$?
set -e

if [ "$OPT_IN_STATUS" -eq 0 ]; then
  echo "expected opt-in root invocation to stop at fake Docker" >&2
  cat "$OPT_IN_OUTPUT" >&2
  exit 1
fi

if [ ! -f "$DOCKER_MARKER" ]; then
  echo "explicit root opt-in did not continue to Docker checks" >&2
  cat "$OPT_IN_OUTPUT" >&2
  exit 1
fi
