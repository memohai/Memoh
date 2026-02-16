package handlers

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/memohai/memoh/internal/media"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
)

// MessageHandler handles bot-scoped messaging endpoints.
type MessageHandler struct {
	runner              flow.Runner
	conversationService conversation.Accessor
	messageService      messagepkg.Service
	messageEvents       messageevent.Subscriber
	mediaService        *media.Service
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

// SetMediaService sets the optional media service for asset serving.
func (h *MessageHandler) SetMediaService(svc *media.Service) {
	h.mediaService = svc
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
	botGroup.GET("/media/:asset_id", h.ServeMedia)
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
	if strings.TrimSpace(req.Query) == "" && len(req.Attachments) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "query or attachments is required")
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
	if req.Attachments, err = h.ingestInlineAttachments(c.Request().Context(), botID, req.Attachments); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

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
	if strings.TrimSpace(req.Query) == "" && len(req.Attachments) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "query or attachments is required")
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
	if req.Attachments, err = h.ingestInlineAttachments(c.Request().Context(), botID, req.Attachments); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

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

func (h *MessageHandler) ingestInlineAttachments(ctx context.Context, botID string, attachments []conversation.ChatAttachment) ([]conversation.ChatAttachment, error) {
	if len(attachments) == 0 || h.mediaService == nil {
		return attachments, nil
	}
	result := make([]conversation.ChatAttachment, 0, len(attachments))
	for _, att := range attachments {
		item := att
		if strings.TrimSpace(item.AssetID) != "" || strings.TrimSpace(item.Base64) == "" {
			result = append(result, item)
			continue
		}
		mediaType := mapAttachmentMediaType(item.Type)
		maxBytes := media.MaxAssetBytes
		raw, err := decodeAttachmentBase64(item.Base64, maxBytes)
		if err != nil {
			return nil, fmt.Errorf("invalid attachment base64: %w", err)
		}
		asset, err := h.mediaService.Ingest(ctx, media.IngestInput{
			BotID:        botID,
			MediaType:    mediaType,
			Mime:         strings.TrimSpace(item.Mime),
			OriginalName: strings.TrimSpace(item.Name),
			Metadata:     item.Metadata,
			Reader:       raw,
			MaxBytes:     maxBytes,
		})
		if err != nil {
			return nil, fmt.Errorf("ingest attachment failed: %w", err)
		}
		item.AssetID = asset.ID
		item.Path = h.mediaService.AccessPath(asset)
		mime := strings.TrimSpace(item.Mime)
		if mime == "" {
			mime = strings.TrimSpace(asset.Mime)
		}
		item.Base64 = normalizeBase64DataURL(item.Base64, mime)
		if strings.TrimSpace(item.Mime) == "" {
			item.Mime = asset.Mime
		}
		result = append(result, item)
	}
	return result, nil
}

func decodeAttachmentBase64(input string, maxBytes int64) (io.Reader, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return nil, fmt.Errorf("base64 payload is empty")
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		if idx := strings.Index(value, ","); idx >= 0 {
			value = value[idx+1:]
		}
	}
	decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(value))
	return io.LimitReader(decoder, maxBytes+1), nil
}

func normalizeBase64DataURL(input, mime string) string {
	value := strings.TrimSpace(input)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "data:") {
		return value
	}
	mime = strings.TrimSpace(mime)
	if mime == "" {
		mime = "application/octet-stream"
	}
	return "data:" + mime + ";base64," + value
}

func mapAttachmentMediaType(t string) media.MediaType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "image", "gif":
		return media.MediaTypeImage
	case "audio", "voice":
		return media.MediaTypeAudio
	case "video":
		return media.MediaTypeVideo
	default:
		return media.MediaTypeFile
	}
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

// ListMessages godoc
// @Summary List bot history messages
// @Description List messages for a bot history with optional pagination
// @Tags messages
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param limit query int false "Limit"
// @Param before query string false "Before"
// @Success 200 {object} map[string][]messagepkg.Message
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/messages [get]
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

// ServeMedia streams a media asset by bot_id + asset_id with read-access authorization.
func (h *MessageHandler) ServeMedia(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	assetID := strings.TrimSpace(c.Param("asset_id"))
	if assetID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "asset id is required")
	}
	if _, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID); err != nil {
		return err
	}
	if err := h.requireReadable(c.Request().Context(), botID, channelIdentityID); err != nil {
		return err
	}
	if h.mediaService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "media service not configured")
	}
	reader, asset, err := h.mediaService.Open(c.Request().Context(), assetID)
	if err != nil {
		if errors.Is(err, media.ErrAssetNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "asset not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer reader.Close()
	// Verify asset belongs to the authorized bot.
	if strings.TrimSpace(asset.BotID) != botID {
		return echo.NewHTTPError(http.StatusForbidden, "asset does not belong to bot")
	}
	contentType := asset.Mime
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Cache-Control", "private, max-age=86400")
	if asset.OriginalName != "" {
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", asset.OriginalName))
	}
	c.Response().WriteHeader(http.StatusOK)
	if _, err := io.Copy(c.Response().Writer, reader); err != nil {
		h.logger.Warn("serve media stream failed", slog.Any("error", err))
	}
	return nil
}
