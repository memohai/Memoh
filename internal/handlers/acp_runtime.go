package handlers

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/acpagent"
	"github.com/memohai/memoh/internal/acpclient"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/session"
)

type ACPRuntimeHandler struct {
	pool           acpRuntimePool
	sessionService *session.Service
	botService     *bots.Service
	accountService *accounts.Service
}

type acpRuntimePool interface {
	RuntimeStatus(sessionID, agentID, projectPath string) acpagent.RuntimeStatus
	Ensure(ctx context.Context, input acpagent.PromptInput) (acpagent.RuntimeStatus, error)
	SetModel(ctx context.Context, input acpagent.PromptInput, modelID string) (acpagent.RuntimeStatus, error)
}

type acpRuntimeModelRequest struct {
	ModelID string `json:"model_id"`
}

func NewACPRuntimeHandler(pool *acpagent.SessionPool, sessionService *session.Service, botService *bots.Service, accountService *accounts.Service) *ACPRuntimeHandler {
	return newACPRuntimeHandler(pool, sessionService, botService, accountService)
}

func newACPRuntimeHandler(pool acpRuntimePool, sessionService *session.Service, botService *bots.Service, accountService *accounts.Service) *ACPRuntimeHandler {
	return &ACPRuntimeHandler{
		pool:           pool,
		sessionService: sessionService,
		botService:     botService,
		accountService: accountService,
	}
}

func (h *ACPRuntimeHandler) Register(e *echo.Echo) {
	e.GET("/bots/:bot_id/sessions/:session_id/acp-runtime", h.GetRuntime)
	e.POST("/bots/:bot_id/sessions/:session_id/acp-runtime", h.EnsureRuntime)
	e.PATCH("/bots/:bot_id/sessions/:session_id/acp-runtime/model", h.SetModel)
}

// GetRuntime godoc
// @Summary Get ACP session runtime state
// @Tags acp
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 200 {object} acpagent.RuntimeStatus
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id}/acp-runtime [get].
func (h *ACPRuntimeHandler) GetRuntime(c echo.Context) error {
	_, sessionID, sess, err := h.authorizedACPSession(c)
	if err != nil {
		return err
	}
	status := h.pool.RuntimeStatus(sessionID, sessionMetadataString(sess.Metadata, "acp_agent_id"), sessionMetadataString(sess.Metadata, "project_path"))
	return c.JSON(http.StatusOK, status)
}

// EnsureRuntime godoc
// @Summary Ensure ACP session runtime is started
// @Tags acp
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 200 {object} acpagent.RuntimeStatus
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id}/acp-runtime [post].
func (h *ACPRuntimeHandler) EnsureRuntime(c echo.Context) error {
	botID, sessionID, sess, err := h.authorizedACPSession(c)
	if err != nil {
		return err
	}
	status, err := h.pool.Ensure(c.Request().Context(), acpagent.PromptInput{
		BotID:        botID,
		SessionID:    sessionID,
		AgentID:      sessionMetadataString(sess.Metadata, "acp_agent_id"),
		ProjectPath:  sessionMetadataString(sess.Metadata, "project_path"),
		SessionToken: extractRawBearerToken(c),
		ToolHTTPURL:  buildACPMCPToolsURL(c, botID),
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, status)
}

// SetModel godoc
// @Summary Set ACP session runtime model
// @Tags acp
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Param body body acpRuntimeModelRequest true "ACP model selection"
// @Success 200 {object} acpagent.RuntimeStatus
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id}/acp-runtime/model [patch].
func (h *ACPRuntimeHandler) SetModel(c echo.Context) error {
	botID, sessionID, sess, err := h.authorizedACPSession(c)
	if err != nil {
		return err
	}
	var req acpRuntimeModelRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	modelID := strings.TrimSpace(req.ModelID)
	if modelID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "model_id is required")
	}
	status, err := h.pool.SetModel(c.Request().Context(), acpagent.PromptInput{
		BotID:        botID,
		SessionID:    sessionID,
		AgentID:      sessionMetadataString(sess.Metadata, "acp_agent_id"),
		ProjectPath:  sessionMetadataString(sess.Metadata, "project_path"),
		SessionToken: extractRawBearerToken(c),
		ToolHTTPURL:  buildACPMCPToolsURL(c, botID),
	}, modelID)
	if err != nil {
		switch {
		case errors.Is(err, acpclient.ErrModelSelectionUnsupported),
			errors.Is(err, acpclient.ErrModelUnavailable),
			errors.Is(err, acpclient.ErrModelIDRequired):
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		case errors.Is(err, acpclient.ErrSessionNotInitialized),
			errors.Is(err, acpclient.ErrSessionClosed):
			return echo.NewHTTPError(http.StatusConflict, err.Error())
		default:
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	return c.JSON(http.StatusOK, status)
}

func (h *ACPRuntimeHandler) authorizedACPSession(c echo.Context) (string, string, session.Session, error) {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", "", session.Session{}, err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", "", session.Session{}, echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	bot, err := AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, bots.PermissionWorkspaceExec)
	if err != nil {
		return "", "", session.Session{}, err
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return "", "", session.Session{}, echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	sess, err := h.sessionService.Get(c.Request().Context(), sessionID)
	if err != nil || sess.BotID != bot.ID {
		return "", "", session.Session{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if sess.Type != session.TypeACPAgent {
		return "", "", session.Session{}, echo.NewHTTPError(http.StatusBadRequest, "session is not an ACP agent session")
	}
	perms, err := h.resolveCurrentUserPermissions(c, channelIdentityID, bot.ID)
	if err != nil {
		return "", "", session.Session{}, err
	}
	if !canAccessSession(sess, channelIdentityID, perms) {
		return "", "", session.Session{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	return bot.ID, sessionID, sess, nil
}

func (h *ACPRuntimeHandler) resolveCurrentUserPermissions(c echo.Context, channelIdentityID, botID string) ([]string, error) {
	if h.botService == nil || h.accountService == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.accountService.IsAdmin(c.Request().Context(), channelIdentityID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	perms, err := h.botService.ResolveUserPermissions(c.Request().Context(), botID, channelIdentityID, isAdmin)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return perms, nil
}

func buildACPMCPToolsURL(c echo.Context, botID string) string {
	if c == nil {
		return ""
	}
	return buildACPMCPToolsURLFromRequest(c.Request(), botID)
}

func buildACPMCPToolsURLFromRequest(req *http.Request, botID string) string {
	if raw := strings.TrimSpace(os.Getenv("MEMOH_ACP_MCP_HTTP_URL")); raw != "" {
		if strings.Contains(raw, "{bot_id}") {
			return strings.ReplaceAll(raw, "{bot_id}", url.PathEscape(strings.TrimSpace(botID)))
		}
		return raw
	}
	base := strings.TrimSpace(os.Getenv("MEMOH_ACP_MCP_HTTP_BASE_URL"))
	if base == "" {
		base = localRequestBaseURL(req)
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	return base + "/bots/" + url.PathEscape(strings.TrimSpace(botID)) + "/tools"
}

func localRequestBaseURL(req *http.Request) string {
	if req == nil {
		return ""
	}
	proto := "http"
	if req.TLS != nil {
		proto = "https"
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		return ""
	}
	if !isLoopbackRequestHost(host) {
		return ""
	}
	return proto + "://" + host
}

func isLoopbackRequestHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" || strings.Contains(host, "/") {
		return false
	}
	name := host
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		name = splitHost
	}
	name = strings.Trim(strings.TrimSpace(name), "[]")
	if strings.EqualFold(name, "localhost") {
		return true
	}
	ip := net.ParseIP(name)
	return ip != nil && ip.IsLoopback()
}
