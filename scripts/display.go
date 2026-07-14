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
	return writeScript("/tmp/memoh-desktop-install.sh", "MEMOH_DESKTOP_INSTALL", sourceOrEmbedded("scripts/desktop-install.sh", DesktopInstall)) +
		writeScript("/tmp/memoh-desktop-style.sh", "MEMOH_DESKTOP_STYLE", sourceOrEmbedded("scripts/desktop-style.sh", DesktopStyle)) +
		writeScript("/tmp/memoh-desktop-apply-style.sh", "MEMOH_DESKTOP_APPLY_STYLE", displayApplyStyleScript())
}

func displayApplyStyleScript() string {
	script := sourceOrEmbedded("scripts/display-apply-style.sh", DisplayApplyStyle)
	return strings.ReplaceAll(script, "__MEMOH_DISPLAY_DESKTOP_STYLE_VERSION__", DesktopStyleVersion)
}

func displayPrepareScript() string {
	return sourceOrEmbedded("scripts/display-prepare.sh", DisplayPrepare)
}

func sourceOrEmbedded(path, embedded string) string {
	if data, err := os.ReadFile(path); err == nil {
		return string(data)
	}
	return embedded
}

func writeScript(path, delimiter, content string) string {
	return `cat >` + path + ` <<'` + delimiter + `'
` + strings.TrimRight(content, "\n") + `
` + delimiter + `
chmod 0755 ` + path + `
`
}
