package handlers

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/email"
)

type EmailOutboxHandler struct {
	outbox *email.OutboxService
	logger *slog.Logger
}

func NewEmailOutboxHandler(log *slog.Logger, outbox *email.OutboxService) *EmailOutboxHandler {
	return &EmailOutboxHandler{
		outbox: outbox,
		logger: log.With(slog.String("handler", "email_outbox")),
	}
}

func (h *EmailOutboxHandler) Register(e *echo.Echo) {
	g := e.Group("/bots/:bot_id/email-outbox")
	g.GET("", h.List)
	g.GET("/:id", h.Get)
}

// List godoc
// @Summary List outbox emails for a bot (audit)
// @Tags email-outbox
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param limit query int false "Limit" default(20)
// @Param offset query int false "Offset" default(0)
// @Success 200 {object} map[string]any
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/email-outbox [get]
func (h *EmailOutboxHandler) List(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id is required")
	}
	limit, _ := strconv.Atoi(c.QueryParam("limit"))
	if limit <= 0 {
		limit = 20
	}
	offset, _ := strconv.Atoi(c.QueryParam("offset"))

	items, total, err := h.outbox.ListByBot(c.Request().Context(), botID, int32(limit), int32(offset))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items": items,
		"total": total,
	})
}

// Get godoc
// @Summary Get outbox email detail
// @Tags email-outbox
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Email ID"
// @Success 200 {object} email.OutboxItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/email-outbox/{id} [get]
func (h *EmailOutboxHandler) Get(c echo.Context) error {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	resp, err := h.outbox.Get(c.Request().Context(), id)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}
