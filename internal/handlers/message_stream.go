package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/bots"
	messagepkg "github.com/memohai/memoh/internal/message"
	messageevent "github.com/memohai/memoh/internal/message/event"
	"github.com/memohai/memoh/internal/session"
)

// sessionMessageBacklogSize is the server-fixed number of backlog messages
// pushed when a client subscribes to a session message stream. Bounding the
// backlog server-side prevents the catch-up explosion that a client-supplied
// cursor allowed: a stale `since=` could replay the entire bot history.
const sessionMessageBacklogSize = 50

// StreamSessionMessageEvents godoc
// @Summary Stream message events for one session
// @Description SSE stream that pushes a server-fixed backlog of the last 50
// @Description messages, then streams future message_created and
// @Description session_title_updated events scoped to this session only.
// @Tags messages
// @Produce text/event-stream
// @Param bot_id path string true "Bot ID"
// @Param session_id path string true "Session ID"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/{session_id}/messages/events [get].
func (h *MessageHandler) StreamSessionMessageEvents(c echo.Context) error {
	channelIdentityID, err := h.requireChannelIdentityID(c)
	if err != nil {
		return err
	}
	botID := strings.TrimSpace(c.Param("bot_id"))
	sessionID := strings.TrimSpace(c.Param("session_id"))
	if botID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "bot id is required")
	}
	if sessionID == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "session id is required")
	}
	if h.messageService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message service not configured")
	}
	if h.messageEvents == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message events not configured")
	}

	bot, _, _, err := h.authorizeMessageSession(c, channelIdentityID, botID, sessionID)
	if err != nil {
		return err
	}
	botID = bot.ID

	writer, flusher, err := beginSSEResponse(c)
	if err != nil {
		return err
	}

	_, stream, cancel := h.messageEvents.Subscribe(botID, 128)
	defer cancel()

	backlog, err := h.messageService.ListLatestBySession(c.Request().Context(), sessionID, sessionMessageBacklogSize)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	reverseMessages(backlog)
	h.fillAssetMimeFromStorage(c.Request().Context(), botID, backlog)
	for _, message := range backlog {
		if err := writeMessageCreated(writer, flusher, botID, message); err != nil {
			return nil
		}
	}

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-heartbeat.C:
			if err := writeSSEJSON(writer, flusher, map[string]any{"type": "ping"}); err != nil {
				return nil
			}
		case event, ok := <-stream:
			if !ok {
				return nil
			}
			if strings.TrimSpace(event.BotID) != botID || len(event.Data) == 0 {
				continue
			}
			switch event.Type {
			case messageevent.EventTypeMessageCreated:
				var message messagepkg.Message
				if err := json.Unmarshal(event.Data, &message); err != nil {
					h.logger.Warn("decode message event failed", slog.Any("error", err))
					continue
				}
				if message.SessionID != sessionID {
					continue
				}
				h.fillAssetMimeFromStorage(c.Request().Context(), botID, []messagepkg.Message{message})
				if err := writeMessageCreated(writer, flusher, botID, message); err != nil {
					return nil
				}
			case messageevent.EventTypeSessionTitleUpdated:
				var payload map[string]string
				if err := json.Unmarshal(event.Data, &payload); err != nil {
					continue
				}
				if payload["session_id"] != sessionID {
					continue
				}
				if err := writeSSEJSON(writer, flusher, map[string]any{
					"type":       string(messageevent.EventTypeSessionTitleUpdated),
					"bot_id":     botID,
					"session_id": sessionID,
					"title":      payload["title"],
				}); err != nil {
					return nil
				}
			}
		}
	}
}

// StreamSessionsActivityEvents godoc
// @Summary Stream bot-wide sessions activity
// @Description Lightweight SSE for sidebar live-sort. Carries only session
// @Description identifiers and minimal metadata (touched timestamps, titles).
// @Description Never includes message bodies. Filters out internal session
// @Description types such as heartbeat, schedule, subagent.
// @Tags messages
// @Produce text/event-stream
// @Param bot_id path string true "Bot ID"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/sessions/events [get].
func (h *MessageHandler) StreamSessionsActivityEvents(c echo.Context) error {
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
	if h.messageEvents == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "message events not configured")
	}
	if h.sessionService == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "session service not configured")
	}

	writer, flusher, err := beginSSEResponse(c)
	if err != nil {
		return err
	}

	_, stream, cancel := h.messageEvents.Subscribe(botID, 128)
	defer cancel()

	cache := newSessionTypeCache(h.sessionService)

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-c.Request().Context().Done():
			return nil
		case <-heartbeat.C:
			if err := writeSSEJSON(writer, flusher, map[string]any{"type": "ping"}); err != nil {
				return nil
			}
		case event, ok := <-stream:
			if !ok {
				return nil
			}
			if strings.TrimSpace(event.BotID) != botID || len(event.Data) == 0 {
				continue
			}

			switch event.Type {
			case messageevent.EventTypeMessageCreated:
				var message messagepkg.Message
				if err := json.Unmarshal(event.Data, &message); err != nil {
					continue
				}
				if !h.canDeliverSessionActivity(c, channelIdentityID, botID, perms, cache, message.SessionID) {
					continue
				}
				if err := writeSSEJSON(writer, flusher, map[string]any{
					"type":       "session_touched",
					"session_id": message.SessionID,
					"updated_at": message.CreatedAt,
				}); err != nil {
					return nil
				}
			case messageevent.EventTypeSessionTitleUpdated:
				var payload map[string]string
				if err := json.Unmarshal(event.Data, &payload); err != nil {
					continue
				}
				sessionID := strings.TrimSpace(payload["session_id"])
				if !h.canDeliverSessionActivity(c, channelIdentityID, botID, perms, cache, sessionID) {
					continue
				}
				if err := writeSSEJSON(writer, flusher, map[string]any{
					"type":       "session_title_changed",
					"session_id": sessionID,
					"title":      payload["title"],
				}); err != nil {
					return nil
				}
			case messageevent.EventTypeSessionCreated:
				var payload map[string]any
				if err := json.Unmarshal(event.Data, &payload); err != nil {
					continue
				}
				sessionID, _ := payload["session_id"].(string)
				sessionID = strings.TrimSpace(sessionID)
				if sessionID == "" {
					continue
				}
				typ, _ := payload["type"].(string)
				if !session.IsUserFacingType(typ) {
					continue
				}
				cache.set(sessionID, typ, true)
				if !h.canReadMessageSession(c, channelIdentityID, botID, perms, sessionID) {
					continue
				}
				out := map[string]any{
					"type":       "session_created",
					"session_id": sessionID,
				}
				if title, ok := payload["title"].(string); ok && title != "" {
					out["title"] = title
				}
				if createdAt, ok := payload["created_at"].(string); ok && createdAt != "" {
					out["created_at"] = createdAt
				}
				if err := writeSSEJSON(writer, flusher, out); err != nil {
					return nil
				}
			}
		}
	}
}

// canDeliverSessionActivity returns true when the subscriber may see an
// activity event for sessionID: the session must be a user-facing type AND
// the subscriber must have read access to it.
func (h *MessageHandler) canDeliverSessionActivity(c echo.Context, channelIdentityID, botID string, perms []string, cache *sessionTypeCache, sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	userFacing, ok := cache.userFacing(c.Request().Context(), sessionID)
	if !ok || !userFacing {
		return false
	}
	if bots.HasPermission(perms, bots.PermissionManage) {
		return true
	}
	return h.canReadMessageSession(c, channelIdentityID, botID, perms, sessionID)
}

func writeMessageCreated(writer *bufio.Writer, flusher http.Flusher, botID string, message messagepkg.Message) error {
	return writeSSEJSON(writer, flusher, map[string]any{
		"type":    string(messageevent.EventTypeMessageCreated),
		"bot_id":  botID,
		"message": message,
	})
}

func beginSSEResponse(c echo.Context) (*bufio.Writer, http.Flusher, error) {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return nil, nil, echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}
	return bufio.NewWriter(c.Response().Writer), flusher, nil
}

// sessionTypeCache memoizes session.type lookups for the lifetime of one
// activity stream. Each event delivery would otherwise hit the DB to decide
// whether the session is user-facing.
type sessionTypeCache struct {
	svc   *session.Service
	mu    sync.Mutex
	known map[string]bool
}

func newSessionTypeCache(svc *session.Service) *sessionTypeCache {
	return &sessionTypeCache{svc: svc, known: map[string]bool{}}
}

func (c *sessionTypeCache) userFacing(ctx context.Context, sessionID string) (bool, bool) {
	c.mu.Lock()
	if cached, ok := c.known[sessionID]; ok {
		c.mu.Unlock()
		return cached, true
	}
	c.mu.Unlock()
	if c.svc == nil {
		return false, false
	}
	sess, err := c.svc.Get(ctx, sessionID)
	if err != nil {
		return false, false
	}
	userFacing := session.IsUserFacingType(sess.Type)
	c.mu.Lock()
	c.known[sessionID] = userFacing
	c.mu.Unlock()
	return userFacing, true
}

func (c *sessionTypeCache) set(sessionID, typ string, _ bool) {
	c.mu.Lock()
	c.known[sessionID] = session.IsUserFacingType(typ)
	c.mu.Unlock()
}
