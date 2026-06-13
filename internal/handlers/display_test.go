package handlers

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestResolveDisplayHostIPsStripsPorts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		want []string
	}{
		{name: "plain ipv4", host: "100.123.2.67", want: []string{"100.123.2.67"}},
		{name: "ipv4 port", host: "100.123.2.67:8082", want: []string{"100.123.2.67"}},
		{name: "bracketed ipv6 port", host: "[::1]:8082", want: []string{"::1"}},
		{name: "plain ipv6", host: "::1", want: []string{"::1"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveDisplayHostIPs(context.Background(), tt.host); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolveDisplayHostIPs(%q) = %#v, want %#v", tt.host, got, tt.want)
			}
		})
	}
}

func TestFirstHeaderValue(t *testing.T) {
	t.Parallel()

	got := firstHeaderValue("100.123.2.67, 10.0.0.2")
	if got != "100.123.2.67" {
		t.Fatalf("firstHeaderValue returned %q", got)
	}
}

func TestDisplayPrepareCommandInjectsInstallScript(t *testing.T) {
	t.Parallel()

	cmd := displayPrepareCommand()
	if !strings.Contains(cmd, "cat >/tmp/memoh-desktop-install.sh") {
		t.Fatal("display prepare command must inject the install script")
	}
	if !strings.Contains(cmd, "cat >/tmp/memoh-desktop-style.sh") {
		t.Fatal("display prepare command must inject the desktop style script")
	}
	if !strings.Contains(cmd, ". /tmp/memoh-desktop-install.sh") {
		t.Fatal("display prepare command must source the injected install script")
	}
	if !strings.Contains(cmd, "install_debian()") || !strings.Contains(cmd, "install_alpine()") {
		t.Fatal("injected install script must define Debian and Alpine installers")
	}
	if !strings.Contains(cmd, "configure_plank()") || !strings.Contains(cmd, "WhiteSur-Dark") {
		t.Fatal("injected style script must define macOS-like desktop styling")
	}
	if !strings.Contains(cmd, "configure_topbar()") || !strings.Contains(cmd, "windowck-plugin") || !strings.Contains(cmd, "appmenu") {
		t.Fatal("injected style script must define macOS-like topbar styling")
	}
	if !strings.Contains(cmd, "memoh-logo-white") || strings.Contains(cmd, "memoh-apple-symbol") {
		t.Fatal("injected style script must use the white Memoh logo for the topbar menu icon")
	}
	if !strings.Contains(cmd, "memoh_logo_png_base64()") ||
		!strings.Contains(cmd, "memoh-logo-white.png") ||
		!strings.Contains(cmd, "/usr/share/icons/hicolor/48x48/apps") ||
		!strings.Contains(cmd, "base64 -d") {
		t.Fatal("injected style script must install a PNG Memoh logo fallback for panels without SVG icon loading")
	}
	if !strings.Contains(cmd, "xfconf_replace_int_array xfce4-panel /panels 1") || !strings.Contains(cmd, "restart_xfce_panel()") {
		t.Fatal("injected style script must remove the default Xfce bottom panel")
	}
	if !strings.Contains(cmd, "xsetroot -cursor_name left_ptr") || !strings.Contains(cmd, "nohup xfwm4 --replace") {
		t.Fatal("injected style script must restore the Xfce window manager and pointer cursor")
	}
	if !strings.Contains(cmd, "plugin-ids 101 102 103 104 105 106 107 108") ||
		!strings.Contains(cmd, "xfconf_reset xfce4-panel /plugins/plugin-109") ||
		strings.Contains(cmd, "plugin-109 string actions") {
		t.Fatal("injected style script must omit the Xfce actions menu with logout and power options")
	}
	if !strings.Contains(cmd, `write_chromium_wrapper "$browser"`) ||
		!strings.Contains(cmd, `write_desktop_file "$file" "Chromium" "chromium" "$wrapper"`) ||
		!strings.Contains(cmd, `--user-data-dir="\$profile"`) ||
		!strings.Contains(cmd, `rm -f "\$profile"/SingletonLock`) ||
		strings.Contains(cmd, `write_desktop_file "$file" "Browser" "web-browser"`) {
		t.Fatal("injected style script must pin the dock browser launcher to a Chromium wrapper with an isolated profile")
	}
	terminalIndex := strings.Index(cmd, `terminal="$(command -v xfce4-terminal`)
	terminalLauncherIndex := strings.Index(cmd, `write_desktop_file "$file" "Terminal" "utilities-terminal" "$terminal"`)
	xtermFallbackIndex := strings.Index(cmd, `/usr/share/applications/debian-xterm.desktop`)
	if terminalIndex < 0 || terminalLauncherIndex < terminalIndex || xtermFallbackIndex < terminalLauncherIndex ||
		strings.Contains(cmd, `write_desktop_file "$file" "XTerm" "xterm" "$terminal"`) {
		t.Fatal("injected style script must prefer a custom terminal launcher with the themed terminal icon over Debian's mini.xterm desktop file")
	}
	filesIndex := strings.Index(cmd, `files_desktop_file()`)
	filesLauncherIndex := strings.Index(cmd, `write_desktop_file "$file" "Files" "$icon"`)
	thunarFallbackIndex := strings.Index(cmd, `/usr/share/applications/thunar.desktop`)
	if filesIndex < 0 || filesLauncherIndex < filesIndex || thunarFallbackIndex < filesLauncherIndex ||
		!strings.Contains(cmd, "Memoh-WhiteSur-dark") ||
		!strings.Contains(cmd, "global_theme_dir") ||
		!strings.Contains(cmd, "install_file_manager_icon_aliases") ||
		!strings.Contains(cmd, "file_manager_icon_path()") ||
		!strings.Contains(cmd, "folder-blue.svg") ||
		!strings.Contains(cmd, "org.xfce.filemanager") ||
		!strings.Contains(cmd, "org.xfce.thunar") ||
		!strings.Contains(cmd, "inode-directory") ||
		!strings.Contains(cmd, "text-x-generic.svg") ||
		!strings.Contains(cmd, `require_xfconf_value xsettings /Net/IconThemeName Memoh-WhiteSur-dark`) ||
		!strings.Contains(cmd, "restart_file_manager()") ||
		strings.Contains(cmd, "memoh_files_icon_png_base64") ||
		strings.Contains(cmd, "write_memoh_files_icon_png") {
		t.Fatal("injected style script must use a custom Files launcher and a WhiteSur-backed icon theme overlay")
	}
	if !strings.Contains(cmd, "install_style_extras_for_current_os") {
		t.Fatal("display prepare must install styling assets even when core display packages already exist")
	}
	if !strings.Contains(cmd, "cat >/tmp/memoh-desktop-apply-style.sh") {
		t.Fatal("display prepare command must inject the shared desktop style apply helper")
	}
	if !strings.Contains(displayPrepareMainCommand, `desktop_style_current()`) ||
		!strings.Contains(displayPrepareMainCommand, `/bin/sh /tmp/memoh-desktop-apply-style.sh --check`) {
		t.Fatal("display prepare readiness must include the desktop style marker")
	}
	if strings.Contains(displayPrepareMainCommand, `nohup /bin/sh /tmp/memoh-desktop-style.sh`) {
		t.Fatal("display prepare must not apply desktop style asynchronously")
	}
	styleIndex := strings.Index(displayPrepareMainCommand, `progress 90 styling "Applying desktop style"`)
	browserIndex := strings.Index(displayPrepareMainCommand, `progress 94 browser "Launching browser"`)
	completeIndex := strings.LastIndex(displayPrepareMainCommand, "\ncomplete\nexit 0")
	if styleIndex < 0 || browserIndex < 0 || styleIndex > browserIndex || browserIndex > completeIndex {
		t.Fatal("display prepare must apply desktop style synchronously before browser launch and completion")
	}
	if !strings.Contains(displayPrepareMainCommand, `/bin/sh /tmp/memoh-desktop-apply-style.sh --ensure`) {
		t.Fatal("display prepare must ensure the current style version before reporting ready")
	}
	if !strings.Contains(cmd, "SUDO_USER") || !strings.Contains(cmd, "sudo git unzip bash") {
		t.Fatal("display prepare must support WhiteSur's installer in non-login root containers")
	}
	if !strings.Contains(cmd, "MEMOH_DISPLAY_INSTALL_ASSETS_GLOBAL") ||
		!strings.Contains(cmd, "/usr/local/share/themes") ||
		!strings.Contains(cmd, "/usr/local/share/icons") ||
		!strings.Contains(cmd, "/usr/local/share/backgrounds/WhiteSur") ||
		!strings.Contains(cmd, "/usr/local/share/plank/themes") {
		t.Fatal("display prepare must support image-baked desktop style assets in global paths")
	}
	if !strings.Contains(cmd, "librsvg2-common") {
		t.Fatal("display prepare must install SVG icon loading support for themed icon assets")
	}
	if !strings.Contains(cmd, "xfce4-appmenu-plugin") || !strings.Contains(cmd, "xfce4-windowck-plugin") || !strings.Contains(cmd, "appmenu-gtk3-module") {
		t.Fatal("display prepare must install macOS-like topbar plugins when available")
	}
	if topbarIndex := strings.Index(cmd, "configure_topbar\n  xfconf_set xfce4-panel /panels/panel-1/mode"); topbarIndex < 0 {
		t.Fatal("injected style script must set panel geometry after rebuilding the topbar panel")
	}
	if !strings.Contains(cmd, "if verify_style; then\n  exit 0\nfi\nexit 1") {
		t.Fatal("injected style script must return non-zero when style verification fails")
	}
	if strings.Contains(displayPrepareMainCommand, "apt-get install") || strings.Contains(displayPrepareMainCommand, "apk add") {
		t.Fatal("package installation details should stay in scripts/desktop-install.sh")
	}
	if strings.Contains(displayPrepareMainCommand, "set -- $(tr") {
		t.Fatal("Xvnc process detection must not word-split shell command lines")
	}
	if !strings.Contains(displayPrepareMainCommand, "grep -Eq '(^|/)Xvnc$'") || !strings.Contains(displayPrepareMainCommand, "grep -Fxq ':99'") {
		t.Fatal("Xvnc process detection must match real Xvnc processes on display :99")
	}
	if !strings.Contains(displayPrepareMainCommand, "grep -Eq '(^|/)(google-chrome-stable|google-chrome|chromium|chromium-browser|chrome)$'") {
		t.Fatal("browser process detection must match real browser argv entries only")
	}
	if !strings.Contains(displayPrepareMainCommand, "grep -Eq '^--type=' && continue") {
		t.Fatal("CDP readiness detection must ignore Chromium child processes")
	}
	if !strings.Contains(displayPrepareMainCommand, "start_desktop_session()") ||
		!strings.Contains(displayPrepareMainCommand, "stop_fallback_wm") ||
		!strings.Contains(displayPrepareMainCommand, "start_xfwm4()") ||
		!strings.Contains(displayPrepareMainCommand, "process_pids_by_name startxfce4 xfce4-session xfdesktop") ||
		!strings.Contains(displayPrepareMainCommand, "process_pids_by_name xfwm4") {
		t.Fatal("display prepare must prefer Xfce over the fallback window manager")
	}
	if strings.Contains(displayPrepareMainCommand, "grep -E 'xfce4-session|xfwm4|twm'") {
		t.Fatal("display prepare must not treat twm as a healthy Xfce desktop session")
	}
	if !strings.Contains(displayPrepareMainCommand, "xsetroot -cursor_name left_ptr") {
		t.Fatal("display prepare must replace the default X root cursor")
	}
	if !strings.Contains(displayPrepareMainCommand, "SingletonLock") {
		t.Fatal("display prepare must clean stale Chromium profile locks before starting the browser")
	}
	if strings.Contains(displayPrepareMainCommand, "rfbunixpath") || strings.Contains(displayPrepareMainCommand, "RFB_SOCKET") {
		t.Fatal("display prepare should use loopback TCP VNC instead of a bind-mounted Unix RFB socket")
	}
	if !strings.Contains(displayPrepareMainCommand, "-localhost -rfbport \"$RFB_PORT\"") {
		t.Fatal("display prepare must keep VNC on container loopback")
	}
	if !strings.Contains(displayPrepareMainCommand, `XVNC_GEOMETRY="${MEMOH_DISPLAY_GEOMETRY:-1280x960}"`) {
		t.Fatal("display prepare must default to the 4:3 desktop geometry")
	}
	if !strings.Contains(displayPrepareMainCommand, `-geometry "$XVNC_GEOMETRY"`) {
		t.Fatal("display prepare must pass the configured geometry to Xvnc")
	}
}

func TestDisplayApplyStyleCommandInjectsStyleScript(t *testing.T) {
	t.Parallel()

	cmd := displayApplyStyleCommand()
	if !strings.Contains(cmd, "cat >/tmp/memoh-desktop-install.sh") {
		t.Fatal("display style command must inject the install script for existing desktops")
	}
	if !strings.Contains(cmd, "cat >/tmp/memoh-desktop-style.sh") {
		t.Fatal("display style command must inject the desktop style script")
	}
	if !strings.Contains(cmd, "install_style_extras_for_current_os") {
		t.Fatal("display style command must install missing macOS styling assets")
	}
	if !strings.Contains(cmd, "/bin/sh /tmp/memoh-desktop-style.sh") {
		t.Fatal("display style command must run the desktop style script")
	}
	if !strings.Contains(cmd, "style_marker=\"$style_config_dir/display-style.version\"") ||
		!strings.Contains(cmd, "style_version='"+displayDesktopStyleVersion+"'") ||
		!strings.Contains(cmd, "style_is_current") {
		t.Fatal("display style command must gate retries with a versioned marker")
	}
	if !strings.Contains(cmd, "style_lock=/tmp/memoh-desktop-style.lock") ||
		!strings.Contains(cmd, "acquire_style_lock") {
		t.Fatal("display style command must serialize style application with a lock")
	}
	if !strings.Contains(cmd, `tail -n 80 "$style_log"`) ||
		!strings.Contains(cmd, `printf '%s\n' "$style_version" >"$style_marker"`) {
		t.Fatal("display style command must persist success and print log tail on failure")
	}
	if !strings.Contains(cmd, `/bin/sh /tmp/memoh-desktop-apply-style.sh --if-needed`) {
		t.Fatal("display style command must only apply style when the marker is missing or stale")
	}
	if !strings.Contains(cmd, "configure_plank()") || !strings.Contains(cmd, "WhiteSur-Dark") {
		t.Fatal("display style command must include macOS-like desktop styling")
	}
	if !strings.Contains(cmd, "configure_topbar()") || !strings.Contains(cmd, "windowck-plugin") || !strings.Contains(cmd, "appmenu") {
		t.Fatal("display style command must include macOS-like topbar styling")
	}
	if !strings.Contains(cmd, "MEMOH_DISPLAY_INSTALL_ASSETS_GLOBAL") ||
		!strings.Contains(cmd, "/usr/local/share/themes") ||
		!strings.Contains(cmd, "/usr/local/share/icons") ||
		!strings.Contains(cmd, "/usr/local/share/backgrounds/WhiteSur") ||
		!strings.Contains(cmd, "/usr/local/share/plank/themes") {
		t.Fatal("display style command must reuse image-baked desktop style assets in global paths")
	}
	if !strings.Contains(cmd, "memoh-logo-white") || strings.Contains(cmd, "memoh-apple-symbol") {
		t.Fatal("display style command must use the white Memoh logo for the topbar menu icon")
	}
	if !strings.Contains(cmd, "memoh_logo_png_base64()") ||
		!strings.Contains(cmd, "memoh-logo-white.png") ||
		!strings.Contains(cmd, "/usr/share/icons/hicolor/48x48/apps") ||
		!strings.Contains(cmd, "base64 -d") {
		t.Fatal("display style command must install a PNG Memoh logo fallback for panels without SVG icon loading")
	}
	if !strings.Contains(cmd, "xfconf_replace_int_array xfce4-panel /panels 1") || !strings.Contains(cmd, "restart_xfce_panel()") {
		t.Fatal("display style command must remove the default Xfce bottom panel")
	}
	if !strings.Contains(cmd, "xsetroot -cursor_name left_ptr") || !strings.Contains(cmd, "nohup xfwm4 --replace") {
		t.Fatal("display style command must restore the Xfce window manager and pointer cursor")
	}
	if !strings.Contains(cmd, "plugin-ids 101 102 103 104 105 106 107 108") ||
		!strings.Contains(cmd, "xfconf_reset xfce4-panel /plugins/plugin-109") ||
		strings.Contains(cmd, "plugin-109 string actions") {
		t.Fatal("display style command must omit the Xfce actions menu with logout and power options")
	}
	if !strings.Contains(cmd, `write_chromium_wrapper "$browser"`) ||
		!strings.Contains(cmd, `write_desktop_file "$file" "Chromium" "chromium" "$wrapper"`) ||
		!strings.Contains(cmd, `--user-data-dir="\$profile"`) ||
		!strings.Contains(cmd, `rm -f "\$profile"/SingletonLock`) ||
		strings.Contains(cmd, `write_desktop_file "$file" "Browser" "web-browser"`) {
		t.Fatal("display style command must pin the dock browser launcher to a Chromium wrapper with an isolated profile")
	}
	if !strings.Contains(cmd, `write_desktop_file "$file" "Files" "$icon"`) ||
		!strings.Contains(cmd, "Memoh-WhiteSur-dark") ||
		!strings.Contains(cmd, "global_theme_dir") ||
		!strings.Contains(cmd, "install_file_manager_icon_aliases") ||
		!strings.Contains(cmd, "file_manager_icon_path()") ||
		!strings.Contains(cmd, "folder-blue.svg") ||
		!strings.Contains(cmd, "org.xfce.filemanager") ||
		!strings.Contains(cmd, "org.xfce.thunar") ||
		!strings.Contains(cmd, "inode-directory") ||
		!strings.Contains(cmd, "text-x-generic.svg") ||
		!strings.Contains(cmd, "verify_file_manager_style()") ||
		!strings.Contains(cmd, "restart_file_manager()") ||
		strings.Contains(cmd, "memoh_files_icon_png_base64") ||
		strings.Contains(cmd, "write_memoh_files_icon_png") {
		t.Fatal("display style command must configure the Files launcher and file-manager icon theme")
	}
}

func TestDisplayStyleStatusCommandChecksVersionMarker(t *testing.T) {
	t.Parallel()

	cmd := displayStyleStatusCommand()
	if !strings.Contains(cmd, "display-style.version") {
		t.Fatal("style status command must check the desktop style marker")
	}
	if !strings.Contains(cmd, "style_version='"+displayDesktopStyleVersion+"'") {
		t.Fatal("style status command must check the current style version")
	}
	if !strings.Contains(cmd, "MEMOH_DISPLAY_DESKTOP_STYLE") {
		t.Fatal("style status command must treat disabled desktop styling as current")
	}
}

func TestWorkspaceDockerfileInstallsDisplayAssetsGlobally(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../docker/Dockerfile.workspace")
	if err != nil {
		t.Fatalf("read workspace Dockerfile: %v", err)
	}
	dockerfile := string(data)
	if !strings.Contains(dockerfile, "COPY scripts/desktop-install.sh /tmp/memoh-desktop-install.sh") {
		t.Fatal("workspace Dockerfile must install display assets through the shared install script")
	}
	if !strings.Contains(dockerfile, "MEMOH_DISPLAY_INSTALL_ASSETS_GLOBAL=1") {
		t.Fatal("workspace Dockerfile must bake static display style assets into global image paths")
	}
}
