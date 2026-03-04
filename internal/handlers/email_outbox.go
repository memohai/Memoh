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
// @Router /bots/{bot_id}/email-outbox [get].
func (h *EmailOutboxHandler) List(c echo.Context) error {
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id is required")
	}
	limit, err := parseInt32Query(c.QueryParam("limit"), 20)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	offset, err := parseInt32Query(c.QueryParam("offset"), 0)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	items, total, err := h.outbox.ListByBot(c.Request().Context(), botID, limit, offset)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{
		"items": items,
		"total": total,
	})
}

func parseInt32Query(raw string, defaultValue int32) (int32, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultValue, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, echo.NewHTTPError(http.StatusBadRequest, "invalid integer query parameter")
	}
	value := int32(parsed)
	if value < 0 {
		return 0, nil
	}
	return value, nil
}

// Get godoc
// @Summary Get outbox email detail
// @Tags email-outbox
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Email ID"
// @Success 200 {object} email.OutboxItemResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/email-outbox/{id} [get].
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
