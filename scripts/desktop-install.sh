#!/bin/sh

install_debian() {
  has_cmd apt-get || { echo "This image looks Debian-like but apt-get is unavailable. Install the Memoh workspace toolkit or use a Debian/Alpine image." >&2; exit 1; }
  export DEBIAN_FRONTEND=noninteractive
  progress 18 system "Detected Debian workspace"
  progress 24 installing "Updating package index"
  apt-get update
  if ! has_cmd apt-extracttemplates; then
    apt-get install -y --no-install-recommends apt-utils
  fi
  progress 42 installing "Installing VNC, desktop, and CJK fonts"
  apt-get install -y --no-install-recommends ca-certificates curl gnupg dbus-x11 x11-xserver-utils xterm xfce4 tigervnc-standalone-server fontconfig fonts-dejavu fonts-noto-cjk fonts-noto-color-emoji procps
  if ! find_browser >/dev/null 2>&1; then
    progress 66 browser "Installing browser"
    if apt-get install -y --no-install-recommends chromium || apt-get install -y --no-install-recommends chromium-browser; then
      return 0
    fi
    arch="$(dpkg --print-architecture)"
    [ "$arch" = "amd64" ] || return 1
    install -d -m 0755 /etc/apt/keyrings
    rm -f /etc/apt/keyrings/google-chrome.gpg
    curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --batch --yes --dearmor -o /etc/apt/keyrings/google-chrome.gpg
    echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/google-chrome.gpg] http://dl.google.com/linux/chrome/deb/ stable main" >/etc/apt/sources.list.d/google-chrome.list
    apt-get update
    apt-get install -y --no-install-recommends google-chrome-stable
  fi
}

install_alpine() {
  has_cmd apk || { echo "This image looks Alpine-like but apk is unavailable. Install the Memoh workspace toolkit or use a Debian/Alpine image." >&2; exit 1; }
  progress 18 system "Detected Alpine workspace"
  progress 42 installing "Installing VNC, desktop, browser, and CJK fonts"
  apk add --no-cache tigervnc xkeyboard-config xfce4 xfce4-terminal dbus-x11 xterm chromium fontconfig ttf-dejavu font-noto-cjk font-noto-emoji
}
