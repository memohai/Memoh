package handlers

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	ctr "github.com/memohai/memoh/internal/container"
	"github.com/memohai/memoh/internal/db"
	"github.com/memohai/memoh/internal/userruntime"
	"github.com/memohai/memoh/internal/workspace"
)

type botRemoteRuntimeService interface {
	Bind(ctx context.Context, botID string, req workspace.BindRemoteWorkspaceRequest) (workspace.RemoteWorkspaceBinding, error)
	Get(ctx context.Context, botID string) (workspace.RemoteWorkspaceBinding, error)
	Unbind(ctx context.Context, botID string) error
}

type botWorkspaceStopper interface {
	StopBot(ctx context.Context, botID string) error
}

type BotRemoteRuntimeHandler struct {
	log        *slog.Logger
	service    botRemoteRuntimeService
	workspaces botWorkspaceStopper
	bots       *bots.Service
	accounts   *accounts.Service
}

func NewBotRemoteRuntimeHandler(log *slog.Logger, service *workspace.RemoteWorkspaceService, manager *workspace.Manager, botService *bots.Service, accountService *accounts.Service) *BotRemoteRuntimeHandler {
	if log == nil {
		log = slog.Default()
	}
	handler := &BotRemoteRuntimeHandler{
		log: log.With(slog.String("handler", "bot_remote_runtime")), service: service,
		bots: botService, accounts: accountService,
	}
	if manager != nil {
		handler.workspaces = manager
	}
	return handler
}

func (h *BotRemoteRuntimeHandler) Register(e *echo.Echo) {
	g := e.Group("/bots/:bot_id/remote-runtime")
	g.PUT("", h.Bind)
	g.GET("", h.Get)
	g.DELETE("", h.Unbind)
}

// Bind godoc
// @Summary Bind a Bot workspace to a Remote Runtime
// @Description Route the Bot's persistent workspace through a user-owned Remote Runtime. An omitted workspace_path uses bots/{bot_id}; multiple Bots may explicitly share the same path.
// @Tags bot-remote-runtime
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param request body workspace.BindRemoteWorkspaceRequest true "Remote workspace binding"
// @Success 200 {object} workspace.RemoteWorkspaceBinding
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/remote-runtime [put].
func (h *BotRemoteRuntimeHandler) Bind(c echo.Context) error {
	botID, err := h.requireManage(c)
	if err != nil {
		return err
	}
	var req workspace.BindRemoteWorkspaceRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Once bound, the container is hidden from status and StopBot refuses,
	// so a still-running container would keep consuming resources with no
	// stop path in the UI. Stop it before the first bind takes effect.
	h.stopStaleContainer(c.Request().Context(), botID)
	binding, err := h.service.Bind(c.Request().Context(), botID, req)
	if err != nil {
		return botRemoteRuntimeHTTPError(h.log, err)
	}
	return c.JSON(http.StatusOK, binding)
}

// stopStaleContainer is best-effort: an absent container or an existing
// binding (rebind) is normal, and a container backend failure must not
// block the binding itself.
func (h *BotRemoteRuntimeHandler) stopStaleContainer(ctx context.Context, botID string) {
	if h.workspaces == nil {
		return
	}
	err := h.workspaces.StopBot(ctx, botID)
	if err == nil || errors.Is(err, ctr.ErrNotSupported) || errors.Is(err, workspace.ErrContainerNotFound) {
		return
	}
	h.log.Warn("stop stale container before remote bind failed",
		slog.String("bot_id", botID), slog.Any("error", err))
}

// Get godoc
// @Summary Get a Bot's Remote Runtime workspace binding
// @Tags bot-remote-runtime
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 200 {object} workspace.RemoteWorkspaceBinding
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/remote-runtime [get].
func (h *BotRemoteRuntimeHandler) Get(c echo.Context) error {
	botID, err := h.requireManage(c)
	if err != nil {
		return err
	}
	binding, err := h.service.Get(c.Request().Context(), botID)
	if err != nil {
		return botRemoteRuntimeHTTPError(h.log, err)
	}
	return c.JSON(http.StatusOK, binding)
}

// Unbind godoc
// @Summary Remove a Bot's Remote Runtime workspace binding
// @Description Stops routing new workspace calls to the Remote Runtime. Remote files are not deleted.
// @Tags bot-remote-runtime
// @Param bot_id path string true "Bot ID"
// @Success 204 "No Content"
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/remote-runtime [delete].
func (h *BotRemoteRuntimeHandler) Unbind(c echo.Context) error {
	botID, err := h.requireManage(c)
	if err != nil {
		return err
	}
	if err := h.service.Unbind(c.Request().Context(), botID); err != nil {
		return botRemoteRuntimeHTTPError(h.log, err)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *BotRemoteRuntimeHandler) requireManage(c echo.Context) (string, error) {
	identityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return "", err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return "", echo.NewHTTPError(http.StatusBadRequest, "bot_id is required")
	}
	if _, err := AuthorizeBotAccessWithPermission(c.Request().Context(), h.bots, h.accounts, identityID, botID, bots.PermissionManage); err != nil {
		return "", err
	}
	return botID, nil
}

func botRemoteRuntimeHTTPError(log *slog.Logger, err error) error {
	switch {
	case errors.Is(err, workspace.ErrInvalidRemoteWorkspacePath), errors.Is(err, userruntime.ErrInvalidInput):
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	case errors.Is(err, workspace.ErrRemoteRuntimeNotUsable):
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	case errors.Is(err, workspace.ErrRemoteWorkspaceNotBound), errors.Is(err, db.ErrNotFound):
		return echo.NewHTTPError(http.StatusNotFound, "remote runtime binding not found")
	default:
		if log != nil {
			log.Error("bot remote runtime request failed", slog.Any("error", err))
		}
		return echo.NewHTTPError(http.StatusInternalServerError, "internal server error")
	}
}
