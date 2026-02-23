package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/heartbeat"
)

type HeartbeatHandler struct {
	service        *heartbeat.Service
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewHeartbeatHandler(log *slog.Logger, service *heartbeat.Service, botService *bots.Service, accountService *accounts.Service) *HeartbeatHandler {
	return &HeartbeatHandler{
		service:        service,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "heartbeat")),
	}
}

func (h *HeartbeatHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/heartbeat")
	group.GET("/logs", h.ListLogs)
	group.DELETE("/logs", h.DeleteLogs)
}

// ListLogs godoc
// @Summary List heartbeat logs
// @Description List heartbeat execution logs for a bot
// @Tags heartbeat
// @Param bot_id path string true "Bot ID"
// @Param before query string false "Before timestamp (RFC3339)"
// @Param limit query int false "Limit" default(50)
// @Success 200 {object} heartbeat.ListLogsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/heartbeat/logs [get]
func (h *HeartbeatHandler) ListLogs(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	var before *time.Time
	if raw := strings.TrimSpace(c.QueryParam("before")); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid before timestamp")
		}
		before = &t
	}
	limit := 50
	if raw := strings.TrimSpace(c.QueryParam("limit")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			limit = v
		}
	}

	items, err := h.service.ListLogs(c.Request().Context(), botID, before, limit)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, heartbeat.ListLogsResponse{Items: items})
}

// DeleteLogs godoc
// @Summary Delete heartbeat logs
// @Description Delete all heartbeat execution logs for a bot
// @Tags heartbeat
// @Param bot_id path string true "Bot ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/heartbeat/logs [delete]
func (h *HeartbeatHandler) DeleteLogs(c echo.Context) error {
	userID, err := h.requireUserID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	if err := h.service.DeleteLogs(c.Request().Context(), botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *HeartbeatHandler) requireUserID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *HeartbeatHandler) authorizeBotAccess(ctx context.Context, userID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, userID, botID, bots.AccessPolicy{AllowPublicMember: false})
}
