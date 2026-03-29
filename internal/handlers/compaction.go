package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/compaction"
)

type CompactionHandler struct {
	service        *compaction.Service
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewCompactionHandler(log *slog.Logger, service *compaction.Service, botService *bots.Service, accountService *accounts.Service) *CompactionHandler {
	return &CompactionHandler{
		service:        service,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "compaction")),
	}
}

func (h *CompactionHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/compaction")
	group.GET("/logs", h.ListLogs)
	group.DELETE("/logs", h.DeleteLogs)
}

// ListLogs godoc
// @Summary List compaction logs
// @Description List compaction logs for a bot
// @Tags compaction
// @Param bot_id path string true "Bot ID"
// @Param limit query int false "Limit" default(50)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} compaction.ListLogsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/compaction/logs [get].
func (h *CompactionHandler) ListLogs(c echo.Context) error {
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

	limit, offset := parseOffsetLimit(c)
	items, total, err := h.service.ListLogs(c.Request().Context(), botID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, compaction.ListLogsResponse{Items: items, TotalCount: total})
}

// DeleteLogs godoc
// @Summary Delete compaction logs
// @Description Delete all compaction logs for a bot
// @Tags compaction
// @Param bot_id path string true "Bot ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/compaction/logs [delete].
func (h *CompactionHandler) DeleteLogs(c echo.Context) error {
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

func (*CompactionHandler) requireUserID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *CompactionHandler) authorizeBotAccess(ctx context.Context, userID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, userID, botID)
}
