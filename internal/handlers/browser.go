package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/browser"
)

type BrowserHandler struct {
	service        *browser.Service
	botService     *bots.Service
	accountService *accounts.Service
}

func NewBrowserHandler(service *browser.Service, botService *bots.Service, accountService *accounts.Service) *BrowserHandler {
	return &BrowserHandler{
		service:        service,
		botService:     botService,
		accountService: accountService,
	}
}

func (h *BrowserHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/browser/sessions")
	group.POST("", h.CreateSession)
	group.GET("", h.ListSessions)
	group.DELETE("/:session_id", h.CloseSession)
	group.POST("/:session_id/actions", h.ExecuteAction)
}

func (h *BrowserHandler) CreateSession(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	var req browser.CreateSessionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	session, err := h.service.CreateSession(c.Request().Context(), botID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusCreated, session)
}

func (h *BrowserHandler) ListSessions(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	items, err := h.service.ListSessions(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items})
}

func (h *BrowserHandler) CloseSession(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}
	item, err := h.service.CloseSession(c.Request().Context(), botID, sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, item)
}

func (h *BrowserHandler) ExecuteAction(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}
	var req browser.ActionRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	result, err := h.service.ExecuteAction(c.Request().Context(), botID, sessionID, req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, result)
}

func (h *BrowserHandler) requireBotAccess(c echo.Context) (string, error) {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}

func (h *BrowserHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: false})
}
