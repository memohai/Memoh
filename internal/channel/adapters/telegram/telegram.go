package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
	"github.com/memohai/memoh/internal/channel/adapters/common"
)

type TelegramAdapter struct {
	logger *slog.Logger
}

func NewTelegramAdapter(log *slog.Logger) *TelegramAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &TelegramAdapter{
		logger: log.With(slog.String("adapter", "telegram")),
	}
}

func (a *TelegramAdapter) Type() channel.ChannelType {
	return Type
}

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
				if a.logger != nil {
					a.logger.Info("stop", slog.String("config_id", cfg.ID))
				}
				bot.StopReceivingUpdates()
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
				externalID, displayName, attrs := resolveTelegramSender(update.Message)
				chatID := ""
				chatType := ""
				chatName := ""
				if update.Message.Chat != nil {
					chatID = strconv.FormatInt(update.Message.Chat.ID, 10)
					chatType = strings.TrimSpace(update.Message.Chat.Type)
					chatName = strings.TrimSpace(update.Message.Chat.Title)
				}
				replyRef := buildTelegramReplyRef(update.Message, chatID)
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
						ExternalID:  externalID,
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

	stop := func(context.Context) error {
		if a.logger != nil {
			a.logger.Info("stop", slog.String("config_id", cfg.ID))
		}
		cancel()
		bot.StopReceivingUpdates()
		return nil
	}
	return channel.NewConnection(cfg, stop), nil
}

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
	bot, err := tgbotapi.NewBotAPI(telegramCfg.BotToken)
	if err != nil {
		if a.logger != nil {
			a.logger.Error("create bot failed", slog.String("config_id", cfg.ID), slog.Any("error", err))
		}
		return err
	}
	if msg.Message.IsEmpty() {
		return fmt.Errorf("message is required")
	}
	text := strings.TrimSpace(msg.Message.PlainText())
	parseMode := resolveTelegramParseMode(msg.Message.Format)
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
			displayName = strings.TrimSpace(strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName))
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
	if strings.HasPrefix(target, "@") {
		message := tgbotapi.NewMessageToChannel(target, text)
		message.ParseMode = parseMode
		if replyTo > 0 {
			message.ReplyToMessageID = replyTo
		}
		_, err := bot.Send(message)
		return err
	}
	chatID, err := strconv.ParseInt(target, 10, 64)
	if err != nil {
		return fmt.Errorf("telegram target must be @username or chat_id")
	}
	message := tgbotapi.NewMessage(chatID, text)
	message.ParseMode = parseMode
	if replyTo > 0 {
		message.ReplyToMessageID = replyTo
	}
	_, err = bot.Send(message)
	return err
}

func sendTelegramAttachment(bot *tgbotapi.BotAPI, target string, att channel.Attachment, caption string, replyTo int, parseMode string) error {
	if strings.TrimSpace(att.URL) == "" {
		return fmt.Errorf("attachment url is required")
	}
	if strings.TrimSpace(caption) == "" && strings.TrimSpace(att.Caption) != "" {
		caption = strings.TrimSpace(att.Caption)
	}
	file := tgbotapi.FileURL(att.URL)
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
		chatID, err := strconv.ParseInt(target, 10, 64)
		if err != nil && !isChannel {
			return fmt.Errorf("telegram target must be @username or chat_id")
		}
		document := tgbotapi.NewDocument(chatID, file)
		if isChannel {
			document.ChatID = 0
			document.ChannelUsername = target
		}
		document.Caption = caption
		document.ParseMode = parseMode
		if replyTo > 0 {
			document.ReplyToMessageID = replyTo
		}
		_, err = bot.Send(document)
		return err
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
		Type:     attType,
		URL:      strings.TrimSpace(url),
		Name:     strings.TrimSpace(name),
		Mime:     strings.TrimSpace(mime),
		Size:     size,
		Metadata: map[string]any{},
	}
	if fileID != "" {
		att.Metadata["file_id"] = fileID
	}
	return att
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
