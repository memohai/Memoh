package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/agent/background"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/media"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/session"
	"github.com/memohai/memoh/internal/toolapproval"
	"github.com/memohai/memoh/internal/userinput"
)

// MessageHandler handles bot-scoped messaging endpoints.
type MessageHandler struct {
	conversationService conversation.Accessor
	messageService      messagepkg.Service
	sessionService      *session.Service
	messageEvents       messageevent.Subscriber
	mediaService        *media.Service
	botService          *bots.Service
	accountService      *accounts.Service
	toolApproval        *toolapproval.Service
	userInput           *userinput.Service
	bgManager           *background.Manager
	logger              *slog.Logger
}

type sessionBeforeCursorLister interface {
	ListBeforeBySessionCursor(ctx context.Context, sessionID string, before time.Time, beforeID string, limit int32) ([]messagepkg.Message, error)
}

// NewMessageHandler creates a MessageHandler.
func NewMessageHandler(log *slog.Logger, conversationService conversation.Accessor, messageService messagepkg.Service, sessionService *session.Service, botService *bots.Service, accountService *accounts.Service, eventSubscribers ...messageevent.Subscriber) *MessageHandler {
	var messageEvents messageevent.Subscriber
	if len(eventSubscribers) > 0 {
		messageEvents = eventSubscribers[0]
	}
	return &MessageHandler{
		conversationService: conversationService,
		messageService:      messageService,
		sessionService:      sessionService,
		messageEvents:       messageEvents,
		botService:          botService,
		accountService:      accountService,
		logger:              log.With(slog.String("handler", "conversation")),
	}
}

// SetMediaService sets the optional media service for asset serving.
func (h *MessageHandler) SetMediaService(svc *media.Service) {
	h.mediaService = svc
}

func (h *MessageHandler) SetToolApprovalService(svc *toolapproval.Service) {
	h.toolApproval = svc
}

func (h *MessageHandler) SetUserInputService(svc *userinput.Service) {
	h.userInput = svc
}

func (h *MessageHandler) SetBackgroundManager(mgr *background.Manager) {
	h.bgManager = mgr
}

// Register registers all conversation routes.
func (h *MessageHandler) Register(e *echo.Echo) {
	// Bot-scoped message container (single shared history per bot).
	botGroup := e.Group("/bots/:bot_id")
	botGroup.GET("/messages", h.ListMessages)
	botGroup.GET("/messages/locate", h.LocateMessage)
	botGroup.GET("/messages/events", h.StreamMessageEvents)
	botGroup.DELETE("/messages", h.DeleteMessages)
	botGroup.GET("/media/:content_hash", h.ServeMedia)
}

// --- Messages ---

func writeSSEData(writer *bufio.Writer, flusher http.Flusher, payload string) error {
	// SSE frames are line-oriented; fold CR/LF to avoid frame injection.
	safePayload := strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(payload)
	if _, err := writer.WriteString("data: "); err != nil {
		return err
	}
	if _, err := writer.WriteString(safePayload); err != nil {
		return err
	}
	if _, err := writer.WriteString("\n\n"); err != nil {
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
	return time.Time{}, false, errors.New("invalid since parameter")
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
// @Router /bots/{bot_id}/messages [get].
func (h *MessageHandler) ListMessages(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
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
	beforeID := strings.TrimSpace(c.QueryParam("before_id"))
	format := strings.ToLower(strings.TrimSpace(c.QueryParam("format")))

	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID != "" {
		bot, _, _, err := h.authorizeMessageSession(c, channelIdentityID, botID, sessionID)
		if err != nil {
			return err
		}
		botID = bot.ID
	} else {
		bot, err := h.authorizeBotManage(c.Request().Context(), channelIdentityID, botID)
		if err != nil {
			return err
		}
		botID = bot.ID
	}

	if format == "ui" {
		if sessionID == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "session_id is required for ui format")
		}
		items, err := h.listSessionUITurns(c.Request().Context(), botID, sessionID, before, beforeID, hasBefore, limit)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusOK, map[string]any{
			"items": items,
		})
	}

	var messages []messagepkg.Message
	if sessionID != "" {
		if hasBefore {
			if cursorLister, ok := h.messageService.(sessionBeforeCursorLister); ok {
				messages, err = cursorLister.ListBeforeBySessionCursor(c.Request().Context(), sessionID, before, beforeID, limit)
			} else {
				messages, err = h.messageService.ListBeforeBySession(c.Request().Context(), sessionID, before, limit)
			}
		} else {
			messages, err = h.messageService.ListLatestBySession(c.Request().Context(), sessionID, limit)
			if err == nil {
				reverseMessages(messages)
			}
		}
	} else {
		if hasBefore {
			messages, err = h.messageService.ListBefore(c.Request().Context(), botID, before, limit)
		} else {
			messages, err = h.messageService.ListLatest(c.Request().Context(), botID, limit)
			if err == nil {
				reverseMessages(messages)
			}
		}
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.fillAssetMimeFromStorage(c.Request().Context(), botID, messages)
	return c.JSON(http.StatusOK, map[string]any{"items": messages})
}

func (h *MessageHandler) listSessionUITurns(ctx context.Context, botID, sessionID string, before time.Time, beforeID string, hasBefore bool, limit int32) ([]conversation.UITurn, error) {
	messages, err := h.messageService.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	h.fillAssetMimeFromStorage(ctx, botID, messages)
	items := conversation.ConvertMessagesToUITurns(messages)
	items = filterUITurnPage(items, before, beforeID, hasBefore, limit)
	h.applyUITurnOverlays(ctx, botID, sessionID, items)
	return items, nil
}

func (h *MessageHandler) applyUITurnOverlays(ctx context.Context, botID, sessionID string, items []conversation.UITurn) {
	if h.bgManager != nil {
		conversation.ApplyBackgroundTaskSnapshots(items, h.backgroundTaskSnapshots(botID, sessionID))
	}
	if h.toolApproval != nil {
		if approvals, err := h.toolApproval.ListBySession(ctx, botID, sessionID); err == nil {
			mergeToolApprovals(items, approvals, h.toolApprovalCanApproveFn(ctx, sessionID))
		}
	}
	if h.userInput != nil {
		if requests, err := h.userInput.ListBySession(ctx, botID, sessionID); err == nil {
			mergeUserInputs(items, requests, h.userInput.CanRespond)
		}
	}
}

func filterUITurnPage(items []conversation.UITurn, before time.Time, beforeID string, hasBefore bool, limit int32) []conversation.UITurn {
	if limit <= 0 {
		return nil
	}
	cursorID := strings.TrimSpace(beforeID)
	end := len(items)
	if hasBefore {
		end = 0
		foundCursor := false
		for idx, item := range items {
			if cursorID != "" && strings.TrimSpace(item.ID) == cursorID {
				end = idx
				foundCursor = true
				break
			}
			if cursorID == "" && item.Timestamp.Before(before) {
				end = idx + 1
			}
		}
		if cursorID != "" && !foundCursor {
			for idx, item := range items {
				if item.Timestamp.Before(before) {
					end = idx + 1
				}
			}
		}
	}
	if end <= 0 {
		return nil
	}
	start := end - int(limit)
	if start < 0 {
		start = 0
	}
	out := make([]conversation.UITurn, end-start)
	copy(out, items[start:end])
	return out
}

// LocateMessage godoc
// @Summary Locate a bot history message
// @Description Locate a session message by external message ID and return nearby UI turns
// @Tags messages
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param session_id query string true "Session ID"
// @Param external_message_id query string true "External message ID"
// @Param before query int false "Messages before target"
// @Param after query int false "Messages after target"
// @Success 200 {object} map[string]any
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/messages/locate [get].
func (h *MessageHandler) LocateMessage(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if h.messageService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message service not configured")
	}

	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
	}
	bot, _, _, err := h.authorizeMessageSession(c, channelIdentityID, botID, sessionID)
	if err != nil {
		return err
	}
	botID = bot.ID
	externalMessageID := strings.TrimSpace(c.QueryParam("external_message_id"))
	if externalMessageID == "" {
		externalMessageID = strings.TrimSpace(c.QueryParam("message_id"))
	}
	if externalMessageID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "external_message_id is required")
	}

	before := parseBoundedInt32(c.QueryParam("before"), 30, 0, 100)
	after := parseBoundedInt32(c.QueryParam("after"), 30, 0, 100)
	located, err := h.messageService.LocateByExternalIDBySession(c.Request().Context(), sessionID, externalMessageID, before, after)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "message not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.fillAssetMimeFromStorage(c.Request().Context(), botID, located.Messages)
	items := conversation.ConvertMessagesToUITurns(located.Messages)
	h.applyUITurnOverlays(c.Request().Context(), botID, sessionID, items)
	return c.JSON(http.StatusOK, map[string]any{
		"items":                      items,
		"target_id":                  located.TargetID,
		"target_external_message_id": externalMessageID,
	})
}

func parseBoundedInt32(raw string, fallback int32, minValue int32, maxValue int32) int32 {
	value := fallback
	if s := strings.TrimSpace(raw); s != "" {
		if n, err := strconv.ParseInt(s, 10, 32); err == nil {
			value = int32(n)
		}
	}
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func (h *MessageHandler) backgroundTaskSnapshots(botID, sessionID string) []conversation.UIBackgroundTask {
	if h.bgManager == nil {
		return nil
	}
	snapshots := h.bgManager.ListSnapshotsForSession(botID, sessionID)
	tasks := make([]conversation.UIBackgroundTask, 0, len(snapshots))
	for _, snapshot := range snapshots {
		status := string(snapshot.Status)
		stalled := snapshot.Stalled
		if stalled {
			status = "stalled"
		}
		label := snapshot.Command
		if label == "" {
			label = snapshot.Description
		}
		tasks = append(tasks, conversation.UIBackgroundTask{
			TaskID:         snapshot.TaskID,
			Status:         status,
			Command:        label,
			AgentID:        snapshot.AgentID,
			AgentSessionID: snapshot.AgentSessionID,
			OutputFile:     snapshot.OutputFile,
			ExitCode:       snapshot.ExitCode,
			Duration:       snapshot.Duration.Round(time.Millisecond).String(),
			OutputTail:     firstNonEmptyString(snapshot.OutputTail, snapshot.AgentReport, snapshot.AgentError),
			Stalled:        stalled,
		})
	}
	return tasks
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (h *MessageHandler) toolApprovalCanApproveFn(ctx context.Context, sessionID string) func(toolapproval.Request) bool {
	defaultFn := func(req toolapproval.Request) bool {
		return toolapproval.CanApprove(req.Status)
	}
	if h == nil || h.toolApproval == nil || h.sessionService == nil || strings.TrimSpace(sessionID) == "" {
		return defaultFn
	}
	sess, err := h.sessionService.Get(ctx, sessionID)
	if err != nil || sess.Type != session.TypeACPAgent {
		return defaultFn
	}
	return h.toolApproval.CanRespond
}

func mergeToolApprovals(turns []conversation.UITurn, approvals []toolapproval.Request, canApproveFn func(toolapproval.Request) bool) {
	if len(turns) == 0 || len(approvals) == 0 {
		return
	}
	if canApproveFn == nil {
		canApproveFn = func(req toolapproval.Request) bool {
			return toolapproval.CanApprove(req.Status)
		}
	}
	byCallID := make(map[string]toolapproval.Request, len(approvals))
	byTurnCallID := make(map[string]toolapproval.Request, len(approvals))
	for _, approval := range approvals {
		if callID := strings.TrimSpace(approval.ToolCallID); callID != "" {
			if turnID := strings.TrimSpace(approval.PersistTurnID); turnID != "" {
				byTurnCallID[turnToolCallKey(turnID, callID)] = approval
			} else {
				byCallID[callID] = approval
			}
		}
	}
	for turnIdx := range turns {
		if turns[turnIdx].Role != "assistant" {
			continue
		}
		for msgIdx := range turns[turnIdx].Messages {
			msg := &turns[turnIdx].Messages[msgIdx]
			if msg.Type != conversation.UIMessageTool {
				continue
			}
			callID := strings.TrimSpace(msg.ToolCallID)
			approval, ok := byTurnCallID[turnToolCallKey(turns[turnIdx].TurnID, callID)]
			if !ok {
				approval, ok = byCallID[callID]
			}
			if !ok {
				continue
			}
			running := false
			msg.Running = &running
			msg.Approval = &conversation.UIToolApproval{
				ApprovalID:     approval.ID,
				ShortID:        approval.ShortID,
				Status:         approval.Status,
				DecisionReason: approval.DecisionReason,
				CanApprove:     canApproveFn(approval),
			}
		}
	}
}

func mergeUserInputs(turns []conversation.UITurn, requests []userinput.Request, canRespondFn func(userinput.Request) bool) {
	if len(turns) == 0 || len(requests) == 0 {
		return
	}
	byCallID := make(map[string]userinput.Request, len(requests))
	byTurnCallID := make(map[string]userinput.Request, len(requests))
	for _, req := range requests {
		if callID := strings.TrimSpace(req.ToolCallID); callID != "" {
			if turnID := strings.TrimSpace(req.PersistTurnID); turnID != "" {
				byTurnCallID[turnToolCallKey(turnID, callID)] = req
			} else {
				byCallID[callID] = req
			}
		}
	}
	for turnIdx := range turns {
		if turns[turnIdx].Role != "assistant" {
			continue
		}
		for msgIdx := range turns[turnIdx].Messages {
			msg := &turns[turnIdx].Messages[msgIdx]
			if msg.Type != conversation.UIMessageTool {
				continue
			}
			callID := strings.TrimSpace(msg.ToolCallID)
			req, ok := byTurnCallID[turnToolCallKey(turns[turnIdx].TurnID, callID)]
			if !ok {
				req, ok = byCallID[callID]
			}
			if !ok {
				continue
			}
			running := false
			msg.Running = &running
			canRespond := req.Status == userinput.StatusPending
			if canRespondFn != nil {
				canRespond = canRespondFn(req)
			}
			msg.UserInput = &conversation.UIUserInput{
				UserInputID: req.ID,
				ShortID:     req.ShortID,
				Status:      req.Status,
				Questions:   req.UIPayload.Questions,
				CanRespond:  canRespond,
			}
		}
	}
}

func turnToolCallKey(turnID, callID string) string {
	return strings.TrimSpace(turnID) + "\x00" + strings.TrimSpace(callID)
}

// fillAssetMimeFromStorage fills mime, storage_key, size_bytes from storage (soft link: DB only has content_hash).
func (h *MessageHandler) fillAssetMimeFromStorage(ctx context.Context, botID string, messages []messagepkg.Message) {
	if h.mediaService == nil {
		return
	}
	for i := range messages {
		for j := range messages[i].Assets {
			a := &messages[i].Assets[j] //nolint:gosec // G602: j is bounded by range loop
			if strings.TrimSpace(a.ContentHash) == "" {
				continue
			}
			asset, err := h.mediaService.Resolve(ctx, botID, a.ContentHash)
			if err != nil {
				continue
			}
			a.Mime = asset.Mime
			a.StorageKey = asset.StorageKey
			a.SizeBytes = asset.SizeBytes
		}
	}
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
	bot, perms, err := h.authorizeBotMessageAccess(c, channelIdentityID, botID)
	if err != nil {
		return err
	}
	botID = bot.ID
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
		backlog, err := h.messageBacklogSince(c, channelIdentityID, botID, perms, since)
		if err != nil {
			return err
		}
		h.fillAssetMimeFromStorage(c.Request().Context(), botID, backlog)
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
			if len(event.Data) == 0 {
				continue
			}
			switch event.Type {
			case messageevent.EventTypeMessageCreated:
				var message messagepkg.Message
				if err := json.Unmarshal(event.Data, &message); err != nil {
					h.logger.Warn("decode message event failed", slog.Any("error", err))
					continue
				}
				if !h.canReadMessageSession(c, channelIdentityID, botID, perms, message.SessionID) {
					continue
				}
				h.fillAssetMimeFromStorage(c.Request().Context(), botID, []messagepkg.Message{message})
				if err := writeCreatedEvent(message); err != nil {
					return nil
				}
			case messageevent.EventTypeSessionTitleUpdated:
				var payload map[string]string
				if err := json.Unmarshal(event.Data, &payload); err != nil {
					continue
				}
				if !h.canReadMessageSession(c, channelIdentityID, botID, perms, payload["session_id"]) {
					continue
				}
				if err := writeSSEJSON(writer, flusher, map[string]any{
					"type":       string(messageevent.EventTypeSessionTitleUpdated),
					"bot_id":     botID,
					"session_id": payload["session_id"],
					"title":      payload["title"],
				}); err != nil {
					return nil
				}
			case messageevent.EventTypeBackgroundTask:
				var payload map[string]any
				if err := json.Unmarshal(event.Data, &payload); err != nil {
					continue
				}
				if !h.canReadPayloadSession(c, channelIdentityID, botID, perms, payload) {
					continue
				}
				payload["type"] = string(messageevent.EventTypeBackgroundTask)
				payload["bot_id"] = botID
				if _, ok := payload["task"]; !ok {
					taskPayload := make(map[string]any, len(payload))
					for key, value := range payload {
						taskPayload[key] = value
					}
					payload["task"] = taskPayload
				}
				if err := writeSSEJSON(writer, flusher, payload); err != nil {
					return nil
				}
			case messageevent.EventTypeAgentStream:
				var payload map[string]any
				if err := json.Unmarshal(event.Data, &payload); err != nil {
					continue
				}
				if !h.canReadPayloadSession(c, channelIdentityID, botID, perms, payload) {
					continue
				}
				payload["type"] = string(messageevent.EventTypeAgentStream)
				payload["bot_id"] = botID
				if err := writeSSEJSON(writer, flusher, payload); err != nil {
					return nil
				}
			}
		}
	}
}

// DeleteMessages godoc
// @Summary Delete all bot history messages
// @Description Clear all persisted bot-level history messages
// @Tags messages
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Success 204 "No Content"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/messages [delete].
func (h *MessageHandler) DeleteMessages(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	bot, err := h.authorizeBotManage(c.Request().Context(), channelIdentityID, botID)
	if err != nil {
		return err
	}
	botID = bot.ID
	if h.messageService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message service not configured")
	}
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID != "" {
		if err := h.messageService.DeleteBySession(c.Request().Context(), sessionID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	} else {
		if err := h.messageService.DeleteByBot(c.Request().Context(), botID); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	return c.NoContent(http.StatusNoContent)
}

// --- helpers ---

func (*MessageHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *MessageHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccessWithPermission(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.PermissionChat)
}

func (h *MessageHandler) authorizeBotManage(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccessWithPermission(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.PermissionManage)
}

func (h *MessageHandler) authorizeBotMessageAccess(c echo.Context, channelIdentityID, botID string) (bots.Bot, []string, error) {
	bot, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID)
	if err != nil {
		bot, err = AuthorizeBotAccessWithPermission(c.Request().Context(), h.botService, h.accountService, channelIdentityID, botID, bots.PermissionWorkspaceExec)
		if err != nil {
			return bots.Bot{}, nil, err
		}
	}
	perms, err := h.resolveCurrentUserPermissions(c, channelIdentityID, bot.ID)
	if err != nil {
		return bots.Bot{}, nil, err
	}
	return bot, perms, nil
}

func (h *MessageHandler) authorizeMessageSession(c echo.Context, channelIdentityID, botID, sessionID string) (bots.Bot, []string, session.Session, error) {
	if h.sessionService == nil {
		return bots.Bot{}, nil, session.Session{}, echo.NewHTTPError(http.StatusInternalServerError, "session service not configured")
	}
	bot, perms, err := h.authorizeBotMessageAccess(c, channelIdentityID, botID)
	if err != nil {
		return bots.Bot{}, nil, session.Session{}, err
	}
	sess, err := h.sessionService.Get(c.Request().Context(), sessionID)
	if err != nil || sess.BotID != bot.ID {
		return bots.Bot{}, nil, session.Session{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	if !canAccessSession(sess, channelIdentityID, perms) {
		return bots.Bot{}, nil, session.Session{}, echo.NewHTTPError(http.StatusNotFound, "session not found")
	}
	return bot, perms, sess, nil
}

func (h *MessageHandler) resolveCurrentUserPermissions(c echo.Context, channelIdentityID, botID string) ([]string, error) {
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

func (h *MessageHandler) messageBacklogSince(c echo.Context, channelIdentityID, botID string, perms []string, since time.Time) ([]messagepkg.Message, error) {
	if bots.HasPermission(perms, bots.PermissionManage) {
		backlog, err := h.messageService.ListSince(c.Request().Context(), botID, since)
		if err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return backlog, nil
	}
	if h.sessionService == nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, "session service not configured")
	}
	sessions, err := h.sessionService.ListByBotAndCreatedByUser(c.Request().Context(), botID, channelIdentityID)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	sessions = filterSessionsForPermissions(sessions, channelIdentityID, perms)
	backlog := make([]messagepkg.Message, 0)
	for _, sess := range sessions {
		items, err := h.messageService.ListSinceBySession(c.Request().Context(), sess.ID, since)
		if err != nil {
			return nil, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		backlog = append(backlog, items...)
	}
	sort.SliceStable(backlog, func(i, j int) bool {
		return backlog[i].CreatedAt.Before(backlog[j].CreatedAt)
	})
	return backlog, nil
}

func (h *MessageHandler) canReadPayloadSession(c echo.Context, channelIdentityID, botID string, perms []string, payload map[string]any) bool {
	sessionID, _ := payload["session_id"].(string)
	if sessionID == "" {
		sessionID, _ = payload["sessionId"].(string)
	}
	return h.canReadMessageSession(c, channelIdentityID, botID, perms, sessionID)
}

func (h *MessageHandler) canReadMessageSession(c echo.Context, channelIdentityID, botID string, perms []string, sessionID string) bool {
	if bots.HasPermission(perms, bots.PermissionManage) {
		return true
	}
	if h.sessionService == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	sess, err := h.sessionService.Get(c.Request().Context(), sessionID)
	if err != nil || sess.BotID != botID {
		return false
	}
	return canAccessSession(sess, channelIdentityID, perms)
}

// ServeMedia streams a media asset by bot_id + content_hash with read-access authorization.
func (h *MessageHandler) ServeMedia(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	contentHash := strings.TrimSpace(c.Param("content_hash"))
	if contentHash == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "content hash is required")
	}
	bot, err := h.authorizeBotAccess(c.Request().Context(), channelIdentityID, botID)
	if err != nil {
		return err
	}
	botID = bot.ID
	if h.mediaService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "media service not configured")
	}
	reader, asset, err := h.mediaService.Open(c.Request().Context(), botID, contentHash)
	if err != nil {
		if errors.Is(err, media.ErrAssetNotFound) {
			return echo.NewHTTPError(http.StatusNotFound, "asset not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer func() { _ = reader.Close() }()
	contentType := asset.Mime
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Response().Header().Set("Content-Type", contentType)
	c.Response().Header().Set("Cache-Control", "private, max-age=86400")
	c.Response().WriteHeader(http.StatusOK)
	if _, err := io.Copy(c.Response().Writer, reader); err != nil {
		h.logger.Warn("serve media stream failed", slog.Any("error", err))
	}
	return nil
}
