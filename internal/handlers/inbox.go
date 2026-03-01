package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/inbox"
)

type InboxHandler struct {
	service        *inbox.Service
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewInboxHandler(log *slog.Logger, service *inbox.Service, botService *bots.Service, accountService *accounts.Service) *InboxHandler {
	return &InboxHandler{
		service:        service,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "inbox")),
	}
}

func (h *InboxHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/inbox")
	group.GET("", h.List)
	group.GET("/count", h.Count)
	group.GET("/:id", h.GetByID)
	group.POST("", h.Create)
	group.DELETE("/:id", h.Delete)
	group.POST("/mark-read", h.MarkRead)
}

// List godoc
// @Summary List inbox items
// @Description List inbox items for a bot with optional filters
// @Tags inbox
// @Param bot_id path string true "Bot ID"
// @Param is_read query string false "Filter by read status (true/false)"
// @Param source query string false "Filter by source"
// @Param limit query int false "Max items to return" default(50)
// @Param offset query int false "Offset for pagination" default(0)
// @Success 200 {array} inbox.Item
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/inbox [get]
func (h *InboxHandler) List(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	filter := inbox.ListFilter{
		Source: strings.TrimSpace(c.QueryParam("source")),
		Limit:  parseIntOr(c.QueryParam("limit"), 50),
		Offset: parseIntOr(c.QueryParam("offset"), 0),
	}
	if isReadStr := strings.TrimSpace(c.QueryParam("is_read")); isReadStr != "" {
		val := strings.EqualFold(isReadStr, "true")
		filter.IsRead = &val
	}
	items, err := h.service.List(c.Request().Context(), botID, filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, items)
}

// GetByID godoc
// @Summary Get inbox item
// @Description Get a single inbox item by ID
// @Tags inbox
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Inbox item ID"
// @Success 200 {object} inbox.Item
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/inbox/{id} [get]
func (h *InboxHandler) GetByID(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	itemID := strings.TrimSpace(c.Param("id"))
	if itemID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "item id is required")
	}
	item, err := h.service.GetByID(c.Request().Context(), botID, itemID)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "inbox item not found")
	}
	return c.JSON(http.StatusOK, item)
}

// Create godoc
// @Summary Create inbox item
// @Description Create a new inbox item (for external integrations)
// @Tags inbox
// @Param bot_id path string true "Bot ID"
// @Param payload body inbox.CreateRequest true "Inbox item payload"
// @Success 201 {object} inbox.Item
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/inbox [post]
func (h *InboxHandler) Create(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	var req inbox.CreateRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	req.BotID = botID
	if strings.TrimSpace(req.Content) == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "content is required")
	}
	item, err := h.service.Create(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, item)
}

// Delete godoc
// @Summary Delete inbox item
// @Description Delete a single inbox item
// @Tags inbox
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Inbox item ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/inbox/{id} [delete]
func (h *InboxHandler) Delete(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	itemID := strings.TrimSpace(c.Param("id"))
	if itemID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "item id is required")
	}
	if err := h.service.Delete(c.Request().Context(), botID, itemID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

type markReadRequest struct {
	IDs []string `json:"ids"`
}

// MarkRead godoc
// @Summary Mark inbox items as read
// @Description Batch mark inbox items as read
// @Tags inbox
// @Param bot_id path string true "Bot ID"
// @Param payload body markReadRequest true "Item IDs to mark as read"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/inbox/mark-read [post]
func (h *InboxHandler) MarkRead(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	var req markReadRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if len(req.IDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "ids is required")
	}
	if err := h.service.MarkRead(c.Request().Context(), botID, req.IDs); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// Count godoc
// @Summary Count inbox items
// @Description Count unread and total inbox items
// @Tags inbox
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} inbox.CountResult
// @Failure 400 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/inbox/count [get]
func (h *InboxHandler) Count(c echo.Context) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	result, err := h.service.Count(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

func (h *InboxHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: false})
}

func parseIntOr(s string, fallback int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}
