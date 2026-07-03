package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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
	botGroup := e.Group("/bots/:bot_id")
	botGroup.GET("/messages", h.ListMessages)
	botGroup.GET("/messages/locate", h.LocateMessage)
	botGroup.DELETE("/messages", h.DeleteMessages)
	botGroup.GET("/media/:content_hash", h.ServeMedia)

	// SSE streams. Per-session messages are subscribed explicitly by the
	// client; bot-wide activity carries only lightweight session metadata
	// (no message bodies) for sidebar live-sort.
	botGroup.GET("/sessions/:session_id/messages/events", h.StreamSessionMessageEvents)
	botGroup.GET("/sessions/events", h.StreamSessionsActivityEvents)
}

// --- Messages ---

func writeSSEData(writer io.Writer, flusher http.Flusher, payload string) error {
	// SSE frames are line-oriented; fold CR/LF to avoid frame injection.
	safePayload := strings.NewReplacer("\r", "\\r", "\n", "\\n").Replace(payload)
	if _, err := io.WriteString(writer, "data: "); err != nil {
		return err
	}
	if _, err := io.WriteString(writer, safePayload); err != nil { //nolint:gosec // G705: SSE body is plain text and CR/LF are escaped above
		return err
	}
	if _, err := io.WriteString(writer, "\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEJSON(writer io.Writer, flusher http.Flusher, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return writeSSEData(writer, flusher, string(data))
}

// ListMessages godoc
// @Summary List session history messages
// @Description List messages for one session with optional pagination
// @Tags messages
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param session_id query string true "Session ID"
// @Param limit query int false "Limit"
// @Param before query string false "Before"
// @Param before_message_id query string false "Message ID cursor before which to page"
// @Param format query string false "Response format: ui returns normalized chat UI turns"
// @Success 200 {object} map[string][]messagepkg.Message
// @Success 200 {object} map[string][]conversation.UITurn "when format=ui"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
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
	sessionID := strings.TrimSpace(c.QueryParam("session_id"))
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session_id is required")
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

	beforeMessageID := strings.TrimSpace(c.QueryParam("before_message_id"))
	if beforeMessageID != "" {
		if _, err := uuid.Parse(beforeMessageID); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid before_message_id")
		}
	}
	before, hasBefore := parseBeforeParam(c.QueryParam("before"))
	format := strings.ToLower(strings.TrimSpace(c.QueryParam("format")))

	bot, _, _, err := h.authorizeMessageSession(c, channelIdentityID, botID, sessionID)
	if err != nil {
		return err
	}
	botID = bot.ID

	var messages []messagepkg.Message
	switch {
	case beforeMessageID != "":
		messages, err = h.messageService.ListBeforeMessageBySession(c.Request().Context(), sessionID, beforeMessageID, limit)
	case hasBefore:
		// ListBeforeBySession already returns oldest-first (its converter
		// reverses the DESC DB rows). Do NOT reverse again: the before page
		// and the turn-head extension both depend on monotonic ASC input.
		messages, err = h.messageService.ListBeforeBySession(c.Request().Context(), sessionID, before, limit)
	default:
		messages, err = h.messageService.ListLatestBySession(c.Request().Context(), sessionID, limit)
		if err == nil {
			reverseMessages(messages)
		}
	}
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// format=ui converts each page independently, so a page that begins mid
	// assistant turn (its earlier rows on the previous page) would render one
	// reply as several turns/action bars. Extend the head back to a real turn
	// boundary so a turn is never split across pages.
	if format == "ui" && sessionID != "" && len(messages) > 0 {
		messages = h.extendToUITurnHead(c.Request().Context(), sessionID, messages)
	}
	h.fillAssetMimeFromStorage(c.Request().Context(), botID, messages)
	if format == "ui" {
		items := conversation.ConvertMessagesToUITurns(messages)
		h.decorateUITurns(c.Request().Context(), botID, sessionID, items)
		return c.JSON(http.StatusOK, map[string]any{
			"items": items,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"items": messages})
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
	h.decorateUITurns(c.Request().Context(), botID, sessionID, items)
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
	if err != nil || !session.IsACPRuntime(sess) {
		return defaultFn
	}
	return h.toolApproval.CanRespond
}

func (h *MessageHandler) decorateUITurns(ctx context.Context, botID, sessionID string, items []conversation.UITurn) {
	if len(items) == 0 {
		return
	}
	if h.bgManager != nil {
		conversation.ApplyBackgroundTaskSnapshots(items, h.backgroundTaskSnapshots(botID, sessionID))
	}
	toolCallIDs := toolCallIDsFromUITurns(items)
	if len(toolCallIDs) == 0 {
		return
	}
	if h.toolApproval != nil {
		if approvals, err := h.toolApproval.ListBySessionToolCalls(ctx, botID, sessionID, toolCallIDs); err == nil {
			mergeToolApprovals(items, approvals, h.toolApprovalCanApproveFn(ctx, sessionID))
		}
	}
	if h.userInput != nil {
		if requests, err := h.userInput.ListBySessionToolCalls(ctx, botID, sessionID, toolCallIDs); err == nil {
			mergeUserInputs(items, requests, h.userInput.CanRespond)
		}
	}
}

func toolCallIDsFromUITurns(turns []conversation.UITurn) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, turn := range turns {
		for _, msg := range turn.Messages {
			if msg.Type != conversation.UIMessageTool {
				continue
			}
			id := strings.TrimSpace(msg.ToolCallID)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
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
	for _, approval := range approvals {
		if callID := strings.TrimSpace(approval.ToolCallID); callID != "" {
			byCallID[callID] = approval
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
			approval, ok := byCallID[strings.TrimSpace(msg.ToolCallID)]
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
	for _, req := range requests {
		if callID := strings.TrimSpace(req.ToolCallID); callID != "" {
			byCallID[callID] = req
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
			req, ok := byCallID[strings.TrimSpace(msg.ToolCallID)]
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

// StreamMessageEvents was removed in favor of two narrower streams: a
// per-session messages SSE (see message_stream.go) and a bot-wide lightweight
// sessions activity SSE. Resolves a catch-up explosion where a stale client
// `since=` cursor could force a multi-megabyte replay of bot history.
// extendToUITurnHead prepends older session messages (oldest-first) until the
// slice starts on a real UI turn boundary — a visible user message or a
// background-task system turn. A turn is the unit of an action bar and is
// indivisible, so when a fixed-size page lands in the middle of an assistant
// turn we pull the turn's earlier rows back in. This makes the page larger than
// `limit`, which is fine: the frontend already pages by turns, not rows. The
// row cap guards against a single pathologically long turn.
func (h *MessageHandler) extendToUITurnHead(ctx context.Context, sessionID string, messages []messagepkg.Message) []messagepkg.Message {
	const batch = int32(50)
	const maxRows = 2000
	// messages is oldest-first (ASC). The cursor is the oldest row on the
	// current page, and we pull rows before that row in visible turn order.
	// ListBeforeMessageBySession already returns oldest-first, so prepend each
	// batch directly; the combined slice stays monotonic and the turn converter
	// (which scans in order) keeps one reply in a single turn.
	for len(messages) > 0 && len(messages) < maxRows && !conversation.IsUITurnBoundary(messages[0]) {
		older, err := h.messageService.ListBeforeMessageBySession(ctx, sessionID, messages[0].ID, batch)
		if err != nil || len(older) == 0 {
			break
		}
		messages = append(older, messages...)
		if len(older) < int(batch) {
			break // reached the start of the session
		}
	}
	return messages
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
