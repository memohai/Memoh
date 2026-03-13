package matrix

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/common"
	"github.com/memohai/memoh/internal/textutil"
)

const Type channel.ChannelType = "matrix"

const (
	matrixDefaultTimeout = 30 * time.Second
	matrixEditThrottle   = 1200 * time.Millisecond
)

type MatrixAdapter struct {
	logger     *slog.Logger
	httpClient *http.Client

	txnMu sync.Mutex
	txnID uint64

	seenMu sync.Mutex
	seen   map[string]map[string]time.Time
}

type matrixSyncResponse struct {
	NextBatch string `json:"next_batch"`
	Rooms     struct {
		Join map[string]struct {
			Timeline struct {
				Events []matrixEvent `json:"events"`
			} `json:"timeline"`
		} `json:"join"`
	} `json:"rooms"`
}

type matrixEvent struct {
	EventID        string                 `json:"event_id"`
	Sender         string                 `json:"sender"`
	Type           string                 `json:"type"`
	OriginServerTS int64                  `json:"origin_server_ts"`
	Content        map[string]any         `json:"content"`
	Unsigned       map[string]any         `json:"unsigned"`
	RoomID         string                 `json:"room_id"`
	StateKey       *string                `json:"state_key,omitempty"`
	Metadata       map[string]interface{} `json:"-"`
}

type matrixSendResponse struct {
	EventID string `json:"event_id"`
}

type matrixCreateRoomRequest struct {
	Invite   []string `json:"invite,omitempty"`
	IsDirect bool     `json:"is_direct,omitempty"`
	Preset   string   `json:"preset,omitempty"`
	Topic    string   `json:"topic,omitempty"`
	Name     string   `json:"name,omitempty"`
}

type matrixCreateRoomResponse struct {
	RoomID string `json:"room_id"`
}

var matrixMentionHrefPattern = regexp.MustCompile(`https://matrix\.to/#/(@[^"'<\s]+)`)

func NewMatrixAdapter(log *slog.Logger) *MatrixAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &MatrixAdapter{
		logger: log.With(slog.String("adapter", "matrix")),
		httpClient: &http.Client{
			Timeout: matrixDefaultTimeout,
		},
		seen: make(map[string]map[string]time.Time),
	}
}

func (*MatrixAdapter) Type() channel.ChannelType {
	return Type
}

func (*MatrixAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "Matrix",
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Markdown:       true,
			Reply:          true,
			Streaming:      true,
			BlockStreaming: true,
			Edit:           true,
			ChatTypes:      []string{"direct", "group"},
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"homeserverUrl": {
					Type:        channel.FieldString,
					Required:    true,
					Title:       "Homeserver URL",
					Description: "Matrix homeserver base URL, e.g. https://matrix.example.com",
				},
				"userId": {
					Type:        channel.FieldString,
					Required:    true,
					Title:       "User ID",
					Description: "Matrix bot/user ID, e.g. @memoh:example.com",
				},
				"accessToken": {
					Type:     channel.FieldSecret,
					Required: true,
					Title:    "Access Token",
				},
				"syncTimeoutSeconds": {
					Type:        channel.FieldNumber,
					Title:       "Sync Timeout Seconds",
					Description: "Long-poll timeout for /sync requests",
					Example:     30,
				},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"room_id": {
					Type:        channel.FieldString,
					Title:       "Room ID or Alias",
					Description: "Preferred outbound target, e.g. !roomid:example.com or #alias:example.com",
				},
				"user_id": {
					Type:        channel.FieldString,
					Title:       "User ID",
					Description: "Optional direct-message target, e.g. @alice:example.com",
				},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "!room:server | #alias:server | @user:server",
			Hints: []channel.TargetHint{
				{Label: "Room ID", Example: "!abcdef:matrix.org"},
				{Label: "Room Alias", Example: "#ops:example.com"},
				{Label: "User ID", Example: "@alice:example.com"},
			},
		},
	}
}

func (*MatrixAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

func (*MatrixAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

func (*MatrixAdapter) NormalizeTarget(raw string) string {
	return normalizeTarget(raw)
}

func (*MatrixAdapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

func (*MatrixAdapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

func (*MatrixAdapter) BuildUserConfig(identity channel.Identity) map[string]any {
	return buildUserConfig(identity)
}

func (a *MatrixAdapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, err
	}
	connCtx, cancel := context.WithCancel(ctx)
	go a.runSyncLoop(connCtx, cfg, parsed, handler)
	return channel.NewConnection(cfg, func(context.Context) error {
		cancel()
		return nil
	}), nil
}

func (a *MatrixAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	if msg.Message.IsEmpty() {
		return errors.New("message is required")
	}
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	roomID, err := a.resolveRoomTarget(ctx, parsed, msg.Target)
	if err != nil {
		return err
	}
	_, err = a.sendTextEvent(ctx, parsed, roomID, buildMatrixMessageContent(msg.Message, false, ""))
	return err
}

func (a *MatrixAdapter) OpenStream(_ context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
	if err := validateTarget(target); err != nil {
		return nil, err
	}
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, err
	}
	reply := opts.Reply
	if reply == nil && strings.TrimSpace(opts.SourceMessageID) != "" {
		reply = &channel.ReplyRef{Target: normalizeTarget(target), MessageID: strings.TrimSpace(opts.SourceMessageID)}
	}
	return &matrixOutboundStream{
		adapter: a,
		cfg:     parsed,
		target:  normalizeTarget(target),
		reply:   reply,
	}, nil
}

func (a *MatrixAdapter) Update(ctx context.Context, cfg channel.ChannelConfig, target string, messageID string, msg channel.Message) error {
	parsed, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	roomID, err := a.resolveRoomTarget(ctx, parsed, target)
	if err != nil {
		return err
	}
	_, err = a.sendTextEvent(ctx, parsed, roomID, buildMatrixMessageContent(msg, true, strings.TrimSpace(messageID)))
	return err
}

func (*MatrixAdapter) Unsend(context.Context, channel.ChannelConfig, string, string) error {
	return errors.New("matrix unsend not supported")
}

func (a *MatrixAdapter) runSyncLoop(ctx context.Context, cfg channel.ChannelConfig, parsed Config, handler channel.InboundHandler) {
	backoffs := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 10 * time.Second, 20 * time.Second}
	attempt := 0
	var since string
	for ctx.Err() == nil {
		nextSince, healthy, err := a.syncOnce(ctx, cfg, parsed, since, handler)
		if strings.TrimSpace(nextSince) != "" {
			since = nextSince
		}
		if err == nil || ctx.Err() != nil {
			attempt = 0
			continue
		}
		if a.logger != nil {
			a.logger.Warn("matrix sync reconnect", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		delay, nextAttempt := nextReconnectDelay(backoffs, attempt, healthy)
		attempt = nextAttempt
		if !sleepContext(ctx, delay) {
			return
		}
	}
}

func (a *MatrixAdapter) syncOnce(ctx context.Context, cfg channel.ChannelConfig, parsed Config, since string, handler channel.InboundHandler) (string, bool, error) {
	query := url.Values{}
	query.Set("timeout", strconv.Itoa(parsed.SyncTimeoutSeconds*1000))
	if strings.TrimSpace(since) != "" {
		query.Set("since", since)
	}
	var resp matrixSyncResponse
	if err := a.doJSON(ctx, parsed, http.MethodGet, "/_matrix/client/v3/sync?"+query.Encode(), nil, &resp); err != nil {
		return since, false, err
	}
	healthy := false
	for roomID, joined := range resp.Rooms.Join {
		for _, evt := range joined.Timeline.Events {
			evt.RoomID = roomID
			delivered, err := a.handleEvent(ctx, cfg, parsed, evt, handler)
			if err != nil {
				return resp.NextBatch, healthy, err
			}
			healthy = healthy || delivered
		}
	}
	return resp.NextBatch, healthy, nil
}

func (a *MatrixAdapter) handleEvent(ctx context.Context, cfg channel.ChannelConfig, parsed Config, evt matrixEvent, handler channel.InboundHandler) (bool, error) {
	if evt.Type != "m.room.message" {
		return false, nil
	}
	if strings.TrimSpace(evt.Sender) == "" || strings.EqualFold(strings.TrimSpace(evt.Sender), parsed.UserID) {
		return false, nil
	}
	if a.seenEvent(cfg.ID, evt.EventID) {
		return false, nil
	}
	if isMatrixEditEvent(evt.Content) {
		return false, nil
	}
	body := strings.TrimSpace(channel.ReadString(evt.Content, "body"))
	if body == "" {
		return false, nil
	}
	isMentioned := isMatrixBotMentioned(parsed.UserID, evt.Content)
	replyTo := readReplyToEventID(evt.Content)
	msg := channel.InboundMessage{
		Channel:     Type,
		BotID:       cfg.BotID,
		ReplyTarget: evt.RoomID,
		Message: channel.Message{
			ID:     strings.TrimSpace(evt.EventID),
			Format: channel.MessageFormatPlain,
			Text:   body,
		},
		Sender: channel.Identity{
			SubjectID:   strings.TrimSpace(evt.Sender),
			DisplayName: matrixDisplayName(evt),
			Attributes: map[string]string{
				"user_id": strings.TrimSpace(evt.Sender),
				"room_id": strings.TrimSpace(evt.RoomID),
			},
		},
		Conversation: channel.Conversation{
			ID:   strings.TrimSpace(evt.RoomID),
			Type: "group",
			Metadata: map[string]any{
				"room_id": strings.TrimSpace(evt.RoomID),
			},
		},
		ReceivedAt: matrixEventTime(evt.OriginServerTS),
		Source:     "matrix",
		Metadata: map[string]any{
			"room_id":         strings.TrimSpace(evt.RoomID),
			"event_id":        strings.TrimSpace(evt.EventID),
			"sender":          strings.TrimSpace(evt.Sender),
			"msgtype":         channel.ReadString(evt.Content, "msgtype"),
			"raw_text":        body,
			"is_mentioned":    isMentioned,
			"is_reply_to_bot": false,
		},
	}
	if replyTo != "" {
		msg.Message.Reply = &channel.ReplyRef{Target: evt.RoomID, MessageID: replyTo}
	}
	if a.logger != nil {
		a.logger.Info("inbound received",
			slog.String("config_id", cfg.ID),
			slog.String("room_id", evt.RoomID),
			slog.String("sender", evt.Sender),
			slog.Bool("is_mentioned", isMentioned),
			slog.String("text", common.SummarizeText(body)),
		)
	}
	return true, handler(ctx, cfg, msg)
}

func buildMatrixMessageContent(msg channel.Message, edit bool, originalEventID string) map[string]any {
	body := strings.TrimSpace(msg.PlainText())
	content := map[string]any{
		"msgtype": "m.notice",
		"body":    body,
	}
	if msg.Reply != nil && strings.TrimSpace(msg.Reply.MessageID) != "" && !edit {
		content["m.relates_to"] = map[string]any{
			"m.in_reply_to": map[string]any{
				"event_id": strings.TrimSpace(msg.Reply.MessageID),
			},
		}
	}
	if edit && strings.TrimSpace(originalEventID) != "" {
		newContent := map[string]any{
			"msgtype": "m.notice",
			"body":    body,
		}
		content["m.new_content"] = newContent
		content["m.relates_to"] = map[string]any{
			"rel_type": "m.replace",
			"event_id": strings.TrimSpace(originalEventID),
		}
		content["body"] = "* " + body
	}
	return content
}

func isMatrixEditEvent(content map[string]any) bool {
	if _, ok := content["m.new_content"]; ok {
		return true
	}
	relatesTo, ok := content["m.relates_to"].(map[string]any)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(channel.ReadString(relatesTo, "rel_type")), "m.replace")
}

func readReplyToEventID(content map[string]any) string {
	relatesTo, ok := content["m.relates_to"].(map[string]any)
	if !ok {
		return ""
	}
	inReplyTo, ok := relatesTo["m.in_reply_to"].(map[string]any)
	if !ok {
		return ""
	}
	return strings.TrimSpace(channel.ReadString(inReplyTo, "event_id"))
}

func isMatrixBotMentioned(botUserID string, content map[string]any) bool {
	botUserID = strings.TrimSpace(botUserID)
	if botUserID == "" {
		return false
	}
	if mentions, ok := content["m.mentions"].(map[string]any); ok {
		if userIDs, ok := mentions["user_ids"].([]any); ok {
			for _, item := range userIDs {
				if strings.EqualFold(strings.TrimSpace(fmt.Sprint(item)), botUserID) {
					return true
				}
			}
		}
	}
	formatted := strings.TrimSpace(channel.ReadString(content, "formatted_body", "formattedBody"))
	if formatted != "" {
		matches := matrixMentionHrefPattern.FindAllStringSubmatch(formatted, -1)
		for _, match := range matches {
			if len(match) > 1 && strings.EqualFold(strings.TrimSpace(match[1]), botUserID) {
				return true
			}
		}
	}
	body := strings.TrimSpace(channel.ReadString(content, "body"))
	if body == "" {
		return false
	}
	localpart := botUserID
	if idx := strings.Index(localpart, ":"); idx > 0 {
		localpart = localpart[:idx]
	}
	for _, candidate := range []string{botUserID, localpart} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if strings.Contains(strings.ToLower(body), strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func (a *MatrixAdapter) sendTextEvent(ctx context.Context, cfg Config, roomID string, content map[string]any) (string, error) {
	txnID := a.nextTxnID()
	path := fmt.Sprintf("/_matrix/client/v3/rooms/%s/send/m.room.message/%s", url.PathEscape(roomID), url.PathEscape(txnID))
	var resp matrixSendResponse
	if err := a.doJSON(ctx, cfg, http.MethodPut, path, content, &resp); err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.EventID), nil
}

func (a *MatrixAdapter) resolveRoomTarget(ctx context.Context, cfg Config, target string) (string, error) {
	target = normalizeTarget(target)
	if err := validateTarget(target); err != nil {
		return "", err
	}
	if strings.HasPrefix(target, "@") {
		return a.ensureDirectRoom(ctx, cfg, target)
	}
	if strings.HasPrefix(target, "#") {
		return "", fmt.Errorf("matrix room aliases are not supported for outbound send yet: %s", target)
	}
	return target, nil
}

func (a *MatrixAdapter) ensureDirectRoom(ctx context.Context, cfg Config, userID string) (string, error) {
	req := matrixCreateRoomRequest{
		Invite:   []string{strings.TrimSpace(userID)},
		IsDirect: true,
		Preset:   "trusted_private_chat",
	}
	var resp matrixCreateRoomResponse
	if err := a.doJSON(ctx, cfg, http.MethodPost, "/_matrix/client/v3/createRoom", req, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.RoomID) == "" {
		return "", errors.New("matrix createRoom returned empty room_id")
	}
	return strings.TrimSpace(resp.RoomID), nil
}

func (a *MatrixAdapter) doJSON(ctx context.Context, cfg Config, method, path string, reqBody any, respBody any) error {
	var body io.Reader
	if reqBody != nil {
		payload, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(ctx, method, cfg.HomeserverURL+path, body)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+cfg.AccessToken)
	if reqBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(data))
		if message == "" {
			message = resp.Status
		}
		return fmt.Errorf("matrix %s %s failed: %s", method, path, textutil.TruncateRunes(message, 300))
	}
	if respBody == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, respBody)
}

func (a *MatrixAdapter) nextTxnID() string {
	a.txnMu.Lock()
	defer a.txnMu.Unlock()
	a.txnID++
	return fmt.Sprintf("memoh-%d-%d-%04d", time.Now().UnixMilli(), a.txnID, rand.IntN(10000))
}

func (a *MatrixAdapter) seenEvent(configID, eventID string) bool {
	configID = strings.TrimSpace(configID)
	eventID = strings.TrimSpace(eventID)
	if configID == "" || eventID == "" {
		return false
	}
	now := time.Now()
	a.seenMu.Lock()
	defer a.seenMu.Unlock()
	byConfig := a.seen[configID]
	if byConfig == nil {
		byConfig = make(map[string]time.Time)
		a.seen[configID] = byConfig
	}
	for id, seenAt := range byConfig {
		if now.Sub(seenAt) > 10*time.Minute {
			delete(byConfig, id)
		}
	}
	if _, ok := byConfig[eventID]; ok {
		return true
	}
	byConfig[eventID] = now
	return false
}

func matrixDisplayName(evt matrixEvent) string {
	unsignedSender, ok := evt.Unsigned["m.relations"].(map[string]any)
	if ok {
		_ = unsignedSender
	}
	if displayName := strings.TrimSpace(channel.ReadString(evt.Unsigned, "displayname", "sender_display_name")); displayName != "" {
		return displayName
	}
	return strings.TrimSpace(evt.Sender)
}

func matrixEventTime(ts int64) time.Time {
	if ts <= 0 {
		return time.Now().UTC()
	}
	return time.UnixMilli(ts).UTC()
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func nextReconnectDelay(backoffs []time.Duration, attempt int, healthySession bool) (time.Duration, int) {
	if healthySession {
		attempt = 0
	}
	if len(backoffs) == 0 {
		return time.Second, attempt + 1
	}
	if attempt < 0 {
		attempt = 0
	}
	if attempt >= len(backoffs) {
		attempt = len(backoffs) - 1
	}
	delay := backoffs[attempt]
	if attempt < len(backoffs)-1 {
		attempt++
	}
	return delay, attempt
}
