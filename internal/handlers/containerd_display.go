package handlers

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
)

const (
	displayTunnelAddress = "127.0.0.1:5999"
	displayProbeTimeout  = 3 * time.Second
)

var displayUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type displayInfoResponse struct {
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
	Running           bool   `json:"running"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
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
		Enabled: h.manager != nil && h.manager.BotDisplayEnabled(ctx, botID),
	}
	if h.manager == nil {
		resp.UnavailableReason = "manager not configured"
		return c.JSON(http.StatusOK, resp)
	}

	client, err := h.manager.MCPClient(ctx, botID)
	if err != nil || client == nil {
		resp.UnavailableReason = "container not reachable"
		return c.JSON(http.StatusOK, resp)
	}

	bundle, err := client.Exec(ctx, "test -x /opt/memoh/toolkit/display/bin/Xvnc", "/", 5)
	if err != nil || bundle.ExitCode != 0 {
		resp.UnavailableReason = "display bundle unavailable"
		return c.JSON(http.StatusOK, resp)
	}
	resp.Available = true

	probeCtx, cancel := context.WithTimeout(ctx, displayProbeTimeout)
	defer cancel()
	conn, err := client.DialContext(probeCtx, "tcp", displayTunnelAddress)
	if err == nil {
		resp.Running = true
		_ = conn.Close()
	} else {
		resp.UnavailableReason = "display server not reachable"
	}
	return c.JSON(http.StatusOK, resp)
}

// HandleDisplayWS godoc
// @Summary WebSocket VNC relay for bot workspace display
// @Tags containerd
// @Param bot_id path string true "Bot ID"
// @Param token query string false "Auth token"
// @Success 101 "WebSocket upgrade"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/container/display/ws [get].
func (h *ContainerdHandler) HandleDisplayWS(c echo.Context) error {
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
		return echo.NewHTTPError(http.StatusServiceUnavailable, "container not reachable: "+err.Error())
	}

	tcpConn, err := client.DialContext(ctx, "tcp", displayTunnelAddress)
	if err != nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "display not reachable: "+err.Error())
	}
	defer func() { _ = tcpConn.Close() }()

	wsConn, err := displayUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = wsConn.Close() }()

	done := make(chan struct{}, 2)
	go pumpDisplayToWS(wsConn, tcpConn, done)
	go pumpWSToDisplay(wsConn, tcpConn, done)
	<-done
	return nil
}

func pumpDisplayToWS(wsConn *websocket.Conn, tcpConn net.Conn, done chan<- struct{}) {
	defer func() {
		_ = wsConn.Close()
		_ = tcpConn.Close()
		done <- struct{}{}
	}()
	buf := make([]byte, 32*1024)
	for {
		n, err := tcpConn.Read(buf)
		if n > 0 {
			if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func pumpWSToDisplay(wsConn *websocket.Conn, tcpConn net.Conn, done chan<- struct{}) {
	defer func() {
		_ = wsConn.Close()
		_ = tcpConn.Close()
		done <- struct{}{}
	}()
	for {
		msgType, data, err := wsConn.ReadMessage()
		if err != nil {
			return
		}
		if msgType != websocket.BinaryMessage && msgType != websocket.TextMessage {
			continue
		}
		if len(data) == 0 {
			continue
		}
		if _, err := tcpConn.Write(data); err != nil {
			return
		}
	}
}
