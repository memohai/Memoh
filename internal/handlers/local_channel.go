package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
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
	resolver       *flow.Resolver
	logger         *slog.Logger
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
		logger:         slog.Default().With(slog.String("handler", "local_channel")),
	}
}

// SetResolver sets the flow resolver for WebSocket streaming.
func (h *LocalChannelHandler) SetResolver(resolver *flow.Resolver) {
	h.resolver = resolver
}

// Register registers the local channel routes.
func (h *LocalChannelHandler) Register(e *echo.Echo) {
	prefix := fmt.Sprintf("/bots/:bot_id/%s", h.channelType.String())
	group := e.Group(prefix)
	group.GET("/stream", h.StreamMessages)
	group.POST("/messages", h.PostMessage)
	group.GET("/ws", h.HandleWebSocket)
}

// StreamMessages godoc
// @Summary Subscribe to local channel events via SSE
// @Description Open a persistent SSE connection to receive real-time stream events for the given bot.
// @Tags local-channel
// @Produce text/event-stream
// @Param bot_id path string true "Bot ID"
// @Success 200 {string} string "SSE stream"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/web/stream [get]
// @Router /bots/{bot_id}/cli/stream [get].
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
			data, err := formatLocalStreamEvent(msg.Event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(writer, "data: %s\n\n", string(data)); err != nil {
				return nil // client disconnected
			}
			if err := writer.Flush(); err != nil {
				return nil
			}
			flusher.Flush()
		}
	}
}

func formatLocalStreamEvent(event channel.StreamEvent) ([]byte, error) {
	return json.Marshal(event)
}

// LocalChannelMessageRequest is the request body for posting a local channel message.
type LocalChannelMessageRequest struct {
	Message channel.Message `json:"message"`
}

// PostMessage godoc
// @Summary Send a message to a local channel
// @Description Post a user message (with optional attachments) through the local channel pipeline.
// @Tags local-channel
// @Accept json
// @Produce json
// @Param bot_id path string true "Bot ID"
// @Param payload body LocalChannelMessageRequest true "Message payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/web/messages [post]
// @Router /bots/{bot_id}/cli/messages [post].
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
	var req LocalChannelMessageRequest
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

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(_ *http.Request) bool { return true },
}

type wsClientMessage struct {
	Type        string            `json:"type"`
	Text        string            `json:"text,omitempty"`
	Attachments []json.RawMessage `json:"attachments,omitempty"`
}

// wsWriter serialises all WebSocket writes through a single goroutine to
// avoid concurrent write panics with gorilla/websocket.
type wsWriter struct {
	conn *websocket.Conn
	ch   chan []byte
	done chan struct{}
}

func newWSWriter(conn *websocket.Conn) *wsWriter {
	w := &wsWriter{
		conn: conn,
		ch:   make(chan []byte, 128),
		done: make(chan struct{}),
	}
	go w.loop()
	return w
}

func (w *wsWriter) loop() {
	defer close(w.done)
	for data := range w.ch {
		_ = w.conn.WriteMessage(websocket.TextMessage, data)
	}
}

func (w *wsWriter) Send(data []byte) {
	select {
	case w.ch <- data:
	case <-w.done:
	}
}

func (w *wsWriter) SendJSON(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	w.Send(data)
}

func (w *wsWriter) Close() {
	close(w.ch)
	<-w.done
}

// extractRawBearerToken returns the raw JWT token suitable for passing to the
// gateway. The gateway WS handler receives the token directly (not as an HTTP
// header), so we must strip the "Bearer " prefix if present.
func extractRawBearerToken(c echo.Context) string {
	auth := strings.TrimSpace(c.Request().Header.Get("Authorization"))
	if auth != "" {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return strings.TrimSpace(c.QueryParam("token"))
}

// HandleWebSocket godoc
// @Summary WebSocket chat endpoint
// @Description Upgrade to WebSocket for bidirectional chat streaming with abort support.
// @Tags local-channel
// @Param bot_id path string true "Bot ID"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/web/ws [get]
// @Router /bots/{bot_id}/cli/ws [get].
func (h *LocalChannelHandler) HandleWebSocket(c echo.Context) error {
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
	if h.resolver == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "resolver not configured")
	}

	conn, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	rawToken := extractRawBearerToken(c)
	bearerToken := "Bearer " + rawToken

	writer := newWSWriter(conn)
	defer writer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	abortCh := make(chan struct{}, 1)
	var activeCancel context.CancelFunc

	for {
		_, raw, readErr := conn.ReadMessage()
		if readErr != nil {
			cancel()
			break
		}
		var msg wsClientMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			writer.SendJSON(map[string]string{"type": "error", "message": "invalid message format"})
			continue
		}

		switch msg.Type {
		case "abort":
			select {
			case abortCh <- struct{}{}:
			default:
			}

		case "message":
			text := strings.TrimSpace(msg.Text)
			if text == "" {
				writer.SendJSON(map[string]string{"type": "error", "message": "message text is required"})
				continue
			}

			chatAttachments := make([]conversation.ChatAttachment, 0, len(msg.Attachments))
			for _, rawAtt := range msg.Attachments {
				var att conversation.ChatAttachment
				if err := json.Unmarshal(rawAtt, &att); err == nil {
					chatAttachments = append(chatAttachments, att)
				}
			}

			// Drain any previous abort signal.
			select {
			case <-abortCh:
			default:
			}

			streamCtx, streamCancel := context.WithCancel(ctx)
			activeCancel = streamCancel
			eventCh := make(chan flow.WSStreamEvent, 64)

			go func() {
				defer streamCancel()
				defer close(eventCh)
				req := conversation.ChatRequest{
					BotID:                   botID,
					ChatID:                  botID,
					Token:                   bearerToken,
					UserID:                  channelIdentityID,
					SourceChannelIdentityID: channelIdentityID,
					ConversationType:        "p2p",
					Query:                   text,
					CurrentChannel:          h.channelType.String(),
					Channels:                []string{h.channelType.String()},
					Attachments:             chatAttachments,
				}
				if streamErr := h.resolver.StreamChatWS(streamCtx, req, eventCh, abortCh); streamErr != nil {
					if ctx.Err() == nil {
						h.logger.Error("ws stream error", slog.Any("error", streamErr))
						writer.SendJSON(map[string]string{"type": "error", "message": streamErr.Error()})
					}
				}
			}()

			go func() {
				for event := range eventCh {
					writer.Send(event)
				}
			}()

		default:
			writer.SendJSON(map[string]string{"type": "error", "message": "unknown message type: " + msg.Type})
		}
	}
	_ = activeCancel
	return nil
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

func (*LocalChannelHandler) requireChannelIdentityID(c echo.Context) (string, error) {
	return RequireChannelIdentityID(c)
}

func (h *LocalChannelHandler) authorizeBotAccess(ctx context.Context, channelIdentityID, botID string) (bots.Bot, error) {
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID, bots.AccessPolicy{AllowPublicMember: true})
}
