package runtimediagnostics

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/memohai/memoh/internal/workspace/bridge"
)

type displayRuntimeProbe struct {
	ToolkitAvailable bool   `json:"toolkit_available"`
	PrepareSupported bool   `json:"prepare_supported"`
	PrepareSystem    string `json:"prepare_system"`
	DesktopAvailable bool   `json:"desktop_available"`
	BrowserAvailable bool   `json:"browser_available"`
	VNCAvailable     bool   `json:"vnc_available"`
	A11yAvailable    bool   `json:"a11y_available"`
}

const displayRuntimeProbeCommand = `has_cmd() { command -v "$1" >/dev/null 2>&1; }
has_exec() { [ -x "$1" ]; }
has_process() { ps -ef 2>/dev/null | grep -E "$1" | grep -v grep >/dev/null 2>&1; }
json_bool() { if "$@"; then printf true; else printf false; fi; }
os_id=unknown
os_like=
if [ -r /etc/os-release ]; then
  . /etc/os-release
  os_id="${ID:-unknown}"
  os_like="${ID:-} ${ID_LIKE:-}"
fi
has_toolkit() {
  has_exec /opt/memoh/toolkit/display/bin/Xvnc ||
    has_exec /opt/memoh/toolkit/display/bin/twm ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/Xvnc ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/twm
}
has_prepare() {
  case " $os_like " in
    *" debian "*|*" ubuntu "*|*" alpine "*) return 0 ;;
    *) return 1 ;;
  esac
}
has_vnc() {
  has_cmd Xvnc ||
    has_exec /opt/memoh/toolkit/display/bin/Xvnc ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/Xvnc ||
    has_exec /usr/bin/Xvnc ||
    has_exec /usr/local/bin/Xvnc
}
has_desktop() {
  has_cmd startxfce4 ||
    has_cmd xfce4-session ||
    has_cmd xfwm4 ||
    has_exec /opt/memoh/toolkit/display/bin/twm ||
    has_exec /opt/memoh/toolkit/display/root/usr/bin/twm ||
    has_process 'xfce4-session|xfwm4|twm'
}
has_browser() {
  has_cmd google-chrome-stable ||
    has_cmd google-chrome ||
    has_cmd chromium ||
    has_cmd chromium-browser ||
    has_process 'google-chrome|chromium'
}
has_a11y() {
  a11y=/opt/memoh/toolkit/display/bin/a11y-cli
  [ -x "$a11y" ] || return 1
  DISPLAY=:99 "$a11y" probe 2>/dev/null | grep -q '"ok":true'
}
printf '{"toolkit_available":%s,"prepare_supported":%s,"prepare_system":"%s","desktop_available":%s,"browser_available":%s,"vnc_available":%s,"a11y_available":%s}\n' \
  "$(json_bool has_toolkit)" \
  "$(json_bool has_prepare)" \
  "$os_id" \
  "$(json_bool has_desktop)" \
  "$(json_bool has_browser)" \
  "$(json_bool has_vnc)" \
  "$(json_bool has_a11y)"`

func probeDisplayRuntime(ctx context.Context, client *bridge.Client) (displayRuntimeProbe, bool) {
	var probe displayRuntimeProbe
	if client == nil {
		return probe, false
	}
	for attempt := 0; attempt < 3; attempt++ {
		result, err := client.Exec(ctx, displayRuntimeProbeCommand, "/", 10)
		if err == nil && result != nil && result.ExitCode == 0 {
			if err := json.Unmarshal([]byte(strings.TrimSpace(result.Stdout)), &probe); err == nil {
				return probe, true
			}
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return probe, false
		case <-timer.C:
		}
	}
	return probe, false
}
