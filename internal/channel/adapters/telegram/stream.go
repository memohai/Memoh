package telegram

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/memohai/memoh/internal/channel"
)

const (
	telegramStreamEditThrottle  = 5000 * time.Millisecond
	telegramDraftThrottle       = 300 * time.Millisecond
	telegramStreamToolHintText  = "Calling tools..."
	telegramStreamPendingSuffix = "\n……"
)

var testEditFunc func(bot *tgbotapi.BotAPI, chatID int64, msgID int, text string, parseMode string) error

type telegramOutboundStream struct {
	adapter       *TelegramAdapter
	cfg           channel.ChannelConfig
	target        string
	reply         *channel.ReplyRef
	parseMode     string
	isPrivateChat bool
	draftID       int
	closed        atomic.Bool
	mu            sync.Mutex
	buf           strings.Builder
	streamChatID  int64
	streamMsgID   int
	lastEdited    string
	lastEditedAt  time.Time
	toolMessages  map[string]telegramToolCallMessage
}

// telegramToolCallMessage tracks the message posted for a tool call's
// "running" state so the matching tool_call_end event can edit the same
// message in-place to show the "completed" / "failed" state.
type telegramToolCallMessage struct {
	chatID     int64
	msgID      int
	hasActions bool
}

func (s *telegramOutboundStream) getBot(_ context.Context) (bot *tgbotapi.BotAPI, err error) {
	telegramCfg, err := parseConfig(s.cfg.Credentials)
	if err != nil {
		return nil, err
	}
	bot, err = s.adapter.getOrCreateBot(telegramCfg, s.cfg.ID)
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
	if err := s.adapter.waitStreamLimit(ctx); err != nil {
		return err
	}
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
	go func() {
		if err := s.refreshTypingAction(ctx); err != nil {
			if s.adapter != nil && s.adapter.logger != nil {
				s.adapter.logger.Debug("refresh typing action failed", slog.Any("error", err))
			}
		}
	}()
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
	if err := s.adapter.waitStreamLimit(ctx); err != nil {
		return err
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

const telegramFinalEditMaxRetries = 5

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
	var lastEditErr error
	for attempt := range telegramFinalEditMaxRetries {
		if err := s.adapter.waitStreamLimit(ctx); err != nil {
			return err
		}
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
		lastEditErr = editErr
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
	return fmt.Errorf("telegram: final edit failed after %d retries: %w", telegramFinalEditMaxRetries, lastEditErr)
}

// sendDraft sends a partial message via sendMessageDraft with throttling.
// Only used for private chats.
func (s *telegramOutboundStream) sendDraft(ctx context.Context, text string) error {
	s.mu.Lock()
	lastEditedAt := s.lastEditedAt
	s.mu.Unlock()

	if time.Since(lastEditedAt) < telegramDraftThrottle {
		return nil
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}

	if err := s.adapter.waitStreamLimit(ctx); err != nil {
		return err
	}
	bot, err := s.getBot(ctx)
	if err != nil {
		return err
	}

	draftErr := sendTelegramDraft(bot, s.streamChatID, s.draftID, text, s.parseMode)
	if draftErr != nil {
		if isTelegramTooManyRequests(draftErr) {
			d := getTelegramRetryAfter(draftErr)
			if d <= 0 {
				d = telegramDraftThrottle
			}
			s.mu.Lock()
			s.lastEditedAt = time.Now().Add(d)
			s.mu.Unlock()
			return nil
		}
		return draftErr
	}

	s.mu.Lock()
	s.lastEditedAt = time.Now()
	s.mu.Unlock()
	return nil
}

// sendPermanentMessage sends a final, permanent message via sendMessage.
// Used in draft mode to commit text after streaming is complete for a phase.
func (s *telegramOutboundStream) sendPermanentMessage(ctx context.Context, text string, parseMode string) error {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if err := s.adapter.waitStreamLimit(ctx); err != nil {
		return err
	}
	bot, replyTo, err := s.getBotAndReply(ctx)
	if err != nil {
		return err
	}
	return sendTelegramText(bot, s.target, text, replyTo, parseMode)
}

// resetStreamState clears the streaming message state so a fresh message will
// be created on the next delta. Must be called without holding s.mu.
func (s *telegramOutboundStream) resetStreamState() {
	s.mu.Lock()
	s.streamMsgID = 0
	if !s.isPrivateChat {
		s.streamChatID = 0
	}
	s.lastEdited = ""
	s.lastEditedAt = time.Time{}
	s.buf.Reset()
	s.mu.Unlock()
}

// deliverFinalText sends or edits the final text depending on chat mode.
func (s *telegramOutboundStream) deliverFinalText(ctx context.Context, text, parseMode string) error {
	if s.isPrivateChat {
		return s.sendPermanentMessage(ctx, text, parseMode)
	}
	if err := s.ensureStreamMessage(ctx, text); err != nil {
		return err
	}
	return s.editStreamMessageFinal(ctx, text)
}

func (s *telegramOutboundStream) pushToolCallStart(ctx context.Context, tc *channel.StreamToolCall) error {
	s.mu.Lock()
	bufText := strings.TrimSpace(s.buf.String())
	hasMsg := s.streamMsgID != 0
	s.mu.Unlock()
	if bufText != "" {
		bufText = s.formatStreamContent(bufText)
	}
	if s.isPrivateChat {
		// In draft mode, send buffered text as a permanent message before tool execution.
		if bufText != "" {
			if err := s.sendPermanentMessage(ctx, bufText, s.parseMode); err != nil {
				if s.adapter != nil && s.adapter.logger != nil {
					s.adapter.logger.Warn("telegram: draft permanent message failed", slog.Any("error", err))
				}
			}
		}
	} else if hasMsg && bufText != "" {
		_ = s.editStreamMessageFinal(ctx, bufText)
	}
	s.resetStreamState()
	return s.sendToolCallMessage(ctx, tc, channel.BuildToolCallStart(tc))
}

func (s *telegramOutboundStream) pushToolCallEnd(ctx context.Context, tc *channel.StreamToolCall) error {
	s.resetStreamState()
	return s.sendToolCallMessage(ctx, tc, channel.BuildToolCallEnd(tc))
}

// renderToolCallPresentation renders a tool-call presentation to IM-ready
// text and parseMode. It prefers Markdown→HTML; falls back to plain text when
// Markdown conversion yields an empty parseMode.
func renderToolCallPresentation(p channel.ToolCallPresentation) (string, string) {
	rendered := strings.TrimSpace(channel.RenderToolCallMessageMarkdown(p))
	if rendered == "" {
		return "", ""
	}
	text, parseMode := formatTelegramOutput(rendered, channel.MessageFormatMarkdown)
	if parseMode == "" {
		text = strings.TrimSpace(channel.RenderToolCallMessage(p))
	}
	return text, parseMode
}

// sendToolCallMessage renders the tool-call presentation. For tool_call_start
// it posts a new message and records the callID→message mapping. For
// tool_call_end it edits the previously-posted message to flip the status to
// completed/failed. If no prior message is tracked (or editing fails), it
// falls back to sending a fresh message.
func (s *telegramOutboundStream) sendToolCallMessage(
	ctx context.Context,
	tc *channel.StreamToolCall,
	p channel.ToolCallPresentation,
) error {
	text, parseMode := renderToolCallPresentation(p)
	if text == "" {
		return nil
	}
	callID := ""
	if tc != nil {
		callID = strings.TrimSpace(tc.CallID)
	}
	if p.Status != channel.ToolCallStatusRunning && callID != "" {
		if existing, ok := s.lookupToolCallMessage(callID); ok {
			if err := s.adapter.waitStreamLimit(ctx); err != nil {
				return err
			}
			bot, err := s.getBot(ctx)
			if err != nil {
				return err
			}
			editErr := error(nil)
			switch {
			case p.Status == channel.ToolCallStatusApprovalRequired && len(tcActions(tc)) > 0 && testEditFunc == nil:
				editErr = editTelegramMessageTextWithActions(bot, existing.chatID, existing.msgID, text, parseMode, tcActions(tc))
			case (p.Status == channel.ToolCallStatusCompleted || p.Status == channel.ToolCallStatusFailed) && existing.hasActions && testEditFunc == nil:
				editErr = editTelegramMessageTextWithActions(bot, existing.chatID, existing.msgID, text, parseMode, nil)
			case testEditFunc != nil:
				editErr = testEditFunc(bot, existing.chatID, existing.msgID, text, parseMode)
			default:
				editErr = editTelegramMessageText(bot, existing.chatID, existing.msgID, text, parseMode)
			}
			if editErr == nil {
				if p.Status != channel.ToolCallStatusApprovalRequired {
					s.forgetToolCallMessage(callID)
				} else if len(tcActions(tc)) > 0 {
					existing.hasActions = true
					s.storeToolCallMessage(callID, existing)
				}
				return nil
			}
			if s.adapter != nil && s.adapter.logger != nil {
				s.adapter.logger.Warn("telegram: tool-call end edit failed, falling back to new message",
					slog.String("call_id", callID),
					slog.Any("error", editErr),
				)
			}
			s.forgetToolCallMessage(callID)
		}
	}
	if err := s.adapter.waitStreamLimit(ctx); err != nil {
		return err
	}
	bot, replyTo, err := s.getBotAndReply(ctx)
	if err != nil {
		return err
	}
	var (
		chatID  int64
		msgID   int
		sendErr error
	)
	if len(tcActions(tc)) > 0 {
		chatID, msgID, sendErr = sendTelegramTextWithActionsReturnMessage(bot, s.target, text, replyTo, parseMode, tcActions(tc))
	} else {
		chatID, msgID, sendErr = sendTelegramTextReturnMessage(bot, s.target, text, replyTo, parseMode)
	}
	if sendErr != nil {
		return sendErr
	}
	if p.Status == channel.ToolCallStatusRunning && callID != "" {
		s.storeToolCallMessage(callID, telegramToolCallMessage{chatID: chatID, msgID: msgID, hasActions: len(tcActions(tc)) > 0})
	}
	return nil
}

func tcActions(tc *channel.StreamToolCall) []channel.Action {
	if tc == nil {
		return nil
	}
	return tc.Actions
}

func (s *telegramOutboundStream) lookupToolCallMessage(callID string) (telegramToolCallMessage, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolMessages == nil {
		return telegramToolCallMessage{}, false
	}
	m, ok := s.toolMessages[callID]
	return m, ok
}

func (s *telegramOutboundStream) storeToolCallMessage(callID string, m telegramToolCallMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolMessages == nil {
		s.toolMessages = make(map[string]telegramToolCallMessage)
	}
	s.toolMessages[callID] = m
}

func (s *telegramOutboundStream) forgetToolCallMessage(callID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolMessages == nil {
		return
	}
	delete(s.toolMessages, callID)
}

func (s *telegramOutboundStream) pushAttachment(ctx context.Context, event channel.PreparedStreamEvent) error {
	if len(event.Attachments) == 0 {
		return nil
	}
	bot, replyTo, err := s.getBotAndReply(ctx)
	if err != nil {
		return err
	}
	for _, att := range event.Attachments {
		if sendErr := sendTelegramAttachmentWithAssets(ctx, bot, s.target, att, "", replyTo, ""); sendErr != nil {
			if s.adapter != nil && s.adapter.logger != nil {
				s.adapter.logger.Warn("telegram: stream attachment send failed",
					slog.String("config_id", s.cfg.ID),
					slog.String("type", string(att.Logical.Type)),
					slog.Any("error", sendErr),
				)
			}
		}
	}
	return nil
}

func (s *telegramOutboundStream) pushPhaseEnd(ctx context.Context, event channel.PreparedStreamEvent) error {
	if event.Phase != channel.StreamPhaseText {
		return nil
	}
	// In draft mode, skip phase-end finalization; StreamEventFinal sends the
	// permanent formatted message.
	if s.isPrivateChat {
		return nil
	}
	s.mu.Lock()
	finalText := strings.TrimSpace(s.buf.String())
	s.mu.Unlock()
	if finalText != "" {
		finalText = s.formatStreamContent(finalText)
		if err := s.ensureStreamMessage(ctx, finalText); err != nil {
			return err
		}
		return s.editStreamMessageFinal(ctx, finalText)
	}
	return nil
}

func (s *telegramOutboundStream) pushDelta(ctx context.Context, event channel.PreparedStreamEvent) error {
	if event.Delta == "" || event.Phase == channel.StreamPhaseReasoning {
		return nil
	}
	s.mu.Lock()
	s.buf.WriteString(event.Delta)
	content := s.buf.String()
	s.mu.Unlock()
	content = s.formatStreamContent(content)
	if s.isPrivateChat {
		return s.sendDraft(ctx, content)
	}
	if err := s.ensureStreamMessage(ctx, content); err != nil {
		return err
	}
	return s.editStreamMessage(ctx, content)
}

func (s *telegramOutboundStream) pushFinal(ctx context.Context, event channel.PreparedStreamEvent) error {
	// In draft mode, read and reset buffer atomically to prevent duplicate
	// permanent messages when multiple StreamEventFinal events fire
	// (one per assistant output in multi-tool-call responses).
	s.mu.Lock()
	bufText := strings.TrimSpace(s.buf.String())
	if s.isPrivateChat {
		s.buf.Reset()
	}
	s.mu.Unlock()

	if event.Final == nil || event.Final.Message.Message.IsEmpty() {
		if bufText != "" {
			bufText = s.formatStreamContent(bufText)
			if err := s.deliverFinalText(ctx, bufText, s.parseMode); err != nil {
				if s.adapter != nil && s.adapter.logger != nil {
					s.adapter.logger.Warn("telegram: deliver buffered final text failed", slog.Any("error", err))
				}
			}
		}
		return nil
	}

	msg := event.Final.Message
	finalText := bufText
	if authoritative := strings.TrimSpace(msg.Message.PlainText()); authoritative != "" {
		if !s.isPrivateChat || bufText != "" || (finalText == "" && len(msg.Message.Actions) > 0) {
			finalText = authoritative
		}
	}
	// Convert markdown to Telegram HTML for the final message.
	formatted, pm := formatTelegramOutput(finalText, msg.Message.Format)
	if pm != "" {
		s.mu.Lock()
		s.parseMode = pm
		s.mu.Unlock()
		finalText = formatted
	}

	if len(msg.Message.Actions) > 0 && len(msg.Attachments) == 0 {
		bot, replyTo, err := s.getBotAndReply(ctx)
		if err != nil {
			return err
		}
		if err := sendTelegramTextWithActions(bot, s.target, finalText, replyTo, s.parseMode, msg.Message.Actions); err != nil {
			return err
		}
	} else if err := s.deliverFinalText(ctx, finalText, s.parseMode); err != nil {
		return err
	}

	if len(msg.Attachments) > 0 {
		bot, err := s.getBot(ctx)
		if err != nil {
			return err
		}
		replyTo := parseReplyToMessageID(s.reply)
		parseMode := s.parseMode
		for i, att := range msg.Attachments {
			to := replyTo
			if i > 0 {
				to = 0
			}
			if err := sendTelegramAttachmentWithAssets(ctx, bot, s.target, att, "", to, parseMode); err != nil && s.adapter.logger != nil {
				s.adapter.logger.Error("stream final attachment failed", slog.String("config_id", s.cfg.ID), slog.Any("error", err))
			}
		}
	}
	return nil
}

func (s *telegramOutboundStream) pushError(ctx context.Context, event channel.PreparedStreamEvent) error {
	errText := channel.RedactIMErrorText(strings.TrimSpace(event.Error))
	if errText == "" {
		return nil
	}
	display := "Error: " + errText
	// Error messages are plain text; reset parseMode so HTML-mode
	// left over from earlier deltas does not corrupt the output.
	s.mu.Lock()
	s.parseMode = ""
	s.mu.Unlock()
	if s.isPrivateChat {
		return s.sendPermanentMessage(ctx, display, "")
	}
	if err := s.ensureStreamMessage(ctx, display); err != nil {
		return err
	}
	return s.editStreamMessage(ctx, display)
}

func (s *telegramOutboundStream) Push(ctx context.Context, event channel.PreparedStreamEvent) error {
	if s == nil || s.adapter == nil {
		return errors.New("telegram stream not configured")
	}
	if s.closed.Load() {
		return errors.New("telegram stream is closed")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	switch event.Type {
	case channel.StreamEventToolCallStart:
		return s.pushToolCallStart(ctx, event.ToolCall)
	case channel.StreamEventToolCallEnd:
		return s.pushToolCallEnd(ctx, event.ToolCall)
	case channel.StreamEventAttachment:
		return s.pushAttachment(ctx, event)
	case channel.StreamEventPhaseEnd:
		return s.pushPhaseEnd(ctx, event)
	case channel.StreamEventDelta:
		return s.pushDelta(ctx, event)
	case channel.StreamEventFinal:
		return s.pushFinal(ctx, event)
	case channel.StreamEventError:
		return s.pushError(ctx, event)
	default:
		return nil
	}
}

// formatStreamContent applies markdown-to-HTML conversion for the accumulated
// stream buffer text and updates parseMode accordingly. Safe for incomplete
// markdown — unclosed constructs are left as plain text.
func (s *telegramOutboundStream) formatStreamContent(text string) string {
	if channel.ContainsMarkdown(text) {
		formatted, pm := formatTelegramOutput(text, channel.MessageFormatMarkdown)
		if pm != "" {
			s.mu.Lock()
			s.parseMode = pm
			s.mu.Unlock()
			return formatted
		}
	}
	return text
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
