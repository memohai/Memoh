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
		if event.Final != nil && !event.Final.Message.Message.IsEmpty() {
			finalText = renderDiscordStreamFinalText(event.Final.Message.Message, bufText)
		}
		if finalText != "" {
			actions := []channel.Action(nil)
			if event.Final != nil {
				actions = event.Final.Message.Message.Actions
			}
			return s.finalizeMessage(finalText, actions)
		}
		return nil

	case channel.StreamEventError:
		errText := channel.RedactIMErrorText(strings.TrimSpace(event.Error))
		if errText == "" {
			return nil
		}
		return s.finalizeMessage("Error: "+errText, nil)

	case channel.StreamEventAttachment:
		if len(event.Attachments) == 0 {
			return nil
		}
		// Finalize current text message before sending attachments
		s.mu.Lock()
		finalText := strings.TrimSpace(s.buffer.String())
		s.mu.Unlock()
		if finalText != "" {
			if err := s.finalizeMessage(finalText, nil); err != nil {
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
			if err := s.finalizeMessage(bufText, nil); err != nil {
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

func renderDiscordStreamFinalText(msg channel.Message, buffered string) string {
	if rich := renderDiscordMessagePartsMarkdown(msg); rich != "" {
		return rich
	}
	if text := strings.TrimSpace(buffered); text != "" {
		return text
	}
	return strings.TrimSpace(msg.PlainText())
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

func (s *discordOutboundStream) finalizeMessage(text string, actions []channel.Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	text = truncateDiscordText(text)
	components, err := discordURLActionComponents(actions)
	if err != nil {
		return err
	}

	if s.msgID == "" {
		var msg *discordgo.Message
		switch {
		case s.reply != nil && s.reply.MessageID != "":
			messageSend := &discordgo.MessageSend{
				Content:    text,
				Components: components,
				Reference: &discordgo.MessageReference{
					ChannelID: s.target,
					MessageID: s.reply.MessageID,
				},
			}
			msg, err = s.session.ChannelMessageSendComplex(s.target, messageSend)
		case len(components) > 0:
			msg, err = s.session.ChannelMessageSendComplex(s.target, &discordgo.MessageSend{
				Content:    text,
				Components: components,
			})
		default:
			msg, err = s.session.ChannelMessageSend(s.target, text)
		}
		if err != nil {
			return err
		}
		s.msgID = msg.ID
		s.lastUpdate = time.Now()
		return nil
	}

	if len(components) > 0 {
		edit := discordgo.NewMessageEdit(s.target, s.msgID)
		edit.SetContent(text)
		edit.Components = &components
		_, err := s.session.ChannelMessageEditComplex(edit)
		return err
	}
	_, err = s.session.ChannelMessageEdit(s.target, s.msgID, text)
	return err
}

// sendToolCallMessage posts a Discord message on tool_call_start and edits it
// on tool_call_end so the running → completed/failed transition is contained
// in one visible post. Falls back to a new message if the edit fails.
func (s *discordOutboundStream) sendToolCallMessage(tc *channel.StreamToolCall, p channel.ToolCallPresentation) error {
	text := truncateDiscordText(strings.TrimSpace(channel.RenderToolCallMessageMarkdown(p)))
	if text == "" {
		return nil
	}
	callID := ""
	if tc != nil {
		callID = strings.TrimSpace(tc.CallID)
	}
	if p.Status != channel.ToolCallStatusRunning && callID != "" {
		if msgID, ok := s.lookupToolCallMessage(callID); ok {
			if _, err := s.session.ChannelMessageEdit(s.target, msgID, text); err == nil {
				s.forgetToolCallMessage(callID)
				return nil
			}
			s.forgetToolCallMessage(callID)
		}
	}
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
	if p.Status == channel.ToolCallStatusRunning && callID != "" && msg != nil && msg.ID != "" {
		s.storeToolCallMessage(callID, msg.ID)
	}
	return nil
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
