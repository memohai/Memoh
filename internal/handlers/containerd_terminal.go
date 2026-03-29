package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/workspace/bridge"
	pb "github.com/memohai/memoh/internal/workspace/bridgepb"
)

var terminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type terminalInfoResponse struct {
	Available bool   `json:"available"`
	Shell     string `json:"shell"`
}

type terminalControlMessage struct {
	Type string `json:"type"`
	Cols uint32 `json:"cols,omitempty"`
	Rows uint32 `json:"rows,omitempty"`
}

// GetTerminalInfo godoc
// @Summary Check terminal availability for bot container
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} terminalInfoResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/container/terminal [get].
func (h *ContainerdHandler) GetTerminalInfo(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	if h.manager == nil {
		return c.JSON(http.StatusOK, terminalInfoResponse{Available: false})
	}

	client, clientErr := h.manager.MCPClient(ctx, botID)
	if clientErr != nil || client == nil {
		return c.JSON(http.StatusOK, terminalInfoResponse{Available: false})
	}

	shell := detectShell(ctx, client)
	return c.JSON(http.StatusOK, terminalInfoResponse{
		Available: true,
		Shell:     shell,
	})
}

// HandleTerminalWS godoc
// @Summary Interactive WebSocket terminal for bot container
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param cols query int false "Initial terminal columns" default(80)
// @Param rows query int false "Initial terminal rows" default(24)
// @Param token query string false "Auth token"
// @Success 101 "WebSocket upgrade"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/terminal/ws [get].
func (h *ContainerdHandler) HandleTerminalWS(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	if h.manager == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "manager not configured")
	}

	client, err := h.manager.MCPClient(ctx, botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "container not reachable: "+err.Error())
	}

	cols := parseUint32Query(c, "cols", 80)
	rows := parseUint32Query(c, "rows", 24)

	conn, err := terminalUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	shell := detectShell(ctx, client)
	execStream, err := client.ExecStreamPTY(ctx, shell, "/data", cols, rows)
	if err != nil {
		_ = conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "exec failed"))
		return nil
	}
	defer func() { _ = execStream.Close() }()

	done := make(chan struct{})

	// gRPC output -> WebSocket
	go func() {
		defer close(done)
		for {
			output, recvErr := execStream.Recv()
			if recvErr != nil {
				return
			}
			switch output.GetStream() {
			case pb.ExecOutput_STDOUT, pb.ExecOutput_STDERR:
				if data := output.GetData(); len(data) > 0 {
					if writeErr := conn.WriteMessage(websocket.BinaryMessage, data); writeErr != nil {
						return
					}
				}
			case pb.ExecOutput_EXIT:
				return
			}
		}
	}()

	// WebSocket -> gRPC stdin/resize
	go func() {
		for {
			msgType, data, readErr := conn.ReadMessage()
			if readErr != nil {
				_ = execStream.Close()
				return
			}
			switch msgType {
			case websocket.BinaryMessage:
				if len(data) > 0 {
					if sendErr := execStream.SendStdin(data); sendErr != nil {
						return
					}
				}
			case websocket.TextMessage:
				var ctrl terminalControlMessage
				if json.Unmarshal(data, &ctrl) == nil && ctrl.Type == "resize" && ctrl.Cols > 0 && ctrl.Rows > 0 {
					if resizeErr := execStream.Resize(ctrl.Cols, ctrl.Rows); resizeErr != nil {
						h.logger.Warn("terminal resize failed",
							slog.String("bot_id", botID), slog.Any("error", resizeErr))
					}
				}
			}
		}
	}()

	<-done
	return nil
}

// detectShell probes the container for an interactive shell with readline support.
// Prefers bash > zsh > /bin/sh.
func detectShell(ctx context.Context, client *bridge.Client) string {
	for _, sh := range []string{"/bin/bash", "/usr/bin/bash", "/bin/zsh", "/usr/bin/zsh"} {
		result, err := client.Exec(ctx, "test -x "+sh, "/", 5)
		if err == nil && result.ExitCode == 0 {
			return sh
		}
	}
	return "/bin/sh"
}

func parseUint32Query(c echo.Context, name string, fallback uint32) uint32 {
	raw := c.QueryParam(name)
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || v == 0 {
		return fallback
	}
	return uint32(v) //nolint:gosec // G115
}
