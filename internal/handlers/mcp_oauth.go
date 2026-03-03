package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/mcp"
)

// MCPOAuthHandler handles OAuth-related endpoints for MCP connections.
type MCPOAuthHandler struct {
	oauthService   *mcp.OAuthService
	connService    *mcp.ConnectionService
	botService     *bots.Service
	accountService *accounts.Service
	logger         *slog.Logger
}

func NewMCPOAuthHandler(log *slog.Logger, oauthService *mcp.OAuthService, connService *mcp.ConnectionService, botService *bots.Service, accountService *accounts.Service) *MCPOAuthHandler {
	return &MCPOAuthHandler{
		oauthService:   oauthService,
		connService:    connService,
		botService:     botService,
		accountService: accountService,
		logger:         log.With(slog.String("handler", "mcp_oauth")),
	}
}

func (h *MCPOAuthHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/mcp/:id/oauth")
	group.POST("/discover", h.Discover)
	group.POST("/authorize", h.Authorize)
	group.GET("/status", h.Status)
	group.DELETE("/token", h.RevokeToken)

	e.GET("/api/oauth/mcp/callback", h.Callback)
}

type oauthDiscoverRequest struct {
	URL string `json:"url"`
}

// Discover godoc
// @Summary Discover OAuth configuration for MCP server
// @Description Probe MCP server URL for OAuth requirements and discover authorization server metadata
// @Tags mcp
// @Param id path string true "MCP connection ID"
// @Param payload body oauthDiscoverRequest false "Optional URL override"
// @Success 200 {object} mcp.DiscoveryResult
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp/{id}/oauth/discover [post]
func (h *MCPOAuthHandler) Discover(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	connID := strings.TrimSpace(c.Param("id"))
	if botID == "" || connID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id and id are required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	conn, err := h.connService.Get(c.Request().Context(), botID, connID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "mcp connection not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	var req oauthDiscoverRequest
	_ = c.Bind(&req)

	serverURL := strings.TrimSpace(req.URL)
	if serverURL == "" {
		if configURL, ok := conn.Config["url"].(string); ok {
			serverURL = strings.TrimSpace(configURL)
		}
	}
	if serverURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "MCP server URL is required for OAuth discovery")
	}

	result, err := h.oauthService.Discover(c.Request().Context(), serverURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if err := h.oauthService.SaveDiscovery(c.Request().Context(), connID, result); err != nil {
		h.logger.Error("failed to save discovery result", slog.Any("error", err))
		return echo.NewHTTPError(http.StatusInternalServerError, "failed to save discovery result")
	}

	return c.JSON(http.StatusOK, result)
}

type oauthAuthorizeRequest struct {
	ClientID string `json:"client_id"`
}

// Authorize godoc
// @Summary Start OAuth authorization flow
// @Description Generate PKCE and return authorization URL for the user to authorize
// @Tags mcp
// @Param id path string true "MCP connection ID"
// @Param payload body oauthAuthorizeRequest false "Optional client_id"
// @Success 200 {object} mcp.AuthorizeResult
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp/{id}/oauth/authorize [post]
func (h *MCPOAuthHandler) Authorize(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	connID := strings.TrimSpace(c.Param("id"))
	if botID == "" || connID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id and id are required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	var req oauthAuthorizeRequest
	_ = c.Bind(&req)

	result, err := h.oauthService.StartAuthorization(c.Request().Context(), connID, req.ClientID)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	return c.JSON(http.StatusOK, result)
}

// Callback godoc
// @Summary OAuth callback handler
// @Description Handles the OAuth authorization callback, exchanges code for tokens
// @Tags mcp
// @Param code query string true "Authorization code"
// @Param state query string true "State parameter"
// @Success 200 {string} string "HTML page that closes the popup"
// @Failure 400 {object} ErrorResponse
// @Router /api/oauth/mcp/callback [get]
func (h *MCPOAuthHandler) Callback(c echo.Context) error {
	code := strings.TrimSpace(c.QueryParam("code"))
	state := strings.TrimSpace(c.QueryParam("state"))
	errParam := strings.TrimSpace(c.QueryParam("error"))

	if errParam != "" {
		errDesc := c.QueryParam("error_description")
		return c.HTML(http.StatusOK, callbackHTML(false, fmt.Sprintf("Authorization failed: %s - %s", errParam, errDesc)))
	}

	if code == "" || state == "" {
		return c.HTML(http.StatusOK, callbackHTML(false, "Missing code or state parameter"))
	}

	_, err := h.oauthService.HandleCallback(c.Request().Context(), state, code)
	if err != nil {
		h.logger.Warn("oauth callback failed", slog.Any("error", err))
		return c.HTML(http.StatusOK, callbackHTML(false, err.Error()))
	}

	return c.HTML(http.StatusOK, callbackHTML(true, ""))
}

// Status godoc
// @Summary Get OAuth status for MCP connection
// @Description Returns the current OAuth status including whether tokens are available
// @Tags mcp
// @Param id path string true "MCP connection ID"
// @Success 200 {object} mcp.OAuthStatus
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp/{id}/oauth/status [get]
func (h *MCPOAuthHandler) Status(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	connID := strings.TrimSpace(c.Param("id"))
	if botID == "" || connID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id and id are required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	status, err := h.oauthService.GetStatus(c.Request().Context(), connID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, status)
}

// RevokeToken godoc
// @Summary Revoke OAuth tokens for MCP connection
// @Description Clears stored OAuth tokens
// @Tags mcp
// @Param id path string true "MCP connection ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Router /bots/{bot_id}/mcp/{id}/oauth/token [delete]
func (h *MCPOAuthHandler) RevokeToken(c echo.Context) error {
	userID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	connID := strings.TrimSpace(c.Param("id"))
	if botID == "" || connID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id and id are required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}

	if err := h.oauthService.RevokeToken(c.Request().Context(), connID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *MCPOAuthHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *MCPOAuthHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: false})
}

func callbackHTML(success bool, errMsg string) string {
	status := "success"
	message := "Authorization successful! You can close this window."
	if !success {
		status = "error"
		message = "Authorization failed: " + errMsg
	}
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><title>MCP OAuth</title></head>
<body>
<p>%s</p>
<script>
  if (window.opener) {
    window.opener.postMessage({ type: 'mcp-oauth-callback', status: '%s', error: '%s' }, '*');
    setTimeout(function() { window.close(); }, 1000);
  }
</script>
</body>
</html>`, message, status, strings.ReplaceAll(errMsg, "'", "\\'"))
}
