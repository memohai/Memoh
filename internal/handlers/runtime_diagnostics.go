package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/runtimediagnostics"
)

type RuntimeDiagnosticsHandler struct {
	service        *runtimediagnostics.Service
	botService     *bots.Service
	accountService *accounts.Service
}

func NewRuntimeDiagnosticsHandler(service *runtimediagnostics.Service, botService *bots.Service, accountService *accounts.Service) *RuntimeDiagnosticsHandler {
	return &RuntimeDiagnosticsHandler{
		service:        service,
		botService:     botService,
		accountService: accountService,
	}
}

func (h *RuntimeDiagnosticsHandler) Register(e *echo.Echo) {
	e.GET("/bots/:bot_id/runtime-diagnostics", h.Get)
}

// Get godoc
// @Summary Get bot runtime diagnostics
// @Description Returns read-only ACP, workspace, container, and display runtime diagnostics for a bot.
// @Tags bots
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} runtimediagnostics.Response
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/runtime-diagnostics [get].
func (h *RuntimeDiagnosticsHandler) Get(c echo.Context) error {
	if h == nil || h.service == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "runtime diagnostics service not configured")
	}
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := c.Param("bot_id")
	bot, err := AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, bots.PermissionManage)
	if err != nil {
		return err
	}
	resp, err := h.service.Get(c.Request().Context(), bot)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}
