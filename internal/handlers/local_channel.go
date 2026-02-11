package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/chat"
	"github.com/memohai/memoh/internal/identity"
)

// LocalChannelHandler handles local channel (CLI/Web) sessions backed by chats.
type LocalChannelHandler struct {
	channelType    channel.ChannelType
	channelManager *channel.Manager
	channelService *channel.Service
	chatService    *chat.Service
	sessionHub     *local.SessionHub
	botService     *bots.Service
	accountService *accounts.Service
}

// NewLocalChannelHandler creates a local channel handler.
func NewLocalChannelHandler(channelType channel.ChannelType, channelManager *channel.Manager, channelService *channel.Service, chatService *chat.Service, sessionHub *local.SessionHub, botService *bots.Service, accountService *accounts.Service) *LocalChannelHandler {
	return &LocalChannelHandler{
		channelType:    channelType,
		channelManager: channelManager,
		channelService: channelService,
		chatService:    chatService,
		sessionHub:     sessionHub,
		botService:     botService,
		accountService: accountService,
	}
}

// Register registers the local channel routes.
func (h *LocalChannelHandler) Register(e *echo.Echo) {
	prefix := fmt.Sprintf("/bots/:bot_id/%s", h.channelType.String())
	group := e.Group(prefix)
	group.POST("/sessions", h.CreateSession)
	group.GET("/sessions/:session_id/stream", h.StreamSession)
	group.POST("/sessions/:session_id/messages", h.PostMessage)
}

type localSessionResponse struct {
	SessionID string `json:"session_id"`
	ChatID    string `json:"chat_id"`
	StreamURL string `json:"stream_url"`
}

// CreateSession creates a new local chat session.
func (h *LocalChannelHandler) CreateSession(c echo.Context) error {
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

	// Create a chat as the underlying container.
	chatObj, err := h.chatService.Create(c.Request().Context(), botID, channelIdentityID, chat.CreateChatRequest{
		Kind: chat.KindDirect,
	})
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Use chat_id as the session_id for the local hub.
	sessionID := chatObj.ID
	streamURL := fmt.Sprintf("/bots/%s/%s/sessions/%s/stream", botID, h.channelType.String(), sessionID)
	return c.JSON(http.StatusOK, localSessionResponse{SessionID: sessionID, ChatID: chatObj.ID, StreamURL: streamURL})
}

// StreamSession streams responses for a local session.
func (h *LocalChannelHandler) StreamSession(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	if err := h.ensureChatParticipant(c.Request().Context(), sessionID, channelIdentityID); err != nil {
		return err
	}
	if h.sessionHub == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session hub not configured")
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

	_, stream, cancel := h.sessionHub.Subscribe(sessionID)
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
				"target":  msg.Target,
				"message": msg.Message,
			}
			data, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			_, _ = writer.WriteString(fmt.Sprintf("data: %s\n\n", string(data)))
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
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	if err := h.ensureChatParticipant(c.Request().Context(), sessionID, channelIdentityID); err != nil {
		return err
	}
	if h.channelManager == nil || h.channelService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "channel manager not configured")
	}
	var req localMessageRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	text := strings.TrimSpace(req.Message.PlainText())
	if text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "message is required")
	}
	cfg, err := h.channelService.ResolveEffectiveConfig(c.Request().Context(), botID, h.channelType)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	msg := channel.InboundMessage{
		Channel:     h.channelType,
		Message:     req.Message,
		BotID:       botID,
		ReplyTarget: sessionID,
		SessionKey:  sessionID,
		Sender: channel.Identity{
			SubjectID: channelIdentityID,
			Attributes: map[string]string{
				"user_id": channelIdentityID,
			},
		},
		Conversation: channel.Conversation{
			ID:   sessionID,
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

func (h *LocalChannelHandler) ensureChatParticipant(ctx context.Context, chatID, channelIdentityID string) error {
	if h.chatService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "chat service not configured")
	}
	ok, err := h.chatService.IsParticipant(ctx, chatID, channelIdentityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusForbidden, "chat access denied")
	}
	return nil
}

func (h *LocalChannelHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	channelIdentityID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateChannelIdentityID(channelIdentityID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return channelIdentityID, nil
}

func (h *LocalChannelHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.accountService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, channelIdentityID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: true})
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
