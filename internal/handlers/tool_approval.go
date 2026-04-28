package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation/flow"
)

type ToolApprovalHandler struct {
	logger         *slog.Logger
	botService     *bots.Service
	accountService *accounts.Service
	resolver       *flow.Resolver
}

type ToolApprovalDecisionRequest struct {
	Reason string `json:"reason,omitempty"`
}

func NewToolApprovalHandler(log *slog.Logger, botService *bots.Service, accountService *accounts.Service, resolver *flow.Resolver) *ToolApprovalHandler {
	return &ToolApprovalHandler{
		logger:         log.With(slog.String("handler", "tool_approval")),
		botService:     botService,
		accountService: accountService,
		resolver:       resolver,
	}
}

func (h *ToolApprovalHandler) Register(e *echo.Echo) {
	group := e.Group("/bots/:bot_id/tool-approvals")
	group.POST("/:approval_id/approve", h.Approve)
	group.POST("/:approval_id/reject", h.Reject)
}

// Approve godoc
// @Summary Approve a pending tool call
// @Tags tool-approvals
// @Param bot_id path string true "Bot ID"
// @Param approval_id path string true "Approval ID"
// @Param payload body ToolApprovalDecisionRequest false "Approval payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/tool-approvals/{approval_id}/approve [post].
func (h *ToolApprovalHandler) Approve(c echo.Context) error {
	return h.respond(c, "approve")
}

// Reject godoc
// @Summary Reject a pending tool call
// @Tags tool-approvals
// @Param bot_id path string true "Bot ID"
// @Param approval_id path string true "Approval ID"
// @Param payload body ToolApprovalDecisionRequest false "Rejection payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/tool-approvals/{approval_id}/reject [post].
func (h *ToolApprovalHandler) Reject(c echo.Context) error {
	return h.respond(c, "reject")
}

func (h *ToolApprovalHandler) respond(c echo.Context, decision string) error {
	channelIdentityID, err := RequireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	approvalID := strings.TrimSpace(c.Param("approval_id"))
	if botID == "" || approvalID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot_id and approval_id are required")
	}
	if _, err := AuthorizeBotAccess(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID); err != nil {
		return err
	}
	var req ToolApprovalDecisionRequest
	_ = c.Bind(&req)
	if err := h.resolver.RespondToolApproval(context.WithoutCancel(c.Request().Context()), flow.ToolApprovalResponseInput{
		BotID:                  botID,
		ActorChannelIdentityID: channelIdentityID,
		ApprovalID:             approvalID,
		Decision:               decision,
		Reason:                 strings.TrimSpace(req.Reason),
	}, nil); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": decision})
}
