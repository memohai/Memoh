package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	displaypkg "github.com/memohai/memoh/internal/display"
	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
	scriptassets "github.com/memohai/memoh/scripts"
)

type displayInfoResponse struct {
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
	Running           bool   `json:"running"`
	Transport         string `json:"transport"`
	Encoder           string `json:"encoder"`
	EncoderAvailable  bool   `json:"encoder_available"`
	DesktopAvailable  bool   `json:"desktop_available"`
	BrowserAvailable  bool   `json:"browser_available"`
	ToolkitAvailable  bool   `json:"toolkit_available"`
	PrepareSupported  bool   `json:"prepare_supported"`
	PrepareSystem     string `json:"prepare_system,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type displayWebRTCOfferRequest struct {
	Type          string `json:"type"`
	SDP           string `json:"sdp"`
	SessionID     string `json:"session_id,omitempty"`
	CandidateHost string `json:"candidate_host,omitempty"`
}

type displayWebRTCOfferResponse struct {
	Type      string `json:"type"`
	SDP       string `json:"sdp"`
	SessionID string `json:"session_id"`
}

type displaySessionListResponse struct {
	Items []displaypkg.SessionInfo `json:"items"`
}

type displayRuntimeProbe struct {
	ToolkitAvailable bool   `json:"toolkit_available"`
	PrepareSupported bool   `json:"prepare_supported"`
	PrepareSystem    string `json:"prepare_system"`
	DesktopAvailable bool   `json:"desktop_available"`
	BrowserAvailable bool   `json:"browser_available"`
	VNCAvailable     bool   `json:"vnc_available"`
}

// GetDisplayInfo godoc
// @Summary Check workspace display availability for bot container
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} displayInfoResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/container/display [get].
func (h *ContainerdHandler) GetDisplayInfo(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	resp := displayInfoResponse{
		Transport: displaypkg.TransportWebRTC,
		Encoder:   displaypkg.EncoderGStreamer,
	}
	if h.manager == nil || h.displayService == nil {
		resp.UnavailableReason = "manager not configured"
		return c.JSON(http.StatusOK, resp)
	}

	resp.Enabled = h.manager.BotDisplayEnabled(ctx, botID)
	if _, err := h.manager.MCPClient(ctx, botID); err != nil {
		resp.UnavailableReason = "container not reachable"
		return c.JSON(http.StatusOK, resp)
	}

	status := h.displayService.Status(ctx, botID)
	resp.Available = status.Available
	resp.Running = status.Running
	resp.Transport = status.Transport
	resp.Encoder = status.Encoder
	resp.EncoderAvailable = status.EncoderAvailable
	resp.UnavailableReason = status.UnavailableReason

	if resp.Enabled {
		client, err := h.manager.MCPClient(ctx, botID)
		if err == nil && client != nil {
			if probe, ok := probeDisplayRuntime(ctx, client); ok {
				resp.ToolkitAvailable = probe.ToolkitAvailable
				resp.PrepareSupported = probe.PrepareSupported
				resp.PrepareSystem = probe.PrepareSystem
				resp.DesktopAvailable = probe.DesktopAvailable
				resp.BrowserAvailable = probe.BrowserAvailable
				if !resp.Running && !probe.VNCAvailable {
					resp.UnavailableReason = "display bundle unavailable"
				}
			} else if resp.Available && resp.Running {
				resp.DesktopAvailable = true
				resp.BrowserAvailable = true
			}
		}
	}

	return c.JSON(http.StatusOK, resp)
}

// HandleDisplayWebRTCOffer godoc
// @Summary Create a WebRTC answer for bot workspace display
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param payload body displayWebRTCOfferRequest true "WebRTC offer payload"
// @Success 200 {object} displayWebRTCOfferResponse
// @Failure 400 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router /bots/{bot_id}/container/display/webrtc/offer [post].
func (h *ContainerdHandler) HandleDisplayWebRTCOffer(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.displayService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "display service not configured")
	}

	var req displayWebRTCOfferRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid display offer payload")
	}

	answer, err := h.displayService.Answer(c.Request().Context(), botID, displaypkg.OfferRequest{
		Type:      req.Type,
		SDP:       req.SDP,
		SessionID: req.SessionID,
		NATIPs:    h.displayNATIPs(c, req.CandidateHost),
	})
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, displaypkg.ErrDisplayDisabled) {
			status = http.StatusBadRequest
		}
		if !errors.Is(err, displaypkg.ErrEncoderUnavailable) &&
			!errors.Is(err, displaypkg.ErrDisplayUnavailable) &&
			!errors.Is(err, displaypkg.ErrDisplayDisabled) {
			status = http.StatusBadRequest
		}
		return echo.NewHTTPError(status, err.Error())
	}

	return c.JSON(http.StatusOK, displayWebRTCOfferResponse{
		Type:      answer.Type,
		SDP:       answer.SDP,
		SessionID: answer.SessionID,
	})
}

// ListDisplaySessions godoc
// @Summary List active workspace display WebRTC sessions
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} displaySessionListResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/container/display/sessions [get].
func (h *ContainerdHandler) ListDisplaySessions(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	if h.displayService == nil {
		return c.JSON(http.StatusOK, displaySessionListResponse{Items: nil})
	}
	return c.JSON(http.StatusOK, displaySessionListResponse{
		Items: h.displayService.ListSessions(botID),
	})
}

// CloseDisplaySession godoc
// @Summary Close a workspace display WebRTC session
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Display session ID"
// @Success 204
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/container/display/sessions/{session_id} [delete].
func (h *ContainerdHandler) CloseDisplaySession(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "display session id is required")
	}
	if h.displayService == nil || !h.displayService.CloseSession(botID, sessionID) {
		return echo.NewHTTPError(http.StatusNotFound, "display session not found")
	}
	return c.NoContent(http.StatusNoContent)
}

const displayPrepareProgressPrefix = "__MEMOH_DISPLAY_PROGRESS__"

type displayPrepareStreamEvent struct {
	Type    string `json:"type"`
	Step    string `json:"step,omitempty"`
	Message string `json:"message,omitempty"`
	Percent int    `json:"percent,omitempty"`
}

// PrepareDisplay godoc
// @Summary Prepare workspace display dependencies
// @Description Installs the workspace desktop/VNC/browser packages when needed, starts the display server, and launches the browser.
// @Tags containerd
// @Produce text/event-stream
// @Param bot_id path string true "Bot ID"
// @Success 200 {string} string "SSE stream of display preparation events"
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/container/display/prepare [post].
func (h *ContainerdHandler) PrepareDisplay(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	writer := bufio.NewWriter(c.Response().Writer)
	send := func(payload displayPrepareStreamEvent) {
		_ = writeSSEJSON(writer, flusher, payload)
	}
	sendError := func(step, message string) {
		send(displayPrepareStreamEvent{Type: "error", Step: step, Message: message})
	}

	ctx := c.Request().Context()
	if h.manager == nil {
		sendError("checking", "manager not configured")
		return nil
	}
	if !h.manager.BotDisplayEnabled(ctx, botID) {
		sendError("checking", "workspace display is not enabled")
		return nil
	}

	client, err := h.manager.MCPClient(ctx, botID)
	if err != nil || client == nil {
		if err != nil {
			sendError("checking", "workspace container is not reachable: "+err.Error())
		} else {
			sendError("checking", "workspace container is not reachable")
		}
		return nil
	}

	send(displayPrepareStreamEvent{
		Type:    "progress",
		Step:    "checking",
		Message: "Checking display runtime",
		Percent: 5,
	})

	stream, err := client.ExecStream(ctx, displayPrepareCommand(), "/", 1200)
	if err != nil {
		sendError("checking", "start display preparation failed: "+err.Error())
		return nil
	}
	defer func() { _ = stream.Close() }()

	var stdout, stderr lineAccumulator
	var stderrText strings.Builder
	completed := false
	exitCode := int32(0)
	lastStep := "checking"
	for {
		msg, recvErr := stream.Recv()
		if errors.Is(recvErr, io.EOF) {
			break
		}
		if recvErr != nil {
			sendError(lastStep, "display preparation stream failed: "+recvErr.Error())
			return nil
		}
		switch msg.GetStream() {
		case pb.ExecOutput_STDOUT:
			for _, line := range stdout.Append(msg.GetData()) {
				if event, ok := parseDisplayPrepareEvent(line); ok {
					if event.Step != "" {
						lastStep = event.Step
					}
					send(event)
					if event.Type == "complete" {
						completed = true
					}
				}
			}
		case pb.ExecOutput_STDERR:
			for _, line := range stderr.Append(msg.GetData()) {
				appendLimitedLine(&stderrText, line)
			}
		case pb.ExecOutput_EXIT:
			exitCode = msg.GetExitCode()
		}
	}
	for _, line := range stdout.Flush() {
		if event, ok := parseDisplayPrepareEvent(line); ok {
			if event.Step != "" {
				lastStep = event.Step
			}
			send(event)
			if event.Type == "complete" {
				completed = true
			}
		}
	}
	for _, line := range stderr.Flush() {
		appendLimitedLine(&stderrText, line)
	}
	if exitCode != 0 && !completed {
		message := strings.TrimSpace(stderrText.String())
		if message == "" {
			message = "display preparation failed"
		}
		sendError(lastStep, message)
		return nil
	}
	if !completed {
		send(displayPrepareStreamEvent{
			Type:    "complete",
			Step:    "complete",
			Message: "Display is ready",
			Percent: 100,
		})
	}
	return nil
}

type lineAccumulator struct {
	partial string
}

func (b *lineAccumulator) Append(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	text := b.partial + string(data)
	parts := strings.Split(text, "\n")
	b.partial = parts[len(parts)-1]
	lines := parts[:len(parts)-1]
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

func (b *lineAccumulator) Flush() []string {
	if b.partial == "" {
		return nil
	}
	line := strings.TrimRight(b.partial, "\r")
	b.partial = ""
	return []string{line}
}

func parseDisplayPrepareEvent(line string) (displayPrepareStreamEvent, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, displayPrepareProgressPrefix) {
		return displayPrepareStreamEvent{}, false
	}
	var event displayPrepareStreamEvent
	if err := json.Unmarshal([]byte(strings.TrimPrefix(line, displayPrepareProgressPrefix)), &event); err != nil {
		return displayPrepareStreamEvent{}, false
	}
	return event, true
}

func appendLimitedLine(builder *strings.Builder, line string) {
	line = strings.TrimSpace(line)
	if line == "" || builder.Len() > 6000 {
		return
	}
	if builder.Len() > 0 {
		builder.WriteByte('\n')
	}
	builder.WriteString(line)
}

func displayPrepareCommand() string {
	return `cat >/tmp/memoh-desktop-install.sh <<'MEMOH_DESKTOP_INSTALL'
` + strings.TrimRight(displayPrepareInstallScript(), "\n") + `
MEMOH_DESKTOP_INSTALL
chmod 0755 /tmp/memoh-desktop-install.sh
` + displayPrepareMainCommand
}

func displayPrepareInstallScript() string {
	if data, err := os.ReadFile("scripts/desktop-install.sh"); err == nil {
		return string(data)
	}
	return scriptassets.DesktopInstall
}

const displayPrepareMainCommand = `cat >/tmp/memoh-display-prepare.sh <<'MEMOH_DISPLAY_PREPARE'
#!/bin/sh
set -eu

prefix='__MEMOH_DISPLAY_PROGRESS__'
progress() {
  percent="$1"
  step="$2"
  shift 2
  message="$*"
  printf '%s{"type":"progress","percent":%s,"step":"%s","message":"%s"}\n' "$prefix" "$percent" "$step" "$message"
}
complete() {
  printf '%s{"type":"complete","percent":100,"step":"complete","message":"Display is ready"}\n' "$prefix"
}
has_cmd() {
  command -v "$1" >/dev/null 2>&1
}
find_xvnc() {
  for candidate in /opt/memoh/toolkit/display/bin/Xvnc /usr/bin/Xvnc /usr/local/bin/Xvnc Xvnc; do
    if echo "$candidate" | grep -q /; then
      [ -x "$candidate" ] && { printf '%s\n' "$candidate"; return 0; }
    elif has_cmd "$candidate"; then
      command -v "$candidate"
      return 0
    fi
  done
  return 1
}
find_browser() {
  for candidate in google-chrome-stable google-chrome chromium chromium-browser; do
    if has_cmd "$candidate"; then
      command -v "$candidate"
      return 0
    fi
  done
  return 1
}
has_desktop() {
  has_cmd startxfce4 || has_cmd xfce4-session || has_cmd xfwm4 || [ -x /opt/memoh/toolkit/display/bin/twm ]
}
has_toolkit() {
  [ -x /opt/memoh/toolkit/display/bin/Xvnc ] || [ -x /opt/memoh/toolkit/display/bin/twm ]
}
needs_install() {
  find_xvnc >/dev/null 2>&1 && find_browser >/dev/null 2>&1 && has_desktop
}
os_id() {
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    printf '%s\n' "${ID:-unknown}"
    return
  fi
  printf unknown
}
os_like() {
  if [ -r /etc/os-release ]; then
    # shellcheck disable=SC1091
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
RFB_PORT=5999
XVNC_GEOMETRY="${MEMOH_DISPLAY_GEOMETRY:-1280x960}"
X_SOCKET=/tmp/.X11-unix/X99
X_LOCK=/tmp/.X99-lock
xvnc_pids() {
  for proc_dir in /proc/[0-9]*; do
    [ -d "$proc_dir" ] || continue
    pid="${proc_dir#/proc/}"
    cmdline="$(tr '\000' '\n' <"$proc_dir/cmdline" 2>/dev/null || true)"
    printf '%s\n' "$cmdline" | grep -Eq '(^|/)Xvnc$' || continue
    printf '%s\n' "$cmdline" | grep -Fxq ':99' || continue
    printf '%s\n' "$pid"
  done
  return 0
}
xvnc_running() {
  [ -n "$(xvnc_pids)" ]
}
browser_pids() {
  for proc_dir in /proc/[0-9]*; do
    [ -d "$proc_dir" ] || continue
    pid="${proc_dir#/proc/}"
    cmdline="$(tr '\000' '\n' <"$proc_dir/cmdline" 2>/dev/null || true)"
    printf '%s\n' "$cmdline" | grep -Eq '(^|/)(google-chrome-stable|google-chrome|chromium|chromium-browser|chrome)$' || continue
    printf '%s\n' "$pid"
  done
  return 0
}
browser_cdp_running() {
  for proc_dir in /proc/[0-9]*; do
    [ -d "$proc_dir" ] || continue
    cmdline="$(tr '\000' '\n' <"$proc_dir/cmdline" 2>/dev/null || true)"
    printf '%s\n' "$cmdline" | grep -Eq '(^|/)(google-chrome-stable|google-chrome|chromium|chromium-browser|chrome)$' || continue
    printf '%s\n' "$cmdline" | grep -Eq '^--type=' && continue
    printf '%s\n' "$cmdline" | grep -Fq -- '--remote-debugging-port=9222' && return 0
  done
  return 1
}
cleanup_browser_profile() {
  [ -n "$(browser_pids)" ] && return 0
  rm -f /tmp/memoh-display-browser/SingletonLock /tmp/memoh-display-browser/SingletonSocket /tmp/memoh-display-browser/SingletonCookie
}
stop_xvnc() {
  pids="$(xvnc_pids)"
  [ -n "$pids" ] || return 0
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  sleep 1
  pids="$(xvnc_pids)"
  for pid in $pids; do
    kill -9 "$pid" 2>/dev/null || true
  done
}
stop_browsers() {
  pids="$(browser_pids)"
  [ -n "$pids" ] || return 0
  for pid in $pids; do
    kill "$pid" 2>/dev/null || true
  done
  sleep 1
  pids="$(browser_pids)"
  for pid in $pids; do
    kill -9 "$pid" 2>/dev/null || true
  done
}
display_socket_ready() {
  xvnc_running && [ -S "$X_SOCKET" ] && awk -v port="$(printf '%04X' "$RFB_PORT")" 'toupper($2) ~ ":" port "$" && $4 == "0A" { found = 1 } END { exit found ? 0 : 1 }' /proc/net/tcp /proc/net/tcp6 2>/dev/null
}
display_ready() {
  display_socket_ready && find_browser >/dev/null 2>&1 && has_desktop
}

. /tmp/memoh-desktop-install.sh

prepare_lock=/tmp/memoh-display-prepare.lock
if mkdir "$prepare_lock" 2>/dev/null; then
  trap 'rmdir "$prepare_lock" 2>/dev/null || true' EXIT INT TERM
else
  progress 12 checking "Waiting for another desktop preparation"
  wait_i=0
  while [ -d "$prepare_lock" ] && [ "$wait_i" -lt 180 ]; do
    if display_ready; then
      complete
      exit 0
    fi
    sleep 1
    wait_i=$((wait_i + 1))
  done
  if display_ready; then
    complete
    exit 0
  fi
  echo "Another desktop preparation is still running." >&2
  exit 1
fi

progress 10 checking "Checking display toolkit"
if ! has_toolkit; then
  progress 14 toolkit "Workspace display toolkit is not installed"
fi

if needs_install; then
  progress 18 checking "Display packages already installed"
else
  if is_debian_like; then
    install_debian
  elif is_alpine; then
    install_alpine
  else
    echo "Unsupported workspace OS: $(os_id). Install the Memoh workspace toolkit, or use a Debian/Ubuntu/Alpine image for automatic desktop preparation." >&2
    exit 1
  fi
fi

XVNC="$(find_xvnc || true)"
BROWSER="$(find_browser || true)"
[ -n "$XVNC" ] || { echo "Xvnc is still unavailable after installation. Install the Memoh workspace toolkit or a TigerVNC package." >&2; exit 1; }
[ -n "$BROWSER" ] || { echo "Chrome or Chromium is still unavailable after installation." >&2; exit 1; }

export DISPLAY=:99
mkdir -p /run/memoh /tmp/.X11-unix
chmod 1777 /tmp/.X11-unix 2>/dev/null || true

wait_for_socket() {
  path="$1"
  seconds="$2"
  i=0
  while [ "$i" -lt "$seconds" ]; do
    [ -S "$path" ] && return 0
    sleep 1
    i=$((i + 1))
  done
  return 1
}

cleanup_stale_display() {
  xvnc_running && return 0
  rm -f "$X_SOCKET" "$X_LOCK"
}

if ! display_socket_ready; then
  progress 78 starting "Starting VNC display"
  if xvnc_running; then
    wait_for_socket "$X_SOCKET" 10 || true
  fi
  if ! display_socket_ready; then
    stop_xvnc
    cleanup_stale_display
    nohup "$XVNC" :99 -geometry "$XVNC_GEOMETRY" -depth 24 -SecurityTypes None -localhost -rfbport "$RFB_PORT" >/tmp/memoh-xvnc.log 2>&1 &
    wait_i=0
    while [ "$wait_i" -lt 25 ]; do
      display_socket_ready && break
      sleep 1
      wait_i=$((wait_i + 1))
    done
    display_socket_ready || { cat /tmp/memoh-xvnc.log >&2 2>/dev/null || true; exit 1; }
  fi
fi

progress 88 desktop "Starting desktop session"
run_quick() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 5 "$@" >/dev/null 2>&1 || true
  else
    "$@" >/dev/null 2>&1 &
  fi
}
if command -v fc-cache >/dev/null 2>&1; then
  nohup fc-cache -f >/tmp/memoh-fc-cache.log 2>&1 &
fi
if [ -S "$X_SOCKET" ]; then
  if command -v xsetroot >/dev/null 2>&1; then
    run_quick xsetroot -solid '#315f7d'
  elif [ -x /opt/memoh/toolkit/display/bin/xsetroot ]; then
    run_quick /opt/memoh/toolkit/display/bin/xsetroot -solid '#315f7d'
  fi
fi
if ! ps -ef 2>/dev/null | grep -E 'xfce4-session|xfwm4|twm' | grep -v grep >/dev/null 2>&1; then
  if has_cmd startxfce4; then
    nohup startxfce4 >/tmp/memoh-xfce.log 2>&1 &
  elif has_cmd xfce4-session; then
    nohup xfce4-session >/tmp/memoh-xfce.log 2>&1 &
  elif has_cmd xfwm4; then
    nohup xfwm4 >/tmp/memoh-xfwm4.log 2>&1 &
  elif [ -x /opt/memoh/toolkit/display/bin/twm ]; then
    nohup /opt/memoh/toolkit/display/bin/twm >/tmp/memoh-twm.log 2>&1 &
  fi
fi

progress 94 browser "Launching browser"
if ! browser_cdp_running; then
  if [ -n "$(browser_pids)" ]; then
    stop_browsers
  fi
  cleanup_browser_profile
  nohup "$BROWSER" --no-sandbox --disable-dev-shm-usage --disable-gpu --no-first-run --no-default-browser-check --remote-debugging-address=127.0.0.1 --remote-debugging-port=9222 --remote-allow-origins='*' --user-data-dir=/tmp/memoh-display-browser about:blank >/tmp/memoh-browser.log 2>&1 &
fi

complete
exit 0
MEMOH_DISPLAY_PREPARE
/bin/sh /tmp/memoh-display-prepare.sh`

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
printf '{"toolkit_available":%s,"prepare_supported":%s,"prepare_system":"%s","desktop_available":%s,"browser_available":%s,"vnc_available":%s}\n' \
  "$(json_bool has_toolkit)" \
  "$(json_bool has_prepare)" \
  "$os_id" \
  "$(json_bool has_desktop)" \
  "$(json_bool has_browser)" \
  "$(json_bool has_vnc)"`

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

func (*ContainerdHandler) displayNATIPs(c echo.Context, candidateHost string) []string {
	ctx := c.Request().Context()
	hosts := []string{
		candidateHost,
		firstHeaderValue(c.Request().Header.Get("X-Forwarded-Host")),
		c.Request().Host,
	}
	seen := make(map[string]struct{})
	var ips []string
	for _, host := range hosts {
		for _, ip := range resolveDisplayHostIPs(ctx, host) {
			if _, ok := seen[ip]; ok {
				continue
			}
			seen[ip] = struct{}{}
			ips = append(ips, ip)
		}
	}
	return ips
}

func resolveDisplayHostIPs(ctx context.Context, value string) []string {
	host := strings.TrimSpace(value)
	if host == "" {
		return nil
	}
	if strings.HasPrefix(host, "[") {
		if end := strings.Index(host, "]"); end >= 0 {
			host = host[1:end]
		}
	} else if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.Count(host, ":") == 0 {
		if idx := strings.LastIndexByte(host, ':'); idx >= 0 {
			host = host[:idx]
		}
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return []string{ip.String()}
	}
	resolved, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(resolved))
	for _, ip := range resolved {
		if ip.IP == nil {
			continue
		}
		out = append(out, ip.IP.String())
	}
	return out
}
