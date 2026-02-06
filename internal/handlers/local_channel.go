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

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/auth"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/identity"
	"github.com/memohai/memoh/internal/users"
)

type LocalChannelHandler struct {
	channelType    channel.ChannelType
	channelManager *channel.Manager
	channelService *channel.Service
	sessionHub     *channel.SessionHub
	botService     *bots.Service
	userService    *users.Service
}

func NewLocalChannelHandler(channelType channel.ChannelType, channelManager *channel.Manager, channelService *channel.Service, sessionHub *channel.SessionHub, botService *bots.Service, userService *users.Service) *LocalChannelHandler {
	return &LocalChannelHandler{
		channelType:    channelType,
		channelManager: channelManager,
		channelService: channelService,
		sessionHub:     sessionHub,
		botService:     botService,
		userService:    userService,
	}
}

func (h *LocalChannelHandler) Register(e *echo.Echo) {
	prefix := fmt.Sprintf("/bots/:bot_id/%s", h.channelType.String())
	group := e.Group(prefix)
	group.POST("/sessions", h.CreateSession)
	group.GET("/sessions/:session_id/stream", h.StreamSession)
	group.POST("/sessions/:session_id/messages", h.PostMessage)
}

type localSessionResponse struct {
	SessionID string `json:"session_id"`
	StreamURL string `json:"stream_url"`
}

func (h *LocalChannelHandler) CreateSession(c echo.Context) error {
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
	if h.channelService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "channel service not configured")
	}
	sessionID := fmt.Sprintf("%s:%s", h.channelType.String(), uuid.NewString())
	if err := h.channelService.UpsertChannelSession(c.Request().Context(), sessionID, botID, "", userID, "", h.channelType.String(), sessionID, "", nil); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	streamURL := fmt.Sprintf("/bots/%s/%s/sessions/%s/stream", botID, h.channelType.String(), sessionID)
	return c.JSON(http.StatusOK, localSessionResponse{SessionID: sessionID, StreamURL: streamURL})
}

func (h *LocalChannelHandler) StreamSession(c echo.Context) error {
	userID, err := h.requireUserID(c)
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
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	if err := h.ensureSessionOwner(c.Request().Context(), botID, sessionID, userID); err != nil {
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

func (h *LocalChannelHandler) PostMessage(c echo.Context) error {
	userID, err := h.requireUserID(c)
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
	if _, err := h.authorizeBotAccess(c.Request().Context(), userID, botID); err != nil {
		return err
	}
	if err := h.ensureSessionOwner(c.Request().Context(), botID, sessionID, userID); err != nil {
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
			ExternalID: userID,
			Attributes: map[string]string{
				"user_id": userID,
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

func (h *LocalChannelHandler) ensureSessionOwner(ctx context.Context, botID, sessionID, userID string) error {
	if h.channelService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "channel service not configured")
	}
	session, err := h.channelService.GetChannelSession(ctx, sessionID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if strings.TrimSpace(session.SessionID) == "" {
		return echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if session.BotID != botID {
		return echo.NewHTTPError(http.StatusForbidden, "session access denied")
	}
	if session.UserID != userID {
		return echo.NewHTTPError(http.StatusForbidden, "session access denied")
	}
	return nil
}

func (h *LocalChannelHandler) requireUserID(c echo.Context) (string, error) {
	userID, err := auth.UserIDFromContext(c)
	if err != nil {
		return "", err
	}
	if err := identity.ValidateUserID(userID); err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return userID, nil
}

func (h *LocalChannelHandler) authorizeBotAccess(ctx context.Context, actorID, botID string) (bots.Bot, error) {
	if h.botService == nil || h.userService == nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, "bot services not configured")
	}
	isAdmin, err := h.userService.IsAdmin(ctx, actorID)
	if err != nil {
		return bots.Bot{}, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	bot, err := h.botService.AuthorizeAccess(ctx, actorID, botID, isAdmin, bots.AccessPolicy{AllowPublicMember: true})
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
