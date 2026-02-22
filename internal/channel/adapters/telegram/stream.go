package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

const telegramStreamEditThrottle = 5000 * time.Millisecond
const telegramStreamToolHintText = "Calling tools..."
const telegramStreamPendingSuffix = "\n……"

var testEditFunc func(bot *tgbotapi.BotAPI, chatID int64, msgID int, text string, parseMode string) error

type telegramOutboundStream struct {
	adapter      *TelegramAdapter
	cfg          channel.ChannelConfig
	target       string
	reply        *channel.ReplyRef
	parseMode    string
	closed       atomic.Bool
	mu           sync.Mutex
	buf          strings.Builder
	streamChatID int64
	streamMsgID  int
	lastEdited   string
	lastEditedAt time.Time
}

func (s *telegramOutboundStream) getBot(ctx context.Context) (bot *tgbotapi.BotAPI, err error) {
	telegramCfg, err := parseConfig(s.cfg.Credentials)
	if err != nil {
		return nil, err
	}
	bot, err = s.adapter.getOrCreateBot(telegramCfg.BotToken, s.cfg.ID)
	if err != nil {
		return nil, err
	}
	return bot, nil
}

func (s *telegramOutboundStream) getBotAndReply(ctx context.Context) (bot *tgbotapi.BotAPI, replyTo int, err error) {
	bot, err = s.getBot(ctx)
	if err != nil {
		return nil, 0, err
	}
	replyTo = parseReplyToMessageID(s.reply)
	return bot, replyTo, nil
}

func (s *telegramOutboundStream) refreshTypingAction(ctx context.Context) error {
	// When ensureStreamMessage is called, always means that the message has not been completely generated
	// so always refresh the "typing" action to improve the user experience
	bot, err := s.getBot(ctx)
	if err != nil {
		return err
	}
	action := tgbotapi.NewChatAction(s.streamChatID, tgbotapi.ChatTyping)
	_, err = bot.Request(action)
	return err
}

func (s *telegramOutboundStream) ensureStreamMessage(ctx context.Context, text string) error {
	s.mu.Lock()
	go s.refreshTypingAction(ctx)
	if s.streamMsgID != 0 {
		s.mu.Unlock()
		return nil
	}
	bot, replyTo, err := s.getBotAndReply(ctx)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if strings.TrimSpace(text) == "" {
		text = "..."
	} else {
		text = strings.TrimSpace(text) + telegramStreamPendingSuffix
	}
	chatID, msgID, err := sendTelegramTextReturnMessage(bot, s.target, text, replyTo, s.parseMode)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	s.streamChatID = chatID
	s.streamMsgID = msgID
	s.lastEdited = text
	s.lastEditedAt = time.Now()
	s.mu.Unlock()
	return nil
}

func normalizeStreamComparableText(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.TrimSuffix(normalized, telegramStreamPendingSuffix)
	return strings.TrimSpace(normalized)
}

func (s *telegramOutboundStream) editStreamMessage(ctx context.Context, text string) error {
	s.mu.Lock()
	chatID := s.streamChatID
	msgID := s.streamMsgID
	lastEdited := s.lastEdited
	lastEditedAt := s.lastEditedAt
	s.mu.Unlock()
	if msgID == 0 {
		return nil
	}
	if normalizeStreamComparableText(text) == normalizeStreamComparableText(lastEdited) {
		return nil
	}
	text = strings.TrimSpace(text) + telegramStreamPendingSuffix
	if time.Since(lastEditedAt) < telegramStreamEditThrottle {
		return nil
	}
	bot, _, err := s.getBotAndReply(ctx)
	if err != nil {
		return err
	}
	editErr := error(nil)
	if testEditFunc != nil {
		editErr = testEditFunc(bot, chatID, msgID, text, s.parseMode)
	} else {
		editErr = editTelegramMessageText(bot, chatID, msgID, text, s.parseMode)
	}
	if editErr != nil {
		if isTelegramTooManyRequests(editErr) {
			d := getTelegramRetryAfter(editErr)
			if d <= 0 {
				d = telegramStreamEditThrottle
			}
			s.mu.Lock()
			s.lastEditedAt = time.Now().Add(d)
			s.mu.Unlock()
			return nil
		}
		return editErr
	}
	s.mu.Lock()
	s.lastEdited = text
	s.lastEditedAt = time.Now()
	s.mu.Unlock()
	return nil
}

const telegramFinalEditMaxRetries = 3

// editStreamMessageFinal edits the streamed message for the final content.
// Retries on 429 with server-provided backoff to ensure delivery.
func (s *telegramOutboundStream) editStreamMessageFinal(ctx context.Context, text string) error {
	s.mu.Lock()
	chatID := s.streamChatID
	msgID := s.streamMsgID
	lastEdited := s.lastEdited
	s.mu.Unlock()
	if msgID == 0 {
		return nil
	}
	if strings.TrimSpace(text) == lastEdited {
		return nil
	}
	bot, _, err := s.getBotAndReply(ctx)
	if err != nil {
		return err
	}
	for attempt := range telegramFinalEditMaxRetries {
		editErr := error(nil)
		if testEditFunc != nil {
			editErr = testEditFunc(bot, chatID, msgID, text, s.parseMode)
		} else {
			editErr = editTelegramMessageText(bot, chatID, msgID, text, s.parseMode)
		}
		if editErr == nil {
			s.mu.Lock()
			s.lastEdited = text
			s.lastEditedAt = time.Now()
			s.mu.Unlock()
			return nil
		}
		if !isTelegramTooManyRequests(editErr) {
			return editErr
		}
		d := getTelegramRetryAfter(editErr)
		if d <= 0 {
			d = time.Duration(attempt+1) * time.Second
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	return nil
}

func (s *telegramOutboundStream) Push(ctx context.Context, event channel.StreamEvent) error {
	if s == nil || s.adapter == nil {
		return fmt.Errorf("telegram stream not configured")
	}
	if s.closed.Load() {
		return fmt.Errorf("telegram stream is closed")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	switch event.Type {
	case channel.StreamEventStatus:
		return nil
	case channel.StreamEventToolCallStart:
		s.mu.Lock()
		bufText := strings.TrimSpace(s.buf.String())
		hasMsg := s.streamMsgID != 0
		s.mu.Unlock()
		if hasMsg && bufText != "" {
			_ = s.editStreamMessageFinal(ctx, bufText)
		}
		s.mu.Lock()
		s.streamMsgID = 0
		s.streamChatID = 0
		s.lastEdited = ""
		s.lastEditedAt = time.Time{}
		s.buf.Reset()
		s.mu.Unlock()
		return nil
	case channel.StreamEventToolCallEnd:
		s.mu.Lock()
		s.streamMsgID = 0
		s.streamChatID = 0
		s.lastEdited = ""
		s.lastEditedAt = time.Time{}
		s.buf.Reset()
		s.mu.Unlock()
		return nil
	case channel.StreamEventAttachment:
		if len(event.Attachments) == 0 {
			return nil
		}
		bot, replyTo, err := s.getBotAndReply(ctx)
		if err != nil {
			return err
		}
		for _, att := range event.Attachments {
			if sendErr := sendTelegramAttachmentWithAssets(ctx, bot, s.target, att, "", replyTo, "", s.adapter.assets); sendErr != nil {
				slog.Warn("telegram: stream attachment send failed",
					slog.String("config_id", s.cfg.ID),
					slog.String("type", string(att.Type)),
					slog.Any("error", sendErr),
				)
			}
		}
		return nil
	case channel.StreamEventPhaseStart, channel.StreamEventPhaseEnd:
		return nil
	case channel.StreamEventProcessingFailed, channel.StreamEventAgentStart, channel.StreamEventAgentEnd, channel.StreamEventProcessingStarted, channel.StreamEventProcessingCompleted:
		return nil
	case channel.StreamEventDelta:
		if event.Delta == "" || event.Phase == channel.StreamPhaseReasoning {
			return nil
		}
		s.mu.Lock()
		s.buf.WriteString(event.Delta)
		content := s.buf.String()
		s.mu.Unlock()
		if err := s.ensureStreamMessage(ctx, content); err != nil {
			return err
		}
		return s.editStreamMessage(ctx, content)
	case channel.StreamEventFinal:
		if event.Final == nil || event.Final.Message.IsEmpty() {
			s.mu.Lock()
			finalText := strings.TrimSpace(s.buf.String())
			s.mu.Unlock()
			if finalText != "" {
				if err := s.ensureStreamMessage(ctx, finalText); err != nil {
					slog.Warn("telegram: ensure stream message failed", slog.Any("error", err))
				}
				if err := s.editStreamMessageFinal(ctx, finalText); err != nil {
					slog.Warn("telegram: edit stream message failed", slog.Any("error", err))
				}
			}
			return nil
		}
		msg := event.Final.Message
		s.mu.Lock()
		bufText := strings.TrimSpace(s.buf.String())
		s.mu.Unlock()
		finalText := bufText
		if finalText == "" {
			finalText = strings.TrimSpace(msg.PlainText())
		}
		// Convert markdown to Telegram HTML for the final message.
		formatted, pm := formatTelegramOutput(finalText, msg.Format)
		if pm != "" {
			s.mu.Lock()
			s.parseMode = pm
			s.mu.Unlock()
			finalText = formatted
		}
		if err := s.ensureStreamMessage(ctx, finalText); err != nil {
			return err
		}
		if err := s.editStreamMessageFinal(ctx, finalText); err != nil {
			return err
		}
		if len(msg.Attachments) > 0 {
			replyTo := parseReplyToMessageID(s.reply)
			telegramCfg, err := parseConfig(s.cfg.Credentials)
			if err != nil {
				return err
			}
			bot, err := s.adapter.getOrCreateBot(telegramCfg.BotToken, s.cfg.ID)
			if err != nil {
				return err
			}
			parseMode := resolveTelegramParseMode(msg.Format)
			for i, att := range msg.Attachments {
				to := replyTo
				if i > 0 {
					to = 0
				}
				if err := sendTelegramAttachmentWithAssets(ctx, bot, s.target, att, "", to, parseMode, s.adapter.assets); err != nil && s.adapter.logger != nil {
					s.adapter.logger.Error("stream final attachment failed", slog.String("config_id", s.cfg.ID), slog.Any("error", err))
				}
			}
		}
		return nil
	case channel.StreamEventError:
		errText := strings.TrimSpace(event.Error)
		if errText == "" {
			return nil
		}
		display := "Error: " + errText
		if err := s.ensureStreamMessage(ctx, display); err != nil {
			return err
		}
		return s.editStreamMessage(ctx, display)
	default:
		return nil
	}
}

func (s *telegramOutboundStream) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	s.closed.Store(true)
	return nil
}
