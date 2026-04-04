package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"

	"github.com/memohai/memoh/internal/accounts"
	attachmentpkg "github.com/memohai/memoh/internal/attachment"
	"github.com/memohai/memoh/internal/bots"
	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/local"
	"github.com/memohai/memoh/internal/conversation"
	"github.com/memohai/memoh/internal/conversation/flow"
	"github.com/memohai/memoh/internal/media"
)

// localTtsSynthesizer synthesizes text to speech audio.
type localTtsSynthesizer interface {
	Synthesize(ctx context.Context, modelID string, text string, overrideCfg map[string]any) ([]byte, string, error)
}

// localTtsModelResolver resolves TTS model IDs for bots.
type localTtsModelResolver interface {
	ResolveTtsModelID(ctx context.Context, botID string) (string, error)
}

// LocalChannelHandler handles local channel routes (WebUI / API) backed by bot history.
type LocalChannelHandler struct {
	channelType      channel.ChannelType
	channelManager   *channel.Manager
	channelStore     *channel.Store
	chatService      *conversation.Service
	routeHub         *local.RouteHub
	botService       *bots.Service
	accountService   *accounts.Service
	resolver         *flow.Resolver
	mediaService     *media.Service
	ttsService       localTtsSynthesizer
	ttsModelResolver localTtsModelResolver
	logger           *slog.Logger
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

// SetMediaService sets the media service for WebSocket attachment ingestion.
func (h *LocalChannelHandler) SetMediaService(svc *media.Service) {
	h.mediaService = svc
}

// SetTtsService configures TTS synthesis for handling speech_delta events.
func (h *LocalChannelHandler) SetTtsService(synth localTtsSynthesizer, resolver localTtsModelResolver) {
	h.ttsService = synth
	h.ttsModelResolver = resolver
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
// @Router /bots/{bot_id}/local/stream [get].
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
	Message         channel.Message `json:"message"`
	ModelID         string          `json:"model_id,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
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
// @Router /bots/{bot_id}/local/messages [post].
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
			Type: channel.ConversationTypePrivate,
		},
		ReceivedAt: time.Now().UTC(),
		Source:     "local",
	}
	if mid := strings.TrimSpace(req.ModelID); mid != "" {
		if msg.Metadata == nil {
			msg.Metadata = make(map[string]any)
		}
		msg.Metadata["model_id"] = mid
	}
	if re := strings.TrimSpace(req.ReasoningEffort); re != "" {
		if msg.Metadata == nil {
			msg.Metadata = make(map[string]any)
		}
		msg.Metadata["reasoning_effort"] = re
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
	Type            string            `json:"type"`
	Text            string            `json:"text,omitempty"`
	SessionID       string            `json:"session_id,omitempty"`
	Attachments     []json.RawMessage `json:"attachments,omitempty"`
	ModelID         string            `json:"model_id,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
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

// HandleWebSocket godoc
// @Summary WebSocket chat endpoint
// @Description Upgrade to WebSocket for bidirectional chat streaming with abort support.
// @Tags local-channel
// @Param bot_id path string true "Bot ID"
// @Success 101 {string} string "Switching Protocols"
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bots/{bot_id}/local/ws [get].
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
	if h.channelManager == nil || h.channelStore == nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "channel manager not configured")
	}

	conn, wsErr := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
	if wsErr != nil {
		return wsErr
	}
	defer func() { _ = conn.Close() }()

	writer := newWSWriter(conn)
	defer writer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to RouteHub so that events produced by HandleInbound
	// (streaming deltas, tool calls, etc.) are forwarded over the WebSocket.
	if h.routeHub != nil {
		_, stream, unsubscribe := h.routeHub.Subscribe(botID)
		defer unsubscribe()

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case hubEvent, ok := <-stream:
					if !ok {
						return
					}
					data, marshalErr := json.Marshal(hubEvent.Event)
					if marshalErr != nil {
						continue
					}
					processed := h.processWSEvent(ctx, botID, data)
					for _, p := range processed {
						writer.Send(p)
					}
				}
			}
		}()
	}

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
			// Abort is no longer wired to a direct resolver call. The
			// HandleInbound path manages its own lifecycle.

		case "message":
			text := strings.TrimSpace(msg.Text)
			if text == "" && len(msg.Attachments) == 0 {
				writer.SendJSON(map[string]string{"type": "error", "message": "message text or attachments required"})
				continue
			}

			cfg, cfgErr := h.channelStore.ResolveEffectiveConfig(ctx, botID, h.channelType)
			if cfgErr != nil {
				writer.SendJSON(map[string]string{"type": "error", "message": cfgErr.Error()})
				continue
			}

			channelMsg := channel.Message{Text: text}
			for _, rawAtt := range msg.Attachments {
				var att channel.Attachment
				if json.Unmarshal(rawAtt, &att) == nil {
					channelMsg.Attachments = append(channelMsg.Attachments, att)
				}
			}

			routeKey := botID
			inbound := channel.InboundMessage{
				Channel:     h.channelType,
				Message:     channelMsg,
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
					Type: channel.ConversationTypePrivate,
				},
				ReceivedAt: time.Now().UTC(),
				Source:     "local",
			}
			if mid := strings.TrimSpace(msg.ModelID); mid != "" {
				if inbound.Metadata == nil {
					inbound.Metadata = make(map[string]any)
				}
				inbound.Metadata["model_id"] = mid
			}
			if re := strings.TrimSpace(msg.ReasoningEffort); re != "" {
				if inbound.Metadata == nil {
					inbound.Metadata = make(map[string]any)
				}
				inbound.Metadata["reasoning_effort"] = re
			}

			if handleErr := h.channelManager.HandleInbound(ctx, cfg, inbound); handleErr != nil {
				h.logger.Error("ws HandleInbound error", slog.Any("error", handleErr))
				writer.SendJSON(map[string]string{"type": "error", "message": handleErr.Error()})
			}

		default:
			writer.SendJSON(map[string]string{"type": "error", "message": "unknown message type: " + msg.Type})
		}
	}
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
	return AuthorizeBotAccess(ctx, h.botService, h.accountService, channelIdentityID, botID)
}

// ---------------------------------------------------------------------------
// WebSocket event processing — attachment ingestion + TTS extraction
// ---------------------------------------------------------------------------

type wsEventEnvelope struct {
	Type     string          `json:"type"`
	ToolName string          `json:"toolName,omitempty"`
	Result   json.RawMessage `json:"result,omitempty"`
}

// processWSEvent transforms a raw WS event, ingesting attachments and
// extracting TTS audio so the web frontend receives content_hash references.
func (h *LocalChannelHandler) processWSEvent(ctx context.Context, botID string, event json.RawMessage) []json.RawMessage {
	var envelope wsEventEnvelope
	if err := json.Unmarshal(event, &envelope); err != nil {
		return []json.RawMessage{event}
	}

	h.logger.Debug("ws event", slog.String("type", envelope.Type), slog.String("bot_id", botID))

	switch envelope.Type {
	case "attachment_delta":
		h.logger.Info("ws processing attachment_delta", slog.String("bot_id", botID))
		return h.wsIngestAttachments(ctx, botID, event)
	case "speech_delta":
		h.logger.Info("ws processing speech_delta", slog.String("bot_id", botID))
		return h.wsSynthesizeSpeech(ctx, botID, event)
	default:
		return []json.RawMessage{event}
	}
}

// wsIngestAttachments persists attachment data (container paths / data URLs)
// and rewrites them with content_hash so the web frontend can resolve them.
func (h *LocalChannelHandler) wsIngestAttachments(ctx context.Context, botID string, original json.RawMessage) []json.RawMessage {
	if h.mediaService == nil {
		return []json.RawMessage{original}
	}

	var event map[string]any
	if err := json.Unmarshal(original, &event); err != nil {
		return []json.RawMessage{original}
	}

	rawItems, _ := event["attachments"].([]any)
	if len(rawItems) == 0 {
		return []json.RawMessage{original}
	}

	changed := false
	for i, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if ch, _ := item["content_hash"].(string); strings.TrimSpace(ch) != "" {
			continue
		}
		rawURL, _ := item["url"].(string)
		if rawURL == "" {
			rawURL, _ = item["path"].(string)
		}
		if rawURL = strings.TrimSpace(rawURL); rawURL == "" {
			continue
		}
		if ingested := h.ingestSingleAttachment(ctx, botID, rawURL, item); ingested != nil {
			rawItems[i] = ingested
			changed = true
		}
	}

	if !changed {
		h.logger.Debug("ws attachment_delta: no items needed ingestion", slog.String("bot_id", botID))
		return []json.RawMessage{original}
	}

	h.logger.Info("ws attachment_delta: ingested attachments", slog.String("bot_id", botID), slog.Int("count", len(rawItems)))

	out, err := json.Marshal(event)
	if err != nil {
		return []json.RawMessage{original}
	}
	return []json.RawMessage{out}
}

func (h *LocalChannelHandler) ingestSingleAttachment(ctx context.Context, botID, rawURL string, item map[string]any) map[string]any {
	lower := strings.ToLower(rawURL)

	if !strings.HasPrefix(lower, "data:") && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		asset, err := h.mediaService.IngestContainerFile(ctx, botID, rawURL)
		if err != nil {
			h.logger.Warn("ws ingest container file failed", slog.String("path", rawURL), slog.Any("error", err))
			return nil
		}
		return applyAssetToItem(item, botID, asset)
	}

	if strings.HasPrefix(lower, "data:") {
		mimeType := attachmentpkg.MimeFromDataURL(rawURL)
		decoded, err := attachmentpkg.DecodeBase64(rawURL, media.MaxAssetBytes)
		if err != nil {
			h.logger.Warn("ws decode data url failed", slog.Any("error", err))
			return nil
		}
		asset, err := h.mediaService.Ingest(ctx, media.IngestInput{
			BotID:    botID,
			Mime:     mimeType,
			Reader:   decoded,
			MaxBytes: media.MaxAssetBytes,
		})
		if err != nil {
			h.logger.Warn("ws ingest data url failed", slog.Any("error", err))
			return nil
		}
		return applyAssetToItem(item, botID, asset)
	}

	return nil
}

// wsSynthesizeSpeech handles speech_delta events by synthesizing audio and
// injecting attachment_delta events with the resulting voice attachments.
func (h *LocalChannelHandler) wsSynthesizeSpeech(ctx context.Context, botID string, original json.RawMessage) []json.RawMessage {
	if h.ttsService == nil || h.ttsModelResolver == nil {
		h.logger.Warn("speech_delta received but TTS service not configured")
		return nil
	}

	modelID, err := h.ttsModelResolver.ResolveTtsModelID(ctx, botID)
	if err != nil || strings.TrimSpace(modelID) == "" {
		h.logger.Warn("speech_delta: bot has no TTS model configured", slog.String("bot_id", botID))
		return nil
	}

	var event struct {
		Speeches []struct {
			Text string `json:"text"`
		} `json:"speeches"`
	}
	if err := json.Unmarshal(original, &event); err != nil || len(event.Speeches) == 0 {
		return nil
	}

	var results []json.RawMessage
	for _, speech := range event.Speeches {
		text := strings.TrimSpace(speech.Text)
		if text == "" {
			continue
		}

		audioData, contentType, synthErr := h.ttsService.Synthesize(ctx, modelID, text, nil)
		if synthErr != nil {
			h.logger.Warn("speech synthesis failed", slog.String("bot_id", botID), slog.Any("error", synthErr))
			continue
		}

		att := h.buildTtsAttachment(ctx, botID, contentType, audioData)
		attachmentEvent, _ := json.Marshal(map[string]any{
			"type":        "attachment_delta",
			"attachments": []any{att},
		})
		results = append(results, attachmentEvent)
	}
	return results
}

func (h *LocalChannelHandler) buildTtsAttachment(ctx context.Context, botID, contentType string, audioData []byte) map[string]any {
	att := map[string]any{
		"type": "voice",
		"mime": contentType,
		"size": len(audioData),
	}

	mimeType := attachmentpkg.NormalizeMime(contentType)
	if h.mediaService != nil {
		asset, err := h.mediaService.Ingest(ctx, media.IngestInput{
			BotID:    botID,
			Mime:     mimeType,
			Reader:   bytes.NewReader(audioData),
			MaxBytes: media.MaxAssetBytes,
		})
		if err == nil {
			applyAssetToMap(att, botID, asset)
			return att
		}
		h.logger.Warn("ws tts ingest failed", slog.Any("error", err))
	}

	att["url"] = "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(audioData)
	return att
}

func applyAssetToItem(item map[string]any, botID string, asset media.Asset) map[string]any {
	result := maps.Clone(item)

	sourcePath := strings.TrimSpace(itemStr(item, "path"))
	sourceURL := strings.TrimSpace(itemStr(item, "url"))

	existingName, _ := result["name"].(string)
	if strings.TrimSpace(existingName) == "" {
		if sourcePath != "" {
			result["name"] = filepath.Base(sourcePath)
		} else if sourceURL != "" {
			result["name"] = filepath.Base(sourceURL)
		}
	}

	delete(result, "path")
	result["url"] = ""
	applyAssetToMap(result, botID, asset)

	if meta, ok := result["metadata"].(map[string]any); ok {
		if n, _ := result["name"].(string); n != "" {
			meta["name"] = n
		}
		if sourcePath != "" {
			meta["source_path"] = sourcePath
		}
		if sourceURL != "" {
			meta["source_url"] = sourceURL
		}
	}
	return result
}

func itemStr(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func applyAssetToMap(m map[string]any, botID string, asset media.Asset) {
	m["content_hash"] = asset.ContentHash
	m["metadata"] = map[string]any{
		"bot_id":      botID,
		"storage_key": asset.StorageKey,
	}
	if mime, _ := m["mime"].(string); strings.TrimSpace(mime) == "" && asset.Mime != "" {
		m["mime"] = asset.Mime
	}
	if size, _ := m["size"].(float64); size == 0 && asset.SizeBytes > 0 {
		m["size"] = asset.SizeBytes
	}
}
