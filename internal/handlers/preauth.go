package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/preauth"
)

type PreauthHandler struct {
	service        *preauth.Service
	botService     *bots.Service
	accountService *accounts.Service
}

func NewPreauthHandler(service *preauth.Service, botService *bots.Service, accountService *accounts.Service) *PreauthHandler {
	return &PreauthHandler{
		service:        service,
		botService:     botService,
		accountService: accountService,
	}
}

func (h *PreauthHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/preauth_keys")
	group.POST("", h.Issue)
}

type preauthIssueRequest struct {
	TTLSeconds int `json:"ttl_seconds"`
}

func (h *PreauthHandler) Issue(c echo.Context) error {
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
	var req preauthIssueRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	ttl := 24 * time.Hour
	if req.TTLSeconds > 0 {
		ttl = time.Duration(req.TTLSeconds) * time.Second
	}
	key, err := h.service.Issue(c.Request().Context(), botID, userID, ttl)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, key)
}

func (h *PreauthHandler) requireUserID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *PreauthHandler) authorizeBotAccess(ctx context.Context, userID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, userID, botID, bots.AccessPolicy{AllowPublicMember: false})
}
