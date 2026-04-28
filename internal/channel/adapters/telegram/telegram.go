package telegram

import (
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

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"golang.org/x/time/rate"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/common"
	"github.com/memohai/memoh/internal/media"
	"github.com/memohai/memoh/internal/textutil"
)

const (
	telegramMaxMessageLength        = 4096
	telegramMediaGroupCollectWindow = 700 * time.Millisecond
	telegramUpdateDedupeTTL         = 10 * time.Minute
)

var (
	telegramBotLogger      = newSlogBotLogger(nil)
	telegramLoggerInitOnce sync.Once
)

type telegramMediaGroupBuffer struct {
	messages []*tgbotapi.Message
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
	bots          map[string]*tgbotapi.BotAPI // keyed by effective bot config
	fileEndpoints map[*tgbotapi.BotAPI]string // bot instance → file endpoint format string
	assets        assetOpener
	streamLimiter *rate.Limiter // global rate limiter for all streaming API calls
	seenUpdatesMu sync.Mutex
	seenUpdates   map[string]time.Time
}

// NewTelegramAdapter creates a TelegramAdapter with the given logger.
func NewTelegramAdapter(log *slog.Logger) *TelegramAdapter {
	if log == nil {
		log = slog.Default()
	}
	adapter := &TelegramAdapter{
		logger:        log.With(slog.String("adapter", "telegram")),
		bots:          make(map[string]*tgbotapi.BotAPI),
		fileEndpoints: make(map[*tgbotapi.BotAPI]string),
		streamLimiter: rate.NewLimiter(rate.Every(time.Second), 3), // 1 req/s sustained, burst of 3
		seenUpdates:   make(map[string]time.Time),
	}
	initTelegramBotLogger(adapter.logger)
	return adapter
}

func initTelegramBotLogger(log *slog.Logger) {
	telegramLoggerInitOnce.Do(func() {
		_ = tgbotapi.SetLogger(telegramBotLogger)
	})
	telegramBotLogger.SetLogger(log)
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

var getOrCreateBotForTest func(a *TelegramAdapter, token, configID string) (*tgbotapi.BotAPI, error)

func (a *TelegramAdapter) getOrCreateBot(cfg Config, configID string) (*tgbotapi.BotAPI, error) {
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
	bot, err = tgbotapi.NewBotAPIWithClient(cfg.BotToken, cfg.apiEndpoint(), httpClient)
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
func (a *TelegramAdapter) getFileDirectURL(bot *tgbotapi.BotAPI, fileID string) (string, error) {
	file, err := bot.GetFile(tgbotapi.FileConfig{FileID: fileID})
	if err != nil {
		return "", err
	}
	a.mu.RLock()
	endpoint := a.fileEndpoints[bot]
	a.mu.RUnlock()
	if endpoint == "" {
		endpoint = tgbotapi.FileEndpoint
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
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30
	updates := bot.GetUpdatesChan(updateConfig)
	connCtx, cancel := context.WithCancel(ctx)
	mediaGroups := make(map[string]*telegramMediaGroupBuffer)
	var mediaGroupsMu sync.Mutex

	flushMediaGroup := func(groupKey string) {
		var batch []*tgbotapi.Message
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
	queueMediaGroup := func(msg *tgbotapi.Message) bool {
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

	go func() {
		for {
			select {
			case <-connCtx.Done():
				flushAllMediaGroups()
				return
			case update, ok := <-updates:
				if !ok {
					flushAllMediaGroups()
					if a.logger != nil {
						a.logger.Info("updates channel closed", slog.String("config_id", cfg.ID))
					}
					return
				}
				if a.seenTelegramUpdate(cfg.ID, update.UpdateID, time.Now()) {
					if a.logger != nil {
						a.logger.Debug("skip duplicate telegram update",
							slog.String("config_id", cfg.ID),
							slog.Int("update_id", update.UpdateID),
						)
					}
					continue
				}
				if update.CallbackQuery != nil {
					if msg, ok := a.buildTelegramCallbackInboundMessage(cfg, update); ok {
						_, _ = bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, "OK"))
						_ = clearTelegramCallbackButtons(bot, update.CallbackQuery)
						a.dispatchInbound(connCtx, cfg, handler, msg)
					}
					continue
				}
				if update.Message == nil {
					continue
				}
				if queueMediaGroup(update.Message) {
					continue
				}
				flushMediaGroupsByChat(telegramChatID(update.Message))
				msg, ok := a.buildTelegramInboundMessage(bot, cfg, update)
				if !ok {
					continue
				}
				a.dispatchInbound(connCtx, cfg, handler, msg)
			}
		}
	}()

	stop := func(_ context.Context) error {
		if a.logger != nil {
			a.logger.Info("stop", slog.String("config_id", cfg.ID))
		}
		bot.StopReceivingUpdates()
		cancel()
		// Drain remaining updates so the library's polling goroutine can
		// finish writing and exit. Without this, the in-flight long-poll
		// HTTP request keeps the old getUpdates session alive, causing
		// "Conflict: terminated by other getUpdates request" when a new
		// connection starts with the same bot token.
		for range updates {
		}
		return nil
	}
	return channel.NewConnection(cfg, stop), nil
}

func telegramMediaGroupKey(msg *tgbotapi.Message) string {
	if msg == nil {
		return ""
	}
	mediaGroupID := strings.TrimSpace(msg.MediaGroupID)
	if mediaGroupID == "" {
		return ""
	}
	chatID := telegramChatID(msg)
	return fmt.Sprintf("%d:%s", chatID, mediaGroupID)
}

func telegramChatID(msg *tgbotapi.Message) int64 {
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

func (a *TelegramAdapter) buildTelegramInboundMessage(bot *tgbotapi.BotAPI, cfg channel.ChannelConfig, update tgbotapi.Update) (channel.InboundMessage, bool) {
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
		"update_id": update.UpdateID,
	})
}

func (a *TelegramAdapter) buildTelegramCallbackInboundMessage(cfg channel.ChannelConfig, update tgbotapi.Update) (channel.InboundMessage, bool) {
	cb := update.CallbackQuery
	if cb == nil || cb.Message == nil {
		return channel.InboundMessage{}, false
	}
	action, approvalID, ok := parseTelegramApprovalCallback(cb.Data)
	if !ok {
		return channel.InboundMessage{}, false
	}
	text := "/" + action + " " + approvalID
	raw := cb.Message
	raw.Text = text
	raw.From = cb.From
	replyID := strconv.Itoa(cb.Message.MessageID)
	msg, ok := a.toInboundTelegramMessage(nil, cfg, raw, text, nil, map[string]any{
		"update_id":         update.UpdateID,
		"callback_query_id": cb.ID,
	})
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

func clearTelegramCallbackButtons(bot *tgbotapi.BotAPI, cb *tgbotapi.CallbackQuery) error {
	if bot == nil || cb == nil || cb.Message == nil || cb.Message.Chat == nil {
		return nil
	}
	edit := tgbotapi.NewEditMessageReplyMarkup(
		cb.Message.Chat.ID,
		cb.Message.MessageID,
		tgbotapi.InlineKeyboardMarkup{},
	)
	_, err := bot.Request(edit)
	return err
}

func (a *TelegramAdapter) buildTelegramMediaGroupInboundMessage(
	bot *tgbotapi.BotAPI,
	cfg channel.ChannelConfig,
	raw []*tgbotapi.Message,
) (channel.InboundMessage, bool) {
	if len(raw) == 0 {
		return channel.InboundMessage{}, false
	}
	items := make([]*tgbotapi.Message, 0, len(raw))
	for _, msg := range raw {
		if msg != nil {
			items = append(items, msg)
		}
	}
	if len(items) == 0 {
		return channel.InboundMessage{}, false
	}
	slices.SortStableFunc(items, func(a, b *tgbotapi.Message) int {
		return cmp.Compare(a.MessageID, b.MessageID)
	})
	anchor := items[0]
	text := ""
	attachments := make([]channel.Attachment, 0, len(items))
	isMentioned := false
	isReplyToBot := false
	botUsername := ""
	botID := int64(0)
	if bot != nil {
		botUsername = bot.Self.UserName
		botID = bot.Self.ID
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
			isReplyToBot = msg.ReplyToMessage != nil &&
				msg.ReplyToMessage.From != nil &&
				msg.ReplyToMessage.From.ID == botID
		}
	}
	metadata := map[string]any{
		"is_mentioned":     isMentioned,
		"is_reply_to_bot":  isReplyToBot,
		"media_group_id":   strings.TrimSpace(anchor.MediaGroupID),
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
	bot *tgbotapi.BotAPI,
	_ channel.ChannelConfig,
	raw *tgbotapi.Message,
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
	if raw.ReplyToMessage != nil {
		if quotedAttachments := a.collectTelegramAttachments(bot, raw.ReplyToMessage); len(quotedAttachments) > 0 {
			attachments = append(attachments, quotedAttachments...)
		}
	}
	// Prepend forward origin so the AI knows where the message was forwarded from.
	if forwardCtx := buildTelegramForwardContext(raw); forwardCtx != "" {
		text = forwardCtx + "\n" + text
	}
	subjectID, displayName, attrs := resolveTelegramSender(raw)
	chatID := ""
	chatTypeRaw := ""
	chatType := channel.ConversationTypePrivate
	chatName := ""
	if raw.Chat != nil {
		chatID = strconv.FormatInt(raw.Chat.ID, 10)
		chatTypeRaw = strings.TrimSpace(raw.Chat.Type)
		chatType = normalizeTelegramConversationType(chatTypeRaw)
		chatName = strings.TrimSpace(raw.Chat.Title)
	}
	replyRef := buildTelegramReplyRef(raw, chatID)
	botUsername := ""
	botID := int64(0)
	if bot != nil {
		botUsername = bot.Self.UserName
		botID = bot.Self.ID
	}
	isReplyToBot := raw.ReplyToMessage != nil &&
		raw.ReplyToMessage.From != nil &&
		raw.ReplyToMessage.From.ID == botID
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
	mentionParts := extractTelegramMentionParts(raw)

	return channel.InboundMessage{
		Channel: Type,
		Message: channel.Message{
			ID:          strconv.Itoa(raw.MessageID),
			Format:      channel.MessageFormatPlain,
			Text:        text,
			Parts:       mentionParts,
			Attachments: attachments,
			Reply:       replyRef,
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
		ReceivedAt: time.Unix(int64(raw.Date), 0).UTC(),
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

func resolveTelegramSender(msg *tgbotapi.Message) (string, string, map[string]string) {
	attrs := map[string]string{}
	if msg == nil {
		return "", "", attrs
	}
	if msg.Chat != nil {
		attrs["chat_id"] = strconv.FormatInt(msg.Chat.ID, 10)
	}
	if msg.From != nil {
		userID := strconv.FormatInt(msg.From.ID, 10)
		username := strings.TrimSpace(msg.From.UserName)
		if userID != "" {
			attrs["user_id"] = userID
		}
		if username != "" {
			attrs["username"] = username
		}
		displayName := resolveTelegramDisplayName(msg.From)
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
		if msg.SenderChat.UserName != "" {
			attrs["sender_chat_username"] = strings.TrimSpace(msg.SenderChat.UserName)
		}
		if msg.SenderChat.Title != "" {
			attrs["sender_chat_title"] = strings.TrimSpace(msg.SenderChat.Title)
		}
		displayName := strings.TrimSpace(msg.SenderChat.Title)
		if displayName == "" {
			displayName = strings.TrimSpace(msg.SenderChat.UserName)
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

func sendTelegramText(bot *tgbotapi.BotAPI, target string, text string, replyTo int, parseMode string) error {
	_, _, err := sendTelegramTextReturnMessage(bot, target, text, replyTo, parseMode)
	return err
}

var sendTextForTest func(bot *tgbotapi.BotAPI, target string, text string, replyTo int, parseMode string) (int64, int, error)

// sendTelegramTextReturnMessage sends a text message and returns the chat ID and message ID for later editing.
func sendTelegramTextReturnMessage(bot *tgbotapi.BotAPI, target string, text string, replyTo int, parseMode string) (chatID int64, messageID int, err error) {
	text = truncateTelegramText(sanitizeTelegramText(text))
	if sendTextForTest != nil {
		return sendTextForTest(bot, target, text, replyTo, parseMode)
	}
	parsedChatID, channelUsername, parseErr := parseTelegramTarget(target)
	if parseErr != nil {
		return 0, 0, parseErr
	}
	var message tgbotapi.MessageConfig
	if channelUsername != "" {
		message = tgbotapi.NewMessageToChannel(channelUsername, text)
	} else {
		message = tgbotapi.NewMessage(parsedChatID, text)
	}
	message.ParseMode = parseMode
	if replyTo > 0 {
		message.ReplyToMessageID = replyTo
	}
	sent, err := bot.Send(message)
	if err != nil {
		return 0, 0, err
	}
	chatID = parsedChatID
	if sent.Chat != nil {
		chatID = sent.Chat.ID
	}
	messageID = sent.MessageID
	return chatID, messageID, nil
}

func sendTelegramTextWithActions(bot *tgbotapi.BotAPI, target string, text string, replyTo int, parseMode string, actions []channel.Action) error {
	_, _, err := sendTelegramTextWithActionsReturnMessage(bot, target, text, replyTo, parseMode, actions)
	return err
}

func sendTelegramTextWithActionsReturnMessage(bot *tgbotapi.BotAPI, target string, text string, replyTo int, parseMode string, actions []channel.Action) (chatID int64, messageID int, err error) {
	text = truncateTelegramText(sanitizeTelegramText(text))
	parsedChatID, channelUsername, parseErr := parseTelegramTarget(target)
	if parseErr != nil {
		return 0, 0, parseErr
	}
	var message tgbotapi.MessageConfig
	if channelUsername != "" {
		message = tgbotapi.NewMessageToChannel(channelUsername, text)
	} else {
		message = tgbotapi.NewMessage(parsedChatID, text)
	}
	message.ParseMode = parseMode
	if replyTo > 0 {
		message.ReplyToMessageID = replyTo
	}
	markup := telegramInlineKeyboard(actions)
	if len(markup.InlineKeyboard) > 0 {
		message.ReplyMarkup = markup
	}
	sent, err := bot.Send(message)
	if err != nil {
		return 0, 0, err
	}
	chatID = parsedChatID
	if sent.Chat != nil {
		chatID = sent.Chat.ID
	}
	return chatID, sent.MessageID, nil
}

var sendEditForTest func(bot *tgbotapi.BotAPI, edit tgbotapi.EditMessageTextConfig) error

// editTelegramMessageText sends an edit request. It handles "message is not modified"
// silently but returns 429 and other errors to the caller for higher-level retry decisions.
func editTelegramMessageText(bot *tgbotapi.BotAPI, chatID int64, messageID int, text string, parseMode string) error {
	text = truncateTelegramText(sanitizeTelegramText(text))
	edit := tgbotapi.NewEditMessageText(chatID, messageID, text)
	edit.ParseMode = parseMode
	send := sendEditForTest
	if send == nil {
		send = func(b *tgbotapi.BotAPI, e tgbotapi.EditMessageTextConfig) error { _, err := b.Send(e); return err }
	}
	err := send(bot, edit)
	if err != nil && isTelegramMessageNotModified(err) {
		return nil
	}
	return err
}

func editTelegramMessageTextWithActions(bot *tgbotapi.BotAPI, chatID int64, messageID int, text string, parseMode string, actions []channel.Action) error {
	text = truncateTelegramText(sanitizeTelegramText(text))
	markup := telegramInlineKeyboard(actions)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, messageID, text, markup)
	edit.ParseMode = parseMode
	_, err := bot.Send(edit)
	if err != nil && isTelegramMessageNotModified(err) {
		return nil
	}
	return err
}

func telegramInlineKeyboard(actions []channel.Action) tgbotapi.InlineKeyboardMarkup {
	keyboard := make([]tgbotapi.InlineKeyboardButton, 0, len(actions))
	for _, action := range actions {
		label := strings.TrimSpace(action.Label)
		value := strings.TrimSpace(action.Value)
		if label == "" || value == "" {
			continue
		}
		keyboard = append(keyboard, tgbotapi.NewInlineKeyboardButtonData(label, value))
	}
	return tgbotapi.NewInlineKeyboardMarkup(keyboard)
}

var sendDraftForTest func(bot *tgbotapi.BotAPI, chatID int64, draftID int, text string, parseMode string) error

// sendTelegramDraft calls the sendMessageDraft Bot API method to stream a
// partial message to a private chat while it is being generated.
func sendTelegramDraft(bot *tgbotapi.BotAPI, chatID int64, draftID int, text string, parseMode string) error {
	text = truncateTelegramText(sanitizeTelegramText(text))
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if sendDraftForTest != nil {
		return sendDraftForTest(bot, chatID, draftID, text, parseMode)
	}
	params := tgbotapi.Params{}
	_ = params.AddFirstValid("chat_id", chatID)
	params.AddNonZero("draft_id", draftID)
	params.AddNonEmpty("text", text)
	params.AddNonEmpty("parse_mode", parseMode)
	_, err := bot.MakeRequest("sendMessageDraft", params)
	return err
}

func isTelegramMessageNotModified(err error) bool {
	if err == nil {
		return false
	}
	var apiErr tgbotapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 400 && strings.Contains(apiErr.Message, "message is not modified")
	}
	return false
}

func isTelegramTooManyRequests(err error) bool {
	if err == nil {
		return false
	}
	var apiErr tgbotapi.Error
	if errors.As(err, &apiErr) {
		return apiErr.Code == 429
	}
	return false
}

func getTelegramRetryAfter(err error) time.Duration {
	if err == nil {
		return 0
	}
	var apiErr tgbotapi.Error
	if errors.As(err, &apiErr) && apiErr.RetryAfter > 0 {
		return time.Duration(apiErr.RetryAfter) * time.Second
	}
	return 0
}

func sendTelegramAttachmentWithAssets(ctx context.Context, bot *tgbotapi.BotAPI, target string, att channel.PreparedAttachment, caption string, replyTo int, parseMode string) error {
	return sendTelegramAttachmentImpl(ctx, bot, target, att, caption, replyTo, parseMode)
}

func sendTelegramAttachmentImpl(ctx context.Context, bot *tgbotapi.BotAPI, target string, att channel.PreparedAttachment, caption string, replyTo int, parseMode string) error {
	if strings.TrimSpace(caption) == "" && strings.TrimSpace(att.Logical.Caption) != "" {
		caption = strings.TrimSpace(att.Logical.Caption)
	}
	file, err := resolveTelegramFile(ctx, att)
	if err != nil {
		return err
	}
	chatID, channelUsername, targetErr := parseTelegramTarget(target)
	if targetErr != nil {
		return targetErr
	}
	isChannel := channelUsername != ""
	switch att.Logical.Type {
	case channel.AttachmentImage:
		var photo tgbotapi.PhotoConfig
		if isChannel {
			photo = tgbotapi.NewPhotoToChannel(channelUsername, file)
		} else {
			photo = tgbotapi.NewPhoto(chatID, file)
		}
		photo.Caption = caption
		photo.ParseMode = parseMode
		if replyTo > 0 {
			photo.ReplyToMessageID = replyTo
		}
		_, err := bot.Send(photo)
		return err
	case channel.AttachmentFile, "":
		var document tgbotapi.DocumentConfig
		if isChannel {
			document = tgbotapi.DocumentConfig{
				BaseFile: tgbotapi.BaseFile{
					BaseChat: tgbotapi.BaseChat{ChannelUsername: channelUsername},
					File:     file,
				},
			}
		} else {
			document = tgbotapi.NewDocument(chatID, file)
		}
		document.Caption = caption
		document.ParseMode = parseMode
		if replyTo > 0 {
			document.ReplyToMessageID = replyTo
		}
		_, sendErr := bot.Send(document)
		return sendErr
	case channel.AttachmentAudio:
		audio, err := buildTelegramAudio(target, file)
		if err != nil {
			return err
		}
		audio.Caption = caption
		audio.ParseMode = parseMode
		if replyTo > 0 {
			audio.ReplyToMessageID = replyTo
		}
		_, err = bot.Send(audio)
		return err
	case channel.AttachmentVoice:
		voice, err := buildTelegramVoice(target, file)
		if err != nil {
			return err
		}
		voice.Caption = caption
		voice.ParseMode = parseMode
		if replyTo > 0 {
			voice.ReplyToMessageID = replyTo
		}
		_, err = bot.Send(voice)
		return err
	case channel.AttachmentVideo:
		video, err := buildTelegramVideo(target, file)
		if err != nil {
			return err
		}
		video.Caption = caption
		video.ParseMode = parseMode
		if replyTo > 0 {
			video.ReplyToMessageID = replyTo
		}
		_, err = bot.Send(video)
		return err
	case channel.AttachmentGIF:
		animation, err := buildTelegramAnimation(target, file)
		if err != nil {
			return err
		}
		animation.Caption = caption
		animation.ParseMode = parseMode
		if replyTo > 0 {
			animation.ReplyToMessageID = replyTo
		}
		_, err = bot.Send(animation)
		return err
	default:
		return fmt.Errorf("unsupported attachment type: %s", att.Logical.Type)
	}
}

// resolveTelegramFile maps a prepared attachment into Telegram's file input model.
func resolveTelegramFile(ctx context.Context, att channel.PreparedAttachment) (tgbotapi.RequestFileData, error) {
	switch att.Kind {
	case channel.PreparedAttachmentNativeRef:
		if strings.TrimSpace(att.NativeRef) == "" {
			return nil, errors.New("telegram native ref is required")
		}
		return tgbotapi.FileID(strings.TrimSpace(att.NativeRef)), nil
	case channel.PreparedAttachmentPublicURL:
		if strings.TrimSpace(att.PublicURL) == "" {
			return nil, errors.New("telegram public url is required")
		}
		return tgbotapi.FileURL(strings.TrimSpace(att.PublicURL)), nil
	case channel.PreparedAttachmentUpload:
		if att.Open == nil {
			return nil, errors.New("telegram upload attachment is not openable")
		}
		reader, err := att.Open(ctx)
		if err != nil {
			return nil, err
		}
		defer func() { _ = reader.Close() }()
		data, err := media.ReadAllWithLimit(reader, media.MaxAssetBytes)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(att.Name)
		if name == "" {
			name = fileNameFromMime(att.Mime, string(att.Logical.Type))
		}
		return tgbotapi.FileBytes{Name: name, Bytes: data}, nil
	default:
		return nil, fmt.Errorf("unsupported telegram attachment kind: %s", att.Kind)
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

func buildTelegramReplyRef(msg *tgbotapi.Message, chatID string) *channel.ReplyRef {
	if msg == nil || msg.ReplyToMessage == nil {
		return nil
	}
	replyTo := msg.ReplyToMessage
	ref := &channel.ReplyRef{
		MessageID: strconv.Itoa(replyTo.MessageID),
		Target:    strings.TrimSpace(chatID),
		Sender:    resolveTelegramDisplayName(replyTo.From),
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
func resolveTelegramDisplayName(user *tgbotapi.User) string {
	if user == nil {
		return ""
	}
	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	username := strings.TrimSpace(user.UserName)
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

// buildTelegramForwardContext returns a text prefix describing the forward origin.
// Handles three cases: forwarded from a user, from a channel, or from a hidden sender.
// Returns an empty string when the message is not forwarded.
func buildTelegramForwardContext(msg *tgbotapi.Message) string {
	if msg == nil {
		return ""
	}
	if msg.ForwardFrom != nil {
		name := resolveTelegramDisplayName(msg.ForwardFrom)
		if name != "" {
			return fmt.Sprintf("[Forwarded from %s]", name)
		}
	}
	if msg.ForwardFromChat != nil {
		title := strings.TrimSpace(msg.ForwardFromChat.Title)
		username := strings.TrimSpace(msg.ForwardFromChat.UserName)
		if title != "" && username != "" {
			return fmt.Sprintf("[Forwarded from %s (@%s)]", title, username)
		}
		if title != "" {
			return fmt.Sprintf("[Forwarded from %s]", title)
		}
		if username != "" {
			return fmt.Sprintf("[Forwarded from @%s]", username)
		}
	}
	senderName := strings.TrimSpace(msg.ForwardSenderName)
	if senderName != "" {
		return fmt.Sprintf("[Forwarded from %s]", senderName)
	}
	return ""
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

func buildTelegramAudio(target string, file tgbotapi.RequestFileData) (tgbotapi.AudioConfig, error) {
	chatID, channelUsername, err := parseTelegramTarget(target)
	if err != nil {
		return tgbotapi.AudioConfig{}, err
	}
	audio := tgbotapi.NewAudio(chatID, file)
	audio.ChannelUsername = channelUsername
	return audio, nil
}

func buildTelegramVoice(target string, file tgbotapi.RequestFileData) (tgbotapi.VoiceConfig, error) {
	chatID, channelUsername, err := parseTelegramTarget(target)
	if err != nil {
		return tgbotapi.VoiceConfig{}, err
	}
	voice := tgbotapi.NewVoice(chatID, file)
	voice.ChannelUsername = channelUsername
	return voice, nil
}

func buildTelegramVideo(target string, file tgbotapi.RequestFileData) (tgbotapi.VideoConfig, error) {
	chatID, channelUsername, err := parseTelegramTarget(target)
	if err != nil {
		return tgbotapi.VideoConfig{}, err
	}
	video := tgbotapi.NewVideo(chatID, file)
	video.ChannelUsername = channelUsername
	return video, nil
}

func buildTelegramAnimation(target string, file tgbotapi.RequestFileData) (tgbotapi.AnimationConfig, error) {
	chatID, channelUsername, err := parseTelegramTarget(target)
	if err != nil {
		return tgbotapi.AnimationConfig{}, err
	}
	animation := tgbotapi.NewAnimation(chatID, file)
	animation.ChannelUsername = channelUsername
	return animation, nil
}

// extractTelegramMentionParts extracts structured mention parts from Telegram message entities.
func extractTelegramMentionParts(msg *tgbotapi.Message) []channel.MessagePart {
	if msg == nil {
		return nil
	}
	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	entities := make([]tgbotapi.MessageEntity, 0, len(msg.Entities)+len(msg.CaptionEntities))
	entities = append(entities, msg.Entities...)
	entities = append(entities, msg.CaptionEntities...)

	var parts []channel.MessagePart
	for _, entity := range entities {
		switch entity.Type {
		case "mention":
			if text != "" && entity.Offset >= 0 && entity.Offset+entity.Length <= len([]rune(text)) {
				runes := []rune(text)
				mentionText := string(runes[entity.Offset : entity.Offset+entity.Length])
				parts = append(parts, channel.MessagePart{
					Type: channel.MessagePartMention,
					Text: mentionText,
				})
			}
		case "text_mention":
			if entity.User != nil {
				name := strings.TrimSpace(entity.User.FirstName + " " + entity.User.LastName)
				if name == "" {
					name = entity.User.UserName
				}
				displayText := "@" + name
				meta := map[string]any{
					"user_id": strconv.FormatInt(entity.User.ID, 10),
				}
				if entity.User.UserName != "" {
					meta["username"] = entity.User.UserName
				}
				parts = append(parts, channel.MessagePart{
					Type:     channel.MessagePartMention,
					Text:     displayText,
					Metadata: meta,
				})
			}
		}
	}
	return parts
}

func isTelegramBotMentioned(msg *tgbotapi.Message, botUsername string) bool {
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
	entities := make([]tgbotapi.MessageEntity, 0, len(msg.Entities)+len(msg.CaptionEntities))
	entities = append(entities, msg.Entities...)
	entities = append(entities, msg.CaptionEntities...)
	for _, entity := range entities {
		if entity.Type == "text_mention" && entity.User != nil && entity.User.IsBot {
			if normalizedBot != "" && strings.EqualFold(entity.User.UserName, normalizedBot) {
				return true
			}
		}
	}
	return false
}

func (a *TelegramAdapter) collectTelegramAttachments(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) []channel.Attachment {
	if msg == nil {
		return nil
	}
	attachments := make([]channel.Attachment, 0, 1)
	if len(msg.Photo) > 0 {
		photo := pickTelegramPhoto(msg.Photo)
		att := a.buildTelegramAttachment(bot, channel.AttachmentImage, photo.FileID, "", "", int64(photo.FileSize))
		att.Width = photo.Width
		att.Height = photo.Height
		attachments = append(attachments, att)
	}
	if msg.Document != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentFile, msg.Document.FileID, msg.Document.FileName, msg.Document.MimeType, int64(msg.Document.FileSize))
		attachments = append(attachments, att)
	}
	if msg.Audio != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentAudio, msg.Audio.FileID, msg.Audio.FileName, msg.Audio.MimeType, int64(msg.Audio.FileSize))
		att.DurationMs = int64(msg.Audio.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Voice != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentVoice, msg.Voice.FileID, "", msg.Voice.MimeType, int64(msg.Voice.FileSize))
		att.DurationMs = int64(msg.Voice.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Video != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentVideo, msg.Video.FileID, msg.Video.FileName, msg.Video.MimeType, int64(msg.Video.FileSize))
		att.Width = msg.Video.Width
		att.Height = msg.Video.Height
		att.DurationMs = int64(msg.Video.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Animation != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentGIF, msg.Animation.FileID, msg.Animation.FileName, msg.Animation.MimeType, int64(msg.Animation.FileSize))
		att.Width = msg.Animation.Width
		att.Height = msg.Animation.Height
		att.DurationMs = int64(msg.Animation.Duration) * 1000
		attachments = append(attachments, att)
	}
	if msg.Sticker != nil {
		att := a.buildTelegramAttachment(bot, channel.AttachmentImage, msg.Sticker.FileID, "", "", int64(msg.Sticker.FileSize))
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

func (a *TelegramAdapter) buildTelegramAttachment(bot *tgbotapi.BotAPI, attType channel.AttachmentType, fileID, name, mime string, size int64) channel.Attachment {
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
	identity := map[string]any{
		"user_id":  strconv.FormatInt(bot.Self.ID, 10),
		"username": bot.Self.UserName,
	}
	name := strings.TrimSpace(bot.Self.FirstName + " " + bot.Self.LastName)
	if name != "" {
		identity["name"] = name
	}
	avatarURL := a.resolveUserAvatarURL(bot, bot.Self.ID)
	if avatarURL != "" {
		identity["avatar_url"] = avatarURL
	}
	return identity, strconv.FormatInt(bot.Self.ID, 10), nil
}

// resolveUserAvatarURL fetches the first profile photo for a Telegram user and returns a direct URL.
func (a *TelegramAdapter) resolveUserAvatarURL(bot *tgbotapi.BotAPI, userID int64) string {
	photos, err := bot.GetUserProfilePhotos(tgbotapi.UserProfilePhotosConfig{
		UserID: userID,
		Limit:  1,
	})
	if err != nil || photos.TotalCount == 0 || len(photos.Photos) == 0 {
		return ""
	}
	best := pickTelegramPhoto(photos.Photos[0])
	if best.FileID == "" {
		return ""
	}
	url, err := a.getFileDirectURL(bot, best.FileID)
	if err != nil {
		return ""
	}
	return url
}

func pickTelegramPhoto(items []tgbotapi.PhotoSize) tgbotapi.PhotoSize {
	if len(items) == 0 {
		return tgbotapi.PhotoSize{}
	}
	best := items[0]
	for _, item := range items[1:] {
		if item.FileSize > best.FileSize {
			best = item
			continue
		}
		if item.Width*item.Height > best.Width*best.Height {
			best = item
		}
	}
	return best
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

func sendTelegramTyping(bot *tgbotapi.BotAPI, chatID string) error {
	chatIDInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	action := tgbotapi.NewChatAction(chatIDInt, tgbotapi.ChatTyping)
	_, err = bot.Request(action)
	return err
}

func setTelegramReaction(bot *tgbotapi.BotAPI, chatID, messageID, emoji string) error {
	params := tgbotapi.Params{}
	params.AddNonEmpty("chat_id", chatID)
	params.AddNonEmpty("message_id", messageID)
	params.AddNonEmpty("reaction", fmt.Sprintf(`[{"type":"emoji","emoji":"%s"}]`, emoji))
	_, err := bot.MakeRequest("setMessageReaction", params)
	return err
}

func clearTelegramReaction(bot *tgbotapi.BotAPI, chatID, messageID string) error {
	params := tgbotapi.Params{}
	params.AddNonEmpty("chat_id", chatID)
	params.AddNonEmpty("message_id", messageID)
	params.AddNonEmpty("reaction", "[]")
	_, err := bot.MakeRequest("setMessageReaction", params)
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
