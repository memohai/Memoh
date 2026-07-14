package scripts

import (
	"os"
	"strings"
)

const DesktopStyleVersion = "2026-06-14.10"

func DisplayPrepareCommand() string {
	return displayScriptPreamble() + writeScript("/tmp/memoh-display-prepare.sh", "MEMOH_DISPLAY_PREPARE", displayPrepareScript()) +
		"\n/bin/sh /tmp/memoh-display-prepare.sh"
}

func DisplayApplyStyleCommand() string {
	return displayScriptPreamble() + "\n/bin/sh /tmp/memoh-desktop-apply-style.sh --if-needed"
}

func DisplayStyleStatusCommand() string {
	return `style_version='` + DesktopStyleVersion + `'
style="${MEMOH_DISPLAY_DESKTOP_STYLE:-macos}"
case "$style" in
  ""|0|false|False|FALSE|off|Off|OFF|none|None|NONE) exit 0 ;;
esac
style_marker="${XDG_CONFIG_HOME:-$HOME/.config}/memoh/display-style.version"
[ -r "$style_marker" ] && [ "$(cat "$style_marker" 2>/dev/null || true)" = "$style_version" ]`
}

func DisplayStyleLogTailCommand() string {
	return `tail -n 80 /tmp/memoh-desktop-style.log 2>/dev/null || true`
}

func displayScriptPreamble() string {
	return writeScript("/tmp/memoh-desktop-install.sh", "MEMOH_DESKTOP_INSTALL", desktopInstallScript()) +
		writeScript("/tmp/memoh-desktop-style.sh", "MEMOH_DESKTOP_STYLE", desktopStyleScript()) +
		writeScript("/tmp/memoh-desktop-apply-style.sh", "MEMOH_DESKTOP_APPLY_STYLE", displayApplyStyleScript())
}

func desktopInstallScript() string {
	if data, err := os.ReadFile("scripts/desktop-install.sh"); err == nil {
		return string(data)
	}
	return DesktopInstall
}

func desktopStyleScript() string {
	if data, err := os.ReadFile("scripts/desktop-style.sh"); err == nil {
		return string(data)
	}
	return DesktopStyle
}

func displayApplyStyleScript() string {
	script := DisplayApplyStyle
	if data, err := os.ReadFile("scripts/display-apply-style.sh"); err == nil {
		script = string(data)
	}
	return strings.ReplaceAll(script, "__MEMOH_DISPLAY_DESKTOP_STYLE_VERSION__", DesktopStyleVersion)
}

func displayPrepareScript() string {
	if data, err := os.ReadFile("scripts/display-prepare.sh"); err == nil {
		return string(data)
	}
	return DisplayPrepare
}

func writeScript(path, delimiter, content string) string {
	return `cat >` + path + ` <<'` + delimiter + `'
` + strings.TrimRight(content, "\n") + `
` + delimiter + `
chmod 0755 ` + path + `
`
}
