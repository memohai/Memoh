package telegram

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"
	tele "gopkg.in/telebot.v4"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/common"
	"github.com/memohai/memoh/internal/command"
	"github.com/memohai/memoh/internal/i18n"
	"github.com/memohai/memoh/internal/media"
	"github.com/memohai/memoh/internal/textutil"
)

const (
	telegramMaxMessageLength        = 4096
	telegramMediaGroupCollectWindow = 700 * time.Millisecond
	telegramUpdateDedupeTTL         = 10 * time.Minute
	defaultTelegramFileEndpoint     = "https://api.telegram.org/file/bot%s/%s"
	telegramLongPollTimeout         = 30 * time.Second
)

type telegramMediaGroupBuffer struct {
	messages []*tele.Message
	timer    *time.Timer
}

// assetOpener reads stored asset bytes by content hash.
type assetOpener interface {
	Open(ctx context.Context, botID, contentHash string) (io.ReadCloser, media.Asset, error)
}

// TelegramAdapter implements the channel.Adapter, channel.Sender, and channel.Receiver interfaces for Telegram.
type TelegramAdapter struct {
	logger        *slog.Logger
	mu            sync.RWMutex
	bots          map[string]*tele.Bot // keyed by effective bot config
	fileEndpoints map[*tele.Bot]string // bot instance → file endpoint format string
	assets        assetOpener
	streamLimiter *rate.Limiter // global rate limiter for all streaming API calls
	seenUpdatesMu sync.Mutex
	seenUpdates   map[string]time.Time
}

// telegramChannelRecipient implements tele.Recipient for @channel_username
// targets; tele.Chat.Recipient() returns the numeric ID which is unset here.
type telegramChannelRecipient string

func (r telegramChannelRecipient) Recipient() string { return string(r) }

// TelegramAdapter edits and deletes messages in place for interactive
// pagination/selection (channel.MessageEditor).
var _ channel.MessageEditor = (*TelegramAdapter)(nil)

// NewTelegramAdapter creates a TelegramAdapter with the given logger.
func NewTelegramAdapter(log *slog.Logger) *TelegramAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &TelegramAdapter{
		logger:        log.With(slog.String("adapter", "telegram")),
		bots:          make(map[string]*tele.Bot),
		fileEndpoints: make(map[*tele.Bot]string),
		streamLimiter: rate.NewLimiter(rate.Every(time.Second), 3), // 1 req/s sustained, burst of 3
		seenUpdates:   make(map[string]time.Time),
	}
}

// waitStreamLimit waits for the global stream rate limiter to allow one request.
// All streams from the same adapter share this limiter to coordinate and avoid
// aggregate Telegram API rate limits across concurrent conversations.
func (a *TelegramAdapter) waitStreamLimit(ctx context.Context) error {
	return a.streamLimiter.Wait(ctx)
}

// SetAssetOpener injects the media asset reader for storage-first file delivery.
func (a *TelegramAdapter) SetAssetOpener(opener assetOpener) {
	a.assets = opener
}

var getOrCreateBotForTest func(a *TelegramAdapter, token, configID string) (*tele.Bot, error)

func (a *TelegramAdapter) getOrCreateBot(cfg Config, configID string) (*tele.Bot, error) {
	channel.SetIMErrorSecrets("telegram:"+configID, cfg.BotToken)
	if getOrCreateBotForTest != nil {
		return getOrCreateBotForTest(a, cfg.BotToken, configID)
	}
	cacheKey := strings.Join([]string{
		cfg.BotToken,
		cfg.baseURL(),
		cfg.HTTPProxy.CacheKey(),
	}, "\x00")
	a.mu.RLock()
	bot, ok := a.bots[cacheKey]
	a.mu.RUnlock()
	if ok {
		return bot, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if bot, ok := a.bots[cacheKey]; ok {
		return bot, nil
	}
	httpClient, err := common.NewHTTPClient(30*time.Second, cfg.HTTPProxy)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("create bot http client failed", slog.String("config_id", configID), slog.Any("error", err))
		}
		return nil, err
	}
	bot, err = tele.NewBot(tele.Settings{
		Token:  cfg.BotToken,
		URL:    cfg.baseURL(),
		Client: httpClient,
		OnError: func(handlerErr error, _ tele.Context) {
			if a.logger != nil {
				a.logger.Warn("telegram bot sdk error", slog.String("config_id", configID), slog.Any("error", handlerErr))
			}
		},
	})
	if err != nil {
		if a.logger != nil {
			a.logger.Error("create bot failed", slog.String("config_id", configID), slog.Any("error", err))
		}
		return nil, err
	}
	a.bots[cacheKey] = bot
	a.fileEndpoints[bot] = cfg.fileEndpoint()
	return bot, nil
}

// getFileDirectURL resolves a file ID to a direct download URL,
// respecting the custom file endpoint for reverse proxy setups.
func (a *TelegramAdapter) getFileDirectURL(bot *tele.Bot, fileID string) (string, error) {
	file, err := bot.FileByID(fileID)
	if err != nil {
		return "", err
	}
	a.mu.RLock()
	endpoint := a.fileEndpoints[bot]
	a.mu.RUnlock()
	if endpoint == "" {
		endpoint = defaultTelegramFileEndpoint
	}
	return formatTelegramFileURL(endpoint, bot.Token, file.FilePath), nil
}

func formatTelegramFileURL(endpoint, token, filePath string) string {
	placeholderCount := strings.Count(endpoint, "%s")
	switch {
	case placeholderCount >= 2:
		return fmt.Sprintf(endpoint, token, filePath)
	case placeholderCount == 1:
		return fmt.Sprintf(endpoint, filePath)
	default:
		base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
		if base == "" {
			return filePath
		}
		return base + "/" + strings.TrimLeft(filePath, "/")
	}
}

// Type returns the Telegram channel type.
func (*TelegramAdapter) Type() channel.ChannelType {
	return Type
}

// Descriptor returns the Telegram channel metadata.
func (*TelegramAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "Telegram",
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Markdown:       true,
			Reply:          true,
			Buttons:        true,
			Attachments:    true,
			Media:          true,
			Streaming:      true,
			BlockStreaming: true,
			Edit:           true,
			Unsend:         true,
		},
		ConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"botToken": {
					Type:     channel.FieldSecret,
					Required: true,
					Order:    0,
					Title:    "Bot Token",
				},
				"apiBaseURL": {
					Type:        channel.FieldString,
					Required:    false,
					Order:       10,
					Title:       "API Base URL",
					Description: "Reverse proxy base URL for the Telegram Bot API. Required in regions where Telegram is blocked (e.g. China mainland). Default: https://api.telegram.org",
				},
				"httpProxyUrl": {
					Type:        channel.FieldSecret,
					Required:    false,
					Order:       20,
					Title:       "HTTP Proxy URL",
					Description: "Optional outbound HTTP proxy URL for Telegram API requests, e.g. http://user:pass@host:port. Explicit adapter proxy overrides HTTP_PROXY/HTTPS_PROXY.",
				},
			},
		},
		UserConfigSchema: channel.ConfigSchema{
			Version: 1,
			Fields: map[string]channel.FieldSchema{
				"username": {Type: channel.FieldString},
				"user_id":  {Type: channel.FieldString},
				"chat_id":  {Type: channel.FieldString},
			},
		},
		TargetSpec: channel.TargetSpec{
			Format: "chat_id | @username",
			Hints: []channel.TargetHint{
				{Label: "Chat ID", Example: "123456789"},
				{Label: "Username", Example: "@alice"},
			},
		},
	}
}

// NormalizeConfig validates and normalizes a Telegram channel configuration map.
func (*TelegramAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

// NormalizeUserConfig validates and normalizes a Telegram user-binding configuration map.
func (*TelegramAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

// NormalizeTarget normalizes a Telegram delivery target string.
func (*TelegramAdapter) NormalizeTarget(raw string) string {
	return normalizeTarget(raw)
}

// ResolveTarget derives a delivery target from a Telegram user-binding configuration.
func (*TelegramAdapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

// MatchBinding reports whether a Telegram user binding matches the given criteria.
func (*TelegramAdapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

// BuildUserConfig constructs a Telegram user-binding config from an Identity.
func (*TelegramAdapter) BuildUserConfig(identity channel.Identity) map[string]any {
	return buildUserConfig(identity)
}

// registerCommandMenu publishes the curated slash-command list to Telegram via
// setMyCommands, so the bot's "/" menu is populated automatically (no per-bot
// setup). Best-effort: errors are logged, never fatal.
func (a *TelegramAdapter) registerCommandMenu(bot *tele.Bot, configID string) {
	// The native command menu is registered once per connection, before any
	// per-bot command-UI locale is available at this transport layer, so it is
	// rendered in the server default locale. TODO: thread the bot's
	// command_ui_language here to localize the native "/" menu per bot.
	menu := command.MenuCommands(i18n.New(""))
	cmds := make([]tele.Command, 0, len(menu))
	for _, m := range menu {
		cmds = append(cmds, tele.Command{Text: m.Command, Description: m.Description})
	}
	if err := bot.SetCommands(cmds); err != nil {
		if a.logger != nil {
			a.logger.Warn("register command menu failed", slog.String("config_id", configID), slog.Any("error", err))
		}
		return
	}
	if a.logger != nil {
		a.logger.Info("registered command menu", slog.String("config_id", configID), slog.Int("count", len(cmds)))
	}
}

// Connect starts long-polling for Telegram updates and forwards messages to the handler.
func (a *TelegramAdapter) Connect(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler) (channel.Connection, error) {
	if a.logger != nil {
		a.logger.Info("start", slog.String("config_id", cfg.ID))
	}
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("decode config failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return nil, err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("create bot failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return nil, err
	}
	// Advertise the slash-command menu so users discover and tap commands from
	// Telegram's native "/" menu without any per-bot configuration. Non-blocking
	// and best-effort — a failure here must not stop the bot from connecting.
	go a.registerCommandMenu(bot, cfg.ID)
	connCtx, cancel := context.WithCancel(ctx)
	mediaGroups := make(map[string]*telegramMediaGroupBuffer)
	var mediaGroupsMu sync.Mutex

	flushMediaGroup := func(groupKey string) {
		var batch []*tele.Message
		mediaGroupsMu.Lock()
		buffer, ok := mediaGroups[groupKey]
		if ok {
			delete(mediaGroups, groupKey)
			batch = append(batch, buffer.messages...)
		}
		mediaGroupsMu.Unlock()
		if !ok || len(batch) == 0 {
			return
		}
		msg, ok := a.buildTelegramMediaGroupInboundMessage(bot, cfg, batch)
		if !ok {
			return
		}
		a.dispatchInbound(connCtx, cfg, handler, msg)
	}
	flushAllMediaGroups := func() {
		mediaGroupsMu.Lock()
		keys := make([]string, 0, len(mediaGroups))
		for key, buffer := range mediaGroups {
			keys = append(keys, key)
			if buffer != nil && buffer.timer != nil {
				buffer.timer.Stop()
			}
		}
		mediaGroupsMu.Unlock()
		for _, key := range keys {
			flushMediaGroup(key)
		}
	}
	flushMediaGroupsByChat := func(chatID int64) {
		if chatID == 0 {
			return
		}
		mediaGroupsMu.Lock()
		keys := make([]string, 0, len(mediaGroups))
		for key, buffer := range mediaGroups {
			if !isTelegramMediaGroupForChat(key, chatID) {
				continue
			}
			keys = append(keys, key)
			if buffer != nil && buffer.timer != nil {
				buffer.timer.Stop()
			}
		}
		mediaGroupsMu.Unlock()
		for _, key := range keys {
			flushMediaGroup(key)
		}
	}
	queueMediaGroup := func(msg *tele.Message) bool {
		groupKey := telegramMediaGroupKey(msg)
		if groupKey == "" {
			return false
		}
		mediaGroupsMu.Lock()
		buffer, ok := mediaGroups[groupKey]
		if !ok {
			buffer = &telegramMediaGroupBuffer{}
			mediaGroups[groupKey] = buffer
		}
		buffer.messages = append(buffer.messages, msg)
		if buffer.timer != nil {
			buffer.timer.Stop()
		}
		buffer.timer = time.AfterFunc(telegramMediaGroupCollectWindow, func() {
			flushMediaGroup(groupKey)
		})
		mediaGroupsMu.Unlock()
		return true
	}

	processUpdate := func(upd *tele.Update) bool {
		if upd == nil {
			return false
		}
		select {
		case <-connCtx.Done():
			return false
		default:
		}
		if a.seenTelegramUpdate(cfg.ID, upd.ID, time.Now()) {
			if a.logger != nil {
				a.logger.Debug("skip duplicate telegram update",
					slog.String("config_id", cfg.ID),
					slog.Int("update_id", upd.ID),
				)
			}
			return false
		}
		if upd.Callback != nil {
			a.handleTelegramCallback(connCtx, cfg, handler, bot, upd)
			return false
		}
		if upd.Message == nil {
			return false
		}
		if queueMediaGroup(upd.Message) {
			return false
		}
		flushMediaGroupsByChat(telegramChatID(upd.Message))
		msg, ok := a.buildTelegramInboundMessage(bot, cfg, upd)
		if !ok {
			return false
		}
		a.dispatchInbound(connCtx, cfg, handler, msg)
		return false
	}

	bot.Poller = tele.NewMiddlewarePoller(
		&tele.LongPoller{Timeout: telegramLongPollTimeout},
		processUpdate,
	)

	go bot.Start()

	stop := func(_ context.Context) error {
		if a.logger != nil {
			a.logger.Info("stop", slog.String("config_id", cfg.ID))
		}
		bot.Stop()
		cancel()
		flushAllMediaGroups()
		return nil
	}
	return channel.NewConnection(cfg, stop), nil
}

func telegramMediaGroupKey(msg *tele.Message) string {
	if msg == nil {
		return ""
	}
	albumID := strings.TrimSpace(msg.AlbumID)
	if albumID == "" {
		return ""
	}
	chatID := telegramChatID(msg)
	return fmt.Sprintf("%d:%s", chatID, albumID)
}

func telegramChatID(msg *tele.Message) int64 {
	if msg == nil || msg.Chat == nil {
		return 0
	}
	return msg.Chat.ID
}

func isTelegramMediaGroupForChat(groupKey string, chatID int64) bool {
	if chatID == 0 || strings.TrimSpace(groupKey) == "" {
		return false
	}
	return strings.HasPrefix(groupKey, fmt.Sprintf("%d:", chatID))
}

func (a *TelegramAdapter) dispatchInbound(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler, msg channel.InboundMessage) {
	a.logTelegramInbound(cfg.ID, msg)
	go func() {
		if err := handler(ctx, cfg, msg); err != nil && a.logger != nil {
			a.logger.Error("handle inbound failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
	}()
}

func (a *TelegramAdapter) buildTelegramInboundMessage(bot *tele.Bot, cfg channel.ChannelConfig, update *tele.Update) (channel.InboundMessage, bool) {
	raw := update.Message
	if raw == nil {
		return channel.InboundMessage{}, false
	}
	text := strings.TrimSpace(raw.Text)
	caption := strings.TrimSpace(raw.Caption)
	if text == "" && caption != "" {
		text = caption
	}
	attachments := a.collectTelegramAttachments(bot, raw)
	return a.toInboundTelegramMessage(bot, cfg, raw, text, attachments, map[string]any{
		"update_id": update.ID,
	})
}

// handleTelegramCallback acknowledges and routes an inline-keyboard callback.
// Interactive callbacks (namespace "m~") re-render the originating message in
// place: pagination/selection re-dispatch a synthetic command, dismiss strips
// the keyboard, and the page-indicator noop is ignored. Legacy approval
// callbacks keep their prior behavior (clear buttons, then dispatch).
func (a *TelegramAdapter) handleTelegramCallback(ctx context.Context, cfg channel.ChannelConfig, handler channel.InboundHandler, bot *tele.Bot, update *tele.Update) {
	cb := update.Callback
	if cb == nil {
		return
	}
	// Acknowledge immediately so the client stops showing a spinner.
	_ = bot.Respond(cb, &tele.CallbackResponse{Text: "OK"})

	if command.IsInteractiveCallback(strings.TrimSpace(cb.Data)) {
		parsed, ok := command.DecodeCallback(strings.TrimSpace(cb.Data))
		if !ok {
			return
		}
		switch {
		case parsed.IsNoop():
			return
		case parsed.IsDismiss():
			// Close: collapse the card to its title line and drop the keyboard,
			// rather than deleting the whole message — the user keeps a short
			// breadcrumb of what was opened instead of having it vanish.
			if cb.Message != nil && cb.Message.Chat != nil {
				if title := collapseToTitle(cb.Message.Text); title != "" {
					_ = editTelegramMessageText(bot, cb.Message.Chat.ID, cb.Message.ID, title, "")
				}
			}
			return
		default:
			// Pagination/selection: re-dispatch a synthetic command that
			// re-renders the message in place. Do NOT clear the keyboard.
			if msg, ok := a.buildTelegramCallbackInboundMessage(cfg, update); ok {
				a.dispatchInbound(ctx, cfg, handler, msg)
			}
			return
		}
	}

	// Legacy tool-approval callbacks.
	if msg, ok := a.buildTelegramCallbackInboundMessage(cfg, update); ok {
		_ = clearTelegramCallbackButtons(bot, cb)
		a.dispatchInbound(ctx, cfg, handler, msg)
	}
}

func (a *TelegramAdapter) buildTelegramCallbackInboundMessage(cfg channel.ChannelConfig, update *tele.Update) (channel.InboundMessage, bool) {
	cb := update.Callback
	if cb == nil || cb.Message == nil {
		return channel.InboundMessage{}, false
	}
	extraMeta := map[string]any{
		"update_id":         update.ID,
		"callback_query_id": cb.ID,
	}
	var text string
	if action, approvalID, ok := parseTelegramApprovalCallback(cb.Data); ok {
		text = "/" + action + " " + approvalID
	} else if parsed, ok := command.DecodeCallback(strings.TrimSpace(cb.Data)); ok {
		syntheticCmd := parsed.SyntheticCommand()
		if syntheticCmd == "" {
			return channel.InboundMessage{}, false
		}
		text = syntheticCmd
		// Re-render the existing message in place rather than posting a new one.
		extraMeta["edit_message_id"] = strconv.Itoa(cb.Message.ID)
		// A tap on the bot's own keyboard is by definition directed at the bot,
		// so the command path runs even in group chats.
		extraMeta["is_mentioned"] = true
	} else {
		return channel.InboundMessage{}, false
	}
	raw := cb.Message
	raw.Text = text
	raw.Sender = cb.Sender
	replyID := strconv.Itoa(cb.Message.ID)
	msg, ok := a.toInboundTelegramMessage(nil, cfg, raw, text, nil, extraMeta)
	if !ok {
		return channel.InboundMessage{}, false
	}
	msg.Message.Reply = &channel.ReplyRef{MessageID: replyID}
	return msg, true
}

func parseTelegramApprovalCallback(data string) (action, approvalID string, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(data), ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	switch parts[0] {
	case "approve", "reject":
		return parts[0], strings.TrimSpace(parts[1]), strings.TrimSpace(parts[1]) != ""
	default:
		return "", "", false
	}
}

func clearTelegramCallbackButtons(bot *tele.Bot, cb *tele.Callback) error {
	if bot == nil || cb == nil || cb.Message == nil || cb.Message.Chat == nil {
		return nil
	}
	// EditReplyMarkup with a nil markup deletes the keyboard (telebot wraps it
	// into an empty ReplyMarkup, which Telegram treats as "remove keyboard").
	_, err := bot.EditReplyMarkup(&tele.StoredMessage{
		MessageID: strconv.Itoa(cb.Message.ID),
		ChatID:    cb.Message.Chat.ID,
	}, nil)
	return err
}

func (a *TelegramAdapter) buildTelegramMediaGroupInboundMessage(
	bot *tele.Bot,
	cfg channel.ChannelConfig,
	raw []*tele.Message,
) (channel.InboundMessage, bool) {
	if len(raw) == 0 {
		return channel.InboundMessage{}, false
	}
	items := make([]*tele.Message, 0, len(raw))
	for _, msg := range raw {
		if msg != nil {
			items = append(items, msg)
		}
	}
	if len(items) == 0 {
		return channel.InboundMessage{}, false
	}
	slices.SortStableFunc(items, func(a, b *tele.Message) int {
		return cmp.Compare(a.ID, b.ID)
	})
	anchor := items[0]
	text := ""
	attachments := make([]channel.Attachment, 0, len(items))
	isMentioned := false
	isReplyToBot := false
	botUsername := ""
	botID := int64(0)
	if bot != nil && bot.Me != nil {
		botUsername = bot.Me.Username
		botID = bot.Me.ID
	}
	for _, msg := range items {
		candidate := strings.TrimSpace(msg.Text)
		if candidate == "" {
			candidate = strings.TrimSpace(msg.Caption)
		}
		if text == "" && candidate != "" {
			text = candidate
			anchor = msg
		}
		attachments = append(attachments, a.collectTelegramAttachments(bot, msg)...)
		if !isMentioned {
			isMentioned = isTelegramBotMentioned(msg, botUsername)
		}
		if !isReplyToBot {
			isReplyToBot = msg.ReplyTo != nil &&
				msg.ReplyTo.Sender != nil &&
				msg.ReplyTo.Sender.ID == botID
		}
	}
	metadata := map[string]any{
		"is_mentioned":     isMentioned,
		"is_reply_to_bot":  isReplyToBot,
		"media_group_id":   strings.TrimSpace(anchor.AlbumID),
		"media_group_size": len(items),
	}
	return a.toInboundTelegramMessage(bot, cfg, anchor, text, attachments, metadata)
}

func (a *TelegramAdapter) seenTelegramUpdate(configID string, updateID int, now time.Time) bool {
	if a == nil || updateID <= 0 {
		return false
	}
	key := strings.TrimSpace(configID) + ":" + strconv.Itoa(updateID)
	if key == ":" {
		return false
	}

	cutoff := now.Add(-telegramUpdateDedupeTTL)

	a.seenUpdatesMu.Lock()
	defer a.seenUpdatesMu.Unlock()

	for seenKey, seenAt := range a.seenUpdates {
		if seenAt.Before(cutoff) {
			delete(a.seenUpdates, seenKey)
		}
	}

	if _, exists := a.seenUpdates[key]; exists {
		return true
	}
	a.seenUpdates[key] = now
	return false
}

func (a *TelegramAdapter) toInboundTelegramMessage(
	bot *tele.Bot,
	_ channel.ChannelConfig,
	raw *tele.Message,
	text string,
	attachments []channel.Attachment,
	metadata map[string]any,
) (channel.InboundMessage, bool) {
	if raw == nil {
		return channel.InboundMessage{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" && len(attachments) == 0 {
		return channel.InboundMessage{}, false
	}
	rawText := text
	subjectID, displayName, attrs := resolveTelegramSender(raw)
	chatID := ""
	chatTypeRaw := ""
	chatType := channel.ConversationTypePrivate
	chatName := ""
	if raw.Chat != nil {
		chatID = strconv.FormatInt(raw.Chat.ID, 10)
		chatTypeRaw = strings.TrimSpace(string(raw.Chat.Type))
		chatType = normalizeTelegramConversationType(chatTypeRaw)
		chatName = strings.TrimSpace(raw.Chat.Title)
	}
	replyRef := buildTelegramReplyRef(raw, chatID)
	if replyRef != nil {
		replyRef.Attachments = a.collectTelegramAttachments(bot, raw.ReplyTo)
	}
	forwardRef := buildTelegramForwardRef(raw)
	botUsername := ""
	botID := int64(0)
	if bot != nil && bot.Me != nil {
		botUsername = bot.Me.Username
		botID = bot.Me.ID
	}
	isReplyToBot := raw.ReplyTo != nil &&
		raw.ReplyTo.Sender != nil &&
		raw.ReplyTo.Sender.ID == botID
	isMentioned := isTelegramBotMentioned(raw, botUsername)
	meta := map[string]any{
		"is_mentioned":    isMentioned,
		"is_reply_to_bot": isReplyToBot,
		"raw_text":        rawText,
		"raw_chat_type":   chatTypeRaw,
	}
	for key, value := range metadata {
		meta[key] = value
	}
	richParts := extractTelegramMessageParts(raw)
	format := channel.MessageFormatPlain
	if len(richParts) > 0 {
		format = channel.MessageFormatRich
	}

	return channel.InboundMessage{
		Channel: Type,
		Message: channel.Message{
			ID:          strconv.Itoa(raw.ID),
			Format:      format,
			Text:        text,
			Parts:       richParts,
			Attachments: attachments,
			Reply:       replyRef,
			Forward:     forwardRef,
		},
		ReplyTarget: chatID,
		Sender: channel.Identity{
			SubjectID:   subjectID,
			DisplayName: displayName,
			Attributes:  attrs,
		},
		Conversation: channel.Conversation{
			ID:   chatID,
			Type: chatType,
			Name: chatName,
		},
		ReceivedAt: time.Unix(raw.Unixtime, 0).UTC(),
		Source:     "telegram",
		Metadata:   meta,
	}, true
}

func (a *TelegramAdapter) logTelegramInbound(configID string, msg channel.InboundMessage) {
	if a.logger == nil {
		return
	}
	a.logger.Info(
		"inbound received",
		slog.String("config_id", configID),
		slog.String("chat_type", msg.Conversation.Type),
		slog.String("chat_id", msg.Conversation.ID),
		slog.String("user_id", msg.Sender.Attribute("user_id")),
		slog.String("username", msg.Sender.Attribute("username")),
		slog.String("text", common.SummarizeText(msg.Message.Text)),
		slog.Int("attachments", len(msg.Message.Attachments)),
	)
}

// Send delivers an outbound message to Telegram, handling text, attachments, and replies.
func (a *TelegramAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.PreparedOutboundMessage) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("decode config failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return err
	}
	to := strings.TrimSpace(msg.Target)
	if to == "" {
		return errors.New("telegram target is required")
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return err
	}
	if msg.Message.Message.IsEmpty() {
		return errors.New("message is required")
	}
	text := strings.TrimSpace(msg.Message.Message.PlainText())
	text, parseMode := formatTelegramOutput(text, msg.Message.Message.Format)
	replyTo := parseReplyToMessageID(msg.Message.Message.Reply)
	if len(msg.Message.Attachments) > 0 {
		usedCaption := false
		for i, att := range msg.Message.Attachments {
			caption := ""
			if !usedCaption && text != "" {
				caption = text
				usedCaption = true
			}
			applyReply := replyTo
			if i > 0 {
				applyReply = 0
			}
			if err := sendTelegramAttachmentWithAssets(ctx, bot, to, att, caption, applyReply, parseMode); err != nil {
				if a.logger != nil {
					a.logger.Error("send attachment failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
				}
				return err
			}
		}
		if text != "" && !usedCaption {
			return sendTelegramText(bot, to, text, replyTo, parseMode)
		}
		return nil
	}
	if len(msg.Message.Message.Actions) > 0 {
		return sendTelegramTextWithActions(bot, to, text, replyTo, parseMode, msg.Message.Message.Actions)
	}
	return sendTelegramText(bot, to, text, replyTo, parseMode)
}

// Update edits an already-sent message in place (text + inline keyboard),
// satisfying channel.MessageEditor. It powers interactive pagination/selection:
// passing empty Actions removes the keyboard. Channel-username targets are not
// supported (edits require a numeric chat ID).
func (a *TelegramAdapter) Update(_ context.Context, cfg channel.ChannelConfig, target string, messageID string, msg channel.PreparedMessage) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return err
	}
	chatID, channelUsername, err := parseTelegramTarget(strings.TrimSpace(target))
	if err != nil {
		return err
	}
	if channelUsername != "" {
		return errors.New("telegram: editing channel-username targets is not supported")
	}
	mid, err := strconv.Atoi(strings.TrimSpace(messageID))
	if err != nil {
		return fmt.Errorf("telegram: invalid message id %q: %w", messageID, err)
	}
	text := strings.TrimSpace(msg.Message.PlainText())
	text, parseMode := formatTelegramOutput(text, msg.Message.Format)
	return editTelegramMessageTextWithActions(bot, chatID, mid, text, parseMode, msg.Message.Actions)
}

// Unsend deletes a previously-sent message, satisfying channel.MessageEditor.
func (a *TelegramAdapter) Unsend(_ context.Context, cfg channel.ChannelConfig, target string, messageID string) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return err
	}
	chatID, channelUsername, err := parseTelegramTarget(strings.TrimSpace(target))
	if err != nil {
		return err
	}
	if channelUsername != "" {
		return errors.New("telegram: deleting channel-username targets is not supported")
	}
	mid, err := strconv.Atoi(strings.TrimSpace(messageID))
	if err != nil {
		return fmt.Errorf("telegram: invalid message id %q: %w", messageID, err)
	}
	return bot.Delete(&tele.StoredMessage{MessageID: strconv.Itoa(mid), ChatID: chatID})
}

// OpenStream opens a Telegram streaming session.
// For private chats, uses sendMessageDraft to stream partial content with smooth
// animation, then sends a final permanent message via sendMessage.
// For group/channel chats, sends one message then edits it in place as deltas
// arrive (editMessageText), avoiding one message per delta and rate limits.
func (a *TelegramAdapter) OpenStream(ctx context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.PreparedOutboundStream, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("telegram target is required")
	}
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return nil, fmt.Errorf("telegram open stream: %w", err)
	}
	channel.SetIMErrorSecrets("telegram:"+cfg.ID, telegramCfg.BotToken)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	isPrivateChat := false
	var chatID int64
	if opts.Metadata != nil {
		if ct, ok := opts.Metadata["conversation_type"].(string); ok && ct == "private" {
			if parsed, err := strconv.ParseInt(target, 10, 64); err == nil {
				isPrivateChat = true
				chatID = parsed
			}
		}
	}
	return &telegramOutboundStream{
		adapter:       a,
		cfg:           cfg,
		target:        target,
		reply:         opts.Reply,
		parseMode:     "",
		isPrivateChat: isPrivateChat,
		streamChatID:  chatID,
		draftID:       1,
	}, nil
}

func resolveTelegramSender(msg *tele.Message) (string, string, map[string]string) {
	attrs := map[string]string{}
	if msg == nil {
		return "", "", attrs
	}
	if msg.Chat != nil {
		attrs["chat_id"] = strconv.FormatInt(msg.Chat.ID, 10)
	}
	if msg.Sender != nil {
		userID := strconv.FormatInt(msg.Sender.ID, 10)
		username := strings.TrimSpace(msg.Sender.Username)
		if userID != "" {
			attrs["user_id"] = userID
		}
		if username != "" {
			attrs["username"] = username
		}
		displayName := resolveTelegramDisplayName(msg.Sender)
		externalID := userID
		if externalID == "" {
			externalID = username
		}
		return externalID, displayName, attrs
	}
	if msg.SenderChat != nil {
		senderChatID := strconv.FormatInt(msg.SenderChat.ID, 10)
		if senderChatID != "" {
			attrs["sender_chat_id"] = senderChatID
		}
		if msg.SenderChat.Username != "" {
			attrs["sender_chat_username"] = strings.TrimSpace(msg.SenderChat.Username)
		}
		if msg.SenderChat.Title != "" {
			attrs["sender_chat_title"] = strings.TrimSpace(msg.SenderChat.Title)
		}
		displayName := strings.TrimSpace(msg.SenderChat.Title)
		if displayName == "" {
			displayName = strings.TrimSpace(msg.SenderChat.Username)
		}
		externalID := senderChatID
		if externalID == "" {
			externalID = attrs["sender_chat_username"]
		}
		if externalID == "" {
			externalID = attrs["chat_id"]
		}
		return externalID, displayName, attrs
	}
	return "", "", attrs
}

func parseReplyToMessageID(reply *channel.ReplyRef) int {
	if reply == nil {
		return 0
	}
	raw := strings.TrimSpace(reply.MessageID)
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return value
}

func normalizeTelegramConversationType(chatType string) string {
	switch strings.ToLower(strings.TrimSpace(chatType)) {
	case "private":
		return channel.ConversationTypePrivate
	case "group", "supergroup", "channel":
		return channel.ConversationTypeGroup
	default:
		return channel.ConversationTypeGroup
	}
}

// telegramRecipient converts a target string into a tele.Recipient. For
// numeric chat IDs it returns tele.ChatID; for @channel_username it returns
// a custom Recipient because tele.Chat.Recipient() returns the (unset) ID.
func telegramRecipient(target string) (tele.Recipient, int64, error) {
	chatID, channelUsername, err := parseTelegramTarget(target)
	if err != nil {
		return nil, 0, err
	}
	if channelUsername != "" {
		return telegramChannelRecipient(channelUsername), 0, nil
	}
	return tele.ChatID(chatID), chatID, nil
}

func sendTelegramText(bot *tele.Bot, target string, text string, replyTo int, parseMode string) error {
	_, _, err := sendTelegramTextReturnMessage(bot, target, text, replyTo, parseMode)
	return err
}

var sendTextForTest func(bot *tele.Bot, target string, text string, replyTo int, parseMode string) (int64, int, error)

// sendTelegramTextReturnMessage sends a text message and returns the chat ID and message ID for later editing.
func sendTelegramTextReturnMessage(bot *tele.Bot, target string, text string, replyTo int, parseMode string) (chatID int64, messageID int, err error) {
	text = truncateTelegramText(sanitizeTelegramText(text))
	if sendTextForTest != nil {
		return sendTextForTest(bot, target, text, replyTo, parseMode)
	}
	recipient, parsedChatID, parseErr := telegramRecipient(target)
	if parseErr != nil {
		return 0, 0, parseErr
	}
	opts := &tele.SendOptions{ParseMode: parseMode}
	if replyTo > 0 {
		opts.ReplyTo = &tele.Message{ID: replyTo}
	}
	sent, err := bot.Send(recipient, text, opts)
	if err != nil {
		return 0, 0, err
	}
	chatID = parsedChatID
	if sent.Chat != nil {
		chatID = sent.Chat.ID
	}
	messageID = sent.ID
	return chatID, messageID, nil
}

func sendTelegramTextWithActions(bot *tele.Bot, target string, text string, replyTo int, parseMode string, actions []channel.Action) error {
	_, _, err := sendTelegramTextWithActionsReturnMessage(bot, target, text, replyTo, parseMode, actions)
	return err
}

func sendTelegramTextWithActionsReturnMessage(bot *tele.Bot, target string, text string, replyTo int, parseMode string, actions []channel.Action) (chatID int64, messageID int, err error) {
	text = truncateTelegramText(sanitizeTelegramText(text))
	recipient, parsedChatID, parseErr := telegramRecipient(target)
	if parseErr != nil {
		return 0, 0, parseErr
	}
	opts := &tele.SendOptions{ParseMode: parseMode}
	if replyTo > 0 {
		opts.ReplyTo = &tele.Message{ID: replyTo}
	}
	markup := telegramInlineKeyboard(actions)
	if markup != nil && len(markup.InlineKeyboard) > 0 {
		opts.ReplyMarkup = markup
	}
	sent, err := bot.Send(recipient, text, opts)
	if err != nil {
		return 0, 0, err
	}
	chatID = parsedChatID
	if sent.Chat != nil {
		chatID = sent.Chat.ID
	}
	return chatID, sent.ID, nil
}

var sendEditForTest func(bot *tele.Bot, chatID int64, messageID int, text string, parseMode string) error

// collapseToTitle returns the first non-empty line of a message, used to
// shrink an interactive card to a short breadcrumb when the user taps Close.
// Returns empty when every line is blank — caller should skip the edit so
// callers don't have to choose a localized "(closed)" placeholder string with
// no localizer available at the callback boundary.
func collapseToTitle(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			return s
		}
	}
	return ""
}

func editTelegramMessageText(bot *tele.Bot, chatID int64, messageID int, text string, parseMode string) error {
	err := rawEditTelegramMessageText(bot, chatID, messageID, text, parseMode)
	if err != nil && (isTelegramMessageNotModified(err) || isTelegramEditUnrecoverable(err)) {
		return nil
	}
	return err
}

// rawEditTelegramMessageText performs the edit and returns the raw API error,
// swallowing nothing. editTelegramMessageText wraps it with the
// not-modified/unrecoverable swallow used by interactive edits (where a tap on a
// deleted card should be a quiet no-op, not a burned retry). The streaming final
// path uses the raw form instead so it can SEE an unrecoverable error and recover
// the answer (post it as a new message) rather than dropping it silently.
func rawEditTelegramMessageText(bot *tele.Bot, chatID int64, messageID int, text string, parseMode string) error {
	text = truncateTelegramText(sanitizeTelegramText(text))
	if sendEditForTest != nil {
		return sendEditForTest(bot, chatID, messageID, text, parseMode)
	}
	stored := &tele.StoredMessage{MessageID: strconv.Itoa(messageID), ChatID: chatID}
	opts := &tele.SendOptions{ParseMode: parseMode}
	_, err := bot.Edit(stored, text, opts)
	return err
}

func editTelegramMessageTextWithActions(bot *tele.Bot, chatID int64, messageID int, text string, parseMode string, actions []channel.Action) error {
	// With no actions, omit reply_markup entirely. Passing a *ReplyMarkup with
	// an empty InlineKeyboard would serialize to {}, which telebot treats as
	// "remove keyboard". For text-only updates after model selection we want
	// to update the text AND remove the stale keyboard, which editTelegramMessageText
	// achieves by sending no reply_markup at all.
	if len(actions) == 0 {
		return editTelegramMessageText(bot, chatID, messageID, text, parseMode)
	}
	text = truncateTelegramText(sanitizeTelegramText(text))
	markup := telegramInlineKeyboard(actions)
	stored := &tele.StoredMessage{MessageID: strconv.Itoa(messageID), ChatID: chatID}
	opts := &tele.SendOptions{ParseMode: parseMode, ReplyMarkup: markup}
	_, err := bot.Edit(stored, text, opts)
	if err != nil && (isTelegramMessageNotModified(err) || isTelegramEditUnrecoverable(err)) {
		return nil
	}
	return err
}

func telegramInlineKeyboard(actions []channel.Action) *tele.ReplyMarkup {
	rowOrder := make([]int, 0, len(actions))
	rowButtons := make(map[int][]tele.InlineButton, len(actions))
	for _, action := range actions {
		label := strings.TrimSpace(action.Label)
		value := strings.TrimSpace(action.Value)
		if label == "" || value == "" {
			continue
		}
		if _, ok := rowButtons[action.Row]; !ok {
			rowOrder = append(rowOrder, action.Row)
		}
		rowButtons[action.Row] = append(rowButtons[action.Row], tele.InlineButton{Text: label, Data: value})
	}
	rows := make([][]tele.InlineButton, 0, len(rowOrder))
	for _, r := range rowOrder {
		rows = append(rows, rowButtons[r])
	}
	return &tele.ReplyMarkup{InlineKeyboard: rows}
}

var sendDraftForTest func(bot *tele.Bot, chatID int64, draftID int, text string, parseMode string) error

// sendTelegramDraft calls the sendMessageDraft Bot API method to stream a
// partial message to a private chat while it is being generated.
func sendTelegramDraft(bot *tele.Bot, chatID int64, draftID int, text string, parseMode string) error {
	text = truncateTelegramText(sanitizeTelegramText(text))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if sendDraftForTest != nil {
		return sendDraftForTest(bot, chatID, draftID, text, parseMode)
	}
	opts := &tele.SendOptions{ParseMode: parseMode}
	return bot.SendDraft(tele.ChatID(chatID), draftID, text, opts)
}

func isTelegramMessageNotModified(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *tele.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 400 && strings.Contains(strings.ToLower(apiErr.Description), "message is not modified")
	}
	// telebot only types errors whose description matches a registered sentinel
	// (ErrMessageNotModified, ErrSameMessageContent, …). Other 4xx variants are
	// returned as fmt.Errorf("telegram: <description> (<code>)") and would slip
	// through *tele.Error type checks. Match by the wire string instead.
	return telegramTextErrorMatches(err, 400, "message is not modified")
}

// isTelegramEditUnrecoverable reports whether an edit failed because the target
// message is gone or can never be edited — retrying cannot help. The
// interactive edit path (pagination/selection via Update) flows through the
// generic outbound retry loop, which would otherwise burn RetryMax attempts
// (each with a linear backoff sleep) on a message the user already deleted or
// that aged past Telegram's edit window. Treated as terminal — the edit is a
// no-op — exactly like "message is not modified".
func isTelegramEditUnrecoverable(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *tele.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code != 400 {
			return false
		}
		m := strings.ToLower(apiErr.Description)
		return strings.Contains(m, "message to edit not found") ||
			strings.Contains(m, "message can't be edited") ||
			strings.Contains(m, "message_id_invalid")
	}
	// "message to edit not found" and "MESSAGE_ID_INVALID" are not registered
	// as telebot sentinels, so they arrive as plain fmt.Errorf strings. Without
	// this fallback the streaming recovery path (editStreamMessageFinal →
	// sendPermanentMessage) silently drops the final answer when the placeholder
	// message has been deleted or aged past Telegram's edit window.
	return telegramTextErrorMatches(err, 400,
		"message to edit not found",
		"message_id_invalid")
}

// telegramTextErrorMatches reports whether err's message looks like telebot's
// fmt.Errorf("telegram: <description> (<code>)") wrapping with the given code
// and at least one of the (case-insensitive) substrings present.
func telegramTextErrorMatches(err error, code int, substrings ...string) bool {
	// FloodError is the only typed telebot error a 429 can take. Skip it here
	// to avoid recursing into its Error() method, which dereferences an
	// unexported inner *Error; test stubs that construct FloodError{RetryAfter:N}
	// directly would panic otherwise.
	var floodErr tele.FloodError
	if errors.As(err, &floodErr) {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, fmt.Sprintf("(%d)", code)) {
		return false
	}
	for _, s := range substrings {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func isTelegramTooManyRequests(err error) bool {
	if err == nil {
		return false
	}
	var floodErr tele.FloodError
	if errors.As(err, &floodErr) {
		return true
	}
	var apiErr *tele.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429
	}
	return false
}

func getTelegramRetryAfter(err error) time.Duration {
	if err == nil {
		return 0
	}
	var floodErr tele.FloodError
	if errors.As(err, &floodErr) && floodErr.RetryAfter > 0 {
		return time.Duration(floodErr.RetryAfter) * time.Second
	}
	return 0
}

func sendTelegramAttachmentWithAssets(ctx context.Context, bot *tele.Bot, target string, att channel.PreparedAttachment, caption string, replyTo int, parseMode string) error {
	return sendTelegramAttachmentImpl(ctx, bot, target, att, caption, replyTo, parseMode)
}

func sendTelegramAttachmentImpl(ctx context.Context, bot *tele.Bot, target string, att channel.PreparedAttachment, caption string, replyTo int, parseMode string) error {
	if strings.TrimSpace(caption) == "" && strings.TrimSpace(att.Logical.Caption) != "" {
		caption = strings.TrimSpace(att.Logical.Caption)
	}
	file, err := resolveTelegramFile(ctx, att)
	if err != nil {
		return err
	}
	recipient, _, recipErr := telegramRecipient(target)
	if recipErr != nil {
		return recipErr
	}
	opts := &tele.SendOptions{ParseMode: parseMode}
	if replyTo > 0 {
		opts.ReplyTo = &tele.Message{ID: replyTo}
	}
	name := strings.TrimSpace(att.Name)
	if name == "" {
		name = fileNameFromMime(att.Mime, string(att.Logical.Type))
	}
	switch att.Logical.Type {
	case channel.AttachmentImage:
		_, sendErr := bot.Send(recipient, &tele.Photo{File: file, Caption: caption}, opts)
		return sendErr
	case channel.AttachmentFile, "":
		_, sendErr := bot.Send(recipient, &tele.Document{File: file, Caption: caption, FileName: name}, opts)
		return sendErr
	case channel.AttachmentAudio:
		_, sendErr := bot.Send(recipient, &tele.Audio{File: file, Caption: caption, FileName: name}, opts)
		return sendErr
	case channel.AttachmentVoice:
		_, sendErr := bot.Send(recipient, &tele.Voice{File: file, Caption: caption}, opts)
		return sendErr
	case channel.AttachmentVideo:
		_, sendErr := bot.Send(recipient, &tele.Video{File: file, Caption: caption, FileName: name}, opts)
		return sendErr
	case channel.AttachmentGIF:
		_, sendErr := bot.Send(recipient, &tele.Animation{File: file, Caption: caption, FileName: name}, opts)
		return sendErr
	default:
		return fmt.Errorf("unsupported attachment type: %s", att.Logical.Type)
	}
}

// resolveTelegramFile maps a prepared attachment into telebot's file model.
func resolveTelegramFile(ctx context.Context, att channel.PreparedAttachment) (tele.File, error) {
	switch att.Kind {
	case channel.PreparedAttachmentNativeRef:
		if strings.TrimSpace(att.NativeRef) == "" {
			return tele.File{}, errors.New("telegram native ref is required")
		}
		return tele.File{FileID: strings.TrimSpace(att.NativeRef)}, nil
	case channel.PreparedAttachmentPublicURL:
		if strings.TrimSpace(att.PublicURL) == "" {
			return tele.File{}, errors.New("telegram public url is required")
		}
		return tele.FromURL(strings.TrimSpace(att.PublicURL)), nil
	case channel.PreparedAttachmentUpload:
		if att.Open == nil {
			return tele.File{}, errors.New("telegram upload attachment is not openable")
		}
		reader, err := att.Open(ctx)
		if err != nil {
			return tele.File{}, err
		}
		defer func() { _ = reader.Close() }()
		data, err := media.ReadAllWithLimit(reader, media.MaxAssetBytes)
		if err != nil {
			return tele.File{}, err
		}
		// telebot.FromReader consumes the reader at send time; buffer the bytes
		// so the reader's lifetime is not tied to this call's defer.
		return tele.FromReader(bytes.NewReader(data)), nil
	default:
		return tele.File{}, fmt.Errorf("unsupported telegram attachment kind: %s", att.Kind)
	}
}

func fileNameFromMime(mime, fallbackType string) string {
	mime = strings.ToLower(strings.TrimSpace(mime))
	switch {
	case strings.HasPrefix(mime, "image/png"):
		return "image.png"
	case strings.HasPrefix(mime, "image/jpeg"), strings.HasPrefix(mime, "image/jpg"):
		return "image.jpg"
	case strings.HasPrefix(mime, "image/gif"):
		return "image.gif"
	case strings.HasPrefix(mime, "image/webp"):
		return "image.webp"
	case strings.HasPrefix(mime, "audio/"):
		return "audio.mp3"
	case strings.HasPrefix(mime, "video/"):
		return "video.mp4"
	default:
		if strings.TrimSpace(fallbackType) == "image" {
			return "image.png"
		}
		return "file.bin"
	}
}

func buildTelegramReplyRef(msg *tele.Message, chatID string) *channel.ReplyRef {
	if msg == nil || msg.ReplyTo == nil {
		return nil
	}
	replyTo := msg.ReplyTo
	ref := &channel.ReplyRef{
		MessageID: strconv.Itoa(replyTo.ID),
		Target:    strings.TrimSpace(chatID),
		Sender:    resolveTelegramDisplayName(replyTo.Sender),
	}
	preview := strings.TrimSpace(replyTo.Text)
	if preview == "" {
		preview = strings.TrimSpace(replyTo.Caption)
	}
	if preview != "" {
		if len([]rune(preview)) > 200 {
			preview = string([]rune(preview)[:200]) + "..."
		}
		ref.Preview = preview
	}
	return ref
}

// resolveTelegramDisplayName returns a display name for a Telegram user.
// Format: "FirstName LastName (@username)" when both are available,
// "FirstName LastName" when only name is set, "@username" when only username is set.
func resolveTelegramDisplayName(user *tele.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	username := strings.TrimSpace(user.Username)
	if name != "" && username != "" {
		return name + " (@" + username + ")"
	}
	if name != "" {
		return name
	}
	if username != "" {
		return "@" + username
	}
	return ""
}

func buildTelegramForwardRef(msg *tele.Message) *channel.ForwardRef {
	if msg == nil {
		return nil
	}
	ref := &channel.ForwardRef{}
	if msg.OriginalSender != nil {
		ref.FromUserID = strconv.FormatInt(msg.OriginalSender.ID, 10)
		ref.Sender = resolveTelegramDisplayName(msg.OriginalSender)
	}
	if msg.OriginalChat != nil {
		ref.FromConversationID = strconv.FormatInt(msg.OriginalChat.ID, 10)
		title := strings.TrimSpace(msg.OriginalChat.Title)
		username := strings.TrimSpace(msg.OriginalChat.Username)
		switch {
		case title != "" && username != "":
			ref.Sender = title + " (@" + username + ")"
		case title != "":
			ref.Sender = title
		case username != "":
			ref.Sender = "@" + username
		}
	}
	if ref.Sender == "" {
		ref.Sender = strings.TrimSpace(msg.OriginalSenderName)
	}
	if msg.OriginalMessageID > 0 {
		ref.MessageID = strconv.Itoa(msg.OriginalMessageID)
	}
	if msg.OriginalUnixtime > 0 {
		ref.Date = int64(msg.OriginalUnixtime)
	}
	if ref.MessageID == "" && ref.FromUserID == "" && ref.FromConversationID == "" && ref.Sender == "" && ref.Date == 0 {
		return nil
	}
	return ref
}

// parseTelegramTarget resolves a target string into a numeric chat ID and an
// optional channel username. Exactly one of chatID or channelUsername will be
// set; callers can use this to construct any message config type.
func parseTelegramTarget(target string) (chatID int64, channelUsername string, err error) {
	if strings.HasPrefix(target, "@") {
		return 0, target, nil
	}
	chatID, err = strconv.ParseInt(target, 10, 64)
	if err != nil {
		return 0, "", errors.New("telegram target must be @username or chat_id")
	}
	return chatID, "", nil
}

func isTelegramBotMentioned(msg *tele.Message, botUsername string) bool {
	if msg == nil {
		return false
	}
	normalizedBot := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(botUsername), "@"))
	if normalizedBot != "" {
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			text = strings.TrimSpace(msg.Caption)
		}
		if text != "" {
			if strings.Contains(strings.ToLower(text), "@"+normalizedBot) {
				return true
			}
		}
	}
	entities := make(tele.Entities, 0, len(msg.Entities)+len(msg.CaptionEntities))
	entities = append(entities, msg.Entities...)
	entities = append(entities, msg.CaptionEntities...)
	for _, entity := range entities {
		if entity.Type == tele.EntityTMention && entity.User != nil && entity.User.IsBot {
			if normalizedBot != "" && strings.EqualFold(entity.User.Username, normalizedBot) {
				return true
			}
		}
	}
	return false
}

func (a *TelegramAdapter) collectTelegramAttachments(bot *tele.Bot, msg *tele.Message) []channel.Attachment {
	if msg == nil {
		return nil
	}
	attachments := make([]channel.Attachment, 0, 1)
	if msg.Photo != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentImage, msg.Photo.FileID, "", "", msg.Photo.FileSize)
		att.Width = msg.Photo.Width
		att.Height = msg.Photo.Height
		attachments = append(attachments, att)
	}
	if msg.Document != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentFile, msg.Document.FileID, msg.Document.FileName, msg.Document.MIME, msg.Document.FileSize)
		attachments = append(attachments, att)
	}
	if msg.Audio != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentAudio, msg.Audio.FileID, msg.Audio.FileName, msg.Audio.MIME, msg.Audio.FileSize)
		att.DurationMs = int64(msg.Audio.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Voice != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentVoice, msg.Voice.FileID, "", msg.Voice.MIME, msg.Voice.FileSize)
		att.DurationMs = int64(msg.Voice.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Video != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentVideo, msg.Video.FileID, msg.Video.FileName, msg.Video.MIME, msg.Video.FileSize)
		att.Width = msg.Video.Width
		att.Height = msg.Video.Height
		att.DurationMs = int64(msg.Video.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Animation != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentGIF, msg.Animation.FileID, msg.Animation.FileName, msg.Animation.MIME, msg.Animation.FileSize)
		att.Width = msg.Animation.Width
		att.Height = msg.Animation.Height
		att.DurationMs = int64(msg.Animation.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Sticker != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentImage, msg.Sticker.FileID, "", "", msg.Sticker.FileSize)
		att.Width = msg.Sticker.Width
		att.Height = msg.Sticker.Height
		attachments = append(attachments, att)
	}
	caption := strings.TrimSpace(msg.Caption)
	if caption != "" {
		for i := range attachments {
			attachments[i].Caption = caption
		}
	}
	return attachments
}

func (a *TelegramAdapter) buildTelegramAttachment(bot *tele.Bot, attType channel.AttachmentType, fileID, name, mime string, size int64) channel.Attachment {
	url := ""
	if bot != nil && strings.TrimSpace(fileID) != "" {
		value, err := a.getFileDirectURL(bot, fileID)
		if err != nil {
			if a.logger != nil {
				a.logger.Warn("resolve file url failed", slog.Any("error", err))
			}
		} else {
			url = value
		}
	}
	att := channel.Attachment{
		Type:           attType,
		URL:            strings.TrimSpace(url),
		PlatformKey:    strings.TrimSpace(fileID),
		SourcePlatform: Type.String(),
		Name:           strings.TrimSpace(name),
		Mime:           strings.TrimSpace(mime),
		Size:           size,
		Metadata:       map[string]any{},
	}
	if fileID != "" {
		att.Metadata["file_id"] = fileID
	}
	return channel.NormalizeInboundChannelAttachment(att)
}

// ResolveAttachment resolves a Telegram attachment reference to a byte stream.
// It supports platform_key-based references and URL fallback.
func (a *TelegramAdapter) ResolveAttachment(ctx context.Context, cfg channel.ChannelConfig, attachment channel.Attachment) (channel.AttachmentPayload, error) {
	fileID := strings.TrimSpace(attachment.PlatformKey)
	if fileID == "" && strings.TrimSpace(attachment.URL) == "" {
		return channel.AttachmentPayload{}, errors.New("telegram attachment requires platform_key or url")
	}
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return channel.AttachmentPayload{}, err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return channel.AttachmentPayload{}, err
	}
	downloadURL := strings.TrimSpace(attachment.URL)
	if downloadURL == "" {
		downloadURL, err = a.getFileDirectURL(bot, fileID)
		if err != nil {
			return channel.AttachmentPayload{}, fmt.Errorf("resolve telegram file url: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("build download request: %w", err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req) //nolint:gosec // G704: URL is a Telegram file download URL from the Telegram Bot API
	if err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("download attachment: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer func() {
			_ = resp.Body.Close()
		}()
		_, _ = io.Copy(io.Discard, resp.Body)
		return channel.AttachmentPayload{}, fmt.Errorf("download attachment status: %d", resp.StatusCode)
	}
	maxBytes := media.MaxAssetBytes
	if resp.ContentLength > maxBytes {
		defer func() {
			_ = resp.Body.Close()
		}()
		_, _ = io.Copy(io.Discard, resp.Body)
		return channel.AttachmentPayload{}, fmt.Errorf("%w: max %d bytes", media.ErrAssetTooLarge, maxBytes)
	}
	mime := strings.TrimSpace(attachment.Mime)
	if mime == "" {
		mime = strings.TrimSpace(resp.Header.Get("Content-Type"))
		if base, _, ok := strings.Cut(mime, ";"); ok {
			mime = strings.TrimSpace(base)
		}
	}
	size := attachment.Size
	if size <= 0 && resp.ContentLength > 0 {
		size = resp.ContentLength
	}
	return channel.AttachmentPayload{
		Reader: resp.Body,
		Mime:   mime,
		Name:   strings.TrimSpace(attachment.Name),
		Size:   size,
	}, nil
}

// DiscoverSelf retrieves the bot's own identity from the Telegram platform.
func (a *TelegramAdapter) DiscoverSelf(_ context.Context, credentials map[string]any) (map[string]any, string, error) {
	cfg, err := parseConfig(credentials)
	if err != nil {
		return nil, "", err
	}
	bot, err := a.getOrCreateBot(cfg, "discover")
	if err != nil {
		return nil, "", fmt.Errorf("telegram discover self: %w", err)
	}
	if bot.Me == nil {
		return nil, "", errors.New("telegram discover self: bot identity not available")
	}
	identity := map[string]any{
		"user_id":  strconv.FormatInt(bot.Me.ID, 10),
		"username": bot.Me.Username,
	}
	name := strings.TrimSpace(bot.Me.FirstName + " " + bot.Me.LastName)
	if name != "" {
		identity["name"] = name
	}
	avatarURL := a.resolveUserAvatarURL(bot, bot.Me.ID)
	if avatarURL != "" {
		identity["avatar_url"] = avatarURL
	}
	return identity, strconv.FormatInt(bot.Me.ID, 10), nil
}

// resolveUserAvatarURL fetches the first profile photo for a Telegram user and returns a direct URL.
func (a *TelegramAdapter) resolveUserAvatarURL(bot *tele.Bot, userID int64) string {
	if bot == nil {
		return ""
	}
	photos, err := bot.ProfilePhotosOf(&tele.User{ID: userID})
	if err != nil || len(photos) == 0 {
		return ""
	}
	best := photos[0]
	if best.FileID == "" {
		return ""
	}
	url, err := a.getFileDirectURL(bot, best.FileID)
	if err != nil {
		return ""
	}
	return url
}

// sanitizeTelegramText ensures text is valid UTF-8 for the Telegram API.
// Strips invalid byte sequences and trailing incomplete multi-byte characters
// that may occur at streaming chunk boundaries.
func sanitizeTelegramText(text string) string {
	if utf8.ValidString(text) {
		return text
	}
	return strings.ToValidUTF8(text, "")
}

// truncateTelegramText truncates text to telegramMaxMessageLength on a valid
// UTF-8 rune boundary, appending "..." when truncation occurs.
func truncateTelegramText(text string) string {
	return textutil.TruncateRunesWithSuffix(text, telegramMaxMessageLength, "...")
}

// ProcessingStarted sends a "typing" chat action to indicate processing.
func (a *TelegramAdapter) ProcessingStarted(_ context.Context, cfg channel.ChannelConfig, _ channel.InboundMessage, info channel.ProcessingStatusInfo) (channel.ProcessingStatusHandle, error) {
	chatID := strings.TrimSpace(info.ReplyTarget)
	if chatID == "" {
		return channel.ProcessingStatusHandle{}, nil
	}
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return channel.ProcessingStatusHandle{}, err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return channel.ProcessingStatusHandle{}, err
	}
	if err := sendTelegramTyping(bot, chatID); err != nil && a.logger != nil {
		a.logger.Warn("send typing action failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
	}
	return channel.ProcessingStatusHandle{}, nil
}

// ProcessingCompleted is a no-op for Telegram (typing indicator clears automatically).
func (*TelegramAdapter) ProcessingCompleted(_ context.Context, _ channel.ChannelConfig, _ channel.InboundMessage, _ channel.ProcessingStatusInfo, _ channel.ProcessingStatusHandle) error {
	return nil
}

// ProcessingFailed is a no-op for Telegram (typing indicator clears automatically).
func (*TelegramAdapter) ProcessingFailed(_ context.Context, _ channel.ChannelConfig, _ channel.InboundMessage, _ channel.ProcessingStatusInfo, _ channel.ProcessingStatusHandle, _ error) error {
	return nil
}

func sendTelegramTyping(bot *tele.Bot, chatID string) error {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	return bot.Notify(tele.ChatID(chatIDInt), tele.Typing)
}

func setTelegramReaction(bot *tele.Bot, chatID, messageID, emoji string) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction":   []map[string]string{{"type": "emoji", "emoji": emoji}},
	}
	_, err := bot.Raw("setMessageReaction", payload)
	return err
}

func clearTelegramReaction(bot *tele.Bot, chatID, messageID string) error {
	payload := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"reaction":   []any{},
	}
	_, err := bot.Raw("setMessageReaction", payload)
	return err
}

// React adds an emoji reaction to a message (implements channel.Reactor).
func (a *TelegramAdapter) React(_ context.Context, cfg channel.ChannelConfig, target string, messageID string, emoji string) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return err
	}
	return setTelegramReaction(bot, target, messageID, emoji)
}

// Unreact removes the bot's reaction from a message (implements channel.Reactor).
// The emoji parameter is ignored; Telegram clears all bot reactions at once.
func (a *TelegramAdapter) Unreact(_ context.Context, cfg channel.ChannelConfig, target string, messageID string, _ string) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	bot, err := a.getOrCreateBot(telegramCfg, cfg.ID)
	if err != nil {
		return err
	}
	return clearTelegramReaction(bot, target, messageID)
}
