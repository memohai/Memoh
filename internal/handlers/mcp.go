package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/mcp"
)

type MCPHandler struct {
	service        *mcp.ConnectionService
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewMCPHandler(log *slog.Logger, service *mcp.ConnectionService, botService *bots.Service, accountService *accounts.Service) *MCPHandler {
	return &MCPHandler{
		service:        service,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "mcp")),
	}
}

func (h *MCPHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/mcp")
	group.GET("", h.List)
	group.POST("", h.Create)
	group.GET("/:id", h.Get)
	group.PUT("/:id", h.Update)
	group.DELETE("/:id", h.Delete)
}

// List godoc
// @Summary List MCP connections
// @Description List MCP connections for a bot
// @Tags mcp
// @Success 200 {object} mcp.ListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp [get]
func (h *MCPHandler) List(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
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
	items, err := h.service.ListByBot(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, mcp.ListResponse{Items: items})
}

// Create godoc
// @Summary Create MCP connection
// @Description Create a MCP connection for a bot
// @Tags mcp
// @Param payload body mcp.UpsertRequest true "MCP payload"
// @Success 201 {object} mcp.Connection
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp [post]
func (h *MCPHandler) Create(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
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
	var req mcp.UpsertRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.Create(c.Request().Context(), botID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusCreated, resp)
}

// Get godoc
// @Summary Get MCP connection
// @Description Get a MCP connection by ID
// @Tags mcp
// @Param id path string true "MCP ID"
// @Success 200 {object} mcp.Connection
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp/{id} [get]
func (h *MCPHandler) Get(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
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
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	resp, err := h.service.Get(c.Request().Context(), botID, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "mcp connection not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Update godoc
// @Summary Update MCP connection
// @Description Update a MCP connection by ID
// @Tags mcp
// @Param id path string true "MCP ID"
// @Param payload body mcp.UpsertRequest true "MCP payload"
// @Success 200 {object} mcp.Connection
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp/{id} [put]
func (h *MCPHandler) Update(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
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
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	var req mcp.UpsertRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.Update(c.Request().Context(), botID, id, req)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "mcp connection not found")
		}
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// Delete godoc
// @Summary Delete MCP connection
// @Description Delete a MCP connection by ID
// @Tags mcp
// @Param id path string true "MCP ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp/{id} [delete]
func (h *MCPHandler) Delete(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
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
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "id is required")
	}
	if err := h.service.Delete(c.Request().Context(), botID, id); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *MCPHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateChannelIdentityID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

func (h *MCPHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.accountService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, channelIdentityID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: false})
	if err != nil {
		if errors.Is(err, bots.ErrBotNotFound) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusNotFound, "bot not found")
		}
		if errors.Is(err, bots.ErrBotAccessDenied) {
			return bots.Bot{}, echo.NewHTTPError(http.StatusForbidden, "bot access denied")
		}
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return bot, nil
}
