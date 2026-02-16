package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/common"
	"github.com/memohai/memoh/internal/media"
)

const telegramMaxMessageLength = 4096

// TelegramAdapter implements the channel.Adapter, channel.Sender, and channel.Receiver interfaces for Telegram.
type TelegramAdapter struct {
	logger *slog.Logger
	mu     sync.RWMutex
	bots   map[string]*tgbotapi.BotAPI // keyed by bot token
}

// NewTelegramAdapter creates a TelegramAdapter with the given logger.
func NewTelegramAdapter(log *slog.Logger) *TelegramAdapter {
	if log == nil {
		log = slog.Default()
	}
	adapter := &TelegramAdapter{
		logger: log.With(slog.String("adapter", "telegram")),
		bots:   make(map[string]*tgbotapi.BotAPI),
	}
	_ = tgbotapi.SetLogger(&slogBotLogger{log: adapter.logger})
	return adapter
}

var getOrCreateBotForTest func(a *TelegramAdapter, token, configID string) (*tgbotapi.BotAPI, error)

func (a *TelegramAdapter) getOrCreateBot(token, configID string) (*tgbotapi.BotAPI, error) {
	if getOrCreateBotForTest != nil {
		return getOrCreateBotForTest(a, token, configID)
	}
	a.mu.RLock()
	bot, ok := a.bots[token]
	a.mu.RUnlock()
	if ok {
		return bot, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if bot, ok := a.bots[token]; ok {
		return bot, nil
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("create bot failed", slog.String("config_id", configID), slog.Any("error", err))
		}
		return nil, err
	}
	a.bots[token] = bot
	return bot, nil
}

// Type returns the Telegram channel type.
func (a *TelegramAdapter) Type() channel.ChannelType {
	return Type
}

// Descriptor returns the Telegram channel metadata.
func (a *TelegramAdapter) Descriptor() channel.Descriptor {
	return channel.Descriptor{
		Type:        Type,
		DisplayName: "Telegram",
		Capabilities: channel.ChannelCapabilities{
			Text:           true,
			Markdown:       true,
			Reply:          true,
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
					Title:    "Bot Token",
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
func (a *TelegramAdapter) NormalizeConfig(raw map[string]any) (map[string]any, error) {
	return normalizeConfig(raw)
}

// NormalizeUserConfig validates and normalizes a Telegram user-binding configuration map.
func (a *TelegramAdapter) NormalizeUserConfig(raw map[string]any) (map[string]any, error) {
	return normalizeUserConfig(raw)
}

// NormalizeTarget normalizes a Telegram delivery target string.
func (a *TelegramAdapter) NormalizeTarget(raw string) string {
	return normalizeTarget(raw)
}

// ResolveTarget derives a delivery target from a Telegram user-binding configuration.
func (a *TelegramAdapter) ResolveTarget(userConfig map[string]any) (string, error) {
	return resolveTarget(userConfig)
}

// MatchBinding reports whether a Telegram user binding matches the given criteria.
func (a *TelegramAdapter) MatchBinding(config map[string]any, criteria channel.BindingCriteria) bool {
	return matchBinding(config, criteria)
}

// BuildUserConfig constructs a Telegram user-binding config from an Identity.
func (a *TelegramAdapter) BuildUserConfig(identity channel.Identity) map[string]any {
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
	bot, err := tgbotapi.NewBotAPI(telegramCfg.BotToken)
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

	go func() {
		for {
			select {
			case <-connCtx.Done():
				return
			case update, ok := <-updates:
				if !ok {
					if a.logger != nil {
						a.logger.Info("updates channel closed", slog.String("config_id", cfg.ID))
					}
					return
				}
				if update.Message == nil {
					continue
				}
				text := strings.TrimSpace(update.Message.Text)
				caption := strings.TrimSpace(update.Message.Caption)
				if text == "" && caption != "" {
					text = caption
				}
				attachments := a.collectTelegramAttachments(bot, update.Message)
				if text == "" && len(attachments) == 0 {
					continue
				}
				subjectID, displayName, attrs := resolveTelegramSender(update.Message)
				chatID := ""
				chatType := ""
				chatName := ""
				if update.Message.Chat != nil {
					chatID = strconv.FormatInt(update.Message.Chat.ID, 10)
					chatType = strings.TrimSpace(update.Message.Chat.Type)
					chatName = strings.TrimSpace(update.Message.Chat.Title)
				}
				replyRef := buildTelegramReplyRef(update.Message, chatID)
				isReplyToBot := update.Message.ReplyToMessage != nil &&
					update.Message.ReplyToMessage.From != nil &&
					update.Message.ReplyToMessage.From.ID == bot.Self.ID
				isMentioned := isTelegramBotMentioned(update.Message, bot.Self.UserName)
				msg := channel.InboundMessage{
					Channel: Type,
					Message: channel.Message{
						ID:          strconv.Itoa(update.Message.MessageID),
						Format:      channel.MessageFormatPlain,
						Text:        text,
						Attachments: attachments,
						Reply:       replyRef,
					},
					BotID:       cfg.BotID,
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
					ReceivedAt: time.Unix(int64(update.Message.Date), 0).UTC(),
					Source:     "telegram",
					Metadata: map[string]any{
						"is_mentioned":    isMentioned,
						"is_reply_to_bot": isReplyToBot,
					},
				}
				if a.logger != nil {
					a.logger.Info(
						"inbound received",
						slog.String("config_id", cfg.ID),
						slog.String("chat_type", msg.Conversation.Type),
						slog.String("chat_id", msg.Conversation.ID),
						slog.String("user_id", attrs["user_id"]),
						slog.String("username", attrs["username"]),
						slog.String("text", common.SummarizeText(text)),
					)
				}
				go func() {
					if err := handler(connCtx, cfg, msg); err != nil && a.logger != nil {
						a.logger.Error("handle inbound failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
					}
				}()
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

// Send delivers an outbound message to Telegram, handling text, attachments, and replies.
func (a *TelegramAdapter) Send(ctx context.Context, cfg channel.ChannelConfig, msg channel.OutboundMessage) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("decode config failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return err
	}
	to := strings.TrimSpace(msg.Target)
	if to == "" {
		return fmt.Errorf("telegram target is required")
	}
	bot, err := a.getOrCreateBot(telegramCfg.BotToken, cfg.ID)
	if err != nil {
		return err
	}
	if msg.Message.IsEmpty() {
		return fmt.Errorf("message is required")
	}
	text := strings.TrimSpace(msg.Message.PlainText())
	text, parseMode := formatTelegramOutput(text, msg.Message.Format)
	replyTo := parseReplyToMessageID(msg.Message.Reply)
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
			if err := sendTelegramAttachment(bot, to, att, caption, applyReply, parseMode); err != nil {
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
	return sendTelegramText(bot, to, text, replyTo, parseMode)
}

// OpenStream opens a Telegram streaming session.
// The adapter sends one message then edits it in place as deltas arrive (editMessageText),
// avoiding one message per delta and rate limits.
func (a *TelegramAdapter) OpenStream(ctx context.Context, cfg channel.ChannelConfig, target string, opts channel.StreamOptions) (channel.OutboundStream, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, fmt.Errorf("telegram target is required")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return &telegramOutboundStream{
		adapter:   a,
		cfg:       cfg,
		target:    target,
		reply:     opts.Reply,
		parseMode: "",
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
		displayName := strings.TrimSpace(msg.From.UserName)
		if displayName == "" {
			displayName = strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
		}
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

func sendTelegramText(bot *tgbotapi.BotAPI, target string, text string, replyTo int, parseMode string) error {
	_, _, err := sendTelegramTextReturnMessage(bot, target, text, replyTo, parseMode)
	return err
}

// sendTelegramTextReturnMessage sends a text message and returns the chat ID and message ID for later editing.
func sendTelegramTextReturnMessage(bot *tgbotapi.BotAPI, target string, text string, replyTo int, parseMode string) (chatID int64, messageID int, err error) {
	text = truncateTelegramText(sanitizeTelegramText(text))
	var sent tgbotapi.Message
	if strings.HasPrefix(target, "@") {
		message := tgbotapi.NewMessageToChannel(target, text)
		message.ParseMode = parseMode
		if replyTo > 0 {
			message.ReplyToMessageID = replyTo
		}
		sent, err = bot.Send(message)
		if err != nil {
			return 0, 0, err
		}
	} else {
		chatID, err = strconv.ParseInt(target, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("telegram target must be @username or chat_id")
		}
		message := tgbotapi.NewMessage(chatID, text)
		message.ParseMode = parseMode
		if replyTo > 0 {
			message.ReplyToMessageID = replyTo
		}
		sent, err = bot.Send(message)
		if err != nil {
			return 0, 0, err
		}
	}
	if sent.Chat != nil {
		chatID = sent.Chat.ID
	}
	messageID = sent.MessageID
	return chatID, messageID, nil
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

func sendTelegramAttachment(bot *tgbotapi.BotAPI, target string, att channel.Attachment, caption string, replyTo int, parseMode string) error {
	urlRef := strings.TrimSpace(att.URL)
	keyRef := strings.TrimSpace(att.PlatformKey)
	sourcePlatform := strings.TrimSpace(att.SourcePlatform)
	if urlRef == "" && keyRef == "" {
		return fmt.Errorf("attachment reference is required")
	}
	if strings.TrimSpace(caption) == "" && strings.TrimSpace(att.Caption) != "" {
		caption = strings.TrimSpace(att.Caption)
	}
	file := tgbotapi.RequestFileData(tgbotapi.FileURL(urlRef))
	if keyRef != "" && (sourcePlatform == "" || strings.EqualFold(sourcePlatform, Type.String())) {
		file = tgbotapi.FileID(keyRef)
	}
	isChannel := strings.HasPrefix(target, "@")
	switch att.Type {
	case channel.AttachmentImage:
		var photo tgbotapi.PhotoConfig
		if isChannel {
			photo = tgbotapi.NewPhotoToChannel(target, file)
		} else {
			chatID, err := strconv.ParseInt(target, 10, 64)
			if err != nil {
				return fmt.Errorf("telegram target must be @username or chat_id")
			}
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
					BaseChat: tgbotapi.BaseChat{ChannelUsername: target},
					File:     file,
				},
			}
		} else {
			chatID, err := strconv.ParseInt(target, 10, 64)
			if err != nil {
				return fmt.Errorf("telegram target must be @username or chat_id")
			}
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
		return fmt.Errorf("unsupported attachment type: %s", att.Type)
	}
}

func buildTelegramReplyRef(msg *tgbotapi.Message, chatID string) *channel.ReplyRef {
	if msg == nil || msg.ReplyToMessage == nil {
		return nil
	}
	return &channel.ReplyRef{
		MessageID: strconv.Itoa(msg.ReplyToMessage.MessageID),
		Target:    strings.TrimSpace(chatID),
	}
}

func buildTelegramAudio(target string, file tgbotapi.RequestFileData) (tgbotapi.AudioConfig, error) {
	if strings.HasPrefix(target, "@") {
		audio := tgbotapi.NewAudio(0, file)
		audio.ChannelUsername = target
		return audio, nil
	}
	chatID, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return tgbotapi.AudioConfig{}, fmt.Errorf("telegram target must be @username or chat_id")
	}
	return tgbotapi.NewAudio(chatID, file), nil
}

func buildTelegramVoice(target string, file tgbotapi.RequestFileData) (tgbotapi.VoiceConfig, error) {
	if strings.HasPrefix(target, "@") {
		voice := tgbotapi.NewVoice(0, file)
		voice.ChannelUsername = target
		return voice, nil
	}
	chatID, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return tgbotapi.VoiceConfig{}, fmt.Errorf("telegram target must be @username or chat_id")
	}
	return tgbotapi.NewVoice(chatID, file), nil
}

func buildTelegramVideo(target string, file tgbotapi.RequestFileData) (tgbotapi.VideoConfig, error) {
	if strings.HasPrefix(target, "@") {
		video := tgbotapi.NewVideo(0, file)
		video.ChannelUsername = target
		return video, nil
	}
	chatID, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return tgbotapi.VideoConfig{}, fmt.Errorf("telegram target must be @username or chat_id")
	}
	return tgbotapi.NewVideo(chatID, file), nil
}

func buildTelegramAnimation(target string, file tgbotapi.RequestFileData) (tgbotapi.AnimationConfig, error) {
	if strings.HasPrefix(target, "@") {
		animation := tgbotapi.NewAnimation(0, file)
		animation.ChannelUsername = target
		return animation, nil
	}
	chatID, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return tgbotapi.AnimationConfig{}, fmt.Errorf("telegram target must be @username or chat_id")
	}
	return tgbotapi.NewAnimation(chatID, file), nil
}

func resolveTelegramParseMode(format channel.MessageFormat) string {
	switch format {
	case channel.MessageFormatMarkdown:
		return tgbotapi.ModeMarkdown
	default:
		return ""
	}
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
			return true
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
		value, err := bot.GetFileDirectURL(fileID)
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
	return att
}

// ResolveAttachment resolves a Telegram attachment reference to a byte stream.
// It supports platform_key-based references and URL fallback.
func (a *TelegramAdapter) ResolveAttachment(ctx context.Context, cfg channel.ChannelConfig, attachment channel.Attachment) (channel.AttachmentPayload, error) {
	fileID := strings.TrimSpace(attachment.PlatformKey)
	if fileID == "" && strings.TrimSpace(attachment.URL) == "" {
		return channel.AttachmentPayload{}, fmt.Errorf("telegram attachment requires platform_key or url")
	}
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return channel.AttachmentPayload{}, err
	}
	bot, err := a.getOrCreateBot(telegramCfg.BotToken, cfg.ID)
	if err != nil {
		return channel.AttachmentPayload{}, err
	}
	downloadURL := strings.TrimSpace(attachment.URL)
	if downloadURL == "" {
		downloadURL, err = bot.GetFileDirectURL(fileID)
		if err != nil {
			return channel.AttachmentPayload{}, fmt.Errorf("resolve telegram file url: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return channel.AttachmentPayload{}, fmt.Errorf("build download request: %w", err)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
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
		if idx := strings.Index(mime, ";"); idx >= 0 {
			mime = strings.TrimSpace(mime[:idx])
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
	if len(text) <= telegramMaxMessageLength {
		return text
	}
	const suffix = "..."
	limit := telegramMaxMessageLength - len(suffix)
	// Walk backwards to a rune boundary.
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit] + suffix
}

// ProcessingStarted sends a "typing" chat action to indicate processing.
func (a *TelegramAdapter) ProcessingStarted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo) (channel.ProcessingStatusHandle, error) {
	chatID := strings.TrimSpace(info.ReplyTarget)
	if chatID == "" {
		return channel.ProcessingStatusHandle{}, nil
	}
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return channel.ProcessingStatusHandle{}, err
	}
	bot, err := a.getOrCreateBot(telegramCfg.BotToken, cfg.ID)
	if err != nil {
		return channel.ProcessingStatusHandle{}, err
	}
	if err := sendTelegramTyping(bot, chatID); err != nil && a.logger != nil {
		a.logger.Warn("send typing action failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
	}
	return channel.ProcessingStatusHandle{}, nil
}

// ProcessingCompleted is a no-op for Telegram (typing indicator clears automatically).
func (a *TelegramAdapter) ProcessingCompleted(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle) error {
	return nil
}

// ProcessingFailed is a no-op for Telegram (typing indicator clears automatically).
func (a *TelegramAdapter) ProcessingFailed(ctx context.Context, cfg channel.ChannelConfig, msg channel.InboundMessage, info channel.ProcessingStatusInfo, handle channel.ProcessingStatusHandle, cause error) error {
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
func (a *TelegramAdapter) React(ctx context.Context, cfg channel.ChannelConfig, target string, messageID string, emoji string) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	bot, err := a.getOrCreateBot(telegramCfg.BotToken, cfg.ID)
	if err != nil {
		return err
	}
	return setTelegramReaction(bot, target, messageID, emoji)
}

// Unreact removes the bot's reaction from a message (implements channel.Reactor).
// The emoji parameter is ignored; Telegram clears all bot reactions at once.
func (a *TelegramAdapter) Unreact(ctx context.Context, cfg channel.ChannelConfig, target string, messageID string, _ string) error {
	telegramCfg, err := parseConfig(cfg.Credentials)
	if err != nil {
		return err
	}
	bot, err := a.getOrCreateBot(telegramCfg.BotToken, cfg.ID)
	if err != nil {
		return err
	}
	return clearTelegramReaction(bot, target, messageID)
}
