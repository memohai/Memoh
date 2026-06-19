package handlers

import (
	"context"
	"encoding/json"
	"io"
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

// sessionMessageStreamBuffer sizes the per-subscriber channel for both SSE
// streams. Large enough to absorb a brief assistant-token burst without
// dropping; smaller buffers get truncated quickly under load.
const sessionMessageStreamBuffer = 128

// sseHeartbeatInterval is the keep-alive cadence — tuned to land under a
// 30s proxy idle cut.
const sseHeartbeatInterval = 20 * time.Second

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

	// Subscribe BEFORE the backlog read so any message persisted during the
	// DB call lands in the live channel. We then dedup against the backlog
	// IDs so the client never sees a message twice across the seam.
	_, stream, cancel := h.messageEvents.Subscribe(botID, sessionMessageStreamBuffer)

	backlog, err := h.messageService.ListLatestBySession(c.Request().Context(), sessionID, sessionMessageBacklogSize)
	if err != nil {
		// Cancel the subscription before returning the HTTP error so we
		// don't leak a channel — and so the framing stays JSON, not SSE.
		cancel()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer cancel()

	writer, flusher, err := beginSSEResponse(c)
	if err != nil {
		return err
	}

	reverseMessages(backlog)
	h.fillAssetMimeFromStorage(c.Request().Context(), botID, backlog)
	backlogIDs := make(map[string]struct{}, len(backlog))
	for _, message := range backlog {
		backlogIDs[message.ID] = struct{}{}
		if err := writeMessageCreated(writer, flusher, botID, message); err != nil {
			return nil
		}
	}

	heartbeat := time.NewTicker(sseHeartbeatInterval)
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
				// Skip messages already delivered as part of the backlog —
				// the Subscribe-before-backlog ordering keeps the seam
				// race-free at the cost of a small dedup set.
				if _, dup := backlogIDs[message.ID]; dup {
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
			case messageevent.EventTypeBackgroundTask, messageevent.EventTypeAgentStream:
				// Forward only to the owning session. The old bot-wide
				// stream forwarded these too — drop them here and the
				// chat UI loses live background-task / agent-stream
				// updates.
				var payload map[string]any
				if err := json.Unmarshal(event.Data, &payload); err != nil {
					continue
				}
				if payloadSessionID(payload) != sessionID {
					continue
				}
				payload["type"] = string(event.Type)
				payload["bot_id"] = botID
				if event.Type == messageevent.EventTypeBackgroundTask {
					if _, ok := payload["task"]; !ok {
						taskPayload := make(map[string]any, len(payload))
						for key, value := range payload {
							taskPayload[key] = value
						}
						payload["task"] = taskPayload
					}
				}
				if err := writeSSEJSON(writer, flusher, payload); err != nil {
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

	_, stream, cancel := h.messageEvents.Subscribe(botID, sessionMessageStreamBuffer)
	defer cancel()

	cache := newSessionTypeCache(h.sessionService)

	heartbeat := time.NewTicker(sseHeartbeatInterval)
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
				cache.setType(sessionID, typ)
				if !h.canDeliverSessionActivity(c, channelIdentityID, botID, perms, cache, sessionID) {
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
// the subscriber must have read access to it. Access decisions are memoized
// per stream so we don't hit the DB twice per event.
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
	if allowed, ok := cache.accessCached(sessionID, channelIdentityID); ok {
		return allowed
	}
	allowed := h.canReadMessageSession(c, channelIdentityID, botID, perms, sessionID)
	cache.storeAccess(sessionID, channelIdentityID, allowed)
	return allowed
}

func writeMessageCreated(writer io.Writer, flusher http.Flusher, botID string, message messagepkg.Message) error {
	return writeSSEJSON(writer, flusher, map[string]any{
		"type":    string(messageevent.EventTypeMessageCreated),
		"bot_id":  botID,
		"message": message,
	})
}

func beginSSEResponse(c echo.Context) (io.Writer, http.Flusher, error) {
	c.Response().Header().Set(echo.HeaderContentType, "text/event-stream")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	c.Response().Header().Set(echo.HeaderConnection, "keep-alive")
	c.Response().WriteHeader(http.StatusOK)
	flusher, ok := c.Response().Writer.(http.Flusher)
	if !ok {
		return nil, nil, echo.NewHTTPError(http.StatusInternalServerError, "streaming not supported")
	}
	return c.Response().Writer, flusher, nil
}

// sessionTypeCache memoizes per-session decisions for the lifetime of one
// activity stream. Each event delivery would otherwise hit the DB twice: once
// for the user-facing-type check and once for the access check. The cached
// access entry is keyed by channel identity to keep the cache correct when
// the same hub feeds multiple streams from different subscribers.
type sessionTypeCache struct {
	svc      *session.Service
	mu       sync.Mutex
	userFace map[string]bool
	access   map[accessKey]bool
}

type accessKey struct {
	sessionID         string
	channelIdentityID string
}

func newSessionTypeCache(svc *session.Service) *sessionTypeCache {
	return &sessionTypeCache{
		svc:      svc,
		userFace: map[string]bool{},
		access:   map[accessKey]bool{},
	}
}

func (c *sessionTypeCache) userFacing(ctx context.Context, sessionID string) (bool, bool) {
	c.mu.Lock()
	if cached, ok := c.userFace[sessionID]; ok {
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
	c.userFace[sessionID] = userFacing
	c.mu.Unlock()
	return userFacing, true
}

// setType primes the user-facing decision from an event payload that already
// carries the session type, sparing one DB round-trip for newly-created
// sessions whose type we just learned.
func (c *sessionTypeCache) setType(sessionID, typ string) {
	c.mu.Lock()
	c.userFace[sessionID] = session.IsUserFacingType(typ)
	c.mu.Unlock()
}

// accessCached returns the cached access decision for (session, identity).
func (c *sessionTypeCache) accessCached(sessionID, channelIdentityID string) (bool, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.access[accessKey{sessionID, channelIdentityID}]
	return v, ok
}

// storeAccess records an access decision for (session, identity).
func (c *sessionTypeCache) storeAccess(sessionID, channelIdentityID string, allowed bool) {
	c.mu.Lock()
	c.access[accessKey{sessionID, channelIdentityID}] = allowed
	c.mu.Unlock()
}

// payloadSessionID extracts the session id from an event payload, tolerating
// both snake_case and camelCase keys that legacy publishers may use.
func payloadSessionID(payload map[string]any) string {
	if v, _ := payload["session_id"].(string); v != "" {
		return v
	}
	v, _ := payload["sessionId"].(string)
	return v
}
