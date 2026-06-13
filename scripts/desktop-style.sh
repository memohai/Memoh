#!/bin/sh

style="${MEMOH_DISPLAY_DESKTOP_STYLE:-macos}"
case "$style" in
  ""|0|false|False|FALSE|off|Off|OFF|none|None|NONE)
    exit 0
    ;;
esac

DISPLAY="${DISPLAY:-:99}"
GTK_A11Y="${GTK_A11Y:-1}"
export DISPLAY GTK_A11Y

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

theme_path() {
  name="$1"
  for dir in "$HOME/.themes" "$HOME/.local/share/themes" /usr/local/share/themes /usr/share/themes; do
    [ -d "$dir/$name" ] || continue
    printf '%s\n' "$dir/$name"
    return 0
  done
  return 1
}

icon_theme_path() {
  name="$1"
  for dir in "$HOME/.icons" "$HOME/.local/share/icons" /usr/local/share/icons /usr/share/icons; do
    [ -d "$dir/$name" ] || continue
    printf '%s\n' "$dir/$name"
    return 0
  done
  return 1
}

first_theme() {
  for name in "$@"; do
    theme_path "$name" >/dev/null 2>&1 || continue
    printf '%s\n' "$name"
    return 0
  done
  return 1
}

first_xfwm_theme() {
  for name in "$@"; do
    path="$(theme_path "$name" 2>/dev/null || true)"
    [ -n "$path" ] && [ -d "$path/xfwm4" ] || continue
    printf '%s\n' "$name"
    return 0
  done
  return 1
}

first_icon_theme() {
  for name in "$@"; do
    icon_theme_path "$name" >/dev/null 2>&1 || continue
    printf '%s\n' "$name"
    return 0
  done
  return 1
}

first_plank_theme() {
  for name in "$@"; do
    for dir in "$HOME/.local/share/plank/themes" "$HOME/.config/plank/themes" /usr/local/share/plank/themes /usr/share/plank/themes; do
      [ -d "$dir/$name" ] || continue
      printf '%s\n' "$name"
      return 0
    done
  done
  return 1
}

font_available() {
  has_cmd fc-match || return 1
  fc-match "$1" 2>/dev/null | grep -qi "$1"
}

first_font() {
  for name in "$@"; do
    if font_available "$name"; then
      printf '%s 10\n' "$name"
      return 0
    fi
  done
  printf 'Sans 10\n'
}

run_xsetroot() {
  color="${MEMOH_DISPLAY_DESKTOP_COLOR:-#1f2329}"
  if has_cmd xsetroot; then
    xsetroot -solid "$color" >/dev/null 2>&1 || true
    xsetroot -cursor_name left_ptr >/dev/null 2>&1 || true
  elif [ -x /opt/memoh/toolkit/display/bin/xsetroot ]; then
    /opt/memoh/toolkit/display/bin/xsetroot -solid "$color" >/dev/null 2>&1 || true
    /opt/memoh/toolkit/display/bin/xsetroot -cursor_name left_ptr >/dev/null 2>&1 || true
  fi
}

wait_for_xfconf() {
  has_cmd xfconf-query || return 1
  i=0
  while [ "$i" -lt 24 ]; do
    xfconf-query -c xsettings -l >/dev/null 2>&1 && return 0
    sleep 1
    i=$((i + 1))
  done
  return 1
}

xfconf_set() {
  channel="$1"
  property="$2"
  value_type="$3"
  value="$4"
  xfconf-query -c "$channel" -p "$property" -s "$value" >/dev/null 2>&1 && return 0
  xfconf-query -c "$channel" -p "$property" -n -t "$value_type" -s "$value" >/dev/null 2>&1 || true
}

xfconf_reset() {
  channel="$1"
  property="$2"
  xfconf-query -c "$channel" -p "$property" -r -R >/dev/null 2>&1 || true
}

xfconf_set_int_array() {
  channel="$1"
  property="$2"
  shift 2
  xfconf_reset "$channel" "$property"
  command="xfconf-query -c \"$channel\" -p \"$property\" -n -a"
  for value in "$@"; do
    command="$command -t int -s \"$value\""
  done
  eval "$command" >/dev/null 2>&1 || true
}

xfconf_replace_int_array() {
  channel="$1"
  property="$2"
  shift 2
  command="xfconf-query -c \"$channel\" -p \"$property\" -a"
  for value in "$@"; do
    command="$command -t int -s \"$value\""
  done
  eval "$command" >/dev/null 2>&1 && return 0
  xfconf_set_int_array "$channel" "$property" "$@"
}

panel_plugin_available() {
  [ -f "/usr/share/xfce4/panel/plugins/$1.desktop" ]
}

start_appmenu_registrar() {
  registrar="$(command -v appmenu-registrar 2>/dev/null || true)"
  [ -n "$registrar" ] || registrar="/usr/libexec/vala-panel/appmenu-registrar"
  [ -x "$registrar" ] || return 0
  ps -ef 2>/dev/null | grep -E '[ /]appmenu-registrar($| )' | grep -v grep >/dev/null 2>&1 && return 0
  nohup "$registrar" >/tmp/memoh-appmenu-registrar.log 2>&1 &
}

install_topbar_menu_icon() {
  icon_dir="$HOME/.local/share/icons/hicolor/scalable/apps"
  mkdir -p "$icon_dir"
  cat >"$icon_dir/memoh-logo-white.svg" <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 454.08 399.96">
  <path fill="#f5f5f7" d="M249.55,307.07c-63.11,20.61-155.69,25.5-182.38,6.68-34.57-24.37-19.05-137.12,7.42-173.36,32.66-44.71,114.75-54.31,152.82-55.97,44.88-1.96,50.44-30.49,50.44-30.49-15.46,14.29-17.33,16.35-64.32,18.23-33.97,1.36-87.49,3.88-130.26,27.67l-2.72-56.39c-.4-8.24-7.4-14.59-15.63-14.2-8.24.4-14.59,7.4-14.2,15.63l3.68,76.38c-4.98,4.93-9.62,10.35-13.82,16.35C3.89,189.99-1.8,249.58.42,297.41c.81,17.44,2.49,31.96,6.82,44.21,0,0,0,.01.01.03.6,1.19,22.35,43.42,104.6,34.64,83.45-8.9,172.88-57.76,208.36-119.64-13.8,23.41-35.2,38.86-70.67,50.43Z"/>
  <path fill="#f5f5f7" d="M429.45,110.2c-23.96-30.8-56.39-44.77-79.74-51.1l13.35-39.38c2.65-7.81-1.54-16.29-9.35-18.93h0c-7.81-2.65-16.29,1.54-18.93,9.35l-14.85,43.78c-8.49,25.06-53.64,36.67-53.64,36.67,0,0,11.72-1.65,40.22,7.62,28.5,9.29,34.21,37.45,31.07,93.85-1.36,24.71-6.6,46.33-17.36,64.57-35.48,61.88-124.9,110.75-208.36,119.64-82.26,8.78-104-33.45-104.6-34.64,2.24,6.32,5.18,12.03,9.07,17.2,13.1,17.4,32.05,24.78,47.08,28.94,81.61,22.55,233.81,7.12,244.66,5.97,24.35-2.57,51.97-5.92,79.03-25.32,5.4-3.87,22.47-18.21,39.94-46.21,25.47-40.82,44.25-158.22,2.42-212.01Z"/>
  <path fill="#f5f5f7" d="M163.41,238.79c.37.09,2.75.63,6.52,1.04,11.65,1.27,36.5,1.31,55.49-17.2,3.11-3.04,3.18-8.02.14-11.13-3.04-3.11-8.02-3.18-11.13-.14-18.85,18.38-47.15,12.17-47.44,12.11-4.23-.99-8.45,1.64-9.44,5.86-1,4.23,1.63,8.47,5.86,9.47Z"/>
  <path fill="#f5f5f7" d="M118.67,218.27c12.32-.73,22.48-9.91,24.45-22.09l2-12.31c1.56-9.59-6.18-18.15-15.87-17.57h0c-12.32.73-22.48,9.91-24.45,22.09l-2,12.31c-1.56,9.59,6.18,18.15,15.87,17.57Z"/>
  <path fill="#f5f5f7" d="M249.71,207.88h0c12.32-.73,22.48-9.91,24.45-22.09l2-12.31c1.56-9.59-6.18-18.15-15.87-17.57h0c-12.32.73-22.48,9.91-24.45,22.09l-2,12.31c-1.56,9.59,6.18,18.15,15.87,17.57Z"/>
</svg>
EOF
  if has_cmd gtk-update-icon-cache; then
    gtk-update-icon-cache -q "$HOME/.local/share/icons/hicolor" >/dev/null 2>&1 || true
  fi
}

write_panel_css() {
  mkdir -p "$HOME/.config/gtk-3.0"
  cat >"$HOME/.config/gtk-3.0/gtk.css" <<'EOF'
#XfcePanelWindow,
#XfcePanelWindow.background,
.xfce4-panel {
  background-color: rgba(28, 31, 36, 0.92);
  color: #f5f5f7;
  font-weight: 600;
}

#XfcePanelWindow button,
#XfcePanelWindow .flat,
#XfcePanelWindow menuitem {
  min-height: 22px;
  padding: 0 8px;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: #f5f5f7;
  box-shadow: none;
}

#XfcePanelWindow button:hover {
  background-color: rgba(255, 255, 255, 0.14);
}
EOF
}

write_gtk_settings() {
  theme="$1"
  icons="$2"
  cursor="$3"
  font="$4"

  mkdir -p "$HOME/.config/gtk-3.0" "$HOME/.config/gtk-4.0"
  cat >"$HOME/.config/gtk-3.0/settings.ini" <<EOF
[Settings]
gtk-theme-name=${theme}
gtk-icon-theme-name=${icons}
gtk-cursor-theme-name=${cursor}
gtk-font-name=${font}
gtk-application-prefer-dark-theme=1
gtk-toolbar-style=GTK_TOOLBAR_ICONS
gtk-button-images=1
gtk-menu-images=1
EOF
  cp "$HOME/.config/gtk-3.0/settings.ini" "$HOME/.config/gtk-4.0/settings.ini" 2>/dev/null || true
}

configure_topbar() {
  wait_for_xfconf || return 0

  appmenu_plugin="separator"
  panel_title_plugin="separator"
  if panel_plugin_available appmenu; then
    appmenu_plugin="appmenu"
    xfconf_set xsettings /Gtk/ShellShowsMenubar bool true
    xfconf_set xsettings /Gtk/ShellShowsAppmenu bool true
    xfconf_set xsettings /Gtk/Modules string "appmenu-gtk-module"
    start_appmenu_registrar
  fi
  if panel_plugin_available windowck-plugin; then
    panel_title_plugin="windowck-plugin"
  elif panel_plugin_available windowmenu; then
    panel_title_plugin="windowmenu"
  fi

  xfconf_replace_int_array xfce4-panel /panels 1
  xfconf_reset xfce4-panel /panels/panel-2
  xfconf_set_int_array xfce4-panel /panels/panel-1/plugin-ids 101 102 103 104 105 106 107 108
  xfconf_reset xfce4-panel /plugins
  xfconf_reset xfce4-panel /plugins/plugin-109

  xfconf_set xfce4-panel /plugins/plugin-101 string applicationsmenu
  install_topbar_menu_icon
  xfconf_set xfce4-panel /plugins/plugin-101/show-button-title bool false
  xfconf_set xfce4-panel /plugins/plugin-101/button-icon string "memoh-logo-white"
  xfconf_set xfce4-panel /plugins/plugin-101/show-menu-icons bool true
  xfconf_set xfce4-panel /plugins/plugin-101/show-generic-names bool false
  xfconf_set xfce4-panel /plugins/plugin-101/small bool true

  xfconf_set xfce4-panel /plugins/plugin-102 string "$panel_title_plugin"
  xfconf_set xfce4-panel /plugins/plugin-102/active_window bool true
  xfconf_set xfce4-panel /plugins/plugin-102/only_maximized bool false
  xfconf_set xfce4-panel /plugins/plugin-102/show_on_desktop bool true
  xfconf_set xfce4-panel /plugins/plugin-102/sync_wm_font bool true
  xfconf_set xfce4-panel /plugins/plugin-102/full_name bool false
  xfconf_set xfce4-panel /plugins/plugin-102/title_padding int 6
  xfconf_set xfce4-panel /plugins/plugin-102/title_size int 28
  xfconf_set xfce4-panel /plugins/plugin-102/title_alignment int 0

  xfconf_set xfce4-panel /plugins/plugin-103 string "$appmenu_plugin"
  xfconf_set xfce4-panel /plugins/plugin-103/compact-mode bool false
  xfconf_set xfce4-panel /plugins/plugin-103/bold-application-name bool true

  xfconf_set xfce4-panel /plugins/plugin-104 string separator
  xfconf_set xfce4-panel /plugins/plugin-104/expand bool true
  xfconf_set xfce4-panel /plugins/plugin-104/style int 0

  xfconf_set xfce4-panel /plugins/plugin-105 string systray
  xfconf_set xfce4-panel /plugins/plugin-105/square-icons bool true

  xfconf_set xfce4-panel /plugins/plugin-106 string pulseaudio
  xfconf_set xfce4-panel /plugins/plugin-106/enable-keyboard-shortcuts bool true
  xfconf_set xfce4-panel /plugins/plugin-106/show-notifications bool false

  xfconf_set xfce4-panel /plugins/plugin-107 string notification-plugin

  xfconf_set xfce4-panel /plugins/plugin-108 string clock
  xfconf_set xfce4-panel /plugins/plugin-108/mode int 2
  xfconf_set xfce4-panel /plugins/plugin-108/digital-layout int 3
  xfconf_set xfce4-panel /plugins/plugin-108/digital-time-format string "%a %b %-d  %H:%M"
  xfconf_set xfce4-panel /plugins/plugin-108/tooltip-format string "%A, %B %-d, %Y"
}

first_wallpaper() {
  for file in \
    "$HOME/.local/share/backgrounds/WhiteSur/WhiteSur-dark.jpg" \
    "$HOME/.local/share/backgrounds/WhiteSur/WhiteSur.jpg" \
    "$HOME/.local/share/backgrounds/WhiteSur/Monterey-dark.jpg" \
    "$HOME/.local/share/backgrounds/WhiteSur/Ventura-dark.jpg" \
    "$HOME/.local/share/backgrounds/WhiteSur/Sonoma-dark.jpg" \
    "$HOME/.local/share/backgrounds/WhiteSur/WhiteSur-light.jpg" \
    "$HOME/.local/share/backgrounds/WhiteSur/Monterey.jpg"; do
    [ -f "$file" ] || continue
    printf '%s\n' "$file"
    return 0
  done

  for dir in \
    "$HOME/.local/share/backgrounds/WhiteSur" \
    "$HOME/.local/share/backgrounds/whitesur" \
    "$HOME/.local/share/backgrounds" \
    /usr/local/share/backgrounds \
    /usr/share/backgrounds; do
    [ -d "$dir" ] || continue
    found="$(find "$dir" -maxdepth 2 -type f \( -iname '*.jpg' -o -iname '*.jpeg' -o -iname '*.png' \) 2>/dev/null | grep -Ei 'WhiteSur|Big[ -]?Sur|Monterey|Ventura|macOS|Mojave' | sort | head -n 1 || true)"
    [ -n "$found" ] && { printf '%s\n' "$found"; return 0; }
  done
  return 1
}

configure_wallpaper() {
  wallpaper="$(first_wallpaper 2>/dev/null || true)"
  [ -n "$wallpaper" ] || return 0
  wait_for_xfconf || return 0

  props="$(xfconf-query -c xfce4-desktop -l 2>/dev/null | grep '/last-image$' || true)"
  if [ -z "$props" ]; then
    props="/backdrop/screen0/monitor0/workspace0/last-image
/backdrop/screen0/monitorVNC-0/workspace0/last-image
/backdrop/screen0/monitorVirtual1/workspace0/last-image
/backdrop/screen0/monitordefault/workspace0/last-image"
  fi

  printf '%s\n' "$props" | while IFS= read -r prop; do
    [ -n "$prop" ] || continue
    base="${prop%/last-image}"
    xfconf_set xfce4-desktop "$prop" string "$wallpaper"
    xfconf_set xfce4-desktop "$base/image-style" int 5
    xfconf_set xfce4-desktop "$base/color-style" int 0
  done
}

configure_xfce() {
  wait_for_xfconf || return 0

  gtk_theme="${MEMOH_DISPLAY_GTK_THEME:-$(first_theme WhiteSur-Dark-solid-alt WhiteSur-Dark-alt WhiteSur-Dark-solid WhiteSur-Dark WhiteSur-dark-solid-alt WhiteSur-dark-alt WhiteSur-dark-solid WhiteSur-dark WhiteSur-Light-solid-alt WhiteSur-Light-alt WhiteSur-Light-solid WhiteSur-Light WhiteSur Arc-Dark Arc Adwaita-dark Adwaita 2>/dev/null || true)}"
  xfwm_theme="${MEMOH_DISPLAY_XFWM_THEME:-$(first_xfwm_theme "$gtk_theme" WhiteSur-Dark-solid-alt WhiteSur-Dark-alt WhiteSur-Dark-solid WhiteSur-Dark WhiteSur-dark-solid-alt WhiteSur-dark-alt WhiteSur-dark-solid WhiteSur-dark WhiteSur-Light-solid-alt WhiteSur-Light-alt WhiteSur-Light-solid WhiteSur-Light WhiteSur Arc-Dark Arc Adwaita-dark Adwaita 2>/dev/null || true)}"
  icons="${MEMOH_DISPLAY_ICON_THEME:-$(first_icon_theme WhiteSur-dark WhiteSur-Dark WhiteSur WhiteSur-light WhiteSur-Light Cupertino McMojave-circle-dark McMojave-circle Papirus-Dark Papirus Adwaita 2>/dev/null || true)}"
  cursor="${MEMOH_DISPLAY_CURSOR_THEME:-$(first_icon_theme WhiteSur-cursors WhiteSur Bibata-Modern-Classic Bibata-Modern-Ice Adwaita 2>/dev/null || true)}"
  font="$(first_font Inter 'SF Pro Display' 'Noto Sans' Cantarell Sans)"

  [ -n "$gtk_theme" ] && xfconf_set xsettings /Net/ThemeName string "$gtk_theme"
  [ -n "$xfwm_theme" ] && xfconf_set xfwm4 /general/theme string "$xfwm_theme"
  [ -n "$icons" ] && xfconf_set xsettings /Net/IconThemeName string "$icons"
  [ -n "$cursor" ] && xfconf_set xsettings /Gtk/CursorThemeName string "$cursor"

  xfconf_set xsettings /Gtk/FontName string "$font"
  xfconf_set xsettings /Gtk/MonospaceFontName string "Monospace 10"
  xfconf_set xsettings /Gtk/ToolbarStyle string "icons"
  xfconf_set xsettings /Gtk/MenuImages bool true
  xfconf_set xsettings /Gtk/ButtonImages bool true
  [ -n "$gtk_theme" ] && [ -n "$icons" ] && [ -n "$cursor" ] && write_gtk_settings "$gtk_theme" "$icons" "$cursor" "$font"
  write_panel_css

  # macOS order: close, minimize, maximize on the left; the WhiteSur xfwm4
  # assets render these as traffic-light buttons.
  xfconf_set xfwm4 /general/button_layout string "CHM|"
  xfconf_set xfwm4 /general/title_alignment string "center"
  xfconf_set xfwm4 /general/workspace_count int 1
  xfconf_set xfwm4 /general/use_compositing bool true
  xfconf_set xfwm4 /general/frame_opacity int 100
  xfconf_set xfwm4 /general/inactive_opacity int 94
  xfconf_set xfwm4 /general/title_font string "$font"

  xfconf_set xfce4-desktop /desktop-icons/style int 0

  xfconf_set xfce4-panel /panels/panel-1/mode int 0
  xfconf_set xfce4-panel /panels/panel-1/position string "p=6;x=0;y=0"
  xfconf_set xfce4-panel /panels/panel-1/position-locked bool true
  xfconf_set xfce4-panel /panels/panel-1/size int 26
  xfconf_set xfce4-panel /panels/panel-1/length int 100
  xfconf_set xfce4-panel /panels/panel-1/length-adjust bool false
  xfconf_set xfce4-panel /panels/panel-1/autohide-behavior int 0
  configure_topbar
}

write_desktop_file() {
  file="$1"
  name="$2"
  icon="$3"
  exec_cmd="$4"
  [ -n "$exec_cmd" ] || return 1
  mkdir -p "$(dirname "$file")"
  cat >"$file" <<EOF
[Desktop Entry]
Type=Application
Name=${name}
Icon=${icon}
Exec=${exec_cmd}
Terminal=false
Categories=Utility;
EOF
}

first_existing_file() {
  for file in "$@"; do
    [ -f "$file" ] || continue
    printf '%s\n' "$file"
    return 0
  done
  return 1
}

write_chromium_wrapper() {
  browser="$1"
  [ -n "$browser" ] || return 1

  wrapper="$HOME/.local/bin/memoh-chromium"
  mkdir -p "$(dirname "$wrapper")"
  cat >"$wrapper" <<EOF
#!/bin/sh
browser="${browser}"
profile="\${MEMOH_DISPLAY_CHROMIUM_PROFILE:-/tmp/memoh-display-browser}"

if [ "\$#" -eq 0 ]; then
  set -- about:blank
fi

mkdir -p "\$profile"
if ! ps -ef 2>/dev/null | grep -F -- "--user-data-dir=\$profile" | grep -v grep >/dev/null 2>&1; then
  rm -f "\$profile"/SingletonLock "\$profile"/SingletonSocket "\$profile"/SingletonCookie
fi

exec "\$browser" \\
  --no-sandbox \\
  --disable-dev-shm-usage \\
  --disable-gpu \\
  --no-first-run \\
  --no-default-browser-check \\
  --force-renderer-accessibility \\
  --remote-debugging-address=127.0.0.1 \\
  --remote-debugging-port=9222 \\
  --remote-allow-origins='*' \\
  --user-data-dir="\$profile" \\
  "\$@"
EOF
  chmod 0755 "$wrapper" 2>/dev/null || true
  printf '%s\n' "$wrapper"
}

browser_desktop_file() {
  browser="$(command -v chromium 2>/dev/null || command -v chromium-browser 2>/dev/null || true)"
  if [ -n "$browser" ]; then
    wrapper="$(write_chromium_wrapper "$browser" 2>/dev/null || true)"
    [ -n "$wrapper" ] || wrapper="$browser"
    file="$HOME/.local/share/applications/memoh-chromium.desktop"
    write_desktop_file "$file" "Chromium" "chromium" "$wrapper"
    printf '%s\n' "$file"
    return 0
  fi

  first_existing_file \
    /usr/share/applications/chromium.desktop \
    /usr/share/applications/chromium-browser.desktop \
    /usr/local/share/applications/chromium.desktop \
    /usr/local/share/applications/chromium-browser.desktop
}

terminal_desktop_file() {
  first_existing_file \
    /usr/share/applications/xfce4-terminal.desktop \
    /usr/share/applications/org.xfce.terminal.desktop \
    /usr/share/applications/debian-xterm.desktop \
    /usr/local/share/applications/xfce4-terminal.desktop && return 0

  terminal="$(command -v xfce4-terminal 2>/dev/null || command -v xterm 2>/dev/null || true)"
  [ -n "$terminal" ] || return 1
  file="$HOME/.local/share/applications/memoh-terminal.desktop"
  write_desktop_file "$file" "Terminal" "utilities-terminal" "$terminal"
  printf '%s\n' "$file"
}

files_desktop_file() {
  first_existing_file \
    /usr/share/applications/thunar.desktop \
    /usr/share/applications/org.xfce.Thunar.desktop \
    /usr/local/share/applications/thunar.desktop && return 0
  return 1
}

write_dockitem() {
  name="$1"
  launcher="$2"
  [ -n "$launcher" ] && [ -f "$launcher" ] || return 0
  mkdir -p "$HOME/.config/plank/dock1/launchers"
  cat >"$HOME/.config/plank/dock1/launchers/$name.dockitem" <<EOF
[PlankDockItemPreferences]
Launcher=file://${launcher}
EOF
}

gsettings_set_plank() {
  has_cmd gsettings || return 0
  key="$1"
  shift
  gsettings set net.launchpad.plank.dock.settings:/net/launchpad/plank/docks/dock1/ "$key" "$@" >/dev/null 2>&1 || true
}

configure_plank() {
  has_cmd plank || return 0

  dock_theme="${MEMOH_DISPLAY_DOCK_THEME:-$(first_plank_theme 'macOS Dark' 'Big Sur Dark' 'Mojave Dark' 'macOS Night Owl' 'macOS Light' 'Big Sur Light' Default 2>/dev/null || true)}"
  [ -n "$dock_theme" ] || dock_theme="Default"

  browser_file="$(browser_desktop_file 2>/dev/null || true)"
  terminal_file="$(terminal_desktop_file 2>/dev/null || true)"
  files_file="$(files_desktop_file 2>/dev/null || true)"

  rm -rf "$HOME/.config/plank/dock1/launchers"
  write_dockitem 01-browser "$browser_file"
  write_dockitem 02-terminal "$terminal_file"
  write_dockitem 03-files "$files_file"

  mkdir -p "$HOME/.config/plank/dock1"
  cat >"$HOME/.config/plank/dock1/settings" <<EOF
[PlankDockPreferences]
CurrentWorkspaceOnly=false
IconSize=56
HideMode=1
UnhideDelay=0
Monitor=
DockItems=01-browser.dockitem;02-terminal.dockitem;03-files.dockitem;
Position=3
Offset=0
Theme=${dock_theme}
Alignment=3
ItemsAlignment=3
LockItems=false
PressureReveal=false
PinnedOnly=false
ZoomEnabled=true
ZoomPercent=150
EOF

  gsettings_set_plank theme "$dock_theme"
  gsettings_set_plank icon-size 56
  gsettings_set_plank position "'bottom'"
  gsettings_set_plank alignment "'center'"
  gsettings_set_plank items-alignment "'center'"
  gsettings_set_plank hide-mode "'intelligent'"
  gsettings_set_plank zoom-enabled true
  gsettings_set_plank zoom-percent 150
}

restart_plank() {
  has_cmd plank || return 0
  pids="$(ps -ef 2>/dev/null | grep -E '[ /]plank($| )' | grep -v grep | awk '{print $2}' || true)"
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  [ -n "$pids" ] && sleep 1
  nohup plank >/tmp/memoh-plank.log 2>&1 &
}

restart_xfce_panel() {
  has_cmd xfce4-panel || return 0
  pids="$(ps -ef 2>/dev/null | grep -E '[ /]xfce4-panel($| )' | grep -v grep | awk '{print $2}' || true)"
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  [ -n "$pids" ] && sleep 1
  pids="$(ps -ef 2>/dev/null | grep -E '[ /]xfce4-panel($| )' | grep -v grep | awk '{print $2}' || true)"
  for pid in $pids; do
    kill -9 "$pid" 2>/dev/null || true
  done
  nohup xfce4-panel >/tmp/memoh-xfce-panel.log 2>&1 &
}

reload_xfce_components() {
  if has_cmd xfwm4; then
    nohup xfwm4 --replace >/tmp/memoh-xfwm4-replace.log 2>&1 &
  fi
  restart_xfce_panel
}

run_xsetroot
configure_xfce
configure_wallpaper
configure_plank
restart_plank
reload_xfce_components

exit 0
