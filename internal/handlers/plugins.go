package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/mcp"
	pluginspkg "github.com/memohai/memoh/internal/plugins"
)

type PluginsHandler struct {
	service        *pluginspkg.Service
	mcpService     *mcp.ConnectionService
	fedGateway     *MCPFederationGateway
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewPluginsHandler(log *slog.Logger, service *pluginspkg.Service, mcpService *mcp.ConnectionService, fedGateway *MCPFederationGateway, botService *bots.Service, accountService *accounts.Service) *PluginsHandler {
	if log == nil {
		log = slog.Default()
	}
	return &PluginsHandler{
		service:        service,
		mcpService:     mcpService,
		fedGateway:     fedGateway,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "plugins")),
	}
}

func (h *PluginsHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/plugins")
	group.GET("", h.List)
	group.GET("/:id", h.Get)
	group.PUT("/:id/config", h.UpdateConfig)
	group.POST("/:id/enable", h.Enable)
	group.POST("/:id/disable", h.Disable)
	group.POST("/:id/uninstall", h.Uninstall)
	group.DELETE("/:id", h.Purge)
	group.POST("/:id/oauth/authorize", h.StartOAuth)
	group.GET("/:id/oauth/status", h.RefreshOAuthStatus)
}

func (h *PluginsHandler) requireBotAccess(c echo.Context) (string, error) {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return "", err
	}
	return botID, nil
}

func pluginIDParam(c echo.Context) (string, error) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "plugin installation id is required")
	}
	return id, nil
}

// List godoc
// @Summary List bot plugins
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} plugins.ListResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins [get].
func (h *PluginsHandler) List(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	items, err := h.service.List(c.Request().Context(), botID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, pluginspkg.ListResponse{Items: items})
}

// Get godoc
// @Summary Get bot plugin installation
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Success 200 {object} plugins.Installation
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id} [get].
func (h *PluginsHandler) Get(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	id, err := pluginIDParam(c)
	if err != nil {
		return err
	}
	resp, err := h.service.Get(c.Request().Context(), botID, id)
	if err != nil {
		return pluginServiceError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

// UpdateConfig godoc
// @Summary Update bot plugin configuration
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Param payload body plugins.UpdateConfigRequest true "Plugin configuration update"
// @Success 200 {object} plugins.Installation
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id}/config [put].
func (h *PluginsHandler) UpdateConfig(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	id, err := pluginIDParam(c)
	if err != nil {
		return err
	}
	var req pluginspkg.UpdateConfigRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.UpdateConfigWithValidation(c.Request().Context(), botID, id, req, func(ctx context.Context, installation pluginspkg.Installation) error {
		return h.probeReadyPluginMCPs(ctx, botID, installation)
	})
	if err != nil {
		if errors.Is(err, pluginspkg.ErrPluginMCPProbeFailed) {
			h.logger.Warn("plugin configuration update failed",
				slog.String("bot_id", botID),
				slog.String("installation_id", id),
				slog.Any("error", err))
			return pluginMCPProbeError(err)
		}
		return pluginServiceError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

// Enable godoc
// @Summary Enable bot plugin
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Success 200 {object} plugins.Installation
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id}/enable [post].
func (h *PluginsHandler) Enable(c echo.Context) error {
	return h.setEnabled(c, true)
}

// Disable godoc
// @Summary Disable bot plugin
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Success 200 {object} plugins.Installation
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id}/disable [post].
func (h *PluginsHandler) Disable(c echo.Context) error {
	return h.setEnabled(c, false)
}

func (h *PluginsHandler) setEnabled(c echo.Context, enabled bool) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	id, err := pluginIDParam(c)
	if err != nil {
		return err
	}
	resp, err := h.service.SetEnabled(c.Request().Context(), botID, id, enabled)
	if err != nil {
		return pluginServiceError(err)
	}
	if enabled {
		if err := h.probeReadyPluginMCPs(c.Request().Context(), botID, resp); err != nil {
			h.logger.Warn("plugin MCP probe failed after enable",
				slog.String("bot_id", botID),
				slog.String("installation_id", id),
				slog.Any("error", err))
			if _, disableErr := h.service.SetEnabled(c.Request().Context(), botID, id, false); disableErr != nil {
				h.logger.Warn("failed to disable plugin after MCP probe failure",
					slog.String("bot_id", botID),
					slog.String("installation_id", id),
					slog.Any("error", disableErr))
			}
			return pluginMCPProbeError(err)
		}
		resp, err = h.service.Activate(c.Request().Context(), botID, id)
		if err != nil {
			return pluginServiceError(err)
		}
		if refreshed, err := h.service.Get(c.Request().Context(), botID, id); err == nil {
			resp = refreshed
		}
	}
	return c.JSON(http.StatusOK, resp)
}

// Uninstall godoc
// @Summary Uninstall bot plugin
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id}/uninstall [post].
func (h *PluginsHandler) Uninstall(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	id, err := pluginIDParam(c)
	if err != nil {
		return err
	}
	if err := h.service.Uninstall(c.Request().Context(), botID, id); err != nil {
		return pluginServiceError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// Purge godoc
// @Summary Purge bot plugin installation
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id} [delete].
func (h *PluginsHandler) Purge(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	id, err := pluginIDParam(c)
	if err != nil {
		return err
	}
	if err := h.service.Purge(c.Request().Context(), botID, id); err != nil {
		return pluginServiceError(err)
	}
	return c.NoContent(http.StatusNoContent)
}

// StartOAuth godoc
// @Summary Start managed OAuth for a bot plugin
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Param payload body plugins.OAuthAuthorizeRequest false "OAuth authorize request"
// @Success 200 {object} mcp.AuthorizeResult
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id}/oauth/authorize [post].
func (h *PluginsHandler) StartOAuth(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	id, err := pluginIDParam(c)
	if err != nil {
		return err
	}
	var req pluginspkg.OAuthAuthorizeRequest
	if err := c.Bind(&req); err != nil && !errors.Is(err, io.EOF) {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	resp, err := h.service.StartOAuth(c.Request().Context(), botID, id, req.CallbackURL)
	if err != nil {
		return pluginServiceError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

// RefreshOAuthStatus godoc
// @Summary Refresh managed OAuth status for a bot plugin
// @Tags plugins
// @Param bot_id path string true "Bot ID"
// @Param id path string true "Plugin installation ID"
// @Success 200 {object} plugins.Installation
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/plugins/{id}/oauth/status [get].
func (h *PluginsHandler) RefreshOAuthStatus(c echo.Context) error {
	botID, err := h.requireBotAccess(c)
	if err != nil {
		return err
	}
	id, err := pluginIDParam(c)
	if err != nil {
		return err
	}
	resp, err := h.service.RefreshOAuthStatusWithValidation(c.Request().Context(), botID, id, func(ctx context.Context, installation pluginspkg.Installation) error {
		return h.probeReadyPluginMCPs(ctx, botID, installation)
	})
	if err != nil {
		if errors.Is(err, pluginspkg.ErrPluginMCPProbeFailed) {
			h.logger.Warn("plugin MCP probe failed after OAuth refresh",
				slog.String("bot_id", botID),
				slog.String("installation_id", id),
				slog.Any("error", err))
			return pluginMCPProbeError(err)
		}
		return pluginServiceError(err)
	}
	return c.JSON(http.StatusOK, resp)
}

func pluginServiceError(err error) error {
	if errors.Is(err, pluginspkg.ErrManagedMCPNameConflict) || errors.Is(err, pluginspkg.ErrPluginAlreadyInstalled) {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, db.ErrNotFound) {
		return echo.NewHTTPError(http.StatusNotFound, "plugin installation not found")
	}
	if isPluginInternalConfigurationError(err) {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return echo.NewHTTPError(http.StatusBadRequest, err.Error())
}

func pluginMCPProbeError(err error) error {
	if isPluginInternalConfigurationError(err) {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return echo.NewHTTPError(http.StatusBadRequest, err.Error())
}

func isPluginInternalConfigurationError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "plugin service is not configured") ||
		strings.Contains(message, "mcp queries not configured") ||
		strings.Contains(message, "plugin MCP probe is not configured")
}
