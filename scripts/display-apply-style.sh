#!/bin/sh

style_version='__MEMOH_DISPLAY_DESKTOP_STYLE_VERSION__'
style_config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/memoh"
style_marker="$style_config_dir/display-style.version"
style_lock=/tmp/memoh-desktop-style.lock
style_lock_stale_seconds=60
style_log=/tmp/memoh-desktop-style.log

style_enabled() {
  style="${MEMOH_DISPLAY_DESKTOP_STYLE:-macos}"
  case "$style" in
    ""|0|false|False|FALSE|off|Off|OFF|none|None|NONE) return 1 ;;
    *) return 0 ;;
  esac
}

style_is_current() {
  [ -r "$style_marker" ] && [ "$(cat "$style_marker" 2>/dev/null || true)" = "$style_version" ]
}

if [ "${1:-}" = "--check" ]; then
  style_enabled || exit 0
  style_is_current
  exit $?
fi

mode="${1:---if-needed}"
style_enabled || exit 0
if { [ "$mode" = "--if-needed" ] || [ "$mode" = "--ensure" ]; } && style_is_current; then
  exit 0
fi

now_seconds() {
  date +%s 2>/dev/null || printf '0\n'
}

lock_file_mtime_seconds() {
  stat -c %Y "$1" 2>/dev/null || stat -f %m "$1" 2>/dev/null || printf '0\n'
}

write_style_lock_owner() {
  printf '%s\n' "$$" >"$style_lock/pid" 2>/dev/null || true
  now_seconds >"$style_lock/created_at" 2>/dev/null || true
}

try_acquire_style_lock() {
  if mkdir "$style_lock" 2>/dev/null; then
    write_style_lock_owner
    return 0
  fi
  return 1
}

read_style_lock_owner_pid() {
  cat "$style_lock/pid" 2>/dev/null || true
}

style_lock_age_seconds() {
  now="$(now_seconds)"
  created="$(cat "$style_lock/created_at" 2>/dev/null || true)"
  case "$created" in
    ""|*[!0-9]*) created="$(lock_file_mtime_seconds "$style_lock")" ;;
  esac
  case "$now:$created" in
    *[!0-9:]*|0:*|*:0) printf '0\n' ;;
    *) printf '%s\n' "$((now - created))" ;;
  esac
}

cleanup_stale_style_lock() {
  [ -d "$style_lock" ] || return 1
  owner_pid="$(read_style_lock_owner_pid)"
  stale_reason=
  case "$owner_pid" in
    ""|*[!0-9]*)
      age="$(style_lock_age_seconds)"
      if [ "$age" -ge "$style_lock_stale_seconds" ]; then
        stale_reason="missing owner metadata for ${age}s"
      fi
      ;;
    *)
      if ! kill -0 "$owner_pid" 2>/dev/null && ! ps -p "$owner_pid" >/dev/null 2>&1; then
        stale_reason="owner pid $owner_pid is not running"
      fi
      ;;
  esac
  [ -n "$stale_reason" ] || return 1
  echo "Removing stale desktop style lock: $stale_reason." >&2
  rm -rf "$style_lock" 2>/dev/null || true
}

acquire_style_lock() {
  wait_seconds="$1"
  if try_acquire_style_lock; then
    return 0
  fi
  i=0
  while [ "$i" -lt "$wait_seconds" ]; do
    if [ "$mode" = "--if-needed" ] && style_is_current; then
      exit 0
    fi
    cleanup_stale_style_lock || true
    if try_acquire_style_lock; then
      return 0
    fi
    sleep 1
    i=$((i + 1))
  done
  cleanup_stale_style_lock || true
  try_acquire_style_lock
}

release_style_lock() {
  owner_pid="$(read_style_lock_owner_pid)"
  if [ "$owner_pid" = "$$" ]; then
    rm -rf "$style_lock" 2>/dev/null || true
  else
    rmdir "$style_lock" 2>/dev/null || true
  fi
}

wait_seconds=30
case "$mode" in
  --ensure|--force) wait_seconds=120 ;;
esac
if ! acquire_style_lock "$wait_seconds"; then
  if [ "$mode" = "--if-needed" ]; then
    echo "Another desktop style apply is running; skipping retry." >&2
    exit 0
  fi
  echo "Another desktop style apply is still running." >&2
  exit 1
fi
trap 'release_style_lock' EXIT INT TERM

if { [ "$mode" = "--if-needed" ] || [ "$mode" = "--ensure" ]; } && style_is_current; then
  exit 0
fi

progress() { :; }
has_cmd() { command -v "$1" >/dev/null 2>&1; }
os_like() {
  if [ -r /etc/os-release ]; then
    . /etc/os-release
    printf '%s %s\n' "${ID:-}" "${ID_LIKE:-}"
    return
  fi
  printf unknown
}
is_debian_like() {
  case " $(os_like) " in
    *" debian "*|*" ubuntu "*) return 0 ;;
    *) return 1 ;;
  esac
}
is_alpine() {
  case " $(os_like) " in
    *" alpine "*) return 0 ;;
    *) return 1 ;;
  esac
}

. /tmp/memoh-desktop-install.sh

install_style_extras_for_current_os
rm -f "$style_log"
if /bin/sh /tmp/memoh-desktop-style.sh >"$style_log" 2>&1; then
  mkdir -p "$style_config_dir"
  printf '%s\n' "$style_version" >"$style_marker"
  exit 0
fi

status=$?
echo "Desktop style apply failed with exit code $status. Log tail:" >&2
tail -n 80 "$style_log" >&2 2>/dev/null || true
exit "$status"
