package handlers

import (
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	displaypkg "github.com/memohai/memoh/internal/display"
)

type displayInfoResponse struct {
	Enabled           bool   `json:"enabled"`
	Available         bool   `json:"available"`
	Running           bool   `json:"running"`
	Transport         string `json:"transport"`
	Encoder           string `json:"encoder"`
	EncoderAvailable  bool   `json:"encoder_available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

type displayWebRTCOfferRequest struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

type displayWebRTCOfferResponse struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
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

	if resp.Enabled && !resp.Running {
		client, err := h.manager.MCPClient(ctx, botID)
		if err == nil && client != nil {
			bundle, execErr := client.Exec(ctx, "test -x /opt/memoh/toolkit/display/bin/Xvnc", "/", 5)
			if execErr != nil || bundle.ExitCode != 0 {
				resp.UnavailableReason = "display bundle unavailable"
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
		Type: req.Type,
		SDP:  req.SDP,
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
		Type: answer.Type,
		SDP:  answer.SDP,
	})
}
