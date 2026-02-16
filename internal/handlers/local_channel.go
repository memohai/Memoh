package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/conversation"
)

// LocalChannelHandler handles local channel (CLI/Web) routes backed by bot history.
type LocalChannelHandler struct {
	channelType    channel.ChannelType
	channelManager *channel.Manager
	channelStore   *channel.Store
	chatService    *conversation.Service
	routeHub       *local.RouteHub
	botService     *bots.Service
	accountService *accounts.Service
}

// NewLocalChannelHandler creates a local channel handler.
func NewLocalChannelHandler(channelType channel.ChannelType, channelManager *channel.Manager, channelStore *channel.Store, chatService *conversation.Service, routeHub *local.RouteHub, botService *bots.Service, accountService *accounts.Service) *LocalChannelHandler {
	return &LocalChannelHandler{
		channelType:    channelType,
		channelManager: channelManager,
		channelStore:   channelStore,
		chatService:    chatService,
		routeHub:       routeHub,
		botService:     botService,
		accountService: accountService,
	}
}

// Register registers the local channel routes.
func (h *LocalChannelHandler) Register(e *echo.Echo) {
	prefix := fmt.Sprintf("/bots/:bot_id/%s", h.channelType.String())
	group := e.Group(prefix)
	group.GET("/stream", h.StreamMessages)
	group.POST("/messages", h.PostMessage)
}

// StreamMessages streams responses for the bot route.
func (h *LocalChannelHandler) StreamMessages(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
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
	if err := h.ensureBotParticipant(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}
	if h.routeHub == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "route hub not configured")
	}

	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}
	writer := bufio.NewWriter(c.Response().Writer)

	_, stream, cancel := h.routeHub.Subscribe(botID)
	defer cancel()

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case msg, ok := <-stream:
			if !ok {
				return nil
			}
			payload := map[string]any{
				"target": msg.Target,
				"event":  msg.Event,
			}
			data, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			if _, err := writer.WriteString(fmt.Sprintf("data: %s\n\n", string(data))); err != nil {
				return nil // client disconnected
			}
			writer.Flush()
			flusher.Flush()
		}
	}
}

type localMessageRequest struct {
	Message channel.Message `json:"message"`
}

// PostMessage sends a message through the local channel.
func (h *LocalChannelHandler) PostMessage(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
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
	if err := h.ensureBotParticipant(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}
	if h.channelManager == nil || h.channelStore == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "channel manager not configured")
	}
	var req localMessageRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Message.IsEmpty() {
		return echo.NewHTTPError(http.StatusBadRequest, "message is required")
	}
	cfg, err := h.channelStore.ResolveEffectiveConfig(c.Request().Context(), botID, h.channelType)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	routeKey := botID
	msg := channel.InboundMessage{
		Channel:     h.channelType,
		Message:     req.Message,
		BotID:       botID,
		ReplyTarget: routeKey,
		RouteKey:    routeKey,
		Sender: channel.Identity{
			SubjectID: channelIdentityID,
			Attributes: map[string]string{
				"user_id": channelIdentityID,
			},
		},
		Conversation: channel.Conversation{
			ID:   routeKey,
			Type: "p2p",
		},
		ReceivedAt: time.Now().UTC(),
		Source:     "local",
	}
	if err := h.channelManager.HandleInbound(c.Request().Context(), cfg, msg); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

func (h *LocalChannelHandler) ensureBotParticipant(ctx context.Context, botID, channelIdentityID string) error {
	if h.chatService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	ok, err := h.chatService.IsParticipant(ctx, botID, channelIdentityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusForbidden, "bot access denied")
	}
	return nil
}

func (h *LocalChannelHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *LocalChannelHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: true})
}
