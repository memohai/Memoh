#!/bin/sh
# Copy binary to writable layer so it survives snapshot restores.
[ -e /app/mcp ] || { mkdir -p /app; [ -f /opt/mcp ] && cp -a /opt/mcp /app/mcp 2>/dev/null || true; }
if [ -x /app/mcp ]; then exec /app/mcp "$@"; fi
exec /opt/mcp "$@"
