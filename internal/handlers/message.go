package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel/identities"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
)

// MessageHandler handles bot-scoped messaging endpoints.
type MessageHandler struct {
	runner              flow.Runner
	conversationService conversation.Accessor
	messageService      messagepkg.Service
	messageEvents       messageevent.Subscriber
	botService          *bots.Service
	accountService      *accounts.Service
	channelIdentitySvc  *identities.Service
	logger              *slog.Logger
}

// NewMessageHandler creates a MessageHandler.
func NewMessageHandler(log *slog.Logger, runner flow.Runner, conversationService conversation.Accessor, messageService messagepkg.Service, botService *bots.Service, accountService *accounts.Service, channelIdentitySvc *identities.Service, eventSubscribers ...messageevent.Subscriber) *MessageHandler {
	var messageEvents messageevent.Subscriber
	if len(eventSubscribers) > 0 {
		messageEvents = eventSubscribers[0]
	}
	return &MessageHandler{
		runner:              runner,
		conversationService: conversationService,
		messageService:      messageService,
		messageEvents:       messageEvents,
		botService:          botService,
		accountService:      accountService,
		channelIdentitySvc:  channelIdentitySvc,
		logger:              log.With(slog.String("handler", "conversation")),
	}
}

// Register registers all conversation routes.
func (h *MessageHandler) Register(e *echo.Echo) {
	// Bot-scoped message container (single shared history per bot).
	botGroup := e.Group("/bots/:bot_id")
	botGroup.POST("/messages", h.SendMessage)
	botGroup.POST("/messages/stream", h.StreamMessage)
	botGroup.GET("/messages", h.ListMessages)
	botGroup.GET("/messages/events", h.StreamMessageEvents)
	botGroup.DELETE("/messages", h.DeleteMessages)
}

// --- Messages ---

// SendMessage sends a synchronous conversation message.
func (h *MessageHandler) SendMessage(c echo.Context) error {
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
	if err := h.requireParticipant(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}

	var req conversation.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.BotID = botID
	req.ChatID = botID
	req.Token = c.Request().Header.Get("Authorization")
	req.UserID = channelIdentityID
	req.SourceChannelIdentityID = channelIdentityID
	if strings.TrimSpace(req.CurrentChannel) == "" {
		req.CurrentChannel = "web"
	}
	if strings.TrimSpace(req.ConversationType) == "" {
		req.ConversationType = "direct"
	}
	if len(req.Channels) == 0 {
		req.Channels = []string{req.CurrentChannel}
	}
	channelIdentityID = h.resolveWebChannelIdentity(c.Request().Context(), channelIdentityID, &req)

	if h.runner == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "conversation runner not configured")
	}
	resp, err := h.runner.Chat(c.Request().Context(), req)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, resp)
}

// StreamMessage sends a streaming conversation message.
func (h *MessageHandler) StreamMessage(c echo.Context) error {
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
	if err := h.requireParticipant(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}

	var req conversation.ChatRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "query is required")
	}
	req.BotID = botID
	req.ChatID = botID
	req.Token = c.Request().Header.Get("Authorization")
	req.UserID = channelIdentityID
	req.SourceChannelIdentityID = channelIdentityID
	if strings.TrimSpace(req.CurrentChannel) == "" {
		req.CurrentChannel = "web"
	}
	if strings.TrimSpace(req.ConversationType) == "" {
		req.ConversationType = "direct"
	}
	if len(req.Channels) == 0 {
		req.Channels = []string{req.CurrentChannel}
	}
	channelIdentityID = h.resolveWebChannelIdentity(c.Request().Context(), channelIdentityID, &req)

	if h.runner == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "conversation runner not configured")
	}
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	chunkChan, errChan := h.runner.StreamChat(c.Request().Context(), req)
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}
	writer := bufio.NewWriter(c.Response().Writer)
	processingState := "started"
	if err := writeSSEJSON(writer, flusher, map[string]string{"type": "processing_started"}); err != nil {
		return nil
	}

	for {
		select {
		case chunk, ok := <-chunkChan:
			if !ok {
				if processingState == "started" {
					processingState = "completed"
					if err := writeSSEJSON(writer, flusher, map[string]string{"type": "processing_completed"}); err != nil {
						return nil
					}
				}
				if err := writeSSEData(writer, flusher, "[DONE]"); err != nil {
					return nil
				}
				return nil
			}
			if processingState == "started" {
				processingState = "completed"
				if err := writeSSEJSON(writer, flusher, map[string]string{"type": "processing_completed"}); err != nil {
					return nil
				}
			}
			if err := writeSSEData(writer, flusher, string(chunk)); err != nil {
				return nil
			}
		case err := <-errChan:
			if err != nil {
				h.logger.Error("conversation stream failed", slog.Any("error", err))
				if processingState == "started" {
					processingState = "failed"
					if writeErr := writeSSEJSON(writer, flusher, map[string]string{
						"type":  "processing_failed",
						"error": err.Error(),
					}); writeErr != nil {
						h.logger.Warn("write SSE processing_failed event failed", slog.Any("error", writeErr))
					}
				}
				errData := map[string]string{
					"type":    "error",
					"error":   err.Error(),
					"message": err.Error(),
				}
				if writeErr := writeSSEJSON(writer, flusher, errData); writeErr != nil {
					return nil
				}
				return nil
			}
		}
	}
}

func writeSSEData(writer *bufio.Writer, flusher http.Flusher, payload string) error {
	if _, err := writer.WriteString(fmt.Sprintf("data: %s\n\n", payload)); err != nil {
		return err
	}
	if err := writer.Flush(); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEJSON(writer *bufio.Writer, flusher http.Flusher, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSEData(writer, flusher, string(data))
}

func parseSinceParam(raw string) (time.Time, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false, nil
	}
	layouts := []string{time.RFC3339Nano, time.RFC3339}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, trimmed)
		if err == nil {
			return parsed.UTC(), true, nil
		}
	}
	if epochMillis, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return time.UnixMilli(epochMillis).UTC(), true, nil
	}
	return time.Time{}, false, fmt.Errorf("invalid since parameter")
}

// ListMessages lists messages for a conversation with optional pagination.
// Query: limit (default 30), before (optional ISO8601 or unix ms) for older messages.
// Returns items in ascending created_at order (oldest first).
func (h *MessageHandler) ListMessages(c echo.Context) error {
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
	if err := h.requireReadable(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}

	if h.messageService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message service not configured")
	}

	limit := int32(30)
	if s := strings.TrimSpace(c.QueryParam("limit")); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}

	before, hasBefore := parseBeforeParam(c.QueryParam("before"))

	var messages []messagepkg.Message
	if hasBefore {
		messages, err = h.messageService.ListBefore(c.Request().Context(), botID, before, limit)
	} else {
		messages, err = h.messageService.ListLatest(c.Request().Context(), botID, limit)
		if err == nil {
			reverseMessages(messages)
		}
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]any{"items": messages})
}

func parseBeforeParam(s string) (time.Time, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return t.UTC(), true
	}
	if epochMillis, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return time.UnixMilli(epochMillis).UTC(), true
	}
	return time.Time{}, false
}

func reverseMessages(m []messagepkg.Message) {
	for i, j := 0, len(m)-1; i < j; i, j = i+1, j-1 {
		m[i], m[j] = m[j], m[i]
	}
}

// StreamMessageEvents streams bot-scoped message events to clients.
func (h *MessageHandler) StreamMessageEvents(c echo.Context) error {
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
	if err := h.requireReadable(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}
	if h.messageService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message service not configured")
	}
	if h.messageEvents == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message events not configured")
	}

	since, hasSince, err := parseSinceParam(c.QueryParam("since"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
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

	sentMessageIDs := map[string]struct{}{}
	writeCreatedEvent := func(message messagepkg.Message) error {
		msgID := strings.TrimSpace(message.ID)
		if msgID != "" {
			if _, exists := sentMessageIDs[msgID]; exists {
				return nil
			}
			sentMessageIDs[msgID] = struct{}{}
		}
		return writeSSEJSON(writer, flusher, map[string]any{
			"type":    string(messageevent.EventTypeMessageCreated),
			"bot_id":  botID,
			"message": message,
		})
	}

	_, stream, cancel := h.messageEvents.Subscribe(botID, 128)
	defer cancel()

	if hasSince {
		backlog, err := h.messageService.ListSince(c.Request().Context(), botID, since)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		for _, message := range backlog {
			if err := writeCreatedEvent(message); err != nil {
				return nil
			}
		}
	}

	heartbeatTicker := time.NewTicker(20 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-heartbeatTicker.C:
			if err := writeSSEJSON(writer, flusher, map[string]any{"type": "ping"}); err != nil {
				return nil
			}
		case event, ok := <-stream:
			if !ok {
				return nil
			}
			if strings.TrimSpace(event.BotID) != botID {
				continue
			}
			if event.Type != messageevent.EventTypeMessageCreated {
				continue
			}
			if len(event.Data) == 0 {
				continue
			}
			var message messagepkg.Message
			if err := json.Unmarshal(event.Data, &message); err != nil {
				h.logger.Warn("decode message event failed", slog.Any("error", err))
				continue
			}
			if err := writeCreatedEvent(message); err != nil {
				return nil
			}
		}
	}
}

// DeleteMessages clears all persisted bot-level history messages.
func (h *MessageHandler) DeleteMessages(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if _, err := h.authorizeBotManage(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	if h.messageService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message service not configured")
	}
	if err := h.messageService.DeleteByBot(c.Request().Context(), botID); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// --- helpers ---

// resolveWebChannelIdentity resolves (web, user_id) to a channel identity and sets req.SourceChannelIdentityID.
// Web uses user_id as the channel subject id (like Feishu open_id); the resolved ci has display_name and is linked to the user.
// Returns the channel_identity_id to use for the rest of the flow, or the original userID if resolution is skipped/fails.
func (h *MessageHandler) resolveWebChannelIdentity(ctx context.Context, userID string, req *conversation.ChatRequest) string {
	if strings.TrimSpace(req.CurrentChannel) != "web" || h.channelIdentitySvc == nil || strings.TrimSpace(userID) == "" {
		return userID
	}
	displayName := ""
	if h.accountService != nil {
		if account, err := h.accountService.Get(ctx, userID); err == nil {
			displayName = strings.TrimSpace(account.DisplayName)
			if displayName == "" {
				displayName = strings.TrimSpace(account.Username)
			}
		}
	}
	ci, err := h.channelIdentitySvc.ResolveByChannelIdentity(ctx, "web", userID, displayName, nil)
	if err != nil {
		return userID
	}
	if err := h.channelIdentitySvc.LinkChannelIdentityToUser(ctx, ci.ID, userID); err != nil {
		h.logger.Warn("link channel identity to user failed", slog.Any("error", err))
	}
	req.SourceChannelIdentityID = ci.ID
	return ci.ID
}

func (h *MessageHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *MessageHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: true})
}

func (h *MessageHandler) authorizeBotManage(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: false})
}

func (h *MessageHandler) requireParticipant(ctx context.Context, conversationID, channelIdentityID string) error {
	if h.conversationService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "conversation service not configured")
	}
	// Admin bypass.
	if h.accountService != nil {
		isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if isAdmin {
			return nil
		}
	}
	ok, err := h.conversationService.IsParticipant(ctx, conversationID, channelIdentityID)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !ok {
		return echo.NewHTTPError(http.StatusForbidden, "not a participant")
	}
	return nil
}

func (h *MessageHandler) requireReadable(ctx context.Context, conversationID, channelIdentityID string) error {
	if h.conversationService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "conversation service not configured")
	}
	// Admin bypass.
	if h.accountService != nil {
		isAdmin, err := h.accountService.IsAdmin(ctx, channelIdentityID)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if isAdmin {
			return nil
		}
	}
	_, err := h.conversationService.GetReadAccess(ctx, conversationID, channelIdentityID)
	if err != nil {
		if errors.Is(err, conversation.ErrPermissionDenied) {
			return echo.NewHTTPError(http.StatusForbidden, "not allowed to read conversation")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return nil
}
