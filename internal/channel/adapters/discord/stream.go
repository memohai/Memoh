package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/memohai/memoh/internal/channel"
)

type discordOutboundStream struct {
	adapter      *DiscordAdapter
	cfg          channel.ChannelConfig
	target       string
	reply        *channel.ReplyRef
	session      *discordgo.Session
	closed       atomic.Bool
	mu           sync.Mutex
	msgID        string
	buffer       strings.Builder
	lastUpdate   time.Time
	toolMessages map[string]string
}

func (s *discordOutboundStream) Push(ctx context.Context, event channel.PreparedStreamEvent) error {
	if s == nil || s.adapter == nil {
		return errors.New("discord stream not configured")
	}
	if s.closed.Load() {
		return errors.New("discord stream is closed")
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	switch event.Type {
	case channel.StreamEventStatus:
		if event.Status == channel.StreamStatusStarted {
			return s.ensureMessage("Thinking...")
		}
		return nil

	case channel.StreamEventDelta:
		if event.Delta == "" || event.Phase == channel.StreamPhaseReasoning {
			return nil
		}
		s.mu.Lock()
		s.buffer.WriteString(event.Delta)
		s.mu.Unlock()

		// Discord has strict rate limits, only update periodically
		if time.Since(s.lastUpdate) > 2*time.Second {
			return s.updateMessage()
		}
		return nil

	case channel.StreamEventFinal:
		s.mu.Lock()
		bufText := strings.TrimSpace(s.buffer.String())
		s.mu.Unlock()
		finalText := bufText
		if finalText == "" && event.Final != nil && !event.Final.Message.Message.IsEmpty() {
			finalText = strings.TrimSpace(event.Final.Message.Message.PlainText())
		}
		if finalText != "" {
			return s.finalizeMessage(finalText)
		}
		return nil

	case channel.StreamEventError:
		errText := channel.RedactIMErrorText(strings.TrimSpace(event.Error))
		if errText == "" {
			return nil
		}
		return s.finalizeMessage("Error: " + errText)

	case channel.StreamEventAttachment:
		if len(event.Attachments) == 0 {
			return nil
		}
		// Finalize current text message before sending attachments
		s.mu.Lock()
		finalText := strings.TrimSpace(s.buffer.String())
		s.mu.Unlock()
		if finalText != "" {
			if err := s.finalizeMessage(finalText); err != nil {
				return err
			}
		}
		// Send attachments
		for _, att := range event.Attachments {
			if err := s.sendAttachment(ctx, att); err != nil {
				return err
			}
		}
		return nil

	case channel.StreamEventToolCallStart:
		s.mu.Lock()
		bufText := strings.TrimSpace(s.buffer.String())
		s.mu.Unlock()
		if bufText != "" {
			if err := s.finalizeMessage(bufText); err != nil {
				return err
			}
		}
		s.resetStreamState()
		return s.sendToolCallMessage(event.ToolCall, channel.BuildToolCallStart(event.ToolCall))
	case channel.StreamEventToolCallEnd:
		return s.sendToolCallMessage(event.ToolCall, channel.BuildToolCallEnd(event.ToolCall))

	case channel.StreamEventAgentStart, channel.StreamEventAgentEnd, channel.StreamEventPhaseStart, channel.StreamEventPhaseEnd, channel.StreamEventProcessingStarted, channel.StreamEventProcessingCompleted, channel.StreamEventProcessingFailed:
		// Status events - no action needed for Discord
		return nil

	default:
		return fmt.Errorf("unsupported stream event type: %s", event.Type)
	}
}

func (s *discordOutboundStream) Close(ctx context.Context) error {
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

func (s *discordOutboundStream) ensureMessage(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.msgID != "" {
		return nil
	}

	content := truncateDiscordText(text)

	var msg *discordgo.Message
	var err error
	if s.reply != nil && s.reply.MessageID != "" {
		msg, err = s.session.ChannelMessageSendReply(s.target, content, &discordgo.MessageReference{
			ChannelID: s.target,
			MessageID: s.reply.MessageID,
		})
	} else {
		msg, err = s.session.ChannelMessageSend(s.target, content)
	}
	if err != nil {
		return err
	}

	s.msgID = msg.ID
	s.lastUpdate = time.Now()
	return nil
}

func (s *discordOutboundStream) updateMessage() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.msgID == "" {
		return nil
	}

	content := s.buffer.String()
	if content == "" {
		return nil
	}

	content = truncateDiscordText(content)

	_, err := s.session.ChannelMessageEdit(s.target, s.msgID, content)
	if err != nil {
		return err
	}

	s.lastUpdate = time.Now()
	return nil
}

func (s *discordOutboundStream) finalizeMessage(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	text = truncateDiscordText(text)

	if s.msgID == "" {
		var msg *discordgo.Message
		var err error
		if s.reply != nil && s.reply.MessageID != "" {
			msg, err = s.session.ChannelMessageSendReply(s.target, text, &discordgo.MessageReference{
				ChannelID: s.target,
				MessageID: s.reply.MessageID,
			})
		} else {
			msg, err = s.session.ChannelMessageSend(s.target, text)
		}
		if err != nil {
			return err
		}
		s.msgID = msg.ID
		s.lastUpdate = time.Now()
		return nil
	}

	_, err := s.session.ChannelMessageEdit(s.target, s.msgID, text)
	return err
}

// sendToolCallMessage posts a Discord message on tool_call_start and edits it
// on tool_call_end so the running → completed/failed transition is contained
// in one visible post. Falls back to a new message if the edit fails.
func (s *discordOutboundStream) sendToolCallMessage(tc *channel.StreamToolCall, p channel.ToolCallPresentation) error {
	payload := renderDiscordToolCallMessage(p)
	text := payload.Content
	if text == "" {
		return nil
	}
	callID := ""
	if tc != nil {
		callID = strings.TrimSpace(tc.CallID)
	}
	if p.Status != channel.ToolCallStatusRunning && callID != "" {
		if msgID, ok := s.lookupToolCallMessage(callID); ok {
			if _, err := s.session.ChannelMessageEditComplex(discordToolCallMessageEdit(s.target, msgID, payload)); err == nil {
				s.forgetToolCallMessage(callID)
				return nil
			}
			if payload.Embed != nil {
				if _, err := s.session.ChannelMessageEdit(s.target, msgID, text); err == nil {
					s.forgetToolCallMessage(callID)
					return nil
				}
			}
			s.forgetToolCallMessage(callID)
		}
	}
	msg, err := s.session.ChannelMessageSendComplex(s.target, s.discordToolCallMessageSend(payload))
	if err != nil && payload.Embed != nil {
		msg, err = s.session.ChannelMessageSendComplex(s.target, s.discordToolCallMessageSend(discordToolCallPayload{Content: text}))
	}
	if err != nil {
		return err
	}
	if p.Status == channel.ToolCallStatusRunning && callID != "" && msg != nil && msg.ID != "" {
		s.storeToolCallMessage(callID, msg.ID)
	}
	return nil
}

func (s *discordOutboundStream) discordToolCallMessageSend(payload discordToolCallPayload) *discordgo.MessageSend {
	messageSend := &discordgo.MessageSend{
		Content: payload.Content,
	}
	if payload.Embed != nil {
		messageSend.Embeds = []*discordgo.MessageEmbed{payload.Embed}
	}
	if s.reply != nil && s.reply.MessageID != "" {
		messageSend.Reference = &discordgo.MessageReference{
			ChannelID: s.target,
			MessageID: s.reply.MessageID,
		}
	}
	return messageSend
}

func discordToolCallMessageEdit(channelID, messageID string, payload discordToolCallPayload) *discordgo.MessageEdit {
	edit := discordgo.NewMessageEdit(channelID, messageID).SetContent(payload.Content)
	if payload.Embed != nil {
		edit.SetEmbeds([]*discordgo.MessageEmbed{payload.Embed})
	}
	return edit
}

func (s *discordOutboundStream) lookupToolCallMessage(callID string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolMessages == nil {
		return "", false
	}
	v, ok := s.toolMessages[callID]
	return v, ok
}

func (s *discordOutboundStream) storeToolCallMessage(callID, msgID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolMessages == nil {
		s.toolMessages = make(map[string]string)
	}
	s.toolMessages[callID] = msgID
}

func (s *discordOutboundStream) forgetToolCallMessage(callID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.toolMessages == nil {
		return
	}
	delete(s.toolMessages, callID)
}

func (s *discordOutboundStream) resetStreamState() {
	s.mu.Lock()
	s.msgID = ""
	s.buffer.Reset()
	s.lastUpdate = time.Time{}
	s.mu.Unlock()
}

func (s *discordOutboundStream) sendAttachment(ctx context.Context, att channel.PreparedAttachment) error {
	file, err := discordPreparedAttachmentToFile(ctx, att)
	if err != nil {
		return err
	}

	messageSend := &discordgo.MessageSend{
		Files: []*discordgo.File{file},
	}

	// Add reply reference if this is the first message and we have a reply target
	if s.reply != nil && s.reply.MessageID != "" {
		messageSend.Reference = &discordgo.MessageReference{
			ChannelID: s.target,
			MessageID: s.reply.MessageID,
		}
	}

	_, err = s.session.ChannelMessageSendComplex(s.target, messageSend)
	return err
}
